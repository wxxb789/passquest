package assess

import (
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
)

func unit(contentID string, questions int) Unit {
	qs := make([]exam.Question, 0, questions)
	for i := 0; i < questions; i++ {
		qs = append(qs, exam.Question{ID: contentID + "-q" + string(rune('a'+i))})
	}
	return Unit{ContentID: contentID, SourceID: "src", Page: 1, Version: "v1", Questions: qs}
}

func TestPoolSpendsWholeContentUnit(t *testing.T) {
	units := []Unit{unit("src-p001", 2), unit("src-p002", 1)}
	spends := []Spend{{ContentID: "src-p001", QuestionID: "src-p001-qa", Surface: SurfacePractice, At: time.Unix(1, 0)}}
	pool := BuildPool(units, spends)
	if pool.ItemCount != 1 || pool.UnitCount != 1 || pool.SpentUnits != 1 || pool.PracticeableItems != 3 {
		t.Fatalf("pool = %+v", pool)
	}
	if !pool.Units[0].Spent || len(pool.Units[0].SpentBy) != 1 || pool.Units[0].SpentBy[0] != SurfacePractice {
		t.Fatalf("spent unit = %+v", pool.Units[0])
	}
	if pool.Units[1].Spent || pool.Units[1].ContentID != "src-p002" {
		t.Fatalf("fresh unit = %+v", pool.Units[1])
	}
	selected := pool.Select(1)
	if len(selected) != 1 || selected[0].ContentID != "src-p002" {
		t.Fatalf("selected = %+v", selected)
	}
}

func TestSelectNeverSplitsAUnit(t *testing.T) {
	pool := BuildPool([]Unit{unit("a", 3), unit("b", 3)}, nil)
	selected := pool.Select(4)
	if len(selected) != 2 || SelectedItems(selected) != 6 {
		t.Fatalf("selected = %+v items=%d", selected, SelectedItems(selected))
	}
}

func TestPerItemTimeout(t *testing.T) {
	if got, ok := PerItemTimeout(60, 40); !ok || got != 90_000 {
		t.Fatalf("60/40 = %d,%t", got, ok)
	}
	if _, ok := PerItemTimeout(0, 10); ok {
		t.Fatal("zero minutes accepted")
	}
	if _, ok := PerItemTimeout(1, 100_000); ok {
		t.Fatal("sub-millisecond timeout accepted")
	}
}

func TestMedianAndP90(t *testing.T) {
	values := []int64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	if got := Median(values); got != 55 {
		t.Fatalf("median = %d", got)
	}
	if got := P90(values); got != 90 {
		t.Fatalf("p90 = %d", got)
	}
	if got := Median([]int64{7}); got != 7 {
		t.Fatalf("single median = %d", got)
	}
}

func baseRun() Run {
	start := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	return Run{
		ID: 1, StartedAt: start, DeadlineAt: start.Add(60 * time.Minute),
		WindowMinutes: 60, RequestedSize: 3, ActualSize: 3, PerItemTimeoutMS: 30_000,
		Annotations: AnnotationsNone, PoolFamily: "family", Status: StatusCompleted,
	}
}

func attemptRecord(id int64, status attempt.Status, elapsed int64, answered time.Time) attempt.Attempt {
	return attempt.Attempt{
		ID: id, ContentID: "src-p001", QuestionID: "q", Status: status,
		ElapsedMS: elapsed, Skills: []string{"not-except-polarity"}, AnsweredAt: answered,
	}
}

func TestReportMarksMissingAttemptUnavailable(t *testing.T) {
	run := baseRun()
	start := run.StartedAt
	items := []Item{{RunID: 1, Position: 0, ContentID: "src-p001", QuestionID: "q0", State: StateAnswered, PresentedAt: start.Add(time.Minute), DeadlineAt: start.Add(90 * time.Second), AnsweredAt: start.Add(20 * time.Second), AttemptID: 99}}
	report := BuildReport(run, items, nil, nil, nil, nil)
	if report.Counts.Unavailable != 1 || report.Counts.Scored != 0 {
		t.Fatalf("counts = %+v", report.Counts)
	}
	if report.Items[0].State != StateUnavailable {
		t.Fatalf("item state = %q", report.Items[0].State)
	}
}

func TestReportExcludesContaminatedItems(t *testing.T) {
	run := baseRun()
	start := run.StartedAt
	items := []Item{
		{RunID: 1, Position: 0, ContentID: "src-p001", QuestionID: "q0", State: StateAnswered, PresentedAt: start.Add(1 * time.Minute), DeadlineAt: start.Add(90 * time.Second), AnsweredAt: start.Add(20 * time.Second), AttemptID: 1},
		{RunID: 1, Position: 1, ContentID: "src-p001", QuestionID: "q1", State: StateAnswered, PresentedAt: start.Add(2 * time.Minute), DeadlineAt: start.Add(3 * time.Minute), AnsweredAt: start.Add(150 * time.Second), AttemptID: 2},
		{RunID: 1, Position: 2, ContentID: "src-p002", QuestionID: "q2", State: StateAnswered, PresentedAt: start.Add(3 * time.Minute), DeadlineAt: start.Add(4 * time.Minute), AnsweredAt: start.Add(210 * time.Second), AttemptID: 3},
	}
	attempts := []attempt.Attempt{
		attemptRecord(1, attempt.StatusCorrect, 10_000, start.Add(20*time.Second)),
		attemptRecord(2, attempt.StatusIncorrect, 20_000, start.Add(150*time.Second)),
		attemptRecord(3, attempt.StatusCorrect, 30_000, start.Add(210*time.Second)),
	}
	spends := []Spend{
		{ContentID: "src-p001", QuestionID: "old", Surface: SurfacePractice, At: start.Add(-time.Minute)},
		{ContentID: "src-p001", QuestionID: "hint", Surface: SurfaceShow, At: start.Add(2*time.Minute + 10*time.Second)},
		{ContentID: "src-p001", QuestionID: "q0", Surface: "attempt", At: start.Add(10 * time.Second), RunID: run.ID},
	}
	report := BuildReport(run, items, attempts, nil, spends, nil)
	if report.Counts.Scored != 1 || report.Counts.Contaminated != 2 || report.Counts.Total != 1 {
		t.Fatalf("counts = %+v", report.Counts)
	}
	if report.Accuracy.Correct != 1 || report.Accuracy.Total != 1 {
		t.Fatalf("accuracy = %+v", report.Accuracy)
	}
	if report.Contamination.PriorExposure != 2 || report.Contamination.HintsDuringRun != 1 {
		t.Fatalf("contamination = %+v", report.Contamination)
	}
	if len(report.Contamination.Excluded) != 2 {
		t.Fatalf("excluded = %+v", report.Contamination.Excluded)
	}
}

func TestReportCountsTimeoutAndMarksResume(t *testing.T) {
	run := baseRun()
	run.ResumeCount = 1
	start := run.StartedAt
	items := []Item{
		{RunID: 1, Position: 0, ContentID: "src-p001", QuestionID: "q0", State: StateTimedOut, PresentedAt: start.Add(1 * time.Minute), DeadlineAt: start.Add(90 * time.Second), AnsweredAt: start.Add(90 * time.Second), AttemptID: 1},
		{RunID: 1, Position: 1, ContentID: "src-p002", QuestionID: "q1", State: StateAnswered, PresentedAt: start.Add(2 * time.Minute), DeadlineAt: start.Add(3 * time.Minute), AnsweredAt: start.Add(150 * time.Second), AttemptID: 2},
	}
	attempts := []attempt.Attempt{
		attemptRecord(1, attempt.StatusSkipped, 30_000, start.Add(90*time.Second)),
		attemptRecord(2, attempt.StatusCorrect, 10_000, start.Add(150*time.Second)),
	}
	events := []Event{{Kind: EventResumed, At: start.Add(2 * time.Minute)}}
	report := BuildReport(run, items, attempts, events, nil, nil)
	if report.Counts.TimedOut != 1 || report.Counts.Correct != 1 || report.Counts.Total != 2 {
		t.Fatalf("counts = %+v", report.Counts)
	}
	if report.Speed.MedianMS != 20_000 || report.Speed.P90MS != 30_000 {
		t.Fatalf("speed = %+v", report.Speed)
	}
	if report.Counts.AnsweredAfterResume != 1 {
		t.Fatalf("answered after resume = %d", report.Counts.AnsweredAfterResume)
	}
	if !report.Items[1].AfterResume {
		t.Fatalf("item after resume = %+v", report.Items[1])
	}
}

func TestReportResumedAfterWindow(t *testing.T) {
	run := baseRun()
	start := run.StartedAt
	items := []Item{{RunID: 1, Position: 0, ContentID: "src-p001", QuestionID: "q0", State: StateAnswered, PresentedAt: start.Add(50 * time.Minute), DeadlineAt: start.Add(51 * time.Minute), AnsweredAt: start.Add(59 * time.Minute), AttemptID: 1}}
	attempts := []attempt.Attempt{attemptRecord(1, attempt.StatusCorrect, 1000, start.Add(59*time.Minute))}
	events := []Event{{Kind: EventResumed, At: start.Add(61 * time.Minute)}}
	report := BuildReport(run, items, attempts, events, nil, nil)
	if report.Contamination.ResumedAfterWindow != 1 {
		t.Fatalf("resumed after window = %+v", report.Contamination)
	}
}

func TestCompareRefusesInOrder(t *testing.T) {
	a := BuildReport(baseRun(), nil, nil, nil, nil, nil)
	b := BuildReport(baseRun(), nil, nil, nil, nil, nil)
	if err := Compare(a, b); err != nil {
		t.Fatalf("identical reports refused: %v", err)
	}
	b.Run.Status = StatusAbandoned
	if err := Compare(a, b); err == nil || err.(*CompareRefusal).Dimension != "status" {
		t.Fatalf("status refusal = %v", err)
	}
	b = BuildReport(baseRun(), nil, nil, nil, nil, nil)
	b.Run.ActualSize = 4
	if err := Compare(a, b); err == nil || err.(*CompareRefusal).Dimension != "item_count" {
		t.Fatalf("item refusal = %v", err)
	}
	b = BuildReport(baseRun(), nil, nil, nil, nil, nil)
	b.Run.WindowMinutes = 30
	if err := Compare(a, b); err == nil || err.(*CompareRefusal).Dimension != "window_minutes" {
		t.Fatalf("window refusal = %v", err)
	}
	b = BuildReport(baseRun(), nil, nil, nil, nil, nil)
	b.Run.PoolFamily = "other"
	if err := Compare(a, b); err == nil || err.(*CompareRefusal).Dimension != "pool_family" {
		t.Fatalf("family refusal = %v", err)
	}
}
