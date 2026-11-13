import path from "node:path";

import { executable, run } from "../lib/process.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate, GateContext, GateResult } from "../lib/types.ts";

/**
 * The CLI protocol and history goldens. The owner decided (2026-11-13, option A
 * of Q-0000-05) that these files are contract evidence: a golden change is a
 * contract change, so it must be declared by a document that carries the
 * contract, not rewritten silently to make a test green.
 */
const GOLDEN_DIR = "internal/cli/testdata/";

/**
 * Documents that can declare a golden change: a measured evidence record, a goal
 * contract, or a decision record (opening `OPEN-QUESTIONS.md`, which is a
 * decisions file). A deleted document never declares anything.
 */
const CONTRACT_DIRS = ["docs/evidence/", "docs/goals/", "docs/decisions/"];

interface ChangedPath {
  path: string;
  status: string;
}

export function isGoldenPath(relative: string): boolean {
  const normalised = relative.split(path.sep).join("/");
  return normalised.startsWith(GOLDEN_DIR) && path.posix.basename(normalised).includes("golden");
}

export function isContractDocument(relative: string): boolean {
  const normalised = relative.split(path.sep).join("/");
  return normalised.endsWith(".md") && CONTRACT_DIRS.some((dir) => normalised.startsWith(dir));
}

async function git(root: string, args: string[]): Promise<{ code: number; stdout: string; stderr: string }> {
  return run(executable("git"), args, { cwd: root });
}

/**
 * Resolves the commit the branch grew from. `origin/main` and friends cover a
 * normal clone. A CI checkout is shallow: the change's base is `HEAD^1` there,
 * and a push to the default branch has a merge base that *is* HEAD, which would
 * hide the very change this gate looks for — so CI prefers `HEAD^1`. A clone
 * where nothing resolves is a refusal, never a silent pass: a check that cannot
 * see history cannot see a golden change.
 */
async function resolveBase(root: string): Promise<{ base: string | null; reason: string }> {
  const override = process.env.PASSQUEST_GOLDEN_BASE?.trim();
  if (override) {
    const verify = await git(root, ["rev-parse", "--verify", "--quiet", `${override}^{commit}`]);
    return verify.code === 0
      ? { base: verify.stdout.trim(), reason: `PASSQUEST_GOLDEN_BASE=${override}` }
      : { base: null, reason: `PASSQUEST_GOLDEN_BASE=${override} is not a commit in this clone` };
  }
  const parent = await git(root, ["rev-parse", "--verify", "--quiet", "HEAD^1^{commit}"]);
  const parentCommit = parent.code === 0 ? parent.stdout.trim() : null;
  if (parentCommit && process.env.GITHUB_ACTIONS === "true") {
    return { base: parentCommit, reason: "HEAD^1 of the CI checkout" };
  }
  const candidates = ["origin/main", "main", "origin/master", "master"];
  const baseRef = process.env.GITHUB_BASE_REF?.trim();
  if (baseRef) candidates.unshift(baseRef, `origin/${baseRef}`);
  for (const candidate of candidates) {
    const exists = await git(root, ["rev-parse", "--verify", "--quiet", `${candidate}^{commit}`]);
    if (exists.code !== 0) continue;
    const merge = await git(root, ["merge-base", "HEAD", candidate]);
    if (merge.code === 0 && merge.stdout.trim()) {
      return { base: merge.stdout.trim(), reason: `merge base with ${candidate}` };
    }
  }
  if (parentCommit) {
    return { base: parentCommit, reason: "HEAD^1, the base of a shallow checkout" };
  }
  return { base: null, reason: "no merge base, origin/main or HEAD^1 is reachable" };
}

/** Committed, staged and unstaged changes against the base, plus new files. */
async function changedPaths(root: string, base: string): Promise<{ paths: ChangedPath[]; problem?: string }> {
  const diff = await git(root, ["diff", "--name-status", "--no-renames", base, "--"]);
  if (diff.code !== 0) {
    return { paths: [], problem: `git diff against ${base} failed: ${diff.stderr || diff.stdout}` };
  }
  const paths: ChangedPath[] = [];
  for (const line of diff.stdout.split("\n")) {
    if (!line.trim()) continue;
    const fields = line.split("\t");
    if (fields.length < 2) continue;
    paths.push({ status: fields[0].charAt(0).toUpperCase(), path: fields[fields.length - 1] });
  }
  const untracked = await git(root, ["ls-files", "--others", "--exclude-standard", "-z"]);
  if (untracked.code !== 0) {
    return { paths: [], problem: `git ls-files failed: ${untracked.stderr || untracked.stdout}` };
  }
  for (const file of untracked.stdout.split("\0").filter(Boolean)) {
    if (!paths.some((entry) => entry.path === file)) paths.push({ status: "A", path: file });
  }
  return { paths };
}

function check(changed: ChangedPath[], base: string): GateResult {
  const goldens = changed.filter((entry) => isGoldenPath(entry.path));
  if (goldens.length === 0) return pass(`no golden changed against ${base.slice(0, 12)}`);
  const declared = changed.filter((entry) => entry.status !== "D" && isContractDocument(entry.path));
  if (declared.length > 0) {
    return pass(
      `${goldens.length} golden(s) declared by ${declared.length} contract document(s) against ${base.slice(0, 12)}`,
    );
  }
  const failures = goldens.map(
    (entry) =>
      `${entry.path}: golden ${entry.status === "D" ? "deleted" : "changed"} without a contract document; ` +
      "add an evidence record under docs/evidence/, a goal contract under docs/goals/, or an entry in " +
      "docs/decisions/OPEN-QUESTIONS.md that states the protocol or history change",
  );
  return fail(failures, `${goldens.length} golden(s) changed against ${base.slice(0, 12)}, no contract document`);
}

export const goldenContract: Gate = {
  id: "golden-contract",
  title: "golden contract",
  async run(ctx: GateContext): Promise<GateResult> {
    const base = await resolveBase(ctx.root);
    if (!base.base) {
      return fail([
        `cannot resolve the branch's base commit: ${base.reason}.`,
        "The golden contract check compares goldens with the base the branch grew from.",
        "Fetch it (`git fetch origin main`) or set PASSQUEST_GOLDEN_BASE=<ref>; a shallow clone needs at least the base commit.",
      ]);
    }
    const changed = await changedPaths(ctx.root, base.base);
    if (changed.problem) return fail([changed.problem], "git failed");
    return check(changed.paths, base.base);
  },
};
