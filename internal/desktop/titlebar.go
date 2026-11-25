package desktop

import (
	"runtime"

	"github.com/egoist/mygo"
)

// The page draws the title area itself on every OS this goal covers, and the
// window hides the title bar it would draw: on Linux a compositor that
// decorates windows itself keeps its own instead (Q-0022-04). The 40 px strip
// is the util group's measured height (Q-0022-01) and sizes the GTK title
// bars; macOS reads AppKit's own band and ignores TitleBarHeight (Q-0022-03).
func applyTitleBar(o *mygo.WindowOptions) {
	switch runtime.GOOS {
	case "windows", "linux":
		o.TitleBarStyle = mygo.TitleBarHidden
		o.TitleBarHeight = 40
	case "darwin":
		o.TitleBarStyle = mygo.TitleBarHidden
	}
}
