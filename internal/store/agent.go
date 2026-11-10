package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// schemaV9 adds the durable-agent surface: one immutable run definition per
// bounded task, an append-only run event trail, append-only advisory steps, and
// an append-only ledger of every effect invocation. None of these tables can
// change an attempt, a grade, a question or an answer key.
var schemaV9 = []string{
	`CREATE TABLE IF NOT EXISTS agent_runs (
		run_id TEXT PRIMARY KEY CHECK (length(run_id) BETWEEN 8 AND 80),
		task_kind TEXT NOT NULL CHECK (task_kind IN ('study-brief')),
		prompt_version TEXT NOT NULL CHECK (length(prompt_version) BETWEEN 1 AND 80),
		model TEXT NOT NULL CHECK (length(model) BETWEEN 1 AND 200),
		store_path TEXT NOT NULL CHECK (length(store_path) BETWEEN 1 AND 4096),
		max_steps INTEGER NOT NULL CHECK (max_steps BETWEEN 1 AND 6),
		max_turns INTEGER NOT NULL CHECK (max_turns BETWEEN 3 AND 16),
		created_at TEXT NOT NULL
	)`,
	`CREATE TRIGGER IF NOT EXISTS agent_runs_append_only_update BEFORE UPDATE ON agent_runs
		BEGIN SELECT RAISE(ABORT, 'agent runs are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS agent_runs_append_only_delete BEFORE DELETE ON agent_runs
		BEGIN SELECT RAISE(ABORT, 'agent runs are append-only'); END`,
	`CREATE TABLE IF NOT EXISTS agent_run_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id TEXT NOT NULL REFERENCES agent_runs(run_id),
		kind TEXT NOT NULL CHECK (kind IN (
			'started', 'resumed', 'interrupted', 'completed', 'failed',
			'needs_action', 'cancel_requested', 'cancelled', 'reconciled')),
		detail_json TEXT NOT NULL DEFAULT '{}',
		at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS agent_run_events_run ON agent_run_events(run_id, id)`,
	`CREATE TRIGGER IF NOT EXISTS agent_run_events_append_only_update BEFORE UPDATE ON agent_run_events
		BEGIN SELECT RAISE(ABORT, 'agent run events are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS agent_run_events_append_only_delete BEFORE DELETE ON agent_run_events
		BEGIN SELECT RAISE(ABORT, 'agent run events are append-only'); END`,
	`CREATE TABLE IF NOT EXISTS agent_steps (
		step_key TEXT PRIMARY KEY CHECK (length(step_key) BETWEEN 4 AND 120),
		run_id TEXT NOT NULL REFERENCES agent_runs(run_id),
		step_index INTEGER NOT NULL CHECK (step_index BETWEEN 1 AND 6),
		kind TEXT NOT NULL CHECK (kind IN ('diagnosis', 'tactic', 'practice')),
		title TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 100),
		body TEXT NOT NULL CHECK (length(body) BETWEEN 1 AND 600),
		advisory INTEGER NOT NULL DEFAULT 1 CHECK (advisory = 1),
		created_at TEXT NOT NULL,
		UNIQUE (run_id, step_index)
	)`,
	`CREATE INDEX IF NOT EXISTS agent_steps_run ON agent_steps(run_id, step_index)`,
	`CREATE TRIGGER IF NOT EXISTS agent_steps_append_only_update BEFORE UPDATE ON agent_steps
		BEGIN SELECT RAISE(ABORT, 'agent steps are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS agent_steps_append_only_delete BEFORE DELETE ON agent_steps
		BEGIN SELECT RAISE(ABORT, 'agent steps are append-only'); END`,
	`CREATE TABLE IF NOT EXISTS agent_step_attempts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id TEXT NOT NULL REFERENCES agent_runs(run_id),
		step_key TEXT NOT NULL,
		attempt_key TEXT NOT NULL CHECK (length(attempt_key) BETWEEN 1 AND 200),
		outcome TEXT NOT NULL CHECK (outcome IN ('committed', 'duplicate')),
		at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS agent_step_attempts_step ON agent_step_attempts(step_key, id)`,
	`CREATE INDEX IF NOT EXISTS agent_step_attempts_run ON agent_step_attempts(run_id, id)`,
	`CREATE TRIGGER IF NOT EXISTS agent_step_attempts_append_only_update BEFORE UPDATE ON agent_step_attempts
		BEGIN SELECT RAISE(ABORT, 'agent step attempts are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS agent_step_attempts_append_only_delete BEFORE DELETE ON agent_step_attempts
		BEGIN SELECT RAISE(ABORT, 'agent step attempts are append-only'); END`,
}

// Agent run event kinds. The first six are transitions of one bounded task;
// reconciled records a retried effect that found its step already committed.
const (
	AgentEventStarted         = "started"
	AgentEventResumed         = "resumed"
	AgentEventInterrupted     = "interrupted"
	AgentEventCompleted       = "completed"
	AgentEventFailed          = "failed"
	AgentEventNeedsAction     = "needs_action"
	AgentEventCancelRequested = "cancel_requested"
	AgentEventCancelled       = "cancelled"
	AgentEventReconciled      = "reconciled"
)

// Agent step kinds. The kinds describe the teaching shape, not a verified
// answer or a claim about the live examination.
const (
	AgentStepDiagnosis = "diagnosis"
	AgentStepTactic    = "tactic"
	AgentStepPractice  = "practice"
)

// ErrAgentRunNotFound marks a command that named a run this database does not
// have.
var ErrAgentRunNotFound = errors.New("store: agent run not found")

// ErrAgentRunUnfinished refuses a second run while any existing run has not
// completed or been cancelled. One Pi store has one writer, so two runs must
// not share a bounded runtime.
var ErrAgentRunUnfinished = errors.New("store: an agent run is unfinished; resume or cancel it before starting another")

// ErrAgentRunCancelled refuses a side effect after cancellation was requested.
var ErrAgentRunCancelled = errors.New("store: agent run is cancelled")

// ErrAgentRunCompleted refuses a side effect after the run already completed.
var ErrAgentRunCompleted = errors.New("store: agent run is already completed")

// ErrAgentRunFinished refuses a transition after completed or cancelled. Those
// terminal states are absorbing: a late process cannot rewrite the outcome.
var ErrAgentRunFinished = errors.New("store: agent run already reached a terminal state")

// agentLatestEventPredicate is the single definition of a run's latest state
// transition. A reconciled event records a duplicate effect, not a transition,
// so it never becomes the latest event. %s is the run-id expression.
const agentLatestEventPredicate = `e.run_id = %s AND e.kind <> 'reconciled'`

// agentLatestEventQuery selects the latest transition kind for one run. The
// run-id expression is "?" for a direct query and "r.run_id" inside a
// correlated subquery.
func agentLatestEventQuery(runIDExpr string) string {
	return `SELECT e.kind FROM agent_run_events e WHERE ` +
		fmt.Sprintf(agentLatestEventPredicate, runIDExpr) + ` ORDER BY e.id DESC LIMIT 1`
}

// agentLatestEventRowQuery selects the full latest transition row.
func agentLatestEventRowQuery(runIDExpr string) string {
	return `SELECT e.id, e.run_id, e.kind, e.detail_json, e.at FROM agent_run_events e WHERE ` +
		fmt.Sprintf(agentLatestEventPredicate, runIDExpr) + ` ORDER BY e.id DESC LIMIT 1`
}

// rowQuerier is the read surface shared by *sql.DB and *sql.Tx.
type rowQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
}

// agentRunCancelPending reports whether a cancellation request is still
// unconsummated. Terminal states are absorbing, so a request stays pending
// until a completed or cancelled event follows it. Reading the whole trail
// keeps a later interrupted or failed transition from hiding the request.
func agentRunCancelPending(q rowQuerier, runID string) (bool, error) {
	var requested, consumed int
	if err := q.QueryRow(`SELECT
		COUNT(CASE WHEN kind = 'cancel_requested' THEN 1 END),
		COUNT(CASE WHEN kind IN ('completed', 'cancelled') THEN 1 END)
		FROM agent_run_events WHERE run_id = ?`, runID).Scan(&requested, &consumed); err != nil {
		return false, err
	}
	return requested > 0 && consumed == 0, nil
}

// AgentRunCancelPending reports whether cancellation was requested and no
// terminal event followed. resume and the step effect use it so a racing
// interruption cannot make a cancelled run look resumable.
func (s *Store) AgentRunCancelPending(runID string) (bool, error) {
	pending, err := agentRunCancelPending(s.db, runID)
	if err != nil {
		return false, fmt.Errorf("store: read agent cancellation: %w", err)
	}
	return pending, nil
}

// AgentRun is one immutable run definition. It records the model the run was
// started with; resuming uses that model, not a later configuration.
type AgentRun struct {
	RunID         string
	TaskKind      string
	PromptVersion string
	Model         string
	StorePath     string
	MaxSteps      int
	MaxTurns      int
	CreatedAt     time.Time
}

// AgentRunEvent is one append-only transition. DetailJSON is a JSON object;
// callers must not put credentials, private source text or model output in it.
type AgentRunEvent struct {
	ID         int64
	RunID      string
	Kind       string
	DetailJSON string
	At         time.Time
}

// AgentStep is one committed advisory teaching step. It is never an answer
// key, a grade or a claim about the examination.
type AgentStep struct {
	RunID     string
	Index     int
	Kind      string
	Title     string
	Body      string
	CreatedAt time.Time
}

// AgentStepAttempt records one invocation of the side-effecting tool. A
// duplicate means the step already existed and was kept unchanged.
type AgentStepAttempt struct {
	ID         int64
	RunID      string
	StepKey    string
	AttemptKey string
	Outcome    string
	At         time.Time
}

// AgentRunSummary is the list view of one run.
type AgentRunSummary struct {
	AgentRun
	LatestEvent string
	Steps       int
	Attempts    int
}

// CreateAgentRun inserts an immutable run and its started event in one
// transaction. It refuses when any other run has not reached completed or
// cancelled, so two bounded tasks cannot drive one Pi store.
func (s *Store) CreateAgentRun(run AgentRun, detail any, at time.Time) error {
	if run.RunID == "" || run.TaskKind == "" || run.PromptVersion == "" || run.Model == "" || run.StorePath == "" {
		return errors.New("store: incomplete agent run")
	}
	encoded, err := encodeAgentDetail(detail)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin agent run: %w", err)
	}
	defer tx.Rollback()

	var unfinished int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM agent_runs r
		WHERE COALESCE((` + agentLatestEventQuery("r.run_id") + `), 'started')
			NOT IN ('completed', 'cancelled')`).Scan(&unfinished); err != nil {
		return fmt.Errorf("store: count unfinished agent runs: %w", err)
	}
	if unfinished > 0 {
		return ErrAgentRunUnfinished
	}
	if _, err := tx.Exec(`INSERT INTO agent_runs (
			run_id, task_kind, prompt_version, model, store_path, max_steps, max_turns, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		run.RunID, run.TaskKind, run.PromptVersion, run.Model, run.StorePath, run.MaxSteps, run.MaxTurns,
		run.CreatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("store: insert agent run: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO agent_run_events (run_id, kind, detail_json, at) VALUES (?, ?, ?, ?)`,
		run.RunID, AgentEventStarted, encoded, at.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("store: insert agent run event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit agent run: %w", err)
	}
	return nil
}

// AppendAgentRunEvent appends one transition. The run must exist.
func (s *Store) AppendAgentRunEvent(runID, kind string, detail any, at time.Time) (AgentRunEvent, error) {
	if !validAgentEvent(kind) {
		return AgentRunEvent{}, fmt.Errorf("store: unknown agent event %q", kind)
	}
	encoded, err := encodeAgentDetail(detail)
	if err != nil {
		return AgentRunEvent{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AgentRunEvent{}, fmt.Errorf("store: begin agent event: %w", err)
	}
	defer tx.Rollback()
	var latest string
	err = tx.QueryRow(agentLatestEventQuery("?"), runID).Scan(&latest)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentRunEvent{}, ErrAgentRunNotFound
	}
	if err != nil {
		return AgentRunEvent{}, fmt.Errorf("store: read agent run state: %w", err)
	}
	if latest == AgentEventCompleted || latest == AgentEventCancelled {
		return AgentRunEvent{}, ErrAgentRunFinished
	}
	res, err := tx.Exec(`INSERT INTO agent_run_events (run_id, kind, detail_json, at) VALUES (?, ?, ?, ?)`,
		runID, kind, encoded, at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return AgentRunEvent{}, fmt.Errorf("store: append agent run event: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return AgentRunEvent{}, fmt.Errorf("store: read agent run event id: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return AgentRunEvent{}, fmt.Errorf("store: commit agent event: %w", err)
	}
	return AgentRunEvent{RunID: runID, Kind: kind, DetailJSON: encoded, At: at.UTC(), ID: id}, nil
}

// AppendAgentRunTerminal appends completed or cancelled only when it is not
// already the latest event. Two processes can race to finish the same run; the
// second appends nothing and reads the existing transition.
func (s *Store) AppendAgentRunTerminal(runID, kind string, detail any, at time.Time) (bool, error) {
	if kind != AgentEventCompleted && kind != AgentEventCancelled {
		return false, fmt.Errorf("store: %q is not a terminal agent event", kind)
	}
	encoded, err := encodeAgentDetail(detail)
	if err != nil {
		return false, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, fmt.Errorf("store: begin agent terminal event: %w", err)
	}
	defer tx.Rollback()
	var latest string
	err = tx.QueryRow(agentLatestEventQuery("?"), runID).Scan(&latest)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrAgentRunNotFound
	}
	if err != nil {
		return false, fmt.Errorf("store: read latest agent event: %w", err)
	}
	if latest == AgentEventCompleted || latest == AgentEventCancelled || latest == kind {
		return false, nil
	}
	if _, err := tx.Exec(`INSERT INTO agent_run_events (run_id, kind, detail_json, at) VALUES (?, ?, ?, ?)`,
		runID, kind, encoded, at.UTC().Format(time.RFC3339Nano)); err != nil {
		return false, fmt.Errorf("store: append terminal agent event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("store: commit terminal agent event: %w", err)
	}
	return true, nil
}

// AgentRun returns one run definition.
func (s *Store) AgentRun(runID string) (AgentRun, bool, error) {
	var (
		run       AgentRun
		createdAt string
	)
	err := s.db.QueryRow(`SELECT run_id, task_kind, prompt_version, model, store_path, max_steps, max_turns, created_at
		FROM agent_runs WHERE run_id = ?`, runID).
		Scan(&run.RunID, &run.TaskKind, &run.PromptVersion, &run.Model, &run.StorePath, &run.MaxSteps, &run.MaxTurns, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentRun{}, false, nil
	}
	if err != nil {
		return AgentRun{}, false, fmt.Errorf("store: read agent run: %w", err)
	}
	run.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return AgentRun{}, false, fmt.Errorf("store: parse agent run time: %w", err)
	}
	return run, true, nil
}

// LatestAgentRun returns the newest run, optionally restricted to runs whose
// latest event is not completed or cancelled.
func (s *Store) LatestAgentRun(unfinishedOnly bool) (AgentRun, bool, error) {
	query := `SELECT r.run_id, r.task_kind, r.prompt_version, r.model, r.store_path, r.max_steps, r.max_turns, r.created_at
		FROM agent_runs r
		WHERE (? = 0 OR COALESCE((` + agentLatestEventQuery("r.run_id") + `), 'started')
			NOT IN ('completed', 'cancelled'))
		ORDER BY r.created_at DESC, r.run_id DESC LIMIT 1`
	var (
		run       AgentRun
		createdAt string
	)
	err := s.db.QueryRow(query, boolInt(unfinishedOnly)).
		Scan(&run.RunID, &run.TaskKind, &run.PromptVersion, &run.Model, &run.StorePath, &run.MaxSteps, &run.MaxTurns, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentRun{}, false, nil
	}
	if err != nil {
		return AgentRun{}, false, fmt.Errorf("store: read latest agent run: %w", err)
	}
	run.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return AgentRun{}, false, fmt.Errorf("store: parse agent run time: %w", err)
	}
	return run, true, nil
}

// AgentRuns lists every run newest first with its latest event, step count and
// effect invocation count.
func (s *Store) AgentRuns() ([]AgentRunSummary, error) {
	rows, err := s.db.Query(`SELECT r.run_id, r.task_kind, r.prompt_version, r.model, r.store_path, r.max_steps, r.max_turns,
			r.created_at,
			COALESCE((` + agentLatestEventQuery("r.run_id") + `), ''),
			(SELECT COUNT(*) FROM agent_steps s WHERE s.run_id = r.run_id),
			(SELECT COUNT(*) FROM agent_step_attempts a WHERE a.run_id = r.run_id)
		FROM agent_runs r ORDER BY r.created_at DESC, r.run_id DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: list agent runs: %w", err)
	}
	defer rows.Close()
	var out []AgentRunSummary
	for rows.Next() {
		var (
			summary   AgentRunSummary
			createdAt string
		)
		if err := rows.Scan(&summary.RunID, &summary.TaskKind, &summary.PromptVersion, &summary.Model,
			&summary.StorePath, &summary.MaxSteps, &summary.MaxTurns, &createdAt, &summary.LatestEvent,
			&summary.Steps, &summary.Attempts); err != nil {
			return nil, fmt.Errorf("store: scan agent run: %w", err)
		}
		summary.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("store: parse agent run time: %w", err)
		}
		out = append(out, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list agent runs: %w", err)
	}
	return out, nil
}

// AgentRunEvents returns a run's event trail in append order.
func (s *Store) AgentRunEvents(runID string) ([]AgentRunEvent, error) {
	rows, err := s.db.Query(`SELECT id, run_id, kind, detail_json, at
		FROM agent_run_events WHERE run_id = ? ORDER BY id`, runID)
	if err != nil {
		return nil, fmt.Errorf("store: list agent run events: %w", err)
	}
	defer rows.Close()
	var out []AgentRunEvent
	for rows.Next() {
		var (
			event AgentRunEvent
			at    string
		)
		if err := rows.Scan(&event.ID, &event.RunID, &event.Kind, &event.DetailJSON, &at); err != nil {
			return nil, fmt.Errorf("store: scan agent run event: %w", err)
		}
		event.At, err = parseTime(at)
		if err != nil {
			return nil, fmt.Errorf("store: parse agent event time: %w", err)
		}
		out = append(out, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list agent run events: %w", err)
	}
	return out, nil
}

// LatestAgentRunEvent returns the newest transition of one run.
func (s *Store) LatestAgentRunEvent(runID string) (AgentRunEvent, bool, error) {
	var (
		event AgentRunEvent
		at    string
	)
	err := s.db.QueryRow(agentLatestEventRowQuery("?"), runID).
		Scan(&event.ID, &event.RunID, &event.Kind, &event.DetailJSON, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentRunEvent{}, false, nil
	}
	if err != nil {
		return AgentRunEvent{}, false, fmt.Errorf("store: read latest agent event: %w", err)
	}
	event.At, err = parseTime(at)
	if err != nil {
		return AgentRunEvent{}, false, fmt.Errorf("store: parse agent event time: %w", err)
	}
	return event, true, nil
}

// RecordAgentStep applies one effect invocation in a single immediate
// transaction. The step key is run_id + ":" + step_index, so a replay after a
// crash finds the committed row and keeps it unchanged; the attempt ledger
// records the duplicate and the run trail gets one reconciled event. A
// cancellation request blocks the effect.
func (s *Store) RecordAgentStep(runID string, index int, kind, title, body, attemptKey string, at time.Time) (AgentStep, bool, int, error) {
	if attemptKey == "" || len(attemptKey) > 200 {
		return AgentStep{}, false, 0, errors.New("store: agent attempt key must be 1-200 bytes")
	}
	stepKey := runID + ":" + strconv.Itoa(index)
	now := at.UTC().Format(time.RFC3339Nano)
	tx, err := s.db.Begin()
	if err != nil {
		return AgentStep{}, false, 0, fmt.Errorf("store: begin agent step: %w", err)
	}
	defer tx.Rollback()

	var maxSteps int
	if err := tx.QueryRow(`SELECT max_steps FROM agent_runs WHERE run_id = ?`, runID).Scan(&maxSteps); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AgentStep{}, false, 0, ErrAgentRunNotFound
		}
		return AgentStep{}, false, 0, fmt.Errorf("store: read agent run for step: %w", err)
	}
	if index < 1 || index > maxSteps {
		return AgentStep{}, false, 0, fmt.Errorf("store: step index %d is outside 1..%d", index, maxSteps)
	}
	var latest string
	if err := tx.QueryRow(agentLatestEventQuery("?"), runID).Scan(&latest); err != nil {
		return AgentStep{}, false, 0, fmt.Errorf("store: read agent run state: %w", err)
	}
	if latest == AgentEventCompleted {
		return AgentStep{}, false, 0, ErrAgentRunCompleted
	}
	if latest == AgentEventCancelRequested || latest == AgentEventCancelled {
		return AgentStep{}, false, 0, ErrAgentRunCancelled
	}
	pending, err := agentRunCancelPending(tx, runID)
	if err != nil {
		return AgentStep{}, false, 0, fmt.Errorf("store: read agent cancellation: %w", err)
	}
	if pending {
		return AgentStep{}, false, 0, ErrAgentRunCancelled
	}

	var (
		existing    AgentStep
		existingKey string
		createdAt   string
	)
	err = tx.QueryRow(`SELECT step_key, run_id, step_index, kind, title, body, created_at
		FROM agent_steps WHERE run_id = ? AND step_index = ?`, runID, index).
		Scan(&existingKey, &existing.RunID, &existing.Index, &existing.Kind, &existing.Title, &existing.Body, &createdAt)
	duplicate := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return AgentStep{}, false, 0, fmt.Errorf("store: read existing agent step: %w", err)
	}
	if duplicate {
		existing.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return AgentStep{}, false, 0, fmt.Errorf("store: parse agent step time: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO agent_step_attempts (run_id, step_key, attempt_key, outcome, at)
			VALUES (?, ?, ?, 'duplicate', ?)`, runID, stepKey, attemptKey, now); err != nil {
			return AgentStep{}, false, 0, fmt.Errorf("store: append duplicate agent attempt: %w", err)
		}
		detail, err := encodeAgentDetail(map[string]any{"step_index": index, "attempt_key": attemptKey})
		if err != nil {
			return AgentStep{}, false, 0, err
		}
		if _, err := tx.Exec(`INSERT INTO agent_run_events (run_id, kind, detail_json, at)
			VALUES (?, ?, ?, ?)`, runID, AgentEventReconciled, detail, now); err != nil {
			return AgentStep{}, false, 0, fmt.Errorf("store: append reconciliation event: %w", err)
		}
	} else {
		step := AgentStep{RunID: runID, Index: index, Kind: kind, Title: title, Body: body, CreatedAt: at.UTC()}
		if _, err := tx.Exec(`INSERT INTO agent_steps (step_key, run_id, step_index, kind, title, body, advisory, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 1, ?)`, stepKey, runID, index, kind, title, body, now); err != nil {
			return AgentStep{}, false, 0, fmt.Errorf("store: insert agent step: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO agent_step_attempts (run_id, step_key, attempt_key, outcome, at)
			VALUES (?, ?, ?, 'committed', ?)`, runID, stepKey, attemptKey, now); err != nil {
			return AgentStep{}, false, 0, fmt.Errorf("store: append committed agent attempt: %w", err)
		}
		existing = step
	}
	var attempts int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM agent_step_attempts WHERE step_key = ?`, stepKey).Scan(&attempts); err != nil {
		return AgentStep{}, false, 0, fmt.Errorf("store: count agent attempts: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return AgentStep{}, false, 0, fmt.Errorf("store: commit agent step: %w", err)
	}
	return existing, duplicate, attempts, nil
}

// AgentSteps returns the run's steps in index order.
func (s *Store) AgentSteps(runID string) ([]AgentStep, error) {
	rows, err := s.db.Query(`SELECT run_id, step_index, kind, title, body, created_at
		FROM agent_steps WHERE run_id = ? ORDER BY step_index`, runID)
	if err != nil {
		return nil, fmt.Errorf("store: list agent steps: %w", err)
	}
	defer rows.Close()
	var out []AgentStep
	for rows.Next() {
		var (
			step      AgentStep
			createdAt string
		)
		if err := rows.Scan(&step.RunID, &step.Index, &step.Kind, &step.Title, &step.Body, &createdAt); err != nil {
			return nil, fmt.Errorf("store: scan agent step: %w", err)
		}
		step.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("store: parse agent step time: %w", err)
		}
		out = append(out, step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list agent steps: %w", err)
	}
	return out, nil
}

// AgentStepAttempts returns every effect invocation for a run in order.
func (s *Store) AgentStepAttempts(runID string) ([]AgentStepAttempt, error) {
	rows, err := s.db.Query(`SELECT id, run_id, step_key, attempt_key, outcome, at
		FROM agent_step_attempts WHERE run_id = ? ORDER BY id`, runID)
	if err != nil {
		return nil, fmt.Errorf("store: list agent step attempts: %w", err)
	}
	defer rows.Close()
	var out []AgentStepAttempt
	for rows.Next() {
		var (
			attempt AgentStepAttempt
			at      string
		)
		if err := rows.Scan(&attempt.ID, &attempt.RunID, &attempt.StepKey, &attempt.AttemptKey, &attempt.Outcome, &at); err != nil {
			return nil, fmt.Errorf("store: scan agent attempt: %w", err)
		}
		attempt.At, err = parseTime(at)
		if err != nil {
			return nil, fmt.Errorf("store: parse agent attempt time: %w", err)
		}
		out = append(out, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list agent step attempts: %w", err)
	}
	return out, nil
}

func validAgentEvent(kind string) bool {
	switch kind {
	case AgentEventStarted, AgentEventResumed, AgentEventInterrupted, AgentEventCompleted,
		AgentEventFailed, AgentEventNeedsAction, AgentEventCancelRequested, AgentEventCancelled,
		AgentEventReconciled:
		return true
	default:
		return false
	}
}

func encodeAgentDetail(detail any) (string, error) {
	if detail == nil {
		return "{}", nil
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		return "", fmt.Errorf("store: encode agent detail: %w", err)
	}
	if len(encoded) == 0 || encoded[0] != '{' {
		return "", errors.New("store: agent event detail must be a JSON object")
	}
	return string(encoded), nil
}
