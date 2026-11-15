// Package hostproc tracks the Node host processes this process starts (tutor,
// Anki, search and durable agent hosts) so the desktop window can end every
// one still running when it quits. On Windows the desktop also puts itself in
// a kill-on-close job object; on Linux and macOS this package is the only
// thing that keeps a host from outliving the window.
package hostproc

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"time"
)

// pollInterval is how often EndAll checks whether the stopped hosts exited.
const pollInterval = 20 * time.Millisecond

type entry struct {
	process *os.Process
	group   bool // the process leads its own process group (unix Setpgid)
}

// ErrShuttingDown is returned by Start after Shutdown: no host may start once
// the window has begun ending the ones it has.
var ErrShuttingDown = errors.New("hostproc: the app is shutting down")

var (
	mu       sync.Mutex
	live     = map[*os.Process]entry{}
	shutdown bool
)

// Start starts cmd and tracks it until release is called, which the caller
// does after cmd.Wait returns.
func Start(cmd *exec.Cmd) (release func(), err error) {
	mu.Lock()
	defer mu.Unlock()
	if shutdown {
		return func() {}, ErrShuttingDown
	}
	if err := cmd.Start(); err != nil {
		return func() {}, err
	}
	e := entry{process: cmd.Process, group: leadsGroup(cmd)}
	live[e.process] = e
	return func() {
		mu.Lock()
		delete(live, e.process)
		mu.Unlock()
	}, nil
}

// Run is cmd.Run with the process tracked while it runs.
func Run(cmd *exec.Cmd) error {
	release, err := Start(cmd)
	if err != nil {
		return err
	}
	defer release()
	return cmd.Wait()
}

// Running reports how many tracked hosts have not been waited for.
func Running() int {
	mu.Lock()
	defer mu.Unlock()
	return len(live)
}

// Shutdown refuses every later Start and ends the tracked hosts (EndAll). The
// desktop calls it once its window has quit.
func Shutdown(grace time.Duration) {
	mu.Lock()
	shutdown = true
	mu.Unlock()
	EndAll(grace)
}

// EndAll ends every tracked host and, for a host that leads its own process
// group, everything it started. It asks politely first (SIGTERM on unix: the
// durable host pauses and keeps its work resumable), waits up to grace for
// the hosts to exit, then kills whatever is left.
func EndAll(grace time.Duration) {
	mu.Lock()
	targets := make([]entry, 0, len(live))
	for _, e := range live {
		targets = append(targets, e)
	}
	mu.Unlock()
	if len(targets) == 0 {
		return
	}
	for _, e := range targets {
		stop(e)
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) && anyAlive(targets) {
		time.Sleep(pollInterval)
	}
	for _, e := range targets {
		if e.group || tracked(e.process) {
			kill(e)
		}
	}
}

func tracked(p *os.Process) bool {
	mu.Lock()
	defer mu.Unlock()
	_, ok := live[p]
	return ok
}

// anyAlive reports whether a target is still tracked (its Wait has not
// returned) or, for a group leader, whether any member of its group remains.
func anyAlive(targets []entry) bool {
	for _, e := range targets {
		if tracked(e.process) || (e.group && groupAlive(e)) {
			return true
		}
	}
	return false
}
