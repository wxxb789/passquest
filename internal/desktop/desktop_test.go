package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/wxxb789/passquest/internal/cli"
	"github.com/wxxb789/passquest/internal/studio"
)

func envOf(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func TestResolveStorage(t *testing.T) {
	userDir := filepath.Join(t.TempDir(), "Pass Quest")
	// MyGo's PathUserData creates the directory 0755 before resolveStorage runs.
	userData := func() (string, error) { return userDir, os.MkdirAll(userDir, 0o755) }

	got, err := resolveStorage(envOf(nil), userData)
	if err != nil {
		t.Fatal(err)
	}
	if got.DataDir != userDir || got.Database != filepath.Join(userDir, DatabaseFile) {
		t.Fatalf("default storage = %+v", got)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(userDir); err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("app-owned data directory mode = %v (%v), want 0700", info.Mode().Perm(), err)
		}
	}

	override := filepath.Join(t.TempDir(), "nested", "data")
	got, err = resolveStorage(envOf(map[string]string{EnvData: override}), func() (string, error) {
		t.Fatal("user data directory consulted despite the override")
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.DataDir != override || got.Database != filepath.Join(override, DatabaseFile) {
		t.Fatalf("overridden storage = %+v", got)
	}
	if info, err := os.Stat(override); err != nil || !info.IsDir() {
		t.Fatalf("data directory not created: %v", err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("created data directory mode = %v, want 0700", info.Mode().Perm())
	}

	db := filepath.Join(t.TempDir(), "elsewhere.sqlite")
	got, err = resolveStorage(envOf(map[string]string{EnvDB: db}), userData)
	if err != nil {
		t.Fatal(err)
	}
	if got.DataDir != userDir || got.Database != db {
		t.Fatalf("PASSQUEST_DB storage = %+v", got)
	}

	if _, err := resolveStorage(envOf(nil), func() (string, error) { return "", errors.New("no home") }); err == nil {
		t.Fatal("a failing user data directory was accepted")
	}
}

func TestAppIdentity(t *testing.T) {
	if got := appName(envOf(nil)); got != AppName {
		t.Fatalf("appName = %q", got)
	}
	a := appName(envOf(map[string]string{EnvData: "one"}))
	b := appName(envOf(map[string]string{EnvData: "two"}))
	if a == AppName || b == AppName || a == b || !strings.HasPrefix(a, AppName+" ") {
		t.Fatalf("overridden data directories share an identity: %q, %q", a, b)
	}
	if got := secretService(envOf(nil)); got != AppIdentifier {
		t.Fatalf("secretService = %q", got)
	}
	if got := secretService(envOf(map[string]string{EnvSecretService: "smoke-1"})); got != "smoke-1" {
		t.Fatalf("secretService override = %q", got)
	}
}

// fakeSecrets is an in-memory credential store; unsupported mimics a system
// without one.
type fakeSecrets struct {
	mu          sync.Mutex
	values      map[string][]byte
	unsupported bool
	failSet     error
}

func (f *fakeSecrets) Get(key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.unsupported {
		return nil, fmt.Errorf("mygo: secrets: %w", errors.ErrUnsupported)
	}
	value, ok := f.values[key]
	if !ok {
		return nil, mygo.ErrSecretNotFound
	}
	return value, nil
}

func (f *fakeSecrets) Set(key string, value []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failSet != nil {
		return f.failSet
	}
	if f.values == nil {
		f.values = map[string][]byte{}
	}
	f.values[key] = value
	return nil
}

func (f *fakeSecrets) Delete(key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.values, key)
	return nil
}

func TestCredentialStore(t *testing.T) {
	if store := newCredentialStore(&fakeSecrets{unsupported: true}); store != nil {
		t.Fatalf("a system without a credential store got %T", store)
	}
	secrets := &fakeSecrets{}
	store := newCredentialStore(secrets)
	if store == nil {
		t.Fatal("no store over a working credential store")
	}
	if value, found, err := store.Get("openai"); err != nil || found || value != "" {
		t.Fatalf("missing key = %q, %v, %v", value, found, err)
	}
	if err := store.Set("openai", "sk-test-123"); err != nil {
		t.Fatal(err)
	}
	if string(secrets.values[credentialAccount("openai")]) != "sk-test-123" {
		t.Fatalf("stored under %v", secrets.values)
	}
	if value, found, err := store.Get("openai"); err != nil || !found || value != "sk-test-123" {
		t.Fatalf("stored key = %q, %v, %v", value, found, err)
	}
	if err := store.Delete("openai"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := store.Get("openai"); found {
		t.Fatal("key still present after Delete")
	}
	secrets.failSet = errors.New("access denied")
	err := store.Set("openai", "sk-secret-456")
	if err == nil || strings.Contains(err.Error(), "sk-secret-456") {
		t.Fatalf("refused Set = %v", err)
	}
	secrets.unsupported = true
	if _, _, err := store.Get("openai"); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("unsupported Get = %v", err)
	}
}

func decodeCallError(t *testing.T, err error) studio.Error {
	t.Helper()
	if err == nil {
		t.Fatal("no error")
	}
	var doc studio.Error
	if decodeErr := json.Unmarshal([]byte(err.Error()), &doc); decodeErr != nil {
		t.Fatalf("error message %q is not JSON: %v", err.Error(), decodeErr)
	}
	return doc
}

func TestCallError(t *testing.T) {
	if callError(nil) != nil {
		t.Fatal("nil error converted")
	}
	typed := &studio.Error{Code: "pool_floor", Message: "not enough items", Action: "import a source"}
	if got := decodeCallError(t, callError(fmt.Errorf("wrapped: %w", typed))); got != *typed {
		t.Fatalf("studio error = %+v", got)
	}
	if got := decodeCallError(t, callError(errors.New("disk full"))); got.Code != "internal_error" || got.Message != "disk full" {
		t.Fatalf("plain error = %+v", got)
	}
}

func openTestStudy(t *testing.T, events func(BriefEvent)) *Study {
	t.Helper()
	study := newStudy(events)
	// No Node: host calls are refused up front, whatever this machine has.
	env := map[string]string{"PASSQUEST_NODE": filepath.Join(t.TempDir(), "no-node-here")}
	if err := study.open(cli.WorkspaceOptions{
		DBPath:      filepath.Join(t.TempDir(), DatabaseFile),
		Getenv:      envOf(env),
		Environ:     func() []string { return nil },
		Credentials: newCredentialStore(&fakeSecrets{}),
	}); err != nil {
		t.Fatal(err)
	}
	return study
}

func TestStudyForwardsAndConvertsErrors(t *testing.T) {
	starting := newStudy(func(BriefEvent) {})
	if got := decodeCallError(t, starting.SetLearnerSettings(studio.LearnerSettings{})); got.Code != "starting" {
		t.Fatalf("call before open = %+v", got)
	}

	study := openTestStudy(t, func(BriefEvent) {})
	today, err := study.Today()
	if err != nil {
		t.Fatal(err)
	}
	if today.Prep.PassThreshold != studio.PassThresholdUnknown || today.GeneratedAt == "" {
		t.Fatalf("today = %+v", today.Prep)
	}
	if err := study.SetLearnerSettings(studio.LearnerSettings{ExamDate: "2027-03-01", DailyMinutes: 45}); err != nil {
		t.Fatal(err)
	}
	if today, err = study.Today(); err != nil || today.Prep.ExamDate != "2027-03-01" || today.Prep.DailyMinutes != 45 {
		t.Fatalf("settings not forwarded: %+v, %v", today.Prep, err)
	}
	_, err = study.StartPractice(studio.PracticeRequest{Kind: "nonsense"})
	if got := decodeCallError(t, err); got.Code != "invalid_input" {
		t.Fatalf("invalid practice = %+v", got)
	}
	services, err := study.Services()
	if err != nil || !services.CredentialStore {
		t.Fatalf("services = %+v, %v", services, err)
	}
}

func TestStudyResumeBriefRunsInBackground(t *testing.T) {
	events := make(chan BriefEvent, 4)
	study := openTestStudy(t, func(e BriefEvent) { events <- e })
	if err := study.ResumeBrief(); err != nil {
		t.Fatal(err)
	}
	if e := <-events; e.Phase != BriefStarted {
		t.Fatalf("first event = %+v", e)
	}
	select {
	case e := <-events:
		if e.Phase != BriefFinished || e.Error == nil || e.Error.Code != "node_unavailable" || e.Status != nil {
			t.Fatalf("finished event = %+v", e)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("no finished event")
	}
	if !study.drain(5 * time.Second) {
		t.Fatal("background resume still counted after it finished")
	}
}

func TestStudyDrainRefusesNewCallsAndBoundsTheWait(t *testing.T) {
	study := openTestStudy(t, func(BriefEvent) {})
	if study.busy() {
		t.Fatal("idle study reports busy")
	}
	ws, err := study.begin() // a call that is still running
	if err != nil || ws == nil {
		t.Fatal(err)
	}
	if !study.busy() {
		t.Fatal("running call not counted")
	}
	start := time.Now()
	if study.drain(50 * time.Millisecond) {
		t.Fatal("drain reported done with a call running")
	}
	if waited := time.Since(start); waited > 5*time.Second {
		t.Fatalf("drain waited %s", waited)
	}
	_, err = study.Today()
	if got := decodeCallError(t, err); got.Code != "quitting" {
		t.Fatalf("call while quitting = %+v", got)
	}
	study.end()
	if !study.drain(time.Second) {
		t.Fatal("drain did not finish once the call returned")
	}
}

func readResult(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("result %s: %v", b, err)
	}
	return doc
}

func TestSmokeHook(t *testing.T) {
	dir := t.TempDir()
	if h, err := smokeFromEnv(envOf(nil), nil); h != nil || err != nil {
		t.Fatalf("no script = %v, %v", h, err)
	}
	script := filepath.Join(dir, "scenario.js")
	if err := os.WriteFile(script, []byte("return 1 + 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := smokeFromEnv(envOf(map[string]string{EnvScript: script}), nil); err == nil {
		t.Fatal("a script without a result path was accepted")
	}

	launch := func(name, scriptPath string) (*smokeHook, string, *int) {
		result := filepath.Join(dir, name+".json")
		quits := new(int)
		h, err := smokeFromEnv(envOf(map[string]string{EnvScript: scriptPath, EnvResult: result}), func() { *quits++ })
		if err != nil || h == nil {
			t.Fatalf("%s: %v, %v", name, h, err)
		}
		return h, result, quits
	}

	h, result, quits := launch("ok", script)
	h.run(func(_ context.Context, body string) (any, error) {
		if body != "return 1 + 1" {
			t.Errorf("evaluated %q", body)
		}
		return map[string]any{"two": 2.0}, nil
	})
	h.finish(nil, errors.New("late")) // a second finish changes nothing
	doc := readResult(t, result)
	if value, _ := doc["value"].(map[string]any); doc["ok"] != true || value["two"] != 2.0 || *quits != 1 {
		t.Fatalf("ok result = %v, quits %d", doc, *quits)
	}

	h, result, _ = launch("throws", script)
	h.run(func(context.Context, string) (any, error) { return nil, &mygo.EvalError{Message: "boom"} })
	if doc := readResult(t, result); doc["ok"] != false || !strings.Contains(fmt.Sprint(doc["error"]), "boom") {
		t.Fatalf("throwing script = %v", doc)
	}

	h, result, _ = launch("slow", script)
	h.timeout = 20 * time.Millisecond
	h.run(func(ctx context.Context, _ string) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if doc := readResult(t, result); doc["ok"] != false || !strings.Contains(fmt.Sprint(doc["error"]), "did not finish") {
		t.Fatalf("slow script = %v", doc)
	}

	h, result, quits = launch("missing", filepath.Join(dir, "absent.js"))
	h.run(func(context.Context, string) (any, error) {
		t.Error("an unreadable script was evaluated")
		return nil, nil
	})
	if doc := readResult(t, result); doc["ok"] != false || !strings.Contains(fmt.Sprint(doc["error"]), "read script") || *quits != 1 {
		t.Fatalf("missing script = %v", doc)
	}

	h, _, _ = launch("unwritable", script)
	h.resultPath = filepath.Join(dir, "no-such-dir", "result.json")
	h.run(func(context.Context, string) (any, error) { return 1, nil })
	if !h.writeFailed.Load() {
		t.Fatal("an unwritable result was not reported")
	}
}

// fakePage loads when told to and evaluates with eval.
type fakePage struct {
	mu     sync.Mutex
	loaded []func()
	eval   func(ctx context.Context, code string) (any, error)
}

func (p *fakePage) OnDidFinishLoad(fn func()) func() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.loaded = append(p.loaded, fn)
	return func() {}
}

func (p *fakePage) EvalContext(ctx context.Context, code string) (any, error) {
	return p.eval(ctx, code)
}

func (p *fakePage) load() {
	p.mu.Lock()
	fns := append([]func(){}, p.loaded...)
	p.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

// A page that loads late gets its whole script budget: the load watchdog stops
// at load and never races the running script, and the result is written once.
func TestSmokeLoadWatchdogDoesNotRaceALateScript(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "scenario.js")
	if err := os.WriteFile(script, []byte("return 'done'"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(dir, "result.json")
	var quits atomic.Int32
	h, err := smokeFromEnv(envOf(map[string]string{EnvScript: script, EnvResult: result}), func() { quits.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	// The script needs longer than what is left of the load watchdog after
	// the late load, but less than its own timeout.
	h.timeout = 400 * time.Millisecond
	h.loadTimeout = 200 * time.Millisecond
	page := &fakePage{eval: func(ctx context.Context, _ string) (any, error) {
		select {
		case <-time.After(250 * time.Millisecond):
			return "done", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	h.attach(page)
	time.Sleep(100 * time.Millisecond)
	page.load()
	page.load() // a second load event starts nothing
	for deadline := time.Now().Add(5 * time.Second); quits.Load() == 0; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("no result")
		}
	}
	time.Sleep(50 * time.Millisecond)
	if doc := readResult(t, result); doc["ok"] != true || doc["value"] != "done" || quits.Load() != 1 {
		t.Fatalf("late-loaded script result = %v, quits %d", doc, quits.Load())
	}

	// A page that never loads still gets a failure result from the watchdog.
	never, err := smokeFromEnv(envOf(map[string]string{EnvScript: script, EnvResult: filepath.Join(dir, "never.json")}), nil)
	if err != nil {
		t.Fatal(err)
	}
	never.loadTimeout = 20 * time.Millisecond
	never.attach(&fakePage{eval: func(context.Context, string) (any, error) {
		t.Error("evaluated without a load")
		return nil, nil
	}})
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(dir, "never.json")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no watchdog result")
		}
	}
	if doc := readResult(t, filepath.Join(dir, "never.json")); doc["ok"] != false || !strings.Contains(fmt.Sprint(doc["error"]), "did not finish loading") {
		t.Fatalf("never-loaded result = %v", doc)
	}
}

func TestStartURL(t *testing.T) {
	frontend, err := frontendFS()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.html", "not-built.html"} {
		if _, err := frontend.Open(name); err != nil {
			t.Fatalf("%s not embedded: %v", name, err)
		}
	}
	want := "/not-built.html"
	if _, err := os.Stat(filepath.Join("web", "dist", "main.js")); err == nil {
		want = "/"
	}
	if got := startURL(frontend); got != want {
		t.Fatalf("startURL = %q, want %q", got, want)
	}
}
