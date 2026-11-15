# Pass Quest GUI prototypes — shared brief

Three static, self-contained HTML prototypes in **three completely different design directions** for
the Pass Quest desktop app (goals 0011/0012). They exist so the owner can choose a direction. They
are throwaway prototypes, not the implementation. The product truth comes from
`docs/core-design.md` and the domain docs in `docs/`; this brief condenses what a screen must show.

## The product in one paragraph

Pass Quest is a local-first, desktop-first AI exam coach. Its learner is a Chinese-speaking adult at
roughly junior-high English level with about 90 days to pass one specific written English exam
(reading/cloze, no listening or speaking). The real exam blueprint, pass threshold, exam date and
daily study budget are **unknown** and must stay visibly unknown. The coach should feel competent and
discreet: it knows what to practice next and why, when to stop explaining, when to test. The learner
spends attention answering exam-like questions, not managing an AI system.

## Hard product rules every screen must respect

1. **Verified truth outranks AI.** The deterministic grade and reviewed answer key are the authority.
   Any model text is labelled *advisory* (建议性 / Advisory) and is visually subordinate to the key.
2. **Independent ≠ assisted.** Every attempt carries a condition: `independent transfer` (独立迁移,
   counts), `assisted` (有辅助 — a hint/walkthrough was visible, never counts), `re-exposure`
   (重复接触 — passage already seen, never counts), `revisit` (重做), `assessment` (测评 — timed,
   unassisted). Before answering, the learner can see whether this attempt *can* count. Opening a
   hint must clearly warn it turns the attempt into assisted.
3. **Honest uncertainty.** Never show a pass probability, readiness %, predicted score, invented exam
   date or invented pass line. Show `及格线：未知 / Pass threshold: unknown` and observed accuracy,
   time, failure modes and coverage instead. Prep day count is fine; exam date is "未设置 / not set"
   with an action to set it.
4. **No gamification or fluff.** No streaks, XP, badges, levels, confetti, leaderboards, mascots,
   "Great job!" praise, or reward economies. Neutral, precise, warm-but-brief language.
5. **User control over external effects.** Any provider call shows: local vs external, which provider
   and model, what data is included, and the budget slot it spends. Private imported source text is
   withheld from a provider until a recorded consent exists for that provider; consent is revocable.
   Credentials are shown only as `present / missing`, never the value.
6. **Anki is an owned integration, not a second SRS.** Pass Quest proposes a small note; the learner
   previews/edits/accepts or skips. Sync states: pushed, queued (Anki not running), already present,
   conflict (learner edited it in Anki — visible, never overwritten silently). AnkiWeb/phone sync is
   the learner's own manual action.
7. **Keyboard-driven study, accessible type, consistent light and dark modes, interruption/resume.**

## Required screens (hash routes) and their content

All prototypes implement exactly these routes, plus the query params below, so that screenshots are
comparable. Each route is a full-window state of the app (the app may have its own navigation model;
the routes are the comparable *states*).

| Route | Screen |
|---|---|
| `#/today` | Today / what's next |
| `#/practice` | Reading practice, before answering |
| `#/feedback` | Graded feedback + AI coach for the same item |
| `#/assess` | Timed unassisted assessment in progress |
| `#/report` | Assessment report + like-for-like comparison |
| `#/anki` | Anki note proposal, preview and sync state |
| `#/settings` | Models, credentials, consent, budget |

Query params: `?theme=light|dark`, `?lang=zh|en` (UI chrome language; exam passage/questions stay
English in both), `?shot=1` (disable all animation/transition and caret blinking so a headless
screenshot is deterministic). The UI must also offer visible theme and language toggles, and both
must be persisted while navigating. Default: light, zh.

### `#/today`
- Prep day **23** (started 2026-10-22). Exam date: not set (action: set). Daily budget: not set.
- **The one next action**, from the transparent policy: `practice_new` on **NOT/EXCEPT 极性**
  (`not-except-polarity`, scoring skill). Reason, naming counts: "独立迁移 3 次，答对 1 次；最近一次错误
  选了与原文重复词最多的选项" / "3 independent transfers, 1 correct; the last miss picked the option that
  reuses the most passage words." The passage will be fresh, so this attempt can count.
- Skill evidence (three skills only — no larger ontology):
  - `not-except-polarity` (scoring): independent 3 (1 correct), assisted 2, re-exposure 1 → action
    `practice_new`
  - `vocabulary-in-context` (language): independent 7 (5 correct), assisted 1 → `review`
  - `sentence-structure` (language): independent 4 (3 correct), assisted 0 → `practice_new`
  - Causal diagnosis is `unresolved` unless evidence supports it — say so.
- Secondary, quiet: unseen assessment pool **63 items / 17 units** (4 units spent; floor 20 items);
  last assessment run #2 on 11-11; Anki: 6 accepted, 1 queued (Anki not running), 1 conflict;
  durable study brief run `brief-7` **paused at step 3/6** (resume / cancel); find practice: no
  search provider configured (0 network calls); tutor model `deepseek/deepseek-chat` verified
  11-13 09:12, budget 4/20 today.

### `#/practice` — the shared item (self-authored placeholder, NOT real exam material)
Label the source as `本地导入 · S1 · 第 24 页 · 已审核` / `Local import · S1 · p.24 · reviewed`, and show a
small disclosure that the prototype text is an original placeholder. Show the assistance condition
*before* answering: `独立 · 新文章 — 本次可计入独立迁移` / `Independent · fresh passage — this attempt can
count`. Show a per-item elapsed clock (learner's own time; not an exam limit). Hint is available but
gated with the assisted warning. Answer by click or keyboard (A–D or 1–4, Enter to submit, H hint,
S skip). Text selection on the passage should offer "mark" (for a later Anki note).

Passage — **Notice: Temporary Closure of the North Pump Station**

> From Monday, 4 March, the North Pump Station will be closed for maintenance for about three weeks.
> During this period, water for the eastern district will be supplied by the South Station. Residents
> may notice lower water pressure between 6 a.m. and 9 a.m., when demand is highest. The lower
> pressure will not affect drinking-water quality, which is tested twice a day.
>
> Engineers will replace two of the four main pumps and install a new control system. The old pumps
> have been in service since 1998 and use nearly 30 percent more electricity than modern models. Once
> the work is finished, the station is expected to run more quietly and to need fewer repairs.
>
> Visitors are not allowed on site while the work is under way. Delivery vehicles must use the west
> gate, and all workers must wear helmets and reflective vests. Questions about the project should be
> sent to the district office rather than to the station itself, because the station's phone line
> will be disconnected until the work is complete.

Questions (the practice screen shows Q1 as the active item; Q2–Q4 can appear as the passage's other
items in the assessment screen or as a list):

- **Q1** (skill `not-except-polarity`) — According to the notice, all of the following are true
  EXCEPT that ____.
  A. the South Station will supply water to the eastern district · B. drinking-water quality is tested
  twice a day · **C. all four main pumps will be replaced** (key) · D. delivery vehicles must enter
  through the west gate
- **Q2** (skill `vocabulary-in-context`) — The phrase "under way" in Paragraph 3 most probably means
  ____. **A. in progress** (key) · B. on the road · C. delayed · D. being planned
- **Q3** (skill `sentence-structure`) — Why should questions be sent to the district office?
  A. The district office manages the engineers. · **B. The station's phone line will not work during
  the project.** (key) · C. Visitors are not allowed on site. · D. The station will close permanently.
- **Q4** (unclassified inference) — What is one expected result of the project?
  A. Water will taste better. · **B. The station will use less electricity.** (key) · C. The eastern
  district will get a new station. · D. Fewer workers will be needed.

### `#/feedback` — Q1 answered
- Learner chose **A**, recorded `incorrect`, condition independent transfer, elapsed **1:48**.
- Verified key **C**. Evidence sentence highlighted in the passage: "Engineers will replace **two of
  the four** main pumps…".
- Per-option explanations (authored, verified):
  A — True: paragraph 1 says the South Station supplies the eastern district. It repeats the most
  passage words, which is exactly why keyword overlap misleads in an EXCEPT item.
  B — True: "tested twice a day".
  C — False: only two of the four pumps are replaced. This is the odd statement → the key.
  D — True: "Delivery vehicles must use the west gate".
- Tactic link: NOT/EXCEPT polarity — "three options are supported, the key is the odd one; word
  overlap measures vocabulary reuse, not truth."
- **AI coach (advisory)**: modes `解释 explain` / `追问 probe` / `微课 micro-lesson`. Shows provider
  `DeepSeek · deepseek-chat · verified`, `外部调用 / external call`, payload preview ("stem, 4 options,
  your choice, verified key, evidence sentence — source S1 p.24"), consent state for S1 (granted for
  DeepSeek by the learner on 11-12, revocable), budget `4/20 today`. Show one completed advisory reply
  (≤ 90 words, e.g. a contrastive probe: "Before you look at the key: which word in option C would you
  check against the passage?" plus a short explanation). It never changes the grade.
- Next steps offered: propose an Anki note from this miss; next item (fresh passage, independent).

### `#/assess` — timed unassisted run
- Run #3, 40 items, 60 minutes, annotations: none. Item **17 / 40**. Run time left **34:12**. Item
  clock **0:52 of 1:30** (per-item = floor(60 min / 40)). Answered 16, timed-out so far 1.
- Shows Q2-style item on a different (fresh) passage or the shared passage — no feedback after
  answering, no glossary, no hints, no AI. A clear visual mode change from practice: this is a test.
- Leaving: "离开后可恢复，计时不暂停 / You can resume later; the clock keeps running."
- Item navigator shows answered / current / pending / timed-out without correctness.

### `#/report` — run #3 complete
- **27 / 40** correct. 及格线：未知 / Pass threshold: unknown. No probability, no verdict.
- Per skill: NOT/EXCEPT 7/10 · vocabulary-in-context 9/14 · sentence-structure 8/12 · unclassified 3/4.
- Time: median **64 s**, p90 **112 s**; timed out **3**; contaminated **0** (items seen before the
  run would be listed and excluded).
- Like-for-like compare with run #2 (11-11): **22 / 40**, same size, same 60-minute window, same pool
  family → **+5 items**. If unlike, the comparison is refused (explain why).
- Withheld annotations (evidence, reasoning) become available now, per item.
- Pool after run: 23 items / 7 units unseen remain (floor 20).

### `#/anki`
- Draft proposed from the Q1 miss. Fields: **Front** `all of the following are true EXCEPT` ·
  **Back** two plain paragraphs (explanation: "Three options match the passage; the answer is the
  one that does not. Check each option against the text — do not pick the one that repeats the most
  words." · example: "Engineers will replace two of the four main pumps…") · **Tactic** `NOT/EXCEPT
  polarity` · **Source** `S1 · p024 · q1` (never the source title).
- Rendered card preview front/back, in light and dark, desktop and phone width. The card style is part
  of the direction: memorable, modern, restrained; offline-safe HTML/CSS.
- Actions: edit fields, accept (revision 1), skip, optional "refine with AI" (advisory, external).
- Note list with sync states: 6 pushed, 1 queued (Anki not running — will push once when reachable),
  1 conflict (edited in Anki; show both versions, keep yours / keep Anki's), plus "already present".
- Remind: phone sync via AnkiWeb is the learner's own action.

### `#/settings`
- Provider registry: OpenAI · Qwen · Doubao · DeepSeek · Gemini · Custom endpoint. Four *separate*
  facts per model: provider, model, capability, authorization. Example rows:
  `deepseek/deepseek-chat` — text.chat `verified@11-13 09:12`, key present, **in use**;
  `openai/gpt-4o-mini` — text.chat declared,unverified, key **no_credential**;
  `qwen/qwen-plus` — declared,unverified, key present (test available);
  `gemini/gemini-2.0-flash` — `failed:auth@11-12 21:40`, key present → action: check key;
  `doubao/…` — no_credential; custom — not configured.
  Capabilities: text.chat, tool.calls, vision, search, speech.synthesis — only text.chat is ever
  verified; the others show declared,unverified or unsupported.
- Budget per provider per day (limit 20): the real ceiling is providers × limit; show per-provider
  counters and the sum. Failover only to a verified provider with budget and an exactly equal consent
  scope.
- Credentials live in the OS secret store; shown as present/missing; never displayed.
- Consent ledger for private source S1: granted to DeepSeek by "Han" on 11-12; revoke appends a
  withholding event. First-party content needs no consent.
- Local services: Agent host (Node) running/stopped; AnkiConnect `127.0.0.1:8765` unreachable;
  search provider: not configured.

## Technical requirements for every prototype file

- One self-contained `.html` file. **No network requests of any kind**: no CDN, no web fonts, no
  remote images, no analytics. Fonts are local stacks. On this Windows machine these exist:
  `Segoe UI Variable`, `Sitka` (serif, variable), `Georgia`, `Cambria`, `Bookman Old Style`,
  `Bahnschrift` (DIN-like, variable), `Cascadia Mono/Code`, `Consolas`, `Noto Sans SC` (variable),
  `Noto Serif SC` (variable), `DengXian`, `Microsoft YaHei UI`, `SimSun`. Always add macOS/Linux
  fallbacks (`-apple-system`, `PingFang SC`, `Songti SC`, `Iowan Old Style`, `SF Mono`, etc.).
- Icons are hand-made inline SVG. No emoji as UI. No icon fonts. No stock illustration.
- Vanilla HTML/CSS/JS only. Hash router for the routes above. A `window.onerror` handler that paints
  a visible red error bar (so screenshots expose JS failures).
- Real interactions, not a picture: theme + language toggles; answering with keyboard and mouse
  (practice → submitting moves to feedback); hint gate dialog; AI mode switching; Anki field edits
  reflected live in the card preview; consent revoke/grant toggle; assessment item navigator;
  keyboard shortcuts with a discoverable shortcut sheet (`?`).
- Complete zh and en strings for all chrome (no leftover mixed language except the deliberately
  English exam content and identifiers such as `text.chat`).
- Target window 1440×900; must still hold at 1280×800 without overflow. Respect
  `prefers-reduced-motion`. WCAG AA contrast in both themes. Visible focus states. Tabular numerals
  for all numbers and clocks.
- Screenshot all states: `bash design/prototypes/shoot.sh <file.html> <dir>` (Git Bash) writes
  `design/prototypes/_shots/<dir>/<route>-<theme>-<lang>.png`; use `ROUTES="practice"` to limit. Read
  the PNGs to look at your work.

## Originality rule

Do not imitate or reference the look of any existing product (Notion, Linear, Figma, Arc, Things,
Duolingo, Anki itself, Material, Fluent, Apple HIG templates, Tailwind UI, shadcn, dashboard kits…).
Derive every form from *this* product's truths: evidence vs inference, independent vs assisted,
verified vs advisory, local vs external, unknown vs known, reading as the core act.
