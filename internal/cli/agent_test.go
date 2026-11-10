package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/wxxb789/passquest/internal/agent"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/tutor"
)

const agentTestKey = "test-key-not-real"

type agentFakeRunner struct {
	mu    sync.Mutex
	calls []agent.HostRequest
	fn    func(agent.HostRequest) (agent.RunOutcome, error)
}

func (f *agentFakeRunner) Run(_ context.Context, request agent.HostRequest) (agent.RunOutcome, error) {
	f.mu.Lock()
	f.calls = append(f.calls, request)
	f.mu.Unlock()
	if f.fn == nil {
		return agent.RunOutcome{}, fmt.Errorf("unexpected host call in %s mode", request.Mode)
	}
	return f.fn(request)
}

func (f *agentFakeRunner) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *agentFakeRunner) requestAt(index int) agent.HostRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[index]
}

type agentIDSequence struct{ next int }

func (s *agentIDSequence) New() (string, error) {
	s.next++
	return fmt.Sprintf("run-20261110T100000Z-%08x", s.next), nil
}

func configureAgentProvider(t *testing.T, db string) {
	t.Helper()
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	cfg := tutor.Config{
		Endpoint: "https://api.example.com/v1/chat/completions", Model: "example-model",
		TimeoutMS: 30000, MaxOutputTokens: 350, DailyCalls: 20,
	}
	if err := st.SaveTutorConfig(cfg, goldenBase); err != nil {
		t.Fatalf("save tutor config: %v", err)
	}
}

func agentTestOptions(t *testing.T, clock *stepClock, runner agent.Runner, ids *agentIDSequence, key string) Options {
	t.Helper()
	if clock == nil {
		clock = &stepClock{base: goldenBase}
	}
	if ids == nil {
		ids = &agentIDSequence{}
	}
	return Options{
		Stdin:      strings.NewReader(""),
		Now:        clock.Now,
		NewSession: func() string { return "sess-test" },
		Getenv: func(name string) string {
			if name == agent.CredentialEnv {
				return key
			}
			return ""
		},
		AgentRunner:  runner,
		NewRunID:     ids.New,
		NewHostToken: func() (string, error) { return strings.Repeat("a", 32), nil },
	}
}

func runAgentTest(t *testing.T, opt Options, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	opt.Stdin = strings.NewReader(stdin)
	opt.Stdout = &stdout
	opt.Stderr = &stderr
	code := Run(args, opt)
	return code, stdout.String(), stderr.String()
}

func completingAgentRunner(t *testing.T, db string, at time.Time) *agentFakeRunner {
	t.Helper()
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store for fake runner: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	var once sync.Once
	return &agentFakeRunner{fn: func(request agent.HostRequest) (agent.RunOutcome, error) {
		if request.Mode != agent.ModeStart && request.Mode != agent.ModeResume {
			return agent.RunOutcome{}, fmt.Errorf("unexpected mode %s", request.Mode)
		}
		once.Do(func() {
			if _, _, _, err := st.RecordAgentStep(request.RunID, 1, store.AgentStepDiagnosis,
				"Check the polarity cue", "Mark NOT before comparing options.", "call-1", at); err != nil {
				t.Fatalf("record step 1: %v", err)
			}
			if _, _, _, err := st.RecordAgentStep(request.RunID, 2, store.AgentStepTactic,
				"Apply the boundary", "A word match is not evidence; check the negation.", "call-2", at.Add(time.Second)); err != nil {
				t.Fatalf("record step 2: %v", err)
			}
		})
		answer := "Two advisory steps recorded."
		return agent.RunOutcome{Response: agent.HostResponse{
			SchemaVersion: agent.ProtocolVersion, State: "completed", RunID: request.RunID,
			ConversationID: 3, Answer: answer, AnswerChars: utf8.RuneCountInString(answer),
			Turns: 3, ToolCalls: 2, ToolErrors: 0,
			Snapshot: &agent.Snapshot{Found: true, ConversationID: 3, AnswerChars: utf8.RuneCountInString(answer),
				Entries: 7, Turns: 3, ToolCalls: 2},
		}}, nil
	}}
}

func createAgentTestRun(t *testing.T, st *store.Store, runID string) {
	t.Helper()
	createAgentTestRunAt(t, st, runID, filepath.Join(t.TempDir(), "agent.sqlite"), goldenBase, nil)
}

func createAgentTestRunAt(t *testing.T, st *store.Store, runID, storePath string, at time.Time, detail any) {
	t.Helper()
	if err := st.CreateAgentRun(store.AgentRun{
		RunID: runID, TaskKind: agent.TaskStudyBrief, PromptVersion: agent.PromptVersion,
		Model: "example-model", StorePath: storePath,
		MaxSteps: 3, MaxTurns: 8, CreatedAt: at,
	}, detail, at); err != nil {
		t.Fatalf("create run: %v", err)
	}
}

func statusAgentRunner(snapshot *agent.Snapshot) *agentFakeRunner {
	return &agentFakeRunner{fn: func(request agent.HostRequest) (agent.RunOutcome, error) {
		return agent.RunOutcome{Response: agent.HostResponse{
			SchemaVersion: agent.ProtocolVersion, State: "status", RunID: request.RunID,
			ConversationID: 3, Snapshot: snapshot,
		}}, nil
	}}
}

func normalizedAgentJSON(t *testing.T, raw, temp string) []byte {
	t.Helper()
	text := strings.ReplaceAll(raw, temp, "<tmp>")
	var value any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		t.Fatalf("decode JSON %q: %v", text, err)
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("encode JSON: %v", err)
	}
	return append(encoded, '\n')
}

func TestAgentStartRequiresProvider(t *testing.T) {
	db := newDB(t)
	runner := &agentFakeRunner{}
	code, stdout, stderr := runAgentTest(t, agentTestOptions(t, nil, runner, nil, agentTestKey), "",
		"agent", "start", "--db", db, "--json")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "not_configured") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if runner.count() != 0 {
		t.Fatal("host was called without a provider")
	}
}

func TestAgentStartMissingCredentialRecordsNeedsAction(t *testing.T) {
	db := newDB(t)
	configureAgentProvider(t, db)
	runner := &agentFakeRunner{}
	ids := &agentIDSequence{}
	code, stdout, stderr := runAgentTest(t, agentTestOptions(t, nil, runner, ids, ""), "",
		"agent", "start", "--db", db, "--json")
	if code != 3 || stdout != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	var failure agentFailureDoc
	if err := json.Unmarshal([]byte(stderr), &failure); err != nil {
		t.Fatalf("decode failure: %v\n%s", err, stderr)
	}
	if failure.Error.Code != agent.CodeMissingCredential || failure.Status.State != string(agent.StateNeedsAction) {
		t.Fatalf("failure = %+v", failure)
	}
	if runner.count() != 0 {
		t.Fatal("host was called without a credential")
	}
	statusRunner := &agentFakeRunner{fn: func(agent.HostRequest) (agent.RunOutcome, error) {
		return agent.RunOutcome{}, &agent.UnavailableError{Err: fmt.Errorf("node missing")}
	}}
	code, stdout, _ = runAgentTest(t, agentTestOptions(t, nil, statusRunner, nil, ""), "",
		"agent", "status", "--db", db, "--json")
	if code != 0 {
		t.Fatalf("status code=%d", code)
	}
	var status agentStatusDoc
	if err := json.Unmarshal([]byte(stdout), &status); err != nil {
		t.Fatalf("decode status: %v\n%s", err, stdout)
	}
	if status.State != string(agent.StateNeedsAction) || status.Durable.Available {
		t.Fatalf("status = %+v", status)
	}
}

func TestAgentStartCompletesRecordsStepsAndKeepsAttempts(t *testing.T) {
	temp := t.TempDir()
	db := filepath.Join(temp, "app.sqlite")
	configureAgentProvider(t, db)
	ids := &agentIDSequence{}
	runner := completingAgentRunner(t, db, goldenBase.Add(10*time.Second))
	clock := &stepClock{base: goldenBase}
	code, stdout, stderr := runAgentTest(t, agentTestOptions(t, clock, runner, ids, agentTestKey), "",
		"agent", "start", "--db", db, "--agent-store", filepath.Join(temp, "agent.sqlite"), "--json")
	if code != 0 {
		t.Fatalf("start code=%d stderr=%s", code, stderr)
	}
	var doc agentRunDoc
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode start: %v\n%s", err, stdout)
	}
	if doc.State != string(agent.StateCompleted) || len(doc.Steps) != 2 || doc.Summary == nil {
		t.Fatalf("doc = %+v", doc)
	}
	if doc.Run.Store != filepath.Join(temp, "agent.sqlite") {
		t.Fatalf("store = %q", doc.Run.Store)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	steps, _ := st.AgentSteps(doc.Run.RunID)
	attempts, _ := st.AgentStepAttempts(doc.Run.RunID)
	if len(steps) != 2 || len(attempts) != 2 {
		t.Fatalf("steps=%d attempts=%d", len(steps), len(attempts))
	}
	if rows, _ := st.List(); len(rows) != 0 {
		t.Fatalf("agent run appended attempts: %+v", rows)
	}
	checkGolden(t, goldenPath(t, "agent-start.golden.json"), normalizedAgentJSON(t, stdout, temp))
}

func TestAgentStatusAndShowGoldenDocuments(t *testing.T) {
	temp := t.TempDir()
	db := filepath.Join(temp, "app.sqlite")
	configureAgentProvider(t, db)
	ids := &agentIDSequence{}
	runner := completingAgentRunner(t, db, goldenBase.Add(10*time.Second))
	clock := &stepClock{base: goldenBase}
	_, stdout, _ := runAgentTest(t, agentTestOptions(t, clock, runner, ids, agentTestKey), "",
		"agent", "start", "--db", db, "--agent-store", filepath.Join(temp, "agent.sqlite"), "--json")
	var started agentRunDoc
	_ = json.Unmarshal([]byte(stdout), &started)
	snapshot := &agent.Snapshot{Found: true, ConversationID: 3, AnswerChars: 28, Entries: 7, Turns: 3, ToolCalls: 2}
	code, statusRaw, stderr := runAgentTest(t, agentTestOptions(t, clock, statusAgentRunner(snapshot), nil, agentTestKey), "",
		"agent", "status", started.Run.RunID, "--db", db, "--json")
	if code != 0 {
		t.Fatalf("status code=%d stderr=%s", code, stderr)
	}
	var status agentStatusDoc
	if err := json.Unmarshal([]byte(statusRaw), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.State != string(agent.StateCompleted) || status.LatestEvent != store.AgentEventCompleted {
		t.Fatalf("status = %+v", status)
	}
	checkGolden(t, goldenPath(t, "agent-status.golden.json"), normalizedAgentJSON(t, statusRaw, temp))

	code, showRaw, stderr := runAgentTest(t, agentTestOptions(t, clock, runner, nil, agentTestKey), "",
		"agent", "show", started.Run.RunID, "--db", db, "--json")
	if code != 0 {
		t.Fatalf("show code=%d stderr=%s", code, stderr)
	}
	var show agentShowDoc
	if err := json.Unmarshal([]byte(showRaw), &show); err != nil {
		t.Fatalf("decode show: %v", err)
	}
	if show.Effects.Steps != 2 || show.Effects.Attempts != 2 || show.TrailState != string(agent.StateCompleted) ||
		show.LatestEvent != store.AgentEventCompleted {
		t.Fatalf("show = %+v", show)
	}
	checkGolden(t, goldenPath(t, "agent-show.golden.json"), normalizedAgentJSON(t, showRaw, temp))

	code, runsRaw, stderr := runAgentTest(t, agentTestOptions(t, clock, runner, nil, agentTestKey), "",
		"agent", "runs", "--db", db, "--json")
	if code != 0 {
		t.Fatalf("runs code=%d stderr=%s", code, stderr)
	}
	checkGolden(t, goldenPath(t, "agent-runs.golden.json"), normalizedAgentJSON(t, runsRaw, temp))
}

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

func TestAgentCancelKeepsTerminalOffALiveHost(t *testing.T) {
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
		first := true
		for {
			raw, _ := json.Marshal(agent.Heartbeat{
				RunID: runID, PID: 0, Token: strings.Repeat("b", 32), At: time.Now().UnixMilli(),
			})
			_ = os.WriteFile(heartbeatPath+".tmp", raw, 0o600)
			_ = os.Rename(heartbeatPath+".tmp", heartbeatPath)
			if first {
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

func TestAgentShowReportsTrailStateNotLiveState(t *testing.T) {
	temp := t.TempDir()
	db := filepath.Join(temp, "app.sqlite")
	storePath := filepath.Join(temp, "agent.sqlite")
	runID := "run-20261110T100000Z-aaaaaaaa"
	if err := os.WriteFile(storePath, []byte("pi fixture"), 0o600); err != nil {
		t.Fatalf("write store fixture: %v", err)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	createAgentTestRunAt(t, st, runID, storePath, goldenBase.Add(-time.Hour), nil)
	if _, _, _, err := st.RecordAgentStep(runID, 1, store.AgentStepDiagnosis, "Title", "Body", "call-1", goldenBase); err != nil {
		t.Fatalf("record step: %v", err)
	}
	// Crash window: the host answered but the CLI died before recording the
	// completed event.
	if _, err := st.AppendAgentRunEvent(runID, store.AgentEventInterrupted, map[string]any{"reason": "host_exit"}, goldenBase); err != nil {
		t.Fatalf("interrupt run: %v", err)
	}
	st.Close()
	runner := statusAgentRunner(&agent.Snapshot{Found: true, ConversationID: 3, AnswerChars: 40, Entries: 7})
	opt := agentTestOptions(t, &stepClock{base: goldenBase}, runner, nil, agentTestKey)
	code, statusRaw, stderr := runAgentTest(t, opt, "", "agent", "status", runID, "--db", db, "--json")
	if code != 0 {
		t.Fatalf("status code=%d stderr=%s", code, stderr)
	}
	var status agentStatusDoc
	if err := json.Unmarshal([]byte(statusRaw), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.State != string(agent.StateCompleted) || status.LatestEvent != store.AgentEventInterrupted {
		t.Fatalf("status = %+v", status)
	}
	code, showRaw, stderr := runAgentTest(t, opt, "", "agent", "show", runID, "--db", db, "--json")
	if code != 0 {
		t.Fatalf("show code=%d stderr=%s", code, stderr)
	}
	var show agentShowDoc
	if err := json.Unmarshal([]byte(showRaw), &show); err != nil {
		t.Fatalf("decode show: %v", err)
	}
	if show.TrailState != string(agent.StatePaused) || show.LatestEvent != store.AgentEventInterrupted {
		t.Fatalf("show trail state = %q latest=%q", show.TrailState, show.LatestEvent)
	}
}

func TestAgentLatestTokenFailsClosedOnAReadError(t *testing.T) {
	db := newDB(t)
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	if _, err := agentLatestToken(st, "run-20261110T100000Z-aaaaaaaa"); err == nil {
		t.Fatal("a closed store returned no error")
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

func TestAgentShowReportsReconciliation(t *testing.T) {
	db := newDB(t)
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	runID := "run-20261110T100000Z-aaaaaaaa"
	createAgentTestRun(t, st, runID)
	if _, _, _, err := st.RecordAgentStep(runID, 1, store.AgentStepDiagnosis, "Title", "Body", "call-1", goldenBase); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, _, _, err := st.RecordAgentStep(runID, 1, store.AgentStepDiagnosis, "Title", "Body", "call-1", goldenBase); err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	st.Close()
	code, stdout, stderr := runAgentTest(t, agentTestOptions(t, &stepClock{base: goldenBase}, &agentFakeRunner{}, nil, agentTestKey), "",
		"agent", "show", runID, "--db", db, "--json")
	if code != 0 {
		t.Fatalf("show code=%d stderr=%s", code, stderr)
	}
	var doc agentShowDoc
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode show: %v", err)
	}
	if doc.Effects.Steps != 1 || doc.Effects.Attempts != 2 || doc.Effects.Duplicates != 1 || doc.Effects.Reconciled != 1 {
		t.Fatalf("effects = %+v", doc.Effects)
	}
}

func TestAgentTranscriptFromCopiedStore(t *testing.T) {
	temp := t.TempDir()
	storePath := filepath.Join(temp, "copy-agent.sqlite")
	if err := os.WriteFile(storePath, []byte("fixture"), 0o600); err != nil {
		t.Fatalf("create store fixture: %v", err)
	}
	runID := "run-20261110T100000Z-aaaaaaaa"
	runner := &agentFakeRunner{fn: func(request agent.HostRequest) (agent.RunOutcome, error) {
		return agent.RunOutcome{Response: agent.HostResponse{
			SchemaVersion: agent.ProtocolVersion, State: "transcript", RunID: request.RunID,
			ConversationID: 3, Found: true,
			Entries: []agent.TranscriptEntry{
				{Kind: "app.agent-task", Role: "user", Text: "task prompt"},
				{Kind: "pi.assistant", Role: "assistant", Text: "summary"},
			},
		}}, nil
	}}
	code, stdout, stderr := runAgentTest(t, agentTestOptions(t, nil, runner, nil, agentTestKey), "",
		"agent", "transcript", runID, "--agent-store", storePath, "--db", filepath.Join(temp, "missing", "app.sqlite"), "--json")
	if code != 0 {
		t.Fatalf("transcript code=%d stderr=%s", code, stderr)
	}
	var doc agentTranscriptDoc
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode transcript: %v", err)
	}
	if !doc.Found || doc.ConversationID != 3 || len(doc.Entries) != 2 {
		t.Fatalf("transcript = %+v", doc)
	}
	if _, err := os.Stat(filepath.Join(temp, "missing", "app.sqlite")); !os.IsNotExist(err) {
		t.Fatal("transcript opened or created the application database")
	}
	code, _, stderr = runAgentTest(t, agentTestOptions(t, nil, runner, nil, agentTestKey), "",
		"agent", "transcript", runID, "--agent-store", filepath.Join(temp, "absent.sqlite"), "--json")
	if code != 2 || !strings.Contains(stderr, "store_missing") {
		t.Fatalf("missing store code=%d stderr=%s", code, stderr)
	}
}

func TestAgentHumanWriters(t *testing.T) {
	temp := t.TempDir()
	db := filepath.Join(temp, "app.sqlite")
	configureAgentProvider(t, db)
	ids := &agentIDSequence{}
	runner := completingAgentRunner(t, db, goldenBase.Add(10*time.Second))
	clock := &stepClock{base: goldenBase}
	code, stdout, _ := runAgentTest(t, agentTestOptions(t, clock, runner, ids, agentTestKey), "",
		"agent", "start", "--db", db, "--agent-store", filepath.Join(temp, "agent.sqlite"))
	if code != 0 || !strings.Contains(stdout, "Pass Quest — durable study brief") || !strings.Contains(stdout, "advisory") {
		t.Fatalf("human start code=%d output=%s", code, stdout)
	}
	code, runs, _ := runAgentTest(t, agentTestOptions(t, clock, runner, nil, agentTestKey), "",
		"agent", "runs", "--db", db)
	if code != 0 || !strings.Contains(runs, "RUN") || !strings.Contains(runs, "completed") {
		t.Fatalf("runs human code=%d output=%s", code, runs)
	}
	code, show, _ := runAgentTest(t, agentTestOptions(t, clock, runner, nil, agentTestKey), "",
		"agent", "show", "--db", db)
	if code != 0 || !strings.Contains(show, "Reconciliation") && !strings.Contains(show, "reconciliation") {
		t.Fatalf("show human code=%d output=%s", code, show)
	}
}
