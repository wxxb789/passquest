import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { runGo } from "../lib/go.ts";
import { arrayField, exposedKeys, field, numberField, stringField } from "../lib/json.ts";
import { run } from "../lib/process.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";

const FORBIDDEN = ["answer", "correct_label", "correct_text", "evidence", "reasoning", "status", "feedback"];

function checkKeyHiding(check: (condition: boolean, message: string) => boolean, doc: unknown, where: string): void {
  const exposed = new Set(exposedKeys(doc, FORBIDDEN));
  for (const key of FORBIDDEN) {
    check(!exposed.has(key), `${where} exposes ${key}`);
  }
}

export const tacticSmoke: Gate = {
  id: "tactic-smoke",
  title: "tactic smoke",
  async run(ctx: GateContext) {
    const binaryName = process.platform === "win32" ? "passquest.exe" : "passquest";

    const temp = mkdtempSync(path.join(tmpdir(), "passquest-tactic-smoke-"));
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
    const callDB = (args: string[]) => run(binary, [...args, "--db", db], { cwd: ctx.root });
    const parse = (raw: string): unknown => JSON.parse(raw);

    try {
      const build = await runGo(ctx, ["build", "-o", binary, "./cmd/passquest"]);
      if (build.code !== 0) {
        return fail(["go build ./cmd/passquest failed", `${build.stdout}${build.stderr}`.trim()]);
      }

      const listResult = await call(["tactic", "list", "--json"]);
      if (!check(listResult.code === 0, `tactic list exit = ${listResult.code}`)) return fail(checks);
      const tactics = arrayField(parse(listResult.stdout), "tactics");
      const lessonID = "not-except-polarity";
      check(tactics.some((item) => stringField(item, "id") === lessonID), `tactic list does not contain ${lessonID}`);

      const showResult = await call(["tactic", "show", lessonID, "--json"]);
      if (!check(showResult.code === 0, `tactic show exit = ${showResult.code}`)) return fail(checks);
      const lesson = field(parse(showResult.stdout), "tactic");
      for (const key of ["principle", "why_it_works", "when_it_works"]) {
        check((stringField(lesson, key) ?? "").length > 0, `tactic show has no ${key}`);
      }
      check(arrayField(lesson, "steps").length >= 3, "tactic show has fewer than 3 steps");
      for (const key of ["folklore", "example", "why_it_fails"]) {
        check((stringField(field(lesson, "boundary"), key) ?? "").length > 0, `tactic boundary has no ${key}`);
      }

      const guidedResult = await callDB(["tactic", "practice", lessonID, "--json"]);
      if (!check(guidedResult.code === 0, `guided awaiting exit = ${guidedResult.code}`)) return fail(checks);
      const guided = parse(guidedResult.stdout);
      check(stringField(guided, "state") === "awaiting_answer", `guided state = ${String(stringField(guided, "state"))}`);
      check(stringField(field(guided, "tactic"), "stage") === "guided", "guided stage is not guided");
      check(stringField(field(guided, "tactic"), "assistance") === "assisted", "guided assistance is not assisted");
      check(field(guided, "guidance") !== undefined, "guided awaiting has no guidance");
      check(stringField(field(guided, "content"), "id") === "tactic-first-party-not-except", "content is not the tactic exercise");
      const guidedQuestion = stringField(field(guided, "question"), "id") ?? "";
      check(guidedQuestion !== "", "guided awaiting has no question id");
      checkKeyHiding(check, guided, "guided awaiting JSON");

      const guidedAnswer = await callDB(["tactic", "practice", lessonID, "--json", "--answer", "A"]);
      if (!check(guidedAnswer.code === 0, `guided answer exit = ${guidedAnswer.code}`)) return fail(checks);
      const guidedAttempt = field(parse(guidedAnswer.stdout), "attempt");
      check(stringField(guidedAttempt, "assistance") === "assisted", "guided attempt assistance is not assisted");
      check(stringField(guidedAttempt, "tactic_id") === lessonID, "guided attempt has no tactic id");
      check(stringField(guidedAttempt, "question_id") === guidedQuestion, "guided attempt question differs from the awaiting question");

      const independentResult = await callDB(["tactic", "practice", lessonID, "--json"]);
      if (!check(independentResult.code === 0, `independent awaiting exit = ${independentResult.code}`)) return fail(checks);
      const independent = parse(independentResult.stdout);
      check(stringField(field(independent, "tactic"), "stage") === "independent", "independent stage is not independent");
      check(stringField(field(independent, "tactic"), "assistance") === "independent", "independent assistance is not independent");
      check(field(independent, "guidance") === undefined, "independent awaiting still carries guidance");
      const independentQuestion = stringField(field(independent, "question"), "id") ?? "";
      check(independentQuestion !== "" && independentQuestion !== guidedQuestion, "independent question is not distinct from the guided question");
      checkKeyHiding(check, independent, "independent awaiting JSON");

      const independentAnswer = await callDB(["tactic", "practice", lessonID, "--json", "--answer", "A"]);
      if (!check(independentAnswer.code === 0, `independent answer exit = ${independentAnswer.code}`)) return fail(checks);
      const independentAttempt = field(parse(independentAnswer.stdout), "attempt");
      check(stringField(independentAttempt, "assistance") === "independent", "independent attempt assistance is not independent");
      check(stringField(independentAttempt, "question_id") === independentQuestion, "independent attempt question differs from the awaiting question");

      const report = parse((await callDB(["report", "--json"])).stdout);
      const counts = field(report, "counts");
      check(numberField(counts, "attempts") === 2, `report attempts = ${String(numberField(counts, "attempts"))}`);
      check(numberField(counts, "assisted_attempts") === 1, `report assisted_attempts = ${String(numberField(counts, "assisted_attempts"))}`);
      check(numberField(counts, "independent_attempts") === 1, `report independent_attempts = ${String(numberField(counts, "independent_attempts"))}`);
      const attempts = arrayField(report, "attempts");
      check(stringField(attempts[0], "assistance") === "assisted", "report first attempt is not assisted");
      check(stringField(attempts[1], "assistance") === "independent", "report second attempt is not independent");

      const exhausted = await callDB(["tactic", "practice", lessonID, "--json", "--stage", "independent"]);
      check(exhausted.code === 2, `exhausted independent exit = ${exhausted.code}`);
      check(stringField(field(parse(exhausted.stderr), "error"), "code") === "not_practiceable", "exhausted independent error code");
    } catch (error) {
      checks.push(error instanceof Error ? error.message : String(error));
    } finally {
      rmSync(temp, { recursive: true, force: true });
    }

    if (checks.length > 0) return fail(checks);
    return pass(`${assertions} assertions`);
  },
};
