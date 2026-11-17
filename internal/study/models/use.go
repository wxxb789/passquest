package models

import (
	"context"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
	"github.com/wxxb789/passquest/internal/tutor"
)

// Used is one model selection: the saved configuration and the facts
// `model use` prints.
type Used struct {
	Config            tutor.Config
	Provider          provider.Provider
	CredentialEnv     string
	CredentialPresent bool
	TextChat          CapabilityState
}

// Use resolves ref against the registry, saves it as the active configuration
// and reports the pair with its credential and text.chat evidence. An invalid
// reference, registry and pair are refusals; a store failure is returned raw.
func Use(ctx context.Context, s *study.Service, ref, endpoint string) (Used, error) {
	_ = ctx
	providerID, modelID, err := provider.ParseRef(ref)
	if err != nil {
		return Used{}, kernel.Refuse("invalid_input", "%s", err.Error())
	}
	registry, _, err := provider.Load(kernel.P(s).Getenv)
	if err != nil {
		return Used{}, kernel.Refuse("invalid_registry", "%s", err.Error())
	}
	base, _, err := storedConfig(s)
	if err != nil {
		return Used{}, err
	}
	cfg, err := providerConfig(base, registry, providerID, modelID, endpoint)
	if err != nil {
		return Used{}, kernel.Refuse("invalid_model", "%s", err.Error())
	}
	if err := kernel.Store(s).SaveTutorConfig(cfg, kernel.P(s).Now()); err != nil {
		return Used{}, err
	}
	env, _, present := kernel.Credential(s, providerID)
	index, err := kernel.LoadVerificationIndex(s)
	if err != nil {
		return Used{}, err
	}
	p, m, _ := registry.Find(providerID, modelID)
	return Used{
		Config:            cfg,
		Provider:          p,
		CredentialEnv:     env,
		CredentialPresent: present,
		TextChat:          CapabilityStateOf(provider.CapTextChat, m.Has(provider.CapTextChat), index.ForModel(providerID, modelID)),
	}, nil
}
