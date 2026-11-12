import type { Gate } from "../lib/types.ts";

import { adaptiveSmoke } from "./adaptive-smoke.ts";
import { agentSmoke } from "./agent-smoke.ts";
import { assessSmoke } from "./assess-smoke.ts";
import { baseline } from "./baseline.ts";
import { docsLinks } from "./docs-links.ts";
import { findSmoke } from "./find-smoke.ts";
import { goBuild } from "./go-build.ts";
import { goTest } from "./go-test.ts";
import { goTidy } from "./go-tidy.ts";
import { goVet } from "./go-vet.ts";
import { gofmt } from "./gofmt.ts";
import { importSmoke } from "./import-smoke.ts";
import { localOnly } from "./local-only.ts";
import { noteSmoke } from "./note-smoke.ts";
import { portablePaths } from "./portable-paths.ts";
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
  selfTest,
];
