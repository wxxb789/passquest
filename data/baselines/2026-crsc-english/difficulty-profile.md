# Difficulty profile and learning target

Measured 2026-11-10 from `pages/` in this directory (165 pages). Numbers are reproducible from the
page files; the metric definitions are stated so they can be challenged.

## Structure

| PDF pages | Section | Evidence |
|---|---|---|
| 1–2 | Cover and 5-entry table of contents | native text |
| 3–16 | Vocabulary, 13 professional domains | 560 numbered entries (555 parsed cleanly): 150 single words, 405 phrases/abbreviations |
| 17–22 | Parallel EN/ZH paragraphs (technical-standard register) | 28 English sentences |
| 23–63 | Full mock exams, 4 sets, SECTION I/II Reading | 13,650 English words, 773 sentences |
| 64–106 | Answers and Chinese analysis | 43 pages, Chinese-dominant |
| 107–135 | Added part 1: cloze (BEC-style samples) | `Questions 19–33` ×5 sets = 15 gaps per set |
| 136–165 | Added part 2: reading (BEC-style samples) | `Questions 13–18` ×5 sets = 6 questions per passage |

Vocabulary domains and entry counts: 通信 112, 信号 106, 工程 77, 市政建筑 53, 生产制造 40, 通用 38,
牵引供电 31, 电力 26, 车辆机械电子 21, 新技术 15, 公司常识 15, 项目管理 12, 国际工程管理 5, 出国常识 4.

## English difficulty

Metric: tokens = latin words; sentence = text between terminal punctuation with ≥4 words; vocabulary
band from `wordfreq` zipf frequency of the token (≥5.0 ≈ A1–B1, 4.5–5.0 ≈ B1–B2, 4.0–4.5 ≈ B2,
3.5–4.0 ≈ C1, <3.5 rare). The zipf→CEFR mapping is a heuristic, used only for comparison inside this
file.

| Section | Words | Mean sentence | p90 sentence | ≥30-word sentences | zipf median | C1+ share |
|---|---|---|---|---|---|---|
| Mock exams (p23–63) | 13,650 | 17.6 | 32 | 98 (12.7%) | 5.79 | 5.9% |
| Cloze (p107–135) | 2,776 | 16.8 | 25 | 11 | 5.46 | 6.1% |
| Reading (p136–165) | 5,755 | 19.8 | 36 | 41 (14.1%) | 5.84 | 6.7% |
| Parallel paragraphs (p17–22) | 644 | 22.9 | 59 | 6 | 5.13 | 9.2% |

Reading at zipf ≈ 5.8 is newspaper-level English; academic prose typically measures 5.0–5.4. Only
about 6% of tokens are C1+, so the difficulty is **long sentences plus domain terminology**, not rare
literary vocabulary. The parallel-paragraph section is the hardest register in the file
(contract/standard language, zipf 5.13, 9.2% C1+).

## Learning target (what the product trains toward)

1. **Passage length:** 200–450 words per item (measured: 19 prose blocks over 350 words, 71 blocks of
   200–350 words in the mock section).
2. **Sentence complexity:** tolerate a mean of 17–20 words with ~12% of sentences at 30+ words,
   including appositives, participial clauses and stacked prepositional phrases.
3. **Vocabulary band:** solid B1–B2 general English plus the ~560 domain terms, 73% of which are
   phrases or abbreviations. C1+ vocabulary should stay under 7% of a practice passage.
4. **Item formats:** four options, one correct; detail and judgement items dominate. Sampled mock
   stems: `according to the passage` 7, NOT/EXCEPT 6, word reference/meaning 3, tone 2, main idea 2,
   inference 2, true/false 1 (89 stems sampled, OCR-derived — a sample, not a census).
5. **Cloze:** 15 gaps per set, testing collocation and part-of-speech choice, four options per gap.
6. **Ceiling:** BEC Vantage-level reading plus CRSC terminology is the pass line. Chasing C1 academic
   prose would train the wrong skill.

## Uncertainty

- The zipf→CEFR mapping is heuristic; OCR noise (stray glyphs, digits, column order) inflates the rare
  word share, so the 6% C1+ figure is an upper bound.
- Item and stem counts come from OCR line structure and are sampled, not exact.
- The exam blueprint (item count per section, pass threshold, time budget per item) is still unknown;
  this profile does not invent one.
