/**
 * The package smoke's "the runtime is gone" scenario (goal 0013, step P5):
 * on an installed copy whose bundled Node runtime was deleted, the app still
 * practices, still drafts and accepts notes, and every agent feature reports
 * itself unavailable with one stated action — the coach call, the note
 * refinement, the Anki push, the durable brief and the local-services sheet.
 * The desktop-smoke scenarios supply the working paths (practice, assessment,
 * coach with the runtime present); this one drives the failure paths.
 */
import { randomBytes } from "node:crypto";

import { pageScript, STUB_CREDENTIAL_ENV, STUB_MODEL } from "../../lib/desktop-page.ts";
import { arrayField, field, stringField } from "../../lib/json.ts";
import { attemptsOf, checkAttempts, cli, launch, setup } from "../desktop-smoke/scenario.ts";
import type { Scenario } from "../desktop-smoke/scenario.ts";

/** The one code every agent surface must report when its runtime is missing. */
export const NODE_UNAVAILABLE = "node_unavailable";

const SCRIPT = `
const callError = (e) => {
  const raw = e && typeof e === "object" && typeof e.message === "string" ? e.message : String(e);
  try {
    const doc = JSON.parse(raw);
    return { code: typeof doc.code === "string" ? doc.code : null, action: typeof doc.action === "string" ? doc.action : "", message: typeof doc.message === "string" ? doc.message : "" };
  } catch {
    return { code: null, action: "", message: raw };
  }
};
const failureCard = (element) => ({
  code: element.dataset.code ?? null,
  next: (element.querySelector(".next")?.textContent ?? "").trim(),
  detail: Array.from(element.querySelectorAll("details.raw code"), (code) => code.textContent ?? ""),
});
await today();
press("Enter");
const answered = await answer("1");
// 1. The coach call: the preview renders locally, the send reports the missing runtime.
await until("the coach preview", () => q('[data-act="coach-send"]') || q('[data-testid="coach-failure"]'), 20000);
let coach = null;
if (q('[data-act="coach-send"]')) {
  await click('[data-act="coach-send"]');
  coach = failureCard(await until("the coach failure", () => q('[data-testid="coach-failure"]'), 30000));
}
// 2. The refinement panel reports the missing runtime; the note still drafts and accepts locally.
press("k");
await until("the card preview", () => q('[data-testid="card-preview"]'), 20000);
await click('[data-act="refine"]');
await until("the refinement preview or its failure", () => q('[data-act="refine-send"]') || q('[data-testid="refine-failure"]'), 20000);
if (q('[data-act="refine-send"]')) await click('[data-act="refine-send"]');
const refine = failureCard(await until("the refinement failure", () => q('[data-testid="refine-failure"]'), 30000));
await click("#acceptbtn");
await until("the accepted note", () => q('[data-testid="note"][data-state="local"]'), 20000);
await sleep(200);
const accepted = all('[data-testid="note"]').map((note) => note.dataset.state);
// 3. The Anki push reports the missing runtime and leaves the accepted note alone.
await click('[data-act="push"]');
await until("the push to finish", () => q('.toast') !== null, 20000);
const pushToast = (q('.toast')?.textContent ?? "").trim();
await sleep(200);
const push = {
  toast: pushToast,
  notes: all('[data-testid="note"]').map((note) => note.dataset.state),
  direct: await window.mygo.call("Study.PushNotes", false).then(() => null, callError),
};
// 4. The durable brief: the call returns at once and reports through brief:changed.
const events = [];
const off = window.mygo.on("brief:changed", (payload) => events.push(payload));
const called = await window.mygo.call("Study.ResumeBrief").then(() => ({ code: "ok", action: "" }), callError);
const finished = await until("the brief failure", () => events.find((event) => event && event.error), 20000);
off();
const brief = { called, phase: finished.phase ?? null, error: finished.error };
// 5. The local-services sheet states the agent host's state and its one action.
location.hash = "#/settings";
await until("the settings room", () => q('[data-testid="settings"]'), 20000);
await click('[data-act="sheet"][data-v="services"]');
const node = await until("the services sheet", () => q('[data-testid="svc-node"]'), 20000);
const services = {
  available: node.dataset.available ?? null,
  next: (q('[data-testid="svc-node-next"]')?.textContent ?? "").trim(),
};
return { answered, coach, refine, accepted, push, brief, services };
`;

export const unavailable: Scenario = {
  id: "unavailable",
  async run(s) {
    // The active model must be configured (and verified, for the refinement
    // preview) for the panels to render their send previews at all; the CLI
    // runs this from the checkout, with the checkout's own Node.
    const key = `sk-smoke-${randomBytes(12).toString("hex")}`;
    const keyEnv = { [STUB_CREDENTIAL_ENV]: key };
    if (!(await setup(s, [["model", "test", STUB_MODEL], ["model", "use", STUB_MODEL]], keyEnv))) return;
    const launched = await launch(s, { label: "unavailable", env: keyEnv, script: pageScript(SCRIPT) });
    if (!launched) return;
    const value = launched.value;

    // Practice still works with the runtime gone: the same attempt cross-check the desktop smoke uses.
    const answered = field(value, "answered");
    const attempts = attemptsOf(await cli(s, ["report"]));
    s.check(attempts.length === 1, `unavailable: report --json lists ${attempts.length} attempt(s) for the answered item, want 1`);
    checkAttempts(s, [answered], attempts);

    // The coach call reports the node it could not run, with one stated next step.
    const coach = field(value, "coach");
    s.check(stringField(coach, "code") === NODE_UNAVAILABLE, `unavailable: the coach failure code is ${String(stringField(coach, "code"))}, want ${NODE_UNAVAILABLE}`);
    s.check((stringField(coach, "next") ?? "") !== "", "unavailable: the coach failure states no next step");

    // The note was accepted locally; the push reported the missing runtime and left it alone.
    const accepted = arrayField(value, "accepted").map(String);
    s.check(accepted.includes("local"), `unavailable: accepting a note left states ${accepted.join(", ") || "none"}`);
    const push = field(value, "push");
    s.check((stringField(push, "toast") ?? "") !== "", "unavailable: pushing with the runtime gone showed no failure in the window");
    const notes = arrayField(push, "notes").map(String);
    s.check(notes.length === 1 && notes[0] === "local", `unavailable: the push changed the note states to ${notes.join(", ")}`);
    const direct = field(push, "direct");
    s.check(stringField(direct, "code") === NODE_UNAVAILABLE, `unavailable: the push call reported ${String(stringField(direct, "code"))}, want ${NODE_UNAVAILABLE}`);
    s.check((stringField(direct, "action") ?? "") !== "", "unavailable: the push failure carries no next action");

    // The refinement reports the missing runtime from its own failure card.
    const refine = field(value, "refine");
    s.check(stringField(refine, "code") === NODE_UNAVAILABLE, `unavailable: the refinement failure code is ${String(stringField(refine, "code"))}, want ${NODE_UNAVAILABLE}`);
    s.check((stringField(refine, "next") ?? "") !== "", "unavailable: the refinement failure states no next step");

    // The durable brief reports the missing runtime through its event, with one action.
    const brief = field(value, "brief");
    const called = field(brief, "called");
    const error = field(brief, "error");
    const code = stringField(error, "code") ?? stringField(called, "code");
    const action = stringField(error, "action") ?? stringField(called, "action");
    s.check(code === NODE_UNAVAILABLE, `unavailable: the durable brief reported ${String(code)}, want ${NODE_UNAVAILABLE}`);
    s.check((action ?? "") !== "", "unavailable: the durable brief failure carries no next action");

    // The window's own services sheet says the agent host is unavailable, one action stated.
    const services = field(value, "services");
    s.check(field(services, "available") === "false", `unavailable: the services sheet reports the agent host available=${String(field(services, "available"))}`);
    s.check((stringField(services, "next") ?? "") !== "", "unavailable: the services sheet states no next step");
  },
};
