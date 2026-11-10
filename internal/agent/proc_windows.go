//go:build windows

package agent

import (
	"os"
	"os/exec"
)

// configureProcess has no process-group equivalent on Windows.
func configureProcess(cmd *exec.Cmd) {}

// terminate and cancel both use Kill because Windows does not deliver POSIX
// signals to a child process. The application cancellation event is the
// authority, and resume refuses after it.
func terminate(process *os.Process) error   { return process.Kill() }
func cancel(process *os.Process) error      { return process.Kill() }
func killProcess(process *os.Process) error { return process.Kill() }
