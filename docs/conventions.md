# Conventions

Binding project conventions. Read this before writing code for any goal.

## Read order

1. [`core-design.md`](core-design.md) — product purpose, truth ownership, trust boundaries, non-goals.
2. [`goals/README.md`](goals/README.md) — the numbered outcome series and its constraints.
3. [`harness.md`](harness.md) — gates, CI, and how the harness is expected to grow.
4. This file.

## Stack (decided; do not silently replace)

- **Go 1.27** (`mise.toml` pins it; CI reads `go.mod`) is the deterministic application core: scoring,
  learning evidence, policy, import, SQLite persistence, Anki integration. Module
  `github.com/wxxb789/passquest`, entry point `cmd/passquest`.
- **SQLite** via a **pure-Go** driver (`modernc.org/sqlite`) — the project builds without cgo.
  One process is the writer/authority for each logical state domain; migrations are versioned and
  idempotent.
- **Node/TypeScript** exists only for the Pi Durable agent host (goal 0005+) under `agent/`. The core
  CLI and every verified local practice path must work with Node absent.
- **MyGo + WebView** desktop shell arrives at goal 0011. Until then the access surface is a real,
  scriptable CLI plus a test harness — screenshots and mock UIs are not evidence.
- **Bun + TypeScript** is the harness runtime (`harness/`, gate modules under `harness/gates/`), not
  a product runtime. Chosen for one reason: the owner develops on Linux, macOS and Windows and the
  harness must run on all three without Git Bash or a system Python. The migration landed with goal
  0002; see [`decisions/D-0001-bun-typescript-harness.md`](decisions/D-0001-bun-typescript-harness.md).
  Bun does not replace Go for product behaviour and does not enter `cmd/` or `internal/`.
- Dependency policy: standard library first; a new module needs a sentence in the plan about why the
  standard library is insufficient. No cgo, no embedded browser engine, no vector database, no cloud
  service.

## Truth ownership (product invariants that shape code)

- Verified answer keys and deterministic grading outrank any model output. AI text is advisory,
  labelled, and never written into answer keys or historical attempts.
- Attempts are append-only evidence. Derived learner state is recomputable and never overwrites
  attempts.
- Private source material (the CRSC PDF, OCR, answer keys, learner data) never leaves the machine
  without consent recorded in the run; tests use independently authored fixtures.
- External effects (Anki notes, provider calls, file exports) are idempotent, budgeted and reversible:
  stable idempotency keys, outbox, read-back reconciliation.

## Repository layout

```text
cmd/passquest/          CLI entry point (the pre-0011 access surface)
internal/<domain>/      Go packages by domain truth, not by layer
agent/                  Node/TypeScript Pi Durable host (from goal 0005)
data/baselines/         owner-directed public parsed text (unreviewed; see its README)
data/private/, data/local/, local-data/, .passquest/   never committed
docs/goals/             numbered contracts; docs/goals/archive/ after close
docs/plans/             per-increment plans
docs/evidence/          measured evidence records
docs/decisions/         OPEN-QUESTIONS.md and durable decisions
harness/                Bun/TypeScript gate runner, lib/ and one module per gate
scripts/                gate.sh compatibility shim only
testdata/               small, independently authored fixtures only
```

## Working rules

- Plans: `docs/plans/YYYY-MM-DD-HHmm-<goal-id>-<kebab-outcome>.md`. Historical plans are not
  contracts.
- Goal close, in the same commit as the code: the goal file moves **unchanged** to
  `docs/goals/archive/`, `docs/goals/README.md` state column updates, and any document made stale by
  the change is reconciled.
- Evidence records: `docs/evidence/<goal-id>-<topic>.md` — one table per tier, real commands, real
  numbers, limitations stated.
- Naming: kebab-case files; no `tmp/`, `misc/`, `notes/`. Tests sit next to the package
  (`*_test.go`); fixtures under `testdata/` and must be independently authored or clearly licensed.
- Commits: conventional prefix (`feat:`, `fix:`, `docs:`, `test:`, `chore:`), imperative, one
  increment per branch. Agent runs do **not** commit or push — the operator does.
- Every gate you report carries its real exit code. A gate you did not run is missing work.
- New capability ⇒ new machine check in the same increment. New checks are TypeScript gate modules
  under `harness/gates/` sharing `harness/lib/`, with an input in `harness/gates/self-test.ts` that
  must fail — never another `scripts/*.sh` or `scripts/*.py` one-off. `bun run gate` is the single
  definition; `bash scripts/gate.sh` only execs it. See
  [`harness.md`](harness.md) and [`decisions/D-0001-bun-typescript-harness.md`](decisions/D-0001-bun-typescript-harness.md).

## Uncertainty protocol (binding)

Do not block on a decision only the owner can make, and do not guess silently either:

1. Pick the most reversible default that satisfies the contract, disclose it in the plan and the
   completion report.
2. Append an entry to [`docs/decisions/OPEN-QUESTIONS.md`](decisions/OPEN-QUESTIONS.md) using the
   format already in that file: context, options, default taken, why, what to revisit. Never delete or
   rewrite someone else's entry; mark one `resolved` only with the owner's answer.
3. Stop and report only where the goal's **Execution boundary** says to: an input only the owner can
   supply makes the outcome impossible, an action is irreversible/unauthorized, or publishing
   restricted material would be required.

## Report format (end of every run)

1. Files written, grouped by area.
2. Gate results with the real command and exit code, including test counts.
3. Contract clause → evidence mapping (one line per "Done when" bullet).
4. Known limits and residual risks, with numbers rather than adjectives.
5. Open questions appended (ids only).
