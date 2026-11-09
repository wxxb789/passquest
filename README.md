# Pass Quest

An AI-native, desktop-first, local-first exam coach focused on **passing a particular written exam within a limited preparation window**, not generic language proficiency.

## Design and development

- [Core Design](docs/core-design.md) — product purpose, safety, quality and architecture principles.
- [Goal series](docs/goals/README.md) — numbered, observable development outcomes. Start with **0001**, and refine provisional later goals before activation.

**Implementation order:** verified headless practice → evidence-driven adaptation and durable AI → Anki integration and on-demand resources → functional MyGo desktop → Opus-led high-fidelity UX → packaging.

## Confidential baseline material

The original 165-page CRSC preparation PDF states *仅限内部使用，严禁外传*. At the project owner's explicit direction, an **unreviewed partial text-layer transcription** is present at [`data/baselines/2025-crsc-english-partial.txt`](data/baselines/2025-crsc-english-partial.txt). This repository is **public**, regardless of the app's intended internal use; the file therefore is publicly accessible. It only covers 22 pages with meaningful native text; the remaining scanned pages are placeholders, and extracted text/answers have not been verified. This commit is not evidence of redistribution rights or permission for agents to upload additional source material. The complete source PDF and future OCR outputs stay local by default, and tests must use independently authored or otherwise redistributable exercises.

No API keys or personal study history belong in this repository.
