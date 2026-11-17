package coach

import (
	"errors"
	"strings"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/skill"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
	"github.com/wxxb789/passquest/internal/tutor"
)

type loadedContent struct {
	Found    bool
	Content  exam.Exam
	Question exam.Question
	Private  bool
	SourceID string
}

// loadContent resolves the stored attempt's question from embedded material or
// a reviewed page of an imported source. A missing or no-longer-practiceable
// question is a state, not an error: the prompt builder discloses that current
// content is unavailable.
func loadContent(s *study.Service, contentID, questionID string) (loadedContent, error) {
	if content, found, err := study.EmbeddedContent(contentID); err != nil {
		return loadedContent{}, err
	} else if found {
		return embedded(content, questionID), nil
	}

	sourceID, pageNumber, ok := source.ParseContentID(contentID)
	if !ok {
		return loadedContent{Private: strings.HasPrefix(contentID, "src-")}, nil
	}
	out := loadedContent{Private: true, SourceID: sourceID}
	src, err := kernel.Store(s).Source(sourceID)
	if errors.Is(err, store.ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return loadedContent{}, err
	}
	page, found := study.FindPage(src, pageNumber)
	if !found {
		return out, nil
	}
	questions, err := kernel.Store(s).Questions(sourceID)
	if err != nil {
		return loadedContent{}, err
	}
	content, _, err := source.BuildPracticeExam(src, page, questions)
	if err != nil {
		return out, nil
	}
	loaded := embedded(content, questionID)
	loaded.Private = true
	loaded.SourceID = sourceID
	return loaded, nil
}

func embedded(content exam.Exam, questionID string) loadedContent {
	question, _, ok := content.Question(questionID)
	if !ok {
		return loadedContent{}
	}
	return loadedContent{Found: true, Content: content, Question: question}
}

// loadTarget builds the prompt target for one attempt: its stored answer,
// skills and, when the question still resolves, the current content.
func loadTarget(s *study.Service, record attempt.Attempt, providerID string) (tutor.Target, error) {
	loaded, err := loadContent(s, record.ContentID, record.QuestionID)
	if err != nil {
		return tutor.Target{}, err
	}
	skills, lessons, mapped := targetSkills(record, loaded)
	target := tutor.Target{
		AttemptID:                   record.ID,
		Status:                      string(record.Status),
		SelectedLabel:               record.SelectedLabel,
		SelectedText:                record.SelectedText,
		CorrectLabel:                record.CorrectLabel,
		CorrectText:                 record.CorrectText,
		ElapsedMS:                   record.ElapsedMS,
		AttemptNumber:               record.AttemptNumber,
		Repeat:                      record.Repeat(),
		Assistance:                  string(record.Assistance),
		Skills:                      skills,
		SkillsMappedFromCurrentTags: mapped,
		Lessons:                     lessons,
		PrivateSource:               loaded.Private,
		SourceID:                    loaded.SourceID,
	}
	if loaded.Found {
		target.ContentMatched = true
		target.ContentChanged = loaded.Content.Version != record.ContentVersion
		target.PassageText = loaded.Content.Passage.Text
		target.QuestionStem = loaded.Question.Stem
		target.Evidence = loaded.Question.Evidence
		target.Reasoning = loaded.Question.Reasoning
		target.Options = make([]tutor.Option, 0, len(loaded.Question.Options))
		for _, option := range loaded.Question.Options {
			target.Options = append(target.Options, tutor.Option{Label: option.Label, Text: option.Text})
		}
	}
	if target.PrivateSource && target.SourceID != "" {
		event, ok, err := kernel.Store(s).TutorConsent(target.SourceID, providerID)
		if err != nil {
			return tutor.Target{}, err
		}
		target.ConsentGranted = ok && event.Granted
	}
	return target, nil
}

func targetSkills(record attempt.Attempt, loaded loadedContent) ([]string, []tutor.SkillLesson, bool) {
	ids, fromCurrent, _ := study.EffectiveSkills(record, loaded.Question.Tactics, loaded.Found)
	var lessons []tutor.SkillLesson
	for _, id := range ids {
		candidate, _ := skill.Get(id)
		lessons = append(lessons, tutor.SkillLesson{
			ID:          candidate.ID,
			Kind:        string(candidate.Kind),
			Title:       candidate.Title,
			Summary:     candidate.Summary,
			Principle:   candidate.Principle,
			WhyItWorks:  candidate.WhyItWorks,
			WhenItWorks: candidate.WhenItWorks,
			Steps:       append([]string(nil), candidate.Steps...),
			WatchOut:    candidate.WatchOut,
		})
	}
	return ids, lessons, fromCurrent && len(ids) > 0
}
