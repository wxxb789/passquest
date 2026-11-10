package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/wxxb789/passquest/internal/agent"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
)

type agentRunView struct {
	RunID         string `json:"run_id"`
	Task          string `json:"task"`
	PromptVersion string `json:"prompt_version"`
	Model         string `json:"model"`
	Store         string `json:"store,omitempty"`
	MaxSteps      int    `json:"max_steps"`
	MaxTurns      int    `json:"max_turns"`
	CreatedAt     string `json:"created_at"`
}

type agentStatusView struct {
	State  string          `json:"state"`
	Reason string          `json:"reason,omitempty"`
	Action string          `json:"action,omitempty"`
	Error  *agent.ErrorDoc `json:"error,omitempty"`
}

type agentStepView struct {
	Index          int    `json:"index"`
	Kind           string `json:"kind"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	Advisory       bool   `json:"advisory"`
	VerifiedAnswer bool   `json:"verified_answer"`
	CreatedAt      string `json:"created_at"`
}

type agentCountsView struct {
	Steps      int `json:"steps"`
	Attempts   int `json:"attempts"`
	Committed  int `json:"committed"`
	Duplicates int `json:"duplicates"`
}

type agentEffectsView struct {
	Steps      int `json:"steps"`
	Attempts   int `json:"attempts"`
	Committed  int `json:"committed"`
	Duplicates int `json:"duplicates"`
	Reconciled int `json:"reconciled_events"`
}

type agentDurableView struct {
	Available      bool            `json:"available"`
	Reason         string          `json:"reason,omitempty"`
	ConversationID int64           `json:"conversation_id,omitempty"`
	HeartbeatPID   int             `json:"heartbeat_pid,omitempty"`
	HeartbeatFresh bool            `json:"heartbeat_fresh"`
	Snapshot       *agent.Snapshot `json:"snapshot,omitempty"`
}

type agentSummaryView struct {
	Answer      string          `json:"answer,omitempty"`
	AnswerChars int             `json:"answer_chars"`
	Turns       int             `json:"turns"`
	ToolCalls   int             `json:"tool_calls"`
	ToolErrors  int             `json:"tool_errors"`
	Usage       json.RawMessage `json:"usage,omitempty"`
}

type agentRunDoc struct {
	SchemaVersion int               `json:"schema_version"`
	State         string            `json:"state"`
	Run           agentRunView      `json:"run"`
	Status        agentStatusView   `json:"status"`
	Steps         []agentStepView   `json:"steps"`
	Summary       *agentSummaryView `json:"summary,omitempty"`
	Durable       agentDurableView  `json:"durable"`
	Limits        []string          `json:"limits"`
}

type agentStatusDoc struct {
	SchemaVersion int              `json:"schema_version"`
	State         string           `json:"state"`
	Run           agentRunView     `json:"run"`
	Status        agentStatusView  `json:"status"`
	Steps         []agentStepView  `json:"steps"`
	Counts        agentCountsView  `json:"counts"`
	Durable       agentDurableView `json:"durable"`
	LatestEvent   string           `json:"latest_event"`
	Events        int              `json:"events"`
	Limits        []string         `json:"limits"`
}

type agentCancelDoc struct {
	SchemaVersion int             `json:"schema_version"`
	State         string          `json:"state"`
	Run           agentRunView    `json:"run"`
	Status        agentStatusView `json:"status"`
	Cancelled     bool            `json:"cancelled"`
	Confirmed     bool            `json:"confirmed"`
	HeartbeatPID  int             `json:"heartbeat_pid,omitempty"`
	Limits        []string        `json:"limits"`
}

type agentRunSummaryView struct {
	RunID       string `json:"run_id"`
	Task        string `json:"task"`
	Model       string `json:"model"`
	MaxSteps    int    `json:"max_steps"`
	CreatedAt   string `json:"created_at"`
	State       string `json:"state"`
	LatestEvent string `json:"latest_event"`
	Steps       int    `json:"steps"`
	Attempts    int    `json:"attempts"`
}

type agentRunsDoc struct {
	SchemaVersion int                   `json:"schema_version"`
	State         string                `json:"state"`
	Runs          []agentRunSummaryView `json:"runs"`
	Limits        []string              `json:"limits"`
}

type agentEventView struct {
	ID     int64           `json:"id"`
	Kind   string          `json:"kind"`
	At     string          `json:"at"`
	Detail json.RawMessage `json:"detail"`
}

type agentShowDoc struct {
	SchemaVersion int              `json:"schema_version"`
	State         string           `json:"state"`
	Run           agentRunView     `json:"run"`
	Status        agentStatusView  `json:"status"`
	Counts        agentCountsView  `json:"counts"`
	Events        []agentEventView `json:"events"`
	Steps         []agentStepView  `json:"steps"`
	Effects       agentEffectsView `json:"effects"`
	Limits        []string         `json:"limits"`
}

type agentTranscriptDoc struct {
	SchemaVersion  int                     `json:"schema_version"`
	State          string                  `json:"state"`
	RunID          string                  `json:"run_id"`
	Store          string                  `json:"store"`
	Found          bool                    `json:"found"`
	ConversationID int64                   `json:"conversation_id,omitempty"`
	Entries        []agent.TranscriptEntry `json:"entries"`
	Truncated      bool                    `json:"truncated"`
	Limits         []string                `json:"limits"`
}

type agentFailureDoc struct {
	SchemaVersion int              `json:"schema_version"`
	Error         agent.ErrorDoc   `json:"error"`
	Run           *agentRunView    `json:"run,omitempty"`
	Status        *agentStatusView `json:"status,omitempty"`
	Steps         []agentStepView  `json:"steps,omitempty"`
	Limits        []string         `json:"limits,omitempty"`
}

type agentStepDoc struct {
	SchemaVersion int           `json:"schema_version"`
	State         string        `json:"state"`
	RunID         string        `json:"run_id"`
	Step          agentStepView `json:"step"`
	Duplicate     bool          `json:"duplicate"`
	AttemptKey    string        `json:"attempt_key"`
	Attempts      int           `json:"attempts"`
}

type agentRecordStepErrorDoc struct {
	SchemaVersion int            `json:"schema_version"`
	Error         agent.ErrorDoc `json:"error"`
}

func agentRunViewOf(run store.AgentRun) agentRunView {
	return agentRunView{
		RunID:         run.RunID,
		Task:          run.TaskKind,
		PromptVersion: run.PromptVersion,
		Model:         run.Model,
		Store:         run.StorePath,
		MaxSteps:      run.MaxSteps,
		MaxTurns:      run.MaxTurns,
		CreatedAt:     source.FormatTime(run.CreatedAt),
	}
}

func agentStatusViewOf(state agent.State, failure *agent.Failure, latestEvent string) agentStatusView {
	view := agentStatusView{State: string(state), Reason: string(state)}
	if latestEvent != "" {
		view.Reason = latestEvent
	}
	if failure != nil {
		view.Error = &agent.ErrorDoc{Code: failure.Code, Message: failure.Message}
		view.Action = failure.Action
		view.Reason = failure.Code
	}
	return view
}

func agentStepViews(steps []store.AgentStep) []agentStepView {
	out := make([]agentStepView, 0, len(steps))
	for _, step := range steps {
		out = append(out, agentStepView{
			Index: step.Index, Kind: step.Kind, Title: step.Title, Body: step.Body,
			Advisory: true, VerifiedAnswer: false, CreatedAt: source.FormatTime(step.CreatedAt),
		})
	}
	return out
}

func agentCountsOf(steps []store.AgentStep, attempts []store.AgentStepAttempt) agentCountsView {
	counts := agentCountsView{Steps: len(steps), Attempts: len(attempts)}
	for _, attempt := range attempts {
		switch attempt.Outcome {
		case "committed":
			counts.Committed++
		case "duplicate":
			counts.Duplicates++
		}
	}
	return counts
}

func agentEffectsOf(steps []store.AgentStep, attempts []store.AgentStepAttempt, events []store.AgentRunEvent) agentEffectsView {
	counts := agentCountsOf(steps, attempts)
	effects := agentEffectsView{
		Steps: counts.Steps, Attempts: counts.Attempts, Committed: counts.Committed, Duplicates: counts.Duplicates,
	}
	for _, event := range events {
		if event.Kind == store.AgentEventReconciled {
			effects.Reconciled++
		}
	}
	return effects
}

func agentEventViews(events []store.AgentRunEvent) []agentEventView {
	out := make([]agentEventView, 0, len(events))
	for _, event := range events {
		detail := json.RawMessage(event.DetailJSON)
		if len(detail) == 0 || !json.Valid(detail) {
			detail = json.RawMessage(`{}`)
		} else {
			var fields map[string]any
			if json.Unmarshal(detail, &fields) == nil {
				// The heartbeat token identifies a live host process; the
				// evidence view does not need it and should not publish it.
				delete(fields, "host_token")
				if encoded, err := json.Marshal(fields); err == nil {
					detail = encoded
				}
			}
		}
		out = append(out, agentEventView{ID: event.ID, Kind: event.Kind, At: source.FormatTime(event.At), Detail: detail})
	}
	return out
}

func agentRunSummaryViews(summaries []store.AgentRunSummary) []agentRunSummaryView {
	out := make([]agentRunSummaryView, 0, len(summaries))
	for _, summary := range summaries {
		state := string(agentAppState(summary.LatestEvent))
		out = append(out, agentRunSummaryView{
			RunID: summary.RunID, Task: summary.TaskKind, Model: summary.Model, MaxSteps: summary.MaxSteps,
			CreatedAt: source.FormatTime(summary.CreatedAt), State: state, LatestEvent: summary.LatestEvent,
			Steps: summary.Steps, Attempts: summary.Attempts,
		})
	}
	return out
}

func agentLimits() []string {
	return []string{
		"Study steps are advisory model output, not verified answers, grades or exam facts.",
		"The durable task carries recorded attempt facts, derived skill state and authored lesson text only; no private source text, answer key or learner identity.",
		"One agent run at a time; a second start is refused until the current run completes or is cancelled.",
	}
}

func agentRunDocOf(run store.AgentRun, state agent.State, failure *agent.Failure, response agent.HostResponse, steps []store.AgentStep) agentRunDoc {
	doc := agentRunDoc{
		SchemaVersion: ProtocolVersion,
		State:         string(state),
		Run:           agentRunViewOf(run),
		Status:        agentStatusViewOf(state, failure, ""),
		Steps:         agentStepViews(steps),
		Durable: agentDurableView{
			Available:      response.ConversationID > 0,
			ConversationID: response.ConversationID,
			Snapshot:       response.Snapshot,
		},
		Limits: append(agentLimits(), response.Limits...),
	}
	if response.Answer != "" || response.AnswerChars > 0 || response.Turns > 0 || response.ToolCalls > 0 {
		doc.Summary = &agentSummaryView{
			Answer: response.Answer, AnswerChars: response.AnswerChars, Turns: response.Turns,
			ToolCalls: response.ToolCalls, ToolErrors: response.ToolErrors, Usage: response.Usage,
		}
	}
	return doc
}

// agentCancelDocOf renders one cancel outcome. The state decides the paired
// cancelled/confirmed flags and the status event, so no caller can emit a
// confirmed cancellation for a run that is still cancelling.
func agentCancelDocOf(run store.AgentRun, state agent.State, heartbeatPID int) agentCancelDoc {
	event := store.AgentEventCancelRequested
	confirmed := false
	if state == agent.StateCancelled {
		event = store.AgentEventCancelled
		confirmed = true
	}
	return agentCancelDoc{
		SchemaVersion: ProtocolVersion,
		State:         string(state),
		Run:           agentRunViewOf(run),
		Status:        agentStatusViewOf(state, nil, event),
		Cancelled:     confirmed,
		Confirmed:     confirmed,
		HeartbeatPID:  heartbeatPID,
		Limits:        agentLimits(),
	}
}

func writeAgentRun(w io.Writer, doc agentRunDoc) {
	fmt.Fprintf(w, "Pass Quest — durable study brief %s\n", doc.Run.RunID)
	fmt.Fprintf(w, "State: %s\n", doc.State)
	fmt.Fprintf(w, "Model: %s | max steps: %d | max turns: %d\n", doc.Run.Model, doc.Run.MaxSteps, doc.Run.MaxTurns)
	if doc.Durable.Available {
		fmt.Fprintf(w, "Durable runtime: conversation %d in the Pi store\n", doc.Durable.ConversationID)
	}
	if len(doc.Steps) > 0 {
		fmt.Fprintln(w, "Steps (advisory, not verified answers):")
		for _, step := range doc.Steps {
			fmt.Fprintf(w, "  %d. %s — %s\n", step.Index, step.Kind, step.Title)
			if step.Body != "" {
				fmt.Fprintf(w, "     %s\n", step.Body)
			}
		}
	} else {
		fmt.Fprintln(w, "Steps: none committed")
	}
	if doc.Summary != nil && doc.Summary.Answer != "" {
		fmt.Fprintf(w, "Summary (advisory): %s\n", doc.Summary.Answer)
	}
	if doc.Status.Action != "" {
		fmt.Fprintf(w, "Action: %s\n", doc.Status.Action)
	}
	switch agent.State(doc.State) {
	case agent.StatePaused:
		fmt.Fprintf(w, "Next: passquest agent resume %s\n", doc.Run.RunID)
	case agent.StateCompleted:
		fmt.Fprintf(w, "Inspect: passquest agent show %s | passquest agent transcript %s\n", doc.Run.RunID, doc.Run.RunID)
	case agent.StateCancelled:
		fmt.Fprintln(w, "Next: passquest agent start")
	}
	writeAgentLimits(w, doc.Limits)
}

func writeAgentStatus(w io.Writer, doc agentStatusDoc) {
	fmt.Fprintf(w, "Pass Quest — agent run %s\n", doc.Run.RunID)
	fmt.Fprintf(w, "State: %s (latest event: %s)\n", doc.State, doc.LatestEvent)
	if doc.Status.Error != nil {
		fmt.Fprintf(w, "Failure: %s — %s\n", doc.Status.Error.Code, doc.Status.Error.Message)
	}
	if doc.Status.Action != "" {
		fmt.Fprintf(w, "Action: %s\n", doc.Status.Action)
	}
	fmt.Fprintf(w, "Model: %s | steps: %d | effect attempts: %d (%d duplicate)\n",
		doc.Run.Model, doc.Counts.Steps, doc.Counts.Attempts, doc.Counts.Duplicates)
	if doc.Durable.HeartbeatFresh {
		fmt.Fprintf(w, "Host: running as pid %d\n", doc.Durable.HeartbeatPID)
	} else {
		fmt.Fprintln(w, "Host: no fresh heartbeat")
	}
	if doc.Durable.Available {
		switch {
		case doc.Durable.Snapshot != nil && doc.Durable.Snapshot.Found:
			fmt.Fprintf(w, "Durable runtime: conversation %d, live run %t, pending tasks %d, unsettled submissions %d\n",
				doc.Durable.ConversationID, doc.Durable.Snapshot.LiveRun,
				doc.Durable.Snapshot.PendingTasks, doc.Durable.Snapshot.UnsettledSubmissions)
		case doc.Durable.Reason == "store_missing":
			fmt.Fprintln(w, "Durable runtime: store missing; the application event trail is the only evidence")
		default:
			fmt.Fprintln(w, "Durable runtime: store available, run not found")
		}
	} else {
		fmt.Fprintf(w, "Durable runtime: unavailable (%s)\n", doc.Durable.Reason)
	}
	if len(doc.Steps) > 0 {
		fmt.Fprintln(w, "Steps:")
		for _, step := range doc.Steps {
			fmt.Fprintf(w, "  %d. %s — %s\n", step.Index, step.Kind, step.Title)
		}
	}
	switch agent.State(doc.State) {
	case agent.StatePaused:
		fmt.Fprintf(w, "Next: passquest agent resume %s\n", doc.Run.RunID)
	case agent.StateNeedsAction, agent.StateFailed:
		fmt.Fprintf(w, "Inspect: passquest agent show %s\n", doc.Run.RunID)
		if agent.State(doc.State) == agent.StateNeedsAction {
			fmt.Fprintf(w, "Next: fix the cause, then passquest agent resume %s\n", doc.Run.RunID)
		}
	case agent.StateRunning, agent.StateCancelling:
		fmt.Fprintf(w, "Next: passquest agent cancel %s\n", doc.Run.RunID)
	case agent.StateCompleted:
		fmt.Fprintf(w, "Inspect: passquest agent show %s | passquest agent transcript %s\n", doc.Run.RunID, doc.Run.RunID)
	case agent.StateCancelled:
		fmt.Fprintln(w, "Next: passquest agent start")
	}
	writeAgentLimits(w, doc.Limits)
}

func writeAgentCancel(w io.Writer, doc agentCancelDoc) {
	fmt.Fprintf(w, "Pass Quest — cancel agent run %s\n", doc.Run.RunID)
	if doc.Confirmed {
		fmt.Fprintln(w, "State: cancelled; the host heartbeat stopped.")
	} else {
		fmt.Fprintln(w, "State: cancellation requested, but the host did not confirm it stopped.")
	}
	if doc.HeartbeatPID > 0 {
		fmt.Fprintf(w, "Host pid: %d\n", doc.HeartbeatPID)
	}
	fmt.Fprintln(w, "Resume refuses a cancelled run; start a new run when ready.")
	writeAgentLimits(w, doc.Limits)
}

func writeAgentRuns(w io.Writer, doc agentRunsDoc) {
	fmt.Fprintln(w, "Pass Quest — agent runs")
	if len(doc.Runs) == 0 {
		fmt.Fprintln(w, "No agent runs recorded.")
		return
	}
	fmt.Fprintln(w, "RUN                              STATE         STEPS ATTEMPTS MODEL")
	for _, run := range doc.Runs {
		fmt.Fprintf(w, "%-32s %-13s %5d %8d %s\n", run.RunID, run.State, run.Steps, run.Attempts, run.Model)
	}
	fmt.Fprintln(w, "Use passquest agent status RUN for live state and passquest agent show RUN for the evidence trail.")
	writeAgentLimits(w, doc.Limits)
}

func writeAgentShow(w io.Writer, doc agentShowDoc) {
	fmt.Fprintf(w, "Pass Quest — agent run %s\n", doc.Run.RunID)
	fmt.Fprintf(w, "State: %s | latest event: %s\n", doc.Status.State, doc.Status.Reason)
	fmt.Fprintf(w, "Model: %s | created %s\n", doc.Run.Model, doc.Run.CreatedAt)
	fmt.Fprintf(w, "Effects: %d steps, %d attempts, %d committed, %d duplicate, %d reconciliation event(s)\n",
		doc.Effects.Steps, doc.Effects.Attempts, doc.Effects.Committed, doc.Effects.Duplicates, doc.Effects.Reconciled)
	if len(doc.Steps) > 0 {
		fmt.Fprintln(w, "Steps (advisory, not verified answers):")
		for _, step := range doc.Steps {
			fmt.Fprintf(w, "  %d. %s — %s\n", step.Index, step.Kind, step.Title)
			fmt.Fprintf(w, "     %s\n", step.Body)
		}
	}
	if len(doc.Events) > 0 {
		fmt.Fprintln(w, "Events:")
		for _, event := range doc.Events {
			fmt.Fprintf(w, "  %s %s %s\n", event.At, event.Kind, compactJSON(event.Detail))
		}
	}
	writeAgentLimits(w, doc.Limits)
}

func writeAgentTranscript(w io.Writer, doc agentTranscriptDoc) {
	fmt.Fprintf(w, "Pass Quest — durable agent history for %s (advisory)\n", doc.RunID)
	fmt.Fprintf(w, "Store: %s | conversation: %d\n", doc.Store, doc.ConversationID)
	if len(doc.Entries) == 0 {
		fmt.Fprintln(w, "No entries.")
		return
	}
	for _, entry := range doc.Entries {
		label := entry.Kind
		if entry.Role != "" {
			label += "/" + entry.Role
		}
		if entry.ToolName != "" {
			label += " " + entry.ToolName
		}
		if entry.IsError {
			label += " (error)"
		}
		if entry.At != "" {
			fmt.Fprintf(w, "\n[%s] %s\n", label, entry.At)
		} else {
			fmt.Fprintf(w, "\n[%s]\n", label)
		}
		if len(entry.Data) > 0 {
			fmt.Fprintf(w, "  data: %s\n", compactJSON(entry.Data))
		}
		if entry.Text != "" {
			fmt.Fprintf(w, "  %s\n", strings.ReplaceAll(entry.Text, "\n", "\n  "))
		}
	}
	if doc.Truncated {
		fmt.Fprintln(w, "Transcript truncated at the requested entry limit.")
	}
	writeAgentLimits(w, doc.Limits)
}

func writeAgentFailureHuman(w io.Writer, doc agentFailureDoc) {
	fmt.Fprintf(w, "passquest: %s: %s\n", doc.Error.Code, doc.Error.Message)
	if doc.Run != nil {
		state := ""
		action := ""
		if doc.Status != nil {
			state = doc.Status.State
			action = doc.Status.Action
		}
		fmt.Fprintf(w, "Run: %s (state %s)\n", doc.Run.RunID, state)
		fmt.Fprintf(w, "Inspect: passquest agent show %s\n", doc.Run.RunID)
		if action == "resume" || state == string(agent.StateNeedsAction) {
			fmt.Fprintf(w, "Resume: passquest agent resume %s\n", doc.Run.RunID)
		}
	}
	if len(doc.Steps) > 0 {
		fmt.Fprintf(w, "Committed steps remain: %d\n", len(doc.Steps))
	}
}

func writeAgentLimits(w io.Writer, limits []string) {
	if len(limits) == 0 {
		return
	}
	fmt.Fprintln(w, "Limits:")
	for _, limit := range limits {
		fmt.Fprintf(w, "  - %s\n", limit)
	}
}

func compactJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return string(raw)
	}
	return string(encoded)
}
