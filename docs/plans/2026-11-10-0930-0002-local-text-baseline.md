# 0002 — Usable private baseline from local source material: increment plan

Contract: [`../goals/archive/0002-local-text-baseline.md`](../goals/archive/0002-local-text-baseline.md)
Shape reference: goal 0001 (`docs/goals/archive/0001-first-independent-exam-attempt.md`, its plan and
evidence record). Harness decision: [`../decisions/D-0001-bun-typescript-harness.md`](../decisions/D-0001-bun-typescript-harness.md).

## Goal in one sentence

A learner imports a locally held, page-linked UTF-8 TXT baseline or the source PDF into a private
SQLite database, sees which pages came from a native text layer, which came from unreviewed OCR, and
which are scans with no text at all; corrects and reviews a page, authors and reviews one question
with a key, practices that question through goal 0001's deterministic scoring, and finds the source
and the attempt after a restart — with no network call and nothing private added to the repository.

## Public shape

```text
passquest import --path PATH [--title TITLE] [--db PATH] [--json]
passquest sources [--db PATH] [--json]
passquest show SOURCE [--page N] [--db PATH] [--json]
passquest questions SOURCE [--page N] [--db PATH] [--json]
passquest page edit SOURCE PAGE --text-file PATH --reviewer NAME [--note TEXT] [--db PATH] [--json]
passquest page review SOURCE PAGE --reviewer NAME [--note TEXT] [--db PATH] [--json]
passquest question add SOURCE PAGE --file QUESTION.json [--db PATH] [--json]
passquest question edit SOURCE QUESTION --file QUESTION.json [--db PATH] [--json]
passquest question review SOURCE QUESTION --answer LABEL --reviewer NAME [--db PATH] [--json]
passquest practice [--db PATH] [--source SOURCE] [--page N] [--question ID]
                    [--answer LABEL | --skip] [--json]
passquest report [--db PATH] [--json]
```

- `import --path` auto-detects: a directory containing `pages/page-NNN.txt` (the owner's baseline
  shape) and an optional `manifest.tsv`; a single UTF-8 TXT split on form feeds or on the
  `=== PDF PAGE N / TOTAL ... ===` markers used by the earlier partial extraction; or a `.pdf` whose
  text layer is extracted with a locally installed `pdftotext`.
- `practice` without `--source` keeps the goal-0001 embedded sample byte-for-byte. With `--source` it
  loads one page with at least one reviewed question and uses the same `attempt`/`store`/CLI
  protocols, feedback and append-only evidence. A question can be practiced only when its page and
  its answer mapping are both `reviewed`, its evidence is a verbatim substring of the current page
  text, and every option has an explanation.
- Exit codes unchanged: `0` success, `2` usage/invalid input/unknown source/question/unreviewed
  material, `1` storage or internal failure. `--json` errors keep the `schema_version: 1` error
  document on stderr.
- Default database unchanged: `$PASSQUEST_DB`, else `./.passquest/passquest.sqlite`, `0600` inside a
  `0700` directory.

### Page and question states (the part the contract tests)

| State | Stored as | Visible effect |
|---|---|---|
| Native text page | `origin=native`, `review_status=unreviewed` | usable after the user reviews it; `flags` empty |
| OCR page from the manifest | `origin=ocr`, `flags=ocr_unverified,column_order_unverified` (+`low_text` under 300 non-space chars) | text is inspectable, never authoritative; wrong column order is stated, not guessed |
| PDF page with no text layer | `origin=none`, `flags=scan_unextracted` (+`empty`) | the page is visible as unextracted; user supplies corrected text or a TXT import |
| Page with a low yield | `flags` include `low_text` under 300 non-space code points | inspection says so before the text is trusted |
| Missing page in a partial import | `origin=none`, `flags=page_missing` | coverage is honest: a 22-of-165 file imports as 165 pages, 143 marked missing |
| Manifest count disagrees with the file | `flags` include `manifest_mismatch` | the import succeeds, the disagreement stays visible |
| Page text corrected | new append-only `page_revisions` row, `review_status=unreviewed` | previous text and revision remain; review must be repeated |
| Page reviewed | `review_status=reviewed`, `reviewed_by`, `reviewed_at` | practice of questions on that page becomes possible |
| Question added | `answer_status=unmapped`, `review_status=unreviewed` | listed with its missing key; cannot be practiced |
| Question reviewed | `answer_status=reviewed`, `review_status=reviewed`, reviewer/time | practiceable if evidence still matches the page |
| Evidence no longer in page text | computed `evidence_missing` flag | listed as stale; practice refuses until re-reviewed |

A correction never deletes the prior text: `page_revisions` and `source_events` are append-only
(SQLite triggers reject `UPDATE`, `DELETE` and id-reusing `INSERT OR REPLACE`). Questions are a mutable
projection whose every add/edit/review is appended to `source_events` with the before/after JSON.

### Import semantics

- Source identity is deterministic: `src-<16 hex>` over title, import kind, page numbers, per-page
  origin/section/text. Importing the same file twice reports `already_imported`, writes no second
  revision and changes nothing.
- TXT directory: `pages/page-NNN.txt` are read in numeric order; gaps and manifest-only pages become
  `page_missing` placeholders rather than silently shortening the document. The manifest is used for
  `section` and `native`/`ocr` origin; per-page `bytes` and `chars_no_ws` are checked, and a mismatch
  is a visible flag, not an import failure. Invalid UTF-8 is a hard error naming the page.
- Single TXT: form feeds separate pages; without form feeds, the partial extraction's page markers
  separate pages and the declared total creates `page_missing` placeholders; otherwise the file is
  one page with `provenance_unknown`.
- PDF: one `pdftotext -layout -enc UTF-8 PATH -` call, split on form feeds, compared with `pdfinfo`'s
  page count. Pages under 20 non-space code points are `origin=none` + `scan_unextracted` and keep
  their raw output for inspection. No OCR is run by the app; no page is silently called extracted.
- Nothing is written outside the SQLite database. No network package is imported and no source text
  is copied into the repository or `testdata/`.

### Practice content

Each imported page with reviewed questions becomes one content value:
`id=<source-id>-p<NNN>`, `version=v1-<8 hex>` over the current page text and reviewed question
material. A correction or a question edit changes the version, never the content id, so attempt
numbering and repeat exposure stay scoped to the same page. The content notice states the source,
page origin, page reviewer/time, question reviewer/time and that this is private local material, not
a live exam or verified external answer key. The pre-answer JSON/human output carries a `review`
object for imported questions and still contains no key, evidence, reasoning or status.

`report` keeps global first/repeated counts and question rows keyed by `(content_id, question_id)`,
adds a `contents` list (sample, imported, or both) and omits the singular `content` when attempts
span more than one content. Existing sample-only human output and goldens remain the baseline; the
golden gains the additive `contents` field.

## Persistence

Schema version 3 adds five tables and keeps `attempts` untouched:

- `sources` — id, title, import origin, absolute local path, sha256, notice, rights, exam relation,
  created_at, page_count.
- `source_pages` — per-page origin, section, review status, flags, review note/reviewer/time.
- `page_revisions` — append-only full text revisions, origin, created_at, note.
- `source_questions` — current question projection with answer, answer status, review status,
  reviewer/time, options/reasoning as JSON.
- `source_events` — append-only audit of import, page correction/review and question add/edit/review.

Migration is versioned and idempotent; a version-0/2 database keeps its attempts and gains the new
tables. A newer schema is refused. Page revisions and events reject updates, deletes and id-reusing
inserts.

Domain split: `internal/source` owns import parsing, page/question state rules and building an
`exam.Exam`; `internal/store` owns persistence; `internal/cli` orchestrates. `internal/exam` and
`internal/attempt` keep their current meaning.

## Harness migration (same increment, per D-0001)

Goal 0002 adds import/OCR gates, so the bash/Python gate chain migrates now:

- `package.json` (`gate` script) + `bun.lock`; Bun pinned in `mise.toml`; `harness/gate.ts` runner,
  `harness/lib/` shared spawn/repo/file/report code, `harness/gates/*.ts` one module per gate with
  `id` + `run(ctx)` returning `pass | fail | skip(reason)`.
- Migrated gates: gofmt, go vet, go build, go test, go mod tidy, docs links, secrets/sources,
  local-only. New gates: `baseline` (manifest/page byte+character integrity, 22 native / 143 OCR
  counts, test/fixture isolation from `data/baselines/`), `import-smoke` (build the CLI, import a
  deliberately damaged redistributable fixture, assert unreviewed/unmapped practice refusal, correct
  → review → practice → second-process report, `0600` database), and `self-test` (feeds every gate a
  deliberately broken input and fails if the gate does not fail).
- `bun run gate` is the single definition. `scripts/gate.sh` becomes a one-line compatibility shim
  that execs it; `scripts/check-docs-links.py`, `scripts/check-secrets.sh` and
  `scripts/check-local-only.sh` are deleted after their gates are ported. CI runs `bun run gate` on
  ubuntu/macos/windows with `oven-sh/setup-bun`. A `.gitattributes` pins LF so gated goldens and
  gofmt agree on Windows.

## Evidence plan

1. Go tests: source parsing (native/OCR/scan/gaps/manifest mismatch/invalid UTF-8/idempotence/PDF
   fake extractor), state rules (review gating, stale evidence, corrections, append-only revisions
   and events), store migration v2→v3, CLI import/inspect/correct/review/practice/report, JSON
   goldens, and a subprocess restart test.
2. Real private-source transcript in `docs/evidence/0002-local-text-baseline.md` using the gitignored
   PDF under `data/private/source/` and the unreviewed baseline directory in a fresh temp directory:
   PDF import (native page 17, scan page 24 → `scan_unextracted`), TXT baseline import (page 1 native,
   page 24 OCR `unreviewed` + `column_order_unverified`), correction of the OCR page, page review,
   question add/review with a key derived by reading the corrected passage, practice through the real
   scoring path, and a new-process report. The reviewer label is a demo label and the record says the
   review is a workflow demonstration, not an owner verification.
3. Boundary evidence: `strace -f -e trace=network` during import and practice (0 socket/connect
   lines), `0600` database, `git ls-files` showing no PDF/SQLite/private source, the baseline gate,
   and `go test` proving tests never read `data/baselines/`.
4. Gate output: `bun run gate` and the `bash scripts/gate.sh` shim, real exit codes and counts.

## Trade-offs and defaults taken

- **TXT first-class, PDF via local `pdftotext`.** The standard library cannot parse PDF; a pure-Go PDF
  parser would be a new dependency with weaker CJK/CID coverage than poppler, and the owner's parse
  recipe already uses poppler. Missing `pdftotext` is a clear exit-2 error that names the TXT path;
  no download, no OCR and no network. Reversible: the extractor is one function behind
  `source.ImportOptions` and `PASSQUEST_PDFTOTEXT`; a pure-Go extractor can replace it later. (Q-0002-01)
- **Review is required before practice.** The contract forbids silent authority; a reviewed question
  on a reviewed page is the only practiceable imported item. A proposed-but-unreviewed key stays
  visible in `questions`. This is stricter than goal 0001's self-authored sample, which remains
  practiceable because its key is machine-checked by construction.
- **Correcting text resets review.** A prior review was a statement about different text; keeping it
  would be false. The correction and the prior review remain in the event/revision history.
- **All OCR pages carry `column_order_unverified`.** The import cannot verify reading order, and the
  baseline README says it was not repaired. The flag is a statement about the import, not a claim
  that the page is wrong in every line.
- **PDF import keeps the raw low-yield text and marks it.** Discarding it would hide what the tool
  produced; treating it as content would overstate extraction. `scan_unextracted` plus the raw
  revision is the honest middle.
- **Per-page content identity.** Practice needs a passage; a 165-page document is not one coherent
  exercise. Page-scoped content ids give stable attempt scoping and repeat exposure while preserving
  the page provenance. Goal 0003 can add tactics over the same rows.
- **One local learner, no encryption.** The database is `0600`; encryption/accounts are out of scope
  until the desktop goal. (Q-0002-02)
- **Reviewer is a local free-text label, not authentication.** It records who the user says performed
  the review; there is no account system. Disclosed in the notice and evidence. (Q-0002-02)

## Out of scope, with reasons

- Automatic OCR of new scans: the contract explicitly forbids publishing further OCR without a new
  recorded decision; bundling an OCR engine would add a large local dependency and is not needed for
  the honest scan-review path.
- PDF layout/column repair, answer-key extraction heuristics, or auto-generating questions from text:
  these would invent authority the source does not have and belong to later content operations.
- Completing a 165-page reviewed conversion: open-ended content work; the contract says so.
- Adaptive selection (0004), tactics (0003), AI/provider calls (0005+), Anki (0007), desktop UI
  (0011). None is needed to prove private page-linked import, review and one practiced question.
- Multi-user identities, encryption, cloud sync and remote backup.
- Committing any part of the PDF, new OCR output, a private learner database or a source-derived
  question; tests use only independently authored fixtures under `testdata/`.

## Completion checklist

- [x] Plan written to `docs/plans/` (this file).
- [x] `internal/source` import/state rules + tests; store schema v3 + tests.
- [x] CLI import/inspect/correct/review/practice/report + tests and goldens.
- [x] Bun/TS harness migration, new baseline/import-smoke/self-test gates, CI matrix, gate.sh shim.
- [x] README, conventions, harness, D-0001 outcome and baseline README reconciled.
- [x] Private-source evidence record with real output and limitations.
- [x] Goal archived unchanged, goals index updated, open questions appended.
- [x] `bun run gate` and `bash scripts/gate.sh` exit 0 recorded.

## Post-demo corrections (2026-11-10)

Two defects surfaced in the first private-source transcript and were fixed before the evidence
record was finalized:

- `--title` was parsed but the parsed source kept its default title, because the importer wrote a
  title before the override was applied. `source.ImportPath` now applies an explicit title before
  computing the content-addressed id; `TestImportTitleOverride` and `TestImportTitleFlag` pin it.
- Imported content versions were rendered as `vv1-…` because the version already carried a `v`
  prefix and the human printers add one. The material version is now `1-<hash>`, so JSON carries
  `1-…` and the human view `v1-…`; the CLI report test asserts the rendering.

These changed the demonstration database ids once, after which the transcript above was re-run from
a fresh directory.
