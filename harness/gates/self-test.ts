import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { context, resolve } from "../lib/repo.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";

import { baseline } from "./baseline.ts";
import { docsLinks } from "./docs-links.ts";
import { goBuild } from "./go-build.ts";
import { goTest } from "./go-test.ts";
import { goTidy } from "./go-tidy.ts";
import { goVet } from "./go-vet.ts";
import { gofmt } from "./gofmt.ts";
import { importSmoke } from "./import-smoke.ts";
import { localOnly } from "./local-only.ts";
import { secrets } from "./secrets.ts";
import { typecheck } from "./typecheck.ts";

const GO_MOD = "module example.com/selftest\n\ngo 1.27.3\n";

interface BrokenCase {
  gate: Gate;
  label: string;
  files: Record<string, string>;
  tracked?: string[];
}

function brokenCases(): BrokenCase[] {
  const fakeKey = `sk-${"A".repeat(24)}`;
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
    },
    {
      gate: localOnly,
      label: "first-party net/http import",
      files: {
        "go.mod": GO_MOD,
        "cmd/passquest/main.go": 'package main\n\nimport (\n\t"fmt"\n\t"net/http"\n)\n\nfunc main() { fmt.Println(http.MethodGet) }\n',
        "internal/x/x.go": "package x\n",
      },
      tracked: [],
    },
    {
      gate: baseline,
      label: "baseline manifest disagrees with its page",
      files: {
        "data/baselines/2026-crsc-english/manifest.tsv": "page\tsection\tsource\tbytes\tchars_no_ws\n1\tx\tnative\t1\t1\n",
        "data/baselines/2026-crsc-english/pages/page-001.txt": "four bytes\n",
        "data/baselines/2026-crsc-english/README.md": "No review statement here.\n",
      },
      tracked: [],
    },
    {
      gate: goTest,
      label: "failing Go test",
      files: {
        "go.mod": GO_MOD,
        "x_test.go": 'package x\n\nimport "testing"\n\nfunc TestFail(t *testing.T) { t.Fatal("boom") }\n',
      },
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
    if (result.status === "fail") return null;
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
    const cases = brokenCases();
    const problems: string[] = [];
    for (const testCase of cases) {
      const problem = await runCase(testCase);
      if (problem) problems.push(problem);
    }
    if (problems.length > 0) return fail(problems);
    return pass(`${cases.length}/${cases.length} gates failed on deliberately broken input`);
  },
};
