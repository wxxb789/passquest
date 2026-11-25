import { providerSmoke } from "../provider-smoke.ts";
import { copyProduct, GO_MOD, NOTE_SMOKE_SOURCES, patchCopy } from "./fixture.ts";
import type { BrokenCase } from "./fixture.ts";

async function refuseEverySuffixBesideChatCompletions(root: string): Promise<string | null> {
  if (copyProduct(root, NOTE_SMOKE_SOURCES) === 0) return "no product source was copied into the fixture root";
  return patchCopy(
    root,
    "internal/provider/registry.go",
    "if _, ok := wire.FormatOfPath(u.Path); !ok {",
    'if !strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/chat/completions") {',
  );
}

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
    {
      // The declared responses provider's own acceptance: the registry must
      // accept the responses endpoint the fixture provider declares.
      gate: providerSmoke,
      label: "registry whose endpoint rule is chat-only",
      files: {},
      prepare: refuseEverySuffixBesideChatCompletions,
      postFail: (failures) =>
        failures.some((line) => line.startsWith("delta:") || line.includes("mismatched registry"))
          ? null
          : `provider-smoke: did not report the refused responses provider: ${failures.slice(0, 4).join("; ")}`,
    },
  ];
}
