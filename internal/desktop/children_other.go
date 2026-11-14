//go:build !windows

package desktop

// killChildrenOnExit has no portable equivalent here: on Linux and macOS a
// Node host still running after the bounded quit drain ends at its own
// timeout (the tutor and Anki hosts' request timeouts, the durable host's
// run timeout), and the durable host keeps its work resumable.
func killChildrenOnExit() error { return nil }
