// Package agent owns the durable-agent contract between the Go application
// core and the Pi Durable Node host: the bounded study-brief packet, the
// versioned host protocol, run and step validation, failure classification and
// the status state machine. It never opens a database and never imports a
// network package; the host is the only provider egress.
package agent

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wxxb789/passquest/internal/tutor"
)

// ProtocolVersion is the durable-host protocol this build speaks.
const ProtocolVersion = 1

// TaskStudyBrief is the only bounded task kind in this increment.
const TaskStudyBrief = "study-brief"

// PromptVersion identifies the system section and task wording.
const PromptVersion = "agent-study-brief-v1"

const (
	// HostEnv overrides discovery of agent/durable-host.mjs.
	HostEnv = "PASSQUEST_AGENT_HOST"
	// StoreEnv selects the Pi runtime SQLite file.
	StoreEnv = "PASSQUEST_AGENT_STORE"
	// NodeEnv overrides the Node binary; it is shared with the tutor host.
	NodeEnv = "PASSQUEST_NODE"
	// CredentialEnv is the only provider credential source, shared with the
	// tutor host so one BYOK setup serves both agent paths.
	CredentialEnv = tutor.CredentialEnv
)

const (
	MinSteps     = 1
	MaxSteps     = 6
	DefaultSteps = 3

	MinTimeoutSeconds     = 1
	MaxTimeoutSeconds     = 900
	DefaultTimeoutSeconds = 120

	MinCancelWaitSeconds     = 1
	MaxCancelWaitSeconds     = 60
	DefaultCancelWaitSeconds = 10

	MaxPromptChars   = 20000
	MaxTitleChars    = 100
	MaxBodyChars     = 600
	MaxAttemptKey    = 200
	HeartbeatStaleMS = 5000
)

// State is the observable lifecycle state of one run.
type State string

const (
	StateRunning     State = "running"
	StatePaused      State = "paused"
	StateCompleted   State = "completed"
	StateFailed      State = "failed"
	StateNeedsAction State = "needs_action"
	StateCancelling  State = "cancelling"
	StateCancelled   State = "cancelled"
)

// Failure code vocabulary shared by the host result and the application
// classifier. The codes are stable protocol values, not provider messages.
const (
	CodeAgentUnavailable  = "agent_unavailable"
	CodeMissingCredential = "missing_credential"
	CodeAuthFailed        = "auth_failed"
	CodeRateLimited       = "rate_limited"
	CodeProviderError     = "provider_error"
	CodeTimeout           = "timeout"
	CodeNetworkError      = "network_error"
	CodeInvalidResponse   = "invalid_response"
	CodeConfigError       = "config_error"
	CodeHostError         = "host_error"
	CodeStorageError      = "storage_error"
	CodeNoSteps           = "no_steps"
	CodeTurnLimit         = "turn_limit"
	CodeHostProtocol      = "host_protocol"
	CodeTaskFaulted       = "task_faulted"
	CodeOrphaned          = "orphaned"
	CodeRestartLimit      = "restart_limit"
	CodeAborted           = "aborted"
	CodeInterrupted       = "interrupted"
)

// Failure explains a non-completed state. Action is a user-facing next step,
// never an automatic retry.
type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Action  string `json:"action"`
}

// NeedsAction reports whether the failure is actionable by the learner or
// operator (credential, endpoint, host install, transient provider wait).
func (f Failure) NeedsAction() bool {
	switch f.Code {
	case CodeAgentUnavailable, CodeMissingCredential, CodeAuthFailed, CodeRateLimited,
		CodeProviderError, CodeTimeout, CodeNetworkError, CodeConfigError, CodeInterrupted:
		return true
	default:
		return false
	}
}

// ClassifyFailure maps a provider/task failure to the stable code and action.
// detail is the Pi run failure detail or a host error message; only the prefix
// and well-known diagnostics are used, never the whole text as a code.
func ClassifyFailure(detail string) Failure {
	text := strings.TrimSpace(detail)
	lower := strings.ToLower(text)
	code := ""
	switch {
	case strings.Contains(lower, "missing credential"), strings.Contains(lower, "no api key"):
		code = CodeMissingCredential
	case strings.HasPrefix(text, "401:"), strings.HasPrefix(text, "403:"),
		strings.Contains(lower, "invalid api key"), strings.Contains(lower, "unauthorized"),
		strings.Contains(lower, "authentication"):
		code = CodeAuthFailed
	case strings.HasPrefix(text, "429:"), strings.Contains(lower, "rate limit"):
		code = CodeRateLimited
	case strings.HasPrefix(text, "400:"), strings.HasPrefix(text, "404:"),
		strings.Contains(lower, "model not found"), strings.Contains(lower, "no model"):
		code = CodeConfigError
	case strings.HasPrefix(text, "500:"), strings.HasPrefix(text, "502:"), strings.HasPrefix(text, "503:"),
		strings.HasPrefix(text, "504:"), strings.Contains(lower, "provider is not configured"):
		code = CodeProviderError
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "timed out"), strings.Contains(lower, "deadline exceeded"):
		code = CodeTimeout
	case strings.Contains(lower, "network"), strings.Contains(lower, "connection"), strings.Contains(lower, "dns"):
		code = CodeNetworkError
	case strings.Contains(lower, "invalid response"), strings.Contains(lower, "malformed"),
		strings.Contains(lower, "unexpected end"), strings.Contains(lower, "json"):
		code = CodeInvalidResponse
	case strings.Contains(lower, "restart limit"):
		code = CodeRestartLimit
	case strings.Contains(lower, "fault"):
		code = CodeTaskFaulted
	case strings.Contains(lower, "orphan"):
		code = CodeOrphaned
	default:
		code = CodeProviderError
	}
	return failureFor(code, text)
}

// ClassifyRunFailure maps a Pi run failure to the stable code. The run reason
// is authoritative for task-level outcomes; a model error still uses the
// provider detail because it carries the HTTP status.
func ClassifyRunFailure(reason, detail string) Failure {
	switch reason {
	case "aborted":
		return failureFor(CodeAborted, detail)
	case "faulted":
		return failureFor(CodeTaskFaulted, detail)
	case "orphaned":
		return failureFor(CodeOrphaned, detail)
	case "restart_limit":
		return failureFor(CodeRestartLimit, detail)
	case "turn_limit":
		return failureFor(CodeTurnLimit, detail)
	case "no_model":
		return failureFor(CodeConfigError, detail)
	case "interrupted":
		return failureFor(CodeInterrupted, detail)
	case "nothing_to_answer":
		return failureFor(CodeInvalidResponse, detail)
	case "model_error":
		return ClassifyFailure(detail)
	default:
		return ClassifyFailure(detail)
	}
}

func failureFor(code, message string) Failure {
	action := "inspect"
	switch code {
	case CodeAgentUnavailable:
		action = "check_host"
	case CodeMissingCredential, CodeAuthFailed:
		action = "fix_credential"
	case CodeRateLimited, CodeProviderError, CodeTimeout, CodeNetworkError, CodeInterrupted:
		action = "resume"
	case CodeConfigError:
		action = "check_endpoint"
	case CodeTurnLimit:
		action = "resume"
	}
	if strings.TrimSpace(message) == "" {
		message = defaultFailureMessage(code)
	}
	return Failure{Code: code, Message: message, Action: action}
}

func defaultFailureMessage(code string) string {
	switch code {
	case CodeAgentUnavailable:
		return "the Node agent host is unavailable; install Node and the agent dependencies"
	case CodeMissingCredential:
		return "the provider credential is missing from the environment"
	case CodeAuthFailed:
		return "the provider rejected the credential"
	case CodeRateLimited:
		return "the provider rate-limited the request"
	case CodeProviderError:
		return "the provider returned an error"
	case CodeTimeout:
		return "the provider request timed out"
	case CodeNetworkError:
		return "the provider could not be reached"
	case CodeInvalidResponse:
		return "the provider response could not be read"
	case CodeConfigError:
		return "the provider configuration is not usable for the agent"
	case CodeHostError:
		return "the agent host failed before returning a result"
	case CodeStorageError:
		return "the durable agent storage could not be read or written"
	case CodeNoSteps:
		return "the run answered without committing a study step"
	case CodeTurnLimit:
		return "the run reached its bounded model-turn limit"
	case CodeHostProtocol:
		return "the agent host returned a document this build does not understand"
	case CodeTaskFaulted:
		return "the durable task failed"
	case CodeOrphaned:
		return "the durable task was orphaned"
	case CodeRestartLimit:
		return "the durable task was cut off too many times"
	case CodeAborted:
		return "the run was aborted"
	case CodeInterrupted:
		return "the run was interrupted before an answer; resume it"
	default:
		return code
	}
}

// FailureForCode exposes the stable failure document for an application-side
// decision such as a missing credential or a zero-step answer.
func FailureForCode(code string) Failure { return failureFor(code, "") }

// RunFailure is the Pi run failure shape the host reports.
type RunFailure struct {
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

// Snapshot is the bounded durable view the host returns for status.
type Snapshot struct {
	Found                bool        `json:"found"`
	ConversationID       int64       `json:"conversation_id,omitempty"`
	LiveRun              bool        `json:"live_run"`
	PendingTasks         int         `json:"pending_tasks"`
	UnsettledSubmissions int         `json:"unsettled_submissions"`
	Failure              *RunFailure `json:"failure,omitempty"`
	AnswerChars          int         `json:"answer_chars"`
	Entries              int         `json:"entries"`
	Turns                int         `json:"turns,omitempty"`
	ToolCalls            int         `json:"tool_calls,omitempty"`
	ToolErrors           int         `json:"tool_errors,omitempty"`
}

// StatusInput is the complete evidence for one status decision.
type StatusInput struct {
	LatestEvent      string
	LatestCode       string
	HeartbeatFresh   bool
	DurableAvailable bool
	Snapshot         *Snapshot
	Steps            int
}

// ClassifyStatus decides the public state. It is deliberately pure: the CLI
// supplies the application event trail, heartbeat and read-only Pi snapshot.
func ClassifyStatus(input StatusInput) (State, *Failure) {
	switch input.LatestEvent {
	case "cancelled":
		return StateCancelled, nil
	case "cancel_requested":
		if input.HeartbeatFresh {
			return StateCancelling, nil
		}
		return StateCancelled, nil
	}
	if input.HeartbeatFresh {
		return StateRunning, nil
	}
	if input.DurableAvailable && input.Snapshot != nil && input.Snapshot.Found {
		snapshot := input.Snapshot
		if snapshot.Failure != nil {
			if snapshot.Failure.Reason == "aborted" {
				return StateCancelled, nil
			}
			failure := ClassifyRunFailure(snapshot.Failure.Reason, snapshot.Failure.Detail)
			return stateForFailure(failure), &failure
		}
		if snapshot.LiveRun || snapshot.PendingTasks > 0 || snapshot.UnsettledSubmissions > 0 {
			return StatePaused, nil
		}
		if snapshot.AnswerChars > 0 {
			if input.Steps == 0 {
				failure := FailureForCode(CodeNoSteps)
				return StateFailed, &failure
			}
			return StateCompleted, nil
		}
		return StatePaused, nil
	}
	// The durable host is unavailable or has no record. The application trail
	// is still authoritative for what happened before the host was reached.
	switch input.LatestEvent {
	case "completed":
		if input.Steps == 0 {
			failure := FailureForCode(CodeNoSteps)
			return StateFailed, &failure
		}
		return StateCompleted, nil
	case "failed":
		failure := failureFor(input.LatestCode, "")
		return StateFailed, &failure
	case "needs_action":
		failure := failureFor(input.LatestCode, "")
		return StateNeedsAction, &failure
	case "interrupted":
		return StatePaused, nil
	case "started", "resumed", "":
		return StatePaused, nil
	default:
		return StatePaused, nil
	}
}

func stateForFailure(failure Failure) State {
	if failure.NeedsAction() {
		return StateNeedsAction
	}
	return StateFailed
}

// RunIDPattern is the filename-safe identity used in the Pi store, heartbeat
// and step keys.
var RunIDPattern = regexp.MustCompile(`^run-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{8}$`)

// NewRunID returns a UTC timestamp plus 32 random bits, safe for a filename.
func NewRunID(now time.Time) (string, error) {
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("agent: generate run id: %w", err)
	}
	return "run-" + now.UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(random[:]), nil
}

// NormalizeRunID validates a run id supplied on the command line.
func NormalizeRunID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !RunIDPattern.MatchString(raw) {
		return "", fmt.Errorf("run id %q is not a passquest run id", raw)
	}
	return raw, nil
}

// NewHostToken returns the random value a live host writes into its heartbeat.
func NewHostToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("agent: generate host token: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

// ValidateHostToken checks the heartbeat token shape.
func ValidateHostToken(token string) error {
	if len(token) != 32 {
		return errors.New("agent: host token must be 32 lowercase hex characters")
	}
	for _, r := range token {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return errors.New("agent: host token must be 32 lowercase hex characters")
		}
	}
	return nil
}

// Step is a validated step request from the host tool.
type Step struct {
	RunID      string
	AttemptKey string
	Index      int
	Kind       string
	Title      string
	Body       string
}

// ValidateStep enforces the exact bounds the prompt and database use. The
// title and body counts are Unicode code points, matching SQLite length() for
// TEXT and the host's Array.from().length.
func ValidateStep(runID, attemptKey, kind, title, body string, index, maxSteps int) (Step, error) {
	runID, err := NormalizeRunID(runID)
	if err != nil {
		return Step{}, err
	}
	if attemptKey == "" || len(attemptKey) > MaxAttemptKey {
		return Step{}, fmt.Errorf("attempt key must be 1-%d bytes", MaxAttemptKey)
	}
	if index < 1 || index > maxSteps || maxSteps > MaxSteps {
		return Step{}, fmt.Errorf("step index %d is outside 1..%d", index, maxSteps)
	}
	switch kind {
	case "diagnosis", "tactic", "practice":
	default:
		return Step{}, errors.New("kind must be diagnosis, tactic or practice")
	}
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if !utf8.ValidString(title) || !utf8.ValidString(body) {
		return Step{}, errors.New("title and body must be valid UTF-8")
	}
	if !validStepText(title) || !validStepText(body) {
		return Step{}, errors.New("title and body must not contain control characters")
	}
	if utf8.RuneCountInString(title) == 0 || utf8.RuneCountInString(title) > MaxTitleChars {
		return Step{}, fmt.Errorf("title must be 1-%d characters", MaxTitleChars)
	}
	if utf8.RuneCountInString(body) == 0 || utf8.RuneCountInString(body) > MaxBodyChars {
		return Step{}, fmt.Errorf("body must be 1-%d characters", MaxBodyChars)
	}
	return Step{RunID: runID, AttemptKey: attemptKey, Index: index, Kind: kind, Title: title, Body: body}, nil
}

func validStepText(text string) bool {
	for _, r := range text {
		if r == '\n' || r == '\t' || r == '\r' {
			continue
		}
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// ErrorDoc is one stable error.
type ErrorDoc struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
