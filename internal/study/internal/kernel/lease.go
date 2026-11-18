package kernel

import (
	"time"

	"github.com/wxxb789/passquest/internal/store"
)

// ReserveFunc claims one slot of a provider's UTC-day budget:
// (*store.Store).ReserveTutorCall or (*store.Store).ReserveNoteRefinement. The
// lease does not choose which one.
type ReserveFunc func(st *store.Store, providerID string, since, at time.Time, limit int) (int64, int, error)

// Lease is one claimed budget slot: the reservation an entry point holds from
// Reserve until it either hands the request to the host (Sending) or releases
// the slot again. A slot that was never sent is released on Close; a slot that
// was sent stays claimed unless the caller releases it explicitly, so a crash
// can under-spend but never over-spend the day's budget.
type Lease struct {
	st      *store.Store
	id      int64
	sending bool
}

// Reserve claims one slot of the provider's budget for the UTC day that now
// falls in. A spent day returns a lease with ID 0 and the number of slots
// already used.
func Reserve(u *Unit, reserve ReserveFunc, providerID string, now time.Time, daily int) (*Lease, int, error) {
	id, used, err := reserve(Store(u), providerID, DayStart(now), now, daily)
	if err != nil {
		return nil, 0, err
	}
	return &Lease{st: Store(u), id: id}, used, nil
}

// Sending marks the slot as claimed by a request handed to the host: from here
// on Close does not release it.
func (l *Lease) Sending() { l.sending = true }

// Close releases the slot iff Sending was not called. Always deferred.
func (l *Lease) Close() {
	if l.sending {
		return
	}
	l.Release()
}

// Release returns the slot to the budget: the request never counted, or a
// probe the caller verified and releases explicitly. Releasing twice is a
// no-op.
func (l *Lease) Release() {
	if l.id == 0 {
		return
	}
	_ = l.st.ReleaseTutorCallReservation(l.id)
	l.id = 0
}

// ID is the reservation row id, 0 when the day's budget is spent.
func (l *Lease) ID() int64 { return l.id }
