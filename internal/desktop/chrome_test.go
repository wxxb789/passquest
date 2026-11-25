package desktop

import (
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo"
)

// The page's theme name maps to MyGo's ThemeSource; anything else is refused
// before the window is touched.
func TestButtonsTheme(t *testing.T) {
	cases := []struct {
		theme string
		want  mygo.ThemeSource
		ok    bool
	}{
		{"light", mygo.ThemeLight, true},
		{"dark", mygo.ThemeDark, true},
		{"system", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, err := buttonsTheme(c.theme)
		if c.ok && (err != nil || got != c.want) {
			t.Fatalf("buttonsTheme(%q) = %q, %v, want %q", c.theme, got, err, c.want)
		}
		if !c.ok && err == nil {
			t.Fatalf("buttonsTheme(%q) = %q, want an error", c.theme, got)
		}
	}
}

// A call before the window exists refuses by name instead of panicking.
func TestSetWindowButtonsThemeWithoutAWindow(t *testing.T) {
	study := newStudy(func(BriefEvent) {})
	study.window = func() *mygo.Window { return nil }
	if err := study.SetWindowButtonsTheme("dark"); err == nil || !strings.Contains(err.Error(), "window does not exist") {
		t.Fatalf("SetWindowButtonsTheme without a window = %v, want the window error", err)
	}
}

// The title bar is the page's own only on Windows (goal 0022); every other OS
// keeps the system's.
func TestApplyTitleBarPerOS(t *testing.T) {
	options := mygo.WindowOptions{}
	applyTitleBar(&options)
	if runtime.GOOS == "windows" {
		if options.TitleBarStyle != mygo.TitleBarHidden || options.TitleBarHeight != 40 {
			t.Fatalf("Windows title bar = %q, %d, want hidden with height 40", options.TitleBarStyle, options.TitleBarHeight)
		}
		return
	}
	if options.TitleBarStyle != "" || options.TitleBarHeight != 0 {
		t.Fatalf("%s title bar = %q, %d, want the system's", runtime.GOOS, options.TitleBarStyle, options.TitleBarHeight)
	}
}
