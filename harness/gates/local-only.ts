import { existsSync } from "node:fs";

import { hasGoMod, runGo } from "../lib/go.ts";
import { outputLines } from "../lib/process.ts";
import { readText, resolve } from "../lib/repo.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate } from "../lib/types.ts";

const NETWORK_IMPORTS = ["net", "net/http", "net/rpc", "net/smtp"];
const LINKED_CLIENTS = ["net/http", "net/rpc", "net/smtp"];
const CONFIDENTIAL_MARKERS = /仅限内部使用|严禁外传/;
const SCANNED_EXTENSIONS = /\.(go|json|md|txt)$/;
/** Desktop frontend source and its embed root: a marker in a bundle input ships inside the binary. */
const DESKTOP_WEB_ROOTS = ["desktop/web/", "internal/desktop/web/"];
const DESKTOP_WEB_EXTENSIONS = /\.(ts|css|html|json|md|txt)$/;
/**
 * The desktop main links MyGo, which itself links net/http, so rule 2 cannot
 * apply to it; its own packages are still held to rule 1 (no direct import).
 */
const DESKTOP_MAIN = "cmd/passquest-desktop";

function scanned(file: string): boolean {
  if ((file.startsWith("cmd/") || file.startsWith("internal/")) && SCANNED_EXTENSIONS.test(file)) return true;
  return DESKTOP_WEB_ROOTS.some((root) => file.startsWith(root)) && DESKTOP_WEB_EXTENSIONS.test(file);
}

export const localOnly: Gate = {
  id: "local-only",
  title: "local-only CLI",
  async run(ctx) {
    if (!hasGoMod(ctx)) return skip("no go.mod yet");
    if (!existsSync(resolve(ctx.root, "cmd/passquest"))) {
      return fail(["cmd/passquest is missing but go.mod exists"]);
    }

    const patterns = ["./cmd/passquest", "./internal/..."];
    if (existsSync(resolve(ctx.root, DESKTOP_MAIN))) patterns.push(`./${DESKTOP_MAIN}/...`);
    const packagesResult = await runGo(ctx, ["list", ...patterns]);
    if (packagesResult.code !== 0) return fail(["go list packages:", ...outputLines(packagesResult)]);
    const packages = packagesResult.stdout.split(/\r?\n/).filter(Boolean);
    if (packages.length === 0) return fail(["no first-party packages found"]);

    const importsResult = await runGo(ctx, [
      "list",
      "-f",
      "{{.ImportPath}}:{{range .Imports}} {{.}}{{end}}",
      ...packages,
    ]);
    if (importsResult.code !== 0) return fail(["go list imports:", ...outputLines(importsResult)]);
    const problems: string[] = [];
    for (const line of importsResult.stdout.split(/\r?\n/).filter(Boolean)) {
      for (const banned of NETWORK_IMPORTS) {
        if (new RegExp(` ${banned}( |$)`).test(line)) {
          problems.push(`first-party network import: ${line}`);
        }
      }
    }

    const depsResult = await runGo(ctx, ["list", "-deps", "./cmd/passquest"]);
    if (depsResult.code !== 0) return fail(["go list -deps ./cmd/passquest:", ...outputLines(depsResult)]);
    for (const dep of depsResult.stdout.split(/\r?\n/).map((line) => line.trim())) {
      if (LINKED_CLIENTS.includes(dep)) problems.push(`network client linked into the CLI: ${dep}`);
    }

    for (const file of (await ctx.workingTreeFiles()).filter(scanned)) {
      let text: string;
      try {
        text = readText(ctx.root, file);
      } catch {
        continue;
      }
      const lines = text.split(/\r?\n/);
      for (let index = 0; index < lines.length; index++) {
        if (CONFIDENTIAL_MARKERS.test(lines[index])) {
          problems.push(`${file}:${index + 1}: confidential source marker`);
        }
      }
    }
    if (problems.length > 0) return fail(problems);
    return pass(`${packages.length} first-party packages, 0 network imports, 0 confidential markers`);
  },
};
