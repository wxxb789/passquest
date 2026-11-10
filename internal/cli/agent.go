package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/wxxb789/passquest/internal/adaptive"
	"github.com/wxxb789/passquest/internal/agent"
	"github.com/wxxb789/passquest/internal/skill"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/tutor"
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

	st, err := openDatabase(*flags.db, opt)
	if err != nil {
		return internalError(opt, *flags.json, err)
	}
	defer st.Close()

	cfg, code := agentProviderConfig(st, opt, *flags.json)
	if code != 0 {
		return code
	}
	if existing, ok, err := st.LatestAgentRun(true); err != nil {
		return internalError(opt, *flags.json, err)
	} else if ok {
		return usageError(opt, *flags.json, "agent_busy",
			fmt.Sprintf("run %s is unfinished; run passquest agent resume %s or passquest agent cancel %s before starting another brief",
				existing.RunID, existing.RunID, existing.RunID))
	}
	storePath, err := agent.StorePath(*flags.store, st.Path(), opt.Getenv)
	if err != nil {
		return usageError(opt, *flags.json, "invalid_input", err.Error())
	}
	if err := agent.PrepareStorePath(storePath); err != nil {
		return internalError(opt, *flags.json, err)
	}
	runID, err := opt.NewRunID()
	if err != nil {
		return internalError(opt, *flags.json, err)
	}
	maxTurns := *stepsFlag*2 + 2
	prompt, err := agentPrompt(st, runID, *stepsFlag, maxTurns)
	if err != nil {
		return sourceError(opt, *flags.json, err)
	}
	token, err := opt.NewHostToken()
	if err != nil {
		return internalError(opt, *flags.json, err)
	}
	run := store.AgentRun{
		RunID:         runID,
		TaskKind:      agent.TaskStudyBrief,
		PromptVersion: agent.PromptVersion,
		Model:         cfg.Model,
		StorePath:     storePath,
		MaxSteps:      *stepsFlag,
		MaxTurns:      maxTurns,
		CreatedAt:     opt.Now(),
	}
	detail := map[string]any{
		"task": run.TaskKind, "model": run.Model, "max_steps": run.MaxSteps,
		"max_turns": run.MaxTurns, "host_token": token, "prompt_version": run.PromptVersion,
	}
	if err := st.CreateAgentRun(run, detail, opt.Now()); err != nil {
		if errors.Is(err, store.ErrAgentRunUnfinished) {
			return usageError(opt, *flags.json, "agent_busy", err.Error())
		}
		return internalError(opt, *flags.json, err)
	}

	if strings.TrimSpace(opt.Getenv(agent.CredentialEnv)) == "" {
		failure := agent.FailureForCode(agent.CodeMissingCredential)
		detail := agentFailureDetail(failure, agent.StateNeedsAction, "")
		if _, err := st.AppendAgentRunEvent(runID, store.AgentEventNeedsAction, detail, opt.Now()); err != nil {
			return internalError(opt, *flags.json, err)
		}
		return writeAgentFailure(opt, *flags.json, run, agent.StateNeedsAction, failure, nil)
	}

	// start binds the model recorded with the run; a model rename needs
	// cancel + start, while endpoint, timeout and credential stay current.
	request, err := agentHostRequest(st, run, agent.ModeStart, cfg.Model, prompt, token, cfg, *timeoutFlag)
	if err != nil {
		return internalError(opt, *flags.json, err)
	}
	return executeAgentRun(opt, *flags.json, st, run, request, time.Duration(*timeoutFlag)*time.Second)
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
	st, err := openDatabase(*flags.db, opt)
	if err != nil {
		return internalError(opt, *flags.json, err)
	}
	defer st.Close()
	run, latest, code := resolveAgentRun(opt, st, flags.fs.Arg(0), true, *flags.json)
	if code != 0 {
		return code
	}
	storePath, code := agentStoreOverride(opt, run, *flags.store, *flags.json)
	if code != 0 {
		return code
	}
	run.StorePath = storePath
	pending, err := st.AgentRunCancelPending(run.RunID)
	if err != nil {
		return internalError(opt, *flags.json, err)
	}
	if pending {
		return usageError(opt, *flags.json, "cancel_pending",
			fmt.Sprintf("cancellation was requested for run %s; run agent cancel to confirm it", run.RunID))
	}
	switch latest.Kind {
	case store.AgentEventCompleted:
		return usageError(opt, *flags.json, "run_completed", fmt.Sprintf("run %s already completed; use agent show or start a new run", run.RunID))
	case store.AgentEventCancelled:
		return usageError(opt, *flags.json, "run_cancelled", fmt.Sprintf("run %s was cancelled; start a new run", run.RunID))
	case store.AgentEventFailed:
		if agentEventCode(latest) == agent.CodeNoSteps {
			return usageError(opt, *flags.json, "run_completed", fmt.Sprintf("run %s answered without steps; start a new run", run.RunID))
		}
	}
	cfg, code := agentProviderConfig(st, opt, *flags.json)
	if code != 0 {
		return code
	}
	if err := agent.PrepareStorePath(run.StorePath); err != nil {
		return internalError(opt, *flags.json, err)
	}
	prompt, err := agentPrompt(st, run.RunID, run.MaxSteps, run.MaxTurns)
	if err != nil {
		return sourceError(opt, *flags.json, err)
	}
	token, err := opt.NewHostToken()
	if err != nil {
		return internalError(opt, *flags.json, err)
	}
	if _, err := st.AppendAgentRunEvent(run.RunID, store.AgentEventResumed, map[string]any{
		"model": run.Model, "host_token": token, "max_steps": run.MaxSteps, "max_turns": run.MaxTurns,
	}, opt.Now()); err != nil {
		if errors.Is(err, store.ErrAgentRunFinished) {
			return agentAppendFinished(opt, *flags.json, st, run, nil)
		}
		return internalError(opt, *flags.json, err)
	}
	// resume keeps the run's recorded model with the current endpoint, timeout
	// and credential (Q-0006-05).
	request, err := agentHostRequest(st, run, agent.ModeResume, run.Model, prompt, token, cfg, *timeoutFlag)
	if err != nil {
		return internalError(opt, *flags.json, err)
	}
	return executeAgentRun(opt, *flags.json, st, run, request, time.Duration(*timeoutFlag)*time.Second)
}

// agentProviderConfig applies the provider preconditions start and resume
// share: a configured endpoint and the agent's endpoint rule.
func agentProviderConfig(st *store.Store, opt Options, jsonMode bool) (tutor.Config, int) {
	cfg, configured, err := st.TutorConfig()
	if err != nil {
		return tutor.Config{}, internalError(opt, jsonMode, err)
	}
	if !configured {
		return tutor.Config{}, usageError(opt, jsonMode, "not_configured",
			"no provider is configured; run passquest tutor config --endpoint URL --model NAME")
	}
	if err := agent.ValidateEndpoint(cfg.Endpoint); err != nil {
		return tutor.Config{}, usageError(opt, jsonMode, "invalid_config", err.Error())
	}
	return cfg, 0
}

// agentHostRequest builds the one host request shape start and resume share.
// model is explicit because resume binds the run's recorded model while start
// binds the current configuration (Q-0006-05).
func agentHostRequest(st *store.Store, run store.AgentRun, mode, model, prompt, token string, cfg tutor.Config, timeoutSeconds int) (agent.HostRequest, error) {
	executable, err := os.Executable()
	if err != nil {
		return agent.HostRequest{}, err
	}
	return agent.HostRequest{
		ProtocolVersion: agent.ProtocolVersion,
		Mode:            mode,
		RunID:           run.RunID,
		TaskKind:        run.TaskKind,
		PromptVersion:   run.PromptVersion,
		StorePath:       run.StorePath,
		HeartbeatPath:   run.StorePath + ".host",
		HostToken:       token,
		BinaryPath:      executable,
		DBPath:          st.Path(),
		CredentialEnv:   agent.CredentialEnv,
		Model: &agent.ModelConfig{
			Endpoint:        cfg.Endpoint,
			Model:           model,
			TimeoutMS:       cfg.TimeoutMS,
			MaxOutputTokens: cfg.MaxOutputTokens,
			MaxSteps:        run.MaxSteps,
			MaxTurns:        run.MaxTurns,
		},
		Prompt:     prompt,
		DeadlineMS: int64(timeoutSeconds)*1000 + 10000,
	}, nil
}

func runAgentStatus(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"db": true, "agent-store": true})
	flags := newAgentFlags("agent status", opt, wantJSON, true)
	if code := parseAgentPositionals(flags.fs, args, opt, wantJSON, 0, 1, "agent status [RUN]"); code != 0 {
		return code
	}
	st, err := openDatabase(*flags.db, opt)
	if err != nil {
		return internalError(opt, *flags.json, err)
	}
	defer st.Close()
	run, latest, code := resolveAgentRun(opt, st, flags.fs.Arg(0), false, *flags.json)
	if code != 0 {
		return code
	}
	storePath, code := agentStoreOverride(opt, run, *flags.store, *flags.json)
	if code != 0 {
		return code
	}
	run.StorePath = storePath
	steps, err := st.AgentSteps(run.RunID)
	if err != nil {
		return internalError(opt, *flags.json, err)
	}
	attempts, err := st.AgentStepAttempts(run.RunID)
	if err != nil {
		return internalError(opt, *flags.json, err)
	}
	// A token read failure leaves status without a heartbeat match; the run
	// state still comes from the event trail and the read-only snapshot.
	token, _ := agentLatestToken(st, run.RunID)
	heartbeat, fresh := agent.HeartbeatFresh(run.StorePath+".host", run.RunID, token, opt.Now(), time.Duration(agent.HeartbeatStaleMS)*time.Millisecond)

	durable := agentDurableView{Available: false, Reason: "not_started", HeartbeatFresh: fresh, HeartbeatPID: heartbeat.PID}
	var snapshot *agent.Snapshot
	if _, statErr := os.Stat(run.StorePath); statErr != nil {
		durable.Available = true
		durable.Reason = "store_missing"
		snapshot = &agent.Snapshot{Found: false}
	} else {
		response, runErr := runAgentHost(opt, agent.HostRequest{
			ProtocolVersion: agent.ProtocolVersion,
			Mode:            agent.ModeStatus,
			RunID:           run.RunID,
			StorePath:       run.StorePath,
			CredentialEnv:   agent.CredentialEnv,
		}, 20*time.Second)
		switch {
		case runErr != nil:
			var unavailable *agent.UnavailableError
			if errors.As(runErr, &unavailable) {
				durable.Reason = agent.CodeAgentUnavailable
			} else {
				durable.Reason = agent.CodeHostProtocol
			}
		case response.Response.State == "status":
			durable.Available = true
			durable.Reason = ""
			snapshot = response.Response.Snapshot
			if snapshot == nil {
				snapshot = &agent.Snapshot{Found: false}
			}
			durable.ConversationID = response.Response.ConversationID
			if durable.ConversationID == 0 && snapshot != nil {
				durable.ConversationID = snapshot.ConversationID
			}
		default:
			durable.Reason = agent.CodeHostProtocol
		}
	}

	state, failure := agent.ClassifyStatus(agent.StatusInput{
		LatestEvent:      latest.Kind,
		LatestCode:       agentEventCode(latest),
		HeartbeatFresh:   fresh,
		DurableAvailable: durable.Available,
		Snapshot:         snapshot,
		Steps:            len(steps),
	})
	doc := agentStatusDoc{
		SchemaVersion: ProtocolVersion,
		State:         string(state),
		Run:           agentRunViewOf(run),
		Status:        agentStatusViewOf(state, failure, latest.Kind),
		Steps:         agentStepViews(steps),
		Counts:        agentCountsOf(steps, attempts),
		Durable:       durable,
		LatestEvent:   latest.Kind,
		Events:        agentEventCount(st, run.RunID),
		Limits:        agentLimits(),
	}
	if snapshot != nil {
		doc.Durable.Snapshot = snapshot
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
	st, err := openDatabase(*flags.db, opt)
	if err != nil {
		return internalError(opt, *flags.json, err)
	}
	defer st.Close()
	run, latest, code := resolveAgentRun(opt, st, flags.fs.Arg(0), true, *flags.json)
	if code != 0 {
		return code
	}
	storePath, code := agentStoreOverride(opt, run, *flags.store, *flags.json)
	if code != 0 {
		return code
	}
	run.StorePath = storePath
	if latest.Kind == store.AgentEventCompleted {
		return usageError(opt, *flags.json, "run_completed", fmt.Sprintf("run %s already completed", run.RunID))
	}
	if latest.Kind != store.AgentEventCancelRequested && latest.Kind != store.AgentEventCancelled {
		token, err := agentLatestToken(st, run.RunID)
		if err != nil {
			return internalError(opt, *flags.json, err)
		}
		info := map[string]any{"host_token": token}
		if heartbeat, fresh := agent.HeartbeatFresh(run.StorePath+".host", run.RunID, token, opt.Now(), time.Duration(agent.HeartbeatStaleMS)*time.Millisecond); fresh {
			info["pid"] = heartbeat.PID
		}
		if _, err := st.AppendAgentRunEvent(run.RunID, store.AgentEventCancelRequested, info, opt.Now()); err != nil {
			if errors.Is(err, store.ErrAgentRunFinished) {
				latest, _, _ := st.LatestAgentRunEvent(run.RunID)
				if latest.Kind == store.AgentEventCompleted {
					return usageError(opt, *flags.json, "run_completed", fmt.Sprintf("run %s already completed", run.RunID))
				}
				existing := agentCancelDocOf(run, agent.StateCancelled, 0)
				if *flags.json {
					return writeJSON(opt, existing)
				}
				writeAgentCancel(opt.Stdout, existing)
				return 0
			}
			return internalError(opt, *flags.json, err)
		}
	}

	token, err := agentLatestToken(st, run.RunID)
	if err != nil {
		return internalError(opt, *flags.json, err)
	}
	heartbeatPath := run.StorePath + ".host"
	heartbeat, fresh := agent.HeartbeatFresh(heartbeatPath, run.RunID, token, opt.Now(), time.Duration(agent.HeartbeatStaleMS)*time.Millisecond)
	confirmed := true
	if fresh {
		confirmed = false
		_ = agent.SignalCancelPID(heartbeat.PID)
		deadline := time.Now().Add(time.Duration(*waitFlag) * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
			if _, still := agent.HeartbeatFresh(heartbeatPath, run.RunID, token, opt.Now(), time.Duration(agent.HeartbeatStaleMS)*time.Millisecond); !still {
				confirmed = true
				break
			}
		}
		if !confirmed {
			_ = agent.SignalKillPID(heartbeat.PID)
			// The heartbeat is only stale after five seconds. A killed process
			// must be given that window before cancellation is called
			// unconfirmed.
			killDeadline := time.Now().Add(6 * time.Second)
			for time.Now().Before(killDeadline) {
				time.Sleep(100 * time.Millisecond)
				if _, still := agent.HeartbeatFresh(heartbeatPath, run.RunID, token, opt.Now(), time.Duration(agent.HeartbeatStaleMS)*time.Millisecond); !still {
					confirmed = true
					break
				}
			}
		}
	}
	if !confirmed {
		// Do not record a terminal cancellation beside a fresh heartbeat: that
		// would hide a process that may still be running. cancel_requested stays
		// the latest event, so status reports cancelling and resume refuses.
		doc := agentCancelDocOf(run, agent.StateCancelling, heartbeat.PID)
		if *flags.json {
			_ = writeJSONTo(opt.Stderr, doc)
		} else {
			writeAgentCancel(opt.Stderr, doc)
		}
		return 1
	}
	detail := map[string]any{"confirmed": true}
	if heartbeat.PID > 0 {
		detail["pid"] = heartbeat.PID
	}
	changed, err := st.AppendAgentRunTerminal(run.RunID, store.AgentEventCancelled, detail, opt.Now())
	if err != nil {
		return internalError(opt, *flags.json, err)
	}
	if !changed {
		latest, _, _ := st.LatestAgentRunEvent(run.RunID)
		if latest.Kind == store.AgentEventCompleted {
			return usageError(opt, *flags.json, "run_completed", fmt.Sprintf("run %s already completed", run.RunID))
		}
		existing := agentCancelDocOf(run, agent.StateCancelled, heartbeat.PID)
		if *flags.json {
			return writeJSON(opt, existing)
		}
		writeAgentCancel(opt.Stdout, existing)
		return 0
	}
	agent.RemoveHeartbeat(heartbeatPath)
	doc := agentCancelDocOf(run, agent.StateCancelled, heartbeat.PID)
	if *flags.json {
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
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	summaries, err := st.AgentRuns()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	doc := agentRunsDoc{SchemaVersion: ProtocolVersion, State: "runs", Runs: agentRunSummaryViews(summaries), Limits: agentLimits()}
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
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	run, latest, code := resolveAgentRun(opt, st, fs.Arg(0), false, *jsonFlag)
	if code != 0 {
		return code
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
	state := agentAppState(latest.Kind)
	doc := agentShowDoc{
		SchemaVersion: ProtocolVersion,
		State:         "run",
		Run:           agentRunViewOf(run),
		Status:        agentStatusViewOf(state, nil, latest.Kind),
		Counts:        agentCountsOf(steps, attempts),
		Events:        agentEventViews(events),
		Steps:         agentStepViews(steps),
		Effects:       agentEffectsOf(steps, attempts, events),
		Limits:        agentLimits(),
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
	dbPath := databasePath(*dbFlag, opt.Getenv)
	storePath, err := agent.StorePath(*storeFlag, dbPath, opt.Getenv)
	if err != nil {
		return usageError(opt, *jsonFlag, "invalid_input", err.Error())
	}
	if _, err := os.Stat(storePath); err != nil {
		return usageError(opt, *jsonFlag, "store_missing",
			fmt.Sprintf("durable store %s does not exist; pass --agent-store with the saved Pi SQLite file", storePath))
	}
	outcome, runErr := runAgentHost(opt, agent.HostRequest{
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
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return agentRecordStepError(opt, *jsonFlag, "internal_error", err.Error())
	}
	defer st.Close()
	run, ok, err := st.AgentRun(request.RunID)
	if err != nil {
		return agentRecordStepError(opt, *jsonFlag, "internal_error", err.Error())
	}
	if !ok {
		return agentRecordStepError(opt, *jsonFlag, "unknown_run", fmt.Sprintf("run %s is unknown", request.RunID))
	}
	step, err := agent.ValidateStep(request.RunID, request.AttemptKey, request.Kind, request.Title, request.Body, request.Index, run.MaxSteps)
	if err != nil {
		return agentRecordStepError(opt, *jsonFlag, "invalid_step", err.Error())
	}
	recorded, duplicate, attempts, err := st.RecordAgentStep(step.RunID, step.Index, step.Kind, step.Title, step.Body, step.AttemptKey, opt.Now())
	if err != nil {
		switch {
		case errors.Is(err, store.ErrAgentRunNotFound):
			return agentRecordStepError(opt, *jsonFlag, "unknown_run", err.Error())
		case errors.Is(err, store.ErrAgentRunCancelled):
			return agentRecordStepError(opt, *jsonFlag, "run_cancelled", err.Error())
		case errors.Is(err, store.ErrAgentRunCompleted):
			return agentRecordStepError(opt, *jsonFlag, "run_completed", err.Error())
		default:
			return agentRecordStepError(opt, *jsonFlag, "internal_error", err.Error())
		}
	}
	doc := agentStepDoc{
		SchemaVersion: ProtocolVersion,
		State:         "step",
		RunID:         recorded.RunID,
		Step: agentStepView{
			Index: recorded.Index, Kind: recorded.Kind, Title: recorded.Title, Body: recorded.Body,
			Advisory: true, VerifiedAnswer: false, CreatedAt: source.FormatTime(recorded.CreatedAt),
		},
		Duplicate:  duplicate,
		AttemptKey: step.AttemptKey,
		Attempts:   attempts,
	}
	return writeJSON(opt, doc)
}

// agentAppendFinished reports the terminal state another process already stored.
// It is used when a late transition loses the race with completion/cancellation
// so the command cannot present the wrong outcome.
func agentAppendFinished(opt Options, jsonMode bool, st *store.Store, run store.AgentRun, steps []store.AgentStep) int {
	latest, ok, err := st.LatestAgentRunEvent(run.RunID)
	if err != nil {
		return internalError(opt, jsonMode, err)
	}
	if !ok {
		return internalError(opt, jsonMode, fmt.Errorf("agent: run %s has no events", run.RunID))
	}
	state := agentAppState(latest.Kind)
	if state != agent.StateCompleted && state != agent.StateCancelled {
		return internalError(opt, jsonMode, fmt.Errorf("agent: run %s is not terminal", run.RunID))
	}
	if steps == nil {
		steps, err = st.AgentSteps(run.RunID)
		if err != nil {
			return internalError(opt, jsonMode, err)
		}
	}
	doc := agentRunDocOf(run, state, nil, agent.HostResponse{}, steps)
	doc.Status.Reason = latest.Kind
	if jsonMode {
		return writeJSON(opt, doc)
	}
	writeAgentRun(opt.Stdout, doc)
	return 0
}

// executeAgentRun runs one start/resume host process, records the transition and
// writes the outcome document. start and resume differ only in the request and
// the events already on disk.
func executeAgentRun(opt Options, jsonMode bool, st *store.Store, run store.AgentRun, request agent.HostRequest, timeout time.Duration) int {
	agent.RemoveHeartbeat(request.HeartbeatPath)
	outcome, runErr := runAgentHost(opt, request, timeout)
	response := outcome.Response
	steps, err := st.AgentSteps(run.RunID)
	if err != nil {
		return internalError(opt, jsonMode, err)
	}

	if runErr != nil {
		var unavailable *agent.UnavailableError
		failure := agent.FailureForCode(agent.CodeHostError)
		if errors.As(runErr, &unavailable) {
			failure = agent.FailureForCode(agent.CodeAgentUnavailable)
			failure.Message = redactAgent(opt, runErr.Error())
		} else {
			var protocol *agent.ProtocolError
			if errors.As(runErr, &protocol) {
				failure = agent.FailureForCode(agent.CodeHostProtocol)
				failure.Message = redactAgent(opt, protocol.Message)
			} else {
				failure.Message = redactAgent(opt, runErr.Error())
			}
		}
		state := agent.StateFailed
		if failure.NeedsAction() {
			state = agent.StateNeedsAction
		}
		if _, err := st.AppendAgentRunEvent(run.RunID, eventForState(state), agentFailureDetail(failure, state, ""), opt.Now()); err != nil {
			if errors.Is(err, store.ErrAgentRunFinished) {
				return agentAppendFinished(opt, jsonMode, st, run, steps)
			}
			return internalError(opt, jsonMode, err)
		}
		return writeAgentFailure(opt, jsonMode, run, state, failure, steps)
	}

	latest, _, err := st.LatestAgentRunEvent(run.RunID)
	if err != nil {
		return internalError(opt, jsonMode, err)
	}
	cancelRequested := latest.Kind == store.AgentEventCancelRequested || (response.State == "cancelled")
	if outcome.Interrupted || response.State == "interrupted" || response.State == "cancelled" {
		state := agent.StatePaused
		if cancelRequested || response.State == "cancelled" {
			state = agent.StateCancelled
			changed, err := st.AppendAgentRunTerminal(run.RunID, store.AgentEventCancelled,
				map[string]any{"confirmed": true, "host_state": response.State}, opt.Now())
			if err != nil {
				return internalError(opt, jsonMode, err)
			}
			if !changed {
				return agentAppendFinished(opt, jsonMode, st, run, steps)
			}
		} else {
			if _, err := st.AppendAgentRunEvent(run.RunID, store.AgentEventInterrupted, map[string]any{
				"reason": outcome.InterruptReason, "exit_code": outcome.ExitCode, "host_state": response.State,
			}, opt.Now()); err != nil {
				if errors.Is(err, store.ErrAgentRunFinished) {
					return agentAppendFinished(opt, jsonMode, st, run, steps)
				}
				return internalError(opt, jsonMode, err)
			}
		}
		doc := agentRunDocOf(run, state, nil, response, steps)
		if state == agent.StatePaused {
			doc.Status.Reason = "interrupted"
			if outcome.InterruptReason != "" {
				doc.Status.Reason = outcome.InterruptReason
			}
		}
		if state == agent.StateCancelled {
			if jsonMode {
				return writeJSON(opt, doc)
			}
			writeAgentRun(opt.Stdout, doc)
			return 0
		}
		if jsonMode {
			if err := writeJSONTo(opt.Stdout, doc); err != nil {
				return internalError(opt, jsonMode, err)
			}
			return 4
		}
		writeAgentRun(opt.Stdout, doc)
		return 4
	}
	if response.State == "completed" {
		if len(steps) == 0 {
			failure := agent.FailureForCode(agent.CodeNoSteps)
			if _, err := st.AppendAgentRunEvent(run.RunID, store.AgentEventFailed, agentFailureDetail(failure, agent.StateFailed, "completed_without_steps"), opt.Now()); err != nil {
				if errors.Is(err, store.ErrAgentRunFinished) {
					return agentAppendFinished(opt, jsonMode, st, run, steps)
				}
				return internalError(opt, jsonMode, err)
			}
			return writeAgentFailure(opt, jsonMode, run, agent.StateFailed, failure, steps)
		}
		detail := agentCompletionDetail(response, len(steps))
		changed, err := st.AppendAgentRunTerminal(run.RunID, store.AgentEventCompleted, detail, opt.Now())
		if err != nil {
			return internalError(opt, jsonMode, err)
		}
		if !changed {
			return agentAppendFinished(opt, jsonMode, st, run, steps)
		}
		doc := agentRunDocOf(run, agent.StateCompleted, nil, response, steps)
		if jsonMode {
			return writeJSON(opt, doc)
		}
		writeAgentRun(opt.Stdout, doc)
		return 0
	}

	var failure agent.Failure
	switch {
	case response.Error != nil:
		failure = agent.FailureForCode(response.Error.Code)
		if strings.TrimSpace(response.Error.Message) != "" {
			failure.Message = redactAgent(opt, response.Error.Message)
		}
	case response.Failure != nil:
		failure = agent.ClassifyRunFailure(response.Failure.Reason, response.Failure.Detail)
		failure.Message = redactAgent(opt, failure.Message)
	default:
		failure = agent.FailureForCode(agent.CodeHostProtocol)
	}
	state := agent.StateFailed
	if failure.NeedsAction() {
		state = agent.StateNeedsAction
	}
	reason := ""
	if response.Failure != nil {
		reason = response.Failure.Reason
	}
	if _, err := st.AppendAgentRunEvent(run.RunID, eventForState(state), agentFailureDetail(failure, state, reason), opt.Now()); err != nil {
		if errors.Is(err, store.ErrAgentRunFinished) {
			return agentAppendFinished(opt, jsonMode, st, run, steps)
		}
		return internalError(opt, jsonMode, err)
	}
	return writeAgentFailure(opt, jsonMode, run, state, failure, steps)
}

func runAgentHost(opt Options, request agent.HostRequest, timeout time.Duration) (agent.RunOutcome, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		select {
		case <-signals:
			cancel()
		case <-ctx.Done():
		}
	}()
	return opt.AgentRunner.Run(ctx, request)
}

func agentPrompt(st *store.Store, runID string, maxSteps, maxTurns int) (string, error) {
	input, _, _, err := buildAdaptiveInput(st, "")
	if err != nil {
		return "", err
	}
	result := adaptive.Recommend(input)
	packet, err := agent.BuildStudyBrief(runID, input.Records, result, skill.All(), maxSteps, maxTurns)
	if err != nil {
		return "", err
	}
	return packet.Prompt()
}

func agentStoreOverride(opt Options, run store.AgentRun, raw string, jsonMode bool) (string, int) {
	if strings.TrimSpace(raw) == "" {
		return run.StorePath, 0
	}
	absolute, err := filepath.Abs(raw)
	if err != nil {
		return "", usageError(opt, jsonMode, "invalid_input", fmt.Sprintf("--agent-store: %v", err))
	}
	return absolute, 0
}

func resolveAgentRun(opt Options, st *store.Store, raw string, unfinished bool, jsonMode bool) (store.AgentRun, store.AgentRunEvent, int) {
	var (
		run store.AgentRun
		ok  bool
		err error
	)
	if strings.TrimSpace(raw) != "" {
		runID, normalizeErr := agent.NormalizeRunID(raw)
		if normalizeErr != nil {
			return store.AgentRun{}, store.AgentRunEvent{}, usageError(opt, jsonMode, "invalid_input", normalizeErr.Error())
		}
		run, ok, err = st.AgentRun(runID)
		if err != nil {
			return store.AgentRun{}, store.AgentRunEvent{}, internalError(opt, jsonMode, err)
		}
		if !ok {
			return store.AgentRun{}, store.AgentRunEvent{}, usageError(opt, jsonMode, "unknown_run", fmt.Sprintf("run %s is unknown", runID))
		}
	} else {
		run, ok, err = st.LatestAgentRun(unfinished)
		if err != nil {
			return store.AgentRun{}, store.AgentRunEvent{}, internalError(opt, jsonMode, err)
		}
		if !ok {
			if unfinished {
				return store.AgentRun{}, store.AgentRunEvent{}, usageError(opt, jsonMode, "no_run", "no unfinished agent run; start one with passquest agent start")
			}
			return store.AgentRun{}, store.AgentRunEvent{}, usageError(opt, jsonMode, "no_run", "no agent run has been recorded")
		}
	}
	latest, exists, err := st.LatestAgentRunEvent(run.RunID)
	if err != nil {
		return store.AgentRun{}, store.AgentRunEvent{}, internalError(opt, jsonMode, err)
	}
	if !exists {
		latest = store.AgentRunEvent{RunID: run.RunID, Kind: store.AgentEventStarted}
	}
	return run, latest, 0
}

func agentLatestToken(st *store.Store, runID string) (string, error) {
	events, err := st.AgentRunEvents(runID)
	if err != nil {
		return "", err
	}
	for i := len(events) - 1; i >= 0; i-- {
		var detail struct {
			HostToken string `json:"host_token"`
		}
		if err := json.Unmarshal([]byte(events[i].DetailJSON), &detail); err == nil && detail.HostToken != "" {
			return detail.HostToken, nil
		}
	}
	return "", nil
}

func agentEventCode(event store.AgentRunEvent) string {
	var detail struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(event.DetailJSON), &detail); err == nil {
		return detail.Code
	}
	return ""
}

func agentEventCount(st *store.Store, runID string) int {
	events, err := st.AgentRunEvents(runID)
	if err != nil {
		return 0
	}
	return len(events)
}

func eventForState(state agent.State) string {
	if state == agent.StateNeedsAction {
		return store.AgentEventNeedsAction
	}
	return store.AgentEventFailed
}

func agentFailureDetail(failure agent.Failure, state agent.State, reason string) map[string]any {
	detail := map[string]any{
		"code": failure.Code, "message": failure.Message, "action": failure.Action, "state": string(state),
	}
	if reason != "" {
		detail["reason"] = reason
	}
	return detail
}

func agentCompletionDetail(response agent.HostResponse, steps int) map[string]any {
	detail := map[string]any{
		"steps": steps, "answer_chars": response.AnswerChars, "turns": response.Turns,
		"tool_calls": response.ToolCalls, "tool_errors": response.ToolErrors,
	}
	if len(response.Usage) > 0 {
		detail["usage"] = json.RawMessage(response.Usage)
	}
	return detail
}

func redactAgent(opt Options, value string) string {
	key := strings.TrimSpace(opt.Getenv(agent.CredentialEnv))
	if key == "" {
		return value
	}
	return strings.ReplaceAll(value, key, "[redacted]")
}

func agentAppState(kind string) agent.State {
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

func writeAgentFailure(opt Options, jsonMode bool, run store.AgentRun, state agent.State, failure agent.Failure, steps []store.AgentStep) int {
	status := agentStatusViewOf(state, &failure, "")
	runView := agentRunViewOf(run)
	doc := agentFailureDoc{
		SchemaVersion: ProtocolVersion,
		Error:         agent.ErrorDoc{Code: failure.Code, Message: failure.Message},
		Run:           &runView,
		Status:        &status,
		Steps:         agentStepViews(steps),
		Limits:        agentLimits(),
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
		Limits:        agentLimits(),
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
