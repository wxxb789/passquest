package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/wxxb789/passquest/internal/assess"
)

// schemaV14 adds the assessment holdout ledger, runs, items and events, the
// optional authored glossary, and widens attempts to admit a timed assessment
// answer (assistance='none' with its run id). SQLite cannot widen a CHECK in
// place, so attempts is rebuilt with an explicit copy; every row keeps its id
// and values, and the append-only triggers are recreated on the new table.
var schemaV14 = []string{
	`DROP TRIGGER IF EXISTS attempts_append_only_update`,
	`DROP TRIGGER IF EXISTS attempts_append_only_delete`,
	`DROP TRIGGER IF EXISTS attempts_append_only_insert`,
	`DROP INDEX IF EXISTS attempts_content_question_id`,
	`ALTER TABLE attempts RENAME TO attempts_v13`,
	`CREATE TABLE attempts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		question_id TEXT NOT NULL,
		content_id TEXT NOT NULL,
		content_version TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('correct', 'incorrect', 'skipped')),
		selected_label TEXT,
		selected_text TEXT,
		correct_label TEXT NOT NULL,
		correct_text TEXT NOT NULL,
		started_at TEXT NOT NULL,
		answered_at TEXT NOT NULL,
		elapsed_ms INTEGER NOT NULL CHECK (elapsed_ms >= 0),
		assistance TEXT NOT NULL DEFAULT 'independent' CHECK (assistance IN ('independent', 'assisted', 'none')),
		tactic_id TEXT NOT NULL DEFAULT '',
		skills_json TEXT,
		assessment_run_id INTEGER NOT NULL DEFAULT 0 CHECK (assessment_run_id >= 0),
		CHECK (
			(status = 'skipped' AND selected_label IS NULL AND selected_text IS NULL)
			OR (status IN ('correct', 'incorrect') AND selected_label IS NOT NULL AND selected_text IS NOT NULL)
		),
		CHECK (
			(status = 'correct' AND selected_label = correct_label)
			OR (status = 'incorrect' AND selected_label <> correct_label)
			OR status = 'skipped'
		),
		CHECK ((assistance = 'none') = (assessment_run_id > 0))
	)`,
	`INSERT INTO attempts (
		id, session_id, question_id, content_id, content_version, status,
		selected_label, selected_text, correct_label, correct_text,
		started_at, answered_at, elapsed_ms, assistance, tactic_id, skills_json,
		assessment_run_id
		)
		SELECT
		id, session_id, question_id, content_id, content_version, status,
		selected_label, selected_text, correct_label, correct_text,
		started_at, answered_at, elapsed_ms, assistance, tactic_id, skills_json,
		0
		FROM attempts_v13`,
	`DROP TABLE attempts_v13`,
	`CREATE INDEX IF NOT EXISTS attempts_content_question_id ON attempts(content_id, question_id, id)`,
	`CREATE INDEX IF NOT EXISTS attempts_assessment_run ON attempts(assessment_run_id, id)
		WHERE assessment_run_id > 0`,
	`CREATE TRIGGER IF NOT EXISTS attempts_append_only_update BEFORE UPDATE ON attempts
		BEGIN SELECT RAISE(ABORT, 'attempts are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS attempts_append_only_delete BEFORE DELETE ON attempts
		BEGIN SELECT RAISE(ABORT, 'attempts are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS attempts_append_only_insert BEFORE INSERT ON attempts
		WHEN EXISTS (SELECT 1 FROM attempts WHERE id = NEW.id)
		BEGIN SELECT RAISE(ABORT, 'attempts are append-only'); END`,
	`ALTER TABLE source_questions ADD COLUMN glossary_json TEXT NOT NULL DEFAULT '[]'`,
	`CREATE TABLE IF NOT EXISTS content_presentations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		content_id TEXT NOT NULL CHECK (length(content_id) BETWEEN 1 AND 200),
		question_id TEXT NOT NULL DEFAULT '',
		surface TEXT NOT NULL CHECK (surface IN ('practice', 'tactic', 'skill', 'assess', 'show', 'questions', 'next')),
		at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS content_presentations_content ON content_presentations(content_id, at)`,
	`CREATE TRIGGER IF NOT EXISTS content_presentations_append_only_update BEFORE UPDATE ON content_presentations
		BEGIN SELECT RAISE(ABORT, 'content presentations are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS content_presentations_append_only_delete BEFORE DELETE ON content_presentations
		BEGIN SELECT RAISE(ABORT, 'content presentations are append-only'); END`,
	`CREATE TABLE IF NOT EXISTS assessment_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at TEXT NOT NULL,
		started_at TEXT NOT NULL,
		deadline_at TEXT NOT NULL,
		window_minutes INTEGER NOT NULL CHECK (window_minutes >= 1),
		requested_size INTEGER NOT NULL CHECK (requested_size >= 1),
		actual_size INTEGER NOT NULL CHECK (actual_size >= 1),
		per_item_timeout_ms INTEGER NOT NULL CHECK (per_item_timeout_ms >= 1),
		annotations TEXT NOT NULL CHECK (annotations = 'none'),
		pool_family TEXT NOT NULL CHECK (length(pool_family) = 64),
		source_count INTEGER NOT NULL CHECK (source_count >= 0),
		status TEXT NOT NULL CHECK (status IN ('active', 'completed', 'abandoned')),
		resume_count INTEGER NOT NULL DEFAULT 0 CHECK (resume_count >= 0),
		last_resumed_at TEXT NOT NULL DEFAULT '',
		completed_at TEXT NOT NULL DEFAULT '',
		abandoned_at TEXT NOT NULL DEFAULT '',
		abandoned_reason TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS assessment_runs_one_active ON assessment_runs(status)
		WHERE status = 'active'`,
	`CREATE TABLE IF NOT EXISTS assessment_items (
		run_id INTEGER NOT NULL REFERENCES assessment_runs(id),
		position INTEGER NOT NULL CHECK (position >= 0),
		content_id TEXT NOT NULL CHECK (length(content_id) BETWEEN 1 AND 200),
		source_id TEXT NOT NULL DEFAULT '',
		page INTEGER NOT NULL DEFAULT 0 CHECK (page >= 0),
		content_version TEXT NOT NULL,
		question_id TEXT NOT NULL CHECK (length(question_id) BETWEEN 1 AND 200),
		skills_json TEXT NOT NULL DEFAULT '[]',
		state TEXT NOT NULL CHECK (state IN ('pending', 'presented', 'answered', 'timed_out', 'unavailable')),
		presented_at TEXT NOT NULL DEFAULT '',
		item_deadline_at TEXT NOT NULL DEFAULT '',
		answered_at TEXT NOT NULL DEFAULT '',
		attempt_id INTEGER NOT NULL DEFAULT 0 CHECK (attempt_id >= 0),
		PRIMARY KEY (run_id, position)
	)`,
	`CREATE INDEX IF NOT EXISTS assessment_items_run ON assessment_items(run_id, position)`,
	`CREATE TABLE IF NOT EXISTS assessment_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id INTEGER NOT NULL REFERENCES assessment_runs(id),
		kind TEXT NOT NULL CHECK (kind IN ('created', 'presented', 'answered', 'timed_out', 'resumed', 'completed', 'abandoned', 'unavailable')),
		position INTEGER NOT NULL DEFAULT -1,
		at TEXT NOT NULL,
		detail TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS assessment_events_run ON assessment_events(run_id, id)`,
	`CREATE TRIGGER IF NOT EXISTS assessment_events_append_only_update BEFORE UPDATE ON assessment_events
		BEGIN SELECT RAISE(ABORT, 'assessment events are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS assessment_events_append_only_delete BEFORE DELETE ON assessment_events
		BEGIN SELECT RAISE(ABORT, 'assessment events are append-only'); END`,
}

// Assessment store errors. The CLI maps the state errors to exit 2 and keeps
// the persisted run available for `assess report`.
var (
	ErrAssessmentActive        = errors.New("store: an assessment run is already active")
	ErrAssessmentRunState      = errors.New("store: assessment run state does not allow this action")
	ErrAssessmentWindowElapsed = errors.New("store: assessment run window has elapsed")
)

const assessmentRunSelect = `SELECT
	id, created_at, started_at, deadline_at, window_minutes, requested_size, actual_size,
	per_item_timeout_ms, annotations, pool_family, source_count, status, resume_count,
	last_resumed_at, completed_at, abandoned_at, abandoned_reason
	FROM assessment_runs`

const assessmentItemSelect = `SELECT
	run_id, position, content_id, source_id, page, content_version, question_id,
	skills_json, state, presented_at, item_deadline_at, answered_at, attempt_id
	FROM assessment_items`

func readAssessmentRun(row rowQuerier, id int64) (assess.Run, error) {
	run, err := scanAssessmentRun(row.QueryRow(assessmentRunSelect+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return assess.Run{}, fmt.Errorf("%w: assessment run %d", ErrNotFound, id)
	}
	if err != nil {
		return assess.Run{}, fmt.Errorf("store: read assessment run %d: %w", id, err)
	}
	return run, nil
}

func abandonAssessmentTx(tx *sql.Tx, id int64, at time.Time, reason string) error {
	if _, err := tx.Exec(`UPDATE assessment_runs
		SET status = 'abandoned', abandoned_at = ?, abandoned_reason = ?
		WHERE id = ? AND status = 'active'`,
		at.UTC().Format(time.RFC3339Nano), reason, id); err != nil {
		return fmt.Errorf("store: abandon assessment run: %w", err)
	}
	return appendAssessmentEvent(tx, id, assess.EventAbandoned, -1, at, reason)
}

func appendAssessmentEvent(tx *sql.Tx, runID int64, kind string, position int, at time.Time, detail string) error {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	if _, err := tx.Exec(`INSERT INTO assessment_events (run_id, kind, position, at, detail)
		VALUES (?, ?, ?, ?, ?)`, runID, kind, position, at.UTC().Format(time.RFC3339Nano), detail); err != nil {
		return fmt.Errorf("store: insert assessment event: %w", err)
	}
	return nil
}

func (s *Store) assessmentItems(runID int64) ([]assess.Item, error) {
	rows, err := s.db.Query(assessmentItemSelect+` WHERE run_id = ? ORDER BY position`, runID)
	if err != nil {
		return nil, fmt.Errorf("store: list assessment items: %w", err)
	}
	defer rows.Close()
	var out []assess.Item
	for rows.Next() {
		item, err := scanAssessmentItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate assessment items: %w", err)
	}
	return out, nil
}

func assessmentItemTx(tx *sql.Tx, runID int64, position int) (assess.Item, error) {
	item, err := scanAssessmentItem(tx.QueryRow(assessmentItemSelect+` WHERE run_id = ? AND position = ?`, runID, position))
	if errors.Is(err, sql.ErrNoRows) {
		return assess.Item{}, fmt.Errorf("%w: assessment item %d of run %d", ErrNotFound, position, runID)
	}
	if err != nil {
		return assess.Item{}, fmt.Errorf("store: read assessment item %d: %w", position, err)
	}
	return item, nil
}

func (s *Store) assessmentEvents(runID int64) ([]assess.Event, error) {
	rows, err := s.db.Query(`SELECT id, run_id, kind, position, at, detail
		FROM assessment_events WHERE run_id = ? ORDER BY id`, runID)
	if err != nil {
		return nil, fmt.Errorf("store: list assessment events: %w", err)
	}
	defer rows.Close()
	var out []assess.Event
	for rows.Next() {
		var (
			event assess.Event
			at    string
		)
		if err := rows.Scan(&event.ID, &event.RunID, &event.Kind, &event.Position, &at, &event.Detail); err != nil {
			return nil, fmt.Errorf("store: scan assessment event: %w", err)
		}
		var err error
		if event.At, err = parseTime(at); err != nil {
			return nil, fmt.Errorf("store: parse assessment event time: %w", err)
		}
		out = append(out, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate assessment events: %w", err)
	}
	return out, nil
}

func scanAssessmentRun(row rowScanner) (assess.Run, error) {
	var (
		run                               assess.Run
		created, started, deadline        string
		lastResumed, completed, abandoned string
	)
	if err := row.Scan(&run.ID, &created, &started, &deadline, &run.WindowMinutes,
		&run.RequestedSize, &run.ActualSize, &run.PerItemTimeoutMS, &run.Annotations,
		&run.PoolFamily, &run.SourceCount, &run.Status, &run.ResumeCount,
		&lastResumed, &completed, &abandoned, &run.AbandonedReason); err != nil {
		return assess.Run{}, err
	}
	var err error
	if run.CreatedAt, err = parseTime(created); err != nil {
		return assess.Run{}, err
	}
	if run.StartedAt, err = parseTime(started); err != nil {
		return assess.Run{}, err
	}
	if run.DeadlineAt, err = parseTime(deadline); err != nil {
		return assess.Run{}, err
	}
	if run.LastResumedAt, err = parseOptionalTime(lastResumed); err != nil {
		return assess.Run{}, err
	}
	if run.CompletedAt, err = parseOptionalTime(completed); err != nil {
		return assess.Run{}, err
	}
	if run.AbandonedAt, err = parseOptionalTime(abandoned); err != nil {
		return assess.Run{}, err
	}
	return run, nil
}

func scanAssessmentItem(row rowScanner) (assess.Item, error) {
	var (
		item                          assess.Item
		skillsJSON                    string
		presented, deadline, answered string
	)
	if err := row.Scan(&item.RunID, &item.Position, &item.ContentID, &item.SourceID,
		&item.Page, &item.ContentVersion, &item.QuestionID, &skillsJSON, &item.State,
		&presented, &deadline, &answered, &item.AttemptID); err != nil {
		return assess.Item{}, err
	}
	var err error
	if item.Skills, err = decodeSkills(skillsJSON); err != nil {
		return assess.Item{}, fmt.Errorf("decode assessment item skills: %w", err)
	}
	if item.PresentedAt, err = parseOptionalTime(presented); err != nil {
		return assess.Item{}, err
	}
	if item.DeadlineAt, err = parseOptionalTime(deadline); err != nil {
		return assess.Item{}, err
	}
	if item.AnsweredAt, err = parseOptionalTime(answered); err != nil {
		return assess.Item{}, err
	}
	return item, nil
}
