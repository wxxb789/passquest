package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const noteColumns = `note_id, target_key, attempt_id, content_id, content_version, question_id, source_id,
	page_number, mark, restricted, revision, front, back, tactic, source_label, content_hash, origin, model,
	anki_note_id, anki_pushed_revision, anki_deck, anki_state, created_at, updated_at`

func scanNote(row rowScanner) (Note, bool, error) {
	var (
		n          Note
		restricted int
		created    string
		updated    string
	)
	err := row.Scan(&n.NoteID, &n.TargetKey, &n.AttemptID, &n.ContentID, &n.ContentVersion, &n.QuestionID,
		&n.SourceID, &n.Page, &n.Mark, &restricted, &n.Revision, &n.Front, &n.Back, &n.Tactic, &n.SourceLabel,
		&n.ContentHash, &n.Origin, &n.Model, &n.AnkiNoteID, &n.AnkiPushedRevision, &n.AnkiDeck, &n.AnkiState,
		&created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Note{}, false, nil
	}
	if err != nil {
		return Note{}, false, err
	}
	if n.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return Note{}, false, err
	}
	if n.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return Note{}, false, err
	}
	n.Restricted = restricted != 0
	return n, true, nil
}

// Notes returns every accepted note ordered by local id.
func (s *Store) Notes() ([]Note, error) {
	rows, err := s.db.Query(`SELECT ` + noteColumns + ` FROM notes ORDER BY note_id`)
	if err != nil {
		return nil, fmt.Errorf("store: list notes: %w", err)
	}
	defer rows.Close()
	var out []Note
	for rows.Next() {
		note, _, err := scanNote(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan note: %w", err)
		}
		out = append(out, note)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate notes: %w", err)
	}
	return out, nil
}

// Note loads one note.
func (s *Store) Note(noteID string) (Note, error) {
	note, found, err := scanNote(s.db.QueryRow(`SELECT `+noteColumns+` FROM notes WHERE note_id = ?`, noteID))
	if err != nil {
		return Note{}, fmt.Errorf("store: read note: %w", err)
	}
	if !found {
		return Note{}, ErrNotFound
	}
	return note, nil
}

const noteRevisionColumns = `id, note_id, revision, front, back, tactic, source_label, content_hash,
	origin, model, restricted, accepted_at`

func scanNoteRevision(row rowScanner) (NoteRevision, error) {
	var (
		r          NoteRevision
		restricted int
		accepted   string
	)
	if err := row.Scan(&r.ID, &r.NoteID, &r.Revision, &r.Front, &r.Back, &r.Tactic, &r.SourceLabel,
		&r.ContentHash, &r.Origin, &r.Model, &restricted, &accepted); err != nil {
		return NoteRevision{}, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, accepted)
	if err != nil {
		return NoteRevision{}, err
	}
	r.AcceptedAt = parsed
	r.Restricted = restricted != 0
	return r, nil
}

// NoteRevisions returns the immutable history in revision order.
func (s *Store) NoteRevisions(noteID string) ([]NoteRevision, error) {
	rows, err := s.db.Query(`SELECT `+noteRevisionColumns+` FROM note_revisions
		WHERE note_id = ? ORDER BY revision`, noteID)
	if err != nil {
		return nil, fmt.Errorf("store: list note revisions: %w", err)
	}
	defer rows.Close()
	var out []NoteRevision
	for rows.Next() {
		revision, err := scanNoteRevision(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan note revision: %w", err)
		}
		out = append(out, revision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate note revisions: %w", err)
	}
	return out, nil
}

// NoteRevision loads one historical revision.
func (s *Store) NoteRevision(noteID string, revision int) (NoteRevision, error) {
	row := s.db.QueryRow(`SELECT `+noteRevisionColumns+` FROM note_revisions
		WHERE note_id = ? AND revision = ?`, noteID, revision)
	got, err := scanNoteRevision(row)
	if errors.Is(err, sql.ErrNoRows) {
		return NoteRevision{}, ErrNotFound
	}
	if err != nil {
		return NoteRevision{}, fmt.Errorf("store: read note revision: %w", err)
	}
	return got, nil
}
