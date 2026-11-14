package hostproc

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestMain doubles as a long-running child: a host that never exits on its own.
func TestMain(m *testing.M) {
	if os.Getenv("HOSTPROC_TEST_CHILD") == "1" {
		time.Sleep(5 * time.Minute)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestEndAllEndsTrackedHostsAndShutdownRefusesNewOnes(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "HOSTPROC_TEST_CHILD=1")
	done := make(chan error, 1)
	go func() { done <- Run(cmd) }()
	for deadline := time.Now().Add(10 * time.Second); Running() == 0; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("child never tracked")
		}
	}
	EndAll(time.Second)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("child survived EndAll")
	}
	if n := Running(); n != 0 {
		t.Fatalf("%d child(ren) still tracked", n)
	}

	Shutdown(time.Second)
	t.Cleanup(func() {
		mu.Lock()
		shutdown = false
		mu.Unlock()
	})
	late := exec.Command(os.Args[0])
	late.Env = append(os.Environ(), "HOSTPROC_TEST_CHILD=1")
	if err := Run(late); err != ErrShuttingDown {
		t.Fatalf("Run after Shutdown = %v", err)
	}
	if late.Process != nil {
		t.Fatal("a host started after Shutdown")
	}
}
