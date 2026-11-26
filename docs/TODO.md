# Deferred work

Work that waits for an outside event or an owner input. Each entry names its trigger and where its
evidence or instructions live. When the trigger fires, start from the named record, not from memory.

| Work | Waits for | Start from |
|---|---|---|
| Goal 0023 item 2: move the four `@earendil-works` Pi packages (chord, pi-ai, pi-durable, pi-schema) to the newest served release, at least 1.6.0, then close 0023 and publish a patch release | The corporate npm feed serving `@earendil-works/*` 1.6.0 or newer for all six lock entries (upstream has 1.6.1; the feed served 1.5.2 on 2026-11-26) | The re-run steps in the P4 section of [the 0023 record](evidence/0023-current-dependencies.md); Q-0023-03 |
| Move MyGo past v0.10.0 and re-read 0022's title-bar room on three OSes. Unreleased fixes on MyGo's main that touch this app: #2189 (Linux hidden title bar room), #2154 (KDE Plasma decorations), #1872 (Windows page full screen), #2197 (Windows speed while hidden) | A tagged MyGo release after v0.10.0 (none as of 2026-11-26; do not pin an untagged commit) | [Goal 0023](goals/0023-current-dependencies.md) constraint on MyGo; the per-OS room readings in [the 0022 record](evidence/0022-integrated-title-bar.md) |
| Goal 0018: macOS upgrades keep the Keychain permission (stable signing identity) | An Apple Developer ID | [Goal 0018](goals/0018-keychain-survives-upgrade.md) |
| Code signing and in-app update | An Apple Developer ID and a Windows code-signing certificate | The goals table in [goals/README.md](goals/README.md) |
| 0022's macOS and Linux manual checks (traffic lights, drag, double-click, resize, minimize, close, inactive look) | A Mac with macOS 26/27, or a Linux desktop; deferred by the owner on 2026-11-26 (Q-0022-05) | The owner list in [the 0022 record](evidence/0022-integrated-title-bar.md) |
| A real Responses provider probe (does a real endpoint or gateway accept `store: false` and `instructions`) | The owner's provider key and endpoint (Q-0021-08) | [The 0021 record](evidence/0021-responses-api.md) |
| Regenerate the 0012 reference sheets and the README's `feedback-coach.webp`, which predate 0021's settings line and 0022's title area | A coordinator pass with the `design/` tooling (Q-0021-06, Q-0022-07) | [evidence/0012/README.md](evidence/0012/README.md) |
| frontend-bench: a macOS launch whose page returns fewer than 20 frame durations (`warmup-a-0: the page returned 19 frame duration(s), want at least 20`) fails the job; the in-job re-measure covers only an unreadable control pair, so this still needs a manual rerun | A decision to extend the re-measure to launch-guard failures, or evidence that it recurs | Run 45814888421 (PR #81); the bench row of [harness.md](harness.md) |
| Telemetry check (low priority) | The owner's go-ahead | — |
