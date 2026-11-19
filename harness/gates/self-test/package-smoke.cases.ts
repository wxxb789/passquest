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
import { Database } from "bun:sqlite";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { makePackageSmokeGate } from "../package-smoke.ts";
import { compareDatabases } from "../package-smoke/upgrade.ts";
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

/**
 * The upgrade comparison (P7) over two small fixture databases, no build and
 * no launch: identical content must pass and count its tables, rows and
 * fields; one changed field must fail and name the table, the row key and the
 * column; a new column, a new table and a new row are allowed.
 */
export function upgradeComparisonProblem(): string | null {
  const dir = mkdtempSync(path.join(tmpdir(), "passquest-upgrade-compare-"));
  const family = (name: string) => path.join(dir, `${name}.sqlite`);
  const write = (file: string, options: { changedFront?: string; newer?: boolean } = {}): void => {
    const db = new Database(file);
    db.run(`CREATE TABLE notes (note_id TEXT PRIMARY KEY, revision INTEGER NOT NULL, front TEXT NOT NULL)`);
    db.run(`CREATE TABLE note_outbox (note_id TEXT NOT NULL, revision INTEGER NOT NULL, status TEXT NOT NULL, PRIMARY KEY (note_id, revision))`);
    db.run(`INSERT INTO notes (note_id, revision, front) VALUES ('pq-1', 1, '${options.changedFront ?? "closes at 4 p.m."}')`);
    db.run(`INSERT INTO notes (note_id, revision, front) VALUES ('pq-2', 1, 'opens at 9 a.m.')`);
    db.run(`INSERT INTO note_outbox (note_id, revision, status) VALUES ('pq-1', 1, 'drained')`);
    if (options.newer) {
      db.run(`ALTER TABLE notes ADD COLUMN anki_state TEXT NOT NULL DEFAULT 'local'`);
      db.run(`CREATE TABLE learner_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`);
      db.run(`INSERT INTO notes (note_id, revision, front) VALUES ('pq-3', 1, 'accepted after the upgrade')`);
    }
    db.close();
  };
  try {
    const before = family("before");
    const same = family("same");
    const changed = family("changed");
    const newer = family("newer");
    write(before);
    write(same);
    write(changed, { changedFront: "closes at 6 p.m." });
    write(newer, { newer: true });
    const clean = compareDatabases(before, same);
    if (clean.problems.length > 0) return `the upgrade comparison reports ${clean.problems.join("; ")} for identical databases`;
    if (clean.tables.length !== 2 || clean.rows !== 3 || clean.fields !== 9) {
      return `the upgrade comparison counted ${clean.tables.length} table(s), ${clean.rows} row(s) and ${clean.fields} field(s), want 2, 3 and 9`;
    }
    const altered = compareDatabases(before, changed);
    const problem = altered.problems.join("; ");
    for (const want of ["notes", "note_id=s:pq-1", "column front"]) {
      if (!problem.includes(want)) {
        return `the change to one field is not reported as ${want}: ${problem === "" ? "no problem" : problem}`;
      }
    }
    const allowed = compareDatabases(before, newer);
    if (allowed.problems.length > 0) return `a new column, table and row are reported as differences: ${allowed.problems.join("; ")}`;
    if (allowed.rows !== 3 || allowed.fields !== 9) {
      return `the comparison over the newer fixture counted ${allowed.rows} row(s) and ${allowed.fields} field(s), want the old schema's 3 and 9`;
    }
    return null;
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
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
