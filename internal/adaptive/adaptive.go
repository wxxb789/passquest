// Package adaptive infers a practice recommendation from recorded attempts.
// It is a pure policy package: it reads attempt facts and a material catalog and
// writes nothing. The recorded attempt is the evidence; every recommendation can
// be recomputed after reopening the database.
package adaptive

import (
	"fmt"
	"sort"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
)

// Skill kinds understood by the policy. The kind decides whether a new skill
// starts with an explanation (the scoring tactic) or a fresh measurement (a
// language prerequisite).
const (
	KindScoring  = "scoring"
	KindLanguage = "language"
)

// Action is the recommended next practice action.
type Action string

const (
	ActionExplain     Action = "explain"
	ActionPracticeNew Action = "practice_new"
	ActionReview      Action = "review"
)

// State is the inferred evidence state for one skill. It describes recorded
// transfer evidence, not a readiness or pass estimate.
type State string

const (
	StateNoEvidence      State = "no_evidence"
	StateFamiliarityOnly State = "familiarity_only"
	StateInsufficient    State = "insufficient"
	StateWeak            State = "weak"
	StateMixed           State = "mixed"
	StateSupported       State = "supported"
	StateNoMaterial      State = "no_material"
)

// Skill is the policy's view of a registered skill.
type Skill struct {
	ID    string
	Kind  string
	Title string
}

// Record is one recorded attempt as the policy sees it. Skills is the snapshot
// stored on the attempt; FromCurrentTags marks a pre-v6 row whose mapping came
// from the question's current tags instead.
type Record struct {
	AttemptID       int64
	ContentID       string
	QuestionID      string
	AttemptNumber   int
	Assistance      attempt.Assistance
	Status          attempt.Status
	ElapsedMS       int64
	AnsweredAt      time.Time
	Skills          []string
	FromCurrentTags bool
}

// Material is one practiceable tagged question in the current catalog. SourceID
// is empty for embedded first-party content.
type Material struct {
	SkillID      string
	ContentID    string
	QuestionID   string
	ContentTitle string
	SourceID     string
}

// Input is the complete policy input.
type Input struct {
	Skills    []Skill
	Records   []Record
	Materials []Material
}

// Evidence is the recorded-fact side of a skill result. It contains no inferred
// label: the caller can inspect exactly which attempts produced it.
type Evidence struct {
	Attempts                int
	AssistedAttempts        int
	AssistedCorrect         int
	IndependentAttempts     int
	IndependentCorrect      int
	IndependentIncorrect    int
	IndependentSkipped      int
	Revisits                int
	PassageRepeats          int
	DistinctPassages        int
	QuestionsSeen           int
	FreshPassages           int
	FastestIndependentMS    int64
	LatestIndependentMS     int64
	LatestIndependentStatus attempt.Status
	LatestAttemptID         int64
	LatestStatus            attempt.Status
	LatestAssistance        attempt.Assistance
	AttemptIDs              []int64
	IndependentAttemptIDs   []int64
	MappedFromCurrentTags   int
}

// Inference is the derived side of a skill result. State, action and reason are
// recomputed; they are not stored.
type Inference struct {
	State           State
	Action          Action
	Reason          string
	CausalDiagnosis string
	Uncertainty     []string
}

// SkillResult pairs a skill with its evidence and inference.
type SkillResult struct {
	Skill     Skill
	Evidence  Evidence
	Inference Inference

	priority       int
	freshMaterials []Material
	reviewMaterial *Material
}

// Recommendation is the selected next action.
type Recommendation struct {
	Action Action
	Skill  Skill
	State  State
	Reason string
	// EvidenceAttemptIDs lists every attempt that contributed to the selected
	// skill, so the reason can be checked against rows.
	EvidenceAttemptIDs []int64
	// FreshMaterial is the first material on an unseen passage. ReviewMaterial
	// is the latest seen material. Both are presentation inputs, not evidence.
	FreshMaterial  *Material
	ReviewMaterial *Material
}

// Result is the complete policy output.
type Result struct {
	Recommendation *Recommendation
	Skills         []SkillResult
}

type skillState struct {
	evidence       Evidence
	answers        int
	latestAnswered attempt.Status
	fresh          []Material
	review         *Material
	passages       map[string]bool
	questions      map[attempt.QuestionKey]bool
}

// Recommend classifies every record, infers a state per skill and selects the
// highest-need recommendation. It never mutates the input.
func Recommend(input Input) Result {
	skills := make([]Skill, 0, len(input.Skills))
	byID := make(map[string]Skill, len(input.Skills))
	for _, candidate := range input.Skills {
		if candidate.ID == "" || byID[candidate.ID].ID != "" {
			continue
		}
		skills = append(skills, candidate)
		byID[candidate.ID] = candidate
	}

	materialsBySkill := make(map[string][]Material, len(skills))
	materialByKey := make(map[attempt.QuestionKey]map[string]Material)
	seenMaterial := make(map[attempt.QuestionKey]map[string]bool)
	for _, material := range input.Materials {
		if byID[material.SkillID].ID == "" {
			continue
		}
		key := attempt.QuestionKey{ContentID: material.ContentID, QuestionID: material.QuestionID}
		if materialByKey[key] == nil {
			materialByKey[key] = make(map[string]Material)
			seenMaterial[key] = make(map[string]bool)
		}
		if seenMaterial[key][material.SkillID] {
			continue
		}
		seenMaterial[key][material.SkillID] = true
		materialByKey[key][material.SkillID] = material
		materialsBySkill[material.SkillID] = append(materialsBySkill[material.SkillID], material)
	}

	records := append([]Record(nil), input.Records...)
	sort.SliceStable(records, func(i, j int) bool { return records[i].AttemptID < records[j].AttemptID })

	states := make(map[string]*skillState, len(skills))
	for _, candidate := range skills {
		states[candidate.ID] = &skillState{
			passages:  make(map[string]bool),
			questions: make(map[attempt.QuestionKey]bool),
		}
	}

	seenContent := make(map[string]bool)
	contentAttempted := make(map[string]bool)
	for _, record := range records {
		firstOnContent := !seenContent[record.ContentID]
		seenContent[record.ContentID] = true
		contentAttempted[record.ContentID] = true

		recordSkills := unique(record.Skills)
		for _, skillID := range recordSkills {
			st := states[skillID]
			if st == nil {
				continue
			}
			ev := &st.evidence
			key := attempt.QuestionKey{ContentID: record.ContentID, QuestionID: record.QuestionID}
			ev.Attempts++
			ev.AttemptIDs = append(ev.AttemptIDs, record.AttemptID)
			st.passages[record.ContentID] = true
			st.questions[key] = true
			ev.LatestAttemptID = record.AttemptID
			ev.LatestStatus = record.Status
			ev.LatestAssistance = record.Assistance

			if record.FromCurrentTags {
				ev.MappedFromCurrentTags++
			}
			if record.Assistance == attempt.AssistanceAssisted {
				ev.AssistedAttempts++
				if record.Status == attempt.StatusCorrect {
					ev.AssistedCorrect++
				}
			}
			if record.AttemptNumber > 1 {
				ev.Revisits++
			}
			if record.AttemptNumber == 1 && record.Assistance == attempt.AssistanceIndependent {
				if firstOnContent {
					ev.IndependentAttempts++
					ev.IndependentAttemptIDs = append(ev.IndependentAttemptIDs, record.AttemptID)
					switch record.Status {
					case attempt.StatusCorrect:
						ev.IndependentCorrect++
					case attempt.StatusIncorrect:
						ev.IndependentIncorrect++
					case attempt.StatusSkipped:
						ev.IndependentSkipped++
					}
					if record.Status != attempt.StatusSkipped {
						st.answers++
						st.latestAnswered = record.Status
					}
					ev.LatestIndependentMS = record.ElapsedMS
					ev.LatestIndependentStatus = record.Status
					if record.Status == attempt.StatusCorrect &&
						(ev.FastestIndependentMS == 0 || record.ElapsedMS < ev.FastestIndependentMS) {
						ev.FastestIndependentMS = record.ElapsedMS
					}
				} else {
					ev.PassageRepeats++
				}
			}
			if material, ok := materialByKey[key]; ok {
				if candidate, ok := material[skillID]; ok {
					st.review = &candidate
				}
			}
		}
	}

	result := Result{Skills: make([]SkillResult, 0, len(skills))}
	for _, candidate := range skills {
		st := states[candidate.ID]
		ev := st.evidence
		ev.DistinctPassages = len(st.passages)
		ev.QuestionsSeen = len(st.questions)
		for _, material := range materialsBySkill[candidate.ID] {
			if !contentAttempted[material.ContentID] {
				st.fresh = append(st.fresh, material)
			}
		}
		ev.FreshPassages = len(freshContentIDs(st.fresh))

		state, action, reason, priority := infer(candidate, ev, st, len(materialsBySkill[candidate.ID]))
		inference := Inference{
			State:           state,
			Action:          action,
			Reason:          reason,
			CausalDiagnosis: causalDiagnosis(ev),
			Uncertainty:     uncertainty(candidate, ev, state),
		}
		skillResult := SkillResult{
			Skill:          candidate,
			Evidence:       ev,
			Inference:      inference,
			priority:       priority,
			freshMaterials: st.fresh,
		}
		skillResult.reviewMaterial = st.review
		result.Skills = append(result.Skills, skillResult)
	}

	var best *SkillResult
	bestRank := 0
	for i := range result.Skills {
		candidate := &result.Skills[i]
		if candidate.Inference.State == StateNoMaterial {
			continue
		}
		if best == nil || candidate.priority < bestRank {
			best = candidate
			bestRank = candidate.priority
		}
	}
	if best != nil {
		recommendation := &Recommendation{
			Action:             best.Inference.Action,
			Skill:              best.Skill,
			State:              best.Inference.State,
			Reason:             best.Inference.Reason,
			EvidenceAttemptIDs: append([]int64(nil), best.Evidence.AttemptIDs...),
		}
		if len(best.freshMaterials) > 0 {
			material := best.freshMaterials[0]
			recommendation.FreshMaterial = &material
		}
		if best.reviewMaterial != nil {
			material := *best.reviewMaterial
			recommendation.ReviewMaterial = &material
		}
		result.Recommendation = recommendation
	}
	return result
}

func infer(candidate Skill, ev Evidence, st *skillState, materialCount int) (State, Action, string, int) {
	if materialCount == 0 {
		return StateNoMaterial, "", fmt.Sprintf("No practiceable question tagged %s is available in the current material; add and review a tagged question or import a source.", candidate.ID), 99
	}
	answers := st.answers
	fresh := len(st.fresh) > 0
	review := st.review != nil
	latestIncorrect := st.latestAnswered == attempt.StatusIncorrect

	switch {
	case ev.Attempts == 0:
		if candidate.Kind == KindScoring {
			return StateNoEvidence, ActionExplain,
				fmt.Sprintf("No attempt is recorded for %s. The scoring tactic starts with a guided walkthrough, so the explanation is selected before independent evidence; the guided attempt is recorded as assisted.", candidate.ID), 5
		}
		return StateNoEvidence, ActionPracticeNew,
			fmt.Sprintf("No attempt is recorded for %s. A fresh passage is selected to measure the skill without help.", candidate.ID), 5

	case answers == 0 && ev.IndependentAttempts == 0:
		if candidate.Kind == KindScoring && ev.AssistedAttempts == 0 {
			return StateFamiliarityOnly, ActionExplain,
				fmt.Sprintf("%d attempt(s) are recorded for %s but none is independent transfer (assisted %d, revisits %d, passage re-exposures %d), and no guided tactic attempt exists. The explanation is selected before independent evidence.", ev.Attempts, candidate.ID, ev.AssistedAttempts, ev.Revisits, ev.PassageRepeats), 1
		}
		if fresh {
			return StateFamiliarityOnly, ActionPracticeNew,
				fmt.Sprintf("%d attempt(s) are recorded for %s but none is independent transfer (assisted %d, revisits %d, passage re-exposures %d). A fresh passage is selected; help and repeat exposure cannot show transfer.", ev.Attempts, candidate.ID, ev.AssistedAttempts, ev.Revisits, ev.PassageRepeats), 1
		}
		if review {
			return StateFamiliarityOnly, ActionReview,
				fmt.Sprintf("%d attempt(s) are recorded for %s but none is independent transfer (assisted %d, revisits %d, passage re-exposures %d). No fresh passage remains, so a seen question is selected for review; a repeat is not independent transfer.", ev.Attempts, candidate.ID, ev.AssistedAttempts, ev.Revisits, ev.PassageRepeats), 1
		}
		return StateFamiliarityOnly, ActionExplain,
			fmt.Sprintf("%d attempt(s) are recorded for %s but none is independent transfer, and no fresh or reviewable question remains. The explanation is selected.", ev.Attempts, candidate.ID), 1

	case answers <= 1:
		detail := fmt.Sprintf("%d correct, %d incorrect", ev.IndependentCorrect, ev.IndependentIncorrect)
		if ev.IndependentSkipped > 0 {
			detail += fmt.Sprintf(", %d skipped", ev.IndependentSkipped)
		}
		if fresh {
			return StateInsufficient, ActionPracticeNew,
				fmt.Sprintf("Only %d answered independent transfer observation(s) exist for %s (%s). A single observation cannot distinguish a skill pattern from luck; a fresh passage is selected.", answers, candidate.ID, detail), 2
		}
		if review {
			return StateInsufficient, ActionReview,
				fmt.Sprintf("Only %d answered independent transfer observation(s) exist for %s (%s). No fresh passage remains, so a seen question is selected for review; a repeat is not independent transfer.", answers, candidate.ID, detail), 2
		}
		return StateInsufficient, ActionExplain,
			fmt.Sprintf("Only %d answered independent transfer observation(s) exist for %s (%s), and no fresh or reviewable question remains. The explanation is selected.", answers, candidate.ID, detail), 2

	case ev.IndependentCorrect == 0:
		return StateWeak, ActionExplain,
			fmt.Sprintf("All %d answered independent transfer(s) for %s are incorrect. The explanation is selected because the gap repeated; the wrong options do not identify whether the cause is the skill, vocabulary, time or a misread stem.", answers, candidate.ID), 0

	case ev.IndependentCorrect < answers:
		if latestIncorrect {
			return StateMixed, ActionExplain,
				fmt.Sprintf("%d of %d answered independent transfer(s) for %s are correct, and the latest answered independent attempt was incorrect. The explanation is selected before another fresh question.", ev.IndependentCorrect, answers, candidate.ID), 0
		}
		if fresh {
			return StateMixed, ActionPracticeNew,
				fmt.Sprintf("%d of %d answered independent transfer(s) for %s are correct, and the latest answered independent attempt was correct. A fresh passage is selected to see whether the correction holds.", ev.IndependentCorrect, answers, candidate.ID), 3
		}
		if review {
			return StateMixed, ActionReview,
				fmt.Sprintf("%d of %d answered independent transfer(s) for %s are correct, and the latest answered independent attempt was correct. No fresh passage remains, so a seen question is selected for review.", ev.IndependentCorrect, answers, candidate.ID), 3
		}
		return StateMixed, ActionExplain,
			fmt.Sprintf("%d of %d answered independent transfer(s) for %s are correct, and no fresh or reviewable question remains. The explanation is selected.", ev.IndependentCorrect, answers, candidate.ID), 3

	default:
		slow := ev.IndependentCorrect >= 2 && ev.FastestIndependentMS > 0 &&
			ev.LatestIndependentStatus == attempt.StatusCorrect &&
			ev.LatestIndependentMS >= 2*ev.FastestIndependentMS
		if slow && review {
			return StateSupported, ActionReview,
				fmt.Sprintf("All %d answered independent transfer(s) for %s are correct, but the latest correct independent answer took %d ms against your fastest %d ms (at least twice as slow). A seen question is selected for review; a repeat is not independent transfer.", answers, candidate.ID, ev.LatestIndependentMS, ev.FastestIndependentMS), 4
		}
		if fresh {
			return StateSupported, ActionPracticeNew,
				fmt.Sprintf("All %d answered independent transfer(s) for %s are correct. A fresh passage is selected to test transfer; the sample is still small.", answers, candidate.ID), 6
		}
		if review {
			return StateSupported, ActionReview,
				fmt.Sprintf("All %d answered independent transfer(s) for %s are correct, but no fresh passage remains. A seen question is selected for review; a repeat is not independent transfer.", answers, candidate.ID), 4
		}
		return StateSupported, ActionExplain,
			fmt.Sprintf("All %d answered independent transfer(s) for %s are correct, and no fresh or reviewable question remains. The explanation is selected.", answers, candidate.ID), 4
	}
}

func causalDiagnosis(ev Evidence) string {
	switch {
	case ev.IndependentIncorrect > 0:
		return "unresolved"
	case ev.IndependentCorrect > 0:
		return "no_error_observed"
	default:
		return "not_attempted"
	}
}

func uncertainty(candidate Skill, ev Evidence, state State) []string {
	out := []string{fmt.Sprintf("This is an inference from %d recorded attempt(s), not a measured pass probability or readiness score.", ev.Attempts)}
	if ev.IndependentCorrect+ev.IndependentIncorrect <= 2 {
		out = append(out, "Two or fewer answered independent observations cannot establish a stable skill state.")
	}
	if ev.PassageRepeats > 0 {
		out = append(out, fmt.Sprintf("%d independent first attempt(s) on an already-seen passage are familiarity, not independent transfer.", ev.PassageRepeats))
	}
	if ev.Revisits > 0 {
		out = append(out, fmt.Sprintf("%d repeated attempt(s) are repeat exposure, not independent transfer.", ev.Revisits))
	}
	if ev.AssistedAttempts > 0 {
		out = append(out, fmt.Sprintf("%d assisted attempt(s) cannot count as independent correctness.", ev.AssistedAttempts))
	}
	if ev.IndependentIncorrect > 0 {
		out = append(out, "A wrong option does not identify whether the cause was the skill, vocabulary, time pressure or a misread stem.")
	}
	if ev.IndependentSkipped > 0 {
		out = append(out, fmt.Sprintf("%d independent transfer attempt(s) were skipped; a skip records no answer.", ev.IndependentSkipped))
	}
	if ev.MappedFromCurrentTags > 0 {
		out = append(out, fmt.Sprintf("%d attempt(s) predate the skill snapshot and were mapped from current question tags; a tag edit can regroup them.", ev.MappedFromCurrentTags))
	}
	if ev.IndependentCorrect >= 2 && ev.FastestIndependentMS > 0 &&
		ev.LatestIndependentStatus == attempt.StatusCorrect &&
		ev.LatestIndependentMS >= 2*ev.FastestIndependentMS {
		out = append(out, "Elapsed time is local prompt-to-answer time, not an exam time limit.")
	}
	if state == StateNoEvidence {
		out = append(out, "No attempt exists yet; the state is a starting recommendation, not a measurement.")
	}
	if candidate.Kind == KindScoring {
		out = append(out, "Scoring evidence is counted only for questions tagged with this skill; untagged attempts are listed separately.")
	}
	return out
}

func freshContentIDs(materials []Material) []string {
	var out []string
	seen := make(map[string]bool, len(materials))
	for _, material := range materials {
		if seen[material.ContentID] {
			continue
		}
		seen[material.ContentID] = true
		out = append(out, material.ContentID)
	}
	return out
}

func unique(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
