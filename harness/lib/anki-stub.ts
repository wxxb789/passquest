/**
 * A loopback AnkiConnect stub covering the actions `agent/anki-host.mjs`
 * sends. It records every action and keeps notes, decks and cards in memory
 * so a gate can assert what reached "Anki". Shared by note-smoke (the CLI)
 * and desktop-smoke (the window).
 */
import { stringField } from "./json.ts";

export interface StubAction {
  action: string;
  params: Record<string, unknown>;
}

export interface AnkiStub {
  endpoint: string;
  actions: StubAction[];
  notes: Map<number, Record<string, string>>;
  noteDecks: Map<number, string>;
  models: string[];
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

export function startAnkiStub(): AnkiStub {
  const noteCards = new Map<number, number[]>();
  let nextID = 101;
  let nextCard = 501;
  const stub: AnkiStub = {
    endpoint: "",
    actions: [],
    notes: new Map(),
    noteDecks: new Map(),
    models: [],
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
      case "createModel":
        stub.models.push(stringField(params, "modelName") ?? "");
        return { result: null, error: null };
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
            modelName: "PassQuest Note",
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
            return [{ cardId: cardID, note: noteID, modelName: "PassQuest Note", deckName: stub.noteDecks.get(noteID) ?? "" }];
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
