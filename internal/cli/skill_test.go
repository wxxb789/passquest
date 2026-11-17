package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestSkillLessons(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestSkillPracticeSkipsGuidedExposure(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestSkillPracticeRejectsBadInput(t *testing.T) {
	t.Parallel()
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

func TestGoldenSkillList(t *testing.T) {
	t.Parallel()
	clock := &stepClock{base: goldenBase}
	out := mustRunNoDB(t, clock, "skill", "list", "--json")
	checkGolden(t, filepath.Join("testdata", "skill-list.golden.json"), []byte(out))
}

func TestGoldenSkillShow(t *testing.T) {
	t.Parallel()
	clock := &stepClock{base: goldenBase}
	out := mustRunNoDB(t, clock, "skill", "show", "vocabulary-in-context", "--json")
	checkGolden(t, filepath.Join("testdata", "skill-show.golden.json"), []byte(out))
}

func TestGoldenSkillAwaiting(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	out := mustRun(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--json")
	checkGolden(t, filepath.Join("testdata", "skill-awaiting.golden.json"), []byte(out))
}

func TestGoldenSkillAnswered(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	out := mustRun(t, db, clock, "", "skill", "practice", "vocabulary-in-context", "--answer", "A", "--json")
	checkGolden(t, filepath.Join("testdata", "skill-answered.golden.json"), []byte(out))
}

func goldenPath(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return path
}
