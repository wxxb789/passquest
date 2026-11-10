// Package store persists attempts in SQLite. The table is append-only: storage
// rejects updates, deletes and id-reusing inserts (including INSERT OR REPLACE)
// so historical evidence cannot be rewritten by a later feature.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"

	_ "modernc.org/sqlite"
)

// SchemaVersion is the persisted schema this build understands.
const SchemaVersion = 2

// schemaV1 is the original version-1 shape. Migrations apply each version in
// order, so a version-0 database gets both steps and a version-1 database gets
// only the insert guard and the composite index.
var schemaV1 = []string{
	`CREATE TABLE IF NOT EXISTS attempts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		question_id TEXT NOT NULL,
		content_id TEXT NOT NULL,
		content_version TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('correct', 'incorrect', 'skipped')),
		selected_label TEXT,
		selected_text TEXT,
		correct_label TEXT NOT NULL,
		correct_text TEXT NOT NULL,
		started_at TEXT NOT NULL,
		answered_at TEXT NOT NULL,
		elapsed_ms INTEGER NOT NULL CHECK (elapsed_ms >= 0),
		CHECK (
			(status = 'skipped' AND selected_label IS NULL AND selected_text IS NULL)
			OR (status IN ('correct', 'incorrect') AND selected_label IS NOT NULL AND selected_text IS NOT NULL)
		),
		CHECK (
			(status = 'correct' AND selected_label = correct_label)
			OR (status = 'incorrect' AND selected_label <> correct_label)
			OR status = 'skipped'
		)
	)`,
	`CREATE INDEX IF NOT EXISTS attempts_question_id ON attempts(question_id, id)`,
	`CREATE TRIGGER IF NOT EXISTS attempts_append_only_update BEFORE UPDATE ON attempts
		BEGIN SELECT RAISE(ABORT, 'attempts are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS attempts_append_only_delete BEFORE DELETE ON attempts
		BEGIN SELECT RAISE(ABORT, 'attempts are append-only'); END`,
}

var schemaV2 = []string{
	`DROP INDEX IF EXISTS attempts_question_id`,
	`CREATE INDEX IF NOT EXISTS attempts_content_question_id ON attempts(content_id, question_id, id)`,
	`CREATE TRIGGER IF NOT EXISTS attempts_append_only_insert BEFORE INSERT ON attempts
		WHEN EXISTS (SELECT 1 FROM attempts WHERE id = NEW.id)
		BEGIN SELECT RAISE(ABORT, 'attempts are append-only'); END`,
}

var migrations = []struct {
	version    int
	statements []string
}{
	{1, schemaV1},
	{2, schemaV2},
}

// Store is a handle to one attempts database.
type Store struct {
	db   *sql.DB
	path string
}

// Open creates the database (and parent directory) when absent, migrates it to
// SchemaVersion, and returns a single-writer handle.
func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("store: empty database path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("store: resolve path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return nil, fmt.Errorf("store: create directory: %w", err)
	}

	u := &url.URL{Scheme: "file", Path: abs}
	q := u.Query()
	q.Add("_pragma", "busy_timeout(5000)")
	// Serialize write transactions before counting prior attempts; the attempt
	// number is assigned inside that lock.
	q.Add("_txlock", "immediate")
	u.RawQuery = q.Encode()

	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	// One CLI process is the writer. A single connection also keeps SQLite's
	// locking behavior simple and deterministic.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := os.Chmod(abs, 0o600); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: protect database file: %w", err)
	}
	return &Store{db: db, path: abs}, nil
}

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("store: read schema version: %w", err)
	}
	switch {
	case version > SchemaVersion:
		return fmt.Errorf("store: database schema version %d is newer than supported version %d", version, SchemaVersion)
	case version == SchemaVersion:
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin migration: %w", err)
	}
	defer tx.Rollback()
	for _, migration := range migrations {
		if version >= migration.version {
			continue
		}
		for _, stmt := range migration.statements {
			if _, err := tx.Exec(stmt); err != nil {
				return fmt.Errorf("store: migrate to version %d: %w", migration.version, err)
			}
		}
		version = migration.version
	}
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, SchemaVersion)); err != nil {
		return fmt.Errorf("store: set schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit migration: %w", err)
	}
	return nil
}

// Path returns the absolute database path.
func (s *Store) Path() string { return s.path }

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// Append stores one graded attempt and returns it with its row id, prior count
// and content-scoped question attempt number filled in.
func (s *Store) Append(a attempt.Attempt) (attempt.Attempt, error) {
	if err := validateForInsert(a); err != nil {
		return attempt.Attempt{}, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return attempt.Attempt{}, fmt.Errorf("store: begin append: %w", err)
	}
	defer tx.Rollback()

	var prior int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM attempts WHERE content_id = ? AND question_id = ?`,
		a.ContentID, a.QuestionID).Scan(&prior); err != nil {
		return attempt.Attempt{}, fmt.Errorf("store: count prior attempts: %w", err)
	}

	res, err := tx.Exec(`INSERT INTO attempts (
			session_id, question_id, content_id, content_version, status,
			selected_label, selected_text, correct_label, correct_text,
			started_at, answered_at, elapsed_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.SessionID, a.QuestionID, a.ContentID, a.ContentVersion, string(a.Status),
		nullIfEmpty(a.SelectedLabel), nullIfEmpty(a.SelectedText), a.CorrectLabel, a.CorrectText,
		a.StartedAt.UTC().Format(time.RFC3339Nano), a.AnsweredAt.UTC().Format(time.RFC3339Nano), a.ElapsedMS,
	)
	if err != nil {
		return attempt.Attempt{}, fmt.Errorf("store: insert attempt: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return attempt.Attempt{}, fmt.Errorf("store: read attempt id: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return attempt.Attempt{}, fmt.Errorf("store: commit attempt: %w", err)
	}

	a.ID = id
	a.PriorAttempts = prior
	a.AttemptNumber = prior + 1
	return a, nil
}

// List returns all attempts in insertion order with content-scoped question
// attempt numbers recomputed from that order.
func (s *Store) List() ([]attempt.Attempt, error) {
	rows, err := s.db.Query(`SELECT
			id, session_id, question_id, content_id, content_version, status,
			selected_label, selected_text, correct_label, correct_text,
			started_at, answered_at, elapsed_ms
		FROM attempts ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: list attempts: %w", err)
	}
	defer rows.Close()

	var out []attempt.Attempt
	seen := make(map[attempt.QuestionKey]int)
	for rows.Next() {
		var (
			a           attempt.Attempt
			status      string
			selectedLbl sql.NullString
			selectedTxt sql.NullString
			started     string
			answered    string
		)
		if err := rows.Scan(
			&a.ID, &a.SessionID, &a.QuestionID, &a.ContentID, &a.ContentVersion, &status,
			&selectedLbl, &selectedTxt, &a.CorrectLabel, &a.CorrectText,
			&started, &answered, &a.ElapsedMS,
		); err != nil {
			return nil, fmt.Errorf("store: scan attempt: %w", err)
		}
		a.Status = attempt.Status(status)
		if selectedLbl.Valid {
			a.SelectedLabel = selectedLbl.String
		}
		if selectedTxt.Valid {
			a.SelectedText = selectedTxt.String
		}
		if a.StartedAt, err = time.Parse(time.RFC3339Nano, started); err != nil {
			return nil, fmt.Errorf("store: parse started_at of attempt %d: %w", a.ID, err)
		}
		if a.AnsweredAt, err = time.Parse(time.RFC3339Nano, answered); err != nil {
			return nil, fmt.Errorf("store: parse answered_at of attempt %d: %w", a.ID, err)
		}
		key := a.Key()
		seen[key]++
		a.AttemptNumber = seen[key]
		a.PriorAttempts = a.AttemptNumber - 1
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate attempts: %w", err)
	}
	return out, nil
}

func validateForInsert(a attempt.Attempt) error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	if strings.TrimSpace(a.SessionID) == "" {
		add("session_id is empty")
	}
	if strings.TrimSpace(a.QuestionID) == "" {
		add("question_id is empty")
	}
	if strings.TrimSpace(a.ContentID) == "" {
		add("content_id is empty")
	}
	if strings.TrimSpace(a.ContentVersion) == "" {
		add("content_version is empty")
	}
	if !a.Status.Valid() {
		add("status %q is invalid", a.Status)
	}
	if strings.TrimSpace(a.CorrectLabel) == "" || strings.TrimSpace(a.CorrectText) == "" {
		add("correct option is empty")
	}
	if a.StartedAt.IsZero() || a.AnsweredAt.IsZero() {
		add("timestamps must be set")
	} else if a.AnsweredAt.Before(a.StartedAt) {
		add("answered_at is before started_at")
	}
	if a.ElapsedMS < 0 {
		add("elapsed_ms is negative")
	}

	switch a.Status {
	case attempt.StatusSkipped:
		if a.SelectedLabel != "" || a.SelectedText != "" {
			add("skipped attempt has a selection")
		}
	case attempt.StatusCorrect:
		if a.SelectedLabel == "" || a.SelectedText == "" {
			add("correct attempt has no selection")
		} else if a.SelectedLabel != a.CorrectLabel {
			add("correct attempt selected %q but key is %q", a.SelectedLabel, a.CorrectLabel)
		}
	case attempt.StatusIncorrect:
		if a.SelectedLabel == "" || a.SelectedText == "" {
			add("incorrect attempt has no selection")
		} else if a.SelectedLabel == a.CorrectLabel {
			add("incorrect attempt selected the key %q", a.SelectedLabel)
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("store: invalid attempt: %s", strings.Join(problems, "; "))
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
