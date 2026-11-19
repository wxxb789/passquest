# 0013 — Install and recover Pass Quest as a personal desktop app: evidence

Contract: [`../goals/0013-reliable-personal-desktop-distribution.md`](../goals/0013-reliable-personal-desktop-distribution.md).
Plan: [`../plans/2026-11-18-1900-0013-desktop-distribution.md`](../plans/2026-11-18-1900-0013-desktop-distribution.md).
Guide: [`../install.md`](../install.md). Decisions: Q-0013-01…10 in
[`../decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md).

## Environment

Windows 11 Enterprise (10.0.26300), Go 1.27.4, Bun 1.4.4, Node 26.13.1, MyGo 0.10.0 (Go tool), WebView2
runtime installed. Ubuntu 24.04 (WSL) ran the local Linux build and the AppImage. macOS was not run
locally; CI's `macos-latest` runner is its only evidence. CI is
[`.github/workflows/gate.yml`](../../.github/workflows/gate.yml): three runners (`ubuntu-latest` x64,
`windows-latest` x64, `macos-latest` arm64) × four groups (`core`, `desktop`, `package`, `self-test`),
with `fetch-depth: 2`.

## Gate runs

| Command | Result |
|---|---|
| CI, run 44467752612 (head `8a77f36`, the last code and docs change; the commit after it only fills in this row) | 12/12 jobs on the first attempt, 6m20s. `package smoke`: Windows x64 484 assertions (`pass-quest-setup-0.13.0-amd64.exe` 36.1 MB) `[282.6s]`, macOS arm64 483 (`pass-quest-0.13.0-arm64.dmg` 44.5 MB) `[251.5s]`, Linux x86_64 483 (`Pass_Quest-0.13.0-x86_64.AppImage` 63.0 MB) `[178.4s]`, each over `contents, layout, study, agents, anki, upgrade, credential, broken`; the upgrade case compared 32 tables, 160 rows and 1546 fields with 0 differences on every OS. Artifacts: `package-windows-latest` 36,063,465 B, `package-macos-latest` 44,443,551 B, `package-ubuntu-latest` 62,386,147 B |
| CI, run 44453184248 (head `d9f994f`) | Attempt 1: 10/12 — `gate (macos-latest, desktop)` failed in `desktop ux` with `page-error` `JS error: Script error. @0:0` on the `practice-long` state, and `gate (macos-latest, package)` failed at `boot: 1 error bar(s): JS error: Script error. @0:0`. Attempt 2 re-ran only those two jobs on the same head: 12/12 passed (`gate (windows-latest, package)` 484 assertions, `gate (ubuntu-latest, package)` 483). The same commit failing then passing marks a WebKit flake whose message carries no detail; see Known limits |
| CI, run 44447814720 (head `adfdfc6`, last run green on its first attempt) | 12/12 jobs, 16m31s; `package smoke` 478 assertions on macOS arm64 (`pass-quest-0.13.0-arm64.dmg` 44.5 MB), 479 on Windows x64, 478 on Linux x86_64 `[179.5s]` |
| `bun run gate` (Windows, 2048×1152 display, the P8 tree before its final wording fixes) | 25 passed, 0 failed, 0 skipped: `go test` 35 packages, 911 tests, 0 skipped; `desktop smoke` 349 assertions over 12 scenarios; `desktop ux` 168 audits over 21 states × 4 modes × 2 sizes, 0 allowed; `package smoke` 484 assertions over `contents, layout, study, agents, anki, upgrade, credential, broken`; `harness self-test` 54 cases (4 must pass) over 24/24 gates; `agent smoke` and `provider smoke` pass locally too |
| `package smoke`, case list | `contents, layout, study, agents, anki, upgrade, credential, broken` |
| `bun run gate --group package` (local Windows, steps P5–P7) | P5: 358 assertions; P6: scan 12,776 files / 194 MB in 6.1 s; P6b: 3,934 + 785 needles, 12.9 s, no collisions, peak ≈1.6 GB; P7: 477 assertions, upgrade case 31.9 s. The gate removes its own install on every exit path it controls |
| `bun run package` (local Windows, P2) | `Pass Quest.exe` (GUI subsystem, icon, version 0.13.0) and `pass-quest-setup-0.13.0-amd64.exe` (36 MB); installs silently, launches and uninstalls cleanly |
| AppImage on Ubuntu 24.04 (WSL, P3) | runs without FUSE under xvfb with no node/go/bun on `PATH`; Today renders; the bundled Node 22.23.5 is reported available; data lands in `~/.config/Pass Quest` |
| `docs-links` / `secrets` / `local-only` (this step, final tree) | 235 links, 0 broken; 895 files, 0 findings; 34 first-party packages, 0 network imports, 0 confidential markers, 0 boundary violations |

The local Windows rows are recorded by the steps themselves (P2–P7 commit messages); the P8 step ran
only the three gates above. Every package case runs against loopback stubs: no external network, no
real credential, no real deck.

## Acceptance → evidence

| # | Done when | Evidence |
|---|---|---|
| 1 | Installable on three platforms | CI run 44467752612; jobs `gate (windows-latest, package)`, `gate (macos-latest, package)`, `gate (ubuntu-latest, package)`. Each built this run's package from the repository and installed/mounted it: NSIS `/S /D=<temp prefix>`, the `.dmg` attached and the `.app` copied out with `ditto`, the AppImage run in place with `APPIMAGE_EXTRACT_AND_RUN=1` and its extract tree inside the gate's temp tree. Artifacts `package-windows-latest`, `package-macos-latest`, `package-ubuntu-latest` (7 days; `Upload package` in `gate.yml`). Observed sizes at `d9f994f`: 36,062,478 / 44,443,345 / 62,385,298 bytes — the macOS upload happened despite its failed smoke, since the upload step runs when the job is not cancelled |
| 2 | A study loop with no developer tools | Case `study`, on each runner: every launch has `go`, `bun` and `node` absent from `PATH` (asserted), a cwd outside the repository and an empty per-user root, so the default data path runs; the installed app boots to Today, practices the verified sample through feedback, and records an attempt whose `report` output is byte-identical after a relaunch and after reinstalling the same version over the prefix (the data tree's digest is unchanged by the reinstall). Case `layout` checks the bundled Node runtime is this platform's executable |
| 3 | First run and lifecycle | Case `study`: the first launch created its database under the per-user root and none under the install directories (on Linux the extract-and-run tree included). Go tests: `TestDataDirectoryIsPerUserOnEveryLayout`, `TestDataDirectoryRefusesInstallLocations`, `TestDataDirectoryAcceptsLooseBinaryAboveHome`, `TestStartRefusesWithoutWebView`, `TestStartWithoutWebViewWritesSmokeResult`, `TestMissingWebViewMessageNamesEachRuntime` (`internal/desktop/lifecycle_test.go`), `TestOpenRejectsNewerSchemaLeavesFileUntouched` (`internal/store/store_test.go`). The uninstall removes the install directory; that it leaves the learner's data is a cleanup sanity check, not proof — the guarantee is the study case plus MyGo 0.10.0's Uninstall section, which removes only `$INSTDIR`, its shortcuts and its HKCU key. [`install.md`](../install.md) says where the data lives and how to remove it |
| 4 | Optional features degrade safely | Case `agents`: the coach call and the note refinement went through the package's own bundled Node runtime and hosts against the loopback provider stub. Case `anki`: a pushed note reached the loopback AnkiConnect stub, and the endpoint-absent path queued it. Case `credential`: a key entered in the window was in no file under the data root or the install directories (bytes searched); on macOS only the data root moved into the temp tree, because the login keychain needs the real HOME. Case `broken`: with the bundled runtime deleted (on Linux from a separate `--appimage-extract` copy of the image, launched through its `AppRun`), the coach send, the refinement, the Anki push and the durable brief each reported `node_unavailable` with one stated action, and the local-services sheet showed the agent host as unavailable with one stated next step, while practice, assessment and note drafting and accepting still worked. `agent record-step` through the GUI-subsystem Windows binary returned JSON over pipes (P4, local) |
| 5 | Forward upgrade keeps the evidence | Case `upgrade`: the previous build — `PREVIOUS_RELEASE_COMMIT` = `357b4c6`, the merge of PR #28 and the last `main` commit before this goal — writes, through its own CLI in a `git worktree`, independent and assisted attempts, a finished and an active assessment run, a note pushed to the id-persisting stub and a queued note. The installed app opens that data root; every old table's row count is unchanged, then 32 tables / 160 rows / 1546 fields compare with 0 differences; the notes room drains the queued note and a revision updates the same Anki note id (one `updateNoteFields`, no second `addNote`); a copy with schema 18 is refused as `database_newer` — the mapped title and next step in the state card, no unmapped-code span — with the file and side files byte-identical. The self-test's `compareDatabases` unit check fails on a changed field, a dropped table, a dropped column and a deleted row. Disclosed: the base and this build share schema version 17, so the case proves a same-version reopen by the packaged build, not a migration step |
| 6 | Nothing confidential ships | Case `contents` walks every file of the installed payload before the first launch: the Windows prefix with `Uninstall.exe` and MyGo's `.mygo-files`, the whole macOS `.app` plus the mounted image's captured root listing, the Linux extract tree. It fails on a `%PDF-` header within the first 1024 bytes, `SQLite format 3\0` at byte 0, a `.env`/`.env.*` name (on a link, its target too), a `data/private`, `data/local`, `.passquest` or `local-data` path segment, the internal-only marker bytes, or either committed extraction's text — a byte-identical file, or a normalised line of at least 40 bytes in the raw, JSON/JS-escaped or UTF-16LE-with-BOM view of a payload file. The `secrets` gate's credential patterns apply to the project-owned files only (`agent/*.mjs`, the embedded web bundle, metadata derived from `mygo.json`). Self-test cases seed a payload with each forbidden kind (the failure names every seed), a copied baseline page, and an accepted credential-shaped string that only sits in `node_modules` |
| 7 | Truthful platform claims | [`install.md`](../install.md): per platform the package, the unsigned-publisher warning and how past it, the prerequisites the package does not bundle (WebView2; GTK3 and WebKitGTK 4.1 with Debian/Ubuntu and Fedora names), where data lives and how to remove it (including the credential-store entries and the OS tools), how to upgrade and that downgrade is refused, and what is untested. Every supported target names the CI job that proves it (item 1): Windows x64, macOS arm64, Linux x86_64. Q-0013-01…10 record the defaults; the five earlier entries that named goal 0013 (Q-0006-02, Q-0007-07, Q-0011-04, Q-0011-10, Q-0011-14) each carry a dated revisit bullet |

## Known limits

- **Unsigned packages.** No certificate and no notarization; the SmartScreen and Gatekeeper paths in
  the guide are the learner's manual way past the warning. CI cannot exercise the Gatekeeper prompt: a
  `.dmg` built on the runner carries no quarantine attribute.
- **Only the CI runners' architectures are built and smoke-tested:** Windows x64 (`windows-latest`),
  macOS arm64 Apple silicon (`macos-latest`), Linux x86_64 (`ubuntu-latest`). Windows arm64, macOS
  x86_64 and Linux arm64 have no package.
- **The macOS credential case keeps the real HOME** so the login keychain is reachable; only its data
  root moves into the gate's temp tree.
- **Linux runs the AppImage only through extract-and-run:** the AppImage is exercised through extract-and-run, not a
  FUSE mount, and under xvfb (X11); Wayland-specific behaviour is not covered. The image file is
  read-only, so the `broken` case deletes the runtime from a separate `--appimage-extract` copy of
  the same image and launches that copy's `AppRun`, not the installed image.
- **The upgrade base has the same schema version** (`357b4c6` and this build: 17 → 17). The case proves
  a same-version reopen by the packaged build — old rows and Anki note ids kept, a newer schema refused
  with its bytes untouched — not a migration step. The pass line prints both `user_version` values.
- **`desktop ux` needs a display larger than its page areas** (Q-0012-05): it ran locally on a
  2048×1152 display; on a 1280×800 display it refuses with its viewport precondition, and the 3-OS CI
  run (Windows at 1920×1080) is then that gate's evidence.
- **The content scan does not detect** UTF-16LE CJK text without a BOM, UTF-16BE, base64, compressed or
  encrypted copies, or a transliteration the normalisation cannot undo (for example a line spread out
  character by character). A compressed container itself is not byte-scanned; its extracted tree is.
- **The uninstall-keeps-data check is a cleanup sanity check, not proof** — an uninstaller deleting a
  data directory would resolve `$APPDATA` through the OS shell folders and escape the run's temp root.
  The guarantee is the study case plus MyGo 0.10.0's Uninstall section, as disclosed.
- **The durable host and search with the bundled runtime present are not exercised by the installed package.** The
  `agents` case covers the tutor host (coach and refinement), `anki` the Anki host, both against
  loopback stubs through the bundled runtime; the durable brief is exercised in the installed app as
  the unavailable path (`broken`) only, and search is not exercised in the installed package at all. Their runtime coverage in a source checkout is `agent smoke` and `find smoke`.
- **An opaque WebKit error on macOS is intermittent.** Run 44453184248 attempt 1 failed two macOS jobs
  with `JS error: Script error. @0:0`, and a re-run of the same head passed. WKWebView reports a
  cross-origin or detail-less error as "Script error.", so the window's error bar cannot name the
  cause; the page-error check was not weakened. Follow-up: capture more detail from WebKit errors.
- **macOS 13 is the minimum.** The Go 1.27 toolchain builds for macOS 13 and later, so `mygo.json`
  declares `macos.minimumSystemVersion` 13.0 (MyGo's default of 12.0 would have let macOS 12 try to
  open a binary it cannot run). CI runs only `macos-latest`.
- **A developer's Windows machine must not hold a real Pass Quest install** while the package gate
  runs: the gate refuses to install while the app's HKCU uninstall key (outside its temp tree) or one
  of its real shell-folder shortcuts exists (Q-0013-09). A run killed from outside can leave its own
  registration behind, and the next run then refuses until that copy is uninstalled.
- **An existing CLI `.passquest/` database is not migrated** into the desktop data directory (out of
  scope; Q-0011-04's move stays open). The old CLI's note push inside the upgrade case runs with the
  repository as cwd, exactly as the CLI smokes do (`upgrade.ts`'s disclosure).
- **No release is published.** The packages exist as 7-day workflow artifacts only (Q-0013-07); there
  is no update check and no published download (goal 0015).
