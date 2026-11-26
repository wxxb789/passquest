import { secrets } from "../secrets.ts";
import type { BrokenCase, PassCase } from "./fixture.ts";

function markerAt(failures: string[], location: string): string | null {
  if (!failures.some((line) => line.startsWith(`${location}: merge-conflict marker`))) {
    return `secrets: no conflict-marker failure at ${location} (${failures.join("; ") || "no failures"})`;
  }
  return null;
}

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
    {
      gate: secrets,
      label: "conflict opening marker",
      files: { "conflict.txt": "<<<<<<< ours\n" },
      tracked: ["conflict.txt"],
      postFail: (failures) => markerAt(failures, "conflict.txt:1"),
    },
    {
      gate: secrets,
      label: "conflict base marker (the 0021 incident shape)",
      files: { "conflict.md": "||||||| parent of 5852feb (Incident)\n" },
      tracked: ["conflict.md"],
      postFail: (failures) => markerAt(failures, "conflict.md:1"),
    },
    {
      gate: secrets,
      label: "conflict closing marker after resolved lines",
      files: { "conflict.txt": "resolved text\n>>>>>>> feature/conflict-markers\n" },
      tracked: ["conflict.txt"],
      postFail: (failures) => markerAt(failures, "conflict.txt:2"),
    },
    {
      gate: secrets,
      label: "conflict block with its separator",
      files: { "conflict.md": "<<<<<<< ours\nours\n=======\ntheirs\n>>>>>>> theirs\n" },
      tracked: ["conflict.md"],
      postFail: (failures) => {
        for (const location of ["conflict.md:1", "conflict.md:3", "conflict.md:5"]) {
          const problem = markerAt(failures, location);
          if (problem) return problem;
        }
        return null;
      },
    },
  ];
}

export function secretsPassCases(): PassCase[] {
  return [
    {
      gate: secrets,
      label: "markdown setext underline",
      files: { "notes.md": "Title\n=======\n\nBody.\n" },
      tracked: ["notes.md"],
      postPass: (detail) => (detail.includes("0 findings") ? null : `secrets: the setext underline case reported ${detail}`),
    },
  ];
}
