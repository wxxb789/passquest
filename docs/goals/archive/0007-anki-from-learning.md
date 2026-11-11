# 0007 — Useful Anki notes created during study

**State:** Active
**Prerequisite:** 0006 — resumable agent work

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Without implementing an Anki clone, allow a desktop learner to turn high-value words, exam patterns or short tactics encountered in real exercises into beautiful, concise Anki notes. Anki Desktop/AnkiConnect is the primary local path, and Anki's existing mobile syncing remains authoritative.

## Refined contract (activated 2026-11-10)

The environment available after goal 0006 has **no Anki Desktop and no AnkiConnect endpoint** (`127.0.0.1:8765`
answers nothing, and no Anki binary is installed), and the product does not control the learner's desktop:
CI cannot reach a real deck on any OS. So the increment proves the parts that are provable here and states
the rest as owner-supplied evidence instead of claiming it.

**In scope**

1. `passquest note propose` — from a stored attempt (or the learner's `--mark`), draft one note: `front`
   (word or pattern), `back` (meaning plus one exam-usable example taken from the question), `tactic`
   (reused from goal 0003 when the question carries one), `source` (source id, page, question id), and a
   `restricted` flag when the material is internal-only. The draft is deterministic without a provider and
   may be refined through the goal-0005 BYOK endpoint when one is configured.
2. `passquest note preview [--json]` — the exact field HTML that would be written, with the answer key of
   an unanswered question never included, and restricted text withheld until the goal-0005 consent is
   recorded.
3. `passquest note accept|skip` — accept persists the note locally exactly once: a repeated accept with
   the same content is a no-op that reports the existing note id and revision; an edited accept bumps the
   revision and keeps the previous one in history.
4. `passquest note export` — an Anki-importable file (tab-separated, HTML fields, header row) written
   atomically, usable when no Anki is reachable.
5. `passquest note push` — the AnkiConnect path: `version`, `createDeck`, `createModel`/`modelNames`,
   `addNote`, `updateNoteFields`. A duplicate returned by Anki is reported as already-present, not as an
   error. A revision updates the existing note id instead of adding a second card, so Anki review history
   survives. When no endpoint answers, the note is queued and drained exactly once by the next push.
6. The note model `PassQuest Note` (deck `Pass Quest`, fields `Front`, `Back`, `Tactic`, `Source`) with a
   tidy, mobile-safe card template: system font stack, no external asset, no `overflow: hidden` on long
   text, readable at phone width.

**Out of scope**

Anki clone, own scheduling algorithm, `.apkg` generation, images or audio, AnkiWeb API calls, mobile apps,
and any automatic promise that a local write reaches a phone.

**Defaults taken (reversible, disclosed)**

- Anki connection: none on this host → the AnkiConnect adapter is proven against a loopback stub; the real
  desktop remains an owner check.
- High-value signal: an incorrect or skipped attempt, or an explicit learner mark.
- Deck and model: `Pass Quest` / `PassQuest Note`; a different name is one flag.
- Confidential material: notes derived from the internal-only source stay local; AnkiWeb sync is described
  as the learner's own manual action.

## Acceptance and evidence

| # | Acceptance | Evidence |
|---|---|---|
| 1 | A draft from a real attempt carries front/back/tactic/source and the restricted flag | CLI run over a stored attempt, JSON shown |
| 2 | Preview is the exact payload and hides an unanswered key; restricted text waits for consent | before/after consent run on the same question |
| 3 | Repeated accept is idempotent; an edit bumps the revision and keeps history | two accepts + one edited accept, note ids and revisions shown |
| 4 | Export is importable text, written atomically | file content plus a failure-path check |
| 5 | AnkiConnect adapter: create model/deck, add, duplicate, update-not-re-add, queue-and-drain | loopback stub smoke gate, 3 OS, no network |
| 6 | The model template is mobile-safe and self-contained | rendered HTML inspected; no external reference |
| 7 | No output claims a phone push | forbidden-phrase assertion in the smoke gate |

The smoke gate `note smoke` uses a loopback AnkiConnect stub, exactly as `tutor smoke` uses a loopback
provider stub; real Anki Desktop and AnkiWeb behaviour is **owner-supplied evidence** and must be reported
as unverified until the owner runs it.

## Execution boundary

At run start, stop/report if an input only the user can supply makes the specified outcome impossible. Otherwise select reversible defaults and disclose them. Stop for irreversible/unauthorized actions; do not invent a missing exam passing score or publish restricted text. Show real behavior with evidence and explicit limitations, not only a diagram, unit stub, screenshot or progress report. Pair each acceptance condition with its evidence at completion.
