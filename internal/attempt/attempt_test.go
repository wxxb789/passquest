package attempt

import (
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/exam"
)

func testQuestion() exam.Question {
	return exam.Question{
		ID:        "q1",
		Stem:      "When does the library close?",
		Options:   []exam.Option{{Label: "A", Text: "At four."}, {Label: "B", Text: "At five."}},
		Answer:    "A",
		Evidence:  "closes at four",
		Reasoning: "The passage states the closing time.",
		OptionReasoning: map[string]string{
			"A": "Four is stated.",
			"B": "Five is not stated.",
		},
	}
}

func TestParseSelection(t *testing.T) {
	q := testQuestion()
	tests := []struct {
		input string
		skip  bool
		label string
		want  string
	}{
		{"A", false, "A", ""},
		{"a", false, "A", ""},
		{" b ", false, "B", ""},
		{"skip", true, "", ""},
		{"SKIP", true, "", ""},
		{"", false, "", "empty answer"},
		{"Z", false, "", "not one of"},
	}
	for _, tt := range tests {
		sel, err := ParseSelection(q, tt.input)
		if tt.want != "" {
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ParseSelection(%q) error = %v, want %q", tt.input, err, tt.want)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSelection(%q): %v", tt.input, err)
			continue
		}
		if sel.Skip != tt.skip {
			t.Errorf("ParseSelection(%q).Skip = %v", tt.input, sel.Skip)
		}
		if tt.label != "" && (sel.Option == nil || sel.Option.Label != tt.label) {
			t.Errorf("ParseSelection(%q) option = %v", tt.input, sel.Option)
		}
	}
}

func TestBuildRecord(t *testing.T) {
	q := testQuestion()
	started := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

	correct := Selection{Option: &exam.Option{Label: "A", Text: "At four."}}
	rec, err := BuildRecord("c", "1", q, correct, "sess", started, started.Add(1500*time.Millisecond))
	if err != nil {
		t.Fatalf("BuildRecord correct: %v", err)
	}
	if rec.Status != StatusCorrect || rec.SelectedLabel != "A" || rec.SelectedText != "At four." {
		t.Fatalf("correct record = %+v", rec)
	}
	if rec.CorrectLabel != "A" || rec.CorrectText != "At four." {
		t.Fatalf("correct snapshot = %q/%q", rec.CorrectLabel, rec.CorrectText)
	}
	if rec.ElapsedMS != 1500 {
		t.Fatalf("elapsed = %d, want 1500", rec.ElapsedMS)
	}
	if rec.Assistance != AssistanceIndependent || rec.TacticID != "" {
		t.Fatalf("default condition = %q/%q, want independent with no tactic", rec.Assistance, rec.TacticID)
	}

	wrong := Selection{Option: &exam.Option{Label: "B", Text: "At five."}}
	rec, err = BuildRecord("c", "1", q, wrong, "sess", started, started.Add(time.Second))
	if err != nil {
		t.Fatalf("BuildRecord incorrect: %v", err)
	}
	if rec.Status != StatusIncorrect || rec.SelectedLabel != "B" {
		t.Fatalf("incorrect record = %+v", rec)
	}

	rec, err = BuildRecord("c", "1", q, Selection{Skip: true}, "sess", started, started.Add(time.Second))
	if err != nil {
		t.Fatalf("BuildRecord skip: %v", err)
	}
	if rec.Status != StatusSkipped || rec.SelectedLabel != "" || rec.SelectedText != "" {
		t.Fatalf("skip record = %+v", rec)
	}
}

func TestBuildRecordRejectsBadInput(t *testing.T) {
	q := testQuestion()
	started := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	ok := Selection{Option: &exam.Option{Label: "A", Text: "At four."}}
	tests := []struct {
		name    string
		session string
		sel     Selection
		start   time.Time
		end     time.Time
		want    string
	}{
		{"empty session", "", ok, started, started.Add(time.Second), "empty session"},
		{"nil selection", "sess", Selection{}, started, started.Add(time.Second), "neither an option nor a skip"},
		{"answered before started", "sess", ok, started, started.Add(-time.Second), "before started_at"},
	}
	for _, tt := range tests {
		_, err := BuildRecord("c", "1", q, tt.sel, tt.session, tt.start, tt.end)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: error = %v, want %q", tt.name, err, tt.want)
		}
	}
	_, err := BuildRecord("", "1", q, ok, "sess", started, started.Add(time.Second))
	if err == nil || !strings.Contains(err.Error(), "content id") {
		t.Errorf("empty content id: error = %v", err)
	}

	emptyQuestion := q
	emptyQuestion.ID = ""
	_, err = BuildRecord("c", "1", emptyQuestion, ok, "sess", started, started.Add(time.Second))
	if err == nil || !strings.Contains(err.Error(), "empty question id") {
		t.Errorf("empty question id: error = %v", err)
	}

	unkeyed := q
	unkeyed.Answer = "Z"
	_, err = BuildRecord("c", "1", unkeyed, ok, "sess", started, started.Add(time.Second))
	if err == nil || !strings.Contains(err.Error(), "has no keyed option") {
		t.Errorf("unkeyed question: error = %v", err)
	}
}

func TestWithTactic(t *testing.T) {
	q := testQuestion()
	started := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	rec, err := BuildRecord("c", "1", q, Selection{Option: &exam.Option{Label: "A", Text: "At four."}}, "sess", started, started.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	assisted, err := rec.WithTactic("not-except-polarity", AssistanceAssisted)
	if err != nil {
		t.Fatalf("WithTactic assisted: %v", err)
	}
	if assisted.TacticID != "not-except-polarity" || assisted.Assistance != AssistanceAssisted {
		t.Fatalf("assisted = %+v", assisted)
	}
	independent, err := rec.WithTactic("not-except-polarity", AssistanceIndependent)
	if err != nil {
		t.Fatalf("WithTactic independent: %v", err)
	}
	if independent.Assistance != AssistanceIndependent {
		t.Fatalf("independent assistance = %q", independent.Assistance)
	}
	if rec.TacticID != "" || rec.Assistance != AssistanceIndependent {
		t.Fatalf("WithTactic mutated the receiver: %+v", rec)
	}

	bad := []struct {
		name       string
		tacticID   string
		assistance Assistance
		want       string
	}{
		{"empty tactic", "", AssistanceAssisted, "empty tactic id"},
		{"bad tactic", "Not Except", AssistanceAssisted, "lowercase slug"},
		{"bad assistance", "not-except-polarity", "guessed", "invalid assistance"},
	}
	for _, tc := range bad {
		if _, err := rec.WithTactic(tc.tacticID, tc.assistance); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %v, want %q", tc.name, err, tc.want)
		}
	}
	if _, err := (Attempt{}).WithTactic("not-except-polarity", AssistanceAssisted); err == nil {
		t.Fatal("WithTactic accepted an ungraded record")
	}
}

func TestExplain(t *testing.T) {
	q := testQuestion()
	rec := Attempt{Status: StatusIncorrect, SelectedLabel: "B", AttemptNumber: 1, CorrectLabel: "A", CorrectText: "At four."}
	fb := Explain(q, rec)
	if fb.Exposure != ExposureFirstAttempt {
		t.Fatalf("exposure = %q", fb.Exposure)
	}
	if fb.SelectedReasoning != "Five is not stated." {
		t.Fatalf("selected reasoning = %q", fb.SelectedReasoning)
	}
	if fb.Evidence != q.Evidence || fb.Reasoning != q.Reasoning {
		t.Fatalf("feedback evidence/reasoning = %q/%q", fb.Evidence, fb.Reasoning)
	}

	rec.AttemptNumber = 2
	fb = Explain(q, rec)
	if fb.Exposure != ExposureAnswerAlreadyShown {
		t.Fatalf("repeat exposure = %q", fb.Exposure)
	}

	rec.Status = StatusCorrect
	rec.SelectedLabel = "A"
	fb = Explain(q, rec)
	if fb.SelectedReasoning != "" {
		t.Fatalf("correct feedback has selected reasoning %q", fb.SelectedReasoning)
	}

	rec.Assistance = AssistanceAssisted
	fb = Explain(q, rec)
	if fb.Assistance != AssistanceAssisted {
		t.Fatalf("feedback assistance = %q, want assisted", fb.Assistance)
	}
}
