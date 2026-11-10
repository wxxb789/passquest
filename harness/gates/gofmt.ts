import { executable, outputLines, run } from "../lib/process.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate } from "../lib/types.ts";

export const gofmt: Gate = {
  id: "gofmt",
  title: "gofmt",
  async run(ctx) {
    const files = (await ctx.trackedFiles()).filter((file) => file.endsWith(".go"));
    if (files.length === 0) return skip("no Go files yet");
    const result = await run(executable("gofmt"), ["-l", ...files], { cwd: ctx.root });
    if (result.code !== 0) return fail(outputLines(result));
    const unformatted = result.stdout
      .split(/\r?\n/)
      .map((line) => line.trim())
      .filter(Boolean);
    if (unformatted.length > 0) {
      return fail(["unformatted files:", ...unformatted]);
    }
    return pass(`${files.length} files`);
  },
};
