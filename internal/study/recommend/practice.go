package recommend

import (
	"context"

	"github.com/wxxb789/passquest/internal/skill"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
	"github.com/wxxb789/passquest/internal/study/practice"
	"github.com/wxxb789/passquest/internal/tactic"
)

// PracticeRequest resolves the adaptive recommendation to the practice request
// the desktop opens for a "next" start: the primary target, or the follow-up
// when the primary is a lesson. The refusals keep the messages the window
// showed before the practice module took over the display.
func PracticeRequest(ctx context.Context, s *study.Service) (practice.Request, error) {
	_ = ctx
	read, err := Recommend(ctx, s, "", SpendNone)
	if err != nil {
		return practice.Request{}, err
	}
	result := read.Adaptive
	if result.Recommendation == nil {
		return practice.Request{}, kernel.Refuse("no_recommendation", "no practiceable skill material is available")
	}
	target, followUp := Targets(result.Recommendation)
	if target != nil && target.Kind == TargetLesson {
		target = followUp
	}
	if target == nil {
		return practice.Request{}, kernel.Refuse("no_recommendation",
			"the recommendation for %s is a lesson with no practice to open", result.Recommendation.Skill.ID)
	}
	request := practice.Request{Kind: practice.KindNext}
	switch target.Kind {
	case TargetTactic:
		if _, ok := tactic.Get(target.SkillID); !ok {
			return practice.Request{}, kernel.Refuse("unknown_tactic", "tactic %q is not registered", target.SkillID)
		}
		request.TacticID, request.Stage = target.SkillID, practice.StageGuided
	default:
		if _, ok := skill.Get(target.SkillID); !ok {
			return practice.Request{}, kernel.Refuse("unknown_skill", "skill %q is not registered", target.SkillID)
		}
		request.SkillID = target.SkillID
		if target.Material != nil {
			request.SourceID = target.Material.SourceID
			if target.IncludeQuestion {
				request.QuestionID = target.Material.QuestionID
			}
		}
	}
	return request, nil
}
