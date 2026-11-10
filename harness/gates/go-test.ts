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
      const failed = [...new Set(events.filter((event) => event.Action === "fail" && event.Test).map((event) => `${event.Package}/${event.Test}`))];
      if (failed.length > 0) return fail(failed.map((name) => `${name}: failed`));
      const output = events.map((event) => event.Output ?? "").join("").trim();
      return fail(output ? output.split(/\r?\n/).filter((line) => line.trim() !== "") : outputLines(result));
    }
    if (skipped.length > 0) {
      return fail(skipped.map((name) => `${name}: skipped test`));
    }
    return pass(`${packages.size} packages, ${tests} tests, 0 skipped`);
  },
};
