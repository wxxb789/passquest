package exam

import (
	"strings"
	"testing"
)

func validExam() Exam {
	return Exam{
		SchemaVersion: SchemaVersion,
		ID:            "test-0001",
		Version:       "1",
		Title:         "Test notice",
		Notice:        "Self-authored sample. Not from any live exam.",
		Provenance: Provenance{
			Author:       "Pass Quest contributors",
			Source:       "self-authored for this repository",
			Rights:       "original; redistributable",
			ExamRelation: "illustrative practice only; not drawn from a live examination",
		},
		Passage: Passage{
			Title: "Library hours",
			Text:  "The library closes at four on Saturday.",
		},
		Questions: []Question{
			{
				ID:        "q1",
				Stem:      "When does the library close on Saturday?",
				Options:   []Option{{Label: "A", Text: "At four."}, {Label: "B", Text: "At five."}},
				Answer:    "A",
				Evidence:  "closes at four on Saturday",
				Reasoning: "The passage states the Saturday closing time directly.",
				OptionReasoning: map[string]string{
					"A": "The passage states four.",
					"B": "The passage does not say five.",
				},
			},
		},
	}
}

func TestSampleIsValidAndEvidenceGrounded(t *testing.T) {
	sample, err := Sample()
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	if err := sample.Validate(); err != nil {
		t.Fatalf("sample validation: %v", err)
	}
	if len(sample.Questions) != 3 {
		t.Fatalf("questions = %d, want 3", len(sample.Questions))
	}
	for _, q := range sample.Questions {
		if !strings.Contains(sample.Passage.Text, q.Evidence) {
			t.Errorf("%s evidence is not in the passage", q.ID)
		}
		if len(q.Options) != 4 {
			t.Errorf("%s options = %d, want 4", q.ID, len(q.Options))
		}
		if len(q.OptionReasoning) != len(q.Options) {
			t.Errorf("%s explanations = %d, want %d", q.ID, len(q.OptionReasoning), len(q.Options))
		}
		if _, ok := q.Option(q.Answer); !ok {
			t.Errorf("%s answer %q is missing", q.ID, q.Answer)
		}
	}
}

func TestSampleMakesNoConfidentialOrLiveExamClaim(t *testing.T) {
	raw := string(sampleJSON)
	// Built from parts so this test does not itself trip the repository's
	// confidential-marker gate.
	for _, marker := range []string{"CR" + "SC", "中国" + "通号", "仅限内部" + "使用", "严禁" + "外传"} {
		if strings.Contains(raw, marker) {
			t.Errorf("sample contains confidential source marker %q", marker)
		}
	}
	lower := strings.ToLower(raw)
	for _, want := range []string{"self-authored", "not from any live exam", "not drawn from a live examination"} {
		if !strings.Contains(lower, want) {
			t.Errorf("sample does not state %q", want)
		}
	}
}

func TestParseRejectsUnknownFieldAndTrailingData(t *testing.T) {
	base := `{"schema_version":1,"id":"x","version":"1","title":"t","notice":"n",` +
		`"provenance":{"author":"a","source":"s","rights":"r","exam_relation":"e"},` +
		`"passage":{"title":"p","text":"The library closes at four."},` +
		`"questions":[{"id":"q1","stem":"s","options":[{"label":"A","text":"a"},{"label":"B","text":"b"}],` +
		`"answer":"A","evidence":"closes at four","reasoning":"r",` +
		`"option_reasoning":{"A":"a","B":"b"}}]}`

	if _, err := Parse([]byte(base)); err != nil {
		t.Fatalf("base exam should parse: %v", err)
	}
	withUnknown := strings.Replace(base, `"title":"t"`, `"title":"t","surprise":true`, 1)
	if _, err := Parse([]byte(withUnknown)); err == nil {
		t.Fatal("unknown field was accepted")
	}
	if _, err := Parse([]byte(base + base)); err == nil {
		t.Fatal("trailing JSON was accepted")
	}
}

func TestValidateRejects(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Exam)
		wantErr string
	}{
		{"wrong schema", func(e *Exam) { e.SchemaVersion = 2 }, "schema_version"},
		{"empty id", func(e *Exam) { e.ID = "" }, "id is empty"},
		{"empty passage", func(e *Exam) { e.Passage.Text = "" }, "passage.text is empty"},
		{"no questions", func(e *Exam) { e.Questions = nil }, "no questions"},
		{"duplicate question id", func(e *Exam) {
			e.Questions = append(e.Questions, e.Questions[0])
		}, "duplicated"},
		{"missing answer", func(e *Exam) { e.Questions[0].Answer = "" }, "is not one of its options"},
		{"answer not an option", func(e *Exam) { e.Questions[0].Answer = "C" }, "is not one of its options"},
		{"duplicate label", func(e *Exam) { e.Questions[0].Options[1].Label = "A" }, "label \"A\" is duplicated"},
		{"bad label", func(e *Exam) { e.Questions[0].Options[1].Label = "b" }, "not one uppercase letter"},
		{"empty option text", func(e *Exam) { e.Questions[0].Options[0].Text = "" }, "text is empty"},
		{"evidence absent from passage", func(e *Exam) { e.Questions[0].Evidence = "closes at noon" }, "not a verbatim passage substring"},
		{"empty evidence", func(e *Exam) { e.Questions[0].Evidence = "" }, "evidence is empty"},
		{"empty reasoning", func(e *Exam) { e.Questions[0].Reasoning = "" }, "reasoning is empty"},
		{"missing option explanation", func(e *Exam) { delete(e.Questions[0].OptionReasoning, "B") }, "option B has no explanation"},
		{"extra option explanation", func(e *Exam) { e.Questions[0].OptionReasoning["C"] = "extra" }, "explains unknown option"},
		{"too few options", func(e *Exam) { e.Questions[0].Options = e.Questions[0].Options[:1] }, "want 2..8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := validExam()
			tt.mutate(&e)
			err := e.Validate()
			if err == nil {
				t.Fatal("Validate accepted an invalid exam")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestQuestionAndOptionLookup(t *testing.T) {
	e := validExam()
	q, number, ok := e.Question("q1")
	if !ok || number != 0 || q.ID != "q1" {
		t.Fatalf("Question(q1) = %v, %d, %v", q, number, ok)
	}
	if _, _, ok := e.Question("missing"); ok {
		t.Fatal("Question(missing) reported ok")
	}
	if opt, ok := q.Option("A"); !ok || opt.Text != "At four." {
		t.Fatalf("Option(A) = %v, %v", opt, ok)
	}
	if _, ok := q.Option("Z"); ok {
		t.Fatal("Option(Z) reported ok")
	}
	if q.Correct().Label != "A" {
		t.Fatalf("Correct = %q, want A", q.Correct().Label)
	}
}
