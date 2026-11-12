// Package provider owns the versioned provider/model registry, the declared
// capability vocabulary, and the mapping from a transport/provider failure to
// one typed failure and one learner action. It never contacts a provider and
// never imports a network package; the Node hosts are the only egress.
package provider

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"unicode"
)

// RegistryVersion is the in-repo registry shape this build understands.
const RegistryVersion = 1

// RegistryEnv overrides the embedded data file. It exists for tests and for a
// deliberately local operator edit; it is never a learner setting.
const RegistryEnv = "PASSQUEST_PROVIDER_REGISTRY"

// EndpointShape is the one transport shape this build speaks.
const EndpointShape = "openai-chat-completions"

// CustomProvider is the provider id that accepts a learner-supplied endpoint
// and model id instead of a registry-declared pair.
const CustomProvider = "custom"

// LegacyCredentialEnv is the goal-0005 credential variable. It remains the
// credential source for a hand-configured endpoint with no registry provider,
// and is accepted by the custom provider.
const LegacyCredentialEnv = "PASSQUEST_TUTOR_API_KEY"

// Capability is one declared model capability. Text chat is the only
// capability this increment can execute or verify; the rest are discoverable
// and never offered as working until a future probe verifies them.
type Capability string

const (
	CapTextChat        Capability = "text.chat"
	CapToolCalls       Capability = "tool.calls"
	CapVision          Capability = "vision"
	CapSearch          Capability = "search"
	CapSpeechSynthesis Capability = "speech.synthesis"
)

// Capabilities is the capability order the views render.
var Capabilities = []Capability{
	CapTextChat,
	CapToolCalls,
	CapVision,
	CapSearch,
	CapSpeechSynthesis,
}

var capabilitySet = map[Capability]bool{
	CapTextChat:        true,
	CapToolCalls:       true,
	CapVision:          true,
	CapSearch:          true,
	CapSpeechSynthesis: true,
}

// ValidCapability reports whether name is a documented capability.
func ValidCapability(name Capability) bool { return capabilitySet[name] }

// Model is one registry-declared model. Capabilities are declarations, not
// verification: model test records the evidence separately.
type Model struct {
	ID           string       `json:"id"`
	Label        string       `json:"label,omitempty"`
	Default      bool         `json:"default,omitempty"`
	Capabilities []Capability `json:"capabilities"`
}

// Has reports whether the model declares capability c.
func (m Model) Has(c Capability) bool {
	for _, declared := range m.Capabilities {
		if declared == c {
			return true
		}
	}
	return false
}

// Provider is one registry entry. EndpointShape names the transport contract;
// a non-custom provider must carry a full chat-completions endpoint.
type Provider struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Endpoint      string  `json:"endpoint"`
	EndpointShape string  `json:"endpoint_shape"`
	CredentialEnv string  `json:"credential_env"`
	Custom        bool    `json:"custom,omitempty"`
	Models        []Model `json:"models"`
}

// DefaultModel returns the provider's declared default text model.
func (p Provider) DefaultModel() (Model, bool) {
	for _, model := range p.Models {
		if model.Default {
			return model, true
		}
	}
	return Model{}, false
}

// Model returns one declared model by id.
func (p Provider) Model(id string) (Model, bool) {
	for _, model := range p.Models {
		if model.ID == id {
			return model, true
		}
	}
	return Model{}, false
}

// Registry is the versioned data file.
type Registry struct {
	Version   int        `json:"version"`
	Providers []Provider `json:"providers"`
}

// Provider returns one provider by id.
func (r Registry) Provider(id string) (Provider, bool) {
	for _, p := range r.Providers {
		if p.ID == id {
			return p, true
		}
	}
	return Provider{}, false
}

// Find resolves one provider/model pair.
func (r Registry) Find(providerID, modelID string) (Provider, Model, bool) {
	p, ok := r.Provider(providerID)
	if !ok {
		return Provider{}, Model{}, false
	}
	if p.Custom {
		if strings.TrimSpace(modelID) == "" {
			return Provider{}, Model{}, false
		}
		return p, Model{ID: strings.TrimSpace(modelID), Capabilities: []Capability{CapTextChat}}, true
	}
	m, ok := p.Model(modelID)
	if !ok {
		return Provider{}, Model{}, false
	}
	return p, m, true
}

// ParseRef splits "<provider>/<model>". A model id may contain further slashes,
// so only the first separator is structural.
func ParseRef(ref string) (string, string, error) {
	value := strings.TrimSpace(ref)
	index := strings.Index(value, "/")
	if index <= 0 || index == len(value)-1 {
		return "", "", errors.New("model reference must be <provider>/<model>")
	}
	providerID := value[:index]
	modelID := value[index+1:]
	if strings.TrimSpace(modelID) == "" {
		return "", "", errors.New("model reference must be <provider>/<model>")
	}
	return providerID, modelID, nil
}

// CredentialEnvFor returns the primary credential variable for a provider. The
// legacy empty provider keeps the goal-0005 variable; the custom provider may
// also use it as a fallback.
func CredentialEnvFor(providerID string) string {
	if providerID == "" {
		return LegacyCredentialEnv
	}
	if providerID == CustomProvider {
		return "PASSQUEST_CUSTOM_API_KEY"
	}
	return "PASSQUEST_" + strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(providerID)) + "_API_KEY"
}

// CredentialValue resolves the credential from the provider's environment
// variable. It returns the variable name, the value and whether it is present.
// A custom endpoint accepts the legacy variable as a documented fallback.
func CredentialValue(getenv func(string) string, providerID string) (string, string, bool) {
	env := CredentialEnvFor(providerID)
	if getenv == nil {
		getenv = os.Getenv
	}
	if value := strings.TrimSpace(getenv(env)); value != "" {
		return env, value, true
	}
	if providerID == CustomProvider {
		if value := strings.TrimSpace(getenv(LegacyCredentialEnv)); value != "" {
			return LegacyCredentialEnv, value, true
		}
	}
	return env, "", false
}

// Load parses the registry selected by PASSQUEST_PROVIDER_REGISTRY, or the
// embedded data file. source is a human-readable origin for the document.
func Load(getenv func(string) string) (Registry, string, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	if path := strings.TrimSpace(getenv(RegistryEnv)); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return Registry{}, "", fmt.Errorf("provider registry %s: %w", path, err)
		}
		registry, err := Decode(data)
		if err != nil {
			return Registry{}, "", fmt.Errorf("provider registry %s: %w", path, err)
		}
		return registry, "file", nil
	}
	registry, err := Default()
	if err != nil {
		return Registry{}, "", err
	}
	return registry, "embedded", nil
}

//go:embed registry.json
var embedded embed.FS

// Default returns the embedded registry.
func Default() (Registry, error) {
	data, err := embedded.ReadFile("registry.json")
	if err != nil {
		return Registry{}, fmt.Errorf("provider registry: read embedded file: %w", err)
	}
	registry, err := Decode(data)
	if err != nil {
		return Registry{}, fmt.Errorf("provider registry: %w", err)
	}
	return registry, nil
}

// Decode parses and validates one registry document.
func Decode(data []byte) (Registry, error) {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var registry Registry
	if err := decoder.Decode(&registry); err != nil {
		return Registry{}, fmt.Errorf("decode: %w", err)
	}
	if err := registry.Validate(); err != nil {
		return Registry{}, err
	}
	return registry, nil
}

var (
	providerIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	envPattern        = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
)

// Validate rejects a registry that cannot drive behaviour truthfully.
func (r Registry) Validate() error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	if r.Version != RegistryVersion {
		add("version %d is not supported (want %d)", r.Version, RegistryVersion)
	}
	if len(r.Providers) == 0 {
		add("at least one provider is required")
	}
	seenProviders := map[string]bool{}
	for index, p := range r.Providers {
		label := p.ID
		if label == "" {
			label = fmt.Sprintf("#%d", index)
		}
		if !providerIDPattern.MatchString(p.ID) {
			add("provider %q: id must be a lowercase slug", label)
		}
		if seenProviders[p.ID] {
			add("provider %q is declared twice", label)
		}
		seenProviders[p.ID] = true
		if strings.TrimSpace(p.Name) == "" {
			add("provider %q: name is required", label)
		}
		if p.EndpointShape != EndpointShape {
			add("provider %q: endpoint_shape %q is not supported", label, p.EndpointShape)
		}
		if !envPattern.MatchString(p.CredentialEnv) {
			add("provider %q: credential_env %q is not an environment variable name", label, p.CredentialEnv)
		}
		if want := CredentialEnvFor(p.ID); p.CredentialEnv != want {
			add("provider %q: credential_env %q must be %q", label, p.CredentialEnv, want)
		}
		if p.Custom {
			if p.Endpoint != "" {
				add("provider %q: a custom provider must not pin an endpoint", label)
			}
			if len(p.Models) > 0 {
				add("provider %q: a custom provider accepts model ids at use time, not in the registry", label)
			}
			continue
		}
		if err := validateEndpoint(p.Endpoint); err != nil {
			add("provider %q: %v", label, err)
		}
		if len(p.Models) == 0 {
			add("provider %q: at least one text model is required", label)
		}
		defaults := 0
		seenModels := map[string]bool{}
		for _, m := range p.Models {
			modelLabel := m.ID
			if modelLabel == "" {
				modelLabel = "<empty>"
			}
			if !validModelID(m.ID) {
				add("provider %q model %q: id is invalid", label, modelLabel)
			}
			if seenModels[m.ID] {
				add("provider %q model %q is declared twice", label, modelLabel)
			}
			seenModels[m.ID] = true
			if m.Default {
				defaults++
			}
			if len(m.Capabilities) == 0 {
				add("provider %q model %q: at least text.chat must be declared", label, modelLabel)
			}
			seenCaps := map[Capability]bool{}
			for _, c := range m.Capabilities {
				if !ValidCapability(c) {
					add("provider %q model %q: capability %q is not documented", label, modelLabel, c)
				}
				if seenCaps[c] {
					add("provider %q model %q: capability %q is declared twice", label, modelLabel, c)
				}
				seenCaps[c] = true
			}
			if !m.Has(CapTextChat) {
				add("provider %q model %q: text.chat is required for a text model", label, modelLabel)
			}
		}
		if defaults != 1 {
			add("provider %q: exactly one default model is required, saw %d", label, defaults)
		}
	}
	if len(problems) > 0 {
		return errors.New("provider registry: " + strings.Join(problems, "; "))
	}
	return nil
}

func validModelID(id string) bool {
	if strings.TrimSpace(id) == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validateEndpoint(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return errors.New("endpoint is required")
	}
	u, err := url.Parse(value)
	switch {
	case err != nil:
		return fmt.Errorf("endpoint is not a URL: %w", err)
	case !u.IsAbs(), u.Opaque != "", u.Host == "":
		return errors.New("endpoint must be an absolute http(s) URL with a host")
	case u.User != nil:
		return errors.New("endpoint must not contain userinfo")
	case u.RawQuery != "":
		return errors.New("endpoint must not contain a query string")
	case u.Fragment != "":
		return errors.New("endpoint must not contain a fragment")
	case u.Scheme != "https" && u.Scheme != "http":
		return fmt.Errorf("endpoint scheme %q is not http or https", u.Scheme)
	case u.Scheme == "http" && !isLoopback(u.Hostname()):
		return errors.New("endpoint uses http off-loopback; use https")
	case !strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/chat/completions"):
		return errors.New("endpoint must end in /chat/completions")
	}
	return nil
}

func isLoopback(host string) bool {
	switch strings.ToLower(strings.Trim(host, "[]")) {
	case "localhost", "127.0.0.1", "::1", "0:0:0:0:0:0:0:1":
		return true
	default:
		return false
	}
}

// ValidateEndpoint checks a learner-supplied custom endpoint with the same
// rules as the registry. It is exported for the CLI.
func ValidateEndpoint(raw string) error { return validateEndpoint(raw) }
