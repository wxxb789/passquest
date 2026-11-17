package cli

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/wxxb789/passquest/internal/anki"
	"github.com/wxxb789/passquest/internal/note"
)

type fakeAnkiTransport struct {
	requests []anki.HostRequest
	actions  []string
	models   []string
	// styles holds each model's stylesheet and card templates; noteModel,
	// when set, is the model name notesInfo reports for every note.
	styles       map[string]fakeAnkiModel
	noteModel    string
	notes        map[int64]map[string]string
	decks        map[int64]string
	cards        map[int64][]int64
	next         int64
	nextCard     int64
	deckOverride string
	duplicate    bool
	unreachable  bool
	// deleteAfterNotesInfo deletes the notes the next notesInfo answer reported,
	// once, right after it is sent: a card the learner removes between the
	// read-back and the write.
	deleteAfterNotesInfo bool
	// strayCards are rows a cardsInfo answer returns for cards this push never
	// asked about.
	strayCards   []map[string]any
	failAddAfter int
	adds         int
	calls        int
	failCall     int
	shortCall    int
	err          error
}

type fakeAnkiModel struct {
	css       string
	templates map[string]anki.CardTemplate
}

func newFakeAnki() *fakeAnkiTransport {
	return &fakeAnkiTransport{
		styles: map[string]fakeAnkiModel{},
		notes:  map[int64]map[string]string{}, decks: map[int64]string{},
		cards: map[int64][]int64{}, next: 101, nextCard: 501,
	}
}

// moveCard records the learner's manual move of a card.
func (f *fakeAnkiTransport) moveCard(id int64, deck string) {
	if _, ok := f.notes[id]; ok {
		f.decks[id] = deck
	}
}

func (f *fakeAnkiTransport) Call(_ context.Context, req anki.HostRequest) (anki.HostResponse, error) {
	f.requests = append(f.requests, req)
	f.calls++
	if f.err != nil {
		return anki.HostResponse{}, f.err
	}
	if f.failCall > 0 && f.calls == f.failCall {
		return anki.HostResponse{}, errors.New("anki host exited before answering")
	}
	resp := anki.HostResponse{ProtocolVersion: anki.ProtocolVersion, State: "ok"}
	for _, action := range req.Actions {
		f.actions = append(f.actions, action.Action)
		result := anki.ActionResult{Action: action.Action, State: "ok"}
		if f.unreachable {
			result.State = "unreachable"
			result.Error = "connection refused"
			resp.State = "unreachable"
			resp.Results = append(resp.Results, result)
			continue
		}
		switch action.Action {
		case "version":
			result.Result = json.RawMessage("6")
		case "createDeck":
			result.Result = json.RawMessage("1")
		case "modelNames":
			raw, _ := json.Marshal(f.models)
			result.Result = raw
		case "createModel":
			name := action.Params["modelName"].(string)
			f.models = append(f.models, name)
			model := fakeAnkiModel{css: action.Params["css"].(string), templates: map[string]anki.CardTemplate{}}
			for _, template := range action.Params["cardTemplates"].([]map[string]string) {
				model.templates[template["Name"]] = anki.CardTemplate{Front: template["Front"], Back: template["Back"]}
			}
			f.styles[name] = model
			result.Result = json.RawMessage("null")
		case "modelStyling", "modelTemplates":
			model, ok := f.styles[action.Params["modelName"].(string)]
			if !ok {
				result.State = "error"
				result.Error = "model was not found"
				break
			}
			if action.Action == "modelStyling" {
				result.Result, _ = json.Marshal(map[string]string{"css": model.css})
			} else {
				result.Result, _ = json.Marshal(model.templates)
			}
		case "updateModelStyling", "updateModelTemplates":
			payload := action.Params["model"].(map[string]any)
			name := payload["name"].(string)
			model, ok := f.styles[name]
			if !ok {
				result.State = "error"
				result.Error = "model was not found"
				break
			}
			if action.Action == "updateModelStyling" {
				model.css = payload["css"].(string)
			} else {
				for templateName, template := range payload["templates"].(map[string]map[string]string) {
					model.templates[templateName] = anki.CardTemplate{Front: template["Front"], Back: template["Back"]}
				}
			}
			f.styles[name] = model
			result.Result = json.RawMessage("null")
		case "addNote":
			f.adds++
			if f.failAddAfter > 0 && f.adds > f.failAddAfter {
				result.State = "unreachable"
				result.Error = "connection lost"
				resp.State = "unreachable"
				break
			}
			if f.duplicate {
				result.State = "error"
				result.Error = "cannot create note because it is a duplicate"
				break
			}
			payload := action.Params["note"].(map[string]any)
			fields := payload["fields"].(map[string]string)
			id := f.next
			f.next++
			f.notes[id] = copyFields(fields)
			f.decks[id] = payload["deckName"].(string)
			if f.deckOverride != "" {
				f.decks[id] = f.deckOverride
			}
			f.cards[id] = []int64{f.nextCard}
			f.nextCard++
			raw, _ := json.Marshal(id)
			result.Result = raw
		case "updateNoteFields":
			payload := action.Params["note"].(map[string]any)
			id := payload["id"].(int64)
			fields, ok := f.notes[id]
			if !ok {
				result.State = "error"
				result.Error = "note was not found"
				break
			}
			for name, value := range payload["fields"].(map[string]string) {
				fields[name] = value
			}
			result.Result = json.RawMessage("null")
		case "notesInfo":
			ids := action.Params["notes"].([]int64)
			var rows []map[string]any
			for _, id := range ids {
				fields, ok := f.notes[id]
				if !ok {
					continue
				}
				encoded := map[string]any{}
				for name, value := range fields {
					encoded[name] = map[string]any{"value": value, "order": 0}
				}
				modelName := note.ModelName
				if f.noteModel != "" {
					modelName = f.noteModel
				}
				rows = append(rows, map[string]any{
					"noteId": id, "modelName": modelName, "fields": encoded, "cards": f.cards[id],
				})
			}
			if f.deleteAfterNotesInfo {
				f.deleteAfterNotesInfo = false
				for _, id := range ids {
					delete(f.notes, id)
					delete(f.cards, id)
					delete(f.decks, id)
				}
			}
			raw, _ := json.Marshal(rows)
			result.Result = raw
		case "cardsInfo":
			cards := action.Params["cards"].([]int64)
			rows := []map[string]any{}
			for _, id := range cards {
				for noteID, ids := range f.cards {
					if !slices.Contains(ids, id) {
						continue
					}
					rows = append(rows, map[string]any{
						"cardId": id, "note": noteID, "deckName": f.decks[noteID],
						"modelName": note.ModelName,
					})
				}
			}
			rows = append(rows, f.strayCards...)
			raw, _ := json.Marshal(rows)
			result.Result = raw
		default:
			result.State = "error"
			result.Error = "unsupported action"
		}
		resp.Results = append(resp.Results, result)
	}
	if f.unreachable {
		resp.State = "unreachable"
	}
	if f.shortCall > 0 && f.calls == f.shortCall && len(resp.Results) > 0 {
		resp.Results = resp.Results[:len(resp.Results)-1]
	}
	return resp, nil
}

func copyFields(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for name, value := range in {
		out[name] = value
	}
	return out
}

func (f *fakeAnkiTransport) edit(id int64, field, value string) {
	if fields, ok := f.notes[id]; ok {
		fields[field] = value
	}
}

func (f *fakeAnkiTransport) count(action string) int {
	total := 0
	for _, got := range f.actions {
		if got == action {
			total++
		}
	}
	return total
}

func (f *fakeAnkiTransport) resetActions() {
	f.actions = nil
	f.requests = nil
	f.calls = 0
}
