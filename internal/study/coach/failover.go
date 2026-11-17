package coach

import (
	"fmt"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
	"github.com/wxxb789/passquest/internal/tutor"
)

// Failover is one bounded retry decision: the provider that failed, the
// candidate it was retried on and why the retry did or did not happen.
type Failover struct {
	FromProvider string
	FromModel    string
	ToProvider   string
	ToModel      string
	Reason       string
	Applied      bool
	Refusal      string
	Detail       string
}

// attemptFailover makes at most one retry on another registry provider. Every
// precondition is checked before a target is contacted; a refusal appends the
// reason and never widens the consent scope.
func attemptFailover(s *study.Service, registry provider.Registry, cfg tutor.Config, record attempt.Attempt, mode tutor.Mode, sourceTarget tutor.Target, sourceCall store.TutorCall, sourceFailure provider.Failure) (store.TutorCall, string, *provider.Failure, *Failover, error) {
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
		verification, found, err := kernel.Store(s).LatestProviderVerification(p.ID, model.ID, string(provider.CapTextChat))
		if err != nil {
			return store.TutorCall{}, "", nil, nil, err
		}
		if !found || verification.Status != "verified" {
			keepReason(p.ID, "target_unverified", "model "+model.ID+" is not verified for text.chat")
			continue
		}
		env, key, present := kernel.Credential(s, p.ID)
		if !present {
			keepReason(p.ID, "no_credential", env+" is not set")
			continue
		}
		target := sourceTarget
		target.ConsentGranted = false
		if target.PrivateSource && target.SourceID != "" {
			event, ok, err := kernel.Store(s).TutorConsent(target.SourceID, p.ID)
			if err != nil {
				return store.TutorCall{}, "", nil, nil, err
			}
			target.ConsentGranted = ok && event.Granted
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
		reserveAt := kernel.P(s).Now()
		lease, _, err := kernel.Reserve(s, (*store.Store).ReserveTutorCall, p.ID, reserveAt, targetCfg.DailyCalls)
		if err != nil {
			return store.TutorCall{}, "", nil, nil, err
		}
		if lease.ID() == 0 {
			keepReason(p.ID, "budget_exhausted", "the target has no provider budget left today")
			continue
		}
		// The slot is spent from here on: a recording failure leaves it
		// claimed, exactly as today.
		lease.Sending()
		targetCall := callRecord(record, preview, targetCfg, mode, p.ID, model.ID, cfg.Provider, kernel.P(s).Now())
		saved, text, failure, err := sendCall(s, targetCall, preview, key, lease)
		if err != nil {
			return store.TutorCall{}, "", nil, nil, err
		}
		event := store.ProviderFailover{
			At:           kernel.P(s).Now(),
			FromProvider: cfg.Provider,
			FromModel:    cfg.Model,
			ToProvider:   p.ID,
			ToModel:      model.ID,
			Reason:       sourceFailure.Type,
			Applied:      true,
			FromCallID:   sourceCall.ID,
			ToCallID:     saved.ID,
		}
		if _, err := kernel.Store(s).AppendProviderFailover(event); err != nil {
			return store.TutorCall{}, "", nil, nil, err
		}
		view := &Failover{
			FromProvider: cfg.Provider,
			FromModel:    cfg.Model,
			ToProvider:   p.ID,
			ToModel:      model.ID,
			Reason:       sourceFailure.Type,
			Applied:      true,
		}
		return saved, text, failure, view, nil
	}
	view := &Failover{
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
		At:           kernel.P(s).Now(),
		FromProvider: cfg.Provider,
		FromModel:    cfg.Model,
		ToProvider:   firstTarget,
		Reason:       sourceFailure.Type,
		Refusal:      view.Refusal,
		FromCallID:   sourceCall.ID,
	}
	if _, err := kernel.Store(s).AppendProviderFailover(event); err != nil {
		return store.TutorCall{}, "", nil, nil, err
	}
	return sourceCall, "", &sourceFailure, view, nil
}

func consentScopeKey(target tutor.Target) string {
	if !target.PrivateSource || target.SourceID == "" {
		return "first-party"
	}
	return fmt.Sprintf("%s=%t", target.SourceID, target.ConsentGranted)
}

func grantWord(granted bool) string {
	if granted {
		return "authorized"
	}
	return "withheld"
}
