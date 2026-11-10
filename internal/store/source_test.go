package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/source"
)

var sourceNow = time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)

func testSource(t *testing.T) source.Source {
	t.Helper()
	src := source.Source{
		Title:  "Test baseline",
		Import: source.ImportTXTDir,
		Path:   "/private/test-baseline",
		Rights: source.DefaultRights,
		Notice: "test import notice",
		Pages: []source.Page{
			{Number: 1, Origin: source.PageNative, Section: "vocabulary", Review: source.Unreviewed,
				Flags: []source.Flag{source.FlagReadingOrderUnverified}, Text: "first page text\n", Revision: 1},
			{Number: 2, Origin: source.PageOCR, Section: "mock-exam", Review: source.Unreviewed,
				Flags: []source.Flag{source.FlagOCRUnverified, source.FlagColumnOrderUnverified}, Text: "it will close at 4 p.m.", Revision: 1},
			{Number: 3, Origin: source.PageNone, Review: source.Unreviewed,
				Flags: []source.Flag{source.FlagPageMissing, source.FlagEmpty}, Text: "", Revision: 1},
		},
		CreatedAt: sourceNow,
	}
	src.ExamRelation = source.DefaultExamRelation
	src.ID, src.SHA256 = source.Identity(src)
	return src
}

func questionForTest(t *testing.T, src source.Source) source.Question {
	t.Helper()
	data := []byte(`{
		"id": "q1",
		"stem": "When does the library close on Saturdays?",
		"options": [{"label":"A","text":"At 7 p.m."},{"label":"B","text":"At 4 p.m."}],
		"answer": "B",
		"evidence": "close at 4 p.m.",
		"reasoning": "The page states the Saturday time.",
		"option_reasoning": {"A":"That is the weekday time.","B":"The page states 4 p.m."}
	}`)
	q, err := source.ParseQuestionJSON(data, src.ID, 2, sourceNow, "")
	if err != nil {
		t.Fatalf("ParseQuestionJSON: %v", err)
	}
	return q
}

func TestImportSourceIsIdempotentAndLoadsPages(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "source.sqlite"))
	src := testSource(t)

	created, err := st.ImportSource(src)
	if err != nil {
		t.Fatalf("ImportSource: %v", err)
	}
	if !created {
		t.Fatal("first import reported created=false")
	}
	again, err := st.ImportSource(src)
	if err != nil {
		t.Fatalf("second ImportSource: %v", err)
	}
	if again {
		t.Fatal("second import reported created=true")
	}
	different := testSource(t)
	different.SHA256 = strings.Repeat("b", 64)
	if _, err := st.ImportSource(different); err == nil || !strings.Contains(err.Error(), "different content hash") {
		t.Fatalf("same id with another hash error = %v, want a content-hash rejection", err)
	}

	loaded, err := st.Source(src.ID)
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	if len(loaded.Pages) != 3 {
		t.Fatalf("pages = %d, want 3", len(loaded.Pages))
	}
	if loaded.Pages[1].Text != "it will close at 4 p.m." || loaded.Pages[1].Origin != source.PageOCR {
		t.Fatalf("page 2 = %q/%s", loaded.Pages[1].Text, loaded.Pages[1].Origin)
	}
	if loaded.Pages[0].Section != "vocabulary" || loaded.Pages[1].Section != "mock-exam" {
		t.Fatalf("sections = %q/%q, want vocabulary/mock-exam", loaded.Pages[0].Section, loaded.Pages[1].Section)
	}
	if !loaded.Pages[2].HasFlag(source.FlagPageMissing) {
		t.Fatalf("page 3 flags = %v", loaded.Pages[2].Flags)
	}

	summaries, err := st.Sources()
	if err != nil {
		t.Fatalf("Sources: %v", err)
	}
	if len(summaries) != 1 || summaries[0].PageCount != 3 || summaries[0].ReviewedPages != 0 {
		t.Fatalf("summaries = %+v", summaries)
	}

	events, err := st.Events(src.ID)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(events) != 1 || events[0].Kind != EventImport {
		t.Fatalf("events = %+v", events)
	}

	var revisions int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM page_revisions WHERE source_id = ?`, src.ID).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if revisions != 3 {
		t.Fatalf("page revisions = %d, want 3", revisions)
	}
}

func TestPageRevisionsAndEventsAreAppendOnly(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "source.sqlite"))
	src := testSource(t)
	if _, err := st.ImportSource(src); err != nil {
		t.Fatal(err)
	}

	for _, stmt := range []string{
		`UPDATE page_revisions SET text = 'rewritten' WHERE source_id = ?`,
		`DELETE FROM page_revisions WHERE source_id = ?`,
	} {
		if _, err := st.db.Exec(stmt, src.ID); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Fatalf("%s error = %v, want append-only rejection", stmt, err)
		}
	}
	if _, err := st.db.Exec(`INSERT OR REPLACE INTO page_revisions
		(id, source_id, page, revision, text, origin, created_at, note)
		VALUES (1, ?, 1, 1, 'rewritten', 'manual', '2026-11-10T10:00:00Z', '')`, src.ID); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("INSERT OR REPLACE error = %v, want append-only rejection", err)
	}
	for _, stmt := range []string{
		`UPDATE source_events SET detail = 'rewritten' WHERE source_id = ?`,
		`DELETE FROM source_events WHERE source_id = ?`,
	} {
		if _, err := st.db.Exec(stmt, src.ID); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Fatalf("%s error = %v, want append-only rejection", stmt, err)
		}
	}
	if _, err := st.db.Exec(`INSERT OR REPLACE INTO source_events
		(id, source_id, at, kind, page, question_id, detail)
		VALUES (1, ?, '2026-11-10T10:00:00Z', 'import', 0, '', '{}')`, src.ID); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("event INSERT OR REPLACE error = %v, want append-only rejection", err)
	}

	loaded, err := st.Source(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Pages[0].Text != "first page text\n" {
		t.Fatalf("page text changed: %q", loaded.Pages[0].Text)
	}
}

func TestCorrectPageAppendsRevisionAndResetsReview(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "source.sqlite"))
	src := testSource(t)
	if _, err := st.ImportSource(src); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ReviewPage(src.ID, 2, "owner", "ocr looks right", sourceNow.Add(time.Minute)); err != nil {
		t.Fatalf("ReviewPage: %v", err)
	}

	corrected := strings.Repeat("it will close at 4 p.m. on Saturdays. ", 20)
	page, err := st.CorrectPage(src.ID, 2, corrected, "fixed column order", "owner", sourceNow.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("CorrectPage: %v", err)
	}
	if page.Revision != 2 || page.Origin != source.PageManual || page.Review != source.Unreviewed {
		t.Fatalf("page = revision %d origin %s review %s", page.Revision, page.Origin, page.Review)
	}
	if page.Text != corrected || page.ReviewedBy != "" || !page.ReviewedAt.IsZero() {
		t.Fatalf("page text/review = %q/%q/%s", page.Text, page.ReviewedBy, page.ReviewedAt)
	}
	if !page.HasFlag(source.FlagOCRUnverified) || !page.HasFlag(source.FlagColumnOrderUnverified) || !page.HasFlag(source.FlagCorrected) {
		t.Fatalf("flags = %v, want OCR facts preserved and corrected added", page.Flags)
	}

	var revisions int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM page_revisions WHERE source_id = ? AND page = 2`, src.ID).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if revisions != 2 {
		t.Fatalf("revisions = %d, want 2", revisions)
	}
	var original string
	if err := st.db.QueryRow(`SELECT text FROM page_revisions WHERE source_id = ? AND page = 2 AND revision = 1`, src.ID).Scan(&original); err != nil {
		t.Fatal(err)
	}
	if original != "it will close at 4 p.m." {
		t.Fatalf("revision 1 text = %q, want the imported text", original)
	}

	events, err := st.Events(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[2].Kind != EventPageCorrected || events[2].Page != 2 {
		t.Fatalf("events = %+v", events)
	}
	if !strings.Contains(events[2].Detail, `"previous_revision":1`) {
		t.Fatalf("correction detail = %s", events[2].Detail)
	}
}

func TestQuestionAddReviewEditKeepsAudit(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "source.sqlite"))
	src := testSource(t)
	if _, err := st.ImportSource(src); err != nil {
		t.Fatal(err)
	}
	q := questionForTest(t, src)
	if err := st.AddQuestion(q); err != nil {
		t.Fatalf("AddQuestion: %v", err)
	}
	if err := st.AddQuestion(q); err == nil {
		t.Fatal("duplicate question was accepted")
	}

	loaded, err := st.Question(src.ID, "q1")
	if err != nil {
		t.Fatalf("Question: %v", err)
	}
	if loaded.Answer != "B" || loaded.AnswerStatus != source.AnswerUnmapped || loaded.Review != source.Unreviewed {
		t.Fatalf("new question = answer %q/%s review %s", loaded.Answer, loaded.AnswerStatus, loaded.Review)
	}

	page, err := st.Page(src.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loaded.ReviewAnswer(page, "B", "owner", sourceNow.Add(time.Minute)); !errors.Is(err, source.ErrNotPracticeable) {
		t.Fatalf("review on unreviewed page error = %v", err)
	}
	page, err = st.ReviewPage(src.ID, 2, "owner", "", sourceNow.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	reviewed, err := loaded.ReviewAnswer(page, "B", "owner", sourceNow.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("ReviewAnswer: %v", err)
	}
	if err := st.SaveQuestion(reviewed, EventQuestionReviewed); err != nil {
		t.Fatalf("SaveQuestion: %v", err)
	}
	loaded, err = st.Question(src.ID, "q1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AnswerStatus != source.AnswerReviewed || loaded.Review != source.Reviewed {
		t.Fatalf("reviewed question = %s/%s", loaded.AnswerStatus, loaded.Review)
	}

	replacement, err := source.ParseQuestionJSON([]byte(`{
		"id": "q1",
		"stem": "When does the library close on Saturdays, from 3 March?",
		"options": [{"label":"A","text":"At 7 p.m."},{"label":"B","text":"At 4 p.m."}],
		"answer": "B",
		"evidence": "close at 4 p.m.",
		"reasoning": "The page states the Saturday time.",
		"option_reasoning": {"A":"That is the weekday time.","B":"The page states 4 p.m."}
	}`), src.ID, 2, sourceNow.Add(4*time.Minute), "q1")
	if err != nil {
		t.Fatal(err)
	}
	edited := loaded.Edited(replacement, sourceNow.Add(5*time.Minute))
	if err := st.SaveQuestion(edited, EventQuestionEdited); err != nil {
		t.Fatalf("SaveQuestion edit: %v", err)
	}
	loaded, err = st.Question(src.ID, "q1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AnswerStatus != source.AnswerUnmapped || loaded.Review != source.Unreviewed {
		t.Fatalf("edited question = %s/%s, want review reset", loaded.AnswerStatus, loaded.Review)
	}

	events, err := st.Events(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	want := []string{EventImport, EventQuestionAdded, EventPageReviewed, EventQuestionReviewed, EventQuestionEdited}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("event kinds = %v, want %v", kinds, want)
	}
}

func TestMigrationFromVersionTwoKeepsAttemptsAndAddsSources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v2.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range schemaV1 {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create v1: %v", err)
		}
	}
	for _, stmt := range schemaV2 {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create v2: %v", err)
		}
	}
	started := sourceNow.Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO attempts (
			session_id, question_id, content_id, content_version, status,
			selected_label, selected_text, correct_label, correct_text,
			started_at, answered_at, elapsed_ms
		) VALUES ('sess', 'q1', 'sample-0001', '1', 'correct', 'B', 'At 4 p.m.', 'B', 'At 4 p.m.', ?, ?, 5)`,
		started, started); err != nil {
		t.Fatalf("insert v2 attempt: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 2`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	st := mustOpen(t, path)
	var version int
	if err := st.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != SchemaVersion {
		t.Fatalf("user_version = %d, want %d", version, SchemaVersion)
	}
	list, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].SelectedLabel != "B" {
		t.Fatalf("attempts after v2 migration = %+v", list)
	}
	if _, err := st.ImportSource(testSource(t)); err != nil {
		t.Fatalf("ImportSource after migration: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if err := again.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != SchemaVersion {
		t.Fatalf("user_version after reopen = %d", version)
	}
}

func TestImportSourceRejectsOutOfOrderPages(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "source.sqlite"))
	src := testSource(t)
	src.Pages[1].Number = 4
	src.ID, src.SHA256 = source.Identity(src)
	if _, err := st.ImportSource(src); err == nil || !strings.Contains(err.Error(), "out of order") {
		t.Fatalf("ImportSource error = %v, want out-of-order rejection", err)
	}
}

func TestImportSourceRejectsReviewedPage(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "source.sqlite"))
	src := testSource(t)
	src.Pages[0].Review = source.Reviewed
	src.Pages[0].ReviewedBy = "owner"
	src.Pages[0].ReviewedAt = sourceNow
	if _, err := st.ImportSource(src); err == nil || !strings.Contains(err.Error(), "must start unreviewed") {
		t.Fatalf("ImportSource error = %v, want a reviewed-page rejection", err)
	}
}

func TestAddQuestionRejectsReviewedState(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "source.sqlite"))
	src := testSource(t)
	if _, err := st.ImportSource(src); err != nil {
		t.Fatal(err)
	}
	q := questionForTest(t, src)
	q.Review = source.Reviewed
	q.AnswerStatus = source.AnswerReviewed
	q.ReviewedBy = "owner"
	q.ReviewedAt = sourceNow
	if err := st.AddQuestion(q); err == nil || !strings.Contains(err.Error(), "must start unreviewed") {
		t.Fatalf("AddQuestion error = %v, want an unreviewed-start rejection", err)
	}
}
