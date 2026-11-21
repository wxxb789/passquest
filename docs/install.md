# Install Pass Quest

**English** | [简体中文](install.zh-CN.md)

Pass Quest ships as an unsigned package for Windows, macOS and Linux: an NSIS installer, a `.dmg`
and an AppImage. Each package carries the desktop app, the four agent host scripts, the third-party
notices the inlined code requires and a pinned Node 22 runtime, so the agent features run on the
package's own Node with no Node installed (CI proves this for the coach, Anki and the study brief;
see [What CI proves](#what-ci-proves-and-what-is-untested)). A host that loads npm packages ships as
one self-contained file and the rest as byte-identical copies, so a package carries no
`node_modules` and nothing the hosts do not run ([`agent/README.md`](../agent/README.md)). Anki,
AnkiConnect and any model are **not** bundled; a coach call needs a provider key you enter yourself.

The packages are built and smoke-tested by CI ([What CI proves](#what-ci-proves-and-what-is-untested)).
Download them from the repository's
[GitHub Releases](https://github.com/wxxb789/passquest/releases/latest): each release carries the
three packages and a `SHA256SUMS.txt`, and its packages are the ones the gate's `package` jobs built,
installed and smoke-tested on the tagged commit ([Releasing](release.md)). A CI run's
`package-<os>` artifact, kept for 7 days, is an untagged build for inspection. This guide covers
installing the packaged app; what the app does is described in the [project README](../README.md).

## Supported systems

Windows and macOS are first-tier: every CI run builds, installs and drives both packages. Linux is
**second-tier** — the AppImage works, but it gets less CI and no installer.

| Platform | Package | Supported |
|---|---|---|
| Windows x64 | `pass-quest-setup-<version>-amd64.exe` | Windows 11 and later |
| macOS Apple silicon | `pass-quest-<version>-arm64.dmg` | macOS 26 and macOS 27, the two latest majors |
| Linux x86_64 | `Pass_Quest-<version>-x86_64.AppImage` | second-tier; AppImage only |

**Windows 10** is not blocked but it is unsupported and untested: the installer runs and the package
may work, yet no CI job covers Windows 10 and nothing here is tested on it. Any macOS older than 26
is unsupported: the packages declare macOS 26 as their minimum.

## Windows

**Package.** `pass-quest-setup-<version>-amd64.exe`, the per-user NSIS installer built on
`windows-latest` (x64).

**Install.** Run the installer and follow it. It installs for the current user only, to
`%LOCALAPPDATA%\Programs\Pass Quest`, and needs no administrator rights. It adds a Start menu entry,
offers a desktop shortcut on the finish page, and registers *Pass Quest* in **Settings > Apps**,
where **Uninstall** (or `Uninstall.exe` in the install folder) removes it.

**Unsigned warning.** The installer has no code-signing certificate, so SmartScreen may show *Windows
protected your PC*. Choose **More info**, then **Run anyway**. The warning is expected for an unsigned
package; it is not a sign of a modified file.

<!-- screenshot: windows-smartscreen -->
*Screenshot: the SmartScreen "Windows protected your PC" dialog and its **More info** link (owner).*

**Prerequisites.** The Microsoft Edge WebView2 Runtime, the system web view. Windows 11 ships it;
Windows 10 may not, and the package does not bundle it. When it is missing, the app shows one message
before any window — *"Pass Quest needs the Microsoft Edge WebView2 Runtime, which this PC does not
have. Install it from https://developer.microsoft.com/microsoft-edge/webview2/ (the Evergreen
Standalone Installer), then start Pass Quest again."* — and exits.

## macOS

**Package.** `pass-quest-<version>-arm64.dmg`, built on `macos-latest` (Apple silicon). An Intel build
is not produced by CI.

**Install.** Open the disk image and drag **Pass Quest** into **Applications**, then eject the image
and open the app from **Applications**. The image is read-only, and macOS does not eject it for you;
macOS records the exception below for the app in **Applications**, and a copy started from the
still-mounted image is a different file that the exception does not cover.

**First launch of a downloaded copy.** The app is ad-hoc signed only and not notarized — the owner's
recorded decision in goal 0013, with no certificate and no Apple Developer account. Gatekeeper
therefore blocks the first launch of a copy you downloaded, and macOS 26 and 27 no longer offer the
older right-click → **Open** way past it. What works:

1. Open **Pass Quest** from **Applications** and let macOS refuse it.
2. Open **System Settings** → **Privacy & Security**, scroll to the notice about the blocked app and
   choose **Open Anyway** — macOS offers the button for about an hour after the attempt.
3. Confirm with your login password or Touch ID. macOS saves the app as an exception, so this is the
   only time the refusal appears.
4. Open **Pass Quest** again; it starts normally.

The steps and the one-hour window are Apple's: [Safely open apps on your
Mac](https://support.apple.com/en-us/102445) (published 2026-05-27) and [Open a Mac app from an
unknown developer](https://support.apple.com/guide/mac-help/open-a-mac-app-from-an-unknown-developer-mh40616/mac).

<!-- screenshot: macos-blocked-dialog -->
*Screenshot: the dialog macOS shows when the first launch of the downloaded app is refused (owner).*

<!-- screenshot: macos-privacy-security -->
*Screenshot: System Settings → Privacy & Security with the blocked-app notice and the Open Anyway button (owner).*

<!-- screenshot: macos-open-anyway-confirm -->
*Screenshot: the confirmation that asks for the login password or Touch ID (owner).*

<!-- screenshot: macos-first-launch -->
*Screenshot: Pass Quest open after the exception was saved (owner).*

This is needed **once per download**: when you upgrade by downloading the new `.dmg` in a browser, the
new copy is a new download and macOS refuses its first launch too. A Homebrew install avoids the
warning instead — its cask clears the quarantine flag after the download, so the first launch is not
blocked:

```bash
brew install --cask wxxb789/tap/pass-quest
```

<!-- screenshot: macos-homebrew-install -->
*Screenshot: `brew install --cask wxxb789/tap/pass-quest` finishing in Terminal (owner).*

**Terminal alternative.** If you prefer the terminal, the exception has a one-line equivalent that
removes the quarantine attribute the browser download set:

```bash
xattr -dr com.apple.quarantine "/Applications/Pass Quest.app"
```

<!-- screenshot: macos-xattr-terminal -->
*Screenshot: the `xattr` command succeeding in Terminal (owner).*

Do not disable Gatekeeper system-wide to get past the warning. The steps above are per-app, and the
rest of the system stays protected.

**Prerequisites.** macOS 26 (Tahoe) or macOS 27 (Golden Gate), on Apple silicon — the two supported
majors. Nothing else: WKWebView is part of the system and the package bundles what else it needs.

## Linux

**Package.** `Pass_Quest-<version>-x86_64.AppImage`, built on `ubuntu-latest` (x86_64).

**Install.** Make it executable and run it:

```bash
chmod +x Pass_Quest-*-x86_64.AppImage
./Pass_Quest-*-x86_64.AppImage
```

Running an AppImage directly mounts it with FUSE: the kernel's FUSE support and the `fusermount3`
helper (Debian and Ubuntu: `sudo apt install fuse3`; Fedora: `sudo dnf install fuse3`). The image
uses the static type2 runtime, so `libfuse2` is not needed. Where FUSE is unavailable (some
containers, WSL), run it through extract-and-run instead:

```bash
./Pass_Quest-*-x86_64.AppImage --appimage-extract-and-run
# or: APPIMAGE_EXTRACT_AND_RUN=1 ./Pass_Quest-*-x86_64.AppImage
```

CI itself runs the AppImage that way and never mounts it with FUSE.

**Prerequisites.** GTK 3 and WebKitGTK 4.1 from the system; the AppImage cannot reasonably bundle
them. Debian and Ubuntu: `libgtk-3-0t64` (`libgtk-3-0` before Ubuntu 24.04 and Debian 13) and
`libwebkit2gtk-4.1-0`. Fedora: `gtk3` and `webkit2gtk4.1`.
When WebKitGTK is missing, the app shows one message naming both. When GTK 3 itself is missing, no
window or dialog can open: start the AppImage from a terminal to see the same message on stderr.

**No integration beyond the AppImage.** There is no installer, no menu entry and no file association;
your desktop may offer to integrate an AppImage itself.

## Where your data lives, and how to remove it

The database, the window state and the agent's own files live in one per-user directory:

| Platform | Directory |
|---|---|
| Windows | `%APPDATA%\Pass Quest` (the Roaming AppData folder) |
| macOS | `~/Library/Application Support/Pass Quest` |
| Linux | `$XDG_CONFIG_HOME/Pass Quest`, default `~/.config/Pass Quest` |

The database is `passquest.sqlite` inside that directory. `PASSQUEST_DESKTOP_DATA` overrides the
directory and `PASSQUEST_DB` the database file. The directory is deliberately kept out of the install
location: an upgrade replaces the install and an uninstall removes it, and your study evidence must
survive both. (A lone binary in your home directory is not an install and is not refused.)

**Uninstalling keeps the data.** On Windows the uninstaller removes the install directory, its
shortcuts and its registry entry, and leaves `%APPDATA%\Pass Quest` in place; on macOS move the app
to the Trash; on Linux delete the AppImage. To remove the data, delete the directory above yourself. The system
web view also keeps a small profile of its own outside it, with your theme and language choices and
its cache, and no study records: on Windows `%LOCALAPPDATA%\Pass Quest\WebView2`, on macOS the
WebKit data and cache folders for the app under `~/Library`, and on Linux WebKitGTK's data and cache
folders under `~/.local/share` and `~/.cache`. Delete these too for a clean removal.

**Credentials are not in those files.** A model key entered in Settings goes to the OS credential
store — Windows Credential Manager, macOS Keychain, Secret Service on Linux — under the service
`com.github.wxxb789.passquest`, one entry per provider, named `api-key:<provider>`. The window's row
for a provider has a remove-key action; the OS tool removes a key too:

| Platform | Remove a key by hand |
|---|---|
| Windows | `cmdkey /delete:com.github.wxxb789.passquest/api-key:<provider>` |
| macOS | `security delete-generic-password -s com.github.wxxb789.passquest -a api-key:<provider>` |
| Linux | `secret-tool clear service com.github.wxxb789.passquest account api-key:<provider>` |

An environment variable still wins over the store and is never copied into it.

## Upgrade

Install the newer package over the old one: run the newer installer, replace the app in Applications,
or replace the AppImage file. Your data carries forward — it lives in the per-user directory above,
never inside the install, and the newer build opens the older database.

**macOS.** A copy you downloaded in a browser is a new download, so the Open Anyway step comes back
once; the Homebrew cask clears the quarantine flag for you instead (see [macOS](#macos)).

Downgrade is not supported. An older build refuses a database written by a newer one and does not
modify it: the window shows *This data is from a newer Pass Quest*, with the next step *Install the
newer Pass Quest that wrote this data; this version will not change it.*, and the CLI reports
`store: database schema version N is newer than supported version M`.

**Checking for updates from the app (goal 0015).** Open **Settings** and expand the **Updates** row: it
shows the version you are running and a **Check for updates** button. Nothing checks on its own — there
is no check at start-up and no timer — and a check never downloads, installs or writes anything. The
button sends one `GET` request to GitHub Releases for this repository (`api.github.com`, the path
`/repos/wxxb789/passquest/releases/latest`) with the two headers the app sets, `Accept` and
`User-Agent: PassQuest`; Go's transport adds only the host (`Host`, or `:authority` over HTTP/2) and `Accept-Encoding`, and the request sends
no account, no study data and no device identifier, and downloads nothing. The app refuses a redirect
and does not use a proxy from your environment. If a newer release exists, the row shows its version
and publication date with a link to its release page, which opens in your browser; you install it the
normal way, as above. If your version is the latest published one, the row says so. Only when the
repository has no published release does the check answer *No release has been published for this
repository yet, so there is nothing newer to install.* Drafts and pre-releases are never offered. If the
request cannot reach GitHub — offline, rate-limited, or a network that requires a proxy — the row says
*could not check* with a short reason, and study is unaffected.

## What CI proves, and what is untested

CI ([`.github/workflows/gate.yml`](../.github/workflows/gate.yml)) runs the `package` job once per OS:
`gate (windows-latest, package)`, `gate (macos-latest, package)` and `gate (ubuntu-latest, package)`.
Each job builds that OS's package from the repository, installs or attaches it (NSIS `/S`, the `.dmg`
mounted and the `.app` copied out with `ditto`, the AppImage run in place), launches the installed app
with `go`, `bun` and `node` absent from `PATH` (asserted), and drives the study loop, the agent
features — including a study brief the installed app completes on its own bundled runtime and hosts —
Anki, the forward upgrade, the credential case and the broken-runtime case through it (the
`package smoke` gate, cases `contents, layout, study, agents, brief, anki, upgrade, credential,
broken`). The macOS job also checks the built `.app`'s `Info.plist` minimum: the package declares
macOS 26. Each run keeps its package as the `package-windows-latest`, `package-macos-latest` or
`package-ubuntu-latest` artifact for 7 days. A run on a version tag hands these same artifacts to the
release workflow, which publishes them only after the owner reviews the draft
([Releasing](release.md)).

Only these targets are claimed as supported:

| Platform | CI job | Package built and smoke-tested |
|---|---|---|
| Windows x64 | `gate (windows-latest, package)` | `pass-quest-setup-*-amd64.exe` |
| macOS Apple silicon (arm64) | `gate (macos-latest, package)` | `pass-quest-*-arm64.dmg` |
| Linux x86_64 | `gate (ubuntu-latest, package)` | `Pass_Quest-*-x86_64.AppImage` |

Untested, and therefore not claimed:

- **Other architectures.** Windows arm64, macOS x86_64 (Intel) and Linux arm64: no package is built
  or run.
- **macOS 27.** No GitHub-hosted runner provides it, so the guide documents the pair from Apple's
  support page (2026-11-09) while CI proves macOS 26 only.
- **Code signing and notarization.** None. The SmartScreen and Gatekeeper paths above are the
  learner's manual way past the warning, and CI cannot exercise the Gatekeeper prompt: a `.dmg` built
  on the runner carries no quarantine attribute.
- **FUSE and display servers on Linux.** CI runs the AppImage only through extract-and-run
  (`APPIMAGE_EXTRACT_AND_RUN=1`), so a direct FUSE mount is not covered; the smoke runs under xvfb (X11), so
  Wayland-specific behaviour is not covered either.
- **Anki and AnkiConnect.** Not bundled. Anki is reported present when a reachable AnkiConnect
  endpoint runs and reported absent otherwise; study works without it.
- **Search with the bundled runtime.** The package carries the search host and the same resolver
  finds it, but no package case exercises it: the `package smoke` cases are `contents, layout, study,
  agents, brief, anki, upgrade, credential, broken`, and the search host's success path is covered by
  the source-tree `find smoke` only. The durable host's success path is tested in the installed app
  instead (the `brief` case above).
- **Updates.** The package smoke does not drive the update check or its browser hand-off; those
  are covered by `desktop smoke`'s `update` scenario in a source checkout, not by the installed
  package.
