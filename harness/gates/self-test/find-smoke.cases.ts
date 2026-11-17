import { findSmoke } from "../find-smoke.ts";
import { GO_MOD } from "./fixture.ts";
import type { BrokenCase } from "./fixture.ts";

export function findSmokeCases(): BrokenCase[] {
  return [
    {
      gate: findSmoke,
      label: "CLI whose find output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
      },
    },
  ];
}
