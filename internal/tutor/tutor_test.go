package tutor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validConfig() Config {
	cfg := DefaultConfig()
	cfg.Endpoint = "https://api.example.com/v1/chat/completions"
	cfg.Model = "example-model"
	return cfg
}

func firstPartyTarget() Target {
	return Target{
		AttemptID:     7,
		Status:        "incorrect",
		SelectedLabel: "A",
		SelectedText:  "At 7 p.m.",
		CorrectLabel:  "B",
		CorrectText:   "At 4 p.m.",
		ElapsedMS:     2400,
		AttemptNumber: 1,
		Assistance:    "independent",
		Skills:        []string{"vocabulary-in-context"},
		Lessons: []SkillLesson{{
			ID: "vocabulary-in-context", Kind: "language", Title: "Vocabulary in context",
			Summary: "Use the sentence around a word.", Principle: "The sentence constrains the sense.",
			WhyItWorks: "Options paraphrase the sentence.", WhenItWorks: "When the stem asks about a word.",
			Steps:    []string{"Find the sentence.", "Name its purpose.", "Fit each option."},
			WatchOut: "A familiar translation can mislead.",
		}},
		ContentMatched: true,
		PassageText:    "The Westbridge Community Library closes at 4 p.m. on Saturdays. A visitor asked about the new hours.",
		QuestionStem:   "What time does the library close on Saturdays?",
		Options: []Option{
			{Label: "A", Text: "At 7 p.m."},
			{Label: "B", Text: "At 4 p.m."},
			{Label: "C", Text: "At 9 a.m."},
		},
		Evidence:  "closes at 4 p.m. on Saturdays",
		Reasoning: "The notice gives the Saturday closing time directly.",
	}
}

func privateTarget() Target {
	target := firstPartyTarget()
	target.PrivateSource = true
	target.SourceID = "src-secret"
	return target
}

func testMode(t *testing.T, mode Mode) string {
	t.Helper()
	preview, err := Build(validConfig(), mode, firstPartyTarget())
	if err != nil {
		t.Fatalf("Build(%s): %v", mode, err)
	}
	return preview.Context.PromptVersion + "\n" + preview.Request.Chat.Messages[1].Content
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{"valid https", func(*Config) {}, ""},
		{"valid loopback http", func(c *Config) { c.Endpoint = "http://127.0.0.1:8080/v1/chat/completions" }, ""},
		{"localhost http", func(c *Config) { c.Endpoint = "http://localhost:1234/v1/chat/completions" }, ""},
		{"remote http", func(c *Config) { c.Endpoint = "http://api.example.com/v1/chat/completions" }, "off-loopback"},
		{"userinfo", func(c *Config) { c.Endpoint = "https://user:secret@api.example.com/v1" }, "userinfo"},
		{"query", func(c *Config) { c.Endpoint = "https://api.example.com/v1?key=abc" }, "query"},
		{"fragment", func(c *Config) { c.Endpoint = "https://api.example.com/v1#x" }, "fragment"},
		{"relative", func(c *Config) { c.Endpoint = "/v1/chat" }, "absolute"},
		{"ftp", func(c *Config) { c.Endpoint = "ftp://api.example.com/v1" }, "http or https"},
		{"empty model", func(c *Config) { c.Model = "" }, "model is required"},
		{"control in model", func(c *Config) { c.Model = "bad\nmodel" }, "control character"},
		{"short timeout", func(c *Config) { c.TimeoutMS = 999 }, "timeout_ms"},
		{"long timeout", func(c *Config) { c.TimeoutMS = 120001 }, "timeout_ms"},
		{"small tokens", func(c *Config) { c.MaxOutputTokens = 63 }, "max_output_tokens"},
		{"large tokens", func(c *Config) { c.MaxOutputTokens = 2049 }, "max_output_tokens"},
		{"zero daily", func(c *Config) { c.DailyCalls = 0 }, "daily_calls"},
		{"large daily", func(c *Config) { c.DailyCalls = 201 }, "daily_calls"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validConfig()
			test.mutate(&cfg)
			err := cfg.Validate()
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestProviderHostOmitsPathAndQuery(t *testing.T) {
	cfg := validConfig()
	cfg.Endpoint = "https://api.example.com:8443/v1/chat/completions"
	if got, want := cfg.ProviderHost(), "api.example.com:8443"; got != want {
		t.Fatalf("ProviderHost = %q, want %q", got, want)
	}
}

func TestBuildFirstPartyIsGrounded(t *testing.T) {
	preview, err := Build(validConfig(), ModeExplain, firstPartyTarget())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	user := preview.Request.Chat.Messages[1].Content
	for _, want := range []string{
		"The Westbridge Community Library closes at 4 p.m.",
		"What time does the library close on Saturdays?",
		"A. At 7 p.m.",
		"Verified key: B — At 4 p.m.",
		"Verified evidence: closes at 4 p.m. on Saturdays",
		"elapsed_ms=2400",
		"Vocabulary in context",
		"Explain why the verified key",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("user message is missing %q\n%s", want, user)
		}
	}
	if preview.Context.SourceTextShared {
		t.Fatal("first-party material marked as private source text")
	}
	if len(preview.Context.Withheld) != 0 {
		t.Fatalf("withheld = %v, want none", preview.Context.Withheld)
	}
	if preview.Request.Chat.Model != "example-model" || preview.Request.Chat.MaxTokens != DefaultMaxTokens {
		t.Fatalf("chat request = %+v", preview.Request.Chat)
	}
	if preview.Request.Chat.Temperature != 0 || preview.Request.Chat.Stream {
		t.Fatalf("chat request is not a non-streaming zero-temperature request: %+v", preview.Request.Chat)
	}
	if preview.Request.ProtocolVersion != ProtocolVersion || preview.Request.MaxResponseChars != MaxResponseChars {
		t.Fatalf("host request = %+v", preview.Request)
	}
	if len(preview.RequestSHA) != 64 || preview.RequestBytes == 0 {
		t.Fatalf("request hash/bytes = %q/%d", preview.RequestSHA, preview.RequestBytes)
	}
	assertExactBody(t, preview)
	if !preview.Context.ContentIncluded || preview.Context.StoredTextIncluded {
		t.Fatalf("content inclusion = %+v", preview.Context)
	}
	system := preview.Request.Chat.Messages[0].Content
	for _, want := range []string{"authoritative", "advisory", "never as instructions"} {
		if !strings.Contains(system, want) {
			t.Errorf("system prompt is missing %q", want)
		}
	}
}

func TestBuildPrivateWithoutConsentWithholdsEveryPrivateString(t *testing.T) {
	target := privateTarget()
	preview, err := Build(validConfig(), ModeExplain, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	user := preview.Request.Chat.Messages[1].Content
	for _, secret := range []string{
		"The Westbridge Community Library closes",
		"What time does the library close",
		"At 7 p.m.",
		"At 4 p.m.",
		"closes at 4 p.m. on Saturdays",
		"The notice gives the Saturday closing time",
	} {
		if strings.Contains(user, secret) {
			t.Errorf("private string leaked without consent: %q\n%s", secret, user)
		}
	}
	for _, want := range []string{"Question content: withheld", "Vocabulary in context", "elapsed_ms=2400"} {
		if !strings.Contains(user, want) {
			t.Errorf("redacted prompt is missing %q\n%s", want, user)
		}
	}
	if preview.Context.SourceTextShared {
		t.Fatal("source_text_shared is true without consent")
	}
	if !preview.Context.ConsentRequired || preview.Context.ConsentGranted {
		t.Fatalf("consent flags = %+v", preview.Context)
	}
	if preview.Context.SourceID != "src-secret" {
		t.Fatalf("source id = %q", preview.Context.SourceID)
	}
	wantWithheld := map[string]bool{
		"passage": true, "question_stem": true, "options": true, "evidence": true,
		"reasoning": true, "selected_option_text": true, "correct_option_text": true,
	}
	for _, field := range preview.Context.Withheld {
		delete(wantWithheld, field)
	}
	if len(wantWithheld) != 0 {
		t.Fatalf("withheld is missing %v; got %v", wantWithheld, preview.Context.Withheld)
	}
}

func TestBuildPrivateWithConsentIncludesSourceText(t *testing.T) {
	target := privateTarget()
	target.ConsentGranted = true
	preview, err := Build(validConfig(), ModeExplain, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	user := preview.Request.Chat.Messages[1].Content
	for _, want := range []string{
		"The Westbridge Community Library closes",
		"What time does the library close",
		"Verified evidence: closes at 4 p.m. on Saturdays",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("authorized prompt is missing %q\n%s", want, user)
		}
	}
	if !preview.Context.SourceTextShared {
		t.Fatal("source_text_shared is false with consent")
	}
	if preview.Context.ConsentRequired != true || preview.Context.ConsentGranted != true {
		t.Fatalf("consent flags = %+v", preview.Context)
	}
	if len(preview.Context.Withheld) != 0 {
		t.Fatalf("withheld = %v, want none", preview.Context.Withheld)
	}
}

func TestBuildChangedContentUsesStoredTextOnly(t *testing.T) {
	target := privateTarget()
	target.ConsentGranted = true
	target.ContentChanged = true
	preview, err := Build(validConfig(), ModeExplain, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	user := preview.Request.Chat.Messages[1].Content
	if strings.Contains(user, "The Westbridge Community Library closes") {
		t.Fatalf("changed passage was sent:\n%s", user)
	}
	if strings.Contains(user, "What time does the library close") {
		t.Fatalf("changed question was sent:\n%s", user)
	}
	for _, want := range []string{
		"Recorded selection: A — At 7 p.m.",
		"Recorded verified key: B — At 4 p.m.",
		"differs from the version recorded",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("stored-text prompt is missing %q\n%s", want, user)
		}
	}
	if !preview.Context.ContentChanged || preview.Context.ContentUnavailable {
		t.Fatalf("content flags = %+v", preview.Context)
	}
	if preview.Context.ContentIncluded || !preview.Context.StoredTextIncluded {
		t.Fatalf("content inclusion = %+v", preview.Context)
	}
	if !preview.Context.SourceTextShared {
		t.Fatal("authorized stored text is not marked shared")
	}
}

func TestBuildUnavailableContentDisclosesAndKeepsStoredLabels(t *testing.T) {
	target := firstPartyTarget()
	target.ContentMatched = false
	preview, err := Build(validConfig(), ModeProbe, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	user := preview.Request.Chat.Messages[1].Content
	if !strings.Contains(user, "Question content: withheld") || !strings.Contains(user, "Recorded verified key: B — At 4 p.m.") {
		t.Fatalf("unavailable-content prompt:\n%s", user)
	}
	if !preview.Context.ContentUnavailable {
		t.Fatalf("context = %+v", preview.Context)
	}
	if preview.Context.ContentIncluded || !preview.Context.StoredTextIncluded {
		t.Fatalf("content inclusion = %+v", preview.Context)
	}
}

func TestBuildModesChangeOnlyTheTask(t *testing.T) {
	explain := testMode(t, ModeExplain)
	probe := testMode(t, ModeProbe)
	lesson := testMode(t, ModeMicroLesson)
	if !strings.Contains(explain, "Explain why the verified key") {
		t.Fatalf("explain task missing:\n%s", explain)
	}
	if !strings.Contains(probe, "diagnostic questions") {
		t.Fatalf("probe task missing:\n%s", probe)
	}
	if !strings.Contains(lesson, "tactical micro-lesson") {
		t.Fatalf("micro-lesson task missing:\n%s", lesson)
	}
	if explain == probe || probe == lesson {
		t.Fatal("modes produced identical prompts")
	}
}

// TestBuildTaskMatchesRecordedOutcome pins that a correct selection is not
// criticized as a misconception and a skipped attempt is not asked about a
// selected option, whether or not the question content is shared.
func TestBuildTaskMatchesRecordedOutcome(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		mode    Mode
		private bool
		want    []string
		notWant []string
	}{
		{
			name: "incorrect explain", status: "incorrect", mode: ModeExplain,
			want: []string{"recorded selection is incorrect", "Explain why the verified key fits the evidence and why that selection does not"},
		},
		{
			name: "correct explain", status: "correct", mode: ModeExplain,
			want:    []string{"recorded selection is correct", "why the other options do not"},
			notWant: []string{"selected option does not", "misconception"},
		},
		{
			name: "skipped explain", status: "skipped", mode: ModeExplain,
			want:    []string{"no selected option", "where that evidence is"},
			notWant: []string{"selected option does not", "misconception"},
		},
		{
			name: "incorrect probe", status: "incorrect", mode: ModeProbe,
			want: []string{"misconception behind that selection"},
		},
		{
			name: "correct probe", status: "correct", mode: ModeProbe,
			want:    []string{"reasoning that produced it"},
			notWant: []string{"misconception"},
		},
		{
			name: "skipped probe", status: "skipped", mode: ModeProbe,
			want:    []string{"no selected option to probe", "what blocked them"},
			notWant: []string{"misconception"},
		},
		{
			name: "correct explain withheld", status: "correct", mode: ModeExplain, private: true,
			want:    []string{"recorded outcome is correct", "tactic that makes the verified key a reliable choice"},
			notWant: []string{"selected option does not", "other options"},
		},
		{
			name: "skipped probe withheld", status: "skipped", mode: ModeProbe, private: true,
			want:    []string{"no selected option to probe", "what blocked them"},
			notWant: []string{"misconception"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := firstPartyTarget()
			if test.private {
				target = privateTarget()
			}
			target.Status = test.status
			preview, err := Build(validConfig(), test.mode, target)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			user := preview.Request.Chat.Messages[1].Content
			for _, want := range test.want {
				if !strings.Contains(user, want) {
					t.Errorf("task is missing %q:\n%s", want, user)
				}
			}
			for _, notWant := range test.notWant {
				if strings.Contains(user, notWant) {
					t.Errorf("task unexpectedly contains %q:\n%s", notWant, user)
				}
			}
		})
	}
}

func TestBuildRejectsUnknownModeAndMissingAttempt(t *testing.T) {
	if _, err := Build(validConfig(), Mode("chat"), firstPartyTarget()); err == nil {
		t.Fatal("unknown mode was accepted")
	}
	target := firstPartyTarget()
	target.AttemptID = 0
	if _, err := Build(validConfig(), ModeExplain, target); err == nil {
		t.Fatal("missing attempt id was accepted")
	}
}

func TestBuildClipsLongPassageAndFields(t *testing.T) {
	target := firstPartyTarget()
	target.PassageText = strings.Repeat("earlier filler sentence. ", 200) +
		"The library closes at 4 p.m. on Saturdays." + strings.Repeat(" later filler sentence.", 200)
	target.QuestionStem = strings.Repeat("stem ", 400)
	target.Options[1].Text = strings.Repeat("option ", 200)
	preview, err := Build(validConfig(), ModeExplain, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	user := preview.Request.Chat.Messages[1].Content
	if !strings.Contains(user, "The library closes at 4 p.m. on Saturdays.") {
		t.Fatalf("evidence sentence fell outside the excerpt:\n%s", user)
	}
	if !strings.Contains(user, "[earlier text omitted]") || !strings.Contains(user, "[later text omitted]") {
		t.Fatalf("passage excerpt is not marked:\n%s", user)
	}
	if !strings.Contains(user, "…[truncated]") {
		t.Fatalf("long fields are not marked:\n%s", user)
	}
	for _, field := range []string{"passage", "question_stem", "option_B"} {
		if !contains(preview.Context.TruncatedFields, field) {
			t.Errorf("truncated fields = %v, want %s", preview.Context.TruncatedFields, field)
		}
	}
}

func TestBuildClipsRecordedOptionTextsInEveryBranch(t *testing.T) {
	longOption := strings.Repeat("recorded option ", 1_500)
	longSelection := strings.Repeat("selected answer ", 1_500)
	tests := []struct {
		name   string
		mutate func(*Target)
	}{
		{"current content", func(*Target) {}},
		{"changed content", func(target *Target) { target.ContentChanged = true }},
		{"unavailable content", func(target *Target) { target.ContentMatched = false }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := firstPartyTarget()
			target.Options[1].Text = longOption
			target.CorrectText = longOption
			target.SelectedText = longSelection
			test.mutate(&target)
			preview, err := Build(validConfig(), ModeExplain, target)
			if err != nil {
				t.Fatalf("Build rejected a prompt with long recorded option text: %v", err)
			}
			user := preview.Request.Chat.Messages[1].Content
			if strings.Contains(user, longOption) || strings.Contains(user, longSelection) {
				t.Fatal("recorded option text was sent unclipped")
			}
			for _, field := range []string{"selected_option_text", "correct_option_text"} {
				if !contains(preview.Context.TruncatedFields, field) {
					t.Errorf("truncated fields = %v, want %s", preview.Context.TruncatedFields, field)
				}
			}
		})
	}
}

func TestBuildRejectsPromptOverTheTotalBound(t *testing.T) {
	target := firstPartyTarget()
	target.Options = nil
	for i := range 40 {
		target.Options = append(target.Options, Option{
			Label: fmt.Sprintf("o%02d", i),
			Text:  strings.Repeat("x", maxOptionRunes),
		})
	}
	_, err := Build(validConfig(), ModeExplain, target)
	if err == nil || !strings.Contains(err.Error(), "limit is 20000") {
		t.Fatalf("Build error = %v, want the total prompt bound", err)
	}
}

func TestBuildRequestHashIsStable(t *testing.T) {
	first, err := Build(validConfig(), ModeExplain, firstPartyTarget())
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(validConfig(), ModeExplain, firstPartyTarget())
	if err != nil {
		t.Fatal(err)
	}
	if first.RequestSHA != second.RequestSHA {
		t.Fatalf("request hash changed: %s vs %s", first.RequestSHA, second.RequestSHA)
	}
	other, err := Build(validConfig(), ModeProbe, firstPartyTarget())
	if err != nil {
		t.Fatal(err)
	}
	if first.RequestSHA == other.RequestSHA {
		t.Fatal("mode change did not change the request hash")
	}
}

// assertExactBody pins that RequestSHA and RequestBytes describe Request.Body,
// and that Body is the exact serialization of the typed chat the preview shows.
func assertExactBody(t *testing.T, preview Preview) {
	t.Helper()
	encoded, err := json.Marshal(preview.Request.Chat)
	if err != nil {
		t.Fatalf("marshal chat: %v", err)
	}
	if preview.Request.Body != string(encoded) {
		t.Fatalf("request body is not the serialized chat:\n%s", preview.Request.Body)
	}
	sum := sha256.Sum256([]byte(preview.Request.Body))
	if preview.RequestSHA != hex.EncodeToString(sum[:]) {
		t.Fatalf("request sha %s does not hash the body", preview.RequestSHA)
	}
	if preview.RequestBytes != len(preview.Request.Body) {
		t.Fatalf("request bytes %d != body length %d", preview.RequestBytes, len(preview.Request.Body))
	}
	if !json.Valid([]byte(preview.Request.Body)) {
		t.Fatalf("request body is not JSON: %s", preview.Request.Body)
	}
}

// TestBuildHashesTheExactProviderBody covers content Go escapes and
// JSON.stringify does not: the recorded hash must cover the bytes posted.
func TestBuildHashesTheExactProviderBody(t *testing.T) {
	target := privateTarget()
	target.ConsentGranted = true
	target.PassageText = "Marks & Spencer <tag> opens at 4 p.m."
	target.QuestionStem = "Which option matches Marks & Spencer <tag>?"
	preview, err := Build(validConfig(), ModeExplain, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, escaped := range []string{`\u0026`, `\u003c`, `\u003e`} {
		if !strings.Contains(preview.Request.Body, escaped) {
			t.Fatalf("serialized body lacks %s:\n%s", escaped, preview.Request.Body)
		}
	}
	if !strings.Contains(preview.Request.Chat.Messages[1].Content, "Marks & Spencer <tag>") {
		t.Fatal("chat content lost the literal characters")
	}
	assertExactBody(t, preview)
}

func TestBuildRecordsPreV6Fallback(t *testing.T) {
	target := firstPartyTarget()
	target.SkillsMappedFromCurrentTags = true
	preview, err := Build(validConfig(), ModeExplain, target)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Context.SkillsMappedFromCurrentTags {
		t.Fatal("fallback flag was lost")
	}
}

func TestBuildPrivateStoredTextWithoutConsentIsWithheld(t *testing.T) {
	target := privateTarget()
	target.ContentMatched = false
	preview, err := Build(validConfig(), ModeExplain, target)
	if err != nil {
		t.Fatal(err)
	}
	user := preview.Request.Chat.Messages[1].Content
	if strings.Contains(user, "At 4 p.m.") || strings.Contains(user, "At 7 p.m.") {
		t.Fatalf("stored option text leaked:\n%s", user)
	}
	if preview.Context.SourceTextShared {
		t.Fatal("stored private text marked shared without consent")
	}
	if preview.Context.ContentIncluded || preview.Context.StoredTextIncluded {
		t.Fatalf("private content inclusion = %+v", preview.Context)
	}
}

func TestDecodeResponse(t *testing.T) {
	req := HostRequest{MaxResponseChars: 100}
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{
			name: "ok",
			raw:  `{"protocol_version":1,"status":"ok","text":"short answer","http_status":200,"latency_ms":12,"request_sent":true,"usage":{"prompt_tokens":3,"completion_tokens":4}}`,
			want: "short answer",
		},
		{
			name:    "unknown key",
			raw:     `{"protocol_version":1,"status":"ok","text":"answer","latency_ms":1,"request_sent":true,"extra":true}`,
			wantErr: "unknown field",
		},
		{
			name:    "wrong protocol",
			raw:     `{"protocol_version":2,"status":"ok","text":"answer","latency_ms":1,"request_sent":true}`,
			wantErr: "protocol",
		},
		{
			name:    "empty text",
			raw:     `{"protocol_version":1,"status":"ok","text":" ","latency_ms":1,"request_sent":true}`,
			wantErr: "empty text",
		},
		{
			name:    "oversized text",
			raw:     `{"protocol_version":1,"status":"ok","text":"` + strings.Repeat("x", 101) + `","latency_ms":1,"request_sent":true}`,
			wantErr: "limit",
		},
		{
			name:    "terminal control",
			raw:     `{"protocol_version":1,"status":"ok","text":"\u001b[31manswer","latency_ms":1,"request_sent":true}`,
			wantErr: "terminal control",
		},
		{
			name:    "negative usage",
			raw:     `{"protocol_version":1,"status":"ok","text":"answer","latency_ms":1,"request_sent":true,"usage":{"prompt_tokens":-1,"completion_tokens":0}}`,
			wantErr: "negative token usage",
		},
		{
			name:    "error",
			raw:     `{"protocol_version":1,"status":"error","error":{"code":"rate_limited","message":"slow down"},"latency_ms":2}`,
			wantErr: "",
		},
		{
			name: "unknown error code",
			raw:  `{"protocol_version":1,"status":"error","error":{"code":"made_up","message":"detail"},"latency_ms":2}`,
		},
		{
			name:    "error without detail",
			raw:     `{"protocol_version":1,"status":"error","latency_ms":2}`,
			wantErr: "no error",
		},
		{
			name:    "ok without request_sent",
			raw:     `{"protocol_version":1,"status":"ok","text":"answer","latency_ms":1}`,
			wantErr: "sent request",
		},
		{
			name:    "trailing data",
			raw:     `{"protocol_version":1,"status":"ok","text":"answer","latency_ms":1,"request_sent":true}{}`,
			wantErr: "trailing",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resp, err := decodeResponse([]byte(test.raw), req)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeResponse: %v", err)
			}
			if test.want != "" && resp.Text != test.want {
				t.Fatalf("text = %q, want %q", resp.Text, test.want)
			}
			if test.name == "unknown error code" && resp.Error.Code != "host_error" {
				t.Fatalf("unknown code = %q, want host_error", resp.Error.Code)
			}
		})
	}
}

func TestResolveHostHonorsEnvironmentOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "host.mjs")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env node\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveHost(func(key string) string {
		if key == HostEnv {
			return path
		}
		return ""
	}, func() (string, error) { return dir, nil }, func() (string, error) { return filepath.Join(dir, "passquest"), nil })
	if err != nil || got != path {
		t.Fatalf("ResolveHost = %q, %v; want %q", got, err, path)
	}
}

func TestResolveHostFindsWorkingDirectoryHost(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent", "tutor-host.mjs")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/usr/bin/env node\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveHost(func(string) string { return "" },
		func() (string, error) { return dir, nil },
		func() (string, error) { return filepath.Join(dir, "bin", "passquest"), nil })
	if err != nil || got != path {
		t.Fatalf("ResolveHost = %q, %v; want %q", got, err, path)
	}
}

func TestHostTransportRejectsBadRequestBeforeSpawn(t *testing.T) {
	transport := HostTransport{
		Getenv:  func(string) string { return "" },
		Environ: func() []string { return nil },
		Getwd:   func() (string, error) { return t.TempDir(), nil },
		Executable: func() (string, error) {
			return filepath.Join(t.TempDir(), "passquest"), nil
		},
	}
	req := HostRequest{ProtocolVersion: ProtocolVersion, Endpoint: "https://example.com/v1", TimeoutMS: 30000, MaxResponseChars: 100, Chat: Chat{Model: "m", Messages: []Message{{Role: "system", Content: "s"}, {Role: "user", Content: "u"}}, MaxTokens: 10}}
	if _, err := transport.Call(context.Background(), req); err == nil {
		t.Fatal("invalid max_tokens was accepted")
	}
}

func TestHostTransportRejectsBodyThatIsNotTheChat(t *testing.T) {
	transport := HostTransport{
		Getenv:  func(string) string { return "" },
		Environ: func() []string { return nil },
		Getwd:   func() (string, error) { return t.TempDir(), nil },
		Executable: func() (string, error) {
			return filepath.Join(t.TempDir(), "passquest"), nil
		},
	}
	chat := Chat{
		Model:       "m",
		Messages:    []Message{{Role: "system", Content: "s"}, {Role: "user", Content: "u"}},
		MaxTokens:   350,
		Temperature: 0,
	}
	encoded, err := json.Marshal(chat)
	if err != nil {
		t.Fatal(err)
	}
	req := HostRequest{
		ProtocolVersion:  ProtocolVersion,
		Endpoint:         "https://example.com/v1",
		TimeoutMS:        30000,
		MaxResponseChars: 100,
		Chat:             chat,
	}
	for _, body := range []string{"", strings.Replace(string(encoded), `"u"`, `"u!"`, 1)} {
		req.Body = body
		_, err := transport.Call(context.Background(), req)
		if err == nil || !strings.Contains(err.Error(), "serialized chat") {
			t.Fatalf("body %q error = %v, want a body/chat mismatch", body, err)
		}
	}
	req.Body = string(encoded)
	if _, err := transport.Call(context.Background(), req); err == nil || strings.Contains(err.Error(), "serialized chat") {
		t.Fatalf("matching body was rejected as a mismatch: %v", err)
	}
}

func TestAgentUnavailableIsADocumentedCode(t *testing.T) {
	if !ValidHostErrorCode("agent_unavailable") {
		t.Fatal("agent_unavailable is not documented")
	}
}

func TestContainsUnsafeControl(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"plain advice\nwith a newline\tand tab", false},
		{"\x1b[31mred\x1b[0m", true},
		{"bell\x07", true},
		{"delete\x7f", true},
		{"c1\u009b", true},
	}
	for _, test := range tests {
		if got := ContainsUnsafeControl(test.text); got != test.want {
			t.Errorf("ContainsUnsafeControl(%q) = %v, want %v", test.text, got, test.want)
		}
	}
}

func TestLimitedBufferRecordsTruncation(t *testing.T) {
	var buffer limitedBuffer
	buffer.limit = 4
	n, err := buffer.Write([]byte("abcdef"))
	if err != nil || n != 6 {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if !buffer.truncated || buffer.String() != "abcd" {
		t.Fatalf("buffer = %q truncated=%v", buffer.String(), buffer.truncated)
	}
}

func TestPreviewJSONShapeIsStable(t *testing.T) {
	preview, err := Build(validConfig(), ModeExplain, firstPartyTarget())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(preview.Request)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"protocol_version":1`) || !strings.Contains(string(encoded), `"max_tokens":350`) {
		t.Fatalf("host request JSON = %s", encoded)
	}
	if !strings.Contains(string(encoded), `"body":"{\"model\":\"example-model\"`) {
		t.Fatalf("host request JSON has no exact body field = %s", encoded)
	}
}

func TestBuildOmitsExtraLessonsWithinThePromptBound(t *testing.T) {
	target := firstPartyTarget()
	target.Lessons = nil
	target.Skills = nil
	for i := range 4 {
		id := "lesson-" + string(rune('a'+i))
		target.Skills = append(target.Skills, id)
		target.Lessons = append(target.Lessons, SkillLesson{
			ID:        id,
			Kind:      "language",
			Title:     id,
			Summary:   strings.Repeat("summary ", 300),
			Principle: strings.Repeat("principle ", 300),
		})
	}
	preview, err := Build(validConfig(), ModeExplain, target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(preview.Request.Chat.Messages[1].Content, "Additional skill lessons omitted") {
		t.Fatalf("omitted lessons are not disclosed:\n%s", preview.Request.Chat.Messages[1].Content)
	}
	found := false
	for _, field := range preview.Context.TruncatedFields {
		if strings.HasSuffix(field, "_omitted") {
			found = true
		}
	}
	if !found {
		t.Fatalf("truncated fields = %v, want an omitted lesson", preview.Context.TruncatedFields)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
