# Importing a private local source

Goal 0002 makes a locally held, page-linked UTF-8 TXT baseline or the source PDF usable as **private
local** working context. Nothing is uploaded, no OCR is run automatically, and imported material is
never treated as an answer key until a user reviews it.

The private boundary is the SQLite database: sources, page revisions and question reviews
are stored under `$PASSQUEST_DB` (default `.passquest/passquest.sqlite`). On POSIX the default
`.passquest/` directory is tightened to `0700` and the database file is `0600`. A directory chosen
with `--db` or `$PASSQUEST_DB` is left to the user, and Windows has no POSIX mode (see Known limits).
No source text is copied into the repository or `testdata/`.

## Accepted inputs

| Input | How it is read |
|---|---|
| A directory containing `pages/page-NNN.txt` and optionally `manifest.tsv` | Page numbers, `section`, `native`/`ocr` provenance and the manifest's byte/non-space-character counts. Gaps and manifest-only pages become `page_missing` placeholders. |
| A single UTF-8 `.txt` split by form feeds | One page per segment; import provenance is `unknown` unless the import declared it, and the flag stays after review. |
| A single UTF-8 `.txt` split by `=== PDF PAGE N / TOTAL ... ===` markers | The earlier partial extraction's shape. Missing page numbers are preserved as `page_missing`. |
| A `.pdf` | Text layer only, through a local `pdftotext -layout -enc UTF-8` call; page count comes from local `pdfinfo`. Pages under 20 non-space code points become `scan_unextracted`. No OCR, no download. |

PDF import needs poppler on `PATH` (override with `PASSQUEST_PDFTOTEXT` / `PASSQUEST_PDFINFO`). A
missing tool is an actionable exit-2 error; the TXT path has no external dependency. The source PDF
in `data/private/source/` is gitignored and is never bundled or exported.

## Commands

```bash
passquest import --path data/baselines/2026-crsc-english
passquest sources
passquest show SOURCE                 # page-by-page status and flags
passquest show SOURCE --page 24       # current text and questions on one page
passquest questions SOURCE
```

Corrections keep provenance. Every page text is an append-only revision; a correction resets the
page to `unreviewed` because the earlier review described different text.

```bash
passquest page edit SOURCE 24 --text-file corrected-page-24.txt \
  --reviewer owner --note "fixed OCR column order"
passquest page review SOURCE 24 --reviewer owner --note "checked against the scan"
```

A question is authored locally as JSON and stays `unreviewed` with an `unmapped` answer until the
answer is reviewed. Practice is refused until **both** the page and the answer are reviewed and the
evidence is a verbatim substring of the current page text.

```json
{
  "id": "q26",
  "stem": "When did the writer arrive at London airport?",
  "options": [
    {"label": "A", "text": "In the early morning."},
    {"label": "B", "text": "Late at night."}
  ],
  "answer": "A",
  "evidence": "It was the small hours of the morning when we reached London Airport.",
  "reasoning": "The passage states the small hours of the morning.",
  "option_reasoning": {
    "A": "The passage says the small hours of the morning.",
    "B": "No late-night arrival is stated."
  },
  "glossary": [
    {"term": "small hours", "meaning": "the early hours after midnight"}
  ]
}
```

```bash
passquest question add SOURCE 24 --file question-q26.json
passquest page review SOURCE 24 --reviewer owner
passquest question review SOURCE q26 --answer A --reviewer owner
passquest practice --source SOURCE --question q26
passquest report
```

`practice` without `--source` still uses the embedded self-authored sample. With `--source` it uses
the same deterministic scoring, append-only attempts and first/repeated counting as goal 0001. The
pre-answer output states the page and answer review provenance and contains no key, evidence,
reasoning or status.

## Annotations

A question file may add an optional `"glossary": [{"term": "...", "meaning": "..."}]`. It is a
small reading aid, not a second answer key: at most 20 entries, 80 characters per term and 300 per
meaning, with unique terms. Ordinary practice, `show`, `questions` and the assessment report after the
run is finished and the item is finalized show it; a timed assessment run withholds it together with
the question reasoning, option reasoning and evidence sentence. See
[`assess.md`](assess.md#annotations).

A question file may add `"tactics": ["not-except-polarity"]` or `"tactics":
["vocabulary-in-context"]` to make the question available to `passquest tactic practice --source
SOURCE` or `passquest skill practice --source SOURCE`. The field keeps its goal-0003 name but its
values are validated against the build's registered skills; a scoring tactic is a skill kind. Tags are
shown by `passquest questions` and survive review and restart. New attempts snapshot them in
`attempts.skills_json`. Only reviewed, tagged questions that still pass every practice gate are used;
see [`tactics.md`](tactics.md) and [`adaptive.md`](adaptive.md).

## Visible doubt, not silent authority

| Flag | Meaning |
|---|---|
| `ocr_unverified` | The imported text came from OCR; an import fact that survives review and correction. |
| `column_order_unverified` | OCR reading order was not repaired by the import. |
| `reading_order_unverified` | A native text extraction preserved layout but not guaranteed reading order. |
| `scan_unextracted` | The PDF page had no usable text layer; supply text via `page edit` or a TXT import. |
| `low_text` / `empty` | Fewer than 300 non-space characters / no text. |
| `manifest_mismatch` | The manifest's byte or character count disagrees with the file. |
| `page_missing` | The import's declared page range includes a page that was not present. |
| `corrected` | A user supplied a newer text revision. |
| `page_unreviewed`, `answer_unmapped`, `evidence_missing` | Computed question warnings; `practice` refuses these. |

Review changes `review_status`, not the import facts. Reviewing an OCR page does not make its text
native: `ocr_unverified` (and `provenance_unknown` for an undeclared single TXT) stay visible after
review, and a corrected page keeps them with `origin: manual`. The review status is the separate
current-state signal.

`passquest show` prints origin, review status, flags, revision and non-space character count next to
the text. `passquest questions` prints each question's answer status, review status and computed
flags.

## Tutor consent boundary

Imported page and question text is private. The optional text tutor withholds it from a provider by
default and sends only attempt facts and first-party skill lessons. Run `passquest tutor preview`
to inspect the exact payload; `passquest tutor consent SOURCE --reviewer NAME` records a per-source
authorization and `passquest tutor revoke SOURCE --reviewer NAME` appends a withholding event. The
source title, path and full page are never included in the provider payload. See
[tutor.md](tutor.md).

## Known limits

- No OCR engine is bundled and no new OCR is produced. A scan with no text layer needs user-supplied
  text; the committed OCR baseline is unreviewed working context, not a verified key.
- Column and reading order are not repaired automatically. A review is the user's statement that the
  text was inspected; `reviewed_by` is a local free-text label, not authentication.
- The 165-page reviewed conversion remains open-ended content work. Completing it is not claimed by
  this increment.
- One local learner, no encryption at rest beyond the `0600` database, no accounts, no sync. Those
  belong to later goals.
- POSIX modes only: Windows has no `0600`/`0700` equivalent, and `os.Chmod(0o600)` does not create a
  restrictive ACL there. The import smoke reports its database-mode assertion as skipped on Windows
  rather than counting it. The directory mode is enforced only for the app-owned default path; a
  directory chosen with `--db` or `$PASSQUEST_DB` is not modified.

Measured behaviour and limitations: [`evidence/0002-local-text-baseline.md`](evidence/0002-local-text-baseline.md).
