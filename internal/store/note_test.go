package store

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
)

func noteTargetFixture() NoteTarget {
	return NoteTarget{
		NoteID:    "note-0123456789abcdef",
		TargetKey: "src-x-p001\x1fq1\x1fcloses",
		AttemptID: 1,
		ContentID: "src-x-p001", ContentVersion: "v1", QuestionID: "q1",
		SourceID: "src-x", Page: 1, Mark: "closes", Restricted: true,
	}
}

func noteContentFixture(marker string) NoteContent {
	return NoteContent{
		Front: "front " + marker, Back: "meaning " + marker + "\n\nExample: example " + marker,
		Tactic: "NOT/EXCEPT polarity", SourceLabel: "src-x · p001 · q1",
		ContentHash: strings.Repeat("a", 64), Origin: "deterministic",
	}
}

func TestAcceptNoteIsIdempotentAndKeepsHistory(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "notes.sqlite"))
	target := noteTargetFixture()
	content := noteContentFixture("one")
	at := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	result, err := st.AcceptNote(target, content, at)
	if err != nil {
		t.Fatalf("first accept: %v", err)
	}
	if !result.Created || !result.Changed || result.Note.Revision != 1 {
		t.Fatalf("first accept = %+v", result)
	}
	if result.Note.AnkiState != AnkiStateLocal {
		t.Fatalf("anki state = %q", result.Note.AnkiState)
	}
	if result.Note.Mark != "closes" {
		t.Fatalf("note mark = %q", result.Note.Mark)
	}
	again, err := st.AcceptNote(target, content, at.Add(time.Second))
	if err != nil {
		t.Fatalf("repeat accept: %v", err)
	}
	if again.Created || again.Changed || again.Note.Revision != 1 || again.Previous != 1 {
		t.Fatalf("repeat accept = %+v", again)
	}
	if pending, err := st.PendingNoteOutbox(); err != nil || len(pending) != 1 {
		t.Fatalf("pending after repeat = %+v, %v", pending, err)
	}

	edited := noteContentFixture("two")
	edited.ContentHash = strings.Repeat("b", 64)
	edited.Origin = "edited"
	bumped, err := st.AcceptNote(target, edited, at.Add(2*time.Second))
	if err != nil {
		t.Fatalf("edited accept: %v", err)
	}
	if bumped.Created || !bumped.Changed || bumped.Note.Revision != 2 || bumped.Previous != 1 {
		t.Fatalf("edited accept = %+v", bumped)
	}
	revisions, err := st.NoteRevisions(target.NoteID)
	if err != nil || len(revisions) != 2 {
		t.Fatalf("revisions = %+v, %v", revisions, err)
	}
	if revisions[0].Front != "front one" || revisions[1].Front != "front two" {
		t.Fatalf("revision history = %+v", revisions)
	}
	if revisions[0].AcceptedAt.IsZero() || revisions[1].AcceptedAt.IsZero() {
		t.Fatal("revision timestamp is missing")
	}
	pending, err := st.PendingNoteOutbox()
	if err != nil || len(pending) != 1 || pending[0].Revision != 2 {
		t.Fatalf("pending after edit = %+v, %v", pending, err)
	}
}

func TestNoteRevisionAndLedgerTablesAreAppendOnly(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "notes.sqlite"))
	target := noteTargetFixture()
	if _, err := st.AcceptNote(target, noteContentFixture("one"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`UPDATE note_revisions SET front = 'evil'`); err == nil {
		t.Fatal("note revision update was accepted")
	}
	if _, err := st.db.Exec(`DELETE FROM note_revisions`); err == nil {
		t.Fatal("note revision delete was accepted")
	}
	if err := st.RecordAnkiAttempt(AnkiAttempt{
		NoteID: target.NoteID, Revision: 1, Action: "add", Outcome: "ok", At: time.Now(), AnkiNoteID: 99,
		AnkiState: AnkiStatePushed, PushedRevision: 1, Drain: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`UPDATE anki_push_attempts SET outcome = 'failed'`); err == nil {
		t.Fatal("anki attempt update was accepted")
	}
	if _, err := st.db.Exec(`DELETE FROM anki_push_attempts`); err == nil {
		t.Fatal("anki attempt delete was accepted")
	}
}

func TestNoteOutboxDrainsOnceAndKeepsPendingOnFailure(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "notes.sqlite"))
	target := noteTargetFixture()
	if _, err := st.AcceptNote(target, noteContentFixture("one"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordAnkiAttempt(AnkiAttempt{
		NoteID: target.NoteID, Revision: 1, Action: "add", Outcome: "conflict", Detail: "conflict",
		At: time.Now(), AnkiState: AnkiStateConflict,
	}); err != nil {
		t.Fatal(err)
	}
	pending, err := st.PendingNoteOutbox()
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending after conflict = %+v, %v", pending, err)
	}
	if pending[0].Attempts != 1 || pending[0].LastError != "conflict" {
		t.Fatalf("outbox attempt bookkeeping = %+v", pending[0])
	}
	if err := st.RecordAnkiAttempt(AnkiAttempt{
		NoteID: target.NoteID, Revision: 1, Action: "add", Outcome: "ok", At: time.Now(),
		AnkiNoteID: 100, AnkiState: AnkiStatePushed, PushedRevision: 1, Drain: true,
	}); err != nil {
		t.Fatal(err)
	}
	pending, err = st.PendingNoteOutbox()
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending after drain = %+v, %v", pending, err)
	}
	note, err := st.Note(target.NoteID)
	if err != nil {
		t.Fatal(err)
	}
	if note.AnkiNoteID != 100 || note.AnkiState != AnkiStatePushed || note.AnkiPushedRevision != 1 {
		t.Fatalf("note = %+v", note)
	}
	all, err := st.NoteOutbox()
	if err != nil || len(all) != 1 || all[0].Status != "drained" || all[0].DrainedAt.IsZero() {
		t.Fatalf("outbox = %+v, %v", all, err)
	}
}

func TestNoteDraftSupersedesChangedInput(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "notes.sqlite"))
	at := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	draft := NoteDraft{
		TargetKey: "c\x1fq\x1fcloses", AttemptID: 1, ContentID: "c", ContentVersion: "v1",
		QuestionID: "q", SourceID: "src", Page: 1, Mark: "closes", Restricted: true,
		Origin: "deterministic",
	}
	first, err := st.SaveNoteDraft(draft, at)
	if err != nil {
		t.Fatalf("SaveNoteDraft: %v", err)
	}
	same, err := st.SaveNoteDraft(draft, at.Add(time.Second))
	if err != nil || same.ID != first.ID {
		t.Fatalf("identical draft = %+v, %v; want id %d", same, err, first.ID)
	}
	changed := draft
	changed.RefinedFront = "refined"
	changed.RefinedBack = "refined back"
	changed.Origin = "refined"
	changed.Model = "m"
	second, err := st.SaveNoteDraft(changed, at.Add(2*time.Second))
	if err != nil || second.ID == first.ID {
		t.Fatalf("changed draft = %+v, %v", second, err)
	}
	if _, found, err := st.NoteDraftByID(first.ID); err != nil || !found {
		t.Fatalf("superseded draft disappeared: %v, %v", found, err)
	} else if old, _, _ := st.NoteDraftByID(first.ID); old.Status != NoteDraftSuperseded {
		t.Fatalf("old draft status = %q", old.Status)
	}
	latest, found, err := st.LatestOpenNoteDraft()
	if err != nil || !found || latest.ID != second.ID {
		t.Fatalf("latest open draft = %+v, %v", latest, err)
	}
	if err := st.SetNoteDraftStatus(second.ID, NoteDraftSkipped); err != nil {
		t.Fatal(err)
	}
	if _, found, err := st.LatestOpenNoteDraft(); err != nil || found {
		t.Fatalf("skipped draft is still open: %v, %v", found, err)
	}
}

func TestNoteRefinementSharesTheDailyBudget(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "notes.sqlite"))
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	day := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	reservation, used, err := st.ReserveNoteRefinement("", day, now, 2)
	if err != nil || reservation == 0 || used != 1 {
		t.Fatalf("first reserve = %d, %d, %v", reservation, used, err)
	}
	refinement := NoteRefinement{
		DraftID: 1, CreatedAt: now, Status: "ok", ProviderHost: "127.0.0.1:1", Model: "m",
		PromptVersion: "note-refine-v1", RequestSHA256: strings.Repeat("c", 64), RequestBytes: 10,
		OutputFront: "f", OutputBack: "b", HTTPStatus: 200, LatencyMS: 5,
	}
	saved, err := st.FinalizeNoteRefinement(refinement, reservation)
	if err != nil || saved.ID == 0 {
		t.Fatalf("finalize = %+v, %v", saved, err)
	}
	if count, err := st.TutorCallCountSince("", day); err != nil || count != 1 {
		t.Fatalf("shared daily count = %d, %v", count, err)
	}
	reservation, used, err = st.ReserveNoteRefinement("", day, now, 2)
	if err != nil || reservation == 0 || used != 2 {
		t.Fatalf("second reserve = %d, %d, %v", reservation, used, err)
	}
	if err := st.ReleaseTutorCallReservation(reservation); err != nil {
		t.Fatal(err)
	}
	if _, used, err := st.ReserveNoteRefinement("", day, now, 1); err != nil || used != 1 {
		t.Fatalf("limit-1 reserve after finalize = %d, %v", used, err)
	}
	if rows, err := st.NoteRefinements(); err != nil || len(rows) != 1 {
		t.Fatalf("refinements = %+v, %v", rows, err)
	}
	if _, err := st.db.Exec(`UPDATE note_refinements SET model = 'evil'`); err == nil {
		t.Fatal("refinement update was accepted")
	}
}

func TestTutorReservationCountsFinalizedRefinements(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "shared-budget.sqlite"))
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	day := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	reservation, _, err := st.ReserveNoteRefinement("", day, now, 2)
	if err != nil {
		t.Fatalf("reserve refinement: %v", err)
	}
	refinement := NoteRefinement{
		DraftID: 1, CreatedAt: now, Status: "ok", ProviderHost: "127.0.0.1:1", Model: "m",
		PromptVersion: "note-refine-v1", RequestSHA256: strings.Repeat("e", 64), RequestBytes: 10,
		OutputFront: "f", OutputBack: "b", HTTPStatus: 200, LatencyMS: 5,
	}
	if _, err := st.FinalizeNoteRefinement(refinement, reservation); err != nil {
		t.Fatalf("finalize refinement: %v", err)
	}
	if refused, used, err := st.ReserveTutorCall("", day, now, 1); err != nil || refused != 0 || used != 1 {
		t.Fatalf("tutor reservation after a refinement = %d, %d, %v; want a refusal at 1", refused, used, err)
	}
}

func TestMigrationFromVersionNineKeepsAttempts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v9.sqlite")
	db := openSQLite(t, path)
	for _, statements := range [][]string{
		schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6, schemaV7, schemaV8, schemaV9,
	} {
		for _, statement := range statements {
			if _, err := db.Exec(statement); err != nil {
				t.Fatalf("create v9 schema: %v", err)
			}
		}
	}
	at := "2026-11-11T09:00:00Z"
	if _, err := db.Exec(`INSERT INTO attempts (
			session_id, question_id, content_id, content_version, status,
			selected_label, selected_text, correct_label, correct_text,
			started_at, answered_at, elapsed_ms, assistance, tactic_id, skills_json
		) VALUES ('sess', 'q1', 'sample-0001', '1', 'correct',
			'B', 'At 4 p.m.', 'B', 'At 4 p.m.', ?, ?, 5, 'independent', '', '[]')`, at, at); err != nil {
		t.Fatalf("insert v9 attempt: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 9`); err != nil {
		t.Fatalf("set v9 version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v9 fixture: %v", err)
	}

	st := mustOpen(t, path)
	records, err := st.List()
	if err != nil || len(records) != 1 {
		t.Fatalf("attempts after migration = %+v, %v", records, err)
	}
	if records[0].QuestionID != "q1" || records[0].Status != attempt.StatusCorrect {
		t.Fatalf("attempt changed: %+v", records[0])
	}
	if _, err := st.AcceptNote(noteTargetFixture(), noteContentFixture("migrated"), time.Now()); err != nil {
		t.Fatalf("v10 tables after migration: %v", err)
	}
	if count, err := st.NoteRefinements(); err != nil || len(count) != 0 {
		t.Fatalf("unexpected refinements: %+v, %v", count, err)
	}
	if _, err := st.db.Exec(`UPDATE attempts SET status = 'incorrect'`); err == nil {
		t.Fatal("attempt update was accepted")
	}
}

func TestMigrationFromVersionTenAddsNoteMark(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v10.sqlite")
	db := openSQLite(t, path)
	for _, statements := range [][]string{
		schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6, schemaV7, schemaV8, schemaV9, schemaV10,
	} {
		for _, statement := range statements {
			if _, err := db.Exec(statement); err != nil {
				t.Fatalf("create v10 schema: %v", err)
			}
		}
	}
	if _, err := db.Exec(`PRAGMA user_version = 10`); err != nil {
		t.Fatalf("set v10 version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v10 fixture: %v", err)
	}

	st := mustOpen(t, path)
	var version int
	if err := st.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != SchemaVersion {
		t.Fatalf("user_version = %d, want %d", version, SchemaVersion)
	}
	target := noteTargetFixture()
	if _, err := st.AcceptNote(target, noteContentFixture("migrated"), time.Now()); err != nil {
		t.Fatalf("accept after v11 migration: %v", err)
	}
	stored, err := st.Note(target.NoteID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Mark != "closes" {
		t.Fatalf("note mark after migration = %q", stored.Mark)
	}
}

func TestMigrationFromVersionElevenAddsAcceptedDraftFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v11.sqlite")
	db := openSQLite(t, path)
	for _, statements := range [][]string{
		schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6, schemaV7, schemaV8, schemaV9, schemaV10, schemaV11,
	} {
		for _, statement := range statements {
			if _, err := db.Exec(statement); err != nil {
				t.Fatalf("create v11 schema: %v", err)
			}
		}
	}
	if _, err := db.Exec(`PRAGMA user_version = 11`); err != nil {
		t.Fatalf("set v11 version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v11 fixture: %v", err)
	}

	st := mustOpen(t, path)
	at := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	targetKey := "src-x-p001\x1fq1\x1fcloses"
	draft, err := st.SaveNoteDraft(NoteDraft{
		TargetKey: targetKey, ContentID: "src-x-p001", QuestionID: "q1", Origin: "deterministic",
	}, at)
	if err != nil {
		t.Fatalf("save draft after v12 migration: %v", err)
	}
	target := NoteTarget{
		NoteID: "note-accepted-draft", TargetKey: targetKey,
		ContentID: "src-x-p001", ContentVersion: "v1", QuestionID: "q1", Page: 1, Mark: "closes",
	}
	content := noteContentFixture("accepted")
	content.Origin = "edited"
	if _, err := st.AcceptDraftNote(draft.ID, target, content, at); err != nil {
		t.Fatalf("accept draft after v12 migration: %v", err)
	}
	stored, found, err := st.NoteDraftByID(draft.ID)
	if err != nil || !found {
		t.Fatalf("read accepted draft = %+v, %t, %v", stored, found, err)
	}
	if stored.AcceptedFront != content.Front || stored.AcceptedBack != content.Back ||
		stored.AcceptedTactic != content.Tactic || stored.AcceptedOrigin != content.Origin {
		t.Fatalf("accepted draft content = %+v", stored)
	}
}

func TestNoteDraftRefinementClearsAcceptedContent(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "notes.sqlite"))
	at := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	targetKey := "src-x-p001\x1fq1\x1fcloses"
	draft, err := st.SaveNoteDraft(NoteDraft{
		TargetKey: targetKey, ContentID: "src-x-p001", QuestionID: "q1", Origin: "deterministic",
	}, at)
	if err != nil {
		t.Fatalf("save draft: %v", err)
	}
	target := NoteTarget{
		NoteID: "note-refined-draft", TargetKey: targetKey,
		ContentID: "src-x-p001", ContentVersion: "v1", QuestionID: "q1", Page: 1, Mark: "closes",
	}
	content := noteContentFixture("edited")
	content.Origin = "edited"
	if _, err := st.AcceptDraftNote(draft.ID, target, content, at); err != nil {
		t.Fatalf("accept draft: %v", err)
	}
	if err := st.SetNoteDraftRefinement(draft.ID, 1, "m", "refined front", "refined back"); err != nil {
		t.Fatalf("set refinement: %v", err)
	}
	stored, found, err := st.NoteDraftByID(draft.ID)
	if err != nil || !found {
		t.Fatalf("read refined draft = %+v, %t, %v", stored, found, err)
	}
	if stored.AcceptedFront != "" || stored.RefinedFront != "refined front" {
		t.Fatalf("refinement did not replace accepted content: %+v", stored)
	}
}

func TestNoteCommandsDoNotChangeAttempts(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "notes.sqlite"))
	mustAppend(t, st, sampleAttempt("q1"))
	before, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	target := noteTargetFixture()
	if _, err := st.AcceptNote(target, noteContentFixture("one"), time.Now()); err != nil {
		t.Fatal(err)
	}
	edited := noteContentFixture("two")
	edited.ContentHash = strings.Repeat("d", 64)
	if _, err := st.AcceptNote(target, edited, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordAnkiAttempt(AnkiAttempt{
		NoteID: target.NoteID, Revision: 2, Action: "add", Outcome: "ok", At: time.Now(),
		AnkiNoteID: 5, AnkiState: AnkiStatePushed, PushedRevision: 2, Drain: true,
	}); err != nil {
		t.Fatal(err)
	}
	after, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) || !reflect.DeepEqual(before[0], after[0]) {
		t.Fatalf("attempt changed: before=%+v after=%+v", before, after)
	}
}

func TestNoteDraftStatusRejectsUnknownState(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "notes.sqlite"))
	draft := NoteDraft{
		TargetKey: "c\x1fq\x1f", ContentID: "c", QuestionID: "q", Origin: "deterministic",
	}
	saved, err := st.SaveNoteDraft(draft, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetNoteDraftStatus(saved.ID, "accepted"); err == nil {
		t.Fatal("SetNoteDraftStatus accepted an unknown state")
	}
	if err := st.SetNoteDraftStatus(saved.ID+99, NoteDraftSkipped); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown draft error = %v", err)
	}
}

func TestMigrationFromVersionFifteenRecordsTheDeckReadBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v15.sqlite")
	db := openSQLite(t, path)
	for _, migration := range migrations {
		if migration.version > 15 {
			break
		}
		for _, statement := range migration.statements {
			if _, err := db.Exec(statement); err != nil {
				t.Fatalf("create v15 schema at %d: %v", migration.version, err)
			}
		}
	}
	if _, err := db.Exec(`PRAGMA user_version = 15`); err != nil {
		t.Fatalf("set v15 version: %v", err)
	}
	at := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	stamp := at.Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO notes (
			note_id, target_key, attempt_id, content_id, content_version, question_id, source_id,
			page_number, restricted, revision, front, back, tactic, source_label, content_hash, origin, model,
			mark, anki_note_id, anki_pushed_revision, anki_state, created_at, updated_at
		) VALUES ('note-legacy', 'legacy' || char(31) || 'q1' || char(31) || 'mark', 0, 'c', 'v1', 'q1', '',
			1, 1, 1, 'f', 'b', 't', 's', ?, 'deterministic', '', 'mark', 77, 1, 'pushed', ?, ?)`,
		strings.Repeat("a", 64), stamp, stamp); err != nil {
		t.Fatalf("insert v15 note: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO anki_push_attempts (note_id, revision, action, outcome, detail, at)
		VALUES ('note-legacy', 1, 'add', 'ok', '', ?)`, stamp); err != nil {
		t.Fatalf("insert v15 attempt: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v15 fixture: %v", err)
	}

	st := mustOpen(t, path)
	var version int
	if err := st.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != SchemaVersion {
		t.Fatalf("user_version = %d, want %d", version, SchemaVersion)
	}
	stored, err := st.Note("note-legacy")
	if err != nil {
		t.Fatalf("note after v16 migration: %v", err)
	}
	if stored.AnkiNoteID != 77 || stored.AnkiState != AnkiStatePushed || stored.AnkiPushedRevision != 1 {
		t.Fatalf("migrated note = %+v", stored)
	}
	if stored.AnkiDeck != "" {
		t.Fatalf("migrated note claims deck %q; only a completed read records one", stored.AnkiDeck)
	}
	// The whole point of the rebuild: the new state and outcome are accepted...
	if err := st.RecordAnkiAttempt(AnkiAttempt{
		NoteID: "note-legacy", Revision: 1, Action: "check", Outcome: "deck_mismatch",
		Detail: `card landed in "Default", not "Pass Quest"`, At: at,
		AnkiDeck: "Default", AnkiState: AnkiStateDeckMismatch, Keep: true,
	}); err != nil {
		t.Fatalf("record deck verdict after migration: %v", err)
	}
	updated, err := st.Note("note-legacy")
	if err != nil {
		t.Fatal(err)
	}
	if updated.AnkiState != AnkiStateDeckMismatch || updated.AnkiDeck != "Default" ||
		updated.AnkiNoteID != 77 || updated.AnkiPushedRevision != 1 {
		t.Fatalf("note after the deck verdict = %+v", updated)
	}
	// ...and the old rows, the checks and the append-only rules are intact.
	var ledger []string
	rows, err := st.db.Query(`SELECT action || '/' || outcome FROM anki_push_attempts WHERE note_id = 'note-legacy' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var row string
		if err := rows.Scan(&row); err != nil {
			t.Fatal(err)
		}
		ledger = append(ledger, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ledger, ","); got != "add/ok,check/deck_mismatch" {
		t.Fatalf("ledger after migration = %s", got)
	}
	if _, err := st.db.Exec(`UPDATE anki_push_attempts SET outcome = 'failed'`); err == nil {
		t.Fatal("the ledger lost its append-only trigger in the v16 rebuild")
	}
	if _, err := st.db.Exec(`UPDATE notes SET anki_state = 'elsewhere'`); err == nil {
		t.Fatal("the note state check was lost in the v16 rebuild")
	}
	if _, err := st.db.Exec(`UPDATE notes SET anki_deck = ? WHERE note_id = 'note-legacy'`,
		strings.Repeat("d", 201)); err == nil {
		t.Fatal("anki_deck accepted a name over the recorded bound")
	}
}

func TestDeckVerdictKeepsTheQueueAsTheWriteLeftIt(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "notes.sqlite"))
	at := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	target := noteTargetFixture()
	if _, err := st.AcceptNote(target, noteContentFixture("one"), at); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if pending, err := st.PendingNoteOutbox(); err != nil || len(pending) != 1 {
		t.Fatalf("pending after accept = %+v, %v", pending, err)
	}
	if err := st.RecordAnkiAttempt(AnkiAttempt{
		NoteID: target.NoteID, Revision: 1, Action: "add", Outcome: "ok", At: at,
		AnkiNoteID: 77, AnkiState: AnkiStatePushed, PushedRevision: 1, Drain: true,
	}); err != nil {
		t.Fatalf("record write: %v", err)
	}
	if pending, err := st.PendingNoteOutbox(); err != nil || len(pending) != 0 {
		t.Fatalf("pending after the write = %+v, %v", pending, err)
	}
	if err := st.RecordAnkiAttempt(AnkiAttempt{
		NoteID: target.NoteID, Revision: 1, Action: "check", Outcome: "deck_mismatch",
		Detail: `card landed in "Default", not "Pass Quest"`, At: at,
		AnkiDeck: "Default", AnkiState: AnkiStateDeckMismatch, Keep: true,
	}); err != nil {
		t.Fatalf("record deck verdict: %v", err)
	}
	pending, err := st.PendingNoteOutbox()
	if err != nil || len(pending) != 0 {
		t.Fatalf("a kept verdict re-pended the queue: %+v, %v", pending, err)
	}
	outbox, err := st.NoteOutbox()
	if err != nil || len(outbox) != 1 || outbox[0].Status != "drained" || outbox[0].AnkiNoteID != 77 {
		t.Fatalf("queue after the verdict = %+v, %v", outbox, err)
	}
	stored, err := st.Note(target.NoteID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AnkiState != AnkiStateDeckMismatch || stored.AnkiDeck != "Default" ||
		stored.AnkiNoteID != 77 || stored.AnkiPushedRevision != 1 {
		t.Fatalf("note after the verdict = %+v", stored)
	}
	// A verdict that is not a read (a retryable failure) still re-pends the note.
	if err := st.RecordAnkiAttempt(AnkiAttempt{
		NoteID: target.NoteID, Revision: 1, Action: "check", Outcome: "unreachable",
		Detail: "connection refused", At: at, AnkiState: AnkiStateQueued,
	}); err != nil {
		t.Fatalf("record retryable verdict: %v", err)
	}
	if pending, err := st.PendingNoteOutbox(); err != nil || len(pending) != 1 {
		t.Fatalf("retryable verdict did not re-pend the queue: %+v, %v", pending, err)
	}
}
