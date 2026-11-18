package brief

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/wxxb789/passquest/internal/agent"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
)

// cancel stops the addressed run's host and records the terminal outcome. A
// host that stays live past the wait is reported unconfirmed (cancelling);
// everything else is the confirmed cancellation.
func cancel(s *study.Service, in AdvanceInput) (Advanced, error) {
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
	if latest.Kind == store.AgentEventCompleted {
		return Advanced{}, study.Refuse("run_completed", "run %s already completed", run.RunID)
	}
	heartbeatPath := run.StorePath + ".host"
	if latest.Kind == store.AgentEventCancelled {
		// Cancellation is already terminal. Do not touch the shared heartbeat
		// or control file: a newer run on the same store owns them now.
		return Advanced{Run: run, State: agent.StateCancelled}, nil
	}
	// The latest claim recorded the token given to the host before that host
	// wrote a heartbeat. A live heartbeat is more precise when a host raced the
	// claim. The token keeps an old control request from stopping a later
	// resume, which always receives a new token.
	controlToken, err := agentLatestToken(st, run.RunID)
	if err != nil {
		return Advanced{}, err
	}
	heartbeat, heartbeatStatus := agent.ProbeHeartbeat(heartbeatPath, run.RunID, p.Now(), heartbeatWindow())
	live := heartbeatStatus.Live()
	if live && agent.HeartbeatAuthority(latest.Kind) {
		if heartbeat.Token != "" {
			controlToken = heartbeat.Token
		}
	} else {
		// A heartbeat left by a host that already exited (interrupted/failed)
		// is not a live host, so cancellation needs no wait.
		live = false
	}
	if latest.Kind != store.AgentEventCancelRequested && latest.Kind != store.AgentEventCancelled {
		info := map[string]any{"host_token": controlToken}
		if live {
			info["pid"] = heartbeat.PID
			// A live heartbeat names the host to signal even when the latest
			// event recorded another token; keep that host's token as
			// provenance in the cancellation request.
			if heartbeat.Token != "" {
				info["host_token"] = heartbeat.Token
			}
		}
		if _, err := st.AppendAgentRunEvent(run.RunID, store.AgentEventCancelRequested, info, p.Now()); err != nil {
			if errors.Is(err, store.ErrAgentRunFinished) {
				agent.RemoveHeartbeatForRun(heartbeatPath, run.RunID)
				agent.RemoveHostControlForRun(heartbeatPath, run.RunID, controlToken)
				latest, _, _ := st.LatestAgentRunEvent(run.RunID)
				if latest.Kind == store.AgentEventCompleted {
					return Advanced{}, study.Refuse("run_completed", "run %s already completed", run.RunID)
				}
				return Advanced{Run: run, State: agent.StateCancelled}, nil
			}
			return Advanced{}, err
		}
	}
	// The host polls this file itself, so the request reaches a Windows host
	// that cannot receive SIGINT. Signalling below is only an optimisation.
	_ = agent.WriteHostControl(heartbeatPath, run.RunID, controlToken, agent.HostControlCancel)

	heartbeat, live = agentHostClaimWait(s, heartbeatPath, run.RunID, latest)
	confirmed := true
	if live {
		confirmed = false
		_ = agent.SignalCancelPID(heartbeat.PID)
		deadline := time.Now().Add(time.Duration(in.WaitSeconds) * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
			if _, still := agent.ProbeHeartbeat(heartbeatPath, run.RunID, p.Now(), heartbeatWindow()); !still.Live() {
				confirmed = true
				break
			}
		}
		if !confirmed {
			_ = agent.SignalKillPID(heartbeat.PID)
			// The heartbeat is only stale after its window. A killed process
			// must be given that window before cancellation is called
			// unconfirmed.
			killDeadline := time.Now().Add(p.KillGrace)
			for time.Now().Before(killDeadline) {
				time.Sleep(100 * time.Millisecond)
				if _, still := agent.ProbeHeartbeat(heartbeatPath, run.RunID, p.Now(), heartbeatWindow()); !still.Live() {
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
		return Advanced{Run: run, State: agent.StateCancelling, HeartbeatPID: heartbeat.PID}, nil
	}
	detail := map[string]any{"confirmed": true}
	if heartbeat.PID > 0 {
		detail["pid"] = heartbeat.PID
	}
	changed, err := st.AppendAgentRunTerminal(run.RunID, store.AgentEventCancelled, detail, p.Now())
	if err != nil {
		return Advanced{}, err
	}
	// The host is gone in both terminal paths, including when another process
	// recorded cancelled first. A Windows hard kill cannot remove these files
	// itself, so this process clears its own run's files. It must not delete a
	// newer run's shared heartbeat or control file.
	agent.RemoveHeartbeatForRun(heartbeatPath, run.RunID)
	agent.RemoveHostControlForRun(heartbeatPath, run.RunID, controlToken)
	if !changed {
		latest, _, _ := st.LatestAgentRunEvent(run.RunID)
		if latest.Kind == store.AgentEventCompleted {
			return Advanced{}, study.Refuse("run_completed", "run %s already completed", run.RunID)
		}
		return Advanced{Run: run, State: agent.StateCancelled, HeartbeatPID: heartbeat.PID}, nil
	}
	return Advanced{Run: run, State: agent.StateCancelled, HeartbeatPID: heartbeat.PID}, nil
}

// agentHostClaimWait gives a start/resume claim younger than the heartbeat
// window a chance to publish its first heartbeat. A cancel that finds no
// heartbeat for such a claim must not record a terminal outcome beside a host
// that is still starting; it either observes the heartbeat or waits until the
// claim can no longer be starting.
func agentHostClaimWait(s *study.Service, path, runID string, latest store.AgentRunEvent) (agent.Heartbeat, bool) {
	p := kernel.P(s)
	heartbeat, status := agent.ProbeHeartbeat(path, runID, p.Now(), heartbeatWindow())
	live := status.Live()
	if live && !agent.HeartbeatAuthority(latest.Kind) {
		// The trail proves the host has gone; a heartbeat it left behind must
		// not make cancellation wait or signal a dead pid.
		live = false
	}
	if live || latest.At.IsZero() ||
		(latest.Kind != store.AgentEventStarted && latest.Kind != store.AgentEventResumed) {
		return heartbeat, live
	}
	age := p.Now().Sub(latest.At)
	if age >= heartbeatWindow() {
		return heartbeat, false
	}
	deadline := time.Now().Add(heartbeatWindow() - age)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		if heartbeat, status = agent.ProbeHeartbeat(path, runID, p.Now(), heartbeatWindow()); status.Live() {
			return heartbeat, true
		}
	}
	return heartbeat, false
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
