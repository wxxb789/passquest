#!/usr/bin/env bun
import { randomBytes } from "node:crypto";
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, statSync, writeFileSync } from "node:fs";
import { release as osRelease, tmpdir } from "node:os";
import path from "node:path";

import {
  BUNDLE_METRICS,
  CONTENT_SIZE,
  compareShots,
  GUARD_RULE,
  guardProblems,
  launchPlan,
  loafTotals,
  metricRows,
  parseBenchArgs,
  PREPARE_GUARDS_WHY,
  prepareScript,
  READY_METRIC,
  readLaunch,
  readLoaf,
  readShotLaunch,
  readVisualAllowlist,
  recordShapeProblems,
  selfCheckCases,
  SHOT_STATES,
  shotScript,
  shotStateOf,
  smokeLaunchPlan,
  summariseFrames,
  templatePlan,
  timingScript,
  USAGE,
  VERDICT_RULE,
  VISUAL_ALLOWLIST,
  visualFindings,
  viewportProblems,
  pngSize,
} from "../lib/bench.ts";
import type {
  BenchOptions,
  BuiltFile,
  GuardReading,
  LaunchPlan,
  LaunchReading,
  LoafTotals,
  MetricSamples,
  ShotRecord,
  ShotRow,
  Spread,
  ViewportReading,
} from "../lib/bench.ts";
import { buildDesktop, FRONTEND_DIST, removeTree } from "../lib/desktop.ts";
import { chatReply, cliJson, launchPage, STUB_CREDENTIAL_ENV, STUB_MODEL, startProviderStub, writeStubRegistry } from "../lib/desktop-page.ts";
import type { Check, Launched, ProviderStub } from "../lib/desktop-page.ts";
import { buildCLI } from "../lib/go.ts";
import { sha256Hex } from "../lib/hash.ts";
import { field, stringField } from "../lib/json.ts";
import { executable, run, runMain, tail } from "../lib/process.ts";
import { context, resolve } from "../lib/repo.ts";
import { seedReviewedSource } from "../lib/reviewed-source.ts";

const OWN_TREE = path.resolve(import.meta.dir, "..", "..");
const OUTPUT_TAIL_LINES = 60;
/** A visible window's WebView2 helpers can take longer than a gate's 10 s to exit on a slow machine. */
const ORPHAN_GRACE_MS = 30_000;
/** Nothing listens on port 1: the app's Anki probe reports "not running" at once instead of waiting on a socket. */
const UNREACHABLE_ANKI = "http://127.0.0.1:1";
const COACH_REPLY = "Check the notice: which option states the Saturday closing time?";
/** What each metric row measures; the record carries this so a number is never read without it. */
const METRIC_NOTES: Record<string, string> = {
  readyMs: "the tool's launch call to the page observing window.__pq.ready (a 50 ms poll: its granularity is part of every sample)",
  answerSelectMs: "the option keypress to .opt[aria-checked=true]",
  submitFeedbackMs: "the Enter keypress on a chosen option to [data-testid=feedback]",
  coachReplyMs: "the coach send click to [data-testid=coach-reply] or [data-testid=coach-failure]",
  "roomSwitch.report": "location.hash = #/report to [data-testid=report] with no loading card",
  "roomSwitch.anki": "location.hash = #/anki to [data-testid=note] with no loading card",
  "roomSwitch.settings": "location.hash = #/settings to a provider row with no loading card",
  "roomSwitch.assess": "location.hash = #/assess to [data-act=run-ask] with no loading card",
  "frames.median": "per launch: the median frame duration of the sheet-open + scroll sample",
  "frames.p95": "per launch: the 95th percentile of that sample",
  "frames.longest": "per launch: the longest frame of that sample",
  "bundle.mainJs": "bytes of internal/desktop/web/dist/main.js in that tree's own build",
  "bundle.mainCss": "bytes of internal/desktop/web/dist/main.css in that tree's own build",
  "loaf.entries": "per launch: long-animation-frame entries the page observed (a Chromium-only enrichment: WebView2 reports them, WKWebView and WebKitGTK have no such entry type, and every launch row of such an engine carries loaf: null)",
  "loaf.totalMs": "per launch: the sum of those entries' durations",
  "loaf.maxMs": "per launch: the longest of those entries",
};

/**
 * Twenty-eight pages, four questions on page 1: the unseen pool Q-0009-03 wants (20 qualifying
 * items) has room for the prepare's attempts, its three run items and the later launches' practice.
 */
const SEED_PAGES = 28;
const SEED_FIRST_PAGE_QUESTIONS = 4;
/** Smaller than this a PNG cannot hold a rendered 1280x800 page. */
const MIN_SHOT_BYTES = 4096;

interface Tree {
  key: "a" | "b";
  /** The root as the caller gave it; every path in the record is written this way. */
  root: string;
  workdir: string;
  binary: string;
  display: string;
  commit: string;
  hook: boolean;
  bundles: BuiltFile[];
  template: string;
}

type LaunchKind = "prepare" | "warmup" | "sample" | "control" | "shot" | "control-shot";

/** One row per launch: its label, its tree, the window it requested (hidden or shown), the engine UA and the guard reading it produced (or why it carries none), its LoAF totals, its shot outcome and why it failed. */
interface LaunchRow {
  label: string;
  tree: "a" | "b";
  kind: LaunchKind;
  hidden: boolean;
  engine: string;
  guards: GuardReading | null;
  /** Why this launch carries no guard reading; "" when it carries one (PREPARE_GUARDS_WHY). */
  guardsWhy: string;
  loaf: LoafTotals | null;
  shots: string;
  error: string;
  result: "ok" | "guard-failed" | "failed" | "no result";
}

interface Run {
  options: BenchOptions;
  temp: string;
  provider: ProviderStub;
  registry: string;
  key: string;
  failures: string[];
  launches: number;
  guards: { checked: number; failed: number };
  rows: LaunchRow[];
  seeds: string[];
  a: Tree;
  b: Tree;
  sameBuild: boolean;
  notes: string[];
}

const say = (line: string): void => {
  console.log(`frontend-bench: ${line}`);
};

function fail(run: Run, message: string): void {
  run.failures.push(message);
  console.error(`frontend-bench: FAIL ${message}`);
}

function note(run: Run, message: string): void {
  run.notes.push(message);
  say(message);
}

async function commitOf(workdir: string): Promise<string> {
  const result = await run(executable("git"), ["rev-parse", "HEAD"], { cwd: workdir });
  return result.code === 0 ? result.stdout.trim() : "";
}

function hasShotHook(workdir: string): boolean {
  const file = path.join(workdir, "internal", "desktop", "smoke.go");
  if (!existsSync(file)) return false;
  const source = readFileSync(file, "utf8");
  return source.includes("PASSQUEST_DESKTOP_SHOT_DIR") && source.includes("SmokeShot");
}

function treeOf(key: "a" | "b", root: string, temp: string): Tree {
  const workdir = path.resolve(root);
  return {
    key,
    root,
    workdir,
    binary: path.join(workdir, "build", "desktop", executable("passquest-desktop")),
    display: path.join(root, "build", "desktop", executable("passquest-desktop")),
    commit: "",
    hook: false,
    bundles: [],
    template: path.join(temp, key, "template", "data"),
  };
}

function printTail(launched: Launched): void {
  const text = tail(launched.output, OUTPUT_TAIL_LINES);
  if (text !== "") console.error(`frontend-bench: the launch's last ${OUTPUT_TAIL_LINES} output lines:\n${text}`);
}

interface Started {
  launched: Launched;
  startedAt: number;
  row: LaunchRow;
}

interface LaunchSpec {
  label: string;
  kind: LaunchKind;
  script: string;
  data: string;
  shots: boolean;
  hidden?: boolean;
}

async function launch(run: Run, tree: Tree, spec: LaunchSpec): Promise<Started | null> {
  const dir = path.join(run.temp, "launches", spec.label);
  mkdirSync(dir, { recursive: true });
  run.launches++;
  const row: LaunchRow = { label: spec.label, tree: tree.key, kind: spec.kind, hidden: spec.hidden !== false, engine: "", guards: null, guardsWhy: spec.kind === "prepare" ? PREPARE_GUARDS_WHY : "", loaf: null, shots: "", error: "", result: "no result" };
  run.rows.push(row);
  const env: Record<string, string> = {
    PASSQUEST_DESKTOP_DATA: spec.data,
    PASSQUEST_DESKTOP_SECRET_SERVICE: `passquest-bench-${process.pid}-${run.launches}`,
    PASSQUEST_DESKTOP_CONTENT_SIZE: CONTENT_SIZE,
    PASSQUEST_PROVIDER_REGISTRY: run.registry,
    PASSQUEST_ANKI_ENDPOINT: UNREACHABLE_ANKI,
    [STUB_CREDENTIAL_ENV]: run.key,
  };
  if (spec.shots) {
    const shotDir = path.join(run.options.shots, spec.label);
    mkdirSync(shotDir, { recursive: true });
    env.PASSQUEST_DESKTOP_SHOT_DIR = shotDir;
  }
  const check: Check = (condition, message) => {
    if (!condition) fail(run, message);
    return condition;
  };
  const startedAt = Date.now();
  const launched = await launchPage({
    binary: tree.binary,
    cwd: tree.workdir,
    dir,
    label: spec.label,
    header: `frontend-bench: ${spec.label} (tree ${tree.key})`,
    where: `frontend-bench ${spec.label}`,
    script: spec.script,
    hidden: spec.hidden,
    orphanGraceMs: ORPHAN_GRACE_MS,
    env,
    check,
  });
  if (!launched) {
    say(`${spec.label}: the app wrote no result; the harness printed the app's own tail above`);
    return null;
  }
  return { launched, startedAt, row };
}

function countGuard(run: Run, label: string, guards: GuardReading | null, strict: boolean): string[] {
  run.guards.checked++;
  const problems = guardProblems(guards).map((message) => `${label}: guard failed: ${message}`);
  if (problems.length > 0) run.guards.failed++;
  if (!strict) return [];
  return problems;
}

/** The record's guard totals: the launches whose reading was graded, and those that carry none by design (the prepare launch, PREPARE_GUARDS_WHY). */
function guardsSummary(run: Run): { rule: string; checked: number; failed: number; notMeasured: number } {
  return { rule: GUARD_RULE, checked: run.guards.checked, failed: run.guards.failed, notMeasured: run.rows.filter((row) => row.guards === null).length };
}

function graded(run: Run, started: Started, strict: boolean): { reading: LaunchReading; metrics: Record<string, number> } | null {
  const label = started.row.label;
  const { reading, problems } = readLaunch(started.launched.value);
  if (problems.length > 0 || !reading) {
    fail(run, `${label}: ${problems.join("; ")}`);
    started.row.result = "failed";
    printTail(started.launched);
    return null;
  }
  started.row.engine = reading.engine;
  started.row.guards = reading.guards;
  started.row.loaf = loafTotals(reading.loaf);
  const guardMessages = countGuard(run, label, reading.guards, strict);
  if (reading.guardsFailed) {
    started.row.result = "guard-failed";
    for (const message of guardMessages.length > 0 ? guardMessages : [`${label}: guard failed`]) {
      if (strict) fail(run, message);
      else say(`${message} (discarded warm-up)`);
    }
    printTail(started.launched);
    return null;
  }
  const viewportMessages = viewportProblems(label, reading.viewport);
  if (viewportMessages.length > 0) {
    started.row.result = "failed";
    for (const message of viewportMessages) {
      if (strict) fail(run, message);
      else say(`${message} (discarded warm-up)`);
    }
    return null;
  }
  started.row.result = "ok";
  const metrics: Record<string, number> = { ...reading.metrics, [READY_METRIC]: reading.readyAt - started.startedAt };
  return { reading, metrics };
}

/** The launch's own shot verdict. A page area that is not the audited one is a failure, but the verdict stands: `unbound` says the tree predates the hook whatever the geometry. */
function gradedShot(run: Run, started: Started, strict: boolean): { viewport: ViewportReading | null; shots: string; readyAt: number } | null {
  const label = started.row.label;
  const { reading, problems } = readShotLaunch(started.launched.value);
  if (problems.length > 0 || !reading) {
    fail(run, `${label}: ${problems.join("; ")}`);
    started.row.result = "failed";
    printTail(started.launched);
    return null;
  }
  started.row.engine = reading.engine;
  started.row.guards = reading.guards;
  started.row.shots = reading.shots;
  started.row.error = reading.error ?? "";
  started.row.loaf = loafTotals(reading.loaf);
  const guardMessages = countGuard(run, label, reading.guards, strict);
  if (reading.guardsFailed) {
    started.row.result = "guard-failed";
    for (const message of guardMessages.length > 0 ? guardMessages : [`${label}: guard failed`]) {
      if (strict) fail(run, message);
      else say(`${message} (discarded warm-up)`);
    }
    printTail(started.launched);
    return null;
  }
  const viewport = reading.viewport;
  const viewportMessages = viewportProblems(label, viewport);
  if (viewportMessages.length > 0) {
    started.row.result = "failed";
    for (const message of viewportMessages) {
      if (strict) fail(run, message);
      else say(`${message} (discarded warm-up)`);
    }
  } else {
    started.row.result = "ok";
  }
  return { viewport, shots: reading.shots, readyAt: reading.readyAt };
}

function readShot(run: Run, label: string, state: string, viewport: ViewportReading): ShotRecord | null {
  const relative = `${label}/${state}.png`;
  const file = path.join(run.options.shots, relative);
  if (!existsSync(file)) {
    fail(run, `${label}: the page captured ${state} but ${relative} does not exist`);
    return null;
  }
  const bytes = readFileSync(file);
  if (bytes.length < MIN_SHOT_BYTES) {
    fail(run, `${label}: ${relative} is ${bytes.length} bytes, too small for a rendered 1280x800 page`);
    return null;
  }
  const pixels = pngSize(bytes);
  if (!pixels) {
    fail(run, `${label}: ${relative} is not a PNG with a readable IHDR header`);
    return null;
  }
  const geometry = { cssWidth: viewport.width, cssHeight: viewport.height, dpr: viewport.dpr, pxWidth: pixels.width, pxHeight: pixels.height };
  return { file: relative, bytes: bytes.length, sha256: sha256Hex(bytes), geometry };
}

async function buildTree(run: Run, tree: Tree): Promise<boolean> {
  const build = await buildDesktop(context(tree.workdir), tree.binary);
  if (!build.ok) {
    fail(run, `tree ${tree.key}: ${build.failures.join("; ")}`);
    return false;
  }
  if (!existsSync(tree.binary)) {
    fail(run, `tree ${tree.key}: the build wrote no binary at ${tree.display}`);
    return false;
  }
  tree.bundles = [];
  for (const name of ["main.js", "main.css"] as const) {
    const file = resolve(tree.workdir, `${FRONTEND_DIST}/${name}`);
    if (!existsSync(file)) {
      fail(run, `tree ${tree.key}: the bundle step wrote no ${FRONTEND_DIST}/${name}`);
      return false;
    }
    const bytes = readFileSync(file);
    tree.bundles.push({ file: `${FRONTEND_DIST}/${name}`, bytes: bytes.length, sha256: sha256Hex(bytes) });
  }
  return true;
}

async function seedTree(run: Run, tree: Tree): Promise<boolean> {
  run.seeds.push(tree.key);
  const built = await buildCLI(context(tree.workdir));
  if ("failure" in built) {
    fail(run, `tree ${tree.key}: go build ./cmd/passquest failed\n${tail(built.failure)}`);
    return false;
  }
  const cli = built.binary;
  mkdirSync(tree.template, { recursive: true });
  const database = path.join(tree.template, "passquest.sqlite");
  const call = (args: string[]) =>
    cliJson(cli, database, tree.workdir, args, { PASSQUEST_PROVIDER_REGISTRY: run.registry, [STUB_CREDENTIAL_ENV]: run.key });
  const check: Check = (condition, message) => {
    if (!condition) fail(run, message);
    return condition;
  };
  const source = await seedReviewedSource(call, check, path.join(run.temp, tree.key, "source"), SEED_PAGES, SEED_FIRST_PAGE_QUESTIONS, true);
  if (!source) return false;
  for (const args of [
    ["model", "test", STUB_MODEL],
    ["model", "use", STUB_MODEL],
  ]) {
    const result = await call(args);
    if (result.code !== 0) {
      fail(run, `tree ${tree.key}: passquest ${args.join(" ")} exit = ${result.code}: ${tail(result.stderr, 4)}`);
      return false;
    }
  }
  return true;
}

async function prepareTree(run: Run, tree: Tree): Promise<boolean> {
  const started = await launch(run, tree, { label: `prepare-${tree.key}`, kind: "prepare", script: prepareScript(), data: tree.template, shots: false });
  if (!started) return false;
  const value = started.launched.value;
  if (started.launched.value === undefined || !(typeof value === "object" && value !== null && Reflect.get(value, "prepared") === true)) {
    fail(run, `tree ${tree.key}: the prepare launch returned ${JSON.stringify(value)}`);
    started.row.result = "failed";
    printTail(started.launched);
    return false;
  }
  started.row.engine = stringField(field(value, "engine"), "userAgent") ?? "";
  started.row.loaf = loafTotals(readLoaf(field(value, "loaf")));
  started.row.result = "ok";
  return true;
}

function freshData(run: Run, tree: Tree, label: string): string {
  const data = path.join(run.temp, "data", `${tree.key}-${label}`);
  mkdirSync(path.dirname(data), { recursive: true });
  cpSync(tree.template, data, { recursive: true });
  return data;
}

function runnerInfo(): Record<string, string> {
  const info: Record<string, string> = {};
  for (const name of ["RUNNER_OS", "RUNNER_ARCH", "ImageOS", "ImageVersion"]) {
    const value = process.env[name];
    if (value !== undefined && value !== "") info[name] = value;
  }
  return info;
}

/** A non-empty --shots directory is refused, never cleared: a PNG from an earlier run could otherwise be read as this run's. */
function staleShotsDirectory(dir: string): string | null {
  if (!existsSync(dir)) return null;
  if (!statSync(dir).isDirectory()) return `--shots ${dir} exists and is not a directory`;
  const entries = readdirSync(dir);
  if (entries.length === 0) return null;
  return `--shots ${dir} already exists and is not empty (${entries.length} entries): a PNG from an earlier run could be read as this run's; remove it or pass a fresh directory`;
}

function newRun(options: BenchOptions): Run {
  const temp = mkdtempSync(path.join(tmpdir(), "passquest-frontend-bench-"));
  const provider = startProviderStub();
  provider.respond = () => chatReply(COACH_REPLY);
  const registry = path.join(temp, "registry.json");
  writeStubRegistry(registry, provider.endpoint);
  mkdirSync(options.shots, { recursive: true });
  return {
    options,
    temp,
    provider,
    registry,
    key: `sk-bench-${randomBytes(12).toString("hex")}`,
    failures: [],
    launches: 0,
    guards: { checked: 0, failed: 0 },
    rows: [],
    seeds: [],
    a: treeOf("a", options.mode === "smoke" ? options.root : options.aRoot, temp),
    b: treeOf("b", options.mode === "smoke" ? options.root : options.bRoot, temp),
    sameBuild: false,
    notes: [],
  };
}

async function closeRun(run: Run): Promise<void> {
  run.provider.stop();
  if (!(await removeTree(run.temp))) say(`could not delete the temporary tree ${run.temp}`);
}

function spreadLine(spread: Spread | null): string {
  if (!spread) return "no samples";
  const r = (value: number) => Math.round(value * 100) / 100;
  return `n=${spread.n} min=${r(spread.min)} q1=${r(spread.q1)} med=${r(spread.median)} q3=${r(spread.q3)} max=${r(spread.max)}`;
}

/** The frame cadence every launch's guard observed: what a number's window was rendering at. */
function cadenceLine(run: Run): string {
  const readings = run.rows.flatMap((row) => (row.guards === null ? [] : [row.guards]));
  const ticks = readings.map((reading) => reading.ticks);
  const elapsed = readings.map((reading) => Math.round(reading.elapsedMs));
  if (ticks.length === 0) return "no guard reading";
  return `ticks ${Math.min(...ticks)}..${Math.max(...ticks)}, elapsed ${Math.min(...elapsed)}..${Math.max(...elapsed)} ms`;
}

function writeRecord(run: Run, record: Record<string, unknown>): void {
  writeFileSync(run.options.out, `${JSON.stringify(record, null, 2)}\n`);
  say(`record written to ${run.options.out}`);
}

function engineLine(trees: Record<string, string>): string {
  return Object.entries(trees)
    .map(([key, userAgent]) => `${key.toUpperCase()} ${userAgent}`)
    .join(" | ");
}

function addSample(samples: MetricSamples, pooled: number[], sample: { reading: LaunchReading; metrics: Record<string, number> }): void {
  pooled.push(...sample.reading.frames);
  for (const [name, value] of Object.entries(sample.metrics)) (samples[name] ??= []).push(value);
  const summary = summariseFrames(sample.reading.frames);
  if (!summary) return;
  (samples["frames.median"] ??= []).push(summary.median);
  (samples["frames.p95"] ??= []).push(summary.p95);
  (samples["frames.longest"] ??= []).push(summary.longest);
}

/** The record's own arithmetic, checked before it is written: one row per launch, the plan's total, one seeded template, one shown window per measured launch. */
function checkRunShape(run: Run, plan: LaunchPlan): void {
  for (const problem of recordShapeProblems({ seeds: run.seeds, rows: run.rows, launches: run.launches, plan })) fail(run, problem);
}

async function runCompare(options: BenchOptions): Promise<number> {
  const run = newRun(options);
  const started = new Date();
  const engines: Record<string, string> = {};
  const samples: Record<"a" | "b" | "controlA" | "controlB", MetricSamples> = { a: {}, b: {}, controlA: {}, controlB: {} };
  const pooled: Record<"a" | "b" | "controlA" | "controlB", number[]> = { a: [], b: [], controlA: [], controlB: [] };
  const shots: Record<"a" | "b" | "controlA" | "controlB", Record<string, ShotRecord>> = { a: {}, b: {}, controlA: {}, controlB: {} };
  const shotTally: Record<"a" | "b", { captured: number; unbound: number; failed: number }> = {
    a: { captured: 0, unbound: 0, failed: 0 },
    b: { captured: 0, unbound: 0, failed: 0 },
  };
  const record: Record<string, unknown> = {
    tool: "frontend-bench",
    version: 1,
    mode: "compare",
    label: options.label,
    startedAt: started.toISOString(),
    platform: { os: process.platform, arch: process.arch, release: osRelease() },
    runner: runnerInfo(),
  };
  try {
    for (const tree of [run.a, run.b]) {
      tree.commit = await commitOf(tree.workdir);
      tree.hook = hasShotHook(tree.workdir);
    }
    run.sameBuild = run.a.commit !== "" ? run.a.commit === run.b.commit : run.a.workdir === run.b.workdir;
    const builds = run.sameBuild ? [run.b] : [run.a, run.b];
    for (const tree of builds) if (!(await buildTree(run, tree))) throw new Error("a tree did not build");
    if (run.sameBuild) {
      run.a.binary = run.b.binary;
      run.a.display = run.b.display;
      run.a.hook = run.b.hook;
      run.a.bundles = run.b.bundles;
      note(run, `a and b are the same build (${run.a.commit || run.b.workdir}): one build, one template, and the comparison is itself the same-build control pair`);
    }
    const plan = launchPlan({ samples: options.samples, warmup: options.warmup, sameBuild: run.sameBuild });
    const template = templatePlan(run.sameBuild);
    const seed = template.seedKey === "a" ? run.a : run.b;
    if (!(await seedTree(run, seed))) throw new Error("the template was not seeded");
    if (!(await prepareTree(run, seed))) throw new Error("the template was not prepared");
    for (const tree of [run.a, run.b]) tree.template = seed.template;
    say(`template: one, seeded and prepared by tree ${seed.key}, copied for every launch of both trees (${template.reason})`);
    for (const key of ["a", "b"] as const) {
      for (const bundle of run[key].bundles) samples[key][bundle.file.endsWith("main.js") ? BUNDLE_METRICS[0] : BUNDLE_METRICS[1]] = [bundle.bytes];
    }

    for (let index = 0; index < options.warmup; index++) {
      for (const tree of [run.a, run.b]) {
        const label = `warmup-${tree.key}-${index}`;
        const launched = await launch(run, tree, { label, kind: "warmup", script: timingScript(), data: freshData(run, tree, `warmup-${index}`), shots: false, hidden: false });
        if (!launched) throw new Error(`the warm-up launch ${label} wrote no result`);
        graded(run, launched, false);
      }
    }
    for (let index = 0; index < options.samples; index++) {
      for (const tree of [run.a, run.b]) {
        const label = `sample-${tree.key}-${index}`;
        const launched = await launch(run, tree, { label, kind: "sample", script: timingScript(), data: freshData(run, tree, `sample-${index}`), shots: false, hidden: false });
        if (!launched) throw new Error(`the measured launch ${label} wrote no result`);
        const sample = graded(run, launched, true);
        if (!sample) continue;
        engines[tree.key] = sample.reading.engine;
        addSample(samples[tree.key], pooled[tree.key], sample);
      }
    }
    if (!run.sameBuild) {
      for (let index = 0; index < options.samples; index++) {
        for (const key of ["controlA", "controlB"] as const) {
          const label = `${key === "controlA" ? "control-a" : "control-b"}-${index}`;
          const launched = await launch(run, run.b, { label, kind: "control", script: timingScript(), data: freshData(run, run.b, label), shots: false, hidden: false });
          if (!launched) throw new Error(`the control launch ${label} wrote no result`);
          const sample = graded(run, launched, true);
          if (!sample) continue;
          addSample(samples[key], pooled[key], sample);
        }
      }
    }

    for (const state of SHOT_STATES) {
      for (const tree of [run.a, run.b]) {
        const label = `shot-${tree.key}-${state}`;
        const launched = await launch(run, tree, { label, kind: "shot", script: shotScript(state), data: freshData(run, tree, label), shots: true, hidden: false });
        if (!launched) throw new Error(`the shot launch ${label} wrote no result`);
        const reading = gradedShot(run, launched, true);
        if (!reading) continue;
        engines[tree.key] ??= launched.row.engine;
        if (reading.shots === "unbound") {
          shotTally[tree.key].unbound++;
          continue;
        }
        if (reading.shots !== "captured") {
          shotTally[tree.key].failed++;
          launched.row.result = "failed";
          fail(run, `${label}: the page reported shots = ${reading.shots}${launched.row.error === "" ? "" : `: ${launched.row.error}`}`);
          continue;
        }
        if (reading.viewport === null) {
          shotTally[tree.key].failed++;
          launched.row.result = "failed";
          continue;
        }
        const shot = readShot(run, label, state, reading.viewport);
        if (shot) {
          shots[tree.key][state] = shot;
          shotTally[tree.key].captured++;
        } else {
          shotTally[tree.key].failed++;
          launched.row.result = "failed";
        }
      }
    }
    if (!run.sameBuild) {
      for (const state of SHOT_STATES) {
        for (const key of ["controlA", "controlB"] as const) {
          const label = `${key === "controlA" ? "shot-control-a" : "shot-control-b"}-${state}`;
          const launched = await launch(run, run.b, { label, kind: "control-shot", script: shotScript(state), data: freshData(run, run.b, label), shots: true, hidden: false });
          if (!launched) throw new Error(`the control shot launch ${label} wrote no result`);
          const reading = gradedShot(run, launched, true);
          if (!reading) continue;
          if (reading.shots !== "captured") {
            launched.row.result = "failed";
            fail(run, `${label}: the page reported shots = ${reading.shots}${launched.row.error === "" ? "" : `: ${launched.row.error}`}`);
            continue;
          }
          if (reading.viewport === null) {
            launched.row.result = "failed";
            continue;
          }
          const shot = readShot(run, label, state, reading.viewport);
          if (shot) shots[key][state] = shot;
          else launched.row.result = "failed";
        }
      }
    }

    const shotState: Record<"a" | "b", "captured" | "unbound" | "failed"> = { a: "failed", b: "failed" };
    for (const key of ["a", "b"] as const) {
      shotState[key] = shotStateOf(shotTally[key], SHOT_STATES.length);
      if (shotState[key] !== "unbound") continue;
      if (!run[key].hook) note(run, `tree ${key} (${run[key].commit}): shots not-measured (base predates the capture hook)`);
      else fail(run, `tree ${key} (${run[key].display}) carries the capture hook but a shot launch reported it unbound`);
    }

    const metrics = metricRows(samples);
    for (const row of metrics) {
      if (row.verdict.worse) {
        fail(
          run,
          run.sameBuild
            ? `the same-build pair of ${row.name} reports a worse head (${row.verdict.detail}): one build measured twice moved beyond the noise, so this run's numbers are not readable`
            : `metric ${row.name} is worse beyond noise: ${row.verdict.detail}`,
        );
      }
      if (!run.sameBuild && row.controlVerdict.worse) {
        fail(run, `the same-build control pair of ${row.name} reports a worse head (${row.controlVerdict.detail}): this run's comparison is not readable`);
      }
    }

    const shotsMeasured = shotState.a === "captured" && shotState.b === "captured";
    const comparison = shotsMeasured ? compareShots(shots.a, shots.b) : { rows: [] as ShotRow[], problems: [] as string[] };
    const controlShots = run.sameBuild ? { rows: [] as ShotRow[], problems: [] as string[] } : compareShots(shots.controlA, shots.controlB);
    const controlCompared = !run.sameBuild && controlShots.rows.every((row) => row.a !== null && row.b !== null);
    for (const problem of comparison.problems) fail(run, problem);
    if (!run.sameBuild) {
      for (const problem of controlShots.problems) fail(run, `control pair: ${problem}`);
      for (const row of controlShots.rows) {
        if (row.a === null || row.b === null) continue;
        if (!row.identical) fail(run, `control pair: state ${row.state} is not byte-identical across two launches of one build: the capture is nondeterministic`);
      }
    }
    let allowed: string[] = [];
    if (shotsMeasured) {
      const file = resolve(OWN_TREE, VISUAL_ALLOWLIST);
      if (!existsSync(file)) {
        fail(run, `${VISUAL_ALLOWLIST} is missing`);
      } else {
        const allow = readVisualAllowlist(readFileSync(file, "utf8"));
        if ("error" in allow) fail(run, allow.error);
        else {
          const findings = visualFindings(comparison.rows, allow.entries);
          for (const line of findings.findings) fail(run, line);
          for (const line of findings.stale) fail(run, line);
          allowed = findings.allowed;
        }
      }
    } else {
      note(run, `shots compared: no (${shotState.a === "captured" ? "the base" : `tree a ${shotState.a}`} / ${shotState.b === "captured" ? "the head" : `tree b ${shotState.b}`})`);
    }

    const identical = comparison.rows.filter((row) => row.identical).length;
    const controlIdentical = controlShots.rows.filter((row) => row.identical).length;
    const guards = guardsSummary(run);
    Object.assign(record, {
      tool: "frontend-bench",
      version: 1,
      mode: "compare",
      label: options.label,
      method: {
        samples: options.samples,
        warmup: options.warmup,
        contentSize: CONTENT_SIZE,
        alternation: "A then B, launch by launch, serially in this job",
        control: run.sameBuild
          ? "a and b are the same commit: the comparison above is the same-build pair"
          : `two interleaved sets of ${options.samples} launches of the B build, after the comparison phase`,
        verdict: VERDICT_RULE,
        guards: `${GUARD_RULE}; a launch whose guard fails records no sample; ${PREPARE_GUARDS_WHY}`,
        metrics: METRIC_NOTES,
        shots: {
          states: SHOT_STATES,
          how: "one visible launch per state and per side, the desktop-ux settle style (desktop/web/styles/responsive.css:9) extended over the runner's cursor and this launch's own measurements: animations, transitions and the caret off, hit-testing inert (no driver step uses the pointer, so a window under the cursor cannot capture the hovered rendering — on Feedback a hovered option lights its own evidence, not the key's), the clock readouts, the answered-in chip's elapsed time and the reply's measured latency pinned, and the send's own budget re-read awaited before the capture; each shot records the page's css size and devicePixelRatio and the PNG's own pixel size (its IHDR), and a page area that is not the audited one fails the shot",
          a: shotState.a,
          b: shotState.b,
          allowlist: VISUAL_ALLOWLIST,
        },
      },
      template: { seededBy: seed.key, preparedBy: seed.key, trees: 1, reason: template.reason },
      control: run.sameBuild
        ? { samples: "none", why: "a and b are the same commit: the A/B columns above are two interleaved sets of one build and are themselves this run's same-build pair, so controlA/controlB carry no samples" }
        : { samples: `two interleaved sets of ${options.samples} launches of the B build`, why: null },
      trees: {
        a: { root: run.a.root, commit: run.a.commit, binary: run.a.display, shotHook: run.a.hook, userAgent: engines.a ?? "", bundles: run.a.bundles },
        b: { root: run.b.root, commit: run.b.commit, binary: run.b.display, shotHook: run.b.hook, userAgent: engines.b ?? "", bundles: run.b.bundles },
      },
      launches: { ...plan, total: run.launches, rows: run.rows },
      guards,
      metrics,
      frames: {
        pooled: Object.fromEntries(Object.entries(pooled).map(([key, values]) => [key, summariseFrames(values)])),
      },
      shots: {
        dir: options.shots,
        rows: comparison.rows.map((row) => ({
          state: row.state,
          a: row.a,
          b: row.b,
          identical: row.identical,
          allowed: allowed.some((line) => line.includes(` ${row.state} `)),
        })),
        identical: shotsMeasured ? identical : null,
        why: shotsMeasured ? null : `shots not compared: A ${shotState.a}, B ${shotState.b}`,
        control: run.sameBuild
          ? { rows: [], identical: null, why: "the A/B rows above are this run's same-build pair" }
          : controlCompared
            ? { rows: controlShots.rows.map((row) => ({ state: row.state, identical: row.identical })), identical: controlIdentical, why: null }
            : { rows: [], identical: null, why: "the control pair's captures are not all in the record: each rejected control launch is in failures" },
        allowed,
      },
      notes: run.notes,
      failures: run.failures,
      ok: run.failures.length === 0,
    });

    say(`${options.label || "compare run"}`);
    say(`A ${run.a.root} ${run.a.commit} (shots ${shotState.a}); B ${run.b.root} ${run.b.commit} (shots ${shotState.b})`);
    say(`engine ${engineLine(engines)}`);
    say(`launches ${run.launches}; guards checked ${guards.checked}, failed ${guards.failed}, not measured ${guards.notMeasured}; ${cadenceLine(run)}`);
    if (engines.a && engines.b && engines.a !== engines.b) say("the two trees report different engines: only a warning, the matrices do not pin one Chromium build");
    for (const row of metrics) {
      say(`${row.name}: A ${spreadLine(row.a)} | B ${spreadLine(row.b)} | verdict ${row.verdict.worse ? "WORSE" : "ok"} (${row.verdict.detail})`);
      if (!run.sameBuild) say(`${row.name}: control ${spreadLine(row.controlA)} | ${spreadLine(row.controlB)} | ${row.controlVerdict.worse ? "WORSE" : "ok"} (${row.controlVerdict.detail})`);
    }
    if (shotsMeasured) say(`shots: ${identical}/${comparison.rows.length} state(s) byte-identical between A and B`);
    else say(`shots: not compared (A ${shotState.a}, B ${shotState.b})`);
    if (!run.sameBuild) {
      say(
        controlCompared
          ? `shots of the control pair: ${controlIdentical}/${controlShots.rows.length} state(s) byte-identical`
          : "shots of the control pair: not compared (the control launches' captures are in failures)",
      );
    }
    for (const line of allowed) say(`allowed: ${line}`);
    for (const line of run.notes) say(`note: ${line}`);
  } catch (error) {
    fail(run, error instanceof Error ? error.message : String(error));
  } finally {
    checkRunShape(run, launchPlan({ samples: options.samples, warmup: options.warmup, sameBuild: run.sameBuild }));
    Object.assign(record, { failures: run.failures, ok: run.failures.length === 0, seconds: Math.round((Date.now() - started.getTime()) / 1000) });
    writeRecord(run, record);
    if (run.failures.length === 0) say(`OK: ${run.launches} launch(es), ${run.guards.checked} guard check(s), 0 failures`);
    else say(`FAIL: ${run.failures.length} failure(s)`);
    await closeRun(run);
  }
  return run.failures.length === 0 ? 0 : 1;
}

async function runSmoke(options: BenchOptions): Promise<number> {
  const run = newRun(options);
  const started = new Date();
  const plan = smokeLaunchPlan();
  const shots: Record<string, ShotRecord> = {};
  const ready: Record<string, number> = {};
  let engine = "";
  const record: Record<string, unknown> = {
    tool: "frontend-bench",
    version: 1,
    mode: "smoke",
    label: options.label,
    startedAt: started.toISOString(),
    platform: { os: process.platform, arch: process.arch, release: osRelease() },
    runner: runnerInfo(),
  };
  try {
    const tree = run.b;
    tree.commit = await commitOf(tree.workdir);
    tree.hook = hasShotHook(tree.workdir);
    if (!(await buildTree(run, tree))) throw new Error("the tree did not build");
    if (!tree.hook) throw new Error(`${tree.root} carries no capture hook (internal/desktop/smoke.go)`);
    if (!(await seedTree(run, tree))) throw new Error("the tree's template was not seeded");
    if (!(await prepareTree(run, tree))) throw new Error("the tree's template was not prepared");
    for (const state of SHOT_STATES) {
      const label = `smoke-${state}`;
      const launched = await launch(run, tree, { label, kind: "shot", script: shotScript(state), data: freshData(run, tree, label), shots: true, hidden: false });
      if (!launched) throw new Error(`the smoke launch ${label} wrote no result`);
      const reading = gradedShot(run, launched, true);
      if (!reading) continue;
      engine ||= launched.row.engine;
      ready[state] = reading.readyAt - launched.startedAt;
      if (reading.shots !== "captured") {
        launched.row.result = "failed";
        fail(run, `${label}: the page reported shots = ${reading.shots}${launched.row.error === "" ? "" : `: ${launched.row.error}`}`);
        continue;
      }
      if (reading.viewport === null) {
        launched.row.result = "failed";
        continue;
      }
      const shot = readShot(run, label, state, reading.viewport);
      if (shot) shots[state] = shot;
      else launched.row.result = "failed";
    }
    const guards = guardsSummary(run);
    Object.assign(record, {
      tool: "frontend-bench",
      version: 1,
      mode: "smoke",
      label: options.label,
      template: { seededBy: tree.key, preparedBy: tree.key, trees: 1, reason: "the render smoke's one tree seeds and prepares its own template" },
      method: {
        contentSize: CONTENT_SIZE,
        guards: `${GUARD_RULE}; ${PREPARE_GUARDS_WHY}`,
        states: SHOT_STATES,
        how: "one visible launch per state of one build: the Linux job proves the states render, and compares no numbers; the same settle as the comparison run (hit-testing inert, the live readouts pinned, the send's budget re-read awaited before the capture)",
      },
      trees: { b: { root: tree.root, commit: tree.commit, binary: tree.display, shotHook: tree.hook, userAgent: engine, bundles: tree.bundles } },
      launches: { ...plan, total: run.launches, rows: run.rows },
      guards,
      readyMs: ready,
      shots: { dir: options.shots, rows: SHOT_STATES.map((state) => ({ state, b: shots[state] ?? null, identical: false })) },
      notes: run.notes,
      failures: run.failures,
      ok: run.failures.length === 0,
    });
    say(`${options.label || "render smoke"} on ${tree.root} (${tree.commit})`);
    say(`engine ${engine}`);
    say(`launches ${run.launches}; guards checked ${guards.checked}, failed ${guards.failed}, not measured ${guards.notMeasured}`);
    say(`readyMs ${SHOT_STATES.map((state) => `${state}=${ready[state] ?? "-"}`).join(" ")}`);
    say(`shots ${SHOT_STATES.filter((state) => shots[state]).length}/${SHOT_STATES.length} written`);
  } catch (error) {
    fail(run, error instanceof Error ? error.message : String(error));
  } finally {
    checkRunShape(run, plan);
    Object.assign(record, { failures: run.failures, ok: run.failures.length === 0, seconds: Math.round((Date.now() - started.getTime()) / 1000) });
    writeRecord(run, record);
    if (run.failures.length === 0) say(`OK: ${run.launches} launch(es), ${run.guards.checked} guard check(s), 0 failures`);
    else say(`FAIL: ${run.failures.length} failure(s)`);
    await closeRun(run);
  }
  return run.failures.length === 0 ? 0 : 1;
}

function selfCheck(): number {
  let failed = 0;
  const cases = selfCheckCases();
  for (const item of cases) {
    let problem: string | null;
    try {
      problem = item.check();
    } catch (error) {
      problem = `threw: ${error instanceof Error ? error.message : String(error)}`;
    }
    if (problem === null) console.log(`[self-check] ${item.name}: pass`);
    else {
      failed += 1;
      console.log(`[self-check] ${item.name}: FAIL — ${problem}`);
    }
  }
  console.log(`[self-check] ${cases.length - failed} of ${cases.length} case(s) passed`);
  return failed === 0 ? 0 : 1;
}

async function main(argv: readonly string[]): Promise<number> {
  const options = parseBenchArgs(argv);
  if (options.mode === "help") {
    console.log(USAGE);
    return 0;
  }
  if (options.mode === "error") {
    console.error(`frontend-bench: ${options.error}\n\n${USAGE}`);
    return 2;
  }
  if (options.mode === "self-check") return selfCheck();
  const stale = staleShotsDirectory(options.shots);
  if (stale !== null) {
    console.error(`frontend-bench: ${stale}`);
    return 2;
  }
  say(`${options.mode} start`);
  return options.mode === "smoke" ? runSmoke(options) : runCompare(options);
}

await runMain("frontend-bench", main);
