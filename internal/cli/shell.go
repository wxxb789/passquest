package cli

import (
	"fmt"
	"path"
	"strings"
)

// shellFamily is the quoting dialect of the shell a learner pastes a generated
// command into. Pass Quest never executes these commands, so the only contract
// is that the printed text means the same argument in that shell.
type shellFamily string

const (
	shellFamilySh         shellFamily = "sh"
	shellFamilyCmd        shellFamily = "cmd"
	shellFamilyPowerShell shellFamily = "powershell"
)

// shellOverrideEnv is the explicit, documented override. A learner whose shell
// is not detectable from the environment (nested shells, a terminal multiplexer
// started from another shell) sets this instead of trusting the guess.
const shellOverrideEnv = "PASSQUEST_SHELL"

const shellOverrideHint = shellOverrideEnv + "=sh|cmd|powershell"

// shellChoice records how the family was chosen, because a guess that cannot be
// detected reliably must be visible next to the command it quoted. Source is
// one of the environment variable names, shellOverrideEnv, or "default".
type shellChoice struct {
	Family shellFamily
	Source string
	Detail string
}

// detectShell picks the family from the environment. Precedence is deliberate:
//
//  1. PASSQUEST_SHELL, an explicit answer that is never overridden.
//  2. SHELL naming a POSIX shell. This is the login shell, not necessarily the
//     current one, but it is the value a POSIX terminal keeps unchanged.
//  3. PSModulePath. PowerShell sets it; a POSIX shell started from PowerShell
//     inherits it, so it ranks below SHELL.
//  4. ComSpec on Windows, i.e. an environment with no POSIX shell and no
//     PowerShell marker in front of it.
//  5. POSIX, the previous behaviour, recorded as a default rather than a fact.
//
// No environment variable is authoritative: cmd.exe leaves an inherited SHELL
// in place and Windows defines PSModulePath machine-wide. That is why the
// caller prints the family, its source and the override hint beside the command.
func detectShell(getenv func(string) string, goos string) (shellChoice, error) {
	if override := strings.TrimSpace(getenv(shellOverrideEnv)); override != "" {
		family, ok := parseShellFamily(override)
		if !ok {
			return shellChoice{}, fmt.Errorf("%s must be sh, cmd or powershell; got %q", shellOverrideEnv, override)
		}
		return shellChoice{Family: family, Source: shellOverrideEnv, Detail: override}, nil
	}
	if shell := strings.TrimSpace(getenv("SHELL")); shell != "" {
		// Both separators are normalised first: a SHELL value from Git Bash is a
		// POSIX path, and the same value must be recognisable on any host.
		name := strings.TrimSuffix(strings.ToLower(path.Base(strings.ReplaceAll(shell, `\`, "/"))), ".exe")
		switch name {
		case "sh", "bash", "zsh", "dash", "ksh", "mksh", "ash":
			return shellChoice{Family: shellFamilySh, Source: "SHELL", Detail: shell}, nil
		}
	}
	if strings.TrimSpace(getenv("PSModulePath")) != "" {
		return shellChoice{Family: shellFamilyPowerShell, Source: "PSModulePath"}, nil
	}
	if goos == "windows" {
		if comspec := strings.TrimSpace(getenv("ComSpec")); comspec != "" {
			return shellChoice{Family: shellFamilyCmd, Source: "ComSpec", Detail: comspec}, nil
		}
	}
	return shellChoice{Family: shellFamilySh, Source: "default"}, nil
}

func parseShellFamily(value string) (shellFamily, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(shellFamilySh):
		return shellFamilySh, true
	case string(shellFamilyCmd):
		return shellFamilyCmd, true
	case string(shellFamilyPowerShell):
		return shellFamilyPowerShell, true
	}
	return "", false
}

// commandQuoter renders one argument for the detected shell.
type commandQuoter struct {
	shell shellChoice
}

// quote leaves a plainly safe argument alone and otherwise quotes it for the
// family. Reads of the result in the wrong shell are possible, which is why the
// family is printed; a path is never silently mangled into an unquoted one.
func (q commandQuoter) quote(value string) string {
	if commandArgumentIsSafe(value) {
		return value
	}
	switch q.shell.Family {
	case shellFamilyCmd:
		return quoteForCmd(value)
	case shellFamilyPowerShell:
		// PowerShell single quotes are literal; one embedded quote is escaped by
		// doubling it. Nothing inside is expanded, so `$`, backticks, `%`, `!` and
		// non-ASCII text reach the program unchanged.
		return "'" + strings.ReplaceAll(value, "'", "''") + "'"
	default:
		// POSIX single quotes are literal too: `$`, backticks, `"`, `%`, `!` and
		// non-ASCII text survive. A single quote ends the run, contributes an
		// escaped quote and reopens it.
		return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
	}
}

// quoteForCmd wraps a value in double quotes, which cmd.exe strips before the
// program sees the argument. An embedded quote is doubled, which is what the C
// runtime argument parser used by most programs decodes back to one quote; a
// backslash immediately before the closing quote is doubled for the same parser.
// Both rules only hold for a value with no backslash directly before an embedded
// quote, which a Windows path cannot contain. cmd.exe has no escape for `%`
// outside a batch file, so a `%NAME%` inside the value still expands and the
// caller warns about it.
func quoteForCmd(value string) string {
	quoted := strings.ReplaceAll(value, `"`, `""`)
	if strings.HasSuffix(quoted, `\`) {
		quoted += `\`
	}
	return `"` + quoted + `"`
}

// shellViewOf exposes the choice in the protocol document.
func shellViewOf(choice shellChoice) *shellView {
	return &shellView{
		Family:       string(choice.Family),
		DetectedFrom: choice.Source,
		Detail:       choice.Detail,
		Override:     shellOverrideHint,
	}
}

func commandArgumentIsSafe(value string) bool {
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_', r == '-', r == '.', r == '/', r == '\\', r == ':', r == '@', r == '+', r == '=', r == ',':
		default:
			return false
		}
	}
	return true
}
