package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wxxb789/passquest/internal/anki"
	"github.com/wxxb789/passquest/internal/note"
)

// pushedNoteFixture accepts one note and pushes it through a fresh fake, so
// the fake holds the model and the note a learner already has.
func pushedNoteFixture(t *testing.T) (string, *stepClock, *fakeAnkiTransport) {
	t.Helper()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	readyNoteSource(t, db, clock)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")
	ankiTransport := newFakeAnki()
	mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")
	ankiTransport.resetActions()
	return db, clock, ankiTransport
}

func notePushOf(t *testing.T, raw string) notePushDoc {
	t.Helper()
	doc := notePushDoc{}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode push: %v\n%s", err, raw)
	}
	return doc
}

// TestNotePushUpgradesAKnownEarlierModelOnce: a deck whose model is the
// version 1 design is updated in place once, the note pushed under it is not
// a conflict afterwards, and the next push only reads the model.
func TestNotePushUpgradesAKnownEarlierModelOnce(t *testing.T) {
	t.Parallel()
	db, clock, ankiTransport := pushedNoteFixture(t)
	css, front, back, ok := note.KnownModel(1)
	if !ok {
		t.Fatal("version 1 is not a known model")
	}
	ankiTransport.styles[note.ModelName] = fakeAnkiModel{css: css,
		templates: map[string]anki.CardTemplate{anki.TemplateName: {Front: front, Back: back}}}

	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "closes on Sundays", "--json")
	upgraded := notePushOf(t, mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json"))
	if got := strings.Join(ankiTransport.actions, ","); got != "version,createDeck,modelNames,modelStyling,modelTemplates,"+
		"updateModelTemplates,updateModelStyling,notesInfo,updateNoteFields,cardsInfo" {
		t.Fatalf("upgrade actions = %s", got)
	}
	if upgraded.State != "pushed" || !upgraded.ModelUpdated || upgraded.ModelCustomized || upgraded.ModelCreated {
		t.Fatalf("upgrade push = %+v", upgraded)
	}
	model := ankiTransport.styles[note.ModelName]
	if model.css != note.ModelCSS || model.templates[anki.TemplateName] != (anki.CardTemplate{Front: note.FrontTemplate, Back: note.BackTemplate}) {
		t.Fatalf("the model was not replaced by this build's design: %+v", model)
	}

	ankiTransport.resetActions()
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "closes on Sunday", "--json")
	again := notePushOf(t, mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json"))
	if again.State != "pushed" || again.ModelUpdated || again.ModelCustomized ||
		ankiTransport.count("updateModelStyling") != 0 || ankiTransport.count("updateModelTemplates") != 0 {
		t.Fatalf("second push = %+v, actions = %v", again, ankiTransport.actions)
	}
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "closes on Sunday only", "--json")
	human := mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push")
	if strings.Contains(human, "Card design") {
		t.Fatalf("a push over the current design mentions it:\n%s", human)
	}
}

// TestNotePushUpgradesAHalfUpdatedModel: a deck whose stylesheet is already
// version 2 while its template is still version 1 (an interrupted update) is
// still Pass Quest's own design and is completed.
func TestNotePushUpgradesAHalfUpdatedModel(t *testing.T) {
	t.Parallel()
	db, clock, ankiTransport := pushedNoteFixture(t)
	_, front, back, _ := note.KnownModel(1)
	ankiTransport.styles[note.ModelName] = fakeAnkiModel{css: note.ModelCSS + "\r\n",
		templates: map[string]anki.CardTemplate{anki.TemplateName: {Front: front, Back: back}}}
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "closes on Sundays", "--json")
	pushed := notePushOf(t, mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json"))
	if !pushed.ModelUpdated || ankiTransport.styles[note.ModelName].templates[anki.TemplateName].Front != note.FrontTemplate {
		t.Fatalf("half-updated push = %+v, actions = %v", pushed, ankiTransport.actions)
	}
}

// TestNotePushNeverOverwritesACustomisedModel: a stylesheet or template the
// learner changed in Anki is reported, left alone, and the note still lands.
func TestNotePushNeverOverwritesACustomisedModel(t *testing.T) {
	t.Parallel()
	for name, custom := range map[string]fakeAnkiModel{
		"stylesheet": {css: note.ModelCSS + "\n.card { color: red; }",
			templates: map[string]anki.CardTemplate{anki.TemplateName: {Front: note.FrontTemplate, Back: note.BackTemplate}}},
		"template": {css: note.ModelCSS,
			templates: map[string]anki.CardTemplate{anki.TemplateName: {Front: "{{Front}} hint", Back: note.BackTemplate}}},
		"renamed template": {css: note.ModelCSS,
			templates: map[string]anki.CardTemplate{"Card 1": {Front: note.FrontTemplate, Back: note.BackTemplate}}},
		"second card type": {css: note.ModelCSS, templates: map[string]anki.CardTemplate{
			anki.TemplateName: {Front: note.FrontTemplate, Back: note.BackTemplate},
			"Reverse":         {Front: "{{Back}}", Back: "{{Front}}"},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			db, clock, ankiTransport := pushedNoteFixture(t)
			ankiTransport.styles[note.ModelName] = custom
			mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "closes on Sundays", "--json")
			pushed := notePushOf(t, mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json"))
			if pushed.State != "pushed" || !pushed.ModelCustomized || pushed.ModelUpdated ||
				ankiTransport.count("updateModelStyling") != 0 || ankiTransport.count("updateModelTemplates") != 0 ||
				ankiTransport.count("updateNoteFields") != 1 {
				t.Fatalf("customised push = %+v, actions = %v", pushed, ankiTransport.actions)
			}
			if got := ankiTransport.styles[note.ModelName]; got.css != custom.css || len(got.templates) != len(custom.templates) {
				t.Fatalf("the customised model changed: %+v", got)
			}
			if ankiTransport.notes[101]["Front"] != "closes on Sundays" {
				t.Fatalf("the note did not land: %v", ankiTransport.notes[101])
			}
			mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "closes on Sunday", "--json")
			human := mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push")
			if !strings.Contains(human, "Card design: the note type was changed in Anki") {
				t.Fatalf("human push does not state the customised model:\n%s", human)
			}
		})
	}
}

// TestNotePushModelMismatchIsAConflict pins the conflict a note gets when Anki
// reports it under another note type: nothing is written and the queue keeps
// the note until the model matches again.
func TestNotePushModelMismatchIsAConflict(t *testing.T) {
	t.Parallel()
	db, clock, ankiTransport := pushedNoteFixture(t)
	ankiTransport.noteModel = "Basic"
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "closes on Sundays", "--json")
	code, _, errOut := runNoteCLI(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")
	if code != 3 {
		t.Fatalf("mismatch push exit = %d, stderr = %s", code, errOut)
	}
	conflict := notePushOf(t, errOut)
	if conflict.State != "conflict" || len(conflict.Notes) != 1 || conflict.Notes[0].Code != "model_mismatch" ||
		conflict.Notes[0].Detail != "Anki note uses model Basic" || conflict.Counts.Pending != 1 ||
		conflict.Error == nil || conflict.Error.Code != "anki_conflict" {
		t.Fatalf("model mismatch = %+v", conflict)
	}
	if ankiTransport.count("updateNoteFields") != 0 || ankiTransport.notes[101]["Front"] == "closes on Sundays" {
		t.Fatalf("a model mismatch wrote the note: %v", ankiTransport.actions)
	}
	ankiTransport.noteModel = ""
	resolved := notePushOf(t, mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json"))
	if resolved.State != "pushed" || ankiTransport.notes[101]["Front"] != "closes on Sundays" {
		t.Fatalf("the push after the model matched again = %+v", resolved)
	}
}

// TestNotePushModelReadFailuresKeepTheQueue: an endpoint that cannot report
// the model's design is an Anki error before any write.
func TestNotePushModelReadFailuresKeepTheQueue(t *testing.T) {
	t.Parallel()
	db, clock, ankiTransport := pushedNoteFixture(t)
	delete(ankiTransport.styles, note.ModelName)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "closes on Sundays", "--json")
	code, _, errOut := runNoteCLI(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")
	if code != 3 || !strings.Contains(errOut, `"anki_error"`) || !strings.Contains(errOut, "modelStyling failed") {
		t.Fatalf("model read failure = %d, %s", code, errOut)
	}
	if ankiTransport.count("updateNoteFields") != 0 || ankiTransport.count("notesInfo") != 0 {
		t.Fatalf("a failed model read went on to write: %v", ankiTransport.actions)
	}
	ankiTransport.unreachable = true
	ankiTransport.resetActions()
	queued := notePushOf(t, mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json"))
	if queued.State != "queued" || queued.Counts.Pending != 1 {
		t.Fatalf("unreachable push = %+v", queued)
	}
}
