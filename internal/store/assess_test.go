package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/attempt"
)

func TestMigrationFromVersionThirteenKeepsAttemptsAndAddsAssessment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v13.sqlite")
	db := openSQLite(t, path)
	for _, statements := range [][]string{
		schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6, schemaV7,
		schemaV8, schemaV9, schemaV10, schemaV11, schemaV12, schemaV13,
	} {
		for _, statement := range statements {
			if _, err := db.Exec(statement); err != nil {
				t.Fatalf("create v13 schema: %v", err)
			}
		}
	}
	at := "2026-11-12T09:00:00Z"
	if _, err := db.Exec(`INSERT INTO attempts (
			session_id, question_id, content_id, content_version, status,
			selected_label, selected_text, correct_label, correct_text,
			started_at, answered_at, elapsed_ms, assistance, tactic_id, skills_json
		) VALUES ('sess', 'q1', 'sample-0001', '1', 'correct',
			'B', 'Right', 'B', 'Right', ?, ?, 5, 'independent', '', '[]')`, at, at); err != nil {
		t.Fatalf("insert v13 attempt: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 13`); err != nil {
		t.Fatalf("set v13 version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v13 fixture: %v", err)
	}

	st := mustOpen(t, path)
	records, err := st.List()
	if err != nil || len(records) != 1 {
		t.Fatalf("attempts after migration = %+v err=%v", records, err)
	}
	if records[0].QuestionID != "q1" || records[0].Assistance != attempt.AssistanceIndependent || records[0].AssessmentRunID != 0 {
		t.Fatalf("attempt changed: %+v", records[0])
	}
	if _, err := st.db.Exec(`UPDATE attempts SET status = 'incorrect'`); err == nil {
		t.Fatal("attempt update was accepted after the table rebuild")
	}
	if _, err := st.db.Exec(`DELETE FROM attempts`); err == nil {
		t.Fatal("attempt delete was accepted after the table rebuild")
	}

	// The widened CHECK admits a timed assessment answer only with its run.
	if _, err := st.db.Exec(`INSERT INTO attempts (
			session_id, question_id, content_id, content_version, status,
			selected_label, selected_text, correct_label, correct_text,
			started_at, answered_at, elapsed_ms, assistance, tactic_id, skills_json,
			assessment_run_id
		) VALUES ('sess', 'q2', 'c', '1', 'correct', 'A', 'x', 'A', 'x', ?, ?, 1, 'none', '', '[]', 0)`, at, at); err == nil {
		t.Fatal("assistance none without a run id was accepted")
	}
}

func assessmentRunFixture(items int) (assess.Run, []assess.Item) {
	start := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	run := assess.Run{
		CreatedAt: start, StartedAt: start, DeadlineAt: start.Add(30 * time.Minute),
		WindowMinutes: 30, RequestedSize: items, PerItemTimeoutMS: 60_000,
		Annotations: assess.AnnotationsNone,
		PoolFamily:  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		SourceCount: 1,
	}
	list := make([]assess.Item, 0, items)
	for index := 0; index < items; index++ {
		list = append(list, assess.Item{
			Position: index, ContentID: "src-p001", SourceID: "src", Page: 1,
			ContentVersion: "v1", QuestionID: "q" + string(rune('a'+index)),
		})
	}
	return run, list
}

func assessmentAttemptFixture(contentID, questionID string, status attempt.Status, start, answered time.Time) attempt.Attempt {
	record := attempt.Attempt{
		SessionID: "sess", ContentID: contentID, QuestionID: questionID, ContentVersion: "v1",
		Status: status, CorrectLabel: "A", CorrectText: "right",
		StartedAt: start, AnsweredAt: answered, ElapsedMS: answered.Sub(start).Milliseconds(),
	}
	if status != attempt.StatusSkipped {
		record.SelectedLabel = "A"
		record.SelectedText = "right"
		if status == attempt.StatusIncorrect {
			record.SelectedLabel = "B"
			record.SelectedText = "wrong"
		}
	}
	return record
}

func TestAssessmentRunLifecycle(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "assess.sqlite"))
	run, items := assessmentRunFixture(2)
	created, err := st.CreateAssessmentRun(run, items)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if created.ID == 0 || created.Status != assess.StatusActive {
		t.Fatalf("created = %+v", created)
	}
	if _, err := st.CreateAssessmentRun(run, items); !errors.Is(err, ErrAssessmentActive) {
		t.Fatalf("second active run = %v", err)
	}

	presented, err := st.MarkAssessmentPresented(created.ID, 0, created.StartedAt.Add(time.Minute))
	if err != nil || presented.State != assess.StatePresented || presented.DeadlineAt.IsZero() {
		t.Fatalf("present = %+v, %v", presented, err)
	}
	answered := assessmentAttemptFixture(presented.ContentID, presented.QuestionID, attempt.StatusCorrect,
		presented.PresentedAt, presented.PresentedAt.Add(10*time.Second))
	stored, err := st.AppendAssessmentAttempt(created.ID, 0, answered, false, answered.AnsweredAt)
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if stored.Assistance != attempt.AssistanceNone || stored.AssessmentRunID != created.ID {
		t.Fatalf("stored answer = %+v", stored)
	}
	if _, err := st.AppendAssessmentAttempt(created.ID, 0, answered, false, answered.AnsweredAt); !errors.Is(err, ErrAssessmentRunState) {
		t.Fatalf("duplicate answer = %v", err)
	}

	presented, err = st.MarkAssessmentPresented(created.ID, 1, created.StartedAt.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("present second: %v", err)
	}
	timedOut := assessmentAttemptFixture(presented.ContentID, presented.QuestionID, attempt.StatusSkipped,
		presented.PresentedAt, presented.DeadlineAt)
	if _, err := st.AppendAssessmentAttempt(created.ID, 1, timedOut, true, presented.DeadlineAt); err != nil {
		t.Fatalf("timeout: %v", err)
	}
	completed, err := st.CompleteAssessmentRun(created.ID, created.StartedAt.Add(10*time.Minute))
	if err != nil || completed.Status != assess.StatusCompleted {
		t.Fatalf("complete = %+v, %v", completed, err)
	}
	if _, err := st.ResumeAssessmentRun(created.ID, created.StartedAt.Add(11*time.Minute)); !errors.Is(err, ErrAssessmentRunState) {
		t.Fatalf("resume completed = %v", err)
	}
	loaded, loadedItems, events, err := st.AssessmentRun(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != assess.StatusCompleted || len(loadedItems) != 2 || len(events) < 4 {
		t.Fatalf("loaded = %+v items=%d events=%d", loaded, len(loadedItems), len(events))
	}
	if loadedItems[0].State != assess.StateAnswered || loadedItems[1].State != assess.StateTimedOut {
		t.Fatalf("item states = %+v", loadedItems)
	}
	attempts, err := st.AssessmentAttempts(created.ID)
	if err != nil || len(attempts) != 2 {
		t.Fatalf("attempts = %+v, %v", attempts, err)
	}
}

func TestAssessmentRunResumeAbandonAndReconcile(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "assess.sqlite"))
	run, items := assessmentRunFixture(1)
	created, err := st.CreateAssessmentRun(run, items)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := st.ResumeAssessmentRun(created.ID, created.StartedAt.Add(time.Minute))
	if err != nil || resumed.ResumeCount != 1 {
		t.Fatalf("resume = %+v, %v", resumed, err)
	}
	abandoned, err := st.AbandonAssessmentRun(created.ID, created.StartedAt.Add(2*time.Minute), "learner_abandoned")
	if err != nil || abandoned.Status != assess.StatusAbandoned {
		t.Fatalf("abandon = %+v, %v", abandoned, err)
	}
	if _, err := st.AbandonAssessmentRun(created.ID, created.StartedAt.Add(3*time.Minute), "again"); err != nil {
		t.Fatalf("idempotent abandon: %v", err)
	}

	second, secondItems := assessmentRunFixture(1)
	second.CreatedAt = created.StartedAt.Add(5 * time.Minute)
	second.StartedAt = second.CreatedAt
	second.DeadlineAt = second.CreatedAt.Add(time.Minute)
	expired, err := st.CreateAssessmentRun(second, secondItems)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := st.ReconcileAssessmentRuns(expired.DeadlineAt.Add(time.Second))
	if err != nil || len(ids) != 1 || ids[0] != expired.ID {
		t.Fatalf("reconcile = %v, %v", ids, err)
	}
	loaded, _, _, err := st.AssessmentRun(expired.ID)
	if err != nil || loaded.Status != assess.StatusAbandoned || loaded.AbandonedReason != "window_elapsed" {
		t.Fatalf("expired run = %+v, %v", loaded, err)
	}
}

func TestSpendsUnionIncludesOldAndNewEvidence(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "spends.sqlite"))
	start := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	record := assessmentAttemptFixture("src-p001", "q1", attempt.StatusCorrect, start, start.Add(time.Second))
	if _, err := st.Append(record); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordExposure(Exposure{TacticID: "not-except-polarity", ContentID: "src-p002", QuestionID: "q2", ExposedAt: start.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordPresentation("src-p003", "q3", assess.SurfaceShow, start.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveNoteDraft(NoteDraft{
		TargetKey: "src-p004\x1fq4\x1f", ContentID: "src-p004", ContentVersion: "v1",
		QuestionID: "q4", Origin: "deterministic",
	}, start.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	run, items := assessmentRunFixture(1)
	items[0].ContentID = "src-p005"
	created, err := st.CreateAssessmentRun(run, items)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.MarkAssessmentPresented(created.ID, 0, start.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}

	spends, err := st.Spends()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"src-p001": false, "src-p002": false, "src-p003": false, "src-p004": false, "src-p005": false,
	}
	for _, spend := range spends {
		if _, ok := want[spend.ContentID]; ok {
			want[spend.ContentID] = true
		}
	}
	for content, found := range want {
		if !found {
			t.Fatalf("spend union missed %s: %+v", content, spends)
		}
	}
}

func TestContentPresentationsAreAppendOnly(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "present.sqlite"))
	if err := st.RecordPresentation("src-p001", "q1", assess.SurfacePractice, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`UPDATE content_presentations SET question_id = 'q2'`); err == nil {
		t.Fatal("presentation update accepted")
	}
	if _, err := st.db.Exec(`DELETE FROM content_presentations`); err == nil {
		t.Fatal("presentation delete accepted")
	}
}
