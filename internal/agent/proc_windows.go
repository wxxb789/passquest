//go:build windows

package agent

import (
	"errors"
	"os"
	"os/exec"
)

// errSignalUnsupported makes the Windows signal path an explicit no-op. The
// host observes HostControlPause/HostControlCancel in the file beside its
// heartbeat, so termination never depends on signal delivery.
var errSignalUnsupported = errors.New("agent: POSIX signals are unavailable on Windows")

// configureProcess has no process-group equivalent on Windows.
func configureProcess(cmd *exec.Cmd) {}

// terminate asks the host to pause. Windows cannot deliver SIGTERM, so the
// durable control file written before this call is the request; HostRunner.Run
// kills the process if the host does not stop within its grace period.
func terminate(*os.Process) error { return errSignalUnsupported }

// cancel asks the host to abort durably. The control file is authoritative;
// agent cancel falls back to killProcess if the host does not stop.
func cancel(*os.Process) error { return errSignalUnsupported }

// killProcess is the bounded fallback when a graceful stop does not finish.
func killProcess(process *os.Process) error { return process.Kill() }
