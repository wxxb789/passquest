package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
)

func decodeNext(t *testing.T, raw string) nextDoc {
	t.Helper()
	var doc nextDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode next: %v\n%s", err, raw)
	}
	return doc
}

func nextSkill(t *testing.T, doc nextDoc, id string) nextSkillView {
	t.Helper()
	for _, candidate := range doc.Skills {
		if candidate.Skill.ID == id {
			return candidate
		}
	}
	t.Fatalf("skill %s is missing from next output", id)
	return nextSkillView{}
}

// slowVocabClock practices the two vocabulary questions 1 s and 2.5 s apart, so
// the latest correct answer is slow against the 1 s fastest answer. It returns
// the clock the two commands consumed.
func slowVocabClock(t *testing.T, db string) *sequenceClock {
	t.Helper()
	base := goldenBase
	clock := &sequenceClock{times: []time.Time{
		base,
		base.Add(time.Second),
		base.Add(10 * time.Second),
		base.Add(12*time.Second + 500*time.Millisecond),
	}}
	mustRunWithNow(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--answer", "A", "--json")
	mustRunWithNow(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--answer", "B", "--json")
	return clock
}

func TestNextSendsUnGuidedScoringToTacticWalkthrough(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	imported := importFixture(t, db, clock)
	sourceID := imported.Source.ID
	reviewedTaggedFixtureQuestion(t, db, clock, sourceID)

	// Ordinary practice can supply independent scoring evidence without the
	// tactic walkthrough. next must still offer a command the CLI accepts.
	mustRun(t, db, clock, "", "practice", "--source", sourceID, "--question", "q2", "--answer", "C", "--json")
	doc := decodeNext(t, mustRun(t, db, clock, "", "next", "--source", sourceID, "--json"))
	scoring := nextSkill(t, doc, "not-except-polarity")
	if scoring.Evidence.IndependentAttempts != 1 || scoring.Evidence.GuidedAttempts != 0 {
		t.Fatalf("ordinary scoring evidence = %+v", scoring.Evidence)
	}
	if scoring.Inference.State != "insufficient" || scoring.Inference.Action != "explain" {
		t.Fatalf("scoring inference = %+v", scoring.Inference)
	}
	wantCommand := "passquest tactic practice not-except-polarity --stage guided --db " + testQuoter.quote(db)
	if doc.Recommendation == nil || doc.Recommendation.Command != wantCommand {
		t.Fatalf("recommendation = %+v, want command %q", doc.Recommendation, wantCommand)
	}

	mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--answer", "D", "--json")
	doc = decodeNext(t, mustRun(t, db, clock, "", "next", "--source", sourceID, "--json"))
	scoring = nextSkill(t, doc, "not-except-polarity")
	if scoring.Evidence.GuidedAttempts != 1 {
		t.Fatalf("guided attempts = %+v", scoring.Evidence)
	}
	if doc.Recommendation == nil || doc.Recommendation.Action != "practice_new" {
		t.Fatalf("recommendation after guided = %+v", doc.Recommendation)
	}
	awaiting := decodeAwaiting(t, mustRun(t, db, clock, "", "skill", "practice", "not-except-polarity", "--json"))
	if awaiting.Question.ID == "t1" {
		t.Fatalf("skill practice selected the guided example %s", awaiting.Question.ID)
	}
}

func TestNextIncludesImportedSourceWithoutScope(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	imported := importFixture(t, db, clock)
	sourceID := imported.Source.ID
	reviewedTaggedFixtureQuestion(t, db, clock, sourceID)
	mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--answer", "D", "--json")
	mustRun(t, db, clock, "", "skill", "practice", "not-except-polarity", "--source", sourceID, "--answer", "C", "--json")

	doc := decodeNext(t, mustRun(t, db, clock, "", "next", "--json"))
	if doc.SourceScope != "" {
		t.Fatalf("source scope = %q, want the unscoped catalog", doc.SourceScope)
	}
	scoring := nextSkill(t, doc, "not-except-polarity")
	if scoring.Evidence.Attempts != 2 || scoring.Evidence.AssistedAttempts != 1 || scoring.Evidence.IndependentAttempts != 1 || scoring.Evidence.IndependentCorrect != 1 {
		t.Fatalf("imported source attempt is not included by default: %+v", scoring.Evidence)
	}
	if len(doc.UnclassifiedAttempts) != 0 {
		t.Fatalf("imported source attempt is unclassified: %+v", doc.UnclassifiedAttempts)
	}
}

func TestNextNoEvidence(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	doc := decodeNext(t, mustRun(t, db, clock, "", "next", "--json"))
	if doc.State != "recommendation" || doc.Recommendation == nil {
		t.Fatalf("next = %+v", doc)
	}
	if doc.Recommendation.Action != "explain" || doc.Recommendation.Skill.ID != "not-except-polarity" {
		t.Fatalf("recommendation = %+v", doc.Recommendation)
	}
	if !strings.Contains(doc.Recommendation.Command, "tactic practice") {
		t.Fatalf("scoring explanation command = %q", doc.Recommendation.Command)
	}
	if len(doc.Skills) != 3 {
		t.Fatalf("skills = %d", len(doc.Skills))
	}
	for _, id := range []string{"vocabulary-in-context", "sentence-structure"} {
		candidate := nextSkill(t, doc, id)
		if candidate.Inference.State != "no_evidence" || candidate.Inference.Action != "practice_new" {
			t.Fatalf("%s inference = %+v", id, candidate.Inference)
		}
		wantFresh := 2
		if id == "vocabulary-in-context" {
			wantFresh = 3
		}
		if candidate.Evidence.FreshPassages != wantFresh {
			t.Fatalf("%s fresh passages = %d, want %d", id, candidate.Evidence.FreshPassages, wantFresh)
		}
	}
	if len(doc.UnclassifiedAttempts) != 0 {
		t.Fatalf("unclassified = %+v", doc.UnclassifiedAttempts)
	}
	if len(doc.Limitations) == 0 {
		t.Fatal("no limitations")
	}

	human := mustRun(t, db, clock, "", "next")
	for _, want := range []string{"Recommendation:", "Why:", "Command:", "Evidence (recorded attempts)", "Inference (recomputed from evidence)", "Limitations"} {
		if !strings.Contains(human, want) {
			t.Errorf("human next lacks %q:\n%s", want, human)
		}
	}
}

func TestNextPassageExposureAndHints(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--answer", "D", "--json")
	mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--answer", "C", "--json")

	doc := decodeNext(t, mustRun(t, db, clock, "", "next", "--json"))
	scoring := nextSkill(t, doc, "not-except-polarity")
	if scoring.Evidence.Attempts != 2 || scoring.Evidence.AssistedAttempts != 1 || scoring.Evidence.IndependentAttempts != 0 {
		t.Fatalf("scoring evidence = %+v", scoring.Evidence)
	}
	if scoring.Evidence.PassageRepeats != 1 || scoring.Evidence.DistinctPassages != 1 {
		t.Fatalf("passage exposure = %+v", scoring.Evidence)
	}
	if scoring.Inference.State != "familiarity_only" || scoring.Inference.Action != "practice_new" {
		t.Fatalf("scoring inference = %+v", scoring.Inference)
	}
	wantCommand := "passquest skill practice not-except-polarity --db " + testQuoter.quote(db)
	if doc.Recommendation == nil || doc.Recommendation.Skill.ID != "not-except-polarity" || doc.Recommendation.Command != wantCommand {
		t.Fatalf("recommendation = %+v, want command %q", doc.Recommendation, wantCommand)
	}
}

func TestNextTimeChangesAction(t *testing.T) {
	t.Parallel()
	base := goldenBase
	fast := &sequenceClock{times: []time.Time{base, base.Add(time.Second), base.Add(2 * time.Second), base.Add(3 * time.Second)}}
	fastDB := newDB(t)
	mustRunWithNow(t, fastDB, fast, "", "skill", "practice", "vocabulary-in-context", "--answer", "A", "--json")
	mustRunWithNow(t, fastDB, fast, "", "skill", "practice", "vocabulary-in-context", "--answer", "B", "--json")
	fastDoc := decodeNext(t, mustRunWithNow(t, fastDB, fast, "", "next", "--json"))
	fastVocab := nextSkill(t, fastDoc, "vocabulary-in-context")
	if fastVocab.Inference.State != "supported" || fastVocab.Inference.Action != "practice_new" {
		t.Fatalf("fast inference = %+v", fastVocab.Inference)
	}
	if fastVocab.Evidence.FastestIndependentMS != 1000 || fastVocab.Evidence.LatestIndependentMS != 1000 {
		t.Fatalf("fast elapsed = %+v", fastVocab.Evidence)
	}

	slowDB := newDB(t)
	slow := slowVocabClock(t, slowDB)
	slowDoc := decodeNext(t, mustRunWithNow(t, slowDB, slow, "", "next", "--json"))
	slowVocab := nextSkill(t, slowDoc, "vocabulary-in-context")
	if slowVocab.Inference.State != "supported" || slowVocab.Inference.Action != "review" {
		t.Fatalf("slow inference = %+v", slowVocab.Inference)
	}
	if !strings.Contains(slowVocab.Inference.Reason, "2500 ms") || !strings.Contains(slowVocab.Inference.Reason, "1000 ms") {
		t.Fatalf("slow reason does not quote elapsed values: %s", slowVocab.Inference.Reason)
	}
	// The slow signal changes the vocabulary skill's action to review, but a
	// review cannot add evidence. The untouched scoring skill can, so the top
	// recommendation yields to it rather than staying on vocabulary forever.
	if slowDoc.Recommendation == nil || slowDoc.Recommendation.Skill.ID != "not-except-polarity" || slowDoc.Recommendation.Action != "explain" {
		t.Fatalf("slow recommendation = %+v, want the unpracticed scoring explanation", slowDoc.Recommendation)
	}
}

func TestNextExhaustedScoringYieldsToLanguage(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--answer", "D", "--json")
	mustRun(t, db, clock, "", "skill", "practice", "not-except-polarity", "--answer", "C", "--json")

	doc := decodeNext(t, mustRun(t, db, clock, "", "next", "--json"))
	scoring := nextSkill(t, doc, "not-except-polarity")
	if scoring.Evidence.IndependentAttempts != 1 || scoring.Evidence.FreshPassages != 0 {
		t.Fatalf("exhausted scoring evidence = %+v", scoring.Evidence)
	}
	if scoring.Inference.State != "insufficient" || scoring.Inference.Action != "review" {
		t.Fatalf("exhausted scoring inference = %+v", scoring.Inference)
	}
	if doc.Recommendation == nil || doc.Recommendation.Skill.ID != "vocabulary-in-context" || doc.Recommendation.Action != "practice_new" {
		t.Fatalf("recommendation = %+v, want fresh vocabulary practice", doc.Recommendation)
	}
	if !strings.Contains(doc.Recommendation.Command, "--db "+testQuoter.quote(db)) {
		t.Fatalf("vocabulary command does not preserve the database: %q", doc.Recommendation.Command)
	}
}

func TestNextExhaustedWeakYieldsToFreshMaterial(t *testing.T) {
	t.Parallel()
	seedScoring := func(t *testing.T, db string, clock *stepClock) {
		t.Helper()
		mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--answer", "D", "--json")
		mustRun(t, db, clock, "", "skill", "practice", "not-except-polarity", "--answer", "C", "--json")
	}

	t.Run("weak", func(t *testing.T) {
		db := newDB(t)
		clock := &stepClock{base: goldenBase}
		seedScoring(t, db, clock)
		mustRun(t, db, clock, "", "skill", "practice", "sentence-structure", "--answer", "A", "--json")
		mustRun(t, db, clock, "", "skill", "practice", "sentence-structure", "--answer", "A", "--json")

		doc := decodeNext(t, mustRun(t, db, clock, "", "next", "--json"))
		sentence := nextSkill(t, doc, "sentence-structure")
		if sentence.Evidence.IndependentAttempts != 2 || sentence.Evidence.IndependentIncorrect != 2 || sentence.Evidence.FreshPassages != 0 {
			t.Fatalf("exhausted weak evidence = %+v", sentence.Evidence)
		}
		if sentence.Inference.State != "weak" || sentence.Inference.Action != "explain" {
			t.Fatalf("exhausted weak inference = %+v", sentence.Inference)
		}
		// The lesson repeats a seen question and cannot add evidence; fresh
		// vocabulary material can, so it is recommended first.
		if doc.Recommendation == nil || doc.Recommendation.Skill.ID != "vocabulary-in-context" || doc.Recommendation.Action != "practice_new" {
			t.Fatalf("recommendation = %+v, want fresh vocabulary practice", doc.Recommendation)
		}
		if !strings.Contains(doc.Recommendation.Command, "--db "+testQuoter.quote(db)) {
			t.Fatalf("vocabulary command does not preserve the database: %q", doc.Recommendation.Command)
		}

		// With every skill exhausted nothing can add evidence; the weak
		// explanation is selected again instead of being lost.
		for _, answer := range []string{"A", "B", "A"} {
			mustRun(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--answer", answer, "--json")
		}
		doc = decodeNext(t, mustRun(t, db, clock, "", "next", "--json"))
		if doc.Recommendation == nil || doc.Recommendation.Skill.ID != "sentence-structure" || doc.Recommendation.Action != "explain" {
			t.Fatalf("exhausted recommendation = %+v, want the weak explanation", doc.Recommendation)
		}
		if doc.Recommendation.Command != "passquest skill show sentence-structure" {
			t.Fatalf("exhausted explanation command = %q", doc.Recommendation.Command)
		}
		if !strings.Contains(doc.Recommendation.FollowUpCommand, "--question s2") ||
			!strings.Contains(doc.Recommendation.FollowUpCommand, "--db "+testQuoter.quote(db)) {
			t.Fatalf("exhausted explanation follow-up = %q", doc.Recommendation.FollowUpCommand)
		}
	})

	t.Run("mixed latest incorrect", func(t *testing.T) {
		db := newDB(t)
		clock := &stepClock{base: goldenBase}
		seedScoring(t, db, clock)
		mustRun(t, db, clock, "", "skill", "practice", "sentence-structure", "--answer", "C", "--json")
		mustRun(t, db, clock, "", "skill", "practice", "sentence-structure", "--answer", "A", "--json")

		doc := decodeNext(t, mustRun(t, db, clock, "", "next", "--json"))
		sentence := nextSkill(t, doc, "sentence-structure")
		if sentence.Inference.State != "mixed" || sentence.Inference.Action != "explain" || sentence.Evidence.FreshPassages != 0 {
			t.Fatalf("exhausted mixed inference = %+v, fresh = %d", sentence.Inference, sentence.Evidence.FreshPassages)
		}
		if doc.Recommendation == nil || doc.Recommendation.Skill.ID != "vocabulary-in-context" || doc.Recommendation.Action != "practice_new" {
			t.Fatalf("recommendation = %+v, want fresh vocabulary practice", doc.Recommendation)
		}
	})
}

func TestNextReviewCommandWhenEverySkillIsExhausted(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--answer", "D", "--json")
	mustRun(t, db, clock, "", "skill", "practice", "not-except-polarity", "--answer", "C", "--json")
	for _, answer := range []struct {
		skill  string
		option string
	}{
		{"vocabulary-in-context", "A"},
		{"vocabulary-in-context", "B"},
		{"vocabulary-in-context", "A"},
		{"sentence-structure", "C"},
		{"sentence-structure", "D"},
	} {
		mustRun(t, db, clock, "", "skill", "practice", answer.skill, "--answer", answer.option, "--json")
	}

	doc := decodeNext(t, mustRun(t, db, clock, "", "next", "--json"))
	if doc.Recommendation == nil || doc.Recommendation.Skill.ID != "not-except-polarity" || doc.Recommendation.Action != "review" {
		t.Fatalf("recommendation = %+v, want the scoring review", doc.Recommendation)
	}
	if !strings.Contains(doc.Recommendation.Command, "--question x1") {
		t.Fatalf("review command does not name a seen question: %q", doc.Recommendation.Command)
	}
	if !strings.Contains(doc.Recommendation.Command, "--db "+testQuoter.quote(db)) {
		t.Fatalf("review command does not preserve the database: %q", doc.Recommendation.Command)
	}
}

func TestNextCommandPreservesSelectedDatabase(t *testing.T) {
	t.Parallel()
	db := filepath.Join(t.TempDir(), "learner data", "attempts.sqlite")
	if err := os.MkdirAll(filepath.Dir(db), 0o700); err != nil {
		t.Fatal(err)
	}
	clock := &stepClock{base: goldenBase}
	doc := decodeNext(t, mustRun(t, db, clock, "", "next", "--json"))
	if doc.Recommendation == nil {
		t.Fatal("no recommendation")
	}
	want := "--db " + testQuoter.quote(db)
	for name, command := range map[string]string{
		"command":   doc.Recommendation.Command,
		"follow-up": doc.Recommendation.FollowUpCommand,
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("%s does not preserve the selected database: %q", name, command)
		}
	}
}

func TestNextCommandOmitsDefaultDatabase(t *testing.T) {
	t.Chdir(t.TempDir())
	clock := &stepClock{base: goldenBase}
	doc := decodeNext(t, mustRunNoDB(t, clock, "next", "--json"))
	if doc.Recommendation == nil {
		t.Fatal("no recommendation")
	}
	for name, command := range map[string]string{
		"command":   doc.Recommendation.Command,
		"follow-up": doc.Recommendation.FollowUpCommand,
	} {
		if strings.Contains(command, "--db") {
			t.Fatalf("%s repeats the default database: %q", name, command)
		}
	}
}

func TestNextRevisitsAndSkips(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	mustRun(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--answer", "A", "--json")
	mustRun(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--answer", "B", "--json")
	mustRun(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--question", "v1", "--answer", "A", "--json")

	doc := decodeNext(t, mustRun(t, db, clock, "", "next", "--json"))
	vocab := nextSkill(t, doc, "vocabulary-in-context")
	if vocab.Evidence.IndependentAttempts != 2 || vocab.Evidence.IndependentCorrect != 2 {
		t.Fatalf("revisit changed independent evidence: %+v", vocab.Evidence)
	}
	if vocab.Evidence.Revisits != 1 {
		t.Fatalf("revisits = %d", vocab.Evidence.Revisits)
	}
	if vocab.Inference.State != "supported" {
		t.Fatalf("state = %s", vocab.Inference.State)
	}

	skipDB := newDB(t)
	mustRun(t, skipDB, clock, "", "skill", "practice", "sentence-structure", "--skip", "--json")
	skipDoc := decodeNext(t, mustRun(t, skipDB, clock, "", "next", "--json"))
	sentence := nextSkill(t, skipDoc, "sentence-structure")
	if sentence.Evidence.IndependentAttempts != 1 || sentence.Evidence.IndependentSkipped != 1 {
		t.Fatalf("skip evidence = %+v", sentence.Evidence)
	}
	if sentence.Inference.State != "insufficient" || sentence.Inference.CausalDiagnosis != "not_attempted" {
		t.Fatalf("skip inference = %+v", sentence.Inference)
	}
}

func TestNextUnclassifiedAttempt(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	mustRun(t, db, clock, "", "practice", "--question", "q1", "--answer", "B", "--json")
	doc := decodeNext(t, mustRun(t, db, clock, "", "next", "--json"))
	if len(doc.UnclassifiedAttempts) != 1 {
		t.Fatalf("unclassified = %+v", doc.UnclassifiedAttempts)
	}
	if doc.UnclassifiedAttempts[0].Reason == "" {
		t.Fatal("unclassified reason is empty")
	}
	scoring := nextSkill(t, doc, "not-except-polarity")
	if scoring.Evidence.Attempts != 0 {
		t.Fatalf("untagged attempt counted for the scoring skill: %+v", scoring.Evidence)
	}
}

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

func runChildCLI(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "PASSQUEST_TEST_RUN_MAIN=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("child %v: %v\nstderr:\n%s", args, err, stderr.String())
	}
	return stdout.String()
}

func TestNextRecordedEmptySnapshotDoesNotRegroupAfterTagEdit(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	imported := importFixture(t, db, clock)
	sourceID := imported.Source.ID
	mustRun(t, db, clock, "", "question", "add", sourceID, "1", "--file", fixtureFile(t, "question-q1.json"), "--json")
	mustRun(t, db, clock, "", "page", "review", sourceID, "1", "--reviewer", "fixture-reviewer", "--json")
	mustRun(t, db, clock, "", "question", "review", sourceID, "q1", "--answer", "B", "--reviewer", "fixture-reviewer", "--json")
	mustRun(t, db, clock, "", "practice", "--source", sourceID, "--question", "q1", "--answer", "B", "--json")

	raw, err := os.ReadFile(fixtureFile(t, "question-q1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var question map[string]any
	if err := json.Unmarshal(raw, &question); err != nil {
		t.Fatal(err)
	}
	question["tactics"] = []string{"not-except-polarity"}
	tagged, err := json.Marshal(question)
	if err != nil {
		t.Fatal(err)
	}
	taggedPath := filepath.Join(t.TempDir(), "question-q1-tagged.json")
	if err := os.WriteFile(taggedPath, tagged, 0o600); err != nil {
		t.Fatal(err)
	}
	mustRun(t, db, clock, "", "question", "edit", sourceID, "q1", "--file", taggedPath, "--json")
	mustRun(t, db, clock, "", "question", "review", sourceID, "q1", "--answer", "B", "--reviewer", "fixture-reviewer", "--json")

	doc := decodeNext(t, mustRun(t, db, clock, "", "next", "--source", sourceID, "--json"))
	scoring := nextSkill(t, doc, "not-except-polarity")
	if scoring.Evidence.Attempts != 0 || scoring.Evidence.MappedFromCurrentTags != 0 {
		t.Fatalf("a recorded empty snapshot was regrouped after a tag edit: %+v", scoring.Evidence)
	}
	if len(doc.UnclassifiedAttempts) != 1 {
		t.Fatalf("unclassified attempts = %+v", doc.UnclassifiedAttempts)
	}
	want := "question had no registered skill tag when the attempt was recorded"
	if doc.UnclassifiedAttempts[0].Reason != want {
		t.Fatalf("unclassified reason = %q, want %q", doc.UnclassifiedAttempts[0].Reason, want)
	}
}

func TestNextReopenRecalculatesWithoutRewriting(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	mustRun(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--answer", "A", "--json")

	first := runChildCLI(t, "next", "--db", db, "--json")
	reportBefore := decodeReport(t, runChildCLI(t, "report", "--db", db, "--json"))
	second := runChildCLI(t, "next", "--db", db, "--json")
	reportAfter := decodeReport(t, runChildCLI(t, "report", "--db", db, "--json"))
	if first != second {
		t.Fatalf("next changed across process reopen\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	if len(reportBefore.Attempts) != len(reportAfter.Attempts) || reportBefore.Attempts[0].ID != reportAfter.Attempts[0].ID {
		t.Fatalf("next rewrote attempts: before %+v after %+v", reportBefore.Attempts, reportAfter.Attempts)
	}
}

func TestGoldenNextEmpty(t *testing.T) {
	golden := goldenPath(t, "next-empty.golden.json")
	t.Chdir(t.TempDir())
	db := "attempts.sqlite"
	clock := &stepClock{base: goldenBase}
	out := mustRun(t, db, clock, "", "next", "--json")
	checkGolden(t, golden, []byte(out))
}

func TestGoldenNextAfterVocab(t *testing.T) {
	golden := goldenPath(t, "next-after-vocab.golden.json")
	t.Chdir(t.TempDir())
	db := "attempts.sqlite"
	clock := &stepClock{base: goldenBase}
	mustRun(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--answer", "A", "--json")
	out := mustRun(t, db, clock, "", "next", "--json")
	checkGolden(t, golden, []byte(out))
}

func TestGoldenNextSlow(t *testing.T) {
	golden := goldenPath(t, "next-slow.golden.json")
	t.Chdir(t.TempDir())
	db := "attempts.sqlite"
	clock := slowVocabClock(t, db)
	out := mustRunWithNow(t, db, clock, "", "next", "--json")
	checkGolden(t, golden, []byte(out))
}
