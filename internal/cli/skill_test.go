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

type sequenceClock struct {
	times []time.Time
	index int
}

func (c *sequenceClock) Now() time.Time {
	if len(c.times) == 0 {
		return goldenBase
	}
	if c.index >= len(c.times) {
		return c.times[len(c.times)-1]
	}
	value := c.times[c.index]
	c.index++
	return value
}

func runCLIWithNow(t *testing.T, db string, clock *sequenceClock, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	opt := Options{
		Stdin:      strings.NewReader(stdin),
		Stdout:     &stdout,
		Stderr:     &stderr,
		Now:        clock.Now,
		NewSession: func() string { return "sess-test" },
		Getenv:     func(string) string { return "" },
	}
	code := Run(append(args, "--db", db), opt)
	return code, stdout.String(), stderr.String()
}

func mustRunWithNow(t *testing.T, db string, clock *sequenceClock, stdin string, args ...string) string {
	t.Helper()
	code, out, errOut := runCLIWithNow(t, db, clock, stdin, args...)
	if code != 0 {
		t.Fatalf("Run(%v) = %d\nstdout:\n%s\nstderr:\n%s", args, code, out, errOut)
	}
	return out
}

func decodeSkillList(t *testing.T, raw string) skillListDoc {
	t.Helper()
	var doc skillListDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode skill list: %v\n%s", err, raw)
	}
	return doc
}

func decodeSkillShow(t *testing.T, raw string) skillShowDoc {
	t.Helper()
	var doc skillShowDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode skill show: %v\n%s", err, raw)
	}
	return doc
}

func decodeSkillAnswered(t *testing.T, raw string) answeredDoc {
	t.Helper()
	var doc answeredDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode skill answered: %v\n%s", err, raw)
	}
	return doc
}

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

func reviewedTaggedFixtureQuestion(t *testing.T, db string, clock *stepClock, sourceID string) {
	t.Helper()
	mustRun(t, db, clock, "", "question", "add", sourceID, "1", "--file", fixtureFile(t, "question-q2-tagged.json"), "--json")
	mustRun(t, db, clock, "", "page", "review", sourceID, "1", "--reviewer", "fixture-reviewer", "--json")
	mustRun(t, db, clock, "", "question", "review", sourceID, "q2", "--answer", "C", "--reviewer", "fixture-reviewer", "--json")
}

func assertAwaitingHidesKey(t *testing.T, raw string) {
	t.Helper()
	var generic any
	if err := json.Unmarshal([]byte(raw), &generic); err != nil {
		t.Fatalf("decode generic awaiting doc: %v", err)
	}
	keys := map[string]bool{}
	collectKeys(generic, keys)
	for _, key := range []string{"answer", "correct_label", "correct_text", "evidence", "reasoning", "selected_reasoning", "status", "feedback"} {
		if keys[key] {
			t.Errorf("awaiting JSON exposes key %q", key)
		}
	}
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

func TestSkillLessons(t *testing.T) {
	clock := &stepClock{base: goldenBase}
	var list skillListDoc
	if err := json.Unmarshal([]byte(mustRunNoDB(t, clock, "skill", "list", "--json")), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Skills) != 3 {
		t.Fatalf("skills = %d, want 3", len(list.Skills))
	}
	if list.Skills[0].ID != "not-except-polarity" || list.Skills[0].Kind != "scoring" {
		t.Fatalf("first skill = %+v", list.Skills[0])
	}
	for _, id := range []string{"vocabulary-in-context", "sentence-structure"} {
		show := decodeSkillShow(t, mustRunNoDB(t, clock, "skill", "show", id, "--json"))
		if show.Skill.ID != id || show.Skill.Kind != "language" {
			t.Fatalf("show %s = %+v", id, show.Skill)
		}
		if show.Skill.Principle == "" || show.Skill.WhyItWorks == "" || show.Skill.WhenItWorks == "" || show.Skill.WatchOut == "" {
			t.Fatalf("show %s is incomplete: %+v", id, show.Skill)
		}
		if len(show.Skill.Steps) < 3 {
			t.Fatalf("show %s steps = %d", id, len(show.Skill.Steps))
		}
		if show.Skill.TacticID != "" {
			t.Fatalf("language skill %s names tactic %q", id, show.Skill.TacticID)
		}
	}

	scoring := decodeSkillShow(t, mustRunNoDB(t, clock, "skill", "show", "not-except-polarity", "--json"))
	if scoring.Skill.TacticID != "not-except-polarity" {
		t.Fatalf("scoring tactic id = %q", scoring.Skill.TacticID)
	}
	for _, want := range []string{"repeats the most words", "longest option"} {
		if !strings.Contains(scoring.Skill.WatchOut, want) {
			t.Fatalf("scoring watch-out lacks %q: %s", want, scoring.Skill.WatchOut)
		}
	}

	human := mustRunNoDB(t, clock, "skill", "show", "vocabulary-in-context")
	for _, want := range []string{"Kind:", "Principle:", "Steps:", "Watch out:"} {
		if !strings.Contains(human, want) {
			t.Errorf("human skill lesson lacks %q:\n%s", want, human)
		}
	}

	code, out, errOut := runNoDB(t, clock, "", "skill", "show", "no-such-skill", "--json")
	if code != 2 || out != "" {
		t.Fatalf("unknown skill = exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got := errorCode(t, errOut); got != "unknown_skill" {
		t.Fatalf("unknown skill code = %q", got)
	}
}

func TestSkillPracticeHidesKeyAndSnapshotsSkill(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}

	awaiting := mustRun(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--json")
	assertAwaitingHidesKey(t, awaiting)
	var doc awaitingDoc
	if err := json.Unmarshal([]byte(awaiting), &doc); err != nil {
		t.Fatalf("decode awaiting: %v", err)
	}
	if doc.State != "awaiting_answer" || doc.Skill == nil {
		t.Fatalf("awaiting = %+v", doc)
	}
	if doc.Skill.ID != "vocabulary-in-context" || doc.Skill.Kind != "language" {
		t.Fatalf("skill view = %+v", doc.Skill)
	}
	if doc.Skill.Assistance != "independent" || doc.Skill.PassageExposure != "fresh" {
		t.Fatalf("skill condition = %+v", doc.Skill)
	}
	if doc.Content.ID != "skill-first-party-vocab-notice" || doc.Question.ID != "v1" {
		t.Fatalf("selected material = %s/%s", doc.Content.ID, doc.Question.ID)
	}

	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	if report.Counts.Attempts != 0 {
		t.Fatalf("awaiting request recorded %d attempts", report.Counts.Attempts)
	}

	answered := decodeSkillAnswered(t, mustRun(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--answer", "A", "--json"))
	if answered.Attempt.Status != "correct" || answered.Attempt.Assistance != "independent" {
		t.Fatalf("attempt = %+v", answered.Attempt)
	}
	if len(answered.Attempt.Skills) != 1 || answered.Attempt.Skills[0] != "vocabulary-in-context" {
		t.Fatalf("attempt skills = %v", answered.Attempt.Skills)
	}
	if answered.Skill == nil || answered.Skill.PassageExposure != "fresh" {
		t.Fatalf("answered skill view = %+v", answered.Skill)
	}
}

func TestSkillPracticeSecondPassageAndRepeat(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	mustRun(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--answer", "A", "--json")
	mustRun(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--answer", "B", "--json")
	mustRun(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--answer", "A", "--json")

	repeat := decodeSkillAnswered(t, mustRun(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--json"))
	if repeat.Skill == nil || repeat.Skill.PassageExposure != "already_seen" {
		t.Fatalf("fourth selection skill view = %+v", repeat.Skill)
	}
	if repeat.Question.ID != "v1" {
		t.Fatalf("fourth selection question = %s, want the first candidate repeat", repeat.Question.ID)
	}
	answered := decodeSkillAnswered(t, mustRun(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--answer", "A", "--json"))
	if answered.Attempt.AttemptNumber != 2 || !answered.Attempt.Repeat {
		t.Fatalf("repeat attempt = %+v", answered.Attempt)
	}
	if answered.Attempt.Assistance != "independent" {
		t.Fatalf("repeat assistance = %q", answered.Attempt.Assistance)
	}

	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	if report.Counts.Attempts != 4 || report.Counts.FirstAttempts != 3 || report.Counts.RepeatedAttempts != 1 {
		t.Fatalf("report counts = %+v", report.Counts)
	}
	if report.Counts.IndependentAttempts != 3 {
		t.Fatalf("independent attempts = %d, want first attempts only", report.Counts.IndependentAttempts)
	}
}

func TestSkillPracticeScoringRequiresGuidedAttempt(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}

	code, out, errOut := runCLI(t, db, clock, "", "skill", "practice", "not-except-polarity", "--json")
	if code != 2 || out != "" {
		t.Fatalf("scoring practice before guided = exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got := errorCode(t, errOut); got != "guided_required" {
		t.Fatalf("error code = %q, want guided_required\n%s", got, errOut)
	}
	if report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json")); report.Counts.Attempts != 0 {
		t.Fatalf("refused scoring practice recorded %d attempts", report.Counts.Attempts)
	}

	mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--answer", "D", "--json")
	awaiting := decodeAwaiting(t, mustRun(t, db, clock, "", "skill", "practice", "not-except-polarity", "--json"))
	if awaiting.Question.ID != "x1" {
		t.Fatalf("scoring practice selected %s, want the fresh scoring passage x1", awaiting.Question.ID)
	}
	if awaiting.Skill == nil || awaiting.Skill.Assistance != "independent" || awaiting.Skill.PassageExposure != "fresh" {
		t.Fatalf("scoring practice skill view = %+v", awaiting.Skill)
	}
	answered := decodeSkillAnswered(t, mustRun(t, db, clock, "", "skill", "practice", "not-except-polarity", "--answer", "C", "--json"))
	if answered.Attempt.Status != "correct" || answered.Attempt.Assistance != "independent" {
		t.Fatalf("scoring practice attempt = %+v", answered.Attempt)
	}
}

func TestNextSendsUnGuidedScoringToTacticWalkthrough(t *testing.T) {
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
	wantCommand := "passquest tactic practice not-except-polarity --stage guided --db " + quoteCommandArgument(db)
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

func TestSkillPracticeSkipsGuidedExposure(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	imported := importFixture(t, db, clock)
	sourceID := imported.Source.ID
	reviewedTaggedFixtureQuestion(t, db, clock, sourceID)

	// The guided attempt is on the source, so the embedded exercise passage stays
	// fresh. Showing its guided walkthrough without answering exposes t1 only.
	mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--source", sourceID, "--answer", "C", "--json")
	mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--question", "t1", "--stage", "guided", "--json")

	awaiting := decodeAwaiting(t, mustRun(t, db, clock, "", "skill", "practice", "not-except-polarity", "--json"))
	if awaiting.Question.ID != "t2" {
		t.Fatalf("skill practice selected %s, want the unexposed t2", awaiting.Question.ID)
	}
	if awaiting.Skill == nil || awaiting.Skill.Assistance != "independent" || awaiting.Skill.PassageExposure != "fresh" {
		t.Fatalf("skill practice skill view = %+v", awaiting.Skill)
	}
}

func TestSkillPracticeAssistedFreshPreambleIsTruthful(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	imported := importFixture(t, db, clock)
	sourceID := imported.Source.ID
	reviewedTaggedFixtureQuestion(t, db, clock, sourceID)

	mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--answer", "D", "--json")
	mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--source", sourceID, "--question", "q2", "--stage", "guided", "--json")

	human := mustRun(t, db, clock, "", "skill", "practice", "not-except-polarity", "--source", sourceID, "--question", "q2", "--answer", "C")
	if !strings.Contains(human, "the earlier walkthrough makes this attempt assisted; it is not independent transfer evidence") {
		t.Fatalf("assisted-fresh preamble is not truthful:\n%s", human)
	}
	if strings.Contains(human, "a correct answer can be independent transfer evidence") {
		t.Fatalf("assisted-fresh preamble claims independent transfer:\n%s", human)
	}
	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	if report.Attempts[len(report.Attempts)-1].Assistance != "assisted" {
		t.Fatalf("recorded assistance = %+v", report.Attempts[len(report.Attempts)-1])
	}
}

func TestSkillPracticeSourcePath(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	imported := importFixture(t, db, clock)
	sourceID := imported.Source.ID
	reviewedTaggedFixtureQuestion(t, db, clock, sourceID)

	// Q-0003-02: scoring practice records independent evidence, so it follows the
	// guided walkthrough. The embedded guided example does not touch the source.
	guided := decodeAnswered(t, mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--answer", "D", "--json"))
	if guided.Attempt.Assistance != "assisted" {
		t.Fatalf("guided attempt assistance = %q", guided.Attempt.Assistance)
	}

	awaitingRaw := mustRun(t, db, clock, "", "skill", "practice", "not-except-polarity", "--source", sourceID, "--json")
	assertAwaitingHidesKey(t, awaitingRaw)
	var awaiting awaitingDoc
	if err := json.Unmarshal([]byte(awaitingRaw), &awaiting); err != nil {
		t.Fatalf("decode awaiting: %v", err)
	}
	if awaiting.Skill == nil || awaiting.Skill.ID != "not-except-polarity" || awaiting.Skill.Kind != "scoring" {
		t.Fatalf("skill view = %+v", awaiting.Skill)
	}
	if awaiting.Question.ID != "q2" || awaiting.Content.ID != sourceID+"-p001" {
		t.Fatalf("selected source material = %s/%s", awaiting.Content.ID, awaiting.Question.ID)
	}
	if awaiting.Question.Review == nil || awaiting.Question.Review.PageStatus != "reviewed" || awaiting.Question.Review.QuestionStatus != "reviewed" {
		t.Fatalf("source review = %+v", awaiting.Question.Review)
	}

	answered := decodeSkillAnswered(t, mustRun(t, db, clock, "", "skill", "practice", "not-except-polarity", "--source", sourceID, "--answer", "C", "--json"))
	if answered.Attempt.ContentID != sourceID+"-p001" || answered.Attempt.Status != "correct" {
		t.Fatalf("source attempt = %+v", answered.Attempt)
	}
	if len(answered.Attempt.Skills) != 1 || answered.Attempt.Skills[0] != "not-except-polarity" {
		t.Fatalf("source attempt skills = %v", answered.Attempt.Skills)
	}

	next := decodeNext(t, mustRun(t, db, clock, "", "next", "--source", sourceID, "--json"))
	if next.SourceScope != sourceID || next.SourceTaggedQuestions < 1 {
		t.Fatalf("source scope = %q/%d", next.SourceScope, next.SourceTaggedQuestions)
	}
	scoring := nextSkill(t, next, "not-except-polarity")
	if scoring.Evidence.Attempts != 2 || scoring.Evidence.AssistedAttempts != 1 || scoring.Evidence.IndependentAttempts != 1 || scoring.Evidence.IndependentCorrect != 1 {
		t.Fatalf("scoring evidence = %+v", scoring.Evidence)
	}
	if scoring.Inference.State != "insufficient" || scoring.Inference.Action != "practice_new" {
		t.Fatalf("scoring inference = %+v", scoring.Inference)
	}

	// The source has no language-tagged question, so the language practice path
	// refuses rather than substituting embedded material.
	code, out, errOut := runCLI(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--source", sourceID, "--json")
	if code != 2 || out != "" {
		t.Fatalf("untagged source practice = exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got := errorCode(t, errOut); got != "not_practiceable" {
		t.Fatalf("untagged source code = %q", got)
	}
}

func TestNextIncludesImportedSourceWithoutScope(t *testing.T) {
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

func TestSkillPracticeRejectsBadInput(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	imported := importFixture(t, db, clock)
	tests := []struct {
		name string
		args []string
		code string
	}{
		{"unknown skill", []string{"skill", "practice", "no-such-skill", "--json"}, "unknown_skill"},
		{"conflicting answer", []string{"skill", "practice", "vocabulary-in-context", "--answer", "A", "--skip", "--json"}, "invalid_input"},
		{"unknown question", []string{"skill", "practice", "vocabulary-in-context", "--question", "missing", "--json"}, "invalid_input"},
		{"unknown source", []string{"skill", "practice", "vocabulary-in-context", "--source", "src-missing", "--json"}, "invalid_input"},
		{"untagged source", []string{"skill", "practice", "vocabulary-in-context", "--source", imported.Source.ID, "--json"}, "not_practiceable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := runCLI(t, db, clock, "", tt.args...)
			if code != 2 || out != "" {
				t.Fatalf("exit = %d, stdout %q, stderr %q", code, out, errOut)
			}
			if got := errorCode(t, errOut); got != tt.code {
				t.Fatalf("error code = %q, want %q\n%s", got, tt.code, errOut)
			}
		})
	}
	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	if report.Counts.Attempts != 0 {
		t.Fatalf("invalid input recorded %d attempts", report.Counts.Attempts)
	}
}

func TestNextNoEvidence(t *testing.T) {
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
	wantCommand := "passquest skill practice not-except-polarity --db " + quoteCommandArgument(db)
	if doc.Recommendation == nil || doc.Recommendation.Skill.ID != "not-except-polarity" || doc.Recommendation.Command != wantCommand {
		t.Fatalf("recommendation = %+v, want command %q", doc.Recommendation, wantCommand)
	}
}

func TestNextTimeChangesAction(t *testing.T) {
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
	if !strings.Contains(doc.Recommendation.Command, "--db "+quoteCommandArgument(db)) {
		t.Fatalf("vocabulary command does not preserve the database: %q", doc.Recommendation.Command)
	}
}

func TestNextExhaustedWeakYieldsToFreshMaterial(t *testing.T) {
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
		if !strings.Contains(doc.Recommendation.Command, "--db "+quoteCommandArgument(db)) {
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
			!strings.Contains(doc.Recommendation.FollowUpCommand, "--db "+quoteCommandArgument(db)) {
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
	if !strings.Contains(doc.Recommendation.Command, "--db "+quoteCommandArgument(db)) {
		t.Fatalf("review command does not preserve the database: %q", doc.Recommendation.Command)
	}
}

func TestNextCommandPreservesSelectedDatabase(t *testing.T) {
	db := filepath.Join(t.TempDir(), "learner data", "attempts.sqlite")
	if err := os.MkdirAll(filepath.Dir(db), 0o700); err != nil {
		t.Fatal(err)
	}
	clock := &stepClock{base: goldenBase}
	doc := decodeNext(t, mustRun(t, db, clock, "", "next", "--json"))
	if doc.Recommendation == nil {
		t.Fatal("no recommendation")
	}
	want := "--db " + quoteCommandArgument(db)
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

func TestQuoteCommandArgument(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "attempts.sqlite", "attempts.sqlite"},
		{"path with space", "/tmp/learner data/attempts.sqlite", `"/tmp/learner data/attempts.sqlite"`},     // portable-paths: allow the POSIX quoting form is the assertion; the Windows form is Q-0006-06
		{"windows path with space", `C:\Users\A Name\attempts.sqlite`, `"C:\Users\A Name\attempts.sqlite"`}, // portable-paths: allow the POSIX quoting form is the assertion; the Windows form is Q-0006-06
		{"dollar", "$HOME/attempts.sqlite", `"\$HOME/attempts.sqlite"`},
		{"double quote", `a"b.sqlite`, `"a\"b.sqlite"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quoteCommandArgument(tt.in); got != tt.want {
				t.Fatalf("quoteCommandArgument(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNextRevisitsAndSkips(t *testing.T) {
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

func TestGoldenSkillList(t *testing.T) {
	clock := &stepClock{base: goldenBase}
	out := mustRunNoDB(t, clock, "skill", "list", "--json")
	checkGolden(t, filepath.Join("testdata", "skill-list.golden.json"), []byte(out))
}

func TestGoldenSkillShow(t *testing.T) {
	clock := &stepClock{base: goldenBase}
	out := mustRunNoDB(t, clock, "skill", "show", "vocabulary-in-context", "--json")
	checkGolden(t, filepath.Join("testdata", "skill-show.golden.json"), []byte(out))
}

func TestGoldenSkillAwaiting(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	out := mustRun(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--json")
	checkGolden(t, filepath.Join("testdata", "skill-awaiting.golden.json"), []byte(out))
}

func TestGoldenSkillAnswered(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	out := mustRun(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--answer", "A", "--json")
	checkGolden(t, filepath.Join("testdata", "skill-answered.golden.json"), []byte(out))
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

func goldenPath(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return path
}
