//go:build !unix

package hostproc

import "os/exec"

// Without process groups or POSIX signals a host is killed directly; on
// Windows the desktop's kill-on-close job object also covers anything a host
// started.
func leadsGroup(*exec.Cmd) bool { return false }
func stop(e entry)              { _ = e.process.Kill() }
func kill(e entry)              { _ = e.process.Kill() }
func groupAlive(entry) bool     { return false }
