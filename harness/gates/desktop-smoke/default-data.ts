/**
 * The window and the command line over one per-user database (goal 0020 P3;
 * Done-when 1 and 3). The app resolves its own default data directory — no
 * PASSQUEST_DESKTOP_DATA, no PASSQUEST_DB — records the recommended walkthrough
 * attempt, and holds: while that process is still running, the CLI on the same
 * per-user root reads the attempt back through `report --json` and records a
 * second one with no `--db`. A second launch reads both back from the window's
 * own Today. The DB file must sit at the platform's per-user path and nowhere
 * else under the scenario's tree, so a run that reads the same directory both
 * ways is proven per OS by the files the two entry points left, not by a
 * promise (the paths are in the gate's detail line).
 */
import { Database } from "bun:sqlite";
import { existsSync, mkdirSync, readdirSync } from "node:fs";
import path from "node:path";

import { cleanEnv, dataRootFor, homeEnv, pageScript, parse, SKILL } from "../../lib/desktop-page.ts";
import { arrayField, field, numberField } from "../../lib/json.ts";
import { run, tail } from "../../lib/process.ts";
import { launch, textEnv } from "./scenario.ts";
import type { Scenario, ScenarioRun } from "./scenario.ts";

/** The attempts the store holds, or null while the file cannot be read yet (not created, or busy). */
function attemptsAt(database: string): number | null {
  if (!existsSync(database)) return null;
  let db: Database | undefined;
  try {
    db = new Database(database, { readonly: true });
    const row = db.query<{ n: number }, []>("SELECT COUNT(*) AS n FROM attempts").get();
    return row ? row.n : null;
  } catch {
    return null;
  } finally {
    db?.close();
  }
}

/** Every `passquest.sqlite` under a tree: the per-user file must be the only one. */
function databasesUnder(dir: string): string[] {
  const found: string[] = [];
  const walk = (current: string): void => {
    for (const entry of readdirSync(current, { withFileTypes: true })) {
      const child = path.join(current, entry.name);
      if (entry.isDirectory()) walk(child);
      else if (entry.name === "passquest.sqlite") found.push(child);
    }
  };
  if (existsSync(dir)) walk(dir);
  return found;
}

interface CliHalf {
  /** Attempts `report --json` listed beside the live window, before the CLI's own write. */
  before: number;
  /** The CLI practice command's exit code. */
  practiceCode: number;
  /** Its stderr tail, empty on success. */
  practiceErr: string;
  /** Attempts listed after it. */
  after: number;
  ids: number[];
}

export const defaultData: Scenario = {
  id: "default-data",
  async run(s) {
    const home = path.join(s.dir, "home");
    mkdirSync(home, { recursive: true });
    const dataRoot = dataRootFor(home, "Pass Quest");
    const database = path.join(dataRoot, "passquest.sqlite");
    const scenario: ScenarioRun = {
      ...s,
      dataDir: dataRoot,
      database,
      shared: { ...s.shared, dataOverride: false, launchEnv: { ...s.shared.launchEnv, ...homeEnv(home) } },
    };
    const cwd = path.join(s.dir, "cwd");
    mkdirSync(cwd, { recursive: true });
    const cliEnv: Record<string, string> = {
      ...textEnv(cleanEnv()),
      ...textEnv(scenario.shared.launchEnv ?? {}),
      PASSQUEST_PROVIDER_REGISTRY: scenario.shared.registry,
    };
    const observed: { half: CliHalf | null } = { half: null };
    const cli = (args: string[]) => run(scenario.shared.cli, args, { cwd, env: cliEnv, replaceEnv: true });

    const held = await launch(scenario, {
      label: "record",
      script: pageScript(`
await today();
press("Enter");
await until("a practice item", () => q('[data-testid="practice-item"]'));
const answered = await answer("1");
await new Promise((hold) => { window.__pqHold = hold; });
return answered;
`),
      // Polled while the window runs: the CLI's read and its own write happen
      // beside the live process, and the true kill ends the launch.
      killWhen: async () => {
        if (observed.half !== null) return true;
        if ((attemptsAt(database) ?? 0) < 1) return false;
        const before = await cli(["report", "--json"]);
        const practice = await cli(["tactic", "practice", SKILL, "--stage", "guided", "--answer", "A", "--json"]);
        const after = await cli(["report", "--json"]);
        observed.half = {
          before: arrayField(parse(before.stdout), "attempts").length,
          practiceCode: practice.code,
          practiceErr: practice.code === 0 ? "" : tail(practice.stderr, 4),
          after: arrayField(parse(after.stdout), "attempts").length,
          ids: arrayField(parse(after.stdout), "attempts").map((attempt) => numberField(attempt, "id") ?? -1),
        };
        return true;
      },
    });
    if (!held) return;

    const attempts = attemptsAt(database);
    const found = databasesUnder(s.dir);
    if (
      !s.check(
        attempts !== null && attempts >= 1,
        `default-data: the window left ${attempts === null ? "no readable" : String(attempts)} attempt(s) at ${database}; databases under ${s.dir}: ${
          found.join(", ") || "none"
        }`,
      )
    ) {
      return;
    }
    if (!s.check(held.killed, "default-data: the window exited on its own, so the CLI never ran beside a live window")) return;
    const cliHalf = observed.half;
    if (cliHalf === null) {
      s.check(false, "default-data: the CLI never saw the window's attempt, so it recorded none");
      return;
    }
    s.check(cliHalf.before === 1, `default-data: the CLI read ${cliHalf.before} attempt(s) beside the live window, want the window's 1`);
    s.check(cliHalf.practiceCode === 0, `default-data: the CLI's practice command exit = ${cliHalf.practiceCode}: ${cliHalf.practiceErr}`);
    s.check(cliHalf.after === 2, `default-data: the CLI's write left ${cliHalf.after} attempt(s), want 2`);
    s.check(
      cliHalf.ids.join(",") === "1,2",
      `default-data: the store holds attempt ids [${cliHalf.ids.join(", ")}], want the window's 1 and the CLI's 2`,
    );
    s.check(
      found.length === 1 && path.resolve(found[0]!) === path.resolve(database),
      `default-data: ${found.length} database(s) under ${s.dir}: ${found.join(", ") || "none"}, want the per-user ${database}`,
    );

    const read = await launch(scenario, {
      label: "read",
      script: pageScript(`
const doc = await window.mygo.call("Study.Today");
const skills = Array.isArray(doc.skills) ? doc.skills : [];
const found = skills.find((entry) => entry.skill && entry.skill.id === "${SKILL}");
return { evidence: found ? found.evidence : null };
`),
    });
    if (!read) return;
    const evidence = field(read.value, "evidence");
    const seen = arrayField(evidence, "attempt_ids").map(Number);
    const assisted = numberField(evidence, "assisted_attempts") ?? 0;
    s.check(
      seen.length === 2 && seen.join(",") === cliHalf.ids.join(","),
      `default-data: the window's Today lists attempt ids [${seen.join(", ")}] for ${SKILL}, the store holds [${cliHalf.ids.join(", ")}]`,
    );
    s.check(
      assisted === 2,
      `default-data: the window's Today counts ${assisted} assisted attempt(s) for ${SKILL}; want the 2 the store holds`,
    );
    s.detail?.(
      `default-data: ${database} written by the window and the CLI, read back by the window's Today (attempts ${cliHalf.ids.join("+")})`,
    );
  },
};
