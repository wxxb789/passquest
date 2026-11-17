import { goBuild } from "../go-build.ts";
import { GO_MOD } from "./fixture.ts";
import type { BrokenCase } from "./fixture.ts";

export function goBuildCases(): BrokenCase[] {
  return [
    {
      gate: goBuild,
      label: "compile error",
      files: {
        "go.mod": GO_MOD,
        "main.go": "package main\n\nfunc main() { missing() }\n",
      },
    },
  ];
}
