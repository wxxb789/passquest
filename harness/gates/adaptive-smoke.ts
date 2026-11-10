import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { runGo } from "../lib/go.ts";
import { arrayField, checkKeyHiding, field, numberField, stringField } from "../lib/json.ts";
import { run } from "../lib/process.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";

function skillEntry(doc: unknown, id: string): unknown {
  return arrayField(doc, "skills").find((item) => stringField(field(item, "skill"), "id") === id);
}

export const adaptiveSmoke: Gate = {
  id: "adaptive-smoke",
  title: "adaptive smoke",
  async run(ctx: GateContext) {
    const binaryName = process.platform === "win32" ? "passquest.exe" : "passquest";

    const temp = mkdtempSync(path.join(tmpdir(), "passquest-adaptive-smoke-"));
    const binary = path.join(temp, binaryName);
    const db = path.join(temp, "smoke.sqlite");
    const checks: string[] = [];
    let assertions = 0;
    const check = (condition: boolean, message: string): boolean => {
      assertions++;
      if (!condition) checks.push(message);
      return condition;
    };
    const call = (args: string[]) => run(binary, args, { cwd: ctx.root });
    const callDB = (args: string[], database = db) => run(binary, [...args, "--db", database], { cwd: ctx.root });
    const parse = (raw: string): unknown => JSON.parse(raw);

    try {
      const build = await runGo(ctx, ["build", "-o", binary, "./cmd/passquest"]);
      if (build.code !== 0) {
        return fail(["go build ./cmd/passquest failed", `${build.stdout}${build.stderr}`.trim()]);
      }

      const listResult = await call(["skill", "list", "--json"]);
      if (!check(listResult.code === 0, `skill list exit = ${listResult.code}`)) return fail(checks);
      const skills = arrayField(parse(listResult.stdout), "skills");
      check(skills.length === 3, `skill list has ${skills.length} skills, want 3`);
      check(skills.some((item) => stringField(item, "id") === "vocabulary-in-context" && stringField(item, "kind") === "language"), "skill list lacks vocabulary-in-context");
      check(skills.some((item) => stringField(item, "id") === "not-except-polarity" && stringField(item, "kind") === "scoring"), "skill list lacks the scoring skill");

      const showResult = await call(["skill", "show", "vocabulary-in-context", "--json"]);
      if (!check(showResult.code === 0, `skill show exit = ${showResult.code}`)) return fail(checks);
      const lesson = field(parse(showResult.stdout), "skill");
      check(stringField(lesson, "kind") === "language", "skill show kind is not language");
      check((stringField(lesson, "principle") ?? "").length > 0, "skill show has no principle");
      check((stringField(lesson, "watch_out") ?? "").length > 0, "skill show has no watch_out");
      check(arrayField(lesson, "steps").length >= 3, "skill show has fewer than 3 steps");

      const emptyNext = parse((await callDB(["next", "--json"])).stdout);
      check(stringField(field(emptyNext, "recommendation"), "action") === "explain", "empty next does not select the scoring explanation");
      check(stringField(field(field(emptyNext, "recommendation"), "skill"), "id") === "not-except-polarity", "empty next recommendation skill");

      const awaitingResult = await callDB(["skill", "practice", "vocabulary-in-context", "--json"]);
      if (!check(awaitingResult.code === 0, `skill awaiting exit = ${awaitingResult.code}`)) return fail(checks);
      const awaiting = parse(awaitingResult.stdout);
      check(stringField(awaiting, "state") === "awaiting_answer", `skill awaiting state = ${String(stringField(awaiting, "state"))}`);
      check(stringField(field(awaiting, "skill"), "id") === "vocabulary-in-context", "skill awaiting skill id");
      check(stringField(field(awaiting, "skill"), "kind") === "language", "skill awaiting kind");
      check(stringField(field(awaiting, "skill"), "assistance") === "independent", "skill awaiting assistance");
      check(stringField(field(awaiting, "skill"), "passage_exposure") === "fresh", "skill awaiting passage exposure");
      check(stringField(field(awaiting, "content"), "id") === "skill-first-party-vocab-notice", "skill awaiting content id");
      check(stringField(field(awaiting, "question"), "id") === "v1", "skill awaiting question id");
      checkKeyHiding(check, awaiting, "skill awaiting JSON");

      const answeredResult = await callDB(["skill", "practice", "vocabulary-in-context", "--json", "--answer", "A"]);
      if (!check(answeredResult.code === 0, `skill answer exit = ${answeredResult.code}`)) return fail(checks);
      const answered = parse(answeredResult.stdout);
      const attempt = field(answered, "attempt");
      check(stringField(attempt, "status") === "correct", `skill attempt status = ${String(stringField(attempt, "status"))}`);
      check(stringField(attempt, "assistance") === "independent", "skill attempt assistance");
      check(arrayField(attempt, "skills").includes("vocabulary-in-context"), "skill attempt has no skill snapshot");

      const afterOne = parse((await callDB(["next", "--json"])).stdout);
      const afterOneVocab = skillEntry(afterOne, "vocabulary-in-context");
      check(numberField(field(afterOneVocab, "evidence"), "independent_attempts") === 1, "next did not count one independent attempt");
      check(numberField(field(afterOneVocab, "evidence"), "independent_correct") === 1, "next did not count one correct attempt");
      check(stringField(field(afterOneVocab, "inference"), "state") === "insufficient", "next did not mark one observation insufficient");
      check(stringField(field(field(afterOne, "recommendation"), "skill"), "id") === "vocabulary-in-context", "next recommendation skill after one attempt");
      check(stringField(field(afterOne, "recommendation"), "action") === "practice_new", "next recommendation action after one attempt");
      check(stringField(field(skillEntry(afterOne, "not-except-polarity"), "inference"), "action") === "explain", "scoring no-evidence action is not explain");

      await callDB(["skill", "practice", "vocabulary-in-context", "--json", "--answer", "B"]);
      const afterTwo = parse((await callDB(["next", "--json"])).stdout);
      const afterTwoVocab = skillEntry(afterTwo, "vocabulary-in-context");
      check(numberField(field(afterTwoVocab, "evidence"), "independent_attempts") === 2, "next did not count two independent attempts");
      check(stringField(field(afterTwoVocab, "inference"), "state") === "supported", "next did not mark two correct observations supported");
      check(numberField(field(afterTwoVocab, "evidence"), "fresh_passages") === 1, "next fresh passage count after two attempts");

      await callDB(["skill", "practice", "vocabulary-in-context", "--json", "--answer", "A"]);
      const afterThree = parse((await callDB(["next", "--json"])).stdout);
      const afterThreeVocab = skillEntry(afterThree, "vocabulary-in-context");
      check(numberField(field(afterThreeVocab, "evidence"), "independent_attempts") === 3, "next did not count three independent attempts");
      check(numberField(field(afterThreeVocab, "evidence"), "fresh_passages") === 0, "next fresh passage count after three attempts");
      check(stringField(field(field(afterThree, "recommendation"), "skill"), "id") === "vocabulary-in-context", "next recommendation after three attempts");
      check(stringField(field(afterThree, "recommendation"), "action") === "review", "next did not select review without a fresh passage");
      check((stringField(field(afterThree, "recommendation"), "command") ?? "").includes("--question"), "review command does not name a seen question");

      const revisit = await callDB(["skill", "practice", "vocabulary-in-context", "--question", "v1", "--answer", "A", "--json"]);
      if (!check(revisit.code === 0, `skill revisit exit = ${revisit.code}`)) return fail(checks);
      const revisitAttempt = field(parse(revisit.stdout), "attempt");
      check(numberField(revisitAttempt, "attempt_number") === 2, "revisit attempt number");
      check(field(revisitAttempt, "repeat") === true, "revisit is not marked repeat");
      const afterRevisit = parse((await callDB(["next", "--json"])).stdout);
      const afterRevisitVocab = skillEntry(afterRevisit, "vocabulary-in-context");
      check(numberField(field(afterRevisitVocab, "evidence"), "revisits") === 1, "next did not count the revisit");
      check(numberField(field(afterRevisitVocab, "evidence"), "independent_attempts") === 3, "revisit changed independent attempt count");

      const passageDB = path.join(temp, "passage.sqlite");
      const guided = await callDB(["tactic", "practice", "not-except-polarity", "--json"], passageDB);
      if (!check(guided.code === 0, `guided tactic exit = ${guided.code}`)) return fail(checks);
      const guidedQuestion = stringField(field(parse(guided.stdout), "question"), "id");
      const guidedAnswer = await callDB(["tactic", "practice", "not-except-polarity", "--json", "--answer", "D"], passageDB);
      if (!check(guidedAnswer.code === 0, `guided tactic answer exit = ${guidedAnswer.code}`)) return fail(checks);
      const independent = await callDB(["tactic", "practice", "not-except-polarity", "--json"], passageDB);
      if (!check(independent.code === 0, `independent tactic exit = ${independent.code}`)) return fail(checks);
      const independentQuestion = stringField(field(parse(independent.stdout), "question"), "id");
      check(guidedQuestion !== undefined && independentQuestion !== undefined && guidedQuestion !== independentQuestion, "tactic questions are not distinct");
      const independentAnswer = await callDB(["tactic", "practice", "not-except-polarity", "--json", "--answer", "C"], passageDB);
      if (!check(independentAnswer.code === 0, `independent tactic answer exit = ${independentAnswer.code}`)) return fail(checks);

      const passageNext = parse((await callDB(["next", "--json"], passageDB)).stdout);
      const scoring = skillEntry(passageNext, "not-except-polarity");
      check(numberField(field(scoring, "evidence"), "assisted_attempts") === 1, "tactic guided attempt is not assisted");
      check(numberField(field(scoring, "evidence"), "independent_attempts") === 0, "same-passage tactic attempt counted as independent transfer");
      check(numberField(field(scoring, "evidence"), "passage_repeats") === 1, "same-passage tactic attempt is not marked passage re-exposure");
      check(numberField(field(scoring, "evidence"), "distinct_passages") === 1, "same-passage tactic attempts span more than one passage");
      check(stringField(field(scoring, "inference"), "state") === "familiarity_only", "same-passage tactic state is not familiarity_only");
      check(stringField(field(scoring, "inference"), "action") === "practice_new", "same-passage tactic action is not practice_new");
      check(stringField(field(field(passageNext, "recommendation"), "skill"), "id") === "not-except-polarity", "same-passage recommendation skill");
      check(stringField(field(passageNext, "recommendation"), "command") === "passquest skill practice not-except-polarity", "same-passage recommendation command");

      const weakDB = path.join(temp, "weak.sqlite");
      await callDB(["skill", "practice", "sentence-structure", "--json", "--answer", "A"], weakDB);
      await callDB(["skill", "practice", "sentence-structure", "--json", "--answer", "A"], weakDB);
      const weak = parse((await callDB(["next", "--json"], weakDB)).stdout);
      const weakSentence = skillEntry(weak, "sentence-structure");
      check(numberField(field(weakSentence, "evidence"), "independent_attempts") === 2, "weak sentence independent attempts");
      check(numberField(field(weakSentence, "evidence"), "independent_correct") === 0, "weak sentence counted a correct answer");
      check(stringField(field(weakSentence, "inference"), "state") === "weak", "weak sentence state");
      check(stringField(field(weakSentence, "inference"), "action") === "explain", "weak sentence action");
      check(stringField(field(weakSentence, "inference"), "causal_diagnosis") === "unresolved", "weak sentence causal diagnosis");
      check(stringField(field(field(weak, "recommendation"), "skill"), "id") === "sentence-structure", "weak recommendation skill");
      check(stringField(field(weak, "recommendation"), "action") === "explain", "weak recommendation action");
      check(stringField(field(weak, "recommendation"), "command") === "passquest skill show sentence-structure", "weak recommendation command");
      check(stringField(field(weak, "recommendation"), "follow_up_command") === "passquest skill practice sentence-structure --question s2", "weak recommendation follow-up command");

      const firstNextRun = await callDB(["next", "--json"]);
      check(firstNextRun.code === 0, `reopened next exit = ${firstNextRun.code}`);
      const secondNextRun = await callDB(["next", "--json"]);
      const firstNext = firstNextRun.stdout;
      const secondNext = secondNextRun.stdout;
      check(firstNext === secondNext, "next output changed across a process reopen");

      const unknownShow = await call(["skill", "show", "no-such-skill", "--json"]);
      check(unknownShow.code === 2, `unknown skill show exit = ${unknownShow.code}`);
      check(stringField(field(parse(unknownShow.stderr), "error"), "code") === "unknown_skill", "unknown skill show error code");
      const unknownPractice = await callDB(["skill", "practice", "no-such-skill", "--json"]);
      check(unknownPractice.code === 2, `unknown skill practice exit = ${unknownPractice.code}`);
      check(stringField(field(parse(unknownPractice.stderr), "error"), "code") === "unknown_skill", "unknown skill practice error code");

      const unclassifiedDB = path.join(temp, "unclassified.sqlite");
      const sample = await callDB(["practice", "--json", "--question", "q1", "--answer", "B"], unclassifiedDB);
      check(sample.code === 0, `sample practice exit = ${sample.code}`);
      const unclassified = parse((await callDB(["next", "--json"], unclassifiedDB)).stdout);
      check(arrayField(unclassified, "unclassified_attempts").length === 1, "untagged sample attempt is not listed as unclassified");
      check(numberField(field(skillEntry(unclassified, "not-except-polarity"), "evidence"), "attempts") === 0, "untagged sample attempt counted for a skill");
    } catch (error) {
      checks.push(error instanceof Error ? error.message : String(error));
    } finally {
      rmSync(temp, { recursive: true, force: true });
    }

    if (checks.length > 0) return fail(checks);
    return pass(`${assertions} assertions`);
  },
};
