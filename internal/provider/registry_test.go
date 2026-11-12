package provider

import (
	"strings"
	"testing"
)

func validProvider(id string) Provider {
	return Provider{
		ID:            id,
		Name:          strings.ToUpper(id),
		Endpoint:      "https://api.example.com/v1/chat/completions",
		EndpointShape: EndpointShape,
		CredentialEnv: "PASSQUEST_" + strings.ToUpper(id) + "_API_KEY",
		Models: []Model{
			{ID: id + "-small", Default: true, Capabilities: []Capability{CapTextChat}},
		},
	}
}

func TestEmbeddedRegistryIsValidAndCoversRequiredProviders(t *testing.T) {
	registry, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"openai", "qwen", "doubao", "deepseek", "gemini", "custom"} {
		p, ok := registry.Provider(id)
		if !ok {
			t.Fatalf("embedded registry does not declare %s", id)
		}
		if id != CustomProvider {
			if _, ok := p.DefaultModel(); !ok {
				t.Fatalf("%s has no default model", id)
			}
		}
	}
	if registry.Version != RegistryVersion {
		t.Fatalf("version = %d", registry.Version)
	}
	custom, _ := registry.Provider(CustomProvider)
	if !custom.Custom || custom.Endpoint != "" || len(custom.Models) != 0 {
		t.Fatalf("custom provider shape = %+v", custom)
	}
}

func TestAddingAnEntryAppearsInTheRegistry(t *testing.T) {
	registry, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	registry.Providers = append(registry.Providers, validProvider("gamma"))
	if err := registry.Validate(); err != nil {
		t.Fatalf("registry with an added entry is invalid: %v", err)
	}
	p, m, ok := registry.Find("gamma", "gamma-small")
	if !ok || p.Name != "GAMMA" || !m.Has(CapTextChat) {
		t.Fatalf("added entry not findable: %+v %+v %v", p, m, ok)
	}
}

func TestRegistryValidationRejectsUnsafeData(t *testing.T) {
	base := func() Registry {
		return Registry{Version: RegistryVersion, Providers: []Provider{validProvider("alpha")}}
	}
	tests := []struct {
		name   string
		mutate func(*Registry)
		want   string
	}{
		{"unknown version", func(r *Registry) { r.Version = 2 }, "version 2"},
		{"duplicate provider", func(r *Registry) { r.Providers = append(r.Providers, validProvider("alpha")) }, "declared twice"},
		{"duplicate model", func(r *Registry) {
			r.Providers[0].Models = append(r.Providers[0].Models, Model{ID: "alpha-small", Capabilities: []Capability{CapTextChat}})
		}, "declared twice"},
		{"unknown capability", func(r *Registry) {
			r.Providers[0].Models[0].Capabilities = []Capability{"magic"}
		}, "not documented"},
		{"two defaults", func(r *Registry) {
			r.Providers[0].Models[0].Default = true
			r.Providers[0].Models = append(r.Providers[0].Models, Model{ID: "alpha-large", Default: true, Capabilities: []Capability{CapTextChat}})
		}, "exactly one default"},
		{"no default", func(r *Registry) { r.Providers[0].Models[0].Default = false }, "exactly one default"},
		{"http off loopback", func(r *Registry) {
			r.Providers[0].Endpoint = "http://api.example.com/v1/chat/completions"
		}, "http off-loopback"},
		{"query in endpoint", func(r *Registry) {
			r.Providers[0].Endpoint = "https://api.example.com/v1/chat/completions?key=x"
		}, "query string"},
		{"wrong path", func(r *Registry) {
			r.Providers[0].Endpoint = "https://api.example.com/v1/responses"
		}, "/chat/completions"},
		{"bad env", func(r *Registry) { r.Providers[0].CredentialEnv = "lower-case" }, "environment variable"},
		{"custom with models", func(r *Registry) {
			r.Providers = append(r.Providers, Provider{
				ID: CustomProvider, Name: "Custom", EndpointShape: EndpointShape,
				CredentialEnv: "PASSQUEST_CUSTOM_API_KEY", Custom: true,
				Models: []Model{{ID: "x", Default: true, Capabilities: []Capability{CapTextChat}}},
			})
		}, "custom provider accepts model ids at use time"},
		{"missing text chat", func(r *Registry) {
			r.Providers[0].Models[0].Capabilities = []Capability{CapVision}
		}, "text.chat is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := base()
			test.mutate(&registry)
			err := registry.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() = %v, want %q", err, test.want)
			}
		})
	}
}

func TestParseRefSplitsOnlyTheFirstSlash(t *testing.T) {
	providerID, modelID, err := ParseRef("custom/vendor/model-1")
	if err != nil || providerID != "custom" || modelID != "vendor/model-1" {
		t.Fatalf("ParseRef = %q, %q, %v", providerID, modelID, err)
	}
	for _, ref := range []string{"", "openai", "/model", "openai/"} {
		if _, _, err := ParseRef(ref); err == nil {
			t.Fatalf("ParseRef(%q) accepted an invalid reference", ref)
		}
	}
}

func TestCredentialValueUsesTheProviderVariable(t *testing.T) {
	env := map[string]string{
		"PASSQUEST_OPENAI_API_KEY": "openai-key",
		LegacyCredentialEnv:        "legacy-key",
	}
	getenv := func(key string) string { return env[key] }
	if name, value, ok := CredentialValue(getenv, "openai"); !ok || name != "PASSQUEST_OPENAI_API_KEY" || value != "openai-key" {
		t.Fatalf("openai credential = %q, %q, %v", name, value, ok)
	}
	if _, value, ok := CredentialValue(getenv, CustomProvider); !ok || value != "legacy-key" {
		t.Fatalf("custom provider did not fall back to the legacy variable: %q, %v", value, ok)
	}
	if name, _, ok := CredentialValue(getenv, "qwen"); ok || name != "PASSQUEST_QWEN_API_KEY" {
		t.Fatalf("qwen unexpectedly resolved a credential: %q, %v", name, ok)
	}
}
