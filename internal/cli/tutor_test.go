package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/tutor"
)

const fakeTutorKey = "tutor-test-key-not-real"

type fakeTutorTransport struct {
	requests []tutor.HostRequest
	response tutor.HostResponse
	err      error
}

func (f *fakeTutorTransport) Call(_ context.Context, req tutor.HostRequest) (tutor.HostResponse, error) {
	f.requests = append(f.requests, req)
	return f.response, f.err
}

func fixedTutorResponse(text string) tutor.HostResponse {
	return tutor.HostResponse{
		ProtocolVersion: tutor.ProtocolVersion,
		Status:          "ok",
		Text:            text,
		HTTPStatus:      200,
		LatencyMS:       42,
		RequestSent:     true,
		Usage:           &tutor.Usage{PromptTokens: 120, CompletionTokens: 30},
	}
}

func fixedTutorError(code, message string) tutor.HostResponse {
	return tutor.HostResponse{
		ProtocolVersion: tutor.ProtocolVersion,
		Status:          "error",
		HTTPStatus:      500,
		LatencyMS:       20,
		RequestSent:     true,
		Error:           &tutor.HostError{Code: code, Message: message},
	}
}

func tutorEnv(present bool) func(string) string {
	return func(key string) string {
		if key == tutor.CredentialEnv && present {
			return fakeTutorKey
		}
		return ""
	}
}

func newTutorOptions(stdout, stderr *bytes.Buffer, clock *stepClock, transport *fakeTutorTransport, keyPresent bool) Options {
	return Options{
		Stdin:          strings.NewReader(""),
		Stdout:         stdout,
		Stderr:         stderr,
		Now:            clock.Now,
		NewSession:     func() string { return "sess-test" },
		Getenv:         tutorEnv(keyPresent),
		TutorTransport: transport,
	}
}

func runTutorCLI(t *testing.T, db string, clock *stepClock, transport *fakeTutorTransport, keyPresent bool, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(append(args, "--db", db), newTutorOptions(&stdout, &stderr, clock, transport, keyPresent))
	return code, stdout.String(), stderr.String()
}

func mustTutorRun(t *testing.T, db string, clock *stepClock, transport *fakeTutorTransport, keyPresent bool, args ...string) string {
	t.Helper()
	code, out, errOut := runTutorCLI(t, db, clock, transport, keyPresent, args...)
	if code != 0 {
		t.Fatalf("Run(%v) = %d\nstdout:\n%s\nstderr:\n%s", args, code, out, errOut)
	}
	return out
}

func configureTutor(t *testing.T, db string, clock *stepClock, transport *fakeTutorTransport, keyPresent bool, extra ...string) string {
	t.Helper()
	args := append([]string{
		"tutor", "config", "--endpoint", "https://api.example.com/v1/chat/completions", "--model", "example-model", "--json",
	}, extra...)
	return mustTutorRun(t, db, clock, transport, keyPresent, args...)
}

func decodeTutorPreview(t *testing.T, raw string) tutorPreviewDoc {
	t.Helper()
	var doc tutorPreviewDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode tutor preview: %v\n%s", err, raw)
	}
	return doc
}

func decodeTutorAdvisory(t *testing.T, raw string) tutorAdvisoryDoc {
	t.Helper()
	var doc tutorAdvisoryDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode tutor advisory: %v\n%s", err, raw)
	}
	return doc
}

func decodeTutorHistory(t *testing.T, raw string) tutorHistoryDoc {
	t.Helper()
	var doc tutorHistoryDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode tutor history: %v\n%s", err, raw)
	}
	return doc
}

func decodeTutorConfig(t *testing.T, raw string) tutorConfigDoc {
	t.Helper()
	var doc tutorConfigDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode tutor config: %v\n%s", err, raw)
	}
	return doc
}

func decodeTutorFailure(t *testing.T, raw string) tutorFailureDoc {
	t.Helper()
	var doc tutorFailureDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode tutor failure: %v\n%s", err, raw)
	}
	return doc
}

func practiceSample(t *testing.T, db string, clock *stepClock) {
	t.Helper()
	mustRun(t, db, clock, "", "practice", "--question", "q1", "--answer", "B", "--json")
}

func importTutorSource(t *testing.T, db string, clock *stepClock) string {
	t.Helper()
	sourceID := importFixture(t, db, clock).Source.ID
	mustRun(t, db, clock, "", "question", "add", sourceID, "1", "--file", fixtureFile(t, "question-q2-tagged.json"), "--json")
	mustRun(t, db, clock, "", "page", "review", sourceID, "1", "--reviewer", "tutor-reviewer", "--json")
	mustRun(t, db, clock, "", "question", "review", sourceID, "q2", "--answer", "C", "--reviewer", "tutor-reviewer", "--json")
	mustRun(t, db, clock, "", "practice", "--source", sourceID, "--page", "1", "--question", "q2", "--answer", "C", "--json")
	return sourceID
}

func TestTutorConfigInspectSaveAndPartialUpdate(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{}

	raw := mustTutorRun(t, db, clock, transport, false, "tutor", "config", "--json")
	doc := decodeTutorConfig(t, raw)
	if doc.Configured {
		t.Fatal("empty config is reported configured")
	}
	if doc.Config.TimeoutMS != tutor.DefaultTimeoutMS || doc.Config.DailyCalls != tutor.DefaultDailyCalls {
		t.Fatalf("defaults = %+v", doc.Config)
	}
	if doc.Credential.Present {
		t.Fatal("credential reported present without an environment value")
	}

	configured := decodeTutorConfig(t, configureTutor(t, db, clock, transport, true, "--daily-calls", "7"))
	if !configured.Configured || configured.Config.Model != "example-model" || configured.Config.DailyCalls != 7 {
		t.Fatalf("configured = %+v", configured)
	}
	if !configured.Credential.Present || configured.Credential.Env != tutor.CredentialEnv {
		t.Fatalf("credential = %+v", configured.Credential)
	}
	if strings.Contains(configured.Config.Endpoint, fakeTutorKey) {
		t.Fatal("config doc contains the credential")
	}

	updated := decodeTutorConfig(t, mustTutorRun(t, db, clock, transport, true,
		"tutor", "config", "--model", "second-model", "--json"))
	if updated.Config.Model != "second-model" || updated.Config.Endpoint != "https://api.example.com/v1/chat/completions" || updated.Config.DailyCalls != 7 {
		t.Fatalf("partial update = %+v", updated.Config)
	}

	code, _, stderr := runTutorCLI(t, db, clock, transport, true,
		"tutor", "config", "--endpoint", "http://api.example.com/v1", "--json")
	if code != 2 {
		t.Fatalf("invalid endpoint exit = %d", code)
	}
	failure := decodeTutorFailure(t, stderr)
	if failure.Error.Code != "invalid_config" {
		t.Fatalf("invalid endpoint error = %+v", failure.Error)
	}
	after := decodeTutorConfig(t, mustTutorRun(t, db, clock, transport, true, "tutor", "config", "--json"))
	if after.Config.Endpoint != "https://api.example.com/v1/chat/completions" {
		t.Fatalf("invalid update changed config: %+v", after.Config)
	}
}

func TestTutorConfigWhitespaceCredentialIsAbsent(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{}
	var stdout, stderr bytes.Buffer
	options := newTutorOptions(&stdout, &stderr, clock, transport, false)
	options.Getenv = func(key string) string {
		if key == tutor.CredentialEnv {
			return " \t"
		}
		return ""
	}

	code := Run([]string{
		"tutor", "config", "--endpoint", "https://api.example.com/v1/chat/completions",
		"--model", "example-model", "--json", "--db", db,
	}, options)
	if code != 0 {
		t.Fatalf("config exit = %d\n%s", code, stderr.String())
	}
	doc := decodeTutorConfig(t, stdout.String())
	if doc.Credential.Present {
		t.Fatal("whitespace-only credential is reported present")
	}

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"tutor", "ask", "--json", "--db", db}, options)
	if code != 2 || !strings.Contains(stderr.String(), "missing_credential") {
		t.Fatalf("whitespace credential ask = %d %s", code, stderr.String())
	}
	if len(transport.requests) != 0 {
		t.Fatal("whitespace credential made a provider request")
	}
}

func TestTutorConfigRejectsCredentialInEndpoint(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{}
	code, _, stderr := runTutorCLI(t, db, clock, transport, true,
		"tutor", "config", "--endpoint", "https://key:secret@api.example.com/v1", "--model", "m", "--json")
	if code != 2 {
		t.Fatalf("userinfo endpoint exit = %d", code)
	}
	if !strings.Contains(stderr, "userinfo") {
		t.Fatalf("userinfo error = %s", stderr)
	}
}

func TestTutorPreviewFirstParty(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{}
	practiceSample(t, db, clock)
	configureTutor(t, db, clock, transport, true)

	raw := mustTutorRun(t, db, clock, transport, true, "tutor", "preview", "--attempt", "1", "--json")
	doc := decodeTutorPreview(t, raw)
	if doc.State != "preview" || doc.Mode != "explain" {
		t.Fatalf("preview state = %+v", doc)
	}
	if doc.Attempt.ID != 1 || doc.Attempt.ContentID != "sample-0001" || doc.Attempt.QuestionID != "q1" {
		t.Fatalf("attempt view = %+v", doc.Attempt)
	}
	if !doc.Provider.CredentialPresent || doc.Provider.CredentialEnv != tutor.CredentialEnv {
		t.Fatalf("provider = %+v", doc.Provider)
	}
	if doc.Context.PrivateSource || doc.Context.SourceTextShared || doc.Context.ContentUnavailable {
		t.Fatalf("context = %+v", doc.Context)
	}
	user := doc.Chat.Messages[1].Content
	for _, want := range []string{"Westbridge", "What time will the library close", "Verified key: B", "elapsed_ms=1000"} {
		if !strings.Contains(user, want) {
			t.Errorf("preview chat is missing %q\n%s", want, user)
		}
	}
	if len(doc.RequestSHA256) != 64 || doc.RequestBytes == 0 {
		t.Fatalf("request identity = %q/%d", doc.RequestSHA256, doc.RequestBytes)
	}
	if strings.Contains(raw, fakeTutorKey) {
		t.Fatal("preview output contains the credential")
	}
	if len(transport.requests) != 0 {
		t.Fatalf("preview sent %d provider requests", len(transport.requests))
	}
}

func TestTutorAskSuccessPreservesAttemptsAndNeverStoresKey(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{response: fixedTutorResponse("Focus on the sentence that gives the Saturday closing time.")}
	practiceSample(t, db, clock)
	configureTutor(t, db, clock, transport, true)
	before := mustRun(t, db, clock, "", "report", "--json")

	raw := mustTutorRun(t, db, clock, transport, true, "tutor", "ask", "--attempt", "1", "--json")
	doc := decodeTutorAdvisory(t, raw)
	if doc.State != "advisory" || !doc.Suggestion.Advisory || doc.Suggestion.VerifiedAnswer {
		t.Fatalf("advisory doc = %+v", doc)
	}
	if doc.Call.Status != "ok" || doc.Call.AttemptID != 1 || doc.Call.Mode != "explain" {
		t.Fatalf("call = %+v", doc.Call)
	}
	if doc.Call.PromptTokens == nil || *doc.Call.PromptTokens != 120 || doc.Call.CompletionTokens == nil || *doc.Call.CompletionTokens != 30 {
		t.Fatalf("usage = %+v", doc.Call)
	}
	if doc.Call.ProviderHost != "api.example.com" || doc.Call.Model != "example-model" {
		t.Fatalf("provider record = %+v", doc.Call)
	}
	if strings.Contains(raw, fakeTutorKey) || strings.Contains(doc.Suggestion.Text, fakeTutorKey) {
		t.Fatal("advisory output contains the credential")
	}
	after := mustRun(t, db, clock, "", "report", "--json")
	if before != after {
		t.Fatalf("tutor ask changed report output\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if len(transport.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(transport.requests))
	}
	if transport.requests[0].Chat.Model != "example-model" || transport.requests[0].Chat.MaxTokens != tutor.DefaultMaxTokens {
		t.Fatalf("provider request = %+v", transport.requests[0])
	}

	history := decodeTutorHistory(t, mustTutorRun(t, db, clock, transport, true, "tutor", "history", "--json"))
	if history.Summary.Calls != 1 || history.Summary.OK != 1 || history.Summary.Failed != 0 ||
		history.Summary.PromptTokens != 120 || history.Summary.CompletionTokens != 30 {
		t.Fatalf("history summary = %+v", history.Summary)
	}
	if len(history.Calls) != 1 || history.Calls[0].OutputText != "" {
		t.Fatalf("history calls = %+v, want no model text", history.Calls)
	}
	show := mustTutorRun(t, db, clock, transport, true, "tutor", "show", "1", "--json")
	if !strings.Contains(show, "Saturday closing time") || !strings.Contains(show, `"advisory": true`) {
		t.Fatalf("show output = %s", show)
	}

	dbBytes, err := os.ReadFile(db)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(dbBytes, []byte(fakeTutorKey)) {
		t.Fatal("database bytes contain the credential")
	}
}

func TestTutorCredentialIsRedactedFromProviderText(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{response: fixedTutorResponse("The provider echoed " + fakeTutorKey + " in its text.")}
	practiceSample(t, db, clock)
	configureTutor(t, db, clock, transport, true)

	raw := mustTutorRun(t, db, clock, transport, true, "tutor", "ask", "--json")
	doc := decodeTutorAdvisory(t, raw)
	if strings.Contains(doc.Suggestion.Text, fakeTutorKey) {
		t.Fatal("provider text kept the credential")
	}
	if !strings.Contains(doc.Suggestion.Text, "[redacted]") {
		t.Fatalf("provider text was not redacted: %s", doc.Suggestion.Text)
	}
	dbBytes, err := os.ReadFile(db)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(dbBytes, []byte(fakeTutorKey)) {
		t.Fatal("database bytes contain the credential")
	}
}

func TestTutorCredentialIsRedactedFromProviderError(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{response: fixedTutorError("provider_error", "The provider echoed "+fakeTutorKey+" in its error.")}
	practiceSample(t, db, clock)
	configureTutor(t, db, clock, transport, true)

	code, stdout, stderr := runTutorCLI(t, db, clock, transport, true, "tutor", "ask", "--json")
	if code != 3 {
		t.Fatalf("provider error exit = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	failure := decodeTutorFailure(t, stderr)
	if strings.Contains(failure.Error.Message, fakeTutorKey) {
		t.Fatal("provider error kept the credential")
	}
	if !strings.Contains(failure.Error.Message, "[redacted]") {
		t.Fatalf("provider error was not redacted: %s", failure.Error.Message)
	}
	dbBytes, err := os.ReadFile(db)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(dbBytes, []byte(fakeTutorKey)) {
		t.Fatal("database bytes contain the credential")
	}
}

func TestTutorPreV6AttemptFallsBackToCurrentTags(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{}
	configureTutor(t, db, clock, transport, true)

	handle, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handle.Exec(`INSERT INTO attempts (
			session_id, question_id, content_id, content_version, status,
			selected_label, selected_text, correct_label, correct_text,
			started_at, answered_at, elapsed_ms, assistance, tactic_id, skills_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		"sess-test", "v1", "skill-first-party-vocab-notice", "1", "correct",
		"A", "moved to a later time", "A", "moved to a later time",
		"2026-11-10T09:00:00Z", "2026-11-10T09:00:01Z", 1000, "independent", ""); err != nil {
		handle.Close()
		t.Fatal(err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}

	preview := decodeTutorPreview(t, mustTutorRun(t, db, clock, transport, true, "tutor", "preview", "--json"))
	if !preview.Context.SkillsMappedFromCurrentTags {
		t.Fatal("pre-v6 fallback is not disclosed")
	}
	if got := strings.Join(preview.Context.Skills, ","); got != "vocabulary-in-context" {
		t.Fatalf("pre-v6 skills = %q, want vocabulary-in-context", got)
	}
	if !strings.Contains(preview.Chat.Messages[1].Content, "Vocabulary in context") {
		t.Fatal("pre-v6 fallback did not include the current lesson")
	}
}

func TestTutorPrivateSourceConsentRoundTrip(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{response: fixedTutorResponse("Use the notice sentence as evidence.")}
	sourceID := importTutorSource(t, db, clock)
	configureTutor(t, db, clock, transport, true)

	preview := decodeTutorPreview(t, mustTutorRun(t, db, clock, transport, true, "tutor", "preview", "--json"))
	if !preview.Context.PrivateSource || !preview.Context.ConsentRequired || preview.Context.ConsentGranted || preview.Context.SourceTextShared {
		t.Fatalf("private context = %+v", preview.Context)
	}
	if preview.Context.SourceID != sourceID {
		t.Fatalf("preview source = %q, want %q", preview.Context.SourceID, sourceID)
	}
	redacted := preview.Chat.Messages[1].Content
	for _, secret := range []string{
		"The Westbridge Community Library closes at 4 p.m. on Saturdays.",
		"Which of the following is NOT stated",
		"The library closes at 4 p.m. on Saturdays.",
	} {
		if strings.Contains(redacted, secret) {
			t.Errorf("withheld source text leaked: %q\n%s", secret, redacted)
		}
	}
	if !strings.Contains(redacted, "Question content: withheld") || !strings.Contains(redacted, "not-except-polarity") {
		t.Fatalf("redacted prompt = %s", redacted)
	}

	consent := mustTutorRun(t, db, clock, transport, true, "tutor", "consent", sourceID, "--reviewer", "owner", "--json")
	if !strings.Contains(consent, `"granted": true`) || !strings.Contains(consent, `"reviewer": "owner"`) {
		t.Fatalf("consent output = %s", consent)
	}
	authorized := decodeTutorPreview(t, mustTutorRun(t, db, clock, transport, true, "tutor", "preview", "--json"))
	if !authorized.Context.ConsentGranted || !authorized.Context.SourceTextShared {
		t.Fatalf("authorized context = %+v", authorized.Context)
	}
	provided := authorized.Chat.Messages[1].Content
	for _, want := range []string{
		"The Westbridge Community Library closes at 4 p.m. on Saturdays.",
		"Which of the following is NOT stated",
		"Verified evidence: The Westbridge Community Library closes at 4 p.m. on Saturdays.",
	} {
		if !strings.Contains(provided, want) {
			t.Errorf("authorized prompt is missing %q\n%s", want, provided)
		}
	}

	ask := decodeTutorAdvisory(t, mustTutorRun(t, db, clock, transport, true, "tutor", "ask", "--json"))
	if !ask.Call.SourceTextShared {
		t.Fatal("ask record does not mark shared source text")
	}
	if len(transport.requests) != 1 || !strings.Contains(transport.requests[0].Chat.Messages[1].Content, "closes at 4 p.m.") {
		t.Fatal("provider request did not include authorized source text")
	}

	revoked := mustTutorRun(t, db, clock, transport, true, "tutor", "revoke", sourceID, "--reviewer", "owner", "--json")
	if !strings.Contains(revoked, `"granted": false`) {
		t.Fatalf("revoke output = %s", revoked)
	}
	afterRevoke := decodeTutorPreview(t, mustTutorRun(t, db, clock, transport, true, "tutor", "preview", "--json"))
	if afterRevoke.Context.ConsentGranted || afterRevoke.Context.SourceTextShared {
		t.Fatalf("revoked context = %+v", afterRevoke.Context)
	}
	if strings.Contains(afterRevoke.Chat.Messages[1].Content, "The Westbridge Community Library closes at 4 p.m.") {
		t.Fatal("revoked source text was still included")
	}

	consents := mustTutorRun(t, db, clock, transport, true, "tutor", "consents", "--json")
	var consentsDoc tutorConsentsDoc
	if err := json.Unmarshal([]byte(consents), &consentsDoc); err != nil {
		t.Fatal(err)
	}
	if len(consentsDoc.Sources) != 1 || consentsDoc.Sources[0].Granted || consentsDoc.Sources[0].Reviewer != "owner" {
		t.Fatalf("consents = %+v", consentsDoc.Sources)
	}
}

func TestTutorConsentValidation(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{}
	sourceID := importTutorSource(t, db, clock)

	code, _, stderr := runTutorCLI(t, db, clock, transport, true, "tutor", "consent", "src-missing", "--reviewer", "owner", "--json")
	if code != 2 || !strings.Contains(stderr, "unknown_source") {
		t.Fatalf("unknown source = %d %s", code, stderr)
	}
	code, _, stderr = runTutorCLI(t, db, clock, transport, true, "tutor", "consent", sourceID, "--json")
	if code != 2 || !strings.Contains(stderr, "reviewer") {
		t.Fatalf("missing reviewer = %d %s", code, stderr)
	}
	code, _, stderr = runTutorCLI(t, db, clock, transport, true, "tutor", "revoke", "--json")
	if code != 2 || !strings.Contains(stderr, "tutor revoke requires exactly one source id") {
		t.Fatalf("revoke usage = %d %s", code, stderr)
	}
}

func TestTutorProviderFailureIsRecordedAndPracticeStillWorks(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{response: fixedTutorError("provider_error", "provider returned HTTP 500")}
	practiceSample(t, db, clock)
	configureTutor(t, db, clock, transport, true)
	before := mustRun(t, db, clock, "", "report", "--json")

	code, stdout, stderr := runTutorCLI(t, db, clock, transport, true, "tutor", "ask", "--json")
	if code != 3 {
		t.Fatalf("provider failure exit = %d\n%s%s", code, stdout, stderr)
	}
	failure := decodeTutorFailure(t, stderr)
	if failure.State != "failed" || failure.Error.Code != "provider_error" || failure.Call.Status != "error" {
		t.Fatalf("failure = %+v", failure)
	}
	if failure.Call.HTTPStatus != 500 || failure.Call.LatencyMS != 20 {
		t.Fatalf("failure call = %+v", failure.Call)
	}
	after := mustRun(t, db, clock, "", "report", "--json")
	if before != after {
		t.Fatalf("failed tutor call changed report output")
	}
	practice := mustRun(t, db, clock, "", "practice", "--question", "q2", "--answer", "B", "--json")
	if !strings.Contains(practice, `"id": 2`) {
		t.Fatalf("practice after failure = %s", practice)
	}
	history := decodeTutorHistory(t, mustTutorRun(t, db, clock, transport, true, "tutor", "history", "--json"))
	if history.Summary.Failed != 1 || history.Calls[0].ErrorCode != "provider_error" {
		t.Fatalf("failure history = %+v", history)
	}
}

func TestTutorTransportFailureIsAgentUnavailable(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{err: errors.New("spawn node: not found")}
	practiceSample(t, db, clock)
	configureTutor(t, db, clock, transport, true)

	code, _, stderr := runTutorCLI(t, db, clock, transport, true, "tutor", "ask", "--json")
	if code != 3 {
		t.Fatalf("transport failure exit = %d", code)
	}
	failure := decodeTutorFailure(t, stderr)
	if failure.Error.Code != "agent_unavailable" || failure.Call.ErrorCode != "agent_unavailable" {
		t.Fatalf("failure = %+v", failure)
	}
	if !strings.Contains(stderr, "cannot append an attempt") {
		t.Fatalf("failure limits are missing the boundary: %s", stderr)
	}
}

func TestTutorTerminalControlResponseIsRecorded(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{response: fixedTutorResponse("\x1b[31mnot advice\x1b[0m")}
	practiceSample(t, db, clock)
	configureTutor(t, db, clock, transport, true)

	code, _, stderr := runTutorCLI(t, db, clock, transport, true, "tutor", "ask", "--json")
	if code != 3 {
		t.Fatalf("control response exit = %d", code)
	}
	failure := decodeTutorFailure(t, stderr)
	if failure.Error.Code != "invalid_response" {
		t.Fatalf("failure = %+v", failure)
	}
	if strings.Contains(stderr, "\x1b") {
		t.Fatal("terminal control escaped into output")
	}
}

func TestTutorInvalidResponseIsRecorded(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{response: tutor.HostResponse{
		ProtocolVersion: tutor.ProtocolVersion,
		Status:          "ok",
		Text:            "",
		HTTPStatus:      200,
	}}
	practiceSample(t, db, clock)
	configureTutor(t, db, clock, transport, true)

	code, _, stderr := runTutorCLI(t, db, clock, transport, true, "tutor", "ask", "--json")
	if code != 3 {
		t.Fatalf("invalid response exit = %d", code)
	}
	failure := decodeTutorFailure(t, stderr)
	if failure.Error.Code != "invalid_response" {
		t.Fatalf("failure = %+v", failure)
	}
}

func TestTutorAskRequiresCredentialAndBudget(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{response: fixedTutorResponse("Short advice.")}
	practiceSample(t, db, clock)
	configureTutor(t, db, clock, transport, false, "--daily-calls", "1")

	code, _, stderr := runTutorCLI(t, db, clock, transport, false, "tutor", "ask", "--json")
	if code != 2 || !strings.Contains(stderr, "missing_credential") {
		t.Fatalf("missing credential = %d %s", code, stderr)
	}
	if len(transport.requests) != 0 {
		t.Fatal("a provider request was made without a credential")
	}
	history := decodeTutorHistory(t, mustTutorRun(t, db, clock, transport, false, "tutor", "history", "--json"))
	if history.Summary.Calls != 0 {
		t.Fatalf("missing credential recorded a call: %+v", history.Summary)
	}

	mustTutorRun(t, db, clock, transport, true, "tutor", "ask", "--json")
	code, _, stderr = runTutorCLI(t, db, clock, transport, true, "tutor", "ask", "--json")
	if code != 2 || !strings.Contains(stderr, "budget_exhausted") {
		t.Fatalf("budget = %d %s", code, stderr)
	}
	history = decodeTutorHistory(t, mustTutorRun(t, db, clock, transport, true, "tutor", "history", "--json"))
	if history.Summary.Calls != 1 || history.Summary.DailyLimit == nil || *history.Summary.DailyLimit != 1 {
		t.Fatalf("budget history = %+v", history.Summary)
	}
}

func TestTutorUsageErrors(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{}

	code, _, stderr := runTutorCLI(t, db, clock, transport, true, "tutor", "preview", "--json")
	if code != 2 || !strings.Contains(stderr, "not_configured") {
		t.Fatalf("unconfigured preview = %d %s", code, stderr)
	}
	configureTutor(t, db, clock, transport, true)
	code, _, stderr = runTutorCLI(t, db, clock, transport, true, "tutor", "preview", "--json")
	if code != 2 || !strings.Contains(stderr, "no_attempts") {
		t.Fatalf("no attempts = %d %s", code, stderr)
	}
	practiceSample(t, db, clock)
	code, _, stderr = runTutorCLI(t, db, clock, transport, true, "tutor", "preview", "--attempt", "99", "--json")
	if code != 2 || !strings.Contains(stderr, "unknown_attempt") {
		t.Fatalf("unknown attempt = %d %s", code, stderr)
	}
	code, _, stderr = runTutorCLI(t, db, clock, transport, true, "tutor", "preview", "--mode", "chat", "--json")
	if code != 2 || !strings.Contains(stderr, "invalid_mode") {
		t.Fatalf("unknown mode = %d %s", code, stderr)
	}
	code, _, stderr = runTutorCLI(t, db, clock, transport, true, "tutor", "show", "999", "--json")
	if code != 2 || !strings.Contains(stderr, "unknown_call") {
		t.Fatalf("unknown call = %d %s", code, stderr)
	}
	code, _, stderr = runTutorCLI(t, db, clock, transport, true, "tutor", "show", "not-a-number", "--json")
	if code != 2 || !strings.Contains(stderr, "invalid_input") {
		t.Fatalf("invalid call id = %d %s", code, stderr)
	}
}

func TestTutorMissingNodeLeavesPracticeWorking(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	practiceSample(t, db, clock)
	configureTutor(t, db, clock, &fakeTutorTransport{}, true)

	host, err := filepath.Abs(filepath.Join("..", "..", "agent", "tutor-host.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		tutor.CredentialEnv: fakeTutorKey,
		tutor.HostEnv:       host,
		tutor.NodeEnv:       filepath.Join(t.TempDir(), "missing-node"),
	}
	getenv := func(key string) string { return env[key] }
	transport := tutor.HostTransport{
		Getenv:  getenv,
		Environ: os.Environ,
	}
	var stdout, stderr bytes.Buffer
	options := Options{
		Stdin:          strings.NewReader(""),
		Stdout:         &stdout,
		Stderr:         &stderr,
		Now:            clock.Now,
		NewSession:     func() string { return "sess-test" },
		Getenv:         getenv,
		TutorTransport: transport,
	}
	code := Run([]string{"tutor", "ask", "--json", "--db", db}, options)
	if code != 3 {
		t.Fatalf("missing node exit = %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "agent_unavailable") {
		t.Fatalf("missing node stderr = %s", stderr.String())
	}
	if strings.Contains(stderr.String(), fakeTutorKey) {
		t.Fatal("missing node output contains the credential")
	}
	call := decodeTutorHistory(t, mustRun(t, db, clock, "", "tutor", "history", "--json"))
	if call.Summary.Failed != 1 || call.Calls[0].ErrorCode != "agent_unavailable" {
		t.Fatalf("missing node history = %+v", call)
	}
	if out := mustRun(t, db, clock, "", "practice", "--question", "q2", "--answer", "B", "--json"); !strings.Contains(out, `"id": 2`) {
		t.Fatalf("practice after missing node = %s", out)
	}
}

func TestTutorGoldenDocuments(t *testing.T) {
	t.Run("config", func(t *testing.T) {
		db := newDB(t)
		clock := &stepClock{base: goldenBase}
		transport := &fakeTutorTransport{}
		raw := configureTutor(t, db, clock, transport, true, "--daily-calls", "7")
		checkGolden(t, filepath.Join("testdata", "tutor-config.golden.json"), []byte(raw))
	})

	t.Run("first-party preview", func(t *testing.T) {
		db := newDB(t)
		clock := &stepClock{base: goldenBase}
		transport := &fakeTutorTransport{}
		practiceSample(t, db, clock)
		configureTutor(t, db, clock, transport, true)
		raw := mustTutorRun(t, db, clock, transport, true, "tutor", "preview", "--attempt", "1", "--json")
		checkGolden(t, filepath.Join("testdata", "tutor-preview.golden.json"), []byte(raw))
	})

	t.Run("withheld private preview", func(t *testing.T) {
		db := newDB(t)
		clock := &stepClock{base: goldenBase}
		transport := &fakeTutorTransport{}
		importTutorSource(t, db, clock)
		configureTutor(t, db, clock, transport, true)
		raw := mustTutorRun(t, db, clock, transport, true, "tutor", "preview", "--json")
		checkGolden(t, filepath.Join("testdata", "tutor-preview-private.golden.json"), []byte(raw))
	})

	t.Run("advisory", func(t *testing.T) {
		db := newDB(t)
		clock := &stepClock{base: goldenBase}
		transport := &fakeTutorTransport{response: fixedTutorResponse("Locate the sentence that gives the Saturday closing time, then check the day word.")}
		practiceSample(t, db, clock)
		configureTutor(t, db, clock, transport, true)
		raw := mustTutorRun(t, db, clock, transport, true, "tutor", "ask", "--attempt", "1", "--json")
		checkGolden(t, filepath.Join("testdata", "tutor-advisory.golden.json"), []byte(raw))
	})

	t.Run("history", func(t *testing.T) {
		db := newDB(t)
		clock := &stepClock{base: goldenBase}
		transport := &fakeTutorTransport{response: fixedTutorResponse("Short advice.")}
		practiceSample(t, db, clock)
		configureTutor(t, db, clock, transport, true)
		mustTutorRun(t, db, clock, transport, true, "tutor", "ask", "--json")
		transport.response = fixedTutorError("provider_error", "provider returned HTTP 500")
		code, _, _ := runTutorCLI(t, db, clock, transport, true, "tutor", "ask", "--json")
		if code != 3 {
			t.Fatalf("second call exit = %d", code)
		}
		raw := mustTutorRun(t, db, clock, transport, true, "tutor", "history", "--json")
		checkGolden(t, filepath.Join("testdata", "tutor-history.golden.json"), []byte(raw))
	})

	t.Run("consents", func(t *testing.T) {
		db := newDB(t)
		clock := &stepClock{base: goldenBase}
		transport := &fakeTutorTransport{}
		sourceID := importTutorSource(t, db, clock)
		mustTutorRun(t, db, clock, transport, true, "tutor", "consent", sourceID, "--reviewer", "owner", "--json")
		raw := mustTutorRun(t, db, clock, transport, true, "tutor", "consents", "--json")
		checkGolden(t, filepath.Join("testdata", "tutor-consents.golden.json"), []byte(raw))
	})
}

func TestTutorShowFailedCallHasNoSuggestion(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{response: fixedTutorError("auth_failed", "provider rejected the credential")}
	practiceSample(t, db, clock)
	configureTutor(t, db, clock, transport, true)
	code, _, _ := runTutorCLI(t, db, clock, transport, true, "tutor", "ask", "--json")
	if code != 3 {
		t.Fatalf("ask exit = %d", code)
	}
	raw := mustTutorRun(t, db, clock, transport, true, "tutor", "show", "1", "--json")
	var doc tutorShowDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.State != "failed" || doc.Suggestion != nil {
		t.Fatalf("failed show = %+v", doc)
	}
	if doc.Call.ErrorCode != "auth_failed" {
		t.Fatalf("failed show call = %+v", doc.Call)
	}
}

func TestTutorHistoryLabelsWithheldPrivateSource(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{response: fixedTutorError("provider_error", "down")}
	importTutorSource(t, db, clock)
	configureTutor(t, db, clock, transport, true)
	code, _, _ := runTutorCLI(t, db, clock, transport, true, "tutor", "ask", "--json")
	if code != 3 {
		t.Fatalf("ask exit = %d", code)
	}
	human := mustTutorRun(t, db, clock, transport, true, "tutor", "history")
	if !strings.Contains(human, "source-text-withheld") {
		t.Fatalf("history does not distinguish withheld private text:\n%s", human)
	}
}

// tutorLine returns the first output line containing needle, so a label
// assertion cannot be satisfied by a different line.
func tutorLine(t *testing.T, text, needle string) string {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	t.Fatalf("no output line contains %q:\n%s", needle, text)
	return ""
}

// TestWriteTutorPrivacyContentSummary pins the human content summary against
// the four reachable content states, including whether stored option texts
// were included or withheld.
func TestWriteTutorPrivacyContentSummary(t *testing.T) {
	tests := []struct {
		name    string
		context tutorContextView
		want    []string
		absent  []string
	}{
		{
			name:    "included first-party content",
			context: tutorContextView{ContentMatched: true, ContentIncluded: true},
			want: []string{
				"Privacy: first-party material only; no private source text is involved",
				"Content: the current passage, question, options, verified key and evidence are included.",
			},
			absent: []string{"Included instead", "Withheld as well"},
		},
		{
			name:    "changed content keeps stored option texts",
			context: tutorContextView{ContentMatched: true, ContentChanged: true, StoredTextIncluded: true},
			want: []string{
				"Content: the current version differs from the recorded attempt; no current passage, question, option, evidence or key text is included.",
				"Included instead: the recorded selection and verified key labels with their stored option texts.",
			},
			absent: []string{"attempt facts and skill lessons", "Withheld as well"},
		},
		{
			name:    "unavailable content keeps stored option texts",
			context: tutorContextView{ContentUnavailable: true, StoredTextIncluded: true},
			want: []string{
				"Content: the attempted question is unavailable in this build; no current passage, question, option, evidence or key text is included.",
				"Included instead: the recorded selection and verified key labels with their stored option texts.",
			},
			absent: []string{"Withheld as well"},
		},
		{
			name: "private content without consent withholds everything",
			context: tutorContextView{
				PrivateSource: true, SourceID: "src-secret",
				Withheld: []string{"passage", "selected_option_text", "correct_option_text"},
			},
			want: []string{
				"Privacy: private source src-secret is NOT authorized; source text shared: no",
				"Content: the current passage and question are withheld because this private source is not authorized.",
				"Withheld as well: the recorded selection and verified key option texts.",
			},
			absent: []string{"Included instead"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var out strings.Builder
			writeTutorPrivacy(&out, test.context)
			got := out.String()
			for _, want := range test.want {
				if !strings.Contains(got, want) {
					t.Errorf("summary is missing %q\n%s", want, got)
				}
			}
			for _, absent := range test.absent {
				if strings.Contains(got, absent) {
					t.Errorf("summary contains %q\n%s", absent, got)
				}
			}
		})
	}
}

func TestTutorHumanPreviewMatchesItsChat(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{}
	practiceSample(t, db, clock)
	configureTutor(t, db, clock, transport, true)

	human := mustTutorRun(t, db, clock, transport, true, "tutor", "preview", "--attempt", "1")
	for _, want := range []string{
		"Pass Quest — tutor preview (not sent)",
		"Privacy: first-party material only; no private source text is involved",
		"Content: the current passage, question, options, verified key and evidence are included.",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("first-party preview is missing %q\n%s", want, human)
		}
	}

	sourceID := importTutorSource(t, db, clock)
	human = mustTutorRun(t, db, clock, transport, true, "tutor", "preview")
	for _, want := range []string{
		"Privacy: private source " + sourceID + " is NOT authorized; source text shared: no",
		"Content: the current passage and question are withheld because this private source is not authorized.",
		"Withheld as well: the recorded selection and verified key option texts.",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("private preview is missing %q\n%s", want, human)
		}
	}
	for _, falseClaim := range []string{"only the stored selection/key are included", "only attempt facts and skill lessons are included"} {
		if strings.Contains(human, falseClaim) {
			t.Errorf("preview repeats the false summary %q\n%s", falseClaim, human)
		}
	}
}

// TestTutorHumanOutputStatesAuthorityAndScope pins the learner-facing labels:
// credentials, advisory status, consent state and history provenance. The JSON
// tests do not protect these strings.
func TestTutorHumanOutputStatesAuthorityAndScope(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{response: fixedTutorResponse("Read the sentence with the closing time.")}

	human := mustTutorRun(t, db, clock, transport, false, "tutor", "config")
	for _, want := range []string{
		"Provider: not configured",
		"Credential: " + tutor.CredentialEnv + " is missing",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("unconfigured config is missing %q\n%s", want, human)
		}
	}

	configureTutor(t, db, clock, transport, true)
	human = mustTutorRun(t, db, clock, transport, true, "tutor", "config")
	for _, want := range []string{
		"Provider endpoint: https://api.example.com/v1/chat/completions",
		"Credential: " + tutor.CredentialEnv + " is set",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("configured config is missing %q\n%s", want, human)
		}
	}

	practiceSample(t, db, clock)
	human = mustTutorRun(t, db, clock, transport, true, "tutor", "ask")
	for _, want := range []string{
		"Pass Quest — advisory AI suggestion (not a verified answer)",
		"--- AI suggestion (advisory) ---",
		"Usage: 120 prompt tokens, 30 completion tokens",
		"This is model output. It is not a verified answer, grade or exam fact.",
		"Advisory call #1 recorded; request sha256 ",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("advisory output is missing %q\n%s", want, human)
		}
	}
	if strings.Contains(human, fakeTutorKey) {
		t.Fatalf("advisory output contains the credential:\n%s", human)
	}

	human = mustTutorRun(t, db, clock, transport, true, "tutor", "show", "1")
	for _, want := range []string{
		"Pass Quest — advisory call #1",
		"--- AI suggestion (advisory) ---",
		"This stored text is model output, not a verified answer, grade or exam fact.",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("show output is missing %q\n%s", want, human)
		}
	}

	human = mustTutorRun(t, db, clock, transport, true, "tutor", "history")
	if line := tutorLine(t, human, "sample-0001/q1"); !strings.Contains(line, "first-party") {
		t.Errorf("history does not label first-party calls:\n%s", line)
	}

	sourceID := importTutorSource(t, db, clock)
	human = mustTutorRun(t, db, clock, transport, true, "tutor", "consents")
	if line := tutorLine(t, human, sourceID); !strings.Contains(line, "withheld") {
		t.Errorf("consents does not label an ungranted source:\n%s", line)
	}

	human = mustTutorRun(t, db, clock, transport, true, "tutor", "consent", sourceID, "--reviewer", "owner")
	if !strings.Contains(human, "Tutor consent for "+sourceID+" (import): authorized") {
		t.Errorf("consent output does not state authorization:\n%s", human)
	}
	human = mustTutorRun(t, db, clock, transport, true, "tutor", "consents")
	if line := tutorLine(t, human, sourceID); !strings.Contains(line, "authorized") {
		t.Errorf("consents does not label a granted source:\n%s", line)
	}

	human = mustTutorRun(t, db, clock, transport, true, "tutor", "ask")
	if !strings.Contains(human, "Privacy: private source "+sourceID+" is authorized; source text shared: yes") {
		t.Errorf("authorized ask does not state shared source text:\n%s", human)
	}
	human = mustTutorRun(t, db, clock, transport, true, "tutor", "history")
	if line := tutorLine(t, human, sourceID); !strings.Contains(line, "source-text-shared") {
		t.Errorf("history does not label a shared private call:\n%s", line)
	}

	human = mustTutorRun(t, db, clock, transport, true, "tutor", "revoke", sourceID, "--reviewer", "owner")
	if !strings.Contains(human, "Tutor consent for "+sourceID+" (import): withheld") {
		t.Errorf("revoke output does not state withholding:\n%s", human)
	}
	human = mustTutorRun(t, db, clock, transport, true, "tutor", "consents")
	if line := tutorLine(t, human, sourceID); !strings.Contains(line, "withheld") {
		t.Errorf("consents does not label a revoked source:\n%s", line)
	}
	human = mustTutorRun(t, db, clock, transport, true, "tutor", "ask")
	if !strings.Contains(human, "Privacy: private source "+sourceID+" is NOT authorized; source text shared: no") {
		t.Errorf("revoked ask does not state withheld source text:\n%s", human)
	}
}

func TestTutorMissingNodeRecordsNoSharedPrivateText(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := importTutorSource(t, db, clock)
	configureTutor(t, db, clock, &fakeTutorTransport{}, true)
	mustTutorRun(t, db, clock, &fakeTutorTransport{}, true, "tutor", "consent", sourceID, "--reviewer", "owner", "--json")
	preview := decodeTutorPreview(t, mustTutorRun(t, db, clock, &fakeTutorTransport{}, true, "tutor", "preview", "--json"))
	if !preview.Context.SourceTextShared {
		t.Fatalf("authorized preview does not prepare shared source text: %+v", preview.Context)
	}

	host, err := filepath.Abs(filepath.Join("..", "..", "agent", "tutor-host.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		tutor.CredentialEnv: fakeTutorKey,
		tutor.HostEnv:       host,
		tutor.NodeEnv:       filepath.Join(t.TempDir(), "missing-node"),
	}
	getenv := func(key string) string { return env[key] }
	var stdout, stderr bytes.Buffer
	options := Options{
		Stdin:          strings.NewReader(""),
		Stdout:         &stdout,
		Stderr:         &stderr,
		Now:            clock.Now,
		NewSession:     func() string { return "sess-test" },
		Getenv:         getenv,
		TutorTransport: tutor.HostTransport{Getenv: getenv, Environ: os.Environ},
	}
	if code := Run([]string{"tutor", "ask", "--json", "--db", db}, options); code != 3 {
		t.Fatalf("missing node exit = %d\n%s", code, stderr.String())
	}
	history := decodeTutorHistory(t, mustRun(t, db, clock, "", "tutor", "history", "--json"))
	if len(history.Calls) != 1 {
		t.Fatalf("history calls = %+v", history.Calls)
	}
	call := history.Calls[0]
	if call.Status != "error" || call.ErrorCode != "agent_unavailable" {
		t.Fatalf("missing node call = %+v", call)
	}
	if call.SourceTextShared {
		t.Fatal("a request that never reached the host is recorded as shared private text")
	}
	if call.SourceID != sourceID {
		t.Fatalf("source id = %q, want %q", call.SourceID, sourceID)
	}
}

func TestTutorDailyCountCountsFailures(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{response: fixedTutorError("provider_error", "down")}
	practiceSample(t, db, clock)
	configureTutor(t, db, clock, transport, true, "--daily-calls", "2")
	for i := range 2 {
		code, _, _ := runTutorCLI(t, db, clock, transport, true, "tutor", "ask", "--json")
		if code != 3 {
			t.Fatalf("failed call %d exit = %d", i, code)
		}
	}
	code, _, stderr := runTutorCLI(t, db, clock, transport, true, "tutor", "ask", "--json")
	if code != 2 || !strings.Contains(stderr, "budget_exhausted") {
		t.Fatalf("budget after failures = %d %s", code, stderr)
	}
}

func TestTutorHistoryAttemptFilter(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{response: fixedTutorResponse("Advice.")}
	practiceSample(t, db, clock)
	configureTutor(t, db, clock, transport, true)
	mustTutorRun(t, db, clock, transport, true, "tutor", "ask", "--attempt", "1", "--json")
	filtered := decodeTutorHistory(t, mustTutorRun(t, db, clock, transport, true, "tutor", "history", "--attempt", "1", "--json"))
	if len(filtered.Calls) != 1 {
		t.Fatalf("filtered calls = %+v", filtered.Calls)
	}
	code, _, stderr := runTutorCLI(t, db, clock, transport, true, "tutor", "history", "--attempt", "99", "--json")
	if code != 2 || !strings.Contains(stderr, "unknown_attempt") {
		t.Fatalf("unknown history attempt = %d %s", code, stderr)
	}
}

func TestTutorHistoryEmptyState(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	raw := mustTutorRun(t, db, clock, &fakeTutorTransport{}, true, "tutor", "history", "--json")
	doc := decodeTutorHistory(t, raw)
	if doc.Summary.Calls != 0 || len(doc.Calls) != 0 || doc.Summary.DailyLimit != nil {
		t.Fatalf("empty history = %+v", doc)
	}
	if !strings.Contains(raw, `"calls": []`) {
		t.Fatalf("empty history is not an array: %s", raw)
	}
}

func TestTutorStoreCallRecordShape(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{response: fixedTutorResponse("Advice.")}
	practiceSample(t, db, clock)
	configureTutor(t, db, clock, transport, true)
	mustTutorRun(t, db, clock, transport, true, "tutor", "ask", "--json")

	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	calls, err := st.TutorCalls(0)
	if err != nil || len(calls) != 1 {
		t.Fatalf("TutorCalls = %+v, %v", calls, err)
	}
	call := calls[0]
	if call.PromptVersion != tutor.PromptVersion || call.Status != "ok" || call.OutputText != "Advice." {
		t.Fatalf("stored call = %+v", call)
	}
	if call.CreatedAt.Before(goldenBase) {
		t.Fatalf("created_at = %v", call.CreatedAt)
	}
}
