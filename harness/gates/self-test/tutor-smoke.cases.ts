import { tutorSmoke } from "../tutor-smoke.ts";
import { copyProduct, GO_MOD, NOTE_SMOKE_SOURCES, patchCopy } from "./fixture.ts";
import type { BrokenCase } from "./fixture.ts";

async function validateTheChatBodyInTheResponsesBranch(root: string): Promise<string | null> {
  if (copyProduct(root, NOTE_SMOKE_SOURCES) === 0) return "no product source was copied into the fixture root";
  return patchCopy(
    root,
    "agent/tutor-host.mjs",
    'const RESPONSES_BODY_KEYS = ["instructions", "input", "max_output_tokens", "model", "store", "stream", "temperature"];',
    'const RESPONSES_BODY_KEYS = ["max_tokens", "messages", "model", "stream", "temperature"];',
  );
}

export function tutorSmokeCases(): BrokenCase[] {
  return [
    {
      gate: tutorSmoke,
      label: "CLI whose tutor output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
      },
    },
    {
      // The responses pass's own request check: the host must refuse the Go
      // document's responses body against its own key set and values.
      gate: tutorSmoke,
      label: "host whose responses branch validates the chat body",
      files: {},
      prepare: validateTheChatBodyInTheResponsesBranch,
      postFail: (failures) =>
        failures.some((line) => line.includes("responses:") && line.includes("exit ="))
          ? null
          : `tutor-smoke: did not report the responses pass: ${failures.slice(0, 3).join("; ")}`,
    },
  ];
}
