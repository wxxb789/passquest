# Pass Quest — Core Design

> **Purpose:** Give a person with limited English proficiency the best defensible chance of passing a specific written English examination within a constrained preparation window. Build an agentic, local-first exam coach—not a general English school, a flashcard clone, or a collection of AI features.
>
> **Status:** Foundational design for pre-0.1 development. Revisit assumptions when real exam or learner evidence contradicts them. Nothing here implies a guaranteed pass rate.

## 1. Product taste

**Optimize for passing the exam, not for broad language proficiency.** The meaningful unit of progress is independently demonstrated examination performance per unit of scarce study time. Words learned, cards created, prompts sent, minutes studied, and pages imported are costs or intermediate evidence—not success.

A great Pass Quest experience feels like a competent, discreet coach: it knows what to practice next, why, when to stop explaining, when to test, and when not to create an Anki card. Users should spend their attention answering exam-like questions, not managing an AI system.

Ambition is welcome; accidental complexity is not. Prefer a small system with clear truth ownership and strong evidence over elaborate knowledge graphs, multi-agent theater, gamification, or a large preinstalled content library.

## 2. Known facts, assumptions, and the exam contract

- Initial user: an adult learner roughly at junior-high-school English level; approximately 90 days to prepare; desktop-first; **no listening or speaking in the examination**.
- The presently available source is a 165-page, 2025 CRSC engineering senior-title foreign-language preparation PDF. It contains 560 bilingual terms, 69 bilingual passages, six PETS-style mock reading sets with explanations, and additional cloze/reading material. Some pages are scans.
- Its first page says **"仅限内部使用，严禁外传"**. This document is NOT permission to redistribute it, upload it to an LLM, include it in a public repository, or sync its verbatim content to a third-party service.
- The actual examination blueprint, scoring weights, pass threshold, exam date, allowed aids and relationship between the supplied material and live questions are **unknown**. Do not invent them.
- Daily study budget, operating system, preferred provider and user authorization are configurable and may initially be unknown. They must not block an honest local, testable first slice.

An **Exam Profile** expresses known exam constraints, candidate question families, provenance and unresolved assumptions. It is dynamically revised when new credible evidence about the examination is discovered; previous versions remain traceable. A separate **Adaptive Study Profile** continuously updates *learner-specific* strengths, scoring-skill needs and practice priorities as the learner improves. Changing the learner's ability must not rewrite external exam facts or quietly change a verified answer. Any claim about exam frequency or pass probability is labeled as an estimate and accompanied by its source and uncertainty. Never confuse familiarity with the provided material with verified live-exam coverage.

The learner's privately imported material is the primary *working context*, not proof of an exam blueprint. The first user-supplied 165-page PDF is the intended **baseline**. Support normalizing it into useful, page-linked UTF-8 `.txt` (and structured verified questions where appropriate), including scanned pages after locally authorized extraction and human/automated verification. A raw PDF-to-text dump is not a valid completed baseline if most scanned pages are absent. The user explicitly requested that two **unreviewed** text extractions be included in this currently public repository: the earlier 22-page native-text file at `data/baselines/2025-crsc-english-partial.txt` and, as a separate recorded exception on 2026-11-10, the page-linked parsed text of all 165 pages at `data/baselines/2026-crsc-english/` (22 native-text pages plus 143 OCR pages). Both are project-specific publishing exceptions, **not verification of redistribution rights** and not permission for agents to publish additional raw or derived source material. The original PDF stays at `data/private/source/` (gitignored); no further OCR of new material and no new source text is committed, exported, bundled or uploaded to an external provider without a new recorded decision. A change of extension does not remove the source's confidentiality notice.

## 3. The essential product loop

**Observe → Diagnose → Select → Coach → Attempt independently → Verify → Adapt.**

1. **Observe:** Record question origin, independent answer, elapsed time, hint exposure, repeated exposure, and relevant context.
2. **Diagnose:** Distinguish exam-strategy mistakes (negated questions, evidence location, distractors, time allocation), minimum language prerequisites (keywords, sentence structure), and uncertainty. A wrong option alone does not reveal a causal misconception.
3. **Select:** Choose the next action for plausible exam-score gain within the learner's remaining time. Candidate actions include review, explain, contrastive probe, timed practice, transfer, assess and find material. A transparent policy remains understandable without an LLM.
4. **Coach:** Favor minimal effective interventions. Teach test-taking tactics and only the English needed to apply them. Do not rely on folklore such as “the longest answer is correct”; explain limits and verify tactics on unseen tasks.
5. **Verify:** Independent, unassisted, unfamiliar and preferably delayed performance matters more than assisted correctness. Promote evidence cautiously and correct earlier assumptions when evidence changes.

**Scores and evidence, not model eloquence, determine completion.** If the exam pass threshold is unknown, show observed accuracy, time, topic-specific failure modes, coverage and uncertainty; do not show an invented percentage chance of passing.

## 4. Domain truths and ownership

Maintain conceptual separation, not gratuitous microservices:

- **Exam intelligence:** question/passages, answer evidence, exam profile, source provenance, scoring skills and prerequisite relationships.
- **Learning evidence:** immutable-in-spirit attempts, exposure/hints, elapsed time, transfers, corrections and chronological changes. Derived learner state (acquisition, retention, transfer and confidence) is replaceable/recomputable, not an alternative source of truth.
- **Learning policy:** conservative, inspectable choice of interventions under time and confidence constraints. Policy versions and reasons should be explainable; do not pretend to have trained a precise personalized reward model from one learner.
- **Assessment:** deterministic grading where a verified key exists; transparent handling of disputed/uncertain questions; holdout by entire passage or source cluster rather than splitting sibling questions. Training and formal independent evaluation must not contaminate each other.
- **Content:** user-imported local material, small verified first-party fixtures, validated user-supplied additions and on-demand discovered resources. Distinguish verified source questions from AI variants, unverified candidate questions and formal evaluation items. Preserve licensing and evidence of answer review.
- **Anki notes:** the Pass Quest canonical note identity, factual content, source and sync state; Anki owns its notes/cards and review scheduling after export. Never replace Anki with a homegrown spaced-repetition system.

Make quality claims proportional to evidence. There is no need for a vast ontology: the smallest effective set of **scoring skills** (evidence location, NOT/EXCEPT, paraphrase, elimination, pacing, cloze constraints) and **prerequisite skills** is sufficient until observed errors demand refinement.

## 5. The agent is powerful but not the authority

Use the Pi ecosystem, specifically **pi-durable** for durable conversation/tool work and **pi-ai** for model access. The agent can explore the learner's evidence, request diagnostic probes, explain, generate bounded teaching content, propose Anki notes, seek authorized external practice, and reflect on ineffective interventions.

The agent cannot silently:
- change scores, historical attempts, answer keys or held-out status;
- declare a generated question authoritative;
- export confidential text or private learner data to external models/search;
- install arbitrary tools, execute unrestricted commands, or spend unbounded provider credits;
- send, delete or overwrite user-edited Anki notes without appropriate controls.

Give it narrow, semantic domain tools—e.g. get learner evidence, propose a lesson, request an approved practice item, propose a note, submit a resource candidate—rather than unrestricted filesystem/network/SQL write access. Tool outputs and fetched pages are **untrusted input**, never higher-priority instructions. Tool behavior must be versioned/testable and scoped by task, consent and budget.

**Durability is not exactly-once side effects.** Persistent task state survives process restarts within storage guarantees; external actions may require confirmation and reconciliation. The application must use stable idempotency keys, an outbox and read-back reconciliation for effects such as Anki note creation. A crashed model turn must not duplicate cards or falsify an attempt.

## 6. Local-first runtime and trust boundaries

Chosen direction:
- **MyGo + Go** is the desktop shell and deterministic application core, including scoring, learning evidence/policy, import, resource governance and Anki integration.
- **Web frontend + TypeScript** provides text-rich reading interactions and polished desktop UX when its later goal is active. MyGo's typed bridge belongs to Go-facing UI actions, not directly to the agent's database.
- **Pi Durable + pi-ai**, hosted in a private local Node.js/TypeScript process, provides the agent loop, providers, tools and recovery.
- **SQLite** is the durable store. The Go application owns its application schema; the Pi host owns its own runtime schema, ordinarily in separate SQLite files to avoid co-writer/lifecycle coupling. One process is the writer/authority for each logical state domain.
- **A versioned, narrow local protocol** connects Go and the Pi host: semantic requests, events, cancellation, timeouts, bounded streaming and restart-safe request identities. Choice of IPC transport is an implementation decision; it must not rely on arbitrary open network exposure.
- **No central server or account** is required to practice. The desktop must work without an LLM for verified local exam attempts and grading. External AI and web search may be unavailable and should degrade visibly.

The shipping design must account for the Node runtime and agent assets; a MyGo Go binary alone does not imply a single-file distribution. Target Windows/macOS/Linux pragmatically, verify platform-specific packaging and lifecycle rather than assuming parity. The developer's initial model choice (DeepSeek v4.1 Flash) must not determine product behavior or artifact quality.

## 7. Provider model, permissions and cost

**Text generation is baseline; everything else is optional.** Design a provider/model/capability/authentication boundary supporting OpenAI, Qwen, Doubao, DeepSeek, Gemini and user-specified compatible endpoints. Auth modes are separate from vendors: a BYOK API key is not a ChatGPT subscription. Offer supported account-based login only where a documented, eligible official integration exists; otherwise provide a clear API-key path. Never invent subscription entitlement or universal model capabilities.

Discover or configure per-model capabilities rather than asserting that all providers support web search, transcription, speech synthesis or image generation. Search can be implemented as a separately authorized tool; speech/image features remain additive, not prerequisites. Credentials must live in the OS secret store, never plaintext in app SQLite, prompts, logs, Anki fields or exports. Show whether a request is local or external, what data is included, and provide per-task budget/cancellation, retry limits and meaningful failure handling.

## 8. Anki as an owned integration, not a second study platform

Primary path: **AnkiConnect** talking to a local running Anki Desktop, with a dedicated Pass Quest note model and cross-platform HTML/CSS templates. AnkiWeb and AnkiMobile/AnkiDroid handle mobile synchronization; Pass Quest must not claim that local Anki writes automatically synchronize to phones. A portable import/export fallback is worthwhile but must not replace the simplest day-to-day path.

A user can select content or accept an agent suggestion; an AI proposes a small useful note, and the user can inspect/edit it. Card types may cover recognition of high-value terminology, compact exam patterns and tactical rules. Do not create a note for every encountered word; a new card adds future review debt. Do not routinely demand productive English spelling where the exam needs only recognition.

Card design is **memorable, modern and restrained**: typography, whitespace, accessible contrast, dark mode, concise context and *semantically useful* diagrams/images where appropriate. No decorative image spam, no model-generated copyrighted assets, no requirement for image generation. Anki templates should render without network access or exotic JavaScript and remain usable on mobile. The user owns their Anki edits and review history; updates must be reconciled rather than blindly overwriting them.

Pass Quest owns canonical note IDs, content revision and sync intents; Anki owns its note IDs and scheduling. Support queueing offline, idempotent upserts, interruption recovery and conflict visibility. Exporting confidential source excerpts to AnkiWeb counts as external data transfer and is not automatic consent.

## 9. Resource acquisition: just enough, just in time

Begin with small, credible redistributable test fixtures, the openly visible **unreviewed parsed-text baseline** (`data/baselines/2026-crsc-english/`) committed at the user's direction, and complete source material privately imported at runtime when available. Treat normalized TXT as a first-class import format with page-level provenance and verification status; the learner can improve text and answer mapping over time. Do not publicly publish restricted material merely because it was converted from PDF. No bundled mass corpus or generic English curricula. The Resource Agent gets a **specific gap**: scoring skill, learner level, target difficulty, provenance requirement, desired assessment/training use and time/cost budget. It searches permitted sources, records links and rights, deduplicates, and proposes usable material. Prefer authoritative exam-family sources over arbitrary SEO exercises.

A candidate is not a verified exam item. Keep provenance, original vs transformed vs generated distinction, confidence and review status. LLM-generated practice may teach, but high-stakes assessment requires separately validated answers and uncontaminated material. Provide a source-link-only mode where redistribution rights are unclear; reject prompt injection and prohibited transfers. Unused transient downloads can expire, while high-value learning evidence persists.

## 10. Product quality, evidence and aesthetic sequence

There are two different development stages, deliberately:

1. **Headless functional stages:** a real local user or test harness can import a small source, take a question, receive correct feedback, preserve evidence, run controlled AI coaching, generate an Anki note and verify recovery—without polished UI. A thin CLI/test harness is acceptable as a temporary access surface; screenshots and mock-only demos are not proof.
2. **Desktop experience stages:** MyGo/WebView supports fluid reading, contextual coaching, Anki previews and examination simulation. The later Opus-led UI pass is free to refine information hierarchy and interaction patterns, without rewriting domain truth.

No agent-generated fluff, congratulatory noise, endless modal dialogs or routine human approval gates. Make the next useful action obvious; expose uncertainty, provenance and costs without making study feel like operations management. The product should gracefully support keyboard-driven study, visual card previews, accessible type, consistent light/dark modes and interruption/resumption.

Every goal must produce an observable capability and evidence, not simply create a database/API/architecture layer. Keep goals in `docs/goals/`, ascending `0001-` filenames, completed ones later archived under `docs/goals/archive/` if the project adopts that convention. Each provisional future goal is refined using feedback before activation. Neither goal files nor this design prescribe a mandatory implementation checklist or a permanent runtime scheduler.

## 11. Explicit non-goals (initial product)

General English proficiency certification, listening/speaking curricula, social learning, leaderboards, reward economies, cloud accounts, remote sync servers, vector databases, graph memories, autonomous multi-agent swarms, broad scraping, own Anki scheduler, guaranteed score forecasts, and public redistribution of source exam material.

An optional feature becomes justified only when it measurably reduces time-to-exam-readiness or resolves a demonstrated learner failure.

## 12. Invariants to protect during iteration

1. **Correctness before fluency:** verified answers and source boundaries outrank AI confidence.
2. **Independent performance before assisted success:** hints and repeated passages never masquerade as holdout improvement.
3. **Gain per study minute before content volume:** training and Anki note creation each incur costs.
4. **User control over external effects:** secrets, licensed/private materials, Anki notes and provider spending receive appropriate permission and reversible operations.
5. **Recoverable, idempotent operations:** restart cannot silently double-post or lose acknowledged learner truth.
6. **Replaceable inference:** changing AI models, pedagogical strategies or estimators does not rewrite historical evidence.
7. **Agent freedom within durable contracts:** encourage better educational ideas while protecting evidence, provenance, billing and permissions.
8. **Honest uncertainty:** pass threshold, exam blueprint and measured outcomes remain explicitly unknown until supported.
9. **Small, usable increments:** the first demonstrable study loop precedes architecture ornamentation; high-fidelity UI comes after the engine is useful.

## 13. Relevant authoritative references (informative, not hard-pinned versions)

- [MyGo documentation](https://github.com/egoist/mygo/blob/main/docs/README.md)
- [Pi Durable documentation](https://github.com/earendil-works/pi/blob/main/packages/durable/README.md)
- [AnkiConnect current project](https://git.sr.ht/~foosoft/anki-connect) (the former GitHub repo is archived)
- [Anki card styling manual](https://docs.ankiweb.net/templates/styling.html)
- [Anki sync manual](https://docs.ankiweb.net/syncing.html)
- [Sign in with ChatGPT official integration guidance](https://developers.openai.com/cookbook/articles/sign-in-with-chatgpt)

These references document intended integration surfaces; runtime compatibility and permissions must be checked against versions actually selected at implementation time.
