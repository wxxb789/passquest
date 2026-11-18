package study

import (
	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/skill"
)

// EffectiveSkills resolves the skill ids an attempt is classified under. A
// recorded snapshot is authoritative, even when it is empty: it records the
// tags the question had when the answer was stored. Only a pre-v6 row falls
// back to the question's current tags, and that fallback is marked so callers
// can disclose it. An unregistered id in a recorded snapshot invalidates the
// whole snapshot, so evidence is never silently regrouped under the remaining
// tags. Callers: the recommendation input and the tutor target's skill
// lessons.
func EffectiveSkills(record attempt.Attempt, current []string, currentKnown bool) (ids []string, fromCurrent bool, unregistered string) {
	raw := record.Skills
	if !record.SkillsRecorded {
		if !currentKnown {
			return nil, false, ""
		}
		raw = current
		fromCurrent = len(current) > 0
	}
	seen := make(map[string]bool)
	for _, id := range raw {
		if !skill.IsRegistered(id) {
			if record.SkillsRecorded {
				return nil, false, id
			}
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, fromCurrent, ""
}
