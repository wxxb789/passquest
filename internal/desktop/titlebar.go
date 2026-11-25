package desktop

import (
	"runtime"

	"github.com/egoist/mygo"
)

// applyTitleBar replaces the system title bar with the page's own title area
// on the OSes whose step has landed (goal 0022): Windows. The height 40 is the
// util group's measured height and a Windows 11 sized caption strip
// (Q-0022-01); macOS (P3) and Linux (P4) keep the system's.
func applyTitleBar(o *mygo.WindowOptions) {
	if runtime.GOOS == "windows" {
		o.TitleBarStyle = mygo.TitleBarHidden
		o.TitleBarHeight = 40
	}
}
