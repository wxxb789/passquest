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
