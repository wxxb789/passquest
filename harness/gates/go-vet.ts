import { existsSync } from "node:fs";

import { executable, outputLines, run } from "../lib/process.ts";
import { resolve } from "../lib/repo.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate } from "../lib/types.ts";

export const goVet: Gate = {
  id: "go-vet",
  title: "go vet",
  async run(ctx) {
    if (!existsSync(resolve(ctx.root, "go.mod"))) return skip("no go.mod yet");
    const result = await run(executable("go"), ["vet", "./..."], { cwd: ctx.root });
    if (result.code !== 0) return fail(outputLines(result));
    return pass();
  },
};
