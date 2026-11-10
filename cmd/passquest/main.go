// Command passquest is the local, scriptable access surface for Pass Quest.
package main

import (
	"os"

	"github.com/wxxb789/passquest/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.Options{}))
}
