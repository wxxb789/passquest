import { assessSmoke } from "../assess-smoke.ts";
import { GO_MOD } from "./fixture.ts";
import type { BrokenCase } from "./fixture.ts";

export function assessSmokeCases(): BrokenCase[] {
  return [
    {
      gate: assessSmoke,
      label: "CLI whose assess output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
      },
    },
  ];
}
