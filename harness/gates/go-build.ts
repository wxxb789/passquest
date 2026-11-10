import { existsSync } from "node:fs";

import { executable, outputLines, run } from "../lib/process.ts";
import { resolve } from "../lib/repo.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate } from "../lib/types.ts";

export const goBuild: Gate = {
  id: "go-build",
  title: "go build",
  async run(ctx) {
    if (!existsSync(resolve(ctx.root, "go.mod"))) return skip("no go.mod yet");
    const result = await run(executable("go"), ["build", "./..."], { cwd: ctx.root });
    if (result.code !== 0) return fail(outputLines(result));
    return pass();
  },
};
