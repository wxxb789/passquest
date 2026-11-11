# Pass Quest

An AI-native, desktop-first, local-first exam coach focused on **passing a particular written exam within a limited preparation window**, not generic language proficiency.

## Design and development

- [Core Design](docs/core-design.md) — product purpose, safety, quality and architecture principles.
- [Goal series](docs/goals/README.md) — numbered, observable development outcomes. Goals 0001–0008
  are done; 0009 is next.
- [Harness](docs/harness.md) — the single `bun run gate` entry point and what each gate reads.
- [Cross-platform rules](docs/cross-platform.md) — Windows, macOS and Linux: paths, processes, mode
  bits, goldens, and the path literals the `portable-paths` gate refuses.
- [BYOK text tutor](docs/tutor.md) — one user-supplied text endpoint, advisory coaching, private
  source consent and credential handling.
- [Durable agent work](docs/agent.md) — a bounded multi-step study brief on the Pi Durable runtime,
  with pause/resume, cancellation, idempotent steps and an independent durable history.
- [Anki notes](docs/notes.md) — a deterministic note from a stored attempt or a learner mark,
  consent-gated private text, revision history, atomic export and a loopback AnkiConnect push.
- [Find practice](docs/find.md) — one evidence rule, a bounded search intent, candidate rights and
  provenance, a visible query/candidate budget and inert fetched text.

**Implementation order:** verified headless practice → private local baseline → scoring tactic
practice → evidence-driven adaptation → user-supplied text coaching and durable agent work → Anki
integration and on-demand resources → unseen assessment → functional MyGo desktop → Opus-led
high-fidelity UX → packaging.

## Try the practice slice

The shipped slice needs Go 1.27 only: no Node, network, LLM, Anki or private PDF.

```bash
export PATH="$HOME/.local/share/mise/shims:$PATH"
go run ./cmd/passquest practice
go run ./cmd/passquest report
```

`practice` shows a self-authored reading exercise without its answer, records the attempt in
`.passquest/passquest.sqlite` (gitignored), and then shows the keyed option, the passage evidence and
a short explanation. `report` reopens the same database in a new process and prints counts split
into first/repeated attempts and assisted/independent conditions. The sample is **not** live exam
content, and no exam score, pass threshold or readiness probability is computed. Evidence:
[`docs/evidence/0001-first-independent-exam-attempt.md`](docs/evidence/0001-first-independent-exam-attempt.md).

## Import your private source

Goal 0002 adds a page-linked local import that never uploads source text and never treats unreviewed
text as an answer key:

```bash
go run ./cmd/passquest import --path data/baselines/2026-crsc-english
go run ./cmd/passquest sources
go run ./cmd/passquest show SOURCE --page 24
```

A native text page, an unreviewed OCR page and a scan with no text layer are visibly different.
Correct a page, review it, add a question, review its answer, and practice it through the same
deterministic scoring as the sample. TXT directories, single TXT files and PDF text layers are
accepted; PDF extraction uses a local `pdftotext` and no OCR. Full commands, states and limits:
[`docs/source-import.md`](docs/source-import.md). Measured evidence:
[`docs/evidence/0002-local-text-baseline.md`](docs/evidence/0002-local-text-baseline.md).

## Practice a scoring tactic

Goal 0003 adds a guided-then-independent NOT/EXCEPT tactic. The guided attempt shows the walkthrough
and is recorded as **assisted**; the independent question is a different, never-exposed, unattempted,
answer-verified question with no hint, and its correctness is counted separately.

```bash
go run ./cmd/passquest tactic list
go run ./cmd/passquest tactic show not-except-polarity
go run ./cmd/passquest tactic practice not-except-polarity --stage guided
go run ./cmd/passquest tactic practice not-except-polarity --stage independent
go run ./cmd/passquest report
```

The lesson states why the tactic works, when it applies, and a counterexample showing why keyword
matching and the "longest answer" folklore fail. Imported source questions opt into a skill with a
`tactics` tag in their local question file; only reviewed, tagged questions are used. Details:
[`docs/tactics.md`](docs/tactics.md). Measured evidence:
[`docs/evidence/0003-exam-tactic-practice.md`](docs/evidence/0003-exam-tactic-practice.md).

## Choose practice from evidence

Goal 0004 adds a small skill set (the NOT/EXCEPT tactic plus vocabulary-in-context and
sentence-structure prerequisites) and a read-only recommendation surface:

```bash
go run ./cmd/passquest skill list
go run ./cmd/passquest skill practice vocabulary-in-context
go run ./cmd/passquest next
go run ./cmd/passquest next --json
```

`next` separates recorded evidence from inferred state. A first independent attempt on a fresh
passage counts as independent transfer; assisted attempts, repeats and a new question on an
already-seen passage are shown but never counted as transfer. Time, hints and revisits can change the
recommended action between `explain`, `practice_new` and `review`, and the reason names the counts.
Recommendations are recomputed from attempts; nothing derived is stored. Details:
[`docs/adaptive.md`](docs/adaptive.md). Measured evidence:
[`docs/evidence/0004-adaptive-next-exercise.md`](docs/evidence/0004-adaptive-next-exercise.md).

## Ask your own text model for coaching

Goal 0005 adds one optional BYOK text endpoint. The model can explain a verified answer, probe a
misconception or give a tactical micro-lesson for one recorded attempt. Its text is labelled
advisory and never changes an attempt, grade or answer key. Node is used only by the narrow host that
makes the HTTPS call; practice, review, `next`, `report`, configuration, preview and history work
without it.

```bash
export PASSQUEST_TUTOR_API_KEY=...   # never stored by Pass Quest
go run ./cmd/passquest tutor config --endpoint https://api.example.com/v1/chat/completions --model MODEL
go run ./cmd/passquest tutor preview --attempt 1
go run ./cmd/passquest tutor ask --attempt 1
go run ./cmd/passquest tutor history
```

Imported private source text is withheld from the provider until the learner inspects the preview
and records `tutor consent SOURCE --reviewer NAME`; first-party exercises need no consent.
`revoke` appends a withholding event. Credentials are read only from `PASSQUEST_TUTOR_API_KEY` and
are redacted from output and stored call records. Details: [`docs/tutor.md`](docs/tutor.md).
Measured evidence: [`docs/evidence/0005-bring-your-own-text-tutor.md`](docs/evidence/0005-bring-your-own-text-tutor.md).

## Resume agent work after interruption

Goal 0006 adds one bounded multi-step `study-brief` on the real Pi Durable runtime. It builds a
privacy-bounded evidence packet from recorded attempts and the adaptive policy, records up to six
advisory steps through an idempotent application-owned effect, and can be paused, resumed or
cancelled without duplicating content.

```bash
bun install                          # pinned Pi Durable workspace under agent/
export PASSQUEST_TUTOR_API_KEY=...   # the host reads it; Pass Quest never stores it
passquest agent start                # complete a bounded brief
passquest agent status               # running / paused / completed / failed / needs action
passquest agent cancel RUN           # stop a live host and refuse resume
passquest agent resume RUN           # continue the same run and context
passquest agent show RUN             # steps, event trail, effect reconciliation
passquest agent transcript RUN       # the independent Pi history
```

The host is the only provider egress; Go owns the run record, the step commit and the verified
history. `practice`, `report`, `next` and the tutor keep working when Node or the host is absent.
Details: [`docs/agent.md`](docs/agent.md). Measured evidence:
[`docs/evidence/0006-resumable-agent-work.md`](docs/evidence/0006-resumable-agent-work.md).

## Turn study evidence into Anki notes

Goal 0007 turns one high-value study moment — an incorrect or skipped attempt, or an explicit mark —
into one concise local note, and then into an Anki card. The Go core needs no Node for the
deterministic draft, preview, accept, skip, show, export or the model view; `note propose --refine`
uses the tutor host, and only `note push` with pending work spawns the narrow Anki host.

```bash
passquest note propose --attempt 1
passquest note preview                    # exact field HTML; private text withheld until consent
passquest tutor consent SOURCE --reviewer NAME
passquest note accept                     # revision 1; repeat is a no-op, an edit bumps it
passquest note show
passquest note export                     # atomic HTML/TSV fallback
passquest note push                       # loopback AnkiConnect
passquest note model                      # deck, model, CSS, template, rendered sample
```

The note model is `PassQuest Note` in deck `Pass Quest` with fields `Front`, `Back`, `Tactic`,
`Source`. Duplicates are reported as already-present, revisions update the same Anki note id, a
learner edit is a visible conflict rather than a silent overwrite, and an unreachable endpoint leaves
the note queued for exactly one later drain. A skipped attempt and an unanswered mark never include
the correct option or its explanation. AnkiWeb and mobile synchronization remain the learner's own
manual action; Pass Quest never contacts AnkiWeb or a phone. Details: [`docs/notes.md`](docs/notes.md).
Measured evidence: [`docs/evidence/0007-anki-from-learning.md`](docs/evidence/0007-anki-from-learning.md).

## Find practice when evidence needs it

Goal 0008 turns one observed gap into a bounded search intent. The rule is fixed: the highest
`(incorrect + skipped) / attempts` among skills with at least three attempts, else the most recent
wrong answer's tactic, else `nothing_to_find`. Candidate classification decides what may be shown:
`local-usable` only when the provider declares a recognised open license and returns text;
`generated-unverified` is advisory; everything else is `link-only`. Every candidate keeps provider,
query, retrieval time, URL and a content SHA-256. A query budget (5/day) and a candidate budget
(20/run) are flags and are persisted; exceeding either refuses instead of truncating.

```bash
passquest find --json
passquest find history
passquest find show 1
```

With no `PASSQUEST_SEARCH_ENDPOINT` and an intent derived from stored evidence, the command prints
`no provider configured` and makes zero network calls; an empty database reports `nothing_to_find`
first. Fetched text is stored as data only: it is never an instruction, never sent to a model,
and cannot change an attempt, grade or the next exercise. Details: [`docs/find.md`](docs/find.md).
Measured evidence: [`docs/evidence/0008-find-fit-practice.md`](docs/evidence/0008-find-fit-practice.md).

## Run the gates

```bash
bun install
bun run gate          # the single definition; bash scripts/gate.sh is a shim
```

## Confidential baseline material

The original 165-page CRSC preparation PDF states *仅限内部使用，严禁外传*. At the project owner's explicit direction, two **unreviewed** text extractions are present in this repository:

- [`data/baselines/2025-crsc-english-partial.txt`](data/baselines/2025-crsc-english-partial.txt) — the earlier 22-page native-text extraction, kept as history.
- [`data/baselines/2026-crsc-english/`](data/baselines/2026-crsc-english/README.md) — the page-linked parsed text of all 165 pages (22 native-text pages plus 143 OCR pages), authorised by the owner on 2026-11-10. Its `README.md` records the parse method, the per-page provenance in `manifest.tsv`, and the visibility boundary.

This repository is **public**, regardless of the app's intended internal use; both extractions are therefore publicly accessible. Extracted text, OCR and answer keys have not been verified. Publication is not evidence of redistribution rights: the original PDF stays at `data/private/source/` (gitignored), no further OCR or new source material is published automatically, no text may be uploaded to an external provider without a new recorded decision, and tests must use independently authored or clearly licensed fixtures — never this material. The import surface stores source text only in the local gitignored database and marks every imported page unreviewed.

No API keys or personal study history belong in this repository.
