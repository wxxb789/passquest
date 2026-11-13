package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func goldenNoteDB(t *testing.T) (string, *stepClock, string) {
	t.Helper()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := readyNoteSource(t, db, clock)
	return db, clock, sourceID
}

func TestGoldenNoteDraftJSON(t *testing.T) {
	db, clock, _ := goldenNoteDB(t)
	out := mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	checkGolden(t, filepath.Join("testdata", "note-draft.golden.json"), []byte(out))
}

func TestGoldenNotePreviewJSON(t *testing.T) {
	db, clock, _ := goldenNoteDB(t)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	out := mustNoteRun(t, db, clock, nil, nil, "", "note", "preview", "--json")
	checkGolden(t, filepath.Join("testdata", "note-preview.golden.json"), []byte(out))
}

func TestGoldenNoteAcceptJSON(t *testing.T) {
	db, clock, _ := goldenNoteDB(t)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	out := mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")
	checkGolden(t, filepath.Join("testdata", "note-accept.golden.json"), []byte(out))
}

func TestGoldenNoteShowJSON(t *testing.T) {
	db, clock, _ := goldenNoteDB(t)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "closes on Sundays", "--json")
	out := mustNoteRun(t, db, clock, nil, nil, "", "note", "show", "--json")
	checkGolden(t, filepath.Join("testdata", "note-show.golden.json"), []byte(out))
}

func TestGoldenNotePushJSON(t *testing.T) {
	db, clock, _ := goldenNoteDB(t)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")
	ankiTransport := newFakeAnki()
	out := mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")
	checkGolden(t, filepath.Join("testdata", "note-push.golden.json"), []byte(out))
}

// TestGoldenNotePushDeckMismatchJSON pins the mismatch document: the endpoint
// accepts the write and files the card in another deck, which is what the
// owner's real Anki 26.x run produced.
func TestGoldenNotePushDeckMismatchJSON(t *testing.T) {
	db, clock, _ := goldenNoteDB(t)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")
	transport := newFakeAnki()
	transport.deckOverride = "Default"
	code, _, errOut := runNoteCLI(t, db, clock, transport, nil, "", "note", "push", "--json")
	if code != 3 {
		t.Fatalf("mismatch push exit = %d, stderr = %s", code, errOut)
	}
	checkGolden(t, filepath.Join("testdata", "note-push-deck-mismatch.golden.json"), []byte(errOut))
}

// TestGoldenNoteShowDeckMismatchJSON pins what the learner can inspect after a
// mismatch: the note keeps its Anki id, records the deck it reached and names
// the deck the push asked for.
func TestGoldenNoteShowDeckMismatchJSON(t *testing.T) {
	db, clock, _ := goldenNoteDB(t)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")
	transport := newFakeAnki()
	transport.deckOverride = "Default"
	if code, _, errOut := runNoteCLI(t, db, clock, transport, nil, "", "note", "push", "--json"); code != 3 {
		t.Fatalf("mismatch push exit = %d, stderr = %s", code, errOut)
	}
	out := mustNoteRun(t, db, clock, nil, nil, "", "note", "show", "--json")
	checkGolden(t, filepath.Join("testdata", "note-show-deck-mismatch.golden.json"), []byte(out))
}

// TestNodeGoldenDocumentsAreValidJSON keeps a broken golden from being read as
// a protocol change.
func TestNoteGoldenDocumentsAreValidJSON(t *testing.T) {
	for _, name := range []string{
		"note-draft.golden.json", "note-preview.golden.json", "note-accept.golden.json",
		"note-show.golden.json", "note-push.golden.json",
		"note-show-deck-mismatch.golden.json", "note-push-deck-mismatch.golden.json",
	} {
		raw, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatalf("%s is not JSON: %v", name, err)
		}
	}
}
