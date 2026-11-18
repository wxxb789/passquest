// Package brief orchestrates the durable study brief: starting one run over
// the bounded Pi task, resuming or cancelling it, reading its live status and
// recording the steps the Node host reports. Results carry exactly the inputs
// the CLI's view builders and the window's studio mappers read; nothing here
// renders. The CLI's agent commands and the window's brief methods are the two
// adapters.
package brief

import (
	"context"

	"github.com/wxxb789/passquest/internal/agent"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
)

// Command selects what one Advance call does with the addressed run.
type Command uint8

const (
	// Start creates the run and drives one host process from the current
	// configuration.
	Start Command = iota + 1
	// Resume continues the addressed unfinished run beside its recorded model
	// and endpoint.
	Resume
	// Cancel stops the addressed run's host and records the terminal outcome.
	Cancel
)

// AdvanceInput is one command's request: the values its flag layer resolved.
type AdvanceInput struct {
	RunRef         string // positional [RUN]; empty addresses the newest run
	StoreOverride  string // --agent-store
	MaxSteps       int    // start: --max-steps
	TimeoutSeconds int    // start, resume: host deadline in seconds
	WaitSeconds    int    // cancel: seconds to wait for the host to stop
}

// Advanced is one Advance call's outcome: the inputs of the CLI's run, cancel
// and failure documents. State is terminal or paused for Start and Resume, and
// cancelling or cancelled for Cancel; a failed or needs_action state pairs with
// Failure. HeartbeatPID is the host a cancel outcome names. Reason is the run
// document's status reason when State is paused: the interruption reason, or
// "interrupted".
type Advanced struct {
	Run          store.AgentRun
	State        agent.State
	Failure      *agent.Failure
	Response     agent.HostResponse
	Steps        []store.AgentStep
	HeartbeatPID int
	Reason       string
}

// Advance runs one brief command over the caller's unit of work.
func Advance(ctx context.Context, s *study.Service, command Command, in AdvanceInput) (Advanced, error) {
	_ = ctx
	switch command {
	case Start:
		return start(s, in)
	case Resume:
		return resume(s, in)
	case Cancel:
		return cancel(s, in)
	}
	return Advanced{}, study.Refuse("invalid_input", "unknown brief command %d", command)
}

// Limits is the disclosure every agent document and the window's empty brief
// status carry: what the durable task contains, and what a step is not.
func Limits() []string {
	return []string{
		"Study steps are advisory model output, not verified answers, grades or exam facts.",
		"The durable task carries recorded attempt facts, derived skill state and authored lesson text only; no private source text, answer key or learner identity.",
		"One agent run at a time; a second start is refused until the current run completes or is cancelled.",
	}
}

// RunLimits is what one run document's limits are: the brief's own disclosure
// followed by the host's recorded limits. Both adapters compose it.
func RunLimits(host []string) []string {
	return append(Limits(), host...)
}

// ReasonAndAction is the status text one run shows: the reason is the state's
// name, the latest recorded event's name when there is one, or the failure's
// code; the action comes from the failure. The CLI's agent documents and the
// window's brief status both show it.
func ReasonAndAction(state agent.State, failure *agent.Failure, latestEvent string) (reason, action string) {
	reason = string(state)
	if latestEvent != "" {
		reason = latestEvent
	}
	if failure != nil {
		reason = failure.Code
		action = failure.Action
	}
	return reason, action
}

// StepAdvisory is what every recorded step is: advisory model output, never a
// verified answer or grade. Both step views carry it.
const StepAdvisory = true

// CancelOutcome is how one cancel state reads on the trail: the event the
// cancellation records and whether the host's stop is confirmed. Only a
// cancelled run is confirmed; anything else is still cancelling.
func CancelOutcome(state agent.State) (event string, confirmed bool) {
	if state == agent.StateCancelled {
		return store.AgentEventCancelled, true
	}
	return store.AgentEventCancelRequested, false
}

// AppState maps a recorded run event to the run state both adapters show.
func AppState(kind string) agent.State {
	switch kind {
	case store.AgentEventCompleted:
		return agent.StateCompleted
	case store.AgentEventCancelled:
		return agent.StateCancelled
	case store.AgentEventCancelRequested:
		return agent.StateCancelling
	case store.AgentEventFailed:
		return agent.StateFailed
	case store.AgentEventNeedsAction:
		return agent.StateNeedsAction
	case store.AgentEventInterrupted:
		return agent.StatePaused
	default:
		return agent.StatePaused
	}
}
