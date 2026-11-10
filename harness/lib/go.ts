import { existsSync } from "node:fs";

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
