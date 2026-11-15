// Command passquest-desktop is the Pass Quest window. With arguments it is the
// CLI (same commands and exit codes), so the durable host can call
// `agent record-step` on this binary; without, it opens the window.
package main

import (
	"os"

	"github.com/wxxb789/passquest/internal/cli"
	"github.com/wxxb789/passquest/internal/desktop"
)

func main() {
	os.Exit(run(os.Args[1:], desktop.Run))
}

func run(args []string, window func() int) int {
	if len(args) > 0 {
		return cli.Run(args, cli.Options{})
	}
	return window()
}
