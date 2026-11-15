#!/usr/bin/env bun
/** `bun run desktop:build`: bundles the frontend and links build/desktop/passquest-desktop. */
import path from "node:path";

import { buildDesktop } from "./lib/desktop.ts";
import { executable } from "./lib/process.ts";
import { context } from "./lib/repo.ts";

const root = path.resolve(import.meta.dir, "..");
const binary = path.join(root, "build", "desktop", executable("passquest-desktop"));
const result = await buildDesktop(context(root), binary);
if (!result.ok) {
  for (const line of result.failures) process.stderr.write(`${line}\n`);
  process.exit(1);
}
process.stdout.write(`built ${binary}\n`);
