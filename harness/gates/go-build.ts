import { hasGoMod, runGo } from "../lib/go.ts";
import { outputLines } from "../lib/process.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate } from "../lib/types.ts";

export const goBuild: Gate = {
  id: "go-build",
  title: "go build",
  async run(ctx) {
    if (!hasGoMod(ctx)) return skip("no go.mod yet");
    const result = await runGo(ctx, ["build", "./..."]);
    if (result.code !== 0) return fail(outputLines(result));
    return pass();
  },
};
