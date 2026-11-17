import type { Gate } from "../lib/types.ts";

import { adaptiveSmoke } from "./adaptive-smoke.ts";
import { agentSmoke } from "./agent-smoke.ts";
import { assessSmoke } from "./assess-smoke.ts";
import { baseline } from "./baseline.ts";
import { desktopSmoke } from "./desktop-smoke.ts";
import { desktopUx } from "./desktop-ux.ts";
import { docsLinks } from "./docs-links.ts";
import { findSmoke } from "./find-smoke.ts";
import { goBuild } from "./go-build.ts";
import { goTest } from "./go-test.ts";
import { goTidy } from "./go-tidy.ts";
import { goVet } from "./go-vet.ts";
import { gofmt } from "./gofmt.ts";
import { goldenContract } from "./golden-contract.ts";
import { importSmoke } from "./import-smoke.ts";
import { localOnly } from "./local-only.ts";
import { noteSmoke } from "./note-smoke.ts";
import { portablePaths } from "./portable-paths.ts";
import { providerSmoke } from "./provider-smoke.ts";
import { secrets } from "./secrets.ts";
import { selfTest } from "./self-test.ts";
import { tacticSmoke } from "./tactic-smoke.ts";
import { tutorSmoke } from "./tutor-smoke.ts";
import { typecheck } from "./typecheck.ts";

export const gates: Gate[] = [
  typecheck,
  gofmt,
  goVet,
  goBuild,
  goTest,
  goTidy,
  docsLinks,
  secrets,
  localOnly,
  baseline,
  importSmoke,
  tacticSmoke,
  adaptiveSmoke,
  tutorSmoke,
  agentSmoke,
  noteSmoke,
  findSmoke,
  assessSmoke,
  portablePaths,
  providerSmoke,
  goldenContract,
  desktopSmoke,
  desktopUx,
  selfTest,
];

/**
 * The CI jobs: `bun run gate --group <name>` runs one group, in `gates` order. Every gate belongs to
 * exactly one group, and the workflow matrix lists exactly these names (both checked by
 * `groupProblems`), so no gate can drop out of CI. `core` needs Go, Bun and Node; `desktop` and
 * `self-test` also need a display and a Secret Service, since self-test feeds the desktop gates too.
 */
export const gateGroups: Record<string, readonly Gate[]> = {
  core: gates.filter((gate) => gate !== desktopSmoke && gate !== desktopUx && gate !== selfTest),
  desktop: [desktopSmoke, desktopUx],
  "self-test": [selfTest],
};
