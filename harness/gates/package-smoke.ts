/**
 * The package gate (goal 0013, step P5). It builds this run's package for the
 * host target (`buildPackage`, the same sequence `bun run package` runs),
 * installs that run's artifact into a temp prefix (NSIS `/S /D=`, `.dmg`
 * attach and copy, AppImage in place) and drives the installed app through
 * the existing smoke hook (`PASSQUEST_DESKTOP_SCRIPT`/`PASSQUEST_DESKTOP_RESULT`)
 * with the desktop smoke's own scenarios in the new group `package`:
 *
 * - Public study (with a restart and a reinstall): Today, the verified sample,
 *   feedback, the recorded attempt, the same attempts after a relaunch, and
 *   the same data after reinstalling the same version.
 * - The agent features with the package's own Node runtime and hosts: the
 *   coach call and a note refinement against the loopback provider stub, the
 *   Anki push against the loopback AnkiConnect stub and its absence after the
 *   stub stops.
 * - A credential set through the window, absent from every file under the
 *   data root and the install directories (the bytes are searched).
 * - The same app with the bundled Node runtime deleted: practice, assessment
 *   and note drafting still work, and every agent feature reports itself
 *   unavailable with one stated action.
 *
 * The app launches on a PATH with no go, bun or node (asserted), a cwd
 * outside the repository, and an empty per-user root (HOME/AppData/
 * XDG_CONFIG_HOME on a temp tree), so the default data path runs; the install
 * directories are scanned and must hold no database. The credential case on
 * macOS is the exception: the login keychain is found through the real HOME
 * (a temp HOME detaches the app from it), so only its data root moves into
 * the temp tree, through PASSQUEST_DESKTOP_DATA; the stored credential is
 * deleted again the way the desktop smoke's settings scenario cleans up.
 *
 * Before any launch the content scan (package-smoke/contents.ts) walks every
 * file the package installed — the Windows prefix with `Uninstall.exe`, the
 * whole macOS `.app` plus the mounted image's captured root listing, the
 * Linux extract tree — and fails on a PDF, a SQLite database, a private path
 * segment, a `.env` file (its target too, on a link), the internal-only
 * marker or the text of either committed extraction of the internal-only PDF
 * — whole-file copies by SHA-256, and normalised lines as substrings of a
 * view (`package-smoke/contents.ts` carries the views, the 40-byte floor and
 * the disclosed gaps) — with the secrets gate's credential patterns applied
 * through `scanFiles` to the project-owned files only. The run's artifact
 * itself must carry the exact expected name and no PDF/database header; a
 * compressed container is not byte-scanned further, because its extracted
 * tree is what the scan checks.
 *
 * The uninstall at the end is a cleanup sanity check, not proof that the
 * uninstaller spares learner data: an uninstaller that deleted a data
 * directory would resolve $APPDATA through the OS shell folders, not this
 * run's temp root, so the check could not observe it. The guarantee rests on
 * the study case (the data root is outside the install directories, asserted)
 * and on MyGo 0.10.0's uninstall section, which touches no data directory: it
 * removes only $INSTDIR, its shortcuts and its registry key
 * (cmd/mygo/nsis.go:255-266). On
 * Windows a registered install outside this run's temp tree, or the app's
 * Start menu / desktop shortcut, stops the gate before the installer runs
 * (the identifier's HKCU key and MyGo's shortcut name), so a developer's copy
 * is never overwritten or removed; and where the OS temp tree holds a space,
 * the install prefix comes from a short directory under the drive root,
 * because NSIS ignores a quoted `/D=`.
 *
 * The forward-upgrade case (`upgrade`, P7) builds the previous release's CLI,
 * creates learner data through it, opens that data root with the installed
 * app, checks that the open changed no old table's row count, compares every
 * old record field by field, flushes a queued note through the app's own
 * notes room and revises a pushed note through the installed app's own binary
 * (its CLI mode), against an id-persisting Anki stub, and proves a database
 * from a newer schema is refused with a clear mapped message and its bytes
 * untouched; `package-smoke/upgrade.ts` carries the whole sequence, its
 * base-commit constant and its disclosed limits.
 *
 * Disclosed limits: on Linux the runner has no libfuse2, so the AppImage runs
 * via `APPIMAGE_EXTRACT_AND_RUN=1` (the runtime's extract-and-run tree is
 * pointed into the gate's temp tree with TMPDIR and kept for the scan by
 * NO_CLEANUP, and the broken-runtime case edits an `--appimage-extract` copy,
 * because the installed image file is read-only); the macOS `.dmg` CI builds
 * carries no quarantine attribute, so the Gatekeeper prompt a learner meets is
 * not exercised; Windows is the locally verified platform and macOS/Linux are
 * verified by CI's runners.
 */
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, statSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { desktopSurface, startProviderStub, writeStubRegistry } from "../lib/desktop-page.ts";
import type { Check, ProviderStub } from "../lib/desktop-page.ts";
import { removeTree } from "../lib/desktop.ts";
import { buildCLI } from "../lib/go.ts";
import { buildPackage, packageNaming, size } from "../lib/package-build.ts";
import type { PackageNaming } from "../lib/package-build.ts";
import { assertPlatformExecutable } from "../lib/package.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate, GateContext, GateResult } from "../lib/types.ts";

import { installPackage, hostPackageArtifact, HOSTS } from "./package-smoke/install.ts";
import type { InstalledApp } from "./package-smoke/install.ts";
import { scanPackageContents } from "./package-smoke/contents.ts";
import type { PackageIdentity } from "./package-smoke/contents.ts";
import { unavailable } from "./package-smoke/unavailable.ts";
import { upgradeCase } from "./package-smoke/upgrade.ts";
import { anki } from "./desktop-smoke/anki.ts";
import { assess } from "./desktop-smoke/assess.ts";
import { boot } from "./desktop-smoke/boot.ts";
import { coach } from "./desktop-smoke/coach.ts";
import { feedback } from "./desktop-smoke/feedback.ts";
import { practice } from "./desktop-smoke/practice.ts";
import { cli } from "./desktop-smoke/scenario.ts";
import type { ScenarioRun, Shared } from "./desktop-smoke/scenario.ts";
import { settings } from "./desktop-smoke/settings.ts";

/**
 * The cases, in the order they run; a later case depends on the app the
 * earlier ones proved. The content scan runs first, before any launch, when
 * the tree still holds exactly what the package shipped.
 */
const CASES = ["contents", "layout", "study", "agents", "anki", "upgrade", "credential", "broken"] as const;

export interface PackageSmokeOptions {
  /**
   * A tree that stands in for an installed package, relative to the gate root:
   * the self-test drives the gate over one without building a package. The
   * tree carries the app binary the smoke hook launches (passquest-desktop on
   * PATH's spelling), a `node/` runtime file and the four `agent/` hosts; the
   * cases still run, but no installer does.
   */
  installedTree?: string;
  /** The cases to run, in their usual order; all by default. */
  only?: readonly string[];
}

/**
 * The package smoke over all cases, or, for a self-test case, over the ones
 * `only` names (kept in their usual order).
 */
export function makePackageSmokeGate(options: PackageSmokeOptions = {}): Gate {
  const only = options.only;
  const cases = (only ? CASES.filter((id) => only.includes(id)) : [...CASES]) as readonly (typeof CASES)[number][];
  return { id: "package-smoke", title: "package smoke", run: (ctx) => runPackageSmoke(ctx, options, cases) };
}

export const packageSmoke = makePackageSmokeGate();

/** The app's data directory for a temp per-user root, as the platform resolves it. */
function dataRootFor(home: string, appName: string): string {
  if (process.platform === "win32") return path.join(home, "AppData", appName);
  if (process.platform === "darwin") return path.join(home, "Library", "Application Support", appName);
  return path.join(home, ".config", appName); // os.UserConfigDir reads XDG_CONFIG_HOME
}

/**
 * The environment variables that point os.UserConfigDir (and on Windows
 * os.UserCacheDir: MyGo creates its WebView2 folder under %LOCALAPPDATA%) at
 * a temp root. They are spelled as the inherited environment spells them: a
 * launch whose block carries both `APPDATA` and `AppData` can have the app
 * read the inherited one, so the gate replaces the entry that is already
 * there.
 */
function homeEnv(home: string): Record<string, string> {
  if (process.platform === "win32") return { APPDATA: path.join(home, "AppData"), LOCALAPPDATA: path.join(home, "LocalAppData") };
  if (process.platform === "darwin") return { HOME: home };
  return { HOME: home, XDG_CONFIG_HOME: path.join(home, ".config") };
}

/** The developer tools no launch may find on its PATH. */
const DEVELOPER_TOOLS = ["go", "bun", "node"];

/**
 * The system directories only: the WebView2 loader and the OS tools live there.
 * A runner image can also link a developer tool into one of them (ubuntu-latest
 * has /usr/bin/go), so on macOS and Linux a system directory that holds one is
 * replaced by a shadow directory under `temp` that links every other entry.
 */
function strippedPath(temp: string): string {
  if (process.platform === "win32") {
    const root = process.env.SystemRoot ?? "C:\\Windows";
    return [path.join(root, "System32"), root].join(";");
  }
  return ["/usr/bin", "/bin", "/usr/sbin", "/sbin"]
    .map((dir, index) => {
      if (!DEVELOPER_TOOLS.some((tool) => existsSync(path.join(dir, tool)))) return dir;
      const shadow = path.join(temp, "path", String(index));
      mkdirSync(shadow, { recursive: true });
      for (const entry of readdirSync(dir)) {
        if (!DEVELOPER_TOOLS.includes(entry)) symlinkSync(path.join(dir, entry), path.join(shadow, entry));
      }
      return shadow;
    })
    .join(":");
}

/** Every `passquest.sqlite` under a tree; the data must never land under the install. */
function databasesUnder(dir: string): string[] {
  const found: string[] = [];
  if (!existsSync(dir)) return found;
  const walk = (current: string): void => {
    for (const entry of readdirSync(current, { withFileTypes: true })) {
      const child = path.join(current, entry.name);
      if (entry.isDirectory()) walk(child);
      else if (entry.name === "passquest.sqlite") found.push(child);
    }
  };
  walk(dir);
  return found;
}

/** A tree's files and their bytes as one digest: paths, sizes and content hashes, in path order. */
function treeDigest(dir: string): string {
  const hash = createHash("sha256");
  const files: string[] = [];
  const walk = (current: string, prefix: string): void => {
    for (const entry of readdirSync(current, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
      const child = path.join(current, entry.name);
      const relative = prefix === "" ? entry.name : `${prefix}/${entry.name}`;
      if (entry.isDirectory()) walk(child, relative);
      else files.push(relative);
    }
  };
  walk(dir, "");
  for (const file of files.sort()) {
    const full = path.join(dir, ...file.split("/"));
    hash.update(file);
    hash.update(String(statSync(full).size));
    hash.update(readFileSync(full));
  }
  return hash.digest("hex");
}

/** The tree a fixture install carries, for the self-test: no installer, no build. */
function fixtureApp(tree: string): InstalledApp {
  const binary = path.join(tree, process.platform === "win32" ? "passquest-desktop.exe" : "passquest-desktop");
  const runtime = process.platform === "win32" ? path.join(tree, "node", "node.exe") : path.join(tree, "node", "bin", "node");
  const hosts = HOSTS.map((host) => path.join(tree, "agent", host));
  return {
    binary,
    installDirs: [tree],
    runtime,
    hosts,
    launchEnv: {},
    broken: { binary, launchEnv: {} },
    reinstall: async () => {},
    breakRuntime: () => {
      rmSync(runtime, { force: true });
      return runtime;
    },
    cleanup: async () => [],
  };
}

async function runPackageSmoke(ctx: GateContext, options: PackageSmokeOptions, cases: readonly string[]): Promise<GateResult> {
  if (!["win32", "darwin", "linux"].includes(process.platform)) {
    return skip(`the package smoke has no install path for ${process.platform}`);
  }
  const unknown = (options.only ?? []).filter((id) => !(CASES as readonly string[]).includes(id));
  if (unknown.length > 0) return fail([`unknown package-smoke case(s) ${unknown.join(", ")}; the cases are ${CASES.join(", ")}`]);
  if (cases.length === 0) return fail(["no package-smoke case was selected"]);
  const surface = desktopSurface(ctx);
  if (surface) return surface;
  let raw: string;
  try {
    raw = readFileSync(path.resolve(ctx.root, "mygo.json"), "utf8");
  } catch (error) {
    return fail([`mygo.json cannot be read: ${error instanceof Error ? error.message : String(error)}`]);
  }
  let config: { name?: unknown; version?: unknown };
  try {
    config = JSON.parse(raw) as { name?: unknown; version?: unknown };
  } catch (error) {
    return fail([`mygo.json is not JSON: ${error instanceof Error ? error.message : String(error)}`]);
  }
  if (typeof config.name !== "string" || config.name.trim() === "") return fail(["mygo.json carries no name; the app's data directory is named after it"]);
  let naming: PackageNaming;
  try {
    naming = packageNaming(raw);
  } catch (error) {
    return fail([`mygo.json cannot name the package: ${error instanceof Error ? error.message : String(error)}`]);
  }

  const temp = mkdtempSync(path.join(tmpdir(), "passquest-package-smoke-"));
  const scratch = path.join(temp, "scratch");
  mkdirSync(scratch, { recursive: true });
  const checks: string[] = [];
  let assertions = 0;
  const check: Check = (condition, message) => {
    assertions++;
    if (!condition) checks.push(message);
    return condition;
  };
  const provider: ProviderStub = startProviderStub();
  const registry = path.join(scratch, "registry.json");
  writeStubRegistry(registry, provider.endpoint);
  let app: InstalledApp | undefined;
  let identity: PackageIdentity | undefined;
  let packageLine = "";
  let studyData = "";
  let scanLine = "";
  let upgradeLine = "";
  try {
    if (options.installedTree !== undefined) {
      const tree = path.resolve(ctx.root, options.installedTree);
      if (!existsSync(tree)) checks.push(`the fixture install tree ${options.installedTree} does not exist`);
      else {
        app = fixtureApp(tree);
        packageLine = "a fixture install tree (no package built)";
      }
    } else {
      const buildLog: string[] = [];
      const built = await buildPackage(ctx, { log: (line) => buildLog.push(line), err: (line) => buildLog.push(line) });
      if (!built.ok) {
        // MyGo's own output went through the sinks into buildLog.
        checks.push("the package build failed", ...built.failures, ...buildLog.slice(-8));
      } else {
        try {
          const artifact = hostPackageArtifact(built.build);
          app = await installPackage(built.build, temp);
          packageLine = `${artifact.name} ${size(artifact.bytes)} (staged ${built.build.staged.files} files)`;
          identity = { artifact, naming, config: { name: config.name, version: typeof config.version === "string" ? config.version : "" } };
        } catch (error) {
          checks.push(`installing the package failed: ${error instanceof Error ? error.message : String(error)}`, ...buildLog.slice(-8));
        }
      }
    }

    // The PATH handed to every launch: no developer tool may resolve on it.
    const launchPath = strippedPath(temp);
    for (const tool of DEVELOPER_TOOLS) {
      const resolved = Bun.which(tool, { PATH: launchPath });
      check(resolved === null, `environment: ${tool} resolves on the stripped PATH at ${String(resolved)}`);
    }

    let cliBinary = "";
    const ensureCli = async (): Promise<boolean> => {
      if (cliBinary !== "") return true;
      const built = await buildCLI(ctx);
      if ("failure" in built) {
        check(false, `the repository CLI did not build: ${built.failure.split("\n").slice(-4).join(" ")}`);
        return false;
      }
      cliBinary = built.binary;
      return true;
    };
    const needsCli =
      cases.includes("study") || cases.includes("agents") || cases.includes("anki") || cases.includes("credential") || cases.includes("broken");
    const ready = app !== undefined && (!needsCli || (await ensureCli()));

    /** One case's launch site: its own temp cwd, per-user root and credential service. */
    const siteOf = (
      id: string,
      binary: string,
      extraEnv: Record<string, string> = {},
      site: { keepHome?: boolean; setData?: boolean } = {},
    ): { run: ScenarioRun; dataRoot: string } => {
      const home = path.join(scratch, `home-${id}`);
      mkdirSync(home, { recursive: true });
      const dir = path.join(scratch, `case-${id}`);
      mkdirSync(dir, { recursive: true });
      const cwd = path.join(dir, "cwd");
      mkdirSync(cwd, { recursive: true });
      const dataRoot = dataRootFor(home, config.name as string);
      const shared: Shared = {
        ctx,
        binary,
        cli: cliBinary,
        registry,
        provider,
        cwd,
        launchEnv: { PATH: launchPath, ...(site.keepHome === true ? {} : homeEnv(home)), ...extraEnv },
        dataOverride: site.setData === true,
        secretScanRoots: app!.installDirs,
      };
      return {
        dataRoot,
        run: {
          shared,
          id,
          dir,
          dataDir: dataRoot,
          database: path.join(dataRoot, "passquest.sqlite"),
          service: `passquest-package-${process.pid}-${Date.now()}-${id}`,
          check,
        },
      };
    };

    const studies: Record<string, () => Promise<void>> = {
      contents: async () => {
        scanLine = await scanPackageContents(app!, ctx, check, identity);
      },
      layout: async () => {
        for (const file of [app!.runtime, ...app!.hosts]) {
          check(existsSync(file), `layout: the installed tree is missing ${file}`);
        }
        if (existsSync(app!.runtime)) {
          try {
            assertPlatformExecutable(app!.runtime, process.platform as "win32" | "darwin" | "linux", app!.runtime);
          } catch (error) {
            check(false, `layout: ${error instanceof Error ? error.message : String(error)}`);
          }
        }
      },
      study: async () => {
        const site = siteOf("study", app!.binary, app!.launchEnv);
        studyData = site.dataRoot;
        await boot.run(site.run);
        const inside = app!.installDirs.flatMap((dir) => databasesUnder(dir));
        const database = existsSync(site.run.database);
        if (inside.length > 0) check(false, `study: the first launch wrote its database inside the install directory: ${inside.join(", ")}`);
        if (!database) check(false, `study: the first launch created no database at the per-user location ${site.run.database}`);
        if (inside.length > 0 || !database) return;
        await feedback.run(site.run);
        const before = (await cli(site.run, ["report"])).stdout;
        await boot.run(site.run);
        check((await cli(site.run, ["report"])).stdout === before, "study: the recorded attempt changed across a relaunch");
        const digest = treeDigest(site.dataRoot);
        await app!.reinstall();
        check(treeDigest(site.dataRoot) === digest, "study: reinstalling the same version changed the learner's data");
        await boot.run(site.run);
        check((await cli(site.run, ["report"])).stdout === before, "study: the attempt report changed across the reinstall");
      },
      agents: async () => {
        await coach.run(siteOf("agents", app!.binary, app!.launchEnv).run);
      },
      anki: async () => {
        await anki.run(siteOf("anki", app!.binary, app!.launchEnv).run);
      },
      upgrade: async () => {
        upgradeLine = await upgradeCase({
          ctx,
          check,
          temp,
          app: { binary: app!.binary, runtime: app!.runtime, hosts: app!.hosts },
          site: (id) => siteOf(id, app!.binary, app!.launchEnv),
        });
      },
      credential: async () => {
        // macOS: the credential store is the file-based login keychain, found
        // through ~/Library, so the real HOME stays and only the data root
        // moves into the temp tree; the study case exercises the default data
        // path with the redirected HOME.
        const keepHome = process.platform === "darwin";
        await settings.run(siteOf("credential", app!.binary, app!.launchEnv, { keepHome, setData: keepHome }).run);
      },
      broken: async () => {
        const removed = app!.breakRuntime();
        if (check(!existsSync(removed), `broken: the bundled runtime is still at ${removed}`)) {
          for (const host of app!.hosts) check(existsSync(host), `broken: the bundled host ${host} went with the runtime`);
        }
        await unavailable.run(siteOf("broken", app!.broken.binary, app!.broken.launchEnv).run);
        await practice.run(siteOf("broken-practice", app!.broken.binary, app!.broken.launchEnv).run);
        await assess.run(siteOf("broken-assess", app!.broken.binary, app!.broken.launchEnv).run);
      },
    };

    if (ready) {
      for (const id of cases) {
        const before = checks.length;
        await studies[id]!();
        // A failed layout or study says the app's own storage is wrong; the
        // later cases would only pile their own failures on top.
        if (checks.length > before && (id === "layout" || id === "study")) break;
      }
    }
  } catch (error) {
    // A case that throws (a failed reinstall, a stuck detach) joins the
    // failures already collected instead of discarding them.
    checks.push(`the package smoke stopped: ${error instanceof Error ? error.message : String(error)}`);
  } finally {
    if (app !== undefined && options.installedTree === undefined) {
      // Every exit path removes the install (the uninstaller on Windows, the
      // prefix removal on macOS/Linux); a cleanup problem is reported, never
      // thrown, so it cannot mask a case's failure.
      try {
        for (const problem of await app.cleanup()) checks.push(`cleanup: ${problem}`);
      } catch (error) {
        checks.push(`cleanup failed: ${error instanceof Error ? error.message : String(error)}`);
      }
      // Sanity, not proof: an uninstaller deleting a data directory would
      // resolve $APPDATA through the OS shell folders, not this temp root. The
      // guarantee is the study case (data root outside the install) plus
      // MyGo 0.10.0's uninstall section, which removes $INSTDIR, its
      // shortcuts and its registry key only (cmd/mygo/nsis.go:255-266).
      if (studyData !== "" && !existsSync(path.join(studyData, "passquest.sqlite"))) {
        checks.push(`cleanup: uninstalling removed the learner's data at ${path.join(studyData, "passquest.sqlite")}`);
      }
    }
    provider.stop();
    // The tree holds the install, the cases' data roots and the WebView2
    // user-data folders: WebView2 or a node child may still be releasing a
    // handle, so removal retries; a tree that survives is reported, never
    // thrown, because a throw here would discard every check collected above.
    if (!(await removeTree(temp))) checks.push(`cleanup: could not remove the temp tree ${temp}`);
  }
  if (checks.length > 0) return fail(checks);
  return pass(
    `${assertions} assertions over ${cases.join(", ")}; ${packageLine}${scanLine === "" ? "" : `; ${scanLine}`}${
      upgradeLine === "" ? "" : `; ${upgradeLine}`
    }`,
  );
}
