import { pageScript, SKILL } from "../../lib/desktop-page.ts";
import { field, stringField } from "../../lib/json.ts";
import { attemptsOf, checkAttempts, cli, launch, setup } from "./scenario.ts";
import type { Scenario } from "./scenario.ts";

export const hint: Scenario = {
  id: "hint",
  async run(s) {
    // After the guided walkthrough, skill practice opens a passage with no
    // attempt, so the item can count until the hint is opened.
    if (!(await setup(s, [["tactic", "practice", SKILL, "--stage", "guided", "--answer", "A"]]))) return;
    const launched = await launch(s, {
      label: "hint",
      script: pageScript(`
await startFromSheet("start-skill", "${SKILL}");
const before = { item: data('[data-testid="practice-item"]'), condition: data('[data-testid="condition"]'), button: !!q('[data-act="hint"]') };
press("h");
await until("the hint gate", () => q('[data-act="hint-yes"]'));
const gated = !q('[data-testid="hint-open"]');
await click('[data-act="hint-yes"]');
await until("the opened hint", () => q('[data-testid="hint-open"]'));
const answered = await answer("1");
return { before, gated, answered };
`),
    });
    if (!launched) return;
    const before = field(launched.value, "before");
    const answered = field(launched.value, "answered");
    const after = field(answered, "condition");
    s.check(stringField(field(before, "condition"), "condition") === "independent", `hint: the item was ${String(stringField(field(before, "condition"), "condition"))}, not independent, before the hint`);
    s.check(stringField(field(before, "condition"), "canCount") === "true", "hint: the item could not count before the hint");
    s.check(field(before, "button") === true, "hint: no hint button on an item with an authored hint");
    s.check(field(launched.value, "gated") === true, "hint: the hint opened without the gate");
    s.check(stringField(after, "condition") === "assisted", `hint: condition after the hint = ${String(stringField(after, "condition"))}`);
    s.check(stringField(after, "canCount") === "false", "hint: the item can still count after the hint");
    for (const name of ["token", "presentedAt"]) {
      s.check(
        stringField(field(before, "item"), name) === stringField(field(answered, "item"), name),
        `hint: opening the hint changed the item's ${name}`,
      );
    }
    s.check(stringField(field(answered, "counted"), "counted") === "false", "hint: the feedback counts a hinted attempt");
    const attempts = attemptsOf(await cli(s, ["report"]));
    checkAttempts(s, [answered], attempts);
  },
};
