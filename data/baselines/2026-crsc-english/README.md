# 2025 CRSC senior-title English prep — page-linked parsed text

Parsed text of the owner's copy of `2025年中国通号高级职称外语考试复习资料.pdf` (165 pages), one
file per PDF page, for private-source import work (goal 0002).

**The original PDF is not in this repository.** It stays at `data/private/source/` (gitignored) —
77,422,890 bytes, first page marked `（仅限内部使用，严禁外传）`.

## What is here

| Path | Content |
|---|---|
| `pages/page-001.txt` … `page-165.txt` | One file per PDF page. Pages 1–22 are the PDF's native text layer; pages 23–165 are OCR. |
| `manifest.tsv` | `page`, `section`, `source` (`native`/`ocr`), `bytes`, `chars_no_ws` — the per-page provenance and size map. |
| `difficulty-profile.md` | Measured English-difficulty profile of this material, and the resulting learning target. |

## Provenance and method

- Native pages: `pdftotext -layout -f N -l N <pdf> -` (poppler). Layout preserved, reading order not
  repaired.
- OCR pages: `pdftoppm -r 300 -gray -png -singlefile`, then
  `tesseract <png> stdout -l eng+chi_sim --psm 6` (tesseract 5.3.4, chi_sim + eng, leptonica 1.82.0).
  Output is raw and **unreviewed** — no column repair, no answer-key reconciliation, no manual
  correction pass.
- 143 of 165 pages carry no text layer at all, which is why OCR exists. A text tool that "ran" is not
  evidence that text was extracted; these files are.

## Known defects (honest limits)

- OCR noise: column order, stray glyphs (`|`, `~`, box characters), vertical page numbers folded into
  lines, and occasional merged words. Numbers in question/option positions are the least reliable
  part.
- Four pages are near-empty (under 300 bytes): p1 (cover) and p50, p77, p135 (figure or separator
  pages).
- `manifest.tsv` `chars_no_ws` counts code points that are not Unicode whitespace; the form feed
  (`\f`) that `pdftotext` emits between pages is also excluded. It is the honesty check: a page with
  a low count is a page you must look at before trusting.
- Answer keys and answer explanations are **not verified**. Nothing here may be treated as an
  authoritative key or used as a test fixture; goal 0002 requires user-reviewed answers with
  provenance before that changes.
- `data/baselines/2025-crsc-english-partial.txt` (the earlier 22-page extraction marked UNREVIEWED) is
  superseded by this directory and kept only as history.

## Visibility and rights

The owner explicitly authorised committing this parsed text to this public repository on 2026-11-10,
knowing that the original document is marked internal-only. That authorisation covers **this parsed
text only**:

- the original PDF is not committed, not bundled, and not exported;
- no further OCR of new material and no upload of this text to an external provider without a new,
  recorded decision;
- no claim that the material is public-domain, licensed, or safe to redistribute — publication rights
  for the original remain unestablished.

## Reproducing the parse

```bash
pdftotext -f N -l N -layout data/private/source/2025-crsc-english-review.pdf -   # pages 1-22
pdftoppm -f N -l N -r 300 -gray -png -singlefile data/private/source/2025-crsc-english-review.pdf /tmp/p
tesseract /tmp/p.png stdout -l eng+chi_sim --psm 6                                # pages 23-165
```

This recipe is documentation, not a harness gate. When import/OCR becomes a product surface (goal
0002) it becomes a tested Bun/TypeScript tool under `harness/`, not another ad-hoc script.
