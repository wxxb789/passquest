import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";

import { noteSmoke } from "../note-smoke.ts";
import { GO_MOD, copyProduct, NOTE_SMOKE_SOURCES } from "./fixture.ts";
import type { BrokenCase } from "./fixture.ts";

/**
 * Copies the sources note-smoke builds into the fixture root and removes the
 * deck comparison from the push. A gate that keeps passing without the
 * comparison is not checking the deck claim, so it must fail here.
 */
async function removeTheDeckComparison(root: string): Promise<string | null> {
  if (copyProduct(root, NOTE_SMOKE_SOURCES) === 0) return "no product source was copied into the fixture root";
  const file = path.join(root, "internal", "cli", "note_anki.go");
  const source = readFileSync(file, "utf8");
  const anchor = "if deck != expected && !containsString(unexpected, deck) {";
  if (source.split(anchor).length !== 2) {
    return `the deck comparison anchor appears ${source.split(anchor).length - 1} time(s) in internal/cli/note_anki.go`;
  }
  writeFileSync(file, source.replace(anchor, "if false && !containsString(unexpected, deck) {"));
  return null;
}

export function noteSmokeCases(): BrokenCase[] {
  return [
    {
      gate: noteSmoke,
      label: "CLI whose note output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
        "internal/cli/testdata/import/question-q1.json": "{}",
      },
    },
    {
      gate: noteSmoke,
      label: "note push that reads the deck back and never compares it",
      files: {},
      prepare: removeTheDeckComparison,
      postFail: (failures) => {
        if (!failures.some((line) => line.includes("mismatch"))) {
          return `note-smoke: removing the deck comparison gave ${failures.length} failure(s) that never name the mismatch: ${failures.slice(0, 3).join("; ")}`;
        }
        if (failures.some((line) => line.includes("go build") || line.includes("import exit = ") || line.includes("first push exit"))) {
          return `note-smoke: the mutated tree broke an earlier stage: ${failures.slice(0, 3).join("; ")}`;
        }
        return null;
      },
    },
  ];
}
