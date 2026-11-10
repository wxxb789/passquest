import { existsSync } from "node:fs";
import path from "node:path";

import { outputLines, run } from "../lib/process.ts";
import { resolve } from "../lib/repo.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate } from "../lib/types.ts";

const repoRoot = path.resolve(import.meta.dir, "../..");
const tsc = path.join(repoRoot, "node_modules", "typescript", "bin", "tsc");

export const typecheck: Gate = {
  id: "typecheck",
  title: "typecheck",
  async run(ctx) {
    if (!existsSync(resolve(ctx.root, "tsconfig.json"))) return skip("no tsconfig.json yet");
    if (!existsSync(tsc)) {
      return fail(["typescript is not installed; run bun install (bun.lock is committed)"]);
    }
    const result = await run(process.execPath, [tsc, "--noEmit", "--project", resolve(ctx.root, "tsconfig.json")], {
      cwd: ctx.root,
    });
    if (result.code !== 0) return fail(outputLines(result));
    return pass("0 type errors");
  },
};
