# 0013 — Install and recover Pass Quest as a personal desktop app

**State:** Ready (refined 2026-11-18)
**Prerequisite:** 0011 — functional desktop workspace, 0012 — visual refinement

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Make the product viable for a nondeveloper studying alone, respecting the fact that Go/MyGo and the Node/Pi agent host both need a lifecycle. No central account or cloud service is required.

Today the desktop app runs only from a source checkout: a developer builds it with Go and Bun, and the optional agent features find Node and the agent host's dependencies in that checkout. After this goal, a learner on Windows, macOS or Linux installs Pass Quest from a package and studies, with no developer tool on the machine. The learner's study evidence then survives a relaunch, a reinstall and an upgrade.

## Owner decisions (2026-11-18)

- **Platforms:** Windows, macOS and Linux. A platform may be claimed as supported only if the project's CI builds its package and runs the installed package on that platform's runner. Anything else is described as untested.
- **Packaging:** unsigned installers, with no certificates: a Windows installer (MSI or NSIS), a macOS `.dmg` and a Linux AppImage. The optional Node agent host is bundled. The "unknown publisher" or Gatekeeper warning each OS shows is documented, together with the learner's way past it.
- **Upgrades:** forward-only data compatibility. A newer build opens a database written by an older build and keeps every accepted learner record and every Anki note identity. Downgrade is not supported. Goal 0015 adds the in-app update check; this goal does not contact the network.

## Done when

Each item below is shown during the run with the evidence named. All of it must hold on the final tree, and the
project's existing gates must stay green.

1. **Installable on three platforms.** One CI workflow builds an unsigned Windows installer, macOS `.dmg` and Linux AppImage from the repository. On each platform's runner it installs or mounts that package and launches the installed app. Evidence: the workflow run on the final head, with the three packages kept as artifacts.
2. **A study loop with no developer tools.** On each platform the installed app runs with no Go, Bun or Node and no source checkout reachable on `PATH` (the run proves their absence). It completes Today, practice with the verified sample, feedback, and a recorded attempt that is still there after a relaunch. This runs through the same scripted desktop path the existing desktop smoke uses, but against the installed package.
3. **First run and lifecycle.** A first launch creates the learner's data in the platform's per-user data location, never inside the install location. A normal quit followed by a relaunch keeps everything. Reinstalling the same version keeps the data. Uninstalling leaves the learner's data in place (the default taken), and the docs say where the data lives and how to remove it.
4. **Optional features degrade safely.** In the installed app:
   - The bundled agent host starts when an agent feature is used.
   - With the host removed or broken, every agent feature reports itself unavailable with one stated action, while practice, assessment and notes keep working.
   - Model credentials still come only from the OS credential store or the environment and are never written to disk in plain text.
   - Anki is detected when present and reported as absent otherwise, without blocking study.

   Evidence: transcripts from the installed package for each case.
5. **Forward upgrade keeps the evidence.**
   - A database written by the build just before this goal (current `main`, or the newest tagged release if one exists by then) is opened by the new installed build.
   - Every attempt, assessment run, note and recorded Anki note id survives unchanged. A later note push updates the same Anki note rather than adding one, against the existing Anki stub.
   - A database from a newer schema is refused with a clear message and is not modified.

   Evidence: an automated upgrade check that fails if any of those records change.
6. **Nothing confidential ships.** An automated check over each package's contents proves that no source PDF, its derived private text, `data/private/`, `.passquest/` or other learner data, no API key, and no developer secret is inside. The check fails on a package seeded with such a file.
7. **Truthful platform claims.**
   - The user-facing install guide lists per platform: the package, the unsigned-publisher warning and how to proceed, the runtime prerequisites the package does not bundle (for example the system webview), where data lives, how to upgrade, and what is untested.
   - Every supported claim maps to item 1's CI evidence.

Constraints:

- No change to scoring, consent, budget, assessment or sync rules.
- No new network call: the local-only gate stays green.
- No certificates and no publication. Building packages as CI artifacts is in scope. Publishing a GitHub Release or any other public download needs the owner's explicit go-ahead at that time.

Stop and report at once if, at the start, something only the owner can supply is missing. For example, a platform's runner cannot run the installed package at all, so the claim cannot be backed. Otherwise take reversible defaults and disclose them in `docs/decisions/OPEN-QUESTIONS.md`: the Windows installer technology, the data-location details, and the uninstall behaviour. Stop only for an irreversible or out-of-bounds decision. End with a report that pairs each item above with its evidence and lists the known limits.

## Out of scope

- Code signing, notarization, an app store, auto-install and the update check (goal 0015).
- Bundling Anki, AnkiConnect or a model.
- Platforms or architectures CI does not run.
- A downgrade path.
