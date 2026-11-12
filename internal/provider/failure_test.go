package provider

import (
	"strings"
	"testing"
)

func TestClassifyMapsEveryNamedFailureAndAction(t *testing.T) {
	tests := []struct {
		name   string
		code   string
		status int
		want   string
		action string
	}{
		{"missing credential", "missing_credential", 0, FailureNoCredential, "export"},
		{"auth code", "auth_failed", 401, FailureUnauthorized, "replace"},
		{"auth status", "provider_error", 403, FailureUnauthorized, "replace"},
		{"rate code", "rate_limited", 429, FailureRateLimited, "wait"},
		{"model 404", "provider_error", 404, FailureModelMissing, "choose a model"},
		{"model wording", "provider_error", 400, FailureModelMissing, "choose a model"},
		{"network", "network_error", 0, FailureEndpointUnreach, "endpoint URL"},
		{"timeout", "timeout", 0, FailureEndpointUnreach, "endpoint URL"},
		{"malformed", "invalid_response", 200, FailureInvalidResponse, "OpenAI-compatible"},
		{"server error", "provider_error", 503, FailureProviderError, "retry later"},
		{"local host", "agent_unavailable", 0, FailureAgentUnavailable, "install Node"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message := "detail"
			if test.name == "model wording" {
				message = "the model does not exist"
			}
			failure := Classify(test.code, test.status, message)
			if failure.Type != test.want {
				t.Fatalf("type = %q, want %q", failure.Type, test.want)
			}
			if failure.Action == "" || failure.Message == "" {
				t.Fatalf("failure has no action/message: %+v", failure)
			}
			if !strings.Contains(failure.Action, test.action) {
				t.Fatalf("action %q does not contain %q", failure.Action, test.action)
			}
		})
	}
}

func TestRetryableOnlyTransportQuotaAndServerFailures(t *testing.T) {
	for _, kind := range []string{FailureRateLimited, FailureEndpointUnreach, FailureProviderError} {
		if !Retryable(Failure{Type: kind}) {
			t.Fatalf("%s is not retryable", kind)
		}
	}
	for _, kind := range []string{FailureNoCredential, FailureUnauthorized, FailureModelMissing, FailureInvalidResponse, FailureAgentUnavailable} {
		if Retryable(Failure{Type: kind}) {
			t.Fatalf("%s must not be retryable", kind)
		}
	}
}
