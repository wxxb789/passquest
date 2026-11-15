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
	// loadTimeout is how long the page may take to load before the hook
	// gives up on it.
	loadTimeout time.Duration

	mu          sync.Mutex
	phase       int
	once        sync.Once
	writeFailed atomic.Bool
	quit        func()
}

// Smoke phases. The hook waits for the page; exactly one of the first load
// (the script runs) and the load watchdog (a failure is written) moves it on.
const (
	smokeWaiting = iota
	smokeRunning
	smokeDone
)

// smokePage is the part of a MyGo page the smoke hook drives.
type smokePage interface {
	OnDidFinishLoad(fn func()) (off func())
	EvalContext(ctx context.Context, code string) (any, error)
}

// attach drives the hook from page. The first load stops the load watchdog
// and runs the script under its own timeout; a watchdog that fires first
// writes the failure. The transition out of waiting happens once, under h.mu,
// so a late load can never race the watchdog and the result is written once.
func (h *smokeHook) attach(page smokePage) {
	watchdog := time.AfterFunc(h.loadTimeout, func() {
		if h.advance(smokeWaiting, smokeDone) {
			h.finish(nil, fmt.Errorf("the page did not finish loading within %s", h.loadTimeout))
		}
	})
	page.OnDidFinishLoad(func() {
		if h.advance(smokeWaiting, smokeRunning) {
			watchdog.Stop()
			go h.run(page.EvalContext)
		}
	})
}

// advance moves the hook from one phase to another and reports whether it
// was in from.
func (h *smokeHook) advance(from, to int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.phase != from {
		return false
	}
	h.phase = to
	return true
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
	h := &smokeHook{resultPath: resultPath, timeout: smokeTimeout, loadTimeout: smokeTimeout + smokeLoadGrace, quit: quit}
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
