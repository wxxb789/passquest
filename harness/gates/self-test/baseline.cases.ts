import { BASELINE, EXPECTED_NATIVE, EXPECTED_PAGES, nonSpaceChars } from "../../lib/baseline.ts";

import { baseline } from "../baseline.ts";
import type { BrokenCase } from "./fixture.ts";

interface BaselineFixtureOptions {
  wrongCharsPage?: number;
  /** Writes U+FEFF into this page; the manifest uses the pre-fix \s count. */
  bomPage?: number;
  /** Renames one page file and manifest row, leaving a gap at `from`. */
  renumber?: { from: number; to: number };
}

/** A complete baseline tree; options break exactly one documented property. */
function baselineFixture(options: BaselineFixtureOptions = {}): { files: Record<string, string>; tracked: string[] } {
  const files: Record<string, string> = {};
  const tracked: string[] = [];
  const rows = ["page\tsection\tsource\tbytes\tchars_no_ws"];
  for (let page = 1; page <= EXPECTED_PAGES; page++) {
    const text = page === options.bomPage ? `page ${page} text\uFEFF\n` : `page ${page} text\n`;
    const number = options.renumber && page === options.renumber.from ? options.renumber.to : page;
    const relative = `${BASELINE}/pages/page-${String(number).padStart(3, "0")}.txt`;
    files[relative] = text;
    tracked.push(relative);
    const source = page <= EXPECTED_NATIVE ? "native" : "ocr";
    let chars = nonSpaceChars(text);
    if (page === options.wrongCharsPage) chars += 1;
    if (page === options.bomPage) chars -= 1;
    rows.push([number, `section-${page}`, source, Buffer.byteLength(text), chars].join("\t"));
  }
  files[`${BASELINE}/manifest.tsv`] = `${rows.join("\n")}\n`;
  files[`${BASELINE}/README.md`] = "The extracted text is unreviewed.\n";
  tracked.push(`${BASELINE}/manifest.tsv`, `${BASELINE}/README.md`);
  return { files, tracked };
}

export function baselineCases(): BrokenCase[] {
  const completeBaseline = baselineFixture();
  const wrongCount = baselineFixture({ wrongCharsPage: 7 });
  const bomCount = baselineFixture({ bomPage: 7 });
  const pageGap = baselineFixture({ renumber: { from: EXPECTED_PAGES, to: EXPECTED_PAGES + 1 } });
  return [
    {
      gate: baseline,
      label: "baseline manifest disagrees with its page",
      files: wrongCount.files,
      tracked: wrongCount.tracked,
      postFail: (failures) =>
        failures.length === 1 && failures[0].includes("page-007.txt") && failures[0].includes("non-space chars")
          ? null
          : `baseline: expected one wrong char count on page-007.txt, saw ${failures.length}: ${failures.join("; ")}`,
    },
    {
      gate: baseline,
      label: "baseline count treats U+FEFF as whitespace",
      files: bomCount.files,
      tracked: bomCount.tracked,
      postFail: (failures) =>
        failures.length === 1 && failures[0].includes("page-007.txt") && failures[0].includes("non-space chars")
          ? null
          : `baseline: expected one wrong char count on page-007.txt, saw ${failures.length}: ${failures.join("; ")}`,
    },
    {
      gate: baseline,
      label: "baseline page set has a gap",
      files: pageGap.files,
      tracked: pageGap.tracked,
      postFail: (failures) =>
        failures.some((line) => line.includes(`manifest page ${EXPECTED_PAGES} is missing`))
          ? null
          : `baseline: expected a missing manifest page, saw: ${failures.join("; ")}`,
    },
    {
      gate: baseline,
      label: "test fixture references the baseline",
      files: {
        ...completeBaseline.files,
        "internal/fixture_test.go": 'package internal\n\nconst baselinePath = "data/baselines/2026-crsc-english"\n',
      },
      tracked: [...completeBaseline.tracked, "internal/fixture_test.go"],
      postFail: (failures) =>
        failures.length === 1 &&
        failures[0].includes("internal/fixture_test.go") &&
        failures[0].includes("references the unreviewed baseline")
          ? null
          : `baseline: expected one fixture-reference failure, saw ${failures.length}: ${failures.join("; ")}`,
    },
  ];
}
