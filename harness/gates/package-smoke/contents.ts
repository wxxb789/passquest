/**
 * The package content scan (goal 0013, step P6). It runs as the package
 * smoke's first case, over the tree the gate just installed and before any
 * launch, so the tree holds exactly what the package shipped:
 *
 * - Windows: the NSIS-installed prefix, `Uninstall.exe` and MyGo's
 *   `.mygo-files` included;
 * - macOS: the `.app` copied out of the mounted image, plus the mounted
 *   root's listing captured at install time — the `.app`, the `/Applications`
 *   link and the `.DS_Store`/`.VolumeIcon.icns` files are all cmd/mygo/dmg.go
 *   puts there;
 * - Linux: the `--appimage-extract` tree, and the prefix holding the AppImage.
 *
 * Every file of the payload fails on:
 *
 * - a PDF: `%PDF-` within the first 1024 bytes, the span the format allows its
 *   header to sit in, whatever the extension. The whole file is not searched:
 *   this app's own binary carries net/http's content-sniff table — `%PDF-`
 *   17 MB in, because cmd/passquest-desktop links net/http through MyGo — so
 *   a whole-buffer search would fail a legitimate payload;
 * - SQLite: the `SQLite format 3\0` magic at byte 0, where a database file
 *   carries it, whatever the extension. Not searched further in: the bundled
 *   node.exe carries its built-in node:sqlite's format string the same way
 *   the app binary carries the sniff table;
 * - a private path: any path segment equal to `data/private`, `data/local`,
 *   `.passquest` or `local-data`, compared case-insensitively on the
 *   `/`/`\`-normalised relative path;
 * - `.env`: a file named `.env` or `.env.*` (case-insensitively; the staged
 *   node_modules holds none today, and this check does not exempt one);
 * - the internal-only marker: the bytes of `CONFIDENTIAL_MARKER_TEXTS` in the
 *   file — UTF-8 text, in binaries too;
 * - the baseline text: the scan reads the committed baseline the `baseline`
 *   gate verifies (`data/baselines/2026-crsc-english`) at run time and fails
 *   on a payload file byte-identical (SHA-256) to a file there, or holding a
 *   whole line — CR/LF-normalised, 40 bytes or longer, no longer than the
 *   baseline's longest line — equal to a line of one of its page files.
 *   Nothing of the baseline is copied into the harness: the needles are
 *   computed from the committed files at run time, and a failure names the
 *   file, never the text. A tree without the baseline (a self-test fixture)
 *   has nothing to match, which the scan line states.
 *
 * The credential patterns the secrets gate carries apply to project-owned
 * files only, through `scanFiles` (its patterns and its binary skip, not a
 * copy): the four bundled `agent/*.mjs` hosts, the metadata MyGo generates
 * from mygo.json as files — Linux's `<slug>.desktop`, macOS's
 * `Contents/Info.plist`; Windows writes no such file, the exe carries the
 * version info and the installer's strings live in the registry — and the
 * frontend dist this build embedded (the same bytes sit in the app binary,
 * which is not credential-scanned). The bundled Node runtime and
 * `node_modules` are not credential-scanned: they legitimately contain
 * published credential-shaped examples (the AWS SDK type doc the plan
 * scouted), and the check must not be weakened per file to pass.
 *
 * The run's package artifact — the NSIS setup, the `.dmg` or the AppImage —
 * is checked cheaply: its name must be the exact name this build writes
 * (`mygoFileName` / `appImageFileName`), and its first 1024 bytes must not be
 * a PDF or database header. A compressed container is not byte-scanned
 * further: its extracted tree is what the rules above are checked against,
 * and the installed copy of the container inside an install tree (the
 * AppImage on Linux) is skipped the same way.
 */
import { closeSync, existsSync, openSync, readFileSync, readdirSync, readlinkSync, readSync } from "node:fs";
import { createHash } from "node:crypto";
import path from "node:path";

import { appImageFileName } from "../../lib/appimage.ts";
import { BASELINE } from "../../lib/baseline.ts";
import { FRONTEND_DIST } from "../../lib/desktop.ts";
import type { Check } from "../../lib/desktop-page.ts";
import { mygoFileName } from "../../lib/package-build.ts";
import type { PackageArtifact, PackageNaming } from "../../lib/package-build.ts";
import { size } from "../../lib/package-build.ts";
import { goTarget, hostTarget } from "../../lib/package.ts";
import { resolve } from "../../lib/repo.ts";
import type { GateContext } from "../../lib/types.ts";
import { CONFIDENTIAL_MARKER_TEXTS } from "../local-only.ts";
import { scanFiles } from "../secrets.ts";
import type { InstalledApp } from "./install.ts";

/** What a real run knows about the package it built; a fixture install tree carries none of it. */
export interface PackageIdentity {
  artifact: PackageArtifact;
  naming: PackageNaming;
  /** mygo.json as written: the AppImage's name comes from its raw name and version. */
  config: { name: string; version: string };
}

const PDF = Buffer.from("%PDF-", "latin1");
const SQLITE = Buffer.from("SQLite format 3\0", "latin1");
/** The span the PDF format allows the header to sit in, and the part of a container the scan reads. */
const HEAD_BYTES = 1024;
/**
 * A line shorter than this — in UTF-8 bytes, the unit both sides are measured
 * in — is too common to stand for the baseline; a line beyond the baseline's
 * longest line cannot be one. A CJK line is three bytes per character, so the
 * units must match or such a line would never be looked up.
 */
const SHORTEST_LINE = 40;
/** The marker texts as bytes: a CJK marker inside a binary is these bytes. */
const MARKERS = CONFIDENTIAL_MARKER_TEXTS.map((text) => Buffer.from(text, "utf8"));

/** The needle set computed from the committed baseline; null where the tree carries no baseline (a fixture). */
interface BaselineSource {
  /** The baseline-relative file -> its SHA-256, for payload files copied whole. */
  hashes: Map<string, string>;
  /** One whole line of a page file -> the page file it came from. */
  lines: Map<string, string>;
  /** The longest page-file line, in bytes. */
  longest: number;
  /** How many baseline files the needles came from, for the scan line. */
  files: number;
}

/**
 * The needles the source of truth gives: every file under the baseline
 * directory hashed for whole-file copies, and the lines of its page files
 * (`pages/page-*.txt`, the naming the baseline gate checks) for text embedded
 * in a bigger file.
 */
function baselineSource(root: string): BaselineSource | null {
  const dir = resolve(root, BASELINE);
  if (!existsSync(dir)) return null;
  const source: BaselineSource = { hashes: new Map(), lines: new Map(), longest: 0, files: 0 };
  const walk = (current: string, prefix: string): void => {
    for (const entry of readdirSync(current, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
      if (!entry.isFile() && !entry.isDirectory()) continue;
      const full = path.join(current, entry.name);
      const relative = prefix === "" ? entry.name : `${prefix}/${entry.name}`;
      if (entry.isDirectory()) {
        walk(full, relative);
        continue;
      }
      const data = readFileSync(full);
      source.hashes.set(createHash("sha256").update(data).digest("hex"), relative);
      source.files += 1;
      if (!/^pages\/page-\d+\.txt$/.test(relative)) continue;
      for (const line of linesOf(data)) {
        const length = Buffer.byteLength(line, "utf8");
        if (length < SHORTEST_LINE) continue;
        source.lines.set(line, relative);
        if (length > source.longest) source.longest = length;
      }
    }
  };
  walk(dir, "");
  return source;
}

/** A file's lines, CR/LF-normalised; invalid UTF-8 decodes with U+FFFD, which no baseline line holds. */
function* linesOf(data: Buffer): Generator<string> {
  let start = 0;
  for (;;) {
    const end = data.indexOf(0x0a, start);
    const stop = end === -1 ? data.length : end;
    let line = data.subarray(start, stop);
    if (line.length > 0 && line[line.length - 1] === 0x0d) line = line.subarray(0, -1);
    yield line.toString("utf8");
    if (end === -1) return;
    start = end + 1;
  }
}

/** The first payload line that is a line of a baseline page file, or ""; the length bounds keep binaries cheap. */
function baselineLine(data: Buffer, source: BaselineSource): string {
  let start = 0;
  for (;;) {
    const end = data.indexOf(0x0a, start);
    const stop = end === -1 ? data.length : end;
    let line = data.subarray(start, stop);
    if (line.length > 0 && line[line.length - 1] === 0x0d) line = line.subarray(0, -1);
    if (line.length >= SHORTEST_LINE && line.length <= source.longest) {
      const page = source.lines.get(line.toString("utf8"));
      if (page !== undefined) return page;
    }
    if (end === -1) return "";
    start = end + 1;
  }
}

/** The file's first bytes, bounded: a container's header, not the whole compressed image. */
function headOf(file: string): Buffer {
  const fd = openSync(file, "r");
  try {
    const head = Buffer.alloc(HEAD_BYTES);
    const read = readSync(fd, head, 0, HEAD_BYTES, 0);
    return head.subarray(0, read);
  } finally {
    closeSync(fd);
  }
}

/** The container rules: the artifact itself, and its installed copy, must be neither a PDF nor a database. */
function containerProblems(file: string, data: Buffer): string[] {
  const problems: string[] = [];
  if (data.subarray(0, HEAD_BYTES).includes(PDF)) problems.push(`${file}: PDF header in the first ${HEAD_BYTES} bytes`);
  if (data.length >= SQLITE.length && data.subarray(0, SQLITE.length).equals(SQLITE)) {
    problems.push(`${file}: SQLite database header at byte 0`);
  }
  return problems;
}

/**
 * The first private path segment of a relative path, or "": `/` and `\` are
 * normalised before the split, and the comparison is case-insensitive the way
 * Windows and macOS compare paths.
 */
function privateSegment(relative: string): string {
  const segments = relative.replace(/\\/g, "/").toLowerCase().split("/");
  for (let index = 0; index < segments.length; index++) {
    const segment = segments[index]!;
    if (segment === ".passquest" || segment === "local-data") return segment;
    if (index + 1 < segments.length) {
      const pair = `${segment}/${segments[index + 1]}`;
      if (pair === "data/private" || pair === "data/local") return pair;
    }
  }
  return "";
}

/** Every forbidden-content problem of one payload file. */
function payloadProblems(file: string, relative: string, data: Buffer, baseline: BaselineSource | null): string[] {
  const problems = containerProblems(file, data);
  const segment = privateSegment(relative);
  if (segment !== "") problems.push(`${file}: private path segment ${JSON.stringify(segment)}`);
  const base = relative.replace(/\\/g, "/").split("/").pop()!.toLowerCase();
  if (base === ".env" || base.startsWith(".env.")) problems.push(`${file}: environment file`);
  for (const marker of MARKERS) {
    if (data.includes(marker)) {
      problems.push(`${file}: internal-only source marker`);
      break;
    }
  }
  if (baseline !== null) {
    const copy = baseline.hashes.get(createHash("sha256").update(data).digest("hex"));
    if (copy !== undefined) problems.push(`${file}: baseline text, byte-identical to ${BASELINE}/${copy}`);
    else {
      const page = baselineLine(data, baseline);
      if (page !== "") problems.push(`${file}: baseline text, a line of ${BASELINE}/${page}`);
    }
  }
  return problems;
}

/** The path of file within dir, slashed, or "" when file does not resolve inside dir. */
function relativeWithin(dir: string, file: string): string {
  const relative = path.relative(dir, file);
  if (relative === "" || relative.startsWith("..") || path.isAbsolute(relative)) return "";
  return relative.split(path.sep).join("/");
}

/** Every file under a directory, absolute, sorted. */
function filesUnder(dir: string): string[] {
  const found: string[] = [];
  const walk = (current: string): void => {
    for (const entry of readdirSync(current, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
      const full = path.join(current, entry.name);
      if (entry.isDirectory()) walk(full);
      else if (entry.isFile()) found.push(full);
    }
  };
  if (existsSync(dir)) walk(dir);
  return found;
}

/**
 * Scans the installed package's contents and reports every problem through
 * `check`; returns the scan line (file count, bytes, needles, wall time) for
 * the gate's pass detail.
 */
export async function scanPackageContents(
  app: InstalledApp,
  ctx: GateContext,
  check: Check,
  identity?: PackageIdentity,
): Promise<string> {
  const started = performance.now();
  const baseline = baselineSource(ctx.root);
  let files = 0;
  let bytes = 0;

  for (const dir of app.installDirs) {
    const walk = (current: string, prefix: string): void => {
      for (const entry of readdirSync(current, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
        const full = path.join(current, entry.name);
        const relative = prefix === "" ? entry.name : `${prefix}/${entry.name}`;
        // A link carries no bytes and is not followed (the AppImage's own links
        // are checked by the image checks, and MyGo writes no link into the
        // other trees), but its name and its target are still paths: neither
        // may name private data or an environment file.
        if (entry.isSymbolicLink()) {
          for (const named of [relative, readlinkSync(full)]) {
            const segment = privateSegment(named);
            if (segment !== "") check(false, `contents: ${full}: a link whose path names private data ${JSON.stringify(segment)}`);
          }
          const base = entry.name.toLowerCase();
          if (base === ".env" || base.startsWith(".env.")) check(false, `contents: ${full}: environment file (a link)`);
          continue;
        }
        if (entry.isDirectory()) {
          walk(full, relative);
          continue;
        }
        if (!entry.isFile()) continue;
        // The installed copy of this run's container (the AppImage on Linux)
        // is not byte-scanned: the artifact checks own its name and header,
        // and its extracted tree carries the payload.
        if (prefix === "" && identity !== undefined && entry.name === identity.artifact.name) {
          for (const problem of containerProblems(full, headOf(full))) check(false, `contents: ${problem}`);
          continue;
        }
        let data: Buffer;
        try {
          data = readFileSync(full);
        } catch (error) {
          check(false, `contents: ${full} cannot be read: ${error instanceof Error ? error.message : String(error)}`);
          continue;
        }
        files += 1;
        bytes += data.length;
        for (const problem of payloadProblems(full, relative, data, baseline)) check(false, `contents: ${problem}`);
      }
    };
    walk(dir, "");
  }

  // The credential patterns apply to the project-owned files only: the four
  // bundled hosts, the metadata MyGo writes from mygo.json as files, and the
  // frontend dist this build embedded.
  const owned: Map<string, string[]> = new Map();
  const add = (root: string, file: string): void => {
    const relative = relativeWithin(root, file);
    if (relative === "") return;
    const list = owned.get(root) ?? [];
    list.push(relative);
    owned.set(root, list);
  };
  for (const host of app.hosts) {
    for (const dir of app.installDirs) {
      if (relativeWithin(dir, host) !== "") {
        add(dir, host);
        break;
      }
    }
  }
  if (app.hosts.length > 0) {
    const resourceRoot = path.dirname(path.dirname(app.hosts[0]!));
    if (existsSync(resourceRoot)) {
      for (const entry of readdirSync(resourceRoot, { withFileTypes: true })) {
        if (entry.isFile() && entry.name.endsWith(".desktop")) add(resourceRoot, path.join(resourceRoot, entry.name));
      }
    }
    if (process.platform === "darwin") {
      // The app bundle's Info.plist: MyGo's bundle writer puts it beside MacOS/.
      const contents = path.dirname(path.dirname(app.binary));
      const plist = path.join(contents, "Info.plist");
      if (existsSync(plist)) add(contents, plist);
    }
  }
  const dist = resolve(ctx.root, FRONTEND_DIST);
  for (const file of filesUnder(dist)) add(ctx.root, file);
  let ownedFiles = 0;
  for (const [root, group] of owned) {
    ownedFiles += group.length;
    for (const problem of await scanFiles(root, group)) check(false, `contents: ${problem}`);
  }

  if (identity !== undefined) {
    const { artifact, naming, config } = identity;
    const target = hostTarget();
    const expected = target.platform === "linux" ? appImageFileName(config) : mygoFileName(naming, target.platform, goTarget(target).split("/")[1]!);
    if (expected !== undefined && artifact.name !== expected) {
      check(false, `contents: the package artifact is ${artifact.name}, want ${expected}`);
    }
    let head: Buffer;
    try {
      head = headOf(artifact.path);
    } catch (error) {
      check(false, `contents: the package artifact ${artifact.path} cannot be read: ${error instanceof Error ? error.message : String(error)}`);
      head = Buffer.alloc(0);
    }
    for (const problem of containerProblems(artifact.name, head)) check(false, `contents: ${problem}`);
  }

  if (app.dmgRoot !== undefined) {
    // cmd/mygo/dmg.go copies the .app and writes the /Applications link and
    // the .DS_Store; with an icon it also puts .VolumeIcon.icns there. Any
    // other entry is a file the image must not hold.
    const installed = path.relative(app.installDirs[0]!, app.binary).split(path.sep)[0] ?? "";
    const bundle = installed.endsWith(".app") ? installed : "";
    const allowed = new Set([bundle, "Applications", ".DS_Store", ".VolumeIcon.icns"]);
    const extras = app.dmgRoot.map((entry) => entry.name).filter((name) => !allowed.has(name));
    if (extras.length > 0) {
      check(false, `contents: the dmg root holds ${extras.join(", ")}: MyGo writes only the ${bundle || ".app"}, its /Applications link, .DS_Store and .VolumeIcon.icns (cmd/mygo/dmg.go)`);
    }
    const bundles = app.dmgRoot.filter((entry) => entry.name.endsWith(".app"));
    if (bundle !== "" && (bundles.length !== 1 || bundles[0]!.name !== bundle)) {
      check(false, `contents: the dmg root's app bundle is ${bundles.map((entry) => entry.name).join(", ") || "missing"}, want ${bundle}`);
    }
    const link = app.dmgRoot.find((entry) => entry.name === "Applications");
    if (link === undefined || link.link !== "/Applications") {
      check(false, `contents: the dmg root's Applications entry is ${link === undefined ? "missing" : link.link === "" ? "not a link" : `a link to ${link.link}`}, want a link to /Applications`);
    }
  }

  const needles = baseline === null ? `no baseline under ${BASELINE}` : `${baseline.files} baseline files, ${baseline.lines.size} page lines as needles`;
  return `contents scan: ${files} files, ${size(bytes)}, ${needles}, ${ownedFiles} project-owned files credential-scanned, ${((performance.now() - started) / 1000).toFixed(1)}s`;
}
