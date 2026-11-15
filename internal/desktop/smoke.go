package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
)

// The smoke hook drives one launch from a file, never from a socket: with
// EnvScript set, the content of that file is evaluated in the page as the
// body of an async function once the page has loaded, and the outcome is
// written to EnvResult as {"ok":true,"value":…} or {"ok":false,"error":"…"};
// then the app quits with exit code 0. Only a result that cannot be written
// makes the exit code non-zero.

// EnvContentSize=WxH (smoke launches only) makes the page area W by H DIPs
// and bypasses the remembered window state, so a layout audit measures the
// viewport it asked for rather than whatever size the last window had
// (Q-0012-05). It is read only when EnvScript is set.
const EnvContentSize = "PASSQUEST_DESKTOP_CONTENT_SIZE"

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
	// contentW and contentH are EnvContentSize; zero leaves the window as it is.
	contentW, contentH int
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
	if size := strings.TrimSpace(getenv(EnvContentSize)); size != "" {
		w, ht, err := parseContentSize(size)
		if err != nil {
			return nil, err
		}
		h.contentW, h.contentH = w, ht
	}
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		h.readErr = fmt.Errorf("read script: %w", err)
	}
	h.script = string(script)
	return h, nil
}

var contentSizePattern = regexp.MustCompile(`^([0-9]{1,5})x([0-9]{1,5})$`)

// parseContentSize reads EnvContentSize: two positive integers joined by x.
func parseContentSize(value string) (int, int, error) {
	m := contentSizePattern.FindStringSubmatch(value)
	if m == nil {
		return 0, 0, fmt.Errorf("%s=%q: want WIDTHxHEIGHT, for example 1280x800", EnvContentSize, value)
	}
	w, _ := strconv.Atoi(m[1])
	h, _ := strconv.Atoi(m[2])
	if w == 0 || h == 0 {
		return 0, 0, fmt.Errorf("%s=%q: width and height must be positive", EnvContentSize, value)
	}
	return w, h, nil
}

// sizeWindow applies EnvContentSize to the main window's options: the size is
// the page area's and no window state is restored or saved.
func (h *smokeHook) sizeWindow(o *mygo.WindowOptions) {
	if h == nil || h.contentW == 0 {
		return
	}
	o.Width, o.Height, o.UseContentSize, o.StateKey = h.contentW, h.contentH, true, ""
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
