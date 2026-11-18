package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/wxxb789/passquest/internal/agent"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/studio"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/brief"
)

// BriefStatus is `agent status` for the latest study brief. Found is false
// when no brief was ever started.
func (w *Workspace) BriefStatus() (studio.BriefStatus, error) {
	var status studio.BriefStatus
	if err := w.codedCall(func(svc *study.Service) error {
		live, err := brief.Status(context.Background(), svc, "", "")
		if err != nil {
			// A database without a run is the "none" brief, not a failure;
			// the shimmed `agent status` refused with no_run.
			var refusal *study.Error
			if errors.As(err, &refusal) && refusal.Code == "no_run" {
				status = studio.BriefStatus{State: "none", Steps: []studio.BriefStep{}, Limits: brief.Limits()}
				return nil
			}
			return err
		}
		status = briefStatusOf(live.Run, live.State, live.Failure, live.LatestEvent, live.Steps, brief.Limits())
		status.LatestEvent = live.LatestEvent
		status.Live = live.Durable.HeartbeatFresh
		return nil
	}); err != nil {
		return studio.BriefStatus{}, err
	}
	return status, nil
}

// ResumeBrief is `agent resume` for the latest unfinished brief. It blocks
// until the durable host returns; the desktop layer runs it off the window
// thread. A host failure is reported in the returned status, not as an error.
func (w *Workspace) ResumeBrief() (studio.BriefStatus, error) {
	if err := w.requireNode(w.hostAgent); err != nil {
		return studio.BriefStatus{}, err
	}
	var advanced brief.Advanced
	if err := w.codedCall(func(svc *study.Service) error {
		var err error
		advanced, err = brief.Advance(context.Background(), svc, brief.Resume,
			brief.AdvanceInput{TimeoutSeconds: agent.DefaultTimeoutSeconds})
		return err
	}); err != nil {
		return studio.BriefStatus{}, err
	}
	if advanced.Failure != nil {
		return briefStatusOf(advanced.Run, advanced.State, advanced.Failure, "", advanced.Steps, brief.Limits()), nil
	}
	if advanced.State == agent.StatePaused {
		// A paused run was an exit code, not a document the window read: the
		// shimmed command wrote the run document to stdout and exited 4, and
		// its reader surfaced the bare exit code instead. Kept byte for byte.
		return studio.BriefStatus{}, &studio.Error{Code: "internal_error", Message: "command exited with 4"}
	}
	// The shimmed command's exit-0 stdout carried the brief's disclosure plus
	// the host's own limits, and no failure: the same composition.
	status := briefStatusOf(advanced.Run, advanced.State, nil, "", advanced.Steps,
		brief.RunLimits(advanced.Response.Limits))
	status.Answer = advanced.Response.Answer
	return status, nil
}

// CancelBrief is `agent cancel` for the latest unfinished brief.
func (w *Workspace) CancelBrief() (studio.BriefStatus, error) {
	var advanced brief.Advanced
	if err := w.codedCall(func(svc *study.Service) error {
		var err error
		advanced, err = brief.Advance(context.Background(), svc, brief.Cancel,
			brief.AdvanceInput{WaitSeconds: agent.DefaultCancelWaitSeconds})
		return err
	}); err != nil {
		return studio.BriefStatus{}, err
	}
	if advanced.State == agent.StateCancelling {
		// An unconfirmed cancellation was an exit code with the cancel
		// document on stderr, which held no error for the window's reader; it
		// surfaced the raw document. Kept byte for byte: this is the shape the
		// capture's hard-killed-host fixture pins the kill grace against.
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		_ = enc.Encode(windowCancelDocOf(advanced.Run, advanced.State, advanced.HeartbeatPID))
		return studio.BriefStatus{}, &studio.Error{Code: "internal_error", Message: strings.TrimSpace(buf.String())}
	}
	event, _ := brief.CancelOutcome(advanced.State)
	return briefStatusOf(advanced.Run, advanced.State, nil, event, nil, brief.Limits()), nil
}

// briefStatusOf maps a brief's stored run, state, failure and steps onto the
// window's status: the same fields and the same reason/action rules the CLI's
// agent documents carried.
func briefStatusOf(run store.AgentRun, state agent.State, failure *agent.Failure, latestEvent string, steps []store.AgentStep, limits []string) studio.BriefStatus {
	reason, action := brief.ReasonAndAction(state, failure, latestEvent)
	view := studio.BriefStatus{
		Found:    true,
		RunID:    run.RunID,
		Task:     run.TaskKind,
		State:    string(state),
		Reason:   reason,
		Action:   action,
		Provider: run.Provider,
		Model:    run.Model,
		MaxSteps: run.MaxSteps,
		Steps:    make([]studio.BriefStep, 0, len(steps)),
		Limits:   limits,
	}
	for _, step := range steps {
		view.Steps = append(view.Steps, studio.BriefStep{
			Index: step.Index, Kind: step.Kind, Title: step.Title, Body: step.Body, Advisory: brief.StepAdvisory,
		})
	}
	return view
}

// The cancel document below and its nested views reproduce, field for field
// and in the same order, the document the shimmed `agent cancel` wrote; the
// JSON field names are protocol.
type windowCancelDoc struct {
	SchemaVersion int                   `json:"schema_version"`
	State         string                `json:"state"`
	Run           windowAgentRunView    `json:"run"`
	Status        windowAgentStatusView `json:"status"`
	Cancelled     bool                  `json:"cancelled"`
	Confirmed     bool                  `json:"confirmed"`
	HeartbeatPID  int                   `json:"heartbeat_pid,omitempty"`
	Limits        []string              `json:"limits"`
}

type windowAgentRunView struct {
	RunID         string `json:"run_id"`
	Task          string `json:"task"`
	PromptVersion string `json:"prompt_version"`
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model"`
	Store         string `json:"store,omitempty"`
	MaxSteps      int    `json:"max_steps"`
	MaxTurns      int    `json:"max_turns"`
	CreatedAt     string `json:"created_at"`
}

type windowAgentStatusView struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
	Action string `json:"action,omitempty"`
}

// windowCancelDocSchemaVersion is the schema_version of the CLI `agent cancel`
// document this message reproduces: cli.ProtocolVersion, "the JSON protocol
// emitted by --json". It is deliberately not agent.ProtocolVersion — the
// durable-host IPC version, which may move independently. The CLI's own
// ProtocolVersion cannot be imported here (the desktop adapter never imports
// internal/cli), so this one reproduced document keeps the value as a literal
// with this note.
const windowCancelDocSchemaVersion = 1

// windowCancelDocOf renders one cancel outcome: the state decides the paired
// cancelled/confirmed flags and the status event, so no caller can emit a
// confirmed cancellation for a run that is still cancelling.
func windowCancelDocOf(run store.AgentRun, state agent.State, heartbeatPID int) windowCancelDoc {
	event, confirmed := brief.CancelOutcome(state)
	return windowCancelDoc{
		SchemaVersion: windowCancelDocSchemaVersion,
		State:         string(state),
		Run: windowAgentRunView{
			RunID:         run.RunID,
			Task:          run.TaskKind,
			PromptVersion: run.PromptVersion,
			Provider:      run.Provider,
			Model:         run.Model,
			Store:         run.StorePath,
			MaxSteps:      run.MaxSteps,
			MaxTurns:      run.MaxTurns,
			CreatedAt:     source.FormatTime(run.CreatedAt),
		},
		Status:       windowAgentStatusView{State: string(state), Reason: event},
		Cancelled:    confirmed,
		Confirmed:    confirmed,
		HeartbeatPID: heartbeatPID,
		Limits:       brief.Limits(),
	}
}
