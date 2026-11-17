package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/tutor"
)

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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestNoteProposeRefusals(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestNoteAcceptEditedFieldsAreValidated(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestNoteProposeRefusesContentChangedAfterTheAttempt(t *testing.T) {
	t.Parallel()
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

func TestNotePreviewAndAcceptRefuseChangedDraftContent(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
