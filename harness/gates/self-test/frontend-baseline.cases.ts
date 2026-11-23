import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { bundleFrontend } from "../../lib/desktop.ts";
import { resolve } from "../../lib/repo.ts";
import { ALLOWLIST, frontendBaseline, scanVendor } from "../frontend-baseline.ts";
import type { AllowlistEntry } from "../frontend-baseline.ts";
import type { BrokenCase } from "./fixture.ts";

const BASELINE: Record<string, string> = {
  "desktop/web/tsconfig.json": JSON.stringify({
    compilerOptions: {
      target: "ES2024",
      module: "ESNext",
      moduleResolution: "bundler",
      lib: ["ES2024", "DOM", "DOM.Iterable"],
      types: [],
      strict: true,
      noEmit: true,
    },
    include: ["src/**/*.ts"],
  }),
  "desktop/web/README.md": '## Engine baseline\n\n| tsconfig key | Value |\n|---|---|\n| target | `"target": "ES2024"` |\n| lib | `"lib": ["ES2024","DOM","DOM.Iterable"]` |\n',
  "desktop/web/src/main.ts": "import './styles.css';\nimport './extra.css';\nexport const ok = 1;\n",
  "desktop/web/src/extra.css": ".x { color: #000; }\n",
  "desktop/web/src/styles.css": [
    "body { -webkit-font-smoothing: antialiased; }",
    "::-webkit-scrollbar { width: 8px; }",
    "::-webkit-scrollbar-track, ::-webkit-scrollbar-corner { background: transparent; }",
    "::-webkit-scrollbar-thumb { background: #000; }",
    "::-webkit-scrollbar-button { display: none; }",
    "",
  ].join("\n"),
};

const TRACKED = [
  "desktop/web/tsconfig.json",
  "desktop/web/README.md",
  "desktop/web/src/main.ts",
  "desktop/web/src/styles.css",
  "desktop/web/src/extra.css",
];

const ONLY_FONT: [AllowlistEntry] = [
  { construct: "-webkit-font-smoothing", side: "source", reason: "test", fires: (token) => token === "-webkit-font-smoothing" },
];

const buildFailed = (failures: string[]): string | null =>
  failures.some((line) => line.startsWith("bun build") && line.includes("failed (exit"))
    ? "frontend-baseline: the fixture's bun build failed, so the built-side scan never ran"
    : null;

export const frontendBaselineCases = (): BrokenCase[] => [
  {
    gate: frontendBaseline,
    label: "frontend baseline: the tsconfig target is ES2022",
    files: {
      ...BASELINE,
      "desktop/web/tsconfig.json": BASELINE["desktop/web/tsconfig.json"]!.replace('"ES2024"', '"ES2022"'),
      "desktop/web/README.md": '## Engine baseline\n\n| target | `"target": "ES2022"` |\n',
    },
    tracked: TRACKED,
    postFail: (failures) => {
      const broken = buildFailed(failures);
      if (broken) return broken;
      const named = failures.some((line) => line.includes('target is "ES2022"') && line.includes("documented baseline"));
      return named ? null : "frontend-baseline: the ES2022 target is not reported as drift from the documented baseline";
    },
  },
  {
    gate: frontendBaseline,
    label: "frontend baseline: a stray -webkit-backdrop-filter in a stylesheet",
    files: {
      ...BASELINE,
      "desktop/web/src/extra.css": ".a { -webkit-backdrop-filter: blur(4px); }\n",
    },
    tracked: TRACKED,
    postFail: (failures) => {
      const broken = buildFailed(failures);
      if (broken) return broken;
      const source = failures.some(
        (line) =>
          line.includes("desktop/web/src/extra.css:") &&
          line.includes("-webkit-backdrop-filter is vendor-prefixed and not on the allowlist"),
      );
      if (!source) return `frontend-baseline: the stray construct is not named with its file: ${failures.join(" | ")}`;
      const built = failures.some(
        (line) => line.startsWith("built ") && line.includes("-webkit-backdrop-filter is vendor-prefixed and not on the allowlist"),
      );
      return built ? null : `frontend-baseline: the stray construct is not reported in the bundle it travels into: ${failures.join(" | ")}`;
    },
  },
  {
    gate: frontendBaseline,
    label: "frontend baseline: a hand-written -moz-any in a stylesheet",
    files: {
      ...BASELINE,
      "desktop/web/src/styles.css": BASELINE["desktop/web/src/styles.css"]!.replace(
        "body { -webkit-font-smoothing: antialiased; }",
        'body { -webkit-font-smoothing: antialiased; }\n.note:-moz-any(.q) .st { color: #999; }',
      ),
    },
    tracked: TRACKED,
    postFail: (failures) => {
      const broken = buildFailed(failures);
      if (broken) return broken;
      const named = failures.some(
        (line) =>
          line.includes("desktop/web/src/styles.css:") &&
          line.includes(": -moz-any is vendor-prefixed and not on the allowlist"),
      );
      if (!named) return `frontend-baseline: the hand-written -moz-any is not reported with its file: ${failures.join(" | ")}`;
      if (failures.some((line) => line.includes("built") && line.includes("is vendor-prefixed") && !line.includes("-moz-any"))) {
        return "frontend-baseline: blamed a construct the source does not carry on the built scan";
      }
      return null;
    },
  },
  {
    gate: frontendBaseline,
    label: "frontend baseline: an allowlist entry that matches nothing in the source",
    files: {
      ...BASELINE,
      "desktop/web/src/styles.css": BASELINE["desktop/web/src/styles.css"]!.replace(
        "body { -webkit-font-smoothing: antialiased; }\n",
        "",
      ),
    },
    tracked: TRACKED,
    postFail: (failures) => {
      const broken = buildFailed(failures);
      if (broken) return broken;
      const source = failures.some(
        (line) => line.includes("allowlist entry -webkit-font-smoothing matches nothing in the source") && !line.includes("built"),
      );
      return source ? null : `frontend-baseline: the unfired source entry is not named: ${failures.join(" | ")}`;
    },
  },
  {
    gate: frontendBaseline,
    label: "frontend baseline: the README stops carrying the pinned lib value",
    files: {
      ...BASELINE,
      "desktop/web/README.md": '## Engine baseline\n\n| tsconfig key | Value |\n|---|---|\n| target | `"target": "ES2024"` |\n',
    },
    tracked: TRACKED,
    postFail: (failures) => {
      const broken = buildFailed(failures);
      if (broken) return broken;
      const named = failures.some(
        (line) =>
          line.startsWith("desktop/web/README.md:") &&
          line.includes("does not carry") &&
          line.includes('"lib": ["ES2024","DOM","DOM.Iterable"]'),
      );
      return named ? null : `frontend-baseline: the dropped README lib pin is not reported: ${failures.join(" | ")}`;
    },
  },
  {
    gate: frontendBaseline,
    label: "frontend baseline: the tsconfig lib drifts from the documented baseline",
    files: {
      ...BASELINE,
      "desktop/web/tsconfig.json": BASELINE["desktop/web/tsconfig.json"]!.replace(
        '["ES2024","DOM","DOM.Iterable"]',
        '["ES2023","DOM","DOM.Iterable"]',
      ),
    },
    tracked: TRACKED,
    postFail: (failures) => {
      const broken = buildFailed(failures);
      if (broken) return broken;
      const named = failures.some(
        (line) =>
          line.includes("desktop/web/tsconfig.json:") &&
          line.includes("lib is") &&
          line.includes('"ES2023"') &&
          line.includes("documented baseline"),
      );
      return named ? null : `frontend-baseline: the drifted lib is not reported as drift from the documented baseline: ${failures.join(" | ")}`;
    },
  },
  {
    gate: frontendBaseline,
    label: "frontend baseline: the bundler's -webkit-any/-moz-any injection from a :not(.q, .c) list",
    files: {
      ...BASELINE,
      "desktop/web/src/styles.css": BASELINE["desktop/web/src/styles.css"]!.replace(
        "body { -webkit-font-smoothing: antialiased; }",
        "body { -webkit-font-smoothing: antialiased; }\n.notes-list .note:not(.q, .c) .st { color: #999; }",
      ),
    },
    tracked: TRACKED,
    postFail: (failures) => {
      const broken = buildFailed(failures);
      if (broken) return broken;
      const missing = ["-webkit-any", "-moz-any"].filter(
        (token) =>
          !failures.some(
            (line) => line.startsWith("built ") && line.includes(`${token} is vendor-prefixed and not on the allowlist`),
          ),
      );
      return missing.length === 0
        ? null
        : `frontend-baseline: the injected ${missing.join(" and ")} is not named in the bundle: ${failures.join(" | ")}`;
    },
  },
  {
    gate: frontendBaseline,
    label: "frontend baseline: a case-varied -Webkit-backdrop-filter in a stylesheet",
    files: {
      ...BASELINE,
      "desktop/web/src/extra.css": ".a { -Webkit-backdrop-filter: blur(4px); }\n",
    },
    tracked: TRACKED,
    postFail: (failures) => {
      const broken = buildFailed(failures);
      if (broken) return broken;
      const source = failures.some(
        (line) =>
          line.startsWith("desktop/web/src/extra.css:") &&
          line.includes("-Webkit-backdrop-filter is vendor-prefixed and not on the allowlist"),
      );
      if (!source) return `frontend-baseline: the case-varied construct is not rejected in the source: ${failures.join(" | ")}`;
      const built = failures.some(
        (line) => line.startsWith("built ") && line.includes("-Webkit-backdrop-filter is vendor-prefixed and not on the allowlist"),
      );
      return built ? null : `frontend-baseline: Bun kept the case-varied construct and the built scan did not reject it: ${failures.join(" | ")}`;
    },
  },
];

export async function frontendBaselineUnitProblem(): Promise<string | null> {
  const css = Buffer.from(".a { -webkit-backdrop-filter: blur(4px); }\n.b { -webkit-mask-image: none; }\n", "utf8");
  const scan = scanVendor(["x.css"], [css], ONLY_FONT);
  if (scan.unfired.join() !== "-webkit-font-smoothing") {
    return `frontend-baseline: an entry matching nothing is not reported (saw ${JSON.stringify(scan.unfired)})`;
  }
  if (scan.violations.length !== 2 || !scan.violations.every((line) => line.includes("is vendor-prefixed and not on the allowlist"))) {
    return `frontend-baseline: covered constructs are not reported as violations (saw ${scan.violations.join("; ")})`;
  }
  const covered = scanVendor(["x.css"], [Buffer.from(".a { -webkit-font-smoothing: antialiased; }\n", "utf8")], ONLY_FONT);
  if (covered.violations.length !== 0 || covered.unfired.length !== 0) {
    return `frontend-baseline: the allowlist entry did not cover its construct (saw ${covered.violations.join("; ")})`;
  }
  const builtOnly: [AllowlistEntry] = [
    { construct: "-webkit-any", side: "built", reason: "test", fires: (token) => token === "-webkit-any" },
  ];
  const stitched = scanVendor(["x.css"], [Buffer.from(".a:-webkit-any(.b) { color: red; }\n", "utf8")], builtOnly, "source");
  if (stitched.violations.join() !== "x.css:1: -webkit-any is vendor-prefixed and not on the allowlist") {
    return `frontend-baseline: a built-side entry covered a token stitched into the source (saw ${stitched.violations.join("; ") || "no violation"})`;
  }
  if (stitched.unfired.length !== 0) {
    return `frontend-baseline: the source scan demanded a built-side entry (saw ${stitched.unfired.join(", ")})`;
  }
  const builtSide = scanVendor(["x.css"], [Buffer.from(".a:-webkit-any(.b) { color: red; }\n", "utf8")], builtOnly, "built");
  if (builtSide.violations.length !== 0 || builtSide.unfired.length !== 0) {
    return `frontend-baseline: a built-side entry did not cover its own side (saw ${[...builtSide.violations, ...builtSide.unfired].join("; ")})`;
  }
  const fixture = mkdtempSync(path.join(tmpdir(), "passquest-fb-inject-"));
  try {
    mkdirSync(resolve(fixture, "desktop/web/src"), { recursive: true });
    writeFileSync(
      resolve(fixture, "desktop/web/src/main.ts"),
      'import "./styles.css";\nexport const ok = 1;\n',
    );
    writeFileSync(
      resolve(fixture, "desktop/web/src/styles.css"),
      '.notes-list .note:not(.q, .c) .st { color: #999; }\n' +
        'html[lang="zh-CN"] :is(.callstrip, .rmeta, .pending, .chip, .sat, .cond, .promise, .kv, .svc, .fine, .eyebrow, .reason, .q-note, .pv-lab, .an-top, .es-f, .reg-h p, .hmeta) { word-break: keep-all; }\n',
    );
    const outDir = path.join(fixture, "bundled");
    const bundle = await bundleFrontend(fixture, outDir);
    if (bundle.code !== 0) return `frontend-baseline: the injection fixture did not bundle: ${bundle.stderr || bundle.stdout}`;
    const built = readFileSync(path.join(outDir, "main.css"), "utf8");
    // Bun injects `:-webkit-any(`/`:-moz-any(` only for a selector list whose every argument is one
    // compound selector: the fixture's own `.note:not(.q, .c)` list gets both twins, its `:is` list
    // with the complex `.reg-h p` argument gets none.
    const webkit = (built.match(/-webkit-any/g) ?? []).length;
    const moz = (built.match(/-moz-any/g) ?? []).length;
    if (webkit !== 1 || moz !== 1) {
      return `frontend-baseline: the injection fixture bundled ${webkit} :-webkit-any( and ${moz} :-moz-any( token(s), want 1 and 1 (the :not(.q, .c) list's twins; the complex-argument :is list must inject none)`;
    }
    // The fixture's entry is main.ts, so Bun writes main.css; name it as the built scan does.
    const scan = scanVendor(["main.css"], [Buffer.from(built, "utf8")], ALLOWLIST, "built");
    const rejected = scan.violations.filter((line) => line.includes("-webkit-any") || line.includes("-moz-any"));
    if (rejected.length !== webkit + moz) {
      return `frontend-baseline: the built scan rejected ${rejected.length} of ${webkit + moz} injected -any token(s)`;
    }
    return null;
  } finally {
    rmSync(fixture, { recursive: true, force: true });
  }
}
