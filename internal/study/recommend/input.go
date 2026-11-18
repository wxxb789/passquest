package recommend

import (
	"fmt"
	"strings"

	"github.com/wxxb789/passquest/internal/adaptive"
	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/skill"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
)

// Unclassified is one recorded attempt the policy could not map to a
// registered skill, with the reason the output discloses.
type Unclassified struct {
	ID         int64
	ContentID  string
	QuestionID string
	Reason     string
}

// Catalog is the practiceable material the policy input was built from and the
// questions that carry a registered skill tag when a source scopes the read.
type Catalog struct {
	materials             []adaptive.Material
	tagsByKey             map[attempt.QuestionKey][]string
	SourceTaggedQuestions int
}

// BuildInput maps every recorded attempt through the same skill snapshot and
// current-tag fallback the adaptive policy uses, and catalogs the practiceable
// material. It is shared by `next`, the desktop recommendation reads and the
// durable study brief so all read learner evidence the same way. It takes the
// store, not the unit of work, because a test that checks the recommended
// spend opens its own store beside the command and builds the input
// independently of the recommendation under test.
func BuildInput(st *store.Store, sourceID string) (adaptive.Input, []Unclassified, Catalog, error) {
	records, err := st.List()
	if err != nil {
		return adaptive.Input{}, nil, Catalog{}, err
	}
	catalog, err := buildCatalog(st, sourceID)
	if err != nil {
		return adaptive.Input{}, nil, Catalog{}, err
	}
	input := adaptive.Input{
		Skills:    nextSkills(),
		Materials: catalog.materials,
	}
	unclassified := make([]Unclassified, 0)
	for _, record := range records {
		adapted, reason := nextRecord(record, catalog.tagsByKey)
		if reason != "" {
			unclassified = append(unclassified, Unclassified{
				ID:         record.ID,
				ContentID:  record.ContentID,
				QuestionID: record.QuestionID,
				Reason:     reason,
			})
		}
		input.Records = append(input.Records, adapted)
	}
	return input, unclassified, catalog, nil
}

func nextSkills() []adaptive.Skill {
	all := skill.All()
	out := make([]adaptive.Skill, 0, len(all))
	for _, candidate := range all {
		out = append(out, adaptive.Skill{ID: candidate.ID, Kind: string(candidate.Kind), Title: candidate.Title})
	}
	return out
}

func buildCatalog(st *store.Store, sourceID string) (Catalog, error) {
	catalog := Catalog{tagsByKey: make(map[attempt.QuestionKey][]string)}
	seen := make(map[attempt.QuestionKey]map[string]bool)

	addContent := func(content exam.Exam, sourceID string) {
		for _, question := range content.Questions {
			key := attempt.QuestionKey{ContentID: content.ID, QuestionID: question.ID}
			if _, known := catalog.tagsByKey[key]; !known {
				catalog.tagsByKey[key] = []string{}
			}
			registered := 0
			for _, tag := range question.Tactics {
				if !skill.IsRegistered(tag) {
					continue
				}
				if seen[key] == nil {
					seen[key] = make(map[string]bool)
				}
				if seen[key][tag] {
					continue
				}
				seen[key][tag] = true
				catalog.tagsByKey[key] = append(catalog.tagsByKey[key], tag)
				catalog.materials = append(catalog.materials, adaptive.Material{
					SkillID:    tag,
					ContentID:  content.ID,
					QuestionID: question.ID,
					SourceID:   sourceID,
				})
				registered++
			}
			if sourceID != "" && registered > 0 {
				catalog.SourceTaggedQuestions++
			}
		}
	}

	addSources := func(sourceIDs []string) error {
		for _, id := range sourceIDs {
			src, questions, err := study.LoadSourceBundle(st, id)
			if err != nil {
				return err
			}
			for _, page := range src.Pages {
				content, _, buildErr := source.BuildPracticeExam(src, page, questions)
				if buildErr != nil {
					continue
				}
				addContent(content, src.ID)
			}
		}
		return nil
	}

	embedded := func() error {
		contents, err := study.EmbeddedContents()
		if err != nil {
			return err
		}
		// The sample is untagged, but cataloguing it distinguishes an untagged
		// attempt from an attempt on unavailable content.
		for _, content := range contents {
			addContent(content, "")
		}
		return nil
	}

	if strings.TrimSpace(sourceID) != "" {
		// A scoped source is placed first so the recommendation's command uses
		// it when it has fresh material. Embedded content remains available.
		if err := addSources([]string{sourceID}); err != nil {
			return Catalog{}, err
		}
		if err := embedded(); err != nil {
			return Catalog{}, err
		}
		return catalog, nil
	}

	if err := embedded(); err != nil {
		return Catalog{}, err
	}
	summaries, err := st.Sources()
	if err != nil {
		return Catalog{}, err
	}
	ids := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		ids = append(ids, summary.ID)
	}
	if err := addSources(ids); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

// nextRecord maps one attempt to the policy's shape. A recorded snapshot is
// authoritative even when it is empty: it means the question had no registered
// skill tag at answer time. Only a row written before the snapshot column
// existed (SkillsRecorded false) falls back to the current question tags, and
// that fallback is marked so the output can disclose it. An unmapped attempt is
// still passed to the policy with no skills so its content still counts as a
// seen passage.
func nextRecord(record attempt.Attempt, tagsByKey map[attempt.QuestionKey][]string) (adaptive.Record, string) {
	key := attempt.QuestionKey{ContentID: record.ContentID, QuestionID: record.QuestionID}
	current, currentKnown := tagsByKey[key]
	ids, fromCurrent, unregistered := study.EffectiveSkills(record, current, currentKnown)
	adapted := adaptive.Record{
		AttemptID:       record.ID,
		ContentID:       record.ContentID,
		QuestionID:      record.QuestionID,
		AttemptNumber:   record.AttemptNumber,
		Assistance:      record.Assistance,
		Status:          record.Status,
		ElapsedMS:       record.ElapsedMS,
		Skills:          ids,
		FromCurrentTags: fromCurrent,
		TacticID:        record.TacticID,
	}
	if record.SkillsRecorded {
		if len(record.Skills) == 0 {
			return adapted, "question had no registered skill tag when the attempt was recorded"
		}
		if unregistered != "" {
			return adapted, fmt.Sprintf("recorded skill tags include %q, which is not registered in this build", unregistered)
		}
		return adapted, ""
	}
	if len(ids) > 0 {
		return adapted, ""
	}
	if currentKnown {
		return adapted, "pre-v6 attempt on a question with no registered skill tag"
	}
	return adapted, "content or question is unavailable in the current material"
}
