package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// testQuoter is the renderer existing next-command tests assert against: the
// hermetic test environment has no shell variables, so `next` defaults to the
// POSIX family there.
var testQuoter = commandQuoter{shell: shellChoice{Family: shellFamilySh, Source: "default"}}

// TestQuoteArgumentPerShell pins every shell family against every hostile path
// shape. The forms are contract: a change here changes what a learner copies.
func TestQuoteArgumentPerShell(t *testing.T) {
	tests := []struct {
		name string
		in   string
		sh   string
		cmd  string
		ps   string
	}{
		{"plain", "attempts.sqlite", "attempts.sqlite", "attempts.sqlite", "attempts.sqlite"},
		{"space", "learner data/attempts.sqlite", "'learner data/attempts.sqlite'", `"learner data/attempts.sqlite"`, "'learner data/attempts.sqlite'"},
		{"dollar", "$HOME/attempts.sqlite", "'$HOME/attempts.sqlite'", `"$HOME/attempts.sqlite"`, "'$HOME/attempts.sqlite'"},
		{"backtick", "`id`/attempts.sqlite", "'`id`/attempts.sqlite'", "\"`id`/attempts.sqlite\"", "'`id`/attempts.sqlite'"},
		{"double quote", `a"b/attempts.sqlite`, `'a"b/attempts.sqlite'`, `"a""b/attempts.sqlite"`, `'a"b/attempts.sqlite'`},
		{"single quote", "it's/attempts.sqlite", `'it'\''s/attempts.sqlite'`, `"it's/attempts.sqlite"`, "'it''s/attempts.sqlite'"},
		{"percent", "100%/attempts.sqlite", "'100%/attempts.sqlite'", `"100%/attempts.sqlite"`, "'100%/attempts.sqlite'"},
		{"bang", "wow!/attempts.sqlite", "'wow!/attempts.sqlite'", `"wow!/attempts.sqlite"`, "'wow!/attempts.sqlite'"},
		{"unicode", "学习/练习.sqlite", "'学习/练习.sqlite'", `"学习/练习.sqlite"`, "'学习/练习.sqlite'"},
		{"windows path", `C:\Users\A Name\attempts.sqlite`, `'C:\Users\A Name\attempts.sqlite'`, `"C:\Users\A Name\attempts.sqlite"`, `'C:\Users\A Name\attempts.sqlite'`},               // portable-paths: allow the quoting form is the assertion, never resolved
		{"windows trailing separator", `C:\Users\A Name\`, `'C:\Users\A Name\'`, `"C:\Users\A Name\\"`, `'C:\Users\A Name\'`},                                                            // portable-paths: allow the quoting form is the assertion, never resolved
		{"windows variable reference", `C:\Users\%TEMP%\attempts.sqlite`, `'C:\Users\%TEMP%\attempts.sqlite'`, `"C:\Users\%TEMP%\attempts.sqlite"`, `'C:\Users\%TEMP%\attempts.sqlite'`}, // portable-paths: allow the quoting form is the assertion, never resolved
	}
	for _, tt := range tests {
		for family, want := range map[shellFamily]string{shellFamilySh: tt.sh, shellFamilyCmd: tt.cmd, shellFamilyPowerShell: tt.ps} {
			quoter := commandQuoter{shell: shellChoice{Family: family}}
			if got := quoter.quote(tt.in); got != want {
				t.Errorf("%s in %s: quote(%q) = %q, want %q", tt.name, family, tt.in, got, want)
			}
		}
	}
}

// TestShQuotingRoundTripsThroughSh copies each rendered POSIX argument through a
// real `sh` parse and compares the argument the shell hands the program with the
// original value. Pinning text proves the shape; this proves the paste.
func TestShQuotingRoundTripsThroughSh(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		return
	}
	quoter := commandQuoter{shell: shellChoice{Family: shellFamilySh}}
	values := []string{
		"attempts.sqlite",
		"learner data/attempts.sqlite",
		"$HOME/attempts.sqlite",
		"`id`/attempts.sqlite",
		`a"b/attempts.sqlite`,
		"it's/attempts.sqlite",
		"100%/attempts.sqlite",
		"wow!/attempts.sqlite",
		"学习/练习.sqlite",
	}
	for _, value := range values {
		quoted := quoter.quote(value)
		command := exec.Command(sh, "-c", "printf %s "+quoted) //nolint:gosec // the test's subject is the escaped string
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		if err := command.Run(); err != nil {
			t.Errorf("sh rejected quote(%q) = %q: %v %s", value, quoted, err, stderr.String())
			continue
		}
		if got := stdout.String(); got != value {
			t.Errorf("sh received %q for quote(%q) = %q, want %q", got, value, quoted, value)
		}
	}
}

func TestDetectShell(t *testing.T) {
	tests := []struct {
		name  string
		goos  string
		env   map[string]string
		want  shellChoice
		error bool
	}{
		{
			name: "no shell variables falls back to POSIX and says so",
			goos: "linux",
			want: shellChoice{Family: shellFamilySh, Source: "default"},
		},
		{
			name: "SHELL names bash",
			goos: "linux",
			env:  map[string]string{"SHELL": "/bin/bash"},
			want: shellChoice{Family: shellFamilySh, Source: "SHELL", Detail: "/bin/bash"},
		},
		{
			name: "SHELL names zsh",
			goos: "darwin",
			env:  map[string]string{"SHELL": "/usr/local/bin/zsh"},                                  // portable-paths: allow an environment value, never resolved
			want: shellChoice{Family: shellFamilySh, Source: "SHELL", Detail: "/usr/local/bin/zsh"}, // portable-paths: allow an environment value, never resolved
		},
		{
			name: "SHELL names a Windows bash",
			goos: "windows",
			env:  map[string]string{"SHELL": `C:\Program Files\Git\usr\bin\bash.exe`, "ComSpec": `C:\Windows\system32\cmd.exe`}, // portable-paths: allow an environment value, never resolved
			want: shellChoice{Family: shellFamilySh, Source: "SHELL", Detail: `C:\Program Files\Git\usr\bin\bash.exe`},          // portable-paths: allow an environment value, never resolved
		},
		{
			name: "PSModulePath without a POSIX SHELL is PowerShell",
			goos: "windows",
			env:  map[string]string{"PSModulePath": `C:\Program Files\PowerShell\Modules`, "ComSpec": `C:\Windows\system32\cmd.exe`}, // portable-paths: allow an environment value, never resolved
			want: shellChoice{Family: shellFamilyPowerShell, Source: "PSModulePath"},
		},
		{
			name: "ComSpec on Windows is cmd",
			goos: "windows",
			env:  map[string]string{"ComSpec": `C:\Windows\system32\cmd.exe`},                                   // portable-paths: allow an environment value, never resolved
			want: shellChoice{Family: shellFamilyCmd, Source: "ComSpec", Detail: `C:\Windows\system32\cmd.exe`}, // portable-paths: allow an environment value, never resolved
		},
		{
			name: "ComSpec off Windows does not decide",
			goos: "linux",
			env:  map[string]string{"ComSpec": "/usr/bin/sh"}, // portable-paths: allow an environment value, never resolved
			want: shellChoice{Family: shellFamilySh, Source: "default"},
		},
		{
			name: "an unknown SHELL falls through",
			goos: "linux",
			env:  map[string]string{"SHELL": "/usr/bin/fish"}, // portable-paths: allow an environment value, never resolved
			want: shellChoice{Family: shellFamilySh, Source: "default"},
		},
		{
			name: "an unknown SHELL still lets PSModulePath decide",
			goos: "linux",
			env:  map[string]string{"SHELL": "/usr/bin/fish", "PSModulePath": "/opt/pwsh/Modules"}, // portable-paths: allow an environment value, never resolved
			want: shellChoice{Family: shellFamilyPowerShell, Source: "PSModulePath"},
		},
		{
			name: "the override beats every environment variable",
			goos: "windows",
			env:  map[string]string{shellOverrideEnv: "cmd", "SHELL": "/bin/bash", "PSModulePath": "/m"},
			want: shellChoice{Family: shellFamilyCmd, Source: shellOverrideEnv, Detail: "cmd"},
		},
		{
			name: "the override is case-insensitive",
			goos: "linux",
			env:  map[string]string{shellOverrideEnv: "PowerShell"},
			want: shellChoice{Family: shellFamilyPowerShell, Source: shellOverrideEnv, Detail: "PowerShell"},
		},
		{
			name:  "an unknown override is an error rather than a silent guess",
			goos:  "linux",
			env:   map[string]string{shellOverrideEnv: "fish"},
			error: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := detectShell(func(key string) string { return tt.env[key] }, tt.goos)
			if tt.error {
				if err == nil {
					t.Fatalf("detectShell(%v) = %+v, want an error", tt.env, got)
				}
				if !strings.Contains(err.Error(), "must be sh, cmd or powershell") {
					t.Fatalf("error = %v, want the accepted values", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("detectShell(%v) error: %v", tt.env, err)
			}
			if got != tt.want {
				t.Fatalf("detectShell(%v) = %+v, want %+v", tt.env, got, tt.want)
			}
		})
	}
}

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
			wantText: "Quoted for sh (no shell detected, defaulted); override with " + shellOverrideHint + ".",
		},
		{
			name:     "SHELL names zsh",
			env:      map[string]string{"SHELL": "/usr/bin/zsh"}, // portable-paths: allow an environment value, never resolved
			wantJSON: "sh",
			wantText: "Quoted for sh (from SHELL=/usr/bin/zsh); override with ",
		},
		{
			name:     "override selects cmd",
			env:      map[string]string{shellOverrideEnv: "cmd"},
			wantJSON: "cmd",
			wantText: "Quoted for cmd (from " + shellOverrideEnv + "); override with ",
		},
		{
			name:     "override selects PowerShell",
			env:      map[string]string{shellOverrideEnv: "powershell"},
			wantJSON: "powershell",
			wantText: "Quoted for powershell (from " + shellOverrideEnv + "); override with ",
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
			if doc.Shell.Override != shellOverrideHint {
				t.Fatalf("override hint = %q, want %q", doc.Shell.Override, shellOverrideHint)
			}
			if doc.Recommendation == nil {
				t.Fatalf("no recommendation in %s", out)
			}
			quoter := commandQuoter{shell: shellChoice{Family: shellFamily(tt.wantJSON)}}
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
	dir := filepath.Join(t.TempDir(), "100% data")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "attempts.sqlite")
	env := map[string]string{shellOverrideEnv: "cmd"}

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
	env := map[string]string{shellOverrideEnv: "fish"}
	code, out, errOut := runCLIWithEnv(t, env, "next", "--json", "--db", filepath.Join(t.TempDir(), "attempts.sqlite"))
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stdout %q, stderr %q)", code, out, errOut)
	}
	if !strings.Contains(errOut, "invalid_input") || !strings.Contains(errOut, "must be sh, cmd or powershell") {
		t.Fatalf("stderr = %q, want an invalid_input error naming the accepted values", errOut)
	}
}
