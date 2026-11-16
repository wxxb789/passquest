//go:build linux

package desktop

import (
	"errors"
	"fmt"

	"github.com/ebitengine/purego"
	"github.com/egoist/mygo"
)

// layOutHidden lays out the page of a window made Hidden at the size the
// window would show at, as WebView2 and WKWebView do with no help.
//
// GTK sizes a toplevel's widgets only as it shows it: gtk_window_show
// realizes the window and gtk_window_move_resize allocates it, while a
// resize queued on a toplevel that does not show allocates nothing
// (gtk_window_check_resize). WebKitGTK takes the page's viewport from its
// web view's allocation, so the page of a window that never shows lays out
// at 0x0: innerWidth and innerHeight are 0. MyGo sizes the window only with
// gtk_window_set_default_size — the page area plus the menu bar with
// UseContentSize — and allocates nothing while it is hidden (mygo v0.9.0
// internal/linux/window.go, NewWindow and Show).
//
// This does what showing does short of mapping the window, so nothing
// appears on screen: it realizes the window, then allocates it so that its
// child, the box holding the menu bar and the web view, gets the default
// size, with whatever GTK draws around it (client-side decorations on
// Wayland, nothing on X11) on top, as gtk_window_show sizes it. The page is
// then measured where it lays out: a size other than the one asked for is
// still what innerWidth and innerHeight report.
func layOutHidden(win *mygo.Window) error {
	gtk, err := purego.Dlopen("libgtk-3.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("lay out the hidden window: %w", err)
	}
	var (
		realize          func(widget uintptr)
		getDefaultSize   func(window uintptr, width, height *int32)
		getPreferredSize func(widget uintptr, minimum, natural *[2]int32)
		sizeAllocate     func(widget uintptr, allocation *[4]int32)
		getAllocation    func(widget uintptr, allocation *[4]int32)
		getChild         func(bin uintptr) uintptr
	)
	purego.RegisterLibFunc(&realize, gtk, "gtk_widget_realize")
	purego.RegisterLibFunc(&getDefaultSize, gtk, "gtk_window_get_default_size")
	purego.RegisterLibFunc(&getPreferredSize, gtk, "gtk_widget_get_preferred_size")
	purego.RegisterLibFunc(&sizeAllocate, gtk, "gtk_widget_size_allocate")
	purego.RegisterLibFunc(&getAllocation, gtk, "gtk_widget_get_allocation")
	purego.RegisterLibFunc(&getChild, gtk, "gtk_bin_get_child")
	mygo.RunOnMain(func() {
		window := win.NativeHandle()
		if window == 0 {
			err = errors.New("lay out the hidden window: no GtkWindow")
			return
		}
		realize(window)
		child := getChild(window)
		if child == 0 {
			err = errors.New("lay out the hidden window: the GtkWindow has no child")
			return
		}
		var width, height int32
		getDefaultSize(window, &width, &height)
		allocate := func(w, h int32) (childW, childH int32) {
			// GTK allocates only what it measured first.
			var minimum, natural [2]int32
			getPreferredSize(window, &minimum, &natural)
			sizeAllocate(window, &[4]int32{0, 0, max(w, minimum[0]), max(h, minimum[1])})
			var got [4]int32
			getAllocation(child, &got)
			return got[2], got[3]
		}
		// The first allocation tells what GTK keeps around the child.
		childW, childH := allocate(width, height)
		if childW != width || childH != height {
			allocate(2*width-childW, 2*height-childH)
		}
	})
	return err
}
