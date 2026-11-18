package anki

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/hostpaths"
	"github.com/wxxb789/passquest/internal/hostproc"
)

const maxHostResponseBytes = 4 << 20

// HostTransport runs agent/anki-host.mjs as a child process and exchanges one
// JSON document over stdin/stdout. It carries no credential.
type HostTransport struct {
	Getenv     func(string) string
	Environ    func() []string
	Getwd      func() (string, error)
	Executable func() (string, error)
	// Resources is the packaged resources directory; empty in a source
	// checkout, where the lookups stay exactly as they were.
	Resources string
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

// ankiHost is how the shared resolver names the Anki host.
var ankiHost = hostpaths.Host{
	Prefix: "anki",
	Noun:   "host",
	Env:    HostEnv,
	Script: "anki-host.mjs",
}

// ResolveHost finds agent/anki-host.mjs. An explicit PASSQUEST_ANKI_HOST is
// authoritative; otherwise the packaged resources directory, the working
// directory, then the executable's directory, then its parent are checked.
func ResolveHost(getenv func(string) string, getwd func() (string, error), executablePath func() (string, error)) (string, error) {
	return hostpaths.ResolveHost(ankiHost, hostpaths.Source{Getenv: getenv, Getwd: getwd, Executable: executablePath})
}

// source is the lookup input shared by host and Node resolution.
func (t HostTransport) source() hostpaths.Source {
	return hostpaths.Source{Getenv: t.Getenv, Getwd: t.Getwd, Executable: t.Executable, Resources: t.Resources}
}

// Call validates the request, runs the host and validates its response. A
// missing Node binary, missing host file or malformed document is a Go error,
// which the CLI reports as anki_unavailable without changing the outbox.
func (t HostTransport) Call(ctx context.Context, req HostRequest) (HostResponse, error) {
	if err := ValidateRequest(req); err != nil {
		return HostResponse{}, err
	}
	host, err := hostpaths.ResolveHost(ankiHost, t.source())
	if err != nil {
		return HostResponse{}, err
	}
	node := hostpaths.ResolveNode(t.source(), runtime.GOOS)
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

	runErr := hostproc.Run(cmd)
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
