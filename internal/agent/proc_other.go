//go:build !unix && !windows

package agent

import (
	"os"
	"os/exec"
)

func configureProcess(cmd *exec.Cmd)        {}
func terminate(process *os.Process) error   { return process.Kill() }
func cancel(process *os.Process) error      { return process.Kill() }
func killProcess(process *os.Process) error { return process.Kill() }
