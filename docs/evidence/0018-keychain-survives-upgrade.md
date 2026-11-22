# 0018 — macOS upgrades keep the learner's Keychain permission: evidence

Contract: [`../goals/0018-keychain-survives-upgrade.md`](../goals/0018-keychain-survives-upgrade.md).
Plan: [`../plans/2026-11-22-0100-0018-keychain-survives-upgrade.md`](../plans/2026-11-22-0100-0018-keychain-survives-upgrade.md).
Experiment: [`.github/workflows/keychain.yml`](../../.github/workflows/keychain.yml) and
[`harness/tools/keychain-experiment.ts`](../../harness/tools/keychain-experiment.ts).
Decisions: Q-0011-03, Q-0013-08, Q-0018-* in [`../decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md).

**State: parked (owner's decision, 2026-11-22; Q-0018-03).** The decisive run is the `keychain`
workflow's run 44994402573 on commit `5dd680b`: four jobs, all green. Its `self-signed` × `login`
job — the keychain a learner's app stores into — is refused by the item's partition list, whose
value (`cdhash:…`) is the storing build's code hash and moves with every build, so a project-owned
self-signed identity cannot meet Done-when 2 and the goal stops at its second stop condition. No
identity was created, no secret exists, and the package and release path are unchanged; both install
guides disclose the one Keychain prompt an upgrade brings, and `install-guide` pins that statement.
Every value below is read from the named artifact's `keychain-record.json` (or the job log named for
a log line) and quoted with its field path; nothing is written from prose, and this document quotes
no runner temp path — the artifacts carry those themselves.

## The runs this record cites

| Run | What it is | Result |
|---|---|---|
| 44994402573 | the decisive run: `arm: [adhoc, self-signed]` × `keychain: [experiment, login]`, one arm against one keychain per `macos-26` job, two packages built per job (A = `3250fb0`, B = `5dd680b`) | four jobs `status: ok`, empty `failures`; 17 assertions in each ad-hoc job, 28 in each self-signed job, `[self-check] 19 of 19` in all four |
| 44986005367 | history: the shape that still imported the throwaway identity into the keychain the app stores into | `self-signed` × `experiment` green (`identity.partitionList.exit: 0`, `read.outcome: read`, `drSame: true`); `self-signed` × `login` red (`partitionList.exit: 1` — the login keychain's password is not available to a run — `read.outcome: not-run`); plan amendment 18 |
| 44983246217 | history: the first four-job matrix run | both ad-hoc jobs green with `blocked` reads; both self-signed jobs stopped at the import check, `security import exited 0 or imported no identity: (no output)` (`SecPKCS12Import` prints nothing); plan amendment 17 |
| 44931358398 | history: the first three-arm single-job shape | the ad-hoc read was `blocked` (a SecurityAgent in 20 of 23 probes) and the arm guard stopped the run before the self-signed arm; the item carried no partition entry; plan amendments 12, 15 |
| 44928776617 | history: the first run | red at the item guard: the ad-hoc item carried no partition entry, which the guard read as a value nobody would want; plan amendment 12 |

Artifacts: each job of run 44994402573 uploaded `keychain-record-<arm>-<keychain>` holding
`keychain-record.json` and `keychain-experiment.log` (30 days); the earlier runs uploaded their own
`keychain-record…` names. The four decisive artifacts were downloaded on 2026-11-22 and every table
below reads them.

Reading the record: each job ran exactly one arm, so the job's arm is `arms[0]`.
`arms[0].read.classification.refusesBy` is computed from the item's dump against the build's
`cdhash:` partition id and its designated requirement (`partition`, `application`, `both`,
`neither` or `unknown`), and `arms[0].read.bInDecryptAcl` says whether the build's designated
requirement matches a `requirement:` in the decrypt entry's application list. `implied: false` on a
designated requirement means `codesign -d -r-` printed the stored form; an ad-hoc signature carries
none and prints the implied one (`implied: true`).

## Done when 1 — the problem is established first

No fix was ever designed: this run is the experiment the goal's first item demands, and it precedes
every change the later steps would have made. Each job built two packages (A from the pull request
base, B from the head), installed A from its `.dmg` into one temp prefix, stored a fresh key
through A's window (the row renders `[data-credential="present"]`, which is itself a read), dumped
the item's ACL, installed B over the same prefix (re-signed in place for an identity arm), launched
B's read script, and classified the outcome under the launch contract's hard timeout.

| Reading | Artifact | Record path | Value |
|---|---|---|---|
| the runner | all four | `platform.swVers`, `platform.machine` | `26.7.2`, `arm64` |
| A's and B's commits | all four | `runs.a.commit`, `runs.b.commit` | `3250fb0edff8be11ebcbc369a6d93d1e8b62aea8`, `5dd680b86de7e58b828f32ab779ae3a5df1fdcd1` |
| that the two builds are different programs | all four | `guards.buildsDiffer`, `guards.aCdhash`, `guards.bCdhash` | `true`; `b9bcd4b4f4cab7f45281383a3b1b0ae7016da185`, `f9ea3de0665c6298db160d2337d12345c9db91a3` |
| the two packages each job built | all four | `runs.a.dmg.sha256`, `runs.b.dmg.sha256` | adhoc-experiment `5cc2150f321276c6a8ea1af4de86cf70dd1f9ff276bb3d2da7f2b5fe87b686ec` / `fb996dba5f71f211abdd787b07288189227d827a3a7a74e2ff583756607ff237`; adhoc-login `2c2823abba3a32426d8b67a69cb6b37ee63f8c22077c07a340ba4ebf69f3e010` / `c6fb47015fd1e0b99704b79f76ac6d133c5f0f8e38e7097b30bbeef535e1d70c`; self-signed-experiment `ea12c52beb492c31795cec316f910645d5e68fe7023030918d4a2fc93dba612f` / `6d696cbad95403539fed03cf2b0470c3053d261308fa8a3c8211d0774532bea8`; self-signed-login `5d53e05f2b5032749395fdd5c3f9528d3aa42da35de029f0f787950935c80084` / `25531b8c9aac32f8ee7230d5c29c5f7e70e750580324856460d17d221eab0e6f` (all four pairs share the name `pass-quest-0.15.0-arm64.dmg`; the digests differ per job because a package build is not byte-reproducible across runner jobs) |
| each arm's fresh key, and that no key byte reached the log | all four | `arms[0].keySha256`; `guards.log` | `c554de46d6665f8a246ac85bd5817aa642c1a1ec490c5fec7e43ed025b222e64` (adhoc × experiment), `5d37a4da30a7857c1a226ae0d93715e211f199130bc6b7f129de7bc9c6ce0477` (adhoc × login), `7d6bbc998388c8f3bacdb3aaaa3b9de3aa8fd816926cb1002647108f199d55a6` (self-signed × experiment), `304dc415c9c79af8a1cc76eecd71b03108d19acf9dee5ad9cc3c8889d2a913e7` (self-signed × login); `{prints: 260, keyBytes: 0}`, `{prints: 313, keyBytes: 0}`, `{prints: 314, keyBytes: 0}`, `{prints: 408, keyBytes: 0}` |
| the ad-hoc builds' designated requirements (the code has none; both print the implied form) | adhoc-experiment, adhoc-login | `arms[0].a.designatedRequirement`, `arms[0].b.designatedRequirement`, `arms[0].a.implied`, `arms[0].b.implied`, `arms[0].drSame` | `cdhash H"b9bcd4b4f4cab7f45281383a3b1b0ae7016da185"`, `cdhash H"f9ea3de0665c6298db160d2337d12345c9db91a3"`, `true`, `true`, `false` — the requirement moves with every build |
| ad-hoc × experiment: what macOS wrote into the item, and what B's read met | keychain-record-adhoc-experiment | `arms[0].itemAfterStore.partitionEntry`, `arms[0].itemAfterStore.partitions`, `arms[0].itemAfterStore.requirements`, `arms[0].read.outcome`, `arms[0].read.bInDecryptAcl`, `arms[0].read.classification.refusesBy`, `arms[0].read.classification.detail` | `false`, `[]`, one entry (`entry: 0`) naming `cdhash H"b9bcd4b4f4cab7f45281383a3b1b0ae7016da185"`; `blocked`; `false`; `application`; "the item carries no partition entry, so no partition list weighs on the read; the decrypt entry's application list is specific requirements that do not match the designated requirement" |
| ad-hoc × login: the item this time carries a partition entry, and B's read still does not complete | keychain-record-adhoc-login | `arms[0].itemAfterStore.partitionEntry`, `arms[0].itemAfterStore.partitions`, `arms[0].read.outcome`, `arms[0].read.bInDecryptAcl`, `arms[0].read.classification.refusesBy`, `arms[0].read.classification.detail` | `true`, `["cdhash:b9bcd4b4f4cab7f45281383a3b1b0ae7016da185"]`; `blocked`; `false`; `both`; "the partition list does not admit cdhash:f9ea3de0665c6298db160d2337d12345c9db91a3; the decrypt entry's application list is specific requirements that do not match the designated requirement" |
| what the two blocked reads met: the prompt path, with the samples | adhoc-experiment, adhoc-login | `arms[0].read.detail`, `arms[0].read.launch.agentScan.probes`, `.matches`, `arms[0].read.securityAgent` | "the window never rendered the provider row inside its own 15 s bound" with a SecurityAgent in 20 of 23 probes (adhoc × experiment) and 20 of 24 (adhoc × login); `found` a SecurityAgent, `killed: true`, a new pid in `left`, `respawned: true` in both — launchd restarts the on-demand daemon |
| the keychain each job's app stored into, restored afterwards | adhoc-experiment, adhoc-login | `keychain.variant`, `keychain.defaultBefore`, `keychain.defaultAfter`, `keychain.deleted`, `keychain.loginCleanup`, `keychain.loginResidue` | `experiment`: the user default was the runner's login keychain (`/Users/runner/Library/Keychains/login.keychain-db`) before and after, the run's keychain deleted, `[]`; `login`: the same login keychain throughout, one synthetic item deleted (`{lookedUp: 0, deleted: 0}`), residue `[]` |

Conclusion: on macOS 26.7.2, an ad-hoc B could not read A's item without a prompt in either
keychain — the premise stands, Done-when 1 holds, and nothing was fixed on the strength of a guess.

## Done when 2 — upgrades keep Keychain access: not achievable without an Apple-issued identity

The same four-job run carries the identity arms. Each identity arm re-signed A and B in place with
the job's own throwaway certificate (the temporary signing keychain, plan amendment 18), so the
designated requirement of both builds names that certificate instead of a code hash.

| Reading | Artifact | Record path | Value |
|---|---|---|---|
| the throwaway identity and that `codesign` could use it | self-signed-experiment, self-signed-login | `arms[0].identity.cn`, `arms[0].identity.sha1`, `arms[0].identity.partitionList.exit`, `arms[0].signingKeychain`, `arms[0].credentialKeychain` | `Pass Quest Keychain Experiment`; `0eebac795e36fbf96b34e2fc17debc0bffedf704` (experiment) and `8ca4ef16d8ae3cfbb64dce49c63241731356e29c` (login — a fresh certificate per job); `0` in both; `temporary`; `experiment` / `login` |
| self-signed × experiment: A's and B's designated requirements, byte-equal, both stored (`implied: false`) | keychain-record-self-signed-experiment | `arms[0].a.designatedRequirement`, `arms[0].b.designatedRequirement`, `arms[0].drSame` | `identifier "com.github.wxxb789.passquest" and certificate root = H"0eebac795e36fbf96b34e2fc17debc0bffedf704"` for both; `true` |
| self-signed × experiment: the read this keychain allowed | keychain-record-self-signed-experiment | `arms[0].itemAfterStore.partitionEntry`, `arms[0].read.outcome`, `arms[0].read.providerCarriedKey`, `arms[0].read.bInDecryptAcl`, `arms[0].read.classification.requirementMatch` | `false` — this keychain's item carries no partition list at all; `read`; `true` (the loopback provider stub received the key bytes); `true`; `exact` |
| self-signed × login: the same byte-equal requirement pair | keychain-record-self-signed-login | `arms[0].a.designatedRequirement`, `arms[0].b.designatedRequirement`, `arms[0].drSame` | `identifier "com.github.wxxb789.passquest" and certificate root = H"8ca4ef16d8ae3cfbb64dce49c63241731356e29c"` for both; `true` |
| self-signed × login: the item's partition list, and the read the requirement could not save | keychain-record-self-signed-login | `arms[0].itemAfterStore.partitionEntry`, `arms[0].itemAfterStore.partitions`, `arms[0].read.outcome`, `arms[0].read.bInDecryptAcl`, `arms[0].read.classification.requirementMatch`, `arms[0].read.classification.refusesBy`, `arms[0].read.classification.detail` | `true`, `["cdhash:073434e460fa6fe9aec4ffa6ff11a417d04bd95e"]`; `blocked`; `true`; `exact`; `partition`; "the partition list does not admit cdhash:20cab049524809e62e5fa69c285639ee16382a84; the decrypt entry's application list is a requirement matching the designated requirement (exact)" |
| the blocked identity read: the same prompt path | keychain-record-self-signed-login | `arms[0].read.detail`, `arms[0].read.launch.agentScan.probes`, `.matches`, `arms[0].read.securityAgent` | "the window never rendered the provider row inside its own 15 s bound" with a SecurityAgent in 20 of 24 probes; `killed: true`, `respawned: true` |
| the login variant's simulated allow: the one write a run cannot make | both login artifacts | `arms[0].allow.skipped` | `security set-generic-password-partition-list … exited 1` — "the login keychain's password is not available to this run, so the partition half of a learner's allow cannot be written here, and the third build measures nothing about an unrecorded allow"; no `third` ran |

Why this settles Done-when 2: the two self-signed builds carry the **same** designated requirement,
and the learner's keychain still refuses the second build. `requirementMatch: exact` says the
decrypt entry's application list admits B — the requirement half of a learner's "Always Allow"
travels across builds under a self-signed certificate, exactly as MyGo documents — but the
partition half does not: macOS 26 wrote `cdhash:<A's code hash>` into the item's partition entry
when A stored it, and the check runs on every read, so it refuses any build whose code hash
differs. The Apple source says why no self-signed identity can change that: only Apple-signed and
team-ID-bearing code (Mac App Store, TestFlight, Developer ID, development) gets a stable
`apple:`/`teamid:` partition, and every other signed process gets `cdhash:` — the value of the
storing build (`securityd/src/clientid.cpp`, tag `Security-61901.120.67`, `:282-286`; plan fact 2;
the plan's fact 2 also quotes the partition write on the storing side and the ACL's validation of
it). The experiment keychain's `read` is not a counterexample: that keychain's item carries **no**
partition entry (`partitionEntry: false`), so only the requirement half weighed on the read;
the learner's login keychain is the one that stores a partition entry, and both login arms show it.

So the plan's second stop condition fired (default 10): Done-when 2 needs an Apple-issued
certificate with a team ID. The owner's decision of 2026-11-22 is to record and park — no identity
is added, and the guides disclose the one prompt an upgrade brings (Done-when 4). Q-0018-02…04
carry the decision, the "not taken" plan defaults (3, 6, 7, 8, 9) and the revisit trigger (a
Developer ID exists → store it as the two secrets, add `project` to the workflow's `arm` matrix,
then run it by `workflow_dispatch`).

## Done when 3 — the private key cannot leak: not applicable, no key was created

This goal created no signing key. The only certificates that ever existed are the two throwaway
identities the decisive run's self-signed jobs generated inside their own `RUNNER_TEMP` (one per
job), imported into a temporary signing keychain the tool creates and deletes in its `finally`;
they were deleted with it in the same run, and nothing in the repository, the package or the release
path carries signing material (the plan's P2–P4, which would have introduced the project identity,
are dropped — Q-0018-02, amendment 19). Cited from the artifacts:

| Reading | Artifact | Record path / log line | Value |
|---|---|---|---|
| the signing keychain, its use and its deletion | both self-signed records | `keychain.signing.created`, `.uses[].restored`, `.deleted`, `.deleteExit` | `created: true`; every `codesign` call's `restored: true` (the user search list went back); `deleted: true`, `deleteExit: 0` |
| the tool's own cleanup lines | both self-signed jobs' `keychain-experiment.log` | the `Run the experiment` step | `[cleanup] deleted the temporary signing keychain … (exit 0); the identity and its key go with it` |
| the workflow's backstop deleted the throwaway files too | both self-signed jobs' job log | the `Delete the throwaway identity files` step (`always()`) | `keychain: deleted …/keychain-throwaway` |
| the login keychain lost the synthetic item | self-signed job, `login` job log | `Run the experiment` step; `keychain.loginCleanup` | `[cleanup] the login keychain: security delete-generic-password … -> exit 0`; `{lookedUp: 0, deleted: 0}` |
| no key byte reached any home the run printed into | all four records | `guards.log.keyBytes`; `[guard]` lines | `0` in all four (260/313/314/408 printed lines scanned); `[guard] no key bytes under … outside the keychain that holds them`, `[guard] no key bytes in the <N> line(s) this tool printed`, `[guard] no signing passphrase in the <N> line(s) this tool printed` |
| the package and release path | this branch's diff | — | no file under `harness/lib/package-build.ts`, `harness/package.ts`, `mygo.json` or `.github/workflows/{gate,release,cask}.yml` changed; the package contents scan and the package smoke's `credential` case are untouched |

## Done when 4 — the learner is told the truth

Both guides' macOS sections now say: after an upgrade — a new download, or `brew upgrade` — macOS
asks once whether Pass Quest may use the model key it saved in your Keychain (enter the login
password, choose **Always Allow**); the key stays in the Keychain; and the prompt exists because the
app is not signed by an Apple-issued identity, so each build looks like a new app to it
([`install.md`](../install.md) and [`install.zh-CN.md`](../install.zh-CN.md), macOS section).
`install-guide` pins the statement in both languages (the FACTS entry "the Keychain prompt one
upgrade brings, the answer to it and why it appears": en `Always Allow` and `not signed by an
Apple-issued identity`, zh `始终允许` and `没有 Apple 签发的签名标识`), and the self-test feeds the
gate a broken pair that drops the answer and the reason (the login password sentence stays) and
requires the gate to fail naming every missing pin — so a later edit that keeps the section talking
about the Keychain but drops what the learner should do, or why the prompt appears, is caught. The
gate pass line: `7 sections, 4 code blocks and 7 screenshot places
agree; 6 links and 5 URLs agree; 6 quoted app strings match their sources; … floor 26.0 from
mygo.json`.

## Known limits of this record

- A runner cannot click a Keychain prompt. A read that did not complete is therefore reported as
  `blocked` only when the sampler saw a `SecurityAgent` while the launch ran (the prompt path), and
  as `failed` (a red run) when it did not; the record carries the samples, never an assumption. All
  three non-`read` outcomes of the decisive run were `blocked` with samples, and each left a
  SecurityAgent that is recorded and killed (launchd respawns it; `read.securityAgent.found/left`,
  `respawned: true`). What the learner would see is the prompt these samples stand for.
- The sampler is one `pgrep` a second, so a `SecurityAgent` that both appears and disappears
  between two probes would be missed; a miss turns the read into `failed`, which is loud, and never
  into `read`.
- A runner cannot write the **partition** half of a real "Always Allow" in the login keychain:
  `security` takes that keychain's password on argv only, and the write's exit 1 is recorded as the
  login arms' `allow.skipped`; no third build C ran in either login arm, so the third-build
  question stays unmeasured on a runner. The read outcome itself — the decisive measurement — is
  unaffected. (In the experiment arm whose read succeeded, no allow ran at all.)
- The experiment keychain the tool creates with an empty password is **not** the learner's login
  keychain: its items carried no partition entry where the login keychain's did
  (`itemAfterStore.partitionEntry` false vs true, in both the ad-hoc and the self-signed pair), so
  the experiment variant understates the checks and its `read` must not be read as real-world
  access. The login variant exists exactly for that difference.
- macOS 27 is not run: no GitHub runner carries it and its Security source is not published; the
  citations are the one published tag `Security-61901.120.67`, and the runner was macOS 26.7.2
  (`platform.swVers` in all four records).
- The `project` arm is untested and not reachable today: the workflow's matrix is `arm: [adhoc,
  self-signed]`, nothing passes `PASSQUEST_MACOS_SIGNING_P12`/`_PASSWORD` to the run step
  (`harness/tools/keychain-experiment.ts` reads them there and skips the arm, by name, when unset),
  so a `workflow_dispatch` repeats the same four jobs and never selects it. It never ran with the
  two secrets (a fork pull request carries none, and no run has dispatched it), and its coordinator
  route exports the `.p12` with `openssl pkcs12 -export`'s default PBE (AES-256/PBKDF2 on OpenSSL
  3+) — the throwaway path's `security import` only passed after the export switched to `-legacy` or
  explicit PBE flags (plan amendment 13), so the re-export must follow that route when the arm first
  runs. The revisit is therefore: with a Developer ID, store it as the two secrets, add `project` to
  the workflow's `arm` matrix, then run it by `workflow_dispatch` (Q-0018-04).
- The log scan covers the text the tool itself printed; the workflow's own `echo` lines (paths only)
  are not part of it, and the record is an artifact, not a published file — the artifacts hold the
  runner's own temp paths, which this document does not quote.
- The dry runs of the P1 rounds (below) prove the orchestration, the record and the guards with
  stubbed `security`/`codesign`/`hdiutil` output; they never prove what a real Keychain answers.

## P1's local rounds (before the decisive run, Windows 11)

The tool and workflow were built and rehearsed in six local rounds; each round's full table stood in
this record before the decisive run (the commits 613af0c…5dd680b carry them). Compressed:

| Round | Change | Checkpoints |
|---|---|---|
| 1 | the first tool, the workflow, the dump parser, the guards, the classifier | 15 self-check cases; two must-fail fixtures (`12 of 13` each with the parser narrowed to the plain form, and with the classifier reading `killed` instead of the samples); typecheck/secrets/docs-links pass |
| 3 | the identity step's PKCS#12 export (Git's OpenSSL 3.5.9; `-legacy` first, explicit PBE flags as the retry, passphrase masked and absent from stdout), the no-partition-entry item, the decrypt-entry comparison | 15 cases; two must-fail fixtures (`14 of 15` each); a full stubbed dry run: exit 0, `status: ok`, 30 assertions |
| 4 | one arm and one keychain per job | 17 cases; two must-fail fixtures (`16 of 17` each); dry runs of all four arm × keychain combinations, including the login keychain whose password is unknown |
| 5 | `security find-identity` without `-v`, signing with the SHA-1 it prints | 18 cases; two must-fail fixtures (`17 of 18` each) |
| 6 | the temporary signing keychain (plan amendment 18) | 19 cases; four must-fail fixtures in the stubbed dry runs, each reproducing the design error it guards (the pre-fix import shape, an unrestored search list, the app's keychain named as the signing one) |

## Local checks of the parking round (2026-11-22, Windows 11)

| Check | Result |
|---|---|
| `mise exec -- bun harness/tools/keychain-experiment.ts --self-check` | PASS — `19 of 19 case(s) passed` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts typecheck.ts typecheck` | PASS — `0 type errors in tsconfig.json, desktop/web/tsconfig.json` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts install-guide.ts installGuide` | PASS — `7 sections, 4 code blocks and 7 screenshot places agree; 6 links and 5 URLs agree; 6 quoted app strings match their sources; the release notes name the tap command and both guides; the cask template renders v0.15.0 with the release's SHA-256, and a synthetic pair, into Casks/pass-quest.rb; floor 26.0 from mygo.json` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts docs-links.ts docsLinks` | PASS — `331 checked, 0 broken` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts secrets.ts secrets` | PASS — `775 files, 0 findings` |
| the new case's failure path, run as the self-test's own case object against a fixture root (`Always Allow` and `not signed by an Apple-issued identity` replaced in the en guide, `始终允许` and `没有 Apple 签发的签名标识` in the zh guide) | PASS — `installGuide` fails with exactly the four lines the case expects: `docs/install.md: the "macOS" section must carry the Keychain prompt one upgrade brings, the answer to it and why it appears: missing "Always Allow"`, the same for `missing "not signed by an Apple-issued identity"`, and the zh pair (`missing "始终允许"`, `missing "没有 Apple 签发的签名标识"`); the fixture found every anchor, so neither replacement slipped |
| the full `self-test` gate | FAIL on this Windows machine, attributed to load, not to this round: all 149 cases ran (15 must pass, counted from the tree) and the only failures were two desktop-ux **window** cases — "did not report `no-focus-indicator` on a stylesheet that removes focus outlines and paints Today's title in its background colour" and "the broken Updates row broke an earlier stage: … timed out waiting for a practice item [route #/today]". No install-guide case failed, so the new Keychain-prompt case holds. Attribution: the two cases read product sources and `harness/lib/ux-page.ts`, none touched by this round, and their failure lines are this machine's documented 100 %-CPU artifacts (`child process(es) outlived the app: …msedgewebview2.exe`, `timed out waiting for a practice item`, seen on base trees); re-running those two cases alone at the same load reproduced both the same way. CI is the authority for window gates. |
| Go checks (`gofmt -l`, `go vet`, `go build`, `go test`) | not run: this round changes no Go file (`harness/`, `.github/` and `docs/` only) |
