package store

import (
	"errors"
	"fmt"
	"time"
)

const noteOutboxColumns = `note_id, revision, status, attempts, last_error, enqueued_at, drained_at, anki_note_id`

func scanNoteOutbox(row rowScanner) (NoteOutboxEntry, error) {
	var (
		entry    NoteOutboxEntry
		enqueued string
		drained  string
	)
	if err := row.Scan(&entry.NoteID, &entry.Revision, &entry.Status, &entry.Attempts, &entry.LastError,
		&enqueued, &drained, &entry.AnkiNoteID); err != nil {
		return NoteOutboxEntry{}, err
	}
	var err error
	if entry.EnqueuedAt, err = time.Parse(time.RFC3339Nano, enqueued); err != nil {
		return NoteOutboxEntry{}, err
	}
	if drained != "" {
		if entry.DrainedAt, err = time.Parse(time.RFC3339Nano, drained); err != nil {
			return NoteOutboxEntry{}, err
		}
	}
	return entry, nil
}

// PendingNoteOutbox returns the notes the next push must drain.
func (s *Store) PendingNoteOutbox() ([]NoteOutboxEntry, error) {
	return s.noteOutboxWhere(`WHERE status = 'pending'`)
}

// NoteOutbox returns every queue row.
func (s *Store) NoteOutbox() ([]NoteOutboxEntry, error) {
	return s.noteOutboxWhere("")
}

func (s *Store) noteOutboxWhere(clause string) ([]NoteOutboxEntry, error) {
	rows, err := s.db.Query(`SELECT ` + noteOutboxColumns + ` FROM note_outbox ` + clause + ` ORDER BY enqueued_at, note_id`)
	if err != nil {
		return nil, fmt.Errorf("store: list note outbox: %w", err)
	}
	defer rows.Close()
	var out []NoteOutboxEntry
	for rows.Next() {
		entry, err := scanNoteOutbox(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan note outbox: %w", err)
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate note outbox: %w", err)
	}
	return out, nil
}

// RecordAnkiAttempt appends one external outcome, updates the note state and
// advances the outbox in one transaction. Exactly one drain is possible per
// revision because only a matching pending row is updated.
func (s *Store) RecordAnkiAttempt(a AnkiAttempt) error {
	switch a.Action {
	case "add", "update", "readd", "check":
	default:
		return fmt.Errorf("store: anki action %q is invalid", a.Action)
	}
	switch a.Outcome {
	case "ok", "duplicate", "not_found", "conflict", "failed", "unreachable", "deck_mismatch":
	default:
		return fmt.Errorf("store: anki outcome %q is invalid", a.Outcome)
	}
	if a.NoteID == "" || a.Revision < 1 || a.At.IsZero() {
		return errors.New("store: anki attempt requires note, revision and timestamp")
	}
	stamp := a.At.UTC().Format(time.RFC3339Nano)
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin anki attempt: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO anki_push_attempts (note_id, revision, action, outcome, detail, at)
		VALUES (?, ?, ?, ?, ?, ?)`, a.NoteID, a.Revision, a.Action, a.Outcome, a.Detail, stamp); err != nil {
		return fmt.Errorf("store: append anki attempt: %w", err)
	}
	state := a.AnkiState
	if state == "" {
		state = AnkiStateQueued
	}
	pushed := a.PushedRevision
	if _, err := tx.Exec(`UPDATE notes SET
			anki_note_id = CASE WHEN ? > 0 THEN ? ELSE anki_note_id END,
			anki_deck = CASE WHEN ? = 1 THEN ? ELSE anki_deck END,
			anki_state = ?,
			anki_pushed_revision = CASE WHEN ? > 0 THEN ? ELSE anki_pushed_revision END,
			updated_at = ?
		WHERE note_id = ?`,
		a.AnkiNoteID, a.AnkiNoteID, boolInt(a.SetDeck), a.AnkiDeck, state, pushed, pushed, stamp, a.NoteID); err != nil {
		return fmt.Errorf("store: update note anki state: %w", err)
	}
	switch {
	case a.Keep:
	case a.Drain:
		if _, err := tx.Exec(`UPDATE note_outbox SET status = 'drained', drained_at = ?, last_error = '',
				anki_note_id = CASE WHEN ? > 0 THEN ? ELSE anki_note_id END
			WHERE note_id = ? AND revision = ? AND status = 'pending'`,
			stamp, a.AnkiNoteID, a.AnkiNoteID, a.NoteID, a.Revision); err != nil {
			return fmt.Errorf("store: drain note outbox: %w", err)
		}
	default:
		if _, err := tx.Exec(`UPDATE note_outbox SET status = 'pending', attempts = attempts + 1, last_error = ?,
				anki_note_id = CASE WHEN ? > 0 THEN ? ELSE anki_note_id END
			WHERE note_id = ?`, a.Detail, a.AnkiNoteID, a.AnkiNoteID, a.NoteID); err != nil {
			return fmt.Errorf("store: update note outbox: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit anki attempt: %w", err)
	}
	return nil
}
