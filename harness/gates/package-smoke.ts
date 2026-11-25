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
 * Disclosed limits: on Linux the AppImage is never mounted with FUSE; it runs
 * via `APPIMAGE_EXTRACT_AND_RUN=1` (the runtime's extract-and-run tree is
 * pointed into the gate's temp tree with TMPDIR and kept for the scan by
 * NO_CLEANUP, and the broken-runtime case edits an `--appimage-extract` copy,
 * because the installed image file is read-only); the macOS `.dmg` CI builds
 * carries no quarantine attribute, so the Gatekeeper prompt a learner meets is
 * not exercised; Windows is the locally verified platform and macOS/Linux are
 * verified by CI's runners.
 */
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, statSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import {
  cleanEnv,
  dataRootFor,
  desktopSurface,
  homeEnv,
  parse,
  startProviderStub,
  STUB_CREDENTIAL_ENV,
  STUB_MODEL,
  stubResponsesProvider,
  writeStubRegistry,
} from "../lib/desktop-page.ts";
import type { Check, ProviderStub } from "../lib/desktop-page.ts";
import { removeTree } from "../lib/desktop.ts";
import { buildCLI } from "../lib/go.ts";
import { stringField } from "../lib/json.ts";
import { buildPackage, packageNaming, size } from "../lib/package-build.ts";
import type { PackageNaming } from "../lib/package-build.ts";
import { assertPlatformExecutable, DURABLE_HOST, NODE_PIN_SOURCES, NODE_VERSION, nodePinProblems, readNodePinSources } from "../lib/package.ts";
import type { StagedNotices } from "../lib/notices.ts";
import { run, tail } from "../lib/process.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate, GateContext, GateResult } from "../lib/types.ts";

import { installPackage, hostPackageArtifact, HOSTS } from "./package-smoke/install.ts";
import type { InstalledApp } from "./package-smoke/install.ts";
import { scanPackageContents } from "./package-smoke/contents.ts";
import type { PackageIdentity } from "./package-smoke/contents.ts";
import { brief, briefResponses } from "./package-smoke/brief.ts";
import { unavailable } from "./package-smoke/unavailable.ts";
import { upgradeCase } from "./package-smoke/upgrade.ts";
import { anki } from "./desktop-smoke/anki.ts";
import { assess } from "./desktop-smoke/assess.ts";
import { boot } from "./desktop-smoke/boot.ts";
import { coach } from "./desktop-smoke/coach.ts";
import { feedback } from "./desktop-smoke/feedback.ts";
import { practice } from "./desktop-smoke/practice.ts";
import { cli, textEnv } from "./desktop-smoke/scenario.ts";
import type { ScenarioRun, Shared } from "./desktop-smoke/scenario.ts";
import { settings } from "./desktop-smoke/settings.ts";

/**
 * The cases, in the order they run; a later case depends on the app the
 * earlier ones proved. The content scan runs first, before any launch, when
 * the tree still holds exactly what the package shipped.
 */
const CASES = ["contents", "layout", "study", "cli-runtime", "agents", "brief", "brief-responses", "anki", "upgrade", "credential", "broken"] as const;

export interface PackageSmokeOptions {
  /**
   * A tree that stands in for an installed package, relative to the gate root:
   * the self-test drives the gate over one without building a package. The
   * tree carries the app binary the smoke hook launches (passquest-desktop on
   * PATH's spelling), a `node/` runtime file and the four `agent/` hosts; the
   * cases still run, but no installer does.
   */
  installedTree?: string;
  /**
   * The notices a fixture install tree must carry (harness/lib/notices.ts):
   * the contents scan checks the fixture's `agent/THIRD-PARTY-NOTICES.txt`
   * against them exactly as a real run checks its installed file against the
   * set its own staging assembled (goal 0016 P4). A real run never needs it —
   * its build's identity carries the set — so it applies beside
   * `installedTree` only.
   */
  notices?: StagedNotices;
  /**
   * The texts the pin check reads (label → text, `harness/lib/package.ts`
   * NODE_PIN_SOURCES), for a self-test case that hands the gate a disagreeing
   * set; a real run, and every fixture case, reads the repository's own files.
   */
  nodePinSources?: Readonly<Record<string, string | undefined>>;
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

/** The developer tools no launch may find on its PATH. */
const DEVELOPER_TOOLS = ["go", "bun", "node"];

/**
 * This gate's own repository root: the pin check reads the repository's files
 * (`mise.toml`, `gate.yml`, the guides), never `ctx.root`, which a self-test
 * fixture points at a tree that carries none of them.
 */
const REPOSITORY_ROOT = path.resolve(import.meta.dir, "..", "..");

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
  const pinProblems = nodePinProblems(options.nodePinSources ?? readNodePinSources(REPOSITORY_ROOT));
  if (pinProblems.length > 0) return fail(pinProblems);
  const pinLine = `node pin: ${Object.keys(NODE_PIN_SOURCES).length} sources agree on Node ${NODE_VERSION}`;
  let raw: string;
  try {
    raw = readFileSync(path.resolve(ctx.root, "mygo.json"), "utf8");
  } catch (error) {
    return fail([`mygo.json cannot be read: ${error instanceof Error ? error.message : String(error)}`]);
  }
  let config: { name?: unknown; version?: unknown; macos?: { minimumSystemVersion?: unknown } };
  try {
    config = JSON.parse(raw) as { name?: unknown; version?: unknown; macos?: { minimumSystemVersion?: unknown } };
  } catch (error) {
    return fail([`mygo.json is not JSON: ${error instanceof Error ? error.message : String(error)}`]);
  }
  if (typeof config.name !== "string" || config.name.trim() === "") return fail(["mygo.json carries no name; the app's data directory is named after it"]);
  // The floor the installed Info.plist must declare (goal 0017 P1), read from
  // mygo.json: empty when it declares none, which the scan reports rather than
  // checks against. `plistProblems` (package-smoke/contents.ts) holds the rule.
  const declaredMinimum = typeof config.macos?.minimumSystemVersion === "string" ? config.macos.minimumSystemVersion : "";
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
  writeStubRegistry(registry, provider.endpoint, [stubResponsesProvider(provider)]);
  let app: InstalledApp | undefined;
  let identity: PackageIdentity | undefined;
  /** What a fixture install tree stands in for: the notices set its file must carry (PackageSmokeOptions.notices). */
  let fixtureNotices: StagedNotices | undefined;
  /** What a fixture install tree declares: the minimum its own mygo.json states, which its Info.plist must carry. */
  let fixtureMinimumSystemVersion = "";
  let packageLine = "";
  let studyData = "";
  let studyHome = "";
  let cliLine = "";
  let scanLine = "";
  let upgradeLine = "";
  try {
    if (options.installedTree !== undefined) {
      const tree = path.resolve(ctx.root, options.installedTree);
      if (!existsSync(tree)) checks.push(`the fixture install tree ${options.installedTree} does not exist`);
      else {
        app = fixtureApp(tree);
        fixtureNotices = options.notices;
        fixtureMinimumSystemVersion = declaredMinimum;
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
          identity = {
            artifact,
            naming,
            config: { name: config.name, version: typeof config.version === "string" ? config.version : "" },
            // The floor mygo.json declares: the scan proves the installed
            // Info.plist carries it (goal 0017 P1).
            minimumSystemVersion: declaredMinimum,
            // The notices this staging assembled: the contents scan proves the
            // installed file carries every text of them (goal 0016 P4).
            notices: built.build.staged.notices,
          };
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
      cases.includes("study") || cases.includes("agents") || cases.includes("brief") || cases.includes("brief-responses") ||
      cases.includes("anki") || cases.includes("credential") || cases.includes("broken");
    const ready = app !== undefined && (!needsCli || (await ensureCli()));

    /** One case's launch site: its own temp cwd, per-user root and credential service. */
    const siteOf = (
      id: string,
      binary: string,
      extraEnv: Record<string, string> = {},
      site: { keepHome?: boolean; setData?: boolean; home?: string } = {},
    ): { run: ScenarioRun; dataRoot: string; home: string } => {
      const home = site.home ?? path.join(scratch, `home-${id}`);
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
        runtime: app!.runtime,
        // The durable host the installed copy carries, by its agent/ name.
        durableHost: app!.hosts.find((file) => path.basename(file) === DURABLE_HOST),
      };
      return {
        dataRoot,
        home,
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
        scanLine = await scanPackageContents(app!, ctx, check, identity, fixtureNotices, fixtureMinimumSystemVersion);
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
        studyHome = site.home;
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
      "cli-runtime": async () => {
        // The study case's per-user root, so the database the CLI opens is the
        // one the window wrote; PATH carries no node and no resolution
        // override is set, so a host-backed success is the packaged runtime's.
        const site = siteOf("cli-runtime", app!.binary, app!.launchEnv, { home: studyHome === "" ? undefined : studyHome });
        const env: Record<string, string> = {
          ...textEnv(cleanEnv()),
          ...textEnv(site.run.shared.launchEnv ?? {}),
          PASSQUEST_PROVIDER_REGISTRY: registry,
          [STUB_CREDENTIAL_ENV]: `sk-smoke-${Date.now().toString(16)}`,
        };
        const pinned = Object.keys(env).filter((name) => name === "PASSQUEST_NODE" || /^PASSQUEST_.*_HOST$/.test(name));
        check(pinned.length === 0, `cli-runtime: the installation's CLI environment pins ${pinned.join(", ")}`);
        const cwd = site.run.shared.cwd ?? site.run.dir;
        mkdirSync(path.join(cwd, ".passquest"), { recursive: true });
        writeFileSync(path.join(cwd, ".passquest", "passquest.sqlite"), "not a database\n");
        const runCLI = (args: string[]) => run(app!.binary, args, { cwd, env, replaceEnv: true });

        // The CLI's own notice names the database it resolved; the app's own
        // resolution for the same per-user root is the file below it.
        const report = await runCLI(["report", "--json"]);
        const notice = report.stderr.split(/\r?\n/).find((line) => line.includes("the command line now uses")) ?? "";
        check(
          notice.includes(site.run.database),
          `cli-runtime: the installed CLI resolved its database elsewhere (${notice === "" ? "no notice line" : notice}), the app's per-user root holds ${site.run.database}`,
        );
        check(report.code === 0, `cli-runtime: report exit = ${report.code}: ${tail(report.stderr, 4)}`);

        const before = provider.received.length;
        const tested = await runCLI(["model", "test", STUB_MODEL, "--json"]);
        const doc = parse(tested.stdout) as Record<string, unknown>;
        check(tested.code === 0, `cli-runtime: model test exit = ${tested.code}: ${tail(tested.stderr, 6)}`);
        check(stringField(doc, "state") === "verified", `cli-runtime: model test state = ${String(stringField(doc, "state"))} (${String(stringField(doc, "result"))})`);
        check(stringField(doc, "provider") === "stub", `cli-runtime: model test provider = ${String(stringField(doc, "provider"))}`);
        check(
          stringField(doc, "credential_env") === STUB_CREDENTIAL_ENV,
          `cli-runtime: model test credential_env = ${String(stringField(doc, "credential_env"))}, want the registry's own ${STUB_CREDENTIAL_ENV}`,
        );
        check(
          provider.received.length - before === 1,
          `cli-runtime: the loopback stub answered ${provider.received.length - before} request(s) for the installed CLI's probe, want 1`,
        );
        const databases = databasesUnder(site.home);
        check(
          databases.length === 1 && path.resolve(databases[0]!) === path.resolve(site.run.database),
          `cli-runtime: ${databases.length} database(s) under ${site.home}: ${databases.join(", ") || "none"}, want the per-user ${site.run.database}`,
        );
        cliLine = `cli-runtime: ${site.run.database} opened by the installed binary with no node on PATH; ${STUB_MODEL} verified`;
      },
      agents: async () => {
        await coach.run(siteOf("agents", app!.binary, app!.launchEnv).run);
      },
      brief: async () => {
        await brief.run(siteOf("brief", app!.binary, app!.launchEnv).run);
      },
      "brief-responses": async () => {
        await briefResponses.run(siteOf("brief-responses", app!.binary, app!.launchEnv).run);
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
      cliLine === "" ? "" : `; ${cliLine}`
    }${upgradeLine === "" ? "" : `; ${upgradeLine}`}; ${pinLine}`,
  );
}
