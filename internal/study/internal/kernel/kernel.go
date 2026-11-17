// Package kernel holds the unit-of-work and port types every study module
// shares. Adapters reach it only through the internal/study façade's type
// aliases, so the store accessor stays invisible to them (Go's internal rule).
package kernel

import "github.com/wxxb789/passquest/internal/store"

// Unit is one unit of work over one opened store (one CLI invocation, one
// desktop call). Its fields are unexported: domain packages read them through
// the functions below, adapters cannot.
type Unit struct {
	st *store.Store
	p  Ports
}

// Open opens the database the ports select: store.OpenPrivate for the
// application's own default state directory, store.Open for a user-chosen
// path.
func Open(p Ports) (*Unit, error) {
	var (
		st  *store.Store
		err error
	)
	if p.PrivateDir {
		st, err = store.OpenPrivate(p.DBPath)
	} else {
		st, err = store.Open(p.DBPath)
	}
	if err != nil {
		return nil, err
	}
	return &Unit{st: st, p: p}, nil
}

// Wrap returns a unit of work over a store the caller already opened, for the
// desktop paths that hold the call's store (withStore) and must not open a
// second one: a second open adds a migration and a failure path the call does
// not have.
func Wrap(st *store.Store, p Ports) *Unit { return &Unit{st: st, p: p} }

// Close releases the store.
func (u *Unit) Close() error { return u.st.Close() }

// Path is the opened database's absolute path, for the CLI's human output.
func (u *Unit) Path() string { return u.st.Path() }

// Store reads the unit's store. It is a function, not a method, so the store
// accessor stays invisible through the façade's type alias.
func Store(u *Unit) *store.Store { return u.st }

// P reads the ports the unit was opened with.
func P(u *Unit) Ports { return u.p }
