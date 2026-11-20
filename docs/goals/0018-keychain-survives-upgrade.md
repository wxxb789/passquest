# 0018 — macOS upgrades keep the learner's Keychain permission

**State:** Ready (defined 2026-11-20)
**Prerequisite:** None. It can run before or after 0016 and 0017. All three change the macOS package or the release workflow, so do not run them at the same time on one tree.

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; recoverable, idempotent local state; no central account or cloud service. Follow [Core Design](../core-design.md).

On macOS the desktop app keeps the learner's provider key in the Keychain (Q-0011-03). The packages stay unsigned by Apple until the owner has certificates (goal 0013). macOS is a first-tier platform, and the supported versions are the latest two majors (Q-0000-07).

## Intended outcome

The macOS package is ad-hoc signed. According to MyGo's own documentation (`cmd/mygo/devsign.go` in MyGo 0.10.0), an ad-hoc signature changes with every build, and macOS ties a Keychain item's "Always Allow" to the signature's designated requirement. That means each upgrade may look like a different app to macOS, and the learner is asked again before the app can read its stored key. Pass Quest has not observed this yet: the package smoke checks the stored credential only within one build.

After this goal, an upgraded macOS app keeps Keychain access the learner already allowed. The way to get there is a stable, project-owned self-signed signing identity, if the evidence shows it is needed. Gatekeeper's first-launch prompt is unchanged, because a self-signed identity is not trusted by Apple. That is goal 0017's concern.

## Owner decisions (2026-11-20)

- The agent may generate a self-signed code-signing certificate, store its private key and password only in this repository's Actions secrets, and delete every local copy afterwards.
- When the owner later obtains a Developer ID, it replaces this identity.

## Done when

Each item below is shown during the run with the evidence named. All of it must hold on the final tree, and the existing gates must stay green.

1. **The problem is established first.** Before any fix, the run shows whether a key stored through the window by one ad-hoc build can be read without a prompt by the next build installed over it. If it can, stop and report: the goal's premise is false, and no identity is added. Evidence: a recorded macOS run of the two builds, with the designated requirement of each (`codesign` output).
2. **Upgrades keep Keychain access.**
   - Two packages built from different commits carry the same designated requirement.
   - A key stored by the first package is read by the second, installed over it, without a Keychain prompt.
   - Evidence: a CI job on a macOS runner that builds both, installs one over the other, and runs the credential case through the smoke hook. Where CI cannot observe a prompt directly, the evidence record says what was observed instead.
3. **The private key cannot leak.**
   - The private key exists only in Actions secrets.
   - The certificate's public fingerprint is recorded in the docs, so a learner can check what signed their app.
   - No log, artifact, package or repository file carries the key.
   - The release keeps its existing integrity checks.
   - Evidence: the secrets gate, the package contents scan, and a review of the job logs.
4. **The learner is told the truth.** The install guide (both languages, if goal 0017 has landed) says that the app is signed with a project identity, that Gatekeeper still asks on first launch, and what the identity does and does not mean. Evidence: the docs-links gate and the owner's review.

Constraints:

- No Apple Developer ID, notarization or change to the Gatekeeper experience.
- Windows and Linux packages are unaffected.
- No change to where or how credentials are stored.

Stop and report at once if, at the start, something only the owner can supply is missing, or if `gh` cannot set the secrets. Otherwise take reversible defaults and disclose them in `docs/decisions/OPEN-QUESTIONS.md`: the certificate's subject, validity and key type. Stop only for an irreversible or out-of-bounds decision. End with a report that pairs each item above with its evidence and lists the known limits.

## Out of scope

- Apple-trusted signing, notarization and Windows signing.
- Any change to the Gatekeeper flow (goal 0017).
