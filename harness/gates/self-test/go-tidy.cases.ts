import { goTidy } from "../go-tidy.ts";
import type { BrokenCase } from "./fixture.ts";

export function goTidyCases(): BrokenCase[] {
  return [
    {
      gate: goTidy,
      label: "stale module requirement",
      files: {
        "go.mod": 'module example.com/selftest\n\ngo 1.27.3\n\nrequire example.com/dep v0.0.0\n\nreplace example.com/dep => ./dep\n',
        "dep/go.mod": "module example.com/dep\n\ngo 1.27.3\n",
        "main.go": "package main\n\nfunc main() {}\n",
      },
    },
  ];
}
