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

// QuestionKey is the identity of a question inside one content. Attempt
// numbering, repeat exposure and report rows are scoped to it: the same
// question id under another content id is a different question.
type QuestionKey struct {
	ContentID  string
	QuestionID string
}

// Assistance is the condition under which an attempt was made. It is separate
// from first/repeat exposure: a learner can be helped on a new question, or
// answer a repeated question without help.
type Assistance string

const (
	// AssistanceIndependent means no hint or tactic walkthrough was shown with
	// the attempt. Repeat exposure is recorded separately.
	AssistanceIndependent Assistance = "independent"
	// AssistanceAssisted means a tactic walkthrough or other help accompanied
	// the attempt: the guided response carries it in the pre-answer document
	// and in the answered document, so a one-shot client still receives it. It
	// is never independent evidence.
	AssistanceAssisted Assistance = "assisted"
)

// Valid reports whether a is a recorded assistance condition.
func (a Assistance) Valid() bool {
	return a == AssistanceIndependent || a == AssistanceAssisted
}

// ValidSkillID reports whether id is a lowercase slug usable as a skill id.
func ValidSkillID(id string) bool {
	return exam.ValidSkillTag(id)
}

// ValidTacticID reports whether id is a lowercase slug usable as a tactic id.
// A tactic is a scoring skill, so the grammar is shared.
func ValidTacticID(id string) bool {
	return ValidSkillID(id)
}

// Attempt is one immutable answer record. AttemptNumber and PriorAttempts are
// assigned by Append inside the insert transaction and recomputed by List from
// row order.
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
	// Assistance and TacticID record whether help was visible and which tactic
	// the help belonged to. TacticID is empty for ordinary practice, which can
	// still be assisted when a tactic walkthrough for the question was already
	// delivered.
	Assistance Assistance
	TacticID   string
	// Skills snapshots the question's skill tags when the answer was recorded.
	// Adaptive selection groups historical attempts by this snapshot, so a
	// later tag edit cannot silently regroup old evidence. It is empty for
	// untagged questions and for rows written before schema version 6.
	Skills []string
	// SkillsRecorded reports whether the row carries a skill snapshot at all.
	// A recorded empty snapshot means the question had no registered tag when
	// the answer was recorded; a row without a snapshot predates version 6 and
	// may fall back to the question's current tags. The distinction is stored
	// as NULL versus a JSON array.
	SkillsRecorded bool
}

// Key returns the content-scoped identity used for numbering and repeats.
func (a Attempt) Key() QuestionKey {
	return QuestionKey{ContentID: a.ContentID, QuestionID: a.QuestionID}
}

// Repeat reports whether the key had already been shown by an earlier attempt
// on the same content-scoped question.
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

// BuildRecord grades a selection and snapshots the selected and correct option
// text, so a later content revision cannot change the meaning of a historical
// attempt. It does not snapshot the passage or stem; the store does not keep
// them.
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
		Assistance:     AssistanceIndependent,
		Skills:         append([]string(nil), q.Tactics...),
		SkillsRecorded: true,
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

// WithAssistance annotates a graded record with the condition under which it
// was made, without attributing it to a tactic. Ordinary practice uses it when
// the guided walkthrough for that question was already delivered: the help is
// real even though the ordinary command did not show it. Tactic practice uses
// WithTactic instead.
func (a Attempt) WithAssistance(assistance Assistance) (Attempt, error) {
	if !assistance.Valid() {
		return Attempt{}, fmt.Errorf("attempt: invalid assistance %q", assistance)
	}
	if a.QuestionID == "" || a.ContentID == "" {
		return Attempt{}, errors.New("attempt: annotate a graded record")
	}
	a.Assistance = assistance
	return a, nil
}

// WithTactic annotates a graded record with the tactic exercise and the
// assistance condition. A guided attempt is Assisted; an unguided attempt is
// Independent. Ordinary practice never calls this.
func (a Attempt) WithTactic(tacticID string, assistance Assistance) (Attempt, error) {
	id := strings.TrimSpace(tacticID)
	if id == "" {
		return Attempt{}, errors.New("attempt: empty tactic id")
	}
	if !ValidTacticID(id) {
		return Attempt{}, fmt.Errorf("attempt: tactic id %q is not a lowercase slug", id)
	}
	a, err := a.WithAssistance(assistance)
	if err != nil {
		return Attempt{}, err
	}
	a.TacticID = id
	for _, skillID := range a.Skills {
		if skillID == id {
			return a, nil
		}
	}
	// Copy before appending so annotating a record cannot mutate the caller's
	// skill slice.
	skills := make([]string, len(a.Skills), len(a.Skills)+1)
	copy(skills, a.Skills)
	a.Skills = append(skills, id)
	return a, nil
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
	Assistance        Assistance
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
		Assistance:   a.Assistance,
	}
	if a.Repeat() {
		fb.Exposure = ExposureAnswerAlreadyShown
	}
	if a.Status == StatusIncorrect {
		fb.SelectedReasoning = q.OptionReasoning[a.SelectedLabel]
	}
	return fb
}
