package workspace

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/studio"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/coach"
	"github.com/wxxb789/passquest/internal/study/models"
	"github.com/wxxb789/passquest/internal/tutor"
)

// consentReviewer attributes a consent decision made in the window.
const consentReviewer = "desktop"

// CoachPreview builds the exact advisory request `tutor preview` builds for a
// recorded attempt: provider and model, whether the endpoint is external, the
// messages with their hash and size, the source consent state and the budget
// slot. Nothing is sent.
func (w *Workspace) CoachPreview(attemptID int64, mode string) (studio.CoachPreview, error) {
	if err := study.GuardPath(w.ports, "tutor"); err != nil {
		return studio.CoachPreview{}, cliCoded(err)
	}
	parsed, err := coach.ParseMode(mode)
	if err != nil {
		return studio.CoachPreview{}, cliCoded(err)
	}
	var preview studio.CoachPreview
	if err := w.codedCall(func(svc *study.Service) error {
		prepared, err := coach.Preview(context.Background(), svc, coach.Request{AttemptID: attemptID, Mode: parsed})
		if err != nil {
			return err
		}
		preview = studioCoachPreviewOf(prepared)
		return nil
	}); err != nil {
		return studio.CoachPreview{}, err
	}
	budget, err := w.Budget()
	if err != nil {
		return studio.CoachPreview{}, err
	}
	preview.Budget = budgetSlot(budget, preview.Provider)
	return preview, nil
}

// studioCoachPreviewOf renders one resolved advisory request the way the
// shimmed `tutor preview` document decoded: same fields, same rendering of the
// text.chat evidence, and an empty message list stays an empty list.
func studioCoachPreviewOf(prepared coach.Prepared) studio.CoachPreview {
	preview := studio.CoachPreview{
		AttemptID:     prepared.Attempt.ID,
		Mode:          string(prepared.Mode),
		Provider:      prepared.Config.Provider,
		Model:         prepared.Config.Model,
		Endpoint:      prepared.Config.Endpoint,
		External:      !loopbackEndpoint(prepared.Config.Endpoint),
		Credential:    studio.Credential{Env: prepared.CredentialEnv, Present: prepared.CredentialPresent},
		Consent:       coachConsentOf(prepared.Preview.Context),
		Messages:      make([]studio.CoachMessage, 0, len(prepared.Preview.Request.Chat.Messages)),
		MaxTokens:     prepared.Preview.Request.Chat.MaxTokens,
		RequestSHA256: prepared.Preview.RequestSHA,
		RequestBytes:  prepared.Preview.RequestBytes,
		Limits:        coach.Limits(),
	}
	for _, message := range prepared.Preview.Request.Chat.Messages {
		preview.Messages = append(preview.Messages, studio.CoachMessage{Role: message.Role, Content: message.Content})
	}
	if prepared.TextChat != nil {
		state := *prepared.TextChat
		preview.TextChatState = models.RenderCapability(state)
		if !state.Verified {
			preview.Action = models.TestCommand(prepared.Config.Provider, prepared.Config.Model)
		}
	}
	preview.Verified = prepared.Config.Provider == "" || preview.Action == ""
	return preview
}

// budgetSlot is the budget row a call to providerID draws on; the
// hand-configured legacy endpoint has its own row.
func budgetSlot(budget studio.Budget, providerID string) studio.BudgetProvider {
	slot := providerID
	if slot == "" {
		slot = "legacy"
	}
	for _, item := range budget.Providers {
		if item.Provider == slot || (item.Active && item.Provider == "") {
			return item
		}
	}
	return studio.BudgetProvider{}
}

// CoachHistory lists the advisory calls recorded for one attempt, oldest
// first, through `tutor history`, with each advisory reply's text from
// `tutor show`. A recorded failure carries its typed code and stored message;
// neither document carries a credential or a request body.
func (w *Workspace) CoachHistory(attemptID int64) (studio.CoachHistory, error) {
	if attemptID <= 0 {
		return studio.CoachHistory{}, &studio.Error{Code: "invalid_input", Message: "an attempt id is required"}
	}
	if err := study.GuardPath(w.ports, "tutor"); err != nil {
		return studio.CoachHistory{}, cliCoded(err)
	}
	var calls []store.TutorCall
	if err := w.codedCall(func(svc *study.Service) error {
		history, err := coach.History(context.Background(), svc, coach.HistoryQuery{AttemptID: attemptID})
		if err != nil {
			return err
		}
		calls = history.Calls
		return nil
	}); err != nil {
		return studio.CoachHistory{}, err
	}
	history := studio.CoachHistory{AttemptID: attemptID, Calls: []studio.CoachCall{}, Limits: coach.Limits()}
	for _, call := range calls {
		item := studio.CoachCall{
			CallID:           call.ID,
			CreatedAt:        call.CreatedAt.UTC().Format(time.RFC3339Nano),
			Mode:             call.Mode,
			Provider:         call.Provider,
			Model:            call.Model,
			FailoverFrom:     call.FailoverFrom,
			State:            "advisory",
			Advisory:         true,
			SourceTextShared: call.SourceTextShared,
			LatencyMS:        call.LatencyMS,
		}
		if call.Status == "ok" {
			// Each reply's text is its own `tutor show`: its own guard and its
			// own store open, exactly as the shimmed read had.
			if err := study.GuardPath(w.ports, "tutor"); err != nil {
				return studio.CoachHistory{}, cliCoded(err)
			}
			var shown coach.Shown
			if err := w.codedCall(func(svc *study.Service) error {
				read, err := coach.Call(context.Background(), svc, call.ID)
				if err != nil {
					return err
				}
				shown = read
				return nil
			}); err != nil {
				return studio.CoachHistory{}, err
			}
			item.Text = shown.Call.OutputText
		} else {
			item.State = "failed"
			item.Failure = &studio.CoachFailure{Code: call.ErrorCode, Message: call.ErrorMessage, Action: codeActions[call.ErrorCode]}
		}
		history.Calls = append(history.Calls, item)
	}
	return history, nil
}

// CoachAsk sends the advisory request through `tutor ask`: verification,
// credential, budget, consent scope and the one bounded failover all apply.
// A provider failure is recorded and returned as the reply's Failure; a
// refusal before any call (no_credential, budget_exhausted, …) is an error.
func (w *Workspace) CoachAsk(attemptID int64, mode string) (studio.CoachReply, error) {
	if err := w.requireNode(w.hostTutor); err != nil {
		return studio.CoachReply{}, err
	}
	if err := study.GuardPath(w.ports, "tutor"); err != nil {
		return studio.CoachReply{}, cliCoded(err)
	}
	parsed, err := coach.ParseMode(mode)
	if err != nil {
		return studio.CoachReply{}, cliCoded(err)
	}
	var out coach.Outcome
	if err := w.codedCall(func(svc *study.Service) error {
		result, err := coach.Ask(context.Background(), svc, coach.Request{AttemptID: attemptID, Mode: parsed})
		out = result
		return err
	}); err != nil {
		return studio.CoachReply{}, err
	}
	if out.Failure == nil {
		reply := coachReplyOf(out.Call, out.Failover, coach.Limits())
		reply.State = "advisory"
		reply.Text = out.Text
		reply.Advisory = true
		reply.VerifiedAnswer = false
		reply.Consent = coachConsentOf(out.Prepared.Preview.Context)
		return reply, nil
	}
	reply := coachReplyOf(out.Call, out.Failover, coach.Limits())
	reply.State = "failed"
	reply.Advisory = true
	reply.Failure = &studio.CoachFailure{Code: out.Call.ErrorCode, Message: out.Call.ErrorMessage, Action: out.Failure.Action}
	return reply, nil
}

func coachReplyOf(call store.TutorCall, failover *coach.Failover, limits []string) studio.CoachReply {
	reply := studio.CoachReply{
		CallID:        call.ID,
		AttemptID:     call.AttemptID,
		Mode:          call.Mode,
		Provider:      call.Provider,
		Model:         call.Model,
		RequestSHA256: call.RequestSHA256,
		RequestBytes:  call.RequestBytes,
		LatencyMS:     call.LatencyMS,
		Consent:       studio.CoachConsent{SourceID: call.SourceID, SourceTextShared: call.SourceTextShared, Withheld: []string{}},
		Limits:        limits,
	}
	if failover != nil {
		reply.Failover = &studio.CoachFailover{
			FromProvider: failover.FromProvider,
			FromModel:    failover.FromModel,
			ToProvider:   failover.ToProvider,
			ToModel:      failover.ToModel,
			Reason:       failover.Reason,
			Applied:      failover.Applied,
			Refusal:      failover.Refusal,
			Detail:       failover.Detail,
		}
	}
	return reply
}

func coachConsentOf(context tutor.Context) studio.CoachConsent {
	return studio.CoachConsent{
		PrivateSource:    context.PrivateSource,
		SourceID:         context.SourceID,
		Required:         context.ConsentRequired,
		Granted:          context.ConsentGranted,
		SourceTextShared: context.SourceTextShared,
		Withheld:         append([]string{}, context.Withheld...),
	}
}

// loopbackEndpoint reports whether an endpoint stays on this machine. The
// parsed host must be exactly one of the loopback hosts the provider registry
// and the tutor host accept (tutor.IsLoopback); any other name, including a
// DNS name that merely starts with "127.", is external.
func loopbackEndpoint(endpoint string) bool {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return false
	}
	return tutor.IsLoopback(parsed.Hostname())
}

// GrantConsent records consent for a source's text to reach providers;
// provider empty means every provider. It is `tutor consent`.
func (w *Workspace) GrantConsent(sourceID, providerID string) (studio.Consent, error) {
	return w.consent("consent", sourceID, providerID)
}

// RevokeConsent withholds a source's text again; it is `tutor revoke`.
func (w *Workspace) RevokeConsent(sourceID, providerID string) (studio.Consent, error) {
	return w.consent("revoke", sourceID, providerID)
}

func (w *Workspace) consent(command, sourceID, providerID string) (studio.Consent, error) {
	if strings.TrimSpace(sourceID) == "" || strings.HasPrefix(sourceID, "-") {
		return studio.Consent{}, &studio.Error{Code: "invalid_input", Message: "a source id is required"}
	}
	// The recorded decision was a locked run of `tutor consent|revoke`; the
	// title read below is the separate locked store segment it had.
	var consented coach.Consented
	record := func() error {
		w.mu.Lock()
		defer w.mu.Unlock()
		if err := study.GuardPath(w.ports, "tutor"); err != nil {
			return cliCoded(err)
		}
		return w.codedCall(func(svc *study.Service) error {
			result, err := coach.Consent(context.Background(), svc, coach.ConsentChange{
				SourceID: sourceID,
				Provider: strings.TrimSpace(providerID),
				Granted:  command == "consent",
				Reviewer: consentReviewer,
			})
			consented = result
			return err
		})
	}
	if err := record(); err != nil {
		return studio.Consent{}, err
	}
	event := consented.Event
	consent := studio.Consent{
		SourceID:   event.SourceID,
		Provider:   event.Provider,
		Granted:    event.Granted,
		RecordedAt: event.At.UTC().Format(time.RFC3339Nano),
		Reviewer:   event.Reviewer,
	}
	if err := w.withStore(func(st *store.Store) error {
		src, err := st.SourceMeta(event.SourceID)
		if err == nil {
			consent.Title = src.Title
		}
		return nil
	}); err != nil {
		return studio.Consent{}, err
	}
	return consent, nil
}

// Consents is the consent ledger's latest decision per source and provider.
func (w *Workspace) Consents() ([]studio.Consent, error) {
	if err := study.GuardPath(w.ports, "tutor"); err != nil {
		return nil, cliCoded(err)
	}
	consents := []studio.Consent{}
	if err := w.codedCall(func(svc *study.Service) error {
		entries, err := coach.Consents(context.Background(), svc)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			consents = append(consents, studioConsentOf(entry))
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return consents, nil
}

// studioConsentOf renders one ledger row the way the shimmed `tutor consents`
// document decoded: a row without a recorded decision carries no timestamp.
func studioConsentOf(entry coach.ConsentEntry) studio.Consent {
	consent := studio.Consent{
		SourceID: entry.SourceID,
		Provider: entry.Provider,
		Title:    entry.Title,
		Granted:  entry.Granted,
		Reviewer: entry.Reviewer,
	}
	if entry.RecordedAt != nil {
		consent.RecordedAt = entry.RecordedAt.UTC().Format(time.RFC3339Nano)
	}
	return consent
}

// credentialProviders lists every registry provider id.
func (w *Workspace) credentialProviders() ([]provider.Provider, error) {
	registry, _, err := provider.Load(w.getenv)
	if err != nil {
		return nil, &studio.Error{Code: "invalid_registry", Message: err.Error()}
	}
	return registry.Providers, nil
}
