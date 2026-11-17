package store

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
)

func TestAppendListAndReopen(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "attempts.sqlite")
	st := mustOpen(t, path)

	first := mustAppend(t, st, sampleAttempt("q1"))
	if first.ID != 1 || first.PriorAttempts != 0 || first.AttemptNumber != 1 {
		t.Fatalf("first append = id %d prior %d number %d", first.ID, first.PriorAttempts, first.AttemptNumber)
	}

	second := sampleAttempt("q1")
	second.Status = attempt.StatusIncorrect
	second.SelectedLabel = "A"
	second.SelectedText = "At 7 p.m."
	second = mustAppend(t, st, second)
	if second.ID != 2 || second.PriorAttempts != 1 || second.AttemptNumber != 2 {
		t.Fatalf("second append = id %d prior %d number %d", second.ID, second.PriorAttempts, second.AttemptNumber)
	}

	skipped := sampleAttempt("q2")
	skipped.Status = attempt.StatusSkipped
	skipped.SelectedLabel = ""
	skipped.SelectedText = ""
	skipped = mustAppend(t, st, skipped)
	if skipped.AttemptNumber != 1 {
		t.Fatalf("q2 attempt number = %d, want 1", skipped.AttemptNumber)
	}

	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	list, err := reopened.List()
	if err != nil {
		t.Fatalf("List after reopen: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("attempts after reopen = %d, want 3", len(list))
	}
	got := list[1]
	if got.Status != attempt.StatusIncorrect || got.SelectedLabel != "A" || got.SelectedText != "At 7 p.m." {
		t.Fatalf("round-trip attempt = %+v", got)
	}
	if got.AttemptNumber != 2 || got.PriorAttempts != 1 {
		t.Fatalf("round-trip numbering = %d/%d", got.AttemptNumber, got.PriorAttempts)
	}
	if !got.StartedAt.Equal(sampleAttempt("q1").StartedAt) {
		t.Fatalf("started_at = %s", got.StartedAt)
	}
	if list[2].SelectedLabel != "" || list[2].SelectedText != "" {
		t.Fatalf("skipped round-trip has selection %q/%q", list[2].SelectedLabel, list[2].SelectedText)
	}

	third := mustAppend(t, reopened, sampleAttempt("q1"))
	if third.AttemptNumber != 3 || third.PriorAttempts != 2 {
		t.Fatalf("append after reopen = %d/%d", third.AttemptNumber, third.PriorAttempts)
	}
}

func TestNumberingSpansContentVersions(t *testing.T) {
	t.Parallel()
	st := mustOpen(t, filepath.Join(t.TempDir(), "attempts.sqlite"))
	first := mustAppend(t, st, sampleAttempt("q1"))
	if first.AttemptNumber != 1 || first.PriorAttempts != 0 {
		t.Fatalf("first version numbering = %d/%d", first.AttemptNumber, first.PriorAttempts)
	}

	nextVersion := sampleAttempt("q1")
	nextVersion.ContentVersion = "2"
	second := mustAppend(t, st, nextVersion)
	if second.AttemptNumber != 2 || second.PriorAttempts != 1 {
		t.Fatalf("second version numbering = %d/%d", second.AttemptNumber, second.PriorAttempts)
	}

	list, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("attempts = %d, want 2", len(list))
	}
	if list[1].ContentVersion != "2" || list[1].AttemptNumber != 2 || list[1].PriorAttempts != 1 {
		t.Fatalf("round-trip second version = %+v", list[1])
	}
}

func TestNumberingScopesToContent(t *testing.T) {
	t.Parallel()
	st := mustOpen(t, filepath.Join(t.TempDir(), "attempts.sqlite"))

	first := mustAppend(t, st, sampleAttempt("q1"))
	if first.AttemptNumber != 1 || first.PriorAttempts != 0 {
		t.Fatalf("first content numbering = %d/%d", first.AttemptNumber, first.PriorAttempts)
	}

	otherContent := sampleAttempt("q1")
	otherContent.ContentID = "sample-9999"
	other := mustAppend(t, st, otherContent)
	if other.AttemptNumber != 1 || other.PriorAttempts != 0 || other.Repeat() {
		t.Fatalf("other content numbering = %d/%d repeat %t", other.AttemptNumber, other.PriorAttempts, other.Repeat())
	}

	sameQuestionAgain := mustAppend(t, st, sampleAttempt("q1"))
	if sameQuestionAgain.AttemptNumber != 2 || sameQuestionAgain.PriorAttempts != 1 {
		t.Fatalf("repeat in first content = %d/%d", sameQuestionAgain.AttemptNumber, sameQuestionAgain.PriorAttempts)
	}

	list, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []int{1, 1, 2}
	if len(list) != len(want) {
		t.Fatalf("attempts = %d, want %d", len(list), len(want))
	}
	for i, number := range want {
		if list[i].AttemptNumber != number {
			t.Fatalf("attempt %d number = %d, want %d", i+1, list[i].AttemptNumber, number)
		}
	}
}

func TestAttemptsAreAppendOnly(t *testing.T) {
	t.Parallel()
	st := mustOpen(t, filepath.Join(t.TempDir(), "attempts.sqlite"))
	mustAppend(t, st, sampleAttempt("q1"))

	if _, err := st.db.Exec(`UPDATE attempts SET status = 'incorrect'`); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("UPDATE error = %v, want append-only rejection", err)
	}
	if _, err := st.db.Exec(`DELETE FROM attempts`); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("DELETE error = %v, want append-only rejection", err)
	}
	if _, err := st.db.Exec(replaceAttemptSQL); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("INSERT OR REPLACE error = %v, want append-only rejection", err)
	}
	list, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("attempts after rejected writes = %d, want 1", len(list))
	}
	if list[0].Status != attempt.StatusCorrect || list[0].SelectedLabel != "B" {
		t.Fatalf("rejected replace changed the row: %+v", list[0])
	}
}

func TestAppendRejectsInconsistentRecords(t *testing.T) {
	t.Parallel()
	st := mustOpen(t, filepath.Join(t.TempDir(), "attempts.sqlite"))

	tests := []struct {
		name   string
		mutate func(*attempt.Attempt)
		want   string
	}{
		{"empty session", func(a *attempt.Attempt) { a.SessionID = "" }, "session_id"},
		{"invalid status", func(a *attempt.Attempt) { a.Status = "maybe" }, "status"},
		{"correct with wrong label", func(a *attempt.Attempt) { a.SelectedLabel = "C" }, "but key is"},
		{"incorrect with key label", func(a *attempt.Attempt) {
			a.Status = attempt.StatusIncorrect
		}, "selected the key"},
		{"skip with selection", func(a *attempt.Attempt) { a.Status = attempt.StatusSkipped }, "has a selection"},
		{"negative elapsed", func(a *attempt.Attempt) { a.ElapsedMS = -1 }, "negative"},
		{"zero timestamp", func(a *attempt.Attempt) { a.StartedAt = time.Time{} }, "timestamps"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := sampleAttempt("q1")
			tt.mutate(&rec)
			if _, err := st.Append(rec); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Append error = %v, want %q", err, tt.want)
			}
		})
	}
	list, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("invalid appends stored %d records", len(list))
	}
}

func TestAppendPersistsAssistanceAndTactic(t *testing.T) {
	t.Parallel()
	st := mustOpen(t, filepath.Join(t.TempDir(), "assistance.sqlite"))

	assisted := sampleAttempt("q1")
	assisted, err := assisted.WithTactic("not-except-polarity", attempt.AssistanceAssisted)
	if err != nil {
		t.Fatalf("WithTactic assisted: %v", err)
	}
	got := mustAppend(t, st, assisted)
	if got.Assistance != attempt.AssistanceAssisted || got.TacticID != "not-except-polarity" {
		t.Fatalf("assisted record = %+v", got)
	}

	independent := sampleAttempt("q2")
	independent, err = independent.WithTactic("not-except-polarity", attempt.AssistanceIndependent)
	if err != nil {
		t.Fatalf("WithTactic independent: %v", err)
	}
	if _, err := st.Append(independent); err != nil {
		t.Fatalf("Append independent tactic: %v", err)
	}

	ordinary := sampleAttempt("q3")
	ordinary.Assistance = ""
	if _, err := st.Append(ordinary); err != nil {
		t.Fatalf("Append ordinary: %v", err)
	}

	list, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("attempts = %d, want 3", len(list))
	}
	if list[0].Assistance != attempt.AssistanceAssisted || list[0].TacticID != "not-except-polarity" {
		t.Fatalf("stored assisted = %+v", list[0])
	}
	if list[1].Assistance != attempt.AssistanceIndependent || list[1].TacticID != "not-except-polarity" {
		t.Fatalf("stored independent = %+v", list[1])
	}
	if list[2].Assistance != attempt.AssistanceIndependent || list[2].TacticID != "" {
		t.Fatalf("stored ordinary = %+v", list[2])
	}

	assistedNoTactic := sampleAttempt("q4")
	assistedNoTactic, err = assistedNoTactic.WithAssistance(attempt.AssistanceAssisted)
	if err != nil {
		t.Fatalf("WithAssistance assisted: %v", err)
	}
	got = mustAppend(t, st, assistedNoTactic)
	if got.Assistance != attempt.AssistanceAssisted || got.TacticID != "" {
		t.Fatalf("assisted ordinary record = %+v", got)
	}
	badAssistance := sampleAttempt("q5")
	badAssistance.Assistance = "guessed"
	if _, err := st.Append(badAssistance); err == nil || !strings.Contains(err.Error(), "assistance") {
		t.Fatalf("invalid assistance error = %v", err)
	}
}

func TestAppendPersistsSkillSnapshot(t *testing.T) {
	t.Parallel()
	st := mustOpen(t, filepath.Join(t.TempDir(), "skills.sqlite"))

	tagged := sampleAttempt("q1")
	tagged.Skills = []string{"vocabulary-in-context", "sentence-structure"}
	got := mustAppend(t, st, tagged)
	if !slices.Equal(got.Skills, tagged.Skills) {
		t.Fatalf("append returned skills %v, want %v", got.Skills, tagged.Skills)
	}

	ordinary := sampleAttempt("q2")
	if _, err := st.Append(ordinary); err != nil {
		t.Fatalf("Append ordinary: %v", err)
	}
	list, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("attempts = %d, want 2", len(list))
	}
	if !slices.Equal(list[0].Skills, tagged.Skills) {
		t.Fatalf("stored skills = %v, want %v", list[0].Skills, tagged.Skills)
	}
	if len(list[1].Skills) != 0 {
		t.Fatalf("untagged stored skills = %v, want empty", list[1].Skills)
	}
}

func TestAppendRejectsBadSkillSnapshot(t *testing.T) {
	t.Parallel()
	st := mustOpen(t, filepath.Join(t.TempDir(), "bad-skills.sqlite"))
	tests := []struct {
		name   string
		skills []string
		want   string
	}{
		{"bad slug", []string{"Vocabulary In Context"}, "lowercase slug"},
		{"duplicate", []string{"vocabulary-in-context", "vocabulary-in-context"}, "duplicated"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := sampleAttempt("q1")
			rec.Skills = tt.skills
			if _, err := st.Append(rec); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Append error = %v, want %q", err, tt.want)
			}
		})
	}
	list, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("invalid skill appends stored %d records", len(list))
	}
}

func TestRecordExposureIsIdempotentAndAppendOnly(t *testing.T) {
	t.Parallel()
	st := mustOpen(t, filepath.Join(t.TempDir(), "exposure.sqlite"))
	at := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	exposure := Exposure{
		TacticID:   "not-except-polarity",
		ContentID:  "tactic-first-party-not-except",
		QuestionID: "t2",
		ExposedAt:  at,
	}
	if err := st.RecordExposure(exposure); err != nil {
		t.Fatalf("RecordExposure: %v", err)
	}
	if err := st.RecordExposure(exposure); err != nil {
		t.Fatalf("repeated RecordExposure: %v", err)
	}
	list, err := st.ListExposures()
	if err != nil {
		t.Fatalf("ListExposures: %v", err)
	}
	if len(list) != 1 || list[0].Key() != exposure.Key() || !list[0].ExposedAt.Equal(at) {
		t.Fatalf("exposures = %+v, want one observation of %+v", list, exposure)
	}
	exposed, err := st.Exposed(exposure.Key())
	if err != nil {
		t.Fatalf("Exposed: %v", err)
	}
	if !exposed {
		t.Fatal("Exposed reports a recorded question as unexposed")
	}
	other, err := st.Exposed(attempt.QuestionKey{ContentID: exposure.ContentID, QuestionID: "t3"})
	if err != nil {
		t.Fatalf("Exposed other question: %v", err)
	}
	if other {
		t.Fatal("Exposed reports another question as exposed")
	}

	if _, err := st.db.Exec(`UPDATE tactic_exposures SET question_id = 't1'`); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("UPDATE exposure error = %v, want append-only rejection", err)
	}
	if _, err := st.db.Exec(`DELETE FROM tactic_exposures`); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("DELETE exposure error = %v, want append-only rejection", err)
	}

	badTactic := exposure
	badTactic.TacticID = "Not A Slug"
	if err := st.RecordExposure(badTactic); err == nil || !strings.Contains(err.Error(), "tactic_id") {
		t.Fatalf("invalid tactic error = %v", err)
	}
	badContent := exposure
	badContent.ContentID = " "
	if err := st.RecordExposure(badContent); err == nil || !strings.Contains(err.Error(), "content_id") {
		t.Fatalf("empty content error = %v", err)
	}
}
