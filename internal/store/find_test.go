package store

import (
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
)

func findQueryFixture(day, status string) FindQuery {
	return FindQuery{
		Day:           day,
		CreatedAt:     time.Date(2026, 11, 11, 8, 0, 0, 0, time.UTC),
		IntentJSON:    `{"selection":"skill_error_ratio"}`,
		SkillID:       "vocabulary-in-context",
		TacticID:      "",
		ExamSection:   "mock-exam",
		LearnerLevel:  "developing",
		Query:         "Vocabulary in context English exam practice mock-exam",
		Provider:      "stub-search",
		EndpointHost:  "127.0.0.1:18768",
		Status:        status,
		MaxCandidates: 20,
		RequestSHA256: strings.Repeat("a", 64),
		RequestBytes:  64,
		HTTPStatus:    200,
		LatencyMS:     4,
	}
}

func findCandidateFixture(classification string) FindCandidate {
	candidate := FindCandidate{
		Provider:       "stub-search",
		Query:          "Vocabulary in context English exam practice mock-exam",
		RetrievedAt:    time.Date(2026, 11, 11, 8, 0, 0, 0, time.UTC),
		Title:          "Candidate " + classification,
		URL:            "https://example.com/" + classification,
		Classification: classification,
		ContentSHA256:  strings.Repeat("b", 64),
	}
	if classification == "local-usable" {
		candidate.Content = "provider returned study text"
		candidate.Displayable = true
		candidate.License = "cc-by-4.0"
	}
	return candidate
}

func TestMigrationFromVersionTwelveAddsFindTables(t *testing.T) {
	path := t.TempDir() + "/v12.sqlite"
	db := openSQLite(t, path)
	for _, statements := range [][]string{
		schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6, schemaV7,
		schemaV8, schemaV9, schemaV10, schemaV11, schemaV12,
	} {
		for _, statement := range statements {
			if _, err := db.Exec(statement); err != nil {
				t.Fatalf("create v12 schema: %v", err)
			}
		}
	}
	at := "2026-11-11T09:00:00Z"
	if _, err := db.Exec(`INSERT INTO attempts (
			session_id, question_id, content_id, content_version, status,
			selected_label, selected_text, correct_label, correct_text,
			started_at, answered_at, elapsed_ms, assistance, tactic_id, skills_json
		) VALUES ('sess', 'q1', 'sample-0001', '1', 'incorrect',
			'A', 'Wrong', 'B', 'Right', ?, ?, 5, 'independent', '', '["vocabulary-in-context"]')`, at, at); err != nil {
		t.Fatalf("insert v12 attempt: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 12`); err != nil {
		t.Fatalf("set v12 version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v12 fixture: %v", err)
	}

	st := mustOpen(t, path)
	var version int
	if err := st.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != SchemaVersion {
		t.Fatalf("user_version = %d, want %d", version, SchemaVersion)
	}
	records, err := st.List()
	if err != nil || len(records) != 1 || records[0].Status != attempt.StatusIncorrect {
		t.Fatalf("attempt after migration = %+v, %v", records, err)
	}
	day := "2026-11-11"
	reservation, _, reserved, err := st.ReserveFindQuery(day, 5, time.Now())
	if err != nil || !reserved {
		t.Fatalf("reserve after migration = %d, %t, %v", reservation, reserved, err)
	}
	query := findQueryFixture(day, "ok")
	query.Provider = "stub-search"
	stored, err := st.AppendFindQuery(query, []FindCandidate{findCandidateFixture("local-usable")}, reservation)
	if err != nil {
		t.Fatalf("append after migration: %v", err)
	}
	if stored.ID == 0 {
		t.Fatalf("stored query = %+v", stored)
	}
	if _, err := st.db.Exec(`UPDATE attempts SET status = 'correct'`); err == nil {
		t.Fatal("attempt update was accepted after v13 migration")
	}
}

func TestFindReservationEnforcesDailyQuota(t *testing.T) {
	st := mustOpen(t, t.TempDir()+"/find.sqlite")
	day := "2026-11-11"
	at := time.Date(2026, 11, 11, 8, 0, 0, 0, time.UTC)

	first, usage, ok, err := st.ReserveFindQuery(day, 2, at)
	if err != nil || !ok || first == 0 || usage.Used() != 1 {
		t.Fatalf("first reservation = %d, %+v, %t, %v", first, usage, ok, err)
	}
	second, usage, ok, err := st.ReserveFindQuery(day, 2, at)
	if err != nil || !ok || second == 0 || usage.Used() != 2 {
		t.Fatalf("second reservation = %d, %+v, %t, %v", second, usage, ok, err)
	}
	third, usage, ok, err := st.ReserveFindQuery(day, 2, at)
	if err != nil || ok || third != 0 || usage.Used() != 2 {
		t.Fatalf("over-quota reservation = %d, %+v, %t, %v", third, usage, ok, err)
	}
	if err := st.ReleaseFindQueryReservation(second); err != nil {
		t.Fatal(err)
	}
	again, usage, ok, err := st.ReserveFindQuery(day, 2, at)
	if err != nil || !ok || again == 0 || usage.Used() != 2 {
		t.Fatalf("reservation after release = %d, %+v, %t, %v", again, usage, ok, err)
	}
}

func TestAppendFindQueryStoresCandidatesAndReleasesReservation(t *testing.T) {
	st := mustOpen(t, t.TempDir()+"/find.sqlite")
	day := "2026-11-11"
	reservation, _, ok, err := st.ReserveFindQuery(day, 5, time.Date(2026, 11, 11, 8, 0, 0, 0, time.UTC))
	if err != nil || !ok {
		t.Fatalf("reserve: %v", err)
	}
	candidates := []FindCandidate{
		findCandidateFixture("local-usable"),
		findCandidateFixture("link-only"),
		findCandidateFixture("generated-unverified"),
	}
	stored, err := st.AppendFindQuery(findQueryFixture(day, "ok"), candidates, reservation)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if stored.ID == 0 || stored.CandidateCount != 3 {
		t.Fatalf("stored = %+v", stored)
	}
	usage, err := st.FindQueryUsage(day)
	if err != nil || usage.Queries != 1 || usage.Reservations != 0 {
		t.Fatalf("usage = %+v, %v", usage, err)
	}
	query, loaded, found, err := st.FindQuery(stored.ID)
	if err != nil || !found {
		t.Fatalf("FindQuery = %+v, %t, %v", query, found, err)
	}
	if query.CandidateCount != 3 || len(loaded) != 3 {
		t.Fatalf("query/candidates = %+v / %d", query, len(loaded))
	}
	if loaded[0].Classification != "local-usable" || loaded[0].Content == "" || !loaded[0].Displayable {
		t.Fatalf("local candidate = %+v", loaded[0])
	}
	for _, candidate := range loaded[1:] {
		if candidate.Content != "" || candidate.Displayable {
			t.Fatalf("non-local candidate carries text: %+v", candidate)
		}
	}
	if loaded[0].ContentSHA256 != strings.Repeat("b", 64) || loaded[0].Provider != "stub-search" {
		t.Fatalf("candidate provenance = %+v", loaded[0])
	}
	if _, err := st.db.Exec(`UPDATE find_queries SET status = 'empty'`); err == nil {
		t.Fatal("find query update was accepted")
	}
	if _, err := st.db.Exec(`DELETE FROM find_candidates`); err == nil {
		t.Fatal("find candidate delete was accepted")
	}
	if _, err := st.db.Exec(`UPDATE find_candidates SET title = 'changed'`); err == nil {
		t.Fatal("find candidate update was accepted")
	}
}

func TestAppendFindQueryRejectsIncoherentRows(t *testing.T) {
	st := mustOpen(t, t.TempDir()+"/find.sqlite")
	day := "2026-11-11"
	cases := []struct {
		name       string
		status     string
		candidates []FindCandidate
	}{
		{"ok without candidates", "ok", nil},
		{"empty with candidates", "empty", []FindCandidate{findCandidateFixture("link-only")}},
		{"all link only with local text", "all_link_only", []FindCandidate{findCandidateFixture("local-usable")}},
		{"failure with candidates", "rate_limited", []FindCandidate{findCandidateFixture("link-only")}},
		{"candidate limit with candidates", "candidate_limit_exceeded", []FindCandidate{findCandidateFixture("link-only")}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			query := findQueryFixture(day, testCase.status)
			if testCase.status != "ok" && testCase.status != "empty" && testCase.status != "all_link_only" {
				query.ErrorCode = "rate_limited"
			}
			if _, err := st.AppendFindQuery(query, testCase.candidates, 0); err == nil {
				t.Fatalf("incoherent row was accepted: %+v", query)
			}
		})
	}
	query := findQueryFixture(day, "ok")
	candidate := findCandidateFixture("local-usable")
	candidate.ContentSHA256 = "short"
	if _, err := st.AppendFindQuery(query, []FindCandidate{candidate}, 0); err == nil {
		t.Fatal("short content hash was accepted")
	}
	query = findQueryFixture(day, "rate_limited")
	query.ErrorCode = ""
	if _, err := st.AppendFindQuery(query, nil, 0); err == nil {
		t.Fatal("failure without an error code was accepted")
	}
}

func TestFindQueryUsageCountsReservationsAndDaysAreSeparate(t *testing.T) {
	st := mustOpen(t, t.TempDir()+"/find.sqlite")
	at := time.Date(2026, 11, 11, 8, 0, 0, 0, time.UTC)
	if _, _, ok, err := st.ReserveFindQuery("2026-11-11", 5, at); err != nil || !ok {
		t.Fatalf("reserve: %v", err)
	}
	usage, err := st.FindQueryUsage("2026-11-11")
	if err != nil || usage.Used() != 1 {
		t.Fatalf("usage = %+v, %v", usage, err)
	}
	other, err := st.FindQueryUsage("2026-11-12")
	if err != nil || other.Used() != 0 {
		t.Fatalf("other day = %+v, %v", other, err)
	}
	if _, err := st.FindQueryUsage("2026-13-11"); err == nil {
		t.Fatal("invalid day was accepted")
	}
}
