package practice

import (
	"fmt"
	"strings"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/skill"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
	"github.com/wxxb789/passquest/internal/tactic"
)

// selectTacticPractice applies the tactic stage rule and picks the question a
// `tactic practice` invocation would show. A guided selection carries the
// walkthrough it delivers.
func selectTacticPractice(st *store.Store, lesson tactic.Tactic, requestedStage, sourceID, questionID string) (selection, error) {
	records, err := st.List()
	if err != nil {
		return selection{}, err
	}
	exposures, err := st.ListExposures()
	if err != nil {
		return selection{}, err
	}
	stage, err := resolveTacticStage(requestedStage, lesson.ID, records)
	if err != nil {
		return selection{}, err
	}
	candidates, err := tacticCandidates(st, lesson, sourceID)
	if err != nil {
		return selection{}, err
	}
	candidate, err := pickTacticCandidate(lesson, stage, questionID, candidates, records, exposures)
	if err != nil {
		return selection{}, err
	}
	tacticRun := &TacticRun{ID: lesson.ID, Title: lesson.Title, Stage: stage}
	if stage == StageGuided {
		tacticRun.Lesson = &lesson
	}
	return selection{
		Content:  candidate.Content,
		Question: candidate.Question,
		Number:   candidate.Number,
		Page:     candidate.Page,
		Review:   candidate.Review,
		Tactic:   tacticRun,
	}, nil
}

// resolveTacticStage applies the "after help" rule. An explicit stage is
// honoured; otherwise the first attempt is guided and later attempts are
// independent. Independent is refused until one guided attempt exists.
func resolveTacticStage(requested, tacticID string, records []attempt.Attempt) (string, error) {
	hasGuided := false
	for _, record := range records {
		if record.TacticID == tacticID && record.Assistance == attempt.AssistanceAssisted {
			hasGuided = true
			break
		}
	}
	switch strings.TrimSpace(requested) {
	case StageGuided:
		return StageGuided, nil
	case StageIndependent:
		if !hasGuided {
			return "", kernel.Refuse("guided_required",
				"complete a guided %s attempt before the independent question; the guided walkthrough is the help this stage follows", tacticID)
		}
		return StageIndependent, nil
	case "":
		if hasGuided {
			return StageIndependent, nil
		}
		return StageGuided, nil
	default:
		return "", kernel.Refuse("invalid_input", "unknown stage %q; use guided or independent", requested)
	}
}

func tacticCandidates(st *store.Store, lesson tactic.Tactic, sourceID string) ([]practiceCandidate, error) {
	if strings.TrimSpace(sourceID) == "" {
		return exerciseCandidates(lesson)
	}
	return sourceCandidates(st, sourceID, lesson.ID)
}

func exerciseCandidates(lesson tactic.Tactic) ([]practiceCandidate, error) {
	content, err := tactic.Exercise()
	if err != nil {
		return nil, err
	}
	tagged, err := tactic.Tagged(lesson.ID)
	if err != nil {
		return nil, err
	}
	out := make([]practiceCandidate, 0, len(tagged))
	for _, taggedQuestion := range tagged {
		question, number, ok := content.Question(taggedQuestion.ID)
		if !ok {
			return nil, fmt.Errorf("tagged exercise question %s is not in the exercise", taggedQuestion.ID)
		}
		out = append(out, practiceCandidate{
			Content:  content,
			Question: question,
			Number:   number,
		})
	}
	if len(out) == 0 {
		return nil, kernel.Refuse("not_practiceable", "the first-party exercise has no question tagged %q", lesson.ID)
	}
	return out, nil
}

func pickTacticCandidate(lesson tactic.Tactic, stage, questionID string, candidates []practiceCandidate, records []attempt.Attempt, exposures []store.Exposure) (practiceCandidate, error) {
	prior := make(map[attempt.QuestionKey]int, len(records))
	guided := make(map[attempt.QuestionKey]bool, len(records))
	for _, record := range records {
		prior[record.Key()]++
		if record.TacticID == lesson.ID && record.Assistance == attempt.AssistanceAssisted {
			guided[record.Key()] = true
		}
	}
	exposed := make(map[attempt.QuestionKey]bool, len(exposures))
	for _, exposure := range exposures {
		if exposure.TacticID == lesson.ID {
			exposed[exposure.Key()] = true
		}
	}

	if strings.TrimSpace(questionID) != "" {
		for _, candidate := range candidates {
			if candidate.Question.ID == questionID {
				if err := checkIndependent(stage, candidate, prior, guided, exposed); err != nil {
					return practiceCandidate{}, err
				}
				return candidate, nil
			}
		}
		return practiceCandidate{}, kernel.Refuse("invalid_input",
			"question %q is not a reviewed question tagged %q in this material", questionID, lesson.ID)
	}

	// A guided pre-answer request leaves a pending condition. The next guided
	// request without --question resumes that exposed question instead of
	// silently showing a different one.
	if stage == StageGuided {
		for _, candidate := range candidates {
			if exposed[candidate.key()] && prior[candidate.key()] == 0 {
				return candidate, nil
			}
		}
	}

	var fallback *practiceCandidate
	for i := range candidates {
		candidate := candidates[i]
		if fallback == nil {
			fallback = &candidate
		}
		if err := checkIndependent(stage, candidate, prior, guided, exposed); err != nil {
			continue
		}
		if stage == StageGuided && prior[candidate.key()] > 0 {
			continue
		}
		return candidate, nil
	}
	if stage == StageGuided && fallback != nil {
		// A second guided run is allowed; the attempt is recorded as assisted and
		// the existing repeat exposure remains visible.
		return *fallback, nil
	}
	if stage == StageIndependent {
		helped := 0
		for _, candidate := range candidates {
			if guided[candidate.key()] || exposed[candidate.key()] {
				helped++
			}
		}
		return practiceCandidate{}, kernel.Refuse("not_practiceable",
			"no unattempted question tagged %q remains distinct from the guided example (%d candidate(s), %d shown with guidance); add and review another tagged question, or repeat the guided example with --stage guided",
			lesson.ID, len(candidates), helped)
	}
	return practiceCandidate{}, kernel.Refuse("not_practiceable", "no practiceable question tagged %q", lesson.ID)
}

func checkIndependent(stage string, candidate practiceCandidate, prior map[attempt.QuestionKey]int, guided, exposed map[attempt.QuestionKey]bool) error {
	if stage != StageIndependent {
		return nil
	}
	key := candidate.key()
	if guided[key] {
		return kernel.Refuse("invalid_input", "question %s was the guided example; choose a different question", candidate.Question.ID)
	}
	if exposed[key] {
		return kernel.Refuse("invalid_input",
			"question %s was already shown with the guided walkthrough; choose a different question", candidate.Question.ID)
	}
	if prior[key] > 0 {
		return kernel.Refuse("not_practiceable",
			"question %s already has %d earlier attempt(s); an independent attempt must be a first attempt", candidate.Question.ID, prior[key])
	}
	return nil
}

// selectSkillPractice applies the guided-first rule for scoring skills and
// picks the question a `skill practice` invocation would show, with its
// assistance and passage exposure.
func selectSkillPractice(st *store.Store, lesson skill.Skill, sourceID, questionID string) (selection, error) {
	records, err := st.List()
	if err != nil {
		return selection{}, err
	}
	if err := requireGuidedTactic(lesson, records); err != nil {
		return selection{}, err
	}
	exposures, err := st.ListExposures()
	if err != nil {
		return selection{}, err
	}
	candidates, err := skillCandidates(st, lesson, sourceID)
	if err != nil {
		return selection{}, err
	}
	candidate, err := pickSkillCandidate(lesson, questionID, candidates, records, exposures)
	if err != nil {
		return selection{}, err
	}
	exposed, err := st.Exposed(candidate.key())
	if err != nil {
		return selection{}, err
	}
	assistance := attempt.AssistanceIndependent
	if exposed {
		assistance = attempt.AssistanceAssisted
	}
	return selection{
		Content:  candidate.Content,
		Question: candidate.Question,
		Number:   candidate.Number,
		Page:     candidate.Page,
		Review:   candidate.Review,
		Skill: &SkillRun{
			Skill:           lesson,
			Assistance:      assistance,
			PassageExposure: PassageExposureBefore(candidate.Content.ID, records, 0),
		},
	}, nil
}

func skillCandidates(st *store.Store, lesson skill.Skill, sourceID string) ([]practiceCandidate, error) {
	if strings.TrimSpace(sourceID) != "" {
		return sourceCandidates(st, sourceID, lesson.ID)
	}

	tagged, err := skill.Tagged(lesson.ID)
	if err != nil {
		return nil, err
	}
	out := make([]practiceCandidate, 0, len(tagged))
	for _, exercise := range tagged {
		out = append(out, practiceCandidate{
			Content: exercise.Content, Question: exercise.Question, Number: exercise.Number,
		})
	}
	if len(out) == 0 {
		return nil, kernel.Refuse("not_practiceable", "the first-party exercises have no question tagged %q", lesson.ID)
	}
	return out, nil
}

// pickSkillCandidate prefers material that can add independent transfer: a
// fresh passage first, then an unattempted question on a seen passage, then a
// repeat. A question whose guided walkthrough was already shown is skipped in
// the first two passes because the attempt would be assisted, not independent;
// an explicit --question still selects it and is recorded as assisted. A repeat
// is allowed but its exposure is printed and never counted as independent
// transfer by the adaptive policy.
func pickSkillCandidate(lesson skill.Skill, questionID string, candidates []practiceCandidate, records []attempt.Attempt, exposures []store.Exposure) (practiceCandidate, error) {
	attempted := make(map[attempt.QuestionKey]bool, len(records))
	seenContent := make(map[string]bool)
	for _, record := range records {
		attempted[record.Key()] = true
		seenContent[record.ContentID] = true
	}
	exposed := make(map[attempt.QuestionKey]bool, len(exposures))
	for _, exposure := range exposures {
		exposed[exposure.Key()] = true
	}
	if strings.TrimSpace(questionID) != "" {
		for _, candidate := range candidates {
			if candidate.Question.ID == questionID {
				return candidate, nil
			}
		}
		return practiceCandidate{}, kernel.Refuse("invalid_input",
			"question %q is not a practiceable question tagged %q in this material", questionID, lesson.ID)
	}
	for _, candidate := range candidates {
		if !seenContent[candidate.Content.ID] && !exposed[candidate.key()] {
			return candidate, nil
		}
	}
	for _, candidate := range candidates {
		if !attempted[candidate.key()] && !exposed[candidate.key()] {
			return candidate, nil
		}
	}
	return candidates[0], nil
}

// requireGuidedTactic applies Q-0003-02 to the scoring skill's practice surface.
// Ordinary language practice measures a prerequisite; scoring practice records
// independent evidence about a tactic, so it must follow the guided walkthrough
// rather than bypass it. Without the rule, a skill-practice answer on the
// guided-example question silently consumes it and makes the tactic's own
// independent stage unreachable.
func requireGuidedTactic(lesson skill.Skill, records []attempt.Attempt) error {
	if lesson.Kind != skill.KindScoring {
		return nil
	}
	for _, record := range records {
		if record.TacticID == lesson.ID && record.Assistance == attempt.AssistanceAssisted {
			return nil
		}
	}
	return kernel.Refuse("guided_required",
		"complete a guided %s attempt before independent %s practice; run passquest tactic practice %s --stage guided",
		lesson.ID, lesson.ID, lesson.ID)
}
