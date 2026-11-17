package coach

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
	"github.com/wxxb789/passquest/internal/tutor"
)

// errNotRecorded and errFailoverNotRecorded are the internal failures of a call
// whose outcome could not be stored; both keep their CLI message.
var (
	errNotRecorded         = errors.New("the advisory call could not be recorded")
	errFailoverNotRecorded = errors.New("the failover call could not be recorded")
)

// Outcome is one advisory call: the resolved request, the recorded row whose
// result is returned, the model text and the provider failure. Text is
// non-empty iff Failure is nil; Failover carries the bounded retry decision.
type Outcome struct {
	// Prepared carries Attempt, Mode, Config and Preview only: Ask does not
	// re-read the text.chat evidence or the credential state for it.
	Prepared Prepared
	Call     store.TutorCall
	Text     string
	Failure  *provider.Failure
	Failover *Failover
}

// Ask sends one advisory request: today's `tutor ask`, in today's order —
// configuration, active pair, text.chat evidence, credential, the budget slot,
// attempt, target, request, one send and at most one consent-equal failover. A
// provider failure is an outcome, not an error.
func Ask(ctx context.Context, s *study.Service, r Request) (Outcome, error) {
	_ = ctx
	cfg, err := kernel.Configured(s, notConfigured)
	if err != nil {
		return Outcome{}, err
	}
	cfg, registry, hasProvider, err := kernel.ResolveActive(s, cfg)
	if err != nil {
		return Outcome{}, err
	}
	providerID := strings.TrimSpace(cfg.Provider)
	env := tutor.CredentialEnv
	key := strings.TrimSpace(kernel.P(s).Getenv(env))
	if hasProvider {
		if err := kernel.RequireVerifiedTextChat(s, registry, providerID, cfg.Model); err != nil {
			return Outcome{}, err
		}
		env, key, _ = kernel.Credential(s, providerID)
		if key == "" {
			return Outcome{}, kernel.Refuse("no_credential",
				"%s is not set; export it in this environment (it is never stored)", env)
		}
	} else if key == "" {
		return Outcome{}, kernel.Refuse("missing_credential",
			"%s is not set; export it in this environment (it is never stored)", tutor.CredentialEnv)
	}
	now := kernel.P(s).Now()
	lease, used, err := kernel.Reserve(s, (*store.Store).ReserveTutorCall, providerID, now, cfg.DailyCalls)
	if err != nil {
		return Outcome{}, err
	}
	if lease.ID() == 0 {
		return Outcome{}, kernel.Refuse("budget_exhausted",
			"%s has used %d of %d provider calls today (UTC); raise the budget with passquest tutor config --daily-calls N",
			budgetLabel(providerID), used, cfg.DailyCalls)
	}
	// The slot stays claimed until the call record replaces it. A path that
	// fails before the host is invoked releases it; a crash or storage failure
	// after that leaves it spent for the UTC day, which can under-spend but
	// never over-spend the budget.
	defer lease.Close()
	record, err := attemptRecord(s, r.AttemptID)
	if err != nil {
		return Outcome{}, err
	}
	target, err := loadTarget(s, record, providerID)
	if err != nil {
		return Outcome{}, err
	}
	preview, err := tutor.Build(cfg, r.Mode, target)
	if err != nil {
		return Outcome{}, kernel.Refuse("invalid_input", "%s", err.Error())
	}
	preview.Request.CredentialEnv = env
	call := callRecord(record, preview, cfg, r.Mode, providerID, cfg.Model, "", now)
	lease.Sending()
	saved, text, failure, err := sendCall(s, call, preview, key, lease)
	if err != nil {
		return Outcome{}, errNotRecorded
	}
	var failover *Failover
	if failure != nil && providerID != "" && provider.Retryable(*failure) {
		saved, text, failure, failover, err = attemptFailover(s, registry, cfg, record, r.Mode, target, saved, *failure)
		if err != nil {
			return Outcome{}, errFailoverNotRecorded
		}
	}
	return Outcome{
		Prepared: Prepared{Attempt: record, Mode: r.Mode, Config: cfg, Preview: preview},
		Call:     saved,
		Text:     text,
		Failure:  failure,
		Failover: failover,
	}, nil
}

// budgetLabel names the provider a budget refusal is about; the
// hand-configured legacy endpoint has its own label.
func budgetLabel(providerID string) string {
	if strings.TrimSpace(providerID) == "" {
		return "the manual provider"
	}
	return providerID
}

// callRecord builds one ledger row from the resolved target and preview so the
// first call and the failover call cannot drift apart when a field is added.
func callRecord(record attempt.Attempt, preview tutor.Preview, cfg tutor.Config, mode tutor.Mode,
	providerID, modelID, failoverFrom string, at time.Time) store.TutorCall {
	return store.TutorCall{
		CreatedAt:     at,
		AttemptID:     record.ID,
		Mode:          string(mode),
		Provider:      providerID,
		ProviderHost:  cfg.ProviderHost(),
		FailoverFrom:  failoverFrom,
		Model:         modelID,
		PromptVersion: preview.Context.PromptVersion,
		SourceID:      preview.Context.SourceID,
		ContentID:     record.ContentID,
		QuestionID:    record.QuestionID,
		RequestSHA256: preview.RequestSHA,
		RequestBytes:  preview.RequestBytes,
	}
}

// sendCall sends one prepared advisory request and records its outcome. It
// returns a typed failure on a provider failure, never an untyped message.
func sendCall(s *study.Service, call store.TutorCall, preview tutor.Preview, key string, lease *kernel.Lease) (store.TutorCall, string, *provider.Failure, error) {
	callStarted := kernel.P(s).Now()
	response, transportErr := kernel.P(s).Tutor.Call(context.Background(), preview.Request)
	if transportErr != nil {
		call.Status = "error"
		call.ErrorCode = "agent_unavailable"
		call.ErrorMessage = kernel.Redact(transportErr.Error(), key)
		call.LatencyMS = kernel.P(s).Now().Sub(callStarted).Milliseconds()
		saved, err := kernel.Store(s).FinalizeTutorCall(call, lease.ID())
		if err != nil {
			return store.TutorCall{}, "", nil, errNotRecorded
		}
		failure := provider.ClassifyTransport(transportErr)
		return saved, "", &failure, nil
	}
	// Private text is shared only when the host actually posted the request. A
	// host that failed before its fetch left the prompt on this machine.
	call.SourceTextShared = response.RequestSent && preview.Context.SourceTextShared
	if response.Status == "error" {
		code, message := kernel.HostError(response)
		call.Status = "error"
		call.ErrorCode = code
		call.ErrorMessage = kernel.Redact(message, key)
		call.HTTPStatus = response.HTTPStatus
		call.LatencyMS = response.LatencyMS
		saved, err := kernel.Store(s).FinalizeTutorCall(call, lease.ID())
		if err != nil {
			return store.TutorCall{}, "", nil, errNotRecorded
		}
		failure := provider.Classify(code, response.HTTPStatus, message)
		return saved, "", &failure, nil
	}
	text := strings.TrimSpace(response.Text)
	if !kernel.ValidText(text) {
		call.Status = "error"
		call.ErrorCode = "invalid_response"
		call.ErrorMessage = "the provider text was empty, invalid or larger than the local bound"
		call.HTTPStatus = response.HTTPStatus
		call.LatencyMS = response.LatencyMS
		saved, err := kernel.Store(s).FinalizeTutorCall(call, lease.ID())
		if err != nil {
			return store.TutorCall{}, "", nil, errNotRecorded
		}
		failure := provider.Classify(provider.FailureInvalidResponse, response.HTTPStatus, call.ErrorMessage)
		return saved, "", &failure, nil
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
	saved, err := kernel.Store(s).FinalizeTutorCall(call, lease.ID())
	if err != nil {
		return store.TutorCall{}, "", nil, errNotRecorded
	}
	return saved, text, nil, nil
}
