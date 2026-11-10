import { hasGoMod, runGo } from "../lib/go.ts";
import { outputLines } from "../lib/process.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate } from "../lib/types.ts";

export const goTidy: Gate = {
  id: "go-tidy",
  title: "go mod tidy",
  async run(ctx) {
    if (!hasGoMod(ctx)) return skip("no go.mod yet");
    const result = await runGo(ctx, ["mod", "tidy", "-diff"]);
    if (result.code !== 0) {
      return fail(["go mod tidy -diff produced a diff:", ...outputLines(result)]);
    }
    return pass();
  },
};
