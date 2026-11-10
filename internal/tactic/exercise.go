package tactic

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/wxxb789/passquest/internal/exam"
)

// The first-party exercise is embedded so the CLI needs no external content file
// and no private source. It is not a live exam item set.
//
//go:embed testdata/first-party-not-except.json
var exerciseJSON []byte

var (
	exerciseOnce sync.Once
	exerciseExam exam.Exam
	exerciseErr  error
)

// Exercise returns the embedded, self-authored tactic exercise. The returned
// value is shared and must not be mutated by callers.
func Exercise() (exam.Exam, error) {
	exerciseOnce.Do(func() {
		exerciseExam, exerciseErr = exam.Parse(exerciseJSON)
	})
	return exerciseExam, exerciseErr
}

// Tagged returns the exercise questions tagged with the tactic, in content
// order. The exercise is first-party, so its answer mapping is verified by
// construction.
func Tagged(id string) ([]exam.Question, error) {
	content, err := Exercise()
	if err != nil {
		return nil, err
	}
	var out []exam.Question
	for _, q := range content.Questions {
		for _, tag := range q.Tactics {
			if tag == id {
				out = append(out, q)
				break
			}
		}
	}
	return out, nil
}

// ValidateExercise checks the invariants the tactic transport relies on: lesson
// and registry are valid, every exercise tag is registered, every registered
// tactic has at least two practiceable questions (a guided example and an
// independent one), and the exercise contains an untagged decoy so selection
// tests cannot pass by taking every question.
func ValidateExercise() error {
	if err := ValidateRegistry(); err != nil {
		return err
	}
	content, err := Exercise()
	if err != nil {
		return err
	}
	counts := make(map[string]int)
	untagged := 0
	for _, q := range content.Questions {
		if len(q.Tactics) == 0 {
			untagged++
		}
		for _, tag := range q.Tactics {
			if !IsRegistered(tag) {
				return fmt.Errorf("exercise question %s has unregistered tactic tag %q", q.ID, tag)
			}
			counts[tag]++
		}
	}
	for _, id := range Registered() {
		if counts[id] < 2 {
			return fmt.Errorf("exercise has %d questions tagged %s, want at least 2", counts[id], id)
		}
	}
	if untagged == 0 {
		return fmt.Errorf("exercise has no untagged decoy question")
	}
	return nil
}
