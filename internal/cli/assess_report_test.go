package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/wxxb789/passquest/internal/assess"
)

func TestAssessReportWithholdsAnnotationsUntilRunFinishes(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	seedAssessmentSource(t, db, 20, 1)
	code, _, errOut := runCLI(t, db, clock, "B\n", "assess", "start", "--size", "20", "--minutes", "60", "--json")
	if code != 4 {
		t.Fatalf("interrupted start = %d, %s", code, errOut)
	}
	runID := activeAssessmentID(t, db)
	raw := mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runID), "--json")
	report := decodeAssessReport(t, raw)
	if report.Run.Status != assess.StatusActive || len(report.Items) != 20 {
		t.Fatalf("active report = %+v items=%d", report.Run, len(report.Items))
	}
	for _, item := range report.Items {
		if !item.Annotations.Withheld {
			t.Fatalf("active run exposed annotations for %s: %+v", item.QuestionID, item.Annotations)
		}
		if item.Annotations.Available || item.Annotations.Evidence != "" || item.Annotations.Reasoning != "" ||
			len(item.Annotations.OptionReasoning) != 0 || len(item.Annotations.Glossary) != 0 {
			t.Fatalf("active run item %s carries annotation content: %+v", item.QuestionID, item.Annotations)
		}
	}
	for _, key := range []string{"Reasoning for", "shuts", `"evidence"`, `"reasoning"`, `"option_reasoning"`, `"glossary"`} {
		if strings.Contains(raw, key) {
			t.Fatalf("active report leaked %q:\n%s", key, raw)
		}
	}
	human := mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runID))
	if !strings.Contains(human, fmt.Sprintf("Annotations: withheld until the run is finished (run %d is active)", runID)) {
		t.Fatalf("active human report does not state the withholding:\n%s", human)
	}
	for _, leaked := range []string{"Evidence:", "Reasoning:", "Option A:", "Glossary:"} {
		if strings.Contains(human, leaked) {
			t.Fatalf("active human report leaked %q:\n%s", leaked, human)
		}
	}

	mustRun(t, db, clock, strings.Repeat("B\n", 19), "assess", "resume", fmt.Sprintf("%d", runID), "--json")
	finished := decodeAssessReport(t, mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runID), "--json"))
	if finished.Run.Status != assess.StatusCompleted || len(finished.Items) != 20 {
		t.Fatalf("finished report = %+v items=%d", finished.Run, len(finished.Items))
	}
	for _, item := range finished.Items {
		if item.Annotations.Withheld || !item.Annotations.Available || !strings.Contains(item.Annotations.Reasoning, "Reasoning for") {
			t.Fatalf("finished run item %s did not expose its annotations: %+v", item.QuestionID, item.Annotations)
		}
	}
	human = mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runID))
	if strings.Contains(human, "withheld until the run is finished") || !strings.Contains(human, "Reasoning: Reasoning for page 1 question 1.") {
		t.Fatalf("finished human report did not expose annotations:\n%s", human)
	}
}

func TestAssessmentReportPrintsMissingOptionExplanation(t *testing.T) {
	t.Parallel()
	item := assess.ItemReport{
		Position:   0,
		ContentID:  "content-1",
		QuestionID: "q1",
		State:      assess.StateAnswered,
		Scored:     true,
		Annotations: assess.Annotations{
			Evidence:               "closes at 4 p.m.",
			Reasoning:              "The notice states the time.",
			OptionReasoning:        map[string]string{"B": "The key is 4 p.m."},
			OptionReasoningMissing: []string{"A", "C"},
		},
	}
	view := reportItemViewOf(item)
	if got := strings.Join(view.Annotations.OptionReasoningMissing, ","); got != "A,C" {
		t.Fatalf("option_reasoning_missing = %q, want A,C", got)
	}
	if len(view.Annotations.OptionReasoning) != 1 || view.Annotations.OptionReasoning["B"] == "" {
		t.Fatalf("option_reasoning = %v, want only B", view.Annotations.OptionReasoning)
	}
	var out bytes.Buffer
	writeAssessmentReport(&out, assessReportDoc{Items: []assessReportItemView{view}}, "test.sqlite")
	text := out.String()
	for _, label := range []string{"A", "C"} {
		if !strings.Contains(text, "    Option "+label+": "+missingOptionExplanation+"\n") {
			t.Fatalf("report does not print the missing explanation for %s:\n%s", label, text)
		}
	}
	if !strings.Contains(text, "    Option B: The key is 4 p.m.\n") {
		t.Fatalf("report does not print the authored explanation:\n%s", text)
	}
}

func TestAssessAbandonedRunWithholdsUnfinalizedAnnotations(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	seedAssessmentSource(t, db, 20, 1)
	code, _, errOut := runCLI(t, db, clock, "B\nB\nB\n", "assess", "start", "--size", "20", "--minutes", "60", "--json")
	if code != 4 {
		t.Fatalf("interrupted start = %d, %s", code, errOut)
	}
	runID := activeAssessmentID(t, db)
	mustRun(t, db, clock, "", "assess", "abandon", fmt.Sprintf("%d", runID), "--json")
	report := decodeAssessReport(t, mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runID), "--json"))
	if report.Run.Status != assess.StatusAbandoned {
		t.Fatalf("run status = %q", report.Run.Status)
	}
	pending, presented, answered := 0, 0, 0
	for _, item := range report.Items {
		switch item.State {
		case assess.StatePending:
			pending++
		case assess.StatePresented:
			presented++
		case assess.StateAnswered:
			answered++
		}
		if item.State == assess.StateAnswered {
			if item.Annotations.Withheld || !item.Annotations.Available {
				t.Fatalf("answered item %s = %+v", item.QuestionID, item.Annotations)
			}
			continue
		}
		if !item.Annotations.Withheld || item.Annotations.Available || item.Annotations.Reasoning != "" ||
			item.Annotations.Evidence != "" || len(item.Annotations.Glossary) != 0 || len(item.Annotations.OptionReasoning) != 0 {
			t.Fatalf("unfinalized item %s (%s) = %+v", item.QuestionID, item.State, item.Annotations)
		}
	}
	if pending != 16 || presented != 1 || answered != 3 {
		t.Fatalf("item states = pending %d, presented %d, answered %d", pending, presented, answered)
	}
	human := mustRun(t, db, clock, "", "assess", "report", fmt.Sprintf("%d", runID))
	if !strings.Contains(human, "Annotations: withheld (item was never finalized)") || strings.Contains(human, "withheld until the run is finished") {
		t.Fatalf("abandoned human report = \n%s", human)
	}
}
