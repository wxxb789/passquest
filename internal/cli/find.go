package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/find"
	"github.com/wxxb789/passquest/internal/search"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
)

const (
	findDefaultMaxQueries    = 5
	findDefaultMaxCandidates = 20
)

func runFind(args []string, opt Options) int {
	if len(args) > 0 {
		switch args[0] {
		case "history":
			return runFindHistory(args[1:], opt)
		case "show":
			return runFindShow(args[1:], opt)
		}
	}
	return runFindQuery(args, opt)
}

func runFindQuery(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	fs := newFlagSet("find", opt, wantJSON)
	maxQueriesFlag := fs.Int("max-queries", findDefaultMaxQueries, "daily query budget (1-100)")
	maxCandidatesFlag := fs.Int("max-candidates", findDefaultMaxCandidates, "candidate budget for this run (1-100)")
	timeoutFlag := fs.Int("timeout-seconds", search.DefaultTimeoutMS/1000, "provider timeout in seconds (1-120)")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	if *maxQueriesFlag < 1 || *maxQueriesFlag > 100 {
		return usageError(opt, *jsonFlag, "invalid_input", fmt.Sprintf("--max-queries %d is outside 1..100", *maxQueriesFlag))
	}
	if *maxCandidatesFlag < 1 || *maxCandidatesFlag > 100 {
		return usageError(opt, *jsonFlag, "invalid_input", fmt.Sprintf("--max-candidates %d is outside 1..100", *maxCandidatesFlag))
	}
	if *timeoutFlag < search.MinTimeoutMS/1000 || *timeoutFlag > search.MaxTimeoutMS/1000 {
		return usageError(opt, *jsonFlag, "invalid_input", fmt.Sprintf("--timeout-seconds %d is outside %d..%d",
			*timeoutFlag, search.MinTimeoutMS/1000, search.MaxTimeoutMS/1000))
	}

	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()

	now := opt.Now().UTC()
	day := now.Format("2006-01-02")
	records, unclassified, err := findRecords(st)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	intent, found := find.Derive(records, *maxCandidatesFlag)
	endpoint := strings.TrimSpace(opt.Getenv(search.EndpointEnv))
	normalized, endpointErr := "", error(nil)
	if endpoint != "" {
		normalized, endpointErr = search.NormalizeEndpoint(endpoint)
	}

	usage, err := st.FindQueryUsage(day)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	budget := findBudgetViewOf(day, *maxQueriesFlag, *maxCandidatesFlag, usage.Used(), 0)
	provider := findProviderView{Configured: endpoint != "", Name: "", EndpointHost: ""}
	if endpointErr == nil {
		provider.EndpointHost = search.EndpointHost(normalized)
	}

	if !found {
		doc := findDoc{
			SchemaVersion:        ProtocolVersion,
			State:                "nothing_to_find",
			Budget:               budget,
			Provider:             provider,
			Candidates:           []findCandidateView{},
			Counts:               findCountsView{},
			UnclassifiedAttempts: len(unclassified),
			Message:              "nothing to find: no skill has three attempts and no wrong answer names a tactic.",
			Limitations:          findLimitations(false),
		}
		if *jsonFlag {
			return writeJSON(opt, doc)
		}
		writeFindNothing(opt.Stdout, doc)
		return 0
	}

	intentView := findIntentViewOf(intent)
	if endpoint == "" {
		doc := findDoc{
			SchemaVersion:        ProtocolVersion,
			State:                "no_provider",
			Intent:               &intentView,
			Budget:               budget,
			Provider:             provider,
			Candidates:           []findCandidateView{},
			Counts:               findCountsView{},
			UnclassifiedAttempts: len(unclassified),
			Message:              "no provider configured: set PASSQUEST_SEARCH_ENDPOINT (and optionally PASSQUEST_SEARCH_KEY). No network request was made.",
			Limitations:          findLimitations(true),
		}
		if *jsonFlag {
			return writeJSON(opt, doc)
		}
		writeFindNoProvider(opt.Stdout, doc)
		return 0
	}
	if endpointErr != nil {
		return usageError(opt, *jsonFlag, "invalid_endpoint", endpointErr.Error())
	}

	reservationID, usage, reserved, err := st.ReserveFindQuery(day, *maxQueriesFlag, now)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	if !reserved {
		message := fmt.Sprintf(
			"query budget exhausted: %d of %d queries used on %s UTC; 0 remain. No network request was made.",
			usage.Used(), *maxQueriesFlag, day,
		)
		return findFailure(opt, *jsonFlag, 2, "budget_exceeded", message, &intentView, budget, provider, 0)
	}
	budget = findBudgetViewOf(day, *maxQueriesFlag, *maxCandidatesFlag, usage.Used(), 0)
	result, searchErr := opt.SearchTransport.Search(context.Background(), search.Request{
		Endpoint:  normalized,
		Query:     intent.Query,
		Limit:     *maxCandidatesFlag,
		TimeoutMS: *timeoutFlag * 1000,
	})
	if searchErr != nil {
		failure, structured := search.AsProviderError(searchErr)
		if !structured {
			_ = st.ReleaseFindQueryReservation(reservationID)
			return internalError(opt, *jsonFlag, searchErr)
		}
		if !failure.RequestSent {
			_ = st.ReleaseFindQueryReservation(reservationID)
			if refreshed, usageErr := st.FindQueryUsage(day); usageErr == nil {
				budget = findBudgetViewOf(day, *maxQueriesFlag, *maxCandidatesFlag, refreshed.Used(), 0)
			}
			message := failure.Message
			if message == "" {
				message = "search host is unavailable"
			}
			return findFailure(opt, *jsonFlag, 3, failure.Code, message, &intentView, budget, provider, 0)
		}
		code := findErrorCode(failure.Code)
		status := code
		query := findStoredQuery(now, day, intent, intentView, failure.RequestSHA, failure.RequestSize,
			"", status, code, failure.Message, failure.HTTPStatus, failure.LatencyMS)
		query.EndpointHost = provider.EndpointHost
		stored, appendErr := st.AppendFindQuery(query, nil, reservationID)
		if appendErr != nil {
			return internalError(opt, *jsonFlag, appendErr)
		}
		message := failure.Message
		if message == "" {
			message = "provider request failed"
		}
		return findFailure(opt, *jsonFlag, 3, code, message, &intentView, budget, provider, stored.ID)
	}

	if len(result.Items) > *maxCandidatesFlag {
		message := fmt.Sprintf(
			"candidate budget exceeded: provider returned %d candidate(s) for a limit of %d; no candidates were stored (%d of %d remain).",
			len(result.Items), *maxCandidatesFlag, *maxCandidatesFlag, *maxCandidatesFlag,
		)
		query := findStoredQuery(now, day, intent, intentView, result.RequestSHA256, result.RequestBytes,
			result.Provider, "candidate_limit_exceeded", "budget_exceeded", message,
			result.HTTPStatus, result.LatencyMS)
		query.EndpointHost = provider.EndpointHost
		stored, appendErr := st.AppendFindQuery(query, nil, reservationID)
		if appendErr != nil {
			return internalError(opt, *jsonFlag, appendErr)
		}
		return findFailure(opt, *jsonFlag, 2, "budget_exceeded", message, &intentView, budget, provider, stored.ID)
	}

	classified := find.Classify(result.Items)
	counts := find.Count(classified)
	storedCandidates := findStoredCandidates(classified, result.Provider, intent.Query, now)
	status := "ok"
	state := "found"
	message := fmt.Sprintf(
		"%d candidate(s) stored; %d local-usable, %d link-only, %d generated-unverified. Only local-usable text is shown for local study.",
		counts.Total, counts.LocalUsable, counts.LinkOnly, counts.GeneratedUnverified,
	)
	switch {
	case counts.Total == 0:
		status = "empty"
		state = "empty"
		message = "provider returned no results for the bounded query."
	case counts.LinkOnly == counts.Total:
		status = "all_link_only"
		state = "all_link_only"
		message = fmt.Sprintf(
			"provider returned %d candidate(s); all are link-only because no open license was declared, so no local study text is available.",
			counts.Total,
		)
	}
	query := findStoredQuery(now, day, intent, intentView, result.RequestSHA256, result.RequestBytes,
		result.Provider, status, "", "", result.HTTPStatus, result.LatencyMS)
	query.EndpointHost = provider.EndpointHost
	stored, err := st.AppendFindQuery(query, storedCandidates, reservationID)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	budget.CandidatesStored = len(storedCandidates)
	provider.Name = result.Provider
	doc := findDoc{
		SchemaVersion:        ProtocolVersion,
		State:                state,
		Intent:               &intentView,
		Budget:               budget,
		Provider:             provider,
		QueryID:              stored.ID,
		Candidates:           findCandidateViewsOf(storedCandidates),
		Counts:               findCountsViewOf(counts),
		UnclassifiedAttempts: len(unclassified),
		Message:              message,
		Limitations:          findLimitations(true),
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeFind(opt.Stdout, doc)
	return 0
}

func runFindHistory(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	fs := newFlagSet("find history", opt, wantJSON)
	limitFlag := fs.Int("limit", 20, "number of stored queries (1-100)")
	maxQueriesFlag := fs.Int("max-queries", findDefaultMaxQueries, "daily query budget used for the remaining count (1-100)")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	if *limitFlag < 1 || *limitFlag > 100 {
		return usageError(opt, *jsonFlag, "invalid_input", fmt.Sprintf("--limit %d is outside 1..100", *limitFlag))
	}
	if *maxQueriesFlag < 1 || *maxQueriesFlag > 100 {
		return usageError(opt, *jsonFlag, "invalid_input", fmt.Sprintf("--max-queries %d is outside 1..100", *maxQueriesFlag))
	}
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	queries, err := st.FindQueries(*limitFlag)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	day := opt.Now().UTC().Format("2006-01-02")
	usage, err := st.FindQueryUsage(day)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	doc := findHistoryDoc{
		SchemaVersion: ProtocolVersion,
		State:         "history",
		Queries:       findQueryViewsOf(queries),
		Budget:        findBudgetViewOf(day, *maxQueriesFlag, findDefaultMaxCandidates, usage.Used(), 0),
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeFindHistory(opt.Stdout, doc)
	return 0
}

func runFindShow(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"db": true})
	fs := newFlagSet("find show", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 1, "find show QUERY [--db PATH] [--json]"); code != 0 {
		return code
	}
	id, err := parsePositiveInt64(fs.Arg(0))
	if err != nil {
		return usageError(opt, *jsonFlag, "invalid_input", fmt.Sprintf("invalid query id %q", fs.Arg(0)))
	}
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	query, candidates, found, err := st.FindQuery(id)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	if !found {
		return usageError(opt, *jsonFlag, "not_found", fmt.Sprintf("find query %d does not exist", id))
	}
	day := opt.Now().UTC().Format("2006-01-02")
	usage, err := st.FindQueryUsage(day)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	doc := findShowDoc{
		SchemaVersion: ProtocolVersion,
		State:         "query",
		Query:         findQueryViewOf(query),
		Candidates:    findCandidateViewsOf(candidates),
		Budget:        findBudgetViewOf(day, findDefaultMaxQueries, findDefaultMaxCandidates, usage.Used(), 0),
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeFindShow(opt.Stdout, doc)
	return 0
}

func findRecords(st *store.Store) ([]find.Record, []unclassifiedAttemptView, error) {
	input, unclassified, _, err := buildAdaptiveInput(st, "")
	if err != nil {
		return nil, nil, err
	}
	sections := make(map[string]string)
	sources := make(map[string]source.Source)
	records := make([]find.Record, 0, len(input.Records))
	for _, record := range input.Records {
		section, err := findExamSection(st, sections, sources, record.ContentID)
		if err != nil {
			return nil, nil, err
		}
		records = append(records, find.Record{
			AttemptID:     record.AttemptID,
			AttemptNumber: record.AttemptNumber,
			Assistance:    record.Assistance,
			Status:        record.Status,
			TacticID:      record.TacticID,
			Skills:        append([]string(nil), record.Skills...),
			ExamSection:   section,
		})
	}
	return records, unclassified, nil
}

func findExamSection(st *store.Store, cache map[string]string, sources map[string]source.Source, contentID string) (string, error) {
	if section, ok := cache[contentID]; ok {
		return section, nil
	}
	section := "unavailable"
	if _, embedded, err := embeddedContent(contentID); err != nil {
		return "", err
	} else if embedded {
		section = "embedded"
	} else if sourceID, pageNumber, ok := source.ParseContentID(contentID); ok {
		src, loaded := sources[sourceID]
		if !loaded {
			var err error
			src, err = st.Source(sourceID)
			if errors.Is(err, store.ErrNotFound) {
				cache[contentID] = section
				return section, nil
			}
			if err != nil {
				return "", err
			}
			sources[sourceID] = src
		}
		if page, ok := findPage(src, pageNumber); ok {
			if strings.TrimSpace(page.Section) == "" {
				section = "unspecified"
			} else {
				section = findClipText(strings.TrimSpace(page.Section), 100)
			}
		}
	}
	cache[contentID] = section
	return section, nil
}

func findStoredQuery(now time.Time, day string, intent find.Intent, view findIntentView, requestSHA string, requestBytes int,
	provider, status, code, message string, httpStatus int, latency int64) store.FindQuery {
	// The output document carries every contributing attempt id. The stored
	// audit bounds the list so a long-running learner cannot outgrow the row
	// limit; the counts above still describe the whole evidence set.
	storedView := view
	if len(storedView.Evidence.AttemptIDs) > 200 {
		storedView.Evidence.AttemptIDs = append([]int64(nil), storedView.Evidence.AttemptIDs[len(storedView.Evidence.AttemptIDs)-200:]...)
		storedView.Evidence.AttemptIDsTruncated = true
	}
	intentJSON, _ := json.Marshal(storedView)
	return store.FindQuery{
		Day:           day,
		CreatedAt:     now,
		IntentJSON:    string(intentJSON),
		SkillID:       intent.SkillID,
		TacticID:      intent.TacticID,
		ExamSection:   intent.ExamSection,
		LearnerLevel:  string(intent.Level),
		Query:         intent.Query,
		Provider:      provider,
		EndpointHost:  "",
		Status:        status,
		ErrorCode:     code,
		ErrorMessage:  findStoreMessage(message),
		HTTPStatus:    httpStatus,
		MaxCandidates: intent.CandidateLimit,
		RequestSHA256: requestSHA,
		RequestBytes:  requestBytes,
		LatencyMS:     latency,
	}
}

func findStoredCandidates(candidates []find.Candidate, provider, query string, at time.Time) []store.FindCandidate {
	out := make([]store.FindCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, store.FindCandidate{
			Provider:       provider,
			Query:          query,
			RetrievedAt:    at,
			Title:          candidate.Title,
			URL:            candidate.URL,
			Classification: string(candidate.Classification),
			License:        candidate.License,
			Reason:         candidate.Reason,
			Content:        candidate.Content,
			ContentSHA256:  candidate.ContentSHA256,
			Displayable:    candidate.Displayable,
		})
	}
	return out
}

func findErrorCode(code string) string {
	switch code {
	case "rate_limited", "auth_failed", "provider_error", "unreachable", "invalid_response", "timeout":
		return code
	case "network_error":
		return "unreachable"
	case "host_error":
		return "invalid_response"
	default:
		return "provider_error"
	}
}

func findClipText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func findStoreMessage(message string) string {
	message = find.SanitizeText(message)
	runes := []rune(message)
	if len(runes) > 2000 {
		runes = runes[:2000]
	}
	return string(runes)
}

func findFailure(opt Options, jsonMode bool, exitCode int, code, message string,
	intent *findIntentView, budget findBudgetView, provider findProviderView, queryID int64) int {
	if jsonMode {
		_ = writeJSONTo(opt.Stderr, findFailureDoc{
			SchemaVersion: ProtocolVersion,
			State:         code,
			Error:         errorBody{Code: code, Message: message},
			Intent:        intent,
			Budget:        budget,
			Provider:      provider,
			QueryID:       queryID,
		})
		return exitCode
	}
	fmt.Fprintf(opt.Stderr, "passquest: %s\n", message)
	writeFindBudget(opt.Stderr, budget, true)
	if queryID > 0 {
		fmt.Fprintf(opt.Stderr, "Stored query: %d (inspect with `passquest find show %d`).\n", queryID, queryID)
	}
	return exitCode
}

func findLimitations(withIntent bool) []string {
	out := []string{
		"Fetched text is untrusted data: it is never treated as an instruction, never sent to a model, and cannot change an attempt, grade, score or the next exercise.",
		"No candidate URL is fetched; only text the provider returned in its response is stored.",
		"A search candidate is not a verified exam item or answer key.",
		"An open license is provider-declared, not independently verified.",
	}
	if withIntent {
		out = append(out, "The learner level is an evidence-derived bucket, not a CEFR, junior-high or exam-readiness measurement.")
	}
	return out
}
