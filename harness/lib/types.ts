export type GateStatus = "pass" | "fail" | "skip";

export interface GateContext {
  /** Repository root the gate must operate on. */
  root: string;
  /** Repository-relative paths of tracked files, `/` separated. */
  trackedFiles: () => Promise<string[]>;
}

export interface GateResult {
  status: GateStatus;
  /** Short numeric or one-line detail shown after PASS. */
  detail?: string;
  /** Failure lines shown under FAIL. */
  failures?: string[];
  /** Why a gate did not run. */
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
