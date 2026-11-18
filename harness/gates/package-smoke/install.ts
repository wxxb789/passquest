/**
 * Installs a built package into a temp prefix, and removes it again (goal
 * 0013, step P5). One path per OS, for what each build actually emits:
 *
 * - Windows: the NSIS setup, `/S /D=<prefix>` — `/D=` last and unquoted, as
 *   NSIS requires. Bun quotes any argument that holds a space, and NSIS
 *   ignores a quoted `/D=`, so the prefix must be space-free: where the OS
 *   temp tree holds one (a user name with a space), the install goes to a
 *   short directory under the drive root instead, which Windows lets a
 *   standard user create. The setup installs per user with no admin and is
 *   synchronous. Before it runs, a registered install outside this run's temp
 *   tree stops the install (the app's HKCU uninstall key), and so does the
 *   app's Start menu or desktop shortcut: the installer writes the same key
 *   and the same two shortcuts a learner's copy uses, the shortcuts through
 *   the real shell folders (`$SMPROGRAMS`, `$DESKTOP`), which `/D=` and the
 *   environment redirection do not move, so a developer's copy is never
 *   overwritten. The uninstaller copies itself to %TEMP% and returns at once,
 *   so removal waits, bounded, for the prefix to disappear; MyGo 0.10.0's
 *   uninstall section removes $INSTDIR, its shortcuts and its registry key
 *   only (cmd/mygo/nsis.go:255-266). A failure that leaves a prefix under the
 *   drive root, where no uninstaller can remove it and the gate's own temp
 *   removal does not reach, removes the prefix here.
 * - macOS: the `.dmg` is attached read-only, the `.app` copied out with
 *   `ditto` (symlinks and modes survive a bundle holding them — a hand-rolled
 *   walk that drops an entry would silently test a different bundle) and the
 *   image detached; the copied bundle is the install. Reinstalling copies the
 *   bundle out of a fresh attach over the old one. The bundle a CI-built dmg
 *   carries has no quarantine attribute (nothing downloaded it), so the
 *   Gatekeeper prompt a learner sees is not exercised here — a disclosed
 *   limit; the install guide documents that path.
 * - Linux: the AppImage is copied into the prefix and runs there with
 *   `APPIMAGE_EXTRACT_AND_RUN=1`, because the runner has no libfuse2 (a
 *   disclosed limit). The type2-runtime unpacks the image under `$TMPDIR`
 *   when it runs it and deletes that tree when the app exits unless
 *   `NO_CLEANUP` is set (type2-runtime `src/runtime/runtime.c`, the
 *   `APPIMAGE_EXTRACT_AND_RUN` branch), so the launch environment points
 *   `TMPDIR` into the gate's temp tree and sets `NO_CLEANUP`: the tree the
 *   app actually ran from is scanned like the install prefix. A separate
 *   `--appimage-extract` copy beside the prefix is what the broken-runtime
 *   case edits: the image file itself is read-only.
 */
import { chmodSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, readdirSync, rmSync } from "node:fs";
import path from "node:path";

import { sleep, tail } from "../../lib/desktop-page.ts";
import { removeTree } from "../../lib/desktop.ts";
import type { PackageArtifact, PackageBuild } from "../../lib/package-build.ts";
import { executable, run } from "../../lib/process.ts";

/** The four host scripts every package carries, as internal/hostpaths looks them up. */
export const HOSTS = ["tutor-host.mjs", "anki-host.mjs", "search-host.mjs", "durable-host.mjs"] as const;

export interface InstalledApp {
  /** The launcher the smoke hook drives: the installed app itself. */
  binary: string;
  /**
   * Every directory the installed copy occupies: the prefix and, on Linux,
   * the trees the AppImage is unpacked into. No learner data and no
   * credential may appear under them.
   */
  installDirs: readonly string[];
  /** The bundled Node runtime inside the installed copy. */
  runtime: string;
  /** The four bundled host scripts inside the installed copy. */
  hosts: string[];
  /** Environment every launch gets beyond the smoke hook's own (Linux runs the AppImage from its own extraction). */
  launchEnv: Record<string, string>;
  /** How the app launches once the bundled runtime is deleted (Linux: the extracted AppRun). */
  broken: { binary: string; launchEnv: Record<string, string> };
  /** Installs the same package over the same prefix again (the reinstall case). */
  reinstall: () => Promise<void>;
  /** Deletes the bundled runtime from the installed copy; returns the path removed. */
  breakRuntime: () => string;
  /** Removes the installation; the messages name anything that survived. */
  cleanup: () => Promise<string[]>;
}

/** Polls until `ready` or the deadline passes; true when it became ready. */
async function waitFor(ready: () => boolean, timeoutMs: number, stepMs = 500): Promise<boolean> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    if (ready()) return true;
    if (Date.now() > deadline) return false;
    await sleep(stepMs);
  }
}

/** One file of a directory, by predicate, or a failure naming what was there. */
function oneFile(dir: string, want: string, test: (name: string) => boolean): string {
  if (!existsSync(dir)) throw new Error(`${want}: ${dir} does not exist`);
  const found = readdirSync(dir).filter(test);
  if (found.length !== 1) throw new Error(`${want}: want exactly one file in ${dir}; it holds ${found.join(", ") || "none"}`);
  return path.join(dir, found[0]!);
}

/**
 * The directory this run installs into. NSIS's `/D=` must be unquoted; Bun
 * quotes any argument that holds a space, and NSIS then ignores the switch,
 * so a temp tree that holds one gives way to a short directory under the
 * drive root, which Windows lets a standard user create.
 */
function installPrefix(temp: string): string {
  const wanted = path.join(temp, "install");
  if (process.platform !== "win32" || !wanted.includes(" ")) return wanted;
  const root = path.parse(temp).root;
  let problem = `${root} holds a space too`;
  try {
    const short = mkdtempSync(path.join(root, "pq-pkg-"));
    if (!short.includes(" ")) return short;
  } catch (error) {
    problem = error instanceof Error ? error.message : String(error);
  }
  throw new Error(`no space-free install prefix: ${temp} holds a space and ${problem}; point TEMP at a path without spaces`);
}

/** The install location the app's uninstall key registers, or null when the key is not there. */
async function registeredInstallDir(identifier: string): Promise<{ present: boolean; location: string | null }> {
  const key = `HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\${identifier}`;
  const value = await run(executable("reg"), ["query", key, "/v", "InstallLocation"]);
  if (value.code !== 0) {
    // A value query fails for a missing key and a missing value alike; the key query tells them apart.
    const keyQuery = await run(executable("reg"), ["query", key]);
    return { present: keyQuery.code === 0, location: null };
  }
  const line = value.stdout.split(/\r?\n/).find((candidate) => candidate.includes("InstallLocation"));
  return { present: true, location: line?.split(/REG_SZ\s+/)[1]?.trim().replace(/^"(.*)"$/, "$1") ?? null };
}

/** Whether child resolves inside root, compared the way Windows compares paths. */
function isUnder(child: string, root: string): boolean {
  return path.resolve(child).toLowerCase().startsWith(path.resolve(root).toLowerCase() + path.sep);
}

/**
 * One HKCU shell folder (`Explorer\Shell Folders`), as the OS itself resolves
 * it: a redirected or OneDrive Desktop is found, where `%USERPROFILE%` had
 * missed it. An unreadable value is a refusal, not an assumption: the
 * shortcut check exists to protect a real copy.
 */
async function shellFolder(name: "Desktop" | "Programs"): Promise<string> {
  const key = "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Explorer\\Shell Folders";
  const query = await run(executable("reg"), ["query", key, "/v", name]);
  const line = query.stdout.split(/\r?\n/).find((candidate) => candidate.includes(name));
  const value = line?.split(/REG_SZ\s+/)[1]?.trim().replace(/^"(.*)"$/, "$1");
  if (query.code !== 0 || value === undefined || value === "") {
    throw new Error(
      `windows: cannot read the ${name} shell folder (reg query exited ${query.code}); refusing to install, because a real Pass Quest shortcut there cannot be ruled out`,
    );
  }
  return value;
}

/**
 * Refuses to install over a real copy. The installer writes the same HKCU
 * uninstall key as a learner's copy, and its Start menu and desktop shortcuts
 * through the real shell folders (`$SMPROGRAMS`, `$DESKTOP`), which `/D=` and
 * this run's environment redirection do not move: a registration outside this
 * run's temp tree, or either shortcut, must be cleared by its owner first,
 * never silently overwritten and then deleted by this run's cleanup.
 */
async function assertNoRealInstall(identifier: string, shortcut: string, temp: string): Promise<void> {
  const registered = await registeredInstallDir(identifier);
  if (registered.present) {
    const location = registered.location;
    if (location === null || location === "" || !isUnder(location, temp)) {
      throw new Error(
        `windows: ${identifier} is already registered with InstallLocation ${location ?? "(unreadable)"} — a real install outside this run's temp tree (${temp}); uninstall it first (Settings > Apps, or its Uninstall.exe /S)`,
      );
    }
  }
  const shortcuts = [path.join(await shellFolder("Programs"), `${shortcut}.lnk`), path.join(await shellFolder("Desktop"), `${shortcut}.lnk`)].filter(
    (file) => existsSync(file),
  );
  if (shortcuts.length > 0) {
    throw new Error(
      `windows: ${shortcuts.join(", ")} ${shortcuts.length === 1 ? "exists" : "exist"} — the app's Start menu and desktop shortcuts live in the real shell folders, whichever /D= says, and the installer would overwrite and its uninstaller delete ${shortcuts.length === 1 ? "it" : "them"}; uninstall the real copy first (Settings > Apps, or its Uninstall.exe /S), or delete ${shortcuts.length === 1 ? "the shortcut" : "the shortcuts"} once no install remains`,
    );
  }
}

/** Windows: the NSIS setup into a space-free prefix, the app discovered beside Uninstall.exe. */
async function installWindows(build: PackageBuild, temp: string): Promise<InstalledApp> {
  const setup = build.setup;
  if (!setup) throw new Error(`windows: ${build.dir} holds no NSIS installer (*-setup-*.exe)`);
  // Before anything is created or written: a registration outside this run's
  // temp tree, or one of the app's shortcuts in the real shell folders, is a
  // real copy, and the installer would adopt its key and overwrite its
  // shortcuts.
  await assertNoRealInstall(build.identifier, build.shortcut, temp);
  const prefix = installPrefix(temp);
  const uninstaller = path.join(prefix, "Uninstall.exe");
  // A step after a successful /S leaves a real install behind: remove it
  // before reporting, so a failed run leaves no registration, and report a
  // failed removal beside the first failure, never instead of it. A prefix
  // without an uninstaller (the installer failed before writing one) cannot
  // be removed that way: when it lives outside the gate's temp tree, where
  // the gate's own removal does not reach, remove it here, or a failure
  // leaks a directory under the drive root.
  const bail = async (message: string): Promise<never> => {
    if (existsSync(uninstaller)) {
      const removed = await run(uninstaller, ["/S"]);
      const gone = await waitFor(() => !existsSync(prefix), 120000, 1000);
      if (removed.code !== 0 || !gone) {
        throw new Error(`${message}; removing the half-made install also failed (${uninstaller} /S exited ${removed.code}, prefix ${gone ? "removed" : "survived"})`);
      }
      throw new Error(message);
    }
    if (!isUnder(prefix, temp) && !(await removeTree(prefix))) {
      throw new Error(`${message}; the half-made prefix ${prefix} could not be removed either`);
    }
    throw new Error(message);
  };
  const install = await run(setup.path, ["/S", `/D=${prefix}`]);
  if (install.code !== 0) return bail(`windows: ${setup.name} /S exited ${install.code}: ${tail(`${install.stdout}${install.stderr}`, 6)}`);
  if (!(await waitFor(() => existsSync(prefix), 60000))) return bail(`windows: ${setup.name} /S wrote no ${prefix}`);
  let binary: string;
  try {
    binary = oneFile(prefix, "windows: the installer wrote no app executable", (name) => name.endsWith(".exe") && name !== "Uninstall.exe");
  } catch (error) {
    return bail(error instanceof Error ? error.message : String(error));
  }
  return {
    binary,
    installDirs: [prefix],
    runtime: path.join(prefix, "node", "node.exe"),
    hosts: HOSTS.map((host) => path.join(prefix, "agent", host)),
    launchEnv: {},
    broken: { binary, launchEnv: {} },
    async reinstall() {
      const again = await run(setup.path, ["/S", `/D=${prefix}`]);
      if (again.code !== 0) throw new Error(`windows: reinstalling ${setup.name} exited ${again.code}: ${tail(`${again.stdout}${again.stderr}`, 6)}`);
      if (!existsSync(binary)) throw new Error(`windows: reinstalling ${setup.name} left no ${binary}`);
    },
    breakRuntime: () => {
      const runtime = path.join(prefix, "node", "node.exe");
      rmSync(runtime, { force: true });
      return runtime;
    },
    async cleanup() {
      if (!existsSync(uninstaller)) {
        // No uninstaller: a prefix outside the temp tree would leak, since
        // the gate's own removal reaches only its temp tree.
        if (!isUnder(prefix, temp) && !(await removeTree(prefix))) {
          return [`windows: the installed tree holds no Uninstall.exe and ${prefix} could not be removed`];
        }
        return [`windows: the installed tree holds no Uninstall.exe to remove it with`];
      }
      const remove = await run(uninstaller, ["/S"]);
      if (remove.code !== 0) return [`windows: ${uninstaller} /S exited ${remove.code}: ${tail(`${remove.stdout}${remove.stderr}`, 4)}`];
      // The uninstaller copies itself to %TEMP% and returns; the copy removes the prefix.
      if (!(await waitFor(() => !existsSync(prefix), 120000, 1000))) return [`windows: the install directory survived the uninstaller: ${prefix}`];
      return [];
    },
  };
}

/** macOS: the .dmg attached read-only, its .app copied into `prefix` with ditto, the image detached. */
async function installMac(build: PackageBuild, prefix: string, temp: string): Promise<InstalledApp> {
  const dmg = build.dmg;
  if (!dmg) throw new Error(`darwin: ${build.dir} holds no .dmg`);
  const app = path.join(prefix, "Pass Quest.app");
  const copyOut = async (): Promise<void> => {
    const mount = path.join(temp, "dmg");
    mkdirSync(mount, { recursive: true });
    const attach = await run("hdiutil", ["attach", "-nobrowse", "-readonly", "-mountpoint", mount, dmg.path]);
    if (attach.code !== 0) throw new Error(`darwin: hdiutil attach ${dmg.name} exited ${attach.code}: ${tail(`${attach.stdout}${attach.stderr}`, 4)}`);
    try {
      const bundle = oneFile(mount, `darwin: ${dmg.name} holds no .app`, (name) => name.endsWith(".app"));
      rmSync(app, { recursive: true, force: true });
      mkdirSync(prefix, { recursive: true });
      const copy = await run("ditto", [bundle, app]);
      if (copy.code !== 0) throw new Error(`darwin: ditto ${bundle} ${app} exited ${copy.code}: ${tail(`${copy.stdout}${copy.stderr}`, 4)}`);
    } finally {
      const detach = await run("hdiutil", ["detach", mount]);
      if (detach.code !== 0) throw new Error(`darwin: hdiutil detach ${mount} exited ${detach.code}: ${tail(`${detach.stdout}${detach.stderr}`, 4)}`);
    }
  };
  await copyOut();
  const macos = path.join(app, "Contents", "MacOS");
  const binary = oneFile(macos, "darwin: the .app holds no executable", () => true);
  const runtime = path.join(app, "Contents", "Resources", "node", "bin", "node");
  return {
    binary,
    installDirs: [prefix],
    runtime,
    hosts: HOSTS.map((host) => path.join(app, "Contents", "Resources", "agent", host)),
    launchEnv: {},
    broken: { binary, launchEnv: {} },
    reinstall: copyOut,
    breakRuntime: () => {
      rmSync(runtime, { force: true });
      return runtime;
    },
    async cleanup() {
      rmSync(prefix, { recursive: true, force: true });
      return existsSync(prefix) ? [`darwin: could not remove ${prefix}`] : [];
    },
  };
}

/** Linux: the AppImage copied into `prefix`, its contents extracted beside the prefix. */
async function installLinux(build: PackageBuild, prefix: string, temp: string): Promise<InstalledApp> {
  const image = build.appimage;
  if (!image) throw new Error("linux: the build produced no AppImage (the step runs during the Linux build; see bun run package)");
  const copy = path.join(prefix, path.basename(image.path));
  const extracted = path.join(temp, "appimage");
  // Where the runtime unpacks the image when it runs it (the TMPDIR below).
  const runtimeTree = path.join(temp, "appimage-run");
  mkdirSync(prefix, { recursive: true });
  mkdirSync(extracted, { recursive: true });
  mkdirSync(runtimeTree, { recursive: true });
  copyFileSync(image.path, copy);
  chmodSync(copy, 0o755);
  const unpack = await run(copy, ["--appimage-extract"], { cwd: extracted });
  if (unpack.code !== 0) throw new Error(`linux: ${path.basename(copy)} --appimage-extract exited ${unpack.code}: ${tail(`${unpack.stdout}${unpack.stderr}`, 4)}`);
  const tree = path.join(extracted, "squashfs-root");
  if (!existsSync(tree)) throw new Error(`linux: ${path.basename(copy)} --appimage-extract wrote no squashfs-root`);
  const runtime = path.join(tree, "node", "bin", "node");
  return {
    binary: copy,
    installDirs: [prefix, extracted, runtimeTree],
    runtime,
    hosts: HOSTS.map((host) => path.join(tree, "agent", host)),
    launchEnv: { APPIMAGE_EXTRACT_AND_RUN: "1", TMPDIR: runtimeTree, NO_CLEANUP: "1" },
    broken: { binary: path.join(tree, "AppRun"), launchEnv: {} },
    async reinstall() {
      copyFileSync(image.path, copy);
      chmodSync(copy, 0o755);
    },
    breakRuntime: () => {
      rmSync(runtime, { force: true });
      return runtime;
    },
    async cleanup() {
      rmSync(prefix, { recursive: true, force: true });
      return existsSync(prefix) ? [`linux: could not remove ${prefix}`] : [];
    },
  };
}

/** Installs the built package for this OS beneath `temp`: `<temp>/install`, or a space-free prefix on Windows. */
export async function installPackage(build: PackageBuild, temp: string): Promise<InstalledApp> {
  switch (process.platform) {
    case "win32":
      return installWindows(build, temp);
    case "darwin":
      return installMac(build, path.join(temp, "install"), temp);
    case "linux":
      return installLinux(build, path.join(temp, "install"), temp);
    default:
      throw new Error(`the package smoke has no install path for ${process.platform}`);
  }
}

/** The artifact a build must carry for this OS, or a failure naming what is missing. */
export function hostPackageArtifact(build: PackageBuild): PackageArtifact {
  const artifact = process.platform === "win32" ? build.setup : process.platform === "darwin" ? build.dmg : build.appimage;
  if (!artifact) throw new Error(`the build wrote no package for ${process.platform} in ${build.dir}`);
  return artifact;
}
