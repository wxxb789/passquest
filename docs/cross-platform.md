# Cross-platform rules

Pass Quest ships on **Windows, macOS and Linux**: the owner develops on all three, the 3-OS CI matrix
is the only proof of platform behaviour, and a green Linux job says nothing about Windows. Every path,
process and filesystem decision is made once, correctly, for all three — not patched after Windows CI
turns red.

## Paths (Go)

- `filepath.Join`, `filepath.IsAbs`, `filepath.ToSlash`, `filepath.VolumeName`. Never `"/" + name`,
  never `strings.Split(p, "/")`.
- User directories come from `os.UserConfigDir` / `os.UserHomeDir` / `os.TempDir`. `$HOME`, `~` and
  `%APPDATA%` are not portable source.
- A path literal such as `"/tmp/x.sqlite"` is **not absolute on Windows** (`filepath.IsAbs` is false),
  so validation rejects it and the test then blames the product. Build fixtures with `t.TempDir()` and
  `filepath.Join`. The `portable-paths` gate enforces this: a literal that is genuinely pure string data
  must carry `// portable-paths: allow <why>` on the line.
- The `portable-paths` gate reads both Go string forms, so a backticked raw string (`C:\Users\…`) is
  caught as well as the escaped one.
- Compare normalised forms: `filepath.ToSlash(filepath.Clean(p))`, and mask the temp prefix before
  comparing against a golden. Never substitute a path into serialised JSON by text replacement — the
  JSON encoder escapes separators (`C:\\Users\\…`), so the substitution silently misses. Decode, then
  substitute, then compare.
- SQLite file URIs are built, not concatenated (`internal/store.sqliteURI`): POSIX and macOS give
  `file:///home/u/db.sqlite`, Windows gives `file:///C:/Users/u/db.sqlite`, a UNC path gives
  `file:////server/share/db.sqlite`. The `_pragma`/`_txlock` query must survive byte-identically.
  A raw `url.URL{Path: "C:\\Users\\…"}` produces `file:C:%5CUsers…` — which is what broke Windows CI
  once already.

## Processes (Go)

- `exec.Command` with an argument slice. No shell string building, no assumption about quoting rules;
  PowerShell and `sh` disagree about `$`, backslash and quotes.
- Signals are POSIX API. `os.FindProcess` succeeds for any pid on Windows and `Process.Signal` refuses
  everything but `Kill`, so cancellation must travel through the durable control file the host polls
  (`internal/agent`); a signal is an optimisation on POSIX only.
- There are no process groups or job objects on Windows: no group kill, no negative pid. The bounded
  `killProcess` fallback exists for exactly this reason.

## Filesystem semantics

- POSIX mode bits are not a correctness property. `os.Chmod(0o600)` does not create a restrictive ACL
  on Windows; assert a mode only behind `runtime.GOOS != "windows"` and report the assertion as skipped
  rather than weakening it silently.
- Paths are case-insensitive on Windows and macOS by default, and may be longer than 260 characters
  without the long-path opt-in. Do not rely on case sensitivity, and do not assume `A` and `a` differ.
- Do not assume a file can be deleted while open, or renamed over an open handle.

## The Node host

- `node:path` `join`/`resolve`, `os.tmpdir()`, `os.homedir()`. No `/` concatenation.
- `new URL(...).hostname` returns bracketed IPv6 (`[::1]`); Go's `ValidateEndpoint` sees the unbracketed
  form. Normalise before comparing, or the same endpoint is accepted by one layer and refused by the
  other.
- A child process on Windows is killed with `taskkill`-level semantics, not with a signal.

## Shell output

- A command the learner copies is shell text. `quoteCommandArgument` (`internal/cli/next.go`) currently
  emits a POSIX form: double quotes with `\$` escaping. That is correct for `sh`/`zsh`/`bash` and wrong
  for `cmd.exe`/PowerShell. This is a known gap, not a decision: see
  [decisions/OPEN-QUESTIONS.md](decisions/OPEN-QUESTIONS.md).

## Tests

- Fixtures: `t.TempDir()` + `filepath.Join`. Scope a platform difference with an explicit `GOOS` field
  in a table test (see `internal/store/store_test.go` `sqliteURI` cases) instead of hoping the assertion
  holds everywhere.
- Goldens: no absolute paths stored; decode → substitute temp prefix → normalise separators → compare.
- Prove a Windows-only behaviour with a fixture that reproduces the shape on any OS (an escaped
  `C:\\…` string, a bracketed IPv6 URL) plus the 3-OS run. A test that can only fail on Windows is a
  test nobody can debug locally.

## Gates

`bun run gate` → 17 gates; `portable-paths` fails on a platform-bound literal, `go test` and the smoke
gates run on all three OS in CI.
