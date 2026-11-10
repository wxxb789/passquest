package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// runNoDB executes a command that does not open the database, so the shared
// runCLI helper's trailing --db flag does not turn a lesson command into a flag
// error.
func runNoDB(t *testing.T, clock *stepClock, stdin string, args ...string) (int, string, string) {
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
	code := Run(args, opt)
	return code, stdout.String(), stderr.String()
}

func mustRunNoDB(t *testing.T, clock *stepClock, args ...string) string {
	t.Helper()
	code, out, errOut := runNoDB(t, clock, "", args...)
	if code != 0 {
		t.Fatalf("Run(%v) = %d\nstdout:\n%s\nstderr:\n%s", args, code, out, errOut)
	}
	return out
}

func errorCode(t *testing.T, raw string) string {
	t.Helper()
	var doc errorDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode error doc: %v\n%s", err, raw)
	}
	return doc.Error.Code
}

func TestTacticLessonSurfacesWhyWhenAndBoundary(t *testing.T) {
	clock := &stepClock{base: goldenBase}

	var list tacticListDoc
	if err := json.Unmarshal([]byte(mustRunNoDB(t, clock, "tactic", "list", "--json")), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Tactics) != 1 || list.Tactics[0].ID != "not-except-polarity" {
		t.Fatalf("tactics = %+v", list.Tactics)
	}
	if list.Tactics[0].Summary == "" || list.Tactics[0].Title == "" {
		t.Fatalf("list entry = %+v", list.Tactics[0])
	}

	raw := mustRunNoDB(t, clock, "tactic", "show", "not-except-polarity", "--json")
	var show tacticShowDoc
	if err := json.Unmarshal([]byte(raw), &show); err != nil {
		t.Fatalf("decode show: %v", err)
	}
	lesson := show.Tactic
	if lesson.ID != "not-except-polarity" || lesson.WhyItWorks == "" || lesson.WhenItWorks == "" || lesson.Principle == "" {
		t.Fatalf("lesson = %+v", lesson)
	}
	if len(lesson.Steps) < 3 {
		t.Fatalf("steps = %d", len(lesson.Steps))
	}
	boundary := strings.ToLower(lesson.Boundary.Folklore + " " + lesson.Boundary.Example + " " + lesson.Boundary.WhyItFails)
	for _, want := range []string{"repeats the most words", "longest option", "not true", "word overlap"} {
		if !strings.Contains(boundary, want) {
			t.Errorf("boundary lacks %q: %s", want, boundary)
		}
	}

	human := mustRunNoDB(t, clock, "tactic", "show", "not-except-polarity")
	for _, want := range []string{"Why it works:", "When it works:", "Steps:", "Boundary", "Example:", "Why it fails:"} {
		if !strings.Contains(human, want) {
			t.Errorf("human lesson lacks %q:\n%s", want, human)
		}
	}

	code, out, errOut := runNoDB(t, clock, "", "tactic", "show", "no-such-tactic", "--json")
	if code != 2 || out != "" {
		t.Fatalf("unknown tactic = exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got := errorCode(t, errOut); got != "unknown_tactic" {
		t.Fatalf("unknown tactic code = %q", got)
	}
}

func TestTacticGuidedThenIndependentKeepsAssistanceSeparate(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}

	guided := decodeAwaiting(t, mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--json"))
	if guided.State != "awaiting_answer" {
		t.Fatalf("guided state = %q", guided.State)
	}
	if guided.Tactic == nil || guided.Tactic.Stage != "guided" || guided.Tactic.Assistance != "assisted" {
		t.Fatalf("guided tactic view = %+v", guided.Tactic)
	}
	if guided.Guidance == nil || guided.Guidance.WhyItWorks == "" || guided.Guidance.Boundary.Example == "" {
		t.Fatalf("guided guidance = %+v", guided.Guidance)
	}
	if guided.Question.ID != "t1" {
		t.Fatalf("guided question = %q, want the first tagged question t1", guided.Question.ID)
	}
	assertTacticAwaitingHidesKey(t, mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--json"))

	guidedAnswer := decodeAnswered(t, mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--json", "--answer", "D"))
	if guidedAnswer.Attempt.Assistance != "assisted" || guidedAnswer.Attempt.TacticID != "not-except-polarity" {
		t.Fatalf("guided attempt = %+v", guidedAnswer.Attempt)
	}
	if guidedAnswer.Attempt.Status != "correct" || guidedAnswer.Feedback.Assistance != "assisted" {
		t.Fatalf("guided result = %+v / %+v", guidedAnswer.Attempt, guidedAnswer.Feedback)
	}
	if guidedAnswer.Tactic == nil || guidedAnswer.Tactic.Stage != "guided" {
		t.Fatalf("guided answered tactic = %+v", guidedAnswer.Tactic)
	}

	independentRaw := mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--json")
	independent := decodeAwaiting(t, independentRaw)
	if independent.Tactic == nil || independent.Tactic.Stage != "independent" || independent.Tactic.Assistance != "independent" {
		t.Fatalf("independent tactic view = %+v", independent.Tactic)
	}
	if independent.Guidance != nil {
		t.Fatal("independent awaiting document still carries guidance")
	}
	if independent.Question.ID == guided.Question.ID {
		t.Fatalf("independent question %q repeats the guided question", independent.Question.ID)
	}
	assertTacticAwaitingHidesKey(t, independentRaw)

	independentAnswer := decodeAnswered(t, mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--json", "--answer", "C"))
	if independentAnswer.Attempt.Assistance != "independent" || independentAnswer.Attempt.TacticID != "not-except-polarity" {
		t.Fatalf("independent attempt = %+v", independentAnswer.Attempt)
	}
	if independentAnswer.Attempt.Status != "correct" {
		t.Fatalf("independent status = %q, want correct", independentAnswer.Attempt.Status)
	}

	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	counts := report.Counts
	if counts.Attempts != 2 || counts.AssistedAttempts != 1 || counts.AssistedCorrect != 1 ||
		counts.IndependentAttempts != 1 || counts.IndependentCorrect != 1 {
		t.Fatalf("counts = %+v", counts)
	}
	if len(report.Questions) != 3 {
		t.Fatalf("question rows = %d", len(report.Questions))
	}
	rows := map[string]questionReport{}
	for _, q := range report.Questions {
		rows[q.ID] = q
	}
	if rows["t1"].Assisted != 1 || rows["t1"].Independent != 0 {
		t.Fatalf("t1 row = %+v", rows["t1"])
	}
	if rows["t2"].Assisted != 0 || rows["t2"].Independent != 1 {
		t.Fatalf("t2 row = %+v", rows["t2"])
	}
	if len(report.Attempts) != 2 ||
		report.Attempts[0].Assistance != "assisted" ||
		report.Attempts[1].Assistance != "independent" {
		t.Fatalf("attempt rows = %+v", report.Attempts)
	}

	human := mustRun(t, db, clock, "", "report")
	for _, want := range []string{
		"Independent attempts (no hint): 1 (correct 1, incorrect 0, skipped 0)",
		"Assisted attempts (help shown): 1 (correct 1, incorrect 0, skipped 0)",
		"t1  attempts 1  first correct  latest correct  repeated 0  assisted 1  independent 0",
		"#1  t1  correct  first  assisted (not-except-polarity)",
		"#2  t2  correct  first  independent (not-except-polarity)",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("human report lacks %q:\n%s", want, human)
		}
	}

	code, out, errOut := runCLI(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--stage", "independent", "--json")
	if code != 2 || out != "" {
		t.Fatalf("exhausted independent = exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got := errorCode(t, errOut); got != "not_practiceable" {
		t.Fatalf("exhausted independent code = %q", got)
	}
	if !strings.Contains(errOut, "no unattempted question") {
		t.Fatalf("exhausted independent message = %s", errOut)
	}
}

func decodeAwaiting(t *testing.T, raw string) awaitingDoc {
	t.Helper()
	var doc awaitingDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode awaiting doc: %v\n%s", err, raw)
	}
	return doc
}

func assertTacticAwaitingHidesKey(t *testing.T, raw string) {
	t.Helper()
	var doc any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode awaiting: %v", err)
	}
	keys := map[string]bool{}
	collectKeys(doc, keys)
	for _, forbidden := range []string{"answer", "correct_label", "correct_text", "evidence", "reasoning", "status", "feedback"} {
		if keys[forbidden] {
			t.Errorf("tactic awaiting JSON exposes %q", forbidden)
		}
	}
}

func TestTacticIndependentRequiresGuided(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	code, out, errOut := runCLI(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--stage", "independent", "--json")
	if code != 2 || out != "" {
		t.Fatalf("independent before help = exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got := errorCode(t, errOut); got != "guided_required" {
		t.Fatalf("independent before help code = %q", got)
	}
	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	if report.Counts.Attempts != 0 {
		t.Fatalf("refused independent recorded %d attempts", report.Counts.Attempts)
	}
}

func TestTacticExplicitQuestionRules(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}

	code, _, errOut := runCLI(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--question", "t2", "--stage", "independent", "--json")
	if code != 2 || errorCode(t, errOut) != "guided_required" {
		t.Fatalf("explicit independent before help = exit %d, %s", code, errOut)
	}

	guided := decodeAwaiting(t, mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--question", "t1", "--json"))
	if guided.Question.ID != "t1" || guided.Tactic == nil || guided.Tactic.Stage != "guided" {
		t.Fatalf("explicit guided = %+v / %+v", guided.Question, guided.Tactic)
	}
	mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--question", "t1", "--answer", "D", "--json")

	code, _, errOut = runCLI(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--stage", "independent", "--question", "t1", "--json")
	if code != 2 || errorCode(t, errOut) != "invalid_input" {
		t.Fatalf("guided question as independent = exit %d, %s", code, errOut)
	}
	if !strings.Contains(errOut, "guided example") {
		t.Fatalf("guided-question refusal = %s", errOut)
	}

	code, _, errOut = runCLI(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--stage", "independent", "--question", "t3", "--json")
	if code != 2 || errorCode(t, errOut) != "invalid_input" {
		t.Fatalf("untagged question = exit %d, %s", code, errOut)
	}
	if !strings.Contains(errOut, "not a reviewed question tagged") {
		t.Fatalf("untagged refusal = %s", errOut)
	}

	independent := decodeAwaiting(t, mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--stage", "independent", "--question", "t2", "--json"))
	if independent.Question.ID != "t2" || independent.Guidance != nil {
		t.Fatalf("explicit independent = %+v / %+v", independent.Question, independent.Guidance)
	}
}

func TestTacticHumanOutputShowsHelpAndCondition(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}

	guided := mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--answer", "D")
	for _, want := range []string{
		"Pass Quest — tactic: NOT/EXCEPT polarity (guided example)",
		"Help shown:",
		"Why it works:",
		"Boundary",
		"Assisted attempt:",
		"Recorded attempt #1",
		"assistance assisted, tactic not-except-polarity",
	} {
		if !strings.Contains(guided, want) {
			t.Errorf("guided human output lacks %q:\n%s", want, guided)
		}
	}

	independent := mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--answer", "C")
	for _, want := range []string{
		"Pass Quest — tactic: NOT/EXCEPT polarity (independent question)",
		"No hint is shown.",
		"Independent attempt:",
		"assistance independent, tactic not-except-polarity",
	} {
		if !strings.Contains(independent, want) {
			t.Errorf("independent human output lacks %q:\n%s", want, independent)
		}
	}
}

func TestTacticSourcePracticeKeepsProvenance(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	fixture := fixtureDir(t)
	imported := decodeImport(t, mustRun(t, db, clock, "", "import", "--path", fixture, "--json"))
	sourceID := imported.Source.ID
	for _, file := range []string{"question-q1-tagged.json", "question-q2-tagged.json"} {
		mustRun(t, db, clock, "", "question", "add", sourceID, "1", "--file", fixtureFile(t, file))
	}
	mustRun(t, db, clock, "", "page", "review", sourceID, "1", "--reviewer", "fixture-reviewer")
	mustRun(t, db, clock, "", "question", "review", sourceID, "q1", "--answer", "B", "--reviewer", "fixture-reviewer")
	mustRun(t, db, clock, "", "question", "review", sourceID, "q2", "--answer", "C", "--reviewer", "fixture-reviewer")

	questionsJSON := mustRun(t, db, clock, "", "questions", sourceID, "--json")
	var questions questionsDoc
	if err := json.Unmarshal([]byte(questionsJSON), &questions); err != nil {
		t.Fatalf("decode questions: %v", err)
	}
	for _, q := range questions.Questions {
		if len(q.Tactics) != 1 || q.Tactics[0] != "not-except-polarity" {
			t.Fatalf("question %s tags = %v", q.ID, q.Tactics)
		}
	}

	guided := decodeAwaiting(t, mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--source", sourceID, "--json"))
	wantContent := sourceID + "-p001"
	if guided.Content.ID != wantContent || guided.Question.Review == nil {
		t.Fatalf("guided content/review = %s / %+v", guided.Content.ID, guided.Question.Review)
	}
	if guided.Question.Review.PageStatus != "reviewed" || guided.Question.Review.QuestionStatus != "reviewed" {
		t.Fatalf("guided review = %+v", guided.Question.Review)
	}
	if guided.Tactic == nil || guided.Tactic.Stage != "guided" || guided.Guidance == nil {
		t.Fatalf("guided tactic = %+v", guided.Tactic)
	}
	assertTacticAwaitingHidesKey(t, mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--source", sourceID, "--json"))

	guidedAnswer := decodeAnswered(t, mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--source", sourceID, "--json", "--answer", "B"))
	if guidedAnswer.Attempt.ContentID != wantContent || guidedAnswer.Attempt.Assistance != "assisted" ||
		guidedAnswer.Attempt.TacticID != "not-except-polarity" || guidedAnswer.Attempt.Status != "correct" {
		t.Fatalf("guided source attempt = %+v", guidedAnswer.Attempt)
	}

	independent := decodeAwaiting(t, mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--source", sourceID, "--json"))
	if independent.Tactic == nil || independent.Tactic.Stage != "independent" || independent.Guidance != nil {
		t.Fatalf("independent tactic = %+v / %+v", independent.Tactic, independent.Guidance)
	}
	if independent.Question.ID == guided.Question.ID || independent.Content.ID != wantContent {
		t.Fatalf("independent question/content = %s / %s", independent.Question.ID, independent.Content.ID)
	}
	independentAnswer := decodeAnswered(t, mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--source", sourceID, "--json", "--answer", "C"))
	if independentAnswer.Attempt.Assistance != "independent" || independentAnswer.Attempt.Status != "correct" {
		t.Fatalf("independent source attempt = %+v", independentAnswer.Attempt)
	}

	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	if report.Counts.AssistedAttempts != 1 || report.Counts.IndependentAttempts != 1 {
		t.Fatalf("source counts = %+v", report.Counts)
	}
	if len(report.Contents) != 1 || report.Contents[0].ID != wantContent {
		t.Fatalf("source contents = %+v", report.Contents)
	}
	if !strings.Contains(report.Contents[0].Notice, "private") {
		t.Fatalf("source content notice = %q", report.Contents[0].Notice)
	}
}

func TestTacticSourceRefusesUntaggedMaterial(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	fixture := fixtureDir(t)
	imported := decodeImport(t, mustRun(t, db, clock, "", "import", "--path", fixture, "--json"))
	sourceID := imported.Source.ID
	mustRun(t, db, clock, "", "question", "add", sourceID, "1", "--file", fixtureFile(t, "question-q1.json"))
	mustRun(t, db, clock, "", "page", "review", sourceID, "1", "--reviewer", "fixture-reviewer")
	mustRun(t, db, clock, "", "question", "review", sourceID, "q1", "--answer", "B", "--reviewer", "fixture-reviewer")

	code, out, errOut := runCLI(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--source", sourceID, "--json")
	if code != 2 || out != "" {
		t.Fatalf("untagged source = exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got := errorCode(t, errOut); got != "not_practiceable" {
		t.Fatalf("untagged source code = %q", got)
	}
	if !strings.Contains(errOut, "no reviewed question tagged") {
		t.Fatalf("untagged source message = %s", errOut)
	}
}

func TestTacticSourceRejectsPreviouslyAttemptedIndependentQuestion(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	fixture := fixtureDir(t)
	imported := decodeImport(t, mustRun(t, db, clock, "", "import", "--path", fixture, "--json"))
	sourceID := imported.Source.ID
	for _, file := range []string{"question-q1-tagged.json", "question-q2-tagged.json"} {
		mustRun(t, db, clock, "", "question", "add", sourceID, "1", "--file", fixtureFile(t, file))
	}
	mustRun(t, db, clock, "", "page", "review", sourceID, "1", "--reviewer", "fixture-reviewer")
	mustRun(t, db, clock, "", "question", "review", sourceID, "q1", "--answer", "B", "--reviewer", "fixture-reviewer")
	mustRun(t, db, clock, "", "question", "review", sourceID, "q2", "--answer", "C", "--reviewer", "fixture-reviewer")

	// Ordinary practice on q1 makes it unavailable as independent evidence, even
	// though it was never the guided example.
	ordinary := decodeAnswered(t, mustRun(t, db, clock, "", "practice", "--source", sourceID, "--question", "q1", "--answer", "B", "--json"))
	if ordinary.Attempt.Assistance != "independent" || ordinary.Attempt.TacticID != "" {
		t.Fatalf("ordinary source attempt = %+v", ordinary.Attempt)
	}

	guided := decodeAwaiting(t, mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--source", sourceID, "--question", "q2", "--json"))
	if guided.Question.ID != "q2" {
		t.Fatalf("guided question = %s", guided.Question.ID)
	}
	mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--source", sourceID, "--question", "q2", "--answer", "C", "--json")

	code, _, errOut := runCLI(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--source", sourceID, "--stage", "independent", "--question", "q1", "--json")
	if code != 2 || errorCode(t, errOut) != "not_practiceable" {
		t.Fatalf("attempted independent question = exit %d, %s", code, errOut)
	}
	if !strings.Contains(errOut, "already has 1 earlier attempt") {
		t.Fatalf("attempted independent message = %s", errOut)
	}

	code, _, errOut = runCLI(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--source", sourceID, "--stage", "independent", "--json")
	if code != 2 || errorCode(t, errOut) != "not_practiceable" {
		t.Fatalf("auto independent with no eligible question = exit %d, %s", code, errOut)
	}
	if !strings.Contains(errOut, "no unattempted question") {
		t.Fatalf("auto independent message = %s", errOut)
	}
}

func TestTacticRejectsBadInput(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	cases := []struct {
		name string
		args []string
		code string
	}{
		{"unknown tactic", []string{"tactic", "practice", "nope", "--json"}, "unknown_tactic"},
		{"bad stage", []string{"tactic", "practice", "not-except-polarity", "--stage", "solo", "--json"}, "invalid_input"},
		{"answer and skip", []string{"tactic", "practice", "not-except-polarity", "--answer", "A", "--skip", "--json"}, "invalid_input"},
		{"missing tactic", []string{"tactic", "practice", "--json"}, "invalid_input"},
		{"missing unknown source", []string{"tactic", "practice", "not-except-polarity", "--source", "src-nope", "--json"}, "invalid_input"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errOut := runCLI(t, db, clock, "", tc.args...)
			if code != 2 || out != "" {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
			}
			if got := errorCode(t, errOut); got != tc.code {
				t.Fatalf("code = %q, want %q; stderr %s", got, tc.code, errOut)
			}
		})
	}
	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	if report.Counts.Attempts != 0 {
		t.Fatalf("invalid input recorded %d attempts", report.Counts.Attempts)
	}
}

func TestTacticProtocolGoldens(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	check := func(name, raw string) {
		t.Helper()
		checkGolden(t, filepath.Join("testdata", name), normalizedGolden(t, raw, nil))
	}
	check("tactic-list.golden.json", mustRunNoDB(t, clock, "tactic", "list", "--json"))
	check("tactic-show.golden.json", mustRunNoDB(t, clock, "tactic", "show", "not-except-polarity", "--json"))
	check("tactic-awaiting.golden.json", mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--json"))
	check("tactic-answered.golden.json", mustRun(t, db, clock, "", "tactic", "practice", "not-except-polarity", "--json", "--answer", "D"))
}
