package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/tutor"
)

func sampleTutorConfig() tutor.Config {
	return tutor.Config{
		Endpoint:        "https://api.example.com/v1/chat/completions",
		Model:           "example-model",
		TimeoutMS:       30000,
		MaxOutputTokens: 350,
		DailyCalls:      20,
	}
}

func sampleTutorCall() TutorCall {
	prompt := 12
	completion := 8
	return TutorCall{
		CreatedAt:        time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC),
		AttemptID:        1,
		Mode:             "explain",
		Status:           "ok",
		ProviderHost:     "api.example.com",
		Model:            "example-model",
		PromptVersion:    "tutor-v1",
		ContentID:        "sample-0001",
		QuestionID:       "q1",
		SourceTextShared: false,
		RequestSHA256:    strings.Repeat("a", 64),
		RequestBytes:     123,
		OutputText:       "short advice",
		OutputChars:      12,
		PromptTokens:     &prompt,
		CompletionTokens: &completion,
		HTTPStatus:       200,
		LatencyMS:        50,
	}
}

func TestMigrationFromVersionSixAddsTutorTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v6.sqlite")
	db := openSQLite(t, path)
	for _, statements := range [][]string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6} {
		for _, stmt := range statements {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("create v6 schema: %v", err)
			}
		}
	}
	at := "2026-11-10T09:00:00Z"
	if _, err := db.Exec(`INSERT INTO attempts (
			session_id, question_id, content_id, content_version, status,
			selected_label, selected_text, correct_label, correct_text,
			started_at, answered_at, elapsed_ms, assistance, tactic_id
		) VALUES ('sess', 'q1', 'sample-0001', '1', 'correct',
			'B', 'At 4 p.m.', 'B', 'At 4 p.m.', ?, ?, 5, 'independent', '')`,
		at, at); err != nil {
		t.Fatalf("insert v6 attempt: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO tactic_exposures (tactic_id, content_id, question_id, exposed_at)
		VALUES ('not-except-polarity', 'sample-0001', 'q1', ?)`, at); err != nil {
		t.Fatalf("insert v6 exposure: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 6`); err != nil {
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
	if len(list) != 1 || list[0].QuestionID != "q1" {
		t.Fatalf("v6 attempt after migration = %+v", list)
	}
	exposures, err := st.ListExposures()
	if err != nil {
		t.Fatalf("ListExposures: %v", err)
	}
	if len(exposures) != 1 || exposures[0].TacticID != "not-except-polarity" ||
		exposures[0].ContentID != "sample-0001" || exposures[0].QuestionID != "q1" {
		t.Fatalf("v6 exposures after migration = %+v", exposures)
	}
	if _, configured, err := st.TutorConfig(); err != nil || configured {
		t.Fatalf("TutorConfig = configured %v, err %v", configured, err)
	}
	if calls, err := st.TutorCalls(0); err != nil || len(calls) != 0 {
		t.Fatalf("TutorCalls = %+v, %v", calls, err)
	}
	if consents, err := st.TutorConsents(); err != nil || len(consents) != 0 {
		t.Fatalf("TutorConsents = %+v, %v", consents, err)
	}

	if err := st.SaveTutorConfig(sampleTutorConfig(), time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SaveTutorConfig after migration: %v", err)
	}
	if _, err := st.AppendTutorConsent(TutorConsentEvent{
		SourceID: "src-test",
		Granted:  true,
		Reviewer: "owner",
		At:       time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendTutorConsent after migration: %v", err)
	}
	if _, err := st.AppendTutorCall(sampleTutorCall()); err != nil {
		t.Fatalf("AppendTutorCall after migration: %v", err)
	}
}

func TestTutorConfigRoundTripAndUpdate(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "config.sqlite"))
	if _, configured, err := st.TutorConfig(); err != nil || configured {
		t.Fatalf("empty TutorConfig = configured %v, err %v", configured, err)
	}
	cfg := sampleTutorConfig()
	at := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	if err := st.SaveTutorConfig(cfg, at); err != nil {
		t.Fatalf("SaveTutorConfig: %v", err)
	}
	got, configured, err := st.TutorConfig()
	if err != nil || !configured {
		t.Fatalf("TutorConfig = %+v, configured %v, err %v", got, configured, err)
	}
	if got != cfg {
		t.Fatalf("TutorConfig = %+v, want %+v", got, cfg)
	}

	cfg.Model = "second-model"
	cfg.DailyCalls = 5
	if err := st.SaveTutorConfig(cfg, at.Add(time.Second)); err != nil {
		t.Fatalf("update TutorConfig: %v", err)
	}
	got, _, err = st.TutorConfig()
	if err != nil || got.Model != "second-model" || got.DailyCalls != 5 {
		t.Fatalf("updated TutorConfig = %+v, err %v", got, err)
	}

	invalid := cfg
	invalid.Endpoint = "http://api.example.com/v1"
	if err := st.SaveTutorConfig(invalid, at); err == nil {
		t.Fatal("invalid config was saved")
	}
}

func TestTutorConsentLatestWinsAndIsAppendOnly(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "consent.sqlite"))
	base := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	grant, err := st.AppendTutorConsent(TutorConsentEvent{SourceID: "src-a", Granted: true, Reviewer: "owner", At: base})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if grant.ID == 0 || !grant.Granted {
		t.Fatalf("grant = %+v", grant)
	}
	if _, err := st.AppendTutorConsent(TutorConsentEvent{SourceID: "src-a", Granted: false, Reviewer: "owner", At: base.Add(time.Second)}); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	latest, ok, err := st.TutorConsent("src-a")
	if err != nil || !ok || latest.Granted {
		t.Fatalf("latest consent = %+v, ok %v, err %v", latest, ok, err)
	}
	if _, err := st.AppendTutorConsent(TutorConsentEvent{SourceID: "src-a", Granted: true, Reviewer: "owner", At: base.Add(2 * time.Second)}); err != nil {
		t.Fatalf("second grant: %v", err)
	}
	latest, _, err = st.TutorConsent("src-a")
	if err != nil || !latest.Granted || latest.Reviewer != "owner" {
		t.Fatalf("latest consent = %+v, err %v", latest, err)
	}
	if _, ok, err := st.TutorConsent("src-missing"); err != nil || ok {
		t.Fatalf("missing consent = ok %v, err %v", ok, err)
	}
	consents, err := st.TutorConsents()
	if err != nil || len(consents) != 1 || !consents[0].Granted {
		t.Fatalf("TutorConsents = %+v, err %v", consents, err)
	}
	if _, err := st.AppendTutorConsent(TutorConsentEvent{SourceID: "src-a", Granted: true, Reviewer: " ", At: base}); err == nil {
		t.Fatal("empty reviewer was accepted")
	}
	if _, err := st.AppendTutorConsent(TutorConsentEvent{SourceID: "", Granted: true, Reviewer: "owner", At: base}); err == nil {
		t.Fatal("empty source id was accepted")
	}

	if _, err := st.db.Exec(`UPDATE tutor_consent_events SET granted = 0`); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("consent UPDATE error = %v, want append-only", err)
	}
	if _, err := st.db.Exec(`DELETE FROM tutor_consent_events`); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("consent DELETE error = %v, want append-only", err)
	}
}

func TestTutorCallRoundTripAndAppendOnly(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "calls.sqlite"))
	first, err := st.AppendTutorCall(sampleTutorCall())
	if err != nil {
		t.Fatalf("AppendTutorCall: %v", err)
	}
	if first.ID == 0 || first.OutputText != "short advice" {
		t.Fatalf("first call = %+v", first)
	}

	errorCall := sampleTutorCall()
	errorCall.AttemptID = 2
	errorCall.Status = "error"
	errorCall.ErrorCode = "provider_error"
	errorCall.ErrorMessage = "provider returned HTTP 500"
	errorCall.OutputText = ""
	errorCall.OutputChars = 0
	errorCall.PromptTokens = nil
	errorCall.CompletionTokens = nil
	errorCall.HTTPStatus = 500
	errorCall.CreatedAt = errorCall.CreatedAt.Add(time.Minute)
	if _, err := st.AppendTutorCall(errorCall); err != nil {
		t.Fatalf("append error call: %v", err)
	}

	calls, err := st.TutorCalls(0)
	if err != nil || len(calls) != 2 {
		t.Fatalf("TutorCalls = %+v, err %v", calls, err)
	}
	if calls[0].PromptTokens == nil || *calls[0].PromptTokens != 12 || calls[0].CompletionTokens == nil || *calls[0].CompletionTokens != 8 {
		t.Fatalf("usage round-trip = %+v", calls[0])
	}
	if calls[1].Status != "error" || calls[1].ErrorCode != "provider_error" || calls[1].PromptTokens != nil {
		t.Fatalf("error row = %+v", calls[1])
	}
	filtered, err := st.TutorCalls(1)
	if err != nil || len(filtered) != 1 || filtered[0].ID != first.ID {
		t.Fatalf("filtered calls = %+v, err %v", filtered, err)
	}
	single, err := st.TutorCall(first.ID)
	if err != nil || single.OutputText != "short advice" {
		t.Fatalf("TutorCall = %+v, err %v", single, err)
	}
	if _, err := st.TutorCall(999); err != ErrNotFound {
		t.Fatalf("missing TutorCall error = %v, want ErrNotFound", err)
	}

	if _, err := st.db.Exec(`UPDATE tutor_calls SET output_text = 'changed'`); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("call UPDATE error = %v, want append-only", err)
	}
	if _, err := st.db.Exec(`DELETE FROM tutor_calls`); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("call DELETE error = %v, want append-only", err)
	}
}

func TestTutorCallCountSinceUsesTheDayBoundary(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "budget.sqlite"))
	early := sampleTutorCall()
	early.CreatedAt = time.Date(2026, 11, 9, 23, 59, 59, 0, time.UTC)
	if _, err := st.AppendTutorCall(early); err != nil {
		t.Fatal(err)
	}
	boundary := sampleTutorCall()
	boundary.CreatedAt = time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	if _, err := st.AppendTutorCall(boundary); err != nil {
		t.Fatal(err)
	}
	late := sampleTutorCall()
	late.CreatedAt = time.Date(2026, 11, 10, 15, 12, 0, 0, time.UTC)
	if _, err := st.AppendTutorCall(late); err != nil {
		t.Fatal(err)
	}
	count, err := st.TutorCallCountSince(time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC))
	if err != nil || count != 2 {
		t.Fatalf("count = %d, err %v; want 2", count, err)
	}
}

func TestAppendTutorCallRejectsMalformedRecords(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "invalid.sqlite"))
	tests := []struct {
		name    string
		mutate  func(*TutorCall)
		wantErr string
	}{
		{"empty output", func(c *TutorCall) { c.OutputText = ""; c.OutputChars = 0 }, "no output text"},
		{"wrong output chars", func(c *TutorCall) { c.OutputChars = 3 }, "output_chars"},
		{"bad mode", func(c *TutorCall) { c.Mode = "chat" }, "mode"},
		{"bad status", func(c *TutorCall) { c.Status = "pending" }, "status"},
		{"bad sha", func(c *TutorCall) { c.RequestSHA256 = "short" }, "request_sha256"},
		{"uppercase sha", func(c *TutorCall) { c.RequestSHA256 = strings.Repeat("A", 64) }, "lowercase hex"},
		{"no content", func(c *TutorCall) { c.ContentID = "" }, "content_id"},
		{"no question", func(c *TutorCall) { c.QuestionID = "" }, "question_id"},
		{"no provider host", func(c *TutorCall) { c.ProviderHost = "" }, "provider_host"},
		{"no model", func(c *TutorCall) { c.Model = "" }, "model"},
		{"no prompt version", func(c *TutorCall) { c.PromptVersion = "" }, "prompt_version"},
		{"no request bytes", func(c *TutorCall) { c.RequestBytes = 0 }, "request_bytes"},
		{"oversized output", func(c *TutorCall) {
			c.OutputText = strings.Repeat("x", tutor.MaxResponseChars+1)
			c.OutputChars = tutor.MaxResponseChars + 1
		}, "exceeds"},
		{"negative tokens", func(c *TutorCall) { value := -1; c.PromptTokens = &value }, "prompt_tokens"},
		{"negative completion tokens", func(c *TutorCall) { value := -1; c.CompletionTokens = &value }, "completion_tokens"},
		{"negative latency", func(c *TutorCall) { c.LatencyMS = -1 }, "latency_ms"},
		{"bad http", func(c *TutorCall) { c.HTTPStatus = 600 }, "http_status"},
		{"error without code", func(c *TutorCall) {
			c.Status = "error"
			c.ErrorCode = ""
			c.ErrorMessage = "boom"
			c.OutputText = ""
			c.OutputChars = 0
		}, "error code"},
		{"error without message", func(c *TutorCall) {
			c.Status = "error"
			c.ErrorCode = "provider_error"
			c.ErrorMessage = ""
			c.OutputText = ""
			c.OutputChars = 0
		}, "no message"},
		{"error with output", func(c *TutorCall) {
			c.Status = "error"
			c.ErrorCode = "provider_error"
			c.ErrorMessage = "boom"
		}, "output text"},
		{"ok with error code", func(c *TutorCall) { c.ErrorCode = "provider_error" }, "ok call has error code"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			call := sampleTutorCall()
			test.mutate(&call)
			if _, err := st.AppendTutorCall(call); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("AppendTutorCall error = %v, want %q", err, test.wantErr)
			}
		})
	}
	if calls, err := st.TutorCalls(0); err != nil || len(calls) != 0 {
		t.Fatalf("invalid calls were stored: %+v, %v", calls, err)
	}
}
