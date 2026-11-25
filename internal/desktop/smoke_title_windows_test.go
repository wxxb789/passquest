//go:build windows

package desktop

import (
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procCreateWindowExW = user32.NewProc("CreateWindowExW")
	procSetWindowTextW  = user32.NewProc("SetWindowTextW")
	procDestroyWindow   = user32.NewProc("DestroyWindow")
)

// readWindowText must read the title the OS holds: the only writer on the
// window below is SetWindowTextW, which never touches MyGo's cached title
// (mygo@v0.10.0/window.go:1143).
func TestReadWindowTextReadsTheOSTitle(t *testing.T) {
	// This test runs no message pump, so the reads must stay on the window's own thread:
	// GetWindowTextW sends WM_GETTEXT, and a cross-thread send blocks until that thread pumps.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	class := utf16Of(t, "STATIC")
	hwnd, _, callErr := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(class)), 0, 0, 0, 0, 80, 24, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Fatalf("CreateWindowExW: %v", callErr)
	}
	defer procDestroyWindow.Call(hwnd)

	if got, err := readWindowText(hwnd); err != nil || got != "" {
		t.Fatalf("readWindowText on a title-less window = %q, %v, want an empty title", got, err)
	}
	for _, want := range []string{"passquest smoke probe", "Pass Quest — 今日"} {
		text := utf16Of(t, want)
		if _, _, callErr := procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(text))); callErrno(callErr) != nil {
			t.Fatalf("SetWindowTextW(%q): %v", want, callErr)
		}
		if got, err := readWindowText(hwnd); err != nil || got != want {
			t.Fatalf("readWindowText after SetWindowTextW(%q) = %q, %v", want, got, err)
		}
	}
	if _, err := readWindowText(1); err == nil {
		t.Fatal("readWindowText on an invalid handle read no error")
	}
}

func utf16Of(t *testing.T, s string) *uint16 {
	t.Helper()
	text, err := windows.UTF16PtrFromString(s)
	if err != nil {
		t.Fatal(err)
	}
	return text
}
