# Agent hosts

`agent/` holds the only Node/TypeScript product runtime: the provider egress for the BYOK tutor, the
loopback AnkiConnect transport and the Pi Durable host for bounded study-brief work. The Go core never
imports a network package; every host is spawned as a child process over a versioned JSON boundary.

Install the pinned runtime dependencies from the repository root (they are a Bun workspace):

```bash
bun install
```

## `tutor-host.mjs` — the single-call tutor path

`tutor-host.mjs` exists for one reason: the Go application core must not import a network client,
while a BYOK text provider requires HTTPS. It is a single-shot process, not an agent loop.

- One JSON request on stdin, one JSON result on stdout, protocol version `1`.
- `chat` is the typed request content; `body` is its exact serialization. The host rejects a request
  whose `body` does not match `chat`, then posts `body` unchanged so the CLI's recorded SHA-256 and
  byte size identify the bytes that left the machine. Message and response bounds count code points.
- The result carries `request_sent`: `true` only after the provider request was invoked. A failure
  before that point (configuration, missing credential) must not be recorded as having shared private
  text.
- The credential is read only from `PASSQUEST_TUTOR_API_KEY`. It is placed in the
  `Authorization: Bearer …` header and nowhere else. The process logs nothing.
- Redirects are refused; `https` is required except for loopback endpoints.
- Responses are capped at 1 MiB and the returned text at the request bound. A provider failure is a
  structured `status: "error"` document with a stable code; the host does not throw or retry.
- The host has no database, filesystem or tool access. It cannot change an attempt, grade or answer
  key.

`docs/tutor.md` documents the CLI, the consent model and the exact request fields.

## `anki-host.mjs` — the loopback AnkiConnect path

`anki-host.mjs` exists for the same boundary reason: the Go core must not import a network client,
while AnkiConnect is an HTTP endpoint. It is a bounded transport, not an Anki client.

- One JSON request on stdin with an endpoint, a timeout and 1–50 allow-listed actions: `version`,
  `createDeck`, `modelNames`, `createModel`, `addNote`, `updateNoteFields`, `notesInfo`.
- The endpoint is validated again in the host: `http`/`https`, no userinfo/query/fragment, and a
  loopback host (`localhost`, `127.0.0.0/8`, `[::1]`). A remote endpoint is a failed document.
- Each action is posted as `{action, version: 6, params}`. The host returns Anki's raw result and
  error string per action; the Go CLI classifies duplicates, missing notes and conflicts.
- Redirects are refused, responses are capped at 1 MiB, the timeout is bounded, and the process logs
  nothing. It has no database, filesystem or tool access and no credential.
- `docs/notes.md` documents the CLI, consent gating, identity, export and push semantics.

## `durable-host.mjs` — the Pi Durable study brief

`durable-host.mjs` is the goal-0006 runtime for one bounded multi-step task. It uses
`@earendil-works/pi-durable` 1.5.1, `@earendil-works/pi-ai` 1.5.1, `@earendil-works/chord` 1.5.1 and
`@earendil-works/pi-schema` 1.5.1, opened over a separate SQLite file (`openNodeSqliteStorage`). The
application database is never opened by this process.

Modes: `start`, `resume`, `status`, `transcript`.

- One conversation per run. A session-scoped document maps `run_id` → `conversation_id`; the
  conversation is configured with the run's model in the same creating commit. The input is admitted
  with `requestId = run_id`, so a retry after a crash finds the existing submission.
- The task is a `study-brief` built by Go from recorded attempt facts, adaptive skill state and the
  authored lesson. The packet contains no private source text, answer key, content id or question id.
- The only tool is `record_study_step`. It validates the step bounds, then invokes
  `passquest agent record-step` with the application database path. It has no generic shell. The
  provider credential is removed from the child environment. The tool is `replay: "safe"`: a process
  death after the application commit reruns the same call, and the application returns the existing
  step with `duplicate: true`.
- The host writes `<agent-store>.host` every second (`0600`, pid, run id, random token) and removes
  it on clean exit. Go uses that file for liveness and cancellation.
- On SIGTERM the host closes the Harness, leaves work pending and returns `interrupted`; on SIGINT it
  aborts the conversation durably and returns `cancelled`. Both are bounded by a 3-second close. A
  forced exit is safe because SQLite commits are atomic and the task remains resumable.
- A missing credential is reported as `needs_action` before the store is opened. A missing Node,
  missing host or missing module is `agent_unavailable`.
- `status` and `transcript` open the store `readOnly`; they never recover or write. `transcript`
  prints bounded entries from the Pi history, so the agent record remains readable without the
  application database.
- `PASSQUEST_AGENT_FAULT=kill-after-step` is a test-only hook used by `agent-smoke`: after a
  successful application commit, before the tool result is stored, the host exits 70. It performs no
  write itself. The next `resume` proves the effect is idempotent.

`docs/agent.md` documents the commands, state vocabulary, exit codes, reconciliation ledger and
platform limits.

## Boundary

Both hosts are replaced by the desktop lifecycle in goal 0011. The versioned JSON documents are the
contract; provider SDKs stay inside Node and never reach `cmd/` or `internal/`.
