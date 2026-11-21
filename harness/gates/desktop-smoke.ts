/**
 * Drives the app contract described in harness/lib/desktop-page.ts. Each
 * scenario gets a fresh data directory; a scenario about persistence launches
 * the window more than once on its own directory. A second process
 * (`passquest ... --db`) cross-checks what the window recorded.
 *
 * One module per scenario under desktop-smoke/; this file holds their order and the run around them.
 */
import { mkdirSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { buildDesktop, removeTree } from "../lib/desktop.ts";
import { desktopSurface, startProviderStub, writeStubRegistry } from "../lib/desktop-page.ts";
import type { Check } from "../lib/desktop-page.ts";
import { buildCLI } from "../lib/go.ts";
import { executable, run, tail } from "../lib/process.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate, GateContext, GateResult } from "../lib/types.ts";

import { anki } from "./desktop-smoke/anki.ts";
import { assess } from "./desktop-smoke/assess.ts";
import { boot } from "./desktop-smoke/boot.ts";
import { clientFresh } from "./desktop-smoke/client-fresh.ts";
import { coach } from "./desktop-smoke/coach.ts";
import { feedback } from "./desktop-smoke/feedback.ts";
import { hint } from "./desktop-smoke/hint.ts";
import { lifecycle } from "./desktop-smoke/lifecycle.ts";
import { manual } from "./desktop-smoke/manual.ts";
import { practice } from "./desktop-smoke/practice.ts";
import { resume } from "./desktop-smoke/resume.ts";
import type { Scenario, Shared } from "./desktop-smoke/scenario.ts";
import { settings } from "./desktop-smoke/settings.ts";
import { update } from "./desktop-smoke/update.ts";

/** client-fresh and boot run first; the rest need a window that boots. */
const SCENARIOS: Scenario[] = [clientFresh, boot, lifecycle, practice, hint, feedback, coach, manual, assess, anki, settings, update, resume];

/**
 * The desktop smoke over every scenario, or, for a self-test case whose failure is decided by one
 * scenario, over the scenarios `only` names (kept in their usual order; client-fresh and boot always
 * run first, as every later scenario needs a window that boots).
 */
export function makeDesktopSmokeGate(only?: readonly string[]): Gate {
  const scenarios = only ? SCENARIOS.filter((scenario) => scenario === clientFresh || scenario === boot || only.includes(scenario.id)) : SCENARIOS;
  return {
    id: "desktop-smoke",
    title: "desktop smoke",
    run: (ctx) => runDesktopSmoke(ctx, scenarios),
  };
}

export const desktopSmoke = makeDesktopSmokeGate();

async function runDesktopSmoke(ctx: GateContext, scenarios: readonly Scenario[]): Promise<GateResult> {
  const surface = desktopSurface(ctx);
  if (surface) return surface;

  const temp = mkdtempSync(path.join(tmpdir(), "passquest-desktop-smoke-"));
  const checks: string[] = [];
  let assertions = 0;
  const check: Check = (condition, message) => {
    assertions++;
    if (!condition) checks.push(message);
    return condition;
  };
  const provider = startProviderStub();
  try {
    const binary = path.join(temp, executable("passquest-desktop"));
    const build = await buildDesktop(ctx, binary);
    if (!build.ok) return fail(build.failures);
    const registry = path.join(temp, "registry.json");
    writeStubRegistry(registry, provider.endpoint);
    const shared: Shared = { ctx, binary, cli: "", registry, provider };
    let ran = 0;
    for (const scenario of scenarios) {
      const dir = path.join(temp, scenario.id);
      const dataDir = path.join(dir, "data");
      mkdirSync(dataDir, { recursive: true });
      const before = checks.length;
      await scenario.run({
        shared,
        id: scenario.id,
        dir,
        dataDir,
        database: path.join(dataDir, "passquest.sqlite"),
        service: `passquest-smoke-${process.pid}-${Date.now()}-${scenario.id}`,
        check,
      });
      ran++;
      if (scenario !== boot) continue;
      if (checks.length > before) {
        checks.push(`${scenarios.length - ran} later scenario(s) not run: the window does not boot`);
        return fail(checks);
      }
      // Later scenarios cross-check the window through a second process.
      const cliBuild = await buildCLI(ctx);
      if ("failure" in cliBuild) return fail([...checks, "go build ./cmd/passquest failed", tail(cliBuild.failure)]);
      shared.cli = cliBuild.binary;
      const node = await run(executable("node"), ["--version"]);
      if (node.code !== 0) return fail([...checks, `node is required for the desktop smoke gate: ${node.stderr || node.stdout}`]);
    }
    if (checks.length > 0) return fail(checks);
    return pass(`${assertions} assertions over ${scenarios.length} scenarios`);
  } finally {
    provider.stop();
    await removeTree(temp);
  }
}
