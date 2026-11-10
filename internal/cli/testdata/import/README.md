# Import fixture

Self-authored for Pass Quest tests and the `import-smoke` gate; redistributable with this
repository. `pages/page-001.txt` is a normal native page, `pages/page-002.txt` is deliberately
damaged OCR-like text, and `manifest.tsv` gives their exact byte and non-space character counts.
`question-q1.json` and `page-001-corrected.txt` are authored for the review workflow.
`question-q2-tagged.json` carries the `not-except-polarity` tactic tag and is used by the tactic
source path and its Go tests. No text here comes from the private CRSC source.
