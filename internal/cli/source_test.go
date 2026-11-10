package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/store"
)

func fixtureDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", "import"))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func fixtureFile(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(fixtureDir(t), name)
}

func decodeImport(t *testing.T, raw string) importDoc {
	t.Helper()
	var doc importDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode import doc: %v\n%s", err, raw)
	}
	return doc
}

func decodeSources(t *testing.T, raw string) sourcesDoc {
	t.Helper()
	var doc sourcesDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode sources doc: %v\n%s", err, raw)
	}
	return doc
}

func decodeShow(t *testing.T, raw string) showDoc {
	t.Helper()
	var doc showDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode show doc: %v\n%s", err, raw)
	}
	return doc
}

func decodeQuestions(t *testing.T, raw string) questionsDoc {
	t.Helper()
	var doc questionsDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode questions doc: %v\n%s", err, raw)
	}
	return doc
}

func decodePageCommand(t *testing.T, raw string) pageCommandDoc {
	t.Helper()
	var doc pageCommandDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode page command: %v\n%s", err, raw)
	}
	return doc
}

func decodeQuestionCommand(t *testing.T, raw string) questionCommandDoc {
	t.Helper()
	var doc questionCommandDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode question command: %v\n%s", err, raw)
	}
	return doc
}

func importFixture(t *testing.T, db string, clock *stepClock) importDoc {
	t.Helper()
	return decodeImport(t, mustRun(t, db, clock, "", "import", "--path", fixtureDir(t), "--json"))
}

func reviewedFixtureQuestion(t *testing.T, db string, clock *stepClock, sourceID string) {
	t.Helper()
	mustRun(t, db, clock, "", "question", "add", sourceID, "1", "--file", fixtureFile(t, "question-q1.json"), "--json")
	mustRun(t, db, clock, "", "page", "review", sourceID, "1", "--reviewer", "fixture-reviewer", "--json")
	mustRun(t, db, clock, "", "question", "review", sourceID, "q1", "--answer", "B", "--reviewer", "fixture-reviewer", "--json")
}

func TestImportInspectReviewAndPractice(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}

	imported := importFixture(t, db, clock)
	if imported.State != "imported" {
		t.Fatalf("state = %q", imported.State)
	}
	if imported.Source.PageCount != 2 || imported.Source.ReviewedPages != 0 || imported.Source.Origin != "txt-directory" {
		t.Fatalf("import source = %+v", imported.Source)
	}
	sourceID := imported.Source.ID

	again := importFixture(t, db, clock)
	if again.State != "already_imported" || again.Source.ID != sourceID ||
		again.Source.CreatedAt != imported.Source.CreatedAt || again.Source.Title != imported.Source.Title {
		t.Fatalf("second import = %+v, want the stored created_at and title", again)
	}

	sources := decodeSources(t, mustRun(t, db, clock, "", "sources", "--json"))
	if len(sources.Sources) != 1 || sources.Sources[0].PageCount != 2 {
		t.Fatalf("sources = %+v", sources.Sources)
	}

	summary := decodeShow(t, mustRun(t, db, clock, "", "show", sourceID, "--json"))
	if len(summary.Pages) != 2 || summary.Page != nil {
		t.Fatalf("summary pages = %d page = %+v", len(summary.Pages), summary.Page)
	}
	if summary.Pages[0].Origin != "native" || summary.Pages[1].Origin != "ocr" {
		t.Fatalf("page origins = %s/%s", summary.Pages[0].Origin, summary.Pages[1].Origin)
	}
	for _, flag := range []string{"ocr_unverified", "column_order_unverified", "low_text"} {
		if !hasString(summary.Pages[1].Flags, flag) {
			t.Errorf("OCR page flags = %v, want %s", summary.Pages[1].Flags, flag)
		}
	}
	for _, page := range summary.Pages {
		if hasString(page.Flags, "manifest_mismatch") {
			t.Errorf("fixture page %d carries manifest_mismatch: %v", page.Number, page.Flags)
		}
	}

	detail := decodeShow(t, mustRun(t, db, clock, "", "show", sourceID, "--page", "1", "--json"))
	if detail.Page == nil || detail.Page.ReviewStatus != "unreviewed" || !strings.Contains(detail.Page.Text, "closes at 4 p.m.") {
		t.Fatalf("page detail = %+v", detail.Page)
	}

	code, out, errOut := runCLI(t, db, clock, "", "practice", "--json", "--source", sourceID)
	if code != 2 || out != "" || !strings.Contains(errOut, "not_practiceable") {
		t.Fatalf("unreviewed practice = %d, stdout %q, stderr %q", code, out, errOut)
	}

	added := decodeQuestionCommand(t, mustRun(t, db, clock, "", "question", "add", sourceID, "1", "--file", fixtureFile(t, "question-q1.json"), "--json"))
	if added.State != "question_added" || added.Question.AnswerStatus != "unmapped" || added.Question.ReviewStatus != "unreviewed" {
		t.Fatalf("added question = %+v", added)
	}
	code, _, errOut = runCLI(t, db, clock, "", "practice", "--json", "--source", sourceID, "--question", "q1")
	if code != 2 || !strings.Contains(errOut, "not_practiceable") {
		t.Fatalf("unreviewed question practice = %d, %s", code, errOut)
	}

	pageReview := decodePageCommand(t, mustRun(t, db, clock, "", "page", "review", sourceID, "1", "--reviewer", "fixture-reviewer", "--json"))
	if pageReview.Page.ReviewStatus != "reviewed" || pageReview.Page.ReviewedBy != "fixture-reviewer" {
		t.Fatalf("page review = %+v", pageReview.Page)
	}
	code, _, errOut = runCLI(t, db, clock, "", "practice", "--json", "--source", sourceID, "--question", "q1")
	if code != 2 || !strings.Contains(errOut, "not_practiceable") {
		t.Fatalf("question review is still required = %d, %s", code, errOut)
	}

	questionReview := decodeQuestionCommand(t, mustRun(t, db, clock, "", "question", "review", sourceID, "q1", "--answer", "B", "--reviewer", "fixture-reviewer", "--json"))
	if questionReview.State != "question_reviewed" || questionReview.Question.Answer == nil || *questionReview.Question.Answer != "B" {
		t.Fatalf("question review = %+v", questionReview)
	}

	awaitingRaw := mustRun(t, db, clock, "", "practice", "--json", "--source", sourceID, "--question", "q1")
	awaiting := decodeAnswered(t, awaitingRaw)
	if awaiting.State != "awaiting_answer" || awaiting.Question.ID != "q1" {
		t.Fatalf("awaiting = %+v", awaiting)
	}
	wantContentID := sourceID + "-p001"
	if awaiting.Content.ID != wantContentID {
		t.Fatalf("content id = %q, want %q", awaiting.Content.ID, wantContentID)
	}
	if awaiting.Question.Review == nil || awaiting.Question.Review.PageStatus != "reviewed" || awaiting.Question.Review.QuestionStatus != "reviewed" {
		t.Fatalf("review view = %+v", awaiting.Question.Review)
	}
	if awaiting.Question.Review.PageReviewedBy != "fixture-reviewer" || awaiting.Question.Review.QuestionReviewedBy != "fixture-reviewer" {
		t.Fatalf("review provenance = %+v", awaiting.Question.Review)
	}
	var generic any
	if err := json.Unmarshal([]byte(awaitingRaw), &generic); err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	collectKeys(generic, keys)
	for _, forbidden := range []string{"answer", "correct_label", "correct_text", "evidence", "reasoning", "feedback", "status"} {
		if keys[forbidden] {
			t.Errorf("imported awaiting JSON exposes %q", forbidden)
		}
	}
	if awaiting.Question.Total != 1 || awaiting.Question.Number != 1 {
		t.Fatalf("question position = %d/%d, want 1/1", awaiting.Question.Number, awaiting.Question.Total)
	}

	answered := decodeAnswered(t, mustRun(t, db, clock, "", "practice", "--json", "--source", sourceID, "--question", "q1", "--answer", "B"))
	if answered.Attempt.Status != "correct" || answered.Attempt.ContentID != wantContentID || answered.Attempt.Repeat {
		t.Fatalf("answered = %+v", answered.Attempt)
	}
	if !strings.Contains(answered.Content.Provenance.Rights, "Private local source") {
		t.Fatalf("content rights = %q", answered.Content.Provenance.Rights)
	}

	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	if len(report.Contents) != 1 || report.Content == nil || report.Content.ID != wantContentID {
		t.Fatalf("report contents = %+v content = %+v", report.Contents, report.Content)
	}
	if report.Counts.Attempts != 1 || report.Counts.FirstCorrect != 1 {
		t.Fatalf("report counts = %+v", report.Counts)
	}
	if len(report.Questions) != 1 || report.Questions[0].ContentID != wantContentID || report.Questions[0].FirstStatus != "correct" {
		t.Fatalf("report questions = %+v", report.Questions)
	}

	human := mustRun(t, db, clock, "", "report")
	if !strings.Contains(human, wantContentID) || !strings.Contains(human, "First attempts: 1") {
		t.Fatalf("human report lacks imported content:\n%s", human)
	}
	if !strings.Contains(human, " v1-") || strings.Contains(human, "vv") {
		t.Fatalf("human report renders version %q badly:\n%s", report.Content.Version, human)
	}
}

func TestReimportAfterReviewKeepsStoredCounts(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	imported := importFixture(t, db, clock)
	sourceID := imported.Source.ID
	reviewedFixtureQuestion(t, db, clock, sourceID)

	reimported := importFixture(t, db, clock)
	if reimported.State != "already_imported" || reimported.Source.ID != sourceID {
		t.Fatalf("re-import = %+v", reimported)
	}
	if reimported.Source.CreatedAt != imported.Source.CreatedAt || reimported.Source.Title != imported.Source.Title {
		t.Fatalf("re-import changed stored source metadata: %+v", reimported.Source)
	}
	if reimported.Source.ReviewedPages != 1 || reimported.Source.ReviewedQuestions != 1 {
		t.Fatalf("re-import counts = %d reviewed pages, %d reviewed questions", reimported.Source.ReviewedPages, reimported.Source.ReviewedQuestions)
	}
	listed := decodeSources(t, mustRun(t, db, clock, "", "sources", "--json"))
	if len(listed.Sources) != 1 || listed.Sources[0].ReviewedPages != 1 || listed.Sources[0].ReviewedQuestions != 1 {
		t.Fatalf("sources after review = %+v", listed.Sources)
	}
}

func TestQuestionFlagsMarkMissingEvidenceAfterCorrection(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	imported := importFixture(t, db, clock)
	sourceID := imported.Source.ID
	reviewedFixtureQuestion(t, db, clock, sourceID)

	noEvidence := filepath.Join(t.TempDir(), "page-001-no-evidence.txt")
	if err := os.WriteFile(noEvidence, []byte("Westbridge Library — Saturday hours\nThe library is closed on Saturdays.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustRun(t, db, clock, "", "page", "edit", sourceID, "1", "--text-file", noEvidence, "--reviewer", "fixture-reviewer", "--json")
	mustRun(t, db, clock, "", "page", "review", sourceID, "1", "--reviewer", "fixture-reviewer", "--json")

	questions := decodeQuestions(t, mustRun(t, db, clock, "", "questions", sourceID, "--json"))
	if len(questions.Questions) != 1 {
		t.Fatalf("questions = %+v", questions.Questions)
	}
	flags := questions.Questions[0].Flags
	if !hasString(flags, "evidence_missing") {
		t.Fatalf("flags = %v, want evidence_missing after the page correction removed the evidence", flags)
	}
	if hasString(flags, "page_unreviewed") {
		t.Fatalf("flags = %v, want the re-reviewed page to clear page_unreviewed", flags)
	}

	code, _, errOut := runCLI(t, db, clock, "", "practice", "--json", "--source", sourceID, "--question", "q1")
	if code != 2 || !strings.Contains(errOut, "not_practiceable") {
		t.Fatalf("practice with missing evidence = %d, %s", code, errOut)
	}
}

func TestImportTitleFlag(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	imported := decodeImport(t, mustRun(t, db, clock, "", "import", "--path", fixtureDir(t), "--title", "Fixture baseline", "--json"))
	if imported.Source.Title != "Fixture baseline" {
		t.Fatalf("title = %q", imported.Source.Title)
	}
}

func TestPageCorrectionResetsReviewAndKeepsProvenance(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	imported := importFixture(t, db, clock)
	sourceID := imported.Source.ID
	reviewedFixtureQuestion(t, db, clock, sourceID)
	before := decodeAnswered(t, mustRun(t, db, clock, "", "practice", "--json", "--source", sourceID, "--question", "q1", "--answer", "B"))

	corrected := decodePageCommand(t, mustRun(t, db, clock, "", "page", "edit", sourceID, "1",
		"--text-file", fixtureFile(t, "page-001-corrected.txt"), "--reviewer", "fixture-reviewer", "--note", "fixed line wrap", "--json"))
	if corrected.Page.Revision != 2 || corrected.Page.ReviewStatus != "unreviewed" || corrected.Page.Origin != "manual" {
		t.Fatalf("corrected page = %+v", corrected.Page)
	}
	if !hasString(corrected.Page.Flags, "corrected") {
		t.Fatalf("corrected flags = %v", corrected.Page.Flags)
	}
	if !strings.Contains(corrected.Page.Text, "Corrected after review") {
		t.Fatalf("corrected text = %q", corrected.Page.Text)
	}

	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := st.Source(sourceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Pages) != 2 || loaded.Pages[0].Revision != 2 {
		t.Fatalf("stored page = %+v", loaded.Pages[0])
	}
	events, err := st.Events(sourceID)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	var kinds []string
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	wantKinds := []string{store.EventImport, store.EventQuestionAdded, store.EventPageReviewed, store.EventQuestionReviewed, store.EventPageCorrected}
	if strings.Join(kinds, ",") != strings.Join(wantKinds, ",") {
		t.Fatalf("events = %v, want %v", kinds, wantKinds)
	}

	code, _, errOut := runCLI(t, db, clock, "", "practice", "--json", "--source", sourceID, "--question", "q1")
	if code != 2 || !strings.Contains(errOut, "not_practiceable") {
		t.Fatalf("practice after correction = %d, %s", code, errOut)
	}
	staleReport := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	if staleReport.Counts.Attempts != 1 || len(staleReport.Contents) != 1 || staleReport.Content == nil {
		t.Fatalf("report after correction = %+v", staleReport)
	}
	if !strings.Contains(staleReport.Content.Notice, "not practiceable") || staleReport.Content.Version != "unavailable" {
		t.Fatalf("unavailable content = %+v", staleReport.Content)
	}
	staleHuman := mustRun(t, db, clock, "", "report")
	if strings.Contains(staleHuman, "vunavailable") {
		t.Fatalf("human report renders unavailable as a version:\n%s", staleHuman)
	}
	questions := decodeQuestions(t, mustRun(t, db, clock, "", "questions", sourceID, "--json"))
	if len(questions.Questions) != 1 || !hasString(questions.Questions[0].Flags, "page_unreviewed") {
		t.Fatalf("question flags after correction = %+v", questions.Questions)
	}

	mustRun(t, db, clock, "", "page", "review", sourceID, "1", "--reviewer", "fixture-reviewer", "--json")
	answered := decodeAnswered(t, mustRun(t, db, clock, "", "practice", "--json", "--source", sourceID, "--question", "q1", "--answer", "B"))
	if answered.Attempt.Status != "correct" || answered.Attempt.AttemptNumber != 2 || !answered.Attempt.Repeat {
		t.Fatalf("repeated attempt after re-review = %+v", answered.Attempt)
	}
	if answered.Content.ID != before.Content.ID || answered.Content.Version == before.Content.Version {
		t.Fatalf("content after correction = %s/%s, want the same id and a new version than %s",
			answered.Content.ID, answered.Content.Version, before.Content.Version)
	}
}

func TestQuestionEditResetsReviewAndDuplicateIsRejected(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	imported := importFixture(t, db, clock)
	sourceID := imported.Source.ID
	if again := importFixture(t, db, clock); again.State != "already_imported" {
		t.Fatalf("second import state = %q", again.State)
	}
	reviewedFixtureQuestion(t, db, clock, sourceID)
	before := decodeAnswered(t, mustRun(t, db, clock, "", "practice", "--json", "--source", sourceID, "--question", "q1", "--answer", "B"))

	code, _, errOut := runCLI(t, db, clock, "", "question", "add", sourceID, "1", "--file", fixtureFile(t, "question-q1.json"), "--json")
	if code != 2 || !strings.Contains(errOut, "already exists") {
		t.Fatalf("duplicate question = %d, %s", code, errOut)
	}

	original, err := os.ReadFile(fixtureFile(t, "question-q1.json"))
	if err != nil {
		t.Fatal(err)
	}
	editedPath := filepath.Join(t.TempDir(), "question-q1-edited.json")
	if err := os.WriteFile(editedPath, []byte(strings.Replace(string(original), "library close", "library shut", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	edited := decodeQuestionCommand(t, mustRun(t, db, clock, "", "question", "edit", sourceID, "q1", "--file", editedPath, "--json"))
	if edited.State != "question_edited" || edited.Question.ReviewStatus != "unreviewed" || edited.Question.AnswerStatus != "unmapped" {
		t.Fatalf("edited question = %+v", edited.Question)
	}
	if edited.Question.Answer == nil || *edited.Question.Answer != "B" || !hasString(edited.Question.Flags, "answer_unmapped") {
		t.Fatalf("edited proposed answer = %+v, want B visibly unmapped", edited.Question)
	}
	code, _, errOut = runCLI(t, db, clock, "", "practice", "--json", "--source", sourceID, "--question", "q1")
	if code != 2 || !strings.Contains(errOut, "not_practiceable") {
		t.Fatalf("practice after question edit = %d, %s", code, errOut)
	}

	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	loaded, err := st.Source(sourceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Pages) != 2 || loaded.Pages[0].Revision != 1 {
		t.Fatalf("idempotent import changed page revisions: %+v", loaded.Pages)
	}
	st.Close()

	mustRun(t, db, clock, "", "question", "review", sourceID, "q1", "--answer", "B", "--reviewer", "fixture-reviewer", "--json")
	after := decodeAnswered(t, mustRun(t, db, clock, "", "practice", "--json", "--source", sourceID, "--question", "q1", "--answer", "B"))
	if after.Content.ID != before.Content.ID || after.Content.Version == before.Content.Version {
		t.Fatalf("content after question edit = %s/%s, want the same id and a new version than %s",
			after.Content.ID, after.Content.Version, before.Content.Version)
	}
}

func TestSourceCommandsRejectBadInput(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	imported := importFixture(t, db, clock)
	sourceID := imported.Source.ID
	mustRun(t, db, clock, "", "question", "add", sourceID, "1", "--file", fixtureFile(t, "question-q1.json"), "--json")

	cases := []struct {
		name    string
		args    []string
		code    string
		message string
	}{
		{"import without path", []string{"import", "--json"}, "invalid_input", "--path is required"},
		{"import missing path", []string{"import", "--json", "--path", filepath.Join(t.TempDir(), "missing")}, "invalid_input", "invalid import input"},
		{"show unknown source", []string{"show", "src-missing", "--json"}, "invalid_input", "not found"},
		{"practice page without source", []string{"practice", "--json", "--page", "1"}, "invalid_input", "--page requires --source"},
		{"page edit missing reviewer", []string{"page", "edit", sourceID, "1", "--text-file", fixtureFile(t, "page-001-corrected.txt"), "--json"}, "invalid_input", "--reviewer is required"},
		{"page review bad page", []string{"page", "review", sourceID, "0", "--reviewer", "x", "--json"}, "invalid_input", `invalid page "0"`},
		{"question add bad file", []string{"question", "add", sourceID, "1", "--file", filepath.Join(t.TempDir(), "missing.json"), "--json"}, "invalid_input", "missing.json"},
		{"question review missing reviewer", []string{"question", "review", sourceID, "q1", "--answer", "A", "--json"}, "invalid_input", "--reviewer is required"},
		{"question review unknown question", []string{"question", "review", sourceID, "q9", "--answer", "A", "--reviewer", "x", "--json"}, "invalid_input", "not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errOut := runCLI(t, db, clock, "", tc.args...)
			if code != 2 {
				t.Fatalf("exit = %d, want 2\nstdout:%s\nstderr:%s", code, out, errOut)
			}
			if out != "" {
				t.Fatalf("stdout = %q", out)
			}
			var doc errorDoc
			if err := json.Unmarshal([]byte(errOut), &doc); err != nil {
				t.Fatalf("decode error doc: %v\n%s", err, errOut)
			}
			if doc.SchemaVersion != ProtocolVersion || doc.Error.Code != tc.code || !strings.Contains(doc.Error.Message, tc.message) {
				t.Fatalf("error doc = %+v, want code %q containing %q", doc, tc.code, tc.message)
			}
		})
	}
}

func TestImportPDFWithoutLocalToolIsUsageError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.7 placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PASSQUEST_PDFINFO", "passquest-test-missing-pdfinfo")
	t.Setenv("PASSQUEST_PDFTOTEXT", "passquest-test-missing-pdftotext")

	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	code, out, errOut := runCLI(t, db, clock, "", "import", "--json", "--path", path)
	if code != 2 || out != "" {
		t.Fatalf("exit = %d, stdout = %q", code, out)
	}
	if !strings.Contains(errOut, "missing") || !strings.Contains(errOut, "TXT") {
		t.Fatalf("stderr = %s", errOut)
	}
}

func TestImportedSourceSurvivesProcessRestart(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	imported := importFixture(t, db, clock)
	sourceID := imported.Source.ID
	reviewedFixtureQuestion(t, db, clock, sourceID)

	runChild := func(args ...string) (string, string) {
		t.Helper()
		cmd := exec.Command(os.Args[0], args...)
		cmd.Env = append(os.Environ(), "PASSQUEST_TEST_RUN_MAIN=1")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("child %v: %v\nstdout:%s\nstderr:%s", args, err, stdout.String(), stderr.String())
		}
		return stdout.String(), stderr.String()
	}

	shown, _ := runChild("show", sourceID, "--page", "1", "--json", "--db", db)
	detail := decodeShow(t, shown)
	if detail.Page == nil || !strings.Contains(detail.Page.Text, "closes at 4 p.m.") || detail.Page.ReviewStatus != "reviewed" {
		t.Fatalf("child show = %+v", detail.Page)
	}
	answeredRaw, _ := runChild("practice", "--json", "--source", sourceID, "--question", "q1", "--answer", "B", "--db", db)
	answered := decodeAnswered(t, answeredRaw)
	if answered.Attempt.Status != "correct" {
		t.Fatalf("child answered = %+v", answered.Attempt)
	}

	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	if report.Counts.Attempts != 1 || report.Counts.FirstCorrect != 1 {
		t.Fatalf("report after child process = %+v", report.Counts)
	}
}

func TestReportShowsMixedContentsTruthfully(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	imported := importFixture(t, db, clock)
	sourceID := imported.Source.ID
	reviewedFixtureQuestion(t, db, clock, sourceID)
	mustRun(t, db, clock, "", "practice", "--json", "--source", sourceID, "--question", "q1", "--answer", "B")
	mustRun(t, db, clock, "", "practice", "--json", "--question", "q1", "--answer", "B")

	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	if report.Content != nil {
		t.Fatalf("singular content = %+v, want omitted for mixed contents", report.Content)
	}
	if len(report.Contents) != 2 {
		t.Fatalf("contents = %d, want 2", len(report.Contents))
	}
	ids := map[string]bool{}
	for _, content := range report.Contents {
		ids[content.ID] = true
	}
	if !ids[sourceID+"-p001"] || !ids["sample-0001"] {
		t.Fatalf("content ids = %v", ids)
	}
	if report.Counts.Attempts != 2 || report.Counts.FirstCorrect != 2 {
		t.Fatalf("counts = %+v", report.Counts)
	}
	human := mustRun(t, db, clock, "", "report")
	for _, want := range []string{"attempt evidence (2 contents)", "Contents", sourceID + "-p001", "sample-0001"} {
		if !strings.Contains(human, want) {
			t.Errorf("human report lacks %q:\n%s", want, human)
		}
	}
}

func TestDefaultDatabaseDirectoryIsTightened(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(".passquest", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(".passquest", 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"sources", "--json"}, Options{
		Stdout: &stdout,
		Stderr: &stderr,
		Now:    (&stepClock{base: goldenBase}).Now,
		Getenv: func(string) string { return "" },
	})
	if code != 0 {
		t.Fatalf("sources = %d, stderr %s", code, stderr.String())
	}
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(".passquest")
	if err != nil {
		t.Fatalf("stat default directory: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("default directory mode = %04o, want 0700", perm)
	}
}

func TestSourceProtocolGoldens(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	fixture := fixtureDir(t)
	replacements := map[string]string{fixture: "<fixture>"}
	check := func(name, raw string) {
		t.Helper()
		checkGolden(t, filepath.Join("testdata", name), normalizedGolden(t, raw, replacements))
	}

	importedRaw := mustRun(t, db, clock, "", "import", "--path", fixture, "--json")
	check("source-import.golden.json", importedRaw)
	sourceID := decodeImport(t, importedRaw).Source.ID

	check("source-sources.golden.json", mustRun(t, db, clock, "", "sources", "--json"))
	check("source-show.golden.json", mustRun(t, db, clock, "", "show", sourceID, "--json"))
	check("source-show-page.golden.json", mustRun(t, db, clock, "", "show", sourceID, "--page", "1", "--json"))
	check("source-questions.golden.json", mustRun(t, db, clock, "", "questions", sourceID, "--json"))

	check("source-question-add.golden.json", mustRun(t, db, clock, "", "question", "add", sourceID, "1", "--file", fixtureFile(t, "question-q1.json"), "--json"))
	check("source-questions-unmapped.golden.json", mustRun(t, db, clock, "", "questions", sourceID, "--json"))
	check("source-page-review.golden.json", mustRun(t, db, clock, "", "page", "review", sourceID, "1", "--reviewer", "fixture-reviewer", "--json"))
	check("source-question-review.golden.json", mustRun(t, db, clock, "", "question", "review", sourceID, "q1", "--answer", "B", "--reviewer", "fixture-reviewer", "--json"))
	check("source-questions-reviewed.golden.json", mustRun(t, db, clock, "", "questions", sourceID, "--json"))

	original, err := os.ReadFile(fixtureFile(t, "question-q1.json"))
	if err != nil {
		t.Fatal(err)
	}
	editedPath := filepath.Join(t.TempDir(), "question-q1-edited.json")
	if err := os.WriteFile(editedPath, []byte(strings.Replace(string(original), "library close", "library shut", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	check("source-question-edit.golden.json", mustRun(t, db, clock, "", "question", "edit", sourceID, "q1", "--file", editedPath, "--json"))
	check("source-page-edit.golden.json", mustRun(t, db, clock, "", "page", "edit", sourceID, "1", "--text-file", fixtureFile(t, "page-001-corrected.txt"), "--reviewer", "fixture-reviewer", "--note", "fixed line wrap", "--json"))
}

// normalizedGolden pins the protocol's keys and values; the fixture path is the
// only machine-specific value, so it is replaced before comparison.
func normalizedGolden(t *testing.T, raw string, replacements map[string]string) []byte {
	t.Helper()
	var doc any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode protocol document: %v\n%s", err, raw)
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(replaceGoldenStrings(doc, replacements)); err != nil {
		t.Fatalf("encode protocol document: %v", err)
	}
	return out.Bytes()
}

func replaceGoldenStrings(value any, replacements map[string]string) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			typed[key] = replaceGoldenStrings(child, replacements)
		}
		return typed
	case []any:
		for i, child := range typed {
			typed[i] = replaceGoldenStrings(child, replacements)
		}
		return typed
	case string:
		if replacement, ok := replacements[typed]; ok {
			return replacement
		}
		return typed
	default:
		return value
	}
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestReportMarksUnknownContentUnavailable(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sample, err := exam.Sample()
	if err != nil {
		t.Fatal(err)
	}
	q := sample.Questions[0]
	opt, ok := q.Option(q.Answer)
	if !ok {
		t.Fatalf("sample question %s has no keyed option", q.ID)
	}
	record, err := attempt.BuildRecord("legacy-content", "1-deadbeef", q,
		attempt.Selection{Option: &opt}, "sess-test", goldenBase, goldenBase.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Append(record); err != nil {
		st.Close()
		t.Fatal(err)
	}
	st.Close()

	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	if len(report.Contents) != 1 || report.Contents[0].ID != "legacy-content" || report.Contents[0].Version != "unavailable" {
		t.Fatalf("contents = %+v", report.Contents)
	}
	if !strings.Contains(report.Contents[0].Notice, "unrecognised content id") {
		t.Fatalf("unavailable notice = %q", report.Contents[0].Notice)
	}
	if report.Counts.Attempts != 1 || len(report.Attempts) != 1 || report.Attempts[0].ContentID != "legacy-content" {
		t.Fatalf("attempt retention = counts %+v, attempts %+v", report.Counts, report.Attempts)
	}
}
