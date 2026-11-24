import { readFileSync, writeFileSync } from "node:fs";

import { resolve } from "../../lib/repo.ts";

import { desktopUx, makeDesktopUxGate, UX_SIZES } from "../desktop-ux.ts";
import { copyProduct, DESKTOP_UX_SOURCES, patchCopy } from "./fixture.ts";
import type { BrokenCase } from "./fixture.ts";

/**
 * The product with a stylesheet appended that removes every focus outline and
 * paints Today's title in its own background colour: desktop-ux must name both.
 */
async function hideFocusAndTitle(root: string): Promise<string | null> {
  if (copyProduct(root, DESKTOP_UX_SOURCES) === 0) return "no product source was copied into the fixture root";
  const file = resolve(root, "desktop/web/styles/index.css");
  const broken = [":focus-visible{outline:none!important;box-shadow:none!important}", ".hero-title{color:var(--bg)!important}"];
  writeFileSync(file, [readFileSync(file, "utf8"), ...broken, ""].join("\n"));
  return null;
}

/** The product with a content-size switch that sizes the page 16 px short: the viewport precondition must fail. */
async function shrinkTheViewport(root: string): Promise<string | null> {
  if (copyProduct(root, DESKTOP_UX_SOURCES) === 0) return "no product source was copied into the fixture root";
  return patchCopy(
    root,
    "internal/desktop/smoke.go",
    "o.Width, o.Height, o.UseContentSize, o.StateKey = h.contentW, h.contentH, true, \"\"",
    "o.Width, o.Height, o.UseContentSize, o.StateKey = h.contentW-16, h.contentH-16, true, \"\"",
  );
}

/**
 * The product whose Updates row freezes its state hook: the audits' waits read
 * `data-update-state`, so the newer-state step must time out, not pass.
 */
async function freezeUpdateState(root: string): Promise<string | null> {
  if (copyProduct(root, DESKTOP_UX_SOURCES) === 0) return "no product source was copied into the fixture root";
  return patchCopy(root, "desktop/web/src/views/settings.ts", 'data-update-state="${state}"', 'data-update-state="stale"');
}

/**
 * The product whose Updates row breaks every check the settings part added, at once: the check
 * action fabricates its states in the page (no request ever reaches the stub), the version the row
 * shows is hardcoded, and the disclosure no longer names the endpoint. One run must report all three.
 */
async function breakEveryUpdateAssertion(root: string): Promise<string | null> {
  if (copyProduct(root, DESKTOP_UX_SOURCES) === 0) return "no product source was copied into the fixture root";
  // The fabricated states mirror what the loopback stub's v9.9.9 release and its 500 answer render,
  // so the script's waits and the state audits still see the row's real layout.
  const fabricated = [
    "    const prior = data.settings.update;",
    "    data.settings.update = { status: 'checking' };",
    "    void Promise.resolve(",
    "      prior.status === 'done' && prior.result.state === 'newer'",
    "        ? { state: 'unavailable', reason: 'server_error' }",
    "        : {",
    "            state: 'newer',",
    "            version: 'v9.9.9',",
    "            published: '2026-11-18',",
    "            notes_url: 'https://github.com/wxxb789/passquest/releases/tag/v9.9.9',",
    "            download_url: 'https://github.com/wxxb789/passquest/releases/tag/v9.9.9',",
    "          },",
    "    ).then((result) => {",
  ].join("\n");
  return (
    patchCopy(
      root,
      "desktop/web/src/controllers/settings.ts",
      "    data.settings.update = { status: 'checking' };\n    void write((a) => a.checkForUpdate()).then((result) => {",
      fabricated,
    ) ??
    patchCopy(
      root,
      "internal/desktop/study.go",
      "return callSolo(s, func() (string, error) { return update.Version, nil })",
      'return callSolo(s, func() (string, error) { return "0.0.0", nil })',
    ) ??
    patchCopy(root, "desktop/web/src/views/settings.ts", '<span translate="no">GitHub Releases</span> (<code>api.github.com</code>)', '<span translate="no">GitHub Releases</span>')
  );
}

/**
 * The product whose modal layers are shown with `show()` instead of `showModal()`: the layer still
 * declares itself modal (role=dialog aria-modal=true) but nothing blocks the page behind it.
 */
async function showLayerWithoutModal(root: string): Promise<string | null> {
  if (copyProduct(root, DESKTOP_UX_SOURCES) === 0) return "no product source was copied into the fixture root";
  return patchCopy(root, "desktop/web/src/core/render.ts", "dialog.showModal();", "dialog.show();");
}

/**
 * The product whose layers ignore a click outside their box: the scrim's own dismissal never reached
 * the native dialog, so a point on the backdrop leaves the layer open.
 */
async function ignoreOutsideClicks(root: string): Promise<string | null> {
  if (copyProduct(root, DESKTOP_UX_SOURCES) === 0) return "no product source was copied into the fixture root";
  return patchCopy(root, "desktop/web/src/core/render.ts", "    if (e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom) closeLayer();\n", "");
}

/**
 * The product whose practice sheet lost its entry animation: opening it with motion allowed leaves
 * no transform animation on the dialog, so the entry check must name it.
 */
async function dropSheetEntryAnimation(root: string): Promise<string | null> {
  if (copyProduct(root, DESKTOP_UX_SOURCES) === 0) return "no product source was copied into the fixture root";
  return patchCopy(root, "desktop/web/styles/layers.css", "  &.in { animation: sheetin var(--d3) var(--ease) both; }\n", "");
}

export function desktopUxCases(): BrokenCase[] {
  return [
    {
      // Both findings are on Today at the first size: the probes, the motion launch and Today's part still
      // run; the whole matrix and its coverage check run in the desktop-ux gate itself.
      gate: makeDesktopUxGate({ sizes: [UX_SIZES[0]], parts: ["one"] }),
      label: "a stylesheet that removes focus outlines and paints Today's title in its background colour",
      files: {},
      prepare: hideFocusAndTitle,
      postFail: (failures) => {
        const earlier = failures.find((line) => /go build|bun build|viewport precondition|probes:|script failed|seed:/.test(line));
        if (earlier) return `desktop-ux: the mutated stylesheet broke an earlier stage: ${earlier}`;
        if (!failures.some((line) => line.startsWith("no-focus-indicator "))) return "desktop-ux: did not report no-focus-indicator";
        // The audit names an element by its id when it has one (h1#hero-t), not by its classes.
        if (!failures.some((line) => line.startsWith("contrast-fail ") && /\bh1#hero-t\b/.test(line))) {
          return "desktop-ux: did not report contrast-fail on the hero title";
        }
        return null;
      },
    },
    {
      gate: desktopUx,
      label: "a window whose page area is not the size asked for",
      files: {},
      prepare: shrinkTheViewport,
      postFail: (failures) =>
        failures.some((line) => line.startsWith("viewport precondition: ") && line.includes("1280x800") && line.includes("1264x784"))
          ? null
          : `desktop-ux: did not report the viewport precondition: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      // The settings part at the first size only: the update row's newer state is what the waits
      // read, so a frozen hook must fail there rather than let the audit pass on a stale row.
      gate: makeDesktopUxGate({ sizes: [UX_SIZES[0]], parts: ["two"] }),
      label: "an Updates row whose check state never leaves the first one",
      files: {},
      prepare: freezeUpdateState,
      postFail: (failures) => {
        const earlier = failures.find((line) => /go build|bun build|viewport precondition|probes:|seed:/.test(line));
        if (earlier) return `desktop-ux: the frozen update row broke an earlier stage: ${earlier}`;
        return failures.some((line) => line.includes("script failed") && line.includes("the newer state"))
          ? null
          : `desktop-ux: did not fail on the frozen update state: ${failures.slice(0, 3).join("; ")}`;
      },
    },
    {
      // The settings part at the first size only. One broken product must fail all three checks the
      // settings part carries: the stub's request count and path, the version the row shows, and the
      // disclosure naming the endpoint.
      gate: makeDesktopUxGate({ sizes: [UX_SIZES[0]], parts: ["two"] }),
      label: "an Updates row that fabricates its states, misstates its version and drops the endpoint from its disclosure",
      files: {},
      prepare: breakEveryUpdateAssertion,
      postFail: (failures) => {
        const earlier = failures.find((line) => /go build|bun build|viewport precondition|probes:|script failed|seed:/.test(line));
        if (earlier) return `desktop-ux: the broken Updates row broke an earlier stage: ${earlier}`;
        const wanted = [
          ["the stub answered", "the stub's request count and path"],
          ["the Updates row shows version", "the version the row shows"],
          ["disclosure does not name api.github.com", "the disclosure naming the endpoint"],
        ] as const;
        const missing = wanted.filter(([needle]) => !failures.some((line) => line.includes(needle))).map(([, what]) => what);
        if (missing.length > 0) return `desktop-ux: did not fail on ${missing.join(" and ")}`;
        return null;
      },
    },
    {
      // The sheets/dialog part at the first size only: a layer that claims aria-modal without blocking
      // the page leaves controls a learner can really tab to behind it, and that must be reported.
      gate: makeDesktopUxGate({ sizes: [UX_SIZES[0]], parts: ["one"] }),
      label: "a layer shown with dialog.show() while the page behind it stays tabbable",
      files: {},
      prepare: showLayerWithoutModal,
      postFail: (failures) => {
        const earlier = failures.find((line) => /go build|bun build|viewport precondition|probes:|script failed|seed:/.test(line));
        if (earlier) return `desktop-ux: the non-modal layer broke an earlier stage: ${earlier}`;
        return failures.some((line) => line.startsWith("modal-not-trapping "))
          ? null
          : `desktop-ux: did not report modal-not-trapping: ${failures.slice(0, 3).join("; ")}`;
      },
    },
    {
      // The sheets/dialog part at the first size only: the scrim's own pointer dismissal must be a
      // check of its own, so a layer that only closes on Escape or its button is reported.
      gate: makeDesktopUxGate({ sizes: [UX_SIZES[0]], parts: ["one"] }),
      label: "a layer that ignores a click outside its box while the dialog stays open",
      files: {},
      prepare: ignoreOutsideClicks,
      postFail: (failures) => {
        const earlier = failures.find((line) => /go build|bun build|viewport precondition|probes:|script failed|seed:/.test(line));
        if (earlier) return `desktop-ux: the ignored outside click broke an earlier stage: ${earlier}`;
        return failures.some((line) => line.startsWith("outside-click-not-closing "))
          ? null
          : `desktop-ux: did not report outside-click-not-closing: ${failures.slice(0, 3).join("; ")}`;
      },
    },
    {
      // The entry motion only: the prepare and motion launches run the whole time, so no audit
      // part is needed and the run stays short.
      gate: makeDesktopUxGate({ sizes: [UX_SIZES[0]], parts: [] }),
      label: "a practice sheet whose entry animation was removed",
      files: {},
      prepare: dropSheetEntryAnimation,
      postFail: (failures) => {
        const earlier = failures.find((line) => /go build|bun build|viewport precondition|probes:|script failed|seed:/.test(line));
        if (earlier) return `desktop-ux: the removed entry animation broke an earlier stage: ${earlier}`;
        return failures.some((line) => line.startsWith("entry-transition-missing ") && line.includes("sheet transform"))
          ? null
          : `desktop-ux: did not report the missing sheet entry animation: ${failures.slice(0, 3).join("; ")}`;
      },
    },
  ];
}
