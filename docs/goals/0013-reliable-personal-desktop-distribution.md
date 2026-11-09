# 0013 — Install and recover Pass Quest as a personal desktop app

**State:** Provisional
**Prerequisite:** 0011 — functional desktop workspace, 0012 — visual refinement

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Make the product viable for a nondeveloper studying alone, respecting the fact that Go/MyGo and the Node/Pi agent host both need a lifecycle. No central account or cloud service is required.

## Provisional basis for acceptance
- A clean target machine can install or launch a distributable and complete a real local study loop with a verified sample without running source build commands.
- The app handles first-run data location, model credentials, disabled/unavailable optional agent features, Anki detection and normal exit/relaunch safely.
- A restart or upgrade preserves accepted learner evidence and Anki note identity; a failed optional agent operation does not destroy exam practice.
- Platform support claims match actual tested Windows/macOS/Linux targets, including their webview/runtime prerequisites; no confidential PDF or API key is shipped.

**Requires refinement before activation:** Refine supported release platforms, package formats and upgrade compatibility commitments at activation based on existing developer/test machines.

## Execution boundary

At run start, stop/report if an input only the user can supply makes the specified outcome impossible. Otherwise select reversible defaults and disclose them. Stop for irreversible/unauthorized actions; do not invent a missing exam passing score or publish restricted text. Show real behavior with evidence and explicit limitations, not only a diagram, unit stub, screenshot or progress report. Pair each acceptance condition with its evidence at completion.
