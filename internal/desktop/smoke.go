package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// The smoke hook drives one launch from a file, never from a socket: with
// EnvScript set, the content of that file is evaluated in the page as the
// body of an async function once the page has loaded, and the outcome is
// written to EnvResult as {"ok":true,"value":…} or {"ok":false,"error":"…"};
// then the app quits with exit code 0. Only a result that cannot be written
// makes the exit code non-zero.

// smokeTimeout bounds the script; smokeLoadGrace is the extra time the page
// gets to finish loading before the hook gives up on it.
const (
	smokeTimeout   = 40 * time.Second
	smokeLoadGrace = 10 * time.Second
)

type smokeHook struct {
	script     string
	readErr    error
	resultPath string
	timeout    time.Duration

	once        sync.Once
	writeFailed atomic.Bool
	quit        func()
}

// smokeFromEnv returns the hook when EnvScript is set. A script that cannot
// be read is reported through the result file once the page has loaded.
func smokeFromEnv(getenv func(string) string, quit func()) (*smokeHook, error) {
	scriptPath := strings.TrimSpace(getenv(EnvScript))
	if scriptPath == "" {
		return nil, nil
	}
	resultPath := strings.TrimSpace(getenv(EnvResult))
	if resultPath == "" {
		return nil, fmt.Errorf("%s is set but %s is not", EnvScript, EnvResult)
	}
	h := &smokeHook{resultPath: resultPath, timeout: smokeTimeout, quit: quit}
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		h.readErr = fmt.Errorf("read script: %w", err)
	}
	h.script = string(script)
	return h, nil
}

// run evaluates the script with eval under the hook's timeout and finishes.
func (h *smokeHook) run(eval func(ctx context.Context, script string) (any, error)) {
	if h.readErr != nil {
		h.finish(nil, h.readErr)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()
	value, err := eval(ctx, h.script)
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		err = fmt.Errorf("script did not finish within %s: %w", h.timeout, err)
	}
	h.finish(value, err)
}

// finish writes the result once and quits.
func (h *smokeHook) finish(value any, err error) {
	h.once.Do(func() {
		if writeErr := os.WriteFile(h.resultPath, smokeResult(value, err), 0o644); writeErr != nil {
			fmt.Fprintf(os.Stderr, "passquest-desktop: write smoke result: %v\n", writeErr)
			h.writeFailed.Store(true)
		}
		if h.quit != nil {
			h.quit()
		}
	})
}

// smokeResult encodes the result document.
func smokeResult(value any, err error) []byte {
	doc := map[string]any{"ok": true, "value": value}
	if err != nil {
		doc = map[string]any{"ok": false, "error": err.Error()}
	}
	b, marshalErr := json.Marshal(doc)
	if marshalErr != nil {
		b, _ = json.Marshal(map[string]any{"ok": false, "error": "encode result: " + marshalErr.Error()})
	}
	return b
}
