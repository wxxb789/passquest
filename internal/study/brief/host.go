package brief

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wxxb789/passquest/internal/agent"
)

// RunHost runs one durable-host process with the interrupt handling both
// surfaces share: a signal cancels the wait instead of killing the
// application. The runner is passed explicitly because the CLI's `agent
// transcript` command reaches the same host without opening the application
// database. The signal.Notify stays inside the wait, exactly where the CLI
// kept it: lifting it to the adapters would be behaviour-visible on the
// desktop.
func RunHost(runner agent.Runner, request agent.HostRequest, timeout time.Duration) (agent.RunOutcome, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		select {
		case <-signals:
			cancel()
		case <-ctx.Done():
		}
	}()
	return runner.Run(ctx, request)
}

// heartbeatWindow is the liveness window for the host heartbeat file and the
// grace period for a start/resume claim whose host has not written one yet.
func heartbeatWindow() time.Duration {
	return time.Duration(agent.HeartbeatStaleMS) * time.Millisecond
}
