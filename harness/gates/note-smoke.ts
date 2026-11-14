import { mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { startAnkiStub } from "../lib/anki-stub.ts";
import { runGo } from "../lib/go.ts";
import { arrayField, field, numberField, stringField } from "../lib/json.ts";
import { executable, run } from "../lib/process.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate, GateContext } from "../lib/types.ts";

const FORBIDDEN_PHONE_CLAIMS = [
  "pushed to your phone",
  "sent to your phone",
  "sync to your phone",
  "syncs to your phone",
  "will appear on your phone",
  "automatically sync",
  "synchronized to ankiweb",
  "phone is up to date",
];

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

function stringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}

export const noteSmoke: Gate = {
  id: "note-smoke",
  title: "note smoke",
  async run(ctx: GateContext) {
    const binaryName = process.platform === "win32" ? "passquest.exe" : "passquest";
    const temp = mkdtempSync(path.join(tmpdir(), "passquest-note-smoke-"));
    const binary = path.join(temp, binaryName);
    const db = path.join(temp, "note.sqlite");
    const checks: string[] = [];
    const outputs: string[] = [];
    let assertions = 0;
    const check = (condition: boolean, message: string): boolean => {
      assertions++;
      if (!condition) checks.push(message);
      return condition;
    };

    const anki = startAnkiStub();
    const { actions, notes, noteDecks, endpoint } = anki;
    const hostPath = path.join(ctx.root, "agent", "anki-host.mjs");
    const baseEnv = { PASSQUEST_ANKI_HOST: hostPath };

    const call = async (args: string[], env: Record<string, string> = {}) => {
      const result = await run(binary, [...args, "--db", db], {
        cwd: ctx.root,
        env: { ...baseEnv, ...env },
      });
      outputs.push(result.stdout, result.stderr);
      return result;
    };
    const callJSON = async (args: string[], env: Record<string, string> = {}) => {
      const result = await call([...args, "--json"], env);
      return { ...result, doc: parse(result.stdout), failure: parse(result.stderr) };
    };
    const callNoDB = async (args: string[], env: Record<string, string> = {}) => {
      const result = await run(binary, [...args, "--json"], { cwd: ctx.root, env: { ...baseEnv, ...env } });
      outputs.push(result.stdout, result.stderr);
      return { ...result, doc: parse(result.stdout) };
    };
    const actionNames = (from: number) => actions.slice(from).map((item) => item.action);

    try {
      const build = await runGo(ctx, ["build", "-o", binary, "./cmd/passquest"]);
      if (build.code !== 0) {
        return fail(["go build ./cmd/passquest failed", `${build.stdout}${build.stderr}`.trim()]);
      }

      const importResult = await callJSON([
        "import",
        "--path",
        path.join(ctx.root, "internal", "cli", "testdata", "import"),
      ]);
      if (!check(importResult.code === 0, `import exit = ${importResult.code}`)) return fail(checks);
      const sourceID = stringField(field(importResult.doc, "source"), "id");
      if (!check(sourceID !== undefined, "import did not return a source id")) return fail(checks);

      for (const args of [
        ["question", "add", sourceID!, "1", "--file", path.join(ctx.root, "internal", "cli", "testdata", "import", "question-q2-tagged.json")],
        ["page", "review", sourceID!, "1", "--reviewer", "smoke"],
        ["question", "review", sourceID!, "q2", "--answer", "C", "--reviewer", "smoke"],
        ["practice", "--source", sourceID!, "--page", "1", "--question", "q2", "--answer", "A"],
      ]) {
        const result = await callJSON(args);
        check(result.code === 0, `${args.join(" ")} exit = ${result.code}: ${result.stderr}`);
      }

      // Before consent, preview must not contain imported text and accept must refuse.
      const propose = await callJSON(["note", "propose", "--attempt", "1"]);
      if (!check(propose.code === 0, `note propose exit = ${propose.code}`)) return fail(checks);
      const draft = field(propose.doc, "draft");
      const draftID = numberField(draft, "id");
      if (!check(draftID !== undefined, "note propose returned no draft id")) return fail(checks);
      check(field(draft, "restricted") === true, "draft is not restricted for an imported source");
      check(field(draft, "consent_granted") === false, "draft has consent before it was granted");
      check(stringArray(field(draft, "withheld")).includes("front"), "draft does not withhold the front");
      check(stringArray(field(draft, "withheld")).includes("back"), "draft does not withhold the back");
      check(stringField(field(draft, "target"), "source_id") === sourceID, "draft source id");
      check(stringField(draft, "tactic_id") === "not-except-polarity", "draft tactic id");
      check(field(field(propose.doc, "fields"), "Front") === "", "draft before consent carries front text");
      check(field(field(propose.doc, "fields"), "Back") === "", "draft before consent carries back text");

      const withheldPreview = await callJSON(["note", "preview"]);
      if (!check(withheldPreview.code === 0, `withheld preview exit = ${withheldPreview.code}`)) return fail(checks);
      for (const secret of [
        "The Westbridge Community Library closes at 4 p.m. on Saturdays.",
        "No Sunday opening time appears in the notice.",
      ]) {
        check(!withheldPreview.stdout.includes(secret), `withheld preview leaked ${secret.slice(0, 32)}`);
      }
      const refusedAccept = await call(["note", "accept", "--json"]);
      check(refusedAccept.code === 2, `accept before consent exit = ${refusedAccept.code}`);
      check(
        stringField(field(parse(refusedAccept.stderr), "error"), "code") === "consent_required",
        "accept before consent error code",
      );

      const consent = await callJSON(["tutor", "consent", sourceID!, "--reviewer", "smoke"]);
      check(consent.code === 0, `tutor consent exit = ${consent.code}`);
      const authorizedPreview = await callJSON(["note", "preview"]);
      check(field(field(authorizedPreview.doc, "draft"), "consent_granted") === true, "preview after consent is not granted");
      check(
        field(field(authorizedPreview.doc, "fields"), "Front") === "The library opens at 8 a.m. on Sundays.",
        "authorized preview front",
      );
      check(
        (stringField(field(authorizedPreview.doc, "fields"), "Back") ?? "").includes(
          "No Sunday opening time appears in the notice.",
        ),
        "authorized preview back",
      );
      check(
        (stringField(field(authorizedPreview.doc, "fields"), "Back") ?? "").includes("pq-example") &&
          (stringField(field(authorizedPreview.doc, "fields"), "Back") ?? "").includes(
            "Example: The Westbridge Community Library closes at 4 p.m. on Saturdays.",
          ),
        "authorized preview back has no example paragraph",
      );

      const firstAccept = await callJSON(["note", "accept"]);
      if (!check(firstAccept.code === 0, `first accept exit = ${firstAccept.code}`)) return fail(checks);
      const noteID = stringField(field(firstAccept.doc, "note"), "note_id");
      if (!check(noteID !== undefined, "first accept returned no note id")) return fail(checks);
      check(field(firstAccept.doc, "state") === "accepted", "first accept state");
      check(numberField(field(firstAccept.doc, "note"), "revision") === 1, "first accept revision");
      const repeatAccept = await callJSON(["note", "accept"]);
      check(repeatAccept.code === 0, `repeat accept exit = ${repeatAccept.code}`);
      check(field(repeatAccept.doc, "state") === "unchanged", "repeat accept state");
      check(stringField(field(repeatAccept.doc, "note"), "note_id") === noteID, "repeat accept note id");
      check(numberField(field(repeatAccept.doc, "note"), "revision") === 1, "repeat accept revision");

      const editedAccept = await callJSON([
        "note",
        "accept",
        "--front",
        "closes on Sundays",
        "--back",
        "A closing time.\n\nExample: The notice gives Saturday hours.",
      ]);
      check(editedAccept.code === 0, `edited accept exit = ${editedAccept.code}`);
      check(numberField(field(editedAccept.doc, "note"), "revision") === 2, "edited accept revision");
      check(numberField(editedAccept.doc, "previous_revision") === 1, "edited accept previous revision");
      const show = await callJSON(["note", "show", noteID!]);
      check(show.code === 0, `note show exit = ${show.code}`);
      check(arrayField(show.doc, "notes").length === 1, "note show did not find the note");
      const revisions = arrayField(arrayField(show.doc, "notes")[0], "revisions");
      check(revisions.length === 2, `note show revisions = ${revisions.length}`);

      const exportResult = await callJSON(["note", "export"]);
      if (!check(exportResult.code === 0, `note export exit = ${exportResult.code}`)) return fail(checks);
      const exportPath = stringField(exportResult.doc, "path");
      if (!check(exportPath !== undefined, "export returned no path")) return fail(checks);
      const exportText = readFileSync(exportPath!, "utf8");
      for (const want of [
        "#separator:tab",
        "#html:true",
        "#notetype:PassQuest Note",
        "#deck:Pass Quest",
        "#tags:passquest",
        "#columns:Front\tBack\tTactic\tSource",
        "closes on Sundays",
      ]) {
        check(exportText.includes(want), `export is missing ${want}`);
      }
      check(exportText.split("\n").length === 8, `export line count = ${exportText.split("\n").length}`);
      check(!exportText.includes("\nFront\tBack\tTactic\tSource\n"), "export has an uncommented header row");
      const exportDir = path.join(temp, "export-target");
      mkdirSync(exportDir);
      const failedExport = await call(["note", "export", "--out", exportDir, "--json"]);
      check(failedExport.code === 1, `failed export exit = ${failedExport.code}`);
      check(
        readdirSync(temp).every((name) => !name.includes(".tmp-")),
        "failed export left a temporary file",
      );

      const model = await callNoDB(["note", "model"]);
      check(model.code === 0, `note model exit = ${model.code}`);
      const css = stringField(model.doc, "css") ?? "";
      check(css.includes("font-family: -apple-system, BlinkMacSystemFont"), "model CSS is not the system font stack");
      check(!css.includes("overflow: hidden"), "model CSS clips long text");
      check(css.includes("overflow-wrap: anywhere"), "model CSS does not wrap long text");
      for (const forbidden of ["@import", "url(", "http://", "https://"]) {
        check(!css.includes(forbidden), `model CSS contains ${forbidden}`);
      }
      check(!/<\/?(link|script|img|iframe)/i.test(`${css}${stringField(model.doc, "front_template")}${stringField(model.doc, "back_template")}`), "model references an external element");
      check((stringField(model.doc, "rendered_sample") ?? "").includes("pq-front"), "model rendered sample is empty");

      // First push creates the deck and model, then adds the note.
      const beforePush = actions.length;
      const push = await callJSON(["note", "push", "--endpoint", endpoint]);
      if (!check(push.code === 0, `first push exit = ${push.code}: ${push.stderr}`)) return fail(checks);
      check(field(push.doc, "state") === "pushed", `first push state = ${String(field(push.doc, "state"))}`);
      check(field(push.doc, "model_created") === true, "first push did not create the model");
      check(numberField(field(push.doc, "counts"), "pushed") === 1, "first push count");
      check(
        numberField(field(push.doc, "counts"), "pending") === 0,
        "first push pending count is not the remaining queue",
      );
      check(numberField(arrayField(push.doc, "notes")[0], "anki_note_id") === 101, "first push note id");
      check(
        actionNames(beforePush).join(",") === "version,createDeck,modelNames,createModel,addNote,notesInfo,cardsInfo",
        `first push actions = ${actionNames(beforePush).join(",")}`,
      );
      const createModel = actions.slice(beforePush).find((item) => item.action === "createModel");
      if (!check(createModel !== undefined, "first push did not call createModel")) return fail(checks);
      check(stringField(createModel!.params, "modelName") === "PassQuest Note", "createModel name");
      check(
        stringArray(createModel!.params.inOrderFields).join(",") === "Front,Back,Tactic,Source",
        "createModel field order",
      );
      check(stringField(createModel!.params, "css") === css, "createModel CSS differs from note model");
      const template = asRecord(arrayField(createModel!.params, "cardTemplates")[0]);
      check(
        (stringField(template, "Front") ?? "").includes("{{Front}}") &&
          (stringField(template, "Back") ?? "").includes("{{FrontSide}}") &&
          (stringField(template, "Back") ?? "").includes("{{Back}}") &&
          (stringField(template, "Back") ?? "").includes("{{Tactic}}") &&
          (stringField(template, "Back") ?? "").includes("{{Source}}"),
        "createModel card template is incomplete",
      );
      const addNote = actions.slice(beforePush).find((item) => item.action === "addNote");
      const addFields = asRecord(asRecord(addNote?.params.note).fields);
      check(
        Object.keys(addFields).sort().join(",") === "Back,Front,Source,Tactic",
        "addNote fields are not the four model fields",
      );
      check(
        field(asRecord(asRecord(addNote?.params.note).options), "allowDuplicate") === false,
        "addNote does not request duplicate protection",
      );

      // An unchanged push must not contact the endpoint at all.
      const beforeIdle = actions.length;
      const idle = await callJSON(["note", "push", "--endpoint", endpoint]);
      check(idle.code === 0, `idle push exit = ${idle.code}`);
      check(field(idle.doc, "state") === "idle", "unchanged push is not idle");
      check(actions.length === beforeIdle, "unchanged push contacted the endpoint");

      // A revision updates the same note id and never adds a second card.
      await callJSON(["note", "accept", "--front", "closes on Sunday"]);
      const beforeUpdate = actions.length;
      const update = await callJSON(["note", "push", "--endpoint", endpoint]);
      check(update.code === 0, `update push exit = ${update.code}: ${update.stderr}`);
      check(
        actionNames(beforeUpdate).join(",") === "version,createDeck,modelNames,notesInfo,updateNoteFields,cardsInfo",
        `update push actions = ${actionNames(beforeUpdate).join(",")}`,
      );
      check((notes.get(101)?.Front ?? "") === "closes on Sunday", "update did not reach the same Anki note");
      check(field(arrayField(update.doc, "notes")[0], "deck") === undefined, "a matching card carries a deck field");
      check(
        numberField(field(update.doc, "counts"), "deck_mismatch") === undefined,
        "a matching push reports a deck mismatch count",
      );

      // A learner edit in Anki is a conflict; --force is the explicit overwrite.
      notes.get(101)!.Back = "learner edit";
      await callJSON(["note", "accept", "--front", "closes on Sunday only"]);
      const beforeConflict = actions.length;
      const conflict = await callJSON(["note", "push", "--endpoint", endpoint]);
      check(conflict.code === 3, `conflict push exit = ${conflict.code}`);
      check(field(conflict.failure, "state") === "conflict", "conflict push state");
      check(numberField(field(conflict.failure, "counts"), "conflicts") === 1, "conflict count");
      check(
        stringField(field(conflict.failure, "error"), "code") === "anki_conflict",
        "conflict push has no error envelope",
      );
      check(!actionNames(beforeConflict).includes("updateNoteFields"), "conflict overwrote the Anki note");
      const forced = await callJSON(["note", "push", "--endpoint", endpoint, "--force"]);
      check(forced.code === 0, `forced push exit = ${forced.code}: ${forced.stderr}`);
      check(notes.get(101)!.Back !== "learner edit", "forced push did not overwrite");

      // No endpoint: the note is queued and the next reachable push drains it once.
      await callJSON(["note", "accept", "--front", "queued wording"]);
      const queued = await callJSON(["note", "push", "--endpoint", "http://127.0.0.1:1"]);
      check(queued.code === 0, `queued push exit = ${queued.code}`);
      check(field(queued.doc, "state") === "queued", "unreachable push is not queued");
      const beforeDrain = actions.length;
      const drained = await callJSON(["note", "push", "--endpoint", endpoint]);
      check(drained.code === 0, `drained push exit = ${drained.code}`);
      check(field(drained.doc, "state") === "pushed", "queued note was not drained");
      const drainActions = actionNames(beforeDrain);
      check(
        drainActions.filter((name) => name === "updateNoteFields").length === 1,
        `drain updated ${drainActions.filter((name) => name === "updateNoteFields").length} times`,
      );
      const afterDrain = await callJSON(["note", "push", "--endpoint", endpoint]);
      check(field(afterDrain.doc, "state") === "idle", "a drained note was sent twice");

      // An endpoint that accepts the write and files the card in another deck.
      const reportBeforeMismatch = (await callJSON(["report", "--json"])).stdout;
      const nextBeforeMismatch = (await callJSON(["next", "--json"])).stdout;
      const questionsBeforeMismatch = (await callJSON(["questions", sourceID!, "--json"])).stdout;
      anki.misassigned = "Default";
      const mislocatedProposal = await callJSON([
        "note",
        "propose",
        "--content",
        `${sourceID}-p001`,
        "--question",
        "q2",
        "--mark",
        "mislocated wording",
      ]);
      const mislocatedDraft = numberField(field(mislocatedProposal.doc, "draft"), "id");
      if (!check(mislocatedDraft !== undefined, "the mismatch proposal returned no draft id")) return fail(checks);
      const mislocatedAccept = await callJSON(["note", "accept", "--draft", String(mislocatedDraft)]);
      if (!check(mislocatedAccept.code === 0, `mismatch accept exit = ${mislocatedAccept.code}: ${mislocatedAccept.stderr}`)) {
        return fail(checks);
      }
      const beforeMismatch = actions.length;
      const mismatch = await callJSON(["note", "push", "--endpoint", endpoint]);
      check(mismatch.code === 3, `mismatch push exit = ${mismatch.code}`);
      check(field(mismatch.failure, "state") === "deck_mismatch", "mismatch push state");
      check(
        stringField(field(mismatch.failure, "error"), "code") === "anki_deck_mismatch",
        "mismatch push has no error envelope",
      );
      check(numberField(field(mismatch.failure, "counts"), "deck_mismatch") === 1, "mismatch count");
      check(numberField(field(mismatch.failure, "counts"), "pushed") === 0, "mismatch push counts as pushed");
      check(numberField(field(mismatch.failure, "counts"), "pending") === 0, "mismatch left the note queued");
      const mismatchItem = arrayField(mismatch.failure, "notes")[0];
      const mismatchNoteID = stringField(mismatchItem, "note_id");
      const mismatchAnkiID = numberField(mismatchItem, "anki_note_id");
      check(field(mismatchItem, "state") === "deck_mismatch", `mismatch item state = ${String(field(mismatchItem, "state"))}`);
      check(field(mismatchItem, "outcome") === "deck_mismatch", "mismatch item outcome");
      check(stringField(mismatchItem, "deck") === "Default", "mismatch item does not name the landed deck");
      check(
        (stringField(mismatchItem, "detail") ?? "").includes('"Default"') &&
          (stringField(mismatchItem, "detail") ?? "").includes('"Pass Quest"'),
        `mismatch detail = ${String(stringField(mismatchItem, "detail"))}`,
      );
      // The deck can only come from the read: the write asked for Pass Quest and
      // the stub filed it elsewhere.
      const mismatchActions = actionNames(beforeMismatch);
      check(mismatchActions.includes("cardsInfo"), "the mismatch push never read the deck back");
      const mismatchAdd = actions.slice(beforeMismatch).find((item) => item.action === "addNote");
      check(
        stringField(asRecord(mismatchAdd?.params.note), "deckName") === "Pass Quest",
        `the mismatch push asked for deck ${String(stringField(asRecord(mismatchAdd?.params.note), "deckName"))}`,
      );
      check(mismatchAnkiID !== undefined && noteDecks.get(mismatchAnkiID) === "Default", "the stub did not mis-assign the card");
      if (!check(typeof mismatchNoteID === "string", "the mismatch item has no note id")) return fail(checks);

      // The note id survives, and note show reports the deck it reached.
      const mismatchShow = await callJSON(["note", "show", mismatchNoteID!]);
      if (!check(arrayField(mismatchShow.doc, "notes").length === 1, "note show lost the mismatched note")) {
        return fail(checks);
      }
      const shownNote = field(arrayField(mismatchShow.doc, "notes")[0], "note");
      const shownAnki = field(shownNote, "anki");
      check(stringField(shownAnki, "state") === "deck_mismatch", "note show does not report the mismatch");
      check(stringField(shownAnki, "deck") === "Default", "note show does not report the landed deck");
      check(stringField(shownAnki, "deck_expected") === "Pass Quest", "note show does not report the expected deck");
      check(numberField(shownAnki, "note_id") === mismatchAnkiID, "note show lost the Anki note id");

      // The stated action is in the human summary, and the count line still does
      // not claim a push.
      await callJSON(["note", "accept", "--draft", String(mislocatedDraft), "--front", "mislocated one"]);
      const humanMismatch = await call(["note", "push", "--endpoint", endpoint]);
      check(humanMismatch.code === 3, `human mismatch exit = ${humanMismatch.code}`);
      for (const want of [
        'Deck mismatch: 1 note(s) are not in "Pass Quest"',
        'landed in "Default"',
        "anki_deck_mismatch",
        'Move the card into "Pass Quest" in Anki',
      ]) {
        check(humanMismatch.stderr.includes(want), `human mismatch output is missing ${want}`);
      }
      check(!humanMismatch.stderr.includes("pushed 1"), "human mismatch output claims a push");
      check(humanMismatch.stdout === "", `human mismatch wrote ${humanMismatch.stdout.length} bytes to stdout`);

      const countsBeforeRepeat = notes.size;
      const beforeRepeat = actions.length;
      const repeatMismatch = await callJSON(["note", "push", "--endpoint", endpoint]);
      check(repeatMismatch.code === 0, `repeat mismatch exit = ${repeatMismatch.code}`);
      check(field(repeatMismatch.doc, "state") === "idle", "a repeat push over a mismatched note is not idle");
      check(actions.length === beforeRepeat, "a repeat push over a mismatched note contacted the endpoint");
      check(notes.size === countsBeforeRepeat, "a repeat push added a second card");

      // The next revision updates the same card, still reporting where it is.
      await callJSON(["note", "accept", "--draft", String(mislocatedDraft), "--front", "mislocated two"]);
      const beforeRevision = actions.length;
      const mismatchedRevision = await callJSON(["note", "push", "--endpoint", endpoint]);
      check(mismatchedRevision.code === 3, `mismatched revision exit = ${mismatchedRevision.code}`);
      const revisionActions = actionNames(beforeRevision);
      check(
        revisionActions.filter((name) => name === "updateNoteFields").length === 1,
        `mismatched revision updated ${revisionActions.filter((name) => name === "updateNoteFields").length} times`,
      );
      check(!revisionActions.includes("addNote"), "mismatched revision added a second card");
      check(
        numberField(arrayField(mismatchedRevision.failure, "notes")[0], "anki_note_id") === mismatchAnkiID,
        "the mismatched revision did not keep the Anki note id",
      );
      check((notes.get(mismatchAnkiID!)?.Front ?? "") === "mislocated two", "the revision did not reach the same card");

      // The learner moves the card by hand; the next revision confirms the deck
      // instead of assuming it.
      noteDecks.set(mismatchAnkiID!, "Pass Quest");
      await callJSON(["note", "accept", "--draft", String(mislocatedDraft), "--front", "mislocated three"]);
      const moved = await callJSON(["note", "push", "--endpoint", endpoint]);
      check(moved.code === 0, `moved push exit = ${moved.code}: ${moved.stderr}`);
      check(field(moved.doc, "state") === "pushed", "a moved card was not confirmed");
      check(
        numberField(field(moved.doc, "counts"), "deck_mismatch") === undefined,
        "a matching deck was counted as a mismatch",
      );
      check(field(arrayField(moved.doc, "notes")[0], "deck") === undefined, "a matching card carries a deck field");
      anki.misassigned = null;
      await callJSON(["note", "skip", "--draft", String(mislocatedDraft)]);

      check(
        (await callJSON(["report", "--json"])).stdout === reportBeforeMismatch,
        "a mismatched push changed the attempt report",
      );
      check((await callJSON(["next", "--json"])).stdout === nextBeforeMismatch, "a mismatched push changed next");
      check(
        (await callJSON(["questions", sourceID!, "--json"])).stdout === questionsBeforeMismatch,
        "a mismatched push changed the stored answer key",
      );

      // An unanswered reviewed question: preview must not include its key.
      for (const args of [
        ["question", "add", sourceID!, "1", "--file", path.join(ctx.root, "internal", "cli", "testdata", "import", "question-q1.json")],
        ["question", "review", sourceID!, "q1", "--answer", "B", "--reviewer", "smoke"],
      ]) {
        const result = await callJSON(args);
        check(result.code === 0, `${args.join(" ")} exit = ${result.code}`);
      }
      const unanswered = await callJSON([
        "note",
        "propose",
        "--content",
        `${sourceID}-p001`,
        "--question",
        "q1",
        "--mark",
        "closes",
      ]);
      check(unanswered.code === 0, `unanswered propose exit = ${unanswered.code}`);
      check(field(field(unanswered.doc, "draft"), "unanswered") === true, "unanswered draft flag");
      const unansweredPreview = await callJSON(["note", "preview", "--draft", String(numberField(field(unanswered.doc, "draft"), "id"))]);
      check(unansweredPreview.code === 0, `unanswered preview exit = ${unansweredPreview.code}`);
      const lowerPreview = unansweredPreview.stdout.toLowerCase();
      check(!lowerPreview.includes("at 4 p.m."), "unanswered preview includes the keyed option");
      check(!unansweredPreview.stdout.includes("At 4 p.m."), "unanswered preview includes the key text");
      const skipped = await call(["note", "skip", "--draft", String(numberField(field(unanswered.doc, "draft"), "id")), "--json"]);
      check(skipped.code === 0, "note skip failed");

      // A duplicate add is already-present; a later revision without an Anki id is a conflict.
      const duplicateDraft = await callJSON([
        "note",
        "propose",
        "--content",
        `${sourceID}-p001`,
        "--question",
        "q2",
        "--mark",
        "Sundays",
      ]);
      const duplicateDraftID = numberField(field(duplicateDraft.doc, "draft"), "id");
      await callJSON(["note", "accept", "--draft", String(duplicateDraftID)]);
      anki.duplicate = true;
      const duplicatePush = await callJSON(["note", "push", "--endpoint", endpoint]);
      check(duplicatePush.code === 0, `duplicate push exit = ${duplicatePush.code}`);
      check(field(duplicatePush.doc, "state") === "already_present", "duplicate push is not already_present");
      check(numberField(field(duplicatePush.doc, "counts"), "already_present") === 1, "duplicate count");
      await callJSON(["note", "accept", "--draft", String(duplicateDraftID), "--front", "Sundays v2"]);
      const duplicateRevision = await callJSON(["note", "push", "--endpoint", endpoint]);
      check(duplicateRevision.code === 3, `duplicate revision exit = ${duplicateRevision.code}`);
      check(field(duplicateRevision.failure, "state") === "conflict", "duplicate revision is not a conflict");
      anki.duplicate = false;

      // Revoked consent blocks a push before any endpoint contact.
      await callJSON(["note", "accept", "--front", "revoked word"]);
      await callJSON(["tutor", "revoke", sourceID!, "--reviewer", "smoke"]);
      const revokedBefore = actions.length;
      const revoked = await call(["note", "push", "--endpoint", endpoint, "--json"]);
      check(revoked.code === 2, `revoked push exit = ${revoked.code}`);
      check(
        stringField(field(parse(revoked.stderr), "error"), "code") === "consent_required",
        "revoked push error code",
      );
      check(actions.length === revokedBefore, "revoked push contacted Anki");
      await callJSON(["tutor", "consent", sourceID!, "--reviewer", "smoke"]);

      // A missing Node is an unavailable host, not a lost queue.
      const missingNode = path.join(temp, "missing-node");
      const nodeMissing = await call(["note", "push", "--endpoint", endpoint, "--json"], { PASSQUEST_NODE: missingNode });
      check(nodeMissing.code === 3, `missing node exit = ${nodeMissing.code}: ${nodeMissing.stderr}`);
      check(
        stringField(field(parse(nodeMissing.stderr), "error"), "code") === "anki_unavailable",
        `missing node error code: ${nodeMissing.stderr}`,
      );

      // The host itself rejects a remote endpoint and an unknown action.
      const remote = await run(executable("node"), [hostPath], {
        cwd: ctx.root,
        stdin: JSON.stringify({
          protocol_version: 1,
          endpoint: "http://10.0.0.2:8765",
          timeout_ms: 1000,
          actions: [{ action: "version" }],
        }),
      });
      check(stringField(parse(remote.stdout), "state") === "failed", "host accepted a remote endpoint");
      const mapped = await run(executable("node"), [hostPath], {
        cwd: ctx.root,
        stdin: JSON.stringify({
          protocol_version: 1,
          endpoint: "http://[::ffff:127.0.0.1]:1",
          timeout_ms: 1000,
          actions: [{ action: "version" }],
        }),
      });
      const mappedDoc = parse(mapped.stdout);
      check(
        stringField(mappedDoc, "state") === "unreachable",
        `host rejected an IPv4-mapped loopback endpoint: ${mapped.stdout}`,
      );
      const unknown = await run(executable("node"), [hostPath], {
        cwd: ctx.root,
        stdin: JSON.stringify({
          protocol_version: 1,
          endpoint,
          timeout_ms: 1000,
          actions: [{ action: "deleteEverything" }],
        }),
      });
      check(stringField(parse(unknown.stdout), "state") === "failed", "host accepted an unknown action");

      // No output may claim a phone push; the manual-sync boundary is stated.
      for (const output of outputs) {
        const lower = output.toLowerCase();
        for (const phrase of FORBIDDEN_PHONE_CLAIMS) {
          check(!lower.includes(phrase), `output claims "${phrase}"`);
        }
      }
      const humanPreview = await call(["note", "preview"]);
      check(
        humanPreview.stdout.includes("AnkiWeb and mobile synchronization are your own manual actions"),
        "human preview does not state the manual-sync boundary",
      );
    } catch (error) {
      checks.push(error instanceof Error ? error.message : String(error));
    } finally {
      anki.stop();
      rmSync(temp, { recursive: true, force: true });
    }

    if (checks.length > 0) return fail(checks);
    return pass(`${assertions} assertions`);
  },
};
