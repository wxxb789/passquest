import { goVet } from "../go-vet.ts";
import { GO_MOD } from "./fixture.ts";
import type { BrokenCase } from "./fixture.ts";

export function goVetCases(): BrokenCase[] {
  return [
    {
      gate: goVet,
      label: "vet printf diagnostic",
      files: {
        "go.mod": GO_MOD,
        "main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Printf("%d", "x") }\n',
      },
    },
  ];
}
