package study

import (
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/skill"
)

// EmbeddedContents lists every content authored in this build: the sample and
// the skill practice passages, which already include the tactic exercise.
// Returned values are shared and must not be mutated.
func EmbeddedContents() ([]exam.Exam, error) {
	sample, err := exam.Sample()
	if err != nil {
		return nil, err
	}
	contents, err := skill.PracticeContent()
	if err != nil {
		return nil, err
	}
	return append([]exam.Exam{sample}, contents...), nil
}

// EmbeddedContent resolves an authored content id. found is false for a parsed
// source page id, which the caller resolves from the store.
func EmbeddedContent(id string) (exam.Exam, bool, error) {
	contents, err := EmbeddedContents()
	if err != nil {
		return exam.Exam{}, false, err
	}
	for _, content := range contents {
		if content.ID == id {
			return content, true, nil
		}
	}
	return exam.Exam{}, false, nil
}
