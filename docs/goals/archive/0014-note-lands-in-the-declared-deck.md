# 0014 — A pushed note lands in the deck the CLI claims

**State:** Active
**Prerequisite:** 0007 — Anki notes from learning

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Goal 0007 shipped a push that prints `Deck: Pass Quest` and reports `pushed/ok`. On the owner's real
machine the card was found in Anki's default deck while the empty `Pass Quest` deck sat beside it: the
AnkiConnect build in use does not honour `deckName` on `addNote` (it writes the requested deck onto the
note type and then adds the note without a deck id), and the push's read-back compares the model and the
fields but never the deck. The result is a false claim about where the learner's card went.

This increment makes that claim checkable. A push must either land the card in the deck it names or say
plainly that it did not — with the deck the card actually reached — while keeping the note id it already
recorded so revision updates keep working and a repeat push never adds a second card.

The defect itself lives in a third-party add-on and is **not** fixed here; the deliverable is that Pass
Quest can no longer report success it did not achieve.

## Done when

- Against a conforming loopback endpoint the push output, JSON and counts stay byte-identical to today.
- Against an endpoint that accepts the write but places the card in a different deck, the push:
  - names the deck the card actually landed in and never presents the requested deck as achieved;
  - gives that item an outcome other than `pushed/ok` (`deck_mismatch`), visible in the human summary,
    in `--json` and in `note show`;
  - keeps the recorded Anki note id, so a later revision updates the same note rather than adding one;
  - exits non-zero with one stated action, and does not change an attempt, a grade or the answer key.
- A repeat push over a mismatched note contacts no write and adds no second card.

## Acceptance and evidence

| # | Acceptance | Evidence |
|---|---|---|
| 1 | A conforming endpoint is unaffected | the existing `note smoke` scenarios still pass with unchanged output; the summary line and JSON for a matching deck are compared before/after |
| 2 | A mis-assigning endpoint is detected | a stub mode that accepts `addNote` but reports another deck (the real Anki 26.x behaviour) drives a transcript where the mismatch is named |
| 3 | The mismatch is typed, actionable and non-zero | the same transcript shows the outcome, the printed action and the exit code |
| 4 | The note id survives | `note show` after the mismatch still carries the Anki note id; the next revision pushes an update, and a repeat push adds no card |
| 5 | The check is a read, not an assumption | the gate proves the deck is read back from the endpoint (removing the comparison makes the gate fail); a note whose deck matches is never flagged |
| 6 | Nothing else moves | the attempt report, `next` and the answer key are byte-identical before and after a mismatched push |

`harness/gates/note-smoke.ts` is the judging artifact: a two-way proof (matching deck passes, mis-assigned
deck fails) plus a self-test case that fails when the comparison is removed. The real-machine run against
Anki Desktop stays owner-supplied evidence, as goal 0007 states.

## Out of scope

- Patching, vendoring or version-pinning AnkiConnect, and moving the card for the learner (`changeDeck`
  repair is deliberately not adopted: it would write to the learner's collection to make a claim true
  instead of reporting what happened).
- AnkiWeb or mobile synchronization; Anki scheduling, decks or note types beyond what 0007 already owns.
- The `agent/tutor-host.mjs` lookup path outside a repository checkout (a documented, self-explaining
  error, not this defect).
- Re-opening the archived 0007 contract; that goal still owns note content and identity.

## Known limits

A real Anki 26.x with the current AnkiConnect build still lands the card in the default deck. After this
increment the learner is told so, with the deck name, and moves it manually once. Whether a released
AnkiConnect on a matching Anki version behaves differently is not settled here and must not be claimed.

## Execution boundary

At run start, stop/report if an input only the user can supply makes the specified outcome impossible. Otherwise select reversible defaults and disclose them. Stop for irreversible/unauthorized actions; do not invent a missing exam passing score or publish restricted text. Show real behavior with evidence and explicit limitations, not only a diagram, unit stub, screenshot or progress report. Pair each acceptance condition with its evidence at completion.
