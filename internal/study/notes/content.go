package notes

import (
	"errors"
	"strings"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
)

// loadedContent is one draft's resolved question from embedded material or a
// reviewed page of an imported source.
type loadedContent struct {
	Found    bool
	Content  exam.Exam
	Question exam.Question
	Private  bool
	SourceID string
}

// loadContent resolves a stored question the way `note propose` has always
// resolved it. A missing or no-longer-practiceable question is a state, not an
// error: the caller discloses which state it found.
func loadContent(s *study.Service, contentID, questionID string) (loadedContent, error) {
	if content, found, err := study.EmbeddedContent(contentID); err != nil {
		return loadedContent{}, err
	} else if found {
		return embeddedContent(content, questionID), nil
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
	loaded := embeddedContent(content, questionID)
	loaded.Private = true
	loaded.SourceID = sourceID
	return loaded, nil
}

func embeddedContent(content exam.Exam, questionID string) loadedContent {
	question, _, ok := content.Question(questionID)
	if !ok {
		return loadedContent{}
	}
	return loadedContent{Found: true, Content: content, Question: question}
}

// attemptRecord is today's tutorAttemptRecord: the stored attempt one entry
// point works on, or a refusal naming the attempt.
func attemptRecord(s *study.Service, id int64) (attempt.Attempt, error) {
	records, err := kernel.Store(s).List()
	if err != nil {
		return attempt.Attempt{}, err
	}
	if id < 0 {
		return attempt.Attempt{}, kernel.Refuse("invalid_input", "--attempt must be positive or 0 for the latest attempt")
	}
	if len(records) == 0 {
		return attempt.Attempt{}, kernel.Refuse("no_attempts", "no attempt is recorded; run passquest practice first")
	}
	if id == 0 {
		return records[len(records)-1], nil
	}
	for _, record := range records {
		if record.ID == id {
			return record, nil
		}
	}
	return attempt.Attempt{}, kernel.Refuse("unknown_attempt", "attempt #%d is not recorded", id)
}
