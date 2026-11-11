package find

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/search"
)

func record(id int64, status attempt.Status, section string, skills ...string) Record {
	return Record{
		AttemptID:     id,
		AttemptNumber: 1,
		Assistance:    attempt.AssistanceIndependent,
		Status:        status,
		Skills:        skills,
		ExamSection:   section,
	}
}

func wrong(id int64, skills ...string) Record {
	return record(id, attempt.StatusIncorrect, "mock-exam", skills...)
}
func correct(id int64, skills ...string) Record {
	return record(id, attempt.StatusCorrect, "mock-exam", skills...)
}
func skipped(id int64, skills ...string) Record {
	return record(id, attempt.StatusSkipped, "mock-exam", skills...)
}

func TestDeriveSelectsHighestErrorRatioAmongEligibleSkills(t *testing.T) {
	records := []Record{
		wrong(1, "vocabulary-in-context"), wrong(2, "vocabulary-in-context"), wrong(3, "vocabulary-in-context"),
		wrong(4, "sentence-structure"), correct(5, "sentence-structure"),
		wrong(6, "sentence-structure"), correct(7, "sentence-structure"),
	}
	intent, ok := Derive(records, 20)
	if !ok {
		t.Fatal("Derive returned no intent")
	}
	if intent.SkillID != "vocabulary-in-context" {
		t.Fatalf("selected %q, want vocabulary-in-context", intent.SkillID)
	}
	if intent.Selection != SelectionSkillErrorRatio {
		t.Fatalf("selection = %q", intent.Selection)
	}
	if intent.Evidence.Attempts != 3 || intent.Evidence.Incorrect != 3 || intent.Evidence.Skipped != 0 {
		t.Fatalf("evidence = %+v", intent.Evidence)
	}
	if len(intent.Evidence.AttemptIDs) != 3 {
		t.Fatalf("attempt ids = %v", intent.Evidence.AttemptIDs)
	}
	if intent.ExamSection != "mock-exam" {
		t.Fatalf("section = %q", intent.ExamSection)
	}
}

func TestDeriveTieBreaksByLargerAttemptCount(t *testing.T) {
	records := []Record{
		wrong(1, "not-except-polarity"), wrong(2, "not-except-polarity"), correct(3, "not-except-polarity"),
		wrong(4, "vocabulary-in-context"), wrong(5, "vocabulary-in-context"), correct(6, "vocabulary-in-context"),
		wrong(7, "vocabulary-in-context"), wrong(8, "vocabulary-in-context"), correct(9, "vocabulary-in-context"),
	}
	intent, ok := Derive(records, 20)
	if !ok {
		t.Fatal("Derive returned no intent")
	}
	if intent.SkillID != "vocabulary-in-context" {
		t.Fatalf("selected %q, want the larger attempt count", intent.SkillID)
	}
	if intent.Evidence.Attempts != 6 || intent.Evidence.Incorrect != 4 {
		t.Fatalf("evidence = %+v", intent.Evidence)
	}
}

func TestDeriveTieAtEqualRatioAndCountUsesRegistryOrder(t *testing.T) {
	records := []Record{
		wrong(1, "vocabulary-in-context"), correct(2, "vocabulary-in-context"), correct(3, "vocabulary-in-context"),
		wrong(4, "not-except-polarity"), correct(5, "not-except-polarity"), correct(6, "not-except-polarity"),
	}
	intent, ok := Derive(records, 20)
	if !ok {
		t.Fatal("Derive returned no intent")
	}
	if intent.SkillID != "not-except-polarity" {
		t.Fatalf("selected %q, want the registry-first scoring skill", intent.SkillID)
	}
}

func TestDeriveSelectsAnEligibleSkillWithNoObservedError(t *testing.T) {
	intent, ok := Derive([]Record{
		correct(1, "vocabulary-in-context"), correct(2, "vocabulary-in-context"), correct(3, "vocabulary-in-context"),
	}, 20)
	if !ok {
		t.Fatal("Derive returned no intent")
	}
	if intent.SkillID != "vocabulary-in-context" {
		t.Fatalf("selected %q", intent.SkillID)
	}
	if !strings.Contains(intent.Reason, "0 of 3") {
		t.Fatalf("reason does not disclose the zero error count: %s", intent.Reason)
	}
}

func TestDeriveFallsBackToTheLatestWrongAnswersTactic(t *testing.T) {
	record := wrong(7, "not-except-polarity")
	record.TacticID = "not-except-polarity"
	intent, ok := Derive([]Record{record, correct(8, "vocabulary-in-context")}, 20)
	if !ok {
		t.Fatal("Derive returned no intent")
	}
	if intent.Selection != SelectionLatestWrongTactic {
		t.Fatalf("selection = %q", intent.Selection)
	}
	if intent.SkillID != "not-except-polarity" || intent.TacticID != "not-except-polarity" {
		t.Fatalf("intent = %+v", intent)
	}
	if len(intent.Evidence.AttemptIDs) != 1 || intent.Evidence.AttemptIDs[0] != 7 {
		t.Fatalf("evidence ids = %v", intent.Evidence.AttemptIDs)
	}
	if !strings.Contains(intent.Reason, "most recent wrong answer") {
		t.Fatalf("reason = %q", intent.Reason)
	}
}

func TestDeriveUsesAScoringSnapshotWhenTacticIDIsEmpty(t *testing.T) {
	intent, ok := Derive([]Record{
		wrong(1, "sentence-structure", "not-except-polarity"),
	}, 20)
	if !ok {
		t.Fatal("Derive returned no intent")
	}
	if intent.TacticID != "not-except-polarity" {
		t.Fatalf("tactic = %q", intent.TacticID)
	}
}

func TestDeriveReportsNothingWithoutEligibleEvidence(t *testing.T) {
	cases := map[string][]Record{
		"empty": nil,
		"only correctness": {
			correct(1, "vocabulary-in-context"),
		},
		"wrong without a scoring tactic": {
			record(1, attempt.StatusIncorrect, "notice", "sentence-structure"),
		},
		"wrong with no skills": {
			record(1, attempt.StatusIncorrect, "notice"),
		},
	}
	for name, records := range cases {
		t.Run(name, func(t *testing.T) {
			if intent, ok := Derive(records, 20); ok {
				t.Fatalf("Derive returned %+v", intent)
			}
		})
	}
}

func TestInferLevelUsesFirstIndependentAttempts(t *testing.T) {
	cases := []struct {
		name    string
		records []Record
		want    Level
		correct int
		skipped int
	}{
		{"empty", nil, LevelEarly, 0, 0},
		{"two observations", []Record{correct(1), wrong(2)}, LevelEarly, 1, 0},
		{"foundation", []Record{wrong(1), wrong(2), correct(3)}, LevelFoundation, 1, 0},
		{"developing at 2/5", []Record{wrong(1), wrong(2), wrong(3), correct(4), correct(5)}, LevelDeveloping, 2, 0},
		{"developing", []Record{correct(1), correct(2), wrong(3)}, LevelDeveloping, 2, 0},
		{"consolidating at 7/10", []Record{
			wrong(1), wrong(2), wrong(3), correct(4), correct(5),
			correct(6), correct(7), correct(8), correct(9), correct(10),
		}, LevelConsolidating, 7, 0},
		{"consolidating", []Record{correct(1), correct(2), correct(3)}, LevelConsolidating, 3, 0},
		{"skip counts in denominator", []Record{correct(1), correct(2), skipped(3)}, LevelDeveloping, 2, 1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			level, basis := InferLevel(testCase.records)
			if level != testCase.want {
				t.Fatalf("level = %q, want %q (basis %+v)", level, testCase.want, basis)
			}
			if basis.Correct != testCase.correct || basis.Skipped != testCase.skipped {
				t.Fatalf("basis = %+v", basis)
			}
		})
	}
}

func TestInferLevelIgnoresRepeatsAndAssistance(t *testing.T) {
	repeat := correct(1)
	repeat.AttemptNumber = 2
	assisted := wrong(2)
	assisted.Assistance = attempt.AssistanceAssisted
	level, basis := InferLevel([]Record{repeat, assisted})
	if level != LevelEarly || basis.IndependentFirstAttempts != 0 {
		t.Fatalf("level/basis = %q/%+v", level, basis)
	}
}

func TestBuildQueryCarriesSkillSectionAndLevelButNoPrivateIdentifier(t *testing.T) {
	query := BuildQuery("Vocabulary in context", "mock-exam", LevelDeveloping)
	if query != "Vocabulary in context English exam practice mock-exam for developing level learners" {
		t.Fatalf("query = %q", query)
	}
	if strings.Contains(query, "src-") || strings.Contains(query, "p001") || strings.Contains(query, "q1") {
		t.Fatalf("query carries a private identifier: %q", query)
	}
	if got := BuildQuery("Vocabulary in context", "embedded", LevelEarly); strings.Contains(got, "embedded") {
		t.Fatalf("embedded section leaked into the query: %q", got)
	}
}

func TestClassifyPrecedence(t *testing.T) {
	items := []search.Item{
		{Title: "Generated", URL: "https://example.com/g", Content: "generated text", License: "cc-by-4.0", Generated: true},
		{Title: "Usable", URL: "https://example.com/u", Content: "open text", License: "CC-BY-4.0"},
		{Title: "No license", URL: "https://example.com/n", Content: "text"},
		{Title: "Closed license", URL: "https://example.com/c", Content: "text", License: "all-rights-reserved"},
		{Title: "License without text", URL: "https://example.com/l", License: "cc-by-4.0"},
	}
	candidates := Classify(items)
	if candidates[0].Classification != ClassGeneratedUnverified || candidates[0].Content != "" || candidates[0].Displayable {
		t.Fatalf("generated candidate = %+v", candidates[0])
	}
	if candidates[1].Classification != ClassLocalUsable || candidates[1].Content != "open text" || !candidates[1].Displayable {
		t.Fatalf("local candidate = %+v", candidates[1])
	}
	if candidates[2].Classification != ClassLinkOnly || candidates[2].Reason != "no_open_license" {
		t.Fatalf("no-license candidate = %+v", candidates[2])
	}
	if candidates[3].Classification != ClassLinkOnly || candidates[3].Reason != "license_not_open" {
		t.Fatalf("closed-license candidate = %+v", candidates[3])
	}
	if candidates[4].Classification != ClassLinkOnly || candidates[4].Reason != "no_returned_text" {
		t.Fatalf("license-without-text candidate = %+v", candidates[4])
	}
	if candidates[0].License != "cc-by-4.0" {
		t.Fatalf("generated license was not retained: %+v", candidates[0])
	}
	counts := Count(candidates)
	if counts.Total != 5 || counts.LocalUsable != 1 || counts.LinkOnly != 3 || counts.GeneratedUnverified != 1 {
		t.Fatalf("counts = %+v", counts)
	}
}

func TestReturnedContentSHA256UsesContentThenSnippet(t *testing.T) {
	content := ReturnedContentSHA256(search.Item{Content: "text", Snippet: "snippet"})
	snippet := ReturnedContentSHA256(search.Item{Snippet: "snippet"})
	empty := ReturnedContentSHA256(search.Item{})
	if len(content) != 64 || len(snippet) != 64 || len(empty) != 64 {
		t.Fatalf("hash lengths = %d/%d/%d", len(content), len(snippet), len(empty))
	}
	if content == snippet || snippet == empty {
		t.Fatalf("hashes are not distinct: %s %s %s", content, snippet, empty)
	}
	sum := sha256.Sum256([]byte("snippet"))
	if snippet != hex.EncodeToString(sum[:]) {
		t.Fatalf("snippet hash = %s", snippet)
	}
}

func TestSanitizeTextKeepsNewlinesAndReplacesControlsAndBidi(t *testing.T) {
	input := "a\x00b\nc\td\x1be\u202ef"
	got := SanitizeText(input)
	if strings.ContainsAny(got, "\x00\x1b") || strings.ContainsRune(got, '\u202e') {
		t.Fatalf("sanitize left a control: %q", got)
	}
	if !strings.Contains(got, "\n") || !strings.Contains(got, "\t") {
		t.Fatalf("sanitize removed whitespace: %q", got)
	}
}

func TestTruncateRunesMarksTheCut(t *testing.T) {
	if got := TruncateRunes("abcdef", 3); got != "abc…" {
		t.Fatalf("truncate = %q", got)
	}
	if got := TruncateRunes("abc", 3); got != "abc" {
		t.Fatalf("short text changed: %q", got)
	}
}
