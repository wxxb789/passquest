# 0006 — Recover useful agent work after interruption: increment plan

Contract: [`../goals/archive/0006-resumable-agent-work.md`](../goals/archive/0006-resumable-agent-work.md)
(archived unchanged at close). Shape reference: goal 0005's plan, `agent/tutor-host.mjs`,
`internal/store/tutor.go`, `internal/cli/tutor.go`, the `tutor-smoke` gate and its evidence record.
Conventions: [`../conventions.md`](../conventions.md). Harness: [`../harness.md`](../harness.md).

## Goal in one sentence

Let one bounded multi-step study brief run on the real Pi Durable runtime, be interrupted and resumed
across a local process restart, be cancelled visibly, and commit each advisory step through an
idempotent application-owned effect so a crash after the commit cannot duplicate content.

## Public shape

```text
passquest agent start  [--task study-brief] [--max-steps N] [--timeout-seconds N]
                       [--db PATH] [--agent-store PATH] [--json]
passquest agent status [RUN] [--db PATH] [--agent-store PATH] [--json]
passquest agent resume [RUN] [--timeout-seconds N] [--db PATH] [--agent-store PATH] [--json]
passquest agent cancel [RUN] [--wait-seconds N] [--db PATH] [--agent-store PATH] [--json]
passquest agent runs   [--db PATH] [--json]
passquest agent show   RUN [--db PATH] [--json]
passquest agent transcript RUN [--db PATH] [--agent-store PATH] [--json]
passquest agent record-step [--db PATH] [--json]      # host-internal, JSON on stdin
```

- `start` creates one run, refuses when another run in this database is unfinished, submits a
  `study-brief` task to the Pi host and waits for the bounded result. `--max-steps` is 1–6 (default
  3); `--timeout-seconds` is 1–900 (default 120).
- `status` reports the five contract states plus `cancelled` and the transient `cancelling`. It reads
  the application event trail, the host heartbeat and a read-only Pi snapshot; it never mutates.
- `resume` continues the same run and the same Pi conversation. It refuses `completed`, `cancelled`
  and the app-level `no_steps` failure.
- `cancel` appends `cancel_requested` **before** signalling, aborts the host on POSIX, kills it on
  Windows, confirms the heartbeat stopped, then appends `cancelled`. `resume` refuses afterwards.
- `runs` and `show` are Go-only; `show` carries the run's event trail, committed steps and
  committed/duplicate effect attempts. `transcript` opens only the Pi store and prints the durable
  agent history, so it works after the application database is gone.
- Defaults: run = latest unfinished run (or latest run for `status`). `--agent-store` defaults to
  `$PASSQUEST_AGENT_STORE` or `<db dir>/<db base>-agent.sqlite`, so the Pi SQLite file is separate
  from the application SQLite file. The provider is the goal-0005 `tutor_config`; the endpoint must
  end in `/chat/completions`, because pi-ai appends that path. The credential is still only
  `PASSQUEST_TUTOR_API_KEY`.

Exit codes:

| Exit | Meaning |
|---|---|
| 0 | Completed; requested cancellation; status/show/runs/transcript success. |
| 2 | Invalid input, no provider configured, no/ambiguous/unsuitable run, resume refused, rejected step. |
| 3 | Agent host, provider, or run failure after a run record exists. Codes: `agent_unavailable`, `missing_credential`, `auth_failed`, `rate_limited`, `provider_error`, `timeout`, `network_error`, `invalid_response`, `config_error`, `host_error`, `storage_error`, `no_steps`, `turn_limit`, `host_protocol`. |
| 4 | Work is paused/interrupted and resumable. |
| 1 | Local storage or internal failure, including a cancellation that could not be confirmed. |

JSON is `schema_version: 1`. `start`/`resume` success and failure documents both carry `run` and
`status`; failures keep the existing `{error: {code, message}}` shape on stderr. `status`, `runs`,
`show` and `transcript` documents are advisory-labelled where they carry model text.

## The bounded task

`study-brief` is a real multi-step teaching task, not a wrapper around one tutor call:

1. The Go core builds a deterministic evidence packet from recorded attempts and the adaptive policy:
   attempt ids, status, assistance, elapsed, attempt number, repeat flag, skill snapshots; every
   skill's state/action/counts; the selected recommendation; and the authored lesson for the focus
   skill. It contains **no** passage, stem, option, evidence, title, path, content id, question id or
   answer-key text.
2. The Pi agent receives a static system section plus that packet as its task. It must call
   `record_study_step` for each step, starting at index 1, with kind `diagnosis`, `tactic` or
   `practice`, title ≤ 100 and body ≤ 600 characters, then answer with a ≤ 180-word summary. At most
   `--max-steps` steps and at most `max_steps*2+2` model turns are allowed.
3. Each tool call runs `passquest agent record-step` as a child of the Pi host. The Go command
   validates the step and inserts it; the host itself never opens the application database.

The task is bounded because the packet, step count, field lengths, turn count, provider timeout and
output tokens have fixed limits. Content quality remains provider-dependent; the app labels every
step advisory and never writes it into an attempt, answer key or grade.

## Durable runtime

- `agent/durable-host.mjs` is a new Node host on `@earendil-works/pi-durable` 1.5.1,
  `@earendil-works/pi-ai` 1.5.1 and `@earendil-works/chord` 1.5.1, pinned exactly and installed as a
  Bun workspace (`agent/package.json`, one committed `bun.lock`). It opens
  `openNodeSqliteStorage(store)`, one SQLite file owned by the Pi runtime, separate from the
  application database.
- One conversation per run. A session-scoped Pi document maps `run_id` → `conversation_id` and is
  written in the same commit that creates the conversation; the conversation is configured with the
  run's model in that commit. The task is submitted with `requestId = run_id`, so a retry after a
  crash finds the existing submission rather than admitting a second one.
- `start` and `resume` share one host path. `resume` finds the mapping, reads `pi.live`, and either
  lets the pending run continue (`harness.resume()`), submits a Pi `continue` after a retryable
  finished failure, or submits the original input when the crash happened before admission. Then it
  waits for idle and reads the committed live state and transcript.
- The host writes a 1-second heartbeat file (`<agent-store>.host`, `0600`, pid + run + random token)
  and removes it on clean exit. Go uses it for cross-platform liveness; the token prevents a reused
  pid from being signalled.
- POSIX interruption: `start`/`resume` timeout, SIGINT or SIGTERM to the CLI sends SIGTERM to the
  host, which closes the Harness (work stays pending), returns `interrupted` and exits; Go records
  `interrupted` and exits 4. `agent cancel` sends SIGINT, which makes the host abort the conversation
  durably before closing. Windows has no POSIX signal or process group; the host is killed, the
  application `cancel_requested`/`cancelled` trail is authoritative, and `resume` refuses.
- The host's own deadline timer is a second line: it closes and returns `interrupted`, or exits 75
  after a 3-second close bound. SQLite commits are atomic, so a forced exit leaves a resumable task.

## Side effect, idempotency and reconciliation

Schema v9 adds four append-only tables. None can touch `attempts`, `source_questions` or a key.

- `agent_runs` — immutable run definition: `run_id`, task, prompt version, recorded model,
  `max_steps`, `max_turns`, `created_at`. A transaction refuses a second unfinished run.
- `agent_run_events` — append-only `{run_id, kind, detail_json, at}` trail: `started`, `resumed`,
  `interrupted`, `completed`, `failed`, `needs_action`, `cancel_requested`, `cancelled`,
  `reconciled`.
- `agent_steps` — append-only advisory content: `step_key`, `run_id`, `step_index`, kind, title, body,
  `advisory=1`, `created_at`; unique `(run_id, step_index)`.
- `agent_step_attempts` — one row per tool invocation: `attempt_key` (the Pi call id), `step_key`,
  `outcome` (`committed`/`duplicate`), `at`. This is the reconciliation ledger.

The step idempotency key is `run_id + ":" + step_index`; the Pi tool call id identifies one
invocation. `record-step` runs one immediate transaction: it rejects a cancelled/cancel-requested or
unknown run, returns the existing step and appends a `duplicate` attempt if `(run_id, step_index)`
exists, otherwise inserts the step and a `committed` attempt. A duplicate also appends a
`reconciled` run event. The tool is declared `replay: "safe"` in Pi, so a process death after the Go
commit but before Pi stores the tool result reruns the same call; the second run returns the existing
row. The test-only `PASSQUEST_AGENT_FAULT=kill-after-step` hook performs no write and exits 70 at
exactly that boundary; `agent-smoke` uses it to prove 2 steps, 3 attempts, 1 duplicate and one row per
step, with the attempts table unchanged.

## Status semantics

| State | Evidence |
|---|---|
| `running` | The latest event is not terminal and the heartbeat token matches and is < 5 s old. |
| `paused` | Durable work exists (a live run, pending tasks or an unsettled submission) and no host is running; also a run whose host never started. |
| `completed` | The Pi submission answered, no run failure remains, at least one step is committed, and the application recorded `completed`. |
| `failed` | A non-actionable failure: malformed response (`invalid_response`), host protocol/storage error, `no_steps`, `turn_limit`, `task_faulted`, `orphaned`, `restart_limit`. |
| `needs_action` | A user-actionable failure: `agent_unavailable`, `missing_credential`, 401/403, 429, 4xx model/config error, timeout, network error, 5xx. The status carries an action hint (`check_host`, `fix_credential`, `wait_and_resume`, `check_endpoint`, `resume`). |
| `cancelling` | `cancel_requested` is the latest event and the heartbeat is still fresh. |
| `cancelled` | `cancelled` is the latest event, or a request is outstanding and the heartbeat is stale. `resume` refuses. |

`agent status` does not require Node: without the host it reports the application event trail and
marks `durable.available: false`. With the host it adds the read-only Pi snapshot. A missing provider
credential is recorded as `needs_action` before any Pi work, so `agent resume` is the repair path
after exporting the key.

## Portability and model binding

- A run is bound to the model recorded at `start`. `resume` uses that model with the current
  endpoint, timeout and credential; new runs use the current `tutor_config`. Changing the model name
  therefore needs `cancel` + `start`, which is stated in the refusal/hint text.
- The host request carries the run's model, the base URL derived by removing `/chat/completions`,
  the configured timeout and output bound, and the step/turn limits. The key is never in the request.
- Node ≥ 22.19 is required by the Pi packages; the repository's Node 22 CI image satisfies it. Agent
  commands fail with exit 3 and `agent_unavailable` when Node or the host file is absent; `practice`,
  `report`, `next`, `show`, `runs` and the rest of the core keep working with Go alone.
- The Pi store and heartbeat are `0600` on POSIX and live next to the application database; the
  default app-owned directory is already `0700` and gitignored.

## Host protocol (version 1)

One JSON request on stdin, one JSON result on stdout; logs go to stderr. Modes: `start`, `resume`,
`status`, `transcript`. Requests carry `run_id`, `task_kind`, `prompt_version`, `store_path`,
`heartbeat_path`, `host_token`, `binary_path`, `db_path`, `credential_env`, `model` and, for
start/resume, the prepared `task` prompt, `max_steps`, `max_turns`, `deadline_ms`. Results carry
`schema_version: 1`, `state` (`completed`/`interrupted`/`cancelled`/`failed`/`needs_action`/`status`/
`transcript`), `run_id`, `conversation_id`, `failure {reason, detail, code, action}`, `answer`,
`answer_chars`, `turns`, `tool_calls`, `tool_errors`, `usage`, and for status/transcript the bounded
Pi snapshot or entries. The host validates the request, refuses an unknown mode/run/task and redacts
the credential from every message. It has no generic shell tool and no application database handle.

## Evidence plan

1. **Go tests**:
   - `internal/agent`: packet determinism, private-string exclusion, clipping and total bound; endpoint
     base derivation; run id/token validation; host request/response validation; failure classification;
     status classification (all seven states); step validation; refusal rules.
   - `internal/store`: v8→v9 migration keeps attempts/tutor rows; run/event/step/attempt round trips;
     unique step and duplicate outcome; cancelled step refusal; unfinished-run refusal; append-only
     update/delete triggers; attempts count/content unchanged.
   - `internal/cli`: not configured; missing credential → `needs_action`; completed/failed/paused JSON
     with a fake runner; runs/show/status from events; cancel then resume refusal; record-step
     validation and duplicate; human writers; golden documents for start/status/show/runs.
2. **Tier 2 real transcripts** in `/tmp/passquest-0006-demo/` with the real binary, real Node, real Pi
   SQLite and a loopback OpenAI-compatible SSE stub (no external network, no real key):
   - Fresh `next`/practice state; `agent start` completes 2 steps; `status`, `show`, `runs`,
     `transcript`; `report` byte-identical before/after.
   - `--timeout-seconds 1` against a delayed stub → `paused` exit 4; `status` paused; `resume`
     completes with the same run id and a coherent transcript (task, tool results, answer).
   - `PASSQUEST_AGENT_FAULT=kill-after-step` → exit 4; `resume`; `show` proves 2 steps, 3 effect
     attempts, 1 duplicate, 1 reconciliation event; `attempts` count and rows unchanged.
   - A delayed run cancelled from a second process → exit 0 `cancelled`, heartbeat gone, no process,
     `resume` exit 2; a completed run's `cancel` exit 2.
   - 401 stub → `needs_action` exit 3; fix the stub and `resume` completes; malformed SSE → `failed`.
   - `PASSQUEST_NODE` missing → `agent start` exit 3 `agent_unavailable`, `status` still reports the
     app trail, `practice` exit 0.
   - Copy the Pi SQLite file to a second directory, point `agent transcript --agent-store` at the copy
     with an empty application database, and require the same entries.
   - Check the Pi store and heartbeat modes are `0600` on POSIX.
3. **Gates**: `bash scripts/gate.sh` real exit code and counts, including the new `agent-smoke` and its
   self-test case; record the gate output verbatim in the evidence record.
4. **Harness growth**: `harness/gates/agent-smoke.ts` (real binary + real host + Pi + loopback SSE stub
   + fault/cancel/needs-action/Node-absent assertions), a broken-CLI case in `self-test.ts`,
   registration in `gates/index.ts`, a new row and updated counts in `docs/harness.md`. No new
   `scripts/*.sh` or `scripts/*.py`.

## Trade-offs and defaults taken

- **Pi Durable, pinned, as a workspace dependency.** The contract names the intended runtime; using a
  hand-written durable loop would not exercise it. Cost: a 125 MB installed dependency tree, Node
  ≥ 22.19 for agent commands, and an experimental upstream API. Go core and local practice are
  unchanged. (Q-0006-02)
- **A study brief with advisory step rows.** It is a real multi-step teaching task whose side effect
  is visible, inspectable and bounded; an Anki or external-resource effect belongs to goals 0007/0008.
  (Q-0006-01)
- **Application-owned effect through a host subprocess.** The host has no database handle and no
  generic shell; one typed tool invokes `passquest agent record-step` with no credential in its
  environment. The application validates and commits, so verified data ownership is unchanged. Cost:
  one process spawn per step. (Q-0006-01)
- **`run_id:step_index` idempotency plus a call-id attempt ledger.** A replay keeps the first
  committed text; a later model retry with a new call id at an occupied index is also a duplicate. The
  trade is that a corrected step cannot overwrite the first one; the learner can cancel and start a
  new run. (Q-0006-01)
- **Signals plus an application cancellation event.** Pi allows one writer per storage, so a second
  process cannot safely write the Pi state; it signals the recorded host instead. The heartbeat is
  the cross-platform liveness signal. POSIX aborts durably; Windows kills and relies on the app
  trail. (Q-0006-03)
- **`needs_action` vs `failed` is a fixed classification, not a heuristic.** User-actionable provider
  and host failures are `needs_action`; malformed or non-retryable task failures are `failed`. Both
  keep the run inspectable, and `resume` is the retry path. (Q-0006-04)
- **A run keeps its recorded model.** A resume may pick up a fixed endpoint or credential, but a model
  change needs a new run; this avoids silently rewriting the model mid-task. (Q-0006-05)
- **One run at a time per application database.** A second `start` is refused while any run is
  unfinished, because two Pi hosts writing one store is unsupported. Cost: the learner must `resume`
  or `cancel` before starting another brief. (Q-0006-03)
- **Agent calls do not consume the `tutor` daily-call ledger.** A run is a bounded multi-call task; its
  cap is `max_turns+1` model requests plus the provider timeout. The single-shot tutor budget stays a
  tutor concept. This is disclosed as a limit and revisited with goal 0010. (Q-0006-04)
- **No streaming UI and no new model capability.** The CLI prints progress/status; streaming belongs
  to the desktop workspace. The host uses one OpenAI-compatible request shape already supported.

## Out of scope, with reasons

- Anki note creation, external resource search, readiness assessment, provider matrix/account login:
  goals 0007, 0008, 0009, 0010.
- A desktop window, system tray, background service lifecycle or OS secret store: goal 0011/0013.
- A second bounded task kind, multi-agent branching, forks or compaction tuning: one task is enough to
  prove durable continuity; add another only when a product need is observed.
- A Pi-owned UI/protocol beyond the versioned stdin/stdout document: the desktop bridge is goal 0011.
- Sending private source text, answer keys or learner identity to the provider: the packet is
  attempt facts, skill state and authored lesson text only.
- A pass probability, exam blueprint, threshold or time limit: still unknown and forbidden.
- A custom retry scheduler, background daemon or automatic resume at boot: explicit `resume` is
  visible and reversible; automatic execution would create the invisible work the contract forbids.
- Committing real provider output, credentials, the Pi SQLite file or private learner data: the demo
  uses authored/synthetic text and a fake key.

## Completion checklist

- [x] Plan written to `docs/plans/` (this file).
- [x] `agent/package.json`, workspace install and `bun.lock`; `agent/durable-host.mjs` on Pi Durable.
- [x] `internal/agent` packet, host protocol/transport, classification and tests.
- [x] Schema v9 store APIs, migration and tests; attempts untouched.
- [x] `agent` CLI commands, goldens, human writers and tests.
- [x] `agent-smoke` gate, self-test case, harness index and documentation.
- [x] `docs/agent.md`, README, tutor/harness notes reconciled.
- [x] Goal file archived unchanged, goals index updated, open questions appended.
- [x] Evidence record with real transcripts, gate output and limitations.
- [x] `bash scripts/gate.sh` exit 0 recorded (16 passed, 0 failed, 0 skipped); no test or contract clause weakened.
