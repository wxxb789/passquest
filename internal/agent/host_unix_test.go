//go:build unix

package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/hostproc"
)

// A durable host started through HostRunner.Run, and everything it started,
// must be gone once hostproc ends the hosts a quitting window left running,
// even when they ignore the polite SIGTERM.
func TestHostRunnerHostAndItsChildrenEndWithHostproc(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pids")
	node := filepath.Join(dir, "node")
	script := "#!/bin/sh\ntrap '' TERM\nsleep 300 &\necho \"$$ $!\" > \"$PIDFILE.tmp\"\nmv \"$PIDFILE.tmp\" \"$PIDFILE\"\nwait\n"
	if err := os.WriteFile(node, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	host := filepath.Join(dir, "durable-host.mjs")
	if err := os.WriteFile(host, []byte("// stub\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{NodeEnv: node, HostEnv: host}
	runner := HostRunner{
		Getenv:  func(key string) string { return env[key] },
		Environ: func() []string { return append(os.Environ(), "PIDFILE="+pidFile) },
	}
	request := HostRequest{ProtocolVersion: ProtocolVersion, Mode: ModeStatus,
		RunID: "run-20261110T100000Z-aaaaaaaa", StorePath: filepath.Join(dir, "store"), CredentialEnv: CredentialEnv}
	done := make(chan error, 1)
	go func() {
		_, err := runner.Run(context.Background(), request)
		done <- err
	}()

	var pids []int
	for deadline := time.Now().Add(10 * time.Second); len(pids) < 2; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("stub host never reported its pids")
		}
		raw, err := os.ReadFile(pidFile)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range strings.Fields(string(raw)) {
			pid, err := strconv.Atoi(field)
			if err != nil {
				t.Fatal(err)
			}
			pids = append(pids, pid)
		}
	}
	if hostproc.Running() == 0 {
		t.Fatal("the host is not tracked")
	}

	hostproc.EndAll(300 * time.Millisecond)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("HostRunner.Run did not return after the host was ended")
	}
	for _, pid := range pids {
		for deadline := time.Now().Add(5 * time.Second); processAlive(pid); time.Sleep(20 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("process %d survived hostproc.EndAll", pid)
			}
		}
	}
	if n := hostproc.Running(); n != 0 {
		t.Fatalf("%d host(s) still tracked", n)
	}
}

// processAlive treats a zombie waiting for its new parent to reap it as gone.
func processAlive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return true
	}
	fields := strings.Fields(string(stat[strings.LastIndexByte(string(stat), ')')+1:]))
	return len(fields) == 0 || fields[0] != "Z"
}
