import { pageScript } from "../../lib/desktop-page.ts";
import { arrayField, field, numberField, stringField } from "../../lib/json.ts";
import { attemptsOf, cli, launch } from "./scenario.ts";
import type { Scenario } from "./scenario.ts";

export const feedback: Scenario = {
  id: "feedback",
  async run(s) {
    const launched = await launch(s, {
      label: "feedback",
      script: pageScript(`
await startFromSheet("start", "sample");
const optionCount = all(".opt").length;
const answered = await answer("1");
await sleep(200);
const rows = all('[data-testid="option-explanation"]').map((row) => {
  const explanation = row.querySelector("[data-missing]");
  return { label: row.dataset.ev, missing: explanation ? explanation.dataset.missing : null, text: explanation ? explanation.textContent.trim() : "", key: row.classList.contains("is-key") };
});
return {
  optionCount,
  answered,
  key: q("#verified-key")?.textContent.trim() ?? null,
  rows,
  evidence: all(".pas-body .s.ev").map((sentence) => sentence.textContent.trim()),
  unlocated: !!q('[data-testid="evidence"][data-located="false"]'),
};
`),
    });
    if (!launched) return;
    const value = launched.value;
    const rows = arrayField(value, "rows");
    const optionCount = numberField(value, "optionCount") ?? 0;
    s.check(optionCount >= 2 && rows.length === optionCount, `feedback: ${rows.length} option explanation(s) for ${optionCount} option(s)`);
    for (const row of rows) {
      const label = String(stringField(row, "label"));
      const missing = stringField(row, "missing");
      s.check(
        missing === "true" || (missing === "false" && (stringField(row, "text") ?? "") !== ""),
        `feedback: option ${label} shows neither its explanation nor the missing state`,
      );
    }
    const key = stringField(value, "key");
    const keyRows = rows.filter((row) => field(row, "key") === true).map((row) => stringField(row, "label"));
    s.check(key !== undefined && keyRows.length === 1 && keyRows[0] === key, `feedback: verified key ${String(key)}, key rows ${keyRows.join(",")}`);
    const evidence = arrayField(value, "evidence");
    s.check(evidence.length > 0 && evidence.every((sentence) => String(sentence) !== ""), "feedback: no evidence sentence is highlighted");
    s.check(field(value, "unlocated") === false, "feedback: the evidence is shown as not located in the passage");
    const answered = field(value, "answered");
    const id = Number(stringField(field(answered, "feedback"), "attempt"));
    const row = attemptsOf(await cli(s, ["report"])).find((attempt) => numberField(attempt, "id") === id);
    if (!s.check(row !== undefined, `feedback: report --json has no attempt ${id}`)) return;
    s.check(stringField(row, "correct_label") === key, `feedback: the window's key ${String(key)} is not the recorded key ${String(stringField(row, "correct_label"))}`);
    s.check(
      stringField(row, "status") === stringField(field(answered, "feedback"), "verdict"),
      `feedback: the window's verdict is not the recorded status ${String(stringField(row, "status"))}`,
    );
  },
};
