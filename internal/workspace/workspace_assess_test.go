package workspace

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/studio"
)

func TestWorkspaceAssessmentStepFlow(t *testing.T) {
	t.Parallel()
	// Seed before the workspace opens: the seed is copied to a fresh path, and
	// opening an already migrated database is a no-op.
	f := newWSFixture(t, func(o *Options) { seedAssessmentSource(t, o.DBPath, 22, 1) })
	pool, err := f.w.AssessmentPool()
	mustNoErr(t, err)
	if !pool.FloorMet || pool.QualifyingItems != 22 {
		t.Fatalf("pool = %+v", pool)
	}
	state, err := f.w.StartAssessment(2, 2)
	mustNoErr(t, err)
	if state.State != studio.AssessmentItemState || state.Item == nil || state.Item.Position != 0 || state.Run.PerItemTimeoutMS != 60000 {
		t.Fatalf("start = %+v", state)
	}
	runID := state.Run.ID
	keys := jsonKeys(t, state.Navigator[0])
	if len(keys) != 3 || keys["state"] != "presented" || keys["current"] != true {
		t.Fatalf("navigator item = %v", keys)
	}
	if _, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample}); studioCode(err) != "assessment_active" {
		t.Fatalf("practice during run = %v", err)
	}
	if _, err := f.w.StartAssessment(2, 2); studioCode(err) != "run_active" {
		t.Fatalf("second start = %v", err)
	}
	refreshed, err := f.w.PresentAssessmentItem(runID)
	mustNoErr(t, err)
	if refreshed.Item.PresentedAt != state.Item.PresentedAt || refreshed.Item.DeadlineAt != state.Item.DeadlineAt {
		t.Fatal("re-presenting moved the item clock")
	}
	answer, err := f.w.AnswerAssessmentItem(runID, 0, "A")
	mustNoErr(t, err)
	if answer.Outcome != assess.StateAnswered || answer.State.Navigator[0].State != assess.StateAnswered {
		t.Fatalf("answer = %+v", answer)
	}
	if raw, _ := json.Marshal(answer); strings.Contains(string(raw), "correct") {
		t.Fatalf("answer reveals correctness: %s", raw)
	}
	next, err := f.w.PresentAssessmentItem(runID)
	mustNoErr(t, err)
	if next.Item == nil || next.Item.Position != 1 {
		t.Fatalf("next item = %+v", next)
	}
	f.clock.Advance(90 * time.Second)
	late, err := f.w.AnswerAssessmentItem(runID, 1, "B")
	mustNoErr(t, err)
	if late.Outcome != assess.StateTimedOut {
		t.Fatalf("late answer = %+v", late)
	}
	done, err := f.w.PresentAssessmentItem(runID)
	mustNoErr(t, err)
	if done.State != studio.AssessmentCompletedState || done.Item != nil {
		t.Fatalf("after last item = %+v", done)
	}
	report, err := f.w.AssessmentReport(runID)
	mustNoErr(t, err)
	if report.PassThreshold != "unknown" || report.Counts.TimedOut != 1 || report.Counts.Finalized != 2 || len(report.Items) != 2 {
		t.Fatalf("report = %+v", report)
	}
	// After the run each item carries its question, the learner's choice and the verified key.
	for _, item := range report.Items {
		q := item.Question
		if q == nil || q.Stem == "" || len(q.Options) == 0 || q.Key == "" {
			t.Fatalf("report item %d question = %+v", item.Position, q)
		}
	}
	if got := report.Items[0].Question.Selected; got != "A" {
		t.Fatalf("item 1 selected = %q, want A", got)
	}
	if got := report.Items[1].Question.Selected; got != "" {
		t.Fatalf("timed-out item 2 selected = %q, want none", got)
	}

	second, err := f.w.StartAssessment(2, 2)
	mustNoErr(t, err)
	secondID := second.Run.ID
	f.reopen(t)
	current, err := f.w.AssessmentState()
	mustNoErr(t, err)
	if current.Item == nil || current.Item.PresentedAt != second.Item.PresentedAt || current.Run.ID != secondID {
		t.Fatalf("state after reopen = %+v", current)
	}
	resumed, err := f.w.ResumeAssessment(secondID)
	mustNoErr(t, err)
	if resumed.Run.ResumeCount != 1 || resumed.Item == nil || resumed.Item.PresentedAt != second.Item.PresentedAt {
		t.Fatalf("resume = %+v", resumed)
	}
	abandoned, err := f.w.AbandonAssessment(secondID)
	mustNoErr(t, err)
	if abandoned.State != studio.AssessmentAbandonedState || abandoned.Run.AbandonedReason != "learner_abandoned" {
		t.Fatalf("abandon = %+v", abandoned)
	}
	runs, err := f.w.AssessmentRuns()
	mustNoErr(t, err)
	if len(runs) != 2 || runs[0].ID != secondID {
		t.Fatalf("runs = %+v", runs)
	}
	if _, err := f.w.AssessmentReport(999); studioCode(err) != "unknown_run" {
		t.Fatalf("unknown run = %v", err)
	}
	if none, err := f.w.AssessmentState(); err != nil || none.State != studio.AssessmentNoneState {
		t.Fatalf("state after abandon = %+v %v", none, err)
	}
	today, err := f.w.Today()
	mustNoErr(t, err)
	if today.LastRun == nil || today.LastRun.ID != secondID || today.ActiveRun != nil {
		t.Fatalf("today runs = %+v / %+v", today.LastRun, today.ActiveRun)
	}
}

func TestWorkspaceAssessmentWindowElapseAbandons(t *testing.T) {
	t.Parallel()
	// Seed before the workspace opens: the seed is copied to a fresh path, and
	// opening an already migrated database is a no-op.
	f := newWSFixture(t, func(o *Options) { seedAssessmentSource(t, o.DBPath, 22, 1) })
	state, err := f.w.StartAssessment(2, 1)
	mustNoErr(t, err)
	f.clock.Advance(2 * time.Minute)
	// As in the CLI loop (abandonElapsedAssessment), an answer after the run
	// window records the item on screen as timed out at its deadline and only
	// then abandons the run, so the item stays in the timeout count.
	late, err := f.w.AnswerAssessmentItem(state.Run.ID, 0, "A")
	mustNoErr(t, err)
	if late.Outcome != assess.StatusAbandoned || late.State.State != studio.AssessmentAbandonedState || late.State.Message == "" {
		t.Fatalf("late answer after window = %+v", late)
	}
	if again, err := f.w.AnswerAssessmentItem(state.Run.ID, 0, "A"); studioCode(err) != "run_state" {
		t.Fatalf("answer on an abandoned run = %+v %v", again, err)
	}
	report, err := f.w.AssessmentReport(state.Run.ID)
	mustNoErr(t, err)
	if report.Run.Status != assess.StatusAbandoned || report.Run.AbandonedReason != "window_elapsed" {
		t.Fatalf("expired run = %+v", report.Run)
	}
	if report.Counts.TimedOut != 1 || report.Counts.Finalized != 1 {
		t.Fatalf("on-screen item not timed out: %+v", report.Counts)
	}

	// Any other call that finds the window elapsed (here the frontend reading
	// state) applies the same rule before the store-level reconcile.
	other, err := f.w.StartAssessment(2, 1)
	mustNoErr(t, err)
	f.clock.Advance(2 * time.Minute)
	read, err := f.w.AssessmentState()
	mustNoErr(t, err)
	if read.State != studio.AssessmentNoneState {
		t.Fatalf("state after window = %+v", read)
	}
	otherReport, err := f.w.AssessmentReport(other.Run.ID)
	mustNoErr(t, err)
	if otherReport.Counts.TimedOut != 1 || otherReport.Run.AbandonedReason != "window_elapsed" {
		t.Fatalf("second run = %+v / %+v", otherReport.Counts, otherReport.Run)
	}
	if _, err := f.w.ResumeAssessment(state.Run.ID); studioCode(err) != "run_abandoned" {
		t.Fatalf("resume abandoned = %v", err)
	}
}

// TestWorkspaceAssessmentReportWithholdsQuestions: the report's question and
// verified key appear only after the run, and only for items the learner was
// shown and that reached an outcome. An active run shows none; an abandoned
// run shows the answered item and not the pending one.
func TestWorkspaceAssessmentReportWithholdsQuestions(t *testing.T) {
	t.Parallel()
	// Seed before the workspace opens: the seed is copied to a fresh path, and
	// opening an already migrated database is a no-op.
	f := newWSFixture(t, func(o *Options) { seedAssessmentSource(t, o.DBPath, 22, 1) })
	state, err := f.w.StartAssessment(3, 30)
	mustNoErr(t, err)
	runID := state.Run.ID
	_, err = f.w.AnswerAssessmentItem(runID, 0, "A")
	mustNoErr(t, err)
	active, err := f.w.AssessmentReport(runID)
	mustNoErr(t, err)
	if len(active.Items) != 3 {
		t.Fatalf("active report items = %+v", active.Items)
	}
	for _, item := range active.Items {
		if item.Question != nil || !item.Annotations.Withheld || item.Annotations.Available {
			t.Fatalf("active run item %d reveals its question: %+v / %+v", item.Position, item.Question, item.Annotations)
		}
	}
	_, err = f.w.AbandonAssessment(runID)
	mustNoErr(t, err)
	abandoned, err := f.w.AssessmentReport(runID)
	mustNoErr(t, err)
	if q := abandoned.Items[0].Question; q == nil || q.Key == "" || q.Selected != "A" {
		t.Fatalf("answered item after abandon = %+v", q)
	}
	for _, item := range abandoned.Items[1:] {
		if item.Question != nil || !item.Annotations.Withheld {
			t.Fatalf("pending item %d of an abandoned run reveals its question: %+v / %+v", item.Position, item.Question, item.Annotations)
		}
	}
}

// TestWorkspaceAssessmentReportHidesUnseenUnavailableItems: an item marked
// unavailable before it was presented stays unspent, so it can be selected as
// unseen again. When its content loads again at the same version, the report
// must still not show its question, key or annotations.
func TestWorkspaceAssessmentReportHidesUnseenUnavailableItems(t *testing.T) {
	t.Parallel()
	// Seed before the workspace opens: the seed is copied to a fresh path, and
	// opening an already migrated database is a no-op.
	f := newWSFixture(t, func(o *Options) { seedAssessmentSource(t, o.DBPath, 22, 1) })
	state, err := f.w.StartAssessment(2, 30)
	mustNoErr(t, err)
	runID := state.Run.ID
	// A transient load failure marks the pending item unavailable; the content
	// itself is unchanged, so its version matches again at report time.
	st, err := store.Open(f.db)
	mustNoErr(t, err)
	err = st.MarkAssessmentUnavailable(runID, 1, f.clock.Now(), "transient read failure")
	st.Close()
	mustNoErr(t, err)
	_, err = f.w.AnswerAssessmentItem(runID, 0, "A")
	mustNoErr(t, err)
	done, err := f.w.PresentAssessmentItem(runID)
	mustNoErr(t, err)
	if done.State != studio.AssessmentCompletedState {
		t.Fatalf("run after the last available item = %+v", done)
	}
	report, err := f.w.AssessmentReport(runID)
	mustNoErr(t, err)
	if len(report.Items) != 2 || report.Items[1].State != assess.StateUnavailable {
		t.Fatalf("report items = %+v", report.Items)
	}
	if report.Items[0].Question == nil {
		t.Fatal("the answered item lost its question")
	}
	unseen := report.Items[1]
	if unseen.Question != nil || unseen.Annotations.Available || unseen.Annotations.Evidence != "" || unseen.Annotations.Reasoning != "" {
		t.Fatalf("an unseen unavailable item reveals its question: %+v / %+v", unseen.Question, unseen.Annotations)
	}
}
