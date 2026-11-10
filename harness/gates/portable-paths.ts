import { fileURLToPath } from "node:url";

import { executable, outputLines, run } from "../lib/process.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate } from "../lib/types.ts";

/**
 * A path literal that only resolves on POSIX is a Windows defect waiting for CI.
 *
 * The scan is delegated to `harness/tools/pathtypo`, which asks go/parser for the
 * real string literals: a rune holding a backtick, a comment quoting a path, or SQL
 * inside a raw string cannot be mistaken for code. A fixture that is genuinely pure
 * string data carries `// portable-paths: allow <why>` on the literal's own line,
 * and that marker covers that line only.
 */
const TOOL = fileURLToPath(new URL("../tools/pathtypo/main.go", import.meta.url));

/** `<file>:<line>: "<literal>"`, tolerating a Windows drive letter in the path. */
function isHit(line: string): boolean {
  return /^(?:[A-Za-z]:)?[^:]+:\d+: /.test(line);
}

export const portablePaths: Gate = {
  id: "portable-paths",
  title: "portable paths",
  async run(ctx) {
    const files = (await ctx.workingTreeFiles()).filter((file) => file.endsWith(".go"));
    if (files.length === 0) return fail(["no Go files in the working tree"]);
    const result = await run(executable("go"), ["run", TOOL, "-files"], {
      cwd: ctx.root,
      stdin: `${files.join("\n")}\n`,
    });
    const output = outputLines(result);
    if (result.code === 2) return fail(["pathtypo could not read its input:", ...output]);
    const hits = output.filter(isHit);
    if (hits.length === 0) {
      if (result.code !== 0) return fail(["pathtypo failed without naming a literal:", ...output]);
      return pass(`${files.length} Go files, no platform-bound path literal`);
    }
    return fail(
      hits.map(
        (hit) =>
          `${hit} — build it with filepath.Join/os.TempDir/t.TempDir, or mark the literal's own line ` +
          "`// portable-paths: allow <why this is pure string data>`",
      ),
    );
  },
};
