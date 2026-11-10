// Package skill owns the small set of skills Pass Quest can select practice
// for: one scoring tactic and two minimal language prerequisites. It owns the
// authored language lessons and the first-party passages they are practiced on.
// It makes no learner-state decision; internal/adaptive owns the policy.
package skill

import (
	"errors"
	"fmt"
	"strings"

	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/tactic"
)

// Kind separates an exam-scoring rule from a minimum language prerequisite.
type Kind string

const (
	KindScoring  Kind = "scoring"
	KindLanguage Kind = "language"
)

// Valid reports whether k is a recorded skill kind.
func (k Kind) Valid() bool { return k == KindScoring || k == KindLanguage }

// Skill is one inspectable practice target. Lesson fields are authored text,
// not model output, and make no claim about the live examination.
type Skill struct {
	ID          string   `json:"id"`
	Kind        Kind     `json:"kind"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Principle   string   `json:"principle"`
	WhyItWorks  string   `json:"why_it_works"`
	WhenItWorks string   `json:"when_it_works"`
	Steps       []string `json:"steps"`
	// WatchOut is the boundary or common misreading that keeps the lesson
	// honest. For the scoring skill it is the goal-0003 counterexample; the
	// full lesson remains available through `tactic show`.
	WatchOut string `json:"watch_out"`
	// TacticID links a scoring skill to its full authored tactic lesson. It is
	// empty for language prerequisites.
	TacticID string `json:"tactic_id,omitempty"`
}

// languageSkills are the two minimum language prerequisites. They are authored
// here rather than derived from the private source.
var languageSkills = []Skill{
	{
		ID:          "vocabulary-in-context",
		Kind:        KindLanguage,
		Title:       "Vocabulary in context",
		Summary:     "Use the sentence around an unknown word to choose its meaning instead of translating the word in isolation.",
		Principle:   "A word's meaning is constrained by the sentence that contains it: the purpose, time, contrast and consequence around the word decide which sense is in use.",
		WhyItWorks:  "Exam items often use a familiar word in an unfamiliar sense or ask for a paraphrase. The surrounding clause, not the word's usual translation, is the evidence that makes one option defensible.",
		WhenItWorks: "Use it when a question asks what a word or phrase means \"in the passage\" and the options are synonyms, paraphrases or restatements. It does not rescue a question whose key is disputed or whose passage is missing.",
		Steps: []string{
			"Find the sentence that contains the target word; include the clause before and after it.",
			"Name what the sentence is doing: purpose, time, contrast, condition or consequence.",
			"Replace the word with each option in turn and check which one keeps that meaning.",
			"Reject an option that only repeats a familiar word or a word from the passage.",
			"If two options still fit, mark the item for review instead of guessing from the word alone.",
		},
		WatchOut: "A familiar root or a word that looks like a translation can mislead. The sentence, not the word's usual meaning, decides; do not choose an option just because it contains vocabulary you recognize.",
	},
	{
		ID:          "sentence-structure",
		Kind:        KindLanguage,
		Title:       "Sentence structure",
		Summary:     "Find the subject and verb first, then attach each modifier to the noun it describes.",
		Principle:   "A long sentence is easier to read when its main subject and main verb are identified before the modifiers. Relative clauses, commas and condition words attach to specific nouns and actions.",
		WhyItWorks:  "Exam questions often ask what a phrase describes or what condition must be met. The answer depends on attachment, not on the first noun or the nearest word.",
		WhenItWorks: "Use it on long stems, sentences with who/which/that, and condition sentences with unless, if or because. It does not replace vocabulary knowledge or decide a disputed key.",
		Steps: []string{
			"Underline the main verb and ask who or what performs it.",
			"Mark the main subject before reading the modifiers.",
			"Attach each who/which/that clause to the noun immediately before it and check the meaning.",
			"Treat unless, if and because as condition or reason markers and identify what they attach to.",
			"Re-read only the subject, verb and attached clause; ignore extra nouns that do not fit the grammar.",
		},
		WatchOut: "The first noun is not always the subject, and the nearest noun is not always what a modifier describes. Parenthetical commas can hide the main subject; do not choose an option just because its words appear near the phrase.",
	},
}

// scoringSkill builds the scoring entry from the goal-0003 tactic so the
// boundary text has one owner.
func scoringSkill() Skill {
	lesson, ok := tactic.Get("not-except-polarity")
	if !ok {
		return Skill{}
	}
	return Skill{
		ID:          lesson.ID,
		Kind:        KindScoring,
		Title:       lesson.Title,
		Summary:     lesson.Summary,
		Principle:   lesson.Principle,
		WhyItWorks:  lesson.WhyItWorks,
		WhenItWorks: lesson.WhenItWorks,
		Steps:       append([]string(nil), lesson.Steps...),
		WatchOut:    lesson.Boundary.Folklore + " " + lesson.Boundary.Example + " " + lesson.Boundary.WhyItFails,
		TacticID:    lesson.ID,
	}
}

// All returns the registered skills in display order: the scoring tactic first,
// then the language prerequisites. The slice and every step slice are copies.
func All() []Skill {
	out := make([]Skill, 0, 1+len(languageSkills))
	out = append(out, scoringSkill())
	for _, candidate := range languageSkills {
		candidate.Steps = append([]string(nil), candidate.Steps...)
		out = append(out, candidate)
	}
	return out
}

// Get returns the registered skill with the given id.
func Get(id string) (Skill, bool) {
	trimmed := strings.TrimSpace(id)
	for _, candidate := range All() {
		if candidate.ID == trimmed {
			return candidate, true
		}
	}
	return Skill{}, false
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

// IsRegistered reports whether id names a registered skill.
func IsRegistered(id string) bool {
	_, ok := Get(id)
	return ok
}

// Validate enforces the lesson invariants. An empty explanation or fewer than
// three steps would turn a skill into a slogan.
func (s Skill) Validate() error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	if !exam.ValidSkillTag(s.ID) {
		add("id %q is not a lowercase slug", s.ID)
	}
	if !s.Kind.Valid() {
		add("kind %q is not scoring or language", s.Kind)
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{"title", s.Title},
		{"summary", s.Summary},
		{"principle", s.Principle},
		{"why_it_works", s.WhyItWorks},
		{"when_it_works", s.WhenItWorks},
		{"watch_out", s.WatchOut},
	} {
		if strings.TrimSpace(field.value) == "" {
			add("%s is empty", field.name)
		}
	}
	if len(s.Steps) < 3 {
		add("has %d steps, want at least 3", len(s.Steps))
	}
	for i, step := range s.Steps {
		if strings.TrimSpace(step) == "" {
			add("steps[%d] is empty", i)
		}
	}
	switch s.Kind {
	case KindScoring:
		if strings.TrimSpace(s.TacticID) == "" {
			add("scoring skill has no tactic id")
		} else if !tactic.IsRegistered(s.TacticID) {
			add("tactic id %q is not registered", s.TacticID)
		}
	case KindLanguage:
		if s.TacticID != "" {
			add("language skill unexpectedly names tactic %q", s.TacticID)
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid skill: %s", strings.Join(problems, "; "))
	}
	return nil
}

// ValidateRegistry checks every registered skill and rejects duplicate ids.
func ValidateRegistry() error {
	var problems []string
	seen := make(map[string]bool)
	for _, candidate := range All() {
		if err := candidate.Validate(); err != nil {
			problems = append(problems, err.Error())
		}
		if seen[candidate.ID] {
			problems = append(problems, fmt.Sprintf("skill id %q is duplicated", candidate.ID))
		}
		seen[candidate.ID] = true
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}
