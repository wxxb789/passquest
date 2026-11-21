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
 *   puts there, beside the volume metadata macOS keeps at a volume's root;
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
 * - `.env`: a file named `.env` or `.env.*` (case-insensitively; a staged tree
 *   carries no node_modules at all today, and this check does not exempt one);
 * - the internal-only marker: the bytes of `CONFIDENTIAL_MARKER_TEXTS` in the
 *   file — UTF-8 text, in binaries too;
 * - the baseline text: the scan reads, at run time, both committed
 *   extractions of the internal-only PDF — `data/baselines/2026-crsc-english`
 *   (the page-linked baseline the `baseline` gate verifies) and
 *   `data/baselines/2025-crsc-english-partial.txt`, the earlier 22-page
 *   extraction — and fails on a payload file byte-identical (SHA-256) to one
 *   of their files, or holding one of their lines as a substring of a view.
 *
 *   Both sides of a text match are normalised the same way: a line is
 *   trimmed and its whitespace runs collapse to one space, and a needle must
 *   still be 40 bytes (UTF-8) or longer after that. Each payload file is
 *   searched in three views — its text with whitespace collapsed; the same
 *   text with the JSON/JS string escapes `\n`, `\t`, `\r`, `\"`, `\\`, `\/`
 *   and `\uXXXX`, a surrogate pair included, decoded and then collapsed,
 *   which is skipped when the file holds no backslash; and, only when the
 *   bytes carry a UTF-16LE BOM or hold NUL bytes in most odd positions, the
 *   file read as UTF-16LE and collapsed. A needle may start anywhere in a
 *   view: a rolling hash (Rabin-Karp) over 40-byte windows, keyed on each
 *   needle's first 40 normalised bytes, finds the candidates, and the whole
 *   needle confirms one, so no needle is searched against a file on its own.
 *
 *   What this does not catch: text in another encoding (UTF-16BE), base64,
 *   compressed or encrypted data, or a transliteration neither normalisation
 *   can undo, such as a line spread out character by character. Nothing of
 *   the baselines is copied into the harness: the needles are computed from
 *   the committed files at run time, and a failure names the payload file
 *   and the baseline file, never the text. A tree carrying neither baseline
 *   (a self-test fixture) has nothing to match, which the scan line states.
 *
 * The installed `agent/` tree is checked as a whole too (`assertLeanAgentTree`,
 * goal 0016): it holds the host scripts and the notices file only — no
 * node_modules, no symlink, no other file — no host names a specifier that is
 * not a `node:` built-in, and no file embeds a path of the machine that built
 * it (a drive path, a user home, a temp directory; the notices file is the
 * one exemption, and only from the shapes and the temp literal — the next
 * paragraph states the rule it does carry).
 *
 * The notices file must carry what the staged bundles required (goal 0016 P4,
 * `noticesProblems`): every licence text staging assembled for the packages
 * the bundles inlined, under the header naming it, verbatim. The build's
 * identity carries that set; a fixture install carries none unless the caller
 * hands one in (`fixtureNotices`), which is how the self-test drives this
 * check over a fixture too. The notices file is exempt from the machine-path
 * shapes and the temp-directory literal — licence prose may name `/home/` or
 * `/tmp/`, and `tmpdir()` is machine-spelled itself — and is refused for the
 * checkout root alone, the same rule on every OS (harness/lib/package.ts,
 * checkoutRootIn); the assembly writes no path of this machine
 * (harness/lib/notices.ts).
 *
 * The credential patterns the secrets gate carries apply to project-owned
 * files only, through `scanFiles` (its patterns and its binary skip, not a
 * copy): the four bundled `agent/*.mjs` hosts, the metadata MyGo generates
 * from mygo.json as files — Linux's `<slug>.desktop`, macOS's
 * `Contents/Info.plist`; Windows writes no such file, the exe carries the
 * version info and the installer's strings live in the registry — and the
 * frontend dist this build embedded (the same bytes sit in the app binary,
 * which is not credential-scanned). The bundled Node runtime is not
 * credential-scanned either: it legitimately contains published
 * credential-shaped examples (a provider SDK's type doc the plan scouted), and
 * the check must not be weakened per file to pass. A package carries no
 * node_modules at all (goal 0016), which the lean-tree check below enforces.
 *
 * The bundle's own `Contents/Info.plist` — under a `.app`, or at a fixture
 * tree's root — must declare `LSMinimumSystemVersion` equal to the minimum
 * mygo.json declares (goal 0017 P1, `plistProblems`): MyGo copies
 * `macos.minimumSystemVersion` verbatim into that key, and the effective floor
 * is the higher of it and the Mach-O's `minos` — 13.0 from the Go 1.27
 * linker, below the declared 26.0 — so the key is the binding one, and an
 * older value would open on a macOS between the linker's `minos` and the
 * declared floor. A real macOS run must carry the file; a tree that declares
 * no minimum fails instead of passing unchecked, and a fixture tree may carry
 * no plist at all.
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
import { BASELINE, BASELINE_PARTIAL } from "../../lib/baseline.ts";
import { FRONTEND_DIST } from "../../lib/desktop.ts";
import type { Check } from "../../lib/desktop-page.ts";
import { noticeSection } from "../../lib/notices.ts";
import type { StagedNotices } from "../../lib/notices.ts";
import { mygoFileName } from "../../lib/package-build.ts";
import type { PackageArtifact, PackageNaming } from "../../lib/package-build.ts";
import { size } from "../../lib/package-build.ts";
import { assertLeanAgentTree, goTarget, hostTarget, NOTICES_FILE } from "../../lib/package.ts";
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
  /** mygo.json's macos.minimumSystemVersion, which MyGo writes into the bundle's Info.plist verbatim and the scan requires there. */
  minimumSystemVersion: string;
  /** The notices this staging's bundles required (harness/lib/notices.ts): the installed notices file must carry every text of them. */
  notices: StagedNotices;
}

const PDF = Buffer.from("%PDF-", "latin1");
const SQLITE = Buffer.from("SQLite format 3\0", "latin1");
/** The span the PDF format allows the header to sit in, and the part of a container the scan reads. */
const HEAD_BYTES = 1024;

/** The volume metadata macOS keeps at a mounted volume's root. */
const VOLUME_METADATA = [".fseventsd", ".Trashes", ".Spotlight-V100"];
/**
 * A line shorter than this — in UTF-8 bytes, the unit both sides are measured
 * in — is too common to stand for a baseline line. A CJK line is three bytes
 * per character, so the units must match or such a line would never be looked
 * up.
 */
const SHORTEST_LINE = 40;
/** The rolling-hash window: a view is hashed in 40-byte windows, and a needle is keyed by its first 40 bytes. */
const WINDOW = SHORTEST_LINE;
/** The polynomial base of the rolling hash, and BASE^(WINDOW-1) mod 2^32 — the weight a window's first byte carries. */
const BASE = 0x01000193;
const POWER = ((): number => {
  let power = 1;
  for (let index = 0; index < WINDOW - 1; index++) power = Math.imul(power, BASE) >>> 0;
  return power;
})();
/** The needle filter is a 2^18-bit bitmap: its array length in bytes, and the mask of a hash's byte within it. */
const FILTER_BYTES = 1 << 15;
const FILTER_MASK = FILTER_BYTES - 1;
/** The marker texts as bytes: a CJK marker inside a binary is these bytes. */
const MARKERS = CONFIDENTIAL_MARKER_TEXTS.map((text) => Buffer.from(text, "utf8"));

/** One needle: a normalised baseline line as bytes, and the baseline file it came from. */
interface Needle {
  bytes: Buffer;
  /** The root-relative baseline file the line was taken from. */
  file: string;
}

/** The needle set computed from the committed baselines; null where the tree carries neither (a fixture). */
interface BaselineSource {
  /** A baseline file's SHA-256 -> its root-relative path, for payload files copied whole. */
  hashes: Map<string, string>;
  /** A needle's first-40-bytes rolling hash -> the needles that start with those bytes. */
  needles: Map<number, Needle[]>;
  /**
   * A bitmap over the low 18 bits of every needle hash: a window whose bit is
   * clear cannot start a needle, so almost no window of a payload file has to
   * touch the map.
   */
  filter: Uint8Array;
  /** Files under the 2026 baseline hashed, and the two sources' needle counts, for the scan line. */
  files: number;
  pageLines: number;
  partialLines: number;
}

/**
 * The needles the sources of truth give: every file under the 2026 baseline
 * directory hashed for whole-file copies, and its page files' lines
 * (`pages/page-*.txt`, the naming the baseline gate checks) as needles, plus
 * the earlier partial extraction of the same PDF — hashed whole, and its
 * lines as needles beside them.
 */
function baselineSource(root: string): BaselineSource | null {
  const dir = resolve(root, BASELINE);
  const partial = resolve(root, BASELINE_PARTIAL);
  if (!existsSync(dir) && !existsSync(partial)) return null;
  const source: BaselineSource = {
    hashes: new Map(),
    needles: new Map(),
    filter: new Uint8Array(FILTER_BYTES),
    files: 0,
    pageLines: 0,
    partialLines: 0,
  };
  if (existsSync(dir)) {
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
        source.hashes.set(createHash("sha256").update(data).digest("hex"), `${BASELINE}/${relative}`);
        source.files += 1;
        if (!/^pages\/page-\d+\.txt$/.test(relative)) continue;
        for (const line of linesOf(data)) if (addNeedle(source, line, `${BASELINE}/${relative}`)) source.pageLines += 1;
      }
    };
    walk(dir, "");
  }
  if (existsSync(partial)) {
    const data = readFileSync(partial);
    source.hashes.set(createHash("sha256").update(data).digest("hex"), BASELINE_PARTIAL);
    for (const line of linesOf(data)) if (addNeedle(source, line, BASELINE_PARTIAL)) source.partialLines += 1;
  }
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

/** Whitespace runs collapse to one space and the ends are trimmed: a needle and a view are normalised the same way. */
function collapse(text: string): string {
  return text.replace(/\s+/g, " ").trim();
}

/** The rolling hash of the WINDOW bytes starting at one offset of a view. */
function windowHash(bytes: Buffer, offset: number): number {
  let hash = 0;
  for (let index = 0; index < WINDOW; index++) hash = (Math.imul(hash, BASE) + bytes[offset + index]!) >>> 0;
  return hash;
}

/** Adds one baseline line, normalised, as a needle when it clears the floor; true when it did. */
function addNeedle(source: BaselineSource, line: string, file: string): boolean {
  const bytes = Buffer.from(collapse(line), "utf8");
  if (bytes.length < SHORTEST_LINE) return false;
  const key = windowHash(bytes, 0);
  source.filter[(key >>> 3) & FILTER_MASK]! |= 1 << (key & 7);
  const list = source.needles.get(key);
  if (list === undefined) source.needles.set(key, [{ bytes, file }]);
  else list.push({ bytes, file });
  return true;
}

/** The escapes a JSON or JavaScript string carries, as the characters they stand for. */
const SIMPLE_ESCAPES: Record<string, string> = { n: "\n", t: "\t", r: "\r", '"': '"', "\\": "\\", "/": "/" };

/**
 * The JSON/JS-string view, before it is collapsed: `\n`, `\t`, `\r`, `\"`,
 * `\\`, `\/` and `\uXXXX` decode to the characters they stand for — a
 * surrogate pair is two such escapes and comes out as the pair it encodes.
 * Any other escape stays as written, so an unrecognised sequence cannot
 * invent a match; a text without a backslash is returned unchanged.
 */
function unescapeStrings(text: string): string {
  if (!text.includes("\\")) return text;
  return text.replace(
    /\\u([0-9a-fA-F]{4})|\\([ntr"\\/])/g,
    (whole: string, code: string | undefined, simple: string | undefined) =>
      code !== undefined ? String.fromCharCode(parseInt(code, 16)) : simple === undefined ? whole : SIMPLE_ESCAPES[simple] ?? whole,
  );
}

/**
 * The file read as UTF-16LE, or null when the bytes do not look like it: a
 * UTF-16LE BOM, or, without one, NUL bytes in most odd positions over a
 * bounded sample — the shape English text stored as UTF-16LE has. (CJK text
 * puts its NUL bytes in even positions, which this does not claim to catch.)
 */
function utf16View(data: Buffer): string | null {
  if (data.length >= 2 && data[0] === 0xff && data[1] === 0xfe) return data.toString("utf16le", 2);
  const pairs = Math.min(Math.floor(data.length / 2), 4096);
  if (pairs < 8) return null;
  let zeros = 0;
  for (let pair = 0; pair < pairs; pair++) if (data[pair * 2 + 1] === 0) zeros += 1;
  return zeros * 2 > pairs ? data.toString("utf16le") : null;
}

/**
 * The views of one payload file, in the order they are searched: the text
 * with whitespace collapsed, the same text with JSON/JS escapes decoded (or
 * nothing, when there is no backslash to decode), and the UTF-16LE reading
 * collapsed.
 */
function* payloadViews(data: Buffer): Generator<Buffer> {
  const text = data.toString("utf8");
  yield Buffer.from(collapse(text), "utf8");
  const decoded = unescapeStrings(text);
  if (decoded !== text) yield Buffer.from(collapse(decoded), "utf8");
  const wide = utf16View(data);
  if (wide !== null) yield Buffer.from(collapse(wide), "utf8");
}

/**
 * The first needle a view holds: only a window whose rolling hash matches a
 * needle's first-40-bytes hash is compared at all, and the whole needle must
 * match at that byte before it counts, so a needle may start anywhere in the
 * view and a hash collision alone names nothing.
 */
function firstNeedle(view: Buffer, source: BaselineSource): Needle | null {
  if (view.length < WINDOW) return null;
  const { needles } = source;
  let hash = windowHash(view, 0);
  for (let offset = 0; ; offset++) {
    if ((source.filter[(hash >>> 3) & FILTER_MASK]! & (1 << (hash & 7))) !== 0) {
      const bucket = needles.get(hash);
      if (bucket !== undefined) {
        for (const needle of bucket) {
          if (offset + needle.bytes.length <= view.length && view.subarray(offset, offset + needle.bytes.length).equals(needle.bytes)) {
            return needle;
          }
        }
      }
    }
    if (offset + WINDOW >= view.length) return null;
    hash = (hash - Math.imul(view[offset]!, POWER)) >>> 0;
    hash = (Math.imul(hash, BASE) + view[offset + WINDOW]!) >>> 0;
  }
}

/** The first baseline needle a payload file holds, searched view by view, or null. */
function baselineNeedle(data: Buffer, source: BaselineSource): Needle | null {
  if (source.needles.size === 0) return null;
  for (const view of payloadViews(data)) {
    const needle = firstNeedle(view, source);
    if (needle !== null) return needle;
  }
  return null;
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

/** Whether the last element of a path or of a link target is `.env` or `.env.*`, the payload rule both use. */
function isEnvironmentName(name: string): boolean {
  const base = name.replace(/\\/g, "/").split("/").pop()!.toLowerCase();
  return base === ".env" || base.startsWith(".env.");
}

/** Every forbidden-content problem of one payload file. */
function payloadProblems(file: string, relative: string, data: Buffer, baseline: BaselineSource | null): string[] {
  const problems = containerProblems(file, data);
  const segment = privateSegment(relative);
  if (segment !== "") problems.push(`${file}: private path segment ${JSON.stringify(segment)}`);
  if (isEnvironmentName(relative)) problems.push(`${file}: environment file`);
  for (const marker of MARKERS) {
    if (data.includes(marker)) {
      problems.push(`${file}: internal-only source marker`);
      break;
    }
  }
  if (baseline !== null) {
    const copy = baseline.hashes.get(createHash("sha256").update(data).digest("hex"));
    if (copy !== undefined) problems.push(`${file}: baseline text, byte-identical to ${copy}`);
    else {
      const needle = baselineNeedle(data, baseline);
      if (needle !== null) problems.push(`${file}: baseline text, a line of ${needle.file}`);
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
 * The problems the installed notices file has against what the staged bundles
 * required (goal 0016 P4): the file must carry every section staging
 * assembled — its header and its text, as one unit (noticeSection) — so a
 * body deleted in transit is seen even where another section carries the same
 * body under a different header, which five of the real file's sections do.
 * A missing file, a section dropped in transit and a file some other staging
 * wrote each fail; the messages name the package and the source file, never
 * the text itself.
 */
export function noticesProblems(file: string, required: StagedNotices): string[] {
  if (!existsSync(file)) {
    return [
      `${NOTICES_FILE} is missing from ${path.dirname(file)}/: the staged bundles inlined ${required.packages.length} package(s), whose notices must travel with the code`,
    ];
  }
  let text: string;
  try {
    text = readFileSync(file, "utf8");
  } catch (error) {
    return [`${file} cannot be read: ${error instanceof Error ? error.message : String(error)}`];
  }
  const problems: string[] = [];
  for (const notice of required.texts) {
    if (text.includes(noticeSection(notice))) continue;
    if (!text.includes(notice.label)) problems.push(`${NOTICES_FILE} names no ${notice.label}`);
    else problems.push(`${NOTICES_FILE} does not carry the text of ${notice.label} under its own header`);
  }
  return problems;
}

/**
 * The problems of an installed bundle's `Contents/Info.plist` against the
 * minimum mygo.json declares (goal 0017 P1): MyGo writes
 * `macos.minimumSystemVersion` verbatim into `LSMinimumSystemVersion`
 * (cmd/mygo/bundle.go:254, writePlistValue — sorted keys, one
 * `<key>`/`<string>` pair per line), and the effective floor is the higher of
 * that key and the Mach-O's `minos` — 13.0 from the Go 1.27 linker, below the
 * declared 26.0 — so the key is the binding one and it must state the floor.
 * The failure names the found and the expected value; an empty
 * `expectedMinimum` — mygo.json declares none, and MyGo's own default is
 * 12.0 — is a failure of its own, so a tree with a plist never passes
 * unchecked.
 */
export function plistProblems(content: string, expectedMinimum: string): string[] {
  if (expectedMinimum === "") {
    return ["mygo.json declares no macos.minimumSystemVersion: LSMinimumSystemVersion cannot be checked (MyGo's own default is 12.0)"];
  }
  const found = /<key>LSMinimumSystemVersion<\/key>\s*<string>([^<]*)<\/string>/.exec(content);
  if (found === null) {
    return [`LSMinimumSystemVersion is not a <key>/<string> pair in the Info.plist; mygo.json declares ${JSON.stringify(expectedMinimum)}`];
  }
  if (found[1] !== expectedMinimum) {
    return [`LSMinimumSystemVersion is ${JSON.stringify(found[1])}, want ${JSON.stringify(expectedMinimum)}, the macos.minimumSystemVersion mygo.json declares`];
  }
  return [];
}

/**
 * Whether a walk-relative path is the installed bundle's own property list:
 * `Contents/Info.plist` under the single `.app` a macOS install puts at the
 * tree's root (install.ts), or at a fixture tree's root, where the tree
 * itself stands in for the bundle. A nested bundle's plist is not the
 * installed app's and is not checked.
 */
function isBundlePlist(relative: string): boolean {
  const segments = relative.replace(/\\/g, "/").split("/");
  if (segments.length === 3 && segments[0]!.endsWith(".app")) segments.shift();
  return segments.length === 2 && segments[0] === "Contents" && segments[1] === "Info.plist";
}

/**
 * Scans the installed package's contents and reports every problem through
 * `check`; returns the scan line (file count, bytes, needles, wall time) for
 * the gate's pass detail. A real run passes the `identity` of the package it
 * built, whose notices set the installed file must carry; a fixture install
 * tree, which has no build, passes `fixtureNotices` instead — a run has one
 * or the other, never both. The minimum mygo.json declares, which the
 * installed `Contents/Info.plist` must carry, arrives the same way: in the
 * identity for a real run, in `fixtureMinimumSystemVersion` for a fixture
 * tree.
 */
export async function scanPackageContents(
  app: InstalledApp,
  ctx: GateContext,
  check: Check,
  identity?: PackageIdentity,
  fixtureNotices?: StagedNotices,
  fixtureMinimumSystemVersion = "",
): Promise<string> {
  const started = performance.now();
  // The floor the installed Info.plist must declare: a real run's declaration
  // travels in its identity, a fixture tree's (which has no build) in
  // fixtureMinimumSystemVersion.
  const expectedMinimum = identity !== undefined ? identity.minimumSystemVersion : fixtureMinimumSystemVersion;
  const baseline = baselineSource(ctx.root);
  // A real package run must have both extractions to match against: a missing
  // one would quietly narrow the scan. Fixture trees may run without them.
  if (identity !== undefined) {
    check(baseline !== null && baseline.pageLines > 0, `contents: no needles from ${BASELINE}; the baseline text cannot be checked`);
    check(baseline !== null && baseline.partialLines > 0, `contents: no needles from ${BASELINE_PARTIAL}; the partial extraction cannot be checked`);
  }
  let files = 0;
  let bytes = 0;
  /** How many `Contents/Info.plist` files the walk met: a real macOS run must meet one. */
  let plistsSeen = 0;

  for (const dir of app.installDirs) {
    const walk = (current: string, prefix: string): void => {
      for (const entry of readdirSync(current, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
        const full = path.join(current, entry.name);
        const relative = prefix === "" ? entry.name : `${prefix}/${entry.name}`;
        // A link carries no bytes and is not followed (the AppImage's own links
        // are checked by the image checks, and MyGo writes no link into the
        // other trees), but its name and its target are still paths: neither
        // may name private data or end in an environment-file name.
        if (entry.isSymbolicLink()) {
          const target = readlinkSync(full);
          for (const named of [relative, target]) {
            const segment = privateSegment(named);
            if (segment !== "") check(false, `contents: ${full}: a link whose path names private data ${JSON.stringify(segment)}`);
            if (isEnvironmentName(named)) {
              check(false, `contents: ${full}: environment file (a link${named === relative ? "" : ` to ${target}`})`);
            }
          }
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
        if (isBundlePlist(relative)) {
          plistsSeen += 1;
          for (const problem of plistProblems(data.toString("utf8"), expectedMinimum)) check(false, `contents: ${full}: ${problem}`);
        }
      }
    };
    walk(dir, "");
  }

  // A real macOS package must carry the bundle's property list: without the
  // file the check above would have nothing to read and the run would pass
  // unchecked (a fixture tree, which stands in for no build, may carry none).
  if (identity !== undefined && process.platform === "darwin" && plistsSeen === 0) {
    check(false, `contents: the installed .app carries no Contents/Info.plist; its LSMinimumSystemVersion (mygo.json declares ${JSON.stringify(expectedMinimum)}) cannot be checked`);
  }

  // The agent tree is lean (goal 0016): the installed agent/ holds the host
  // scripts and the notices file only, no host names a package specifier, no
  // package carries node_modules, and no file embeds the path of the machine
  // that built it — the checkout root included, which this scan knows the way
  // staging does (the notices file is refused for that literal alone, and for
  // no other machine path).
  const agentDir = app.hosts.length > 0 ? path.dirname(app.hosts[0]!) : "";
  if (agentDir !== "" && existsSync(agentDir)) {
    try {
      assertLeanAgentTree(agentDir, { root: ctx.root });
    } catch (error) {
      check(false, `contents: ${error instanceof Error ? error.message : String(error)}`);
    }
  }

  // The notices the staged bundles required must have travelled into the
  // install (goal 0016 P4): mygo.json packs the staged agent/ tree, so the
  // file the build assembled is the file the package carries. A fixture
  // install stands in for a build's set through fixtureNotices, so the
  // self-test drives this check over a fixture too. The scan line names how
  // many texts were checked, so a passing run shows the check ran.
  const required = identity !== undefined ? identity.notices : fixtureNotices;
  let noticeTextsChecked = 0;
  if (required !== undefined && agentDir !== "" && existsSync(agentDir)) {
    for (const problem of noticesProblems(path.join(agentDir, NOTICES_FILE), required)) {
      check(false, `contents: ${problem}`);
    }
    noticeTextsChecked = required.texts.length;
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
    // the .DS_Store; with an icon it also puts .VolumeIcon.icns there. macOS
    // itself keeps volume metadata at a volume's root (.fseventsd appears on
    // the macos-latest runner's attached image). Any other entry is a file the
    // image must not hold.
    const installed = path.relative(app.installDirs[0]!, app.binary).split(path.sep)[0] ?? "";
    const bundle = installed.endsWith(".app") ? installed : "";
    const allowed = new Set([bundle, "Applications", ".DS_Store", ".VolumeIcon.icns", ...VOLUME_METADATA]);
    const extras = app.dmgRoot.map((entry) => entry.name).filter((name) => !allowed.has(name));
    if (extras.length > 0) {
      check(false, `contents: the dmg root holds ${extras.join(", ")}: MyGo writes only the ${bundle || ".app"}, its /Applications link, .DS_Store and .VolumeIcon.icns (cmd/mygo/dmg.go), and macOS only its volume metadata (${VOLUME_METADATA.join(", ")})`);
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

  const needles =
    baseline === null
      ? `no baseline under ${BASELINE} or ${BASELINE_PARTIAL}`
      : `needles: ${baseline.pageLines} page-file lines of ${BASELINE} (${baseline.files} files hashed) and ${baseline.partialLines} lines of ${BASELINE_PARTIAL}`;
  const noticesNote = noticeTextsChecked === 0 ? "" : `, ${noticeTextsChecked} notice text(s) verified against the staged bundles' set`;
  const plistNote = plistsSeen === 0 ? "" : `, Contents/Info.plist declares LSMinimumSystemVersion ${expectedMinimum}`;
  return `contents scan: ${files} files, ${size(bytes)}, ${needles}, ${ownedFiles} project-owned files credential-scanned${noticesNote}${plistNote}, ${((performance.now() - started) / 1000).toFixed(1)}s`;
}
