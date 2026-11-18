# 0013 — Install and recover Pass Quest as a personal desktop app: increment plan

Contract: [`../goals/0013-reliable-personal-desktop-distribution.md`](../goals/0013-reliable-personal-desktop-distribution.md).
Conventions: [`../conventions.md`](../conventions.md). Harness: [`../harness.md`](../harness.md).
Cross-platform rules: [`../cross-platform.md`](../cross-platform.md). Base: `main` after the MyGo 0.10.0 /
Pi 1.5.2 dependency upgrade.

## Goal in one sentence

CI builds three unsigned packages from the repository:

- a Windows NSIS installer;
- a macOS `.dmg`;
- a Linux AppImage.

Each package carries the desktop app, the agent hosts, their production dependencies and a pinned Node runtime. On its own runner, CI installs the package, then launches it with no developer tool reachable. The installed app completes a study loop and survives a relaunch, a reinstall and a forward upgrade. The package contains nothing confidential.

## Facts this plan rests on (scouted 2026-11-18)

- **MyGo build.** MyGo (`github.com/egoist/mygo`) builds with `go tool mygo build` from `mygo.json`.
  - Package info: it sets `-H=windowsgui` on Windows, `production=1`, and the `-X` package name, version and identifier that MyGo's single-instance, login and deep-link code read.
  - Outputs: a Windows NSIS per-user installer (no admin, uninstaller, `/S` silent) and a macOS `.app` + `.dmg` (ad-hoc signed). On Linux it produces a binary, `.desktop`, `.tar.gz`, `.deb` and `.rpm`. It produces **no AppImage**.
  - Resources: `resources/` (per-platform subdirectories allowed) is copied into the app and found with `App.Path(PathResources)`.
  - Cross-builds: binaries build with `CGO_ENABLED=0`; the `.dmg` needs macOS.
- **Today's build.** The repo builds the desktop app with a plain `go build` in `harness/lib/desktop.ts` `buildDesktop`. That build has a console window on Windows, no package info and the inspector on, and it copies only `agent/*.mjs`.
- **Host lookup.** The four hosts (tutor, anki, search, durable agent) each copy the same resolver:
  - `PASSQUEST_*_HOST`, else `<cwd>/agent`, `<exedir>/agent`, `<exedir>/../agent`.
  - Node is `PASSQUEST_NODE`, else `node` on `PATH`.
  - Nothing looks in MyGo's resources directory or next to the executable for Node.
- **record-step.** The durable host runs the desktop binary itself as a CLI (`agent record-step`, `cmd/passquest-desktop/main.go`) and reads its stdout over a pipe. This must keep working once the binary is built with `-H=windowsgui`.
- **App identity.** `internal/desktop/paths.go` names the app `Pass Quest` with identifier `com.github.wxxb789.passquest`. The identifier is also the credential-store service name, so `mygo.json` must use exactly these values.
- **Data location.** Data lives in the platform's per-user directory, from `App.Name()`.
- **Schema.** The store refuses a newer schema, but only when the first call fails (`internal_error`). No test proves the file is left untouched.
- **Anki stub.** The existing Anki stub re-adds a missing note under a new id. An upgrade check must use a stub that persists ids, or it passes or fails for the wrong reason.
- **Webview runtime.** The NSIS installer does not ensure WebView2. `mygo.WebViewAvailable()` can probe before `Run`. Linux needs GTK3 and WebKitGTK 4.1 from the system.
- **CI.** CI runs `gate.yml` as `os × group` (`core`, `desktop`, `self-test`). `harness/lib/gate-groups.ts` requires every gate to be in exactly one group.

## Steps (one commit each; each leaves every gate green)

| # | Step | Proof |
|---|---|---|
| P1 | **One resolver for hosts and Node.** One shared function finds each host script and the Node runtime (see "P1 lookup order" below). The four copies are deleted. In a source checkout nothing changes. | Go tests for each lookup case; every existing smoke unchanged |
| P2 | **The MyGo build.** Add MyGo as a Go `tool` dependency, plus `mygo.json` (name, identifier, version, icon, resources). A harness module stages the package resources (see "P2 staged resources" below). `buildDesktop` keeps its role for the existing gates. | a local `go tool mygo build` on Windows produces the installer; the staged tree is listed with sizes |
| P3 | **Linux AppImage.** Wrap MyGo's Linux binary and the staged resources into an AppDir, then build it with a pinned `appimagetool` (SHA-256 checked). The AppImage runs with `--appimage-extract-and-run` where FUSE is absent. The `.desktop` file and icon come from MyGo's output. | the AppImage builds and launches on ubuntu-latest |
| P4 | **First run and lifecycle.** See "P4 first run" below. | Go tests; package smoke (P5) |
| P5 | **Package gate.** A new gate in a new CI group `package`. It installs the package built in this run on its own OS (NSIS `/S` to a temp prefix, `.dmg` mounted and the `.app` copied out, AppImage run in place). It launches the installed app with a `PATH` stripped of go/bun/node, an empty HOME-like data root and a cwd outside the repository, then drives the existing smoke hook through the cases in "P5 cases" below. | the gate on all three runners; artifacts uploaded |
| P6 | **Package content scan.** The gate scans every file of each package (extracting the installer/dmg/AppImage contents; see "P6 forbidden contents" below). | a self-test case seeds a forbidden file into a fixture package and the scan fails on it |
| P7 | **Forward upgrade.** See "P7 upgrade check" below. | the upgrade check in the package gate; a self-test case alters one record and the check fails |
| P8 | **Docs and decisions.** See "P8 docs" below. | `docs-links`; the final report |

### P1 lookup order

1. The explicit environment override.
2. `App.Path(PathResources)/agent/<host>.mjs` and `.../node/<node binary>` when running as a packaged app.
3. The current repository-relative candidates.

### P2 staged resources

- `agent/*.mjs`.
- A production `node_modules` with a hoisted, symlink-free layout, built from `agent/package.json` and `bun.lock`.
- A pinned Node 22 LTS runtime per platform: the official archive, its SHA-256 checked against `SHASUMS256.txt`.

### P4 first run

- Probe the webview before `Run`. When it is missing, show one native message that names the runtime to install, instead of a blank or crashing window.
- Prove the data directory is per-user and never under the install location.
- Configure the NSIS uninstaller to leave learner data.
- Make a database from a newer schema fail at open with a clear, typed message and leave the file byte-identical (hash before and after).
- `record-step` works through the GUI-subsystem binary.

### P5 cases

1. Today → verified sample practice → feedback → attempt recorded → quit → relaunch → the attempt is still there (the same assertions desktop smoke uses).
2. Reinstall the same version → the data is unchanged.
3. Agent features:
   - the bundled host answers a coach call against the existing loopback provider stub;
   - with the bundled host or runtime removed, each agent feature reports itself unavailable with one action, while practice, assessment and notes still work.
4. Anki present (the loopback stub) and absent.
5. A credential set in the window is in no file under the data root or the install directory.

### P6 forbidden contents

The scan fails on any of:

- a PDF;
- `data/private`, `data/local`, `.passquest`, `local-data`;
- a SQLite file;
- the internal-only source marker;
- the baseline text;
- `.env` or any key pattern the secrets gate knows.

### P7 upgrade check

- Build the previous release with `git worktree` at the recorded base commit. Until a release is tagged, the base is the first-parent commit before this goal.
- Through that build, create a database with:
  - attempts (assisted and independent);
  - a finished and an active assessment run;
  - an accepted note pushed to an id-persisting Anki stub;
  - a queued note.
- Open the database with the packaged build and compare every learner record field-for-field.
- Push a revision and prove it updates the same Anki note id.
- Prove a newer schema is refused and its file left untouched.

### P8 docs

- `docs/install.md`, the user-facing install guide. Per platform:
  - package and how to install;
  - unsigned warning and how to proceed;
  - prerequisites the package does not bundle (WebView2 on Windows 10, GTK3/WebKitGTK 4.1 on Linux);
  - where data lives and how to remove it;
  - how to upgrade, and that downgrade is unsupported;
  - what is untested.
- `docs/harness.md` rows for the package gate and the new group.
- `docs/cross-platform.md`.
- OPEN-QUESTIONS defaults:
  - installer technology (NSIS);
  - uninstall keeps data;
  - Node version pin;
  - data directories.

## Defaults taken (reversible, recorded in OPEN-QUESTIONS at P8)

- **Windows installer.** NSIS, because MyGo builds it natively and it installs per user without admin; the owner allowed MSI or NSIS.
- **Uninstall.** Uninstall keeps the learner's data; the install guide says where it is.
- **Node.** Node is pinned to the latest 22 LTS that `mise.toml` and the agent `engines` accept, and checked by SHA-256.
- **Package checks.** The package checks run in their own CI group. A local `bun run gate` runs the `package` group for the local OS only.

## Out of scope

- Signing and notarization.
- The update check (goal 0015).
- Publishing a GitHub Release (needs the owner's go-ahead).
- MSI, `.deb` and `.rpm` as supported formats. MyGo's extra Linux outputs are not claimed.
- Migrating the CLI's `.passquest/` database into the desktop data directory (Q-0011-04 stays open).

## Risks

- `-H=windowsgui` and stdout over a pipe for `record-step` must be proven on Windows, in P4 and P5.
- macOS Gatekeeper on a learner's machine cannot be exercised by CI, because a locally built app carries no quarantine. The install guide documents the manual path, and the claim is limited to what CI shows.
- The size of the hoisted `node_modules` and the lazy provider loading in `pi-ai`: stage the full production tree rather than tree-shaking.
- The desktop gates on Linux depend on the hidden-window sizing workaround in `internal/desktop/hidden_linux.go`. That workaround was written against MyGo 0.9.0's GTK calls; the 0.10.0 upgrade's CI run is its proof.
