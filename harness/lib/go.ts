import { existsSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { executable, run } from "./process.ts";
import type { RunResult } from "./process.ts";
import { resolve } from "./repo.ts";
import type { GateContext } from "./types.ts";

export function hasGoMod(ctx: GateContext): boolean {
  return existsSync(resolve(ctx.root, "go.mod"));
}

/** Runs `go` with the repository root as cwd. */
export function runGo(ctx: GateContext, args: string[]): Promise<RunResult> {
  return run(executable("go"), args, { cwd: ctx.root });
}

const cliBuilds = new Map<string, Promise<{ binary: string } | { failure: string }>>();

/**
 * Builds `./cmd/passquest` from `ctx.root` once per gate process and root: every later caller with the
 * same root gets the same binary (the build of the tree being gated), a fixture root builds its own.
 * The binary lives in a temp dir that this process removes when it exits; no gate may delete it.
 * A failed build resolves to its output, so each gate keeps its own failure text.
 */
export function buildCLI(ctx: GateContext): Promise<{ binary: string } | { failure: string }> {
  const root = path.resolve(ctx.root);
  let build = cliBuilds.get(root);
  if (!build) {
    build = (async () => {
      const dir = mkdtempSync(path.join(tmpdir(), "passquest-cli-"));
      process.once("exit", () => rmSync(dir, { recursive: true, force: true }));
      const binary = path.join(dir, executable("passquest"));
      const result = await runGo(ctx, ["build", "-o", binary, "./cmd/passquest"]);
      return result.code === 0 ? { binary } : { failure: `${result.stdout}${result.stderr}`.trim() };
    })();
    cliBuilds.set(root, build);
  }
  return build;
}
