# 0014 — A pushed note lands in the deck the CLI claims: increment plan

Contract: [`../goals/archive/0014-note-lands-in-the-declared-deck.md`](../goals/archive/0014-note-lands-in-the-declared-deck.md)
(archived unchanged at close). Shape references: goal 0007's plan and evidence record,
`internal/anki/anki.go`, `internal/cli/note_anki.go`, `internal/store/note.go`,
`harness/gates/note-smoke.ts`. Conventions: [`../conventions.md`](../conventions.md). Harness:
[`../harness.md`](../harness.md). The finding and the reversible default are already recorded as
[`Q-0014-01`](../decisions/OPEN-QUESTIONS.md).

## Goal in one sentence

Make the deck name `note push` prints a checked claim: after a write, the push reads the deck each
card actually reached, reports a typed `deck_mismatch` (naming that deck, exit 3, one stated action)
when it is not `Pass Quest`, keeps the recorded Anki note id and pushed revision so later revisions
update the same card, and leaves a conforming endpoint's output, JSON and counts byte-identical.

## Public shape

No new command, no new flag, no removed flag.

```text
passquest note push [--endpoint URL] [--timeout-seconds N] [--force] [--db PATH] [--json]
passquest note show [NOTE] [--db PATH] [--json]
```

On a mismatch the push document gains, and only then:

```json
{ "state": "deck_mismatch",
  "counts": { "pending": 0, "pushed": 0, "already_present": 0, "conflicts": 0, "failed": 0,
              "queued": 0, "deck_mismatch": 1 },
  "notes": [ { "note_id": "note-…", "revision": 2, "anki_note_id": 101,
               "state": "deck_mismatch", "outcome": "deck_mismatch",
               "deck": "Default",
               "detail": "card landed in \"Default\", not \"Pass Quest\"" } ],
  "error": { "code": "anki_deck_mismatch", "message": "1 note(s) reached a deck other than \"Pass Quest\" …" } }
```

Human summary: the existing `Deck: … · model: …` line and count line are unchanged; a mismatch adds

```text
Deck mismatch: 1 note(s) are not in "Pass Quest":
  note-… rev 2 landed in "Default" (anki note 101)
Move the card into "Pass Quest" in Anki — later revisions still update the same card.
```

`note show` prints the recorded deck (`anki=pushed deck=Pass Quest`, or
`anki=deck_mismatch deck=Default (expected Pass Quest)`); its JSON gains `anki.deck` (always set by a
deck read) and `anki.deck_expected` (set only when the reported deck differs).

Exit code: `3` for a mismatch — an external write happened, the learner must act, and the local queue
is already drained (a retry must not re-add the card). `2`/`1`/`0` keep their meanings.

New optional fields (`counts.deck_mismatch`, `notes[].deck`, `anki.deck`, `anki.deck_expected`,
`error`) are omitted when they carry nothing, which is what keeps a conforming push byte-identical.

## Where the deck comes from (the read, not the assumption)

`notesInfo` returns a note's `cards`; that action is already allow-listed and its rows are already
read for the conflict check in this push. `cardsInfo` is the only AnkiConnect action that reports a
card's `deckName`, so it is newly allow-listed in three places: `internal/anki.allowedActions`,
`agent/anki-host.mjs` `ALLOWED_ACTIONS`, and the gate stub.

Order inside one push (write path untouched, verification appended):

```text
capabilities → [createModel] → pre-write notesInfo (existing conflict read-back)
→ write batch(es) of addNote/updateNoteFields  (persisted per batch, as today)
→ readd batch (existing)                       (persisted, as today)
→ deck verify pass:  notesInfo (only for notes whose cards are not already known)
                     cardsInfo (chunked at 50)
→ per verified note: one ledger row action=check, outcome=ok|deck_mismatch
→ report
```

Placing the verify pass after the write loops keeps two properties that matter: every acknowledged
write batch is persisted before the next host call (unchanged), and the write ledger row for a note
keeps the write's own outcome (`add/ok`, `update/ok`, `readd/ok`) instead of being overwritten by a
verification verdict. A note therefore has two rows for one push — the write and the check — which is
exactly what the existing `check/not_found` row already establishes as the pattern for a read.

The check row does not touch the outbox: `AnkiAttempt` gains `Keep` (report a read; leave the queue
row as the write left it) and `AnkiDeck` (the deck Anki reported). A write row leaves `AnkiDeck`
empty, so a note whose deck was never read records no deck at all.

## Edge-case semantics

| Case | Behaviour |
|---|---|
| Every card's `deckName` equals `Pass Quest` | `notes.anki_deck = 'Pass Quest'`, item stays `pushed`/`ok`, state `pushed`, exit 0. No new visible field. |
| A card reports another deck | Item `state`/`outcome` = `deck_mismatch`, `deck` = the reported deck, `counts.deck_mismatch = 1`, `counts.pushed = 0`, doc state `deck_mismatch`, envelope `anki_deck_mismatch`, exit 3. Note row: state `deck_mismatch`, `anki_deck` = reported deck, `anki_note_id`, `anki_pushed_revision` and the drained outbox kept. |
| Several cards report different decks | `deck` lists the distinct decks in response order, joined with `, `; detail names all of them. |
| The note's cards are unknown (`notesInfo` returns no `cards`) or present in no `cardsInfo` row | Unconfirmed, not assumed: `deck_mismatch` with `deck` empty and detail `Anki reported no card for note N` (resp. `no deck for card N`). |
| A card's `deckName` is empty | Same unconfirmed branch: `Anki reported no deck for card N`. A claim that cannot be read is never printed as achieved. |
| The verify pass itself fails (transport error, unanswered endpoint, `cardsInfo` error, undecodable result) | Acknowledged writes stay recorded and drained; the push exits 3 with `anki_unavailable` (transport) or `anki_error` (endpoint answer) and the deck stays unrecorded. No success claim, no re-add on the next push. |
| A repeat push with no new revision after a mismatch | Outbox drained, so it stays `idle`, contacts nothing, writes nothing, adds no card. |
| A new revision after a mismatch | `updateNoteFields` on the recorded id (never a second `addNote`), then the verify pass again: still `deck_mismatch` while the card sits elsewhere, `pushed` once it is in `Pass Quest` (the learner's manual move). |
| `already_present`, `conflict`, `failed`, `queued` items | Not verified: no card of ours was written, so no new deck claim is made. Unchanged behaviour. |
| `--force` | Unchanged; the forced update is verified like any other write (its card ids come from the verify pass's own `notesInfo`). |
| Note accepted after a push | `accept` resets the sync state to `local` and clears the recorded deck, so a stale deck cannot outlive the revision it described. |

## Persistence (schema v16)

SQLite cannot widen a `CHECK` in place, so both affected tables are rebuilt with an explicit copy
(the `schemaV14` precedent for `attempts`), keeping every row id and value:

- `notes` + `anki_deck TEXT NOT NULL DEFAULT ''`, `anki_state` widened with `deck_mismatch`.
- `anki_push_attempts` `outcome` widened with `deck_mismatch` (no new column: the deck is on the note
  row, and the row's `detail` names it).
- Append-only triggers and indexes recreated; no column is added to any other table.

## Evidence plan

| Acceptance | Evidence |
|---|---|
| 1 conforming endpoint unaffected | `note-push.golden.json` and `note-show.golden.json` unchanged byte for byte; `TestGoldenNotePushJSON` re-run; `note smoke` keeps its existing assertions and adds the read to the expected action sequence only. |
| 2 mis-assigning endpoint detected | `harness/gates/note-smoke.ts` gains a stub mode that accepts `addNote` and files the card in `Default` (the real Anki 26.x behaviour); the transcript names `Default`. |
| 3 typed, actionable, non-zero | Same transcript: `state`/`outcome`/`counts.deck_mismatch`, the printed action line, exit 3. |
| 4 the note id survives | The stub keeps note 101; `note show` reports it with state `deck_mismatch`; the next revision calls `updateNoteFields` on 101 with no `addNote`, and a repeat push sends nothing. |
| 5 the check is a read | The stub's `cardsInfo` rows are the only source of the deck; the gate asserts the read action was sent. `harness/gates/self-test.ts` gains a case that copies the real tree, removes the deck comparison line from `internal/cli/note_anki.go`, and requires `note smoke` to fail. A conforming note never carries `deck_mismatch`. |
| 6 nothing else moves | The gate compares `report --json` and `next --json` byte for byte before and after the mismatched push, and re-reads the recorded attempt/key. |

Go-side: `internal/anki` decoder tests; `internal/cli` fake-transport support for `cardsInfo` plus a
mismatch scenario, a repeat/new-revision case, the ledger sequences, and a v15→v16 migration test;
new goldens `note-push-deck-mismatch.golden.json` and `note-show-deck-mismatch.golden.json` as the
contract evidence for the mismatch documents.

## Trade-offs and defaults taken

- **Verify after the write, as a separate `check` row** rather than folding the verdict into the write
  row. Rejected alternative: one row per note (`add/deck_mismatch`) — it loses whether the add itself
  succeeded, and it would have to interleave reads into the write loop, breaking "persist each
  acknowledged batch before the next call".
- **`deck_mismatch` is a stored state** rather than a derived flag over (`anki_state='pushed'`,
  `anki_deck != deck`). Fewer fields to compare in every view, and no view can read `pushed` as "the
  card is where the CLI said".
- **A repeat push does not re-check the deck** once nothing is pending: the contract's repeat-push
  clause is about not writing twice, and an idle push that suddenly contacted the endpoint would change
  the byte-identical idle path. Recorded as an open question with the reversal.
- **An unreadable deck is not a success**: the endpoint's answer decides, and an endpoint that does not
  report cards or decks produces `deck_mismatch` (unconfirmed) rather than `pushed`. This makes the
  documented requirement explicit: the deck claim needs `notesInfo.cards` and `cardsInfo.deckName`
  (AnkiConnect API v6 as documented by the project).
- **`cardsInfo` costs one more host call per push.** Bounded at 50 cards per call like every other
  action, and skipped entirely when the push writes nothing.
- **Reading only what the push wrote**: the read-back is scoped to notes written in this run, so a card
  the learner moved by hand is not re-verified until the next write (documented as a known limit).

## Out of scope, with reasons

- Repairing the collection (`changeDeck`) or pinning/patching AnkiConnect: the contract excludes it;
  reporting what happened is smaller and does not write to the learner's deck to make a claim true.
- Re-reading decks on an idle push, a `note verify` command, or deck checks for `already_present` and
  conflict items: no card of ours was placed on those paths, and the contract asks for the write path.
- AnkiWeb/mobile sync, scheduling, note types, and the `agent/tutor-host.mjs` lookup outside a
  checkout: unchanged, owned elsewhere.
- Rewriting the archived 0007 contract: note content, identity and consent semantics are untouched.

## Completion checklist

- [x] `internal/anki`: `cardsInfo` allow-listed, payload builder, `CardInfo`/`DecodeCardsInfo`,
      `NoteInfo.Cards`.
- [x] `agent/anki-host.mjs`: `cardsInfo` in the allowlist.
- [x] `internal/store`: schema v16, `Note.AnkiDeck`, `AnkiAttempt.{Keep,AnkiDeck}`, `RecordAnkiAttempt`
      queue/no-queue branches, `AnkiStateDeckMismatch`.
- [x] `internal/cli`: verify pass, mismatch classification and detail, counts/state/exit, human block,
      `note show` deck fields.
- [x] Go tests: fake transport `cardsInfo`/`cards`/deck override, mismatch + revision + repeat,
      ledger expectations, v16 migration, two new goldens.
- [x] `harness/gates/note-smoke.ts`: conforming read assertion, mismatch scenario, `note show`,
      repeat push, `report`/`next`/`questions` byte comparison.
- [x] `harness/gates/self-test.ts`: the comparison-removed case (fails when the deck check is deleted).
- [x] Docs: `docs/notes.md`, `docs/harness.md` (gate 16, self-test counts), `agent/README.md`,
      evidence record, `OPEN-QUESTIONS` entries.
- [x] `docs/goals/archive/0014-note-lands-in-the-declared-deck.md` (unchanged) + `docs/goals/README.md`
      state.
- [x] `bun run gate` run once on the final tree with real counts and exit code (22 passed, 0 failed,
      0 skipped; exit 0; recorded in the evidence record).
