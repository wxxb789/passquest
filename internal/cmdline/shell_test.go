package cmdline

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// TestQuoteArgumentPerShell pins every shell family against every hostile path
// shape. The forms are contract: a change here changes what a learner copies.
func TestQuoteArgumentPerShell(t *testing.T) {
	t.Parallel()
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
		for family, want := range map[ShellFamily]string{ShellSh: tt.sh, ShellCmd: tt.cmd, ShellPowerShell: tt.ps} {
			quoter := CommandQuoter{Shell: ShellChoice{Family: family}}
			if got := quoter.Quote(tt.in); got != want {
				t.Errorf("%s in %s: quote(%q) = %q, want %q", tt.name, family, tt.in, got, want)
			}
		}
	}
}

// TestShQuotingRoundTripsThroughSh copies each rendered POSIX argument through a
// real `sh` parse and compares the argument the shell hands the program with the
// original value. Pinning text proves the shape; this proves the paste.
func TestShQuotingRoundTripsThroughSh(t *testing.T) {
	t.Parallel()
	sh, err := exec.LookPath("sh")
	if err != nil {
		return
	}
	quoter := CommandQuoter{Shell: ShellChoice{Family: ShellSh}}
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
		quoted := quoter.Quote(value)
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
	t.Parallel()
	tests := []struct {
		name  string
		goos  string
		env   map[string]string
		want  ShellChoice
		error bool
	}{
		{
			name: "no shell variables falls back to POSIX and says so",
			goos: "linux",
			want: ShellChoice{Family: ShellSh, Source: "default"},
		},
		{
			name: "SHELL names bash",
			goos: "linux",
			env:  map[string]string{"SHELL": "/bin/bash"},
			want: ShellChoice{Family: ShellSh, Source: "SHELL", Detail: "/bin/bash"},
		},
		{
			name: "SHELL names zsh",
			goos: "darwin",
			env:  map[string]string{"SHELL": "/usr/local/bin/zsh"},                            // portable-paths: allow an environment value, never resolved
			want: ShellChoice{Family: ShellSh, Source: "SHELL", Detail: "/usr/local/bin/zsh"}, // portable-paths: allow an environment value, never resolved
		},
		{
			name: "SHELL names a Windows bash",
			goos: "windows",
			env:  map[string]string{"SHELL": `C:\Program Files\Git\usr\bin\bash.exe`, "ComSpec": `C:\Windows\system32\cmd.exe`}, // portable-paths: allow an environment value, never resolved
			want: ShellChoice{Family: ShellSh, Source: "SHELL", Detail: `C:\Program Files\Git\usr\bin\bash.exe`},                // portable-paths: allow an environment value, never resolved
		},
		{
			name: "PSModulePath without a POSIX SHELL is PowerShell",
			goos: "windows",
			env:  map[string]string{"PSModulePath": `C:\Program Files\PowerShell\Modules`, "ComSpec": `C:\Windows\system32\cmd.exe`}, // portable-paths: allow an environment value, never resolved
			want: ShellChoice{Family: ShellPowerShell, Source: "PSModulePath"},
		},
		{
			name: "ComSpec on Windows is cmd",
			goos: "windows",
			env:  map[string]string{"ComSpec": `C:\Windows\system32\cmd.exe`},                             // portable-paths: allow an environment value, never resolved
			want: ShellChoice{Family: ShellCmd, Source: "ComSpec", Detail: `C:\Windows\system32\cmd.exe`}, // portable-paths: allow an environment value, never resolved
		},
		{
			name: "ComSpec off Windows does not decide",
			goos: "linux",
			env:  map[string]string{"ComSpec": "/usr/bin/sh"}, // portable-paths: allow an environment value, never resolved
			want: ShellChoice{Family: ShellSh, Source: "default"},
		},
		{
			name: "an unknown SHELL falls through",
			goos: "linux",
			env:  map[string]string{"SHELL": "/usr/bin/fish"}, // portable-paths: allow an environment value, never resolved
			want: ShellChoice{Family: ShellSh, Source: "default"},
		},
		{
			name: "an unknown SHELL still lets PSModulePath decide",
			goos: "linux",
			env:  map[string]string{"SHELL": "/usr/bin/fish", "PSModulePath": "/opt/pwsh/Modules"}, // portable-paths: allow an environment value, never resolved
			want: ShellChoice{Family: ShellPowerShell, Source: "PSModulePath"},
		},
		{
			name: "the override beats every environment variable",
			goos: "windows",
			env:  map[string]string{ShellOverrideEnv: "cmd", "SHELL": "/bin/bash", "PSModulePath": "/m"},
			want: ShellChoice{Family: ShellCmd, Source: ShellOverrideEnv, Detail: "cmd"},
		},
		{
			name: "the override is case-insensitive",
			goos: "linux",
			env:  map[string]string{ShellOverrideEnv: "PowerShell"},
			want: ShellChoice{Family: ShellPowerShell, Source: ShellOverrideEnv, Detail: "PowerShell"},
		},
		{
			name:  "an unknown override is an error rather than a silent guess",
			goos:  "linux",
			env:   map[string]string{ShellOverrideEnv: "fish"},
			error: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DetectShell(func(key string) string { return tt.env[key] }, tt.goos)
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
