//go:build unix

package agent

import (
	"os"
	"os/exec"
	"syscall"
)

// configureProcess puts the host in its own process group so a terminal signal
// reaches the CLI, which decides between pause and cancel. The CLI signals the
// host explicitly.
func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminate asks the host to pause: close the durable runtime and leave work
// pending for the next process.
func terminate(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}

// cancel asks the host to abort the conversation durably before closing.
func cancel(process *os.Process) error {
	return process.Signal(syscall.SIGINT)
}

// killProcess is the bounded fallback when a graceful stop does not finish.
func killProcess(process *os.Process) error {
	return process.Kill()
}
