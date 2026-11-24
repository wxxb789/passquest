# 0019 — A modern frontend written for the engines Pass Quest supports

**State:** Ready (defined 2026-11-20)
**Prerequisite:** 0017 — Install on current Windows and macOS. 0017 makes Windows 11+ and macOS 26+ the supported systems, which is what allows this goal to drop older engines.

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; recoverable, idempotent local state; no central account or cloud service. Follow [Core Design](../core-design.md).

The desktop window is a MyGo web view: WebView2 on Windows and WKWebView on macOS. Linux's WebKitGTK is second-tier (Q-0000-07). The frontend is plain TypeScript with no framework, bundled by Bun (Q-0011-02, `desktop/web/README.md`). Its look is the owner-chosen "C · Stage" design of goal 0012, audited by the desktop-ux gate.

## Owner decision (2026-11-20)

Pass Quest supports only Windows 11+ and the latest two macOS majors. The owner therefore wants the frontend written as modern code for those engines: easier to write and maintain, and faster where modern platform features allow. This is recorded in Q-0000-09.

## Intended outcome

Today the frontend still carries code for older engines. It targets ES2022. Its stylesheets carry 135 vendor-prefixed or compatibility constructs (for example `-webkit-backdrop-filter` beside `backdrop-filter`), and its interactions are built without newer platform features. Heavy translucent layers (`backdrop-filter` blur) are used throughout, and their rendering cost has never been measured.

After this goal:

- The frontend is written against the baseline engines it actually runs on: current WebView2 on Windows 11, and WKWebView on macOS 26.
- Workarounds for older engines are gone.
- Native platform features replace hand-built ones where that makes the code simpler or faster.
- Performance is measured and is better or the same, never worse.

The C · Stage design and every behaviour stay as they are.

## Done when

Each item below is shown during the run with the evidence named. All of it must hold on the final tree, and the existing gates must stay green.

1. **The baseline is written down.** The engine baseline is the oldest engine each first-tier platform can run. `desktop/web/README.md` and `docs/conventions.md` name it, along with the JavaScript and CSS feature level it allows and what happens on Linux's WebKitGTK (second-tier: it must still render and work). Evidence: the docs, and the TypeScript and bundle targets that match them.
2. **Old-engine workarounds are gone.** Prefixes and fallbacks that the baseline engines no longer need are removed. Any that remain are listed with the reason each one is still needed. Evidence: a before/after count in the evidence record, and a gate check that a removed workaround cannot silently return. Choose the check's form; a self-test case must prove it fails.
3. **Modern features are used where they pay off.** Every place where a native platform feature replaced hand-built code (for example native dialogs, popovers, CSS nesting, layers or container queries, or transitions) is listed, with what it removed or simplified. A modern feature that would change the design or a behaviour is left out and disclosed. Evidence: the list in the evidence record, plus the desktop-ux and desktop smoke gates passing unchanged.
4. **Performance is measured, before and after.** On Windows and macOS, the run measures:
   - time from launch to a ready Today screen;
   - the cost of the main interactions: answering, opening feedback, opening the coach, and switching rooms;
   - rendering cost of the translucent layers;
   - the frontend bundle size.

   Each measurement has a stated method, sample count and spread. No metric gets worse beyond its measured noise. Where the run deliberately pays for a change, the record says why. Evidence: the tables in the evidence record, from CI or a stated local machine.
5. **Nothing the learner sees changes unintentionally.** The design, text, keyboard behaviour, accessibility audits and every behaviour stay as they are. Evidence: desktop-ux (all states, modes and sizes), desktop smoke, and package smoke pass; any intended visual difference is shown and justified.

Constraints:

- No framework or new runtime dependency unless the evidence shows a clear gain. Record any such addition in `OPEN-QUESTIONS.md`.
- No change to the Go core, the bound API, scoring, consent or data.
- Linux keeps working, but no Linux-only polish.

Stop and report at once if, at the start, something only the owner can supply is missing. Otherwise take reversible defaults and disclose them in `docs/decisions/OPEN-QUESTIONS.md`: the exact engine baseline, the measurement method, and which features were adopted. Stop only for an irreversible or out-of-bounds decision. End with a report that pairs each item above with its evidence and lists the known limits.

## Out of scope

- A redesign, a new visual language, or new screens.
- A framework migration as a goal in itself.
- Performance work in the Go core or the agent hosts (0016 covers host start-up).
