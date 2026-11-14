package cli

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/studio"
	"github.com/wxxb789/passquest/internal/tutor"
)

// consentReviewer attributes a consent decision made in the window.
const consentReviewer = "desktop"

// CoachPreview builds the exact advisory request `tutor preview` builds for a
// recorded attempt: provider and model, whether the endpoint is external, the
// messages with their hash and size, the source consent state and the budget
// slot. Nothing is sent.
func (w *Workspace) CoachPreview(attemptID int64, mode string) (studio.CoachPreview, error) {
	var doc tutorPreviewDoc
	if err := w.run("tutor", "preview", "--attempt", strconv.FormatInt(attemptID, 10), "--mode", mode).decode(&doc); err != nil {
		return studio.CoachPreview{}, err
	}
	preview := studio.CoachPreview{
		AttemptID:     doc.Attempt.ID,
		Mode:          doc.Mode,
		Provider:      doc.Provider.Provider,
		Model:         doc.Provider.Model,
		Endpoint:      doc.Provider.Endpoint,
		External:      !loopbackEndpoint(doc.Provider.Endpoint),
		Verified:      doc.Provider.Provider == "" || doc.Provider.TextChatAction == "",
		TextChatState: doc.Provider.TextChatState,
		Action:        doc.Provider.TextChatAction,
		Credential:    studio.Credential{Env: doc.Provider.CredentialEnv, Present: doc.Provider.CredentialPresent},
		Consent:       coachConsentOf(doc.Context),
		Messages:      make([]studio.CoachMessage, 0, len(doc.Chat.Messages)),
		MaxTokens:     doc.Chat.MaxTokens,
		RequestSHA256: doc.RequestSHA256,
		RequestBytes:  doc.RequestBytes,
		Limits:        doc.Limits,
	}
	for _, message := range doc.Chat.Messages {
		preview.Messages = append(preview.Messages, studio.CoachMessage{Role: message.Role, Content: message.Content})
	}
	budget, err := w.Budget()
	if err != nil {
		return studio.CoachPreview{}, err
	}
	slot := doc.Provider.Provider
	if slot == "" {
		slot = "legacy"
	}
	for _, item := range budget.Providers {
		if item.Provider == slot || (item.Active && item.Provider == "") {
			preview.Budget = item
			break
		}
	}
	return preview, nil
}

// CoachAsk sends the advisory request through `tutor ask`: verification,
// credential, budget, consent scope and the one bounded failover all apply.
// A provider failure is recorded and returned as the reply's Failure; a
// refusal before any call (no_credential, budget_exhausted, …) is an error.
func (w *Workspace) CoachAsk(attemptID int64, mode string) (studio.CoachReply, error) {
	if err := w.requireNode(w.hostTutor); err != nil {
		return studio.CoachReply{}, err
	}
	result := w.run("tutor", "ask", "--attempt", strconv.FormatInt(attemptID, 10), "--mode", mode)
	if result.code == 0 {
		var doc tutorAdvisoryDoc
		if err := result.decode(&doc); err != nil {
			return studio.CoachReply{}, err
		}
		reply := coachReplyOf(doc.Call, doc.Failover, doc.Limits)
		reply.State = "advisory"
		reply.Text = doc.Suggestion.Text
		reply.Advisory = true
		reply.VerifiedAnswer = false
		reply.Consent = coachConsentOf(doc.Context)
		return reply, nil
	}
	var failure tutorFailureDoc
	if result.code == 3 && json.Unmarshal(result.stderr, &failure) == nil && failure.State == "failed" {
		reply := coachReplyOf(failure.Call, failure.Failover, failure.Limits)
		reply.State = "failed"
		reply.Advisory = true
		reply.Failure = &studio.CoachFailure{Code: failure.Error.Code, Message: failure.Error.Message, Action: failure.Action}
		return reply, nil
	}
	return studio.CoachReply{}, result.err()
}

func coachReplyOf(call tutorCallView, failover *tutorFailoverView, limits []string) studio.CoachReply {
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

func coachConsentOf(context tutorContextView) studio.CoachConsent {
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
	args := []string{"tutor", command, "--reviewer", consentReviewer}
	if strings.TrimSpace(providerID) != "" {
		args = append(args, "--provider="+providerID)
	}
	args = append(args, sourceID)
	w.mu.Lock()
	result := w.run(args...)
	w.mu.Unlock()
	var doc tutorConsentDoc
	if err := result.decode(&doc); err != nil {
		return studio.Consent{}, err
	}
	consent := studio.Consent{
		SourceID:   doc.SourceID,
		Provider:   doc.Provider,
		Granted:    doc.Granted,
		RecordedAt: doc.RecordedAt,
		Reviewer:   doc.Reviewer,
	}
	if err := w.withStore(func(st *store.Store) error {
		src, err := st.SourceMeta(doc.SourceID)
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
	var doc tutorConsentsDoc
	if err := w.run("tutor", "consents").decode(&doc); err != nil {
		return nil, err
	}
	consents := []studio.Consent{}
	if err := convertJSON(doc.Sources, &consents); err != nil {
		return nil, workspaceError(err)
	}
	return consents, nil
}

// credentialProviders lists every registry provider id.
func (w *Workspace) credentialProviders() ([]provider.Provider, error) {
	registry, _, err := provider.Load(w.getenv)
	if err != nil {
		return nil, &studio.Error{Code: "invalid_registry", Message: err.Error()}
	}
	return registry.Providers, nil
}
