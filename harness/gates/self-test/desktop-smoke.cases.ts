import { desktopSmoke, makeDesktopSmokeGate } from "../desktop-smoke.ts";
import { copyProduct, DESKTOP_SMOKE_SOURCES, GO_MOD, patchCopy } from "./fixture.ts";
import type { BrokenCase } from "./fixture.ts";

async function renameEndpointFormatsLine(root: string): Promise<string | null> {
  if (copyProduct(root, DESKTOP_SMOKE_SOURCES) === 0) return "no product source was copied into the fixture root";
  return patchCopy(
    root,
    "desktop/web/src/views/settings.ts",
    'data-testid="endpoint-formats"',
    'data-testid="endpoint-formats-missing"',
  );
}

/**
 * The stub desktop app the titlebar cases drive: its 880x600 walk reports a
 * room, every route and both appearances, and its pointer stage reports the
 * press taken over with the theme control's press and click intact. Each case
 * patches one piece of it to fail exactly what it is about.
 */
function titlebarStubFiles(): Record<string, string> {
  return {
    "go.mod": GO_MOD,
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

func stops() []any {
	out := []any{}
	for _, route := range []string{"today", "practice", "feedback", "assess", "report", "anki", "settings"} {
		for _, want := range []string{"light", "dark"} {
			issues := []any{}
			if route == "today" && want == "light" {
				issues = append(issues, map[string]any{"check": "titlebar-room-covered", "el": "header#top > div.util > button[data-act=theme]", "room": "top-right", "box": []any{1182, 13, 1214, 45}})
			}
			out = append(out, map[string]any{
				"route": route, "want": want, "shown": want,
				"room": map[string]any{"height": 40, "left": 0, "right": 138, "bottom": 40},
				"marker": true, "why": "", "counted": 70, "probe": []any{8, 20},
				"rects":  []any{map[string]any{"name": "top-right", "box": []any{1142, 0, 1280, 40}}},
				"paint":  []any{}, "skipped": map[string]any{},
				"issues": issues,
			})
		}
	}
	return out
}

func pointer() map[string]any {
	return map[string]any{
		"room":   map[string]any{"height": 40, "left": 0, "right": 138, "bottom": 40},
		"marker": true, "why": "", "issues": []any{}, "probe": []any{8, 20},
		"takeover":    map[string]any{"prevented": true, "hit": "header"},
		"maxBefore":   false, "maxAfter": true, "maxBack": false,
		"themePress":  map[string]any{"prevented": false, "hit": "themebtn"},
		"themeBefore": "dark", "themeFirst": "light", "themeBack": "dark",
	}
}

func main() {
	script, _ := os.ReadFile(os.Getenv("PASSQUEST_DESKTOP_SCRIPT"))
	switch {
	case strings.Contains(string(script), "desktop-smoke: boot/"):
		write(map[string]any{"textLength": 900, "stageChildren": 3, "stageTextLength": 800, "errorBars": 0, "ready": true, "probe": "clear", "opaqueErrors": 0})
	case strings.Contains(string(script), "desktop-smoke: titlebar/"):
		write(map[string]any{"size": map[string]any{"width": 880, "height": 600}, "stops": stops(), "pointer": pointer()})
	}
}
`,
    "cmd/passquest/main.go": 'package main\n\nfunc main() {}\n',
    "desktop/web/src/main.ts": 'document.title = "selftest";\n',
  };
}

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
        "cmd/passquest-desktop/main.go": `package main\n\nimport "os"\n\nfunc main() {\n\tblank := \`{"ok":true,"value":{"textLength":0,"stageChildren":0,"stageTextLength":0,"errorBars":0,"ready":null,"probe":"clear","opaqueErrors":0}}\`\n\t_ = os.WriteFile(os.Getenv("PASSQUEST_DESKTOP_RESULT"), []byte(blank), 0o600)\n}\n`,
        "desktop/web/src/main.ts": 'document.title = "selftest";\n',
      },
      postFail: (failures) =>
        failures.some((line) => line.startsWith("boot: blank Today"))
          ? null
          : `desktop-smoke: did not report the blank window: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      // The default-data scenario's own assertion path: this app keeps its
      // database beside its executable instead of the per-user root, so the
      // both-directions check must fail naming the missing per-user path.
      gate: makeDesktopSmokeGate(["default-data"]),
      label: "desktop window that keeps its database outside the per-user root",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest-desktop/main.go": `package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func write(value any) {
	document, _ := json.Marshal(map[string]any{"ok": true, "value": value})
	_ = os.WriteFile(os.Getenv("PASSQUEST_DESKTOP_RESULT"), document, 0o600)
}

func main() {
	exe, _ := os.Executable()
	dir := filepath.Join(filepath.Dir(exe), "Pass Quest")
	_ = os.MkdirAll(dir, 0o700)
	_ = os.WriteFile(filepath.Join(dir, "passquest.sqlite"), []byte("fixture"), 0o600)
	script, _ := os.ReadFile(os.Getenv("PASSQUEST_DESKTOP_SCRIPT"))
	if strings.Contains(string(script), "desktop-smoke: boot/") {
		write(map[string]any{"textLength": 900, "stageChildren": 3, "stageTextLength": 800, "errorBars": 0, "ready": true, "probe": "clear", "opaqueErrors": 0})
		return
	}
	write(map[string]any{})
}
`,
        "cmd/passquest/main.go": 'package main\n\nfunc main() {}\n',
        "desktop/web/src/main.ts": 'document.title = "selftest";\n',
      },
      postFail: (failures) =>
        failures.some((line) => line.startsWith("default-data: the window left") && line.includes("passquest.sqlite"))
          ? null
          : `desktop-smoke: did not report the database outside the per-user root: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      gate: desktopSmoke,
      label: "desktop window whose boot probe never reaches window.onerror",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest-desktop/main.go": `package main\n\nimport "os"\n\nfunc main() {\n\tstuck := \`{"ok":true,"value":{"textLength":900,"stageChildren":3,"stageTextLength":800,"errorBars":0,"ready":true,"probe":"pending","opaqueErrors":0}}\`\n\t_ = os.WriteFile(os.Getenv("PASSQUEST_DESKTOP_RESULT"), []byte(stuck), 0o600)\n}\n`,
        "desktop/web/src/main.ts": 'document.title = "selftest";\n',
      },
      postFail: (failures) =>
        failures.some((line) => line.startsWith("boot: the bundle's own error did not reach window.onerror in full"))
          ? null
          : `desktop-smoke: did not report the stuck probe: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      gate: desktopSmoke,
      label: "desktop window where the smoke hook answers without the shot directory",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest-desktop/main.go": `package main\n\nimport "os"\n\nfunc main() {\n\tanswered := \`{"ok":true,"value":{"textLength":900,"stageChildren":3,"stageTextLength":800,"errorBars":0,"ready":true,"probe":"clear","opaqueErrors":0,"shotBound":true,"shotError":""}}\`\n\t_ = os.WriteFile(os.Getenv("PASSQUEST_DESKTOP_RESULT"), []byte(answered), 0o600)\n}\n`,
        "desktop/web/src/main.ts": 'document.title = "selftest";\n',
      },
      postFail: (failures) =>
        failures.some((line) => line.startsWith("boot: a call to SmokeShot.Capture without PASSQUEST_DESKTOP_SHOT_DIR"))
          ? null
          : `desktop-smoke: did not report the smoke hook that answered: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      gate: desktopSmoke,
      label: "desktop window whose title buttons' theme call fails",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest-desktop/main.go": `package main\n\nimport "os"\n\nfunc main() {\n\tfailed := \`{"ok":true,"value":{"textLength":900,"stageChildren":3,"stageTextLength":800,"errorBars":0,"ready":true,"probe":"clear","opaqueErrors":0,"buttonsThemeError":"mygo: Theme.SetGTKCSS: 3:1: Not a valid selector"}}\`\n\t_ = os.WriteFile(os.Getenv("PASSQUEST_DESKTOP_RESULT"), []byte(failed), 0o600)\n}\n`,
        "desktop/web/src/main.ts": 'document.title = "selftest";\n',
      },
      postFail: (failures) =>
        failures.some((line) => line.startsWith("boot: the title buttons' theme call failed"))
          ? null
          : `desktop-smoke: did not report the failed title button theme call: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      gate: makeDesktopSmokeGate(["practice"]),
      label: "desktop window whose page has no mygo runtime, whose later scenario still runs",
      files: {
        "go.mod": GO_MOD,
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
		write(map[string]any{"textLength": 900, "stageChildren": 3, "stageTextLength": 800, "errorBars": 0, "ready": nil, "probe": "clear", "opaqueErrors": 0, "shotBound": nil, "shotError": ""})
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
          : `desktop-smoke: did not report the missing attempt after a boot without a mygo runtime: ${failures.slice(0, 3).join("; ")}`,
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
		write(map[string]any{"textLength": 900, "stageChildren": 3, "stageTextLength": 800, "errorBars": 0, "ready": true, "probe": "clear", "opaqueErrors": 0})
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
    {
      // The failure is decided in the practice scenario's mark-popover step; the later scenarios
      // would only relaunch the stub.
      gate: makeDesktopSmokeGate(["practice"]),
      label: "desktop practice flow whose mark popover never shows",
      files: {
        "go.mod": GO_MOD,
        // Claims the answer and the mark the scenario asks for, but the popover reports hidden.
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
		write(map[string]any{"textLength": 900, "stageChildren": 3, "stageTextLength": 800, "errorBars": 0, "ready": true, "probe": "clear", "opaqueErrors": 0})
	case strings.Contains(string(script), "desktop-smoke: practice/"):
		write(map[string]any{
			"markpop": map[string]string{"popover": "auto", "open": "false", "topLayer": "false", "taken": "false", "countBefore": "b", "countAfter": "b"},
			"answers": []any{map[string]any{
				"item":      map[string]string{"token": "pt-selftest", "question": "q1", "presentedAt": "2026-01-01T00:00:00Z"},
				"condition": map[string]string{"condition": "independent", "canCount": "true"},
				"chosen":    "B",
				"feedback":  map[string]string{"attempt": "1", "verdict": "correct", "elapsedMs": "1200"},
			}},
		})
	}
}
`,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println(`{"attempts":[]}`) }\n',
        "desktop/web/src/main.ts": 'document.title = "selftest";\n',
      },
      postFail: (failures) =>
        failures.some((line) => line.startsWith("practice: the mark popover after a selection"))
          ? null
          : `desktop-smoke: did not report the mark popover that never showed: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      // The failure is decided in the update scenario: its own launch deviates
      // from the allowed request shape and nothing else runs it.
      gate: makeDesktopSmokeGate(["update"]),
      label: "desktop window whose update check adds a header",
      files: {
        "go.mod": GO_MOD,
        "mygo.json": '{"name": "selftest", "version": "0.15.0", "main": "./cmd/passquest-desktop"}\n',
        "internal/update/update.go": `package update

const Endpoint = "https://api.github.com/repos/wxxb789/passquest/releases/latest"
`,
        "cmd/passquest-desktop/main.go": `package main

import (
	"encoding/json"
	"net/http"
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
		write(map[string]any{"textLength": 900, "stageChildren": 3, "stageTextLength": 800, "errorBars": 0, "ready": true, "probe": "clear", "opaqueErrors": 0})
	case strings.Contains(string(script), "desktop-smoke: update/"):
		request, _ := http.NewRequest("GET", os.Getenv("PASSQUEST_DESKTOP_UPDATE_URL"), nil)
		request.Header.Set("x-device", "selftest")
		_, _ = http.DefaultClient.Do(request)
		write(map[string]any{})
	}
}
`,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "desktop/web/src/main.ts": 'document.title = "selftest";\n',
      },
      postFail: (failures) =>
        failures.some((line) => line.startsWith("update: request 1 carried header(s) x-device"))
          ? null
          : `desktop-smoke: did not report the extra request header: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      // The other half of the update scenario's data-directory check: the
      // framework's own window-state.json is no longer tolerated, and a file
      // written into the data directory during the launch must be reported by
      // name. The launch's PASSQUEST_DESKTOP_CONTENT_SIZE is what keeps the
      // real app from writing that file; this fake app writes it anyway.
      gate: makeDesktopSmokeGate(["update"]),
      label: "desktop window that saves window state into the data directory",
      files: {
        "go.mod": GO_MOD,
        "mygo.json": '{"name": "selftest", "version": "0.15.0", "main": "./cmd/passquest-desktop"}\n',
        "internal/update/update.go": `package update

const Endpoint = "https://api.github.com/repos/wxxb789/passquest/releases/latest"
`,
        "cmd/passquest-desktop/main.go": `package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
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
		write(map[string]any{"textLength": 900, "stageChildren": 3, "stageTextLength": 800, "errorBars": 0, "ready": true, "probe": "clear", "opaqueErrors": 0})
	case strings.Contains(string(script), "desktop-smoke: update/"):
		state := filepath.Join(os.Getenv("PASSQUEST_DESKTOP_DATA"), "window-state.json")
		_ = os.WriteFile(state, []byte("{}\\n"), 0o600)
		request, _ := http.NewRequest("GET", os.Getenv("PASSQUEST_DESKTOP_UPDATE_URL"), nil)
		request.Header.Set("Accept", "application/vnd.github+json")
		request.Header.Set("User-Agent", "PassQuest")
		_, _ = http.DefaultClient.Do(request)
		write(map[string]any{})
	}
}
`,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "desktop/web/src/main.ts": 'document.title = "selftest";\n',
      },
      postFail: (failures) =>
        failures.some((line) => line.startsWith("update: the data directory changed across the checks") && line.includes("added window-state.json"))
          ? null
          : `desktop-smoke: did not report the window state written into the data directory: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      // The titlebar scenario's own assertion path: its 880x600 walk reported a room
      // and a control under it, so the gate must fail naming that finding.
      gate: makeDesktopSmokeGate(["titlebar"]),
      label: "desktop window whose 880x600 walk reports a control under the caption buttons",
      files: titlebarStubFiles(),
      postFail: (failures) =>
        failures.some((line) => line.startsWith("titlebar: ") && line.includes("titlebar-room-covered"))
          ? null
          : `desktop-smoke: did not report the control under the caption buttons: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      // The settings scenario's own assertion path: the page line naming both
      // endpoint suffixes is read from the DOM, so the real page without that
      // hook must fail by the line's name.
      gate: makeDesktopSmokeGate(["settings"]),
      label: "a settings page whose endpoint formats line is missing",
      files: {},
      prepare: renameEndpointFormatsLine,
      postFail: (failures) =>
        failures.some((line) => line.includes("timed out waiting for the endpoint formats line"))
          ? null
          : `desktop-smoke: did not report the missing endpoint formats line: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      // The pointer stage's own assertion path: the stub reports that the press
      // at the strip's probe point was not taken over, so the gate must fail
      // naming the takeover (goal 0022's Done-when 2 evidence).
      gate: makeDesktopSmokeGate(["titlebar"]),
      label: "desktop window whose title-strip press was not taken over as a window drag",
      files: titlebarStubFiles(),
      prepare: async (root) => patchCopy(root, "cmd/passquest-desktop/main.go", '"prevented": true', '"prevented": false'),
      postFail: (failures) =>
        failures.some((line) => line.startsWith("titlebar: ") && line.includes("was not taken over"))
          ? null
          : `desktop-smoke: did not report the untaken press: ${failures.slice(0, 3).join("; ")}`,
    },
  ];
}
