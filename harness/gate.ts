#!/usr/bin/env bun
import path from "node:path";

import { gateGroups, gates } from "./gates/index.ts";
import { groupProblems, workflowGroups } from "./lib/gate-groups.ts";
import { context, resolve } from "./lib/repo.ts";
import type { Gate, GateResult } from "./lib/types.ts";

const root = path.resolve(import.meta.dir, "..");
const ctx = context(root);

function refuse(lines: string[]): never {
  for (const line of lines) process.stdout.write(`${line}\n`);
  process.stdout.write("GATE: FAIL\n");
  process.exit(1);
}

// No argument runs every gate in order. `--group <name>` runs one CI job's share. Either way the run
// first proves that the groups and the workflow matrix still cover every gate exactly once on every
// runner, so a local full gate catches a gate that CI would drop.
let selected: readonly Gate[] = gates;
const args = process.argv.slice(2);
let members: readonly Gate[] | undefined;
const name = args[1];
if (args.length > 0) {
  if (args[0] !== "--group" || args.length !== 2 || !name) {
    refuse([`usage: bun run gate [--group <${Object.keys(gateGroups).join("|")}>]; got: ${args.join(" ")}`]);
  }
  members = Object.hasOwn(gateGroups, name) ? gateGroups[name] : undefined;
  if (!members) {
    refuse([`unknown gate group "${name}"; declared groups: ${Object.keys(gateGroups).join(", ")}`]);
  }
}
const workflow = workflowGroups(resolve(root, ".github/workflows/gate.yml"));
const problems = typeof workflow === "string" ? [workflow] : groupProblems(gates, gateGroups, workflow);
if (problems.length > 0) {
  refuse(["gate groups do not cover every gate exactly once in CI:", ...problems.map((problem) => `  ${problem}`)]);
}
if (members) {
  selected = gates.filter((gate) => members.includes(gate));
  process.stdout.write(`group ${name}: ${selected.length} of ${gates.length} gates\n`);
}

let failed = 0;
let skipped = 0;
let passed = 0;
const timings: { title: string; ms: number }[] = [];
const seconds = (ms: number) => `${(ms / 1000).toFixed(1)}s`;

for (const gate of selected) {
  process.stdout.write(`\n== ${gate.title}\n`);
  const started = performance.now();
  let result: GateResult;
  try {
    result = await gate.run(ctx);
  } catch (error) {
    result = { status: "fail", failures: [error instanceof Error ? error.message : String(error)] };
  }
  const ms = performance.now() - started;
  timings.push({ title: gate.title, ms });
  // The wall time trails the line, after the gate's own detail, so that text is unchanged.
  const took = ` [${seconds(ms)}]`;
  switch (result.status) {
    case "pass":
      passed++;
      process.stdout.write(`PASS  ${gate.title}${result.detail ? ` (${result.detail})` : ""}${took}\n`);
      break;
    case "skip":
      skipped++;
      process.stdout.write(`SKIP  ${gate.title} (${result.skipReason ?? "no reason"})${took}\n`);
      break;
    case "fail": {
      failed++;
      process.stdout.write(`FAIL  ${gate.title}${result.detail ? ` (${result.detail})` : ""}${took}\n`);
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

process.stdout.write("\nTiming, slowest first:\n");
for (const { title, ms } of [...timings].sort((a, b) => b.ms - a.ms)) {
  process.stdout.write(`${seconds(ms).padStart(9)}  ${title}\n`);
}

process.stdout.write(`\n${passed} passed, ${failed} failed, ${skipped} skipped\n`);
if (failed > 0) {
  process.stdout.write("GATE: FAIL\n");
  process.exit(1);
}
process.stdout.write("GATE: PASS\n");
