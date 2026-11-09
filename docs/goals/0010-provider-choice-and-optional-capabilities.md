# 0010 — Learners can choose supported models without losing study flow

**State:** Provisional
**Prerequisite:** 0005 — BYOK text tutor

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Offer realistic model/provider choice for OpenAI, Qwen, Doubao, DeepSeek, Gemini and a custom compatible endpoint. Treat provider, model, capability and authentication as different things. Account-based authorization is supported only via documented eligible channels; never use unofficial subscription-token extraction.

## Provisional basis for acceptance
- A user can select among real supported text providers and authenticate with authorized BYOK credentials without exposing them in application data or exported content.
- Per-model capability and usable authorization are accurately represented, and a model/provider failure can be understood or changed without losing practice state.
- Optional web search, speech and image features are discoverable and only offered where authorized; absence of these does not prevent core text coaching.
- Usage, external data exposure and applicable budget are intelligible.

**Requires refinement before activation:** Refine current API availability, official account-login conditions and provider-specific test contracts at activation; do not promise all extras on all providers.

## Execution boundary

At run start, stop/report if an input only the user can supply makes the specified outcome impossible. Otherwise select reversible defaults and disclose them. Stop for irreversible/unauthorized actions; do not invent a missing exam passing score or publish restricted text. Show real behavior with evidence and explicit limitations, not only a diagram, unit stub, screenshot or progress report. Pair each acceptance condition with its evidence at completion.
