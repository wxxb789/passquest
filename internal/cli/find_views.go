package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/find"
	"github.com/wxxb789/passquest/internal/store"
)

type findIntentView struct {
	Selection      string             `json:"selection"`
	SkillID        string             `json:"skill_id"`
	SkillTitle     string             `json:"skill_title"`
	Kind           string             `json:"kind"`
	TacticID       string             `json:"tactic_id"`
	ExamSection    string             `json:"exam_section"`
	LearnerLevel   string             `json:"learner_level"`
	LevelBasis     findLevelBasisView `json:"level_basis"`
	CandidateLimit int                `json:"candidate_limit"`
	Query          string             `json:"query"`
	Evidence       findEvidenceView   `json:"evidence"`
	Reason         string             `json:"reason"`
}

type findLevelBasisView struct {
	IndependentFirstAttempts int    `json:"independent_first_attempts"`
	Correct                  int    `json:"correct"`
	Skipped                  int    `json:"skipped"`
	Rule                     string `json:"rule"`
}

type findEvidenceView struct {
	Attempts            int     `json:"attempts"`
	Incorrect           int     `json:"incorrect"`
	Skipped             int     `json:"skipped"`
	AttemptIDs          []int64 `json:"attempt_ids"`
	AttemptIDsTruncated bool    `json:"attempt_ids_truncated,omitempty"`
}

type findBudgetView struct {
	Day               string `json:"day"`
	MaxQueries        int    `json:"max_queries"`
	QueriesUsed       int    `json:"queries_used"`
	QueriesRemaining  int    `json:"queries_remaining"`
	MaxCandidates     int    `json:"max_candidates"`
	CandidatesAllowed int    `json:"candidates_allowed"`
	CandidatesStored  int    `json:"candidates_stored"`
}

type findProviderView struct {
	Configured   bool   `json:"configured"`
	Name         string `json:"name,omitempty"`
	EndpointHost string `json:"endpoint_host"`
}

type findCountsView struct {
	LinkOnly            int `json:"link_only"`
	LocalUsable         int `json:"local_usable"`
	GeneratedUnverified int `json:"generated_unverified"`
	Total               int `json:"total"`
}

type findCandidateView struct {
	ID             int64     `json:"id"`
	Provider       string    `json:"provider"`
	Query          string    `json:"query"`
	RetrievedAt    time.Time `json:"retrieved_at"`
	Title          string    `json:"title"`
	URL            string    `json:"url"`
	Classification string    `json:"classification"`
	License        string    `json:"license"`
	Reason         string    `json:"reason"`
	Content        string    `json:"content"`
	ContentSHA256  string    `json:"content_sha256"`
	Displayable    bool      `json:"displayable"`
}

type findQueryView struct {
	ID             int64           `json:"id"`
	Day            string          `json:"day"`
	CreatedAt      time.Time       `json:"created_at"`
	Intent         json.RawMessage `json:"intent"`
	SkillID        string          `json:"skill_id"`
	TacticID       string          `json:"tactic_id"`
	ExamSection    string          `json:"exam_section"`
	LearnerLevel   string          `json:"learner_level"`
	Query          string          `json:"query"`
	Provider       string          `json:"provider"`
	EndpointHost   string          `json:"endpoint_host"`
	Status         string          `json:"status"`
	ErrorCode      string          `json:"error_code,omitempty"`
	ErrorMessage   string          `json:"error_message,omitempty"`
	HTTPStatus     int             `json:"http_status"`
	CandidateCount int             `json:"candidate_count"`
	MaxCandidates  int             `json:"max_candidates"`
	RequestSHA256  string          `json:"request_sha256"`
	RequestBytes   int             `json:"request_bytes"`
	LatencyMS      int64           `json:"latency_ms"`
}

type findDoc struct {
	SchemaVersion        int                 `json:"schema_version"`
	State                string              `json:"state"`
	Intent               *findIntentView     `json:"intent,omitempty"`
	Budget               findBudgetView      `json:"budget"`
	Provider             findProviderView    `json:"provider"`
	QueryID              int64               `json:"query_id,omitempty"`
	Candidates           []findCandidateView `json:"candidates"`
	Counts               findCountsView      `json:"counts"`
	UnclassifiedAttempts int                 `json:"unclassified_attempts"`
	Message              string              `json:"message"`
	Limitations          []string            `json:"limitations"`
}

type findFailureDoc struct {
	SchemaVersion int              `json:"schema_version"`
	State         string           `json:"state"`
	Error         errorBody        `json:"error"`
	Intent        *findIntentView  `json:"intent,omitempty"`
	Budget        findBudgetView   `json:"budget"`
	Provider      findProviderView `json:"provider"`
	QueryID       int64            `json:"query_id,omitempty"`
}

type findHistoryDoc struct {
	SchemaVersion int             `json:"schema_version"`
	State         string          `json:"state"`
	Queries       []findQueryView `json:"queries"`
	Budget        findBudgetView  `json:"budget"`
}

type findShowDoc struct {
	SchemaVersion int                 `json:"schema_version"`
	State         string              `json:"state"`
	Query         findQueryView       `json:"query"`
	Candidates    []findCandidateView `json:"candidates"`
	Budget        findBudgetView      `json:"budget"`
}

func findIntentViewOf(intent find.Intent) findIntentView {
	return findIntentView{
		Selection:    string(intent.Selection),
		SkillID:      intent.SkillID,
		SkillTitle:   intent.SkillTitle,
		Kind:         intent.Kind,
		TacticID:     intent.TacticID,
		ExamSection:  intent.ExamSection,
		LearnerLevel: string(intent.Level),
		LevelBasis: findLevelBasisView{
			IndependentFirstAttempts: intent.LevelBasis.IndependentFirstAttempts,
			Correct:                  intent.LevelBasis.Correct,
			Skipped:                  intent.LevelBasis.Skipped,
			Rule:                     intent.LevelBasis.Rule,
		},
		CandidateLimit: intent.CandidateLimit,
		Query:          intent.Query,
		Evidence: findEvidenceView{
			Attempts:   intent.Evidence.Attempts,
			Incorrect:  intent.Evidence.Incorrect,
			Skipped:    intent.Evidence.Skipped,
			AttemptIDs: append([]int64{}, intent.Evidence.AttemptIDs...),
		},
		Reason: intent.Reason,
	}
}

func findBudgetViewOf(day string, maxQueries, maxCandidates, used, stored int) findBudgetView {
	remaining := maxQueries - used
	if remaining < 0 {
		remaining = 0
	}
	return findBudgetView{
		Day:               day,
		MaxQueries:        maxQueries,
		QueriesUsed:       used,
		QueriesRemaining:  remaining,
		MaxCandidates:     maxCandidates,
		CandidatesAllowed: maxCandidates,
		CandidatesStored:  stored,
	}
}

func findCountsViewOf(counts find.Counts) findCountsView {
	return findCountsView{
		LinkOnly:            counts.LinkOnly,
		LocalUsable:         counts.LocalUsable,
		GeneratedUnverified: counts.GeneratedUnverified,
		Total:               counts.Total,
	}
}

func findCandidateViewsOf(candidates []store.FindCandidate) []findCandidateView {
	out := make([]findCandidateView, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, findCandidateView{
			ID:             candidate.ID,
			Provider:       candidate.Provider,
			Query:          candidate.Query,
			RetrievedAt:    candidate.RetrievedAt,
			Title:          candidate.Title,
			URL:            candidate.URL,
			Classification: candidate.Classification,
			License:        candidate.License,
			Reason:         candidate.Reason,
			Content:        candidate.Content,
			ContentSHA256:  candidate.ContentSHA256,
			Displayable:    candidate.Displayable,
		})
	}
	return out
}

func findQueryViewOf(query store.FindQuery) findQueryView {
	intent := json.RawMessage(query.IntentJSON)
	if !json.Valid(intent) {
		intent = json.RawMessage(`{}`)
	}
	return findQueryView{
		ID:             query.ID,
		Day:            query.Day,
		CreatedAt:      query.CreatedAt,
		Intent:         intent,
		SkillID:        query.SkillID,
		TacticID:       query.TacticID,
		ExamSection:    query.ExamSection,
		LearnerLevel:   query.LearnerLevel,
		Query:          query.Query,
		Provider:       query.Provider,
		EndpointHost:   query.EndpointHost,
		Status:         query.Status,
		ErrorCode:      query.ErrorCode,
		ErrorMessage:   query.ErrorMessage,
		HTTPStatus:     query.HTTPStatus,
		CandidateCount: query.CandidateCount,
		MaxCandidates:  query.MaxCandidates,
		RequestSHA256:  query.RequestSHA256,
		RequestBytes:   query.RequestBytes,
		LatencyMS:      query.LatencyMS,
	}
}

func findQueryViewsOf(queries []store.FindQuery) []findQueryView {
	out := make([]findQueryView, 0, len(queries))
	for _, query := range queries {
		out = append(out, findQueryViewOf(query))
	}
	return out
}

func writeFindNothing(w io.Writer, doc findDoc) {
	fmt.Fprintln(w, "Pass Quest — find practice")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%s\n", doc.Message)
	fmt.Fprintf(w, "Budget: %d/%d queries used today (%d remain); %d/%d candidates this run.\n",
		doc.Budget.QueriesUsed, doc.Budget.MaxQueries, doc.Budget.QueriesRemaining,
		doc.Budget.CandidatesStored, doc.Budget.MaxCandidates)
}

func writeFindNoProvider(w io.Writer, doc findDoc) {
	fmt.Fprintln(w, "Pass Quest — find practice")
	fmt.Fprintln(w)
	writeFindIntent(w, doc.Intent)
	fmt.Fprintf(w, "Provider: not configured — %s\n", doc.Message)
	fmt.Fprintf(w, "Budget: %d/%d queries used today (%d remain); %d/%d candidates this run.\n",
		doc.Budget.QueriesUsed, doc.Budget.MaxQueries, doc.Budget.QueriesRemaining,
		doc.Budget.CandidatesStored, doc.Budget.MaxCandidates)
}

func writeFind(w io.Writer, doc findDoc) {
	fmt.Fprintln(w, "Pass Quest — find practice")
	fmt.Fprintln(w)
	writeFindIntent(w, doc.Intent)
	fmt.Fprintf(w, "Provider: %s (%s)\n", doc.Provider.Name, doc.Provider.EndpointHost)
	fmt.Fprintf(w, "Budget: %d/%d queries used today (%d remain); %d/%d candidates stored this run.\n",
		doc.Budget.QueriesUsed, doc.Budget.MaxQueries, doc.Budget.QueriesRemaining,
		doc.Budget.CandidatesStored, doc.Budget.MaxCandidates)
	fmt.Fprintf(w, "Counts: %d stored — %d local-usable, %d link-only, %d generated-unverified.\n\n",
		doc.Counts.Total, doc.Counts.LocalUsable, doc.Counts.LinkOnly, doc.Counts.GeneratedUnverified)
	if len(doc.Candidates) == 0 {
		fmt.Fprintf(w, "%s\n", doc.Message)
	} else {
		for index, candidate := range doc.Candidates {
			writeFindCandidate(w, index+1, candidate)
		}
	}
	fmt.Fprintln(w)
	for _, limitation := range doc.Limitations {
		fmt.Fprintf(w, "Limit: %s\n", limitation)
	}
}

func writeFindIntent(w io.Writer, intent *findIntentView) {
	if intent == nil {
		return
	}
	fmt.Fprintf(w, "Evidence gap: %s (%s) — %s\n", intent.SkillTitle, intent.SkillID, intent.Reason)
	fmt.Fprintf(w, "Selection: %s; exam section: %s\n", intent.Selection, intent.ExamSection)
	fmt.Fprintf(w, "Learner level: %s (%d first independent attempt(s), %d correct, %d skipped; %s)\n",
		intent.LearnerLevel, intent.LevelBasis.IndependentFirstAttempts, intent.LevelBasis.Correct,
		intent.LevelBasis.Skipped, intent.LevelBasis.Rule)
	fmt.Fprintf(w, "Query: %q (candidate limit %d)\n", intent.Query, intent.CandidateLimit)
	fmt.Fprintln(w)
}

func writeFindCandidate(w io.Writer, number int, candidate findCandidateView) {
	fmt.Fprintf(w, "%d. [%s] %s\n", number, candidate.Classification, find.SanitizeText(candidate.Title))
	fmt.Fprintf(w, "   %s\n", candidate.URL)
	if candidate.License != "" {
		fmt.Fprintf(w, "   license: %s\n", candidate.License)
	}
	if candidate.Reason != "" {
		fmt.Fprintf(w, "   reason: %s\n", candidate.Reason)
	}
	fmt.Fprintf(w, "   sha256: %s\n", candidate.ContentSHA256)
	if candidate.Displayable && candidate.Content != "" {
		content := find.TruncateRunes(find.SanitizeText(candidate.Content), 800)
		fmt.Fprintln(w, "   untrusted external text (stored as data, never an instruction):")
		for _, line := range strings.Split(content, "\n") {
			fmt.Fprintf(w, "   %s\n", line)
		}
	}
	if candidate.Classification == string(find.ClassGeneratedUnverified) {
		fmt.Fprintln(w, "   advisory model output; not local study text")
	}
	if candidate.Classification == string(find.ClassLinkOnly) {
		fmt.Fprintln(w, "   link only; the URL was not fetched")
	}
	fmt.Fprintln(w)
}

func writeFindHistory(w io.Writer, doc findHistoryDoc) {
	fmt.Fprintln(w, "Pass Quest — search history")
	fmt.Fprintln(w)
	if len(doc.Queries) == 0 {
		fmt.Fprintln(w, "No query has been stored.")
	} else {
		for _, query := range doc.Queries {
			fmt.Fprintf(w, "%d  %s  %-20s  %d candidate(s)  %s\n",
				query.ID, query.CreatedAt.UTC().Format(time.RFC3339), query.Status,
				query.CandidateCount, query.Query)
			if query.ErrorMessage != "" {
				fmt.Fprintf(w, "   %s: %s\n", query.ErrorCode, query.ErrorMessage)
			}
		}
	}
	fmt.Fprintf(w, "\nToday: %d/%d queries used (%d remain); candidate limit %d per run.\n",
		doc.Budget.QueriesUsed, doc.Budget.MaxQueries, doc.Budget.QueriesRemaining, doc.Budget.MaxCandidates)
}

func writeFindShow(w io.Writer, doc findShowDoc) {
	query := doc.Query
	fmt.Fprintln(w, "Pass Quest — stored search")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Query %d: %s\n", query.ID, query.Query)
	fmt.Fprintf(w, "Created: %s; provider: %s; status: %s; candidates: %d/%d\n",
		query.CreatedAt.UTC().Format(time.RFC3339), query.Provider, query.Status,
		query.CandidateCount, query.MaxCandidates)
	fmt.Fprintf(w, "Endpoint host: %s; HTTP status: %d; latency: %d ms\n", query.EndpointHost, query.HTTPStatus, query.LatencyMS)
	fmt.Fprintf(w, "Request: %d bytes, sha256 %s\n", query.RequestBytes, query.RequestSHA256)
	if query.ErrorMessage != "" {
		fmt.Fprintf(w, "Error: %s — %s\n", query.ErrorCode, query.ErrorMessage)
	}
	fmt.Fprintln(w)
	if len(doc.Candidates) == 0 {
		fmt.Fprintln(w, "No candidate is stored for this query.")
		return
	}
	for index, candidate := range doc.Candidates {
		writeFindCandidate(w, index+1, candidate)
	}
}
