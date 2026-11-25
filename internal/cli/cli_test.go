package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/study"
)

var updateGolden = flag.Bool("update-golden", false, "rewrite golden files")

// TestMain doubles as a child-process entry point: the parent test runs the
// real test binary with PASSQUEST_TEST_RUN_MAIN=1, which exercises process
// restart against a real SQLite file instead of influencing an in-process handle.
func TestMain(m *testing.M) {
	if os.Getenv("PASSQUEST_TEST_RUN_MAIN") == "1" {
		os.Exit(Run(os.Args[1:], Options{}))
	}
	code := m.Run()
	removeAssessmentSeeds()
	os.Exit(code)
}

var goldenBase = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

type stepClock struct {
	base  time.Time
	calls int
}

func (c *stepClock) Now() time.Time {
	c.calls++
	return c.base.Add(time.Duration(c.calls) * time.Second)
}

func newDB(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "attempts.sqlite")
}

func runCLI(t *testing.T, db string, clock *stepClock, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	opt := Options{
		Stdin:      strings.NewReader(stdin),
		Stdout:     &stdout,
		Stderr:     &stderr,
		Now:        clock.Now,
		NewSession: func() string { return "sess-test" },
		Getenv:     func(string) string { return "" },
	}
	code := Run(append(args, "--db", db), opt)
	return code, stdout.String(), stderr.String()
}

func decodeAnswered(t *testing.T, raw string) answeredDoc {
	t.Helper()
	var doc answeredDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode answered doc: %v\n%s", err, raw)
	}
	return doc
}

func decodeReport(t *testing.T, raw string) reportDoc {
	t.Helper()
	var doc reportDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode report doc: %v\n%s", err, raw)
	}
	return doc
}

func mustRun(t *testing.T, db string, clock *stepClock, stdin string, args ...string) string {
	t.Helper()
	code, out, errOut := runCLI(t, db, clock, stdin, args...)
	if code != 0 {
		t.Fatalf("Run(%v) = %d\nstdout:\n%s\nstderr:\n%s", args, code, out, errOut)
	}
	return out
}

// seedFourAttempts runs the shared q1 correct, q2 incorrect, q3 skipped,
// q1 repeated scenario. Callers keep their own oracles over the returned docs.
func seedFourAttempts(t *testing.T, db string, clock *stepClock) []answeredDoc {
	t.Helper()
	steps := [][]string{
		{"practice", "--json", "--question", "q1", "--answer", "B"},
		{"practice", "--json", "--question", "q2", "--answer", "A"},
		{"practice", "--json", "--question", "q3", "--skip"},
		{"practice", "--json", "--question", "q1", "--answer", "B"},
	}
	docs := make([]answeredDoc, 0, len(steps))
	for _, args := range steps {
		docs = append(docs, decodeAnswered(t, mustRun(t, db, clock, "", args...)))
	}
	return docs
}

func collectKeys(v any, out map[string]bool) {
	switch value := v.(type) {
	case map[string]any:
		for key, child := range value {
			out[key] = true
			collectKeys(child, out)
		}
	case []any:
		for _, child := range value {
			collectKeys(child, out)
		}
	}
}

func TestDatabasePath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tests := []struct {
		name        string
		flag        string
		env         map[string]string
		want        string
		wantPrivate bool
	}{
		{"flag wins", "/tmp/flag.sqlite", map[string]string{"PASSQUEST_DB": "/tmp/env.sqlite"}, "/tmp/flag.sqlite", false}, // portable-paths: allow compared as strings; the resolution never touches it
		{"env when flag blank", "  ", map[string]string{"PASSQUEST_DB": "/tmp/env.sqlite"}, "/tmp/env.sqlite", false},      // portable-paths: allow compared as strings; the resolution never touches it
		{"per-user default when both blank", "", map[string]string{"PASSQUEST_DB": " ", "PASSQUEST_DESKTOP_DATA": " "}, filepath.Join(root, "Pass Quest", "passquest.sqlite"), true},
		{"data directory override", "", map[string]string{"PASSQUEST_DESKTOP_DATA": filepath.Join(root, "elsewhere")}, filepath.Join(root, "elsewhere", "passquest.sqlite"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			opt := Options{
				Stderr:    &stderr,
				Getenv:    func(name string) string { return tt.env[name] },
				ConfigDir: func() (string, error) { return root, nil },
			}
			got, err := opt.resolveDatabase(tt.flag)
			if err != nil {
				t.Fatalf("resolveDatabase(%q) = %v", tt.flag, err)
			}
			if got.Path != tt.want || got.Private != tt.wantPrivate {
				t.Fatalf("resolveDatabase(%q) = %+v, want path %q private %v", tt.flag, got, tt.want, tt.wantPrivate)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want no notice in the package directory", stderr.String())
			}
		})
	}
}

func TestUnresolvablePerUserDirectoryRefuses(t *testing.T) {
	t.Chdir(t.TempDir())
	var stdout, stderr bytes.Buffer
	code := Run([]string{"sources", "--json"}, Options{
		Stdout:    &stdout,
		Stderr:    &stderr,
		Getenv:    func(string) string { return "" },
		ConfigDir: func() (string, error) { return "", errors.New("no config directory") },
	})
	if code != 2 {
		t.Fatalf("sources exit = %d, stderr %s", code, stderr.String())
	}
	var doc errorDoc
	if err := json.Unmarshal(stderr.Bytes(), &doc); err != nil {
		t.Fatalf("decode error doc: %v\n%s", err, stderr.String())
	}
	if doc.Error.Code != "config_dir_unavailable" {
		t.Fatalf("error code = %q, message %q", doc.Error.Code, doc.Error.Message)
	}
	for _, want := range []string{"--db", "PASSQUEST_DB", "no config directory"} {
		if !strings.Contains(doc.Error.Message, want) {
			t.Fatalf("message %q lacks %q", doc.Error.Message, want)
		}
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want nothing", stdout.String())
	}
}

func TestDatabasePathUnresolvableIsRefusal(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	opt := Options{
		Stderr:    &stderr,
		Getenv:    func(string) string { return "" },
		ConfigDir: func() (string, error) { return "", errors.New("no config directory") },
	}
	_, err := opt.resolveDatabase("")
	if err == nil {
		t.Fatal("an unresolvable per-user directory was accepted")
	}
	var refusal *study.Error
	if !errors.As(err, &refusal) || refusal.Code != "config_dir_unavailable" {
		t.Fatalf("error = %v (%T), want a config_dir_unavailable refusal", err, err)
	}
	if !strings.Contains(refusal.Message, "PASSQUEST_DB") || !strings.Contains(refusal.Message, "--db") {
		t.Fatalf("refusal message = %q", refusal.Message)
	}
}

func TestVersionAndHelp(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	opt := Options{Stdout: &stdout, Stderr: &stderr, Now: time.Now, Getenv: func(string) string { return "" }}
	if code := Run([]string{"version"}, opt); code != 0 {
		t.Fatalf("version exit = %d", code)
	}
	if !strings.Contains(stdout.String(), "passquest 0.1.0-dev") || !strings.Contains(stdout.String(), "sample-0001") {
		t.Fatalf("version output = %s", stdout.String())
	}

	stdout.Reset()
	if code := Run([]string{"help"}, opt); code != 0 || !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("help = %d, %s", code, stdout.String())
	}
	stderr.Reset()
	if code := Run(nil, opt); code != 2 || !strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("no args = %d, %s", code, stderr.String())
	}
}

func TestProcessReopenSeesStoredEvidence(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	runChild := func(args ...string) (string, string) {
		t.Helper()
		cmd := exec.Command(os.Args[0], args...)
		cmd.Env = append(os.Environ(), "PASSQUEST_TEST_RUN_MAIN=1")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("child %v: %v\nstderr:\n%s", args, err, stderr.String())
		}
		return stdout.String(), stderr.String()
	}

	first, _ := runChild("practice", "--db", db, "--question", "q1", "--answer", "C", "--json")
	firstDoc := decodeAnswered(t, first)
	if firstDoc.Attempt.Status != "incorrect" {
		t.Fatalf("child status = %q", firstDoc.Attempt.Status)
	}

	second, _ := runChild("report", "--db", db, "--json")
	report := decodeReport(t, second)
	if report.Counts.Attempts != 1 || report.Counts.Incorrect != 1 {
		t.Fatalf("reopened report counts = %+v", report.Counts)
	}
	if len(report.Attempts) != 1 || report.Attempts[0].ID != firstDoc.Attempt.ID {
		t.Fatalf("reopened attempts = %+v", report.Attempts)
	}
	if report.Attempts[0].SessionID != firstDoc.Attempt.SessionID {
		t.Fatalf("session changed across processes: %q vs %q", report.Attempts[0].SessionID, firstDoc.Attempt.SessionID)
	}
	if _, err := os.Stat(db); err != nil {
		t.Fatalf("database file: %v", err)
	}
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func checkGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create golden dir: %v", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with -update-golden): %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}
