package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/wxxb789/passquest/internal/adaptive"
	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/skill"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/studio"
	"github.com/wxxb789/passquest/internal/tactic"
)

// StartPractice selects local practice with the same rules as the matching
// CLI command and displays it: the presentation is recorded now (Q-0011-08),
// spending the unit exactly as `show` does, and a guided tactic walkthrough is
// recorded as delivered. The returned token binds the answer to this display;
// any other open item is closed as abandoned. An empty kind is sample
// practice.
func (w *Workspace) StartPractice(request studio.PracticeRequest) (studio.PracticeItem, error) {
	if request.Kind == "" {
		request.Kind = studio.PracticeSample
	}
	var item studio.PracticeItem
	err := w.withStore(func(st *store.Store) error {
		if err := w.guard(st, "practice"); err != nil {
			return err
		}
		selection, err := w.selectPractice(st, request)
		if err != nil {
			return err
		}
		now := w.opt.Now()
		p := store.PracticePresentation{
			Token:          w.newToken(),
			Kind:           request.Kind,
			ContentID:      selection.Content.ID,
			ContentVersion: selection.Content.Version,
			QuestionID:     selection.Question.ID,
			PresentedAt:    now,
		}
		if selection.Page != nil {
			p.SourceID, _, _ = source.ParseContentID(selection.Content.ID)
		}
		if selection.Tactic != nil {
			p.TacticID, p.Stage = selection.Tactic.ID, selection.Tactic.Stage
		}
		if selection.Skill != nil {
			p.SkillID = selection.Skill.Skill.ID
		}
		if err := st.OpenPracticePresentation(p, practiceSurface(selection)); err != nil {
			return err
		}
		if err := recordGuidedExposure(st, selection.Tactic, selection.Content.ID, selection.Question.ID, now); err != nil {
			return err
		}
		stored, err := st.PracticePresentationByToken(p.Token)
		if err != nil {
			return err
		}
		item, err = practiceItemOf(st, selection, stored)
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
		p, found, err := st.LatestOpenPracticePresentation()
		if err != nil || !found {
			return err
		}
		selection, err := presentedSelection(st, p)
		if err != nil {
			return err
		}
		view, err := practiceItemOf(st, selection, p)
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
		p, err := openPresentation(st, token)
		if err != nil {
			return err
		}
		selection, err := presentedSelection(st, p)
		if err != nil {
			return err
		}
		lesson, ok := hintTactic(selection)
		if !ok {
			return refuse("hint_unavailable", "no authored walkthrough exists for question %s; the attempt stays as it is", selection.Question.ID)
		}
		now := w.opt.Now()
		if p, err = st.MarkPracticeHint(token, now); err != nil {
			return err
		}
		if err := st.RecordExposure(store.Exposure{
			TacticID:   lesson.ID,
			ContentID:  selection.Content.ID,
			QuestionID: selection.Question.ID,
			ExposedAt:  p.HintOpenedAt,
		}); err != nil {
			return err
		}
		item, err = practiceItemOf(st, selection, p)
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
	err := w.withStore(func(st *store.Store) error {
		p, err := st.PracticePresentationByToken(token)
		if errors.Is(err, store.ErrNotFound) {
			return refuse("unknown_token", "practice item %q was never displayed", token)
		}
		if err != nil {
			return err
		}
		if !p.Open() {
			feedback, err = storedFeedback(st, p)
			return err
		}
		if err := w.guard(st, "practice"); err != nil {
			return err
		}
		selection, err := presentedSelection(st, p)
		if err != nil {
			return err
		}
		chosen := attempt.Selection{Skip: true}
		if !skip {
			if chosen, err = attempt.ParseSelection(selection.Question, label); err != nil {
				return refuse("invalid_answer", "%s", err.Error())
			}
		}
		now := w.opt.Now()
		if now.Before(p.PresentedAt) {
			now = p.PresentedAt
		}
		record, err := buildPracticeAttempt(st, selection, chosen, w.opt.NewSession(), p.PresentedAt, now)
		if err != nil {
			return err
		}
		if !p.HintOpenedAt.IsZero() && record.Assistance != attempt.AssistanceAssisted {
			if record.TacticID != "" {
				record, err = record.WithTactic(record.TacticID, attempt.AssistanceAssisted)
			} else {
				record, err = record.WithAssistance(attempt.AssistanceAssisted)
			}
			if err != nil {
				return err
			}
		}
		stored, closed, err := st.AppendPracticeAttempt(token, record, now)
		if errors.Is(err, store.ErrPracticeClosed) {
			feedback, err = storedFeedback(st, closed)
			return err
		}
		if err != nil {
			return err
		}
		records, err := st.List()
		if err != nil {
			return err
		}
		feedback = feedbackOf(selection, stored, closed, records, false)
		return nil
	})
	return feedback, err
}

func openPresentation(st *store.Store, token string) (store.PracticePresentation, error) {
	p, err := st.PracticePresentationByToken(token)
	if errors.Is(err, store.ErrNotFound) {
		return p, refuse("unknown_token", "practice item %q was never displayed", token)
	}
	if err != nil {
		return p, err
	}
	if !p.Open() {
		return p, refuse("presentation_closed", "practice item %q was already %s", token, p.ClosedReason)
	}
	return p, nil
}

func storedFeedback(st *store.Store, p store.PracticePresentation) (studio.Feedback, error) {
	if p.ClosedReason != store.PracticeClosedAnswered || p.AttemptID == 0 {
		return studio.Feedback{}, refuse("presentation_closed", "practice item %q was %s without an answer", p.Token, p.ClosedReason)
	}
	record, err := tutorAttemptRecord(st, p.AttemptID)
	if err != nil {
		return studio.Feedback{}, err
	}
	selection, err := presentedSelection(st, p)
	if err != nil {
		return studio.Feedback{}, err
	}
	records, err := st.List()
	if err != nil {
		return studio.Feedback{}, err
	}
	return feedbackOf(selection, record, p, records, true), nil
}

// selectPractice applies the CLI selection rule for one request kind.
func (w *Workspace) selectPractice(st *store.Store, request studio.PracticeRequest) (practiceSelection, error) {
	switch request.Kind {
	case studio.PracticeSample:
		records, err := st.List()
		if err != nil {
			return practiceSelection{}, err
		}
		return selectSamplePractice(request.QuestionID, records)
	case studio.PracticeSource:
		if strings.TrimSpace(request.SourceID) == "" {
			return practiceSelection{}, refuse("invalid_input", "source practice needs a source id")
		}
		records, err := st.List()
		if err != nil {
			return practiceSelection{}, err
		}
		return selectSourcePractice(st, request.SourceID, request.Page, request.QuestionID, records)
	case studio.PracticeSkill:
		lesson, ok := skill.Get(request.Skill)
		if !ok {
			return practiceSelection{}, refuse("unknown_skill", "skill %q is not registered; available: %s",
				request.Skill, strings.Join(skill.Registered(), ", "))
		}
		return selectSkillPractice(st, lesson, request.SourceID, request.QuestionID)
	case studio.PracticeTactic:
		lesson, ok := tactic.Get(request.Tactic)
		if !ok {
			return practiceSelection{}, refuse("unknown_tactic", "tactic %q is not registered; available: %s",
				request.Tactic, strings.Join(tactic.Registered(), ", "))
		}
		return selectTacticPractice(st, lesson, request.Stage, request.SourceID, request.QuestionID)
	case studio.PracticeNext:
		return selectNextPractice(st)
	default:
		return practiceSelection{}, refuse("invalid_input", "unknown practice kind %q; use next, skill, tactic, sample or source", request.Kind)
	}
}

// selectNextPractice opens the practice the adaptive recommendation names:
// its primary target, or its follow-up when the primary is a lesson.
func selectNextPractice(st *store.Store) (practiceSelection, error) {
	input, _, _, err := buildAdaptiveInput(st, "")
	if err != nil {
		return practiceSelection{}, err
	}
	result := adaptive.Recommend(input)
	if result.Recommendation == nil {
		return practiceSelection{}, refuse("no_recommendation", "no practiceable skill material is available")
	}
	target, followUp := nextTargetsOf(result.Recommendation)
	if target != nil && target.kind == nextTargetLesson {
		target = followUp
	}
	if target == nil {
		return practiceSelection{}, refuse("no_recommendation",
			"the recommendation for %s is a lesson with no practice to open", result.Recommendation.Skill.ID)
	}
	if target.kind == nextTargetTactic {
		lesson, ok := tactic.Get(target.skillID)
		if !ok {
			return practiceSelection{}, refuse("unknown_tactic", "tactic %q is not registered", target.skillID)
		}
		return selectTacticPractice(st, lesson, stageGuided, "", "")
	}
	lesson, ok := skill.Get(target.skillID)
	if !ok {
		return practiceSelection{}, refuse("unknown_skill", "skill %q is not registered", target.skillID)
	}
	sourceID, questionID := "", ""
	if target.material != nil {
		sourceID = target.material.SourceID
		if target.includeQuestion {
			questionID = target.material.QuestionID
		}
	}
	return selectSkillPractice(st, lesson, sourceID, questionID)
}

// presentedSelection rebuilds the selection a presentation displayed, from
// the material its row names. Content that changed since the display is
// refused rather than graded against a different version.
func presentedSelection(st *store.Store, p store.PracticePresentation) (practiceSelection, error) {
	var (
		selection practiceSelection
		err       error
	)
	switch {
	case p.TacticID != "":
		lesson, ok := tactic.Get(p.TacticID)
		if !ok {
			return practiceSelection{}, refuse("content_unavailable", "tactic %q is not registered in this build", p.TacticID)
		}
		var candidates []practiceCandidate
		if candidates, err = tacticCandidates(st, lesson, p.SourceID); err == nil {
			selection, err = presentedCandidate(candidates, p)
			selection.Tactic = &tacticRun{ID: lesson.ID, Title: lesson.Title, Stage: p.Stage}
			if p.Stage == stageGuided {
				selection.Tactic.Guidance = guidanceFor(lesson)
			}
		}
	case p.SkillID != "":
		lesson, ok := skill.Get(p.SkillID)
		if !ok {
			return practiceSelection{}, refuse("content_unavailable", "skill %q is not registered in this build", p.SkillID)
		}
		var candidates []practiceCandidate
		if candidates, err = skillCandidates(st, lesson, p.SourceID); err == nil {
			if selection, err = presentedCandidate(candidates, p); err == nil {
				var records []attempt.Attempt
				if records, err = st.List(); err == nil {
					assistance := attempt.AssistanceIndependent
					var exposed bool
					if exposed, err = st.Exposed(attempt.QuestionKey{ContentID: p.ContentID, QuestionID: p.QuestionID}); exposed {
						assistance = attempt.AssistanceAssisted
					}
					selection.Skill = &skillRun{Skill: lesson, Assistance: assistance,
						PassageExposure: passageExposureBefore(p.ContentID, records, 0)}
				}
			}
		}
	case p.SourceID != "":
		_, page, _ := source.ParseContentID(p.ContentID)
		selection, err = selectSourcePractice(st, p.SourceID, page, p.QuestionID, nil)
	default:
		selection, err = selectSamplePractice(p.QuestionID, nil)
	}
	if err != nil {
		return practiceSelection{}, err
	}
	if selection.Content.ID != p.ContentID || selection.Question.ID != p.QuestionID || selection.Content.Version != p.ContentVersion {
		return practiceSelection{}, refuse("content_changed",
			"question %s of %s changed after it was displayed; start practice again", p.QuestionID, p.ContentID)
	}
	return selection, nil
}

func presentedCandidate(candidates []practiceCandidate, p store.PracticePresentation) (practiceSelection, error) {
	for _, c := range candidates {
		if c.Content.ID == p.ContentID && c.Question.ID == p.QuestionID {
			return practiceSelection{Content: c.Content, Question: c.Question, Number: c.Number, Page: c.Page, Review: c.Review}, nil
		}
	}
	return practiceSelection{}, refuse("content_changed",
		"question %s of %s is no longer practiceable; start practice again", p.QuestionID, p.ContentID)
}

// passageExposureBefore applies the skill-practice passage rule to the
// records before attempt id before (0: all records).
func passageExposureBefore(contentID string, records []attempt.Attempt, before int64) string {
	for _, record := range records {
		if before > 0 && record.ID >= before {
			break
		}
		if record.ContentID == contentID {
			return passageSeen
		}
	}
	return passageFresh
}

// hintTactic is the authored walkthrough a hint shows: the tactic being
// practiced, else the first registered tactic the question is tagged with.
func hintTactic(selection practiceSelection) (tactic.Tactic, bool) {
	if selection.Tactic != nil {
		if lesson, ok := tactic.Get(selection.Tactic.ID); ok {
			return lesson, true
		}
	}
	for _, tag := range selection.Question.Tactics {
		if lesson, ok := tactic.Get(tag); ok {
			return lesson, true
		}
	}
	return tactic.Tactic{}, false
}

// practiceItemOf renders a displayed selection. The condition is the one the
// recorded attempt will carry under adaptive.Classify, the rule Today's skill
// evidence counts by: assisted when help was shown, a revisit after an earlier
// attempt, a re-exposure on a passage that already has an attempt (whatever
// the practice kind), otherwise independent.
func practiceItemOf(st *store.Store, selection practiceSelection, p store.PracticePresentation) (studio.PracticeItem, error) {
	records, err := st.List()
	if err != nil {
		return studio.PracticeItem{}, err
	}
	key := attempt.QuestionKey{ContentID: selection.Content.ID, QuestionID: selection.Question.ID}
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
	item := studio.PracticeItem{
		Token:          p.Token,
		Kind:           p.Kind,
		ContentID:      selection.Content.ID,
		ContentVersion: selection.Content.Version,
		QuestionID:     selection.Question.ID,
		Passage:        passageOf(selection),
		Stem:           selection.Question.Stem,
		Options:        studioOptions(selection.Question.Options),
		Glossary:       studioGlossary(selection.Question.Glossary),
		Index:          selection.Number,
		Count:          len(selection.Content.Questions),
		PriorAttempts:  prior,
		HintOpened:     !p.HintOpenedAt.IsZero(),
		PresentedAt:    formatTime(p.PresentedAt),
		Skills:         append([]string{}, selection.Question.Tactics...),
		Notice:         selection.Content.Notice,
	}
	if selection.Tactic != nil {
		item.TacticID, item.Stage = selection.Tactic.ID, selection.Tactic.Stage
		if selection.Tactic.Stage == stageGuided {
			if lesson, ok := tactic.Get(selection.Tactic.ID); ok {
				item.Guidance = hintOf(lesson)
			}
		}
	}
	if selection.Skill != nil {
		item.SkillID = selection.Skill.Skill.ID
	}
	guided := selection.Tactic != nil && selection.Tactic.Stage == stageGuided
	assistance := attempt.AssistanceIndependent
	if guided || exposed || item.HintOpened {
		assistance = attempt.AssistanceAssisted
	}
	firstOnContent := passageExposureBefore(selection.Content.ID, records, 0) == passageFresh
	switch adaptive.Classify(prior+1, assistance, firstOnContent) {
	case adaptive.ClassAssisted:
		item.Condition, item.ConditionReason = studio.ConditionAssisted,
			"a tactic walkthrough was shown for this question, so the attempt is assisted"
		if guided {
			item.ConditionReason = "the guided walkthrough is shown with this question, so the attempt is assisted"
		}
	case adaptive.ClassRevisit:
		item.Condition, item.ConditionReason = studio.ConditionRevisit,
			fmt.Sprintf("this question has %d earlier attempt(s); the answer and evidence were already shown", prior)
	case adaptive.ClassReExposure:
		item.Condition, item.ConditionReason = studio.ConditionReExposure,
			"this passage was already seen, so the attempt is repeat exposure, not independent transfer"
	default:
		item.Condition, item.ConditionReason = studio.ConditionIndependent,
			"first attempt with no help shown; it can count as independent evidence"
	}
	item.CanCount = item.Condition == studio.ConditionIndependent
	if lesson, ok := hintTactic(selection); ok && !guided {
		item.HintAvailable = true
		if item.HintOpened {
			item.Hint = hintOf(lesson)
		}
	} else if !guided {
		item.HintReason = "no authored walkthrough exists for this question"
	} else {
		item.HintReason = "the guided walkthrough is already shown"
	}
	return item, nil
}

// feedbackOf is the verified grade of a recorded attempt with every option's
// authored explanation and the evidence located in the passage.
func feedbackOf(selection practiceSelection, record attempt.Attempt, p store.PracticePresentation, records []attempt.Attempt, already bool) studio.Feedback {
	question := selection.Question
	explained := attempt.Explain(question, record)
	passage := passageOf(selection)
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
		PresentedAt:     formatTime(p.PresentedAt),
		AnsweredAt:      formatTime(record.AnsweredAt),
		Passage:         passage,
		Stem:            question.Stem,
		AlreadyAnswered: already,
		Evidence:        studio.EvidenceSpan{Text: explained.Evidence, Paragraph: -1},
	}
	if record.SelectedLabel != "" {
		fb.Selected = &studio.Option{Label: record.SelectedLabel, Text: record.SelectedText}
	}
	for _, option := range question.Options {
		text, ok := optionExplanation(question.OptionReasoning, option.Label)
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
	firstOnContent := passageExposureBefore(record.ContentID, records, record.ID) == passageFresh
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
func passageOf(selection practiceSelection) studio.Passage {
	content := selection.Content
	label := studio.SourceLabel{
		Kind:     studio.SourceFirstParty,
		Reviewed: true,
		Title:    content.Title,
		Rights:   content.Provenance.Rights,
	}
	switch {
	case selection.Page != nil:
		label.Kind = studio.SourceImported
		label.SourceID, label.Page, _ = source.ParseContentID(content.ID)
		label.Reviewed = selection.Page.Review == source.Reviewed &&
			(selection.Review == nil || selection.Review.Review == source.Reviewed)
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
