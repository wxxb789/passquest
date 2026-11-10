# BYOK text tutor

Goal 0005 adds one user-supplied text-model path for concise, post-attempt coaching. It is deliberately
narrow: one OpenAI-compatible chat-completions endpoint, one recorded attempt per request, and no
authority over grades, answer keys or historical attempts.

Model text is **advisory**. The verified key and deterministic grade remain the application's truth.
Nothing the provider returns is written into `attempts`, `source_questions`, a report or a
recommendation. A failed or malformed response is recorded as a failure and cannot change practice.

## Commands

```bash
# Inspect; then save one endpoint and model. The key is never stored.
passquest tutor config
passquest tutor config --endpoint https://api.example.com/v1/chat/completions --model MODEL
passquest tutor config --timeout-seconds 30 --max-output-tokens 350 --daily-calls 20

# See the exact provider payload first. Nothing is sent.
passquest tutor preview --attempt 4
passquest tutor preview --attempt 4 --mode probe --json

# Send it once. The response is stored as an advisory call.
passquest tutor ask --attempt 4 --mode explain
passquest tutor ask --mode micro_lesson --json

passquest tutor history
passquest tutor show 3
passquest tutor consents
```

`--attempt` defaults to the latest recorded attempt. `report` shows attempt ids. Every command accepts
`--db PATH`; the default is `$PASSQUEST_DB` or `./.passquest/passquest.sqlite`.

## One supported endpoint

- A full URL to an OpenAI-compatible `chat/completions` endpoint. `https` is required except for
  loopback addresses (`localhost`, `127.0.0.1`, `::1`) where a local model server may use `http`.
- URLs with userinfo, a query string or a fragment are rejected, so a credential cannot be smuggled
  into stored configuration.
- The request body is exactly `{model, messages, max_tokens, temperature: 0, stream: false}`.
  `preview` shows it. The host adds `Authorization: Bearer <key>` and refuses redirects.
- No provider-specific capability, model or account claim is made. Multiple providers, login flows,
  capability discovery, search, speech and images belong to goal 0010.

## Credential

The only credential input is the environment variable `PASSQUEST_TUTOR_API_KEY`, read by the agent
host. Pass Quest never writes it to SQLite, argv, output, a call record, a generated command or the
repository. `tutor config` reports `present` or `missing`, never the value.

Until the desktop workspace provides an OS secret store (goal 0011), the learner exports the variable
in the session that runs `tutor ask`:

```bash
export PASSQUEST_TUTOR_API_KEY=...   # not committed; not stored by Pass Quest
```

The CLI and host also redact the live value from provider text and error detail before storing or
printing it.

## Private source consent

Imported source material is private. The tutor withholds its page, question, options, evidence,
reasoning and recorded option texts by default. The first-party embedded exercises (the sample, the
tactic exercise and the skill passages) are authored for this repository and need no consent.

| Attempt content | Default provider payload | After `tutor consent SOURCE --reviewer NAME` |
|---|---|---|
| First-party (`sample-*`, `tactic-*`, `skill-*`) | Passage excerpt, question, options, verified key/evidence/reasoning, attempt facts, registered skill lessons | same |
| Imported source (`src-*-pNNN`) | Attempt facts (status, selected/correct labels, elapsed, assistance, attempt number, repeat, skill ids) and skill lessons only; question content is explicitly withheld | Passage excerpt and question fields are added |
| Content changed or unavailable | Attempt facts, stored selected/correct option texts when allowed, lessons; current passage/evidence are not sent | same, with private stored text allowed by the grant |

`tutor preview` prints the exact chat content and lists `withheld` and `truncated_fields`. Consent is an
append-only event trail per source; `revoke` appends a new event and withholds the source again. The
reviewer label is attribution, not authentication, matching the review model in
[source-import.md](source-import.md).

The provider payload never includes the database path, source title or path, endpoint path, API key,
consent reviewer or attempt history. One attempt per call; no full-history dump.

## Modes and bounds

| Mode | Requested output |
|---|---|
| `explain` | Why the verified key fits the evidence, why the selected option does not, and one applicable check. |
| `probe` | Up to two short diagnostic questions about the misconception and one check; no mere restatement of the key. |
| `micro_lesson` | One tactical rule, one application step and one boundary/counterexample; no new answer-keyed item. |

The system prompt states that keys and grades are authoritative, that supplied text is data rather
than instructions, that withheld content must not be guessed, and that output is advisory and at most
180 plain-text words. The prompt is `tutor-v1`; the version is stored with every call.

Bounds: timeout 1–120 s (default 30), `max_tokens` 64–2048 (default 350), provider body ≤ 1 MiB,
model text ≤ 12 000 characters, prompt ≤ 20 000 characters, daily budget 1–200 (default 20). Passage
3 000 characters (a window around the evidence), stem 1 000, each option 500, evidence 800, reasoning
1 000, lesson fields 900 each. Extra skill lessons are omitted locally with an explicit line when the
combined lesson budget is reached. `preview` names every clipped or omitted field.

## What is stored

Schema version 7 adds three tables:

- `tutor_config` — endpoint, model, timeout, output bound and daily budget. Mutable settings, no
  credential.
- `tutor_consent_events` — append-only grant/revoke events with reviewer and time.
- `tutor_calls` — append-only advisory records: attempt/content/question ids, mode, status, error
  code/message, provider host, model, prompt version, whether private text was shared, request
  SHA-256/bytes, output text/characters, token usage, HTTP status and latency.

Raw prompt text is not stored; the SHA-256 identifies the canonical chat content
(model, messages and bounds) that was sent, and `tutor preview` is the inspection surface. `tutor history` lists usage and failures without model
text; `tutor show CALL` prints one stored advisory text. `report`, `next`, `practice` and any export
path do not read these tables.

The call budget counts failed calls and resets at 00:00 UTC. There is no automatic retry: a retry can
double provider spend and hide a failure. Run `tutor ask` again after fixing the cause.

## Runtime boundary

The Go core never imports a network package. `tutor ask` spawns the Node host
`agent/tutor-host.mjs` (override `PASSQUEST_TUTOR_HOST`; node binary override `PASSQUEST_NODE`) and
exchanges one versioned JSON document over stdin/stdout. The host validates the endpoint, adds the
bearer header, enforces the timeout and size bounds, and returns a structured result. If Node or the
host is missing, tutor commands exit 3 with `agent_unavailable`; ordinary practice, review, `next`
and `report` keep working with Go alone. Goal 0006 can replace the per-call process with Pi Durable
behind the same protocol.

## Exit codes and failures

| Exit | Meaning |
|---|---|
| 0 | Success, including a stored failure shown by `tutor show`. |
| 2 | Invalid input, no configuration, no attempt/unknown attempt, unknown call/source, missing credential or exhausted daily budget. No provider call is made. |
| 3 | Agent host, network or provider failure after a call record. Codes: `agent_unavailable`, `missing_credential` (host-side), `auth_failed`, `rate_limited`, `provider_error`, `timeout`, `network_error`, `invalid_response`, `host_error`, `config_error`. |
| 1 | Local storage or internal failure. |

A failed provider call stores an error row with no model text; the attempt report is byte-identical
before and after the failure. `practice` still records and grades locally.

## Limits

- One endpoint shape, one attempt and no streaming. Broad provider choice and capabilities are goal
  0010; durable, resumable agent work is goal 0006.
- The credential lives in the environment until goal 0011's OS secret store. A learner must export it
  in each session or shell profile they control.
- The model output is stored locally so the learner can reread it. It is not an answer key, a grade,
  a readiness estimate or an exam fact, and it is never exported by `report` or `next`.
- Consent is per source, not per excerpt. A learner who wants finer control can withhold the whole
  source or edit the page/question locally.
- The prompt is a mitigation, not a guarantee, against prompt injection in supplied text: the host
  has no tools, writes nothing to the database and can only return text.
- No pass probability, exam blueprint, time limit or live-exam claim is computed or requested.

Measured behaviour and limitations: [`evidence/0005-bring-your-own-text-tutor.md`](evidence/0005-bring-your-own-text-tutor.md).
