# 0015 — Know when a newer Pass Quest exists, without giving up local-first

**State:** Ready (defined 2026-11-18)
**Prerequisite:** 0013 — installable personal desktop app

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; recoverable, idempotent local state; no central account or cloud service. Follow [Core Design](../core-design.md).

## Intended outcome

A learner who installed Pass Quest from a package (goal 0013) cannot easily tell that a fixed or improved version exists. Without that knowledge, they keep studying on an old build.

After this goal, the desktop app can tell the learner that a newer release exists, show what it is, and point to where they can download it. To do this the app contacts GitHub Releases for this repository. That is the first deliberate network call the app makes on its own, and it happens only on the learner's terms.

## Owner decisions (2026-11-18)

- The app checks GitHub Releases for this repository. It never downloads or installs anything itself.
- The check needs an explicit, documented exception to the local-only rule. The exception covers only this one request, and the CLI stays network-free.
- Upgrades are forward-only, and goal 0013 already proves the data side. A "newer version" therefore means a newer published release, never an older one.

## Done when

Each item below is shown during the run with the evidence named. All of it must hold on the final tree, and the
existing gates must stay green.

1. **The learner decides.** The update check is off until the learner turns it on, or runs only when they ask for it (the default taken; record it). Either way it is visible in settings with a one-line statement of what is contacted and what is sent. Evidence: the settings state, before and after enabling, in the desktop UX audit.
2. **It sends nothing about the learner.** The request carries no learner data, attempt, note, credential, source identifier or stable device identifier. It goes only to the GitHub Releases endpoint of this repository. Evidence: a captured request from a test that fails if any other host, path or field appears.
3. **It tells the truth about versions.** Against a local stand-in for the Releases response, the app shows each of these as a distinct, readable state:
   - a newer release (version, date, release-notes link and download page);
   - already current;
   - an older or pre-release entry, which it ignores;
   - offline or timed out, or a malformed or rate-limited response: a quiet "could not check", and study is unaffected.

   Evidence: a scripted desktop scenario for each state.
4. **It never changes the install.** No file is downloaded, no installer is run and no data is touched. Opening the download page hands off to the system browser. Evidence: the scenarios above show no file written outside the app's settings.
5. **The exception is enforced, not just written down.** The local-only gate allows exactly this request from exactly the desktop package, and fails in each of these cases:
   - another first-party package gains a network import;
   - the CLI gains a network import;
   - the allowed request targets any other host.

   Self-test cases prove each failure. `docs/core-design.md` and the install guide describe the exception.

Constraints:

- No scoring, consent, budget, assessment or sync change.
- No telemetry.
- No certificates.
- The real GitHub API is not required for the evidence.
- Publishing a GitHub Release needs the owner's explicit go-ahead at that time. Until a release exists, the live check reports "already current" or "could not check", and the report says so.

Stop and report at once if, at the start, something only the owner can supply is missing. Otherwise take reversible defaults and disclose them in `docs/decisions/OPEN-QUESTIONS.md`: check frequency, timeout, and opt-in versus on-demand. Stop only for an irreversible or out-of-bounds decision. End with a report that pairs each item above with its evidence and lists the known limits.

## Out of scope

- Auto-download, auto-install, delta updates or a background updater service.
- Update channels and release signing.
- Notifying the learner outside the app window.
- The CLI.
