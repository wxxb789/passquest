// Package study is the orchestration seam between the domain packages and the
// two delivery adapters (the CLI and the desktop window). This file is the
// façade adapters import: the type aliases and entry points below. The kernel
// that holds the shared unit-of-work plumbing
// (internal/study/internal/kernel) stays invisible to them.
package study

import (
	"time"

	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
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

// Guard refuses command with assessment_active while a run is active (and
// records an elapsed run abandoned). It reads Now only while a run is active.
func Guard(s *Service, command string) error { return kernel.Guard(s, command) }

// GuardPath is the CLI's pre-flag-parse guard: no database file, no guard and
// no file created. Otherwise the store opens through Open, so PrivateDir
// decides OpenPrivate vs Open exactly as the command's own open does.
func GuardPath(p Ports, command string) error { return kernel.GuardPath(p, command) }
