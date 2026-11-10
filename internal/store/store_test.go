package store

import (
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
)

func sampleAttempt(questionID string) attempt.Attempt {
	started := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	return attempt.Attempt{
		SessionID:      "sess-test",
		QuestionID:     questionID,
		ContentID:      "sample-0001",
		ContentVersion: "1",
		Status:         attempt.StatusCorrect,
		SelectedLabel:  "B",
		SelectedText:   "At 4 p.m.",
		CorrectLabel:   "B",
		CorrectText:    "At 4 p.m.",
		StartedAt:      started,
		AnsweredAt:     started.Add(1200 * time.Millisecond),
		ElapsedMS:      1200,
	}
}

const replaceAttemptSQL = `INSERT OR REPLACE INTO attempts (
		id, session_id, question_id, content_id, content_version, status,
		selected_label, selected_text, correct_label, correct_text,
		started_at, answered_at, elapsed_ms
	) VALUES (1, 'sess-evil', 'q1', 'sample-0001', '1', 'incorrect',
		'A', 'At 7 p.m.', 'B', 'At 4 p.m.',
		'2026-11-10T09:00:00Z', '2026-11-10T09:00:02Z', 2000)`

func mustOpen(t *testing.T, path string) *Store {
	t.Helper()
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// openSQLite opens a fixture through the same DSN construction as Open, so
// test setup cannot drift to a second path scheme.
func openSQLite(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", sqliteURI(path, runtime.GOOS))
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	return db
}

func mustAppend(t *testing.T, st *Store, rec attempt.Attempt) attempt.Attempt {
	t.Helper()
	got, err := st.Append(rec)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	return got
}

func TestOpenCreatesSchemaAndProtectsFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	path := filepath.Join(dir, "attempts.sqlite")
	st := mustOpen(t, path)

	if got, want := st.Path(), path; got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
	var version int
	if err := st.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != SchemaVersion {
		t.Fatalf("user_version = %d, want %d", version, SchemaVersion)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat database: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("database mode = %04o, want 0600", perm)
		}
		dirInfo, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat directory: %v", err)
		}
		if perm := dirInfo.Mode().Perm(); perm != 0o700 {
			t.Errorf("directory mode = %04o, want 0700", perm)
		}
	}
}

func TestOpenPrivateTightensExistingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		return
	}
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := OpenPrivate(filepath.Join(dir, "attempts.sqlite"))
	if err != nil {
		t.Fatalf("OpenPrivate: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat directory: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("directory mode = %04o, want 0700", perm)
	}
}

func TestAppendListAndReopen(t *testing.T) {
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
	assistedNoTactic.Assistance = attempt.AssistanceAssisted
	if _, err := st.Append(assistedNoTactic); err == nil || !strings.Contains(err.Error(), "no tactic id") {
		t.Fatalf("assisted attempt without a tactic error = %v", err)
	}
	badAssistance := sampleAttempt("q5")
	badAssistance.Assistance = "guessed"
	if _, err := st.Append(badAssistance); err == nil || !strings.Contains(err.Error(), "assistance") {
		t.Fatalf("invalid assistance error = %v", err)
	}
}

func TestMigrationKeepsOlderDatabaseAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	db := openSQLite(t, path)
	if _, err := db.Exec(`CREATE TABLE legacy_notes (body TEXT)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO legacy_notes (body) VALUES ('keep me')`); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 0`); err != nil {
		t.Fatalf("set version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy: %v", err)
	}

	st := mustOpen(t, path)
	var body string
	if err := st.db.QueryRow(`SELECT body FROM legacy_notes`).Scan(&body); err != nil {
		t.Fatalf("legacy table after migration: %v", err)
	}
	if body != "keep me" {
		t.Fatalf("legacy row = %q", body)
	}
	mustAppend(t, st, sampleAttempt("q1"))
	if err := st.Close(); err != nil {
		t.Fatalf("close migrated: %v", err)
	}

	again, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer again.Close()
	var version int
	if err := again.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	if version != SchemaVersion {
		t.Fatalf("user_version = %d, want %d", version, SchemaVersion)
	}
	list, err := again.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("attempts after idempotent migration = %d, want 1", len(list))
	}
}

func TestMigrationAddsInsertGuardToVersionOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1.sqlite")
	db := openSQLite(t, path)
	for _, stmt := range schemaV1 {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create v1 schema: %v", err)
		}
	}
	if _, err := db.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatalf("set version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v1: %v", err)
	}

	st := mustOpen(t, path)
	var version int
	if err := st.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	if version != SchemaVersion {
		t.Fatalf("user_version = %d, want %d", version, SchemaVersion)
	}
	var indexCount int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'attempts_content_question_id'`).Scan(&indexCount); err != nil {
		t.Fatalf("read index: %v", err)
	}
	if indexCount != 1 {
		t.Fatalf("composite index count = %d, want 1", indexCount)
	}

	mustAppend(t, st, sampleAttempt("q1"))
	if _, err := st.db.Exec(replaceAttemptSQL); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("INSERT OR REPLACE after migration error = %v, want append-only rejection", err)
	}
	list, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Status != attempt.StatusCorrect {
		t.Fatalf("attempt after rejected replace = %+v", list)
	}
}

func TestMigrationFromVersionThreeAddsAssistanceColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v3.sqlite")
	db := openSQLite(t, path)
	for _, statements := range [][]string{schemaV1, schemaV2, schemaV3} {
		for _, stmt := range statements {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("create v3 schema: %v", err)
			}
		}
	}
	started := "2026-11-10T09:00:00Z"
	if _, err := db.Exec(`INSERT INTO attempts (
			session_id, question_id, content_id, content_version, status,
			selected_label, selected_text, correct_label, correct_text,
			started_at, answered_at, elapsed_ms
		) VALUES ('sess', 'q1', 'sample-0001', '1', 'correct', 'B', 'At 4 p.m.', 'B', 'At 4 p.m.', ?, ?, 5)`,
		started, started); err != nil {
		t.Fatalf("insert v3 attempt: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 3`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	st := mustOpen(t, path)
	var version int
	if err := st.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != SchemaVersion {
		t.Fatalf("user_version = %d, want %d", version, SchemaVersion)
	}
	list, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("attempts = %d, want 1", len(list))
	}
	if list[0].Assistance != attempt.AssistanceIndependent || list[0].TacticID != "" {
		t.Fatalf("pre-v4 attempt = %+v, want independent with no tactic", list[0])
	}

	// The database enforces the assistance vocabulary even for direct writes.
	if _, err := st.db.Exec(`INSERT INTO attempts (
			session_id, question_id, content_id, content_version, status,
			selected_label, selected_text, correct_label, correct_text,
			started_at, answered_at, elapsed_ms, assistance
		) VALUES ('sess', 'q2', 'sample-0001', '1', 'correct', 'B', 'At 4 p.m.', 'B', 'At 4 p.m.', ?, ?, 5, 'guessed')`,
		started, started); err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Fatalf("invalid assistance insert error = %v, want CHECK failure", err)
	}
}

func TestRecordExposureIsIdempotentAndAppendOnly(t *testing.T) {
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

func TestMigrationFromVersionFourAddsGuidedExposures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v4.sqlite")
	db := openSQLite(t, path)
	for _, statements := range [][]string{schemaV1, schemaV2, schemaV3, schemaV4} {
		for _, stmt := range statements {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("create v4 schema: %v", err)
			}
		}
	}
	at := "2026-11-10T09:00:00Z"
	if _, err := db.Exec(`INSERT INTO attempts (
			session_id, question_id, content_id, content_version, status,
			selected_label, selected_text, correct_label, correct_text,
			started_at, answered_at, elapsed_ms, assistance, tactic_id
		) VALUES ('sess', 't1', 'tactic-first-party-not-except', '1', 'correct',
			'D', 'option d', 'D', 'option d', ?, ?, 5, 'assisted', 'not-except-polarity')`,
		at, at); err != nil {
		t.Fatalf("insert v4 attempt: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 4`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	st := mustOpen(t, path)
	var version int
	if err := st.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != SchemaVersion {
		t.Fatalf("user_version = %d, want %d", version, SchemaVersion)
	}
	list, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Assistance != attempt.AssistanceAssisted || list[0].TacticID != "not-except-polarity" {
		t.Fatalf("v4 attempt after migration = %+v", list)
	}

	exposure := Exposure{
		TacticID:   "not-except-polarity",
		ContentID:  "tactic-first-party-not-except",
		QuestionID: "t2",
		ExposedAt:  time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC),
	}
	if err := st.RecordExposure(exposure); err != nil {
		t.Fatalf("RecordExposure after migration: %v", err)
	}
	exposures, err := st.ListExposures()
	if err != nil {
		t.Fatalf("ListExposures: %v", err)
	}
	if len(exposures) != 1 || exposures[0].QuestionID != "t2" {
		t.Fatalf("exposures after migration = %+v", exposures)
	}
}

func TestOpenRejectsNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.sqlite")
	db := openSQLite(t, path)
	if _, err := db.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatalf("set version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := Open(path); err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("Open error = %v, want newer-schema rejection", err)
	}
}

func TestOpenRejectsEmptyPath(t *testing.T) {
	if _, err := Open("  "); err == nil || !strings.Contains(err.Error(), "empty database path") {
		t.Fatalf("Open error = %v", err)
	}
}

// TestSQLiteURIFormatsPathsForEveryOS pins the DSN for both path families on
// any host. A Windows drive path must not land in the URI authority, which is
// what produced "invalid uri authority: C:..." on windows-latest.
func TestSQLiteURIFormatsPathsForEveryOS(t *testing.T) {
	const options = "?_pragma=busy_timeout%285000%29&_txlock=immediate"
	tests := []struct {
		name     string
		path     string
		goos     string
		want     string
		wantPath string
	}{
		{
			name: "posix", path: "/home/u/db.sqlite", goos: "linux",
			want: "file:///home/u/db.sqlite" + options, wantPath: "/home/u/db.sqlite",
		},
		{
			name: "posix darwin", path: "/Users/u/db.sqlite", goos: "darwin",
			want: "file:///Users/u/db.sqlite" + options, wantPath: "/Users/u/db.sqlite",
		},
		{
			name: "windows drive", path: `C:\Users\runner\db.sqlite`, goos: "windows",
			want: "file:///C:/Users/runner/db.sqlite" + options, wantPath: "/C:/Users/runner/db.sqlite",
		},
		{
			name: "windows drive with forward slashes", path: "C:/Users/runner/db.sqlite", goos: "windows",
			want: "file:///C:/Users/runner/db.sqlite" + options, wantPath: "/C:/Users/runner/db.sqlite",
		},
		{
			name:     "windows import smoke temp path",
			path:     `C:\Users\RUNNER~1\AppData\Local\Temp\passquest-import-smoke-bEarG2\smoke.sqlite`,
			goos:     "windows",
			want:     "file:///C:/Users/RUNNER~1/AppData/Local/Temp/passquest-import-smoke-bEarG2/smoke.sqlite" + options,
			wantPath: "/C:/Users/RUNNER~1/AppData/Local/Temp/passquest-import-smoke-bEarG2/smoke.sqlite",
		},
		{
			// SQLite rejects every authority but "" and "localhost", so a UNC
			// path must keep its leading "\\" in the path component: the DSN
			// needs four slashes, not two.
			name: "windows unc", path: `\\server\share\db.sqlite`, goos: "windows",
			want: "file:////server/share/db.sqlite" + options, wantPath: "//server/share/db.sqlite",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sqliteURI(tt.path, tt.goos)
			if got != tt.want {
				t.Fatalf("sqliteURI(%q, %q) = %q, want %q", tt.path, tt.goos, got, tt.want)
			}
			if strings.Contains(got, "%5C") || strings.Contains(got, `\`) {
				t.Fatalf("URI keeps a backslash: %q", got)
			}
			if strings.Contains(got, "file:C:") {
				t.Fatalf("drive path is in the URI authority: %q", got)
			}
			u, err := url.Parse(got)
			if err != nil {
				t.Fatalf("parse %q: %v", got, err)
			}
			if u.Scheme != "file" || u.Host != "" || u.User != nil {
				t.Fatalf("scheme/authority = %q/%q/%v, want file and no authority", u.Scheme, u.Host, u.User)
			}
			if u.Path != tt.wantPath {
				t.Fatalf("path component = %q, want %q", u.Path, tt.wantPath)
			}
			q := u.Query()
			if q.Get("_pragma") != "busy_timeout(5000)" {
				t.Fatalf("_pragma = %q, want busy_timeout(5000)", q.Get("_pragma"))
			}
			if q.Get("_txlock") != "immediate" {
				t.Fatalf("_txlock = %q, want immediate", q.Get("_txlock"))
			}
		})
	}
}
