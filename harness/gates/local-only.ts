import { existsSync } from "node:fs";

import { hasGoMod, runGo } from "../lib/go.ts";
import { outputLines } from "../lib/process.ts";
import { readEach, resolve } from "../lib/repo.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";

const NETWORK_IMPORTS = ["net", "net/http", "net/rpc", "net/smtp"];
const LINKED_CLIENTS = ["net/http", "net/rpc", "net/smtp"];
/**
 * Goal 0015: the one package allowed to reach the network, through one
 * import, for the one release check the learner asks for. Rule 1 still
 * covers every other package exactly (the module-relative path, so a
 * sub-package is not covered), and rule 2 keeps the import out of the CLI's
 * transitive dependencies; the pass line below already names the allowed
 * package. The rest of the exception - the endpoint literal and the set of
 * packages allowed to import this one - arrives with the same goal's next
 * step.
 */
const ALLOWED_NETWORK_PACKAGE = "internal/update";
const ALLOWED_NETWORK_IMPORTS = ["net/http"];
/**
 * The internal-only source markers. The texts are exported because the package
 * content scan (gates/package-smoke/contents.ts) searches their UTF-8 bytes in
 * the installed files, binaries included; this gate matches them per line of
 * decoded text through the regex built from the same list.
 */
export const CONFIDENTIAL_MARKER_TEXTS = ["仅限内部使用", "严禁外传"];
export const CONFIDENTIAL_MARKERS = new RegExp(CONFIDENTIAL_MARKER_TEXTS.join("|"));
const SCANNED_EXTENSIONS = /\.(go|json|md|txt)$/;
/** Desktop frontend source and its embed root: a marker in a bundle input ships inside the binary. */
const DESKTOP_WEB_ROOTS = ["desktop/web/", "internal/desktop/web/"];
const DESKTOP_WEB_EXTENSIONS = /\.(ts|css|html|json|md|txt)$/;
/**
 * The desktop main links MyGo, which itself links net/http, so rule 2 cannot
 * apply to it; its own packages are still held to rule 1 (no direct import).
 */
const DESKTOP_MAIN = "cmd/passquest-desktop";

/** Module boundaries (docs/conventions.md): from -> imports it must not have. Paths are module-relative. */
const BOUNDARIES: Array<{ from: string; banned: string[]; why: string }> = [
  { from: "internal/study", banned: ["internal/cli", "internal/studio", "internal/workspace", "internal/desktop"],
    why: "study is the orchestration module behind both adapters" },
  { from: "internal/workspace", banned: ["internal/cli"], why: "the desktop adapter calls study, not the CLI" },
  { from: "internal/desktop", banned: ["internal/cli"], why: "the window reaches the CLI only through cmd/passquest-desktop" },
  { from: "internal/cli", banned: ["internal/workspace", "internal/desktop", "internal/studio"], why: "the CLI adapter never sees desktop DTOs" },
  { from: "internal/cmdline", banned: ["internal/cli", "internal/studio", "internal/workspace", "internal/desktop", "internal/store"],
    why: "cmdline is a leaf both adapters share" },
];
const within = (path: string, root: string) => path === root || path.startsWith(`${root}/`);

/**
 * One `go list` loads the package graph once for every fact the gate needs: each first-party
 * package, its direct imports, and its transitive deps (`.Deps` is what `go list -deps` prints for
 * that package, minus the package itself). The module path picks out `./cmd/passquest`.
 */
const LIST_FORMAT = '{{.ImportPath}}\t{{if .Module}}{{.Module.Path}}{{end}}\t{{join .Imports " "}}\t{{join .Deps " "}}';

function scanned(file: string): boolean {
  if ((file.startsWith("cmd/") || file.startsWith("internal/")) && SCANNED_EXTENSIONS.test(file)) return true;
  return DESKTOP_WEB_ROOTS.some((root) => file.startsWith(root)) && DESKTOP_WEB_EXTENSIONS.test(file);
}

async function readScannedFiles(ctx: GateContext): Promise<{ files: string[]; contents: Array<Buffer | null> }> {
  const files = (await ctx.workingTreeFiles()).filter(scanned);
  return { files, contents: await readEach(ctx.root, files) };
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
    // The marker scan does not depend on the package graph, so its reads overlap the `go list`.
    const [listResult, { files, contents }] = await Promise.all([
      runGo(ctx, ["list", "-f", LIST_FORMAT, ...patterns]),
      readScannedFiles(ctx),
    ]);
    if (listResult.code !== 0) return fail(["go list packages:", ...outputLines(listResult)]);
    const packages = listResult.stdout
      .split(/\r?\n/)
      .filter(Boolean)
      .map((line) => {
        const [importPath, module = "", imports = "", deps = ""] = line.split("\t");
        return { importPath, module, imports: imports.split(" ").filter(Boolean), deps: deps.split(" ").filter(Boolean) };
      });
    if (packages.length === 0) return fail(["no first-party packages found"]);

    const problems: string[] = [];
    const allowedNetwork: string[] = [];
    for (const { importPath, module, imports } of packages) {
      const line = `${importPath}:${imports.map((name) => ` ${name}`).join("")}`;
      const relative = module === "" ? "" : importPath.slice(module.length + 1);
      const allowedHere = relative === ALLOWED_NETWORK_PACKAGE ? ALLOWED_NETWORK_IMPORTS : [];
      for (const banned of NETWORK_IMPORTS) {
        if (!new RegExp(` ${banned}( |$)`).test(line)) continue;
        if (allowedHere.includes(banned)) {
          allowedNetwork.push(relative);
          continue;
        }
        problems.push(`first-party network import: ${line}`);
      }
      if (module === "") continue;
      for (const name of imports) {
        if (!name.startsWith(`${module}/`)) continue;
        const dependency = name.slice(module.length + 1);
        for (const rule of BOUNDARIES) {
          if (within(relative, rule.from) && rule.banned.some((bannedPath) => within(dependency, bannedPath))) {
            problems.push(`module boundary: ${relative} imports ${dependency} (${rule.why})`);
          }
        }
      }
    }

    const cli = packages.find(({ importPath, module }) => module !== "" && importPath === `${module}/cmd/passquest`);
    if (!cli) return fail(["go list packages:", "./cmd/passquest is not in the listed packages"]);
    for (const dep of cli.deps) {
      if (LINKED_CLIENTS.includes(dep)) problems.push(`network client linked into the CLI: ${dep}`);
    }

    for (const [fileIndex, file] of files.entries()) {
      const bytes = contents[fileIndex];
      if (bytes === null) continue;
      const lines = bytes.toString("utf8").split(/\r?\n/);
      for (let index = 0; index < lines.length; index++) {
        if (CONFIDENTIAL_MARKERS.test(lines[index])) {
          problems.push(`${file}:${index + 1}: confidential source marker`);
        }
      }
    }
    if (problems.length > 0) return fail(problems);
    const network = allowedNetwork.length === 0
      ? "0 network imports"
      : `${allowedNetwork.length} allowed network package: ${allowedNetwork.join(", ")}`;
    return pass(`${packages.length} first-party packages, ${network}, 0 confidential markers, 0 boundary violations`);
  },
};
