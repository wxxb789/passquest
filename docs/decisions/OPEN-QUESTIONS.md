# Open questions

Owner-only decisions, unresolved concerns and defaults taken under the uncertainty protocol in
[`../conventions.md`](../conventions.md). Agent runs **append** here instead of blocking; the owner
resolves at the next intervention. Never delete an entry. Status values: `open`, `decided(default
taken)`, `resolved`.

Entry format:

```markdown
### Q-<goal-id>-<nn> — <one-line question>
- Status: open
- Context: what forced the question
- Options considered: A / B / C
- Default taken: what the run did, and how to reverse it
- Revisit when: the trigger
```

---

### Q-0000-01 — Unattended merge authorization for goals 0001–0010
- Status: decided(default taken)
- Context: the owner asked for unattended development of goals 0001–0010 through the goal-driven
  delivery flow. That flow ends in a squash merge, which is a write to `main` on a public repository.
- Options considered: (A) merge each goal when the merge gate is green; (B) leave every PR open for
  the owner; (C) merge only after the owner reviews the first goal.
- Default taken: (A) for this goal range only, and only when `scripts/pr-merge-check.py` reports the
  head verified, CI green, the codex marker `has_any_findings: false` on that same head, zero
  unresolved threads, and local gates re-run. Any failure leaves the PR open and the goal blocked.
  Revoke by saying so; the flow then stops at "PR open".
- Revisit when: the owner returns, or a merge gate produces a false green.

### Q-0000-02 — Project instruction file location
- Status: open
- Context: `AGENTS.md` at the repository root is the file coding agents read first by convention. Its
  creation required an interactive approval that was unavailable during the unattended run, so the
  binding conventions currently live in `docs/conventions.md`.
- Options considered: (A) create `AGENTS.md` as a thin pointer to `docs/conventions.md`; (B) keep
  `docs/conventions.md` and pass it explicitly in every dispatch prompt; (C) fold conventions into
  `docs/core-design.md`.
- Default taken: (B) for now; dispatch prompts name `docs/conventions.md` and `docs/harness.md`.
- Revisit when: the owner is present — one approval creates the pointer file.

### Q-0000-03 — Toolchain pinning
- Status: decided(default taken)
- Context: the host provides Go through `mise`; a contributor without mise needs a documented floor.
- Options considered: (A) pin in `mise.toml` plus `go.mod` directive; (B) a container/CI-only
  toolchain; (C) no pin.
- Default taken: (A) — `mise.toml` pins Go and Node for the dev host, `go.mod` is the authority for
  CI and for the language version.
- Revisit when: a second machine or a contributor joins.

### Q-0000-04 — Two agent loops are writing the same worktree and branch
- Status: resolved
- Resolution: (owner, 2026-11-10) the second loop is confirmed stopped — no process with a
  `~/repos/passquest` cwd, no `omp-passquest-*` tmux session, no `.git` lock, and the last foreign
  touch was the 01:20:56 commit; a single writer (the delivery loop) resumed at the `apply` stage by
  instruction. Option (A) file-scope splitting is therefore unnecessary, and the fetch-then-rebase
  guard in `conductor.py` stays as the standing protection against a recurrence.
- Context: while the agent loop was running goal 0001, two owner commits appeared on the same branch
  in the same clone (`5e89188` D-0001 Bun/TS harness, `4a861ca` parsed private source). The reflog
  shows a `reset --hard` + two commits + push from another process at the same second, i.e. a second
  agent/loop edits `~/repos/passquest` directly. Two writers in one worktree can interleave stage
  output and make a reviewed head unreproducible.
- Options considered: (A) split file scopes — the other loop owns `data/`, `docs/decisions/`,
  `harness/`; the delivery loop owns `cmd/`, `internal/`, `docs/goals|plans|evidence`, both
  fetch-then-commit and stop on conflict; (B) the other loop works only in its own clone/worktree;
  (C) the delivery loop stops until the owner finishes.
- Default taken: (C) temporarily — the unattended conductor was stopped and the in-flight `apply` run
  killed before it could commit. The working tree is clean at `4a861ca` and `bash scripts/gate.sh`
  still reports `GATE: PASS` (45 links, 214 tracked files). The conductor now fetches and rebases
  before every commit and push, blocks on a real conflict, and never force-pushes, so it can be
  resumed safely under (A).
- Revisit when: the owner answers, or a second concurrent commit appears while the conductor runs.

### Q-0001-01 — May a learner repeat a question after seeing the key, and how is it counted?
- Status: decided(default taken)
- Context: goal 0001 requires correct, incorrect, skipped and repeated attempts to be represented
  truthfully, but does not say whether repeats are allowed or how they relate to independent
  evidence. Goal 0003 (tactics) and goal 0009 (readiness) will interpret the same records.
- Options considered: (A) allow repeats, keep every attempt, label repeats and exclude them from
  first-attempt counts; (B) block a question once its key has been shown; (C) allow repeats and count
  every attempt equally.
- Default taken: (A). The CLI stores the repeat with `attempt_number`, `prior_attempts` and
  `repeat`, and `report` prints first and repeated counts separately. Reverse by changing the
  selection/report policy only; stored attempts stay intact. The Store API and SQLite triggers
  reject `UPDATE`, `DELETE` and an id-reusing `INSERT OR REPLACE`, so a future policy can be
  recomputed from the same evidence.
- Revisit when: goal 0004 selects practice from evidence, or goal 0009 defines independent
  readiness and needs a stricter exposure rule.
