import { localOnly } from "../local-only.ts";
import { GO_MOD } from "./fixture.ts";
import type { BrokenCase, PassCase } from "./fixture.ts";

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
        "internal/update/update.go": 'package update\n\nimport "net/http"\n\nvar _ = http.MethodGet\n',
      },
      tracked: [],
      postPass: (detail) =>
        detail === "2 first-party packages, 1 allowed network package: internal/update, 0 confidential markers, 0 boundary violations"
          ? null
          : `local-only: accepted detail ${JSON.stringify(detail)} does not name the allowed network package`,
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
  ];
}
