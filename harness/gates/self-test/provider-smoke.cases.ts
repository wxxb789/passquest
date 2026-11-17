import { providerSmoke } from "../provider-smoke.ts";
import { GO_MOD } from "./fixture.ts";
import type { BrokenCase } from "./fixture.ts";

export function providerSmokeCases(): BrokenCase[] {
  return [
    {
      gate: providerSmoke,
      label: "CLI whose model output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
      },
    },
  ];
}
