//go:build unix

package hostproc

import (
	"os/exec"
	"syscall"
)

func leadsGroup(cmd *exec.Cmd) bool {
	return cmd.SysProcAttr != nil && cmd.SysProcAttr.Setpgid
}

// stop asks a host to finish: SIGTERM to its whole group when it leads one,
// else to the process.
func stop(e entry) { signal(e, syscall.SIGTERM) }

// kill is the bounded fallback.
func kill(e entry) { signal(e, syscall.SIGKILL) }

func signal(e entry, sig syscall.Signal) {
	if e.group {
		_ = syscall.Kill(-e.process.Pid, sig)
		return
	}
	_ = e.process.Signal(sig)
}

func groupAlive(e entry) bool {
	return syscall.Kill(-e.process.Pid, 0) == nil
}
