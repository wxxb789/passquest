import { readText } from "../lib/repo.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate } from "../lib/types.ts";

/**
 * A path literal that only resolves on POSIX is a Windows defect waiting for CI.
 * The fixture cases where a literal is genuinely pure string data must say why
 * on the line, so the next reader does not have to re-derive the argument.
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

const WINDOWS_ROOTS = ["C:\\\\", "D:\\\\"];

const ALLOW_MARKER = /portable-paths:\s*allow\s+\S/;

/** A table entry spans several lines, so a marker may sit up to three lines above. */
const ALLOW_LOOKBACK = 3;

interface Hit {
  file: string;
  line: number;
  literal: string;
}

function literalOnLine(text: string): string | null {
  for (const root of [...POSIX_ROOTS, ...WINDOWS_ROOTS]) {
    const index = text.indexOf(`"${root}`);
    if (index < 0) continue;
    const end = text.indexOf('"', index + 1);
    return end < 0 ? text.slice(index + 1) : text.slice(index + 1, end);
  }
  return null;
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
    if (allowed) continue;
    const literal = literalOnLine(line);
    if (literal === null) continue;
    hits.push({ file, line: index + 1, literal });
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
