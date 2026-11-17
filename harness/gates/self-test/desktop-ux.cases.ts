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
  ];
}
