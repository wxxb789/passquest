import { portablePaths } from "../portable-paths.ts";
import { GO_MOD } from "./fixture.ts";
import type { BrokenCase } from "./fixture.ts";

export function portablePathsCases(): BrokenCase[] {
  return [
    {
      gate: portablePaths,
      label: "an unjustified literal beside a justified one",
      files: {
        "go.mod": GO_MOD,
        "internal/x/x.go": "package x\n",
        "internal/x/x_test.go":
          'package x\n\nimport "testing"\n\nfunc TestA(t *testing.T) {\n\topen("/tmp/fixture.sqlite")\n\tif "C:\\\\Users\\\\x" != "C:\\\\Users\\\\x" { // portable-paths: allow compared as string data, never resolved\n\t\tt.Fatal("unreachable")\n\t}\n\topen("/tmp/adjacent.sqlite")\n}\n',
      },
      tracked: ["internal/x/x_test.go"],
      postFail: (failures) => {
        if (failures.length !== 2) {
          return `portable-paths: expected both unjustified literals, saw ${failures.length}: ${failures.join("; ")}`;
        }
        if (!failures.some((line) => line.includes("x_test.go:6") && line.includes("/tmp/fixture.sqlite"))) {
          return `portable-paths: did not name the first unjustified literal: ${failures.join("; ")}`;
        }
        if (!failures.some((line) => line.includes("/tmp/adjacent.sqlite"))) {
          return "portable-paths: the marker on the previous line silenced an adjacent unjustified path";
        }
        if (failures.some((line) => line.includes("C:\\\\Users"))) {
          return "portable-paths: reported the literal its own marker justified";
        }
        return null;
      },
    },
    {
      gate: portablePaths,
      label: "pointer assignment and a block comment",
      files: {
        "go.mod": GO_MOD,
        "internal/x/x.go": "package x\n",
        "internal/x/x_test.go":
          'package x\n\nimport "testing"\n\nfunc TestPointer(t *testing.T) {\n\tvar p *string\n\t*p = "/tmp/pointer.sqlite"\n\t_ = p\n}\n',
        "internal/x/doc_test.go":
          'package x\n\n/*\nA fixture usually looks like "/tmp/documentation.sqlite".\n*/\nfunc TestDoc(t *testing.T) {}\n',
      },
      tracked: ["internal/x/x_test.go", "internal/x/doc_test.go"],
      postFail: (failures) => {
        if (failures.length !== 1) {
          return `portable-paths: expected only the pointer assignment, saw ${failures.length}: ${failures.join("; ")}`;
        }
        if (!failures[0].includes("x_test.go:7") || !failures[0].includes("/tmp/pointer.sqlite")) {
          return `portable-paths: did not report the pointer assignment: ${failures[0]}`;
        }
        if (failures.some((line) => line.includes("documentation.sqlite"))) {
          return "portable-paths: treated a path inside a block comment as code";
        }
        return null;
      },
    },
    {
      gate: portablePaths,
      label: "comment syntax inside a multiline raw string",
      files: {
        "go.mod": GO_MOD,
        "internal/x/x.go": "package x\n",
        "internal/x/x_test.go":
          'package x\n\nimport "testing"\n\nconst example = `\nselect /*\nfrom t\n`\n\nfunc TestAfterRaw(t *testing.T) {\n\topen("/tmp/after-raw.sqlite")\n}\n',
      },
      tracked: ["internal/x/x_test.go"],
      postFail: (failures) => {
        if (failures.length !== 1 || !failures[0].includes("/tmp/after-raw.sqlite")) {
          return `portable-paths: a raw string opened a block comment and hid the next path: ${failures.join("; ") || "no failure"}`;
        }
        return null;
      },
    },
    {
      gate: portablePaths,
      label: "a backtick rune before an unmarked path",
      files: {
        "go.mod": GO_MOD,
        "internal/x/x.go": "package x\n",
        "internal/x/x_test.go":
          "package x\n\nimport \"testing\"\n\nconst delimiter = '`'\nconst quote = '\"'\n\nfunc TestRune(t *testing.T) {\n\topen(\"/tmp/after-rune.sqlite\")\n\t_, _ = delimiter, quote\n}\n",
      },
      tracked: ["internal/x/x_test.go"],
      postFail: (failures) => {
        if (failures.length !== 1 || !failures[0].includes("/tmp/after-rune.sqlite")) {
          return `portable-paths: a rune holding a quote hid the next path: ${failures.join("; ") || "no failure"}`;
        }
        return null;
      },
    },
    {
      gate: portablePaths,
      label: "raw-string path literals",
      files: {
        "go.mod": GO_MOD,
        "internal/x/x.go": "package x\n",
        "internal/x/x_test.go":
          "package x\n\nimport \"testing\"\n\nfunc TestRaw(t *testing.T) {\n\tposix := `/tmp/raw.sqlite`\n\twin := `C:\\Users\\runner\\agent.sqlite`\n\t_ = posix\n\t_ = win\n}\n",
      },
      tracked: ["internal/x/x_test.go"],
      postFail: (failures) => {
        if (failures.length !== 2) {
          return `portable-paths: expected both raw-string literals, saw ${failures.length}: ${failures.join("; ")}`;
        }
        if (!failures.some((line) => line.includes("/tmp/raw.sqlite"))) {
          return "portable-paths: did not report the raw-string POSIX path";
        }
        if (!failures.some((line) => line.includes("C:\\\\Users\\\\runner\\\\agent.sqlite"))) {
          return "portable-paths: did not report the raw-string Windows path";
        }
        return null;
      },
    },
    {
      gate: portablePaths,
      label: "interpreted Windows paths that start with t, n or r",
      files: {
        "go.mod": GO_MOD,
        "internal/x/x.go": "package x\n",
        "internal/x/x_test.go":
          'package x\n\nimport "testing"\n\nfunc TestEscapes(t *testing.T) {\n\t_t := "C:\\\\tmp\\\\fixture.sqlite"\n\t_n := "C:\\\\new\\\\fixture.sqlite"\n\t_r := "C:\\\\repo\\\\fixture.sqlite"\n\t_, _, _ = _t, _n, _r\n}\n',
      },
      tracked: ["internal/x/x_test.go"],
      postFail: (failures) => {
        if (failures.length !== 3) {
          return `portable-paths: expected all three interpreted paths, saw ${failures.length}: ${failures.join("; ")}`;
        }
        for (const segment of ["\\tmp\\", "\\new\\", "\\repo\\"]) {
          if (!failures.some((line) => line.includes(segment))) {
            return `portable-paths: did not decode ${segment} in an interpreted literal`;
          }
        }
        return null;
      },
    },
  ];
}
