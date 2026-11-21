# 0018 — macOS upgrades keep the learner's Keychain permission: evidence

Contract: [`../goals/0018-keychain-survives-upgrade.md`](../goals/0018-keychain-survives-upgrade.md).
Plan: [`../plans/2026-11-22-0100-0018-keychain-survives-upgrade.md`](../plans/2026-11-22-0100-0018-keychain-survives-upgrade.md).
Experiment: [`.github/workflows/keychain.yml`](../../.github/workflows/keychain.yml) and
[`harness/tools/keychain-experiment.ts`](../../harness/tools/keychain-experiment.ts).
Decisions: Q-0011-03, Q-0013-08, Q-0017-* in [`../decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md).

**State: draft.** Every outcome below is a placeholder until the coordinator fills it from the
`keychain-record` artifact of the run this table names. No outcome was written by hand: each
`<pending: …>` names the exact JSON path the value is read from, each `<from run: …>` is a log line
to quote once the run exists, and only the "Local checks in this step" table holds values that
already happened (the P1 implementer run on Windows, which cannot run the macOS experiment at all).

## The run this record cites

| Run | Workflow | What it is | Result |
|---|---|---|---|
| `<pending: run id>` | `keychain` | the P1 experiment on `macos-26`: two packages built (A = the pull request base sha, B = its head), three arms run | `<pending: success, or failure with the arm the record names>` |

Artifact: `keychain-record` — `keychain-record.json` and `keychain-experiment.log`, retained 30 days.

Filling it:

```sh
gh run download <run id> -n keychain-record --dir /tmp/0018
jq '{runs, guards, keychain, status, failures}' /tmp/0018/keychain-record.json
jq '.arms[] | {id, identity, skipped, keySha256, login, a, b, drSame, read, allow, third, failures}' /tmp/0018/keychain-record.json
```

The record's vocabulary, so a value cannot be read as more than it is:

- `arms[i].read.outcome`, classified from what happened and never from a message:
  - `read`: the row showed present and the loopback stub received the key bytes.
  - `refused`: the app ran to completion and its window reported the credential absent or
    unreadable (`detail` quotes the page state and any error bar; the item stays in the dump).
  - `blocked`: the read did not complete — the tool's own watchdog kill, the launch helper's hard
    timeout, or a window that never rendered the row (the script's own 15 s/20 s bounds, or the
    smoke hook's 40 s bound) — **and** the
    `read.launch.agentScan` samples show a `SecurityAgent` process while the launch ran (one
    `pgrep -lf SecurityAgent` probe a second; every probe and every hit is in the record). Nothing
    else counts as blocked.
  - `failed`: anything else, with the failure lines. A `failed` read fails the run (exit non-zero):
    a green run is one where the experiment actually happened.
  - `not-run`: the arm stopped before its read step. Also a failure.
- `arms[i].read.classification.refusesBy`: `partition`, `application`, `both`, `neither` or
  `unknown` — computed from the item's dump against the build's `cdhash:` partition id and its
  designated requirement: the partition list (or `partition: "none"` with
  `itemAfterStore.partitionEntry` false — an item with no partition entry has no partition check to
  refuse it) and the decrypt entry's application list with every trusted application's
  `requirement:`. `neither` with a refused read would say the lists admit the build and the refusal
  came from elsewhere.
- `arms[i].read.bInDecryptAcl`: whether the build's designated requirement matches a `requirement:`
  in the decrypt entry's application list (false when the list names none, e.g. an any-application
  entry) — the application-side half of what a learner's Always Allow would record, and the
  decisive comparison for an item whose ACL is the run-44928776617 shape.
- `arms[i].a.implied` / `arms[i].b.implied`: `codesign -d -r-` prints a stored requirement as
  `designated => …` and, for code that carries none (an ad-hoc signature), the implied one as a
  source comment (`# designated => cdhash H"…"`, CODESIGN(1), `-r`). `implied: true` is expected for
  the ad-hoc arm's builds; a build re-signed by an identity arm that still prints the implied form
  fails that arm (the re-sign did not take), so both arms' `implied` values and the DR strings are
  read, never assumed.
- `arms[i].allow`: after a read that was not a success, the simulated allow — the partition half of
  a learner's Always Allow, the only half a runner can write. It is `{"skipped": "no partition
  entry"}` when the item carries no partition entry, because there is nothing to add; the
  application-list half is never simulated, so such an item keeps only that half.
- `arms[i].third.read`: C is B's bundle with a bumped `CFBundleShortVersionString`, signed the way
  its arm signs; `expected` is `refused`, and the record keeps `expected` and the observed outcome
  side by side. It runs with the allow (a skipped allow runs no third build).
- `keychain.loginPath` and `arms[i].login`: the learner's keychain, looked up for each of this run's
  synthetic services before and after the arm and swept in the run's `finally` (`exit 0` is
  "present"; the lookup never passes `-g`, which would withdraw the secret). A hit is deleted and
  fails the run: the item belongs in the experiment keychain this run measured.

## Done when 1 — the problem is established first

Each value is read from `keychain-record.json`; `<arm>` is `0` for `adhoc` and `1` for
`self-signed` in this run's default arm order (`adhoc`, `self-signed`, `project`).

| Reading | Record path | Value |
|---|---|---|
| the runner's macOS version and architecture | `platform.swVers`, `platform.machine` | `<pending: …>` |
| A's commit (the pull request base sha) and B's commit (the head) | `runs.a.commit`, `runs.b.commit` | `<pending: …>`, `<pending: …>` |
| the two packages' names and bytes | `runs.a.dmg`, `runs.b.dmg` (`.name`, `.bytes`) | `<pending: …>` |
| the two `.dmg` SHA-256s | `runs.a.dmg.sha256`, `runs.b.dmg.sha256` | `<pending: …>`, `<pending: …>` |
| the two built cdhashes, and that they differ (the premise every real upgrade has) | `guards.aCdhash`, `guards.bCdhash`, `guards.buildsDiffer` | `<pending: …>`, `<pending: …>`, `<pending: true>` |
| the ad-hoc arm: A's designated requirement and that it is the implied one | `arms[0].a.designatedRequirement`, `arms[0].a.implied` | `<pending: …>`, `<pending: true>` |
| the ad-hoc arm: B's designated requirement (the DR moves with every ad-hoc build) | `arms[0].b.designatedRequirement`, `arms[0].b.implied` | `<pending: …>` |
| the ad-hoc arm: A's and B's cdhashes | `arms[0].a.cdhash`, `arms[0].b.cdhash` | `<pending: …>` |
| the ad-hoc arm: whether A's item carries a partition entry, the stored list, and the computed value (as `codesign -dvvv` sees it) | `arms[0].itemAfterStore.partitionEntry`, `arms[0].itemAfterStore.partitions`, `arms[0].a.computedPartition` | `<pending: …>`, `<pending: …>`, `<pending: …>` |
| the ad-hoc arm: the item's ACL after A stores it — application list | `arms[0].itemAfterStore.anyApplication`, `arms[0].itemAfterStore.requirements` | `<pending: any application / requirements>` |
| the ad-hoc arm: the item's ACL after B's read | `arms[0].itemAfterRead.partitionEntry`, `arms[0].itemAfterRead.partitions`, `arms[0].itemAfterRead.anyApplication`, `arms[0].itemAfterRead.block` | `<pending: …>` |
| the ad-hoc arm: B's read, whether B's designated requirement is in the decrypt entry's list, and which check refused it | `arms[0].read.outcome`, `arms[0].read.bInDecryptAcl`, `arms[0].read.classification.refusesBy`, `arms[0].read.classification.detail` | `<pending: …>` |
| the ad-hoc arm: the SecurityAgent samples taken while B's read ran | `arms[0].read.launch.agentScan.probes`, `.matches`, `.error` | `<pending: …>` |
| the self-signed arm: the throwaway identity's CN and certificate SHA-1 | `arms[1].identity.cn`, `arms[1].identity.sha1` | `<pending: Pass Quest Keychain Experiment>`, `<pending: …>` |
| the self-signed arm: A's and B's designated requirements (both stored, `implied: false`), and whether they are equal | `arms[1].a.designatedRequirement`, `arms[1].b.designatedRequirement`, `arms[1].a.implied`, `arms[1].b.implied`, `arms[1].drSame` | `<pending: …>` |
| the self-signed arm: A's and B's cdhashes | `arms[1].a.cdhash`, `arms[1].b.cdhash` | `<pending: …>` |
| the self-signed arm: the item's ACL after A stores it and after B's read | `arms[1].itemAfterStore.block`, `arms[1].itemAfterRead.block` | `<pending: the raw dump blocks>` |
| the self-signed arm: B's read, whether B's designated requirement is in the decrypt entry's list, and which check refused it | `arms[1].read.outcome`, `arms[1].read.bInDecryptAcl`, `arms[1].read.classification.refusesBy` | `<pending: …>` |
| each refused/blocked arm: whether the allow was skipped (items with no partition entry) | `arms[<arm>].allow.skipped` | `<pending: skipped / absent>` |
| each refused/blocked arm with a partition entry: B's own item (its runtime partition id) | `arms[<arm>].allow.bPartition`, `arms[<arm>].allow.bPartitionComputed`, `arms[<arm>].allow.bItem.partitions` | `<pending: …>` |
| each refused/blocked arm with a partition entry: the simulated allow's new list and exit | `arms[<arm>].allow.command`, `arms[<arm>].allow.exit`, `arms[<arm>].allow.list` | `<pending: …>` |
| each refused/blocked arm with a partition entry: B's read after the allow (expected `read`) | `arms[<arm>].allow.reread.outcome` | `<pending: read>` |
| each refused/blocked arm with a partition entry: the third build C (version, cdhash, read, DR equal to B's) | `arms[<arm>].third.versionBefore`, `.versionAfter`, `.signature.cdhash`, `.read.outcome`, `.drEqualsB` | `<pending: refused>` |
| a read that was not a success: the SecurityAgent it left | `arms[<arm>].read.securityAgent`, `arms[<arm>].third.securityAgent` | `<pending: found / none>` |
| the project arm (P1 has no signing secrets) | `arms[2].skipped` | `<pending: PASSQUEST_MACOS_SIGNING_P12, PASSQUEST_MACOS_SIGNING_PASSWORD not set …>` |
| the keychain the run used, that it was the only store the app could reach, and that it was restored and deleted | `keychain.path`, `keychain.defaultDuring`, `keychain.searchListDuring`, `keychain.defaultBefore`, `keychain.defaultAfter`, `keychain.searchListBefore`, `keychain.searchListAfter`, `keychain.deleted` | `<pending: …>` |
| the learner's keychain, and every synthetic service found there (expected: none) | `keychain.loginPath`, `keychain.loginResidue` | `<pending: …>`, `<pending: []>` |
| the key never left its homes | `arms[<arm>].keySha256` (the key is only ever recorded as this); `guards.log.keyBytes` (the tool scanned its own printed text before it exited) | `<pending: …>`, `<pending: 0>` |

## What the first run must show

1. Both checkouts and both `bun run package` steps green; `runs.a.commit` and `runs.b.commit` are
   the base and head commits of the pull request and differ; both `.dmg`s exist with different
   SHA-256s.
2. `guards.buildsDiffer` is `true`, with both cdhashes quoted.
3. The three `security` calls that take an empty password are accepted on the runner, and the
   record shows it: `create-keychain -p ""`, `unlock-keychain -p ""` and `set-*-partition-list -k
   ""` (the first command that refuses fails the run by name, with its output in the failure line;
   this is the one thing the Windows implementer run cannot prove). The fourth call,
   `security import`, carries the throwaway identity's random passphrase on `-P` argv (redacted in
   the log) against a p12 the step exported legacy-encrypted; run 44928776617's `-P ""` against
   OpenSSL's default PKCS#12 encryption failed with `SecKeychainItemImport: The user name or
   passphrase you entered is not correct.`
4. The `adhoc` arm: `store.stored` is `true`; `itemAfterStore.partitionEntry` says whether A's item
   carries a partition entry at all, and when it does, `itemAfterStore.partitions` equals
   `a.computedPartition` (run 44928776617's item carried none, and the arm now reads on, so the
   partition guard is conditional while the item guard must still pass); `a.implied` and `b.implied`
   are both `true` (the implied requirement the ad-hoc signature prints as a source comment);
   `read.outcome` is recorded with `read.bInDecryptAcl`, `classification.refusesBy`, the two raw
   dump blocks and the agent samples.
5. The `self-signed` arm: `identity.cn` is the throwaway CN, `identity.sha1` is set, the arm signed
   (`couldNotSign` absent), both DRs are stored (`implied` false) and `drSame` is `true` (expected —
   the requirement names the certificate, the partition does not); the read outcome and
   classification are recorded exactly like the ad-hoc arm's.
6. The `project` arm records its skip by name (no secrets in the P1 run).
7. The log carries every `security`/`codesign`/`hdiutil`/`openssl`/`plutil`/`ditto`/`pgrep` argv
   (`security import`'s passphrase replaced by its placeholder),
   `<from run: the tool's [cleanup] deleted the experiment keychain … line>`,
   `<from run: the tool's [cleanup] restored the default keychain … line>`,
   `<from run: the workflow's keychain: deleted … line from the always() step>`, and no key bytes:
   the tool scans the text it printed, the record it writes and the files under its temp tree for
   every key it generated, records `guards.log`, and fails the run on a hit.
8. The `keychain-record` artifact holds both files, and the run's early step prints
   `<from run: the [self-check] N of N case(s) passed line>` (the parser, the requirement parser,
   the guards and the classifier, on the runner).

## What each outcome means for the next step (the plan's stop conditions)

- `arms[0].read.outcome == "read"` (an ad-hoc B reads A's item): the goal's premise is false — stop,
  report the run, add no identity.
- `arms[1].read.outcome` is `refused` or `blocked`: a project-owned self-signed identity cannot meet
  Done-when 2; `read.bInDecryptAcl`, `classification.refusesBy` and the `allow`/`third` rows say
  which side refused it (with run 44928776617's ACL shape: the decrypt entry's application list and
  a `skipped` allow; when the item does carry a partition entry: the partition list and the third
  build C's outcome). Then the owner's decision (plan default 10) is the only way forward: an
  Apple-issued certificate, or accepting a re-prompt per upgrade.
- `arms[1].read.outcome == "read"`: P2–P4 proceed (the identity keeps access across builds); the
  pinned DR string for P2 is read from `arms[1].a.designatedRequirement` (or
  `arms[1].b.designatedRequirement`; they must be byte-equal in a passing run).
- `arms[<arm>].read.outcome == "failed"` or `"not-run"`: the run is red; nothing about that arm was
  established, and its `read.detail` and `failures` say what broke. No stop-condition reading is
  made from it.
- `arms[<arm>].read.classification.refusesBy == "application"`: the item's application list holds
  requirements that refuse the build; the simulated allow (partition half only) would not repair
  it, and the record says so instead of pretending otherwise.
- `arms[<arm>].third.read.outcome == "read"` (C is admitted): the allow recorded something
  build-independent — report it; the partition half of the model would be contradicted by the run.
- A named SecurityAgent fails the run by design (the plan's guard); its record is the finding.

## Local checks in this step (the P1 implementer run, Windows 11)

The experiment itself is macOS-only (the tool exits 2 elsewhere); everything below is the pure
parser/guard/classifier check, the must-fail fixtures and the text gates.

| Check | Result |
|---|---|
| `mise exec -- bun harness/tools/keychain-experiment.ts --self-check` | PASS — `15 of 15 case(s) passed`: the dump fixtures (partition entry, any-application owner, an unparsable partition description that must leave the list empty, and run 44928776617's 3-entry no-partition-entry ACL), the build/partition/item guards (including the must-fail equal-cdhash case), the refusal classifier (partition, application, matched requirement, unmatched requirement), the `bInDecryptAcl` comparison, the designated-requirement parser (the commented implied form and the plain stored form, plus the re-sign rule that refuses an implied requirement) and the read classifier (a kill or a stuck window is `blocked` only with a SecurityAgent sample, `refused` for a missing row, `read` only when the stub carried the key) |
| must-fail fixtures, round 1 (`Q:/scratch/omp/g18/P1/mustfail/`) | PASS — with the parser narrowed to the plain form `12 of 13 case(s) passed ... the commented form was not read as the implied one`; with the classifier reading `killed` instead of the samples `12 of 13 ... a kill with no SecurityAgent sampled gave blocked` — each new case fails on a deliberately broken input, and the repo copy is untouched |
| must-fail fixtures, round 3 (`Q:/scratch/omp/g18/p1-r3/mustfail/`) | PASS — with `itemGuardProblem` back to failing on any empty partition list the no-partition-entry case fails (`14 of 15 case(s) passed ... the item guard failed the observed ACL: the item carries a partition entry whose value cannot be read …`); with `decryptAclAdmits` reading the `encrypt` entry instead of the `decrypt` entry the `bInDecryptAcl` case fails (`14 of 15 ... A's own requirement was not matched in the decrypt entry`) — each new case catches its deliberate break, and the repo copy is untouched |
| the workflow's identity step, run locally (`Q:/scratch/omp/g18/p1-r3/`, Git's OpenSSL 3.5.9) | PASS — the step script passes `bash -n`, chooses `pkcs12 pbe flags -legacy` for `OpenSSL 3.5.9`, and with an empty `OPENSSL_MODULES` the first export fails (`pkcs12: unable to load provider legacy`) and the retry produces `MAC: sha1`, `PKCS7 Encrypted data: pbeWithSHA1And3-KeyTripleDES-CBC`, `Shrouded Keybag: pbeWithSHA1And3-KeyTripleDES-CBC`; the passphrase appears on the step's stdout zero times (`::add-mask::` is consumed by the runner, `OmitEcho`), and the throwaway key, certificate and p12 were deleted in the same run |
| dry run of the whole tool (`Q:/scratch/omp/g18/p1-r3/dryrun/`, Windows) | PASS — the macOS commands are stand-in stubs (`security`, `codesign`, `hdiutil`, `ditto`, `plutil`, `pgrep`, `sw_vers`, `uname`), the stubbed `security dump-keychain` prints the 3-entry no-partition ACL, and the app is a fake smoke hook: `--arms adhoc,self-signed,project` → tool exit 0, `status: ok`, 30 assertions, `guards.buildsDiffer: true`, `guards.log: {prints: 366, keyBytes: 0}`; both arms record `itemAfterStore.partitionEntry: false`, `read.outcome: refused`, `read.bInDecryptAcl: false`, `classification.refusesBy: application`, `allow: {"skipped": "no partition entry"}` and no `third`; the self-signed arm's import line is logged `-P "<passphrase-redacted>"`, the stub fails the import unless `-P` is exactly the passphrase the env carries, and the passphrase appears in neither the log nor the record (`[guard] no signing passphrase in the 367 line(s) this tool printed`); the "user default and sole search entry" check passes on the one resolved form. Round 1's runs of the same stub harness covered the `blocked` path (a stubbed SecurityAgent) and the login-keychain sweep's failure; those paths are unchanged by this round |
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts typecheck.ts typecheck` | PASS — `0 type errors in tsconfig.json, desktop/web/tsconfig.json` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts secrets.ts secrets` | PASS — `775 files, 0 findings` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts docs-links.ts docsLinks` | PASS — `327 checked, 0 broken` |
| Go checks (`gofmt -l`, `go vet`, `go build`, `go test`) | not run: this step changes no Go file (`harness/`, `.github/` and `docs/` only) |

## Known limits of this record

- A runner cannot click a Keychain prompt. A read that does not complete is therefore reported as
  `blocked` only when the sampler saw a `SecurityAgent` while the launch ran (the prompt path), and
  as `failed` (a red run) when it did not; the record carries the samples, never an assumption.
- The sampler is one `pgrep` a second, so a `SecurityAgent` that both appears and disappears between
  two probes would be missed; a miss turns the read into `failed`, which is loud, and never into
  `read`.
- A runner cannot click a Keychain prompt, so the simulated allow can write only the **partition**
  half of a real "Always Allow"; the application-list half (the requester's code requirement) is
  never written by this tool. Run 44928776617's item carried no partition entry at all, so for that
  shape the allow is recorded as `skipped: no partition entry` and neither half is simulated — the
  decisive comparison there is `read.bInDecryptAcl`, which is why the record keeps it beside the
  read outcome.
- macOS 27 has no runner and its Security source is not published; the citations are
  `Security-61901.120.67`, one macOS 26-era tag (`docs/plans/2026-11-22-0100-0018-keychain-survives-upgrade.md`,
  fact 2), and the record names the runner's `sw_vers`.
- The log scan covers the text the tool itself printed; the workflow's own `echo` lines (paths
  only) are not part of it.
- The dry run's numbers are the orchestration's, not macOS's: its `security`, `codesign` and
  `hdiutil` output is written by stubs in the documented formats, so it proves the flow, the record
  and the guards, never what the real Keychain answers.
- The record is an artifact, not a published file: it holds the runner's own temp paths
  (`/Users/runner/...`), and no secret, key byte or passphrase.
