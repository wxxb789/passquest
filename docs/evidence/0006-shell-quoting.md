# 0006 — Shell-aware quoting for copied `next` commands: evidence

- Tree: working tree on branch `fix/shell-quoting-and-golden-contract`, based on `5ff0a9a`
  (`feat(0010): registry-driven provider and model choice (#21)`).
- Date: 2026-11-13. Host: Linux 7.0.0-1017-azure x64.
- Toolchain: Go 1.27.3 via mise; Bun 1.4.4. No Node host, network call or provider credential was used
  in this increment.
- Decision: [`../decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md) Q-0006-06, resolved
  `(owner, 2026-11-13) option (B) confirmed`. Rules:
  [`../cross-platform.md`](../cross-platform.md) § Shell output.

All commands below ran from the repository root. The demonstration used a temporary directory and a
separately built binary (`go build -o /tmp/pq ./cmd/passquest`); nothing from it was added to the
repository.

## What was built

| Area | What changed |
|---|---|
| `internal/cli/shell.go` | `detectShell` classifies the environment into `sh`, `cmd` or `powershell`; `commandQuoter.quote` renders one argument per family. The old single POSIX renderer (`quoteCommandArgument` in `next.go`) is gone. |
| `internal/cli/next.go` | `nextCommands`, `skillPracticeCommand`, `tacticPracticeCommand` and `withDatabaseArgument` take the quoter; `nextDoc` gains `shell.{family,detected_from,detail,override}`; the human output prints the family beside the command and warns when a `cmd` command contains `%`. |
| `internal/cli/shell_test.go` | Detection table, family × hostile-path quoting table, a POSIX round trip through a real `sh -c`, and four CLI-level cases. |
| `internal/cli/testdata/next-*.golden.json` | The three `next` goldens moved by exactly the additive `shell` block (5 lines each); no other line changed. |

Detection order, deliberate and each step testable: `PASSQUEST_SHELL` (explicit; an unknown value is an
exit-2 `invalid_input` error), `SHELL` naming a POSIX shell, `PSModulePath`, `ComSpec` on Windows, then
POSIX as a recorded default. No environment variable is authoritative, so the chosen family, its source
and the override hint are printed with the command.

## Tier 1 — automated checks

| Command | Result |
|---|---|
| `go test ./internal/cli/ -count=1 -run 'TestQuoteArgumentPerShell\|TestShQuotingRoundTripsThroughSh\|TestDetectShell\|TestNextQuotesCommandsForTheDetectedShell\|TestNextWarnsAboutCmdPercentExpansion\|TestNextRejectsAnUnknownShellOverride' -v` | exit 0; 6 test functions, 38 runs, all PASS; 12 values × 3 families = 36 quoting assertions, 9 POSIX round trips, 12 detection cases, 4 CLI family cases |
| `go test ./internal/cli/ -count=1` | exit 0, whole package |
| `go vet ./internal/cli/` | exit 0 |
| `gofmt -l internal/cli` | empty output |
| `bash scripts/gate.sh` | see [harness.md](../harness.md) gate list; the final run for this branch is recorded in the pull request body |

The POSIX form is not only pinned as text: `TestShQuotingRoundTripsThroughSh` passes each rendered
argument through a real `sh -c "printf %s <quoted>"` and compares what the shell hands the program with
the original value, for `$HOME`, backticks, `"`, `'`, `%`, `!`, a space and non-ASCII text. The Windows
and PowerShell forms are pinned as text only — see Limits.

## Tier 2 — real CLI behaviour

Binary `/tmp/pq`, database path `<tmp>/100% learner data/attempts.sqlite` (space and `%`), one run per
family. Selected output lines:

```text
$ PASSQUEST_SHELL=sh /tmp/pq next --db "$db"
Command: passquest tactic practice not-except-polarity --stage guided --db '/tmp/…/100% learner data/attempts.sqlite'
Quoted for sh (from PASSQUEST_SHELL); override with PASSQUEST_SHELL=sh|cmd|powershell.

$ PASSQUEST_SHELL=cmd /tmp/pq next --db "$db"
Command: passquest tactic practice not-except-polarity --stage guided --db "/tmp/…/100% learner data/attempts.sqlite"
Quoted for cmd (from PASSQUEST_SHELL); override with PASSQUEST_SHELL=sh|cmd|powershell.
Warning: cmd.exe expands %NAME% even inside double quotes, so this command is not copy-safe with that path; set PASSQUEST_SHELL=powershell or pass the path another way.

$ PASSQUEST_SHELL=powershell /tmp/pq next --db "$db"
Command: passquest tactic practice not-except-polarity --stage guided --db '/tmp/…/100% learner data/attempts.sqlite'
Quoted for powershell (from PASSQUEST_SHELL); override with PASSQUEST_SHELL=sh|cmd|powershell.

$ SHELL=/usr/bin/zsh /tmp/pq next --db "$db"
Command: passquest tactic practice not-except-polarity --stage guided --db '/tmp/…/100% learner data/attempts.sqlite'
Quoted for sh (from SHELL=/usr/bin/zsh); override with PASSQUEST_SHELL=sh|cmd|powershell.

$ PASSQUEST_SHELL=fish /tmp/pq next --json --db "$db"; echo $?
{"schema_version": 1, "error": {"code": "invalid_input", "message": "PASSQUEST_SHELL must be sh, cmd or powershell; got \"fish\""}}
2

$ PASSQUEST_SHELL=cmd /tmp/pq next --json --db "$db"   # selected fields
{"shell": {"family": "cmd", "detected_from": "PASSQUEST_SHELL", "detail": "cmd", "override": "PASSQUEST_SHELL=sh|cmd|powershell"}}
```

## Tier 3 — protocol goldens

| Golden | Diff against `5ff0a9a` |
|---|---|
| `internal/cli/testdata/next-empty.golden.json` | +5 lines: the `shell` object (`family`, `detected_from`, `override`); `detected_from` is `default` because the test environment is empty |
| `internal/cli/testdata/next-after-vocab.golden.json` | +5 lines, same object |
| `internal/cli/testdata/next-slow.golden.json` | +5 lines, same object |

No existing field or ordering changed. `TestGoldenNext*` passes unchanged otherwise.

## Limits

- **Windows and PowerShell quoting is pinned as text, not executed here.** This host is Linux: no
  `cmd.exe`, no `powershell.exe`/`pwsh`. The table tests pin the exact rendered argument for spaces,
  `$`, backticks, `"`, `'`, `%`, `!`, a trailing `\` and non-ASCII text, and `cross-platform.md`
  records the rules. The 3-OS CI run (`windows-latest`) is the remaining evidence for those two
  families; nothing in this record claims they were run.
- **`cmd.exe` cannot suppress `%NAME%` expansion outside a batch file.** A rendered `cmd` command whose
  argument contains a variable reference is therefore not copy-safe; the CLI prints a warning when it
  detects `%` in the command it just rendered. Doubling `%` was rejected because it is correct only
  inside a batch file and wrong at the interactive prompt. Delayed expansion (`!`) is off by default
  and is not defended against.
- **Detection is a documented guess.** `cmd.exe` keeps an inherited `SHELL`; Windows defines
  `PSModulePath` machine-wide; PowerShell started from `bash` inherits `SHELL=/bin/bash` and is quoted
  as `sh`. Each of those is visible in the printed line, which is the contract: the learner sees the
  family and the override before pasting.
- The `sh` round trip runs wherever `sh` is on `PATH` (Linux, macOS and Git Bash on Windows); where it is
  absent the text table still runs. No test is marked `skip` — the gate forbids skips.
- The default database path still carries no `--db` argument, so a learner on the app-owned default path
  sees no quoting at all; the disclosure line prints anyway.

## Not in scope

- `next` is the only command that renders a path argument today (`--db`, `--source`, `--question`). The
  other printed actions (`passquest skill show <id>`, `passquest model test provider/model`) carry a
  registered slug and no path, so they need no family-specific quoting. A copyable command added later
  must use `commandQuoter`.
- No attempt, schema, migration or stored-evidence change: quoting affects printed text only.
