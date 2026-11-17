import { secrets } from "../secrets.ts";
import type { BrokenCase } from "./fixture.ts";

export function secretsCases(): BrokenCase[] {
  const fakeKey = `sk-${"A".repeat(24)}`;
  return [
    {
      gate: secrets,
      label: "credential-shaped string",
      files: { "leak.txt": `const key = "${fakeKey}";\n` },
      tracked: ["leak.txt"],
      postFail: (failures) =>
        failures.some((line) => line.includes(fakeKey)) ? "secrets: failure output echoed the credential value" : null,
    },
    {
      gate: secrets,
      label: "credential in a large tracked text file",
      files: { "large.md": `${"x".repeat(5 * 1024 * 1024 + 1)}\nconst key = "${fakeKey}";\n` },
      tracked: ["large.md"],
      postFail: (failures) => {
        if (!failures.some((line) => line.includes("large.md") && line.includes("OpenAI-style key"))) {
          return "secrets: did not scan the large text file";
        }
        if (failures.some((line) => line.includes(fakeKey))) {
          return "secrets: failure output echoed the credential value";
        }
        return null;
      },
    },
  ];
}
