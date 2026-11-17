import path from "node:path";

import { pageScript, tail } from "../../lib/desktop-page.ts";
import { arrayField, field, numberField, stringField } from "../../lib/json.ts";
import { seedReviewedSource } from "../../lib/reviewed-source.ts";
import { cli, launch } from "./scenario.ts";
import type { Scenario } from "./scenario.ts";

const NAVIGATOR_STATES = ["pending", "presented", "answered", "timed_out", "unavailable"];
const NAVIGATOR_CLASSES = ["cell", "cur", "ans", "to"];
const NAVIGATOR_ATTRIBUTES = ["class", "role", "data-state", "data-position", "aria-label", "title", "aria-current"];

function navigatorProblems(where: string, navigators: unknown[]): string[] {
  const problems: string[] = [];
  for (const navigator of navigators) {
    const html = stringField(navigator, "html") ?? "";
    if (/correct|incorrect|wrong/i.test(html)) problems.push(`${where}: the navigator names correctness: ${html.slice(0, 160)}`);
    for (const cell of arrayField(navigator, "cells")) {
      const state = stringField(cell, "state") ?? "";
      if (!NAVIGATOR_STATES.includes(state)) problems.push(`${where}: navigator cell state ${JSON.stringify(state)}`);
      for (const name of arrayField(cell, "classes")) {
        if (!NAVIGATOR_CLASSES.includes(String(name))) problems.push(`${where}: navigator cell class ${String(name)}`);
      }
      for (const name of arrayField(cell, "attributes")) {
        if (!NAVIGATOR_ATTRIBUTES.includes(String(name))) problems.push(`${where}: navigator cell attribute ${String(name)}`);
      }
    }
  }
  return problems;
}

export const assess: Scenario = {
  id: "assess",
  async run(s) {
    const source = await seedReviewedSource((args) => cli(s, args), s.check, path.join(s.dir, "source"), 20, 4);
    if (!source) return;
    const helpers = `
const navigator = () => {
  const nav = q('[data-testid="navigator"]');
  return nav ? { html: nav.innerHTML, cells: all('[data-testid="navigator"] .cell[data-state]').map((cell) => ({ state: cell.dataset.state, classes: Array.from(cell.classList), attributes: cell.getAttributeNames() })) } : null;
};
const position = () => q('[data-testid="assess-item"]')?.dataset.position;
const answerItem = async (key) => {
  const before = position();
  await sleep(150);
  press(key);
  await until("assessment option " + key + " to be selected", () => q('.opt[aria-checked="true"]'));
  press("Enter");
  await until("the next assessment item or the report", () => position() !== before || q('[data-testid="report"]'));
};
`;
    const started = await launch(s, {
      label: "start",
      script: pageScript(`${helpers}
location.hash = "#/assess";
await until("the start button", () => q('[data-act="run-ask"]'));
await click('[data-act="run-ask"]');
await until("the run form", () => q('#runform input[name="size"]'));
q('#runform input[name="size"]').value = "3";
q('#runform input[name="minutes"]').value = "30";
await click('[data-act="run-start"]');
await until("the first assessment item", () => q('[data-testid="assess-item"]'));
const first = data('[data-testid="assess-item"]');
const navigators = [navigator()];
await answerItem("2");
await until("the second item", () => q('[data-testid="assess-item"]'));
navigators.push(navigator());
const left = data('[data-testid="assess-item"]');
await click('[data-act="leave"]');
await until("the leave dialog", () => q('[data-act="leave-yes"]'));
await click('[data-act="leave-yes"]');
await until("Today's way back to the run", () => q('[data-testid="back-to-run"]'));
return { first, left, navigators };
`),
    });
    if (!started) return;
    const resumed = await launch(s, {
      label: "resume",
      script: pageScript(`${helpers}
// The relaunched window offers the active run: Today's way back, or the resume card.
await until("the active run", () => q('[data-testid="back-to-run"]') || q('[data-testid="resume-run"]'));
if (!q('[data-testid="resume-run"]')) await click('[data-testid="back-to-run"]');
await until("the resume card", () => q('[data-testid="resume-run"]'));
await click('[data-testid="resume-run"]');
await until("the resumed item", () => q('[data-testid="assess-item"]'));
const resumed = data('[data-testid="assess-item"]');
const navigators = [];
let answered = 0;
while (q('[data-testid="assess-item"]') && answered < 10) {
  navigators.push(navigator());
  await answerItem(String(1 + (answered % 2)));
  answered++;
}
await until("the run report", () => q('[data-testid="report"]'));
await sleep(200);
return { resumed, answered, navigators, report: data('[data-testid="report"]'), threshold: data('#pass-threshold') };
`),
    });
    if (!resumed) return;
    const first = field(started.value, "first");
    const left = field(started.value, "left");
    const back = field(resumed.value, "resumed");
    const runID = stringField(first, "run");
    s.check(runID !== undefined && stringField(left, "position") !== stringField(first, "position"), "assess: answering did not advance the run");
    s.check(stringField(back, "run") === runID, `assess: resumed run ${String(stringField(back, "run"))}, started ${String(runID)}`);
    for (const name of ["position", "presentedAt"]) {
      s.check(stringField(back, name) === stringField(left, name), `assess: the resumed item's ${name} is not the item left`);
    }
    s.check(stringField(field(resumed.value, "report"), "run") === runID, "assess: the window's report is not the run");
    const threshold = stringField(field(resumed.value, "threshold"), "value");
    s.check(threshold === "unknown", `assess: the window's pass threshold = ${String(threshold)}`);
    const navigators = [...arrayField(started.value, "navigators"), ...arrayField(resumed.value, "navigators")];
    s.check(navigators.length > 2 && navigators.every((navigator) => arrayField(navigator, "cells").length > 0), "assess: an item showed no navigator");
    const problems = navigatorProblems("assess", navigators);
    s.check(problems.length === 0, problems.slice(0, 3).join("; "));
    const report = await cli(s, ["assess", "report", String(runID)]);
    if (!s.check(report.code === 0, `assess: \`passquest assess report ${String(runID)}\` exit = ${report.code}: ${tail(report.stderr, 4)}`)) return;
    s.check(arrayField(report.doc, "unknowns").includes("pass threshold: unknown"), "assess: the CLI report does not state the unknown pass threshold");
    const runDoc = field(report.doc, "run");
    s.check(numberField(runDoc, "resume_count") === 1, `assess: CLI resume_count = ${String(numberField(runDoc, "resume_count"))}`);
    const afterResume = numberField(resumed.value, "answered") ?? 0;
    const counts = field(report.doc, "counts");
    s.check(numberField(counts, "total") === 1 + afterResume, `assess: CLI total ${String(numberField(counts, "total"))}, the window answered ${1 + afterResume}`);
    s.check(
      numberField(counts, "answered_after_resume") === afterResume,
      `assess: CLI answered_after_resume ${String(numberField(counts, "answered_after_resume"))}, the window answered ${afterResume} after resuming`,
    );
  },
};
