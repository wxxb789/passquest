package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wxxb789/passquest/internal/cmdline"
)

// testQuoter is the renderer existing next-command tests assert against: the
// hermetic test environment has no shell variables, so `next` defaults to the
// POSIX family there.
var testQuoter = commandQuoter{shell: cmdline.ShellChoice{Family: cmdline.ShellSh, Source: "default"}}

// runCLIWithEnv runs one command with a controlled environment. Paths are given
// explicitly so a test can select a database path with hostile characters.
func runCLIWithEnv(t *testing.T, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	clock := &stepClock{base: goldenBase}
	opt := Options{
		Stdout:     &stdout,
		Stderr:     &stderr,
		Now:        clock.Now,
		NewSession: func() string { return "sess-test" },
		Getenv:     func(key string) string { return env[key] },
	}
	code := Run(args, opt)
	return code, stdout.String(), stderr.String()
}

func TestNextQuotesCommandsForTheDetectedShell(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "learner data")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "attempts.sqlite")

	tests := []struct {
		name     string
		env      map[string]string
		wantJSON string
		wantText string
	}{
		{
			name:     "default is POSIX and is labelled a default",
			wantJSON: "sh",
			wantText: "Quoted for sh (no shell detected, defaulted); override with " + cmdline.ShellOverrideHint + ".",
		},
		{
			name:     "SHELL names zsh",
			env:      map[string]string{"SHELL": "/usr/bin/zsh"}, // portable-paths: allow an environment value, never resolved
			wantJSON: "sh",
			wantText: "Quoted for sh (from SHELL=/usr/bin/zsh); override with ",
		},
		{
			name:     "override selects cmd",
			env:      map[string]string{cmdline.ShellOverrideEnv: "cmd"},
			wantJSON: "cmd",
			wantText: "Quoted for cmd (from " + cmdline.ShellOverrideEnv + "); override with ",
		},
		{
			name:     "override selects PowerShell",
			env:      map[string]string{cmdline.ShellOverrideEnv: "powershell"},
			wantJSON: "powershell",
			wantText: "Quoted for powershell (from " + cmdline.ShellOverrideEnv + "); override with ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := runCLIWithEnv(t, tt.env, "next", "--json", "--db", dbPath)
			if code != 0 {
				t.Fatalf("exit = %d, stderr = %s", code, errOut)
			}
			doc := decodeNext(t, out)
			if doc.Shell == nil || doc.Shell.Family != tt.wantJSON {
				t.Fatalf("shell view = %+v, want family %q", doc.Shell, tt.wantJSON)
			}
			if doc.Shell.Override != cmdline.ShellOverrideHint {
				t.Fatalf("override hint = %q, want %q", doc.Shell.Override, cmdline.ShellOverrideHint)
			}
			if doc.Recommendation == nil {
				t.Fatalf("no recommendation in %s", out)
			}
			quoter := commandQuoter{shell: cmdline.ShellChoice{Family: cmdline.ShellFamily(tt.wantJSON)}}
			if want := "--db " + quoter.quote(dbPath); !strings.Contains(doc.Recommendation.Command, want) {
				t.Fatalf("command = %q, want it to contain %q", doc.Recommendation.Command, want)
			}

			code, human, errOut := runCLIWithEnv(t, tt.env, "next", "--db", dbPath)
			if code != 0 {
				t.Fatalf("human exit = %d, stderr = %s", code, errOut)
			}
			if !strings.Contains(human, tt.wantText) {
				t.Fatalf("human output does not disclose the shell guess:\n%s", human)
			}
			if strings.Contains(human, "Warning: cmd.exe expands") {
				t.Fatalf("unexpected cmd %% warning for a path without a variable reference:\n%s", human)
			}
		})
	}
}

func TestNextWarnsAboutCmdPercentExpansion(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "100% data")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "attempts.sqlite")
	env := map[string]string{cmdline.ShellOverrideEnv: "cmd"}

	code, out, errOut := runCLIWithEnv(t, env, "next", "--json", "--db", dbPath)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	doc := decodeNext(t, out)
	if doc.Recommendation == nil || !strings.Contains(doc.Recommendation.Command, `"`+dbPath+`"`) {
		t.Fatalf("cmd command = %+v, want the path in double quotes", doc.Recommendation)
	}

	code, human, errOut := runCLIWithEnv(t, env, "next", "--db", dbPath)
	if code != 0 {
		t.Fatalf("human exit = %d, stderr = %s", code, errOut)
	}
	if !strings.Contains(human, "Warning: cmd.exe expands %NAME% even inside double quotes") {
		t.Fatalf("a cmd command with %% is not disclosed as unsafe:\n%s", human)
	}
}

func TestNextRejectsAnUnknownShellOverride(t *testing.T) {
	t.Parallel()
	env := map[string]string{cmdline.ShellOverrideEnv: "fish"}
	code, out, errOut := runCLIWithEnv(t, env, "next", "--json", "--db", filepath.Join(t.TempDir(), "attempts.sqlite"))
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stdout %q, stderr %q)", code, out, errOut)
	}
	if !strings.Contains(errOut, "invalid_input") || !strings.Contains(errOut, "must be sh, cmd or powershell") {
		t.Fatalf("stderr = %q, want an invalid_input error naming the accepted values", errOut)
	}
}
