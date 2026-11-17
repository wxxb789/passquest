package cli

import (
	"database/sql"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/store"
)

func TestNotePushReaddsANoteDeletedFromAnki(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	readyNoteSource(t, db, clock)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	first := noteAcceptDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")), &first); err != nil {
		t.Fatal(err)
	}

	ankiTransport := newFakeAnki()
	initial := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &initial); err != nil {
		t.Fatal(err)
	}
	if initial.State != "pushed" || initial.Notes[0].AnkiNoteID != 101 {
		t.Fatalf("first push = %+v", initial)
	}

	delete(ankiTransport.notes, 101)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "closes on Sundays", "--json")
	ankiTransport.resetActions()
	readded := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &readded); err != nil {
		t.Fatal(err)
	}
	if readded.State != "pushed" || len(readded.Notes) != 1 {
		t.Fatalf("readd push = %+v", readded)
	}
	item := readded.Notes[0]
	if item.AnkiNoteID != 102 {
		t.Fatalf("readd item = %+v", item)
	}
	if got := strings.Join(ankiTransport.actions, ","); got != "version,createDeck,modelNames,modelStyling,modelTemplates,notesInfo,addNote,notesInfo,cardsInfo" {
		t.Fatalf("readd actions = %s", got)
	}
	if ankiTransport.count("updateNoteFields") != 0 {
		t.Fatalf("readd wrote an update: %v", ankiTransport.actions)
	}

	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	stored, err := st.Note(first.Note.NoteID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AnkiNoteID != 102 || stored.AnkiState != store.AnkiStatePushed || stored.AnkiPushedRevision != 2 {
		t.Fatalf("note after readd = %+v", stored)
	}

	handle, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	rows, err := handle.Query(`SELECT action, outcome FROM anki_push_attempts
		WHERE note_id = ? ORDER BY id`, first.Note.NoteID)
	if err != nil {
		t.Fatalf("read attempt ledger: %v", err)
	}
	defer rows.Close()
	var ledger []string
	for rows.Next() {
		var action, outcome string
		if err := rows.Scan(&action, &outcome); err != nil {
			t.Fatal(err)
		}
		ledger = append(ledger, action+"/"+outcome)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ledger, ","); got != "add/ok,check/ok,check/not_found,readd/ok,check/ok" {
		t.Fatalf("ledger = %s, want add/ok,check/ok,check/not_found,readd/ok,check/ok", got)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}

	// --force skips the read-back; the failed update records update/not_found.
	delete(ankiTransport.notes, 102)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "closes on Sunday", "--json")
	ankiTransport.resetActions()
	forcedReadd := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--force", "--json")), &forcedReadd); err != nil {
		t.Fatal(err)
	}
	if forcedReadd.State != "pushed" || forcedReadd.Notes[0].AnkiNoteID != 103 {
		t.Fatalf("forced readd = %+v, actions = %v", forcedReadd, ankiTransport.actions)
	}
	rows, err = handle.Query(`SELECT action, outcome FROM anki_push_attempts
		WHERE note_id = ? ORDER BY id`, first.Note.NoteID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	ledger = ledger[:0]
	for rows.Next() {
		var action, outcome string
		if err := rows.Scan(&action, &outcome); err != nil {
			t.Fatal(err)
		}
		ledger = append(ledger, action+"/"+outcome)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ledger, ","); got != "add/ok,check/ok,check/not_found,readd/ok,check/ok,update/not_found,readd/ok,check/ok" {
		t.Fatalf("forced readd ledger = %s", got)
	}
}

// A card deleted between the pre-write read-back and the update: the readd
// writes a new note with a new id and a new card, and the deck claim is read
// from that card — never from the deleted note's, which would report a mismatch
// for a card that is exactly where the push put it.
func TestNotePushReaddsANoteDeletedAfterTheReadBack(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	readyNoteSource(t, db, clock)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	accepted := noteAcceptDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")), &accepted); err != nil {
		t.Fatal(err)
	}

	ankiTransport := newFakeAnki()
	initial := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &initial); err != nil {
		t.Fatal(err)
	}
	if initial.State != "pushed" || initial.Notes[0].AnkiNoteID != 101 || initial.Notes[0].Deck != "" {
		t.Fatalf("first push = %+v", initial)
	}

	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "closes on Sundays", "--json")
	ankiTransport.resetActions()
	ankiTransport.deleteAfterNotesInfo = true
	code, out, errOut := runNoteCLI(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")
	if code != 0 {
		t.Fatalf("readd after a late delete exit = %d, stdout = %s, stderr = %s", code, out, errOut)
	}
	readded := notePushDoc{}
	if err := json.Unmarshal([]byte(out), &readded); err != nil {
		t.Fatal(err)
	}
	if readded.State != "pushed" || readded.Counts.Pushed != 1 || readded.Counts.DeckMismatch != 0 {
		t.Fatalf("readd after a late delete = %+v", readded)
	}
	item := readded.Notes[0]
	if item.AnkiNoteID != 102 || item.State != store.AnkiStatePushed || item.Outcome != "ok" || item.Deck != "" {
		t.Fatalf("readd item = %+v", item)
	}
	if got := strings.Join(ankiTransport.actions, ","); got != "version,createDeck,modelNames,modelStyling,modelTemplates,notesInfo,updateNoteFields,addNote,notesInfo,cardsInfo" {
		t.Fatalf("readd actions = %s", got)
	}
	// The read is about the new note and its own card: the deleted note's id and
	// card are never asked about again.
	var askedNotes, askedCards []int64
	for _, request := range ankiTransport.requests {
		for _, action := range request.Actions {
			switch action.Action {
			case "notesInfo":
				askedNotes = append(askedNotes, action.Params["notes"].([]int64)...)
			case "cardsInfo":
				askedCards = append(askedCards, action.Params["cards"].([]int64)...)
			}
		}
	}
	if !slices.Contains(askedNotes, int64(102)) || !slices.Equal(askedCards, ankiTransport.cards[102]) {
		t.Fatalf("read-back asked notes %v, cards %v; the new note's card is %v", askedNotes, askedCards, ankiTransport.cards[102])
	}

	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	stored, err := st.Note(accepted.Note.NoteID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AnkiNoteID != 102 || stored.AnkiState != store.AnkiStatePushed ||
		stored.AnkiDeck != note.Deck || stored.AnkiPushedRevision != 2 {
		t.Fatalf("note after the late delete = %+v", stored)
	}
}

func TestNotePushReportsADeckMismatchAndKeepsTheNoteID(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	readyNoteSource(t, db, clock)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	accepted := noteAcceptDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")), &accepted); err != nil {
		t.Fatal(err)
	}
	reportBefore := mustRun(t, db, &stepClock{base: goldenBase}, "", "report", "--json")
	nextBefore := mustRun(t, db, &stepClock{base: goldenBase}, "", "next", "--json")

	// The endpoint accepts the write and files the card in another deck.
	ankiTransport := newFakeAnki()
	ankiTransport.deckOverride = "Default"
	// A row about a card this push never asked about cannot become this note's
	// deck claim.
	ankiTransport.strayCards = []map[string]any{{"cardId": 9001, "note": 101, "deckName": "Foreign"}}
	code, out, errOut := runNoteCLI(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")
	if code != 3 {
		t.Fatalf("mismatch push exit = %d, stdout = %s, stderr = %s", code, out, errOut)
	}
	mismatch := notePushDoc{}
	if err := json.Unmarshal([]byte(errOut), &mismatch); err != nil {
		t.Fatalf("decode mismatch document: %v\n%s", err, errOut)
	}
	if mismatch.State != "deck_mismatch" || mismatch.Counts.DeckMismatch != 1 ||
		mismatch.Counts.Pushed != 0 || mismatch.Counts.Pending != 0 {
		t.Fatalf("mismatch document = %+v", mismatch)
	}
	if mismatch.Error == nil || mismatch.Error.Code != "anki_deck_mismatch" || mismatch.Error.Message == "" {
		t.Fatalf("mismatch error envelope = %+v", mismatch.Error)
	}
	item := mismatch.Notes[0]
	if item.State != store.AnkiStateDeckMismatch || item.Outcome != "deck_mismatch" || item.Deck != "Default" ||
		item.AnkiNoteID != 101 || !strings.Contains(item.Detail, `"Default"`) {
		t.Fatalf("mismatch item = %+v", item)
	}
	if strings.Contains(item.Detail, "Foreign") {
		t.Fatalf("a card row this push never asked about became the note's deck claim: %+v", item)
	}
	if ankiTransport.count("addNote") != 1 || ankiTransport.count("cardsInfo") != 1 {
		t.Fatalf("mismatch actions = %v", ankiTransport.actions)
	}

	// The recorded note keeps its Anki id and pushed revision, so the next
	// revision is an update of the same card and a repeat push writes nothing.
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := st.Note(accepted.Note.NoteID)
	if err != nil {
		st.Close()
		t.Fatal(err)
	}
	if stored.AnkiState != store.AnkiStateDeckMismatch || stored.AnkiDeck != "Default" ||
		stored.AnkiNoteID != 101 || stored.AnkiPushedRevision != 1 {
		st.Close()
		t.Fatalf("note after the mismatch = %+v", stored)
	}
	if pending, err := st.PendingNoteOutbox(); err != nil || len(pending) != 0 {
		st.Close()
		t.Fatalf("mismatch left %+v pending (%v); a repeat push would write again", pending, err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	show := noteShowDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "show", accepted.Note.NoteID, "--json")), &show); err != nil {
		t.Fatal(err)
	}
	if len(show.Notes) != 1 {
		t.Fatalf("note show found %d notes", len(show.Notes))
	}
	shown := show.Notes[0].Note.Anki
	if shown.State != store.AnkiStateDeckMismatch || shown.Deck != "Default" ||
		shown.DeckExpected != note.Deck || shown.NoteID != 101 {
		t.Fatalf("note show anki = %+v", shown)
	}

	// A repeat push over the mismatched note contacts nothing and adds no card.
	ankiTransport.resetActions()
	repeat := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &repeat); err != nil {
		t.Fatal(err)
	}
	if repeat.State != "idle" || len(repeat.Notes) != 0 {
		t.Fatalf("repeat push = %+v", repeat)
	}
	if len(ankiTransport.requests) != 0 {
		t.Fatalf("repeat push contacted the endpoint %d time(s)", len(ankiTransport.requests))
	}
	if len(ankiTransport.notes) != 1 || ankiTransport.count("addNote") != 0 {
		t.Fatalf("repeat push wrote: notes = %d, actions = %v", len(ankiTransport.notes), ankiTransport.actions)
	}

	// The next revision updates the same card, still reports where it is, and
	// states one action in the human summary.
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "closes at 4 p.m.", "--json")
	ankiTransport.resetActions()
	code, out, errOut = runNoteCLI(t, db, clock, ankiTransport, nil, "", "note", "push")
	if code != 3 {
		t.Fatalf("mismatched revision exit = %d, stdout = %s, stderr = %s", code, out, errOut)
	}
	for _, want := range []string{
		"Deck mismatch: 1 note(s) are not in \"Pass Quest\"",
		"landed in \"Default\" (anki note 101)",
		"Move the card into \"Pass Quest\" in Anki",
		"anki_deck_mismatch",
	} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("human mismatch output is missing %q:\n%s", want, errOut)
		}
	}
	if strings.Contains(errOut, "pushed 1") {
		t.Fatalf("human mismatch output claims a push:\n%s", errOut)
	}
	if ankiTransport.count("addNote") != 0 || ankiTransport.count("updateNoteFields") != 1 ||
		ankiTransport.count("cardsInfo") != 1 {
		t.Fatalf("mismatched revision actions = %v", ankiTransport.actions)
	}
	if ankiTransport.notes[101]["Front"] != "closes at 4 p.m." {
		t.Fatalf("the revision did not reach the same card: %+v", ankiTransport.notes[101])
	}
	if len(ankiTransport.notes) != 1 {
		t.Fatalf("the mismatched note created a second card: %d notes", len(ankiTransport.notes))
	}

	// The learner moves the card by hand; the next revision confirms the deck
	// instead of claiming it.
	ankiTransport.moveCard(101, note.Deck)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "closes at 4 p.m. only", "--json")
	ankiTransport.resetActions()
	recovered := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &recovered); err != nil {
		t.Fatal(err)
	}
	if recovered.State != "pushed" || recovered.Counts.DeckMismatch != 0 ||
		recovered.Counts.Pushed != 1 || recovered.Notes[0].AnkiNoteID != 101 || recovered.Notes[0].Deck != "" {
		t.Fatalf("recovered push = %+v", recovered)
	}
	st, err = store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	stored, err = st.Note(accepted.Note.NoteID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AnkiState != store.AnkiStatePushed || stored.AnkiDeck != note.Deck {
		t.Fatalf("note after the manual move = %+v", stored)
	}

	// Nothing else moved.
	if again := mustRun(t, db, &stepClock{base: goldenBase}, "", "report", "--json"); again != reportBefore {
		t.Fatalf("the mismatched push changed the attempt report:\n--- before ---\n%s\n--- after ---\n%s", reportBefore, again)
	}
	if again := mustRun(t, db, &stepClock{base: goldenBase}, "", "next", "--json"); again != nextBefore {
		t.Fatalf("the mismatched push changed next:\n--- before ---\n%s\n--- after ---\n%s", nextBefore, again)
	}
}

// A deck read that cannot name a deck leaves the note claiming none, so
// `note show` cannot contradict the deck_mismatch state with a stale name.
//
// The state the check has to correct is written through the store, because the
// CLI cannot produce it: a completed read drains the outbox, only a new accepted
// revision re-pends the note, and accepting clears the recorded deck. The CLI
// half here is the read-back path and what the two `note show` surfaces print.
func TestNotePushRecordsNoDeckWhenTheReadCannotNameIt(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	readyNoteSource(t, db, clock)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	accepted := noteAcceptDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")), &accepted); err != nil {
		t.Fatal(err)
	}

	ankiTransport := newFakeAnki()
	first := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &first); err != nil {
		t.Fatal(err)
	}
	if first.State != "pushed" || first.Notes[0].Deck != "" {
		t.Fatalf("first push = %+v", first)
	}

	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "closes on Sundays", "--json")
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.RecordAnkiAttempt(store.AnkiAttempt{
		NoteID: accepted.Note.NoteID, Revision: 2, Action: "check", Outcome: "deck_mismatch",
		Detail: `card landed in "Default", not "Pass Quest"`, At: clock.Now(),
		AnkiDeck: "Default", SetDeck: true, AnkiState: store.AnkiStateDeckMismatch, Keep: true,
	}); err != nil {
		t.Fatalf("record the name an earlier read left behind: %v", err)
	}

	// The endpoint answers cardsInfo for the card but names no deck.
	delete(ankiTransport.decks, 101)
	ankiTransport.resetActions()
	code, out, errOut := runNoteCLI(t, db, clock, ankiTransport, nil, "", "note", "push")
	if code != 3 {
		t.Fatalf("unconfirmed push exit = %d, stdout = %s, stderr = %s", code, out, errOut)
	}
	for _, want := range []string{
		"Deck mismatch: 1 note(s) are not in \"Pass Quest\".",
		"reached no reported deck (anki note 101)",
		`passquest: anki_deck_mismatch: 1 note(s) landed in a deck other than "Pass Quest"`,
	} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("human unconfirmed output is missing %q:\n%s", want, errOut)
		}
	}
	if strings.Contains(errOut, `"Default"`) {
		t.Fatalf("human unconfirmed output claims the stale deck:\n%s", errOut)
	}
	if ankiTransport.count("cardsInfo") != 1 || ankiTransport.count("updateNoteFields") != 1 {
		t.Fatalf("unconfirmed actions = %v", ankiTransport.actions)
	}

	// `note show` records the state without a deck and without an expectation.
	shown := mustNoteRun(t, db, clock, nil, nil, "", "note", "show", accepted.Note.NoteID)
	if !strings.Contains(shown, "anki=deck_mismatch") {
		t.Fatalf("note show lost the state:\n%s", shown)
	}
	if strings.Contains(shown, "deck=") || strings.Contains(shown, "expected") {
		t.Fatalf("note show claims a deck after an unconfirmed read:\n%s", shown)
	}
	doc := noteShowDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "show", accepted.Note.NoteID, "--json")), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Notes) != 1 {
		t.Fatalf("note show found %d notes", len(doc.Notes))
	}
	anki := doc.Notes[0].Note.Anki
	if anki.State != store.AnkiStateDeckMismatch || anki.Deck != "" || anki.DeckExpected != "" ||
		anki.NoteID != 101 || anki.PushedRevision != 2 {
		t.Fatalf("note show anki = %+v", anki)
	}
	stored, err := st.Note(accepted.Note.NoteID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AnkiState != store.AnkiStateDeckMismatch || stored.AnkiDeck != "" ||
		stored.AnkiNoteID != 101 || stored.AnkiPushedRevision != 2 {
		t.Fatalf("note after the unconfirmed read = %+v", stored)
	}
}
