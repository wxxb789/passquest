package brief

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/agent"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
)

// RunStatus is one run's live status: the recorded trail, the heartbeat left
// beside the Pi store and that store's own snapshot when its host answers a
// status request. Its fields are the inputs of the CLI's agent status document
// and the window's brief status.
type RunStatus struct {
	Run         store.AgentRun
	State       agent.State
	Failure     *agent.Failure
	Steps       []store.AgentStep
	Attempts    []store.AgentStepAttempt
	LatestEvent string
	Events      int
	Durable     Durable
}

// Durable is what the trail can say about the run's host. Available is false
// while no snapshot was read, with Reason naming why ("not_started",
// "store_missing", agent.CodeAgentUnavailable or agent.CodeHostProtocol).
type Durable struct {
	Available      bool
	Reason         string
	ConversationID int64
	HeartbeatPID   int
	HeartbeatFresh bool
	Snapshot       *agent.Snapshot
}

// Status reads the live status of the run runRef addresses: the explicit
// reference, or the newest run when it is empty. storeOverride is the
// exploratory --agent-store this command keeps, unlike the commands that
// address one recorded run. It reads one clock value and one host status
// document, exactly as the CLI's agent status did.
func Status(ctx context.Context, s *study.Service, runRef, storeOverride string) (RunStatus, error) {
	_ = ctx
	st := kernel.Store(s)
	p := kernel.P(s)
	run, latest, err := Resolve(st, runRef, false)
	if err != nil {
		return RunStatus{}, err
	}
	storePath, err := storeOverridePath(st, run, storeOverride)
	if err != nil {
		return RunStatus{}, err
	}
	run.StorePath = storePath
	steps, err := st.AgentSteps(run.RunID)
	if err != nil {
		return RunStatus{}, err
	}
	attempts, err := st.AgentStepAttempts(run.RunID)
	if err != nil {
		return RunStatus{}, err
	}
	// A fresh heartbeat belongs to the run, not to the token of the latest
	// trail event: a second host that raced the claim still means a live run.
	// It describes a live host only while the trail is a live claim; an
	// interrupted/failed/completed trail wins over a heartbeat the dead host
	// could not remove.
	heartbeat, status := agent.ProbeHeartbeat(run.StorePath+".host", run.RunID, p.Now(), heartbeatWindow())
	fresh := status.Live()
	if !agent.HeartbeatAuthority(latest.Kind) {
		fresh = false
	}

	durable := Durable{Available: false, Reason: "not_started", HeartbeatFresh: fresh, HeartbeatPID: heartbeat.PID}
	var snapshot *agent.Snapshot
	if _, statErr := os.Stat(run.StorePath); statErr != nil {
		durable.Available = true
		durable.Reason = "store_missing"
		snapshot = &agent.Snapshot{Found: false}
	} else {
		response, runErr := RunHost(p.Agent, agent.HostRequest{
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
	if snapshot != nil {
		durable.Snapshot = snapshot
	}
	return RunStatus{
		Run:         run,
		State:       state,
		Failure:     failure,
		Steps:       steps,
		Attempts:    attempts,
		LatestEvent: latest.Kind,
		Events:      agentEventCount(st, run.RunID),
		Durable:     durable,
	}, nil
}

// Resolve reads the run a command addresses: the explicit run reference, or
// the newest run (unfinished only for the commands that continue one). The
// latest event falls back to the run's started claim when no event is stored,
// so a fresh run reads as claimed rather than event-less. The CLI's
// `agent show` uses it too.
func Resolve(st *store.Store, raw string, unfinished bool) (store.AgentRun, store.AgentRunEvent, error) {
	var (
		run store.AgentRun
		ok  bool
		err error
	)
	if strings.TrimSpace(raw) != "" {
		runID, normalizeErr := agent.NormalizeRunID(raw)
		if normalizeErr != nil {
			return store.AgentRun{}, store.AgentRunEvent{}, study.Refuse("invalid_input", "%s", normalizeErr.Error())
		}
		run, ok, err = st.AgentRun(runID)
		if err != nil {
			return store.AgentRun{}, store.AgentRunEvent{}, err
		}
		if !ok {
			return store.AgentRun{}, store.AgentRunEvent{}, study.Refuse("unknown_run", "run %s is unknown", runID)
		}
	} else {
		run, ok, err = st.LatestAgentRun(unfinished)
		if err != nil {
			return store.AgentRun{}, store.AgentRunEvent{}, err
		}
		if !ok {
			if unfinished {
				return store.AgentRun{}, store.AgentRunEvent{}, study.Refuse("no_run", "no unfinished agent run; start one with passquest agent start")
			}
			return store.AgentRun{}, store.AgentRunEvent{}, study.Refuse("no_run", "no agent run has been recorded")
		}
	}
	latest, exists, err := st.LatestAgentRunEvent(run.RunID)
	if err != nil {
		return store.AgentRun{}, store.AgentRunEvent{}, err
	}
	if !exists {
		latest = store.AgentRunEvent{RunID: run.RunID, Kind: store.AgentEventStarted}
	}
	return run, latest, nil
}

// storeOverridePath resolves one command's --agent-store override. Every
// command shares agent.ResolveStorePath's inequality guard, so no override can
// point the Pi host at the application database.
func storeOverridePath(st *store.Store, run store.AgentRun, raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return run.StorePath, nil
	}
	resolved, err := agent.ResolveStorePath(raw, st.Path())
	if err != nil {
		return "", study.Refuse("invalid_input", "%s", err.Error())
	}
	return resolved, nil
}

// recordedStorePath resolves --agent-store for a command that addresses one
// recorded run. The recorded store is authoritative: a different path would
// read or delete another store's shared heartbeat and control files.
func recordedStorePath(st *store.Store, run store.AgentRun, raw string) (string, error) {
	storePath, err := storeOverridePath(st, run, raw)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(raw) != "" && storePath != run.StorePath {
		return "", study.Refuse("store_mismatch",
			"--agent-store %s is not the store recorded for run %s; the recorded Pi store is authoritative", storePath, run.RunID)
	}
	return storePath, nil
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
