package adaptive

import (
	"slices"
	"strings"
	"testing"

	"github.com/wxxb789/passquest/internal/attempt"
)

var testSkills = []Skill{
	{ID: "not-except-polarity", Kind: KindScoring, Title: "NOT/EXCEPT polarity"},
	{ID: "vocabulary-in-context", Kind: KindLanguage, Title: "Vocabulary in context"},
	{ID: "sentence-structure", Kind: KindLanguage, Title: "Sentence structure"},
}

func testMaterials() []Material {
	return []Material{
		{SkillID: "not-except-polarity", ContentID: "score-a", QuestionID: "x1"},
		{SkillID: "not-except-polarity", ContentID: "score-b", QuestionID: "x2"},
		{SkillID: "vocabulary-in-context", ContentID: "vocab-a", QuestionID: "v1"},
		{SkillID: "vocabulary-in-context", ContentID: "vocab-a", QuestionID: "v1b"},
		{SkillID: "vocabulary-in-context", ContentID: "vocab-b", QuestionID: "v2"},
		{SkillID: "vocabulary-in-context", ContentID: "vocab-c", QuestionID: "v3"},
		{SkillID: "sentence-structure", ContentID: "sent-a", QuestionID: "s1"},
		{SkillID: "sentence-structure", ContentID: "sent-b", QuestionID: "s2"},
	}
}

func testInput() Input {
	return Input{Skills: testSkills, Materials: testMaterials()}
}

func vocabOnlyInput() Input {
	return Input{
		Skills: []Skill{{ID: "vocabulary-in-context", Kind: KindLanguage, Title: "Vocabulary in context"}},
		Materials: []Material{
			{SkillID: "vocabulary-in-context", ContentID: "vocab-a", QuestionID: "v1"},
			{SkillID: "vocabulary-in-context", ContentID: "vocab-b", QuestionID: "v2"},
			{SkillID: "vocabulary-in-context", ContentID: "vocab-c", QuestionID: "v3"},
		},
	}
}

func record(id int64, content, question string, number int, assistance attempt.Assistance, status attempt.Status, elapsed int64, skills ...string) Record {
	return Record{
		AttemptID:     id,
		ContentID:     content,
		QuestionID:    question,
		AttemptNumber: number,
		Assistance:    assistance,
		Status:        status,
		ElapsedMS:     elapsed,
		Skills:        skills,
	}
}

// guidedRecord is an assisted attempt delivered with the tactic walkthrough.
// Ordinary practice after a walkthrough is assisted too but names no tactic.
func guidedRecord(id int64, content, question string, number int, status attempt.Status, elapsed int64, skill string) Record {
	rec := record(id, content, question, number, attempt.AssistanceAssisted, status, elapsed, skill)
	rec.TacticID = skill
	return rec
}

func findSkill(t *testing.T, result Result, id string) SkillResult {
	t.Helper()
	for _, candidate := range result.Skills {
		if candidate.Skill.ID == id {
			return candidate
		}
	}
	t.Fatalf("skill %s is missing from the result", id)
	return SkillResult{}
}

func requireRecommendation(t *testing.T, result Result, skillID string, action Action) {
	t.Helper()
	rec := result.Recommendation
	if rec == nil {
		t.Fatal("no recommendation")
	}
	if rec.Skill.ID != skillID || rec.Action != action {
		t.Fatalf("recommendation = %s/%s, want %s/%s", rec.Skill.ID, rec.Action, skillID, action)
	}
	if strings.TrimSpace(rec.Reason) == "" {
		t.Fatal("recommendation reason is empty")
	}
}

func TestNoEvidence(t *testing.T) {
	result := Recommend(testInput())
	requireRecommendation(t, result, "not-except-polarity", ActionExplain)

	scoring := findSkill(t, result, "not-except-polarity")
	if scoring.Inference.State != StateNoEvidence || scoring.Evidence.FreshPassages != 2 {
		t.Fatalf("scoring = %+v", scoring)
	}
	if scoring.Inference.CausalDiagnosis != "not_attempted" {
		t.Fatalf("causal diagnosis = %q", scoring.Inference.CausalDiagnosis)
	}
	if len(scoring.Inference.Uncertainty) == 0 {
		t.Fatal("no uncertainty is stated")
	}

	vocab := findSkill(t, result, "vocabulary-in-context")
	if vocab.Inference.State != StateNoEvidence || vocab.Inference.Action != ActionPracticeNew {
		t.Fatalf("vocabulary = %+v", vocab)
	}
	if vocab.Evidence.FreshPassages != 3 {
		t.Fatalf("vocabulary fresh passages = %d, want 3", vocab.Evidence.FreshPassages)
	}
}

func TestAssistedOnlyIsNotIndependent(t *testing.T) {
	input := testInput()
	input.Records = []Record{
		guidedRecord(1, "score-a", "x1", 1, attempt.StatusCorrect, 1000, "not-except-polarity"),
	}
	result := Recommend(input)
	requireRecommendation(t, result, "not-except-polarity", ActionPracticeNew)
	scoring := findSkill(t, result, "not-except-polarity")
	if scoring.Evidence.AssistedAttempts != 1 || scoring.Evidence.AssistedCorrect != 1 {
		t.Fatalf("assisted evidence = %+v", scoring.Evidence)
	}
	if scoring.Evidence.IndependentAttempts != 0 || scoring.Evidence.IndependentCorrect != 0 {
		t.Fatalf("assisted attempt counted as independent: %+v", scoring.Evidence)
	}
	if scoring.Inference.State != StateFamiliarityOnly {
		t.Fatalf("state = %s", scoring.Inference.State)
	}
}

func TestAssistedPracticeWithoutGuidedTacticStillExplains(t *testing.T) {
	input := testInput()
	input.Records = []Record{
		record(1, "score-a", "x1", 1, attempt.AssistanceAssisted, attempt.StatusCorrect, 1000, "not-except-polarity"),
	}
	result := Recommend(input)
	requireRecommendation(t, result, "not-except-polarity", ActionExplain)
	scoring := findSkill(t, result, "not-except-polarity")
	if scoring.Evidence.AssistedAttempts != 1 || scoring.Evidence.GuidedAttempts != 0 {
		t.Fatalf("assisted evidence = %+v", scoring.Evidence)
	}
	if scoring.Inference.State != StateFamiliarityOnly {
		t.Fatalf("state = %s", scoring.Inference.State)
	}
	if !strings.Contains(scoring.Inference.Reason, "no guided tactic attempt exists") {
		t.Fatalf("reason = %s", scoring.Inference.Reason)
	}
}

func TestOneIndependentObservationIsInsufficient(t *testing.T) {
	for _, status := range []attempt.Status{attempt.StatusCorrect, attempt.StatusIncorrect} {
		input := testInput()
		input.Records = []Record{
			record(1, "vocab-a", "v1", 1, attempt.AssistanceIndependent, status, 1000, "vocabulary-in-context"),
		}
		result := Recommend(input)
		requireRecommendation(t, result, "vocabulary-in-context", ActionPracticeNew)
		vocab := findSkill(t, result, "vocabulary-in-context")
		if vocab.Inference.State != StateInsufficient {
			t.Fatalf("%s state = %s, want insufficient", status, vocab.Inference.State)
		}
		if !strings.Contains(vocab.Inference.Reason, "single observation") {
			t.Fatalf("reason does not explain the single observation: %s", vocab.Inference.Reason)
		}
		if status == attempt.StatusIncorrect && vocab.Inference.Action == ActionExplain {
			t.Fatal("a single wrong answer selected an explanation instead of a second observation")
		}
	}
}

func TestRepeatedIncorrectSelectsExplanation(t *testing.T) {
	input := testInput()
	input.Records = []Record{
		record(1, "vocab-a", "v1", 1, attempt.AssistanceIndependent, attempt.StatusIncorrect, 1000, "vocabulary-in-context"),
		record(2, "vocab-b", "v2", 1, attempt.AssistanceIndependent, attempt.StatusIncorrect, 1200, "vocabulary-in-context"),
	}
	result := Recommend(input)
	requireRecommendation(t, result, "vocabulary-in-context", ActionExplain)
	vocab := findSkill(t, result, "vocabulary-in-context")
	if vocab.Inference.State != StateWeak {
		t.Fatalf("state = %s", vocab.Inference.State)
	}
	if vocab.Inference.CausalDiagnosis != "unresolved" {
		t.Fatalf("causal diagnosis = %q", vocab.Inference.CausalDiagnosis)
	}
	if !strings.Contains(vocab.Inference.Reason, "do not identify") {
		t.Fatalf("reason overclaims a cause: %s", vocab.Inference.Reason)
	}
}

func TestMixedEvidenceUsesLatestIndependentAttempt(t *testing.T) {
	tests := []struct {
		name     string
		statuses []attempt.Status
		action   Action
	}{
		{"latest incorrect", []attempt.Status{attempt.StatusCorrect, attempt.StatusIncorrect}, ActionExplain},
		{"latest correct", []attempt.Status{attempt.StatusIncorrect, attempt.StatusCorrect}, ActionPracticeNew},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := testInput()
			input.Records = []Record{
				record(1, "vocab-a", "v1", 1, attempt.AssistanceIndependent, tt.statuses[0], 1000, "vocabulary-in-context"),
				record(2, "vocab-b", "v2", 1, attempt.AssistanceIndependent, tt.statuses[1], 1000, "vocabulary-in-context"),
			}
			result := Recommend(input)
			requireRecommendation(t, result, "vocabulary-in-context", tt.action)
			vocab := findSkill(t, result, "vocabulary-in-context")
			if vocab.Inference.State != StateMixed {
				t.Fatalf("state = %s", vocab.Inference.State)
			}
		})
	}
}

func TestSupportedEvidenceUsesTime(t *testing.T) {
	tests := []struct {
		name     string
		elapsed  []int64
		action   Action
		contains []string
	}{
		{"fast", []int64{1000, 1000}, ActionPracticeNew, []string{"fresh passage"}},
		{"slow latest", []int64{1000, 2500}, ActionReview, []string{"2500 ms", "1000 ms", "twice"}},
		{"slower first", []int64{2500, 1000}, ActionPracticeNew, []string{"fresh passage"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := vocabOnlyInput()
			input.Records = []Record{
				record(1, "vocab-a", "v1", 1, attempt.AssistanceIndependent, attempt.StatusCorrect, tt.elapsed[0], "vocabulary-in-context"),
				record(2, "vocab-b", "v2", 1, attempt.AssistanceIndependent, attempt.StatusCorrect, tt.elapsed[1], "vocabulary-in-context"),
			}
			result := Recommend(input)
			requireRecommendation(t, result, "vocabulary-in-context", tt.action)
			vocab := findSkill(t, result, "vocabulary-in-context")
			if vocab.Inference.State != StateSupported {
				t.Fatalf("state = %s", vocab.Inference.State)
			}
			for _, want := range tt.contains {
				if !strings.Contains(vocab.Inference.Reason, want) {
					t.Fatalf("reason lacks %q: %s", want, vocab.Inference.Reason)
				}
			}
			if tt.action == ActionPracticeNew && result.Recommendation.FreshMaterial == nil {
				t.Fatal("practice_new has no fresh material")
			}
			if tt.action == ActionReview && result.Recommendation.ReviewMaterial == nil {
				t.Fatal("review has no seen material")
			}
		})
	}
}

func TestPassageExposureIsNotIndependentTransfer(t *testing.T) {
	input := testInput()
	input.Records = []Record{
		record(1, "vocab-a", "v1", 1, attempt.AssistanceIndependent, attempt.StatusCorrect, 1000, "vocabulary-in-context"),
		record(2, "vocab-a", "v1b", 1, attempt.AssistanceIndependent, attempt.StatusCorrect, 1000, "vocabulary-in-context"),
	}
	result := Recommend(input)
	vocab := findSkill(t, result, "vocabulary-in-context")
	if vocab.Evidence.IndependentAttempts != 1 || vocab.Evidence.IndependentCorrect != 1 {
		t.Fatalf("independent evidence = %+v, want only the first passage attempt", vocab.Evidence)
	}
	if vocab.Evidence.PassageRepeats != 1 {
		t.Fatalf("passage repeats = %d, want 1", vocab.Evidence.PassageRepeats)
	}
	if vocab.Evidence.DistinctPassages != 1 {
		t.Fatalf("distinct passages = %d, want 1", vocab.Evidence.DistinctPassages)
	}
	if vocab.Inference.State == StateSupported {
		t.Fatal("two questions on one passage were treated as supported transfer")
	}
	requireRecommendation(t, result, "vocabulary-in-context", ActionPracticeNew)
}

func TestAssistedFirstAttemptExposesThePassage(t *testing.T) {
	input := testInput()
	input.Records = []Record{
		record(1, "vocab-a", "v1", 1, attempt.AssistanceAssisted, attempt.StatusCorrect, 1000, "vocabulary-in-context"),
		record(2, "vocab-a", "v1b", 1, attempt.AssistanceIndependent, attempt.StatusCorrect, 1000, "vocabulary-in-context"),
	}
	result := Recommend(input)
	vocab := findSkill(t, result, "vocabulary-in-context")
	if vocab.Evidence.IndependentAttempts != 0 || vocab.Evidence.PassageRepeats != 1 {
		t.Fatalf("evidence = %+v, want no independent transfer after assisted exposure", vocab.Evidence)
	}
	if vocab.Inference.State != StateFamiliarityOnly {
		t.Fatalf("state = %s", vocab.Inference.State)
	}
}

func TestRevisitsAreNotIndependentTransfer(t *testing.T) {
	input := testInput()
	input.Records = []Record{
		record(1, "vocab-a", "v1", 1, attempt.AssistanceIndependent, attempt.StatusCorrect, 1000, "vocabulary-in-context"),
		record(2, "vocab-a", "v1", 2, attempt.AssistanceIndependent, attempt.StatusCorrect, 1000, "vocabulary-in-context"),
	}
	result := Recommend(input)
	vocab := findSkill(t, result, "vocabulary-in-context")
	if vocab.Evidence.IndependentAttempts != 1 || vocab.Evidence.Revisits != 1 || vocab.Evidence.IndependentCorrect != 1 {
		t.Fatalf("evidence = %+v", vocab.Evidence)
	}
	if vocab.Inference.State != StateInsufficient {
		t.Fatalf("state = %s", vocab.Inference.State)
	}

	input.Records = append(input.Records,
		record(3, "vocab-b", "v2", 1, attempt.AssistanceIndependent, attempt.StatusCorrect, 1000, "vocabulary-in-context"))
	result = Recommend(input)
	vocab = findSkill(t, result, "vocabulary-in-context")
	if vocab.Inference.State != StateSupported || vocab.Evidence.IndependentAttempts != 2 || vocab.Evidence.Revisits != 1 {
		t.Fatalf("after fresh passage = %+v", vocab)
	}
}

func TestSkipsRecordNoAnswerEvidence(t *testing.T) {
	input := testInput()
	input.Records = []Record{
		record(1, "vocab-a", "v1", 1, attempt.AssistanceIndependent, attempt.StatusSkipped, 1000, "vocabulary-in-context"),
	}
	result := Recommend(input)
	vocab := findSkill(t, result, "vocabulary-in-context")
	if vocab.Evidence.IndependentAttempts != 1 || vocab.Evidence.IndependentSkipped != 1 {
		t.Fatalf("evidence = %+v", vocab.Evidence)
	}
	if vocab.Inference.State != StateInsufficient {
		t.Fatalf("state = %s", vocab.Inference.State)
	}
	if vocab.Inference.CausalDiagnosis != "not_attempted" {
		t.Fatalf("causal diagnosis = %q", vocab.Inference.CausalDiagnosis)
	}
	for _, line := range vocab.Inference.Uncertainty {
		if strings.Contains(line, "skip") {
			return
		}
	}
	t.Fatal("skip uncertainty is not stated")
}

func TestSkipAfterAnswersKeepsAnsweredEvidence(t *testing.T) {
	input := vocabOnlyInput()
	input.Records = []Record{
		record(1, "vocab-a", "v1", 1, attempt.AssistanceIndependent, attempt.StatusCorrect, 1000, "vocabulary-in-context"),
		record(2, "vocab-b", "v2", 1, attempt.AssistanceIndependent, attempt.StatusIncorrect, 1000, "vocabulary-in-context"),
		record(3, "vocab-c", "v3", 1, attempt.AssistanceIndependent, attempt.StatusSkipped, 1000, "vocabulary-in-context"),
	}
	result := Recommend(input)
	vocab := findSkill(t, result, "vocabulary-in-context")
	if vocab.Evidence.IndependentSkipped != 1 || vocab.Evidence.IndependentAttempts != 3 {
		t.Fatalf("skip evidence = %+v", vocab.Evidence)
	}
	if vocab.Inference.State != StateMixed || vocab.Inference.Action != ActionExplain {
		t.Fatalf("inference = %+v", vocab.Inference)
	}
	if !strings.Contains(vocab.Inference.Reason, "latest answered independent attempt was incorrect") {
		t.Fatalf("reason does not distinguish the skip from the answered attempt: %s", vocab.Inference.Reason)
	}
}

func TestNoFreshPassageSelectsReview(t *testing.T) {
	input := testInput()
	input.Records = []Record{
		record(1, "vocab-a", "v1", 1, attempt.AssistanceIndependent, attempt.StatusCorrect, 1000, "vocabulary-in-context"),
		record(2, "vocab-b", "v2", 1, attempt.AssistanceIndependent, attempt.StatusIncorrect, 1000, "vocabulary-in-context"),
	}
	// The latest is incorrect, so an explanation is selected even though fresh
	// material exists for another skill.
	result := Recommend(input)
	requireRecommendation(t, result, "vocabulary-in-context", ActionExplain)

	input = testInput()
	input.Records = []Record{
		record(1, "vocab-a", "v1", 1, attempt.AssistanceIndependent, attempt.StatusCorrect, 1000, "vocabulary-in-context"),
		record(2, "vocab-b", "v2", 1, attempt.AssistanceIndependent, attempt.StatusCorrect, 1000, "vocabulary-in-context"),
		record(3, "vocab-c", "v3", 1, attempt.AssistanceAssisted, attempt.StatusCorrect, 1000, "vocabulary-in-context"),
	}
	result = Recommend(input)
	requireRecommendation(t, result, "vocabulary-in-context", ActionReview)
	if result.Recommendation.ReviewMaterial == nil {
		t.Fatal("review has no seen material")
	}
}

func TestPriorityOrder(t *testing.T) {
	input := testInput()
	input.Records = []Record{
		guidedRecord(1, "score-a", "x1", 1, attempt.StatusCorrect, 1000, "not-except-polarity"),
		record(2, "vocab-a", "v1", 1, attempt.AssistanceIndependent, attempt.StatusIncorrect, 1000, "vocabulary-in-context"),
		record(3, "vocab-b", "v2", 1, attempt.AssistanceIndependent, attempt.StatusIncorrect, 1000, "vocabulary-in-context"),
	}
	result := Recommend(input)
	requireRecommendation(t, result, "vocabulary-in-context", ActionExplain)

	input = testInput()
	input.Records = []Record{
		guidedRecord(1, "score-a", "x1", 1, attempt.StatusCorrect, 1000, "not-except-polarity"),
		record(2, "vocab-a", "v1", 1, attempt.AssistanceIndependent, attempt.StatusCorrect, 1000, "vocabulary-in-context"),
	}
	result = Recommend(input)
	requireRecommendation(t, result, "not-except-polarity", ActionPracticeNew)
}

func TestTiesFollowRegistryOrder(t *testing.T) {
	input := Input{
		Skills: []Skill{
			{ID: "vocabulary-in-context", Kind: KindLanguage, Title: "Vocabulary in context"},
			{ID: "sentence-structure", Kind: KindLanguage, Title: "Sentence structure"},
		},
		Materials: []Material{
			{SkillID: "vocabulary-in-context", ContentID: "vocab-a", QuestionID: "v1"},
			{SkillID: "sentence-structure", ContentID: "sent-a", QuestionID: "s1"},
		},
	}
	result := Recommend(input)
	requireRecommendation(t, result, "vocabulary-in-context", ActionPracticeNew)

	input.Skills[0], input.Skills[1] = input.Skills[1], input.Skills[0]
	result = Recommend(input)
	requireRecommendation(t, result, "sentence-structure", ActionPracticeNew)
}

func TestCurrentTagFallbackIsDisclosed(t *testing.T) {
	input := testInput()
	rec := record(1, "vocab-a", "v1", 1, attempt.AssistanceIndependent, attempt.StatusCorrect, 1000, "vocabulary-in-context")
	rec.FromCurrentTags = true
	input.Records = []Record{rec}
	result := Recommend(input)
	vocab := findSkill(t, result, "vocabulary-in-context")
	if vocab.Evidence.MappedFromCurrentTags != 1 {
		t.Fatalf("mapped count = %d", vocab.Evidence.MappedFromCurrentTags)
	}
	if !hasSubstring(vocab.Inference.Uncertainty, "snapshot") {
		t.Fatalf("uncertainty does not disclose the current-tag fallback: %v", vocab.Inference.Uncertainty)
	}
}

func TestMultiSkillRecordCountsForEachAndExposesContent(t *testing.T) {
	input := Input{
		Skills: []Skill{
			{ID: "vocabulary-in-context", Kind: KindLanguage, Title: "Vocabulary in context"},
			{ID: "sentence-structure", Kind: KindLanguage, Title: "Sentence structure"},
		},
		Materials: []Material{
			{SkillID: "vocabulary-in-context", ContentID: "shared", QuestionID: "q1"},
			{SkillID: "sentence-structure", ContentID: "shared", QuestionID: "q1"},
			{SkillID: "sentence-structure", ContentID: "shared", QuestionID: "q2"},
		},
		Records: []Record{
			record(1, "shared", "q1", 1, attempt.AssistanceIndependent, attempt.StatusCorrect, 1000, "vocabulary-in-context", "sentence-structure"),
		},
	}
	result := Recommend(input)
	vocab := findSkill(t, result, "vocabulary-in-context")
	sentence := findSkill(t, result, "sentence-structure")
	if vocab.Evidence.IndependentAttempts != 1 || sentence.Evidence.IndependentAttempts != 1 {
		t.Fatalf("multi-skill evidence = %+v / %+v", vocab.Evidence, sentence.Evidence)
	}
	if sentence.Evidence.FreshPassages != 0 {
		t.Fatalf("sentence fresh passages = %d, want 0 after a shared passage was attempted", sentence.Evidence.FreshPassages)
	}
}

func TestNoMaterialSkill(t *testing.T) {
	result := Recommend(Input{
		Skills: []Skill{{ID: "vocabulary-in-context", Kind: KindLanguage, Title: "Vocabulary in context"}},
	})
	vocab := findSkill(t, result, "vocabulary-in-context")
	if vocab.Inference.State != StateNoMaterial || vocab.Inference.Action != "" {
		t.Fatalf("no-material skill = %+v", vocab)
	}
	if result.Recommendation != nil {
		t.Fatalf("recommendation = %+v, want none", result.Recommendation)
	}
}

func hasSubstring(values []string, want string) bool {
	return slices.ContainsFunc(values, func(value string) bool {
		return strings.Contains(value, want)
	})
}
