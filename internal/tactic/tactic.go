// Package tactic owns the authored scoring tactics and their exercise material.
// A tactic explains why an exam-scoring rule works, when it applies and where a
// naive rule fails; it makes no learner-state or scoring decision.
package tactic

import (
	"errors"
	"fmt"
	"strings"

	"github.com/wxxb789/passquest/internal/exam"
)

// Boundary is the counterexample that keeps a tactic honest: a familiar shortcut,
// a concrete case where it produces the wrong answer, and the reason it fails.
type Boundary struct {
	Folklore   string `json:"folklore"`
	Example    string `json:"example"`
	WhyItFails string `json:"why_it_fails"`
}

// Tactic is one inspectable scoring rule. Lesson fields are authored text, not
// model output, and carry no claim about the live examination.
type Tactic struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Principle   string   `json:"principle"`
	WhyItWorks  string   `json:"why_it_works"`
	WhenItWorks string   `json:"when_it_works"`
	Steps       []string `json:"steps"`
	Boundary    Boundary `json:"boundary"`
}

// Validate enforces the lesson invariants. An empty why/when/boundary would turn
// a tactic into a slogan, so all of them are required.
func (t Tactic) Validate() error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	if !exam.ValidSkillTag(t.ID) {
		add("id %q is not a lowercase slug", t.ID)
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{"title", t.Title},
		{"summary", t.Summary},
		{"principle", t.Principle},
		{"why_it_works", t.WhyItWorks},
		{"when_it_works", t.WhenItWorks},
		{"boundary.folklore", t.Boundary.Folklore},
		{"boundary.example", t.Boundary.Example},
		{"boundary.why_it_fails", t.Boundary.WhyItFails},
	} {
		if strings.TrimSpace(field.value) == "" {
			add("%s is empty", field.name)
		}
	}
	if len(t.Steps) < 3 {
		add("has %d steps, want at least 3", len(t.Steps))
	}
	for i, step := range t.Steps {
		if strings.TrimSpace(step) == "" {
			add("steps[%d] is empty", i)
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid tactic: %s", strings.Join(problems, "; "))
	}
	return nil
}

// notExceptPolarity is the tactic delivered by goal 0003. The counterexample is
// deliberately a different item from the embedded exercise so displaying the
// lesson cannot reveal an exercise answer.
var notExceptPolarity = Tactic{
	ID:      "not-except-polarity",
	Title:   "NOT/EXCEPT polarity",
	Summary: "Read the stem's negative word first, then match statements to the passage instead of matching words.",
	Principle: "In a NOT/EXCEPT item, three options are supported by the passage and one is not (or is false). " +
		"The key is the odd statement, not the option that reuses the most passage vocabulary.",
	WhyItWorks: "The item author gives the key a different truth relation from the other three options. " +
		"Negation is carried by the stem and by small words such as 'not', 'except', 'free' or 'after', so an option can repeat passage nouns and still be false. " +
		"A deterministic key can be checked by locating evidence for each option, not by guessing from word overlap or length.",
	WhenItWorks: "Use it on multiple-choice questions whose stem asks for what is NOT stated, NOT true, NOT included, or contains EXCEPT. " +
		"It needs an answer mapping that has been reviewed. " +
		"It does not decide a 'best title' or an inference question, and it cannot rescue a question whose key is disputed.",
	Steps: []string{
		"Find the negative word in the stem: NOT, EXCEPT, INCORRECT, or 'all of the following ... except'.",
		"For each option, locate the passage sentence it depends on, or record that the statement is absent.",
		"Mark each option as supported or contradicted by the passage.",
		"Choose the option whose support disagrees with the stem's polarity.",
		"Check the other options against the same rule; exactly one statement should be the odd one.",
		"If two options fit or the passage is ambiguous, do not guess from keyword overlap; mark the item for review.",
	},
	Boundary: Boundary{
		Folklore: "Choose the option that repeats the most words from the passage, and prefer the longest option.",
		Example: "A NOT item says: 'The reading room is closed on Mondays. Which of the following is NOT true?' " +
			"Option A says 'The reading room is closed on Mondays'; option B says 'The reading room is open on Mondays.' " +
			"Option A repeats the passage exactly, but under a NOT stem it is the supported statement and therefore wrong. " +
			"Option B reverses the fact and is the key. The longer option is not more likely to be right; it is often a wordy paraphrase that copies vocabulary.",
		WhyItFails: "Word overlap measures vocabulary reuse, not truth. The stem decides which truth relation the key needs, " +
			"and a small negation word changes that relation without changing the repeated nouns.",
	},
}

// All returns the registered tactics in display order. The slice and each
// tactic's step slice are copies, so a caller cannot mutate the registry.
func All() []Tactic {
	lesson := notExceptPolarity
	lesson.Steps = append([]string(nil), notExceptPolarity.Steps...)
	return []Tactic{lesson}
}

// Get returns the registered tactic with the given id.
func Get(id string) (Tactic, bool) {
	for _, candidate := range All() {
		if candidate.ID == strings.TrimSpace(id) {
			return candidate, true
		}
	}
	return Tactic{}, false
}

// Registered returns the registered ids in display order.
func Registered() []string {
	all := All()
	ids := make([]string, 0, len(all))
	for _, candidate := range all {
		ids = append(ids, candidate.ID)
	}
	return ids
}

// IsRegistered reports whether id names a registered tactic.
func IsRegistered(id string) bool {
	_, ok := Get(id)
	return ok
}

// ValidateRegistry checks every registered tactic. A build whose authored lesson
// violates the invariant must not ship.
func ValidateRegistry() error {
	var problems []string
	seen := make(map[string]bool)
	for _, candidate := range All() {
		if err := candidate.Validate(); err != nil {
			problems = append(problems, err.Error())
		}
		if seen[candidate.ID] {
			problems = append(problems, fmt.Sprintf("tactic id %q is duplicated", candidate.ID))
		}
		seen[candidate.ID] = true
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}
