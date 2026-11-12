# Harness

The harness is the part of this repository that makes a claim checkable by a machine. It is expected
to grow with the product: a new capability that nothing can verify is unfinished work.

Binding project conventions (stack, truth ownership, layout, uncertainty protocol):
[`conventions.md`](conventions.md).

## One entry point

```bash
bun run gate
```

`bash scripts/gate.sh` remains a compatibility shim that execs the same command. There is no second
gate definition: CI and a local run both call `harness/gate.ts` through the root `gate` script.

The harness is a **Bun + TypeScript program** — one module per gate under `harness/gates/`, shared
code under `harness/lib/`. File-based gates read the working tree: tracked paths plus untracked,
non-ignored paths that exist, so a pre-commit run scans new deliverables and does not fail on a
pending archive move. Decision and rationale:
[`decisions/D-0001-bun-typescript-harness.md`](decisions/D-0001-bun-typescript-harness.md). The
migration landed with goal 0002; no `bash` or `python3` gate scripts remain.

CI (`.github/workflows/gate.yml`) runs `bun install --frozen-lockfile` and then `bun run gate` on
ubuntu, macos and windows. A green Linux job is not evidence for Windows; the matrix is the evidence.

## Gates, in order

| # | Gate | Module | Reading rule |
|---|------|--------|--------------|
| 1 | Typecheck | `harness/gates/typecheck.ts` | `tsc --noEmit` over `harness/**`; a diagnostic is a defect. Requires `bun install`; `bun.lock` is committed. |
| 2 | Format | `harness/gates/gofmt.ts` | Any listed working-tree `*.go` file is a failure; fix formatting, do not add an ignore list. |
| 3 | Vet | `harness/gates/go-vet.ts` | A vet diagnostic is a defect, not noise. |
| 4 | Build | `harness/gates/go-build.ts` | Must succeed with an empty module cache too (CI proves it). |
| 5 | Test | `harness/gates/go-test.ts` | No skipped tests: `go test -json` test-level `skip` actions fail the gate and are named. A failing test is named with the last 12 output lines it produced, so the reason is in the gate log. A test that only asserts a count is not verification. |
| 6 | Tidy | `harness/gates/go-tidy.ts` | Empty diff; a dirty module file means a dependency was added or dropped silently. |
| 7 | Docs links | `harness/gates/docs-links.ts` | Relative links in working-tree markdown must resolve to a real path. |
| 8 | Secrets / sources | `harness/gates/secrets.ts` | Credentials, private keys, `.env` files and confidential source files outside the allowlist fail the run. Finding locations are printed, never the matched secret. Every working-tree text file is scanned; there is no size cap. |
| 9 | Local-only CLI | `harness/gates/local-only.ts` | Pass Quest's own packages must not import `net`, `net/http`, `net/rpc` or `net/smtp`; files under `cmd/` and `internal/` — runtime **and** test files (`*.go`, `*.json`, `*.md`, `*.txt`) — must not contain the internal-only source marker. `net` is linked transitively by the pure-Go SQLite chain (`modernc.org/sqlite` → `libc` → `google/uuid`) and is never called; runtime proof belongs in the evidence record. |
| 10 | Baseline integrity | `harness/gates/baseline.ts` | The committed page-linked baseline must keep 165 pages / 22 native / 143 OCR, the manifest page set and the page files must be exactly 1..165, every manifest byte and non-space character count must match its file (Unicode `White_Space` plus form feed, matching `source.CountNonSpace`), each page must be valid UTF-8 (strict decode; a literal U+FFFD is allowed), the README must state `unreviewed`, and no Go test or `testdata/` fixture may reference `data/baselines`. The baseline is working context, never an answer key or fixture. |
| 11 | Import smoke | `harness/gates/import-smoke.ts` | Builds the real CLI and drives a deliberately damaged redistributable fixture: unreviewed pages and unmapped answers must be visible, practice must be refused until page and answer are reviewed, the pre-answer JSON must hide the key, an attempt must survive into a second process's report, and the database must be `0600`. 35 assertions on POSIX; on Windows the database-mode assertion is reported as skipped (34 assertions, 1 skipped) because Windows has no POSIX mode. |
| 12 | Tactic smoke | `harness/gates/tactic-smoke.ts` | Drives the real CLI through `tactic list`, `show`, the guided awaiting/answer pair, the independent awaiting/answer pair, and `report --json`. The lesson must carry why/when/steps/boundary; the guided awaiting document must be `assisted`, carry guidance and hide the key, and the guided answered document must carry the same guidance; the independent awaiting and answered documents must be `independent` and carry no guidance, with a different question and a key-hiding pre-answer document; the report must show one assisted and one independent attempt, both correct, with `correct`, `assisted_correct` and `independent_correct` visible; an exhausted independent request must exit 2. On a second database, a guided pre-answer document must record exposure without an attempt, the exposed question must be refused as independent (`invalid_input`), and the next guided request must resume that question. 70 assertions. |
| 13 | Adaptive smoke | `harness/gates/adaptive-smoke.ts` | Builds the real CLI and drives `skill list/show`, a fresh-passage `skill practice` awaiting/answered pair with key hiding and a skill snapshot, and `next --json`. The empty state must select the scoring explanation and `not-except-polarity`; one independent observation must be `insufficient`; two correct fresh passages must be `supported`; a repeat must be a revisit, not transfer; a third vocabulary passage must exhaust fresh material with its own action `review`, while the top recommendation yields to the untouched scoring skill because review cannot add evidence; an exhausted scoring skill must likewise yield to untouched language material; reviewing must be selected only when every skill is exhausted, and its command must name a seen question; generated commands must preserve the selected database and a review command must include `--db`; the goal-0003 `t1`/`t2` pair on one passage must show 1 assisted, 0 independent and 1 passage re-exposure; an exhausted weak or mixed-latest-incorrect sentence skill must keep its own `explain`/`unresolved` inference with zero fresh passages while the top recommendation yields to untouched vocabulary material, and the sentence explanation must be selected again once every skill is exhausted; reopened `next` output must be byte-identical; unknown skills must exit 2; an untagged sample attempt must be listed as unclassified. 120 assertions. |
| 14 | Tutor smoke | `harness/gates/tutor-smoke.ts` | Builds the real CLI and the real Node host against a loopback OpenAI-compatible stub, with no external network and no real credential. It must configure one endpoint, keep `tutor preview` send-free, hash the raw body the stub received and match it to `preview.request_sha256` and the recorded call (including a `&`/`<`/`>` body the host must post verbatim), record an advisory call with usage, leave the attempt report byte-identical, withhold imported source text before a recorded consent and include it after, withhold it again after revoke, label the human config/advisory/show/history/consents output, record provider/auth/malformed/rate-limited/timeout/network and missing-Node failures without changing attempts, reject host requests outside the documented token and message bounds or whose body does not match its chat, count prompt and response bounds in code points, redact a credential echoed in a provider error or host stderr, enforce the daily budget including failures, and find no credential in any output or SQLite file. 156 assertions. |
| 15 | Agent smoke | `harness/gates/agent-smoke.ts` | Builds the real CLI and the real Pi Durable host against a loopback OpenAI-compatible SSE stub, with no external network and no real credential. It must complete a two-step brief with no attempt/report change; expose status, show, runs and transcript; recover a timeout pause with the same run and coherent context; rerun a fault-injected effect and prove 2 steps / 3 attempts / 1 duplicate / 1 reconciliation event with the attempt report unchanged; refuse a resume beside the running host, then cancel that host from a second process and refuse resume afterwards; map a 401 to `needs_action`, a malformed stream to `failed`, and a completed answer with zero committed steps to `failed`/`no_steps`; keep `practice` working with Node absent; keep a completed run completed while a later run on the same store is paused; keep a cancelled run's recovered tool call out of a newer run on the same store; and accept a bracketed IPv6 loopback endpoint through the host config path. The Pi store and heartbeat must be `0600` on POSIX. |
| 16 | Note smoke | `harness/gates/note-smoke.ts` | Builds the real CLI and the real Anki host against a loopback AnkiConnect stub, with no external network and no real deck. It must draft from a stored attempt, withhold imported text before consent and include it after, keep an unanswered question's key out of the draft and preview, accept once and report an unchanged repeat, bump and retain a revision, export HTML/TSV through a temporary file, render a self-contained system-font model, create the deck and model once, add a note, send no write for an unchanged push, update the same note id on revision, surface a learner edit as a conflict until `--force`, report a duplicate as `already_present`, queue an unreachable endpoint and drain it exactly once, refuse a revoked consent before any endpoint contact, report a missing Node as `anki_unavailable` without losing the queue, reject a remote endpoint or unknown action in the host, and accept an IPv4-mapped loopback endpoint there. 805 assertions. |
| 17 | Find smoke | `harness/gates/find-smoke.ts` | Builds the real CLI and the real search host against a loopback search stub, with no external network and no real credential. It must derive the documented intent from stored attempts, make zero network calls and store no query with no endpoint (asserted against both a stub request counter and a raw TCP accept counter with a probe proving the counter works), report `nothing_to_find` for empty evidence, reject a remote plain-http endpoint, classify all three candidate classes, store provenance and recompute a candidate hash, recompute the request SHA-256 and byte count from the raw body the stub received, prove that no candidate URL is fetched, leave `report` and `next` byte-identical after storing an injected instruction, report empty and all-link-only outcomes, map a 429 to `rate_limited`, refuse the sixth query of the day before contacting the provider, refuse a provider response over the candidate limit without truncating, keep the budget slot and store a `provider_error` row when a host posts and then exits nonzero, and reject a malformed provider body as `invalid_response`. |
| 18 | Assess smoke | `harness/gates/assess-smoke.ts` | Builds the real CLI and seeds a generated reviewed source in a temporary database. It must report the holdout pool in items and content units, prove one exposed question spends all four siblings on its page while a fresh page still counts, spend a page shown before any question was authored on it, refuse a four-item pool with the item and unit shortfall, run 20 items with stored per-item elapsed time, refuse `tutor`/`skill`/`find` with `assessment_active` while a run is active, withhold every annotation in an active run's report, force one per-item timeout through delayed stdin, kill and resume a run and mark the resumed portion, expose a `show` during a run as prior exposure excluded from the headline and listed, refuse a comparison whose window differs while naming `window_minutes`, accept a matching self-comparison, hide annotations in the run and print them with `pass threshold: unknown` in the report, and never print a verdict. 275 assertions. |
| 19 | Portable paths | `harness/gates/portable-paths.ts` → `harness/tools/pathtypo` | A Go tool asks `go/parser` for the real string literals in every working-tree `*.go` file and reports a platform-bound one (`"/tmp/…"`, `"/Users/…"`, `"C:\\…"`, a UNC path, …) as `file:line` plus its value. Using the parser instead of a hand-written lexer is why a rune holding a backtick, a comment quoting a path, SQL inside a raw string, and a pointer assignment cannot fool it. A fixture that is genuinely pure string data carries `// portable-paths: allow <why>` **on the literal's own line** — the marker covers that line only, so one justified fixture cannot silence an unjustified path beside it. |
| 20 | Harness self-test | `harness/gates/self-test.ts` | Feeds each other gate a deliberately broken input (31 cases over 19/19 gates) and asserts that file discovery includes tracked plus untracked non-ignored files while excluding ignored ones. A gate that passes its broken input is itself a failure: it converts an unknown into a false green. |

Skipped gates print `SKIP` with a reason. A skip is only legitimate for a surface that does not exist
yet (for example: no `go.mod` before the first increment, no baseline before it is committed); a skip
that hides a real surface is a harness defect.

## What CI proves and what it does not

- CI runs the same command on Linux, macOS and Windows. Platform-specific packaging, WebView or Anki
  behaviour still needs its own runtime evidence; the matrix is not a desktop-app test.
- CI is a floor. A merged pull request additionally needs the machine review marker and a local gate
  re-run on the exact head being merged.
- Do not run the full gate chain twice on one commit and call it two proofs. The local run on the
  final tree is the evidence; CI confirms it reproduces from a clean checkout.
- Do not run the import smoke or a benchmark concurrently with the full Go test suite on a
  memory-constrained host; they are cheap, but the Go toolchain parallelises packages by default.

## Growth policy

Add a gate when, and only when, a real failure mode exists that the current set cannot see:

| New surface | Gate it should bring |
|---|---|
| JSON/CLI protocol or persisted schema | golden-file or schema test, and a migration test from the previous version |
| Local practice CLI | first-party import scan for network packages and confidential markers (`local-only.ts`) |
| Import / OCR | fixture-based test with a deliberately damaged input, plus an explicit "unverified" state assertion (`import-smoke.ts` and Go tests) |
| Scoring tactic / assisted evidence | guided/independent smoke with key hiding, stage gating and an assisted-vs-independent report split (`tactic-smoke.ts`) |
| Adaptive selection / skill evidence | recommendation smoke separating recorded evidence from inference, with passage re-exposure, hints, revisits and reopen stability (`adaptive-smoke.ts`) |
| Private source baseline | provenance/manifest integrity and test/fixture isolation (`baseline.ts`) |
| Provider call | recorded-fixture test with the network disabled; a failure path that cannot corrupt attempts (`tutor-smoke.ts`, plus Go prompt/consent/store tests) |
| Durable agent / Pi runtime | real host restart, pause/resume, cancellation and reconciliation of a retried effect (`agent-smoke.ts`, plus Go packet/store/CLI tests) |
| Anki integration | fake AnkiConnect server test (idempotency, conflict, offline queue) — never a real deck |
| Desktop shell | a smoke launch that fails on a blank window; screenshots alone are not evidence |

Rules for the harness itself:

- A check that always passes is worse than no check: it converts an unknown into a false green.
  Delete it, or add it to `harness/gates/self-test.ts` with an input that must fail.
- A check that depends on a network call belongs in CI only if the call is deterministic; otherwise
  record the fixture.
- Keep gate output short and numeric: counts, exit codes, paths.
- New checks are TypeScript gate modules sharing `harness/lib/`; never another `scripts/*.sh` or
  `scripts/*.py` one-off. Duplicate logic between gates is a harness defect.

## Environment

- Toolchain is pinned by `mise.toml` (Go, Node, Bun). A contributor without mise needs matching Go,
  Node and Bun on `PATH`; `python3` and a POSIX shell are not required. Node is used by the product's
  tutor and search hosts and by `tutor-smoke`; the rest of the CLI and all local practice tests stay
  Go-only.
- `tutor-smoke`, `find-smoke` and `agent-smoke` start loopback stubs in the gate process. They make no
  external network call and use a generated credential that exists only in the gate's memory.
- `agent-smoke` also runs the real Node + Pi Durable runtime against a temporary Pi SQLite file. It
  needs the workspace dependencies installed; a missing `node_modules` fails the gate rather than
  silently skipping the runtime.
- `bun install` installs the two harness dev dependencies (TypeScript and Bun types) used by the
  typecheck gate plus the pinned Pi Durable workspace under `agent/`; `bun.lock` is committed so the
  install is reproducible. CI runs the same install before the gate.
- Private material lives under `data/private/`, `data/local/`, `local-data/`, or `.passquest/`
  (gitignored). Nothing there may be copied into `testdata/`.
