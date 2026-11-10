# 0006 — Recover useful agent work after interruption: evidence

- Tree: working tree on top of `aef9834df6093828de87ad1c890b3763741c5475` (`feat(0005): Helpful AI
  tutor with user-supplied text model`). The operator owns the commit; this run did not commit,
  push or open a pull request.
- Date: 2026-11-10. Host: Linux 7.0.0-1017-azure x64.
- Toolchain: Go 1.27.3; Node v24.16.0 (the host requires ≥ 22.19); Bun 1.4.4; Pi Durable /
  pi-ai / chord 1.5.1 (pinned in `agent/package.json`).
- Contract: [`../goals/archive/0006-resumable-agent-work.md`](../goals/archive/0006-resumable-agent-work.md).
  Plan: [`../plans/2026-11-10-1748-0006-resumable-agent-work.md`](../plans/2026-11-10-1748-0006-resumable-agent-work.md).
  Design: [`../agent.md`](../agent.md).

The demonstration used `/tmp/passquest-0006-demo/`, a locally built binary, the real
`agent/durable-host.mjs` on the real Pi Durable runtime, and a loopback OpenAI-compatible SSE stub.
The stub made no external network call and the provider credential was the synthetic value
`test-key-not-real`. Nothing from the demonstration was added to the repository; the Pi SQLite files,
application databases and stub output stayed under `/tmp`.

## What was built

| Package / area | Truth it owns |
|---|---|
| `internal/agent` | The deterministic study-brief packet (recorded facts, derived skill state, authored lesson, no private source text), the versioned host request/response and transport, endpoint/step/run validation, failure classification and the status state machine. No database, no network import. |
| `internal/store` | Schema v9: immutable `agent_runs`, append-only `agent_run_events`, append-only advisory `agent_steps`, and the append-only `agent_step_attempts` effect ledger. The step key is idempotent; a duplicate keeps the first text and appends a `reconciled` event. No agent table can update `attempts`. |
| `internal/cli` | `agent start/status/resume/cancel/runs/show/transcript/record-step`, run selection, provider/credential checks, signal/timeout pause, cancellation confirmation, the paired JSON/human documents and exit codes. |
| `agent/durable-host.mjs` | The Pi Durable runtime: one conversation per run, a session mapping, a replay-safe `record_study_step` tool that invokes the application's validated command, read-only status/transcript, heartbeat, SIGTERM pause and SIGINT cancel. |
| `harness/gates/agent-smoke.ts` | Real binary + real host + real Pi SQLite + loopback SSE stub; 98 assertions over 8 scenarios. |

## Demonstration setup

```bash
cd /tmp/passquest-0006-demo
go build -o passquest ./cmd/passquest
node stub.mjs &   # 127.0.0.1:18091, OpenAI-compatible SSE
export PASSQUEST_TUTOR_API_KEY=test-key-not-real
export PASSQUEST_AGENT_HOST=/home/azureuser/repos/passquest/agent/durable-host.mjs
passquest tutor config --endpoint http://127.0.0.1:18091/v1/chat/completions --model demo-model \
  --max-output-tokens 600 --db <db>
```

The stub returns a `record_study_step` tool call for index 1, then index 2, then a final answer when
both tool results are present. It can be switched to delay, 401, malformed SSE and plain text to
exercise failure states. The application binary, not the host, commits every step.

## Scenario results

### Completed run, status, evidence and independent history

| Step | Observed |
|---|---|
| `agent start --db fresh/app.sqlite --agent-store fresh/agent.sqlite --timeout-seconds 60 --json` | exit 0; state `completed`; 2 steps; Pi conversation 3; final answer 128 characters |
| `report --json` before and after | byte-identical (`cmp` exit 0) |
| `agent status RUN --json` | `completed`, latest event `completed`, 2 steps; snapshot `live_run:false`, `pending_tasks:0`, `unsettled_submissions:0`, `answer_chars:128`, `entries:7`, `turns:3`, `tool_calls:2` |
| `agent show RUN --json` | effects `{steps:2, attempts:2, committed:2, duplicates:0, reconciled_events:0}`; events `started,completed` |
| `agent transcript RUN --agent-store fresh/agent.sqlite --json` | found; 7 entries `app.agent-task, pi.system, pi.assistant, pi.tool-result, pi.assistant, pi.tool-result, pi.assistant`; the final answer is present |
| `agent transcript RUN --agent-store fresh/agent.sqlite --db missing/app.sqlite --json` | same 7 entries copied from the Pi file; the application database was not created |
| store file | `-rw-------` (0600), 135168 bytes |

### Interruption and resume

The stub was set to delay; `agent start --timeout-seconds 1` exceeded the local deadline.

| Step | Observed |
|---|---|
| start | exit 4; state `paused`, reason `timeout`, 0 steps |
| `agent status RUN --json` | `paused`, latest event `interrupted`; snapshot `live_run:true`, `pending_tasks:1`, `unsettled_submissions:1` |
| a second `agent start` | exit 2, `agent_busy` |
| `agent resume RUN --timeout-seconds 60` after the stub was restored | exit 0; state `completed`; the same run id; 2 steps |
| `agent transcript RUN` | 7 entries with the original task, two tool results and the answer |

### Side effect interrupted after the commit

`PASSQUEST_AGENT_FAULT=kill-after-step` exits the host with code 70 after the application commit and
before the tool result is stored.

| Step | Observed |
|---|---|
| start | exit 4; state `paused`; 1 step; reason `host_exit` |
| `agent show` before resume | effects `{steps:1, attempts:1, committed:1, duplicates:0, reconciled_events:0}`; events `started,interrupted` |
| `agent resume` | exit 0; state `completed`; 2 steps |
| `agent show` after resume | effects `{steps:2, attempts:3, committed:2, duplicates:1, reconciled_events:1}`; events `started,interrupted,resumed,reconciled,completed` |
| steps | index 1 `diagnosis` "Review the missed polarity cue"; index 2 `tactic` "Mark NOT and EXCEPT first"; the first step text is unchanged |
| reconciliation event | `{"attempt_key":"call_1","step_index":1}` |
| `report --json` before and after | byte-identical; no attempt was added or changed |

The retry ran the same Pi call id; the application returned the existing step and recorded a
`duplicate` attempt. This is the required interruption/retry proof for a side-effecting operation.

### Running state and cancellation

| Step | Observed |
|---|---|
| delayed run, polled from a second process | `agent status` reported `running`, `heartbeat_fresh:true`, a host pid, and the heartbeat file was `0600` |
| `agent cancel RUN --json` | exit 0; `cancelled`, `confirmed:true`, host pid 3700139 |
| the original `agent start` | exit 0; state `cancelled` |
| `agent status RUN` | `cancelled` |
| `agent resume RUN` | exit 2, `run_cancelled` |
| heartbeat | removed |

### Needs action and terminal failure

| Scenario | Observed |
|---|---|
| provider returns 401 | start exit 3; state `needs_action`; code `auth_failed`; action `fix_credential`; status `needs_action` with the 401 message |
| stub fixed, `agent resume RUN` | exit 0; state `completed`; 2 steps |
| provider returns malformed SSE | start exit 3; state `failed`; code `invalid_response`; status `failed` |
| `PASSQUEST_NODE` missing | start exit 3; state `needs_action`; code `agent_unavailable` |
| `agent status` with Node missing | exit 0; `needs_action`; `durable.available:false`, reason `agent_unavailable` |
| `practice --answer A` with Node missing | exit 0; attempt recorded and graded (`answered`, `q1`, `incorrect`) |

## Gate output

The final run of `bash scripts/gate.sh` on this tree:

```text
$ bun run harness/gate.ts

== typecheck
PASS  typecheck (0 type errors)

== gofmt
PASS  gofmt (53 files)

== go vet
PASS  go vet

== go build
PASS  go build

== go test
PASS  go test (11 packages, 485 tests, 0 skipped)

== go mod tidy
PASS  go mod tidy

== docs links
PASS  docs links (102 checked, 0 broken)

== secrets/sources
PASS  secrets/sources (350 files, 0 findings)

== local-only CLI
PASS  local-only CLI (11 first-party packages, 0 network imports, 0 confidential markers)

== baseline integrity
PASS  baseline integrity (165 pages (22 native, 143 OCR), 0 fixture references)

== import smoke
PASS  import smoke (35 assertions)

== tactic smoke
PASS  tactic smoke (70 assertions)

== adaptive smoke
PASS  adaptive smoke (120 assertions)

== tutor smoke
PASS  tutor smoke (156 assertions)

== agent smoke
PASS  agent smoke (98 assertions, 8 scenarios, 17 provider requests, 2 permission checks)

== harness self-test
PASS  harness self-test (22 cases, 15/15 gates)

16 passed, 0 failed, 0 skipped
GATE: PASS
```

`agent-smoke` itself covers the fresh completion, status/show/runs/transcript, timeout pause and
resume, fault injection with reconciliation, cancellation, 401 handling, malformed stream handling,
a zero-step answer, Node absence, the attempt-report invariant, the copied-store transcript, the
`0600` store/heartbeat modes and a credential scan of the application and Pi stores. It runs after
the Go suite, not concurrently with it.

## Contract clause → evidence

| Done-when clause | Evidence |
|---|---|
| A real, nontrivial AI task can be started, interrupted and resumed from persisted state; prior context and observable output remain coherent. | `study-brief` uses two model/tool rounds and a final answer. The 1-second timeout left `live_run:true`/`pending_tasks:1`; resume completed the same run id; the transcript has the original task, both tool results and the answer. Gate scenario 2. |
| The user can tell if work is running, paused, completed, failed or needs action, and can cancel without invisible execution. | `running` (fresh heartbeat, pid), `paused` (timeout), `completed`, `failed` (malformed SSE) and `needs_action` (401, missing Node) were observed. `cancel` recorded `confirmed:true`, the start process exited `cancelled`, `resume` refused, and the heartbeat file was removed. Gate scenarios 4–6. |
| At least one side-effecting operation is interrupted/retried without duplicate committed learner evidence or duplicate content, with reconciliation evidence. | The fault run left 1 step/1 attempt; resume produced 2 steps/3 attempts/1 duplicate/1 reconciled event, kept the first text, and `report --json` was byte-identical. Gate scenario 3, plus `TestAgentStepEffectIsIdempotentAndLeavesAttemptsAlone`. |
| Verified scoring/history remain application-owned; durable history is independently recoverable; core practice works without the agent host. | The agent never writes `attempts`; the report is unchanged across runs. A copied Pi SQLite file printed the same 7 transcript entries with a missing application database. With Node absent, `agent start` exited 3/`agent_unavailable` and `practice` still recorded and graded. Gate scenario 7. |

## Limitations and residual risks

- The provider was a loopback scripted stub. The evidence verifies mechanics, bounds, state
  transitions and truth ownership, not the pedagogical quality or latency of a real model. A real
  provider is not committed and no real credential was used.
- Pi Durable 1.5.1 is marked experimental upstream and is pinned exactly. An incompatible upstream
  release needs a deliberate upgrade; the application run/effect tables and the host document are
  the stable boundary.
- Agent commands require Node ≥ 22.19 and the workspace dependencies. The Go core and all local
  practice paths do not. A missing dependency is reported as `agent_unavailable`/`needs_action`, not
  silently skipped.
- One run at a time per application database. A paused run must be resumed or cancelled before a new
  brief starts; two Pi writers on one store are unsupported.
- A run keeps its recorded model. A changed model name needs `cancel` + `start`; a changed endpoint,
  timeout or credential is picked up by `resume`.
- Windows has no POSIX signal/process-group behaviour: cancellation kills the host and relies on the
  application `cancel_requested`/`cancelled` trail. The durable conversation may remain pending in the
  Pi store, but `resume` refuses. The smoke gate reports permissions as skipped on Windows.
- The heartbeat is considered stale after 5 seconds. A host that hangs without refreshing it is
  reported as paused and can be cancelled; a stale pid is never signalled because the token must match.
- Agent runs do not consume the `tutor` daily-call ledger. The run is bounded by `max_turns+1`
  provider requests and the configured timeout; this is disclosed and revisited with goal 0010.
- `transcript` clips each entry to 4,000 characters and at most 200 entries. It is a local recovery
  view, not an export format.
- The packet intentionally excludes passage, stem, option, evidence, reasoning, content id, question
  id, source title/path, answer key and learner identity. The Pi store still contains the advisory
  prompt and model output; it is private local data and is `0600` on POSIX.
- No streaming UI, desktop lifecycle, OS secret store, Anki note, external search, readiness estimate,
  exam blueprint or pass probability is claimed.
