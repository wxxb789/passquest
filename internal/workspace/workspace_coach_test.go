package workspace

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/studio"
)

func TestWorkspaceCoachPreviewAndFailure(t *testing.T) {
	t.Parallel()
	transport := &fakeTutorTransport{response: fixedTutorResponse("OK")}
	f := newWSFixture(t, func(o *Options) { o.TutorTransport = transport })
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

// TestWorkspaceCoachHistory lists an attempt's calls with each advisory
// reply's text and each failure's code, and nothing that was sent.
func TestWorkspaceCoachHistory(t *testing.T) {
	t.Parallel()
	transport := &fakeTutorTransport{response: fixedTutorResponse("OK")}
	f := newWSFixture(t, func(o *Options) { o.TutorTransport = transport })
	f.env[provider.RegistryEnv] = modelRegistryFile(t, modelProvider("alpha", "http://127.0.0.1:9/v1/chat/completions"))
	f.env[provider.CredentialEnvFor("alpha")] = fakeTutorKey
	f.reopen(t)
	if _, err := f.w.CoachHistory(0); studioCode(err) != "invalid_input" {
		t.Fatalf("history without an attempt = %v", err)
	}
	if _, err := f.w.CoachHistory(7); studioCode(err) != "no_attempts" {
		t.Fatalf("history on an empty database = %v", err)
	}
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample})
	mustNoErr(t, err)
	feedback, err := f.w.Answer(item.Token, "A")
	mustNoErr(t, err)
	empty, err := f.w.CoachHistory(feedback.AttemptID)
	mustNoErr(t, err)
	if empty.AttemptID != feedback.AttemptID || empty.Calls == nil || len(empty.Calls) != 0 || len(empty.Limits) == 0 {
		t.Fatalf("empty history = %+v", empty)
	}
	_, err = f.w.UseModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	_, err = f.w.TestModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	transport.response = fixedTutorResponse("Read the <b>second</b> sentence again.")
	reply, err := f.w.CoachAsk(feedback.AttemptID, studio.CoachExplain)
	mustNoErr(t, err)
	transport.response = fixedTutorError("rate_limited", "slow down")
	failed, err := f.w.CoachAsk(feedback.AttemptID, studio.CoachProbe)
	mustNoErr(t, err)

	history, err := f.w.CoachHistory(feedback.AttemptID)
	mustNoErr(t, err)
	if len(history.Calls) != 2 {
		t.Fatalf("history = %+v", history)
	}
	first, second := history.Calls[0], history.Calls[1]
	if first.CallID != reply.CallID || first.State != "advisory" || first.Text != "Read the <b>second</b> sentence again." ||
		first.Mode != studio.CoachExplain || first.Provider != "alpha" || first.Model != "alpha-text" ||
		!first.Advisory || first.VerifiedAnswer || first.Failure != nil || first.CreatedAt == "" {
		t.Fatalf("advisory call = %+v", first)
	}
	if second.CallID != failed.CallID || second.State != "failed" || second.Text != "" || second.Mode != studio.CoachProbe ||
		second.Failure == nil || second.Failure.Code != failed.Failure.Code {
		t.Fatalf("failed call = %+v", second)
	}
	raw, _ := json.Marshal(history)
	for _, leak := range []string{fakeTutorKey, `"messages"`, `"body"`, "request_sha256", "\"system\""} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("history carries %s: %s", leak, raw)
		}
	}
	if _, err := f.w.CoachHistory(feedback.AttemptID + 5); studioCode(err) != "unknown_attempt" {
		t.Fatalf("unknown attempt = %v", err)
	}
}

// TestWorkspaceCoachPreviewReadsTheResponsesDocument pins the window's preview
// against a responses endpoint: its messages come from the document the host
// validates, not from a nil chat field.
func TestWorkspaceCoachPreviewReadsTheResponsesDocument(t *testing.T) {
	t.Parallel()
	transport := &fakeTutorTransport{response: fixedTutorResponse("OK")}
	f := newWSFixture(t, func(o *Options) { o.TutorTransport = transport })
	responsesProvider := modelProvider("alpha", "http://127.0.0.1:9/v1/responses")
	responsesProvider["endpoint_shape"] = provider.ShapeResponses
	registry := modelRegistryFile(t, responsesProvider)
	f.env[provider.RegistryEnv] = registry
	f.env[provider.CredentialEnvFor("alpha")] = fakeTutorKey
	f.reopen(t)
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample})
	mustNoErr(t, err)
	feedback, err := f.w.Answer(item.Token, "A")
	mustNoErr(t, err)
	_, err = f.w.UseModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	_, err = f.w.TestModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	if len(transport.requests) != 1 || transport.requests[0].Responses == nil || transport.requests[0].Chat != nil {
		t.Fatalf("model test request = %+v", transport.requests)
	}
	preview, err := f.w.CoachPreview(feedback.AttemptID, studio.CoachExplain)
	mustNoErr(t, err)
	if len(preview.Messages) != 2 || preview.Messages[0].Role != "system" || preview.Messages[1].Role != "user" {
		t.Fatalf("preview messages = %+v", preview.Messages)
	}
	if !strings.Contains(preview.Messages[0].Content, "text study coach") {
		t.Fatalf("preview system message = %q", preview.Messages[0].Content)
	}
	if preview.MaxTokens == 0 || preview.RequestSHA256 == "" || preview.RequestBytes == 0 {
		t.Fatalf("preview identity = %+v", preview)
	}
}
