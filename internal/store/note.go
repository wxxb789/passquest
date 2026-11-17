package store

import "time"

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
// tables are rebuilt. Only a completed deck read writes anki_deck, and the read
// sets it whole: an unconfirmed read records no deck.
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
	// completed deck read. A read that names no deck leaves it empty.
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
// the outbox row is left exactly as the last write left it. SetDeck marks the
// attempt as a completed deck read, so AnkiDeck is the note's whole deck claim:
// an empty one clears the name an earlier read recorded. An attempt that is not
// a deck read leaves the stored name untouched.
type AnkiAttempt struct {
	NoteID         string
	Revision       int
	Action         string
	Outcome        string
	Detail         string
	At             time.Time
	AnkiNoteID     int64
	AnkiDeck       string
	SetDeck        bool
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
