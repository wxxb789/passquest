/**
 * A generated, fully reviewed import source: one placeholder notice per page
 * and one question per page (more on page 1), imported and reviewed through
 * the CLI. Twenty pages with four questions on page 1 make the 23-item unseen
 * pool the assessment gates run on. The text is self-authored filler, never
 * exam material.
 */
import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";

import { field, stringField } from "./json.ts";

function fixturePageText(page: number): string {
  return `Page ${page}. The library closes at 4 p.m. on Saturdays. It opens at 9 a.m. on weekdays.\n`;
}

/** Writes questions/<id>.json for one fixture question (answer B) and returns its id. */
export function writeFixtureQuestion(dir: string, page: number, index: number): string {
  const id = `p${String(page).padStart(2, "0")}-q${index}`;
  writeFileSync(
    path.join(dir, "questions", `${id}.json`),
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
  return id;
}

function writeFixtureTree(dir: string, pages: number, firstPageQuestions: number): Array<{ page: number; id: string }> {
  mkdirSync(path.join(dir, "pages"), { recursive: true });
  mkdirSync(path.join(dir, "questions"), { recursive: true });
  const written: Array<{ page: number; id: string }> = [];
  for (let page = 1; page <= pages; page++) {
    writeFileSync(path.join(dir, "pages", `page-${String(page).padStart(3, "0")}.txt`), fixturePageText(page));
    const count = page === 1 ? firstPageQuestions : 1;
    for (let index = 1; index <= count; index++) {
      written.push({ page, id: writeFixtureQuestion(dir, page, index) });
    }
  }
  return written;
}

/** One `passquest <args> --json` call against the caller's database. */
export type JsonCall = (args: string[]) => Promise<{ code: number; stderr: string; doc: unknown }>;

/**
 * Writes the fixture tree into `dir`, imports it and reviews every page and
 * question. Each step is one assertion; returns the source id, or null after
 * the first failed step.
 */
export async function seedReviewedSource(
  call: JsonCall,
  check: (condition: boolean, message: string) => boolean,
  dir: string,
  pages: number,
  firstPageQuestions: number,
): Promise<string | null> {
  const questions = writeFixtureTree(dir, pages, firstPageQuestions);
  const imported = await call(["import", "--path", dir]);
  if (!check(imported.code === 0, `import ${dir} exit = ${imported.code}: ${imported.stderr}`)) return null;
  const sourceID = stringField(field(imported.doc, "source"), "id");
  if (!sourceID) {
    check(false, `import ${dir} returned no source id`);
    return null;
  }
  for (const question of questions) {
    const file = path.join(dir, "questions", `${question.id}.json`);
    const added = await call(["question", "add", sourceID, String(question.page), "--file", file]);
    if (!check(added.code === 0, `question add ${question.id} exit = ${added.code}: ${added.stderr}`)) return null;
  }
  for (let page = 1; page <= pages; page++) {
    const reviewedPage = await call(["page", "review", sourceID, String(page), "--reviewer", "smoke"]);
    if (!check(reviewedPage.code === 0, `page review ${page} exit = ${reviewedPage.code}: ${reviewedPage.stderr}`)) return null;
  }
  for (const question of questions) {
    const reviewed = await call(["question", "review", sourceID, question.id, "--answer", "B", "--reviewer", "smoke"]);
    if (!check(reviewed.code === 0, `question review ${question.id} exit = ${reviewed.code}: ${reviewed.stderr}`)) return null;
  }
  return sourceID;
}
