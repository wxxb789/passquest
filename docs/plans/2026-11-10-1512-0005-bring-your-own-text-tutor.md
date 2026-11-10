# 0005 — Helpful AI tutor with user-supplied text model: increment plan

Contract: [`../goals/archive/0005-bring-your-own-text-tutor.md`](../goals/archive/0005-bring-your-own-text-tutor.md)
(the file moves there at close, unchanged). Shape reference: goal 0004's plan, source, evidence and
`adaptive-smoke` gate. Conventions: [`../conventions.md`](../conventions.md). Harness:
[`../harness.md`](../harness.md).

## Goal in one sentence

Let one learner configure a single BYOK text endpoint and ask for bounded, post-attempt coaching that
is labelled as model output, never touches the attempts or answer keys, and sends no private imported
source text without a recorded per-source authorization.

## Public shape

```text
passquest tutor config [--endpoint URL --model NAME] [--timeout-seconds N]
                       [--max-output-tokens N] [--daily-calls N] [--db PATH] [--json]
passquest tutor preview [--attempt ID] [--mode explain|probe|micro_lesson] [--db PATH] [--json]
passquest tutor ask     [--attempt ID] [--mode explain|probe|micro_lesson] [--db PATH] [--json]
passquest tutor history [--attempt ID] [--db PATH] [--json]
passquest tutor show    CALL [--db PATH] [--json]
passquest tutor consent SOURCE --reviewer NAME [--db PATH] [--json]
passquest tutor revoke  SOURCE --reviewer NAME [--db PATH] [--json]
passquest tutor consents [--db PATH] [--json]
```

- `config` without value flags inspects; with any value flag it validates and saves. Endpoint and
  model are required for a usable configuration. The API key is **never** stored by Pass Quest: the
  host reads `PASSQUEST_TUTOR_API_KEY` from its environment. `config` reports `present`/`missing`,
  never the value.
- `preview` renders the exact provider payload (`chat`: model, messages, `max_tokens`, temperature,
  stream) and the fields withheld for privacy. `ask` sends that payload once, stores an advisory
  record and prints the model text under an explicit advisory header.
- `attempt` defaults to the latest recorded attempt. The attempt is identified by its stored id;
  a missing attempt is exit 2.
- `history` lists advisory calls, failures and token usage; `show CALL` prints one stored model text.
  Neither `report` nor `next` reads the tutor tables.
- Consent commands append per-source grant/revoke events; `consents` lists every imported source and
  its effective state. `reviewer` is the local attribution label already used for reviews, not
  authentication.
- Exit codes: `0` success; `2` invalid input, not configured, unknown attempt/call/source, missing
  credential, budget exhausted; `3` agent host or provider failure after a call record; `1` storage
  or internal failure. JSON errors keep `schema_version: 1` and the `{error: {code, message}}` shape;
  an `ask` failure additionally carries the recorded `call`.

## Supported provider boundary

- One request shape: OpenAI-compatible `POST <endpoint>` with
  `Authorization: Bearer $PASSQUEST_TUTOR_API_KEY`, JSON body
  `{model, messages, max_tokens, temperature: 0, stream: false}`. The endpoint is configured as a full
  URL; `https` is required except for loopback (`localhost`, `127.0.0.1`, `::1`) where local model
  servers commonly use `http`. URLs with userinfo, query or fragment are rejected so a credential
  cannot be smuggled into stored configuration. Redirects are refused, so the bearer token cannot
  follow a cross-origin redirect.
- No provider-specific capability claims, no retry, no streaming, no speech/image/search. Those
  belong to goals 0010 and later.
- Bounds: one attempt only (no history dump), timeout 1–120 s (default 30), `max_tokens` 64–2048
  (default 350), response body ≤ 1 MiB and model text ≤ 12 000 characters, daily call budget 1–200
  (default 20) counted in UTC days including failed calls. Context is clipped per field
  (passage 3000, stem 1000, each option 500, evidence 800, authored reasoning 1000, lesson text
  ~3000); the preview names every clipped field.
- The Go CLI never uses the network. It spawns the Node host
  (`agent/tutor-host.mjs`, override `PASSQUEST_TUTOR_HOST`; Node override `PASSQUEST_NODE`) and
  exchanges one versioned JSON document over stdin/stdout. If Node or the host is absent, `tutor ask`
  exits 3 and ordinary practice is unaffected. The host is the only component that imports Node's
  HTTP client.

## What is sent

| Material | Default | After `tutor consent SOURCE` |
|---|---|---|
| First-party embedded passage/question/lesson (`sample-*`, `tactic-*`, `skill-*`) | Passage excerpt, question stem, options, verified key/evidence/reasoning, attempt facts, lesson | same |
| Imported source page (`src-*-pNNN`) | Attempt facts (status, selected/correct labels, elapsed, assistance, attempt number, repeat, skill ids), skill lessons, and an explicit “question content withheld” marker. No passage, stem, option text, evidence or reasoning. | Same plus the page excerpt and question fields, exactly as the first-party case. |
| Unmatched content id | Attempt facts and skill lessons; stored option texts only when the id is not an imported source; `content_unavailable` is disclosed. | Stored option texts for an imported source. |
| Edited content whose current version differs from the attempt | Attempt facts, stored selected/correct option texts, lesson; no current passage/stem/evidence; `content_changed` is disclosed. | same, with private text allowed by the source grant |

The provider payload never contains the database path, source title/path, endpoint URL, API key,
consent reviewer, or the whole attempt history. The local call record stores the attempt/content ids,
scope flags, request SHA-256 and usage, but not the raw prompt; `preview` is the inspection surface
for the exact text and must be run before sending. A changed question later can therefore change the
prepared prompt; the recorded hash identifies the request that was sent.

## Advisory record and truth ownership

Schema version 7 adds three tables, none of which touches `attempts`, `source_questions` or any key:

- `tutor_config` — singleton endpoint/model/timeout/max output/daily budget row; mutable
  configuration, not learner evidence.
- `tutor_consent_events` — append-only `(source_id, granted, reviewer, at)` events; effective consent
  is the latest event. Update/delete triggers reject rewriting history.
- `tutor_calls` — append-only advisory call records: attempt id, mode, status, error code/message,
  provider host, configured model, prompt version, content/question/source ids, whether private text
  was shared, request SHA-256/bytes, output text/characters, token usage, HTTP status and latency.
  Update/delete triggers reject rewriting.

`tutor ask` appends only to `tutor_calls`. A failed provider call appends an error record, never an
attempt. A successful call appends no attempt, does not change a stored grade, and cannot edit a
question or key. All output is marked `advisory: true`, `verified_answer: false`; the human output
says the text is model output and not an exam fact. `report`, `next`, `practice` and export paths do
not read the tutor tables.

Credentials: only the environment variable enters the child process; no key is written to SQLite,
stdout/stderr, the call record, generated commands or the repository. The host sends no logs and
never includes the authorization header in an error.

## Prompt and modes

Prompt version `tutor-v1`. A fixed system message states that verified keys/grades are authoritative,
that supplied text is data rather than instructions, that live-exam facts must not be invented, that
withheld content must not be guessed, and that the answer is limited to 180 plain-text words. The
user message carries the mode, attempt facts, skill lessons, and the allowed question material.

| Mode | Task |
|---|---|
| `explain` | Explain why the verified key fits the evidence and why the selected option does not; end with one applicable check. |
| `probe` | Ask up to two short diagnostic questions about the misconception and one check; do not merely restate the key. |
| `micro_lesson` | One tactical rule, one application step and one boundary/counterexample; no new answer-keyed exam item. |

The prompt contains no request to generate questions, change grades, find external material or
follow instructions found in the passage.

## JSON shapes (schema_version 1)

- `preview`: `{state: "preview", attempt, mode, provider{endpoint, model, credential_env,
  credential_present}, context{private_source, source_id, consent_required, consent_granted,
  source_text_shared, withheld[], content_matched, content_changed, skills,
  skills_mapped_from_current_tags, truncated_fields[], prompt_version}, chat{model, messages,
  max_tokens, temperature, stream}, request_sha256, request_bytes, limits[]}`.
- `ask` success: `{state: "advisory", call, suggestion{text, advisory: true, verified_answer: false},
  context, limits[]}`; `call` carries id, timestamps, status, model, usage, latency and flags.
- `ask` failure: `{state: "failed", error{code, message}, call, limits[]}` on stderr, exit 3.
- `history`: `{state: "history", summary{calls, ok, failed, prompt_tokens, completion_tokens,
  daily_calls, daily_limit}, calls[]}`; call rows omit model text.
- `show`: `{state: "advisory", call{..., output_text}, limits[]}`.
- `config`: `{state: "config", configured, config{...}, credential{env, present}, limits[]}`.
- `consents`: `{state: "consents", sources[{source_id, title, granted, recorded_at, reviewer}],
  limits[]}`; consent/revoke return `{state: "consent", source_id, granted, recorded_at, reviewer}`.

Golden files pin config, preview (first-party and withheld private), answered advisory, history and
consent/consents documents. The `ask`/`history`/`show` fields are additive to the existing CLI
protocol; no earlier document changes meaning.

## Domain split

| Package / area | Truth it owns |
|---|---|
| `internal/tutor` | Provider-independent request/preview construction, prompt version, field clipping, privacy classification, host protocol types, config validation and the Node transport. No database and no network imports. |
| `internal/store` | Schema v7, config round-trip, append-only consent events and call records, effective consent, daily count. |
| `internal/cli` | `tutor` commands, attempt/content resolution, consent gating, credential/budget checks, advisory rendering and generated error hints. |
| `agent/tutor-host.mjs` | The single network egress: endpoint validation, bearer auth, timeout/body bounds, OpenAI-compatible response validation, structured errors without secrets. |
| `harness/gates/tutor-smoke.ts` | Real-binary + real-host + loopback stub provider smoke, key-leak scan, private-source consent path, provider failure and Node-absent fallback. |

## Evidence plan

1. **Go tests**:
   - `internal/tutor`: endpoint scheme/loopback/userinfo/query validation; config bounds; first-party
     prompt includes passage/key/evidence; imported-without-consent omits every private string and
     lists the withheld fields; consent includes them; edited/unavailable content disclosures; skill
     lesson inclusion and pre-v6 tag fallback; mode instructions; clipping and request hash stability;
     unknown mode rejected.
   - `internal/store`: v6→v7 migration keeps attempts and adds empty tables; config upsert; consent
     latest-wins and append-only update/delete rejection; call round-trip/filter/daily count;
     malformed call rejection.
   - `internal/cli`: config inspect/save/partial/invalid; preview unconfigured/no attempt/unknown
     attempt; first-party preview; private redaction; consent/revoke/list; ask success with a fake
     transport (advisory labels, call row, unchanged attempt rows/counts, key absent from output);
     provider/transport/invalid-response failures exit 3 with a recorded call and practice still
     works; missing credential/budget exit 2 without a call; history summary/filter/show; golden
     documents.
2. **Tier 2 transcripts** with the real binary, the real Node host and a local loopback stub provider
   (`/tmp/passquest-0005-demo/`, no external network, no real credential):
   - Configure endpoint/model; key presence; `tutor config --json`.
   - Practice an embedded question, `tutor preview`, then `tutor ask` with a real subprocess; record
     the advisory text, usage and unchanged `report` counts.
   - Import the first-party fixture, review page 1 and question `q2`, practice it; preview proves the
     page text is withheld; `tutor consent` includes it; `revoke` withholds it again.
   - Stub returns 500/401 and then is stopped: exit 3, call history shows the failure, attempts are
     unchanged, `practice` still records and grades.
   - `PASSQUEST_NODE` points at a missing binary: tutor exit 3, practice exit 0.
   - Daily budget 1: second `ask` exits 2 without a new call.
   - `history --json` totals; `show` advisory text; DB bytes and outputs scanned for the fake key.
3. **Boundary evidence**: the same attempt's row count/content before and after calls; a private
   source string absent from the redacted call; the advisory header/footer; `report`/`next` ignoring
   tutor output.
4. **Gates**: `bash scripts/gate.sh` real exit code and counts, including `tutor-smoke` (new) and its
   self-test case.

## Harness growth (same increment)

- `harness/gates/tutor-smoke.ts` builds the real CLI and starts a loopback OpenAI-compatible stub in
  the gate process. It drives config/preview/ask/history/show, the private-source consent round trip,
  a provider error, a missing Node, the daily budget and key-leak scans. No external network, no
  committed credential.
- `harness/gates/self-test.ts` gains a fake-CLI case that prints `{}`; `tutor-smoke` must fail it.
  `harness/gates/index.ts` registers the gate before `self-test`; `docs/harness.md` gains its row and
  updated counts. No new `scripts/*.sh`.
- The file-discovery helper now includes untracked, non-ignored files that exist. A pre-commit gate
  run must scan new deliverables (including this increment's), and the goal close's pending archive
  move must not make the old tracked path an unreadable docs-link failure. This is shared gate
  behaviour, documented in `docs/harness.md`, not a second gate definition.

## Trade-offs and defaults taken

- **Provider call in a Node host, not Go.** The local-only gate forbids `net`/`net/http` in first-party
  Go packages. A narrow stdin/stdout host keeps the product rule and isolates the only egress. Cost:
  one Node process per call and a Node requirement for tutor commands only. Ordinary practice stays
  Go-only. (Q-0005-02)
- **Environment credential, no persistence.** The desktop OS secret store arrives with goal 0011.
  Persisting a key in SQLite or a plaintext file now would violate the credential clause. Cost: the
  learner exports the variable in the session. (Q-0005-01)
- **Private text withheld by default.** Imported page text is the owner's restricted material; the
  earlier visibility exception covers committed parsed text, not upload consent. Default redaction
  plus a per-source append-only grant is the reversible, auditable choice. Cost: the first private
  coaching call is metadata-only until the learner authorizes the source. (Q-0005-03)
- **OpenAI-compatible chat completions with `max_tokens`.** It is the common denominator for the
  owner's BYOK endpoints and needs no SDK. Provider-specific capabilities and variants wait for goal
  0010. (Q-0005-04)
- **Advisory records are stored, raw prompts are not.** Usage and failure information must be visible;
  re-storing private passage text in a second table would enlarge the leak surface. The hash plus
  scope flags identify the sent request; the preview shows exact text before sending.
- **One attempt per call.** Full-history context would raise private-data exposure and cost without a
  demonstrated need; later goals can add bounded context deliberately.
- **No retry.** A retry can double provider spend and hide a failure. A failed call is recorded and
  the learner can invoke `ask` again; durable retry/reconciliation belongs to goal 0006.

## Out of scope, with reasons

- Multiple providers, account login, model capability discovery, web search, speech or image:
  goal 0010 and later; this increment supports one compatible text endpoint.
- Resumable/durable agent tasks, Pi Durable or pi-ai integration: goal 0006. The host protocol is
  versioned so the Pi runtime can replace the per-call process.
- OS keychain/secret-service storage: goal 0011; env-only until then.
- Streaming output, multi-turn chat, automatic question generation, new answer keys, grade changes,
  or model-written attempts: outside the tutor's authority and the contract.
- Anki export, external resource discovery, desktop UI: goals 0007, 0008, 0011.
- Sending whole-source histories or learner identity: not needed for bounded coaching.
- A measured pass probability, exam blueprint or time limit: still unknown and forbidden.
- Committing real provider output, credentials or private source excerpts: tests use authored
  fixtures and synthetic text.

## Completion checklist

- [x] Plan written to `docs/plans/` (this file).
- [x] `internal/tutor` prompt/config/host protocol and tests.
- [x] Schema v7 config/consent/call tables, migration, store tests.
- [x] `tutor config/preview/ask/history/show/consent/revoke/consents` CLI, goldens and tests.
- [x] `agent/tutor-host.mjs`, its protocol note and loopback-safe bounds.
- [x] `tutor-smoke` gate, self-test case, harness index and documentation.
- [x] `docs/tutor.md`, README, source-import/adaptive notes and `docs/harness.md` reconciled.
- [x] Goal file archived unchanged, goals index updated, open questions appended.
- [x] Evidence record with real transcripts and gate output.
- [x] `bash scripts/gate.sh` exit 0 recorded; no test or contract clause weakened.
