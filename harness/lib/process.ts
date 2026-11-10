export interface RunOptions {
  cwd?: string;
  env?: Record<string, string>;
}

export interface RunResult {
  code: number;
  stdout: string;
  stderr: string;
}

/** Windows resolves tool names through PATHEXT only if the suffix is present. */
export function executable(name: string): string {
  if (process.platform !== "win32") return name;
  if (name.endsWith(".exe") || name.endsWith(".bat") || name.endsWith(".cmd")) return name;
  return `${name}.exe`;
}

/** Spawn one process without a shell. A missing executable is a result, not a throw. */
export async function run(cmd: string, args: string[], options: RunOptions = {}): Promise<RunResult> {
  let proc: Bun.Subprocess;
  try {
    proc = Bun.spawn({
      cmd: [cmd, ...args],
      cwd: options.cwd,
      stdout: "pipe",
      stderr: "pipe",
      env: options.env ? { ...process.env, ...options.env } : process.env,
    });
  } catch (error) {
    return { code: 127, stdout: "", stderr: error instanceof Error ? error.message : String(error) };
  }
  const [stdout, stderr] = await Promise.all([
    new Response(proc.stdout as ReadableStream).text(),
    new Response(proc.stderr as ReadableStream).text(),
  ]);
  const code = await proc.exited;
  return { code, stdout, stderr };
}

export function outputLines(result: RunResult): string[] {
  const text = `${result.stdout}${result.stderr}`.replace(/\r\n/g, "\n");
  return text.split("\n").filter((line) => line.trim() !== "");
}
