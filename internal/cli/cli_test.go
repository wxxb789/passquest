package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/store"
)

var updateGolden = flag.Bool("update-golden", false, "rewrite golden files")

// TestMain doubles as a child-process entry point: the parent test runs the
// real test binary with PASSQUEST_TEST_RUN_MAIN=1, which exercises process
// restart against a real SQLite file instead of influencing an in-process handle.
func TestMain(m *testing.M) {
	if os.Getenv("PASSQUEST_TEST_RUN_MAIN") == "1" {
		os.Exit(Run(os.Args[1:], Options{}))
	}
	os.Exit(m.Run())
}

var goldenBase = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

type stepClock struct {
	base  time.Time
	calls int
}

func (c *stepClock) Now() time.Time {
	c.calls++
	return c.base.Add(time.Duration(c.calls) * time.Second)
}

func newDB(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "attempts.sqlite")
}

func runCLI(t *testing.T, db string, clock *stepClock, stdin string, args ...string) (int, string, string) {
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

func decodeAnswered(t *testing.T, raw string) answeredDoc {
	t.Helper()
	var doc answeredDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode answered doc: %v\n%s", err, raw)
	}
	return doc
}

func decodeReport(t *testing.T, raw string) reportDoc {
	t.Helper()
	var doc reportDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode report doc: %v\n%s", err, raw)
	}
	return doc
}

func mustRun(t *testing.T, db string, clock *stepClock, stdin string, args ...string) string {
	t.Helper()
	code, out, errOut := runCLI(t, db, clock, stdin, args...)
	if code != 0 {
		t.Fatalf("Run(%v) = %d\nstdout:\n%s\nstderr:\n%s", args, code, out, errOut)
	}
	return out
}

// seedFourAttempts runs the shared q1 correct, q2 incorrect, q3 skipped,
// q1 repeated scenario. Callers keep their own oracles over the returned docs.
func seedFourAttempts(t *testing.T, db string, clock *stepClock) []answeredDoc {
	t.Helper()
	steps := [][]string{
		{"practice", "--json", "--question", "q1", "--answer", "B"},
		{"practice", "--json", "--question", "q2", "--answer", "A"},
		{"practice", "--json", "--question", "q3", "--skip"},
		{"practice", "--json", "--question", "q1", "--answer", "B"},
	}
	docs := make([]answeredDoc, 0, len(steps))
	for _, args := range steps {
		docs = append(docs, decodeAnswered(t, mustRun(t, db, clock, "", args...)))
	}
	return docs
}

func TestAwaitingJSONHidesTheKey(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	code, out, errOut := runCLI(t, db, clock, "", "practice", "--json", "--question", "q1")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	doc := decodeAnswered(t, out)
	if doc.State != "awaiting_answer" {
		t.Fatalf("state = %q", doc.State)
	}
	if doc.Question.ID != "q1" || doc.Question.Number != 1 || doc.Question.Total != 3 {
		t.Fatalf("question = %+v", doc.Question)
	}
	if len(doc.Question.Options) != 4 {
		t.Fatalf("options = %d", len(doc.Question.Options))
	}

	var generic any
	if err := json.Unmarshal([]byte(out), &generic); err != nil {
		t.Fatalf("decode generic: %v", err)
	}
	forbidden := []string{"answer", "correct_label", "correct_text", "evidence", "reasoning", "selected_reasoning", "status", "feedback"}
	keys := map[string]bool{}
	collectKeys(generic, keys)
	for _, key := range forbidden {
		if keys[key] {
			t.Errorf("awaiting JSON exposes key %q", key)
		}
	}

	sample, err := exam.Sample()
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	for _, q := range sample.Questions {
		if strings.Contains(out, q.Reasoning) {
			t.Errorf("awaiting JSON contains reasoning for %s", q.ID)
		}
		for _, why := range q.OptionReasoning {
			if strings.Contains(out, why) {
				t.Errorf("awaiting JSON contains option reasoning for %s", q.ID)
			}
		}
	}

	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	if report.Counts.Attempts != 0 {
		t.Fatalf("awaiting request stored %d attempts", report.Counts.Attempts)
	}
}

func collectKeys(v any, out map[string]bool) {
	switch value := v.(type) {
	case map[string]any:
		for key, child := range value {
			out[key] = true
			collectKeys(child, out)
		}
	case []any:
		for _, child := range value {
			collectKeys(child, out)
		}
	}
}

func TestHumanPromptDoesNotRevealTheKey(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	code, out, errOut := runCLI(t, db, clock, "", "practice", "--question", "q1")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	for _, leaked := range []string{"Correct answer:", "Result:", "Why:", "Why your answer is wrong:", "Evidence:", "Recorded attempt"} {
		if strings.Contains(out, leaked) {
			t.Errorf("pre-answer output contains %q", leaked)
		}
	}
	checkGolden(t, filepath.Join("testdata", "human-prompt.golden.txt"), []byte(out))
	sample, err := exam.Sample()
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	for _, q := range sample.Questions {
		if strings.Contains(out, q.Reasoning) {
			t.Errorf("pre-answer output contains reasoning for %s", q.ID)
		}
		for _, why := range q.OptionReasoning {
			if strings.Contains(out, why) {
				t.Errorf("pre-answer output contains option reasoning for %s", q.ID)
			}
		}
	}
	if !strings.Contains(out, "No attempt recorded.") {
		t.Errorf("end-of-input message missing:\n%s", out)
	}
	if !strings.Contains(out, "Not from any live exam") {
		t.Errorf("sample provenance notice missing:\n%s", out)
	}
	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	if report.Counts.Attempts != 0 {
		t.Fatalf("prompt-only request stored %d attempts", report.Counts.Attempts)
	}
}

func TestCorrectIncorrectSkipAndRepeat(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	docs := seedFourAttempts(t, db, clock)
	correct, incorrect, skipped, repeated := docs[0], docs[1], docs[2], docs[3]

	if correct.State != "answered" || correct.Attempt.Status != "correct" {
		t.Fatalf("correct doc = %+v", correct)
	}
	if correct.Attempt.AttemptNumber != 1 || correct.Attempt.PriorAttempts != 0 || correct.Attempt.Repeat {
		t.Fatalf("first attempt numbering = %+v", correct.Attempt)
	}
	if correct.Attempt.SelectedLabel == nil || *correct.Attempt.SelectedLabel != "B" {
		t.Fatalf("selected label = %v", correct.Attempt.SelectedLabel)
	}
	if correct.Feedback.Exposure != attempt.ExposureFirstAttempt {
		t.Fatalf("exposure = %q", correct.Feedback.Exposure)
	}
	if correct.Feedback.Evidence == "" || correct.Feedback.Reasoning == "" {
		t.Fatalf("feedback lacks evidence/reasoning: %+v", correct.Feedback)
	}

	if incorrect.Attempt.Status != "incorrect" || incorrect.Attempt.Repeat {
		t.Fatalf("incorrect doc = %+v", incorrect)
	}
	if incorrect.Feedback.SelectedReasoning == "" {
		t.Fatal("incorrect feedback has no distractor explanation")
	}

	if skipped.Attempt.Status != "skipped" {
		t.Fatalf("skipped status = %q", skipped.Attempt.Status)
	}
	if skipped.Attempt.SelectedLabel != nil || skipped.Attempt.SelectedText != nil {
		t.Fatalf("skipped selection = %v/%v", skipped.Attempt.SelectedLabel, skipped.Attempt.SelectedText)
	}
	if skipped.Feedback.Exposure != attempt.ExposureFirstAttempt {
		t.Fatalf("skip exposure = %q", skipped.Feedback.Exposure)
	}

	if !repeated.Attempt.Repeat || repeated.Attempt.AttemptNumber != 2 || repeated.Attempt.PriorAttempts != 1 {
		t.Fatalf("repeat numbering = %+v", repeated.Attempt)
	}
	if repeated.Feedback.Exposure != attempt.ExposureAnswerAlreadyShown {
		t.Fatalf("repeat exposure = %q", repeated.Feedback.Exposure)
	}

	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	want := countsView{
		Attempts: 4, Correct: 2, Incorrect: 1, Skipped: 1,
		FirstAttempts: 3, FirstCorrect: 1, FirstIncorrect: 1, FirstSkipped: 1,
		RepeatedAttempts: 1, RepeatedCorrect: 1,
		IndependentAttempts: 3, IndependentCorrect: 1, IndependentIncorrect: 1, IndependentSkipped: 1,
	}
	if report.Counts != want {
		t.Fatalf("counts = %+v, want %+v", report.Counts, want)
	}
	if len(report.Questions) != 3 {
		t.Fatalf("question rows = %d", len(report.Questions))
	}
	q1 := report.Questions[0]
	if q1.ID != "q1" || q1.Attempts != 2 || q1.FirstStatus != "correct" || q1.LatestStatus != "correct" || q1.Repeated != 1 {
		t.Fatalf("q1 row = %+v", q1)
	}
	if q1.Assisted != 0 || q1.Independent != 1 {
		t.Fatalf("q1 assistance split = %+v", q1)
	}
	if len(report.Attempts) != 4 {
		t.Fatalf("attempt rows = %d", len(report.Attempts))
	}
	if report.Attempts[3].ID != 4 || !report.Attempts[3].Repeat {
		t.Fatalf("last attempt row = %+v", report.Attempts[3])
	}
	for _, attemptRow := range report.Attempts {
		if attemptRow.Assistance != "independent" || attemptRow.TacticID != "" {
			t.Fatalf("ordinary attempt assistance = %+v", attemptRow)
		}
	}
	if len(report.Limitations) != 5 {
		t.Fatalf("limitations = %d", len(report.Limitations))
	}
	joined := strings.ToLower(strings.Join(report.Limitations, " "))
	for _, want := range []string{"not an exam score", "not independent evidence", "tactic walkthrough", "pass probability is computed", "user-attested"} {
		if !strings.Contains(joined, want) {
			t.Errorf("limitations do not state %q: %v", want, report.Limitations)
		}
	}
}

func TestHumanFeedbackAndReport(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}

	first := mustRun(t, db, clock, "", "practice", "--question", "q1", "--answer", "B")
	for _, want := range []string{"Result: CORRECT", "Evidence:", "First attempt on this question"} {
		if !strings.Contains(first, want) {
			t.Errorf("first human output lacks %q:\n%s", want, first)
		}
	}

	incorrect := mustRun(t, db, clock, "", "practice", "--question", "q2", "--answer", "A")
	for _, want := range []string{"Result: INCORRECT", "Why your answer is wrong:", "Evidence:", "Correct answer:"} {
		if !strings.Contains(incorrect, want) {
			t.Errorf("incorrect human output lacks %q:\n%s", want, incorrect)
		}
	}

	repeated := mustRun(t, db, clock, "", "practice", "--question", "q1", "--answer", "B")
	for _, want := range []string{"Result: CORRECT", "Repeated attempt:", "not independent evidence"} {
		if !strings.Contains(repeated, want) {
			t.Errorf("repeated human output lacks %q:\n%s", want, repeated)
		}
	}

	report := mustRun(t, db, clock, "", "report")
	for _, want := range []string{
		"First attempts: 2 (correct 1, incorrect 1, skipped 0)",
		"Repeated attempts (answer already shown): 1 (correct 1, incorrect 0, skipped 0)",
		"q3  attempts 0  first not attempted",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("human report lacks %q:\n%s", want, report)
		}
	}
}

func TestReportRepeatedIncorrectAndSkipped(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	for _, args := range [][]string{
		{"practice", "--json", "--question", "q1", "--answer", "B"},
		{"practice", "--json", "--question", "q1", "--answer", "A"},
		{"practice", "--json", "--question", "q2", "--skip"},
		{"practice", "--json", "--question", "q2", "--skip"},
	} {
		mustRun(t, db, clock, "", args...)
	}

	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	want := countsView{
		Attempts: 4, Correct: 1, Incorrect: 1, Skipped: 2,
		FirstAttempts: 2, FirstCorrect: 1, FirstSkipped: 1,
		RepeatedAttempts: 2, RepeatedIncorrect: 1, RepeatedSkipped: 1,
		IndependentAttempts: 2, IndependentCorrect: 1, IndependentSkipped: 1,
	}
	if report.Counts != want {
		t.Fatalf("counts = %+v, want %+v", report.Counts, want)
	}
	if report.Questions[0].LatestStatus != "incorrect" || report.Questions[0].Repeated != 1 {
		t.Fatalf("q1 row = %+v", report.Questions[0])
	}
	if report.Questions[1].LatestStatus != "skipped" || report.Questions[1].Repeated != 1 {
		t.Fatalf("q2 row = %+v", report.Questions[1])
	}
}

func TestInvalidInputRecordsNothing(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}

	code, out, errOut := runCLI(t, db, clock, "", "practice", "--json", "--answer", "Z")
	if code != 2 {
		t.Fatalf("invalid answer exit = %d, want 2", code)
	}
	if out != "" {
		t.Fatalf("invalid answer wrote stdout: %s", out)
	}
	var errDoc errorDoc
	if err := json.Unmarshal([]byte(errOut), &errDoc); err != nil {
		t.Fatalf("decode error doc: %v\n%s", err, errOut)
	}
	if errDoc.Error.Code != "invalid_answer" {
		t.Fatalf("error code = %q", errDoc.Error.Code)
	}

	code, _, errOut = runCLI(t, db, clock, "", "practice", "--json", "--question", "missing", "--answer", "A")
	if code != 2 || !strings.Contains(errOut, "unknown_question") {
		t.Fatalf("unknown question = %d, %s", code, errOut)
	}

	code, _, errOut = runCLI(t, db, clock, "", "practice", "--answer", "A", "--skip")
	if code != 2 || !strings.Contains(errOut, "mutually exclusive") {
		t.Fatalf("conflicting flags = %d, %s", code, errOut)
	}

	code, _, errOut = runCLI(t, db, clock, "", "practice", "--answer", "Z")
	if code != 2 || !strings.Contains(errOut, "not one of") {
		t.Fatalf("human invalid answer = %d, %s", code, errOut)
	}

	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	if report.Counts.Attempts != 0 {
		t.Fatalf("invalid requests stored %d attempts", report.Counts.Attempts)
	}
}

func TestJSONUsageErrorsUseJSON(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	tests := []struct {
		name string
		args []string
		code string
	}{
		{"practice unknown flag", []string{"practice", "--json", "--bogus"}, "invalid_flags"},
		{"practice extra argument", []string{"practice", "--json", "extra"}, "unexpected_argument"},
		{"report unknown flag", []string{"report", "--json", "--bogus"}, "invalid_flags"},
		{"report extra argument", []string{"report", "--json", "extra"}, "unexpected_argument"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, db, clock, "", tt.args...)
			if code != 2 {
				t.Fatalf("exit = %d, want 2\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
			}
			if stdout != "" {
				t.Fatalf("stdout = %s", stdout)
			}
			var doc errorDoc
			if err := json.Unmarshal([]byte(stderr), &doc); err != nil {
				t.Fatalf("stderr is not exactly one JSON document: %v\n%s", err, stderr)
			}
			if doc.SchemaVersion != ProtocolVersion || doc.Error.Code != tt.code {
				t.Fatalf("error doc = %+v, want code %q", doc, tt.code)
			}
			if doc.Error.Message == "" {
				t.Fatal("error message is empty")
			}
		})
	}
}

func TestInteractiveInput(t *testing.T) {
	t.Run("lowercase answer", func(t *testing.T) {
		db := newDB(t)
		clock := &stepClock{base: goldenBase}
		out := mustRun(t, db, clock, "b\n", "practice", "--question", "q1")
		if !strings.Contains(out, "Result: CORRECT") {
			t.Fatalf("output:\n%s", out)
		}
		if !strings.Contains(out, "First attempt on this question") {
			t.Fatalf("first-attempt note missing:\n%s", out)
		}
	})
	t.Run("invalid then skip", func(t *testing.T) {
		db := newDB(t)
		clock := &stepClock{base: goldenBase}
		out := mustRun(t, db, clock, "X\nskip\n", "practice", "--question", "q1")
		if !strings.Contains(out, "Enter A, B, C, D, skip, or quit.") {
			t.Fatalf("re-prompt missing:\n%s", out)
		}
		if !strings.Contains(out, "Result: SKIPPED") {
			t.Fatalf("skip result missing:\n%s", out)
		}
	})
	t.Run("quit records nothing", func(t *testing.T) {
		db := newDB(t)
		clock := &stepClock{base: goldenBase}
		out := mustRun(t, db, clock, "quit\n", "practice", "--question", "q1")
		if !strings.Contains(out, "No attempt recorded.") {
			t.Fatalf("quit message missing:\n%s", out)
		}
		report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
		if report.Counts.Attempts != 0 {
			t.Fatalf("quit stored %d attempts", report.Counts.Attempts)
		}
	})
}

func TestInvalidInputThenEOF(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	out := mustRun(t, db, clock, "X", "practice", "--question", "q1")
	if got := strings.Count(out, "Your answer ("); got != 1 {
		t.Fatalf("prompted %d times before EOF, want 1:\n%s", got, out)
	}
	if !strings.Contains(out, "Enter A, B, C, D, skip, or quit.") {
		t.Fatalf("re-prompt missing:\n%s", out)
	}
	if !strings.Contains(out, "No attempt recorded.") {
		t.Fatalf("end-of-input message missing:\n%s", out)
	}
	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	if report.Counts.Attempts != 0 {
		t.Fatalf("EOF after invalid input stored %d attempts", report.Counts.Attempts)
	}
}

func TestCLIFailurePaths(t *testing.T) {
	newOptions := func(stdout, stderr *bytes.Buffer) Options {
		return Options{
			Stdout:     stdout,
			Stderr:     stderr,
			Now:        (&stepClock{base: goldenBase}).Now,
			NewSession: func() string { return "sess-test" },
			Getenv:     func(string) string { return "" },
		}
	}

	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	badDB := filepath.Join(blocker, "db.sqlite")

	t.Run("storage failure is a JSON internal error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"report", "--db", badDB, "--json"}, newOptions(&stdout, &stderr))
		if code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		if stdout.Len() != 0 {
			t.Fatalf("stdout = %s", stdout.String())
		}
		var doc errorDoc
		if err := json.Unmarshal(stderr.Bytes(), &doc); err != nil {
			t.Fatalf("decode error doc: %v\n%s", err, stderr.String())
		}
		if doc.Error.Code != "internal_error" {
			t.Fatalf("error code = %q", doc.Error.Code)
		}
	})

	t.Run("storage failure is a human internal error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"report", "--db", badDB}, newOptions(&stdout, &stderr))
		if code != 1 || !strings.Contains(stderr.String(), "passquest: ") {
			t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
		}
	})

	t.Run("stdin read error", func(t *testing.T) {
		db := newDB(t)
		var stdout, stderr bytes.Buffer
		opt := newOptions(&stdout, &stderr)
		opt.Stdin = failingReader{err: errors.New("boom")}
		code := Run([]string{"practice", "--db", db, "--question", "q1"}, opt)
		if code != 1 || !strings.Contains(stderr.String(), "read answer: boom") {
			t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
		}
		report := decodeReport(t, mustRun(t, db, &stepClock{base: goldenBase}, "", "report", "--json"))
		if report.Counts.Attempts != 0 {
			t.Fatalf("read failure stored %d attempts", report.Counts.Attempts)
		}
	})

	t.Run("JSON encode error", func(t *testing.T) {
		var stderr bytes.Buffer
		code := Run([]string{"practice", "--db", newDB(t), "--json", "--question", "q1"}, Options{
			Stdout:     failWriter{},
			Stderr:     &stderr,
			Now:        (&stepClock{base: goldenBase}).Now,
			NewSession: func() string { return "sess-test" },
			Getenv:     func(string) string { return "" },
		})
		if code != 1 || !strings.Contains(stderr.String(), "encode JSON") {
			t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
		}
	})
}

func TestReportEmpty(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	doc := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	if doc.Counts.Attempts != 0 {
		t.Fatalf("counts = %+v", doc.Counts)
	}
	if len(doc.Questions) != 3 {
		t.Fatalf("questions = %d", len(doc.Questions))
	}
	for _, q := range doc.Questions {
		if q.FirstStatus != "" || q.LatestStatus != "" || q.Attempts != 0 {
			t.Fatalf("empty question row = %+v", q)
		}
	}
	if doc.Attempts == nil || len(doc.Attempts) != 0 {
		t.Fatalf("attempts = %v", doc.Attempts)
	}

	human := mustRun(t, db, clock, "", "report")
	if !strings.Contains(human, "(none recorded)") {
		t.Fatalf("empty human report lacks the no-attempts line:\n%s", human)
	}
}

func TestDefaultQuestionSelection(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}

	if doc := decodeAnswered(t, mustRun(t, db, clock, "", "practice", "--json")); doc.Question.ID != "q1" || doc.Question.Number != 1 {
		t.Fatalf("first default question = %s #%d", doc.Question.ID, doc.Question.Number)
	}
	mustRun(t, db, clock, "", "practice", "--json", "--answer", "B")

	if doc := decodeAnswered(t, mustRun(t, db, clock, "", "practice", "--json")); doc.Question.ID != "q2" || doc.Question.Number != 2 {
		t.Fatalf("second default question = %s #%d", doc.Question.ID, doc.Question.Number)
	}
	mustRun(t, db, clock, "", "practice", "--json", "--answer", "B")

	if doc := decodeAnswered(t, mustRun(t, db, clock, "", "practice", "--json")); doc.Question.ID != "q3" || doc.Question.Number != 3 {
		t.Fatalf("third default question = %s #%d", doc.Question.ID, doc.Question.Number)
	}
	mustRun(t, db, clock, "", "practice", "--json", "--skip")

	if doc := decodeAnswered(t, mustRun(t, db, clock, "", "practice", "--json")); doc.Question.ID != "q1" || doc.Question.Number != 1 {
		t.Fatalf("fallback default question = %s #%d", doc.Question.ID, doc.Question.Number)
	}

	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	if report.Counts.Attempts != 3 {
		t.Fatalf("default selection stored %d attempts", report.Counts.Attempts)
	}
}

func TestDatabasePath(t *testing.T) {
	tests := []struct {
		name string
		flag string
		env  string
		want string
	}{
		// portable-paths: allow databasePath compares these as strings and never resolves them
		{"flag wins", "/tmp/flag.sqlite", "/tmp/env.sqlite", "/tmp/flag.sqlite"},
		{"env when flag blank", "  ", "/tmp/env.sqlite", "/tmp/env.sqlite"},
		{"default when both blank", "", " ", filepath.Join(".passquest", "passquest.sqlite")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := databasePath(tt.flag, func(string) string { return tt.env })
			if got != tt.want {
				t.Fatalf("databasePath(%q, env %q) = %q, want %q", tt.flag, tt.env, got, tt.want)
			}
		})
	}
}

func TestReportKeepsHistoricalQuestionRows(t *testing.T) {
	db := newDB(t)
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	started := goldenBase
	_, err = st.Append(attempt.Attempt{
		SessionID:      "sess-historical",
		QuestionID:     "q9",
		ContentID:      "sample-0001",
		ContentVersion: "1",
		Status:         attempt.StatusCorrect,
		SelectedLabel:  "A",
		SelectedText:   "Historical option.",
		CorrectLabel:   "A",
		CorrectText:    "Historical option.",
		StartedAt:      started,
		AnsweredAt:     started.Add(time.Second),
		ElapsedMS:      1000,
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	report := decodeReport(t, mustRun(t, db, &stepClock{base: goldenBase}, "", "report", "--json"))
	if report.Counts.Attempts != 1 || report.Counts.Correct != 1 {
		t.Fatalf("counts = %+v", report.Counts)
	}
	if len(report.Questions) != 4 {
		t.Fatalf("question rows = %d, want 4", len(report.Questions))
	}
	historical := report.Questions[3]
	if historical.ID != "q9" || historical.Attempts != 1 || historical.FirstStatus != "correct" || historical.LatestStatus != "correct" {
		t.Fatalf("historical row = %+v", historical)
	}
}

func TestReportScopesQuestionRowsByContent(t *testing.T) {
	db := newDB(t)
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	started := goldenBase
	records := []attempt.Attempt{
		{
			SessionID:      "sess-sample",
			QuestionID:     "q1",
			ContentID:      "sample-0001",
			ContentVersion: "1",
			Status:         attempt.StatusCorrect,
			SelectedLabel:  "B",
			SelectedText:   "At 4 p.m.",
			CorrectLabel:   "B",
			CorrectText:    "At 4 p.m.",
			StartedAt:      started,
			AnsweredAt:     started.Add(time.Second),
			ElapsedMS:      1000,
		},
		{
			SessionID:      "sess-other",
			QuestionID:     "q1",
			ContentID:      "sample-9999",
			ContentVersion: "1",
			Status:         attempt.StatusIncorrect,
			SelectedLabel:  "A",
			SelectedText:   "At 7 p.m.",
			CorrectLabel:   "B",
			CorrectText:    "At 4 p.m.",
			StartedAt:      started,
			AnsweredAt:     started.Add(time.Second),
			ElapsedMS:      1000,
		},
	}
	for _, record := range records {
		if _, err := st.Append(record); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	clock := &stepClock{base: goldenBase}
	report := decodeReport(t, mustRun(t, db, clock, "", "report", "--json"))
	want := countsView{
		Attempts: 2, Correct: 1, Incorrect: 1,
		FirstAttempts: 2, FirstCorrect: 1, FirstIncorrect: 1,
		IndependentAttempts: 2, IndependentCorrect: 1, IndependentIncorrect: 1,
	}
	if report.Counts != want {
		t.Fatalf("counts = %+v, want %+v", report.Counts, want)
	}
	if len(report.Questions) != 4 {
		t.Fatalf("question rows = %d, want 4", len(report.Questions))
	}
	rows := make(map[string]questionReport, len(report.Questions))
	for _, q := range report.Questions {
		rows[q.ContentID+"/"+q.ID] = q
	}
	first := rows["sample-0001/q1"]
	if first.Attempts != 1 || first.FirstStatus != "correct" || first.Repeated != 0 {
		t.Fatalf("sample-0001 q1 row = %+v", first)
	}
	other := rows["sample-9999/q1"]
	if other.Attempts != 1 || other.FirstStatus != "incorrect" || other.Repeated != 0 {
		t.Fatalf("sample-9999 q1 row = %+v", other)
	}

	human := mustRun(t, db, clock, "", "report")
	for _, want := range []string{
		"  q1  attempts 1  first correct",
		"  sample-9999/q1  attempts 1  first incorrect",
		"  #2  sample-9999/q1  incorrect",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("human report lacks %q:\n%s", want, human)
		}
	}
}

func TestVersionAndHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opt := Options{Stdout: &stdout, Stderr: &stderr, Now: time.Now, Getenv: func(string) string { return "" }}
	if code := Run([]string{"version"}, opt); code != 0 {
		t.Fatalf("version exit = %d", code)
	}
	if !strings.Contains(stdout.String(), "passquest 0.1.0-dev") || !strings.Contains(stdout.String(), "sample-0001") {
		t.Fatalf("version output = %s", stdout.String())
	}

	stdout.Reset()
	if code := Run([]string{"help"}, opt); code != 0 || !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("help = %d, %s", code, stdout.String())
	}
	stderr.Reset()
	if code := Run(nil, opt); code != 2 || !strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("no args = %d, %s", code, stderr.String())
	}
}

func TestProcessReopenSeesStoredEvidence(t *testing.T) {
	db := newDB(t)
	runChild := func(args ...string) (string, string) {
		t.Helper()
		cmd := exec.Command(os.Args[0], args...)
		cmd.Env = append(os.Environ(), "PASSQUEST_TEST_RUN_MAIN=1")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("child %v: %v\nstderr:\n%s", args, err, stderr.String())
		}
		return stdout.String(), stderr.String()
	}

	first, _ := runChild("practice", "--db", db, "--question", "q1", "--answer", "C", "--json")
	firstDoc := decodeAnswered(t, first)
	if firstDoc.Attempt.Status != "incorrect" {
		t.Fatalf("child status = %q", firstDoc.Attempt.Status)
	}

	second, _ := runChild("report", "--db", db, "--json")
	report := decodeReport(t, second)
	if report.Counts.Attempts != 1 || report.Counts.Incorrect != 1 {
		t.Fatalf("reopened report counts = %+v", report.Counts)
	}
	if len(report.Attempts) != 1 || report.Attempts[0].ID != firstDoc.Attempt.ID {
		t.Fatalf("reopened attempts = %+v", report.Attempts)
	}
	if report.Attempts[0].SessionID != firstDoc.Attempt.SessionID {
		t.Fatalf("session changed across processes: %q vs %q", report.Attempts[0].SessionID, firstDoc.Attempt.SessionID)
	}
	if _, err := os.Stat(db); err != nil {
		t.Fatalf("database file: %v", err)
	}
}

func TestGoldenAwaitingJSON(t *testing.T) {
	db := newDB(t)
	out := mustRun(t, db, &stepClock{base: goldenBase}, "", "practice", "--json", "--question", "q1")
	checkGolden(t, filepath.Join("testdata", "awaiting.golden.json"), []byte(out))
}

func TestGoldenReportJSON(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	seedFourAttempts(t, db, clock)
	out := mustRun(t, db, clock, "", "report", "--json")
	checkGolden(t, filepath.Join("testdata", "report.golden.json"), []byte(out))
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func checkGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create golden dir: %v", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with -update-golden): %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}
