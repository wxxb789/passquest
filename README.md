# Pass Quest

An AI-native, desktop-first, local-first exam coach focused on **passing a particular written exam within a limited preparation window**, not generic language proficiency.

## Design and development

- [Core Design](docs/core-design.md) — product purpose, safety, quality and architecture principles.
- [Goal series](docs/goals/README.md) — numbered, observable development outcomes. Goals 0001–0003
  are done; 0004 is next.
- [Harness](docs/harness.md) — the single `bun run gate` entry point and what each gate reads.

**Implementation order:** verified headless practice → private local baseline → scoring tactic
practice → evidence-driven adaptation and durable AI → Anki integration and on-demand resources →
functional MyGo desktop → Opus-led high-fidelity UX → packaging.

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
matching and the "longest answer" folklore fail. Imported source questions opt into a tactic with a
`tactics` tag in their local question file; only reviewed, tagged questions are used. Details:
[`docs/tactics.md`](docs/tactics.md). Measured evidence:
[`docs/evidence/0003-exam-tactic-practice.md`](docs/evidence/0003-exam-tactic-practice.md).

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
