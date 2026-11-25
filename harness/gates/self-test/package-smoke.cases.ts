/**
 * The package smoke gate's deliberately broken inputs. Both drive the gate
 * over a fixture tree that stands in for an installed package (no installer,
 * no mygo build): one whose data lands inside the install directory — the
 * exact thing the gate exists to catch — and one whose bundled Node runtime
 * is missing, which the layout case must name before any launch.
 *
 * The content scan gets further cases, driven over the same fixture tree with
 * `only: contents` (no package, no launch): one payload that seeds a PDF
 * under another name, a SQLite header, every private-path and `.env` spelling
 * (mixed case included) plus links whose own name and target name private
 * data or an environment file, the internal-only marker and a
 * credential-shaped string in a bundled host — the failure must name every
 * seed and stay silent on the same credential in a third-party file, which
 * this fixture places under the agent tree's own `node_modules`: the lean
 * check refuses that directory on its own, so the credential inside it must
 * stay unnamed too — one synthetic baseline pair (a page file plus the partial
 * extraction) whose byte-identical copies, embedded lines and encoded forms —
 * JSON-escaped, `\uXXXX`-escaped, UTF-16LE — must all be reported while an
 * unrelated line is not, three that drive the lean-tree check (goal 0016) — a
 * shipped `agent/node_modules`, a host that still names the package `openai`,
 * and a host embedding a build machine's account path in bun's per-module
 * comment — one whose installed `agent/THIRD-PARTY-NOTICES.txt` lost one of
 * the licence texts the staging assembled, which the gate's `notices` option
 * requires of the fixture exactly as a real run requires it of the file its
 * own staging wrote. Two more drive cases of their own: a brief over an
 * installed copy whose `agent/durable-host.mjs` is gone must fail by that
 * name before any launch, and a brief whose paused run left no pending
 * durable work must be refused (goal 0016 P3). Three accepted trees close the
 * list: one whose only secret-shaped string sits in a third-party file, one
 * whose host carries urls and clock-shaped text the machine-path scan must
 * read as ordinary content, and one whose notices file carries every text
 * whole.
 *
 * Three cases drive the bundle's Info.plist (goal 0017 P1): a tree whose
 * `.app` plist kept an older `LSMinimumSystemVersion` than the mygo.json it
 * carries declares must fail naming both values, the same tree with the
 * declared value must pass, and one whose nested `Helpers/X.app` plist keeps
 * an older value must pass too, because a nested bundle's plist is not the
 * installed app's — on every OS, so the check is not macOS-only.
 */
import { BASELINE, BASELINE_PARTIAL } from "../../lib/baseline.ts";
import { Database } from "bun:sqlite";
import { chmodSync, copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { makePackageSmokeGate } from "../package-smoke.ts";
import { noticesProblems } from "../package-smoke/contents.ts";
import { compareDatabases, rowCountProblems } from "../package-smoke/upgrade.ts";
import { assembleNotices } from "../../lib/notices.ts";
import type { BundlePackage, NoticeSource, NoticeText, StagedNotices } from "../../lib/notices.ts";
import { packageNaming } from "../../lib/package-build.ts";
import {
  AGENT_DIR,
  assertLeanAgentTree,
  bundleEntrySource,
  bundlePackages,
  emptyInlinedSetProblem,
  LOCK_FILE,
  lockedVersions,
  lockProblems,
  NOTICES_FILE,
  NODE_VERSION,
  noticeSource,
  runtimeResolutionUses,
  stageAgent,
} from "../../lib/package.ts";
import type { NoticeFs } from "../../lib/package.ts";
import { executable, run } from "../../lib/process.ts";
import { copyProduct, GO_MOD, linkStore, NOTE_SMOKE_SOURCES, patchCopy } from "./fixture.ts";
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

/** The fixture mygo.json of a case that carries the macOS floor: the value MyGo copies into LSMinimumSystemVersion. */
const FIXTURE_MYGO_MACOS = '{"name": "Pass Quest", "version": "0.0.0", "macos": {"minimumSystemVersion": "26.0"}}\n';

/**
 * The bundle's Info.plist as MyGo writes it (cmd/mygo/bundle.go, infoPlist
 * and writePlistValue: XML, sorted keys, tab-indented), reduced to two keys.
 * Placed where the gate's walk meets it in an installed macOS tree, the
 * `.app`'s `Contents/Info.plist`.
 */
function fixturePlist(minimum: string): string {
  return [
    `<?xml version="1.0" encoding="UTF-8"?>`,
    `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`,
    `<plist version="1.0">`,
    `<dict>`,
    `\t<key>CFBundleIdentifier</key>`,
    `\t<string>com.github.wxxb789.passquest</string>`,
    `\t<key>LSMinimumSystemVersion</key>`,
    `\t<string>${minimum}</string>`,
    `</dict>`,
    `</plist>`,
    "",
  ].join("\n");
}

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

/**
 * The staging rules (goal 0016): a host that resolves a module at run time
 * beyond a string-literal specifier — a computed `import()`/`require()`
 * argument, an argument concatenated after a quote, `createRequire`,
 * `require.resolve`, `import.meta.resolve` — must be refused by
 * `runtimeResolutionUses`, while a literal import (static, dynamic, or a
 * template with no substitution) and a plain `import.meta.url` read must not
 * be; and `assertLeanAgentTree` must refuse a staged file that embeds a
 * machine path while ordinary urls and clock-shaped text pass. The last
 * fixture drives `stageAgent` itself, so a refusal that stops being applied
 * in staging fails here: a host a package cannot resolve must never be
 * staged silently.
 */
export async function stagingRefusalsProblem(): Promise<string | null> {
  const refused: Array<[string, string]> = [
    ["a computed import argument", `const name = "no-such-package";\nawait import(name);\n`],
    ["a computed require argument", `const name = "no-such-package";\nrequire(name);\n`],
    ["an import argument concatenated after the quote", `await import("open" + "ai");\n`],
    ["a require argument built with .concat", `require("pkg-".concat(kind));\n`],
    ["a template argument concatenated with a variable", "await import(`openai` + kind);\n"],
    ["a template with a substitution", "await import(`pkg-${kind}`);\n"],
    ["a createRequire resolver", `const req = createRequire(import.meta.url);\nconst x = req("openai");\n`],
    ["a require.resolve call", `const p = require.resolve("openai");\n`],
    ["an import.meta.resolve call", `const p = import.meta.resolve("openai");\n`],
  ];
  for (const [label, source] of refused) {
    if (runtimeResolutionUses(source).length === 0) return `staging rules: a host with ${label} is not refused`;
  }
  const accepted: Array<[string, string]> = [
    ["a static literal import", `import { x } from "openai";\n`],
    ["a literal dynamic import", `const m = await import("openai");\n`],
    ["a literal require argument", `const m = require("openai");\n`],
    ["a substitution-free template import", "const m = await import(`openai`);\n"],
    ["an import.meta.url read", `const here = import.meta.url;\n`],
    ["a url and clock-shaped text", `const u = "https://example.com/v1";\nconst t = "12:30";\n`],
  ];
  for (const [label, source] of accepted) {
    const uses = runtimeResolutionUses(source);
    if (uses.length > 0) return `staging rules: a host with ${label} is refused as ${uses.join(", ")}`;
  }
  const dir = mkdtempSync(path.join(tmpdir(), "passquest-staging-rules-"));
  try {
    const host = path.join(dir, "durable-host.mjs");
    for (const [label, text] of [
      ["a Windows drive path", `// C:/Users/dev/AppData/Local/Temp/passquest-agent-entry-x1y2/entry.mjs\n`],
      ["a POSIX user home", `// /Users/dev/projects/checkout/agent/durable-host.mjs\n`],
      ["a Linux temp path", `// /tmp/passquest-agent-entry-x1y2/entry.mjs\n`],
    ] as const) {
      writeFileSync(host, text);
      let message = "";
      try {
        assertLeanAgentTree(dir);
      } catch (error) {
        message = error instanceof Error ? error.message : String(error);
      }
      if (!message.includes("embeds the machine path")) {
        return `staging rules: a staged file with ${label} is not refused (${message === "" ? "no failure" : message})`;
      }
    }
    writeFileSync(host, `const u = "https://example.com/v1";\nconst t = "12:30";\n`);
    assertLeanAgentTree(dir);
    // The notices file's machine rule is the same on every OS (goal 0016 P4,
    // checkoutRootIn): the shapes and the temp-directory literal are exempt —
    // licence prose may name /home/ or /tmp/, and tmpdir() is machine-spelled
    // itself, so a notice naming this machine's own temp path must pass here
    // too — while the checkout root is refused, which the staging fixture
    // below drives end to end.
    const notices = path.join(dir, NOTICES_FILE);
    writeFileSync(
      notices,
      "partial-json@0.1.7 / LICENSE\n" +
        "See /home/runner/LICENSE, C:/Users/runner/AppData/Local/x and /tmp/passquest-build/openai in the original.\n" +
        `The build read ${tmpdir()}/passquest-build/openai.\n`,
    );
    assertLeanAgentTree(dir);
    rmSync(notices, { force: true });
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
  // The checks above call the rules' functions directly; this fixture drives
  // staging itself, so it fails when stageAgent stops making either call: the
  // run-time-resolution refusal, or the lean-tree proof.
  const root = mkdtempSync(path.join(tmpdir(), "passquest-staging-wiring-"));
  try {
    const agentDir = path.join(root, AGENT_DIR);
    mkdirSync(agentDir, { recursive: true });
    const stage = path.join(root, "build", "stage");
    const stageFailure = async (): Promise<string> => {
      try {
        await stageAgent(root, stage);
        return "";
      } catch (error) {
        return error instanceof Error ? error.message : String(error);
      }
    };
    writeFileSync(path.join(agentDir, "durable-host.mjs"), `const name = "no-such-package";\nawait import(name);\n`);
    let message = await stageFailure();
    if (!message.includes("resolves a module at run time")) {
      return `staging rules: stageAgent did not refuse a host that resolves a module at run time (${message === "" ? "no failure" : message})`;
    }
    writeFileSync(path.join(agentDir, "durable-host.mjs"), `// built from ${root}/agent/durable-host.mjs\n`);
    message = await stageFailure();
    if (!message.includes("embeds the machine path")) {
      return `staging rules: stageAgent did not refuse a copied host embedding the checkout root (${message === "" ? "no failure" : message})`;
    }
    writeFileSync(path.join(agentDir, "durable-host.mjs"), `const u = "https://example.com/v1";\n`);
    const staged = await stageAgent(root, stage);
    if (staged.hosts.length !== 1 || !staged.check.includes("is lean")) {
      return `staging rules: a clean host did not stage as one lean host (${staged.hosts.length} host(s): ${staged.check})`;
    }
    // The lock check (goal 0016 P4): resolution goes through the checkout's
    // node_modules, so a bundle that inlines a package the committed lock
    // does not pin at the version on disk must fail staging. With the lock
    // agreeing, the same host stages and the notices file carries the
    // package's licence text — and a package whose terms the assembly cannot
    // state fails staging instead of shipping without a notice.
    const pkg = path.join(agentDir, "node_modules", "fixture-pkg");
    mkdirSync(pkg, { recursive: true });
    const fixtureManifest = (license: string): string =>
      JSON.stringify({ name: "fixture-pkg", version: "1.2.3", type: "module", main: "index.js", license, author: "Fiction Author" });
    writeFileSync(path.join(pkg, "package.json"), fixtureManifest("MIT"));
    writeFileSync(path.join(pkg, "index.js"), "export const thing = 1;\n");
    writeFileSync(path.join(pkg, "LICENSE"), "The Fiction Licence, version 1.\n");
    writeFileSync(path.join(agentDir, "durable-host.mjs"), `import { thing } from "fixture-pkg";\nconsole.log(thing);\n`);
    const lock = (version: string): string =>
      `{\n  "lockfileVersion": 2,\n  "packages": {\n    "fixture-pkg": ["fixture-pkg@${version}", "", {}, "sha512-x"],\n  },\n}\n`;
    writeFileSync(path.join(root, LOCK_FILE), lock("1.0.0"));
    message = await stageFailure();
    if (!message.includes(LOCK_FILE) || !message.includes("fixture-pkg@1.2.3")) {
      return `staging rules: a bundle whose package the lock pins at another version is not refused as such (${message === "" ? "no failure" : message})`;
    }
    writeFileSync(path.join(root, LOCK_FILE), lock("1.2.3"));
    const bundled = await stageAgent(root, stage);
    const noticesText = readFileSync(path.join(stage, AGENT_DIR, NOTICES_FILE), "utf8");
    if (!noticesText.includes("fixture-pkg@1.2.3 / LICENSE") || !noticesText.includes("The Fiction Licence, version 1.")) {
      return `staging rules: the staged notices do not carry the inlined package's licence text (${JSON.stringify(noticesText.slice(0, 160))})`;
    }
    const inlined = bundled.notices.packages.map((entry) => entry.name).join(", ");
    if (inlined !== "fixture-pkg") return `staging rules: the staged tree reports the inlined packages as ${inlined === "" ? "none" : inlined}, want fixture-pkg`;
    writeFileSync(path.join(pkg, "package.json"), fixtureManifest("Apache-2.0"));
    rmSync(path.join(pkg, "LICENSE"), { force: true });
    message = await stageFailure();
    if (!message.includes("ships no licence file of its own")) {
      return `staging rules: a package with no licence file and no MIT declaration did not fail staging (${message === "" ? "no failure" : message})`;
    }
    // The notices file's one machine literal: a licence text carrying the
    // checkout root reaches the assembled file verbatim, and staging — the
    // caller that knows the root — must refuse it (goal 0016 P4). The shapes
    // and the temp directory stay exempt, as the direct fixtures above show.
    writeFileSync(path.join(pkg, "package.json"), fixtureManifest("MIT"));
    writeFileSync(path.join(pkg, "LICENSE"), `Read from ${root}/node_modules/fixture-pkg in the original.\n`);
    message = await stageFailure();
    if (!message.includes(NOTICES_FILE) || !message.includes("embeds the machine path")) {
      return `staging rules: a staged notices file carrying the checkout root is not refused (${message === "" ? "no failure" : message})`;
    }
    return null;
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

/**
 * The generated bundle entry (bundleEntrySource, goal 0016 P2): one namespace
 * import per specifier the host names, each held in the exported `pinned`
 * array, then the host itself last. The held array is what keeps the
 * packages' module initialization — their `sideEffects` arrays make bun drop
 * an init block nothing holds, and the plain `bun build
 * agent/durable-host.mjs` form then faults on a real brief with `Cannot read
 * properties of undefined` — so the prelude must not lose a specifier or fall
 * behind the host unnoticed.
 */
export function bundleEntryProblem(): string | null {
  const base = process.platform === "win32" ? "C:\\x" : "/x";
  const hostPath = path.join(base, AGENT_DIR, "durable-host.mjs");
  const pins = [
    {
      specifier: "@earendil-works/chord/context",
      path: path.join(base, AGENT_DIR, "node_modules", "@earendil-works", "chord", "context.js"),
    },
    { specifier: "openai", path: path.join(base, AGENT_DIR, "node_modules", "openai", "index.js") },
  ];
  const wanted = [
    "// Generated by the harness for staging: do not edit.",
    ...pins.map((pin, index) => {
      const target = JSON.stringify(pin.path.split(path.sep).join("/"));
      return `import * as pinned${index} from ${target}; // ${pin.specifier}`;
    }),
    `export const pinned = [${pins.map((_, index) => `pinned${index}`).join(", ")}];`,
    `import ${JSON.stringify(hostPath.split(path.sep).join("/"))};`,
  ];
  const lines = bundleEntrySource(hostPath, pins).trimEnd().split("\n");
  if (lines.length !== wanted.length) {
    return `the bundle entry has ${lines.length} line(s): ${JSON.stringify(lines)}, want ${JSON.stringify(wanted)}`;
  }
  for (const [index, line] of wanted.entries()) {
    if (lines[index] !== line) {
      return `the bundle entry line ${index + 1} is ${JSON.stringify(lines[index])}, want ${JSON.stringify(line)}`;
    }
  }
  return null;
}

/** A notices source over a fixture's files: read by path, and list the names under a directory (a deeper file's first segment stands for a subdirectory). */
function fixtureSource(files: Record<string, string>): NoticeSource {
  return {
    read: (file) => files[file],
    list: (dir) => {
      const prefix = dir.endsWith(path.sep) ? dir : `${dir}${path.sep}`;
      const names = new Set<string>();
      for (const file of Object.keys(files)) {
        if (file.startsWith(prefix)) names.add(file.slice(prefix.length).split(path.sep)[0]);
      }
      return [...names].sort();
    },
  };
}

/**
 * The notices assembly (notices.ts, goal 0016 P4): every inlined package's
 * licence-like files travel verbatim under a header naming the package and
 * the file's path within it — the ones under the package's sources included,
 * which is where the licences of the code it vendors sit — while a nested
 * node_modules is left to the packages the metafile names. The files under
 * THIRD_PARTY_NOTICES follow, and a package whose tree carries no licence text
 * of its own falls back to the SPDX MIT text with its author line. Where the
 * assembly cannot state a package's terms — no licence text of its own and not
 * a declared MIT with an author, a manifest disagreeing with the metafile, or
 * a licence file it cannot read — it must fail the run rather than ship
 * without a notice. The assembled text never carries the directories the
 * packages were read from, which is what the machine-path exemption in
 * assertLeanAgentTree rests on.
 */
export function noticesAssemblyProblem(): string | null {
  const base = process.platform === "win32" ? "C:\\fixture" : "/fixture";
  const root = (...parts: string[]): string => path.join(base, "node_modules", ...parts);
  const openai = root("openai");
  const vector = root("@scope", "vector");
  const manifest = (name: string, version: string, extra: Record<string, string>): string => JSON.stringify({ name, version, ...extra });
  const packages: BundlePackage[] = [
    { name: "openai", version: "7.19.0", root: openai },
    { name: "@scope/vector", version: "2.0.0", root: vector },
  ];
  const files: Record<string, string> = {
    [path.join(openai, "package.json")]: manifest("openai", "7.19.0", { license: "Apache-2.0", author: "OpenAI <support@openai.com>" }),
    [path.join(openai, "LICENSE")]: "Apache License\nVersion 2.0, January 2004\n",
    [path.join(openai, "NOTICE")]: "OpenAI\r\n",
    // The code openai vendors: the bundle inlines these modules
    // (openai/internal/qs/stringify.mjs, openai/_vendor/partial-json-parser/parser.mjs)
    // while their licences sit under the package's sources, so a search of the
    // package root alone would ship the code without its terms (goal 0016 P4).
    [path.join(openai, "src", "internal", "qs", "LICENSE.md")]: "BSD 3-Clause\nCopyright (c) 2014, Nathan LaFreniere\n",
    [path.join(openai, "_vendor", "partial-json-parser", "LICENSE")]: "MIT License\nCopyright (c) 2023 Promplate Dev Team\n",
    // A package installed beside this one is another package, named by the
    // metafile and noticed for itself: its licence is not this package's to
    // carry, and the walk must not descend into it.
    [path.join(openai, "node_modules", "dep", "LICENSE")]: "Dependency Licence\n",
    [path.join(vector, "package.json")]: manifest("@scope/vector", "2.0.0", { license: "MIT", author: "Vector Author" }),
    [path.join(vector, "THIRD_PARTY_NOTICES", "dep-LICENSE")]: "TypeBox\nMIT License\n",
  };
  const assembly = assembleNotices(packages, fixtureSource(files));
  const names = assembly.packages.map((pkg) => pkg.name).join(", ");
  if (names !== "@scope/vector, openai") return `the notices assembly orders the packages ${names}, want @scope/vector, openai`;
  if (assembly.texts.length !== 6) return `the notices assembly made ${assembly.texts.length} text(s), want 6: ${assembly.texts.map((notice) => notice.label).join("; ")}`;
  const [fallback, third, licence, notice, vendored, sourceLicence] = assembly.texts;
  if (!fallback.label.includes("@scope/vector@2.0.0") || !fallback.label.includes("SPDX")) {
    return `the SPDX fallback's header is ${JSON.stringify(fallback.label)}, want the package and SPDX named`;
  }
  if (!fallback.text.startsWith("MIT License") || !fallback.text.includes("Copyright (c) Vector Author")) {
    return `the SPDX fallback's text does not carry the author's copyright line: ${JSON.stringify(fallback.text.slice(0, 80))}`;
  }
  if (third.label !== "@scope/vector@2.0.0 / THIRD_PARTY_NOTICES/dep-LICENSE" || third.text !== "TypeBox\nMIT License\n") {
    return `the third-party notice is ${JSON.stringify(third.label)} ${JSON.stringify(third.text)}, want the slash-spelled label and the file's text`;
  }
  if (licence.label !== "openai@7.19.0 / LICENSE" || licence.text !== "Apache License\nVersion 2.0, January 2004\n") {
    return `the Apache text is ${JSON.stringify(licence.label)} ${JSON.stringify(licence.text)}`;
  }
  if (notice.label !== "openai@7.19.0 / NOTICE" || notice.text !== "OpenAI\n") {
    return `the CRLF notice is ${JSON.stringify(notice.label)} ${JSON.stringify(notice.text)}, want LF line endings`;
  }
  if (vendored.label !== "openai@7.19.0 / _vendor/partial-json-parser/LICENSE" || vendored.text !== "MIT License\nCopyright (c) 2023 Promplate Dev Team\n") {
    return `the vendored package's licence is ${JSON.stringify(vendored.label)} ${JSON.stringify(vendored.text)}, want the package-relative path`;
  }
  if (sourceLicence.label !== "openai@7.19.0 / src/internal/qs/LICENSE.md" || sourceLicence.text !== "BSD 3-Clause\nCopyright (c) 2014, Nathan LaFreniere\n") {
    return `the licence under the package's sources is ${JSON.stringify(sourceLicence.label)} ${JSON.stringify(sourceLicence.text)}, want the package-relative path`;
  }
  if (assembly.text.includes("Dependency Licence")) {
    return "the notices assembly carries the licence of a package installed inside another package's node_modules";
  }
  if (assembly.text.includes("\r")) return "the assembled text carries a carriage return";
  for (const [index, text] of [fallback, third, licence, notice, vendored, sourceLicence].entries()) {
    for (const part of [text.label, text.text]) {
      if (!assembly.text.includes(part)) return `the assembled text does not carry ${JSON.stringify(part)} (text ${index + 1})`;
    }
  }
  const order = [fallback.label, third.label, licence.label, notice.label, vendored.label, sourceLicence.label].map((label) => assembly.text.indexOf(label));
  if (!order.every((at, index) => at >= 0 && (index === 0 || at > order[index - 1]))) {
    return `the assembled file's section order is ${order.join(", ")}, want the packages sorted and each package's own texts before its THIRD_PARTY_NOTICES`;
  }
  if (assembly.text.includes(base)) return `the assembled text carries the fixture's own directory ${base}`;

  const failure = (extra: Record<string, string>, pkg: BundlePackage): string => {
    try {
      assembleNotices([pkg], fixtureSource({ ...files, ...extra }));
      return "";
    } catch (error) {
      return error instanceof Error ? error.message : String(error);
    }
  };
  const unknown = root("unknown");
  const unknownFiles = {
    [path.join(unknown, "package.json")]: manifest("unknown", "1.0.0", { license: "Apache-2.0", author: "Someone" }),
  };
  let message = failure(unknownFiles, { name: "unknown", version: "1.0.0", root: unknown });
  if (!message.includes("ships no licence file of its own")) {
    return `a package with no licence file and no MIT declaration is not refused (${message === "" ? "no failure" : message})`;
  }
  const authorless = root("authorless");
  const authorlessFiles = {
    [path.join(authorless, "package.json")]: manifest("authorless", "1.0.0", { license: "MIT" }),
  };
  message = failure(authorlessFiles, { name: "authorless", version: "1.0.0", root: authorless });
  if (!message.includes("names no author")) {
    return `a package with no licence file and no author is not refused (${message === "" ? "no failure" : message})`;
  }
  // The SPDX fallback substitutes the author by function, not by a
  // replacement pattern (goal 0016 P4): an author carrying `$&` or `$1` must
  // arrive verbatim, where a string replacement would expand it and leave the
  // `<holder>` placeholder in the shipped file.
  const dollar = root("dollar");
  const dollarFiles = {
    [path.join(dollar, "package.json")]: manifest("dollar", "1.0.0", { license: "MIT", author: "Dollar $& Author $1" }),
  };
  const dollarText = assembleNotices([{ name: "dollar", version: "1.0.0", root: dollar }], fixtureSource(dollarFiles)).text;
  if (!dollarText.includes("Copyright (c) Dollar $& Author $1")) {
    return `the SPDX fallback does not carry an author's $& and $1 verbatim: ${JSON.stringify(dollarText.split("\n").slice(0, 4))}`;
  }
  if (dollarText.includes("<holder>")) {
    return "the SPDX fallback left its <holder> placeholder in place for an author carrying $&";
  }
  message = failure({ [path.join(openai, "package.json")]: manifest("openai", "6.0.0", { license: "Apache-2.0" }) }, packages[0]);
  if (!message.includes("disagree")) {
    return `a manifest disagreeing with the metafile is not refused (${message === "" ? "no failure" : message})`;
  }
  // A THIRD_PARTY_NOTICES that keeps its notices in subdirectories — the
  // layout upstream packages use (`THIRD_PARTY_NOTICES/lodash/LICENSE`) — must
  // travel like any other notice: the walk reaches it, and no listing reads a
  // directory as though it were a file (goal 0016 P4, the first
  // implementation's false failure on this shape).
  const nestedThird = assembleNotices([packages[1]!], fixtureSource({ ...files, [path.join(vector, "THIRD_PARTY_NOTICES", "sub", "x-LICENSE")]: "x\n" }));
  const nestedLabel = "@scope/vector@2.0.0 / THIRD_PARTY_NOTICES/sub/x-LICENSE";
  if (!nestedThird.texts.some((notice) => notice.label === nestedLabel && notice.text === "x\n")) {
    return `a notice under a THIRD_PARTY_NOTICES subdirectory does not travel: ${nestedThird.texts.map((notice) => notice.label).join("; ")}`;
  }
  // A licence file below the package root is what the walk exists for: with a
  // root-only listing this package declares Apache-2.0 and ships no licence
  // file, so the assembly would refuse the run where the vendored code's terms
  // must travel (goal 0016 P4).
  const nested = root("nested-only");
  const nestedFiles = {
    [path.join(nested, "package.json")]: manifest("nested-only", "1.0.0", { license: "Apache-2.0", author: "Someone" }),
    [path.join(nested, "dist", "vendor", "dep", "LICENSE")]: "Vendored Terms\n",
  };
  let nestedLabels = "";
  try {
    nestedLabels = assembleNotices([{ name: "nested-only", version: "1.0.0", root: nested }], fixtureSource(nestedFiles))
      .texts.map((notice) => notice.label)
      .join("; ");
  } catch (error) {
    nestedLabels = error instanceof Error ? error.message : String(error);
  }
  if (nestedLabels !== "nested-only@1.0.0 / dist/vendor/dep/LICENSE") {
    return `a package whose only licence file sits in a nested directory is not assembled from it: ${nestedLabels}`;
  }
  return null;
}

/**
 * The licence-name rule (goal 0016 P4): a regular file travels when its name
 * carries a licence word — the compound spellings included, `MIT-LICENSE`,
 * `THIRD-PARTY-NOTICES.txt`, `ThirdPartyNotices.txt` — or when it sits under a
 * directory whose name carries one (`LICENSES/`, `THIRD_PARTY_NOTICES/`, at
 * whatever depth), and a name that ends in a code extension does not travel,
 * licence word or not (`license-checker.js`, `notices.json`). The names come
 * from the goal's review of the first implementation, which matched a name
 * only when it *started* with the licence word and so missed every compound
 * spelling a package may carry its terms under.
 */
export function licenceNamesProblem(): string | null {
  const base = process.platform === "win32" ? "C:\\names" : "/names";
  const pkg = path.join(base, "node_modules", "named");
  const files: Record<string, string> = {
    [path.join(pkg, "package.json")]: JSON.stringify({ name: "named", version: "1.0.0", license: "Apache-2.0" }),
    [path.join(pkg, "MIT-LICENSE")]: "MIT-LICENSE\n",
    [path.join(pkg, "THIRD-PARTY-NOTICES.txt")]: "THIRD-PARTY-NOTICES.txt\n",
    [path.join(pkg, "ThirdPartyNotices.txt")]: "ThirdPartyNotices.txt\n",
    [path.join(pkg, "COPYING")]: "COPYING\n",
    [path.join(pkg, "licenses", "MIT.txt")]: "A file under a licence directory\n",
    [path.join(pkg, "THIRD_PARTY_NOTICES", "sub", "x-LICENSE")]: "A licence under a notices subdirectory\n",
    [path.join(pkg, "THIRD_PARTY_NOTICES", "sub", "upstream-terms.txt")]: "Terms under a notices subdirectory\n",
    // Code, however it is named: none of these may travel as a licence text.
    [path.join(pkg, "license-checker.js")]: "const licence = 1;\n",
    [path.join(pkg, "notices.json")]: "{}\n",
    [path.join(pkg, "licenses", "index.js")]: "export const licences = [];\n",
    [path.join(pkg, "src", "index.js")]: "export const x = 1;\n",
  };
  const assembly = assembleNotices([{ name: "named", version: "1.0.0", root: pkg }], fixtureSource(files));
  const carried = [
    "named@1.0.0 / COPYING",
    "named@1.0.0 / MIT-LICENSE",
    "named@1.0.0 / THIRD-PARTY-NOTICES.txt",
    "named@1.0.0 / THIRD_PARTY_NOTICES/sub/upstream-terms.txt",
    "named@1.0.0 / THIRD_PARTY_NOTICES/sub/x-LICENSE",
    "named@1.0.0 / ThirdPartyNotices.txt",
    "named@1.0.0 / licenses/MIT.txt",
  ].join("; ");
  const labels = assembly.texts.map((notice) => notice.label).join("; ");
  if (labels !== carried) {
    return `the licence-name rule carries ${labels}, want ${carried}`;
  }
  for (const skipped of ["license-checker.js", "notices.json", "licenses/index.js", "src/index.js"]) {
    if (assembly.text.includes(skipped)) return `the licence-name rule carries ${skipped}, which is source code, not a notice`;
  }
  if (assembly.texts.some((notice) => notice.label.includes("SPDX"))) {
    return "the licence-name rule falls back to the SPDX text for a package that carries licence texts";
  }
  return null;
}

/**
 * The licence source fails closed (goal 0016 P4): only a path that is not
 * there — ENOENT, or a component that is not a directory, ENOTDIR — reads as
 * absent. A read or listing that fails for any other reason (EACCES, EPERM,
 * EBUSY, ELOOP, ENAMETOOLONG) must throw, naming the path: reading it as
 * absent would drop a licence text the package ships, and the notices check
 * over the installed file would compare against that dropped set — the whole
 * P4 chain would pass over the loss, on Windows under a deny-ACL or a held
 * handle and on every POSIX under EACCES. The injected file system drives the
 * failures no fixture can produce on demand; the real one covers the absent
 * paths, a regular file and its directory, and the symlink refusal.
 */
export function noticeSourceProblem(): string | null {
  const dir = mkdtempSync(path.join(tmpdir(), "passquest-notice-source-"));
  try {
    const file = path.join(dir, "plain.txt");
    writeFileSync(file, "text\n");
    const source = noticeSource();
    const real: Array<[string, unknown, unknown]> = [
      ["a regular file", source.read(file), "text\n"],
      ["a directory read as a file", source.read(dir), undefined],
      ["a path that is not there", source.read(path.join(dir, "absent")), undefined],
      ["a path under a file", source.read(path.join(file, "child")), undefined],
      ["a directory's names", source.list(dir), ["plain.txt"]],
      ["a file listed as a directory", source.list(file), []],
      ["a directory that is not there", source.list(path.join(dir, "absent")), []],
      ["a path under a file", source.list(path.join(file, "child")), []],
    ];
    for (const [what, got, want] of real) {
      if (JSON.stringify(got) !== JSON.stringify(want)) {
        return `the licence source reads ${what} as ${JSON.stringify(got)}, want ${JSON.stringify(want)}`;
      }
    }
    const failure = (code: string): Error => Object.assign(new Error(`${code}: injected`), { code });
    const lstatFails = (code: string): NoticeFs => ({
      lstat: () => {
        throw failure(code);
      },
      readFile: () => "",
      readdir: () => [],
    });
    const readFails = (code: string): NoticeFs => ({
      lstat: () => ({ isFile: () => true, isSymbolicLink: () => false, isDirectory: () => false }),
      readFile: () => {
        throw failure(code);
      },
      readdir: () => [],
    });
    const readdirFails = (code: string): NoticeFs => ({
      lstat: () => ({ isFile: () => false, isSymbolicLink: () => false, isDirectory: () => true }),
      readFile: () => "",
      readdir: () => {
        throw failure(code);
      },
    });
    for (const code of ["EACCES", "EPERM", "EBUSY", "ELOOP", "ENAMETOOLONG", "EISDIR"]) {
      const calls: Array<[string, () => unknown, string]> = [
        [`a read whose lstat fails with ${code}`, () => noticeSource(lstatFails(code)).read(file), file],
        [`a read whose readFile fails with ${code}`, () => noticeSource(readFails(code)).read(file), file],
        [`a listing whose lstat fails with ${code}`, () => noticeSource(lstatFails(code)).list(dir), dir],
        [`a listing whose readdir fails with ${code}`, () => noticeSource(readdirFails(code)).list(dir), dir],
      ];
      for (const [what, call, at] of calls) {
        let message = "";
        try {
          call();
        } catch (error) {
          message = error instanceof Error ? error.message : String(error);
        }
        if (message === "") return `the licence source reads ${what} as absent; it must fail the run instead`;
        if (!message.includes(code) || !message.includes(at)) {
          return `the licence source's failure for ${what} does not name the path and the error: ${message}`;
        }
      }
    }
    // The two codes that mean the path is not there: absent, on both calls,
    // whether the stat or the open reports them.
    if (noticeSource(lstatFails("ENOENT")).read(file) !== undefined) return "an injected ENOENT on a read is not absent";
    if (noticeSource(lstatFails("ENOTDIR")).read(file) !== undefined) return "an injected ENOTDIR on a read is not absent";
    if (noticeSource(readFails("ENOENT")).read(file) !== undefined) return "an injected ENOENT from a file read is not absent";
    if (noticeSource(lstatFails("ENOENT")).list(dir).length !== 0) return "an injected ENOENT on a listing is not absent";
    if (noticeSource(lstatFails("ENOTDIR")).list(dir).length !== 0) return "an injected ENOTDIR on a listing is not absent";
    if (noticeSource(readdirFails("ENOENT")).list(dir).length !== 0) return "an injected ENOENT from a directory listing is not absent";
    const link: NoticeFs = {
      lstat: () => ({ isFile: () => false, isSymbolicLink: () => true, isDirectory: () => false }),
      readFile: () => "",
      readdir: () => [],
    };
    if (noticeSource(link).read(file) !== undefined) return "a symlink reads as a regular file";
    let refusal = "";
    try {
      noticeSource(link).list(file);
    } catch (error) {
      refusal = error instanceof Error ? error.message : String(error);
    }
    if (!refusal.includes("is a symlink")) return `a symlink directory is not refused by name: ${refusal === "" ? "no failure" : refusal}`;
    return null;
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

/**
 * The lock check (goal 0016 P4): the versions bun.lock pins come out of the
 * text lockfile bun writes — nested resolutions included, so a name two
 * parents resolve differently keeps both versions — and an inlined package
 * the lock names no version of, or pins at another version, is reported and
 * fails the bundle. The committed lock must parse and pin every direct
 * dependency of agent/package.json: the names the host's own imports resolve
 * through, and the first thing a stale tree or a lockfile-format change would
 * break.
 */
export function bundleLockProblem(): string | null {
  const locked = lockedVersions(`{
  "lockfileVersion": 2,
  "packages": {
    "openai": ["openai@7.19.0", "", {}, "sha512-x"],
    "@scope/dup": ["@scope/dup@1.0.0", "", {}, "sha512-x"],
    "parent/@scope/dup": ["@scope/dup@2.0.0", "", {}, "sha512-x"],
  },
}
`);
  const versions = (name: string): string => (locked.get(name) ?? []).join(", ");
  if (versions("openai") !== "7.19.0" || versions("@scope/dup") !== "1.0.0, 2.0.0") {
    return `the text lockfile reads openai as [${versions("openai")}] and @scope/dup as [${versions("@scope/dup")}], want 7.19.0 and 1.0.0, 2.0.0`;
  }
  const pkg = (name: string, version: string): BundlePackage => ({ name, version, root: "" });
  if (lockProblems([pkg("openai", "7.19.0"), pkg("@scope/dup", "2.0.0")], locked).length > 0) {
    return `the lock check reports ${lockProblems([pkg("openai", "7.19.0"), pkg("@scope/dup", "2.0.0")], locked).join("; ")} for packages the lock pins`;
  }
  const problems = lockProblems([pkg("openai", "6.0.0"), pkg("absent", "1.0.0")], locked).join("; ");
  if (!problems.includes("pins openai at 7.19.0")) return `the lock check does not report the version mismatch: ${problems === "" ? "no problem" : problems}`;
  if (!problems.includes("names no such package")) return `the lock check does not report a package the lock does not pin: ${problems}`;
  let message = "";
  try {
    lockedVersions('{"packages": {"bad": ["not a resolution"]}}');
  } catch (error) {
    message = error instanceof Error ? error.message : String(error);
  }
  if (!message.includes("not a resolved package@version")) {
    return `an unreadable lock entry is not refused (${message === "" ? "no failure" : message})`;
  }
  const root = path.resolve(import.meta.dir, "..", "..", "..");
  const committed = lockedVersions(readFileSync(path.resolve(root, LOCK_FILE), "utf8"));
  const raw: unknown = JSON.parse(readFileSync(path.resolve(root, AGENT_DIR, "package.json"), "utf8"));
  const fields: object = raw !== null && typeof raw === "object" ? raw : {};
  const declared = "dependencies" in fields && fields.dependencies !== null && typeof fields.dependencies === "object" ? Object.keys(fields.dependencies) : [];
  if (declared.length === 0) return `agent/package.json names no dependencies to check against ${LOCK_FILE}`;
  const missing = declared.filter((name) => !committed.has(name));
  if (missing.length > 0) return `the committed ${LOCK_FILE} pins no ${missing.join(", ")}`;
  return null;
}

/**
 * The bundle's packages come out of the metafile (goal 0016 P4):
 * `bundlePackages` folds the metafile's `node_modules` inputs into one entry
 * per package, read from that package's own package.json, and refuses a
 * metafile it cannot parse, one that names no inputs at all, and one whose
 * package the tree does not carry or that declares another name. Those
 * refusals are what keep the lock check and the notices assembly honest: a
 * metafile that went empty — a bun whose format changed — must fail the run
 * rather than let both pass over a set that is not the bundle's, and the
 * package the metafile names must be the package on disk, so a stale store
 * cannot contribute another version's licence texts.
 */
export function bundlePackagesProblem(): string | null {
  const dir = mkdtempSync(path.join(tmpdir(), "passquest-bundle-packages-"));
  try {
    const metafile = path.join(dir, "durable-host.json");
    const read = (text: string): { packages: BundlePackage[]; message: string } => {
      writeFileSync(metafile, text);
      try {
        return { packages: bundlePackages(metafile, dir), message: "" };
      } catch (error) {
        return { packages: [], message: error instanceof Error ? error.message : String(error) };
      }
    };
    const manifest = (body: string): void => {
      const pkg = path.join(dir, "node_modules", "fixture-pkg");
      mkdirSync(pkg, { recursive: true });
      writeFileSync(path.join(pkg, "package.json"), body);
    };
    manifest('{"name": "fixture-pkg", "version": "1.2.3"}\n');
    // The host script and the generated entry live outside node_modules: only
    // the package input is a package.
    const good = read(JSON.stringify({ inputs: { "node_modules/fixture-pkg/index.mjs": {}, "agent/durable-host.mjs": {} } }));
    const [only] = good.packages;
    const root = path.join(dir, "node_modules", "fixture-pkg");
    if (
      good.message !== "" ||
      good.packages.length !== 1 ||
      only === undefined ||
      only.name !== "fixture-pkg" ||
      only.version !== "1.2.3" ||
      only.root !== root
    ) {
      return `the metafile's package is read as ${JSON.stringify(good.packages)} (${good.message}), want fixture-pkg@1.2.3 at its own directory`;
    }
    const refusals: Array<[string, string, string]> = [
      ["a metafile that is not JSON", "not json at all\n", "cannot be read as the bundle's metafile"],
      ["a metafile that names no inputs", '{"outputs": {}}\n', "names no inputs"],
      ["an input inside node_modules with no package name", JSON.stringify({ inputs: { "node_modules/": {} } }), "names no package"],
      ["an input whose package.json declares another package", JSON.stringify({ inputs: { "node_modules/claimed/index.mjs": {} } }), "does not sit in the package it claims"],
      ["an input whose package the tree does not carry", JSON.stringify({ inputs: { "node_modules/absent/index.mjs": {} } }), "a bundle inlined a package the tree does not carry"],
    ];
    const claimed = path.join(dir, "node_modules", "claimed");
    mkdirSync(claimed, { recursive: true });
    writeFileSync(path.join(claimed, "package.json"), '{"name": "other", "version": "1.0.0"}\n');
    const withoutName = path.join(dir, "node_modules", "nameless");
    mkdirSync(withoutName, { recursive: true });
    writeFileSync(path.join(withoutName, "package.json"), "{}\n");
    refusals.push(["an input whose package.json declares no name and version", JSON.stringify({ inputs: { "node_modules/nameless/index.mjs": {} } }), "declares no name and version"]);
    for (const [what, text, expected] of refusals) {
      const { message } = read(text);
      if (message === "") return `the bundle's packages accept ${what}`;
      if (!message.includes(expected)) return `the refusal of ${what} does not say ${JSON.stringify(expected)}: ${message}`;
    }
    if (process.platform === "win32") {
      // Bun writes `<ups>/C:/path` for a module reached through a junction onto
      // another drive; only the drive-qualified tail names the real file.
      const drive = dir.slice(0, 2);
      const tail = `${drive}/${dir.slice(2).split(path.sep).join("/")}`;
      const crossDrive = read(JSON.stringify({ inputs: { [`${"../".repeat(20)}${tail}/node_modules/fixture-pkg/index.mjs`]: {} } }));
      if (crossDrive.message !== "" || crossDrive.packages.length !== 1 || crossDrive.packages[0]?.root !== root) {
        return `a bun metafile key crossing drives is not read back to its package: ${crossDrive.message || JSON.stringify(crossDrive.packages)}`;
      }
      const absentTail = read(JSON.stringify({ inputs: { [`${"../".repeat(20)}${drive}/no-such-tree/node_modules/fixture-pkg/index.mjs`]: {} } }));
      if (!absentTail.message.includes("a bundle inlined a package the tree does not carry")) {
        return `a drive-qualified key naming no file is not refused as missing: ${absentTail.message || "it was accepted"}`;
      }
    }
    return null;
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

/**
 * The inlined set is not empty (goal 0016 P4): a bundled host that names a
 * package must have inlined at least one, or the `bun.lock` check and the
 * notices assembly would both pass over an empty set — the package would ship
 * the packages' code with no notice at all. A host that names only files it
 * carries itself (a relative or absolute path, a `file:` url, a `#name` a
 * manifest maps) goes through no node_modules, and the guard must leave it
 * alone. Staging applies this beside the lock check (package.ts, bundleHost).
 */
export function inlinedSetProblem(): string | null {
  const pkg = (name: string, version: string): BundlePackage => ({ name, version, root: "" });
  const specifiers = ["openai", "@earendil-works/chord/context"];
  const empty = emptyInlinedSetProblem("durable-host.mjs", specifiers, []);
  if (empty === undefined || !empty.includes("durable-host.mjs") || !empty.includes("inlined no package")) {
    return `a bundled host that inlined no package of the two it names is accepted (${empty ?? "no problem"})`;
  }
  for (const specifier of specifiers) {
    if (!empty.includes(JSON.stringify(specifier))) return `the empty-set refusal does not name ${JSON.stringify(specifier)}: ${empty}`;
  }
  if (emptyInlinedSetProblem("durable-host.mjs", ["openai"], [pkg("openai", "7.19.0")]) !== undefined) {
    return "a bundle that inlined the package its host names is refused by the empty-set guard";
  }
  if (emptyInlinedSetProblem("helper-host.mjs", ["./helper.mjs", "#internal", "file:./x.mjs", "C:/x.mjs"], []) !== undefined) {
    return "a host that names only files is refused by the empty-set guard";
  }
  return null;
}

/**
 * The installed notices check (goal 0016 P4): the scan refuses a missing
 * THIRD-PARTY-NOTICES.txt and a file that lost one of the sections staging
 * assembled — each section is required whole, its header and its text
 * together, so a file that kept the header still fails, and a body another
 * section shares cannot stand in for the one that lost it — and accepts a
 * file that carries every section. The fixture cases below drive the same
 * function through the gate's contents case over a fixture install tree, so
 * the scan's call site is covered too.
 */
export function noticesContentsProblem(): string | null {
  const dir = mkdtempSync(path.join(tmpdir(), "passquest-notices-contents-"));
  try {
    const file = path.join(dir, NOTICES_FILE);
    const required: StagedNotices = {
      packages: [{ name: "openai", version: "7.19.0" }],
      texts: [{ label: "openai@7.19.0 / LICENSE", text: "Apache License\nVersion 2.0, January 2004\n" }],
    };
    const notice = required.texts[0];
    const missing = noticesProblems(file, required).join("; ");
    if (!missing.includes("is missing")) return `the notices check accepts a missing ${NOTICES_FILE}: ${missing === "" ? "no problem" : missing}`;
    // Written by hand, the rule line included, so a check that drifted from
    // the format the assembly writes cannot pass by reading its own rule back.
    writeFileSync(file, ["Third-party notices", "", SECTION_RULE, notice.label, SECTION_RULE, notice.text].join("\n"));
    const clean = noticesProblems(file, required).join("; ");
    if (clean !== "") return `the notices check reports ${clean} for a file that carries the header and the text`;
    writeFileSync(file, `${notice.label}\n`);
    const lost = noticesProblems(file, required).join("; ");
    if (!lost.includes(`does not carry the text of ${notice.label}`)) {
      return `a file that kept the header but lost the text is not reported: ${lost === "" ? "no problem" : lost}`;
    }
    // Two sections can share one body — the SPDX fallback for two packages
    // with the same author, the shape several of the real file's sections
    // have — so a check that looked for the text anywhere would pass a file
    // whose first section lost its body while the second still carries it
    // verbatim. The section is required whole.
    const [twinFirst, twinSecond] = TWIN_NOTICES.required.texts;
    if (twinFirst === undefined || twinSecond === undefined || twinFirst.text !== twinSecond.text) {
      return "the twin fixture's two sections do not share one body, so the case below proves nothing";
    }
    writeFileSync(file, TWIN_NOTICES.whole);
    const twins = noticesProblems(file, TWIN_NOTICES.required).join("; ");
    if (twins !== "") return `the notices check reports ${twins} for a file carrying both twin sections whole`;
    writeFileSync(file, TWIN_NOTICES.oneBodyShort);
    const short = noticesProblems(file, TWIN_NOTICES.required).join("; ");
    if (!short.includes(`does not carry the text of ${twinFirst.label}`)) {
      return `a section whose body was deleted while an identical body stayed is not reported: ${short === "" ? "no problem" : short}`;
    }
    return null;
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

/** The 80-column rule the assembled notices file puts above and below every header (notices.ts, RULE); the fixtures write it literally. */
const SECTION_RULE = "=".repeat(80);

/** The two licence texts the notices fixtures carry, as the assembly writes them: the label it makes, and the file's text verbatim. */
const FIXTURE_LICENCES: readonly NoticeText[] = [
  { label: "openai@7.19.0 / LICENSE", text: "Apache License\nVersion 2.0, January 2004\nhttp://www.apache.org/licenses/\n" },
  { label: "partial-json@0.1.7 / LICENSE", text: "MIT License\n\nCopyright (c) 2023 Promplate Dev Team\n" },
];

/**
 * The notices a fixture install tree stands in for (goal 0016 P4): two
 * packages' licence texts, assembled by the real function over a fixture
 * source, so the contents-check cases drive `noticesProblems` through the
 * gate's own call site and not only as a unit. `whole` is the file staging
 * would have written; `deleted` is that file with the second text removed —
 * the loss the installed-file check exists for.
 */
function fixtureNotices(): { required: StagedNotices; whole: string; deleted: string } {
  const base = process.platform === "win32" ? "C:\\fixture" : "/fixture";
  const root = (...parts: string[]): string => path.join(base, "node_modules", ...parts);
  const [first, second] = FIXTURE_LICENCES;
  const openai = root("openai");
  const partial = root("partial-json");
  const files: Record<string, string> = {
    [path.join(openai, "package.json")]: JSON.stringify({ name: "openai", version: "7.19.0", license: "Apache-2.0" }),
    [path.join(openai, "LICENSE")]: first.text,
    [path.join(partial, "package.json")]: JSON.stringify({ name: "partial-json", version: "0.1.7", license: "MIT", author: "Promplate Dev Team" }),
    [path.join(partial, "LICENSE")]: second.text,
  };
  const whole = assembleNotices(
    [
      { name: "openai", version: "7.19.0", root: openai },
      { name: "partial-json", version: "0.1.7", root: partial },
    ],
    fixtureSource(files),
  ).text;
  return {
    required: { packages: [{ name: "openai", version: "7.19.0" }, { name: "partial-json", version: "0.1.7" }], texts: FIXTURE_LICENCES },
    whole,
    deleted: whole.split(second.text).join(""),
  };
}

/**
 * The notices set the contents-check fixtures stand in for, and the two files
 * they carry: the whole assembly output and one text short of it. Built at
 * module load, like the fixture file maps below.
 */
const FIXTURE_NOTICES = fixtureNotices();

/**
 * The notices a fixture install stands in for where two sections share one
 * body (goal 0016 P4): two packages that ship no licence file and declare the
 * same MIT author, so the SPDX fallback gives them byte-identical bodies
 * under different headers — the shape several of the real file's sections
 * have. The labels and the bodies come from the real assembly; the file's
 * bytes are written here, the rule line included. `oneBodyShort` is that file
 * with the first section's body deleted and its header kept, so that exact
 * text is still in the file under the second section's header: only a
 * whole-section check sees the loss.
 */
function twinNotices(): { required: StagedNotices; whole: string; oneBodyShort: string } {
  const base = process.platform === "win32" ? "C:\\fixture" : "/fixture";
  const root = (...parts: string[]): string => path.join(base, "node_modules", ...parts);
  const alpha = root("@earendil-works", "alpha");
  const beta = root("@earendil-works", "beta");
  const files: Record<string, string> = {
    [path.join(alpha, "package.json")]: JSON.stringify({ name: "@earendil-works/alpha", version: "1.5.2", license: "MIT", author: "Earendil Works" }),
    [path.join(beta, "package.json")]: JSON.stringify({ name: "@earendil-works/beta", version: "1.5.2", license: "MIT", author: "Earendil Works" }),
  };
  const assembly = assembleNotices(
    [
      { name: "@earendil-works/alpha", version: "1.5.2", root: alpha },
      { name: "@earendil-works/beta", version: "1.5.2", root: beta },
    ],
    fixtureSource(files),
  );
  const [first, second] = assembly.texts;
  const firstSection = `${SECTION_RULE}\n${first.label}\n${SECTION_RULE}\n`;
  const secondSection = `${SECTION_RULE}\n${second.label}\n${SECTION_RULE}\n`;
  const whole = ["Third-party notices", "", firstSection + first.text, secondSection + second.text].join("\n");
  return {
    required: { packages: assembly.packages, texts: assembly.texts },
    whole,
    oneBodyShort: whole.replace(firstSection + first.text, () => firstSection),
  };
}

/** The twin-body notices fixture the contents-check unit and the gate fixture case share. */
const TWIN_NOTICES = twinNotices();

/** The pin source texts a case hands the gate, in the shapes the repository's own files carry them. */
function pinSourceTexts(version: string): Record<string, string> {
  return {
    "mise.toml": `[tools]\nnode = "${version}"\n`,
    ".github/workflows/gate.yml": `- uses: actions/setup-node@v4\n  with:\n    node-version: "${version}"\n`,
    "docs/install.md": `notices the inlined code requires and a pinned Node ${version} runtime, so the agent features run on the\n`,
    "docs/install.zh-CN.md": `AppImage。每个包都自带桌面应用、四个 agent host 脚本、内联代码所需的第三方声明，以及固定版本的\nNode ${version} 运行时，所以 agent 功能用包内自带的 Node 运行\n`,
    "harness/lib/package.ts": ` * agent hosts, the pinned Node ${version} LTS runtime and the app icon. A host that\n`,
  };
}

/**
 * The responses brief over a copied product whose durable host drops the
 * responses branch: the tree is the installed copy — the repository CLI at the
 * app's path, the repository's own hosts and package store, and a real Node
 * runtime — so the case's seed configures the responses model through the real
 * CLI, and the patched host refuses the endpoint the installed run is carrying.
 */
async function dropTheResponsesBranch(root: string): Promise<string | null> {
  if (copyProduct(root, [...NOTE_SMOKE_SOURCES, "bun.lock", "mygo.json"]) === 0) {
    return "no product source was copied into the fixture root";
  }
  linkStore(root);
  const patch = patchCopy(root, "agent/durable-host.mjs", '  if (path.endsWith("/responses")) return "responses";\n', "");
  if (patch) return patch;
  // The installed entry passes its resources directory into cli.Run; a fixture
  // build is a dev build, so the fixture wires the tree itself (no PASSQUEST_* pin).
  writeFileSync(
    path.join(root, "cmd", "passquest-desktop", "main.go"),
    `package main

import (
	"os"
	"path/filepath"

	"github.com/wxxb789/passquest/internal/cli"
)

func main() {
	exe, _ := os.Executable()
	os.Exit(cli.Run(os.Args[1:], cli.Options{Resources: filepath.Dir(exe)}))
}
`,
  );
  const node = Bun.which("node");
  if (node === null) return "node is not on PATH";
  const runtime = path.join(root, "node", process.platform === "win32" ? "node.exe" : path.join("bin", "node"));
  mkdirSync(path.dirname(runtime), { recursive: true });
  copyFileSync(node, runtime);
  if (process.platform !== "win32") chmodSync(runtime, 0o755);
  const build = await run(executable("go"), ["build", "-o", path.join(root, executable("passquest-desktop")), "./cmd/passquest-desktop"], { cwd: root });
  return build.code === 0 ? null : `the fixture CLI did not build: ${build.stderr || build.stdout}`;
}

export function packageSmokeCases(): BrokenCase[] {
  return [
    {
      gate: makePackageSmokeGate({
        installedTree: "build/fixture",
        only: ["contents"],
        nodePinSources: {
          ...pinSourceTexts(NODE_VERSION),
          "mise.toml": `[tools]\nnode = "22"\n`,
          "docs/install.md": "notices the inlined code requires and a pinned Node 22.23.5 runtime, so the agent features run on the\n",
        },
      }),
      label: "package whose pinned Node runtime disagrees with two sources that carry it",
      files: { ...FIXTURE_FILES },
      postFail: (failures) => {
        const expected = [
          `mise.toml: pinned Node reads 22, want ${NODE_VERSION}`,
          `docs/install.md: pinned Node reads 22.23.5, want ${NODE_VERSION}`,
        ];
        const missing = expected.filter((line) => !failures.includes(line));
        if (missing.length > 0) {
          return `package-smoke: the pin check did not name ${missing.join("; ")} (${failures.slice(0, 3).join("; ")})`;
        }
        const extra = failures.filter((line) => !expected.includes(line));
        if (extra.length > 0) return `package-smoke: the pin check reported a source that agrees (${extra.slice(0, 3).join("; ")})`;
        return null;
      },
    },
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
      gate: makePackageSmokeGate({ installedTree: "build/fixture", only: ["cli-runtime"] }),
      label: "installed CLI whose data root is not the app's per-user directory",
      files: {
        ...FIXTURE_FILES,
        // The installed binary in CLI mode keeps its database in its own
        // directory and says so in the notice's own words: the case must fail
        // the both-directions check, naming the directory it resolved beside
        // the per-user root the app's resolution produces.
        "cmd/passquest-desktop/main.go": `package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) < 2 {
		return
	}
	exe, _ := os.Executable()
	dir := filepath.Join(filepath.Dir(exe), "data")
	_ = os.MkdirAll(dir, 0o700)
	target := filepath.Join(dir, "passquest.sqlite")
	_ = os.WriteFile(target, []byte("fixture"), 0o600)
	fmt.Fprintf(os.Stderr, "passquest: a database from the old default location exists at %s; the command line now uses %s. Pass Quest will not move it; move or merge it yourself.\\n",
		filepath.Join(filepath.Dir(exe), ".passquest", "passquest.sqlite"), target)
	fmt.Println(\`{"state":"verified","result":"verified","provider":"stub"}\`)
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
        failures.some(
          (line) => line.startsWith("cli-runtime: the installed CLI resolved its database elsewhere") && line.includes("the app's per-user root holds"),
        )
          ? null
          : `package-smoke: did not report the CLI's own data root: ${failures.slice(0, 3).join("; ")}`,
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
      gate: makePackageSmokeGate({ installedTree: "build/fixture", only: ["brief"] }),
      label: "installed brief paused before its durable work was enqueued",
      files: {
        ...FIXTURE_FILES,
        // A fixture cannot run the real host, so the installed binary answers
        // the case's own calls: the seeding CLI accepts everything, and the
        // installed binary reports the shape a pause that lands before the
        // durable submit produces — a paused run whose status snapshot carries
        // no pending work. The brief case must refuse to resume it: that
        // snapshot is what the window's Resume relies on, so the case stops at
        // that precondition and the window never launches — an installed copy
        // needs no window behaviour here. The fixture never reaches the stub,
        // so the case asks for no pause either: lib/host-control.ts pauses a
        // live host only once the stub has recorded its request.
        "cmd/passquest/main.go": `package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "import" {
		fmt.Println("{\\"source\\":{\\"id\\":\\"fixture-source\\"}}")
		return
	}
	fmt.Println("{}")
}
`,
        "cmd/passquest-desktop/main.go": `package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		return
	}
	switch os.Args[2] {
	case "start":
		fmt.Println(\`{"state":"paused","run":{"run_id":"run-20261120T000000Z-00000000"}}\`)
		os.Exit(4)
	case "status":
		fmt.Println(\`{"state":"paused","durable":{"snapshot":{"pending_tasks":0,"unsettled_submissions":0}}}\`)
	}
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
        failures.some((line) => line.startsWith("brief: the paused run left no pending durable work"))
          ? null
          : `package-smoke: did not report the paused run without durable work: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      gate: makePackageSmokeGate({ installedTree: "build/fixture", only: ["brief"] }),
      label: "installed brief whose durable host is missing",
      files: {
        ...FIXTURE_FILES,
        // The gate builds the repository CLI before the brief case runs; the
        // case stops before any CLI call, so a stub is enough.
        "cmd/passquest/main.go": "package main\n\nfunc main() {}\n",
      },
      prepare: async (root) => {
        // A copy that lost agent/durable-host.mjs: the case must name the
        // missing file itself (goal 0016 P3's guard, reached here) instead of
        // leaving it to a resolution error that would list the cwd's and the
        // exe's neighbours.
        rmSync(path.join(root, "build", "fixture", "agent", "durable-host.mjs"), { force: true });
        const runtime = fixtureRuntime(root);
        mkdirSync(path.dirname(runtime), { recursive: true });
        writeFileSync(runtime, Buffer.from([...runtimeMagic(), 0, 0, 0, 0]));
        return null;
      },
      postFail: (failures) =>
        failures.some((line) => line.startsWith("brief: the installed app carries no agent/durable-host.mjs"))
          ? null
          : `package-smoke: did not report the missing durable host by name: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      gate: makePackageSmokeGate({ installedTree: ".", only: ["brief-responses"] }),
      label: "installed Responses brief whose durable host drops the responses branch",
      files: {},
      prepare: dropTheResponsesBranch,
      postFail: (failures) =>
        failures.some(
          (line) =>
            line.startsWith("brief-responses: the installed run did not pause") &&
            line.includes("model endpoint must end in /chat/completions or /responses"),
        )
          ? null
          : `package-smoke: did not report the Responses refusal by name: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      gate: makePackageSmokeGate({ installedTree: "build/fixture", only: ["contents"] }),
      label: "installed agent tree carrying node_modules",
      files: {
        ...FIXTURE_FILES,
        // A package ships no third-party package at all (goal 0016): the
        // contents scan's lean-tree check must name the directory.
        "build/fixture/agent/node_modules/openai/package.json": `{"name": "openai", "version": "0.0.0"}\n`,
      },
      postFail: (failures) =>
        failures.some((line) => slashed(line).includes("agent/node_modules") && line.includes("directory"))
          ? null
          : `package-smoke: did not report the shipped node_modules: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      gate: makePackageSmokeGate({ installedTree: "build/fixture", only: ["contents"] }),
      label: "installed agent host naming a package specifier",
      files: {
        ...FIXTURE_FILES,
        // Every host ships as one self-contained bundle (goal 0016): a staged
        // host that still names a package is a tree that cannot run where it
        // lands, and no node_modules travels beside it.
        "build/fixture/agent/durable-host.mjs": `import { OpenAI } from "openai";\n`,
      },
      postFail: (failures) =>
        failures.some((line) => slashed(line).includes("agent/durable-host.mjs") && line.includes(`names the specifier "openai"`))
          ? null
          : `package-smoke: did not report the host's package specifier: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      gate: makePackageSmokeGate({ installedTree: "build/fixture", only: ["contents"] }),
      label: "installed agent file embedding a machine path",
      files: {
        ...FIXTURE_FILES,
        // bun writes each module's path as a comment at the top of the
        // bundle; an entry outside the checkout made it absolute, so the
        // shipped file named the build machine's account and temp directory
        // (goal 0016's review, the leak this case exists for).
        "build/fixture/agent/durable-host.mjs": `// C:/Users/runneradmin/AppData/Local/Temp/passquest-agent-entry-abc123/entry.mjs\n`,
      },
      postFail: (failures) =>
        failures.some((line) => slashed(line).includes("agent/durable-host.mjs") && line.includes("embeds the machine path"))
          ? null
          : `package-smoke: did not report the machine path in the installed host: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      gate: makePackageSmokeGate({ installedTree: "build/fixture", only: ["contents"], notices: FIXTURE_NOTICES.required }),
      label: "installed notices file that lost a licence text",
      files: {
        ...FIXTURE_FILES,
        // The file staging assembled, with one inlined package's own licence
        // text deleted: the header survives, so only the body's absence can
        // catch it (goal 0016 P4).
        "build/fixture/agent/THIRD-PARTY-NOTICES.txt": FIXTURE_NOTICES.deleted,
      },
      postFail: (failures) =>
        failures.some((line) => line.includes(NOTICES_FILE) && line.includes("does not carry the text of partial-json@0.1.7 / LICENSE"))
          ? null
          : `package-smoke: did not report the licence text missing from the installed notices file: ${failures.slice(0, 3).join("; ")}`,
    },
    {
      gate: makePackageSmokeGate({ installedTree: "build/fixture", only: ["contents"], notices: TWIN_NOTICES.required }),
      label: "installed notices file that lost one of two identical licence texts",
      files: {
        ...FIXTURE_FILES,
        // Two of the installed file's sections share one body, as several of
        // the real file's do: the first section's body is deleted and its
        // header kept, so that exact text is still in the file under the
        // second section's header, and only a whole-section check sees the
        // loss (goal 0016 P4).
        "build/fixture/agent/THIRD-PARTY-NOTICES.txt": TWIN_NOTICES.oneBodyShort,
      },
      postFail: (failures) =>
        failures.some((line) => line.includes(NOTICES_FILE) && line.includes(`does not carry the text of ${TWIN_NOTICES.required.texts[0]!.label}`))
          ? null
          : `package-smoke: did not report the section whose body was deleted while an identical body stayed: ${failures.slice(0, 3).join("; ")}`,
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
    {
      gate: makePackageSmokeGate({ installedTree: "build/fixture", only: ["contents"] }),
      label: "installed bundle whose Info.plist kept an older minimum than mygo.json declares",
      files: {
        ...FIXTURE_FILES,
        // MyGo copies the declared floor into LSMinimumSystemVersion verbatim,
        // and the effective floor is the higher of that key and the Mach-O's
        // minos — 13.0 from the Go 1.27 linker — so the key binds and an older
        // value would open on a macOS between the linker's minos and the
        // declared floor (goal 0017 P1). The failure must name both values.
        "mygo.json": FIXTURE_MYGO_MACOS,
        "build/fixture/Contents/Info.plist": fixturePlist("13.0"),
      },
      postFail: (failures) => {
        const line = failures.map(slashed).find((entry) => entry.includes("Contents/Info.plist"));
        if (line === undefined || !line.includes('"13.0"') || !line.includes('"26.0"')) {
          return `package-smoke: the content scan did not report the plist minimum against both values: ${failures.slice(0, 3).join("; ")}`;
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
      label: "third-party file outside the agent tree with a credential-shaped string",
      files: {
        ...FIXTURE_FILES,
        "build/fixture/vendor/@vendor/sdk/index.mjs": `const key = "${fakeKey()}";\n`,
      },
      postPass: (detail) =>
        detail.includes("contents scan:")
          ? null
          : `package-smoke: the content scan completed without a scan line (${detail})`,
    },
    {
      gate: makePackageSmokeGate({ installedTree: "build/fixture", only: ["contents"] }),
      label: "installed agent host carrying urls and clock-shaped text",
      files: {
        ...FIXTURE_FILES,
        // The machine-path scan reads drive paths and user or temp
        // directories, not a colon after any text: a host's urls and time
        // strings are ordinary content (goal 0016).
        "build/fixture/agent/search-host.mjs": `const base = "https://example.com/v1";\nconst at = "12:30";\n`,
      },
      postPass: (detail) =>
        detail.includes("contents scan:")
          ? null
          : `package-smoke: the content scan completed without a scan line (${detail})`,
    },
    {
      gate: makePackageSmokeGate({ installedTree: "build/fixture", only: ["contents"], notices: FIXTURE_NOTICES.required }),
      label: "installed notices file carrying every licence text",
      files: {
        ...FIXTURE_FILES,
        // The same file the broken case carries one text short of: the
        // contents check must accept it, and its failure there must be the
        // missing text and nothing else (goal 0016 P4).
        "build/fixture/agent/THIRD-PARTY-NOTICES.txt": FIXTURE_NOTICES.whole,
      },
      postPass: (detail) =>
        detail.includes("contents scan:")
          ? null
          : `package-smoke: the content scan completed without a scan line (${detail})`,
    },
    {
      gate: makePackageSmokeGate({ installedTree: "build/fixture", only: ["contents"] }),
      label: "installed bundle whose Info.plist declares the minimum mygo.json states",
      files: {
        ...FIXTURE_FILES,
        // The counterpart of the broken case above (goal 0017 P1): the scan
        // must accept the declared floor, and its pass line names the value it
        // checked, so a check that stopped running shows here too.
        "mygo.json": FIXTURE_MYGO_MACOS,
        "build/fixture/Contents/Info.plist": fixturePlist("26.0"),
      },
      postPass: (detail) =>
        detail.includes("Contents/Info.plist declares LSMinimumSystemVersion 26.0")
          ? null
          : `package-smoke: the content scan did not report the checked plist minimum (${detail})`,
    },
    {
      gate: makePackageSmokeGate({ installedTree: "build/fixture", only: ["contents"] }),
      label: "installed bundle whose nested app Info.plist kept an older minimum",
      files: {
        ...FIXTURE_FILES,
        // The scan reads the bundle at the tree's root only (goal 0017 P1):
        // the root plist carries the declared floor, and the nested
        // `Helpers/X.app` plist, which is not the installed app's, must not
        // fail the tree even though its value is older.
        "mygo.json": FIXTURE_MYGO_MACOS,
        "build/fixture/Contents/Info.plist": fixturePlist("26.0"),
        "build/fixture/Helpers/X.app/Contents/Info.plist": fixturePlist("13.0"),
      },
      postPass: (detail) =>
        detail.includes("Contents/Info.plist declares LSMinimumSystemVersion 26.0")
          ? null
          : `package-smoke: the content scan did not report the checked plist minimum (${detail})`,
    },
    {
      gate: makePackageSmokeGate({ installedTree: "build/fixture", only: ["contents"], nodePinSources: pinSourceTexts(NODE_VERSION) }),
      label: "package whose pinned Node runtime agrees with every source that carries it",
      files: { ...FIXTURE_FILES },
      postPass: (detail) =>
        detail.includes("node pin:") && detail.includes(`agree on Node ${NODE_VERSION}`)
          ? null
          : `package-smoke: the pass line does not report the pin check (${detail})`,
    },
  ];
}
