# 0002 — Usable private baseline from local source material: evidence

- Tree: working tree over `bf5eb1f` with the goal-0002 increment staged (not committed), branch
  `feat/0002-local-text-baseline`; the operator commits.
- Date: 2026-11-10. Host: Linux 7.0.0-1017-azure x64.
- Toolchain: Go 1.27.3 via mise; Bun 1.4.4; poppler `pdftotext`/`pdfinfo` 24.02.0. No OCR engine was
  invoked in this increment (`tesseract` was not run).
- Contract: [`../goals/archive/0002-local-text-baseline.md`](../goals/archive/0002-local-text-baseline.md) (moved
  unchanged at close). Plan:
  [`../plans/2026-11-10-0930-0002-local-text-baseline.md`](../plans/2026-11-10-0930-0002-local-text-baseline.md).

All commands below ran from the repository root. The demonstration used a fresh temporary directory
(`/tmp/passquest-0002-demo`), a fresh database, a separately built binary, the committed baseline
directory, and the gitignored source PDF at `data/private/source/2025-crsc-english-review.pdf`.
Nothing from the demonstration was added to the repository.

## What was built

| Package | Truth it owns |
|---|---|
| `internal/source` | TXT directory / single TXT / PDF text-layer import, page provenance and flags, review state rules, question-file validation, reviewed-answer gating, and conversion of a reviewed page into the goal-0001 `exam.Exam` shape. |
| `internal/store` | Schema version 3: `sources`, `source_pages`, append-only `page_revisions`, mutable `source_questions` with append-only `source_events`. Migration from v0/v2 keeps attempts. Page revisions and events reject `UPDATE`, `DELETE` and id-reusing `INSERT OR REPLACE`. |
| `internal/cli` | `import`, `sources`, `show`, `questions`, `page edit`, `page review`, `question add`, `question edit`, `question review`, and `practice --source/--page/--question`; `report` now carries a `contents` list and omits the singular `content` when attempts span contents. |
| `harness/` | Bun/TypeScript gate runner and 12 gate modules, including `baseline`, `import-smoke` and `harness-self-test`; the bash/Python gate scripts were migrated and deleted. |

The shipped sample and its goal-0001 protocol are unchanged for `practice` without `--source`. Imported
questions are practiceable only when the page and the answer mapping are both `reviewed`, the answer
is set, every option is explained, and the evidence is a verbatim substring of the current page text.

## Tier 1 — automated checks

| Command | Result |
|---|---|
| `go test ./... -count=1 -v` | exit 0; 75 top-level tests, 59 subtests, 0 failures, 0 skips |
| `bun run gate` | exit 0; 12 gates passed (typecheck, gofmt, vet, build, test, tidy, docs links, secrets/sources, local-only, baseline integrity, import smoke, harness self-test), 0 failed, 0 skipped |
| `go vet ./...` | exit 0 |
| `go mod tidy -diff` | empty diff, exit 0 |
| `gofmt -l` (tracked `*.go`) | empty output (19 files checked by the gate) |
| `./node_modules/.bin/tsc --noEmit` (`bun run gate` typecheck) | exit 0, 0 type errors |

The tests that pin this increment's contract clauses:

| Test | Clause it pins |
|---|---|
| `TestImportTXTDirReadsManifestAndPages` | Manifest `section`/`native`/`ocr` provenance, native and OCR flags, deterministic source id. |
| `TestImportTXTDirFlagsManifestMismatchAndMissingPages` | A manifest count disagreement is a visible flag; missing declared pages become `page_missing` instead of shrinking the document. |
| `TestImportTXTDirRejectsInvalidInput` | No pages, invalid UTF-8 and a malformed manifest are hard errors. |
| `TestImportTXTFileSplitsOnFormFeeds`, `TestImportTXTFileSplitsPageMarkersAndPreservesGaps` | Single-file TXT import paths, provenance unknown, empty pages, and the partial extraction's page markers with gaps. |
| `TestImportPDFMarksPagesWithoutTextLayer` | A PDF page under 20 non-space code points is `none` + `scan_unextracted`; a native page is `native` + `reading_order_unverified`. |
| `TestImportPDFRejectsMismatchedPageCounts`, `TestImportPDFReportsMissingTool` | Ambiguous page linkage fails; a missing local converter is an actionable error. |
| `TestParseQuestionJSONStaysUnreviewed`, `TestParseQuestionJSONRejectsBadInput` | A proposed answer is stored `unmapped`/`unreviewed`; structural question errors are rejected. |
| `TestQuestionEditResetsReviewAndAnswerMapping` | Editing content resets review and answer mapping while keeping `created_at`. |
| `TestReviewAnswerRequiresReviewedPageAndMatchingEvidence` | Answer review requires a reviewed page, a reviewer, and evidence still present in the page. |
| `TestBuildPracticeExamOnlyServesReviewedQuestions` | Unreviewed questions are excluded; stale reviewed questions are reported and refused; content id/version/notice are built. |
| `TestPracticeContentIDRoundTrip` | `<source-id>-p<NNN>` is the stable content identity. |
| `TestImportSourceIsIdempotentAndLoadsPages` | A second import returns `created=false`, writes no second revision, and preserves pages/events. |
| `TestPageRevisionsAndEventsAreAppendOnly` | Both histories reject update/delete/id-reusing replace. |
| `TestCorrectPageAppendsRevisionAndResetsReview` | Correction keeps the old revision, moves origin to `manual`, preserves import flags, adds `corrected`, and resets review. |
| `TestQuestionAddReviewEditKeepsAudit` | Add, page review, answer review and edit produce the expected append-only event order. |
| `TestMigrationFromVersionTwoKeepsAttemptsAndAddsSources` | A v2 database keeps attempts, migrates to 3, and can import sources. |
| CLI tests `TestImportInspectReviewAndPractice`, `TestPageCorrectionResetsReviewAndKeepsProvenance`, `TestQuestionEditResetsReviewAndDuplicateIsRejected`, `TestSourceCommandsRejectBadInput`, `TestImportPDFWithoutLocalToolIsUsageError`, `TestImportedSourceSurvivesProcessRestart`, `TestReportShowsMixedContentsTruthfully`, `TestImportTitleFlag` | The end-to-end command surface, review gates, provenance, restart, PDF-tool failure, mixed-content report and title override. |
| `TestGoldenReportJSON` and the existing goal-0001 tests | The sample protocol is preserved; the report golden gains only the additive `contents` field and the imported-material limitation line. |

The test count is not the evidence that the product works; the Tier 2 transcript is.

## Tier 2 — private local demonstration

Binary and database:

```text
go build -o /tmp/passquest-0002-demo/passquest ./cmd/passquest
db=/tmp/passquest-0002-demo/passquest.sqlite
```

### Normal native page and unreviewed OCR page (committed baseline TXT)

```text
$ passquest import --path data/baselines/2026-crsc-english --db "$db"
Imported source src-029b9408ceddd473 (165 pages, 0 reviewed, 0 questions).
Title: 2026-crsc-english
Origin: txt-directory
Database: /tmp/passquest-0002-demo/passquest.sqlite
real 0m0.058s
```

Page 1 (native; its text contains the owner's internal-only cover notice and is not quoted here):

```text
Origin: native
Review: unreviewed
Flags: low_text,reading_order_unverified
Revision: 1
Chars (no whitespace): 49
```

Page 24 (OCR text from a scanned page; only the header line and metadata are quoted):

```text
Origin: ocr
Review: unreviewed
Flags: column_order_unverified,ocr_unverified
Revision: 1
Chars (no whitespace): 2003

第 一 矢 ~ SECTION II Reading
```

The OCR header is visibly wrong; the page is not usable as an authoritative passage in this state.

### Correction, review, question and answer review

A local corrected text (same file, two OCR glyph/typo fixes) was supplied:

```text
$ passquest page edit src-029b9408ceddd473 24 \
    --text-file /tmp/passquest-0002-demo/page-024-corrected.txt \
    --reviewer demo-operator --note "fixed OCR header glyphs and one typo after inspecting the scan"
Page 24 corrected: revision 2, review reset to unreviewed.
Flags: column_order_unverified,corrected,ocr_unverified

$ passquest page review src-029b9408ceddd473 24 --reviewer demo-operator \
    --note "compared with the OCR text and source page"
Page 24 reviewed by demo-operator.

$ passquest question add src-029b9408ceddd473 24 --file question-q26.json
Question q26 added to page 24: review unreviewed, answer unmapped.

$ passquest question review src-029b9408ceddd473 q26 --answer A --reviewer demo-operator
Question q26 reviewed: answer A, reviewed by demo-operator.
```

The question's evidence is the passage sentence `It was the small hours of the morning when we
reached London Airport.`; the reviewed answer is `A — In the early morning.` This review is a
**workflow demonstration by the agent under the label `demo-operator`**, not a claim that the owner
reviewed this page. The whole 165-page conversion remains unreviewed.

After correction, `show` reports `origin: manual`, `review: reviewed`, `revision: 2`,
`chars: 1999`, and flags `column_order_unverified,corrected,ocr_unverified` — the OCR doubt stays
visible while the correction and review are recorded separately. The database keeps both revisions:

```text
(revision, origin, bytes, note)
(1, 'ocr', 2457, 'imported')
(2, 'manual', 2449, 'fixed OCR header glyphs and one typo after inspecting the scan')
```

### Pre-answer privacy and reviewed practice

`passquest practice --json --source … --question q26` returned `state: awaiting_answer`, content id
`src-029b9408ceddd473-p024`, version `1-dac7cdc1`, and the page/answer review provenance. A recursive
key check found **none** of `answer`, `correct_label`, `correct_text`, `evidence`, `reasoning`,
`status`, `feedback`. The question's `options` were present (A–D) without a key marker.

```text
$ passquest practice --source src-029b9408ceddd473 --question q26 --answer A
Result: CORRECT
Your answer: A — In the early morning.
Correct answer: A — In the early morning.
Evidence: "It was the small hours of the morning when we reached London Airport."
Why: The passage states that the writer reached London Airport in the small hours of the morning, which is early morning.
First attempt on this question; no earlier answer or evidence was shown.
Recorded attempt #1 (question q26, attempt 1 on this question, 0 ms).
```

A second attempt after the key was shown was stored as repeat; the report prints first and repeated
counts separately:

```text
$ passquest report
Pass Quest — attempt evidence (2 contents)
Contents
  src-029b9408ceddd473-p024  2026-crsc-english — page 24
  src-5591c1f7538df1df-p024  CRSC source PDF (private) — page 24

Attempts: 3 (correct 3, incorrect 0, skipped 0)
First attempts: 2 (correct 2, incorrect 0, skipped 0)
Repeated attempts (answer already shown): 1 (correct 1, incorrect 0, skipped 0)

Questions
  q26  attempts 2  first correct  latest correct  repeated 1
  src-5591c1f7538df1df-p024/q26  attempts 1  first correct  latest correct  repeated 0

Attempts
  #1  q26  correct  first  selected A  0 ms  session de74d530689c6c6f
  #2  q26  correct  repeat  selected A  0 ms  session 16f0483ff24c7cfa
  #3  src-5591c1f7538df1df-p024/q26  correct  first  selected A  0 ms  session 70ef5817899030f7
```

The report is a separate process from all three attempts; the rows are durable SQLite evidence, not
in-memory state. The `#2` row is explicitly a repeat, and no percentage, score, pass estimate or
readiness claim is produced.

### Source PDF: native page, unextracted scan, supplied text

```text
$ passquest import --path data/private/source/2025-crsc-english-review.pdf \
    --title "CRSC source PDF (private)" --db "$db"
Imported source src-5591c1f7538df1df (165 pages, 0 reviewed, 0 questions).
Title: CRSC source PDF (private)
Origin: pdf
real 0m0.103s
```

Page 17 (a native text-layer page):

```text
Origin: native
Review: unreviewed
Flags: reading_order_unverified
Revision: 1
Chars (no whitespace): 2594
```

Page 24 (a scan with no usable text layer; the raw five non-space characters are kept for
inspection, not treated as content):

```text
Origin: none
Review: unreviewed
Flags: scan_unextracted
Revision: 1
Chars (no whitespace): 5
```

Supplying the corrected text through `page edit` and reviewing it turns that scanned page into a
readable, reviewed page (`origin: manual`, `flags: corrected`, revision 2). A question authored on
that page and reviewed (`q26`, answer A) was then practiced from the **PDF-imported source**, adding
the third attempt and the second content in the report above. PDF import uses only the local text
layer; no OCR was run.

### Database state after the demonstration

```text
user_version: 3
tables: attempts, page_revisions, source_events, source_pages, source_questions, sources
triggers: attempts_append_only_* (3), page_revisions_append_only_* (3), source_events_append_only_* (3)
source src-029b9408ceddd473: 165 pages, 1 reviewed, 1 question, 5 events
source src-5591c1f7538df1df: 165 pages, 1 reviewed, 1 question, 5 events
attempts: 3
event kinds: import 2, page_corrected 2, page_reviewed 2, question_added 2, question_reviewed 2
database mode: 0600
```

The second import of the baseline directory returned `Source already imported` with the same id and
165 pages, and wrote no second revision. The source id is deterministic over title, import kind and
page content.

## Tier 3 — local-only and source boundary

| Check | Command | Result |
|---|---|---|
| Runtime network syscalls | `strace -f -e trace=network -o network.log passquest practice …` | exit 0; **0** `socket`/`connect`/`bind`/`sendto`/`recvfrom`/`getaddrinfo` lines (8 log lines total) |
| Import network syscalls | `strace -f -e trace=network -o network-import.log passquest import …` | exit 0; **0** network syscalls |
| Tracked private files | `git ls-files` filtered for `.pdf`, `.sqlite*`, `.db`, `data/private/`, `data/local/`, `local-data/`, `.passquest/` | no matches |
| Private PDF ignored | `git check-ignore -v data/private/source/2025-crsc-english-review.pdf` | `.gitignore:2:/data/private/` |
| First-party network imports | `bun run gate` local-only gate | 6 first-party packages, 0 network imports, 0 confidential markers |
| Baseline provenance | `bun run gate` baseline integrity gate | 165 pages, 22 native, 143 OCR, every manifest byte/character count matches, README states `unreviewed` |
| Baseline is not a test fixture | same gate | 0 Go tests or `testdata/` fixtures reference `data/baselines` |
| Tests use authored fixtures | `internal/cli/testdata/import/` and `internal/cli/testdata/import/question-q1.json` | Independently authored native page, deliberately damaged OCR page, matching manifest and a two-option question; no baseline text |
| No new OCR committed | repository tree | only the owner's existing baseline; no new OCR output, no PDF, no database |
| Quotation boundary | this record | short lines quoted here already exist in the owner-authorised `data/baselines/2026-crsc-english/` text; no new source text, OCR or PDF bytes were added |
| Database protection | `stat` after the demo | `0600`; the default directory is created `0700` (pinned by `TestOpenCreatesSchemaAndProtectsFiles`) |

`net` remains linked transitively by the pure-Go SQLite chain (`modernc.org/sqlite` → `libc` →
`google/uuid`); no Pass Quest package imports or calls it. The strace runs are the runtime proof for
this increment.

## Harness migration (same increment)

The bash/Python gate chain was migrated because goal 0002 adds import/OCR gates, as required by
[`../decisions/D-0001-bun-typescript-harness.md`](../decisions/D-0001-bun-typescript-harness.md).

- `bun run gate` runs `harness/gate.ts`; 12 gates in order. `bash scripts/gate.sh` is a shim that
  execs it. `scripts/check-docs-links.py`, `scripts/check-secrets.sh` and
  `scripts/check-local-only.sh` were deleted.
- `harness/gates/self-test.ts` feeds each other gate a deliberately broken input and passed 14 cases
  over 11/11 gates. It includes a real `tsc` type error, an unformatted Go file, a broken markdown
  link, a fake credential (whose value must not be echoed), three isolating `local-only` cases
  (first-party import, transitive linked client, confidential marker), two `baseline` cases (a
  complete 165-page tree with one wrong count, and a fixture that references the baseline),
  failing/vet/build/tidy Go projects, and a fake CLI whose import output is not the protocol.
- `bun.lock` is committed; the only dependencies are `typescript` and `@types/bun`, used by the
  typecheck gate. CI installs with `bun install --frozen-lockfile` and runs the same gate on ubuntu,
  macos and windows; `.gitattributes` pins LF.
- `mise.toml` now pins Bun 1.4.4 alongside Go 1.27.3 and Node 22.

## Contract clause → evidence

| "Done when" clause | Evidence |
|---|---|
| A user can import a locally held UTF-8 TXT baseline or source PDF and inspect page-linked content without sending it to an external service; normal text pages and a representative scanned page both have an honest usable/reviewable path. | Baseline TXT import (165 pages, native page 1, OCR page 24), PDF import (native page 17, scan page 24), `show` output with origin/review/flags/revision/character count; 0 network syscalls under strace for both import and practice. The OCR page and the PDF scan were corrected, reviewed and made practiceable through `page edit` + `page review`. |
| Unextracted scans, doubtful OCR, wrong column order and uncertain answer mappings are visibly marked rather than silently treated as authoritative; users can correct material and keep provenance. | `scan_unextracted`, `ocr_unverified`, `column_order_unverified`, `reading_order_unverified`, `low_text`, `page_missing`, `manifest_mismatch`, `answer_unmapped`, `page_unreviewed`, `evidence_missing` states; page 24 correction retains revision 1 (`ocr`) and revision 2 (`manual`) and an event trail; question add shows `review unreviewed, answer unmapped`. Tests pin the gates and the append-only histories. |
| At least one user-reviewed question/passage from a private local source can be practiced through goal 0001's real scoring experience, and the app preserves that source privately across restart. | Question `q26` from page 24 of both imports was reviewed (`demo-operator`), practiced deterministically (`StatusCorrect`), stored as an attempt with content id `<source>-p024`; the report was produced by a later process and shows 3 attempts / first 2 / repeated 1; `TestImportedSourceSurvivesProcessRestart` runs `show` and `practice` in child OS processes. The database is `0600` and the private PDF is gitignored. |
| The public repo's owner-directed partial TXT extraction is explicitly **unreviewed** and is never treated as an authoritative key or test fixture; separately authored redistributable exercises are used for automated tests. | The baseline README and import notice state unreviewed; every imported page starts `unreviewed`; practice refuses until review. `baseline.ts` checks the README statement and 0 test/fixture references; the automated fixtures are the self-authored `internal/cli/testdata/import/` files. The source PDF, new OCR, unverified keys and private data are absent from tracked files. |

## Known limits and residual risks

1. **The 165-page conversion is not reviewed.** Only page 24 of two imports was corrected and
   reviewed in this demonstration. The committed baseline keeps 143 OCR pages and 22 native pages;
   all remain unreviewed except the two local demo rows. The contract's open-ended content operation
   is not finished.
2. **The demonstration review is the agent's workflow run**, labelled `demo-operator`. It is not the
   owner's review and is not evidence that the OCR is correct. Reviewer labels are free text, local,
   and not authenticated.
3. **No OCR engine is bundled and no new OCR is produced.** PDF import reads only the native text
   layer; 143 of 165 PDF pages were `scan_unextracted` and required user-supplied text. Column order
   is not repaired; `column_order_unverified` remains after correction.
4. **PDF import requires poppler.** `pdftotext` and `pdfinfo` must be on `PATH` (overridable with
   `PASSQUEST_PDFTOTEXT` / `PASSQUEST_PDFINFO`). A missing tool is exit 2 and points at the TXT path;
   the TXT import has no external tool.
5. **One local learner, no encryption or accounts.** The database is `0600` in a `0700` directory
   under the gitignored state path. At-rest encryption and multiple identities belong to later goals
   (Q-0002-02).
6. **Page-scoped content.** A source page is the practice content unit; a 165-page PDF is not one
   coherent exercise. Multi-page passages or whole-set holdout scoping are deferred to goals 0003 and
   0009 (Q-0002-03).
7. **Source identity is content-addressed.** Correcting the on-disk TXT and importing it again
   creates a new source id rather than mutating the old one; corrections made through the CLI keep
   the original source and append revisions. This is deliberate but worth knowing before re-importing
   a changed directory.
8. **Harness portability is claimed only where CI runs.** The new CI matrix is ubuntu/macos/windows,
   but this evidence was measured on Linux only; macOS/Windows proof is the matrix run, not this
   record.
9. **Performance numbers are not a benchmark.** The demo import timings (58 ms TXT, 103 ms PDF for
   165 pages on this host) measure this dataset and are not a promise for other PDFs.

## Final gate run

```text
$ bun run harness/gate.ts

== typecheck
PASS  typecheck (0 type errors)

== gofmt
PASS  gofmt (19 files)

== go vet
PASS  go vet

== go build
PASS  go build

== go test
PASS  go test (6 packages, 0 failures)

== go mod tidy
PASS  go mod tidy

== docs links
PASS  docs links (58 checked, 0 broken)

== secrets/sources
PASS  secrets/sources (252 tracked files, 0 findings)

== local-only CLI
PASS  local-only CLI (6 first-party packages, 0 network imports, 0 confidential markers)

== baseline integrity
PASS  baseline integrity (165 pages (22 native, 143 OCR), 0 fixture references)

== import smoke
PASS  import smoke (35 assertions)

== harness self-test
PASS  harness self-test (14 cases, 11/11 gates)

12 passed, 0 failed, 0 skipped
GATE: PASS
```
