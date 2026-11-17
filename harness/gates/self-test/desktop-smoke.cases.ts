import { desktopSmoke, makeDesktopSmokeGate } from "../desktop-smoke.ts";
import { GO_MOD } from "./fixture.ts";
import type { BrokenCase } from "./fixture.ts";

export function desktopSmokeCases(): BrokenCase[] {
  return [
    {
      gate: desktopSmoke,
      label: "desktop main that exits 0 without writing a result",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest-desktop/main.go": "package main\n\nfunc main() {}\n",
        "desktop/web/src/main.ts": 'document.title = "selftest";\n',
      },
      postFail: (failures) =>
        failures.some((line) => line.startsWith("boot: app exited 0 without writing a result"))
          ? null
          : `desktop-smoke: did not report the missing result: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      gate: desktopSmoke,
      label: "desktop window whose Today stays blank",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest-desktop/main.go": `package main\n\nimport "os"\n\nfunc main() {\n\tblank := \`{"ok":true,"value":{"textLength":0,"stageChildren":0,"stageTextLength":0,"errorBars":0,"ready":null}}\`\n\t_ = os.WriteFile(os.Getenv("PASSQUEST_DESKTOP_RESULT"), []byte(blank), 0o600)\n}\n`,
        "desktop/web/src/main.ts": 'document.title = "selftest";\n',
      },
      postFail: (failures) =>
        failures.some((line) => line.startsWith("boot: blank Today"))
          ? null
          : `desktop-smoke: did not report the blank window: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      gate: desktopSmoke,
      label: "desktop binary whose generated client differs from the committed one",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest-desktop/main.go": `package main

import "os"

func main() {
	if out := os.Getenv("MYGO_GENERATE"); out != "" {
		_ = os.WriteFile(out, []byte("export const client = 2;\\n"), 0o600)
	}
}
`,
        "desktop/web/src/main.ts": 'document.title = "selftest";\n',
        "desktop/web/src/api/mygo.ts": "export const client = 1;\r\n",
      },
      postFail: (failures) =>
        failures.some((line) => line.startsWith("client-fresh: desktop/web/src/api/mygo.ts is stale"))
          ? null
          : `desktop-smoke: did not report the stale client: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      // The failure is decided in the practice scenario; the later scenarios would only relaunch the stub.
      gate: makeDesktopSmokeGate(["practice"]),
      label: "desktop practice flow that writes no attempt",
      files: {
        "go.mod": GO_MOD,
        // Boots and claims a keyboard answer, but the second process finds no attempt.
        "cmd/passquest-desktop/main.go": `package main

import (
	"encoding/json"
	"os"
	"strings"
)

func write(value any) {
	document, _ := json.Marshal(map[string]any{"ok": true, "value": value})
	_ = os.WriteFile(os.Getenv("PASSQUEST_DESKTOP_RESULT"), document, 0o600)
}

func main() {
	script, _ := os.ReadFile(os.Getenv("PASSQUEST_DESKTOP_SCRIPT"))
	switch {
	case strings.Contains(string(script), "desktop-smoke: boot/"):
		write(map[string]any{"textLength": 900, "stageChildren": 3, "stageTextLength": 800, "errorBars": 0, "ready": true})
	case strings.Contains(string(script), "desktop-smoke: practice/"):
		write(map[string]any{"answers": []any{map[string]any{
			"item":      map[string]string{"token": "pt-selftest", "question": "q1", "presentedAt": "2026-01-01T00:00:00Z"},
			"condition": map[string]string{"condition": "independent", "canCount": "true"},
			"chosen":    "B",
			"feedback":  map[string]string{"attempt": "1", "verdict": "correct", "elapsedMs": "1200"},
		}}})
	}
}
`,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println(`{"attempts":[]}`) }\n',
        "desktop/web/src/main.ts": 'document.title = "selftest";\n',
      },
      postFail: (failures) =>
        failures.some((line) => line.startsWith("practice: report --json has no attempt 1 "))
          ? null
          : `desktop-smoke: did not report the missing attempt: ${failures.slice(0, 3).join("; ")}`,
    },
  ];
}
