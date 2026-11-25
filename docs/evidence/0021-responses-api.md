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
