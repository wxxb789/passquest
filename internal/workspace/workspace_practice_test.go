package workspace

import (
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/studio"
)

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
	f := newWSFixture(t, func(o *Options) { o.NewSession = nil })
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

func TestWorkspaceEmptyPracticeKindIsSample(t *testing.T) {
	t.Parallel()
	f := newWSFixture(t, nil)
	item, err := f.w.StartPractice(studio.PracticeRequest{})
	mustNoErr(t, err)
	if item.Kind != studio.PracticeSample || item.Passage.Label.Kind != studio.SourceSample {
		t.Fatalf("empty-kind item = %s / %s", item.Kind, item.Passage.Label.Kind)
	}
}
