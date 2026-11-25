import { createHash, randomBytes } from "node:crypto";

import { startAnkiStub } from "../../lib/anki-stub.ts";
import { chatReply, defaultStubReply, pageScript, parse, STUB_CREDENTIAL_ENV, STUB_MODEL } from "../../lib/desktop-page.ts";
import { arrayField, field, numberField, stringField } from "../../lib/json.ts";
import { run } from "../../lib/process.ts";
import { cli, HOSTILE_MARKUP, launch, setup } from "./scenario.ts";
import type { Scenario } from "./scenario.ts";

/** The AI refinement the stub returns; its back carries markup that must stay text. */
const REFINED_FRONT = "smoke-refined NOT EXCEPT";
const REFINED_BACK = `Refined back: read the options against the passage. ${HOSTILE_MARKUP}`;
/** How long the stub holds the refinement sent after Retry: the window is checked while it waits. */
const REFINE_HOLD_MS = 5_000;

export const anki: Scenario = {
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
      // The first refinement is not a JSON object (a typed failure); the one sent after Retry is, and
      // it is held back so the window can be checked while the request waits.
      let refineReleasedAt = 0;
      provider.respond = async () => {
        if (provider.received.length === 1) return chatReply("I would rather not answer in JSON.");
        if (provider.received.length === 2) {
          await new Promise((done) => setTimeout(done, REFINE_HOLD_MS));
          refineReleasedAt = Date.now();
        }
        return chatReply(JSON.stringify({ front: REFINED_FRONT, back: REFINED_BACK }));
      };
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
    layout: "offset " + f.offsetHeight + ", rect " + Math.round(f.getBoundingClientRect().height) + ", computed " + getComputedStyle(f).height + ", page " + document.visibilityState +
      ", anims [" + (f.getAnimations ? f.getAnimations().map((a) => a.transitionProperty || a.animationName || a.constructor.name).join("/") : "n/a") + "]",
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
// The stub holds this reply. While it waits the draft is held: no Accept or Skip.
await until("the send in flight", () => q('[data-act="refine-send"][disabled]'), 5000);
const accepts = () => all('[data-act="accept"]').map((b) => b.disabled);
const skips = () => all('[data-act="askip"]').map((b) => b.disabled);
const sending = { accept: accepts(), skip: skips(), reason: q('[data-testid="refine-wait"]')?.textContent ?? null };
// Closing the panel hides the request without dropping it; reopening shows it still waiting.
await click('[data-act="refine-close"]');
await until("the closed panel", () => !q('[data-testid="refine"]'));
const closed = { accept: accepts(), skip: skips(), reason: q('[data-testid="refine-wait"]')?.textContent ?? null };
await click('.esheet [data-act="refine"]');
await until("the reopened panel", () => q('[data-testid="refine"]'));
// Long enough for a fresh preview to load, if the window wrongly asked for one.
await sleep(800);
const resend = q('[data-act="refine-send"]');
const reopened = {
  send: resend ? { disabled: resend.disabled, text: resend.textContent } : null,
  strip: data('[data-testid="refine-strip"]'),
  suggestion: !!q('[data-testid="refine-suggestion"]'),
  failure: !!q('[data-testid="refine-failure"]'),
  focused: q('[data-testid="refine"]').contains(document.activeElement),
  accept: accepts(),
};
// A second send for the same draft must not start while the first waits.
if (resend && !resend.disabled) resend.click();
const sug = await until("the suggestion", () => q('[data-testid="refine-suggestion"]'), 30000);
const refineSettled = { at: Date.now(), accept: accepts(), skip: skips(), reason: !!q('[data-testid="refine-wait"]') };
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
return { restored, proposed, edited, pushed, queued: data('[data-testid="sync-counts"]'), notes: all('[data-testid="note"]').map((note) => note.dataset.state), card, refinePreview, refineFailed, refineRetried, sending, closed, reopened, refineSettled, suggestion, used };
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
      // A refinement on its way holds the draft, survives closing the panel and is never sent twice.
      const held = (what: string, state: unknown) => {
        const accept = arrayField(state, "accept");
        const skip = arrayField(state, "skip");
        s.check(accept.length > 0 && accept.every((d) => d === true), `anki: Accept is enabled while a refinement waits (${what}: ${JSON.stringify(accept)})`);
        s.check(skip.length > 0 && skip.every((d) => d === true), `anki: Skip is enabled while a refinement waits (${what}: ${JSON.stringify(skip)})`);
        s.check((stringField(state, "reason") ?? "").trim() !== "", `anki: no reason is shown for the held Accept and Skip (${what})`);
      };
      held("sending", field(value, "sending"));
      held("panel closed", field(value, "closed"));
      const reopened = field(value, "reopened");
      const resend = field(reopened, "send");
      s.check(
        resend !== undefined && resend !== null && field(resend, "disabled") === true && field(reopened, "suggestion") === false && field(reopened, "failure") === false,
        `anki: the reopened panel does not show the waiting request (send ${JSON.stringify(resend)}, suggestion ${String(field(reopened, "suggestion"))})`,
      );
      s.check(
        stringField(field(reopened, "strip"), "sha") === stringField(field(value, "refineRetried"), "sha"),
        "anki: the reopened panel shows a fresh preview instead of the request on its way",
      );
      s.check(field(reopened, "focused") === true, "anki: focus is not in the reopened refinement panel");
      const settled = field(value, "refineSettled");
      s.check(
        refineReleasedAt > 0 && (numberField(settled, "at") ?? 0) >= refineReleasedAt,
        `anki: the suggestion was shown before the held reply was released (shown ${String(numberField(settled, "at"))}, released ${refineReleasedAt})`,
      );
      s.check(
        arrayField(settled, "accept").every((d) => d === false) && arrayField(settled, "skip").every((d) => d === false) && field(settled, "reason") === false,
        "anki: Accept or Skip stays held after the refinement settled",
      );
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
      provider.respond = defaultStubReply;
      stub.stop();
    }
  },
};
