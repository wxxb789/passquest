package practice

import (
	"errors"
	"time"

	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/skill"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
	"github.com/wxxb789/passquest/internal/tactic"
)

// practiceSurface is the holdout-ledger surface of a practice selection.
func practiceSurface(sel selection) string {
	surface := assess.SurfacePractice
	if sel.Tactic != nil {
		surface = assess.SurfaceTactic
	}
	if sel.Skill != nil {
		surface = assess.SurfaceSkill
	}
	return surface
}

// buildPracticeAttempt grades a selection and records the condition it was
// made under, without storing it.
func buildPracticeAttempt(st *store.Store, sel selection, chosen attempt.Selection, sessionID string, started, answered time.Time) (attempt.Attempt, error) {
	record, err := attempt.BuildRecord(sel.Content.ID, sel.Content.Version, sel.Question, chosen, sessionID, started, answered)
	if err != nil {
		return attempt.Attempt{}, err
	}
	return annotateAssistance(st, sel, record)
}

// annotateAssistance records the condition under which the attempt was made. A
// tactic run carries its stage and id. Ordinary practice has no tactic id, but
// a question whose guided walkthrough was already delivered is still assisted:
// the help was shown even though this command did not show it.
func annotateAssistance(st *store.Store, sel selection, record attempt.Attempt) (attempt.Attempt, error) {
	if sel.Tactic != nil {
		return record.WithTactic(sel.Tactic.ID, sel.Tactic.assistance())
	}
	exposed, err := st.Exposed(record.Key())
	if err != nil {
		return attempt.Attempt{}, err
	}
	if !exposed {
		return record, nil
	}
	return record.WithAssistance(attempt.AssistanceAssisted)
}

// recordGuidedExposure persists that the walkthrough is being delivered. The
// pre-answer document and the interactive prompt show help before any answer,
// so the question must stop qualifying as independent evidence even when the
// learner never answers it. The answered document's assisted attempt already
// carries the help for a one-shot answer.
func recordGuidedExposure(st *store.Store, sel selection, at time.Time) error {
	if sel.Tactic == nil || sel.Tactic.Stage != StageGuided {
		return nil
	}
	return st.RecordExposure(store.Exposure{
		TacticID:   sel.Tactic.ID,
		ContentID:  sel.Content.ID,
		QuestionID: sel.Question.ID,
		ExposedAt:  at,
	})
}

// openPresentation reads an open presentation. An unknown token and a closed
// item are refusals with the code and message the window showed before this
// step.
func openPresentation(st *store.Store, token string) (store.PracticePresentation, error) {
	p, err := st.PracticePresentationByToken(token)
	if errors.Is(err, store.ErrNotFound) {
		return p, kernel.Refuse("unknown_token", "practice item %q was never displayed", token)
	}
	if err != nil {
		return p, err
	}
	if !p.Open() {
		return p, kernel.Refuse("presentation_closed", "practice item %q was already %s", token, p.ClosedReason)
	}
	return p, nil
}

// storedGraded returns the stored feedback of a token that was already
// answered, as the grade of a replayed answer.
func storedGraded(st *store.Store, p store.PracticePresentation) (Graded, error) {
	if p.ClosedReason != store.PracticeClosedAnswered || p.AttemptID == 0 {
		return Graded{}, kernel.Refuse("presentation_closed", "practice item %q was %s without an answer", p.Token, p.ClosedReason)
	}
	record, err := attemptByID(st, p.AttemptID)
	if err != nil {
		return Graded{}, err
	}
	sel, err := presentedSelection(st, p)
	if err != nil {
		return Graded{}, err
	}
	records, err := st.List()
	if err != nil {
		return Graded{}, err
	}
	return Graded{
		Item:     sel.item(&p, ref{hold: HoldToken, token: p.Token}),
		Attempt:  record,
		Feedback: attempt.Explain(sel.Question, record),
		Records:  records,
		Replayed: true,
	}, nil
}

// attemptByID reads one recorded attempt.
func attemptByID(st *store.Store, id int64) (attempt.Attempt, error) {
	records, err := st.List()
	if err != nil {
		return attempt.Attempt{}, err
	}
	for _, record := range records {
		if record.ID == id {
			return record, nil
		}
	}
	return attempt.Attempt{}, kernel.Refuse("unknown_attempt", "attempt #%d is not recorded", id)
}

// presentedSelection rebuilds the selection a presentation displayed, from
// the material its row names. Content that changed since the display is
// refused rather than graded against a different version.
func presentedSelection(st *store.Store, p store.PracticePresentation) (selection, error) {
	var (
		sel selection
		err error
	)
	switch {
	case p.TacticID != "":
		lesson, ok := tactic.Get(p.TacticID)
		if !ok {
			return selection{}, kernel.Refuse("content_unavailable", "tactic %q is not registered in this build", p.TacticID)
		}
		var candidates []practiceCandidate
		if candidates, err = tacticCandidates(st, lesson, p.SourceID); err == nil {
			sel, err = presentedCandidate(candidates, p)
			sel.Tactic = &TacticRun{ID: lesson.ID, Title: lesson.Title, Stage: p.Stage}
			if p.Stage == StageGuided {
				sel.Tactic.Lesson = &lesson
			}
		}
	case p.SkillID != "":
		lesson, ok := skill.Get(p.SkillID)
		if !ok {
			return selection{}, kernel.Refuse("content_unavailable", "skill %q is not registered in this build", p.SkillID)
		}
		var candidates []practiceCandidate
		if candidates, err = skillCandidates(st, lesson, p.SourceID); err == nil {
			if sel, err = presentedCandidate(candidates, p); err == nil {
				var records []attempt.Attempt
				if records, err = st.List(); err == nil {
					assistance := attempt.AssistanceIndependent
					var exposed bool
					if exposed, err = st.Exposed(attempt.QuestionKey{ContentID: p.ContentID, QuestionID: p.QuestionID}); exposed {
						assistance = attempt.AssistanceAssisted
					}
					sel.Skill = &SkillRun{Skill: lesson, Assistance: assistance,
						PassageExposure: PassageExposureBefore(p.ContentID, records, 0)}
				}
			}
		}
	case p.SourceID != "":
		_, page, _ := source.ParseContentID(p.ContentID)
		sel, err = selectSourcePractice(st, p.SourceID, page, p.QuestionID, nil)
	default:
		sel, err = selectSamplePractice(p.QuestionID, nil)
	}
	if err != nil {
		return selection{}, err
	}
	if sel.Content.ID != p.ContentID || sel.Question.ID != p.QuestionID || sel.Content.Version != p.ContentVersion {
		return selection{}, kernel.Refuse("content_changed",
			"question %s of %s changed after it was displayed; start practice again", p.QuestionID, p.ContentID)
	}
	return sel, nil
}

func presentedCandidate(candidates []practiceCandidate, p store.PracticePresentation) (selection, error) {
	for _, c := range candidates {
		if c.Content.ID == p.ContentID && c.Question.ID == p.QuestionID {
			return selection{Content: c.Content, Question: c.Question, Number: c.Number, Page: c.Page, Review: c.Review}, nil
		}
	}
	return selection{}, kernel.Refuse("content_changed",
		"question %s of %s is no longer practiceable; start practice again", p.QuestionID, p.ContentID)
}

// PassageExposureBefore applies the skill-practice passage rule to the
// records before attempt id before (0: all records). The desktop adapter
// computes the same condition over the same records, so it calls this.
func PassageExposureBefore(contentID string, records []attempt.Attempt, before int64) string {
	for _, record := range records {
		if before > 0 && record.ID >= before {
			break
		}
		if record.ContentID == contentID {
			return PassageSeen
		}
	}
	return PassageFresh
}

// HintTactic is the authored walkthrough a hint shows: the tactic being
// practiced, else the first registered tactic the question is tagged with.
// The desktop adapter asks the same question of an item it already has.
func HintTactic(item Item) (tactic.Tactic, bool) {
	if item.Tactic != nil {
		if lesson, ok := tactic.Get(item.Tactic.ID); ok {
			return lesson, true
		}
	}
	for _, tag := range item.Question.Tactics {
		if lesson, ok := tactic.Get(tag); ok {
			return lesson, true
		}
	}
	return tactic.Tactic{}, false
}
