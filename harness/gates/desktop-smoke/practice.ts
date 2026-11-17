import { pageScript, SKILL } from "../../lib/desktop-page.ts";
import { arrayField, field, numberField, stringField } from "../../lib/json.ts";
import { attemptsOf, checkAttempts, cli, launch } from "./scenario.ts";
import type { Scenario } from "./scenario.ts";

export const practice: Scenario = {
  id: "practice",
  async run(s) {
    // The recommendation opens the guided walkthrough (assisted). After it, skill
    // practice picks a passage with no attempt: independent, it can count. The
    // tactic's independent stage reuses the walkthrough's passage, so it is a
    // re-exposure that cannot count (Q-0004-01). All answered by keyboard.
    const launched = await launch(s, {
      label: "practice",
      script: pageScript(`
await today();
const promise = data('[data-testid="promise"]');
press("Enter");
const guided = await answer("1");
await startFromSheet("start-skill", "${SKILL}");
const independent = await answer("2");
await startFromSheet("start-tactic", "${SKILL}|independent");
const repeated = await answer("1");
return { promise, answers: [guided, independent, repeated] };
`),
    });
    if (!launched) return;
    const answers = arrayField(launched.value, "answers");
    const attempts = attemptsOf(await cli(s, ["report"]));
    s.check(
      attempts.length === answers.length,
      `practice: report --json lists ${attempts.length} attempt(s) for ${answers.length} answer(s) in the window`,
    );
    checkAttempts(s, answers, attempts);
    const expected = [
      ["guided", "assisted", "false"],
      ["skill", "independent", "true"],
      ["tactic independent", "re_exposure", "false"],
    ];
    expected.forEach(([label, condition, canCount], index) => {
      const answered = answers[index];
      const shown = field(answered, "condition");
      const counted = field(answered, "counted");
      s.check(
        stringField(shown, "condition") === condition && stringField(shown, "canCount") === canCount,
        `practice: the ${label} item showed ${String(stringField(shown, "condition"))} (can count ${String(stringField(shown, "canCount"))}) before answering, expected ${condition} (can count ${canCount})`,
      );
      s.check(
        stringField(counted, "condition") === condition && stringField(counted, "counted") === canCount,
        `practice: the ${label} feedback recorded ${String(stringField(counted, "condition"))} (counted ${String(stringField(counted, "counted"))}), the item showed ${condition}`,
      );
    });
    const promise = field(launched.value, "promise");
    s.check(
      stringField(promise, "canCount") === stringField(field(answers[0], "condition"), "canCount"),
      `practice: Today promised can count ${String(stringField(promise, "canCount"))}, the opened item showed ${String(stringField(field(answers[0], "condition"), "canCount"))}`,
    );
    // The adaptive evidence a second process reads counts exactly the item the window said could count.
    const ids = answers.map((answered) => Number(stringField(field(answered, "feedback"), "attempt")));
    const skill = arrayField((await cli(s, ["next"])).doc, "skills").find(
      (entry) => stringField(field(entry, "skill"), "id") === SKILL,
    );
    const evidence = field(skill, "evidence");
    const counted = arrayField(evidence, "independent_attempt_ids").map(Number);
    s.check(
      counted.length === 1 && counted[0] === ids[1],
      `practice: next --json counts independent attempt(s) [${counted.join(", ")}], the window showed only attempt ${ids[1]} as counting`,
    );
    s.check(
      (numberField(evidence, "passage_repeats") ?? 0) === 1,
      `practice: next --json passage_repeats = ${String(numberField(evidence, "passage_repeats"))} after one re-exposure attempt ${ids[2]}`,
    );
  },
};
