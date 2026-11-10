package store

import (
	"fmt"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
)

// schemaV5 adds the guided-exposure ledger. Delivering the tactic walkthrough
// is a fact separate from answering: the learner has seen help on that
// content-scoped question even when no attempt follows, and the question must
// not later count as independent evidence. The table is append-only for the
// same reason attempts are.
var schemaV5 = []string{
	`CREATE TABLE IF NOT EXISTS tactic_exposures (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tactic_id TEXT NOT NULL,
		content_id TEXT NOT NULL,
		question_id TEXT NOT NULL,
		exposed_at TEXT NOT NULL,
		UNIQUE (tactic_id, content_id, question_id)
	)`,
	`CREATE TRIGGER IF NOT EXISTS tactic_exposures_append_only_update BEFORE UPDATE ON tactic_exposures
		BEGIN SELECT RAISE(ABORT, 'tactic exposures are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS tactic_exposures_append_only_delete BEFORE DELETE ON tactic_exposures
		BEGIN SELECT RAISE(ABORT, 'tactic exposures are append-only'); END`,
}

// Exposure records that the guided walkthrough for one content-scoped question
// was delivered under a tactic. It is evidence of help shown, not an attempt:
// no answer, grade or repeat exposure is implied.
type Exposure struct {
	TacticID   string
	ContentID  string
	QuestionID string
	ExposedAt  time.Time
}

// Key returns the content-scoped identity this exposure restricts.
func (e Exposure) Key() attempt.QuestionKey {
	return attempt.QuestionKey{ContentID: e.ContentID, QuestionID: e.QuestionID}
}

// RecordExposure stores one delivered walkthrough. Recording the same exposure
// again is a no-op: the row is set membership, not a counter, so the first
// exposure time is kept.
func (s *Store) RecordExposure(e Exposure) error {
	if err := validateExposure(e); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO tactic_exposures (tactic_id, content_id, question_id, exposed_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (tactic_id, content_id, question_id) DO NOTHING`,
		e.TacticID, e.ContentID, e.QuestionID, e.ExposedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: record tactic exposure: %w", err)
	}
	return nil
}

// Exposed reports whether the guided walkthrough has been delivered for the
// content-scoped question under any tactic. Ordinary practice consults it so an
// answer after a walkthrough is still recorded as assisted.
func (s *Store) Exposed(key attempt.QuestionKey) (bool, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tactic_exposures WHERE content_id = ? AND question_id = ?`,
		key.ContentID, key.QuestionID).Scan(&count); err != nil {
		return false, fmt.Errorf("store: check tactic exposure: %w", err)
	}
	return count > 0, nil
}

// ListExposures returns every recorded exposure in insertion order.
func (s *Store) ListExposures() ([]Exposure, error) {
	rows, err := s.db.Query(`SELECT tactic_id, content_id, question_id, exposed_at
		FROM tactic_exposures ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: list tactic exposures: %w", err)
	}
	defer rows.Close()

	var out []Exposure
	for rows.Next() {
		var (
			e    Exposure
			when string
		)
		if err := rows.Scan(&e.TacticID, &e.ContentID, &e.QuestionID, &when); err != nil {
			return nil, fmt.Errorf("store: scan tactic exposure: %w", err)
		}
		if e.ExposedAt, err = time.Parse(time.RFC3339Nano, when); err != nil {
			return nil, fmt.Errorf("store: parse exposure time of %s: %w", e.QuestionID, err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate tactic exposures: %w", err)
	}
	return out, nil
}

func validateExposure(e Exposure) error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	if !attempt.ValidTacticID(e.TacticID) {
		add("tactic_id %q is not a lowercase slug", e.TacticID)
	}
	if strings.TrimSpace(e.ContentID) == "" {
		add("content_id is empty")
	}
	if strings.TrimSpace(e.QuestionID) == "" {
		add("question_id is empty")
	}
	if e.ExposedAt.IsZero() {
		add("exposed_at must be set")
	}
	if len(problems) > 0 {
		return fmt.Errorf("store: invalid exposure: %s", strings.Join(problems, "; "))
	}
	return nil
}
