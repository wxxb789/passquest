# Pass Quest

An AI-native, desktop-first, local-first exam coach focused on **passing a particular written exam within a limited preparation window**, not generic language proficiency.

## Design and development

- [Core Design](docs/core-design.md) — product purpose, safety, quality and architecture principles.
- [Goal series](docs/goals/README.md) — numbered, observable development outcomes. Start with **0001**, and refine provisional later goals before activation.

**Implementation order:** verified headless practice → evidence-driven adaptation and durable AI → Anki integration and on-demand resources → functional MyGo desktop → Opus-led high-fidelity UX → packaging.

## Try the first practice slice

The first slice needs Go 1.27 only: no Node, network, LLM, Anki or private PDF.

```bash
export PATH="$HOME/.local/share/mise/shims:$PATH"
go run ./cmd/passquest practice
go run ./cmd/passquest report
```

`practice` shows a self-authored reading exercise without its answer, records the attempt in
`.passquest/passquest.sqlite` (gitignored), and then shows the keyed option, the passage evidence and
a short explanation. `report` reopens the same database in a new process and prints counts split
into first and repeated attempts. The sample is **not** live exam content, and no exam score, pass
threshold or readiness probability is computed. Design and evidence:
[`docs/evidence/0001-first-independent-exam-attempt.md`](docs/evidence/0001-first-independent-exam-attempt.md).

## Confidential baseline material

The original 165-page CRSC preparation PDF states *仅限内部使用，严禁外传*. At the project owner's explicit direction, two **unreviewed** text extractions are present in this repository:

- [`data/baselines/2025-crsc-english-partial.txt`](data/baselines/2025-crsc-english-partial.txt) — the earlier 22-page native-text extraction, kept as history.
- [`data/baselines/2026-crsc-english/`](data/baselines/2026-crsc-english/README.md) — the page-linked parsed text of all 165 pages (22 native-text pages plus 143 OCR pages), authorised by the owner on 2026-11-10. Its `README.md` records the parse method, the per-page provenance in `manifest.tsv`, and the visibility boundary.

This repository is **public**, regardless of the app's intended internal use; both extractions are therefore publicly accessible. Extracted text, OCR and answer keys have not been verified. Publication is not evidence of redistribution rights: the original PDF stays at `data/private/source/` (gitignored), no further OCR or new source material is published automatically, no text may be uploaded to an external provider without a new recorded decision, and tests must use independently authored or clearly licensed fixtures — never this material.

No API keys or personal study history belong in this repository.
