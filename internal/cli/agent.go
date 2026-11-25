package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/agent"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/brief"
)

func runAgent(args []string, opt Options) int {
	if len(args) == 0 {
		return usageError(opt, jsonFlagRequested(args), "invalid_input",
			"agent requires start, status, resume, cancel, runs, show, transcript or record-step")
	}
	switch args[0] {
	case "start":
		return runAgentStart(args[1:], opt)
	case "status":
		return runAgentStatus(args[1:], opt)
	case "resume":
		return runAgentResume(args[1:], opt)
	case "cancel":
		return runAgentCancel(args[1:], opt)
	case "runs":
		return runAgentRuns(args[1:], opt)
	case "show":
		return runAgentShow(args[1:], opt)
	case "transcript":
		return runAgentTranscript(args[1:], opt)
	case "record-step":
		return runAgentRecordStep(args[1:], opt)
	default:
		return usageError(opt, jsonFlagRequested(args), "invalid_input", fmt.Sprintf("unknown agent command %q", args[0]))
	}
}

type agentCommonFlags struct {
	db     *string
	store  *string
	json   *bool
	fs     *flag.FlagSet
	opt    Options
	parsed bool
}

func newAgentFlags(name string, opt Options, wantJSON bool, withStore bool) agentCommonFlags {
	fs := newFlagSet(name, opt, wantJSON)
	common := agentCommonFlags{opt: opt, fs: fs}
	common.db = fs.String("db", "", "SQLite application database path")
	if withStore {
		common.store = fs.String("agent-store", "", "Pi durable-agent SQLite path")
	}
	common.json = fs.Bool("json", false, "emit JSON instead of human-readable text")
	return common
}

func parseAgentPositionals(fs *flag.FlagSet, args []string, opt Options, wantJSON bool, min, max int, usage string) int {
	if err := fs.Parse(args); err != nil {
		if wantJSON {
			return usageError(opt, true, "invalid_flags", err.Error())
		}
		return 2
	}
	if fs.NArg() < min || fs.NArg() > max {
		return usageError(opt, wantJSON, "invalid_input", usage)
	}
	return 0
}

func runAgentStart(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{
		"task": true, "max-steps": true, "timeout-seconds": true, "db": true, "agent-store": true,
	})
	flags := newAgentFlags("agent start", opt, wantJSON, true)
	taskFlag := flags.fs.String("task", agent.TaskStudyBrief, "bounded task kind; only study-brief")
	stepsFlag := flags.fs.Int("max-steps", agent.DefaultSteps, "maximum recorded steps (1-6)")
	timeoutFlag := flags.fs.Int("timeout-seconds", agent.DefaultTimeoutSeconds, "host deadline in seconds (1-900)")
	if code := parseAgentPositionals(flags.fs, args, opt, wantJSON, 0, 0, "agent start takes no positional arguments"); code != 0 {
		return code
	}
	if *taskFlag != agent.TaskStudyBrief {
		return usageError(opt, *flags.json, "invalid_input", "only --task study-brief is supported")
	}
	if *stepsFlag < agent.MinSteps || *stepsFlag > agent.MaxSteps {
		return usageError(opt, *flags.json, "invalid_input",
			fmt.Sprintf("--max-steps %d is outside %d..%d", *stepsFlag, agent.MinSteps, agent.MaxSteps))
	}
	if *timeoutFlag < agent.MinTimeoutSeconds || *timeoutFlag > agent.MaxTimeoutSeconds {
		return usageError(opt, *flags.json, "invalid_input",
			fmt.Sprintf("--timeout-seconds %d is outside %d..%d", *timeoutFlag, agent.MinTimeoutSeconds, agent.MaxTimeoutSeconds))
	}
	ports, err := opt.ports(*flags.db)
	if err != nil {
		return fail(opt, *flags.json, err)
	}
	svc, err := study.Open(ports)
	if err != nil {
		return internalError(opt, *flags.json, err)
	}
	defer svc.Close()
	advanced, err := brief.Advance(context.Background(), svc, brief.Start, brief.AdvanceInput{
		MaxSteps: *stepsFlag, TimeoutSeconds: *timeoutFlag, StoreOverride: *flags.store,
	})
	if err != nil {
		return agentAdvanceError(opt, *flags.json, err)
	}
	return writeAgentRunOutcome(opt, *flags.json, advanced)
}

func runAgentResume(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"timeout-seconds": true, "db": true, "agent-store": true})
	flags := newAgentFlags("agent resume", opt, wantJSON, true)
	timeoutFlag := flags.fs.Int("timeout-seconds", agent.DefaultTimeoutSeconds, "host deadline in seconds (1-900)")
	if code := parseAgentPositionals(flags.fs, args, opt, wantJSON, 0, 1, "agent resume [RUN]"); code != 0 {
		return code
	}
	if *timeoutFlag < agent.MinTimeoutSeconds || *timeoutFlag > agent.MaxTimeoutSeconds {
		return usageError(opt, *flags.json, "invalid_input",
			fmt.Sprintf("--timeout-seconds %d is outside %d..%d", *timeoutFlag, agent.MinTimeoutSeconds, agent.MaxTimeoutSeconds))
	}
	ports, err := opt.ports(*flags.db)
	if err != nil {
		return fail(opt, *flags.json, err)
	}
	svc, err := study.Open(ports)
	if err != nil {
		return internalError(opt, *flags.json, err)
	}
	defer svc.Close()
	advanced, err := brief.Advance(context.Background(), svc, brief.Resume, brief.AdvanceInput{
		RunRef: flags.fs.Arg(0), TimeoutSeconds: *timeoutFlag, StoreOverride: *flags.store,
	})
	if err != nil {
		return agentAdvanceError(opt, *flags.json, err)
	}
	return writeAgentRunOutcome(opt, *flags.json, advanced)
}

func runAgentStatus(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"db": true, "agent-store": true})
	flags := newAgentFlags("agent status", opt, wantJSON, true)
	if code := parseAgentPositionals(flags.fs, args, opt, wantJSON, 0, 1, "agent status [RUN]"); code != 0 {
		return code
	}
	ports, err := opt.ports(*flags.db)
	if err != nil {
		return fail(opt, *flags.json, err)
	}
	svc, err := study.Open(ports)
	if err != nil {
		return internalError(opt, *flags.json, err)
	}
	defer svc.Close()
	live, err := brief.Status(context.Background(), svc, flags.fs.Arg(0), *flags.store)
	if err != nil {
		return fail(opt, *flags.json, err)
	}
	doc := agentStatusDoc{
		SchemaVersion: ProtocolVersion,
		State:         string(live.State),
		Run:           agentRunViewOf(live.Run),
		Status:        agentStatusViewOf(live.State, live.Failure, live.LatestEvent),
		Steps:         agentStepViews(live.Steps),
		Counts:        agentCountsOf(live.Steps, live.Attempts),
		Durable: agentDurableView{
			Available:      live.Durable.Available,
			Reason:         live.Durable.Reason,
			ConversationID: live.Durable.ConversationID,
			HeartbeatPID:   live.Durable.HeartbeatPID,
			HeartbeatFresh: live.Durable.HeartbeatFresh,
			Snapshot:       live.Durable.Snapshot,
		},
		LatestEvent: live.LatestEvent,
		Events:      live.Events,
		Limits:      brief.Limits(),
	}
	if *flags.json {
		return writeJSON(opt, doc)
	}
	writeAgentStatus(opt.Stdout, doc)
	return 0
}

func runAgentCancel(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"wait-seconds": true, "db": true, "agent-store": true})
	flags := newAgentFlags("agent cancel", opt, wantJSON, true)
	waitFlag := flags.fs.Int("wait-seconds", agent.DefaultCancelWaitSeconds, "seconds to wait for the host to stop (1-60)")
	if code := parseAgentPositionals(flags.fs, args, opt, wantJSON, 0, 1, "agent cancel [RUN]"); code != 0 {
		return code
	}
	if *waitFlag < agent.MinCancelWaitSeconds || *waitFlag > agent.MaxCancelWaitSeconds {
		return usageError(opt, *flags.json, "invalid_input",
			fmt.Sprintf("--wait-seconds %d is outside %d..%d", *waitFlag, agent.MinCancelWaitSeconds, agent.MaxCancelWaitSeconds))
	}
	ports, err := opt.ports(*flags.db)
	if err != nil {
		return fail(opt, *flags.json, err)
	}
	svc, err := study.Open(ports)
	if err != nil {
		return internalError(opt, *flags.json, err)
	}
	defer svc.Close()
	advanced, err := brief.Advance(context.Background(), svc, brief.Cancel, brief.AdvanceInput{
		RunRef: flags.fs.Arg(0), StoreOverride: *flags.store, WaitSeconds: *waitFlag,
	})
	if err != nil {
		return agentAdvanceError(opt, *flags.json, err)
	}
	return writeAgentCancelOutcome(opt, *flags.json, advanced)
}

// agentAdvanceError maps a brief.Advance error the way the agent commands
// mapped theirs: a refusal is the CLI's usage error, the brief packet's
// learner-fixable errors keep the source table, and anything else is an
// internal error.
func agentAdvanceError(opt Options, jsonMode bool, err error) int {
	var e *study.Error
	if errors.As(err, &e) {
		return usageError(opt, jsonMode, e.Code, e.Message)
	}
	return sourceError(opt, jsonMode, err)
}

// writeAgentRunOutcome renders one start or resume result and returns its exit
// code: a recorded failure is the stderr failure document (3), a paused run
// the stdout run document (4), and a completed or cancelled run the stdout run
// document (0).
func writeAgentRunOutcome(opt Options, jsonMode bool, advanced brief.Advanced) int {
	if advanced.Failure != nil {
		return writeAgentFailure(opt, jsonMode, advanced.Run, advanced.State, *advanced.Failure, advanced.Steps)
	}
	doc := agentRunDocOf(advanced.Run, advanced.State, nil, advanced.Response, advanced.Steps)
	if advanced.State == agent.StatePaused {
		doc.Status.Reason = advanced.Reason
		if jsonMode {
			if err := writeJSONTo(opt.Stdout, doc); err != nil {
				return internalError(opt, jsonMode, err)
			}
			return 4
		}
		writeAgentRun(opt.Stdout, doc)
		return 4
	}
	if jsonMode {
		return writeJSON(opt, doc)
	}
	writeAgentRun(opt.Stdout, doc)
	return 0
}

// writeAgentCancelOutcome renders one cancel result and returns its exit code:
// a cancellation the host did not confirm goes to stderr with exit 1, a
// confirmed (or already terminal) cancellation to stdout with exit 0.
func writeAgentCancelOutcome(opt Options, jsonMode bool, advanced brief.Advanced) int {
	doc := agentCancelDocOf(advanced.Run, advanced.State, advanced.HeartbeatPID)
	if advanced.State == agent.StateCancelling {
		if jsonMode {
			_ = writeJSONTo(opt.Stderr, doc)
		} else {
			writeAgentCancel(opt.Stderr, doc)
		}
		return 1
	}
	if jsonMode {
		return writeJSON(opt, doc)
	}
	writeAgentCancel(opt.Stdout, doc)
	return 0
}

func runAgentRuns(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"db": true})
	fs := newFlagSet("agent runs", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite application database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseAgentPositionals(fs, args, opt, wantJSON, 0, 0, "agent runs takes no positional arguments"); code != 0 {
		return code
	}
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	defer st.Close()
	summaries, err := st.AgentRuns()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	doc := agentRunsDoc{SchemaVersion: ProtocolVersion, State: "runs", Runs: agentRunSummaryViews(summaries), Limits: brief.Limits()}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeAgentRuns(opt.Stdout, doc)
	return 0
}

func runAgentShow(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"db": true})
	fs := newFlagSet("agent show", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite application database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseAgentPositionals(fs, args, opt, wantJSON, 0, 1, "agent show [RUN]"); code != 0 {
		return code
	}
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	defer st.Close()
	run, latest, err := brief.Resolve(st, fs.Arg(0), false)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	events, err := st.AgentRunEvents(run.RunID)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	steps, err := st.AgentSteps(run.RunID)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	attempts, err := st.AgentStepAttempts(run.RunID)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	// show is a Go-only view of the recorded trail. Its state is the trail
	// state, which cannot see the heartbeat or the Pi snapshot; agent status is
	// the live-state view.
	doc := agentShowDoc{
		SchemaVersion: ProtocolVersion,
		State:         "run",
		Run:           agentRunViewOf(run),
		TrailState:    string(brief.AppState(latest.Kind)),
		LatestEvent:   latest.Kind,
		Counts:        agentCountsOf(steps, attempts),
		Events:        agentEventViews(events),
		Steps:         agentStepViews(steps),
		Effects:       agentEffectsOf(steps, attempts, events),
		Limits:        brief.Limits(),
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeAgentShow(opt.Stdout, doc)
	return 0
}

func runAgentTranscript(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"db": true, "agent-store": true, "limit": true})
	fs := newFlagSet("agent transcript", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite application database path")
	storeFlag := fs.String("agent-store", "", "Pi durable-agent SQLite path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	limitFlag := fs.Int("limit", 100, "maximum transcript entries (1-200)")
	if code := parseAgentPositionals(fs, args, opt, wantJSON, 1, 1, "agent transcript RUN"); code != 0 {
		return code
	}
	runID, err := agent.NormalizeRunID(fs.Arg(0))
	if err != nil {
		return usageError(opt, *jsonFlag, "invalid_input", err.Error())
	}
	if *limitFlag < 1 || *limitFlag > 200 {
		return usageError(opt, *jsonFlag, "invalid_input", "--limit must be 1-200")
	}
	resolved, err := opt.resolveDatabase(*dbFlag)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	storePath, err := agent.StorePath(*storeFlag, resolved.Path, opt.Getenv)
	if err != nil {
		return usageError(opt, *jsonFlag, "invalid_input", err.Error())
	}
	if _, err := os.Stat(storePath); err != nil {
		return usageError(opt, *jsonFlag, "store_missing",
			fmt.Sprintf("durable store %s does not exist; pass --agent-store with the saved Pi SQLite file", storePath))
	}
	outcome, runErr := brief.RunHost(opt.AgentRunner, agent.HostRequest{
		ProtocolVersion: agent.ProtocolVersion,
		Mode:            agent.ModeTranscript,
		RunID:           runID,
		StorePath:       storePath,
		CredentialEnv:   agent.CredentialEnv,
		TranscriptLimit: *limitFlag,
	}, 30*time.Second)
	if runErr != nil {
		var unavailable *agent.UnavailableError
		if errors.As(runErr, &unavailable) {
			return agentCommandError(opt, *jsonFlag, agent.CodeAgentUnavailable, runErr.Error())
		}
		return agentCommandError(opt, *jsonFlag, agent.CodeHostProtocol, runErr.Error())
	}
	if outcome.Response.State != "transcript" {
		return agentCommandError(opt, *jsonFlag, agent.CodeHostProtocol, "durable host returned an unexpected document")
	}
	if !outcome.Response.Found {
		return usageError(opt, *jsonFlag, "unknown_run", fmt.Sprintf("run %s is not in durable store %s", runID, storePath))
	}
	doc := agentTranscriptDoc{
		SchemaVersion:  ProtocolVersion,
		State:          "transcript",
		RunID:          runID,
		Store:          storePath,
		Found:          true,
		ConversationID: outcome.Response.ConversationID,
		Entries:        outcome.Response.Entries,
		Truncated:      outcome.Response.Truncated,
		Limits:         append(outcome.Response.Limits, "Durable agent history is advisory model output; it is not a grade, answer key or export of private source text."),
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeAgentTranscript(opt.Stdout, doc)
	return 0
}

func runAgentRecordStep(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"db": true})
	fs := newFlagSet("agent record-step", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite application database path")
	jsonFlag := fs.Bool("json", false, "emit a JSON result")
	if code := parseAgentPositionals(fs, args, opt, wantJSON, 0, 0, "agent record-step takes no positional arguments"); code != 0 {
		return code
	}
	var request struct {
		RunID      string `json:"run_id"`
		AttemptKey string `json:"attempt_key"`
		Index      int    `json:"index"`
		Kind       string `json:"kind"`
		Title      string `json:"title"`
		Body       string `json:"body"`
	}
	limited := io.LimitReader(opt.Stdin, 64<<10)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return agentRecordStepError(opt, *jsonFlag, "invalid_step", fmt.Sprintf("read step: %v", err))
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		return agentRecordStepError(opt, *jsonFlag, "invalid_step", fmt.Sprintf("step is not JSON: %v", err))
	}
	ports, err := opt.ports(*dbFlag)
	if err != nil {
		return agentRecordStepError(opt, *jsonFlag, "config_dir_unavailable", err.Error())
	}
	svc, err := study.Open(ports)
	if err != nil {
		return agentRecordStepError(opt, *jsonFlag, "internal_error", err.Error())
	}
	defer svc.Close()
	recorded, err := brief.Record(context.Background(), svc, brief.StepInput{
		RunID: request.RunID, AttemptKey: request.AttemptKey, Index: request.Index,
		Kind: request.Kind, Title: request.Title, Body: request.Body,
	})
	if err != nil {
		var e *study.Error
		if errors.As(err, &e) {
			return agentRecordStepError(opt, *jsonFlag, e.Code, e.Message)
		}
		return agentRecordStepError(opt, *jsonFlag, "internal_error", err.Error())
	}
	doc := agentStepDoc{
		SchemaVersion: ProtocolVersion,
		State:         "step",
		RunID:         recorded.Step.RunID,
		Step: agentStepView{
			Index: recorded.Step.Index, Kind: recorded.Step.Kind, Title: recorded.Step.Title, Body: recorded.Step.Body,
			Advisory: true, VerifiedAnswer: false, CreatedAt: source.FormatTime(recorded.Step.CreatedAt),
		},
		Duplicate:  recorded.Duplicate,
		AttemptKey: recorded.AttemptKey,
		Attempts:   recorded.Attempts,
	}
	return writeJSON(opt, doc)
}

func writeAgentFailure(opt Options, jsonMode bool, run store.AgentRun, state agent.State, failure agent.Failure, steps []store.AgentStep) int {
	status := agentStatusViewOf(state, &failure, "")
	runView := agentRunViewOf(run)
	doc := agentFailureDoc{
		SchemaVersion: ProtocolVersion,
		Error:         agent.ErrorDoc{Code: failure.Code, Message: failure.Message},
		Run:           &runView,
		Status:        &status,
		Steps:         agentStepViews(steps),
		Limits:        brief.Limits(),
	}
	if jsonMode {
		_ = writeJSONTo(opt.Stderr, doc)
	} else {
		writeAgentFailureHuman(opt.Stderr, doc)
	}
	return 3
}

func agentCommandError(opt Options, jsonMode bool, code, message string) int {
	doc := agentFailureDoc{
		SchemaVersion: ProtocolVersion,
		Error:         agent.ErrorDoc{Code: code, Message: redactAgent(opt, message)},
		Limits:        brief.Limits(),
	}
	if jsonMode {
		_ = writeJSONTo(opt.Stderr, doc)
	} else {
		fmt.Fprintf(opt.Stderr, "passquest: %s\n", redactAgent(opt, message))
	}
	return 3
}

func agentRecordStepError(opt Options, jsonMode bool, code, message string) int {
	doc := agentRecordStepErrorDoc{SchemaVersion: ProtocolVersion, Error: agent.ErrorDoc{Code: code, Message: message}}
	_ = writeJSONTo(opt.Stdout, doc)
	if !jsonMode {
		fmt.Fprintf(opt.Stderr, "passquest: %s\n", message)
	}
	if code == "invalid_step" || code == "unknown_run" || code == "run_cancelled" || code == "run_completed" {
		return 2
	}
	return 1
}

func redactAgent(opt Options, value string) string {
	key := strings.TrimSpace(opt.Getenv(agent.CredentialEnv))
	if key == "" {
		return value
	}
	return strings.ReplaceAll(value, key, "[redacted]")
}

// agentKillGrace is how long cancel waits after a hard kill: one heartbeat
// window plus a second of margin for the last write to age out.
func agentKillGrace() time.Duration {
	return study.DefaultKillGrace()
}
