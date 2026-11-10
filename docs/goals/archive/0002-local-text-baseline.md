# 0002 — Usable private baseline from local source material

**State:** Ready
**Prerequisite:** 0001 — first independent exam attempt

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Enable the original PDF and its normalized UTF-8 text to serve as **private local** working context. The PDF has 165 pages; many are scans and cannot be treated as extracted text merely because a text tool ran. No publication rights have been established. Build a useful first source-to-question experience that preserves page provenance and permits progressive correction.

## Done when
- A user can import a locally held UTF-8 TXT baseline or source PDF and inspect page-linked content without sending it to an external service; normal text pages and a representative scanned page both have an honest usable/reviewable path.
- Unextracted scans, doubtful OCR, wrong column order and uncertain answer mappings are visibly marked rather than silently treated as authoritative; users can correct material and keep provenance.
- At least one user-reviewed question/passage from a private local source can be practiced through goal 0001's real scoring experience, and the app preserves that source privately across restart.
- The public repo's owner-directed partial TXT extraction is explicitly **unreviewed** and is never treated as an authoritative key or test fixture; separately authored redistributable exercises are used for automated tests. The source PDF, further OCR, unverified answer keys and private user data are not automatically published or bundled. Complete 165-page reviewed conversion remains an open-ended content operation unless evidence shows it can be safely finished within this run.

## Execution boundary

At run start, stop/report if an input only the user can supply makes the specified outcome impossible. Otherwise select reversible defaults and disclose them. Stop for irreversible/unauthorized actions; do not invent a missing exam passing score or publish restricted text. Show real behavior with evidence and explicit limitations, not only a diagram, unit stub, screenshot or progress report. Pair each acceptance condition with its evidence at completion.
