/**
 * A loopback AnkiConnect stub covering the actions `agent/anki-host.mjs`
 * sends. It records every action and keeps notes, decks and cards in memory
 * so a gate can assert what reached "Anki". Shared by note-smoke (the CLI)
 * and desktop-smoke (the window).
 */
import { existsSync, readFileSync, writeFileSync } from "node:fs";

import { stringField } from "./json.ts";

export interface StubCardTemplate {
  Front: string;
  Back: string;
}

/** A note type's design as modelStyling and modelTemplates report it. */
export interface StubModelDesign {
  css: string;
  templates: Record<string, StubCardTemplate>;
}

export interface StubAction {
  action: string;
  params: Record<string, unknown>;
}

/** The stub's deck state as the state file carries it; arrays of pairs because JSON keys objects by string only. */
interface StubStateFile {
  nextNote: number;
  nextCard: number;
  notes: Array<[number, Record<string, string>]>;
  noteDecks: Array<[number, string]>;
  noteCards: Array<[number, number[]]>;
  models: string[];
  modelDesigns: Array<[string, StubModelDesign]>;
}

export interface AnkiStubOptions {
  /**
   * Persist the deck state here, and start from what the file already holds:
   * note ids, notes, decks, cards and the model design survive the stub's
   * lifetime, so two sequentially started stubs serve the same deck. The file
   * is written after every mutating action. Without the option the stub is
   * the in-memory one every other gate uses.
   */
  stateFile?: string;
}

export interface AnkiStub {
  endpoint: string;
  actions: StubAction[];
  notes: Map<number, Record<string, string>>;
  noteDecks: Map<number, string>;
  models: string[];
  /** Each model's stylesheet and card templates, keyed by model name. */
  modelDesigns: Map<string, StubModelDesign>;
  /** When set, notesInfo and cardsInfo report every note under this note type. */
  noteModelName: string | null;
  /** When set, addNote fails as an AnkiConnect duplicate. */
  duplicate: boolean;
  /** When set, added notes are filed in this deck instead of the requested one. */
  misassigned: string | null;
  /** Called after each action is answered. */
  afterAction: ((action: string) => void) | null;
  /** Closes the listener: later connections are refused (an unreachable endpoint). */
  stop: () => void;
}

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

/** Reads the state file; malformed content is an error, never a silently empty deck. */
function readStateFile(file: string): StubStateFile {
  const state = JSON.parse(readFileSync(file, "utf8")) as Partial<StubStateFile>;
  if (
    typeof state.nextNote !== "number" ||
    typeof state.nextCard !== "number" ||
    !Array.isArray(state.notes) ||
    !Array.isArray(state.noteDecks) ||
    !Array.isArray(state.noteCards) ||
    !Array.isArray(state.models) ||
    !Array.isArray(state.modelDesigns)
  ) {
    throw new Error(`${file} is not an Anki stub state file`);
  }
  return state as StubStateFile;
}

/** The actions that change the deck; the state file is written after each of them. */
const MUTATING_ACTIONS: Record<string, true> = {
  addNote: true,
  updateNoteFields: true,
  createModel: true,
  updateModelStyling: true,
  updateModelTemplates: true,
};

export function startAnkiStub(options: AnkiStubOptions = {}): AnkiStub {
  const stateFile = options.stateFile;
  const loaded = stateFile !== undefined && existsSync(stateFile) ? readStateFile(stateFile) : null;
  const noteCards = new Map<number, number[]>();
  let nextID = loaded?.nextNote ?? 101;
  let nextCard = loaded?.nextCard ?? 501;
  const notes = new Map(loaded?.notes ?? []);
  const noteDecks = new Map(loaded?.noteDecks ?? []);
  for (const [id, cards] of loaded?.noteCards ?? []) noteCards.set(id, cards);
  const models = loaded?.models ?? [];
  const modelDesigns = new Map(loaded?.modelDesigns ?? []);
  const save = (): void => {
    if (stateFile === undefined) return;
    const state: StubStateFile = {
      nextNote: nextID,
      nextCard,
      notes: [...notes],
      noteDecks: [...noteDecks],
      noteCards: [...noteCards],
      models,
      modelDesigns: [...modelDesigns],
    };
    writeFileSync(stateFile, JSON.stringify(state));
  };
  const stub: AnkiStub = {
    endpoint: "",
    actions: [],
    notes,
    noteDecks,
    models,
    modelDesigns,
    noteModelName: null,
    duplicate: false,
    misassigned: null,
    afterAction: null,
    stop: () => server.stop(true),
  };
  const answer = (action: string, params: Record<string, unknown>): { result: unknown; error: string | null } => {
    switch (action) {
      case "version":
        return { result: 6, error: null };
      case "createDeck":
        return { result: 1, error: null };
      case "modelNames":
        return { result: [...stub.models], error: null };
      case "createModel": {
        const name = stringField(params, "modelName") ?? "";
        stub.models.push(name);
        const templates: Record<string, StubCardTemplate> = {};
        for (const template of Array.isArray(params.cardTemplates) ? params.cardTemplates : []) {
          const record = asRecord(template);
          templates[stringField(record, "Name") ?? ""] = {
            Front: stringField(record, "Front") ?? "",
            Back: stringField(record, "Back") ?? "",
          };
        }
        stub.modelDesigns.set(name, { css: stringField(params, "css") ?? "", templates });
        return { result: null, error: null };
      }
      case "modelStyling":
      case "modelTemplates": {
        const name = stringField(params, "modelName") ?? "";
        const design = stub.modelDesigns.get(name);
        if (!design) return { result: null, error: `model was not found: ${name}` };
        return action === "modelStyling"
          ? { result: { css: design.css }, error: null }
          : { result: structuredClone(design.templates), error: null };
      }
      case "updateModelStyling":
      case "updateModelTemplates": {
        const model = asRecord(params.model);
        const name = stringField(model, "name") ?? "";
        const design = stub.modelDesigns.get(name);
        if (!design) return { result: null, error: `model was not found: ${name}` };
        if (action === "updateModelStyling") {
          design.css = stringField(model, "css") ?? "";
        } else {
          for (const [templateName, template] of Object.entries(asRecord(model.templates))) {
            const record = asRecord(template);
            design.templates[templateName] = {
              Front: stringField(record, "Front") ?? "",
              Back: stringField(record, "Back") ?? "",
            };
          }
        }
        return { result: null, error: null };
      }
      case "addNote": {
        if (stub.duplicate) return { result: null, error: "cannot create note because it is a duplicate" };
        const note = asRecord(params.note);
        const stored: Record<string, string> = {};
        for (const [name, value] of Object.entries(asRecord(note.fields))) stored[name] = String(value);
        const id = nextID++;
        stub.notes.set(id, stored);
        stub.noteDecks.set(id, stub.misassigned ?? stringField(note, "deckName") ?? "");
        noteCards.set(id, [nextCard++]);
        return { result: id, error: null };
      }
      case "updateNoteFields": {
        const note = asRecord(params.note);
        const fields = stub.notes.get(Number(note.id));
        if (!fields) return { result: null, error: "note was not found" };
        for (const [name, value] of Object.entries(asRecord(note.fields))) fields[name] = String(value);
        return { result: null, error: null };
      }
      case "notesInfo": {
        const ids = Array.isArray(params.notes) ? params.notes.map(Number) : [];
        const result = ids
          .filter((id) => stub.notes.has(id))
          .map((id) => ({
            noteId: id,
            modelName: stub.noteModelName ?? "PassQuest Note",
            fields: Object.fromEntries(Object.entries(stub.notes.get(id)!).map(([name, value]) => [name, { value, order: 0 }])),
            cards: noteCards.get(id) ?? [],
          }));
        return { result, error: null };
      }
      case "cardsInfo": {
        const asked = Array.isArray(params.cards) ? params.cards.map(Number) : [];
        const result = asked.flatMap((cardID) => {
          for (const [noteID, ids] of noteCards) {
            if (!ids.includes(cardID)) continue;
            return [
              {
                cardId: cardID,
                note: noteID,
                modelName: stub.noteModelName ?? "PassQuest Note",
                deckName: stub.noteDecks.get(noteID) ?? "",
              },
            ];
          }
          return [];
        });
        return { result, error: null };
      }
      default:
        return { result: null, error: "unsupported action" };
    }
  };
  const server = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    async fetch(request) {
      const body = asRecord(parse(await request.text()));
      const action = stringField(body, "action") ?? "";
      const params = asRecord(body.params);
      stub.actions.push({ action, params });
      const response = Response.json(answer(action, params));
      if (MUTATING_ACTIONS[action] === true) save();
      if (stub.afterAction) {
        const after = stub.afterAction;
        queueMicrotask(() => after(action));
      }
      return response;
    },
  });
  stub.endpoint = `http://127.0.0.1:${server.port}`;
  return stub;
}
