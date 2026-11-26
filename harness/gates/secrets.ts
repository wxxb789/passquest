import { readEach } from "../lib/repo.ts";
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
  ".mjs": true,
  ".json": true,
  ".yaml": true,
  ".yml": true,
  ".toml": true,
};

const CONFLICT_OPEN = /^<{7} /;
const CONFLICT_BASE = /^\|{7} /;
const CONFLICT_CLOSE = /^>{7} /;

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

function conflictMarkerProblems(file: string, bytes: Buffer | null): string[] {
  if (bytes === null || !isText(bytes)) return [];
  const problems: string[] = [];
  const lines = bytes.toString("utf8").split(/\r?\n/);
  let open = false;
  for (let index = 0; index < lines.length; index++) {
    const line = lines[index];
    const opens = CONFLICT_OPEN.test(line);
    const closes = CONFLICT_CLOSE.test(line);
    if (opens || closes || CONFLICT_BASE.test(line)) {
      problems.push(`${file}:${index + 1}: merge-conflict marker`);
    }
    if (opens) open = true;
    else if (closes) open = false;
    else if (open && line === "=======") problems.push(`${file}:${index + 1}: merge-conflict marker`);
  }
  return problems;
}

function scanFile(file: string, bytes: Buffer | null): string[] {
  const problems: string[] = [];
  const pathProblem = forbiddenPath(file);
  if (pathProblem) problems.push(`${file}: ${pathProblem} is tracked`);
  if (envFile(file)) problems.push(`${file}: tracked environment file`);

  if (bytes === null || !isText(bytes)) return problems;
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

export async function scanFiles(root: string, files: string[]): Promise<string[]> {
  const contents = await readEach(root, files);
  return files.flatMap((file, index) => scanFile(file, contents[index]));
}

export const secrets: Gate = {
  id: "secrets",
  title: "secrets/sources",
  async run(ctx: GateContext) {
    const files = await ctx.workingTreeFiles();
    const contents = await readEach(ctx.root, files);
    const problems = files.flatMap((file, index) => [
      ...scanFile(file, contents[index]),
      ...conflictMarkerProblems(file, contents[index]),
    ]);
    if (problems.length > 0) return fail(problems);
    return pass(`${files.length} files, 0 findings`);
  },
};
