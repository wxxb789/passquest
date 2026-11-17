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
