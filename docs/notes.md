# Anki notes from study

Goal 0007 turns one high-value study moment into one concise Anki note. It is an owned integration,
not an Anki clone: Pass Quest owns the local note identity, content revision and sync intent; Anki
owns its note ids, cards and review scheduling after a push or import.

A note is a **study aid**, not a verified answer key or grade. Nothing here changes an attempt, a
question or a score.

## Commands

```bash
# Draft from the latest attempt (an incorrect or skipped attempt is the default signal).
passquest note propose
passquest note propose --attempt 4 --mark "closes at 4 p.m." --json
passquest note propose --content src-…-p024 --question q2 --mark "NOT stated"

# See the exact four field values that would be written.
passquest note preview
passquest note preview --json

# Accept the local revision, or close the draft without writing.
passquest note accept
passquest note accept --front "closes at 4 p.m." --back "Shut at a stated time.\n\nExample: …"
passquest note skip

# Inspect identity and retained history.
passquest note show
passquest note show note-…

# Export an Anki-importable file, or push to a loopback AnkiConnect endpoint.
passquest note export
passquest note push
passquest note push --endpoint http://127.0.0.1:8765 --force
```

Commands that open the database (`propose`, `preview`, `accept`, `skip`, `show`, `export` and `push`)
accept `--db PATH`; the default is `$PASSQUEST_DB` or `./.passquest/passquest.sqlite`.
`note model` does not open a database and takes no `--db`; it prints the deck, model and its
version, CSS, card template and a rendered sample.

## What earns a note

| Target | Allowed without a mark | Front |
|---|---|---|
| Incorrect attempt | yes | the recorded correct option text (the wording that was missed) |
| Skipped attempt | yes | the question's registered tactic title, else the stem |
| Correct attempt | no — `low_value` | the mark |
| Reviewed question with no attempt | only with `--mark` | the mark |

`--mark` is the learner's own wording, trimmed to one line of 1–120 characters. It is not treated as
authoritative material. A note from an attempt refuses with `content_changed` if the question changed
after that attempt; answer the current question again or propose from it with `--mark`. The same check
runs at preview/accept if the content changes while a draft is open.

The note carries four fields:

- **Front** — the word or pattern.
- **Back** — two plain paragraphs: an explanation and one exam-usable example. The explanation is
  authored question/option reasoning when the learner already answered; otherwise it is a first-party
  tactic principle or the visible stem. The example is the reviewed evidence sentence, or the stem
  when the evidence is unavailable.
- **Tactic** — the registered skill title (`NOT/EXCEPT polarity` for the tagged fixture), reused from
  goal 0003 when the attempt or question carries one.
- **Source** — `source-id · pNNN · question-id`, or `sample · content-id · question-id` for first-party
  content. The imported source title is never used, so it cannot leak through a note.

### Unanswered questions never include the key

A skipped attempt is treated as unanswered for note content even though the attempt row records the
key. A reviewed question with no attempt is allowed only with `--mark`. On that branch the builder
does **not** write or return the correct label, the keyed option text, the keyed option reasoning or
the question reasoning. It reads that key material only to reject a passage sentence that would
reproduce it; otherwise the example is the visible stem. A provider refinement for an unanswered
target receives only the draft's pre-answer-visible example, and a response that reproduces key
material is rejected (`invalid_response`, exit 3) with the deterministic draft left open.

## Private source consent

Every imported source is restricted, matching the goal-0005 private-source rule. Before consent:

- `note preview` shows the tactic and source identifiers but withholds the front and back; the
  `withheld` list names them.
- `note accept` is refused with `consent_required`; a note without its example would be misleading.
- `note export` and `note push` refuse a restricted note whose source is not currently authorized.

```bash
passquest tutor consent SOURCE --reviewer NAME
passquest tutor revoke SOURCE --reviewer NAME
```

Consent is the existing append-only per-source ledger. The note body and its Anki export are a
destination separate from provider payloads, so these paths require the global decision: a
`--provider`-scoped grant authorizes that provider's inference payload only and never satisfies a
note export or push. `revoke` withholds the source on the next preview and blocks a new export or
push; it does not delete a note the learner already accepted. First-party sample and skill content
needs no consent.

## Identity and revisions

A note is identified by content, question and the marked wording:

```text
target_key = content_id + U+001F + question_id + U+001F + lower(trim(mark))
note_id    = "note-" + first 16 hex of sha256(target_key)
```

One question without a mark is one note; two marks on the same question are two notes. Re-answering a
question therefore cannot create a second card for the same lesson.

`accept` writes the current content as a revision:

- First accept: revision 1, a new local note.
- An accept with identical content: `unchanged`, the same note id and revision; no revision row and no
  new queue entry.
- An edited accept (`--front`, `--back`, `--tactic`): revision `n+1`; every earlier revision stays in
  append-only history and `note show` prints it.
- Every accept starts from the content of the last successful accept on that draft. A repeated accept
  therefore reports `unchanged` instead of restoring the original proposal, and an accept that edits
  one field (`--tactic`, say) preserves the learner's other edited fields. A new `--refine` proposal
  replaces those accepted values because it is a new proposal to review.

Drafts are transient proposals. An accepted draft stays open so a repeated or edited accept works;
`note skip` closes it without writing anything. When more than one draft is open, `--draft ID`
selects one; `preview`, `accept` and `skip` default to the latest open draft.

## Export

`note export` writes an Anki-importable text file, by default `<db dir>/passquest-notes.tsv`:

```text
#separator:tab
#html:true
#notetype:PassQuest Note
#deck:Pass Quest
#tags:passquest
#columns:Front	Back	Tactic	Source
…
```

Fields are HTML: text is escaped, newlines become `<br>`, and the back's blank line becomes the
meaning/example paragraphs. `#columns` declares the four field names (Anki separates them with the
separator set above) and is not imported as a note; there is no uncommented header row that would
become a card with front `Front`. Anki's importer reads a row beginning with `#` as an option or
comment line, so a front that starts with `#` is written with the `&#35;` entity; it renders as the
learner's text and the row stays data. The file is written `0600` through a temporary file in the destination
directory, synced and renamed; a failure leaves an existing export untouched and removes the
temporary file. A missing parent directory is created `0700`; an existing `--out` directory keeps its
mode, so choose a private directory.

Anki's text importer has no note id. Re-importing an edited export can create a duplicate card;
`note push` is the revision path. The export file contains private note text and must be treated like
the database.

## AnkiConnect push

The Go core never imports a network package. `note push` spawns the Node host `agent/anki-host.mjs`,
which is the only Anki egress. Override the host with `PASSQUEST_ANKI_HOST` and the Node binary with
`PASSQUEST_NODE`.

- Endpoint default `http://127.0.0.1:8765`, override with `--endpoint` or
  `PASSQUEST_ANKI_ENDPOINT`. Only `http`/`https`, no userinfo/query/fragment, and the host must be
  loopback (`127.0.0.1`, `localhost`, `[::1]`, or an IPv4-mapped loopback such as
  `[::ffff:127.0.0.1]`). A remote endpoint is refused with exit 2: a note can contain private source
  text and AnkiConnect is a local bridge.
- The host accepts one versioned document with at most 50 allow-listed actions: `version`,
  `createDeck`, `modelNames`, `createModel`, `addNote`, `updateNoteFields`, `notesInfo`,
  `cardsInfo`, `modelStyling`, `modelTemplates`, `updateModelStyling`, `updateModelTemplates`. The Go
  allowlist and the host's are the same set (a Go test compares them). It refuses redirects and has no database or filesystem access. A push with more than
  50 pending notes splits its writes into consecutive documents of at most 50 actions, preserving
  queue order; a chunk whose endpoint does not answer stays queued. Each completed chunk is recorded
  before the next call, so a later failure cannot discard an acknowledged note id.
- The read-back is bounded the same way: more than 50 previously pushed notes are queried in
  consecutive `notesInfo` documents of at most 50 ids, and the results are combined before conflict
  classification. The host caps one HTTP response at 1 MiB, so one unbounded request would make every
  push of a larger collection fail before any update.
- `version` must be at least 6. The deck `Pass Quest` is created when missing; the model
  `PassQuest Note` is created when `modelNames` does not contain it.
- **An existing model's design is read, and upgraded only when it is Pass Quest's own**
  (Q-0012-02). When the model exists, the push reads `modelStyling` and `modelTemplates` once:
  - the stylesheet and the single `PassQuest` template equal this build's → nothing more is sent;
  - each part equals a known earlier version (line endings and surrounding whitespace are ignored; a
    deck left half-updated by an interrupted update also qualifies) → one `updateModelTemplates` and
    one `updateModelStyling` replace them in place, and the push document reports
    `model_updated: true` (human: `Card design: the earlier Pass Quest design in Anki was updated in
    place.`);
  - anything else — an edited stylesheet or template, a renamed template, a second card type — is the
    learner's design: it is never overwritten, the document reports `model_customized: true` (human:
    `Card design: the note type was changed in Anki, so Pass Quest left its design as it is.`) and the
    notes are pushed as usual.
  Both fields are omitted when false. The model name, field names and order, template name and field
  HTML never change, so an upgrade cannot turn a pushed note into a `fields_changed` or
  `model_mismatch` conflict, and the deck read-back is unaffected. A read or update that fails is
  `anki_error`/`anki_unavailable` (exit 3) before any note is written; an endpoint that stops answering
  leaves the notes queued. The model is touched only by a push that has pending work.
- A pushed note that Anki reports under another note type is a `model_mismatch` **conflict**
  (exit 3): nothing is written and the note stays queued until the note type matches again.
- **The deck is read back, not assumed.** After the writes of one push are recorded, every note that
  write touched is verified: `notesInfo` for the cards of notes whose cards are not already known,
  then `cardsInfo` for those cards (chunked at 50 like every other action). `addNote` carrying
  `deckName: "Pass Quest"` is a request, and the AnkiConnect build on the owner's machine is known to
  file the card elsewhere anyway, so the push reports where the card reached:
  - every card in `Pass Quest` → the note records that deck, the item stays `pushed`/`ok`, exit 0; a
    conforming push prints and prints exactly what it printed before this check existed;
  - any card in another deck → the item's state and outcome are `deck_mismatch`, the document's
    `counts.deck_mismatch` is 1, the named deck is the one the card reached, `push` exits 3 with the
    envelope `anki_deck_mismatch`, and the human summary states one action: move the card into
    `Pass Quest` in Anki (the learner's manual move; Pass Quest never moves it for them);
  - a card the endpoint does not answer about, or whose `deckName` is missing, empty, unprintable or
    longer than 200 characters, or a note whose several decks together exceed those 200 characters →
    the same `deck_mismatch` outcome, with the detail saying what could not be read. An unread claim
    is never printed as achieved;
  - a read that fails after the write → exit 3 with `anki_unavailable`/`anki_error`; the notes keep
    their recorded ids and revisions and record no deck.
  The verification does not re-send anything: the write already drained the queue, the recorded
  Anki note id and pushed revision stay, a repeat push therefore contacts nothing and adds no second
  card, and a later revision updates the same card. `--force` is verified like any other write.
  The ledger records the check as its own `check/ok` or `check/deck_mismatch` row, so the write's own
  outcome stays visible beside it.
- A push that exits 3 states its failure in human mode as one `passquest: <code>: <message>` line —
  the same code and message the `--json` document carries in `error`. The post-write deck-failure path
  has no other human explanation, so that line is the one place it is stated.
- `addNote` uses `allowDuplicate: false`. A duplicate of a note's first revision, with nothing pushed
  before, is reported as `already_present` (exit 0) and that revision is marked delivered; a later
  revision of that duplicate-only note is a **conflict**, because AnkiConnect reports no note id for
  the existing card. `--force` cannot update it: remove the card in Anki or align the local content,
  then push again. A note edited before its first push is queued at revision ≥ 2, so a duplicate
  error there is a **conflict** rather than `already_present`: Anki never received revision 1, and
  marking an unwritten revision delivered would be false.
- A revision with an Anki note id calls `notesInfo` first (in bounded batches). If the fields no
  longer match the revision last pushed, the card is a **conflict**: Anki is left untouched and
  `push` exits 3. `--force` is the explicit overwrite. If the card is gone, a `check/not_found`
  ledger row is recorded, it is re-added once and the new id is stored.
- An update calls `updateNoteFields` for the recorded id, so no second card is created and Anki's
  review history survives.
- When no endpoint answers, every pending note stays queued and `push` exits 0. The next reachable
  push drains each pending revision once; a second push sends nothing. A transport failure after a
  request was sent leaves only the unacknowledged notes pending: a completed earlier batch keeps its
  recorded note ids and is not sent again. A retried `addNote` is deduplicated by Anki; if Anki
  applied an `updateNoteFields` before the error, the next push's read-back can surface a **conflict**
  (exit 3) that `--force` resolves. No note text is lost.
- A missing Node or host is `anki_unavailable` (exit 3); nothing was acknowledged before the first
  call, so the queue is unchanged. A failure after an earlier batch is reported with `partial` counts
  and the acknowledged rows already recorded. `propose` without
  `--refine`, `preview`, `accept`, `skip`, `show` and `export` work with no Node and no Anki;
  `propose --refine` needs Node, and `push` needs it only when there is pending work.

The local outbox is one desired revision per note with `pending`/`drained` state. Every acknowledged
add/update/readd is appended to a per-note attempt ledger, and so is the deck check of each write
(`check/ok`, `check/deck_mismatch`), so duplicate suppression, retries and the deck claim are
countable. In a push document, `counts.pending` is the number of outbox rows still pending after that
push: `queued`, `conflict` and `failed` rows all stay pending until a later push resolves them, while a
`deck_mismatch` row is drained and keeps its note id. A conflict, failed or mismatched push on stderr
carries the usual `error` object (`anki_conflict`, `anki_failed` or `anki_deck_mismatch`) beside the
counts and note list. `note show` prints the recorded deck for a note whose deck was read
(`anki=deck_mismatch deck=Default (expected Pass Quest)`, and `anki.deck` / `anki.deck_expected` in
`--json`); a new accepted revision clears it until the next push reads it again, and a read that
cannot name a deck records none — a `deck_mismatch` from an unreadable card is shown as
`anki=deck_mismatch` with no `deck=` clause rather than with the name an earlier read left behind.

## Model and template

Deck `Pass Quest`, model `PassQuest Note`, fields in order `Front`, `Back`, `Tactic`, `Source`, one card
template named `PassQuest`. The design is versioned (`model_version` in `note model --json`); this build
ships version 2, the "C · Stage" card of goal 0012. Every released version's stylesheet and templates
are frozen and their SHA-256 pinned (`KnownModelHashes` in `internal/note/model.go`, and again in
`note-smoke`), which is what makes an in-place upgrade recognisable.

- **Front:** the recognition prompt alone, in a reading serif on a paper sheet. The tactic name is on
  the back only, so the prompt does not name the category of its answer (Q-0012-04).
- **Back:** the prompt again (`{{FrontSide}}`), `<hr id="answer">`, the meaning and the example (the
  `pq-meaning`/`pq-example` field HTML is unchanged), and a footer with the tactic and the source label.
- **Stylesheet:** system fonts only — UI `-apple-system, BlinkMacSystemFont, "Segoe UI Variable Text",
  "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif`; reading `"Sitka Text", "Iowan Old Style",
  Charter, Georgia, "Noto Serif", serif`; source `"Cascadia Mono", "SF Mono", Consolas, Menlo, "Roboto
  Mono", monospace`. Colours are tokens on `.card`; Anki's night mode is matched by the compound
  selectors `.card.nightMode`/`.card.night_mode` (the classes sit on the card's own `<body>`) with the
  descendant form kept as a fallback, never by `prefers-color-scheme`. Every text colour reaches 4.5:1 on
  both surfaces in both modes and no text is below 12 px (Go tests). Below 480 px the sheet loses its
  border and padding shrinks; there is no fixed width, fixed height, `overflow: hidden` or
  `min-height`, so a long imported sentence wraps at 320 px.
- No external font, stylesheet, script, image, URL or event handler (Go tests and `note-smoke`).
  `note model` prints the whole template and a rendered sample.

The desktop preview renders the same bytes: `note.RenderSides` substitutes the field HTML into the two
templates in one pass (a value is never expanded again, as in Anki), and `note.CardDocument` wraps a
side as Anki does — the model stylesheet and `<body class="card card1">` (plus `nightMode night_mode`
at night) around `<div id="qa">`. The window's `PreviewNote` returns those documents for the question
and the answer, light and night, with `css_sha256` and `model_version`.

## AI refinement in the window

The desktop window offers the refinement of `note propose --refine` for an open draft in two steps.
`RefineNotePreview` builds the exact request the refinement would send — provider and model, whether
the endpoint is external, the `text.chat` verification, whether the credential is present, the
consent that covers this provider, the messages with their SHA-256 and size, and the provider's budget
row — and sends nothing; while consent withholds the source text the messages are left out rather than
shown. `RefineNote` then runs the same code as `note propose --refine` on that draft (verification,
credential, provider-scoped consent, budget reservation, recorded failure with the same codes) and
returns the suggested front and back as an advisory suggestion. The window never applies it: the
learner copies what they keep into the fields and accepts them with the usual accept. The draft's
own fields are not replaced by the suggestion, even after a relaunch (the suggestion is returned
beside them); a field left equal to the draft's own value keeps its origin, a field equal to the
suggestion records `refined`, and anything else `edited`.

## Exit codes

| Exit | Meaning |
|---|---|
| 0 | Success, including a queued push because no endpoint answered and an `already_present` duplicate. |
| 2 | Invalid input, no open draft, low-value target, missing consent, no configured provider, not-practiceable question, remote endpoint, exhausted provider budget. |
| 3 | Provider or Anki failure after an external attempt; Anki host unavailable; a conflict found by read-back; a card that reached another deck. The local queue is retained, and a mismatch keeps its recorded note id instead of staying queued. |
| 1 | Local storage or export I/O failure. |

## Limits

- The card design (version 2) is checked by Go tests and the loopback stub only. How Anki Desktop,
  AnkiDroid and AnkiMobile render it in light and night mode at phone width, which element carries the
  night-mode classes on each client, and whether `updateModelStyling`/`updateModelTemplates` cause a
  full sync are owner evidence: no Anki installation exists on the development machine.
- An exported file carries no stylesheet: a learner who only imports the TSV keeps whatever design the
  note type in their collection has.
- A real Anki Desktop write and AnkiWeb/mobile synchronization are **owner-supplied evidence**. This
  machine has no Anki binary or endpoint; the adapter is proven against a loopback AnkiConnect stub.
  CI cannot reach a learner's desktop on any OS.
- Pass Quest never contacts AnkiWeb or a phone. Synchronization is the learner's own manual action in
  Anki, and no output claims otherwise.
- No Anki clone, own scheduling, `.apkg`, images, audio, AnkiWeb API calls or mobile apps.
- `--refine` sends one provider request through the configured/selected provider and its per-provider
  daily budget. The deterministic draft never needs a provider. Refined text is labelled advisory;
  the learner accepts
  it explicitly. A refinement that would reproduce an unanswered question's key is rejected and the
  deterministic draft stays available.
- `export` is a fallback, not a revision channel; re-importing an edited export may duplicate cards.
- A conflict is never overwritten silently. A read-back field difference can be overwritten with
  `--force`; a duplicate conflict whose Anki note id is unknown cannot be, and needs the card removed
  in Anki or the local content aligned before a later push.
- The recorded deck is what Anki reported at the last push that wrote to that note. A card the learner
  moves by hand is not re-read until the next write, and a push with nothing pending contacts no
  endpoint at all; `note show` therefore reports the last known placement, not a live one.
- The deck claim needs an endpoint that reports a note's cards (`notesInfo.cards`) and a card's deck
  (`cardsInfo.deckName`). An endpoint that omits either is reported as `deck_mismatch` with the detail
  saying what could not be read, rather than being trusted.

Measured evidence: [`evidence/0007-anki-from-learning.md`](evidence/0007-anki-from-learning.md) and
[`evidence/0014-note-lands-in-the-declared-deck.md`](evidence/0014-note-lands-in-the-declared-deck.md).
Decisions: [`decisions/OPEN-QUESTIONS.md`](decisions/OPEN-QUESTIONS.md) (Q-0007-01..10, Q-0014-01..04).
