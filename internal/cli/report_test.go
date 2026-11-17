package cli

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/store"
)

func TestHumanFeedbackAndReport(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}

	first := mustRun(t, db, clock, "", "practice", "--question", "q1", "--answer", "B")
	for _, want := range []string{"Result: CORRECT", "Evidence:", "First attempt on this question"} {
		if !strings.Contains(first, want) {
			t.Errorf("first human output lacks %q:\n%s", want, first)
		}
	}

	incorrect := mustRun(t, db, clock, "", "practice", "--question", "q2", "--answer", "A")
	for _, want := range []string{"Result: INCORRECT", "Why your answer is wrong:", "Evidence:", "Correct answer:"} {
		if !strings.Contains(incorrect, want) {
			t.Errorf("incorrect human output lacks %q:\n%s", want, incorrect)
		}
	}

	repeated := mustRun(t, db, clock, "", "practice", "--question", "q1", "--answer", "B")
	for _, want := range []string{"Result: CORRECT", "Repeated attempt:", "not independent evidence"} {
		if !strings.Contains(repeated, want) {
			t.Errorf("repeated human output lacks %q:\n%s", want, repeated)
		}
	}

	report := mustRun(t, db, clock, "", "report")
	for _, want := range []string{
		"First attempts: 2 (correct 1, incorrect 1, skipped 0)",
		"Repeated attempts (answer already shown): 1 (correct 1, incorrect 0, skipped 0)",
		"q3  attempts 0  first not attempted",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("human report lacks %q:\n%s", want, report)
		}
	}
}

func TestReportRepeatedIncorrectAndSkipped(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	for _, args := range [][]string{
		{"practice", "--json", "--question", "q1", "--answer", "B"},
		{"practice", "--json", "--question", "q1", "--answer", "A"},
		{"practice", "--json", "--question", "q2", "--skip"},
		{"practice", "--json", "--question", "q2", "--skip"},
	} {
		mustRun(t, db, clock, "", args...)
	}

	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	want := countsView{
		Attempts: 4, Correct: 1, Incorrect: 1, Skipped: 2,
		FirstAttempts: 2, FirstCorrect: 1, FirstSkipped: 1,
		RepeatedAttempts: 2, RepeatedIncorrect: 1, RepeatedSkipped: 1,
		IndependentAttempts: 2, IndependentCorrect: 1, IndependentSkipped: 1,
	}
	if report.Counts != want {
		t.Fatalf("counts = %+v, want %+v", report.Counts, want)
	}
	if report.Questions[0].LatestStatus != "incorrect" || report.Questions[0].Repeated != 1 {
		t.Fatalf("q1 row = %+v", report.Questions[0])
	}
	if report.Questions[1].LatestStatus != "skipped" || report.Questions[1].Repeated != 1 {
		t.Fatalf("q2 row = %+v", report.Questions[1])
	}
}

func TestReportEmpty(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	doc := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	if doc.Counts.Attempts != 0 {
		t.Fatalf("counts = %+v", doc.Counts)
	}
	if len(doc.Questions) != 3 {
		t.Fatalf("questions = %d", len(doc.Questions))
	}
	for _, q := range doc.Questions {
		if q.FirstStatus != "" || q.LatestStatus != "" || q.Attempts != 0 {
			t.Fatalf("empty question row = %+v", q)
		}
	}
	if doc.Attempts == nil || len(doc.Attempts) != 0 {
		t.Fatalf("attempts = %v", doc.Attempts)
	}

	human := mustRun(t, db, clock, "", "report")
	if !strings.Contains(human, "(none recorded)") {
		t.Fatalf("empty human report lacks the no-attempts line:\n%s", human)
	}
}

func TestReportKeepsHistoricalQuestionRows(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	started := goldenBase
	_, err = st.Append(attempt.Attempt{
		SessionID:      "sess-historical",
		QuestionID:     "q9",
		ContentID:      "sample-0001",
		ContentVersion: "1",
		Status:         attempt.StatusCorrect,
		SelectedLabel:  "A",
		SelectedText:   "Historical option.",
		CorrectLabel:   "A",
		CorrectText:    "Historical option.",
		StartedAt:      started,
		AnsweredAt:     started.Add(time.Second),
		ElapsedMS:      1000,
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	report := decodeReport(t, mustRun(t, db, &stepClock{base: goldenBase}, "", "report", "--json"))
	if report.Counts.Attempts != 1 || report.Counts.Correct != 1 {
		t.Fatalf("counts = %+v", report.Counts)
	}
	if len(report.Questions) != 4 {
		t.Fatalf("question rows = %d, want 4", len(report.Questions))
	}
	historical := report.Questions[3]
	if historical.ID != "q9" || historical.Attempts != 1 || historical.FirstStatus != "correct" || historical.LatestStatus != "correct" {
		t.Fatalf("historical row = %+v", historical)
	}
}

func TestReportScopesQuestionRowsByContent(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	started := goldenBase
	records := []attempt.Attempt{
		{
			SessionID:      "sess-sample",
			QuestionID:     "q1",
			ContentID:      "sample-0001",
			ContentVersion: "1",
			Status:         attempt.StatusCorrect,
			SelectedLabel:  "B",
			SelectedText:   "At 4 p.m.",
			CorrectLabel:   "B",
			CorrectText:    "At 4 p.m.",
			StartedAt:      started,
			AnsweredAt:     started.Add(time.Second),
			ElapsedMS:      1000,
		},
		{
			SessionID:      "sess-other",
			QuestionID:     "q1",
			ContentID:      "sample-9999",
			ContentVersion: "1",
			Status:         attempt.StatusIncorrect,
			SelectedLabel:  "A",
			SelectedText:   "At 7 p.m.",
			CorrectLabel:   "B",
			CorrectText:    "At 4 p.m.",
			StartedAt:      started,
			AnsweredAt:     started.Add(time.Second),
			ElapsedMS:      1000,
		},
	}
	for _, record := range records {
		if _, err := st.Append(record); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	clock := &stepClock{base: goldenBase}
	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	want := countsView{
		Attempts: 2, Correct: 1, Incorrect: 1,
		FirstAttempts: 2, FirstCorrect: 1, FirstIncorrect: 1,
		IndependentAttempts: 2, IndependentCorrect: 1, IndependentIncorrect: 1,
	}
	if report.Counts != want {
		t.Fatalf("counts = %+v, want %+v", report.Counts, want)
	}
	if len(report.Questions) != 4 {
		t.Fatalf("question rows = %d, want 4", len(report.Questions))
	}
	rows := make(map[string]questionReport, len(report.Questions))
	for _, q := range report.Questions {
		rows[q.ContentID+"/"+q.ID] = q
	}
	first := rows["sample-0001/q1"]
	if first.Attempts != 1 || first.FirstStatus != "correct" || first.Repeated != 0 {
		t.Fatalf("sample-0001 q1 row = %+v", first)
	}
	other := rows["sample-9999/q1"]
	if other.Attempts != 1 || other.FirstStatus != "incorrect" || other.Repeated != 0 {
		t.Fatalf("sample-9999 q1 row = %+v", other)
	}

	human := mustRun(t, db, clock, "", "report")
	for _, want := range []string{
		"  q1  attempts 1  first correct",
		"  sample-9999/q1  attempts 1  first incorrect",
		"  #2  sample-9999/q1  incorrect",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("human report lacks %q:\n%s", want, human)
		}
	}
}

func TestGoldenReportJSON(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	seedFourAttempts(t, db, clock)
	out := mustRun(t, db, clock, "", "report", "--json")
	checkGolden(t, filepath.Join("testdata", "report.golden.json"), []byte(out))
}
