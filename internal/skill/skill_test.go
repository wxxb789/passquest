package skill

import (
	"slices"
	"strings"
	"testing"

	"github.com/wxxb789/passquest/internal/tactic"
)

func TestRegistryIsValid(t *testing.T) {
	if err := ValidateRegistry(); err != nil {
		t.Fatalf("ValidateRegistry: %v", err)
	}
	all := All()
	if len(all) != 3 {
		t.Fatalf("skills = %d, want 3", len(all))
	}
	if all[0].ID != "not-except-polarity" || all[0].Kind != KindScoring {
		t.Fatalf("first skill = %+v, want the scoring tactic", all[0])
	}
	for _, id := range []string{"vocabulary-in-context", "sentence-structure"} {
		candidate, ok := Get(id)
		if !ok {
			t.Fatalf("%s is not registered", id)
		}
		if candidate.Kind != KindLanguage || candidate.TacticID != "" {
			t.Fatalf("%s = %+v, want a language skill with no tactic", id, candidate)
		}
		if len(candidate.Steps) < 3 {
			t.Fatalf("%s has %d steps", id, len(candidate.Steps))
		}
		for _, field := range []struct {
			name  string
			value string
		}{
			{"principle", candidate.Principle},
			{"why_it_works", candidate.WhyItWorks},
			{"when_it_works", candidate.WhenItWorks},
			{"watch_out", candidate.WatchOut},
		} {
			if strings.TrimSpace(field.value) == "" {
				t.Errorf("%s %s is empty", id, field.name)
			}
		}
	}

	scoring, _ := Get("not-except-polarity")
	lesson, ok := tactic.Get(scoring.TacticID)
	if !ok {
		t.Fatalf("scoring tactic %q is not registered", scoring.TacticID)
	}
	if scoring.Principle != lesson.Principle || !slices.Equal(scoring.Steps, lesson.Steps) {
		t.Fatal("scoring skill does not reuse the tactic lesson")
	}
	if !strings.Contains(scoring.WatchOut, lesson.Boundary.WhyItFails) {
		t.Fatal("scoring skill does not carry the tactic boundary")
	}
}

func TestAllReturnsCopies(t *testing.T) {
	first := All()
	first[0].Steps[0] = "mutated"
	first[1].Title = "mutated"
	second := All()
	if second[0].Steps[0] == "mutated" || second[1].Title == "mutated" {
		t.Fatal("All returned a skill that aliases the registry")
	}
	if candidate, _ := Get("not-except-polarity"); candidate.Steps[0] == "mutated" {
		t.Fatal("Get returned a skill that aliases the registry")
	}
}

func TestInvalidSkillIsRejected(t *testing.T) {
	candidate, _ := Get("vocabulary-in-context")
	cases := []struct {
		name   string
		mutate func(*Skill)
		want   string
	}{
		{"bad id", func(s *Skill) { s.ID = "Vocabulary In Context" }, "lowercase slug"},
		{"bad kind", func(s *Skill) { s.Kind = "grammar" }, "kind"},
		{"no principle", func(s *Skill) { s.Principle = " " }, "principle is empty"},
		{"no watch out", func(s *Skill) { s.WatchOut = "" }, "watch_out is empty"},
		{"no steps", func(s *Skill) { s.Steps = nil }, "at least 3"},
		{"empty step", func(s *Skill) { s.Steps = []string{"a", " ", "c"} }, "steps[1] is empty"},
		{"language names tactic", func(s *Skill) { s.TacticID = "not-except-polarity" }, "unexpectedly names tactic"},
		{"scoring without tactic", func(s *Skill) { s.Kind = KindScoring }, "no tactic id"},
		{"scoring unknown tactic", func(s *Skill) { s.Kind = KindScoring; s.TacticID = "not-a-tactic" }, "not registered"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := candidate
			tc.mutate(&broken)
			err := broken.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() = %v, want fragment %q", err, tc.want)
			}
		})
	}
}

func TestValidateContent(t *testing.T) {
	if err := ValidateContent(); err != nil {
		t.Fatalf("ValidateContent: %v", err)
	}
	contents, err := PracticeContent()
	if err != nil {
		t.Fatalf("PracticeContent: %v", err)
	}
	if len(contents) != 7 {
		t.Fatalf("contents = %d, want the tactic exercise plus five language passages and one scoring passage", len(contents))
	}
	for _, id := range Registered() {
		tagged, err := Tagged(id)
		if err != nil {
			t.Fatalf("Tagged(%s): %v", id, err)
		}
		passages := make(map[string]bool)
		for _, exercise := range tagged {
			passages[exercise.Content.ID] = true
		}
		if len(passages) < 2 {
			t.Fatalf("%s has %d distinct passages, want at least 2", id, len(passages))
		}
	}
}

func TestTaggedReturnsOnlyTaggedQuestions(t *testing.T) {
	tests := []struct {
		id   string
		want []string
	}{
		{"not-except-polarity", []string{"t1", "t2", "x1"}},
		{"vocabulary-in-context", []string{"v1", "v2", "v3"}},
		{"sentence-structure", []string{"s1", "s2"}},
	}
	for _, tt := range tests {
		tagged, err := Tagged(tt.id)
		if err != nil {
			t.Fatalf("Tagged(%s): %v", tt.id, err)
		}
		got := make([]string, 0, len(tagged))
		for _, exercise := range tagged {
			got = append(got, exercise.Question.ID)
			if !slices.Contains(exercise.Question.Tactics, tt.id) {
				t.Fatalf("%s returned untagged question %s", tt.id, exercise.Question.ID)
			}
			if exercise.Content.Questions[exercise.Number].ID != exercise.Question.ID {
				t.Fatalf("%s number %d does not point at %s", tt.id, exercise.Number, exercise.Question.ID)
			}
		}
		if !slices.Equal(got, tt.want) {
			t.Fatalf("Tagged(%s) = %v, want %v", tt.id, got, tt.want)
		}
	}
	none, err := Tagged("not-a-skill")
	if err != nil {
		t.Fatalf("Tagged unknown: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("unknown skill returned %d questions", len(none))
	}
}

func TestScoringExerciseCarriesTheKeywordTrap(t *testing.T) {
	tagged, err := Tagged("not-except-polarity")
	if err != nil {
		t.Fatal(err)
	}
	for _, exercise := range tagged {
		if exercise.Question.ID != "x1" {
			continue
		}
		reasoning := strings.ToLower(exercise.Question.OptionReasoning["C"])
		for _, want := range []string{"repeats", "reserved", "false"} {
			if !strings.Contains(reasoning, want) {
				t.Fatalf("x1 key explanation does not mention %q: %s", want, reasoning)
			}
		}
		return
	}
	t.Fatal("scoring exercise x1 is not tagged not-except-polarity")
}
