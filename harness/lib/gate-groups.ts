import { readFileSync } from "node:fs";

import type { Gate } from "./types.ts";

/** The runners every gate group must run on; the matrix is the cross-platform evidence. */
export const ciRunners = ["ubuntu-latest", "macos-latest", "windows-latest"] as const;

type GateJob = {
  env?: Record<string, unknown>;
  strategy?: { matrix?: Record<string, unknown> };
  steps?: { run?: unknown }[];
};

/**
 * The matrix `group` list of the `gate` job in the CI workflow, or a reason it cannot stand for every
 * group on every runner: the matrix must be exactly `os` × `group` (an `include` or `exclude` would
 * add or drop a cell unseen), list each runner in `ciRunners` once, and every step that runs the gate
 * must pass the cell's group through `GATE_GROUP`.
 */
export function workflowGroups(workflowPath: string): string[] | string {
  let document: unknown;
  try {
    document = Bun.YAML.parse(readFileSync(workflowPath, "utf8"));
  } catch (error) {
    return `cannot read ${workflowPath}: ${error instanceof Error ? error.message : String(error)}`;
  }
  const job = (document as { jobs?: { gate?: GateJob } } | null)?.jobs?.gate;
  const matrix = job?.strategy?.matrix;
  const group = matrix?.group;
  if (!Array.isArray(group) || !group.every((name) => typeof name === "string")) {
    return `${workflowPath} has no jobs.gate.strategy.matrix.group list of names`;
  }
  const keys = Object.keys(matrix!).sort();
  if (keys.join(",") !== "group,os") {
    return `${workflowPath} jobs.gate.strategy.matrix must hold exactly os and group; it holds ${keys.join(", ")}`;
  }
  const os = matrix!.os;
  if (!Array.isArray(os) || os.length !== ciRunners.length || !ciRunners.every((runner) => os.includes(runner))) {
    return `${workflowPath} jobs.gate.strategy.matrix.os must list ${ciRunners.join(", ")} once each; it lists ${JSON.stringify(os)}`;
  }
  if (job?.env?.GATE_GROUP !== "${{ matrix.group }}") {
    return `${workflowPath} jobs.gate.env.GATE_GROUP must be \${{ matrix.group }}`;
  }
  const gateRuns = (job.steps ?? []).map((step) => step.run).filter((run) => typeof run === "string" && run.includes("bun run gate"));
  if (gateRuns.length === 0 || gateRuns.some((run) => !(run as string).includes('bun run gate --group "$GATE_GROUP"'))) {
    return `${workflowPath} jobs.gate must run the gate only as bun run gate --group "$GATE_GROUP"`;
  }
  return group;
}

/**
 * Why the declared groups cannot stand in for the full gate: a gate in no group or in two, a group
 * holding a gate the runner does not declare, or a workflow matrix that does not list exactly the
 * declared groups once each. An empty list means every gate runs in exactly one CI job.
 */
export function groupProblems(gates: readonly Gate[], groups: Record<string, readonly Gate[]>, workflow: string[]): string[] {
  const problems: string[] = [];
  for (const gate of gates) {
    const owners = Object.keys(groups).filter((name) => groups[name]!.includes(gate));
    if (owners.length !== 1) {
      problems.push(`gate ${gate.id} is in ${owners.length} groups (${owners.join(", ") || "none"}); it must be in exactly one`);
    }
  }
  for (const [name, members] of Object.entries(groups)) {
    for (const gate of members) {
      if (!gates.includes(gate)) problems.push(`group ${name} holds ${gate.id}, which is not in the gate list`);
    }
  }
  const declared = Object.keys(groups);
  for (const name of declared) {
    const count = workflow.filter((listed) => listed === name).length;
    if (count !== 1) problems.push(`the workflow matrix lists group ${name} ${count} times; it must list it once`);
  }
  for (const name of new Set(workflow)) {
    if (!declared.includes(name)) problems.push(`the workflow matrix lists group ${name}, which the runner does not declare`);
  }
  return problems;
}
