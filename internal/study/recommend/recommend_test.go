package recommend

import (
	"slices"
	"testing"

	"github.com/wxxb789/passquest/internal/attempt"
)

func TestNextRecordClassification(t *testing.T) {
	t.Parallel()
	key := attempt.QuestionKey{ContentID: "content", QuestionID: "q1"}
	record := attempt.Attempt{ID: 7, ContentID: key.ContentID, QuestionID: key.QuestionID, AttemptNumber: 1}
	tests := []struct {
		name           string
		record         attempt.Attempt
		tags           map[attempt.QuestionKey][]string
		reason         string
		wantSkills     []string
		fromCurrentTag bool
	}{
		{
			name:   "recorded attempt on a question without a registered tag",
			record: attempt.Attempt{ID: 7, ContentID: key.ContentID, QuestionID: key.QuestionID, AttemptNumber: 1, SkillsRecorded: true},
			tags:   map[attempt.QuestionKey][]string{key: {"vocabulary-in-context"}},
			reason: "question had no registered skill tag when the attempt was recorded",
		},
		{
			name:   "pre-v6 attempt maps from current tags",
			record: record,
			tags:   map[attempt.QuestionKey][]string{key: {"vocabulary-in-context"}},
			reason: "",
			wantSkills: []string{
				"vocabulary-in-context",
			},
			fromCurrentTag: true,
		},
		{
			name:   "pre-v6 attempt on a known untagged question",
			record: record,
			tags:   map[attempt.QuestionKey][]string{key: {}},
			reason: "pre-v6 attempt on a question with no registered skill tag",
		},
		{
			name:   "content or question unavailable",
			record: record,
			tags:   map[attempt.QuestionKey][]string{},
			reason: "content or question is unavailable in the current material",
		},
		{
			name:   "snapshot names an unregistered skill",
			record: attempt.Attempt{ID: 8, ContentID: "content", QuestionID: "q2", AttemptNumber: 1, Skills: []string{"retired-skill"}, SkillsRecorded: true},
			tags:   map[attempt.QuestionKey][]string{},
			reason: `recorded skill tags include "retired-skill", which is not registered in this build`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapted, reason := nextRecord(tt.record, tt.tags)
			if reason != tt.reason {
				t.Fatalf("reason = %q, want %q", reason, tt.reason)
			}
			if adapted.AttemptID != tt.record.ID {
				t.Fatalf("attempt id = %d, want %d", adapted.AttemptID, tt.record.ID)
			}
			if !slices.Equal(adapted.Skills, tt.wantSkills) || adapted.FromCurrentTags != tt.fromCurrentTag {
				t.Fatalf("record = %+v, want skills %v and fromCurrentTags %v", adapted, tt.wantSkills, tt.fromCurrentTag)
			}
		})
	}
}
