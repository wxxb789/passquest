# Durable agent work

Goal 0006 makes one bounded multi-step study brief run on the real
[Pi Durable](https://github.com/earendil-works/pi/blob/main/packages/durable/README.md) runtime, survive a
local process restart, report what it is doing, and commit each advisory step through an idempotent
application-owned effect. It is continuity for the tutor, not a second scoring or content authority.

Model text is **advisory**. The verified key, deterministic grade, attempt history and learner state
remain owned by the Go application. A step is never written into `attempts`, `source_questions`, a
key, a report recommendation or an export.

## Commands

```bash
passquest agent start  [--task study-brief] [--max-steps N] [--timeout-seconds N]
                       [--db PATH] [--agent-store PATH] [--json]
passquest agent status [RUN] [--db PATH] [--agent-store PATH] [--json]
passquest agent resume [RUN] [--timeout-seconds N] [--db PATH] [--agent-store PATH] [--json]
passquest agent cancel [RUN] [--wait-seconds N] [--db PATH] [--agent-store PATH] [--json]
passquest agent runs   [--db PATH] [--json]
passquest agent show   [RUN] [--db PATH] [--json]
passquest agent transcript RUN [--agent-store PATH] [--db PATH] [--json]
```

`agent record-step` is a host-internal command and is not part of the learner surface. It reads one
step JSON document on stdin and writes one JSON document on stdout.

`--max-steps` is 1–6 (default 3); `--timeout-seconds` is 1–900 (default 120); `--wait-seconds` is
1–60 (default 10). `--agent-store` defaults to `$PASSQUEST_AGENT_STORE` or
`<db dir>/<db base>-agent.sqlite`; the Pi SQLite file is separate from the application database and is
recorded in the run, so `status`, `resume` and `cancel` find it without repeating the flag. Every
command that accepts `--agent-store` refuses a path equal to the application database, and `resume`
and `cancel` refuse an override that differs from the recorded store (`store_mismatch`): a run
cannot silently switch Pi stores or point the host at the application database.

## The bounded task

`study-brief` is a real multi-step teaching task:

1. Go builds a deterministic evidence packet from recorded attempts and the adaptive policy: attempt
   ids, status, assistance, elapsed, attempt number, repeat flag, skill snapshots, every skill's
   state/action/counts, the selected recommendation and the authored lesson for the focus skill.
2. The Pi agent receives that packet as a user task and a static system section. It must call
   `record_study_step` for each step, starting at index 1, with kind `diagnosis`, `tactic` or
   `practice`, title ≤ 100 and body ≤ 600 characters, then answer in ≤ 180 words.
3. Each tool call invokes `passquest agent record-step` as a child of the Pi host. The host itself
   never opens the application database. The step is inserted only through the Go command, which
   validates every bound and commits it in one transaction.

The task, step count, title/body lengths, model turns (`max_steps*2+2`, 3–16), provider timeout and
per-request output tokens are fixed before the run. Provider-request count is at most
`max_turns + 1`. Content quality remains provider-dependent.

## Privacy

The packet contains no passage, stem, option, evidence, reasoning, title, path, content id, question
id, answer key or learner identity. Imported source text is not read into the packet at all. The Pi
store holds the prompt, attempt facts, authored lesson text and model output; it is `0600` on POSIX
and next to the application database. The provider credential is read only from
`PASSQUEST_TUTOR_API_KEY` by the host, is never in the request document, and is removed from the
`record-step` child's environment.

## Runtime and provider

- The Node host is `agent/durable-host.mjs`; it imports `@earendil-works/pi-durable`,
  `@earendil-works/pi-ai`, `@earendil-works/chord` and `@earendil-works/pi-schema`, all pinned exactly
  in `agent/package.json`. `bun install` at the repository root installs the workspace.
- The provider is the goal-0005 `tutor_config`: one OpenAI-compatible endpoint, model, timeout and
  output bound. The endpoint must end in `/chat/completions`; `https` is required except on loopback
  (`localhost`, `127.0.0.1` or `[::1]`, which the Go validator and the Node host both accept).
- A run is bound to the model recorded at `start`. `resume` uses that model with the current endpoint,
  timeout and credential. Changing the model name needs `cancel` + `start`; a changed endpoint or fixed
  credential is picked up by `resume`.
- There is one run at a time per application database. A second `start` while any run is unfinished is
  refused with `agent_busy`, because two Pi hosts writing one store is unsupported.
- One conversation per run. A session-scoped Pi document maps `run_id` → `conversation_id` and is
  written in the same commit that creates the conversation. The task is admitted with
  `requestId = run_id`, so a retry after a crash finds the existing submission.

Agent commands need Node ≥ 22.19. `practice`, `report`, `next`, `tutor`, `show`, `runs` and every
local practice path keep working with Go alone. `status` without Node reports the application event
trail and marks `durable.available: false`; `transcript` needs the host because it reads the Pi store.

## Status

| State | Meaning |
|---|---|
| `running` | A host process holds a fresh heartbeat (≤ 5 s) for this run and the latest event is a live claim (`started`/`resumed`) or an unacknowledged `cancel_requested`. |
| `paused` | Durable work (a live run, pending tasks or an unsettled submission) exists and no host is running; or an interrupted/started run has not completed. |
| `completed` | The run answered and at least one step is committed. The host snapshot can supply the answer when the CLI died before recording completion; otherwise the recorded `completed` event does. A zero-step answer is `failed`. |
| `failed` | A non-actionable failure: malformed response, host protocol/storage error, zero committed steps, turn limit, faulted/orphaned/restart-limited task. |
| `needs_action` | A user-actionable failure: missing credential, 401/403, 429, model/config 4xx, timeout, network error, 5xx, missing host, or a run interrupted before its answer. |
| `cancelling` | `cancel_requested` is the latest event and the heartbeat is still fresh. |
| `cancelled` | `cancelled` is the latest event, or cancellation was requested and the heartbeat is stale. `resume` refuses. |

`agent runs` lists the event-trail state; `agent status` adds the live heartbeat and a read-only Pi
snapshot. `agent show` is Go-only: it reports `trail_state` (the state implied by the recorded event
trail) and `latest_event`, and does not read the heartbeat or the Pi store. It can therefore disagree
with `status` in a crash window — for example the host answered but the CLI died before appending
`completed`: `status` reports `completed` from the snapshot, while `show` still reports the
`interrupted` trail. A failure document carries the run, status, action hint and any steps already
committed.

Two different `interrupted` values are deliberately distinct: the application event `interrupted`
recorded on a timeout or SIGTERM is `paused` and resumable, while a Pi snapshot failure whose reason
is `interrupted` is a host failure and is `needs_action`.

## Interruption, resume and cancel

- `start`/`resume` sends a deadline to the host and enforces one locally. On timeout, SIGINT or
  SIGTERM to the CLI, the application publishes a pause control request and sends SIGTERM; the host
  closes the durable runtime and leaves work pending. The application records `interrupted`; the
  command exits 4 and prints a paused document.
- `resume` reopens the same Pi store, finds the conversation, resumes pending work, continues a
  retryable finished failure, or submits the original task if the crash happened before admission.
  It uses the same run id and keeps prior context and output. It refuses with exit 2 `run_running`
  while a host holds a fresh heartbeat for the run, or while a `started`/`resumed` claim is younger
  than the 5-second heartbeat window (its host may not have written the first heartbeat yet). The
  claim is one immediate transaction that must still see the latest event the command read, so two
  concurrent resumes cannot both start a host. Before resuming, the host aborts every other mapped
  conversation in the shared Pi store, so a cancelled run's pending provider request or tool call
  cannot recover under the new run's identity. `status` counts only the requested conversation's
  pending tasks and unsettled submissions.
- `agent cancel` appends `cancel_requested` **before** publishing `<agent-store>.host.control` and
  signalling. The host polls that control file every 200 ms and aborts the conversation durably on
  every OS, so signal delivery is only an optimisation: SIGINT on POSIX, no-op on Windows. It waits
  for the heartbeat to stop, escalates to SIGKILL if needed, and appends `cancelled`. If the
  heartbeat is still fresh after the bounded wait and kill, the run stays `cancel_requested`, the
  command exits 1 and status reports `cancelling`; it never records a terminal cancellation beside a
  live process. `resume` refuses either way. Cancelling an already cancelled run returns the
  recorded outcome without touching the shared heartbeat or control file, so a newer run on the same
  store keeps them; cleanup deletes a file only when it names the cancelled run (and, for a control
  request, the host token this command wrote).
- A fresh heartbeat belongs to the run, not to the token of the latest event. When a racing host
  wrote a different token, `cancel` still treats the run as live, signals that pid and records the
  live token in the request as provenance. But a heartbeat means `running` only while the latest
  event is a live claim or `cancel_requested`: an `interrupted`, `failed` or terminal event proves
  the recording process reaped the host, so a heartbeat it could not remove (a Windows hard kill
  cannot) never vetoes `paused`, `status` or `resume`.
- A process that dies before returning a document is treated as interrupted (exit 4), not failed, so
  resume can recover the work. A missing Node/host or missing module is `agent_unavailable` and
  `needs_action`.
- The host writes `<agent-store>.host` every second with its pid, run id and a random token, polls
  `<agent-store>.host.control`, and removes both on clean exit. A control request names the run and
  the host token, so a stale request cannot stop a later resume. Go uses the heartbeat for
  cross-platform liveness; a stale pid is never signalled.

## The side effect and reconciliation

Schema v9 adds four append-only tables:

| Table | Content |
|---|---|
| `agent_runs` | Immutable run definition: run id, task, prompt version, model, store path, step/turn bounds, created time. |
| `agent_run_events` | Append-only transition trail: `started`, `resumed`, `interrupted`, `completed`, `failed`, `needs_action`, `cancel_requested`, `cancelled`, `reconciled`. |
| `agent_steps` | Append-only advisory steps: unique `(run_id, step_index)`, kind, title, body, created time. |
| `agent_step_attempts` | One row per tool invocation: Pi call id, step key, `committed`/`duplicate`, time. |

The idempotency key is `run_id + ":" + step_index`. `record-step` runs one immediate transaction: it
rejects an unknown, cancelled or completed run, returns the existing step and appends a `duplicate` attempt plus
one `reconciled` event when the index is occupied, otherwise inserts the step and a `committed`
attempt. The step text is never overwritten. The Pi tool is declared `replay: "safe"`, so a process
death after the application commit but before the tool result is stored reruns the same call; the
application keeps one step row.

`completed` and `cancelled` are absorbing: a late process cannot append a transition that rewrites
the outcome. `agent show` prints the event trail, the steps and the effect ledger (`steps`, `attempts`,
`committed`, `duplicates`, `reconciled_events`). The test-only
`PASSQUEST_AGENT_FAULT=kill-after-step` hook exits the host with code 70 immediately after a successful
step commit and performs no write; `agent-smoke` uses it to prove 2 steps, 3 attempts and one duplicate.

## Exit codes

| Exit | Meaning |
|---|---|
| 0 | Completed, cancelled as requested, or a successful status/show/runs/transcript. |
| 1 | Local storage/internal failure, including an unconfirmed cancellation. |
| 2 | Invalid input, no provider, no/ambiguous/unsuitable run, resume refused, rejected step. |
| 3 | Host, provider or run failure after a run record exists; codes: `agent_unavailable`, `missing_credential`, `auth_failed`, `rate_limited`, `provider_error`, `timeout`, `network_error`, `invalid_response`, `config_error`, `host_error`, `storage_error`, `no_steps`, `turn_limit`, `host_protocol`, `task_faulted`, `orphaned`, `restart_limit`, `interrupted`. |
| 4 | Work is paused/interrupted and resumable. |

JSON documents use `schema_version: 1`. Failures keep the `{error: {code, message}}` shape on stderr
and add the run, status, action and any committed steps. `status` exits 0 for any known state; the
document carries the state.

## Limits

- One provider shape and one task kind. Completion quality is model-dependent; the application can
  verify mechanics and bounds, not pedagogical quality.
- One run at a time; a paused run must be resumed or cancelled before a new brief can start.
- `resume` after a deleted Pi store re-executes the task; existing step indices dedupe, so a new step
  at an occupied index keeps the old text.
- `transcript` reads the Pi store directly and does not need the application database; `status`,
  `resume` and `cancel` need the application run record.
- Windows has no POSIX signals/process groups. Pause and cancel use the durable control file the
  host polls; signal delivery is only an optimisation. The application trail decides liveness, so a
  hard-killed host's leftover heartbeat cannot block `status` or `resume`.
- Agent runs do not consume the `tutor` daily-call ledger; the run's turn bound and timeout are their
  cost cap. This is revisited with goal 0010.
- No streaming UI, background service, OS secret store, Anki note, external resource search,
  readiness estimate, exam blueprint or pass probability.

Measured behaviour and limitations: [`evidence/0006-resumable-agent-work.md`](evidence/0006-resumable-agent-work.md).
