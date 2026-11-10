import { hasGoMod, runGo } from "../lib/go.ts";
import { outputLines } from "../lib/process.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate } from "../lib/types.ts";

export const goTest: Gate = {
  id: "go-test",
  title: "go test",
  async run(ctx) {
    if (!hasGoMod(ctx)) return skip("no go.mod yet");
    const result = await runGo(ctx, ["test", "./...", "-count=1"]);
    if (result.code !== 0) return fail(outputLines(result));
    const packages = `${result.stdout}${result.stderr}`
      .split(/\r?\n/)
      .filter((line) => line.startsWith("ok ") || line.startsWith("? ")).length;
    return pass(`${packages} packages, 0 failures`);
  },
};
