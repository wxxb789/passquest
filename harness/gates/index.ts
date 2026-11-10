import type { Gate } from "../lib/types.ts";

import { baseline } from "./baseline.ts";
import { docsLinks } from "./docs-links.ts";
import { goBuild } from "./go-build.ts";
import { goTest } from "./go-test.ts";
import { goTidy } from "./go-tidy.ts";
import { goVet } from "./go-vet.ts";
import { gofmt } from "./gofmt.ts";
import { importSmoke } from "./import-smoke.ts";
import { localOnly } from "./local-only.ts";
import { secrets } from "./secrets.ts";
import { selfTest } from "./self-test.ts";
import { typecheck } from "./typecheck.ts";

/** Gate order is the reported order: cheap and structural checks first. */
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
  selfTest,
];
