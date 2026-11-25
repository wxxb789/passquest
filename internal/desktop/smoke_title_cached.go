//go:build !windows

package desktop

import "github.com/egoist/mygo"

// windowTitle returns the title MyGo cached and pushed to the platform
// (mygo@v0.10.0/window.go:1143): reading [NSWindow title] or
// gtk_window_get_title back needs a cgo/Objective-C binding this project does
// not carry. Disclosed limit: docs/harness.md row 24, carried into P5.
func windowTitle(win *mygo.Window) (string, error) { return win.Title(), nil }
