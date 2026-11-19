import { mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { descendants, parsePsLine, parsePsTable, sameProcess } from "../lib/desktop.ts";
import type { ProcessInfo } from "../lib/desktop.ts";
import { groupProblems, workflowGroups } from "../lib/gate-groups.ts";
import { executable, run } from "../lib/process.ts";
import { context, resolve, workingTreeFiles } from "../lib/repo.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";

import { coverageProblems, UX_SIZES, UX_STATES } from "./desktop-ux.ts";
import type { Audited } from "./desktop-ux.ts";
import { adaptiveSmokeCases } from "./self-test/adaptive-smoke.cases.ts";
import { agentSmokeCases } from "./self-test/agent-smoke.cases.ts";
import { assessSmokeCases } from "./self-test/assess-smoke.cases.ts";
import { baselineCases } from "./self-test/baseline.cases.ts";
import { desktopSmokeCases } from "./self-test/desktop-smoke.cases.ts";
import { desktopUxCases } from "./self-test/desktop-ux.cases.ts";
import { docsLinksCases } from "./self-test/docs-links.cases.ts";
import { findSmokeCases } from "./self-test/find-smoke.cases.ts";
import type { BrokenCase, PassCase } from "./self-test/fixture.ts";
import { goBuildCases } from "./self-test/go-build.cases.ts";
import { goTestCases } from "./self-test/go-test.cases.ts";
import { goTidyCases } from "./self-test/go-tidy.cases.ts";
import { goVetCases } from "./self-test/go-vet.cases.ts";
import { gofmtCases } from "./self-test/gofmt.cases.ts";
import { goldenContractCases, goldenContractPassCases } from "./self-test/golden-contract.cases.ts";
import { importSmokeCases } from "./self-test/import-smoke.cases.ts";
import { localOnlyCases, localOnlyPassCases } from "./self-test/local-only.cases.ts";
import { noteSmokeCases } from "./self-test/note-smoke.cases.ts";
import { packageNamingProblem, packageSmokeCases, packageSmokePassCases } from "./self-test/package-smoke.cases.ts";
import { portablePathsCases } from "./self-test/portable-paths.cases.ts";
import { providerSmokeCases } from "./self-test/provider-smoke.cases.ts";
import { secretsCases } from "./self-test/secrets.cases.ts";
import { tacticSmokeCases } from "./self-test/tactic-smoke.cases.ts";
import { tutorSmokeCases } from "./self-test/tutor-smoke.cases.ts";
import { typecheckCases } from "./self-test/typecheck.cases.ts";

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
    // Recursive: the desktop-smoke scenarios and the self-test cases live in sub-directories of gates/.
    for (const name of readdirSync(path.join(harness, dir), { recursive: true, encoding: "utf8" }).filter((file) => file.endsWith(".ts"))) {
      for (const escape of cookedEscapes(readFileSync(path.join(harness, dir, name), "utf8"))) {
        problems.push(`harness/${dir}/${name.split(path.sep).join("/")} ${escape}`);
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

/** Every gate's deliberately broken inputs, in a fixed order; each gate's cases live in self-test/<gate>.cases.ts. */
function brokenCases(): BrokenCase[] {
  return [
    ...typecheckCases(),
    ...gofmtCases(),
    ...docsLinksCases(),
    ...secretsCases(),
    ...localOnlyCases(),
    ...portablePathsCases(),
    ...baselineCases(),
    ...goTestCases(),
    ...goVetCases(),
    ...goBuildCases(),
    ...goTidyCases(),
    ...importSmokeCases(),
    ...tacticSmokeCases(),
    ...adaptiveSmokeCases(),
    ...tutorSmokeCases(),
    ...agentSmokeCases(),
    ...noteSmokeCases(),
    ...findSmokeCases(),
    ...assessSmokeCases(),
    ...providerSmokeCases(),
    ...goldenContractCases(),
    ...desktopSmokeCases(),
    ...desktopUxCases(),
    ...packageSmokeCases(),
  ];
}

/** Fixtures the gates must accept; a check that always fails is also a defect. */
function passCases(): PassCase[] {
  return [...goldenContractPassCases(), ...localOnlyPassCases(), ...packageSmokePassCases()];
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
    for (const unit of [
      processTreeProblem,
      pageScriptEscapeProblem,
      uxCoverageProblem,
      packageNamingProblem,
      () => gateGroupProblem(gates, gateGroups, ctx.root),
    ]) {
      const problem = unit();
      if (problem) problems.push(problem);
    }
    const timings: { label: string; ms: number }[] = [];
    const timed = async (label: string, check: () => Promise<string | null>) => {
      const started = performance.now();
      const problem = await check();
      timings.push({ label, ms: performance.now() - started });
      if (problem) problems.push(problem);
    };
    for (const testCase of cases) await timed(testCase.label, () => runCase(testCase));
    for (const testCase of accepted) await timed(testCase.label, () => runPassCase(testCase));
    const covered = new Set([...cases, ...accepted].map((testCase) => testCase.gate.id));
    const expected = gates.filter((gate) => gate.id !== selfTest.id);
    for (const gate of expected) {
      if (!covered.has(gate.id)) problems.push(`${gate.id}: no self-test case`);
    }
    if (problems.length > 0) return fail(problems);
    const slowest = [...timings]
      .sort((a, b) => b.ms - a.ms)
      .slice(0, 3)
      .map((timing) => `${timing.label} ${(timing.ms / 1000).toFixed(1)}s`);
    return pass(`${cases.length + accepted.length} cases (${accepted.length} must pass), ${covered.size}/${expected.length} gates; slowest cases: ${slowest.join("; ")}`);
  },
};
