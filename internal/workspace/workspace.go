package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wxxb789/passquest/internal/agent"
	"github.com/wxxb789/passquest/internal/anki"
	"github.com/wxxb789/passquest/internal/hostpaths"
	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/search"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/studio"
	"github.com/wxxb789/passquest/internal/study"
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

// Options are the process seams of one desktop workspace. They are
// the CLI's Options seams plus the database path and a credential store.
type Options struct {
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
	// Resources is the packaged resources directory (mygo.PathResources in
	// the desktop): the bundled hosts and Node runtime are looked up there
	// first. Empty in a source checkout.
	Resources  string
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
	resources      string
	baseGetenv     func(string) string
	baseEnviron    func() []string
	credentials    CredentialStore
	credentialEnvs map[string]string
	newToken       func() string
	opt            windowOptions
	// ports is the completed process seam set; every direct call from C3b on
	// opens its service through it.
	ports study.Ports
	// hostTutor, hostAnki and hostAgent are true when the default Node host
	// transports are in use, so a missing Node is reported before the call.
	hostTutor, hostAnki, hostAgent bool
	// refining holds the drafts with a refinement in flight: one paid request per
	// draft at a time, whatever the window's own state (a reload forgets it).
	refining   map[int64]bool
	refiningMu sync.Mutex
}

// windowOptions are the process seams the window's methods read directly: the
// clock every document is stamped with and the credential-layered
// environment. The transports and identity seams live in ports.
type windowOptions struct {
	Now    func() time.Time
	Getenv func(string) string
}

// Open validates the options, migrates the database once and
// returns the workspace.
func Open(options Options) (*Workspace, error) {
	w := &Workspace{
		baseGetenv:  options.Getenv,
		baseEnviron: options.Environ,
		credentials: options.Credentials,
		newToken:    options.NewToken,
		resources:   options.Resources,
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
		w.dbPath = study.DatabasePath("", w.baseGetenv)
	}
	w.opt = windowOptions{Now: options.Now, Getenv: w.getenv}
	if w.opt.Now == nil {
		w.opt.Now = time.Now
	}
	newSession := options.NewSession
	if newSession == nil {
		newSession = study.NewSessionID
	}
	w.hostTutor = options.TutorTransport == nil
	w.hostAnki = options.AnkiTransport == nil
	w.hostAgent = options.AgentRunner == nil
	tutorTransport := options.TutorTransport
	if tutorTransport == nil {
		tutorTransport = tutor.HostTransport{Getenv: w.getenv, Environ: w.environ, Executable: options.Executable, Resources: options.Resources}
	}
	ankiTransport := options.AnkiTransport
	if ankiTransport == nil {
		ankiTransport = anki.HostTransport{Getenv: w.getenv, Environ: w.environ, Executable: options.Executable, Resources: options.Resources}
	}
	searchTransport := options.SearchTransport
	if searchTransport == nil {
		searchTransport = search.HostTransport{Getenv: w.getenv, Environ: w.environ, Executable: options.Executable, Resources: options.Resources}
	}
	agentRunner := options.AgentRunner
	if agentRunner == nil {
		agentRunner = agent.HostRunner{Getenv: w.getenv, Environ: w.environ, Executable: options.Executable, Resources: options.Resources}
	}
	// The desktop always opens a path the window resolved (its own default or
	// the user's --db), never the app-owned private default of a bare CLI
	// invocation, so PrivateDir stays false and Complete fills what
	// cli.withDefaults filled for every call that ran through the CLI: the run
	// id over this clock, the host token and the cancel kill grace.
	// Side-effect exposure rows use the same clock as everything else the
	// window records.
	w.ports = study.Ports{
		DBPath:          w.dbPath,
		PrivateDir:      false,
		Now:             w.opt.Now,
		PresentationNow: w.opt.Now,
		NewSession:      newSession,
		NewToken:        w.newToken,
		NewRunID:        options.NewRunID,
		NewHostToken:    options.NewHostToken,
		Getenv:          w.getenv,
		Tutor:           tutorTransport,
		Anki:            ankiTransport,
		Search:          searchTransport,
		Agent:           agentRunner,
	}
	study.Complete(&w.ports)
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

// withService opens one study service over the workspace ports, runs fn and
// closes it; the error maps exactly like a direct store path.
func (w *Workspace) withService(fn func(*study.Service) error) error {
	svc, err := study.Open(w.ports)
	if err != nil {
		return workspaceError(err)
	}
	defer svc.Close()
	return workspaceError(fn(svc))
}

// locked runs fn with a service opened for this call while holding the write
// lock: the short local writes that need no host round trip.
func (w *Workspace) locked(fn func(*study.Service) error) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.withService(fn)
}

// workspaceError converts an orchestration error into a typed studio error
// carrying the same code the CLI would print. A study refusal keeps its code;
// a classified study error is matched on its wrapped sentinel with the
// desktop's own table, because the direct practice and assessment paths never
// saw the area code.
func workspaceError(err error) error {
	if err == nil {
		return nil
	}
	var typed *studio.Error
	if errors.As(err, &typed) {
		return withAction(typed)
	}
	var refusal *study.Error
	if errors.As(err, &refusal) {
		if refusal.Err == nil {
			return withAction(&studio.Error{Code: refusal.Code, Message: refusal.Message})
		}
		err = refusal.Err
	}
	if code, ok := desktopSentinelCode(err); ok {
		return &studio.Error{Code: code, Message: err.Error()}
	}
	return &studio.Error{Code: "internal_error", Message: err.Error()}
}

// cliCoded maps an orchestration error to the studio error the window shows
// for a call that ran through the CLI: a study error keeps its code, anything
// else is internal_error with the error's message.
func cliCoded(err error) error {
	if err == nil {
		return nil
	}
	var e *study.Error
	if errors.As(err, &e) {
		return withAction(&studio.Error{Code: e.Code, Message: e.Message})
	}
	return &studio.Error{Code: "internal_error", Message: err.Error()}
}

// desktopSentinelCode is the desktop's own table for the domain sentinels that
// reach it. The ErrNotFound split from the CLI's per-area tables (not_found
// here, invalid_input or unknown_run there) is behaviour.
func desktopSentinelCode(err error) (string, bool) {
	switch {
	case errors.Is(err, source.ErrNotPracticeable):
		return "not_practiceable", true
	case errors.Is(err, source.ErrInvalidInput), errors.Is(err, source.ErrToolMissing):
		return "invalid_input", true
	case errors.Is(err, store.ErrNotFound):
		return "not_found", true
	case errors.Is(err, store.ErrAssessmentRunState):
		return "run_state", true
	case errors.Is(err, store.ErrAssessmentWindowElapsed):
		return "run_abandoned", true
	}
	return "", false
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

// nodePath resolves the Node executable the hosts would spawn: the
// PASSQUEST_NODE override, the runtime packaged in the resources directory,
// or node on PATH. Services and requireNode report on exactly this lookup.
func (w *Workspace) nodePath() (string, error) {
	return exec.LookPath(hostpaths.ResolveNode(hostpaths.Source{Getenv: w.getenv, Resources: w.resources}, runtime.GOOS))
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
