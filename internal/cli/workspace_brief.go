package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"

	"github.com/wxxb789/passquest/internal/agent"
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
		status = briefStatusOf(agentRunViewOf(live.Run),
			agentStatusViewOf(live.State, live.Failure, live.LatestEvent),
			agentStepViews(live.Steps), brief.Limits())
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
		return briefStatusOf(agentRunViewOf(advanced.Run),
			agentStatusViewOf(advanced.State, advanced.Failure, ""),
			agentStepViews(advanced.Steps), brief.Limits()), nil
	}
	if advanced.State == agent.StatePaused {
		// A paused run was an exit code, not a document the window read: the
		// shimmed command wrote the run document to stdout and exited 4, and
		// its reader surfaced the bare exit code instead. Kept byte for byte.
		return studio.BriefStatus{}, &studio.Error{Code: "internal_error", Message: "command exited with 4"}
	}
	// The shimmed command's exit-0 stdout was the CLI's run document
	// (agentRunDocOf); its limits are the brief's disclosure plus the host's
	// own. Build that same document rather than repeat the composition.
	doc := agentRunDocOf(advanced.Run, advanced.State, nil, advanced.Response, advanced.Steps)
	status := briefStatusOf(doc.Run, doc.Status, doc.Steps, doc.Limits)
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
	doc := agentCancelDocOf(advanced.Run, advanced.State, advanced.HeartbeatPID)
	if advanced.State == agent.StateCancelling {
		// An unconfirmed cancellation was an exit code with the cancel
		// document on stderr, which held no error for the window's reader; it
		// surfaced the raw document. Kept byte for byte: this is the shape the
		// capture's hard-killed-host fixture pins the kill grace against.
		var buf bytes.Buffer
		_ = writeJSONTo(&buf, doc)
		return studio.BriefStatus{}, &studio.Error{Code: "internal_error", Message: strings.TrimSpace(buf.String())}
	}
	return briefStatusOf(doc.Run, doc.Status, nil, doc.Limits), nil
}

func briefStatusOf(run agentRunView, status agentStatusView, steps []agentStepView, limits []string) studio.BriefStatus {
	view := studio.BriefStatus{
		Found:    true,
		RunID:    run.RunID,
		Task:     run.Task,
		State:    status.State,
		Reason:   status.Reason,
		Action:   status.Action,
		Provider: run.Provider,
		Model:    run.Model,
		MaxSteps: run.MaxSteps,
		Steps:    make([]studio.BriefStep, 0, len(steps)),
		Limits:   limits,
	}
	for _, step := range steps {
		view.Steps = append(view.Steps, studio.BriefStep{
			Index: step.Index, Kind: step.Kind, Title: step.Title, Body: step.Body, Advisory: step.Advisory,
		})
	}
	return view
}
