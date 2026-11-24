//go:build windows

package desktop

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
	"golang.org/x/sys/windows"
)

// user32's title calls, the pair MyGo's own Windows backend uses
// (mygo@v0.10.0/internal/windows/window.go:974-980).
var (
	user32                   = windows.NewLazySystemDLL("user32.dll")
	procGetWindowTextW       = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
)

// windowTitle reads the title Windows holds for the window. MyGo's
// Window.Title returns the string MyGo itself cached (mygo@v0.10.0/window.go:1143),
// so the read goes to the OS through the window's HWND.
func windowTitle(win *mygo.Window) (string, error) {
	hwnd := win.NativeHandle()
	if hwnd == 0 {
		return "", fmt.Errorf("the window has no native handle yet")
	}
	return readWindowText(hwnd)
}

// readWindowText is GetWindowTextLengthW to size the buffer, then
// GetWindowTextW; same-process calls run the window's own proc directly.
func readWindowText(hwnd uintptr) (string, error) {
	length, _, callErr := procGetWindowTextLengthW.Call(hwnd)
	if err := callErrno(callErr); err != nil {
		return "", fmt.Errorf("GetWindowTextLengthW: %w", err)
	}
	if length == 0 {
		return "", nil
	}
	buf := make([]uint16, length+1)
	copied, _, callErr := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if err := callErrno(callErr); err != nil {
		return "", fmt.Errorf("GetWindowTextW: %w", err)
	}
	return windows.UTF16ToString(buf[:copied]), nil
}

// callErrno turns a LazyProc call's error — always non-nil, holding
// GetLastError as its errno — into nil when that errno is 0.
func callErrno(callErr error) error {
	if errno, ok := callErr.(syscall.Errno); ok && errno != 0 {
		return errno
	}
	return nil
}
