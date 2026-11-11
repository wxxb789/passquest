package anki

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
)

const maxHostResponseBytes = 4 << 20

// HostTransport runs agent/anki-host.mjs as a child process and exchanges one
// JSON document over stdin/stdout. It carries no credential.
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

// ResolveHost finds agent/anki-host.mjs. An explicit PASSQUEST_ANKI_HOST is
// authoritative; otherwise the working directory, then the executable's
// directory, then its parent are checked.
func ResolveHost(getenv func(string) string, getwd func() (string, error), executablePath func() (string, error)) (string, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	if value := strings.TrimSpace(getenv(HostEnv)); value != "" {
		path, err := filepath.Abs(value)
		if err != nil {
			return "", fmt.Errorf("anki: resolve %s: %w", HostEnv, err)
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
		candidates = append(candidates, filepath.Join(cwd, "agent", "anki-host.mjs"))
	}
	if executablePath == nil {
		executablePath = os.Executable
	}
	if exe, err := executablePath(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "agent", "anki-host.mjs"),
			filepath.Join(dir, "..", "agent", "anki-host.mjs"),
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
	return "", fmt.Errorf("anki: host not found; set %s to agent/anki-host.mjs (looked in %s)",
		HostEnv, strings.Join(candidates, ", "))
}

func requireHostFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("anki: host %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("anki: host %s is a directory", path)
	}
	return nil
}

// Call validates the request, runs the host and validates its response. A
// missing Node binary, missing host file or malformed document is a Go error,
// which the CLI reports as anki_unavailable without changing the outbox.
func (t HostTransport) Call(ctx context.Context, req HostRequest) (HostResponse, error) {
	if err := ValidateRequest(req); err != nil {
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
		return HostResponse{}, fmt.Errorf("anki: encode host request: %w", err)
	}
	timeout := time.Duration(req.TimeoutMS)*time.Millisecond + 5*time.Second
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(callCtx, node, host)
	cmd.Env = t.environ()
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr limitedBuffer
	stdout.limit = maxHostResponseBytes
	stderr.limit = 64 << 10
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = 2 * time.Second

	runErr := cmd.Run()
	if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
		return HostResponse{}, fmt.Errorf("anki: host did not finish within %d ms", req.TimeoutMS+5000)
	}
	if runErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = runErr.Error()
		}
		return HostResponse{}, fmt.Errorf("anki: host failed: %s", detail)
	}
	if stdout.truncated {
		return HostResponse{}, fmt.Errorf("anki: host response exceeded %d bytes", maxHostResponseBytes)
	}
	var resp HostResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return HostResponse{}, fmt.Errorf("anki: host response is not JSON: %w", err)
	}
	if err := ValidateResponse(req, resp); err != nil {
		return HostResponse{}, err
	}
	return resp, nil
}

// limitedBuffer keeps at most limit bytes and records whether it dropped any.
type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
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
