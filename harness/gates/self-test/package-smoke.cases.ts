/**
 * The package smoke gate's deliberately broken inputs. Both drive the gate
 * over a fixture tree that stands in for an installed package (no installer,
 * no mygo build): one whose data lands inside the install directory — the
 * exact thing the gate exists to catch — and one whose bundled Node runtime
 * is missing, which the layout case must name before any launch.
 *
 * The content scan gets three more, driven over the same fixture tree with
 * `only: contents` (no package, no launch): one payload that seeds a PDF
 * under another name, a SQLite header, a `data/private` path, a `.env`, the
 * internal-only marker and a credential-shaped string in a bundled host —
 * the failure must name every seed and stay silent on the same credential in
 * a `node_modules` file — one synthetic baseline whose byte-identical copy
 * and embedded line in the payload must both be reported, and one accepted
 * tree whose only secret-shaped string sits in `node_modules`.
 */
import { BASELINE } from "../../lib/baseline.ts";
import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";

import { makePackageSmokeGate } from "../package-smoke.ts";
import { packageNaming } from "../../lib/package-build.ts";
import { executable, run } from "../../lib/process.ts";
import { GO_MOD } from "./fixture.ts";
import type { BrokenCase, PassCase } from "./fixture.ts";

/** The four host files a package-shaped fixture tree carries. */
const FIXTURE_HOSTS: Record<string, string> = {
  "build/fixture/agent/tutor-host.mjs": "// fixture\n",
  "build/fixture/agent/anki-host.mjs": "// fixture\n",
  "build/fixture/agent/search-host.mjs": "// fixture\n",
  "build/fixture/agent/durable-host.mjs": "// fixture\n",
};

/** The first bytes of a native executable on this platform, so the layout check reads a real one. */
function runtimeMagic(): number[] {
  if (process.platform === "win32") return [0x4d, 0x5a];
  if (process.platform === "darwin") return [0xcf, 0xfa, 0xed, 0xfe];
  return [0x7f, 0x45, 0x4c, 0x46];
}

/** The runtime path inside a fixture tree, as the gate computes it for this platform. */
function fixtureRuntime(root: string): string {
  return process.platform === "win32"
    ? path.join(root, "build", "fixture", "node", "node.exe")
    : path.join(root, "build", "fixture", "node", "bin", "node");
}

/** The fixture app's binary, as the gate looks it up in a fixture tree. */
function fixtureBinary(root: string): string {
  return path.join(root, "build", "fixture", executable("passquest-desktop"));
}

const FIXTURE_FILES: Record<string, string> = {
  "go.mod": GO_MOD,
  "mygo.json": '{"name": "Pass Quest", "version": "0.0.0"}\n',
  // The desktop surface the gate looks for; the study case replaces it with a real fixture app.
  "cmd/passquest-desktop/main.go": "package main\n\nfunc main() {}\n",
  "desktop/web/src/main.ts": 'document.title = "selftest";\n',
  ...FIXTURE_HOSTS,
};

/** A credential-shaped string built at run time, the way the secrets cases build one. */
function fakeKey(): string {
  return `sk-${"A".repeat(24)}`;
}

/** A failure line with Windows separators normalised: gates print absolute platform paths. */
const slashed = (line: string): string => line.replace(/\\/g, "/");

/** An independently authored stand-in for a baseline page: the scan reads whatever baseline the tree carries. */
const PAGE_LINE = "A synthetic page line long enough to stand for the baseline text.";
const PAGE_TEXT = `${PAGE_LINE}\nAnd a second line that the copied page carries whole.\n`;

/**
 * The package's naming must mirror MyGo's own. The default identifier is
 * `com.mygo.` plus the name's letters and digits, lower-cased (`app` when
 * nothing is left) — cmd/mygo/config.go applyDefaults — and not the slug: it
 * is the uninstall registry key's last element, so a slug-based one would
 * name a different app's key. The Windows shortcut name is fsName of
 * windows.displayName, else of the name (cmd/mygo/config.go shortcutName);
 * the install refusal looks for that file in both shell folders.
 */
export function packageNamingProblem(): string | null {
  const fixture = packageNaming('{"name": "Pass Quest", "version": "0.0.0"}');
  if (fixture.identifier !== "com.mygo.passquest") {
    return `package naming: the fixture mygo.json (no identifier) derives ${fixture.identifier}, want com.mygo.passquest (cmd/mygo/config.go applyDefaults)`;
  }
  if (fixture.slug !== "pass-quest") return `package naming: the fixture mygo.json derives the slug ${fixture.slug}, want pass-quest`;
  const empty = packageNaming('{"name": "!!!", "version": "0.0.0"}');
  if (empty.identifier !== "com.mygo.app") return `package naming: a name with no letters or digits derives ${empty.identifier}, want com.mygo.app`;
  const given = packageNaming('{"name": "Pass Quest", "version": "0.0.0", "identifier": "com.github.wxxb789.passquest"}');
  if (given.identifier !== "com.github.wxxb789.passquest") return `package naming: the explicit identifier became ${given.identifier}`;
  const display = packageNaming('{"name": "Pass Quest", "version": "0.0.0", "windows": {"displayName": "Pass Quest: Dev?"}}');
  if (display.shortcut !== "Pass Quest- Dev-") {
    return `package naming: windows.displayName derives the shortcut ${JSON.stringify(display.shortcut)}, want "Pass Quest- Dev-" (cmd/mygo/config.go shortcutName and fsName)`;
  }
  return null;
}

export function packageSmokeCases(): BrokenCase[] {
  return [
    {
      gate: makePackageSmokeGate({ installedTree: "build/fixture", only: ["study"] }),
      label: "installed app whose data lands inside the install directory",
      files: {
        ...FIXTURE_FILES,
        // The gate builds the repository CLI before the study case runs; the
        // data-location check fails before any CLI call, so a stub is enough.
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
        // Boots a Today-shaped window, but keeps its database in its own directory.
        "cmd/passquest-desktop/main.go": `package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func main() {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Join(filepath.Dir(exe), "Pass Quest")
		if os.MkdirAll(dir, 0o700) == nil {
			_ = os.WriteFile(filepath.Join(dir, "passquest.sqlite"), []byte("fixture"), 0o600)
		}
	}
	document, _ := json.Marshal(map[string]any{"ok": true, "value": map[string]any{
		"textLength": 900, "stageChildren": 3, "stageTextLength": 800, "errorBars": 0, "ready": true,
	}})
	_ = os.WriteFile(os.Getenv("PASSQUEST_DESKTOP_RESULT"), document, 0o600)
}
`,
      },
      prepare: async (root) => {
        const runtime = fixtureRuntime(root);
        mkdirSync(path.dirname(runtime), { recursive: true });
        writeFileSync(runtime, Buffer.from([...runtimeMagic(), 0, 0, 0, 0]));
        const binary = fixtureBinary(root);
        mkdirSync(path.dirname(binary), { recursive: true });
        const build = await run(executable("go"), ["build", "-o", binary, "./cmd/passquest-desktop"], { cwd: root });
        return build.code === 0 ? null : `the fixture app did not build: ${build.stderr || build.stdout}`;
      },
      postFail: (failures) =>
        failures.some((line) => line.startsWith("study: the first launch wrote its database inside the install directory:"))
          ? null
          : `package-smoke: did not report data inside the install directory: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      gate: makePackageSmokeGate({ installedTree: "build/fixture", only: ["layout"] }),
      label: "installed tree whose bundled Node runtime is missing",
      files: FIXTURE_FILES,
      postFail: (failures) =>
        failures.some((line) => line.startsWith("layout: the installed tree is missing") && line.includes("node"))
          ? null
          : `package-smoke: did not report the missing runtime: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      gate: makePackageSmokeGate({ installedTree: "build/fixture", only: ["contents"] }),
      label: "installed payload seeded with forbidden content",
      files: {
        ...FIXTURE_FILES,
        "build/fixture/vendor/handbook.txt": "%PDF-1.7\n1 0 obj\n",
        "build/fixture/vendor/cache.dat": "SQLite format 3\u0000fixture bytes\n",
        "build/fixture/data/private/x.txt": "private source\n",
        "build/fixture/.env": "TOKEN=fixture\n",
        // The internal-only marker, assembled at run time so this file holds no full marker text.
        "build/fixture/vendor/marked.bin": `${["仅限", "内部", "使用"].join("")}\n`,
        "build/fixture/agent/tutor-host.mjs": `// fixture\nconst key = "${fakeKey()}";\n`,
        "build/fixture/agent/node_modules/vendor/index.mjs": `const key = "${fakeKey()}";\n`,
      },
      postFail: (failures) => {
        const seeds: Array<[string, string]> = [
          ["vendor/handbook.txt", "PDF"],
          ["vendor/cache.dat", "SQLite"],
          ["data/private/x.txt", "private path"],
          [".env", "environment file"],
          ["vendor/marked.bin", "internal-only source marker"],
          ["agent/tutor-host.mjs", "credential-shaped string"],
        ];
        const missing = seeds.filter(([name, reason]) => !failures.some((line) => slashed(line).includes(name) && line.includes(reason)));
        if (missing.length > 0) {
          return `package-smoke: the content scan did not name ${missing.map(([name]) => name).join(", ")}: ${failures.slice(0, 6).join("; ")}`;
        }
        if (failures.some((line) => slashed(line).includes("node_modules/vendor"))) {
          return "package-smoke: the content scan reported the credential in a third-party file";
        }
        return null;
      },
    },
    {
      gate: makePackageSmokeGate({ installedTree: "build/fixture", only: ["contents"] }),
      label: "installed payload holding a baseline page",
      files: {
        ...FIXTURE_FILES,
        [`${BASELINE}/pages/page-001.txt`]: PAGE_TEXT,
        "build/fixture/vendor/copied-page.txt": PAGE_TEXT,
        "build/fixture/vendor/report.txt": `notes\n${PAGE_LINE}\n`,
      },
      postFail: (failures) => {
        for (const name of ["vendor/copied-page.txt", "vendor/report.txt"]) {
          if (!failures.some((line) => slashed(line).includes(name) && line.includes("baseline text"))) {
            return `package-smoke: the content scan did not report ${name} as baseline text: ${failures.slice(0, 6).join("; ")}`;
          }
        }
        return null;
      },
    },
  ];
}

export function packageSmokePassCases(): PassCase[] {
  return [
    {
      gate: makePackageSmokeGate({ installedTree: "build/fixture", only: ["contents"] }),
      label: "third-party file with a credential-shaped string",
      files: {
        ...FIXTURE_FILES,
        "build/fixture/agent/node_modules/@vendor/sdk/index.mjs": `const key = "${fakeKey()}";\n`,
      },
      postPass: (detail) =>
        detail.includes("contents scan:")
          ? null
          : `package-smoke: the content scan completed without a scan line (${detail})`,
    },
  ];
}
