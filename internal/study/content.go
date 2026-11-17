package study

import (
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/skill"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
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

// LoadSourceBundle reads an imported source and its stored questions. Callers
// in practice, recommend, coach and the CLI-only commands share it.
func LoadSourceBundle(st *store.Store, sourceID string) (source.Source, []source.Question, error) {
	src, err := st.Source(sourceID)
	if err != nil {
		return source.Source{}, nil, err
	}
	questions, err := st.Questions(sourceID)
	if err != nil {
		return source.Source{}, nil, err
	}
	return src, questions, nil
}

// FindPage returns the page with the given number.
func FindPage(src source.Source, number int) (source.Page, bool) {
	for _, page := range src.Pages {
		if page.Number == number {
			return page, true
		}
	}
	return source.Page{}, false
}

// FindQuestion returns the stored question with the given id.
func FindQuestion(questions []source.Question, id string) (source.Question, bool) {
	for _, q := range questions {
		if q.ID == id {
			return q, true
		}
	}
	return source.Question{}, false
}
