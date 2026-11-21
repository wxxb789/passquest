export interface RunOptions {
  cwd?: string;
  env?: Record<string, string>;
  /**
   * Hand the child exactly `env` instead of this process's environment with
   * `env` layered on top; the caller then carries everything the child needs
   * (PATH, SystemRoot, TEMP, HOME). The desktop launches already work this way
   * (harness/lib/desktop.ts): a launch must not inherit a PASSQUEST_*
   * variable the developer's shell happens to carry, which is what cleanEnv
   * exists for.
   */
  replaceEnv?: boolean;
  stdin?: string;
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
      stdin: options.stdin === undefined ? "ignore" : new TextEncoder().encode(options.stdin),
      stdout: "pipe",
      stderr: "pipe",
      env: options.env ? (options.replaceEnv === true ? options.env : { ...process.env, ...options.env }) : process.env,
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

/**
 * `run` with the command, its exact argv and its exit code on stdout — the
 * evidence lines the cask tools' logs quote. An argument a shell would read is
 * JSON-quoted in the line, so the log holds the argv as it was passed.
 */
export async function loggedRun(cmd: string, args: readonly string[], options: RunOptions = {}): Promise<RunResult> {
  const shown = [cmd, ...args].map((arg) => (/^[A-Za-z0-9_@%+=:,./-]+$/.test(arg) ? arg : JSON.stringify(arg)));
  console.log(`[run] ${shown.join(" ")}`);
  const result = await run(cmd, [...args], options);
  console.log(`[run] exit ${result.code}`);
  return result;
}

export function outputLines(result: RunResult): string[] {
  const text = `${result.stdout}${result.stderr}`.replace(/\r\n/g, "\n");
  return text.split("\n").filter((line) => line.trim() !== "");
}

/**
 * The wrapper the cask tools end with: `main` decides the exit code, and a
 * throw out of it is one `name: message` line and exit 1.
 */
export async function runMain(name: string, main: (argv: readonly string[]) => Promise<number>): Promise<void> {
  try {
    process.exitCode = await main(process.argv.slice(2));
  } catch (error) {
    console.error(`${name}: ${error instanceof Error ? error.message : String(error)}`);
    process.exitCode = 1;
  }
}
