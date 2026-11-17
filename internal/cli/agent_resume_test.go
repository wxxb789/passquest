package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/agent"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/tutor"
)

func TestAgentPausedThenResume(t *testing.T) {
	db := newDB(t)
	configureAgentProvider(t, db)
	ids := &agentIDSequence{}
	interrupted := &agentFakeRunner{fn: func(request agent.HostRequest) (agent.RunOutcome, error) {
		return agent.RunOutcome{Interrupted: true, InterruptReason: "timeout", ExitCode: 70}, nil
	}}
	clock := &stepClock{base: goldenBase}
	code, stdout, stderr := runAgentTest(t, agentTestOptions(t, clock, interrupted, ids, agentTestKey), "",
		"agent", "start", "--db", db, "--agent-store", filepath.Join(t.TempDir(), "agent.sqlite"), "--json")
	if code != 4 {
		t.Fatalf("paused code=%d stderr=%s", code, stderr)
	}
	var paused agentRunDoc
	if err := json.Unmarshal([]byte(stdout), &paused); err != nil {
		t.Fatalf("decode paused: %v", err)
	}
	if paused.State != string(agent.StatePaused) || paused.Status.Reason != "timeout" {
		t.Fatalf("paused = %+v", paused)
	}
	code, _, stderr = runAgentTest(t, agentTestOptions(t, clock, interrupted, &agentIDSequence{next: 1}, agentTestKey), "",
		"agent", "start", "--db", db, "--json")
	if code != 2 || !strings.Contains(stderr, "agent_busy") {
		t.Fatalf("second start while paused code=%d stderr=%s", code, stderr)
	}
	unavailable := &agentFakeRunner{fn: func(agent.HostRequest) (agent.RunOutcome, error) {
		return agent.RunOutcome{}, &agent.UnavailableError{Err: fmt.Errorf("node missing")}
	}}
	code, statusRaw, _ := runAgentTest(t, agentTestOptions(t, clock, unavailable, nil, agentTestKey), "",
		"agent", "status", paused.Run.RunID, "--db", db, "--json")
	if code != 0 || !strings.Contains(statusRaw, `"state": "paused"`) {
		t.Fatalf("paused status code=%d raw=%s", code, statusRaw)
	}
	if err := os.Remove(paused.Run.Store); err != nil {
		t.Fatalf("remove store: %v", err)
	}
	code, humanStatus, _ := runAgentTest(t, agentTestOptions(t, clock, unavailable, nil, agentTestKey), "",
		"agent", "status", paused.Run.RunID, "--db", db)
	if code != 0 || !strings.Contains(humanStatus, "store missing") {
		t.Fatalf("missing-store human status code=%d output=%s", code, humanStatus)
	}

	// A later configuration must not change the resumed model: the run keeps
	// its recorded model and picks up the current endpoint (Q-0006-05).
	newEndpoint := "https://second.example.com/v1/chat/completions"
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := st.SaveTutorConfig(tutor.Config{
		Endpoint: newEndpoint, Model: "other-model",
		TimeoutMS: 45000, MaxOutputTokens: 400, DailyCalls: 20,
	}, goldenBase); err != nil {
		t.Fatalf("save second tutor config: %v", err)
	}
	st.Close()

	completing := completingAgentRunner(t, db, goldenBase.Add(20*time.Second))
	code, resumeRaw, stderr := runAgentTest(t, agentTestOptions(t, clock, completing, nil, agentTestKey), "",
		"agent", "resume", paused.Run.RunID, "--db", db, "--json")
	if code != 0 {
		t.Fatalf("resume code=%d stderr=%s", code, stderr)
	}
	var resumed agentRunDoc
	if err := json.Unmarshal([]byte(resumeRaw), &resumed); err != nil {
		t.Fatalf("decode resumed: %v", err)
	}
	if resumed.State != string(agent.StateCompleted) || len(resumed.Steps) != 2 || resumed.Run.RunID != paused.Run.RunID {
		t.Fatalf("resumed = %+v", resumed)
	}
	got := completing.requestAt(0)
	if got.Model == nil || got.Model.Model != paused.Run.Model || got.Model.Endpoint != newEndpoint {
		t.Fatalf("resume request model = %+v, want recorded %q at %q", got.Model, paused.Run.Model, newEndpoint)
	}
	code, _, stderr = runAgentTest(t, agentTestOptions(t, clock, completing, nil, agentTestKey), "",
		"agent", "resume", paused.Run.RunID, "--db", db, "--json")
	if code != 2 || !strings.Contains(stderr, "run_completed") {
		t.Fatalf("second resume code=%d stderr=%s", code, stderr)
	}
}

// TestAgentInterruptedHeartbeatDoesNotBlockStatusOrResume reproduces the
// Windows timeout shape on Linux: the host is killed before it can remove its
// heartbeat, so a fresh liveness file survives beside a recorded interruption.
// The trail must decide liveness on every OS.
func TestAgentInterruptedHeartbeatDoesNotBlockStatusOrResume(t *testing.T) {
	temp := t.TempDir()
	db := filepath.Join(temp, "app.sqlite")
	storePath := filepath.Join(temp, "agent.sqlite")
	configureAgentProvider(t, db)
	clock := &stepClock{base: goldenBase}
	leaked := &agentFakeRunner{fn: func(request agent.HostRequest) (agent.RunOutcome, error) {
		raw, _ := json.Marshal(agent.Heartbeat{
			RunID: request.RunID, PID: 4242, Token: request.HostToken, At: clock.Now().UnixMilli(),
		})
		if err := os.WriteFile(request.HeartbeatPath, raw, 0o600); err != nil {
			t.Fatalf("write leaked heartbeat: %v", err)
		}
		return agent.RunOutcome{Interrupted: true, InterruptReason: "timeout", ExitCode: 1}, nil
	}}
	code, stdout, stderr := runAgentTest(t, agentTestOptions(t, clock, leaked, &agentIDSequence{}, agentTestKey), "",
		"agent", "start", "--db", db, "--agent-store", storePath, "--json")
	if code != 4 {
		t.Fatalf("start code=%d stderr=%s", code, stderr)
	}
	var paused agentRunDoc
	if err := json.Unmarshal([]byte(stdout), &paused); err != nil {
		t.Fatalf("decode paused: %v", err)
	}
	runID := paused.Run.RunID
	// The application reaped the host, so it must clear the liveness file the
	// killed host could not remove.
	if _, err := os.Stat(storePath + ".host"); !os.IsNotExist(err) {
		t.Fatalf("interrupted host left a heartbeat after exit: %v", err)
	}
	// Recreate the leak as if the application had also crashed before cleanup.
	raw, _ := json.Marshal(agent.Heartbeat{
		RunID: runID, PID: 4242, Token: strings.Repeat("a", 32), At: clock.Now().UnixMilli(),
	})
	if err := os.WriteFile(storePath+".host", raw, 0o600); err != nil {
		t.Fatalf("recreate leaked heartbeat: %v", err)
	}
	status := statusAgentRunner(&agent.Snapshot{Found: true, LiveRun: true, PendingTasks: 1})
	code, statusRaw, stderr := runAgentTest(t, agentTestOptions(t, clock, status, nil, agentTestKey), "",
		"agent", "status", runID, "--db", db, "--json")
	if code != 0 || !strings.Contains(statusRaw, `"state": "paused"`) ||
		!strings.Contains(statusRaw, `"heartbeat_fresh": false`) {
		t.Fatalf("status beside a leaked heartbeat code=%d raw=%s stderr=%s", code, statusRaw, stderr)
	}
	completing := completingAgentRunner(t, db, goldenBase.Add(20*time.Second))
	code, resumeRaw, stderr := runAgentTest(t, agentTestOptions(t, clock, completing, nil, agentTestKey), "",
		"agent", "resume", runID, "--db", db, "--json")
	if code != 0 {
		t.Fatalf("resume beside a leaked heartbeat code=%d stderr=%s", code, stderr)
	}
	var resumed agentRunDoc
	if err := json.Unmarshal([]byte(resumeRaw), &resumed); err != nil {
		t.Fatalf("decode resumed: %v", err)
	}
	if resumed.State != string(agent.StateCompleted) || resumed.Run.RunID != runID || len(resumed.Steps) != 2 {
		t.Fatalf("resumed = %+v", resumed)
	}
	if _, err := os.Stat(storePath + ".host"); !os.IsNotExist(err) {
		t.Fatalf("heartbeat survived the resume: %v", err)
	}
}

func TestAgentResumeRefusesLiveHost(t *testing.T) {
	temp := t.TempDir()
	db := filepath.Join(temp, "app.sqlite")
	configureAgentProvider(t, db)
	storePath := filepath.Join(temp, "agent.sqlite")
	runID := "run-20261110T100000Z-aaaaaaaa"
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	// The claim is old enough to be hostless; the live heartbeat carries a
	// token the trail never recorded, which must still count as a live host.
	createAgentTestRunAt(t, st, runID, storePath, goldenBase.Add(-time.Hour), map[string]any{"host_token": strings.Repeat("a", 32)})
	st.Close()
	raw, err := json.Marshal(agent.Heartbeat{
		RunID: runID, PID: 4242, Token: strings.Repeat("b", 32), At: goldenBase.UnixMilli(),
	})
	if err != nil {
		t.Fatalf("encode heartbeat: %v", err)
	}
	if err := os.WriteFile(storePath+".host", raw, 0o600); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}
	runner := &agentFakeRunner{}
	code, stdout, stderr := runAgentTest(t, agentTestOptions(t, &stepClock{base: goldenBase}, runner, nil, agentTestKey), "",
		"agent", "resume", runID, "--db", db, "--json")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "run_running") {
		t.Fatalf("resume with a live host code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if runner.count() != 0 {
		t.Fatal("resume started a second host beside a live heartbeat")
	}
	st, err = store.Open(db)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer st.Close()
	events, err := st.AgentRunEvents(runID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	for _, event := range events {
		if event.Kind == store.AgentEventResumed {
			t.Fatalf("refused resume appended %+v", event)
		}
	}
}

func TestAgentResumeStoreOverrideGuards(t *testing.T) {
	t.Parallel()
	temp := t.TempDir()
	db := filepath.Join(temp, "app.sqlite")
	configureAgentProvider(t, db)
	storePath := filepath.Join(temp, "agent.sqlite")
	runID := "run-20261110T100000Z-aaaaaaaa"
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	createAgentTestRunAt(t, st, runID, storePath, goldenBase.Add(-time.Hour), nil)
	st.Close()
	runner := &agentFakeRunner{}
	opt := agentTestOptions(t, &stepClock{base: goldenBase}, runner, nil, agentTestKey)
	for _, command := range []string{"resume", "status"} {
		code, _, stderr := runAgentTest(t, opt, "",
			"agent", command, runID, "--db", db, "--agent-store", db, "--json")
		if code != 2 || !strings.Contains(stderr, "must differ") {
			t.Fatalf("%s with the application database as store code=%d stderr=%s", command, code, stderr)
		}
	}
	code, _, stderr := runAgentTest(t, opt, "",
		"agent", "resume", runID, "--db", db, "--agent-store", filepath.Join(temp, "other.sqlite"), "--json")
	if code != 2 || !strings.Contains(stderr, "store_mismatch") {
		t.Fatalf("resume with a rebound store code=%d stderr=%s", code, stderr)
	}
	if runner.count() != 0 {
		t.Fatal("a refused store override reached the host")
	}
}

func TestAgentResumeRefusesUnconsummatedCancellation(t *testing.T) {
	db := newDB(t)
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	runID := "run-20261110T100000Z-aaaaaaaa"
	createAgentTestRun(t, st, runID)
	if _, err := st.AppendAgentRunEvent(runID, store.AgentEventCancelRequested, map[string]any{"pid": 4}, goldenBase); err != nil {
		t.Fatalf("cancel request: %v", err)
	}
	// A racing run process can append an interruption after the cancellation
	// request. Resume must read the trail, not only the latest event.
	if _, err := st.AppendAgentRunEvent(runID, store.AgentEventInterrupted,
		map[string]any{"reason": "host_exit"}, goldenBase.Add(time.Second)); err != nil {
		t.Fatalf("late interruption: %v", err)
	}
	st.Close()
	code, _, stderr := runAgentTest(t, agentTestOptions(t, nil, &agentFakeRunner{}, nil, agentTestKey), "",
		"agent", "resume", runID, "--db", db, "--json")
	if code != 2 || !strings.Contains(stderr, "cancel_pending") {
		t.Fatalf("resume with a pending cancel code=%d stderr=%s", code, stderr)
	}
}

func TestAgentRecordStepValidationAndDuplicate(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	runID := "run-20261110T100000Z-aaaaaaaa"
	createAgentTestRun(t, st, runID)
	st.Close()
	opt := agentTestOptions(t, &stepClock{base: goldenBase}, &agentFakeRunner{}, nil, agentTestKey)
	stepJSON := func(index int, title string) string {
		return fmt.Sprintf(`{"run_id":%q,"attempt_key":"call-1","index":%d,"kind":"diagnosis","title":%q,"body":"body"}`, runID, index, title)
	}
	code, stdout, stderr := runAgentTest(t, opt, stepJSON(1, "First"), "agent", "record-step", "--db", db, "--json")
	if code != 0 {
		t.Fatalf("record step code=%d stderr=%s", code, stderr)
	}
	var recorded agentStepDoc
	if err := json.Unmarshal([]byte(stdout), &recorded); err != nil || recorded.Duplicate || recorded.Attempts != 1 {
		t.Fatalf("recorded = %+v err=%v", recorded, err)
	}
	code, stdout, _ = runAgentTest(t, opt, stepJSON(1, "Different"), "agent", "record-step", "--db", db, "--json")
	if code != 0 {
		t.Fatalf("duplicate code=%d", code)
	}
	var duplicate agentStepDoc
	if err := json.Unmarshal([]byte(stdout), &duplicate); err != nil || !duplicate.Duplicate || duplicate.Attempts != 2 {
		t.Fatalf("duplicate = %+v err=%v", duplicate, err)
	}
	if duplicate.Step.Title != "First" {
		t.Fatalf("duplicate overwrote the title: %+v", duplicate.Step)
	}
	code, _, _ = runAgentTest(t, opt, stepJSON(4, "Too far"), "agent", "record-step", "--db", db, "--json")
	if code != 2 {
		t.Fatalf("index 4 code=%d", code)
	}
	code, _, _ = runAgentTest(t, opt, `{"run_id":"run-20261110T100000Z-bbbbbbbb","attempt_key":"x","index":1,"kind":"diagnosis","title":"t","body":"b"}`,
		"agent", "record-step", "--db", db, "--json")
	if code != 2 {
		t.Fatalf("unknown run code=%d", code)
	}
	st, _ = store.Open(db)
	if _, err := st.AppendAgentRunEvent(runID, store.AgentEventCancelRequested, nil, goldenBase); err != nil {
		t.Fatalf("cancel request: %v", err)
	}
	st.Close()
	code, stdout, _ = runAgentTest(t, opt, stepJSON(2, "Cancelled"), "agent", "record-step", "--db", db, "--json")
	if code != 2 || !strings.Contains(stdout, "run_cancelled") {
		t.Fatalf("cancelled step code=%d stdout=%s", code, stdout)
	}
	st, _ = store.Open(db)
	if _, err := st.AppendAgentRunTerminal(runID, store.AgentEventCancelled, map[string]any{"confirmed": true}, goldenBase); err != nil {
		t.Fatalf("cancel run: %v", err)
	}
	completedID := "run-20261110T100000Z-cccccccc"
	createAgentTestRun(t, st, completedID)
	if _, err := st.AppendAgentRunTerminal(completedID, store.AgentEventCompleted, map[string]any{"steps": 1}, goldenBase); err != nil {
		t.Fatalf("complete run: %v", err)
	}
	st.Close()
	code, stdout, _ = runAgentTest(t, opt,
		fmt.Sprintf(`{"run_id":%q,"attempt_key":"call-1","index":1,"kind":"diagnosis","title":"t","body":"b"}`, completedID),
		"agent", "record-step", "--db", db, "--json")
	if code != 2 || !strings.Contains(stdout, "run_completed") {
		t.Fatalf("completed step code=%d stdout=%s", code, stdout)
	}
}
