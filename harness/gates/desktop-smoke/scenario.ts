/**
 * What every desktop-smoke scenario shares: the run it is handed, launching the window on its own data
 * directory, the second process (`passquest ... --db`) and the attempt cross-check.
 */
import { cliJson, launchPage, tail } from "../../lib/desktop-page.ts";
import type { Check, Launched, ProviderStub } from "../../lib/desktop-page.ts";
import { arrayField, field, numberField, stringField } from "../../lib/json.ts";
import type { RunResult } from "../../lib/process.ts";
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
    // The repository root is the working directory, so the app finds
    // agent/*.mjs and runs them under Node the way the CLI smokes do.
    cwd: s.shared.ctx.root,
    dir: s.dir,
    label: spec.label,
    header: `desktop-smoke: ${s.id}/${spec.label}`,
    where: spec.label === s.id ? s.id : `${s.id} (${spec.label})`,
    script: spec.script,
    env: {
      PASSQUEST_DESKTOP_DATA: s.dataDir,
      PASSQUEST_DESKTOP_SECRET_SERVICE: s.service,
      PASSQUEST_PROVIDER_REGISTRY: s.shared.registry,
      PASSQUEST_ANKI_ENDPOINT: UNREACHABLE_ENDPOINT,
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
