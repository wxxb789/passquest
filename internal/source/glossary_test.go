package source

import (
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/exam"
)

func TestQuestionFileGlossaryRoundTripsIntoPracticeExam(t *testing.T) {
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	page := Page{
		Number: 1, Origin: PageNative, Review: Reviewed,
		Text: "The library closes at 4 p.m. on Saturdays.",
	}
	q, err := ParseQuestionJSON([]byte(`{
		"id": "q1",
		"stem": "When does the library close?",
		"options": [{"label": "A", "text": "4 p.m."}, {"label": "B", "text": "9 a.m."}],
		"answer": "A",
		"evidence": "closes at 4 p.m.",
		"reasoning": "The notice states it.",
		"option_reasoning": {"A": "stated", "B": "not stated"},
		"glossary": [{"term": "closes", "meaning": "shuts"}]
	}`), "src", 1, now, "")
	if err != nil {
		t.Fatalf("parse question: %v", err)
	}
	if len(q.Glossary) != 1 || q.Glossary[0].Term != "closes" {
		t.Fatalf("glossary = %+v", q.Glossary)
	}
	reviewed, err := q.ReviewAnswer(page, "A", "reviewer", now)
	if err != nil {
		t.Fatalf("review answer: %v", err)
	}
	src := Source{ID: "src", Title: "Fixture", Notice: "notice", Rights: "private", ExamRelation: "none"}
	content, _, err := BuildPracticeExam(src, page, []Question{reviewed})
	if err != nil {
		t.Fatalf("build practice exam: %v", err)
	}
	if len(content.Questions) != 1 || len(content.Questions[0].Glossary) != 1 {
		t.Fatalf("practice exam glossary = %+v", content.Questions)
	}
	if got := content.Questions[0].Glossary[0]; got != (exam.GlossaryEntry{Term: "closes", Meaning: "shuts"}) {
		t.Fatalf("glossary entry = %+v", got)
	}
}

func TestQuestionFileRejectsAnEmptyGlossaryMeaning(t *testing.T) {
	_, err := ParseQuestionJSON([]byte(`{
		"id": "q1",
		"stem": "When?",
		"options": [{"label": "A", "text": "4 p.m."}, {"label": "B", "text": "9 a.m."}],
		"answer": "A",
		"evidence": "closes",
		"reasoning": "stated",
		"option_reasoning": {"A": "stated", "B": "not stated"},
		"glossary": [{"term": "closes", "meaning": ""}]
	}`), "src", 1, time.Now(), "")
	if err == nil {
		t.Fatal("empty glossary meaning accepted")
	}
}
