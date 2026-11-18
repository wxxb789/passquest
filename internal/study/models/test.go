package models

import (
	"context"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
	"github.com/wxxb789/passquest/internal/tutor"
)

// Tested is one bounded capability probe: the request identity, the provider's
// answer or failure, and the verdict recorded as verification evidence. A
// missing credential is a failure with CallMade false, not an error.
type Tested struct {
	Provider   provider.Provider
	Model      string
	Config     tutor.Config
	Env        string
	SHA        string
	Bytes      int
	LatencyMS  int64
	HTTPStatus int
	CheckedAt  time.Time
	Failure    *provider.Failure
	CallMade   bool
}

// Test probes the pair's text.chat capability once and records the verdict.
// Refusals cover the invalid reference, registry, pair, probe request and the
// day's budget; a provider, transport or invalid-answer failure is a Tested
// outcome carrying CallMade.
func Test(ctx context.Context, s *study.Service, ref, endpoint string) (Tested, error) {
	_ = ctx
	providerID, modelID, err := provider.ParseRef(ref)
	if err != nil {
		return Tested{}, kernel.Refuse("invalid_input", "%s", err.Error())
	}
	registry, _, err := provider.Load(kernel.P(s).Getenv)
	if err != nil {
		return Tested{}, kernel.Refuse("invalid_registry", "%s", err.Error())
	}
	base, _, err := storedConfig(s)
	if err != nil {
		return Tested{}, err
	}
	cfg, err := providerConfig(base, registry, providerID, modelID, endpoint)
	if err != nil {
		return Tested{}, kernel.Refuse("invalid_model", "%s", err.Error())
	}
	p, m, _ := registry.Find(providerID, modelID)
	if !m.Has(provider.CapTextChat) {
		return Tested{}, kernel.Refuse("capability_not_declared",
			"%s/%s does not declare text.chat", providerID, modelID)
	}
	env, key, present := kernel.Credential(s, providerID)
	now := kernel.P(s).Now()
	tested := Tested{Provider: p, Model: modelID, Config: cfg, Env: env, CheckedAt: now}
	if !present {
		failure := provider.Classify(provider.FailureNoCredential, 0, "")
		failure.Action = "export " + env + " (it is never stored)"
		return failModelTest(s, tested, failure, false, nil)
	}
	budgetNow := kernel.P(s).Now()
	lease, used, err := kernel.Reserve(s, (*store.Store).ReserveTutorCall, providerID, budgetNow, cfg.DailyCalls)
	if err != nil {
		return Tested{}, err
	}
	if lease.ID() == 0 {
		return Tested{}, kernel.Refuse("budget_exhausted",
			"%s has used %d of %d provider calls today (UTC); raise the budget with passquest tutor config --daily-calls N",
			providerID, used, cfg.DailyCalls)
	}
	defer lease.Close()
	request, sha, bytes, err := tutor.ProbeRequest(cfg.Endpoint, cfg.Model, cfg.TimeoutMS)
	if err != nil {
		return Tested{}, kernel.Refuse("invalid_model", "%s", err.Error())
	}
	request.CredentialEnv = env
	tested.SHA = sha
	tested.Bytes = bytes
	lease.Sending()
	started := kernel.P(s).Now()
	response, transportErr := kernel.P(s).Tutor.Call(context.Background(), request)
	latency := kernel.P(s).Now().Sub(started).Milliseconds()
	tested.LatencyMS = latency
	tested.HTTPStatus = response.HTTPStatus
	if transportErr != nil {
		failure := provider.ClassifyTransport(transportErr)
		failure.Message = kernel.Redact(failure.Message, key)
		return failModelTest(s, tested, failure, true, lease)
	}
	if response.Status == "error" {
		failure := provider.ClassifyResponse(response)
		failure.Message = kernel.Redact(failure.Message, key)
		return failModelTest(s, tested, failure, true, lease)
	}
	text := strings.TrimSpace(response.Text)
	if text == "" || !kernel.ValidText(text) {
		failure := provider.Classify(provider.FailureInvalidResponse, response.HTTPStatus, "the capability probe returned no valid text")
		return failModelTest(s, tested, failure, true, lease)
	}
	verified := store.ProviderVerification{
		Provider:   providerID,
		Model:      modelID,
		Capability: string(provider.CapTextChat),
		Status:     "verified",
		CallMade:   true,
		LatencyMS:  latency,
		HTTPStatus: response.HTTPStatus,
		CheckedAt:  now,
	}
	if _, err := kernel.Store(s).AppendProviderVerification(verified); err != nil {
		return Tested{}, err
	}
	lease.Release()
	tested.CallMade = true
	return tested, nil
}

// failModelTest records one failed verification and releases the claimed slot;
// the missing-credential path has no lease. A recording failure returns before
// the release, so a sent slot stays claimed.
func failModelTest(s *study.Service, tested Tested, failure provider.Failure, callMade bool, lease *kernel.Lease) (Tested, error) {
	if _, err := kernel.Store(s).AppendProviderVerification(store.ProviderVerification{
		Provider:    tested.Provider.ID,
		Model:       tested.Model,
		Capability:  string(provider.CapTextChat),
		Status:      "failed",
		FailureType: failure.Type,
		Action:      failure.Action,
		CallMade:    callMade,
		LatencyMS:   tested.LatencyMS,
		HTTPStatus:  tested.HTTPStatus,
		CheckedAt:   tested.CheckedAt,
	}); err != nil {
		return Tested{}, err
	}
	if lease != nil {
		lease.Release()
	}
	tested.CallMade = callMade
	tested.Failure = &failure
	return tested, nil
}
