package store

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
)

func sampleAgentRun(id string) AgentRun {
	return AgentRun{
		RunID:         id,
		TaskKind:      "study-brief",
		PromptVersion: "agent-study-brief-v1",
		Model:         "example-model",
		StorePath:     "/tmp/example-agent.sqlite",
		MaxSteps:      3,
		MaxTurns:      8,
		CreatedAt:     time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC),
	}
}

func mustCreateAgentRun(t *testing.T, st *Store, run AgentRun) {
	t.Helper()
	if err := st.CreateAgentRun(run, map[string]any{"host_token": strings.Repeat("a", 32)}, run.CreatedAt); err != nil {
		t.Fatalf("CreateAgentRun: %v", err)
	}
}

func TestAgentStepEffectIsIdempotentAndLeavesAttemptsAlone(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "agent.sqlite"))
	existing := mustAppend(t, st, sampleAttempt("q1"))
	run := sampleAgentRun("run-20261110T100000Z-aaaaaaaa")
	mustCreateAgentRun(t, st, run)

	step, duplicate, attempts, err := st.RecordAgentStep(run.RunID, 1, AgentStepDiagnosis,
		"Check polarity", "Mark NOT before comparing options.", "call-1", run.CreatedAt)
	if err != nil {
		t.Fatalf("RecordAgentStep committed: %v", err)
	}
	if duplicate || attempts != 1 || step.Index != 1 || step.Kind != AgentStepDiagnosis {
		t.Fatalf("first record = %+v duplicate=%v attempts=%d", step, duplicate, attempts)
	}

	again, duplicate, attempts, err := st.RecordAgentStep(run.RunID, 1, AgentStepTactic,
		"Different text", "A replay must not overwrite the first step.", "call-1", run.CreatedAt.Add(time.Second))
	if err != nil {
		t.Fatalf("RecordAgentStep duplicate: %v", err)
	}
	if !duplicate || attempts != 2 {
		t.Fatalf("duplicate = %v attempts=%d", duplicate, attempts)
	}
	if again.Title != "Check polarity" || again.Kind != AgentStepDiagnosis {
		t.Fatalf("duplicate returned %+v; the first step must be kept", again)
	}

	steps, err := st.AgentSteps(run.RunID)
	if err != nil || len(steps) != 1 {
		t.Fatalf("steps = %+v err=%v", steps, err)
	}
	recorded, err := st.AgentStepAttempts(run.RunID)
	if err != nil || len(recorded) != 2 {
		t.Fatalf("attempts = %+v err=%v", recorded, err)
	}
	if recorded[0].Outcome != "committed" || recorded[1].Outcome != "duplicate" {
		t.Fatalf("outcomes = %s,%s", recorded[0].Outcome, recorded[1].Outcome)
	}
	events, err := st.AgentRunEvents(run.RunID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	reconciled := 0
	for _, event := range events {
		if event.Kind == AgentEventReconciled {
			reconciled++
		}
	}
	if reconciled != 1 {
		t.Fatalf("reconciled events = %d, want 1", reconciled)
	}

	// The effect tables and the append-only attempt table are disjoint.
	rows, err := st.List()
	if err != nil || len(rows) != 1 || rows[0].ID != existing.ID {
		t.Fatalf("attempts changed: %+v err=%v", rows, err)
	}
}

func TestAgentRunRefusesASecondUnfinishedRun(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "agent.sqlite"))
	first := sampleAgentRun("run-20261110T100000Z-aaaaaaaa")
	mustCreateAgentRun(t, st, first)
	second := sampleAgentRun("run-20261110T100001Z-bbbbbbbb")
	if err := st.CreateAgentRun(second, nil, second.CreatedAt); !errors.Is(err, ErrAgentRunUnfinished) {
		t.Fatalf("second run error = %v, want ErrAgentRunUnfinished", err)
	}
	if _, err := st.AppendAgentRunTerminal(first.RunID, AgentEventCompleted,
		map[string]any{"steps": 1}, first.CreatedAt.Add(time.Minute)); err != nil {
		t.Fatalf("complete first run: %v", err)
	}
	if err := st.CreateAgentRun(second, nil, second.CreatedAt); err != nil {
		t.Fatalf("CreateAgentRun after completion: %v", err)
	}
}

func TestAgentStepRefusedAfterCancellation(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "agent.sqlite"))
	run := sampleAgentRun("run-20261110T100000Z-aaaaaaaa")
	mustCreateAgentRun(t, st, run)
	if _, err := st.AppendAgentRunEvent(run.RunID, AgentEventCancelRequested, map[string]any{"pid": 4}, run.CreatedAt); err != nil {
		t.Fatalf("cancel request: %v", err)
	}
	if _, _, _, err := st.RecordAgentStep(run.RunID, 1, AgentStepDiagnosis, "t", "b", "call-1", run.CreatedAt); !errors.Is(err, ErrAgentRunCancelled) {
		t.Fatalf("record after cancel = %v, want ErrAgentRunCancelled", err)
	}
}

func TestAgentCancellationRequestedIsABarrier(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "agent.sqlite"))
	run := sampleAgentRun("run-20261110T100000Z-aaaaaaaa")
	mustCreateAgentRun(t, st, run)
	if _, err := st.AppendAgentRunEvent(run.RunID, AgentEventCancelRequested, map[string]any{"pid": 4}, run.CreatedAt); err != nil {
		t.Fatalf("cancel request: %v", err)
	}
	// A racing run process can append an interruption after the request; the
	// step effect must still see the cancellation.
	if _, err := st.AppendAgentRunEvent(run.RunID, AgentEventInterrupted,
		map[string]any{"reason": "host_exit"}, run.CreatedAt.Add(time.Second)); err != nil {
		t.Fatalf("late interruption: %v", err)
	}
	pending, err := st.AgentRunCancelPending(run.RunID)
	if err != nil || !pending {
		t.Fatalf("pending cancel = %v,%v; want true", pending, err)
	}
	if _, _, _, err := st.RecordAgentStep(run.RunID, 1, AgentStepDiagnosis, "t", "b", "call-1", run.CreatedAt); !errors.Is(err, ErrAgentRunCancelled) {
		t.Fatalf("record after a hidden cancel = %v, want ErrAgentRunCancelled", err)
	}
}

func TestAgentStepRefusedAfterCompletion(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "agent.sqlite"))
	run := sampleAgentRun("run-20261110T100000Z-aaaaaaaa")
	mustCreateAgentRun(t, st, run)
	if _, err := st.AppendAgentRunTerminal(run.RunID, AgentEventCompleted, map[string]any{"steps": 1}, run.CreatedAt); err != nil {
		t.Fatalf("complete run: %v", err)
	}
	if _, _, _, err := st.RecordAgentStep(run.RunID, 2, AgentStepTactic, "t", "b", "call-2", run.CreatedAt); !errors.Is(err, ErrAgentRunCompleted) {
		t.Fatalf("record after completion = %v, want ErrAgentRunCompleted", err)
	}
}

func TestAgentTablesAreAppendOnly(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "agent.sqlite"))
	run := sampleAgentRun("run-20261110T100000Z-aaaaaaaa")
	mustCreateAgentRun(t, st, run)
	if _, _, _, err := st.RecordAgentStep(run.RunID, 1, AgentStepDiagnosis, "t", "b", "call-1", run.CreatedAt); err != nil {
		t.Fatalf("record step: %v", err)
	}
	for _, statement := range []string{
		`UPDATE agent_runs SET model = 'other' WHERE run_id = ?`,
		`DELETE FROM agent_runs WHERE run_id = ?`,
		`UPDATE agent_run_events SET kind = 'failed' WHERE run_id = ?`,
		`DELETE FROM agent_run_events WHERE run_id = ?`,
		`UPDATE agent_steps SET title = 'other' WHERE run_id = ?`,
		`DELETE FROM agent_steps WHERE run_id = ?`,
		`UPDATE agent_step_attempts SET outcome = 'duplicate' WHERE run_id = ?`,
		`DELETE FROM agent_step_attempts WHERE run_id = ?`,
	} {
		if _, err := st.db.Exec(statement, run.RunID); err == nil {
			t.Fatalf("append-only trigger did not reject %q", statement)
		}
	}
}

func TestAgentEventAppendRequiresARun(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "agent.sqlite"))
	if _, err := st.AppendAgentRunEvent("run-20261110T100000Z-aaaaaaaa", AgentEventInterrupted, nil, time.Now()); !errors.Is(err, ErrAgentRunNotFound) {
		t.Fatalf("unknown run error = %v", err)
	}
}

func TestAgentResumeClaimIsSingleWriter(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "agent.sqlite"))
	run := sampleAgentRun("run-20261110T100000Z-aaaaaaaa")
	mustCreateAgentRun(t, st, run)
	latest, ok, err := st.LatestAgentRunEvent(run.RunID)
	if err != nil || !ok {
		t.Fatalf("latest event = %+v ok=%v err=%v", latest, ok, err)
	}
	claim, err := st.ClaimAgentResume(run.RunID, latest.ID, map[string]any{"host_token": strings.Repeat("b", 32)}, run.CreatedAt)
	if err != nil || claim.Kind != AgentEventResumed {
		t.Fatalf("claim = %+v err=%v", claim, err)
	}
	// A second resume that observed the same latest event loses the claim and
	// must not start another host.
	if _, err := st.ClaimAgentResume(run.RunID, latest.ID, nil, run.CreatedAt); !errors.Is(err, ErrAgentRunChanged) {
		t.Fatalf("second claim = %v, want ErrAgentRunChanged", err)
	}
	// A retry that re-reads the trail can claim after the first host crashed.
	latest, _, err = st.LatestAgentRunEvent(run.RunID)
	if err != nil {
		t.Fatalf("latest event after claim: %v", err)
	}
	if _, err := st.ClaimAgentResume(run.RunID, latest.ID, nil, run.CreatedAt.Add(time.Second)); err != nil {
		t.Fatalf("retry claim: %v", err)
	}
	if _, err := st.AppendAgentRunEvent(run.RunID, AgentEventCancelRequested, nil, run.CreatedAt); err != nil {
		t.Fatalf("cancel request: %v", err)
	}
	latest, _, _ = st.LatestAgentRunEvent(run.RunID)
	if _, err := st.ClaimAgentResume(run.RunID, latest.ID, nil, run.CreatedAt); !errors.Is(err, ErrAgentRunCancelled) {
		t.Fatalf("claim with a pending cancel = %v, want ErrAgentRunCancelled", err)
	}
	if _, err := st.AppendAgentRunTerminal(run.RunID, AgentEventCancelled, map[string]any{"confirmed": true}, run.CreatedAt); err != nil {
		t.Fatalf("cancel run: %v", err)
	}
	latest, _, _ = st.LatestAgentRunEvent(run.RunID)
	if _, err := st.ClaimAgentResume(run.RunID, latest.ID, nil, run.CreatedAt); !errors.Is(err, ErrAgentRunFinished) {
		t.Fatalf("claim on a cancelled run = %v, want ErrAgentRunFinished", err)
	}
	if _, err := st.ClaimAgentResume("run-20261110T100000Z-bbbbbbbb", 0, nil, run.CreatedAt); !errors.Is(err, ErrAgentRunNotFound) {
		t.Fatalf("claim on an unknown run = %v, want ErrAgentRunNotFound", err)
	}
}

func TestAgentTerminalStateIsAbsorbing(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "agent.sqlite"))
	run := sampleAgentRun("run-20261110T100000Z-aaaaaaaa")
	mustCreateAgentRun(t, st, run)
	if _, err := st.AppendAgentRunTerminal(run.RunID, AgentEventCompleted, map[string]any{"steps": 1}, run.CreatedAt); err != nil {
		t.Fatalf("complete run: %v", err)
	}
	if _, err := st.AppendAgentRunEvent(run.RunID, AgentEventFailed, map[string]any{"code": "late"}, run.CreatedAt); !errors.Is(err, ErrAgentRunFinished) {
		t.Fatalf("late failed event = %v, want ErrAgentRunFinished", err)
	}
	changed, err := st.AppendAgentRunTerminal(run.RunID, AgentEventCancelled, map[string]any{"confirmed": true}, run.CreatedAt)
	if err != nil || changed {
		t.Fatalf("late cancelled = %v,%v; want no change", changed, err)
	}
	events, err := st.AgentRunEvents(run.RunID)
	if err != nil || len(events) != 2 {
		t.Fatalf("events = %+v err=%v", events, err)
	}
}

func TestAgentTerminalEventIsIdempotent(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "agent.sqlite"))
	run := sampleAgentRun("run-20261110T100000Z-aaaaaaaa")
	mustCreateAgentRun(t, st, run)
	changed, err := st.AppendAgentRunTerminal(run.RunID, AgentEventCancelled, map[string]any{"confirmed": true}, run.CreatedAt)
	if err != nil || !changed {
		t.Fatalf("first terminal = %v,%v", changed, err)
	}
	changed, err = st.AppendAgentRunTerminal(run.RunID, AgentEventCancelled, map[string]any{"confirmed": true}, run.CreatedAt.Add(time.Second))
	if err != nil || changed {
		t.Fatalf("second terminal = %v,%v; want no change", changed, err)
	}
	events, err := st.AgentRunEvents(run.RunID)
	if err != nil || len(events) != 2 {
		t.Fatalf("events = %+v err=%v", events, err)
	}
}

func TestAgentRunsSummaryListsStepsAndAttempts(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "agent.sqlite"))
	run := sampleAgentRun("run-20261110T100000Z-aaaaaaaa")
	mustCreateAgentRun(t, st, run)
	if _, _, _, err := st.RecordAgentStep(run.RunID, 1, AgentStepDiagnosis, "t", "b", "call-1", run.CreatedAt); err != nil {
		t.Fatalf("record step: %v", err)
	}
	if _, _, _, err := st.RecordAgentStep(run.RunID, 1, AgentStepDiagnosis, "t", "b", "call-1", run.CreatedAt); err != nil {
		t.Fatalf("record duplicate: %v", err)
	}
	summaries, err := st.AgentRuns()
	if err != nil || len(summaries) != 1 {
		t.Fatalf("runs = %+v err=%v", summaries, err)
	}
	summary := summaries[0]
	if summary.Steps != 1 || summary.Attempts != 2 || summary.LatestEvent != AgentEventStarted || summary.StorePath != run.StorePath {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestMigrationFromVersionEightKeepsAttemptsAndTutorRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v8.sqlite")
	db := openSQLite(t, path)
	for _, statements := range [][]string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6, schemaV7, schemaV8} {
		for _, statement := range statements {
			if _, err := db.Exec(statement); err != nil {
				t.Fatalf("create v8 schema: %v", err)
			}
		}
	}
	at := "2026-11-10T09:00:00Z"
	if _, err := db.Exec(`INSERT INTO attempts (
			session_id, question_id, content_id, content_version, status,
			selected_label, selected_text, correct_label, correct_text,
			started_at, answered_at, elapsed_ms, assistance, tactic_id, skills_json
		) VALUES ('sess', 'q1', 'sample-0001', '1', 'correct',
			'B', 'At 4 p.m.', 'B', 'At 4 p.m.', ?, ?, 5, 'independent', '', '[]')`, at, at); err != nil {
		t.Fatalf("insert v8 attempt: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO tutor_config (
			id, endpoint, model, timeout_ms, max_output_tokens, daily_calls, updated_at
		) VALUES (1, 'https://example.com/v1/chat/completions', 'm', 30000, 350, 20, ?)`, at); err != nil {
		t.Fatalf("insert v8 tutor config: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 8`); err != nil {
		t.Fatalf("set v8 version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v8 fixture: %v", err)
	}

	st := mustOpen(t, path)
	records, err := st.List()
	if err != nil || len(records) != 1 {
		t.Fatalf("attempts after migration = %+v err=%v", records, err)
	}
	var migrated attempt.Attempt = records[0]
	if migrated.QuestionID != "q1" || migrated.Status != attempt.StatusCorrect {
		t.Fatalf("attempt changed: %+v", migrated)
	}
	if _, configured, err := st.TutorConfig(); err != nil || !configured {
		t.Fatalf("tutor config after migration = %v,%v", configured, err)
	}
	run := sampleAgentRun("run-20261110T100000Z-aaaaaaaa")
	mustCreateAgentRun(t, st, run)
	if _, _, _, err := st.RecordAgentStep(run.RunID, 1, AgentStepDiagnosis, "t", "b", "call-1", run.CreatedAt); err != nil {
		t.Fatalf("v9 tables after migration: %v", err)
	}
}
