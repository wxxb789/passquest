package desktop

import (
	"runtime"

	"github.com/egoist/mygo"
)

// applyTitleBar replaces the system title bar with the page's own title area
// on the OSes whose step has landed (goal 0022): Windows and macOS. The height
// 40 is the util group's measured height and a Windows 11 sized caption strip
// (Q-0022-01); macOS reads AppKit's own band and ignores TitleBarHeight
// (Q-0022-03); Linux (P4) keeps the system's.
func applyTitleBar(o *mygo.WindowOptions) {
	switch runtime.GOOS {
	case "windows":
		o.TitleBarStyle = mygo.TitleBarHidden
		o.TitleBarHeight = 40
	case "darwin":
		o.TitleBarStyle = mygo.TitleBarHidden
	}
}
