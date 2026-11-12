package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/attempt"
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

// Presentation is one question presentation to append.
type Presentation struct {
	ContentID  string
	QuestionID string
	Surface    string
	At         time.Time
}

// RecordPresentation appends one question presentation to the holdout ledger.
// It is evidence of exposure, not an attempt: no answer or grade is implied.
func (s *Store) RecordPresentation(contentID, questionID, surface string, at time.Time) error {
	return s.RecordPresentations([]Presentation{{ContentID: contentID, QuestionID: questionID, Surface: surface, At: at}})
}

// RecordPresentations appends presentations in one transaction. A listing that
// prints many stems spends every printed unit with one write.
func (s *Store) RecordPresentations(entries []Presentation) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin presentations: %w", err)
	}
	defer tx.Rollback()
	for _, entry := range entries {
		contentID := strings.TrimSpace(entry.ContentID)
		if contentID == "" {
			return errors.New("store: presentation content id is required")
		}
		if !assess.ValidSurface(entry.Surface) {
			return fmt.Errorf("store: invalid presentation surface %q", entry.Surface)
		}
		at := entry.At
		if at.IsZero() {
			at = time.Now().UTC()
		}
		if _, err := tx.Exec(`INSERT INTO content_presentations (content_id, question_id, surface, at)
			VALUES (?, ?, ?, ?)`, contentID, entry.QuestionID, entry.Surface, at.UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("store: record presentation: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit presentations: %w", err)
	}
	return nil
}

// Spends returns the complete spend union: old attempts, guided exposures,
// presentations, derived notes and assessment items. A pool therefore needs no
// backfill when this build first opens an older database.
func (s *Store) Spends() ([]assess.Spend, error) {
	rows, err := s.db.Query(`
		SELECT content_id, question_id, 'attempt', started_at, assessment_run_id FROM attempts
		UNION ALL
		SELECT content_id, question_id, 'guided_exposure', exposed_at, 0 FROM tactic_exposures
		UNION ALL
		SELECT content_id, question_id, surface, at, 0 FROM content_presentations
		UNION ALL
		SELECT content_id, question_id, 'note', created_at, 0 FROM note_drafts
		UNION ALL
		SELECT content_id, question_id, 'note', created_at, 0 FROM notes
		UNION ALL
		SELECT content_id, question_id, 'assess', presented_at, run_id FROM assessment_items
			WHERE presented_at <> ''`)
	if err != nil {
		return nil, fmt.Errorf("store: list spends: %w", err)
	}
	defer rows.Close()
	var out []assess.Spend
	for rows.Next() {
		var (
			spend   assess.Spend
			at      string
			surface string
		)
		if err := rows.Scan(&spend.ContentID, &spend.QuestionID, &surface, &at, &spend.RunID); err != nil {
			return nil, fmt.Errorf("store: scan spend: %w", err)
		}
		spend.Surface = surface
		if spend.At, err = parseTime(at); err != nil {
			return nil, fmt.Errorf("store: parse spend time: %w", err)
		}
		out = append(out, spend)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate spends: %w", err)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].At.Equal(out[j].At) {
			return out[i].ContentID < out[j].ContentID
		}
		return out[i].At.Before(out[j].At)
	})
	return out, nil
}

// CreateAssessmentRun stores one active run and its selected items in order.
func (s *Store) CreateAssessmentRun(run assess.Run, items []assess.Item) (assess.Run, error) {
	if len(items) == 0 {
		return assess.Run{}, errors.New("store: assessment run has no items")
	}
	if run.WindowMinutes < 1 || run.RequestedSize < 1 || run.PerItemTimeoutMS < 1 {
		return assess.Run{}, errors.New("store: assessment run is incomplete")
	}
	if run.Annotations == "" {
		run.Annotations = assess.AnnotationsNone
	}
	if run.Annotations != assess.AnnotationsNone {
		return assess.Run{}, errors.New("store: only annotations=none is supported")
	}
	if run.Status == "" {
		run.Status = assess.StatusActive
	}
	if run.Status != assess.StatusActive {
		return assess.Run{}, errors.New("store: a new assessment run must start active")
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = run.StartedAt
	}
	if run.DeadlineAt.IsZero() {
		run.DeadlineAt = run.StartedAt.Add(time.Duration(run.WindowMinutes) * time.Minute)
	}
	run.ActualSize = len(items)

	tx, err := s.db.Begin()
	if err != nil {
		return assess.Run{}, fmt.Errorf("store: begin assessment run: %w", err)
	}
	defer tx.Rollback()
	var active int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM assessment_runs WHERE status = 'active'`).Scan(&active); err != nil {
		return assess.Run{}, fmt.Errorf("store: count active assessment runs: %w", err)
	}
	if active > 0 {
		return assess.Run{}, ErrAssessmentActive
	}
	res, err := tx.Exec(`INSERT INTO assessment_runs (
		created_at, started_at, deadline_at, window_minutes, requested_size, actual_size,
		per_item_timeout_ms, annotations, pool_family, source_count, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.CreatedAt.UTC().Format(time.RFC3339Nano),
		run.StartedAt.UTC().Format(time.RFC3339Nano),
		run.DeadlineAt.UTC().Format(time.RFC3339Nano),
		run.WindowMinutes, run.RequestedSize, run.ActualSize, run.PerItemTimeoutMS,
		run.Annotations, run.PoolFamily, run.SourceCount, run.Status,
	)
	if err != nil {
		return assess.Run{}, fmt.Errorf("store: insert assessment run: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return assess.Run{}, fmt.Errorf("store: read assessment run id: %w", err)
	}
	for index, item := range items {
		if item.Position != index {
			return assess.Run{}, fmt.Errorf("store: assessment item %d has position %d", index, item.Position)
		}
		if strings.TrimSpace(item.ContentID) == "" || strings.TrimSpace(item.QuestionID) == "" {
			return assess.Run{}, fmt.Errorf("store: assessment item %d has no content or question", index)
		}
		if item.State == "" {
			item.State = assess.StatePending
		}
		if item.State != assess.StatePending {
			return assess.Run{}, fmt.Errorf("store: assessment item %d must start pending", index)
		}
		skillsJSON, err := encodeSkills(item.Skills)
		if err != nil {
			return assess.Run{}, err
		}
		if _, err := tx.Exec(`INSERT INTO assessment_items (
			run_id, position, content_id, source_id, page, content_version, question_id,
			skills_json, state
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, index, item.ContentID, item.SourceID, item.Page, item.ContentVersion,
			item.QuestionID, skillsJSON, item.State,
		); err != nil {
			return assess.Run{}, fmt.Errorf("store: insert assessment item %d: %w", index, err)
		}
	}
	if err := appendAssessmentEvent(tx, id, assess.EventCreated, -1, run.CreatedAt, ""); err != nil {
		return assess.Run{}, err
	}
	if err := tx.Commit(); err != nil {
		return assess.Run{}, fmt.Errorf("store: commit assessment run: %w", err)
	}
	run.ID = id
	return run, nil
}

// AssessmentRunItems reads one run and its items without the event trail.
func (s *Store) AssessmentRunItems(id int64) (assess.Run, []assess.Item, error) {
	run, err := readAssessmentRun(s.db, id)
	if err != nil {
		return assess.Run{}, nil, err
	}
	items, err := s.assessmentItems(id)
	if err != nil {
		return assess.Run{}, nil, err
	}
	return run, items, nil
}

// AssessmentRun reads one run, its items in order and its event trail.
func (s *Store) AssessmentRun(id int64) (assess.Run, []assess.Item, []assess.Event, error) {
	run, items, err := s.AssessmentRunItems(id)
	if err != nil {
		return assess.Run{}, nil, nil, err
	}
	events, err := s.assessmentEvents(id)
	if err != nil {
		return assess.Run{}, nil, nil, err
	}
	return run, items, events, nil
}

// ActiveAssessmentRun returns the one active run, if any.
func (s *Store) ActiveAssessmentRun() (assess.Run, bool, error) {
	run, err := scanAssessmentRun(s.db.QueryRow(assessmentRunSelect + ` WHERE status = 'active' ORDER BY id LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return assess.Run{}, false, nil
	}
	if err != nil {
		return assess.Run{}, false, fmt.Errorf("store: read active assessment run: %w", err)
	}
	return run, true, nil
}

// ReconcileAssessmentRuns abandons every active run whose whole-run deadline
// has passed. It returns the affected ids. Called before a coaching command so
// an expired window stops blocking and a run is never silently dropped.
func (s *Store) ReconcileAssessmentRuns(now time.Time) ([]int64, error) {
	rows, err := s.db.Query(`SELECT id, deadline_at FROM assessment_runs WHERE status = 'active' ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: list active assessment runs: %w", err)
	}
	var expired []int64
	for rows.Next() {
		var (
			id       int64
			deadline string
		)
		if err := rows.Scan(&id, &deadline); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: scan active assessment run: %w", err)
		}
		at, err := parseTime(deadline)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: parse assessment deadline: %w", err)
		}
		if !now.Before(at) {
			expired = append(expired, id)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("store: iterate active assessment runs: %w", err)
	}
	rows.Close()
	for _, id := range expired {
		if _, err := s.AbandonAssessmentRun(id, now, "window_elapsed"); err != nil && !errors.Is(err, ErrAssessmentRunState) {
			return nil, err
		}
	}
	return expired, nil
}

// MarkAssessmentPresented records the first presentation of an item and its
// item deadline. Re-presenting an item on resume keeps the original clock.
func (s *Store) MarkAssessmentPresented(runID int64, position int, at time.Time) (assess.Item, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return assess.Item{}, fmt.Errorf("store: begin assessment presentation: %w", err)
	}
	defer tx.Rollback()
	run, err := readAssessmentRun(tx, runID)
	if err != nil {
		return assess.Item{}, err
	}
	if run.Status != assess.StatusActive {
		return assess.Item{}, ErrAssessmentRunState
	}
	if !at.Before(run.DeadlineAt) {
		return assess.Item{}, ErrAssessmentWindowElapsed
	}
	item, err := assessmentItemTx(tx, runID, position)
	if err != nil {
		return assess.Item{}, err
	}
	if item.State != assess.StatePending {
		return item, nil
	}
	deadline := at.Add(time.Duration(run.PerItemTimeoutMS) * time.Millisecond)
	if deadline.After(run.DeadlineAt) {
		deadline = run.DeadlineAt
	}
	if _, err := tx.Exec(`UPDATE assessment_items SET state = 'presented', presented_at = ?, item_deadline_at = ?
		WHERE run_id = ? AND position = ?`,
		at.UTC().Format(time.RFC3339Nano), deadline.UTC().Format(time.RFC3339Nano), runID, position); err != nil {
		return assess.Item{}, fmt.Errorf("store: mark assessment item presented: %w", err)
	}
	if err := appendAssessmentEvent(tx, runID, assess.EventPresented, position, at, ""); err != nil {
		return assess.Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return assess.Item{}, fmt.Errorf("store: commit assessment presentation: %w", err)
	}
	item.State = assess.StatePresented
	item.PresentedAt = at.UTC()
	item.DeadlineAt = deadline.UTC()
	return item, nil
}

// AppendAssessmentAttempt stores one assessment answer and updates its item in
// the same transaction. timedOut records a skipped attempt at the item
// deadline; the attempt still carries the correct key so the report can teach
// afterwards.
func (s *Store) AppendAssessmentAttempt(runID int64, position int, record attempt.Attempt, timedOut bool, at time.Time) (attempt.Attempt, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return attempt.Attempt{}, fmt.Errorf("store: begin assessment answer: %w", err)
	}
	defer tx.Rollback()
	run, err := readAssessmentRun(tx, runID)
	if err != nil {
		return attempt.Attempt{}, err
	}
	if run.Status != assess.StatusActive {
		return attempt.Attempt{}, ErrAssessmentRunState
	}
	item, err := assessmentItemTx(tx, runID, position)
	if err != nil {
		return attempt.Attempt{}, err
	}
	if !item.Finalized() && item.State != assess.StatePresented {
		return attempt.Attempt{}, fmt.Errorf("%w: assessment item %d is %s", ErrAssessmentRunState, position, item.State)
	}
	if item.Finalized() {
		return attempt.Attempt{}, fmt.Errorf("%w: assessment item %d is already %s", ErrAssessmentRunState, position, item.State)
	}
	record.AssessmentRunID = runID
	record.Assistance = attempt.AssistanceNone
	stored, err := insertAttempt(tx, record)
	if err != nil {
		return attempt.Attempt{}, err
	}
	state := assess.StateAnswered
	kind := assess.EventAnswered
	if timedOut {
		state = assess.StateTimedOut
		kind = assess.EventTimedOut
	}
	if _, err := tx.Exec(`UPDATE assessment_items SET state = ?, answered_at = ?, attempt_id = ?
		WHERE run_id = ? AND position = ?`,
		state, at.UTC().Format(time.RFC3339Nano), stored.ID, runID, position); err != nil {
		return attempt.Attempt{}, fmt.Errorf("store: finish assessment item: %w", err)
	}
	if err := appendAssessmentEvent(tx, runID, kind, position, at, ""); err != nil {
		return attempt.Attempt{}, err
	}
	if err := tx.Commit(); err != nil {
		return attempt.Attempt{}, fmt.Errorf("store: commit assessment answer: %w", err)
	}
	return stored, nil
}

// MarkAssessmentUnavailable records an item whose content can no longer be
// loaded. The run continues; the report excludes it instead of scoring it.
func (s *Store) MarkAssessmentUnavailable(runID int64, position int, at time.Time, detail string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin unavailable item: %w", err)
	}
	defer tx.Rollback()
	run, err := readAssessmentRun(tx, runID)
	if err != nil {
		return err
	}
	if run.Status != assess.StatusActive {
		return ErrAssessmentRunState
	}
	res, err := tx.Exec(`UPDATE assessment_items SET state = 'unavailable'
		WHERE run_id = ? AND position = ? AND state IN ('pending', 'presented')`, runID, position)
	if err != nil {
		return fmt.Errorf("store: mark assessment item unavailable: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: mark assessment item unavailable: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: assessment item %d is not open", ErrAssessmentRunState, position)
	}
	if err := appendAssessmentEvent(tx, runID, assess.EventUnavailable, position, at, detail); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit unavailable item: %w", err)
	}
	return nil
}

// ResumeAssessmentRun marks one resume and returns the run. A run whose whole
// window has elapsed is abandoned first and the window error is returned with
// the run state still readable.
func (s *Store) ResumeAssessmentRun(runID int64, at time.Time) (assess.Run, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return assess.Run{}, fmt.Errorf("store: begin assessment resume: %w", err)
	}
	defer tx.Rollback()
	run, err := readAssessmentRun(tx, runID)
	if err != nil {
		return assess.Run{}, err
	}
	if run.Status != assess.StatusActive {
		return run, ErrAssessmentRunState
	}
	if !at.Before(run.DeadlineAt) {
		if err := abandonAssessmentTx(tx, runID, at, "window_elapsed"); err != nil {
			return assess.Run{}, err
		}
		if err := tx.Commit(); err != nil {
			return assess.Run{}, fmt.Errorf("store: commit abandoned assessment: %w", err)
		}
		run.Status = assess.StatusAbandoned
		run.AbandonedAt = at.UTC()
		run.AbandonedReason = "window_elapsed"
		return run, ErrAssessmentWindowElapsed
	}
	if _, err := tx.Exec(`UPDATE assessment_runs SET resume_count = resume_count + 1, last_resumed_at = ?
		WHERE id = ?`, at.UTC().Format(time.RFC3339Nano), runID); err != nil {
		return assess.Run{}, fmt.Errorf("store: record assessment resume: %w", err)
	}
	if err := appendAssessmentEvent(tx, runID, assess.EventResumed, -1, at, ""); err != nil {
		return assess.Run{}, err
	}
	if err := tx.Commit(); err != nil {
		return assess.Run{}, fmt.Errorf("store: commit assessment resume: %w", err)
	}
	run.ResumeCount++
	run.LastResumedAt = at.UTC()
	return run, nil
}

// CompleteAssessmentRun marks a run completed once no item is pending.
func (s *Store) CompleteAssessmentRun(runID int64, at time.Time) (assess.Run, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return assess.Run{}, fmt.Errorf("store: begin assessment completion: %w", err)
	}
	defer tx.Rollback()
	run, err := readAssessmentRun(tx, runID)
	if err != nil {
		return assess.Run{}, err
	}
	if run.Status != assess.StatusActive {
		return run, ErrAssessmentRunState
	}
	var open int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM assessment_items
		WHERE run_id = ? AND state IN ('pending', 'presented')`, runID).Scan(&open); err != nil {
		return assess.Run{}, fmt.Errorf("store: count open assessment items: %w", err)
	}
	if open > 0 {
		return run, fmt.Errorf("%w: %d item(s) are still open", ErrAssessmentRunState, open)
	}
	if _, err := tx.Exec(`UPDATE assessment_runs SET status = 'completed', completed_at = ? WHERE id = ?`,
		at.UTC().Format(time.RFC3339Nano), runID); err != nil {
		return assess.Run{}, fmt.Errorf("store: complete assessment run: %w", err)
	}
	if err := appendAssessmentEvent(tx, runID, assess.EventCompleted, -1, at, ""); err != nil {
		return assess.Run{}, err
	}
	if err := tx.Commit(); err != nil {
		return assess.Run{}, fmt.Errorf("store: commit assessment completion: %w", err)
	}
	run.Status = assess.StatusCompleted
	run.CompletedAt = at.UTC()
	return run, nil
}

// AbandonAssessmentRun marks an active run abandoned. Abandoning an already
// abandoned run is idempotent; a completed run is not rewritten.
func (s *Store) AbandonAssessmentRun(runID int64, at time.Time, reason string) (assess.Run, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return assess.Run{}, fmt.Errorf("store: begin assessment abandonment: %w", err)
	}
	defer tx.Rollback()
	run, err := readAssessmentRun(tx, runID)
	if err != nil {
		return assess.Run{}, err
	}
	switch run.Status {
	case assess.StatusAbandoned:
		return run, nil
	case assess.StatusCompleted:
		return run, ErrAssessmentRunState
	}
	if reason == "" {
		reason = "learner_abandoned"
	}
	if err := abandonAssessmentTx(tx, runID, at, reason); err != nil {
		return assess.Run{}, err
	}
	if err := tx.Commit(); err != nil {
		return assess.Run{}, fmt.Errorf("store: commit assessment abandonment: %w", err)
	}
	run.Status = assess.StatusAbandoned
	run.AbandonedAt = at.UTC()
	run.AbandonedReason = reason
	return run, nil
}

// AssessmentAttempts returns the attempts recorded by one run.
func (s *Store) AssessmentAttempts(runID int64) ([]attempt.Attempt, error) {
	rows, err := s.db.Query(attemptSelect+` WHERE assessment_run_id = ? ORDER BY id`, runID)
	if err != nil {
		return nil, fmt.Errorf("store: list assessment attempts: %w", err)
	}
	defer rows.Close()
	var out []attempt.Attempt
	seen := make(map[attempt.QuestionKey]int)
	for rows.Next() {
		record, err := scanAttemptRow(rows, seen)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate assessment attempts: %w", err)
	}
	return out, nil
}

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
