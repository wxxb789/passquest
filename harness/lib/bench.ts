import path from "node:path";

import { pageScript, SKILL, STUB_PROVIDER } from "./desktop-page.ts";
import { field, numberField, stringField } from "./json.ts";

/** The first audited layout (Q-0012-05); the desktop-ux gate audits this width too. */
export const CONTENT_SIZE = "1280x800";
const [contentWidth, contentHeight] = CONTENT_SIZE.split("x");
export const CONTENT_WIDTH = Number(contentWidth);
export const CONTENT_HEIGHT = Number(contentHeight);
export const DEFAULT_SAMPLES = 9;
export const DEFAULT_WARMUP = 2;
/** A shorter frame sample than this means the sampler never ran. */
export const MIN_FRAME_DURATIONS = 20;
export const RAF_TICKS = 30;
/** 30 ticks is a real cadence even on a loaded runner; a throttled window cannot reach them inside this budget. */
export const RAF_BUDGET_MS = 4000;
export const MEDIAN_GAP_FRACTION = 0.05;
export const MEDIAN_GAP_FLOOR_MS = 5;
const FLOOR_SOURCES = ["5ms", "5%", "control IQR"] as const;
export type FloorSource = (typeof FLOOR_SOURCES)[number];
export const VERDICT_RULE = `head Q1 > base Q3 and the median gap >= the floor = max(${MEDIAN_GAP_FLOOR_MS} ms, ${
  MEDIAN_GAP_FRACTION * 100
}% of the base median, the larger of the two control-pair sides' IQRs (q3 - q1)) where the metric has control-pair samples, and max(${MEDIAN_GAP_FLOOR_MS} ms, ${
  MEDIAN_GAP_FRACTION * 100
}% of the base median) where it has none; every verdict names its floor and which term set it (${FLOOR_SOURCES.join(", ")})`;
export const GUARD_RULE = `document.visibilityState === "visible" and ${RAF_TICKS} requestAnimationFrame ticks within ${RAF_BUDGET_MS} ms`;
/** Why the one prepare launch carries no guard reading: it measures nothing, and its hidden window's page draws no frames on macOS and Linux (mygo@v0.10.0 page.go: ThrottlingOff), where a wait on an animation frame never finishes. */
export const PREPARE_GUARDS_WHY =
  "the prepare launch is not measured and its window is hidden; a hidden window's page draws no frames on macOS and Linux (mygo@v0.10.0 page.go, ThrottlingOff), so its script waits on no animation frame";
export const VISUAL_ALLOWLIST = "harness/frontend-visual-allow.json";
export const SHOT_STATES = ["today", "sheet-open", "practice-independent", "feedback-coach", "settings-row", "report"] as const;
export type ShotState = (typeof SHOT_STATES)[number];

export const PAGE_METRICS = [
  "answerSelectMs",
  "submitFeedbackMs",
  "coachReplyMs",
  "roomSwitch.report",
  "roomSwitch.anki",
  "roomSwitch.settings",
  "roomSwitch.assess",
] as const;
export const READY_METRIC = "readyMs";
export const TIMING_METRICS = [READY_METRIC, ...PAGE_METRICS] as const;
export const FRAME_METRICS = ["frames.median", "frames.p95", "frames.longest"] as const;
export const BUNDLE_METRICS = ["bundle.mainJs", "bundle.mainCss"] as const;
export const METRIC_ROWS = [...TIMING_METRICS, ...FRAME_METRICS, ...BUNDLE_METRICS] as const;

export interface BenchOptions {
  mode: "compare" | "smoke" | "self-check" | "help" | "error";
  aRoot: string;
  bRoot: string;
  root: string;
  out: string;
  shots: string;
  label: string;
  samples: number;
  warmup: number;
  error: string;
}

export const USAGE = `Run the frontend bench on two built trees, or one tree's render smoke.

  bun harness/tools/frontend-bench.ts --self-check
  bun harness/tools/frontend-bench.ts --a-root <dir> --b-root <dir> --out <file> [--shots <dir>] \\
      [--samples ${DEFAULT_SAMPLES}] [--warmup ${DEFAULT_WARMUP}] [--label <text>]
  bun harness/tools/frontend-bench.ts --root <dir> --smoke --out <file> [--shots <dir>]

  --a-root <dir>   the base tree (A), checked out beside the tree the tool runs from
  --b-root <dir>   the tree under test (B); the tool builds both trees with the harness of the tree it runs from
  --root <dir>     one tree: --smoke launches a visible window per state and needs no comparison
  --out <file>     the JSON record (always written, also on a failed run)
  --shots <dir>    where the state screenshots go; default <out's dir>/shots; must not exist or be empty
  --samples <n>    measured launches per build per metric (default ${DEFAULT_SAMPLES})
  --warmup <n>     discarded launches per build before the measured ones (default ${DEFAULT_WARMUP})
  --label <text>   what the record says this run was (the workflow passes its refs)
  --self-check     run the argument, spread, verdict, guard, launch and shot cases over fixtures
  --help           this text

The same-build control pair is measured on the B tree after the comparison: two interleaved sets of
--samples launches of one build, the noise floor every verdict is read against (${VERDICT_RULE}).
When A and B are the same commit the A/B columns are themselves two interleaved sets of one build,
and the record says so instead of launching a third pair.
Exit codes: 0 every build, launch, guard and verdict passed, 1 a failure (the record lists it), 2
usage.`;

function optionError(options: BenchOptions, error: string): BenchOptions {
  return { ...options, mode: "error", error };
}

function takeValue(argv: readonly string[], index: number): { value: string; next: number } | null {
  const next = argv[index + 1];
  if (next === undefined || next === "" || next.startsWith("--")) return null;
  return { value: next, next: index + 1 };
}

function parseCount(options: BenchOptions, arg: string, raw: string | null, minimum: number): number | BenchOptions {
  const count = raw === null ? Number.NaN : Number(raw);
  if (!Number.isInteger(count) || count < minimum || count > 99) {
    return optionError(options, `${arg} ${raw ?? ""} is not a whole count between ${minimum} and 99`);
  }
  return count;
}

export function parseBenchArgs(argv: readonly string[]): BenchOptions {
  let options: BenchOptions = {
    mode: "compare",
    aRoot: "",
    bRoot: "",
    root: "",
    out: "",
    shots: "",
    label: "",
    samples: DEFAULT_SAMPLES,
    warmup: DEFAULT_WARMUP,
    error: "",
  };
  let smoke = false;
  for (let index = 0; index < argv.length; index++) {
    const arg = argv[index] ?? "";
    const taken = takeValue(argv, index);
    const value = (): string | null => {
      if (taken) index = taken.next;
      return taken?.value ?? null;
    };
    switch (arg) {
      case "--a-root":
      case "--b-root":
      case "--root":
      case "--out":
      case "--shots":
      case "--label": {
        const raw = value();
        if (raw === null) return optionError(options, `${arg} needs a value`);
        if (arg === "--a-root") options = { ...options, aRoot: raw };
        else if (arg === "--b-root") options = { ...options, bRoot: raw };
        else if (arg === "--root") options = { ...options, root: raw };
        else if (arg === "--out") options = { ...options, out: raw };
        else if (arg === "--shots") options = { ...options, shots: raw };
        else options = { ...options, label: raw };
        break;
      }
      case "--samples": {
        const count = parseCount(options, arg, value(), 1);
        if (typeof count !== "number") return count;
        options = { ...options, samples: count };
        break;
      }
      case "--warmup": {
        const count = parseCount(options, arg, value(), 0);
        if (typeof count !== "number") return count;
        options = { ...options, warmup: count };
        break;
      }
      case "--smoke":
        smoke = true;
        break;
      case "--self-check":
        return { ...options, mode: "self-check" };
      case "--help":
      case "-h":
        return { ...options, mode: "help" };
      default:
        return optionError(options, `unknown argument ${JSON.stringify(arg)}`);
    }
  }
  if (smoke) {
    if (!options.root || options.aRoot || options.bRoot) {
      return optionError(options, `--smoke needs --root <dir> and neither --a-root nor --b-root`);
    }
    if (!options.out) return optionError(options, "--smoke needs --out <file>");
    return resolvedOutput(options, "smoke");
  }
  if (!options.aRoot || !options.bRoot) return optionError(options, "compare mode needs --a-root <dir> and --b-root <dir>");
  if (!options.out) return optionError(options, "compare mode needs --out <file>");
  return resolvedOutput(options, "compare");
}

/** Resolved once here: the app writes screenshots relative to its own cwd, so the tool must read them back from the same absolute path whatever directory it was started in. */
function resolvedOutput(options: BenchOptions, mode: "compare" | "smoke"): BenchOptions {
  const out = path.resolve(options.out);
  return { ...options, mode, out, shots: path.resolve(options.shots || path.join(path.dirname(out), "shots")) };
}

export interface TemplatePlan {
  seedKey: "a" | "b";
  reason: string;
}

/**
 * One template for both trees: the base build seeds and prepares it, so the head only ever reads
 * the older schema, and no launch can see a second seeding (two seedings differ in `checked_at`,
 * which the settings row renders: desktop/web/src/views/settings.ts:92).
 */
export function templatePlan(sameBuild: boolean): TemplatePlan {
  return sameBuild
    ? { seedKey: "b", reason: "a and b are the same commit (one build): the built tree seeds the one template every launch copies" }
    : { seedKey: "a", reason: "the base tree seeds and prepares the one template every launch of both trees copies" };
}

/** Empty for the one seed a run must make; a second seeding is the difference the settings row would show. */
export function seedProblems(seeds: readonly string[]): string[] {
  if (seeds.length === 1) return [];
  return [`${seeds.length} tree(s) seeded their own template (${seeds.join(", ") || "none"}): both trees launch from copies of one template`];
}

export interface LaunchPlan {
  prepare: number;
  warmup: number;
  measured: number;
  control: number;
  shots: number;
  controlShots: number;
  total: number;
}

/** What a comparison run launches per phase, so the record's total is checkable against its parts. */
export function launchPlan(options: { samples: number; warmup: number; sameBuild: boolean }): LaunchPlan {
  const prepare = 1;
  const warmup = options.warmup * 2;
  const measured = options.samples * 2;
  const control = options.sameBuild ? 0 : options.samples * 2;
  const shots = SHOT_STATES.length * 2;
  const controlShots = options.sameBuild ? 0 : SHOT_STATES.length * 2;
  return { prepare, warmup, measured, control, shots, controlShots, total: prepare + warmup + measured + control + shots + controlShots };
}

export function smokeLaunchPlan(): LaunchPlan {
  const prepare = 1;
  const shots = SHOT_STATES.length;
  return { prepare, warmup: 0, measured: 0, control: 0, shots, controlShots: 0, total: prepare + shots };
}

export const MEASURED_KINDS = ["warmup", "sample", "control", "shot", "control-shot"] as const;

export interface LaunchShapeRow {
  label: string;
  kind: string;
  /** WebView2 reports a hidden window as `visibility: "visible"` and keeps its ticks, so the request is the only witness that a measured launch was meant to be shown. */
  hidden: boolean;
  /** The launch's frame-guard reading; null for a launch that waits on no animation frame (the prepare launch). */
  guards: GuardReading | null;
  /** Why there is no reading; empty when there is one (PREPARE_GUARDS_WHY). */
  guardsWhy: string;
}

/** The record's own arithmetic, checked before it is written: one row per launch, the plan's total, one seeded template, a shown window for every measured launch, and no guard reading without the reason why a launch waits on no animation frame. */
export function recordShapeProblems(input: {
  seeds: readonly string[];
  rows: readonly LaunchShapeRow[];
  launches: number;
  plan: LaunchPlan;
}): string[] {
  const problems = seedProblems(input.seeds);
  if (input.rows.length !== input.launches) {
    problems.push(`${input.rows.length} launch row(s) for ${input.launches} launch(es): every launch must be in the record`);
  }
  if (input.plan.total !== input.launches) {
    problems.push(`the launch plan says ${input.plan.total} launch(es), the run made ${input.launches} (${JSON.stringify(input.plan)})`);
  }
  for (const row of input.rows) {
    const measured = (MEASURED_KINDS as readonly string[]).includes(row.kind);
    if (row.hidden && measured) {
      problems.push(`${row.label}: a ${row.kind} launch requested the hidden window; every measured launch must request a shown one`);
    }
    if (!measured && row.guards !== null) {
      problems.push(`${row.label}: a ${row.kind} launch carries a guard reading; only a measured launch waits on animation frames, and a hidden window's page draws none on macOS and Linux`);
    }
    if (!measured && row.guards === null && row.guardsWhy.trim() === "") {
      problems.push(`${row.label}: a ${row.kind} launch carries no guard reading and no guardsWhy saying why`);
    }
  }
  return problems;
}

export interface Spread {
  n: number;
  min: number;
  q1: number;
  median: number;
  q3: number;
  max: number;
}

/** The p-th percentile of a sorted sample, linearly interpolated between the nearest two order statistics (type 7, the definition R and numpy default to). */
export function percentile(sorted: readonly number[], p: number): number {
  if (sorted.length === 0) return Number.NaN;
  if (sorted.length === 1) return sorted[0] as number;
  const index = (sorted.length - 1) * p;
  const lower = Math.floor(index);
  const upper = Math.ceil(index);
  const weight = index - lower;
  return (sorted[lower] as number) * (1 - weight) + (sorted[upper] as number) * weight;
}

export function spread(values: readonly number[]): Spread | null {
  if (values.length === 0) return null;
  const sorted = [...values].sort((a, b) => a - b);
  return {
    n: sorted.length,
    min: sorted[0] as number,
    q1: percentile(sorted, 0.25),
    median: percentile(sorted, 0.5),
    q3: percentile(sorted, 0.75),
    max: sorted[sorted.length - 1] as number,
  };
}

export interface Verdict {
  worse: boolean;
  /** The gap threshold read against, null when a side has no samples. */
  floor: number | null;
  /** Which term set the floor; VERDICT_RULE names the three. */
  floorSource: FloorSource | null;
  detail: string;
}

const round2 = (value: number): number => Math.round(value * 100) / 100;

export function noiseFloor(baseMedian: number, controls: readonly (Spread | null)[]): { floor: number; floorSource: FloorSource } {
  const fiveMs = MEDIAN_GAP_FLOOR_MS;
  const fivePercent = baseMedian * MEDIAN_GAP_FRACTION;
  const sides = controls.filter((side): side is Spread => side !== null);
  if (sides.length > 0) {
    const controlIqr = Math.max(...sides.map((side) => side.q3 - side.q1));
    if (controlIqr >= fiveMs && controlIqr >= fivePercent) return { floor: controlIqr, floorSource: "control IQR" };
  }
  if (fiveMs >= fivePercent) return { floor: fiveMs, floorSource: "5ms" };
  return { floor: fivePercent, floorSource: "5%" };
}

export function worseBeyondNoise(base: Spread | null, head: Spread | null, controls: readonly (Spread | null)[] = []): Verdict {
  if (!base || !head) {
    return { worse: false, floor: null, floorSource: null, detail: `one side has no samples (base ${base ? base.n : 0}, head ${head ? head.n : 0})` };
  }
  const { floor, floorSource } = noiseFloor(base.median, controls);
  const gap = head.median - base.median;
  const above = head.q1 > base.q3;
  const beyond = gap >= floor;
  return {
    worse: above && beyond,
    floor,
    floorSource,
    detail: `q1 ${round2(head.q1)} vs base q3 ${round2(base.q3)} (${above ? "above" : "not above"}); median ${round2(base.median)} -> ${
      round2(head.median)
    }, gap ${round2(gap)} (${beyond ? "beyond" : "within"})`,
  };
}

/** The floor as the record and the log state it: the rounded value and the term that set it, "floor none" without samples. */
export function floorText(verdict: Verdict): string {
  if (verdict.floor === null) return "floor none";
  return `floor ${round2(verdict.floor)} (${verdict.floorSource})`;
}

export interface MetricSamples {
  [name: string]: number[];
}

export interface MetricRowResult {
  name: string;
  a: Spread | null;
  b: Spread | null;
  controlA: Spread | null;
  controlB: Spread | null;
  verdict: Verdict;
  controlVerdict: Verdict;
}

export function metricRows(samples: {
  a: MetricSamples;
  b: MetricSamples;
  controlA: MetricSamples;
  controlB: MetricSamples;
}): MetricRowResult[] {
  const names = new Set<string>();
  for (const side of [samples.a, samples.b, samples.controlA, samples.controlB]) {
    for (const name of Object.keys(side)) names.add(name);
  }
  const order = (name: string): number => {
    const index = (METRIC_ROWS as readonly string[]).indexOf(name);
    return index < 0 ? METRIC_ROWS.length : index;
  };
  return [...names]
    .sort((left, right) => order(left) - order(right) || left.localeCompare(right))
    .map((name) => {
      const a = spread(samples.a[name] ?? []);
      const b = spread(samples.b[name] ?? []);
      const controlA = spread(samples.controlA[name] ?? []);
      const controlB = spread(samples.controlB[name] ?? []);
      return {
        name,
        a,
        b,
        controlA,
        controlB,
        verdict: worseBeyondNoise(a, b, [controlA, controlB]),
        controlVerdict: worseBeyondNoise(controlA, controlB),
      };
    });
}

export interface GuardReading {
  visibility: string;
  ticks: number;
  elapsedMs: number;
}

export function readGuards(value: unknown): GuardReading | null {
  const guards = field(value, "guards");
  const visibility = stringField(guards, "visibility");
  const ticks = numberField(guards, "ticks");
  const elapsedMs = numberField(guards, "elapsedMs");
  if (visibility === undefined || ticks === undefined || elapsedMs === undefined) return null;
  if (!Number.isFinite(ticks) || !Number.isFinite(elapsedMs)) return null;
  return { visibility, ticks, elapsedMs };
}

export function guardProblems(reading: GuardReading | null): string[] {
  if (!reading) return [`the page returned no guard reading (want {guards:{visibility,ticks,elapsedMs}}); ${GUARD_RULE}`];
  const problems: string[] = [];
  if (reading.visibility !== "visible") problems.push(`document.visibilityState = ${JSON.stringify(reading.visibility)}`);
  if (!(reading.ticks >= RAF_TICKS)) {
    problems.push(`only ${reading.ticks} requestAnimationFrame tick(s) in ${Math.round(reading.elapsedMs)} ms (want ${RAF_TICKS} within ${RAF_BUDGET_MS} ms)`);
  } else if (reading.elapsedMs > RAF_BUDGET_MS) {
    problems.push(`${RAF_TICKS} requestAnimationFrame ticks took ${Math.round(reading.elapsedMs)} ms (want at most ${RAF_BUDGET_MS} ms)`);
  }
  return problems;
}

export interface ViewportReading {
  width: number;
  height: number;
  dpr: number;
}

export function readViewport(value: unknown): ViewportReading | null {
  const viewport = field(value, "viewport");
  const width = numberField(viewport, "width");
  const height = numberField(viewport, "height");
  const dpr = numberField(viewport, "dpr");
  if (width === undefined || height === undefined || dpr === undefined) return null;
  if (![width, height, dpr].every((entry) => Number.isFinite(entry) && entry > 0)) return null;
  return { width, height, dpr };
}

/** A page area that is not the audited one is another layout, so its sample and shot are worthless (Q-0012-05). */
export function viewportProblems(label: string, viewport: ViewportReading | null): string[] {
  if (!viewport) return [`${label}: the page returned no viewport reading (want {viewport:{width,height,dpr}})`];
  if (viewport.width !== CONTENT_WIDTH || viewport.height !== CONTENT_HEIGHT) {
    return [`${label}: the page area is ${viewport.width}x${viewport.height}, not the audited ${CONTENT_SIZE} (Q-0012-05)`];
  }
  return [];
}

export interface PixelSize {
  width: number;
  height: number;
}

/** The pixel size in a PNG's IHDR (PNG spec: 8-byte signature, 4-byte length, "IHDR", width, height). */
export function pngSize(bytes: Uint8Array): PixelSize | null {
  const magic = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];
  if (bytes.length < 24 || !magic.every((byte, index) => bytes[index] === byte)) return null;
  if (String.fromCharCode(...bytes.slice(12, 16)) !== "IHDR") return null;
  const header = new DataView(bytes.buffer, bytes.byteOffset + 16, 8);
  return { width: header.getUint32(0), height: header.getUint32(4) };
}

export interface FrameSummary {
  count: number;
  median: number;
  p95: number;
  longest: number;
}

export function summariseFrames(durations: readonly number[]): FrameSummary | null {
  if (durations.length === 0) return null;
  const sorted = [...durations].sort((a, b) => a - b);
  return {
    count: sorted.length,
    median: percentile(sorted, 0.5),
    p95: percentile(sorted, 0.95),
    longest: sorted[sorted.length - 1] as number,
  };
}

export interface LoafReading {
  supported: boolean;
  entries: number;
  totalMs: number;
  maxMs: number;
}

export function readLoaf(value: unknown): LoafReading | null {
  if (value === undefined || value === null) return null;
  const supported = field(value, "supported") === true;
  const entries = numberField(value, "entries") ?? 0;
  const totalMs = numberField(value, "totalMs") ?? 0;
  const maxMs = numberField(value, "maxMs") ?? 0;
  if (![entries, totalMs, maxMs].every((entry) => Number.isFinite(entry))) return null;
  return { supported, entries, totalMs, maxMs };
}

export interface LoafTotals {
  entries: number;
  totalMs: number;
  maxMs: number;
}

/** Null where the engine has no long-animation-frame entry type; the record's metric notes carry the reason. */
export function loafTotals(loaf: LoafReading | null): LoafTotals | null {
  if (!loaf?.supported) return null;
  return { entries: loaf.entries, totalMs: loaf.totalMs, maxMs: loaf.maxMs };
}

export interface LaunchReading {
  engine: string;
  readyAt: number;
  viewport: ViewportReading | null;
  guards: GuardReading | null;
  guardsFailed: boolean;
  metrics: Record<string, number>;
  frames: number[];
  pooled: FrameSummary | null;
  loaf: LoafReading | null;
}

export function readLaunch(value: unknown): { reading: LaunchReading | null; problems: string[] } {
  const problems: string[] = [];
  const engine = stringField(field(value, "engine"), "userAgent");
  if (!engine) problems.push("the page returned no navigator.userAgent");
  const readyAt = numberField(value, "readyAt");
  if (readyAt === undefined || !Number.isFinite(readyAt)) problems.push("the page returned no readyAt timestamp");
  const viewport = readViewport(value);
  if (!viewport) problems.push("the page returned no viewport reading (want {viewport:{width,height,dpr}})");
  const guards = readGuards(value);
  const failed = guardProblems(guards);
  if (problems.length > 0) return { reading: null, problems };
  if (failed.length > 0) {
    return {
      reading: {
        engine: engine as string,
        readyAt: readyAt as number,
        viewport,
        guards,
        guardsFailed: true,
        metrics: {},
        frames: [],
        pooled: null,
        loaf: readLoaf(field(value, "loaf")),
      },
      problems: [],
    };
  }
  const metrics: Record<string, number> = {};
  for (const name of PAGE_METRICS) {
    const seconds = numberField(field(value, "metrics"), name);
    if (seconds === undefined || !Number.isFinite(seconds) || seconds < 0) problems.push(`the page returned no ${name}`);
    else metrics[name] = seconds;
  }
  const durations = field(value, "durations");
  const frames: number[] = [];
  if (!Array.isArray(durations) || durations.length < MIN_FRAME_DURATIONS) {
    problems.push(`the page returned ${Array.isArray(durations) ? durations.length : "no"} frame duration(s), want at least ${MIN_FRAME_DURATIONS}`);
  } else {
    for (const duration of durations) {
      if (typeof duration !== "number" || !Number.isFinite(duration) || duration < 0) {
        problems.push("a frame duration is not a finite, non-negative number");
        break;
      }
      frames.push(duration);
    }
  }
  if (problems.length > 0) return { reading: null, problems };
  return {
    reading: {
      engine: engine as string,
      readyAt: readyAt as number,
      viewport,
      guards,
      guardsFailed: false,
      metrics,
      frames,
      pooled: summariseFrames(frames),
      loaf: readLoaf(field(value, "loaf")),
    },
    problems: [],
  };
}

export interface ShotGeometry {
  cssWidth: number;
  cssHeight: number;
  dpr: number;
  pxWidth: number;
  pxHeight: number;
}

export interface BuiltFile {
  file: string;
  bytes: number;
  sha256: string;
}

export interface ShotRecord extends BuiltFile {
  geometry: ShotGeometry;
}

export interface ShotRow {
  state: string;
  a: ShotRecord | null;
  b: ShotRecord | null;
  identical: boolean;
}

export interface ShotLaunchReading {
  engine: string;
  readyAt: number;
  viewport: ViewportReading | null;
  guards: GuardReading | null;
  guardsFailed: boolean;
  shots: "captured" | "unbound" | "failed" | string;
  file: string | null;
  error: string | null;
  loaf: LoafReading | null;
}

export function readShotLaunch(value: unknown): { reading: ShotLaunchReading | null; problems: string[] } {
  const problems: string[] = [];
  const engine = stringField(field(value, "engine"), "userAgent");
  if (!engine) problems.push("the page returned no navigator.userAgent");
  const readyAt = numberField(value, "readyAt");
  if (readyAt === undefined || !Number.isFinite(readyAt)) problems.push("the page returned no readyAt timestamp");
  const viewport = readViewport(value);
  if (!viewport) problems.push("the page returned no viewport reading (want {viewport:{width,height,dpr}})");
  const guards = readGuards(value);
  const failed = guardProblems(guards);
  const shots = stringField(value, "shots");
  if (failed.length === 0 && shots === undefined) problems.push("the page returned no shots field");
  if (problems.length > 0) return { reading: null, problems };
  return {
    reading: {
      engine: engine as string,
      readyAt: readyAt as number,
      viewport,
      guards,
      guardsFailed: failed.length > 0,
      shots: shots ?? "",
      file: stringField(value, "file") ?? null,
      error: stringField(value, "error") ?? null,
      loaf: readLoaf(field(value, "loaf")),
    },
    problems: [],
  };
}

export function sameGeometry(left: ShotRecord, right: ShotRecord): boolean {
  return (
    left.geometry.cssWidth === right.geometry.cssWidth &&
    left.geometry.cssHeight === right.geometry.cssHeight &&
    left.geometry.dpr === right.geometry.dpr &&
    left.geometry.pxWidth === right.geometry.pxWidth &&
    left.geometry.pxHeight === right.geometry.pxHeight
  );
}

const geometryText = (geometry: ShotGeometry): string => `${geometry.cssWidth}x${geometry.cssHeight} css at dpr ${geometry.dpr} (${geometry.pxWidth}x${geometry.pxHeight} px)`;

export function compareShots(a: Record<string, ShotRecord>, b: Record<string, ShotRecord>): { rows: ShotRow[]; problems: string[] } {
  const problems: string[] = [];
  for (const state of Object.keys(a)) if (!(SHOT_STATES as readonly string[]).includes(state)) problems.push(`state ${state} was captured but is not declared`);
  for (const state of Object.keys(b)) if (!(SHOT_STATES as readonly string[]).includes(state)) problems.push(`state ${state} was captured but is not declared`);
  const rows: ShotRow[] = SHOT_STATES.map((state) => {
    const left = a[state] ?? null;
    const right = b[state] ?? null;
    if (left && !right) problems.push(`state ${state} was captured from A only`);
    if (right && !left) problems.push(`state ${state} was captured from B only`);
    if (left && right && !sameGeometry(left, right)) {
      problems.push(`state ${state}: A and B were captured at different geometry (A ${geometryText(left.geometry)}, B ${geometryText(right.geometry)})`);
    }
    return { state, a: left, b: right, identical: !!left && !!right && left.sha256 === right.sha256 && left.bytes === right.bytes };
  });
  return { rows, problems };
}

export interface ShotTally {
  captured: number;
  unbound: number;
  failed: number;
}

export function shotStateOf(tally: ShotTally, states: number): "captured" | "unbound" | "failed" {
  if (tally.captured === states) return "captured";
  if (tally.captured === 0 && tally.unbound === states) return "unbound";
  return "failed";
}

export interface VisualAllowEntry {
  state: string;
  reason: string;
}

export function readVisualAllowlist(text: string): { entries: VisualAllowEntry[] } | { error: string } {
  let document: unknown;
  try {
    document = JSON.parse(text);
  } catch (error) {
    return { error: `${VISUAL_ALLOWLIST} is not JSON: ${error instanceof Error ? error.message : String(error)}` };
  }
  if (numberField(document, "version") !== 1) return { error: `${VISUAL_ALLOWLIST}: version must be 1` };
  const entries: VisualAllowEntry[] = [];
  const listed = field(document, "entries");
  if (!Array.isArray(listed)) return { error: `${VISUAL_ALLOWLIST}: entries must be a list` };
  for (const [index, entry] of listed.entries()) {
    const state = stringField(entry, "state")?.trim() ?? "";
    const reason = stringField(entry, "reason")?.trim() ?? "";
    if (!state || !reason) return { error: `${VISUAL_ALLOWLIST}: entry ${index} needs a state and a reason` };
    if (!(SHOT_STATES as readonly string[]).includes(state)) {
      return { error: `${VISUAL_ALLOWLIST}: entry ${index} names ${state}, which is not a captured state` };
    }
    entries.push({ state, reason });
  }
  return { entries };
}

export function visualFindings(rows: readonly ShotRow[], entries: readonly VisualAllowEntry[]): { findings: string[]; allowed: string[]; stale: string[] } {
  const listed = new Map(entries.map((entry) => [entry.state, entry.reason]));
  const findings: string[] = [];
  const allowed: string[] = [];
  const stale: string[] = [];
  for (const row of rows) {
    const reason = listed.get(row.state);
    if (row.identical && reason !== undefined) stale.push(`the allowlist entry for ${row.state} names no difference (${reason})`);
    else if (!row.identical && reason === undefined) findings.push(`state ${row.state} differs from the base and no allowlist entry names it`);
    else if (!row.identical) allowed.push(`state ${row.state} differs from the base and is allowed: ${reason}`);
  }
  return { findings, allowed, stale };
}

const HELPERS = `
const benchNow = () => performance.now();
const benchLoading = () => q('#stage [aria-busy="true"]');
// A measured interaction ends at the instant its condition first held, at the engine's own clock resolution: the shared until (desktop-page.ts PRELUDE) polls every 50 ms, so a metric taken through it lands on that poll's grid and a same-build pair reads a bucket flip as a real regression.
// Every condition these launches wait on is a selector query, so it can only flip on a DOM mutation; the deadline throws until's own message.
const benchWaitAt = (what, probe, ms = 15000) => {
  const deadline = benchNow() + ms;
  const first = probe() ? benchNow() : null;
  if (first !== null) return Promise.resolve(first);
  const { promise, resolve, reject } = Promise.withResolvers();
  const timeout = new Error("timed out waiting for " + what + situation());
  const observer = new MutationObserver(() => {
    const at = probe() ? benchNow() : null;
    if (at !== null) resolve(at);
    else if (benchNow() > deadline) reject(timeout);
  });
  observer.observe(document, { childList: true, subtree: true, attributes: true, characterData: true });
  const timer = setTimeout(() => reject(timeout), ms);
  return promise.finally(() => {
    observer.disconnect();
    clearTimeout(timer);
  });
};
const benchViewport = () => ({ width: innerWidth, height: innerHeight, dpr: devicePixelRatio });
const benchFrames = (frames) => {
  const durations = [];
  for (let i = 1; i < frames.length; i++) durations.push(frames[i] - frames[i - 1]);
  return durations;
};
// A live clock readout would differ between two runs of the same build, so it is pinned to a value the design's own shot mode uses.
const benchPinClocks = () => {
  for (const el of document.querySelectorAll("[data-clock]")) {
    if (el.dataset.clock === "since" || el.dataset.clock === "until") el.textContent = "0:00";
    el.removeAttribute("data-clock");
  }
};
// The answered-in chip and the reply's latency are this launch's own measurements (views/feedback.ts renders
// fb.elapsed_ms, views/coach.ts the reply's latency_ms), so two launches of one build render two values:
// each is pinned to the zero of its own format, like the clock views' initial text.
const benchPinMeasurements = () => {
  for (const [selector, text] of [['[data-testid="elapsed"] .num', "0:00"], ['[data-testid="coach-reply"] .rmeta .num', "0.0 s"]]) {
    const el = q(selector);
    if (el) el.textContent = text;
  }
};
// A shot launch takes no pointer input (every driver step is a synthetic click, key or focus) and the
// runner's cursor is wherever the OS left it: hit-testing goes inert so no capture shows a hovered element.
const benchInertPointer = () => {
  const style = document.createElement("style");
  style.textContent = "*,*::before,*::after{pointer-events:none!important}";
  document.head.append(style);
};
// Shot mode's own CSS rules (desktop/web/styles/motion.css:12): ?shot=1 would need a reload, and the smoke hook evaluates the script on the first load only.
const benchSettle = () => {
  const style = document.createElement("style");
  style.textContent = "*,*::before,*::after{animation:none!important;transition:none!important;caret-color:transparent!important}";
  document.head.append(style);
};
const benchCapture = async (state) => {
  benchSettle();
  benchPinClocks();
  benchPinMeasurements();
  await sleep(400);
  benchPinClocks();
  benchPinMeasurements();
  return await window.mygo.call("SmokeShot.Capture", state);
};
let benchLoaf = { supported: false, entries: 0, totalMs: 0, maxMs: 0 };
if (typeof PerformanceObserver === "function" && (PerformanceObserver.supportedEntryTypes || []).includes("long-animation-frame")) {
  benchLoaf = { supported: true, entries: 0, totalMs: 0, maxMs: 0 };
  const observer = new PerformanceObserver((list) => {
    for (const entry of list.getEntries()) {
      benchLoaf.entries++;
      benchLoaf.totalMs += entry.duration;
      if (entry.duration > benchLoaf.maxMs) benchLoaf.maxMs = entry.duration;
    }
  });
  observer.observe({ type: "long-animation-frame", buffered: true });
}
`;

/** The frame-cadence guard, carried by the launches that measure a shown window and never by the prepare launch (PREPARE_GUARDS_WHY). */
const GUARD_HELPERS = `
const benchRafGuard = async () => {
  let ticks = 0;
  const from = benchNow();
  const { promise, resolve } = Promise.withResolvers();
  const step = () => {
    ticks++;
    if (ticks >= ${RAF_TICKS} || benchNow() - from >= ${RAF_BUDGET_MS}) resolve();
    else requestAnimationFrame(step);
  };
  requestAnimationFrame(step);
  await promise;
  return { visibility: document.visibilityState, ticks, elapsedMs: benchNow() - from };
};
`;

/** The same script body drives a shot state in the bench and in the Linux render smoke. */
const DRIVERS: Record<ShotState, string> = {
  today: `await today();`,
  "sheet-open": `await today();
await click('[data-act="sheet"][data-v="pick"]');
await until("the practice sheet", () => q('#layer [aria-modal="true"]'));`,
  "practice-independent": `await startFromSheet("start-skill", "${SKILL}");
await sleep(250);`,
  "feedback-coach": `await startFromSheet("start-skill", "${SKILL}");
await answer("2");
await until("the coach call strip", () => q('[data-act="coach-send"]') && !q('[data-act="coach-send"]').disabled, 30000);
const budget = q('[data-testid="call-strip"] b.num')?.textContent ?? "";
q('[data-act="coach-send"]').click();
const reply = await until("the coach reply", () => q('[data-testid="coach-reply"]') || q('[data-testid="coach-failure"]'), 30000);
if (reply.matches('[data-testid="coach-reply"]')) await until("the spent budget slot", () => (q('[data-testid="call-strip"] b.num')?.textContent ?? "") !== budget, 30000);
await sleep(200);`,
  "settings-row": `await today();
location.hash = "#/settings";
const row = await until("the ${STUB_PROVIDER} provider row", () => q('[data-act="row"][data-v="${STUB_PROVIDER}"]'), 15000);
row.focus();
row.click();
await until("the expanded row", () => q(".rrow.open"), 10000);`,
  report: `await today();
location.hash = "#/report";
await until("the run report", () => q('[data-testid="report"]'), 15000);`,
};

/**
 * What every later launch copies: the scoring skill's guided walkthrough first (a skill practice is
 * refused before it, Q-0003-02), an accepted note, the coach's consent, and a finished assessment
 * run — the report state the desktop-ux gate audits shows a run that ended, and a fresh database
 * cannot open one (Q-0009-03 requires 20 qualifying unseen items).
 * It measures nothing, so its hidden window records no guard reading and waits on no animation
 * frame (PREPARE_GUARDS_WHY).
 */
export function prepareScript(): string {
  return pageScript(`
${HELPERS}
await today();
press("Enter");
await until("the recommended item", () => q('[data-testid="practice-item"]'));
await answer("1");
press("k");
await until("the card preview", () => q('[data-testid="card-preview"]'), 20000);
await click("#acceptbtn");
await until("the accepted note", () => q('[data-testid="note"]'), 20000);
await startFromSheet("start-skill", "${SKILL}");
await answer("2");
await until("the coach call strip", () => q('[data-act="coach-send"]') && !q('[data-act="coach-send"]').disabled, 30000);
const grant = q('[data-act="consent-grant"]');
if (grant) {
  grant.click();
  await until("the granted consent", () => !q('[data-act="consent-grant"]'), 20000);
}
await startFromSheet("start", "sample");
await answer("1");
location.hash = "#/assess";
await click('[data-act="run-ask"]');
await until("the run form", () => q('#runform input[name="size"]'));
q('#runform input[name="size"]').value = "3";
q('#runform input[name="minutes"]').value = "30";
await click('[data-act="run-start"]');
for (let i = 0; i < 3; i++) {
  const item = await until("assessment item " + (i + 1), () => q('[data-testid="assess-item"]'));
  const position = item.dataset.position;
  await sleep(150);
  press(String(1 + (i % 2)));
  await until("a selected option", () => q('.opt[aria-checked="true"]'));
  press("Enter");
  await until("the next item or the report", () => q('[data-testid="assess-item"]')?.dataset.position !== position || q('[data-testid="report"]'));
}
await until("the run report", () => q('[data-testid="report"]'));
return { prepared: true, granted: !!grant, engine: { userAgent: navigator.userAgent }, viewport: benchViewport(), loaf: benchLoaf };
`);
}

export function shotScript(state: ShotState): string {
  return pageScript(`
${HELPERS}${GUARD_HELPERS}
const engine = { userAgent: navigator.userAgent };
const readyAt = performance.timeOrigin + performance.now();
const viewport = benchViewport();
const guards = await benchRafGuard();
if (guards.visibility !== "visible" || guards.ticks < ${RAF_TICKS}) return { engine, readyAt, viewport, guards, loaf: benchLoaf };
if (!(window.mygo && typeof window.mygo.call === "function")) return { engine, readyAt, viewport, guards, shots: "no smoke runtime", loaf: benchLoaf };
benchInertPointer();
${DRIVERS[state]}
let file = "";
try {
  file = await benchCapture("${state}");
} catch (error) {
  const message = String((error && error.message) || error);
  return { engine, readyAt, viewport, guards, shots: message.includes("not bound") ? "unbound" : "failed", error: message, loaf: benchLoaf };
}
return { engine, readyAt, viewport, guards, shots: "captured", file, loaf: benchLoaf };
`);
}

export function timingScript(): string {
  return pageScript(`
${HELPERS}${GUARD_HELPERS}
const engine = { userAgent: navigator.userAgent };
const readyAt = performance.timeOrigin + performance.now();
const viewport = benchViewport();
const guards = await benchRafGuard();
if (guards.visibility !== "visible" || guards.ticks < ${RAF_TICKS}) return { engine, readyAt, viewport, guards };
const metrics = {};
await today();
const frames = [];
let sampling = true;
const sample = (t) => {
  frames.push(t);
  if (sampling) requestAnimationFrame(sample);
};
requestAnimationFrame(sample);
await click('[data-act="sheet"][data-v="pick"]');
const sheet = await until("the practice sheet", () => q('#layer [aria-modal="true"]'));
const sheetBody = sheet.querySelector(".sh-b");
for (let i = 0; i <= 9; i++) {
  if (sheetBody) sheetBody.scrollTop = (sheetBody.scrollHeight * i) / 9;
  await sleep(40);
}
press("Escape");
await until("the closed practice sheet", () => !q('#layer [aria-modal="true"]'));
await startFromSheet("start-skill", "${SKILL}");
await sleep(200);
const card = q(".q-scroll");
for (let i = 0; i <= 9; i++) {
  if (card) card.scrollTop = (card.scrollHeight * i) / 9;
  await sleep(40);
}
sampling = false;
{
  const at = benchNow();
  press("2");
  metrics.answerSelectMs = (await benchWaitAt("a selected option", () => q('.opt[aria-checked="true"]'))) - at;
}
{
  const at = benchNow();
  press("Enter");
  metrics.submitFeedbackMs = (await benchWaitAt("the feedback card", () => q('[data-testid="feedback"]'))) - at;
}
await until("the coach call strip", () => q('[data-act="coach-send"]') && !q('[data-act="coach-send"]').disabled, 30000);
{
  const at = benchNow();
  q('[data-act="coach-send"]').click();
  metrics.coachReplyMs = (await benchWaitAt("the coach reply", () => q('[data-testid="coach-reply"]') || q('[data-testid="coach-failure"]'), 30000)) - at;
}
const rooms = [
  ["report", () => q('[data-testid="report"]')],
  ["anki", () => q('[data-testid="note"]')],
  ["settings", () => q('[data-act="row"]')],
  ["assess", () => q('[data-act="run-ask"]')],
];
for (const [room, alive] of rooms) {
  const at = benchNow();
  location.hash = "#/" + room;
  metrics["roomSwitch." + room] = (await benchWaitAt("the " + room + " room", () => (benchLoading() ? null : alive()), 15000)) - at;
}
return { engine, readyAt, viewport, guards, metrics, durations: benchFrames(frames), loaf: benchLoaf };
`);
}

export interface SelfCheckCase {
  name: string;
  /** Null when the function under test behaved as required; otherwise what it did instead. */
  check: () => string | null;
}

const expect = (ok: boolean, message: string): string | null => (ok ? null : message);

export function selfCheckCases(): SelfCheckCase[] {
  const options = (argv: string[]): BenchOptions => parseBenchArgs(argv);
  const gradedMetrics = (): Record<string, number> => {
    const metrics: Record<string, number> = {};
    for (const name of PAGE_METRICS) metrics[name] = 100;
    return metrics;
  };
  const auditedViewport = { width: CONTENT_WIDTH, height: CONTENT_HEIGHT, dpr: 1 };
  const shotRecord = (file: string, bytes: number, sha256: string, dpr = 1.25): ShotRecord => ({
    file,
    bytes,
    sha256,
    geometry: { cssWidth: CONTENT_WIDTH, cssHeight: CONTENT_HEIGHT, dpr, pxWidth: CONTENT_WIDTH * dpr, pxHeight: CONTENT_HEIGHT * dpr },
  });
  return [
    {
      name: "args: compare defaults",
      check: () => {
        const parsed = options(["--a-root", "a", "--b-root", ".", "--out", "run/record.json"]);
        const explicit = options(["--a-root", "a", "--b-root", ".", "--out", "run/record.json", "--shots", "pics"]);
        return expect(
          parsed.mode === "compare" &&
            parsed.aRoot === "a" &&
            parsed.bRoot === "." &&
            parsed.samples === DEFAULT_SAMPLES &&
            parsed.warmup === DEFAULT_WARMUP &&
            parsed.out === path.resolve("run", "record.json") &&
            parsed.shots === path.resolve("run", "shots") &&
            explicit.shots === path.resolve("pics"),
          `parsed ${JSON.stringify(parsed)}, explicit ${JSON.stringify(explicit)}`,
        );
      },
    },
    {
      name: "args must fail: one tree without the other",
      check: () => {
        const parsed = options(["--a-root", "a", "--out", "record.json"]);
        return expect(parsed.mode === "error" && parsed.error.includes("--b-root"), `parsed ${JSON.stringify(parsed)}`);
      },
    },
    {
      name: "args must fail: --samples 0",
      check: () => {
        const parsed = options(["--a-root", "a", "--b-root", ".", "--out", "r.json", "--samples", "0"]);
        return expect(parsed.mode === "error" && parsed.error.includes("--samples"), `parsed ${JSON.stringify(parsed)}`);
      },
    },
    {
      name: "args: render smoke",
      check: () => {
        const parsed = options(["--root", ".", "--smoke", "--out", "smoke.json"]);
        return expect(
          parsed.mode === "smoke" && parsed.root === "." && parsed.out === path.resolve("smoke.json") && parsed.shots === path.resolve("shots"),
          `parsed ${JSON.stringify(parsed)}`,
        );
      },
    },
    {
      name: "args must fail: a smoke mixed with two trees",
      check: () => {
        const parsed = options(["--root", ".", "--smoke", "--a-root", "a", "--b-root", ".", "--out", "s.json"]);
        return expect(parsed.mode === "error", `parsed ${JSON.stringify(parsed)}`);
      },
    },
    {
      name: "args must fail: an unknown argument",
      check: () => {
        const parsed = options(["--depth", "2"]);
        return expect(parsed.mode === "error" && parsed.error.includes("--depth"), `parsed ${JSON.stringify(parsed)}`);
      },
    },
    {
      name: "spread: quartiles of an odd sample",
      check: () => {
        const got = spread([5, 1, 3]);
        return expect(got?.n === 3 && got.min === 1 && got.q1 === 2 && got.median === 3 && got.q3 === 4 && got.max === 5, `spread = ${JSON.stringify(got)}`);
      },
    },
    {
      name: "spread must fail: an empty sample is no spread",
      check: () => expect(spread([]) === null, `spread([]) = ${JSON.stringify(spread([]))}`),
    },
    {
      name: "verdict must fail: a shifted build is worse",
      check: () => {
        const base = spread(Array.from({ length: 9 }, (_, i) => 100 + i));
        const head = spread(Array.from({ length: 9 }, (_, i) => 140 + i));
        const verdict = worseBeyondNoise(base, head);
        return expect(verdict.worse, `verdict = ${JSON.stringify(verdict)}`);
      },
    },
    {
      name: "verdict: an identical sample is not worse",
      check: () => {
        const sample = [10, 11, 12, 13, 14, 15, 16, 17, 18];
        const verdict = worseBeyondNoise(spread(sample), spread(sample));
        return expect(!verdict.worse, `verdict = ${JSON.stringify(verdict)}`);
      },
    },
    {
      name: "verdict: a gap inside the floor is not worse",
      check: () => {
        const base = spread(Array.from({ length: 9 }, () => 100));
        const head = spread(Array.from({ length: 9 }, () => 101));
        const verdict = worseBeyondNoise(base, head);
        return expect(!verdict.worse, `verdict = ${JSON.stringify(verdict)}`);
      },
    },
    {
      name: "verdict: quartiles that overlap are not worse",
      check: () => {
        const base = spread([100, 110, 120, 130, 140, 150, 160, 170, 180]);
        const head = spread([110, 120, 130, 140, 150, 160, 170, 180, 190]);
        const verdict = worseBeyondNoise(base, head);
        return expect(!verdict.worse, `verdict = ${JSON.stringify(verdict)}`);
      },
    },
    {
      name: "verdict must fail: a 5% gap with Q1 above Q3 alone is worse",
      check: () => {
        const base = spread(Array.from({ length: 9 }, (_, i) => 1000 + i));
        const head = spread(Array.from({ length: 9 }, (_, i) => 1100 + i));
        const verdict = worseBeyondNoise(base, head);
        return expect(verdict.worse && verdict.detail.includes("beyond"), `verdict = ${JSON.stringify(verdict)}`);
      },
    },
    {
      name: "verdict: the control IQR sets the floor and turns a WORSE into ok",
      check: () => {
        const base = spread([10, 20, 30, 40, 50]);
        const head = spread([60, 70, 80, 90, 100]);
        const controls = [spread([0, 10, 20, 30, 40]), spread([0, 10, 90, 100, 110])];
        const fixed = worseBeyondNoise(base, head);
        const measured = worseBeyondNoise(base, head, controls);
        return expect(
          fixed.worse && fixed.floor === 5 && fixed.floorSource === "5ms" && !measured.worse && measured.floor === 90 && measured.floorSource === "control IQR",
          `fixed ${JSON.stringify(fixed)}, measured ${JSON.stringify(measured)}`,
        );
      },
    },
    {
      name: "verdict must fail: a gap beyond the control IQR is still worse",
      check: () => {
        const base = spread([10, 20, 30, 40, 50]);
        const head = spread([110, 120, 130, 140, 150]);
        const controls = [spread([0, 10, 20, 30, 40]), spread([0, 10, 90, 100, 110])];
        const verdict = worseBeyondNoise(base, head, controls);
        return expect(verdict.worse && verdict.floor === 90 && verdict.floorSource === "control IQR", `verdict = ${JSON.stringify(verdict)}`);
      },
    },
    {
      name: "verdict: a metric without control samples keeps max(5 ms, 5%)",
      check: () => {
        const bytes = spread([186489]);
        const large = worseBeyondNoise(bytes, bytes, [null, null]);
        const short = worseBeyondNoise(spread([40, 50, 60]), spread([40, 50, 60]));
        return expect(
          large.floor === 186489 * MEDIAN_GAP_FRACTION &&
            large.floorSource === "5%" &&
            short.floor === MEDIAN_GAP_FLOOR_MS &&
            short.floorSource === "5ms",
          `large ${JSON.stringify(large)}, short ${JSON.stringify(short)}`,
        );
      },
    },
    {
      name: "verdict: the record and log floor text names the rounded floor and the term that set it",
      check: () => {
        const measured = worseBeyondNoise(spread([10, 20, 30, 40, 50]), spread([60, 70, 80, 90, 100]), [spread([0, 10, 90, 100, 110])]);
        const none = worseBeyondNoise(null, spread([1]));
        return expect(
          floorText(measured) === "floor 90 (control IQR)" && floorText(none) === "floor none",
          `measured ${floorText(measured)}, none ${floorText(none)}`,
        );
      },
    },
    {
      name: "metric rows: the A/B verdict reads the control pair, the control pair's own verdict keeps the fixed floor",
      check: () => {
        const rows = metricRows({
          a: { m: [10, 20, 30, 40, 50] },
          b: { m: [60, 70, 80, 90, 100] },
          controlA: { m: [0, 10, 20, 30, 40] },
          controlB: { m: [0, 10, 90, 100, 110] },
        });
        const row = rows[0];
        return expect(
          rows.length === 1 &&
            row !== undefined &&
            !row.verdict.worse &&
            row.verdict.floor === 90 &&
            row.verdict.floorSource === "control IQR" &&
            row.controlVerdict.floor === 5 &&
            row.controlVerdict.floorSource === "5ms",
          `rows = ${JSON.stringify(rows)}`,
        );
      },
    },
    {
      name: "guards must fail: a hidden page",
      check: () => {
        const problems = guardProblems({ visibility: "hidden", ticks: 30, elapsedMs: 500 });
        return expect(problems.length === 1 && problems[0]?.includes("visibilityState"), `problems = ${JSON.stringify(problems)}`);
      },
    },
    {
      name: "guards must fail: a throttled page",
      check: () => {
        const problems = guardProblems({ visibility: "visible", ticks: 1, elapsedMs: 750 });
        return expect(problems.length === 1 && (problems[0] ?? "").includes("requestAnimationFrame"), `problems = ${JSON.stringify(problems)}`);
      },
    },
    {
      name: "guards must fail: no guard reading",
      check: () => expect(guardProblems(null).length === 1, `problems = ${JSON.stringify(guardProblems(null))}`),
    },
    {
      name: "guards: a visible page at 30 ticks passes",
      check: () => expect(guardProblems({ visibility: "visible", ticks: 30, elapsedMs: 502 }).length === 0, `problems = ${JSON.stringify(guardProblems({ visibility: "visible", ticks: 30, elapsedMs: 502 }))}`),
    },
    {
      name: "guards must fail: 30 ticks that took longer than the budget",
      check: () => {
        const problems = guardProblems({ visibility: "visible", ticks: 31, elapsedMs: RAF_BUDGET_MS + 500 });
        return expect(problems.length === 1 && (problems[0] ?? "").includes("at most"), `problems = ${JSON.stringify(problems)}`);
      },
    },
    {
      name: "launch: a guard-failed launch parses without samples",
      check: () => {
        const { reading, problems } = readLaunch({ engine: { userAgent: "ua" }, readyAt: 1, viewport: auditedViewport, guards: { visibility: "hidden", ticks: 0, elapsedMs: 750 } });
        return expect(problems.length === 0 && reading?.guardsFailed === true && reading.metrics.readyMs === undefined, `reading ${JSON.stringify(reading)}, problems ${JSON.stringify(problems)}`);
      },
    },
    {
      name: "launch must fail: a graded launch without a viewport",
      check: () => {
        const { reading, problems } = readLaunch({ engine: { userAgent: "ua" }, readyAt: 1, guards: { visibility: "visible", ticks: 30, elapsedMs: 500 } });
        return expect(reading === null && problems.some((problem) => problem.includes("viewport")), `reading ${JSON.stringify(reading)}, problems ${JSON.stringify(problems)}`);
      },
    },
    {
      name: "launch must fail: a graded launch without one metric",
      check: () => {
        const metrics = gradedMetrics();
        delete metrics["coachReplyMs"];
        const { reading, problems } = readLaunch({
          engine: { userAgent: "ua" },
          readyAt: 1,
          viewport: auditedViewport,
          guards: { visibility: "visible", ticks: 30, elapsedMs: 500 },
          metrics,
          durations: Array.from({ length: MIN_FRAME_DURATIONS }, () => 16.7),
        });
        return expect(reading === null && problems.some((problem) => problem.includes("coachReplyMs")), `reading ${JSON.stringify(reading)}, problems ${JSON.stringify(problems)}`);
      },
    },
    {
      name: "launch must fail: a graded launch without frames",
      check: () => {
        const { reading, problems } = readLaunch({
          engine: { userAgent: "ua" },
          readyAt: 1,
          viewport: auditedViewport,
          guards: { visibility: "visible", ticks: 30, elapsedMs: 500 },
          metrics: gradedMetrics(),
          durations: [],
        });
        return expect(reading === null && problems.some((problem) => problem.includes("frame duration")), `reading ${JSON.stringify(reading)}, problems ${JSON.stringify(problems)}`);
      },
    },
    {
      name: "launch: a graded launch parses into a sample",
      check: () => {
        const { reading, problems } = readLaunch({
          engine: { userAgent: "ua" },
          readyAt: 1,
          viewport: { width: CONTENT_WIDTH, height: CONTENT_HEIGHT, dpr: 1.25 },
          guards: { visibility: "visible", ticks: 30, elapsedMs: 500 },
          metrics: gradedMetrics(),
          durations: [16, 17, 16, 18, 16, 33, 16, 17, 16, 17, 16, 17, 16, 17, 16, 17, 16, 17, 16, 17],
          loaf: { supported: false, entries: 0, totalMs: 0, maxMs: 0 },
        });
        return expect(
          problems.length === 0 &&
            reading?.metrics["answerSelectMs"] === 100 &&
            reading.pooled?.count === 20 &&
            reading.pooled.longest === 33 &&
            reading.viewport?.dpr === 1.25 &&
            reading.loaf?.supported === false,
          `reading ${JSON.stringify(reading)}, problems ${JSON.stringify(problems)}`,
        );
      },
    },
    {
      name: "shot launch: a capture parses",
      check: () => {
        const { reading, problems } = readShotLaunch({
          engine: { userAgent: "ua" },
          readyAt: 2,
          viewport: auditedViewport,
          guards: { visibility: "visible", ticks: 31, elapsedMs: 480 },
          shots: "captured",
          file: "today.png",
          loaf: { supported: true, entries: 2, totalMs: 40, maxMs: 25 },
        });
        return expect(
          problems.length === 0 &&
            reading?.shots === "captured" &&
            reading.file === "today.png" &&
            reading.viewport?.width === CONTENT_WIDTH &&
            reading.loaf?.entries === 2 &&
            !reading.guardsFailed,
          `reading ${JSON.stringify(reading)}, problems ${JSON.stringify(problems)}`,
        );
      },
    },
    {
      name: "shot launch must fail: a capture with no result field",
      check: () => {
        const { reading, problems } = readShotLaunch({
          engine: { userAgent: "ua" },
          readyAt: 2,
          viewport: auditedViewport,
          guards: { visibility: "visible", ticks: 31, elapsedMs: 480 },
        });
        return expect(reading === null && problems.some((problem) => problem.includes("shots")), `reading ${JSON.stringify(reading)}, problems ${JSON.stringify(problems)}`);
      },
    },
    {
      name: "shot launch: a guard-failed shot parses without a result field",
      check: () => {
        const { reading, problems } = readShotLaunch({
          engine: { userAgent: "ua" },
          readyAt: 2,
          viewport: auditedViewport,
          guards: { visibility: "hidden", ticks: 1, elapsedMs: 750 },
        });
        return expect(problems.length === 0 && reading?.guardsFailed === true, `reading ${JSON.stringify(reading)}, problems ${JSON.stringify(problems)}`);
      },
    },
    {
      name: "shots must fail: differing bytes",
      check: () => {
        const a = { today: shotRecord("today.png", 10, "aa") };
        const b = { today: shotRecord("today.png", 10, "bb") };
        const { rows } = compareShots(a, b);
        return expect(rows.length === SHOT_STATES.length && rows[0]?.state === "today" && rows[0]?.identical === false, `rows = ${JSON.stringify(rows)}`);
      },
    },
    {
      name: "shots must fail: a state only one side captured",
      check: () => {
        const a = { today: shotRecord("today.png", 10, "aa") };
        const { problems } = compareShots(a, {});
        return expect(problems.some((problem) => problem.includes("from A only")), `problems = ${JSON.stringify(problems)}`);
      },
    },
    {
      name: "shots: equal bytes of both sides are identical",
      check: () => {
        const shots = { today: shotRecord("today.png", 10, "aa"), "sheet-open": shotRecord("sheet-open.png", 11, "ab") };
        const { rows, problems } = compareShots(shots, shots);
        return expect(problems.length === 0 && rows.filter((row) => row.identical).length === 2, `rows = ${JSON.stringify(rows)}, problems ${JSON.stringify(problems)}`);
      },
    },
    {
      name: "shots must fail: a geometry difference between A and B",
      check: () => {
        const a = { today: shotRecord("today.png", 10, "aa") };
        const b = { today: shotRecord("today.png", 10, "bb", 1) };
        const { problems } = compareShots(a, b);
        return expect(problems.some((problem) => problem.includes("geometry") && problem.includes("dpr 1.25")), `problems = ${JSON.stringify(problems)}`);
      },
    },
    {
      name: "viewport: the audited page area passes",
      check: () => expect(viewportProblems("shot-a-today", { width: CONTENT_WIDTH, height: CONTENT_HEIGHT, dpr: 1.25 }).length === 0, `problems = ${JSON.stringify(viewportProblems("shot-a-today", { width: CONTENT_WIDTH, height: CONTENT_HEIGHT, dpr: 1.25 }))}`),
    },
    {
      name: "viewport must fail: a page area that is not the audited size",
      check: () => {
        const problems = viewportProblems("shot-a-today", { width: 1024, height: 768, dpr: 1 });
        return expect(problems.length === 1 && (problems[0] ?? "").includes("1024x768") && (problems[0] ?? "").includes(CONTENT_SIZE), `problems = ${JSON.stringify(problems)}`);
      },
    },
    {
      name: "viewport must fail: the width right and the height short",
      check: () => {
        // 1280x780: the clamp a 1280x800 display produces (20 CSS px to the title bar); only the height half of the check rejects it.
        const problems = viewportProblems("shot-a-today", { width: CONTENT_WIDTH, height: 780, dpr: 1.5 });
        return expect(problems.length === 1 && (problems[0] ?? "").includes("1280x780") && (problems[0] ?? "").includes(CONTENT_SIZE), `problems = ${JSON.stringify(problems)}`);
      },
    },
    {
      name: "image: the PNG header carries the pixel size",
      check: () => {
        const bytes = new Uint8Array(24);
        bytes.set([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
        bytes.set([...new TextEncoder().encode("IHDR")], 12);
        const header = new DataView(bytes.buffer);
        header.setUint32(16, 1600);
        header.setUint32(20, 1000);
        const size = pngSize(bytes);
        return expect(size?.width === 1600 && size.height === 1000, `pngSize = ${JSON.stringify(size)}`);
      },
    },
    {
      name: "image must fail: a truncated PNG header",
      check: () => expect(pngSize(new Uint8Array([0x89, 0x50, 0x4e, 0x47])) === null, `pngSize = ${JSON.stringify(pngSize(new Uint8Array([0x89, 0x50, 0x4e, 0x47])))}`),
    },
    {
      name: "loaf: WebView2 totals are carried, an engine without the entry type records null",
      check: () => {
        const supported = loafTotals({ supported: true, entries: 3, totalMs: 120, maxMs: 60 });
        const absent = loafTotals({ supported: false, entries: 0, totalMs: 0, maxMs: 0 });
        return expect(
          supported?.entries === 3 && supported.totalMs === 120 && supported.maxMs === 60 && absent === null,
          `supported ${JSON.stringify(supported)}, absent ${JSON.stringify(absent)}`,
        );
      },
    },
    {
      name: "template: one seed, from the base tree when the two differ",
      check: () => {
        const separate = templatePlan(false);
        const together = templatePlan(true);
        return expect(
          separate.seedKey === "a" && separate.reason.includes("base") && together.seedKey === "b" && together.reason.includes("same commit"),
          `separate ${JSON.stringify(separate)}, together ${JSON.stringify(together)}`,
        );
      },
    },
    {
      name: "seeds: one seeding is a run, two are a finding",
      check: () => {
        const one = seedProblems(["a"]);
        const two = seedProblems(["a", "b"]);
        return expect(one.length === 0 && two.length === 1 && two[0]?.includes("2 tree(s) seeded"), `one ${JSON.stringify(one)}, two ${JSON.stringify(two)}`);
      },
    },
    {
      name: "record shape must fail: a measured launch that requested the hidden window",
      check: () => {
        const plan = { prepare: 1, warmup: 0, measured: 1, control: 0, shots: 0, controlShots: 0, total: 2 };
        const prepareRow = { label: "prepare-a", kind: "prepare", hidden: true, guards: null, guardsWhy: PREPARE_GUARDS_WHY };
        const sampleRow = { label: "sample-b-0", kind: "sample", hidden: false, guards: { visibility: "visible", ticks: 30, elapsedMs: 500 }, guardsWhy: "" };
        const prepareHidden = recordShapeProblems({ seeds: ["a"], rows: [prepareRow], launches: 1, plan: { ...plan, measured: 0, total: 1 } });
        const sampleShown = recordShapeProblems({ seeds: ["a"], rows: [sampleRow], launches: 1, plan: { ...plan, prepare: 0, total: 1 } });
        const sampleHidden = recordShapeProblems({
          seeds: ["a"],
          rows: [prepareRow, { ...sampleRow, hidden: true }],
          launches: 2,
          plan,
        });
        return expect(
          prepareHidden.length === 0 && sampleShown.length === 0 && sampleHidden.length === 1 && (sampleHidden[0] ?? "").includes("sample-b-0"),
          `prepareHidden ${JSON.stringify(prepareHidden)}, sampleShown ${JSON.stringify(sampleShown)}, sampleHidden ${JSON.stringify(sampleHidden)}`,
        );
      },
    },
    {
      name: "record shape must fail: the prepare launch carries a guard reading, or none without a reason",
      check: () => {
        const plan = { prepare: 1, warmup: 0, measured: 1, control: 0, shots: 0, controlShots: 0, total: 2 };
        const silent = recordShapeProblems({ seeds: ["a"], rows: [{ label: "prepare-a", kind: "prepare", hidden: true, guards: null, guardsWhy: "" }], launches: 1, plan: { ...plan, measured: 0, total: 1 } });
        const fake = recordShapeProblems({
          seeds: ["a"],
          rows: [{ label: "prepare-a", kind: "prepare", hidden: true, guards: { visibility: "visible", ticks: 30, elapsedMs: 500 }, guardsWhy: "" }],
          launches: 1,
          plan: { ...plan, measured: 0, total: 1 },
        });
        return expect(
          silent.length === 1 && (silent[0] ?? "").includes("guardsWhy") && fake.length === 1 && (fake[0] ?? "").includes("carries a guard reading"),
          `silent ${JSON.stringify(silent)}, fake ${JSON.stringify(fake)}`,
        );
      },
    },
    {
      name: "prepare: the script must not wait for animation frames",
      check: () => {
        const script = prepareScript();
        const offenders = ["requestAnimationFrame", "benchRafGuard"].filter((token) => script.includes(token));
        return expect(offenders.length === 0, `prepareScript carries ${offenders.join(" and ")}: its window is hidden, a hidden page draws no frames on macOS and Linux, and a wait there never finishes`);
      },
    },
    {
      name: "shot: the feedback-coach script settles the pointer, the readouts and the send's re-read",
      check: () => {
        const script = shotScript("feedback-coach");
        const missing = ["benchInertPointer()", "benchPinMeasurements()", '[data-testid="elapsed"] .num', '[data-testid="coach-reply"] .rmeta .num', "the spent budget slot"].filter((token) => !script.includes(token));
        const inertAt = script.indexOf("benchInertPointer();");
        const driverAt = script.indexOf('await startFromSheet("start-skill"');
        return expect(
          missing.length === 0 && inertAt !== -1 && driverAt !== -1 && inertAt < driverAt,
          `the script is missing ${JSON.stringify(missing)} (inert hit-testing at ${inertAt}, the driver at ${driverAt}): a capture that does not settle these shows the runner's cursor, this launch's own measurements and a budget slot the send had not spent yet`,
        );
      },
    },
    {
      name: "timing: a measured interaction ends on the mutation that made it true, never on the 50 ms until poll",
      check: () => {
        const script = timingScript();
        const blocks = [...script.matchAll(/const at = benchNow\(\);[\s\S]*?metrics(?:\.\w+|\[[^\]]+\]) = [^;]+;/g)].map((match) => match[0]);
        const offenders = blocks.filter((block) => !block.includes("await benchWaitAt(") || block.includes("until("));
        return expect(
          blocks.length === 4 && offenders.length === 0 && script.includes("new MutationObserver"),
          `4 measured blocks (the three single metrics and the room loop) want the mutation-driven waiter; ${blocks.length} block(s), offenders ${JSON.stringify(offenders)}: a metric taken through the shared 50 ms until poll lands on that poll's grid, so a same-build pair reads a bucket flip as a regression`,
        );
      },
    },
    {
      name: "launches: the plan's total is the sum of its parts",
      check: () => {
        const separate = launchPlan({ samples: 3, warmup: 1, sameBuild: false });
        const together = launchPlan({ samples: 3, warmup: 1, sameBuild: true });
        const smoke = smokeLaunchPlan();
        const sums = [separate, together, smoke].map((plan) => plan.prepare + plan.warmup + plan.measured + plan.control + plan.shots + plan.controlShots);
        return expect(
          separate.total === 39 && together.total === 21 && smoke.total === 7 && sums.every((sum, index) => sum === [separate, together, smoke][index]?.total),
          `separate ${JSON.stringify(separate)}, together ${JSON.stringify(together)}, smoke ${JSON.stringify(smoke)}`,
        );
      },
    },
    {
      name: "shot tally: every state captured, or the hook absent, or a failure",
      check: () => {
        const captured = shotStateOf({ captured: 6, unbound: 0, failed: 0 }, 6);
        const unbound = shotStateOf({ captured: 0, unbound: 6, failed: 0 }, 6);
        const failed = shotStateOf({ captured: 5, unbound: 0, failed: 1 }, 6);
        return expect(captured === "captured" && unbound === "unbound" && failed === "failed", `captured ${captured}, unbound ${unbound}, failed ${failed}`);
      },
    },
    {
      name: "shot tally must fail: one unbound launch beside five captures is a failure",
      check: () => expect(shotStateOf({ captured: 5, unbound: 1, failed: 0 }, 6) === "failed", `state = ${shotStateOf({ captured: 5, unbound: 1, failed: 0 }, 6)}`),
    },
    {
      name: "allowlist: a differing state is allowed, an identical one is stale",
      check: () => {
        const rows = [
          { state: "today", a: null, b: null, identical: false },
          { state: "report", a: null, b: null, identical: true },
        ];
        const { findings, allowed, stale } = visualFindings(rows, [{ state: "today", reason: "the step changes it" }, { state: "report", reason: "stale" }]);
        return expect(findings.length === 0 && allowed.length === 1 && stale.length === 1, `findings ${JSON.stringify(findings)}, allowed ${JSON.stringify(allowed)}, stale ${JSON.stringify(stale)}`);
      },
    },
    {
      name: "allowlist must fail: a differing state nobody names",
      check: () => {
        const rows = [{ state: "today", a: null, b: null, identical: false }];
        const { findings } = visualFindings(rows, []);
        return expect(findings.length === 1 && (findings[0] ?? "").includes("today"), `findings = ${JSON.stringify(findings)}`);
      },
    },
    {
      name: "allowlist must fail: a malformed document",
      check: () => {
        const read = readVisualAllowlist(JSON.stringify({ version: 1, entries: [{ state: "nope", reason: "x" }] }));
        return expect("error" in read && read.error.includes("nope"), `read = ${JSON.stringify(read)}`);
      },
    },
  ];
}

