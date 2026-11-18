package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"maps"
	"net/url"
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

const replaceAttemptSQL = `INSERT OR REPLACE INTO attempts (
		id, session_id, question_id, content_id, content_version, status,
		selected_label, selected_text, correct_label, correct_text,
		started_at, answered_at, elapsed_ms
	) VALUES (1, 'sess-evil', 'q1', 'sample-0001', '1', 'incorrect',
		'A', 'At 7 p.m.', 'B', 'At 4 p.m.',
		'2026-11-10T09:00:00Z', '2026-11-10T09:00:02Z', 2000)`

func mustOpen(t *testing.T, path string) *Store {
	t.Helper()
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// openSQLite opens a fixture through the same DSN construction as Open, so
// test setup cannot drift to a second path scheme.
func openSQLite(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", sqliteURI(path, runtime.GOOS))
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	// Match Open: one connection, so fixture setup cannot contend with itself
	// for the file lock while other tests run in parallel.
	db.SetMaxOpenConns(1)
	return db
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
	t.Parallel()
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

func TestOpenPrivateTightensExistingDirectory(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		return
	}
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := OpenPrivate(filepath.Join(dir, "attempts.sqlite"))
	if err != nil {
		t.Fatalf("OpenPrivate: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat directory: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("directory mode = %04o, want 0700", perm)
	}
}

func TestOpenRejectsNewerSchema(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "future.sqlite")
	db := openSQLite(t, path)
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

// sideFileDigests hashes the database and every side file SQLite may keep
// beside it (WAL, shared memory, rollback journal), by base name. A write to
// any of them, or a side file that appears, changes the result.
func sideFileDigests(t *testing.T, path string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, name := range []string{path, path + "-wal", path + "-shm", path + "-journal"} {
		raw, err := os.ReadFile(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		sum := sha256.Sum256(raw)
		out[filepath.Base(name)] = hex.EncodeToString(sum[:])
	}
	return out
}

// TestOpenRejectsNewerSchemaLeavesFileUntouched pins the refusal of a database
// a newer build wrote: Open fails with the typed error before it writes
// anything, so the build that owns the file keeps working. The version check
// runs before any migration, pragma or WAL creation, and the side files count:
// a refused open must neither appear nor change one.
//
// The fixture is what a downgrade meets: a newer build that quit, which
// SQLite leaves with no side files. A database whose writer is still running
// (or crashed mid-transaction) makes SQLite itself touch -shm bookkeeping or
// recover the WAL; that is SQLite's own, data-preserving work, and no reader
// can avoid it.
func TestOpenRejectsNewerSchemaLeavesFileUntouched(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "future.sqlite")
	db := openSQLite(t, path)
	if _, err := db.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatalf("set version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	before := sideFileDigests(t, path)
	if _, err := Open(path); err == nil {
		t.Fatal("Open accepted a newer schema")
	} else {
		var newer *SchemaNewerError
		if !errors.As(err, &newer) {
			t.Fatalf("Open error = %v (%T), want *SchemaNewerError", err, err)
		}
		if newer.Version != 99 || newer.Supported != SchemaVersion {
			t.Fatalf("SchemaNewerError = %+v, want version 99 supported %d", newer, SchemaVersion)
		}
	}
	after := sideFileDigests(t, path)
	if !maps.Equal(before, after) {
		t.Fatalf("a refused open changed the database files:\nbefore: %v\nafter:  %v", before, after)
	}
	if _, ok := after[filepath.Base(path)]; !ok {
		t.Fatal("the database file vanished")
	}
}

func TestOpenRejectsEmptyPath(t *testing.T) {
	t.Parallel()
	if _, err := Open("  "); err == nil || !strings.Contains(err.Error(), "empty database path") {
		t.Fatalf("Open error = %v", err)
	}
}

// TestSQLiteURIFormatsPathsForEveryOS pins the DSN for both path families on
// any host. A Windows drive path must not land in the URI authority, which is
// what produced "invalid uri authority: C:..." on windows-latest.
func TestSQLiteURIFormatsPathsForEveryOS(t *testing.T) {
	t.Parallel()
	const options = "?_pragma=busy_timeout%285000%29&_txlock=immediate"
	tests := []struct {
		name     string
		path     string
		goos     string
		want     string
		wantPath string
	}{
		{
			name: "posix", path: "/home/u/db.sqlite", goos: "linux", // portable-paths: allow the goos field selects the branch; this literal is the input under test
			want: "file:///home/u/db.sqlite" + options, wantPath: "/home/u/db.sqlite", // portable-paths: allow the goos field selects the branch; this literal is the input under test
		},
		{
			name: "posix darwin", path: "/Users/u/db.sqlite", goos: "darwin", // portable-paths: allow the goos field selects the branch; this literal is the input under test
			want: "file:///Users/u/db.sqlite" + options, wantPath: "/Users/u/db.sqlite", // portable-paths: allow the goos field selects the branch; this literal is the input under test
		},
		{
			name: "windows drive", path: `C:\Users\runner\db.sqlite`, goos: "windows", // portable-paths: allow the goos field selects the branch; this literal is the input under test
			want: "file:///C:/Users/runner/db.sqlite" + options, wantPath: "/C:/Users/runner/db.sqlite",
		},
		{
			name: "windows drive with forward slashes", path: "C:/Users/runner/db.sqlite", goos: "windows", // portable-paths: allow the goos field selects the branch; this literal is the input under test
			want: "file:///C:/Users/runner/db.sqlite" + options, wantPath: "/C:/Users/runner/db.sqlite",
		},
		{
			name:     "windows import smoke temp path",
			path:     `C:\Users\RUNNER~1\AppData\Local\Temp\passquest-import-smoke-bEarG2\smoke.sqlite`, // portable-paths: allow the goos field selects the branch; this literal is the input under test
			goos:     "windows",
			want:     "file:///C:/Users/RUNNER~1/AppData/Local/Temp/passquest-import-smoke-bEarG2/smoke.sqlite" + options,
			wantPath: "/C:/Users/RUNNER~1/AppData/Local/Temp/passquest-import-smoke-bEarG2/smoke.sqlite",
		},
		{
			// SQLite rejects every authority but "" and "localhost", so a UNC
			// path must keep its leading "\\" in the path component: the DSN
			// needs four slashes, not two.
			name: "windows unc", path: `\\server\share\db.sqlite`, goos: "windows", // portable-paths: allow the goos field selects the branch; this literal is the input under test
			want: "file:////server/share/db.sqlite" + options, wantPath: "//server/share/db.sqlite",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sqliteURI(tt.path, tt.goos)
			if got != tt.want {
				t.Fatalf("sqliteURI(%q, %q) = %q, want %q", tt.path, tt.goos, got, tt.want)
			}
			if strings.Contains(got, "%5C") || strings.Contains(got, `\`) {
				t.Fatalf("URI keeps a backslash: %q", got)
			}
			if strings.Contains(got, "file:C:") {
				t.Fatalf("drive path is in the URI authority: %q", got)
			}
			u, err := url.Parse(got)
			if err != nil {
				t.Fatalf("parse %q: %v", got, err)
			}
			if u.Scheme != "file" || u.Host != "" || u.User != nil {
				t.Fatalf("scheme/authority = %q/%q/%v, want file and no authority", u.Scheme, u.Host, u.User)
			}
			if u.Path != tt.wantPath {
				t.Fatalf("path component = %q, want %q", u.Path, tt.wantPath)
			}
			q := u.Query()
			if q.Get("_pragma") != "busy_timeout(5000)" {
				t.Fatalf("_pragma = %q, want busy_timeout(5000)", q.Get("_pragma"))
			}
			if q.Get("_txlock") != "immediate" {
				t.Fatalf("_txlock = %q, want immediate", q.Get("_txlock"))
			}
		})
	}
}
