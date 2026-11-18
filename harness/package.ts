#!/usr/bin/env bun
/**
 * `bun run package`: bundles the frontend, stages the package resources into
 * build/package and builds the app with MyGo (`go tool mygo build`), which
 * copies the staged tree into the app and produces this platform's package.
 * The development app, the gates and `bun run desktop:build` are unaffected:
 * this is the packaging path of goal 0013.
 */
import { readdirSync, rmSync, statSync } from "node:fs";
import path from "node:path";

import { bundleFrontend, FRONTEND_DIST } from "./lib/desktop.ts";
import { runGo } from "./lib/go.ts";
import { STAGE_DIR, stageResources } from "./lib/package.ts";
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

/** Go's name for this machine's architecture, as MyGo names an output directory. */
function goarch(arch: string): string {
  switch (arch) {
    case "x64":
      return "amd64";
    case "ia32":
      return "386";
    default:
      return arch;
  }
}

const dist = resolve(root, FRONTEND_DIST);
rmSync(dist, { recursive: true, force: true });
const bundle = await bundleFrontend(root, dist);
if (bundle.code !== 0) {
  for (const line of outputLines(bundle)) process.stderr.write(`${line}\n`);
  process.exit(1);
}

const staged = await stageResources(root, { log: (line) => process.stdout.write(`${line}\n`) });
for (const entry of staged.entries) {
  process.stdout.write(`  ${entry.path.padEnd(26)} ${String(entry.files).padStart(5)} files ${size(entry.bytes).padStart(10)}\n`);
}
process.stdout.write(`staged ${staged.files} files, ${size(staged.bytes)} in ${STAGE_DIR}\n`);

const build = await runGo(ctx, ["tool", "mygo", "build"]);
process.stdout.write(build.stdout);
process.stderr.write(build.stderr);
if (build.code !== 0) process.exit(build.code);

const target = `${process.platform === "win32" ? "windows" : process.platform}-${goarch(process.arch)}`;
const out = resolve(root, `build/${target}`);
const artifacts = readdirSync(out, { withFileTypes: true })
  .filter((entry) => entry.isFile())
  .map((entry) => ({ name: entry.name, bytes: statSync(path.join(out, entry.name)).size }))
  .sort((a, b) => a.name.localeCompare(b.name));
process.stdout.write(`packages in build/${target}\n`);
for (const artifact of artifacts) {
  process.stdout.write(`  ${artifact.name.padEnd(40)} ${size(artifact.bytes).padStart(10)}\n`);
}
