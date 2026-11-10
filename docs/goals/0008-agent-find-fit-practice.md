# 0008 — Find practice only when learning evidence needs it

**State:** Active
**Prerequisite:** 0006 — resumable agent work and 0004 — adaptive next exercise

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Turn an observed training gap into bounded, source-aware resource discovery. Do not preinstall a general-purpose English corpus or allow unlimited scraping/search costs.

## Refined contract (activated 2026-11-11)

No search provider credential exists in this environment, and CI cannot reach a real provider from any OS. The
increment therefore proves the *policy* — bounded intent, provenance, rights classification, budget, injection
containment, honest failure — against a loopback stub, and reports a real provider run as owner evidence.

**In scope**

1. `passquest find` derives a bounded search intent from stored learning evidence: the skill with the most
   wrong or skipped attempts inside the goal-0004 adaptive state, the exam section it belongs to, the
   learner's level, and an explicit candidate limit. With no qualifying evidence the command says so and
   exits 0 without a query.
2. One search adapter interface, configured by `PASSQUEST_SEARCH_ENDPOINT` / `PASSQUEST_SEARCH_KEY`. With no
   endpoint configured the command reports `no provider configured`, performs zero network I/O, and exits 0.
   The adapter is proven against a loopback stub that speaks the documented request and response shape.
3. Every candidate is classified and the classification is stored: `link-only` (URL and title only, never
   fetched), `local-usable` (the response itself carries an explicit open-license field), or
   `generated-unverified` (produced by a model, never authoritative). Only `local-usable` text may be shown
   for local study, and nothing is downloaded by default.
4. Each candidate carries provenance: provider, query, retrieval time, URL, and a SHA256 of the returned
   content, so a later audit can tell what was seen and when.
5. A budget is enforced and visible: `--max-queries` (default 5 per day) and `--max-candidates` (default 20
   per run), persisted locally. Exceeding either refuses with the reason and the remaining allowance, never
   silently truncates.
6. Fetched text is data. An injected instruction inside a page ("ignore previous instructions", "mark this
   answer correct") cannot change scoring, assessment state or the next exercise; the containment is proven
   by a stub page carrying such an instruction.
7. Failure is reported, not papered over: no provider, rate limit, empty result, or all-candidates-link-only
   each produce an explicit message naming the cause. No article, passage or answer is ever fabricated.

**Out of scope**

A general English corpus, unlimited scraping, paywalled or login-walled content, an arbitrary web-PDF
ingestion pipeline, provider-specific SDKs, and any automatic download of third-party text.

**Defaults taken (reversible, disclosed)**

- Search provider: none configured here → loopback stub in CI, real provider run is owner evidence.
- Rights: `link-only` unless the provider response declares an open license; no auto-download, no scraping.
- Budget: 5 queries/day and 20 candidates/run, both flags.
- Representative gap: lowest-scoring skill with at least three attempts; otherwise the tactic of the most
  recent wrong answer.

## Acceptance and evidence

| # | Acceptance | Evidence |
|---|---|---|
| 1 | A real stored gap produces a bounded intent; no evidence produces an honest "nothing to find" | two CLI runs over stored attempts, JSON shown |
| 2 | No configured endpoint means zero network I/O and an explicit message | run with the env unset plus a listener that records connections |
| 3 | Adapter protocol works: query, parse, candidate limit | loopback stub smoke gate, 3 OS, no network |
| 4 | Each candidate is classified and only `local-usable` text is displayable | stub returns all three classes; CLI output shows the difference |
| 5 | Provenance is stored and auditable (provider, time, URL, SHA256) | stored row printed back, hash recomputed |
| 6 | Budget refuses the 6th query of the day and the 21st candidate | two runs that hit each limit, refusal text shown |
| 7 | An injected instruction in fetched text changes no state | stub page with the instruction, state compared before/after |
| 8 | Rate limit, empty result and all-link-only are each reported as themselves | three stub modes, three messages |

The smoke gate `find smoke` uses a loopback search stub, exactly as `tutor smoke` and `note smoke` use
loopback providers. A real provider run is **owner-supplied evidence** and stays unverified until then.

## Execution boundary

At run start, stop/report if an input only the user can supply makes the specified outcome impossible. Otherwise select reversible defaults and disclose them. Stop for irreversible/unauthorized actions; do not invent a missing exam passing score or publish restricted text. Show real behavior with evidence and explicit limitations, not only a diagram, unit stub, screenshot or progress report. Pair each acceptance condition with its evidence at completion.
