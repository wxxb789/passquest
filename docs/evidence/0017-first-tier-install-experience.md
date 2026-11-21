# 0017 — Install on current Windows and macOS without fighting the operating system: evidence

Contract: [`../goals/0017-first-tier-install-experience.md`](../goals/0017-first-tier-install-experience.md).
Plan: [`../plans/2026-11-21-1000-0017-first-tier-install.md`](../plans/2026-11-21-1000-0017-first-tier-install.md).
Guides: [`../install.md`](../install.md), [`../install.zh-CN.md`](../install.zh-CN.md).
Decisions: Q-0017-01…09 in [`../decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md).

Log lines below are quoted from the named runs' logs (`gh run view <id> --log`), with the
timestamp/step prefix and ANSI colour escapes removed and nothing else changed, except that a
quote's trailing progress noise (a trailing `==> Caveats` and similar) is dropped and a dropped
middle is marked with `…`; adjacent log lines inside one quoted block are joined with ` | `. Where
a number is read from an artifact rather than a log, the artifact is named.

## Environment

Windows 11 Enterprise (10.0.26300), 16 logical CPUs, Go 1.27.4, Bun 1.4.5, mise Node 22.23.4 for the
harness. This machine was at **100% CPU**
(`(Get-CimInstance Win32_Processor).LoadPercentage`) while this step ran; every local gate below is
cheap text work, and no local run of a window gate is claimed.

macOS and Linux were not run locally. The macOS-specific claims (the built `.app`'s `Info.plist`, the
tap install, `brew`) are proven only by the CI runs below on `macos-26` runners, and the Linux and
Windows package jobs prove their own platforms. CI has no macOS 27 runner: macOS 27 is documented
from Apple's list, never run (the contract's own known limit).

## The runs this record cites

| Run | Workflow | Commit | Result | What it is evidence for |
|---|---|---|---|---|
| `44905438010` | `cask` | `5bfd872` | failure: 16/17 assertions | the first tap install; the `~/Library` listing; the audit failure with the tail not captured |
| `44909338098` | `cask` | `aa4f25d` | failure: tap mode 20/20, quarantine mutant 7/10 | the app in `/Applications`, the measured zap list, the audit passing, and the ambiguous-uninstall failure the next commit fixed |
| `44910568731` | `cask` | `3af7baa` | failure: 19/20 assertions | Homebrew 7.0.14 refusing to load the tap for the audit as untrusted — why the explicit `brew trust` exists |
| `44912433442` | `cask` | `041aa6a` | **success** | the final tree: tap mode 21/21 and the three mutants 10/10 each |
| `44905437929` | `gate` | `5bfd872` | **success**, 12/12 jobs | the branch's gate matrix at the first tap-install tree |
| `44909338152` | `gate` | `aa4f25d` | **success**, 12/12 jobs | the gate matrix at the measured-zap tree |
| `44912433474` | `gate` | `041aa6a` | **success**, 12/12 jobs | the final tree: `Info.plist` minimum, guides' gate counts, self-test numbers, secrets |

The release the cask follows is the published **v0.15.0** (2026-11-20T06:50:53Z), asset
`pass-quest-0.15.0-arm64.dmg`, 44,963,840 B, SHA-256
`477938dc1635971e23b2de952e04c684a87eda5c29fece7508d8b42c787b3e66`; the dry run below read that
digest from the asset and the release's own `SHA256SUMS.txt` and the two agreed.

## Done when 1 — the supported systems are what the package and the docs say

`mygo.json` declares `"macos": { "minimumSystemVersion": "26.0" }` (line 10), and MyGo copies that
value verbatim into the bundle's `LSMinimumSystemVersion`. The effective floor is the higher of that
key and the Mach-O's `minos` — the Go 1.27 linker still writes `minos 13.0`, below 26.0 — so the key
binds and raising `minos` is not needed (amendment 3; the comment in `TestPackagingIdentity`).

| Reading | Command / run | Value |
|---|---|---|
| the declaration | `mygo.json` | `"minimumSystemVersion": "26.0"` |
| the test pinning it | `mise exec -- go test ./internal/desktop/ -run TestPackagingIdentity -count=1 -v` (this machine, P5) | `--- PASS: TestPackagingIdentity (0.16s)`; `ok github.com/wxxb789/passquest/internal/desktop 0.443s` |
| the built `.app`'s own plist, macOS | run `44912433474`, job `gate (macos-latest, package)` pass line | `contents scan: 11 files, 133 MB, … Contents/Info.plist declares LSMinimumSystemVersion 26.0, 5.1s` (the scan fails unless the plist equals the identity's minimum) |
| the check bites | `harness/gates/self-test/package-smoke.cases.ts` cases, run `44912433474`'s three self-test jobs | must-fail case `installed bundle whose Info.plist kept an older minimum than mygo.json declares` (the failure names both `"13.0"` and `"26.0"`), accepted cases `installed bundle whose Info.plist declares the minimum mygo.json states` and `installed bundle whose nested app Info.plist kept an older minimum` |
| the docs' claim | both guides, `install-guide` gate, run `44912433474` | `PASS  install guide (7 sections, 4 code blocks and 7 screenshot places agree; 6 links and 5 URLs agree; 6 quoted app strings match their sources; the release notes name the tap command and both guides; the cask template renders v0.15.0 with the release's SHA-256, and a synthetic pair, into Casks/pass-quest.rb; floor 26.0 from mygo.json)` |
| the supported pair | `docs/install.md`, both languages; `docs/cross-platform.md`; `docs/desktop.md` | Windows 11+ (Windows 10 unsupported/untested, not blocked), macOS 26 and 27 the latest two majors from [Apple's list](https://support.apple.com/en-us/109033), Linux second-tier; the `.dmg` is arm64-only |

The Windows and Linux packages carry no `Info.plist`; the self-test's plist cases run in all three
OSes' `self-test` jobs, so the assertion is exercised off macOS too.

## Done when 2 — the guide matches what a learner sees

The English guide was rewritten for macOS 26/27 (try to open, then **System Settings → Privacy &
Security → Open Anyway**, password; why the app goes to `/Applications` first; once per download;
the `xattr -dr com.apple.quarantine` alternative; no system-wide Gatekeeper disabling) and the
Chinese guide carries the same facts beside it. Both READMEs link their own language's guide and the
release-notes template names the Homebrew command and both guides.

| Reading | Run | Value |
|---|---|---|
| the gate on all three OSes | `44912433474`, the three `core` jobs | `PASS  install guide (7 sections, 4 code blocks and 7 screenshot places agree; 6 links and 5 URLs agree; 6 quoted app strings match their sources; …)` |
| links in the working tree | `44912433474`, the three `core` jobs | `PASS  docs links (313 checked, 0 broken)` |
| the screenshot places | the same pass line + both guides | 7 places, each an HTML comment plus an italic caption, in both languages, and the counts must agree |
| the broken cases | `harness/gates/self-test/install-guide.cases.ts`, the `self-test` jobs | a guide missing the Open Anyway step, a guide naming macOS 13, notes without the Homebrew line, and the Chinese guide's link to the English one each fail the gate; the accepted pair passes |

**Pending the owner:** the screenshots themselves (7 places, marked in both guides) and the owner's
review of the wording. Nothing in this branch blocks on either (the goal records both as the owner's;
plan default 7).

## Done when 3 — Homebrew installs without Gatekeeper

### The final tap install (`44912433442`, `041aa6a`, job `install from the tap (macOS)`)

macOS 26.7.2, Homebrew 7.0.11 (printed by the job's own assertions). Selected lines, in order:

```
[assert] macos: pass — sw_vers -productVersion -> exit 0: 26.7.2 (the cask declares macOS 26)
[assert] brew: pass — brew --version -> exit 0: Homebrew 7.0.11
[assert] tap-clone: pass — git clone --depth 1 https://github.com/wxxb789/homebrew-tap.git -> exit 0
[tap] Casks/pass-quest.rb: version 0.15.0, sha256 477938dc1635971e23b2de952e04c684a87eda5c29fece7508d8b42c787b3e66, sha256(file) 760e3ef87e84660e61b398b059de75cd08f46a7efc640f10ddd1842653c1564d
[tap] render of .github/homebrew-cask-template.rb with those values: sha256 760e3ef87e84660e61b398b059de75cd08f46a7efc640f10ddd1842653c1564d
[assert] tap-render: pass — the tap file is byte-identical to the render (version 0.15.0, sha256 760e3ef8…564d)
[assert] install: pass — brew install --cask wxxb789/tap/pass-quest -> exit 0: ==> Installing Cask pass-quest | ==> Moving App 'Pass Quest.app' to '/Applications/Pass Quest.app' | 🍺  pass-quest was successfully installed!
[assert] app-path: pass — the install printed "Moving App 'Pass Quest.app' to '/Applications/Pass Quest.app'"
[assert] quarantine-app: pass — xattr -p com.apple.quarantine "/Applications/Pass Quest.app" -> exit 1: xattr: /Applications/Pass Quest.app: No such xattr: com.apple.quarantine
[assert] quarantine-nested: pass — xattr -p com.apple.quarantine "/Applications/Pass Quest.app/Contents/MacOS/Pass Quest" -> exit 1: … No such xattr: com.apple.quarantine
[assert] boot: pass — the smoke hook returned body text 420 characters (> 200), 1 stage children, 0 error bar(s), ready=true
[listing] before the uninstall — $HOME/Library, maxdepth 2, names matching *passquest* or *pass quest*: 2 entr(y|ies)
[listing]   ~/Library/Caches/com.github.wxxb789.passquest
[listing]   ~/Library/WebKit/com.github.wxxb789.passquest
[assert] data-marker: pass — /Users/runner/Library/Application Support/Pass Quest holds cask-install-marker.txt before the uninstall
[assert] quit-precondition: pass — the app is running before the uninstall (pid 4445, state alive)
[assert] uninstall: pass — brew uninstall --zap --cask wxxb789/tap/pass-quest -> exit 0: ==> Zapping Cask pass-quest | ==> Quitting application 'com.github.wxxb789.passquest'... | Application 'com.github.wxxb789.passquest' quit successfully. | ==> Backing up App 'Pass Quest.app' to '/opt/homebrew/Caskroom/pass-quest/0.15.0/Pass Quest.app' | ==> Removing App '/Applications/Pass Quest.app' | ==> Trashing files:
[assert] quit-log: pass — Application 'com.github.wxxb789.passquest' quit successfully. — from the uninstall output
[assert] quit-exit: pass — the app process exited within 30s of the uninstall (the watchdog did not kill it)
[assert] bundle-gone: pass — ls -d "/Applications/Pass Quest.app" -> exit 1: ls: /Applications/Pass Quest.app: No such file or directory
[assert] data-gone: pass — ls -d "/Users/runner/Library/Application Support/Pass Quest" -> exit 1: … No such file or directory (the marker was inside)
[state] the installed cask file's zap trash: names 3 path(s): ~/Library/Application Support/Pass Quest, ~/Library/Caches/com.github.wxxb789.passquest, ~/Library/WebKit/com.github.wxxb789.passquest
[assert] zap-gone ~/Library/Application Support/Pass Quest: pass — ls -d … -> exit 1 …
[assert] zap-gone ~/Library/Caches/com.github.wxxb789.passquest: pass — ls -d … -> exit 1 …
[assert] zap-gone ~/Library/WebKit/com.github.wxxb789.passquest: pass — ls -d … -> exit 1 …
[listing] after the uninstall — $HOME/Library, maxdepth 2, names matching *passquest* or *pass quest*: 0 entr(y|ies)
[assert] trust: pass — brew trust --cask wxxb789/tap/pass-quest -> exit 0: Already trusted cask: wxxb789/tap/pass-quest
[assert] audit: pass — brew audit --cask --strict wxxb789/tap/pass-quest -> exit 0
[summary] tap mode: 21 assertion(s) passed, 0 failed
```

In run `44912433442` the tap file and the tree's render both hashed
`760e3ef87e84660e61b398b059de75cd08f46a7efc640f10ddd1842653c1564d`. **Dated note (final apply run,
2026-11-21):** that run's tree is not this one — the template's header comment was reworded (the
simplify review's item 14), so this tree's render and pinned golden are now
`578f57a7b2232488874a5bdf638e4fb08f0f594404157c3aeb3f09d6fb01b4b6` and the tap must be re-seeded
before `install`'s `tap-render` assertion can pass again (the command is in the plan's "Coordinator
actions needed"). The `trust` assertion exists because Homebrew 7.0.14 refuses an
untrusted tap load for the audit; the tap file's `zap trash:` is read back out of the *installed*
cask file, not from a list in the tool, so the assertion cannot drift from the cask.

**The fully qualified name.** The learner's install names the tap fully
(`brew install --cask wxxb789/tap/pass-quest`), which is what carries Homebrew's item-level trust;
and the guides' paragraph about this workflow names the uninstall fully qualified too
(`brew uninstall --zap --cask wxxb789/tap/pass-quest`), the command the job runs: the short
`pass-quest` resolves only while one tap carries the cask. The job's own calls are all fully
qualified because the runner also carries the scratch mutant tap (`passquest/mutant`): run
`44909338098` failed exactly there — `Cask pass-quest exists in multiple taps`.

### The three mutant runs (same job, same run)

Each derives the cask from the tap's own file, writes it into the scratch tap `passquest/mutant`
(`brew tap-new --no-git`), installs it **by name** (current Homebrew cannot install a cask from a
path) and fails unless the matching assertion catches the removal.

| Mutant | Summary | The must-catch line |
|---|---|---|
| `quarantine` (no `postflight_steps`) | `[summary] mutants quarantine: 10 assertion(s) passed, 0 failed` | `[assert] mutant-quarantine: pass — xattr -p com.apple.quarantine "/Applications/Pass Quest.app" -> exit 0: 0181;6b01a61e;Homebrew\x20Cask;0C968A64-… — the attribute postflight_steps removes is still there` |
| `zap` (no `zap trash:`) | `[summary] mutants zap: 10 assertion(s) passed, 0 failed` | `[assert] mutant-zap: pass — ls -d "/Users/runner/Library/Application Support/Pass Quest" -> exit 0: …; cask-install-marker.txt is still inside` |
| `quit` (no `uninstall quit:`) | `[summary] mutants quit: 10 assertion(s) passed, 0 failed` | `[assert] mutant-quit: pass — pid 8955 is still running 15s after the uninstall; the log carries no quit line` |

### The rounds before it (disclosed, because the tap is public history)

| Run | Commit | Reading |
|---|---|---|
| `44905438010` | `5bfd872` | `[summary] tap mode: 16 assertion(s) passed, 1 failed`; the failed assertion is the audit, and its tail was not captured — the tool kept 12 lines of the output (`[assert] audit: FAIL — brew audit --cask --strict wxxb789/tap/pass-quest exited 1: Warning: audit is a developer command … Fetching gem metadata from https://rubygems.org/…`). The `~/Library` listing measured the two paths above; the app was asserted at `brew list --cask`'s `/opt/homebrew/Caskroom/pass-quest/0.15.0/Pass Quest.app` (`[assert] app-installed: pass`), a symlink to the real `/Applications` copy (`cask/artifact/moved.rb` `post_move`), which the next round corrected to the install's own `Moving App … to …` line |
| `44909338098` | `aa4f25d` | `[summary] tap mode: 20 assertion(s) passed, 0 failed` on macOS 26.7.2 / Homebrew 7.0.11, including `[assert] audit: pass`; the quarantine mutant showed the attribute survives without `postflight_steps` — `[assert] mutant-quarantine: pass — xattr -p com.apple.quarantine "/Applications/Pass Quest.app" -> exit 0: 0181;6b019d6c;Homebrew\x20Cask;62B5486E-…` — and then failed on the uninstall: `[summary] mutants quarantine: 7 assertion(s) passed, 3 failed` with `[assert] uninstall: FAIL — … ::error::Cask pass-quest exists in multiple taps: * passquest/mutant/pass-quest * wxxb789/tap/pass-quest` — the next commit gives every `brew` call the fully qualified name |
| `44910568731` | `3af7baa` | macOS 26.7.2 / Homebrew 7.0.14: `[summary] tap mode: 19 assertion(s) passed, 1 failed`; the audit's tail now captured the offence: ``::error::Refusing to load cask wxxb789/tap/pass-quest from untrusted tap wxxb789/tap. Run `brew trust --cask wxxb789/tap/pass-quest` or `brew trust wxxb789/tap` to trust it.`` — the reason `cask-install.ts` runs `brew trust --cask` before the strict audit |

The two earlier gate runs (`44905437929` at `5bfd872`, `44909338152` at `aa4f25d`) are 12/12 jobs
green; the final tree's gate matrix is `44912433474`.

## Done when 4 — the tap follows releases on its own

### The dry run (`44912433442`, job `update the tap cask (dry run unless a release was published)`)

Event `pull_request` → `cask: event=pull_request tag=v0.15.0 push=false`, then:

```
cask-publish: wxxb789/passquest release v0.15.0 -> wxxb789/homebrew-tap (dry run) | [run] gh release download v0.15.0 --repo wxxb789/passquest --pattern pass-quest-0.15.0-arm64.dmg --pattern SHA256SUMS.txt --dir /tmp/passquest-cask-…/release | [run] exit 0 | [release] pass-quest-0.15.0-arm64.dmg (44963840 bytes) sha256 477938dc1635971e23b2de952e04c684a87eda5c29fece7508d8b42c787b3e66 | [release] v0.15.0's SHA256SUMS.txt states the same digest | [render] .github/homebrew-cask-template.rb with v0.15.0 -> /tmp/passquest-cask-…/pass-quest.rb | [render] version 0.15.0, sha256 477938dc…b3e66, rendered sha256 760e3ef87e84660e61b398b059de75cd08f46a7efc640f10ddd1842653c1564d | [render] equals the pinned render harness/gates/testdata/pass-quest-0.15.0.rb byte for byte | [push] skipped: dry run (pass --push to commit the render to the tap)
```

The digest the tool computes from the downloaded asset and the release's own `SHA256SUMS.txt` line
agree, and the render equals the committed golden byte for byte — the same golden the `install-guide`
gate pins on all three OSes.

### The triggers (read from `.github/workflows/cask.yml`)

| Event | Jobs | Notes |
|---|---|---|
| `pull_request` with a `paths:` filter (the workflow, the template, `harness/lib/cask.ts` and the two tools' repo-local import closure, the golden, `mygo.json`) | `update` dry run (no secret in its environment), `install` (real tap install + mutants) | the pre-merge evidence; `workflow_dispatch` cannot start a workflow that only exists on a branch |
| `release: [published]`, guarded `!draft && !prerelease` | `update` pushes, `install` verifies | drafts trigger nothing (GitHub's event semantics) and the guard refuses them anyway |
| `workflow_dispatch` (`tag` default `v0.15.0`, `push` default `false`) | both | after the merge: `gh workflow run cask.yml -f tag=v0.15.0 -f push=false` (dry) or `push=true` (write) |

No weekly `schedule`: the goal does not ask for one, and a template change re-runs the workflow
through the `pull_request` path (amendment 14). The tag reaches the tool through `env:`, and both
the step and `harness/tools/cask-publish.ts` require `^v[0-9]+\.[0-9]+\.[0-9]+$` (amendment 21).

### The credential, and the tap's history

The tap `wxxb789/homebrew-tap` is public; its write credential is one deploy key, title
`passquest cask update`, id `170659265`, write access, created 2026-11-21T12:17:40Z, whose private
half lives only in this repository's Actions secret `HOMEBREW_TAP_DEPLOY_KEY` (set
12:17:41Z). The GitHub API's deploy-keys list carries no other key. `harness/tools/cask-publish.ts`
takes it from the environment only on `--push`, writes it to a `0600` file (inside the run's `0700`
temp tree; NTFS ACLs decide on Windows, the comment says so) under `umask 077`, hands it to `git`
through a `GIT_SSH_COMMAND` with `-F /dev/null -o IdentitiesOnly=yes -o
StrictHostKeyChecking=accept-new -o BatchMode=yes`, prints only the ssh remote and the commit id,
and deletes the file; the commit identity is the owner's (`-c user.name/user.email`, never the
machine's). The `secrets` gate scans the tree — `44912433474`: `PASS  secrets/sources (769 files, 0
findings)` — and no log in any run above carries a key: the dry-run step's environment holds only
`GH_TOKEN` and `TAG`, and the push step has never run in CI.

The tap's history is public record. It is disclosed here whole — this branch does not touch the tap:

| Commit | Date (UTC) | Author | What it is |
|---|---|---|---|
| `6692aaa` | 2026-11-21T10:59:35Z | wxxb789 (GitHub noreply address) | the `gh repo create … --add-readme` initial commit |
| `77210f9` | 2026-11-21T11:16:10Z | `passquest-cask-update` | the **unauthorized** push: a round-1 implementer probe pushed through the owner's https credential. The tool now commits as the owner and never as this identity (amendment 20) |
| `d9cf7eb` | 2026-11-21T12:17:51Z | wxxb789 `wxxb789@outlook.com` | the coordinator's seed with the deploy key, run locally through the same `--push` path |
| `82b6693` | 2026-11-21T13:26:32Z | wxxb789 `wxxb789@outlook.com` | the re-seed after the measured `zap trash:` paths landed (the round-4 render), pushed with a temporary deploy key that was removed afterwards |

The tap's current file is that round-4 render, `760e3ef8…564d`; since the dated note above moved
this tree's render to `578f57a7…b4b6`, the tap needs one more seed for `install`'s `tap-render`
assertion to pass again. The local seeds used the same tool and key path as the workflow's
`--push`; the workflow's own push path (a `release: published` event, or `push=true` dispatch) has
not run yet — the first release after the merge exercises it.

## The constraints row

| Constraint | Evidence |
|---|---|
| no signing, no notarization | `.github/release-notes.md` and the guides still state the unsigned path; nothing in the goal's diff signs anything |
| no change to study behaviour, data location or update check | `git diff d74f079..HEAD --stat -- internal cmd agent mygo.json` prints only `internal/desktop/packaging_test.go` and `mygo.json` (`2 files changed, 10 insertions(+), 6 deletions(-)`); `internal/update`, `internal/desktop/paths.go` and `agent/*.mjs` are untouched; the `core` jobs' `go test` (955 tests on macOS/Linux, 953 on Windows, 0 skipped) and the four host smokes stayed green in all three gate runs |
| publishing stays the owner's | `.github/workflows/release.yml` and `.github/workflows/gate.yml` are not in the diff (`git diff d74f079..HEAD --name-only -- .github` lists only the cask workflow, the template and the release notes); no release was created by this goal — `gh api repos/wxxb789/passquest/releases` lists one release, v0.15.0, published 2026-11-20T06:50:53Z, before this branch's work |
| no new platforms | no new target in `mygo.json`; the cask declares `depends_on arch: :arm64`, the published artifact's architecture |

## Local checks run in this step (final tree)

| Check | Result |
|---|---|
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts docs-links.ts docsLinks` | PASS — `318 checked, 0 broken` (this file included) |
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts install-guide.ts installGuide` | PASS — `7 sections, 4 code blocks and 7 screenshot places agree; 6 links and 5 URLs agree; 6 quoted app strings match their sources; the release notes name the tap command and both guides; the cask template renders v0.15.0 with the release's SHA-256, and a synthetic pair, into Casks/pass-quest.rb; floor 26.0 from mygo.json` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts secrets.ts secrets` | PASS — `770 files, 0 findings` |
| `mise exec -- go test ./internal/desktop/ -run TestPackagingIdentity -count=1 -v` | PASS — `--- PASS: TestPackagingIdentity (0.16s)` (this step changed documents only) |

## Known limits

- **macOS 27 has no runner.** The install job runs on `macos-26` and asserts `sw_vers` against the
  cask's declared major first; macOS 27 is documented from Apple's list and never executed.
- **The trust path on Homebrew 7.0.14 is exercised only up to the refusal.** Run `44910568731`
  (7.0.14) ends at `Refusing to load cask wxxb789/tap/pass-quest from untrusted tap wxxb789/tap`;
  the run with the `brew trust` fix (`44912433442`) landed on a 7.0.11 runner, where the assertion
  reads `Already trusted cask: wxxb789/tap/pass-quest`. Runners differ in the Homebrew they carry
  (7.0.11 and 7.0.14 seen), which is why the job prints `brew --version` and asserts on it.
- **`brew audit --online` did not run.** The job audits with `brew audit --cask --strict` — the
  offline checks, exit 0 in run `44912433442` — and `--online` was not run: it downloads the
  artifact and compares the cask's declared minimum with the artifact's own
  `LSMinimumSystemVersion` (`audit_min_os`, an `online?`-guarded check), and the historical v0.15.0
  plist says 13.0 against the cask's 26, so the comparison is expected to report a mismatch until
  the first release built after P1. A gate makes no external network call, so `--online` is not part
  of the local checks either.
- **The workflow's push path has not run in CI.** The seeds (`d9cf7eb`, `82b6693`) exercised the
  same tool and key from a workstation; a `release: published` event or a `push=true` dispatch is
  what will exercise it inside Actions.
- **The Gatekeeper prompt itself is not exercised.** A `.dmg` built on a runner carries no
  quarantine attribute, so the invariant job cannot meet the prompt; the quarantine mutant shows the
  attribute Homebrew's download adds is real, and the guide's Apple-cited steps are the owner's
  review's subject. Screenshots are pending.
- **The bundle-removal assertion has no mutant.** The three mutants cover the quarantine clearing,
  the `zap trash:` list and the `quit:`; a cask without `app` is not run.
- **`xattr -rd` over an app that carries no attribute** (a re-install over the same cask) is not
  exercised: each run installs once into a clean runner. The step is `must_succeed: false` for that
  case (amendment 2).
- **The tap must be re-seeded when the template changes.** `install`'s `tap-render` assertion fails
  until the tap carries the tree's render again; the plan's "Coordinator actions needed" holds the
  command, and the seeds above are the record of it having happened.
- **The `~/Library` listing is name-based** (`*passquest*` / `*pass quest*` under two levels); a
  path the app writes under another name would not be listed. The cask's `zap trash:` is the
  boundary, and the tool asserts every path that list names.
