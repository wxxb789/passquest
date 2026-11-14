/**
 * Bundle, build and launch helpers for the desktop app, shared by the
 * desktop-smoke gate and `bun run desktop:build`. Every launch is bounded by a
 * hard watchdog: a webview that cannot start leaves the process hanging (a
 * reused WebView2 profile does exactly that), so a missing deadline would stall
 * the gate until the CI job limit.
 */
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { runGo } from "./go.ts";
import { executable, outputLines, run } from "./process.ts";
import type { RunResult } from "./process.ts";
import { resolve } from "./repo.ts";
import type { GateContext } from "./types.ts";

export const DESKTOP_PACKAGE = "./cmd/passquest-desktop";
export const FRONTEND_ENTRY = "desktop/web/src/main.ts";
/** The go:embed root's generated bundle; gitignored by the global `dist/` rule. */
export const FRONTEND_DIST = "internal/desktop/web/dist";

/** Bundles the frontend with the harness's own Bun; no shell is involved. */
export function bundleFrontend(root: string, outDir: string): Promise<RunResult> {
  return run(process.execPath, ["build", FRONTEND_ENTRY, "--outdir", outDir, "--target", "browser", "--minify"], {
    cwd: root,
  });
}

export interface BuildResult {
  ok: boolean;
  failures: string[];
}

/**
 * Bundles into the embed root, replacing any stale bundle, then links the
 * desktop binary so the bundle it embeds is the one just built.
 */
export async function buildDesktop(ctx: GateContext, outBinary: string): Promise<BuildResult> {
  const dist = resolve(ctx.root, FRONTEND_DIST);
  rmSync(dist, { recursive: true, force: true });
  const bundle = await bundleFrontend(ctx.root, dist);
  if (bundle.code !== 0) {
    return { ok: false, failures: [`bun build ${FRONTEND_ENTRY} failed (exit ${bundle.code})`, ...outputLines(bundle)] };
  }
  const build = await runGo(ctx, ["build", "-o", outBinary, DESKTOP_PACKAGE]);
  if (build.code !== 0) {
    return { ok: false, failures: [`go build ${DESKTOP_PACKAGE} failed (exit ${build.code})`, ...outputLines(build)] };
  }
  return { ok: true, failures: [] };
}

export interface LaunchOptions {
  binary: string;
  args?: string[];
  /** The complete environment of the launch; it is not merged with this process's. */
  env: Record<string, string | undefined>;
  timeoutMs: number;
  cwd?: string;
}

export interface LaunchResult {
  code: number;
  stdout: string;
  stderr: string;
  timedOut: boolean;
  /** Set when the app could not be started at all (no display, missing binary). */
  error?: string;
}

export interface DesktopRun {
  /** The spawned process: the app itself, or `xvfb-run` when the launch is wrapped. */
  pid: number;
  wrapped: boolean;
  /** Resolves the app's own pid (the wrapper's descendant when wrapped). */
  appPid: () => Promise<number | null>;
  /** The watchdog's kill: ends the whole process tree now; `result` then resolves. */
  kill: () => Promise<void>;
  result: Promise<LaunchResult>;
}

function sleep(ms: number): Promise<void> {
  return new Promise((done) => setTimeout(done, ms));
}

/** Deletes a tree, retrying while a webview child still holds a file open. */
export async function removeTree(dir: string, attempts = 20): Promise<boolean> {
  for (let attempt = 0; attempt < attempts; attempt++) {
    try {
      rmSync(dir, { recursive: true, force: true });
      return true;
    } catch {
      await sleep(250);
    }
  }
  return false;
}

function failedLaunch(error: string): DesktopRun {
  return {
    pid: -1,
    wrapped: false,
    appPid: async () => null,
    kill: async () => {},
    result: Promise.resolve({ code: -1, stdout: "", stderr: "", timedOut: false, error }),
  };
}

/**
 * Starts the app under a watchdog. On Windows each launch gets a fresh
 * WEBVIEW2_USER_DATA_FOLDER unless `env` names one: WebView2 helpers can still
 * write to the profile shortly after the host exits, so a caller that waits for
 * them passes a folder it removes itself. On Linux without a display the launch
 * is wrapped in `xvfb-run -a`. Output goes to files, not pipes, so a webview child that
 * inherits the handles cannot keep the result pending after the app exits.
 */
export function startDesktop(options: LaunchOptions): DesktopRun {
  const scratch = mkdtempSync(path.join(tmpdir(), "passquest-desktop-launch-"));
  const env: Record<string, string | undefined> = { ...options.env };
  if (process.platform === "win32") {
    env.WEBVIEW2_USER_DATA_FOLDER ??= path.join(scratch, "webview2");
  }
  let cmd = [options.binary, ...(options.args ?? [])];
  let wrapped = false;
  if (process.platform === "linux" && !env.DISPLAY && !env.WAYLAND_DISPLAY) {
    const xvfb = Bun.which("xvfb-run", { PATH: env.PATH ?? "" });
    if (!xvfb) {
      rmSync(scratch, { recursive: true, force: true });
      return failedLaunch("no DISPLAY or WAYLAND_DISPLAY and xvfb-run is not installed (apt-get install xvfb)");
    }
    cmd = [xvfb, "-a", ...cmd];
    wrapped = true;
  }
  const stdoutPath = path.join(scratch, "stdout.txt");
  const stderrPath = path.join(scratch, "stderr.txt");
  let proc: Bun.Subprocess;
  try {
    proc = Bun.spawn({
      cmd,
      cwd: options.cwd,
      env,
      stdin: "ignore",
      stdout: Bun.file(stdoutPath),
      stderr: Bun.file(stderrPath),
    });
  } catch (error) {
    rmSync(scratch, { recursive: true, force: true });
    return failedLaunch(error instanceof Error ? error.message : String(error));
  }
  const pid = proc.pid;
  const appPid = async (): Promise<number | null> => {
    if (!wrapped) return pid;
    const table = await listProcesses();
    if ("error" in table) return null;
    const name = path.basename(options.binary);
    // argv[0] only: the wrapper's own shells carry the binary path as an argument.
    const app = descendants(table.items, pid).find((entry) => path.basename(entry.command.split(" ")[0]) === name);
    return app ? app.pid : null;
  };
  const kill = async (): Promise<void> => {
    await killTree(pid);
    proc.kill("SIGKILL");
  };
  const result = (async (): Promise<LaunchResult> => {
    let timedOut = false;
    const watchdog = setTimeout(() => {
      timedOut = true;
      void kill();
    }, options.timeoutMs);
    const code = await proc.exited;
    clearTimeout(watchdog);
    const read = (file: string) => {
      try {
        return readFileSync(file, "utf8");
      } catch {
        return "";
      }
    };
    const stdout = read(stdoutPath);
    const stderr = read(stderrPath);
    await removeTree(scratch);
    return { code, stdout, stderr, timedOut };
  })();
  return { pid, wrapped, appPid, kill, result };
}

export function launchDesktop(options: LaunchOptions): Promise<LaunchResult> {
  return startDesktop(options).result;
}

export interface ProcessInfo {
  pid: number;
  ppid: number;
  /** Executable name on Windows; the full argument string on POSIX. */
  command: string;
}

export interface ListeningSocket {
  pid: number;
  address: string;
  port: number;
}

/** A probe either lists what it saw or says why it could not look. */
export type Probe<T> = { items: T[] } | { error: string };

function powershell(script: string): Promise<RunResult> {
  return run(executable("powershell"), ["-NoProfile", "-NonInteractive", "-Command", script]);
}

function jsonRows(text: string): unknown[] {
  const trimmed = text.trim();
  if (trimmed === "") return [];
  const parsed: unknown = JSON.parse(trimmed);
  return Array.isArray(parsed) ? parsed : [parsed];
}

function numberOf(row: unknown, key: string): number {
  const value = row !== null && typeof row === "object" ? Reflect.get(row, key) : undefined;
  return typeof value === "number" ? value : Number(value);
}

function stringOf(row: unknown, key: string): string {
  const value = row !== null && typeof row === "object" ? Reflect.get(row, key) : undefined;
  return typeof value === "string" ? value : String(value ?? "");
}

/** The whole process table: Win32_Process on Windows, `ps` on Linux and macOS. */
export async function listProcesses(): Promise<Probe<ProcessInfo>> {
  if (process.platform === "win32") {
    const result = await powershell(
      "Get-CimInstance Win32_Process | Select-Object ProcessId,ParentProcessId,Name | ConvertTo-Json -Compress",
    );
    if (result.code !== 0) return { error: `Get-CimInstance Win32_Process exit ${result.code}: ${result.stderr.trim()}` };
    try {
      return {
        items: jsonRows(result.stdout).map((row) => ({
          pid: numberOf(row, "ProcessId"),
          ppid: numberOf(row, "ParentProcessId"),
          command: stringOf(row, "Name"),
        })),
      };
    } catch (error) {
      return { error: `Win32_Process output is not JSON: ${error instanceof Error ? error.message : String(error)}` };
    }
  }
  const result = await run("ps", ["-A", "-o", "pid=,ppid=,args="]);
  if (result.code !== 0) return { error: `ps exit ${result.code}: ${result.stderr.trim()}` };
  const items: ProcessInfo[] = [];
  for (const line of result.stdout.split(/\r?\n/)) {
    const match = /^\s*(\d+)\s+(\d+)\s+(.*)$/.exec(line);
    if (match) items.push({ pid: Number(match[1]), ppid: Number(match[2]), command: match[3] });
  }
  return { items };
}

/** Every transitive child of `root` in a process table snapshot. */
export function descendants(table: ProcessInfo[], root: number): ProcessInfo[] {
  const found: ProcessInfo[] = [];
  const frontier = [root];
  const seen = new Set<number>([root]);
  while (frontier.length > 0) {
    const parent = frontier.pop() as number;
    for (const entry of table) {
      if (entry.ppid !== parent || seen.has(entry.pid)) continue;
      seen.add(entry.pid);
      found.push(entry);
      frontier.push(entry.pid);
    }
  }
  return found;
}

/**
 * Child processes of `pid`. Windows keeps a dead parent's id on its children,
 * so this also finds orphans after the app exited; POSIX reparents them, so a
 * caller that needs orphans there compares against a snapshot taken while the
 * app was running (see `stillRunning`).
 */
export async function childProcesses(pid: number): Promise<Probe<ProcessInfo>> {
  const table = await listProcesses();
  if ("error" in table) return table;
  return { items: descendants(table.items, pid) };
}

/** The entries of an earlier snapshot that still run with the same pid and command. */
export async function stillRunning(snapshot: ProcessInfo[]): Promise<Probe<ProcessInfo>> {
  if (snapshot.length === 0) return { items: [] };
  const table = await listProcesses();
  if ("error" in table) return table;
  return {
    items: snapshot.filter((old) => table.items.some((now) => now.pid === old.pid && now.command === old.command)),
  };
}

/** Listening TCP sockets owned by any of `pids`. */
export async function listeningSockets(pids: number[]): Promise<Probe<ListeningSocket>> {
  if (pids.length === 0) return { items: [] };
  if (process.platform === "win32") {
    // The cmdlet reports "no matching connection" as an ObjectNotFound error
    // (exit 1); that is the empty list. Any other error is a failed probe.
    const script = [
      "if (-not (Get-Command Get-NetTCPConnection -ErrorAction SilentlyContinue)) { [Console]::Error.WriteLine('Get-NetTCPConnection is not available'); exit 3 }",
      "$rows = @()",
      `try { $rows = @(Get-NetTCPConnection -State Listen -OwningProcess ${pids.join(",")} -ErrorAction Stop) } ` +
        "catch { if ($_.CategoryInfo.Category -ne 'ObjectNotFound') { [Console]::Error.WriteLine($_.Exception.Message); exit 4 } }",
      "$rows | Select-Object OwningProcess,LocalAddress,LocalPort | ConvertTo-Json -Compress",
    ].join("; ");
    const result = await powershell(script);
    if (result.code !== 0) return { error: `Get-NetTCPConnection exit ${result.code}: ${result.stderr.trim()}` };
    try {
      return {
        items: jsonRows(result.stdout).map((row) => ({
          pid: numberOf(row, "OwningProcess"),
          address: stringOf(row, "LocalAddress"),
          port: numberOf(row, "LocalPort"),
        })),
      };
    } catch (error) {
      return { error: `Get-NetTCPConnection output is not JSON: ${error instanceof Error ? error.message : String(error)}` };
    }
  }
  if (process.platform === "darwin") {
    const items: ListeningSocket[] = [];
    for (const pid of pids) {
      const result = await run("lsof", ["-nP", "-iTCP", "-sTCP:LISTEN", "-a", "-p", String(pid), "-F", "n"]);
      // lsof exits 1 with no output when the process has no matching socket.
      if (result.code === 1 && result.stdout.trim() === "" && result.stderr.trim() === "") continue;
      if (result.code !== 0) return { error: `lsof exit ${result.code}: ${result.stderr.trim()}` };
      for (const line of result.stdout.split(/\r?\n/)) {
        if (!line.startsWith("n")) continue;
        const name = line.slice(1);
        const colon = name.lastIndexOf(":");
        items.push({ pid, address: name.slice(0, colon), port: Number(name.slice(colon + 1)) });
      }
    }
    return { items };
  }
  const result = await run("ss", ["-H", "-ltnp"]);
  if (result.code !== 0) return { error: `ss exit ${result.code}: ${result.stderr.trim()}` };
  const items: ListeningSocket[] = [];
  for (const line of result.stdout.split(/\r?\n/)) {
    const columns = line.trim().split(/\s+/);
    if (columns.length < 4) continue;
    const local = columns[3];
    const colon = local.lastIndexOf(":");
    for (const match of line.matchAll(/pid=(\d+)/g)) {
      const pid = Number(match[1]);
      if (pids.includes(pid)) items.push({ pid, address: local.slice(0, colon), port: Number(local.slice(colon + 1)) });
    }
  }
  return { items };
}

/** Kills a process and everything below it; best effort, never throws. */
export async function killTree(pid: number): Promise<void> {
  if (process.platform === "win32") {
    await run(executable("taskkill"), ["/PID", String(pid), "/T", "/F"]);
    return;
  }
  const table = await listProcesses();
  const victims = "error" in table ? [] : descendants(table.items, pid).map((entry) => entry.pid);
  for (const victim of [...victims.reverse(), pid]) {
    try {
      process.kill(victim, "SIGKILL");
    } catch {
      // Already gone.
    }
  }
}
