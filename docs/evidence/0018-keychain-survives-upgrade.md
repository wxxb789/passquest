# 0018 — macOS upgrades keep the learner's Keychain permission: evidence

Contract: [`../goals/0018-keychain-survives-upgrade.md`](../goals/0018-keychain-survives-upgrade.md).
Plan: [`../plans/2026-11-22-0100-0018-keychain-survives-upgrade.md`](../plans/2026-11-22-0100-0018-keychain-survives-upgrade.md).
Experiment: [`.github/workflows/keychain.yml`](../../.github/workflows/keychain.yml) and
[`harness/tools/keychain-experiment.ts`](../../harness/tools/keychain-experiment.ts).
Decisions: Q-0011-03, Q-0013-08, Q-0017-* in [`../decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md).

**State: draft.** Run 44931358398 established the problem on a macOS 26 runner and its facts are
below, each read from its `keychain-record` artifact. What run 44931358398 could not answer — the
learner's own keychain, where securityd may add a partition entry, and the self-signed arm, which
that run never reached — was the pending half: the four-job run (one arm × one keychain per artifact)
named in the run table is run 44983246217, whose answers stand in *What run 44983246217 (the matrix
run) showed*; the self-signed arm's own readings stay pending the identity check of plan amendment 17.
No outcome was written by hand: every
value names the artifact and JSON path it is read from, and the "Local checks" tables hold only what
already happened on this Windows machine (which cannot run the macOS experiment at all).

## The run this record cites

| Run | Workflow | What it is | Result |
|---|---|---|---|
| 44931358398 | `keychain` | the P1 experiment on `macos-26`, the three-arm shape: two packages built (A = the pull request base sha, B = its head), arms `adhoc,self-signed,project` in one job | the ad-hoc arm's read was **blocked** — a SecurityAgent was up in 20 of the 23 probes, the window never rendered the provider row — and the arm guard (since removed: one arm runs per job now) stopped the run before the self-signed arm; the item carried no partition entry and refuses by application (`keychain-record.json`: `arms[0]`) |
| `<pending: run id>` | `keychain` | the matrix run this tree drives: `arm: [adhoc, self-signed]` × `keychain: [experiment, login]`, one arm against one keychain per job, `fail-fast: false` | `<pending: the four jobs' outcomes>` |

Artifacts: run 44931358398 uploaded `keychain-record`; the matrix run uploads
`keychain-record-<arm>-<keychain>` (four names), each holding `keychain-record.json` and
`keychain-experiment.log`, retained 30 days.

Filling it:

```sh
gh run download 44931358398 -n keychain-record --dir /tmp/0018
jq '{runs, guards, keychain, status, failures}' /tmp/0018/keychain-record.json
jq '.arms[0] | {id, identity, skipped, keySha256, a, b, drSame, itemAfterStore, itemAfterRead, read, allow, third, failures}' /tmp/0018/keychain-record.json
gh run download <matrix run id> -n keychain-record-self-signed-login --dir /tmp/0018/ssl
jq '.arms[0] | {id, identity, skipped, keySha256, a, b, drSame, itemAfterStore, read, allow, third, failures}' /tmp/0018/ssl/keychain-record.json
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
  application-list half is never simulated, so such an item keeps only that half. In the login
  variant the partition write needs the login keychain's password, which a run cannot present, so
  the skip reason there is the write's own exit; a skipped allow runs no third build.
- `arms[i].third.read`: C is B's bundle with a bumped `CFBundleShortVersionString`, signed the way
  its arm signs; `expected` is `refused`, and the record keeps `expected` and the observed outcome
  side by side. It runs with the allow (a skipped allow runs no third build).
- `arms[i].identity.partitionList`: `security set-key-partition-list -S apple-tool:,apple: -s -k ""`
  and its exit — the write that lets `codesign` use an imported key (`security.1:810`) and that
  needs the keychain's password (`security.1:736-742`). In the login variant the password is not
  available, so a non-zero exit stops that arm before it signs (a prompt nobody can click would
  otherwise hold the job to its own timeout), with the exit in the record.
- `arms[i].signs`: every `codesign --force --deep --sign …` this arm ran, in order, with its
  `where` label (`<arm>-a`, `<arm>-b`, `<arm>-c`), the `signer` it used — the identity's SHA-1 as
  `security find-identity -p codesigning` prints it, its CN on the retry, `-` for the ad-hoc third
  build — the exit, and the last 60 output lines.
- `keychain.variant`: `experiment` (a throwaway keychain created, made the sole store,
  restored and deleted in the `finally`) or `login` (nothing created or switched; the app stores
  into the runner's default login keychain, as a learner's would).
- `keychain.loginPath` and `arms[i].login`: the learner's keychain in the experiment variant, looked
  up for each of this run's synthetic services before and after the arm and swept in the run's
  `finally` (`exit 0` is "present"; the lookup never passes `-g`, which would withdraw the secret).
  A hit is deleted and fails the run: the item belongs in the experiment keychain this run measured.
- `keychain.loginCleanup` (login variant): the synthetic services looked up and deleted in the login
  keychain by the `finally`, each with the lookup's exit and the delete's exit (`null`: nothing to
  delete). `keychain.identityCleanup`: every imported throwaway CN and the `security delete-identity
  -c <CN>` exit; a failed delete fails the run.
- `arms[i].read.securityAgent`: the SecurityAgent a read that did not complete left behind — the
  pids seen before the kill, whether the kill was issued, the pids seen after it, and
  `respawned: true` when a pid after the kill was not there before (launchd holds SecurityAgent as
  an on-demand service and restarts it). One arm runs per job, so this is recorded, never a stop.

## Done when 1 — the problem is established first

Run 44931358398's `keychain-record` artifact is the ad-hoc arm against the tool's own experiment
keychain. Each matrix job uploads one arm against one keychain and every record holds that arm as
`arms[0]`. The `<pending>` rows belong to the matrix run: run 44931358398 never reached the
self-signed arm, and login-variant rows are new with this tree.

| Reading | Artifact | Record path | Value |
|---|---|---|---|
| the runner's macOS version and architecture | `keychain-record` (44931358398) | `platform.swVers`, `platform.machine` | `26.7.2`, `arm64` |
| A's commit (the pull request base sha) and B's commit (the head) | `keychain-record` | `runs.a.commit`, `runs.b.commit` | `3250fb0edff8be11ebcbc369a6d93d1e8b62aea8` (main's tip, the 0017 merge), `f1f85c456495a3295047beacdb244d6b2dfe3f0d` |
| the two packages' names and bytes | `keychain-record` | `runs.a.dmg`, `runs.b.dmg` (`.name`, `.bytes`) | `pass-quest-0.15.0-arm64.dmg` 31 944 704 bytes; the same name, 31 945 728 bytes |
| the two `.dmg` SHA-256s | `keychain-record` | `runs.a.dmg.sha256`, `runs.b.dmg.sha256` | `8247c93264113988655e4c4a524a0bd3290d5ca6ec84ac9bb29c743c84f34e2f`, `81567c0f787efcc4e2a5921acf577654ad306146ff738f28f6bd161e9179bc73` |
| the two built cdhashes, and that they differ (the premise every real upgrade has) | `keychain-record` | `guards.aCdhash`, `guards.bCdhash`, `guards.buildsDiffer` | `b9bcd4b4f4cab7f45281383a3b1b0ae7016da185`, `54b4a4b191d819ddf66afab80a898407237838e3`, `true` |
| the ad-hoc arm: A's designated requirement and that it is the implied one | `keychain-record` | `arms[0].a.designatedRequirement`, `arms[0].a.implied` | `cdhash H"b9bcd4b4f4cab7f45281383a3b1b0ae7016da185"`, `true` |
| the ad-hoc arm: B's designated requirement (the DR moves with every ad-hoc build) | `keychain-record` | `arms[0].b.designatedRequirement`, `arms[0].b.implied` | `cdhash H"54b4a4b191d819ddf66afab80a898407237838e3"`, `true` |
| the ad-hoc arm: A's and B's cdhashes | `keychain-record` | `arms[0].a.cdhash`, `arms[0].b.cdhash` | the two `guards` values above |
| the ad-hoc arm: whether A's item carries a partition entry, the stored list, and the computed value (as `codesign -dvvv` sees it) | `keychain-record` | `arms[0].itemAfterStore.partitionEntry`, `arms[0].itemAfterStore.partitions`, `arms[0].a.computedPartition` | `false`, `[]`, `cdhash:b9bcd4b4f4cab7f45281383a3b1b0ae7016da185` |
| the ad-hoc arm: the item's ACL after A stores it — application list | `keychain-record` | `arms[0].itemAfterStore.anyApplication`, `arms[0].itemAfterStore.requirements` | `false`; one entry (1) carries `cdhash H"b9bcd4b4f4cab7f45281383a3b1b0ae7016da185"` (the encrypt entry's any-application list is not the verdict) |
| the ad-hoc arm: the item's ACL after B's read (the raw dump block is in the record) | `keychain-record` | `arms[0].itemAfterRead.partitionEntry`, `arms[0].itemAfterRead.partitions`, `arms[0].itemAfterRead.anyApplication`, `arms[0].itemAfterRead.block` | `false`, `[]`, `false`; the same 3-entry ACL as after the store: `encrypt` any-application / `decrypt derive export_clear export_wrapped mac sign` naming A's requirement / `change_acl` empty |
| the ad-hoc arm: B's read, whether B's designated requirement is in the decrypt entry's list, and which check refused it | `keychain-record` | `arms[0].read.outcome`, `arms[0].read.bInDecryptAcl`, `arms[0].read.classification.refusesBy`, `arms[0].read.classification.detail` | `blocked`; `false`; `application` (partition `none`); "the item carries no partition entry, so no partition list weighs on the read; the decrypt entry's application list is specific requirements that do not match the designated requirement" |
| the ad-hoc arm: the read itself (a prompt nobody could click) | `keychain-record` | `arms[0].read.detail` | "the window never rendered the provider row inside its own 15 s bound; the SecurityAgent sampler saw a SecurityAgent in 20 of 23 probe(s), last at 2026-11-21T17:43:21.819Z: 7557 …/SecurityAgent …; a read that did not complete while a SecurityAgent was up is the prompt path" |
| the ad-hoc arm: the SecurityAgent samples taken while B's read ran | `keychain-record` | `arms[0].read.launch.agentScan.probes`, `.matches`, `.error` | 23 probes, 20 samples (each with its timestamp and the `7557` line), `error: null` |
| the ad-hoc arm: the SecurityAgent it left, and that launchd respawned it | `keychain-record` | `arms[0].read.securityAgent.found`, `.killed`, `.left` | `7557 …/SecurityAgent`; `true`; a new pid, `8112 …/SecurityAgent` — this run's guard then stopped the experiment (removed: the record now carries `respawned`) |
| the self-signed arm: the throwaway identity's CN and certificate SHA-1 | `keychain-record-self-signed-experiment`, `keychain-record-self-signed-login` | `arms[0].identity.cn`, `arms[0].identity.sha1` | `<pending: Pass Quest Keychain Experiment>`, `<pending>` |
| the self-signed arm: the partition-list write that lets `codesign` use the key | `keychain-record-self-signed-experiment`, `keychain-record-self-signed-login` | `arms[0].identity.partitionList.exit` | `<pending: 0 in the experiment keychain; the login keychain's password is not available to a run>` |
| the self-signed arm: A's and B's designated requirements (both stored, `implied: false`), and whether they are equal | `keychain-record-self-signed-experiment`, `keychain-record-self-signed-login` | `arms[0].a.designatedRequirement`, `arms[0].b.designatedRequirement`, `arms[0].a.implied`, `arms[0].b.implied`, `arms[0].drSame` | `<pending: expected identifier "com.github.wxxb789.passquest" and certificate root = H"…", drSame true>` |
| the self-signed arm: A's and B's cdhashes | `keychain-record-self-signed-experiment`, `keychain-record-self-signed-login` | `arms[0].a.cdhash`, `arms[0].b.cdhash` | `<pending>` |
| the self-signed arm: the item's ACL after A stores it and after B's read (the raw dump blocks are in the record) | `keychain-record-self-signed-experiment`, `keychain-record-self-signed-login` | `arms[0].itemAfterStore.block`, `arms[0].itemAfterRead.block` | `<pending>` |
| the self-signed arm: B's read, whether B's designated requirement is in the decrypt entry's list, and which check refused it | `keychain-record-self-signed-experiment`, `keychain-record-self-signed-login` | `arms[0].read.outcome`, `arms[0].read.bInDecryptAcl`, `arms[0].read.classification.refusesBy` | `<pending>` |
| each refused/blocked arm: whether the allow was skipped | `keychain-record` | `arms[0].allow.skipped` | `no partition entry` (the item carried none, so the partition half had nothing to add) |
| each refused/blocked arm with a partition entry: B's own item (its runtime partition id) | `<pending: no run yet>` | `arms[0].allow.bPartition`, `arms[0].allow.bPartitionComputed`, `arms[0].allow.bItem.partitions` | `<pending: the items of run 44931358398 carry no partition entry>` |
| each refused/blocked arm with a partition entry: the simulated allow's new list and exit | `<pending: no run yet>` | `arms[0].allow.command`, `arms[0].allow.exit`, `arms[0].allow.list` | `<pending: in the login variant the write needs the login keychain's password and its exit lands in the allow.skipped field>` |
| each refused/blocked arm with a partition entry: B's read after the allow (expected `read`) | `<pending: no run yet>` | `arms[0].allow.reread.outcome` | `<pending: read>` |
| each refused/blocked arm with a partition entry: the third build C (version, cdhash, read, DR equal to B's) | `<pending: no run yet>` | `arms[0].third.versionBefore`, `.versionAfter`, `.signature.cdhash`, `.read.outcome`, `.drEqualsB` | `<pending: refused>` |
| the keychain the run used, that it was the only store the app could reach, and that it was restored and deleted | `keychain-record` | `keychain.variant`, `keychain.defaultDuring`, `keychain.searchListDuring`, `keychain.defaultBefore`, `keychain.defaultAfter`, `keychain.searchListBefore`, `keychain.searchListAfter`, `keychain.deleted` | `experiment` (a throwaway `experiment.keychain` under the runner temp dir — the record carries the full path); during the run it was the user default and the sole search entry; before and after, both were `~/Library/Keychains/login.keychain-db`; deleted |
| the login variant: the item in the learner's keychain, and the cleanup that deletes it | `keychain-record-adhoc-login`, `keychain-record-self-signed-login` | `keychain.variant`, `arms[0].itemAfterStore.partitionEntry`, `arms[0].itemAfterStore.partitions`, `arms[0].itemAfterStore.block`, `keychain.loginCleanup` | `<pending: this is the open question — whether the login keychain's item carries the partition entry the experiment keychain's does not>` |
| the login variant: the identity deleted in the `finally` | `keychain-record-self-signed-login` | `keychain.identityCleanup` | `<pending: {cn, exit}>` |
| the learner's keychain, and every synthetic service found there (expected: none) | `keychain-record` | `keychain.loginPath`, `keychain.loginResidue` | `~/Library/Keychains/login.keychain-db`, `[]` |
| the key never left its homes | `keychain-record` | `arms[0].keySha256`; `guards.log.keyBytes` | `631aff91851105e305e809b0c9848a8fd5074802de3f4ff103e9ee893f9daeab`; `0` (254 printed lines scanned) |

The project arm is not in the matrix this tree drives (`arm: [adhoc, self-signed]`); a `project` run
still records `arms[0].skipped` by name when the two signing secrets are absent, which is the P3 path.

## What run 44931358398 showed

1. Both checkouts and both `bun run package` steps were green; `runs.a.commit` and `runs.b.commit`
   differ and the two `.dmg`s carry the SHA-256s in the table above.
2. `guards.buildsDiffer` is `true`, with both cdhashes quoted.
3. The empty-password `security` calls this shape reaches were accepted: `create-keychain -p ""`
   and `unlock-keychain -p ""` (the log shows both exit 0). No `set-*-partition-list -k ""` ran —
   the ad-hoc arm imports no identity, and the item carried no partition entry, so the allow was
   skipped; those writes are the matrix run's identity arms. The identity step's export and its
   `-P` passphrase (redacted in the log) are unchanged from run 44928776617, whose `-P ""` against
   OpenSSL's default PKCS#12 encryption failed with `SecKeychainItemImport: The user name or
   passphrase you entered is not correct.`
4. The `adhoc` arm: `store.stored` `true`; `itemAfterStore.partitionEntry` `false` (so the
   partition guard stayed conditional and the item guard passed); `a.implied` and `b.implied` both
   `true`; `read.outcome` `blocked` with `read.bInDecryptAcl` `false`,
   `classification.refusesBy` `application`, the two raw dump blocks and the agent samples — all in
   the table above.
5. The `self-signed` arm did not run: the arm guard (since removed — one arm per job means nothing
   needs it gone) stopped the experiment after the ad-hoc arm's blocked read. The matrix run's
   `keychain-record-self-signed-experiment` and `keychain-record-self-signed-login` artifacts carry
   it.
6. The `project` arm did not run either: it is not in this tree's matrix (the P3 path).
7. The log carries every `security`/`codesign`/`hdiutil`/`openssl`/`plutil`/`ditto`/`pgrep` argv,
   the two `[cleanup]` lines (the default keychain and the search list restored, the experiment
   keychain deleted), the workflow's always() `keychain: deleted …` line for the throwaway
   directory, and no key bytes: `guards.log` is `{prints: 254, keyBytes: 0}`.
8. The `keychain-record` artifact holds both files, and the run's early step printed
   `[self-check] 15 of 15 case(s) passed` (this tree's tool prints 17).

## What the matrix run must show (pending)

1. Four artifacts, `keychain-record-<arm>-<keychain>`, each with its own record and log, and each
   job's self-check line; the job's log names the arm and the keychain it ran against, and
   `guards.log.keyBytes` stays `0`.
2. The two login jobs: `keychain.variant: login`; `keychain.defaultBefore` is the runner's login
   keychain; nothing switched it; the item's full ACL after the store and after the read is in
   `arms[0].itemAfterStore.block` / `itemAfterRead.block` — the open question is
   `itemAfterStore.partitionEntry` there, because the experiment keychain's item carries none; and
   `keychain.loginCleanup` carries the delete's exit for every synthetic service.
3. The two self-signed jobs: `identity.cn` is the throwaway CN, `identity.sha1` is set,
   `identity.partitionList.exit` is recorded (in the login job it may stop the arm with the record
   saying why), both DRs are stored (`implied` false) and `drSame` is `true` (expected — the
   requirement names the certificate, the partition does not); `keychain.identityCleanup` carries
   the `security delete-identity -c <CN>` exit.
4. Every read outcome and classification exactly like the ad-hoc arm's, and — for an item that
   carries a partition entry — the allow's partition write, the reread and the third build.

## What run 44983246217 (the matrix run) showed

Run 44983246217 is the matrix run this table was waiting for: four jobs, four artifacts
`keychain-record-<arm>-<keychain>`, each holding a record and a log. Its two ad-hoc jobs are green
(`status: ok`, `17 assertion(s) passed, 0 failed`, empty `failures`); its two self-signed jobs
stopped at the identity import, which plan amendment 17 fixes. Every value below is read from that
run's artifacts:

```sh
gh run download 44983246217 --dir /tmp/0018-r5   # four artifact directories
jq '{status, platform, guards, failures}' /tmp/0018-r5/keychain-record-adhoc-login/keychain-record.json
```

| Reading | Artifact | Record path | Value |
|---|---|---|---|
| the two ad-hoc jobs: status, runner, and the printed-line scan | `keychain-record-adhoc-experiment`, `keychain-record-adhoc-login` | `status`, `platform.swVers`, `platform.machine`, `guards.log` | `ok`; `26.7.2`, `arm64`; `{prints: 262, keyBytes: 0}` and `{prints: 319, keyBytes: 0}` |
| the commits, and that the two jobs' builds are not byte-identical | both ad-hoc records | `runs.a.commit`, `runs.b.commit`, `runs.a.dmg.sha256`, `runs.b.dmg.sha256` | A `3250fb0edff8be11ebcbc369a6d93d1e8b62aea8`, B `d340a08d69165473294a5a53f1cc5a4507f548e9`; the experiment job `c618f7b53a20094b8e8d423d130e3f8f07a71a432b72bb4909136c04f8279422` / `07afa5f2a732f7d2e5bca25e9e39d00be54698cf4dd3864d33c6698e8a9f310c`, the login job `aaf81a7be2a40dca0f52dc9f29f441ddfa6d24a509b5dcedfbb014ebb8b4adee` / `a7e558c46d023547f042505fd8e0ddb1465875f2edf0bb0c4a4e73f8c2e72a04` |
| the two builds differ, and their cdhashes | both ad-hoc records | `guards.buildsDiffer`, `guards.aCdhash`, `guards.bCdhash` | `true`; `b9bcd4b4f4cab7f45281383a3b1b0ae7016da185`, `7d974323112626c1e88098342bc115b5a3e55b61` |
| the ad-hoc DRs move with the build (both the implied form) | both ad-hoc records | `arms[0].a.designatedRequirement`, `arms[0].a.implied`, `arms[0].b.designatedRequirement`, `arms[0].b.implied`, `arms[0].drSame` | `cdhash H"b9bc…185"` and `cdhash H"7d97…b61"`, both `implied: true`, `drSame: false` |
| the open question of this record: the experiment keychain's item carries no partition entry, the learner's login keychain's does | `keychain-record-adhoc-experiment`, `keychain-record-adhoc-login` | `arms[0].itemAfterStore.partitionEntry`, `arms[0].itemAfterStore.partitions`, `arms[0].a.computedPartition` | `false`, `[]` versus `true`, `["cdhash:b9bcd4b4f4cab7f45281383a3b1b0ae7016da185"]`; the computed value equals the stored one in both |
| B's read in both ad-hoc jobs: blocked, and which check refuses it | both ad-hoc records | `arms[0].read.outcome`, `arms[0].read.bInDecryptAcl`, `arms[0].read.classification.refusesBy` | `blocked`, `false`; `application` in the experiment keychain (no partition list to weigh), `both` in the login keychain (whose stored partition list names A) |
| the blocked read: the prompt it met, and the SecurityAgent launchd respawned | both ad-hoc records | `arms[0].read.detail`, `arms[0].read.launch.agentScan.probes`, `arms[0].read.launch.agentScan.matches`, `arms[0].read.securityAgent` | "the window never rendered the provider row inside its own 15 s bound" with a SecurityAgent sampled in 20 of 24 probes; `{found: [12515 …], killed: true, left: [13184 …], respawned: true}` and `{found: [5926 …], killed: true, left: [6428 …], respawned: true}` |
| the login variant's allow: the partition write a run cannot make | `keychain-record-adhoc-login` | `arms[0].allow.skipped` | `security set-generic-password-partition-list … exited 1 (keychain: "/Users/runner/Library/Keychains/login.keychain-db" \| version: 512)` — the login keychain's password is not available to a run, so no third build ran (`arms[0].third` absent) |
| the learner's keychain after the login job | `keychain-record-adhoc-login` | `keychain.variant`, `keychain.loginCleanup` | `login`; one synthetic service, `{lookedUp: 0, deleted: 0}` |
| the two self-signed jobs: where they stopped | `keychain-record-self-signed-experiment`, `keychain-record-self-signed-login` | `status`, `failures`, `guards.log.keyBytes` | `failed`; `identity self-signed: security import: security import exited 0 or imported no identity: (no output)` — `security import -f pkcs12` prints nothing at all (plan amendment 17) — then `the arm did not reach its read step`; `keyBytes: 0` |

The placeholder rows in the table above stay for the coordinator's evidence pass; this section
records the run, it does not rewrite that table, and the self-signed arm's own rows are unanswered
until the identity check of amendment 17 reaches CI. The `project` arm is not in the matrix.

## What each outcome means for the next step (the plan's stop conditions)

- Run 44931358398's ad-hoc read is `blocked`: on that runner, in the tool's own experiment keychain,
  an ad-hoc B did **not** read A's item without a prompt — Done-when 1's problem is established for
  that shape, and the premise is not falsified. The login variant is what closes Done-when 1: the
  learner's item may be the one that carries a partition entry.
- `arms[0].read.outcome == "read"` in `keychain-record-adhoc-experiment` or
  `keychain-record-adhoc-login` (an ad-hoc B reads A's item without a prompt): the goal's premise is
  false for that shape — stop, report the run, add no identity.
- `arms[0].read.outcome` is `refused` or `blocked` in the self-signed artifacts: a project-owned
  self-signed identity cannot meet Done-when 2; `read.bInDecryptAcl`, `classification.refusesBy` and
  the `allow`/`third` rows say which side refused it (with the observed ACL shape: the decrypt
  entry's application list and a `skipped` allow; when the item does carry a partition entry: the
  partition list and the third build C's outcome). Then the owner's decision (plan default 10) is
  the only way forward: an Apple-issued certificate, or accepting a re-prompt per upgrade — note
  that in the login variant the identity's `partitionList` failure can stop the arm before its read,
  which is itself a limit of a self-signed identity on a machine whose keychain password a run does
  not hold.
- `arms[0].read.outcome == "read"` in the self-signed artifacts: P2–P4 proceed (the identity keeps
  access across builds); the pinned DR string for P2 is read from
  `arms[0].a.designatedRequirement` (or `arms[0].b.designatedRequirement`; they must be byte-equal
  in a passing run).
- `arms[0].read.outcome == "failed"` or `"not-run"`: the run is red; nothing about that arm was
  established, and its `read.detail` and `failures` say what broke. No stop-condition reading is
  made from it.
- `arms[0].read.classification.refusesBy == "application"`: the item's application list holds
  requirements that refuse the build; the simulated allow (partition half only) would not repair
  it, and the record says so instead of pretending otherwise.
- `arms[0].third.read.outcome == "read"` (C is admitted): the allow recorded something
  build-independent — report it; the partition half of the model would be contradicted by the run.
- A named SecurityAgent is recorded, never a failure: `read.securityAgent` carries the pids before
  the kill, that the kill was issued, the pids after it and whether launchd respawned the daemon
  (it did in run 44931358398: `7557` → `8112`). One arm runs per job, so nothing follows that a
  lingering daemon could hold back, and the kill stays as cleanup.

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

## Local checks of the one-arm-per-job tree (round 4, Windows 11)

| Check | Result |
|---|---|
| `mise exec -- bun harness/tools/keychain-experiment.ts --self-check` | PASS — `17 of 17 case(s) passed`: the 15 cases above plus the argument rules (one `--arms`, one `--keychain`, both required) and the SecurityAgent respawn comparison |
| must-fail fixtures, round 4 (`Q:/scratch/omp/g18/r4/mustfail/`) | PASS — with `agentRespawned` forced to `false` the respawn case fails (`16 of 17 case(s) passed … a new pid was not read as a respawn`); with the single-arm check disabled the argument case fails (`16 of 17 … two arms were accepted`) — each new case catches its deliberate break, and the repo copy is untouched |
| dry run, round 4 (`Q:/scratch/omp/g18/r4/dryrun/`, Windows) | PASS — the stubs model per-keychain item maps and a login keychain whose password is unknown unless `PQ_STUB_LOGIN_PASSWORD=""`, and the app stub writes the item's requirement and, with `PQ_STUB_ITEM_PARTITION=self`, a partition entry. `--arms adhoc --keychain experiment` → exit 0, `refused`, `allow: {"skipped": "no partition entry"}`; `--arms adhoc --keychain login` → exit 0, the item deleted from login (`loginCleanup` `{lookedUp: 0, deleted: 0}`); `--arms self-signed --keychain experiment` → exit 0, both DRs explicit and `drSame: true`, the identity deleted, the keychain deleted; `--arms self-signed --keychain login` with the login password unknown → exit 1, `identity.partitionList.exit: 51` and the man-page reason, the identity still deleted in the finally; the same with an empty login password → exit 0 with `loginCleanup`/`identityCleanup` exit 0; the partition-entry run → the allow applied, the reread `read`, the third `refused`, both items deleted; the agent-sample run → `read.outcome: blocked` with 6 of 6 samples in `agentScan` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts typecheck.ts typecheck` | PASS — `0 type errors in tsconfig.json, desktop/web/tsconfig.json` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts docs-links.ts docsLinks` | PASS — `327 checked, 0 broken` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts secrets.ts secrets` | PASS — `775 files, 0 findings` |
| the workflow's YAML | PASS — parsed with `Bun.YAML.parse`: `jobs.experiment.strategy.matrix` is `{arm: [adhoc, self-signed], keychain: [experiment, login]}`, `fail-fast: false`, and the upload step's `with.name` is `keychain-record-${{ matrix.arm }}-${{ matrix.keychain }}` |

## Local checks of the find-identity tree (round 5, Windows 11)

This tree's change is the identity check and the signer of plan amendment 17: `security import
-f pkcs12` prints nothing, so the import is verified by `security find-identity -p codesigning`
without `-v`, whose listing carries the identity's SHA-1 — the value `codesign --force --deep --sign
<SHA-1> --keychain <keychain>` now signs with.

| Check | Result |
|---|---|
| `mise exec -- bun harness/tools/keychain-experiment.ts --self-check` | PASS — `18 of 18 case(s) passed`: the round-4 cases plus `an identity the system does not trust is listed only by find-identity without -v`, whose fixture is the plain listing (`Matching identities`, `1) <SHA-1> "CN" (CSSMERR_TP_NOT_TRUSTED)`, `1 identities found`) against the `-v` listing of the same run (`0 valid identities found`): the SHA-1 is read from the plain listing, and the `-v` listing yields nothing |
| must-fail fixtures, round 5 (`Q:/scratch/omp/g18/r5/mustfail/`) | PASS — with the identity line's pattern anchored to the end of the line (so a trust error after the label hides the identity) the new case fails (`17 of 18 case(s) passed … the full output gave 0 identity line(s)`); with the SHA-1 group narrowed to lowercase, which `find-identity` never prints, it fails the same way (`17 of 18 …`); each run restored the repo copy (`git diff --stat` unchanged after both) |
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts typecheck.ts typecheck` | PASS — `0 type errors in tsconfig.json, desktop/web/tsconfig.json` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts docs-links.ts docsLinks` | PASS — `327 checked, 0 broken` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts secrets.ts secrets` | PASS — `775 files, 0 findings` |
| the tool's platform guard and argument path (`--arms self-signed --keychain login`) | PASS — exit 2, `this tool asserts a Keychain experiment, which is macOS-only (process.platform = win32)` — no arm ran, and nothing was written |
| Go checks (`gofmt -l`, `go vet`, `go build`, `go test`) | not run: this step changes no Go file (`harness/` and `docs/` only) |

What this round cannot prove on Windows: that a runner's `security find-identity -p codesigning`
listing holds an imported, untrusted, self-signed identity (the parser case is a fixture in the
form `identity_find.m` prints), and that `codesign --keychain <keychain> --sign <SHA-1>` signs with
it. Both are the self-signed jobs' answer in the next `keychain` run.

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
- A runner does not know the login keychain's password, and `security` takes it on argv only
  (`security.1:736-742`), so the login variant's partition-list writes carry an empty password and
  their exit is recorded. If the runner's login keychain has a non-empty password — likely, but the
  matrix run is what records it — the self-signed login arm stops before it signs (its
  `identity.partitionList.exit`) and an item that carries a partition entry gets a `skipped` allow
  instead of a written one; the experiment then records that this combination is not measurable on
  a GitHub runner, which is itself an answer about a self-signed identity.
  The login variant's deletes (`security delete-generic-password` per synthetic item,
  `security delete-identity -c <CN>` per import) are bounded at 30 s each so a prompt on them is
  killed and recorded rather than holding the job; an exit that is neither 0 nor "item absent"
  fails the run.
- One arm runs per job, so a `SecurityAgent` a blocked read left behind is recorded
  (`read.securityAgent`: the pids before the kill, that the kill was issued, the pids after it, and
  `respawned` — launchd restarts the on-demand daemon) and killed as cleanup; nothing that follows
  in the job depends on it being gone, and its presence never fails the run.
- macOS 27 has no runner and its Security source is not published; the citations are
  `Security-61901.120.67`, one macOS 26-era tag (`docs/plans/2026-11-22-0100-0018-keychain-survives-upgrade.md`,
  fact 2), and the record names the runner's `sw_vers`.
- The log scan covers the text the tool itself printed; the workflow's own `echo` lines (paths
  only) are not part of it.
- The dry runs' numbers are the orchestration's, not macOS's: their `security`, `codesign` and
  `hdiutil` output is written by stubs in the documented formats, so they prove the flow, the record
  and the guards, never what the real Keychain answers — and the round-4 run's `agentScan` path
  cannot exercise `read.securityAgent.found` on Windows, where no real SecurityAgent exists (the
  self-check's respawn case covers that pure comparison).
- The record is an artifact, not a published file: it holds the runner's own temp paths, and no
  secret, key byte or passphrase.
