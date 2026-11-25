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
- Status: resolved
- Resolution: (owner, 2026-11-11) option (A) confirmed: a root `AGENTS.md` now exists as a thin
  pointer to `docs/conventions.md` and the rest of the read order. It restates no rule of its own.
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

### Q-0000-05 — Are the CLI goldens contract evidence or test fixtures?
- Status: resolved
- Resolution: (owner, 2026-11-13) option (A) confirmed: the protocol and history goldens under
  `internal/cli/testdata/` (`*.golden.json`, `*golden.txt`) **are** contract evidence. A golden change
  is a contract change, so the gate refuses a diff that touches a `*golden*` file there without also
  touching a contract-bearing document — an evidence record under `docs/evidence/`, a goal contract
  under `docs/goals/`, or a decisions record under `docs/decisions/` (including this file) — and names
  every golden it saw plus the document that is missing. The rules live in
  [`../harness.md`](../harness.md) (gate 21) and the check is `harness/gates/golden-contract.ts`,
  proven both ways by `harness/gates/self-test.ts` (a golden-only change fails; a new golden plus its
  evidence row passes).
- Context: the goldens pin the CLI's JSON protocol and its human history output, but nothing stopped
  `-update-golden` from rewriting one to make a red test green, which silently changes the protocol a
  learner's scripts and the agent bridge depend on. The 2026-11-13 quoting change moved three `next`
  goldens for a documented reason, which is exactly the case the record must distinguish from a silent
  rewrite.
- Options considered: (A) treat a golden as contract evidence and fail a golden change that no
  contract document declares; (B) keep goldens as ordinary fixtures and rely on review; (C) require an
  owner approval marker on every golden change.
- Default taken: (A). The check is diff-based against the branch's merge base (falling back to
  `HEAD^1` in CI's shallow pull-request checkout, or `PASSQUEST_GOLDEN_BASE`), and an unresolvable base
  fails instead of passing silently. Reverse by deleting the gate from `harness/gates/index.ts` and
  this entry's rule from `docs/harness.md`.
- Revisit when: a golden change is genuinely mechanical across many files at once, or a second golden
  directory appears outside `internal/cli/testdata/`.

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
- Platform limit extended (2026-11-25, goal 0020 P1): the app-owned default directory is the per-user
  one both entry points resolve (Q-0020-01, `internal/userdata`), and the `0700` tightening follows
  it; the `.passquest/` path named above is the historical command-line default, now only the subject
  of the notice (Q-0020-02). The exempt choices are three — `--db`, `PASSQUEST_DB` and
  `PASSQUEST_DESKTOP_DATA` — each the learner's own selection, left as given, not modified.
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
- Moved (2026-11-18): `requireGuidedTactic` and the exposure skip now live in
  `internal/study/practice/lesson.go` (refactor/study-seam); the reversal now touches
  `internal/study/practice` and the `internal/cli/skill.go` adapter.

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
- Revisited (2026-11-20): goal 0011 added the OS credential store for the desktop app (Q-0011-03); the
  environment variable still wins, and the CLI still reads only the environment.

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
- Revisited (2026-11-20): goal 0006 added the Pi Durable host beside the single-shot tutor host rather
  than replacing it; Node hosts remain the only provider egress. Goal 0015 added one recorded Go
  exception, `internal/update`, which the `local-only` gate pins to the release-check endpoint.

### Q-0005-03 — Is imported source text sent to the provider by default?
- Status: resolved
- Resolution: (owner, 2026-11-11) option (A) confirmed: imported source text reaches the provider only after an
  explicit per-source `tutor consent`; `preview` shows the exact payload and `revoke` appends a
  withholding event. The implementation already does this and is unchanged.
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
- Revisited (2026-11-20): goal 0010's provider registry kept this one request shape for every provider,
  including the custom endpoint (`docs/providers.md`); no provider SDK was added.

### Q-0005-05 — Should the Node host enforce the same request bounds as the Go client?
- Status: decided(default taken)
- Context: the simplify stage found `agent/tutor-host.mjs` accepted `max_tokens` 1–4096 and 24,000
  message characters while `internal/tutor` and [`../tutor.md`](../tutor.md) state 64–2048 and
  20,000. The CLI revalidates every request, so the looser host bound was not reachable through a
  supported command, but the host is the product's only network egress and should not accept a
  wider shape.
- Options considered: (A) align the host to the documented Go bounds; (B) document the host's
  looser second line as intentional; (C) remove validation from one side and keep a single check.
- Default taken: (A). The host now rejects `max_tokens` outside 64–2048 and message content above
  20,000 characters with `config_error`, and `tutor-smoke` probes both values directly. The Go
  client, the record and the docs were already the authority; the change makes the second line
  match them. Reverse by editing the two constants in `agent/tutor-host.mjs`; no stored data or
  protocol shape changes.
- Revisit when: a supported provider needs a larger bound than the documented one, or the host
  protocol publishes an intentional host-only allowance.
- Review fix (2026-11-10): the host counted UTF-16 code units while the Go client and this document
  mean code points, so a 15,000-astral-character prompt passed `Build` and was then rejected as
  over 20,000 units, and a 12,000-code-point response could be refused. `agent/tutor-host.mjs` now
  counts `Array.from(text).length` for the message total and the response text; `tutor-smoke` sends a
  15,000-code-point astral prompt and a 12,000-code-point astral response. Bound values are unchanged.

### Q-0005-06 — What happens to a daily-call reservation left by a crashed `tutor ask`?
- Status: decided(default taken)
- Context: round-2 review found that two concurrent `tutor ask` processes could both pass the daily
  budget check, because neither appended a call until after the provider responded. `ask` now claims
  a slot in `tutor_call_reservations` inside an immediate transaction before spawning the host, and
  the recorded call replaces that claim in the same transaction. A process that dies between the
  claim and the record, or a storage failure while recording it, leaves the row behind.
- Options considered: (A) leave the claim spent until the next UTC day; (B) reclaim claims older
  than the provider timeout plus a margin; (C) keep the race and document the budget as best-effort.
- Default taken: (A). The crashed process may already have posted a billable request, so the claim
  is kept and the budget can only under-spend. Reverse by deleting rows from
  `tutor_call_reservations`; no call or attempt record changes.
- Revisit when: goal 0006 gives the host a durable call lifecycle that can report a call as finished
  or abandoned, or an operator needs the slot back before midnight.

### Q-0006-01 — Which bounded agent task and side effect demonstrate interruption safety?
- Status: decided(default taken)
- Context: goal 0006 requires a real, nontrivial multi-step AI task whose side effect can be
  interrupted/retried without duplicate learner evidence or content. The options had to fit before
  Anki notes (0007) and resource discovery (0008), and could not invent an answer key or publish
  private text.
- Options considered: (A) a `study-brief` that records 2–3 advisory teaching steps through an
  application-owned command; (B) a content-analysis task that proposes lesson rows; (C) an Anki-note
  outbox, which would pre-empt goal 0007.
- Default taken: (A). The packet contains recorded attempt facts, derived skill state and authored
  lesson text only; the only tool is `record_study_step`, idempotent on `run_id:step_index`, with one
  ledger row per invocation and one `reconciled` event per duplicate. `PASSQUEST_AGENT_FAULT=kill-after-step`
  proves a crash after the commit reruns the same call without duplicate content. Reverse by adding a
  new task kind and tool; stored steps and attempts are unaffected.
- Revisit when: goal 0007 or 0008 introduces a note/resource effect, or real use shows the brief is
  not useful enough to keep as the durable task.
- Revisited (2026-11-20): goals 0007 and 0008 gave notes and found resources their own commands and
  records (`note`, `find`); the study brief stays the durable task with its one tool.

### Q-0006-02 — Is Pi Durable worth a product-runtime dependency now, and where does its state live?
- Status: decided(default taken)
- Context: the contract names the intended Pi Durable runtime, which is experimental (1.5.1), requires
  Node ≥ 22.19 and pulls a large dependency tree. The core cannot import network packages, and the
  application and agent have different lifecycles.
- Options considered: (A) pin Pi Durable as a Bun workspace under `agent/`, with its own SQLite file;
  (B) reimplement a durable loop in Go/Node; (C) keep the single-shot tutor host and defer.
- Default taken: (A). `agent/package.json` pins `@earendil-works/pi-durable`, `pi-ai` and `chord` at
  1.5.1; `bun install` at the root installs the workspace; the Pi store is
  `<db dir>/<db base>-agent.sqlite` (`PASSQUEST_AGENT_STORE` overrides) and is recorded in the run.
  Agent commands need Node; ordinary practice and the Go core do not. Reverse by replacing the host
  behind the versioned document; the application run/effect tables stay.
- Revisit when: Pi's API changes incompatibly, packaging goal 0013 starts, or the dependency tree is a
  material install problem on a target platform.
- Updated (2026-11-18): the pins are now 1.5.2 (`agent/package.json`); the store location and the
  reversal are unchanged.
- Revisited (2026-11-19): goal 0013 bundles the pinned Node 22.23.5 runtime, the four host scripts and
  the hosts' hoisted production `node_modules` (`@earendil-works/*` 1.5.2) into every package; the
  installed app's coach and refinement calls run through that runtime, and with it deleted every
  agent surface — the durable brief included — reports `node_unavailable` with one action. The store
  location and the reversal are unchanged.
- Revisited (2026-11-21): goal 0016 ships no `node_modules` at all — the one host that loads packages
  is staged as a self-contained bundle whose seven inlined packages travel with their notices, and
  the other three hosts are byte-identical copies (Q-0016-01, Q-0016-05). The store location and the
  reversal are unchanged.

### Q-0006-03 — How is one run paused, resumed and cancelled across processes?
- Status: decided(default taken)
- Context: Pi allows one writer per storage, so a second process cannot safely write the durable state
  while the host runs. The contract also forbids invisible uncontrolled execution and requires
  running/paused/completed/failed/needs-action visibility.
- Options considered: (A) application events plus POSIX signals and a 1-second heartbeat file;
  (B) a long-lived agent service with an IPC socket; (C) terminal Ctrl-C only, with no cross-process
  cancel.
- Default taken: (A). `start`/`resume` run one host; a timeout or CLI signal sends SIGTERM, the host
  closes with work pending, the application records `interrupted` and exits 4. `cancel` appends
  `cancel_requested` first, sends SIGINT on POSIX (durable abort) or kills on Windows, waits up to
  `--wait-seconds` for the heartbeat to stop, escalates to a kill, and records `cancelled` with
  `confirmed`; resume refuses afterwards. The host is placed in its own process group so terminal
  signals are the CLI's decision. One unfinished run is allowed per application database. Reverse by
  replacing the signalling layer with a service; the event trail remains authoritative.
- Revisit when: goal 0011 introduces a desktop lifecycle, or a target platform cannot stop a child
  promptly.
- Revisited (2026-11-20): goal 0011's desktop lifecycle kept this protocol and ends every host on quit
  (Q-0011-11); the durable host pauses on SIGTERM, so a brief interrupted by quit resumes later.

### Q-0006-04 — Which failures are `failed` and which are `needs_action`, and do agent calls count against the tutor budget?
- Status: decided(default taken)
- Context: the contract lists `failed` and `needs action` as distinct observable states but does not
  define the boundary. Goal 0005 already has a daily-call ledger for one-call tutoring.
- Options considered: (A) user-actionable provider/host failures are `needs_action`; malformed or
  non-retryable task failures are `failed`; agent runs use a turn bound rather than the tutor ledger;
  (B) every non-completed run is `failed`; (C) every provider failure is `needs_action` and agent
  calls consume the daily ledger.
- Default taken: (A). `missing_credential`, 401/403, 429, model/config 4xx, timeout, network, 5xx,
  an `interrupted` run and `agent_unavailable` are `needs_action` with an action hint; `invalid_response`, host protocol or
  storage errors, `no_steps`, `turn_limit`, `task_faulted`, `orphaned` and `restart_limit` are
  `failed`. `resume` is the retry path; it refuses a completed run and a `no_steps` failure. A run is
  capped at `max_turns + 1` provider requests and the configured timeout, not the `tutor` daily
  ledger. Reverse by changing the classifier and adding a run-level reservation.
- Revisit when: goal 0010 adds provider capabilities/budgets, or a real learner needs one spend cap
  across tutor and agent calls.
- Revisited (2026-11-20): goal 0010 made the daily budget one counter per provider for single calls
  (Q-0010-04); agent runs are still capped by their turn bound and timeout, not the ledger.

### Q-0006-05 — Does a resumed run use the current model or the recorded one?
- Status: decided(default taken)
- Context: a run can be interrupted, then the learner changes `tutor config` (endpoint, model,
  timeout, output bound) before resuming. Switching models mid-task changes the durable conversation's
  context and can invalidate prior output; keeping the old model can preserve a typo.
- Options considered: (A) bind the run to its recorded model but use the current endpoint, timeout and
  credential; (B) always use the current model; (C) refuse resume when any configuration changed.
- Default taken: (A). `resume` uses the model stored on the run, so a corrected endpoint or credential
  is picked up but a model rename needs `cancel` + `start`. The status/resume text says so. Reverse by
  configuring the conversation to the current model before resume; the transcript records the model
  per answer.
- Revisit when: goal 0010 exposes provider/model choice or a run needs planned model hand-off.
- Revisited (2026-11-20): goal 0010 kept (A): agent runs keep their recorded provider and model, and
  `resume` is their retry path (Q-0010-08).

### Q-0006-06 — Is a copied `next` command POSIX shell text or per-platform shell text?
- Status: resolved
- Resolution: (owner, 2026-11-13) option (B) confirmed: `next` renders each copyable command for the
  detected shell family — `sh`-family, `cmd.exe` or PowerShell — and every family is pinned by table
  tests over hostile paths (spaces, `$`, backticks, `"`, `'`, `%`, `!`, non-ASCII, a trailing Windows
  separator). The POSIX form is additionally round-tripped through a real `sh -c`. Detection order,
  its visible disclosure and the residual limits are recorded in
  [cross-platform.md](../cross-platform.md); `next --json` carries
  `shell.{family,detected_from,detail,override}`. `cmd.exe` has no way to suppress `%NAME%` expansion
  outside a batch file, so a rendered command containing `%` is printed with a warning and the limit
  is disclosed instead of being claimed fixed. The Windows and PowerShell forms are pinned as text
  and were not executed on the Linux host of this change; the 3-OS CI run is the remaining evidence.
- Context: `quoteCommandArgument` (`internal/cli/next.go`) emitted a POSIX form: double quotes with
  `\$`, `` \` `` and `\"` escaping, and treated `\`, `/`, `:` and `@` as safe characters. That was
  correct for `sh`/`zsh`/`bash` and wrong for `cmd.exe` and PowerShell, where `$HOME` expands
  differently and a path with a space needs different quoting. The product targets Windows, macOS and
  Linux, so the learner on Windows copied a command that might not run.
- Options considered: (A) emit the POSIX form and say so in the command's surrounding text; (B) render
  the command for the detected shell (`sh`-family vs `cmd.exe` vs PowerShell) and pin each form with
  table tests; (C) print a shell-neutral form only (no quoting, refuse paths with spaces).
- Default taken: (A) from 2026-11-10 until this resolution; it was recorded as a **known gap**, not a
  decision. The gap closed by replacing the single POSIX renderer with `internal/cli/shell.go`; no
  stored attempt, database column or command name changed.
- Revisit when: a learner reports a copied command that still fails in their shell, or goal 0007/0012
  introduces a copyable command whose paths leave the ASCII-safe set.

### Q-0007-01 — Can the AnkiConnect path be proven without Anki Desktop on this machine?
- Status: decided(default taken)
- Context: goal 0007 lists Anki Desktop/AnkiConnect as the primary local path, but the environment has
  no Anki binary and nothing answers `127.0.0.1:8765`; CI cannot reach a learner's desktop on any OS.
- Options considered: (A) prove the adapter against a loopback AnkiConnect stub and keep the real desktop
  as owner evidence; (B) block the goal until Anki is installed here; (C) ship export only and drop the
  adapter.
- Default taken: (A). The stub speaks the documented AnkiConnect shape (`version`, `createDeck`,
  `createModel`/`modelNames`, `addNote`, `updateNoteFields`, `duplicate`), and every report states that a
  real deck write is unverified until the owner runs it. Export remains the offline fallback.
- Revisit when: the owner runs the app beside a real Anki Desktop, or Anki changes its protocol.

### Q-0007-02 — Which deck and note model does a note land in?
- Status: decided(default taken)
- Context: a deck name is visible to the learner and a model name pins the field schema; both are cheap to
  change but expensive to migrate after cards exist.
- Options considered: (A) deck `Pass Quest`, model `PassQuest Note`, fields `Front`, `Back`, `Tactic`,
  `Source`; (B) reuse Anki's built-in `Basic` model; (C) one model per content type.
- Default taken: (A). `Basic` cannot carry tactic and source context without stuffing them into one field,
  and per-type models would multiply migration cost before the content shape is known. The deck name is a
  flag, not a constant.

### Q-0007-03 — How much visual design belongs to this goal?
- Status: decided(default taken)
- Context: "beautiful, concise" is in the outcome, but goal 0012 owns the visual system.
- Options considered: (A) tidy minimal CSS that survives Anki mobile; (B) a designed template now;
  (C) unstyled text.
- Default taken: (A): system font stack, no external asset, no fixed height, readable at phone width.

### Q-0007-04 — May a note carry text from the internal-only source?
- Status: resolved
- Resolution: (owner, 2026-11-11) option (A) confirmed: restricted text may reach a note only after the goal-0005
  consent is recorded, and AnkiWeb sync stays the learner's own manual act. The app never claims to
  push anything to a phone.
- Context: the 165-page source is internal-only; an Anki note can leave the machine through AnkiWeb.
- Options considered: (A) restricted text is written only after the goal-0005 consent is recorded, and the
  app says AnkiWeb sync is the learner's own action; (B) always include the source sentence; (C) never
  include it.
- Default taken: (A). Without it a note cannot teach the pattern it came from, and the consent ledger
  already exists; the app never claims to push anything to a phone.

### Q-0007-05 — What makes a word or pattern worth a note?
- Status: decided(default taken)
- Context: "high-value" is the goal's qualifier and has no definition.
- Options considered: (A) an incorrect or skipped attempt, or an explicit learner mark; (B) every question
  seen; (C) only wrong answers.
- Default taken: (A). Every question floods the deck, wrong-only misses the patterns a learner notices
  themselves; the mark is one flag.

### Q-0007-06 — Does a revision update the existing card or add a new one?
- Status: decided(default taken)
- Context: Anki review history lives on the note; re-adding on edit would reset it.
- Options considered: (A) keep the note id locally and use `updateNoteFields`; (B) delete and re-add;
  (C) create a new note and leave the old one.
- Default taken: (A); a local revision that disagrees with what Anki holds is surfaced as a conflict, not
  silently overwritten.

### Q-0008-01 — Which search provider, and what happens with no credential?
- Status: resolved
- Resolution: (owner, 2026-11-11) option (A) confirmed: one adapter behind `PASSQUEST_SEARCH_ENDPOINT`/`_KEY` with
  `no_provider` as a first-class outcome, proven against a loopback stub. A real-provider run is
  deferred until a credential exists and is owner evidence, not a gate.
- Context: goal 0008 needs external discovery, but no provider key or endpoint exists in this environment and
  CI cannot reach a real provider from any OS.
- Options considered: (A) one adapter behind `PASSQUEST_SEARCH_ENDPOINT`/`_KEY`, proven against a loopback
  stub, with "no provider configured" as a first-class outcome; (B) block until a credential arrives;
  (C) hard-code one vendor's SDK.
- Default taken: (A). The policy this goal owns — bounded intent, provenance, rights, budget, injection
  containment — is provable without a vendor; a vendor SDK would bury it.
- Revisit when: a real credential exists, or a second provider must be supported.

### Q-0008-02 — What may be shown or downloaded from a result?
- Status: decided(default taken)
- Context: external content carries copyright risk, and the app already refuses to bundle the internal-only
  source.
- Options considered: (A) classify each candidate `link-only` / `local-usable` / `generated-unverified`, and
  download nothing by default; (B) fetch and store every result; (C) show only links.
- Default taken: (A). A link with provenance still helps, `generated-unverified` must never become study
  state, and `local-usable` requires an explicit open-license field from the provider.
- Revisit when: a curated open-license source is added on purpose.

### Q-0008-03 — How large is a search budget?
- Status: resolved
- Resolution: (owner, 2026-11-11) option (A) confirmed: 5 queries/day and 20 candidates/run as visible flags with
  persisted counters; a refusal names the remaining allowance instead of truncating. The numbers stay
  flags, so a real study budget is one argument away.
- Context: "no unlimited scraping or search costs" is the goal's constraint; the learner's real budget is
  unknown and the goal must not invent one silently.
- Options considered: (A) 5 queries/day and 20 candidates/run as visible flags with persisted counters;
  (B) no limit but a warning; (C) one query per day.
- Default taken: (A). Both are flags, so a different number is one argument, and a refusal names the
  remaining allowance instead of truncating.
- Revisit when: the daily study budget is known.

### Q-0008-04 — What counts as the observed gap that justifies a search?
- Status: decided(default taken)
- Context: "a learner's observed skill gap" has no definition and a search must not fire on noise.
- Options considered: (A) highest `(wrong + skipped) / attempts` ratio among skills with at least three
  attempts, tie broken by attempt count, else the most recent wrong answer's tactic, else "nothing to find";
  (B) the skill with the most wrong answers in absolute terms; (C) the learner asks.
- Default taken: (A). A ratio keeps a 3-of-3 skill above a 10-of-100 one, three attempts is the smallest count
  that is not a coin flip, and the two fallbacks keep the command honest instead of silent. Clause 1 of the
  goal, this entry and the acceptance checks use this same rule.
- Revisit when: real practice volume makes a better signal visible.

### Q-0008-05 — How is an injected instruction in fetched text contained?
- Status: decided(default taken)
- Context: web pages can carry text that reads like a command, and the app has an agent loop that can act.
- Options considered: (A) fetched text is stored as data only, never parsed for instructions, and a stub page
  carrying "ignore previous instructions" must leave scoring and the next exercise unchanged; (B) rely on the
  model to resist; (C) strip suspicious sentences by keyword.
- Default taken: (A). Keyword stripping invites a bypass; not treating page text as instructions at all is the
  property to prove.
- Revisit when: fetched text is ever fed to an agent prompt.

### Q-0008-06 — How is "the learner's level" derived without a stored profile?
- Status: decided(default taken)
- Context: contract clause 1 requires the bounded search intent to carry the learner's level, but no
  learner profile, CEFR assessment or exam-readiness measure exists or should be invented. Goal 0009
  owns defensible readiness evidence.
- Options considered: (A) an evidence-derived bucket over first, independent attempts, with its counts
  printed; (B) a fixed "junior-high" profile from the core design; (C) require the learner to choose a
  level before searching.
- Default taken: (A). Fewer than three first independent attempts is `early`; otherwise the correct
  ratio is `foundation` below 0.40, `developing` below 0.70 and `consolidating` at or above 0.70,
  with skips in the denominator. The document carries `level_basis` and a limitation stating this is
  not a CEFR, junior-high or exam-readiness measurement. Reverse by changing `find.InferLevel`; no
  stored data changes.
- Revisit when: goal 0009 defines a readiness measure, or the owner supplies a real learner level.

### Q-0008-07 — Which license identifiers count as an explicit open license?
- Status: decided(default taken)
- Context: contract clause 3 makes `local-usable` depend on "an explicit open-license field", but a
  provider can send any string in that field. Accepting every non-empty value would treat
  `all-rights-reserved` as permission to reproduce text.
- Options considered: (A) a small allowlist of open text licenses, with an unrecognised value
  downgraded to `link-only`; (B) accept any non-empty license string; (C) require the owner to approve
  each provider license manually before any candidate is usable.
- Default taken: (A). The allowlist is `cc0-1.0`, `pdm-1.0`, `public-domain`, `cc-by-3.0`,
  `cc-by-4.0`, `cc-by-sa-3.0` and `cc-by-sa-4.0`; `nc` and `nd` variants are excluded because they
  are not open for unrestricted local study. A present but unrecognised license is stored as
  link-only with reason `license_not_open`. The license remains provider-declared, not independently
  verified. Reverse by editing the allowlist in `internal/find`.
- Revisit when: a supported provider uses a different documented open-license vocabulary, or a
  curated source is added on purpose.

### Q-0008-08 — What is an embedded skill's "exam section"?
- Status: decided(default taken)
- Context: contract clause 1 requires the intent to carry an exam section. Imported source pages carry
  a stored section (`notice`, `mock-exam`, …), but first-party sample and skill passages have no exam
  section and the app must not invent an exam blueprint.
- Options considered: (A) use the source page's stored section, `embedded` for first-party content and
  `unavailable` when the page cannot be read; (B) invent a section from the skill kind; (C) omit the
  field for embedded content.
- Default taken: (A). The field is metadata from the attempt's content, not a claim about the live
  examination; `embedded` keeps the document complete without inventing one. Reverse by changing the
  CLI section resolver; persisted queries keep the value they recorded.
- Revisit when: an Exam Profile with real section definitions exists, or first-party content is
  deliberately tagged with a section.

### Q-0007-07 — Must AnkiConnect be reached through a Node host?
- Status: decided(default taken)
- Context: the local-only gate forbids `net/http` in first-party packages, a note can contain private
  source text, and AnkiConnect is an HTTP endpoint. Goal 0013 will eventually own a packaged runtime
  lifecycle.
- Options considered: (A) a second narrow Node host over the existing stdin/stdout pattern; (B) allow
  `net/http` in an `internal/anki` package; (C) export only, with no adapter.
- Default taken: (A). `agent/anki-host.mjs` is a 1–50 action allow-listed loopback transport; Go owns
  endpoint validation, duplicate/missing-note/conflict classification and all queue state. Only
  `note push` needs the Anki host; drafting (without `--refine`), previewing, accepting, skipping,
  showing, exporting and the model view work with Go alone, while `propose --refine` uses the existing
  tutor host. Reverse by replacing `anki.HostTransport` behind the same `anki.Transport` interface.
- Revisit when: goal 0013 packages the desktop app with its own runtime lifecycle, or Node is absent
  on a target platform.
- Revisited (2026-11-19): each package carries the pinned Node runtime and `agent/anki-host.mjs`; the
  package smoke pushes a note through the installed app's own bundled runtime and host, and with the
  runtime deleted the push reports `node_unavailable` with one action while the queue keeps its note.

### Q-0007-08 — Is every imported source restricted for notes, and may accept write before consent?
- Status: decided(default taken)
- Context: the source model has no rights classification finer than the goal-0005 private-source
  rule, and the contract requires restricted text to wait for a recorded consent.
- Options considered: (A) every imported source is restricted, preview withholds front/back, accept
  refuses until consent, and export/push re-check current consent; (B) only a marker-detected source
  is restricted; (C) local accept is allowed and only export/push are gated.
- Default taken: (A). A note without its example would be misleading, and a false "public" is
  irreversible. `note preview` remains send-free and shows only first-party tactic text plus source
  identifiers until consent; `accept` refuses with `consent_required`. A revoke blocks a later export
  or push but does not delete an accepted local note. Reverse by adding a per-source rights field and
  relaxing `noteEffectiveFor`/`runNoteAccept`.
- Revisit when: an imported source carries an explicit redistribution or public-domain grant.
- Moved (2026-11-18): `noteEffectiveFor` is now `draftOf` in `internal/study/notes/notes.go` and the
  accept write is `notes.Settle` in `internal/study/notes/settle.go` (refactor/study-seam);
  `runNoteAccept` in `internal/cli/note.go` is the CLI adapter the reversal now touches.

### Q-0007-09 — What is note identity, and is a skipped attempt unanswered?
- Status: decided(default taken)
- Context: re-answering a question must not add a second card; a learner may mark two different words
  on one question; a skipped attempt records a key the learner never selected.
- Options considered: (A) one note per content + question + normalized mark, with a skipped attempt
  treated as unanswered and an incorrect attempt allowed to include the key; (B) one note per
  attempt; (C) one note per question always.
- Default taken: (A). `note-<sha256(target_key)[:16]>` is stable across processes; an edited accept
  appends a revision. A skipped or unattempted draft never writes or returns the correct label, keyed
  option text, keyed option reasoning or question reasoning; it reads those fields only to replace a
  key-revealing passage sentence with the visible stem. Reverse by changing `note.Target.Key` and the
  unanswered branch; stored revisions remain.
- Revisit when: a learner needs two unmarked notes per question, or goal 0009 defines a different
  passage-exposure rule for notes.

### Q-0007-10 — How is a provider refinement recorded and budgeted?
- Status: decided(default taken)
- Context: the contract allows refining a draft through the goal-0005 endpoint, but `tutor_calls`
  requires an attempt id and a coaching mode, while a mark-only note may have no attempt.
- Options considered: (A) a separate append-only `note_refinements` table sharing the tutor daily
  budget through the same reservation ledger; (B) reuse `tutor_calls` with a new mode and attempt 0;
  (C) omit provider refinement.
- Default taken: (A). `note propose --refine` reserves one slot, records the model, prompt version,
  request SHA-256/bytes, output, usage and failure, and releases the reservation in the same
  transaction. `tutor history` reports the shared daily count but lists coaching calls only. Reverse
  by adding a dedicated note budget display or removing the flag; deterministic drafts are unaffected.
- Revisit when: goal 0010 introduces provider/capability choice or per-task budgets.
- Revisited (2026-11-20): goal 0010 kept the shared ledger; note refinements now count against the
  per-provider counter of the provider they reached (Q-0010-04, `docs/providers.md`).

### Q-0009-01 — What item count and time window does an assessment use?
- Status: resolved
- Resolution: (owner, 2026-11-12) option (B) confirmed: `--size 40 --minutes 60` as visible flags, with the
  values used always printed in the report. The real CRSC item count and duration remain unknown, so a
  learned spec is a two-argument change, not a code change.
- Context: the whole report shape depends on the window, and no authoritative exam spec is available here.
- Options considered: (A) the owner supplies the real numbers now; (B) 40/60 as explicit flags; (C) a small
  first pass at 20 items / 30 minutes.
- Default taken: (B).

### Q-0009-02 — Does the report show a pass threshold?
- Status: resolved
- Resolution: (owner, 2026-11-12) option (A) confirmed: no threshold. The report prints accuracy and speed
  bands plus `pass threshold: unknown`, and never a pass probability.
- Context: the real passing score is unknown; a fabricated threshold is the most damaging false claim the
  product could make.
- Options considered: (A) none, accuracy and speed bands only; (B) a dual track with an owner-set target;
  (C) a fixed target line such as 70%.
- Default taken: (A).

### Q-0009-03 — What is the minimum viable holdout pool?
- Status: resolved
- Resolution: (owner, 2026-11-12) option (A) confirmed: refuse to start below 20 qualifying unseen items and
  report the shortfall. A looser practice pool that also feeds assessments is out of scope; if the pool is
  exhausted the next step is a separate goal, not a weaker exposure rule.
- Context: at 20 items one item is 5% of the score, which is the smallest count that is still readable.
- Options considered: (A) 20 items; (B) 40 items; (C) no floor, warn only.
- Default taken: (A).

### Q-0009-04 — What counts as prior exposure to an item?
- Status: resolved
- Resolution: (owner, 2026-11-12) option (A) confirmed: any presentation — `practice`, `next`, `show` or a
  derived note — spends the item for assessment purposes, whether or not it was answered. The **unit** of
  spending is the content unit, not the question: presenting one question spends its whole passage or
  source cluster, because a learner who has read the passage can no longer answer its sibling questions
  independently (Core Design, Assessment).
- Context: counting only wrong answers keeps the pool alive but admits items whose key the learner already
  saw, which inflates the independent score.
- Options considered: (A) any presentation; (B) only an incorrect or assisted attempt; (C) two pools, a strict
  assessment pool and a loose practice pool.
- Default taken: (A). Option (C) is the natural successor if the pool proves too small in real use.

### Q-0009-05 — Are glossary and reasoning visible during a run?
- Status: resolved
- Resolution: (owner, 2026-11-12) option (A) confirmed: hidden during the run (`--annotations none` is the
  only value this goal accepts), printed in the report afterwards.
- Context: the real examination has no glossary, but early learners may not recognise the stem vocabulary.
- Options considered: (A) hide everything; (B) keep the glossary, hide the reasoning; (C) show everything.
- Default taken: (A).

### Q-0010-01 — Which providers get real credentials in this goal?
- Status: resolved
- Resolution: (owner, 2026-11-12) option (A) confirmed: none. The registry, a loopback stub and every typed
  failure are proven without a key; a real provider call is owner evidence recorded in the goal report.
- Context: CI cannot reach a provider from any OS, and a key in this environment adds a secret surface without
  adding gate coverage.
- Options considered: (A) none, stub only; (B) one or two providers the owner already holds; (C) the cheapest
  single provider.
- Default taken: (A).

### Q-0010-02 — Is account-based login supported?
- Status: resolved
- Resolution: (owner, 2026-11-12) option (A) confirmed: BYOK keys only. No account login, no pasted session
  token, no subscription-token extraction in this goal.
- Context: login is the only part of this goal with a terms-of-service and credential-custody risk.
- Options considered: (A) BYOK only; (B) documented device-code OAuth for named providers; (C) pasted token.
- Default taken: (A).

### Q-0010-03 — How many models does the registry declare per provider?
- Status: resolved
- Resolution: (owner, 2026-11-12) option (A) confirmed: one cheapest documented text model per provider, with
  every other model listed `declared, unverified` and never offered as working until `model test` verifies it.
- Context: a hand-maintained registry with many entries goes stale quickly and turns `model_missing` into the
  normal experience.
- Options considered: (A) one model per provider; (B) three to five tiers per provider; (C) only the providers
  the owner will actually use.
- Default taken: (A).

### Q-0010-04 — Is the daily call budget shared or per provider?
- Status: resolved
- Resolution: (owner, 2026-11-12) option (B) confirmed: **one counter per provider**, each at the goal-0005
  daily value. Because N providers therefore allow N times the calls, `model list` and the call report print
  every per-provider counter and their sum, and the goal report states the ceiling as
  `providers × per-provider limit` rather than as one global cap.
- Context: this number is the real spending ceiling; a shared counter is predictable but blocks every provider
  once one is exhausted, which the owner judged worse than the multiplicative ceiling.
- Options considered: (A) one shared counter; (B) independent per-provider counters; (C) shared total with a
  per-provider soft cap.
- Default taken: (B). Reverse by changing the counter key back to the provider-agnostic ledger.

### Q-0010-05 — May a failed call retry on a different provider?
- Status: resolved
- Resolution: (owner, 2026-11-12) option (B) confirmed, with the consent condition made explicit: one retry on
  another provider is allowed only when the target model is verified for every capability the request needs,
  the target's recorded consent scope is exactly equal to the failing request's consent scope, the target has
  budget left, and it is not the provider that already failed. Every failover prints
  `failover: <from> → <to> (reason: <type>)` and is recorded on the call ledger; any consent difference
  refuses failover and names the difference. Failover never widens what private text can reach.
- Context: the question is a privacy question, not an availability question — an automatic retry can move the
  same private passage to a vendor the learner did not choose for it.
- Options considered: (A) no automatic failover; (B) failover to a verified provider with an equal consent
  scope, printed; (C) failover only for requests that carry no private text.
- Default taken: (B). Reverse by disabling the retry branch in the host; the ledger keeps every attempt.

### Q-0010-06 — What does the desktop learner see when every provider is unusable?
- Status: decided(default taken)
- Context: clause 5 requires one action per failure type, but a learner can be in a state where all configured
  providers are `no_credential` or over budget. The goal does not yet say whether that state is one aggregated
  message or a per-provider list.
- Options considered: (A) one line naming the cheapest recoverable action across all providers; (B) the full
  per-provider table; (C) the aggregated line plus the table behind `--verbose`.
- Default taken: (C) — the short line stays actionable, the table stays inspectable. Revisit when goal 0011
  renders this in the desktop workspace.
- Revisit when: the desktop workspace lands, or a learner reports the aggregate line as unclear.
- Revisited (2026-11-20): goal 0011 rendered this state in the desktop workspace and kept (C); see
  Q-0011-09, which now carries the decision.

### Q-0009-06 — Does `next` spend the assessment unit it names?
- Status: decided(default taken)
- Context: Q-0009-04 lists `next` among the presentation surfaces, but `next` prints only a
  recommendation command, not the question stem or passage. Treating it as non-spending would keep
  the pool larger; treating it as spending is the literal contract reading.
- Options considered: (A) spend the recommended content unit; (B) spend nothing until
  `practice`/`skill practice` renders the question.
- Default taken: (A). `runNext` appends one `next` presentation for the material the recommendation
  names. This is conservative and reversible by deleting that single `recordPresentation` call; no
  stored attempt or answer changes. `skill show` and `tactic show` remain non-spending lessons.
- Revisit when: the owner wants adaptive recommendation to stop shrinking the holdout pool.
- Moved (2026-11-18): the presentation `next` records is now `recommend.RecordPresentation`,
  called once inside `recommend.Recommend` in `internal/study/recommend/recommend.go`
  (refactor/study-seam); `internal/cli/next.go` asks for it with `recommend.SpendRecommended`, so the
  reversal is passing `recommend.SpendNone` there.

### Q-0009-07 — How is the per-item clock derived, and what happens to a late answer?
- Status: decided(default taken)
- Context: the contract requires a per-item timeout but supplies only `--size` and `--minutes`.
  A terminal command can block on input, so it cannot preempt a read portably.
- Options considered: (A) `floor(minutes × 60000 / actual_items)` ms, checked when input returns;
  (B) a fixed per-item default independent of the window; (C) no item clock, only the whole window.
- Default taken: (A). The value is stored on the run and each item, and a late answer is recorded as
  a timed-out `skipped` attempt with `elapsed_ms` equal to the item deadline minus its first
  presentation. The original presentation time survives a resume. Reverse by changing
  `assess.PerItemTimeout` and the loop's deadline checks; stored attempts keep their elapsed values.
- Revisit when: the real examination timing is known, or the desktop shell can enforce a live timer.

### Q-0009-08 — What exactly is a comparison "pool family"?
- Status: decided(default taken)
- Context: the contract refuses a comparison when the pool family differs but does not define the
  family. Item-level identity would refuse any two runs that selected different units, which is
  almost every pair; source text alone would ignore a newly imported corpus.
- Options considered: (A) SHA-256 of the sorted `source_id@source_sha256` lines of every imported
  source at run start; (B) the exact selected content ids; (C) only the source count.
- Default taken: (A). Importing or removing a source changes the family; a page correction inside a
  source does not, because the correction changes the item, not the corpus. Reverse by storing
  another digest in `assessment_runs.pool_family`; historical run rows keep the value they recorded.
- Revisit when: a source review changes enough items that two runs on the same corpus are no longer
  like-for-like, or a curated content family is introduced.

### Q-0009-09 — Is the glossary a new authored field?
- Status: decided(default taken)
- Context: the contract says a run renders the question without "glossary or reasoning", but the
  authored question format had reasoning, option reasoning and evidence and no glossary. The owner's
  Q-0009-05 notes that early learners may not recognise stem vocabulary.
- Options considered: (A) add an optional `glossary` array of `{term, meaning}` entries to the
  authored question file; (B) treat option reasoning as the glossary; (C) leave glossary undefined.
- Default taken: (A). It is optional, bounded (20 entries, 80/300 characters) and stored in
  `source_questions.glossary_json`; existing fixtures are unchanged, and a question without one
  prints `glossary: none`. The run withholds it and the report prints it. Reverse by ignoring the
  field; stored questions keep their JSON.
- Revisit when: imported source material supplies a real bilingual glossary that should be attached
  automatically rather than authored per question.

### Q-0009-10 — May a run exceed the requested `--size`?
- Status: decided(default taken)
- Context: a content unit is the holdout boundary. Selecting exactly N items could split a unit,
  spending it while leaving its siblings unpresented and unusable for any later run.
- Options considered: (A) take whole units until at least N items are selected, storing and printing
  the actual count; (B) stop before N to avoid overshoot; (C) split the final unit.
- Default taken: (A). The actual item count is stored on the run, printed in the report, and used by
  `assess compare`; the per-item clock uses the actual count. Reverse by changing `Pool.Select`.
- Revisit when: the owner wants an exact item count more than whole-unit holdout semantics.

### Q-0010-07 — Which capabilities does `model test` actually probe?
- Status: decided(default taken)
- Context: clause 4 requires a minimal call and a verified/failed record; clause 7 makes search,
  speech and images discoverable. The goal-0005 host validates an exact five-key chat body and has no
  tool/image path, so probing `tool.calls` or `vision` would widen the host protocol without an
  acceptance row.
- Options considered: (A) probe only `text.chat` and leave the optional capabilities
  `declared,unverified`; (B) add tool and image probes to the host; (C) declare no optional
  capabilities at all.
- Default taken: (A). `model test` verifies `text.chat`; every other declared capability stays
  discoverable and unverified, so no feature offers it. Reverse by adding a probe and one integration
  test per capability; existing verification rows keep their capability name.
- Revisit when: the desktop workspace actually executes an image or speech feature, or a provider's
  tool-call behavior must be verified for a feature that ships.

### Q-0010-08 — Does bounded failover apply to durable agent runs?
- Status: decided(default taken)
- Context: clause 8 says "when a call fails"; agent hosts make several provider calls inside one
  durable run, and goal 0006 records the model at start.
- Options considered: (A) fail over only in the single-call `tutor ask` path; (B) fail over inside the
  Pi host at every model call; (C) fail over only on the first agent call.
- Default taken: (A). Agent runs keep the recorded provider and model; `resume` is their retry
  mechanism. Reverse by adding an explicit model/provider hand-off event and a host-side retry; that
  needs a new recorded-model decision first.
- Revisit when: a durable run repeatedly fails on a transient provider error and the owner wants an
  automatic one-step hand-off.

### Q-0010-09 — Does a failover target need its credential present?
- Status: decided(default taken)
- Context: clause 8 lists verification, consent scope, budget and a different provider. A target
  whose credential variable is absent cannot complete the retry.
- Options considered: (A) require the target credential and refuse with `no_credential`; (B) try
  anyway and record the host's failure; (C) skip the condition because verification implies a past
  key.
- Default taken: (A). A verified model can outlive the key that verified it. The refusal names
  `no_credential` and makes no target call. Reverse by removing the presence check before
  `ReserveTutorCall`.
- Revisit when: the OS secret store makes credential presence a durable, provider-owned fact.

### Q-0010-10 — Is `speech.synthesis` a fifth registry capability?
- Status: decided(default taken)
- Context: clause 1 names four capabilities; clause 7 also requires speech to be discoverable.
- Options considered: (A) add `speech.synthesis` to the capability vocabulary; (B) treat speech as a
  provider-level feature outside the model matrix; (C) omit speech from this increment.
- Default taken: (A). It is always `declared` or `unsupported` for the shipped text models and has no
  execution path. Reverse by removing it from the data file and the vocabulary; verification rows
  keep the name.
- Revisit when: a speech feature ships and needs its own probe.

### Q-0010-11 — Does `model test` spend the provider budget?
- Status: decided(default taken)
- Context: clause 6 counts provider calls per day; a model test is a real provider call. Exempting it
  would let repeated tests exceed the printed ceiling.
- Options considered: (A) spend one slot per test after the host is invoked; (B) exempt probes; (C)
  count probes in a separate budget.
- Default taken: (A). A missing key, unknown model, invalid endpoint or exhausted budget refuses
  before a call and spends nothing. A verification event with `call_made = 1` consumes the slot; a
  crash between the reservation and that event leaves the reservation spent for the UTC day, so
  accounting can under-spend. Reverse by excluding `call_made` events from the counter.
- Revisit when: an owner wants a separate diagnostic budget for setup.

### Q-0010-12 — Is a hand-configured tutor endpoint verified like a registry model?
- Status: decided(default taken)
- Context: goal 0005 documented `tutor config --endpoint --model` as the only path. Requiring a
  registry id would break the 0005 surface and every 0005 test.
- Options considered: (A) keep the legacy path permissive and mark it `legacy,unverified` in
  `model list`; (B) require `model test` for every configured endpoint; (C) delete the legacy path.
- Default taken: (A). `model use` sets the provider field and is strict; a manual
  `tutor config --endpoint --model` clears the provider field and remains the documented exception.
  `model list` labels it `legacy`. Reverse by requiring verification whenever an endpoint is
  configured.
- Revisit when: goal 0011 can migrate every stored legacy configuration into the registry.

### Q-0010-13 — Does a legacy agent run bind its endpoint at start?
- Status: open
- Context: `agentProviderConfig` (`internal/cli/agent.go`) refreshes `cfg.Endpoint` from the current
  active registry provider, but `runAgentResume` skips the recorded-provider block when the run has no
  provider (a goal-0005 legacy run). After `model use beta/...`, resuming a legacy run therefore sends
  beta's endpoint with the run's legacy model id and `PASSQUEST_TUTOR_API_KEY`.
- Repro: configure a hand-edited endpoint, start an agent run that pauses, run
  `model use beta/<model>`, then `agent resume RUN`: the resume document keeps `run.model` (the legacy
  id), while the host request targets beta's endpoint.
- Options considered: (A) bind the endpoint at start for legacy runs and keep it on resume; (B) keep
  the current endpoint as Q-0006-05 deliberately says; (C) refuse resume when the endpoint no longer
  matches the run's recorded provider.
- Default taken: (B), unchanged in goal 0010 — this record names the behaviour so it is not mistaken
  for a verified provider/model pairing. Reverse by storing the endpoint on a legacy run or refusing
  the mismatch.
- Revisit when: the owner decides whether Q-0006-05's "endpoint is current" rule extends to a run that
  predates registry providers.
- Moved (2026-11-18): `agentProviderConfig` and the resume logic now live in
  `internal/study/brief/advance.go` (refactor/study-seam); `runAgentResume` in
  `internal/cli/agent.go` is the CLI adapter that calls it. The reversal and repro now touch
  `internal/study/brief`.

### Q-0014-01 — Can Pass Quest tell that AnkiConnect ignored the requested deck?
- Status: open
- Context: the owner's real-machine verification (2026-11-13, Anki Desktop 26.9.4 with AnkiConnect built
  from source) found `note push` printing `Deck: Pass Quest` and `pushed/ok` while the card was in Anki's
  default deck; the `Pass Quest` deck existed and was empty. A direct AnkiConnect `addNote` with
  `deckName: "Pass Quest"` landed in the default deck too, and the add-on sets the requested deck on the
  note type (`ankiNote.model()['did']`) before `collection.addNote(ankiNote)` without a deck id — so the
  payload is correct and the add-on/Anki pair is what drops it. Pass Quest's read-back compares the model
  and the fields, never the deck, so neither the CLI nor the stub-based `note smoke` gate can see it.
- Repro: point `note push` at a real Anki Desktop on Anki 26.x with the current AnkiConnect, push one
  note, then ask the same endpoint `getDecks`/`cardsInfo` for the card's deck.
- Options considered: (A) read the landed deck back and report a typed mismatch, keeping the note id;
  (B) additionally move the card with `changeDeck` so the claim becomes true; (C) document the add-on
  version sensitivity and change nothing.
- Default taken: (A), as active goal 0014: reporting what happened is a smaller, more honest change than
  writing to the learner's collection, and (C) leaves a printed false claim in place. Reverse by adopting
  (B) once a compatible add-on version is known.
- Revisit when: a released AnkiConnect is confirmed to honour `deckName` on the supported Anki version, or
  the owner wants the card moved automatically.

### Q-0014-02 — What happens when the endpoint will not report a card's deck?
- Status: decided(default taken)
- Context: goal 0014 needs the deck a card reached. `notesInfo` gives a note's cards and `cardsInfo`
  is the only documented read that names a card's deck, but an endpoint can omit either (an older
  build, or a stub that answers something else). Two ways to treat that: trust the requested deck, or
  refuse to claim it.
- Options considered: (A) treat an unread deck as `deck_mismatch` with a detail naming what was not
  read; (B) treat it as success because the write was acknowledged; (C) add a third state such as
  `deck_unknown`.
- Default taken: (A). Refusing the claim keeps the contract's promise ("either the card is in the
  deck it names or the push says it is not"), it needs no extra state in the vocabulary, and the exit
  code and the learner's action are the same as for a card found elsewhere. Reverse by mapping the
  unread case to (B) or (C) once a supported AnkiConnect version is confirmed to always answer both
  reads (the documented v6 API does; see `docs/notes.md`).
- Revisit when: a released AnkiConnect on a supported Anki version is confirmed to omit `cards` or
  `deckName`, or an endpoint in the field answers `cardsInfo` without a deck.

### Q-0014-03 — Does a later push re-read the deck of a note that is already drained?
- Status: decided(default taken)
- Context: after a mismatch the learner moves the card by hand. Nothing is pending, so the next push
  is `idle` and contacts nothing, which keeps the idle path byte-identical and never adds a card. The
  recorded deck therefore stays the deck reported at the last write until a new revision pushes.
- Options considered: (A) leave it — the recorded fact is what Anki said at the last write; (B) an
  idle push re-reads the decks of notes whose recorded state is `deck_mismatch` (or whose deck is
  empty) and refreshes them, exiting 3 only while a card is still elsewhere; (C) add a
  `passquest note verify` command.
- Default taken: (A). (B) and (C) are useful but they are new surfaces, not the contract's clause: it
  asks that a repeat push contact no write and add no second card, which holds, and the fix for the
  learner is one manual move plus the next revision push. Reverse by adopting (B) or (C) if the
  learner reports the stale recorded deck as misleading.
- Revisit when: an owner's real-machine run shows the stale `deck` value causing a wrong decision, or
  goal 0011 gives the workspace a place to show a live deck.

### Q-0014-04 — Is the deck check charged to the same push or a new write action?
- Status: decided(default taken)
- Context: the verdict could be folded into the write's ledger row (`add/deck_mismatch`) or recorded
  as its own read row. Folding it would also have to interleave the read between a write batch and its
  persistence, which the harness pins as "each acknowledged batch is written before the next call".
- Options considered: (A) a separate `check/ok` or `check/deck_mismatch` row after the writes are
  recorded; (B) one `add/deck_mismatch` row; (C) no ledger row, only the note's state.
- Default taken: (A). The write's own outcome stays visible, the read is evidence in the append-only
  ledger, and the queue keeps the drain the write decided. Reverse by folding the verdict into the
  write row if a later review shows the extra row per write is noise.
- Revisit when: a learner's ledger grows large enough that the extra row per push costs anything
  measurable, or a view wants a per-push summary instead of per-note history.

### Q-0011-00 — Unattended merge authorization for goals 0011–0013
- Status: resolved
- Resolution: (owner, 2026-11-14) the owner asked for goals 0011–0013 to be completed in the "C · Stage"
  design language and chose "one branch and PR per goal, merged by the agent". Merge conditions are those of
  Q-0000-01: CI green on the head, the codex marker `has_any_findings: false` on that same head, zero
  unresolved threads, and a local gate re-run on the merged tree.
- Context: Q-0000-01 authorised unattended merges for goals 0001–0010 only.
- Options considered: (A) merge each goal when the merge gate is green; (B) open PRs only; (C) local commits.
- Default taken: none — the owner chose (A).
- Revisit when: a merge gate produces a false green, or the owner revokes it.

### Q-0011-01 — Which desktop platform is verified in goal 0011?
- Status: decided(default taken)
- Context: the contract asks to refine the primary desktop OS. The development machine is Windows 11 with
  the WebView2 runtime; CI runs ubuntu, macos and windows.
- Options considered: (A) Windows verified locally, macOS/Linux launched by the CI smoke; (B) Windows only;
  (C) all three verified by hand.
- Default taken: (A). The Linux CI job installs WebKitGTK and runs the smoke under a virtual display; a
  platform whose smoke cannot launch fails the gate rather than skipping it.
- Revisit when: the owner studies on macOS or Linux, or a CI image drops a webview prerequisite.

### Q-0011-02 — Which frontend toolchain does the desktop window use?
- Status: decided(default taken)
- Context: core design names "Web frontend + TypeScript"; the stack forbids an embedded browser engine and
  asks for the standard library first. MyGo's template uses Vite.
- Options considered: (A) plain TypeScript bundled by Bun, no framework; (B) Vite + a UI framework;
  (C) hand-written JavaScript with no build.
- Default taken: (A). Bun is already the pinned harness runtime, the prototype is framework-free, and one
  bundler call keeps the build reproducible on all three OSes. The bundle is generated into a gitignored
  directory and embedded by `go:embed`; a binary built without it shows a "frontend not built" page instead of
  a blank window.
- Revisit when: the frontend outgrows hand-rolled rendering, or Bun's bundler output stops being stable.

### Q-0011-03 — Where does the desktop app keep provider credentials?
- Status: decided(default taken)
- Context: core design §7 requires the OS secret store; Q-0005-01 kept environment variables only until goal
  0011 provides one. A process started from a desktop launcher has no shell environment.
- Options considered: (A) environment variable first, then the OS credential store; (B) OS store only;
  (C) environment only.
- Default taken: (A). The variable still wins so the gates and a developer shell behave as before; the window
  writes keys only to the OS credential store (Credential Manager, Keychain, Secret Service) and reads back
  only presence. Nothing is written to SQLite or files. Where no store exists, the window says so and the
  variable remains the only path.
- Revisit when: the owner wants one source of truth, or a platform's store prompts too often.

### Q-0011-04 — Where does the desktop app keep its database?
- Status: decided(default taken)
- Context: the CLI defaults to `.passquest/passquest.sqlite` relative to the working directory, which is
  arbitrary when a window is launched from a menu.
- Options considered: (A) the app's per-user data directory, `PASSQUEST_DB` overriding; (B) the CLI default;
  (C) ask on first run.
- Default taken: (A). The CLI default is unchanged. Onboarding and moving an existing CLI database are goal
  0013 work.
- Revisit when: goal 0013 defines first-run data location.
- Revisited (2026-11-19): goal 0013 kept the per-user directory (`%APPDATA%\Pass Quest`,
  `~/Library/Application Support/Pass Quest`, `$XDG_CONFIG_HOME/Pass Quest`) with the
  `PASSQUEST_DESKTOP_DATA` override, and a default directory that lands inside the app's own files is
  now refused before anything is written; moving an existing CLI database stays out of scope (the
  plan's out-of-scope list). See Q-0013-04.
- Revisited (2026-11-25): goal 0020 moved the command line's default to that same per-user directory
  through one shared resolution (`internal/userdata`, Q-0020-01), so the window and the command line
  read one database by default. `--db` and `PASSQUEST_DB` still decide first; the window's own
  location, `PASSQUEST_DESKTOP_DATA`'s meaning for it, and its refusal rule are unchanged. Moving an
  existing `.passquest` database stays out of scope — the command line only tells the learner
  (Q-0020-02).

### Q-0011-05 — What is a "hint" in ordinary practice?
- Status: decided(default taken)
- Context: the contract and the design brief show a hint gated by an "assisted" warning, but the only
  authored assistance in the domain is the tactic guided walkthrough; ordinary items carry none.
- Options considered: (A) offer a hint only where an authored walkthrough exists and record it as an
  exposure; (B) generate a hint with the tutor; (C) show the evidence sentence as a hint.
- Default taken: (A). No AI text and no answer-bearing evidence becomes a hint. Where no walkthrough exists the
  hint control says so instead of disappearing.
- Revisit when: authored hints exist for imported items.

### Q-0011-06 — Are exam date and daily study budget part of the domain?
- Status: decided(default taken)
- Context: the design brief shows "exam date: not set" and "daily budget: not set" with a set action; no
  such fact exists in the store, and the goal series forbids inventing an exam date.
- Options considered: (A) optional learner settings, display-only; (B) omit them; (C) let them drive policy.
- Default taken: (A). They are stored only when the learner enters them, shown as entered, and are not
  inputs to selection, scoring, assessment or any readiness statement. Prep day is derived from the first
  recorded attempt.
- Revisit when: the owner wants the plan to use the date, which would need its own contract.

### Q-0011-07 — Can the learner keep Anki's version of a conflicting note?
- Status: decided(default taken)
- Context: the design brief shows "keep yours / keep Anki's"; the outbox records a conflict but not the
  Anki-side field values, so Anki's version cannot be shown or adopted without new reads from AnkiConnect.
- Options considered: (A) offer "keep mine" (the existing `--force`) and "leave as is"; (B) fetch Anki's
  fields and offer both; (C) hide conflicts.
- Default taken: (A). The conflict stays visible and is never overwritten silently.
- Revisit when: goal 0012 or a later Anki goal adds a read of Anki-side fields.

### Q-0011-08 — When does the window record a presentation?
- Status: decided(default taken)
- Context: the CLI's two-step JSON practice records a presentation only at answer time, so an item viewed and
  abandoned does not spend an assessment unit; `show` and `questions` spend at display.
- Options considered: (A) record at display; (B) record at answer; (C) never.
- Default taken: (A). The window shows the passage, so it follows the display rule of `show` (Q-0009-04).
  The presentation time is also the start of the attempt's elapsed time and the resume point after an
  interruption.
- Revisit when: the owner finds the pool spending too fast in practice.

### Q-0011-09 — What does the window show when every provider is unusable?
- Status: decided(default taken)
- Context: Q-0010-06 left the aggregated-line choice to be revisited when the desktop workspace lands.
- Options considered: as Q-0010-06.
- Default taken: (C) of Q-0010-06 — the coach panel shows the one aggregated line with its action, and
  Settings shows the per-provider table.
- Revisit when: a learner reports the aggregated line as unclear.

### Q-0011-10 — What identity does the desktop app use, and is an overridden data directory a separate instance?
- Status: decided(default taken)
- Context: MyGo keys the single-instance lock, the WebView profile and the credential-store service on the
  app's name and identifier. The smoke gate launches the app on temporary data directories while a learner's
  own window may be open.
- Options considered: (A) one fixed identity; (B) fixed identity, and an overridden data directory
  (`PASSQUEST_DESKTOP_DATA`) gets a name suffixed with a hash of that directory; (C) a per-launch identity.
- Default taken: (B). Name "Pass Quest", identifier `com.github.wxxb789.passquest` (also the default
  credential service). A second launch on the same data directory focuses the first window; a launch on a
  different overridden directory is an independent instance, so a smoke run can never hand its commands to the
  learner's window.
- Revisit when: goal 0013 signs and packages the app, which may fix the identifier for good.
- Revisited (2026-11-19): the MyGo package build keeps the name "Pass Quest" and the identifier
  `com.github.wxxb789.passquest` (`mygo.json`, pinned by `TestPackagingIdentity`); the packages ship
  unsigned, so the identifier is unchanged, and an overridden `PASSQUEST_DESKTOP_DATA` still gets the
  hash-suffixed instance name.

### Q-0011-11 — How long does quit wait for in-flight work?
- Status: decided(default taken)
- Context: a coach call, a model test, an Anki push or a brief may be running when the window closes; clause 10
  forbids a surviving Node host.
- Options considered: (A) quit immediately and kill children; (B) refuse new calls, wait a bounded time, then
  end every host; (C) wait indefinitely.
- Default taken: (B) — up to 5 s for in-flight bound calls, then every spawned host is ended: a job object on
  Windows, SIGTERM then SIGKILL to the host (and the durable host's process group) on macOS and Linux. The
  durable host pauses on SIGTERM, so a brief interrupted by quit resumes later.
- Revisit when: a learner loses a reply by closing the window too early.

### Q-0011-12 — Which consent unlocks imported text in an Anki note?
- Status: decided(default taken)
- Context: notes accept only a global (all-provider) consent event for an imported source (Q-0007-08); the coach
  panel records per-provider consent. A one-click grant from the Anki preview would silently widen scope.
- Options considered: (A) a per-destination consent for notes; (B) keep the global rule and make the window name
  the scope and ask for confirmation; (C) remove the action from the Anki screen.
- Default taken: (B). The dialog lists every provider the grant covers, says future providers are included, and
  defaults focus to Cancel; per-provider grants stay in the coach panel and Settings.
- Revisit when: a later goal introduces consent scoped to notes.

### Q-0011-13 — Does `passquest next` spend an assessment unit during an active run?
- Status: open
- Context: the window's Today spends no `next` presentation while a run is active, because Today renders on
  every navigation and its recommendation can name a pending run item, which would then be reported as
  contaminated. The CLI's `next` still records its presentation during a run (pre-existing behaviour, unchanged
  here because CLI output is protocol), though a learner has to type it.
- Options considered: (A) leave the CLI as is; (B) make `next` spend nothing during a run; (C) refuse `next`
  during a run as `tutor`/`skill`/`find` are refused.
- Default taken: (A) for this goal; no CLI behaviour changed.
- Revisit when: the owner decides; (C) matches the existing assessment guard best.

### Q-0011-14 — Console or GUI subsystem for the Windows desktop binary?
- Status: open
- Context: a `go build` binary is a console program, so launching it from Explorer also opens a console window;
  linking with `-H windowsgui` hides it but drops the stdout of the CLI branch (`passquest-desktop report …`)
  that the durable host relies on.
- Options considered: (A) console binary in 0011; (B) GUI binary plus a separate console CLI shipped beside it;
  (C) GUI binary that attaches to a parent console when one exists.
- Default taken: (A) for goal 0011, which ships no installer.
- Revisit when: goal 0013 packages the app.
- Revisited (2026-11-19): the packaged Windows binary is GUI-subsystem (`-H=windowsgui`) and keeps the
  CLI branch in the same executable — `agent record-step` through it returns JSON over an inherited
  pipe (verified on Windows, P4) and the upgrade case runs `note accept`/`note push` through the
  installed binary — so no second console binary ships and no console is attached.

### Q-0011-15 — Does the CLI lose an on-screen assessment item when `assess report` runs first after the window elapsed?
- Status: open
- Context: the window now times out the presented item of an elapsed run before abandoning it, on every entry
  point including Today (codex review on PR #24). The CLI's `assess` subcommands reconcile through the store
  only, so `assess report RUN` run before `assess resume RUN` after the window elapsed may abandon the run with
  the presented item still unfinalized. Not verified; CLI output is protocol and was not changed in goal 0011.
- Options considered: (A) route the CLI through the same workspace reconciler; (B) leave as is.
- Default taken: (B) for this goal.
- Revisit when: a follow-up fix confirms or refutes the CLI behaviour with a test.

### Q-0011-16 — May a goal merge when the local gate fails only for a missing npm package?
- Status: resolved
- Resolution: (owner, 2026-11-15) yes. The development machine can reach only a corporate npm feed proxy that
  publishes packages about seven days after the public registry, so the locked Pi Durable packages
  (`@earendil-works/*@1.5.1`, 24 packages in all) are not yet installable there and `agent smoke` and
  `provider smoke` fail locally with lines byte-identical to unmodified `main`. A goal may merge when every
  other gate passes locally on the merged tree and CI passes all gates on ubuntu, macOS and Windows at the
  same head, with the exception named in the merge commit. PR #24 (goal 0011) merged under this rule.
- Context: the merge rule of Q-0000-01 / Q-0011-00 requires a local gate re-run; the registry is not
  switchable on this network and the lock file must not be downgraded to make a local run pass.
- Options considered: (A) merge on local-minus-environment plus CI; (B) wait for the feed; (C) obtain the
  packages another way.
- Default taken: none — the owner chose (A).
- Revisit when: the feed carries the locked versions; then the local gate must be fully green again.
- Revisited (2026-11-18): the dependency upgrade moved the Pi packages to `@earendil-works/*@1.5.2`, which
  the feed carries; `agent smoke` (118 assertions) and `provider smoke` (92) pass locally again, so the
  local gate is expected to be fully green and this exception no longer applies to new merges.

### Q-0012-00 — Which design direction does goal 0012 refine?
- Status: resolved
- Resolution: (owner, 2026-11-14) "C · Stage" (`design/prototypes/c-stage.html`), chosen over "A · Folio" and
  "B · Instrument" when the owner asked for the remaining goals to be completed in that design language.
- Context: goal 0012 asked for the visual language to be refined with Opus and learner feedback rather than
  frozen early; three self-contained prototypes were produced for the choice.
- Options considered: (A) Folio; (B) Instrument; (C) Stage.
- Default taken: none — the owner chose (C).
- Revisit when: representative learner feedback contradicts the direction.

### Q-0012-01 — How is visual and interaction quality gated?
- Status: decided(default taken)
- Context: the goal requires review "based on actual user flows and rendered screens, not aesthetic assertions
  alone"; screenshots alone are not evidence under the harness rules.
- Options considered: (A) a `desktop-ux` gate auditing the real window per route × theme × language × size,
  with an allowlist that may only shrink; (B) screenshots reviewed by hand; (C) extend `desktop-smoke`.
- Default taken: (A). Overflow, coverage, primary-action visibility, composited contrast, font-size floor,
  focus and motion are measured in the page; every run first proves its probes on planted defects. At close the
  allowlist holds only entries whose reason is exam content or a learner-entered value.
- Revisit when: a check proves flaky across CI platforms, or the owner wants pixel-level screenshot diffs.

### Q-0012-02 — How does an existing deck get the new card design?
- Status: decided(default taken)
- Context: the note model is created once and never updated (goal 0007); a new model name would turn every
  pushed note into `model_mismatch` and need a schema change in Anki.
- Options considered: (A) update styling and templates in place under the same model name, only when the deck
  holds parts Pass Quest shipped; (B) a new model name and a note-type migration; (C) leave old decks.
- Default taken: (A). The stylesheet, the front and the back are each compared with every version Pass Quest
  shipped, after normalising line endings and surrounding whitespace (a client may store them differently); a
  deck whose parts all match some shipped version — including one left half-updated by an interrupted update —
  is upgraded, because overwriting it can only replace Pass Quest's own text. Any other difference is the
  learner's customisation: it is reported as `model_customized` and never overwritten. Fields, field order, model
  name and template name do not change, so conflict detection and the deck claim of goal 0014 are unaffected.
- Revisit when: real Anki evidence shows a template update forces a full sync.

### Q-0012-03 — Is there a free-text coach composer?
- Status: decided(default taken)
- Context: the prototype shows a reply box under the coach; the tutor domain accepts only a mode
  (`explain`/`probe`/`micro_lesson`) per attempt.
- Options considered: (A) no composer; (B) a composer mapped to a new tutor request kind.
- Default taken: (A). A new request kind needs its own prompt, budget and consent contract.
- Revisit when: a goal defines follow-up questions as a tutor capability.

### Q-0012-04 — Does the card front show the tactic name?
- Status: decided(default taken)
- Context: the prototype card shows the tactic as an eyebrow on the front; on a recall card that names the
  category of the answer before the learner tries.
- Options considered: (A) tactic on the back only; (B) on the front.
- Default taken: (A).
- Revisit when: the owner prefers the cue on the front.

### Q-0012-05 — Which window sizes are measured?
- Status: decided(default taken)
- Context: the brief targets 1440×900 and 1280×800; the development display (1536×864 DIP) caps a 1440-wide
  content area at 844 px high, and the window size is otherwise restored from MyGo's window state.
- Options considered: (A) 1280×800 and 1440×844 with a smoke-only content-size switch; (B) 1280×800 only.
- Default taken: (A). The audit asserts the real viewport before measuring and fails rather than measuring a
  different size.
- Revisit when: CI or the owner's display can host 1440×900.

### Q-0012-06 — May a test shorten the agent kill grace through a product seam?
- Status: resolved
- Resolution: (owner, 2026-11-17) (A) approved as test-efficiency plan item A8.
- Context: `TestAgentCancelKeepsTerminalOffALiveHost` is the only coverage of the unconfirmed cancel path
  (agent-smoke covers only the confirmed one) and waited the full hard-coded 6 s kill grace on every run.
- Options considered: (A) one grace, `agentKillGrace()` = `agentHeartbeatWindow()` + 1 s (6 s in production),
  carried by an unexported `Options.agentKillGrace` field that `withDefaults()` fills, so only a test in
  `internal/cli` can shorten it; (B) a package variable; (C) keep the 6 s wait.
- Default taken: none — the owner chose (A). The test sets 500 ms and keeps its five assertions; it also asserts
  that `(Options{}).withDefaults()` yields 6 s, so the production value cannot drift unseen.
- Revisit when: the cancel loop's grace is derived from anything other than the heartbeat window.

### Q-0012-07 — May assess-smoke drop sections whose assertions a Go test already makes?
- Status: resolved
- Resolution: (owner, 2026-11-17) (A) approved as test-efficiency plan item A10.
- Context: assess-smoke sections 9 (pool floor) and 6 (compare) repeated assertions that
  `internal/cli/assess_test.go` makes more strictly; section 6 seeded 40 pages (about 127 process spawns) for
  one read-only compare.
- Options considered: (A) drop the duplicated sections after the Go test gains the one string it lacked;
  (B) keep both.
- Default taken: none — the owner chose (A). `TestAssessStartRefusesFloorAndShortfall` first gained the
  "4 unseen content unit(s)" assertion, shown to fail when the message changes; it asserts exit 2, `pool_floor`,
  "4 qualifying item(s)", "16 more" and that text. `TestAssessCompareRefusesUnlikeRuns` asserts
  `incomparable_runs`, `window_minutes` and `compared` for two distinct like runs (the smoke compared a run with
  itself; the compare has no same-id branch). Assess smoke went from 275 to 125 assertions. Section 1 (pool in
  items and units) stays, against the plan: the `qualifying_items` and `question_count` wire keys are checked
  nowhere else, because the Go test decodes them through the production structs. Section 1b stays as the plan
  required. The dated `docs/evidence/0009-unseen-exam-assessment.md` record ("275 assertions over the nine
  acceptance rows") is left as written; acceptance rows 6 and 9 are now carried by the two Go tests.
- Revisit when: a Go test asserts the raw `"qualifying_items"` and `"question_count"` keys; section 1 may then go.

### Q-0012-08 — How is the CI gate split into jobs?
- Status: decided(default taken)
- Context: one job per OS ran all 24 gates serially, so a failure cost the whole chain's wall time and a
  re-run repeated every gate.
- Options considered: (A) one job per OS × group, grouped by environment need; (B) one job per gate; (C) keep
  one job per OS.
- Default taken: (A): `core` (gates 1-21; Go, Bun, Node), `desktop` (desktop-smoke, desktop-ux; display,
  webview, Secret Service) and `self-test` (it feeds the desktop gates broken trees, so it needs the same) on
  ubuntu, macOS and Windows, fail-fast off, 20-minute timeout. Self-test is its own group so the desktop job is
  not the bottleneck. The split was sized from earlier CI logs, not measured on the split pipeline. Every
  `bun run gate` refuses to run unless `harness/lib/gate-groups.ts` finds every gate in exactly one group and
  the matrix exactly `os` × `group` for those groups. The three jobs of one OS share setup-go's cache key, so
  only the first to finish saves the build cache; this is disclosed, to be measured on the first split run.
  The owner may rebalance the groups.
- Revisit when: the first split CI run shows the desktop or self-test builds slower than before (then key the
  cache per group), or the per-gate timings show one group as the bottleneck.
- Updated (2026-11-18): goal 0013's package gate added a fourth group, `package` (the package smoke, gate 24).
  Its job builds, installs and drives a real package on its own OS, so the matrix gives it a 60-minute timeout
  (`timeout-minutes: ${{ matrix.group == 'package' && 60 || 20 }}`) while every other group keeps its 20, and it
  uploads the package this run built as the 7-day `package-<os>` artifact. The four jobs of one OS still share
  setup-go's cache key, with the same disclosure as above.

### Q-0012-09 — Should the gate run independent gates in parallel in one process?
- Status: decided(default taken)
- Context: test-efficiency plan item A13 proposed overlapping go-test with the static and CLI smoke gates
  before the desktop gates, conditional on 20 CI trials per OS.
- Options considered: (A) defer; (B) a two-phase runner with `--serial` kept.
- Default taken: (A). The CI split (Q-0012-08) already runs go-test and the desktop gates on separate runners;
  what remains is overlap inside `core` on one 4-vCPU runner, where heartbeat and timing tests would contend
  with the smokes — the flake source the plan named.
- Revisit when: the per-gate timings on the split pipeline show `core` as the slowest group.

### Q-0012-10 — Do the parallel store tests flake on SQLite locking?
- Status: open
- Context: plan item A9 runs `internal/cli` and `internal/store` top-level tests with `t.Parallel()`. One local
  run failed `TestMigrationFromVersionSixAddsTutorTables` with `SQLITE_BUSY` after the 5 s busy timeout (1 of
  22 store-package runs, root cause unconfirmed).
- Options considered: (A) match production: the test helper `openSQLite` sets `SetMaxOpenConns(1)` as
  `store.open` does; (B) make the `openSQLite` migration-fixture tests serial; (C) drop parallelism in the
  store package.
- Default taken: (A). It removes one fixture opening a second connection against its own file; it does not
  prove that was the cause.
- Evidence since (A): `go test ./internal/store -count=20` passed locally (2026-11-17), and the go-test gate passed
  in the first split CI run on ubuntu, macOS and Windows.
- Revisit when: go-test flakes on Windows CI; then take (B).

### Q-0013-01 — Which Windows installer technology does the package use?
- Status: decided(default taken)
- Context: the owner allowed MSI or NSIS (2026-11-18); the choice had to come from the pinned
  toolchain, install per user without admin rights, and offer CI a silent install to drive.
- Options considered: (A) the NSIS installer MyGo builds; (B) an MSI through WiX; (C) Inno Setup.
- Default taken: (A). `go tool mygo build` writes the NSIS installer from `mygo.json` and, on Windows,
  downloads NSIS 3.13 with its SHA-256 checked; it installs per user to
  `%LOCALAPPDATA%\Programs\Pass Quest` (`RequestExecutionLevel user`), registers `Uninstall.exe` in
  Settings > Apps, creates the Start menu shortcut and offers the desktop one, and takes `/S` and
  `/D=` for the package smoke. An MSI or Inno installer would re-implement what MyGo already ships
  (shortcuts, the uninstall key, signing and update hooks) and add a tool to every runner. Reverse by
  building a WiX or Inno installer from MyGo's output; the app, its resources and its data directory
  do not change.
- Revisit when: the owner needs an enterprise deployment (MSI/GPO), or MyGo drops or breaks its NSIS
  output.

### Q-0013-02 — Does uninstalling remove the learner's data?
- Status: decided(default taken)
- Context: the uninstaller could delete anything under the install directory, while Done-when 3
  requires the learner's evidence to survive an uninstall and the guide to say where the data is.
- Options considered: (A) uninstall keeps the data, and the guide says where it is; (B) the uninstaller
  prompts before deleting it; (C) the uninstaller deletes it.
- Default taken: (A). MyGo's Uninstall section removes `$INSTDIR`, its shortcuts and the HKCU uninstall
  key only (`cmd/mygo/nsis.go`), so `%APPDATA%\Pass Quest` is untouched, and
  [`install.md`](../install.md) documents the data directory and its removal. The package smoke's
  uninstall is a cleanup sanity check, not proof — an uninstaller deleting a data directory would
  resolve `$APPDATA` through the OS shell folders and escape the run's temp root — so the guarantee is
  the study case plus that uninstall section, as disclosed. Reverse by adding an uninstall hook if the
  owner wants a delete prompt.
- Revisit when: the owner wants a delete-on-uninstall choice, or MyGo's uninstall section changes.

### Q-0013-03 — Which Node runtime does a package bundle, and how is it pinned?
- Status: decided(default taken)
- Context: the owner chose a bundled Node runtime (2026-11-18) so coach, brief, search and Anki work
  right after install with no Node present; the build must not resolve a version at build time or
  trust an unfixed download.
- Options considered: (A) bundle the official archive per platform with its SHA-256 from
  `SHASUMS256.txt`; (B) require a system Node; (C) bundle a self-built Node.
- Default taken: (A). Node 22.23.5 — the newest 22 LTS that `mise.toml` and the hosts' `engines`
  accept — for `win32-x64`, `darwin-arm64`, `darwin-x64` and `linux-x64` (`NODE_PINS` in
  `harness/lib/package.ts`); each SHA-256 is nodejs.org's `SHASUMS256.txt` value and is checked again
  on download, and a target with no pin fails instead of falling back. `PASSQUEST_NODE` still
  overrides the bundled runtime. Reverse by changing `NODE_PINS` (or dropping `node/` from
  `mygo.json`'s resources) and re-running the package gate.
- Revisited (2026-11-25, goal 0020 P4): the pin is now **Node 26.13.1**, the newest 26 LTS when the
  step landed (`https://nodejs.org/dist/index.json` lists v26.13.1 as 2026-11-10, `"lts":
  "Lithium"`, `"security": true`), with its `SHASUMS256.txt` SHA-256s:
  `e2edda75bfb089654705cf65a13b58933bfd5f57148e8ebda916f58a2474f1f7` (win-x64.zip),
  `aec2514ea1ceb2ba52455e9ec65986f6749a06dd759e5db2a72e6889668c9735` (darwin-arm64.tar.gz),
  `82bb96078a24ca50b9c9a38f2379e6be6d87171fcd400fc14c8934b3bce96e2f` (darwin-x64.tar.gz),
  `5c063e46bab0fbc1c17cc445d7ff129cbdcd0471be6ed83b6ab864f8143bc864` (linux-x64.tar.xz); each was
  recomputed from the downloaded archive before the pin was written, and the archive, member and
  staged-path names follow the same shape as the 22 pins. `mise.toml`, `gate.yml` and the two
  guides' pinned-runtime sentence carry the same patch, and the package gate's start check reads all
  of them (goal 0020 P4). The four targets, the reversal and `PASSQUEST_NODE`'s precedence are
  unchanged; the move is the owner's 2026-11-22 decision in goal 0020's contract, not a fired
  "Revisit when" clause — Node 22 is still in maintenance until 2027-04-30.
- Revisit when: Node 22 leaves support, another target's pin is added, or the owner decides a system
  Node is required.

### Q-0013-04 — Where does the packaged app keep its data?
- Status: decided(default taken)
- Context: Q-0011-04 left the first-run data location to this goal; the data must be per user, survive
  a reinstall and an uninstall (Done-when 3), and never sit inside the install, which an upgrade
  replaces and an uninstall removes.
- Options considered: (A) `PathUserData` (`os.UserConfigDir()` joined with the app name) with
  `PASSQUEST_DESKTOP_DATA` overriding; (B) a directory beside the executable; (C) `~/.local/share` on
  Linux for XDG familiarity.
- Default taken: (A). `%APPDATA%\Pass Quest` (Roaming) on Windows,
  `~/Library/Application Support/Pass Quest` on macOS, `$XDG_CONFIG_HOME/Pass Quest` (default
  `~/.config/Pass Quest`) on Linux; the database is `passquest.sqlite` inside it, with `PASSQUEST_DB`
  overriding the file. A default directory that lands inside the app's own files (the executable's
  directory or the resources directory) is refused before anything is created, naming
  `PASSQUEST_DESKTOP_DATA`; a directory that is the user's home or one of its ancestors is exempt,
  because a loose binary there is not an install. Reverse by changing the resolver in
  `internal/desktop/paths.go` and the harness's `dataRootFor`.
- Revisit when: the owner prefers `~/.local/share` on Linux, or a learner meets a path problem in real
  use.

### Q-0013-05 — Which tools build the Linux AppImage, and how are they pinned?
- Status: decided(default taken)
- Context: MyGo builds no AppImage (the owner chose an extra tool, 2026-11-18); a build-time download
  must be pinned by version and SHA-256, and appimagetool otherwise downloads the AppImage runtime
  from the type2-runtime project's rolling "continuous" release at build time.
- Options considered: (A) appimagetool 1.9.1 with the type2 runtime pinned to its versioned 20251108
  release asset; (B) the rolling continuous appimagetool; (C) a self-built runtime.
- Default taken: (A). `harness/lib/appimage.ts` pins appimagetool 1.9.1 (`appimagetool-x86_64.AppImage`)
  and type2-runtime 20251108 (`runtime-x86_64`), each SHA-256 checked, and passes the runtime with
  `--runtime-file`, so nothing is fetched unpinned. The step is pinned to linux-x64 and refuses
  another host or target rather than reporting a silent success; no FUSE is assumed — the tool runs
  from its own extraction (`APPIMAGE_EXTRACT_AND_RUN=1`), the built image is verified by unpacking
  it with `--appimage-extract` and checking the tree, and the CI package job launches it with
  `APPIMAGE_EXTRACT_AND_RUN=1`. Reverse by bumping the two constants.
- Revisit when: appimagetool publishes a newer versioned release, or a second Linux architecture is
  claimed.

### Q-0013-06 — What is the "previous build" for the forward-upgrade check before any release exists?
- Status: decided(default taken)
- Context: Done-when 5 opens a database written by the build just before this goal; no release is
  tagged, and a check that built this tree twice would prove nothing.
- Options considered: (A) one recorded commit — the last `main` commit before this goal's work;
  (B) the merge base with `main` resolved at run time; (C) skip the check until a release exists.
- Default taken: (A). `PREVIOUS_RELEASE_COMMIT` in `harness/gates/package-smoke/upgrade.ts` is
  `357b4c6`, the merge of PR #28 (the MyGo 0.10.0 / Pi 1.5.2 upgrade) and the last `main` commit
  before goal 0013; the gate builds that commit's CLI in a `git worktree`, fetching the commit by full
  id when the depth-2 checkout does not hold it, and has it write the learner data. The base and this
  build both persist schema version 17, so the case proves a same-version reopen by the packaged build
  — no migration statement runs — and the two `user_version` values are read from the files at run
  time and printed in the pass line, a disclosure repeated in the evidence record. When a release is
  tagged, the constant becomes that tag's commit. Reverse by pointing the constant at another base
  commit.
- Revisit when: the first release is tagged, or the store's schema changes and a migration must be
  proven.
- Revisited (2026-11-20): v0.15.0 was published at `c9a83a0`, so `PREVIOUS_RELEASE_COMMIT` now names
  that commit, and `docs/release.md` moves it to each new tag's commit after publishing.

### Q-0013-07 — What happens to the package CI artifacts, and when is one a release?
- Status: decided(default taken)
- Context: the goal keeps each run's package for evidence, and publishing a GitHub Release or any
  other public download needs the owner's explicit go-ahead at the time.
- Options considered: (A) the package job uploads the run's package as a 7-day workflow artifact,
  never a release; (B) no artifact, rebuilt on demand; (C) a GitHub release per green run.
- Default taken: (A). The package job uploads `package-windows-latest`, `package-macos-latest` or
  `package-ubuntu-latest` (`retention-days: 7`, and a missing package fails the upload rather than
  passing silently); a failed job keeps its full gate output as `gate-output-<os>-<group>` for 14
  days. Reverse by changing the retention or removing the upload step; (C) needs the owner's
  go-ahead.
- Revisit when: the owner authorizes a release, or an artifact must outlive 7 days for an audit.
- Revisited (2026-11-20): the owner authorized a release. v0.15.0 was published by hand from the
  package artifacts of `main`'s CI run 44687477700, and releases are now drafted by `release.yml`
  (Q-0000-06). The per-run artifacts stay as they were.

### Q-0013-08 — Why does the macOS credential case of the package smoke keep the real HOME?
- Status: decided(default taken)
- Context: the package smoke launches the installed app with an empty per-user root so the default
  data path runs; on macOS the login keychain is a file-based store the app finds through the real
  `~/Library`, and a temp HOME detaches it from that store.
- Options considered: (A) keep the real HOME for the credential case and move only its data root with
  `PASSQUEST_DESKTOP_DATA`; (B) redirect HOME there too and drop the macOS credential case;
  (C) provision a temporary keychain for the run.
- Default taken: (A). Every other case keeps the redirected HOME; the credential case — which stores a
  real entry through the window, proves it is in no file under the data root or the installed tree,
  then clears it — runs with the real HOME and a data root under the gate's temp tree. Its own
  cleanup deletes the entry on every exit path. Reverse by standing up a temporary keychain before the
  case.
- Revisit when: the gate moves to a temporary keychain, or the macOS runner stops exposing a usable
  login keychain.

### Q-0013-09 — What does the package gate require of a developer's Windows machine?
- Status: decided(default taken)
- Context: the NSIS installer writes the same HKCU uninstall key and the same Start menu and desktop
  shortcuts as a learner's copy, and neither `/D=` nor the run's environment redirection moves the
  shell folders, so a run could overwrite and then delete a real install's registration and
  shortcuts.
- Options considered: (A) refuse to install while a real copy is registered or one of its shortcuts
  exists; (B) install anyway and let the cleanup remove what it overwrote; (C) install under a
  machine-wide prefix.
- Default taken: (A). Before the installer runs, the gate reads
  `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\com.github.wxxb789.passquest` — refusing
  when its `InstallLocation` is outside the run's temp tree — and checks `Pass Quest.lnk` in the real
  `Programs` and `Desktop` shell folders; either fails the gate with the uninstall instruction, and an
  unreadable shell folder is a refusal, not an assumption. A temp path that holds a space moves the
  install prefix to a short directory created under the drive root, because NSIS ignores a quoted
  `/D=`. The gate's own install is removed on every exit path it controls; a run killed from outside
  can leave it registered, and the next run then refuses until it is uninstalled. Reverse by making
  the check advisory.
- Revisit when: the gate can run in a disposable VM, or the refusal blocks a legitimate developer
  workflow.

### Q-0013-10 — Which macOS version is the package's minimum?
- Status: decided(default taken)
- Context: the Go 1.27 toolchain links darwin binaries with `LC_BUILD_VERSION minos 13.0`, while
  MyGo 0.10.0 writes `LSMinimumSystemVersion` 12.0 into `Info.plist` unless told otherwise, so a
  macOS 12 Mac would accept a bundle whose binary it cannot run.
- Options considered: (A) declare 13.0 in `mygo.json` (`macos.minimumSystemVersion`) and document
  macOS 13 as the minimum; (B) build with an older Go toolchain to keep 12; (C) leave the default.
- Default taken: (A). `TestPackagingIdentity` pins the value, `docs/install.md` names macOS 13
  (Ventura), and CI proves only `macos-latest`. Reverse by building with an older Go toolchain.
- Revisit when: the Go toolchain raises its macOS minimum again, or a learner needs an older macOS.
- Revisited (2026-11-20): the owner supports only the latest two macOS majors (Q-0000-07); goal 0017
  raises the declared minimum to macOS 26.

### Q-0013-11 — How does the window treat an opaque, source-less `Script error.`?
- Status: decided(default taken)
- Context: on macOS only, the desktop window intermittently (25-50% of CI runs; diagnosed
  2026-11-19) showed the red `JS error: Script error. @0:0` bar about 1.5 s after load, failing
  desktop-ux (`page-error`) and the desktop/package smoke boot check. Diagnostic CI runs on PR #30
  (44472253919, 44475350810, 44479262910) showed `window.onerror` received `msg="Script error."`,
  `src=""`, line 0, col 0, `error=null`, while a probe error thrown from our own bundle always
  reached `onerror` in full and wrapping every listener, observer, timer, rAF and microtask in the
  bundle caught nothing: the error comes from a script MyGo injects or evaluates (likely its
  unguarded native `__mygo.receive` eval), which WKWebView reports with no origin. The app kept
  working, and MyGo 0.10.0 (latest) has no fix.
- Options considered: (A) classify an opaque, source-less error as a non-app error — counted in
  `window.__pqErrors.opaque`, no red bar — and guard the classification with a permanent
  desktop-smoke probe on every OS; (B) keep showing the bar and accept the macOS flake; (C) filter
  only the exact string with no guard.
- Default taken: (A). An error is opaque only when the message is exactly `Script error.` (or
  `Script error`), `src` is empty/undefined, line 0/undefined, col 0/undefined and the error object
  null/undefined; anything else — including `unhandledrejection` — shows the bar exactly as before.
  `desktop/web/src/core/errors.ts` owns the classification, the bar and the probe; the desktop
  smoke (and package smoke, which reuses the boot scenario) calls `window.__pqErrors.startProbe()`
  on every boot and fails the run unless the bundle's own error reaches `window.onerror` in full,
  so the classification can never hide a real app error. Reverse by removing the opaque branch; the
  probe then always arrives in full and stays green.
- Revisit when: MyGo guards its `__mygo.receive` eval or reports the underlying error, or the smoke
  probe fails on a platform.

### Q-0015-01 — Does the app check for updates on its own, or only when the learner asks?
- Status: decided(default taken)
- Context: Done-when 1 allows either an opt-in switch or an on-demand check and asks for the default
  taken; the plan left frequency open. A scheduled check needs a stored preference, runs while the
  learner is not there and would turn the app's one network request into a background behaviour.
- Options considered: (A) on demand only, with no stored preference and no timer; (B) an opt-in
  toggle that schedules a periodic check; (C) a check on every start-up.
- Default taken: (A). Settings holds one "Updates" row (the one-line disclosure, the running version
  and a "Check for updates" button) and nothing is scheduled: `desktop/web/src/views/settings.ts`
  (`updateRow`, `updateChip`) and `desktop/web/src/core/state.ts` ("driven by the learner
  (Q-0015-01): a fresh window starts idle"). The state before the first check is "not checked in this
  session", and nothing is persisted, so a relaunch starts idle again. Reverse by adding a preference
  and a scheduler; the request itself does not change.
- Revisit when: a learner misses a release because they never open Settings, or the owner wants a
  start-up check.

### Q-0015-02 — What exactly does one update check send, and what bounds it?
- Status: decided(default taken)
- Context: the local-only exception has to be exact and provable: one request that says nothing about
  the learner, with bounds that make a hung or oversized answer harmless.
- Options considered: (A) `GET` the constant endpoint with the app's two headers, a 10 s deadline, a
  1 MiB cap, redirects refused and the environment's proxy ignored; (B) a wider header set (a version
  in `User-Agent`); (C) follow redirects and let proxy settings apply.
- Default taken: (A). `internal/update` holds the whole request: `Endpoint`
  (`https://api.github.com/repos/wxxb789/passquest/releases/latest`), `Accept:
  application/vnd.github+json`, `User-Agent: PassQuest` (no version), no query, no body, no cookie;
  Go's transport adds the host (`Host` over HTTP/1.1, the `:authority` pseudo-header over HTTP/2)
  and `Accept-Encoding: gzip`, and nothing else leaves the app.
  The target (endpoint and timeout) is unexported, so no caller outside the package can name a URL of
  its own: `DefaultTarget` returns the constant pair, and `TargetFor` only honours the smoke
  overrides (Q-0015-06). `NewClient` sets `Proxy: nil` and refuses a redirect
  (`http.ErrUseLastResponse`), so the request reaches the named endpoint or nowhere; `Check` applies
  `DefaultTimeout` (10 s) as the request context and reads one byte past `maxBody` (1 MiB) before
  reporting `too_large`. `TestCheckCapturesTheRequest` fails on any other method, path, header or a
  body, and `TestCheckTimeout`, `TestCheckCapsTheBody` and `TestCheckRefusesRedirects` pin the
  bounds; Go adding a transport header fails the capture test loudly instead of passing silently.
  Reverse by changing the constants.
- Revisit when: GitHub requires another header, the endpoint redirects for real, or a rate-limited
  learner would need an authenticated request (which would change the exception itself).

### Q-0015-03 — How does the check classify what it found, and is "no release yet" its own state?
- Status: decided(default taken)
- Context: Done-when 3 asks for distinct, readable states; this repository has no published release,
  so the everyday answer is a fact to state rather than a failure to report.
- Options considered: (A) four states — `newer`, `current`, `no_release`, `unavailable` — with a
  coded reason naming a failure; (B) fold HTTP 404 into "could not check" as one more reason; (C) a
  boolean "newer or not".
- Default taken: (A). 404 is `no_release` on its own (`update.StatusNoRelease`): nothing is published
  yet. A draft or pre-release entry and an older or equal tag are `current` — the first two are
  defensive, because `/releases/latest` already excludes them. Offline, timeout, 403/429, 5xx, a
  refused redirect, an oversized or malformed body and an invalid tag are `unavailable` with one of
  nine reason codes shown as a short clause (`UPDATE_REASONS` in `desktop/web/src/core/i18n.ts`);
  study is never blocked. Reverse by dropping `no_release`; the smoke scenario's 404 step and
  [`install.md`](../install.md) change with it.
- Revisit when: GitHub returns another status for "no release", or the owner wants the failure detail
  to say more than the coded clause.

### Q-0015-04 — Are the release links taken from the response or built here?
- Status: decided(default taken)
- Context: the window shows a link the learner clicks; the response's `html_url` is data from
  elsewhere, and the P1 reviews showed a `\..`-style spelling can leave this repository once the
  browser normalises it.
- Options considered: (A) build both links from the validated tag under a constant prefix; (B) use
  `html_url`, possibly after normalising it; (C) use `html_url` for the notes and show no download
  link.
- Default taken: (A). `releasesURL` is `https://github.com/wxxb789/passquest/releases/` and both the
  notes and download links are `<prefix>tag/<tag>`; a tag reaches that point only through
  `parseVersion`, which restricts it to semver, so it can carry no slash, space, dot segment or
  escape. `TestLinksComeFromTheRepository` sends a foreign repository, a `\..` spelling and a
  `javascript:` URL in `html_url` and requires each to be ignored. The link is `target="_blank"
  rel="noopener"`, so MyGo hands it to the system browser, and the app downloads nothing. Reverse by
  using `html_url` again; the local-only gate's rule 4 would also lose the prefix literal.
- Revisit when: GitHub's release pages move off `github.com/<owner>/<repo>/releases/tag/`.

### Q-0015-05 — Where does the running version live, and what keeps the two copies equal?
- Status: decided(default taken)
- Context: the check compares the release tag with the version the learner runs; `mygo.json` carries
  the package version, and no Go constant held it (the plan's facts).
- Options considered: (A) one `update.Version` constant and `mygo.json` bumped together, held equal
  by a Go test; (B) inject the version with `-ldflags -X` at build time; (C) read `mygo.json` at run
  time.
- Default taken: (A). `mygo.json` and `internal/update.Version` are both `0.15.0`;
  `TestVersionMatchesMygoJSON` parses `mygo.json` and fails when the two differ, and desktop-ux also
  compares the version the Updates row shows with `mygo.json`, so a drifting package version fails
  that gate too. (C) would make the packaged binary depend on a resource file for its own version,
  and (B) adds a build flag nothing else needs. Reverse by bumping both; the tests fail until they
  agree.
- Revisit when: the package version and the running version must differ (a rebuilt release), or MyGo
  exposes its version to Go directly.

### Q-0015-06 — How does a scripted launch walk every check state without touching the real network?
- Status: decided(default taken)
- Context: the smoke gate needs the newer, current, no-release, timeout and offline states; the
  constant endpoint can serve none of them, and no test may contact the real internet.
  `PASSQUEST_DESKTOP_CONTENT_SIZE` is the precedent for a variable read only with the smoke hook
  (Q-0012-05).
- Options considered: (A) two smoke-only variables, read only while `PASSQUEST_DESKTOP_SCRIPT` is
  set, accepting only a loopback URL; (B) a build tag that swaps the endpoint; (C) Go tests only,
  with no scenario for the states.
- Default taken: (A). `TargetFor(getenv, smokeHook)` returns `DefaultTarget` unless the hook is set,
  then honours `PASSQUEST_DESKTOP_UPDATE_URL` only when it is exactly `http://127.0.0.1:<port>/...`
  (checked by `loopbackURL`, which does not use `net`) and `PASSQUEST_DESKTOP_UPDATE_TIMEOUT_MS` only
  within 1..10000 ms; `TestTargetForSmokeOverride` proves another host, https, a userinfo, a query, a
  fragment or an out-of-range timeout is ignored. The desktop smoke's `update` scenario points
  both variables, and desktop-ux the URL variable, at a stub in the harness process
  (`harness/lib/update-stub.ts`), whose path is read from `internal/update`'s own `Endpoint` literal,
  so a drifted stub throws instead of serving a request the app would never make. A packaged run
  reads neither variable and can only reach `Endpoint`. Reverse by deleting the two variables; the
  scenarios then fail loudly against the constant endpoint.
- Revisit when: another harness needs to script the check, or one of the variables must be honoured
  outside the hook.

### Q-0000-06 — Does a pushed version tag publish a release on its own?
- Status: decided(default taken)
- Context: on 2026-11-20 the owner asked for a release pipeline. Publishing is public and cannot
  be taken back, and project rules make it the owner's decision at the time.
- Options considered: (A) the tag builds a **draft** release, and the owner presses Publish;
  (B) the tag publishes directly; (C) manual releases only.
- Default taken: (A). `.github/workflows/release.yml` runs on `v*` tags:
  - It checks that the tag equals `mygo.json`'s version.
  - It runs the full `gate.yml` on the tagged commit (through `workflow_call`).
  - It attaches the three packages that the gate's `package` jobs built and smoke-tested, plus
    `SHA256SUMS.txt`.
  - It creates a draft with the notes header from `.github/release-notes.md` followed by GitHub's
    generated notes.
  Only the release job holds `contents: write`. Reverse by dropping `--draft` from the
  `gh release create` call (B).
- Owner decision (2026-11-24): every completed goal ships as a patch release that learners can
  download. The workflow still builds a draft; the coordinator publishes it under this standing
  decision once the tagged run is green, then follows `docs/release.md` step 5.
- Revisit when: the owner wants tag-to-public releases, signing, or an update channel.

### Q-0000-07 — Which platforms and system versions does Pass Quest support?
- Status: resolved
- Resolution: (owner, 2026-11-20) Windows and macOS are first-tier and get the best experience; Linux
  is second-tier (still built and smoke-tested, no Linux-only polish). Supported: Windows 11 and
  later, and the latest two macOS majors — macOS 27 Golden Gate and macOS 26 Tahoe per Apple's
  support page dated 2026-11-09. Windows 10 is documented as unsupported and untested but not
  blocked by the installer. Platform and architecture coverage is sufficient: no new targets.
- Context: goal 0013 declared macOS 13 (Q-0013-10) and claimed Windows x64 without a version floor;
  the owner judged older systems not worth supporting.
- Options considered: (A) keep the goal-0013 floors; (B) latest two macOS majors and Windows 11+,
  enforced on macOS by the package minimum and documented on Windows; (C) also block Windows 10 in
  the installer.
- Default taken: (B), applied by goal 0017.
- Revisit when: Apple ships the next macOS major (the supported pair moves), or a learner needs an
  older system.

### Q-0000-08 — Does the app download and install updates itself?
- Status: resolved
- Resolution: (owner, 2026-11-20) wanted eventually, deferred now: not a current priority. Goal
  0015's check-only behaviour stands until a goal revisits it.
- Context: comparable unsigned apps (FlClash) download, verify and run the installer from inside the
  app, which also spares a browser-downloaded upgrade from SmartScreen and Gatekeeper.
- Options considered: (A) keep check-only; (B) add in-app download and install as its own goal.
- Default taken: (A) for now.
- Revisit when: the owner schedules it, or a release needs learners to upgrade quickly.

### Q-0000-09 — Which engines does the desktop frontend target?
- Status: resolved
- Resolution: (owner, 2026-11-20) the frontend is written as modern code for the engines the supported
  systems run (Q-0000-07): current WebView2 on Windows 11 and WKWebView on macOS 26 and later.
  Workarounds for older engines go, native platform features are preferred where they simplify the
  code or improve performance, and performance is measured rather than assumed. Linux's WebKitGTK
  must keep working as second-tier.
- Context: goal 0011 targeted ES2022 and the styles carry 135 vendor-prefixed or compatibility
  constructs; dropping older systems makes them unnecessary.
- Options considered: (A) keep the goal-0011 feature level; (B) target the supported engines' baseline.
- Default taken: (B), applied by goal 0019.
- Revisit when: the supported system pair moves, or a modern feature measurably slows an engine.

### Q-0016-01 — Is bundling the shipping rule, with no fallback to the old tree?
- Status: decided(default taken)
- Context: every package used to stage the hosts' whole production `node_modules` (12,768 files /
  84.9 MB, 88 packages) while only `agent/durable-host.mjs` loads any of it. A bundle was proven
  workable, but a staging path that *could* fall back to the old tree would ship it silently the day
  one specifier stops resolving or one asset appears — and the size evidence would describe a tree
  no gate tested.
- Options considered: (A) bundling is the rule — a host that names a non-`node:` specifier is bundled,
  a host that names none is copied byte-identically, and a bundle that cannot be complete (an
  unresolvable specifier, a specifier resolved at run time, a symlink, a directory, a non-`.mjs`
  file, an embedded machine path) fails the run; (B) stage the hoisted tree and bundle
  opportunistically; (C) ship both and choose at run time.
- Default taken: (A). `stageAgent`/`bundleHost` (`harness/lib/package.ts`) and `assertLeanAgentTree`,
  which runs at staging and again over the installed tree in the package smoke's contents check, so
  the installed tree — not only the staged one — holds the four hosts plus
  `THIRD-PARTY-NOTICES.txt` and nothing else. Reverse by restoring the `stageAgentModules` path
  (deleted in P1) and the checks; the contents check's must-fail fixtures (`installed agent tree
  carrying node_modules`, a host that still names `openai`) fail until the old shape returns.
- Revisit when: a host genuinely needs a native addon, a wasm asset or a non-JavaScript file — then
  the rule needs a decided exception that proves the asset travels, not a silent fallback.

### Q-0016-02 — How is a host that loads packages bundled?
- Status: decided(default taken)
- Context: a plain `bun build agent/durable-host.mjs --target=node --format=esm` answers `status` but
  faults on a real brief (`task_faulted: Cannot read properties of undefined (reading 'get')`): the
  packages declare `sideEffects`, so bun resolves the host's named imports to hoisted declarations
  and drops their initialization. `--minify`, `--splitting`, `--sourcemap`, `treeShaking: false` and
  `ignoreDCEAnnotations: true` all still fault — the experiment was a start-and-answer lead, never a
  model call.
- Options considered: (A) build from a generated entry that namespace-imports every specifier the
  host names (resolved from `agent/` with `Bun.resolveSync`) and then imports the host; (B) keep the
  tree and patch the affected initialization; (C) vendor the packages into the repository and build
  without a bundler.
- Default taken: (A). `bundleEntrySource` writes the entry and `bundleHost` builds it; both the entry
  and bun's metafile live under `build/package-cache/bundles/` — intermediates that never ship, at a
  stable machine-free path, because a bundle comment carrying the build machine's absolute temp path
  (the P1 review's finding) leaks an account name and makes two stagings of one source differ. The
  run builds the host twice and requires byte-identical output. Reverse by deleting the entry
  generator and building the host itself; the durable matrix's bundle pass then faults in its first
  section.
- Revisit when: bun changes how it treats `sideEffects` — a plain bundle would have to pass the
  matrix before the entry is removed.

### Q-0016-03 — Do the staged file's name and the source-tree resolution stay?
- Status: decided(default taken)
- Context: `internal/hostpaths` resolves `agent/<host>.mjs` by name from the packaged resources;
  `--outdir` makes bun rename a `.mjs` entry to `.js`; and `agent/package.json`, `bun.lock` and
  `agent/node_modules` are what every source-tree gate and a developer working from `agent/`
  resolve through.
- Options considered: (A) the staged file keeps the host's own name and only the packaging path
  changes; (B) rename the hosts to `.js` on both sides; (C) bundle from a copied tree and keep
  `node_modules` in the checkout.
- Default taken: (A). `bun build … --outfile=<stage>/agent/<host>` keeps the name (`--outdir` is
  never used), `agent/package.json`, `bun.lock` and `internal/hostpaths` are untouched, and the
  durable matrix runs the source host first so a broken source path fails the same gate. Reverse by
  renaming both sides in one change; the layout check and the matrix name the host.
- Revisit when: the hosts stop being `.mjs`, or a host must resolve by a path other than its name.

### Q-0016-04 — Are the shipped bundles minified, or mapped back to their sources?
- Status: decided(default taken)
- Context: a 1.55 MB bundle is a reviewable artefact: human-readable output is what makes a future
  notice, provenance or "which code is this" check possible on the shipped file.
- Options considered: (A) no minification and no source maps; (B) minify; (C) minify with an external
  source map.
- Default taken: (A). The staging command is `bun build … --target=node --format=esm --outfile
  --metafile` and nothing else, so what the size table measures is what a reader can open. A source
  map would also be a second file in the staged tree, which `assertLeanAgentTree` refuses (the tree
  holds `*.mjs` and the notices file). Reverse by adding the flag; the lean check must be taught the
  new file type first.
- Revisit when: bundle size or start-up becomes a measured problem — with a number, not a
  preference.

### Q-0016-05 — How do the third-party notices travel with the shipped code?
- Status: decided(default taken)
- Context: the bundles inline seven third-party packages (Apache-2.0, MIT, ISC and BSD-3-Clause
  texts); those licences require their notices to accompany a redistribution, and the package ships
  no `node_modules` whose files would carry them.
- Options considered: (A) staging assembles `agent/THIRD-PARTY-NOTICES.txt` from the packages each
  bundle inlines and the package smoke verifies the installed file carries every text the staged
  bundles required; (B) ship each licence beside its package inside the bundle's tree (needs
  `node_modules`); (C) link to the licences in the documentation.
- Default taken: (A), in `harness/lib/notices.ts` (a pure `assembleNotices(packages, source)` so the
  self-test can drive it) and `harness/lib/package.ts` (the assembly reads the metafiles of every
  bundle it staged, unioned, deduplicated and sorted, and the file lands beside the hosts). The
  contents check compares the installed file's texts against the staging result's set and names how
  many it verified, and `THIRD-PARTY-NOTICES.txt` is the one file `assertLeanAgentTree` accepts next
  to the hosts. Where a package ships no licence file of its own (all `@earendil-works` packages)
  the SPDX MIT text with the package's own `author` line stands in — our wording, disclosed in the
  evidence record. Reverse by dropping the assembly; the contents check and four self-test units
  fail.
- Revisit when: a dependency's licence cannot be stated by a copied text or the SPDX MIT fallback
  (then the assembly must gain that licence's text rather than the package's terms being guessed).

### Q-0016-06 — Is the durable host's cold start gated or reported?
- Status: decided(default taken)
- Context: the goal asks for the durable host's cold start before and after with a sample count and
  spread; this machine is frequently at 100% CPU, so any absolute bound would either gate the
  machine's load or be too loose to notice a real regression.
- Options considered: (A) reported — 15 `status` samples on the durable host per tree, min/median/max
  and the raw samples in the evidence record, with the CPU load stated beside them; (B) gated on an
  absolute bound; (C) gated as a ratio against the unbundled host.
- Default taken: (A). The plan's wording was 15 samples `per host`; only the durable host is
  measured, because it is the one host whose staging changed — the other three ship as byte-identical
  copies of their sources, and the goal asks for the durable host's start-up. Staging keeps a *load
  proof* only (the staged host answers one `status` request on the staged runtime, printed as
  `durable-host.mjs answered status on the staged runtime in N ms`) — a plain bundle passes that
  too, which is why P2's matrix and P3's `brief` case are the behavioural checks. The measured
  numbers live in `docs/evidence/0016-lean-package.md`. Reverse by adding a bound to the staging
  check or a gate case; it must be a margin this machine's worst observed slowdown cannot cross.
- Revisit when: a start-up regression slips through — then gate a ratio (bundle against source host
  on one machine), not a millisecond bound.

### Q-0016-07 — Must the generated bundle entry hold its bindings?
- Status: decided(default taken)
- Context: the entry (Q-0016-02) exists because bun dropped the packages' initialization; an entry
  that namespace-imports a specifier but never mentions the namespace again could be dropped by the
  same pass — silently, in a file that still answers `status`.
- Options considered: (A) the entry references every namespace binding in an exported value, and a
  unit self-test pins the entry's exact shape; (B) rely on the durable matrix to catch a dropped
  prelude; (C) a build flag that marks the packages side-effectful.
- Default taken: (A). `bundleEntrySource` emits one `import * as pinnedN from <resolved path>` per
  specifier in the host's order, then `export const pinned = [pinned0, …]`, then the host, and
  `bundleEntryProblem` (a self-test unit) asserts exactly that shape — so a dropped prelude or a
  reordered entry fails a unit instead of shipping. The matrix's bundle pass stays the end-to-end
  check: `plainBundleFaultProblem` runs the matrix against a plain bundle and requires the
  `task_faulted` failure, so the entry's necessity is itself tested. Reverse by emitting bare
  imports; the unit fails before the bundle reaches a package.
- Revisit when: a second host needs bundling or the entry shape changes — the assertions cover the
  loaded specifier set, not one hard-coded host.

### Q-0016-08 — Does `--metafile` take a separated value?
- Status: decided(default taken)
- Context: staging reads bun's metafile to learn which packages a bundle inlined — the notices set
  (Q-0016-05) and the lock check (Q-0016-11) both come from it. On the pinned bun 1.4.5 the
  documented `--metafile <file>` spelling parses the value as a second entry point.
- Options considered: (A) the inline form `--metafile=<file>`; (B) a bun config file; (C) parse the
  bundle text instead of the metafile.
- Default taken: (A), with the reason in a comment beside the command (`bundleHost`). A wrong
  spelling fails loudly at staging — without the metafile, `bundlePackages` cannot read the inlined
  set — so the failure mode is a red run, never a silently unnoticed bundle. Same for `--outfile=`.
  Reverse when bun accepts the separated form again; the metafile stays required either way.
- Revisit when: the pinned bun version changes (re-test both spellings on the upgrade).

### Q-0016-09 — Which files of an inlined package travel in the notices?
- Status: decided(default taken)
- Context: the licences that must travel belong to the code the bundle carries, and the inlined
  packages keep licence texts in many shapes — `LICENSE`, `MIT-LICENSE`, `THIRD-PARTY-NOTICES.txt`,
  a `LICENSES/` directory, `THIRD_PARTY_NOTICES/<subdir>/…` — and some vendor code whose licence is
  neither (`openai` carries `src/internal/qs/LICENSE.md`). Omitting a licence is a compliance
  failure; carrying one text too many is not.
- Options considered: (A) over-include: a file travels when its *name* carries a licence word
  (`licen[cs]e|notice|copying`) or when it sits under a directory whose name carries one
  (`licen[cs]es|notices`), at any depth, minus code extensions (`.js`, `.mjs`, `.cjs`, `.ts`,
  `.mts`, `.cts`, `.d.ts`, `.map`, `.json`) so `license-checker.js` stays out; (B) only a root-level
  `LICENSE`/`NOTICE`; (C) only the file a package's `license` field names.
- Default taken: (A), in `harness/lib/notices.ts` (`LICENCE_WORD`, `LICENCE_DIR_WORD`,
  `CODE_EXTENSIONS`). The search walks each package's own tree only; a nested `node_modules` holds
  other packages, each named by the metafile and noticed for itself. The asymmetry is deliberate and
  disclosed: the real set carries 11 sections for 7 packages, a licence name the rule misses is a
  known limit in the evidence record, and a package whose *own* terms are not in its tree — the only
  licence text it ships sits under `THIRD_PARTY_NOTICES/`, as in `@earendil-works/pi-schema` — still
  gets the SPDX MIT fallback for its own terms (10 sections instead of 11 would be the failure).
  Reverse by narrowing the rule; `licenceNamesProblem` and `noticesAssemblyProblem` pin both sides.
- Revisit when: a package ships a licence under a name the rule misses (the fix is a name, not a
  broader blacklist of code extensions).

### Q-0016-10 — What happens when a licence read fails?
- Status: decided(default taken)
- Context: the licence search reads a package's tree for the notices. A permission error read as
  "not there" would silently replace a real licence with the SPDX fallback — a fail-open that turns
  a compliance requirement into a green run. The P4 platform review reproduced exactly that.
- Options considered: (A) fail closed: only `ENOENT`/`ENOTDIR` mean "not there", every other error
  (`EACCES`, `EPERM`, `EBUSY`, `ELOOP`, `EISDIR`, …) aborts staging with the path and the errno, and
  a symlink where a file or directory was expected is refused by name; (B) any error counts as
  absent; (C) log and continue.
- Default taken: (A). `noticeSource(fs = FILE_SYSTEM)` in `harness/lib/package.ts` is the read seam:
  `pathAbsent` recognises only `ENOENT`/`ENOTDIR`, `unreadable` builds the staging error naming the
  file and the operation. The self-test drives the seam with 24 injected failures (six errnos at
  four call sites) and requires each to throw, and a probe (`denied-read.ts`, under
  `Q:/scratch/omp/g16/P4/newrule/`) reproduced the old shape's silent SPDX swap. Reverse by
  returning `undefined` for every error; `noticeSourceProblem` fails.
- Revisit when: a package legitimately needs a skipped path (a broken link inside a licence
  directory) — the skip must then be named, not silent.

### Q-0016-11 — Are a bundle's inputs taken from the checkout or checked against the lock?
- Status: decided(default taken)
- Context: bundling resolves from the checkout's `agent/node_modules` (bun's workspace store), not
  from a fresh `--frozen-lockfile` install; a stale tree there would ship a version `bun.lock` does
  not pin, and the lock is the only committed statement of what a release contains.
- Options considered: (A) read every inlined package's name and version out of its own
  `package.json` (through the metafile) and fail staging unless `bun.lock` pins exactly that
  version; (B) install a fresh tree into a temp directory for every staging; (C) trust the checkout.
- Default taken: (A). `bundlePackages` (metafile → inlined packages), `lockedVersions` (the text
  lockfile, read with `Bun.JSON5`), `lockProblems` (a name the lock does not carry, or another
  version, fails). The registry settings and the lock are untouched, and an unreadable or
  unparsable lock fails staging instead of skipping the check. `bundleLockProblem` and
  `bundlePackagesProblem` are self-test units with must-fail inputs. Reverse by deleting the check;
  the units fail.
- Revisit when: staging grows a mode that installs its own tree — then that install must satisfy the
  same check against the same lock.

### Q-0016-12 — How does the harness pause a run without racing a loaded machine?
- Status: decided(default taken)
- Context: agent-smoke's section 9 and the package smoke's `brief` case need a *paused* run that
  still carries durable work. The original recipe (`agent start --timeout-seconds 1` against a
  delaying stub) lost the race on a machine at 100% CPU — the deadline fired before the host had
  enqueued anything — and even a 20-second deadline left only ~1.2× over the same machine's worst
  observed slowdown.
- Options considered: (A) trigger the pause from the stub's side: wait until the loopback stub has
  recorded the run's own first provider request (the host submits its durable task before it calls
  the provider), then write the control-file pause request the host already polls, with
  `--timeout-seconds 60` kept as a bound rather than the trigger; (B) keep a larger deadline;
  (C) retry the run until the pause finds work.
- Default taken: (A). `harness/lib/host-control.ts`: `PAUSE_DEADLINE_SECONDS = 60` and
  `startPausedRun(start, requests, storePath)`, used by both callers; the deadline stays in the
  command line so a host that never reaches the provider still fails a gate in bounded time instead
  of hanging it. This makes "the pause found durable work" true by construction instead of by
  timing. Reverse by restoring a short deadline; both cases then fail intermittently on a loaded
  machine, which is why the recipe exists.
- Revisit when: a gate needs a pause at a different lifecycle point (before the provider request) —
  the trigger must then be a different real event, not a tighter bound.

### Q-0017-01 — Which macOS version does the package declare as its minimum?
- Status: decided(default taken)
- Context: Q-0013-10 declared 13.0 as "the toolchain's minimum": the Go 1.27 linker writes the
  Mach-O `LC_BUILD_VERSION minos 13.0`, MyGo 0.10.0's own `LSMinimumSystemVersion` default is 12.0,
  and 13.0 was the last value the tree could defend. The owner's decision (Q-0000-07) supports only
  the latest two macOS majors — macOS 26 Tahoe and macOS 27 Golden Gate, per Apple's list — so a
  package that declares 13.0 invites an unsupported system to open a bundle no CI job covers.
- Options considered: (A) declare 26.0 in `mygo.json` (`macos.minimumSystemVersion`) and leave the
  linker's `minos` at 13.0; (B) also build with `-ldflags -macos=26.0` so the Mach-O states it;
  (C) leave 13.0 and only document the pair.
- Default taken: (A). MyGo copies the value verbatim into `LSMinimumSystemVersion`
  (`cmd/mygo/bundle.go:254`), and the effective floor is the higher of that key and `minos` — the
  key (26.0) is above Go 1.27's 13.0, so the key binds; `minos` is not ignored (dyld refuses a
  binary whose `minos` is newer than the running system), it is simply not the binding floor.
  `TestPackagingIdentity` pins 26.0 and its comment states this; `plistProblems`
  (`package-smoke/contents.ts`) fails an installed `.app` whose plist disagrees, naming both
  values, and three self-test cases (one must fail at 13.0, two must pass) run on all three OSes.
  The guides, `docs/desktop.md` and `docs/cross-platform.md` state the 26/27 pair. Reverse by
  restoring 13.0 and the old comment; the test then fails at 26.0 the other way.
- Revisit when: the supported pair moves again (Apple ships macOS 28), or the Go toolchain's own
  `minos` passes the declared floor — then the plist key stops being the binding one and the
  choice must be re-read, not re-applied.

### Q-0017-02 — What is the cask called, where does it live, and what does the learner run?
- Status: decided(default taken)
- Context: goal 0017 needs a public tap whose cask installs the published `.dmg`. Homebrew
  requires explicit trust for a non-official tap since 6.0.0, but a fully qualified install trusts
  "only that item" (`docs.brew.sh/Tap-Trust`), so the learner's whole step can stay one command.
  The tap repository, its deploy key and the Actions secret are the owner's actions, taken by the
  coordinator.
- Options considered: (A) token `pass-quest` in tap `wxxb789/homebrew-tap`, file
  `Casks/pass-quest.rb` (the basename must equal the token, or `cask/cask_loader.rb` raises
  `CaskTokenMismatchError`), learner runs `brew install --cask wxxb789/tap/pass-quest`; (B) a
  longer token, or a separate `brew trust` step in the guide; (C) install from a URL instead of a
  tap.
- Default taken: (A). `install-guide` pins the literal command in both guides and the release
  notes; `harness/lib/cask.ts` renders the template and checks the token, tap, identifier and data
  directory; the tap's file is byte-compared with a fresh render (the `install` job's `tap-render`
  assertion). Every `brew` call in `harness/tools/cask-install.ts` addresses the cask fully
  qualified because the runner also carries the mutant tap (`Cask pass-quest exists in multiple
  taps`, run `44909338098`), and the tool runs `brew trust --cask wxxb789/tap/pass-quest` before
  the strict audit because Homebrew 7.0.14 refuses to load an untrusted tap for `brew audit` even
  after the qualified install trusted it (run `44910568731`). The guides' paragraph about the
  workflow names the uninstall fully qualified too (`brew uninstall --zap --cask
  wxxb789/tap/pass-quest`), the command the job runs: the short name resolves only while one tap
  carries the cask.
- Revisit when: Homebrew changes the trust rules for casks or `brew audit`, or a second cask with
  this token has to be installed beside ours anywhere a command here is copied.

### Q-0017-03 — What does the cask declare besides the version and SHA-256?
- Status: decided(default taken)
- Context: the goal asks the cask to install the latest published release and to serve only the
  systems the goal supports; Homebrew has a house vocabulary for both, and `brew audit --online`
  compares a cask's declared macOS minimum with the artifact's own `LSMinimumSystemVersion`
  (Cask-Cookbook, "Choosing the minimum macOS release").
- Options considered: (A) `depends_on macos: :tahoe` plus `depends_on arch: :arm64`, version and
  sha256 from the release asset, `livecheck do url :url; strategy :github_latest end`, and a `url`
  interpolating `v#{version}`; (B) a looser `depends_on macos: ">= :tahoe"` and no arch line;
  (C) no livecheck.
- Default taken: (A). `depends_on macos: :tahoe` is 26 and `:golden_gate` is 27
  (`macos_version.rb`), so the cask refuses every older system and accepts both supported majors;
  the `.dmg` is arm64 only, and the arch line makes the refusal explicit; the digest is
  cross-checked against the release's own `SHA256SUMS.txt` by `cask-publish.ts`, and the gate pins
  the v0.15.0 render byte for byte; `livecheck`'s `:url` symbol resolves to the cask's own `url`
  and `GithubLatest` applies only when named (`livecheck/strategy.rb`).
- Revisit when: an Intel or universal `.dmg` is ever published, or Apple ships a third supported
  major and the cask should accept it.

### Q-0017-04 — How does the cask clear the quarantine flag, and why `must_succeed: false` with no `writable_*`?
- Status: decided(default taken)
- Context: Homebrew quarantines every download by default (its security page says it does so "so
  Gatekeeper performs these checks instead of Homebrew bypassing them"), and `--no-quarantine` no
  longer exists (Homebrew/brew#23363), so an unsigned cask must clear the flag itself or the first
  launch meets Gatekeeper. A third-party cask does that in the structured `postflight_steps` form.
  The first draft in the tree declared `writable_paths: ["{{appdir}}/Pass Quest.app"]` with
  `writable_base: :appdir` — a token combined with a base, which is not the documented relative
  form.
- Options considered: (A) `postflight_steps do run "/usr/bin/xattr", args: ["-rd",
  "com.apple.quarantine", "{{appdir}}/Pass Quest.app"], must_succeed: false end` with no
  `writable_*` — the Cookbook's `run` paragraph already permits writes to the app directory, and
  `xattr -rd` exits non-zero when the path carries no such attribute, which a re-install over the
  same cask produces; (B) keep the declared `writable_*`; (C) clear the flag in a shell
  `postflight` block.
- Default taken: (A). The install job proves the minimal stanza removes the flag on the app and on
  the nested executable (run `44912433442`), and its `quarantine` mutant proves the assertion
  catches a cask without the block
  (`[assert] mutant-quarantine: … 0181;…;Homebrew\x20Cask;… — the attribute postflight_steps
  removes is still there`). `must_succeed: false` follows the Cookbook's own rule for a cleanup
  command whose target state — no attribute — is the state the step wants, and the template
  comment states the reason.
- Revisit when: Homebrew's step sandbox changes so the app directory is no longer writable, or
  `xattr` changes its exit status for a missing attribute.

### Q-0017-05 — When does the cask workflow run, and what does each path do?
- Status: decided(default taken)
- Context: the plan wanted `release: [published]` plus `workflow_dispatch`; GitHub registers a
  dispatchable workflow only once its file is on the default branch, so dispatch cannot produce
  the pre-merge evidence, and the plan's weekly `schedule` is not in the contract. The tap must
  also be installed and asserted on a real macOS runner, which a gate cannot do (gates make no
  external network call).
- Options considered: (A) `pull_request` with a `paths:` filter running the v0.15.0 dry run plus
  the real install, `release: [published]` guarded `!draft && !prerelease` (update pushes, install
  verifies), and `workflow_dispatch` with `tag` default `v0.15.0` and `push` default `false`;
  (B) `push` on the branch; (C) the weekly schedule.
- Default taken: (A). The `paths:` filter lists the workflow, the template, `mygo.json`, the
  golden and the twelve repo files of the two tools' import closure, so a change to anything the
  tools load re-runs the workflow; the dry-run step's environment holds no secret; the install job
  runs on `macos-26` (the floor, not `macos-latest`) and asserts `sw_vers` first. The pull-request
  run is what proved the tap install before the merge (runs `44905438010`, `44909338098`,
  `44910568731`, `44912433442`); after the merge `gh workflow run cask.yml -f tag=v0.15.0 -f
  push=false` (or `push=true`) repeats it. The tag travels in `env:` and never inside a `run:`
  body, and both the step and the tools require the three-number shape.
- Revisit when: GitHub changes how a branch-only workflow can be dispatched, or the goal gains a
  scheduled check.

### Q-0017-06 — What credential writes the tap, and where does it live?
- Status: decided(default taken)
- Context: the cask must move on its own when a release is published, with no owner login in the
  loop and with the key never reaching a log or the tree. One write credential is enough: a deploy
  key scoped to `wxxb789/homebrew-tap`.
- Options considered: (A) one write deploy key, only as the `HOMEBREW_TAP_DEPLOY_KEY` Actions
  secret, written by the tool to a `0600` file for the one `git push`; (B) a fine-grained PAT;
  (C) the workflow's `GITHUB_TOKEN` (it has no access to another repository).
- Default taken: (A). `cask-publish.ts` reads the key from the environment only on `--push`,
  writes it under `umask 077` into the run's `0700` temp tree (NTFS ACLs decide on Windows; the
  comment says so), passes it to `git` through `GIT_SSH_COMMAND`
  (`-F /dev/null -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new -o BatchMode=yes`),
  prints only the ssh remote and the commit id, deletes the file, commits as the owner
  (`-c user.name/user.email`), and skips a render the tap already carries. The tap's history is
  public and disclosed whole in the evidence record: `77210f9` is the round-1 accidental push
  through the owner's https credential under the identity `passquest-cask-update`; `d9cf7eb` and
  `82b6693` are the coordinator's seeds with the deploy key, the second with a temporary key that
  was removed afterwards. The deploy-keys list now holds one key (`passquest cask update`,
  write, created 12:17:40Z) and the secret is its pair; `secrets` scans the tree (769 files, 0
  findings in run `44912433474`).
- Revisit when: the tap moves, the owner rotates the key, or GitHub changes deploy-key
  capabilities.

### Q-0017-07 — Which paths does `zap trash:` carry, and where were they measured?
- Status: decided(default taken)
- Context: the goal asks `--zap` to remove the per-user data directory the install guide names.
  The first tap install (run `44905438010`) listed `$HOME/Library` before and after the uninstall
  and found two more paths the app creates: `~/Library/Caches/com.github.wxxb789.passquest` and
  `~/Library/WebKit/com.github.wxxb789.passquest` — both survived that round's `--zap`.
- Options considered: (A) the three-element array the measurement named, alphabetical, read back
  from the installed cask file by the tool and asserted path by path; (B) the data directory only;
  (C) a glob.
- Default taken: (A); the array is `brew generate-zap`'s own output format and alphabetical
  because the `Cask/ArrayAlphabetization` cop requires that of a `zap` array
  (`rubocops/cask/array_alphabetization.rb`). The final run's `[state] the installed cask file's
  zap trash: names 3 path(s)` line lists them, the before/after listing goes 2 → 0 entries, and
  the `zap` mutant shows a cask without the stanza leaves the marker behind. The path list is also
  what the guides describe under "Where your data lives".
- Revisit when: the app writes another per-user path (the same listing command catches it on the
  next install), or WebKit changes where it keeps the app's profile.

### Q-0017-08 — Where do the owner's screenshots go, and what ships until they exist?
- Status: decided(default taken)
- Context: the goal says Gatekeeper screenshots come later from the owner and the guide ships with
  accurate text and marked places for them; a markdown image link to a missing file would fail
  `docs-links` (it resolves images too), so a placeholder cannot be an image.
- Options considered: (A) an HTML comment `<!-- screenshot: <name> -->` plus one italic caption
  line per place, no image link; (B) a committed placeholder PNG; (C) nothing.
- Default taken: (A). Both guides carry 7 places with the same names, the gate requires the counts
  to agree, and the evidence record lists them as pending. The owner's wording review is the other
  half of Done-when 2 and blocks nothing.
- Revisit when: the owner supplies the images — then each place becomes an image link (the
  `docs-links` gate will resolve it) and the italic line becomes the caption.

### Q-0017-09 — What is not proven, and why is the tap install a workflow rather than a gate?
- Status: decided(default taken)
- Context: the goal's own known limit: CI has no macOS 27 runner, so macOS 27 is covered by
  documentation only. Separately, the tap install downloads a public release and runs `brew` — an
  external network call — which harness rules keep out of gates ("a gate makes no external network
  call", `docs/harness.md`).
- Options considered: (A) the tap install stays a workflow job on `macos-26` with `sw_vers`
  asserted first, and macOS 27 stays documented; (B) a gate that skips without the network;
  (C) a macOS 27 runner that does not exist.
- Default taken: (A). The install job prints its runner (`macos-26-arm64`, macOS 26.7.2 in run
  `44912433442`) and its first assertion names the runner instead of failing like a broken cask
  below the floor; `install-guide` keeps the platform-independent half (the template render, the
  guides, the supported pair) on all three OSes. The one Homebrew the fix was never re-run on is
  disclosed: run `44910568731` (7.0.14) ends at the untrusted-tap refusal, and the run carrying
  the `brew trust` fix landed on a 7.0.11 runner.
- Revisit when: GitHub provides a macOS 27 runner, or Homebrew's cask sandbox makes the tap
  install runnable without the network (a fixture cask, like the mutants' scratch tap).

### Q-0018-01 — How does the Keychain experiment run?
- Status: decided(default taken)
- Context: P1 had to settle, on a runner where nobody can click a Keychain prompt, whether the next
  ad-hoc or self-signed build can read what the build installed before it stored in the learner's
  Keychain. The first shape (three arms in one job) ended at the ad-hoc arm's blocked read before the
  self-signed arm ran (run `44931358398`); a later shape imported the throwaway identity into the
  login keychain, where `security set-key-partition-list -k ""` exits 1 — the login keychain's
  password is not available to a run — so `codesign` never signed (run `44986005367`).
- Options considered: (A) one job per arm and keychain, the identity always in a temporary signing
  keychain the run creates itself with an empty password and signs with the SHA-1 `security
  find-identity` prints; (B) one job carrying every arm; (C) the throwaway identity imported into the
  keychain the app stores its credential in.
- Default taken: (A). `.github/workflows/keychain.yml` is a `fail-fast: false` matrix `arm: [adhoc,
  self-signed]` × `keychain: [experiment, login]` on `macos-26`; each job builds two packages (A from
  the pull request base, B from the head) and runs one arm against one keychain. The `project` arm
  exists in the tool but sits outside the matrix and records its skip by name without the two signing
  secrets (a fork pull request has none). Plan amendments 15 and 18 record the shape; reversal is
  running one arm by hand or deleting the workflow.
- Revisit when: the goal reopens (Q-0018-04). If the `project` arm is wanted, it is not in the matrix:
  with a Developer ID, store it as the two secrets, add `project` to the workflow's `arm` matrix, then
  run the workflow by `workflow_dispatch`.

### Q-0018-02 — What did the decisive run show, and does a self-signed identity keep Keychain access?
- Status: decided(default taken)
- Context: the decisive run is `keychain` 44994402573 on commit `5dd680b`: four jobs, all green
  (`status: ok`, empty `failures`; 17 assertions in each ad-hoc job, 28 in each self-signed job,
  `[self-check] 19 of 19` in all four). A is `3250fb0`, B is `5dd680b`; the builds differ
  (`guards.buildsDiffer: true`, cdhashes `b9bcd4b4…` and `f9ea3de0…`). Each reading below is from the
  named artifact's `keychain-record.json` and is quoted with its field path in the evidence record.
- What the run showed: the two ad-hoc jobs' reads were `blocked` by a prompt (`refusesBy:
  application` in the tool's own keychain, `both` in the login keychain). The self-signed arm in the
  login keychain — the learner's shape — was `blocked` with `refusesBy: partition` while
  `requirementMatch: exact`: both builds carry the byte-equal designated requirement `identifier
  "com.github.wxxb789.passquest" and certificate root = H"8ca4ef16…"`, and the item's partition list
  `["cdhash:073434e4…"]` still does not admit B's `cdhash:20cab049…`. The self-signed arm in the
  tool's own keychain reads (`read.outcome: read`, the stub received the key) only because that
  keychain's item carries no partition entry (`partitionEntry: false`); the login keychain's item
  does carry one. Apple's source agrees: only Apple-signed and team-ID-bearing code (Mac App Store,
  TestFlight, Developer ID, development) gets a stable `apple:`/`teamid:` partition, and every other
  signed process gets `cdhash:` — the value of the build that stored the item
  (`securityd/src/clientid.cpp`, tag `Security-61901.120.67`, `:282-286`; plan fact 2).
- Options considered: (A) stop and report — the plan's second stop condition; (B) ship the
  self-signed identity anyway; (C) read the tool keychain's `read` as the answer.
- Default taken: (A). Done-when 2 is not achievable without an Apple-issued certificate with a team
  ID, and the plan's defaults 3 (a `project` arm in the pull request run), 6 (the project identity),
  7 (`-sign` in `package-build.ts`), 8 (the pinned DR) and 9 (the CI wiring) are **not taken**;
  default 10 is taken with the owner's decision in Q-0018-03. Reversal: a Developer ID exists, then
  Q-0018-04 runs.
- Revisit when: a Developer ID exists (Q-0018-04).

### Q-0018-03 — What happens to goal 0018 now that a self-signed identity cannot keep Keychain access?
- Status: resolved
- Resolution: (owner, 2026-11-22) record and park. No identity is added, no secret is created, and
  nothing in the package or release path changes; the goal stops at its second stop condition, P2–P4
  are dropped (plan amendment 19), and both install guides say in their macOS section that an
  upgrade — a new download, or `brew upgrade` — asks once whether Pass Quest may use the saved key
  (enter the login password, choose **Always Allow**), that the key stays in the Keychain, and that
  the prompt exists because the app is not signed by an Apple-issued identity. `install-guide` pins
  the statement in both languages.
- Context: the decisive run's self-signed × login job refuses on the partition list (Q-0018-02), so
  a project-owned self-signed certificate cannot meet Done-when 2; Done-when 3 is not applicable —
  no key was created, and the throwaway identities lived only inside the CI jobs and were deleted
  with their temporary signing keychains (cited in `docs/evidence/0018-keychain-survives-upgrade.md`).
- Options considered: (A) record and park until a Developer ID exists, with the guides disclosing the
  prompt; (B) ship a self-signed identity anyway; (C) leave the guides silent and treat the prompt as
  expected behaviour.
- Default taken: (A). Reversal: the owner says the cost of a Developer ID is not worth it (then (C)'s
  silence becomes the only alternative) or supplies one (Q-0018-04).
- Revisit when: a Developer ID exists.

### Q-0018-04 — When and how is the parked goal revisited?
- Status: open
- Context: the goal can only be finished by an Apple-issued identity: a Developer ID gives the app a
  `teamid:` partition that does not move with the build, and the workflow's `project` arm exists to
  measure exactly that. The parked run cost is also recorded here: four macOS jobs of two package
  builds each is too much for every desktop change, so `keychain.yml`'s `pull_request` trigger now
  lists only the workflow and `harness/tools/keychain-experiment.ts` plus that tool's repo-local
  import closure (plan amendment 20), and `workflow_dispatch` is the standing way to re-measure after
  a product change.
- Options considered: (A) keep the workflow dispatchable and, when a Developer ID exists, add the
  `project` arm to its matrix and run it; (B) delete the workflow now and rebuild it later; (C) widen
  the `pull_request` trigger back to the product sources.
- Default taken: (A). `.github/workflows/keychain.yml` runs `arm: [adhoc, self-signed]` and its run
  step's `env:` carries the throwaway self-signed variables only — nothing passes
  `PASSQUEST_MACOS_SIGNING_P12`/`_PASSWORD` to the tool — so a `workflow_dispatch` today repeats the
  same four jobs and never selects the `project` arm (`grep -n project .github/workflows/keychain.yml`
  is empty). The arm is therefore untested: it never ran with the two signing secrets, and the plan's
  coordinator route exports the `.p12` with `openssl pkcs12 -export`'s default PBE (AES-256/PBKDF2 on
  OpenSSL 3+), which the throwaway path had to replace with `-legacy` or explicit PBE flags before
  `security import` accepted it (plan amendment 13) — the re-export must follow that route. When the
  arm first runs, the DR it prints is read from the run's record before anything is pinned.
- Revisit when: a Developer ID (or an equivalent team-ID-bearing certificate) exists; then store it as
  the two secrets, add `project` to the workflow's `arm` matrix, and run `keychain.yml` by
  `workflow_dispatch` against a released package, continuing at plan P2/P3 with the identity's own DR
  and fingerprint.

### Q-0019-01 — What is the frontend's engine baseline?
- Status: decided(default taken)
- Context: goal 0019 writes the frontend for the engines Pass Quest actually ships on, and the exact
  baseline decides which workarounds may go. The grounds are the plan's fact 1 (scouted 2026-11-22):
  the Windows runner's WebView2 Evergreen channel updates to current stable Chromium (157 at scout
  time, `learn.microsoft.com/…/webview2/concepts/versioning`); the `macos-26` runner's WKWebView is
  the OS's WebKit framework, Safari 26.6 on image `20260824.0517.1`; `ubuntu-latest` installs
  `libwebkit2gtk-4.1-0` **2.52.6** (`packages.ubuntu.com/{noble,resolute}`), which MyGo `dlopen`s
  (`mygo@v0.10.0/internal/linux/ffi.go:904-912`); and `bun build` has no browser-version target, so
  the tsconfig `target`/`lib` is the only switch.
- Options considered: (A) name the oldest engine each first-tier platform can run — Evergreen
  WebView2, macOS 26's system WebKit, WebKitGTK 2.52.6 second-tier — and pin `ES2024` plus the CSS
  level all three support; (B) write for the newest engines and take features the floor cannot run;
  (C) keep ES2022 and the old-engine workarounds.
- Default taken: (A). `desktop/web/README.md`'s engine-baseline section and `docs/conventions.md`
  name it, `desktop/web/tsconfig.json` sets `target`/`lib` to `ES2024`, and the `frontend-baseline`
  gate pins both values and the README, so a target cannot drift from the docs. Reversal: change the
  documented values, the tsconfig and the gate together.
- Confirmed (2026-11-24, P9): every bench record of the goal names the same engines — the Windows
  job `Edg/157.0.0.0`, the macOS job the frozen `AppleWebKit/605.1.15`, the ubuntu smoke
  `Version/60.5 Safari/605.1.15` (run 45338748380) — and the final tree's gate run 45336617606
  reads the `frontend baseline` line green on all three OSes: `ES2024 target/lib pinned, README
  carries both, all 4 allowlist entries fire on their own side; 6 vendor token(s) in the source, 6
  in the bundle`.
- Revisit when: a first-tier platform's floor moves (a new macOS minimum, the app's Windows floor), or
  an engine's build in the matrix stops supporting a feature the frontend uses.

### Q-0019-02 — How is performance measured, before and after?
- Status: decided(default taken)
- Context: Done-when 4 needs launch time, the four main interactions, the translucent layers'
  rendering cost and the bundle size, measured before and after without comparing numbers across
  machines or runs. Hidden web-view windows throttle timers (WebKit also suspends animations), so
  rendering cost needs a shown window, which no gate has ever used; the app's own
  `Window.CapturePage()` (WebView2 `CapturePreview`, WKWebView `takeSnapshot`) avoids an OS-level
  capture and the macOS screen-recording permission.
- Options considered: (A) one `bun`-run tool (`harness/tools/frontend-bench.ts` + its workflow) that
  measures A and B serially in one job, in shown windows, on both first-tier OSes; (B) numbers from
  separate runs or machines; (C) a proxy measured in the hidden windows the gates use.
- Default taken: (A). 2 warm-ups + 9 samples per build per metric, A and B alternated serially in one
  job, median and IQR recorded, "worse beyond noise" = the head's Q1 above the base's Q3 **and** a
  median gap of at least the metric's floor; the same-build control pair is the noise floor. Every
  number in `docs/evidence/0019-modern-frontend.md` is quoted from a named run's artifact. Reversal:
  change the tool and the workflow together and re-run the control pair.
- Owner decision (2026-11-22, after run 45082668207): the floor uses the control pair's measured
  noise. For each metric that has control-pair samples in the run, the floor is
  `max(5 ms, 5 % of the base median, the larger of the two control-pair sides' IQRs (q3 - q1))`; a
  metric without control samples (the bundle bytes) keeps `max(5 %, 5 ms)`. Every verdict in the
  record and the log names its floor and which term set it (`5ms`, `5%` or `control IQR`) — the
  record through `metrics[].verdict.floor`/`.floorSource`, the log through the metric lines. The
  same-build control pair's own verdict keeps the fixed `max(5 %, 5 ms)` floor: there is no second
  control pair to read noise from, and a control pair that reads WORSE must still make the run
  unreadable. The run that forced it: P2's head (`82226bf`) had a byte-identical bundle to A
  (`71c366b`) yet macOS read frames.p95 as WORSE (median gap 24.8 ms against the old 5 ms floor)
  while the same job's control pair had measured frames.p95 IQRs of 17.85 ms and 25 ms — a false
  positive the new floor removes (25 ms, so the gap reads within).
- Owner input (2026-11-22; recorded 2026-11-24, P9): after choosing the control-IQR floor, the owner
  said the macOS tests may be somewhat more lenient.
- Disclosed limit (owner decision, 2026-11-22): on a noisy runner a regression whose median gap is
  smaller than the control pair's IQR is not detectable (the head's Q1 above the base's Q3 still
  gates every verdict separately, but a gap below the floor can never read WORSE). The macOS
  frames.p95 control IQR of 25 ms in run 45082668207 is the example: a real regression of up to
  25 ms on that metric would read ok there.
- Owner decision to take (2026-11-24, P9): the final run 45338748380 ran twice. Attempt 1 failed on
  macOS `frames.longest` (`A` median 81 -> `B` 107 ms, control 95/86, floor 23 = the control pair's
  IQR; `q1 106 vs base q3 88 (above)`); attempt 2 read the same metric 146 -> 154 within its 71 ms
  floor, and both attempts' Windows jobs read `frames.p95` 15.7 -> 15.7. Across the
  goal-wide/bisect dispatch runs, macOS `frames.longest` reads `B`'s median above `A`'s in 8 of 12
  rows, everything inside its floor except that attempt; the same-build control pairs read medians of
  86–201 ms and set floors of 23–121 ms on this metric. The owner's remark above (the macOS tests may
  be somewhat more lenient) is about how much weight a macOS reading gets; it does not by itself accept
  this WORSE, so the attempt-1 reading stands as a disclosed, unresolved observation and Done-when 4's
  macOS `frames.longest` item was open for the owner to accept or have investigated (the P9 evidence
  record carries the per-run table).
- Owner decision (2026-11-24): the owner accepted the final run's macOS `frames.longest` reading
  (attempt 1 WORSE, attempt 2 ok) as a disclosed observation, not to be investigated further; goal
  0019 closes on it.
- Revisit when: the same-build control pair itself reads "worse", or a metric's spread swamps the
  difference the goal needs (the disclosed limit: a regression below the control pair's IQR cannot
  be detected).

### Q-0019-03 — What happens when a runner cannot show a window?
- Status: decided(default taken)
- Context: the bench's rendering-cost metric needs a genuinely visible window (Q-0019-02). A shared
  runner may refuse or throttle one (macOS most likely); WebView2 cannot be canvassed this way — a
  hidden window there reports `visible` and keeps ticking — so the guarantee that no number is
  measured hidden is structural: every measured launch records the visibility it requested, and the
  record's shape check fails a launch that requested the hidden window.
- Options considered: (A) record the guard failure, measure nothing for that metric, and let the
  owner decide; (B) accept a disclosed proxy (style/layout cost measured hidden, compositing
  excluded); (C) change the contract's Done-when 4.
- Default taken: (A). The run records the failure and the metric has no honest value until the owner
  answers. No proxy is invented silently.
- Outcome (2026-11-24, P9): no run needed this. Every bench record reads `guards: 64 checked, 0
  failed, not measured 1` per OS job (the not-measured launch is the hidden prepare), the guard
  ticks fit their budget (the final run: `ticks 30..30, elapsed 444..530 ms` on Windows, `460..1087
  ms` on macOS), and the ubuntu smoke reads `6 checked, 0 failed`. The decision stays unused.
- Revisit when: a bench run's guard rows show a runner that cannot hold a visible window.

### Q-0019-04 — Is there a screenshot companion to the ux gate?
- Status: decided(default taken)
- Context: there is no pixel or screenshot comparison anywhere in the harness, and Q-0012-01 says
  screenshots alone are not evidence while naming "the owner wants pixel-level screenshot diffs" as
  the revisit trigger. The cheapest honest way to show "nothing visible changed" is the bench's PNG
  captures: same runner, base vs head, byte equality.
- Options considered: (A) PNG captures through the smoke hook, base vs head from the same runner,
  with the same-build control pair required identical first, and `harness/frontend-visual-allow.json`
  naming intended differences (a stale entry fails); (B) no visual check; (C) screenshots as the
  primary oracle.
- Default taken: (A) as a **companion** to `desktop-ux`, never a substitute (Q-0012-01): a screenshot
  pair never excuses an audit finding, and the ux gate stays the primary oracle. Reversal: delete the
  captures from the bench tool and the allowlist with it.
- Outcome (2026-11-24, P9): the companion held through the goal. Every step run's and both final
  attempts' A/B shot pair and same-build control pair read `6/6 state(s) byte-identical` (the P1 run's
  A predates the hook, so its A/B pair is `not-measured` while its control pair reads 6/6), with one
  exception: the rejected `will-change` candidate's Windows pair read `5/6` (run 45330890948:
  `state sheet-open differs from the base and no allowlist entry names it`; macOS read 6/6 on the same
  candidate). `harness/frontend-visual-allow.json` stayed empty. The goal's one intended stacking
  difference (the toast above a modal, P5) is outside the bench's six states and is recorded in the
  plan's amendments 14–15, not as an allowlist entry.
- Revisit when: the captures prove nondeterministic on a runner (the same-build control pair differs
  per state), or the owner decides image diffs need a tolerance instead of byte equality.

### Q-0019-05 — Which modern features are left out, and why?
- Status: decided(default taken)
- Context: Done-when 3 wants every adoption and every refusal disclosed. Anchor positioning and
  `<dialog>`-free focus trapping for the popovers would need a Linux run to prove the WebKitGTK
  behaviour; `@scope`, scroll-driven animations, view transitions and `content-visibility` have no
  hand-built part they would replace, or would change a behaviour the goal forbids.
- Options considered: (A) adopt only what removes or simplifies hand-built code on the baseline, and
  list each left-out candidate with its reason; (B) adopt anchor positioning and the rest
  optimistically; (C) adopt nothing modern.
- Default taken: (A). The evidence record lists each adopted feature with what it removed and each
  left-out candidate with its reason; a left-out candidate is revisited only where the plan names
  its evidence path (a Linux gate run for popover anchor positioning).
- Final left-out list (2026-11-24, P9), each with the evidence it rests on: the coach pop as a
  popover (reverted — the manual popover's box-shadow painted over the orb; plan amendment 15);
  native light dismiss as the only outside-click path for the mark popover (native dismissal does
  not clear `pendingRange`, and `openAfterSyntheticOutsideClick=true` shows no gate can drive the
  engine path; amendment 15); the backdrop as a transition or `transition-behavior: allow-discrete`
  (the engines run the backdrop animation but do not name the `::backdrop` pseudo in
  `getAnimations()`; amendment 16(e)); `@starting-style` for the sheet's slide (the restored
  `sheetin` keyframe is the base's own entry and the measured frame step went back to the base's
  level; amendment 19); container queries (the compact mode's gutter change makes the stage's
  content width non-monotone at the 1340 boundary — 1292.00 px at 1340 vs 1261.33 px at 1341,
  1330 and 1362 both present 1282.00 px — and with the coach column the stage is 952 px at 1440;
  amendment 18); anchor positioning and `<dialog>`-free focus trapping for the popovers (no Linux
  proof on WebKitGTK 2.52.6); the dock tooltip (no gain); `@scope`, scroll-driven animations, view
  transitions and `content-visibility` (no hand-built part they would replace, or a behaviour the
  goal forbids).
- Revisit when: a first-tier or second-tier engine version change makes a left-out feature safe, or an
  adopted feature's removal (P4–P8) is reverted with the finding that caused it.

### Q-0019-06 — What WebKit-only constructs stay?
- Status: decided(default taken)
- Context: the workaround removal (P3) must leave the constructs the WebKit engines genuinely need:
  `-webkit-font-smoothing` has no standard equivalent that keeps the design's text weight, and the
  `::-webkit-scrollbar*` family is WebKit's own API with no equivalent for the design's 8 px thumb.
- Options considered: (A) keep them, as allowlist entries in the `frontend-baseline` gate, each with
  its reason, and fail on an entry that matches nothing; (B) remove them anyway; (C) keep them with no
  machine check.
- Default taken: (A). The gate's allowlist carries the reasons, an entry that matches nothing in the
  source fails the gate (every entry is source-side, so the built scan has nothing to fire: on the
  bundle side the guarantee is the violation direction — a vendor construct no entry covers is a
  violation), and no new dependency is added for the visuals: the shot comparison is byte equality, not
  an image library. Reversal: change the allowlist and the stylesheets together.
- Outcome (2026-11-24, P9): the six tokens are the final tree's own (`base.css:8` and `:34-37`;
  re-run `git grep -oE -- "-webkit-[a-z-]*|-moz-[a-z-]*|-ms-[a-z-]*" bb94a9d -- desktop/web/styles`
  → 6), and the final tree's gate run 45336617606 reads `all 4 allowlist entries fire on their own
  side; 6 vendor token(s) in the source, 6 in the bundle` on all three OSes.
- Revisit when: a baseline engine gains a standard equivalent that preserves the design, or the
  design changes the text weight or the scrollbar.

### Q-0022-01 — What is the title area's height, content and drag region?
- Status: decided(default taken)
- Context: goal 0022 replaces the system caption with a title area the Stage draws itself. The
  header `header#top.top` spans the window top, already holds the util group and is the top-most
  app element; MyGo's room reaches the page only as four `--mygo-titlebar-*` CSS variables, and on
  Windows the top rows of the page are a separate invisible window's resize band
  (`internal/windows/topedge.go`: `SM_CYFRAME + SM_CXPADDEDBORDER`, 8 DIP where the system reports
  none), so a control moved to the strip's own top would lose its top rows to the resize.
- Options considered: (A) the header itself is the title area, keeps its designed heights (64 px,
  58 px under the `max-height: 820px` query), reserves the insets inside its own outer columns and
  carries `--app-region: drag` under a page marker while any room value is above zero; (B) a
  shorter dedicated band inside the header; (C) leave the header and paint a strip beside it.
- Default taken: (A). `TitleBarHeight: 40` matches the `.util` group's own height; the brand's
  leading padding grows by `--mygo-titlebar-inset-left` and the util group's trailing **margin** by
  `--mygo-titlebar-inset-right` — not `padding-right`, which would grow the capsule's painted glass
  under the caption buttons while the controls stay clear (the `titlebar-room-paint` check and its
  must-fail case pin the margin); so the two `1fr` columns stay equal and the middle slot stays
  centred. The drag marker is set while any of the four values is above zero (a macOS left inset
  alone still needs its clearance), read at boot and kept from `mygo:title-bar` (no replay for a
  late subscriber). Disclosed: the header's controls do not share the caption buttons' vertical
  centre (Windows: group centre y = 29 against the buttons' 20); P3's read and the manual pass judge
  the look. Reversal: drop the switch and the page rules together.
  P5 outcome (2026-11-25): the design held on all three OSes, read from run 45599616722's `desktop`
  jobs (the evidence record's Done-when 1 and 2 sections): no title-area finding and no clipped
  control at 880x600, 1280x800 or 1440x844, and the disclosed vertical-centre consequence stands
  (Windows: the util group's centre y = 29 against the buttons' 20; macOS: the AppKit band is the
  32 px run 45599616722 reads, and the group keeps its own 9..49 band — the plan's measured
  util-group box at 1280x800, header 58 px and group 40 px with its top y = 9; plan lines 154 and
  217-223).
- Revisit when: a room value makes a header control clip (the `titlebar-strip-clip` check), or MyGo
  offers a title-bar layout API the page can use instead of CSS variables.

### Q-0022-02 — Which appearance do the caption buttons follow?
- Status: decided(default taken)
- Context: with a hidden title bar, Windows' buttons default to the **system** appearance
  (`AppsUseLightTheme`) while this app's appearance is its own page theme; macOS's lights keep the
  app's appearance on their own (`mygo@v0.10.0/window.go:1431-1434`), while GTK's title buttons
  follow the **desktop's** GTK theme — `Window.SetWindowButtonsTheme` is a no-op on Linux
  (`mygo@v0.10.0/internal/linux/window.go:947-949`) and the app never set `Theme.SetSource`, the
  other way GTK's appearance follows the app's — so a light desktop theme would put dark glyphs on
  the app's dark page (found in the P4 round 2 review, 2026-11-25).
- Options considered: (A) the app tells MyGo which appearance it wants, on Windows, through
  `Window.SetWindowButtonsTheme`; (B) accept the system appearance; (C) a page-drawn caption (out of
  scope, Q-0022-09).
- Default taken: (A). The page's theme lives in `localStorage`, so the page tells Go: one new bound
  method (bound like `Study`), its regenerated client entry and its `desktop/web/src/api/client.ts`
  wrapper, called at boot and on the theme toggle. Disclosed: this adds one method to the app's
  bound surface; `window.mygo.window`'s behaviour does not change. Reversal: drop the call and
  accept system-appearance buttons, legible only while the system and app appearances agree.
  Amended 2026-11-25 in the P4 round 2: on Linux the same bound method also applies a
  `Theme.SetGTKCSS` sheet that draws the GTK title buttons from the page's appearance — the ink of
  `desktop/web/styles/tokens.css`'s two themes, with hover and active wells of the same ink — so a
  desktop theme of the other appearance cannot make a button invisible; the boot scenario calls it
  on Ubuntu, where a sheet GTK rejects fails the gate.
  P5 outcome (2026-11-25): shipped and read from artifacts — the boot scenario's button-sheet call is
  green in all three `desktop` jobs of run 45599616722 (GTK parsed both sheets on Ubuntu), and the
  manual pass's OS-level captures read dark glyphs on the light page and light glyphs on the dark
  page, with the two-square restore glyph after a maximize. The inactive-window palette is an owner
  action (no input desktop on this machine; the evidence record's manual-pass section).
- Revisit when: MyGo gives the page a way to set the buttons' theme, or the caption stops being
  MyGo's own drawing.

### Q-0022-03 — Which macOS hidden-title-bar style?
- Status: resolved
- Resolution: (owner, 2026-11-25) option (A) confirmed by the owner: the traffic lights stay at the
  top left where macOS puts them for every app; no `TrafficLightPosition` and no `hiddenInset`. P5's
  outcome: the macOS `desktop` job of run 45599616722 reads the hidden room `32/69/0/23` with the
  marker true (the shown `32/69/0/23` is run 45596813256's shot rows), and the evidence record
  carries the owner actions for the Mac-only manual items.
- Context: `TitleBarHiddenInset` adds an `NSToolbar` and changes the window's corner radius;
  `TitleBarHeight` plays no part on macOS, where the room is AppKit's own title-bar band (~28 pt on
  a standard window — the formula and the full-size content view are the sources, P3 reads the
  value); a double-click's action is the user's `AppleActionOnDoubleClick` (default: Zoom).
- Options considered: (A) `TitleBarHidden`, no `TrafficLightPosition`, the page reads
  `--mygo-titlebar-inset-left`/`--mygo-titlebar-controls-bottom`; (B) `hiddenInset`; (C) move the
  lights.
- Default taken: (A): the lights stay where macOS puts them for every app, and Q-0022-01's vertical
  rule (the header at its own designed height) is what keeps the util group whole under a band
  shorter than 40 px. The double-click assertion states the zoom path and names the default as its
  first suspect on failure. Reversal: switch styles and re-read the room.
- Revisit when: the macOS minimum moves or AppKit's band stops matching the plan's formula.

### Q-0022-04 — What do Linux decorations do?
- Status: decided(default taken)
- Context: Linux hides the title bar too (Done-when 1 asks for all three platforms). A Wayland
  compositor that enforces server-side decorations keeps its own bar with no room for the page
  (`xdg-decoration`: "The compositor can decide not to use the client's mode"); GNOME/X11 and
  Mutter's CSD give GTK's buttons in MyGo's overlay. CI has no window manager: `gtk_window_maximize`
  is the WM's business and its state must not be assumed ("or at all", GTK's own docs).
- Options considered: (A) hide on Linux and accept the compositor's own bar where one is enforced;
  (B) keep GTK's bar outside Wayland; (C) assert the maximize state on CI.
- Default taken: (A). Disclosed: under an enforcing compositor the page's title area sits below a
  compositor-drawn bar and reserves nothing; the CI case asserts only the press's takeover (what
  the page can observe) and records the maximize reading; the Ubuntu artifact carries the hidden
  window's room, and the effect on a real desktop is the manual pass. Reversal: drop the Linux arm
  if no first-tier Linux appears.
  Amended 2026-11-25 in the P4 round 2: on Linux that room's `height` is the app's requested
  `TitleBarHeight` (mygo `internal/linux/titlebar.go:123`), so whether GTK keeps buttons is read
  from `left`/`right` above zero, not from a zero height, which no non-fullscreen Linux window
  reports; the two Ubuntu jobs log the effective `gtk-decoration-layout` beside the readings.
  P5 outcome (2026-11-25): the two Ubuntu jobs — run 45599616722's `desktop` job (the hidden room
  `40/0/207/40`, marker true) and run 45596813256's `linux-smoke` job (the shown room) — read the
  same value and both log the runner's GNOME key `'appmenu:close'`; the `desktop` job recorded the
  maximize reading `false -> true -> false` without asserting it. The Wayland/SSD disclosure stands,
  and the real-desktop behaviour is an owner action (the evidence record).
- Revisit when: a first-tier OS gains a Linux desktop in the matrix, or a compositor starts
  reporting the room.

### Q-0022-05 — What does the machine own, and what does the manual pass own?
- Status: decided(default taken)
- Context: Done-when 1's six cells (three OSes × two appearances, the today view at 1280x800) can be
  mechanical — `today` and `today-dark` in each of the Windows and macOS bench jobs and the same two
  states from the Linux render-smoke job — while a page capture cannot show the frame or the native
  buttons (`Window.CapturePage()` captures the visible page only).
- Options considered: (A) the six cells are bench shots and the manual pass carries only what a page
  capture cannot show; (B) manual pass for everything; (C) OS-level captures in CI.
- Default taken: (A). The manual pass records the OS, the appearance and what was seen: the frame and
  the buttons' legibility over the drawn title area, Snap layouts, a real pointer's drag and
  double-click, the resize from the top edge and both sides, the inactive and full-screen looks,
  and the right-click. On Windows it is a coordinator action; on macOS and Linux an owner action
  where a machine of that OS is available, recorded with the reason where none is.
  P5 outcome (2026-11-25): the six cells are closed from run 45596813256's artifacts (paths and
  sha256 in the evidence record), and the manual pass on this machine produced OS-level captures of
  the light, dark, maximized, full-screen and restored window but could not drive the real pointer
  (`SendInput` returns 0 with `ERROR_ACCESS_DENIED` and no `mousedown` reaches the page; the record
  carries the evidence) — the Snap hover, the real double-click/drag/top-edge resize,
  minimize/restore, the inactive look and the right-click are owner actions, listed there.
- Revisit when: a gate can capture the frame or drive a real pointer.

### Q-0022-06 — How are per-OS visual differences allowlisted?
- Status: decided(default taken)
- Context: the bench compares A's and B's shots byte-for-byte and fails a stale entry; a step that
  hides the title bar on Windows only changes Windows' page shots, so a shared allowlist entry would
  read as stale in the macOS job (which compares its own shots).
- Options considered: (A) entries gain an optional `os` (`"windows" | "macos" | "any"`, default
  `"any"`; the Linux `--smoke` job compares no numbers); (B) separate allowlists per job; (C) an
  entry that carries the other OS's name in its reason.
- Default taken: (A). An entry belongs to one run's A/B pair, never to the tree in general: it is
  added in the branch whose own run needs it and removed by the next step, so the merged tree's
  allowlist is empty again. The goal-level comparison runs from a throwaway branch that carries the
  head plus its own entries (fourteen for the seven states, each with its reason); that branch is
  never merged. Disclosed cost: between a step's merge and the next step's removal, another branch's
  bench run reads those entries in its own head and fails as stale — the coordinator serialises the
  steps so no other branch's bench run starts inside that window.
  P5 outcome (2026-11-25): the runs behaved as designed — P2's seven `os: "windows"` entries, P3's
  seven `os: "macos"` entries scoped to `branch: "goal/0022-p3"`, and P4's tree carrying only 0021
  P2's branch-scoped entry (read from the branch heads and the runs' own `shots.allowed` rows). The
  goal-level comparison ran from a throwaway branch carrying the final head plus fourteen entries
  without `branch` (`A = 4913932` → `B = 0a5b688`, run 45610151815): both OSes' seven states differ
  and are allowed, control 7/7, every metric `worse=false`, guards 68/0/1 — the recipe and numbers
  are in the evidence record.
- Revisit when: the bench compares shots on Linux too, or the comparison stops failing stale
  entries.

### Q-0022-07 — What happens to the 0012 contact sheets?
- Status: decided(default taken)
- Context: the 17 sheets under `docs/evidence/0012/` are the four-appearance reference shots of the
  page area (1280x800, downscaled 640x400, MyGo's automation build); nothing in the repository
  regenerates them, and every sheet shows the page's own top bar, not the frame.
- Options considered: (A) regenerate them for the final head with their own record's method and list
  per sheet what changed; (B) leave them and disclose the age; (C) regenerate only if the build
  reproduces.
- Default taken: (A), with (C) as the fallback: if that build cannot be reproduced, the evidence
  record says so and regenerating the sheets becomes an owner action — the six per-OS cells keep
  their own mechanical artifacts, so the fallback does not narrow Done-when 1; neither the sheets
  nor their README claim a state they do not show.
  P5 outcome (2026-11-25): the fallback is invoked — the sheets' method is MyGo's automation (e2e)
  build plus the state walk and 2x2 composer that goal 0012's record documents and this repository
  does not carry (`design/` holds prototypes only and was not touched), so the record says so and
  regenerating the 17 sheets for the final head is an owner/coordinator action; the six mechanical
  cells keep Done-when 1, and the sheets' README carries a dated note (added 2026-11-25) that they
  still show `354f08c`. `README.md` and `README.zh-CN.md` embed the same pre-0022
  `feedback-coach.webp`, stale with the sheets, and are replaced when the sheets are regenerated.
- Revisit when: the automation build stops reproducing, or the sheets' method changes.

### Q-0022-08 — Which framework gaps shape the workarounds?
- Status: decided(default taken)
- Context: MyGo offers no page-visible title getter, no replay of `mygo:title-bar` for a subscriber
  that registers after document start, no room API besides the CSS variables, and `CapturePage()`
  cannot show the native buttons or the frame; `TitleBarHeight` does nothing on macOS.
- Options considered: (A) work with the frame's own surface: read computed styles, subscribe for
  changes, add one smoke-only bound service for the window title, judge the buttons in the manual
  pass; (B) reach into MyGo internals; (C) widen the framework first.
- Default taken: (A). The smoke-only `SmokeWindow.Title` reads the OS title on Windows
  (`GetWindowTextW` through the window's HWND — MyGo's own `Window.Title` returns a cached string)
  and, on macOS and Linux, MyGo's cached title, the only read without a cgo or Objective-C binding
  this project does not carry — recorded as a disclosed limit in `docs/harness.md` row 24 and the
  evidence record. Each workaround is revisited when MyGo offers the missing capability.
  P5 outcome (2026-11-25): every workaround held on the final tree — the room is read from computed
  styles at boot (the checks' 192/192 audits per OS in run 45599616722), one bound method remains the
  only new surface, and the macOS/Linux cached-title read-back is carried in `docs/harness.md` row 24
  and the evidence record.
- Revisit when: MyGo adds a title getter, an event replay, a room API beyond the variables, or a
  frame capture.

### Q-0022-09 — Are the Windows caption buttons still the system's?
- Status: decided(default taken)
- Context: the contract keeps the system's own window buttons unless an OS cannot show them legibly
  over the page, and requires any replacement to behave like the system's, Snap layouts included,
  disclosed. With `TitleBarHidden`, Windows has no system caption left: MyGo's backend draws
  minimize, maximize-or-restore and close itself in a layered child window over the page, answers
  `WM_NCHITTEST` with the buttons' hit-test codes (which is what brings Snap layouts over maximize),
  and paints Windows 11's state palette in the system appearance unless the app asks otherwise
  (Q-0022-02).
- Options considered: (A) accept MyGo's drawing as the disclosed replacement; (B) a page-drawn
  caption answering its own hit test; (C) keep the system caption on Windows only.
- Default taken: (A), recorded as the disclosed replacement of Done-when 1's "system buttons".
  Verification: the manual pass's Snap hover and OS-level captures of the buttons (Q-0022-05), the
  per-OS room reading, and the guard that the window never sets `HideWindowButtons`. Reversal: none
  inside MyGo.
  P5 outcome (2026-11-25): the room and the OS-level captures confirm MyGo's drawing (light/dark
  glyphs, the restore glyph maximized, the caption strip's `inset-right` 138 px); the Snap flyout
  hover itself could not be driven on this machine (no input desktop) and is an owner action — the
  assumption stays unverified until the owner hovers the button.
- Revisit when: MyGo draws the caption differently, or Windows 11's Snap rules change.

### Q-0020-01 — One resolution, shared by both entry points
- Status: decided(default taken)
- Context: the command line resolves its default database as the relative `.passquest/passquest.sqlite`
  under the working directory (`internal/study.DatabasePath`), while the window resolves the per-user
  data directory (`%APPDATA%\Pass Quest`, `~/Library/Application Support/Pass Quest`,
  `$XDG_CONFIG_HOME/Pass Quest`) in `internal/desktop.resolveStorage`. Done-when 1 requires one
  computation both entry points share; `--db` and `PASSQUEST_DB` keep winning exactly as today, and the
  window's location must not move.
- Options considered: (A) a new `internal/userdata` package owns the directory and the path, and both
  roots call it; (B) `internal/desktop` exports the resolution and the CLI imports it; (C) each keeps
  its own spelling with a parity test between them.
- Default taken: (A). `internal/userdata` owns `AppName = "Pass Quest"`, `DataEnv =
  "PASSQUEST_DESKTOP_DATA"`, `DatabaseEnv = "PASSQUEST_DB"`, `DatabaseFile = "passquest.sqlite"`,
  `LegacyDatabasePath = .passquest/passquest.sqlite`, `Dir(configDir, getenv) (string, error)` (the
  `DataEnv` override — whitespace-trimmed and taken only when non-empty, the rule the window applies —
  else `configDir()` joined with `AppName`) and `DatabasePath(flag, configDir, getenv) (string, error)`
  (the flag as given when it is not whitespace-only → the trimmed `DatabaseEnv`, exactly
  `study.DatabasePath`'s rule today → `DatabaseFile` inside `Dir`). Both reads trim whitespace and the
  flag is returned untrimmed, so a whitespace-only `PASSQUEST_DESKTOP_DATA` still resolves the
  app-owned per-user default — never a directory named from a space, as the window ignores the blank
  value today. `internal/study.DatabasePath` and the window's default seam both call it;
  `internal/desktop` keeps its own literals for the four names the package shares (its `AppName` is
  where packaging's identity lives and what the install-guide gate reads) with a parity test pinning
  all four against `internal/userdata`'s values, so a drift in any of them fails before a run.
  `configDir` is a seam, not a second implementation: both roots pass `os.UserConfigDir`, so the
  directory stays `os.UserConfigDir()`'s own answer and the window's location cannot move. The CLI
  honors `PASSQUEST_DESKTOP_DATA` for the same reason the window does — it *is* the window's
  data-directory override, and the smoke gates isolate runs with it — with `--db` and `PASSQUEST_DB`
  still deciding first. A `PASSQUEST_DESKTOP_DATA` directory is the learner's explicit choice, taken as
  given exactly as the window takes it (only the app-owned default is tightened to 0700), so the CLI
  leaves it alone too; only the app-owned per-user default keeps `store.OpenPrivate`'s 0700 tightening.
  When no config directory can be resolved and no override is set, the command fails with one clear
  message naming `PASSQUEST_DB`/`--db` rather than falling back anywhere.
- Outcome (2026-11-25, P1): as planned. `internal/userdata` holds the names and the two functions;
  `internal/cli` resolves through `Options.resolveDatabase` (one helper `ports` and `openDatabase`
  share) and renders an unresolvable config directory as the refusal `config_dir_unavailable`
  (`exit 2`) naming `--db`/`PASSQUEST_DB`; the desktop's `openStudy` seam calls `userdata.Dir` with
  `os.UserConfigDir`. The one exception is `agent record-step`, which keeps its own stdout error
  document (`config_dir_unavailable`, exit 1) because that document is the durable host's protocol.
  The parity test is `TestUserdataParity` (`internal/desktop/paths_test.go`).
- Outcome (2026-11-25, P1 fix round): `study.DefaultDatabasePath` takes only the config-directory
  seam in the final shape — it returns the clean app-owned default (no `PASSQUEST_DB`, no
  `PASSQUEST_DESKTOP_DATA`), the path a copied command with no overrides resolves, and that is what
  the generated-command comparison reads (Q-0020-06) — and the two `config_dir_unavailable` messages
  are built by one shared `study.ConfigDirUnavailableMessage`.
- Revisit when: a third entry point needs the directory, or `os.UserConfigDir` stops naming the
  per-user root the guides print.

### Q-0020-02 — The notice about the old default
- Status: decided(default taken)
- Context: goal 0020 moves the command line's default database to the per-user directory. A learner
  whose data still sits in `./.passquest/passquest.sqlite` must not silently lose sight of it, and the
  contract forbids moving, copying or merging it (Done-when 2).
- Options considered: (A) one stderr notice per run naming both paths and a move command quoted for the
  detected shell family; (B) print no notice and let the guides carry the fact; (C) offer an interactive
  move prompt.
- Default taken: (A). On a run that resolves the default location (no `--db`, no `PASSQUEST_DB`), when a
  database file exists at `filepath.Join(workdir, ".passquest", "passquest.sqlite")`, the CLI writes
  **one** line to stderr, before the command's own work, naming the old path, the new path and the
  promise `Pass Quest will not move it; move or merge it yourself`, whose move command names the
  resolved **absolute** paths (the working directory's old file and the per-user database — the same
  pair the prose prints, never a `%APPDATA%`/`$env:` spelling) with each argument rendered through
  `cmdline`'s shared `CommandQuoter.Quote` for the family `cmdline.DetectShell` reports — the family,
  its source and the override hint printed beside it, exactly as `next` does, through one helper both
  callers share. When detection fails (an unknown `PASSQUEST_SHELL`) the notice prints the paths and
  the prose without a command and the run continues: an unrelated command must not fail because of the
  notice. Nothing is copied, moved, merged or deleted — the notice is a read of the filesystem plus a
  line on stderr, so `--json` stdout stays byte-identical. It shows whether or not the per-user
  database exists: with both files the learner must still be told which one the command reads, and the
  wording names both paths either way. Reverse: condition it on the per-user database being absent (the
  contract's literal case) and drop the other branch.
- Outcome (2026-11-25, P1): as planned, with two implementation details the notice's tests pin: the
  move verb follows the family (`mv` for sh, `move` for cmd, `Move-Item` for powershell), so the pasted
  command exists in the detected shell; and the notice is suppressed when the old file *is* the
  resolved database (an absolutes compare, case-insensitive on Windows), which would otherwise print a
  command moving a file onto itself.
- Outcome (2026-11-25, P1 fix round): the move command is now printed only when the per-user database
  does not exist yet. In the steady state — both files present — a POSIX `mv` would replace the
  database the window is writing, so with both files the notice names both paths, says no move command
  is printed and to merge instead, and offers no command
  (`TestOldDefaultNoticeWithBothDatabasesPrintsNoMoveCommand`). The command is also omitted when the
  detected family is `cmd` and either path contains `%`, which cmd.exe expands inside the quotes
  (`TestOldDefaultNoticeDropsCmdCommandOnPercentPath`).
- Revisit when: the notice's wording proves confusing in the owner's use, or a later goal migrates the
  old data itself.

### Q-0020-03 — Failing clearly when the wait runs out
- Status: decided(default taken)
- Context: the window opens its store per call and the command line is one process per command, so
  SQLite's own write lock plus `busy_timeout(5000)` is the whole concurrency guarantee (Done-when 3).
  What the learner reads when the wait runs out is undecided.
- Options considered: (A) map the driver's busy error to a typed refusal in one place; (B) let the
  driver's text through; (C) add WAL, a lock file or a retry loop.
- Default taken: (A). No locking change: the window opens per call, the CLI is one process per command,
  and SQLite's write lock plus `busy_timeout(5000)` is the guarantee. `internal/store` gains
  `IsBusy(err) bool` (the driver's `Error.Code()` with `code&0xff` 5 or 6), and `internal/cli`'s
  `internalError` maps a busy error to a usage refusal (exit 2, code `database_busy`) whose message says
  the window has the database open and gives one action (close the window, or retry). One mapping
  point, so a busy error from the open *or* from a later write reads the same. No WAL, no lock file, no
  retry loop, no process-level lock: each would be a piece of state this goal does not need. Reverse:
  drop `IsBusy` and let the driver's text through again.
- Outcome (2026-11-25, P2): as planned. `store.IsBusy` matches the driver's typed result code
  (`errors.As` to `*sqlite.Error`, primary code `SQLITE_BUSY`/`SQLITE_LOCKED`); `internalError`
  renders a busy error as exit 2, code `database_busy`, message `database is busy: the Pass Quest
  window (or another command) has it open; close the window or retry`. The cases live in
  `internal/store/busy_test.go` (typed code vs. lookalike text; two interleaved handles; a hold the
  writer waits out under its 5000 ms wait; one past the timeout with the wait shortened to 300 ms
  in the DSN) and `TestBusyDatabaseRefusal` (`internal/cli/busy_test.go`): the lock held past the
  CLI's own 5000 ms wait ended in the refusal after 5.10 s, and with the mapping removed the same
  command read `internal_error: store: begin append: database is locked (5) (SQLITE_BUSY)` at
  exit 1.
- Revisit when: a busy refusal is seen in the owner's own use, or the store grows a second writer.

### Q-0020-04 — How the package is recognised when a binary runs as the CLI
- Status: decided(default taken)
- Context: the window passes `resourceDir()` (MyGo's `PathResources`) to the host resolution; the CLI's
  composition root passes nothing, so an installed binary started with arguments finds `node` only on
  `PATH` and, on macOS, cannot reach `Contents/Resources/agent` at all (Done-when 4).
- Options considered: (A) the CLI path takes the resources directory from the same computation the
  window uses, exposed by `internal/desktop`; (B) a new heuristic in `internal/hostpaths` sniffing an
  install; (C) keep the harness's `PASSQUEST_NODE`/`PASSQUEST_*_HOST` pins.
- Default taken: (A). The CLI path takes its resources directory from the **same** computation the
  window uses — `mygo.App.Path(mygo.PathResources)`, already in `resourceDir()` — exposed to
  `cmd/passquest-desktop` as `desktop.ResourceDir()` and carried into `cli.Options` as `Resources`,
  which `withDefaults` puts into every default host transport. A development build (`mygo.IsDev()`)
  passes `""`, so a source checkout's CLI keeps today's lookups; an installed binary passes the
  packaged directory on all three platforms, so the resolution's own rules (explicit `PASSQUEST_*`
  first, packaged directory second) decide, and macOS's `Contents/Resources` is reachable.
  `internal/hostpaths` is not changed: no new heuristic, no install sniffing. The standalone
  `cmd/passquest` binary keeps its source-checkout behaviour (it never had resources). Reverse: pass
  `""` again and restore the harness's `PASSQUEST_NODE`/`PASSQUEST_*_HOST` pins in the package smoke.
- Revisit when: a third entry point ships without a resources directory, or MyGo moves the resources
  path.

### Q-0020-05 — The Node 26 pin
- Status: decided(default taken)
- Context: the package bundles Node 22.23.5 (`harness/lib/package.ts` `NODE_VERSION`), `mise.toml`
  pins `node = "22"` and CI's `gate.yml` `node-version: "22"`; Node 22 is in maintenance and reaches
  end of life on 2027-04-30, and the owner's decision (2026-11-22) is Node 26. Measured 2026-11-25
  (`https://nodejs.org/dist/index.json`): the newest 26 LTS is **v26.13.1** (2026-11-10,
  `"lts": "Lithium"`, `"security": true`); 26.13.0 was the first `Lithium` tag (2026-10-28); v27 is
  still alpha. `https://nodejs.org/dist/v26.13.1/SHASUMS256.txt` carries
  `e2edda75bfb089654705cf65a13b58933bfd5f57148e8ebda916f58a2474f1f7  node-v26.13.1-win-x64.zip`,
  `aec2514ea1ceb2ba52455e9ec65986f6749a06dd759e5db2a72e6889668c9735  node-v26.13.1-darwin-arm64.tar.gz`,
  `82bb96078a24ca50b9c9a38f2379e6be6d87171fcd400fc14c8934b3bce96e2f  node-v26.13.1-darwin-x64.tar.gz`,
  `5c063e46bab0fbc1c17cc445d7ff129cbdcd0471be6ed83b6ab864f8143bc864  node-v26.13.1-linux-x64.tar.xz`;
  all four targets the pins cover exist for 26.13.1.
- Options considered: (A) 26.13.1 as above; (B) the newest 26.x whatever it is at implementation time;
  (C) stay on 22 until it leaves maintenance.
- Default taken: (A). **26.13.1** with the four SHA-256s above, in `harness/lib/package.ts`
  `NODE_VERSION`/`NODE_PINS`; `mise.toml` and `.github/workflows/gate.yml` pin the *same exact
  version*, so the development toolchain, the source-host runs in CI and the shipped runtime agree and
  a patch difference cannot hide a defect. The step that lands the pin re-reads
  `https://nodejs.org/dist/index.json` and that version's `SHASUMS256.txt` at implementation time: if
  26 has a newer LTS release by then, it takes that release and records the version and the reason in
  the evidence record instead of copying the four values above. Reversal: any later step may bump every
  version-carrying place together — the pins, `mise.toml`, `gate.yml` and the three pinned sentences
  the agreement check reads; the archive and member names, the download URL's `v<NODE_VERSION>` prefix
  and the staged `node --version` comparison all carry the version too, so a half-done bump fails the
  download, the agreement check or a `node --version` comparison rather than shipping.
- Outcome (2026-11-25, P4): **26.13.1** landed, as planned. The implementation-time re-read found no
  newer 26 LTS (`index.json` still listed v26.13.1 as the newest 26 release, 2026-11-10,
  `Lithium`), so the planned version and the four SHA-256s above were kept — each recomputed from
  the downloaded archive and equal to `SHASUMS256.txt`. The version lives in `NODE_VERSION`,
  `NODE_PINS`, `mise.toml` (`node = "26.13.1"`), `gate.yml` (`node-version: "26.13.1"`) and the two
  guides' pinned-runtime sentence plus this module's own header sentence; `NODE_PIN_SOURCES` /
  `nodePinProblems` in `harness/lib/package.ts` compare all five, the package gate runs the check
  before its first case (over the repository's own files, never `ctx.root`), and the self-test's
  "package whose pinned Node runtime disagrees with two sources that carry it" fails by source name
  while the accepted twin passes. Nothing else about staging changed.
- Revisit when: Node 26 leaves maintenance, a new target OS appears, or the packages' own engine
  requirement rises above 26.

### Q-0020-06 — What the learner reads
- Status: decided(default taken)
- Context: the CLI's usage line names the literal old default, and the two install guides describe the
  window's data directory but not the command line's. Done-when 6 requires the command line's default,
  the installed binary's CLI use and the old-file advice to be stated and consistent.
- Options considered: (A) the usage line names the rule, the guides gain the sentences inside their
  existing data section; (B) the usage line keeps a literal path (wrong on two platforms); (C) a new
  guide section for the command line.
- Default taken: (A). The CLI's usage line names the rule, not a literal path: `Default database:
  $PASSQUEST_DB or passquest.sqlite in the per-user data directory (docs/install.md)`, because the
  literal path differs per platform and the guides already name it. `docs/install.md` and
  `docs/install.zh-CN.md` gain, inside their existing data section, what the command line and the
  installed binary do: where the command line keeps its database by default, that
  `PASSQUEST_DESKTOP_DATA` moves both entry points together, that the installed app's own binary is
  the command line when given arguments (with the per-platform way to reach it), and what to do with
  an old `./.passquest`. `docs/agent.md` states the same resolution for the host lookups. Disclosed
  consequence of one shared resolution: the window's Today command now omits `--db` when the generated
  command resolves the same store (it always printed one before), because the window's own path *is*
  the default now — a copied command still opens the same database, and nothing below the header
  changes. Reverse: restore the literal usage line and the guides' old sentences; the command text
  follows the resolution again if the comparison is dropped.
- Outcome (2026-11-25, P1): the rule sentence and the five product documents landed with P1
  (`docs/find.md`, `docs/notes.md`, `docs/tutor.md`, `docs/source-import.md`, `docs/desktop.md`); the
  guides' data sections and `docs/agent.md` are P3/P5's, as planned. The Today-command consequence is
  pinned by `TestNextCommandOmitsDefaultDatabase` in its new per-user shape.
- Outcome (2026-11-25, P1 fix round): the omission rule the first cut over-applied is corrected — the
  comparison resolves the clean app-owned default (no `--db`, no `PASSQUEST_DB`, no
  `PASSQUEST_DESKTOP_DATA`), so a store selected through any override is repeated in the generated
  command again and a copy pasted without the override cannot silently open another store; the
  omission happens exactly when the selected path *is* that app-owned default — the disclosed change.
  Pinned by `TestNextCommandRepeatsOverrideSelectedDatabase` (command line) and
  `TestWorkspaceTodayDatabaseArgumentFollowsTheSelectedStore` (window).
- Revisit when: the guides' data section moves, or a platform's per-user directory changes.

### Q-0020-07 — What the machine owns now, CI the rest
- Status: decided(default taken)
- Context: macOS and Linux window behaviour cannot be run on this Windows machine, and its window gates
  fail spuriously under load; the package gate costs about eight minutes per run and the self-test
  about twenty-five.
- Options considered: (A) the two new per-OS checks plus cheap local paths here, CI's three-OS matrix
  as the authority; (B) run the full package gate and window gates locally per step; (C) only CI.
- Default taken: (A). The two new per-OS checks are a `desktop-smoke` scenario (`default-data`) and a
  `package-smoke` case (`cli-runtime`), each with a self-test case proving it fails on a deliberately
  broken fixture; both run on all three OSes in CI. Locally this machine proves the Go tests and the
  two gates' cheap paths only: window cases are advisory here and CI's run is the authority. The
  package gate runs once, at the end of the step that changes the package.
- Revisit when: a Linux or macOS machine is available to the owner, or the package gate's cost drops.

### Q-0020-08 — What this goal does not do
- Status: decided(default taken)
- Context: goal 0020 changes where the command line's default database lives and where the installed
  binary finds its runtime. Several adjacent behaviours look related but stay out of scope.
- Options considered: (A) record the exclusions and let a later goal opt in; (B) migrate existing data;
  (C) change the window's own location or the store's locking model with the same PR.
- Default taken: (A). No migration of existing data, no WAL change, no lock file, no change to where
  the window keeps data, no change to `PASSQUEST_DESKTOP_DATA`'s meaning for the window, no change to
  the database schema, the bound window API, the host protocols, scoring, consent or any golden.
  `PASSQUEST_NODE` and the four `PASSQUEST_*_HOST` variables keep winning over everything. Reverse:
  none needed — this entry records what was left alone; a later goal that wants any of these starts
  its own decision.
- Outcome (2026-11-25, P1): held. No schema, consent, bound-API, host-protocol or golden change; the
  notice only reads the filesystem, and P1's diff changes no window behaviour beyond the default seam's
  computation.
- Revisit when: a later goal needs one of the excluded behaviours; it starts its own entry.
||||||| parent of 5852feb (provider: accept OpenAI Responses endpoints beside chat completions (0021 P1))

### Q-0021-01 — Which endpoint paths select which wire format?
- Status: decided(default taken)
- Context: the app speaks one wire format today (OpenAI-compatible chat completions) and the owner
  made Responses support a must; the learner brings one endpoint URL, and a second setting would be
  one more fact to keep consistent with the stored path.
- Options considered: (A) the endpoint path selects the format, with no separate setting;
  (B) an explicit `--format`/settings toggle; (C) a provider-registry field per provider.
- Default taken: (A), the goal's own expected default. One Go derivation
  (`internal/provider`: `FormatChatCompletions`/`FormatResponses`, from the path's final segment
  after `TrimRight("/")`) is the only place the choice is made; it feeds `validateEndpoint`, the
  registry shape check, `tutor.ProbeRequest`/`hostRequestFor`, and the legacy/custom row's reported
  shape. The Node hosts derive the same value from `request.endpoint` (the tutor host, which
  validates the Go document) and from the model endpoint (the durable host, which has no Go-built
  body). A path that is neither form is refused with a message naming both, in the Go validator,
  both host validators and the CLI flag help. Reversal: one line per site back to the single
  suffix. Disclosed: no separate setting exists to disagree with the path.
- Revisit when: a provider needs a format the path cannot name, or a gateway's path stops matching
  the API it serves.

### Q-0021-02 — What is the tutor's Responses body, field by field?
- Status: decided(default taken)
- Context: the tutor host posts exactly one non-streaming body the Go side builds and validates;
  the Responses API renames the token-limit field, moves the system text and defaults `store` to
  `true` (>= 30 days retention), so every field needs a decision.
- Options considered: (A) the mapping below; (B) keep `messages` and add `instructions` only;
  (C) hand-roll a Responses body in the host instead of validating Go's.
- Default taken: (A): `{"model":…, "instructions":<the leading system text>,
  "input":[{"role":"user","content":[{"type":"input_text","text":…}]}], "max_output_tokens":<the
  configured bound>, "temperature":0, "stream":false, "store":false}` — `instructions` per the
  reference's own words, `input` as an item list (`role` `user`/`assistant` for every non-system
  message, in order; one leading system message today), `max_output_tokens` for `max_tokens`,
  `temperature: 0` and `stream: false` unchanged, and `store: false` because the app has never
  opted into provider-side storage (both hosts agree). The host validates the posted bytes against
  the same mapping, so a Go/host disagreement fails closed before anything is posted. Reversal:
  drop the format branch; a learner then gets the host's `invalid_response` on a responses
  endpoint, never a silent wrong-shaped post. The refine path builds the same two documents from
  its own messages, and the preview readers (`internal/study/notes/refine.go`,
  `internal/workspace/workspace_coach.go`) read the non-nil wire doc, so a responses learner sees
  the content the host validates.
- Revisit when: the API reference changes the field names or the `store` default, or a gateway the
  owner probes rejects one of these fields (Q-0021-08).

### Q-0021-03 — Does the durable path hand-roll a Responses body?
- Status: decided(default taken)
- Context: pi-ai 1.5.2 already ships an OpenAI Responses adapter
  (`openAIResponsesApi()`), and the durable host declares its provider with one streams value
  today; the Responses adapter would otherwise need a hand-written streaming/tool body.
- Options considered: (A) the library's own adapter, selected by `model.api`; (B) hand-roll the
  Responses body; (C) refuse Responses on the durable path until the library offers it.
- Default taken: (A): the host declares the provider `api` as a map
  (`{"openai-completions": openAICompletionsApi(), "openai-responses": openAIResponsesApi()}`) and
  sets `model.api` from the endpoint's path, so the library keeps owning streaming and tool
  plumbing. Disclosed: the durable request therefore differs from the tutor's in *where* the
  system text goes (an `input` item) and in carrying `stream: true`, plus the library's own
  defaults — `store: false`, and no `prompt_cache_key` because the host passes no session id
  (`clampOpenAIPromptCacheKey(undefined)` returns `undefined` and the encoder drops it). The
  recorded bodies are the authority for every field. Reversal: one `api:` line back to the single
  completions adapter.
- Revisit when: the library changes the adapter's request shape, or a hosted gateway rejects the
  library's own fields.

### Q-0021-04 — One registry shape or two, and what must an entry declare?
- Status: decided(default taken)
- Context: `endpoint_shape` is an authored registry field (`EndpointShape =
  "openai-chat-completions"` today) and reaches `model list` and the settings JSON; a build that
  speaks two shapes must decide what a wrong declaration means.
- Options considered: (A) add `openai-responses` and require an entry's declared shape to agree
  with its endpoint's derived format; (B) accept both shapes without the agreement rule; (C) derive
  the shape everywhere and drop the field.
- Default taken: (A). An entry **with** a non-empty `endpoint` must declare the shape that
  endpoint derives; a mismatch is a registry error naming both shapes. An entry **without** an
  endpoint — the built-in `custom` row — keeps its declared value: there is no path to disagree
  with, and the embedded registry must stay valid. The learner-visible shape of a custom or legacy
  selection is derived from the active endpoint (`internal/study/models`), so `model list` and the
  settings document cannot report `openai-chat-completions` for a `/responses` endpoint. No
  previously-valid registry with an endpoint changes meaning: today's validator refuses a
  `/responses` endpoint outright. Reversal: accept both shapes without the agreement rule (a
  one-condition delete). The plan chooses agreement because the field is authored and a wrong value
  shows in `model list` and the settings JSON.
- Revisit when: the registry gains a shape whose endpoint cannot derive it, or the field stops
  being authored.

### Q-0021-05 — How does the tutor read a Responses reply?
- Status: decided(default taken)
- Context: the two formats report text, completion state and usage differently; the tutor host
  must keep one error class and one non-negative usage rule for both.
- Options considered: (A) concatenate every `output_text` part and refuse a non-`completed`
  status; (B) read only `output[0]` and accept text with any status; (C) treat every Responses
  reply as `invalid_response` until the owner probes a real endpoint.
- Default taken: (A). Text = the concatenation of `text` from every `output[].content[]` part of
  type `output_text` across `output[]` items of type `message`, in order; a response whose `status`
  is not `"completed"` (or that yields no text, or has a `refusal` part where text was expected) is
  `invalid_response` with a message naming the state and `incomplete_details.reason` when present
  — the same error class a chat response with no content gets today. Disclosed strictness: a chat
  reply a provider cut off is accepted whenever `choices[0].message.content` is non-empty, while a
  Responses reply whose `status` is not `completed` is refused even when it carries text — the
  Responses API reports the reason, the chat form does not, and the record's difference table
  states the asymmetry. Usage maps `input_tokens` → `prompt_tokens` and `output_tokens` →
  `completion_tokens`, reported only when both are integers >= 0. The 1 MiB body cap, credential
  redaction, `request_sent` and the HTTP-status classes are unchanged. Reversal: fall back to
  `invalid_response` for every responses reply, which loses text the API does return.
- Revisit when: the reference changes the status vocabulary or usage fields, or the owner's probe
  shows a provider whose completed reply carries no `output_text` part.

### Q-0021-06 — What does "the settings text" mean, and what does the page change cost?
- Status: decided(default taken)
- Context: Done-when 6 asks the learner to be told how the app picks a format, the window's
  settings are where a learner configures providers, and the ux language audit fires on bare
  ASCII words inside a Chinese line (`zh-ascii-words`, `harness/lib/ux-page.ts`).
- Options considered: (A) one bilingual line in the Models & providers section;
  (B) only the CLI help and the docs; (C) a settings control per endpoint.
- Default taken: (A), one line in `desktop/web/src/views/settings.ts`
  (`data-testid="endpoint-formats"`): English "Endpoints ending in `/chat/completions` or
  `/responses` both work; the path picks the wire format." and the same fact in Chinese, with both
  suffixes inside `<code translate="no">…</code>` (the `zh-ascii-words` rule). It changes the
  `settings-row` shot, so the adding step's bench run allowlists `settings-row` with its reason on
  both comparison OSes and the next step removes the entry (the 0022 P1 allowlist rule).
  `docs/evidence/0012/settings.webp` is the commit-`354f08c` sheet its README names; regenerating
  it is a coordinator action ordered after 0022's own regeneration so one pass covers both goals'
  page changes. Reversal: delete the line; the CLI help and the docs still tell the truth.
- Revisit when: the settings page is redesigned, or a second page needs the same statement.

### Q-0021-07 — When do the Pi packages move, and what happens while the feed lags?
- Status: decided(default taken)
- Context: upstream 1.6.0 exists (undici security updates; a system-role change for
  OpenAI-compatible non-OpenAI providers) but the corporate feed served 1.5.2 for all four direct
  pins on 2026-11-25, and the goal's own rule says a feed that does not serve 1.6.0 is evidence,
  not a blocker for items 1-4 and 6.
- Options considered: (A) do the Responses work on the pinned 1.5.2 and make the bump its own
  last, feed-gated step; (B) wait for the feed before any Responses work; (C) vendor 1.6.0 by
  hand.
- Default taken: (A): P1-P3 land on 1.5.2 (its Responses adapter is present), P5 re-runs the feed
  check first, over all six `@earendil-works` entries `bun.lock` holds (the four direct pins plus
  `pi-codemode`/`pi-telemetry`), and if the feed serves >= 1.6.0 for all six, moves the four
  direct pins together and re-resolves the lock from the feed (a mixed set is reported as blocked,
  not shipped). While the feed serves 1.5.2, the disclosed default is **item 5 blocked**: no
  package file changes, the feed's version listing and the 404 recorded as the blocked evidence,
  the owner action named (ask the feed owner to publish 1.6.0), and the check left re-runnable.
  Reversal: none needed — the check is a command.
- Revisit when: the feed serves >= 1.6.0 for all six entries, or the feed owner reports it will
  not.

### Q-0021-08 — What do the fakes prove, and what only the owner can?
- Status: decided(default taken)
- Context: every model feature must be shown working over Responses, but a gate may not contact a
  real provider, and a gateway that claims `/v1/responses` can still differ from the reference.
- Options considered: (A) loopback fakes that reproduce each format's request shape, response
  shape and refusals, plus an owner probe for the real-endpoint questions; (B) one fake that
  answers both shapes permissively; (C) skip the probe and claim reference compliance.
- Default taken: (A). Each fake reproduces its format's request shape (the strict key set and
  value mapping the app sends), its response shape (chat: `choices[0].message.content`; responses:
  `output[]` message items and `usage.input_tokens/output_tokens`, plus the SSE event set the
  durable path's library requires) and its refusals (401/403, 429, 400-with-`error.message`,
  200-with-no-text, non-JSON). A real OpenAI or Azure Responses endpoint, and a gateway that
  claims `/v1/responses`, are **not** contacted: a credentials-and-budget probe against one is an
  owner action, first-party text only, and its result is recorded as the owner's evidence or as an
  untested limit. Reversal: none — contacting real providers from a gate is forbidden.
- Revisit when: the owner's probe runs, or a fake's shape stops matching the reference.

