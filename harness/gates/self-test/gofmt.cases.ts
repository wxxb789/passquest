import { gofmt } from "../gofmt.ts";
import type { BrokenCase } from "./fixture.ts";

export function gofmtCases(): BrokenCase[] {
  return [
    {
      gate: gofmt,
      label: "unformatted Go file",
      files: { "bad.go": "package bad\n\nfunc  F( ) { }\n" },
      tracked: ["bad.go"],
    },
  ];
}
