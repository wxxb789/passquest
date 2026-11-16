import { Database } from "bun:sqlite";
import { createHash, randomBytes } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { startAnkiStub } from "../lib/anki-stub.ts";
import { buildDesktop, launchDesktop, removeTree } from "../lib/desktop.ts";
import {
  chatReply,
  cleanEnv,
  cliJson,
  desktopSurface,
  launchPage,
  pageScript,
  parse,
  SKILL,
  startProviderStub,
  STUB_CREDENTIAL_ENV,
  STUB_MODEL,
  STUB_PROVIDER,
  tail,
  writeStubRegistry,
} from "../lib/desktop-page.ts";
import type { Check, Launched, ProviderStub } from "../lib/desktop-page.ts";
import { runGo } from "../lib/go.ts";
import { arrayField, field, numberField, stringField } from "../lib/json.ts";
import { executable, run } from "../lib/process.ts";
import type { RunResult } from "../lib/process.ts";
import { resolve } from "../lib/repo.ts";
import { seedReviewedSource } from "../lib/reviewed-source.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";

/**
 * Drives the app contract described in harness/lib/desktop-page.ts. Each
 * scenario gets a fresh data directory; a scenario about persistence launches
 * the window more than once on its own directory. A second process
 * (`passquest ... --db`) cross-checks what the window recorded.
 */
const CLIENT_PATH = "desktop/web/src/api/mygo.ts";
const GENERATE_TIMEOUT_MS = 30_000;
const MIN_TODAY_TEXT = 200;
/** Nothing listens on port 1: the "Anki is not running" endpoint. */
const UNREACHABLE_ENDPOINT = "http://127.0.0.1:1";
const IMPORT_FIXTURE = "internal/cli/testdata/import";
const IMPORTED_SENTENCE = "The Westbridge Community Library closes at 4 p.m. on Saturdays.";
/** Markup a model could return; it must reach the learner as text, never as elements. */
const HOSTILE_MARKUP = '<b>x</b><img src=x onerror="document.title=\'pwned\'">';
/** The AI refinement the stub returns; its back carries markup that must stay text. */
const REFINED_FRONT = "smoke-refined NOT EXCEPT";
const REFINED_BACK = `Refined back: read the options against the passage. ${HOSTILE_MARKUP}`;
const NAVIGATOR_STATES = ["pending", "presented", "answered", "timed_out", "unavailable"];
const NAVIGATOR_CLASSES = ["cell", "cur", "ans", "to"];
const NAVIGATOR_ATTRIBUTES = ["class", "role", "data-state", "data-position", "aria-label", "title", "aria-current"];

interface Shared {
  ctx: GateContext;
  binary: string;
  /** The CLI, built once the window boots. */
  cli: string;
  registry: string;
  provider: ProviderStub;
}

interface ScenarioRun {
  shared: Shared;
  id: string;
  dir: string;
  dataDir: string;
  database: string;
  /** Credential-store service of this scenario's launches. */
  service: string;
  check: Check;
}

interface Scenario {
  id: string;
  run: (s: ScenarioRun) => Promise<void>;
}

interface LaunchSpec {
  label: string;
  /** Script body; `pageScript` adds the shared helpers. */
  script: string;
  env?: Record<string, string>;
  /** Probes listening TCP sockets of the app and its children while it runs. */
  probeListeners?: boolean;
  /** Polled while the app runs; true kills the process tree with the watchdog's kill. */
  killWhen?: () => Promise<boolean>;
}

const BOOT_SCRIPT = `
const stage = () => document.querySelector("#stage");
const ready = () => window.__pq === undefined || window.__pq.ready === true;
const content = () => (stage()?.children.length ?? 0) > 0 && document.body.innerText.length > ${MIN_TODAY_TEXT};
const deadline = Date.now() + 15000;
while (!(ready() && content()) && Date.now() < deadline) await new Promise((done) => setTimeout(done, 100));
const node = stage();
const errors = document.querySelectorAll("#errbar, .error-bar");
return {
  textLength: document.body.innerText.length,
  stageChildren: node ? node.children.length : -1,
  stageTextLength: node ? node.innerText.trim().length : 0,
  stageText: node ? node.innerText.replace(/\\s+/g, " ").trim().slice(0, 240) : "",
  errorBars: errors.length,
  errorText: Array.from(errors, (element) => element.textContent ?? "").join(" | ").slice(0, 300),
  ready: window.__pq === undefined ? null : window.__pq.ready === true,
};
`;

/** Milliseconds since the epoch; the window and the CLI print different sub-millisecond digits. */
function instant(text: string | undefined): number {
  return text === undefined ? Number.NaN : Date.parse(text.replace(/(\.\d{3})\d+/, "$1"));
}

/** Launches the window once with a script and returns the script's value, or null after a failed check. */
function launch(s: ScenarioRun, spec: LaunchSpec): Promise<Launched | null> {
  return launchPage({
    binary: s.shared.binary,
    // The repository root is the working directory, so the app finds
    // agent/*.mjs and runs them under Node the way the CLI smokes do.
    cwd: s.shared.ctx.root,
    dir: s.dir,
    label: spec.label,
    header: `desktop-smoke: ${s.id}/${spec.label}`,
    where: spec.label === s.id ? s.id : `${s.id} (${spec.label})`,
    script: spec.script,
    env: {
      PASSQUEST_DESKTOP_DATA: s.dataDir,
      PASSQUEST_DESKTOP_SECRET_SERVICE: s.service,
      PASSQUEST_PROVIDER_REGISTRY: s.shared.registry,
      PASSQUEST_ANKI_ENDPOINT: UNREACHABLE_ENDPOINT,
      ...spec.env,
    },
    check: s.check,
    probeListeners: spec.probeListeners,
    killWhen: spec.killWhen,
  });
}

/** `passquest <args> --db <scenario db> --json` from the repository root. */
function cli(s: ScenarioRun, args: string[], env: Record<string, string> = {}): Promise<RunResult & { doc: unknown }> {
  return cliJson(s.shared.cli, s.database, s.shared.ctx.root, args, { PASSQUEST_PROVIDER_REGISTRY: s.shared.registry, ...env });
}

/** Runs CLI setup steps in order; false after the first one that fails. */
async function setup(s: ScenarioRun, steps: string[][], env: Record<string, string> = {}): Promise<boolean> {
  for (const args of steps) {
    const result = await cli(s, args, env);
    if (!s.check(result.code === 0, `${s.id}: setup \`passquest ${args.join(" ")}\` exit = ${result.code}: ${tail(result.stderr, 4)}`)) {
      return false;
    }
  }
  return true;
}

function attemptsOf(report: { doc: unknown }): unknown[] {
  return arrayField(report.doc, "attempts");
}

/** The latest displayed, unanswered practice item as the store keeps it (what a relaunch resumes). */
function openPresentation(database: string): { token: string; presentedAt: string } | null {
  if (!existsSync(database)) return null;
  let db: Database | undefined;
  try {
    db = new Database(database, { readonly: true });
    const row = db
      .query("SELECT token, presented_at FROM practice_presentations WHERE closed_at = '' ORDER BY rowid DESC LIMIT 1")
      .get();
    const token = stringField(row, "token");
    const presentedAt = stringField(row, "presented_at");
    return token && presentedAt ? { token, presentedAt } : null;
  } catch {
    // Not created yet, or busy with a write: look again on the next poll.
    return null;
  } finally {
    db?.close();
  }
}

/** Every regular file under `dir`. */
function filesUnder(dir: string): string[] {
  if (!existsSync(dir)) return [];
  return readdirSync(dir).flatMap((name) => {
    const full = path.join(dir, name);
    return statSync(full).isDirectory() ? filesUnder(full) : [full];
  });
}

function containsSecret(bytes: Buffer, secret: string): boolean {
  return bytes.includes(Buffer.from(secret, "utf8")) || bytes.includes(Buffer.from(secret, "utf16le"));
}

/** Removes a credential the window stored, through the OS tool; best effort, never throws. */
async function forgetCredential(service: string, account: string): Promise<void> {
  if (process.platform === "win32") {
    await run(executable("cmdkey"), [`/delete:${service}/${account}`]);
  } else if (process.platform === "darwin") {
    await run("security", ["delete-generic-password", "-s", service, "-a", account]);
  } else {
    await run("secret-tool", ["clear", "service", service, "account", account]);
  }
}

function navigatorProblems(where: string, navigators: unknown[]): string[] {
  const problems: string[] = [];
  for (const navigator of navigators) {
    const html = stringField(navigator, "html") ?? "";
    if (/correct|incorrect|wrong/i.test(html)) problems.push(`${where}: the navigator names correctness: ${html.slice(0, 160)}`);
    for (const cell of arrayField(navigator, "cells")) {
      const state = stringField(cell, "state") ?? "";
      if (!NAVIGATOR_STATES.includes(state)) problems.push(`${where}: navigator cell state ${JSON.stringify(state)}`);
      for (const name of arrayField(cell, "classes")) {
        if (!NAVIGATOR_CLASSES.includes(String(name))) problems.push(`${where}: navigator cell class ${String(name)}`);
      }
      for (const name of arrayField(cell, "attributes")) {
        if (!NAVIGATOR_ATTRIBUTES.includes(String(name))) problems.push(`${where}: navigator cell attribute ${String(name)}`);
      }
    }
  }
  return problems;
}

/** Compares the window's answers with the attempts a second process reads. */
function checkAttempts(s: ScenarioRun, answers: unknown[], attempts: unknown[]): void {
  for (const answered of answers) {
    const item = field(answered, "item");
    const shown = stringField(field(answered, "condition"), "condition") ?? "";
    const feedback = field(answered, "feedback");
    const id = Number(stringField(feedback, "attempt"));
    const row = attempts.find((attempt) => numberField(attempt, "id") === id);
    if (!s.check(row !== undefined, `${s.id}: report --json has no attempt ${id} for the answer the window recorded`)) continue;
    s.check(
      stringField(row, "question_id") === stringField(item, "question"),
      `${s.id}: attempt ${id} is question ${String(stringField(row, "question_id"))}, the window showed ${String(stringField(item, "question"))}`,
    );
    const elapsed = numberField(row, "elapsed_ms") ?? 0;
    s.check(elapsed > 0, `${s.id}: attempt ${id} elapsed_ms = ${elapsed}`);
    s.check(
      String(elapsed) === stringField(feedback, "elapsedMs"),
      `${s.id}: attempt ${id} elapsed_ms = ${elapsed}, the window showed ${String(stringField(feedback, "elapsedMs"))}`,
    );
    const assistance = stringField(row, "assistance");
    s.check(
      assistance === (shown === "assisted" ? "assisted" : "independent"),
      `${s.id}: the window showed condition ${shown} before answering but attempt ${id} is ${String(assistance)}`,
    );
    s.check(
      stringField(row, "selected_label") === stringField(answered, "chosen"),
      `${s.id}: attempt ${id} recorded ${String(stringField(row, "selected_label"))}, the keyboard chose ${String(stringField(answered, "chosen"))}`,
    );
    const started = stringField(row, "started_at");
    s.check(
      instant(started) === instant(stringField(item, "presentedAt")),
      `${s.id}: attempt ${id} started_at ${String(started)} is not the presentation time the window showed (${String(stringField(item, "presentedAt"))})`,
    );
  }
}

const clientFresh: Scenario = {
  id: "client-fresh",
  async run(s) {
    const generated = path.join(s.dir, "mygo.generated.ts");
    // From the repository root the generator finds the Go sources and keeps their doc comments.
    const result = await launchDesktop({
      binary: s.shared.binary,
      env: { ...cleanEnv(), MYGO_GENERATE: generated },
      timeoutMs: GENERATE_TIMEOUT_MS,
      cwd: s.shared.ctx.root,
    });
    const how = result.error ?? `exit ${result.code}${result.timedOut ? ", timed out" : ""}`;
    if (!s.check(existsSync(generated), `client-fresh: the binary wrote no client with MYGO_GENERATE (${how})`)) return;
    const committedPath = resolve(s.shared.ctx.root, CLIENT_PATH);
    if (!s.check(existsSync(committedPath), `client-fresh: ${CLIENT_PATH} is missing`)) return;
    const fresh = readFileSync(generated, "utf8").replace(/\r\n/g, "\n").split("\n");
    const committed = readFileSync(committedPath, "utf8").replace(/\r\n/g, "\n").split("\n");
    let line = 0;
    while (line < Math.max(fresh.length, committed.length) && fresh[line] === committed[line]) line++;
    s.check(
      line === fresh.length && line === committed.length,
      `client-fresh: ${CLIENT_PATH} is stale: the binary generates a different client from line ${line + 1}; regenerate it with MYGO_GENERATE`,
    );
  },
};

const boot: Scenario = {
  id: "boot",
  async run(s) {
    const launched = await launch(s, { label: "boot", script: BOOT_SCRIPT });
    if (!launched) return;
    const value = launched.value;
    const textLength = numberField(value, "textLength") ?? 0;
    const stageChildren = numberField(value, "stageChildren") ?? -1;
    s.check(textLength > MIN_TODAY_TEXT, `boot: blank Today, body text length ${textLength} <= ${MIN_TODAY_TEXT}`);
    s.check(stageChildren >= 0, "boot: #stage is missing");
    s.check(
      stageChildren > 0 && (numberField(value, "stageTextLength") ?? 0) > 0,
      `boot: #stage is empty (${stageChildren} children)`,
    );
    const errorBars = numberField(value, "errorBars") ?? -1;
    s.check(errorBars === 0, `boot: ${errorBars} error bar(s): ${String(field(value, "errorText") ?? "")}`);
    const ready = field(value, "ready");
    s.check(
      ready === null || ready === true,
      `boot: window.__pq.ready = ${JSON.stringify(ready)}; the stage shows: ${String(field(value, "stageText") ?? "")}`,
    );
  },
};

const lifecycle: Scenario = {
  id: "lifecycle",
  async run(s) {
    const launched = await launch(s, {
      label: "lifecycle",
      script: "await new Promise((done) => setTimeout(done, 2000));\nreturn { waited: true };",
      probeListeners: true,
    });
    if (launched) s.check(field(launched.value, "waited") === true, `lifecycle: script returned ${JSON.stringify(launched.value)}`);
  },
};

const practice: Scenario = {
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

const hint: Scenario = {
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

const feedback: Scenario = {
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

const coach: Scenario = {
  id: "coach",
  async run(s) {
    const { provider } = s.shared;
    const key = `sk-smoke-${randomBytes(12).toString("hex")}`;
    const keyEnv = { [STUB_CREDENTIAL_ENV]: key };
    const imported = await cli(s, ["import", "--path", resolve(s.shared.ctx.root, IMPORT_FIXTURE)]);
    const sourceID = stringField(field(imported.doc, "source"), "id");
    if (!s.check(imported.code === 0 && sourceID !== undefined, `coach: setup import exit = ${imported.code}: ${tail(imported.stderr, 4)}`)) return;
    // A guided and an independent first-party attempt make the recommendation
    // open the imported item; the model is tested and chosen through the CLI.
    const steps = [
      ["question", "add", sourceID!, "1", "--file", resolve(s.shared.ctx.root, `${IMPORT_FIXTURE}/question-q2-tagged.json`)],
      ["page", "review", sourceID!, "1", "--reviewer", "smoke"],
      ["question", "review", sourceID!, "q2", "--answer", "C", "--reviewer", "smoke"],
      ["tactic", "practice", SKILL, "--stage", "guided", "--answer", "A"],
      ["skill", "practice", SKILL, "--answer", "A"],
      ["model", "test", STUB_MODEL],
      ["model", "use", STUB_MODEL],
    ];
    if (!(await setup(s, steps, keyEnv))) return;

    provider.received.length = 0;
    let reportAtFailure: string | null = null;
    let reportAfterFailure: string | null = null;
    let retryArrivedAt = 0;
    // A reply, then a 401, then (after Retry and a second send in the same mode) a reply again.
    provider.respond = async () => {
      if (provider.received.length === 1) return chatReply(`Check the closing time. ${HOSTILE_MARKUP}`);
      if (provider.received.length === 2) {
        reportAtFailure ??= (await cli(s, ["report"])).stdout;
        return Response.json({ error: { message: "invalid token" } }, { status: 401 });
      }
      retryArrivedAt ||= Date.now();
      reportAfterFailure ??= (await cli(s, ["report"])).stdout;
      return chatReply("Retried: read the closing time again.");
    };
    const launched = await launch(s, {
      label: "coach",
      env: keyEnv,
      probeListeners: true,
      script: pageScript(`
await today();
press("Enter");
const answered = await answer("1");
const strip = () => q('[data-testid="call-strip"]');
const preview = () => {
  const element = strip();
  return { sha: element.dataset.sha, bytes: element.dataset.bytes, consent: element.querySelector("[data-consent]")?.dataset.consent ?? null, payload: element.querySelector('[data-testid="payload"]')?.textContent ?? "" };
};
await until("the coach call strip", () => strip() && q('[data-act="coach-send"]'), 20000);
const withheld = preview();
await click('[data-act="coach-send"]');
const reply = await until("the coach reply", () => q('[data-testid="coach-reply"]') || q('[data-testid="coach-failure"]'), 30000);
const shown = { failed: reply.dataset.testid !== "coach-reply", text: reply.textContent, elements: reply.querySelectorAll("p b, p img, p script").length, injected: document.querySelectorAll('img[src="x"]').length, title: document.title };
await click('[data-act="consent-grant"]');
await until("the grant to be recorded", () => !q('[data-act="consent-grant"]'), 20000);
// The answered mode keeps its reply; the next send uses a mode without one.
await click('#coach [data-act="mode"][aria-pressed="false"]');
await until("a preview with consent", () => strip() && strip().dataset.sha !== withheld.sha && q('[data-act="coach-send"]') && q('[data-consent="granted"]'), 20000);
const granted = preview();
await click('[data-act="coach-send"]');
const failure = await until("the typed failure", () => q('[data-testid="coach-failure"]'), 30000);
const failed = { code: failure.dataset.code, text: failure.textContent };
// Retry only clears the failure and shows the preview again; the learner sends from it.
await click('[data-testid="coach-failure"] [data-act="coach-retry"]');
await until("the preview after Retry", () => !q('[data-testid="coach-failure"]') && strip() && q('[data-act="coach-send"]'), 20000);
const retried = preview();
await sleep(300);
const retrySentAt = Date.now();
await click('[data-act="coach-send"]');
const after = await until("the retried reply", () => q('[data-testid="coach-reply"]') || q('[data-testid="coach-failure"]'), 30000);
// The history view lists this attempt's calls, oldest first, each reply as text.
await click('[data-act="coach-history"]');
await until("the coach history", () => q('[data-testid="coach-history"]'), 20000);
const history = {
  calls: all('[data-testid="coach-call"]').map((c) => ({ call: c.dataset.call, state: c.dataset.state, mode: c.dataset.mode, text: c.textContent })),
  elements: document.querySelectorAll('[data-testid="coach-history"] p b, [data-testid="coach-history"] p img, [data-testid="coach-history"] script').length,
  title: document.title,
};
return { answered, withheld, reply: shown, granted, failure: failed, retried, retrySentAt, retry: { failed: after.dataset.testid !== "coach-reply", code: after.dataset.code ?? null, text: after.textContent }, history };
`),
    });
    provider.respond = () => chatReply("OK");
    if (!launched) return;
    const value = launched.value;
    const question = stringField(field(field(value, "answered"), "item"), "question");
    if (!s.check(question === "q2", `coach: the window opened ${String(question)}, not the imported item q2`)) return;
    const withheld = field(value, "withheld");
    const granted = field(value, "granted");
    s.check(stringField(withheld, "consent") === "withheld", `coach: consent before a grant = ${String(stringField(withheld, "consent"))}`);
    s.check(!(stringField(withheld, "payload") ?? "").includes(IMPORTED_SENTENCE), "coach: the preview shows imported text before consent");
    if (!s.check(provider.received.length === 3, `coach: the provider received ${provider.received.length} request(s), want 3`)) return;
    const [first, second, third] = provider.received;
    s.check(first.sha256 === stringField(withheld, "sha"), "coach: the preview sha256 is not the sha256 of the body the provider received");
    s.check(String(Buffer.byteLength(first.raw)) === stringField(withheld, "bytes"), "coach: the preview size is not the size of the body sent");
    s.check(!first.raw.includes(IMPORTED_SENTENCE), "coach: imported text was sent before consent");
    s.check(first.authorization === `Bearer ${key}`, "coach: the provider did not receive the credential");
    s.check(first.path === "/v1/chat/completions", `coach: provider path ${first.path}`);
    const reply = field(value, "reply");
    s.check(field(reply, "failed") === false, "coach: the first send did not render a reply");
    s.check((stringField(reply, "text") ?? "").includes(HOSTILE_MARKUP), "coach: the reply's markup is not shown as literal text");
    s.check(numberField(reply, "elements") === 0 && numberField(reply, "injected") === 0, "coach: the reply's markup became elements");
    s.check(stringField(reply, "title") !== "pwned", "coach: the reply's markup ran script");
    s.check(stringField(granted, "consent") === "granted", `coach: consent after the grant = ${String(stringField(granted, "consent"))}`);
    s.check((stringField(granted, "payload") ?? "").includes(IMPORTED_SENTENCE), "coach: the preview omits imported text after consent");
    s.check(second.sha256 === stringField(granted, "sha"), "coach: the granted preview sha256 is not the body the provider received");
    s.check(second.raw.includes(IMPORTED_SENTENCE), "coach: imported text was not sent after consent");
    const failure = field(value, "failure");
    s.check(stringField(failure, "code") === "auth_failed", `coach: a 401 shows failure code ${String(stringField(failure, "code"))}`);
    s.check(reportAtFailure !== null && reportAfterFailure === reportAtFailure, "coach: the failed call changed report --json");
    const retry = field(value, "retry");
    s.check(
      field(retry, "failed") === false && (stringField(retry, "text") ?? "").includes("Retried"),
      `coach: Retry after the 401 did not lead to a reply (shown ${String(stringField(retry, "code"))})`,
    );
    s.check(
      retryArrivedAt >= (numberField(value, "retrySentAt") ?? Number.POSITIVE_INFINITY),
      "coach: Retry called the provider before the learner sent from the preview",
    );
    s.check(third.sha256 === stringField(field(value, "retried"), "sha"), "coach: the preview shown after Retry is not the body the provider received");
    // History: the window lists exactly the attempt's recorded calls, in order, replies as text.
    const attemptID = stringField(field(field(value, "answered"), "feedback"), "attempt");
    const recorded = attemptID === undefined ? null : await cli(s, ["tutor", "history", "--attempt", attemptID]);
    const recordedCalls = recorded ? arrayField(recorded.doc, "calls") : [];
    const history = field(value, "history");
    const shownCalls = arrayField(history, "calls");
    s.check(
      recordedCalls.length === 3 && shownCalls.length === recordedCalls.length,
      `coach: history shows ${shownCalls.length} call(s); tutor history --attempt records ${recordedCalls.length}, want 3`,
    );
    s.check(
      shownCalls.map((c) => stringField(c, "call")).join(",") === recordedCalls.map((c) => String(numberField(c, "id"))).join(","),
      "coach: the history's calls are not the attempt's recorded calls in order",
    );
    s.check(
      shownCalls.map((c) => stringField(c, "state")).join(",") === "advisory,failed,advisory",
      `coach: history states ${shownCalls.map((c) => stringField(c, "state")).join(",")}, want advisory,failed,advisory`,
    );
    s.check(
      shownCalls.map((c) => stringField(c, "mode")).join(",") === recordedCalls.map((c) => stringField(c, "mode")).join(","),
      "coach: the history's modes are not the recorded modes",
    );
    s.check((stringField(shownCalls[0], "text") ?? "").includes(HOSTILE_MARKUP), "coach: the history does not show the earlier advisory reply as literal text");
    s.check((stringField(shownCalls[1], "text") ?? "").includes("auth_failed"), "coach: the history does not show the failed call's code");
    s.check((stringField(shownCalls[2], "text") ?? "").includes("Retried"), "coach: the history does not show the retried reply");
    s.check(numberField(history, "elements") === 0 && stringField(history, "title") !== "pwned", "coach: the history's reply markup became elements");
  },
};

const assess: Scenario = {
  id: "assess",
  async run(s) {
    const source = await seedReviewedSource((args) => cli(s, args), s.check, path.join(s.dir, "source"), 20, 4);
    if (!source) return;
    const helpers = `
const navigator = () => {
  const nav = q('[data-testid="navigator"]');
  return nav ? { html: nav.innerHTML, cells: all('[data-testid="navigator"] .cell[data-state]').map((cell) => ({ state: cell.dataset.state, classes: Array.from(cell.classList), attributes: cell.getAttributeNames() })) } : null;
};
const position = () => q('[data-testid="assess-item"]')?.dataset.position;
const answerItem = async (key) => {
  const before = position();
  await sleep(150);
  press(key);
  await until("assessment option " + key + " to be selected", () => q('.opt[aria-checked="true"]'));
  press("Enter");
  await until("the next assessment item or the report", () => position() !== before || q('[data-testid="report"]'));
};
`;
    const started = await launch(s, {
      label: "start",
      script: pageScript(`${helpers}
location.hash = "#/assess";
await until("the start button", () => q('[data-act="run-ask"]'));
await click('[data-act="run-ask"]');
await until("the run form", () => q('#runform input[name="size"]'));
q('#runform input[name="size"]').value = "3";
q('#runform input[name="minutes"]').value = "30";
await click('[data-act="run-start"]');
await until("the first assessment item", () => q('[data-testid="assess-item"]'));
const first = data('[data-testid="assess-item"]');
const navigators = [navigator()];
await answerItem("2");
await until("the second item", () => q('[data-testid="assess-item"]'));
navigators.push(navigator());
const left = data('[data-testid="assess-item"]');
await click('[data-act="leave"]');
await until("the leave dialog", () => q('[data-act="leave-yes"]'));
await click('[data-act="leave-yes"]');
await until("Today's way back to the run", () => q('[data-testid="back-to-run"]'));
return { first, left, navigators };
`),
    });
    if (!started) return;
    const resumed = await launch(s, {
      label: "resume",
      script: pageScript(`${helpers}
// The relaunched window offers the active run: Today's way back, or the resume card.
await until("the active run", () => q('[data-testid="back-to-run"]') || q('[data-testid="resume-run"]'));
if (!q('[data-testid="resume-run"]')) await click('[data-testid="back-to-run"]');
await until("the resume card", () => q('[data-testid="resume-run"]'));
await click('[data-testid="resume-run"]');
await until("the resumed item", () => q('[data-testid="assess-item"]'));
const resumed = data('[data-testid="assess-item"]');
const navigators = [];
let answered = 0;
while (q('[data-testid="assess-item"]') && answered < 10) {
  navigators.push(navigator());
  await answerItem(String(1 + (answered % 2)));
  answered++;
}
await until("the run report", () => q('[data-testid="report"]'));
await sleep(200);
return { resumed, answered, navigators, report: data('[data-testid="report"]'), threshold: data('#pass-threshold') };
`),
    });
    if (!resumed) return;
    const first = field(started.value, "first");
    const left = field(started.value, "left");
    const back = field(resumed.value, "resumed");
    const runID = stringField(first, "run");
    s.check(runID !== undefined && stringField(left, "position") !== stringField(first, "position"), "assess: answering did not advance the run");
    s.check(stringField(back, "run") === runID, `assess: resumed run ${String(stringField(back, "run"))}, started ${String(runID)}`);
    for (const name of ["position", "presentedAt"]) {
      s.check(stringField(back, name) === stringField(left, name), `assess: the resumed item's ${name} is not the item left`);
    }
    s.check(stringField(field(resumed.value, "report"), "run") === runID, "assess: the window's report is not the run");
    const threshold = stringField(field(resumed.value, "threshold"), "value");
    s.check(threshold === "unknown", `assess: the window's pass threshold = ${String(threshold)}`);
    const navigators = [...arrayField(started.value, "navigators"), ...arrayField(resumed.value, "navigators")];
    s.check(navigators.length > 2 && navigators.every((navigator) => arrayField(navigator, "cells").length > 0), "assess: an item showed no navigator");
    const problems = navigatorProblems("assess", navigators);
    s.check(problems.length === 0, problems.slice(0, 3).join("; "));
    const report = await cli(s, ["assess", "report", String(runID)]);
    if (!s.check(report.code === 0, `assess: \`passquest assess report ${String(runID)}\` exit = ${report.code}: ${tail(report.stderr, 4)}`)) return;
    s.check(arrayField(report.doc, "unknowns").includes("pass threshold: unknown"), "assess: the CLI report does not state the unknown pass threshold");
    const runDoc = field(report.doc, "run");
    s.check(numberField(runDoc, "resume_count") === 1, `assess: CLI resume_count = ${String(numberField(runDoc, "resume_count"))}`);
    const afterResume = numberField(resumed.value, "answered") ?? 0;
    const counts = field(report.doc, "counts");
    s.check(numberField(counts, "total") === 1 + afterResume, `assess: CLI total ${String(numberField(counts, "total"))}, the window answered ${1 + afterResume}`);
    s.check(
      numberField(counts, "answered_after_resume") === afterResume,
      `assess: CLI answered_after_resume ${String(numberField(counts, "answered_after_resume"))}, the window answered ${afterResume} after resuming`,
    );
  },
};

const anki: Scenario = {
  id: "anki",
  async run(s) {
    const { provider } = s.shared;
    const key = `sk-smoke-${randomBytes(12).toString("hex")}`;
    const keyEnv = { [STUB_CREDENTIAL_ENV]: key };
    // The refinement goes to the loopback provider stub through the real Node tutor host.
    if (!(await setup(s, [["model", "test", STUB_MODEL], ["model", "use", STUB_MODEL]], keyEnv))) return;
    // `note model` reads no database: the model is the package's own.
    const model = await run(s.shared.cli, ["note", "model", "--json"], { cwd: s.shared.ctx.root });
    const modelCSS = stringField(parse(model.stdout), "css");
    if (!s.check(model.code === 0 && modelCSS !== undefined, `anki: note model exit = ${model.code}`)) return;
    const modelCSSSHA = createHash("sha256").update(modelCSS!).digest("hex");
    const stub = startAnkiStub();
    // After the first push has verified its card, Anki stops listening.
    stub.afterAction = (action) => {
      if (action === "cardsInfo" && stub.actions.some((entry) => entry.action === "addNote")) setTimeout(() => stub.stop(), 100);
    };
    try {
      // A proposed draft that is neither accepted nor skipped survives a restart.
      const pending = await launch(s, {
        label: "propose",
        env: { PASSQUEST_ANKI_ENDPOINT: stub.endpoint },
        script: pageScript(`
await today();
press("Enter");
await answer("1");
press("k");
await until("the card preview", () => q('[data-testid="card-preview"]'), 20000);
return { draft: data('[data-testid="anki"]').draft, preview: data('[data-testid="card-preview"]') };
`),
      });
      if (!pending) return;
      provider.received.length = 0;
      // The first refinement is not a JSON object (a typed failure); the one sent after Retry is.
      provider.respond = () =>
        provider.received.length === 1
          ? chatReply("I would rather not answer in JSON.")
          : chatReply(JSON.stringify({ front: REFINED_FRONT, back: REFINED_BACK }));
      const launched = await launch(s, {
        label: "anki",
        env: { PASSQUEST_ANKI_ENDPOINT: stub.endpoint, ...keyEnv },
        script: pageScript(`
location.hash = "#/anki";
await until("the restored draft", () => q('[data-testid="anki"][data-draft]') && q('[data-testid="card-preview"]'), 20000);
const restored = { draft: data('[data-testid="anki"]').draft, preview: data('[data-testid="card-preview"]') };
// The restored draft is the one edited, accepted and pushed below.
const proposed = restored.preview;
// Every shown frame against the backend's own document for the shown side and theme.
const opened = await window.mygo.call("Study.OpenNoteDraft");
const backend = await window.mygo.call("Study.PreviewNote", opened.id, opened.fields);
const frameState = (f) => {
  const doc = f.contentDocument;
  const root = doc ? doc.documentElement : null;
  return {
    side: f.dataset.side,
    theme: f.dataset.theme,
    same: f.getAttribute("srcdoc") === backend[f.dataset.side][f.dataset.theme],
    body: doc && doc.body ? doc.body.className : null,
    scrollHeight: root ? root.scrollHeight : -1,
    contentHeight: root ? Math.ceil(root.getBoundingClientRect().height) : -1,
    bodyHeight: doc && doc.body ? Math.ceil(doc.body.getBoundingClientRect().height) : -1,
    scrollWidth: root ? root.scrollWidth : -1,
    height: f.clientHeight,
    // The height the window set on the frame, if any: tells a fit that never ran from one that measured wrong.
    styled: f.style.height || "unset",
    // How the frame's box reads by other routes, and whether the page is shown: diagnostics only.
    layout: "offset " + f.offsetHeight + ", rect " + Math.round(f.getBoundingClientRect().height) + ", computed " + getComputedStyle(f).height + ", page " + document.visibilityState,
    width: f.clientWidth,
    scripts: doc ? doc.scripts.length : -1,
  };
};
const settled = async (what, sides, theme) => {
  await until(what, () => {
    const frames = all('[data-testid="card-frame"]');
    return frames.length === sides.length && frames.every((f, i) => f.dataset.side === sides[i] && f.dataset.theme === theme && f.contentDocument && f.contentDocument.readyState === "complete" && f.contentDocument.body && f.clientHeight > 40);
  }, 20000);
  await sleep(700);
  return all('[data-testid="card-frame"]').map(frameState);
};
await click('[data-act="ctheme"][data-v="light"]');
const frames = [];
frames.push(...(await settled("both light sides", ["question", "answer"], "light")));
await click('[data-act="ctheme"][data-v="dark"]');
frames.push(...(await settled("both night sides", ["question", "answer"], "night")));
await click('[data-act="side"][data-v="front"]');
frames.push(...(await settled("the night front", ["question"], "night")));
await click('[data-act="cwidth"][data-v="phone"]');
await click('[data-act="side"][data-v="back"]');
const phone = await settled("the phone back", ["answer"], "night");
frames.push(...phone);
await click('[data-act="cwidth"][data-v="desk"]');
await click('[data-act="side"][data-v="both"]');
await click('[data-act="ctheme"][data-v="light"]');
await settled("both light sides again", ["question", "answer"], "light");
const card = { frames, phone, cssSha: data('[data-testid="card-preview"]').cssSha, backendCssSha: backend.css_sha256, sha: data('[data-testid="card-preview"]').sha, backendSha: backend.fields_sha256, document: backend.question.light };
press("e");
await until("the front field", () => q('[data-field="front"]'));
// Refine: preview → a typed failure → Retry (preview only) → send → an advisory suggestion.
await click('[data-act="refine"]');
const strip = () => q('[data-testid="refine-strip"]');
await until("the refinement preview", () => strip() && q('[data-act="refine-send"]:not([disabled])'), 20000);
const refinePreview = data('[data-testid="refine-strip"]');
await click('[data-act="refine-send"]');
const refineFailure = await until("the refinement failure", () => q('[data-testid="refine-failure"]'), 30000);
const refineFailed = { code: refineFailure.dataset.code, text: refineFailure.textContent };
await click('[data-testid="refine-failure"] [data-act="refine-retry"]');
await until("the preview after Retry", () => !q('[data-testid="refine-failure"]') && strip() && q('[data-act="refine-send"]:not([disabled])'), 20000);
const refineRetried = data('[data-testid="refine-strip"]');
await click('[data-act="refine-send"]');
const sug = await until("the suggestion", () => q('[data-testid="refine-suggestion"]'), 30000);
const suggestion = {
  front: q('[data-testid="suggested-front"]').textContent,
  back: q('[data-testid="suggested-back"]').textContent,
  elements: sug.querySelectorAll("b, img, script").length,
  injected: document.querySelectorAll('img[src="x"]').length,
  title: document.title,
  fieldsBefore: { front: q('[data-field="front"]').value, back: q('[data-field="back"]').value },
};
await click('[data-act="refine-use"]');
await until("the suggestion in the fields", () => q('[data-field="front"]').value === suggestion.front && q('[data-field="back"]').value === suggestion.back);
// The suggestion as it stands is the refined card, not a learner edit.
await until("the used suggestion's preview", () => { const preview = q('[data-testid="card-preview"]'); return preview && preview.dataset.sha !== proposed.sha; });
const used = {
  preview: data('[data-testid="card-preview"]'),
  draft: data('[data-testid="anki"]').draft,
  notes: (await window.mygo.call("Study.Notes")).notes.length,
  localNotes: all('[data-testid="note"]').length,
  editing: !!q('[data-field="front"]'),
};
const front = q('[data-field="front"]');
front.value = front.value + " smoke-edit";
front.dispatchEvent(new Event("input", { bubbles: true }));
await until("the edited preview", () => { const preview = q('[data-testid="card-preview"]'); return preview && preview.dataset.sha !== used.preview.sha && preview.dataset.edited === "true"; });
const edited = data('[data-testid="card-preview"]');
await click("#acceptbtn2");
await until("the accepted note", () => q('[data-testid="note"]') && q('[data-act="push"]'), 20000);
await sleep(300);
await click('[data-act="push"]');
await until("one pushed note", () => q('[data-testid="sync-counts"]')?.dataset.pushed === "1", 30000);
const pushed = data('[data-testid="sync-counts"]');
await today();
press("Enter");
await answer("1");
press("k");
await until("the second card preview", () => q('[data-testid="card-preview"]') && !q('[data-testid="note"][data-state="local"]'), 20000);
await click("#acceptbtn");
await until("the second accepted note", () => q('[data-testid="note"][data-state="local"]') && q('[data-act="push"]'), 20000);
await sleep(300);
await click('[data-act="push"]');
await until("a queued note", () => q('[data-testid="sync-counts"]')?.dataset.queued === "1", 30000);
return { restored, proposed, edited, pushed, queued: data('[data-testid="sync-counts"]'), notes: all('[data-testid="note"]').map((note) => note.dataset.state), card, refinePreview, refineFailed, refineRetried, suggestion, used };
`),
      });
      if (!launched) return;
      const value = launched.value;
      const proposedDraft = stringField(pending.value, "draft");
      const restored = field(value, "restored");
      s.check(
        proposedDraft !== undefined && stringField(restored, "draft") === proposedDraft,
        `anki: after a restart the window shows draft ${String(stringField(restored, "draft"))}, the open draft was ${String(proposedDraft)}`,
      );
      s.check(
        stringField(field(restored, "preview"), "sha") === stringField(field(pending.value, "preview"), "sha"),
        "anki: the restored draft's card is not the card proposed before the restart",
      );
      s.check(
        stringField(field(value, "edited"), "sha") !== stringField(field(value, "proposed"), "sha") &&
          stringField(field(value, "edited"), "edited") === "true",
        "anki: editing a field did not update the preview",
      );
      const added = stub.actions.filter((entry) => entry.action === "addNote");
      s.check(added.length === 1, `anki: the stub received addNote ${added.length} time(s), want 1`);
      const fronts = [...stub.notes.values()].map((fields) => fields.Front ?? "");
      s.check(fronts.length === 1 && fronts[0].includes("smoke-edit"), "anki: the pushed note does not carry the edited field");
      s.check(stringField(field(value, "pushed"), "pushed") === "1", "anki: the window does not count the pushed note");
      s.check(stringField(field(value, "queued"), "queued") === "1", "anki: an unreachable Anki did not queue the note");
      const states = arrayField(value, "notes").map(String);
      s.check(states.includes("pushed") && states.includes("queued"), `anki: note states ${states.join(", ")}`);

      // The card preview is the backend's documents, byte for byte, with the model stylesheet.
      const card = field(value, "card");
      const frames = arrayField(card, "frames");
      s.check(frames.length === 6, `anki: ${frames.length} card frame(s) observed, want 6`);
      for (const frame of frames) {
        const where = `${String(stringField(frame, "side"))}/${String(stringField(frame, "theme"))}`;
        s.check(field(frame, "same") === true, `anki: the ${where} frame's srcdoc is not the backend document for that side and theme`);
        const night = stringField(frame, "theme") === "night";
        s.check(
          stringField(frame, "body") === (night ? "card card1 nightMode night_mode" : "card card1"),
          `anki: the ${where} frame's body class is ${String(stringField(frame, "body"))}`,
        );
        const height = numberField(frame, "height") ?? 0;
        s.check(
          height > 40 && (numberField(frame, "scrollHeight") ?? Infinity) <= height,
          `anki: the ${where} frame clips or scrolls (content ${String(numberField(frame, "scrollHeight"))} px, frame ${height} px)`,
        );
        s.check(
          Math.abs(height - (numberField(frame, "contentHeight") ?? -100)) <= 1,
          `anki: the ${where} frame is not sized to its card (document ${String(numberField(frame, "contentHeight"))} px, body ${String(numberField(frame, "bodyHeight"))} px, frame ${height} px, set ${String(stringField(frame, "styled"))}; ${String(stringField(frame, "layout"))})`,
        );
        s.check(
          (numberField(frame, "scrollWidth") ?? Infinity) <= (numberField(frame, "width") ?? 0),
          `anki: the ${where} frame scrolls sideways (content ${String(numberField(frame, "scrollWidth"))} px, frame ${String(numberField(frame, "width"))} px)`,
        );
        s.check(numberField(frame, "scripts") === 0, `anki: the ${where} frame contains a script`);
      }
      const phone = arrayField(card, "phone");
      s.check(phone.length === 1 && (numberField(phone[0], "width") ?? Infinity) <= 300, `anki: the phone frame is ${String(numberField(phone[0], "width"))} px wide`);
      s.check(stringField(card, "cssSha") === modelCSSSHA, "anki: the preview's css_sha256 is not the sha256 of `note model --json` css");
      s.check(stringField(card, "backendCssSha") === modelCSSSHA, "anki: PreviewNote's css_sha256 is not the sha256 of `note model --json` css");
      s.check((stringField(card, "document") ?? "").includes(`<style>${modelCSS}</style>`), "anki: the card document does not carry the `note model` stylesheet");
      s.check(
        stringField(card, "sha") !== undefined && stringField(card, "sha") === stringField(card, "backendSha"),
        "anki: the preview's data-sha is not PreviewNote's fields_sha256",
      );

      // Refine: the preview is the body the provider received; failures are typed; the suggestion
      // is text and only fills the editor; accepting records the learner's edit.
      if (s.check(provider.received.length === 2, `anki: the provider received ${provider.received.length} refinement request(s), want 2`)) {
        const [failedBody, refinedBody] = provider.received;
        const refinePreview = field(value, "refinePreview");
        s.check(failedBody.sha256 === stringField(refinePreview, "sha"), "anki: the refinement preview sha256 is not the sha256 of the body the provider received");
        s.check(String(Buffer.byteLength(failedBody.raw)) === stringField(refinePreview, "bytes"), "anki: the refinement preview size is not the size of the body sent");
        s.check(refinedBody.sha256 === stringField(field(value, "refineRetried"), "sha"), "anki: the preview shown after Retry is not the body the provider received");
        s.check(failedBody.authorization === `Bearer ${key}`, "anki: the provider did not receive the credential");
      }
      s.check(
        stringField(field(value, "refineFailed"), "code") === "invalid_response",
        `anki: a non-JSON refinement shows failure code ${String(stringField(field(value, "refineFailed"), "code"))}`,
      );
      const suggestion = field(value, "suggestion");
      s.check(stringField(suggestion, "front") === REFINED_FRONT, `anki: the suggested front is ${String(stringField(suggestion, "front"))}`);
      s.check((stringField(suggestion, "back") ?? "").includes(HOSTILE_MARKUP), "anki: the suggestion's markup is not shown as literal text");
      s.check(
        numberField(suggestion, "elements") === 0 && numberField(suggestion, "injected") === 0 && stringField(suggestion, "title") !== "pwned",
        "anki: the suggestion's markup became elements",
      );
      s.check(
        stringField(field(suggestion, "fieldsBefore"), "front") !== REFINED_FRONT,
        "anki: the suggestion filled the fields before the learner used it",
      );
      const used = field(value, "used");
      s.check(stringField(field(used, "preview"), "edited") === "false", "anki: the card built from the unchanged suggestion is marked as a learner edit");
      s.check(
        numberField(used, "notes") === 0 && numberField(used, "localNotes") === 0 && stringField(used, "draft") === stringField(restored, "draft") && field(used, "editing") === true,
        "anki: using the suggestion accepted the note or closed the draft",
      );
      const shown = await cli(s, ["note", "show"]);
      const accepted = arrayField(shown.doc, "notes")
        .map((item) => field(item, "note"))
        .find((n) => (stringField(n, "front") ?? "").includes("smoke-edit"));
      s.check(
        accepted !== undefined && (stringField(accepted, "front") ?? "").startsWith(REFINED_FRONT) && stringField(accepted, "origin") === "edited",
        `anki: the accepted note is front ${String(stringField(accepted, "front"))}, origin ${String(stringField(accepted, "origin"))}; want the edited suggestion, origin edited`,
      );
      s.check(fronts.length === 1 && fronts[0].includes(REFINED_FRONT), "anki: the pushed note does not carry the used suggestion");
    } finally {
      provider.respond = () => chatReply("OK");
      stub.stop();
    }
  },
};

const settings: Scenario = {
  id: "settings",
  async run(s) {
    const { provider } = s.shared;
    const key = `pq-smoke-${randomBytes(16).toString("hex")}`;
    const account = `api-key:${STUB_PROVIDER}`;
    provider.received.length = 0;
    try {
      const launched = await launch(s, {
        label: "settings",
        script: pageScript(`
const KEY = ${JSON.stringify(key)};
const row = '.rrow[data-provider="${STUB_PROVIDER}"]';
const leaks = [];
const look = (when) => {
  if (document.documentElement.outerHTML.includes(KEY) || document.body.innerText.includes(KEY)) leaks.push(when);
};
location.hash = "#/settings";
await until("the stub provider row", () => q(row + ' [data-credential="missing"]'));
await click('[data-act="cred"][data-v="${STUB_PROVIDER}"]');
const input = await until("the key field", () => q('#credform input[name="key"]'));
input.value = KEY;
await click('[data-act="credential-save"]');
await until("the stored key", () => q(row + ' [data-credential="present"]'), 20000);
look("after saving");
await click('[data-act="model-test"][data-v="${STUB_MODEL}"]');
await until("the verified model", () => q(row + ' [data-capability="verified"]'), 30000);
look("after the model test");
if (!q('[data-act="cred-clear"][data-v="${STUB_PROVIDER}"]')) await click('[data-act="row"][data-v="${STUB_PROVIDER}"]');
await until("the remove-key button", () => q('[data-act="cred-clear"][data-v="${STUB_PROVIDER}"]'));
await click('[data-act="cred-clear"][data-v="${STUB_PROVIDER}"]');
await until("the cleared key", () => q(row + ' [data-credential="missing"]'), 20000);
look("after clearing");
return { leaks };
`),
      });
      if (!launched) return;
      const leaks = arrayField(launched.value, "leaks");
      s.check(leaks.length === 0, `settings: the key is in the DOM ${leaks.join(", ")}`);
      // The launch environment carries no key, so the provider can only have
      // received it from the OS credential store.
      s.check(
        provider.received.some((request) => request.authorization === `Bearer ${key}`),
        "settings: the provider call after entering the key did not carry it (OS store path)",
      );
      const scanned = [...filesUnder(s.dataDir), launched.resultPath];
      const holders = scanned.filter((file) => containsSecret(readFileSync(file), key)).map((file) => path.relative(s.dir, file));
      if (containsSecret(Buffer.from(launched.output), key)) holders.push("app output");
      s.check(holders.length === 0, `settings: the key bytes are in ${holders.join(", ")}`);
      s.check(scanned.some((file) => file.endsWith(".sqlite")), "settings: no SQLite file was scanned for the key");
      if (process.platform === "win32") {
        // The full list names stored targets only (`/list:<target>` echoes the target it looked for).
        const listed = await run(executable("cmdkey"), ["/list"]);
        s.check(
          listed.code === 0 && !listed.stdout.includes(s.service),
          "settings: the cleared key is still in the Windows credential store",
        );
      }
    } finally {
      await forgetCredential(s.service, account);
    }
  },
};

const resume: Scenario = {
  id: "resume",
  async run(s) {
    const displayed: { item: { token: string; presentedAt: string } | null } = { item: null };
    const killedRun = await launch(s, {
      label: "present",
      script: pageScript(`
await today();
press("Enter");
await until("a practice item", () => q('[data-testid="practice-item"]'));
await new Promise(() => {});
`),
      // Kill one poll after the store holds the open presentation.
      killWhen: async () => {
        if (displayed.item) return true;
        displayed.item = openPresentation(s.database);
        return false;
      },
    });
    if (!killedRun) return;
    const kept = displayed.item;
    if (!s.check(killedRun.killed && kept !== null, "resume: no item was presented, so nothing was killed mid-item") || !kept) return;
    const relaunchedAt = Date.now();
    const relaunched = await launch(s, {
      label: "relaunch",
      script: pageScript(`
await until("the resumed item", () => q('[data-testid="practice-item"]'), 20000);
return { item: data('[data-testid="practice-item"]') };
`),
    });
    if (!relaunched) return;
    const item = field(relaunched.value, "item");
    s.check(stringField(item, "token") === kept.token, "resume: the relaunched window shows another presentation token");
    s.check(stringField(item, "presentedAt") === kept.presentedAt, "resume: the relaunched window shows another presentation time");
    s.check(instant(stringField(item, "presentedAt")) < relaunchedAt, "resume: the item was presented again on relaunch");
    const attempts = attemptsOf(await cli(s, ["report"]));
    s.check(attempts.length === 0, `resume: the killed item wrote ${attempts.length} attempt(s)`);
  },
};

/** client-fresh and boot run first; the rest need a window that boots. */
const SCENARIOS: Scenario[] = [clientFresh, boot, lifecycle, practice, hint, feedback, coach, assess, anki, settings, resume];

export const desktopSmoke: Gate = {
  id: "desktop-smoke",
  title: "desktop smoke",
  async run(ctx) {
    const surface = desktopSurface(ctx);
    if (surface) return surface;

    const temp = mkdtempSync(path.join(tmpdir(), "passquest-desktop-smoke-"));
    const checks: string[] = [];
    let assertions = 0;
    const check: Check = (condition, message) => {
      assertions++;
      if (!condition) checks.push(message);
      return condition;
    };
    const provider = startProviderStub();
    try {
      const binary = path.join(temp, executable("passquest-desktop"));
      const build = await buildDesktop(ctx, binary);
      if (!build.ok) return fail(build.failures);
      const registry = path.join(temp, "registry.json");
      writeStubRegistry(registry, provider.endpoint);
      const shared: Shared = { ctx, binary, cli: path.join(temp, executable("passquest")), registry, provider };
      let ran = 0;
      for (const scenario of SCENARIOS) {
        const dir = path.join(temp, scenario.id);
        const dataDir = path.join(dir, "data");
        mkdirSync(dataDir, { recursive: true });
        const before = checks.length;
        await scenario.run({
          shared,
          id: scenario.id,
          dir,
          dataDir,
          database: path.join(dataDir, "passquest.sqlite"),
          service: `passquest-smoke-${process.pid}-${Date.now()}-${scenario.id}`,
          check,
        });
        ran++;
        if (scenario !== boot) continue;
        if (checks.length > before) {
          checks.push(`${SCENARIOS.length - ran} later scenario(s) not run: the window does not boot`);
          return fail(checks);
        }
        // Later scenarios cross-check the window through a second process.
        const cliBuild = await runGo(ctx, ["build", "-o", shared.cli, "./cmd/passquest"]);
        if (cliBuild.code !== 0) return fail([...checks, "go build ./cmd/passquest failed", tail(`${cliBuild.stdout}${cliBuild.stderr}`)]);
        const node = await run(executable("node"), ["--version"]);
        if (node.code !== 0) return fail([...checks, `node is required for the desktop smoke gate: ${node.stderr || node.stdout}`]);
      }
      if (checks.length > 0) return fail(checks);
      return pass(`${assertions} assertions over ${SCENARIOS.length} scenarios`);
    } finally {
      provider.stop();
      await removeTree(temp);
    }
  },
};
