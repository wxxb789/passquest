package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/studio"
)

func TestWorkspaceTodayIsStructuredAndSpendsOncePerUnit(t *testing.T) {
	t.Parallel()
	f := newWSFixture(t, nil)
	first, err := f.w.Today()
	mustNoErr(t, err)
	if first.Recommendation == nil || first.Recommendation.Target.Kind == "" || first.Recommendation.Command == "" {
		t.Fatalf("recommendation = %+v", first.Recommendation)
	}
	if first.Prep.PassThreshold != "unknown" || first.Prep.ExamDate != "" || first.Prep.DailyMinutes != 0 || first.Prep.Day != 0 {
		t.Fatalf("prep = %+v", first.Prep)
	}
	if len(first.Skills) == 0 || first.Skills[0].Skill.ID == "" || first.Pool.FloorItems != assess.MinPoolItems {
		t.Fatalf("skills/pool = %+v / %+v", first.Skills, first.Pool)
	}
	if first.Model.Configured || first.Brief != nil || first.ActiveRun != nil || first.LastRun != nil {
		t.Fatalf("fresh database shows state: %+v", first)
	}
	keys := jsonKeys(t, first)
	for _, key := range []string{"recommendation", "skills", "pool", "anki", "brief", "find", "model", "prep", "last_run"} {
		if _, ok := keys[key]; !ok {
			t.Fatalf("Today JSON lacks %q: %v", key, keys)
		}
	}
	spentAfterFirst := countSpends(t, f.db, assess.SurfaceNext)
	for range 3 {
		again, err := f.w.Today()
		mustNoErr(t, err)
		if again.Recommendation.Target != first.Recommendation.Target {
			t.Fatalf("target changed without evidence: %+v vs %+v", again.Recommendation.Target, first.Recommendation.Target)
		}
	}
	if spentAfterFirst > 1 || countSpends(t, f.db, assess.SurfaceNext) != spentAfterFirst {
		t.Fatalf("next spends after renders = %d, first = %d", countSpends(t, f.db, assess.SurfaceNext), spentAfterFirst)
	}

	mustNoErr(t, f.w.SetLearnerSettings(studio.LearnerSettings{ExamDate: "2027-02-01", DailyMinutes: 45}))
	if err := f.w.SetLearnerSettings(studio.LearnerSettings{ExamDate: "next spring"}); studioCode(err) != "invalid_input" {
		t.Fatalf("bad date = %v", err)
	}
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample})
	mustNoErr(t, err)
	_, err = f.w.Answer(item.Token, "B")
	mustNoErr(t, err)
	today, err := f.w.Today()
	mustNoErr(t, err)
	if today.Prep.ExamDate != "2027-02-01" || today.Prep.DailyMinutes != 45 || today.Prep.Day != 1 || today.Prep.FirstAttemptAt == "" {
		t.Fatalf("prep after settings = %+v", today.Prep)
	}
	mustNoErr(t, f.w.SetLearnerSettings(studio.LearnerSettings{}))
	if cleared, _ := f.w.Today(); cleared.Prep.ExamDate != "" || cleared.Prep.DailyMinutes != 0 {
		t.Fatalf("cleared prep = %+v", cleared.Prep)
	}
}

// The desktop renders Today on boot. Reopening it after a run's window has
// passed must close the run under the CLI loop's rule, so the item that was
// on screen is recorded as timed out, not silently dropped from the report.
func TestWorkspaceTodayAfterElapsedRunTimesOutPresentedItem(t *testing.T) {
	t.Parallel()
	// Seed before the workspace opens: the seed is copied to a fresh path, and
	// opening an already migrated database is a no-op.
	f := newWSFixture(t, func(o *Options) { seedAssessmentSource(t, o.DBPath, 22, 1) })
	state, err := f.w.StartAssessment(2, 1)
	mustNoErr(t, err)
	if state.Item == nil || state.Item.Position != 0 {
		t.Fatalf("start = %+v", state)
	}
	f.clock.Advance(2 * time.Minute)
	f.reopen(t)
	today, err := f.w.Today()
	mustNoErr(t, err)
	if today.ActiveRun != nil || today.LastRun == nil || today.LastRun.ID != state.Run.ID {
		t.Fatalf("today runs = %+v / %+v", today.LastRun, today.ActiveRun)
	}
	report, err := f.w.AssessmentReport(state.Run.ID)
	mustNoErr(t, err)
	if report.Run.Status != assess.StatusAbandoned || report.Run.AbandonedReason != "window_elapsed" {
		t.Fatalf("expired run = %+v", report.Run)
	}
	if report.Counts.TimedOut != 1 || report.Counts.Finalized != 1 {
		t.Fatalf("on-screen item not timed out: %+v", report.Counts)
	}
	if len(report.Items) == 0 || !report.Items[0].TimedOut {
		t.Fatalf("presented item = %+v", report.Items[0])
	}
}

func TestWorkspaceTodaySpendsNothingDuringAnActiveRun(t *testing.T) {
	t.Parallel()
	// Seed before the workspace opens: the seed is copied to a fresh path, and
	// opening an already migrated database is a no-op.
	f := newWSFixture(t, func(o *Options) { seedAssessmentSource(t, o.DBPath, 22, 1) })
	state, err := f.w.StartAssessment(2, 30)
	mustNoErr(t, err)
	// The window renders Today on boot and on leaving a run; that must not
	// spend a unit that could be a pending item of the active run.
	today, err := f.w.Today()
	mustNoErr(t, err)
	if today.ActiveRun == nil || today.ActiveRun.ID != state.Run.ID {
		t.Fatalf("active run = %+v", today.ActiveRun)
	}
	if spent := countSpends(t, f.db, assess.SurfaceNext); spent != 0 {
		t.Fatalf("Today spent %d next presentation(s) during an active run", spent)
	}
	_, err = f.w.AbandonAssessment(state.Run.ID)
	mustNoErr(t, err)
	_, err = f.w.Today()
	mustNoErr(t, err)
	if spent := countSpends(t, f.db, assess.SurfaceNext); spent != 1 {
		t.Fatalf("Today after the run spent %d next presentation(s), want 1", spent)
	}
}

func TestWorkspaceCanCountAgreesWithAdaptiveEvidence(t *testing.T) {
	t.Parallel()
	f := newWSFixture(t, nil)
	first, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample, QuestionID: "q1"})
	mustNoErr(t, err)
	_, err = f.w.Answer(first.Token, "B")
	mustNoErr(t, err)
	// q2 shares q1's passage. Today's evidence (adaptive.Classify) counts only
	// the first attempt on a content id, so sample practice must not promise
	// independent evidence either.
	second, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample, QuestionID: "q2"})
	mustNoErr(t, err)
	if second.ContentID != first.ContentID {
		t.Fatalf("fixture: q2 content %q differs from q1 %q", second.ContentID, first.ContentID)
	}
	if second.CanCount || second.Condition != studio.ConditionReExposure {
		t.Fatalf("seen-passage sample item = %s can_count=%v", second.Condition, second.CanCount)
	}
	feedback, err := f.w.Answer(second.Token, "A")
	mustNoErr(t, err)
	if feedback.Counted || feedback.Condition != studio.ConditionReExposure {
		t.Fatalf("seen-passage feedback = %s counted=%v", feedback.Condition, feedback.Counted)
	}
}

// The window's generated command repeats a store selected through an override
// and omits --db only when its selected path is the app-owned default a
// command with no overrides resolves.
func TestWorkspaceTodayDatabaseArgumentFollowsTheSelectedStore(t *testing.T) {
	t.Parallel()
	config := t.TempDir()
	dataDir := filepath.Join(config, "moved")
	envDB := filepath.Join(config, "environment.sqlite")
	if err := os.MkdirAll(filepath.Join(config, "Pass Quest"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"app-owned default", nil, ""},
		{"PASSQUEST_DB", map[string]string{"PASSQUEST_DB": envDB}, envDB},
		{"PASSQUEST_DESKTOP_DATA", map[string]string{"PASSQUEST_DESKTOP_DATA": dataDir}, filepath.Join(dataDir, "passquest.sqlite")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := tt.env
			if env == nil {
				env = map[string]string{}
			}
			f := newWSFixture(t, func(o *Options) {
				o.DBPath = ""
				o.ConfigDir = func() (string, error) { return config, nil }
				o.Getenv = func(key string) string { return env[key] }
				o.Environ = func() []string { return envList(env) }
			})
			today, err := f.w.Today()
			mustNoErr(t, err)
			if today.Recommendation == nil {
				t.Fatal("no recommendation")
			}
			command := today.Recommendation.Command
			if tt.want == "" {
				if strings.Contains(command, "--db") {
					t.Fatalf("command repeats the app-owned default: %q", command)
				}
				return
			}
			if !strings.Contains(command, "--db") || !strings.Contains(command, tt.want) {
				t.Fatalf("command does not repeat the override-selected database %q: %q", tt.want, command)
			}
		})
	}
}
