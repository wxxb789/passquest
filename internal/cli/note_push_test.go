package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/wxxb789/passquest/internal/anki"
	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/store"
)

func TestNotePushCreatesUpdatesConflictsAndQueues(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := readyNoteSource(t, db, clock)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	first := noteAcceptDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")), &first); err != nil {
		t.Fatal(err)
	}

	ankiTransport := newFakeAnki()
	push := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &push); err != nil {
		t.Fatal(err)
	}
	if push.State != "pushed" || !push.ModelCreated || push.Counts.Pushed != 1 || push.Notes[0].AnkiNoteID != 101 {
		t.Fatalf("first push = %+v", push)
	}
	if push.Counts.Pending != 0 {
		t.Fatalf("first push reports %d still pending; the outbox was drained", push.Counts.Pending)
	}
	for _, action := range []string{"version", "createDeck", "modelNames", "createModel", "addNote"} {
		if ankiTransport.count(action) != 1 {
			t.Fatalf("%s count = %d, want 1", action, ankiTransport.count(action))
		}
	}
	if ankiTransport.count("notesInfo") != 1 || ankiTransport.count("cardsInfo") != 1 {
		t.Fatalf("first push read-back = notesInfo %d, cardsInfo %d, want 1 each",
			ankiTransport.count("notesInfo"), ankiTransport.count("cardsInfo"))
	}
	if ankiTransport.count("updateNoteFields") != 0 {
		t.Fatalf("first push wrote an update: %v", ankiTransport.actions)
	}
	if push.Notes[0].Deck != "" || push.Counts.DeckMismatch != 0 {
		t.Fatalf("matching deck was flagged: item = %+v, counts = %+v", push.Notes[0], push.Counts)
	}

	ankiTransport.resetActions()
	idle := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &idle); err != nil {
		t.Fatal(err)
	}
	if idle.State != "idle" || len(ankiTransport.requests) != 0 {
		t.Fatalf("unchanged push = %+v, requests = %d", idle, len(ankiTransport.requests))
	}

	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "closes on Sundays", "--json")
	ankiTransport.resetActions()
	updated := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.State != "pushed" || ankiTransport.count("addNote") != 0 || ankiTransport.count("updateNoteFields") != 1 || ankiTransport.count("notesInfo") != 1 {
		t.Fatalf("revision push = %+v, actions = %v", updated, ankiTransport.actions)
	}
	if ankiTransport.notes[101]["Front"] != "closes on Sundays" {
		t.Fatalf("update did not reach the same note: %+v", ankiTransport.notes[101])
	}

	ankiTransport.edit(101, "Back", "a learner edit")
	ankiTransport.resetActions()
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "closes on Sunday", "--json")
	conflict := notePushDoc{}
	code, out, errOut := runNoteCLI(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")
	if code != 3 {
		t.Fatalf("conflict exit = %d, out=%s err=%s", code, out, errOut)
	}
	if err := json.Unmarshal([]byte(errOut), &conflict); err != nil {
		t.Fatalf("decode conflict: %v\n%s", err, errOut)
	}
	if conflict.State != "conflict" || conflict.Counts.Conflicts != 1 || ankiTransport.count("updateNoteFields") != 0 {
		t.Fatalf("conflict push = %+v, actions = %v", conflict, ankiTransport.actions)
	}
	if conflict.Error == nil || conflict.Error.Code != "anki_conflict" {
		t.Fatalf("conflict stderr has no error envelope: %+v", conflict)
	}
	if conflict.Counts.Pending != 1 {
		t.Fatalf("conflict push reports pending = %d, want the still-queued revision", conflict.Counts.Pending)
	}
	if pending := notePending(t, db); len(pending) != 1 || pending[0].Revision != 3 {
		t.Fatalf("conflict was not kept pending: %+v", pending)
	}

	ankiTransport.resetActions()
	forced := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--force", "--json")), &forced); err != nil {
		t.Fatal(err)
	}
	if forced.State != "pushed" || ankiTransport.count("updateNoteFields") != 1 || forced.Notes[0].AnkiNoteID != 101 {
		t.Fatalf("forced push = %+v, actions = %v", forced, ankiTransport.actions)
	}

	// A duplicate add is reported as already present, not as a failure.
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose",
		"--content", sourceID+"-p001", "--question", "q2", "--mark", "Sundays", "--json")
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")
	ankiTransport.duplicate = true
	ankiTransport.resetActions()
	duplicate := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &duplicate); err != nil {
		t.Fatal(err)
	}
	if duplicate.State != "already_present" || duplicate.Counts.AlreadyPresent != 1 || duplicate.Counts.Pushed != 0 {
		t.Fatalf("duplicate push = %+v", duplicate)
	}
	if ankiTransport.count("addNote") != 1 || ankiTransport.count("updateNoteFields") != 0 {
		t.Fatalf("duplicate actions = %v", ankiTransport.actions)
	}

	// A later revision of a duplicate-only note is a conflict, not a second card.
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "Sundays v2", "--json")
	ankiTransport.resetActions()
	code, _, errOut = runNoteCLI(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")
	if code != 3 || !strings.Contains(errOut, "already holds a note") {
		t.Fatalf("duplicate revision = %d, %s", code, errOut)
	}
	// --force cannot update a duplicate whose note id AnkiConnect does not report.
	ankiTransport.resetActions()
	code, _, errOut = runNoteCLI(t, db, clock, ankiTransport, nil, "", "note", "push", "--force", "--json")
	if code != 3 || !strings.Contains(errOut, "note_id_unknown") || !strings.Contains(errOut, "remove that card") {
		t.Fatalf("forced duplicate revision = %d, %s", code, errOut)
	}
	forcedDuplicate := notePushDoc{}
	if err := json.Unmarshal([]byte(errOut), &forcedDuplicate); err != nil {
		t.Fatal(err)
	}
	if forcedDuplicate.Error == nil || forcedDuplicate.Error.Code != "anki_conflict" {
		t.Fatalf("forced duplicate has no error envelope: %+v", forcedDuplicate)
	}
	if forcedDuplicate.Counts.Conflicts != 1 || forcedDuplicate.Counts.Pending != 1 ||
		ankiTransport.count("updateNoteFields") != 0 {
		t.Fatalf("forced duplicate push = %+v, actions = %v", forcedDuplicate, ankiTransport.actions)
	}
	ankiTransport.duplicate = false

	// No endpoint: queued, then drained exactly once by the next push.
	ankiTransport.unreachable = true
	ankiTransport.resetActions()
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "queued wording", "--json")
	queued := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &queued); err != nil {
		t.Fatal(err)
	}
	if queued.State != "queued" || queued.Counts.Queued == 0 || queued.Counts.Pushed != 0 {
		t.Fatalf("queued push = %+v", queued)
	}
	if pending := notePending(t, db); len(pending) == 0 {
		t.Fatal("unreachable push drained the queue")
	}
	ankiTransport.unreachable = false
	ankiTransport.resetActions()
	drained := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &drained); err != nil {
		t.Fatal(err)
	}
	if drained.State != "pushed" || drained.Counts.Pushed != 1 {
		t.Fatalf("drained push = %+v, actions = %v", drained, ankiTransport.actions)
	}
	ankiTransport.resetActions()
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &idle); err != nil {
		t.Fatal(err)
	}
	if idle.State != "idle" || ankiTransport.count("addNote") != 0 || ankiTransport.count("updateNoteFields") != 0 {
		t.Fatalf("second drain = %+v, actions = %v", idle, ankiTransport.actions)
	}
}

func TestNotePushChunksMoreThanFiftyActions(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 51 {
		targetKey := fmt.Sprintf("bulk\x1fq\x1fmark-%02d", i)
		target := store.NoteTarget{
			NoteID: note.NoteID(targetKey), TargetKey: targetKey,
			ContentID: "bulk", ContentVersion: "v1", QuestionID: "q",
			Page: 1, Mark: fmt.Sprintf("mark-%02d", i),
		}
		content := store.NoteContent{
			Front: fmt.Sprintf("front %02d", i), Back: "meaning", Tactic: "",
			SourceLabel: "sample · bulk · q", ContentHash: strings.Repeat("a", 64),
			Origin: "deterministic",
		}
		if _, err := st.AcceptNote(target, content, clock.Now()); err != nil {
			t.Fatalf("accept note %d: %v", i, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	ankiTransport := newFakeAnki()
	push := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &push); err != nil {
		t.Fatal(err)
	}
	if push.State != "pushed" || push.Counts.Pushed != 51 || push.Counts.Pending != 0 {
		t.Fatalf("bulk push = %+v", push)
	}
	if ankiTransport.count("addNote") != 51 {
		t.Fatalf("addNote count = %d, want 51", ankiTransport.count("addNote"))
	}
	for i, request := range ankiTransport.requests {
		if len(request.Actions) > anki.MaxActionsPerCall {
			t.Fatalf("request %d carried %d actions, over the %d limit", i, len(request.Actions), anki.MaxActionsPerCall)
		}
	}
	if pending := notePending(t, db); len(pending) != 0 {
		t.Fatalf("bulk push left %d pending", len(pending))
	}
}

func seedBulkNotes(t *testing.T, db string, clock *stepClock, count int) []store.NoteTarget {
	t.Helper()
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	targets := make([]store.NoteTarget, 0, count)
	for i := range count {
		targetKey := fmt.Sprintf("bulk\x1fq\x1fmark-%02d", i)
		target := store.NoteTarget{
			NoteID: note.NoteID(targetKey), TargetKey: targetKey,
			ContentID: "bulk", ContentVersion: "v1", QuestionID: "q",
			Page: 1, Mark: fmt.Sprintf("mark-%02d", i),
		}
		content := store.NoteContent{
			Front: fmt.Sprintf("front %02d", i), Back: "meaning",
			SourceLabel: "sample · bulk · q", ContentHash: strings.Repeat("a", 64),
			Origin: "deterministic",
		}
		if _, err := st.AcceptNote(target, content, clock.Now()); err != nil {
			st.Close()
			t.Fatalf("accept bulk note %d: %v", i, err)
		}
		targets = append(targets, target)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	return targets
}

func reviseBulkNotes(t *testing.T, db string, clock *stepClock, targets []store.NoteTarget) {
	t.Helper()
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for i, target := range targets {
		content := store.NoteContent{
			Front: fmt.Sprintf("revised %02d", i), Back: "revised meaning",
			SourceLabel: "sample · bulk · q", ContentHash: strings.Repeat("b", 64),
			Origin: "deterministic",
		}
		if _, err := st.AcceptNote(target, content, clock.Now()); err != nil {
			t.Fatalf("revise bulk note %d: %v", i, err)
		}
	}
}

func TestNotePushBatchesReadbackAboveFiftyNotes(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	targets := seedBulkNotes(t, db, clock, 51)

	ankiTransport := newFakeAnki()
	first := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &first); err != nil {
		t.Fatal(err)
	}
	if first.State != "pushed" || first.Counts.Pushed != 51 {
		t.Fatalf("initial bulk push = %+v", first)
	}
	reviseBulkNotes(t, db, clock, targets)

	ankiTransport.resetActions()
	updated := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.State != "pushed" || updated.Counts.Pushed != 51 || updated.Counts.Pending != 0 {
		t.Fatalf("readback bulk push = %+v", updated)
	}
	var readbackIDs []int
	for _, request := range ankiTransport.requests {
		for _, action := range request.Actions {
			if action.Action != "notesInfo" {
				continue
			}
			ids, ok := action.Params["notes"].([]int64)
			if !ok {
				t.Fatalf("notesInfo params = %#v", action.Params)
			}
			if len(ids) > anki.MaxActionsPerCall {
				t.Fatalf("notesInfo carried %d ids, over the %d bound", len(ids), anki.MaxActionsPerCall)
			}
			readbackIDs = append(readbackIDs, len(ids))
		}
	}
	if len(readbackIDs) != 2 || readbackIDs[0] != 50 || readbackIDs[1] != 1 {
		t.Fatalf("notesInfo batch sizes = %v, want [50 1]", readbackIDs)
	}
	for _, fields := range ankiTransport.notes {
		if !strings.HasPrefix(fields["Front"], "revised ") {
			t.Fatalf("note was not updated: %+v", fields)
		}
	}
	if pending := notePending(t, db); len(pending) != 0 {
		t.Fatalf("readback bulk push left %d pending", len(pending))
	}
}

func TestNotePushPersistsAcknowledgedBatchesBeforeTheNextCall(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	seedBulkNotes(t, db, clock, 51)

	ankiTransport := newFakeAnki()
	// Call 1 is the capability pass, 2 creates the model, 3 adds the first 50
	// notes; call 4 must fail before the last note is sent.
	ankiTransport.failCall = 4
	code, _, errOut := runNoteCLI(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")
	if code != 3 {
		t.Fatalf("failed batch exit = %d, stderr = %s", code, errOut)
	}
	partial := notePushDoc{}
	if err := json.Unmarshal([]byte(errOut), &partial); err != nil {
		t.Fatalf("decode failed batch: %v\n%s", err, errOut)
	}
	if partial.State != "partial" || partial.Counts.Pushed != 50 ||
		partial.Counts.Queued != 1 || partial.Counts.Pending != 1 {
		t.Fatalf("failed batch document = %+v", partial)
	}
	if partial.Error == nil || partial.Error.Code != "anki_unavailable" {
		t.Fatalf("failed batch error = %+v", partial.Error)
	}
	if ankiTransport.count("addNote") != 50 {
		t.Fatalf("addNote count before the failed call = %d, want 50", ankiTransport.count("addNote"))
	}

	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := st.PendingNoteOutbox()
	if err != nil {
		st.Close()
		t.Fatal(err)
	}
	if len(pending) != 1 {
		st.Close()
		t.Fatalf("pending after the failed batch = %+v", pending)
	}
	notes, err := st.Notes()
	if err != nil {
		st.Close()
		t.Fatal(err)
	}
	pushed := 0
	for _, n := range notes {
		if n.AnkiState == store.AnkiStatePushed && n.AnkiNoteID > 0 {
			pushed++
		}
	}
	if pushed != 50 {
		st.Close()
		t.Fatalf("persisted pushes after the failed batch = %d, want 50", pushed)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	ankiTransport.failCall = 0
	ankiTransport.resetActions()
	retry := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &retry); err != nil {
		t.Fatal(err)
	}
	if retry.State != "pushed" || retry.Counts.Pushed != 1 || retry.Counts.Pending != 0 {
		t.Fatalf("retry = %+v", retry)
	}
	if ankiTransport.count("addNote") != 1 {
		t.Fatalf("retry re-added acknowledged notes: addNote = %d", ankiTransport.count("addNote"))
	}
	if len(ankiTransport.notes) != 51 {
		t.Fatalf("fake Anki holds %d notes, want 51", len(ankiTransport.notes))
	}
}

func TestNotePushRefusesRevokedConsentAndMissingHost(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := readyNoteSource(t, db, clock)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "pending revision", "--json")
	mustRun(t, db, clock, "", "tutor", "revoke", sourceID, "--reviewer", "note-reviewer", "--json")

	ankiTransport := newFakeAnki()
	code, _, errOut := runNoteCLI(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")
	if code != 2 || !strings.Contains(errOut, "consent_required") || len(ankiTransport.requests) != 0 {
		t.Fatalf("revoked push = %d, %s, requests %d", code, errOut, len(ankiTransport.requests))
	}
	noteConsent(t, db, clock, sourceID, "note-reviewer")

	missingHost := &fakeAnkiTransport{err: errors.New("node not found")}
	code, _, errOut = runNoteCLI(t, db, clock, missingHost, nil, "", "note", "push", "--json")
	if code != 3 || !strings.Contains(errOut, "anki_unavailable") {
		t.Fatalf("missing host = %d, %s", code, errOut)
	}
	if pending := notePending(t, db); len(pending) != 1 {
		t.Fatalf("missing host changed the queue: %+v", pending)
	}
}

func TestNotePushEndpointMustBeLoopback(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	readyNoteSource(t, db, clock)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")
	code, _, errOut := runNoteCLI(t, db, clock, newFakeAnki(), nil, "", "note", "push", "--endpoint", "http://10.0.0.2:8765", "--json")
	if code != 2 || !strings.Contains(errOut, "not loopback") {
		t.Fatalf("remote endpoint = %d, %s", code, errOut)
	}
}

func TestNotePushKeepsAnAcknowledgedWriteInAPartialBatch(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := readyNoteSource(t, db, clock)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose",
		"--content", sourceID+"-p001", "--question", "q2", "--mark", "Sundays", "--json")
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")

	ankiTransport := newFakeAnki()
	ankiTransport.failAddAfter = 1
	partial := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &partial); err != nil {
		t.Fatal(err)
	}
	if partial.State != "partial" || partial.Counts.Pushed != 1 || partial.Counts.Queued != 1 {
		t.Fatalf("partial push = %+v, actions = %v", partial, ankiTransport.actions)
	}
	pending := notePending(t, db)
	if len(pending) != 1 {
		t.Fatalf("pending after partial push = %+v", pending)
	}
	if ankiTransport.count("addNote") != 2 {
		t.Fatalf("addNote count = %d, want 2", ankiTransport.count("addNote"))
	}

	ankiTransport.failAddAfter = 0
	ankiTransport.resetActions()
	rest := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &rest); err != nil {
		t.Fatal(err)
	}
	if rest.State != "pushed" || rest.Counts.Pushed != 1 || ankiTransport.count("addNote") != 1 {
		t.Fatalf("retry = %+v, actions = %v", rest, ankiTransport.actions)
	}
	if pending := notePending(t, db); len(pending) != 0 {
		t.Fatalf("pending after retry = %+v", pending)
	}
}

func TestNotePushReportsShortResultBatches(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	readyNoteSource(t, db, clock)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")

	ankiTransport := newFakeAnki()
	ankiTransport.shortCall = 1
	code, _, errOut := runNoteCLI(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")
	if code != 3 || !strings.Contains(errOut, "anki_error") {
		t.Fatalf("short capability batch = %d, %s", code, errOut)
	}

	ankiTransport.shortCall = 3
	ankiTransport.resetActions()
	code, _, errOut = runNoteCLI(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")
	if code != 3 {
		t.Fatalf("short write batch = %d, %s", code, errOut)
	}
	doc := notePushDoc{}
	if err := json.Unmarshal([]byte(errOut), &doc); err != nil {
		t.Fatalf("decode short write batch: %v\n%s", err, errOut)
	}
	if doc.State != "failed" || doc.Counts.Failed != 1 ||
		doc.Notes[0].Detail != "AnkiConnect returned fewer results than actions" {
		t.Fatalf("short write batch = %+v", doc)
	}
	if doc.Error == nil || doc.Error.Code != "anki_failed" {
		t.Fatalf("failed push stderr has no error envelope: %+v", doc)
	}
}
