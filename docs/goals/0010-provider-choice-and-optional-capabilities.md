# 0010 — Learners can choose supported models without losing study flow

**State:** Active
**Prerequisite:** 0005 — BYOK text tutor

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Offer realistic model/provider choice for OpenAI, Qwen, Doubao, DeepSeek, Gemini and a custom compatible endpoint. Treat provider, model, capability and authentication as different things. Account-based authorization is supported only via documented eligible channels; never use unofficial subscription-token extraction.

## Refined contract (activated 2026-11-12)

The increment is a **registry-driven provider and model choice** that keeps practice state intact, states a
failure in one line, and never presents an unverified capability as working. Goal 0005 already provides BYOK
credential resolution, one documented request shape, a Node host as the only egress, a per-day call budget
with reservations, consent gating and typed failures; goal 0008 provides one bounded search adapter. What does
not exist is a registry, a capability matrix, or a way to switch provider without hand-editing configuration.

**In scope**

1. **Registry.** A versioned, in-repo data file describing supported providers: id, display name, endpoint
   shape, models, and per-model declared capabilities (`text.chat`, `tool.calls`, `vision`, `search`). Adding
   a provider or model is a data edit plus a test, not a vendor SDK.
2. **Four-way separation.** `passquest model list` prints provider ≠ model ≠ capability ≠ authorization state.
   A model whose credential is absent prints `no_credential`, never "unsupported".
3. **Switch without losing state.** `passquest model use <provider>/<model>` writes one local setting. An
   in-flight `agent` run keeps the model it started with (goal 0006 rule) and is never silently upgraded
   mid-run; resume reports the recorded model.
4. **Verified, not claimed.** `passquest model test <provider>/<model>` performs one minimal call and records
   `verified` or `failed:<type>` with a timestamp. The list renders verified and unverified capabilities
   differently, and an unverified capability is never offered as working.
5. **Typed failures with a next action.** `no_credential`, `unauthorized`, `rate_limited`, `model_missing`,
   `endpoint_unreachable`, `invalid_response` — each names the one action the learner can take. A failure
   leaves practice state untouched.
6. **Budget and exposure stay intelligible.** The per-day call budget is counted **per provider**, and the
   report prints both the per-provider counters and their sum, so the total ceiling is readable as
   `providers × per-provider limit` instead of looking like one global cap.
7. **Optional capabilities are opt-in.** Search (0008), speech and images are discoverable, offered only where
   a provider is verified for them, and their absence never blocks text coaching.
8. **Bounded automatic failover.** When a call fails with a retryable transport or quota type, the host may
   retry once on another provider **only if** all of the following hold: the target model is verified for
   every capability the request needs; the target's recorded consent scope is exactly equal to the failing
   request's consent scope (same sources, same grant set); the target still has budget left; and the target
   is not already the provider that failed. Each failover prints `failover: <from> → <to> (reason: <type>)` and
   is recorded on the call ledger. Any consent difference — including a target that would receive private
   text the failing provider was allowed to receive — refuses failover with the reason.

**Out of scope**

Unofficial subscription-token extraction or any account-sharing trick; automatic failover across different
consent scopes; per-provider price claims or model-quality rankings (both change faster than this contract);
account-based login (Q-0010-02).

**Defaults taken (reversible, disclosed)**

- Real credentials: none in this environment. The registry, the loopback stub and the typed failures are
  proven without a key; a real call is owner evidence recorded in the report (Q-0010-01).
- Account login: not supported in this goal; BYOK keys only (Q-0010-02).
- Supported models: one cheapest documented text model per provider, with every other model listed
  `declared, unverified` (Q-0010-03).
- Budget: one counter **per provider**, each at the goal-0005 daily value, with the sum printed (Q-0010-04).
- Failover: bounded automatic failover under clause 8, never across differing consent scopes (Q-0010-05).

## Acceptance and evidence

| # | Acceptance | Evidence |
|---|---|---|
| 1 | The registry drives behaviour | `model list` output derived from the registry file; a test adds an entry and sees it appear |
| 2 | The four axes stay separate | listing with no key set prints `no_credential` rows while the provider and model still appear |
| 3 | Switching is local and reversible | `model use` then `model list` shows the active model; `practice`/`next` state identical before and after (`cmp`) |
| 4 | Verification is real | `model test` against a loopback stub records `verified`; against a dead endpoint records `endpoint_unreachable`, both with timestamps |
| 5 | Failures are typed and actionable | one stub-driven transcript per failure type, each naming the next action |
| 6 | A run keeps its model | start an `agent` run, `model use` another, resume: resume reports the recorded model |
| 7 | Credentials never leak | exported content and `model list` scanned for key material; the secrets gate stays green |
| 8 | Optional features cannot block text | with search and speech unverified, `tutor` and `practice` still succeed |
| 9 | Budget is per provider and the total is visible | counters for two providers, a refusal at each provider's own limit, and the printed sum |
| 10 | Failover respects consent | stub run: retryable failure on provider A with a same-consent verified provider B fails over and prints the line; the same failure with a differing consent scope refuses and prints why |

The smoke gate uses a loopback provider stub, as `tutor smoke`, `note smoke` and `find smoke` already do. A
real provider run is owner-supplied evidence and stays unverified until the owner records it.

## Known limits

No real provider is contacted here; CI cannot reach one from any OS, so capability claims are
registry-declared until a `model test` actually runs on the owner's machine. The registry is maintained by
hand, so a provider that changes its API fails as `invalid_response` until the file is updated. Budget
accounting is per local database, not per account, and per-provider counters mean the real ceiling is the sum,
not one number. Failover can only be as precise as the recorded consent scope; a source granted for two
providers is reachable by both, and this contract does not claim otherwise.

## Execution boundary

At run start, stop/report if an input only the user can supply makes the specified outcome impossible. Otherwise select reversible defaults and disclose them. Stop for irreversible/unauthorized actions; do not invent a missing exam passing score or publish restricted text. Show real behavior with evidence and explicit limitations, not only a diagram, unit stub, screenshot or progress report. Pair each acceptance condition with its evidence at completion.
