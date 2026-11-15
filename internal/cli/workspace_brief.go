package cli

import (
	"encoding/json"

	"github.com/wxxb789/passquest/internal/studio"
)

// BriefStatus is `agent status` for the latest study brief. Found is false
// when no brief was ever started.
func (w *Workspace) BriefStatus() (studio.BriefStatus, error) {
	result := w.run("agent", "status")
	if result.code != 0 {
		if err := result.err(); isCode(err, "no_run") {
			return studio.BriefStatus{State: "none", Steps: []studio.BriefStep{}, Limits: agentLimits()}, nil
		}
		return studio.BriefStatus{}, result.err()
	}
	var doc agentStatusDoc
	if err := result.decode(&doc); err != nil {
		return studio.BriefStatus{}, err
	}
	status := briefStatusOf(doc.Run, doc.Status, doc.Steps, doc.Limits)
	status.LatestEvent = doc.LatestEvent
	status.Live = doc.Durable.HeartbeatFresh
	return status, nil
}

// ResumeBrief is `agent resume` for the latest unfinished brief. It blocks
// until the durable host returns; the desktop layer runs it off the window
// thread. A host failure is reported in the returned status, not as an error.
func (w *Workspace) ResumeBrief() (studio.BriefStatus, error) {
	if err := w.requireNode(w.hostAgent); err != nil {
		return studio.BriefStatus{}, err
	}
	result := w.run("agent", "resume")
	if result.code == 0 {
		var doc agentRunDoc
		if err := result.decode(&doc); err != nil {
			return studio.BriefStatus{}, err
		}
		status := briefStatusOf(doc.Run, doc.Status, doc.Steps, doc.Limits)
		if doc.Summary != nil {
			status.Answer = doc.Summary.Answer
		}
		return status, nil
	}
	var failure agentFailureDoc
	if json.Unmarshal(result.stderr, &failure) == nil && failure.Run != nil && failure.Status != nil {
		return briefStatusOf(*failure.Run, *failure.Status, failure.Steps, failure.Limits), nil
	}
	return studio.BriefStatus{}, result.err()
}

// CancelBrief is `agent cancel` for the latest unfinished brief.
func (w *Workspace) CancelBrief() (studio.BriefStatus, error) {
	result := w.run("agent", "cancel")
	var doc agentCancelDoc
	if err := result.decode(&doc); err != nil {
		return studio.BriefStatus{}, err
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

func isCode(err error, code string) bool {
	typed, ok := err.(*studio.Error)
	return ok && typed.Code == code
}
