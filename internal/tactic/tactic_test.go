package tactic

import (
	"strings"
	"testing"
)

func TestRegistryIsValid(t *testing.T) {
	if err := ValidateRegistry(); err != nil {
		t.Fatalf("ValidateRegistry: %v", err)
	}
	lesson, ok := Get("not-except-polarity")
	if !ok {
		t.Fatal("not-except-polarity is not registered")
	}
	if lesson.ID != "not-except-polarity" || lesson.Title == "" {
		t.Fatalf("lesson = %+v", lesson)
	}
	if len(lesson.Steps) < 3 {
		t.Fatalf("steps = %d, want at least 3", len(lesson.Steps))
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{"principle", lesson.Principle},
		{"why_it_works", lesson.WhyItWorks},
		{"when_it_works", lesson.WhenItWorks},
		{"boundary.folklore", lesson.Boundary.Folklore},
		{"boundary.example", lesson.Boundary.Example},
		{"boundary.why_it_fails", lesson.Boundary.WhyItFails},
	} {
		if strings.TrimSpace(field.value) == "" {
			t.Errorf("%s is empty", field.name)
		}
	}
}

func TestAllReturnsCopies(t *testing.T) {
	first := All()
	first[0].Steps[0] = "mutated"
	second := All()
	if second[0].Steps[0] == "mutated" {
		t.Fatal("All returned a tactic whose step slice aliases the registry")
	}
	if lesson, _ := Get("not-except-polarity"); lesson.Steps[0] == "mutated" {
		t.Fatal("Get returned a tactic whose step slice aliases the registry")
	}
}

func TestInvalidTacticIsRejected(t *testing.T) {
	lesson, _ := Get("not-except-polarity")
	cases := []struct {
		name   string
		mutate func(*Tactic)
		want   string
	}{
		{"bad id", func(t *Tactic) { t.ID = "Not Except" }, "lowercase slug"},
		{"no steps", func(t *Tactic) { t.Steps = nil }, "at least 3"},
		{"empty step", func(t *Tactic) { t.Steps = []string{"a", " ", "c"} }, "steps[1] is empty"},
		{"no boundary example", func(t *Tactic) { t.Boundary.Example = "" }, "boundary.example is empty"},
		{"no when", func(t *Tactic) { t.WhenItWorks = "" }, "when_it_works is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := lesson
			tc.mutate(&broken)
			err := broken.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() = %v, want fragment %q", err, tc.want)
			}
		})
	}
}

func TestExerciseCoversEveryRegisteredTactic(t *testing.T) {
	if err := ValidateExercise(); err != nil {
		t.Fatalf("ValidateExercise: %v", err)
	}
	for _, id := range Registered() {
		tagged, err := Tagged(id)
		if err != nil {
			t.Fatalf("Tagged(%s): %v", id, err)
		}
		if len(tagged) < 2 {
			t.Fatalf("%s has %d tagged questions, want at least a guided and an independent question", id, len(tagged))
		}
	}
}

func TestTaggedReturnsOnlyTaggedQuestions(t *testing.T) {
	content, err := Exercise()
	if err != nil {
		t.Fatal(err)
	}
	tagged, err := Tagged("not-except-polarity")
	if err != nil {
		t.Fatal(err)
	}
	if len(tagged) != 2 || tagged[0].ID != "t1" || tagged[1].ID != "t2" {
		t.Fatalf("tagged = %+v, want t1 and t2", tagged)
	}
	for _, q := range content.Questions {
		if q.ID == "t3" && len(q.Tactics) != 0 {
			t.Fatalf("untagged decoy t3 has tags %v", q.Tactics)
		}
	}
	none, err := Tagged("no-such-tactic")
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("unknown tactic returned %d questions", len(none))
	}
}

// The contract requires a counterexample or boundary showing why blind keyword
// matching fails. The lesson must state the failure, and the exercise must
// contain a real keyword-overlap trap whose explanation names the wrong rule.
func TestBoundaryAndExerciseExposeTheKeywordTrap(t *testing.T) {
	lesson, _ := Get("not-except-polarity")
	boundary := strings.ToLower(lesson.Boundary.Folklore + " " + lesson.Boundary.Example + " " + lesson.Boundary.WhyItFails)
	for _, want := range []string{"repeats the most words", "longest option", "word overlap", "not truth"} {
		if !strings.Contains(boundary, want) {
			t.Errorf("boundary does not state %q: %s", want, boundary)
		}
	}

	content, err := Exercise()
	if err != nil {
		t.Fatal(err)
	}
	traps := 0
	for _, q := range content.Questions {
		for _, label := range []string{"A", "B", "C", "D"} {
			reasoning := strings.ToLower(q.OptionReasoning[label])
			if strings.Contains(reasoning, "keyword") || strings.Contains(reasoning, "repeats") {
				traps++
			}
		}
	}
	if traps < 2 {
		t.Fatalf("exercise has %d option explanations naming keyword/repetition traps, want at least 2", traps)
	}

	// The lesson's counterexample must not be copied from the current exercise,
	// or showing help would reveal an exercise answer.
	passage := strings.ToLower(content.Passage.Text)
	for _, sentence := range []string{"reading room is closed on mondays", "reading room is open on mondays"} {
		if strings.Contains(passage, sentence) {
			t.Fatalf("boundary example %q appears in the exercise passage", sentence)
		}
	}
}
