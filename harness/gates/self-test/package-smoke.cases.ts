/**
 * The package smoke gate's deliberately broken inputs. Both drive the gate
 * over a fixture tree that stands in for an installed package (no installer,
 * no mygo build): one whose data lands inside the install directory — the
 * exact thing the gate exists to catch — and one whose bundled Node runtime
 * is missing, which the layout case must name before any launch.
 *
 * The content scan gets three more, driven over the same fixture tree with
 * `only: contents` (no package, no launch): one payload that seeds a PDF
 * under another name, a SQLite header, every private-path and `.env` spelling
 * (mixed case included) plus links whose own name and target name private
 * data or an environment file, the internal-only marker and a
 * credential-shaped string in a bundled host — the failure must name every
 * seed and stay silent on the same credential in a `node_modules` file — one
 * synthetic baseline pair (a page file plus the partial extraction) whose
 * byte-identical copies, embedded lines and encoded forms — JSON-escaped,
 * `\uXXXX`-escaped, UTF-16LE — must all be reported while an unrelated line
 * is not, and one accepted tree whose only secret-shaped string sits in
 * `node_modules`.
 */
import { BASELINE, BASELINE_PARTIAL } from "../../lib/baseline.ts";
import { Database } from "bun:sqlite";
import { mkdirSync, mkdtempSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { makePackageSmokeGate } from "../package-smoke.ts";
import { compareDatabases, rowCountProblems } from "../package-smoke/upgrade.ts";
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

/**
 * The link seeds the last prepared fixture created. Windows refuses a
 * symlink to a process without the symlink privilege (EPERM); that case then
 * skips exactly those seeds, says so in its output, and still runs every
 * other seed.
 */
let linkSeeds: string[] = [];

/**
 * An independently authored stand-in for a baseline page: the scan reads
 * whatever baseline the tree carries. The first line carries the shapes a
 * real page carries — leading whitespace, a double quote, a character
 * outside the BMP — so one page text stands for every encoding the scan
 * claims to see through; the second is under the scan's 40-byte floor.
 */
const PAGE_LINE = 'A synthetic page line long enough to stand for the baseline text, "quoted" 🔒.';
const PAGE_TEXT = `   ${PAGE_LINE}\nA second line, short.\n`;
/** The same line with its quote and its surrogate pair written as `\uXXXX` escapes. */
const ESCAPED_PAGE_LINE = PAGE_LINE.replaceAll('"', "\\u0022").replace("🔒", "\\ud83d\\udd12");

/** A non-page baseline file, so a byte-identical copy of it can be told from a line match. */
const MANIFEST_TEXT = "page\tsection\tsource\tbytes\tchars_no_ws\n001\tsynthetic\tnative\t1\t1\n";
/** A synthetic stand-in for the earlier partial extraction, one needle-length line and one short line. */
const PARTIAL_LINE = "A synthetic stand-in for the earlier partial extraction, long enough to matter.";
const PARTIAL_TEXT = `${PARTIAL_LINE}\nSecond line.\n`;

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
 * column; a fixture that lost an old row and an old table must name both
 * losses, and a separate fixture that lost an old column must name it — that
 * branch reports the column and skips row matching, so the two losses need
 * one fixture each; a new column, a new table and a new row are allowed.
 */
export function upgradeComparisonProblem(): string | null {
  const dir = mkdtempSync(path.join(tmpdir(), "passquest-upgrade-compare-"));
  const family = (name: string) => path.join(dir, `${name}.sqlite`);
  const write = (
    file: string,
    options: { changedFront?: string; newer?: boolean; droppedRow?: boolean; droppedColumn?: boolean } = {},
  ): void => {
    const db = new Database(file);
    db.run(`CREATE TABLE notes (note_id TEXT PRIMARY KEY, revision INTEGER NOT NULL, front TEXT NOT NULL)`);
    db.run(`CREATE TABLE note_outbox (note_id TEXT NOT NULL, revision INTEGER NOT NULL, status TEXT NOT NULL, PRIMARY KEY (note_id, revision))`);
    db.run(`INSERT INTO notes (note_id, revision, front) VALUES ('pq-1', 1, '${options.changedFront ?? "closes at 4 p.m."}')`);
    if (!options.droppedRow) db.run(`INSERT INTO notes (note_id, revision, front) VALUES ('pq-2', 1, 'opens at 9 a.m.')`);
    if (!options.droppedRow) db.run(`INSERT INTO note_outbox (note_id, revision, status) VALUES ('pq-1', 1, 'drained')`);
    if (options.newer) {
      db.run(`ALTER TABLE notes ADD COLUMN anki_state TEXT NOT NULL DEFAULT 'local'`);
      db.run(`CREATE TABLE learner_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`);
      db.run(`INSERT INTO notes (note_id, revision, front) VALUES ('pq-3', 1, 'accepted after the upgrade')`);
    }
    if (options.droppedRow) db.run(`DROP TABLE note_outbox`);
    if (options.droppedColumn) db.run(`ALTER TABLE notes DROP COLUMN front`);
    db.close();
  };
  try {
    const before = family("before");
    const same = family("same");
    const changed = family("changed");
    const newer = family("newer");
    const lossy = family("lossy");
    const column = family("column");
    write(before);
    write(same);
    write(changed, { changedFront: "closes at 6 p.m." });
    write(newer, { newer: true });
    write(lossy, { droppedRow: true });
    write(column, { droppedColumn: true });
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
    const lost = compareDatabases(before, lossy).problems.join("; ");
    for (const want of ["table note_outbox is missing after the upgrade", "row notes note_id=s:pq-2 is missing after the upgrade"]) {
      if (!lost.includes(want)) {
        return `a dropped old table and a deleted old row in one fixture are not both reported: want ${want}, got ${lost === "" ? "no problem" : lost}`;
      }
    }
    // The open-only row-count check: identical tables pass, a deleted or an
    // added row in an old table fails and names the table.
    if (rowCountProblems(before, same).length > 0) return `the row-count check reports ${rowCountProblems(before, same).join("; ")} for identical databases`;
    for (const [name, fixture] of [["a deleted row", lossy], ["an added row", newer]] as const) {
      const counted = rowCountProblems(before, fixture).join("; ");
      if (!counted.includes("table notes")) return `the row-count check does not report ${name} in notes: ${counted === "" ? "no problem" : counted}`;
    }
    const lostColumn = compareDatabases(before, column).problems.join("; ");
    const wantColumn = "table notes lost column(s) front";
    if (!lostColumn.includes(wantColumn)) {
      return `a dropped old column is not reported as ${wantColumn}: ${lostColumn === "" ? "no problem" : lostColumn}`;
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
		"textLength": 900, "stageChildren": 3, "stageTextLength": 800, "errorBars": 0, "ready": true, "probe": "clear", "opaqueErrors": 0,
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
        "build/fixture/data/local/x.txt": "private source\n",
        "build/fixture/.PassQuest/x.txt": "private source\n",
        "build/fixture/Local-Data/x.txt": "private source\n",
        "build/fixture/.env": "TOKEN=fixture\n",
        "build/fixture/config/.env.production": "TOKEN=fixture\n",
        // The internal-only marker, assembled at run time so this file holds no full marker text.
        "build/fixture/vendor/marked.bin": `${["仅限", "内部", "使用"].join("")}\n`,
        "build/fixture/agent/tutor-host.mjs": `// fixture\nconst key = "${fakeKey()}";\n`,
        "build/fixture/agent/node_modules/vendor/index.mjs": `const key = "${fakeKey()}";\n`,
      },
      prepare: async (root) => {
        linkSeeds = [];
        for (const [link, target] of [
          ["vendor/private-data-link", "../data/private/x.txt"],
          ["vendor/environment-link", "../../config/.env"],
        ] as const) {
          try {
            symlinkSync(target, path.join(root, "build", "fixture", link), "file");
            linkSeeds.push(link);
          } catch (error) {
            if (process.platform !== "win32" || (error as NodeJS.ErrnoException).code !== "EPERM") {
              return `the link seed ${link} could not be created: ${error instanceof Error ? error.message : String(error)}`;
            }
            process.stdout.write(`    package-smoke self-test: link seed ${link} skipped (symlinkSync EPERM)\n`);
          }
        }
        return null;
      },
      postFail: (failures) => {
        const seeds: Array<[string, string]> = [
          ["vendor/handbook.txt", "PDF"],
          ["vendor/cache.dat", "SQLite"],
          ["data/private/x.txt", "private path"],
          ["data/local/x.txt", "private path"],
          [".PassQuest/x.txt", "private path"],
          ["Local-Data/x.txt", "private path"],
          [".env", "environment file"],
          ["config/.env.production", "environment file"],
          ["vendor/marked.bin", "internal-only source marker"],
          ["agent/tutor-host.mjs", "credential-shaped string"],
          ["vendor/private-data-link", "names private data"],
          ["vendor/environment-link", "environment file"],
        ];
        const expected = seeds.filter(([name]) => !name.includes("link") || linkSeeds.includes(name));
        const missing = expected.filter(([name, reason]) => !failures.some((line) => slashed(line).includes(name) && line.includes(reason)));
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
      label: "installed payload holding baseline text in its encoded forms",
      files: {
        ...FIXTURE_FILES,
        [`${BASELINE}/pages/page-001.txt`]: PAGE_TEXT,
        [`${BASELINE}/manifest.tsv`]: MANIFEST_TEXT,
        [BASELINE_PARTIAL]: PARTIAL_TEXT,
        // The page and a non-page baseline file, copied whole: only their
        // hashes can tell them from a file that merely holds a line.
        "build/fixture/vendor/copied-page.txt": PAGE_TEXT,
        "build/fixture/vendor/copied-manifest.tsv": MANIFEST_TEXT,
        // The first line, indent stripped and sharing a physical line with other bytes.
        "build/fixture/vendor/report.txt": `notes: ${PAGE_LINE} — see the appendix\n`,
        // The page as a JSON string: its quote, its newlines and its padlock arrive escaped.
        "build/fixture/vendor/page.json": JSON.stringify({ page: PAGE_TEXT }),
        // The same line with its quote and its surrogate pair as `\uXXXX` escapes.
        "build/fixture/vendor/escaped.txt": `${ESCAPED_PAGE_LINE}\n`,
        // The earlier partial extraction, copied whole and embedded mid-line.
        "build/fixture/vendor/the-2025-copy.txt": PARTIAL_TEXT,
        "build/fixture/vendor/the-2025-line.txt": `excerpt: ${PARTIAL_LINE} (unreviewed)\n`,
        // The negative: an unrelated English line of the same length class.
        "build/fixture/vendor/notes.txt": "An unrelated English line, longer than forty bytes, about something else.\n",
      },
      // The UTF-16LE copies are bytes no template literal can spell: one with
      // the BOM, one without it, which only the mostly-NUL odd positions tell
      // from a binary.
      prepare: async (root) => {
        const vendor = path.join(root, "build", "fixture", "vendor");
        const wide = Buffer.from(PAGE_TEXT, "utf16le");
        writeFileSync(path.join(vendor, "page-utf16.txt"), Buffer.concat([Buffer.from([0xff, 0xfe]), wide]));
        writeFileSync(path.join(vendor, "page-utf16-nobom.txt"), wide);
        return null;
      },
      postFail: (failures) => {
        const lines = failures.map(slashed);
        const wants: Array<[string, string]> = [
          ["vendor/copied-page.txt", `byte-identical to ${BASELINE}/pages/page-001.txt`],
          ["vendor/copied-manifest.tsv", `byte-identical to ${BASELINE}/manifest.tsv`],
          ["vendor/report.txt", `a line of ${BASELINE}/pages/page-001.txt`],
          ["vendor/page.json", `a line of ${BASELINE}/pages/page-001.txt`],
          ["vendor/escaped.txt", `a line of ${BASELINE}/pages/page-001.txt`],
          ["vendor/page-utf16.txt", `a line of ${BASELINE}/pages/page-001.txt`],
          ["vendor/page-utf16-nobom.txt", `a line of ${BASELINE}/pages/page-001.txt`],
          ["vendor/the-2025-copy.txt", `byte-identical to ${BASELINE_PARTIAL}`],
          ["vendor/the-2025-line.txt", `a line of ${BASELINE_PARTIAL}`],
        ];
        const missing = wants.filter(([name, reason]) => !lines.some((line) => line.includes(name) && line.includes(reason)));
        if (missing.length > 0) {
          return `package-smoke: the content scan did not report ${missing.map(([name]) => name).join(", ")}: ${failures.slice(0, 8).join("; ")}`;
        }
        if (lines.some((line) => line.includes("vendor/notes.txt"))) {
          return `package-smoke: the content scan reported the unrelated line: ${failures.slice(0, 4).join("; ")}`;
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
