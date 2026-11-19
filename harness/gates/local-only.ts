import { existsSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { hasGoMod, runGo } from "../lib/go.ts";
import { executable, outputLines, run } from "../lib/process.ts";
import { readEach, resolve } from "../lib/repo.ts";
import { UPDATE_ENDPOINT, UPDATE_RELEASES_PREFIX } from "../lib/update-endpoint.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";

/**
 * First-party network imports: `net` and every `net/...` sub-package. The
 * exact strings this list held before missed sub-packages like
 * `net/http/cookiejar` and `net/http/httputil`, which dial like `net/http`
 * does; the whole `net` tree is covered now. The two pure sub-packages below
 * are an explicit allowlist of what the tree already imports: they open no
 * socket, resolve no name and dial nothing - they parse addresses and URLs.
 * A new pure use is a deliberate line in this list, so a dialing sub-package
 * can never ride in on a looser pattern.
 */
const PURE_NET_IMPORTS = ["net/netip", "net/url"];
/**
 * Rule 2: the network clients `./cmd/passquest` must never link, directly or
 * through any dependency, plus the allowed network package itself. Known
 * limit: `net` is already linked into the CLI by github.com/google/uuid and
 * golang.org/x/sys/windows, so the rule proves the CLI links no http/rpc/smtp
 * client, not that it links no network package; goal 0015's evidence (P5)
 * discloses it.
 */
const LINKED_CLIENTS = ["net/http", "net/rpc", "net/smtp"];
/**
 * Goal 0015: the one package allowed to reach the network, through its one
 * import, for the one release check the learner asks for. The exception is
 * exact: only this module-relative path gets it, no sub-package, and a
 * self-test case proves the sub-package does not inherit it. Rule 2 keeps
 * the package out of the CLI's transitive dependencies, rule 3 keeps the
 * list of packages allowed to import it to the desktop surface, and rule 4
 * pins its http(s) literals - read by the go/parser tool below - to the
 * constant endpoint and the release-page prefix.
 */
const ALLOWED_NETWORK_PACKAGE = "internal/update";
const ALLOWED_NETWORK_IMPORTS = ["net/http"];
/** The only packages that may import the allowed network package (rule 3): the window and its main. */
const UPDATE_IMPORTERS = ["internal/desktop", "cmd/passquest-desktop"];
/**
 * Rule 4: the constant endpoint and the release-page prefix are the only
 * http(s) literals the allowed package's non-test Go files may hold, nested
 * packages included. Both come from harness/lib/update-endpoint.ts, the one
 * definition the update stub binds the desktop scenario to as well, and both
 * are compared exactly, so another host or a drifted path is a named
 * failure. That comparison reads the shared file, so on its own an edit of
 * the shared file would move the check with it; the pinned literals below
 * are the gate's own copy of what the two constants must say.
 */
const UPDATE_ENDPOINT_SOURCE = "harness/lib/update-endpoint.ts";

/**
 * The pinned shape of the two constants in harness/lib/update-endpoint.ts:
 * the protocol `https:`, the hosts `api.github.com` and `github.com`, the
 * paths `/repos/wxxb789/passquest/releases/latest` and
 * `/wxxb789/passquest/releases/`, and no query or fragment on either. One
 * problem per drifted part, naming what moved. Exported because a fixture
 * cannot rewrite a harness module: the self-test checks it directly, with a
 * URL of its own.
 */
export function endpointDriftProblems(endpoint: string = UPDATE_ENDPOINT, prefix: string = UPDATE_RELEASES_PREFIX): string[] {
  const problems: string[] = [];
  const pinned: Array<{ name: string; value: string; protocol: string; host: string; pathname: string }> = [
    { name: "UPDATE_ENDPOINT", value: endpoint, protocol: "https:", host: "api.github.com", pathname: "/repos/wxxb789/passquest/releases/latest" },
    { name: "UPDATE_RELEASES_PREFIX", value: prefix, protocol: "https:", host: "github.com", pathname: "/wxxb789/passquest/releases/" },
  ];
  for (const { name, value, protocol, host, pathname } of pinned) {
    let url: URL;
    try {
      url = new URL(value);
    } catch {
      problems.push(`${UPDATE_ENDPOINT_SOURCE}: ${name} is not a URL: ${JSON.stringify(value)}`);
      continue;
    }
    if (url.protocol !== protocol) problems.push(`${UPDATE_ENDPOINT_SOURCE}: ${name} protocol drifted: ${url.protocol}, want ${protocol}`);
    if (url.host !== host) problems.push(`${UPDATE_ENDPOINT_SOURCE}: ${name} host drifted: ${url.host}, want ${host}`);
    if (url.pathname !== pathname) problems.push(`${UPDATE_ENDPOINT_SOURCE}: ${name} pathname drifted: ${url.pathname}, want ${pathname}`);
    if (url.search !== "" || url.hash !== "") problems.push(`${UPDATE_ENDPOINT_SOURCE}: ${name} carries a query or fragment: ${JSON.stringify(value)}`);
  }
  return problems;
}

/** The go/parser tool (harness/tools/urllits) that reports the real http(s) literals. */
const URL_LITERAL_TOOL = fileURLToPath(new URL("../tools/urllits/main.go", import.meta.url));
/** `<file>:<line>: "<literal>"`, tolerating a Windows drive letter in the path. */
const URL_LITERAL_LINE = /^(?:[A-Za-z]:)?[^:]+:\d+: (".*")$/;
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
  title: "local-only CLI (one allowed network package)",
  async run(ctx) {
    if (!hasGoMod(ctx)) return skip("no go.mod yet");
    if (!existsSync(resolve(ctx.root, "cmd/passquest"))) {
      return fail(["cmd/passquest is missing but go.mod exists"]);
    }

    // Rules 1 and 3 cover every package of the module: `./...` lists the two
    // mains, every internal package, the harness tools and any new cmd/<x>,
    // so none of them can import around the rules.
    const patterns = ["./..."];
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

    const problems: string[] = endpointDriftProblems();
    const allowedNetwork = new Set<string>();
    for (const { importPath, module, imports } of packages) {
      const relative = module === "" ? "" : importPath.slice(module.length + 1);
      const allowedHere = relative === ALLOWED_NETWORK_PACKAGE ? ALLOWED_NETWORK_IMPORTS : [];
      for (const name of imports) {
        if (name !== "net" && !name.startsWith("net/")) continue;
        if (PURE_NET_IMPORTS.includes(name)) continue;
        if (allowedHere.includes(name)) {
          allowedNetwork.add(relative);
          continue;
        }
        problems.push(`first-party network import: ${importPath}: ${name}`);
      }
      if (module === "") continue;
      if (imports.includes(`${module}/${ALLOWED_NETWORK_PACKAGE}`) && !UPDATE_IMPORTERS.includes(relative)) {
        problems.push(`internal/update may be imported only by ${UPDATE_IMPORTERS.join(" and ")}: ${relative} imports it`);
      }
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

    // Rule 2 holds for the CLI alone: the desktop main links MyGo, which
    // links net/http, so its dependency tree can never satisfy the rule;
    // every package, the desktop's included, is still held to rule 1.
    const cli = packages.find(({ importPath, module }) => module !== "" && importPath === `${module}/cmd/passquest`);
    if (!cli) return fail(["go list packages:", "./cmd/passquest is not in the listed packages"]);
    const cliClients = new Set([...LINKED_CLIENTS, `${cli.module}/${ALLOWED_NETWORK_PACKAGE}`]);
    for (const dep of cli.deps) {
      if (cliClients.has(dep)) problems.push(`network client linked into the CLI: ${dep}`);
    }

    // Rule 4: the constant endpoint and the release-page prefix are the only http(s) literals
    // the allowed package's non-test Go files may hold - the whole subtree, nested packages
    // included - and the endpoint appears exactly once.
    if (packages.some(({ importPath, module }) => module !== "" && importPath.slice(module.length + 1) === ALLOWED_NETWORK_PACKAGE)) {
      const prefix = `${ALLOWED_NETWORK_PACKAGE}/`;
      const literalFiles = files.filter(
        (file) =>
          file.startsWith(prefix) &&
          file.endsWith(".go") &&
          !file.endsWith("_test.go") &&
          existsSync(resolve(ctx.root, file)),
      );
      if (literalFiles.length === 0) {
        problems.push("internal/update: no non-test Go file in the working tree to read the constant endpoint from");
      } else {
        const scan = await run(executable("go"), ["run", URL_LITERAL_TOOL, "-files"], {
          cwd: ctx.root,
          stdin: `${literalFiles.join("\n")}\n`,
        });
        if (scan.code !== 0) {
          // A read failure is one more problem: the findings above and the
          // marker scan below are reported with it, never instead of it.
          problems.push("http(s) literals of internal/update could not be read:", ...outputLines(scan));
        } else {
          const quotedEndpoint = JSON.stringify(UPDATE_ENDPOINT);
          const quotedPrefix = JSON.stringify(UPDATE_RELEASES_PREFIX);
          let endpoints = 0;
          for (const line of outputLines(scan)) {
            const quoted = URL_LITERAL_LINE.exec(line)?.[1];
            if (quoted === undefined) continue;
            if (quoted === quotedEndpoint) {
              endpoints++;
              continue;
            }
            if (quoted === quotedPrefix) continue;
            problems.push(`internal/update: http(s) literal that is neither the constant endpoint nor the release-page prefix: ${line}`);
          }
          if (endpoints !== 1) {
            problems.push(`internal/update: expected exactly one ${quotedEndpoint} literal in its non-test Go files, saw ${endpoints}`);
          }
        }
      }
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
    // No drift problem above, so the shared constant parses and names the host the pass line shows.
    const endpointHost = new URL(UPDATE_ENDPOINT).host;
    const network = allowedNetwork.size === 0
      ? "0 network imports"
      : `${allowedNetwork.size} allowed network package: ${[...allowedNetwork].join(", ")} -> ${endpointHost}`;
    return pass(`${packages.length} first-party packages, ${network}, 0 confidential markers, 0 boundary violations`);
  },
};
