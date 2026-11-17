import { mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { BASELINE, EXPECTED_NATIVE, EXPECTED_PAGES, nonSpaceChars } from "../lib/baseline.ts";
import { descendants, parsePsLine, parsePsTable, sameProcess } from "../lib/desktop.ts";
import type { ProcessInfo } from "../lib/desktop.ts";
import { groupProblems, workflowGroups } from "../lib/gate-groups.ts";
import { executable, run } from "../lib/process.ts";
import { context, resolve, workingTreeFiles } from "../lib/repo.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";

import { adaptiveSmoke } from "./adaptive-smoke.ts";
import { agentSmoke } from "./agent-smoke.ts";
import { assessSmoke } from "./assess-smoke.ts";
import { baseline } from "./baseline.ts";
import { desktopSmoke } from "./desktop-smoke.ts";
import { coverageProblems, desktopUx, makeDesktopUxGate, UX_SIZES, UX_STATES } from "./desktop-ux.ts";
import type { Audited } from "./desktop-ux.ts";
import { docsLinks } from "./docs-links.ts";
import { findSmoke } from "./find-smoke.ts";
import { goBuild } from "./go-build.ts";
import { goTest } from "./go-test.ts";
import { goTidy } from "./go-tidy.ts";
import { goVet } from "./go-vet.ts";
import { gofmt } from "./gofmt.ts";
import { goldenContract } from "./golden-contract.ts";
import { importSmoke } from "./import-smoke.ts";
import { localOnly } from "./local-only.ts";
import { noteSmoke } from "./note-smoke.ts";
import { portablePaths } from "./portable-paths.ts";
import { providerSmoke } from "./provider-smoke.ts";
import { secrets } from "./secrets.ts";
import { tacticSmoke } from "./tactic-smoke.ts";
import { tutorSmoke } from "./tutor-smoke.ts";
import { typecheck } from "./typecheck.ts";

const GO_MOD = "module example.com/selftest\n\ngo 1.27.3\n";

interface BrokenCase {
  gate: Gate;
  label: string;
  files: Record<string, string>;
  tracked?: string[];
  /** Runs after the fixture files exist and before the gate (usually git state). */
  prepare?: (root: string) => Promise<string | null>;
  postFail?: (failures: string[]) => string | null;
}

/** A fixture the gate must accept; proves a check is not unconditionally red. */
interface PassCase {
  gate: Gate;
  label: string;
  files: Record<string, string>;
  tracked?: string[];
  prepare?: (root: string) => Promise<string | null>;
  postPass?: (detail: string) => string | null;
}

interface BaselineFixtureOptions {
  wrongCharsPage?: number;
  /** Writes U+FEFF into this page; the manifest uses the pre-fix \s count. */
  bomPage?: number;
  /** Renames one page file and manifest row, leaving a gap at `from`. */
  renumber?: { from: number; to: number };
}

/** A complete baseline tree; options break exactly one documented property. */
function baselineFixture(options: BaselineFixtureOptions = {}): { files: Record<string, string>; tracked: string[] } {
  const files: Record<string, string> = {};
  const tracked: string[] = [];
  const rows = ["page\tsection\tsource\tbytes\tchars_no_ws"];
  for (let page = 1; page <= EXPECTED_PAGES; page++) {
    const text = page === options.bomPage ? `page ${page} text\uFEFF\n` : `page ${page} text\n`;
    const number = options.renumber && page === options.renumber.from ? options.renumber.to : page;
    const relative = `${BASELINE}/pages/page-${String(number).padStart(3, "0")}.txt`;
    files[relative] = text;
    tracked.push(relative);
    const source = page <= EXPECTED_NATIVE ? "native" : "ocr";
    let chars = nonSpaceChars(text);
    if (page === options.wrongCharsPage) chars += 1;
    if (page === options.bomPage) chars -= 1;
    rows.push([number, `section-${page}`, source, Buffer.byteLength(text), chars].join("\t"));
  }
  files[`${BASELINE}/manifest.tsv`] = `${rows.join("\n")}\n`;
  files[`${BASELINE}/README.md`] = "The extracted text is unreviewed.\n";
  tracked.push(`${BASELINE}/manifest.tsv`, `${BASELINE}/README.md`);
  return { files, tracked };
}

async function workingTreeDiscoveryProblem(): Promise<string | null> {
  const root = mkdtempSync(path.join(tmpdir(), "passquest-repo-selftest-"));
  try {
    const init = await run(executable("git"), ["init", "-q"], { cwd: root });
    if (init.code !== 0) return `working-tree discovery: git init failed: ${init.stderr || init.stdout}`;
    writeFileSync(resolve(root, ".gitignore"), "ignored.txt\n");
    writeFileSync(resolve(root, "tracked.txt"), "tracked\n");
    writeFileSync(resolve(root, "untracked.txt"), "untracked\n");
    writeFileSync(resolve(root, "ignored.txt"), "ignored\n");
    const add = await run(executable("git"), ["add", "tracked.txt"], { cwd: root });
    if (add.code !== 0) return `working-tree discovery: git add failed: ${add.stderr || add.stdout}`;
    const files = (await workingTreeFiles(root)).sort();
    const expected = [".gitignore", "tracked.txt", "untracked.txt"];
    if (files.join(",") !== expected.join(",")) {
      return `working-tree discovery = [${files.join(", ")}], want [${expected.join(", ")}]`;
    }
    return null;
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

/**
 * The app's process tree is told apart from processes that only carry its pid
 * as a stale parent id (Windows keeps a dead parent's id and reuses it): while
 * it runs, and after it exits when a new process has taken its pid.
 */
export function processTreeProblem(): string | null {
  const row = (pid: number, ppid: number, started: number, command = "x.exe"): ProcessInfo => ({ pid, ppid, started, command });
  const app = row(100, 1, 2000, "passquest-desktop.exe");
  const staleDaemon = [row(200, 100, 1000, "omp.exe"), row(201, 200, 1100, "msedge.exe")];
  const helpers = [row(300, 100, 2100, "msedgewebview2.exe"), row(301, 300, 2200, "msedgewebview2.exe")];
  const pids = (list: ProcessInfo[]) => list.map((entry) => entry.pid).sort().join(",");
  const running = pids(descendants([app, ...staleDaemon, ...helpers], app.pid));
  if (running !== "300,301") return `process tree while the app runs = [${running}], want [300,301] (a stale parent id is not a child)`;
  const reused = [row(100, 7, 5000, "svchost.exe"), row(400, 100, 5100, "conhost.exe")];
  const after = pids(descendants([...staleDaemon, ...helpers, ...reused], { pid: app.pid, started: app.started }, 3000));
  if (after !== "300,301") return `process tree after the app exited = [${after}], want [300,301] (a reused pid's tree is not the app's)`;
  if (sameProcess(row(300, 100, 2100, "msedgewebview2.exe"), row(300, 9, 6000, "msedgewebview2.exe"))) {
    return "a process that reused a remembered pid and name counts as the remembered one";
  }
  const ps = parsePsLine(" 4242     1 Mon Nov  6 09:05:07 2026 /usr/bin/passquest-desktop --flag a b");
  const want = Date.UTC(2026, 10, 6, 9, 5, 7);
  if (!ps || ps.pid !== 4242 || ps.ppid !== 1 || ps.started !== want || ps.command !== "/usr/bin/passquest-desktop --flag a b") {
    return `ps line parsed as ${JSON.stringify(ps)}`;
  }
  // A table the parser cannot read is a failed probe, never an empty one: the orphan check would see nothing.
  for (const table of ["", "  PID  PPID STARTED\n", "4242 1 2026-11-06T09:05:07 /usr/bin/x\n", " 4242     1 Mon Nov  6 09:05:07 2026 /x\nkthreadd\n"]) {
    const parsed = parsePsTable(table);
    if (!("error" in parsed)) return `unreadable ps table ${JSON.stringify(table)} parsed as ${JSON.stringify(parsed.items)}`;
  }
  // US DST ends 2026-11-01 06:00 UTC: local 01:30 EDT (parent) comes before local 01:10 EST (child). In a DST zone
  // the child is still the parent's, because ps runs under TZ=UTC0 and its times are read as UTC.
  const zone = process.env.TZ;
  process.env.TZ = "America/New_York";
  try {
    const fallBack = parsePsTable(" 500     1 Sun Nov  1 05:30:00 2026 /usr/bin/app\n 501   500 Sun Nov  1 06:10:00 2026 /usr/bin/helper\n");
    if ("error" in fallBack) return `DST fall-back table: ${fallBack.error}`;
    const child = fallBack.items.find((entry) => entry.pid === 501);
    if (child?.started !== Date.UTC(2026, 10, 1, 6, 10, 0)) return `ps start under TZ=${process.env.TZ} = ${child?.started}, want UTC`;
    const kept = pids(descendants(fallBack.items, 500));
    if (kept !== "501") return `process tree across the DST fall-back = [${kept}], want [501]`;
  } finally {
    if (zone === undefined) delete process.env.TZ;
    else process.env.TZ = zone;
  }
  return null;
}

/**
 * Backslash escapes a plain (untagged) multi-line template literal cooks away
 * before a page script reaches the window: `\s` arrives as `s`. Only `\\`,
 * `` \` `` and `\$` survive as written; a page script that needs a regex escape
 * writes `\\s` or is tagged `String.raw`.
 */
export function cookedEscapes(source: string): string[] {
  const found: string[] = [];
  const opening = /(String\.raw)?`\r?\n/g;
  let match: RegExpExecArray | null;
  while ((match = opening.exec(source)) !== null) {
    let at = match.index + match[0].length;
    let depth = 0;
    for (; at < source.length; at++) {
      const char = source[at];
      if (depth === 0 && char === "\\") {
        const next = source[at + 1];
        if (!match[1] && !["\\", "`", "$"].includes(next)) {
          found.push(`line ${source.slice(0, at).split("\n").length}: \\${next}`);
        }
        at++;
      } else if (depth === 0 && char === "`") break;
      else if (depth === 0 && char === "$" && source[at + 1] === "{") {
        depth = 1;
        at++;
      } else if (depth > 0 && char === "{") depth++;
      else if (depth > 0 && char === "}") depth--;
    }
    opening.lastIndex = at + 1;
  }
  return found;
}

/** No page script in the harness loses a backslash escape, and the scan finds the one that did. */
export function pageScriptEscapeProblem(): string | null {
  const corrupted = 'const SCRIPT = `\nreturn node.innerText.replace(/\\s+/g, " ");\n`;\n';
  if (cookedEscapes(corrupted).join() !== "line 2: \\s") return `cookedEscapes missed \\s in a plain template: ${JSON.stringify(cookedEscapes(corrupted))}`;
  const kept = 'const A = `\nx.replace(/\\\\s+/g, " ") ${n}\n`;\nconst B = String.raw`\n/\\s+/\n`;\n';
  if (cookedEscapes(kept).length > 0) return `cookedEscapes flagged kept escapes: ${cookedEscapes(kept).join(", ")}`;
  const harness = path.resolve(import.meta.dir, "..");
  const problems: string[] = [];
  for (const dir of ["gates", "lib"]) {
    for (const name of readdirSync(path.join(harness, dir)).filter((file) => file.endsWith(".ts"))) {
      for (const escape of cookedEscapes(readFileSync(path.join(harness, dir, name), "utf8"))) {
        problems.push(`harness/${dir}/${name} ${escape}`);
      }
    }
  }
  return problems.length > 0 ? `page script escape(s) a plain template literal cooks away: ${problems.join("; ")}` : null;
}

/** desktop-ux names a skipped or doubled state; a complete audit set has no problem. */
export function uxCoverageProblem(): string | null {
  const sizes = UX_SIZES.map((size) => `${size[0]}x${size[1]}`);
  const modes = ["light-zh", "light-en", "dark-zh", "dark-en"];
  const full: Audited[] = sizes.flatMap((size) => UX_STATES.flatMap((state) => modes.map((mode) => ({ size, state, mode }))));
  const clean = coverageProblems(full, sizes);
  if (clean.length > 0) return `desktop-ux coverage: a complete audit set reports ${clean.join("; ")}`;
  const skipped = coverageProblems(full.filter((entry) => !(entry.size === sizes[1] && entry.state === "hint-gate")), sizes);
  if (!skipped.some((line) => line.includes(`${sizes[1]} hint-gate was not audited`))) {
    return `desktop-ux coverage: a skipped hint gate is not reported (${skipped.join("; ") || "no problem"})`;
  }
  return null;
}

/**
 * The CI gate-group guard (harness/lib/gate-groups.ts) accepts the real workflow and refuses a workflow
 * or a grouping that would leave a gate out of some CI job: an `exclude`, a missing runner, a gate step
 * without its group, a matrix missing a group, a gate in no group and a gate in two.
 */
export function gateGroupProblem(gates: readonly Gate[], groups: Record<string, readonly Gate[]>, root: string): string | null {
  const real = path.join(root, ".github", "workflows", "gate.yml");
  const listed = workflowGroups(real);
  if (typeof listed === "string") return `gate groups: the real workflow is refused: ${listed}`;
  const clean = groupProblems(gates, groups, listed);
  if (clean.length > 0) return `gate groups: the real workflow reports ${clean.join("; ")}`;
  const text = readFileSync(real, "utf8");
  const broken: [string, string][] = [
    ["an exclude", text.replace(/\n(\s+)group: \[([^\]]*)\]\n/, "\n$1group: [$2]\n$1exclude: [{ os: windows-latest, group: desktop }]\n")],
    ["a missing runner", text.replace(", windows-latest]", "]")],
    ["a gate step without its group", text.replace('bun run gate --group "$GATE_GROUP"', "bun run gate")],
  ];
  const dir = mkdtempSync(path.join(tmpdir(), "passquest-gate-groups-"));
  try {
    for (const [name, workflow] of broken) {
      if (workflow === text) return `gate groups: the ${name} fixture did not change the workflow`;
      const file = path.join(dir, "gate.yml");
      writeFileSync(file, workflow);
      if (typeof workflowGroups(file) !== "string") return `gate groups: a workflow with ${name} is accepted`;
    }
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
  const [first, ...rest] = Object.keys(groups);
  if (!first || rest.length === 0) return `gate groups: expected several groups, got ${Object.keys(groups).join(", ")}`;
  const gate = groups[first]![0]!;
  const refused: [string, string[], string][] = [
    ["a matrix missing a group", groupProblems(gates, groups, listed.filter((name) => name !== first)), `group ${first} 0 times`],
    ["a gate in no group", groupProblems(gates, { ...groups, [first]: groups[first]!.filter((member) => member !== gate) }, listed), `gate ${gate.id} is in 0 groups`],
    ["a gate in two groups", groupProblems(gates, { ...groups, [rest[0]!]: [...groups[rest[0]!]!, gate] }, listed), `gate ${gate.id} is in 2 groups`],
  ];
  for (const [name, problems, expected] of refused) {
    if (!problems.some((line) => line.includes(expected))) return `gate groups: ${name} is not reported (${problems.join("; ") || "no problem"})`;
  }
  return null;
}

/** Product sources a smoke gate builds and drives from the repository root. */
const NOTE_SMOKE_SOURCES = ["go.mod", "go.sum", "cmd", "internal", "agent"];
/** desktop-ux also bundles the frontend and reads its allowlist. */
const DESKTOP_UX_SOURCES = [...NOTE_SMOKE_SOURCES, "desktop", "harness/ux-allow.json"];

function copyTree(from: string, to: string): number {
  let copied = 0;
  for (const entry of readdirSync(from, { withFileTypes: true })) {
    const source = path.join(from, entry.name);
    const target = path.join(to, entry.name);
    if (entry.isDirectory()) {
      mkdirSync(target, { recursive: true });
      copied += copyTree(source, target);
      continue;
    }
    if (!entry.isFile()) continue;
    writeFileSync(target, readFileSync(source));
    copied++;
  }
  return copied;
}

/** Copies working-tree product paths into the fixture root; returns the number of files. */
function copyProduct(root: string, relatives: string[]): number {
  const repo = path.resolve(import.meta.dir, "..", "..");
  let copied = 0;
  for (const relative of relatives) {
    const source = resolve(repo, relative);
    const target = resolve(root, relative);
    if (statSync(source).isDirectory()) {
      mkdirSync(target, { recursive: true });
      copied += copyTree(source, target);
      continue;
    }
    mkdirSync(path.dirname(target), { recursive: true });
    writeFileSync(target, readFileSync(source));
    copied++;
  }
  return copied;
}

/** Replaces the one occurrence of `anchor` in a copied product file. */
function patchCopy(root: string, relative: string, anchor: string, replacement: string): string | null {
  const file = resolve(root, relative);
  const source = readFileSync(file, "utf8");
  const count = source.split(anchor).length - 1;
  if (count !== 1) return `the anchor appears ${count} time(s) in ${relative}`;
  writeFileSync(file, source.replace(anchor, () => replacement));
  return null;
}

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
 * Copies the sources note-smoke builds into the fixture root and removes the
 * deck comparison from the push. A gate that keeps passing without the
 * comparison is not checking the deck claim, so it must fail here.
 */
async function removeTheDeckComparison(root: string): Promise<string | null> {
  if (copyProduct(root, NOTE_SMOKE_SOURCES) === 0) return "no product source was copied into the fixture root";
  const file = path.join(root, "internal", "cli", "note_anki.go");
  const source = readFileSync(file, "utf8");
  const anchor = "if deck != expected && !containsString(unexpected, deck) {";
  if (source.split(anchor).length !== 2) {
    return `the deck comparison anchor appears ${source.split(anchor).length - 1} time(s) in internal/cli/note_anki.go`;
  }
  writeFileSync(file, source.replace(anchor, "if false && !containsString(unexpected, deck) {"));
  return null;
}

async function gitAt(root: string, args: string[]): Promise<string | null> {
  const result = await run(executable("git"), args, { cwd: root });
  return result.code === 0 ? null : `git ${args.join(" ")} failed: ${result.stderr || result.stdout}`;
}

async function commitAll(root: string, message: string): Promise<string | null> {
  const add = await gitAt(root, ["add", "-A"]);
  if (add) return add;
  return gitAt(root, [
    "-c",
    "user.name=selftest",
    "-c",
    "user.email=selftest@example.invalid",
    "-c",
    "commit.gpgsign=false",
    "commit",
    "-q",
    "-m",
    message,
  ]);
}

/** A committed golden the fixtures then change or replace. */
const baseGoldenJson = '{\n  "schema_version": 1\n}\n';

/**
 * A repository with a `main` base commit and a `feature` branch, so the golden
 * gate's merge-base path is the one exercised. The fixture's first commit holds
 * the pre-change files; `change` then edits the working tree on `feature`.
 */
async function withFeatureBranch(root: string, change: (root: string) => Promise<string | null>): Promise<string | null> {
  const init = await gitAt(root, ["init", "-q", "-b", "main"]);
  if (init) return init;
  const base = await commitAll(root, "base");
  if (base) return base;
  const branch = await gitAt(root, ["checkout", "-q", "-b", "feature"]);
  if (branch) return branch;
  return change(root);
}

async function goldenOnlyChange(root: string): Promise<string | null> {
  writeFileSync(resolve(root, "internal/cli/testdata/next-empty.golden.json"), '{\n  "schema_version": 2\n}\n');
  return commitAll(root, "rewrite a golden without evidence");
}

async function newGoldenWithEvidence(root: string): Promise<string | null> {
  writeFileSync(resolve(root, "internal/cli/testdata/brand-new.golden.json"), "{}\n");
  const commit = await commitAll(root, "add a golden");
  if (commit) return commit;
  // The evidence row stays untracked: the gate must read the working tree, not
  // only committed history.
  mkdirSync(resolve(root, "docs/evidence"), { recursive: true });
  writeFileSync(resolve(root, "docs/evidence/0001-golden-contract.md"), "# Evidence\n\nNew golden declared.\n");
  return null;
}

function brokenCases(): BrokenCase[] {
  const fakeKey = `sk-${"A".repeat(24)}`;
  const completeBaseline = baselineFixture();
  const wrongCount = baselineFixture({ wrongCharsPage: 7 });
  const bomCount = baselineFixture({ bomPage: 7 });
  const pageGap = baselineFixture({ renumber: { from: EXPECTED_PAGES, to: EXPECTED_PAGES + 1 } });
  return [
    {
      gate: typecheck,
      label: "TypeScript type error",
      files: {
        "tsconfig.json": JSON.stringify({
          compilerOptions: {
            target: "ES2022",
            module: "ESNext",
            moduleResolution: "bundler",
            strict: true,
            noEmit: true,
            skipLibCheck: true,
            types: [],
          },
          include: ["bad.ts"],
        }),
        "bad.ts": 'const value: number = "text";\n',
      },
      tracked: ["bad.ts", "tsconfig.json"],
    },
    {
      gate: typecheck,
      label: "type error in the desktop frontend project",
      files: {
        "tsconfig.json": JSON.stringify({
          compilerOptions: { target: "ES2022", module: "ESNext", strict: true, noEmit: true, types: [] },
          include: ["ok.ts"],
        }),
        "ok.ts": "export const ok: number = 1;\n",
        "desktop/web/tsconfig.json": JSON.stringify({
          compilerOptions: {
            target: "ES2022",
            module: "ESNext",
            moduleResolution: "bundler",
            lib: ["ES2022", "DOM"],
            strict: true,
            noEmit: true,
            types: [],
          },
          include: ["src/**/*.ts"],
        }),
        "desktop/web/src/main.ts": "export const length: number = document.body.innerText;\n",
      },
      tracked: ["ok.ts", "tsconfig.json", "desktop/web/tsconfig.json", "desktop/web/src/main.ts"],
      postFail: (failures) => {
        if (!failures.includes("desktop/web/tsconfig.json:")) return "typecheck: did not name the frontend project";
        if (failures.includes("tsconfig.json:")) return "typecheck: blamed the valid root project";
        return null;
      },
    },
    {
      gate: typecheck,
      label: "desktop frontend without its own project",
      files: { "desktop/web/src/main.ts": "export const ok = 1;\n" },
      tracked: ["desktop/web/src/main.ts"],
      postFail: (failures) =>
        failures.some((line) => line.includes("desktop/web/tsconfig.json is missing"))
          ? null
          : "typecheck: did not report the missing frontend project",
    },
    {
      gate: gofmt,
      label: "unformatted Go file",
      files: { "bad.go": "package bad\n\nfunc  F( ) { }\n" },
      tracked: ["bad.go"],
    },
    {
      gate: docsLinks,
      label: "broken relative link",
      files: { "README.md": "[missing](does-not-exist.md)\n" },
      tracked: ["README.md"],
    },
    {
      gate: secrets,
      label: "credential-shaped string",
      files: { "leak.txt": `const key = "${fakeKey}";\n` },
      tracked: ["leak.txt"],
      postFail: (failures) =>
        failures.some((line) => line.includes(fakeKey)) ? "secrets: failure output echoed the credential value" : null,
    },
    {
      gate: secrets,
      label: "credential in a large tracked text file",
      files: { "large.md": `${"x".repeat(5 * 1024 * 1024 + 1)}\nconst key = "${fakeKey}";\n` },
      tracked: ["large.md"],
      postFail: (failures) => {
        if (!failures.some((line) => line.includes("large.md") && line.includes("OpenAI-style key"))) {
          return "secrets: did not scan the large text file";
        }
        if (failures.some((line) => line.includes(fakeKey))) {
          return "secrets: failure output echoed the credential value";
        }
        return null;
      },
    },
    {
      gate: localOnly,
      label: "first-party net/http import",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("ok") }\n',
        "internal/x/x.go": 'package x\n\nimport "net/http"\n\nvar _ = http.MethodGet\n',
      },
      tracked: [],
      postFail: (failures) =>
        failures.some((line) => line.includes("first-party network import"))
          ? null
          : "local-only: did not report the first-party network import",
    },
    {
      gate: localOnly,
      label: "network client linked through a module dependency",
      files: {
        "go.mod": 'module example.com/selftest\n\ngo 1.27.3\n\nrequire example.com/dep v0.0.0\n\nreplace example.com/dep => ./dep\n',
        "dep/go.mod": "module example.com/dep\n\ngo 1.27.3\n",
        "dep/client.go": 'package dep\n\nimport "net/http"\n\nfunc Client() *http.Client { return &http.Client{} }\n',
        "cmd/passquest/main.go": 'package main\n\nimport "example.com/dep"\n\nfunc main() { _ = dep.Client() }\n',
        "internal/x/x.go": "package x\n",
      },
      tracked: [],
      postFail: (failures) => {
        if (!failures.some((line) => line.includes("network client linked into the CLI: net/http"))) {
          return "local-only: did not report the linked network client";
        }
        if (failures.some((line) => line.includes("first-party network import"))) {
          return "local-only: first-party scan also matched the dependency import";
        }
        return null;
      },
    },
    {
      gate: localOnly,
      label: "confidential marker in a scanned non-Go file",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/x/x.go": "package x\n",
        "internal/x/notes.md": "# notes\n\n仅限内部使用\n",
      },
      tracked: ["internal/x/notes.md"],
      postFail: (failures) =>
        failures.length === 1 && failures[0].includes("confidential source marker")
          ? null
          : `local-only: expected one confidential marker failure, saw ${failures.length}`,
    },
    {
      gate: localOnly,
      label: "desktop main that imports net/http itself",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/x/x.go": "package x\n",
        "cmd/passquest-desktop/main.go": 'package main\n\nimport "net/http"\n\nfunc main() { _ = http.MethodGet }\n',
      },
      tracked: [],
      postFail: (failures) => {
        if (!failures.some((line) => line.includes("first-party network import") && line.includes("cmd/passquest-desktop"))) {
          return "local-only: did not report the desktop main's network import";
        }
        if (failures.some((line) => line.includes("network client linked into the CLI"))) {
          return "local-only: blamed the CLI for the desktop main's import";
        }
        return null;
      },
    },
    {
      gate: localOnly,
      label: "confidential marker in desktop frontend source",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/x/x.go": "package x\n",
        "desktop/web/src/main.ts": 'export const banner = "严禁外传";\n',
      },
      tracked: ["desktop/web/src/main.ts"],
      postFail: (failures) =>
        failures.length === 1 && failures[0].startsWith("desktop/web/src/main.ts:1: confidential source marker")
          ? null
          : `local-only: expected one marker failure in desktop/web/src/main.ts, saw ${failures.join("; ")}`,
    },
    {
      gate: portablePaths,
      label: "an unjustified literal beside a justified one",
      files: {
        "go.mod": GO_MOD,
        "internal/x/x.go": "package x\n",
        "internal/x/x_test.go":
          'package x\n\nimport "testing"\n\nfunc TestA(t *testing.T) {\n\topen("/tmp/fixture.sqlite")\n\tif "C:\\\\Users\\\\x" != "C:\\\\Users\\\\x" { // portable-paths: allow compared as string data, never resolved\n\t\tt.Fatal("unreachable")\n\t}\n\topen("/tmp/adjacent.sqlite")\n}\n',
      },
      tracked: ["internal/x/x_test.go"],
      postFail: (failures) => {
        if (failures.length !== 2) {
          return `portable-paths: expected both unjustified literals, saw ${failures.length}: ${failures.join("; ")}`;
        }
        if (!failures.some((line) => line.includes("x_test.go:6") && line.includes("/tmp/fixture.sqlite"))) {
          return `portable-paths: did not name the first unjustified literal: ${failures.join("; ")}`;
        }
        if (!failures.some((line) => line.includes("/tmp/adjacent.sqlite"))) {
          return "portable-paths: the marker on the previous line silenced an adjacent unjustified path";
        }
        if (failures.some((line) => line.includes("C:\\\\Users"))) {
          return "portable-paths: reported the literal its own marker justified";
        }
        return null;
      },
    },
    {
      gate: portablePaths,
      label: "pointer assignment and a block comment",
      files: {
        "go.mod": GO_MOD,
        "internal/x/x.go": "package x\n",
        "internal/x/x_test.go":
          'package x\n\nimport "testing"\n\nfunc TestPointer(t *testing.T) {\n\tvar p *string\n\t*p = "/tmp/pointer.sqlite"\n\t_ = p\n}\n',
        "internal/x/doc_test.go":
          'package x\n\n/*\nA fixture usually looks like "/tmp/documentation.sqlite".\n*/\nfunc TestDoc(t *testing.T) {}\n',
      },
      tracked: ["internal/x/x_test.go", "internal/x/doc_test.go"],
      postFail: (failures) => {
        if (failures.length !== 1) {
          return `portable-paths: expected only the pointer assignment, saw ${failures.length}: ${failures.join("; ")}`;
        }
        if (!failures[0].includes("x_test.go:7") || !failures[0].includes("/tmp/pointer.sqlite")) {
          return `portable-paths: did not report the pointer assignment: ${failures[0]}`;
        }
        if (failures.some((line) => line.includes("documentation.sqlite"))) {
          return "portable-paths: treated a path inside a block comment as code";
        }
        return null;
      },
    },
    {
      gate: portablePaths,
      label: "comment syntax inside a multiline raw string",
      files: {
        "go.mod": GO_MOD,
        "internal/x/x.go": "package x\n",
        "internal/x/x_test.go":
          'package x\n\nimport "testing"\n\nconst example = `\nselect /*\nfrom t\n`\n\nfunc TestAfterRaw(t *testing.T) {\n\topen("/tmp/after-raw.sqlite")\n}\n',
      },
      tracked: ["internal/x/x_test.go"],
      postFail: (failures) => {
        if (failures.length !== 1 || !failures[0].includes("/tmp/after-raw.sqlite")) {
          return `portable-paths: a raw string opened a block comment and hid the next path: ${failures.join("; ") || "no failure"}`;
        }
        return null;
      },
    },
    {
      gate: portablePaths,
      label: "a backtick rune before an unmarked path",
      files: {
        "go.mod": GO_MOD,
        "internal/x/x.go": "package x\n",
        "internal/x/x_test.go":
          "package x\n\nimport \"testing\"\n\nconst delimiter = '`'\nconst quote = '\"'\n\nfunc TestRune(t *testing.T) {\n\topen(\"/tmp/after-rune.sqlite\")\n\t_, _ = delimiter, quote\n}\n",
      },
      tracked: ["internal/x/x_test.go"],
      postFail: (failures) => {
        if (failures.length !== 1 || !failures[0].includes("/tmp/after-rune.sqlite")) {
          return `portable-paths: a rune holding a quote hid the next path: ${failures.join("; ") || "no failure"}`;
        }
        return null;
      },
    },
    {
      gate: portablePaths,
      label: "raw-string path literals",
      files: {
        "go.mod": GO_MOD,
        "internal/x/x.go": "package x\n",
        "internal/x/x_test.go":
          "package x\n\nimport \"testing\"\n\nfunc TestRaw(t *testing.T) {\n\tposix := `/tmp/raw.sqlite`\n\twin := `C:\\Users\\runner\\agent.sqlite`\n\t_ = posix\n\t_ = win\n}\n",
      },
      tracked: ["internal/x/x_test.go"],
      postFail: (failures) => {
        if (failures.length !== 2) {
          return `portable-paths: expected both raw-string literals, saw ${failures.length}: ${failures.join("; ")}`;
        }
        if (!failures.some((line) => line.includes("/tmp/raw.sqlite"))) {
          return "portable-paths: did not report the raw-string POSIX path";
        }
        if (!failures.some((line) => line.includes("C:\\\\Users\\\\runner\\\\agent.sqlite"))) {
          return "portable-paths: did not report the raw-string Windows path";
        }
        return null;
      },
    },
    {
      gate: portablePaths,
      label: "interpreted Windows paths that start with t, n or r",
      files: {
        "go.mod": GO_MOD,
        "internal/x/x.go": "package x\n",
        "internal/x/x_test.go":
          'package x\n\nimport "testing"\n\nfunc TestEscapes(t *testing.T) {\n\t_t := "C:\\\\tmp\\\\fixture.sqlite"\n\t_n := "C:\\\\new\\\\fixture.sqlite"\n\t_r := "C:\\\\repo\\\\fixture.sqlite"\n\t_, _, _ = _t, _n, _r\n}\n',
      },
      tracked: ["internal/x/x_test.go"],
      postFail: (failures) => {
        if (failures.length !== 3) {
          return `portable-paths: expected all three interpreted paths, saw ${failures.length}: ${failures.join("; ")}`;
        }
        for (const segment of ["\\tmp\\", "\\new\\", "\\repo\\"]) {
          if (!failures.some((line) => line.includes(segment))) {
            return `portable-paths: did not decode ${segment} in an interpreted literal`;
          }
        }
        return null;
      },
    },
    {
      gate: baseline,
      label: "baseline manifest disagrees with its page",
      files: wrongCount.files,
      tracked: wrongCount.tracked,
      postFail: (failures) =>
        failures.length === 1 && failures[0].includes("page-007.txt") && failures[0].includes("non-space chars")
          ? null
          : `baseline: expected one wrong char count on page-007.txt, saw ${failures.length}: ${failures.join("; ")}`,
    },
    {
      gate: baseline,
      label: "baseline count treats U+FEFF as whitespace",
      files: bomCount.files,
      tracked: bomCount.tracked,
      postFail: (failures) =>
        failures.length === 1 && failures[0].includes("page-007.txt") && failures[0].includes("non-space chars")
          ? null
          : `baseline: expected one wrong char count on page-007.txt, saw ${failures.length}: ${failures.join("; ")}`,
    },
    {
      gate: baseline,
      label: "baseline page set has a gap",
      files: pageGap.files,
      tracked: pageGap.tracked,
      postFail: (failures) =>
        failures.some((line) => line.includes(`manifest page ${EXPECTED_PAGES} is missing`))
          ? null
          : `baseline: expected a missing manifest page, saw: ${failures.join("; ")}`,
    },
    {
      gate: baseline,
      label: "test fixture references the baseline",
      files: {
        ...completeBaseline.files,
        "internal/fixture_test.go": 'package internal\n\nconst baselinePath = "data/baselines/2026-crsc-english"\n',
      },
      tracked: [...completeBaseline.tracked, "internal/fixture_test.go"],
      postFail: (failures) =>
        failures.length === 1 &&
        failures[0].includes("internal/fixture_test.go") &&
        failures[0].includes("references the unreviewed baseline")
          ? null
          : `baseline: expected one fixture-reference failure, saw ${failures.length}: ${failures.join("; ")}`,
    },
    {
      gate: goTest,
      label: "failing Go test",
      files: {
        "go.mod": GO_MOD,
        "x_test.go": 'package x\n\nimport "testing"\n\nfunc TestFail(t *testing.T) { t.Fatal("boom") }\n',
      },
      postFail: (failures) => {
        if (!failures.some((line) => line.includes("TestFail: failed"))) {
          return `go-test: did not report the failing test, saw: ${failures.join("; ")}`;
        }
        if (!failures.some((line) => line.includes("boom"))) {
          return "go-test: dropped the test's failure message";
        }
        return null;
      },
    },
    {
      gate: goTest,
      label: "skipped Go test",
      files: {
        "go.mod": GO_MOD,
        "x_test.go": 'package x\n\nimport "testing"\n\nfunc TestSkipped(t *testing.T) { t.Skip("deliberately skipped") }\n',
      },
      postFail: (failures) =>
        failures.some((line) => line.includes("TestSkipped")) ? null : `go-test: did not report the skipped test, saw: ${failures.join("; ")}`,
    },
    {
      gate: goVet,
      label: "vet printf diagnostic",
      files: {
        "go.mod": GO_MOD,
        "main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Printf("%d", "x") }\n',
      },
    },
    {
      gate: goBuild,
      label: "compile error",
      files: {
        "go.mod": GO_MOD,
        "main.go": "package main\n\nfunc main() { missing() }\n",
      },
    },
    {
      gate: goTidy,
      label: "stale module requirement",
      files: {
        "go.mod": 'module example.com/selftest\n\ngo 1.27.3\n\nrequire example.com/dep v0.0.0\n\nreplace example.com/dep => ./dep\n',
        "dep/go.mod": "module example.com/dep\n\ngo 1.27.3\n",
        "main.go": "package main\n\nfunc main() {}\n",
      },
    },
    {
      gate: importSmoke,
      label: "CLI whose import output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
        "internal/cli/testdata/import/placeholder.txt": "fixture placeholder\n",
      },
    },
    {
      gate: tacticSmoke,
      label: "CLI whose tactic output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
      },
    },
    {
      gate: adaptiveSmoke,
      label: "CLI whose skill and next output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
      },
    },
    {
      gate: tutorSmoke,
      label: "CLI whose tutor output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
      },
    },
    {
      gate: agentSmoke,
      label: "CLI whose agent output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
      },
    },
    {
      gate: noteSmoke,
      label: "CLI whose note output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
        "internal/cli/testdata/import/question-q1.json": "{}",
      },
    },
    {
      gate: noteSmoke,
      label: "note push that reads the deck back and never compares it",
      files: {},
      prepare: removeTheDeckComparison,
      postFail: (failures) => {
        if (!failures.some((line) => line.includes("mismatch"))) {
          return `note-smoke: removing the deck comparison gave ${failures.length} failure(s) that never name the mismatch: ${failures.slice(0, 3).join("; ")}`;
        }
        if (failures.some((line) => line.includes("go build") || line.includes("import exit = ") || line.includes("first push exit"))) {
          return `note-smoke: the mutated tree broke an earlier stage: ${failures.slice(0, 3).join("; ")}`;
        }
        return null;
      },
    },
    {
      gate: findSmoke,
      label: "CLI whose find output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
      },
    },
    {
      gate: assessSmoke,
      label: "CLI whose assess output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
      },
    },
    {
      gate: providerSmoke,
      label: "CLI whose model output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
      },
    },
    {
      gate: goldenContract,
      label: "golden rewritten on a branch without a contract document",
      files: { "internal/cli/testdata/next-empty.golden.json": baseGoldenJson },
      prepare: (root) => withFeatureBranch(root, goldenOnlyChange),
      postFail: (failures) => {
        if (failures.length !== 1) {
          return `golden-contract: expected one failure, saw ${failures.length}: ${failures.join("; ")}`;
        }
        if (!failures[0].includes("internal/cli/testdata/next-empty.golden.json")) {
          return "golden-contract: did not name the golden that changed";
        }
        if (!failures[0].includes("docs/evidence/") || !failures[0].includes("OPEN-QUESTIONS.md")) {
          return "golden-contract: did not name the contract document that is missing";
        }
        return null;
      },
    },
    {
      gate: desktopSmoke,
      label: "desktop main that exits 0 without writing a result",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest-desktop/main.go": "package main\n\nfunc main() {}\n",
        "desktop/web/src/main.ts": 'document.title = "selftest";\n',
      },
      postFail: (failures) =>
        failures.some((line) => line.startsWith("boot: app exited 0 without writing a result"))
          ? null
          : `desktop-smoke: did not report the missing result: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      gate: desktopSmoke,
      label: "desktop window whose Today stays blank",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest-desktop/main.go": `package main\n\nimport "os"\n\nfunc main() {\n\tblank := \`{"ok":true,"value":{"textLength":0,"stageChildren":0,"stageTextLength":0,"errorBars":0,"ready":null}}\`\n\t_ = os.WriteFile(os.Getenv("PASSQUEST_DESKTOP_RESULT"), []byte(blank), 0o600)\n}\n`,
        "desktop/web/src/main.ts": 'document.title = "selftest";\n',
      },
      postFail: (failures) =>
        failures.some((line) => line.startsWith("boot: blank Today"))
          ? null
          : `desktop-smoke: did not report the blank window: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      gate: desktopSmoke,
      label: "desktop binary whose generated client differs from the committed one",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest-desktop/main.go": `package main

import "os"

func main() {
	if out := os.Getenv("MYGO_GENERATE"); out != "" {
		_ = os.WriteFile(out, []byte("export const client = 2;\\n"), 0o600)
	}
}
`,
        "desktop/web/src/main.ts": 'document.title = "selftest";\n',
        "desktop/web/src/api/mygo.ts": "export const client = 1;\r\n",
      },
      postFail: (failures) =>
        failures.some((line) => line.startsWith("client-fresh: desktop/web/src/api/mygo.ts is stale"))
          ? null
          : `desktop-smoke: did not report the stale client: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      gate: desktopSmoke,
      label: "desktop practice flow that writes no attempt",
      files: {
        "go.mod": GO_MOD,
        // Boots and claims a keyboard answer, but the second process finds no attempt.
        "cmd/passquest-desktop/main.go": `package main

import (
	"encoding/json"
	"os"
	"strings"
)

func write(value any) {
	document, _ := json.Marshal(map[string]any{"ok": true, "value": value})
	_ = os.WriteFile(os.Getenv("PASSQUEST_DESKTOP_RESULT"), document, 0o600)
}

func main() {
	script, _ := os.ReadFile(os.Getenv("PASSQUEST_DESKTOP_SCRIPT"))
	switch {
	case strings.Contains(string(script), "desktop-smoke: boot/"):
		write(map[string]any{"textLength": 900, "stageChildren": 3, "stageTextLength": 800, "errorBars": 0, "ready": true})
	case strings.Contains(string(script), "desktop-smoke: practice/"):
		write(map[string]any{"answers": []any{map[string]any{
			"item":      map[string]string{"token": "pt-selftest", "question": "q1", "presentedAt": "2026-01-01T00:00:00Z"},
			"condition": map[string]string{"condition": "independent", "canCount": "true"},
			"chosen":    "B",
			"feedback":  map[string]string{"attempt": "1", "verdict": "correct", "elapsedMs": "1200"},
		}}})
	}
}
`,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println(`{"attempts":[]}`) }\n',
        "desktop/web/src/main.ts": 'document.title = "selftest";\n',
      },
      postFail: (failures) =>
        failures.some((line) => line.startsWith("practice: report --json has no attempt 1 "))
          ? null
          : `desktop-smoke: did not report the missing attempt: ${failures.slice(0, 3).join("; ")}`,
    },
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

/** Fixtures the gates must accept; a check that always fails is also a defect. */
function passCases(): PassCase[] {
  return [
    {
      gate: goldenContract,
      label: "new golden declared by an untracked evidence record",
      files: { "internal/cli/testdata/next-empty.golden.json": baseGoldenJson },
      prepare: (root) => withFeatureBranch(root, newGoldenWithEvidence),
      postPass: (detail) =>
        detail.includes("1 golden(s) declared by 1 contract document(s)")
          ? null
          : `golden-contract: accepted detail ${JSON.stringify(detail)} does not name the declaring evidence row`,
    },
    {
      gate: localOnly,
      label: "desktop main that links net/http only through a dependency",
      files: {
        "go.mod": 'module example.com/selftest\n\ngo 1.27.3\n\nrequire example.com/dep v0.0.0\n\nreplace example.com/dep => ./dep\n',
        "dep/go.mod": "module example.com/dep\n\ngo 1.27.3\n",
        "dep/client.go": 'package dep\n\nimport "net/http"\n\nfunc Client() *http.Client { return &http.Client{} }\n',
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "cmd/passquest-desktop/main.go": 'package main\n\nimport "example.com/dep"\n\nfunc main() { _ = dep.Client() }\n',
        "internal/x/x.go": "package x\n",
      },
      tracked: [],
      postPass: (detail) =>
        detail.startsWith("3 first-party packages")
          ? null
          : `local-only: accepted detail ${JSON.stringify(detail)} does not count the desktop main`,
    },
  ];
}

let fixtures = 0;

/**
 * A new fixture root, never the path of an earlier one in this process: gates cache what they build per
 * root (buildCLI), so a root that reused a removed fixture's path would run that fixture's binary.
 */
function fixtureRoot(): string {
  return mkdtempSync(path.join(tmpdir(), `passquest-selftest-${process.pid}-${++fixtures}-`));
}

async function prepareFixture(
  root: string,
  testCase: { files: Record<string, string>; prepare?: (root: string) => Promise<string | null> },
): Promise<string | null> {
  for (const [file, content] of Object.entries(testCase.files)) {
    const absolute = resolve(root, file);
    mkdirSync(path.dirname(absolute), { recursive: true });
    writeFileSync(absolute, content);
  }
  return testCase.prepare ? testCase.prepare(root) : null;
}

async function runPassCase(testCase: PassCase): Promise<string | null> {
  const root = fixtureRoot();
  try {
    const preparation = await prepareFixture(root, testCase);
    if (preparation) return `${testCase.gate.id}: fixture on ${testCase.label}: ${preparation}`;
    const result = await testCase.gate.run(context(root, testCase.tracked));
    if (result.status !== "pass") {
      return `${testCase.gate.id}: status ${result.status} on ${testCase.label}, want pass: ${(result.failures ?? []).join("; ")}`;
    }
    return testCase.postPass?.(result.detail ?? "") ?? null;
  } catch (error) {
    return `${testCase.gate.id}: threw on ${testCase.label}: ${error instanceof Error ? error.message : String(error)}`;
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

async function runCase(testCase: BrokenCase): Promise<string | null> {
  const root = fixtureRoot();
  try {
    const preparation = await prepareFixture(root, testCase);
    if (preparation) return `${testCase.gate.id}: fixture on ${testCase.label}: ${preparation}`;
    const ctx: GateContext = context(root, testCase.tracked);
    const result = await testCase.gate.run(ctx);
    if (result.status === "fail") {
      const problem = testCase.postFail?.(result.failures ?? []) ?? null;
      return problem ? `${testCase.gate.id}: ${problem} on ${testCase.label}` : null;
    }
    const reason = result.status === "skip" ? `skip (${result.skipReason ?? "no reason"})` : `status ${result.status}`;
    return `${testCase.gate.id}: ${reason} on ${testCase.label}`;
  } catch (error) {
    return `${testCase.gate.id}: threw on ${testCase.label}: ${error instanceof Error ? error.message : String(error)}`;
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

export const selfTest: Gate = {
  id: "harness-self-test",
  title: "harness self-test",
  async run(ctx) {
    // Static import cannot work: index.ts builds `gates` from `selfTest` at module
    // evaluation, so the cycle would read `selfTest` before initialization when
    // this module is the entry point. The list is only needed here, at run time.
    const { gates, gateGroups } = await import("./index.ts");
    const cases = brokenCases();
    const accepted = passCases();
    const problems: string[] = [];
    const discovery = await workingTreeDiscoveryProblem();
    if (discovery) problems.push(discovery);
    for (const unit of [processTreeProblem, pageScriptEscapeProblem, uxCoverageProblem, () => gateGroupProblem(gates, gateGroups, ctx.root)]) {
      const problem = unit();
      if (problem) problems.push(problem);
    }
    for (const testCase of cases) {
      const problem = await runCase(testCase);
      if (problem) problems.push(problem);
    }
    for (const testCase of accepted) {
      const problem = await runPassCase(testCase);
      if (problem) problems.push(problem);
    }
    const covered = new Set([...cases, ...accepted].map((testCase) => testCase.gate.id));
    const expected = gates.filter((gate) => gate.id !== selfTest.id);
    for (const gate of expected) {
      if (!covered.has(gate.id)) problems.push(`${gate.id}: no self-test case`);
    }
    if (problems.length > 0) return fail(problems);
    return pass(`${cases.length + accepted.length} cases (${accepted.length} must pass), ${covered.size}/${expected.length} gates`);
  },
};
