package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/skill"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
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

func runTutorConfig(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{
		"endpoint": true, "model": true, "timeout-seconds": true,
		"max-output-tokens": true, "daily-calls": true, "db": true,
	})
	fs := newFlagSet("tutor config", opt, wantJSON)
	endpointFlag := fs.String("endpoint", "", "OpenAI-compatible chat-completions endpoint URL")
	modelFlag := fs.String("model", "", "provider model name")
	timeoutFlag := fs.Int("timeout-seconds", 0, "provider timeout in seconds (1-120)")
	maxTokensFlag := fs.Int("max-output-tokens", 0, "provider max_tokens (64-2048)")
	dailyFlag := fs.Int("daily-calls", 0, "daily call budget (1-200)")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}

	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()

	cfg, configured, err := st.TutorConfig()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	if !configured {
		cfg = tutor.DefaultConfig()
	}
	changed := false
	manual := false
	invalid := ""
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "endpoint":
			changed = true
			manual = true
			cfg.Endpoint = *endpointFlag
		case "model":
			changed = true
			manual = true
			cfg.Model = *modelFlag
		case "timeout-seconds":
			changed = true
			if *timeoutFlag < tutor.MinTimeoutMS/1000 || *timeoutFlag > tutor.MaxTimeoutMS/1000 {
				invalid = fmt.Sprintf("--timeout-seconds %d is outside %d..%d",
					*timeoutFlag, tutor.MinTimeoutMS/1000, tutor.MaxTimeoutMS/1000)
				return
			}
			cfg.TimeoutMS = int64(*timeoutFlag) * 1000
		case "max-output-tokens":
			changed = true
			cfg.MaxOutputTokens = *maxTokensFlag
		case "daily-calls":
			changed = true
			cfg.DailyCalls = *dailyFlag
		}
	})
	if invalid != "" {
		return usageError(opt, *jsonFlag, "invalid_config", invalid)
	}
	if manual {
		// A hand-edited endpoint and model are the goal-0005 legacy
		// configuration; the registry selection no longer describes them.
		cfg.Provider = ""
	}
	if changed {
		if err := cfg.Validate(); err != nil {
			return usageError(opt, *jsonFlag, "invalid_config", err.Error())
		}
		if err := st.SaveTutorConfig(cfg, opt.Now()); err != nil {
			return internalError(opt, *jsonFlag, err)
		}
		configured = true
	}

	env, _, present := provider.CredentialValue(opt.Getenv, cfg.Provider)
	doc := tutorConfigDoc{
		SchemaVersion: ProtocolVersion,
		State:         "config",
		Configured:    configured,
		Config:        tutorConfigViewOf(cfg),
		Credential:    tutorCredentialView{Env: env, Present: present},
		Limits:        tutorLimits,
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeTutorConfig(opt.Stdout, doc)
	return 0
}

func writeTutorConfig(w io.Writer, doc tutorConfigDoc) {
	fmt.Fprintln(w, "Pass Quest — tutor configuration")
	if !doc.Configured {
		fmt.Fprintln(w, "Provider: not configured")
		fmt.Fprintln(w, "Set one OpenAI-compatible endpoint and model:")
		fmt.Fprintln(w, `  passquest tutor config --endpoint https://api.example.com/v1/chat/completions --model MODEL`)
	} else {
		if doc.Config.Provider != "" {
			fmt.Fprintf(w, "Provider: %s\n", doc.Config.Provider)
		}
		fmt.Fprintf(w, "Provider endpoint: %s\n", doc.Config.Endpoint)
		fmt.Fprintf(w, "Model: %s\n", doc.Config.Model)
		fmt.Fprintf(w, "Timeout: %d ms\n", doc.Config.TimeoutMS)
		fmt.Fprintf(w, "Max output tokens: %d\n", doc.Config.MaxOutputTokens)
		fmt.Fprintf(w, "Daily call budget: %d (UTC day, failed calls included)\n", doc.Config.DailyCalls)
	}
	if doc.Credential.Present {
		fmt.Fprintf(w, "Credential: %s is set (value never shown or stored)\n", doc.Credential.Env)
	} else {
		fmt.Fprintf(w, "Credential: %s is missing (export it in this environment before tutor ask)\n", doc.Credential.Env)
	}
	fmt.Fprintln(w, "\nThe agent host is the only component that contacts the provider; ordinary practice works without it.")
	fmt.Fprintln(w, "Prefer a registered provider: passquest model list, then passquest model use <provider>/<model>.")
}

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
	mode, code := tutorMode(*modeFlag, opt, *jsonFlag)
	if code != 0 {
		return code
	}
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	cfg, configured, err := st.TutorConfig()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	if !configured {
		return usageError(opt, *jsonFlag, "not_configured",
			"no tutor provider is configured; run passquest tutor config --endpoint URL --model NAME")
	}
	record, err := tutorAttemptRecord(st, *attemptFlag)
	if err != nil {
		return tutorCommandError(opt, *jsonFlag, err)
	}
	registry, _, err := provider.Load(opt.Getenv)
	if err != nil {
		return usageError(opt, *jsonFlag, "invalid_registry", err.Error())
	}
	providerID := strings.TrimSpace(cfg.Provider)
	if providerID != "" {
		active, model, _, err := activeProvider(cfg, registry)
		if err != nil {
			return usageError(opt, *jsonFlag, "unknown_model", err.Error())
		}
		if !model.Has(provider.CapTextChat) {
			return usageError(opt, *jsonFlag, "capability_not_declared",
				fmt.Sprintf("%s/%s does not declare text.chat", providerID, cfg.Model))
		}
		if !active.Custom {
			cfg.Endpoint = active.Endpoint
		}
	}
	target, err := loadTutorTargetForProvider(st, record, providerID)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	preview, err := tutor.Build(cfg, mode, target)
	if err != nil {
		return usageError(opt, *jsonFlag, "invalid_input", err.Error())
	}
	doc := tutorPreviewDoc{
		SchemaVersion: ProtocolVersion,
		State:         "preview",
		Attempt:       tutorAttemptViewOf(record),
		Mode:          string(mode),
		Provider:      tutorProviderViewOf(cfg, opt),
		Context:       tutorContextViewOf(preview.Context),
		Chat:          preview.Request.Chat,
		RequestSHA256: preview.RequestSHA,
		RequestBytes:  preview.RequestBytes,
		Limits:        tutorLimits,
	}
	if providerID != "" {
		verification, found, err := st.LatestProviderVerification(providerID, cfg.Model, string(provider.CapTextChat))
		if err != nil {
			return internalError(opt, *jsonFlag, err)
		}
		switch {
		case found && verification.Status == "verified":
			doc.Provider.TextChatState = "verified@" + verification.CheckedAt.UTC().Format(time.RFC3339)
		case found && verification.Status == "failed":
			doc.Provider.TextChatState = "failed:" + verification.FailureType
			doc.Provider.TextChatAction = fmt.Sprintf("passquest model test %s/%s", providerID, cfg.Model)
		default:
			doc.Provider.TextChatState = "declared,unverified"
			doc.Provider.TextChatAction = fmt.Sprintf("passquest model test %s/%s", providerID, cfg.Model)
		}
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeTutorPreview(opt.Stdout, doc)
	return 0
}

func tutorMode(raw string, opt Options, jsonMode bool) (tutor.Mode, int) {
	mode := tutor.Mode(strings.TrimSpace(raw))
	if !mode.Valid() {
		return "", usageError(opt, jsonMode, "invalid_mode",
			fmt.Sprintf("mode %q is not supported; use explain, probe or micro_lesson", raw))
	}
	return mode, 0
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
	mode, code := tutorMode(*modeFlag, opt, *jsonFlag)
	if code != 0 {
		return code
	}
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()

	cfg, configured, err := st.TutorConfig()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	if !configured {
		return usageError(opt, *jsonFlag, "not_configured",
			"no tutor provider is configured; run passquest tutor config --endpoint URL --model NAME")
	}
	registry, _, err := provider.Load(opt.Getenv)
	if err != nil {
		return usageError(opt, *jsonFlag, "invalid_registry", err.Error())
	}
	providerID := strings.TrimSpace(cfg.Provider)
	env := tutor.CredentialEnv
	key := strings.TrimSpace(opt.Getenv(env))
	if providerID != "" {
		active, model, _, err := activeProvider(cfg, registry)
		if err != nil {
			return usageError(opt, *jsonFlag, "unknown_model", err.Error())
		}
		if !model.Has(provider.CapTextChat) {
			return usageError(opt, *jsonFlag, "capability_not_declared",
				fmt.Sprintf("%s/%s does not declare text.chat", providerID, cfg.Model))
		}
		if !active.Custom {
			cfg.Endpoint = active.Endpoint
		}
		verification, found, err := st.LatestProviderVerification(providerID, cfg.Model, string(provider.CapTextChat))
		if err != nil {
			return internalError(opt, *jsonFlag, err)
		}
		if !found || verification.Status != "verified" {
			detail := "text.chat is declared but unverified"
			if found && verification.Status == "failed" {
				detail = "the last text.chat test failed: " + verification.FailureType
			}
			return usageError(opt, *jsonFlag, "capability_unverified",
				fmt.Sprintf("%s/%s: %s; run passquest model test %s/%s before asking",
					providerID, cfg.Model, detail, providerID, cfg.Model))
		}
		env, key, _ = provider.CredentialValue(opt.Getenv, providerID)
		if key == "" {
			return usageError(opt, *jsonFlag, "no_credential",
				fmt.Sprintf("%s is not set; export it in this environment (it is never stored)", env))
		}
	} else if key == "" {
		return usageError(opt, *jsonFlag, "missing_credential",
			fmt.Sprintf("%s is not set; export it in this environment (it is never stored)", tutor.CredentialEnv))
	}
	now := opt.Now()
	reservation, used, err := st.ReserveTutorCall(providerID, tutorDayStart(now), now, cfg.DailyCalls)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	if reservation == 0 {
		return usageError(opt, *jsonFlag, "budget_exhausted",
			fmt.Sprintf("%s has used %d of %d provider calls today (UTC); raise the budget with passquest tutor config --daily-calls N",
				budgetLabel(providerID), used, cfg.DailyCalls))
	}
	// The slot stays claimed until the call record replaces it. A path that
	// fails before the host is invoked releases it; a crash or storage failure
	// after that leaves it spent for the UTC day, which can under-spend but
	// never over-spend the budget.
	sendStarted := false
	defer func() {
		if !sendStarted {
			_ = st.ReleaseTutorCallReservation(reservation)
		}
	}()
	record, err := tutorAttemptRecord(st, *attemptFlag)
	if err != nil {
		return tutorCommandError(opt, *jsonFlag, err)
	}
	target, err := loadTutorTargetForProvider(st, record, providerID)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	preview, err := tutor.Build(cfg, mode, target)
	if err != nil {
		return usageError(opt, *jsonFlag, "invalid_input", err.Error())
	}
	preview.Request.CredentialEnv = env

	call := store.TutorCall{
		CreatedAt:     now,
		AttemptID:     record.ID,
		Mode:          string(mode),
		Provider:      providerID,
		ProviderHost:  cfg.ProviderHost(),
		Model:         cfg.Model,
		PromptVersion: preview.Context.PromptVersion,
		SourceID:      preview.Context.SourceID,
		ContentID:     record.ContentID,
		QuestionID:    record.QuestionID,
		RequestSHA256: preview.RequestSHA,
		RequestBytes:  preview.RequestBytes,
	}
	sendStarted = true
	saved, text, failure, ok, code := sendTutorCall(opt, st, call, preview, key, reservation)
	if code != 0 {
		return internalError(opt, *jsonFlag, errors.New("the advisory call could not be recorded"))
	}
	var failover *tutorFailoverView
	if !ok && providerID != "" && provider.Retryable(failure) {
		var failoverCode int
		saved, text, failure, ok, failover, failoverCode = attemptFailover(opt, st, registry, cfg, record, mode, target, saved, failure)
		if failoverCode != 0 {
			return internalError(opt, *jsonFlag, errors.New("the failover call could not be recorded"))
		}
	}
	if !*jsonFlag && failover != nil {
		writeFailoverLine(opt.Stdout, failover)
	}
	if ok {
		doc := tutorAdvisoryDoc{
			SchemaVersion: ProtocolVersion,
			State:         "advisory",
			Call:          tutorCallViewOf(saved, false),
			Suggestion:    tutorSuggestionView{Text: text, Advisory: true, VerifiedAnswer: false},
			Context:       tutorContextViewOf(preview.Context),
			Failover:      failover,
			Limits:        tutorLimits,
		}
		if *jsonFlag {
			return writeJSON(opt, doc)
		}
		writeTutorAdvisory(opt.Stdout, doc, record, st.Path())
		return 0
	}
	doc := tutorFailureDoc{
		SchemaVersion: ProtocolVersion,
		State:         "failed",
		Error:         errorBody{Code: saved.ErrorCode, Message: saved.ErrorMessage},
		Action:        failure.Action,
		Call:          tutorCallViewOf(saved, false),
		Failover:      failover,
		Limits:        tutorLimits,
	}
	if *jsonFlag {
		_ = writeJSONTo(opt.Stderr, doc)
	} else {
		fmt.Fprintf(opt.Stderr, "passquest: %s: %s\n", saved.ErrorCode, saved.ErrorMessage)
		if failure.Action != "" {
			fmt.Fprintf(opt.Stderr, "Next: %s\n", failure.Action)
		}
		fmt.Fprintf(opt.Stderr, "Advisory call #%d recorded as a failure; no attempt or answer key was changed.\n", saved.ID)
		fmt.Fprintf(opt.Stderr, "See `passquest tutor history` for usage and `passquest practice` to keep practicing locally.\n")
	}
	return 3
}

func budgetLabel(providerID string) string {
	if strings.TrimSpace(providerID) == "" {
		return "the manual provider"
	}
	return providerID
}

// sendTutorCall sends one prepared advisory request and records its outcome.
// It returns a typed failure on a provider failure, never an untyped message.
func sendTutorCall(opt Options, st *store.Store, call store.TutorCall, preview tutor.Preview, key string, reservation int64) (store.TutorCall, string, provider.Failure, bool, int) {
	callStarted := opt.Now()
	response, transportErr := opt.TutorTransport.Call(context.Background(), preview.Request)
	if transportErr != nil {
		call.Status = "error"
		call.ErrorCode = "agent_unavailable"
		call.ErrorMessage = tutorMessage(transportErr.Error(), key)
		call.LatencyMS = opt.Now().Sub(callStarted).Milliseconds()
		saved, err := st.FinalizeTutorCall(call, reservation)
		if err != nil {
			return store.TutorCall{}, "", provider.Failure{}, false, 1
		}
		return saved, "", provider.ClassifyTransport(transportErr), false, 0
	}
	// Private text is shared only when the host actually posted the request. A
	// host that failed before its fetch left the prompt on this machine.
	call.SourceTextShared = response.RequestSent && preview.Context.SourceTextShared
	if response.Status == "error" {
		code := "host_error"
		message := "the tutor host returned no error detail"
		if response.Error != nil {
			if tutor.ValidHostErrorCode(response.Error.Code) {
				code = response.Error.Code
			}
			if strings.TrimSpace(response.Error.Message) != "" {
				message = response.Error.Message
			}
		}
		call.Status = "error"
		call.ErrorCode = code
		call.ErrorMessage = tutorMessage(message, key)
		call.HTTPStatus = response.HTTPStatus
		call.LatencyMS = response.LatencyMS
		saved, err := st.FinalizeTutorCall(call, reservation)
		if err != nil {
			return store.TutorCall{}, "", provider.Failure{}, false, 1
		}
		return saved, "", provider.Classify(code, response.HTTPStatus, message), false, 0
	}
	text := strings.TrimSpace(response.Text)
	if !validTutorText(text) {
		call.Status = "error"
		call.ErrorCode = "invalid_response"
		call.ErrorMessage = "the provider text was empty, invalid or larger than the local bound"
		call.HTTPStatus = response.HTTPStatus
		call.LatencyMS = response.LatencyMS
		saved, err := st.FinalizeTutorCall(call, reservation)
		if err != nil {
			return store.TutorCall{}, "", provider.Failure{}, false, 1
		}
		return saved, "", provider.Classify(provider.FailureInvalidResponse, response.HTTPStatus, call.ErrorMessage), false, 0
	}
	text = strings.ReplaceAll(text, key, "[redacted]")
	call.Status = "ok"
	call.OutputText = text
	call.OutputChars = utf8.RuneCountInString(text)
	call.HTTPStatus = response.HTTPStatus
	call.LatencyMS = response.LatencyMS
	if response.Usage != nil {
		prompt := response.Usage.PromptTokens
		completion := response.Usage.CompletionTokens
		call.PromptTokens = &prompt
		call.CompletionTokens = &completion
	}
	saved, err := st.FinalizeTutorCall(call, reservation)
	if err != nil {
		return store.TutorCall{}, "", provider.Failure{}, false, 1
	}
	return saved, text, provider.Failure{}, true, 0
}

// attemptFailover makes at most one retry on another registry provider. Every
// precondition is checked before a target is contacted; a refusal appends the
// reason and never widens the consent scope.
func attemptFailover(opt Options, st *store.Store, registry provider.Registry, cfg tutor.Config, record attempt.Attempt, mode tutor.Mode, sourceTarget tutor.Target, sourceCall store.TutorCall, sourceFailure provider.Failure) (store.TutorCall, string, provider.Failure, bool, *tutorFailoverView, int) {
	sourceScope := consentScopeKey(sourceTarget)
	firstTarget := ""
	firstReason := ""
	firstDetail := ""
	keepReason := func(targetID, reason, detail string) {
		if firstReason == "" {
			firstTarget = targetID
			firstReason = reason
			firstDetail = detail
		}
	}
	for _, p := range registry.Providers {
		if p.Custom || p.ID == cfg.Provider {
			continue
		}
		model, ok := p.DefaultModel()
		if !ok || !model.Has(provider.CapTextChat) {
			continue
		}
		verification, found, err := st.LatestProviderVerification(p.ID, model.ID, string(provider.CapTextChat))
		if err != nil {
			return store.TutorCall{}, "", provider.Failure{}, false, nil, 1
		}
		if !found || verification.Status != "verified" {
			keepReason(p.ID, "target_unverified", "model "+model.ID+" is not verified for text.chat")
			continue
		}
		env, key, present := provider.CredentialValue(opt.Getenv, p.ID)
		if !present {
			keepReason(p.ID, "no_credential", env+" is not set")
			continue
		}
		target, err := loadTutorTargetForProvider(st, record, p.ID)
		if err != nil {
			return store.TutorCall{}, "", provider.Failure{}, false, nil, 1
		}
		if consentScopeKey(target) != sourceScope {
			detail := "consent scope differs"
			if target.PrivateSource && target.SourceID != "" {
				detail = fmt.Sprintf("source %s is %s for %s but %s for %s",
					target.SourceID, grantWord(sourceTarget.ConsentGranted), cfg.Provider,
					grantWord(target.ConsentGranted), p.ID)
			}
			keepReason(p.ID, "consent_scope_differs", detail)
			continue
		}
		targetCfg := cfg
		targetCfg.Provider = p.ID
		targetCfg.Model = model.ID
		targetCfg.Endpoint = p.Endpoint
		preview, err := tutor.Build(targetCfg, mode, target)
		if err != nil {
			keepReason(p.ID, "target_invalid", "the target configuration is not usable")
			continue
		}
		preview.Request.CredentialEnv = env
		reserveAt := opt.Now()
		reservation, _, err := st.ReserveTutorCall(p.ID, tutorDayStart(reserveAt), reserveAt, targetCfg.DailyCalls)
		if err != nil {
			return store.TutorCall{}, "", provider.Failure{}, false, nil, 1
		}
		if reservation == 0 {
			keepReason(p.ID, "budget_exhausted", "the target has no provider budget left today")
			continue
		}
		targetCall := store.TutorCall{
			CreatedAt:     opt.Now(),
			AttemptID:     record.ID,
			Mode:          string(mode),
			Provider:      p.ID,
			ProviderHost:  targetCfg.ProviderHost(),
			FailoverFrom:  cfg.Provider,
			Model:         model.ID,
			PromptVersion: preview.Context.PromptVersion,
			SourceID:      preview.Context.SourceID,
			ContentID:     record.ContentID,
			QuestionID:    record.QuestionID,
			RequestSHA256: preview.RequestSHA,
			RequestBytes:  preview.RequestBytes,
		}
		saved, text, failure, ok, code := sendTutorCall(opt, st, targetCall, preview, key, reservation)
		if code != 0 {
			return store.TutorCall{}, "", provider.Failure{}, false, nil, 1
		}
		event := store.ProviderFailover{
			At:           opt.Now(),
			FromProvider: cfg.Provider,
			FromModel:    cfg.Model,
			ToProvider:   p.ID,
			ToModel:      model.ID,
			Reason:       sourceFailure.Type,
			Applied:      true,
			FromCallID:   sourceCall.ID,
			ToCallID:     saved.ID,
		}
		if _, err := st.AppendProviderFailover(event); err != nil {
			return store.TutorCall{}, "", provider.Failure{}, false, nil, 1
		}
		view := &tutorFailoverView{
			FromProvider: cfg.Provider,
			FromModel:    cfg.Model,
			ToProvider:   p.ID,
			ToModel:      model.ID,
			Reason:       sourceFailure.Type,
			Applied:      true,
		}
		return saved, text, failure, ok, view, 0
	}
	view := &tutorFailoverView{
		FromProvider: cfg.Provider,
		FromModel:    cfg.Model,
		ToProvider:   firstTarget,
		Reason:       sourceFailure.Type,
		Refusal:      firstReason,
		Detail:       firstDetail,
	}
	if view.Refusal == "" {
		view.Refusal = "no_eligible_provider"
		view.Detail = "no other registry provider declares a usable text model"
	}
	event := store.ProviderFailover{
		At:           opt.Now(),
		FromProvider: cfg.Provider,
		FromModel:    cfg.Model,
		ToProvider:   firstTarget,
		Reason:       sourceFailure.Type,
		Refusal:      view.Refusal,
		FromCallID:   sourceCall.ID,
	}
	if _, err := st.AppendProviderFailover(event); err != nil {
		return store.TutorCall{}, "", provider.Failure{}, false, nil, 1
	}
	return sourceCall, "", sourceFailure, false, view, 0
}

func consentScopeKey(target tutor.Target) string {
	if !target.PrivateSource || target.SourceID == "" {
		return "first-party"
	}
	return fmt.Sprintf("%s=%t", target.SourceID, target.ConsentGranted)
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

func grantWord(granted bool) string {
	if granted {
		return "authorized"
	}
	return "withheld"
}

func tutorMessage(value, key string) string {
	value = strings.ReplaceAll(value, key, "[redacted]")
	value = strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t':
			return r
		}
		if (r < 0x20) || (r >= 0x7f && r <= 0x9f) {
			return ' '
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	if value == "" {
		return "no detail"
	}
	runes := []rune(value)
	if len(runes) > 500 {
		value = string(runes[:500]) + "…"
	}
	return value
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
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	if *attemptFlag < 0 {
		return usageError(opt, *jsonFlag, "invalid_input", "--attempt must be positive or 0 for all")
	}
	if *attemptFlag > 0 {
		if _, err := tutorAttemptRecord(st, *attemptFlag); err != nil {
			return tutorCommandError(opt, *jsonFlag, err)
		}
	}
	calls, err := st.TutorCalls(*attemptFlag)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	cfg, configured, err := st.TutorConfig()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	now := opt.Now()
	daily, err := st.TutorCallCountSince(cfg.Provider, tutorDayStart(now))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	summary := tutorHistorySummaryView{DailyCalls: daily, Providers: []modelBudgetProviderView{}}
	if configured {
		limit := cfg.DailyCalls
		summary.DailyLimit = &limit
	}
	registry, _, err := provider.Load(opt.Getenv)
	if err != nil {
		return usageError(opt, *jsonFlag, "invalid_registry", err.Error())
	}
	budget, err := buildBudgetView(st, cfg, registry, strings.TrimSpace(cfg.Provider), opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	summary.Providers = budget.Providers
	summary.TotalCalls = budget.TotalCalls
	summary.TotalLimit = budget.TotalLimit
	summary.Ceiling = budget.Ceiling
	doc := tutorHistoryDoc{
		SchemaVersion: ProtocolVersion,
		State:         "history",
		Summary:       summary,
		Calls:         make([]tutorCallView, 0, len(calls)),
		Limits:        tutorLimits,
	}
	for _, call := range calls {
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
	writeTutorHistory(opt.Stdout, doc, st.Path())
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
			providerLabel := call.Provider
			if providerLabel == "" {
				providerLabel = "manual"
			}
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
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	call, err := st.TutorCall(callID)
	if errors.Is(err, store.ErrNotFound) {
		return usageError(opt, *jsonFlag, "unknown_call", fmt.Sprintf("tutor call #%d is not recorded", callID))
	}
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	doc := tutorShowDoc{
		SchemaVersion: ProtocolVersion,
		State:         "advisory",
		Call:          tutorCallViewOf(call, true),
		Limits:        tutorLimits,
	}
	if call.Status == "error" {
		doc.State = "failed"
	} else {
		doc.Suggestion = &tutorSuggestionView{Text: call.OutputText, Advisory: true, VerifiedAnswer: false}
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
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	src, err := st.Source(fs.Arg(0))
	if errors.Is(err, store.ErrNotFound) {
		return usageError(opt, *jsonFlag, "unknown_source", fmt.Sprintf("source %q is not imported", fs.Arg(0)))
	}
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	event, err := st.AppendTutorConsent(store.TutorConsentEvent{
		SourceID: src.ID,
		Provider: providerID,
		Granted:  granted,
		Reviewer: reviewer,
		At:       opt.Now(),
	})
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	doc := tutorConsentDoc{
		SchemaVersion: ProtocolVersion,
		State:         "consent",
		SourceID:      event.SourceID,
		Provider:      event.Provider,
		Granted:       event.Granted,
		RecordedAt:    event.At.UTC().Format(time.RFC3339Nano),
		Reviewer:      event.Reviewer,
		Limits:        tutorLimits,
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
	fmt.Fprintf(opt.Stdout, "Tutor consent for %s (%s): %s (%s)\n", src.ID, src.Title, action, scope)
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
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	sources, err := st.Sources()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	events, err := st.TutorConsents()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	bySource := make(map[string][]store.TutorConsentEvent)
	for _, event := range events {
		bySource[event.SourceID] = append(bySource[event.SourceID], event)
	}
	doc := tutorConsentsDoc{
		SchemaVersion: ProtocolVersion,
		State:         "consents",
		Sources:       make([]tutorConsentSourceView, 0, len(sources)),
		Limits:        tutorLimits,
	}
	for _, summary := range sources {
		matched := bySource[summary.ID]
		if len(matched) == 0 {
			doc.Sources = append(doc.Sources, tutorConsentSourceView{
				SourceID: summary.ID,
				Title:    summary.Title,
			})
			continue
		}
		for _, event := range matched {
			doc.Sources = append(doc.Sources, tutorConsentSourceView{
				SourceID:   summary.ID,
				Provider:   event.Provider,
				Title:      summary.Title,
				Granted:    event.Granted,
				RecordedAt: event.At.UTC().Format(time.RFC3339Nano),
				Reviewer:   event.Reviewer,
			})
		}
	}
	sort.SliceStable(doc.Sources, func(i, j int) bool {
		if doc.Sources[i].SourceID != doc.Sources[j].SourceID {
			return doc.Sources[i].SourceID < doc.Sources[j].SourceID
		}
		return doc.Sources[i].Provider < doc.Sources[j].Provider
	})
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

func tutorAttemptRecord(st *store.Store, id int64) (attempt.Attempt, error) {
	records, err := st.List()
	if err != nil {
		return attempt.Attempt{}, err
	}
	if id < 0 {
		return attempt.Attempt{}, refuse("invalid_input", "--attempt must be positive or 0 for the latest attempt")
	}
	if len(records) == 0 {
		return attempt.Attempt{}, refuse("no_attempts", "no attempt is recorded; run passquest practice first")
	}
	if id == 0 {
		return records[len(records)-1], nil
	}
	for _, record := range records {
		if record.ID == id {
			return record, nil
		}
	}
	return attempt.Attempt{}, refuse("unknown_attempt", "attempt #%d is not recorded", id)
}

type tutorLoadedContent struct {
	Found    bool
	Content  exam.Exam
	Question exam.Question
	Private  bool
	SourceID string
}

// embeddedContents lists every content authored in this build: the sample and
// the skill practice passages, which already include the tactic exercise.
// Returned values are shared and must not be mutated.
func embeddedContents() ([]exam.Exam, error) {
	sample, err := exam.Sample()
	if err != nil {
		return nil, err
	}
	contents, err := skill.PracticeContent()
	if err != nil {
		return nil, err
	}
	return append([]exam.Exam{sample}, contents...), nil
}

// embeddedContent resolves an authored content id. found is false for a parsed
// source page id, which the caller resolves from the store.
func embeddedContent(id string) (exam.Exam, bool, error) {
	contents, err := embeddedContents()
	if err != nil {
		return exam.Exam{}, false, err
	}
	for _, content := range contents {
		if content.ID == id {
			return content, true, nil
		}
	}
	return exam.Exam{}, false, nil
}

// loadTutorContent resolves the stored attempt's question from embedded
// material or a reviewed page of an imported source. A missing or no-longer-
// practiceable question is a state, not an error: the prompt builder discloses
// that current content is unavailable.
func loadTutorContent(st *store.Store, contentID, questionID string) (tutorLoadedContent, error) {
	if content, found, err := embeddedContent(contentID); err != nil {
		return tutorLoadedContent{}, err
	} else if found {
		return tutorEmbedded(content, questionID), nil
	}

	sourceID, pageNumber, ok := source.ParseContentID(contentID)
	if !ok {
		return tutorLoadedContent{Private: strings.HasPrefix(contentID, "src-")}, nil
	}
	out := tutorLoadedContent{Private: true, SourceID: sourceID}
	src, err := st.Source(sourceID)
	if errors.Is(err, store.ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return tutorLoadedContent{}, err
	}
	page, found := findPage(src, pageNumber)
	if !found {
		return out, nil
	}
	questions, err := st.Questions(sourceID)
	if err != nil {
		return tutorLoadedContent{}, err
	}
	content, _, err := source.BuildPracticeExam(src, page, questions)
	if err != nil {
		return out, nil
	}
	loaded := tutorEmbedded(content, questionID)
	loaded.Private = true
	loaded.SourceID = sourceID
	return loaded, nil
}

func tutorEmbedded(content exam.Exam, questionID string) tutorLoadedContent {
	question, _, ok := content.Question(questionID)
	if !ok {
		return tutorLoadedContent{}
	}
	return tutorLoadedContent{Found: true, Content: content, Question: question}
}

func loadTutorTarget(st *store.Store, record attempt.Attempt) (tutor.Target, error) {
	cfg, _, err := st.TutorConfig()
	if err != nil {
		return tutor.Target{}, err
	}
	return loadTutorTargetForProvider(st, record, cfg.Provider)
}

func loadTutorTargetForProvider(st *store.Store, record attempt.Attempt, providerID string) (tutor.Target, error) {
	loaded, err := loadTutorContent(st, record.ContentID, record.QuestionID)
	if err != nil {
		return tutor.Target{}, err
	}
	skills, lessons, mapped := tutorSkills(record, loaded)
	target := tutor.Target{
		AttemptID:                   record.ID,
		Status:                      string(record.Status),
		SelectedLabel:               record.SelectedLabel,
		SelectedText:                record.SelectedText,
		CorrectLabel:                record.CorrectLabel,
		CorrectText:                 record.CorrectText,
		ElapsedMS:                   record.ElapsedMS,
		AttemptNumber:               record.AttemptNumber,
		Repeat:                      record.Repeat(),
		Assistance:                  string(record.Assistance),
		Skills:                      skills,
		SkillsMappedFromCurrentTags: mapped,
		Lessons:                     lessons,
		PrivateSource:               loaded.Private,
		SourceID:                    loaded.SourceID,
	}
	if loaded.Found {
		target.ContentMatched = true
		target.ContentChanged = loaded.Content.Version != record.ContentVersion
		target.PassageText = loaded.Content.Passage.Text
		target.QuestionStem = loaded.Question.Stem
		target.Evidence = loaded.Question.Evidence
		target.Reasoning = loaded.Question.Reasoning
		target.Options = make([]tutor.Option, 0, len(loaded.Question.Options))
		for _, option := range loaded.Question.Options {
			target.Options = append(target.Options, tutor.Option{Label: option.Label, Text: option.Text})
		}
	}
	if target.PrivateSource && target.SourceID != "" {
		event, ok, err := st.TutorConsent(target.SourceID, providerID)
		if err != nil {
			return tutor.Target{}, err
		}
		target.ConsentGranted = ok && event.Granted
	}
	return target, nil
}

// effectiveSkills resolves the skill ids an attempt is classified under. A
// recorded snapshot is authoritative, even when it is empty: it records the
// tags the question had when the answer was stored. Only a pre-v6 row falls
// back to the question's current tags, and that fallback is marked so callers
// can disclose it. An unregistered id in a recorded snapshot invalidates the
// whole snapshot, so evidence is never silently regrouped under the remaining
// tags.
func effectiveSkills(record attempt.Attempt, current []string, currentKnown bool) (ids []string, fromCurrent bool, unregistered string) {
	raw := record.Skills
	if !record.SkillsRecorded {
		if !currentKnown {
			return nil, false, ""
		}
		raw = current
		fromCurrent = len(current) > 0
	}
	seen := make(map[string]bool)
	for _, id := range raw {
		if !skill.IsRegistered(id) {
			if record.SkillsRecorded {
				return nil, false, id
			}
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, fromCurrent, ""
}

func tutorSkills(record attempt.Attempt, loaded tutorLoadedContent) ([]string, []tutor.SkillLesson, bool) {
	ids, fromCurrent, _ := effectiveSkills(record, loaded.Question.Tactics, loaded.Found)
	var lessons []tutor.SkillLesson
	for _, id := range ids {
		candidate, _ := skill.Get(id)
		lessons = append(lessons, tutor.SkillLesson{
			ID:          candidate.ID,
			Kind:        string(candidate.Kind),
			Title:       candidate.Title,
			Summary:     candidate.Summary,
			Principle:   candidate.Principle,
			WhyItWorks:  candidate.WhyItWorks,
			WhenItWorks: candidate.WhenItWorks,
			Steps:       append([]string(nil), candidate.Steps...),
			WatchOut:    candidate.WatchOut,
		})
	}
	return ids, lessons, fromCurrent && len(ids) > 0
}

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

func tutorCommandError(opt Options, jsonMode bool, err error) int {
	var refusal tacticRefusal
	if errors.As(err, &refusal) {
		return usageError(opt, jsonMode, refusal.code, refusal.message)
	}
	return internalError(opt, jsonMode, err)
}
