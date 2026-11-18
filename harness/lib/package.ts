/**
 * Stages the resources a package carries (goal 0013, step P2): the agent host
 * scripts, a hoisted and symlink-free production node_modules for them, a
 * pinned Node 22 LTS runtime, and the app icon. `mygo.json` lists the tree, so
 * `go tool mygo build` copies it into the app (mygo.PathResources), where
 * internal/hostpaths resolves the hosts and the runtime from.
 *
 * What MyGo's own resource copy would skip is pruned here, so the staged tree
 * is the tree the package carries.
 *
 * The tree lives under build/, which is gitignored: nothing here is committed,
 * and the gates keep building the development app through buildDesktop.
 */
import { createHash } from "node:crypto";
import {
  chmodSync,
  copyFileSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  renameSync,
  rmSync,
  statSync,
  writeFileSync,
} from "node:fs";
import path from "node:path";
import { deflateSync } from "node:zlib";

import { copyAgentHosts } from "./desktop.ts";
import { executable, run } from "./process.ts";

/** The staged tree, under the gitignored build/ directory. */
export const STAGE_DIR = "build/package";

/**
 * Where downloaded archives are kept, so a re-run does not download again. It sits beside the
 * staged tree, not inside it, so nothing that copies the staged tree can carry other platforms' archives.
 */
const CACHE_DIR = "build/package-cache";

/** The Node runtime every package carries. */
export const NODE_VERSION = "22.23.5";

/**
 * One platform's Node runtime: the official archive, its executable inside it,
 * where the app looks for that executable, and the archive's SHA-256 as
 * nodejs.org/dist/v<NODE_VERSION>/SHASUMS256.txt publishes it (cross-checked
 * against the downloaded bytes when the pins were written).
 */
export interface NodePin {
  /** process.platform of the target. */
  platform: "win32" | "darwin" | "linux";
  /** process.arch of the target. */
  arch: "x64" | "arm64";
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
  /** What the load checks printed (the runtime's version, the imports). */
  checks: string[];
}

export interface StageOptions {
  /** Progress lines, one per step; a build log, not gate output. */
  log?: (line: string) => void;
}

/**
 * Writes the whole resources tree into build/package. The agent hosts are
 * copied, their production dependencies are installed with Bun's hoisted
 * linker into a temporary directory (so bun.lock is not written and the
 * checkout's node_modules is untouched), the pinned Node runtime comes from
 * the cached archive, and the icon is drawn here. Then the staged runtime
 * loads the host scripts' own imports, which is what proves the tree before a
 * build copies it.
 */
export async function stageResources(root: string, options: StageOptions = {}): Promise<StagedResources> {
  const log = options.log ?? (() => {});
  const dir = path.join(root, STAGE_DIR);
  const pin = nodePin();
  clearStage(dir);
  mkdirSync(dir, { recursive: true });

  const hosts = copyAgentHosts(root, dir);
  if (hosts.length === 0) throw new Error("agent/ holds no host scripts to package");
  log(`agent: ${hosts.length} host scripts`);
  const modules = await stageAgentModules(root, dir);
  log(`agent/node_modules: ${modules.files} files, ${modules.bytes} bytes (hoisted, no symlinks)`);
  if (modules.dropped > 0) {
    log(`agent/node_modules: ${modules.dropped} hidden directories MyGo would not copy are removed`);
  }
  const node = await stageNodeRuntime(dir, path.join(root, CACHE_DIR), pin, log);
  log(`node ${NODE_VERSION}: ${STAGE_DIR}/${pin.path}`);
  const icon = path.join(dir, "icon.png");
  writeFileSync(icon, iconPng());
  log(`icon: ${STAGE_DIR}/icon.png (${statSync(icon).size} bytes)`);

  const checks = await checkStaged(root, dir, pin, hosts);
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

/**
 * Installs the agent workspace's production dependencies and moves the result
 * to build/package/agent/node_modules. The temporary install reproduces the
 * repository's workspace shape (the root package.json with its `agent`
 * workspace, agent/package.json and bun.lock), so Bun installs exactly the
 * locked versions with `--frozen-lockfile`; `--production` leaves the harness
 * devDependencies out and `--linker=hoisted` writes one node_modules without
 * the isolated linker's store.
 */
async function stageAgentModules(root: string, dir: string): Promise<{ files: number; bytes: number; dropped: number }> {
  const target = path.join(dir, "agent", "node_modules");
  const tmp = mkdtempSync(path.join(dir, "install-"));
  try {
    copyFileSync(path.join(root, "package.json"), path.join(tmp, "package.json"));
    copyFileSync(path.join(root, "bun.lock"), path.join(tmp, "bun.lock"));
    mkdirSync(path.join(tmp, "agent"));
    copyFileSync(path.join(root, "agent", "package.json"), path.join(tmp, "agent", "package.json"));
    const install = await run(
      process.execPath,
      ["install", "--production", "--frozen-lockfile", "--linker=hoisted"],
      { cwd: tmp },
    );
    if (install.code !== 0) {
      throw new Error(`bun install --production --frozen-lockfile --linker=hoisted failed (exit ${install.code}):\n${install.stdout}${install.stderr}`);
    }
    const modules = path.join(tmp, "node_modules");
    // The hoisted linker still links the workspace package itself; a package
    // carries no link, and MyGo would copy a link as a link. The .bin shims it
    // also writes are a hidden directory, which pruneUncopied removes the way
    // MyGo's own resource copy would.
    for (const entry of readdirSync(modules, { withFileTypes: true })) {
      if (entry.isSymbolicLink()) rmSync(path.join(modules, entry.name), { recursive: true, force: true });
    }
    const dropped = pruneUncopied(modules);
    assertNoLinks(modules);
    renameSync(modules, target);
    return { ...count(target), dropped };
  } finally {
    rmSync(tmp, { recursive: true, force: true });
  }
}

/** The droppings of a file manager, which MyGo's resource copy skips. */
const JUNK_NAMES: Record<string, true> = { ".DS_Store": true, "Thumbs.db": true, "desktop.ini": true };

/**
 * Removes what MyGo's resource copy leaves behind: a hidden directory at any
 * depth, and the junkName files (mygo@v0.10.0 cmd/mygo/resources.go). A hidden
 * file ships, so it stays. The staged tree is then the tree the package
 * carries, and the import check below proves the package rather than a wider
 * tree.
 */
function pruneUncopied(dir: string): number {
  let dropped = 0;
  const walk = (current: string): void => {
    for (const entry of readdirSync(current, { withFileTypes: true })) {
      const child = path.join(current, entry.name);
      if (entry.isDirectory() && entry.name.startsWith(".")) {
        rmSync(child, { recursive: true, force: true });
        dropped += 1;
      } else if (entry.isDirectory()) {
        walk(child);
      } else if (JUNK_NAMES[entry.name]) {
        rmSync(child, { force: true });
        dropped += 1;
      }
    }
  };
  walk(dir);
  return dropped;
}

/** Every symlink under dir, so that a tree that would ship one fails here. */
function assertNoLinks(dir: string): void {
  const links: string[] = [];
  const walk = (current: string): void => {
    for (const entry of readdirSync(current, { withFileTypes: true })) {
      const child = path.join(current, entry.name);
      if (entry.isSymbolicLink()) links.push(child);
      else if (entry.isDirectory()) walk(child);
    }
  };
  walk(dir);
  if (links.length > 0) throw new Error(`node_modules holds ${links.length} symlinks, which a package must not: ${links.slice(0, 5).join(", ")}`);
}

/** How long one archive's whole download may take before it fails. */
const DOWNLOAD_TIMEOUT_MS = 10 * 60 * 1000;

/** Downloads, verifies and extracts the pinned runtime for the host platform. */
async function stageNodeRuntime(dir: string, cache: string, pin: NodePin, log: (line: string) => void): Promise<string> {
  mkdirSync(cache, { recursive: true });
  const archive = path.join(cache, pin.archive);
  if (!existsSync(archive)) {
    const url = `https://nodejs.org/dist/v${NODE_VERSION}/${pin.archive}`;
    log(`node ${NODE_VERSION}: downloading ${url}`);
    const part = `${archive}.part`;
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(DOWNLOAD_TIMEOUT_MS) });
      if (!response.ok) throw new Error(`HTTP ${response.status} ${response.statusText}`);
      await Bun.write(part, response);
    } catch (error) {
      rmSync(part, { force: true });
      const reason = error instanceof Error ? error.message : String(error);
      throw new Error(`${url}: download failed within ${DOWNLOAD_TIMEOUT_MS / 60000} minutes: ${reason}`);
    }
    renameSync(part, archive);
  }
  const sum = createHash("sha256").update(new Uint8Array(await Bun.file(archive).arrayBuffer())).digest("hex");
  if (sum !== pin.sha256) {
    throw new Error(
      `${pin.archive}: SHA-256 ${sum}, want ${pin.sha256} (nodejs.org/dist/v${NODE_VERSION}/SHASUMS256.txt)`,
    );
  }
  const dest = path.join(dir, pin.path);
  mkdirSync(path.dirname(dest), { recursive: true });
  const unpack = mkdtempSync(path.join(dir, "unpack-"));
  try {
    const binary = await extractMember(archive, pin.member, unpack);
    const magic = Buffer.from(await Bun.file(binary).slice(0, 4).arrayBuffer()).toString("hex");
    if (!EXECUTABLE_MAGIC[pin.platform].some((prefix) => magic.startsWith(prefix))) {
      throw new Error(`${pin.archive}: ${pin.member} starts with ${magic}, not a ${pin.platform} executable`);
    }
    renameSync(binary, dest);
  } finally {
    rmSync(unpack, { recursive: true, force: true });
  }
  if (process.platform !== "win32") chmodSync(dest, 0o755);
  return dest;
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
 * Proves the staged tree with the staged runtime: the runtime runs at all, and
 * every module the host scripts and agent/package.json name resolves from the
 * staged agent directory. Every specifier comes from the tree itself, so a
 * dependency the hosts gain or the manifest declares cannot pass on a stale
 * check.
 */
async function checkStaged(root: string, dir: string, pin: NodePin, hosts: string[]): Promise<string[]> {
  const node = path.join(dir, pin.path);
  const version = await run(node, ["--version"]);
  if (version.code !== 0) throw new Error(`${node} --version exited ${version.code}: ${version.stdout}${version.stderr}`);
  if (version.stdout.trim() !== `v${NODE_VERSION}`) {
    throw new Error(`${node} --version printed ${version.stdout.trim()}, want v${NODE_VERSION}`);
  }
  const imports = await hostImports(root, path.join(dir, "agent"), hosts);
  const check = await run(node, ["--input-type=module", "-e", imports], { cwd: path.join(dir, "agent") });
  if (check.code !== 0) {
    throw new Error(`the staged runtime could not import the hosts' dependencies (exit ${check.code}):\n${check.stdout}${check.stderr}`);
  }
  return [`node --version: ${version.stdout.trim()}`, ...check.stdout.trim().split("\n")];
}

/** The one host with dependencies; the check below must see them in it. */
const DURABLE_HOST = "durable-host.mjs";

/**
 * The import forms a host script writes: `from "x"` or `from 'x'`, a bare
 * `import "x"`, and a dynamic `import("x")`. A form this misses would be a
 * dependency a package ships without, so the check also imports every
 * dependency agent/package.json declares.
 */
const HOST_IMPORT = /\bfrom\s*["']([^"']+)["']|\bimport\s*\(\s*["']([^"']+)["']\s*\)|^\s*import\s*["']([^"']+)["']/gm;

/**
 * The module specifiers the staged tree must resolve: what the host scripts
 * import outside the standard library, and what agent/package.json declares,
 * as a script the staged runtime runs from the staged agent directory: one
 * `import()` per specifier, which is the resolution the hosts use.
 */
async function hostImports(root: string, agentDir: string, hosts: string[]): Promise<string> {
  if (!hosts.includes(DURABLE_HOST)) {
    throw new Error(`agent/ holds no ${DURABLE_HOST}; the hosts to package are ${hosts.join(", ") || "none"}`);
  }
  const specifiers = new Set<string>();
  for (const host of hosts) {
    const source = await Bun.file(path.join(agentDir, host)).text();
    const found: string[] = [];
    for (const match of source.matchAll(HOST_IMPORT)) {
      const specifier = match[1] ?? match[2] ?? match[3];
      if (specifier && !specifier.startsWith("node:")) found.push(specifier);
    }
    // The durable host is the host with dependencies (six specifiers today). A
    // parse that finds none of them would pass on a package that carries no
    // node_modules at all.
    if (host === DURABLE_HOST && found.length === 0) {
      throw new Error(`${host} imports no package the check can see: the staged tree would pass without its dependencies`);
    }
    for (const specifier of found) specifiers.add(specifier);
  }
  for (const name of Object.keys(await declaredDependencies(root))) specifiers.add(name);
  if (specifiers.size === 0) throw new Error("neither a host script nor agent/package.json names a package to check");
  const lines = [...specifiers].sort().map((name) => `await import(${JSON.stringify(name)});`);
  return `${lines.join("\n")}\nconsole.log("imported ${specifiers.size} specifiers the hosts and agent/package.json name");\n`;
}

/** The dependencies agent/package.json declares: every one must be in a package. */
async function declaredDependencies(root: string): Promise<Record<string, string>> {
  const manifest = JSON.parse(await Bun.file(path.join(root, "agent", "package.json")).text()) as {
    dependencies?: Record<string, string>;
  };
  return manifest.dependencies ?? {};
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
 * top-level directory, and every deeper directory folded into one line with
 * its file count and bytes (node_modules has thousands of files, which no log
 * wants one per line).
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
