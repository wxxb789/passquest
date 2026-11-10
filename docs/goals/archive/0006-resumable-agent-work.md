# 0006 — Recover useful agent work after interruption

**State:** Ready
**Prerequisite:** 0005 — BYOK tutor

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Make the existing tutor able to carry forward a bounded multi-step teaching or content-analysis task across a local process restart, using the intended Pi Durable agent runtime. This is an **end-user continuity outcome**, not a goal to build infrastructure for its own sake.

## Done when
- A real, nontrivial AI task can be started, interrupted and resumed from persisted state; its prior context and observable output remain coherent.
- The user can tell if work is running, paused, completed, failed or needs action, and can cancel work without leaving invisible uncontrolled execution.
- At least one side-effecting operation is interrupted/retried without duplicate committed learner evidence or duplicate content, with reconciliation evidence.
- Verified scoring/history remain owned by the application and the durable agent history remains independently recoverable; core practice still works when the agent host is absent.

## Execution boundary

At run start, stop/report if an input only the user can supply makes the specified outcome impossible. Otherwise select reversible defaults and disclose them. Stop for irreversible/unauthorized actions; do not invent a missing exam passing score or publish restricted text. Show real behavior with evidence and explicit limitations, not only a diagram, unit stub, screenshot or progress report. Pair each acceptance condition with its evidence at completion.
