#!/usr/bin/env bun
/**
 * `bun run package [--target <platform>-<arch>] [--appimage-only]`: bundles the frontend, stages the
 * package resources into build/package and builds the app with MyGo (`go tool mygo build -platform
 * <goos>/<goarch>`), which copies the staged tree into the app and produces this target's package.
 * On Linux the AppImage is built afterwards from MyGo's Linux output (harness/lib/appimage.ts).
 *
 * `--target` stages and builds for another target than this machine: Windows or macOS can build
 * linux-x64, which is what the AppImage needs, but the AppImage step itself runs on Linux only —
 * `bun run package --appimage-only` there, over the output of the cross build.
 * `--appimage-only` never reports a success it did not perform: where the step cannot run (a host
 * that is not Linux, a target that is not linux-x64), it fails with the same guidance a normal
 * cross build prints.
 *
 * The development app, the gates and `bun run desktop:build` are unaffected: this is the packaging
 * path of goal 0013.
 */
import { readdirSync, rmSync, statSync } from "node:fs";
import path from "node:path";

import { buildAppImage } from "./lib/appimage.ts";
import { bundleFrontend, FRONTEND_DIST } from "./lib/desktop.ts";
import { runGo } from "./lib/go.ts";
import { goTarget, hostTarget, parseTarget, STAGE_DIR, stageResources, targetDir } from "./lib/package.ts";
import type { PackageTarget } from "./lib/package.ts";
import { outputLines } from "./lib/process.ts";
import { context, resolve } from "./lib/repo.ts";

const root = path.resolve(import.meta.dir, "..");
const ctx = context(root);

/** Bytes as a short human figure, for the listings below. */
function size(bytes: number): string {
  const units = ["B", "kB", "MB", "GB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1000 && unit < units.length - 1) {
    value /= 1000;
    unit += 1;
  }
  return `${value >= 100 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
}

interface Options {
  /** The target to stage and build for. */
  target: PackageTarget;
  /** Run only the AppImage step, over a target MyGo built earlier. */
  appimageOnly: boolean;
}

function options(argv: string[]): Options {
  let target: PackageTarget | undefined;
  let appimageOnly = false;
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === "--target") {
      const value = argv[i + 1];
      if (value === undefined || value.startsWith("--")) throw new Error("--target needs <platform>-<arch>, e.g. linux-x64");
      target = parseTarget(value);
      i += 1;
    } else if (arg === "--appimage-only") {
      appimageOnly = true;
    } else {
      throw new Error(`unknown argument ${arg}; usage: bun run package [--target <platform>-<arch>] [--appimage-only]`);
    }
  }
  if (appimageOnly && target === undefined) target = { platform: "linux", arch: "x64" };
  return { target: target ?? hostTarget(), appimageOnly };
}

const { target, appimageOnly } = options(process.argv.slice(2));
const log = (line: string) => process.stdout.write(`${line}\n`);

/** Runs the AppImage step over the target MyGo built and prints what it wrote. */
async function appimage(): Promise<void> {
  const image = await buildAppImage(root, { target, log });
  process.stdout.write(`appimage in ${targetDir(target)}\n`);
  process.stdout.write(`  ${path.basename(image.file).padEnd(40)} ${size(image.bytes).padStart(10)}\n`);
}

if (appimageOnly) {
  // The step itself refuses where it cannot run (a host that is not Linux, a
  // target that is not linux-x64), so this is never a silent success.
  await appimage();
} else {
  const dist = resolve(root, FRONTEND_DIST);
  rmSync(dist, { recursive: true, force: true });
  const bundle = await bundleFrontend(root, dist);
  if (bundle.code !== 0) {
    for (const line of outputLines(bundle)) process.stderr.write(`${line}\n`);
    process.exit(1);
  }

  const staged = await stageResources(root, { target, log });
  for (const entry of staged.entries) {
    process.stdout.write(`  ${entry.path.padEnd(26)} ${String(entry.files).padStart(5)} files ${size(entry.bytes).padStart(10)}\n`);
  }
  process.stdout.write(`staged ${staged.files} files, ${size(staged.bytes)} in ${STAGE_DIR}\n`);

  const build = await runGo(ctx, ["tool", "mygo", "build", "-platform", goTarget(target)]);
  process.stdout.write(build.stdout);
  process.stderr.write(build.stderr);
  if (build.code !== 0) process.exit(build.code);

  const out = resolve(root, targetDir(target));
  const artifacts = readdirSync(out, { withFileTypes: true })
    .filter((entry) => entry.isFile())
    .map((entry) => ({ name: entry.name, bytes: statSync(path.join(out, entry.name)).size }))
    .sort((a, b) => a.name.localeCompare(b.name));
  process.stdout.write(`packages in ${targetDir(target)}\n`);
  for (const artifact of artifacts) {
    process.stdout.write(`  ${artifact.name.padEnd(40)} ${size(artifact.bytes).padStart(10)}\n`);
  }

  if (target.platform === "linux") {
    if (process.platform === "linux") {
      await appimage();
    } else {
      process.stdout.write(`appimage: needs Linux; run \`bun run package --appimage-only\` on Linux over this output\n`);
    }
  }
}
