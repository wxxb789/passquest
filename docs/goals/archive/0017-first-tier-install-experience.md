# 0017 — Install on current Windows and macOS without fighting the operating system

**State:** Ready (defined 2026-11-20)
**Prerequisite:** None. It builds on the published v0.15.0 release and can run before or after 0016. 0016 also changes the package build, so do not run the two at the same time on one tree.

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; recoverable, idempotent local state; no central account or cloud service. Follow [Core Design](../core-design.md).

The packages stay unsigned until the owner has certificates (goal 0013). Within that limit, installing on the first-tier platforms must be no worse than on comparable unsigned apps such as FlClash. FlClash's macOS dmg is ad-hoc signed and not notarized, exactly like ours. Where it is ahead is a Homebrew tap whose cask clears the quarantine flag, so `brew install` never meets Gatekeeper.

## Owner decisions (2026-11-20)

These are recorded in Q-0000-07 and Q-0000-08.

- Windows and macOS are first-tier; Linux is second-tier.
- Supported systems are Windows 11 and later, and the latest two macOS majors: macOS 27 Golden Gate and macOS 26 Tahoe, per Apple's support page dated 2026-11-09.
- Windows 10 is not blocked. It is documented as unsupported and untested.
- Platform and architecture coverage is sufficient; no new targets.
- In-app download-and-install updates are deferred.
- The agent may create the public repository `wxxb789/homebrew-tap`, add its deploy key, and store the matching secret in this repository's Actions secrets.
- Gatekeeper screenshots come later from the owner. The guide ships with accurate text and marked places for the images.

## Intended outcome

On the latest two macOS versions, a browser-downloaded unsigned app can no longer be opened with right-click → Open. The learner has to try to open it, go to **System Settings → Privacy & Security**, choose **Open Anyway**, confirm, and enter their password. The current install guide still leads with right-click → Open. The guide is English-only, while most learners read Chinese. The package still declares macOS 13 as its minimum.

After this goal:

- A macOS learner can install with one Homebrew command and never meet Gatekeeper.
- A learner who downloads from the browser follows an accurate guide in English or Chinese, written for the systems Pass Quest actually supports.
- The packages state the supported systems honestly.

## Done when

Each item below is shown during the run with the evidence named. All of it must hold on the final tree, and the existing gates must stay green.

1. **The supported systems are what the package and the docs say.**
   - The macOS package declares macOS 26 as its minimum (`Info.plist` and `mygo.json`), replacing the 13.0 recorded in Q-0013-10.
   - The install guide names Windows 11 and later, the latest two macOS majors, and Linux as second-tier.
   - Evidence: the packaging identity test and the package contents check pin the new minimum.
2. **The guide matches what a learner sees.** The macOS section of the install guide covers only macOS 26 and 27. It gives:
   - the Open Anyway flow step by step;
   - why the app must be dragged into Applications before it is opened, instead of run from the disk image;
   - that this is needed once per download, so a browser-downloaded upgrade needs it again, while Homebrew avoids it;
   - the Terminal alternative that clears the quarantine flag on the installed app.

   The guide marks where the owner's screenshots will go. It does not suggest disabling Gatekeeper system-wide. A Chinese guide with the same content sits beside the English one, both READMEs link the guide in their language, and the release notes template points macOS users to the Homebrew command and the guide. Evidence: the docs-links gate, plus the owner's review of the text.
3. **Homebrew installs without Gatekeeper.**
   - The public tap `wxxb789/homebrew-tap` holds a cask that installs the latest published release's disk image, verified by its SHA-256.
   - It clears the quarantine flag so the first launch is not blocked.
   - Uninstalling quits the app. Zapping removes the per-user data directory the install guide names.
   - Evidence: a CI job on a macOS runner installs the cask from the tap against the published release, asserts the installed app carries no quarantine flag, and launches it through the existing smoke hook.
   - If current Homebrew no longer allows a third-party cask to clear the flag, stop and report what it allows instead.
4. **The tap follows releases on its own.** Publishing a GitHub Release updates the cask's version and SHA-256 with no manual step. The tap credential is the only one involved, it lives only in Actions secrets, and it never appears in logs. A draft release changes nothing. Evidence: a dry run of the update job against the published v0.15.0 that produces the expected cask, and the secrets gate.

Constraints:

- No Apple Developer ID, notarization or Windows code signing.
- No change to the app's study behaviour, data location or update check.
- Publishing a GitHub Release stays the owner's decision. Nothing in this goal publishes one.
- No new platforms.

Stop and report at once if, at the start, something only the owner can supply is missing, or if `gh` cannot create the tap repository or set the secret. Otherwise take reversible defaults and disclose them in `docs/decisions/OPEN-QUESTIONS.md`: the cask token, the tap layout and the workflow trigger details. Stop only for an irreversible or out-of-bounds decision. End with a report that pairs each item above with its evidence and lists the known limits. One known limit: CI has no macOS 27 runner today, so macOS 27 is covered by documentation, not by CI.

## Out of scope

- A stable signing identity (goal 0018).
- In-app updates, a Windows package manager, and Linux packaging changes.
