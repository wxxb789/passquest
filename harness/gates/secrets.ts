import { readBytes } from "../lib/repo.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";

const CREDENTIALS: Array<{ name: string; pattern: RegExp }> = [
  { name: "OpenAI-style key", pattern: /sk-[A-Za-z0-9]{20,}/ },
  { name: "AWS access key", pattern: /AKIA[0-9A-Z]{16}/ },
  { name: "GitHub token", pattern: /gh[pousr]_[A-Za-z0-9]{30,}/ },
  { name: "Slack token", pattern: /xox[baprs]-[A-Za-z0-9-]{10,}/ },
  { name: "Google API key", pattern: /AIza[0-9A-Za-z_-]{30,}/ },
  { name: "private key block", pattern: /-----BEGIN [A-Z ]*PRIVATE KEY-----/ },
];

const ASSIGNMENT = /"?(api[_-]?key|secret|password|token|client[_-]?secret)"?[ \t]*[:=][ \t]*["'][A-Za-z0-9_/+-]{24,}["']/i;
const SOURCE_EXTENSIONS: Record<string, true> = {
  ".go": true,
  ".ts": true,
  ".js": true,
  ".json": true,
  ".yaml": true,
  ".yml": true,
  ".toml": true,
};

function forbiddenPath(file: string): string | null {
  const normalized = file.replace(/\\/g, "/");
  if (/\.(pdf|sqlite|sqlite-wal|sqlite-shm|db)$/i.test(normalized)) return "private or binary state";
  if (normalized.startsWith("data/private/")) return "private source tree";
  if (normalized.startsWith("data/local/")) return "local source tree";
  if (normalized.startsWith("local-data/")) return "local data tree";
  if (normalized.startsWith(".passquest/")) return "local application state";
  return null;
}

function envFile(file: string): boolean {
  const base = file.split("/").pop() ?? file;
  return /^\.env($|\.)/.test(base) && !base.endsWith(".env.example");
}

function isText(bytes: Buffer): boolean {
  return !bytes.includes(0);
}

function scanFile(root: string, file: string): string[] {
  const problems: string[] = [];
  const pathProblem = forbiddenPath(file);
  if (pathProblem) problems.push(`${file}: ${pathProblem} is tracked`);
  if (envFile(file)) problems.push(`${file}: tracked environment file`);

  let bytes: Buffer;
  try {
    bytes = readBytes(root, file);
  } catch {
    return problems;
  }
  if (!isText(bytes)) return problems;
  const extension = file.slice(file.lastIndexOf("."));
  const lines = bytes.toString("utf8").split(/\r?\n/);
  for (let index = 0; index < lines.length; index++) {
    const line = lines[index];
    for (const credential of CREDENTIALS) {
      if (credential.pattern.test(line)) {
        // Never echo the matched value: the point is the location, not a leak.
        problems.push(`${file}:${index + 1}: credential-shaped string (${credential.name})`);
      }
    }
    if (SOURCE_EXTENSIONS[extension] === true && ASSIGNMENT.test(line)) {
      problems.push(`${file}:${index + 1}: literal secret assignment`);
    }
  }
  return problems;
}

export async function scanTracked(root: string, tracked: string[]): Promise<string[]> {
  const problems: string[] = [];
  for (const file of tracked) {
    problems.push(...scanFile(root, file));
  }
  return problems;
}

export const secrets: Gate = {
  id: "secrets",
  title: "secrets/sources",
  async run(ctx: GateContext) {
    const tracked = await ctx.trackedFiles();
    const problems = await scanTracked(ctx.root, tracked);
    if (problems.length > 0) return fail(problems);
    return pass(`${tracked.length} tracked files, 0 findings`);
  },
};
