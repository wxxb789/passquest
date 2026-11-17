import { randomBytes } from "node:crypto";
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { startAnkiStub } from "../lib/anki-stub.ts";
import { buildDesktop, removeTree } from "../lib/desktop.ts";
import {
  chatReply,
  cliJson,
  desktopSurface,
  launchPage,
  pageScript,
  SKILL,
  startProviderStub,
  STUB_CREDENTIAL_ENV,
  STUB_MODEL,
  STUB_PROVIDER,
  tail,
  writeStubRegistry,
} from "../lib/desktop-page.ts";
import type { Check } from "../lib/desktop-page.ts";
import { runGo } from "../lib/go.ts";
import { arrayField, field, numberField, stringField } from "../lib/json.ts";
import { executable, run } from "../lib/process.ts";
import { resolve } from "../lib/repo.ts";
import { seedReviewedSource } from "../lib/reviewed-source.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";
import { UX_LIBRARY } from "../lib/ux-page.ts";

/**
 * The visual and interaction quality budget of the desktop window (goal 0012,
 * Q-0012-01). On a database seeded through the CLI and the window itself, it
 * audits each state below in light/dark × zh/en at each size with the in-page
 * library of harness/lib/ux-page.ts. Every run first proves the library on
 * planted defects. A finding fails the gate unless harness/ux-allow.json names
 * it with a reason; an allowlist entry that matches nothing fails too, so the
 * list can only shrink.
 *
 * Each state's primary action is the one visible element marked
 * `data-primary` in the state's root (the open modal layer, else the page).
 */
export const UX_SIZES: ReadonlyArray<readonly [number, number]> = [
  [1280, 800],
  [1440, 844],
];
const ALLOWLIST = "harness/ux-allow.json";
const MODES = ["light-zh", "light-en", "dark-zh", "dark-en"];
/** Every state PART_ONE and PART_TWO audit; each must be audited once per mode at each size. */
export const UX_STATES = [
  "today",
  "practice-sheet",
  "shortcut-sheet",
  "practice-independent",
  "hint-gate",
  "feedback-coach",
  "anki-edit",
  "anki-refine",
  "practice-guided",
  "report",
  "report-annotations",
  "anki-notes",
  "settings-row",
  "run-form",
  "assess-running",
  "practice-long",
  "anki-long-edit",
  "anki-long",
  "anki-long-phone",
  "consent-long",
  "assess-many",
];

export interface Audited {
  size: string;
  state: string;
  mode: string;
}

/** What the audits left out or did twice: a state skipped silently would otherwise pass. */
export function coverageProblems(audited: Audited[], sizes: string[]): string[] {
  const problems: string[] = [];
  const count = new Map<string, number>();
  for (const entry of audited) {
    const key = `${entry.size} ${entry.state} ${entry.mode}`;
    count.set(key, (count.get(key) ?? 0) + 1);
    if (!UX_STATES.includes(entry.state)) problems.push(`coverage: ${entry.size} audited an undeclared state ${entry.state}`);
  }
  for (const size of sizes) {
    for (const state of UX_STATES) {
      const missing = MODES.filter((mode) => !count.has(`${size} ${state} ${mode}`));
      if (missing.length > 0) problems.push(`coverage: ${size} ${state} was not audited in ${missing.join(", ")}`);
      const twice = MODES.filter((mode) => (count.get(`${size} ${state} ${mode}`) ?? 0) > 1);
      if (twice.length > 0) problems.push(`coverage: ${size} ${state} was audited more than once in ${twice.join(", ")}`);
    }
  }
  return [...new Set(problems)];
}
const COACH_REPLY =
  "Before you look at the key: which word in option C would you check against the passage? In an EXCEPT item three options are supported by the notice.";

/** Fails the launch's script unless the page area is exactly the size asked for (Q-0012-05). */
function viewportGuard(width: number, height: number): string {
  return `
{
  const deadline = Date.now() + 3000;
  while ((innerWidth !== ${width} || innerHeight !== ${height}) && Date.now() < deadline) await sleep(100);
  if (innerWidth !== ${width} || innerHeight !== ${height}) return { precondition: { width: innerWidth, height: innerHeight } };
}
${UX_LIBRARY}
`;
}

/** Creates what the audited states show: answered attempts, an accepted note and a finished run. */
const PREPARE = `
await today();
press("Enter");
await answer("1");
press("k");
await until("the card preview", () => q('[data-testid="card-preview"]'), 20000);
await click("#acceptbtn");
await until("the accepted note", () => q('[data-testid="note"]'), 20000);
await today();
press("Enter");
await answer("2");
location.hash = "#/assess";
await click('[data-act="run-ask"]');
await until("the run form", () => q('#runform input[name="size"]'));
q('#runform input[name="size"]').value = "3";
q('#runform input[name="minutes"]').value = "30";
await click('[data-act="run-start"]');
for (let i = 0; i < 3; i++) {
  const item = await until("assessment item " + (i + 1), () => q('[data-testid="assess-item"]'));
  const position = item.dataset.position;
  await sleep(150);
  press(String(1 + (i % 2)));
  await until("a selected option", () => q('.opt[aria-checked="true"]'));
  press("Enter");
  await until("the next item or the report", () => q('[data-testid="assess-item"]')?.dataset.position !== position || q('[data-testid="report"]'));
}
await until("the run report", () => q('[data-testid="report"]'));
return { report: data('[data-testid="report"]').run ?? null };
`;

/**
 * State audits share these helpers. Animations are settled so colours are
 * final; each state is audited in the four modes and left in light/zh.
 * Focus checks record where focus went after a scripted action.
 */
const DRIVE = `
const settle = document.createElement("style");
settle.textContent = "*,*::before,*::after{animation:none!important;transition:none!important;caret-color:transparent!important}";
document.head.append(settle);
const MODES = [["light", "zh"], ["light", "en"], ["dark", "zh"], ["dark", "en"]];
const results = [];
const focus = [];
let checked = 0;
const setMode = async (theme, lang) => {
  if (document.documentElement.dataset.theme !== theme) q('[data-act="theme"]').click();
  if (document.documentElement.lang !== (lang === "zh" ? "zh-CN" : "en")) q('[data-act="lang"][data-v="' + lang + '"]').click();
  await sleep(120);
};
const loading = () => q('#stage [aria-busy="true"]');
// A probe that has held for a while with no loading card: a render that a pending load is
// about to replace does not count. Resolves to the probe's latest value.
const steady = (what, probe, holdMs = 500) => {
  let since = 0;
  return until(what + " to settle", () => {
    const value = loading() ? null : probe();
    if (!value) {
      since = 0;
      return null;
    }
    since ||= Date.now();
    return Date.now() - since >= holdMs ? value : null;
  });
};
const each = async (state, withPrimary) => {
  await steady(state + " without a loading card", () => true, 0);
  for (const [theme, lang] of MODES) {
    await setMode(theme, lang);
    const audit = UX.audit(state);
    const issues = withPrimary ? audit.issues.concat(UX.primary()) : audit.issues;
    const html = document.documentElement;
    results.push({ state, mode: theme + "-" + lang, shown: html.dataset.theme + "-" + (html.lang === "en" ? "en" : html.lang === "zh-CN" ? "zh" : html.lang), issues });
  }
  await setMode("light", "zh");
};
const lost = () => !document.activeElement || document.activeElement === document.body || document.activeElement === document.documentElement;
const after = (action) => {
  checked++;
  if (lost()) focus.push({ check: "focus-lost", el: action });
};
const layerOpen = () => {
  const modal = q('#layer [aria-modal="true"]');
  return modal && UX.visible(modal);
};
const openDialog = async (name, opener, open) => {
  if (opener) opener.focus();
  open();
  await until(name, layerOpen, 10000);
  await sleep(150);
  checked++;
  if (!q("#layer").contains(document.activeElement)) focus.push({ check: "dialog-focus-outside", el: name, active: UX.path(document.activeElement) });
  return opener ? UX.path(opener) : null;
};
const closeDialog = async (name, openerPath) => {
  press("Escape");
  await until(name + " to close", () => !layerOpen(), 5000);
  await sleep(150);
  checked++;
  if (lost()) focus.push({ check: "dialog-focus-lost", el: name });
  else if (openerPath && UX.path(document.activeElement) !== openerPath) focus.push({ check: "dialog-focus-not-restored", el: name, active: UX.path(document.activeElement) });
};
let opener = null;
let back = null;
await setMode("light", "zh");
`;

/** The short lines of the long card's Back (anki-long). */
const LONG_BACK_LINES = 50;

/** Today, its two sheets, an independent item (and its hint gate), feedback with the coach, the Anki editor and its refine panel, then the card with a long Back in the editor and at desktop and phone width. */
const PART_ONE = `
await today();
await each("today", true);
opener = q('[data-act="sheet"][data-v="pick"]');
back = await openDialog("practice sheet", opener, () => opener.click());
await each("practice-sheet", true);
await closeDialog("practice sheet", back);
await openDialog("shortcut sheet", null, () => press("?"));
await each("shortcut-sheet", true);
await closeDialog("shortcut sheet", null);
opener = q('[data-act="sheet"][data-v="pick"]');
await openDialog("practice sheet", opener, () => opener.click());
await click('#layer [data-act="start-skill"][data-v="${SKILL}"]');
await until("an independent item", () => q('[data-testid="practice-item"]'));
await sleep(250);
after("start from the practice sheet");
await each("practice-independent", true);
// The skill's independent item has an authored hint; a missing hint button fails the script.
opener = await until("the hint button", () => q('[data-act="hint"]'), 5000);
back = await openDialog("hint gate", opener, () => opener.click());
await each("hint-gate", false);
await closeDialog("hint gate", back);
press("2");
await until("a selected option", () => q('.opt[aria-checked="true"]'));
press("Enter");
await until("the feedback card", () => q('[data-testid="feedback"]'));
await sleep(250);
after("submit an answer");
await until("the coach preview", () => q('[data-act="coach-send"]') && !q('[data-act="coach-send"]').disabled, 20000);
q('[data-act="coach-send"]').click();
await until("the coach reply", () => q('[data-testid="coach-reply"]') || q('[data-testid="coach-failure"]'), 30000);
await sleep(200);
await each("feedback-coach", true);
press("k");
await until("the card preview", () => q('[data-testid="card-preview"]'), 20000);
await sleep(250);
after("propose a note");
press("e");
await until("the note editor", () => q('[data-field="front"]'));
await sleep(200);
await each("anki-edit", true);
await click('.esheet [data-act="refine"]');
await steady("the refine preview", () => q('.es-refine [data-act="refine-send"]:not([disabled])'));
after("open the refine panel");
await each("anki-refine", true);
await click('[data-act="refine-close"]');
await until("the closed refine panel", () => !q('[data-testid="refine"]'));
// A Back of ${LONG_BACK_LINES} short lines (within MaxBackRunes) makes the answer card taller than the stage:
// the preview scrolls with Accept in view, and both edges of each card frame are reachable; with the
// editor open, the preview scrolled to its end shows the card's end above the sheet, not behind it.
const preview = () => q('[data-testid="card-preview"]');
const shown = preview().dataset.sha;
const backField = q('[data-field="back"]');
backField.value = Array.from({ length: ${LONG_BACK_LINES} }, (_, i) => "Line " + (i + 1) + ".").join("\\n");
backField.dispatchEvent(new Event("input", { bubbles: true }));
await until("the long card's preview", () => preview() && preview().dataset.sha !== shown, 20000);
const answerFrame = () => q('[data-testid="card-frame"][data-side="answer"]');
await steady("the long answer frame fitted in the editor", () => q(".esheet") && answerFrame() && answerFrame().clientHeight > 900);
await each("anki-long-edit", true);
const table = q("#preview");
table.scrollTop = table.scrollHeight;
const under = answerFrame().getBoundingClientRect().bottom - q(".esheet").getBoundingClientRect().top;
checked++;
if (under > 1) focus.push({ check: "card-end-under-sheet", el: UX.path(answerFrame()), under: Math.round(under) });
table.scrollTop = 0;
await click('.esheet [data-act="edit"]');
await steady("the long answer frame fitted", () => !q(".esheet") && answerFrame() && answerFrame().clientWidth > 300 && answerFrame().clientHeight > 900);
await each("anki-long", true);
await click('[data-act="cwidth"][data-v="phone"]');
await steady("the long answer frame at phone width", () => answerFrame() && answerFrame().clientWidth <= 300 && answerFrame().clientHeight > 900);
await each("anki-long-phone", true);
return { results, focus, checked };
`;

/** The recommended item and the next one, a guided item, the report and its annotations, the Anki notes list, Settings, the dock, the run form, a running assessment. */
const PART_TWO = `
await today();
press("Enter");
await until("the recommended item", () => q('[data-testid="practice-item"]'));
await sleep(250);
after("Enter on Today");
await answer("1");
await sleep(250);
after("submit an answer");
press("n");
await until("the next item", () => q('[data-testid="practice-item"]') || q('[data-testid="hero"]'));
await sleep(250);
after("Next on feedback");
if (q('[data-testid="practice-item"]')) await answer("1");
await startFromSheet("start-tactic", "${SKILL}|guided");
await sleep(250);
after("start a guided item");
await each("practice-guided", true);
location.hash = "#/report";
await steady("the run report", () => q('[data-testid="report"]') && q('[data-act="sheet"][data-v="ann"]'));
await each("report", true);
opener = await steady("the annotations button", () => q('[data-act="sheet"][data-v="ann"]'));
back = await openDialog("annotation sheet", opener, () => opener.click());
await click('#layer [data-act="ann"]');
await sleep(250);
await each("report-annotations", true);
await closeDialog("annotation sheet", back);
location.hash = "#/anki";
// The prepared data holds one accepted, not yet pushed note and no open draft.
await steady("the notes list", () => q('[data-testid="note"]') && !q('[data-testid="anki"][data-draft]') && q('[data-act="push"]'));
await each("anki-notes", true);
location.hash = "#/settings";
// Settings first draws the cached registry, then its reloads may swap in the loading card:
// use the row only once it has held, and in the same tick it was found.
opener = await steady("the provider rows", () => q('[data-act="row"][data-v="${STUB_PROVIDER}"]'));
opener.focus();
opener.click();
await steady("the expanded row", () => q(".rrow.open"));
after("expand a provider row");
await each("settings-row", true);
for (const href of all(".dock a[href]").map((link) => link.getAttribute("href"))) {
  const link = q('.dock a[href="' + href + '"]');
  link.focus();
  link.click();
  await sleep(300);
  after("dock " + href);
}
location.hash = "#/assess";
await until("the run button", () => q('[data-act="run-ask"]'));
await sleep(200);
opener = q('[data-act="run-ask"]');
back = await openDialog("run form", opener, () => opener.click());
await each("run-form", true);
await closeDialog("run form", back);
await click('[data-act="run-ask"]');
await until("the run form", () => q('#runform input[name="size"]'));
q('#runform input[name="size"]').value = "3";
q('#runform input[name="minutes"]').value = "30";
await click('[data-act="run-start"]');
await until("the first assessment item", () => q('[data-testid="assess-item"]'));
await sleep(250);
after("start a run");
await each("assess-running", true);
return { results, focus, checked };
`;

/** The question of the long source (seeded on its own database). */
const LONG_QUESTION = "p01-q1";

/**
 * An imported question whose stem and four options run to several sentences each, opened by the
 * recommendation (its guided walkthrough and one skill attempt are recorded through the CLI). The
 * audit's primary check needs Submit in view and its unreachable check every option within a scroll
 * the learner can make; choosing the last option by key must bring it into view.
 */
const PART_LONG = `
await today();
press("Enter");
const item = await until("the long imported item", () => q('[data-testid="practice-item"]'));
if (item.dataset.question !== "${LONG_QUESTION}") throw new Error("the recommendation opened " + item.dataset.question + ", not the long question");
await sleep(250);
after("Enter on Today");
await each("practice-long", true);
const last = all(".opt").pop();
press(last.dataset.v);
await until("the last option to be selected", () => last.getAttribute("aria-checked") === "true");
await sleep(150);
checked++;
const hidden = UX.hidden(last);
if (hidden) focus.push(Object.assign({ check: "chosen-option-hidden", el: UX.path(last) }, hidden));
return { results, focus, checked };
`;

/** The long questions on the first page of the many-item source (seeded on its own database): five rows in the run's navigator. */
const MANY_ITEMS = 100;
/** The many-item source's title, self-authored: an import title has no length limit (internal/source/import.go). */
const LONG_TITLE = `Long title ${"of a self-authored placeholder notice about library opening hours ".repeat(10)}`.slice(0, 600);

/**
 * A source with a page of MANY_ITEMS long questions and a long title. Its every-provider consent dialog,
 * opened from the preview of a note drafted through the CLI (its text is withheld until that grant), names
 * the title twice and must keep Cancel and Allow reachable; a run over the whole pool grows the item navigator
 * to five rows, which must leave the long question card's Submit uncovered.
 */
const PART_MANY = `
location.hash = "#/anki";
// The seeded draft's text is withheld until the every-provider grant: the preview offers it.
opener = await steady("the every-provider grant", () => q('[data-testid="preview-error"] [data-act="consent-all-ask"]:not([disabled])'));
back = await openDialog("consent dialog", opener, () => opener.click());
await steady("the dialog with the source title", () => q('#layer [data-testid="consent-scope"]') && q("#dca") && q("#dca").textContent.includes("Long title"));
await each("consent-long", false);
await closeDialog("consent dialog", back);
location.hash = "#/assess";
await click('[data-act="run-ask"]');
await until("the run form", () => q('#runform input[name="size"]'));
q('#runform input[name="size"]').value = "${MANY_ITEMS}";
q('#runform input[name="minutes"]').value = "1440";
await click('[data-act="run-start"]');
await steady("the first item of the long run", () => q('[data-testid="assess-item"]') && all('[data-testid="navigator"] .cells .cell').length > 80);
after("start a long run");
await each("assess-many", true);
return { results, focus, checked };
`;

/**
 * The planted-defect probes, then motion: with motion allowed, leaving an
 * independent item for the assessment room must run the room transition; with
 * reduced motion emulated, no animation may run longer than 1 ms or loop, and a
 * style change (theme, sheet, answer, route) may start no CSS transition at all.
 */
const MOTION = `
await today();
const probes = await UX.probes(q('[data-testid="hero"]'));
UX.emulateNoPreference();
await startFromSheet("start-skill", "${SKILL}");
// Only transitions the condition change starts count. What entering practice started may still be
// running, and in a window WebKit does not show (macOS runs this window hidden) CSS animations are
// suspended and never end, so waiting for them would never return: they are set aside by identity
// instead. A transition that changes its end value is a new animation (CSS Transitions, "starting of
// transitions"), so the room change's own transitions are never among them.
const earlier = new Set(document.getAnimations());
const before = document.body.dataset.room;
location.hash = "#/assess";
await until("the assessment room", () => document.body.dataset.room !== before, 5000);
const room = { before, after: document.body.dataset.room, transitions: UX.roomTransitions(earlier) };
// Motion allowed ends here: its transitions end as they would on screen before reduced motion starts.
for (const a of document.getAnimations()) if (a.transitionProperty && a.playState === "running") a.finish();
UX.emulateReducedMotion();
const motion = [];
// A style change under reduced motion applies at once: no transition may start, read in the same task
// as the change (before any frame could end a short one), and none may be left after it.
const snap = async (label) => {
  motion.push(...UX.transitionReport(label));
  await sleep(60);
  motion.push(...UX.motionReport(label), ...UX.transitionReport(label + " later"));
};
await today();
await snap("today");
q('[data-act="theme"]').click();
await snap("theme");
q('[data-act="theme"]').click();
await snap("theme back");
q('[data-act="sheet"][data-v="pick"]').click();
await snap("open sheet");
press("Escape");
await snap("close sheet");
press("Enter");
await until("a practice item", () => q('[data-testid="practice-item"]'));
await snap("practice");
press("1");
await until("a selected option", () => q('.opt[aria-checked="true"]'));
await snap("select");
press("Enter");
await until("the feedback card", () => q('[data-testid="feedback"]'));
await snap("feedback");
for (const route of ["report", "anki", "settings", "assess"]) {
  location.hash = "#/" + route;
  await sleep(300);
  await snap(route);
}
return { probes, room, motion };
`;

interface AllowEntry {
  check: string;
  element: string;
  states?: string[];
  reason: string;
}

export interface Finding {
  check: string;
  el: string;
  state: string;
  mode: string;
  size: string;
  detail: string;
}

function readAllowlist(root: string): { entries: AllowEntry[] } | { error: string } {
  const file = resolve(root, ALLOWLIST);
  if (!existsSync(file)) return { error: `${ALLOWLIST} is missing` };
  let doc: unknown;
  try {
    doc = JSON.parse(readFileSync(file, "utf8"));
  } catch (error) {
    return { error: `${ALLOWLIST} is not JSON: ${error instanceof Error ? error.message : String(error)}` };
  }
  if (numberField(doc, "version") !== 1) return { error: `${ALLOWLIST}: version must be 1` };
  const entries: AllowEntry[] = [];
  for (const [index, entry] of arrayField(doc, "entries").entries()) {
    const check = stringField(entry, "check");
    const element = stringField(entry, "element");
    const reason = stringField(entry, "reason")?.trim();
    const states = field(entry, "states");
    if (!check || !element || !reason) return { error: `${ALLOWLIST}: entry ${index} needs check, element and a reason` };
    if (states !== undefined && !(Array.isArray(states) && states.every((state) => typeof state === "string"))) {
      return { error: `${ALLOWLIST}: entry ${index} states must be a list of state names` };
    }
    entries.push({ check, element, reason, states: states as string[] | undefined });
  }
  return { entries };
}

/** The issue's own detail, one short line. */
function detailOf(issue: unknown): string {
  const rest: Record<string, unknown> = {};
  if (issue !== null && typeof issue === "object") {
    for (const [key, value] of Object.entries(issue)) if (key !== "check" && key !== "el") rest[key] = value;
  }
  const text = JSON.stringify(rest);
  return text === "{}" ? "" : text.length > 140 ? `${text.slice(0, 137)}...` : text;
}

function toFinding(issue: unknown, state: string, mode: string, size: string): Finding {
  return {
    check: stringField(issue, "check") ?? "unknown",
    el: stringField(issue, "el") ?? "",
    state,
    mode,
    size,
    detail: detailOf(issue),
  };
}

/** One line per check and element, with where it was seen. */
export function findingLines(findings: Finding[]): string[] {
  const groups = new Map<string, Finding[]>();
  for (const finding of findings) {
    const key = `${finding.check}\u0000${finding.el}`;
    groups.set(key, [...(groups.get(key) ?? []), finding]);
  }
  const unique = (list: string[]) => [...new Set(list)].join(",");
  return [...groups.values()]
    .sort((a, b) => a[0].check.localeCompare(b[0].check) || a[0].el.localeCompare(b[0].el))
    .map((group) => {
      const first = group[0];
      const modes = unique(group.map((entry) => entry.mode));
      return `${first.check} ${first.el} @ ${unique(group.map((entry) => entry.state))} [${unique(group.map((entry) => entry.size))}${modes ? ` ${modes}` : ""}] ${first.detail}`.trim();
    });
}

export interface UxRun {
  /** The harness could not measure: build, seed, launch, viewport or probe failures. */
  problems: string[];
  findings: Finding[];
  /** Findings an allowlist entry covers. */
  allowed: number;
  staleEntries: string[];
  audits: number;
  states: number;
  focusChecks: number;
  probes: number;
  seconds: number;
}

/** Runs the audit; the gate and a findings dump share it. */
export async function runDesktopUx(ctx: GateContext): Promise<UxRun> {
  const started = Date.now();
  const out: UxRun = { problems: [], findings: [], allowed: 0, staleEntries: [], audits: 0, states: 0, focusChecks: 0, probes: 0, seconds: 0 };
  const done = () => ({ ...out, seconds: Math.round((Date.now() - started) / 1000) });
  const check: Check = (condition, message) => {
    if (!condition) out.problems.push(message);
    return condition;
  };
  const allow = readAllowlist(ctx.root);
  if ("error" in allow) {
    out.problems.push(allow.error);
    return done();
  }

  const temp = mkdtempSync(path.join(tmpdir(), "passquest-desktop-ux-"));
  const provider = startProviderStub();
  provider.respond = () => chatReply(COACH_REPLY);
  const anki = startAnkiStub();
  try {
    const binary = path.join(temp, executable("passquest-desktop"));
    const build = await buildDesktop(ctx, binary);
    if (!build.ok) {
      out.problems.push(...build.failures);
      return done();
    }
    const cli = path.join(temp, executable("passquest"));
    const cliBuild = await runGo(ctx, ["build", "-o", cli, "./cmd/passquest"]);
    if (!check(cliBuild.code === 0, `go build ./cmd/passquest failed\n${tail(`${cliBuild.stdout}${cliBuild.stderr}`)}`)) return done();
    const node = await run(executable("node"), ["--version"]);
    if (!check(node.code === 0, `node is required for the coach state: ${node.stderr || node.stdout}`)) return done();

    const registry = path.join(temp, "registry.json");
    writeStubRegistry(registry, provider.endpoint);
    const key = `sk-ux-${randomBytes(12).toString("hex")}`;
    const keyEnv = { [STUB_CREDENTIAL_ENV]: key };
    const template = path.join(temp, "template");
    const templateData = path.join(template, "data");
    mkdirSync(templateData, { recursive: true });
    const database = path.join(templateData, "passquest.sqlite");
    const call = (args: string[]) => cliJson(cli, database, ctx.root, args, { PASSQUEST_PROVIDER_REGISTRY: registry, ...keyEnv });
    // 40 pages: practice in the prepared state spends pages, and a run needs an unseen pool above its floor.
    const source = await seedReviewedSource(call, check, path.join(template, "source"), 40, 4);
    if (!source) return done();
    for (const args of [["model", "test", STUB_MODEL], ["model", "use", STUB_MODEL]]) {
      const result = await call(args);
      if (!check(result.code === 0, `seed: passquest ${args.join(" ")} exit = ${result.code}: ${tail(result.stderr, 4)}`)) return done();
    }
    // The long question's own database: one reviewed page with one long question tagged with the
    // scoring skill; after its guided walkthrough and one skill attempt the recommendation opens it.
    const longTemplate = path.join(temp, "long-template");
    const longData = path.join(longTemplate, "data");
    mkdirSync(longData, { recursive: true });
    const longCall = (args: string[]) => cliJson(cli, path.join(longData, "passquest.sqlite"), ctx.root, args, {});
    if (!(await seedReviewedSource(longCall, check, path.join(longTemplate, "source"), 1, 1, true))) return done();
    for (const args of [["tactic", "practice", SKILL, "--stage", "guided", "--answer", "A"], ["skill", "practice", SKILL, "--answer", "A"]]) {
      const result = await longCall(args);
      if (!check(result.code === 0, `seed: passquest ${args.join(" ")} exit = ${result.code}: ${tail(result.stderr, 4)}`)) return done();
    }

    // The many-item source's own database: a page of MANY_ITEMS long questions and a page of one under a
    // long title, and a note drafted from one attempt.
    const manyTemplate = path.join(temp, "many-template");
    const manyData = path.join(manyTemplate, "data");
    mkdirSync(manyData, { recursive: true });
    const manyCall = (args: string[]) => cliJson(cli, path.join(manyData, "passquest.sqlite"), ctx.root, args, {});
    const manySource = await seedReviewedSource(manyCall, check, path.join(manyTemplate, "source"), 2, MANY_ITEMS, true, LONG_TITLE);
    if (!manySource) return done();
    // The note's attempt is on page 2: a practised passage leaves the pool, and page 1 keeps all its items.
    const practiced = await manyCall(["practice", "--source", manySource, "--page", "2", "--question", "p02-q1", "--answer", "A"]);
    const attempt = numberField(field(practiced.doc, "attempt"), "id");
    if (!check(practiced.code === 0 && attempt !== undefined, `seed: passquest practice p02-q1 exit = ${practiced.code}: ${tail(practiced.stderr, 4)}`)) return done();
    const proposed = await manyCall(["note", "propose", "--attempt", String(attempt)]);
    if (!check(proposed.code === 0, `seed: passquest note propose exit = ${proposed.code}: ${tail(proposed.stderr, 4)}`)) return done();

    let launches = 0;
    const launch = async (label: string, dataDir: string, size: readonly [number, number], body: string) => {
      const dir = path.join(temp, label);
      mkdirSync(dir, { recursive: true });
      launches++;
      const launched = await launchPage({
        binary,
        cwd: ctx.root,
        dir,
        label,
        header: `desktop-ux: ${label}`,
        where: `desktop-ux ${label}`,
        script: pageScript(`${viewportGuard(size[0], size[1])}\n${body}`),
        env: {
          PASSQUEST_DESKTOP_DATA: dataDir,
          PASSQUEST_DESKTOP_SECRET_SERVICE: `passquest-ux-${process.pid}-${Date.now()}-${launches}`,
          PASSQUEST_DESKTOP_CONTENT_SIZE: `${size[0]}x${size[1]}`,
          PASSQUEST_PROVIDER_REGISTRY: registry,
          PASSQUEST_ANKI_ENDPOINT: anki.endpoint,
          ...keyEnv,
        },
        check,
      });
      if (!launched) return null;
      const precondition = field(launched.value, "precondition");
      if (precondition !== undefined) {
        check(
          false,
          `viewport precondition: ${label} asked for a ${size[0]}x${size[1]} page area and got ${String(numberField(precondition, "width"))}x${String(numberField(precondition, "height"))}; nothing is measured at another size`,
        );
        return null;
      }
      return launched.value;
    };
    const copyTemplate = (label: string, from = templateData) => {
      const dataDir = path.join(temp, label, "data");
      cpSync(from, dataDir, { recursive: true });
      return dataDir;
    };

    if ((await launch("prepare", templateData, UX_SIZES[0], PREPARE)) === null) return done();

    // Planted defects first: a library that misses one cannot vouch for the app.
    const motion = await launch("motion", copyTemplate("motion"), UX_SIZES[0], MOTION);
    if (motion === null) return done();
    const probes = Object.entries((field(motion, "probes") ?? {}) as Record<string, unknown>);
    out.probes = probes.length;
    const missed = probes.filter(([, fired]) => fired !== true).map(([name]) => name);
    if (!check(probes.length > 0 && missed.length === 0, `probes: ${missed.length}/${probes.length} planted defect(s) not reported: ${missed.join(", ")}`)) {
      return done();
    }
    const size0 = `${UX_SIZES[0][0]}x${UX_SIZES[0][1]}`;
    const room = field(motion, "room");
    if (arrayField(room, "transitions").length === 0) {
      out.findings.push({
        check: "room-transition-missing",
        el: `body[data-room] ${String(stringField(room, "before"))} -> ${String(stringField(room, "after"))}`,
        state: "practice-independent -> assess",
        mode: "light-zh",
        size: size0,
        detail: "",
      });
    }
    for (const issue of arrayField(motion, "motion")) {
      out.findings.push(toFinding(issue, `reduced:${String(stringField(issue, "at"))}`, "light-zh", size0));
    }

    const states = new Set<string>();
    const audited: Audited[] = [];
    for (const size of UX_SIZES) {
      const sizeName = `${size[0]}x${size[1]}`;
      for (const [part, body, from] of [["one", PART_ONE, templateData], ["two", PART_TWO, templateData], ["long", PART_LONG, longData], ["many", PART_MANY, manyData]] as const) {
        const label = `audit-${sizeName}-${part}`;
        const value = await launch(label, copyTemplate(label, from), size, `${DRIVE}\n${body}`);
        if (value === null) return done();
        for (const result of arrayField(value, "results")) {
          const state = stringField(result, "state") ?? "?";
          const mode = stringField(result, "mode") ?? "?";
          states.add(state);
          audited.push({ size: sizeName, state, mode });
          out.audits++;
          check(stringField(result, "shown") === mode, `${label}: ${state} was audited as ${String(stringField(result, "shown"))}, not ${mode}`);
          for (const issue of arrayField(result, "issues")) out.findings.push(toFinding(issue, state, mode, sizeName));
        }
        for (const issue of arrayField(value, "focus")) {
          out.findings.push(toFinding(issue, "navigation", "light-zh", sizeName));
        }
        out.focusChecks += numberField(value, "checked") ?? 0;
      }
    }
    out.states = states.size;
    out.problems.push(...coverageProblems(audited, UX_SIZES.map((size) => `${size[0]}x${size[1]}`)));

    const used = new Set<number>();
    out.findings = out.findings.filter((finding) => {
      const index = allow.entries.findIndex(
        (entry) => entry.check === finding.check && entry.element === finding.el && (!entry.states || entry.states.includes(finding.state)),
      );
      if (index < 0) return true;
      used.add(index);
      out.allowed++;
      return false;
    });
    out.staleEntries = allow.entries.filter((_, index) => !used.has(index)).map((entry) => `${entry.check} ${entry.element}`);
    return done();
  } finally {
    provider.stop();
    anki.stop();
    await removeTree(temp);
  }
}

export const desktopUx: Gate = {
  id: "desktop-ux",
  title: "desktop ux",
  async run(ctx) {
    const surface = desktopSurface(ctx);
    if (surface) return surface;
    const result = await runDesktopUx(ctx);
    if (result.problems.length > 0) return fail(result.problems, `${result.seconds}s`);
    const counts = new Map<string, number>();
    for (const finding of result.findings) counts.set(finding.check, (counts.get(finding.check) ?? 0) + 1);
    const lines = findingLines(result.findings);
    const failures = [
      ...(result.findings.length > 0
        ? [`${result.findings.length} finding(s), ${lines.length} distinct: ${[...counts].map(([name, count]) => `${name} ${count}`).join(", ")}`]
        : []),
      ...lines,
      ...result.staleEntries.map((entry) => `${ALLOWLIST}: entry matches nothing (remove it): ${entry}`),
    ];
    const detail = `${result.audits} audits over ${result.states} states × ${MODES.length} modes × ${UX_SIZES.length} sizes, ${result.probes} probes, ${result.focusChecks} focus checks, ${result.allowed} allowed, ${result.seconds}s`;
    if (failures.length > 0) return fail(failures, detail);
    return pass(detail);
  },
};
