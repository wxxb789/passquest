import { existsSync, readFileSync } from "node:fs";
import { readFile } from "node:fs/promises";
import path from "node:path";

import { executable, run } from "./process.ts";
import type { GateContext } from "./types.ts";

export function resolve(root: string, relative: string): string {
  return path.join(root, ...relative.split("/"));
}

export function readText(root: string, relative: string): string {
  return readFileSync(resolve(root, relative), "utf8");
}

/** Concurrent reads: one file at a time costs a few ms of open latency each on Windows. */
const READ_CONCURRENCY = 64;

/**
 * Reads every file concurrently, keeping the input order; a file that cannot be
 * read is null. Several hundred sequential synchronous reads are what made the
 * file-scanning gates slow, not the scanning.
 */
export async function readEach(root: string, files: string[]): Promise<Array<Buffer | null>> {
  const contents = new Array<Buffer | null>(files.length).fill(null);
  let next = 0;
  async function worker(): Promise<void> {
    while (next < files.length) {
      const index = next++;
      try {
        contents[index] = await readFile(resolve(root, files[index]));
      } catch {
        // Unreadable: the entry stays null and the caller decides what that means.
      }
    }
  }
  await Promise.all(Array.from({ length: Math.min(READ_CONCURRENCY, files.length) }, worker));
  return contents;
}

/**
 * Lists the files a pre-commit gate can read: tracked paths plus untracked,
 * non-ignored paths, keeping only files that exist in the working tree. New
 * deliverables are scanned before they are staged, and a pending move does not
 * make the old path an unreadable failure.
 */
export async function workingTreeFiles(root: string): Promise<string[]> {
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

/**
 * A normal context walks git once, on the first call, and gives every later
 * gate of the run that snapshot (each caller gets its own copy of the list);
 * files added after the first call are not listed, and a reader must tolerate
 * a listed file that has since been removed. A failed walk is not kept, so the
 * next caller retries. Tests can supply their own working-tree list.
 */
export function context(root: string, files?: string[]): GateContext {
  if (files) {
    return { root, workingTreeFiles: async () => files };
  }
  let walk: Promise<string[]> | null = null;
  return {
    root,
    workingTreeFiles: () => {
      walk ??= workingTreeFiles(root).catch((error: unknown) => {
        walk = null;
        throw error;
      });
      return walk.then((list) => [...list]);
    },
  };
}
