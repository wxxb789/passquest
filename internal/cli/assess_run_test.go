package cli

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAssessRunHidesAnnotationsAndReportsThem(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	seedAssessmentSource(t, db, 20, 1)
	answers := strings.Repeat("B\n", 20)
	code, out, errOut := runCLI(t, db, clock, answers, "assess", "start", "--size", "20", "--minutes", "60", "--json")
	if code != 0 {
		t.Fatalf("start = %d, %s", code, errOut)
	}
	if strings.Contains(out, "Reasoning for") || strings.Contains(out, "shuts") || strings.Contains(out, "closes at 4 p.m.") {
		t.Fatalf("run transcript exposed annotations:\n%s", out)
	}
	runDoc := decodeRunResult(t, out)
	if runDoc.State != "completed" || len(runDoc.Items) != 20 {
		t.Fatalf("run = %+v", runDoc)
	}
	reportRaw := mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runDoc.Run.ID), "--json")
	report := decodeAssessReport(t, reportRaw)
	if report.Counts.Total != 20 || report.Accuracy.Correct != 20 || report.Counts.Scored != 20 {
		t.Fatalf("report counts = %+v accuracy=%+v", report.Counts, report.Accuracy)
	}
	if len(report.Items) != 20 {
		t.Fatalf("report items = %d", len(report.Items))
	}
	first := report.Items[0]
	if !first.Annotations.Available || !strings.Contains(first.Annotations.Reasoning, "Reasoning for") {
		t.Fatalf("annotations = %+v", first.Annotations)
	}
	if len(first.Annotations.Glossary) != 1 || first.Annotations.Glossary[0].Meaning != "shuts" {
		t.Fatalf("glossary = %+v", first.Annotations.Glossary)
	}
	if !containsExact(report.Unknowns, "pass threshold: unknown") {
		t.Fatalf("unknowns = %+v", report.Unknowns)
	}
	if strings.Contains(strings.ToLower(reportRaw), `"verdict"`) || strings.Contains(strings.ToLower(reportRaw), `"pass":`) {
		t.Fatalf("report invented a verdict:\n%s", reportRaw)
	}
	human := mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runDoc.Run.ID))
	if !strings.Contains(human, "pass threshold: unknown") || !strings.Contains(human, "Glossary: closes — shuts") {
		t.Fatalf("human report missing unknowns or annotations:\n%s", human)
	}
	if strings.Contains(human, "Result: PASS") || strings.Contains(human, "Result: FAIL") {
		t.Fatalf("human report printed a verdict:\n%s", human)
	}
}

func TestAssessRefusesCoachingWhileActive(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	seedAssessmentSource(t, db, 20, 1)
	code, _, errOut := runCLI(t, db, clock, "B\n", "assess", "start", "--size", "20", "--minutes", "60", "--json")
	if code != 4 || !strings.Contains(errOut, "run_interrupted") {
		t.Fatalf("interrupted start = %d, %s", code, errOut)
	}
	runID := activeAssessmentID(t, db)
	for _, args := range [][]string{
		{"tutor", "history", "--json"},
		{"skill", "show", "vocabulary-in-context", "--json"},
		{"find", "--json"},
	} {
		code, _, errOut = runCLI(t, db, clock, "", args...)
		if code != 2 || !strings.Contains(errOut, `"code": "assessment_active"`) || !strings.Contains(errOut, fmt.Sprintf("%d", runID)) {
			t.Fatalf("%v = %d, %s", args, code, errOut)
		}
	}
	mustRun(t, db, clock, "", "assess", "abandon", fmt.Sprintf("%d", runID), "--json")
	code, out, errOut := runCLI(t, db, clock, "", "find", "--json")
	if code != 0 || strings.Contains(errOut, "assessment_active") {
		t.Fatalf("coaching after abandon = %d, %s, %s", code, out, errOut)
	}
}

type onceAdvanceReader struct {
	mu      sync.Mutex
	data    []byte
	once    sync.Once
	advance func()
}

func (r *onceAdvanceReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	r.once.Do(r.advance)
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func TestAssessForcedItemTimeout(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	seedAssessmentSource(t, db, 20, 1)
	now := goldenBase
	reader := &onceAdvanceReader{data: []byte("B\nB\nB\nB\n"), advance: func() { now = now.Add(40 * time.Second) }}
	var stdout, stderr bytes.Buffer
	opt := Options{
		Stdin:      reader,
		Stdout:     &stdout,
		Stderr:     &stderr,
		Now:        func() time.Time { return now },
		NewSession: func() string { return "sess-test" },
		Getenv:     func(string) string { return "" },
	}
	code := Run([]string{"assess", "start", "--size", "4", "--minutes", "2", "--json", "--db", db}, opt)
	if code != 0 {
		t.Fatalf("timeout run = %d, %s", code, stderr.String())
	}
	runDoc := decodeRunResult(t, stdout.String())
	report := decodeAssessReport(t, mustRun(t, db, &stepClock{base: goldenBase.Add(time.Hour)}, "", "assess", "report", fmt.Sprintf("%d", runDoc.Run.ID), "--json"))
	if report.Counts.TimedOut != 1 || report.Counts.Total != 4 || report.Accuracy.Correct != 3 {
		t.Fatalf("timeout report = %+v accuracy=%+v", report.Counts, report.Accuracy)
	}
}

func TestAssessRunWindowElapseRecordsOpenItemTimeout(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	seedAssessmentSource(t, db, 20, 1)
	now := goldenBase
	reader := &lineAdvanceReader{
		lines:     []string{"B\n", "B\n", "B\n", "B\n"},
		advanceAt: 3,
		advance:   func() { now = goldenBase.Add(2 * time.Minute) },
	}
	var stdout, stderr bytes.Buffer
	opt := Options{
		Stdin:      reader,
		Stdout:     &stdout,
		Stderr:     &stderr,
		Now:        func() time.Time { return now },
		NewSession: func() string { return "sess-test" },
		Getenv:     func(string) string { return "" },
	}
	code := Run([]string{"assess", "start", "--size", "4", "--minutes", "1", "--json", "--db", db}, opt)
	if code != 2 || !strings.Contains(stderr.String(), `"code": "run_abandoned"`) || !strings.Contains(stderr.String(), "assess report") {
		t.Fatalf("expired run = %d, stdout: %s, stderr: %s", code, stdout.String(), stderr.String())
	}
	report := decodeAssessReport(t, mustRun(t, db, &stepClock{base: goldenBase.Add(time.Hour)}, "", "assess", "report", "1", "--json"))
	if report.Run.Status != "abandoned" || report.Run.AbandonedReason != "window_elapsed" {
		t.Fatalf("abandoned run = %+v", report.Run)
	}
	answered, timedOut, presented := 0, 0, 0
	for _, item := range report.Items {
		switch item.State {
		case "answered":
			answered++
		case "timed_out":
			timedOut++
		case "presented":
			presented++
		case "pending", "unavailable":
		default:
			t.Fatalf("item %d has unknown state %q", item.Position, item.State)
		}
	}
	if presented != 0 || answered != 3 || timedOut != 1 {
		t.Fatalf("item states: answered=%d timed_out=%d presented=%d, items=%+v", answered, timedOut, presented, report.Items)
	}
	if report.Counts.Presented != 4 || report.Counts.TimedOut != 1 || report.Counts.Finalized != 4 {
		t.Fatalf("counts = %+v", report.Counts)
	}
	if report.Accuracy.Correct != 3 || report.Accuracy.Total != 4 || report.Accuracy.Ratio >= 1 {
		t.Fatalf("accuracy = %+v", report.Accuracy)
	}
}

type lineAdvanceReader struct {
	lines     []string
	advanceAt int
	advance   func()
	index     int
}

func (r *lineAdvanceReader) Read(p []byte) (int, error) {
	if r.index >= len(r.lines) {
		return 0, io.EOF
	}
	if r.index == r.advanceAt {
		r.advance()
	}
	n := copy(p, r.lines[r.index])
	r.index++
	return n, nil
}

func TestAssessResumeMarksResumedPortion(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	seedAssessmentSource(t, db, 20, 1)
	code, _, errOut := runCLI(t, db, clock, strings.Repeat("B\n", 5), "assess", "start", "--size", "20", "--minutes", "60", "--json")
	if code != 4 {
		t.Fatalf("first start = %d, %s", code, errOut)
	}
	runID := activeAssessmentID(t, db)
	out := mustRun(t, db, clock, strings.Repeat("B\n", 15), "assess", "resume", fmt.Sprintf("%d", runID), "--json")
	runDoc := decodeRunResult(t, out)
	if runDoc.Run.ResumeCount != 1 || runDoc.State != "completed" {
		t.Fatalf("resumed run = %+v", runDoc.Run)
	}
	report := decodeAssessReport(t, mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runID), "--json"))
	if report.Run.ResumeCount != 1 || report.Counts.AnsweredAfterResume != 15 {
		t.Fatalf("resume report = %+v counts=%+v", report.Run, report.Counts)
	}
	after := 0
	for _, item := range report.Items {
		if item.AfterResume {
			after++
		}
	}
	if after != 15 {
		t.Fatalf("items after resume = %d", after)
	}
}

func TestAssessContaminationFromShowDuringRun(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := seedAssessmentSource(t, db, 20, 1)
	var once sync.Once
	showDone := false
	reader := &callbackReader{
		data: []byte(strings.Repeat("B\n", 20)),
		onRead: func() {
			once.Do(func() {
				var stdout, stderr bytes.Buffer
				code := Run([]string{"show", sourceID, "--page", "5", "--json", "--db", db}, Options{
					Stdin:           strings.NewReader(""),
					Stdout:          &stdout,
					Stderr:          &stderr,
					Now:             clock.Now,
					PresentationNow: clock.Now,
					NewSession:      func() string { return "show-test" },
					Getenv:          func(string) string { return "" },
				})
				if code != 0 {
					t.Fatalf("show during run = %d, %s", code, stderr.String())
				}
				showDone = true
			})
		},
	}
	var stdout, stderr bytes.Buffer
	opt := Options{
		Stdin:      reader,
		Stdout:     &stdout,
		Stderr:     &stderr,
		Now:        clock.Now,
		NewSession: func() string { return "sess-test" },
		Getenv:     func(string) string { return "" },
	}
	code := Run([]string{"assess", "start", "--size", "20", "--minutes", "60", "--json", "--db", db}, opt)
	if code != 0 || !showDone {
		t.Fatalf("contaminated run = %d, showDone=%t, %s", code, showDone, stderr.String())
	}
	runDoc := decodeRunResult(t, stdout.String())
	report := decodeAssessReport(t, mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runDoc.Run.ID), "--json"))
	if report.Counts.Contaminated != 1 || report.Counts.Scored != 19 || report.Counts.Total != 19 {
		t.Fatalf("contamination counts = %+v", report.Counts)
	}
	if len(report.Contamination.ExcludedItems) != 1 {
		t.Fatalf("excluded = %+v", report.Contamination.ExcludedItems)
	}
	excluded := report.Contamination.ExcludedItems[0]
	if excluded.QuestionID != "p05-q1" || !containsExact(excluded.Reasons, "prior_exposure") {
		t.Fatalf("excluded item = %+v", excluded)
	}
}

type callbackReader struct {
	mu     sync.Mutex
	data   []byte
	onRead func()
	once   sync.Once
}

func (r *callbackReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	r.once.Do(r.onRead)
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}
