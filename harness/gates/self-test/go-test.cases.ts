import { goTest } from "../go-test.ts";
import { GO_MOD } from "./fixture.ts";
import type { BrokenCase } from "./fixture.ts";

export function goTestCases(): BrokenCase[] {
  return [
    {
      gate: goTest,
      label: "failing Go test",
      files: {
        "go.mod": GO_MOD,
        "x_test.go": 'package x\n\nimport "testing"\n\nfunc TestFail(t *testing.T) { t.Fatal("boom") }\n',
      },
      postFail: (failures) => {
        if (!failures.some((line) => line.includes("TestFail: failed"))) {
          return `go-test: did not report the failing test, saw: ${failures.join("; ")}`;
        }
        if (!failures.some((line) => line.includes("boom"))) {
          return "go-test: dropped the test's failure message";
        }
        return null;
      },
    },
    {
      gate: goTest,
      label: "skipped Go test",
      files: {
        "go.mod": GO_MOD,
        "x_test.go": 'package x\n\nimport "testing"\n\nfunc TestSkipped(t *testing.T) { t.Skip("deliberately skipped") }\n',
      },
      postFail: (failures) =>
        failures.some((line) => line.includes("TestSkipped")) ? null : `go-test: did not report the skipped test, saw: ${failures.join("; ")}`,
    },
  ];
}
