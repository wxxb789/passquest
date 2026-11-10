import { readText } from "../lib/repo.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate } from "../lib/types.ts";

/**
 * A path literal that only resolves on POSIX is a Windows defect waiting for CI.
 * Go has two string forms and a Windows path is usually written in the backticked
 * one, so both are scanned; a fixture that is genuinely pure string data must say
 * why on its own line, so the next reader does not have to re-derive the argument
 * and one justified fixture cannot silence an unjustified path beside it.
 */
const POSIX_ROOTS = [
  "/tmp/",
  "/home/",
  "/Users/",
  "/usr/",
  "/var/",
  "/etc/",
  "/opt/",
  "/private/",
  "/root/",
  "/mnt/",
];

const ALLOW_MARKER = /portable-paths:\s*allow\s+\S/;

/** Both Go string forms: interpreted (escapes intact) and raw (backticked). */
const LITERAL = /"(?:[^"\\\n]|\\.)*"|`[^`\n]*`/g;

const DRIVE_PATH = /^[A-Za-z]:[\\/]/;
const UNC_PATH = /^\\\\[^\\]/;

interface Hit {
  file: string;
  line: number;
  literal: string;
}

/** Collapses the escapes that matter, so both string forms compare the same way. */
function unescape(body: string): string {
  return body.replace(/\\(.)/g, (_match, ch: string) => {
    switch (ch) {
      case "n":
        return "\n";
      case "t":
        return "\t";
      case "r":
        return "\r";
      case "\\":
        return "\\";
      case '"':
        return '"';
      default:
        // Go rejects an unknown escape in an interpreted string, so this is unreachable in valid source.
        return ch;
    }
  });
}

function platformBound(value: string): boolean {
  if (DRIVE_PATH.test(value) || UNC_PATH.test(value)) return true;
  return POSIX_ROOTS.some((root) => value.startsWith(root));
}

function literalsOnLine(text: string): string[] {
  const found: string[] = [];
  for (const match of text.matchAll(LITERAL)) {
    const token = match[0];
    const body = token.slice(1, -1);
    const value = token.startsWith("`") ? body : unescape(body);
    if (platformBound(value)) found.push(value);
  }
  return found;
}

/**
 * Removes comments without touching string contents: a line beginning with `*` is a
 * pointer assignment, not a doc comment, and a path inside a block comment is prose.
 */
function stripComments(line: string, state: { inBlock: boolean; inRaw: boolean }): string {
  let out = "";
  let i = 0;
  while (i < line.length) {
    if (state.inRaw) {
      const end = line.indexOf("`", i);
      if (end < 0) return out;
      state.inRaw = false;
      i = end + 1;
      continue;
    }
    if (state.inBlock) {
      const end = line.indexOf("*/", i);
      if (end < 0) return out;
      state.inBlock = false;
      i = end + 2;
      continue;
    }
    const ch = line[i];
    if (ch === "`") {
      const end = line.indexOf("`", i + 1);
      if (end < 0) {
        // A raw string that continues on the next line: its content is prose, not a path.
        state.inRaw = true;
        i = line.length;
        continue;
      }
      out += line.slice(i, end + 1);
      i = end + 1;
      continue;
    }
    if (ch === '"') {
      let j = i + 1;
      while (j < line.length) {
        if (line[j] === "\\") {
          j += 2;
          continue;
        }
        if (line[j] === '"') {
          j += 1;
          break;
        }
        j += 1;
      }
      out += line.slice(i, j);
      i = j;
      continue;
    }
    if (ch === "/" && line[i + 1] === "/") return out;
    if (ch === "/" && line[i + 1] === "*") {
      state.inBlock = true;
      i += 2;
      continue;
    }
    out += ch;
    i += 1;
  }
  return out;
}

export function scanGoSource(file: string, content: string): Hit[] {
  const hits: Hit[] = [];
  const lines = content.split(/\r?\n/);
  const state = { inBlock: false, inRaw: false };
  for (let index = 0; index < lines.length; index++) {
    const line = lines[index];
    const code = stripComments(line, state);
    if (code.trim() === "") continue;
    // The marker justifies the literals on its own line only: a window would let one
    // justified fixture silence an unjustified path next to it.
    if (ALLOW_MARKER.test(line)) continue;
    for (const literal of literalsOnLine(code)) {
      hits.push({ file, line: index + 1, literal });
    }
  }
  return hits;
}

export const portablePaths: Gate = {
  id: "portable-paths",
  title: "portable paths",
  async run(ctx) {
    const files = (await ctx.workingTreeFiles()).filter((file) => file.endsWith(".go"));
    if (files.length === 0) return fail(["no Go files in the working tree"]);
    const hits: Hit[] = [];
    for (const file of files) {
      hits.push(...scanGoSource(file, readText(ctx.root, file)));
    }
    if (hits.length === 0) return pass(`${files.length} Go files, no platform-bound path literal`);
    return fail(
      hits.map(
        (hit) =>
          `${hit.file}:${hit.line}: platform-bound path literal ${JSON.stringify(hit.literal)} — ` +
          "build it with filepath.Join/os.TempDir/t.TempDir, or mark the line " +
          "`// portable-paths: allow <why this is pure string data>`",
      ),
    );
  },
};
