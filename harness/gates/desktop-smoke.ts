import { existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { buildDesktop, childProcesses, listeningSockets, removeTree, startDesktop, stillRunning } from "../lib/desktop.ts";
import type { LaunchResult, ProcessInfo } from "../lib/desktop.ts";
import { hasGoMod } from "../lib/go.ts";
import { field, numberField } from "../lib/json.ts";
import { executable } from "../lib/process.ts";
import { resolve } from "../lib/repo.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";

/**
 * The app contract this gate drives (implemented by cmd/passquest-desktop):
 * PASSQUEST_DESKTOP_SCRIPT is a .js file whose content is the body of an async
 * function evaluated in the page after load; the app writes
 * {"ok":bool,"value":...,"error":string} to PASSQUEST_DESKTOP_RESULT and quits.
 * PASSQUEST_DESKTOP_HIDDEN=1 hides the window, PASSQUEST_DESKTOP_DATA is the
 * user-data directory (database at <dir>/passquest.sqlite), and
 * PASSQUEST_DESKTOP_SECRET_SERVICE names the credential-store service so a run
 * never touches the learner's real one.
 */
const DESKTOP_SURFACES = ["cmd/passquest-desktop", "desktop/web", "internal/desktop"];
const LAUNCH_TIMEOUT_MS = 60_000;
/** WebView2 and WebKit helpers exit shortly after the host; longer is an orphan. */
const ORPHAN_GRACE_MS = 10_000;
const MIN_TODAY_TEXT = 200;

type Check = (condition: boolean, message: string) => boolean;

interface ScenarioRun {
  ctx: GateContext;
  dataDir: string;
  database: string;
  check: Check;
}

interface Scenario {
  id: string;
  /** Body of an async function evaluated in the page; returns a JSON-serialisable value. */
  script: string;
  /** Probes listening TCP sockets of the app and its children while it runs. */
  probeListeners?: boolean;
  /** Seeds the data directory before launch (TODO hook: imports, stubs, a prior launch). */
  setup?: (run: ScenarioRun) => Promise<void>;
  assert: (value: unknown, run: ScenarioRun) => void;
  /** Cross-checks after exit (TODO hook: `passquest report --json` against the database). */
  afterExit?: (run: ScenarioRun) => Promise<void>;
}

/**
 * Scenarios a later increment of this goal fills in; each is listed in the
 * gate detail until it exists, so a pass never hides an unbuilt check.
 * TODO(0011): practice (keyboard answer, elapsed_ms > 0 via the CLI report),
 * hint (assisted attempt), coach (loopback provider stub, consent, 401),
 * assess (start, leave, resume, report "pass threshold: unknown"),
 * anki (loopback AnkiConnect stub, queued when unreachable), settings
 * (credential stored under the test service, absent from SQLite/logs/DOM),
 * resume (kill mid-item, relaunch, same presentation, no attempt), and
 * client-fresh (regenerate the MyGo client with MYGO_GENERATE and compare it
 * with desktop/web/src/api/mygo.ts).
 */
type PendingScenario = "practice" | "hint" | "coach" | "assess" | "anki" | "settings" | "resume" | "client-fresh";
const PENDING_SCENARIOS: readonly PendingScenario[] = [
  "practice",
  "hint",
  "coach",
  "assess",
  "anki",
  "settings",
  "resume",
  "client-fresh",
];

const BOOT_SCRIPT = `
const stage = () => document.querySelector("#stage");
const ready = () => window.__pq === undefined || window.__pq.ready === true;
const content = () => (stage()?.children.length ?? 0) > 0 && document.body.innerText.length > ${MIN_TODAY_TEXT};
const deadline = Date.now() + 15000;
while (!(ready() && content()) && Date.now() < deadline) await new Promise((done) => setTimeout(done, 100));
const node = stage();
const errors = document.querySelectorAll("#errbar, .error-bar");
return {
  textLength: document.body.innerText.length,
  stageChildren: node ? node.children.length : -1,
  stageTextLength: node ? node.innerText.trim().length : 0,
  errorBars: errors.length,
  errorText: Array.from(errors, (element) => element.textContent ?? "").join(" | ").slice(0, 300),
  ready: window.__pq === undefined ? null : window.__pq.ready === true,
};
`;

const LIFECYCLE_SCRIPT = `
await new Promise((done) => setTimeout(done, 2000));
return { waited: true };
`;

const SCENARIOS: Scenario[] = [
  {
    id: "boot",
    script: BOOT_SCRIPT,
    assert(value, { check }) {
      const textLength = numberField(value, "textLength") ?? 0;
      const stageChildren = numberField(value, "stageChildren") ?? -1;
      check(textLength > MIN_TODAY_TEXT, `boot: blank Today, body text length ${textLength} <= ${MIN_TODAY_TEXT}`);
      check(stageChildren >= 0, "boot: #stage is missing");
      check(
        stageChildren > 0 && (numberField(value, "stageTextLength") ?? 0) > 0,
        `boot: #stage is empty (${stageChildren} children)`,
      );
      const errorBars = numberField(value, "errorBars") ?? -1;
      check(errorBars === 0, `boot: ${errorBars} error bar(s): ${String(field(value, "errorText") ?? "")}`);
      const ready = field(value, "ready");
      check(ready === null || ready === true, `boot: window.__pq.ready = ${JSON.stringify(ready)}`);
    },
  },
  {
    id: "lifecycle",
    script: LIFECYCLE_SCRIPT,
    probeListeners: true,
    assert(value, { check }) {
      check(field(value, "waited") === true, `lifecycle: script returned ${JSON.stringify(value)}`);
    },
  },
];

function tail(text: string, lines = 12): string {
  return text.replace(/\r\n/g, "\n").trim().split("\n").slice(-lines).join("\n");
}

function sleep(ms: number): Promise<void> {
  return new Promise((done) => setTimeout(done, ms));
}

function describe(processes: ProcessInfo[]): string {
  return processes.map((entry) => `${entry.pid} ${entry.command}`).join(", ");
}

async function runScenario(ctx: GateContext, scenario: Scenario, binary: string, temp: string, check: Check): Promise<void> {
  const dir = path.join(temp, scenario.id);
  const dataDir = path.join(dir, "data");
  mkdirSync(dataDir, { recursive: true });
  const scriptPath = path.join(dir, "scenario.js");
  const resultPath = path.join(dir, "result.json");
  writeFileSync(scriptPath, scenario.script);
  const run: ScenarioRun = { ctx, dataDir, database: path.join(dataDir, "passquest.sqlite"), check };
  if (scenario.setup) await scenario.setup(run);

  const env: Record<string, string | undefined> = {
    ...process.env,
    PASSQUEST_DESKTOP_SCRIPT: scriptPath,
    PASSQUEST_DESKTOP_RESULT: resultPath,
    PASSQUEST_DESKTOP_HIDDEN: "1",
    PASSQUEST_DESKTOP_DATA: dataDir,
    PASSQUEST_DESKTOP_SECRET_SERVICE: `passquest-smoke-${process.pid}-${Date.now()}`,
  };
  // A developer's PASSQUEST_DB would point the window at a real database.
  delete env.PASSQUEST_DB;

  const launch = startDesktop({ binary, env, timeoutMs: LAUNCH_TIMEOUT_MS, cwd: dir });
  let exited = false;
  let outcome: LaunchResult | undefined;
  const finished = launch.result.then((result) => {
    exited = true;
    outcome = result;
  });

  // While the app runs: remember its children (POSIX reparents orphans, so the
  // after-exit check compares against this snapshot) and probe listeners.
  let appPid: number | null = null;
  const seen = new Map<number, ProcessInfo>();
  let listenerProbes = 0;
  while (!exited) {
    appPid ??= await launch.appPid();
    if (appPid !== null && !exited) {
      const children = await childProcesses(appPid);
      if ("items" in children) for (const child of children.items) seen.set(child.pid, child);
      if (scenario.probeListeners && !exited) {
        const owners = [appPid, ...("items" in children ? children.items.map((child) => child.pid) : [])];
        const sockets = await listeningSockets(owners);
        if ("error" in sockets) {
          check(false, `${scenario.id}: cannot list listening sockets: ${sockets.error}`);
        } else {
          listenerProbes++;
          for (const socket of sockets.items) {
            check(false, `${scenario.id}: process ${socket.pid} listens on TCP ${socket.address}:${socket.port}`);
          }
        }
      }
    }
    await Promise.race([finished, sleep(250)]);
  }
  await finished;
  const result = outcome as LaunchResult;
  if (scenario.probeListeners) {
    check(listenerProbes > 0, `${scenario.id}: no listening-socket probe completed while the app ran`);
  }

  const output = tail(`${result.stdout}\n${result.stderr}`);
  if (result.error) {
    check(false, `${scenario.id}: could not launch: ${result.error}`);
    return;
  }
  if (!check(!result.timedOut, `${scenario.id}: timeout after ${LAUNCH_TIMEOUT_MS} ms; killed\n${output}`)) return;

  // After exit: nothing the app started may outlive it.
  let survivors: ProcessInfo[] = [];
  const deadline = Date.now() + ORPHAN_GRACE_MS;
  for (;;) {
    const remembered = await stillRunning([...seen.values()]);
    const orphans = process.platform === "win32" && appPid !== null ? await childProcesses(appPid) : { items: [] };
    if ("error" in remembered || "error" in orphans) {
      const error = "error" in remembered ? remembered.error : "error" in orphans ? orphans.error : "";
      check(false, `${scenario.id}: cannot list child processes: ${error}`);
      break;
    }
    survivors = [...remembered.items, ...orphans.items.filter((entry) => !seen.has(entry.pid))];
    if (survivors.length === 0 || Date.now() > deadline) break;
    await sleep(500);
  }
  check(survivors.length === 0, `${scenario.id}: child process(es) outlived the app: ${describe(survivors)}`);

  if (!existsSync(resultPath)) {
    check(false, `${scenario.id}: app exited ${result.code} without writing a result\n${output}`);
    return;
  }
  let document: unknown;
  try {
    document = JSON.parse(readFileSync(resultPath, "utf8"));
  } catch (error) {
    check(false, `${scenario.id}: result is not JSON: ${error instanceof Error ? error.message : String(error)}`);
    return;
  }
  if (!check(field(document, "ok") === true, `${scenario.id}: script failed: ${String(field(document, "error") ?? "no error text")}`)) {
    return;
  }
  check(result.code === 0, `${scenario.id}: app exit = ${result.code}\n${output}`);
  scenario.assert(field(document, "value"), run);
  if (scenario.afterExit) await scenario.afterExit(run);
}

export const desktopSmoke: Gate = {
  id: "desktop-smoke",
  title: "desktop smoke",
  async run(ctx) {
    const present = DESKTOP_SURFACES.filter((surface) => existsSync(resolve(ctx.root, surface)));
    if (present.length === 0) return skip("no desktop surface yet");
    if (!present.includes("cmd/passquest-desktop")) {
      return fail([`desktop surface exists (${present.join(", ")}) but cmd/passquest-desktop is missing`]);
    }
    if (!hasGoMod(ctx)) return fail(["cmd/passquest-desktop exists but go.mod is missing"]);

    const temp = mkdtempSync(path.join(tmpdir(), "passquest-desktop-smoke-"));
    const binary = path.join(temp, executable("passquest-desktop"));
    const checks: string[] = [];
    let assertions = 0;
    const check: Check = (condition, message) => {
      assertions++;
      if (!condition) checks.push(message);
      return condition;
    };
    try {
      const build = await buildDesktop(ctx, binary);
      if (!build.ok) return fail(build.failures);
      for (const scenario of SCENARIOS) {
        await runScenario(ctx, scenario, binary, temp, check);
      }
      if (checks.length > 0) return fail(checks);
      return pass(
        `${assertions} assertions over ${SCENARIOS.length} scenarios (${SCENARIOS.map((s) => s.id).join(", ")}); pending: ${PENDING_SCENARIOS.join(", ")}`,
      );
    } finally {
      await removeTree(temp);
    }
  },
};
