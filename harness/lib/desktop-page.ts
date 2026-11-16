/**
 * What every gate that drives the real desktop window through the smoke hook
 * shares: the page-script prelude, the loopback provider stub and its
 * registry, the launch with its orphan and listener checks, and the CLI call
 * that cross-checks the window from a second process.
 *
 * The app contract (implemented by cmd/passquest-desktop):
 * PASSQUEST_DESKTOP_SCRIPT is a .js file whose content is the body of an async
 * function evaluated in the page after load; the app writes
 * {"ok":bool,"value":...,"error":string} to PASSQUEST_DESKTOP_RESULT and quits.
 * PASSQUEST_DESKTOP_HIDDEN=1 hides the window, PASSQUEST_DESKTOP_DATA is the
 * user-data directory (database at <dir>/passquest.sqlite),
 * PASSQUEST_DESKTOP_SECRET_SERVICE names the credential-store service so a run
 * never touches the learner's real one, and PASSQUEST_DESKTOP_CONTENT_SIZE=WxH
 * fixes the page area of a smoke launch. With MYGO_GENERATE set the binary
 * only writes its TypeScript client to that path.
 */
import { createHash } from "node:crypto";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";

import { childProcesses, descendants, listeningSockets, listProcesses, startDesktop, stillRunning } from "./desktop.ts";
import type { LaunchResult, ProcessIdentity, ProcessInfo } from "./desktop.ts";
import { hasGoMod } from "./go.ts";
import { field } from "./json.ts";
import { run } from "./process.ts";
import type { RunResult } from "./process.ts";
import { resolve } from "./repo.ts";
import { fail, skip } from "./types.ts";
import type { GateContext, GateResult } from "./types.ts";

export const LAUNCH_TIMEOUT_MS = 60_000;
/** WebView2 and WebKit helpers exit shortly after the host; longer is an orphan. */
const ORPHAN_GRACE_MS = 10_000;

export const STUB_PROVIDER = "stub";
export const STUB_MODEL = `${STUB_PROVIDER}/stub-model`;
/** The registry requires PASSQUEST_<PROVIDER>_API_KEY. */
export const STUB_CREDENTIAL_ENV = "PASSQUEST_STUB_API_KEY";
/** The scoring skill a fresh database recommends; its tactic has a guided and an independent stage. */
export const SKILL = "not-except-polarity";

export type Check = (condition: boolean, message: string) => boolean;

const DESKTOP_SURFACES = ["cmd/passquest-desktop", "desktop/web", "internal/desktop"];

/** Null when the desktop app can be built; otherwise the skip (no surface yet) or the failure. */
export function desktopSurface(ctx: GateContext): GateResult | null {
  const present = DESKTOP_SURFACES.filter((surface) => existsSync(resolve(ctx.root, surface)));
  if (present.length === 0) return skip("no desktop surface yet");
  if (!present.includes("cmd/passquest-desktop")) {
    return fail([`desktop surface exists (${present.join(", ")}) but cmd/passquest-desktop is missing`]);
  }
  if (!hasGoMod(ctx)) return fail(["cmd/passquest-desktop exists but go.mod is missing"]);
  return null;
}

/**
 * Helpers every page script starts with. Steps wait on the DOM hooks the views
 * expose; a step that never happens throws with the route, error bar and
 * toast, so the failed result says where the window stopped.
 */
export const PRELUDE = `
const sleep = (ms) => new Promise((done) => setTimeout(done, ms));
const q = (selector) => document.querySelector(selector);
const all = (selector) => Array.from(document.querySelectorAll(selector));
const data = (selector) => Object.assign({}, q(selector)?.dataset);
const situation = () => {
  const bar = q("#errbar, .error-bar");
  const toast = q(".toast");
  return " [route " + (location.hash || "#/today") + (bar ? "; error bar: " + bar.textContent : "") + (toast ? "; toast: " + toast.textContent : "") + "]";
};
const until = async (what, probe, ms = 15000) => {
  const deadline = Date.now() + ms;
  for (;;) {
    const value = probe();
    if (value) return value;
    if (Date.now() > deadline) throw new Error("timed out waiting for " + what + situation());
    await sleep(50);
  }
};
const click = async (selector) => {
  const element = await until(selector + " (enabled)", () => {
    const found = q(selector);
    return found && !found.disabled ? found : null;
  }, 10000);
  element.click();
};
const press = (key) => document.dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: true }));
const today = async () => {
  if (location.hash !== "" && location.hash !== "#/today") location.hash = "#/today";
  await until("Today", () => q('[data-testid="hero"]'));
  await sleep(150);
};
const startFromSheet = async (act, value) => {
  await today();
  await click('[data-act="sheet"][data-v="pick"]');
  const target = '#layer [data-act="' + act + '"][data-v="' + value + '"]';
  await until("the practice sheet entry " + value, () => q(target));
  await click(target);
  await until("a practice item", () => q('[data-testid="practice-item"]'));
};
const answer = async (key) => {
  await until("a practice item", () => q('[data-testid="practice-item"]'));
  await sleep(250);
  const shown = { item: data('[data-testid="practice-item"]'), condition: data('[data-testid="condition"]') };
  press(key);
  const chosen = await until("option " + key + " to be selected", () => q('.opt[aria-checked="true"]'));
  press("Enter");
  await until("the feedback card", () => q('[data-testid="feedback"]'));
  return { ...shown, chosen: chosen.dataset.v, feedback: data('[data-testid="feedback"]'), counted: data('[data-testid="feedback-condition"]') };
};
await until("window.__pq.ready", () => window.__pq && window.__pq.ready === true, 20000);
`;

export function pageScript(body: string): string {
  return `${PRELUDE}\n${body}`;
}

export function tail(text: string, lines = 12): string {
  return text.replace(/\r\n/g, "\n").trim().split("\n").slice(-lines).join("\n");
}

export function sleep(ms: number): Promise<void> {
  return new Promise((done) => setTimeout(done, ms));
}

export function parse(raw: string): unknown {
  try {
    return JSON.parse(raw);
  } catch {
    return {};
  }
}

/** This process's environment without any PASSQUEST_* variable a developer shell may carry. */
export function cleanEnv(): Record<string, string | undefined> {
  const env: Record<string, string | undefined> = {};
  for (const [name, value] of Object.entries(process.env)) {
    if (!name.startsWith("PASSQUEST_")) env[name] = value;
  }
  return env;
}

export interface ProviderRequest {
  authorization: string;
  path: string;
  raw: string;
  sha256: string;
}

/** A loopback OpenAI-compatible endpoint; `respond` is set per scenario. */
export interface ProviderStub {
  endpoint: string;
  received: ProviderRequest[];
  respond: (request: ProviderRequest) => Response | Promise<Response>;
  stop: () => void;
}

export function chatReply(text: string): Response {
  return Response.json({ choices: [{ message: { content: text } }], usage: { prompt_tokens: 10, completion_tokens: 5 } });
}

export function startProviderStub(): ProviderStub {
  const stub: ProviderStub = {
    endpoint: "",
    received: [],
    respond: () => chatReply("OK"),
    stop: () => server.stop(true),
  };
  const server = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    async fetch(request) {
      const raw = await request.text();
      const entry: ProviderRequest = {
        authorization: request.headers.get("authorization") ?? "",
        path: new URL(request.url).pathname,
        raw,
        sha256: createHash("sha256").update(raw).digest("hex"),
      };
      stub.received.push(entry);
      return stub.respond(entry);
    },
  });
  stub.endpoint = `http://127.0.0.1:${server.port}/v1/chat/completions`;
  return stub;
}

/** A provider registry with the one stub provider and its one text.chat model. */
export function writeStubRegistry(file: string, endpoint: string): void {
  writeFileSync(
    file,
    JSON.stringify({
      version: 1,
      providers: [
        {
          id: STUB_PROVIDER,
          name: "Smoke stub",
          endpoint,
          endpoint_shape: "openai-chat-completions",
          credential_env: STUB_CREDENTIAL_ENV,
          models: [{ id: STUB_MODEL.split("/")[1], default: true, capabilities: ["text.chat"] }],
        },
      ],
    }),
  );
}

/** `passquest <args> --db <database> --json` from `cwd`; `doc` is the parsed stdout. */
export async function cliJson(
  cli: string,
  database: string,
  cwd: string,
  args: string[],
  env: Record<string, string>,
): Promise<RunResult & { doc: unknown }> {
  const result = await run(cli, [...args, "--db", database, "--json"], { cwd, env });
  return { ...result, doc: parse(result.stdout) };
}

export interface PageLaunch {
  binary: string;
  /** The app's working directory: the repository root, so it finds agent/*.mjs. */
  cwd: string;
  /** Where the script, the result and the launch's WebView2 profile live. */
  dir: string;
  label: string;
  /** Written as the script's first line comment (`// <header>`). */
  header: string;
  /** How failures name this launch. */
  where: string;
  script: string;
  /** App variables on top of the clean environment and the smoke hook's own. */
  env: Record<string, string | undefined>;
  check: Check;
  /** Probes listening TCP sockets of the app and its children while it runs. */
  probeListeners?: boolean;
  /** Polled while the app runs; true kills the process tree with the watchdog's kill. */
  killWhen?: () => Promise<boolean>;
}

export interface Launched {
  value: unknown;
  killed: boolean;
  /** The app's stdout and stderr. */
  output: string;
  resultPath: string;
}

function describe(processes: ProcessInfo[]): string {
  return processes.map((entry) => `${entry.pid} ${entry.command}`).join(", ");
}

/**
 * Launches the hidden window once with a script and returns the script's
 * value, or null after a failed check. While the app runs it remembers the
 * app's children (POSIX reparents orphans) and optionally probes listeners;
 * after exit nothing the app started may outlive it.
 */
export async function launchPage(spec: PageLaunch): Promise<Launched | null> {
  const { check, where } = spec;
  const scriptPath = path.join(spec.dir, `${spec.label}.js`);
  const resultPath = path.join(spec.dir, `${spec.label}.result.json`);
  writeFileSync(scriptPath, `// ${spec.header}\n${spec.script}`);

  const env: Record<string, string | undefined> = {
    ...cleanEnv(),
    PASSQUEST_DESKTOP_SCRIPT: scriptPath,
    PASSQUEST_DESKTOP_RESULT: resultPath,
    PASSQUEST_DESKTOP_HIDDEN: "1",
    // A fresh profile per launch, removed with the gate's temporary tree once
    // the orphan check has seen every WebView2 helper exit.
    WEBVIEW2_USER_DATA_FOLDER: path.join(spec.dir, `${spec.label}.webview2`),
    ...spec.env,
  };
  // The app cannot have been created before this instant; it is the app's
  // creation time until the process table shows the real one, so a process
  // that only carries the app's pid as a stale parent id is never its child.
  // Floored to the second and one more second earlier: `ps` reports start
  // times to the second, and on Linux derives them from an integer boot time,
  // so a process started now can be listed up to a second before the floor.
  // Erring early only ever keeps a process in the orphan check.
  const launchedAt = Math.floor(Date.now() / 1000) * 1000 - 1000;
  const handle = startDesktop({ binary: spec.binary, env, timeoutMs: LAUNCH_TIMEOUT_MS, cwd: spec.cwd });
  let exited = false;
  let outcome: LaunchResult | undefined;
  const finished = handle.result.then((result) => {
    exited = true;
    outcome = result;
  });

  let app: ProcessIdentity | null = null;
  const seen = new Map<number, ProcessInfo>();
  let listenerProbes = 0;
  // The first process-table failure: the orphan check cannot see children it never listed.
  let tableError: string | null = null;
  let killed = false;
  while (!exited) {
    if (app === null) {
      const pid = await handle.appPid();
      if (pid !== null && typeof pid === "object") tableError ??= pid.error;
      else if (pid !== null) app = { pid, started: launchedAt };
    }
    if (app !== null && !exited) {
      const table = await listProcesses();
      if ("error" in table) tableError ??= table.error;
      const rows = "items" in table ? table.items : [];
      const pid: number = app.pid;
      const self = rows.find((entry) => entry.pid === pid && entry.started >= launchedAt);
      if (self) app = { pid, started: self.started };
      const children = descendants(rows, app);
      for (const child of children) seen.set(child.pid, child);
      if (spec.probeListeners && !exited) {
        const owners = [app.pid, ...children.map((child) => child.pid)];
        const sockets = await listeningSockets(owners);
        if ("error" in sockets) {
          check(false, `${where}: cannot list listening sockets: ${sockets.error}`);
        } else {
          listenerProbes++;
          for (const socket of sockets.items) {
            check(false, `${where}: process ${socket.pid} listens on TCP ${socket.address}:${socket.port}`);
          }
        }
      }
    }
    if (spec.killWhen && !killed && !exited && (await spec.killWhen())) {
      killed = true;
      await handle.kill();
    }
    await Promise.race([finished, sleep(250)]);
  }
  await finished;
  const result = outcome as LaunchResult;
  check(tableError === null, `${where}: cannot list processes while the app ran: ${tableError}`);
  if (spec.probeListeners) {
    check(listenerProbes > 0, `${where}: no listening-socket probe completed while the app ran`);
  }

  const output = `${result.stdout}\n${result.stderr}`;
  const shortOutput = tail(output);
  if (result.error) {
    check(false, `${where}: could not launch: ${result.error}`);
    return null;
  }
  if (!check(!result.timedOut, `${where}: timeout after ${LAUNCH_TIMEOUT_MS} ms; killed\n${shortOutput}`)) return null;

  let survivors: ProcessInfo[] = [];
  const deadline = Date.now() + ORPHAN_GRACE_MS;
  for (;;) {
    const remembered = await stillRunning([...seen.values()]);
    // Windows keeps the dead app's pid on its orphans: children created between the app's start and its exit.
    const orphans = process.platform === "win32" && app !== null ? await childProcesses(app, result.exitedAt) : { items: [] };
    if ("error" in remembered || "error" in orphans) {
      const error = "error" in remembered ? remembered.error : "error" in orphans ? orphans.error : "";
      check(false, `${where}: cannot list child processes: ${error}`);
      break;
    }
    survivors = [...remembered.items, ...orphans.items.filter((entry) => !seen.has(entry.pid))];
    if (survivors.length === 0 || Date.now() > deadline) break;
    await sleep(500);
  }
  check(survivors.length === 0, `${where}: child process(es) outlived the app: ${describe(survivors)}`);
  if (killed) return { value: undefined, killed, output, resultPath };

  if (!existsSync(resultPath)) {
    check(false, `${where}: app exited ${result.code} without writing a result\n${shortOutput}`);
    return null;
  }
  let document: unknown;
  try {
    document = JSON.parse(readFileSync(resultPath, "utf8"));
  } catch (error) {
    check(false, `${where}: result is not JSON: ${error instanceof Error ? error.message : String(error)}`);
    return null;
  }
  if (!check(field(document, "ok") === true, `${where}: script failed: ${String(field(document, "error") ?? "no error text")}`)) {
    return null;
  }
  if (!check(result.code === 0, `${where}: app exit = ${result.code}\n${shortOutput}`)) return null;
  return { value: field(document, "value"), killed, output, resultPath };
}
