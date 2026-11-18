package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/coach"
	"github.com/wxxb789/passquest/internal/study/models"
	"github.com/wxxb789/passquest/internal/tutor"
)

func runTutor(args []string, opt Options) int {
	if code := guardAssessmentCommand(args, opt, "tutor"); code != 0 {
		return code
	}
	if len(args) == 0 {
		return usageError(opt, jsonFlagRequested(args), "invalid_input",
			"tutor requires config, preview, ask, history, show, consent, revoke or consents")
	}
	switch args[0] {
	case "config":
		return runTutorConfig(args[1:], opt)
	case "preview":
		return runTutorPreview(args[1:], opt)
	case "ask":
		return runTutorAsk(args[1:], opt)
	case "history":
		return runTutorHistory(args[1:], opt)
	case "show":
		return runTutorShow(args[1:], opt)
	case "consent":
		return runTutorConsentCommand(args[1:], opt, "consent", true)
	case "revoke":
		return runTutorConsentCommand(args[1:], opt, "revoke", false)
	case "consents":
		return runTutorConsents(args[1:], opt)
	default:
		return usageError(opt, jsonFlagRequested(args), "invalid_input", fmt.Sprintf("unknown tutor command %q", args[0]))
	}
}

// runTutorPreview renders the exact request `tutor ask` would send: the
// configuration, target, chat content, text.chat evidence and credential
// state. Nothing is sent and no budget slot is claimed.
func runTutorPreview(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	fs := newFlagSet("tutor preview", opt, wantJSON)
	attemptFlag := fs.Int64("attempt", 0, "attempt id; 0 selects the latest attempt")
	modeFlag := fs.String("mode", string(tutor.ModeExplain), "coaching mode: explain, probe or micro_lesson")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	mode, err := coach.ParseMode(*modeFlag)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	svc, err := study.Open(opt.ports(*dbFlag))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	prepared, err := coach.Preview(context.Background(), svc, coach.Request{AttemptID: *attemptFlag, Mode: mode})
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	doc := tutorPreviewDoc{
		SchemaVersion: ProtocolVersion,
		State:         "preview",
		Attempt:       tutorAttemptViewOf(prepared.Attempt),
		Mode:          string(prepared.Mode),
		Provider:      tutorProviderViewOf(prepared.Config, opt),
		Context:       tutorContextViewOf(prepared.Preview.Context),
		Chat:          prepared.Preview.Request.Chat,
		RequestSHA256: prepared.Preview.RequestSHA,
		RequestBytes:  prepared.Preview.RequestBytes,
		Limits:        coach.Limits(),
	}
	if prepared.TextChat != nil {
		state := *prepared.TextChat
		doc.Provider.TextChatState = models.RenderCapability(state)
		if !state.Verified {
			doc.Provider.TextChatAction = models.TestCommand(prepared.Config.Provider, prepared.Config.Model)
		}
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeTutorPreview(opt.Stdout, doc)
	return 0
}

func writeTutorPreview(w io.Writer, doc tutorPreviewDoc) {
	fmt.Fprintln(w, "Pass Quest — tutor preview (not sent)")
	fmt.Fprintf(w, "Attempt #%d: %s / %s — %s, %d ms, %s\n",
		doc.Attempt.ID, doc.Attempt.ContentID, doc.Attempt.QuestionID,
		doc.Attempt.Status, doc.Attempt.ElapsedMS, doc.Attempt.Assistance)
	fmt.Fprintf(w, "Mode: %s\n", doc.Mode)
	fmt.Fprintf(w, "Provider: %s @ %s\n", doc.Provider.Model, doc.Provider.Endpoint)
	if doc.Provider.Provider != "" {
		fmt.Fprintf(w, "Registry provider: %s\n", doc.Provider.Provider)
	}
	if doc.Provider.TextChatState != "" {
		fmt.Fprintf(w, "text.chat: %s\n", doc.Provider.TextChatState)
		if doc.Provider.TextChatAction != "" {
			fmt.Fprintf(w, "Next: %s\n", doc.Provider.TextChatAction)
		}
	}
	if doc.Provider.CredentialPresent {
		fmt.Fprintf(w, "Credential: %s is set (value never printed)\n", doc.Provider.CredentialEnv)
	} else {
		fmt.Fprintf(w, "Credential: %s is missing; `tutor ask` will refuse\n", doc.Provider.CredentialEnv)
	}
	writeTutorPrivacy(w, doc.Context)
	if len(doc.Context.TruncatedFields) > 0 {
		fmt.Fprintf(w, "Truncated fields: %s\n", strings.Join(doc.Context.TruncatedFields, ", "))
	}
	fmt.Fprintf(w, "Provider request: %d bytes, sha256 %s\n", doc.RequestBytes, doc.RequestSHA256)
	fmt.Fprintln(w, "\n--- provider chat content (what would be sent) ---")
	for _, message := range doc.Chat.Messages {
		fmt.Fprintf(w, "[%s]\n%s\n\n", message.Role, message.Content)
	}
	fmt.Fprintln(w, "--- end provider chat content ---")
	if doc.Context.PrivateSource && !doc.Context.ConsentGranted && doc.Context.SourceID != "" {
		fmt.Fprintf(w, "\nPrivate source text is withheld. Inspect this preview, then authorize with:\n")
		fmt.Fprintf(w, "  passquest tutor consent %s --reviewer NAME --db PATH\n", doc.Context.SourceID)
	}
	fmt.Fprintf(w, "\nRun `passquest tutor ask --attempt %d --mode %s` to send this request.\n", doc.Attempt.ID, doc.Mode)
}

func writeTutorPrivacy(w io.Writer, context tutorContextView) {
	switch {
	case context.PrivateSource && context.ConsentGranted:
		fmt.Fprintf(w, "Privacy: private source %s is authorized; source text shared: yes\n", context.SourceID)
	case context.PrivateSource:
		fmt.Fprintf(w, "Privacy: private source %s is NOT authorized; source text shared: no\n", context.SourceID)
		fmt.Fprintf(w, "Withheld: %s\n", strings.Join(context.Withheld, ", "))
	default:
		fmt.Fprintln(w, "Privacy: first-party material only; no private source text is involved")
	}
	writeTutorContent(w, context)
	if len(context.Skills) > 0 {
		fmt.Fprintf(w, "Skills: %s\n", strings.Join(context.Skills, ", "))
	}
	if context.SkillsMappedFromCurrentTags {
		fmt.Fprintln(w, "Note: this pre-v6 attempt's skills come from the question's current tags, not a recorded snapshot.")
	}
}

// writeTutorContent states which attempt content the prompt carries. It is
// derived from the same flags Build used, so the summary cannot contradict the
// chat content or the withheld list printed with it.
func writeTutorContent(w io.Writer, context tutorContextView) {
	switch {
	case context.ContentIncluded:
		fmt.Fprintln(w, "Content: the current passage, question, options, verified key and evidence are included.")
		return
	case context.ContentChanged:
		fmt.Fprintln(w, "Content: the current version differs from the recorded attempt; no current passage, question, option, evidence or key text is included.")
	case context.ContentUnavailable:
		fmt.Fprintln(w, "Content: the attempted question is unavailable in this build; no current passage, question, option, evidence or key text is included.")
	default:
		fmt.Fprintln(w, "Content: the current passage and question are withheld because this private source is not authorized.")
	}
	if context.StoredTextIncluded {
		fmt.Fprintln(w, "Included instead: the recorded selection and verified key labels with their stored option texts.")
	} else {
		fmt.Fprintln(w, "Withheld as well: the recorded selection and verified key option texts.")
	}
}

// runTutorAsk sends one advisory request and renders its outcome: exit 0 with
// the advisory document, or exit 3 with the recorded failure on stderr.
func runTutorAsk(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	fs := newFlagSet("tutor ask", opt, wantJSON)
	attemptFlag := fs.Int64("attempt", 0, "attempt id; 0 selects the latest attempt")
	modeFlag := fs.String("mode", string(tutor.ModeExplain), "coaching mode: explain, probe or micro_lesson")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	mode, err := coach.ParseMode(*modeFlag)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	svc, err := study.Open(opt.ports(*dbFlag))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	out, err := coach.Ask(context.Background(), svc, coach.Request{AttemptID: *attemptFlag, Mode: mode})
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	failover := tutorFailoverViewOf(out.Failover)
	if !*jsonFlag && failover != nil {
		writeFailoverLine(opt.Stdout, failover)
	}
	if out.Failure == nil {
		doc := tutorAdvisoryDoc{
			SchemaVersion: ProtocolVersion,
			State:         "advisory",
			Call:          tutorCallViewOf(out.Call, false),
			Suggestion:    tutorSuggestionView{Text: out.Text, Advisory: true, VerifiedAnswer: false},
			Context:       tutorContextViewOf(out.Prepared.Preview.Context),
			Failover:      failover,
			Limits:        coach.Limits(),
		}
		if *jsonFlag {
			return writeJSON(opt, doc)
		}
		writeTutorAdvisory(opt.Stdout, doc, out.Prepared.Attempt, svc.Path())
		return 0
	}
	doc := tutorFailureDoc{
		SchemaVersion: ProtocolVersion,
		State:         "failed",
		Error:         errorBody{Code: out.Call.ErrorCode, Message: out.Call.ErrorMessage},
		Action:        out.Failure.Action,
		Call:          tutorCallViewOf(out.Call, false),
		Failover:      failover,
		Limits:        coach.Limits(),
	}
	if *jsonFlag {
		_ = writeJSONTo(opt.Stderr, doc)
	} else {
		fmt.Fprintf(opt.Stderr, "passquest: %s: %s\n", out.Call.ErrorCode, out.Call.ErrorMessage)
		if out.Failure.Action != "" {
			fmt.Fprintf(opt.Stderr, "Next: %s\n", out.Failure.Action)
		}
		fmt.Fprintf(opt.Stderr, "Advisory call #%d recorded as a failure; no attempt or answer key was changed.\n", out.Call.ID)
		fmt.Fprintf(opt.Stderr, "See `passquest tutor history` for usage and `passquest practice` to keep practicing locally.\n")
	}
	return 3
}

func writeFailoverLine(w io.Writer, view *tutorFailoverView) {
	if view == nil {
		return
	}
	if view.Applied {
		fmt.Fprintf(w, "failover: %s → %s (reason: %s)\n", view.FromProvider, view.ToProvider, view.Reason)
		return
	}
	target := view.ToProvider
	if target == "" {
		target = "no_eligible_provider"
	}
	if view.Detail != "" {
		fmt.Fprintf(w, "failover refused: %s → %s (reason: %s; %s)\n", view.FromProvider, target, view.Refusal, view.Detail)
		return
	}
	fmt.Fprintf(w, "failover refused: %s → %s (reason: %s)\n", view.FromProvider, target, view.Refusal)
}

func writeTutorAdvisory(w io.Writer, doc tutorAdvisoryDoc, record attempt.Attempt, dbPath string) {
	fmt.Fprintln(w, "Pass Quest — advisory AI suggestion (not a verified answer)")
	fmt.Fprintf(w, "Attempt #%d: %s / %s — %s, %d ms, %s\n",
		record.ID, record.ContentID, record.QuestionID, record.Status, record.ElapsedMS, record.Assistance)
	fmt.Fprintf(w, "Mode: %s\n", doc.Call.Mode)
	fmt.Fprintf(w, "Model: %s @ %s\n", doc.Call.Model, doc.Call.ProviderHost)
	writeTutorPrivacy(w, doc.Context)
	if doc.Call.PromptTokens != nil && doc.Call.CompletionTokens != nil {
		fmt.Fprintf(w, "Usage: %d prompt tokens, %d completion tokens, %d ms\n",
			*doc.Call.PromptTokens, *doc.Call.CompletionTokens, doc.Call.LatencyMS)
	} else {
		fmt.Fprintf(w, "Usage: provider reported no token counts; %d ms\n", doc.Call.LatencyMS)
	}
	fmt.Fprintln(w, "\n--- AI suggestion (advisory) ---")
	fmt.Fprintln(w, doc.Suggestion.Text)
	fmt.Fprintln(w, "--- end AI suggestion ---")
	fmt.Fprintln(w, "\nThis is model output. It is not a verified answer, grade or exam fact. The recorded attempt and answer key are unchanged.")
	fmt.Fprintf(w, "Advisory call #%d recorded; request sha256 %s.\n", doc.Call.ID, doc.Call.RequestSHA256)
	fmt.Fprintf(w, "Database: %s\n", dbPath)
}

func runTutorHistory(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	fs := newFlagSet("tutor history", opt, wantJSON)
	attemptFlag := fs.Int64("attempt", 0, "only calls for this attempt id")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	svc, err := study.Open(opt.ports(*dbFlag))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	history, err := coach.History(context.Background(), svc, coach.HistoryQuery{AttemptID: *attemptFlag})
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	budget := budgetViewOf(history.Budget)
	summary := tutorHistorySummaryView{DailyCalls: history.Daily, Providers: []modelBudgetProviderView{}}
	if history.Configured {
		limit := history.Config.DailyCalls
		summary.DailyLimit = &limit
	}
	summary.Providers = budget.Providers
	summary.TotalCalls = budget.TotalCalls
	summary.TotalLimit = budget.TotalLimit
	summary.Ceiling = budget.Ceiling
	doc := tutorHistoryDoc{
		SchemaVersion: ProtocolVersion,
		State:         "history",
		Summary:       summary,
		Calls:         make([]tutorCallView, 0, len(history.Calls)),
		Limits:        coach.Limits(),
	}
	for _, call := range history.Calls {
		doc.Summary.Calls++
		if call.Status == "ok" {
			doc.Summary.OK++
		} else {
			doc.Summary.Failed++
		}
		if call.PromptTokens != nil {
			doc.Summary.PromptTokens += *call.PromptTokens
		}
		if call.CompletionTokens != nil {
			doc.Summary.CompletionTokens += *call.CompletionTokens
		}
		doc.Calls = append(doc.Calls, tutorCallViewOf(call, false))
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeTutorHistory(opt.Stdout, doc, svc.Path())
	return 0
}

func writeTutorHistory(w io.Writer, doc tutorHistoryDoc, dbPath string) {
	fmt.Fprintln(w, "Pass Quest — tutor call history")
	fmt.Fprintf(w, "Calls: %d (ok %d, failed %d); tokens: %d prompt, %d completion\n",
		doc.Summary.Calls, doc.Summary.OK, doc.Summary.Failed,
		doc.Summary.PromptTokens, doc.Summary.CompletionTokens)
	if doc.Summary.DailyLimit != nil {
		fmt.Fprintf(w, "Active provider budget: %d/%d calls used (UTC)\n", doc.Summary.DailyCalls, *doc.Summary.DailyLimit)
	} else {
		fmt.Fprintf(w, "Daily budget: not configured; %d calls recorded today (UTC)\n", doc.Summary.DailyCalls)
	}
	for _, item := range doc.Summary.Providers {
		marker := ""
		if item.Active {
			marker = " *active"
		}
		fmt.Fprintf(w, "  %-12s %3d / %3d%s\n", item.Provider, item.Calls, item.Limit, marker)
	}
	fmt.Fprintf(w, "  %-12s %3d / %3d  (%s)\n", "TOTAL", doc.Summary.TotalCalls, doc.Summary.TotalLimit, doc.Summary.Ceiling)
	if len(doc.Calls) == 0 {
		fmt.Fprintln(w, "\n(no tutor calls recorded)")
	} else {
		fmt.Fprintln(w, "\nCalls")
		for _, call := range doc.Calls {
			privacy := "first-party"
			switch {
			case call.SourceID != "" && call.SourceTextShared:
				privacy = "source-text-shared"
			case call.SourceID != "":
				privacy = "source-text-withheld"
			}
			providerLabel := providerLabelOrManual(call.Provider)
			if call.FailoverFrom != "" {
				providerLabel += " (failover from " + call.FailoverFrom + ")"
			}
			status := call.Status
			if call.Status == "error" {
				status = "error " + call.ErrorCode
				if call.HTTPStatus != 0 {
					status += fmt.Sprintf(" (HTTP %d)", call.HTTPStatus)
				}
			}
			tokens := "no tokens"
			if call.PromptTokens != nil && call.CompletionTokens != nil {
				tokens = fmt.Sprintf("%d/%d tokens", *call.PromptTokens, *call.CompletionTokens)
			}
			fmt.Fprintf(w, "  #%d  attempt #%d  %s  %s  %s  %s  %s  %d ms  %s  %s\n",
				call.ID, call.AttemptID, call.ContentID+"/"+call.QuestionID, call.Mode,
				status, providerLabel, call.Model, call.LatencyMS, tokens, privacy)
		}
	}
	fmt.Fprintln(w, "\nModel text is shown by `passquest tutor show CALL`, never mixed into `report` or `next`.")
	fmt.Fprintf(w, "Database: %s\n", dbPath)
}

func runTutorShow(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"db": true})
	fs := newFlagSet("tutor show", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 1, "tutor show requires exactly one call id"); code != 0 {
		return code
	}
	callID, err := parsePositiveInt64(fs.Arg(0))
	if err != nil {
		return usageError(opt, *jsonFlag, "invalid_input", fmt.Sprintf("invalid call id %q", fs.Arg(0)))
	}
	svc, err := study.Open(opt.ports(*dbFlag))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	shown, err := coach.Call(context.Background(), svc, callID)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	doc := tutorShowDoc{
		SchemaVersion: ProtocolVersion,
		State:         "advisory",
		Call:          tutorCallViewOf(shown.Call, true),
		Limits:        coach.Limits(),
	}
	if shown.Call.Status == "error" {
		doc.State = "failed"
	} else {
		doc.Suggestion = &tutorSuggestionView{Text: shown.Call.OutputText, Advisory: true, VerifiedAnswer: false}
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeTutorShow(opt.Stdout, doc)
	return 0
}

func writeTutorShow(w io.Writer, doc tutorShowDoc) {
	fmt.Fprintf(w, "Pass Quest — advisory call #%d\n", doc.Call.ID)
	fmt.Fprintf(w, "Attempt #%d: %s / %s\n", doc.Call.AttemptID, doc.Call.ContentID, doc.Call.QuestionID)
	fmt.Fprintf(w, "Mode: %s; model: %s @ %s\n", doc.Call.Mode, doc.Call.Model, doc.Call.ProviderHost)
	if doc.State == "failed" {
		fmt.Fprintf(w, "Stored failure: %s: %s\n", doc.Call.ErrorCode, doc.Call.ErrorMessage)
		fmt.Fprintln(w, "No model text was stored for this call.")
		return
	}
	fmt.Fprintln(w, "\n--- AI suggestion (advisory) ---")
	if doc.Suggestion != nil {
		fmt.Fprintln(w, doc.Suggestion.Text)
	}
	fmt.Fprintln(w, "--- end AI suggestion ---")
	fmt.Fprintln(w, "\nThis stored text is model output, not a verified answer, grade or exam fact.")
}

// runTutorConsentCommand records `tutor consent` and `tutor revoke`. The
// reviewer check is the flag layer's, so it precedes the store open exactly as
// it does today.
func runTutorConsentCommand(args []string, opt Options, command string, granted bool) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"reviewer": true, "provider": true, "db": true})
	fs := newFlagSet("tutor "+command, opt, wantJSON)
	reviewerFlag := fs.String("reviewer", "", "local attribution label for this consent decision")
	providerFlag := fs.String("provider", "", "optional registry provider id this decision applies to")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 1, "tutor "+command+" requires exactly one source id"); code != 0 {
		return code
	}
	reviewer := strings.TrimSpace(*reviewerFlag)
	if reviewer == "" {
		return usageError(opt, *jsonFlag, "invalid_input", "--reviewer is required")
	}
	providerID := strings.TrimSpace(*providerFlag)
	svc, err := study.Open(opt.ports(*dbFlag))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	consented, err := coach.Consent(context.Background(), svc, coach.ConsentChange{
		SourceID: fs.Arg(0),
		Provider: providerID,
		Granted:  granted,
		Reviewer: reviewer,
	})
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	event := consented.Event
	doc := tutorConsentDoc{
		SchemaVersion: ProtocolVersion,
		State:         "consent",
		SourceID:      event.SourceID,
		Provider:      event.Provider,
		Granted:       event.Granted,
		RecordedAt:    event.At.UTC().Format(time.RFC3339Nano),
		Reviewer:      event.Reviewer,
		Limits:        coach.Limits(),
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	action := "withheld"
	if event.Granted {
		action = "authorized"
	}
	scope := "all providers"
	if event.Provider != "" {
		scope = "provider " + event.Provider
	}
	fmt.Fprintf(opt.Stdout, "Tutor consent for %s (%s): %s (%s)\n", consented.SourceID, consented.Title, action, scope)
	fmt.Fprintf(opt.Stdout, "Recorded by %s at %s.\n", event.Reviewer, doc.RecordedAt)
	if event.Granted {
		fmt.Fprintln(opt.Stdout, "`passquest tutor preview` now includes this source's page and question text in the provider payload.")
	} else {
		fmt.Fprintln(opt.Stdout, "This source's page and question text is withheld from future provider payloads.")
	}
	fmt.Fprintln(opt.Stdout, "The event is append-only; earlier decisions remain in the local audit trail.")
	return 0
}

func runTutorConsents(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	fs := newFlagSet("tutor consents", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	svc, err := study.Open(opt.ports(*dbFlag))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	entries, err := coach.Consents(context.Background(), svc)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	doc := tutorConsentsDoc{
		SchemaVersion: ProtocolVersion,
		State:         "consents",
		Sources:       make([]tutorConsentSourceView, 0, len(entries)),
		Limits:        coach.Limits(),
	}
	for _, entry := range entries {
		item := tutorConsentSourceView{
			SourceID: entry.SourceID,
			Provider: entry.Provider,
			Title:    entry.Title,
			Granted:  entry.Granted,
			Reviewer: entry.Reviewer,
		}
		if entry.RecordedAt != nil {
			item.RecordedAt = entry.RecordedAt.UTC().Format(time.RFC3339Nano)
		}
		doc.Sources = append(doc.Sources, item)
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeTutorConsents(opt.Stdout, doc)
	return 0
}

func writeTutorConsents(w io.Writer, doc tutorConsentsDoc) {
	fmt.Fprintln(w, "Pass Quest — tutor source consent")
	if len(doc.Sources) == 0 {
		fmt.Fprintln(w, "No imported sources are stored; only first-party material is available to the tutor.")
		fmt.Fprintln(w, "Imported source text is always withheld until `passquest tutor consent SOURCE --reviewer NAME`.")
		return
	}
	fmt.Fprintln(w, "First-party material is always available. Imported source text is withheld unless authorized.")
	for _, item := range doc.Sources {
		state := "withheld"
		if item.Granted {
			state = "authorized"
		}
		last := ""
		if item.Reviewer != "" {
			last = fmt.Sprintf("  last: %s at %s", item.Reviewer, item.RecordedAt)
		}
		scope := "all providers"
		if item.Provider != "" {
			scope = item.Provider
		}
		fmt.Fprintf(w, "  %s  %s  %s (%s)%s\n", item.SourceID, item.Title, state, scope, last)
	}
}

// tutorDayStart stays for its one remaining caller, note_refine_test.go: the
// note refinement path itself now uses the kernel's UTC-day start.
func tutorDayStart(now time.Time) time.Time {
	year, month, day := now.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func parsePositiveInt64(raw string) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return 0, errors.New("not a positive integer")
	}
	return value, nil
}
