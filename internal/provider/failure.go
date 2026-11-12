package provider

import (
	"strings"

	"github.com/wxxb789/passquest/internal/tutor"
)

// Failure types are stable protocol values. The first six are the contract's
// named failures; provider_error covers a 5xx and the two local failures name
// a local cause instead of pretending the endpoint answered.
const (
	FailureNoCredential     = "no_credential"
	FailureUnauthorized     = "unauthorized"
	FailureRateLimited      = "rate_limited"
	FailureModelMissing     = "model_missing"
	FailureEndpointUnreach  = "endpoint_unreachable"
	FailureInvalidResponse  = "invalid_response"
	FailureProviderError    = "provider_error"
	FailureAgentUnavailable = "agent_unavailable"
	FailureConfig           = "config_error"
)

// Failure is one typed provider failure and the single next action.
type Failure struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Action  string `json:"action"`
}

// Classify maps one host error code, HTTP status and message to a typed
// failure. Unknown codes become provider_error rather than a false success.
func Classify(code string, httpStatus int, message string) Failure {
	text := strings.ToLower(strings.TrimSpace(message))
	switch {
	case code == "missing_credential", code == FailureNoCredential:
		return failure(FailureNoCredential, message)
	case code == "auth_failed", httpStatus == 401, httpStatus == 403:
		return failure(FailureUnauthorized, message)
	case code == "rate_limited", httpStatus == 429:
		return failure(FailureRateLimited, message)
	case httpStatus == 404, strings.Contains(text, "model not found"), strings.Contains(text, "no model"),
		strings.Contains(text, "unknown model"), strings.Contains(text, "does not exist"):
		return failure(FailureModelMissing, message)
	case code == "network_error", code == "timeout":
		return failure(FailureEndpointUnreach, message)
	case code == "invalid_response":
		return failure(FailureInvalidResponse, message)
	case code == "agent_unavailable":
		return failure(FailureAgentUnavailable, message)
	case code == "config_error":
		return failure(FailureConfig, message)
	case httpStatus >= 500:
		return failure(FailureProviderError, message)
	default:
		return failure(FailureProviderError, message)
	}
}

// ClassifyTransport maps a local host process error. The provider was never
// reached, so the endpoint has no verdict and the host install is the action.
func ClassifyTransport(err error) Failure {
	message := "the agent host could not be started"
	if err != nil {
		message = err.Error()
	}
	return failure(FailureAgentUnavailable, message)
}

// ClassifyResponse maps the validated host response to a typed failure. A
// provider that answers with text is not a failure; the caller checks text.
func ClassifyResponse(response tutor.HostResponse) Failure {
	code := "host_error"
	message := "the provider call failed without detail"
	if response.Error != nil {
		if response.Error.Code != "" {
			code = response.Error.Code
		}
		if strings.TrimSpace(response.Error.Message) != "" {
			message = response.Error.Message
		}
	}
	return Classify(code, response.HTTPStatus, message)
}

// Retryable reports whether a failure may trigger the one bounded failover
// retry. Quota and transport/server failures qualify; credential, model and
// malformed-response failures do not.
func Retryable(f Failure) bool {
	switch f.Type {
	case FailureRateLimited, FailureEndpointUnreach, FailureProviderError:
		return true
	default:
		return false
	}
}

func failure(kind, message string) Failure {
	message = strings.TrimSpace(message)
	if message == "" {
		message = defaultMessage(kind)
	}
	return Failure{Type: kind, Message: message, Action: actionFor(kind)}
}

func defaultMessage(kind string) string {
	switch kind {
	case FailureNoCredential:
		return "the provider credential is missing"
	case FailureUnauthorized:
		return "the provider rejected the credential"
	case FailureRateLimited:
		return "the provider rate-limited this call"
	case FailureModelMissing:
		return "the provider does not know this model"
	case FailureEndpointUnreach:
		return "the provider endpoint could not be reached"
	case FailureInvalidResponse:
		return "the provider response was not a readable chat completion"
	case FailureProviderError:
		return "the provider reported a server error"
	case FailureAgentUnavailable:
		return "the local agent host is unavailable"
	default:
		return "the provider configuration could not be used"
	}
}

func actionFor(kind string) string {
	switch kind {
	case FailureNoCredential:
		return "export this provider's credential variable (it is never stored)"
	case FailureUnauthorized:
		return "replace this provider's API key, then run model test again"
	case FailureRateLimited:
		return "wait for the provider quota window, then retry"
	case FailureModelMissing:
		return "choose a model from passquest model list, or update the provider registry"
	case FailureEndpointUnreach:
		return "check the endpoint URL and this machine's network connection"
	case FailureInvalidResponse:
		return "point the endpoint at an OpenAI-compatible /chat/completions API"
	case FailureProviderError:
		return "retry later; the provider reported a server error"
	case FailureAgentUnavailable:
		return "install Node and the agent dependencies, then retry"
	default:
		return "check the endpoint and this Pass Quest build"
	}
}
