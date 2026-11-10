package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/adaptive"
	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/skill"
)

func sampleAdaptiveResult() adaptive.Result {
	scoring := adaptive.Skill{ID: "not-except-polarity", Kind: "scoring", Title: "NOT/EXCEPT polarity"}
	return adaptive.Result{
		Skills: []adaptive.SkillResult{
			{
				Skill: scoring,
				Evidence: adaptive.Evidence{
					Attempts: 2, AssistedAttempts: 1, GuidedAttempts: 1,
					IndependentAttempts: 1, IndependentCorrect: 0, IndependentIncorrect: 1,
					DistinctPassages: 1, LatestStatus: attempt.StatusIncorrect,
				},
				Inference: adaptive.Inference{State: adaptive.StateWeak, Action: adaptive.ActionExplain, Reason: "one incorrect independent answer"},
			},
			{
				Skill:     adaptive.Skill{ID: "vocabulary-in-context", Kind: "language", Title: "Vocabulary in context"},
				Evidence:  adaptive.Evidence{Attempts: 0},
				Inference: adaptive.Inference{State: adaptive.StateNoEvidence, Action: adaptive.ActionExplain, Reason: "no independent transfer evidence"},
			},
		},
		Recommendation: &adaptive.Recommendation{
			Action: adaptive.ActionExplain, Skill: scoring, State: adaptive.StateWeak,
			Reason: "one incorrect independent answer", EvidenceAttemptIDs: []int64{1},
		},
	}
}

func sampleRecords() []adaptive.Record {
	return []adaptive.Record{
		{
			AttemptID: 1, ContentID: "src-private-p001", QuestionID: "SECRET-QUESTION",
			AttemptNumber: 1, Assistance: attempt.AssistanceAssisted, Status: attempt.StatusIncorrect,
			ElapsedMS: 4200, Skills: []string{"not-except-polarity"}, TacticID: "not-except-polarity",
		},
		{
			AttemptID: 2, ContentID: "src-private-p002", QuestionID: "SECRET-QUESTION-2",
			AttemptNumber: 2, Assistance: attempt.AssistanceIndependent, Status: attempt.StatusCorrect,
			ElapsedMS: 5100, Skills: []string{"not-except-polarity"},
		},
	}
}

func TestRunIDAndTokenShape(t *testing.T) {
	now := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	first, err := NewRunID(now)
	if err != nil {
		t.Fatalf("NewRunID: %v", err)
	}
	if !RunIDPattern.MatchString(first) || !strings.HasPrefix(first, "run-20261110T100000Z-") {
		t.Fatalf("run id %q", first)
	}
	second, err := NewRunID(now)
	if err != nil || first == second {
		t.Fatalf("run ids not unique: %q %q err=%v", first, second, err)
	}
	if _, err := NormalizeRunID("run-not-a-run"); err == nil {
		t.Fatal("invalid run id accepted")
	}
	token, err := NewHostToken()
	if err != nil || ValidateHostToken(token) != nil {
		t.Fatalf("token %q err=%v", token, err)
	}
	if ValidateHostToken(strings.Repeat("A", 32)) == nil {
		t.Fatal("uppercase token accepted")
	}
}

func TestValidateEndpoint(t *testing.T) {
	tests := []struct {
		endpoint string
		ok       bool
	}{
		{"https://api.example.com/v1/chat/completions", true},
		{"http://127.0.0.1:11434/v1/chat/completions", true},
		{"http://localhost:8080/chat/completions", true},
		{"http://api.example.com/v1/chat/completions", false},
		{"https://user:pass@api.example.com/v1/chat/completions", false},
		{"https://api.example.com/v1/chat/completions?x=1", false},
		{"https://api.example.com/v1/chat/completions#frag", false},
		{"https://api.example.com/v1/responses", false},
		{"ftp://api.example.com/chat/completions", false},
	}
	for _, test := range tests {
		err := ValidateEndpoint(test.endpoint)
		if test.ok && err != nil {
			t.Errorf("%s rejected: %v", test.endpoint, err)
		}
		if !test.ok && err == nil {
			t.Errorf("%s accepted", test.endpoint)
		}
	}
}

func TestValidateStepBounds(t *testing.T) {
	key := "tool:123:call"
	if _, err := ValidateStep("run-20261110T100000Z-aaaaaaaa", key, "diagnosis", "title", "body", 1, 3); err != nil {
		t.Fatalf("valid step: %v", err)
	}
	if _, err := ValidateStep("run-20261110T100000Z-aaaaaaaa", key, "diagnosis", "title", "body", 4, 3); err == nil {
		t.Fatal("index above bound accepted")
	}
	if _, err := ValidateStep("run-20261110T100000Z-aaaaaaaa", key, "other", "title", "body", 1, 3); err == nil {
		t.Fatal("unknown kind accepted")
	}
	if _, err := ValidateStep("run-20261110T100000Z-aaaaaaaa", key, "diagnosis", strings.Repeat("x", MaxTitleChars+1), "body", 1, 3); err == nil {
		t.Fatal("long title accepted")
	}
	if _, err := ValidateStep("run-20261110T100000Z-aaaaaaaa", key, "diagnosis", "title", "bad\x00body", 1, 3); err == nil {
		t.Fatal("control character accepted")
	}
	// Code points, not bytes: 600 astral characters are exactly the body bound.
	body := strings.Repeat("🚀", MaxBodyChars)
	if _, err := ValidateStep("run-20261110T100000Z-aaaaaaaa", key, "tactic", "title", body, 2, 3); err != nil {
		t.Fatalf("astral body rejected: %v", err)
	}
	if _, err := ValidateStep("run-20261110T100000Z-aaaaaaaa", key, "tactic", "title", body+"🚀", 2, 3); err == nil {
		t.Fatal("one astral character over the body bound accepted")
	}
}

func TestStudyBriefPacketIsDeterministicAndExcludesPrivateText(t *testing.T) {
	runID := "run-20261110T100000Z-aaaaaaaa"
	packet, err := BuildStudyBrief(runID, sampleRecords(), sampleAdaptiveResult(), skill.All(), 3, 8)
	if err != nil {
		t.Fatalf("BuildStudyBrief: %v", err)
	}
	prompt, err := packet.Prompt()
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	for _, forbidden := range []string{"src-private-p001", "src-private-p002", "SECRET-QUESTION", "SECRET-QUESTION-2"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("packet leaked %q", forbidden)
		}
	}
	if packet.Privacy.SourceTextIncluded || packet.Privacy.AnswerKeyIncluded || packet.Privacy.LearnerTextIncluded {
		t.Fatalf("privacy flags = %+v", packet.Privacy)
	}
	if len(packet.RecentAttempts) != 2 || packet.RecentAttempts[0].AttemptID != 1 {
		t.Fatalf("recent attempts = %+v", packet.RecentAttempts)
	}
	if packet.Recommendation == nil || packet.Recommendation.SkillID != "not-except-polarity" {
		t.Fatalf("recommendation = %+v", packet.Recommendation)
	}
	if !strings.Contains(prompt, "NOT/EXCEPT") || !strings.Contains(prompt, "record_study_step") {
		t.Fatalf("prompt missing lesson or tool instruction:\n%s", prompt)
	}
	again, err := BuildStudyBrief(runID, sampleRecords(), sampleAdaptiveResult(), skill.All(), 3, 8)
	if err != nil {
		t.Fatalf("BuildStudyBrief again: %v", err)
	}
	againPrompt, _ := again.Prompt()
	if prompt != againPrompt {
		t.Fatal("packet prompt is not deterministic")
	}
	encoded, err := json.Marshal(packet)
	if err != nil {
		t.Fatalf("marshal packet: %v", err)
	}
	if strings.Contains(string(encoded), "SECRET") {
		t.Fatalf("packet JSON leaked a sentinel: %s", encoded)
	}
}

func TestStudyBriefPacketFallsBackWhenTheRecommendationIsAbsent(t *testing.T) {
	result := sampleAdaptiveResult()
	result.Recommendation = nil
	packet, err := BuildStudyBrief("run-20261110T100000Z-aaaaaaaa", nil, result, skill.All(), 2, 6)
	if err != nil {
		t.Fatalf("BuildStudyBrief: %v", err)
	}
	if packet.Focus.ID != "not-except-polarity" || packet.Recommendation != nil {
		t.Fatalf("focus = %q recommendation = %+v", packet.Focus.ID, packet.Recommendation)
	}
	if packet.Learner.Attempts != 0 {
		t.Fatalf("learner = %+v", packet.Learner)
	}
}

func TestStudyBriefPacketWithOneStepDoesNotAskForTwo(t *testing.T) {
	packet, err := BuildStudyBrief("run-20261110T100000Z-aaaaaaaa", sampleRecords(), sampleAdaptiveResult(), skill.All(), 1, 4)
	if err != nil {
		t.Fatalf("BuildStudyBrief: %v", err)
	}
	prompt, err := packet.Prompt()
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if strings.Contains(prompt, "at least two") {
		t.Fatalf("one-step prompt asks for a second step:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Use at most 1 step.") {
		t.Fatalf("one-step prompt is missing its bound:\n%s", prompt)
	}
}

func TestClassifyFailure(t *testing.T) {
	tests := []struct {
		detail string
		code   string
		action string
	}{
		{"401: invalid api key", CodeAuthFailed, "fix_credential"},
		{"403: forbidden", CodeAuthFailed, "fix_credential"},
		{"429: rate limit", CodeRateLimited, "resume"},
		{"500: upstream failed", CodeProviderError, "resume"},
		{"404: model not found", CodeConfigError, "check_endpoint"},
		{"context deadline exceeded", CodeTimeout, "resume"},
		{"network connection reset", CodeNetworkError, "resume"},
		{"Error reading response: malformed server-sent event JSON.", CodeInvalidResponse, "inspect"},
		{"unexpected end of JSON input", CodeInvalidResponse, "inspect"},
		{"restart limit reached", CodeRestartLimit, "inspect"},
		{"task faulted", CodeTaskFaulted, "inspect"},
	}
	for _, test := range tests {
		failure := ClassifyFailure(test.detail)
		if failure.Code != test.code || failure.Action != test.action {
			t.Errorf("%q => %+v, want %s/%s", test.detail, failure, test.code, test.action)
		}
	}
	if !FailureForCode(CodeMissingCredential).NeedsAction() {
		t.Fatal("missing credential must need action")
	}
	if FailureForCode(CodeNoSteps).NeedsAction() {
		t.Fatal("no_steps must not need action")
	}
}

func TestClassifyRunFailure(t *testing.T) {
	tests := []struct {
		reason string
		detail string
		code   string
	}{
		{"faulted", "task exploded", CodeTaskFaulted},
		{"orphaned", "missing_task", CodeOrphaned},
		{"restart_limit", "3 times", CodeRestartLimit},
		{"turn_limit", "8 turns", CodeTurnLimit},
		{"no_model", "No model is configured", CodeConfigError},
		{"interrupted", "the host stopped", CodeInterrupted},
		{"nothing_to_answer", "", CodeInvalidResponse},
		{"model_error", "429: rate limit", CodeRateLimited},
		{"model_error", "401: invalid api key", CodeAuthFailed},
	}
	for _, test := range tests {
		failure := ClassifyRunFailure(test.reason, test.detail)
		if failure.Code != test.code {
			t.Errorf("%s/%q => %+v, want %s", test.reason, test.detail, failure, test.code)
		}
	}
	if !ClassifyRunFailure("interrupted", "").NeedsAction() {
		t.Fatal("interrupted run must need action (resume)")
	}
	if ClassifyRunFailure("faulted", "").NeedsAction() {
		t.Fatal("faulted run must be failed, not needs_action")
	}
}

func TestClassifyStatus(t *testing.T) {
	answer := &Snapshot{Found: true, AnswerChars: 40}
	tests := []struct {
		name  string
		input StatusInput
		state State
		code  string
	}{
		{
			name:  "cancelled event wins",
			input: StatusInput{LatestEvent: "cancelled", HeartbeatFresh: true},
			state: StateCancelled,
		},
		{
			name:  "cancel requested while fresh",
			input: StatusInput{LatestEvent: "cancel_requested", HeartbeatFresh: true},
			state: StateCancelling,
		},
		{
			name:  "cancel requested with stale heartbeat",
			input: StatusInput{LatestEvent: "cancel_requested"},
			state: StateCancelled,
		},
		{
			name:  "fresh heartbeat is running",
			input: StatusInput{LatestEvent: "started", HeartbeatFresh: true},
			state: StateRunning,
		},
		{
			name: "provider auth failure needs action",
			input: StatusInput{LatestEvent: "failed", DurableAvailable: true,
				Snapshot: &Snapshot{Found: true, Failure: &RunFailure{Reason: "model_error", Detail: "401: invalid api key"}}},
			state: StateNeedsAction, code: CodeAuthFailed,
		},
		{
			name: "interrupted run needs action",
			input: StatusInput{LatestEvent: "failed", DurableAvailable: true,
				Snapshot: &Snapshot{Found: true, Failure: &RunFailure{Reason: "interrupted", Detail: "host stopped"}}},
			state: StateNeedsAction, code: CodeInterrupted,
		},
		{
			name: "live durable work is paused",
			input: StatusInput{LatestEvent: "interrupted", DurableAvailable: true,
				Snapshot: &Snapshot{Found: true, LiveRun: true, PendingTasks: 1}},
			state: StatePaused,
		},
		{
			name: "answer with steps is completed",
			input: StatusInput{LatestEvent: "completed", DurableAvailable: true,
				Snapshot: answer, Steps: 2},
			state: StateCompleted,
		},
		{
			name: "answer without steps fails",
			input: StatusInput{LatestEvent: "completed", DurableAvailable: true,
				Snapshot: answer, Steps: 0},
			state: StateFailed, code: CodeNoSteps,
		},
		{
			name:  "host unavailable keeps the app hazard",
			input: StatusInput{LatestEvent: "needs_action", LatestCode: CodeAgentUnavailable},
			state: StateNeedsAction, code: CodeAgentUnavailable,
		},
		{
			name:  "interrupted without host is paused",
			input: StatusInput{LatestEvent: "interrupted"},
			state: StatePaused,
		},
		{
			name:  "started without host is paused",
			input: StatusInput{LatestEvent: "started"},
			state: StatePaused,
		},
		{
			name: "aborted durable run is cancelled",
			input: StatusInput{LatestEvent: "failed", DurableAvailable: true,
				Snapshot: &Snapshot{Found: true, Failure: &RunFailure{Reason: "aborted"}}},
			state: StateCancelled,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, failure := ClassifyStatus(test.input)
			if state != test.state {
				t.Fatalf("state = %s, want %s (failure %+v)", state, test.state, failure)
			}
			if test.code != "" && (failure == nil || failure.Code != test.code) {
				t.Fatalf("failure = %+v, want code %s", failure, test.code)
			}
		})
	}
}

func TestHostRequestValidation(t *testing.T) {
	valid := HostRequest{
		ProtocolVersion: ProtocolVersion,
		Mode:            ModeStart,
		RunID:           "run-20261110T100000Z-aaaaaaaa",
		TaskKind:        TaskStudyBrief,
		PromptVersion:   PromptVersion,
		StorePath:       "/tmp/agent.sqlite",
		HeartbeatPath:   "/tmp/agent.host",
		HostToken:       strings.Repeat("a", 32),
		BinaryPath:      "/usr/local/bin/passquest",
		DBPath:          "/tmp/app.sqlite",
		CredentialEnv:   CredentialEnv,
		Model: &ModelConfig{
			Endpoint: "https://api.example.com/v1/chat/completions", Model: "m",
			TimeoutMS: 30000, MaxOutputTokens: 350, MaxSteps: 3, MaxTurns: 8,
		},
		Prompt:     "prompt",
		DeadlineMS: 40000,
	}
	if err := ValidateHostRequest(valid); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	broken := valid
	broken.StorePath = "relative.sqlite"
	if err := ValidateHostRequest(broken); err == nil {
		t.Fatal("relative store path accepted")
	}
	broken = valid
	broken.HostToken = "short"
	if err := ValidateHostRequest(broken); err == nil {
		t.Fatal("short token accepted")
	}
	broken = valid
	broken.Model = &ModelConfig{Endpoint: "http://api.example.com/v1/chat/completions", Model: "m",
		TimeoutMS: 30000, MaxOutputTokens: 350, MaxSteps: 3, MaxTurns: 8}
	if err := ValidateHostRequest(broken); err == nil {
		t.Fatal("non-loopback http endpoint accepted")
	}
	broken = valid
	broken.Prompt = strings.Repeat("x", MaxPromptChars+1)
	if err := ValidateHostRequest(broken); err == nil {
		t.Fatal("oversized prompt accepted")
	}
	status := HostRequest{ProtocolVersion: ProtocolVersion, Mode: ModeStatus,
		RunID: valid.RunID, StorePath: valid.StorePath, CredentialEnv: CredentialEnv}
	if err := ValidateHostRequest(status); err != nil {
		t.Fatalf("status request rejected: %v", err)
	}
}

func TestHostResponseValidation(t *testing.T) {
	request := HostRequest{ProtocolVersion: ProtocolVersion, Mode: ModeStatus,
		RunID: "run-20261110T100000Z-aaaaaaaa"}
	response := HostResponse{SchemaVersion: ProtocolVersion, State: "status", RunID: request.RunID}
	if err := ValidateHostResponse(response, request); err != nil {
		t.Fatalf("valid response rejected: %v", err)
	}
	response.RunID = "run-20261110T100000Z-bbbbbbbb"
	if err := ValidateHostResponse(response, request); err == nil {
		t.Fatal("mismatched run id accepted")
	}
	response.RunID = request.RunID
	response.State = "unknown"
	if err := ValidateHostResponse(response, request); err == nil {
		t.Fatal("unknown state accepted")
	}
}

func TestHeartbeatFreshnessAndStorePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.host")
	now := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	raw, _ := json.Marshal(Heartbeat{RunID: "run-20261110T100000Z-aaaaaaaa", PID: 42, Token: strings.Repeat("a", 32), At: now.UnixMilli()})
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}
	heartbeat, fresh := HeartbeatFresh(path, "run-20261110T100000Z-aaaaaaaa", strings.Repeat("a", 32), now.Add(2*time.Second), 5*time.Second)
	if !fresh || heartbeat.PID != 42 {
		t.Fatalf("heartbeat = %+v fresh=%v", heartbeat, fresh)
	}
	if _, fresh := HeartbeatFresh(path, "run-20261110T100000Z-bbbbbbbb", strings.Repeat("a", 32), now, 5*time.Second); fresh {
		t.Fatal("heartbeat matched a different run")
	}
	if _, fresh := HeartbeatFresh(path, "run-20261110T100000Z-aaaaaaaa", strings.Repeat("a", 32), now.Add(10*time.Second), 5*time.Second); fresh {
		t.Fatal("stale heartbeat reported fresh")
	}
	if got := DefaultStorePath("/tmp/demo/app.sqlite"); got != "/tmp/demo/app-agent.sqlite" {
		t.Fatalf("DefaultStorePath = %q", got)
	}
	if _, err := StorePath("/tmp/app.sqlite", "/tmp/app.sqlite", nil); err == nil {
		t.Fatal("store equal to the application database accepted")
	}
}
