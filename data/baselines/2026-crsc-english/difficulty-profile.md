# Difficulty profile and learning target

Measured 2026-11-10 from `pages/` in this directory (165 pages). The word and sentence counts
below are reproducible from the page files with the selection rule and script stated here.
`manifest.tsv` byte/character counts are a separate metric and are documented in
[`README.md`](README.md).

## Measurement rule

Page ranges are inclusive and taken from the Structure table. For each page:

1. Drop every line that contains a CJK ideograph (`U+3400–U+4DBF`, `U+4E00–U+9FFF`,
   `U+F900–U+FAFF`). This drops the Chinese column of the bilingual pages and the Chinese answer
   analysis; English text on its own line remains, including English quoted inside an answer
   explanation.
2. **Token** = a maximal run of ASCII letters, keeping an internal `'`, `’` or `-`
   (`fail-safe` is one token). Count every token on the retained lines.
3. **Sentence** = text between terminal `.`, `!` or `?` containing at least four tokens. The
   retained lines of a page are joined with one space before splitting, so a sentence may cross a
   PDF line break but not a page break.
4. **zipf** = `wordfreq.zipf_frequency(token.lower(), "en")`; **C1+** = tokens with zipf < 4.0.
   The median is over all tokens, not over unique tokens.
5. **p90** = nearest-rank 90th percentile of sentence length.

`wordfreq` is a measurement dependency only; it is not a product or harness dependency.

```bash
# Run from data/baselines/2026-crsc-english/.
python3 -m pip install --user wordfreq   # or run inside a throwaway virtualenv
python3 - <<'PY'
import pathlib, re, statistics
from wordfreq import zipf_frequency

TOKEN = re.compile(r"[A-Za-z]+(?:['’-][A-Za-z]+)*")
CJK = re.compile(r"[\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff]")
SECTIONS = {
    "mock": (23, 63),
    "cloze": (107, 135),
    "reading": (136, 165),
    "parallel": (17, 22),
}

def measure(first, last):
    pages = []
    for page in range(first, last + 1):
        text = pathlib.Path(f"pages/page-{page:03d}.txt").read_text()
        pages.append(" ".join(line for line in text.splitlines() if not CJK.search(line)))
    flat = re.sub(r"\s+", " ", " ".join(pages))
    tokens = TOKEN.findall(flat)
    sentences = [s for s in re.split(r"(?<=[.!?])\s+", flat) if len(TOKEN.findall(s)) >= 4]
    lengths = [len(TOKEN.findall(s)) for s in sentences]
    zipfs = [zipf_frequency(token.lower(), "en") for token in tokens]
    print(
        f"{len(tokens)} words, {len(sentences)} sentences, "
        f"mean {statistics.mean(lengths):.1f}, p90 {sorted(lengths)[int(len(lengths) * 0.9)]}, "
        f"30+ {sum(n >= 30 for n in lengths)}, median zipf {statistics.median(zipfs):.2f}, "
        f"C1+ {100 * sum(z < 4.0 for z in zipfs) / len(zipfs):.1f}%"
    )

for name, (first, last) in SECTIONS.items():
    print(name, end=": ")
    measure(first, last)
PY
```

## Structure

| PDF pages | Section | Evidence |
|---|---|---|
| 1–2 | Cover and 5-entry table of contents | native text |
| 3–16 | Vocabulary, 13 professional domains | 560 numbered entries (555 parsed cleanly): 150 single words, 405 phrases/abbreviations |
| 17–22 | Parallel EN/ZH paragraphs (technical-standard register) | 31 English sentences |
| 23–63 | Full mock exams, 4 sets, SECTION I/II Reading | 13,650 English words, 797 sentences |
| 64–106 | Answers and Chinese analysis | 43 pages, Chinese-dominant |
| 107–135 | Added part 1: cloze (BEC-style samples) | `Questions 19–33` ×5 sets = 15 gaps per set |
| 136–165 | Added part 2: reading (BEC-style samples) | `Questions 13–18` ×5 sets = 6 questions per passage |

Vocabulary domains and entry counts: 通信 112, 信号 106, 工程 77, 市政建筑 53, 生产制造 40, 通用 38,
牵引供电 31, 电力 26, 车辆机械电子 21, 新技术 15, 公司常识 15, 项目管理 12, 国际工程管理 5, 出国常识 4.

## English difficulty

Vocabulary band from the `wordfreq` zipf frequency of the token (≥5.0 ≈ A1–B1, 4.5–5.0 ≈ B1–B2,
4.0–4.5 ≈ B2, 3.5–4.0 ≈ C1, <3.5 rare). The zipf→CEFR mapping is a heuristic, used only for
comparison inside this file.

| Section | Words | Mean sentence | p90 sentence | ≥30-word sentences | zipf median | C1+ share |
|---|---|---|---|---|---|---|
| Mock exams (p23–63) | 13,650 | 17.0 | 31 | 94 (11.8%) | 5.75 | 6.3% |
| Cloze (p107–135) | 2,519 | 16.0 | 23 | 9 (5.9%) | 5.46 | 7.0% |
| Reading (p136–165) | 5,170 | 18.9 | 34 | 34 (12.5%) | 5.77 | 7.2% |
| Parallel paragraphs (p17–22) | 646 | 20.7 | 36 | 4 (12.9%) | 5.11 | 10.4% |

Reading at zipf ≈ 5.8 is newspaper-level English; academic prose typically measures 5.0–5.4. The
mock, cloze and reading sections have 6–7% C1+ tokens, so the difficulty is **long sentences plus
domain terminology**, not rare literary vocabulary. The parallel-paragraph section is the hardest
register in the file (contract/standard language, zipf 5.11, 10.4% C1+).

## Learning target (what the product trains toward)

1. **Passage length:** 200–450 words per item. The source's mock reading sets are far longer; a
   practice item should be a bounded excerpt, not a whole source set.
2. **Sentence complexity:** tolerate a mean of 17–20 words with ~12% of sentences at 30+ words,
   including appositives, participial clauses and stacked prepositional phrases.
3. **Vocabulary band:** solid B1–B2 general English plus the ~560 domain terms, 73% of which are
   phrases or abbreviations. C1+ vocabulary should stay under 7% of a practice passage.
4. **Item formats:** four options, one correct; detail and judgement items dominate. Sampled mock
   stems: `according to the passage` 7, NOT/EXCEPT 6, word reference/meaning 3, tone 2, main idea 2,
   inference 2, true/false 1 (89 stems sampled, OCR-derived — a sample, not a census).
5. **Cloze:** 15 gaps per set, testing collocation and part-of-speech choice, four options per gap.
6. **Ceiling:** BEC Vantage-level reading plus CRSC terminology is the working training target
   suggested by this material; the actual pass line is unknown. Chasing C1 academic prose would
   train the wrong skill.

## Uncertainty

- The zipf→CEFR mapping is heuristic; OCR noise (stray glyphs, digits, column order) inflates the rare
  word share, so the 6–10% C1+ figures are upper bounds.
- Item and stem counts come from OCR line structure and are sampled, not exact.
- The exam blueprint (item count per section, pass threshold, time budget per item) is still unknown;
  this profile does not invent one.
