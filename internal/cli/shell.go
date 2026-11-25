package cli

import (
	"fmt"

	"github.com/wxxb789/passquest/internal/cmdline"
)

// commandQuoter is the CLI's quoting handle: the shell family detected for this
// invocation. The quoting rules and the command builders are shared with the
// window (internal/cmdline), so the two adapters cannot drift; this type keeps
// the CLI's document and its tests naming the detected choice once.
type commandQuoter struct {
	shell cmdline.ShellChoice
}

// renderer is the shared quoter the command builders take.
func (q commandQuoter) renderer() cmdline.CommandQuoter {
	return cmdline.CommandQuoter{Shell: q.shell}
}

// quote renders one argument for the detected shell.
func (q commandQuoter) quote(value string) string {
	return q.renderer().Quote(value)
}

// shellViewOf exposes the choice in the protocol document.
func shellViewOf(choice cmdline.ShellChoice) *shellView {
	return &shellView{
		Family:       string(choice.Family),
		DetectedFrom: choice.Source,
		Detail:       choice.Detail,
		Override:     cmdline.ShellOverrideHint,
	}
}

// shellSourceDetail states what decided a family, as next's disclosure and the
// old-default notice print it.
func shellSourceDetail(source, detail string) string {
	switch source {
	case cmdline.ShellOverrideEnv:
		return "from " + cmdline.ShellOverrideEnv
	case "PSModulePath":
		return "from PSModulePath"
	case "SHELL", "ComSpec":
		return fmt.Sprintf("from %s=%s", source, detail)
	}
	return "no shell detected, defaulted"
}
