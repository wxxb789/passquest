package kernel

import (
	"time"

	"github.com/wxxb789/passquest/internal/agent"
	"github.com/wxxb789/passquest/internal/anki"
	"github.com/wxxb789/passquest/internal/search"
	"github.com/wxxb789/passquest/internal/tutor"
)

// Ports are the process seams. A composition root fills them once per process
// kind and then calls Complete; Open itself fills no defaults.
type Ports struct {
	DBPath string // resolved: --db, $PASSQUEST_DB or the default
	// PrivateDir opens with store.OpenPrivate (the state directory is
	// tightened to 0700). The CLI sets it iff neither --db nor $PASSQUEST_DB
	// is set; the desktop always opens a user-chosen path.
	PrivateDir      bool
	Now             func() time.Time
	PresentationNow func() time.Time // CLI: separate wall clock; desktop: == Now
	NewSession      func() string
	NewToken        func() string // practice presentation tokens (desktop only)
	NewRunID        func() (string, error)
	NewHostToken    func() (string, error)
	Getenv          func(string) string // desktop: credential-layered already
	Tutor           tutor.Transport
	Anki            anki.Transport
	Search          search.Adapter
	Agent           agent.Runner
	KillGrace       time.Duration // agent cancel; the CLI's Options.agentKillGrace
}

// Complete fills exactly the three defaults the desktop gets today only
// because every shimmed call runs through cli.Options.withDefaults: NewRunID
// over the completed clock, NewHostToken and the cancel kill grace. Nil and
// zero fields only.
func Complete(p *Ports) {
	if p.NewRunID == nil {
		p.NewRunID = func() (string, error) { return agent.NewRunID(p.Now()) }
	}
	if p.NewHostToken == nil {
		p.NewHostToken = agent.NewHostToken
	}
	if p.KillGrace == 0 {
		p.KillGrace = DefaultKillGrace()
	}
}

// DefaultKillGrace is how long cancel waits after a hard kill: one heartbeat
// window plus a second of margin for the last write to age out.
func DefaultKillGrace() time.Duration {
	return time.Duration(agent.HeartbeatStaleMS)*time.Millisecond + time.Second
}
