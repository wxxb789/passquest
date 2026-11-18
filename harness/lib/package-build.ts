/**
 * One package build (goal 0013, step P2/P3): bundles the frontend, stages the
 * package resources, builds the app with MyGo (`go tool mygo build -platform
 * <goos>/<goarch>`) and, on Linux, wraps MyGo's output in the pinned AppImage.
 *
 * `bun run package` and the package smoke gate both call this, so the package
 * the gate installs is the one sequence the developer script runs — the gate
 * manages a build failure, it does not re-implement the build.
 *
 * The artifact a caller installs is selected by the exact name MyGo writes
 * for this run (mygo.json's slug and version, the target's goarch): a stale
 * package an earlier build left in the output directory is never it.
 */
import { existsSync, readFileSync, readdirSync, rmSync, statSync } from "node:fs";
import path from "node:path";

import { buildAppImage } from "./appimage.ts";
import { bundleFrontend, FRONTEND_DIST } from "./desktop.ts";
import { runGo } from "./go.ts";
import { goTarget, hostTarget, STAGE_DIR, stageResources, targetDir } from "./package.ts";
import type { PackageTarget, StagedResources } from "./package.ts";
import { outputLines } from "./process.ts";
import { resolve } from "./repo.ts";
import type { GateContext } from "./types.ts";

/** One file a build wrote, as the listings and the installs name it. */
export interface PackageArtifact {
  name: string;
  /** The absolute path. */
  path: string;
  bytes: number;
}

export interface PackageBuild {
  /** The absolute output directory MyGo wrote the target's files into. */
  dir: string;
  /** The app's identifier (mygo.json): the uninstall registry key carries it. */
  identifier: string;
  /** The app's Windows shortcut name without `.lnk` (mygo.json): the installer writes it into both real shell folders. */
  shortcut: string;
  /**
   * Windows: the NSIS installer of this run, selected by its exact name
   * (`<slug>-setup-<version>-<arch>.exe`). Absent when the target's platform
   * is not this host's: MyGo skips the installer without NSIS and the disk
   * image off macOS, and exits 0, so another platform's build may carry
   * neither.
   */
  setup?: PackageArtifact;
  /** macOS: the disk image of this run, selected by its exact name (`<slug>-<version>-<arch>.dmg`); see `setup`. */
  dmg?: PackageArtifact;
  /** Linux: the AppImage built from MyGo's output. */
  appimage?: PackageArtifact;
  staged: StagedResources;
}

export type PackageBuildResult =
  | { ok: true; build: PackageBuild }
  | {
      ok: false;
      /** The exit code `bun run package` exits with: the failing command's own code, 1 otherwise. */
      code: number;
      /** What failed. MyGo's own output is not repeated here: the caller's stderr sink printed it. */
      failures: string[];
    };

export interface PackageBuildOptions {
  /** The target to stage and build for; this machine's by default. */
  target?: PackageTarget;
  /** Progress lines, one per build step (the developer script prints them; the gate keeps them for a failure). */
  log?: (line: string) => void;
  /** MyGo's and the tools' stderr, when a caller prints progress. */
  err?: (line: string) => void;
}

/** Bytes as a short human figure, for the listings. */
export function size(bytes: number): string {
  const units = ["B", "kB", "MB", "GB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1000 && unit < units.length - 1) {
    value /= 1000;
    unit += 1;
  }
  return `${value >= 100 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
}

/** What mygo.json gives the package its names: the app's slug, version, identifier and shortcut name. */
export interface PackageNaming {
  /**
   * The app's identifier, or MyGo's default: `com.mygo.` plus the name's
   * letters and digits, lower-cased (`"Pass Quest"` → `com.mygo.passquest`,
   * `com.mygo.app` when nothing is left), as cmd/mygo/config.go applyDefaults
   * derives it. It is the uninstall registry key's last element, so it must
   * be MyGo's own and not the slug.
   */
  identifier: string;
  /** MyGo's slug for the app, as cmd/mygo/slug.go makes it. */
  slug: string;
  /**
   * The app's Windows shortcut name without `.lnk`, as cmd/mygo/config.go
   * shortcutName builds it: windows.displayName made a file name, else the
   * name made a file name, else `app`.
   */
  shortcut: string;
  /** The version as cmd/mygo/config.go fsName spells it in a file name. */
  version: string;
}

/**
 * mygo.json's naming inputs. MyGo's slug for an ASCII name is lower-case
 * words of letters and digits joined by single hyphens; a non-ASCII name
 * (which mygo transliterates, cmd/mygo/slug.go) must carry the slug mygo
 * would use as `slug`, because this does not reproduce the transliteration —
 * and the wrong slug would look for the wrong artifact file.
 */
export function packageNaming(raw: string): PackageNaming {
  const config = JSON.parse(raw) as {
    name?: unknown;
    slug?: unknown;
    version?: unknown;
    identifier?: unknown;
    windows?: { displayName?: unknown };
  };
  const name = typeof config.name === "string" ? config.name.trim() : "";
  if (name === "") throw new Error("mygo.json carries no name");
  // cmd/mygo/config.go fsName: the version as a file name on every platform.
  const version = typeof config.version === "string" ? config.version.trim().replace(/[\u0000-\u001f/\\:*?"<>|]/g, "-") : "";
  if (version === "") throw new Error("mygo.json carries no version");
  // cmd/mygo/config.go fsName: a file name on every platform (a space
  // survives; a control character or `/\:*?"<>|` becomes `-`).
  const fsName = (value: string): string =>
    [...value.trim()].map((character) => (character < " " || "/\\:*?\"<>|".includes(character) ? "-" : character)).join("");
  // cmd/mygo/slug.go makes an ASCII name lower-case words of letters and
  // digits joined by single hyphens; a non-ASCII name goes through a
  // transliteration this does not reproduce, so it must carry the slug.
  let slug: string;
  if (typeof config.slug === "string" && config.slug !== "") slug = config.slug;
  else if ([...name].every((character) => /[\x20-\x7e]/.test(character))) slug = name.toLowerCase().match(/[a-z0-9]+/g)?.join("-") ?? "app";
  else {
    throw new Error(
      `the name ${JSON.stringify(name)} holds non-ASCII characters, which mygo transliterates (cmd/mygo/slug.go): write the slug mygo would use as "slug" in mygo.json`,
    );
  }
  // cmd/mygo/config.go applyDefaults: non-alphanumerics dropped, lower-cased.
  const derivedIdentifier = name.replace(/[^a-zA-Z0-9]+/g, "").toLowerCase() || "app";
  const identifier = typeof config.identifier === "string" && config.identifier !== "" ? config.identifier : `com.mygo.${derivedIdentifier}`;
  // cmd/mygo/config.go windowsName and shortcutName.
  const displayName = typeof config.windows?.displayName === "string" ? config.windows.displayName.trim() : "";
  const shortcut = fsName(displayName !== "" ? displayName : name) || fsName(name) || "app";
  return { identifier, slug, shortcut, version };
}

/**
 * The package of one target. The bundle goes into the embed root the binary is
 * linked with, the staged tree (agent hosts, node_modules, Node runtime, icon)
 * is proved before MyGo copies it, and the AppImage — MyGo builds none — is
 * wrapped from MyGo's Linux output where the tool can run: on Linux, for
 * linux-x64. A target that refuses (an AppImage on another host) fails here
 * with the guidance `bun run package` printed before.
 */
export async function buildPackage(ctx: GateContext, options: PackageBuildOptions = {}): Promise<PackageBuildResult> {
  const log = options.log ?? (() => {});
  const err = options.err ?? (() => {});
  const target = options.target ?? hostTarget();
  let naming: PackageNaming;
  try {
    naming = packageNaming(readFileSync(resolve(ctx.root, "mygo.json"), "utf8"));
  } catch (error) {
    return { ok: false, code: 1, failures: [`mygo.json cannot name the package: ${error instanceof Error ? error.message : String(error)}`] };
  }
  const dist = resolve(ctx.root, FRONTEND_DIST);
  rmSync(dist, { recursive: true, force: true });
  const bundle = await bundleFrontend(ctx.root, dist);
  if (bundle.code !== 0) {
    return { ok: false, code: 1, failures: [`bun build ${FRONTEND_DIST} failed (exit ${bundle.code})`, ...outputLines(bundle)] };
  }

  let staged: StagedResources;
  try {
    staged = await stageResources(ctx.root, { target, log: (line) => log(`stage: ${line}`) });
  } catch (error) {
    return { ok: false, code: 1, failures: [`staging the package resources failed: ${error instanceof Error ? error.message : String(error)}`] };
  }
  for (const entry of staged.entries) {
    log(`  ${entry.path.padEnd(26)} ${String(entry.files).padStart(5)} files ${size(entry.bytes).padStart(10)}`);
  }
  log(`staged ${staged.files} files, ${size(staged.bytes)} in ${STAGE_DIR}`);

  const build = await runGo(ctx, ["tool", "mygo", "build", "-platform", goTarget(target)]);
  const lines = (text: string) => text.replace(/\r\n/g, "\n").split("\n").filter((line) => line.trim() !== "");
  for (const line of lines(build.stdout)) log(line);
  // MyGo prints its progress to stderr; each line goes to the caller's stderr
  // sink once, so a caller that keeps both sinks sees no line twice.
  for (const line of lines(build.stderr)) err(line);
  if (build.code !== 0) {
    // MyGo's output already went through the sinks above; the caller exits
    // with MyGo's own code, the way `bun run package` always did.
    return { ok: false, code: build.code, failures: [`go tool mygo build -platform ${goTarget(target)} failed (exit ${build.code})`] };
  }

  const dir = resolve(ctx.root, targetDir(target));
  if (!existsSync(dir)) return { ok: false, code: 1, failures: [`mygo build wrote no ${targetDir(target)}`] };
  const artifact = (file: string): PackageArtifact => ({ name: path.basename(file), path: file, bytes: statSync(file).size });
  const files = readdirSync(dir, { withFileTypes: true })
    .filter((entry) => entry.isFile())
    .map((entry) => artifact(path.join(dir, entry.name)))
    .sort((a, b) => a.name.localeCompare(b.name));
  log(`packages in ${targetDir(target)}`);
  for (const file of files) log(`  ${file.name.padEnd(40)} ${size(file.bytes).padStart(10)}`);

  // The exact name MyGo writes for this target (cmd/mygo/nsis.go
  // installerFileName, cmd/mygo/dmg.go dmgFileName): selecting by it means a
  // stale package an earlier version left in the output directory can never
  // be the one installed.
  const goarch = goTarget(target).split("/")[1]!;
  const expectedName =
    target.platform === "win32"
      ? `${naming.slug}-setup-${naming.version}-${goarch}.exe`
      : target.platform === "darwin"
        ? `${naming.slug}-${naming.version}-${goarch}.dmg`
        : undefined;
  const expected = expectedName === undefined ? undefined : files.find((file) => file.name === expectedName);
  if (expectedName !== undefined && expected === undefined) {
    if (target.platform === process.platform) {
      return {
        ok: false,
        code: 1,
        failures: [`mygo build wrote no ${expectedName} in ${targetDir(target)}; it holds ${files.map((file) => file.name).join(", ") || "nothing"}`],
      };
    }
    // Another platform's target may legitimately carry none: MyGo skips the
    // Windows installer without NSIS and the disk image off macOS, printing
    // its warning and exiting 0 (cmd/mygo/nsis.go writeInstaller,
    // cmd/mygo/build.go). This machine's own artifact stays a failure, since
    // `bun run package` exists to produce it and the package smoke installs
    // it; for the other targets the skip is only logged.
    log(`mygo skipped ${expectedName} for ${target.platform}; ${targetDir(target)} holds ${files.map((file) => file.name).join(", ") || "nothing"}`);
  }

  let appimage: PackageArtifact | undefined;
  if (target.platform === "linux") {
    if (process.platform !== "linux") {
      log("appimage: needs Linux; run `bun run package --appimage-only` on Linux over this output");
    } else {
      try {
        const image = await buildAppImage(ctx.root, { target, log: (line) => log(line) });
        appimage = artifact(image.file);
        log(`appimage in ${targetDir(target)}`);
        log(`  ${path.basename(image.file).padEnd(40)} ${size(image.bytes).padStart(10)}`);
      } catch (error) {
        return { ok: false, code: 1, failures: [`the AppImage step failed: ${error instanceof Error ? error.message : String(error)}`] };
      }
    }
  }

  return {
    ok: true,
    build: {
      dir,
      identifier: naming.identifier,
      shortcut: naming.shortcut,
      setup: target.platform === "win32" ? expected : undefined,
      dmg: target.platform === "darwin" ? expected : undefined,
      appimage,
      staged,
    },
  };
}
