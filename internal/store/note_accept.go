package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AcceptNote inserts or revises one note in a single transaction. Equal content
// is a no-op; different content appends a revision and enqueues the note.
func (s *Store) AcceptNote(target NoteTarget, content NoteContent, at time.Time) (AcceptResult, error) {
	return s.acceptNote(target, content, 0, "", at)
}

// AcceptDraftNote accepts a note and records its content on the open draft in
// the same transaction. A later accept of that draft starts from the accepted
// values instead of re-deriving the original proposal.
func (s *Store) AcceptDraftNote(draftID int64, target NoteTarget, content NoteContent, at time.Time) (AcceptResult, error) {
	if draftID <= 0 {
		return AcceptResult{}, errors.New("store: note draft id is required")
	}
	return s.acceptNote(target, content, draftID, target.TargetKey, at)
}

func (s *Store) acceptNote(target NoteTarget, content NoteContent, draftID int64, targetKey string, at time.Time) (AcceptResult, error) {
	if strings.TrimSpace(target.NoteID) == "" || strings.TrimSpace(target.TargetKey) == "" {
		return AcceptResult{}, errors.New("store: note id and target key are required")
	}
	if len(content.ContentHash) != 64 {
		return AcceptResult{}, errors.New("store: note content hash must be 64 characters")
	}
	if content.Origin != "deterministic" && content.Origin != "refined" && content.Origin != "edited" {
		return AcceptResult{}, fmt.Errorf("store: note origin %q is invalid", content.Origin)
	}
	if at.IsZero() {
		return AcceptResult{}, errors.New("store: note accept timestamp is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AcceptResult{}, fmt.Errorf("store: begin note accept: %w", err)
	}
	defer tx.Rollback()

	existing, found, err := scanNote(tx.QueryRow(`SELECT `+noteColumns+` FROM notes WHERE target_key = ?`, target.TargetKey))
	if err != nil {
		return AcceptResult{}, fmt.Errorf("store: read note: %w", err)
	}
	stamp := at.UTC().Format(time.RFC3339Nano)
	if found {
		if existing.ContentHash == content.ContentHash {
			if err := recordDraftAccepted(tx, draftID, targetKey, content); err != nil {
				return AcceptResult{}, err
			}
			if err := tx.Commit(); err != nil {
				return AcceptResult{}, fmt.Errorf("store: commit unchanged note: %w", err)
			}
			return AcceptResult{Note: existing, Previous: existing.Revision}, nil
		}
		revision := existing.Revision + 1
		if _, err := tx.Exec(`INSERT INTO note_revisions (
				note_id, revision, front, back, tactic, source_label, content_hash, origin, model, restricted, accepted_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			existing.NoteID, revision, content.Front, content.Back, content.Tactic, content.SourceLabel,
			content.ContentHash, content.Origin, content.Model, boolInt(target.Restricted), stamp); err != nil {
			return AcceptResult{}, fmt.Errorf("store: insert note revision: %w", err)
		}
		if _, err := tx.Exec(`UPDATE notes SET revision = ?, front = ?, back = ?, tactic = ?, source_label = ?,
				content_hash = ?, origin = ?, model = ?, anki_state = 'local', anki_deck = '', updated_at = ?
			WHERE note_id = ?`,
			revision, content.Front, content.Back, content.Tactic, content.SourceLabel,
			content.ContentHash, content.Origin, content.Model, stamp, existing.NoteID); err != nil {
			return AcceptResult{}, fmt.Errorf("store: update note: %w", err)
		}
		if err := upsertNoteOutbox(tx, existing.NoteID, revision, stamp); err != nil {
			return AcceptResult{}, err
		}
		if err := recordDraftAccepted(tx, draftID, targetKey, content); err != nil {
			return AcceptResult{}, err
		}
		updated, _, err := scanNote(tx.QueryRow(`SELECT `+noteColumns+` FROM notes WHERE note_id = ?`, existing.NoteID))
		if err != nil {
			return AcceptResult{}, fmt.Errorf("store: reread note: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return AcceptResult{}, fmt.Errorf("store: commit note revision: %w", err)
		}
		return AcceptResult{Note: updated, Changed: true, Previous: existing.Revision}, nil
	}

	if _, err := tx.Exec(`INSERT INTO notes (
			note_id, target_key, attempt_id, content_id, content_version, question_id, source_id, page_number,
			mark, restricted, revision, front, back, tactic, source_label, content_hash, origin, model,
			anki_state, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, 'local', ?, ?)`,
		target.NoteID, target.TargetKey, target.AttemptID, target.ContentID, target.ContentVersion,
		target.QuestionID, target.SourceID, target.Page, target.Mark, boolInt(target.Restricted),
		content.Front, content.Back, content.Tactic, content.SourceLabel, content.ContentHash,
		content.Origin, content.Model, stamp, stamp); err != nil {
		return AcceptResult{}, fmt.Errorf("store: insert note: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO note_revisions (
			note_id, revision, front, back, tactic, source_label, content_hash, origin, model, restricted, accepted_at
		) VALUES (?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		target.NoteID, content.Front, content.Back, content.Tactic, content.SourceLabel,
		content.ContentHash, content.Origin, content.Model, boolInt(target.Restricted), stamp); err != nil {
		return AcceptResult{}, fmt.Errorf("store: insert first note revision: %w", err)
	}
	if err := upsertNoteOutbox(tx, target.NoteID, 1, stamp); err != nil {
		return AcceptResult{}, err
	}
	if err := recordDraftAccepted(tx, draftID, targetKey, content); err != nil {
		return AcceptResult{}, err
	}
	created, _, err := scanNote(tx.QueryRow(`SELECT `+noteColumns+` FROM notes WHERE note_id = ?`, target.NoteID))
	if err != nil {
		return AcceptResult{}, fmt.Errorf("store: reread created note: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return AcceptResult{}, fmt.Errorf("store: commit note: %w", err)
	}
	return AcceptResult{Note: created, Created: true, Changed: true}, nil
}

// recordDraftAccepted stores the accepted content on the draft in the same
// transaction as the note write. A draft that closed between the read and this
// write makes the whole accept fail rather than leaving the two out of step.
func recordDraftAccepted(tx *sql.Tx, draftID int64, targetKey string, content NoteContent) error {
	if draftID == 0 {
		return nil
	}
	res, err := tx.Exec(`UPDATE note_drafts SET
			accepted_front = ?, accepted_back = ?, accepted_tactic = ?, accepted_origin = ?, accepted_model = ?
		WHERE id = ? AND status = 'open' AND target_key = ?`,
		content.Front, content.Back, content.Tactic, content.Origin, content.Model, draftID, targetKey)
	if err != nil {
		return fmt.Errorf("store: record accepted draft content: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: read accepted draft update: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("store: note draft %d is not open for this target", draftID)
	}
	return nil
}

func upsertNoteOutbox(tx *sql.Tx, noteID string, revision int, stamp string) error {
	if _, err := tx.Exec(`INSERT INTO note_outbox (note_id, revision, status, attempts, last_error, enqueued_at)
		VALUES (?, ?, 'pending', 0, '', ?)
		ON CONFLICT(note_id) DO UPDATE SET
			revision = excluded.revision,
			status = 'pending',
			attempts = 0,
			last_error = '',
			enqueued_at = excluded.enqueued_at,
			drained_at = ''`, noteID, revision, stamp); err != nil {
		return fmt.Errorf("store: enqueue note: %w", err)
	}
	return nil
}
