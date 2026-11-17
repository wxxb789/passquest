package store

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/assess"
)

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
