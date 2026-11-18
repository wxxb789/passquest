package models

import (
	"time"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
)

// CapabilityState is one capability's evidence state: the fields the CLI's
// capability view and the desktop's text.chat state read.
type CapabilityState struct {
	ID          string
	Declared    bool
	State       string
	Verified    bool
	CheckedAt   string
	FailureType string
	Action      string
}

// CapabilityStateOf is today's cli.capabilityView: the state a capability is
// shown as, from its declaration and the latest recorded verification.
func CapabilityStateOf(capability provider.Capability, declared bool, verifications map[provider.Capability]store.ProviderVerification) CapabilityState {
	state := CapabilityState{ID: string(capability), Declared: declared, State: "unsupported"}
	if !declared {
		return state
	}
	state.State = "declared"
	if event, ok := verifications[capability]; ok {
		state.CheckedAt = event.CheckedAt.UTC().Format(time.RFC3339)
		switch event.Status {
		case "verified":
			state.State = "verified"
			state.Verified = true
		case "failed":
			state.State = "failed"
			state.FailureType = event.FailureType
			state.Action = event.Action
		}
	}
	return state
}

// CredentialState is the credential variable of one provider and whether the
// environment supplies a value for it. An empty provider id is the
// hand-configured legacy endpoint.
func CredentialState(getenv func(string) string, providerID string) (string, bool) {
	env, _, present := provider.CredentialValue(getenv, providerID)
	return env, present
}

// AuthState is how a provider's authorization is shown: present means the
// credential variable is set, no_credential never means unsupported.
func AuthState(present bool) string {
	if present {
		return "present"
	}
	return "no_credential"
}

// capabilityStateText renders the state without the table's timestamp. The
// model list and the previews both derive their wording from here.
func capabilityStateText(c CapabilityState) string {
	if c.ID == "" || !c.Declared {
		return "unsupported"
	}
	switch c.State {
	case "verified":
		return "verified"
	case "failed":
		return "failed:" + c.FailureType
	case "legacy":
		return "legacy,unverified"
	default:
		return "declared,unverified"
	}
}

// RenderCapability renders one capability's evidence as the sentence both
// adapters show, with the model test timestamp when there is one.
func RenderCapability(c CapabilityState) string {
	state := capabilityStateText(c)
	if (c.State == "verified" || c.State == "failed") && c.CheckedAt != "" {
		return state + "@" + shortTimestamp(c.CheckedAt)
	}
	return state
}

func shortTimestamp(value string) string {
	if len(value) >= 20 {
		return value[:19] + "Z"
	}
	return value
}
