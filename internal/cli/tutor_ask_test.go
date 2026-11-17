package cli

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/tutor"
)

func TestTutorAskSuccessPreservesAttemptsAndNeverStoresKey(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestTutorProviderFailureIsRecordedAndPracticeStillWorks(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestTutorAskReservesTheBudgetBeforeContactingTheProvider(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	blocking := &blockingTutorTransport{
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		response: fixedTutorResponse("Short advice."),
	}
	second := &fakeTutorTransport{response: fixedTutorResponse("Second advice.")}
	practiceSample(t, db, clock)
	configureTutor(t, db, clock, blocking, true, "--daily-calls", "1")

	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(blocking.release) }) }
	defer release()

	type result struct {
		code   int
		stderr string
	}
	done := make(chan result, 1)
	go func() {
		code, _, stderr := runTutorCLI(t, db, &stepClock{base: goldenBase}, blocking, true, "tutor", "ask", "--json")
		done <- result{code: code, stderr: stderr}
	}()
	select {
	case <-blocking.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the first ask never reached the transport")
	}

	code, _, stderr := runTutorCLI(t, db, clock, second, true, "tutor", "ask", "--json")
	if code != 2 || !strings.Contains(stderr, "budget_exhausted") {
		t.Fatalf("second ask while the first is in flight = %d %s", code, stderr)
	}
	if len(second.requests) != 0 {
		t.Fatal("a second provider request was made while the first held the only slot")
	}

	release()
	first := <-done
	if first.code != 0 {
		t.Fatalf("first ask = %d %s", first.code, first.stderr)
	}

	// The finalized call replaced its reservation: one of two slots is free.
	configureTutor(t, db, clock, second, true, "--daily-calls", "2")
	mustTutorRun(t, db, clock, second, true, "tutor", "ask", "--json")
	history := decodeTutorHistory(t, mustTutorRun(t, db, clock, second, true, "tutor", "history", "--json"))
	if history.Summary.Calls != 2 || history.Summary.DailyCalls != 2 {
		t.Fatalf("history after two asks = %+v", history.Summary)
	}
}

func TestTutorMissingNodeLeavesPracticeWorking(t *testing.T) {
	t.Parallel()
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

func TestTutorMissingNodeRecordsNoSharedPrivateText(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
