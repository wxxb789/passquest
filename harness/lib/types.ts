export type GateStatus = "pass" | "fail" | "skip";

export interface GateContext {
  root: string;
  trackedFiles: () => Promise<string[]>;
}

export interface GateResult {
  status: GateStatus;
  detail?: string;
  failures?: string[];
  skipReason?: string;
}

export interface Gate {
  id: string;
  title: string;
  run: (ctx: GateContext) => Promise<GateResult>;
}

export function pass(detail?: string): GateResult {
  return { status: "pass", detail };
}

export function fail(failures: string[], detail?: string): GateResult {
  return { status: "fail", failures, detail };
}

export function skip(reason: string): GateResult {
  return { status: "skip", skipReason: reason };
}
