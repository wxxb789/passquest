import { adaptiveSmoke } from "../adaptive-smoke.ts";
import { GO_MOD } from "./fixture.ts";
import type { BrokenCase } from "./fixture.ts";

export function adaptiveSmokeCases(): BrokenCase[] {
  return [
    {
      gate: adaptiveSmoke,
      label: "CLI whose skill and next output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
      },
    },
  ];
}
