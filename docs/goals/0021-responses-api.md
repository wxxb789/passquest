# 0021 — Model features work with an OpenAI Responses endpoint, on current Pi packages

**State:** Ready (defined 2026-11-22)
**Prerequisite:** None. It can run before or after 0020. Do not run it at the same time as 0020 on one tree; both change the package's dependencies.

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; recoverable, idempotent local state; no central account or cloud service. Follow [Core Design](../core-design.md).

The learner brings their own model provider: an endpoint, a model name and a key (see `docs/agent.md`). Today the app speaks one wire format only, OpenAI-compatible chat completions. The Go side accepts only an endpoint path ending in `/chat/completions` (`internal/agent/host.go`), and a test asserts that a `/v1/responses` endpoint is refused. Two Node hosts call the model: the tutor host (`agent/tutor-host.mjs`, a direct non-streaming request with temperature 0 and a 64–2048 token limit) and the durable host (`agent/durable-host.mjs`, through the Pi `pi-ai` chat-completions adapter). The search host calls a separate search endpoint and is not a model call.

The owner's decision (2026-11-22): supporting the OpenAI Responses API is a must, because current OpenAI and Azure OpenAI models and gateways are moving to it. The Pi packages the app depends on (`@earendil-works/chord`, `pi-ai`, `pi-durable`, `pi-schema`, all at 1.5.2) also have a newer release. Upstream 1.6.0 shipped on 2026-11-20 and updates `undici` for six security advisories. It also changes how system-role messages are sent to non-OpenAI providers that use the OpenAI-compatible format. The owner wants them on the newest release.

## Intended outcome

A learner who configures a Responses endpoint gets every model-backed feature working exactly as a chat-completions learner does: the provider check, the tutor's answers and coaching, and the durable agent brief. A learner on chat completions sees no change. The app runs on the newest `@earendil-works` release the corporate npm feed serves, at least 1.6.0.

## Done when

Each item below is shown during the run with the evidence named. All of it must hold on the final tree, and the existing gates must stay green.

1. **The endpoint accepts both formats.** An endpoint whose path ends in `/responses` is accepted wherever one ending in `/chat/completions` is today: in the CLI flags, the window's settings and the stored configuration. Any other path is still refused with a message that names both accepted forms. The test that refused `/v1/responses` is replaced by tests for both forms and for a refused third form. Evidence: the Go tests and the settings smoke.
2. **Every model feature works over Responses.** The provider check, the tutor (answers and coaching) and the durable brief each complete against a Responses endpoint, with the same validation, limits, redaction, consent checks and error classes they have on chat completions. Evidence: the provider, tutor and agent smokes, extended so each runs against a local fake for each format. Each fake reproduces its format's real request shape, response shape and refusals for the fields the app sends.
3. **Format differences are handled, not hidden.** The run establishes, from the official API reference, how the Responses format differs in what the app relies on: the token limit field, temperature, the system or instruction text, response text extraction, usage counts and error bodies. Each difference is handled in code. Any behaviour that cannot match chat completions is disclosed in `docs/agent.md` and in a Q-0021-NN entry. Evidence: a table in the evidence record from each difference to the code and the test that covers it.
4. **Chat completions are unchanged.** For a chat-completions endpoint, the requests the hosts send are the same as before this goal, apart from changes the Pi bump makes, which are listed. Evidence: the existing provider, tutor and agent smokes pass unchanged, and a recorded request-body comparison before and after.
5. **The Pi packages are current.** All four direct `@earendil-works` dependencies move together to the newest version the corporate feed serves, at least 1.6.0, with the lockfile resolved from that feed. The run checks what 1.6.0's system-role change does to the durable host's requests and shows the result. Evidence: the lockfile diff, the feed's version listing, and the agent and package smokes on all three OSes.
6. **The learner is told the truth.** `docs/agent.md`, `docs/install.md`, `docs/install.zh-CN.md` and the settings text say that both formats work and how the app picks one. Evidence: `docs-links`, `install-guide`, and the owner's review.

Constraints:

- Use only the corporate npm feed. Never switch the registry, and never move any package to an older version than the lock holds.
- No telemetry. Pi's telemetry package is out of scope here and must not be turned on.
- No change to scoring, consent, the database schema or the internal-only source rules. Nothing derived from the source PDF reaches a provider except where a recorded consent event already allows it today.
- Streaming, tool calls and new model parameters are not added.

Stop and report at once if, at the start, something only the owner can supply is missing. A feed that does not yet serve 1.6.0 is not a reason to stop: finish items 1–4 and 6 on the current packages, then report item 5 as blocked by the feed, with the feed's version listing as evidence. Take reversible defaults and disclose them in `docs/decisions/OPEN-QUESTIONS.md` (Q-0021-NN). The expected default is that the endpoint path selects the format, with no separate setting. Stop only for an irreversible or out-of-bounds decision. End with a report that pairs each item above with its evidence and lists the known limits.

## Out of scope

- Telemetry of any kind.
- Providers that speak neither format (Anthropic's native API, Gemini's native API).
- Moving the command line's data or Node runtime (goal 0020).
