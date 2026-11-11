package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Host request modes.
const (
	ModeStart      = "start"
	ModeResume     = "resume"
	ModeStatus     = "status"
	ModeTranscript = "transcript"
)

// HostRequest is the one versioned JSON document sent to the Node host.
type HostRequest struct {
	ProtocolVersion int          `json:"protocol_version"`
	Mode            string       `json:"mode"`
	RunID           string       `json:"run_id"`
	TaskKind        string       `json:"task_kind,omitempty"`
	PromptVersion   string       `json:"prompt_version,omitempty"`
	StorePath       string       `json:"store_path"`
	HeartbeatPath   string       `json:"heartbeat_path,omitempty"`
	HostToken       string       `json:"host_token,omitempty"`
	BinaryPath      string       `json:"binary_path,omitempty"`
	DBPath          string       `json:"db_path,omitempty"`
	CredentialEnv   string       `json:"credential_env"`
	Model           *ModelConfig `json:"model,omitempty"`
	Prompt          string       `json:"prompt,omitempty"`
	DeadlineMS      int64        `json:"deadline_ms,omitempty"`
	TranscriptLimit int          `json:"transcript_limit,omitempty"`
}

// ModelConfig is the agent-visible part of the tutor configuration. The
// credential is never part of the request.
type ModelConfig struct {
	Endpoint        string `json:"endpoint"`
	Model           string `json:"model"`
	TimeoutMS       int64  `json:"timeout_ms"`
	MaxOutputTokens int    `json:"max_output_tokens"`
	MaxSteps        int    `json:"max_steps"`
	MaxTurns        int    `json:"max_turns"`
}

// HostFailure is the Pi run failure as reported by the host.
type HostFailure struct {
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

// TranscriptEntry is one bounded durable-history entry.
type TranscriptEntry struct {
	Kind     string          `json:"kind"`
	At       string          `json:"at,omitempty"`
	Role     string          `json:"role,omitempty"`
	Text     string          `json:"text,omitempty"`
	ToolName string          `json:"tool_name,omitempty"`
	IsError  bool            `json:"is_error,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// HostResponse is the one JSON result from the Node host.
type HostResponse struct {
	SchemaVersion  int               `json:"schema_version"`
	State          string            `json:"state"`
	RunID          string            `json:"run_id"`
	ConversationID int64             `json:"conversation_id,omitempty"`
	Failure        *HostFailure      `json:"failure,omitempty"`
	Error          *ErrorDoc         `json:"error,omitempty"`
	Answer         string            `json:"answer,omitempty"`
	AnswerChars    int               `json:"answer_chars,omitempty"`
	Turns          int               `json:"turns,omitempty"`
	ToolCalls      int               `json:"tool_calls,omitempty"`
	ToolErrors     int               `json:"tool_errors,omitempty"`
	Usage          json.RawMessage   `json:"usage,omitempty"`
	Snapshot       *Snapshot         `json:"snapshot,omitempty"`
	Found          bool              `json:"found,omitempty"`
	Entries        []TranscriptEntry `json:"entries,omitempty"`
	Truncated      bool              `json:"truncated,omitempty"`
	Limits         []string          `json:"limits,omitempty"`
}

// RunOutcome is the local result of one host process.
type RunOutcome struct {
	Response        HostResponse
	Interrupted     bool
	InterruptReason string
	ExitCode        int
}

// UnavailableError means Node, the host file, or the host process could not
// be started. The CLI records this as a needs_action run failure.
type UnavailableError struct{ Err error }

func (e *UnavailableError) Error() string { return "agent host unavailable: " + e.Err.Error() }
func (e *UnavailableError) Unwrap() error { return e.Err }

// ProtocolError means a host process ran but its document was unusable.
type ProtocolError struct{ Message string }

func (e *ProtocolError) Error() string { return "agent host protocol: " + e.Message }

// Runner executes one host request. Tests inject a fake.
type Runner interface {
	Run(ctx context.Context, request HostRequest) (RunOutcome, error)
}

// HostRunner starts the real Node host. The provider credential is inherited
// through the environment and never placed in the request.
type HostRunner struct {
	Getenv     func(string) string
	Environ    func() []string
	Getwd      func() (string, error)
	Executable func() (string, error)
}

func (r HostRunner) getenv(key string) string {
	if r.Getenv != nil {
		return r.Getenv(key)
	}
	return os.Getenv(key)
}

func (r HostRunner) environ() []string {
	if r.Environ != nil {
		return r.Environ()
	}
	return os.Environ()
}

func (r HostRunner) getwd() (string, error) {
	if r.Getwd != nil {
		return r.Getwd()
	}
	return os.Getwd()
}

func (r HostRunner) executable() (string, error) {
	if r.Executable != nil {
		return r.Executable()
	}
	return os.Executable()
}

// ResolveHost finds agent/durable-host.mjs. An explicit PASSQUEST_AGENT_HOST is
// authoritative; otherwise the working directory, the executable's directory
// and its parent are checked.
func ResolveHost(getenv func(string) string, getwd func() (string, error), executablePath func() (string, error)) (string, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	if value := strings.TrimSpace(getenv(HostEnv)); value != "" {
		path, err := filepath.Abs(value)
		if err != nil {
			return "", fmt.Errorf("agent: resolve %s: %w", HostEnv, err)
		}
		if err := requireHostFile(path); err != nil {
			return "", err
		}
		return path, nil
	}
	var candidates []string
	if getwd == nil {
		getwd = os.Getwd
	}
	if cwd, err := getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "agent", "durable-host.mjs"))
	}
	if executablePath == nil {
		executablePath = os.Executable
	}
	if exe, err := executablePath(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "agent", "durable-host.mjs"),
			filepath.Join(dir, "..", "agent", "durable-host.mjs"),
		)
	}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if requireHostFile(absolute) == nil {
			return absolute, nil
		}
	}
	return "", fmt.Errorf("agent: durable host not found; set %s to agent/durable-host.mjs (looked in %s)",
		HostEnv, strings.Join(candidates, ", "))
}

func requireHostFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("agent: durable host %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("agent: durable host %s is a directory", path)
	}
	return nil
}

// Run validates the request, starts the host and waits for one JSON document.
// A timeout or context cancellation sends a graceful stop first and reports
// the work as interrupted rather than failed.
func (r HostRunner) Run(ctx context.Context, request HostRequest) (RunOutcome, error) {
	if err := ValidateHostRequest(request); err != nil {
		return RunOutcome{}, &ProtocolError{Message: err.Error()}
	}
	host, err := ResolveHost(r.Getenv, r.Getwd, r.Executable)
	if err != nil {
		return RunOutcome{}, &UnavailableError{Err: err}
	}
	node := strings.TrimSpace(r.getenv(NodeEnv))
	if node == "" {
		node = "node"
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return RunOutcome{}, fmt.Errorf("agent: encode host request: %w", err)
	}
	cmd := exec.Command(node, host)
	cmd.Env = r.environ()
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr limitedBuffer
	stdout.limit = 2 << 20
	stderr.limit = 64 << 10
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	configureProcess(cmd)
	if err := cmd.Start(); err != nil {
		return RunOutcome{}, &UnavailableError{Err: fmt.Errorf("start agent host: %w", err)}
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var interrupted bool
	interruptReason := ""
	select {
	case err = <-done:
	case <-ctx.Done():
		interrupted = true
		interruptReason = "signal"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			interruptReason = "timeout"
		}
		// The control file is the authoritative pause request. Signals are an
		// optimisation: Windows cannot deliver them, so the host polls this
		// file and the kill below remains the bounded fallback.
		_ = WriteHostControl(request.HeartbeatPath, request.RunID, request.HostToken, HostControlPause)
		_ = terminate(cmd.Process)
		select {
		case err = <-done:
		case <-time.After(5 * time.Second):
			_ = killProcess(cmd.Process)
			err = <-done
		}
	}
	if stdout.truncated {
		return RunOutcome{}, &ProtocolError{Message: "host response exceeded 2 MiB"}
	}
	stderrText := strings.TrimSpace(stderr.String())
	out := RunOutcome{Interrupted: interrupted, InterruptReason: interruptReason}
	if cmd.ProcessState != nil {
		out.ExitCode = cmd.ProcessState.ExitCode()
	}
	if len(stdout.Bytes()) > 0 {
		var response HostResponse
		if decodeErr := json.Unmarshal(stdout.Bytes(), &response); decodeErr == nil {
			if validateErr := ValidateHostResponse(response, request); validateErr == nil {
				out.Response = response
				// A valid document is authoritative even when the process
				// exited nonzero or was interrupted.
				return out, nil
			}
		}
	}
	if interrupted {
		return out, nil
	}
	if err != nil {
		// A host process that dies before returning a document leaves durable
		// work behind. Treat that as an interruption so resume can pick it up;
		// a missing dependency is the one startup failure we can name.
		lower := strings.ToLower(stderrText)
		if strings.Contains(lower, "err_module_not_found") || strings.Contains(lower, "cannot find module") ||
			strings.Contains(lower, "cannot find package") {
			return out, &UnavailableError{Err: errors.New("agent host dependencies are not installed; run bun install")}
		}
		out.Interrupted = true
		out.InterruptReason = "host_exit"
		return out, nil
	}
	return out, &ProtocolError{Message: "host returned no usable JSON result"}
}

// ValidateHostRequest checks both the protocol and the mode-specific fields.
func ValidateHostRequest(request HostRequest) error {
	if request.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("protocol %d is not supported", request.ProtocolVersion)
	}
	if _, err := NormalizeRunID(request.RunID); err != nil {
		return err
	}
	if strings.TrimSpace(request.StorePath) == "" || strings.ContainsRune(request.StorePath, 0) {
		return errors.New("store_path is required")
	}
	if !filepath.IsAbs(request.StorePath) {
		return errors.New("store_path must be absolute")
	}
	switch request.Mode {
	case ModeStatus:
		return nil
	case ModeTranscript:
		if request.TranscriptLimit == 0 {
			request.TranscriptLimit = 100
		}
		if request.TranscriptLimit < 1 || request.TranscriptLimit > 200 {
			return errors.New("transcript_limit must be 1-200")
		}
		return nil
	case ModeStart, ModeResume:
	default:
		return fmt.Errorf("mode %q is not supported", request.Mode)
	}
	if request.CredentialEnv == "" {
		return errors.New("credential_env is required")
	}
	if request.TaskKind != TaskStudyBrief {
		return fmt.Errorf("task %q is not supported", request.TaskKind)
	}
	if request.PromptVersion != PromptVersion {
		return fmt.Errorf("prompt version %q is not supported", request.PromptVersion)
	}
	if strings.TrimSpace(request.Prompt) == "" || len([]rune(request.Prompt)) > MaxPromptChars {
		return fmt.Errorf("prompt must be 1-%d characters", MaxPromptChars)
	}
	if !filepath.IsAbs(request.BinaryPath) || strings.ContainsRune(request.BinaryPath, 0) {
		return errors.New("binary_path must be absolute")
	}
	if !filepath.IsAbs(request.DBPath) || strings.ContainsRune(request.DBPath, 0) {
		return errors.New("db_path must be absolute")
	}
	if !filepath.IsAbs(request.HeartbeatPath) || strings.ContainsRune(request.HeartbeatPath, 0) {
		return errors.New("heartbeat_path must be absolute")
	}
	if err := ValidateHostToken(request.HostToken); err != nil {
		return err
	}
	if request.DeadlineMS <= 0 {
		return errors.New("deadline_ms must be positive")
	}
	if request.Model == nil {
		return errors.New("model is required")
	}
	return ValidateModelConfig(*request.Model)
}

// ValidateModelConfig checks the provider and bounded-task settings.
func ValidateModelConfig(model ModelConfig) error {
	if err := ValidateEndpoint(model.Endpoint); err != nil {
		return err
	}
	if strings.TrimSpace(model.Model) == "" || len([]rune(model.Model)) > 200 {
		return errors.New("model name must be 1-200 characters")
	}
	if model.TimeoutMS < 1000 || model.TimeoutMS > 900000 {
		return errors.New("timeout_ms must be 1000-900000")
	}
	if model.MaxOutputTokens < 64 || model.MaxOutputTokens > 2048 {
		return errors.New("max_output_tokens must be 64-2048")
	}
	if model.MaxSteps < MinSteps || model.MaxSteps > MaxSteps {
		return fmt.Errorf("max_steps must be %d-%d", MinSteps, MaxSteps)
	}
	if model.MaxTurns < 3 || model.MaxTurns > 16 {
		return errors.New("max_turns must be 3-16")
	}
	return nil
}

// ValidateEndpoint accepts one OpenAI-compatible chat-completions URL. https is
// required except on loopback; userinfo, query and fragment are refused so a
// credential cannot be smuggled into stored configuration.
func ValidateEndpoint(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("endpoint is not a URL: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return errors.New("endpoint must use https (http is allowed only on loopback)")
	}
	host := parsed.Hostname()
	if host == "" {
		return errors.New("endpoint has no host")
	}
	if parsed.User != nil {
		return fmt.Errorf("endpoint must not contain userinfo; the credential belongs only in %s", CredentialEnv)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("endpoint must not contain a query or fragment")
	}
	loopback := host == "localhost" || host == "127.0.0.1" || host == "::1"
	if parsed.Scheme == "http" && !loopback {
		return errors.New("endpoint must use https outside loopback")
	}
	if !strings.HasSuffix(parsed.Path, "/chat/completions") {
		return errors.New("endpoint must end in /chat/completions")
	}
	return nil
}

// ValidateHostResponse checks the document identity and known state.
func ValidateHostResponse(response HostResponse, request HostRequest) error {
	if response.SchemaVersion != ProtocolVersion {
		return fmt.Errorf("response protocol %d is not supported", response.SchemaVersion)
	}
	if response.RunID != request.RunID {
		return fmt.Errorf("response run id %q does not match %q", response.RunID, request.RunID)
	}
	if request.Mode == ModeStatus && response.State != "status" {
		return fmt.Errorf("status request returned %q", response.State)
	}
	if request.Mode == ModeTranscript && response.State != "transcript" {
		return fmt.Errorf("transcript request returned %q", response.State)
	}
	switch response.State {
	case "completed", "interrupted", "cancelled", "failed", "needs_action", "status", "transcript":
		return nil
	default:
		return fmt.Errorf("unknown state %q", response.State)
	}
}

type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		if len(p) > 0 {
			b.truncated = true
		}
		return len(p), nil
	}
	if len(p) > remaining {
		b.buf.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	b.buf.Write(p)
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte  { return b.buf.Bytes() }
func (b *limitedBuffer) String() string { return b.buf.String() }

// Heartbeat is the liveness file a running host refreshes every second.
type Heartbeat struct {
	RunID string `json:"run_id"`
	PID   int    `json:"pid"`
	Token string `json:"token"`
	At    int64  `json:"at"`
}

// Host control actions. The host polls the control file beside its heartbeat,
// so cancellation and pause do not depend on POSIX signal delivery.
const (
	HostControlPause  = "pause"
	HostControlCancel = "cancel"
)

// HostControl is one durable request the host observes itself. The token keeps
// a request from an earlier host from stopping a later resume of the same run.
type HostControl struct {
	RunID     string `json:"run_id"`
	HostToken string `json:"host_token,omitempty"`
	Action    string `json:"action"`
}

// HostControlPath is the control file beside one run's heartbeat.
func HostControlPath(heartbeatPath string) string {
	return heartbeatPath + ".control"
}

// WriteHostControl publishes one control request atomically. An empty token
// applies to any host for the run; otherwise only the named host observes it.
func WriteHostControl(heartbeatPath, runID, token, action string) error {
	if heartbeatPath == "" {
		return nil
	}
	path := HostControlPath(heartbeatPath)
	payload, err := json.Marshal(HostControl{RunID: runID, HostToken: token, Action: action})
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(payload); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

// RemoveHostControlForRun clears this run's consumed or abandoned control
// request and leaves a newer run's request alone. An empty token removes the
// request regardless of the host token it names; a non-empty token also
// requires the request to name that host.
func RemoveHostControlForRun(heartbeatPath, runID, token string) {
	if heartbeatPath == "" {
		return
	}
	path := HostControlPath(heartbeatPath)
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var control HostControl
	if json.Unmarshal(raw, &control) != nil || control.RunID != runID {
		return
	}
	if token != "" && control.HostToken != "" && control.HostToken != token {
		return
	}
	_ = os.Remove(path)
}

// HeartbeatAuthority reports whether a fresh heartbeat can still describe a
// live host for the run's latest application event. Only a live claim, or a
// cancellation the host has not yet acknowledged, can. An interrupted,
// failed, completed or cancelled event is proof the host process this
// application started has exited, so a heartbeat it left behind (a hard kill
// cannot remove it) must not override that trail on any OS.
func HeartbeatAuthority(latestEvent string) bool {
	switch latestEvent {
	case "started", "resumed", "cancel_requested":
		return true
	default:
		return false
	}
}

// HeartbeatStatus classifies one liveness probe. Absent is authoritative: the
// run has no heartbeat. Unreadable is not: a read can fail transiently, notably
// on Windows while the host replaces the file with os.Rename.
type HeartbeatStatus int

const (
	HeartbeatAbsent HeartbeatStatus = iota
	HeartbeatStale
	HeartbeatFresh
	HeartbeatUnreadable
)

// Live reports whether the run must be treated as live. Unreadable is live on
// purpose: a false live costs one wait, but a false not-live confirms a cancel
// beside a running host.
func (s HeartbeatStatus) Live() bool {
	return s == HeartbeatFresh || s == HeartbeatUnreadable
}

// heartbeatReadAttempts bounds retries of a non-missing read error. The total
// backoff (150ms) is far below HeartbeatStaleMS, so probing cannot stretch a
// cancel or status call.
const (
	heartbeatReadAttempts = 5
	heartbeatReadBackoff  = 10 * time.Millisecond
)

// readHeartbeat retries a non-missing read error with a short bounded backoff.
// Absence is authoritative and returned immediately: retrying it would make
// every cancel of an idle run wait.
func readHeartbeat(path string, read func(string) ([]byte, error)) ([]byte, error) {
	delay := heartbeatReadBackoff
	var err error
	for attempt := range heartbeatReadAttempts {
		var raw []byte
		raw, err = read(path)
		if err == nil {
			return raw, nil
		}
		if errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if attempt+1 < heartbeatReadAttempts {
			time.Sleep(delay)
			delay *= 2
		}
	}
	return nil, err
}

// ProbeHeartbeat classifies the heartbeat file for one run without matching the
// token. Liveness belongs to the run: a host that raced another claim still
// holds the Pi store, so a token mismatch must not make a running task look
// idle. The parsed heartbeat is returned even when stale so a caller can name
// the last pid.
func ProbeHeartbeat(path, runID string, now time.Time, stale time.Duration) (Heartbeat, HeartbeatStatus) {
	return probeHeartbeat(os.ReadFile, path, runID, now, stale)
}

func probeHeartbeat(read func(string) ([]byte, error), path, runID string, now time.Time, stale time.Duration) (Heartbeat, HeartbeatStatus) {
	raw, err := readHeartbeat(path, read)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Heartbeat{}, HeartbeatAbsent
		}
		return Heartbeat{}, HeartbeatUnreadable
	}
	var heartbeat Heartbeat
	if err := json.Unmarshal(raw, &heartbeat); err != nil {
		return Heartbeat{}, HeartbeatAbsent
	}
	if heartbeat.RunID != runID {
		return Heartbeat{}, HeartbeatAbsent
	}
	if heartbeat.At <= 0 {
		return heartbeat, HeartbeatStale
	}
	at := time.UnixMilli(heartbeat.At)
	age := now.Sub(at)
	if age < -2*time.Second {
		return heartbeat, HeartbeatStale
	}
	if age > stale {
		return heartbeat, HeartbeatStale
	}
	return heartbeat, HeartbeatFresh
}

// HeartbeatLive reports whether the run must be treated as live: a fresh
// heartbeat, or an unreadable one whose error is not absence. Missing and stale
// heartbeats are not live.
func HeartbeatLive(path, runID string, now time.Time, stale time.Duration) (Heartbeat, bool) {
	heartbeat, status := ProbeHeartbeat(path, runID, now, stale)
	return heartbeat, status.Live()
}

// RemoveHeartbeatForRun clears a stale or finished liveness file only when it
// names runID. Ownership is the run id: the token records which host wrote the
// heartbeat and deliberately does not make a live host look idle (see
// HeartbeatLive). A newer run on the same store owns the path, so deleting its
// heartbeat would make it look paused.
func RemoveHeartbeatForRun(path, runID string) {
	if path == "" {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var heartbeat Heartbeat
	if json.Unmarshal(raw, &heartbeat) != nil || heartbeat.RunID != runID {
		return
	}
	_ = os.Remove(path)
}

// DefaultStorePath derives the Pi SQLite path next to the application database.
func DefaultStorePath(dbPath string) string {
	absolute, err := filepath.Abs(dbPath)
	if err != nil {
		absolute = dbPath
	}
	base := strings.TrimSuffix(filepath.Base(absolute), filepath.Ext(absolute))
	if base == "" {
		base = "passquest"
	}
	return filepath.Join(filepath.Dir(absolute), base+"-agent.sqlite")
}

// StorePath resolves the --agent-store flag, the environment override and the
// derived default. The application database and Pi store must differ.
func StorePath(flagValue, dbPath string, getenv func(string) string) (string, error) {
	selected := strings.TrimSpace(flagValue)
	if selected == "" && getenv != nil {
		selected = strings.TrimSpace(getenv(StoreEnv))
	}
	if selected == "" {
		selected = DefaultStorePath(dbPath)
	}
	return ResolveStorePath(selected, dbPath)
}

// ResolveStorePath makes one explicit Pi store path absolute and refuses one
// equal to the application database. Every command that accepts an override
// shares this guard: the host never opens the application database.
func ResolveStorePath(selected, dbPath string) (string, error) {
	absolute, err := filepath.Abs(selected)
	if err != nil {
		return "", fmt.Errorf("agent: resolve store path: %w", err)
	}
	dbAbsolute, err := filepath.Abs(dbPath)
	if err == nil && absolute == dbAbsolute {
		return "", errors.New("--agent-store must differ from the application database")
	}
	return absolute, nil
}

// PrepareStorePath creates the parent directory with a private mode, creates
// the SQLite file with 0600 when absent, and returns the absolute path. The Pi
// store holds the task prompt and model output, so it is private local data.
func PrepareStorePath(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("agent: resolve store path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return fmt.Errorf("agent: create store directory: %w", err)
	}
	file, err := os.OpenFile(absolute, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("agent: create store file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("agent: close store file: %w", err)
	}
	if err := os.Chmod(absolute, 0o600); err != nil {
		return fmt.Errorf("agent: protect store file: %w", err)
	}
	return nil
}

// SignalCancelPID asks a recorded host process to abort the conversation
// durably. On Windows it terminates the process instead; the application
// cancellation event remains the authority.
func SignalCancelPID(pid int) error {
	if pid <= 0 {
		return errors.New("agent: no host process recorded")
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return cancel(process)
}

// SignalKillPID is the bounded fallback when a graceful cancel does not stop
// the heartbeat.
func SignalKillPID(pid int) error {
	if pid <= 0 {
		return errors.New("agent: no host process recorded")
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return killProcess(process)
}
