/**
 * Pausing a live durable host the way the application itself does, for the two
 * harness callers that need a *paused* run still carrying its durable work:
 * agent-smoke section 9 and the package smoke's brief case.
 *
 * The control file beside the heartbeat is the authoritative pause request on
 * every OS — internal/agent HostControlPause; a POSIX signal is only an extra
 * prompt — so the harness writes that same document, but only once the loopback
 * stub has recorded the run's first provider request. The host submits its
 * durable task before it ever calls the provider, so waiting for that arrival
 * makes "the pause found pending work" true by construction instead of by a
 * race against host startup: the 2026-11-20 goal 0016 review saw the
 * `--timeout-seconds 1` recipe pause runs before they had enqueued anything on
 * a machine at 100% CPU, and a 20-second deadline still left only ~1.2x over
 * the same machine's worst observed slowdown.
 *
 * `--timeout-seconds` stays in the command line as the bound under which the
 * run must reach the provider, so a host that never arrives still fails a gate
 * in bounded time instead of hanging it.
 */
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";

/**
 * `--timeout-seconds` for a run the harness pauses itself: the bound, not the
 * trigger. It covers the whole start measured on a busy machine (~3.7 s) many
 * times over — the review's worst machine ran ~4.5x slower than nominal — while
 * the pause itself lands as soon as the stub records the request.
 */
export const PAUSE_DEADLINE_SECONDS = 60;

/** How often `startPausedRun` re-reads the stub's request list. */
const ARRIVAL_POLL_MS = 25;

/**
 * internal/agent.DefaultStorePath: the Pi store beside the application
 * database. The installed CLI derives it from `--db` and so does the window, so
 * a harness that must name the same store has to mirror that rule.
 */
export function durableStorePath(dbPath: string): string {
  const base = path.basename(dbPath, path.extname(dbPath)) || "passquest";
  return path.join(path.dirname(dbPath), `${base}-agent.sqlite`);
}

/**
 * Start a durable run and pause it once it has reached the provider: waits for
 * the stub's request list to grow past its current length, writes the pause
 * request with pauseLiveHost, and returns the CLI result. A run that ends
 * before any arrival — a fixture binary that never contacts the stub, a host
 * that fails at startup — ends the wait, so a settled run never waits out the
 * deadline.
 */
export async function startPausedRun<T>(
  start: () => Promise<T>,
  requests: { length: number },
  storePath: string,
): Promise<T> {
  const baseline = requests.length;
  const running = start();
  if (await waitForProviderRequest(requests, baseline, running, PAUSE_DEADLINE_SECONDS * 1000)) {
    pauseLiveHost(storePath);
  }
  return running;
}

/** True once the stub's list grew past `baseline`; false when `done` settles or `timeoutMs` passes first. */
async function waitForProviderRequest(
  requests: { length: number },
  baseline: number,
  done: Promise<unknown>,
  timeoutMs: number,
): Promise<boolean> {
  const finished = done.then(() => "finished" as const, () => "finished" as const);
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    if (requests.length > baseline) return true;
    if (Date.now() >= deadline) return false;
    const tick = Bun.sleep(ARRIVAL_POLL_MS).then(() => "tick" as const);
    if ((await Promise.race([tick, finished])) === "finished") return requests.length > baseline;
  }
}

/**
 * Write the pause request the CLI's own deadline writes
 * (internal/agent.WriteHostControl): the run id and host token the live host
 * publishes in its heartbeat, and the action it polls for. A store with no
 * readable heartbeat — the fixture binaries that stand in for an installed
 * app never run one — gets no request; the caller's own checks, not a thrown
 * error, then report the state the run ended in.
 */
function pauseLiveHost(storePath: string): void {
  const heartbeatPath = `${storePath}.host`;
  let heartbeat: { run_id?: unknown; token?: unknown };
  try {
    heartbeat = JSON.parse(readFileSync(heartbeatPath, "utf8")) as { run_id?: unknown; token?: unknown };
  } catch {
    return;
  }
  if (typeof heartbeat.run_id !== "string" || heartbeat.run_id === "" || typeof heartbeat.token !== "string") return;
  writeFileSync(
    `${heartbeatPath}.control`,
    JSON.stringify({ run_id: heartbeat.run_id, host_token: heartbeat.token, action: "pause" }),
  );
}
