package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/agent"
	"github.com/wxxb789/passquest/internal/store"
)

// TestAgentCancelInterruptedRunClearsLeakedHeartbeat pins the cancel rule when
// the host is already gone: a fresh heartbeat left by the dead host must not
// make cancellation wait for a signal or for the stale window.
func TestAgentCancelInterruptedRunClearsLeakedHeartbeat(t *testing.T) {
	temp := t.TempDir()
	db := filepath.Join(temp, "app.sqlite")
	storePath := filepath.Join(temp, "agent.sqlite")
	configureAgentProvider(t, db)
	runID := "run-20261110T100000Z-aaaaaaaa"
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	createAgentTestRunAt(t, st, runID, storePath, goldenBase.Add(-time.Minute),
		map[string]any{"host_token": strings.Repeat("a", 32)})
	if _, err := st.AppendAgentRunEvent(runID, store.AgentEventInterrupted,
		map[string]any{"reason": "timeout"}, goldenBase.Add(-30*time.Second)); err != nil {
		t.Fatalf("append interrupted: %v", err)
	}
	st.Close()
	raw, _ := json.Marshal(agent.Heartbeat{
		RunID: runID, PID: 0, Token: strings.Repeat("b", 32), At: goldenBase.UnixMilli(),
	})
	heartbeatPath := storePath + ".host"
	if err := os.WriteFile(heartbeatPath, raw, 0o600); err != nil {
		t.Fatalf("write leaked heartbeat: %v", err)
	}
	opt := agentTestOptions(t, &stepClock{base: goldenBase}, &agentFakeRunner{}, nil, agentTestKey)
	code, stdout, stderr := runAgentTest(t, opt, "",
		"agent", "cancel", runID, "--db", db, "--wait-seconds", "1", "--json")
	if code != 0 {
		t.Fatalf("cancel beside a dead host code=%d stderr=%s", code, stderr)
	}
	var cancelled agentCancelDoc
	if err := json.Unmarshal([]byte(stdout), &cancelled); err != nil {
		t.Fatalf("decode cancel: %v", err)
	}
	if !cancelled.Confirmed || cancelled.State != string(agent.StateCancelled) {
		t.Fatalf("cancel = %+v", cancelled)
	}
	for _, path := range []string{heartbeatPath, agent.HostControlPath(heartbeatPath)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s survived cancel: %v", path, err)
		}
	}
}

// TestAgentCancelRefusesStoreOverride pins that cancel reads the recorded
// heartbeat path. A different --agent-store must not be able to confirm a
// cancellation nobody delivered or clear another store's files.
func TestAgentCancelRefusesStoreOverride(t *testing.T) {
	temp := t.TempDir()
	db := filepath.Join(temp, "app.sqlite")
	storePath := filepath.Join(temp, "agent.sqlite")
	otherPath := filepath.Join(temp, "other.sqlite")
	runID := "run-20261110T100000Z-aaaaaaaa"
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	createAgentTestRunAt(t, st, runID, storePath, goldenBase, nil)
	st.Close()
	if err := os.WriteFile(otherPath, nil, 0o600); err != nil {
		t.Fatalf("write other store: %v", err)
	}
	heartbeatPath := otherPath + ".host"
	raw, _ := json.Marshal(agent.Heartbeat{
		RunID: "run-20261110T100000Z-bbbbbbbb", PID: 4242, Token: strings.Repeat("b", 32), At: goldenBase.UnixMilli(),
	})
	if err := os.WriteFile(heartbeatPath, raw, 0o600); err != nil {
		t.Fatalf("write other heartbeat: %v", err)
	}
	if err := agent.WriteHostControl(heartbeatPath, "run-20261110T100000Z-bbbbbbbb", strings.Repeat("b", 32), agent.HostControlCancel); err != nil {
		t.Fatalf("write other control: %v", err)
	}
	code, stdout, stderr := runAgentTest(t, agentTestOptions(t, &stepClock{base: goldenBase}, &agentFakeRunner{}, nil, agentTestKey), "",
		"agent", "cancel", runID, "--db", db, "--agent-store", otherPath, "--wait-seconds", "1", "--json")
	if code != 2 || !strings.Contains(stderr, "store_mismatch") {
		t.Fatalf("cancel with a rebound store code=%d stdout=%q stderr=%s", code, stdout, stderr)
	}
	for _, path := range []string{heartbeatPath, agent.HostControlPath(heartbeatPath)} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("other store file %s was removed: %v", path, err)
		}
	}
	st, err = store.Open(db)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	latest, _, err := st.LatestAgentRunEvent(runID)
	st.Close()
	if err != nil || latest.Kind != store.AgentEventStarted {
		t.Fatalf("refused cancel changed the trail: %+v err=%v", latest, err)
	}
}

// TestAgentCancelTerminalRunKeepsNewerRunFiles pins that cancelling an already
// cancelled run returns without touching the shared heartbeat and control files
// of a newer run on the same Pi store.
func TestAgentCancelTerminalRunKeepsNewerRunFiles(t *testing.T) {
	temp := t.TempDir()
	db := filepath.Join(temp, "app.sqlite")
	storePath := filepath.Join(temp, "agent.sqlite")
	runA := "run-20261110T100000Z-aaaaaaaa"
	runB := "run-20261110T100000Z-bbbbbbbb"
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	createAgentTestRunAt(t, st, runA, storePath, goldenBase, nil)
	if changed, err := st.AppendAgentRunTerminal(runA, store.AgentEventCancelled,
		map[string]any{"confirmed": true}, goldenBase); err != nil || !changed {
		t.Fatalf("cancel run A changed=%v err=%v", changed, err)
	}
	createAgentTestRunAt(t, st, runB, storePath, goldenBase.Add(time.Second),
		map[string]any{"host_token": strings.Repeat("b", 32)})
	st.Close()
	heartbeatPath := storePath + ".host"
	token := strings.Repeat("b", 32)
	raw, _ := json.Marshal(agent.Heartbeat{RunID: runB, PID: 4242, Token: token, At: goldenBase.UnixMilli()})
	if err := os.WriteFile(heartbeatPath, raw, 0o600); err != nil {
		t.Fatalf("write run B heartbeat: %v", err)
	}
	if err := agent.WriteHostControl(heartbeatPath, runB, token, agent.HostControlCancel); err != nil {
		t.Fatalf("write run B control: %v", err)
	}
	code, stdout, stderr := runAgentTest(t, agentTestOptions(t, &stepClock{base: goldenBase}, &agentFakeRunner{}, nil, agentTestKey), "",
		"agent", "cancel", runA, "--db", db, "--json")
	if code != 0 {
		t.Fatalf("idempotent cancel code=%d stdout=%q stderr=%s", code, stdout, stderr)
	}
	var cancelled agentCancelDoc
	if err := json.Unmarshal([]byte(stdout), &cancelled); err != nil {
		t.Fatalf("decode cancel: %v", err)
	}
	if !cancelled.Confirmed || cancelled.State != string(agent.StateCancelled) {
		t.Fatalf("cancel = %+v", cancelled)
	}
	beatRaw, err := os.ReadFile(heartbeatPath)
	if err != nil {
		t.Fatalf("run B heartbeat was removed: %v", err)
	}
	var heartbeat agent.Heartbeat
	if err := json.Unmarshal(beatRaw, &heartbeat); err != nil {
		t.Fatalf("decode run B heartbeat: %v", err)
	}
	if heartbeat.RunID != runB || heartbeat.PID != 4242 || heartbeat.Token != token {
		t.Fatalf("run B heartbeat = %+v", heartbeat)
	}
	controlRaw, err := os.ReadFile(agent.HostControlPath(heartbeatPath))
	if err != nil {
		t.Fatalf("run B control was removed: %v", err)
	}
	var control agent.HostControl
	if err := json.Unmarshal(controlRaw, &control); err != nil {
		t.Fatalf("decode run B control: %v", err)
	}
	if control.RunID != runB || control.HostToken != token || control.Action != agent.HostControlCancel {
		t.Fatalf("run B control = %+v", control)
	}
	st, err = store.Open(db)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	events, err := st.AgentRunEvents(runA)
	st.Close()
	if err != nil {
		t.Fatalf("read run A events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("run A events = %d, want started and cancelled only", len(events))
	}
}

func TestAgentCancelThenResumeRefused(t *testing.T) {
	db := newDB(t)
	configureAgentProvider(t, db)
	ids := &agentIDSequence{}
	interrupted := &agentFakeRunner{fn: func(agent.HostRequest) (agent.RunOutcome, error) {
		return agent.RunOutcome{Interrupted: true, InterruptReason: "host_exit", ExitCode: 70}, nil
	}}
	clock := &stepClock{base: goldenBase}
	code, stdout, _ := runAgentTest(t, agentTestOptions(t, clock, interrupted, ids, agentTestKey), "",
		"agent", "start", "--db", db, "--json")
	if code != 4 {
		t.Fatalf("start code=%d", code)
	}
	var paused agentRunDoc
	_ = json.Unmarshal([]byte(stdout), &paused)
	code, cancelRaw, stderr := runAgentTest(t, agentTestOptions(t, clock, interrupted, nil, agentTestKey), "",
		"agent", "cancel", paused.Run.RunID, "--db", db, "--json")
	if code != 0 {
		t.Fatalf("cancel code=%d stderr=%s", code, stderr)
	}
	var cancelled agentCancelDoc
	if err := json.Unmarshal([]byte(cancelRaw), &cancelled); err != nil {
		t.Fatalf("decode cancel: %v", err)
	}
	if !cancelled.Confirmed || cancelled.State != string(agent.StateCancelled) {
		t.Fatalf("cancel = %+v", cancelled)
	}
	code, _, stderr = runAgentTest(t, agentTestOptions(t, clock, interrupted, nil, agentTestKey), "",
		"agent", "resume", paused.Run.RunID, "--db", db, "--json")
	if code != 2 || !strings.Contains(stderr, "run_cancelled") {
		t.Fatalf("resume after cancel code=%d stderr=%s", code, stderr)
	}
	code, _, stderr = runAgentTest(t, agentTestOptions(t, clock, interrupted, nil, agentTestKey), "",
		"agent", "cancel", paused.Run.RunID, "--db", db, "--json")
	if code != 0 {
		t.Fatalf("idempotent cancel code=%d stderr=%s", code, stderr)
	}
}

func TestAgentCancelCompletedRunRefused(t *testing.T) {
	temp := t.TempDir()
	db := filepath.Join(temp, "app.sqlite")
	configureAgentProvider(t, db)
	ids := &agentIDSequence{}
	runner := completingAgentRunner(t, db, goldenBase.Add(10*time.Second))
	clock := &stepClock{base: goldenBase}
	code, stdout, _ := runAgentTest(t, agentTestOptions(t, clock, runner, ids, agentTestKey), "",
		"agent", "start", "--db", db, "--agent-store", filepath.Join(temp, "agent.sqlite"), "--json")
	if code != 0 {
		t.Fatalf("start code=%d", code)
	}
	var doc agentRunDoc
	_ = json.Unmarshal([]byte(stdout), &doc)
	code, _, stderr := runAgentTest(t, agentTestOptions(t, clock, runner, nil, agentTestKey), "",
		"agent", "cancel", doc.Run.RunID, "--db", db, "--json")
	if code != 2 || !strings.Contains(stderr, "run_completed") {
		t.Fatalf("cancel completed code=%d stderr=%s", code, stderr)
	}
}

func TestAgentCancelKeepsTerminalOffALiveHost(t *testing.T) {
	// Production waits one heartbeat window plus a second after a hard kill;
	// this test shortens only its own wait below.
	if got := (Options{}).withDefaults().agentKillGrace; got != 6*time.Second {
		t.Fatalf("production agent kill grace = %v, want 6s", got)
	}
	temp := t.TempDir()
	db := filepath.Join(temp, "app.sqlite")
	configureAgentProvider(t, db)
	storePath := filepath.Join(temp, "agent.sqlite")
	runID := "run-20261110T100000Z-aaaaaaaa"
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	createAgentTestRunAt(t, st, runID, storePath, time.Now().Add(-time.Minute), map[string]any{"host_token": strings.Repeat("a", 32)})
	st.Close()
	heartbeatPath := storePath + ".host"
	// A host keeps refreshing the heartbeat while its pid cannot be signalled
	// (0 is rejected before any OS call). cancel must still treat the run as
	// live and must not record a terminal outcome.
	stop := make(chan struct{})
	ready := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		publish := func() bool {
			raw, _ := json.Marshal(agent.Heartbeat{
				RunID: runID, PID: 0, Token: strings.Repeat("b", 32), At: time.Now().UnixMilli(),
			})
			if err := os.WriteFile(heartbeatPath+".tmp", raw, 0o600); err != nil {
				return false
			}
			if err := os.Rename(heartbeatPath+".tmp", heartbeatPath); err != nil {
				// A concurrent probe can hold the destination open on Windows,
				// so replacement fails with a sharing violation. Retry next tick;
				// the heartbeat stays fresh as long as one replacement wins.
				_ = os.Remove(heartbeatPath + ".tmp")
				return false
			}
			return true
		}
		first := true
		for {
			if publish() && first {
				close(ready)
				first = false
			}
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()
	<-ready
	opt := agentTestOptions(t, nil, &agentFakeRunner{}, nil, agentTestKey)
	opt.Now = time.Now
	// The heartbeat never goes stale here, so the full production grace would
	// only add wall time to the same unconfirmed outcome.
	opt.agentKillGrace = 500 * time.Millisecond
	code, stdout, stderr := runAgentTest(t, opt, "",
		"agent", "cancel", runID, "--db", db, "--wait-seconds", "1", "--json")
	if code != 1 || stdout != "" {
		t.Fatalf("cancel beside a live host code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	var doc agentCancelDoc
	if err := json.Unmarshal([]byte(stderr), &doc); err != nil {
		t.Fatalf("decode unconfirmed cancel: %v\n%s", err, stderr)
	}
	if doc.State != string(agent.StateCancelling) || doc.Confirmed {
		t.Fatalf("unconfirmed cancel = %+v", doc)
	}
	// The host-observed control request is written even though no OS signal can
	// reach pid 0. This is the path a Windows host relies on.
	controlPath := agent.HostControlPath(heartbeatPath)
	controlRaw, err := os.ReadFile(controlPath)
	if err != nil {
		t.Fatalf("read control request: %v", err)
	}
	var control agent.HostControl
	if err := json.Unmarshal(controlRaw, &control); err != nil {
		t.Fatalf("decode control request: %v", err)
	}
	if control.RunID != runID || control.Action != agent.HostControlCancel || control.HostToken != strings.Repeat("b", 32) {
		t.Fatalf("control request = %+v", control)
	}
	close(stop)
	<-done
	st, err = store.Open(db)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	latest, _, err := st.LatestAgentRunEvent(runID)
	st.Close()
	if err != nil || latest.Kind != store.AgentEventCancelRequested {
		t.Fatalf("latest event after an unconfirmed cancel = %+v err=%v", latest, err)
	}
	if err := os.Remove(heartbeatPath); err != nil {
		t.Fatalf("remove heartbeat: %v", err)
	}
	code, stdout, stderr = runAgentTest(t, opt, "",
		"agent", "cancel", runID, "--db", db, "--wait-seconds", "1", "--json")
	if code != 0 {
		t.Fatalf("confirmed cancel code=%d stderr=%s", code, stderr)
	}
	var confirmed agentCancelDoc
	if err := json.Unmarshal([]byte(stdout), &confirmed); err != nil {
		t.Fatalf("decode confirmed cancel: %v", err)
	}
	if !confirmed.Confirmed || confirmed.State != string(agent.StateCancelled) {
		t.Fatalf("confirmed cancel = %+v", confirmed)
	}
	if _, err := os.Stat(controlPath); !os.IsNotExist(err) {
		t.Fatalf("control request survived cancel: %v", err)
	}
}
