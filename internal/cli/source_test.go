package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wxxb789/passquest/internal/source"
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
	if again.State != "already_imported" || again.Source.ID != sourceID {
		t.Fatalf("second import = %+v", again)
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
	wantContentID := source.ContentID(sourceID, 1)
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
	if !strings.Contains(human, "v"+report.Content.Version) || strings.Contains(human, "vv") {
		t.Fatalf("human report renders version %q badly:\n%s", report.Content.Version, human)
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
	mustRun(t, db, clock, "", "practice", "--json", "--source", sourceID, "--question", "q1", "--answer", "B")

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

	code, _, errOut := runCLI(t, db, clock, "", "question", "add", sourceID, "1", "--file", fixtureFile(t, "question-q1.json"), "--json")
	if code != 2 || !strings.Contains(errOut, "already exists") {
		t.Fatalf("duplicate question = %d, %s", code, errOut)
	}

	edited := decodeQuestionCommand(t, mustRun(t, db, clock, "", "question", "edit", sourceID, "q1", "--file", fixtureFile(t, "question-q1.json"), "--json"))
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
}

func TestSourceCommandsRejectBadInput(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	cases := []struct {
		name string
		args []string
	}{
		{"import without path", []string{"import", "--json"}},
		{"import missing path", []string{"import", "--json", "--path", filepath.Join(t.TempDir(), "missing")}},
		{"show unknown source", []string{"show", "src-missing", "--json"}},
		{"page without source", []string{"practice", "--json", "--page", "1"}},
		{"page edit missing reviewer", []string{"page", "edit", "src-missing", "1", "--text-file", fixtureFile(t, "page-001-corrected.txt"), "--json"}},
		{"page review bad page", []string{"page", "review", "src-missing", "0", "--reviewer", "x", "--json"}},
		{"question add bad file", []string{"question", "add", "src-missing", "1", "--file", filepath.Join(t.TempDir(), "missing.json"), "--json"}},
		{"question review missing", []string{"question", "review", "src-missing", "q1", "--answer", "A", "--reviewer", "x", "--json"}},
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
			if doc.SchemaVersion != ProtocolVersion || doc.Error.Message == "" {
				t.Fatalf("error doc = %+v", doc)
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
	if !ids[source.ContentID(sourceID, 1)] || !ids["sample-0001"] {
		t.Fatalf("content ids = %v", ids)
	}
	if report.Counts.Attempts != 2 || report.Counts.FirstCorrect != 2 {
		t.Fatalf("counts = %+v", report.Counts)
	}
	human := mustRun(t, db, clock, "", "report")
	for _, want := range []string{"attempt evidence (2 contents)", "Contents", source.ContentID(sourceID, 1), "sample-0001"} {
		if !strings.Contains(human, want) {
			t.Errorf("human report lacks %q:\n%s", want, human)
		}
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
