// Package attempt owns the append-only evidence of a learner answering an
// authored question. Grading is deterministic; nothing here contacts a model or
// a network.
package attempt

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/exam"
)

// Status is the recorded outcome of one attempt.
type Status string

const (
	StatusCorrect   Status = "correct"
	StatusIncorrect Status = "incorrect"
	StatusSkipped   Status = "skipped"
)

// Valid reports whether s is a recorded outcome.
func (s Status) Valid() bool {
	switch s {
	case StatusCorrect, StatusIncorrect, StatusSkipped:
		return true
	default:
		return false
	}
}

// Attempt is one immutable answer record. AttemptNumber and PriorAttempts are
// assigned by the store inside the insert transaction.
type Attempt struct {
	ID             int64
	SessionID      string
	QuestionID     string
	ContentID      string
	ContentVersion string
	Status         Status
	SelectedLabel  string
	SelectedText   string
	CorrectLabel   string
	CorrectText    string
	StartedAt      time.Time
	AnsweredAt     time.Time
	ElapsedMS      int64
	AttemptNumber  int
	PriorAttempts  int
}

// Repeat reports whether the key had already been shown by an earlier attempt
// on the same question.
func (a Attempt) Repeat() bool { return a.AttemptNumber > 1 }

// Selection is a parsed learner response: an option, or an explicit skip.
type Selection struct {
	Option *exam.Option
	Skip   bool
}

// ParseSelection maps typed input to a selection. It accepts an option label
// case-insensitively, or "skip".
func ParseSelection(q exam.Question, raw string) (Selection, error) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	if value == "SKIP" {
		return Selection{Skip: true}, nil
	}
	if value == "" {
		return Selection{}, errors.New("empty answer")
	}
	opt, ok := q.Option(value)
	if !ok {
		labels := make([]string, 0, len(q.Options))
		for _, o := range q.Options {
			labels = append(labels, o.Label)
		}
		return Selection{}, fmt.Errorf("answer %q is not one of %s", raw, strings.Join(labels, ", "))
	}
	return Selection{Option: &opt}, nil
}

// BuildRecord grades a selection and snapshots what was shown, so a later
// content revision cannot change the meaning of a historical attempt.
func BuildRecord(contentID, contentVersion string, q exam.Question, sel Selection, sessionID string, started, answered time.Time) (Attempt, error) {
	if strings.TrimSpace(sessionID) == "" {
		return Attempt{}, errors.New("attempt: empty session id")
	}
	if strings.TrimSpace(contentID) == "" || strings.TrimSpace(contentVersion) == "" {
		return Attempt{}, errors.New("attempt: empty content id or version")
	}
	if strings.TrimSpace(q.ID) == "" {
		return Attempt{}, errors.New("attempt: empty question id")
	}
	if answered.Before(started) {
		return Attempt{}, errors.New("attempt: answered_at is before started_at")
	}
	correct := q.Correct()
	if correct.Label == "" {
		return Attempt{}, fmt.Errorf("attempt: question %s has no keyed option", q.ID)
	}

	rec := Attempt{
		SessionID:      sessionID,
		QuestionID:     q.ID,
		ContentID:      contentID,
		ContentVersion: contentVersion,
		CorrectLabel:   correct.Label,
		CorrectText:    correct.Text,
		StartedAt:      started.UTC(),
		AnsweredAt:     answered.UTC(),
		ElapsedMS:      answered.Sub(started).Milliseconds(),
	}
	if rec.ElapsedMS < 0 {
		rec.ElapsedMS = 0
	}

	switch {
	case sel.Skip:
		rec.Status = StatusSkipped
	case sel.Option == nil:
		return Attempt{}, errors.New("attempt: selection is neither an option nor a skip")
	case sel.Option.Label == q.Answer:
		rec.Status = StatusCorrect
		rec.SelectedLabel = sel.Option.Label
		rec.SelectedText = sel.Option.Text
	default:
		rec.Status = StatusIncorrect
		rec.SelectedLabel = sel.Option.Label
		rec.SelectedText = sel.Option.Text
	}
	return rec, nil
}

// Feedback is the deterministic explanation shown after an attempt.
type Feedback struct {
	Verdict           Status
	CorrectLabel      string
	CorrectText       string
	Evidence          string
	Reasoning         string
	SelectedReasoning string
	Exposure          string
}

// Exposure values are part of the JSON protocol.
const (
	ExposureFirstAttempt       = "first_attempt"
	ExposureAnswerAlreadyShown = "answer_already_shown"
)

// Explain builds feedback from the stored attempt and the authored content.
func Explain(q exam.Question, a Attempt) Feedback {
	fb := Feedback{
		Verdict:      a.Status,
		CorrectLabel: a.CorrectLabel,
		CorrectText:  a.CorrectText,
		Evidence:     q.Evidence,
		Reasoning:    q.Reasoning,
		Exposure:     ExposureFirstAttempt,
	}
	if a.Repeat() {
		fb.Exposure = ExposureAnswerAlreadyShown
	}
	if a.Status == StatusIncorrect {
		fb.SelectedReasoning = q.OptionReasoning[a.SelectedLabel]
	}
	return fb
}
