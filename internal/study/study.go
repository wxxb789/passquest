// Package study is the orchestration seam between the domain packages and the
// two delivery adapters (the CLI and the desktop window). This file is the
// façade adapters import: the type aliases and entry points below. The kernel
// that holds the shared unit-of-work plumbing
// (internal/study/internal/kernel) stays invisible to them.
package study

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"time"

	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
	"github.com/wxxb789/passquest/internal/userdata"
)

// Service is one unit of work over one opened store (one CLI invocation, one
// desktop call).
type Service = kernel.Unit

// Ports are the process seams. A composition root fills them once per process
// kind and then calls Complete.
type Ports = kernel.Ports

// Error is the only error type study code constructs, and it is always a
// refusal (learner-fixable; CLI usage error, exit 2).
type Error = kernel.Error

// Open opens the database the ports select.
func Open(p Ports) (*Service, error) { return kernel.Open(p) }

// Wrap returns a unit of work over a store the caller already opened, for an
// adapter path that holds the call's store and must not open a second one.
func Wrap(st *store.Store, p Ports) *Service { return kernel.Wrap(st, p) }

// Complete fills the port defaults a root did not set.
func Complete(p *Ports) { kernel.Complete(p) }

// DefaultKillGrace is the cancel kill grace every root uses.
func DefaultKillGrace() time.Duration { return kernel.DefaultKillGrace() }

// Refuse builds a refusal: the code and message the CLI prints today.
func Refuse(code, format string, args ...any) *Error {
	return kernel.Refuse(code, format, args...)
}

// ConfigDirUnavailableMessage is the one refusal message both entry points
// render when no per-user data directory can be resolved and no override
// selects a database: it names the two overrides, never a fallback store.
func ConfigDirUnavailableMessage(err error) string {
	return fmt.Sprintf("cannot resolve the per-user data directory: %v; name the database with --db PATH or %s", err, userdata.DatabaseEnv)
}

// DefaultDatabasePath resolves the path a generated command relies on when it
// does not repeat a database: passquest.sqlite in the app-owned per-user data
// directory, with no environment override applied. A copied command with no
// --db, no PASSQUEST_DB and no PASSQUEST_DESKTOP_DATA resolves it, so callers
// omit --db from a generated command only when the path it selected equals this
// one (cmdline.NextDatabaseArgument); a store selected through an override is
// repeated instead, because a copy pasted into an environment with different
// overrides would silently open another store.
func DefaultDatabasePath(configDir func() (string, error)) (string, error) {
	dir, err := userdata.Dir(configDir, func(string) string { return "" })
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, userdata.DatabaseFile), nil
}

// DatabasePath resolves the database a command opens: --db (or the window's
// selected path), then PASSQUEST_DB, then the per-user default. A flag value
// that is only whitespace selects the next source; the selected value is
// returned as given. Both composition roots use it, so the CLI's commands and
// the window's generated commands name the same store.
func DatabasePath(flagValue string, configDir func() (string, error), getenv func(string) string) (string, error) {
	return userdata.DatabasePath(flagValue, configDir, getenv)
}

// NewSessionID is the default session identity both roots fill a nil
// Ports.NewSession with: random bits, with the wall clock only as a fallback.
func NewSessionID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("session-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// Guard refuses command with assessment_active while a run is active (and
// records an elapsed run abandoned). It reads Now only while a run is active.
func Guard(s *Service, command string) error { return kernel.Guard(s, command) }

// GuardPath is the CLI's pre-flag-parse guard: no database file, no guard and
// no file created. Otherwise the store opens through Open, so PrivateDir
// decides OpenPrivate vs Open exactly as the command's own open does.
func GuardPath(p Ports, command string) error { return kernel.GuardPath(p, command) }
