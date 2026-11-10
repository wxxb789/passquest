import { hasGoMod, runGo } from "../lib/go.ts";
import { outputLines } from "../lib/process.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate } from "../lib/types.ts";

interface TestEvent {
  Action?: string;
  Package?: string;
  Test?: string;
  Output?: string;
}

// The failure line alone ("pkg/Test: failed") forces a second CI round-trip to
// learn why. Keep each failing test's own output tail; package-level output and
// the raw stream remain the fallback when no test-level failure was reported.
const TEST_OUTPUT_TAIL = 12;

function failureOutput(events: TestEvent[], pkg: string, test: string): string[] {
  const lines: string[] = [];
  for (const event of events) {
    if (event.Action !== "output" || event.Package !== pkg || event.Test !== test) continue;
    for (const line of (event.Output ?? "").split(/\r?\n/)) {
      if (line.trim() !== "") lines.push(line.replace(/\s+$/, ""));
    }
  }
  return lines.slice(-TEST_OUTPUT_TAIL).map((line) => `  ${line}`);
}

function parseEvents(stdout: string): TestEvent[] {
  const events: TestEvent[] = [];
  for (const line of stdout.split(/\r?\n/)) {
    if (line.trim() === "") continue;
    try {
      const event = JSON.parse(line) as TestEvent;
      if (event !== null && typeof event === "object") events.push(event);
    } catch {
      // `go` may print toolchain lines before the JSON stream; they are not events.
    }
  }
  return events;
}

export const goTest: Gate = {
  id: "go-test",
  title: "go test",
  async run(ctx) {
    if (!hasGoMod(ctx)) return skip("no go.mod yet");
    // -json is the only way to see test-level skips: the process exit code is 0
    // when every test is skipped, so a skipped contract test would pass silently.
    const result = await runGo(ctx, ["test", "./...", "-count=1", "-json"]);
    const events = parseEvents(result.stdout);
    const packages = new Set<string>();
    const skipped: string[] = [];
    let tests = 0;
    for (const event of events) {
      if (!event.Package) continue;
      if (event.Action === "pass" && event.Test) tests++;
      if ((event.Action === "pass" || event.Action === "skip") && !event.Test) packages.add(event.Package);
      if (event.Action === "skip" && event.Test) skipped.push(`${event.Package}/${event.Test}`);
    }
    if (result.code !== 0) {
      const failed = new Map<string, { pkg: string; test: string }>();
      for (const event of events) {
        if (event.Action === "fail" && event.Package && event.Test) {
          failed.set(`${event.Package}/${event.Test}`, { pkg: event.Package, test: event.Test });
        }
      }
      if (failed.size > 0) {
        const failures: string[] = [];
        for (const [name, { pkg, test }] of failed) {
          failures.push(`${name}: failed`);
          failures.push(...failureOutput(events, pkg, test));
        }
        return fail(failures);
      }
      const output = events.map((event) => event.Output ?? "").join("").trim();
      return fail(output ? output.split(/\r?\n/).filter((line) => line.trim() !== "") : outputLines(result));
    }
    if (skipped.length > 0) {
      return fail(skipped.map((name) => `${name}: skipped test`));
    }
    return pass(`${packages.size} packages, ${tests} tests, 0 skipped`);
  },
};
