import { docsLinks } from "../docs-links.ts";
import type { BrokenCase } from "./fixture.ts";

export function docsLinksCases(): BrokenCase[] {
  return [
    {
      gate: docsLinks,
      label: "broken relative link",
      files: { "README.md": "[missing](does-not-exist.md)\n" },
      tracked: ["README.md"],
    },
  ];
}
