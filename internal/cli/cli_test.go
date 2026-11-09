package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
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
	for _, leaked := range []string{"Correct answer:", "Result:", "Why:", "Why your answer is wrong:", "Recorded attempt"} {
		if strings.Contains(out, leaked) {
			t.Errorf("pre-answer output contains %q", leaked)
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

	correct := decodeAnswered(t, mustRun(t, db, clock, "", "practice", "--json", "--question", "q1", "--answer", "B"))
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

	incorrect := decodeAnswered(t, mustRun(t, db, clock, "", "practice", "--json", "--question", "q2", "--answer", "A"))
	if incorrect.Attempt.Status != "incorrect" || incorrect.Attempt.Repeat {
		t.Fatalf("incorrect doc = %+v", incorrect)
	}
	if incorrect.Feedback.SelectedReasoning == "" {
		t.Fatal("incorrect feedback has no distractor explanation")
	}

	skipped := decodeAnswered(t, mustRun(t, db, clock, "", "practice", "--json", "--question", "q3", "--skip"))
	if skipped.Attempt.Status != "skipped" {
		t.Fatalf("skipped status = %q", skipped.Attempt.Status)
	}
	if skipped.Attempt.SelectedLabel != nil || skipped.Attempt.SelectedText != nil {
		t.Fatalf("skipped selection = %v/%v", skipped.Attempt.SelectedLabel, skipped.Attempt.SelectedText)
	}
	if skipped.Feedback.Exposure != attempt.ExposureFirstAttempt {
		t.Fatalf("skip exposure = %q", skipped.Feedback.Exposure)
	}

	repeated := decodeAnswered(t, mustRun(t, db, clock, "", "practice", "--json", "--question", "q1", "--answer", "B"))
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
	if len(report.Attempts) != 4 {
		t.Fatalf("attempt rows = %d", len(report.Attempts))
	}
	if report.Attempts[3].ID != 4 || !report.Attempts[3].Repeat {
		t.Fatalf("last attempt row = %+v", report.Attempts[3])
	}
	if len(report.Limitations) != 3 {
		t.Fatalf("limitations = %d", len(report.Limitations))
	}
	joined := strings.ToLower(strings.Join(report.Limitations, " "))
	for _, want := range []string{"not an exam score", "not independent evidence", "pass probability is computed"} {
		if !strings.Contains(joined, want) {
			t.Errorf("limitations do not state %q: %v", want, report.Limitations)
		}
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

func TestReportEmpty(t *testing.T) {
	doc := decodeReport(t, mustRun(t, newDB(t), &stepClock{base: goldenBase}, "", "report", "--json"))
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
	for _, args := range [][]string{
		{"practice", "--json", "--question", "q1", "--answer", "B"},
		{"practice", "--json", "--question", "q2", "--answer", "A"},
		{"practice", "--json", "--question", "q3", "--skip"},
		{"practice", "--json", "--question", "q1", "--answer", "B"},
	} {
		mustRun(t, db, clock, "", args...)
	}
	out := mustRun(t, db, clock, "", "report", "--json")
	checkGolden(t, filepath.Join("testdata", "report.golden.json"), []byte(out))
}

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
