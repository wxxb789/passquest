package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/attempt"
)

func TestMigrationFromVersionSixteenAddsWorkspaceTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v16.sqlite")
	db := openSQLite(t, path)
	for _, migration := range migrations {
		if migration.version > 16 {
			break
		}
		for _, statement := range migration.statements {
			if _, err := db.Exec(statement); err != nil {
				t.Fatalf("create v16 schema at %d: %v", migration.version, err)
			}
		}
	}
	at := "2026-11-14T09:00:00Z"
	if _, err := db.Exec(`INSERT INTO attempts (
			session_id, question_id, content_id, content_version, status,
			selected_label, selected_text, correct_label, correct_text,
			started_at, answered_at, elapsed_ms, assistance, tactic_id, skills_json
		) VALUES ('sess', 'q1', 'sample-0001', '1', 'correct',
			'B', 'Right', 'B', 'Right', ?, ?, 5, 'independent', '', '[]')`, at, at); err != nil {
		t.Fatalf("insert v16 attempt: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO content_presentations (content_id, question_id, surface, at)
		VALUES ('sample-0001', 'q1', 'next', ?)`, at); err != nil {
		t.Fatalf("insert v16 presentation: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 16`); err != nil {
		t.Fatalf("set v16 version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	st := mustOpen(t, path)
	var version int
	if err := st.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 17 || SchemaVersion != 17 {
		t.Fatalf("user_version = %d, %v; want 17", version, err)
	}
	records, err := st.List()
	if err != nil || len(records) != 1 || records[0].ElapsedMS != 5 {
		t.Fatalf("attempts after migration = %+v, %v", records, err)
	}
	first, found, err := st.FirstAttemptAt()
	if err != nil || !found || first.Format(time.RFC3339) != at {
		t.Fatalf("first attempt = %v %v %v", first, found, err)
	}
	if _, found, err := st.LatestOpenPracticePresentation(); err != nil || found {
		t.Fatalf("open presentation after migration = %v %v", found, err)
	}
	if wrote, err := st.RecordPresentationOnce("sample-0001", "q1", assess.SurfaceNext, time.Now()); err != nil || wrote {
		t.Fatalf("existing next presentation was written again: %v %v", wrote, err)
	}
	if _, err := st.db.Exec(`INSERT INTO learner_settings (key, value, updated_at) VALUES ('pass_mark', '60', ?)`, at); err == nil {
		t.Fatal("learner_settings accepted a key outside the display-only pair")
	}
}

func TestLearnerSettingsSetReadAndClear(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "settings.sqlite"))
	at := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	if _, found, err := st.LearnerSetting(LearnerSettingExamDate); err != nil || found {
		t.Fatalf("unset setting = %v %v", found, err)
	}
	for _, value := range []string{"2027-01-10", "2027-02-01"} {
		if err := st.SetLearnerSetting(LearnerSettingExamDate, value, at); err != nil {
			t.Fatal(err)
		}
		if got, found, err := st.LearnerSetting(LearnerSettingExamDate); err != nil || !found || got != value {
			t.Fatalf("setting = %q %v %v; want %q", got, found, err, value)
		}
	}
	if err := st.SetLearnerSetting(LearnerSettingExamDate, "", at); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := st.LearnerSetting(LearnerSettingExamDate); found {
		t.Fatal("empty value did not clear the setting")
	}
	if err := st.SetLearnerSetting("pass_threshold", "60", at); err == nil {
		t.Fatal("unknown learner setting accepted")
	}
}

func practiceRecord(t *testing.T, questionID string, started, answered time.Time) attempt.Attempt {
	t.Helper()
	return attempt.Attempt{
		SessionID: "sess", QuestionID: questionID, ContentID: "sample-0001", ContentVersion: "1",
		Status: attempt.StatusCorrect, SelectedLabel: "B", SelectedText: "Right",
		CorrectLabel: "B", CorrectText: "Right", StartedAt: started, AnsweredAt: answered,
		ElapsedMS: answered.Sub(started).Milliseconds(), Assistance: attempt.AssistanceIndependent,
	}
}

func TestPracticePresentationLifecycle(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "presentations.sqlite"))
	base := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	first := PracticePresentation{Token: "token-0001", Kind: "sample", ContentID: "sample-0001", ContentVersion: "1", QuestionID: "q1", PresentedAt: base}
	tests := []struct {
		name string
		p    PracticePresentation
		surf string
	}{
		{"short token", PracticePresentation{Token: "x", Kind: "sample", ContentID: "c", ContentVersion: "1", QuestionID: "q", PresentedAt: base}, assess.SurfacePractice},
		{"no time", PracticePresentation{Token: "token-bad1", Kind: "sample", ContentID: "c", ContentVersion: "1", QuestionID: "q"}, assess.SurfacePractice},
		{"bad surface", PracticePresentation{Token: "token-bad2", Kind: "sample", ContentID: "c", ContentVersion: "1", QuestionID: "q", PresentedAt: base}, "window"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := st.OpenPracticePresentation(tt.p, tt.surf); err == nil {
				t.Fatal("invalid presentation accepted")
			}
		})
	}
	if err := st.OpenPracticePresentation(first, assess.SurfacePractice); err != nil {
		t.Fatal(err)
	}
	open, found, err := st.LatestOpenPracticePresentation()
	if err != nil || !found || open.Token != first.Token || !open.PresentedAt.Equal(base) {
		t.Fatalf("open = %+v %v %v", open, found, err)
	}
	hinted, err := st.MarkPracticeHint(first.Token, base.Add(time.Second))
	if err != nil || !hinted.HintOpenedAt.Equal(base.Add(time.Second)) {
		t.Fatalf("hint = %+v %v", hinted, err)
	}
	if again, _ := st.MarkPracticeHint(first.Token, base.Add(time.Minute)); !again.HintOpenedAt.Equal(base.Add(time.Second)) {
		t.Fatal("second hint moved the first hint time")
	}

	second := first
	second.Token, second.QuestionID, second.PresentedAt = "token-0002", "q2", base.Add(2*time.Second)
	if err := st.OpenPracticePresentation(second, assess.SurfacePractice); err != nil {
		t.Fatal(err)
	}
	closed, err := st.PracticePresentationByToken(first.Token)
	if err != nil || closed.Open() || closed.ClosedReason != PracticeClosedAbandoned {
		t.Fatalf("superseded presentation = %+v %v", closed, err)
	}
	if _, _, err := st.AppendPracticeAttempt(first.Token, practiceRecord(t, "q1", base, base.Add(3*time.Second)), base); !errors.Is(err, ErrPracticeClosed) {
		t.Fatalf("answer on abandoned token = %v", err)
	}
	if _, _, err := st.AppendPracticeAttempt(second.Token, practiceRecord(t, "q1", base, base.Add(3*time.Second)), base); err == nil {
		t.Fatal("attempt for another question closed the presentation")
	}
	stored, p, err := st.AppendPracticeAttempt(second.Token, practiceRecord(t, "q2", second.PresentedAt, base.Add(9*time.Second)), base.Add(9*time.Second))
	if err != nil || stored.ID == 0 || p.AttemptID != stored.ID || p.ClosedReason != PracticeClosedAnswered || stored.ElapsedMS != 7000 {
		t.Fatalf("answer = %+v / %+v / %v", stored, p, err)
	}
	if _, p, err := st.AppendPracticeAttempt(second.Token, practiceRecord(t, "q2", second.PresentedAt, base.Add(10*time.Second)), base); !errors.Is(err, ErrPracticeClosed) || p.AttemptID != stored.ID {
		t.Fatalf("second answer = %+v %v", p, err)
	}
	records, err := st.List()
	if err != nil || len(records) != 1 {
		t.Fatalf("attempts = %d %v", len(records), err)
	}
	if _, found, _ := st.LatestOpenPracticePresentation(); found {
		t.Fatal("an answered presentation is still open")
	}
	spends, err := st.Spends()
	if err != nil {
		t.Fatal(err)
	}
	practice := 0
	for _, spend := range spends {
		if spend.Surface == assess.SurfacePractice {
			practice++
		}
	}
	if practice != 2 {
		t.Fatalf("practice spends = %d, want one per display", practice)
	}
	if _, err := st.PracticePresentationByToken("token-none"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown token = %v", err)
	}
}

func TestRecordPresentationOnceAndAssessmentRuns(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "once.sqlite"))
	at := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	for i, want := range []bool{true, false, false} {
		wrote, err := st.RecordPresentationOnce("src-p001", "q1", assess.SurfaceNext, at.Add(time.Duration(i)*time.Minute))
		if err != nil || wrote != want {
			t.Fatalf("call %d wrote = %v, %v; want %v", i, wrote, err, want)
		}
	}
	if wrote, _ := st.RecordPresentationOnce("src-p001", "q1", assess.SurfaceShow, at); !wrote {
		t.Fatal("another surface is a separate presentation")
	}
	if _, err := st.RecordPresentationOnce("", "q1", assess.SurfaceNext, at); err == nil {
		t.Fatal("empty content accepted")
	}
	runs, err := st.AssessmentRuns()
	if err != nil || len(runs) != 0 {
		t.Fatalf("runs on empty store = %+v %v", runs, err)
	}
	run := assess.Run{StartedAt: at, WindowMinutes: 10, RequestedSize: 1, PerItemTimeoutMS: 1000, PoolFamily: "0123456789012345678901234567890123456789012345678901234567890123"}
	items := []assess.Item{{Position: 0, ContentID: "src-p001", ContentVersion: "1", QuestionID: "q1"}}
	created, err := st.CreateAssessmentRun(run, items)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AbandonAssessmentRun(created.ID, at.Add(time.Minute), "learner_abandoned"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAssessmentRun(run, items); err != nil {
		t.Fatal(err)
	}
	runs, err = st.AssessmentRuns()
	if err != nil || len(runs) != 2 || runs[0].ID != created.ID || runs[0].Status != assess.StatusAbandoned || runs[1].Status != assess.StatusActive {
		t.Fatalf("runs = %+v %v", runs, err)
	}
}
