# 0022 — The window's title bar belongs to the Pass Quest interface

**State:** Ready (defined 2026-11-22)
**Prerequisite:** 0019 — a modern frontend. Both change the page's layout, styles and the desktop-ux reference shots; this goal builds on 0019's merged frontend and its benchmark.

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; recoverable, idempotent local state; no central account or cloud service. Follow [Core Design](../core-design.md).

Goal 0012 gave the app its owner-chosen "C · Stage" interface. The window around it still uses each system's default title bar. On Windows that is the plain system caption, which does not match the page. The owner saw it during 0019's benchmark runs (2026-11-22) and called it ugly. The owner wants the title bar to look like part of the app.

The desktop framework (MyGo v0.10.0, `docs/windows.md` and `docs/frontend.md` in its module) can hide the system title bar on Windows, macOS and Linux. In that mode the system's window buttons stay, the page is told how much room they take, and the page draws the title area itself. It is configured through the window options in `internal/desktop/desktop.go`.

## Intended outcome

On Windows 11, the latest two macOS majors and Linux, the top of the window reads as part of the Stage interface rather than as system chrome. Everything a learner expects from a title bar still works.

## Done when

Each item below is shown during the run with the evidence named. All of it must hold on the final tree, and the existing gates must stay green.

1. **The title area is the app's.** On all three platforms the system caption is replaced by a title area the page draws, in the Stage design's colours, type and spacing, in both the light and dark appearances the app supports. The system window buttons, where kept, are legible against it. Evidence: screenshots of the today view per OS and appearance at the audited 1280×800 size (Q-0012-05), in the evidence record.
2. **Window behaviour is intact.** On each OS the window can be moved by dragging the title area, maximized and restored by double-clicking it (where that OS does so), minimized, maximized and closed with its buttons, resized from every edge including the top, and snapped (Windows Snap layouts from the maximize button). Interactive controls inside the title area are clickable and do not drag the window. Nothing on any view is hidden under the window buttons, at the minimum window size (880×600) and in full screen. Evidence: a desktop-smoke or desktop-ux case per OS that checks what can be checked mechanically (the title area's reserved room, the controls not covered, the drag region), plus a recorded manual pass for what cannot be checked mechanically (snap, double-click), listed as such.
3. **Keyboard and assistive use are unchanged.** The window title is still announced and shown in the taskbar, the Dock and the window switcher. Keyboard focus order and the existing accessibility checks are unchanged. Evidence: the desktop-ux accessibility checks green, and the window title read back on each OS.
4. **No regression in speed or stability.** The 0019 frontend benchmark shows no regression against the merged 0019 baseline, with any visual difference explained. Evidence: the benchmark record and its allowlist entries.
5. **The reference shots are updated honestly.** The desktop-ux reference shots and any documentation screenshot that shows the window are updated, and every change is the title area. Evidence: a per-shot list of what changed.

Constraints:

- Only the window chrome and the page area it frees change. No change to study flows, scoring, consent, the bound window API's behaviour, or data.
- Keep the system's own window buttons unless an OS cannot show them legibly over the page. Any replacement must behave like the system's, including Snap layouts on Windows, and is disclosed.
- Supported systems stay as goal 0017 defined them.

Stop and report at once if, at the start, something only the owner can supply is missing. Otherwise take reversible defaults and disclose them in `docs/decisions/OPEN-QUESTIONS.md` (Q-0022-NN): the title area's exact height and content, and whether Linux keeps its system decorations where the desktop requires them. Stop only for an irreversible or out-of-bounds decision. End with a report that pairs each item above with its evidence and lists the known limits. The owner's visual review of the screenshots follows the run.

## Out of scope

- Menus, tray icons, or new window types.
- Translucent window materials (Mica, Liquid Glass) beyond what the title area needs to match the page.
- Any change to the study views' layout other than the room the title area takes.
