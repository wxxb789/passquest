import { existsSync, mkdtempSync, readdirSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { bundleFrontend, FRONTEND_ENTRY } from "../lib/desktop.ts";
import { outputLines } from "../lib/process.ts";
import { readEach, resolve } from "../lib/repo.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";

const FRONTEND = "desktop/web";
const TSCONFIG = `${FRONTEND}/tsconfig.json`;
const README = `${FRONTEND}/README.md`;

// Q-0019-01's pinned values; the README's engine-baseline section must carry them too.
const TARGET = "ES2024";
const LIB = ["ES2024", "DOM", "DOM.Iterable"];
const README_VALUES = ['"target": "ES2024"', '"lib": ["ES2024","DOM","DOM.Iterable"]'];

const VENDOR = /(?:-webkit-|-moz-|-ms-)[a-z][a-z-]*/gi;
const SCANNED = /\.(css|ts|js|mjs)$/;

export interface AllowlistEntry {
  construct: string;
  side: "source" | "built";
  reason: string;
  fires: (token: string) => boolean;
}

export const ALLOWLIST: AllowlistEntry[] = [
  {
    construct: "-webkit-font-smoothing",
    side: "source",
    reason:
      "WebKit-only property by design; no standard equivalent keeps the design's antialiased text weight",
    fires: (token) => token === "-webkit-font-smoothing",
  },
  {
    construct: "-webkit-scrollbar",
    side: "source",
    reason: "WebKit's scrollbar pseudo-element family; no standard equivalent gives the design's 8 px thumb",
    fires: (token) => ["-webkit-scrollbar", "-webkit-scrollbar-button", "-webkit-scrollbar-corner"].includes(token),
  },
  {
    construct: "-webkit-scrollbar-track",
    side: "source",
    reason: "WebKit's scrollbar pseudo-element family; no standard equivalent gives the design's 8 px thumb",
    fires: (token) => token === "-webkit-scrollbar-track",
  },
  {
    construct: "-webkit-scrollbar-thumb",
    side: "source",
    reason: "WebKit's scrollbar pseudo-element family; no standard equivalent gives the design's 8 px thumb",
    fires: (token) => token === "-webkit-scrollbar-thumb",
  },
];

export interface VendorScan {
  violations: string[];
  unfired: string[];
  tokens: number;
}

export function scanVendor(
  files: string[],
  contents: Array<Buffer | null>,
  allowlist: AllowlistEntry[],
  side: AllowlistEntry["side"] = "source",
): VendorScan {
  const fired = new Set<string>();
  const violations: string[] = [];
  let tokens = 0;
  for (const [index, file] of files.entries()) {
    const bytes = contents[index];
    if (bytes === null) {
      violations.push(`${file}: unreadable`);
      continue;
    }
    const lines = bytes.toString("utf8").split(/\r?\n/);
    for (const [lineIndex, line] of lines.entries()) {
      const matches = line.match(VENDOR) ?? [];
      tokens += matches.length;
      for (const token of matches) {
        const entry = allowlist.find((candidate) => (side === "source" ? candidate.side === "source" : true) && candidate.fires(token));
        if (entry) {
          fired.add(entry.construct);
          continue;
        }
        violations.push(`${file}:${lineIndex + 1}: ${token} is vendor-prefixed and not on the allowlist`);
      }
    }
  }
  return {
    violations,
    tokens,
    unfired: allowlist
      // Only the scanned side's entries must fire; the source scan is where the source constructs are read from.
      .filter((entry) => entry.side === side && !fired.has(entry.construct))
      .map((entry) => entry.construct),
  };
}

interface SideScan {
  problems: string[];
  tokens: number;
}

async function sourceProblems(root: string): Promise<SideScan> {
  const files = readdirSync(resolve(root, FRONTEND), { recursive: true, encoding: "utf8" })
    .map((file) => file.split(path.sep).join("/"))
    .filter((file) => SCANNED.test(file))
    .map((file) => `${FRONTEND}/${file}`)
    .sort();
  const scan = scanVendor(files, await readEach(root, files), ALLOWLIST, "source");
  return {
    tokens: scan.tokens,
    problems: [
      ...scan.violations,
      ...scan.unfired.map((construct) => `${FRONTEND}: allowlist entry ${construct} matches nothing in the source`),
    ],
  };
}

async function builtProblems(ctx: GateContext): Promise<SideScan> {
  const outDir = mkdtempSync(path.join(tmpdir(), "passquest-frontend-baseline-"));
  try {
    const bundle = await bundleFrontend(ctx.root, outDir);
    if (bundle.code !== 0) {
      return { tokens: 0, problems: [`bun build ${FRONTEND_ENTRY} failed (exit ${bundle.code})`, ...outputLines(bundle)] };
    }
    const files = readdirSync(outDir, { encoding: "utf8" })
      .filter((file) => SCANNED.test(file))
      .sort();
    if (files.length === 0) return { tokens: 0, problems: [`bun build ${FRONTEND_ENTRY} emitted no scanned file`] };
    const scan = scanVendor(files, await readEach(outDir, files), ALLOWLIST, "built");
    return {
      tokens: scan.tokens,
      problems: [
        ...scan.violations.map((violation) => `built ${violation}`),
        ...scan.unfired.map((construct) => `built: allowlist entry ${construct} matches nothing in the bundle`),
      ],
    };
  } finally {
    rmSync(outDir, { recursive: true, force: true });
  }
}

export const frontendBaseline: Gate = {
  id: "frontend-baseline",
  title: "frontend baseline",
  async run(ctx) {
    if (!existsSync(resolve(ctx.root, FRONTEND))) return skip(`no ${FRONTEND} directory`);
    const problems: string[] = [];
    const config = JSON.parse(readFileSync(resolve(ctx.root, TSCONFIG), "utf8")) as {
      compilerOptions?: { target?: unknown; lib?: unknown };
    };
    const compilerOptions = config.compilerOptions;
    if (!compilerOptions) {
      problems.push(`${TSCONFIG}: no compilerOptions`);
    } else {
      if (compilerOptions.target !== TARGET) {
        problems.push(
          `${TSCONFIG}: target is ${JSON.stringify(compilerOptions.target)}, the documented baseline is "${TARGET}"`,
        );
      }
      const lib = compilerOptions.lib;
      if (!Array.isArray(lib) || lib.join(",") !== LIB.join(",")) {
        problems.push(`${TSCONFIG}: lib is ${JSON.stringify(lib)}, the documented baseline is ${JSON.stringify(LIB)}`);
      }
    }
    const readme = readFileSync(resolve(ctx.root, README), "utf8");
    for (const value of README_VALUES) {
      if (!readme.includes(value)) {
        problems.push(
          `${README}: the engine-baseline section does not carry ${value} (the gate pins the README to the tsconfig it documents)`,
        );
      }
    }
    const source = await sourceProblems(ctx.root);
    const built = await builtProblems(ctx);
    problems.push(...source.problems, ...built.problems);
    if (problems.length > 0) return fail(problems);
    return pass(
      `${TARGET} target/lib pinned, README carries both, all ${ALLOWLIST.length} allowlist entries fire on their own side; ${source.tokens} vendor token(s) in the source, ${built.tokens} in the bundle`,
    );
  },
};
