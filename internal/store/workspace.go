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

// schemaV17 adds the desktop workspace's local state: two optional learner
// settings that are display-only (Q-0011-06), and the practice presentation
// ledger that binds an answer to the item the window displayed (Q-0011-08).
// A presentation row carries its own display time, so an interrupted item
// resumes with its original clock, and the attempt id it closed with, so a
// re-submitted answer returns the stored attempt instead of appending another.
// Attempts stay append-only and unchanged.
var schemaV17 = []string{
	`CREATE TABLE IF NOT EXISTS learner_settings (
		key TEXT PRIMARY KEY CHECK (key IN ('exam_date', 'daily_minutes')),
		value TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS practice_presentations (
		token TEXT PRIMARY KEY CHECK (length(token) BETWEEN 8 AND 80),
		kind TEXT NOT NULL CHECK (kind IN ('next', 'skill', 'tactic', 'sample', 'source')),
		content_id TEXT NOT NULL CHECK (length(content_id) BETWEEN 1 AND 200),
		content_version TEXT NOT NULL,
		question_id TEXT NOT NULL CHECK (length(question_id) BETWEEN 1 AND 200),
		source_id TEXT NOT NULL DEFAULT '',
		tactic_id TEXT NOT NULL DEFAULT '',
		skill_id TEXT NOT NULL DEFAULT '',
		stage TEXT NOT NULL DEFAULT '',
		presented_at TEXT NOT NULL,
		hint_opened_at TEXT NOT NULL DEFAULT '',
		closed_at TEXT NOT NULL DEFAULT '',
		closed_reason TEXT NOT NULL DEFAULT '' CHECK (closed_reason IN ('', 'answered', 'abandoned')),
		attempt_id INTEGER NOT NULL DEFAULT 0 CHECK (attempt_id >= 0)
	)`,
	`CREATE INDEX IF NOT EXISTS practice_presentations_open ON practice_presentations(closed_at, presented_at)`,
}

// Learner setting keys. Both are optional and never inputs to scoring,
// selection or assessment.
const (
	LearnerSettingExamDate     = "exam_date"
	LearnerSettingDailyMinutes = "daily_minutes"
)

// Practice presentation close reasons.
const (
	PracticeClosedAnswered  = "answered"
	PracticeClosedAbandoned = "abandoned"
)

// ErrPracticeClosed reports that a presentation token was already answered or
// abandoned. The presentation is returned with it so the caller can read the
// attempt it closed with.
var ErrPracticeClosed = errors.New("store: practice presentation is closed")

// LearnerSetting reads one optional learner setting.
func (s *Store) LearnerSetting(key string) (string, bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM learner_settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: read learner setting %s: %w", key, err)
	}
	return value, true, nil
}

// SetLearnerSetting stores one learner setting; an empty value clears it.
func (s *Store) SetLearnerSetting(key, value string, at time.Time) error {
	if key != LearnerSettingExamDate && key != LearnerSettingDailyMinutes {
		return fmt.Errorf("store: unknown learner setting %q", key)
	}
	if strings.TrimSpace(value) == "" {
		if _, err := s.db.Exec(`DELETE FROM learner_settings WHERE key = ?`, key); err != nil {
			return fmt.Errorf("store: clear learner setting %s: %w", key, err)
		}
		return nil
	}
	if _, err := s.db.Exec(`INSERT INTO learner_settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, at.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("store: save learner setting %s: %w", key, err)
	}
	return nil
}

// PracticePresentation is one item the desktop window displayed.
type PracticePresentation struct {
	Token          string
	Kind           string
	ContentID      string
	ContentVersion string
	QuestionID     string
	SourceID       string
	TacticID       string
	SkillID        string
	Stage          string
	PresentedAt    time.Time
	HintOpenedAt   time.Time
	ClosedAt       time.Time
	ClosedReason   string
	AttemptID      int64
}

// Open reports whether the presentation still awaits an answer.
func (p PracticePresentation) Open() bool { return p.ClosedAt.IsZero() }

const practicePresentationSelect = `SELECT token, kind, content_id, content_version, question_id,
	source_id, tactic_id, skill_id, stage, presented_at, hint_opened_at, closed_at, closed_reason, attempt_id
	FROM practice_presentations`

// OpenPracticePresentation records a displayed practice item and appends its
// holdout presentation (surface) in one transaction. Any other open item is
// closed as abandoned first: the window shows one item at a time, and an item
// left for another one was viewed, not answered.
func (s *Store) OpenPracticePresentation(p PracticePresentation, surface string) error {
	if len(strings.TrimSpace(p.Token)) < 8 {
		return errors.New("store: practice presentation token is too short")
	}
	if strings.TrimSpace(p.ContentID) == "" || strings.TrimSpace(p.QuestionID) == "" || strings.TrimSpace(p.ContentVersion) == "" {
		return errors.New("store: practice presentation needs content, version and question")
	}
	if p.PresentedAt.IsZero() {
		return errors.New("store: practice presentation time must be set")
	}
	if !assess.ValidSurface(surface) {
		return fmt.Errorf("store: invalid presentation surface %q", surface)
	}
	at := p.PresentedAt.UTC().Format(time.RFC3339Nano)
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin practice presentation: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE practice_presentations SET closed_at = ?, closed_reason = 'abandoned'
		WHERE closed_at = ''`, at); err != nil {
		return fmt.Errorf("store: close open practice presentations: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO practice_presentations (
		token, kind, content_id, content_version, question_id, source_id, tactic_id, skill_id, stage, presented_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Token, p.Kind, p.ContentID, p.ContentVersion, p.QuestionID, p.SourceID, p.TacticID, p.SkillID, p.Stage, at); err != nil {
		return fmt.Errorf("store: insert practice presentation: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO content_presentations (content_id, question_id, surface, at) VALUES (?, ?, ?, ?)`,
		p.ContentID, p.QuestionID, surface, at); err != nil {
		return fmt.Errorf("store: record presentation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit practice presentation: %w", err)
	}
	return nil
}

// PracticePresentationByToken reads one presentation.
func (s *Store) PracticePresentationByToken(token string) (PracticePresentation, error) {
	p, err := scanPracticePresentation(s.db.QueryRow(practicePresentationSelect+` WHERE token = ?`, token))
	if errors.Is(err, sql.ErrNoRows) {
		return PracticePresentation{}, fmt.Errorf("%w: practice presentation %q", ErrNotFound, token)
	}
	if err != nil {
		return PracticePresentation{}, fmt.Errorf("store: read practice presentation: %w", err)
	}
	return p, nil
}

// LatestOpenPracticePresentation returns the most recently displayed item
// that has neither an attempt nor an abandonment.
func (s *Store) LatestOpenPracticePresentation() (PracticePresentation, bool, error) {
	p, err := scanPracticePresentation(s.db.QueryRow(practicePresentationSelect +
		` WHERE closed_at = '' ORDER BY rowid DESC LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return PracticePresentation{}, false, nil
	}
	if err != nil {
		return PracticePresentation{}, false, fmt.Errorf("store: read open practice presentation: %w", err)
	}
	return p, true, nil
}

// MarkPracticeHint records the first time the hint was opened on an open
// presentation. Opening it again keeps the first time.
func (s *Store) MarkPracticeHint(token string, at time.Time) (PracticePresentation, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return PracticePresentation{}, fmt.Errorf("store: begin practice hint: %w", err)
	}
	defer tx.Rollback()
	p, err := scanPracticePresentation(tx.QueryRow(practicePresentationSelect+` WHERE token = ?`, token))
	if errors.Is(err, sql.ErrNoRows) {
		return PracticePresentation{}, fmt.Errorf("%w: practice presentation %q", ErrNotFound, token)
	}
	if err != nil {
		return PracticePresentation{}, fmt.Errorf("store: read practice presentation: %w", err)
	}
	if !p.Open() {
		return p, ErrPracticeClosed
	}
	if p.HintOpenedAt.IsZero() {
		if _, err := tx.Exec(`UPDATE practice_presentations SET hint_opened_at = ? WHERE token = ?`,
			at.UTC().Format(time.RFC3339Nano), token); err != nil {
			return PracticePresentation{}, fmt.Errorf("store: mark practice hint: %w", err)
		}
		p.HintOpenedAt = at.UTC()
	}
	if err := tx.Commit(); err != nil {
		return PracticePresentation{}, fmt.Errorf("store: commit practice hint: %w", err)
	}
	return p, nil
}

// AppendPracticeAttempt stores the attempt for an open presentation and
// closes it in the same transaction, so a crash cannot leave an answer without
// its presentation closed or the reverse. A closed presentation returns
// ErrPracticeClosed with the presentation, and nothing is written.
func (s *Store) AppendPracticeAttempt(token string, record attempt.Attempt, at time.Time) (attempt.Attempt, PracticePresentation, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return attempt.Attempt{}, PracticePresentation{}, fmt.Errorf("store: begin practice answer: %w", err)
	}
	defer tx.Rollback()
	p, err := scanPracticePresentation(tx.QueryRow(practicePresentationSelect+` WHERE token = ?`, token))
	if errors.Is(err, sql.ErrNoRows) {
		return attempt.Attempt{}, PracticePresentation{}, fmt.Errorf("%w: practice presentation %q", ErrNotFound, token)
	}
	if err != nil {
		return attempt.Attempt{}, PracticePresentation{}, fmt.Errorf("store: read practice presentation: %w", err)
	}
	if !p.Open() {
		return attempt.Attempt{}, p, ErrPracticeClosed
	}
	if record.ContentID != p.ContentID || record.QuestionID != p.QuestionID {
		return attempt.Attempt{}, p, fmt.Errorf("store: attempt %s/%s does not match presentation %s/%s",
			record.ContentID, record.QuestionID, p.ContentID, p.QuestionID)
	}
	stored, err := insertAttempt(tx, record)
	if err != nil {
		return attempt.Attempt{}, p, err
	}
	if _, err := tx.Exec(`UPDATE practice_presentations SET closed_at = ?, closed_reason = 'answered', attempt_id = ?
		WHERE token = ?`, at.UTC().Format(time.RFC3339Nano), stored.ID, token); err != nil {
		return attempt.Attempt{}, p, fmt.Errorf("store: close practice presentation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return attempt.Attempt{}, p, fmt.Errorf("store: commit practice answer: %w", err)
	}
	p.ClosedAt = at.UTC()
	p.ClosedReason = PracticeClosedAnswered
	p.AttemptID = stored.ID
	return stored, p, nil
}

func scanPracticePresentation(row rowScanner) (PracticePresentation, error) {
	var (
		p                         PracticePresentation
		presented, hint, closedAt string
	)
	if err := row.Scan(&p.Token, &p.Kind, &p.ContentID, &p.ContentVersion, &p.QuestionID,
		&p.SourceID, &p.TacticID, &p.SkillID, &p.Stage, &presented, &hint, &closedAt, &p.ClosedReason, &p.AttemptID); err != nil {
		return PracticePresentation{}, err
	}
	var err error
	if p.PresentedAt, err = parseTime(presented); err != nil {
		return PracticePresentation{}, err
	}
	if p.HintOpenedAt, err = parseOptionalTime(hint); err != nil {
		return PracticePresentation{}, err
	}
	if p.ClosedAt, err = parseOptionalTime(closedAt); err != nil {
		return PracticePresentation{}, err
	}
	return p, nil
}

// RecordPresentationOnce appends a presentation only when no row with the same
// content, question and surface exists. A recommendation that is rendered
// again names the same unit; the pool counts one spend per unit, so a repeat
// row would add nothing but noise. It reports whether a row was written.
func (s *Store) RecordPresentationOnce(contentID, questionID, surface string, at time.Time) (bool, error) {
	if strings.TrimSpace(contentID) == "" {
		return false, errors.New("store: presentation content id is required")
	}
	if !assess.ValidSurface(surface) {
		return false, fmt.Errorf("store: invalid presentation surface %q", surface)
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	res, err := s.db.Exec(`INSERT INTO content_presentations (content_id, question_id, surface, at)
		SELECT ?, ?, ?, ?
		WHERE NOT EXISTS (SELECT 1 FROM content_presentations WHERE content_id = ? AND question_id = ? AND surface = ?)`,
		contentID, questionID, surface, at.UTC().Format(time.RFC3339Nano), contentID, questionID, surface)
	if err != nil {
		return false, fmt.Errorf("store: record presentation: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: record presentation: %w", err)
	}
	return n > 0, nil
}

// AssessmentRuns lists every assessment run, oldest first.
func (s *Store) AssessmentRuns() ([]assess.Run, error) {
	rows, err := s.db.Query(assessmentRunSelect + ` ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: list assessment runs: %w", err)
	}
	defer rows.Close()
	var out []assess.Run
	for rows.Next() {
		run, err := scanAssessmentRun(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan assessment run: %w", err)
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate assessment runs: %w", err)
	}
	return out, nil
}

// FirstAttemptAt returns the start time of the earliest recorded attempt.
func (s *Store) FirstAttemptAt() (time.Time, bool, error) {
	var raw sql.NullString
	if err := s.db.QueryRow(`SELECT started_at FROM attempts ORDER BY id LIMIT 1`).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, fmt.Errorf("store: read first attempt: %w", err)
	}
	at, err := parseTime(raw.String)
	if err != nil {
		return time.Time{}, false, err
	}
	return at, true, nil
}
