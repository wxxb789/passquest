package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SaveNoteDraft stores the open proposal for a target. Re-proposing identical
// inputs returns the existing draft; a changed input for the same target
// supersedes the previous open draft. A different mark produces a different
// target key, so it leaves the previous draft open.
func (s *Store) SaveNoteDraft(d NoteDraft, at time.Time) (NoteDraft, error) {
	if strings.TrimSpace(d.TargetKey) == "" || strings.TrimSpace(d.ContentID) == "" || strings.TrimSpace(d.QuestionID) == "" {
		return NoteDraft{}, errors.New("store: note draft requires target, content and question")
	}
	if d.Origin != "deterministic" && d.Origin != "refined" {
		return NoteDraft{}, fmt.Errorf("store: note draft origin %q is invalid", d.Origin)
	}
	if at.IsZero() {
		return NoteDraft{}, errors.New("store: note draft timestamp is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return NoteDraft{}, fmt.Errorf("store: begin note draft: %w", err)
	}
	defer tx.Rollback()

	existing, found, err := scanNoteDraft(tx.QueryRow(`SELECT `+noteDraftColumns+` FROM note_drafts
		WHERE target_key = ? AND status = 'open'`, d.TargetKey))
	if err != nil {
		return NoteDraft{}, fmt.Errorf("store: read open note draft: %w", err)
	}
	if found && sameNoteDraftInput(existing, d) {
		return existing, nil
	}
	if _, err := tx.Exec(`UPDATE note_drafts SET status = 'superseded'
		WHERE target_key = ? AND status = 'open'`, d.TargetKey); err != nil {
		return NoteDraft{}, fmt.Errorf("store: supersede note draft: %w", err)
	}
	res, err := tx.Exec(`INSERT INTO note_drafts (
			target_key, attempt_id, content_id, content_version, question_id, source_id, page_number,
			mark, restricted, origin, model, refinement_id, refined_front, refined_back, status, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'open', ?)`,
		d.TargetKey, d.AttemptID, d.ContentID, d.ContentVersion, d.QuestionID, d.SourceID, d.Page,
		d.Mark, boolInt(d.Restricted), d.Origin, d.Model, d.RefinementID, d.RefinedFront, d.RefinedBack,
		at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return NoteDraft{}, fmt.Errorf("store: insert note draft: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return NoteDraft{}, fmt.Errorf("store: read note draft id: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return NoteDraft{}, fmt.Errorf("store: commit note draft: %w", err)
	}
	d.ID = id
	d.Status = NoteDraftOpen
	d.CreatedAt = at.UTC()
	return d, nil
}

func sameNoteDraftInput(a, b NoteDraft) bool {
	return a.AttemptID == b.AttemptID &&
		a.ContentID == b.ContentID && a.ContentVersion == b.ContentVersion &&
		a.QuestionID == b.QuestionID && a.SourceID == b.SourceID && a.Page == b.Page &&
		a.Mark == b.Mark && a.Restricted == b.Restricted && a.Origin == b.Origin &&
		a.RefinedFront == b.RefinedFront && a.RefinedBack == b.RefinedBack
}

const noteDraftColumns = `id, target_key, attempt_id, content_id, content_version, question_id, source_id,
	page_number, mark, restricted, origin, model, refinement_id, refined_front, refined_back,
	accepted_front, accepted_back, accepted_tactic, accepted_origin, accepted_model, status, created_at`

func scanNoteDraft(row rowScanner) (NoteDraft, bool, error) {
	var (
		d          NoteDraft
		restricted int
		created    string
	)
	err := row.Scan(&d.ID, &d.TargetKey, &d.AttemptID, &d.ContentID, &d.ContentVersion, &d.QuestionID,
		&d.SourceID, &d.Page, &d.Mark, &restricted, &d.Origin, &d.Model, &d.RefinementID,
		&d.RefinedFront, &d.RefinedBack, &d.AcceptedFront, &d.AcceptedBack, &d.AcceptedTactic,
		&d.AcceptedOrigin, &d.AcceptedModel, &d.Status, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return NoteDraft{}, false, nil
	}
	if err != nil {
		return NoteDraft{}, false, err
	}
	if d.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return NoteDraft{}, false, err
	}
	d.Restricted = restricted != 0
	return d, true, nil
}

// NoteDraftByID loads one draft.
func (s *Store) NoteDraftByID(id int64) (NoteDraft, bool, error) {
	return scanNoteDraft(s.db.QueryRow(`SELECT `+noteDraftColumns+` FROM note_drafts WHERE id = ?`, id))
}

// LatestOpenNoteDraft returns the most recently created open draft.
func (s *Store) LatestOpenNoteDraft() (NoteDraft, bool, error) {
	return scanNoteDraft(s.db.QueryRow(`SELECT ` + noteDraftColumns + ` FROM note_drafts
		WHERE status = 'open' ORDER BY id DESC LIMIT 1`))
}

// SetNoteDraftStatus closes one draft as skipped or superseded. An accepted
// draft stays open so a repeated or edited accept can report the same note.
func (s *Store) SetNoteDraftStatus(id int64, status string) error {
	if status != NoteDraftSkipped && status != NoteDraftSuperseded {
		return fmt.Errorf("store: note draft status %q is invalid", status)
	}
	res, err := s.db.Exec(`UPDATE note_drafts SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return fmt.Errorf("store: update note draft status: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: read note draft update: %w", err)
	}
	if affected != 1 {
		return ErrNotFound
	}
	return nil
}

// ErrNoteDraftChanged reports that a draft was accepted, skipped or superseded
// after a refinement read it, so the refinement no longer applies to it.
var ErrNoteDraftChanged = errors.New("store: note draft changed while it was being refined")

// SetNoteDraftRefinement stores an advisory refined front/back on a draft. It
// applies only while the draft is as observed when the refinement began: same
// status and same accepted content. A refinement that finishes after the
// learner accepted or skipped the draft changes nothing and returns
// ErrNoteDraftChanged, so it never erases an acceptance it did not see.
func (s *Store) SetNoteDraftRefinement(observed NoteDraft, refinementID int64, model, front, back string) error {
	res, err := s.db.Exec(`UPDATE note_drafts SET origin = 'refined', model = ?, refinement_id = ?,
		refined_front = ?, refined_back = ?,
		accepted_front = '', accepted_back = '', accepted_tactic = '', accepted_origin = '', accepted_model = ''
		WHERE id = ? AND status = ? AND accepted_front = ? AND accepted_back = ? AND accepted_tactic = ?
			AND accepted_origin = ? AND accepted_model = ?`,
		model, refinementID, front, back, observed.ID, observed.Status, observed.AcceptedFront, observed.AcceptedBack,
		observed.AcceptedTactic, observed.AcceptedOrigin, observed.AcceptedModel)
	if err != nil {
		return fmt.Errorf("store: refine note draft: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: read note draft refinement: %w", err)
	}
	if affected == 1 {
		return nil
	}
	if _, found, err := s.NoteDraftByID(observed.ID); err != nil {
		return fmt.Errorf("store: read note draft: %w", err)
	} else if found {
		return ErrNoteDraftChanged
	}
	return ErrNotFound
}
