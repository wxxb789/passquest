package cli

import (
	"bytes"
	"context"
	"encoding/json"
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

// blockingTutorTransport holds one call in flight until the test releases it,
// so a second invocation can be observed against the same daily budget.
type blockingTutorTransport struct {
	started  chan struct{}
	release  chan struct{}
	response tutor.HostResponse
}

func (b *blockingTutorTransport) Call(_ context.Context, _ tutor.HostRequest) (tutor.HostResponse, error) {
	close(b.started)
	<-b.release
	return b.response, nil
}

func tutorEnv(present bool) func(string) string {
	return func(key string) string {
		if key == tutor.CredentialEnv && present {
			return fakeTutorKey
		}
		return ""
	}
}

func newTutorOptions(stdout, stderr *bytes.Buffer, clock *stepClock, transport tutor.Transport, keyPresent bool) Options {
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

func runTutorCLI(t *testing.T, db string, clock *stepClock, transport tutor.Transport, keyPresent bool, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(append(args, "--db", db), newTutorOptions(&stdout, &stderr, clock, transport, keyPresent))
	return code, stdout.String(), stderr.String()
}

func mustTutorRun(t *testing.T, db string, clock *stepClock, transport tutor.Transport, keyPresent bool, args ...string) string {
	t.Helper()
	code, out, errOut := runTutorCLI(t, db, clock, transport, keyPresent, args...)
	if code != 0 {
		t.Fatalf("Run(%v) = %d\nstdout:\n%s\nstderr:\n%s", args, code, out, errOut)
	}
	return out
}

func configureTutor(t *testing.T, db string, clock *stepClock, transport tutor.Transport, keyPresent bool, extra ...string) string {
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestTutorUsageErrors(t *testing.T) {
	t.Parallel()
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

func TestTutorGoldenDocuments(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestTutorHistoryAttemptFilter(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
