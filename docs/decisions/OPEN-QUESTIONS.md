# Open questions

Owner-only decisions, unresolved concerns and defaults taken under the uncertainty protocol in
[`../conventions.md`](../conventions.md). Agent runs **append** here instead of blocking; the owner
resolves at the next intervention. Never delete an entry. Status values: `open`, `decided(default
taken)`, `resolved`.

Entry format:

```markdown
### Q-<goal-id>-<nn> — <one-line question>
- Status: open
- Context: what forced the question
- Options considered: A / B / C
- Default taken: what the run did, and how to reverse it
- Revisit when: the trigger
```

---

### Q-0000-01 — Unattended merge authorization for goals 0001–0010
- Status: decided(default taken)
- Context: the owner asked for unattended development of goals 0001–0010 through the goal-driven
  delivery flow. That flow ends in a squash merge, which is a write to `main` on a public repository.
- Options considered: (A) merge each goal when the merge gate is green; (B) leave every PR open for
  the owner; (C) merge only after the owner reviews the first goal.
- Default taken: (A) for this goal range only, and only when `scripts/pr-merge-check.py` reports the
  head verified, CI green, the codex marker `has_any_findings: false` on that same head, zero
  unresolved threads, and local gates re-run. Any failure leaves the PR open and the goal blocked.
  Revoke by saying so; the flow then stops at "PR open".
- Revisit when: the owner returns, or a merge gate produces a false green.

### Q-0000-02 — Project instruction file location
- Status: open
- Context: `AGENTS.md` at the repository root is the file coding agents read first by convention. Its
  creation required an interactive approval that was unavailable during the unattended run, so the
  binding conventions currently live in `docs/conventions.md`.
- Options considered: (A) create `AGENTS.md` as a thin pointer to `docs/conventions.md`; (B) keep
  `docs/conventions.md` and pass it explicitly in every dispatch prompt; (C) fold conventions into
  `docs/core-design.md`.
- Default taken: (B) for now; dispatch prompts name `docs/conventions.md` and `docs/harness.md`.
- Revisit when: the owner is present — one approval creates the pointer file.

### Q-0000-03 — Toolchain pinning
- Status: decided(default taken)
- Context: the host provides Go through `mise`; a contributor without mise needs a documented floor.
- Options considered: (A) pin in `mise.toml` plus `go.mod` directive; (B) a container/CI-only
  toolchain; (C) no pin.
- Default taken: (A) — `mise.toml` pins Go and Node for the dev host, `go.mod` is the authority for
  CI and for the language version.
- Revisit when: a second machine or a contributor joins.

### Q-0000-04 — Two agent loops are writing the same worktree and branch
- Status: resolved
- Resolution: (owner, 2026-11-10) the second loop is confirmed stopped — no process with a
  `~/repos/passquest` cwd, no `omp-passquest-*` tmux session, no `.git` lock, and the last foreign
  touch was the 01:20:56 commit; a single writer (the delivery loop) resumed at the `apply` stage by
  instruction. Option (A) file-scope splitting is therefore unnecessary, and the fetch-then-rebase
  guard in `conductor.py` stays as the standing protection against a recurrence.
- Context: while the agent loop was running goal 0001, two owner commits appeared on the same branch
  in the same clone (`5e89188` D-0001 Bun/TS harness, `4a861ca` parsed private source). The reflog
  shows a `reset --hard` + two commits + push from another process at the same second, i.e. a second
  agent/loop edits `~/repos/passquest` directly. Two writers in one worktree can interleave stage
  output and make a reviewed head unreproducible.
- Options considered: (A) split file scopes — the other loop owns `data/`, `docs/decisions/`,
  `harness/`; the delivery loop owns `cmd/`, `internal/`, `docs/goals|plans|evidence`, both
  fetch-then-commit and stop on conflict; (B) the other loop works only in its own clone/worktree;
  (C) the delivery loop stops until the owner finishes.
- Default taken: (C) temporarily — the unattended conductor was stopped and the in-flight `apply` run
  killed before it could commit. The working tree is clean at `4a861ca` and `bash scripts/gate.sh`
  still reports `GATE: PASS` (45 links, 214 tracked files). The conductor now fetches and rebases
  before every commit and push, blocks on a real conflict, and never force-pushes, so it can be
  resumed safely under (A).
- Revisit when: the owner answers, or a second concurrent commit appears while the conductor runs.

### Q-0001-01 — May a learner repeat a question after seeing the key, and how is it counted?
- Status: decided(default taken)
- Context: goal 0001 requires correct, incorrect, skipped and repeated attempts to be represented
  truthfully, but does not say whether repeats are allowed or how they relate to independent
  evidence. Goal 0003 (tactics) and goal 0009 (readiness) will interpret the same records.
- Options considered: (A) allow repeats, keep every attempt, label repeats and exclude them from
  first-attempt counts; (B) block a question once its key has been shown; (C) allow repeats and count
  every attempt equally.
- Default taken: (A). The CLI stores the repeat with `attempt_number`, `prior_attempts` and
  `repeat`, and `report` prints first and repeated counts separately. Reverse by changing the
  selection/report policy only; stored attempts stay intact. The Store API and SQLite triggers
  reject `UPDATE`, `DELETE` and an id-reusing `INSERT OR REPLACE`, so a future policy can be
  recomputed from the same evidence.
- Revisit when: goal 0004 selects practice from evidence, or goal 0009 defines independent
  readiness and needs a stricter exposure rule.

### Q-0002-01 — Is a locally installed `pdftotext` an acceptable PDF import dependency?
- Status: decided(default taken)
- Context: goal 0002 requires importing a locally held source PDF. Go's standard library cannot read
  PDF, and the owner's recorded parse recipe already uses poppler `pdftotext`/`pdftoppm`. A pure-Go
  PDF parser would be a new module with weaker CID/CJK font coverage; bundling an OCR engine is
  explicitly out of scope.
- Options considered: (A) call the local `pdftotext` binary when present, otherwise refuse PDF import
  and point at the TXT path; (B) add a pure-Go PDF module; (C) require the user to pre-convert PDF to
  TXT outside the app.
- Default taken: (A). PDF import makes one local `pdftotext -layout -enc UTF-8` call (path
  overridable with `PASSQUEST_PDFTOTEXT`) and requires `pdfinfo` for the page count (overridable with
  `PASSQUEST_PDFINFO`); both tools must be present, and it never downloads or OCRs anything. A
  missing tool is exit 2 with the TXT instructions. The extractor sits behind `source.ImportOptions`,
  so a pure-Go implementation can replace it without changing the CLI or schema. Reverse by removing
  the PDF branch.
- Revisit when: a target platform cannot install poppler, or PDF import produces materially worse
  text than a maintained pure-Go parser on the owner's source.

### Q-0002-02 — Review provenance and local protection without accounts or encryption
- Status: decided(default taken)
- Context: the contract requires user-reviewed material and private preservation across restart, but
  the project has one local learner, no accounts and no desktop secret storage until goal 0011.
- Options considered: (A) free-text reviewer labels plus a `0600` unencrypted SQLite database;
  (B) invent a local account/identity; (C) encrypt the database with a user-supplied passphrase.
- Default taken: (A). Every page/question review records `reviewed_by` (a local label the user types)
  and `reviewed_at`; the database stays `0600` in a `0700` directory and is gitignored. The label is
  attribution, not authentication, and the evidence record says so. Reverse by adding an identity or
  encryption layer later; the columns remain.
- Platform limit (2026-11-10): the `0700` rule is enforced for the app-owned default `.passquest/`
  path on POSIX, including an existing directory created before the rule; a directory chosen with
  `--db` or `PASSQUEST_DB` is left to the user. Windows has no POSIX mode and `os.Chmod(0o600)` does
  not create an ACL, so the import smoke reports its database-mode assertion as skipped there. The
  `0600` database file remains the private-data boundary on POSIX.
- Revisit when: the desktop workspace (0011) stores multiple learners, or private source material
  needs at-rest encryption.

### Q-0002-03 — Is a source page or a whole document the practiceable content unit?
- Status: decided(default taken)
- Context: goal 0001's scoring model is one passage plus questions. A 165-page document contains many
  unrelated passages and mock sets; treating it as one content value would make the passage and
  attempt scoping meaningless.
- Options considered: (A) one content per source page (`<source-id>-p<NNN>`); (B) one content per
  source document with page references inside; (C) one content per question.
- Default taken: (A). Attempt numbering and repeat exposure stay scoped to
  `(content_id, question_id)`, a correction changes the content version but not its id, and a page is
  the natural passage boundary for this material. Goal 0003 can group pages into a tactic exercise
  without rewriting attempts. Reverse by adding a grouping layer; stored attempts keep their ids.
- Revisit when: goal 0003 needs multi-page passages, or goal 0009 needs whole-set holdout scoping.

### Q-0003-01 — Which scoring tactic is demonstrated, and is one enough?
- Status: decided(default taken)
- Context: goal 0003 names NOT/EXCEPT polarity, evidence location and distractor paraphrases as
  examples and requires a counterexample or boundary. A concrete tactic must be chosen before lesson
  content, item tags and the smoke gate can be designed.
- Options considered: (A) NOT/EXCEPT polarity, with a keyword-overlap counterexample; (B) locating
  textual evidence; (C) avoiding distractor paraphrases; (D) several tactics at once.
- Default taken: (A). NOT/EXCEPT polarity has a deterministic odd-statement rule, a crisp
  counterexample against both "repeat the most words" and "pick the longest option", and it does not
  require source text in the repository. The registry and the question `tactics` field accept more
  tactics; adding one needs its own lesson, boundary and tested exercise, so (D) was deferred rather
  than shipped untested. Reverse by registering another tactic and tagging questions with it; stored
  attempts keep their `tactic_id`.
- Revisit when: goal 0004 selects practice from evidence, or the learner's observed errors point at a
  different tactic than polarity.

### Q-0003-02 — What separates an assisted attempt from independent correctness?
- Status: decided(default taken)
- Context: the contract requires an assisted success and an independent correctness to stay
  separately visible, and goal 0001 already records first/repeat exposure. "Help shown" and "answer
  already shown" are different facts, but the contract does not define the boundary.
- Options considered: (A) store assistance as a separate append-only field and require independence
  to be a first attempt on a different question; (B) treat any first attempt as independent, even
  after a walkthrough; (C) treat a repeat without help as independent.
- Default taken: (A). `assistance` (`assisted`/`independent`) and `tactic_id` are stored per attempt;
  a guided attempt shows the walkthrough and is assisted; an independent attempt shows no guidance,
  must be a different question from every guided example for that tactic, and must have zero earlier
  attempts on that content-scoped question. The first tactic call defaults to guided and later calls
  default to independent; an independent call before any guided attempt is refused with exit 2. If no
  eligible question remains, the command refuses rather than substituting. Existing rows and ordinary
  practice default to independent. Reverse by changing the selection policy and report grouping; the
  stored fields remain.
- Revisit when: goal 0009 defines delayed or held-out assessment, or the learner needs a second
  independent transfer item after the first one is used.

### Q-0003-03 — Are tactic tags on local questions a closed registry or free text?
- Status: decided(default taken)
- Context: imported questions are authored locally as JSON. A tactic can only select questions that
  declare they exercise it, but a typo or an unknown tag could silently remove a question from every
  tactic.
- Options considered: (A) require a tag to name a registered tactic and reject unknown tags at
  `question add`/`edit`; (B) accept any lowercase tag and let selection ignore unknown ones;
  (C) infer the tactic from the question stem automatically.
- Default taken: (A). Tags share the lowercase-slug grammar with tactic ids and are checked against
  the build's registry; an unknown tag is an exit-2 invalid-input error naming the registered ids.
  This keeps a typo visible and prevents silent ineligibility. The cost is that adding a tactic needs
  a code change, which is acceptable while tactics are authored, reviewed content. Reverse by
  relaxing validation and adding a separate tag-vocabulary table.
- Revisit when: users need to author their own tactic vocabularies, or goal 0004 introduces a larger
  skill taxonomy that should not be a compiled list.

### Q-0004-01 — Is a new question on an already-seen passage independent transfer?
- Status: decided(default taken)
- Context: goal 0003 records `assistance=independent` for a first attempt on a distinct question and
  calls it independent. Goal 0004 requires that "the same question or passage exposure does not
  masquerade as independent transfer", and feedback on any question reveals the passage and its
  evidence.
- Options considered: (A) keep goal 0003's question-level rule and treat a new question on a seen
  passage as independent; (B) count only the first independent attempt on a fresh content id as
  independent transfer, and show later first attempts as passage re-exposure; (C) refuse a new
  question on a seen passage entirely.
- Default taken: (B). Stored attempts are unchanged; the adaptive policy classifies an independent
  first attempt on an already-seen content id as `passage_repeats`, never as independent correctness.
  Goal-0003 `t1`/`t2` therefore remain an assisted attempt plus an independent attempt in the report,
  but the adaptive view shows one passage and zero independent transfers. Reverse by removing the
  content-id condition in `internal/adaptive`; no migration is needed.
- Revisit when: goal 0009 defines a delayed or held-out assessment whose passage exposure rule is
  intentionally different, or a learner needs transfer measured on a passage they have already read.

### Q-0004-02 — How small is the skill set, and which evidence thresholds are allowed?
- Status: decided(default taken)
- Context: goal 0004 asks for "a small set of scoring and minimal language prerequisite skills" and
  requires recommendations to react to attempts, time, hints and revisits without claiming a measured
  pass probability. The thresholds decide when a skill is called weak, mixed or supported.
- Options considered: (A) one scoring skill plus two language prerequisites, with exact-count states
  and a learner-relative pace rule; (B) a larger taxonomy with percentage thresholds and an
  estimated mastery score; (C) one skill only, with no time rule.
- Default taken: (A). `not-except-polarity` (scoring), `vocabulary-in-context` and
  `sentence-structure` (language). States use exact counts: zero independent transfer is
  `familiarity_only`, one answered observation is `insufficient`, two or more with zero correct is
  `weak`, some correct is `mixed`, all correct is `supported`. "Slow" means the latest correct
  independent answer took at least twice the learner's own fastest correct answer for that skill; no
  exam time limit is invented. Vocabulary has three first-party passages so a supported state can
  still have a fresh passage; sentence structure has two. Reverse by editing the registry thresholds
  and authored content; stored attempts are unaffected.
- Revisit when: real learner evidence shows the exact-count states are too coarse, or the owner wants
  a different skill set or more passages.

### Q-0004-03 — Does the question-file `tactics` field now accept language skill tags?
- Status: decided(default taken)
- Context: goal 0003 named the field `tactics` and validated it against the tactic registry. Goal
  0004 needs question tags for language prerequisites, and renaming the JSON field would break every
  authored question file and the goal-0003 protocol.
- Options considered: (A) keep `tactics` as the field name and widen its values to any registered
  skill; (B) rename it to `skills` and migrate question files; (C) add a second `skills` field and
  accept both.
- Default taken: (A). The field keeps its name and lowercase-slug grammar; validation now uses the
  skill registry, where a scoring tactic is a skill kind. Human output labels the values `skills`.
  Reverse by renaming the field with a one-release alias once existing local files can be migrated.
- Revisit when: a question-file schema version is introduced or users need independent tactic and
  language tag fields.

### Q-0004-04 — Are skill tags snapshotted on attempts?
- Status: decided(default taken)
- Context: adaptive inference groups historical attempts by a skill. A question tag can be edited
  after the attempt, which would otherwise regroup old evidence under a new label.
- Options considered: (A) add an append-only `skills_json` snapshot to attempts and use it for
  inference; (B) recompute every attempt from the question's current tags; (C) store a full derived
  learner state.
- Default taken: (A). Schema v6 adds a nullable `attempts.skills_json`; new attempts always write a
  JSON array (including `[]`), and `NULL` marks a pre-v6 row. A later `question edit` cannot regroup a
  recorded snapshot, even an empty one. Pre-v6 rows fall back to current tags, marked
  `mapped_from_current_tags` with an uncertainty line. No derived learner state is stored; `next`
  recomputes it. Reverse by ignoring the column and reading current tags, at the cost of historical
  regrouping.
- Revisit when: a tag-correction workflow needs to apply retroactively, or pre-v6 learner evidence
  must stop being inferred from current tags.

### Q-0004-05 — Does the guided-first rule apply to scoring `skill practice`?
- Status: decided(default taken)
- Context: Q-0003-02 requires an independent attempt to be a different question from every guided
  example and refuses an independent tactic call before a guided attempt. Goal 0004 adds
  `skill practice`, which the plan calls ordinary unassisted practice. On a fresh database it could
  select the tactic exercise's guided example, record it as independent, and make the tactic's own
  independent stage unreachable; the adaptive view then counted that answer as scoring transfer
  before the walkthrough. The review asked whether the rule is universal or scoped to
  `tactic practice`.
- Options considered: (A) enforce Q-0003-02 on the scoring skill practice surface; (B) scope the rule
  to `tactic practice` and amend the decision text.
- Default taken: (A). `skill practice` for a scoring skill refuses with exit 2 (`guided_required`)
  until a guided attempt for that skill exists, and candidate selection skips any question whose
  walkthrough was already shown. Language skills are unaffected. `next` counts `guided_attempts` from
  attempts that carry the tactic id, not from every assisted attempt, and a scoring skill with
  recorded attempts but no guided tactic attempt selects `explain`; an assisted ordinary answer after
  a walkthrough still selects the guided explanation instead of printing an unrunnable command.
  This is the safer default: it cannot consume the guided-example question or overstate independent
  scoring evidence. Reverse by removing `requireGuidedTactic` and the exposure skip in
  `internal/cli/skill.go`; stored attempts are unaffected.
- Revisit when: the owner wants a separate unassisted scoring surface that does not feed adaptive
  transfer evidence, or goal 0009 defines a held-out assessment with different help rules.

### Q-0004-06 — Does an exhausted review always yield to another skill's evidence-adding action?
- Status: decided(default taken)
- Context: round-1 review found that with only embedded material the scoring skill reaches
  `insufficient`, has no fresh passage, and selects `review` at priority 2 forever. A review cannot
  add an independent observation, so the untouched language skills at priority 5 were never
  recommended. The same logic applies to any review-only skill, including a `supported` slow review.
- Options considered: (A) every `review` action ranks after every action that can add independent
  evidence, and reviews compete only when every material-bearing skill is on review; (B) only
  under-sampled reviews (`familiarity_only`, `insufficient`) yield, preserving the slow-time review
  above `no_evidence`; (C) change the review action to `explain` when no fresh passage remains.
- Default taken: (A). `internal/adaptive.Recommend` selects a non-review candidate before a review
  candidate; among reviews the existing state priority and registry order still decide. This removes
  the permanent stall. The disclosed trade is that a slow `supported` review changes the skill's
  inferred action and reason but does not become the top recommendation while another skill has an
  action that can add evidence; the skill list still shows it. Reverse by changing the candidate
  comparison in `Recommend`; stored attempts and the per-skill inference are unaffected.
- Revisit when: goal 0009 defines a delayed or held-out assessment whose consolidation signal should
  outrank fresh material, or real learner evidence shows the top recommendation should prefer a
  review over an unpracticed skill.

### Q-0004-07 — Does an exhausted explanation yield to another skill's evidence-adding action?
- Status: decided(default taken)
- Context: round-2 review found that with only embedded material, two wrong `sentence-structure`
  answers leave the skill `weak` with `explain` at priority 0, and its lesson followed by
  `skill practice --question s2` records no independent observation; the untouched vocabulary skill
  was therefore never recommended. The same trap exists for an exhausted `mixed` skill whose latest
  answered independent attempt was incorrect. Q-0004-06 covered `review` only, and the selection rule
  demoted reviews alone.
- Options considered: (A) at selection, rank any candidate that cannot add evidence — a review, or an
  `explain` with no fresh passage — after one that can, while keeping the skill's own inferred
  action; (B) change the `weak`/`mixed` branches to `review` when no fresh passage remains; (C) keep
  the explanation as the unconditional top recommendation.
- Default taken: (A). `internal/adaptive.Recommend` compares candidates by whether they lead to a
  fresh passage; the skill's `inference.action` stays `explain` because the lesson is still its own
  need, and the explanation is selected again once every skill is exhausted (its priority 0 outranks
  a review). Reverse by changing the candidate comparison in `Recommend`; stored attempts and the
  per-skill inference are unaffected.
- Revisit when: goal 0009 needs a consolidation signal to outrank fresh material, or the owner wants
  an exhausted repeated-error lesson to keep the top slot.

### Q-0005-01 — Where does a BYOK provider credential live before the desktop secret store?
- Status: decided(default taken)
- Context: goal 0005 requires a user-supplied text endpoint, while the contract also says credentials
  must not be written into learning data, exports, logs or public artifacts. The OS secret store
  named by the core design does not exist until the desktop workspace (goal 0011), and this build is
  a CLI.
- Options considered: (A) environment variable only (`PASSQUEST_TUTOR_API_KEY`), inherited by the
  host process and never persisted; (B) a `0600` local secret file next to the database; (C) a
  passphrase-encrypted database column or a new local key service.
- Default taken: (A). `tutor config` stores only endpoint, model, timeout, output bound and daily
  budget; the key is read by `agent/tutor-host.mjs` from the environment, placed only in the
  Authorization header and redacted from stored/printed error and model text. Reverse by adding a
  keychain/file provider behind the same host request; no stored data changes.
- Revisit when: goal 0011 provides an OS secret store, or a real learner needs a persisted credential
  without exporting an environment variable each session.

### Q-0005-02 — Which process may contact the provider?
- Status: decided(default taken)
- Context: the local-only gate forbids `net`, `net/http`, `net/rpc` and `net/smtp` in first-party Go
  packages, and the core design names a private local Node process for the Pi agent. Goal 0006 is the
  durable Pi runtime; goal 0005 only needs one bounded text call.
- Options considered: (A) a single-shot Node host (`agent/tutor-host.mjs`) invoked by the Go CLI over
  a versioned stdin/stdout JSON protocol, with the host as the only network egress; (B) relax the Go
  network rule and use `net/http` in the core; (C) defer the provider path until Pi Durable lands.
- Default taken: (A). Ordinary practice, review, `next`, `report`, `config`, `preview`, `history`
  and consent commands need only Go; `ask` requires Node and fails with exit 3 `agent_unavailable`
  otherwise. The host has no database, filesystem or tool access. Reverse by moving the transport
  into the Pi runtime behind the same request shape (goal 0006) or by changing the local-only gate
  with a recorded decision.
- Revisit when: goal 0006 integrates Pi Durable, or a supported provider requires a transport that
  the single-shot host cannot express.

### Q-0005-03 — Is imported source text sent to the provider by default?
- Status: decided(default taken)
- Context: the contract requires the learner to inspect, avoid or authorize any private source
  material sent to the provider, and the owner's source is restricted. The embedded first-party
  exercises are redistributable; imported pages, questions and evidence are private.
- Options considered: (A) withhold imported text by default, show the exact payload in `preview`,
  and require an append-only per-source `tutor consent` event; (B) send the attempted question and
  passage by default with an opt-out; (C) never send imported source text, even with consent.
- Default taken: (A). Without a grant the provider receives attempt facts (status, selected/correct
  labels, elapsed, assistance, attempt number, repeat, skill ids), registered first-party skill
  lessons and an explicit “question content withheld” marker. A grant adds a bounded passage window
  and the question fields; `revoke` appends a withholding event. The source title and path are never
  sent. Reverse by changing the privacy classification in `internal/tutor.Build`; the consent trail
  remains.
- Revisit when: the owner records a rights decision for a specific source, or learners need
  per-excerpt rather than per-source control.

### Q-0005-04 — Which request shape is the one supported BYOK endpoint?
- Status: decided(default taken)
- Context: goal 0010 later covers OpenAI, Qwen, Doubao, DeepSeek, Gemini, a custom endpoint and
  optional capabilities. Goal 0005 needs one honest, testable text path now; provider-specific
  marketing claims are not acceptance criteria.
- Options considered: (A) OpenAI-compatible `chat/completions` with a full URL, bearer auth,
  `max_tokens`, `temperature: 0`, no streaming and no redirects; (B) add the OpenAI SDK or several
  provider SDKs; (C) build a vendor-capability matrix now.
- Default taken: (A). `https` is required except on loopback; URLs with userinfo, query or fragment
  are rejected. One request per call, no retry, no streaming. The host validates the response shape,
  bounds and usage; unknown provider capabilities are not inferred. Reverse by adding a provider
  adapter under the same host protocol in goal 0010.
- Revisit when: goal 0010 starts provider choice, or the owner's authorized endpoint is not
  chat-completions compatible.
