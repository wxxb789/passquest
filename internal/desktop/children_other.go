//go:build !windows

package desktop

// killChildrenOnExit has no job-object equivalent here. On Linux and macOS
// hostproc.Shutdown, called when the window's run loop returns, ends every
// Node host the app started and the process group of a durable host.
func killChildrenOnExit() error { return nil }
