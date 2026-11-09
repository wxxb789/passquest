# Pass Quest

An AI-native, desktop-first, local-first exam coach focused on **passing a particular written exam within a limited preparation window**, not generic language proficiency.

## Design and development

- [Core Design](docs/core-design.md) — product purpose, safety, quality and architecture principles.
- [Goal series](docs/goals/README.md) — numbered, observable development outcomes. Start with **0001**, and refine provisional later goals before activation.

**Implementation order:** verified headless practice → evidence-driven adaptation and durable AI → Anki integration and on-demand resources → functional MyGo desktop → Opus-led high-fidelity UX → packaging.

## Confidential baseline material

The original 165-page CRSC preparation PDF provided during project design states *仅限内部使用，严禁外传*. Its text transcription, original questions and answer keys are **not** included in this public repository. Pass Quest should support importing a locally held, page-linked UTF-8 TXT baseline or PDF, including human review of scanned pages; private source files belong under a gitignored user-data directory. Publication requires confirmed redistribution rights. The sample exercises committed for automated tests must be independently authored or otherwise redistributable.

No API keys or personal study history belong in this repository.
