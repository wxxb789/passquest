// Package models orchestrates the provider and model surfaces: the registry
// catalog with its verification evidence and per-provider budget, one selected
// model and one bounded capability probe. Results carry exactly the inputs the
// CLI's view builders and the desktop's studio mappers read; nothing here
// renders.
package models

import (
	"errors"
	"fmt"
	"strings"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
	"github.com/wxxb789/passquest/internal/tutor"
)

// storedConfig is the saved provider configuration, filled with the defaults
// an empty database has not saved yet.
func storedConfig(s *study.Service) (tutor.Config, bool, error) {
	cfg, configured, err := kernel.Store(s).TutorConfig()
	if err != nil {
		return tutor.Config{}, false, err
	}
	if !configured {
		cfg = tutor.DefaultConfig()
	}
	return cfg, configured, nil
}

// providerConfig resolves one registry model into the transport configuration
// the existing tutor/agent paths already speak. A custom provider needs the
// learner's endpoint; a registry provider's endpoint is authoritative.
func providerConfig(base tutor.Config, registry provider.Registry, providerID, modelID, endpoint string) (tutor.Config, error) {
	p, m, ok := registry.Find(providerID, modelID)
	if !ok {
		return tutor.Config{}, fmt.Errorf("provider/model %s/%s is not in the registry", providerID, modelID)
	}
	if p.Custom {
		if strings.TrimSpace(endpoint) == "" && strings.TrimSpace(base.Provider) == p.ID {
			endpoint = base.Endpoint
		}
		if strings.TrimSpace(endpoint) == "" {
			return tutor.Config{}, errors.New("a custom provider requires --endpoint URL")
		}
		if err := provider.ValidateEndpoint(endpoint); err != nil {
			return tutor.Config{}, err
		}
	} else {
		if strings.TrimSpace(endpoint) != "" {
			return tutor.Config{}, errors.New("--endpoint is only accepted for the custom provider")
		}
		endpoint = p.Endpoint
	}
	cfg := base
	cfg.Provider = p.ID
	cfg.Model = m.ID
	cfg.Endpoint = strings.TrimSpace(endpoint)
	if err := cfg.Validate(); err != nil {
		return tutor.Config{}, err
	}
	return cfg, nil
}

// Limits is the disclosure every model document carries: what a registry entry
// and a budget row mean. The CLI's model documents and the window's model list
// show the same statements.
func Limits() []string {
	return []string{
		"A registry entry is a declaration; only model test records verified evidence.",
		"A capability that is not verified is never offered as working.",
		"The daily budget is counted per provider in UTC; the ceiling is the printed sum, not one global cap.",
		"Model text, provider availability and price are not exam facts and never change an attempt or answer key.",
	}
}

// TestCommand is the command that verifies one provider/model selection. The
// tutor and note-refinement previews point at it while text.chat is
// unverified, in the CLI documents and in the window values alike.
func TestCommand(providerID, modelID string) string {
	return fmt.Sprintf("passquest model test %s/%s", strings.TrimSpace(providerID), modelID)
}
