# AGENTS.md — pointers for coding agents

This file exists because coding agents read `AGENTS.md` by convention. It points at the authority and
restates nothing: the documents below win if they disagree with this page.

Read, in order:

1. [`docs/core-design.md`](docs/core-design.md) — purpose, truth ownership, trust boundaries, non-goals.
2. [`docs/conventions.md`](docs/conventions.md) — binding conventions: stack, module boundaries, what must not be silently replaced.
3. [`docs/harness.md`](docs/harness.md) — gates, CI matrix, and how the harness is expected to grow.
4. [`docs/goals/README.md`](docs/goals/README.md) — the numbered outcome series and its constraints.
5. [`docs/decisions/OPEN-QUESTIONS.md`](docs/decisions/OPEN-QUESTIONS.md) — owner decisions and the defaults taken under uncertainty.

Before adding a path, a shell invocation or any OS-specific behaviour, read
[`docs/cross-platform.md`](docs/cross-platform.md).

## Non-negotiables

- Never commit, bundle or export the internal-only source PDF or its derived text, and never send it
  to a public provider without a recorded consent event.
- Never weaken or delete a gate, a test, an evidence row, a disclosed limit or a contract statement
  to make CI pass. Fix the cause.
- A goal is not done because its code exists; it is done when its pull request is merged on a
  mechanically green gate.
- Hardware, credentials, provider access and the learner's real budget are owner inputs. Take a
  disclosed default, record it in `OPEN-QUESTIONS.md`, and say so — never invent one silently.
