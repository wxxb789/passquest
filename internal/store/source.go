package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/source"
)

// ErrNotFound distinguishes a missing source, page or question from a storage
// failure so the CLI can return a usage error.
var ErrNotFound = errors.New("store: not found")

// Source event kinds. Events are append-only provenance for every correction
// and review.
const (
	EventImport           = "import"
	EventPageCorrected    = "page_corrected"
	EventPageReviewed     = "page_reviewed"
	EventQuestionAdded    = "question_added"
	EventQuestionEdited   = "question_edited"
	EventQuestionReviewed = "question_reviewed"
)

var schemaV3 = []string{
	`CREATE TABLE IF NOT EXISTS sources (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		import_origin TEXT NOT NULL CHECK (import_origin IN ('txt-directory', 'txt-file', 'pdf')),
		path TEXT NOT NULL,
		sha256 TEXT NOT NULL,
		notice TEXT NOT NULL,
		rights TEXT NOT NULL,
		exam_relation TEXT NOT NULL,
		created_at TEXT NOT NULL,
		page_count INTEGER NOT NULL CHECK (page_count >= 1)
	)`,
	`CREATE TABLE IF NOT EXISTS source_pages (
		source_id TEXT NOT NULL REFERENCES sources(id),
		page INTEGER NOT NULL CHECK (page >= 1),
		origin TEXT NOT NULL CHECK (origin IN ('native', 'ocr', 'none', 'unknown', 'manual')),
		section TEXT NOT NULL DEFAULT '',
		review_status TEXT NOT NULL DEFAULT 'unreviewed' CHECK (review_status IN ('unreviewed', 'reviewed')),
		flags TEXT NOT NULL DEFAULT '',
		review_note TEXT NOT NULL DEFAULT '',
		reviewed_by TEXT NOT NULL DEFAULT '',
		reviewed_at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (source_id, page)
	)`,
	`CREATE TABLE IF NOT EXISTS page_revisions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_id TEXT NOT NULL REFERENCES sources(id),
		page INTEGER NOT NULL CHECK (page >= 1),
		revision INTEGER NOT NULL CHECK (revision >= 1),
		text TEXT NOT NULL,
		origin TEXT NOT NULL CHECK (origin IN ('native', 'ocr', 'none', 'unknown', 'manual')),
		created_at TEXT NOT NULL,
		note TEXT NOT NULL DEFAULT '',
		UNIQUE (source_id, page, revision)
	)`,
	`CREATE TRIGGER IF NOT EXISTS page_revisions_append_only_update BEFORE UPDATE ON page_revisions
		BEGIN SELECT RAISE(ABORT, 'page revisions are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS page_revisions_append_only_delete BEFORE DELETE ON page_revisions
		BEGIN SELECT RAISE(ABORT, 'page revisions are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS page_revisions_append_only_insert BEFORE INSERT ON page_revisions
		WHEN EXISTS (SELECT 1 FROM page_revisions WHERE id = NEW.id)
		BEGIN SELECT RAISE(ABORT, 'page revisions are append-only'); END`,
	`CREATE TABLE IF NOT EXISTS source_questions (
		source_id TEXT NOT NULL REFERENCES sources(id),
		id TEXT NOT NULL,
		page INTEGER NOT NULL CHECK (page >= 1),
		stem TEXT NOT NULL,
		options_json TEXT NOT NULL,
		answer TEXT NOT NULL DEFAULT '',
		evidence TEXT NOT NULL,
		reasoning TEXT NOT NULL,
		option_reasoning_json TEXT NOT NULL,
		answer_status TEXT NOT NULL CHECK (answer_status IN ('unmapped', 'reviewed')),
		review_status TEXT NOT NULL CHECK (review_status IN ('unreviewed', 'reviewed')),
		reviewed_by TEXT NOT NULL DEFAULT '',
		reviewed_at TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		PRIMARY KEY (source_id, id)
	)`,
	`CREATE INDEX IF NOT EXISTS source_questions_page ON source_questions(source_id, page, id)`,
	`CREATE TABLE IF NOT EXISTS source_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_id TEXT NOT NULL REFERENCES sources(id),
		at TEXT NOT NULL,
		kind TEXT NOT NULL,
		page INTEGER NOT NULL DEFAULT 0,
		question_id TEXT NOT NULL DEFAULT '',
		detail TEXT NOT NULL
	)`,
	`CREATE TRIGGER IF NOT EXISTS source_events_append_only_update BEFORE UPDATE ON source_events
		BEGIN SELECT RAISE(ABORT, 'source events are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS source_events_append_only_delete BEFORE DELETE ON source_events
		BEGIN SELECT RAISE(ABORT, 'source events are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS source_events_append_only_insert BEFORE INSERT ON source_events
		WHEN EXISTS (SELECT 1 FROM source_events WHERE id = NEW.id)
		BEGIN SELECT RAISE(ABORT, 'source events are append-only'); END`,
}

// ImportSource stores one imported source and its page revisions. It is
// idempotent: an existing source id with the same sha256 returns created=false
// and writes nothing. A different hash under the same id is a storage error.
func (s *Store) ImportSource(src source.Source) (bool, error) {
	if strings.TrimSpace(src.ID) == "" || strings.TrimSpace(src.Title) == "" {
		return false, errors.New("store: source id and title are required")
	}
	if len(src.Pages) == 0 {
		return false, errors.New("store: source has no pages")
	}
	for i, page := range src.Pages {
		if page.Number != i+1 {
			return false, fmt.Errorf("store: source page %d is out of order at index %d", page.Number, i)
		}
		if !utf8.ValidString(page.Text) {
			return false, fmt.Errorf("store: source page %d is not valid UTF-8", page.Number)
		}
		if page.Review != source.Unreviewed || page.ReviewedBy != "" || !page.ReviewedAt.IsZero() || page.ReviewNote != "" {
			return false, fmt.Errorf("store: imported page %d must start unreviewed", page.Number)
		}
	}
	if !src.Import.Valid() {
		return false, fmt.Errorf("store: invalid import origin %q", src.Import)
	}
	createdAt := src.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	tx, err := s.db.Begin()
	if err != nil {
		return false, fmt.Errorf("store: begin import: %w", err)
	}
	defer tx.Rollback()

	var existingSHA string
	err = tx.QueryRow(`SELECT sha256 FROM sources WHERE id = ?`, src.ID).Scan(&existingSHA)
	switch {
	case err == nil:
		if existingSHA != src.SHA256 {
			return false, fmt.Errorf("store: source %s exists with a different content hash", src.ID)
		}
		return false, nil
	case !errors.Is(err, sql.ErrNoRows):
		return false, fmt.Errorf("store: check source: %w", err)
	}

	if _, err := tx.Exec(`INSERT INTO sources (
			id, title, import_origin, path, sha256, notice, rights, exam_relation, created_at, page_count
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		src.ID, src.Title, string(src.Import), src.Path, src.SHA256, src.Notice, src.Rights, src.ExamRelation,
		source.FormatTime(createdAt), len(src.Pages),
	); err != nil {
		return false, fmt.Errorf("store: insert source: %w", err)
	}
	for _, page := range src.Pages {
		if !page.Origin.Valid() {
			return false, fmt.Errorf("store: source page %d has invalid origin %q", page.Number, page.Origin)
		}
		if _, err := tx.Exec(`INSERT INTO source_pages (
				source_id, page, origin, section, review_status, flags, review_note, reviewed_by, reviewed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			src.ID, page.Number, string(page.Origin), page.Section, string(source.Unreviewed),
			source.FormatFlags(page.Flags), "", "", "",
		); err != nil {
			return false, fmt.Errorf("store: insert source page %d: %w", page.Number, err)
		}
		if _, err := tx.Exec(`INSERT INTO page_revisions (
				source_id, page, revision, text, origin, created_at, note
			) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			src.ID, page.Number, 1, page.Text, string(page.Origin), source.FormatTime(createdAt), "imported",
		); err != nil {
			return false, fmt.Errorf("store: insert page revision %d: %w", page.Number, err)
		}
	}
	detail := fmt.Sprintf(`{"pages":%d,"sha256":%q}`, len(src.Pages), src.SHA256)
	if err := appendEvent(tx, src.ID, createdAt, EventImport, 0, "", detail); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("store: commit import: %w", err)
	}
	return true, nil
}

func (s *Store) Sources() ([]source.Summary, error) {
	rows, err := s.db.Query(`SELECT
			s.id, s.title, s.import_origin, s.path, s.sha256, s.created_at, s.page_count,
			(SELECT COUNT(*) FROM source_pages p WHERE p.source_id = s.id AND p.review_status = 'reviewed'),
			(SELECT COUNT(*) FROM source_questions q WHERE q.source_id = s.id),
			(SELECT COUNT(*) FROM source_questions q WHERE q.source_id = s.id
				AND q.review_status = 'reviewed' AND q.answer_status = 'reviewed')
		FROM sources s ORDER BY s.created_at, s.id`)
	if err != nil {
		return nil, fmt.Errorf("store: list sources: %w", err)
	}
	defer rows.Close()

	var out []source.Summary
	for rows.Next() {
		var (
			summary source.Summary
			origin  string
			created string
		)
		if err := rows.Scan(&summary.ID, &summary.Title, &origin, &summary.Path, &summary.SHA256,
			&created, &summary.PageCount, &summary.ReviewedPages, &summary.QuestionCount,
			&summary.ReviewedQuestions); err != nil {
			return nil, fmt.Errorf("store: scan source: %w", err)
		}
		summary.Import = source.ImportOrigin(origin)
		if summary.CreatedAt, err = parseTime(created); err != nil {
			return nil, fmt.Errorf("store: parse source %s created_at: %w", summary.ID, err)
		}
		out = append(out, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate sources: %w", err)
	}
	return out, nil
}

// SourceMeta reads one source's metadata without its page texts.
func (s *Store) SourceMeta(id string) (source.Source, error) {
	var (
		src     source.Source
		origin  string
		created string
	)
	err := s.db.QueryRow(`SELECT id, title, import_origin, path, sha256, notice, rights, exam_relation, created_at
		FROM sources WHERE id = ?`, id).Scan(
		&src.ID, &src.Title, &origin, &src.Path, &src.SHA256, &src.Notice, &src.Rights,
		&src.ExamRelation, &created,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return source.Source{}, fmt.Errorf("%w: source %q", ErrNotFound, id)
		}
		return source.Source{}, fmt.Errorf("store: read source %s: %w", id, err)
	}
	src.Import = source.ImportOrigin(origin)
	if src.CreatedAt, err = parseTime(created); err != nil {
		return source.Source{}, fmt.Errorf("store: parse source %s created_at: %w", id, err)
	}
	return src, nil
}

func (s *Store) Source(id string) (source.Source, error) {
	src, err := s.SourceMeta(id)
	if err != nil {
		return source.Source{}, err
	}

	rows, err := s.db.Query(pageSelect+` ORDER BY p.page`, id)
	if err != nil {
		return source.Source{}, fmt.Errorf("store: read pages of %s: %w", id, err)
	}
	defer rows.Close()
	for rows.Next() {
		page, err := scanPage(rows)
		if err != nil {
			return source.Source{}, fmt.Errorf("store: scan page of %s: %w", id, err)
		}
		src.Pages = append(src.Pages, page)
	}
	if err := rows.Err(); err != nil {
		return source.Source{}, fmt.Errorf("store: iterate pages of %s: %w", id, err)
	}
	return src, nil
}

const (
	pageSelect = `SELECT
			p.page, p.origin, p.section, p.review_status, p.flags, p.review_note, p.reviewed_by, p.reviewed_at,
			r.revision, r.text
		FROM source_pages p
		JOIN page_revisions r ON r.source_id = p.source_id AND r.page = p.page
		WHERE p.source_id = ?
			AND r.revision = (SELECT MAX(revision) FROM page_revisions WHERE source_id = p.source_id AND page = p.page)`

	questionSelect = `SELECT
			source_id, id, page, stem, options_json, answer, evidence, reasoning, option_reasoning_json,
			answer_status, review_status, reviewed_by, reviewed_at, created_at, updated_at, tactics_json,
			glossary_json
		FROM source_questions`

	attemptSelect = `SELECT
			id, session_id, question_id, content_id, content_version, status,
			selected_label, selected_text, correct_label, correct_text,
			started_at, answered_at, elapsed_ms, assistance, tactic_id, skills_json,
			assessment_run_id
		FROM attempts`
)

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPage(row rowScanner) (source.Page, error) {
	var (
		page         source.Page
		pageOrigin   string
		reviewStatus string
		flags        string
		reviewedAt   string
	)
	if err := row.Scan(&page.Number, &pageOrigin, &page.Section, &reviewStatus, &flags,
		&page.ReviewNote, &page.ReviewedBy, &reviewedAt, &page.Revision, &page.Text); err != nil {
		return source.Page{}, err
	}
	page.Origin = source.PageOrigin(pageOrigin)
	page.Review = source.ReviewStatus(reviewStatus)
	page.Flags = source.ParseFlags(flags)
	var err error
	if page.ReviewedAt, err = parseOptionalTime(reviewedAt); err != nil {
		return source.Page{}, fmt.Errorf("parse page %d reviewed_at: %w", page.Number, err)
	}
	return page, nil
}

func scanQuestion(row rowScanner) (source.Question, error) {
	var (
		q             source.Question
		optionsJSON   string
		reasoningJSON string
		tacticsJSON   string
		glossaryJSON  string
		answerStatus  string
		reviewStatus  string
		reviewedAt    string
		createdAt     string
		updatedAt     string
	)
	if err := row.Scan(&q.SourceID, &q.ID, &q.Page, &q.Stem, &optionsJSON, &q.Answer,
		&q.Evidence, &q.Reasoning, &reasoningJSON, &answerStatus, &reviewStatus,
		&q.ReviewedBy, &reviewedAt, &createdAt, &updatedAt, &tacticsJSON, &glossaryJSON); err != nil {
		return source.Question{}, err
	}
	if err := json.Unmarshal([]byte(optionsJSON), &q.Options); err != nil {
		return source.Question{}, fmt.Errorf("decode options of question %s/%s: %w", q.SourceID, q.ID, err)
	}
	if err := json.Unmarshal([]byte(reasoningJSON), &q.OptionReasoning); err != nil {
		return source.Question{}, fmt.Errorf("decode option reasoning of question %s/%s: %w", q.SourceID, q.ID, err)
	}
	if err := json.Unmarshal([]byte(tacticsJSON), &q.Tactics); err != nil {
		return source.Question{}, fmt.Errorf("decode tactic tags of question %s/%s: %w", q.SourceID, q.ID, err)
	}
	if err := json.Unmarshal([]byte(glossaryJSON), &q.Glossary); err != nil {
		return source.Question{}, fmt.Errorf("decode glossary of question %s/%s: %w", q.SourceID, q.ID, err)
	}
	q.AnswerStatus = source.AnswerStatus(answerStatus)
	q.Review = source.ReviewStatus(reviewStatus)
	var err error
	if q.ReviewedAt, err = parseOptionalTime(reviewedAt); err != nil {
		return source.Question{}, fmt.Errorf("parse question %s reviewed_at: %w", q.ID, err)
	}
	if q.CreatedAt, err = parseTime(createdAt); err != nil {
		return source.Question{}, fmt.Errorf("parse question %s created_at: %w", q.ID, err)
	}
	if q.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return source.Question{}, fmt.Errorf("parse question %s updated_at: %w", q.ID, err)
	}
	return q, nil
}

func (s *Store) Questions(sourceID string) ([]source.Question, error) {
	rows, err := s.db.Query(questionSelect+` WHERE source_id = ? ORDER BY page, id`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("store: list questions of %s: %w", sourceID, err)
	}
	defer rows.Close()

	var out []source.Question
	for rows.Next() {
		q, err := scanQuestion(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan question: %w", err)
		}
		out = append(out, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate questions of %s: %w", sourceID, err)
	}
	return out, nil
}

func (s *Store) Question(sourceID, questionID string) (source.Question, error) {
	row := s.db.QueryRow(questionSelect+` WHERE source_id = ? AND id = ?`, sourceID, questionID)
	q, err := scanQuestion(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return source.Question{}, fmt.Errorf("%w: question %q in source %s", ErrNotFound, questionID, sourceID)
		}
		return source.Question{}, fmt.Errorf("store: read question %q in source %s: %w", questionID, sourceID, err)
	}
	return q, nil
}

func (s *Store) Page(sourceID string, page int) (source.Page, error) {
	row := s.db.QueryRow(pageSelect+` AND p.page = ?`, sourceID, page)
	out, err := scanPage(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return source.Page{}, fmt.Errorf("%w: page %d in source %s", ErrNotFound, page, sourceID)
		}
		return source.Page{}, fmt.Errorf("store: read page %d in source %s: %w", page, sourceID, err)
	}
	return out, nil
}

// CorrectPage appends a new text revision and resets the page review: the old
// review described different text.
func (s *Store) CorrectPage(sourceID string, page int, text, note, reviewer string, at time.Time) (source.Page, error) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	if !utf8.ValidString(text) {
		return source.Page{}, errors.New("store: corrected page text is not valid UTF-8")
	}
	reviewer = strings.TrimSpace(reviewer)
	if reviewer == "" {
		return source.Page{}, errors.New("store: reviewer is required")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return source.Page{}, fmt.Errorf("store: begin page correction: %w", err)
	}
	defer tx.Rollback()

	var (
		origin   string
		flags    string
		revision int
	)
	err = tx.QueryRow(`SELECT p.origin, p.flags,
			(SELECT MAX(revision) FROM page_revisions WHERE source_id = p.source_id AND page = p.page)
		FROM source_pages p WHERE p.source_id = ? AND p.page = ?`, sourceID, page).Scan(&origin, &flags, &revision)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return source.Page{}, fmt.Errorf("%w: page %d in source %s", ErrNotFound, page, sourceID)
		}
		return source.Page{}, fmt.Errorf("store: read page %d: %w", page, err)
	}
	next := revision + 1
	newFlags := source.FlagsAfterCorrection(source.ParseFlags(flags), text)

	if _, err := tx.Exec(`INSERT INTO page_revisions (
			source_id, page, revision, text, origin, created_at, note
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sourceID, page, next, text, string(source.PageManual), source.FormatTime(at), note,
	); err != nil {
		return source.Page{}, fmt.Errorf("store: insert page revision: %w", err)
	}
	if _, err := tx.Exec(`UPDATE source_pages SET origin = ?, flags = ?, review_status = ?,
			review_note = '', reviewed_by = '', reviewed_at = '' WHERE source_id = ? AND page = ?`,
		string(source.PageManual), source.FormatFlags(newFlags), string(source.Unreviewed), sourceID, page,
	); err != nil {
		return source.Page{}, fmt.Errorf("store: update page correction: %w", err)
	}
	detail := fmt.Sprintf(`{"revision":%d,"previous_revision":%d,"previous_origin":%q,"reviewer":%q,"note":%q,"chars_no_ws":%d}`,
		next, revision, origin, reviewer, note, source.CountNonSpace(text))
	if err := appendEvent(tx, sourceID, at, EventPageCorrected, page, "", detail); err != nil {
		return source.Page{}, err
	}
	if err := tx.Commit(); err != nil {
		return source.Page{}, fmt.Errorf("store: commit page correction: %w", err)
	}
	return s.Page(sourceID, page)
}

// ReviewPage records the user's statement that the current page text was
// inspected.
func (s *Store) ReviewPage(sourceID string, page int, reviewer, note string, at time.Time) (source.Page, error) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	reviewer = strings.TrimSpace(reviewer)
	if reviewer == "" {
		return source.Page{}, errors.New("store: reviewer is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return source.Page{}, fmt.Errorf("store: begin page review: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(`UPDATE source_pages SET review_status = ?, reviewed_by = ?, reviewed_at = ?, review_note = ?
		WHERE source_id = ? AND page = ?`,
		string(source.Reviewed), reviewer, source.FormatTime(at), note, sourceID, page)
	if err != nil {
		return source.Page{}, fmt.Errorf("store: review page: %w", err)
	}
	if affected, err := res.RowsAffected(); err != nil {
		return source.Page{}, fmt.Errorf("store: review page rows: %w", err)
	} else if affected == 0 {
		return source.Page{}, fmt.Errorf("%w: page %d in source %s", ErrNotFound, page, sourceID)
	}
	detail := fmt.Sprintf(`{"reviewer":%q,"note":%q}`, reviewer, note)
	if err := appendEvent(tx, sourceID, at, EventPageReviewed, page, "", detail); err != nil {
		return source.Page{}, err
	}
	if err := tx.Commit(); err != nil {
		return source.Page{}, fmt.Errorf("store: commit page review: %w", err)
	}
	return s.Page(sourceID, page)
}

// AddQuestion inserts a new unreviewed question and its audit event.
func (s *Store) AddQuestion(q source.Question) error {
	if q.CreatedAt.IsZero() {
		q.CreatedAt = time.Now().UTC()
	}
	if q.UpdatedAt.IsZero() {
		q.UpdatedAt = time.Now().UTC()
	}
	if err := q.Validate(); err != nil {
		return err
	}
	if q.Review != source.Unreviewed || q.AnswerStatus != source.AnswerUnmapped || q.ReviewedBy != "" || !q.ReviewedAt.IsZero() {
		return fmt.Errorf("store: new question %s must start unreviewed with an unmapped answer", q.ID)
	}
	optionsJSON, reasoningJSON, tacticsJSON, glossaryJSON, err := encodeQuestionParts(q)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin question add: %w", err)
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM source_pages WHERE source_id = ? AND page = ?`, q.SourceID, q.Page).Scan(&exists); err != nil {
		return fmt.Errorf("store: check question page: %w", err)
	}
	if exists == 0 {
		return fmt.Errorf("%w: page %d in source %s", ErrNotFound, q.Page, q.SourceID)
	}
	if _, err := tx.Exec(`INSERT INTO source_questions (
			source_id, id, page, stem, options_json, answer, evidence, reasoning, option_reasoning_json,
			answer_status, review_status, reviewed_by, reviewed_at, created_at, updated_at, tactics_json,
			glossary_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		q.SourceID, q.ID, q.Page, q.Stem, optionsJSON, q.Answer, q.Evidence, q.Reasoning, reasoningJSON,
		string(q.AnswerStatus), string(q.Review), q.ReviewedBy, source.FormatTime(q.ReviewedAt),
		source.FormatTime(q.CreatedAt), source.FormatTime(q.UpdatedAt), tacticsJSON, glossaryJSON,
	); err != nil {
		return fmt.Errorf("store: insert question %s: %w", q.ID, err)
	}
	detail, err := questionDetail(q, optionsJSON, reasoningJSON, tacticsJSON, glossaryJSON)
	if err != nil {
		return err
	}
	if err := appendEvent(tx, q.SourceID, q.UpdatedAt, EventQuestionAdded, q.Page, q.ID, detail); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit question add: %w", err)
	}
	return nil
}

// SaveQuestion updates an existing question after an edit or a review. The
// caller is responsible for the state transition (source.Question.Edited or
// source.Question.Review); the audit event records the resulting snapshot.
func (s *Store) SaveQuestion(q source.Question, kind string) error {
	if q.UpdatedAt.IsZero() {
		q.UpdatedAt = time.Now().UTC()
	}
	if kind != EventQuestionEdited && kind != EventQuestionReviewed {
		return fmt.Errorf("store: invalid question event %q", kind)
	}
	if err := q.Validate(); err != nil {
		return err
	}
	optionsJSON, reasoningJSON, tacticsJSON, glossaryJSON, err := encodeQuestionParts(q)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin question save: %w", err)
	}
	defer tx.Rollback()

	var pageExists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM source_pages WHERE source_id = ? AND page = ?`, q.SourceID, q.Page).Scan(&pageExists); err != nil {
		return fmt.Errorf("store: check question page: %w", err)
	}
	if pageExists == 0 {
		return fmt.Errorf("%w: page %d in source %s", ErrNotFound, q.Page, q.SourceID)
	}

	res, err := tx.Exec(`UPDATE source_questions SET
			page = ?, stem = ?, options_json = ?, answer = ?, evidence = ?, reasoning = ?,
			option_reasoning_json = ?, answer_status = ?, review_status = ?, reviewed_by = ?,
			reviewed_at = ?, updated_at = ?, tactics_json = ?, glossary_json = ?
		WHERE source_id = ? AND id = ?`,
		q.Page, q.Stem, optionsJSON, q.Answer, q.Evidence, q.Reasoning, reasoningJSON,
		string(q.AnswerStatus), string(q.Review), q.ReviewedBy, source.FormatTime(q.ReviewedAt),
		source.FormatTime(q.UpdatedAt), tacticsJSON, glossaryJSON, q.SourceID, q.ID,
	)
	if err != nil {
		return fmt.Errorf("store: save question %s: %w", q.ID, err)
	}
	if affected, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("store: save question rows: %w", err)
	} else if affected == 0 {
		return fmt.Errorf("%w: question %q in source %s", ErrNotFound, q.ID, q.SourceID)
	}
	detail, err := questionDetail(q, optionsJSON, reasoningJSON, tacticsJSON, glossaryJSON)
	if err != nil {
		return err
	}
	if err := appendEvent(tx, q.SourceID, q.UpdatedAt, kind, q.Page, q.ID, detail); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit question save: %w", err)
	}
	return nil
}

func encodeQuestionParts(q source.Question) (string, string, string, string, error) {
	optionsJSON, err := json.Marshal(q.Options)
	if err != nil {
		return "", "", "", "", fmt.Errorf("store: encode options: %w", err)
	}
	reasoning := q.OptionReasoning
	if reasoning == nil {
		reasoning = map[string]string{}
	}
	reasoningJSON, err := json.Marshal(reasoning)
	if err != nil {
		return "", "", "", "", fmt.Errorf("store: encode option reasoning: %w", err)
	}
	tactics := q.Tactics
	if tactics == nil {
		tactics = []string{}
	}
	tacticsJSON, err := json.Marshal(tactics)
	if err != nil {
		return "", "", "", "", fmt.Errorf("store: encode tactic tags: %w", err)
	}
	glossary := q.Glossary
	if glossary == nil {
		glossary = []exam.GlossaryEntry{}
	}
	glossaryJSON, err := json.Marshal(glossary)
	if err != nil {
		return "", "", "", "", fmt.Errorf("store: encode glossary: %w", err)
	}
	return string(optionsJSON), string(reasoningJSON), string(tacticsJSON), string(glossaryJSON), nil
}

// questionDetail encodes the complete question snapshot appended to the audit
// trail. Options and option reasoning arrive already encoded so the event and
// the stored row cannot disagree about what was added, reviewed or edited.
func questionDetail(q source.Question, optionsJSON, reasoningJSON, tacticsJSON, glossaryJSON string) (string, error) {
	payload := struct {
		ID           string          `json:"id"`
		Page         int             `json:"page"`
		Stem         string          `json:"stem"`
		Options      json.RawMessage `json:"options"`
		Answer       string          `json:"answer"`
		AnswerStatus string          `json:"answer_status"`
		Review       string          `json:"review_status"`
		ReviewedBy   string          `json:"reviewed_by"`
		ReviewedAt   string          `json:"reviewed_at"`
		Evidence     string          `json:"evidence"`
		Reasoning    string          `json:"reasoning"`
		OptionReason json.RawMessage `json:"option_reasoning"`
		Tactics      json.RawMessage `json:"tactics"`
		Glossary     json.RawMessage `json:"glossary"`
	}{
		ID: q.ID, Page: q.Page, Stem: q.Stem, Options: json.RawMessage(optionsJSON),
		Answer: q.Answer, AnswerStatus: string(q.AnswerStatus),
		Review: string(q.Review), ReviewedBy: q.ReviewedBy, ReviewedAt: source.FormatTime(q.ReviewedAt),
		Evidence: q.Evidence, Reasoning: q.Reasoning, OptionReason: json.RawMessage(reasoningJSON),
		Tactics: json.RawMessage(tacticsJSON), Glossary: json.RawMessage(glossaryJSON),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("store: encode question detail: %w", err)
	}
	return string(data), nil
}

// Events returns the append-only audit trail of a source, oldest first. It is
// used by tests and by operators who need to see how material changed.
func (s *Store) Events(sourceID string) ([]Event, error) {
	rows, err := s.db.Query(`SELECT id, source_id, at, kind, page, question_id, detail
		FROM source_events WHERE source_id = ? ORDER BY id`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("store: list events of %s: %w", sourceID, err)
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var (
			event Event
			at    string
		)
		if err := rows.Scan(&event.ID, &event.SourceID, &at, &event.Kind, &event.Page, &event.QuestionID, &event.Detail); err != nil {
			return nil, fmt.Errorf("store: scan event: %w", err)
		}
		if event.At, err = parseTime(at); err != nil {
			return nil, fmt.Errorf("store: parse event time: %w", err)
		}
		out = append(out, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate events: %w", err)
	}
	return out, nil
}

type Event struct {
	ID         int64
	SourceID   string
	At         time.Time
	Kind       string
	Page       int
	QuestionID string
	Detail     string
}

func appendEvent(tx *sql.Tx, sourceID string, at time.Time, kind string, page int, questionID, detail string) error {
	if _, err := tx.Exec(`INSERT INTO source_events (source_id, at, kind, page, question_id, detail)
		VALUES (?, ?, ?, ?, ?, ?)`, sourceID, source.FormatTime(at), kind, page, questionID, detail); err != nil {
		return fmt.Errorf("store: append source event %s: %w", kind, err)
	}
	return nil
}

func parseTime(raw string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, raw)
}

func parseOptionalTime(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, nil
	}
	return parseTime(raw)
}
