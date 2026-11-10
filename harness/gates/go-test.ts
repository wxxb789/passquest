import { existsSync } from "node:fs";

import { executable, outputLines, run } from "../lib/process.ts";
import { resolve } from "../lib/repo.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate } from "../lib/types.ts";

export const goTest: Gate = {
  id: "go-test",
  title: "go test",
  async run(ctx) {
    if (!existsSync(resolve(ctx.root, "go.mod"))) return skip("no go.mod yet");
    const result = await run(executable("go"), ["test", "./...", "-count=1"], { cwd: ctx.root });
    if (result.code !== 0) return fail(outputLines(result));
    const packages = `${result.stdout}${result.stderr}`
      .split(/\r?\n/)
      .filter((line) => line.startsWith("ok ") || line.startsWith("? ")).length;
    return pass(`${packages} packages, 0 failures`);
  },
};
