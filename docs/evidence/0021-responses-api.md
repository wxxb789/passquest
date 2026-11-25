# 0021 — Model features work with an OpenAI Responses endpoint: evidence

Contract: [`../goals/0021-responses-api.md`](../goals/0021-responses-api.md).
Plan: [`../plans/2026-11-25-0900-0021-responses-api.md`](../plans/2026-11-25-0900-0021-responses-api.md).
Decisions: Q-0021-01…08 in [`../decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md).

Later steps append their dated notes; P4 completes the record and P5 appends the feed outcome. Every
number below is read from the artifact or the run its note names.

## Environment

Windows 11 Enterprise (10.0.26300), Go and Node through `mise` (the pins in `mise.toml`). No macOS or
Linux machine was used for P1; CI's three-OS `core` job is the authority for the gates P1 names. The
machine is shared and was often at high CPU load during the run (the brief's known environment
fact); every local gate below is a single run's reading.

## P1 (2026-11-25) — both endpoint forms, one rule (branch `goal/0021-p1`, base `ac58e08`)

**What changed.**

- `internal/provider/registry.go` gains the one format derivation: `Format`
  (`FormatChatCompletions`/`FormatResponses`) with `FormatOf(raw)` reading the path's final segment
  after `TrimRight("/")` and `Shape()` mapping to the registry shape values, plus
  `ShapeResponses = "openai-responses"` beside `EndpointShape` (which keeps its name and value and
  is no longer described as the only shape). `validateEndpoint` accepts a path ending in either
  suffix — trailing slash tolerated for both — and refuses everything else with
  `endpoint must end in /chat/completions or /responses`.
- The registry shape check accepts both shapes and, for an entry **with** a non-empty endpoint,
  requires the declared shape to equal the endpoint's derived shape, with a message naming both
  shapes. An entry **without** an endpoint (the embedded `custom` row) keeps its declared value
  (Q-0021-04) — `TestEmbeddedRegistryIsValidAndCoversRequiredProviders` stays green and now also
  pins the custom row's declared shape.
- `internal/study/models/providers.go`: the active custom row and the legacy row report the shape
  derived from the active endpoint (falling back to the declared value when the path derives
  neither); a row without an endpoint keeps its declared value. This is what `model list`
  (`internal/cli/model.go:78`) and the settings document (`internal/workspace/workspace_settings.go`)
  show.
- Strings and docs now name both forms: `internal/cli/model.go` (`model use`/`model test` flag
  help), `internal/cli/tutor_config.go` (flag help and the human hint), `internal/provider/failure.go`
  (the `invalid_response` action), `internal/agent/host.go` (the `ValidateEndpoint` doc comment),
  `docs/agent.md`, `docs/tutor.md` and `docs/providers.md`. No gate pins those wordings
  (`grep -rn` over `harness --include=*.ts` finds no content check), and the one test that reads an
  action string (`internal/provider/failure_test.go`) asserts `OpenAI-compatible`, which still
  holds.

**Disclosed transitional state (plan P1 item 7).** From P1 until P3 a `/responses` endpoint is
accepted by Go — `passquest model use custom/M --endpoint …/responses` succeeds and `agent start`
passes Go's validation — while both hosts still speak chat: the tutor host posts a chat body and the
durable host refuses a `/responses` endpoint at `agent/durable-host.mjs:214-215`. P2 closes the tutor
half, P3 the durable half; no gate asserts a `/responses` endpoint before P2. The P1 doc sentences
state the goal's end state (the path picks the wire format) and are made true by P2/P3 before this
goal merges; this note discloses the doc half of that state.

**Acceptance cases, proven sensitive against the old rule.** The new test table was run once in a
scratch copy of this tree whose `internal/provider/registry.go` tail was reverted to the old
single-suffix rule (`Q:/scratch/omp/g21/P1/oldrule`, log `Q:/scratch/omp/g21/P1/oldrule-tests.log`,
command `mise exec -- go test ./internal/agent ./internal/provider ./internal/study/models -count=1`):

```
--- FAIL: TestValidateEndpoint (0.00s)
    agent_test.go:108: https://api.example.com/v1/responses rejected: endpoint must end in /chat/completions
    agent_test.go:108: https://api.example.com/v1/responses/ rejected: endpoint must end in /chat/completions
    agent_test.go:120: refusal "endpoint must end in /chat/completions" does not name "/responses"
--- FAIL: TestResponsesProviderIsAccepted (0.00s)
    registry_test.go:54: responses provider rejected: provider registry: provider "alpha": endpoint must end in /chat/completions
    --- FAIL: TestRegistryValidationRejectsUnsafeData/third_endpoint_form
        registry_test.go:163: Validate() = provider registry: provider "alpha": endpoint must end in /chat/completions, want "must end in /chat/completions or /responses"
```

So a green run on this tree means the rule changed, not that the tests are blind. (`internal/study/models`
stays green in the copy: that package's derivation does not depend on the validator's suffix rule.)

**The item-4 before-picture (chat bodies on the base tree).** Script
`Q:/scratch/omp/g21/P1/capture-bodies.mjs` (scratch only), run on the clean base tree
(`git rev-parse HEAD` → `ac58e0860087c2539fce1193f62af9c6a7b22b28`):

```
mise exec -- bun Q:/scratch/omp/g21/P1/capture-bodies.mjs Q:/scratch/omp/g21/P1/base/bodies.json
```

It posts one fixed tutor `HostRequest` through `agent/tutor-host.mjs` and runs one fixed `agent start`
through the source `agent/durable-host.mjs` against a loopback stub, and writes every received raw
provider request body with its SHA-256 to `Q:/scratch/omp/g21/P1/base/bodies.json`
(`captured_at` `2026-11-25T03:16:17.413Z`). Result: one tutor request and three durable requests, all
at `/v1/chat/completions`; the tutor body equals the `HostRequest.body` verbatim
(`matches_host_request_body: true`).

| # | path | SHA-256 | bytes |
|---|---|---|---|
| tutor 1 | `/v1/chat/completions` | `942be11ee6699878549ebc7c3cbb0e4de8519c38bf1a9c2212b07ac9065a459a` | 215 |
| durable 1 | `/v1/chat/completions` | `68d3f96e2977727d5a9c9ac6471d5663ff2da904d45d74427bf3aead167b01a4` | 8488 |
| durable 2 | `/v1/chat/completions` | `4717cc610c59815a228c23fd7e747853f38fae75a9e7ba6c39ae557815a1a59b` | 8874 |
| durable 3 | `/v1/chat/completions` | `b2059a42bdf288d2367ae0db67c468f8b480bdd0af05a5695d062558da6509d6` | 9236 |

The tutor body keys are `model,messages,max_tokens,temperature,stream`; the durable bodies' keys are
`model,messages,stream,store,max_tokens,tools`, with the system text as a `messages[]` entry of role
`system`. This artifact is the "before" of item 4's comparison; P2's tutor re-run and P3's durable
re-run diff against it.

**Local audits (this machine, one run each; the commands of the brief's check list).**

| command | result |
|---|---|
| `mise exec -- gofmt -l internal cmd` | no output |
| `mise exec -- go vet ./...` | exit 0 |
| `mise exec -- go build ./...` | exit 0 |
| `mise exec -- bun Q:/scratch/pq-harness-run/run-0021.ts typecheck.ts typecheck` | pass — 0 type errors in `tsconfig.json`, `desktop/web/tsconfig.json` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run-0021.ts secrets.ts secrets` | pass — 800 files, 0 findings (re-run after this record was added; the first run read 799) |
| `mise exec -- bun Q:/scratch/pq-harness-run/run-0021.ts local-only.ts localOnly` | pass — 37 first-party packages, 1 allowed network package, 0 confidential markers, 0 boundary violations |
| `mise exec -- bun Q:/scratch/pq-harness-run/run-0021.ts docs-links.ts docsLinks` | pass — 358 checked, 0 broken |
| `mise exec -- bun Q:/scratch/pq-harness-run/run-0021.ts provider-smoke.ts providerSmoke` | pass — 92 assertions |
| `mise exec -- bun Q:/scratch/pq-harness-run/run-0021.ts tutor-smoke.ts tutorSmoke` | pass — 156 assertions |
| `mise exec -- bun Q:/scratch/pq-harness-run/run-0021.ts agent-smoke.ts agentSmoke` | pass — 238 assertions, 11 scenarios × 2 hosts (source host, staged bundle), 49 provider requests, bundle 1,557,656 B with 7 inlined packages |

`mise exec -- go test ./internal/... -count=1` was run once: every package passes except
`internal/desktop`, where `TestReadWindowTextReadsTheOSTitle`
(`internal/desktop/smoke_title_windows_test.go:34`) blocked in its user32
`GetWindowTextLengthW`/`GetWindowTextW` call on the window the test itself created, until Go's 600 s
package timeout killed the binary (`FAIL github.com/wxxb789/passquest/internal/desktop 601.348s`);
the run took 722 s overall. Attribution: this diff changes no `internal/desktop` file and no code the
failing test calls; the run overlapped this branch's own sensitivity-copy test run and three smokes
plus other sessions' load, the brief's known window-under-load class (`findings-log.md:50`), and the
same test passes on the untouched base tree (`ac58e08` extracted with `git archive` to
`Q:/scratch/omp/g21/P1/base-tree`: `go test ./internal/desktop -run TestReadWindowTextReadsTheOSTitle
-count=1 -timeout 150s` → `ok … 0.367s`), and the whole desktop package passes on this tree in
isolation at 96 % CPU (`go test ./internal/desktop -count=1 -v -timeout 420s` → 34 tests,
`ok … 1.340s`, `--- PASS: TestReadWindowTextReadsTheOSTitle (0.02s)`; log
`Q:/scratch/omp/g21/P1/desktop-isolation.log`). A second run over every package except
`internal/desktop` then finished clean in 73.91 s (`rest exit=0`; `ok` for `internal/cli` 37.2 s,
`internal/workspace` 5.4 s, `internal/provider`, `internal/agent`, `internal/study/models` and the
rest). `typecheck`, `secrets`, `docs-links` and `local-only` were re-run after this record's text
was added. CI's three-OS `core` job is the authority for `go test ./...`.

**What this note does not claim.** P1's evidence is Windows-only; the tutor and durable halves of
Done-when 2 and the settings line are P2/P3/P4's, the golden and the responses smoke cases are P2's,
and the Pi bump or its feed-blocked record is P5's. No real provider endpoint was contacted
(Q-0021-08).

## P2 (2026-11-25) — the tutor host speaks Responses; the settings line (branch `goal/0021-p2`, stacked on `d477655`)

**What changed.** One Go wire document per request: `internal/tutor` gains `Responses` /
`ResponsesMessage` / `ResponsesText` and `HostRequestFor`, which derives the format from the
endpoint's path (`internal/wire`'s shared `FormatOf`, which `provider.FormatOf` re-exports) and
builds exactly one of `chat` / `responses`, with `Body` the serialization of that same document and
`ResponsesFor` (`internal/tutor/tutor.go:640`) mapping the system text to `instructions` and every
other message to an `input` item in order;
`transport.validateRequest` checks the document the request carries against the endpoint's own
format, the per-format bounds (temperature 0, `stream: false`, `store: false`, one `input_text`
part per item, the 20,000-code-point total) and the `Body` invariant;
`internal/note/refine.go` builds its refinement document through the same builder;
`internal/study/notes/refine.go`, `internal/workspace/workspace_coach.go` and the CLI's preview
read the non-nil document through `WireMessages()`/`WireMaxTokens()`; `internal/cli/tutor_views.go`
carries `chat` or `responses` (omitempty) and `writeTutorPreview` prints the document it sends.
`agent/tutor-host.mjs` derives the format from `request.endpoint`, refuses a document for the other
format or a third form, validates the responses document's exact key set and values, posts the same
bytes, and maps text (`output[].content[].type == "output_text"` across message items), usage
(`input_tokens`/`output_tokens` → `prompt_tokens`/`completion_tokens`), `status != "completed"`
(with `incomplete_details.reason`), a `refusal` part, and a 200 that carries `error` to
`invalid_response`. `desktop/web/src/views/settings.ts` gains the `endpoint-formats` line.

**Deviation from the plan's Q-0021-01 wording (one derivation's home).** The plan names `provider`
as the format derivation's home, but `internal/provider` imports `internal/tutor`
(`provider.ClassifyResponse(tutor.HostResponse)`, `internal/provider/failure.go:67`), so `tutor`
cannot import `provider` without a cycle. The derivation therefore lives in the new leaf package
`internal/wire` (`Format`, `FormatChatCompletions`, `FormatResponses`, `FormatOf`, `FormatOfPath`,
`Shape`, `ValidShape`, the two shape values), and `internal/provider` re-exports every P1 name as
an alias or one-line wrapper (`registry.go:26-45`), so the registry's behaviour, its tests and
`internal/study/models/providers.go` are unchanged. One implementation, read by both packages;
P1's `provider.FormatOf` test table still passes verbatim.

**The tutor chat bodies are unchanged, and the responses body is new (item 4's tutor half).** The
capture ran twice in round 1 with P1's script
(`mise exec -- bun Q:/scratch/omp/g21/P1/capture-bodies.mjs Q:/scratch/omp/g21/P2/bodies.json` →
`Q:/scratch/omp/g21/P2/bodies.json`, the chat half) and once in round 2 with
`Q:/scratch/omp/g21/P2/capture-bodies-r2.mjs`, which adds one tutor host request over a
`/v1/responses` endpoint carrying the Q-0021-02 document (→ `Q:/scratch/omp/g21/P2/bodies-r2.json`).
The chat tutor body is byte-identical to `Q:/scratch/omp/g21/P1/base/bodies.json` (same SHA-256
`942be11ee6699878549ebc7c3cbb0e4de8519c38bf1a9c2212b07ac9065a459a`, same 215 bytes, posted
verbatim); the new responses body went to `/v1/responses` verbatim (SHA-256
`7fc9e43c046388c14353c66bdb14faccd1f3ef0f498ab2c8062b31a0e662d629`, 251 bytes):

```json
{"model":"capture-model","instructions":"You are a concise exam coach.","input":[{"role":"user","content":[{"type":"input_text","text":"Explain why 'an' precedes a vowel sound."}]}],"max_output_tokens":350,"temperature":0,"stream":false,"store":false}
```

and the stub's `usage.input_tokens`/`output_tokens` came back as `prompt_tokens`/`completion_tokens`
in the host's reply. The three durable chat requests are identical in length (8488/8874/9236 bytes)
and differ only in the run id the capture mints
(`run-20261125T031603Z-d54219a1` → `run-20261125T074041Z-bce53e0a`): a byte-by-byte comparison of the
two `raw` fields puts the first difference at 0-based byte 1544, the run id's `3`→`6`-style digit
(`Q:/scratch/omp/g21/P2/compare-r2.ts`); the durable host stamps it. The durable half stays chat
until P3.

**New golden, declared here for `golden-contract`.**
`internal/cli/testdata/tutor-preview-responses.golden.json` is a `tutor preview --json` against
`https://api.example.com/v1/responses`; `TestTutorGoldenDocuments`'s new `responses preview` subtest
compares it (`internal/cli/tutor_test.go:338`), and the six existing chat goldens are unchanged —
a changed chat golden would be a failure, not an update. The preview document carries `responses`
and no `chat` key; the human renderer prints `[instructions]` and the input items.

**Local audits (this machine; the commands of the brief's check list, each run on this tree and re-run on the round-2 tree with the same readings).**

| command | result |
|---|---|
| `mise exec -- gofmt -l internal cmd` | no output |
| `mise exec -- go vet ./...` | exit 0 |
| `mise exec -- go build ./...` | exit 0 |
| `go test ./internal/... -count=1` | all packages pass (see P1's note for the `internal/desktop` window-test class; it passed here) |
| `run-0021.ts tutor-smoke.ts tutorSmoke` | pass — 359 assertions, both formats (was 156) |
| `run-0021.ts provider-smoke.ts providerSmoke` | pass — 115 assertions (was 92) |
| `run-0021.ts golden-contract.ts goldenContract` | pass — 1 golden declared by 2 contract document(s) |
| `run-0021.ts typecheck.ts typecheck` | pass — 0 type errors |
| `run-0021.ts local-only.ts localOnly` | pass — 38 first-party packages, 1 allowed network package |
| `run-0021.ts docs-links.ts docsLinks` | pass — 358 checked, 0 broken |
| `run-0021.ts secrets.ts secrets` | pass — 802 files, 0 findings |
| `run-0021.ts frontend-baseline.ts frontendBaseline` | pass — all 4 allowlist entries fire, 6 vendor tokens in source and bundle |
| `run-0021.ts note-smoke.ts noteSmoke` | pass — 1367 assertions (its `note propose` seed and host path are unchanged) |
| `run-0021.ts agent-smoke.ts agentSmoke` | pass — 238 assertions, 11 scenarios × 2 hosts, 50 provider requests, bundle 1,557,656 B with 7 inlined packages (the durable path is untouched until P3) |

**The settings scenario, run once on this machine (advisory; CI's three OSes are the authority).**
`Q:/scratch/omp/g21/P2/settings-run.ts` runs `makeDesktopSmokeGate(["settings"])` against this tree:
`STATUS pass 35 assertions over 3 scenarios; boot: room 40/0/138/40 px, marker true`. Inside it, the
second registry provider (`stub-responses`, `endpoint_shape: openai-responses`) had its key saved
through the window and its `model-test` read `[data-capability="verified"]` after the stub received
the probe at `/v1/responses` carrying the OS-store `Bearer` key, the page's
`[data-testid="endpoint-formats"]` line named both `/chat/completions` and `/responses`, and both
keys were cleared again (the chat provider's row and the clearing half are the P1/P2 scenario's
own). This is P2 → P3/P4's handed-over reading: probe path `/v1/responses`, key path the OS secret
store (`api-key:stub-responses`), page hook `endpoint-formats`, both suffixes inside
`<code translate="no">`.

**Must-fail self-test cases (one per changed gate, in the repository's own case files).** P2 adds
`harness/gates/self-test/tutor-smoke.cases.ts`'s case over a copied `agent/tutor-host.mjs` whose
responses branch validates the chat key set and values, and `provider-smoke.cases.ts`'s case over a
copied `internal/provider/registry.go` whose endpoint rule is chat-only again, beside the
desktop-smoke case in `desktop-smoke.cases.ts` — the three case objects the `self-test` gate itself
runs on all three OSes. Run here from those definitions with the self-test's own fixture logic
(`Q:/scratch/omp/g21/P2/must-fail-r2.ts`): the tutor case `FAILED AS DESIGNED` at
`responses: tutor ask exit = 3`, the provider case at `delta: model test exit = 2` (the declared
responses provider is refused), and the desktop-smoke case through a real window at
`settings: script failed: mygo: javascript error: timed out waiting for the endpoint formats line
[route #/settings]` (advisory as above). The same gates on the *unpatched* copy stop failing where
the patch is the fixture's only change: tutor-smoke reads `PASS 359 assertions`. The unpatched
provider copy is not a clean pass fixture here — `copyProduct` does not follow
`agent/node_modules`'s symlinks (`@earendil-works/*`), so that copy's agent section fails on its own
(`agent start did not pause (exit 3)`); provider-smoke's pass proof is its real-tree run above.

**Round-2 fixes (2026-11-25, after the three reviews of the first round).** Each finding was
verified against the tree before it was fixed or answered:

- *The missing must-fail cases (all three reviews, the round's blocker).* Fixed: the tutor and
  provider cases now live in the repository's case files (above), which is what the `self-test` gate
  runs on three OSes. Round 1 had proven them only through a scratch runner while this note and
  `docs/harness.md` said they were in the tree; the claim is true now, and `docs/harness.md`'s P2
  re-quote carries the counted total (173 cases: 157 broken + 16 must pass).
- *Stale names in this note.* Fixed: `ResponsesFor` (`internal/tutor/tutor.go:640`), and the
  derivation is named where it lives (`internal/wire`), with `provider.FormatOf` as its re-export.
- *The duplicated stub replies.* Fixed: the tutor smoke and the provider smoke import
  `chatReply`/`responsesReply` from `harness/lib/desktop-page.ts`; the provider smoke's own third
  copy is gone.
- *provider-smoke could not fail on most of its checks (found while fixing the above).* Its pass path
  lacked the `if (checks.length > 0) return fail(checks)` guard its sibling gates carry. Measured on
  this tree, two checks were failing silently: the `next`/`report` comparison never matched a Windows
  path written escaped in JSON, and the human failover call passed `--db` before its subcommand
  (`exit = 2`, no line). Both are fixed, the guard is in place, and the gate reads 115 assertions
  with zero accumulated failures — the same measurement read 2 before the fix.
- *Evidence numbers.* The capture's first-differing-byte claim was re-measured byte by byte against
  the artifacts this note names: 0-based 1544 (the run id's digit) and three durable bodies equal in
  length; the review's 1546 does not reproduce against them. `agent-smoke`'s provider-request count
  read 50 in round 2 where P1's note read 49: it is printed, not asserted, CI's goal-0016 record
  (`docs/evidence/0016-lean-package.md`) reads 50 too, and no P2 file is on that gate's path
  (`git diff --stat HEAD -- internal/study/brief internal/agent agent/durable-host.mjs
  harness/lib/sse.ts harness/gates/agent-smoke.ts` is empty) — not attributed to this step, cause
  unknown.
- *The allowlist entry's overlap with 0022 P2's `settings-row` entry.* Accepted, not a defect: the
  settings line moves `settings-row` on both comparison OSes, so the entry must apply to both;
  `bench.ts`'s lookup is last-entry-wins by state, so on Windows the printed reason is this step's,
  and the pass/fail decision is unaffected; P3's row removes the entry.
- *Comment-policy items.* Fixed: `agent/tutor-host.mjs`'s "validated against the chat content" now
  names the wire document, and the desktop case's helper docstring that duplicated its case comment
  is gone.

Round 2 re-ran the settings scenario (`Q:/scratch/omp/g21/P2/settings-run-r2.log` → `STATUS pass 35
assertions over 3 scenarios; boot: room 40/0/138/40 px, marker true`) and the desktop-smoke must-fail
case (`must-fail-desktop-r2.log` → `settings: script failed: … timed out waiting for the endpoint
formats line [route #/settings]`), both through a real window and both advisory, and the whole audit
list above on the fixed tree.

**Round-3 fix (2026-11-25): the settings scenario inside the full gate.** The correct-lens round-2
review's blocker was real and is fixed. In the full `desktop-smoke` (and in `package-smoke`, which
runs the same scenario modules on one shared stub) the `settings` scenario timed out waiting for the
verified responses model: `coach` and `anki` end by restoring the shared stub's `respond` with a
chat-only closure (`() => chatReply("OK")`), so the settings scenario's responses provider probe
reached `/v1/responses` and was answered with a chat-completions body — the host refused it
(`invalid_response`) and `[data-capability="verified"]` never appeared. Reproduced on this machine on
the pre-fix tree with `makeDesktopSmokeGate(["coach", "settings"])`
(`Q:/scratch/omp/g21/P2/fix-r3/slice.ts`): `STATUS fail` /
`settings: script failed: mygo: javascript error: timed out waiting for the verified responses model
[route #/settings]`, ELAPSED 236s — the round-2 settings reading came from `only: ["settings"]`,
which skipped this ordering interaction. The same cold slice run also printed
`coach: no listening-socket probe completed while the app ran`, the launch probe loop's own
machine-timing artifact; it did not recur in the warm slice run or in the full run below.

Fix: `harness/lib/desktop-page.ts` exports the stub's default answer as `defaultStubReply` (the
path-dispatched reply `startProviderStub` now uses), and every scenario that overrides `respond`
restores that default: `harness/gates/desktop-smoke/coach.ts:79`, `anki.ts:336` and
`harness/gates/package-smoke/brief.ts:261` (the package smoke's `credential` case is the settings
module, so its stub state is the same one). `agent/tutor-host.mjs` also validates the endpoint before
dispatching on its format (`:143-146`), so an unparseable endpoint carrying a responses document
reports the true `endpoint is not a URL` instead of `endpoint ends in /chat/completions but the
request carries a responses document`; the chat path's message precedence is the pre-P2 host's
again (probe `Q:/scratch/omp/g21/P2/fix-r3/f3-probe.ts`: `before: config_error — endpoint ends in
/chat/completions but the request carries a responses document` / `after : config_error — endpoint is
not a URL`).

Readings after the fix, all on this tree: the slice `STATUS pass 79 assertions over 4 scenarios`
(ELAPSED 38s, `Q:/scratch/omp/g21/P2/fix-r3/slice.log`), and the **full** gate in the foreground
`mise exec -- bun Q:/scratch/pq-harness-run/run-0021.ts desktop-smoke.ts desktopSmoke` →
`STATUS pass 566 assertions over 14 scenarios; boot: room 40/0/138/40 px, marker true; titlebar:
880x600 room 40/0/138/40 px x14 at 14 stops, marker 14/14` (ELAPSED 157s,
`Q:/scratch/omp/g21/P2/fix-r3/full-desktop-smoke.log`) — the P2 row's own proof, advisory, with CI's
three-OS run as the authority. Re-run checks: `gofmt -l internal cmd` empty, `go vet ./...` and
`go build ./...` exit 0, `typecheck` 0 type errors, `tutor-smoke` 359 assertions, `provider-smoke`
115 assertions, `go test ./internal/... -count=1` clean, `golden-contract` 1 golden declared by 2
contract documents, `docs-links` and `secrets` as below, the three must-fail self-test cases
`FAILED AS DESIGNED` again (`must-fail-r3.log`), and the capture re-run
(`Q:/scratch/omp/g21/P2/fix-r3/bodies-r3.json`): the tutor chat body is byte-identical to the P1
base artifact (`942be11e…`, 215 bytes), the responses body is unchanged (`7fc9e43c…`, 251 bytes), and
the three durable chat bodies keep their 8488/8874/9236 lengths with new run ids.

**What this note does not claim.** The window halves of the settings scenario and the bench run are
CI's; the durable half of item 2 and the `brief-responses` case are P3's; the Pi bump or its
feed-blocked record is P5's. The plan's per-step `frontend-bench.yml` run is the coordinator's
dispatch; this branch carries the `settings-row` allowlist entry (`os: "any"`) the P2 page change
needs, and P3's row removes it. No real provider endpoint was contacted (Q-0021-08).

## P3 (2026-11-25) — the durable host speaks Responses through pi-ai's own adapter (branch `goal/0021-p3`, stacked on P2's `d71a922`)

**The step's changes.** `agent/durable-host.mjs` derives the endpoint's form once
(`endpointFormat`, the same rule the Go validator and the tutor host apply, trailing slashes
included — round 2) and both of its sites use
it: `validateRequest` accepts either suffix and refuses anything else with `model endpoint must end
in /chat/completions or /responses`, and `buildModels` strips the endpoint's own suffix for
`baseUrl`, sets the model's `api` to `openai-responses` or `openai-completions`, and declares the
provider's `api` as the map form (Q-0021-03), so `createProvider` dispatches on `model.api`
(`dist/models.js:576`) to pi-ai's own Responses adapter; the compat block is unchanged, and its
`supportsDeveloperRole: false` keeps the system text at role `system`. `harness/lib/sse.ts` gains
the Responses twin of the brief handshake (`sseResponsesText`, `sseResponsesTool`,
`briefResponsesAnswer`, `stubResponsesInput`/`stubResponsesText`/`stubResponsesToolResults`) over
the event set the library's parser requires — `response.created`, the item's added/delta/done
events and a terminal `response.completed` carrying `usage.input_tokens`/`output_tokens` — with the
chat builders byte-identical. `harness/gates/agent-smoke.ts` gains
`DurableMatrixOptions.format`, the stub answers the configured path, the matrix asserts every
provider request's path and the format's own threading (chat: a `role: tool` message; responses: a
`function_call_output` item, and neither shape carrying the other form's message field), and
`agentSmoke` runs four passes (source host and staged bundle × both forms).
`harness/gates/package-smoke/brief.ts` is parameterized by endpoint form — `brief` and
`brief-responses`, the latter seeding the stub's responses provider (which
`harness/lib/desktop-page.ts`'s `stubResponsesProvider` now also builds for the settings scenario)
and asserting the resume's path and its `function_call_output` threading — and a start that does not
pause reports the CLI's own error code and message, so a refusal is readable from the failure line.
Two self-test cases are added (`harness/gates/self-test/agent-smoke.cases.ts`,
`package-smoke.cases.ts`) with `linkStore` in `fixture.ts`: a copied product whose provider streams
every model through the chat adapter (the responses passes must fail, the chat passes stay green),
and an installed Responses brief whose durable host drops the responses branch (scoped to
`brief-responses`, which must name the host's endpoint rule). `internal/agent/agent.go`'s
`ClassifyFailure` learns the parenthesized status pi-ai's Responses adapter composes
(`passquest API error (401): …`), with rows in `TestClassifyFailure` and `TestClassifyRunFailure`;
without it the agent smoke's auth scenario reported `provider_error` in both responses passes (probe
`Q:/scratch/omp/g21/P3/probe-auth.ts`: chat `auth_failed`, responses `provider_error` on the
pre-fix tree) while the chat path's classification is untouched (round 2 pins that precedence with
its own rows, below). P2's `settings-row` allowlist entry
is removed, as P3's row says; `seedStudyTrail` takes the model to test and use
(`harness/gates/desktop-smoke/scenario.ts`, default `STUB_MODEL`, so the coach case's call is
unchanged).

**The capture (item 4).** `Q:/scratch/omp/g21/P3/capture-bodies-r3.mjs` →
`Q:/scratch/omp/g21/P3/bodies-r3.json` (comparison `…/compare-r3.ts`). The three durable
chat-completions bodies are length-identical to P1's base capture (8488/8874/9236 bytes) and equal
after run-id normalisation — the 22 differing bytes all lie inside the two run-id tokens, the first
at 0-based offset 1543 — so the chat path
did not move; the tutor halves reproduce P2's hashes (`942be11e…` chat, `7fc9e43c…` responses). The
three responses bodies (8568/8922/9252 bytes) all reach `/v1/responses`, carry
`function_call_output` items for the recorded steps, the system text as an `input` item with role
`system`, flat function tools, no `messages` field, `store: false`, `stream: true`,
`max_output_tokens: 600` — and `prompt_cache_key`, a per-conversation uuidv7 (see the correction
below).

**Correction to the plan.** Q-0021-03 discloses that `prompt_cache_key` is *not* in the posted body
because "the host passes no session id". The artifact refutes that: pi-durable's harness owns a
per-conversation `ProviderDoc.sessionId` (`dist/harness/provider.js:10`, passed with the generation
at `generation.js:152`), and pi-ai's Responses adapter sends it (`dist/api/openai-responses.js:263`
with `openai-prompt-cache.js`), so the durable responses body carries
`"prompt_cache_key":"01a2122f-…"`. The difference table in this record names it as the library's own
field; the reversal statement is unchanged. The `ClassifyFailure` change is not named in the plan's
P3 row; it is required by the plan's own four-pass matrix (the auth scenario) and leaves the chat
path's classification untouched: round 2 made the leading bare `<status>:` outrank the
parenthesized status, so a chat body that quotes a gateway's `(429)` keeps its own class.

**Readings, all on this tree.** `mise exec -- bun Q:/scratch/pq-harness-run/run-0021.ts
agent-smoke.ts agentSmoke` → `STATUS pass 488 assertions, 11 scenarios x 4 passes (source host,
chat, source host, responses, staged bundle, chat, staged bundle, responses), 100 provider
requests, permissions skipped on Windows, bundle 1598993 bytes with 7 inlined packages` (round 2,
ELAPSED 141s; round 1 read the same assertions and requests in 175s with a 1598913-byte bundle,
and the provider-request count includes retries and read 98 on the platform review's loaded runs.
The first run of the step failed with `source host, responses: auth code provider_error` and
`staged bundle, responses: auth code provider_error` and nothing else).
The package smoke (`… package-smoke.ts packageSmoke`, the step's one local package run) →
`STATUS pass 612 assertions over contents, layout, study, agents, brief, brief-responses, anki,
upgrade, credential, broken; …` (ELAPSED 625s; round 2's re-run after the host trim read the same
612 in 375s under this machine's load) — the new `brief-responses` case's paused run, its
window resume and its installed-binary read-back all pass beside the chat `brief` case. The two
self-test cases, each run individually through
`Q:/scratch/omp/g21/P3/run-case.ts` (the full self-test is CI's): the package case failed as
designed in both rounds (`status fail in 22s` / round 2 `12s`), its failure line
`brief-responses: the installed run did not pause (exit 3: config_error: model endpoint must end in
/chat/completions or /responses)`, accepted by its `postFail`; the agent-smoke case failed as
designed (`status fail in 318s`; round 2 `93s` with the full list,
`Q:/scratch/omp/g21/P3/fix-r1/self-test-agent-r2.log`): 96 failure lines — 48 prefixed `source
host, responses:` and 48 `staged bundle, responses:`, none from a chat pass (the chat adapter
posted the responses pass's requests to `/v1/chat/completions` and the run then failed
`provider_error: Stream ended without finish_reason`) — accepted by its `postFail`. The step's other
checks: `gofmt -l internal cmd` empty, `go vet ./...` and
`go build ./...` exit 0, `go test ./internal/... -count=1` ok, `typecheck` 0 type errors,
`docs-links` 368 checked / 0 broken, `secrets` 811 files / 0 findings, and the self-test's case
total 175 (159 broken + 16 must pass) over 26/26 gates counted from the per-gate arrays.

**Round-2 fix (2026-11-25).** The reviews' blocker: `endpointFormat` matched the raw string while
`wire.FormatOf` and the tutor host trim trailing slashes, so a learner's `https://…/v1/responses/`
was accepted by the Go validator and the window and then refused by the durable host
(`config_error`, exit 3). The probe `Q:/scratch/omp/g21/P3/fix-r1/trim-probe.ts` over the real
host: with the trim removed, `/v1/responses/` reports `model endpoint must end in
/chat/completions or /responses` while `/v1/responses` advances to `missing_credential`; with the
fix, both advance, and so does `/v1/chat/completions/`. `agent-smoke`'s Responses passes now
configure the trailing-slash endpoint, so the gate itself fails when the trim is removed
(`Q:/scratch/omp/g21/P3/fix-r1/gate-red.ts` over a copy whose trim is gone: `status fail in 243s`,
the responses passes' first lines `start exit 3: … model endpoint must end in /chat/completions or
/responses`, the chat passes green). `ClassifyFailure` now tries the leading bare `<status>:`
before the parenthesized status, so a chat detail that quotes a gateway's `(429)` keeps its class
(`502: upstream failed (429): retry later` → `provider_error`/`resume` and `400: quota (429): slow
down` → `config_error`/`check_endpoint`, both red before the precedence in `TestClassifyFailure`);
the Responses form still classifies (`passquest API error (401): …` → `auth_failed`).
`docs/harness.md`'s row 26 body now names `brief-responses` in its case order. The self-test
fixture sets the executable bit on the Node runtime it copies on POSIX (the install paths' own
rule), and the package case's patch anchor follows the new line. Re-run checks: `gofmt -l internal
cmd` empty, `go vet ./...` and `go build ./...` exit 0, `go test ./internal/... -count=1` ok,
`typecheck` 0 type errors, `docs-links` and `secrets` as above.

**What this note does not claim.** The package smoke's and the agent smoke's window halves on
macOS and Linux, the full self-test, and the `frontend-bench.yml` A/B run (the coordinator's
dispatch) are CI's or the coordinator's; this machine proves the Windows source and bundle passes
above. The durable responses request's acceptance by a real provider, and whether a gateway that
claims `/v1/responses` accepts `store`/`instructions`, remain the owner's probe (Q-0021-08) — the
stub's refusals are the fake's, not a real provider's. P4 completes the record's Done-when map and
the difference table; P5 owns item 5, still pending its feed check.
