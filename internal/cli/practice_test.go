package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
)

func TestAwaitingJSONHidesTheKey(t *testing.T) {
	t.Parallel()
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

func TestHumanPromptDoesNotRevealTheKey(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestInvalidInputRecordsNothing(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestDefaultQuestionSelection(t *testing.T) {
	t.Parallel()
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

func TestGoldenAwaitingJSON(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	out := mustRun(t, db, &stepClock{base: goldenBase}, "", "practice", "--json", "--question", "q1")
	checkGolden(t, filepath.Join("testdata", "awaiting.golden.json"), []byte(out))
}
