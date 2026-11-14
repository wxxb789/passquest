package cli

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/wxxb789/passquest/internal/agent"
	"github.com/wxxb789/passquest/internal/anki"
	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/search"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/studio"
	"github.com/wxxb789/passquest/internal/tutor"
)

// CredentialStore keeps provider keys outside the process environment (the OS
// credential store in the desktop app, an in-memory map in tests). The
// workspace only ever reports presence; values never reach SQLite, logs or a
// studio document.
type CredentialStore interface {
	Get(provider string) (string, bool, error)
	Set(provider, key string) error
	Delete(provider string) error
}

// WorkspaceOptions are the process seams of one desktop workspace. They are
// the CLI's Options seams plus the database path and a credential store.
type WorkspaceOptions struct {
	// DBPath is the database the workspace opens per call. Empty selects
	// PASSQUEST_DB or the CLI default, exactly as the CLI does.
	DBPath string
	// Getenv and Environ read the process environment; nil means os.Getenv
	// and os.Environ. Credentials from Credentials are layered under them:
	// a non-empty variable always wins (Q-0011-03).
	Getenv  func(string) string
	Environ func() []string
	Now     func() time.Time
	// Executable locates the bundled Node hosts next to the binary.
	Executable func() (string, error)
	NewSession func() string
	// NewToken mints practice presentation tokens; nil uses random bits.
	NewToken    func() string
	Credentials CredentialStore

	TutorTransport  tutor.Transport
	AnkiTransport   anki.Transport
	SearchTransport search.Adapter
	AgentRunner     agent.Runner
	NewRunID        func() (string, error)
	NewHostToken    func() (string, error)
}

// Workspace is the desktop's typed service over the CLI's orchestration. One
// long-lived value serves one window; every call opens the store for the
// operation, exactly like a CLI command, so a second process (the durable host
// recording a step) can write between calls. Short local writes are
// serialised; calls that wait on a host process (coach ask, model test, note
// push, brief resume) are not, and rely on the store's reservations and
// transactions exactly as two CLI processes would.
type Workspace struct {
	mu             sync.Mutex
	dbPath         string
	baseGetenv     func(string) string
	baseEnviron    func() []string
	credentials    CredentialStore
	credentialEnvs map[string]string
	newToken       func() string
	opt            Options
	// hostTutor, hostAnki and hostAgent are true when the default Node host
	// transports are in use, so a missing Node is reported before the call.
	hostTutor, hostAnki, hostAgent bool
}

// OpenWorkspace validates the options, migrates the database once and
// returns the workspace.
func OpenWorkspace(options WorkspaceOptions) (*Workspace, error) {
	w := &Workspace{
		baseGetenv:  options.Getenv,
		baseEnviron: options.Environ,
		credentials: options.Credentials,
		newToken:    options.NewToken,
	}
	if w.baseGetenv == nil {
		w.baseGetenv = os.Getenv
	}
	if w.baseEnviron == nil {
		w.baseEnviron = os.Environ
	}
	if w.newToken == nil {
		w.newToken = newPresentationToken
	}
	registry, _, err := provider.Load(w.baseGetenv)
	if err != nil {
		return nil, &studio.Error{Code: "invalid_registry", Message: err.Error()}
	}
	w.credentialEnvs = map[string]string{}
	for _, p := range registry.Providers {
		w.credentialEnvs[provider.CredentialEnvFor(p.ID)] = p.ID
	}
	w.dbPath = strings.TrimSpace(options.DBPath)
	if w.dbPath == "" {
		w.dbPath = databasePath("", w.baseGetenv)
	}
	w.opt = Options{
		Stdin:           strings.NewReader(""),
		Now:             options.Now,
		NewSession:      options.NewSession,
		Getenv:          w.getenv,
		TutorTransport:  options.TutorTransport,
		AnkiTransport:   options.AnkiTransport,
		SearchTransport: options.SearchTransport,
		AgentRunner:     options.AgentRunner,
		NewRunID:        options.NewRunID,
		NewHostToken:    options.NewHostToken,
	}
	if w.opt.Now == nil {
		w.opt.Now = time.Now
	}
	if w.opt.NewSession == nil {
		w.opt.NewSession = newSessionID
	}
	// Side-effect exposure rows use the same clock as everything else the
	// window records.
	w.opt.PresentationNow = w.opt.Now
	w.hostTutor = w.opt.TutorTransport == nil
	w.hostAnki = w.opt.AnkiTransport == nil
	w.hostAgent = w.opt.AgentRunner == nil
	if w.opt.TutorTransport == nil {
		w.opt.TutorTransport = tutor.HostTransport{Getenv: w.getenv, Environ: w.environ, Executable: options.Executable}
	}
	if w.opt.AnkiTransport == nil {
		w.opt.AnkiTransport = anki.HostTransport{Getenv: w.getenv, Environ: w.environ, Executable: options.Executable}
	}
	if w.opt.SearchTransport == nil {
		w.opt.SearchTransport = search.HostTransport{Getenv: w.getenv, Environ: w.environ, Executable: options.Executable}
	}
	if w.opt.AgentRunner == nil {
		w.opt.AgentRunner = agent.HostRunner{Getenv: w.getenv, Environ: w.environ, Executable: options.Executable}
	}
	st, err := w.open()
	if err != nil {
		return nil, err
	}
	st.Close()
	return w, nil
}

// DatabasePath is the absolute database path the workspace writes.
func (w *Workspace) DatabasePath() string { return w.dbPath }

func newPresentationToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("pt-%d", time.Now().UnixNano())
	}
	return "pt-" + hex.EncodeToString(b[:])
}

// getenv is the environment the workspace hands to every provider, tutor,
// Anki and agent path: a non-empty variable wins; a provider credential
// variable that is unset falls back to the credential store only when the
// environment supplies no credential for that provider at all.
func (w *Workspace) getenv(name string) string {
	if value := w.baseGetenv(name); strings.TrimSpace(value) != "" {
		return value
	}
	if value, ok := w.storedCredential(name); ok {
		return value
	}
	return w.baseGetenv(name)
}

// environ is the child-process environment: the process environment plus a
// stored credential for every provider the environment leaves without one,
// because the durable host reads its key from its own environment.
func (w *Workspace) environ() []string {
	env := w.baseEnviron()
	for name := range w.credentialEnvs {
		if value, ok := w.storedCredential(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}

// storedCredential is the stored key for the provider whose credential
// variable is name. The environment wins (Q-0011-03): any variable
// provider.CredentialValue reads for that provider, including the custom
// provider's legacy fallback, suppresses the stored key.
func (w *Workspace) storedCredential(name string) (string, bool) {
	providerID, ok := w.credentialEnvs[name]
	if !ok || w.credentials == nil {
		return "", false
	}
	if _, _, present := provider.CredentialValue(w.baseGetenv, providerID); present {
		return "", false
	}
	value, found, err := w.credentials.Get(providerID)
	if err != nil || !found || strings.TrimSpace(value) == "" {
		return "", false
	}
	return value, true
}

func (w *Workspace) open() (*store.Store, error) {
	st, err := store.Open(w.dbPath)
	if err != nil {
		return nil, &studio.Error{Code: "internal_error", Message: err.Error()}
	}
	return st, nil
}

// withStore runs fn with a store opened for this call while holding the write
// lock.
func (w *Workspace) withStore(fn func(st *store.Store) error) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	st, err := w.open()
	if err != nil {
		return err
	}
	defer st.Close()
	return workspaceError(fn(st))
}

// guard refuses command while an assessment run is active, with the CLI's
// assessment_active code and message.
func (w *Workspace) guard(st *store.Store, command string) error {
	return activeAssessmentRefusal(st, w.opt.Now, command)
}

// workspaceError converts an orchestration error into a typed studio error
// carrying the same code the CLI would print.
func workspaceError(err error) error {
	if err == nil {
		return nil
	}
	var typed *studio.Error
	if errors.As(err, &typed) {
		return withAction(typed)
	}
	var refusal tacticRefusal
	if errors.As(err, &refusal) {
		return withAction(&studio.Error{Code: refusal.code, Message: refusal.message})
	}
	switch {
	case errors.Is(err, source.ErrNotPracticeable):
		return &studio.Error{Code: "not_practiceable", Message: err.Error()}
	case errors.Is(err, source.ErrInvalidInput), errors.Is(err, source.ErrToolMissing):
		return &studio.Error{Code: "invalid_input", Message: err.Error()}
	case errors.Is(err, store.ErrNotFound):
		return &studio.Error{Code: "not_found", Message: err.Error()}
	case errors.Is(err, store.ErrAssessmentRunState):
		return &studio.Error{Code: "run_state", Message: err.Error()}
	case errors.Is(err, store.ErrAssessmentWindowElapsed):
		return &studio.Error{Code: "run_abandoned", Message: err.Error()}
	}
	return &studio.Error{Code: "internal_error", Message: err.Error()}
}

// codeActions are the learner's next step for codes the window cannot fix by
// itself.
var codeActions = map[string]string{
	"assessment_active":     "finish or abandon the active assessment run first",
	"no_credential":         "add the provider key in Settings, or set its environment variable",
	"missing_credential":    "add the provider key in Settings, or set its environment variable",
	"capability_unverified": "run the model test for the active model in Settings",
	"not_configured":        "choose a model in Settings",
	"node_unavailable":      "install Node.js 22, or set PASSQUEST_NODE to its path, then retry",
	"pool_floor":            "import and review more source pages and questions",
	"budget_exhausted":      "wait for the UTC day to reset or raise the daily call budget",
	"consent_required":      "grant consent for this source in the coach panel",
}

func withAction(e *studio.Error) *studio.Error {
	if e.Action == "" {
		e.Action = codeActions[e.Code]
	}
	return e
}

// commandResult is one in-process CLI invocation.
type commandResult struct {
	code   int
	stdout []byte
	stderr []byte
}

// run executes one CLI command in-process against the workspace database with
// --json, using the workspace's clock, environment and transports. The
// commands it runs are the CLI's own orchestration; the workspace decodes
// their protocol documents into studio types instead of reimplementing them.
func (w *Workspace) run(args ...string) commandResult {
	var stdout, stderr bytes.Buffer
	opt := w.opt
	opt.Stdout = &stdout
	opt.Stderr = &stderr
	opt.Stdin = strings.NewReader("")
	full := make([]string, 0, len(args)+3)
	full = append(full, args...)
	full = append(full, "--db", w.dbPath, "--json")
	code := Run(full, opt)
	return commandResult{code: code, stdout: stdout.Bytes(), stderr: stderr.Bytes()}
}

// decode reads a successful document into dst, or turns the error document
// into a studio error.
func (r commandResult) decode(dst any) error {
	if r.code != 0 {
		return r.err()
	}
	if err := json.Unmarshal(r.stdout, dst); err != nil {
		return &studio.Error{Code: "internal_error", Message: fmt.Sprintf("decode command output: %v", err)}
	}
	return nil
}

// err converts the error document a failed command wrote to stderr.
func (r commandResult) err() error {
	var doc struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Action string `json:"action"`
	}
	if json.Unmarshal(r.stderr, &doc) != nil || doc.Error.Code == "" {
		message := strings.TrimSpace(string(r.stderr))
		if message == "" {
			message = fmt.Sprintf("command exited with %d", r.code)
		}
		return &studio.Error{Code: "internal_error", Message: message}
	}
	return withAction(&studio.Error{Code: doc.Error.Code, Message: doc.Error.Message, Action: doc.Action})
}

// convertJSON copies a CLI protocol document into the studio type with the
// same JSON field names.
func convertJSON(src, dst any) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

// nodePath resolves the Node executable the hosts would spawn.
func (w *Workspace) nodePath() (string, error) {
	node := strings.TrimSpace(w.getenv(tutor.NodeEnv))
	if node == "" {
		node = "node"
	}
	return exec.LookPath(node)
}

// requireNode refuses a host call up front when Node cannot be found, so the
// window shows node_unavailable instead of a recorded host failure.
func (w *Workspace) requireNode(host bool) error {
	if !host {
		return nil
	}
	if _, err := w.nodePath(); err != nil {
		return withAction(&studio.Error{Code: "node_unavailable", Message: fmt.Sprintf("Node.js was not found: %v", err)})
	}
	return nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
