package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/tutor"
)

// schemaV10 adds the local note surface. Drafts are transient inputs, notes and
// revisions are canonical local content, the outbox is the Anki effect queue
// and the two ledgers make refinement and push retries countable. No table can
// update an attempt, a source, a question or an answer key.
var schemaV10 = []string{
	`CREATE TABLE IF NOT EXISTS note_drafts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		target_key TEXT NOT NULL CHECK (length(target_key) BETWEEN 1 AND 400),
		attempt_id INTEGER NOT NULL DEFAULT 0 CHECK (attempt_id >= 0),
		content_id TEXT NOT NULL CHECK (length(content_id) BETWEEN 1 AND 200),
		content_version TEXT NOT NULL DEFAULT '',
		question_id TEXT NOT NULL CHECK (length(question_id) BETWEEN 1 AND 200),
		source_id TEXT NOT NULL DEFAULT '',
		page_number INTEGER NOT NULL DEFAULT 0 CHECK (page_number >= 0),
		mark TEXT NOT NULL DEFAULT '',
		restricted INTEGER NOT NULL CHECK (restricted IN (0, 1)),
		origin TEXT NOT NULL CHECK (origin IN ('deterministic', 'refined')),
		model TEXT NOT NULL DEFAULT '',
		refinement_id INTEGER NOT NULL DEFAULT 0,
		refined_front TEXT NOT NULL DEFAULT '',
		refined_back TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL CHECK (status IN ('open', 'skipped', 'superseded')),
		created_at TEXT NOT NULL
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS note_drafts_open_target ON note_drafts(target_key) WHERE status = 'open'`,
	`CREATE INDEX IF NOT EXISTS note_drafts_status ON note_drafts(status, id)`,
	`CREATE TABLE IF NOT EXISTS notes (
		note_id TEXT PRIMARY KEY CHECK (length(note_id) BETWEEN 1 AND 80),
		target_key TEXT NOT NULL UNIQUE,
		attempt_id INTEGER NOT NULL DEFAULT 0,
		content_id TEXT NOT NULL,
		content_version TEXT NOT NULL,
		question_id TEXT NOT NULL,
		source_id TEXT NOT NULL DEFAULT '',
		page_number INTEGER NOT NULL DEFAULT 0,
		restricted INTEGER NOT NULL CHECK (restricted IN (0, 1)),
		revision INTEGER NOT NULL CHECK (revision >= 1),
		front TEXT NOT NULL,
		back TEXT NOT NULL,
		tactic TEXT NOT NULL,
		source_label TEXT NOT NULL,
		content_hash TEXT NOT NULL CHECK (length(content_hash) = 64),
		origin TEXT NOT NULL CHECK (origin IN ('deterministic', 'refined', 'edited')),
		model TEXT NOT NULL DEFAULT '',
		anki_note_id INTEGER NOT NULL DEFAULT 0 CHECK (anki_note_id >= 0),
		anki_pushed_revision INTEGER NOT NULL DEFAULT 0 CHECK (anki_pushed_revision >= 0),
		anki_state TEXT NOT NULL CHECK (anki_state IN ('local', 'queued', 'pushed', 'already_present', 'conflict', 'failed')),
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS note_revisions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		note_id TEXT NOT NULL,
		revision INTEGER NOT NULL CHECK (revision >= 1),
		front TEXT NOT NULL,
		back TEXT NOT NULL,
		tactic TEXT NOT NULL,
		source_label TEXT NOT NULL,
		content_hash TEXT NOT NULL CHECK (length(content_hash) = 64),
		origin TEXT NOT NULL CHECK (origin IN ('deterministic', 'refined', 'edited')),
		model TEXT NOT NULL DEFAULT '',
		restricted INTEGER NOT NULL CHECK (restricted IN (0, 1)),
		accepted_at TEXT NOT NULL,
		UNIQUE (note_id, revision)
	)`,
	`CREATE INDEX IF NOT EXISTS note_revisions_note ON note_revisions(note_id, revision)`,
	`CREATE TRIGGER IF NOT EXISTS note_revisions_append_only_update BEFORE UPDATE ON note_revisions
		BEGIN SELECT RAISE(ABORT, 'note revisions are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS note_revisions_append_only_delete BEFORE DELETE ON note_revisions
		BEGIN SELECT RAISE(ABORT, 'note revisions are append-only'); END`,
	`CREATE TABLE IF NOT EXISTS note_outbox (
		note_id TEXT PRIMARY KEY,
		revision INTEGER NOT NULL CHECK (revision >= 1),
		status TEXT NOT NULL CHECK (status IN ('pending', 'drained')),
		attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
		last_error TEXT NOT NULL DEFAULT '',
		enqueued_at TEXT NOT NULL,
		drained_at TEXT NOT NULL DEFAULT '',
		anki_note_id INTEGER NOT NULL DEFAULT 0 CHECK (anki_note_id >= 0)
	)`,
	`CREATE INDEX IF NOT EXISTS note_outbox_status ON note_outbox(status, enqueued_at)`,
	`CREATE TABLE IF NOT EXISTS anki_push_attempts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		note_id TEXT NOT NULL,
		revision INTEGER NOT NULL CHECK (revision >= 1),
		action TEXT NOT NULL CHECK (action IN ('add', 'update', 'readd', 'check')),
		outcome TEXT NOT NULL CHECK (outcome IN ('ok', 'duplicate', 'not_found', 'conflict', 'failed', 'unreachable')),
		detail TEXT NOT NULL DEFAULT '',
		at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS anki_push_attempts_note ON anki_push_attempts(note_id, id)`,
	`CREATE TRIGGER IF NOT EXISTS anki_push_attempts_append_only_update BEFORE UPDATE ON anki_push_attempts
		BEGIN SELECT RAISE(ABORT, 'anki push attempts are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS anki_push_attempts_append_only_delete BEFORE DELETE ON anki_push_attempts
		BEGIN SELECT RAISE(ABORT, 'anki push attempts are append-only'); END`,
	`CREATE TABLE IF NOT EXISTS note_refinements (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		draft_id INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('ok', 'error')),
		error_code TEXT NOT NULL DEFAULT '',
		error_message TEXT NOT NULL DEFAULT '',
		provider_host TEXT NOT NULL,
		model TEXT NOT NULL,
		prompt_version TEXT NOT NULL,
		request_sha256 TEXT NOT NULL CHECK (length(request_sha256) = 64),
		request_bytes INTEGER NOT NULL CHECK (request_bytes > 0),
		output_front TEXT NOT NULL DEFAULT '',
		output_back TEXT NOT NULL DEFAULT '',
		prompt_tokens INTEGER CHECK (prompt_tokens IS NULL OR prompt_tokens >= 0),
		completion_tokens INTEGER CHECK (completion_tokens IS NULL OR completion_tokens >= 0),
		http_status INTEGER NOT NULL DEFAULT 0 CHECK (http_status BETWEEN 0 AND 599),
		latency_ms INTEGER NOT NULL DEFAULT 0 CHECK (latency_ms >= 0),
		CHECK (status = 'ok' OR error_code <> ''),
		CHECK (status = 'error' OR (output_front <> '' AND output_back <> ''))
	)`,
	`CREATE INDEX IF NOT EXISTS note_refinements_draft ON note_refinements(draft_id, id)`,
	`CREATE TRIGGER IF NOT EXISTS note_refinements_append_only_update BEFORE UPDATE ON note_refinements
		BEGIN SELECT RAISE(ABORT, 'note refinements are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS note_refinements_append_only_delete BEFORE DELETE ON note_refinements
		BEGIN SELECT RAISE(ABORT, 'note refinements are append-only'); END`,
}

// schemaV11 records the learner's marked wording on the note row, so
// `note show` can report the third identity component without re-parsing
// target_key. Older rows keep an empty mark.
var schemaV11 = []string{
	`ALTER TABLE notes ADD COLUMN mark TEXT NOT NULL DEFAULT ''`,
}

// schemaV12 records the content of the last successful accept on its open
// draft. A repeated or incremental accept starts from these values instead of
// re-deriving the original draft, so an edit is never silently reverted. An
// empty accepted_front means the draft has not been accepted yet.
var schemaV12 = []string{
	`ALTER TABLE note_drafts ADD COLUMN accepted_front TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE note_drafts ADD COLUMN accepted_back TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE note_drafts ADD COLUMN accepted_tactic TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE note_drafts ADD COLUMN accepted_origin TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE note_drafts ADD COLUMN accepted_model TEXT NOT NULL DEFAULT ''`,
}

// schemaV16 records, on the note row, the deck Anki reported for the note's
// card at the last push, and widens two vocabularies: a note can be
// deck_mismatch (written, but not into the deck the push named) and one ledger
// row can report that verdict. SQLite cannot widen a CHECK in place, so both
// tables are rebuilt. A write never sets anki_deck: only a completed deck read
// does, so an unread deck records nothing.
var schemaV16 = []string{
	`DROP TRIGGER IF EXISTS anki_push_attempts_append_only_update`,
	`DROP TRIGGER IF EXISTS anki_push_attempts_append_only_delete`,
	`DROP INDEX IF EXISTS anki_push_attempts_note`,
	`ALTER TABLE notes RENAME TO notes_v15`,
	`CREATE TABLE notes (
		note_id TEXT PRIMARY KEY CHECK (length(note_id) BETWEEN 1 AND 80),
		target_key TEXT NOT NULL UNIQUE,
		attempt_id INTEGER NOT NULL DEFAULT 0,
		content_id TEXT NOT NULL,
		content_version TEXT NOT NULL,
		question_id TEXT NOT NULL,
		source_id TEXT NOT NULL DEFAULT '',
		page_number INTEGER NOT NULL DEFAULT 0,
		restricted INTEGER NOT NULL CHECK (restricted IN (0, 1)),
		revision INTEGER NOT NULL CHECK (revision >= 1),
		front TEXT NOT NULL,
		back TEXT NOT NULL,
		tactic TEXT NOT NULL,
		source_label TEXT NOT NULL,
		content_hash TEXT NOT NULL CHECK (length(content_hash) = 64),
		origin TEXT NOT NULL CHECK (origin IN ('deterministic', 'refined', 'edited')),
		model TEXT NOT NULL DEFAULT '',
		mark TEXT NOT NULL DEFAULT '',
		anki_note_id INTEGER NOT NULL DEFAULT 0 CHECK (anki_note_id >= 0),
		anki_pushed_revision INTEGER NOT NULL DEFAULT 0 CHECK (anki_pushed_revision >= 0),
		anki_deck TEXT NOT NULL DEFAULT '' CHECK (length(anki_deck) <= 200),
		anki_state TEXT NOT NULL CHECK (anki_state IN
			('local', 'queued', 'pushed', 'already_present', 'conflict', 'failed', 'deck_mismatch')),
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`INSERT INTO notes (
		note_id, target_key, attempt_id, content_id, content_version, question_id, source_id,
		page_number, restricted, revision, front, back, tactic, source_label, content_hash, origin, model,
		mark, anki_note_id, anki_pushed_revision, anki_deck, anki_state, created_at, updated_at
		)
		SELECT
		note_id, target_key, attempt_id, content_id, content_version, question_id, source_id,
		page_number, restricted, revision, front, back, tactic, source_label, content_hash, origin, model,
		mark, anki_note_id, anki_pushed_revision, '', anki_state, created_at, updated_at
		FROM notes_v15`,
	`DROP TABLE notes_v15`,
	`ALTER TABLE anki_push_attempts RENAME TO anki_push_attempts_v15`,
	`CREATE TABLE anki_push_attempts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		note_id TEXT NOT NULL,
		revision INTEGER NOT NULL CHECK (revision >= 1),
		action TEXT NOT NULL CHECK (action IN ('add', 'update', 'readd', 'check')),
		outcome TEXT NOT NULL CHECK (outcome IN
			('ok', 'duplicate', 'not_found', 'conflict', 'failed', 'unreachable', 'deck_mismatch')),
		detail TEXT NOT NULL DEFAULT '',
		at TEXT NOT NULL
	)`,
	`INSERT INTO anki_push_attempts (id, note_id, revision, action, outcome, detail, at)
		SELECT id, note_id, revision, action, outcome, detail, at FROM anki_push_attempts_v15`,
	`DROP TABLE anki_push_attempts_v15`,
	`CREATE INDEX IF NOT EXISTS anki_push_attempts_note ON anki_push_attempts(note_id, id)`,
	`CREATE TRIGGER IF NOT EXISTS anki_push_attempts_append_only_update BEFORE UPDATE ON anki_push_attempts
		BEGIN SELECT RAISE(ABORT, 'anki push attempts are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS anki_push_attempts_append_only_delete BEFORE DELETE ON anki_push_attempts
		BEGIN SELECT RAISE(ABORT, 'anki push attempts are append-only'); END`,
}

// Note draft states. A draft is a proposal, not evidence; only notes/revisions
// are canonical local content.
const (
	NoteDraftOpen       = "open"
	NoteDraftSkipped    = "skipped"
	NoteDraftSuperseded = "superseded"
)

// Anki synchronization states stored on a note.
const (
	AnkiStateLocal         = "local"
	AnkiStateQueued        = "queued"
	AnkiStatePushed        = "pushed"
	AnkiStateAlreadyExists = "already_present"
	AnkiStateConflict      = "conflict"
	AnkiStateFailed        = "failed"
	AnkiStateDeckMismatch  = "deck_mismatch"
)

// NoteDraft is one open proposal. RefinedFront/RefinedBack are advisory text
// retained until the learner accepts or skips the draft. AcceptedFront and
// friends hold the content of the last successful accept, so a repeated or
// incremental accept starts from the learner's current text.
type NoteDraft struct {
	ID             int64
	TargetKey      string
	AttemptID      int64
	ContentID      string
	ContentVersion string
	QuestionID     string
	SourceID       string
	Page           int
	Mark           string
	Restricted     bool
	Origin         string
	Model          string
	RefinementID   int64
	RefinedFront   string
	RefinedBack    string
	AcceptedFront  string
	AcceptedBack   string
	AcceptedTactic string
	AcceptedOrigin string
	AcceptedModel  string
	Status         string
	CreatedAt      time.Time
}

// Note is the current local content of one note.
type Note struct {
	NoteID             string
	TargetKey          string
	AttemptID          int64
	ContentID          string
	ContentVersion     string
	QuestionID         string
	SourceID           string
	Page               int
	Mark               string
	Restricted         bool
	Revision           int
	Front              string
	Back               string
	Tactic             string
	SourceLabel        string
	ContentHash        string
	Origin             string
	Model              string
	AnkiNoteID         int64
	AnkiPushedRevision int
	// AnkiDeck is the deck Anki reported for the note's card at the last
	// completed deck read.
	AnkiDeck  string
	AnkiState string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NoteRevision is one immutable accepted revision.
type NoteRevision struct {
	ID          int64
	NoteID      string
	Revision    int
	Front       string
	Back        string
	Tactic      string
	SourceLabel string
	ContentHash string
	Origin      string
	Model       string
	Restricted  bool
	AcceptedAt  time.Time
}

// NoteTarget is the identity passed to AcceptNote.
type NoteTarget struct {
	NoteID         string
	TargetKey      string
	AttemptID      int64
	ContentID      string
	ContentVersion string
	QuestionID     string
	SourceID       string
	Page           int
	Mark           string
	Restricted     bool
}

// NoteContent is one accepted revision's content.
type NoteContent struct {
	Front       string
	Back        string
	Tactic      string
	SourceLabel string
	ContentHash string
	Origin      string
	Model       string
}

// AcceptResult reports what AcceptNote did.
type AcceptResult struct {
	Note     Note
	Created  bool
	Changed  bool
	Previous int
}

// NoteOutboxEntry is the desired Anki revision for one note.
type NoteOutboxEntry struct {
	NoteID     string
	Revision   int
	Status     string
	Attempts   int
	LastError  string
	EnqueuedAt time.Time
	DrainedAt  time.Time
	AnkiNoteID int64
}

// AnkiAttempt is one external action outcome. The store appends the ledger row,
// updates the note's Anki state and advances the outbox in one transaction.
// Keep marks a read-only verdict: the row and the note state are written, and
// the outbox row is left exactly as the last write left it.
type AnkiAttempt struct {
	NoteID         string
	Revision       int
	Action         string
	Outcome        string
	Detail         string
	At             time.Time
	AnkiNoteID     int64
	AnkiDeck       string
	AnkiState      string
	PushedRevision int
	Drain          bool
	Keep           bool
}

// NoteRefinement is one recorded provider refinement.
type NoteRefinement struct {
	ID               int64
	DraftID          int64
	CreatedAt        time.Time
	Status           string
	ErrorCode        string
	ErrorMessage     string
	Provider         string
	ProviderHost     string
	Model            string
	PromptVersion    string
	RequestSHA256    string
	RequestBytes     int
	OutputFront      string
	OutputBack       string
	PromptTokens     *int
	CompletionTokens *int
	HTTPStatus       int
	LatencyMS        int64
}

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

// SetNoteDraftRefinement stores an advisory refined front/back on a draft.
func (s *Store) SetNoteDraftRefinement(id, refinementID int64, model, front, back string) error {
	res, err := s.db.Exec(`UPDATE note_drafts SET origin = 'refined', model = ?, refinement_id = ?,
		refined_front = ?, refined_back = ?,
		accepted_front = '', accepted_back = '', accepted_tactic = '', accepted_origin = '', accepted_model = ''
		WHERE id = ?`, model, refinementID, front, back, id)
	if err != nil {
		return fmt.Errorf("store: refine note draft: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: read note draft refinement: %w", err)
	}
	if affected != 1 {
		return ErrNotFound
	}
	return nil
}

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
			anki_deck = CASE WHEN ? <> '' THEN ? ELSE anki_deck END,
			anki_state = ?,
			anki_pushed_revision = CASE WHEN ? > 0 THEN ? ELSE anki_pushed_revision END,
			updated_at = ?
		WHERE note_id = ?`,
		a.AnkiNoteID, a.AnkiNoteID, a.AnkiDeck, a.AnkiDeck, state, pushed, pushed, stamp, a.NoteID); err != nil {
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

// ReserveNoteRefinement claims one slot in the shared daily provider budget.
// It uses the tutor reservation table, so a tutor ask and a note refinement
// cannot both pass the same limit.
func (s *Store) ReserveNoteRefinement(providerID string, since, at time.Time, limit int) (int64, int, error) {
	if limit <= 0 {
		return 0, 0, errors.New("store: provider daily call limit must be positive")
	}
	providerID = strings.TrimSpace(providerID)
	boundary := since.UTC().Format(time.RFC3339Nano)
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, fmt.Errorf("store: begin refinement reservation: %w", err)
	}
	defer tx.Rollback()
	var used int
	if err := tx.QueryRow(providerCallCountSQL(true), providerCallCountArgs(providerID, boundary, true)...).Scan(&used); err != nil {
		return 0, 0, fmt.Errorf("store: count provider budget: %w", err)
	}
	if used >= limit {
		return 0, used, nil
	}
	res, err := tx.Exec(`INSERT INTO tutor_call_reservations (provider, created_at) VALUES (?, ?)`,
		providerID, at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, 0, fmt.Errorf("store: reserve refinement: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, 0, fmt.Errorf("store: read refinement reservation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("store: commit refinement reservation: %w", err)
	}
	return id, used + 1, nil
}

// FinalizeNoteRefinement stores the refinement outcome and releases its
// reservation in one transaction.
func (s *Store) FinalizeNoteRefinement(r NoteRefinement, reservationID int64) (NoteRefinement, error) {
	if err := validateNoteRefinement(r); err != nil {
		return NoteRefinement{}, err
	}
	if reservationID <= 0 {
		return NoteRefinement{}, errors.New("store: refinement requires a budget reservation")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return NoteRefinement{}, fmt.Errorf("store: begin refinement: %w", err)
	}
	defer tx.Rollback()
	released, err := tx.Exec(`DELETE FROM tutor_call_reservations WHERE id = ?`, reservationID)
	if err != nil {
		return NoteRefinement{}, fmt.Errorf("store: release refinement reservation: %w", err)
	}
	held, err := released.RowsAffected()
	if err != nil {
		return NoteRefinement{}, fmt.Errorf("store: read refinement release: %w", err)
	}
	if held != 1 {
		return NoteRefinement{}, fmt.Errorf("store: refinement reservation %d is not held", reservationID)
	}
	res, err := tx.Exec(`INSERT INTO note_refinements (
			draft_id, created_at, status, error_code, error_message, provider, provider_host, model, prompt_version,
			request_sha256, request_bytes, output_front, output_back, prompt_tokens, completion_tokens,
			http_status, latency_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.DraftID, r.CreatedAt.UTC().Format(time.RFC3339Nano), r.Status, r.ErrorCode, r.ErrorMessage,
		r.Provider, r.ProviderHost, r.Model, r.PromptVersion, r.RequestSHA256, r.RequestBytes,
		r.OutputFront, r.OutputBack, nullableInt(r.PromptTokens), nullableInt(r.CompletionTokens),
		r.HTTPStatus, r.LatencyMS)
	if err != nil {
		return NoteRefinement{}, fmt.Errorf("store: append refinement: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return NoteRefinement{}, fmt.Errorf("store: read refinement id: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return NoteRefinement{}, fmt.Errorf("store: commit refinement: %w", err)
	}
	r.ID = id
	return r, nil
}

func validateNoteRefinement(r NoteRefinement) error {
	if r.CreatedAt.IsZero() {
		return errors.New("store: refinement timestamp is required")
	}
	switch r.Status {
	case "ok":
		if r.ErrorCode != "" {
			return errors.New("store: successful refinement carries an error code")
		}
		if strings.TrimSpace(r.OutputFront) == "" || strings.TrimSpace(r.OutputBack) == "" {
			return errors.New("store: successful refinement has no output")
		}
	case "error":
		if !tutor.ValidHostErrorCode(r.ErrorCode) {
			return fmt.Errorf("store: refinement error code %q is not documented", r.ErrorCode)
		}
		if strings.TrimSpace(r.ErrorMessage) == "" {
			return errors.New("store: failed refinement has no message")
		}
		if r.OutputFront != "" || r.OutputBack != "" {
			return errors.New("store: failed refinement carries output")
		}
	default:
		return fmt.Errorf("store: refinement status %q is invalid", r.Status)
	}
	if strings.TrimSpace(r.ProviderHost) == "" || strings.TrimSpace(r.Model) == "" {
		return errors.New("store: refinement requires provider host and model")
	}
	if strings.TrimSpace(r.PromptVersion) == "" {
		return errors.New("store: refinement prompt version is required")
	}
	if len(r.RequestSHA256) != 64 || strings.ToLower(r.RequestSHA256) != r.RequestSHA256 {
		return errors.New("store: refinement request hash must be 64 lowercase hex characters")
	}
	for _, char := range r.RequestSHA256 {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return errors.New("store: refinement request hash must be 64 lowercase hex characters")
		}
	}
	if r.RequestBytes <= 0 {
		return errors.New("store: refinement request size must be positive")
	}
	if r.HTTPStatus < 0 || r.HTTPStatus > 599 || r.LatencyMS < 0 {
		return errors.New("store: refinement transport numbers are invalid")
	}
	return nil
}

// NoteRefinements returns recorded refinements in insertion order.
func (s *Store) NoteRefinements() ([]NoteRefinement, error) {
	rows, err := s.db.Query(`SELECT id, draft_id, created_at, status, error_code, error_message,
			provider, provider_host, model, prompt_version, request_sha256, request_bytes, output_front,
			output_back, prompt_tokens, completion_tokens, http_status, latency_ms
		FROM note_refinements ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: list refinements: %w", err)
	}
	defer rows.Close()
	var out []NoteRefinement
	for rows.Next() {
		var (
			r                NoteRefinement
			created          string
			promptTokens     sql.NullInt64
			completionTokens sql.NullInt64
		)
		if err := rows.Scan(&r.ID, &r.DraftID, &created, &r.Status, &r.ErrorCode, &r.ErrorMessage,
			&r.Provider, &r.ProviderHost, &r.Model, &r.PromptVersion, &r.RequestSHA256, &r.RequestBytes,
			&r.OutputFront, &r.OutputBack, &promptTokens, &completionTokens, &r.HTTPStatus, &r.LatencyMS); err != nil {
			return nil, fmt.Errorf("store: scan refinement: %w", err)
		}
		if r.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, fmt.Errorf("store: parse refinement time: %w", err)
		}
		if promptTokens.Valid {
			value := int(promptTokens.Int64)
			r.PromptTokens = &value
		}
		if completionTokens.Valid {
			value := int(completionTokens.Int64)
			r.CompletionTokens = &value
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate refinements: %w", err)
	}
	return out, nil
}
