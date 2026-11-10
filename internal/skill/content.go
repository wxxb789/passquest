package skill

import (
	"embed"
	"fmt"
	"sync"

	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/tactic"
)

// The language exercises are embedded so the CLI needs no external content
// file and no private source. Every passage is first-party and redistributable.
//
//go:embed content/*.json
var contentFS embed.FS

// languageFiles fixes the practice order instead of relying on embed's sorted
// directory order. Vocabulary comes before sentence structure because the
// vocabulary question is the shortest first measurement.
var languageFiles = []string{
	"content/skill-first-party-vocab-notice.json",
	"content/skill-first-party-vocab-library.json",
	"content/skill-first-party-vocab-market.json",
	"content/skill-first-party-sentence-notice.json",
	"content/skill-first-party-sentence-library.json",
	"content/skill-first-party-scoring-library.json",
}

var (
	contentOnce sync.Once
	language    []exam.Exam
	contentErr  error
)

func loadContent() ([]exam.Exam, error) {
	contentOnce.Do(func() {
		out := make([]exam.Exam, 0, len(languageFiles))
		for _, name := range languageFiles {
			data, err := contentFS.ReadFile(name)
			if err != nil {
				contentErr = fmt.Errorf("skill: read %s: %w", name, err)
				return
			}
			parsed, err := exam.Parse(data)
			if err != nil {
				contentErr = fmt.Errorf("skill: parse %s: %w", name, err)
				return
			}
			out = append(out, parsed)
		}
		language = out
	})
	return language, contentErr
}

// PracticeContent returns every first-party exercise that can supply a skill
// practice question: the goal-0003 tactic exercise first, then the language
// exercises in practice order. Returned values are shared and must not be
// mutated by callers.
func PracticeContent() ([]exam.Exam, error) {
	languages, err := loadContent()
	if err != nil {
		return nil, err
	}
	tacticExam, err := tactic.Exercise()
	if err != nil {
		return nil, err
	}
	out := make([]exam.Exam, 0, 1+len(languages))
	out = append(out, tacticExam)
	out = append(out, languages...)
	return out, nil
}

// Exercise is one tagged question together with the content it belongs to.
type Exercise struct {
	Content  exam.Exam
	Question exam.Question
	Number   int
}

// Tagged returns every first-party question tagged with the skill, in practice
// content order. Returned content is shared and must not be mutated.
func Tagged(id string) ([]Exercise, error) {
	contents, err := PracticeContent()
	if err != nil {
		return nil, err
	}
	var out []Exercise
	for _, content := range contents {
		for i, question := range content.Questions {
			for _, tag := range question.Tactics {
				if tag == id {
					out = append(out, Exercise{Content: content, Question: question, Number: i})
					break
				}
			}
		}
	}
	return out, nil
}

// ValidateContent checks the invariants adaptive selection relies on: every
// first-party tag names a registered skill, ids are unique, and every skill has
// at least two distinct passages so one fresh passage remains after the first
// independent transfer.
func ValidateContent() error {
	if err := ValidateRegistry(); err != nil {
		return err
	}
	if err := tactic.ValidateExercise(); err != nil {
		return err
	}
	contents, err := PracticeContent()
	if err != nil {
		return err
	}

	contentIDs := make(map[string]bool, len(contents))
	questionIDs := make(map[string]string)
	passages := make(map[string]map[string]bool)
	for _, content := range contents {
		if contentIDs[content.ID] {
			return fmt.Errorf("first-party content id %q is duplicated", content.ID)
		}
		contentIDs[content.ID] = true
		for _, question := range content.Questions {
			if other, ok := questionIDs[question.ID]; ok {
				return fmt.Errorf("first-party question id %q appears in both %s and %s", question.ID, other, content.ID)
			}
			questionIDs[question.ID] = content.ID
			for _, tag := range question.Tactics {
				if !IsRegistered(tag) {
					return fmt.Errorf("question %s has unregistered skill tag %q", question.ID, tag)
				}
				if passages[tag] == nil {
					passages[tag] = make(map[string]bool)
				}
				passages[tag][content.ID] = true
			}
			if content.ID != "tactic-first-party-not-except" && len(question.Tactics) == 0 {
				return fmt.Errorf("language question %s has no skill tag", question.ID)
			}
		}
	}
	for _, id := range Registered() {
		if len(passages[id]) < 2 {
			return fmt.Errorf("skill %s has questions on %d distinct passage(s), want at least 2", id, len(passages[id]))
		}
	}
	return nil
}
