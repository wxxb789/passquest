import { localOnly } from "../local-only.ts";
import { GO_MOD } from "./fixture.ts";
import type { BrokenCase, PassCase } from "./fixture.ts";

/**
 * The allowed network package at its minimum: the one allowed net import, and
 * the two literal forms the local-only gate's rule 4 admits.
 */
const UPDATE_FIXTURE = `package update

import "net/http"

const Endpoint = "https://api.github.com/repos/wxxb789/passquest/releases/latest"
const releasesURL = "https://github.com/wxxb789/passquest/releases/"

var _ = http.MethodGet
`;

export function localOnlyCases(): BrokenCase[] {
  return [
    {
      gate: localOnly,
      label: "first-party net/http import",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("ok") }\n',
        "internal/x/x.go": 'package x\n\nimport "net/http"\n\nvar _ = http.MethodGet\n',
      },
      tracked: [],
      postFail: (failures) =>
        failures.some((line) => line.includes("first-party network import"))
          ? null
          : "local-only: did not report the first-party network import",
    },
    {
      gate: localOnly,
      label: "network client linked through a module dependency",
      files: {
        "go.mod": 'module example.com/selftest\n\ngo 1.27.3\n\nrequire example.com/dep v0.0.0\n\nreplace example.com/dep => ./dep\n',
        "dep/go.mod": "module example.com/dep\n\ngo 1.27.3\n",
        "dep/client.go": 'package dep\n\nimport "net/http"\n\nfunc Client() *http.Client { return &http.Client{} }\n',
        "cmd/passquest/main.go": 'package main\n\nimport "example.com/dep"\n\nfunc main() { _ = dep.Client() }\n',
        "internal/x/x.go": "package x\n",
      },
      tracked: [],
      postFail: (failures) => {
        if (!failures.some((line) => line.includes("network client linked into the CLI: net/http"))) {
          return "local-only: did not report the linked network client";
        }
        if (failures.some((line) => line.includes("first-party network import"))) {
          return "local-only: first-party scan also matched the dependency import";
        }
        return null;
      },
    },
    {
      gate: localOnly,
      label: "confidential marker in a scanned non-Go file",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/x/x.go": "package x\n",
        "internal/x/notes.md": "# notes\n\n仅限内部使用\n",
      },
      tracked: ["internal/x/notes.md"],
      postFail: (failures) =>
        failures.length === 1 && failures[0].includes("confidential source marker")
          ? null
          : `local-only: expected one confidential marker failure, saw ${failures.length}`,
    },
    {
      gate: localOnly,
      label: "desktop main that imports net/http itself",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/x/x.go": "package x\n",
        "cmd/passquest-desktop/main.go": 'package main\n\nimport "net/http"\n\nfunc main() { _ = http.MethodGet }\n',
      },
      tracked: [],
      postFail: (failures) => {
        if (!failures.some((line) => line.includes("first-party network import") && line.includes("cmd/passquest-desktop"))) {
          return "local-only: did not report the desktop main's network import";
        }
        if (failures.some((line) => line.includes("network client linked into the CLI"))) {
          return "local-only: blamed the CLI for the desktop main's import";
        }
        return null;
      },
    },
    {
      gate: localOnly,
      label: "confidential marker in desktop frontend source",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/x/x.go": "package x\n",
        "desktop/web/src/main.ts": 'export const banner = "严禁外传";\n',
      },
      tracked: ["desktop/web/src/main.ts"],
      postFail: (failures) =>
        failures.length === 1 && failures[0].startsWith("desktop/web/src/main.ts:1: confidential source marker")
          ? null
          : `local-only: expected one marker failure in desktop/web/src/main.ts, saw ${failures.join("; ")}`,
    },
    {
      gate: localOnly,
      label: "study package importing the CLI adapter",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/cli/cli.go": "package cli\n\nconst X = 1\n",
        "internal/study/practice/practice.go":
          'package practice\n\nimport "example.com/selftest/internal/cli"\n\nvar _ = cli.X\n',
      },
      tracked: [],
      postFail: (failures) =>
        failures.some((line) => line.includes("module boundary: internal/study/practice imports internal/cli"))
          ? null
          : "local-only: did not report the study -> cli import",
    },
    {
      gate: localOnly,
      label: "network import in a sub-package of the one allowed package",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/update/sub/sub.go": 'package sub\n\nimport "net/http"\n\nvar _ = http.MethodGet\n',
      },
      tracked: [],
      postFail: (failures) =>
        failures.some((line) => line.includes("first-party network import") && line.includes("internal/update/sub"))
          ? null
          : "local-only: the network exception leaked past the allowed package",
    },
    {
      gate: localOnly,
      label: "study package importing a sub-package of a banned module",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/workspace/sub/sub.go": "package sub\n\nconst X = 1\n",
        "internal/study/internal/kernel/kernel.go":
          'package kernel\n\nimport "example.com/selftest/internal/workspace/sub"\n\nvar _ = sub.X\n',
      },
      tracked: [],
      postFail: (failures) =>
        failures.some((line) => line.includes("module boundary: internal/study/internal/kernel imports internal/workspace/sub"))
          ? null
          : "local-only: an exact-match rule missed the sub-package import",
    },
    {
      gate: localOnly,
      label: "net/http sub-package import inside the allowed package",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/update/update.go": `package update

import (
	"net/http"
	"net/http/cookiejar"
)

const Endpoint = "https://api.github.com/repos/wxxb789/passquest/releases/latest"

var _ = http.MethodGet
var _ = cookiejar.New
`,
      },
      tracked: ["internal/update/update.go"],
      postFail: (failures) =>
        failures.length === 1 && failures[0].includes("first-party network import") && failures[0].includes("net/http/cookiejar")
          ? null
          : `local-only: expected one net/http/cookiejar failure, saw ${failures.join("; ")}`,
    },
    {
      gate: localOnly,
      label: "CLI importing the update package",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "example.com/selftest/internal/update"\n\nfunc main() { _ = update.Endpoint }\n',
        "internal/update/update.go": UPDATE_FIXTURE,
      },
      tracked: ["internal/update/update.go"],
      postFail: (failures) =>
        failures.some(
          (line) => line.includes("network client linked into the CLI") && line.includes("example.com/selftest/internal/update"),
        )
          ? null
          : `local-only: did not report the update package linked into the CLI (${failures.join("; ")})`,
    },
    {
      gate: localOnly,
      label: "update package imported outside the desktop",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/update/update.go": UPDATE_FIXTURE,
        "internal/x/x.go": 'package x\n\nimport "example.com/selftest/internal/update"\n\nvar _ = update.Endpoint\n',
      },
      tracked: ["internal/update/update.go"],
      postFail: (failures) =>
        failures.length === 1 &&
        failures[0].includes("may be imported only by internal/desktop and cmd/passquest-desktop") &&
        failures[0].includes("internal/x imports it")
          ? null
          : `local-only: expected one import-surface failure, saw ${failures.join("; ")}`,
    },
    {
      gate: localOnly,
      label: "endpoint literal naming another host",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/update/update.go": `package update

import "net/http"

const Endpoint = "https://evil.example/repos/wxxb789/passquest/releases/latest"

var _ = http.MethodGet
`,
      },
      tracked: ["internal/update/update.go"],
      postFail: (failures) =>
        failures.some((line) => line.includes("neither the constant endpoint nor the release-page prefix") && line.includes("evil.example"))
          ? null
          : `local-only: did not report the endpoint on another host (${failures.join("; ")})`,
    },
    {
      gate: localOnly,
      label: "endpoint literal with a drifted path",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/update/update.go": `package update

import "net/http"

const Endpoint = "https://api.github.com/repos/wxxb789/passquest/releases"

var _ = http.MethodGet
`,
      },
      tracked: ["internal/update/update.go"],
      postFail: (failures) =>
        failures.some(
          (line) =>
            line.includes("neither the constant endpoint nor the release-page prefix") &&
            line.includes('"https://api.github.com/repos/wxxb789/passquest/releases"'),
        )
          ? null
          : `local-only: did not report the drifted endpoint path (${failures.join("; ")})`,
    },
    {
      gate: localOnly,
      label: "network import in a cmd package beside the CLI",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "cmd/passquest-updater/main.go": 'package main\n\nimport "net/http"\n\nfunc main() { _ = http.MethodGet }\n',
      },
      tracked: [],
      postFail: (failures) =>
        failures.some((line) => line.includes("first-party network import") && line.includes("cmd/passquest-updater"))
          ? null
          : `local-only: did not report the network import in a new cmd package (${failures.join("; ")})`,
    },
    {
      gate: localOnly,
      label: "net/http/cookiejar import in another package",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/x/x.go": 'package x\n\nimport "net/http/cookiejar"\n\nvar _ = cookiejar.New\n',
      },
      tracked: [],
      postFail: (failures) =>
        failures.includes("first-party network import: example.com/selftest/internal/x: net/http/cookiejar")
          ? null
          : `local-only: did not report the cookiejar import outside the allowed package (${failures.join("; ")})`,
    },
    {
      gate: localOnly,
      label: "http(s) literal in a nested package of the allowed package",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/update/update.go": UPDATE_FIXTURE,
        "internal/update/internal/pin/pin.go": `package pin

const Mirror = "https://evil.example/repos/wxxb789/passquest/releases/latest"
`,
      },
      tracked: ["internal/update/update.go", "internal/update/internal/pin/pin.go"],
      postFail: (failures) =>
        failures.some(
          (line) =>
            line.includes("neither the constant endpoint nor the release-page prefix") &&
            line.includes("internal/update/internal/pin/pin.go") &&
            line.includes("evil.example"),
        )
          ? null
          : `local-only: did not read the nested package's literal (${failures.join("; ")})`,
    },
    {
      gate: localOnly,
      label: "two endpoint literals in the allowed package",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/update/update.go": `package update

import "net/http"

const Endpoint = "https://api.github.com/repos/wxxb789/passquest/releases/latest"
const Mirror = "https://api.github.com/repos/wxxb789/passquest/releases/latest"

var _ = http.MethodGet
`,
      },
      tracked: ["internal/update/update.go"],
      postFail: (failures) =>
        failures.length === 1 && failures[0].includes("expected exactly one") && failures[0].includes("saw 2")
          ? null
          : `local-only: expected one exactly-one-endpoint failure, saw ${failures.join("; ")}`,
    },
    {
      gate: localOnly,
      label: "allowed package absent from the working-tree file list",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/update/update.go": UPDATE_FIXTURE,
      },
      // No tracked list: `go list` sees the package, but the working-tree walk the gate reads
      // rule 4's files from returns nothing, so the rule has nothing to pin.
      tracked: [],
      postFail: (failures) =>
        failures.length === 1 &&
        failures[0] === "internal/update: no non-test Go file in the working tree to read the constant endpoint from"
          ? null
          : `local-only: expected the empty-working-tree failure, saw ${failures.join("; ")}`,
    },
    {
      gate: localOnly,
      label: "unreadable update literal keeps the other problems and the marker scan",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/x/x.go": 'package x\n\nimport "net/http"\n\nvar _ = http.MethodGet\n',
        "internal/x/notes.md": "# notes\n\n仅限内部使用\n",
        "internal/update/update.go": UPDATE_FIXTURE,
        "internal/update/sub.go/placeholder": "not a Go file\n",
      },
      // The tracked list names internal/update/sub.go, a directory on disk: the
      // literal tool cannot read it and exits non-zero, mid-subtree.
      tracked: ["internal/x/notes.md", "internal/update/update.go", "internal/update/sub.go"],
      postFail: (failures) => {
        const missing = [
          "first-party network import: example.com/selftest/internal/x: net/http",
          "http(s) literals of internal/update could not be read",
          "internal/x/notes.md:3: confidential source marker",
        ].filter((expected) => !failures.some((line) => line.includes(expected)));
        return missing.length === 0 ? null : `local-only: read failure dropped ${missing.join("; ")} (saw ${failures.join("; ")})`;
      },
    },
  ];
}

export function localOnlyPassCases(): PassCase[] {
  return [
    {
      gate: localOnly,
      label: "the one allowed network package",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/update/update.go": UPDATE_FIXTURE,
      },
      tracked: ["internal/update/update.go"],
      postPass: (detail) =>
        detail ===
        "2 first-party packages, 1 allowed network package: internal/update -> api.github.com, 0 confidential markers, 0 boundary violations"
          ? null
          : `local-only: accepted detail ${JSON.stringify(detail)} does not name the allowed network package`,
    },
    {
      gate: localOnly,
      label: "the desktop importing the update package",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/desktop/desktop.go":
          'package desktop\n\nimport "example.com/selftest/internal/update"\n\nvar _ = update.Endpoint\n',
        "internal/update/update.go": UPDATE_FIXTURE,
      },
      tracked: ["internal/update/update.go"],
      postPass: (detail) =>
        detail ===
        "3 first-party packages, 1 allowed network package: internal/update -> api.github.com, 0 confidential markers, 0 boundary violations"
          ? null
          : `local-only: accepted detail ${JSON.stringify(detail)} does not admit the desktop importer`,
    },
    {
      gate: localOnly,
      label: "desktop main that links net/http only through a dependency",
      files: {
        "go.mod": 'module example.com/selftest\n\ngo 1.27.3\n\nrequire example.com/dep v0.0.0\n\nreplace example.com/dep => ./dep\n',
        "dep/go.mod": "module example.com/dep\n\ngo 1.27.3\n",
        "dep/client.go": 'package dep\n\nimport "net/http"\n\nfunc Client() *http.Client { return &http.Client{} }\n',
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "cmd/passquest-desktop/main.go": 'package main\n\nimport "example.com/dep"\n\nfunc main() { _ = dep.Client() }\n',
        "internal/x/x.go": "package x\n",
      },
      tracked: [],
      postPass: (detail) =>
        detail.startsWith("3 first-party packages")
          ? null
          : `local-only: accepted detail ${JSON.stringify(detail)} does not count the desktop main`,
    },
    {
      gate: localOnly,
      label: "workspace test importing the CLI adapter",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/cli/cli.go": "package cli\n\nconst X = 1\n",
        "internal/workspace/workspace.go": "package workspace\n",
        "internal/workspace/workspace_test.go":
          'package workspace\n\nimport "example.com/selftest/internal/cli"\n\nvar _ = cli.X\n',
      },
      tracked: [],
      postPass: (detail) =>
        detail === "3 first-party packages, 0 network imports, 0 confidential markers, 0 boundary violations"
          ? null
          : `local-only: accepted detail ${JSON.stringify(detail)} is not the boundary-aware pass detail`,
    },
    {
      gate: localOnly,
      label: "nested package with no http(s) literals inside the allowed package",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        "internal/update/update.go": UPDATE_FIXTURE,
        "internal/update/internal/pin/pin.go": 'package pin\n\nconst Name = "pin"\n',
      },
      tracked: ["internal/update/update.go", "internal/update/internal/pin/pin.go"],
      postPass: (detail) =>
        detail ===
        "3 first-party packages, 1 allowed network package: internal/update -> api.github.com, 0 confidential markers, 0 boundary violations"
          ? null
          : `local-only: accepted detail ${JSON.stringify(detail)} fails on the nested package's plain literal`,
    },
  ];
}
