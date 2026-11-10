import { existsSync, readdirSync } from "node:fs";

import { BASELINE, EXPECTED_NATIVE, EXPECTED_OCR, EXPECTED_PAGES, decodeUTF8, nonSpaceChars } from "../lib/baseline.ts";
import { readBytes, readText, resolve } from "../lib/repo.ts";
import { fail, pass, skip } from "../lib/types.ts";
import type { Gate } from "../lib/types.ts";

interface ManifestRow {
  page: number;
  source: string;
  bytes: number;
  chars: number;
}

function parseManifest(text: string, problems: string[]): ManifestRow[] {
  const rows: ManifestRow[] = [];
  const lines = text.replace(/\r\n/g, "\n").split("\n");
  for (let index = 0; index < lines.length; index++) {
    const line = lines[index];
    if (line.trim() === "") continue;
    const fields = line.split("\t");
    if (index === 0 && fields[0].toLowerCase() === "page") continue;
    if (fields.length < 5) {
      problems.push(`manifest line ${index + 1}: ${fields.length} fields, want 5`);
      continue;
    }
    const page = Number(fields[0]);
    const bytes = Number(fields[3]);
    const chars = Number(fields[4]);
    if (!Number.isInteger(page) || page < 1 || !Number.isInteger(bytes) || bytes < 0 || !Number.isInteger(chars) || chars < 0) {
      problems.push(`manifest line ${index + 1}: invalid page/bytes/chars`);
      continue;
    }
    rows.push({ page, source: fields[2].toLowerCase(), bytes, chars });
  }
  return rows;
}

function pageFile(number: number): string {
  return `${BASELINE}/pages/page-${String(number).padStart(3, "0")}.txt`;
}

export const baseline: Gate = {
  id: "baseline",
  title: "baseline integrity",
  async run(ctx) {
    if (!existsSync(resolve(ctx.root, BASELINE))) return skip("no page-linked baseline");
    const problems: string[] = [];
    const manifestPath = `${BASELINE}/manifest.tsv`;
    if (!existsSync(resolve(ctx.root, manifestPath))) return fail([`${manifestPath}: missing`]);
    const rows = parseManifest(readText(ctx.root, manifestPath), problems);
    if (rows.length !== EXPECTED_PAGES) {
      problems.push(`manifest rows = ${rows.length}, want ${EXPECTED_PAGES}`);
    }
    const sources: Record<string, number> = {};
    const seen = new Set<number>();
    for (const row of rows) {
      if (seen.has(row.page)) problems.push(`manifest page ${row.page} appears twice`);
      seen.add(row.page);
      sources[row.source] = (sources[row.source] ?? 0) + 1;
      const pagePath = pageFile(row.page);
      if (!existsSync(resolve(ctx.root, pagePath))) {
        problems.push(`${pagePath}: missing`);
        continue;
      }
      const bytes = readBytes(ctx.root, pagePath);
      const text = decodeUTF8(bytes);
      if (text === null) {
        problems.push(`${pagePath}: not valid UTF-8`);
        continue;
      }
      if (bytes.length !== row.bytes) {
        problems.push(`${pagePath}: ${bytes.length} bytes, manifest ${row.bytes}`);
      }
      const chars = nonSpaceChars(text);
      if (chars !== row.chars) {
        problems.push(`${pagePath}: ${chars} non-space chars, manifest ${row.chars}`);
      }
    }
    // A page number outside 1..EXPECTED_PAGES must not hide a missing page.
    for (let page = 1; page <= EXPECTED_PAGES; page++) {
      if (!seen.has(page)) problems.push(`manifest page ${page} is missing`);
    }
    for (const page of seen) {
      if (page < 1 || page > EXPECTED_PAGES) problems.push(`manifest page ${page} is outside 1..${EXPECTED_PAGES}`);
    }

    const pagesDir = resolve(ctx.root, `${BASELINE}/pages`);
    const files = existsSync(pagesDir) ? readdirSync(pagesDir).filter((name) => /^page-\d+\.txt$/.test(name)) : [];
    if (files.length !== EXPECTED_PAGES) {
      problems.push(`page files = ${files.length}, want ${EXPECTED_PAGES}`);
    }
    const fileNumbers = new Set<number>();
    for (const name of files) {
      const match = /^page-(\d+)\.txt$/.exec(name);
      if (match) fileNumbers.add(Number(match[1]));
    }
    for (let page = 1; page <= EXPECTED_PAGES; page++) {
      if (!fileNumbers.has(page)) problems.push(`${pageFile(page)}: missing`);
    }
    for (const page of fileNumbers) {
      if (page < 1 || page > EXPECTED_PAGES) problems.push(`${pageFile(page)}: outside 1..${EXPECTED_PAGES}`);
    }
    if ((sources["native"] ?? 0) !== EXPECTED_NATIVE) {
      problems.push(`native rows = ${sources["native"] ?? 0}, want ${EXPECTED_NATIVE}`);
    }
    if ((sources["ocr"] ?? 0) !== EXPECTED_OCR) {
      problems.push(`OCR rows = ${sources["ocr"] ?? 0}, want ${EXPECTED_OCR}`);
    }
    const readme = `${BASELINE}/README.md`;
    if (!existsSync(resolve(ctx.root, readme))) {
      problems.push(`${readme}: missing`);
    } else if (!/unreviewed/i.test(readText(ctx.root, readme))) {
      problems.push(`${readme}: does not state that the text is unreviewed`);
    }

    for (const file of await ctx.workingTreeFiles()) {
      const isFixture = file.endsWith("_test.go") || /(^|\/)testdata\//.test(file);
      if (!isFixture) continue;
      let text: string;
      try {
        text = readBytes(ctx.root, file).toString("utf8");
      } catch {
        continue;
      }
      if (text.includes("data/baselines")) {
        problems.push(`${file}: test or fixture references the unreviewed baseline`);
      }
    }

    if (problems.length > 0) return fail(problems);
    return pass(`${rows.length} pages (${sources["native"] ?? 0} native, ${sources["ocr"] ?? 0} OCR), 0 fixture references`);
  },
};
