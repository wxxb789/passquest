import { existsSync, mkdtempSync, rmSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { runGo } from "../lib/go.ts";
import { arrayField, exposedKeys, field, numberField, stringField } from "../lib/json.ts";
import { run } from "../lib/process.ts";
import { resolve } from "../lib/repo.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";

export const importSmoke: Gate = {
  id: "import-smoke",
  title: "import smoke",
  async run(ctx: GateContext) {
    const fixture = resolve(ctx.root, "internal/cli/testdata/import");
    if (!existsSync(fixture)) return skip("no import fixture");
    if (!existsSync(resolve(ctx.root, "cmd/passquest"))) return fail(["cmd/passquest is missing"]);

    const temp = mkdtempSync(path.join(tmpdir(), "passquest-import-smoke-"));
    const binary = path.join(temp, process.platform === "win32" ? "passquest.exe" : "passquest");
    const db = path.join(temp, "smoke.sqlite");
    const checks: string[] = [];
    let assertions = 0;
    let skipped = 0;
    const check = (condition: boolean, message: string): boolean => {
      assertions++;
      if (!condition) checks.push(message);
      return condition;
    };
    const call = (args: string[]) => run(binary, [...args, "--db", db], { cwd: ctx.root });
    const parse = (raw: string): unknown => JSON.parse(raw);

    try {
      const build = await runGo(ctx, ["build", "-o", binary, "./cmd/passquest"]);
      if (build.code !== 0) {
        return fail(["go build ./cmd/passquest failed", `${build.stdout}${build.stderr}`.trim()]);
      }

      const importedResult = await call(["import", "--json", "--path", fixture]);
      if (importedResult.code !== 0) {
        return fail([`import failed with exit ${importedResult.code}`, importedResult.stderr.trim()]);
      }
      const imported = parse(importedResult.stdout);
      check(stringField(imported, "state") === "imported", `import state = ${String(stringField(imported, "state"))}`);
      check(numberField(field(imported, "source"), "page_count") === 2, "page count is not 2");
      const sourceID = stringField(field(imported, "source"), "id") ?? "";
      if (!check(sourceID.startsWith("src-"), `source id = ${sourceID}`)) return fail(checks);

      const shown = parse((await call(["show", sourceID, "--page", "2", "--json"])).stdout);
      const shownPage = field(shown, "page");
      check(stringField(shownPage, "origin") === "ocr", "damaged page origin is not ocr");
      check(stringField(shownPage, "review_status") === "unreviewed", "damaged page is not unreviewed");
      const flags = arrayField(shownPage, "flags").map(String);
      for (const flag of ["ocr_unverified", "column_order_unverified"]) {
        check(flags.includes(flag), `damaged page lacks ${flag}`);
      }
      check((stringField(shownPage, "text") ?? "").length > 0, "damaged page text is empty");

      const unreviewedPractice = await call(["practice", "--json", "--source", sourceID]);
      check(unreviewedPractice.code === 2, `unreviewed source practice exit = ${unreviewedPractice.code}`);
      check(stringField(field(parse(unreviewedPractice.stderr), "error"), "code") === "not_practiceable", "unreviewed source practice code");

      const addedResult = await call(["question", "add", sourceID, "1", "--file", path.join(fixture, "question-q1.json"), "--json"]);
      if (!check(addedResult.code === 0, `question add exit = ${addedResult.code}`)) return fail(checks);
      const added = field(parse(addedResult.stdout), "question");
      check(stringField(added, "review_status") === "unreviewed", "new question is not visibly unreviewed");
      check(stringField(added, "answer_status") === "unmapped", "new question is not visibly unmapped");

      const questionUnreviewed = await call(["practice", "--json", "--source", sourceID, "--question", "q1"]);
      check(questionUnreviewed.code === 2, `unreviewed question practice exit = ${questionUnreviewed.code}`);

      const pageReview = await call(["page", "review", sourceID, "1", "--reviewer", "harness", "--json"]);
      check(pageReview.code === 0, `page review exit = ${pageReview.code}`);
      const stillUnreviewed = await call(["practice", "--json", "--source", sourceID, "--question", "q1"]);
      check(stillUnreviewed.code === 2, `question review gate exit = ${stillUnreviewed.code}`);

      const questionReview = await call([
        "question", "review", sourceID, "q1", "--answer", "B", "--reviewer", "harness", "--json",
      ]);
      if (!check(questionReview.code === 0, `question review exit = ${questionReview.code}`)) return fail(checks);

      const awaitingResult = await call(["practice", "--json", "--source", sourceID, "--question", "q1"]);
      if (!check(awaitingResult.code === 0, `awaiting practice exit = ${awaitingResult.code}`)) return fail(checks);
      const awaiting = parse(awaitingResult.stdout);
      check(stringField(awaiting, "state") === "awaiting_answer", "awaiting state is not awaiting_answer");
      const review = field(field(awaiting, "question"), "review");
      check(stringField(review, "page_status") === "reviewed", "awaiting page review is not visible");
      check(stringField(review, "question_status") === "reviewed", "awaiting question review is not visible");
      const forbiddenKeys = ["answer", "correct_label", "correct_text", "evidence", "reasoning", "status", "feedback"];
      const exposed = new Set(exposedKeys(awaiting, forbiddenKeys));
      for (const forbidden of forbiddenKeys) {
        check(!exposed.has(forbidden), `awaiting JSON exposes ${forbidden}`);
      }

      const answeredResult = await call([
        "practice", "--json", "--source", sourceID, "--question", "q1", "--answer", "B",
      ]);
      if (!check(answeredResult.code === 0, `answered practice exit = ${answeredResult.code}`)) return fail(checks);
      const attempt = field(parse(answeredResult.stdout), "attempt");
      check(stringField(attempt, "status") === "correct", "attempt status is not correct");
      const contentID = `${sourceID}-p001`;
      check(stringField(attempt, "content_id") === contentID, "attempt content id does not match the page");

      const report = parse((await call(["report", "--json"])).stdout);
      check(numberField(field(report, "counts"), "attempts") === 1, "report attempts is not 1");
      check(numberField(field(report, "counts"), "first_correct") === 1, "report first_correct is not 1");
      const contents = arrayField(report, "contents");
      check(stringField(contents[0], "id") === contentID, "report content id does not match");

      if (process.platform !== "win32") {
        const mode = statSync(db).mode & 0o777;
        check(mode === 0o600, `database mode = 0${mode.toString(8)}, want 0600`);
      } else {
        // Windows has no POSIX file mode; os.Chmod(0600) does not create an ACL.
        // Report the skip instead of silently dropping the assertion.
        skipped++;
      }
    } catch (error) {
      checks.push(error instanceof Error ? error.message : String(error));
    } finally {
      rmSync(temp, { recursive: true, force: true });
    }

    if (checks.length > 0) return fail(checks);
    const skipNote = skipped > 0 ? `, ${skipped} skipped (POSIX mode unavailable on ${process.platform})` : "";
    return pass(`${assertions} assertions${skipNote}`);
  },
};
