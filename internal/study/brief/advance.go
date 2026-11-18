package brief

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/adaptive"
	"github.com/wxxb789/passquest/internal/agent"
	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/skill"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
	"github.com/wxxb789/passquest/internal/study/recommend"
	"github.com/wxxb789/passquest/internal/tutor"
)

// start creates one run over the current provider configuration and drives
// one host process from it.
func start(s *study.Service, in AdvanceInput) (Advanced, error) {
	st := kernel.Store(s)
	p := kernel.P(s)
	cfg, registry, err := agentProviderConfig(s)
	if err != nil {
		return Advanced{}, err
	}
	if err := kernel.RequireVerifiedTextChat(s, registry, cfg.Provider, cfg.Model); err != nil {
		return Advanced{}, err
	}
	if existing, ok, err := st.LatestAgentRun(true); err != nil {
		return Advanced{}, err
	} else if ok {
		return Advanced{}, study.Refuse("agent_busy",
			"run %s is unfinished; run passquest agent resume %s or passquest agent cancel %s before starting another brief",
			existing.RunID, existing.RunID, existing.RunID)
	}
	storePath, err := agent.StorePath(in.StoreOverride, s.Path(), p.Getenv)
	if err != nil {
		return Advanced{}, study.Refuse("invalid_input", "%s", err.Error())
	}
	if err := agent.PrepareStorePath(storePath); err != nil {
		return Advanced{}, err
	}
	runID, err := p.NewRunID()
	if err != nil {
		return Advanced{}, err
	}
	maxTurns := in.MaxSteps*2 + 2
	prompt, err := agentPrompt(s, runID, in.MaxSteps, maxTurns)
	if err != nil {
		return Advanced{}, err
	}
	token, err := p.NewHostToken()
	if err != nil {
		return Advanced{}, err
	}
	run := store.AgentRun{
		RunID:         runID,
		TaskKind:      agent.TaskStudyBrief,
		PromptVersion: agent.PromptVersion,
		Provider:      cfg.Provider,
		Model:         cfg.Model,
		StorePath:     storePath,
		MaxSteps:      in.MaxSteps,
		MaxTurns:      maxTurns,
		CreatedAt:     p.Now(),
	}
	detail := map[string]any{
		"task": run.TaskKind, "model": run.Model, "max_steps": run.MaxSteps,
		"max_turns": run.MaxTurns, "host_token": token, "prompt_version": run.PromptVersion,
	}
	if err := st.CreateAgentRun(run, detail, p.Now()); err != nil {
		if errors.Is(err, store.ErrAgentRunUnfinished) {
			return Advanced{}, study.Refuse("agent_busy", "%s", err.Error())
		}
		return Advanced{}, err
	}

	env, _, present := provider.CredentialValue(p.Getenv, run.Provider)
	if !present {
		return agentMissingCredential(s, run, env)
	}

	// start binds the model recorded with the run; a model rename needs
	// cancel + start, while endpoint, timeout and credential stay current.
	request, err := agentHostRequest(s, run, agent.ModeStart, cfg.Model, prompt, token, cfg, in.TimeoutSeconds, env)
	if err != nil {
		return Advanced{}, err
	}
	return executeAgentRun(s, run, request, time.Duration(in.TimeoutSeconds)*time.Second)
}

// resume continues the addressed unfinished run. The run's recorded model and
// store are authoritative; the current endpoint and credential are not.
func resume(s *study.Service, in AdvanceInput) (Advanced, error) {
	st := kernel.Store(s)
	p := kernel.P(s)
	run, latest, err := Resolve(st, in.RunRef, true)
	if err != nil {
		return Advanced{}, err
	}
	storePath, err := recordedStorePath(st, run, in.StoreOverride)
	if err != nil {
		return Advanced{}, err
	}
	run.StorePath = storePath
	pending, err := st.AgentRunCancelPending(run.RunID)
	if err != nil {
		return Advanced{}, err
	}
	if pending {
		return Advanced{}, study.Refuse("cancel_pending",
			"cancellation was requested for run %s; run agent cancel to confirm it", run.RunID)
	}
	// A fresh heartbeat is a live host only while the trail still describes a
	// live claim. After an interruption or failure the recording process has
	// already reaped the host; a heartbeat it could not remove (a Windows kill
	// cannot) must not veto resume. A start/resume claim younger than the
	// heartbeat window may not have written its heartbeat yet, so it is
	// treated as live too.
	// An unknown read (present file, transient failure) is live: refusing a
	// resume costs nothing, starting a second host beside the old one does not.
	if heartbeat, status := agent.ProbeHeartbeat(run.StorePath+".host", run.RunID, p.Now(), heartbeatWindow()); status.Live() &&
		agent.HeartbeatAuthority(latest.Kind) {
		return Advanced{}, study.Refuse("run_running",
			"run %s has a live host (pid %d); cancel it before resuming", run.RunID, heartbeat.PID)
	}
	if (latest.Kind == store.AgentEventStarted || latest.Kind == store.AgentEventResumed) &&
		!latest.At.IsZero() && p.Now().Sub(latest.At) < heartbeatWindow() {
		return Advanced{}, study.Refuse("run_running",
			"run %s was claimed moments ago and its host may still be starting; check agent status before resuming", run.RunID)
	}
	switch latest.Kind {
	case store.AgentEventCompleted:
		return Advanced{}, study.Refuse("run_completed", "run %s already completed; use agent show or start a new run", run.RunID)
	case store.AgentEventCancelled:
		return Advanced{}, study.Refuse("run_cancelled", "run %s was cancelled; start a new run", run.RunID)
	case store.AgentEventFailed:
		if agentEventCode(latest) == agent.CodeNoSteps {
			return Advanced{}, study.Refuse("run_completed", "run %s answered without steps; start a new run", run.RunID)
		}
	}
	cfg, registry, err := agentProviderConfig(s)
	if err != nil {
		return Advanced{}, err
	}
	env := provider.CredentialEnvFor(run.Provider)
	if strings.TrimSpace(run.Provider) != "" {
		if len(registry.Providers) == 0 {
			loaded, _, err := provider.Load(p.Getenv)
			if err != nil {
				return Advanced{}, study.Refuse("invalid_registry", "%s", err.Error())
			}
			registry = loaded
		}
		recorded, _, ok := registry.Find(run.Provider, run.Model)
		if !ok {
			return Advanced{}, study.Refuse("unknown_model",
				"run %s recorded model %s/%s, which is no longer in the registry", run.RunID, run.Provider, run.Model)
		}
		if recorded.Custom {
			if cfg.Provider != run.Provider {
				return Advanced{}, study.Refuse("provider_changed",
					"run %s uses the custom provider; run passquest model use custom/%s --endpoint URL before resuming",
					run.RunID, run.Model)
			}
		} else {
			cfg.Endpoint = recorded.Endpoint
		}
		if err := kernel.RequireVerifiedTextChat(s, registry, run.Provider, run.Model); err != nil {
			return Advanced{}, err
		}
		var present bool
		env, _, present = provider.CredentialValue(p.Getenv, run.Provider)
		if !present {
			return agentMissingCredential(s, run, env)
		}
	}
	if err := agent.PrepareStorePath(run.StorePath); err != nil {
		return Advanced{}, err
	}
	prompt, err := agentPrompt(s, run.RunID, run.MaxSteps, run.MaxTurns)
	if err != nil {
		return Advanced{}, err
	}
	token, err := p.NewHostToken()
	if err != nil {
		return Advanced{}, err
	}
	if _, err := st.ClaimAgentResume(run.RunID, latest.ID, map[string]any{
		"model": run.Model, "host_token": token, "max_steps": run.MaxSteps, "max_turns": run.MaxTurns,
	}, p.Now()); err != nil {
		switch {
		case errors.Is(err, store.ErrAgentRunChanged):
			return Advanced{}, study.Refuse("run_running",
				"run %s changed or was claimed by another process; check agent status before retrying", run.RunID)
		case errors.Is(err, store.ErrAgentRunFinished):
			return agentAppendFinished(s, run, nil)
		case errors.Is(err, store.ErrAgentRunCancelled):
			return Advanced{}, study.Refuse("cancel_pending",
				"cancellation was requested for run %s; run agent cancel to confirm it", run.RunID)
		default:
			return Advanced{}, err
		}
	}
	request, err := agentHostRequest(s, run, agent.ModeResume, run.Model, prompt, token, cfg, in.TimeoutSeconds, env)
	if err != nil {
		return Advanced{}, err
	}
	return executeAgentRun(s, run, request, time.Duration(in.TimeoutSeconds)*time.Second)
}

// agentProviderConfig applies the provider preconditions start and resume
// share: a configured endpoint, the active registry endpoint and the agent's
// endpoint rule. The loaded registry is returned so a run resolves it once.
func agentProviderConfig(s *study.Service) (tutor.Config, provider.Registry, error) {
	cfg, configured, err := kernel.Store(s).TutorConfig()
	if err != nil {
		return tutor.Config{}, provider.Registry{}, err
	}
	if !configured {
		return tutor.Config{}, provider.Registry{}, study.Refuse("not_configured",
			"no provider is configured; run passquest tutor config --endpoint URL --model NAME")
	}
	var registry provider.Registry
	if strings.TrimSpace(cfg.Provider) != "" {
		var err error
		registry, _, err = provider.Load(kernel.P(s).Getenv)
		if err != nil {
			return tutor.Config{}, provider.Registry{}, study.Refuse("invalid_registry", "%s", err.Error())
		}
		_, _, refreshed, _, err := kernel.ActiveProvider(cfg, registry)
		if err != nil {
			return tutor.Config{}, provider.Registry{}, study.Refuse("unknown_model", "%s", err.Error())
		}
		cfg = refreshed
	}
	if err := agent.ValidateEndpoint(cfg.Endpoint); err != nil {
		return tutor.Config{}, provider.Registry{}, study.Refuse("invalid_config", "%s", err.Error())
	}
	return cfg, registry, nil
}

// agentMissingCredential records the one needs_action event every missing-key
// path shares.
func agentMissingCredential(s *study.Service, run store.AgentRun, env string) (Advanced, error) {
	failure := agent.FailureForCode(agent.CodeMissingCredential)
	failure.Message = fmt.Sprintf("%s is not set", env)
	detail := agentFailureDetail(failure, agent.StateNeedsAction, "")
	if _, err := kernel.Store(s).AppendAgentRunEvent(run.RunID, store.AgentEventNeedsAction, detail, kernel.P(s).Now()); err != nil {
		return Advanced{}, err
	}
	return Advanced{Run: run, State: agent.StateNeedsAction, Failure: &failure}, nil
}

// agentHostRequest builds the one host request shape start and resume share.
// model is explicit because resume binds the run's recorded model while start
// binds the current configuration (Q-0006-05).
func agentHostRequest(s *study.Service, run store.AgentRun, mode, model, prompt, token string, cfg tutor.Config, timeoutSeconds int, credentialEnv string) (agent.HostRequest, error) {
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
		DBPath:          s.Path(),
		CredentialEnv:   credentialEnv,
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

// agentPrompt builds the bounded study-brief packet from the recommendation
// input, exactly as the CLI's agent start and resume did.
func agentPrompt(s *study.Service, runID string, maxSteps, maxTurns int) (string, error) {
	input, _, _, err := recommend.BuildInput(kernel.Store(s), "")
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

// executeAgentRun runs one start/resume host process, records the transition
// and reports the outcome document's inputs. start and resume differ only in
// the request and the events already on disk.
func executeAgentRun(s *study.Service, run store.AgentRun, request agent.HostRequest, timeout time.Duration) (Advanced, error) {
	st := kernel.Store(s)
	p := kernel.P(s)
	agent.RemoveHeartbeatForRun(request.HeartbeatPath, request.RunID)
	outcome, runErr := RunHost(p.Agent, request, timeout)
	// RunHost returns only after the host process has exited. A hard kill
	// (Windows) cannot remove its liveness or control files itself, so the
	// application clears this run's files once the process is gone. Files that
	// name another run are left for that run's host.
	agent.RemoveHeartbeatForRun(request.HeartbeatPath, request.RunID)
	agent.RemoveHostControlForRun(request.HeartbeatPath, request.RunID, "")
	response := outcome.Response
	steps, err := st.AgentSteps(run.RunID)
	if err != nil {
		return Advanced{}, err
	}

	if runErr != nil {
		var unavailable *agent.UnavailableError
		failure := agent.FailureForCode(agent.CodeHostError)
		if errors.As(runErr, &unavailable) {
			failure = agent.FailureForCode(agent.CodeAgentUnavailable)
			failure.Message = redact(s, runErr.Error())
		} else {
			var protocol *agent.ProtocolError
			if errors.As(runErr, &protocol) {
				failure = agent.FailureForCode(agent.CodeHostProtocol)
				failure.Message = redact(s, protocol.Message)
			} else {
				failure.Message = redact(s, runErr.Error())
			}
		}
		state := agent.StateFailed
		if failure.NeedsAction() {
			state = agent.StateNeedsAction
		}
		if _, err := st.AppendAgentRunEvent(run.RunID, eventForState(state), agentFailureDetail(failure, state, ""), p.Now()); err != nil {
			if errors.Is(err, store.ErrAgentRunFinished) {
				return agentAppendFinished(s, run, steps)
			}
			return Advanced{}, err
		}
		return Advanced{Run: run, State: state, Failure: &failure, Steps: steps}, nil
	}

	latest, _, err := st.LatestAgentRunEvent(run.RunID)
	if err != nil {
		return Advanced{}, err
	}
	cancelRequested := latest.Kind == store.AgentEventCancelRequested || (response.State == "cancelled")
	if outcome.Interrupted || response.State == "interrupted" || response.State == "cancelled" {
		state := agent.StatePaused
		if cancelRequested || response.State == "cancelled" {
			state = agent.StateCancelled
			changed, err := st.AppendAgentRunTerminal(run.RunID, store.AgentEventCancelled,
				map[string]any{"confirmed": true, "host_state": response.State}, p.Now())
			if err != nil {
				return Advanced{}, err
			}
			if !changed {
				return agentAppendFinished(s, run, steps)
			}
		} else {
			if _, err := st.AppendAgentRunEvent(run.RunID, store.AgentEventInterrupted, map[string]any{
				"reason": outcome.InterruptReason, "exit_code": outcome.ExitCode, "host_state": response.State,
			}, p.Now()); err != nil {
				if errors.Is(err, store.ErrAgentRunFinished) {
					return agentAppendFinished(s, run, steps)
				}
				return Advanced{}, err
			}
		}
		advanced := Advanced{Run: run, State: state, Response: response, Steps: steps}
		if state == agent.StatePaused {
			advanced.Reason = "interrupted"
			if outcome.InterruptReason != "" {
				advanced.Reason = outcome.InterruptReason
			}
		}
		return advanced, nil
	}
	if response.State == "completed" {
		if len(steps) == 0 {
			failure := agent.FailureForCode(agent.CodeNoSteps)
			if _, err := st.AppendAgentRunEvent(run.RunID, store.AgentEventFailed, agentFailureDetail(failure, agent.StateFailed, "completed_without_steps"), p.Now()); err != nil {
				if errors.Is(err, store.ErrAgentRunFinished) {
					return agentAppendFinished(s, run, steps)
				}
				return Advanced{}, err
			}
			return Advanced{Run: run, State: agent.StateFailed, Failure: &failure, Steps: steps}, nil
		}
		detail := agentCompletionDetail(response, len(steps))
		changed, err := st.AppendAgentRunTerminal(run.RunID, store.AgentEventCompleted, detail, p.Now())
		if err != nil {
			return Advanced{}, err
		}
		if !changed {
			return agentAppendFinished(s, run, steps)
		}
		return Advanced{Run: run, State: agent.StateCompleted, Response: response, Steps: steps}, nil
	}

	var failure agent.Failure
	switch {
	case response.Error != nil:
		failure = agent.FailureForCode(response.Error.Code)
		if strings.TrimSpace(response.Error.Message) != "" {
			failure.Message = redact(s, response.Error.Message)
		}
	case response.Failure != nil:
		failure = agent.ClassifyRunFailure(response.Failure.Reason, response.Failure.Detail)
		failure.Message = redact(s, failure.Message)
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
	if _, err := st.AppendAgentRunEvent(run.RunID, eventForState(state), agentFailureDetail(failure, state, reason), p.Now()); err != nil {
		if errors.Is(err, store.ErrAgentRunFinished) {
			return agentAppendFinished(s, run, steps)
		}
		return Advanced{}, err
	}
	return Advanced{Run: run, State: state, Failure: &failure, Steps: steps}, nil
}

// agentAppendFinished reports the terminal state another process already
// stored. It is used when a late transition loses the race with completion or
// cancellation so the command cannot present the wrong outcome.
func agentAppendFinished(s *study.Service, run store.AgentRun, steps []store.AgentStep) (Advanced, error) {
	st := kernel.Store(s)
	latest, ok, err := st.LatestAgentRunEvent(run.RunID)
	if err != nil {
		return Advanced{}, err
	}
	if !ok {
		return Advanced{}, fmt.Errorf("agent: run %s has no events", run.RunID)
	}
	// Only a terminal event reaches here; its kind spells its state.
	state := agent.State(latest.Kind)
	if state != agent.StateCompleted && state != agent.StateCancelled {
		return Advanced{}, fmt.Errorf("agent: run %s is not terminal", run.RunID)
	}
	if steps == nil {
		steps, err = st.AgentSteps(run.RunID)
		if err != nil {
			return Advanced{}, err
		}
	}
	return Advanced{Run: run, State: state, Steps: steps}, nil
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

// redact removes the credential value from a host message: exactly the
// substitution the CLI's failure documents made, and nothing more.
func redact(s *study.Service, value string) string {
	key := strings.TrimSpace(kernel.P(s).Getenv(agent.CredentialEnv))
	if key == "" {
		return value
	}
	return strings.ReplaceAll(value, key, "[redacted]")
}
