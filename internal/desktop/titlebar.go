package desktop

import (
	"runtime"

	"github.com/egoist/mygo"
)

// The page draws the title area itself (goal 0022), so the window hides its
// system title bar. The height 40 is the util group's measured height and a
// Windows 11 sized caption strip (Q-0022-01); macOS reads AppKit's own band and
// ignores TitleBarHeight (Q-0022-03).
func applyTitleBar(o *mygo.WindowOptions) {
	switch runtime.GOOS {
	case "windows":
		o.TitleBarStyle = mygo.TitleBarHidden
		o.TitleBarHeight = 40
	case "darwin":
		o.TitleBarStyle = mygo.TitleBarHidden
	}
}
