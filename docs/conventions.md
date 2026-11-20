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
- **Node/TypeScript** exists only for the narrow hosts under `agent/`: the single-call BYOK tutor
  (goal 0005), the bounded search transport (goal 0008), the Pi Durable study brief (goal 0006) and
  the loopback AnkiConnect transport (goal 0007). The core CLI, note drafting/preview/accept/export
  and every verified local practice path must work with Node absent; only `tutor ask`, `agent` work,
  `note propose --refine`, `note push` when it has pending work, and `find` with a configured search
  endpoint need it.
- **MyGo + WebView** is the desktop shell (`cmd/passquest-desktop`, `internal/desktop`, the TypeScript
  frontend in `desktop/web`) beside the scriptable CLI; the gates drive both, and screenshots and mock
  UIs are not evidence.
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
cmd/passquest/          CLI entry point
cmd/passquest-desktop/  desktop app entry point (the CLI when given arguments)
internal/<domain>/      Go packages by domain truth, not by layer
internal/study/         orchestration per learner workflow (practice, recommend, models, coach, notes,
                        outbox, assessment, brief and the shared kernel), imported by both adapters
internal/cli/           Go adapter for the CLI surface
internal/workspace/     Go adapter for the desktop Workspace surface
internal/studio/        DTOs of the desktop's bound API
internal/desktop/       MyGo app, data directory, credential store and lifecycle
internal/update/        the on-demand release check
internal/cmdline/       shared by both adapters: shell detection/quoting and the generated command
                        builders
desktop/web/            desktop frontend source (TypeScript + CSS)
agent/                  Node/TypeScript tutor, search, AnkiConnect and Pi Durable hosts (`docs/agent.md`)
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

`internal/study/<workflow>/` orchestrates one learner workflow each and is cut by workflow, not by
layer; `internal/cli/` and `internal/workspace/` adapt that orchestration to the two delivery
surfaces over one contract. They are the only layer-shaped packages in the tree; `internal/cmdline/`
is the shell-quoting and command-builder package both adapters share. The local-only gate
(`BOUNDARIES` in `harness/gates/local-only.ts`) enforces this layout over direct imports only:
`internal/study` may not import `internal/cli`, `internal/studio`, `internal/workspace` or
`internal/desktop`; `internal/workspace` and `internal/desktop` may not import `internal/cli`;
`internal/cli` may not import `internal/workspace`, `internal/desktop` or `internal/studio`; and
`internal/cmdline` may not import `internal/cli`, `internal/studio`, `internal/workspace`,
`internal/desktop` or `internal/store`.

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
- Commits: conventional prefix (`feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `chore:`), imperative,
  one increment per branch. Agent runs do **not** commit or push — the operator does.
- Every gate you report carries its real exit code. A gate you did not run is missing work.
- New capability ⇒ new machine check in the same increment. New checks are TypeScript gate modules
  under `harness/gates/` sharing `harness/lib/`, with an input that must fail — a case in
  `harness/gates/self-test/<gate module>.cases.ts` listed in `brokenCases()` in
  `harness/gates/self-test.ts` — never another `scripts/*.sh` or `scripts/*.py` one-off.
  `bun run gate` is the single definition; `bash scripts/gate.sh` only execs it. See
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
