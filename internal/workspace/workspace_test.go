package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/studio"
)

// wsClock advances by step on every reading and can jump forward, so elapsed
// time and assessment deadlines are deterministic.
type wsClock struct {
	mu   sync.Mutex
	now  time.Time
	step time.Duration
}

func newWSClock() *wsClock { return &wsClock{now: goldenBase, step: time.Second} }

func (c *wsClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(c.step)
	return c.now
}

func (c *wsClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// memCredentials is the in-memory credential store tests inject in place of
// the OS store.
type memCredentials struct {
	mu     sync.Mutex
	values map[string]string
}

func newMemCredentials() *memCredentials { return &memCredentials{values: map[string]string{}} }

func (m *memCredentials) Get(p string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.values[p]
	return value, ok, nil
}

func (m *memCredentials) Set(p, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[p] = key
	return nil
}

func (m *memCredentials) Delete(p string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, p)
	return nil
}

type wsFixture struct {
	db    string
	clock *wsClock
	env   map[string]string
	opts  Options
	w     *Workspace
}

func newWSFixture(t *testing.T, mutate func(*Options)) *wsFixture {
	t.Helper()
	f := &wsFixture{db: newDB(t), clock: newWSClock(), env: map[string]string{}}
	tokens := 0
	f.opts = Options{
		DBPath:     f.db,
		Getenv:     func(key string) string { return f.env[key] },
		Environ:    func() []string { return envList(f.env) },
		Now:        f.clock.Now,
		NewSession: func() string { return "sess-ws" },
		NewToken: func() string {
			tokens++
			return fmt.Sprintf("token-%04d", tokens)
		},
	}
	if mutate != nil {
		mutate(&f.opts)
	}
	f.reopen(t)
	return f
}

func (f *wsFixture) reopen(t *testing.T) {
	t.Helper()
	w, err := Open(f.opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	f.w = w
}

func envList(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, key+"="+value)
	}
	sort.Strings(out)
	return out
}

func studioCode(err error) string {
	var typed *studio.Error
	if errors.As(err, &typed) {
		return typed.Code
	}
	return ""
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func jsonKeys(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func listAttempts(t *testing.T, db string) int {
	t.Helper()
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	records, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	return len(records)
}

func countSpends(t *testing.T, db, surface string) int {
	t.Helper()
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	spends, err := st.Spends()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, spend := range spends {
		if spend.Surface == surface {
			count++
		}
	}
	return count
}

func TestWorkspaceCredentialStoreLayering(t *testing.T) {
	t.Parallel()
	creds := newMemCredentials()
	const storeKey = "store-key-0123456789-not-real"
	const envKey = "env-key-9876543210-not-real"
	f := newWSFixture(t, func(o *Options) { o.Credentials = creds })
	registry := modelRegistryFile(t, modelProvider("alpha", "https://alpha.example.com/v1/chat/completions"))
	f.env[provider.RegistryEnv] = registry
	transport := &fakeTutorTransport{response: fixedTutorResponse("OK")}
	f.opts.TutorTransport = transport
	f.reopen(t)
	envName := provider.CredentialEnvFor("alpha")

	if _, err := f.w.SetCredential("nobody", storeKey); studioCode(err) != "unknown_provider" {
		t.Fatalf("unknown provider = %v", err)
	}
	if _, err := f.w.SetCredential("alpha", "two words"); studioCode(err) != "invalid_input" {
		t.Fatalf("invalid key = %v", err)
	}
	status, err := f.w.SetCredential("alpha", storeKey)
	mustNoErr(t, err)
	if !status.Present || status.From != "store" {
		t.Fatalf("status after set = %+v", status)
	}
	if got := f.w.getenv(envName); got != storeKey {
		t.Fatalf("store layer not used: %q", got)
	}
	if !containsExact(f.w.environ(), envName+"="+storeKey) {
		t.Fatal("child environment lacks the stored credential")
	}

	_, err = f.w.UseModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	test, err := f.w.TestModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	if test.Result != "verified" || len(transport.requests) != 1 {
		t.Fatalf("model test with stored key = %+v", test)
	}
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample})
	mustNoErr(t, err)
	feedback, err := f.w.Answer(item.Token, "A")
	mustNoErr(t, err)
	transport.response = fixedTutorResponse("Look at the second sentence again.")
	reply, err := f.w.CoachAsk(feedback.AttemptID, studio.CoachExplain)
	mustNoErr(t, err)
	if reply.State != "advisory" || !reply.Advisory || reply.VerifiedAnswer || reply.Text == "" {
		t.Fatalf("coach reply = %+v", reply)
	}

	f.env[envName] = envKey
	if got := f.w.getenv(envName); got != envKey {
		t.Fatalf("environment did not win: %q", got)
	}
	if containsExact(f.w.environ(), envName+"="+storeKey) {
		t.Fatal("stored credential shadowed the environment in the child environment")
	}
	services, err := f.w.Services()
	mustNoErr(t, err)
	found := false
	for _, c := range services.Credentials {
		if c.Provider == "alpha" {
			found = c.Present && c.From == "env"
		}
	}
	if !found || !services.CredentialStore {
		t.Fatalf("services = %+v", services)
	}
	delete(f.env, envName)
	cleared, err := f.w.ClearCredential("alpha")
	mustNoErr(t, err)
	if cleared.Present || cleared.From != "none" {
		t.Fatalf("after clear = %+v", cleared)
	}

	for _, value := range []any{status, services, test, reply, feedback} {
		raw, _ := json.Marshal(value)
		if strings.Contains(string(raw), storeKey) || strings.Contains(string(raw), envKey) {
			t.Fatalf("a document carries the key: %s", raw)
		}
	}
	matches, _ := filepath.Glob(f.db + "*")
	if len(matches) == 0 {
		t.Fatal("no database files to scan")
	}
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), storeKey) || strings.Contains(string(data), envKey) {
			t.Fatalf("%s contains a credential", path)
		}
	}

	noStore := newWSFixture(t, nil)
	if _, err := noStore.w.SetCredential("openai", storeKey); studioCode(err) != "credential_store_unavailable" {
		t.Fatalf("set without a store = %v", err)
	}
}

func TestWorkspaceBriefWithoutRun(t *testing.T) {
	t.Parallel()
	f := newWSFixture(t, nil)
	status, err := f.w.BriefStatus()
	mustNoErr(t, err)
	if status.Found || status.State != "none" {
		t.Fatalf("brief status = %+v", status)
	}
}

// Q-0011-03: an environment variable still wins over a stored key, including
// the custom provider's legacy fallback that provider.CredentialValue reads.
func TestWorkspaceLegacyCustomCredentialBeatsStoredKey(t *testing.T) {
	t.Parallel()
	creds := newMemCredentials()
	const storeKey = "store-key-0123456789-not-real"
	const legacyKey = "legacy-key-9876543210-not-real"
	f := newWSFixture(t, func(o *Options) { o.Credentials = creds })
	_, err := f.w.SetCredential(provider.CustomProvider, storeKey)
	mustNoErr(t, err)
	primary := provider.CredentialEnvFor(provider.CustomProvider)
	if _, value, _ := provider.CredentialValue(f.w.getenv, provider.CustomProvider); value != storeKey {
		t.Fatalf("stored custom key not used without an environment key: %q", value)
	}

	f.env[provider.LegacyCredentialEnv] = legacyKey
	if name, value, _ := provider.CredentialValue(f.w.getenv, provider.CustomProvider); value != legacyKey || name != provider.LegacyCredentialEnv {
		t.Fatalf("legacy environment key lost to the store: %s=%q", name, value)
	}
	if containsExact(f.w.environ(), primary+"="+storeKey) {
		t.Fatal("stored custom key shadows the legacy variable in the child environment")
	}
	if status := f.w.credentialStatus(provider.CustomProvider); !status.Present || status.From != "env" {
		t.Fatalf("custom status with legacy variable = %+v", status)
	}
}

func TestLoopbackEndpointParsesTheHost(t *testing.T) {
	t.Parallel()
	for endpoint, want := range map[string]bool{
		"http://127.0.0.1:11434/v1/chat/completions":        true,
		"http://localhost:8080/v1/chat/completions":         true,
		"http://[::1]:8080/v1/chat/completions":             true,
		"https://127.0.0.1.attacker.example/v1":             false,
		"https://127.evil.example/v1/chat/completions":      false,
		"https://localhost.attacker.example/v1":             false,
		"https://api.example.com/v1/chat/completions":       false,
		"https://user@127.0.0.1.example:443/v1/completions": false,
	} {
		if got := loopbackEndpoint(endpoint); got != want {
			t.Errorf("loopbackEndpoint(%q) = %v, want %v", endpoint, got, want)
		}
	}
}
