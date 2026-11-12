import { copyFileSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { runGo } from "../lib/go.ts";
import { arrayField, field, numberField, stringField } from "../lib/json.ts";
import { run } from "../lib/process.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";

function parse(raw: string): unknown {
  try {
    return JSON.parse(raw);
  } catch {
    return {};
  }
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

interface SpawnResult {
  code: number;
  stdout: string;
  stderr: string;
}

function fixturePageText(page: number): string {
  return `Page ${page}. The library closes at 4 p.m. on Saturdays. It opens at 9 a.m. on weekdays.\n`;
}

function writeFixtureTree(dir: string, pages: number, firstPageQuestions: number): void {
  const pagesDir = path.join(dir, "pages");
  const questionsDir = path.join(dir, "questions");
  mkdirSync(pagesDir, { recursive: true });
  mkdirSync(questionsDir, { recursive: true });
  for (let page = 1; page <= pages; page++) {
    writeFileSync(path.join(pagesDir, `page-${String(page).padStart(3, "0")}.txt`), fixturePageText(page));
    const count = page === 1 ? firstPageQuestions : 1;
    for (let index = 1; index <= count; index++) {
      const id = `p${String(page).padStart(2, "0")}-q${index}`;
      writeFileSync(
        path.join(questionsDir, `${id}.json`),
        JSON.stringify({
          id,
          stem: `Page ${page} question ${index}: When does the library close on Saturdays?`,
          options: [
            { label: "A", text: "At 9 a.m." },
            { label: "B", text: "At 4 p.m." },
          ],
          answer: "B",
          evidence: "closes at 4 p.m. on Saturdays",
          reasoning: `Reasoning for page ${page} question ${index}.`,
          option_reasoning: {
            A: "Nine is the weekday opening time.",
            B: "The passage states 4 p.m. on Saturdays.",
          },
          tactics: [index === 1 ? "not-except-polarity" : "vocabulary-in-context"],
          glossary: [{ term: "closes", meaning: "shuts" }],
        }),
      );
    }
  }
}

function questionIDs(pages: number, firstPageQuestions: number): Array<{ page: number; id: string }> {
  const out: Array<{ page: number; id: string }> = [];
  for (let page = 1; page <= pages; page++) {
    const count = page === 1 ? firstPageQuestions : 1;
    for (let index = 1; index <= count; index++) {
      out.push({ page, id: `p${String(page).padStart(2, "0")}-q${index}` });
    }
  }
  return out;
}

export const assessSmoke: Gate = {
  id: "assess-smoke",
  title: "assess smoke",
  async run(ctx: GateContext) {
    const binaryName = process.platform === "win32" ? "passquest.exe" : "passquest";
    const temp = mkdtempSync(path.join(tmpdir(), "passquest-assess-smoke-"));
    const binary = path.join(temp, binaryName);
    const checks: string[] = [];
    let assertions = 0;
    const check = (condition: boolean, message: string): boolean => {
      assertions++;
      if (!condition) checks.push(message);
      return condition;
    };

    try {
      const build = await runGo(ctx, ["build", "-o", binary, "./cmd/passquest"]);
      if (build.code !== 0) {
        return fail(["go build ./cmd/passquest failed", `${build.stdout}${build.stderr}`.trim()]);
      }

      const call = async (db: string, args: string[], stdin?: string): Promise<SpawnResult> =>
        run(binary, [...args, "--db", db], { cwd: ctx.root, stdin });
      const callJSON = async (db: string, args: string[], stdin?: string) => {
        const result = await call(db, [...args, "--json"], stdin);
        return { ...result, doc: parse(result.stdout), failure: parse(result.stderr) };
      };
      const spawnWithStdin = (db: string, args: string[]) =>
        Bun.spawn({
          cmd: [binary, ...args, "--db", db],
          cwd: ctx.root,
          stdin: "pipe",
          stdout: "pipe",
          stderr: "pipe",
        });
      const collect = async (proc: ReturnType<typeof spawnWithStdin>): Promise<SpawnResult> => {
        const [stdout, stderr] = await Promise.all([
          new Response(proc.stdout).text(),
          new Response(proc.stderr).text(),
        ]);
        const code = await proc.exited;
        return { code, stdout, stderr };
      };

      const seedSource = async (db: string, dir: string, pages: number, firstPageQuestions: number): Promise<string | null> => {
        writeFixtureTree(dir, pages, firstPageQuestions);
        const imported = await callJSON(db, ["import", "--path", dir]);
        if (!check(imported.code === 0, `import ${dir} exit = ${imported.code}: ${imported.stderr}`)) return null;
        const sourceID = stringField(field(imported.doc, "source"), "id");
        if (!sourceID) {
          check(false, `import ${dir} returned no source id`);
          return null;
        }
        const questions = questionIDs(pages, firstPageQuestions);
        for (const question of questions) {
          const added = await callJSON(db, ["question", "add", sourceID, String(question.page), "--file", path.join(dir, "questions", `${question.id}.json`)]);
          if (!check(added.code === 0, `question add ${question.id} exit = ${added.code}: ${added.stderr}`)) return null;
        }
        for (let page = 1; page <= pages; page++) {
          const reviewedPage = await callJSON(db, ["page", "review", sourceID, String(page), "--reviewer", "smoke"]);
          if (!check(reviewedPage.code === 0, `page review ${page} exit = ${reviewedPage.code}: ${reviewedPage.stderr}`)) return null;
        }
        for (const question of questions) {
          const reviewed = await callJSON(db, ["question", "review", sourceID, question.id, "--answer", "B", "--reviewer", "smoke"]);
          if (!check(reviewed.code === 0, `question review ${question.id} exit = ${reviewed.code}: ${reviewed.stderr}`)) return null;
        }
        return sourceID;
      };

      const richDir = path.join(temp, "rich-source");
      const richDB = path.join(temp, "rich.sqlite");
      const richSource = await seedSource(richDB, richDir, 20, 4);
      if (!richSource) return fail(checks);
      const copy = (name: string): string => {
        const target = path.join(temp, `${name}.sqlite`);
        copyFileSync(richDB, target);
        return target;
      };

      // 1. Holdout per content unit: one exposed question spends all four on
      // page 1, and a fresh page still counts.
      const poolDB = copy("pool");
      const poolBefore = await callJSON(poolDB, ["assess", "pool"]);
      if (!check(poolBefore.code === 0, `pool before exit = ${poolBefore.code}: ${poolBefore.stderr}`)) return fail(checks);
      check(numberField(poolBefore.doc, "qualifying_items") === 23, "pool before items");
      check(numberField(poolBefore.doc, "qualifying_units") === 20, "pool before units");
      const practiced = await callJSON(poolDB, ["practice", "--source", richSource, "--page", "1", "--question", "p01-q1", "--answer", "B"]);
      check(practiced.code === 0, `practice exit = ${practiced.code}: ${practiced.stderr}`);
      const poolAfter = await callJSON(poolDB, ["assess", "pool"]);
      check(numberField(poolAfter.doc, "qualifying_items") === 19, "spending one question did not remove all four siblings");
      check(numberField(poolAfter.doc, "qualifying_units") === 19, "spent unit count");
      const units = arrayField(poolAfter.doc, "content_units");
      const spentUnit = units.find((unit) => stringField(unit, "content_id") === `${richSource}-p001`);
      const freshUnit = units.find((unit) => stringField(unit, "content_id") === `${richSource}-p002`);
      check(asRecord(spentUnit).spent === true && numberField(spentUnit, "question_count") === 4, "page 1 is not shown spent with four questions");
      check(asRecord(freshUnit).spent === false, "the fresh page no longer counts");

      // 9. Pool floor: a four-item source refuses with the shortfall in items
      // and the unit count behind it.
      const smallDir = path.join(temp, "small-source");
      const smallDB = path.join(temp, "small.sqlite");
      if (!(await seedSource(smallDB, smallDir, 4, 1))) return fail(checks);
      const floor = await callJSON(smallDB, ["assess", "start", "--size", "20"]);
      check(floor.code === 2, `pool floor exit = ${floor.code}`);
      check(stringField(field(floor.failure, "error"), "code") === "pool_floor", "pool floor code");
      check((stringField(field(floor.failure, "error"), "message") ?? "").includes("4 qualifying item(s)"), "pool floor item shortfall");
      check((stringField(field(floor.failure, "error"), "message") ?? "").includes("16 more"), "pool floor missing shortfall");
      check((stringField(field(floor.failure, "error"), "message") ?? "").includes("4 unseen content unit(s)"), "pool floor unit count");

      // 3 and 4 and 7. A completed run records elapsed time, hides annotations
      // during the run and prints them with the unknowns in the report.
      const runDB = copy("run");
      const runArgs = ["assess", "start", "--size", "20", "--minutes", "60", "--json"];
      const running = spawnWithStdin(runDB, runArgs);
      const runningResult = collect(running);
      running.stdin!.write("B\n".repeat(20));
      running.stdin!.end();
      const runResult = await runningResult;
      check(runResult.code === 0, `run exit = ${runResult.code}: ${runResult.stderr}`);
      check(!runResult.stdout.includes("Reasoning for") && !runResult.stdout.includes("shuts"), "run output exposed annotations");
      const runDoc = asRecord(parse(runResult.stdout));
      const runID = numberField(asRecord(field(runDoc, "run")), "id");
      if (!check(runID !== undefined && runID > 0, "run id")) return fail(checks);
      const runItems = arrayField(runDoc, "items");
      check(runItems.length === 20, `run items = ${runItems.length}`);
      check(runItems.every((item) => typeof numberField(item, "elapsed_ms") === "number" && (numberField(item, "elapsed_ms") ?? 0) >= 0), "an item has no stored elapsed_ms");
      const report = await callJSON(runDB, ["assess", "report", String(runID)]);
      if (!check(report.code === 0, `report exit = ${report.code}: ${report.stderr}`)) return fail(checks);
      check(numberField(field(report.doc, "counts"), "total") === 20, "report total");
      check(numberField(field(report.doc, "counts"), "scored") === 20, "report scored");
      check(numberField(field(report.doc, "accuracy"), "correct") === 20, "report correct");
      const reportItems = arrayField(report.doc, "items");
      check(asRecord(field(reportItems[0], "annotations")).available === true, "report withheld annotations");
      check((stringField(field(reportItems[0], "annotations"), "reasoning") ?? "").includes("Reasoning for"), "report reasoning missing");
      check(arrayField(field(reportItems[0], "annotations"), "glossary").length === 1, "report glossary missing");
      const unknowns = arrayField(report.doc, "unknowns").map((value) => String(value));
      check(unknowns.includes("pass threshold: unknown"), "report does not state the unknown pass threshold");
      check(!report.stdout.toLowerCase().includes("\"verdict\""), "report invented a verdict");

      // 2. Coaching is refused while the run is active. The run is interrupted
      // by killing the process, so the active state survives.
      const coachingDB = copy("coaching");
      const coachingRun = spawnWithStdin(coachingDB, ["assess", "start", "--size", "20", "--minutes", "60", "--json"]);
      coachingRun.stdin!.write("B\n");
      await Bun.sleep(300);
      coachingRun.kill();
      await collect(coachingRun);
      const active = await callJSON(coachingDB, ["assess", "pool"]);
      check(active.code === 0, "pool while active");
      const tutor = await callJSON(coachingDB, ["tutor", "history"]);
      const skill = await callJSON(coachingDB, ["skill", "show", "vocabulary-in-context"]);
      const find = await callJSON(coachingDB, ["find"]);
      check(tutor.code === 2 && stringField(field(tutor.failure, "error"), "code") === "assessment_active", `tutor during run = ${tutor.code}`);
      check(skill.code === 2 && stringField(field(skill.failure, "error"), "code") === "assessment_active", `skill during run = ${skill.code}`);
      check(find.code === 2 && stringField(field(find.failure, "error"), "code") === "assessment_active", `find during run = ${find.code}`);

      // 3. Forced per-item timeout: the first answer arrives after the 3-second
      // item clock, and the remaining items answer immediately.
      const timeoutDB = copy("timeout");
      const timeoutRun = spawnWithStdin(timeoutDB, ["assess", "start", "--size", "20", "--minutes", "1", "--json"]);
      const timeoutResult = collect(timeoutRun);
      await Bun.sleep(3200);
      timeoutRun.stdin!.write("B\n");
      timeoutRun.stdin!.write("B\n".repeat(19));
      timeoutRun.stdin!.end();
      const timeout = await timeoutResult;
      check(timeout.code === 0, `timeout run exit = ${timeout.code}: ${timeout.stderr}`);
      const timeoutID = numberField(asRecord(field(parse(timeout.stdout), "run")), "id");
      if (!check(timeoutID !== undefined, "timeout run id")) return fail(checks);
      const timeoutReport = await callJSON(timeoutDB, ["assess", "report", String(timeoutID)]);
      check(numberField(field(timeoutReport.doc, "counts"), "timed_out") === 1, "forced timeout was not counted");
      const timedItems = arrayField(timeoutReport.doc, "items").filter((item) => field(item, "timed_out") === true);
      check(timedItems.length === 1, "timed-out item is not listed");
      check(numberField(timedItems[0], "elapsed_ms") === 3000, `timed-out elapsed = ${numberField(timedItems[0], "elapsed_ms")}`);

      // 8. Interrupt and resume: the report marks the resumed portion.
      const resumeDB = copy("resume");
      const resumeRun = spawnWithStdin(resumeDB, ["assess", "start", "--size", "20", "--minutes", "60", "--json"]);
      const resumeCollected = collect(resumeRun);
      resumeRun.stdin!.write("B\n".repeat(5));
      await Bun.sleep(300);
      resumeRun.kill();
      await resumeCollected;
      // The copy was created fresh, so the interrupted run is id 1.
      const resumeReportID = 1;
      const resumed = await callJSON(resumeDB, ["assess", "resume", String(resumeReportID)], "B\n".repeat(15));
      if (!check(resumed.code === 0, `resume exit = ${resumed.code}: ${resumed.stderr}`)) return fail(checks);
      const resumedReport = await callJSON(resumeDB, ["assess", "report", String(resumeReportID)]);
      check(numberField(asRecord(field(resumedReport.doc, "run")), "resume_count") === 1, "resume count");
      const afterResume = numberField(field(resumedReport.doc, "counts"), "answered_after_resume") ?? 0;
      check(afterResume > 0, "answered-after-resume count");
      check(arrayField(resumedReport.doc, "items").filter((item) => field(item, "after_resume") === true).length === afterResume, "resumed items are not marked");

      // 5. Contamination: a show on a future unit during the run is prior
      // exposure, excluded from the headline and listed.
      const contaminationDB = copy("contamination");
      const contaminationRun = spawnWithStdin(contaminationDB, ["assess", "start", "--size", "20", "--minutes", "60", "--json"]);
      const contaminationResult = collect(contaminationRun);
      contaminationRun.stdin!.write("B\nB\n");
      await Bun.sleep(300);
      const shown = await callJSON(contaminationDB, ["show", richSource, "--page", "15"]);
      check(shown.code === 0, `show during run = ${shown.code}: ${shown.stderr}`);
      contaminationRun.stdin!.write("B\n".repeat(18));
      contaminationRun.stdin!.end();
      const contaminated = await contaminationResult;
      check(contaminated.code === 0, `contaminated run exit = ${contaminated.code}: ${contaminated.stderr}`);
      const contaminatedID = numberField(asRecord(field(parse(contaminated.stdout), "run")), "id");
      if (!check(contaminatedID !== undefined, "contaminated run id")) return fail(checks);
      const contaminatedReport = await callJSON(contaminationDB, ["assess", "report", String(contaminatedID)]);
      check(numberField(field(contaminatedReport.doc, "counts"), "contaminated") === 1, "contamination was averaged away");
      check(numberField(field(contaminatedReport.doc, "counts"), "scored") === 19, "contaminated item stayed in the headline");
      const excluded = arrayField(field(contaminatedReport.doc, "contamination"), "excluded_items");
      check(excluded.length === 1, "contaminated item is not listed");
      check(arrayField(excluded[0], "contamination_reasons").includes("prior_exposure"), "contamination reason");

      // 6. Compare refuses unlike runs and names the dimension. A fresh run
      // also needs 20 qualifying items left, so this database carries 43.
      const compareDB = path.join(temp, "compare.sqlite");
      const compareDir = path.join(temp, "compare-source");
      if (!(await seedSource(compareDB, compareDir, 40, 4))) return fail(checks);
      const runA = await callJSON(compareDB, ["assess", "start", "--size", "8", "--minutes", "60"], "B\n".repeat(8));
      const runB = await callJSON(compareDB, ["assess", "start", "--size", "8", "--minutes", "30"], "B\n".repeat(8));
      const idA = numberField(asRecord(field(runA.doc, "run")), "id");
      const idB = numberField(asRecord(field(runB.doc, "run")), "id");
      if (!check(idA !== undefined && idB !== undefined, `compare run ids: A=${runA.code} ${runA.stderr.slice(0, 200)} B=${runB.code} ${runB.stderr.slice(0, 200)}`)) return fail(checks);
      const refused = await callJSON(compareDB, ["assess", "compare", String(idA), String(idB)]);
      check(refused.code === 2, `compare exit = ${refused.code}`);
      check(stringField(field(refused.failure, "error"), "code") === "incomparable_runs", "compare error code");
      check((stringField(field(refused.failure, "error"), "message") ?? "").includes("window_minutes"), "compare did not name the window dimension");
      const selfCompare = await callJSON(compareDB, ["assess", "compare", String(idA), String(idA)]);
      check(selfCompare.code === 0 && stringField(selfCompare.doc, "state") === "compared", "matching compare refused");
    } catch (error) {
      checks.push(error instanceof Error ? error.message : String(error));
    } finally {
      rmSync(temp, { recursive: true, force: true });
    }

    if (checks.length > 0) return fail(checks);
    return pass(`${assertions} assertions`);
  },
};
