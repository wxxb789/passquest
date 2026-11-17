import { mkdirSync, writeFileSync } from "node:fs";

import { executable, run } from "../../lib/process.ts";
import { resolve } from "../../lib/repo.ts";

import { goldenContract } from "../golden-contract.ts";
import type { BrokenCase, PassCase } from "./fixture.ts";

async function gitAt(root: string, args: string[]): Promise<string | null> {
  const result = await run(executable("git"), args, { cwd: root });
  return result.code === 0 ? null : `git ${args.join(" ")} failed: ${result.stderr || result.stdout}`;
}

async function commitAll(root: string, message: string): Promise<string | null> {
  const add = await gitAt(root, ["add", "-A"]);
  if (add) return add;
  return gitAt(root, [
    "-c",
    "user.name=selftest",
    "-c",
    "user.email=selftest@example.invalid",
    "-c",
    "commit.gpgsign=false",
    "commit",
    "-q",
    "-m",
    message,
  ]);
}

/** A committed golden the fixtures then change or replace. */
const baseGoldenJson = '{\n  "schema_version": 1\n}\n';

/**
 * A repository with a `main` base commit and a `feature` branch, so the golden
 * gate's merge-base path is the one exercised. The fixture's first commit holds
 * the pre-change files; `change` then edits the working tree on `feature`.
 */
async function withFeatureBranch(root: string, change: (root: string) => Promise<string | null>): Promise<string | null> {
  const init = await gitAt(root, ["init", "-q", "-b", "main"]);
  if (init) return init;
  const base = await commitAll(root, "base");
  if (base) return base;
  const branch = await gitAt(root, ["checkout", "-q", "-b", "feature"]);
  if (branch) return branch;
  return change(root);
}

async function goldenOnlyChange(root: string): Promise<string | null> {
  writeFileSync(resolve(root, "internal/cli/testdata/next-empty.golden.json"), '{\n  "schema_version": 2\n}\n');
  return commitAll(root, "rewrite a golden without evidence");
}

async function newGoldenWithEvidence(root: string): Promise<string | null> {
  writeFileSync(resolve(root, "internal/cli/testdata/brand-new.golden.json"), "{}\n");
  const commit = await commitAll(root, "add a golden");
  if (commit) return commit;
  // The evidence row stays untracked: the gate must read the working tree, not
  // only committed history.
  mkdirSync(resolve(root, "docs/evidence"), { recursive: true });
  writeFileSync(resolve(root, "docs/evidence/0001-golden-contract.md"), "# Evidence\n\nNew golden declared.\n");
  return null;
}

export function goldenContractCases(): BrokenCase[] {
  return [
    {
      gate: goldenContract,
      label: "golden rewritten on a branch without a contract document",
      files: { "internal/cli/testdata/next-empty.golden.json": baseGoldenJson },
      prepare: (root) => withFeatureBranch(root, goldenOnlyChange),
      postFail: (failures) => {
        if (failures.length !== 1) {
          return `golden-contract: expected one failure, saw ${failures.length}: ${failures.join("; ")}`;
        }
        if (!failures[0].includes("internal/cli/testdata/next-empty.golden.json")) {
          return "golden-contract: did not name the golden that changed";
        }
        if (!failures[0].includes("docs/evidence/") || !failures[0].includes("OPEN-QUESTIONS.md")) {
          return "golden-contract: did not name the contract document that is missing";
        }
        return null;
      },
    },
  ];
}

export function goldenContractPassCases(): PassCase[] {
  return [
    {
      gate: goldenContract,
      label: "new golden declared by an untracked evidence record",
      files: { "internal/cli/testdata/next-empty.golden.json": baseGoldenJson },
      prepare: (root) => withFeatureBranch(root, newGoldenWithEvidence),
      postPass: (detail) =>
        detail.includes("1 golden(s) declared by 1 contract document(s)")
          ? null
          : `golden-contract: accepted detail ${JSON.stringify(detail)} does not name the declaring evidence row`,
    },
  ];
}
