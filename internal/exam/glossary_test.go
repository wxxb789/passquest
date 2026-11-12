package exam

import "testing"

func TestGlossaryProblems(t *testing.T) {
	if problems := GlossaryProblems([]GlossaryEntry{{Term: "closes", Meaning: "shuts"}}); len(problems) != 0 {
		t.Fatalf("valid glossary = %v", problems)
	}
	problems := GlossaryProblems([]GlossaryEntry{
		{Term: "", Meaning: "x"},
		{Term: "closes", Meaning: ""},
		{Term: "closes", Meaning: "again"},
	})
	if len(problems) < 3 {
		t.Fatalf("problems = %v", problems)
	}
}

func TestParseAcceptsGlossaryAndRejectsAnEmptyMeaning(t *testing.T) {
	valid := `{
		"schema_version": 1,
		"id": "g1",
		"version": "1",
		"title": "Glossary",
		"notice": "fixture",
		"provenance": {"author": "a", "source": "s", "rights": "r", "exam_relation": "e"},
		"passage": {"title": "p", "text": "The library closes at 4 p.m."},
		"questions": [{
			"id": "q1",
			"stem": "When?",
			"options": [{"label": "A", "text": "4 p.m."}, {"label": "B", "text": "9 a.m."}],
			"answer": "A",
			"evidence": "closes at 4 p.m.",
			"reasoning": "stated",
			"option_reasoning": {"A": "stated", "B": "not stated"},
			"glossary": [{"term": "closes", "meaning": "shuts"}]
		}]
	}`
	if _, err := Parse([]byte(valid)); err != nil {
		t.Fatalf("valid glossary rejected: %v", err)
	}
	invalid := `{
		"schema_version": 1,
		"id": "g2",
		"version": "1",
		"title": "Glossary",
		"notice": "fixture",
		"provenance": {"author": "a", "source": "s", "rights": "r", "exam_relation": "e"},
		"passage": {"title": "p", "text": "The library closes at 4 p.m."},
		"questions": [{
			"id": "q1",
			"stem": "When?",
			"options": [{"label": "A", "text": "4 p.m."}, {"label": "B", "text": "9 a.m."}],
			"answer": "A",
			"evidence": "closes at 4 p.m.",
			"reasoning": "stated",
			"option_reasoning": {"A": "stated", "B": "not stated"},
			"glossary": [{"term": "closes", "meaning": "  "}]
		}]
	}`
	if _, err := Parse([]byte(invalid)); err == nil {
		t.Fatal("empty glossary meaning accepted")
	}
}
