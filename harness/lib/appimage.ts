/**
 * Builds the Linux AppImage (goal 0013, step P3). MyGo writes a Linux binary,
 * a .desktop entry and an icon next to the staged agent/ and node/ trees, and
 * no AppImage, so this module wraps that output in an AppDir and runs the
 * pinned appimagetool over it. It is the step `bun run package` runs on Linux
 * after MyGo's build; on another OS it refuses, because the tool it runs is a
 * Linux executable and the image it writes is run where it ships.
 *
 * No FUSE is assumed: the tool itself runs from its own extraction
 * (APPIMAGE_EXTRACT_AND_RUN=1), and the image it writes is verified through
 * the same extraction the app runs from without FUSE.
 *
 * The AppDir is built in the OS temp directory, a native file system whose
 * modes are real even when the staged tree came from Windows, and it is
 * removed once the image is verified.
 */
import { chmodSync, copyFileSync, existsSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, readlinkSync, rmSync, statSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { AGENT_DIR, assertPlatformExecutable, cacheDir, DURABLE_HOST, fetchPinned, nodePin, type NodePin, type PackageTarget, targetDir } from "./package.ts";
import { run } from "./process.ts";

/** The appimagetool release the AppImage is built with. */
const APPIMAGETOOL_VERSION = "1.9.1";

/**
 * appimagetool as its release publishes it, for the one architecture the
 * project's CI builds (linux x86_64): the asset and its SHA-256, cross-checked
 * against the released bytes when the pin was written (2026-11-18). The
 * project also publishes a rolling "continuous" build, which is no pin; 1.9.1
 * is its newest versioned release.
 */
const APPIMAGETOOL_X64 = {
  asset: "appimagetool-x86_64.AppImage",
  sha256: "ed4ce84f0d9caff66f50bcca6ff6f35aae54ce8135408b3fa33abfc3cb384eb0",
};

/**
 * The AppImage runtime appimagetool prepends to the image. Left to itself,
 * appimagetool 1.9.1 downloads it from the type2-runtime project's rolling
 * "continuous" release at build time, which is no pin for a build artifact and
 * no offline build; it is pinned here like the Node runtime, by its versioned
 * release and that release's SHA-256, and passed with --runtime-file.
 */
const APPIMAGE_RUNTIME = {
  version: "20251108",
  asset: "runtime-x86_64",
  sha256: "2fca8b443c92510f1483a883f60061ad09b46b978b2631c807cd873a47ec260d",
};

/** What the AppImage takes from mygo.json: the app's names and version. */
export interface PackagingConfig {
  name: string;
  version: string;
  linux?: { comment?: string };
}

/** MyGo's Linux output as the AppDir takes it: the binary, the entry and the icon. */
interface LinuxOutput {
  /** The file name MyGo gave the binary and its entry and icon (the slug). */
  slug: string;
  binary: string;
  desktop: string;
  icon: string;
}

export interface AppImageOptions {
  /** The target being packaged; the step is pinned for linux-x64. */
  target: PackageTarget;
  /** Progress lines, one per step; a build log, not gate output. */
  log?: (line: string) => void;
}

export interface AppImageResult {
  /** The AppImage's absolute path. */
  file: string;
  bytes: number;
}

/**
 * Wraps the target's MyGo output into an AppDir and builds the AppImage with
 * the pinned appimagetool, then proves the image by extracting it (see
 * checkImageTree): the tree is exactly the binary, the entry, the icon, AppRun
 * and the staged resources, the resources sit where mygo.PathResources looks
 * on Linux (the executable's directory), the only link is the spec's own
 * AppRun, and the modes are the ones set here.
 */
export async function buildAppImage(root: string, options: AppImageOptions): Promise<AppImageResult> {
  const log = options.log ?? (() => {});
  const { target } = options;
  if (process.platform !== "linux") {
    throw new Error(
      `the AppImage step runs on Linux; this is ${process.platform}: stage and build the target with \`bun run package --target ${target.platform}-${target.arch}\`, then run \`bun run package --appimage-only\` on Linux`,
    );
  }
  if (target.platform !== "linux" || target.arch !== "x64") {
    throw new Error(
      `the AppImage step is pinned for linux-x64: appimagetool ${APPIMAGETOOL_VERSION} (x86_64) is the only pinned build, and ${target.platform}-${target.arch} has no pin`,
    );
  }
  const pin = nodePin(target.platform, target.arch);
  const config = JSON.parse(await Bun.file(path.join(root, "mygo.json")).text()) as PackagingConfig;
  if (!config.name || !config.version) throw new Error("mygo.json needs a name and a version to name the AppImage after");
  const appName = config.name.replace(/[^A-Za-z0-9._-]+/g, "_");

  const stage = path.join(root, targetDir(target));
  const output = linuxOutput(stage);

  // The AppDir is scratch: the OS temp directory has real POSIX modes, and
  // nothing outside the AppImage needs the tree afterwards.
  const dir = mkdtempSync(path.join(tmpdir(), "passquest-appdir-"));
  chmodSync(dir, 0o755);
  // The staged tree's modes come from the staging host, which may be Windows
  // through a mount without POSIX modes (a file there can read 0777); every
  // mode the image carries is set here.
  for (const resource of [AGENT_DIR, pin.path.split("/")[0]]) copyTree(path.join(stage, resource), path.join(dir, resource));
  copyOutput(output.binary, path.join(dir, output.slug), 0o755);
  copyOutput(output.desktop, path.join(dir, path.basename(output.desktop)), 0o644);
  copyOutput(output.icon, path.join(dir, path.basename(output.icon)), 0o644);
  // The spec's AppRun: a link to the binary, which is also what makes the
  // executable's directory (mygo.PathResources on Linux) the AppDir root.
  symlinkSync(output.slug, path.join(dir, "AppRun"));
  // The staged runtime: copyTree wrote it 0644 like every other file.
  chmodSync(path.join(dir, ...pin.path.split("/")), 0o755);
  log(`appimage: AppDir ${dir}`);

  const tool = await fetchPinned({
    cache: cacheDir(root),
    asset: APPIMAGETOOL_X64.asset,
    url: `https://github.com/AppImage/appimagetool/releases/download/${APPIMAGETOOL_VERSION}/${APPIMAGETOOL_X64.asset}`,
    sha256: APPIMAGETOOL_X64.sha256,
    label: `appimagetool ${APPIMAGETOOL_VERSION}`,
    published: `the appimagetool ${APPIMAGETOOL_VERSION} release`,
    log,
  });
  chmodSync(tool, 0o755);
  const runtime = await fetchPinned({
    cache: cacheDir(root),
    asset: `${APPIMAGE_RUNTIME.asset}-${APPIMAGE_RUNTIME.version}`,
    url: `https://github.com/AppImage/type2-runtime/releases/download/${APPIMAGE_RUNTIME.version}/${APPIMAGE_RUNTIME.asset}`,
    sha256: APPIMAGE_RUNTIME.sha256,
    label: `the AppImage runtime ${APPIMAGE_RUNTIME.version}`,
    published: `the type2-runtime ${APPIMAGE_RUNTIME.version} release`,
    log,
  });
  chmodSync(runtime, 0o755);

  const file = path.join(stage, `${appName}-${config.version}-x86_64.AppImage`);
  rmSync(file, { force: true });
  log(`appimage: appimagetool ${APPIMAGETOOL_VERSION}: ${dir} -> ${path.relative(root, file)}`);
  const build = await run(tool, ["--runtime-file", runtime, dir, file], { env: { APPIMAGE_EXTRACT_AND_RUN: "1", ARCH: "x86_64" } });
  if (build.code !== 0) {
    throw new Error(`appimagetool ${APPIMAGETOOL_VERSION} exited ${build.code}:\n${build.stdout}${build.stderr}`);
  }
  if (!existsSync(file)) throw new Error(`appimagetool ${APPIMAGETOOL_VERSION} exited 0 but wrote no ${file}`);
  chmodSync(file, 0o755);

  const checks = await verifyImage(file, output.slug, pin, config);
  for (const line of checks) log(`check: ${line}`);
  rmSync(dir, { recursive: true, force: true });
  return { file, bytes: statSync(file).size };
}

/**
 * MyGo's Linux output, found by the entry it wrote rather than by a slug of
 * our own: a change in MyGo's naming fails here instead of shipping an image
 * without an entry, an icon or a binary.
 */
function linuxOutput(stage: string): LinuxOutput {
  if (!existsSync(stage)) {
    throw new Error(`${stage} does not exist; run \`bun run package --target linux-x64\` first`);
  }
  const desktops = readdirSync(stage, { withFileTypes: true }).filter((entry) => entry.isFile() && entry.name.endsWith(".desktop"));
  if (desktops.length !== 1) {
    throw new Error(`${stage}: want exactly one .desktop entry, found ${desktops.map((entry) => entry.name).join(", ") || "none"}`);
  }
  const slug = desktops[0].name.slice(0, -".desktop".length);
  const binary = path.join(stage, slug);
  const icon = path.join(stage, `${slug}.png`);
  for (const required of [binary, icon]) {
    if (!existsSync(required)) {
      throw new Error(`${stage}: ${desktops[0].name} expects ${path.basename(required)}, which MyGo did not write`);
    }
  }
  return { slug, binary, desktop: path.join(stage, desktops[0].name), icon };
}

/** Copies one file of MyGo's output into the AppDir, with the mode the image carries. */
function copyOutput(from: string, to: string, mode: number): void {
  copyFileSync(from, to);
  chmodSync(to, mode);
}

/**
 * Copies a resource tree into the AppDir, with the modes the image carries:
 * 0755 for directories, 0644 for files. The staged tree holds files and
 * directories only.
 */
function copyTree(source: string, dest: string): void {
  mkdirSync(dest, { recursive: true });
  for (const entry of readdirSync(source, { withFileTypes: true })) {
    const from = path.join(source, entry.name);
    const to = path.join(dest, entry.name);
    if (entry.isDirectory()) copyTree(from, to);
    else if (entry.isFile()) {
      copyFileSync(from, to);
      chmodSync(to, 0o644);
    } else throw new Error(`${from} is neither a file nor a directory; the staged tree must hold no links`);
  }
  chmodSync(dest, 0o755);
}

/** One path inside the built image: a file, a directory, or a link with what it names. */
interface ImageEntry {
  /** Path relative to the image root, with slashes. */
  path: string;
  /** What a link points at; empty for a file or directory. */
  link: string;
  /** Whether the entry is a directory. */
  directory: boolean;
  /** The permission bits, for everything statSync reports on. */
  mode: number;
  bytes: number;
}

/**
 * What an image's extracted tree must carry: the binary's name, the Node pin
 * whose staged path it holds, and the names mygo.json gives the app.
 */
export interface ImageExpectations {
  /** The image named in every failure: the AppImage's path, or a fixture tree's. */
  label: string;
  /** The binary's file name, which AppRun links to and Exec runs. */
  slug: string;
  /** The Node pin: the staged runtime's path inside the image. */
  pin: NodePin;
  /** mygo.json: the app's name, and the comment the Linux entry carries. */
  config: PackagingConfig;
}

/**
 * The checks over an image's extracted tree: the tree is exactly the binary,
 * the desktop entry, the icon, AppRun and the staged agent/ and node/ trees;
 * the entry carries the app's name, the comment from mygo.json and execs the
 * binary; every link is the spec's own AppRun (or a .DirIcon appimagetool
 * adds); no file or directory is group- or world-writable; and the binary and
 * the staged Node runtime are executable.
 *
 * Exported like the portability scan, so a fixture can feed it a tree that
 * must fail: an extra top-level entry, a link of its own, a group- or
 * world-writable file, a missing runtime.
 */
export function checkImageTree(dir: string, expect: ImageExpectations): string[] {
  const { label, slug, pin, config } = expect;
  // appimagetool adds the spec's .DirIcon from the desktop entry's icon.
  const dirIcon = path.join(dir, ".DirIcon");
  const hasDirIcon = existsSync(dirIcon);
  const top = readdirSync(dir).sort();
  const expected = [slug, `${slug}.desktop`, `${slug}.png`, "AppRun", AGENT_DIR, pin.path.split("/")[0], ...(hasDirIcon ? [".DirIcon"] : [])].sort();
  const missing = expected.filter((name) => !top.includes(name));
  const extra = top.filter((name) => !expected.includes(name));
  if (missing.length > 0 || extra.length > 0) {
    throw new Error(`${label}: want ${expected.join(", ")}; missing ${missing.join(", ") || "nothing"}, extra ${extra.join(", ") || "nothing"}`);
  }
  if (hasDirIcon && (lstatSync(dirIcon).isSymbolicLink() ? readlinkSync(dirIcon) : "") !== `${slug}.png`) {
    throw new Error(`${label}: .DirIcon is not a link to the icon ${slug}.png`);
  }

  const entries = walkImage(dir);
  for (const link of entries.filter((entry) => entry.link !== "")) {
    if (link.path === "AppRun" || link.path === ".DirIcon") continue;
    throw new Error(`${label}: ${link.path} is a link to ${link.link}; only the spec's own AppRun and .DirIcon are allowed`);
  }
  const appRun = readlinkSync(path.join(dir, "AppRun"));
  if (appRun !== slug) throw new Error(`${label}: AppRun points at ${appRun}, want the binary ${slug}`);

  const unwritable = entries.filter((entry) => entry.link === "" && (entry.mode & 0o022) !== 0);
  if (unwritable.length > 0) {
    throw new Error(
      `${label}: ${unwritable.map((entry) => `${entry.path} (${octal(entry.mode)})`).join(", ")} group- or world-writable; the AppDir sets directories 0755 and files 0644`,
    );
  }
  const node = path.join(dir, ...pin.path.split("/"));
  if (!existsSync(node)) throw new Error(`${label}: ${pin.path} is missing`);
  assertPlatformExecutable(node, pin.platform, `${label}: ${pin.path}`);
  for (const executable of ["AppRun", slug, pin.path]) {
    if ((statSync(path.join(dir, ...executable.split("/"))).mode & 0o111) === 0) {
      throw new Error(`${label}: ${executable} is not executable`);
    }
  }
  if (!existsSync(path.join(dir, AGENT_DIR, DURABLE_HOST))) throw new Error(`${label}: ${AGENT_DIR}/${DURABLE_HOST} is missing`);

  const desktop = readFileSync(path.join(dir, `${slug}.desktop`), "utf8");
  const comment = config.linux?.comment ?? "";
  for (const line of [`Name=${config.name}`, `Exec=${slug}`, ...(comment === "" ? [] : [`Comment=${comment}`])]) {
    if (!desktop.split("\n").includes(line)) throw new Error(`${label}: ${slug}.desktop has no ${line}`);
  }

  const files = entries.filter((entry) => entry.link === "" && !entry.directory);
  const directories = entries.filter((entry) => entry.link === "" && entry.directory);
  const bytes = files.reduce((total, entry) => total + entry.bytes, 0);
  const modes = [...new Set([...files, ...directories].map((entry) => octal(entry.mode)))].sort();
  return [
    `the image holds ${files.length} files, ${bytes} bytes, in ${top.length} top-level entries: ${top.join(", ")}`,
    `the .desktop entry carries Name=${config.name}, Comment=${comment} and Exec=${slug}`,
    `AppRun is a link to ${slug}, ${pin.path} is executable, and ${hasDirIcon ? "the only other link is the spec's .DirIcon" : "no other link is in the image"}`,
    `no file or directory is group- or world-writable (${files.length} files and ${directories.length} directories carry ${modes.join(", ")})`,
  ];
}

/**
 * Proves the built image by extracting it (the runtime's own extraction, the
 * path a machine without FUSE runs the app through) and checking the tree it
 * wrote.
 *
 * The extraction goes to the OS temp directory, not under build/: the runtime
 * reads the image through its own mapping, which a network or 9p filesystem
 * can stall for minutes while a local disk does it in seconds.
 */
async function verifyImage(file: string, slug: string, pin: NodePin, config: PackagingConfig): Promise<string[]> {
  const scratch = mkdtempSync(path.join(tmpdir(), "passquest-appimage-"));
  try {
    const extract = await run(file, ["--appimage-extract"], { cwd: scratch });
    if (extract.code !== 0) {
      throw new Error(`${file} --appimage-extract exited ${extract.code}:\n${extract.stdout}${extract.stderr}`);
    }
    const dir = path.join(scratch, "squashfs-root");
    if (!existsSync(dir)) throw new Error(`${file} --appimage-extract wrote no squashfs-root`);
    return checkImageTree(dir, { label: file, slug, pin, config });
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
}

/** Every file, directory and link under the image root, in path order. */
function walkImage(dir: string, prefix = ""): ImageEntry[] {
  const found: ImageEntry[] = [];
  for (const entry of readdirSync(dir, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
    const relative = prefix === "" ? entry.name : `${prefix}/${entry.name}`;
    const target = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      found.push({ path: relative, link: "", directory: true, mode: statSync(target).mode & 0o7777, bytes: 0 });
      found.push(...walkImage(target, relative));
    } else if (entry.isSymbolicLink()) {
      found.push({ path: relative, link: readlinkSync(target), directory: false, mode: 0, bytes: 0 });
    } else {
      const stat = statSync(target);
      found.push({ path: relative, link: "", directory: false, mode: stat.mode & 0o7777, bytes: stat.size });
    }
  }
  return found;
}

/** A mode as the four-digit octal an error message reads best in. */
function octal(mode: number): string {
  return (mode & 0o7777).toString(8).padStart(4, "0");
}
