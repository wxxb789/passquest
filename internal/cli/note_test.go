package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/wxxb789/passquest/internal/anki"
	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/tutor"
)

type fakeAnkiTransport struct {
	requests     []anki.HostRequest
	actions      []string
	models       []string
	notes        map[int64]map[string]string
	next         int64
	duplicate    bool
	unreachable  bool
	failAddAfter int
	adds         int
	calls        int
	failCall     int
	shortCall    int
	err          error
}

func newFakeAnki() *fakeAnkiTransport {
	return &fakeAnkiTransport{notes: map[int64]map[string]string{}, next: 101}
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
			f.models = append(f.models, action.Params["modelName"].(string))
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
				rows = append(rows, map[string]any{
					"noteId": id, "modelName": note.ModelName, "fields": encoded,
				})
			}
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

func runNoteCLI(t *testing.T, db string, clock *stepClock, ankiTransport *fakeAnkiTransport,
	tutorTransport tutor.Transport, key string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	options := Options{
		Stdin:      strings.NewReader(""),
		Stdout:     &stdout,
		Stderr:     &stderr,
		Now:        clock.Now,
		NewSession: func() string { return "sess-note" },
		Getenv: func(name string) string {
			if name == tutor.CredentialEnv {
				return key
			}
			return ""
		},
	}
	if ankiTransport != nil {
		options.AnkiTransport = ankiTransport
	}
	if tutorTransport != nil {
		options.TutorTransport = tutorTransport
	}
	code := Run(append(args, "--db", db), options)
	return code, stdout.String(), stderr.String()
}

func mustNoteRun(t *testing.T, db string, clock *stepClock, ankiTransport *fakeAnkiTransport,
	tutorTransport tutor.Transport, key string, args ...string) string {
	t.Helper()
	code, out, errOut := runNoteCLI(t, db, clock, ankiTransport, tutorTransport, key, args...)
	if code != 0 {
		t.Fatalf("Run(%v) = %d\nstdout:\n%s\nstderr:\n%s", args, code, out, errOut)
	}
	return out
}

func noteSourceFixture(t *testing.T, db string, clock *stepClock) string {
	t.Helper()
	sourceID := importFixture(t, db, clock).Source.ID
	mustRun(t, db, clock, "", "question", "add", sourceID, "1", "--file", fixtureFile(t, "question-q2-tagged.json"), "--json")
	mustRun(t, db, clock, "", "page", "review", sourceID, "1", "--reviewer", "note-reviewer", "--json")
	mustRun(t, db, clock, "", "question", "review", sourceID, "q2", "--answer", "C", "--reviewer", "note-reviewer", "--json")
	return sourceID
}

func noteConsent(t *testing.T, db string, clock *stepClock, sourceID, reviewer string) {
	t.Helper()
	mustRun(t, db, clock, "", "tutor", "consent", sourceID, "--reviewer", reviewer, "--json")
}

func noteAttempt(t *testing.T, db string, clock *stepClock, sourceID string) {
	t.Helper()
	mustRun(t, db, clock, "", "practice", "--source", sourceID, "--page", "1", "--question", "q2", "--answer", "A", "--json")
}

// readyNoteSource leaves the fixture with one incorrect attempt and current
// consent, so note commands can accept their draft.
func readyNoteSource(t *testing.T, db string, clock *stepClock) string {
	t.Helper()
	sourceID := noteSourceFixture(t, db, clock)
	noteAttempt(t, db, clock, sourceID)
	noteConsent(t, db, clock, sourceID, "note-reviewer")
	return sourceID
}

func notePending(t *testing.T, db string) []store.NoteOutboxEntry {
	t.Helper()
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	pending, err := st.PendingNoteOutbox()
	if err != nil {
		t.Fatalf("pending outbox: %v", err)
	}
	return pending
}

func TestNoteProposeConsentAcceptExport(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := noteSourceFixture(t, db, clock)
	noteAttempt(t, db, clock, sourceID)

	draftDoc := noteDraftDoc{}
	raw := mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	if err := json.Unmarshal([]byte(raw), &draftDoc); err != nil {
		t.Fatalf("decode draft: %v\n%s", err, raw)
	}
	if draftDoc.Draft.Origin != "deterministic" || !draftDoc.Draft.Restricted || draftDoc.Draft.ConsentGranted {
		t.Fatalf("draft flags = %+v", draftDoc.Draft)
	}
	if strings.Join(draftDoc.Draft.Withheld, ",") != "front,back" {
		t.Fatalf("withheld = %v", draftDoc.Draft.Withheld)
	}
	if draftDoc.Plain.Tactic != "NOT/EXCEPT polarity" || draftDoc.Plain.Source == "" {
		t.Fatalf("draft tactic/source = %+v", draftDoc.Plain)
	}

	previewRaw := mustNoteRun(t, db, clock, nil, nil, "", "note", "preview", "--json")
	if strings.Contains(previewRaw, "Westbridge Community Library") || strings.Contains(previewRaw, "No Sunday opening time") {
		t.Fatalf("restricted preview leaked text:\n%s", previewRaw)
	}
	preview := notePreviewDoc{}
	if err := json.Unmarshal([]byte(previewRaw), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Fields.Front != "" || preview.Fields.Back != "" || preview.Draft.Withheld == nil {
		t.Fatalf("preview = %+v", preview)
	}
	if preview.HTMLSHA256 == "" || preview.Deck != "Pass Quest" || preview.Model != "PassQuest Note" {
		t.Fatalf("preview identity = %+v", preview)
	}

	code, _, errOut := runNoteCLI(t, db, clock, nil, nil, "", "note", "accept", "--json")
	if code != 2 || !strings.Contains(errOut, "consent_required") {
		t.Fatalf("accept before consent = %d, %s", code, errOut)
	}
	noteConsent(t, db, clock, sourceID, "note-reviewer")

	authorizedRaw := mustNoteRun(t, db, clock, nil, nil, "", "note", "preview", "--json")
	authorized := notePreviewDoc{}
	if err := json.Unmarshal([]byte(authorizedRaw), &authorized); err != nil {
		t.Fatal(err)
	}
	if !authorized.Draft.ConsentGranted || authorized.Fields.Front != "The library opens at 8 a.m. on Sundays." {
		t.Fatalf("authorized preview = %+v", authorized)
	}
	if !strings.Contains(authorized.Fields.Back, "No Sunday opening time") || !strings.Contains(authorized.Fields.Back, "Westbridge Community Library closes") {
		t.Fatalf("authorized preview back = %q", authorized.Fields.Back)
	}

	first := noteAcceptDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")), &first); err != nil {
		t.Fatal(err)
	}
	if first.State != "accepted" || !first.Created || first.Note.Revision != 1 || !strings.HasPrefix(first.Note.NoteID, "note-") {
		t.Fatalf("first accept = %+v", first)
	}
	if pending := notePending(t, db); len(pending) != 1 || pending[0].Revision != 1 {
		t.Fatalf("pending after accept = %+v", pending)
	}

	second := noteAcceptDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")), &second); err != nil {
		t.Fatal(err)
	}
	if second.State != "unchanged" || second.Note.NoteID != first.Note.NoteID || second.Note.Revision != 1 || second.Changed {
		t.Fatalf("repeat accept = %+v", second)
	}

	edited := noteAcceptDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "",
		"note", "accept", "--front", "closes on Sundays", "--back", "A closing time.\n\nExample: The notice gives Saturday hours.", "--json")), &edited); err != nil {
		t.Fatal(err)
	}
	if edited.State != "accepted" || !edited.Changed || edited.Note.Revision != 2 || edited.PreviousRevision != 1 {
		t.Fatalf("edited accept = %+v", edited)
	}

	show := noteShowDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "show", "--json")), &show); err != nil {
		t.Fatal(err)
	}
	if len(show.Notes) != 1 || len(show.Notes[0].Revisions) != 2 {
		t.Fatalf("show = %+v", show)
	}
	if show.Notes[0].Revisions[0].Front != first.Note.Front || show.Notes[0].Revisions[1].Front != "closes on Sundays" {
		t.Fatalf("revision history = %+v", show.Notes[0].Revisions)
	}

	beforeReport := mustRun(t, db, clock, "", "report", "--json")
	exportDoc := noteExportDoc{}
	exportRaw := mustNoteRun(t, db, clock, nil, nil, "", "note", "export", "--json")
	if err := json.Unmarshal([]byte(exportRaw), &exportDoc); err != nil {
		t.Fatal(err)
	}
	if exportDoc.Notes != 1 || exportDoc.Bytes == 0 || exportDoc.SHA256 == "" {
		t.Fatalf("export doc = %+v", exportDoc)
	}
	content, err := os.ReadFile(exportDoc.Path)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if !strings.Contains(string(content), exportDoc.Header) || !strings.Contains(string(content), "closes on Sundays") {
		t.Fatalf("export content:\n%s", content)
	}
	if strings.Count(string(content), "\n") != 7 {
		t.Fatalf("export has an unexpected row count:\n%s", content)
	}
	// A second export replaces the file atomically.
	if _, err := note.WriteExport([]note.Fields{note.HTML("replacement", "m", "t", "s")}, exportDoc.Path); err != nil {
		t.Fatalf("replace export: %v", err)
	}
	replaced, err := os.ReadFile(exportDoc.Path)
	if err != nil || !strings.Contains(string(replaced), "replacement") {
		t.Fatalf("export was not replaced: %v", err)
	}
	code, _, errOut = runNoteCLI(t, db, clock, nil, nil, "", "note", "export", "--out", t.TempDir(), "--json")
	if code != 1 || !strings.Contains(errOut, "internal_error") {
		t.Fatalf("export failure = %d, %s", code, errOut)
	}
	if mustRun(t, db, clock, "", "report", "--json") != beforeReport {
		t.Fatal("note commands changed the attempt report")
	}
}

func TestNoteUnansweredPreviewHidesTheKey(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := noteSourceFixture(t, db, clock)
	noteConsent(t, db, clock, sourceID, "note-reviewer")

	raw := mustNoteRun(t, db, clock, nil, nil, "", "note", "propose",
		"--content", sourceID+"-p001", "--question", "q2", "--mark", "Sunday", "--json")
	for _, secret := range []string{
		"The library opens at 8 a.m. on Sundays.",
		"No Sunday opening time appears in the notice.",
		"The notice states the Saturday closing time",
	} {
		if strings.Contains(raw, secret) {
			t.Fatalf("unanswered draft leaked %q:\n%s", secret, raw)
		}
	}
	draft := noteDraftDoc{}
	if err := json.Unmarshal([]byte(raw), &draft); err != nil {
		t.Fatal(err)
	}
	if !draft.Draft.Unanswered || draft.Plain.Front != "Sunday" {
		t.Fatalf("draft = %+v", draft)
	}
	preview := mustNoteRun(t, db, clock, nil, nil, "", "note", "preview", "--json")
	if strings.Contains(preview, "The library opens at 8 a.m. on Sundays.") {
		t.Fatalf("unanswered preview leaked the key:\n%s", preview)
	}
	accepted := noteAcceptDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.Note.Front != "Sunday" {
		t.Fatalf("accepted front = %q", accepted.Note.Front)
	}
	show := noteShowDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "show", "--json")), &show); err != nil {
		t.Fatal(err)
	}
	if len(show.Notes) != 1 || show.Notes[0].Note.Target.Mark != "Sunday" {
		t.Fatalf("note show target mark = %+v", show.Notes)
	}
}

func TestNoteSkippedAttemptIsUnanswered(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := noteSourceFixture(t, db, clock)
	mustRun(t, db, clock, "", "practice", "--source", sourceID, "--page", "1", "--question", "q2", "--skip", "--json")
	noteConsent(t, db, clock, sourceID, "note-reviewer")
	draft := noteDraftDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")), &draft); err != nil {
		t.Fatal(err)
	}
	if !draft.Draft.Unanswered {
		t.Fatal("skipped attempt is not reported as unanswered")
	}
	for _, secret := range []string{"The library opens at 8 a.m. on Sundays.", "No Sunday opening time appears"} {
		if strings.Contains(draft.Plain.Front+draft.Plain.Back, secret) {
			t.Fatalf("skipped draft leaked %q: %+v", secret, draft.Plain)
		}
	}
}

func TestNotePushCreatesUpdatesConflictsAndQueues(t *testing.T) {
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
	if ankiTransport.count("notesInfo") != 0 {
		t.Fatal("first push read back a note it had not pushed")
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

func TestNoteRefineUsesProviderAndRecordsIt(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := readyNoteSource(t, db, clock)
	mustRun(t, db, clock, "", "tutor", "config", "--endpoint", "https://api.example.com/v1/chat/completions", "--model", "note-model", "--json")

	transport := &fakeTutorTransport{response: tutor.HostResponse{
		ProtocolVersion: tutor.ProtocolVersion, Status: "ok",
		Text:  `{"front":"refined front","back":"refined meaning\n\nExample: refined example"}`,
		Usage: &tutor.Usage{PromptTokens: 10, CompletionTokens: 4}, HTTPStatus: 200,
	}}
	doc := noteDraftDoc{}
	raw := mustNoteRun(t, db, clock, nil, transport, "key-not-real", "note", "propose", "--attempt", "1", "--refine", "--json")
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Draft.Origin != "refined" || !doc.Advisory || doc.Refinement == nil {
		t.Fatalf("refined doc = %+v", doc)
	}
	if doc.Plain.Front != "refined front" || !strings.Contains(doc.Plain.Back, "refined meaning") {
		t.Fatalf("refined content = %+v", doc.Plain)
	}
	if len(transport.requests) != 1 || transport.requests[0].Chat.Model != "note-model" {
		t.Fatalf("provider requests = %+v", transport.requests)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	refinements, err := st.NoteRefinements()
	if err != nil || len(refinements) != 1 || refinements[0].Status != "ok" || refinements[0].PromptTokens == nil || *refinements[0].PromptTokens != 10 {
		t.Fatalf("refinements = %+v, %v", refinements, err)
	}
	count, err := st.TutorCallCountSince("", tutorDayStart(clock.Now()))
	if err != nil || count != 1 {
		t.Fatalf("daily count = %d, %v", count, err)
	}
	if pending, err := st.PendingNoteOutbox(); err != nil || len(pending) != 0 {
		t.Fatalf("refinement created a note: %+v, %v", pending, err)
	}

	// A revoke withholds the refined text again on the next preview and refuses accept.
	mustRun(t, db, clock, "", "tutor", "revoke", sourceID, "--reviewer", "note-reviewer", "--json")
	revokedRaw := mustNoteRun(t, db, clock, nil, nil, "", "note", "preview", "--json")
	if strings.Contains(revokedRaw, "refined front") || strings.Contains(revokedRaw, "refined meaning") {
		t.Fatalf("revoked preview kept refined text:\n%s", revokedRaw)
	}
	revoked := notePreviewDoc{}
	if err := json.Unmarshal([]byte(revokedRaw), &revoked); err != nil {
		t.Fatal(err)
	}
	if len(revoked.Draft.Withheld) == 0 || revoked.Plain.Front != "" || revoked.Plain.Back != "" {
		t.Fatalf("revoked preview = %+v", revoked)
	}
	code, _, errOut := runNoteCLI(t, db, clock, nil, nil, "", "note", "accept", "--json")
	if code != 2 || !strings.Contains(errOut, "consent_required") {
		t.Fatalf("revoked accept = %d, %s", code, errOut)
	}
}

func TestNoteRefineRefusesRestrictedSourceWithoutConsent(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := noteSourceFixture(t, db, clock)
	mustRun(t, db, clock, "", "tutor", "config", "--endpoint", "https://api.example.com/v1/chat/completions", "--model", "note-model", "--json")

	transport := &fakeTutorTransport{response: fixedTutorResponse(`{"front":"kept","back":"kept meaning"}`)}
	code, _, errOut := runNoteCLI(t, db, clock, nil, transport, "key-not-real", "note", "propose",
		"--content", sourceID+"-p001", "--question", "q2", "--mark", "Sunday", "--refine", "--json")
	if code != 2 || !strings.Contains(errOut, "consent_required") {
		t.Fatalf("refine without consent = %d, %s", code, errOut)
	}
	if len(transport.requests) != 0 {
		t.Fatalf("refine without consent contacted the provider: %+v", transport.requests)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if refinements, err := st.NoteRefinements(); err != nil || len(refinements) != 0 {
		t.Fatalf("refinements without consent = %+v, %v", refinements, err)
	}
}

func TestNoteRefineRejectsAnAnswerRevealingResponse(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := noteSourceFixture(t, db, clock)
	mustRun(t, db, clock, "", "question", "add", sourceID, "1", "--file", fixtureFile(t, "question-q1.json"), "--json")
	mustRun(t, db, clock, "", "question", "review", sourceID, "q1", "--answer", "B", "--reviewer", "note-reviewer", "--json")
	noteConsent(t, db, clock, sourceID, "note-reviewer")
	mustRun(t, db, clock, "", "tutor", "config", "--endpoint", "https://api.example.com/v1/chat/completions", "--model", "note-model", "--json")

	transport := &fakeTutorTransport{response: fixedTutorResponse(
		`{"front":"At 4 p.m.","back":"The library closes at 4 p.m. on Saturdays."}`)}
	code, _, errOut := runNoteCLI(t, db, clock, nil, transport, "key-not-real", "note", "propose",
		"--content", sourceID+"-p001", "--question", "q1", "--mark", "closes", "--refine", "--json")
	if code != 3 || !strings.Contains(errOut, "invalid_response") || !strings.Contains(errOut, "reveal") {
		t.Fatalf("answer-revealing refinement = %d, %s", code, errOut)
	}
	if len(transport.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(transport.requests))
	}
	body := transport.requests[0].Body
	for _, secret := range []string{"At 4 p.m.", "closes at 4 p.m. on Saturdays"} {
		if strings.Contains(body, secret) {
			t.Fatalf("unanswered refinement request carried the key (%q): %s", secret, body)
		}
	}

	previewRaw := mustNoteRun(t, db, clock, nil, nil, "", "note", "preview", "--json")
	for _, secret := range []string{"At 4 p.m.", "closes at 4 p.m.", "The notice states 4 p.m."} {
		if strings.Contains(previewRaw, secret) {
			t.Fatalf("preview leaked %q after a rejected refinement:\n%s", secret, previewRaw)
		}
	}
	preview := notePreviewDoc{}
	if err := json.Unmarshal([]byte(previewRaw), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Plain.Front != "closes" || preview.Draft.Origin != "deterministic" {
		t.Fatalf("deterministic draft was lost: %+v", preview)
	}
	acceptedRaw := mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")
	if strings.Contains(acceptedRaw, "At 4 p.m.") {
		t.Fatalf("accepted note leaked the key:\n%s", acceptedRaw)
	}

	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	refinements, err := st.NoteRefinements()
	if err != nil || len(refinements) != 1 || refinements[0].Status != "error" || refinements[0].ErrorCode != "invalid_response" {
		t.Fatalf("rejected refinement record = %+v, %v", refinements, err)
	}
}

func TestNotePreviewIgnoresAStoredAnswerRevealingRefinement(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := noteSourceFixture(t, db, clock)
	mustRun(t, db, clock, "", "question", "add", sourceID, "1", "--file", fixtureFile(t, "question-q1.json"), "--json")
	mustRun(t, db, clock, "", "question", "review", sourceID, "q1", "--answer", "B", "--reviewer", "note-reviewer", "--json")
	noteConsent(t, db, clock, sourceID, "note-reviewer")

	draft := noteDraftDoc{}
	raw := mustNoteRun(t, db, clock, nil, nil, "", "note", "propose",
		"--content", sourceID+"-p001", "--question", "q1", "--mark", "closes", "--json")
	if err := json.Unmarshal([]byte(raw), &draft); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetNoteDraftRefinement(draft.Draft.ID, 1, "hostile-model", "At 4 p.m.", "The library closes at 4 p.m. on Saturdays."); err != nil {
		t.Fatalf("store hostile refinement: %v", err)
	}
	st.Close()

	previewRaw := mustNoteRun(t, db, clock, nil, nil, "", "note", "preview", "--json")
	if strings.Contains(previewRaw, "At 4 p.m.") {
		t.Fatalf("stored refinement leaked the key:\n%s", previewRaw)
	}
	preview := notePreviewDoc{}
	if err := json.Unmarshal([]byte(previewRaw), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Plain.Front != "closes" || preview.Draft.Origin != "deterministic" {
		t.Fatalf("stored refinement was not discarded: %+v", preview)
	}
}

func TestNoteRefineFailureKeepsDeterministicDraft(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	readyNoteSource(t, db, clock)
	mustRun(t, db, clock, "", "tutor", "config", "--endpoint", "https://api.example.com/v1/chat/completions", "--model", "note-model", "--json")

	transport := &fakeTutorTransport{response: fixedTutorError("timeout", "provider timed out")}
	code, _, errOut := runNoteCLI(t, db, clock, nil, transport, "key-not-real", "note", "propose", "--attempt", "1", "--refine", "--json")
	if code != 3 || !strings.Contains(errOut, "timeout") {
		t.Fatalf("failed refinement = %d, %s", code, errOut)
	}
	preview := mustNoteRun(t, db, clock, nil, nil, "", "note", "preview", "--json")
	if !strings.Contains(preview, "No Sunday opening time") {
		t.Fatalf("deterministic draft was lost:\n%s", preview)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	refinements, err := st.NoteRefinements()
	if err != nil || len(refinements) != 1 || refinements[0].Status != "error" || refinements[0].ErrorCode != "timeout" {
		t.Fatalf("refinements = %+v, %v", refinements, err)
	}
}

func TestNoteProposeRefusals(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := noteSourceFixture(t, db, clock)
	mustRun(t, db, clock, "", "practice", "--source", sourceID, "--page", "1", "--question", "q2", "--answer", "C", "--json")
	code, _, errOut := runNoteCLI(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	if code != 2 || !strings.Contains(errOut, "low_value") {
		t.Fatalf("correct attempt without mark = %d, %s", code, errOut)
	}
	code, _, errOut = runNoteCLI(t, db, clock, nil, nil, "", "note", "propose",
		"--content", sourceID+"-p001", "--question", "q2", "--json")
	if code != 2 || !strings.Contains(errOut, "invalid_input") {
		t.Fatalf("question without mark = %d, %s", code, errOut)
	}
	code, _, errOut = runNoteCLI(t, db, clock, nil, nil, "", "note", "preview", "--json")
	if code != 2 || !strings.Contains(errOut, "no_draft") {
		t.Fatalf("preview without draft = %d, %s", code, errOut)
	}
	code, _, errOut = runNoteCLI(t, db, clock, nil, nil, "", "note", "accept", "--json")
	if code != 2 || !strings.Contains(errOut, "no_draft") {
		t.Fatalf("accept without draft = %d, %s", code, errOut)
	}
}

func TestNoteModelDocument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"note", "model", "--json"}, Options{
		Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr,
		Now: (&stepClock{base: goldenBase}).Now, NewSession: func() string { return "sess-note" },
		Getenv: func(string) string { return "" },
	})
	if code != 0 {
		t.Fatalf("note model = %d: %s", code, stderr.String())
	}
	doc := noteModelDoc{}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Deck != "Pass Quest" || doc.Model != "PassQuest Note" || len(doc.Fields) != 4 {
		t.Fatalf("model identity = %+v", doc)
	}
	if strings.Contains(strings.ToLower(doc.CSS), "overflow: hidden") || strings.Contains(doc.Rendered, "http") {
		t.Fatalf("model is not self-contained: %+v", doc)
	}
	if !strings.Contains(doc.Rendered, "pq-front") || !strings.Contains(doc.Rendered, "pq-source") {
		t.Fatalf("rendered sample = %s", doc.Rendered)
	}
}

func TestNoteOutputsNeverClaimAPhonePush(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	readyNoteSource(t, db, clock)
	var outputs []string
	outputs = append(outputs, mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1"))
	outputs = append(outputs, mustNoteRun(t, db, clock, nil, nil, "", "note", "preview"))
	outputs = append(outputs, mustNoteRun(t, db, clock, nil, nil, "", "note", "accept"))
	outputs = append(outputs, mustNoteRun(t, db, clock, nil, nil, "", "note", "show"))
	var modelOut, modelErr bytes.Buffer
	if code := Run([]string{"note", "model"}, Options{
		Stdin: strings.NewReader(""), Stdout: &modelOut, Stderr: &modelErr,
		Now: clock.Now, NewSession: func() string { return "sess-note" }, Getenv: func(string) string { return "" },
	}); code != 0 {
		t.Fatalf("note model = %d: %s", code, modelErr.String())
	}
	outputs = append(outputs, modelOut.String())
	outputs = append(outputs, mustNoteRun(t, db, clock, newFakeAnki(), nil, "", "note", "push"))
	outputs = append(outputs, mustNoteRun(t, db, clock, nil, nil, "", "note", "export"))
	for _, output := range outputs {
		lower := strings.ToLower(output)
		for _, phrase := range []string{
			"pushed to your phone", "sent to your phone", "sync to your phone", "syncs to your phone",
			"will appear on your phone", "automatically sync", "synchronized to ankiweb", "phone is up to date",
		} {
			if strings.Contains(lower, phrase) {
				t.Fatalf("output claims %q:\n%s", phrase, output)
			}
		}
	}
	if !strings.Contains(outputs[1], "AnkiWeb and mobile synchronization are your own manual actions") {
		t.Fatal("preview does not state the manual-sync boundary")
	}
}

func TestNotePushEndpointMustBeLoopback(t *testing.T) {
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

func TestNoteAcceptEditedFieldsAreValidated(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	readyNoteSource(t, db, clock)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	code, _, errOut := runNoteCLI(t, db, clock, nil, nil, "", "note", "accept", "--front", "   ", "--json")
	if code != 2 || !strings.Contains(errOut, "invalid_front") {
		t.Fatalf("empty front = %d, %s", code, errOut)
	}
	code, _, errOut = runNoteCLI(t, db, clock, nil, nil, "", "note", "accept", "--back", "", "--json")
	if code != 2 || !strings.Contains(errOut, "invalid_back") {
		t.Fatalf("empty back = %d, %s", code, errOut)
	}
}

func TestNoteAcceptPreservesAcceptedEdits(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	readyNoteSource(t, db, clock)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")

	first := noteAcceptDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")), &first); err != nil {
		t.Fatal(err)
	}
	edited := noteAcceptDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "",
		"note", "accept", "--front", "custom wording", "--tactic", "custom tactic", "--json")), &edited); err != nil {
		t.Fatal(err)
	}
	if edited.Note.Revision != 2 || edited.Plain.Front != "custom wording" ||
		edited.Plain.Back != first.Plain.Back || edited.Plain.Tactic != "custom tactic" {
		t.Fatalf("edited accept = %+v", edited)
	}

	preview := notePreviewDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "preview", "--json")), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Plain.Front != "custom wording" || preview.Plain.Back != first.Plain.Back ||
		preview.Plain.Tactic != "custom tactic" {
		t.Fatalf("preview after an edited accept = %+v", preview.Plain)
	}

	repeat := noteAcceptDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")), &repeat); err != nil {
		t.Fatal(err)
	}
	if repeat.State != "unchanged" || repeat.Note.Revision != 2 ||
		repeat.Plain.Front != "custom wording" || repeat.Plain.Tactic != "custom tactic" {
		t.Fatalf("repeat accept reverted the edit = %+v", repeat)
	}

	incremental := noteAcceptDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "",
		"note", "accept", "--back", "A learner meaning.", "--json")), &incremental); err != nil {
		t.Fatal(err)
	}
	if incremental.Note.Revision != 3 || incremental.Plain.Front != "custom wording" ||
		incremental.Plain.Back != "A learner meaning." || incremental.Plain.Tactic != "custom tactic" {
		t.Fatalf("incremental accept reset another field = %+v", incremental)
	}

	confirm := noteAcceptDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")), &confirm); err != nil {
		t.Fatal(err)
	}
	if confirm.State != "unchanged" || confirm.Note.Revision != 3 {
		t.Fatalf("confirm accept = %+v", confirm)
	}
}

func TestNotePreviewWithholdsAcceptedTextAfterRevoke(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := readyNoteSource(t, db, clock)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--front", "learner wording", "--json")
	mustRun(t, db, clock, "", "tutor", "revoke", sourceID, "--reviewer", "note-reviewer", "--json")

	preview := notePreviewDoc{}
	out := mustNoteRun(t, db, clock, nil, nil, "", "note", "preview", "--json")
	if err := json.Unmarshal([]byte(out), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Fields.Front != "" || preview.Fields.Back != "" || preview.Plain.Front != "" {
		t.Fatalf("revoked preview exposed accepted text: %+v", preview)
	}
	if strings.Contains(out, "learner wording") {
		t.Fatal("revoked preview document contains the accepted front")
	}
}

func TestNotePushKeepsAnAcknowledgedWriteInAPartialBatch(t *testing.T) {
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

func TestNoteProposeRefusesContentChangedAfterTheAttempt(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := noteSourceFixture(t, db, clock)
	noteAttempt(t, db, clock, sourceID)
	mustRun(t, db, clock, "", "page", "edit", sourceID, "1", "--text-file", fixtureFile(t, "page-001-corrected.txt"), "--reviewer", "note-reviewer", "--json")
	mustRun(t, db, clock, "", "page", "review", sourceID, "1", "--reviewer", "note-reviewer", "--json")
	code, _, errOut := runNoteCLI(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	if code != 2 || !strings.Contains(errOut, "content_changed") {
		t.Fatalf("changed content = %d, %s", code, errOut)
	}
}

func TestNotePushReaddsANoteDeletedFromAnki(t *testing.T) {
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
	if got := strings.Join(ankiTransport.actions, ","); got != "version,createDeck,modelNames,notesInfo,addNote" {
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
	if got := strings.Join(ledger, ","); got != "add/ok,check/not_found,readd/ok" {
		t.Fatalf("ledger = %s, want add/ok,check/not_found,readd/ok", got)
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
	if got := strings.Join(ledger, ","); got != "add/ok,check/not_found,readd/ok,update/not_found,readd/ok" {
		t.Fatalf("forced readd ledger = %s", got)
	}
}

func TestNoteExportRechecksConsent(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := readyNoteSource(t, db, clock)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")
	mustRun(t, db, clock, "", "tutor", "revoke", sourceID, "--reviewer", "note-reviewer", "--json")

	out := filepath.Join(t.TempDir(), "revoked.tsv")
	code, _, errOut := runNoteCLI(t, db, clock, nil, nil, "", "note", "export", "--out", out, "--json")
	if code != 2 || !strings.Contains(errOut, "consent_required") {
		t.Fatalf("revoked export = %d, %s", code, errOut)
	}
	if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("revoked export wrote a file: %v", err)
	}

	noteConsent(t, db, clock, sourceID, "note-reviewer")
	exported := noteExportDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "export", "--out", out, "--json")), &exported); err != nil {
		t.Fatal(err)
	}
	if exported.Notes != 1 {
		t.Fatalf("re-consented export = %+v", exported)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("re-consented export did not write: %v", err)
	}
}

func TestNotePreviewAndAcceptRefuseChangedDraftContent(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := readyNoteSource(t, db, clock)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	mustRun(t, db, clock, "", "page", "edit", sourceID, "1", "--text-file", fixtureFile(t, "page-001-corrected.txt"), "--reviewer", "note-reviewer", "--json")
	mustRun(t, db, clock, "", "page", "review", sourceID, "1", "--reviewer", "note-reviewer", "--json")
	for _, command := range []string{"preview", "accept"} {
		code, _, errOut := runNoteCLI(t, db, clock, nil, nil, "", "note", command, "--json")
		if code != 2 || !strings.Contains(errOut, "content_changed") {
			t.Fatalf("note %s after a content change = %d, %s", command, code, errOut)
		}
	}
}

func TestNoteDefaultDraftIsTheLatestOpenOne(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := readyNoteSource(t, db, clock)
	first := noteDraftDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "propose",
		"--content", sourceID+"-p001", "--question", "q2", "--mark", "Sunday", "--json")), &first); err != nil {
		t.Fatal(err)
	}
	second := noteDraftDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "propose",
		"--content", sourceID+"-p001", "--question", "q2", "--mark", "Saturday", "--json")), &second); err != nil {
		t.Fatal(err)
	}
	if first.Draft.ID == second.Draft.ID {
		t.Fatalf("second mark reused draft %d", first.Draft.ID)
	}

	latest := notePreviewDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "preview", "--json")), &latest); err != nil {
		t.Fatal(err)
	}
	if latest.Draft.ID != second.Draft.ID || latest.Plain.Front != "Saturday" {
		t.Fatalf("default preview = %+v", latest.Draft)
	}

	explicit := notePreviewDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "preview",
		"--draft", fmt.Sprint(first.Draft.ID), "--json")), &explicit); err != nil {
		t.Fatal(err)
	}
	if explicit.Draft.ID != first.Draft.ID || explicit.Plain.Front != "Sunday" {
		t.Fatalf("explicit preview = %+v", explicit.Draft)
	}

	accepted := noteAcceptDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.Plain.Front != "Saturday" {
		t.Fatalf("default accept = %+v", accepted.Plain)
	}
}

func TestNotePushReportsShortResultBatches(t *testing.T) {
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
