#!/usr/bin/env bun
/**
 * `bun run package [--target <platform>-<arch>] [--appimage-only]`: bundles the frontend, stages the
 * package resources into build/package and builds the app with MyGo (`go tool mygo build -platform
 * <goos>/<goarch>`), which copies the staged tree into the app and produces this target's package.
 * On Linux the AppImage is built afterwards from MyGo's Linux output (harness/lib/appimage.ts).
 * The build sequence itself lives in harness/lib/package-build.ts, so the package smoke gate
 * installs the package this script builds.
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
import path from "node:path";

import { buildAppImage } from "./lib/appimage.ts";
import { buildPackage, size } from "./lib/package-build.ts";
import { hostTarget, parseTarget, targetDir } from "./lib/package.ts";
import type { PackageTarget } from "./lib/package.ts";
import { context } from "./lib/repo.ts";

const root = path.resolve(import.meta.dir, "..");
const ctx = context(root);

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

if (appimageOnly) {
  // The step itself refuses where it cannot run (a host that is not Linux, a
  // target that is not linux-x64), so this is never a silent success.
  const image = await buildAppImage(root, { target, log });
  process.stdout.write(`appimage in ${targetDir(target)}\n`);
  process.stdout.write(`  ${path.basename(image.file).padEnd(40)} ${size(image.bytes).padStart(10)}\n`);
} else {
  const built = await buildPackage(ctx, { target, log, err: (line) => process.stderr.write(`${line}\n`) });
  if (!built.ok) {
    for (const line of built.failures) process.stderr.write(`${line}\n`);
    process.exit(built.code);
  }
}
