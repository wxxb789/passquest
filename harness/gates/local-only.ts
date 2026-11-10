import { existsSync } from "node:fs";

import { executable, outputLines, run } from "../lib/process.ts";
import { readText, resolve } from "../lib/repo.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate } from "../lib/types.ts";

const NETWORK_IMPORTS = ["net", "net/http", "net/rpc", "net/smtp"];
const LINKED_CLIENTS = ["net/http", "net/rpc", "net/smtp"];
const CONFIDENTIAL_MARKERS = /仅限内部使用|严禁外传/;
const SCANNED_EXTENSIONS = /\.(go|json|md|txt)$/;

export const localOnly: Gate = {
  id: "local-only",
  title: "local-only CLI",
  async run(ctx) {
    if (!existsSync(resolve(ctx.root, "go.mod"))) return skip("no go.mod yet");
    if (!existsSync(resolve(ctx.root, "cmd/passquest"))) {
      return fail(["cmd/passquest is missing but go.mod exists"]);
    }

    const packagesResult = await run(executable("go"), ["list", "./cmd/passquest", "./internal/..."], { cwd: ctx.root });
    if (packagesResult.code !== 0) return fail(["go list packages:", ...outputLines(packagesResult)]);
    const packages = packagesResult.stdout.split(/\r?\n/).filter(Boolean);
    if (packages.length === 0) return fail(["no first-party packages found"]);

    const importsResult = await run(
      executable("go"),
      ["list", "-f", "{{.ImportPath}}:{{range .Imports}} {{.}}{{end}}", ...packages],
      { cwd: ctx.root },
    );
    if (importsResult.code !== 0) return fail(["go list imports:", ...outputLines(importsResult)]);
    const problems: string[] = [];
    for (const line of importsResult.stdout.split(/\r?\n/).filter(Boolean)) {
      for (const banned of NETWORK_IMPORTS) {
        if (new RegExp(` ${banned}( |$)`).test(line)) {
          problems.push(`first-party network import: ${line}`);
        }
      }
    }

    const depsResult = await run(executable("go"), ["list", "-deps", "./cmd/passquest"], { cwd: ctx.root });
    if (depsResult.code !== 0) return fail(["go list -deps ./cmd/passquest:", ...outputLines(depsResult)]);
    for (const dep of depsResult.stdout.split(/\r?\n/).map((line) => line.trim())) {
      if (LINKED_CLIENTS.includes(dep)) problems.push(`network client linked into the CLI: ${dep}`);
    }

    let markers = 0;
    const scanned = (await ctx.trackedFiles()).filter(
      (file) => (file.startsWith("cmd/") || file.startsWith("internal/")) && SCANNED_EXTENSIONS.test(file),
    );
    for (const file of scanned) {
      let text: string;
      try {
        text = readText(ctx.root, file);
      } catch {
        continue;
      }
      const lines = text.split(/\r?\n/);
      for (let index = 0; index < lines.length; index++) {
        if (CONFIDENTIAL_MARKERS.test(lines[index])) {
          markers++;
          problems.push(`${file}:${index + 1}: confidential source marker`);
        }
      }
    }
    if (problems.length > 0) return fail(problems.slice(0, 40));
    return pass(`${packages.length} first-party packages, 0 network imports, 0 confidential markers`);
  },
};
