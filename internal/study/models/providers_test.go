package models

import (
	"testing"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/tutor"
)

func catalogRegistry(alphaEndpoint, alphaShape string) provider.Registry {
	return provider.Registry{Version: provider.RegistryVersion, Providers: []provider.Provider{
		{
			ID: "alpha", Name: "ALPHA", Endpoint: alphaEndpoint, EndpointShape: alphaShape,
			CredentialEnv: "PASSQUEST_ALPHA_API_KEY",
			Models:        []provider.Model{{ID: "alpha-text", Default: true, Capabilities: []provider.Capability{provider.CapTextChat}}},
		},
		{
			ID: provider.CustomProvider, Name: "Custom compatible endpoint",
			EndpointShape: provider.EndpointShape, CredentialEnv: "PASSQUEST_CUSTOM_API_KEY", Custom: true,
		},
	}}
}

func TestProviderRowsReportTheActiveEndpointShape(t *testing.T) {
	chatEndpoint := "https://alpha.example.com/v1/chat/completions"
	responsesEndpoint := "https://alpha.example.com/v1/responses"
	tests := []struct {
		name       string
		alphaShape string
		active     tutor.Config
		want       map[string]string
	}{
		{
			"active custom responses endpoint",
			provider.EndpointShape,
			tutor.Config{Provider: provider.CustomProvider, Model: "M", Endpoint: "https://api.example.com/v1/responses"},
			map[string]string{provider.CustomProvider: provider.ShapeResponses, "alpha": provider.EndpointShape},
		},
		{
			"active custom chat endpoint",
			provider.EndpointShape,
			tutor.Config{Provider: provider.CustomProvider, Model: "M", Endpoint: "https://api.example.com/v1/chat/completions"},
			map[string]string{provider.CustomProvider: provider.EndpointShape, "alpha": provider.EndpointShape},
		},
		{
			"legacy responses endpoint",
			provider.EndpointShape,
			tutor.Config{Model: "M", Endpoint: "https://api.example.com/v1/responses"},
			map[string]string{"legacy": provider.ShapeResponses, "alpha": provider.EndpointShape},
		},
		{
			"legacy chat endpoint",
			provider.EndpointShape,
			tutor.Config{Model: "M", Endpoint: "https://api.example.com/v1/chat/completions"},
			map[string]string{"legacy": provider.EndpointShape, "alpha": provider.EndpointShape},
		},
		{
			"inactive custom keeps its declaration beside a responses provider",
			provider.ShapeResponses,
			tutor.Config{Provider: "alpha", Model: "alpha-text", Endpoint: responsesEndpoint},
			map[string]string{provider.CustomProvider: provider.EndpointShape, "alpha": provider.ShapeResponses},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows := ProvidersOf(Listing{
				Registry:   catalogRegistry(chatEndpoint, test.alphaShape),
				Configured: true,
				Active:     test.active,
			})
			shapes := map[string]string{}
			for _, row := range rows {
				shapes[row.ID] = row.EndpointShape
			}
			for id, want := range test.want {
				if shapes[id] != want {
					t.Fatalf("row %q endpoint_shape = %q, want %q (all rows: %+v)", id, shapes[id], want, shapes)
				}
			}
		})
	}
}
