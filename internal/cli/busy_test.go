package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/store"
)

// TestBusyDatabaseRefusal runs a real command while another connection holds
// the write lock past the CLI's own 5 s wait: the command must end with the
// database_busy refusal naming the window, not the driver's text as an
// internal error.
func TestBusyDatabaseRefusal(t *testing.T) {
	db := newDB(t)
	seed, err := store.Open(db)
	if err != nil {
		t.Fatalf("seed schema: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close seed: %v", err)
	}

	// A plain file-name DSN: the holder needs no pragmas, only the lock.
	holder, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatalf("open holder: %v", err)
	}
	holder.SetMaxOpenConns(1)
	t.Cleanup(func() { holder.Close() })
	if _, err := holder.Exec("BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("hold write lock: %v", err)
	}

	var stdout, stderr bytes.Buffer
	clock := &stepClock{base: goldenBase}
	start := time.Now()
	code := Run([]string{"practice", "--db", db, "--answer", "B", "--json"}, Options{
		Stdin:      strings.NewReader(""),
		Stdout:     &stdout,
		Stderr:     &stderr,
		Now:        clock.Now,
		NewSession: func() string { return "sess-test" },
		Getenv:     func(string) string { return "" },
	})
	waited := time.Since(start)
	t.Logf("busy refusal after %v (the write wait Open configures is 5000 ms)", waited)

	if _, err := holder.Exec("COMMIT"); err != nil {
		t.Fatalf("release write lock: %v", err)
	}
	if code != 2 {
		t.Fatalf("practice exit = %d, want 2\nstderr: %s", code, stderr.String())
	}
	var doc errorDoc
	if err := json.Unmarshal(stderr.Bytes(), &doc); err != nil {
		t.Fatalf("decode error doc: %v\n%s", err, stderr.String())
	}
	if doc.Error.Code != "database_busy" {
		t.Fatalf("error code = %q, message %q", doc.Error.Code, doc.Error.Message)
	}
	for _, want := range []string{"window", "close", "retry"} {
		if !strings.Contains(doc.Error.Message, want) {
			t.Fatalf("message %q lacks %q", doc.Error.Message, want)
		}
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want nothing", stdout.String())
	}
	if waited < time.Second {
		t.Fatalf("refusal after %v, before a wait happened", waited)
	}
}
