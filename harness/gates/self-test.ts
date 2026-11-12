import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { BASELINE, EXPECTED_NATIVE, EXPECTED_PAGES, nonSpaceChars } from "../lib/baseline.ts";
import { executable, run } from "../lib/process.ts";
import { context, resolve, workingTreeFiles } from "../lib/repo.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";

import { adaptiveSmoke } from "./adaptive-smoke.ts";
import { agentSmoke } from "./agent-smoke.ts";
import { assessSmoke } from "./assess-smoke.ts";
import { baseline } from "./baseline.ts";
import { docsLinks } from "./docs-links.ts";
import { findSmoke } from "./find-smoke.ts";
import { goBuild } from "./go-build.ts";
import { goTest } from "./go-test.ts";
import { goTidy } from "./go-tidy.ts";
import { goVet } from "./go-vet.ts";
import { gofmt } from "./gofmt.ts";
import { importSmoke } from "./import-smoke.ts";
import { localOnly } from "./local-only.ts";
import { noteSmoke } from "./note-smoke.ts";
import { portablePaths } from "./portable-paths.ts";
import { secrets } from "./secrets.ts";
import { tacticSmoke } from "./tactic-smoke.ts";
import { tutorSmoke } from "./tutor-smoke.ts";
import { typecheck } from "./typecheck.ts";

const GO_MOD = "module example.com/selftest\n\ngo 1.27.3\n";

interface BrokenCase {
  gate: Gate;
  label: string;
  files: Record<string, string>;
  tracked?: string[];
  postFail?: (failures: string[]) => string | null;
}

interface BaselineFixtureOptions {
  wrongCharsPage?: number;
  /** Writes U+FEFF into this page; the manifest uses the pre-fix \s count. */
  bomPage?: number;
  /** Renames one page file and manifest row, leaving a gap at `from`. */
  renumber?: { from: number; to: number };
}

/** A complete baseline tree; options break exactly one documented property. */
function baselineFixture(options: BaselineFixtureOptions = {}): { files: Record<string, string>; tracked: string[] } {
  const files: Record<string, string> = {};
  const tracked: string[] = [];
  const rows = ["page\tsection\tsource\tbytes\tchars_no_ws"];
  for (let page = 1; page <= EXPECTED_PAGES; page++) {
    const text = page === options.bomPage ? `page ${page} text\uFEFF\n` : `page ${page} text\n`;
    const number = options.renumber && page === options.renumber.from ? options.renumber.to : page;
    const relative = `${BASELINE}/pages/page-${String(number).padStart(3, "0")}.txt`;
    files[relative] = text;
    tracked.push(relative);
    const source = page <= EXPECTED_NATIVE ? "native" : "ocr";
    let chars = nonSpaceChars(text);
    if (page === options.wrongCharsPage) chars += 1;
    if (page === options.bomPage) chars -= 1;
    rows.push([number, `section-${page}`, source, Buffer.byteLength(text), chars].join("\t"));
  }
  files[`${BASELINE}/manifest.tsv`] = `${rows.join("\n")}\n`;
  files[`${BASELINE}/README.md`] = "The extracted text is unreviewed.\n";
  tracked.push(`${BASELINE}/manifest.tsv`, `${BASELINE}/README.md`);
  return { files, tracked };
}

async function workingTreeDiscoveryProblem(): Promise<string | null> {
  const root = mkdtempSync(path.join(tmpdir(), "passquest-repo-selftest-"));
  try {
    const init = await run(executable("git"), ["init", "-q"], { cwd: root });
    if (init.code !== 0) return `working-tree discovery: git init failed: ${init.stderr || init.stdout}`;
    writeFileSync(resolve(root, ".gitignore"), "ignored.txt\n");
    writeFileSync(resolve(root, "tracked.txt"), "tracked\n");
    writeFileSync(resolve(root, "untracked.txt"), "untracked\n");
    writeFileSync(resolve(root, "ignored.txt"), "ignored\n");
    const add = await run(executable("git"), ["add", "tracked.txt"], { cwd: root });
    if (add.code !== 0) return `working-tree discovery: git add failed: ${add.stderr || add.stdout}`;
    const files = (await workingTreeFiles(root)).sort();
    const expected = [".gitignore", "tracked.txt", "untracked.txt"];
    if (files.join(",") !== expected.join(",")) {
      return `working-tree discovery = [${files.join(", ")}], want [${expected.join(", ")}]`;
    }
    return null;
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

function brokenCases(): BrokenCase[] {
  const fakeKey = `sk-${"A".repeat(24)}`;
  const completeBaseline = baselineFixture();
  const wrongCount = baselineFixture({ wrongCharsPage: 7 });
  const bomCount = baselineFixture({ bomPage: 7 });
  const pageGap = baselineFixture({ renumber: { from: EXPECTED_PAGES, to: EXPECTED_PAGES + 1 } });
  return [
    {
      gate: typecheck,
      label: "TypeScript type error",
      files: {
        "tsconfig.json": JSON.stringify({
          compilerOptions: {
            target: "ES2022",
            module: "ESNext",
            moduleResolution: "bundler",
            strict: true,
            noEmit: true,
            skipLibCheck: true,
            types: [],
          },
          include: ["bad.ts"],
        }),
        "bad.ts": 'const value: number = "text";\n',
      },
      tracked: ["bad.ts", "tsconfig.json"],
    },
    {
      gate: gofmt,
      label: "unformatted Go file",
      files: { "bad.go": "package bad\n\nfunc  F( ) { }\n" },
      tracked: ["bad.go"],
    },
    {
      gate: docsLinks,
      label: "broken relative link",
      files: { "README.md": "[missing](does-not-exist.md)\n" },
      tracked: ["README.md"],
    },
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
    {
      gate: baseline,
      label: "baseline manifest disagrees with its page",
      files: wrongCount.files,
      tracked: wrongCount.tracked,
      postFail: (failures) =>
        failures.length === 1 && failures[0].includes("page-007.txt") && failures[0].includes("non-space chars")
          ? null
          : `baseline: expected one wrong char count on page-007.txt, saw ${failures.length}: ${failures.join("; ")}`,
    },
    {
      gate: baseline,
      label: "baseline count treats U+FEFF as whitespace",
      files: bomCount.files,
      tracked: bomCount.tracked,
      postFail: (failures) =>
        failures.length === 1 && failures[0].includes("page-007.txt") && failures[0].includes("non-space chars")
          ? null
          : `baseline: expected one wrong char count on page-007.txt, saw ${failures.length}: ${failures.join("; ")}`,
    },
    {
      gate: baseline,
      label: "baseline page set has a gap",
      files: pageGap.files,
      tracked: pageGap.tracked,
      postFail: (failures) =>
        failures.some((line) => line.includes(`manifest page ${EXPECTED_PAGES} is missing`))
          ? null
          : `baseline: expected a missing manifest page, saw: ${failures.join("; ")}`,
    },
    {
      gate: baseline,
      label: "test fixture references the baseline",
      files: {
        ...completeBaseline.files,
        "internal/fixture_test.go": 'package internal\n\nconst baselinePath = "data/baselines/2026-crsc-english"\n',
      },
      tracked: [...completeBaseline.tracked, "internal/fixture_test.go"],
      postFail: (failures) =>
        failures.length === 1 &&
        failures[0].includes("internal/fixture_test.go") &&
        failures[0].includes("references the unreviewed baseline")
          ? null
          : `baseline: expected one fixture-reference failure, saw ${failures.length}: ${failures.join("; ")}`,
    },
    {
      gate: goTest,
      label: "failing Go test",
      files: {
        "go.mod": GO_MOD,
        "x_test.go": 'package x\n\nimport "testing"\n\nfunc TestFail(t *testing.T) { t.Fatal("boom") }\n',
      },
      postFail: (failures) => {
        if (!failures.some((line) => line.includes("TestFail: failed"))) {
          return `go-test: did not report the failing test, saw: ${failures.join("; ")}`;
        }
        if (!failures.some((line) => line.includes("boom"))) {
          return "go-test: dropped the test's failure message";
        }
        return null;
      },
    },
    {
      gate: goTest,
      label: "skipped Go test",
      files: {
        "go.mod": GO_MOD,
        "x_test.go": 'package x\n\nimport "testing"\n\nfunc TestSkipped(t *testing.T) { t.Skip("deliberately skipped") }\n',
      },
      postFail: (failures) =>
        failures.some((line) => line.includes("TestSkipped")) ? null : `go-test: did not report the skipped test, saw: ${failures.join("; ")}`,
    },
    {
      gate: goVet,
      label: "vet printf diagnostic",
      files: {
        "go.mod": GO_MOD,
        "main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Printf("%d", "x") }\n',
      },
    },
    {
      gate: goBuild,
      label: "compile error",
      files: {
        "go.mod": GO_MOD,
        "main.go": "package main\n\nfunc main() { missing() }\n",
      },
    },
    {
      gate: goTidy,
      label: "stale module requirement",
      files: {
        "go.mod": 'module example.com/selftest\n\ngo 1.27.3\n\nrequire example.com/dep v0.0.0\n\nreplace example.com/dep => ./dep\n',
        "dep/go.mod": "module example.com/dep\n\ngo 1.27.3\n",
        "main.go": "package main\n\nfunc main() {}\n",
      },
    },
    {
      gate: importSmoke,
      label: "CLI whose import output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
        "internal/cli/testdata/import/placeholder.txt": "fixture placeholder\n",
      },
    },
    {
      gate: tacticSmoke,
      label: "CLI whose tactic output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
      },
    },
    {
      gate: adaptiveSmoke,
      label: "CLI whose skill and next output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
      },
    },
    {
      gate: tutorSmoke,
      label: "CLI whose tutor output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
      },
    },
    {
      gate: agentSmoke,
      label: "CLI whose agent output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
      },
    },
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
      gate: findSmoke,
      label: "CLI whose find output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
      },
    },
    {
      gate: assessSmoke,
      label: "CLI whose assess output is not the protocol",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("{}") }\n',
      },
    },
  ];
}

async function runCase(testCase: BrokenCase): Promise<string | null> {
  const root = mkdtempSync(path.join(tmpdir(), "passquest-selftest-"));
  try {
    for (const [file, content] of Object.entries(testCase.files)) {
      const absolute = resolve(root, file);
      mkdirSync(path.dirname(absolute), { recursive: true });
      writeFileSync(absolute, content);
    }
    const ctx: GateContext = context(root, testCase.tracked);
    const result = await testCase.gate.run(ctx);
    if (result.status === "fail") {
      const problem = testCase.postFail?.(result.failures ?? []) ?? null;
      return problem ? `${testCase.gate.id}: ${problem} on ${testCase.label}` : null;
    }
    const reason = result.status === "skip" ? `skip (${result.skipReason ?? "no reason"})` : `status ${result.status}`;
    return `${testCase.gate.id}: ${reason} on ${testCase.label}`;
  } catch (error) {
    return `${testCase.gate.id}: threw on ${testCase.label}: ${error instanceof Error ? error.message : String(error)}`;
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

export const selfTest: Gate = {
  id: "harness-self-test",
  title: "harness self-test",
  async run() {
    // Static import cannot work: index.ts builds `gates` from `selfTest` at module
    // evaluation, so the cycle would read `selfTest` before initialization when
    // this module is the entry point. The list is only needed here, at run time.
    const { gates } = await import("./index.ts");
    const cases = brokenCases();
    const problems: string[] = [];
    const discovery = await workingTreeDiscoveryProblem();
    if (discovery) problems.push(discovery);
    for (const testCase of cases) {
      const problem = await runCase(testCase);
      if (problem) problems.push(problem);
    }
    const covered = new Set(cases.map((testCase) => testCase.gate.id));
    const expected = gates.filter((gate) => gate.id !== selfTest.id);
    for (const gate of expected) {
      if (!covered.has(gate.id)) problems.push(`${gate.id}: no self-test case`);
    }
    if (problems.length > 0) return fail(problems);
    return pass(`${cases.length} cases, ${covered.size}/${expected.length} gates`);
  },
};
