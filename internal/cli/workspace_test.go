package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/studio"
	"github.com/wxxb789/passquest/internal/tutor"
)

// wsClock advances by step on every reading and can jump forward, so elapsed
// time and assessment deadlines are deterministic.
type wsClock struct {
	mu   sync.Mutex
	now  time.Time
	step time.Duration
}

func newWSClock() *wsClock { return &wsClock{now: goldenBase, step: time.Second} }

func (c *wsClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(c.step)
	return c.now
}

func (c *wsClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// memCredentials is the in-memory credential store tests inject in place of
// the OS store.
type memCredentials struct {
	mu     sync.Mutex
	values map[string]string
}

func newMemCredentials() *memCredentials { return &memCredentials{values: map[string]string{}} }

func (m *memCredentials) Get(p string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.values[p]
	return value, ok, nil
}

func (m *memCredentials) Set(p, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[p] = key
	return nil
}

func (m *memCredentials) Delete(p string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, p)
	return nil
}

type wsFixture struct {
	db    string
	clock *wsClock
	env   map[string]string
	opts  WorkspaceOptions
	w     *Workspace
}

func newWSFixture(t *testing.T, mutate func(*WorkspaceOptions)) *wsFixture {
	t.Helper()
	f := &wsFixture{db: newDB(t), clock: newWSClock(), env: map[string]string{}}
	tokens := 0
	f.opts = WorkspaceOptions{
		DBPath:     f.db,
		Getenv:     func(key string) string { return f.env[key] },
		Environ:    func() []string { return envList(f.env) },
		Now:        f.clock.Now,
		NewSession: func() string { return "sess-ws" },
		NewToken: func() string {
			tokens++
			return fmt.Sprintf("token-%04d", tokens)
		},
	}
	if mutate != nil {
		mutate(&f.opts)
	}
	f.reopen(t)
	return f
}

func (f *wsFixture) reopen(t *testing.T) {
	t.Helper()
	w, err := OpenWorkspace(f.opts)
	if err != nil {
		t.Fatalf("OpenWorkspace: %v", err)
	}
	f.w = w
}

func envList(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, key+"="+value)
	}
	sort.Strings(out)
	return out
}

func studioCode(err error) string {
	var typed *studio.Error
	if errors.As(err, &typed) {
		return typed.Code
	}
	return ""
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func jsonKeys(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func listAttempts(t *testing.T, db string) int {
	t.Helper()
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	records, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	return len(records)
}

func countSpends(t *testing.T, db, surface string) int {
	t.Helper()
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	spends, err := st.Spends()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, spend := range spends {
		if spend.Surface == surface {
			count++
		}
	}
	return count
}

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

func TestWorkspacePracticeRecordsElapsedAndIsIdempotent(t *testing.T) {
	t.Parallel()
	f := newWSFixture(t, nil)
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample, QuestionID: "q1"})
	mustNoErr(t, err)
	if item.Token == "" || item.PresentedAt == "" || item.Condition != studio.ConditionIndependent || !item.CanCount {
		t.Fatalf("item = %+v", item)
	}
	if item.Passage.Label.Kind != studio.SourceSample || len(item.Passage.Paragraphs) == 0 || len(item.Options) == 0 {
		t.Fatalf("passage/options = %+v / %+v", item.Passage, item.Options)
	}
	keys := jsonKeys(t, item)
	for _, banned := range []string{"answer", "evidence", "reasoning", "option_reasoning", "correct_label", "key"} {
		if _, ok := keys[banned]; ok {
			t.Fatalf("pre-answer item exposes %q", banned)
		}
	}
	if item.HintAvailable || item.HintReason == "" {
		t.Fatalf("sample hint = %v %q", item.HintAvailable, item.HintReason)
	}
	if _, err := f.w.OpenHint(item.Token); studioCode(err) != "hint_unavailable" {
		t.Fatalf("hint on sample = %v", err)
	}
	if countSpends(t, f.db, assess.SurfacePractice) != 1 {
		t.Fatal("display did not record the practice presentation")
	}
	if listAttempts(t, f.db) != 0 {
		t.Fatal("display wrote an attempt")
	}

	f.clock.Advance(7 * time.Second)
	feedback, err := f.w.Answer(item.Token, "B")
	mustNoErr(t, err)
	if feedback.AttemptID == 0 || feedback.ElapsedMS < 7000 || feedback.Verdict != "correct" || !feedback.Counted || feedback.AlreadyAnswered {
		t.Fatalf("feedback = %+v", feedback)
	}
	if feedback.Key.Label != "B" || len(feedback.Options) != len(item.Options) || !feedback.Evidence.Located {
		t.Fatalf("key/options/evidence = %+v / %+v / %+v", feedback.Key, feedback.Options, feedback.Evidence)
	}
	paragraph := []rune(feedback.Passage.Paragraphs[feedback.Evidence.Paragraph])
	if string(paragraph[feedback.Evidence.Start:feedback.Evidence.End]) != feedback.Evidence.Text {
		t.Fatalf("evidence span does not select the evidence: %+v", feedback.Evidence)
	}
	for _, option := range feedback.Options {
		if option.Missing == (option.Explanation != "") {
			t.Fatalf("option %s missing flag inconsistent: %+v", option.Label, option)
		}
	}

	again, err := f.w.Answer(item.Token, "A")
	mustNoErr(t, err)
	if again.AttemptID != feedback.AttemptID || !again.AlreadyAnswered || again.Verdict != "correct" || listAttempts(t, f.db) != 1 {
		t.Fatalf("re-submit = %+v, attempts = %d", again, listAttempts(t, f.db))
	}

	code, out, errOut := runCLI(t, f.db, &stepClock{base: goldenBase}, "", "report", "--json")
	if code != 0 {
		t.Fatalf("report = %d %s", code, errOut)
	}
	report := decodeReport(t, out)
	if len(report.Attempts) != 1 || report.Attempts[0].ElapsedMS != feedback.ElapsedMS || report.Attempts[0].Assistance != "independent" || report.Attempts[0].Repeat {
		t.Fatalf("report attempt = %+v", report.Attempts)
	}

	revisit, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample, QuestionID: "q1"})
	mustNoErr(t, err)
	if revisit.Condition != studio.ConditionRevisit || revisit.CanCount || revisit.PriorAttempts != 1 {
		t.Fatalf("revisit item = %+v", revisit)
	}
	skipped, err := f.w.Skip(revisit.Token)
	mustNoErr(t, err)
	if skipped.Verdict != "skipped" || skipped.Condition != studio.ConditionRevisit || skipped.Counted {
		t.Fatalf("skip = %+v", skipped)
	}
	if _, err := f.w.Answer("token-never", "A"); studioCode(err) != "unknown_token" {
		t.Fatalf("unknown token = %v", err)
	}
	bad, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample, QuestionID: "q2"})
	mustNoErr(t, err)
	if _, err := f.w.Answer(bad.Token, "Z"); studioCode(err) != "invalid_answer" {
		t.Fatalf("invalid label = %v", err)
	}
	if _, err := f.w.StartPractice(studio.PracticeRequest{Kind: "quiz"}); studioCode(err) != "invalid_input" {
		t.Fatalf("unknown kind = %v", err)
	}
}

func TestWorkspaceHintMakesTacticAttemptAssisted(t *testing.T) {
	t.Parallel()
	f := newWSFixture(t, nil)
	guided, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeTactic, Tactic: "not-except-polarity"})
	mustNoErr(t, err)
	if guided.Stage != stageGuided || guided.Condition != studio.ConditionAssisted || guided.Guidance == nil || guided.HintAvailable {
		t.Fatalf("guided item = %+v", guided)
	}
	if _, err := f.w.Answer(guided.Token, "A"); err != nil {
		t.Fatal(err)
	}
	independent, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeTactic, Tactic: "not-except-polarity"})
	mustNoErr(t, err)
	// The independent stage reuses the guided passage, so under Q-0004-01 (B)
	// it is passage re-exposure: recorded unassisted, but never counted as
	// independent transfer in Today's evidence.
	if independent.Stage != stageIndependent || independent.Condition != studio.ConditionReExposure || independent.CanCount || !independent.HintAvailable || independent.Hint != nil {
		t.Fatalf("independent item = %+v", independent)
	}
	hinted, err := f.w.OpenHint(independent.Token)
	mustNoErr(t, err)
	if hinted.Condition != studio.ConditionAssisted || hinted.CanCount || hinted.Hint == nil || !hinted.HintOpened {
		t.Fatalf("after hint = %+v", hinted)
	}
	if hinted.Token != independent.Token || hinted.PresentedAt != independent.PresentedAt {
		t.Fatal("hint changed the presentation")
	}
	feedback, err := f.w.Answer(independent.Token, "B")
	mustNoErr(t, err)
	if feedback.Condition != studio.ConditionAssisted || feedback.Counted || feedback.Assistance != "assisted" || feedback.Tactic == nil || feedback.Tactic.ID != "not-except-polarity" {
		t.Fatalf("hinted feedback = %+v", feedback)
	}
	_, out, _ := runCLI(t, f.db, &stepClock{base: goldenBase}, "", "report", "--json")
	report := decodeReport(t, out)
	last := report.Attempts[len(report.Attempts)-1]
	if last.ID != feedback.AttemptID || last.Assistance != "assisted" || last.TacticID != "not-except-polarity" {
		t.Fatalf("report attempt after hint = %+v", last)
	}
}

// The desktop opens the workspace without a session minter; answering must
// still record the attempt under a fresh session.
func TestWorkspaceAnswerWithDefaultSession(t *testing.T) {
	t.Parallel()
	f := newWSFixture(t, func(o *WorkspaceOptions) { o.NewSession = nil })
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample})
	mustNoErr(t, err)
	feedback, err := f.w.Answer(item.Token, item.Options[0].Label)
	mustNoErr(t, err)
	if feedback.AttemptID == 0 || listAttempts(t, f.db) != 1 {
		t.Fatalf("feedback = %+v, attempts = %d", feedback, listAttempts(t, f.db))
	}
}

func TestWorkspaceResumesPracticeAfterReopen(t *testing.T) {
	t.Parallel()
	f := newWSFixture(t, nil)
	if none, err := f.w.ResumePractice(); err != nil || none != nil {
		t.Fatalf("resume on empty = %+v, %v", none, err)
	}
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSkill, Skill: "vocabulary-in-context"})
	mustNoErr(t, err)
	f.reopen(t)
	resumed, err := f.w.ResumePractice()
	mustNoErr(t, err)
	if resumed == nil || resumed.Token != item.Token || resumed.PresentedAt != item.PresentedAt || resumed.QuestionID != item.QuestionID || resumed.SkillID != "vocabulary-in-context" {
		t.Fatalf("resumed = %+v, want %+v", resumed, item)
	}
	if listAttempts(t, f.db) != 0 || countSpends(t, f.db, assess.SurfaceSkill) != 1 {
		t.Fatal("resume wrote an attempt or a second presentation")
	}
	f.clock.Advance(30 * time.Second)
	feedback, err := f.w.Answer(resumed.Token, "A")
	mustNoErr(t, err)
	if feedback.ElapsedMS < 30000 || feedback.PresentedAt != item.PresentedAt {
		t.Fatalf("elapsed after resume = %+v", feedback)
	}
	if after, err := f.w.ResumePractice(); err != nil || after != nil {
		t.Fatalf("answered item still resumable: %+v %v", after, err)
	}

	first, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample})
	mustNoErr(t, err)
	second, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample})
	mustNoErr(t, err)
	if _, err := f.w.Answer(first.Token, "B"); studioCode(err) != "presentation_closed" {
		t.Fatalf("abandoned token answer = %v", err)
	}
	if latest, _ := f.w.ResumePractice(); latest == nil || latest.Token != second.Token {
		t.Fatalf("latest open = %+v", latest)
	}
}

func TestWorkspaceAssessmentStepFlow(t *testing.T) {
	t.Parallel()
	// Seed before the workspace opens: the seed is copied to a fresh path, and
	// opening an already migrated database is a no-op.
	f := newWSFixture(t, func(o *WorkspaceOptions) { seedAssessmentSource(t, o.DBPath, 22, 1) })
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
	f := newWSFixture(t, func(o *WorkspaceOptions) { seedAssessmentSource(t, o.DBPath, 22, 1) })
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

// The desktop renders Today on boot. Reopening it after a run's window has
// passed must close the run under the CLI loop's rule, so the item that was
// on screen is recorded as timed out, not silently dropped from the report.
func TestWorkspaceTodayAfterElapsedRunTimesOutPresentedItem(t *testing.T) {
	t.Parallel()
	// Seed before the workspace opens: the seed is copied to a fresh path, and
	// opening an already migrated database is a no-op.
	f := newWSFixture(t, func(o *WorkspaceOptions) { seedAssessmentSource(t, o.DBPath, 22, 1) })
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

func TestWorkspaceCredentialStoreLayering(t *testing.T) {
	t.Parallel()
	creds := newMemCredentials()
	const storeKey = "store-key-0123456789-not-real"
	const envKey = "env-key-9876543210-not-real"
	f := newWSFixture(t, func(o *WorkspaceOptions) { o.Credentials = creds })
	registry := modelRegistryFile(t, modelProvider("alpha", "https://alpha.example.com/v1/chat/completions"))
	f.env[provider.RegistryEnv] = registry
	transport := &fakeTutorTransport{response: fixedTutorResponse("OK")}
	f.opts.TutorTransport = transport
	f.reopen(t)
	envName := provider.CredentialEnvFor("alpha")

	if _, err := f.w.SetCredential("nobody", storeKey); studioCode(err) != "unknown_provider" {
		t.Fatalf("unknown provider = %v", err)
	}
	if _, err := f.w.SetCredential("alpha", "two words"); studioCode(err) != "invalid_input" {
		t.Fatalf("invalid key = %v", err)
	}
	status, err := f.w.SetCredential("alpha", storeKey)
	mustNoErr(t, err)
	if !status.Present || status.From != "store" {
		t.Fatalf("status after set = %+v", status)
	}
	if got := f.w.getenv(envName); got != storeKey {
		t.Fatalf("store layer not used: %q", got)
	}
	if !containsExact(f.w.environ(), envName+"="+storeKey) {
		t.Fatal("child environment lacks the stored credential")
	}

	_, err = f.w.UseModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	test, err := f.w.TestModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	if test.Result != "verified" || len(transport.requests) != 1 {
		t.Fatalf("model test with stored key = %+v", test)
	}
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample})
	mustNoErr(t, err)
	feedback, err := f.w.Answer(item.Token, "A")
	mustNoErr(t, err)
	transport.response = fixedTutorResponse("Look at the second sentence again.")
	reply, err := f.w.CoachAsk(feedback.AttemptID, studio.CoachExplain)
	mustNoErr(t, err)
	if reply.State != "advisory" || !reply.Advisory || reply.VerifiedAnswer || reply.Text == "" {
		t.Fatalf("coach reply = %+v", reply)
	}

	f.env[envName] = envKey
	if got := f.w.getenv(envName); got != envKey {
		t.Fatalf("environment did not win: %q", got)
	}
	if containsExact(f.w.environ(), envName+"="+storeKey) {
		t.Fatal("stored credential shadowed the environment in the child environment")
	}
	services, err := f.w.Services()
	mustNoErr(t, err)
	found := false
	for _, c := range services.Credentials {
		if c.Provider == "alpha" {
			found = c.Present && c.From == "env"
		}
	}
	if !found || !services.CredentialStore {
		t.Fatalf("services = %+v", services)
	}
	delete(f.env, envName)
	cleared, err := f.w.ClearCredential("alpha")
	mustNoErr(t, err)
	if cleared.Present || cleared.From != "none" {
		t.Fatalf("after clear = %+v", cleared)
	}

	for _, value := range []any{status, services, test, reply, feedback} {
		raw, _ := json.Marshal(value)
		if strings.Contains(string(raw), storeKey) || strings.Contains(string(raw), envKey) {
			t.Fatalf("a document carries the key: %s", raw)
		}
	}
	matches, _ := filepath.Glob(f.db + "*")
	if len(matches) == 0 {
		t.Fatal("no database files to scan")
	}
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), storeKey) || strings.Contains(string(data), envKey) {
			t.Fatalf("%s contains a credential", path)
		}
	}

	noStore := newWSFixture(t, nil)
	if _, err := noStore.w.SetCredential("openai", storeKey); studioCode(err) != "credential_store_unavailable" {
		t.Fatalf("set without a store = %v", err)
	}
}

func TestWorkspaceCoachPreviewAndFailure(t *testing.T) {
	t.Parallel()
	transport := &fakeTutorTransport{response: fixedTutorResponse("OK")}
	f := newWSFixture(t, func(o *WorkspaceOptions) { o.TutorTransport = transport })
	registry := modelRegistryFile(t, modelProvider("alpha", "http://127.0.0.1:9/v1/chat/completions"))
	f.env[provider.RegistryEnv] = registry
	f.env[provider.CredentialEnvFor("alpha")] = fakeTutorKey
	f.reopen(t)
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample})
	mustNoErr(t, err)
	feedback, err := f.w.Answer(item.Token, "A")
	mustNoErr(t, err)
	if _, err := f.w.CoachPreview(feedback.AttemptID, studio.CoachExplain); studioCode(err) != "not_configured" {
		t.Fatalf("preview without model = %v", err)
	}
	_, err = f.w.UseModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	if _, err := f.w.CoachAsk(feedback.AttemptID, studio.CoachExplain); studioCode(err) != "capability_unverified" {
		t.Fatalf("ask before test = %v", err)
	}
	_, err = f.w.TestModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	models, err := f.w.Models()
	mustNoErr(t, err)
	if !models.Usable || len(models.Providers) != 1 || models.Providers[0].Auth != "present" {
		t.Fatalf("models = %+v", models)
	}
	preview, err := f.w.CoachPreview(feedback.AttemptID, studio.CoachProbe)
	mustNoErr(t, err)
	if preview.External || !preview.Verified || len(preview.Messages) == 0 || preview.RequestSHA256 == "" || preview.RequestBytes == 0 {
		t.Fatalf("preview = %+v", preview)
	}
	if preview.Budget.Provider != "alpha" || preview.Budget.Limit == 0 || !preview.Credential.Present {
		t.Fatalf("preview budget/credential = %+v / %+v", preview.Budget, preview.Credential)
	}
	transport.response = fixedTutorError("rate_limited", "slow down")
	reply, err := f.w.CoachAsk(feedback.AttemptID, studio.CoachExplain)
	mustNoErr(t, err)
	if reply.State != "failed" || reply.Failure == nil || reply.Failure.Code == "" || reply.CallID == 0 {
		t.Fatalf("failure reply = %+v", reply)
	}
	if listAttempts(t, f.db) != 1 {
		t.Fatal("a coach failure changed attempts")
	}
	budget, err := f.w.Budget()
	mustNoErr(t, err)
	if budget.TotalCalls == 0 {
		t.Fatalf("budget = %+v", budget)
	}
	consents, err := f.w.Consents()
	mustNoErr(t, err)
	if len(consents) != 0 {
		t.Fatalf("consents on a database without sources = %+v", consents)
	}
	if _, err := f.w.GrantConsent("missing-source", ""); studioCode(err) != "unknown_source" {
		t.Fatalf("consent for unknown source = %v", err)
	}
}

func TestWorkspaceNotesFlow(t *testing.T) {
	t.Parallel()
	anki := newFakeAnki()
	f := newWSFixture(t, func(o *WorkspaceOptions) { o.AnkiTransport = anki })
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample, QuestionID: "q2"})
	mustNoErr(t, err)
	feedback, err := f.w.Answer(item.Token, "A")
	mustNoErr(t, err)
	if open, err := f.w.OpenNoteDraft(); err != nil || open != nil {
		t.Fatalf("open draft before any proposal = %+v, %v", open, err)
	}
	draft, err := f.w.ProposeNote(studio.NoteRequest{AttemptID: feedback.AttemptID})
	mustNoErr(t, err)
	if draft.ID == 0 || draft.Fields.Front == "" || draft.Target.AttemptID != feedback.AttemptID {
		t.Fatalf("draft = %+v", draft)
	}
	// A restart finds the proposed draft unchanged.
	f.reopen(t)
	open, err := f.w.OpenNoteDraft()
	mustNoErr(t, err)
	if open == nil || !reflect.DeepEqual(*open, draft) {
		t.Fatalf("open draft after reopening = %+v, proposed %+v", open, draft)
	}
	preview, err := f.w.PreviewNote(draft.ID, studio.NoteFields{Front: "My own front"})
	mustNoErr(t, err)
	if !preview.Edited || !strings.Contains(preview.HTML, "My own front") || preview.CSS == "" || preview.HTMLSHA256 == "" {
		t.Fatalf("preview = %+v", preview)
	}
	accepted, err := f.w.AcceptNote(draft.ID, studio.NoteFields{Front: "My own front"})
	mustNoErr(t, err)
	if !accepted.Created || accepted.Note.Origin != "edited" || accepted.Note.Fields.Front != "My own front" {
		t.Fatalf("accept = %+v", accepted)
	}
	if open, err := f.w.OpenNoteDraft(); err != nil || open != nil {
		t.Fatalf("open draft after accept = %+v, %v", open, err)
	}
	list, err := f.w.Notes()
	mustNoErr(t, err)
	if len(list.Notes) != 1 || list.Counts.Total != 1 {
		t.Fatalf("notes = %+v", list)
	}
	anki.unreachable = true
	queued, err := f.w.PushNotes(false)
	mustNoErr(t, err)
	if queued.State != "queued" || queued.Counts.Queued != 1 || queued.Notes[0].Outcome != "unreachable" {
		t.Fatalf("unreachable push = %+v", queued)
	}
	anki.unreachable = false
	pushed, err := f.w.PushNotes(false)
	mustNoErr(t, err)
	if pushed.Error != nil || pushed.Counts.Pushed != 1 {
		t.Fatalf("push = %+v", pushed)
	}
	if after, _ := f.w.Notes(); after.Notes[0].AnkiState != store.AnkiStatePushed || after.Counts.Pushed != 1 {
		t.Fatalf("notes after push = %+v", after)
	}

	second, err := f.w.ProposeNote(studio.NoteRequest{ContentID: feedback.ContentID, QuestionID: "q3", Mark: "a passage phrase"})
	mustNoErr(t, err)
	if second.Target.Mark != "a passage phrase" || second.Target.AttemptID != 0 {
		t.Fatalf("mark draft = %+v", second)
	}
	if open, err := f.w.OpenNoteDraft(); err != nil || open == nil || open.ID != second.ID {
		t.Fatalf("open draft after the mark proposal = %+v, %v", open, err)
	}
	mustNoErr(t, f.w.SkipNote(second.ID))
	if open, err := f.w.OpenNoteDraft(); err != nil || open != nil {
		t.Fatalf("open draft after skip = %+v, %v", open, err)
	}
	if _, err := f.w.PreviewNote(second.ID, studio.NoteFields{}); studioCode(err) != "draft_closed" {
		t.Fatalf("preview of skipped draft = %v", err)
	}
	if _, err := f.w.ProposeNote(studio.NoteRequest{AttemptID: feedback.AttemptID, Mark: strings.Repeat("x", 200)}); studioCode(err) != "invalid_mark" {
		t.Fatalf("over-long mark = %v", err)
	}
}

func TestWorkspaceBriefWithoutRun(t *testing.T) {
	t.Parallel()
	f := newWSFixture(t, nil)
	status, err := f.w.BriefStatus()
	mustNoErr(t, err)
	if status.Found || status.State != "none" {
		t.Fatalf("brief status = %+v", status)
	}
}

func TestWorkspaceTodaySpendsNothingDuringAnActiveRun(t *testing.T) {
	t.Parallel()
	// Seed before the workspace opens: the seed is copied to a fresh path, and
	// opening an already migrated database is a no-op.
	f := newWSFixture(t, func(o *WorkspaceOptions) { seedAssessmentSource(t, o.DBPath, 22, 1) })
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

func TestWorkspaceEmptyPracticeKindIsSample(t *testing.T) {
	t.Parallel()
	f := newWSFixture(t, nil)
	item, err := f.w.StartPractice(studio.PracticeRequest{})
	mustNoErr(t, err)
	if item.Kind != studio.PracticeSample || item.Passage.Label.Kind != studio.SourceSample {
		t.Fatalf("empty-kind item = %s / %s", item.Kind, item.Passage.Label.Kind)
	}
}

// Q-0011-03: an environment variable still wins over a stored key, including
// the custom provider's legacy fallback that provider.CredentialValue reads.
func TestWorkspaceLegacyCustomCredentialBeatsStoredKey(t *testing.T) {
	t.Parallel()
	creds := newMemCredentials()
	const storeKey = "store-key-0123456789-not-real"
	const legacyKey = "legacy-key-9876543210-not-real"
	f := newWSFixture(t, func(o *WorkspaceOptions) { o.Credentials = creds })
	_, err := f.w.SetCredential(provider.CustomProvider, storeKey)
	mustNoErr(t, err)
	primary := provider.CredentialEnvFor(provider.CustomProvider)
	if _, value, _ := provider.CredentialValue(f.w.getenv, provider.CustomProvider); value != storeKey {
		t.Fatalf("stored custom key not used without an environment key: %q", value)
	}

	f.env[provider.LegacyCredentialEnv] = legacyKey
	if name, value, _ := provider.CredentialValue(f.w.getenv, provider.CustomProvider); value != legacyKey || name != provider.LegacyCredentialEnv {
		t.Fatalf("legacy environment key lost to the store: %s=%q", name, value)
	}
	if containsExact(f.w.environ(), primary+"="+storeKey) {
		t.Fatal("stored custom key shadows the legacy variable in the child environment")
	}
	if status := f.w.credentialStatus(provider.CustomProvider); !status.Present || status.From != "env" {
		t.Fatalf("custom status with legacy variable = %+v", status)
	}
}

func TestLoopbackEndpointParsesTheHost(t *testing.T) {
	t.Parallel()
	for endpoint, want := range map[string]bool{
		"http://127.0.0.1:11434/v1/chat/completions":        true,
		"http://localhost:8080/v1/chat/completions":         true,
		"http://[::1]:8080/v1/chat/completions":             true,
		"https://127.0.0.1.attacker.example/v1":             false,
		"https://127.evil.example/v1/chat/completions":      false,
		"https://localhost.attacker.example/v1":             false,
		"https://api.example.com/v1/chat/completions":       false,
		"https://user@127.0.0.1.example:443/v1/completions": false,
	} {
		if got := loopbackEndpoint(endpoint); got != want {
			t.Errorf("loopbackEndpoint(%q) = %v, want %v", endpoint, got, want)
		}
	}
}

// TestWorkspaceNotePreviewIsTheDeckCard: the window's card documents are the
// note package's rendering of the same fields with the model stylesheet, in
// Anki's DOM, for both sides and both themes.
func TestWorkspaceNotePreviewIsTheDeckCard(t *testing.T) {
	t.Parallel()
	f := newWSFixture(t, nil)
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample, QuestionID: "q2"})
	mustNoErr(t, err)
	feedback, err := f.w.Answer(item.Token, "A")
	mustNoErr(t, err)
	draft, err := f.w.ProposeNote(studio.NoteRequest{AttemptID: feedback.AttemptID})
	mustNoErr(t, err)
	preview, err := f.w.PreviewNote(draft.ID, studio.NoteFields{Front: "{{Back}} stays literal"})
	mustNoErr(t, err)
	fields := note.HTML(preview.Fields.Front, preview.Fields.Back, preview.Fields.Tactic, preview.Source)
	question, answer := note.RenderSides(fields)
	want := studio.NoteCardSide{Light: note.CardDocument(question, note.ModelCSS, false), Night: note.CardDocument(question, note.ModelCSS, true)}
	if preview.Question != want {
		t.Fatalf("question documents = %+v", preview.Question)
	}
	want = studio.NoteCardSide{Light: note.CardDocument(answer, note.ModelCSS, false), Night: note.CardDocument(answer, note.ModelCSS, true)}
	if preview.Answer != want || preview.HTML != answer || preview.CSS != note.ModelCSS {
		t.Fatalf("answer documents = %+v", preview.Answer)
	}
	if !strings.Contains(preview.Answer.Night, `<body class="card card1 nightMode night_mode">`) ||
		!strings.Contains(preview.Question.Light, `<body class="card card1">`) {
		t.Fatal("the documents do not carry Anki's body classes")
	}
	_, questionBody, _ := strings.Cut(preview.Question.Light, "<body")
	if !strings.Contains(questionBody, "{{Back}} stays literal") || strings.Contains(questionBody, "pq-tactic") {
		t.Fatalf("question side = %s", questionBody)
	}
	if preview.FieldsSHA256 != fields.Hash() || preview.HTMLSHA256 != preview.FieldsSHA256 {
		t.Fatalf("field hashes = %s / %s", preview.FieldsSHA256, preview.HTMLSHA256)
	}
	if preview.CSSSHA256 != note.CSSSHA256() || preview.ModelVersion != note.ModelVersion || !preview.Edited {
		t.Fatalf("preview model = %s / %d / %t", preview.CSSSHA256, preview.ModelVersion, preview.Edited)
	}
	keys := jsonKeys(t, preview)
	for _, key := range []string{"question", "answer", "html", "css", "html_sha256", "fields_sha256", "css_sha256", "model_version"} {
		if _, ok := keys[key]; !ok {
			t.Fatalf("preview JSON lacks %s: %v", key, keys)
		}
	}
}

// TestWorkspaceRefineNote: the refinement preview is the request the
// refinement sends; the refusals, the budget and the recorded failure are
// note propose --refine's; the suggestion is advisory and only accept applies
// it.
func TestWorkspaceRefineNote(t *testing.T) {
	t.Parallel()
	transport := &fakeTutorTransport{response: fixedTutorResponse("OK")}
	f := newWSFixture(t, func(o *WorkspaceOptions) { o.TutorTransport = transport })
	f.env[provider.RegistryEnv] = modelRegistryFile(t, modelProvider("alpha", "http://127.0.0.1:9/v1/chat/completions"))
	f.env[provider.CredentialEnvFor("alpha")] = fakeTutorKey
	f.reopen(t)
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample, QuestionID: "q2"})
	mustNoErr(t, err)
	feedback, err := f.w.Answer(item.Token, "A")
	mustNoErr(t, err)
	draft, err := f.w.ProposeNote(studio.NoteRequest{AttemptID: feedback.AttemptID})
	mustNoErr(t, err)

	if _, err := f.w.RefineNotePreview(draft.ID); studioCode(err) != "not_configured" {
		t.Fatalf("preview without a model = %v", err)
	}
	if _, err := f.w.RefineNote(draft.ID); studioCode(err) != "not_configured" {
		t.Fatalf("refine without a model = %v", err)
	}
	_, err = f.w.UseModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	unverified, err := f.w.RefineNotePreview(draft.ID)
	mustNoErr(t, err)
	if unverified.Verified || unverified.Action == "" || unverified.TextChatState == "" {
		t.Fatalf("unverified preview = %+v", unverified)
	}
	if _, err := f.w.RefineNote(draft.ID); studioCode(err) != "capability_unverified" {
		t.Fatalf("refine before the model test = %v", err)
	}
	_, err = f.w.TestModel("alpha/alpha-text", "")
	mustNoErr(t, err)

	preview, err := f.w.RefineNotePreview(draft.ID)
	mustNoErr(t, err)
	if preview.DraftID != draft.ID || preview.Provider != "alpha" || preview.Model != "alpha-text" || preview.External ||
		!preview.Verified || preview.Action != "" || !preview.Credential.Present || preview.Unanswered {
		t.Fatalf("preview = %+v", preview)
	}
	if preview.Consent.Required || !preview.Consent.Granted || len(preview.Consent.Withheld) != 0 {
		t.Fatalf("preview consent = %+v", preview.Consent)
	}
	if len(preview.Messages) != 2 || preview.Messages[0].Role != "system" || preview.RequestSHA256 == "" || preview.RequestBytes == 0 {
		t.Fatalf("preview messages = %+v", preview)
	}
	if preview.Budget.Provider != "alpha" || preview.Budget.Limit == 0 {
		t.Fatalf("preview budget = %+v", preview.Budget)
	}
	callsBefore := preview.Budget.Calls
	sent := len(transport.requests)

	transport.response = fixedTutorResponse(`{"front": "closes at 4 p.m.", "back": "To shut at a stated time.\n\nExample: The library closes at 4 p.m."}`)
	refined, err := f.w.RefineNote(draft.ID)
	mustNoErr(t, err)
	if len(transport.requests) != sent+1 {
		t.Fatalf("provider calls = %d, want %d", len(transport.requests), sent+1)
	}
	body := transport.requests[len(transport.requests)-1].Body
	if sha, size := note.RequestIdentity(body); sha != preview.RequestSHA256 || size != preview.RequestBytes {
		t.Fatalf("the refinement sent %s/%d, the preview showed %s/%d", sha, size, preview.RequestSHA256, preview.RequestBytes)
	}
	if refined.State != "advisory" || !refined.Advisory || refined.DraftID != draft.ID || refined.RefinementID == 0 ||
		refined.Provider != "alpha" || refined.Model != "alpha-text" || refined.RequestSHA256 != preview.RequestSHA256 {
		t.Fatalf("refinement = %+v", refined)
	}
	if refined.Suggestion.Front != "closes at 4 p.m." || !strings.Contains(refined.Suggestion.Back, "Example: The library") ||
		refined.Suggestion.Tactic != draft.Fields.Tactic {
		t.Fatalf("suggestion = %+v", refined.Suggestion)
	}
	after, err := f.w.RefineNotePreview(draft.ID)
	mustNoErr(t, err)
	if after.Budget.Calls != callsBefore+1 {
		t.Fatalf("budget calls %d -> %d", callsBefore, after.Budget.Calls)
	}
	// The window's own fields stay the learner's: previewing them is not the
	// suggestion.
	kept, err := f.w.PreviewNote(draft.ID, draft.Fields)
	mustNoErr(t, err)
	if kept.Fields.Front != draft.Fields.Front || strings.Contains(kept.HTML, "closes at 4 p.m.</div>") {
		t.Fatalf("the suggestion replaced the learner's fields: %+v", kept.Fields)
	}

	// Typed failures are the CLI's: a provider error is recorded and spends
	// the slot; a malformed reply is invalid_response; no key refuses first.
	transport.response = fixedTutorError("rate_limited", "slow down")
	if _, err := f.w.RefineNote(draft.ID); studioCode(err) != "rate_limited" {
		t.Fatalf("provider failure = %v", err)
	}
	transport.response = fixedTutorResponse("no JSON here")
	if _, err := f.w.RefineNote(draft.ID); studioCode(err) != "invalid_response" {
		t.Fatalf("malformed reply = %v", err)
	}
	failed, err := f.w.RefineNotePreview(draft.ID)
	mustNoErr(t, err)
	if failed.Budget.Calls != callsBefore+3 {
		t.Fatalf("recorded failures did not spend the budget: %d -> %d", callsBefore, failed.Budget.Calls)
	}
	delete(f.env, provider.CredentialEnvFor("alpha"))
	sent = len(transport.requests)
	if _, err := f.w.RefineNote(draft.ID); studioCode(err) != "no_credential" || len(transport.requests) != sent {
		t.Fatalf("refine without a key = %v (calls %d -> %d)", err, sent, len(transport.requests))
	}
	if _, err := f.w.RefineNote(draft.ID + 99); studioCode(err) != "unknown_draft" {
		t.Fatalf("unknown draft = %v", err)
	}

	// The learner applies the suggestion explicitly.
	accepted, err := f.w.AcceptNote(draft.ID, refined.Suggestion)
	mustNoErr(t, err)
	if accepted.Note.Origin != "refined" || accepted.Note.Fields.Front != "closes at 4 p.m." {
		t.Fatalf("accept of the suggestion = %+v", accepted)
	}
	if listAttempts(t, f.db) != 1 {
		t.Fatal("a refinement changed attempts")
	}
	for _, value := range []any{preview, refined} {
		raw, _ := json.Marshal(value)
		if strings.Contains(string(raw), fakeTutorKey) {
			t.Fatalf("a document carries the key: %s", raw)
		}
	}
}

// TestWorkspaceRefineNoteRespectsConsent: a private source's draft shows no
// source text before the grant and the refinement refuses before any call.
func TestWorkspaceRefineNoteRespectsConsent(t *testing.T) {
	t.Parallel()
	transport := &fakeTutorTransport{response: fixedTutorResponse("OK")}
	f := newWSFixture(t, func(o *WorkspaceOptions) { o.TutorTransport = transport })
	f.env[provider.RegistryEnv] = modelRegistryFile(t, modelProvider("alpha", "http://127.0.0.1:9/v1/chat/completions"))
	f.env[provider.CredentialEnvFor("alpha")] = fakeTutorKey
	f.reopen(t)
	clock := &stepClock{base: goldenBase}
	sourceID := noteSourceFixture(t, f.db, clock)
	mustRun(t, f.db, clock, "", "practice", "--source", sourceID, "--page", "1", "--question", "q2", "--answer", "A", "--json")
	draft, err := f.w.ProposeNote(studio.NoteRequest{AttemptID: 1})
	mustNoErr(t, err)
	_, err = f.w.UseModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	_, err = f.w.TestModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	sent := len(transport.requests)

	preview, err := f.w.RefineNotePreview(draft.ID)
	mustNoErr(t, err)
	if !preview.Consent.Required || preview.Consent.Granted || preview.Consent.SourceID != sourceID ||
		len(preview.Messages) != 0 || preview.RequestSHA256 != "" || !slices.Contains(preview.Consent.Withheld, "passage") {
		t.Fatalf("withheld preview = %+v", preview)
	}
	raw, _ := json.Marshal(preview)
	if strings.Contains(string(raw), "Westbridge") {
		t.Fatalf("the withheld preview shows source text: %s", raw)
	}
	if _, err := f.w.RefineNote(draft.ID); studioCode(err) != "consent_required" || len(transport.requests) != sent {
		t.Fatalf("refine before consent = %v", err)
	}
	// A grant for another provider covers the note text, not this provider.
	_, err = f.w.GrantConsent(sourceID, "")
	mustNoErr(t, err)
	_, err = f.w.RevokeConsent(sourceID, "alpha")
	mustNoErr(t, err)
	scoped, err := f.w.RefineNotePreview(draft.ID)
	mustNoErr(t, err)
	if scoped.Consent.Granted || len(scoped.Messages) != 0 {
		t.Fatalf("provider-scoped denial = %+v", scoped.Consent)
	}
	if _, err := f.w.RefineNote(draft.ID); studioCode(err) != "consent_required" || len(transport.requests) != sent {
		t.Fatalf("refine under a provider denial = %v", err)
	}
	_, err = f.w.GrantConsent(sourceID, "alpha")
	mustNoErr(t, err)
	granted, err := f.w.RefineNotePreview(draft.ID)
	mustNoErr(t, err)
	if !granted.Consent.Granted || !granted.Consent.SourceTextShared || len(granted.Messages) != 2 || granted.RequestSHA256 == "" {
		t.Fatalf("granted preview = %+v", granted)
	}
}

// TestWorkspaceCoachHistory lists an attempt's calls with each advisory
// reply's text and each failure's code, and nothing that was sent.
func TestWorkspaceCoachHistory(t *testing.T) {
	t.Parallel()
	transport := &fakeTutorTransport{response: fixedTutorResponse("OK")}
	f := newWSFixture(t, func(o *WorkspaceOptions) { o.TutorTransport = transport })
	f.env[provider.RegistryEnv] = modelRegistryFile(t, modelProvider("alpha", "http://127.0.0.1:9/v1/chat/completions"))
	f.env[provider.CredentialEnvFor("alpha")] = fakeTutorKey
	f.reopen(t)
	if _, err := f.w.CoachHistory(0); studioCode(err) != "invalid_input" {
		t.Fatalf("history without an attempt = %v", err)
	}
	if _, err := f.w.CoachHistory(7); studioCode(err) != "no_attempts" {
		t.Fatalf("history on an empty database = %v", err)
	}
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample})
	mustNoErr(t, err)
	feedback, err := f.w.Answer(item.Token, "A")
	mustNoErr(t, err)
	empty, err := f.w.CoachHistory(feedback.AttemptID)
	mustNoErr(t, err)
	if empty.AttemptID != feedback.AttemptID || empty.Calls == nil || len(empty.Calls) != 0 || len(empty.Limits) == 0 {
		t.Fatalf("empty history = %+v", empty)
	}
	_, err = f.w.UseModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	_, err = f.w.TestModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	transport.response = fixedTutorResponse("Read the <b>second</b> sentence again.")
	reply, err := f.w.CoachAsk(feedback.AttemptID, studio.CoachExplain)
	mustNoErr(t, err)
	transport.response = fixedTutorError("rate_limited", "slow down")
	failed, err := f.w.CoachAsk(feedback.AttemptID, studio.CoachProbe)
	mustNoErr(t, err)

	history, err := f.w.CoachHistory(feedback.AttemptID)
	mustNoErr(t, err)
	if len(history.Calls) != 2 {
		t.Fatalf("history = %+v", history)
	}
	first, second := history.Calls[0], history.Calls[1]
	if first.CallID != reply.CallID || first.State != "advisory" || first.Text != "Read the <b>second</b> sentence again." ||
		first.Mode != studio.CoachExplain || first.Provider != "alpha" || first.Model != "alpha-text" ||
		!first.Advisory || first.VerifiedAnswer || first.Failure != nil || first.CreatedAt == "" {
		t.Fatalf("advisory call = %+v", first)
	}
	if second.CallID != failed.CallID || second.State != "failed" || second.Text != "" || second.Mode != studio.CoachProbe ||
		second.Failure == nil || second.Failure.Code != failed.Failure.Code {
		t.Fatalf("failed call = %+v", second)
	}
	raw, _ := json.Marshal(history)
	for _, leak := range []string{fakeTutorKey, `"messages"`, `"body"`, "request_sha256", "\"system\""} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("history carries %s: %s", leak, raw)
		}
	}
	if _, err := f.w.CoachHistory(feedback.AttemptID + 5); studioCode(err) != "unknown_attempt" {
		t.Fatalf("unknown attempt = %v", err)
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
	f := newWSFixture(t, func(o *WorkspaceOptions) { seedAssessmentSource(t, o.DBPath, 22, 1) })
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
	f := newWSFixture(t, func(o *WorkspaceOptions) { seedAssessmentSource(t, o.DBPath, 22, 1) })
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

// TestWorkspaceRefinementStaysAdvisory: a stored refinement is a suggestion
// beside the draft, never the draft's own fields. After a restart the window
// restores the deterministic fields with the suggestion apart; an emptied
// field falls back to the deterministic value; accepting the draft's own
// fields records a deterministic note, and only the suggestion itself records
// a refined one.
// gatedTutorTransport answers like fakeTutorTransport; with a gate set, a call
// reports that it started and then waits for the gate before it replies.
type gatedTutorTransport struct {
	fakeTutorTransport
	started chan struct{}
	gate    chan struct{}
}

func (g *gatedTutorTransport) Call(ctx context.Context, req tutor.HostRequest) (tutor.HostResponse, error) {
	if g.gate != nil {
		close(g.started)
		<-g.gate
	}
	return g.fakeTutorTransport.Call(ctx, req)
}

// TestWorkspaceRefinementDoesNotUndoAnAccept: a refinement that is still
// waiting on the provider when the learner accepts the draft must not erase
// that acceptance when its reply lands. The paid call stays recorded; the
// draft keeps the accepted fields, so a restart does not bring it back.
func TestWorkspaceRefinementDoesNotUndoAnAccept(t *testing.T) {
	transport := &gatedTutorTransport{fakeTutorTransport: fakeTutorTransport{response: fixedTutorResponse("OK")}}
	f := newWSFixture(t, func(o *WorkspaceOptions) { o.TutorTransport = transport })
	f.env[provider.RegistryEnv] = modelRegistryFile(t, modelProvider("alpha", "http://127.0.0.1:9/v1/chat/completions"))
	f.env[provider.CredentialEnvFor("alpha")] = fakeTutorKey
	f.reopen(t)
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample, QuestionID: "q2"})
	mustNoErr(t, err)
	feedback, err := f.w.Answer(item.Token, "A")
	mustNoErr(t, err)
	draft, err := f.w.ProposeNote(studio.NoteRequest{AttemptID: feedback.AttemptID})
	mustNoErr(t, err)
	_, err = f.w.UseModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	_, err = f.w.TestModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	before, err := f.w.RefineNotePreview(draft.ID)
	mustNoErr(t, err)

	transport.response = fixedTutorResponse(`{"front": "closes at 4 p.m.", "back": "To shut at a stated time.\n\nExample: The library closes at 4 p.m."}`)
	transport.started, transport.gate = make(chan struct{}), make(chan struct{})
	type outcome struct {
		refinement studio.NoteRefinement
		err        error
	}
	done := make(chan outcome, 1)
	go func() {
		refinement, err := f.w.RefineNote(draft.ID)
		done <- outcome{refinement, err}
	}()
	select {
	case <-transport.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the refinement never reached the provider")
	}
	// One paid request per draft at a time, whatever the window remembers.
	if _, err := f.w.RefineNote(draft.ID); studioCode(err) != "refine_in_progress" {
		t.Fatalf("a second refinement while the first waits = %v; want refine_in_progress", err)
	}
	accepted, err := f.w.AcceptNote(draft.ID, draft.Fields)
	mustNoErr(t, err)
	close(transport.gate)
	var result outcome
	select {
	case result = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the refinement did not return after its reply")
	}
	if studioCode(result.err) != "draft_changed" {
		t.Fatalf("a refinement that finished after the accept = %+v, %v; want draft_changed", result.refinement, result.err)
	}

	f.reopen(t)
	open, err := f.w.OpenNoteDraft()
	mustNoErr(t, err)
	if open != nil {
		t.Fatalf("after a restart the accepted draft is pending again: %+v", open)
	}
	st, err := store.Open(f.db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	stored, found, err := st.NoteDraftByID(draft.ID)
	if err != nil || !found {
		t.Fatalf("read draft = %+v, %t, %v", stored, found, err)
	}
	if stored.AcceptedFront != accepted.Note.Fields.Front || stored.AcceptedBack != accepted.Note.Fields.Back ||
		stored.RefinedFront != "" || stored.Origin != "deterministic" {
		t.Fatalf("the late refinement changed the accepted draft: %+v", stored)
	}
	records, err := st.NoteRefinements()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Status != "ok" || records[0].DraftID != draft.ID {
		t.Fatalf("the provider call is not recorded: %+v", records)
	}
	after, err := f.w.RefineNotePreview(draft.ID)
	mustNoErr(t, err)
	if after.Budget.Calls != before.Budget.Calls+1 {
		t.Fatalf("budget calls %d -> %d, want the paid call counted", before.Budget.Calls, after.Budget.Calls)
	}
}

func TestWorkspaceRefinementStaysAdvisory(t *testing.T) {
	t.Parallel()
	transport := &fakeTutorTransport{response: fixedTutorResponse("OK")}
	f := newWSFixture(t, func(o *WorkspaceOptions) { o.TutorTransport = transport })
	f.env[provider.RegistryEnv] = modelRegistryFile(t, modelProvider("alpha", "http://127.0.0.1:9/v1/chat/completions"))
	f.env[provider.CredentialEnvFor("alpha")] = fakeTutorKey
	f.reopen(t)
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample, QuestionID: "q2"})
	mustNoErr(t, err)
	feedback, err := f.w.Answer(item.Token, "A")
	mustNoErr(t, err)
	draft, err := f.w.ProposeNote(studio.NoteRequest{AttemptID: feedback.AttemptID})
	mustNoErr(t, err)
	_, err = f.w.UseModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	_, err = f.w.TestModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	transport.response = fixedTutorResponse(`{"front": "closes at 4 p.m.", "back": "To shut at a stated time.\n\nExample: The library closes at 4 p.m."}`)
	refined, err := f.w.RefineNote(draft.ID)
	mustNoErr(t, err)

	f.reopen(t)
	open, err := f.w.OpenNoteDraft()
	mustNoErr(t, err)
	if open == nil || open.ID != draft.ID || open.Fields != draft.Fields || open.Origin != "deterministic" || open.Advisory {
		t.Fatalf("restored draft = %+v, want the deterministic fields %+v", open, draft.Fields)
	}
	if open.Suggestion == nil || open.Suggestion.Suggestion != refined.Suggestion || open.Suggestion.RefinementID != refined.RefinementID ||
		!open.Suggestion.Advisory || open.Suggestion.Provider != "alpha" || open.Suggestion.Model != "alpha-text" {
		t.Fatalf("restored suggestion = %+v, want %+v", open.Suggestion, refined)
	}

	own, err := f.w.PreviewNote(draft.ID, draft.Fields)
	mustNoErr(t, err)
	if own.Fields != draft.Fields || own.Edited {
		t.Fatalf("preview of the draft's own fields = %+v (edited %v)", own.Fields, own.Edited)
	}
	used, err := f.w.PreviewNote(draft.ID, refined.Suggestion)
	mustNoErr(t, err)
	if used.Fields != refined.Suggestion || used.Edited {
		t.Fatalf("preview of the used suggestion = %+v (edited %v)", used.Fields, used.Edited)
	}
	emptied, err := f.w.PreviewNote(draft.ID, studio.NoteFields{Front: "My own front", Tactic: draft.Fields.Tactic})
	mustNoErr(t, err)
	if emptied.Fields.Back != draft.Fields.Back || !emptied.Edited {
		t.Fatalf("an emptied back became %q, want the draft's %q", emptied.Fields.Back, draft.Fields.Back)
	}

	accepted, err := f.w.AcceptNote(draft.ID, draft.Fields)
	mustNoErr(t, err)
	if accepted.Note.Origin != "deterministic" || accepted.Note.Fields.Front != draft.Fields.Front ||
		accepted.Note.Fields.Back != draft.Fields.Back {
		t.Fatalf("accepting the draft's own fields recorded %+v", accepted.Note)
	}
	again, err := f.w.AcceptNote(draft.ID, refined.Suggestion)
	mustNoErr(t, err)
	if again.Note.Origin != "edited" || again.Note.Fields.Front != refined.Suggestion.Front {
		t.Fatalf("a later accept of the suggestion over the accepted fields = %+v", again.Note)
	}
}
