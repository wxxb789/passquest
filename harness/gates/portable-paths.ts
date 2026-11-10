import { readText } from "../lib/repo.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate } from "../lib/types.ts";

/**
 * A path literal that only resolves on POSIX is a Windows defect waiting for CI.
 * Go has two string forms and a Windows path is usually written in the backticked
 * one, so both are scanned; a fixture that is genuinely pure string data must say
 * why on the line, so the next reader does not have to re-derive the argument.
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

/** A table entry spans several lines, so a marker may sit up to three lines away. */
const ALLOW_LOOKBACK = 3;

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

function isComment(trimmed: string): boolean {
  return trimmed.startsWith("//") || trimmed.startsWith("*") || trimmed.startsWith("/*");
}

export function scanGoSource(file: string, content: string): Hit[] {
  const hits: Hit[] = [];
  const lines = content.split(/\r?\n/);
  for (let index = 0; index < lines.length; index++) {
    const line = lines[index];
    const trimmed = line.trim();
    if (isComment(trimmed)) continue;
    if (ALLOW_MARKER.test(line)) continue;
    let allowed = false;
    for (let back = 1; back <= ALLOW_LOOKBACK && index - back >= 0; back++) {
      if (ALLOW_MARKER.test(lines[index - back])) {
        allowed = true;
        break;
      }
    }
    for (let ahead = 1; !allowed && ahead <= ALLOW_LOOKBACK && index + ahead < lines.length; ahead++) {
      if (ALLOW_MARKER.test(lines[index + ahead])) {
        allowed = true;
        break;
      }
    }
    if (allowed) continue;
    for (const literal of literalsOnLine(line)) {
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
