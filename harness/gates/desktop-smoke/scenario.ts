/**
 * What every desktop-smoke scenario shares: the run it is handed, launching the window on its own data
 * directory, the second process (`passquest ... --db`) and the attempt cross-check.
 */
import { cliJson, launchPage, SKILL, STUB_MODEL } from "../../lib/desktop-page.ts";
import type { Check, Launched, ProviderStub } from "../../lib/desktop-page.ts";
import { arrayField, field, numberField, stringField } from "../../lib/json.ts";
import { tail } from "../../lib/process.ts";
import type { RunResult } from "../../lib/process.ts";
import { resolve } from "../../lib/repo.ts";
import type { GateContext } from "../../lib/types.ts";

/** Nothing listens on port 1: the "Anki is not running" endpoint. */
export const UNREACHABLE_ENDPOINT = "http://127.0.0.1:1";

/** Markup a model could return; it must reach the learner as text, never as elements. */
export const HOSTILE_MARKUP = '<b>x</b><img src=x onerror="document.title=\'pwned\'">';

export interface Shared {
  ctx: GateContext;
  binary: string;
  /** The CLI, built once the window boots. */
  cli: string;
  registry: string;
  provider: ProviderStub;
  /**
   * The directory every launch runs in. The desktop smoke keeps the
   * repository root, so the app finds `agent/*.mjs` and runs them under Node
   * the way the CLI smokes do; the package smoke runs outside the checkout,
   * where only the installed copy is reachable.
   */
  cwd?: string;
  /**
   * Environment every launch gets beyond the smoke hook's own variables and
   * the scenario's. The package smoke strips PATH and points HOME/AppData/
   * XDG_CONFIG_HOME at a temp tree here. Every value is a string: unsetting a
   * variable is not supported, so a launch that must not see one leaves the
   * key out of this record rather than giving it an `undefined` value.
   */
  launchEnv?: Record<string, string>;
  /**
   * Set false to leave PASSQUEST_DESKTOP_DATA unset, so the app resolves its
   * own per-user data directory (the package smoke, whose data root is what
   * the default path code produces); the desktop smoke points it at the
   * scenario's own directory.
   */
  dataOverride?: boolean;
  /**
   * Extra trees the settings scenario searches for the credential it set. The
   * package smoke passes the installed tree, which no credential may reach.
   */
  secretScanRoots?: readonly string[];
  /**
   * The Node runtime and the durable host an installed copy carries. A
   * scenario that runs the installed binary as its own CLI pins them through
   * PASSQUEST_NODE and PASSQUEST_AGENT_HOST: the CLI composition root
   * resolves no resources directory (only the window does, see
   * docs/harness.md), so unlike the window it cannot find packaged files on
   * its own. Undefined in the source tree, where both resolve normally.
   */
  runtime?: string;
  durableHost?: string;
}

export interface ScenarioRun {
  shared: Shared;
  id: string;
  dir: string;
  dataDir: string;
  database: string;
  /** Credential-store service of this scenario's launches. */
  service: string;
  check: Check;
  /**
   * A line for the gate's pass detail: what this scenario read that the
   * artifact must carry — the boot and titlebar scenarios record the room a
   * hidden window reads (Q-0022-01).
   */
  detail?: (line: string) => void;
}

export interface Scenario {
  id: string;
  run: (s: ScenarioRun) => Promise<void>;
}

export interface LaunchSpec {
  label: string;
  /** Script body; `pageScript` adds the shared helpers. */
  script: string;
  env?: Record<string, string>;
  /** Probes listening TCP sockets of the app and its children while it runs. */
  probeListeners?: boolean;
  /** Polled while the app runs; true kills the process tree with the watchdog's kill. */
  killWhen?: () => Promise<boolean>;
}

/** Milliseconds since the epoch; the window and the CLI print different sub-millisecond digits. */
export function instant(text: string | undefined): number {
  return text === undefined ? Number.NaN : Date.parse(text.replace(/(\.\d{3})\d+/, "$1"));
}

/** Launches the window once with a script and returns the script's value, or null after a failed check. */
export function launch(s: ScenarioRun, spec: LaunchSpec): Promise<Launched | null> {
  return launchPage({
    binary: s.shared.binary,
    // The repository root by default, so the app finds agent/*.mjs and runs
    // them under Node the way the CLI smokes do; the package smoke runs in a
    // temp directory, where only the installed copy is reachable.
    cwd: s.shared.cwd ?? s.shared.ctx.root,
    dir: s.dir,
    label: spec.label,
    header: `desktop-smoke: ${s.id}/${spec.label}`,
    where: spec.label === s.id ? s.id : `${s.id} (${spec.label})`,
    script: spec.script,
    env: {
      ...(s.shared.dataOverride === false ? {} : { PASSQUEST_DESKTOP_DATA: s.dataDir }),
      PASSQUEST_DESKTOP_SECRET_SERVICE: s.service,
      PASSQUEST_PROVIDER_REGISTRY: s.shared.registry,
      PASSQUEST_ANKI_ENDPOINT: UNREACHABLE_ENDPOINT,
      ...s.shared.launchEnv,
      ...spec.env,
    },
    check: s.check,
    probeListeners: spec.probeListeners,
    killWhen: spec.killWhen,
  });
}

/** `passquest <args> --db <scenario db> --json` from the repository root. */
export function cli(s: ScenarioRun, args: string[], env: Record<string, string> = {}): Promise<RunResult & { doc: unknown }> {
  return cliJson(s.shared.cli, s.database, s.shared.ctx.root, args, { PASSQUEST_PROVIDER_REGISTRY: s.shared.registry, ...env });
}

/** Runs CLI setup steps in order; false after the first one that fails. */
export async function setup(s: ScenarioRun, steps: string[][], env: Record<string, string> = {}): Promise<boolean> {
  for (const args of steps) {
    const result = await cli(s, args, env);
    if (!s.check(result.code === 0, `${s.id}: setup \`passquest ${args.join(" ")}\` exit = ${result.code}: ${tail(result.stderr, 4)}`)) {
      return false;
    }
  }
  return true;
}

/**
 * An environment's string-valued entries: what a launch's own
 * `Record<string, string>` can take, with an absent variable simply absent
 * rather than carrying an `undefined` value.
 */
export function textEnv(env: Record<string, string | undefined>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [name, value] of Object.entries(env)) if (typeof value === "string") out[name] = value;
  return out;
}

const IMPORT_FIXTURE = "internal/cli/testdata/import";

/**
 * Seeds the study trail the coach case and the package smoke's brief case
 * share, through the repository CLI: the import fixture, then a guided and an
 * independent first-party attempt on the imported item — so the recommendation
 * opens it and the brief's packet is non-empty — and the stub model tested and
 * chosen. False after the first failed step; every failure line is prefixed
 * with `${s.id}`.
 */
export async function seedStudyTrail(s: ScenarioRun, env: Record<string, string>): Promise<boolean> {
  const imported = await cli(s, ["import", "--path", resolve(s.shared.ctx.root, IMPORT_FIXTURE)]);
  const sourceID = stringField(field(imported.doc, "source"), "id");
  if (
    !s.check(imported.code === 0 && sourceID !== undefined, `${s.id}: setup import exit = ${imported.code}: ${tail(imported.stderr, 4)}`)
  ) {
    return false;
  }
  const steps = [
    ["question", "add", sourceID!, "1", "--file", resolve(s.shared.ctx.root, `${IMPORT_FIXTURE}/question-q2-tagged.json`)],
    ["page", "review", sourceID!, "1", "--reviewer", "smoke"],
    ["question", "review", sourceID!, "q2", "--answer", "C", "--reviewer", "smoke"],
    ["tactic", "practice", SKILL, "--stage", "guided", "--answer", "A"],
    ["skill", "practice", SKILL, "--answer", "A"],
    ["model", "test", STUB_MODEL],
    ["model", "use", STUB_MODEL],
  ];
  return setup(s, steps, env);
}

export function attemptsOf(report: { doc: unknown }): unknown[] {
  return arrayField(report.doc, "attempts");
}

/** Compares the window's answers with the attempts a second process reads. */
export function checkAttempts(s: ScenarioRun, answers: unknown[], attempts: unknown[]): void {
  for (const answered of answers) {
    const item = field(answered, "item");
    const shown = stringField(field(answered, "condition"), "condition") ?? "";
    const feedback = field(answered, "feedback");
    const id = Number(stringField(feedback, "attempt"));
    const row = attempts.find((attempt) => numberField(attempt, "id") === id);
    if (!s.check(row !== undefined, `${s.id}: report --json has no attempt ${id} for the answer the window recorded`)) continue;
    s.check(
      stringField(row, "question_id") === stringField(item, "question"),
      `${s.id}: attempt ${id} is question ${String(stringField(row, "question_id"))}, the window showed ${String(stringField(item, "question"))}`,
    );
    const elapsed = numberField(row, "elapsed_ms") ?? 0;
    s.check(elapsed > 0, `${s.id}: attempt ${id} elapsed_ms = ${elapsed}`);
    s.check(
      String(elapsed) === stringField(feedback, "elapsedMs"),
      `${s.id}: attempt ${id} elapsed_ms = ${elapsed}, the window showed ${String(stringField(feedback, "elapsedMs"))}`,
    );
    const assistance = stringField(row, "assistance");
    s.check(
      assistance === (shown === "assisted" ? "assisted" : "independent"),
      `${s.id}: the window showed condition ${shown} before answering but attempt ${id} is ${String(assistance)}`,
    );
    s.check(
      stringField(row, "selected_label") === stringField(answered, "chosen"),
      `${s.id}: attempt ${id} recorded ${String(stringField(row, "selected_label"))}, the keyboard chose ${String(stringField(answered, "chosen"))}`,
    );
    const started = stringField(row, "started_at");
    s.check(
      instant(started) === instant(stringField(item, "presentedAt")),
      `${s.id}: attempt ${id} started_at ${String(started)} is not the presentation time the window showed (${String(stringField(item, "presentedAt"))})`,
    );
  }
}
