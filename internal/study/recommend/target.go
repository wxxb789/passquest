package recommend

import "github.com/wxxb789/passquest/internal/adaptive"

// Target kinds: a guided tactic practice, a skill practice (optionally on
// named material) or a skill lesson.
const (
	TargetTactic = "tactic"
	TargetSkill  = "skill"
	TargetLesson = "lesson"
)

// Target is one destination a recommendation names. The CLI renders it as a
// command; the desktop workspace opens it directly.
type Target struct {
	Kind            string
	SkillID         string
	Material        *adaptive.Material
	IncludeQuestion bool
}

// Targets maps a recommendation to its primary destination and an optional
// follow-up.
func Targets(rec *adaptive.Recommendation) (primary, followUp *Target) {
	skillTarget := func(material *adaptive.Material, includeQuestion bool) *Target {
		return &Target{Kind: TargetSkill, SkillID: rec.Skill.ID, Material: material, IncludeQuestion: includeQuestion}
	}
	lesson := &Target{Kind: TargetLesson, SkillID: rec.Skill.ID}
	switch rec.Action {
	case adaptive.ActionExplain:
		if rec.FreshMaterial != nil {
			followUp = skillTarget(rec.FreshMaterial, false)
		} else if rec.ReviewMaterial != nil {
			followUp = skillTarget(rec.ReviewMaterial, true)
		}
		if rec.Skill.Kind == adaptive.KindScoring {
			return &Target{Kind: TargetTactic, SkillID: rec.Skill.ID}, followUp
		}
		return lesson, followUp
	case adaptive.ActionPracticeNew:
		return skillTarget(rec.FreshMaterial, false), nil
	case adaptive.ActionReview:
		if rec.ReviewMaterial != nil {
			return skillTarget(rec.ReviewMaterial, true), nil
		}
		return lesson, nil
	default:
		return nil, nil
	}
}
