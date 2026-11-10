import { existsSync, readFileSync } from "node:fs";
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

/**
 * Lists the files a pre-commit gate can read: tracked paths plus untracked,
 * non-ignored paths, keeping only files that exist in the working tree. New
 * deliverables are scanned before they are staged, and a pending move does not
 * make the old path an unreadable failure.
 */
export async function gitTrackedFiles(root: string): Promise<string[]> {
  const result = await run(executable("git"), ["ls-files", "-z", "--cached", "--others", "--exclude-standard"], {
    cwd: root,
  });
  if (result.code !== 0) {
    throw new Error(`git ls-files failed: ${result.stderr || result.stdout}`);
  }
  const seen = new Set<string>();
  const files: string[] = [];
  for (const file of result.stdout.split("\0").filter(Boolean)) {
    if (seen.has(file) || !existsSync(resolve(root, file))) continue;
    seen.add(file);
    files.push(file);
  }
  return files;
}

/** A normal context walks git; tests can supply their own tracked list. */
export function context(root: string, tracked?: string[]): GateContext {
  if (tracked) {
    return { root, trackedFiles: async () => tracked };
  }
  return { root, trackedFiles: () => gitTrackedFiles(root) };
}
