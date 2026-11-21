import { randomBytes } from "node:crypto";

import { chatReply, pageScript, STUB_CREDENTIAL_ENV } from "../../lib/desktop-page.ts";
import { arrayField, field, numberField, stringField } from "../../lib/json.ts";
import { cli, HOSTILE_MARKUP, launch, seedStudyTrail } from "./scenario.ts";
import type { Scenario } from "./scenario.ts";

const IMPORTED_SENTENCE = "The Westbridge Community Library closes at 4 p.m. on Saturdays.";

export const coach: Scenario = {
  id: "coach",
  async run(s) {
    const { provider } = s.shared;
    const key = `sk-smoke-${randomBytes(12).toString("hex")}`;
    const keyEnv = { [STUB_CREDENTIAL_ENV]: key };
    if (!(await seedStudyTrail(s, keyEnv))) return;

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
