package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/studio"
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
	if independent.Stage != stageIndependent || independent.Condition != studio.ConditionIndependent || !independent.CanCount || !independent.HintAvailable || independent.Hint != nil {
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
	f := newWSFixture(t, nil)
	seedAssessmentSource(t, f.db, 22, 1)
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
	f := newWSFixture(t, nil)
	seedAssessmentSource(t, f.db, 20, 1)
	state, err := f.w.StartAssessment(2, 1)
	mustNoErr(t, err)
	f.clock.Advance(2 * time.Minute)
	// The reconcile every assess call runs first records the expired window,
	// exactly as a CLI assess command does, so the late answer is refused.
	if _, err := f.w.AnswerAssessmentItem(state.Run.ID, 0, "A"); studioCode(err) != "run_state" {
		t.Fatalf("late answer after window = %v", err)
	}
	report, err := f.w.AssessmentReport(state.Run.ID)
	mustNoErr(t, err)
	if report.Run.Status != assess.StatusAbandoned || report.Run.AbandonedReason != "window_elapsed" {
		t.Fatalf("expired run = %+v", report.Run)
	}
	if _, err := f.w.ResumeAssessment(state.Run.ID); studioCode(err) != "run_abandoned" {
		t.Fatalf("resume abandoned = %v", err)
	}
}

func TestWorkspaceCredentialStoreLayering(t *testing.T) {
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
	anki := newFakeAnki()
	f := newWSFixture(t, func(o *WorkspaceOptions) { o.AnkiTransport = anki })
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample, QuestionID: "q2"})
	mustNoErr(t, err)
	feedback, err := f.w.Answer(item.Token, "A")
	mustNoErr(t, err)
	draft, err := f.w.ProposeNote(studio.NoteRequest{AttemptID: feedback.AttemptID})
	mustNoErr(t, err)
	if draft.ID == 0 || draft.Fields.Front == "" || draft.Target.AttemptID != feedback.AttemptID {
		t.Fatalf("draft = %+v", draft)
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
	mustNoErr(t, f.w.SkipNote(second.ID))
	if _, err := f.w.PreviewNote(second.ID, studio.NoteFields{}); studioCode(err) != "draft_closed" {
		t.Fatalf("preview of skipped draft = %v", err)
	}
	if _, err := f.w.ProposeNote(studio.NoteRequest{AttemptID: feedback.AttemptID, Mark: strings.Repeat("x", 200)}); studioCode(err) != "invalid_mark" {
		t.Fatalf("over-long mark = %v", err)
	}
}

func TestWorkspaceBriefWithoutRun(t *testing.T) {
	f := newWSFixture(t, nil)
	status, err := f.w.BriefStatus()
	mustNoErr(t, err)
	if status.Found || status.State != "none" {
		t.Fatalf("brief status = %+v", status)
	}
}
