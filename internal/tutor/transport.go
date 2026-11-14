package tutor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wxxb789/passquest/internal/hostproc"
)

// HostTransport runs the Node host as a child process and exchanges one JSON
// document over stdin/stdout. The credential is inherited through the
// environment; it is never placed in argv, the request document, an error
// message or the returned response.
type HostTransport struct {
	Getenv     func(string) string
	Environ    func() []string
	Getwd      func() (string, error)
	Executable func() (string, error)
}

func (t HostTransport) getenv(key string) string {
	if t.Getenv != nil {
		return t.Getenv(key)
	}
	return os.Getenv(key)
}

func (t HostTransport) environ() []string {
	if t.Environ != nil {
		return t.Environ()
	}
	return os.Environ()
}

func (t HostTransport) environFor(req HostRequest) []string {
	env := t.environ()
	envName := strings.TrimSpace(req.CredentialEnv)
	if envName == "" || envName == CredentialEnv {
		return env
	}
	value := strings.TrimSpace(t.getenv(envName))
	if value == "" {
		return env
	}
	out := make([]string, 0, len(env)+1)
	prefix := CredentialEnv + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			continue
		}
		out = append(out, entry)
	}
	return append(out, prefix+value)
}

func (t HostTransport) credentialFor(req HostRequest) string {
	envName := strings.TrimSpace(req.CredentialEnv)
	if envName == "" {
		envName = CredentialEnv
	}
	return strings.TrimSpace(t.getenv(envName))
}

func (t HostTransport) getwd() (string, error) {
	if t.Getwd != nil {
		return t.Getwd()
	}
	return os.Getwd()
}

func (t HostTransport) executable() (string, error) {
	if t.Executable != nil {
		return t.Executable()
	}
	return os.Executable()
}

// ResolveHost finds agent/tutor-host.mjs. An explicit PASSQUEST_TUTOR_HOST is
// authoritative; otherwise the working directory, then the executable's
// directory, then its parent are checked. The returned path is absolute.
func ResolveHost(getenv func(string) string, getwd func() (string, error), executablePath func() (string, error)) (string, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	if value := strings.TrimSpace(getenv(HostEnv)); value != "" {
		path, err := filepath.Abs(value)
		if err != nil {
			return "", fmt.Errorf("tutor: resolve %s: %w", HostEnv, err)
		}
		if err := requireFile(path); err != nil {
			return "", err
		}
		return path, nil
	}

	var candidates []string
	if getwd == nil {
		getwd = os.Getwd
	}
	if cwd, err := getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "agent", "tutor-host.mjs"))
	}
	if executablePath == nil {
		executablePath = os.Executable
	}
	if exe, err := executablePath(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "agent", "tutor-host.mjs"),
			filepath.Join(dir, "..", "agent", "tutor-host.mjs"),
		)
	}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if requireFile(absolute) == nil {
			return absolute, nil
		}
	}
	return "", fmt.Errorf("tutor: agent host not found; set %s to agent/tutor-host.mjs (looked in %s)",
		HostEnv, strings.Join(candidates, ", "))
}

func requireFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("tutor: agent host %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("tutor: agent host %s is a directory", path)
	}
	return nil
}

// Call validates the request, runs the host with the inherited environment and
// returns a validated response. Provider and host failures are structured
// responses; only a broken local invocation is a Go error.
func (t HostTransport) Call(ctx context.Context, req HostRequest) (HostResponse, error) {
	if err := validateRequest(req); err != nil {
		return HostResponse{}, err
	}
	host, err := ResolveHost(t.Getenv, t.Getwd, t.Executable)
	if err != nil {
		return HostResponse{}, err
	}
	node := strings.TrimSpace(t.getenv(NodeEnv))
	if node == "" {
		node = "node"
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return HostResponse{}, fmt.Errorf("tutor: encode host request: %w", err)
	}

	timeout := time.Duration(req.TimeoutMS)*time.Millisecond + 5*time.Second
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(callCtx, node, host)
	cmd.Env = t.environFor(req)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr limitedBuffer
	stdout.limit = MaxHostResponseByte
	stderr.limit = 64 << 10
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = 2 * time.Second

	key := t.credentialFor(req)
	runErr := hostproc.Run(cmd)
	if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
		return HostResponse{}, fmt.Errorf("tutor: agent host did not finish within %d ms", req.TimeoutMS+5000)
	}
	if runErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = runErr.Error()
		}
		return HostResponse{}, fmt.Errorf("tutor: agent host failed: %s", redact(detail, key))
	}
	if stdout.truncated {
		return HostResponse{}, fmt.Errorf("tutor: agent host response exceeded %d bytes", MaxHostResponseByte)
	}
	resp, err := decodeResponse(stdout.Bytes(), req)
	if err != nil {
		return HostResponse{}, err
	}
	resp.Text = redact(resp.Text, key)
	if resp.Error != nil {
		resp.Error.Message = redact(resp.Error.Message, key)
	}
	return resp, nil
}

func validateRequest(req HostRequest) error {
	if req.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("tutor: host protocol %d is not supported", req.ProtocolVersion)
	}
	if err := (Config{
		Endpoint:        req.Endpoint,
		Model:           req.Chat.Model,
		TimeoutMS:       req.TimeoutMS,
		MaxOutputTokens: req.Chat.MaxTokens,
		DailyCalls:      DefaultDailyCalls,
	}).Validate(); err != nil {
		return err
	}
	if req.MaxResponseChars <= 0 || req.MaxResponseChars > MaxResponseChars {
		return fmt.Errorf("tutor: max response chars %d is invalid", req.MaxResponseChars)
	}
	if req.Chat.Temperature != 0 || req.Chat.Stream {
		return errors.New("tutor: chat request must be a non-streaming zero-temperature request")
	}
	body, err := json.Marshal(req.Chat)
	if err != nil {
		return fmt.Errorf("tutor: encode chat: %w", err)
	}
	if req.Body != string(body) {
		return errors.New("tutor: host request body is not the serialized chat")
	}
	if len(req.Chat.Messages) < 2 {
		return errors.New("tutor: chat request needs system and user messages")
	}
	total := 0
	for i, message := range req.Chat.Messages {
		if strings.TrimSpace(message.Role) == "" {
			return fmt.Errorf("tutor: message %d has no role", i)
		}
		if strings.TrimSpace(message.Content) == "" {
			return fmt.Errorf("tutor: message %d has no content", i)
		}
		total += utf8.RuneCountInString(message.Content)
	}
	if total > MaxMessagesChars {
		return fmt.Errorf("tutor: messages are %d characters, limit is %d", total, MaxMessagesChars)
	}
	return nil
}

func decodeResponse(data []byte, req HostRequest) (HostResponse, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var resp HostResponse
	if err := dec.Decode(&resp); err != nil {
		return HostResponse{}, fmt.Errorf("tutor: decode agent host response: %w", err)
	}
	if dec.More() {
		return HostResponse{}, errors.New("tutor: agent host response has trailing data")
	}
	if resp.ProtocolVersion != ProtocolVersion {
		return HostResponse{}, fmt.Errorf("tutor: agent host protocol %d is not supported", resp.ProtocolVersion)
	}
	if resp.LatencyMS < 0 {
		resp.LatencyMS = 0
	}
	if resp.HTTPStatus < 0 || resp.HTTPStatus > 599 {
		resp.HTTPStatus = 0
	}
	switch resp.Status {
	case "ok":
		if resp.Error != nil {
			return HostResponse{}, errors.New("tutor: successful host response carries an error")
		}
		if !resp.RequestSent {
			return HostResponse{}, errors.New("tutor: successful host response does not confirm a sent request")
		}
		if strings.TrimSpace(resp.Text) == "" {
			return HostResponse{}, errors.New("tutor: agent host returned empty text")
		}
		if !utf8.ValidString(resp.Text) || strings.ContainsRune(resp.Text, 0) {
			return HostResponse{}, errors.New("tutor: agent host returned invalid text")
		}
		if ContainsUnsafeControl(resp.Text) {
			return HostResponse{}, errors.New("tutor: agent host returned text with a terminal control character")
		}
		if utf8.RuneCountInString(resp.Text) > req.MaxResponseChars {
			return HostResponse{}, fmt.Errorf("tutor: model text is %d characters, limit is %d",
				utf8.RuneCountInString(resp.Text), req.MaxResponseChars)
		}
		if resp.Usage != nil && (resp.Usage.PromptTokens < 0 || resp.Usage.CompletionTokens < 0) {
			return HostResponse{}, errors.New("tutor: agent host returned negative token usage")
		}
		resp.Text = strings.TrimSpace(resp.Text)
	case "error":
		if resp.Error == nil {
			return HostResponse{}, errors.New("tutor: failed host response has no error")
		}
		if !ValidHostErrorCode(resp.Error.Code) {
			resp.Error.Code = "host_error"
		}
		if strings.TrimSpace(resp.Error.Message) == "" {
			resp.Error.Message = resp.Error.Code
		}
		if len([]rune(resp.Error.Message)) > 800 {
			resp.Error.Message = string([]rune(resp.Error.Message)[:800]) + "…"
		}
		resp.Text = ""
	default:
		return HostResponse{}, fmt.Errorf("tutor: agent host status %q is invalid", resp.Status)
	}
	return resp, nil
}

func redact(value, key string) string {
	if key == "" {
		return value
	}
	return strings.ReplaceAll(value, key, "[redacted]")
}

// limitedBuffer keeps at most limit bytes and records whether it dropped any.
// It always reports a full write so a child process cannot block on a short
// write when its output is oversized.
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
