import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { buildCLI } from "../../lib/go.ts";
import { executable, run } from "../../lib/process.ts";
import type { GateContext } from "../../lib/types.ts";
import { agentSmoke, durableMatrix } from "../agent-smoke.ts";
import { GO_MOD } from "./fixture.ts";
import type { BrokenCase } from "./fixture.ts";

export function agentSmokeCases(): BrokenCase[] {
  return [
    {
      gate: agentSmoke,
      label: "CLI whose agent output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
        // The gate bundles the durable host before the matrix's second pass
        // (goal 0016 P2): the fixture carries a host that names a resolvable
        // package, so it fails on its broken CLI, not on a bundle build that
        // cannot resolve anything — and a `bun.lock` pinning that package, as
        // the checkout root has, because the bundle's inputs are checked
        // against the lock (goal 0016 P4).
        "bun.lock": '{\n  "lockfileVersion": 2,\n  "packages": {\n    "tiny-fixture": ["tiny-fixture@0.0.0", "", {}, "sha512-fixture"],\n  },\n}\n',
        "agent/durable-host.mjs": 'import "tiny-fixture";\n',
        "agent/node_modules/tiny-fixture/package.json": '{"name": "tiny-fixture", "version": "0.0.0", "main": "index.mjs"}\n',
        "agent/node_modules/tiny-fixture/index.mjs": "export const marker = 1;\n",
      },
    },
  ];
}

/**
 * The plain bundle must fault in the durable matrix, and the matrix must
 * report it: `bun build agent/durable-host.mjs` without the generated entry
 * (bundleEntrySource) drops the packages' module initialization — their
 * `sideEffects` arrays — and the matrix's first scenario then fails with
 * `task_faulted: Cannot read properties of undefined (reading 'get')`
 * (goal 0016 P2). This is the must-fail case for the reason the generated
 * entry exists: it fails when the matrix stops catching a host that lost its
 * initialization — and, if a future bun stops eliding them, when the plain
 * form stops faulting at all, which is the signal to re-evaluate that reason.
 */
export async function plainBundleFaultProblem(ctx: GateContext): Promise<string | null> {
  const node = executable("node");
  const nodeCheck = await run(node, ["--version"], { cwd: ctx.root });
  if (nodeCheck.code !== 0) return `the plain bundle case: node is required: ${nodeCheck.stderr || nodeCheck.stdout}`;
  const build = await buildCLI(ctx);
  if ("failure" in build) return `the plain bundle case: go build ./cmd/passquest failed: ${build.failure}`;
  const temp = mkdtempSync(path.join(tmpdir(), "passquest-plain-bundle-"));
  try {
    const bundle = path.join(temp, "durable-host.mjs");
    const built = await run(
      process.execPath,
      ["build", "agent/durable-host.mjs", "--target=node", "--format=esm", `--outfile=${bundle}`],
      { cwd: ctx.root },
    );
    if (built.code !== 0) return `the plain bundle case: bun build exited ${built.code}: ${built.stdout}${built.stderr}`;
    const result = await durableMatrix(ctx, build.binary, { label: "plain bundle", host: bundle, node });
    const fault = result.failures.find((line) => line.startsWith("plain bundle: ") && line.includes("task_faulted"));
    if (!fault) {
      const first = result.failures.slice(0, 3).map((line) => line.split("\n")[0]).join("; ");
      return (
        `the plain bundle case: the durable matrix did not catch the bundle's lost module initialization ` +
        `(${result.failures.length} failure(s): ${first || "none"})`
      );
    }
    return null;
  } finally {
    rmSync(temp, { recursive: true, force: true });
  }
}
