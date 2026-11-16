//go:build !linux

package desktop

import "github.com/egoist/mygo"

// layOutHidden does nothing here: WebView2 and WKWebView lay out the page of
// a window that does not show at the size it was made with.
func layOutHidden(*mygo.Window) error { return nil }
