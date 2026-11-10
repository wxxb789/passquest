package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/wxxb789/passquest/internal/adaptive"
	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/skill"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
)

func runNext(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"source": true, "db": true})
	fs := newFlagSet("next", opt, wantJSON)
	sourceFlag := fs.String("source", "", "imported source id to include; embedded first-party material is always included")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}

	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	records, err := st.List()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	catalog, err := buildNextCatalog(st, *sourceFlag)
	if err != nil {
		if *sourceFlag != "" {
			return sourceError(opt, *jsonFlag, err)
		}
		return internalError(opt, *jsonFlag, err)
	}

	input := adaptive.Input{
		Skills:    nextSkills(),
		Materials: catalog.materials,
	}
	unclassified := make([]unclassifiedAttemptView, 0)
	for _, record := range records {
		adapted, reason := nextRecord(record, catalog.tagsByKey)
		if reason != "" {
			unclassified = append(unclassified, unclassifiedAttemptView{
				ID:         record.ID,
				ContentID:  record.ContentID,
				QuestionID: record.QuestionID,
				Reason:     reason,
			})
		}
		input.Records = append(input.Records, adapted)
	}

	result := adaptive.Recommend(input)
	doc := nextDocOf(result, unclassified, *sourceFlag, catalog.sourceTaggedQuestions)
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeNext(opt.Stdout, doc, st.Path())
	return 0
}

func nextSkills() []adaptive.Skill {
	all := skill.All()
	out := make([]adaptive.Skill, 0, len(all))
	for _, candidate := range all {
		out = append(out, adaptive.Skill{ID: candidate.ID, Kind: string(candidate.Kind), Title: candidate.Title})
	}
	return out
}

type nextCatalog struct {
	materials             []adaptive.Material
	tagsByKey             map[attempt.QuestionKey][]string
	sourceTaggedQuestions int
}

func buildNextCatalog(st *store.Store, sourceID string) (nextCatalog, error) {
	catalog := nextCatalog{tagsByKey: make(map[attempt.QuestionKey][]string)}
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
				catalog.sourceTaggedQuestions++
			}
		}
	}

	addSources := func(sourceIDs []string) error {
		for _, id := range sourceIDs {
			src, questions, err := loadSourceBundle(st, id)
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
		sample, err := exam.Sample()
		if err != nil {
			return err
		}
		// The sample is untagged, but cataloguing it distinguishes an untagged
		// attempt from an attempt on unavailable content.
		addContent(sample, "")
		contents, err := skill.PracticeContent()
		if err != nil {
			return err
		}
		for _, content := range contents {
			addContent(content, "")
		}
		return nil
	}

	if strings.TrimSpace(sourceID) != "" {
		// A scoped source is placed first so the recommendation's command uses
		// it when it has fresh material. Embedded content remains available.
		if err := addSources([]string{sourceID}); err != nil {
			return nextCatalog{}, err
		}
		if err := embedded(); err != nil {
			return nextCatalog{}, err
		}
		return catalog, nil
	}

	if err := embedded(); err != nil {
		return nextCatalog{}, err
	}
	summaries, err := st.Sources()
	if err != nil {
		return nextCatalog{}, err
	}
	ids := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		ids = append(ids, summary.ID)
	}
	if err := addSources(ids); err != nil {
		return nextCatalog{}, err
	}
	return catalog, nil
}

// nextRecord maps one attempt to the policy's shape. The snapshot is
// authoritative; a pre-v6 row without a snapshot falls back to the current
// question tags and is marked so the output can disclose it. An unmapped
// attempt is still passed to the policy with no skills so its content still
// counts as a seen passage.
func nextRecord(record attempt.Attempt, tagsByKey map[attempt.QuestionKey][]string) (adaptive.Record, string) {
	adapted := adaptive.Record{
		AttemptID:       record.ID,
		ContentID:       record.ContentID,
		QuestionID:      record.QuestionID,
		AttemptNumber:   record.AttemptNumber,
		Assistance:      record.Assistance,
		Status:          record.Status,
		ElapsedMS:       record.ElapsedMS,
		Skills:          append([]string(nil), record.Skills...),
		FromCurrentTags: false,
	}
	reason := ""
	if len(adapted.Skills) > 0 {
		for _, id := range adapted.Skills {
			if !skill.IsRegistered(id) {
				reason = fmt.Sprintf("recorded skill tags include %q, which is not registered in this build", id)
				adapted.Skills = nil
				break
			}
		}
		return adapted, reason
	}
	if current := tagsByKey[attempt.QuestionKey{ContentID: record.ContentID, QuestionID: record.QuestionID}]; len(current) > 0 {
		adapted.Skills = append([]string(nil), current...)
		adapted.FromCurrentTags = true
		return adapted, ""
	}
	if _, known := tagsByKey[attempt.QuestionKey{ContentID: record.ContentID, QuestionID: record.QuestionID}]; known {
		return adapted, "question has no registered skill tag"
	}
	return adapted, "content or question is unavailable in the current material"
}

func nextDocOf(result adaptive.Result, unclassified []unclassifiedAttemptView, sourceScope string, sourceTagged int) nextDoc {
	doc := nextDoc{
		SchemaVersion:         ProtocolVersion,
		State:                 "recommendation",
		Skills:                make([]nextSkillView, 0, len(result.Skills)),
		UnclassifiedAttempts:  unclassified,
		Limitations:           nextLimitations(len(unclassified)),
		SourceScope:           sourceScope,
		SourceTaggedQuestions: sourceTagged,
	}
	if result.Recommendation == nil {
		doc.State = "no_recommendation"
	}
	for _, candidate := range result.Skills {
		doc.Skills = append(doc.Skills, nextSkillView{
			Skill:     skillRefOf(candidate.Skill),
			Evidence:  nextEvidenceViewOf(candidate.Evidence),
			Inference: nextInferenceViewOf(candidate.Inference),
		})
	}
	if result.Recommendation != nil {
		command, followUp := nextCommands(result.Recommendation)
		doc.Recommendation = &nextRecommendationView{
			Action:             string(result.Recommendation.Action),
			Skill:              skillRefOf(result.Recommendation.Skill),
			State:              string(result.Recommendation.State),
			Reason:             result.Recommendation.Reason,
			Command:            command,
			FollowUpCommand:    followUp,
			EvidenceAttemptIDs: append([]int64{}, result.Recommendation.EvidenceAttemptIDs...),
		}
	}
	return doc
}

func nextCommands(rec *adaptive.Recommendation) (string, string) {
	switch rec.Action {
	case adaptive.ActionExplain:
		followUp := ""
		if rec.FreshMaterial != nil {
			followUp = skillPracticeCommand(rec.Skill.ID, rec.FreshMaterial, false)
		} else if rec.ReviewMaterial != nil {
			followUp = skillPracticeCommand(rec.Skill.ID, rec.ReviewMaterial, true)
		}
		if rec.Skill.Kind == adaptive.KindScoring {
			return fmt.Sprintf("passquest tactic practice %s --stage guided", rec.Skill.ID), followUp
		}
		return fmt.Sprintf("passquest skill show %s", rec.Skill.ID), followUp
	case adaptive.ActionPracticeNew:
		if rec.FreshMaterial != nil {
			return skillPracticeCommand(rec.Skill.ID, rec.FreshMaterial, false), ""
		}
		return fmt.Sprintf("passquest skill practice %s", rec.Skill.ID), ""
	case adaptive.ActionReview:
		if rec.ReviewMaterial != nil {
			return skillPracticeCommand(rec.Skill.ID, rec.ReviewMaterial, true), ""
		}
		return fmt.Sprintf("passquest skill show %s", rec.Skill.ID), ""
	default:
		return "", ""
	}
}

func skillPracticeCommand(skillID string, material *adaptive.Material, includeQuestion bool) string {
	parts := []string{"passquest", "skill", "practice", skillID}
	if material != nil && material.SourceID != "" {
		parts = append(parts, "--source", material.SourceID)
	}
	if material != nil && includeQuestion {
		parts = append(parts, "--question", material.QuestionID)
	}
	return strings.Join(parts, " ")
}

func skillRefOf(candidate adaptive.Skill) skillRefView {
	return skillRefView{ID: candidate.ID, Kind: candidate.Kind, Title: candidate.Title}
}

func nextEvidenceViewOf(ev adaptive.Evidence) nextEvidenceView {
	return nextEvidenceView{
		Attempts:                ev.Attempts,
		AssistedAttempts:        ev.AssistedAttempts,
		AssistedCorrect:         ev.AssistedCorrect,
		IndependentAttempts:     ev.IndependentAttempts,
		IndependentCorrect:      ev.IndependentCorrect,
		IndependentIncorrect:    ev.IndependentIncorrect,
		IndependentSkipped:      ev.IndependentSkipped,
		Revisits:                ev.Revisits,
		PassageRepeats:          ev.PassageRepeats,
		DistinctPassages:        ev.DistinctPassages,
		QuestionsSeen:           ev.QuestionsSeen,
		FreshPassages:           ev.FreshPassages,
		FastestIndependentMS:    ev.FastestIndependentMS,
		LatestIndependentMS:     ev.LatestIndependentMS,
		LatestIndependentStatus: string(ev.LatestIndependentStatus),
		LatestAttemptID:         ev.LatestAttemptID,
		LatestStatus:            string(ev.LatestStatus),
		LatestAssistance:        string(ev.LatestAssistance),
		AttemptIDs:              append([]int64{}, ev.AttemptIDs...),
		IndependentAttemptIDs:   append([]int64{}, ev.IndependentAttemptIDs...),
		MappedFromCurrentTags:   ev.MappedFromCurrentTags,
	}
}

func nextInferenceViewOf(inference adaptive.Inference) nextInferenceView {
	return nextInferenceView{
		State:           string(inference.State),
		Action:          string(inference.Action),
		Reason:          inference.Reason,
		CausalDiagnosis: inference.CausalDiagnosis,
		Uncertainty:     append([]string(nil), inference.Uncertainty...),
	}
}

func nextLimitations(unclassified int) []string {
	out := []string{
		"Skill evidence is inferred from the local attempts shown; it is not an exam score, pass estimate or readiness claim.",
		"Only a first independent attempt on a fresh passage counts as independent transfer; assisted, repeated and same-passage attempts are shown separately.",
		"The exam blueprint, time limit and pass threshold are unknown; elapsed time is local prompt-to-answer time, not an exam clock.",
		"Recommendations are recomputed from attempts and current question tags; no recommendation or derived learner state is stored.",
	}
	if unclassified > 0 {
		out = append(out, fmt.Sprintf("%d attempt(s) could not be mapped to a registered skill and are listed separately; they still mark their passage as seen.", unclassified))
	}
	return out
}

func writeNext(w io.Writer, doc nextDoc, dbPath string) {
	fmt.Fprintln(w, "Pass Quest — next practice")
	if doc.SourceScope != "" {
		fmt.Fprintf(w, "Imported source scope: %s (%d tagged question(s)); embedded first-party material is included.\n", doc.SourceScope, doc.SourceTaggedQuestions)
	}
	if doc.Recommendation == nil {
		fmt.Fprintln(w, "Recommendation: none (no practiceable skill material)")
	} else {
		rec := doc.Recommendation
		fmt.Fprintf(w, "Recommendation: %s — %s (%s)\n", rec.Action, rec.Skill.ID, rec.Skill.Kind)
		fmt.Fprintf(w, "State: %s\n", rec.State)
		fmt.Fprintf(w, "Why: %s\n", rec.Reason)
		fmt.Fprintf(w, "Command: %s\n", rec.Command)
		if rec.FollowUpCommand != "" {
			fmt.Fprintf(w, "Then: %s\n", rec.FollowUpCommand)
		}
	}
	fmt.Fprintln(w, "\nEvidence (recorded attempts)")
	for _, candidate := range doc.Skills {
		ev := candidate.Evidence
		latest := ""
		if ev.LatestStatus != "" {
			latest = "  latest " + ev.LatestStatus
		}
		fmt.Fprintf(w, "  %-24s %-8s attempts %d  independent %d (correct %d, incorrect %d, skipped %d)  assisted %d  revisits %d  passage-repeats %d  fresh passages %d%s\n",
			candidate.Skill.ID, candidate.Skill.Kind, ev.Attempts,
			ev.IndependentAttempts, ev.IndependentCorrect, ev.IndependentIncorrect, ev.IndependentSkipped,
			ev.AssistedAttempts, ev.Revisits, ev.PassageRepeats, ev.FreshPassages, latest)
	}
	fmt.Fprintln(w, "\nInference (recomputed from evidence)")
	for _, candidate := range doc.Skills {
		action := candidate.Inference.Action
		if action == "" {
			action = "none"
		}
		fmt.Fprintf(w, "  %-24s %s -> %s\n", candidate.Skill.ID, candidate.Inference.State, action)
		if candidate.Skill.ID == selectedSkillID(doc) {
			for _, line := range candidate.Inference.Uncertainty {
				fmt.Fprintf(w, "    uncertain: %s\n", line)
			}
		}
	}
	if len(doc.UnclassifiedAttempts) > 0 {
		fmt.Fprintf(w, "\nUnclassified attempts: %d\n", len(doc.UnclassifiedAttempts))
		for _, item := range doc.UnclassifiedAttempts {
			fmt.Fprintf(w, "  #%d  %s/%s  %s\n", item.ID, item.ContentID, item.QuestionID, item.Reason)
		}
	}
	fmt.Fprintln(w, "\nLimitations")
	for _, line := range doc.Limitations {
		fmt.Fprintf(w, "  - %s\n", line)
	}
	fmt.Fprintf(w, "\nDatabase: %s\n", dbPath)
}

func selectedSkillID(doc nextDoc) string {
	if doc.Recommendation == nil {
		return ""
	}
	return doc.Recommendation.Skill.ID
}

// JSON documents. Field names are protocol: tests and golden files pin them.

type skillRefView struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
}

type nextEvidenceView struct {
	Attempts                int     `json:"attempts"`
	AssistedAttempts        int     `json:"assisted_attempts"`
	AssistedCorrect         int     `json:"assisted_correct"`
	IndependentAttempts     int     `json:"independent_attempts"`
	IndependentCorrect      int     `json:"independent_correct"`
	IndependentIncorrect    int     `json:"independent_incorrect"`
	IndependentSkipped      int     `json:"independent_skipped"`
	Revisits                int     `json:"revisits"`
	PassageRepeats          int     `json:"passage_repeats"`
	DistinctPassages        int     `json:"distinct_passages"`
	QuestionsSeen           int     `json:"questions_seen"`
	FreshPassages           int     `json:"fresh_passages"`
	FastestIndependentMS    int64   `json:"fastest_independent_ms"`
	LatestIndependentMS     int64   `json:"latest_independent_ms"`
	LatestIndependentStatus string  `json:"latest_independent_status,omitempty"`
	LatestAttemptID         int64   `json:"latest_attempt_id,omitempty"`
	LatestStatus            string  `json:"latest_status,omitempty"`
	LatestAssistance        string  `json:"latest_assistance,omitempty"`
	AttemptIDs              []int64 `json:"attempt_ids"`
	IndependentAttemptIDs   []int64 `json:"independent_attempt_ids"`
	MappedFromCurrentTags   int     `json:"mapped_from_current_tags"`
}

type nextInferenceView struct {
	State           string   `json:"state"`
	Action          string   `json:"action"`
	Reason          string   `json:"reason"`
	CausalDiagnosis string   `json:"causal_diagnosis"`
	Uncertainty     []string `json:"uncertainty"`
}

type nextSkillView struct {
	Skill     skillRefView      `json:"skill"`
	Evidence  nextEvidenceView  `json:"evidence"`
	Inference nextInferenceView `json:"inference"`
}

type nextRecommendationView struct {
	Action             string       `json:"action"`
	Skill              skillRefView `json:"skill"`
	State              string       `json:"state"`
	Reason             string       `json:"reason"`
	Command            string       `json:"command"`
	FollowUpCommand    string       `json:"follow_up_command,omitempty"`
	EvidenceAttemptIDs []int64      `json:"evidence_attempt_ids"`
}

type unclassifiedAttemptView struct {
	ID         int64  `json:"id"`
	ContentID  string `json:"content_id"`
	QuestionID string `json:"question_id"`
	Reason     string `json:"reason"`
}

type nextDoc struct {
	SchemaVersion         int                       `json:"schema_version"`
	State                 string                    `json:"state"`
	SourceScope           string                    `json:"source_scope,omitempty"`
	SourceTaggedQuestions int                       `json:"source_tagged_questions"`
	Recommendation        *nextRecommendationView   `json:"recommendation,omitempty"`
	Skills                []nextSkillView           `json:"skills"`
	UnclassifiedAttempts  []unclassifiedAttemptView `json:"unclassified_attempts"`
	Limitations           []string                  `json:"limitations"`
}
