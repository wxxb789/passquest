# 0018 — macOS upgrades keep the learner's Keychain permission: increment plan

Contract: [`../goals/0018-keychain-survives-upgrade.md`](../goals/0018-keychain-survives-upgrade.md).
Conventions: [`../conventions.md`](../conventions.md). Harness: [`../harness.md`](../harness.md).
Cross-platform rules: [`../cross-platform.md`](../cross-platform.md). Base: `goal/0018-keychain-survives-upgrade` at `963aa6f`.

## Goal in one sentence

An upgraded macOS package keeps the Keychain access the learner already allowed: before any fix, one recorded
macOS run establishes whether a key stored through the window by one ad-hoc build can be read by the next build
installed over it, and whether a project-owned self-signed identity — or any identity without an Apple-issued
team ID — changes that answer.

## Facts this plan rests on (scouted 2026-11-21; cited, not remembered)

1. **How the app stores and reads the key.** The store is `newCredentialStore(mygo.Secrets.Service(secretService(getenv)))`
   (`internal/desktop/desktop.go:234`); the service is `PASSQUEST_DESKTOP_SECRET_SERVICE`, else
   `com.github.wxxb789.passquest` (`internal/desktop/paths.go:18-19,28-29,173-178`), the account is `api-key:<provider>`
   (`internal/desktop/credentials.go:20`), and the three operations delegate to `mygo.Secrets`
   (`credentials.go:26-47`; `newCredentialStore` probes with a read and yields no store on `errors.ErrUnsupported`,
   `:55-60`). MyGo 0.10.0's `Secrets` is documented as "the login Keychain on macOS, the Secret Service on Linux …
   and the Credential Manager on Windows" (`secrets.go:19-21`) and warns "They may block while the system asks the
   user, e.g. to unlock the keyring or, on macOS, to let a rebuilt app read what it stored" (`secrets.go:44-47`).
   On darwin it is the Security framework, loaded with `dlopen`, calling `SecItemAdd`/`SecItemCopyMatching`/
   `SecItemUpdate`/`SecItemDelete` (`internal/darwin/secrets.go:29-49`, `:94`, `:123`, `:150`) against the
   **file-based login keychain**: "It uses the file-based keychain: the data protection one needs a signed app with a
   keychain-access-groups entitlement (unsigned builds get errSecMissingEntitlement)" (`internal/darwin/secrets.go:16-18`).
   `kSecUseDataProtectionKeychain` appears nowhere in the module; `/usr/bin/security` appears in the app nowhere —
   only the harness uses it, to delete the smoke's item (`harness/gates/desktop-smoke/settings.ts:25-33`). So the
   item is created by Pass Quest's own code signature; the premise's "the ACL trusts an Apple binary, not Pass Quest"
   variant does not apply to this app.
2. **What decides whether a different build may read the item.** From Apple's open source
   (`apple-oss-distributions/Security`, newest published tag `Security-61901.120.67`, commit
   `97c3a4296c1ea06b0fe1877a7e616aa84450b5b2`, fetched read-only with `gh api` on 2026-11-21; its `security.1` is
   cited from the same tag):
   - A new keychain item's ACL is the "any application" owner plus a **partition entry naming the storing process's
     partition ID** (`securityd/src/localkey.cpp:84-98`, the `createClientPartitionID` call at `:96`), and every item
     read validates it: `KeychainKey` is `LocalKey, SecurityServerAcl` (`securityd/src/kckey.h:51`), its constructor
     calls `setOwner` (`securityd/src/kckey.cpp:76-81`), and `KeychainKey::validate` runs
     `SecurityServerAcl::validate` (`kckey.cpp:212-220`) — which calls the partition check **beside** the owner-ACL
     check, not instead of it (`securityd/src/acls.cpp:121-147`, `validatePartition` at `:146`). Decrypting an item's
     data takes `CSSM_ACL_AUTHORIZATION_DECRYPT` on the item's own key (`securityd/src/localdatabase.cpp:128-132`),
     which is one of the read operations for which a prompt may be offered (`acls.cpp:133-146`); a partition list
     without the client's ID either prompts or throws `CSSM_ERRCODE_OPERATION_AUTH_DENIED` (`acls.cpp:159-200`, the
     throw at `:193`, the prompt path at `:216-245`).
   - What the learner's "Always Allow" records: the prompt subject's success block adds the requester's **code
     requirement** to the ACL's application list (`securityd/src/acl_keychain.cpp:100-120`, `addToStandardACL` at
     `:113` → `acls.cpp:316-321`) **and** its **partition ID** to the partition list (`:115` →
     `acls.cpp:408-421`). The application-list half is requirement-based (stable for a certificate-signed app); the
     partition half is whatever `partitionIdForProcess` returns.
   - What that ID is (`securityd/src/clientid.cpp:187-288`): Apple-signed → `apple:` (`:265`), `security(1)` itself
     → `apple-tool:` (`:263`), Mac App Store/TestFlight → `teamid:<embedded team>` (`:267-271`), Developer ID /
     Mac Development / Mac Distribution — all `anchor apple generic` — → `teamid:<OU of the signing certificate>`
     (`:273-281`), unsigned → `unsigned:` (`:251-252`), and **everything else, "cannot positively classify this
     code, but it's signed", → `"cdhash:" + hashString(cdhashData)` (`:282-286`)**.
     **Answer to the question this plan exists to settle: an ad-hoc-signed process and a self-signed-certificate
     process both land in that last branch.** A self-signed identity therefore gives a stable *designated
     requirement* and a per-build *partition*; the partition check runs on every read, so by this source a
     self-signed identity does **not** by itself keep access across builds. The experiment (P1) is what decides:
     the source is one tag of one OS version, macOS 27's is not published, and only a run shows which check refuses
     on the runner.
   - What is observable: `security dump-keychain [-adhir]` — `-a` "Dump access control list of items"
     (`SecurityTool/macOS/security.1:369-383`); it prints each ACL entry's authorizations including `partition_id`
     (`SecurityTool/macOS/keychain_utilities.c:377`), the partition list itself in that entry's `description:`
     (`:402`, `:1008-1049`), and every trusted application with its `requirement:` (`:410-460`); the dump copies
     attributes only, no data (`:538`), so it does not decrypt the secret. `set-generic-password-partition-list -S
     <list> -k <password>` is documented as: "The \"partition list\" is an extra parameter in the ACL which limits
     access to the item based on an application's code signature. You must present the keychain's password to change
     a partition list." (`security.1:736-742`); and for signing keys, "If you'd like to run /usr/bin/codesign with
     the key, \"apple:\" must be an element of the partition list." (`security.1:810`). `find-generic-password -g`
     and `-w` both "Display the password for the item found" (`:577-580`) — i.e. both withdraw the secret and take
     the same check, so neither is a neutral way to prove a key is present.
3. **What MyGo says and does about signing.** `cmd/mygo/devsign.go:25-30`, verbatim: "Its signature, unlike an ad hoc
   one, is the same from build to build: the designated requirement names the identifier and the certificate, not a
   hash of the code. macOS keeps what the user allowed the app, such as Accessibility or Screen Recording, and
   \"Always Allow\" for its Keychain items, by that requirement, so with ad hoc signing it takes every build for
   another app, asking again." A developer build without a certificate warns at `cmd/mygo/dev.go:243`: "Signing the
   development app ad hoc: macOS asks again for its permissions and Keychain items after each build. An Apple
   Development certificate keeps them; Xcode makes one for an Apple ID, free, in Settings > Accounts > Manage
   Certificates". A release build signs with `codesign --force --deep --sign <identity>`, adding `--options runtime
   --timestamp` only in production and only when the identity is not `-` (`cmd/mygo/bundle.go:376-389`), the app at
   `cmd/mygo/build.go:347`, the disk image at `cmd/mygo/dmg.go:126-127`; off macOS it prints "Not signing …: code
   signing needs macOS" and continues (`bundle.go:395-408`). The identity comes from `mygo.json`'s
   `macos.signingIdentity` (`cmd/mygo/config.go:211-215`; default `-`, `:590-591`) or the `-sign` flag, which wins
   (`build.go:71,97-99`); **no environment variable names a signing identity** (`MYGO_LDFLAGS` only appends linker
   flags, `build.go:552-556`). The designated requirement an ad-hoc signature carries is not stored at all —
   `drmaker.cpp:66-69` ("we can't make an explicit DR for a (proposed) ad-hoc signing") — so `codesign -d -r-`
   prints the **implied** one, and the man page says so: "If the code does not contain an explicit designated
   requirement, the implied one will be retrieved and written out as a source comment" (CODESIGN(1), section
   `-r, --requirements`, `https://keith.github.io/xcode-man-pages/codesign.1.html`, fetched 2026-11-21). For ad-hoc
   code that implied DR is "a cdhash requirement for all architectures" (`staticcode.cpp:2190-2208`,
   `maker.cdhash(...)`), rendered `cdhash H"…"` (`reqdumper.cpp:189-191`); for a certificate-signed, non-Apple
   anchored app it is `identifier "<bundle id>" and certificate root = H"<sha1 of the anchor certificate>"`
   (`drmaker.cpp:66-72` puts the identifier first, `:86-117` hashes the first certificate whose Organization differs
   from the leaf's — for a one-certificate chain that is the certificate itself; `reqmaker.cpp:92-97`;
   `reqdumper.cpp:159-161,270-283`). So the *requirement* moves with every ad-hoc build and stands still for a
   certificate-signed app — what that requirement is weighed against, and what sits beside it, is fact 2. This
   repository builds ad-hoc today: `mygo.json:1-14` declares no `macos.signingIdentity`, and
   `harness/lib/package-build.ts:201` runs `go tool mygo build -platform <goos>/<goarch>` with no `-sign`.
4. **What Pass Quest tests today.** The package smoke's `credential` case (`harness/gates/package-smoke.ts:117`,
   `:471-479`) runs the desktop smoke's `settings` scenario: it stores a fresh `pq-smoke-…` key through the window
   (`[data-act="credential-save"]`, then `[data-credential="present"]`, which is itself a read of the store —
   `harness/gates/desktop-smoke/settings.ts:39,48-68`), proves the value reached the loopback provider stub and not
   the environment ("the launch environment carries no key, so the provider can only have received it from the OS
   credential store", `:71-80`), proves no file under the data root, the launch's result document, the installed
   tree or the app's output holds the bytes (`:82-90`), and deletes the item with `security delete-generic-password`
   on macOS (`:25-33`). It runs on all three OSes; on macOS it keeps the real HOME because the login keychain is
   found through `~/Library` (`package-smoke.ts:473-477`; Q-0013-08 records why and names a temporary keychain as
   the way to reverse it). The launch contract is the smoke hook (`PASSQUEST_DESKTOP_SCRIPT`/`_RESULT`,
   `internal/desktop/smoke.go:19-27`; 40 s script, 10 s load grace, `:35-36`) under a hard 60 s process timeout that
   kills the tree and requires nothing to outlive the app (`harness/lib/desktop-page.ts:32,273-297,351-376`), with
   the page helpers' own deadlines at 15 s by default (`:73`). A refused read is invisible as a refusal in the
   window: `internal/workspace/workspace_settings.go:367-388` reports `problem: "the credential store could not be
   read"` and no key, so the row renders `[data-credential="missing"]` (`desktop/web/src/views/settings.ts:41-51`).
   Evidence that this works on a macOS runner: CI run 44467752612, `package smoke` on macOS arm64, 483 assertions
   over `contents, layout, study, agents, anki, upgrade, credential, broken`
   (`docs/evidence/0013-reliable-personal-desktop-distribution.md:21`); `docs/harness.md:41` states "macOS uses the
   login keychain".
   **Can a runner observe a prompt?** It has a window session (the window gates run there) and no one to click, so
   a needed prompt either blocks until the launch's 60 s kill or fails. Which one is not in the tree, and the source
   gives both shapes: the partition check throws when it cannot prompt (`acls.cpp:193`), mapping
   `CSSM_ERRCODE_OPERATION_AUTH_DENIED` → `errSecAuthFailed` (-25293) and `CSSM_ERRCODE_NO_USER_INTERACTION` →
   `errSecInteractionNotAllowed` (-25308) (`OSX/libsecurity_keychain/lib/SecBase.cpp:174-177`, the
   `CSSMERR_*_NO_USER_INTERACTION` group at `:460-466`). So the experiment records the outcome — read succeeded;
   refused promptly with the item still in the keychain; or blocked and killed — instead of assuming one.
5. **The experiment (P1) and what it must show.** Two packages built from two commits (A older, B the branch head)
   carry different ad-hoc cdhashes for free: Go's default VCS stamping (`-buildvcs=auto`, `AutoVCS: true` for
   `go build`, `src/cmd/go/internal/work/build.go:472`) puts the commit in the main module's version, so two commits
   are different binaries even when their sources are identical — measured here: `go build -trimpath -ldflags "-s -w"`
   followed by `go version -m` prints `mod github.com/wxxb789/passquest v0.15.1-0.20261121153003-963aa6f56d91+dirty`
   for this tree (`+dirty` is this uncommitted plan file; a clean checkout carries the revision alone). The tool
   asserts the two cdhashes differ, which is the property every real upgrade has. The identity arms re-sign the
   installed bundle in place with
   `codesign --force --deep --sign <identity>` (MyGo's argv without the production-only `--options runtime
   --timestamp`, `bundle.go:376-389`) rather than build two more packages; the P3 arm still proves MyGo's own
   `-sign` path. The experiment runs on a throwaway keychain of its own so that the job knows the password
   `set-generic-password-partition-list` demands (`security.1:736-742`) and can simulate the learner's allow, which
   no runner can click; Q-0013-08 already names that reversal.
6. **Where a real identity would plug in.** `harness/lib/package-build.ts:201` (the one mygo invocation every
   package goes through: `bun run package` and the package smoke's build), `mygo.json:1-14` (no signing key today).
   `.github/workflows/gate.yml:1-20` triggers on `pull_request`, `push: [main]` and `workflow_call`, keeps
   `permissions: contents: read`, and its matrix must stay exactly `ubuntu-latest, macos-latest, windows-latest` ×
   the four groups (`harness/lib/gate-groups.ts` refuses anything else), so identity steps join the existing macOS
   `package` job (60-minute timeout) rather than changing the matrix. `.github/workflows/release.yml:36-38` calls
   `uses: ./.github/workflows/gate.yml` without `secrets: inherit`, and `:41-73` attaches the `package-*` artifacts
   the gate uploaded — so the signed `.dmg` must be the gate job's own artifact. The only secret in the workflows
   today is cask.yml's deploy key, passed through one step's `env:` (`cask.yml:128`), which is the pattern to keep;
   `harness/gates/secrets.ts:5-16` fails the tree on credential-shaped strings and literal secret assignments, so no
   key, password or `.p12` may enter the repository. Windows and Linux share `package-build.ts`, so the sign flag
   must be added only for a darwin host; the contents scan and the credential case keep their existing assertions
   (`package-smoke.ts:14-40`).

## Defaults (append as Q-0018-01…; the last entry is Q-0017-09)

1. **The workflow.** `.github/workflows/keychain.yml`, job `experiment`, `runs-on: macos-26` (the floor 0017 pinned),
   `timeout-minutes: 90`. Triggers: `pull_request` with a `paths:` filter listing the tool's repo-local import
   closure, the workflow and the harness files it drives, plus `workflow_dispatch` (input `a-ref`, default the pinned
   commit `963aa6f`) after the merge — the 0017 lesson (amendment 14) stands: a dispatchable workflow exists only
   once its file is on the default branch, so pre-merge evidence comes from the pull request run. The job uploads
   its JSON record and log as the `keychain-record` artifact (30 days).
2. **The two builds.** A from the pull request's base sha (dispatch: the input), B from the pull request head; each
   built in its own checkout by that tree's own harness (`bun install --frozen-lockfile`, `bun run package`), because
   the package must be the one the tree ships. The tool fails unless the two apps' cdhashes differ. If a future run
   ever finds them equal, the documented fallback is a content divergence in B's scratch copy (`mygo.json`'s
   version), never a silently accepted experiment.
3. **Three arms in one run.** `adhoc` (today's package), `self-signed` (a throwaway identity created inside the job),
   and `project` (only when the two signing secrets are present — on a pull request from a fork they are not, and the
   arm records its skip by name). Each arm installs A, stores a key through the window, records the ACL and the DR,
   installs B over A in the same prefix, reads the key with a hard timeout, and records the outcome.
4. **The experiment's keychain is its own.** `security create-keychain -p <random>` in `RUNNER_TEMP`, made the user
   default and the sole user search-list entry, deleted in `always()`; the login keychain and its preferences are
   saved, restored in a `finally`, and both values are in the record. The standing package smoke keeps the login
   keychain (Q-0013-08).
5. **The simulated allow.** When B's read is refused, the job learns B's real partition ID from an item B stores,
   adds it with `security set-generic-password-partition-list -S "<A's>,<B's>" -k <keychain password> -s <service> -a
   api-key:<stub provider> <keychain>`, re-runs B's read (expected: success) and then runs a third build C — B's
   bundle with a bumped `CFBundleShortVersionString`, signed again the way its arm signs (ad-hoc, or the arm's
   certificate) — whose read is expected to be refused again. That is the difference between "a learner's allow
   records the app" and "it records the build".
6. **The identity (P2+).** Subject `CN=Pass Quest Project Signing, O=Pass Quest`, RSA 3072, SHA-256, 3650 days,
   `extendedKeyUsage=critical,codeSigning`, `basicConstraints=critical,CA:TRUE`, self-signed, created from an
   `openssl.cnf` (`-config`, not `-addext`: LibreSSL and OpenSSL both take the file form). The `.p12` and its
   password live only in the Actions secrets `PASSQUEST_MACOS_SIGNING_P12` (base64) and
   `PASSQUEST_MACOS_SIGNING_PASSWORD`; the public certificate, its SHA-256 fingerprint and its SHA-1 (the one the DR
   carries) are recorded in the docs and pinned in the harness.
7. **Where the identity enters the build.** Only in `harness/lib/package-build.ts`: for a darwin target on a darwin
   host, the mygo invocation gains `-sign <identity>` when the keychain holds it — the pinned CN from
   `harness/lib/signing.ts`, or `PASSQUEST_MACOS_SIGNING_IDENTITY` when set (the experiment's throwaway arm). The
   probe is `security find-certificate -c "<CN>"` (a bare `find-identity -v -p codesigning` may not list a
   self-signed certificate, so it is not used); without it the build stays ad-hoc and one line says so. `mygo.json`
   keeps no `macos.signingIdentity`, so MyGo's `-sign` (which wins over the config, `build.go:97-99`) is the single
   door, and Windows and Linux builds pass no flag and keep today's bytes.
8. **The pinned public facts.** The DR string the project signature must carry is pinned **exactly** as
   `codesign -d -r-` prints it (read from P1's record and the coordinator's certificate, never composed by hand),
   together with the SHA-256 fingerprint; the package smoke checks the installed app's DR against it whenever the run
   says the job signed the package (default 9), and the keychain job asserts that two builds carry the same DR
   (Done-when 2's first bullet).
9. **CI wiring (P3).** `gate.yml`'s macOS `package` job imports the `.p12` into a `RUNNER_TEMP` keychain
   (`security create-keychain`/`import`/`set-key-partition-list -S apple-tool:,apple: -s`/`default-keychain`/
   `list-keychains`, `security.1:810`), takes the secrets only through a step's `env:`, and reports
   `have=true|false` through `$GITHUB_OUTPUT` (never `if: secrets…`); the gate step receives that as
   `PASSQUEST_MACOS_SIGNING_EXPECTED`, so the smoke requires the pinned DR when the job signed and names the missing
   secret in a SKIP when it did not — a fork pull request still builds and passes as an ad-hoc package, and the run
   says which it was. The keychain and the decoded `.p12` are deleted in an `always()` step. `release.yml`'s `gate`
   job adds `secrets: inherit`.
10. **When the experiment says the premise is false, or that a self-signed identity cannot help.** P2–P4 do not run:
    the run is reported with its evidence, a Q-0018 entry records which check refused (with the partition values), and the
    next step belongs to the owner — an Apple-issued certificate (an Apple Development certificate from a free Apple
    ID already yields a `teamid:` partition and keeps today's Gatekeeper behaviour; MyGo's own warning says so,
    `cmd/mygo/dev.go:243`) or accepting a re-prompt per upgrade, which would be a contract change.

## Steps (one implementer run each; each leaves every gate green)

| # | Step | Proof |
|---|---|---|
| P1 | The experiment: `harness/tools/keychain-experiment.ts`, `.github/workflows/keychain.yml`, the 0018 evidence record | the pull request run's `keychain-record`: each arm's DRs, the item's ACL before and after, the read outcomes; the tool's own guards fail when the two cdhashes match or the item carries no partition entry |
| P2 | *Only if P1 shows the self-signed arm keeps access:* the identity signs the package and the tree pins the DR | the pinned check fails for an ad-hoc build (must-fail case) and passes with the identity; `typecheck`; CI's macOS package job with the secret |
| P3 | *Same condition:* the release path carries the identity | the keychain job's `project` arm: two builds, one DR, B reads A's item, no prompt; the gate job's log shows the keychain made and deleted and no material echoed |
| P4 | *Same condition:* docs, decisions, evidence | `install-guide` (both languages, the fingerprint pinned), `docs-links`, `secrets`; the owner's review |

### P1 — the experiment (facts 2, 4 and 5; nothing is fixed here)

`harness/tools/keychain-experiment.ts` (the cask tools' shape: a `bun`-run tool, no gate), taking `--a-root`,
`--b-root`, `--out` and `--arms`. It builds each root with `buildPackage` (`harness/lib/package-build.ts`) for the
host target, selects each `.dmg` by its exact name and records its SHA-256; then, per arm:

1. Install A from its `.dmg` into one temp prefix (`hdiutil attach` + `ditto`, the package smoke's own route,
   `harness/gates/package-smoke/install.ts:272-315`), re-sign the installed bundle in place when the arm has an
   identity, and record `codesign -d -r-`, `codesign -dvv`, `codesign -dvvv` and `sw_vers -productVersion`.
2. Launch A with the store script through the smoke hook (`launchPage`, `harness/lib/desktop-page.ts:273-297`): the
   settings page stores a fresh key and the script returns when `[data-credential="present"]`, so the key is in the
   OS store. Record the key's SHA-256 (never the key), the launch's result document and `security dump-keychain -a`
   for the item: its application list with each `requirement:` and its partition list.
3. Install B over the same prefix, re-sign it for the arm, record B's DR and cdhash, launch B with the read script —
   wait for `[data-credential="present"]`, then run the model test and require the loopback stub received
   `Bearer <key>` — under the launch contract's 60 s kill (`desktop-page.ts:32,351-376`); record the outcome as
   *read* (`present`, stub saw the key), *refused* (the row stays `missing` while the dump still holds the item) or
   *blocked* (the timeout/kill line), plus a fresh dump and a `ps` listing of any `SecurityAgent` left behind.
4. From a refused read, run the simulated allow (default 5) and the third build C, and record both outcomes.
5. Delete the throwaway keychain, restore the default keychain and the search list, and write the JSON record.

Guards the tool fails on, so the record can never be read as more than it is: the two builds' cdhashes differ; the
item exists after step 2 and its partition list is non-empty; the partition value the tool computes for A from
`codesign -dvvv` equals the value the dump shows for A (otherwise the simulated allow would use a value nobody
wants); no key byte reaches the record, the log or the filesystem; a `SecurityAgent` left by a blocked read fails
the arm before the next one starts.

`.github/workflows/keychain.yml` (defaults 1 and 2): two checkouts (`a/` at the resolved A ref, the head at the
root), `actions/setup-go`, `oven-sh/setup-bun`, `bun install --frozen-lockfile` in both, `bun run package` in both,
then the tool; `actions/upload-artifact` for the record with `if: always()`. The throwaway identity for the
`self-signed` arm is created in the job with the default-6 parameters, imported into the experiment's keychain with
`-T /usr/bin/codesign` and unlocked for codesign by `security set-key-partition-list -S apple-tool:,apple: -s`
(`security.1:810`), and the arm signs with its CN through `PASSQUEST_MACOS_SIGNING_IDENTITY` (default 7's override);
if signing fails, the arm records *could not sign* (with `security find-identity -p codesigning` and the `codesign`
error) instead of reporting a self-signed result, and the fallback is the certificate's SHA-1 as the identity. For
the `project` arm the run's step `env:` carries the two secrets and the tool does the import itself: the `.p12` is
base64-decoded to a `0600` file under its own temp tree, imported into the keychain the tool creates, and both are
deleted with it — the material never reaches a command line, a log line or the record.

P1 also creates `docs/evidence/0018-keychain-survives-upgrade.md` with the run id, the two commits, the two
`.dmg` SHA-256s, both arms' DRs and cdhashes, the item's ACL (applications and partition lists) and the outcomes —
each number read from the record artifact, not from prose.

**Stop conditions, before any fix is designed.** If the `adhoc` arm's read *succeeds*, the goal's premise is false:
stop, report the run, add no identity (Done-when 1's own wording). If the `self-signed` arm's read is *refused* or
*blocked*, a project-owned self-signed identity cannot meet Done-when 2, and the simulated allow (default 5) shows
why: the fix needs a partition ID macOS keeps stable, i.e. an Apple-issued certificate with a team ID, which the
goal's constraints exclude — stop and report, with the owner's decision (default 10) as the only way forward.

### P2 — the identity signs the package (only if P1 says a self-signed identity keeps access)

`harness/lib/signing.ts`: the pinned `IDENTITY` (the CN — the name `package-build.ts` signs with, and the same CN
the coordinator's certificate carries), the pinned DR string and the SHA-256 fingerprint, plus a pure
`signatureProblems(dr: string, expected: string): string[]` that rejects a DR with no `identifier "<bundle id>"`
clause, one naming another identifier, one containing `cdhash H"`, and one that is not byte-equal to the pinned
string. `harness/lib/package-build.ts` passes `-sign` per default 7. The package smoke's macOS path, when
`PASSQUEST_MACOS_SIGNING_EXPECTED` says the job signed the package, runs `codesign -d -r-` on the installed app and
fails on `signatureProblems` (`harness/gates/package-smoke/contents.ts` is the natural home: it already reads the
bundle and holds `PackageIdentity`). The self-test gains must-fail cases for the pure check (a cdhash DR, a wrong
identifier, an empty string) on all three OSes, and the macOS-only fixture case that proves the `codesign -d -r-`
invocation itself fails for an unsigned fixture bundle. `docs/harness.md` gains one clause in the package smoke's
row.

*Proof:* `mise exec -- bun Q:/scratch/pq-harness-run/run.ts typecheck.ts typecheck`;
`run.ts self-test.ts selfTest` (the new cases fail as designed); CI's macOS `package` job with the secret shows a DR
equal to the pinned one, and the same job without the secret (a fork-shaped run) still passes and says the package is
ad-hoc.

### P3 — the release path carries the identity (same condition)

`gate.yml`'s macOS `package` job gains, before the gate step, the keychain import (default 9: a `RUNNER_TEMP`
keychain, `security import … -T /usr/bin/codesign`, `set-key-partition-list -S apple-tool:,apple: -s`, added to the
search list and made default, all through `env:`), passes `have=true` to the gate step as
`PASSQUEST_MACOS_SIGNING_EXPECTED` (default 9), and deletes the keychain and the decoded `.p12` in `always()`;
`release.yml`'s `gate` job adds `secrets: inherit` (`:36-38`). The keychain workflow gains the `project` arm (P1's
tool, unchanged) so the released identity's answer to Done-when 2 is a standing job, and the tool asserts the
`project` arm's DR equals the pinned one.

*Proof:* the pull request run of `keychain.yml` (project arm: two builds, one DR, the read succeeding without a
prompt) and the branch's `gate` run on macOS (the DR check passing on the job's own package); the job log shows the
keychain created and deleted and echoes no secret; `secrets` passes.

### P4 — the learner is told, and the record is written (same condition)

`docs/install.md` and `docs/install.zh-CN.md` (0017 landed both) stop saying "ad-hoc signed only"
(`install.md:74`, `install.zh-CN.md:50`, and "Code signing and notarization. None." at `install.md:275`,
`install.zh-CN.md:183`): the app is signed with the project's own certificate, the SHA-256 fingerprint is given for
verification, Gatekeeper's first-launch flow is unchanged and what the identity does *not* mean (no Apple trust, no
notarization) is spelled out. The `install-guide` gate pins the fingerprint and the statement in both guides'
macOS sections, so one side cannot drift. `docs/evidence/0018-keychain-survives-upgrade.md` gains one row per
Done-when item with the run ids and the numbers read from the record; Q-0018-01…10 are appended; `docs/harness.md`
and `docs/cross-platform.md:109-112` are reconciled to the new posture.

*Proof:* `run.ts install-guide.ts installGuide`, `run.ts docs-links.ts docsLinks`, `run.ts secrets.ts secrets`; the
owner's review of the wording (Done-when 4).

## Done-when mapping

| Contract item | Steps | Evidence |
|---|---|---|
| 1 the problem is established first | P1 | the record's `adhoc` arm: the two `codesign -d -r-` outputs, the item's ACL, the read outcome |
| 2 upgrades keep Keychain access | P1, P2, P3 | two builds with one DR; B reads A's item with no prompt (the `project` arm); where a prompt cannot be clicked, the record says what was observed instead |
| 3 the private key cannot leak | P3, P4 | `secrets`; the contents scan unchanged; the job log shows the temp keychain deleted and no material; the public fingerprint in the docs |
| 4 the learner is told the truth | P4 | both guides plus the `install-guide` gate; the owner's review |
| constraints: no Developer ID or notarization, Windows and Linux untouched, where the key lives unchanged | P2 | the flag exists only on a darwin host; no Go path changed; the credential store's service and account stay as they are |

## Risks (each with how it is detected)

- **The premise is false** (an ad-hoc B reads A's item) → P1's `adhoc` arm records `present` plus the stub carrying
  the key; the run stops there and no identity is added.
- **A self-signed identity cannot help** (the partition half of the allow is per-build) → P1's `self-signed` arm is
  refused and the simulated allow's third build C is refused again, with the partition values quoted from the dumps;
  the plan stops and the owner decides (default 10).
- **The runner meets no prompt and fails silently** → the record distinguishes *refused* (`missing` with the item
  still in the dump) from *blocked* (the 60 s kill line), and `ps` names a left-behind `SecurityAgent`.
- **The throwaway identity cannot sign** (`codesign` cannot use an untrusted certificate) → the arm reports *could
  not sign* with the `codesign` error and `security find-identity -p codesigning` output, and tries the certificate's
  SHA-1 as the identity; a failed arm is never reported as a self-signed result.
- **`--timestamp` with a project identity fails in a release build** (MyGo adds it in production,
  `bundle.go:379-387`) → the P3 job's build log carries MyGo's own error; if the timestamp service refuses, no
  silent workaround: the record says so and the decision returns to the owner.
- **The experiment's keychain swap leaks into the runner** → the tool restores the default keychain and the search
  list in a `finally`, records both before and after, and `always()` deletes the throwaway keychain; a leftover
  shows up as a difference in the record.
- **A gate is weakened to make the run green** → the contents scan and the credential case keep every existing
  assertion; the DR check is added beside them and carries a must-fail case in the self-test.
- **The pinned DR drifts from the certificate** → the DR string is pinned exactly as `codesign -d -r-` prints it and
  the keychain job's `project` arm compares two real builds against it, so a wrong constant fails a run rather than
  a review.
- **macOS 27 differs from macOS 26** → the record names `sw_vers` and the CodeDirectory version; the citations are
  one published tag (`Security-61901.120.67`); macOS 27's source is not published and no runner carries it — a
  disclosed limit.

## Evidence plan

Local (Windows): `run.ts docs-links.ts docsLinks`, `run.ts secrets.ts secrets`, `run.ts typecheck.ts typecheck` (P2
changes TypeScript); P2's `run.ts self-test.ts selfTest`. Nothing Go changes; if any does, `gofmt -l internal cmd`,
`go vet ./...`, `go build ./...`, `go test ./internal/... -count=1`. CI: the branch's `keychain.yml` run (the
`keychain-record` artifact) for Done-when 1 and, after P2/P3, for Done-when 2; the branch's `gate` matrix for the
DR check on the job's own package; the run ids and counts go into the evidence record. Disclosed limits: the
experiment can never click a prompt, so it records the outcome a click would have changed; the identity arms re-sign
an installed bundle instead of building one, and P3's own job covers the build-time `-sign` path; macOS 27 has no
runner; the `adhoc` arm is the only one that runs without a secret.

## Coordinator actions needed

The P1 run needs none — it can run on the pull request as soon as the workflow file is on the branch. Everything
below belongs to the coordinator and is needed only if P1's stop conditions do not fire, before P3's proof can run.
**Not taken:** the goal is parked (amendment 19 dropped P2–P4; Q-0018-03 adds no identity and no secret), so this
block is kept only as the route a revisit would take — with a Developer ID, store it as the two secrets, add
`project` to the workflow's `arm` matrix, then run it by `workflow_dispatch`. The private key never leaves the
coordinator's shell and no local copy survives:

```sh
work="$(mktemp -d)" && cd "$work" && umask 077
cat > openssl.cnf <<'EOF'
[req]
distinguished_name = dn
x509_extensions = ext
prompt = no
[dn]
CN = Pass Quest Project Signing
O  = Pass Quest
[ext]
basicConstraints = critical, CA:TRUE
keyUsage = critical, digitalSignature
extendedKeyUsage = critical, codeSigning
subjectKeyIdentifier = hash
EOF
openssl req -x509 -newkey rsa:3072 -nodes -sha256 -days 3650 -config openssl.cnf \
  -keyout passquest-signing-key.pem -out passquest-signing-cert.pem
export P12_PASSWORD="$(openssl rand -base64 24 | tr -d '\n')"
openssl pkcs12 -export -inkey passquest-signing-key.pem -in passquest-signing-cert.pem \
  -name "Pass Quest Project Signing" -out passquest-signing.p12 -passout env:P12_PASSWORD
base64 < passquest-signing.p12 | tr -d '\n' | gh secret set PASSQUEST_MACOS_SIGNING_P12 --repo wxxb789/passquest
printf '%s' "$P12_PASSWORD" | gh secret set PASSQUEST_MACOS_SIGNING_PASSWORD --repo wxxb789/passquest
# No secret names the identity: P2 pins the CN above in harness/lib/signing.ts, and the certificate's CN must be
# exactly it. Public values for the tree and the docs (the DR the harness pins, and the fingerprint):
openssl x509 -in passquest-signing-cert.pem -noout -fingerprint -sha256
openssl x509 -in passquest-signing-cert.pem -outform DER | openssl dgst -sha1
shred -u passquest-signing-key.pem passquest-signing-cert.pem passquest-signing.p12
rm -rf "$work"
```

Then push the branch and let its pull request run. The DR the harness pins must be the string `codesign -d -r-`
prints for a build signed by this certificate — read from that run's record (the `project` arm), not composed here.
If `gh` cannot set the secrets, the goal stops and reports, per the contract. Nothing in this goal writes to any
remote other than the repository's own secrets.

## Facts each step hands to the next, read from artifacts

- **P1 → P2/P3:** the record JSON — both arms' outcomes (which of *read*/*refused*/*blocked* happened, with the
  partition values and the DRs quoted), whether the two DRs of an identity arm are byte-equal, whether a grant to one
  build survives the next (C's outcome), the runner's `sw_vers`, and the two `.dmg` SHA-256s.
- **P2 → P3:** the pinned DR string and fingerprint (cross-checked against the certificate and against P1's arms),
  and the fact that a build signed by MyGo's own `-sign` path prints exactly that DR.
- **P3 → P4:** the run ids, the `project` arm's two builds and read outcome, and the gate log lines that show the
  temp keychain created, used and deleted with no material echoed.
- **P4 → the goal close:** the run ids, the gate counts and the fingerprint, each read back from the named runs;
  nothing from memory.

## Amendments

One line per amendment, made where the step's row met the tree (owner's comment policy, 2026-11-22: source
comments reduced to the keep-list; the design intent they carried is recorded here).

1. **P1 the keychain and every import are passphrase-less.** `security` takes keychain passwords on argv only
   (`create-keychain -p`, `import -P`, `unlock-keychain -p`, `set-*-partition-list -k`) and this repository treats
   `security`/`openssl` argv as public, so no secret can be on a command line; the tool's file header keeps that
   reason, and the record's `keychain.password` field is unchanged: `empty (see the tool header)`.
2. **P1 the throwaway identity's CN must differ from the project identity's**, because the tool signs by CN
   (default 7).
3. **P1 B is checked out at the workspace root and A under `a/`, each with `fetch-depth: 2`.** The tool and the
   harness it runs live in B's tree; A is checked out by commit id so `go build`'s VCS stamping finds the commit
   exactly as the gate's package job does.
4. **P1 the self-check step runs before the two package builds**, so a broken dump parser fails in seconds, not
   after the builds; and the `paths:` filter also lists the experiment's semantic inputs (`internal/desktop/**`,
   the settings view), because a change to either can change the answer this run exists to establish.
5. **P1 an arm's failure does not stop the run.** `ArmFailure` is recorded and the next arm still runs so its
   evidence is kept; only a run-level condition (`StopRun`: the pair guard, a keychain that cannot be created, a
   SecurityAgent that survives its kill) stops the experiment early.
6. **P1 *blocked* needs a SecurityAgent sample.** A read that did not complete is *blocked* only when the
   one-probe-a-second `pgrep -lf SecurityAgent` sampler saw a SecurityAgent while the launch ran; otherwise it is
   *failed*, and a `failed` read fails the run.
7. **P1 every read that is not a success is still cleared and covered.** `settleSecurityAgent` and the simulated
   allow run for `refused`, `blocked` and `failed` alike: a prompt must be cleared before the next arm, and a
   `failed` read is red at the arm's end.
8. **P1 a build read after a re-sign must show a stored requirement.** `readSignature(…, explicitRequired=true)`
   fails the arm when codesign prints the implied form: the signature carries none, so the re-sign did not take.
9. **P1 a green run's premises.** The pair guard must have compared the two cdhashes, and only the project arm
   may skip (a fork pull request carries no project secrets); every other skip fails the run.
10. **P1 the launch's `check` records failure messages only, and classification never matches message text**: a
    message pushed for every launch would let whatever a later reader greps for turn a failed launch into a pass.
11. **P1 `tool()` pairs `loggedRun` with the harness's `cleanEnv`** so a `PASSQUEST_*` variable from the caller's
    shell cannot reach a child; `sha256File` streams (a dmg is tens of MB, not held in memory twice); and
    `observedHead`/`observedTail` split an assertion's head from a failure's tail, where the offending line sits
    (the 0017 lesson).
12. **P1 the item macOS 26 stores carries no partition entry (run 44928776617).** The ad-hoc item's ACL is
    `encrypt` (any application) + `decrypt derive export_clear export_wrapped mac sign` (one application, A's
    `requirement: cdhash H"b9bc…"`) + `change_acl` (none), so the no-partition guard stopped the arm before its
    read; a missing partition list is now the recorded fact `partitionEntry: false` with `partitions: []` and the
    arm reads on, the simulated allow (partition half only) is recorded as `allow: {"skipped": "no partition
    entry"}` when there is nothing to add, the classifier weighs the decrypt entry's application list (the
    `encrypt` entry's any-application list no longer counts as the item's verdict), and `read.bInDecryptAcl`
    records whether B's designated requirement matches a `requirement:` in that list.
13. **P1 the throwaway p12 must be legacy-encrypted and carry a passphrase (supersedes amendment 1's "every
    import are passphrase-less").** `security import … -P ""` failed (`SecKeychainItemImport: The user name or
    passphrase you entered is not correct.`) because the runner's `openssl` is OpenSSL 4.1.0 (the macos-26-arm64
    image's readme), whose PKCS#12 default is AES-256/PBKDF2 (`openssl-pkcs12(1)`, fetched 2026-11-22); the
    workflow prints `openssl version` and exports with `-legacy` for OpenSSL 3+ or the explicit `-keypbe
    PBE-SHA1-3DES -certpbe PBE-SHA1-3DES -macalg sha1` (OpenBSD's `pkcs12.c`, LibreSSL's app, takes the same
    three options), with a random non-empty passphrase handed to the tool through the
    step's output and the run step's `env:`; if the chosen export fails (measured locally on OpenSSL 3.5.9:
    `-legacy` needs the legacy provider module, `pkcs12: unable to load provider legacy`), the step retries
    once with the explicit flags, which the same build accepts without that provider. `security` takes that
    passphrase on `-P` argv only (a platform
    limit), so the tool builds that one log line with the passphrase replaced and scans its record and log for the
    value; the experiment keychain and the `-T /usr/bin/codesign` and `set-key-partition-list` steps are unchanged.
14. **P1 the tool resolves its temp path once.** `security` prints the keychain's resolved path
    (`/private/var/folders/…` for `os.tmpdir()`'s `/var/folders/…`), which failed the "user default and sole
    search entry" check although the swap had taken (run 44928776617); `realpathSync` on the mkdtemp result makes
    the record and every comparison carry one form.
15. **P1 the experiment runs one arm against one keychain per job (run 44931358398's finding).** That run's
    ad-hoc read landed *blocked* — a SecurityAgent was up in 20 of 23 probes, the window never rendered the
    provider row, the item carried no partition entry and refuses by application — and the arm guard stopped the
    run before the self-signed arm. The workflow is now a `fail-fast: false` matrix `arm: [adhoc, self-signed]` ×
    `keychain: [experiment, login]`, each job calls the tool with exactly one `--arms <arm>` and one
    `--keychain <mode>` and uploads `keychain-record-<arm>-<keychain>`; a SecurityAgent a blocked read left is
    recorded (pids before and after the kill, `read.securityAgent.respawned`) and killed as cleanup instead of
    stopping anything, because nothing follows in a one-arm job; and `codesign` and the finally's deletes are
    bounded, so a Keychain prompt nobody can answer can never hold a job to its own 90-minute timeout (a
    cancelled job uploads no artifact). The simulated allow and the third build C keep their amendment-7 rule
    (they run for every read that was not a success), and the allow is skipped, with the write's exit recorded,
    where a keychain password the run does not hold is needed.
16. **P1 the login variant stores where a learner does.** It creates and switches nothing: it records
    `security default-keychain -d user` and `security list-keychains -d user` first, checks the default is the
    login keychain (`keychain.variant: login`), the app stores into it, and the finally deletes every synthetic
    item (`security delete-generic-password -s <service> -a <account>`) and every throwaway identity the arm
    imported into it (`security delete-identity -c <CN>`, run in both variants; in the experiment variant the
    keychain's own deletion is what removes it), with the exits in `keychain.loginCleanup` and
    `keychain.identityCleanup`. Both partition-list writes the login variant needs — the identity's
    `set-key-partition-list` and the allow's `set-generic-password-partition-list` — carry an empty password
    because the login keychain's real one is not available to a run; a failed identity write stops that arm
    before it signs (the record names the man page reason: `security.1:810`, `:736-742`) rather than letting
    `codesign` wait on a prompt, and a failed allow write is recorded as the skip's reason. On a GitHub runner
    whose login keychain has a non-empty password the login×self-signed combination can therefore be
    unmeasurable; the record states which happened, and that is the answer the login variant exists to give.
17. **P1 the identity is judged by `security find-identity` without `-v` and picked by the SHA-1 that output carries
    (run 44983246217).** Both self-signed jobs stopped at the import check's `security import exited 0 or imported no
    identity: (no output)`: `security import -f pkcs12` prints nothing whatever it imports, because `SecPKCS12Import`
    returns dictionaries, not identities, so `keychain_import.c`'s identity/key/certificate counters
    (`SecurityTool/macOS/keychain_import.c:152-201`, tag `Security-61901.120.67`) stay zero and the count the check
    parsed can never be nonzero; and `-v` "Show valid identities only (default is to show all identities)"
    (`security.1:896`) hides an identity whose trust evaluation fails, which is every self-signed one. The import check
    now requires exit 0 alone, the arm requires the CN to appear in the plain `find-identity -p codesigning` listing
    (`identity_find.m:198-259,352-390`, whose line carries the certificate's SHA-1 and any trust error), both that
    listing and the `-v` one are logged in full, and the re-sign signs with that SHA-1
    (`codesign --force --deep --sign <SHA-1> --keychain <keychain>`) so an ambiguous CN cannot pick another identity,
    with the CN kept as the retry; every codesign attempt and its output are recorded in the arm's `signs`, and a
    failed sign prints the last 60 output lines. `security find-certificate -a -Z`, whose only purpose was the SHA-1,
    is gone.
18. **P1 the identity signs from a keychain of its own (run 44986005367's finding).** That run's `login` job imported the
    throwaway identity into the login keychain, where `security set-key-partition-list -S apple-tool:,apple: -s -k ""`
    exited 1 — the login keychain's password is not available to a run — so `codesign` could not use the key and the arm
    stopped before its read (`identity.partitionList.exit: 1`, `arms[0].read.outcome: not-run`); its `experiment` job
    signed and read fine (`partitionList.exit: 0`, `read.outcome: read`, `drSame: true`). The signing identity never needs
    to live in the keychain the app stores its credential in: every identity arm now imports into a temporary signing
    keychain this run creates with an empty password (`keychain.signing`), adds it to the user search list for each
    `codesign` call alone (each addition and its restoration are recorded in `keychain.signing.uses` and logged, and both
    variants end with the default keychain and the search list as they began), signs with the identity's SHA-1 through
    `codesign --force --deep --sign <SHA-1> --keychain <signing keychain>`, and deletes the keychain — identity and key
    with it — in the finally (the workflow's `always()` backstop deletes a leftover keychain of either name). The arm
    records `signingKeychain: "temporary" | "none"` and `credentialKeychain: "<experiment|login>"`, and two checks hold
    the split: after the import, `security find-identity -p codesigning` on the app's keychain must list nothing this arm
    imported (`identity <arm>: the app's keychain holds nothing this arm imported`), and the login variant asserts the
    app's credential keychain is the runner's login keychain as `security default-keychain -d user` prints it.
    `--keychain` now chooses only where the app stores and reads its credential; the simulated allow's
    `set-generic-password-partition-list` remains the one partition-list write a login-mode run cannot make, and its exit
    stays that skip's reason.
19. **P2–P4 are dropped by the goal's second stop condition (owner's decision, 2026-11-22).** The decisive run is
    44994402573 on commit 5dd680b — four jobs, all green: adhoc × experiment `blocked` / `refusesBy: application` with no
     partition entry; adhoc × login `blocked` / `refusesBy: both`, partition `cdhash:b9bc…` not admitting
     `cdhash:f9ea…`; self-signed × experiment `read` with `drSame: true` but an item that carries no partition entry;
     self-signed × login `blocked` / `refusesBy: partition` with `requirementMatch: exact` — partition `cdhash:0734…` not
     admitting `cdhash:20ca…` even though both builds' designated requirement (identifier
     `com.github.wxxb789.passquest`, certificate root `H"8ca4ef16…"`) is byte-equal. A self-signed identity therefore
     cannot meet Done-when 2 (`securityd/src/clientid.cpp` in `Security-61901.120.67:282-286` is why: non-team-ID signed
     code gets a `cdhash:` partition), so the owner decided to record and park: no identity is added, no secret is
     created, and the package and release path are unchanged. The plan's defaults 3, 6, 7, 8 and 9 are **not taken**;
     default 10 is taken, with the guides disclosing the re-prompt. Q-0018-02…04 and
     [`../evidence/0018-keychain-survives-upgrade.md`](../evidence/0018-keychain-survives-upgrade.md) carry the values.
20. **The workflow's `pull_request` trigger is narrowed to the parked scope (coordinator, 2026-11-22).**
    `.github/workflows/keychain.yml` now runs on `pull_request` only for changes to the workflow itself,
    `harness/tools/keychain-experiment.ts` or that tool's repo-local import closure (`harness/lib/{appimage,desktop,
    desktop-page,go,hash,json,notices,package,package-build,process,repo,types}.ts`), plus `workflow_dispatch`;
    `mygo.json`, `go.mod`, `go.sum`, `package.json`, `bun.lock`, `internal/desktop/**` and
    `desktop/web/src/views/settings.ts` are out of the filter (supersedes amendment 4's semantic inputs) — four macOS
    jobs of two package builds on every desktop change is too costly for a parked goal. Re-measuring a product change is
    a `workflow_dispatch` run; Q-0018-04 keeps that as the revisit path, and the `project` arm that would finish
    Done-when 2 is **not** in the matrix today (`arm: [adhoc, self-signed]`, and no step passes the two signing
    secrets): with a Developer ID, store it as the two secrets, add `project` to the `arm` matrix, then run the
    workflow by `workflow_dispatch`.

## Status

This file is the plan for goal 0018. P1 ran (run 44994402573 closed it), the second stop condition fired, and
P2–P4 are dropped by the owner's decision of 2026-11-22: record and park until a Developer ID exists (amendments
19 and 20; Q-0018-02…04). The decision's evidence is
[`../evidence/0018-keychain-survives-upgrade.md`](../evidence/0018-keychain-survives-upgrade.md).

STATUS: DONE
