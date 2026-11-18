package workspace

import (
	"context"
	"fmt"

	"github.com/wxxb789/passquest/internal/adaptive"
	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/skill"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/studio"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/practice"
	"github.com/wxxb789/passquest/internal/study/recommend"
	"github.com/wxxb789/passquest/internal/tactic"
)

// StartPractice selects local practice with the same rules as the matching
// CLI command and displays it: the practice module records the presentation
// now (Q-0011-08), spending the unit exactly as `show` does, and records a
// guided tactic walkthrough as delivered. The returned token binds the answer
// to this display; any other open item is closed as abandoned. An empty kind
// is sample practice.
func (w *Workspace) StartPractice(request studio.PracticeRequest) (studio.PracticeItem, error) {
	if request.Kind == "" {
		request.Kind = studio.PracticeSample
	}
	var item studio.PracticeItem
	err := w.withStore(func(st *store.Store) error {
		req := practiceRequestOf(request)
		svc, err := study.Open(w.ports)
		if err != nil {
			return err
		}
		defer svc.Close()
		if request.Kind == studio.PracticeNext {
			// The recommendation read lives in internal/study/recommend; the
			// resolution only reads, but Show's guard runs after it, so a
			// failing resolution still reports assessment_active while a run is
			// active: before this step the guard preceded the whole selection.
			next, err := recommend.PracticeRequest(context.Background(), svc)
			if err != nil {
				if guardErr := study.Guard(svc, "practice"); guardErr != nil {
					return guardErr
				}
				return err
			}
			req = next
		}
		shown, err := practice.Show(context.Background(), svc, req, practice.HoldToken)
		if err != nil {
			return err
		}
		item, err = practiceItemOf(st, shown)
		return err
	})
	return item, err
}

// ResumePractice returns the latest displayed item that was neither answered
// nor abandoned, with its original token and presentation time, or nil.
// Nothing is recorded: the item was already presented.
func (w *Workspace) ResumePractice() (*studio.PracticeItem, error) {
	var item *studio.PracticeItem
	err := w.withStore(func(st *store.Store) error {
		svc, err := study.Open(w.ports)
		if err != nil {
			return err
		}
		defer svc.Close()
		shown, err := practice.Open(context.Background(), svc)
		if err != nil {
			return err
		}
		if shown == nil {
			return nil
		}
		view, err := practiceItemOf(st, *shown)
		if err != nil {
			return err
		}
		item = &view
		return nil
	})
	return item, err
}

// OpenHint shows the authored walkthrough for the displayed item and records
// it as an exposure, so the attempt is recorded as assisted (Q-0011-05). An
// item without an authored walkthrough is refused with hint_unavailable.
func (w *Workspace) OpenHint(token string) (studio.PracticeItem, error) {
	var item studio.PracticeItem
	err := w.withStore(func(st *store.Store) error {
		svc, err := study.Open(w.ports)
		if err != nil {
			return err
		}
		defer svc.Close()
		shown, err := practice.Hint(context.Background(), svc, token)
		if err != nil {
			return err
		}
		item, err = practiceItemOf(st, shown)
		return err
	})
	return item, err
}

// Answer grades the displayed item. Elapsed time runs from the presentation
// to now. Answering a token again returns the stored feedback and writes
// nothing.
func (w *Workspace) Answer(token, label string) (studio.Feedback, error) {
	return w.answer(token, label, false)
}

// Skip records a skipped attempt for the displayed item; it is evidence, and a
// later attempt on the question is a revisit.
func (w *Workspace) Skip(token string) (studio.Feedback, error) {
	return w.answer(token, "", true)
}

func (w *Workspace) answer(token, label string, skip bool) (studio.Feedback, error) {
	var feedback studio.Feedback
	err := w.locked(func(svc *study.Service) error {
		graded, err := practice.Answer(context.Background(), svc, practice.Token(token), practice.Choice{Label: label, Skip: skip})
		if err != nil {
			return err
		}
		feedback = feedbackOf(graded.Item, graded.Attempt, graded.Feedback, graded.Records, graded.Replayed)
		return nil
	})
	return feedback, err
}

// practiceRequestOf maps a studio practice request onto the practice module's
// request. An unknown kind is passed through so the module refuses it with the
// message the window showed before this step. A source request requires a
// non-blank id: the window asks the learner to pick a source, so a blank one is
// the form error it reported before this step (the CLI instead looks the id up
// and fails as not-found).
func practiceRequestOf(request studio.PracticeRequest) practice.Request {
	req := practice.Request{
		Kind:       practice.Kind(request.Kind),
		SourceID:   request.SourceID,
		Page:       request.Page,
		QuestionID: request.QuestionID,
	}
	switch request.Kind {
	case studio.PracticeTactic:
		req.TacticID, req.Stage = request.Tactic, request.Stage
	case studio.PracticeSkill:
		req.SkillID = request.Skill
	case studio.PracticeSource:
		req.RequireSourceID = true
	}
	return req
}

// practiceItemOf renders a displayed item. The condition is the one the
// recorded attempt will carry under adaptive.Classify, the rule Today's skill
// evidence counts by: assisted when help was shown, a revisit after an earlier
// attempt, a re-exposure on a passage that already has an attempt (whatever
// the practice kind), otherwise independent.
func practiceItemOf(st *store.Store, item practice.Item) (studio.PracticeItem, error) {
	records, err := st.List()
	if err != nil {
		return studio.PracticeItem{}, err
	}
	p := item.Presentation
	key := attempt.QuestionKey{ContentID: item.Content.ID, QuestionID: item.Question.ID}
	exposed, err := st.Exposed(key)
	if err != nil {
		return studio.PracticeItem{}, err
	}
	prior := 0
	for _, record := range records {
		if record.Key() == key {
			prior++
		}
	}
	view := studio.PracticeItem{
		Token:          p.Token,
		Kind:           p.Kind,
		ContentID:      item.Content.ID,
		ContentVersion: item.Content.Version,
		QuestionID:     item.Question.ID,
		Passage:        passageOf(item),
		Stem:           item.Question.Stem,
		Options:        studioOptions(item.Question.Options),
		Glossary:       studioGlossary(item.Question.Glossary),
		Index:          item.Number,
		Count:          len(item.Content.Questions),
		PriorAttempts:  prior,
		HintOpened:     !p.HintOpenedAt.IsZero(),
		PresentedAt:    source.FormatTime(p.PresentedAt),
		Skills:         append([]string{}, item.Question.Tactics...),
		Notice:         item.Content.Notice,
	}
	if item.Tactic != nil {
		view.TacticID, view.Stage = item.Tactic.ID, item.Tactic.Stage
		if item.Tactic.Stage == practice.StageGuided && item.Tactic.Lesson != nil {
			view.Guidance = hintOf(*item.Tactic.Lesson)
		}
	}
	if item.Skill != nil {
		view.SkillID = item.Skill.Skill.ID
	}
	guided := item.Tactic != nil && item.Tactic.Stage == practice.StageGuided
	assistance := attempt.AssistanceIndependent
	if guided || exposed || view.HintOpened {
		assistance = attempt.AssistanceAssisted
	}
	firstOnContent := practice.PassageExposureBefore(item.Content.ID, records, 0) == practice.PassageFresh
	switch adaptive.Classify(prior+1, assistance, firstOnContent) {
	case adaptive.ClassAssisted:
		view.Condition, view.ConditionReason = studio.ConditionAssisted,
			"a tactic walkthrough was shown for this question, so the attempt is assisted"
		if guided {
			view.ConditionReason = "the guided walkthrough is shown with this question, so the attempt is assisted"
		}
	case adaptive.ClassRevisit:
		view.Condition, view.ConditionReason = studio.ConditionRevisit,
			fmt.Sprintf("this question has %d earlier attempt(s); the answer and evidence were already shown", prior)
	case adaptive.ClassReExposure:
		view.Condition, view.ConditionReason = studio.ConditionReExposure,
			"this passage was already seen, so the attempt is repeat exposure, not independent transfer"
	default:
		view.Condition, view.ConditionReason = studio.ConditionIndependent,
			"first attempt with no help shown; it can count as independent evidence"
	}
	view.CanCount = view.Condition == studio.ConditionIndependent
	if lesson, ok := practice.HintTactic(item); ok && !guided {
		view.HintAvailable = true
		if view.HintOpened {
			view.Hint = hintOf(lesson)
		}
	} else if !guided {
		view.HintReason = "no authored walkthrough exists for this question"
	} else {
		view.HintReason = "the guided walkthrough is already shown"
	}
	return view, nil
}

// feedbackOf is the verified grade of a recorded attempt with every option's
// authored explanation and the evidence located in the passage.
func feedbackOf(item practice.Item, record attempt.Attempt, explained attempt.Feedback, records []attempt.Attempt, already bool) studio.Feedback {
	question := item.Question
	passage := passageOf(item)
	p := item.Presentation
	fb := studio.Feedback{
		Token:           p.Token,
		AttemptID:       record.ID,
		ContentID:       record.ContentID,
		QuestionID:      record.QuestionID,
		Verdict:         string(explained.Verdict),
		Key:             studio.Option{Label: explained.CorrectLabel, Text: explained.CorrectText},
		Reasoning:       explained.Reasoning,
		Assistance:      string(record.Assistance),
		Exposure:        explained.Exposure,
		AttemptNumber:   record.AttemptNumber,
		ElapsedMS:       record.ElapsedMS,
		PresentedAt:     source.FormatTime(p.PresentedAt),
		AnsweredAt:      source.FormatTime(record.AnsweredAt),
		Passage:         passage,
		Stem:            question.Stem,
		AlreadyAnswered: already,
		Evidence:        studio.EvidenceSpan{Text: explained.Evidence, Paragraph: -1},
	}
	if record.SelectedLabel != "" {
		fb.Selected = &studio.Option{Label: record.SelectedLabel, Text: record.SelectedText}
	}
	for _, option := range question.Options {
		text, ok := study.OptionExplanation(question.OptionReasoning, option.Label)
		fb.Options = append(fb.Options, studio.OptionExplanation{
			Label:       option.Label,
			Text:        option.Text,
			Explanation: text,
			Missing:     !ok,
			Correct:     option.Label == record.CorrectLabel,
			Selected:    option.Label == record.SelectedLabel,
		})
	}
	if span, ok := exam.LocateExcerpt(passage.Paragraphs, explained.Evidence); ok {
		fb.Evidence = studio.EvidenceSpan{Text: explained.Evidence, Located: true, Paragraph: span.Paragraph, Start: span.Start, End: span.End}
	}
	fb.Tactic = tacticLinkOf(record, question)
	firstOnContent := practice.PassageExposureBefore(record.ContentID, records, record.ID) == practice.PassageFresh
	switch adaptive.Classify(record.AttemptNumber, record.Assistance, firstOnContent) {
	case adaptive.ClassAssisted:
		fb.Condition = studio.ConditionAssisted
	case adaptive.ClassRevisit:
		fb.Condition = studio.ConditionRevisit
	case adaptive.ClassReExposure:
		fb.Condition = studio.ConditionReExposure
	default:
		fb.Condition = studio.ConditionIndependent
	}
	fb.Counted = fb.Condition == studio.ConditionIndependent
	return fb
}

// tacticLinkOf links feedback to the tactic lesson the attempt practiced, or
// the one its question is tagged with directly or through a scoring skill.
func tacticLinkOf(record attempt.Attempt, question exam.Question) *studio.TacticLink {
	ids := []string{}
	if record.TacticID != "" {
		ids = append(ids, record.TacticID)
	}
	for _, tag := range question.Tactics {
		ids = append(ids, tag)
		if lesson, ok := skill.Get(tag); ok && lesson.TacticID != "" {
			ids = append(ids, lesson.TacticID)
		}
	}
	for _, id := range ids {
		if lesson, ok := tactic.Get(id); ok {
			return &studio.TacticLink{ID: lesson.ID, Name: lesson.Title, Summary: lesson.Summary}
		}
	}
	return nil
}

func hintOf(lesson tactic.Tactic) *studio.Hint {
	return &studio.Hint{
		TacticID:    lesson.ID,
		Title:       lesson.Title,
		Principle:   lesson.Principle,
		WhyItWorks:  lesson.WhyItWorks,
		WhenItWorks: lesson.WhenItWorks,
		Steps:       append([]string{}, lesson.Steps...),
		Boundary: studio.Boundary{
			Folklore:   lesson.Boundary.Folklore,
			Example:    lesson.Boundary.Example,
			WhyItFails: lesson.Boundary.WhyItFails,
		},
	}
}

// passageOf splits the passage for display and labels its origin.
func passageOf(item practice.Item) studio.Passage {
	content := item.Content
	label := studio.SourceLabel{
		Kind:     studio.SourceFirstParty,
		Reviewed: true,
		Title:    content.Title,
		Rights:   content.Provenance.Rights,
	}
	switch {
	case item.Page != nil:
		label.Kind = studio.SourceImported
		label.SourceID, label.Page, _ = source.ParseContentID(content.ID)
		label.Reviewed = item.Page.Review == source.Reviewed &&
			(item.Review == nil || item.Review.Review == source.Reviewed)
	case isSampleContent(content.ID):
		label.Kind = studio.SourceSample
	}
	return studio.Passage{
		Title:      content.Passage.Title,
		Label:      label,
		Paragraphs: exam.Paragraphs(content.Passage.Text),
	}
}

func isSampleContent(id string) bool {
	sample, err := exam.Sample()
	return err == nil && sample.ID == id
}

func studioOptions(options []exam.Option) []studio.Option {
	out := make([]studio.Option, 0, len(options))
	for _, option := range options {
		out = append(out, studio.Option{Label: option.Label, Text: option.Text})
	}
	return out
}

func studioGlossary(entries []exam.GlossaryEntry) []studio.GlossaryEntry {
	out := make([]studio.GlossaryEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, studio.GlossaryEntry{Term: entry.Term, Meaning: entry.Meaning})
	}
	return out
}
