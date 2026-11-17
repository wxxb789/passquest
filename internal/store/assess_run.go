package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/attempt"
)

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
