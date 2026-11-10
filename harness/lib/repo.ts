import { readFileSync } from "node:fs";
import path from "node:path";

import { executable, run } from "./process.ts";
import type { GateContext } from "./types.ts";

export function resolve(root: string, relative: string): string {
  return path.join(root, ...relative.split("/"));
}

export function readText(root: string, relative: string): string {
  return readFileSync(resolve(root, relative), "utf8");
}

export function readBytes(root: string, relative: string): Buffer {
  return readFileSync(resolve(root, relative));
}

export async function gitTrackedFiles(root: string): Promise<string[]> {
  const result = await run(executable("git"), ["ls-files", "-z"], { cwd: root });
  if (result.code !== 0) {
    throw new Error(`git ls-files failed: ${result.stderr || result.stdout}`);
  }
  return result.stdout.split("\0").filter(Boolean);
}

/** A normal context walks git; tests can supply their own tracked list. */
export function context(root: string, tracked?: string[]): GateContext {
  if (tracked) {
    return { root, trackedFiles: async () => tracked };
  }
  return { root, trackedFiles: () => gitTrackedFiles(root) };
}
