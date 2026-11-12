package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/adaptive"
	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
)

func assessmentFixtureDir(t *testing.T, pages int) string {
	t.Helper()
	dir := t.TempDir()
	pagesDir := filepath.Join(dir, "pages")
	if err := os.MkdirAll(pagesDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for page := 1; page <= pages; page++ {
		text := fmt.Sprintf("Page %d. The library closes at 4 p.m. on Saturdays. It opens at 9 a.m. on weekdays.\n", page)
		if err := os.WriteFile(filepath.Join(pagesDir, fmt.Sprintf("page-%03d.txt", page)), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func seedAssessmentSource(t *testing.T, db string, pages int, firstPageQuestions int) string {
	t.Helper()
	dir := assessmentFixtureDir(t, pages)
	src, err := source.ImportPath(dir, source.ImportOptions{Title: "assessment fixture", Now: goldenBase})
	if err != nil {
		t.Fatalf("import fixture: %v", err)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.ImportSource(src); err != nil {
		t.Fatalf("store import: %v", err)
	}
	for pageNumber := 1; pageNumber <= pages; pageNumber++ {
		count := 1
		if pageNumber == 1 {
			count = firstPageQuestions
		}
		if _, err := st.ReviewPage(src.ID, pageNumber, "fixture-reviewer", "", goldenBase); err != nil {
			t.Fatalf("review page %d: %v", pageNumber, err)
		}
		page, err := st.Page(src.ID, pageNumber)
		if err != nil {
			t.Fatal(err)
		}
		for index := 1; index <= count; index++ {
			addReviewedFixtureQuestion(t, st, page, src.ID, pageNumber, index)
		}
	}
	return src.ID
}

// addReviewedFixtureQuestion authors and review-approves one fixture question
// on an already reviewed page. It is used by the seed helper and by tests that
// add a question to a page the learner already viewed.
func addReviewedFixtureQuestion(t *testing.T, st *store.Store, page source.Page, sourceID string, pageNumber, index int) {
	t.Helper()
	questionID := fmt.Sprintf("p%02d-q%d", pageNumber, index)
	tag := "vocabulary-in-context"
	if index == 1 {
		tag = "not-except-polarity"
	}
	q := source.Question{
		SourceID: sourceID,
		ID:       questionID,
		Page:     pageNumber,
		Stem:     fmt.Sprintf("Page %d question %d: When does the library close on Saturdays?", pageNumber, index),
		Options: []exam.Option{
			{Label: "A", Text: "At 9 a.m."},
			{Label: "B", Text: "At 4 p.m."},
		},
		Evidence:        "closes at 4 p.m. on Saturdays",
		Reasoning:       fmt.Sprintf("Reasoning for page %d question %d.", pageNumber, index),
		OptionReasoning: map[string]string{"A": "Nine is the weekday opening time.", "B": "The passage states 4 p.m. on Saturdays."},
		Tactics:         []string{tag},
		Glossary: []exam.GlossaryEntry{
			{Term: "closes", Meaning: "shuts"},
		},
		AnswerStatus: source.AnswerUnmapped,
		Review:       source.Unreviewed,
		CreatedAt:    goldenBase,
		UpdatedAt:    goldenBase,
	}
	if err := st.AddQuestion(q); err != nil {
		t.Fatalf("add question %s: %v", questionID, err)
	}
	reviewed, err := q.ReviewAnswer(page, "B", "fixture-reviewer", goldenBase)
	if err != nil {
		t.Fatalf("review answer %s: %v", questionID, err)
	}
	if err := st.SaveQuestion(reviewed, store.EventQuestionReviewed); err != nil {
		t.Fatalf("save question %s: %v", questionID, err)
	}
}

func decodePool(t *testing.T, raw string) poolDoc {
	t.Helper()
	var doc poolDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode pool: %v\n%s", err, raw)
	}
	return doc
}

func decodeRunResult(t *testing.T, raw string) assessmentRunResultDoc {
	t.Helper()
	var doc assessmentRunResultDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode run result: %v\n%s", err, raw)
	}
	return doc
}

func decodeAssessReport(t *testing.T, raw string) assessReportDoc {
	t.Helper()
	var doc assessReportDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode assess report: %v\n%s", err, raw)
	}
	return doc
}

func activeAssessmentID(t *testing.T, db string) int64 {
	t.Helper()
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	run, found, err := st.ActiveAssessmentRun()
	if err != nil || !found {
		t.Fatalf("active run = %+v, %v", run, err)
	}
	return run.ID
}

func TestAssessNextSpendsRecommendedUnit(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	seedAssessmentSource(t, db, 4, 1)
	mustRun(t, db, clock, "", "next", "--json")

	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	input, _, _, err := buildAdaptiveInput(st, "")
	if err != nil {
		t.Fatal(err)
	}
	result := adaptive.Recommend(input)
	if result.Recommendation == nil {
		t.Fatal("no recommendation")
	}
	material := result.Recommendation.FreshMaterial
	if material == nil {
		material = result.Recommendation.ReviewMaterial
	}
	if material == nil {
		t.Fatal("recommendation names no material")
	}

	spends, err := st.Spends()
	if err != nil {
		t.Fatal(err)
	}
	var next []assess.Spend
	for _, spend := range spends {
		if spend.Surface == assess.SurfaceNext {
			next = append(next, spend)
		}
	}
	if len(next) != 1 || next[0].ContentID != material.ContentID || next[0].QuestionID != material.QuestionID {
		t.Fatalf("next spends = %+v, recommended %s/%s", next, material.ContentID, material.QuestionID)
	}
}

func TestAssessPoolSpendsWholeContentUnit(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := seedAssessmentSource(t, db, 20, 4)

	pool := decodePool(t, mustRun(t, db, clock, "", "assess", "pool", "--json"))
	if pool.QualifyingItems != 23 || pool.QualifyingUnits != 20 || pool.SpentUnits != 0 {
		t.Fatalf("fresh pool = %+v", pool)
	}
	mustRun(t, db, clock, "", "practice", "--source", sourceID, "--page", "1", "--question", "p01-q1", "--answer", "B", "--json")

	pool = decodePool(t, mustRun(t, db, clock, "", "assess", "pool", "--json"))
	if pool.QualifyingItems != 19 || pool.QualifyingUnits != 19 || pool.SpentUnits != 1 {
		t.Fatalf("pool after one exposure = %+v", pool)
	}
	var spent, fresh *poolUnitView
	for index := range pool.ContentUnits {
		unit := &pool.ContentUnits[index]
		if unit.ContentID == sourceID+"-p001" {
			spent = unit
		}
		if unit.ContentID == sourceID+"-p002" {
			fresh = unit
		}
	}
	if spent == nil || !spent.Spent || spent.QuestionCount != 4 {
		t.Fatalf("spent unit = %+v", spent)
	}
	if len(spent.SpentBy) < 2 || spent.SpentBy[0] != "attempt" || spent.SpentBy[1] != "practice" {
		t.Fatalf("spent by = %+v", spent.SpentBy)
	}
	if fresh == nil || fresh.Spent || fresh.QuestionCount != 1 {
		t.Fatalf("fresh unit = %+v", fresh)
	}
}

func TestAssessStartRefusesFloorAndShortfall(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	seedAssessmentSource(t, db, 4, 1)
	code, _, errOut := runCLI(t, db, clock, "", "assess", "start", "--size", "20", "--json")
	if code != 2 || !strings.Contains(errOut, `"code": "pool_floor"`) || !strings.Contains(errOut, "4 qualifying item(s)") || !strings.Contains(errOut, "16 more") {
		t.Fatalf("pool floor = %d, %s", code, errOut)
	}

	db2 := newDB(t)
	clock2 := &stepClock{base: goldenBase}
	seedAssessmentSource(t, db2, 20, 1)
	code, _, errOut = runCLI(t, db2, clock2, "", "assess", "start", "--size", "21", "--json")
	if code != 2 || !strings.Contains(errOut, `"code": "pool_short"`) || !strings.Contains(errOut, "1 more") {
		t.Fatalf("pool short = %d, %s", code, errOut)
	}
	code, _, errOut = runCLI(t, db2, clock2, "", "assess", "start", "--annotations", "show", "--json")
	if code != 2 || !strings.Contains(errOut, `"code": "invalid_annotations"`) {
		t.Fatalf("annotations = %d, %s", code, errOut)
	}
}

func TestAssessRunHidesAnnotationsAndReportsThem(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	seedAssessmentSource(t, db, 20, 1)
	answers := strings.Repeat("B\n", 20)
	code, out, errOut := runCLI(t, db, clock, answers, "assess", "start", "--size", "20", "--minutes", "60", "--json")
	if code != 0 {
		t.Fatalf("start = %d, %s", code, errOut)
	}
	if strings.Contains(out, "Reasoning for") || strings.Contains(out, "shuts") || strings.Contains(out, "closes at 4 p.m.") {
		t.Fatalf("run transcript exposed annotations:\n%s", out)
	}
	runDoc := decodeRunResult(t, out)
	if runDoc.State != "completed" || len(runDoc.Items) != 20 {
		t.Fatalf("run = %+v", runDoc)
	}
	reportRaw := mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runDoc.Run.ID), "--json")
	report := decodeAssessReport(t, reportRaw)
	if report.Counts.Total != 20 || report.Accuracy.Correct != 20 || report.Counts.Scored != 20 {
		t.Fatalf("report counts = %+v accuracy=%+v", report.Counts, report.Accuracy)
	}
	if len(report.Items) != 20 {
		t.Fatalf("report items = %d", len(report.Items))
	}
	first := report.Items[0]
	if !first.Annotations.Available || !strings.Contains(first.Annotations.Reasoning, "Reasoning for") {
		t.Fatalf("annotations = %+v", first.Annotations)
	}
	if len(first.Annotations.Glossary) != 1 || first.Annotations.Glossary[0].Meaning != "shuts" {
		t.Fatalf("glossary = %+v", first.Annotations.Glossary)
	}
	if !containsExact(report.Unknowns, "pass threshold: unknown") {
		t.Fatalf("unknowns = %+v", report.Unknowns)
	}
	if strings.Contains(strings.ToLower(reportRaw), `"verdict"`) || strings.Contains(strings.ToLower(reportRaw), `"pass":`) {
		t.Fatalf("report invented a verdict:\n%s", reportRaw)
	}
	human := mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runDoc.Run.ID))
	if !strings.Contains(human, "pass threshold: unknown") || !strings.Contains(human, "Glossary: closes — shuts") {
		t.Fatalf("human report missing unknowns or annotations:\n%s", human)
	}
	if strings.Contains(human, "Result: PASS") || strings.Contains(human, "Result: FAIL") {
		t.Fatalf("human report printed a verdict:\n%s", human)
	}
}

func TestAssessReportWithholdsAnnotationsUntilRunFinishes(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	seedAssessmentSource(t, db, 20, 1)
	code, _, errOut := runCLI(t, db, clock, "B\n", "assess", "start", "--size", "20", "--minutes", "60", "--json")
	if code != 4 {
		t.Fatalf("interrupted start = %d, %s", code, errOut)
	}
	runID := activeAssessmentID(t, db)
	raw := mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runID), "--json")
	report := decodeAssessReport(t, raw)
	if report.Run.Status != assess.StatusActive || len(report.Items) != 20 {
		t.Fatalf("active report = %+v items=%d", report.Run, len(report.Items))
	}
	for _, item := range report.Items {
		if !item.Annotations.Withheld {
			t.Fatalf("active run exposed annotations for %s: %+v", item.QuestionID, item.Annotations)
		}
		if item.Annotations.Available || item.Annotations.Evidence != "" || item.Annotations.Reasoning != "" ||
			len(item.Annotations.OptionReasoning) != 0 || len(item.Annotations.Glossary) != 0 {
			t.Fatalf("active run item %s carries annotation content: %+v", item.QuestionID, item.Annotations)
		}
	}
	for _, key := range []string{"Reasoning for", "shuts", `"evidence"`, `"reasoning"`, `"option_reasoning"`, `"glossary"`} {
		if strings.Contains(raw, key) {
			t.Fatalf("active report leaked %q:\n%s", key, raw)
		}
	}
	human := mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runID))
	if !strings.Contains(human, fmt.Sprintf("Annotations: withheld until the run is finished (run %d is active)", runID)) {
		t.Fatalf("active human report does not state the withholding:\n%s", human)
	}
	for _, leaked := range []string{"Evidence:", "Reasoning:", "Option A:", "Glossary:"} {
		if strings.Contains(human, leaked) {
			t.Fatalf("active human report leaked %q:\n%s", leaked, human)
		}
	}

	mustRun(t, db, clock, strings.Repeat("B\n", 19), "assess", "resume", fmt.Sprintf("%d", runID), "--json")
	finished := decodeAssessReport(t, mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runID), "--json"))
	if finished.Run.Status != assess.StatusCompleted || len(finished.Items) != 20 {
		t.Fatalf("finished report = %+v items=%d", finished.Run, len(finished.Items))
	}
	for _, item := range finished.Items {
		if item.Annotations.Withheld || !item.Annotations.Available || !strings.Contains(item.Annotations.Reasoning, "Reasoning for") {
			t.Fatalf("finished run item %s did not expose its annotations: %+v", item.QuestionID, item.Annotations)
		}
	}
	human = mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runID))
	if strings.Contains(human, "withheld until the run is finished") || !strings.Contains(human, "Reasoning: Reasoning for page 1 question 1.") {
		t.Fatalf("finished human report did not expose annotations:\n%s", human)
	}
}

func TestAssessmentReportPrintsMissingOptionExplanation(t *testing.T) {
	item := assess.ItemReport{
		Position:   0,
		ContentID:  "content-1",
		QuestionID: "q1",
		State:      assess.StateAnswered,
		Scored:     true,
		Annotations: assess.Annotations{
			Evidence:               "closes at 4 p.m.",
			Reasoning:              "The notice states the time.",
			OptionReasoning:        map[string]string{"B": "The key is 4 p.m."},
			OptionReasoningMissing: []string{"A", "C"},
		},
	}
	view := reportItemViewOf(item)
	if got := strings.Join(view.Annotations.OptionReasoningMissing, ","); got != "A,C" {
		t.Fatalf("option_reasoning_missing = %q, want A,C", got)
	}
	if len(view.Annotations.OptionReasoning) != 1 || view.Annotations.OptionReasoning["B"] == "" {
		t.Fatalf("option_reasoning = %v, want only B", view.Annotations.OptionReasoning)
	}
	var out bytes.Buffer
	writeAssessmentReport(&out, assessReportDoc{Items: []assessReportItemView{view}}, "test.sqlite")
	text := out.String()
	for _, label := range []string{"A", "C"} {
		if !strings.Contains(text, "    Option "+label+": "+missingOptionExplanation+"\n") {
			t.Fatalf("report does not print the missing explanation for %s:\n%s", label, text)
		}
	}
	if !strings.Contains(text, "    Option B: The key is 4 p.m.\n") {
		t.Fatalf("report does not print the authored explanation:\n%s", text)
	}
}

func TestAssessAbandonedRunWithholdsUnfinalizedAnnotations(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	seedAssessmentSource(t, db, 20, 1)
	code, _, errOut := runCLI(t, db, clock, "B\nB\nB\n", "assess", "start", "--size", "20", "--minutes", "60", "--json")
	if code != 4 {
		t.Fatalf("interrupted start = %d, %s", code, errOut)
	}
	runID := activeAssessmentID(t, db)
	mustRun(t, db, clock, "", "assess", "abandon", fmt.Sprintf("%d", runID), "--json")
	report := decodeAssessReport(t, mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runID), "--json"))
	if report.Run.Status != assess.StatusAbandoned {
		t.Fatalf("run status = %q", report.Run.Status)
	}
	pending, presented, answered := 0, 0, 0
	for _, item := range report.Items {
		switch item.State {
		case assess.StatePending:
			pending++
		case assess.StatePresented:
			presented++
		case assess.StateAnswered:
			answered++
		}
		if item.State == assess.StateAnswered {
			if item.Annotations.Withheld || !item.Annotations.Available {
				t.Fatalf("answered item %s = %+v", item.QuestionID, item.Annotations)
			}
			continue
		}
		if !item.Annotations.Withheld || item.Annotations.Available || item.Annotations.Reasoning != "" ||
			item.Annotations.Evidence != "" || len(item.Annotations.Glossary) != 0 || len(item.Annotations.OptionReasoning) != 0 {
			t.Fatalf("unfinalized item %s (%s) = %+v", item.QuestionID, item.State, item.Annotations)
		}
	}
	if pending != 16 || presented != 1 || answered != 3 {
		t.Fatalf("item states = pending %d, presented %d, answered %d", pending, presented, answered)
	}
	human := mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runID))
	if !strings.Contains(human, "Annotations: withheld (item was never finalized)") || strings.Contains(human, "withheld until the run is finished") {
		t.Fatalf("abandoned human report = \n%s", human)
	}
}

func TestShowPageWithoutQuestionsSpendsItsUnit(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := seedAssessmentSource(t, db, 20, 0)
	pool := decodePool(t, mustRun(t, db, clock, "", "assess", "pool", "--json"))
	if pool.QualifyingUnits != 19 || pool.PracticeableUnits != 19 {
		t.Fatalf("pool with an empty page = %+v", pool)
	}
	shown := mustRun(t, db, clock, "", "show", sourceID, "--page", "1", "--json")
	if strings.Contains(shown, "p01-q1") {
		t.Fatalf("page 1 unexpectedly lists a question:\n%s", shown)
	}

	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	spends, err := st.Spends()
	if err != nil {
		st.Close()
		t.Fatal(err)
	}
	var pageRows []assess.Spend
	for _, spend := range spends {
		if spend.ContentID == sourceID+"-p001" && spend.Surface == assess.SurfaceShow {
			pageRows = append(pageRows, spend)
		}
	}
	if len(pageRows) != 1 || pageRows[0].QuestionID != "" {
		st.Close()
		t.Fatalf("page presentation rows = %+v", pageRows)
	}
	page, err := st.Page(sourceID, 1)
	if err != nil {
		st.Close()
		t.Fatal(err)
	}
	addReviewedFixtureQuestion(t, st, page, sourceID, 1, 1)
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	pool = decodePool(t, mustRun(t, db, clock, "", "assess", "pool", "--json"))
	if pool.PracticeableUnits != 20 || pool.QualifyingUnits != 19 || pool.QualifyingItems != 19 {
		t.Fatalf("pool after authoring on the shown page = %+v", pool)
	}
	var unit *poolUnitView
	for index := range pool.ContentUnits {
		if pool.ContentUnits[index].ContentID == sourceID+"-p001" {
			unit = &pool.ContentUnits[index]
		}
	}
	if unit == nil || !unit.Spent || unit.QuestionCount != 1 || !containsExact(unit.SpentBy, assess.SurfaceShow) {
		t.Fatalf("shown page unit = %+v", unit)
	}
}

func TestAssessRefusesCoachingWhileActive(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	seedAssessmentSource(t, db, 20, 1)
	code, _, errOut := runCLI(t, db, clock, "B\n", "assess", "start", "--size", "20", "--minutes", "60", "--json")
	if code != 4 || !strings.Contains(errOut, "run_interrupted") {
		t.Fatalf("interrupted start = %d, %s", code, errOut)
	}
	runID := activeAssessmentID(t, db)
	for _, args := range [][]string{
		{"tutor", "history", "--json"},
		{"skill", "show", "vocabulary-in-context", "--json"},
		{"find", "--json"},
	} {
		code, _, errOut = runCLI(t, db, clock, "", args...)
		if code != 2 || !strings.Contains(errOut, `"code": "assessment_active"`) || !strings.Contains(errOut, fmt.Sprintf("%d", runID)) {
			t.Fatalf("%v = %d, %s", args, code, errOut)
		}
	}
	mustRun(t, db, clock, "", "assess", "abandon", fmt.Sprintf("%d", runID), "--json")
	code, out, errOut := runCLI(t, db, clock, "", "find", "--json")
	if code != 0 || strings.Contains(errOut, "assessment_active") {
		t.Fatalf("coaching after abandon = %d, %s, %s", code, out, errOut)
	}
}

type onceAdvanceReader struct {
	mu      sync.Mutex
	data    []byte
	once    sync.Once
	advance func()
}

func (r *onceAdvanceReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	r.once.Do(r.advance)
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func TestAssessForcedItemTimeout(t *testing.T) {
	db := newDB(t)
	seedAssessmentSource(t, db, 20, 1)
	now := goldenBase
	reader := &onceAdvanceReader{data: []byte("B\nB\nB\nB\n"), advance: func() { now = now.Add(40 * time.Second) }}
	var stdout, stderr bytes.Buffer
	opt := Options{
		Stdin:      reader,
		Stdout:     &stdout,
		Stderr:     &stderr,
		Now:        func() time.Time { return now },
		NewSession: func() string { return "sess-test" },
		Getenv:     func(string) string { return "" },
	}
	code := Run([]string{"assess", "start", "--size", "4", "--minutes", "2", "--json", "--db", db}, opt)
	if code != 0 {
		t.Fatalf("timeout run = %d, %s", code, stderr.String())
	}
	runDoc := decodeRunResult(t, stdout.String())
	report := decodeAssessReport(t, mustRun(t, db, &stepClock{base: goldenBase.Add(time.Hour)}, "", "assess", "report", fmt.Sprintf("%d", runDoc.Run.ID), "--json"))
	if report.Counts.TimedOut != 1 || report.Counts.Total != 4 || report.Accuracy.Correct != 3 {
		t.Fatalf("timeout report = %+v accuracy=%+v", report.Counts, report.Accuracy)
	}
}

func TestAssessRunWindowElapseRecordsOpenItemTimeout(t *testing.T) {
	db := newDB(t)
	seedAssessmentSource(t, db, 20, 1)
	now := goldenBase
	reader := &lineAdvanceReader{
		lines:     []string{"B\n", "B\n", "B\n", "B\n"},
		advanceAt: 3,
		advance:   func() { now = goldenBase.Add(2 * time.Minute) },
	}
	var stdout, stderr bytes.Buffer
	opt := Options{
		Stdin:      reader,
		Stdout:     &stdout,
		Stderr:     &stderr,
		Now:        func() time.Time { return now },
		NewSession: func() string { return "sess-test" },
		Getenv:     func(string) string { return "" },
	}
	code := Run([]string{"assess", "start", "--size", "4", "--minutes", "1", "--json", "--db", db}, opt)
	if code != 2 || !strings.Contains(stderr.String(), `"code": "run_abandoned"`) || !strings.Contains(stderr.String(), "assess report") {
		t.Fatalf("expired run = %d, stdout: %s, stderr: %s", code, stdout.String(), stderr.String())
	}
	report := decodeAssessReport(t, mustRun(t, db, &stepClock{base: goldenBase.Add(time.Hour)}, "", "assess", "report", "1", "--json"))
	if report.Run.Status != "abandoned" || report.Run.AbandonedReason != "window_elapsed" {
		t.Fatalf("abandoned run = %+v", report.Run)
	}
	answered, timedOut, presented := 0, 0, 0
	for _, item := range report.Items {
		switch item.State {
		case "answered":
			answered++
		case "timed_out":
			timedOut++
		case "presented":
			presented++
		case "pending", "unavailable":
		default:
			t.Fatalf("item %d has unknown state %q", item.Position, item.State)
		}
	}
	if presented != 0 || answered != 3 || timedOut != 1 {
		t.Fatalf("item states: answered=%d timed_out=%d presented=%d, items=%+v", answered, timedOut, presented, report.Items)
	}
	if report.Counts.Presented != 4 || report.Counts.TimedOut != 1 || report.Counts.Finalized != 4 {
		t.Fatalf("counts = %+v", report.Counts)
	}
	if report.Accuracy.Correct != 3 || report.Accuracy.Total != 4 || report.Accuracy.Ratio >= 1 {
		t.Fatalf("accuracy = %+v", report.Accuracy)
	}
}

type lineAdvanceReader struct {
	lines     []string
	advanceAt int
	advance   func()
	index     int
}

func (r *lineAdvanceReader) Read(p []byte) (int, error) {
	if r.index >= len(r.lines) {
		return 0, io.EOF
	}
	if r.index == r.advanceAt {
		r.advance()
	}
	n := copy(p, r.lines[r.index])
	r.index++
	return n, nil
}

func TestAssessResumeMarksResumedPortion(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	seedAssessmentSource(t, db, 20, 1)
	code, _, errOut := runCLI(t, db, clock, strings.Repeat("B\n", 5), "assess", "start", "--size", "20", "--minutes", "60", "--json")
	if code != 4 {
		t.Fatalf("first start = %d, %s", code, errOut)
	}
	runID := activeAssessmentID(t, db)
	out := mustRun(t, db, clock, strings.Repeat("B\n", 15), "assess", "resume", fmt.Sprintf("%d", runID), "--json")
	runDoc := decodeRunResult(t, out)
	if runDoc.Run.ResumeCount != 1 || runDoc.State != "completed" {
		t.Fatalf("resumed run = %+v", runDoc.Run)
	}
	report := decodeAssessReport(t, mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runID), "--json"))
	if report.Run.ResumeCount != 1 || report.Counts.AnsweredAfterResume != 15 {
		t.Fatalf("resume report = %+v counts=%+v", report.Run, report.Counts)
	}
	after := 0
	for _, item := range report.Items {
		if item.AfterResume {
			after++
		}
	}
	if after != 15 {
		t.Fatalf("items after resume = %d", after)
	}
}

func TestAssessContaminationFromShowDuringRun(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := seedAssessmentSource(t, db, 20, 1)
	var once sync.Once
	showDone := false
	reader := &callbackReader{
		data: []byte(strings.Repeat("B\n", 20)),
		onRead: func() {
			once.Do(func() {
				var stdout, stderr bytes.Buffer
				code := Run([]string{"show", sourceID, "--page", "5", "--json", "--db", db}, Options{
					Stdin:           strings.NewReader(""),
					Stdout:          &stdout,
					Stderr:          &stderr,
					Now:             clock.Now,
					PresentationNow: clock.Now,
					NewSession:      func() string { return "show-test" },
					Getenv:          func(string) string { return "" },
				})
				if code != 0 {
					t.Fatalf("show during run = %d, %s", code, stderr.String())
				}
				showDone = true
			})
		},
	}
	var stdout, stderr bytes.Buffer
	opt := Options{
		Stdin:      reader,
		Stdout:     &stdout,
		Stderr:     &stderr,
		Now:        clock.Now,
		NewSession: func() string { return "sess-test" },
		Getenv:     func(string) string { return "" },
	}
	code := Run([]string{"assess", "start", "--size", "20", "--minutes", "60", "--json", "--db", db}, opt)
	if code != 0 || !showDone {
		t.Fatalf("contaminated run = %d, showDone=%t, %s", code, showDone, stderr.String())
	}
	runDoc := decodeRunResult(t, stdout.String())
	report := decodeAssessReport(t, mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runDoc.Run.ID), "--json"))
	if report.Counts.Contaminated != 1 || report.Counts.Scored != 19 || report.Counts.Total != 19 {
		t.Fatalf("contamination counts = %+v", report.Counts)
	}
	if len(report.Contamination.ExcludedItems) != 1 {
		t.Fatalf("excluded = %+v", report.Contamination.ExcludedItems)
	}
	excluded := report.Contamination.ExcludedItems[0]
	if excluded.QuestionID != "p05-q1" || !containsExact(excluded.Reasons, "prior_exposure") {
		t.Fatalf("excluded item = %+v", excluded)
	}
}

type callbackReader struct {
	mu     sync.Mutex
	data   []byte
	onRead func()
	once   sync.Once
}

func (r *callbackReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	r.once.Do(r.onRead)
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func TestAssessCompareRefusesUnlikeRuns(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	seedAssessmentSource(t, db, 60, 1)
	first := decodeAssessRun(t, mustRun(t, db, clock, strings.Repeat("B\n", 20), "assess", "start", "--size", "20", "--minutes", "60", "--json"))
	second := decodeAssessRun(t, mustRun(t, db, clock, strings.Repeat("B\n", 20), "assess", "start", "--size", "20", "--minutes", "30", "--json"))
	third := decodeAssessRun(t, mustRun(t, db, clock, strings.Repeat("B\n", 20), "assess", "start", "--size", "20", "--minutes", "60", "--json"))
	code, _, errOut := runCLI(t, db, clock, "", "assess", "compare", fmt.Sprintf("%d", first), fmt.Sprintf("%d", second), "--json")
	if code != 2 || !strings.Contains(errOut, `"code": "incomparable_runs"`) || !strings.Contains(errOut, "window_minutes") {
		t.Fatalf("compare window = %d, %s", code, errOut)
	}
	out := mustRun(t, db, clock, "", "assess", "compare", fmt.Sprintf("%d", first), fmt.Sprintf("%d", third), "--json")
	if !strings.Contains(out, `"state": "compared"`) || !strings.Contains(out, "does not show that AI") {
		t.Fatalf("compare success = %s", out)
	}
}

func decodeAssessRun(t *testing.T, raw string) int64 {
	t.Helper()
	doc := decodeRunResult(t, raw)
	if doc.Run.ID == 0 {
		t.Fatalf("run result has no id: %s", raw)
	}
	return doc.Run.ID
}

func containsExact(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
