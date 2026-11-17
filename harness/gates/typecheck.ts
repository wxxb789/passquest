import { existsSync } from "node:fs";
import path from "node:path";

import { outputLines, run } from "../lib/process.ts";
import { resolve } from "../lib/repo.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate } from "../lib/types.ts";

const repoRoot = path.resolve(import.meta.dir, "../..");
const tsc = path.join(repoRoot, "node_modules", "typescript", "bin", "tsc");

/**
 * Each TypeScript surface and the tsconfig that must cover it. The harness
 * project is checked once it exists; the desktop frontend has its own project
 * (DOM lib, no Bun types), and its directory without that project is a failure,
 * never a skip.
 */
const PROJECTS = [
  { surface: null, project: "tsconfig.json" },
  { surface: "desktop/web", project: "desktop/web/tsconfig.json" },
] as const;

export const typecheck: Gate = {
  id: "typecheck",
  title: "typecheck",
  async run(ctx) {
    const projects: string[] = [];
    const problems: string[] = [];
    for (const { surface, project } of PROJECTS) {
      if (existsSync(resolve(ctx.root, project))) {
        projects.push(project);
      } else if (surface !== null && existsSync(resolve(ctx.root, surface))) {
        problems.push(`${surface} exists but ${project} is missing`);
      }
    }
    if (projects.length === 0 && problems.length === 0) return skip("no tsconfig.json yet");
    if (projects.length > 0 && !existsSync(tsc)) {
      return fail(["typescript is not installed; run bun install (bun.lock is committed)", ...problems]);
    }
    // The projects are independent; check them concurrently and report in declaration order.
    const results = await Promise.all(
      projects.map((project) =>
        run(process.execPath, [tsc, "--noEmit", "--project", resolve(ctx.root, project)], { cwd: ctx.root }),
      ),
    );
    for (const [index, result] of results.entries()) {
      if (result.code !== 0) problems.push(`${projects[index]}:`, ...outputLines(result));
    }
    if (problems.length > 0) return fail(problems);
    return pass(`0 type errors in ${projects.join(", ")}`);
  },
};
