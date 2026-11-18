package kernel

import (
	"fmt"
	"strings"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/tutor"
)

// ActiveProvider refreshes the endpoint from the registry so a data edit
// drives behaviour on the next call. The active model id stays the stored
// setting. The returned configuration is the one every caller should use.
func ActiveProvider(cfg tutor.Config, registry provider.Registry) (provider.Provider, provider.Model, tutor.Config, bool, error) {
	providerID := strings.TrimSpace(cfg.Provider)
	if providerID == "" {
		return provider.Provider{}, provider.Model{}, cfg, false, nil
	}
	p, m, ok := registry.Find(providerID, cfg.Model)
	if !ok {
		return provider.Provider{}, provider.Model{}, cfg, true,
			fmt.Errorf("active model %s/%s is no longer in the provider registry", providerID, cfg.Model)
	}
	if !p.Custom {
		cfg.Endpoint = p.Endpoint
	}
	return p, m, cfg, true, nil
}

// Credential is the provider credential variable and value the process
// environment supplies.
func Credential(u *Unit, providerID string) (string, string, bool) {
	return provider.CredentialValue(P(u).Getenv, providerID)
}

// Configured reads the saved provider configuration and refuses with the
// caller's notConfigured message when no provider is saved: the first check
// every advisory entry point makes.
func Configured(u *Unit, notConfigured string) (tutor.Config, error) {
	cfg, configured, err := Store(u).TutorConfig()
	if err != nil {
		return tutor.Config{}, err
	}
	if !configured {
		return tutor.Config{}, Refuse("not_configured", "%s", notConfigured)
	}
	return cfg, nil
}

// ResolveActive loads the registry, resolves the active registry provider and
// refreshes the endpoint from registry data. It refuses a pair that does not
// declare text.chat, which every text surface requires. A legacy configuration
// (no provider) passes through with active=false.
func ResolveActive(u *Unit, cfg tutor.Config) (tutor.Config, provider.Registry, bool, error) {
	registry, _, err := provider.Load(P(u).Getenv)
	if err != nil {
		return cfg, provider.Registry{}, false, Refuse("invalid_registry", "%s", err.Error())
	}
	providerID := strings.TrimSpace(cfg.Provider)
	if providerID == "" {
		return cfg, registry, false, nil
	}
	_, model, refreshed, _, err := ActiveProvider(cfg, registry)
	if err != nil {
		return cfg, registry, false, Refuse("unknown_model", "%s", err.Error())
	}
	if !model.Has(provider.CapTextChat) {
		return cfg, registry, false, Refuse("capability_not_declared",
			"%s/%s does not declare text.chat", providerID, cfg.Model)
	}
	return refreshed, registry, true, nil
}

// RequireVerifiedTextChat refuses a registry pair whose text.chat has no
// verified evidence. The legacy hand-configured path needs no verification.
func RequireVerifiedTextChat(u *Unit, registry provider.Registry, providerID, modelID string) error {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil
	}
	_, model, ok := registry.Find(providerID, modelID)
	if !ok {
		return Refuse("unknown_model",
			"%s/%s is no longer in the provider registry", providerID, modelID)
	}
	if !model.Has(provider.CapTextChat) {
		return Refuse("capability_not_declared",
			"%s/%s does not declare text.chat", providerID, modelID)
	}
	verification, found, err := Store(u).LatestProviderVerification(providerID, modelID, string(provider.CapTextChat))
	if err != nil {
		return err
	}
	if !found || verification.Status != "verified" {
		detail := "text.chat is declared but unverified"
		if found && verification.Status == "failed" {
			detail = "the last text.chat test failed: " + verification.FailureType
		}
		return Refuse("capability_unverified",
			"%s/%s: %s; run passquest model test %s/%s before asking",
			providerID, modelID, detail, providerID, modelID)
	}
	return nil
}

// VerificationIndex groups the latest verification per provider, model and
// capability. A model's evidence is never applied to a sibling.
type VerificationIndex map[modelRef]map[provider.Capability]store.ProviderVerification

type modelRef struct {
	provider string
	model    string
}

// LoadVerificationIndex reads every recorded verification into per-model
// groups.
func LoadVerificationIndex(u *Unit) (VerificationIndex, error) {
	all, err := Store(u).ProviderVerifications()
	if err != nil {
		return nil, err
	}
	index := VerificationIndex{}
	for _, event := range all {
		ref := modelRef{provider: event.Provider, model: event.Model}
		if index[ref] == nil {
			index[ref] = map[provider.Capability]store.ProviderVerification{}
		}
		index[ref][provider.Capability(event.Capability)] = event
	}
	return index, nil
}

// ForModel is one model's recorded verifications.
func (index VerificationIndex) ForModel(providerID, modelID string) map[provider.Capability]store.ProviderVerification {
	return index[modelRef{provider: providerID, model: modelID}]
}
