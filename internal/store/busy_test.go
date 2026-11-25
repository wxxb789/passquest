package store

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// busyTestTimeoutMS is the shortened write wait the two timeout cases open
// their writer with; Open's own wait is 5000 ms and would make the cases slow.
const busyTestTimeoutMS = 300

// openShortBusy builds the handle Open builds, with only the busy_timeout
// pragma changed. The DSN keeps sqliteURI's path spelling, so the Windows
// drive-path rule is the product's, not a second scheme.
func openShortBusy(t *testing.T, path string, ms int) *Store {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve %s: %v", path, err)
	}
	u, err := url.Parse(sqliteURI(abs, runtime.GOOS))
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	q := u.Query()
	pragmas := q["_pragma"]
	replaced := false
	for i, pragma := range pragmas {
		if strings.HasPrefix(pragma, "busy_timeout") {
			pragmas[i] = fmt.Sprintf("busy_timeout(%d)", ms)
			replaced = true
		}
	}
	if !replaced {
		t.Fatalf("sqliteURI carries no busy_timeout pragma: %q", sqliteURI(abs, runtime.GOOS))
	}
	q["_pragma"] = pragmas
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("ping %s: %v", path, err)
	}
	t.Cleanup(func() { db.Close() })
	return &Store{db: db, path: abs}
}

// holdWriteLock takes the handle's write lock on its single connection and
// returns the release. It is the other-writer half of every case here: the
// window, a second command or a crashed transaction all present as this lock.
func holdWriteLock(t *testing.T, st *Store) func() {
	t.Helper()
	if _, err := st.db.Exec("BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("hold write lock: %v", err)
	}
	return func() {
		t.Helper()
		if _, err := st.db.Exec("COMMIT"); err != nil {
			t.Fatalf("release write lock: %v", err)
		}
	}
}

// TestIsBusyDistinguishesTypedCodeFromLookalikeText feeds IsBusy a real busy
// error and a message-only lookalike: only the driver's result code counts, so
// a mapping that matched the text alone would fail the lookalike half while
// the text itself is not a contract.
func TestIsBusyDistinguishesTypedCodeFromLookalikeText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attempts.sqlite")
	holder := mustOpen(t, path)
	release := holdWriteLock(t, holder)

	writer := openShortBusy(t, path, busyTestTimeoutMS)
	_, err := writer.Append(sampleAttempt("q1"))
	release()
	if err == nil {
		t.Fatal("a write under a held write lock succeeded")
	}
	if !IsBusy(err) {
		t.Fatalf("IsBusy(%v) = false, want true for the driver's busy error", err)
	}
	if !IsBusy(fmt.Errorf("store: begin append: %w", err)) {
		t.Fatal("IsBusy lost the busy code through a wrap")
	}
	if IsBusy(errors.New("database is locked (5) (SQLITE_BUSY)")) {
		t.Fatal("IsBusy accepted a message without the driver's typed code")
	}
	if IsBusy(nil) {
		t.Fatal("IsBusy(nil) = true")
	}
}

// TestInterleavedWritersBothPersist runs two handles as two processes would:
// the writes alternate, each committed before the next, and both handles read
// the same rows back in insertion order.
func TestInterleavedWritersBothPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attempts.sqlite")
	first := mustOpen(t, path)
	second := mustOpen(t, path)

	firstID := mustAppend(t, first, sampleAttempt("q1")).ID
	secondID := mustAppend(t, second, sampleAttempt("q2")).ID
	repeatID := mustAppend(t, first, sampleAttempt("q1")).ID
	want := []int64{firstID, secondID, repeatID}

	read := func(st *Store) []int64 {
		rows, err := st.List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		ids := make([]int64, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		return ids
	}
	firstRead, secondRead := read(first), read(second)
	t.Logf("ids read: first handle %v, second handle %v (writes interleaved as %v)", firstRead, secondRead, want)
	if !slices.Equal(firstRead, want) || !slices.Equal(secondRead, want) {
		t.Fatalf("ids read = %v / %v, want the interleaved insertion order %v", firstRead, secondRead, want)
	}
}

// TestWriteWaitsForAHolderUnderTheTimeout holds the write lock for less than
// the writer's 5 s wait: the write waits, then succeeds once the holder
// commits. The asserted lower bound on the wait is lock order (the writer
// cannot commit before the release), not a wall-clock band.
func TestWriteWaitsForAHolderUnderTheTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attempts.sqlite")
	holder := mustOpen(t, path)
	writer := mustOpen(t, path)

	const hold = 300 * time.Millisecond
	if _, err := holder.db.Exec("BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("hold write lock: %v", err)
	}
	released := make(chan error, 1)
	go func() {
		time.Sleep(hold)
		_, err := holder.db.Exec("COMMIT")
		released <- err
	}()

	start := time.Now()
	stored, err := writer.Append(sampleAttempt("q1"))
	waited := time.Since(start)
	t.Logf("write waited %v for a lock held %v", waited, hold)
	if err != nil {
		t.Fatalf("Append under a lock released at %v: %v", hold, err)
	}
	if err := <-released; err != nil {
		t.Fatalf("release write lock: %v", err)
	}
	if waited < hold {
		t.Fatalf("write committed after %v, before the lock was released at %v", waited, hold)
	}
	if stored.ID == 0 {
		t.Fatal("Append filled in no row id")
	}
	rows, err := writer.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows after the waited write = %d, want 1", len(rows))
	}
}

// TestWritePastTimeoutRefusesAndLeavesNoRow holds the lock past the writer's
// shortened wait (busyTestTimeoutMS; Open's own is 5000 ms): the write fails
// with the driver's typed busy error and its transaction leaves no row behind,
// neither before nor after the holder commits.
func TestWritePastTimeoutRefusesAndLeavesNoRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attempts.sqlite")
	holder := mustOpen(t, path)
	writer := openShortBusy(t, path, busyTestTimeoutMS)
	release := holdWriteLock(t, holder)

	start := time.Now()
	_, err := writer.Append(sampleAttempt("q1"))
	waited := time.Since(start)
	t.Logf("busy refusal after %v with busy_timeout(%d)", waited, busyTestTimeoutMS)
	if err == nil {
		t.Fatal("a write past the timeout succeeded")
	}
	if !IsBusy(err) {
		t.Fatalf("Append error = %v (%T), want the driver's typed busy error", err, err)
	}
	if waited < busyTestTimeoutMS*time.Millisecond {
		t.Fatalf("refusal after %v, before the %d ms wait ran out", waited, busyTestTimeoutMS)
	}
	release()

	rows, err := writer.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("a refused write left %d rows: %+v", len(rows), rows)
	}
	reopened := mustOpen(t, path)
	rows, err = reopened.List()
	if err != nil {
		t.Fatalf("List from a second handle: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("a refused write left %d rows visible to a second handle: %+v", len(rows), rows)
	}
}
