package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
)

func sampleAttempt(questionID string) attempt.Attempt {
	started := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	return attempt.Attempt{
		SessionID:      "sess-test",
		QuestionID:     questionID,
		ContentID:      "sample-0001",
		ContentVersion: "1",
		Status:         attempt.StatusCorrect,
		SelectedLabel:  "B",
		SelectedText:   "At 4 p.m.",
		CorrectLabel:   "B",
		CorrectText:    "At 4 p.m.",
		StartedAt:      started,
		AnsweredAt:     started.Add(1200 * time.Millisecond),
		ElapsedMS:      1200,
	}
}

func mustOpen(t *testing.T, path string) *Store {
	t.Helper()
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func mustAppend(t *testing.T, st *Store, rec attempt.Attempt) attempt.Attempt {
	t.Helper()
	got, err := st.Append(rec)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	return got
}

func TestOpenCreatesSchemaAndProtectsFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	path := filepath.Join(dir, "attempts.sqlite")
	st := mustOpen(t, path)

	if got, want := st.Path(), path; got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
	var version int
	if err := st.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != SchemaVersion {
		t.Fatalf("user_version = %d, want %d", version, SchemaVersion)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat database: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("database mode = %04o, want 0600", perm)
		}
		dirInfo, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat directory: %v", err)
		}
		if perm := dirInfo.Mode().Perm(); perm != 0o700 {
			t.Errorf("directory mode = %04o, want 0700", perm)
		}
	}
}

func TestAppendListAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attempts.sqlite")
	st := mustOpen(t, path)

	first := mustAppend(t, st, sampleAttempt("q1"))
	if first.ID != 1 || first.PriorAttempts != 0 || first.AttemptNumber != 1 {
		t.Fatalf("first append = id %d prior %d number %d", first.ID, first.PriorAttempts, first.AttemptNumber)
	}

	second := sampleAttempt("q1")
	second.Status = attempt.StatusIncorrect
	second.SelectedLabel = "A"
	second.SelectedText = "At 7 p.m."
	second = mustAppend(t, st, second)
	if second.ID != 2 || second.PriorAttempts != 1 || second.AttemptNumber != 2 {
		t.Fatalf("second append = id %d prior %d number %d", second.ID, second.PriorAttempts, second.AttemptNumber)
	}

	skipped := sampleAttempt("q2")
	skipped.Status = attempt.StatusSkipped
	skipped.SelectedLabel = ""
	skipped.SelectedText = ""
	skipped = mustAppend(t, st, skipped)
	if skipped.AttemptNumber != 1 {
		t.Fatalf("q2 attempt number = %d, want 1", skipped.AttemptNumber)
	}

	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	list, err := reopened.List()
	if err != nil {
		t.Fatalf("List after reopen: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("attempts after reopen = %d, want 3", len(list))
	}
	got := list[1]
	if got.Status != attempt.StatusIncorrect || got.SelectedLabel != "A" || got.SelectedText != "At 7 p.m." {
		t.Fatalf("round-trip attempt = %+v", got)
	}
	if got.AttemptNumber != 2 || got.PriorAttempts != 1 {
		t.Fatalf("round-trip numbering = %d/%d", got.AttemptNumber, got.PriorAttempts)
	}
	if !got.StartedAt.Equal(sampleAttempt("q1").StartedAt) {
		t.Fatalf("started_at = %s", got.StartedAt)
	}
	if list[2].SelectedLabel != "" || list[2].SelectedText != "" {
		t.Fatalf("skipped round-trip has selection %q/%q", list[2].SelectedLabel, list[2].SelectedText)
	}

	third := mustAppend(t, reopened, sampleAttempt("q1"))
	if third.AttemptNumber != 3 || third.PriorAttempts != 2 {
		t.Fatalf("append after reopen = %d/%d", third.AttemptNumber, third.PriorAttempts)
	}
}

func TestNumberingSpansContentVersions(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "attempts.sqlite"))
	first := mustAppend(t, st, sampleAttempt("q1"))
	if first.AttemptNumber != 1 || first.PriorAttempts != 0 {
		t.Fatalf("first version numbering = %d/%d", first.AttemptNumber, first.PriorAttempts)
	}

	nextVersion := sampleAttempt("q1")
	nextVersion.ContentVersion = "2"
	second := mustAppend(t, st, nextVersion)
	if second.AttemptNumber != 2 || second.PriorAttempts != 1 {
		t.Fatalf("second version numbering = %d/%d", second.AttemptNumber, second.PriorAttempts)
	}

	list, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("attempts = %d, want 2", len(list))
	}
	if list[1].ContentVersion != "2" || list[1].AttemptNumber != 2 || list[1].PriorAttempts != 1 {
		t.Fatalf("round-trip second version = %+v", list[1])
	}
}

func TestAttemptsAreAppendOnly(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "attempts.sqlite"))
	mustAppend(t, st, sampleAttempt("q1"))

	if _, err := st.db.Exec(`UPDATE attempts SET status = 'incorrect'`); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("UPDATE error = %v, want append-only rejection", err)
	}
	if _, err := st.db.Exec(`DELETE FROM attempts`); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("DELETE error = %v, want append-only rejection", err)
	}
	list, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("attempts after rejected writes = %d, want 1", len(list))
	}
}

func TestAppendRejectsInconsistentRecords(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "attempts.sqlite"))

	tests := []struct {
		name   string
		mutate func(*attempt.Attempt)
		want   string
	}{
		{"empty session", func(a *attempt.Attempt) { a.SessionID = "" }, "session_id"},
		{"invalid status", func(a *attempt.Attempt) { a.Status = "maybe" }, "status"},
		{"correct with wrong label", func(a *attempt.Attempt) { a.SelectedLabel = "C" }, "but key is"},
		{"incorrect with key label", func(a *attempt.Attempt) {
			a.Status = attempt.StatusIncorrect
		}, "selected the key"},
		{"skip with selection", func(a *attempt.Attempt) { a.Status = attempt.StatusSkipped }, "has a selection"},
		{"negative elapsed", func(a *attempt.Attempt) { a.ElapsedMS = -1 }, "negative"},
		{"zero timestamp", func(a *attempt.Attempt) { a.StartedAt = time.Time{} }, "timestamps"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := sampleAttempt("q1")
			tt.mutate(&rec)
			if _, err := st.Append(rec); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Append error = %v, want %q", err, tt.want)
			}
		})
	}
	list, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("invalid appends stored %d records", len(list))
	}
}

func TestMigrationKeepsOlderDatabaseAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE legacy_notes (body TEXT)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO legacy_notes (body) VALUES ('keep me')`); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 0`); err != nil {
		t.Fatalf("set version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy: %v", err)
	}

	st := mustOpen(t, path)
	var body string
	if err := st.db.QueryRow(`SELECT body FROM legacy_notes`).Scan(&body); err != nil {
		t.Fatalf("legacy table after migration: %v", err)
	}
	if body != "keep me" {
		t.Fatalf("legacy row = %q", body)
	}
	mustAppend(t, st, sampleAttempt("q1"))
	if err := st.Close(); err != nil {
		t.Fatalf("close migrated: %v", err)
	}

	again, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer again.Close()
	var version int
	if err := again.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	if version != SchemaVersion {
		t.Fatalf("user_version = %d, want %d", version, SchemaVersion)
	}
	list, err := again.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("attempts after idempotent migration = %d, want 1", len(list))
	}
}

func TestOpenRejectsNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatalf("set version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := Open(path); err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("Open error = %v, want newer-schema rejection", err)
	}
}

func TestOpenRejectsEmptyPath(t *testing.T) {
	if _, err := Open("  "); err == nil || !strings.Contains(err.Error(), "empty database path") {
		t.Fatalf("Open error = %v", err)
	}
}
