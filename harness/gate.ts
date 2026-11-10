#!/usr/bin/env bun
import path from "node:path";

import { gates } from "./gates/index.ts";
import { context } from "./lib/repo.ts";
import type { GateResult } from "./lib/types.ts";

const root = path.resolve(import.meta.dir, "..");
const ctx = context(root);

let failed = 0;
let skipped = 0;
let passed = 0;

for (const gate of gates) {
  process.stdout.write(`\n== ${gate.title}\n`);
  let result: GateResult;
  try {
    result = await gate.run(ctx);
  } catch (error) {
    result = { status: "fail", failures: [error instanceof Error ? error.message : String(error)] };
  }
  switch (result.status) {
    case "pass":
      passed++;
      process.stdout.write(`PASS  ${gate.title}${result.detail ? ` (${result.detail})` : ""}\n`);
      break;
    case "skip":
      skipped++;
      process.stdout.write(`SKIP  ${gate.title} (${result.skipReason ?? "no reason"})\n`);
      break;
    case "fail": {
      failed++;
      process.stdout.write(`FAIL  ${gate.title}${result.detail ? ` (${result.detail})` : ""}\n`);
      const failures = result.failures ?? [];
      // Gates bound their own output (go-test keeps 12 lines per failing test).
      // This cap only guards against an unbounded list; at 40 the four-failure
      // Windows run would have dropped the last test's reason entirely.
      const limit = 200;
      for (const line of failures.slice(0, limit)) {
        process.stdout.write(`  ${line}\n`);
      }
      if (failures.length > limit) {
        process.stdout.write(`  ... ${failures.length - limit} more line(s)\n`);
      }
      break;
    }
  }
}

process.stdout.write(`\n${passed} passed, ${failed} failed, ${skipped} skipped\n`);
if (failed > 0) {
  process.stdout.write("GATE: FAIL\n");
  process.exit(1);
}
process.stdout.write("GATE: PASS\n");
