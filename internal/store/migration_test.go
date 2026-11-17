package store

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
)

func TestMigrationFromVersionFiveAddsSkillSnapshot(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "v5.sqlite")
	db := openSQLite(t, path)
	for _, statements := range [][]string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5} {
		for _, stmt := range statements {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("create v5 schema: %v", err)
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
		t.Fatalf("insert v5 attempt: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 5`); err != nil {
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
	if len(list) != 1 || list[0].TacticID != "not-except-polarity" || len(list[0].Skills) != 0 || list[0].SkillsRecorded {
		t.Fatalf("v5 attempt after migration = %+v, want preserved tactic and no recorded snapshot", list)
	}

	next := sampleAttempt("q2")
	next.Skills = []string{"vocabulary-in-context"}
	if _, err := st.Append(next); err != nil {
		t.Fatalf("Append after migration: %v", err)
	}
	untagged := sampleAttempt("q3")
	if _, err := st.Append(untagged); err != nil {
		t.Fatalf("Append untagged after migration: %v", err)
	}
	list, err = st.List()
	if err != nil {
		t.Fatalf("List after append: %v", err)
	}
	if len(list) != 3 || !slices.Equal(list[1].Skills, next.Skills) || !list[1].SkillsRecorded {
		t.Fatalf("post-migration snapshot = %+v", list)
	}
	if len(list[2].Skills) != 0 || !list[2].SkillsRecorded {
		t.Fatalf("untagged snapshot = %+v, want a recorded empty snapshot", list[2])
	}
}

func TestMigrationKeepsOlderDatabaseAndIsIdempotent(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestMigrationFromVersionFourAddsGuidedExposures(t *testing.T) {
	t.Parallel()
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
