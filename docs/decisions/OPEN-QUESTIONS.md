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
- Status: open
- Context: clause 5 requires one action per failure type, but a learner can be in a state where all configured
  providers are `no_credential` or over budget. The goal does not yet say whether that state is one aggregated
  message or a per-provider list.
- Options considered: (A) one line naming the cheapest recoverable action across all providers; (B) the full
  per-provider table; (C) the aggregated line plus the table behind `--verbose`.
- Default taken: (C) — the short line stays actionable, the table stays inspectable. Revisit when goal 0011
  renders this in the desktop workspace.
- Revisit when: the desktop workspace lands, or a learner reports the aggregate line as unclear.

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
