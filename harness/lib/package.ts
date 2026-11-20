/**
 * Stages the resources a package carries (goal 0013 P2; goal 0016 P1): the
 * agent hosts, the pinned Node 22 LTS runtime and the app icon. A host that
 * imports a package is staged as one self-contained ESM bundle built from a
 * generated entry; a host that imports nothing but `node:` built-ins is
 * copied byte-identically. No package carries node_modules: what the app does
 * not load is not shipped. `mygo.json` lists the tree, so
 * `go tool mygo build` copies it into the app (mygo.PathResources), where
 * internal/hostpaths resolves the hosts and the runtime from.
 *
 * The generated input to a bundle — the entry — and bun's metafile are
 * intermediates in build/package-cache/bundles beside the staged tree: they
 * never ship, and the tree's check fails if anything of the machine that
 * built it — a path, a temp directory, an account name — reached the files
 * that do.
 *
 * The tree lives under build/, which is gitignored: nothing here is committed,
 * and the gates keep building the development app through buildDesktop.
 */
import { createHash } from "node:crypto";
import {
  chmodSync,
  closeSync,
  copyFileSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  openSync,
  readFileSync,
  readSync,
  readdirSync,
  renameSync,
  rmSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { deflateSync } from "node:zlib";

import { stringField } from "./json.ts";
import { executable, run } from "./process.ts";

/** The staged tree, under the gitignored build/ directory. */
export const STAGE_DIR = "build/package";

/** The staged agent directory: the host bundles, the copied hosts, and the third-party notices file. */
export const AGENT_DIR = "agent";

/**
 * Where downloaded archives are kept, so a re-run does not download again. It sits beside the
 * staged tree, not inside it, so nothing that copies the staged tree can carry other platforms' archives.
 */
const CACHE_DIR = "build/package-cache";

/** The Node runtime every package carries. */
export const NODE_VERSION = "22.23.5";

const PLATFORMS = ["win32", "darwin", "linux"] as const;
const ARCHS = ["x64", "arm64"] as const;

/** The systems a package is built for, spelled as Node pins them. */
export type PackagePlatform = (typeof PLATFORMS)[number];
/** The architectures a package is built for, spelled as Node pins them. */
export type PackageArch = (typeof ARCHS)[number];

/** One package target: the system and architecture a build produces its package for. */
export interface PackageTarget {
  platform: PackagePlatform;
  arch: PackageArch;
}

/** The target a `--target` argument names: `<platform>-<arch>`, as in linux-x64. */
export function parseTarget(value: string): PackageTarget {
  const [platform, arch, ...rest] = value.split("-");
  const platforms: readonly string[] = PLATFORMS;
  const arches: readonly string[] = ARCHS;
  if (rest.length > 0 || !platforms.includes(platform) || !arches.includes(arch)) {
    throw new Error(
      `--target ${value}: want <platform>-<arch>, where platform is ${PLATFORMS.join(", ")} and arch is ${ARCHS.join(", ")}`,
    );
  }
  return { platform: platform as PackagePlatform, arch: arch as PackageArch };
}

/**
 * The target of the machine running the build. Only this target's staged
 * runtime can be executed here; a package for another target is staged,
 * built and then run where it ships.
 */
export function hostTarget(): PackageTarget {
  return { platform: process.platform as PackagePlatform, arch: process.arch as PackageArch };
}

/** Go's spelling of a target, as MyGo's `-platform` and Go's GOOS/GOARCH take it. */
export function goTarget(target: PackageTarget): string {
  return `${target.platform === "win32" ? "windows" : target.platform}/${target.arch === "x64" ? "amd64" : target.arch}`;
}

/** The directory MyGo writes a target's output to (mygo.json `out`), relative to the root. */
export function targetDir(target: PackageTarget): string {
  return `build/${goTarget(target).replace("/", "-")}`;
}

/**
 * One platform's Node runtime: the official archive, its executable inside it,
 * where the app looks for that executable, and the archive's SHA-256 as
 * nodejs.org/dist/v<NODE_VERSION>/SHASUMS256.txt publishes it (cross-checked
 * against the downloaded bytes when the pins were written).
 */
export interface NodePin {
  /** process.platform of the target. */
  platform: PackagePlatform;
  /** process.arch of the target. */
  arch: PackageArch;
  /** The archive's file name under nodejs.org/dist/v<NODE_VERSION>/. */
  archive: string;
  /** SHA-256 of that archive. */
  sha256: string;
  /** The executable inside the archive. */
  member: string;
  /** Where it is staged, relative to the resources root. */
  path: string;
}

export const NODE_PINS: readonly NodePin[] = [
  {
    platform: "win32",
    arch: "x64",
    archive: "node-v22.23.5-win-x64.zip",
    sha256: "4a191082e9706866830379d577d2cc8a50c776fcd9436a8d061b063ed0d9a0e3",
    member: "node-v22.23.5-win-x64/node.exe",
    path: "node/node.exe",
  },
  {
    platform: "darwin",
    arch: "arm64",
    archive: "node-v22.23.5-darwin-arm64.tar.gz",
    sha256: "e7131995759b61a0170b646010d2ad35a5d37b4cb0d471f3e8317eac6f742f3d",
    member: "node-v22.23.5-darwin-arm64/bin/node",
    path: "node/bin/node",
  },
  {
    platform: "darwin",
    arch: "x64",
    archive: "node-v22.23.5-darwin-x64.tar.gz",
    sha256: "c53383238206b9ae4db23c5d8a63645e2e753bec8ab51a49cc81939aa64e9422",
    member: "node-v22.23.5-darwin-x64/bin/node",
    path: "node/bin/node",
  },
  {
    platform: "linux",
    arch: "x64",
    archive: "node-v22.23.5-linux-x64.tar.xz",
    sha256: "cdeb8670a0c81f51b4dc4ac61919f9b1ad917b304bf8221eb939570663d240f8",
    member: "node-v22.23.5-linux-x64/bin/node",
    path: "node/bin/node",
  },
];

/** The pin for a platform, or a failure naming the targets that are pinned. */
export function nodePin(platform: string = process.platform, arch: string = process.arch): NodePin {
  const pin = NODE_PINS.find((candidate) => candidate.platform === platform && candidate.arch === arch);
  if (pin) return pin;
  const known = NODE_PINS.map((candidate) => `${candidate.platform}/${candidate.arch}`).join(", ");
  throw new Error(`no Node ${NODE_VERSION} runtime is pinned for ${platform}/${arch}; pinned: ${known}`);
}

/** One entry of the staged tree: a file, or a directory as one line. */
export interface StageEntry {
  /** Path under the stage directory, with slashes. */
  path: string;
  files: number;
  bytes: number;
}

export interface StagedResources {
  /** The absolute stage directory. */
  dir: string;
  /** The staged runtime's absolute path. */
  node: string;
  /** The tree, one entry per file and per folded directory, in path order. */
  entries: StageEntry[];
  files: number;
  bytes: number;
  /** What the checks printed (the lean-tree scan, and where the runtime can run, the staged host's status probe). */
  checks: string[];
}

export interface StageOptions {
  /** Progress lines, one per step; a build log, not gate output. */
  log?: (line: string) => void;
  /** The target to stage for; the host by default. */
  target?: PackageTarget;
}

/**
 * Writes the whole resources tree into build/package. Every host that names a
 * package is bundled into one self-contained ESM file (bundleHost) and every
 * other host is copied byte-identically; the pinned Node runtime comes from
 * the cached archive of the target's platform, and the icon is drawn here. The
 * tree is proved lean (assertLeanAgentTree), and where the target is this
 * machine's, the staged runtime loads the host that carries the packages and
 * answers a `status` request, which is what proves the tree before a build
 * copies it. A package for another target is proved where it runs.
 */
export async function stageResources(root: string, options: StageOptions = {}): Promise<StagedResources> {
  const log = options.log ?? (() => {});
  const target = options.target ?? hostTarget();
  const dir = path.join(root, STAGE_DIR);
  const pin = nodePin(target.platform, target.arch);
  clearStage(dir);
  mkdirSync(dir, { recursive: true });
  log(`target: ${target.platform}-${target.arch}`);

  const hosts = await stageAgent(root, dir);
  if (hosts.length === 0) throw new Error("agent/ holds no host scripts to package");
  const bundled = hosts.filter((host) => host.bundled);
  log(`agent: ${hosts.length} host scripts (${bundled.length} bundled, ${hosts.length - bundled.length} copied byte-identical)`);
  for (const host of bundled) log(`agent/${host.host}: ${host.specifiers.length} specifiers pinned, ${host.bytes} bytes`);
  const node = await stageNodeRuntime(dir, cacheDir(root), pin, log);
  log(`node ${NODE_VERSION}: ${STAGE_DIR}/${pin.path}`);
  const icon = path.join(dir, "icon.png");
  writeFileSync(icon, iconPng());
  log(`icon: ${STAGE_DIR}/icon.png (${statSync(icon).size} bytes)`);

  const checks = [assertLeanAgentTree(path.join(dir, AGENT_DIR), { root }), ...(await checkStagedFor(dir, pin, target))];
  for (const line of checks) log(`check: ${line}`);
  const { entries, files, bytes } = summarize(dir);
  return { dir, node, entries, files, bytes, checks };
}

/**
 * Empties the staged tree (the download cache lives outside it): an entry an older
 * staging layout left would otherwise stay in the listing below while
 * `mygo.json` packs only agent/ and node/, and a temporary directory a failed
 * run left would stay until someone noticed it.
 */
function clearStage(dir: string): void {
  if (!existsSync(dir)) return;
  for (const entry of readdirSync(dir)) {
    rmSync(path.join(dir, entry), { recursive: true, force: true });
  }
}

/** One staged host: the file in the staged tree, and how it got there. */
export interface StagedHost {
  /** The file name under agent/, which a bundle keeps. */
  host: string;
  /** True when the staged file is a self-contained ESM bundle built from a generated entry; false when it is a byte-identical copy. */
  bundled: boolean;
  /** The staged file's size in bytes. */
  bytes: number;
  /** The specifiers the generated entry pinned, in the host's order; empty for a copy. */
  specifiers: string[];
  /** The metafile bun wrote for the bundle (a staging intermediate, never shipped); empty for a copy. */
  metafile: string;
}

/** Where a bundle's metafile is kept: beside the staged tree, so nothing that copies the tree can carry it. */
const BUNDLE_DIR = "bundles";

/**
 * Stages every `agent/*.mjs` host into `<dir>/agent`: a host that names a
 * package is bundled (bundleHost), and every other host is copied
 * byte-identically, so the staged tree holds exactly what a package carries.
 * A host that resolves a module at run time beyond a string-literal
 * specifier fails here (runtimeResolutionUses): a package carries no
 * node_modules for it to resolve against.
 *
 * The entries and metafiles go to a run directory beside the staged tree
 * (build/package-cache/bundles), where the notices assembly reads the modules
 * each bundle inlined; the directory is removed first, so a stale file can
 * never be read as this run's.
 */
export async function stageAgent(root: string, dir: string): Promise<StagedHost[]> {
  const source = path.join(root, AGENT_DIR);
  const outDir = path.join(dir, AGENT_DIR);
  mkdirSync(outDir, { recursive: true });
  const metafiles = path.join(cacheDir(root), BUNDLE_DIR);
  rmSync(metafiles, { recursive: true, force: true });
  const hosts: StagedHost[] = [];
  for (const host of readdirSync(source).filter((name) => name.endsWith(".mjs")).sort()) {
    const from = path.join(source, host);
    const to = path.join(outDir, host);
    const text = readFileSync(from, "utf8");
    const runtime = runtimeResolutionUses(text);
    if (runtime.length > 0) {
      throw new Error(
        `${AGENT_DIR}/${host}: ${runtime.join(", ")} resolves a module at run time, where staging cannot see it; a host must name its packages as string literals (a package carries no node_modules to resolve them from)`,
      );
    }
    const specifiers = hostSpecifiers(text);
    if (specifiers.length === 0) {
      copyFileSync(from, to);
      hosts.push({ host, bundled: false, bytes: statSync(to).size, specifiers: [], metafile: "" });
      continue;
    }
    const bundle = await bundleHost(root, host, outDir, { metafile: path.join(metafiles, `${host}.json`) });
    hosts.push({ host, bundled: true, bytes: bundle.bytes, specifiers: bundle.specifiers, metafile: bundle.metafile });
  }
  return hosts;
}

/** One built host bundle: the staged file, what it pinned, and the metafile naming every module it inlined. */
export interface HostBundle {
  /** The bundle's absolute path inside outDir. */
  file: string;
  bytes: number;
  /** The package specifiers the generated entry pinned, in the host's order. */
  specifiers: string[];
  /** The metafile bun wrote (a staging intermediate, never shipped). */
  metafile: string;
}

/**
 * Builds one host into `outDir/<host>` as a single self-contained ESM file for
 * the pinned Node runtime, through the generated entry bundleEntrySource:
 * every specifier the host names is resolved from agent/ with Bun.resolveSync
 * (so the entry itself may live outside the tree) and pinned as a namespace
 * import, and the host follows. The metafile names every module bun inlined,
 * which is what the notices assembly reads.
 *
 * The entry is written beside the metafile — the run's intermediates
 * directory, outside the staged tree, never shipped — so bun's per-module
 * comment renders its path relative to the checkout; an entry in the system
 * temp directory can sit on another drive than the checkout on Windows, and
 * the comment then carries the build machine's absolute path (account name
 * included) into the shipped bundle. The build runs twice and both outputs
 * must be byte-identical: a bundle that differs between two runs of one
 * source would make two stagings of one commit differ too.
 */
export async function bundleHost(
  root: string,
  host: string,
  outDir: string,
  options: { metafile: string },
): Promise<HostBundle> {
  const agentDir = path.join(root, AGENT_DIR);
  const source = path.join(agentDir, host);
  const specifiers = hostSpecifiers(readFileSync(source, "utf8"));
  if (specifiers.length === 0) throw new Error(`${host} names no package specifier; it is staged as a copy, not a bundle`);
  const pins = specifiers.map((specifier) => {
    try {
      return { specifier, path: Bun.resolveSync(specifier, agentDir) };
    } catch (error) {
      const reason = error instanceof Error ? error.message : String(error);
      throw new Error(`${host}: cannot resolve the specifier ${JSON.stringify(specifier)} from ${AGENT_DIR}/: ${reason}`);
    }
  });
  const file = path.join(outDir, host);
  const entry = path.join(path.dirname(options.metafile), `${host}.entry.mjs`);
  mkdirSync(path.dirname(options.metafile), { recursive: true });
  writeFileSync(entry, bundleEntrySource(source, pins));
  // `--outfile`, never `--outdir`: bun renames a .mjs entry to .js in an
  // output directory, and the staged file keeps the host's own name. The
  // metafile flag takes its value inline (`--metafile=<file>`) on bun 1.4.5:
  // a separated value is parsed as a second entry point.
  const build = async (): Promise<void> => {
    const result = await run(
      process.execPath,
      ["build", entry, "--target=node", "--format=esm", `--outfile=${file}`, `--metafile=${options.metafile}`],
      { cwd: agentDir },
    );
    if (result.code !== 0) throw new Error(`bun build ${host} exited ${result.code}: ${result.stdout}${result.stderr}`);
    if (!existsSync(file)) throw new Error(`bun build ${host} wrote no ${file}`);
  };
  await build();
  const first = readFileSync(file);
  await build();
  const second = readFileSync(file);
  if (!first.equals(second)) {
    throw new Error(
      `bun build ${host} is not reproducible: one source built twice differs (${first.length} vs ${second.length} bytes); something that varies between runs would ship in every package`,
    );
  }
  return { file, bytes: second.length, specifiers, metafile: options.metafile };
}

/**
 * The generated entry bun bundles for a host: one namespace import per
 * specifier the host names, held in the exported `pinned` array, then the host
 * itself. The array is what keeps a package's module initialization: the
 * packages declare `sideEffects` arrays, so bun's bundler otherwise resolves
 * the host's named imports to hoisted declarations and drops the modules' init
 * blocks, and the host faults on a real brief (`Cannot read properties of
 * undefined`, goal 0016's probe). An unused namespace import does not keep
 * them either; the binding must be held.
 */
export function bundleEntrySource(hostPath: string, pins: readonly { specifier: string; path: string }[]): string {
  const specifierOf = (target: string): string => target.split(path.sep).join("/");
  const imports = pins.map((pin, index) => `import * as pinned${index} from ${JSON.stringify(specifierOf(pin.path))}; // ${pin.specifier}`);
  const held = pins.map((_, index) => `pinned${index}`).join(", ");
  return [
    "// Generated by the harness for staging: do not edit.",
    ...imports,
    `export const pinned = [${held}];`,
    `import ${JSON.stringify(specifierOf(hostPath))};`,
    "",
  ].join("\n");
}

/**
 * The package specifiers a host script names, in the order it names them: the
 * string-literal arguments of `import`, `export … from`, `require` and
 * `import()` (a template with no substitution counts as a literal), read by
 * bun's own parser (scanImports) so a specifier is found where the language
 * has one, and not where a string or a comment spells one — the bundled openai
 * SDK's error text contains `from 'undici'`, which a text scan would take for
 * an import. `node:` built-ins are not packages: a staged host may use those
 * anywhere. A specifier a host resolves at run time is invisible to this
 * scan; staging refuses those in a source host (runtimeResolutionUses).
 */
export function hostSpecifiers(source: string): string[] {
  // bun's transpiler refuses the hosts' `#!/usr/bin/env node` line.
  const text = source.replace(/^#!.*(\r?\n|$)/, "");
  const scanner = new Bun.Transpiler({ loader: "js" });
  const named = scanner.scanImports(text).map((entry) => entry.path);
  return [...new Set(named)].filter((specifier) => !specifier.startsWith("node:"));
}

/**
 * The places a host script resolves a module at run time beyond a
 * string-literal specifier: an `import()`/`require()` call whose argument is
 * not a string literal (a quoted string, or a template with no substitution),
 * and the resolver APIs staging cannot see the answer of — `createRequire`,
 * `require.resolve` and `import.meta.resolve`. Their module exists only at
 * run time, so staging can neither resolve it into a bundle nor prove that a
 * copied host runs where it lands: the run fails instead of shipping a host
 * that cannot resolve its own packages outside the checkout (goal 0016,
 * default 1).
 *
 * hostSpecifiers reads only the specifiers bun's parser sees, so this scan is
 * what covers the rest. It reads the source text, not a parsed tree: a use
 * spelled inside a comment or a string is reported too, which fails closed
 * with a message that names the site.
 */
export function runtimeResolutionUses(source: string): string[] {
  const text = source.replace(/^#!.*(\r?\n|$)/, "");
  const site = (match: RegExpExecArray): string => {
    const rest = text.slice(match.index + match[0].length);
    const close = rest.indexOf(")");
    const argument = (close === -1 ? rest.slice(0, 32) : rest.slice(0, close)).replace(/\s+/g, " ").trim();
    return `${match[0].replace(/\s+/g, "")}${argument})`;
  };
  const uses: string[] = [];
  for (const match of text.matchAll(/\b(?:import|require)\s*\(/g)) {
    const rest = text.slice(match.index + match[0].length);
    if (literalArgument(rest)) continue;
    uses.push(site(match));
  }
  for (const match of text.matchAll(/\bcreateRequire\s*\(|\brequire\s*\.\s*resolve\s*\(|\bimport\s*\.\s*meta\s*\.\s*resolve\s*\(/g)) {
    uses.push(site(match));
  }
  return uses;
}

/**
 * Whether the text after a call's `(` is an argument hostSpecifiers can read:
 * a quoted string, or a template literal with no substitution.
 */
function literalArgument(rest: string): boolean {
  const argument = rest.replace(/^\s*/, "");
  const quote = argument[0];
  if (quote === `"` || quote === `'`) return true;
  if (quote !== "`") return false;
  const end = argument.indexOf("`", 1);
  return end !== -1 && !argument.slice(1, end).includes("${");
}

/** The one non-host file the staged agent tree may carry: the assembled third-party notices. */
export const NOTICES_FILE = "THIRD-PARTY-NOTICES.txt";

/**
 * The staged agent tree must carry the host scripts and nothing else: no
 * node_modules, no symlink, no file that is not a host script or the notices
 * file, no host that names a package specifier, and no file that embeds a
 * path of the machine that built it (machinePathIn). A bundle is
 * self-contained, so anything else is a tree that cannot run where it ships —
 * or one that names the account and directory it was built in — and the run
 * fails rather than shipping the old tree (goal 0016, default 1). The
 * specifier scan is hostSpecifiers (bun's own parser), so a `.node` or wasm
 * asset a bundle still requires appears here as a specifier that is not a
 * `node:` built-in.
 *
 * Exported for a caller that checks a tree outside staging: the package
 * smoke's contents scan over the installed tree, and fixtures that must fail.
 * `root` adds the repository's absolute path to the forbidden text; only
 * staging knows it.
 */
export function assertLeanAgentTree(dir: string, options: { root?: string } = {}): string {
  const offenders: string[] = [];
  let files = 0;
  const label = path.basename(dir);
  const walk = (current: string, prefix: string): void => {
    for (const entry of readdirSync(current, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
      const full = path.join(current, entry.name);
      const relative = `${label}/${prefix === "" ? entry.name : `${prefix}/${entry.name}`}`;
      if (entry.isSymbolicLink()) {
        offenders.push(`${relative}: a symlink`);
        continue;
      }
      if (entry.isDirectory()) {
        offenders.push(`${relative}: a directory (the tree holds the host scripts and ${NOTICES_FILE} only)`);
        continue;
      }
      if (!entry.isFile()) {
        offenders.push(`${relative}: not a regular file`);
        continue;
      }
      files += 1;
      const text = readFileSync(full, "utf8");
      const machine = machinePathIn(text, options.root);
      if (machine !== undefined) {
        offenders.push(`${relative}: embeds the machine path ${JSON.stringify(machine)}; a package carries no path of the machine that built it`);
      }
      if (!entry.name.endsWith(".mjs")) {
        if (entry.name !== NOTICES_FILE) offenders.push(`${relative}: not a host script or ${NOTICES_FILE}`);
        continue;
      }
      for (const specifier of hostSpecifiers(text)) {
        offenders.push(`${relative}: names the specifier ${JSON.stringify(specifier)}; a staged host may name only node: built-ins`);
      }
    }
  };
  walk(dir, "");
  if (offenders.length > 0) {
    const more = offenders.length > 5 ? ` (${offenders.length - 5} more)` : "";
    throw new Error(`the ${label} tree is not lean: ${offenders.slice(0, 5).join("; ")}${more}`);
  }
  return `${label} is lean: ${files} files (host scripts and ${NOTICES_FILE} only), no node_modules, no symlinks, no package specifier, no machine path`;
}

/**
 * The shapes of a machine-specific path a staged file must not carry (goal
 * 0016): a Windows drive path, a POSIX user home or temp path, and Windows'
 * per-user data directory. A file that embeds one names the machine it was
 * built on — a private account, a temp directory — and keeps two stagings of
 * one source from being byte-identical.
 */
const MACHINE_PATH_PATTERNS: readonly RegExp[] = [
  /\b[A-Za-z]:[\\/]/,
  /\/Users\//,
  /\/home\//,
  /\/tmp\//,
  /\/var\/folders\//,
  /AppData[\\/]/,
];

/**
 * The first machine-specific path in the text: the shapes above, this
 * machine's temp directory, and — when the caller knows it — the reader's
 * repository root in both spellings.
 */
function machinePathIn(text: string, root: string | undefined): string | undefined {
  for (const pattern of MACHINE_PATH_PATTERNS) {
    const match = pattern.exec(text);
    // The whole token, not just the shape's first characters, so the
    // offender line names the path a reader must remove.
    if (match !== null) return text.slice(match.index, match.index + 96).split(/\s/)[0];
  }
  const literals = [tmpdir(), root ?? ""];
  if (root !== undefined) literals.push(root.split(path.sep).join("/"));
  for (const literal of literals) {
    if (literal !== "" && text.includes(literal)) return literal;
  }
  return undefined;
}

/** How long one archive's whole download may take before it fails. */
const DOWNLOAD_TIMEOUT_MS = 10 * 60 * 1000;

/** Where a build's downloaded archives are cached, so a re-run does not download again. */
export function cacheDir(root: string): string {
  return path.join(root, CACHE_DIR);
}

/**
 * One pinned build artifact, downloaded into the cache once and checked
 * against its SHA-256 every time. `label` names the artifact in the progress
 * line and `published` names where the pin was published, for a mismatch.
 */
export async function fetchPinned(options: {
  cache: string;
  asset: string;
  url: string;
  sha256: string;
  label: string;
  published: string;
  log: (line: string) => void;
}): Promise<string> {
  const { cache, asset, url, sha256, label, published, log } = options;
  mkdirSync(cache, { recursive: true });
  const file = path.join(cache, asset);
  if (!existsSync(file)) {
    log(`${label}: downloading ${url}`);
    const part = `${file}.part`;
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(DOWNLOAD_TIMEOUT_MS) });
      if (!response.ok) throw new Error(`HTTP ${response.status} ${response.statusText}`);
      await Bun.write(part, response);
    } catch (error) {
      rmSync(part, { force: true });
      const reason = error instanceof Error ? error.message : String(error);
      throw new Error(`${url}: download failed within ${DOWNLOAD_TIMEOUT_MS / 60000} minutes: ${reason}`);
    }
    renameSync(part, file);
  }
  const sum = createHash("sha256").update(new Uint8Array(await Bun.file(file).arrayBuffer())).digest("hex");
  if (sum !== sha256) {
    throw new Error(`${asset}: SHA-256 ${sum}, want ${sha256} (${published})`);
  }
  return file;
}

/** Downloads, verifies and extracts the pinned runtime for the target's platform. */
async function stageNodeRuntime(dir: string, cache: string, pin: NodePin, log: (line: string) => void): Promise<string> {
  const archive = await fetchPinned({
    cache,
    asset: pin.archive,
    url: `https://nodejs.org/dist/v${NODE_VERSION}/${pin.archive}`,
    sha256: pin.sha256,
    label: `node ${NODE_VERSION}`,
    published: `nodejs.org/dist/v${NODE_VERSION}/SHASUMS256.txt`,
    log,
  });
  const dest = path.join(dir, pin.path);
  mkdirSync(path.dirname(dest), { recursive: true });
  const unpack = mkdtempSync(path.join(dir, "unpack-"));
  try {
    const binary = await extractMember(archive, pin.member, unpack);
    assertPlatformExecutable(binary, pin.platform, `${pin.archive}: ${pin.member}`);
    renameSync(binary, dest);
  } finally {
    rmSync(unpack, { recursive: true, force: true });
  }
  // Cross-staged on Windows the bit is meaningless here; the AppImage and the
  // packages set it where the runtime ships (buildAppImage chmods node).
  if (process.platform !== "win32") chmodSync(dest, 0o755);
  return dest;
}

/** Fails unless file starts with the native executable header of platform. */
export function assertPlatformExecutable(file: string, platform: NodePin["platform"], label: string): void {
  const header = Buffer.alloc(4);
  const fd = openSync(file, "r");
  try {
    readSync(fd, header, 0, 4, 0);
  } finally {
    closeSync(fd);
  }
  const magic = header.toString("hex");
  if (!EXECUTABLE_MAGIC[platform].some((prefix) => magic.startsWith(prefix))) {
    throw new Error(`${label} starts with ${magic}, not a ${platform} executable`);
  }
}

/** The first bytes of a native executable per platform: PE, Mach-O, ELF. */
const EXECUTABLE_MAGIC: Record<NodePin["platform"], string[]> = {
  win32: ["4d5a"],
  darwin: ["cffaedfe", "cafebabe"],
  linux: ["7f454c46"],
};

/**
 * One archive member, extracted into dir: the file's path. Each platform's own
 * tar reads its own archive format (bsdtar reads the Windows zip, GNU tar and
 * bsdtar read the macOS and Linux tarballs); only the needed member is
 * extracted, never the whole distribution. The member is a native executable
 * of about 90 MB, so it goes to disk rather than through run()'s text stdout,
 * which would mangle it.
 */
async function extractMember(archive: string, member: string, dir: string): Promise<string> {
  const tar = tarExecutable();
  const result = await run(tar, ["-x", "-C", dir, "-f", archive, member]);
  if (result.code !== 0) {
    throw new Error(`${tar} -x ${path.basename(archive)} ${member} exited ${result.code}: ${result.stdout}${result.stderr}`);
  }
  const binary = path.join(dir, ...member.split("/"));
  if (!existsSync(binary)) throw new Error(`${path.basename(archive)} holds no member ${member}`);
  return binary;
}

/** The tar that reads the host's archive: Windows' own bsdtar is a full path, not PATH's. */
function tarExecutable(): string {
  if (process.platform === "win32") {
    const systemRoot = process.env.SystemRoot ?? "C:\\Windows";
    return executable(path.join(systemRoot, "System32", "tar"));
  }
  return executable("tar");
}

/**
 * The runtime checks, run only where the staged runtime is executable: a
 * package for another target is proved by its tests there (its archive's
 * SHA-256 and executable magic were checked when it was unpacked), and the
 * lean-tree check covers the tree either way.
 */
async function checkStagedFor(dir: string, pin: NodePin, target: PackageTarget): Promise<string[]> {
  if (target.platform === process.platform && target.arch === process.arch) return checkStaged(dir, pin);
  return [
    `the ${target.platform}-${target.arch} runtime was not run: it does not execute on ${process.platform}-${process.arch}; the package proves it where it ships`,
  ];
}

/** The one host that carries packages; the staged-runtime probe runs it, and the AppImage carries it. */
export const DURABLE_HOST = "durable-host.mjs";

/**
 * Proves the staged tree with the staged runtime: the runtime runs at all, and
 * the staged durable host — the one host that carries packages, so the one
 * bundle — loads on it and answers a `status` request, from a temp directory
 * with no node_modules anywhere near it. This is a load proof: a bundle that
 * lost a module's initialization answers `status` and still faults on a real
 * brief, which the durable matrix and the package smoke's brief case prove
 * where behaviour matters.
 */
async function checkStaged(dir: string, pin: NodePin): Promise<string[]> {
  const node = path.join(dir, pin.path);
  const version = await run(node, ["--version"]);
  if (version.code !== 0) throw new Error(`${node} --version exited ${version.code}: ${version.stdout}${version.stderr}`);
  if (version.stdout.trim() !== `v${NODE_VERSION}`) {
    throw new Error(`${node} --version printed ${version.stdout.trim()}, want v${NODE_VERSION}`);
  }
  const host = path.join(dir, AGENT_DIR, DURABLE_HOST);
  const probe = mkdtempSync(path.join(tmpdir(), "passquest-stage-probe-"));
  try {
    const store = path.join(probe, "store.sqlite");
    writeFileSync(store, "");
    const request = JSON.stringify({
      protocol_version: 1,
      run_id: "run-19700101T000000Z-00000000",
      store_path: store,
      mode: "status",
    });
    const started = performance.now();
    const answer = await run(node, [host], { cwd: probe, stdin: request });
    let document: unknown;
    try {
      document = JSON.parse(answer.stdout);
    } catch {
      document = undefined;
    }
    const state = stringField(document, "state") ?? "";
    if (answer.code !== 0 || state !== "status") {
      throw new Error(
        `${DURABLE_HOST} did not answer status on the staged runtime (exit ${answer.code}, state ${JSON.stringify(state)}):\n${answer.stdout}${answer.stderr}`,
      );
    }
    return [
      `node --version: ${version.stdout.trim()}`,
      `${DURABLE_HOST} answered status on the staged runtime in ${Math.round(performance.now() - started)} ms`,
    ];
  } finally {
    rmSync(probe, { recursive: true, force: true });
  }
}

/** Counts a file, or a tree's regular files and bytes. */
function count(target: string): { files: number; bytes: number } {
  if (statSync(target).isFile()) return { files: 1, bytes: statSync(target).size };
  let files = 0;
  let bytes = 0;
  const walk = (current: string): void => {
    for (const entry of readdirSync(current, { withFileTypes: true })) {
      const child = path.join(current, entry.name);
      if (entry.isDirectory()) walk(child);
      else {
        files += 1;
        bytes += statSync(child).size;
      }
    }
  };
  walk(target);
  return { files, bytes };
}

/**
 * The tree as printed lines: every file at the top level and inside a
 * top-level directory; a deeper directory is folded into one line with its
 * file count and bytes.
 */
function summarize(dir: string): { entries: StageEntry[]; files: number; bytes: number } {
  const entries: StageEntry[] = [];
  for (const entry of readdirSync(dir, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
    const child = path.join(dir, entry.name);
    if (entry.isFile()) entries.push({ path: entry.name, ...count(child) });
    else {
      for (const nested of readdirSync(child, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
        const target = path.join(child, nested.name);
        entries.push({ path: `${entry.name}/${nested.name}`, ...count(target) });
      }
    }
  }
  const files = entries.reduce((total, entry) => total + entry.files, 0);
  const bytes = entries.reduce((total, entry) => total + entry.bytes, 0);
  return { entries, files, bytes };
}

/**
 * The app icon: a 1024 px rounded square in the app's accent (tokens.css
 * `--acc`, oklch(48.5% 0.19 268) as sRGB) with a white check, drawn with
 * antialiasing and written as an 8-bit RGBA PNG. It is generated, not stored:
 * only the packager reads it, and only to hand it to MyGo.
 */
export function iconPng(size = 1024): Uint8Array {
  const background = [51, 80, 200];
  const mark = [255, 255, 255];
  const corner = 0.2237; // the rounded-rect corner shared by macOS app icons
  const stroke = [0.285, 0.535, 0.445, 0.69, 0.73, 0.35]; // the check's two capsules
  const half = 0.052;
  const samples = 3;
  const pixels = new Uint8Array(size * size * 4);
  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) {
      let inside = 0;
      let ink = 0;
      for (let sy = 0; sy < samples; sy++) {
        for (let sx = 0; sx < samples; sx++) {
          const u = (x + (sx + 0.5) / samples) / size;
          const v = (y + (sy + 0.5) / samples) / size;
          if (!inRoundedSquare(u, v, corner)) continue;
          inside++;
          if (
            distanceToSegment(u, v, stroke[0], stroke[1], stroke[2], stroke[3]) <= half ||
            distanceToSegment(u, v, stroke[2], stroke[3], stroke[4], stroke[5]) <= half
          ) {
            ink++;
          }
        }
      }
      const at = (y * size + x) * 4;
      const alpha = Math.round((inside / (samples * samples)) * 255);
      const mix = inside === 0 ? 0 : ink / inside;
      for (let channel = 0; channel < 3; channel++) {
        pixels[at + channel] = Math.round(background[channel] * (1 - mix) + mark[channel] * mix);
      }
      pixels[at + 3] = alpha;
    }
  }
  return png(size, pixels);
}

/** Whether a point in [0,1]² is inside a square with rounded corners of radius r. */
function inRoundedSquare(x: number, y: number, r: number): boolean {
  const dx = x - Math.min(Math.max(x, r), 1 - r);
  const dy = y - Math.min(Math.max(y, r), 1 - r);
  return dx * dx + dy * dy <= r * r;
}

/** The distance from a point to a segment, for capsules with round ends. */
function distanceToSegment(px: number, py: number, ax: number, ay: number, bx: number, by: number): number {
  const dx = bx - ax;
  const dy = by - ay;
  const length = dx * dx + dy * dy;
  const t = length === 0 ? 0 : Math.max(0, Math.min(1, ((px - ax) * dx + (py - ay) * dy) / length));
  const x = ax + t * dx - px;
  const y = ay + t * dy - py;
  return Math.hypot(x, y);
}

/** An 8-bit RGBA PNG of one square image: one IDAT, filter 0, zlib deflate. */
function png(size: number, pixels: Uint8Array): Uint8Array {
  const stride = size * 4;
  const raw = new Uint8Array((stride + 1) * size);
  for (let y = 0; y < size; y++) {
    raw.set(pixels.subarray(y * stride, (y + 1) * stride), y * (stride + 1) + 1);
  }
  const ihdr = new Uint8Array(13);
  new DataView(ihdr.buffer).setUint32(0, size);
  new DataView(ihdr.buffer).setUint32(4, size);
  ihdr[8] = 8; // bit depth
  ihdr[9] = 6; // color type: RGBA
  return concat([
    Uint8Array.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk("IHDR", ihdr),
    chunk("IDAT", deflateSync(raw, { level: 9 })),
    chunk("IEND", new Uint8Array(0)),
  ]);
}

/** One PNG chunk: length, type, data, CRC-32 of type and data. */
function chunk(type: string, data: Uint8Array): Uint8Array {
  const head = new Uint8Array(4);
  new DataView(head.buffer).setUint32(0, data.byteLength);
  const body = concat([new TextEncoder().encode(type), data]);
  const crc = new Uint8Array(4);
  new DataView(crc.buffer).setUint32(0, Bun.hash.crc32(body));
  return concat([head, body, crc]);
}

function concat(parts: Uint8Array[]): Uint8Array {
  const total = new Uint8Array(parts.reduce((sum, part) => sum + part.byteLength, 0));
  let at = 0;
  for (const part of parts) {
    total.set(part, at);
    at += part.byteLength;
  }
  return total;
}
