package desktop

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
)

// EnvContentSize sizes the page area of a smoke launch and bypasses the
// remembered window state; without the smoke hook, or unset, the window keeps
// its defaults.
func TestSmokeContentSize(t *testing.T) {
	dir := t.TempDir()
	env := func(size string) func(string) string {
		return envOf(map[string]string{
			EnvScript:      filepath.Join(dir, "scenario.js"),
			EnvResult:      filepath.Join(dir, "result.json"),
			EnvContentSize: size,
		})
	}
	defaults := func() mygo.WindowOptions {
		return mygo.WindowOptions{Width: 1280, Height: 820, StateKey: "main"}
	}

	h, err := smokeFromEnv(env(" 1440x844 "), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := defaults()
	h.sizeWindow(&got)
	if got.Width != 1440 || got.Height != 844 || !got.UseContentSize || got.StateKey != "" {
		t.Fatalf("1440x844 gave %dx%d content=%v state=%q", got.Width, got.Height, got.UseContentSize, got.StateKey)
	}

	h, err = smokeFromEnv(env(""), nil)
	if err != nil {
		t.Fatal(err)
	}
	got = defaults()
	h.sizeWindow(&got)
	if !sameSize(got, defaults()) {
		t.Fatalf("an unset size changed the window: %+v", got)
	}

	// Not a smoke launch: the variable is not read at all.
	h, err = smokeFromEnv(envOf(map[string]string{EnvContentSize: "1x1"}), nil)
	if h != nil || err != nil {
		t.Fatalf("content size without a script = %v, %v", h, err)
	}
	got = defaults()
	h.sizeWindow(&got)
	if !sameSize(got, defaults()) {
		t.Fatalf("a nil hook changed the window: %+v", got)
	}

	for _, bad := range []string{"1280", "0x800", "1280x0", "1280X800", "1280x800x2", "-1x800", "wide", "1280 x 800", "123456x800"} {
		if h, err := smokeFromEnv(env(bad), nil); err == nil || h != nil || !strings.Contains(err.Error(), EnvContentSize) {
			t.Errorf("%q = %v, %v; want an error naming %s", bad, h, err, EnvContentSize)
		}
	}
}

func sameSize(a, b mygo.WindowOptions) bool {
	return a.Width == b.Width && a.Height == b.Height && a.UseContentSize == b.UseContentSize && a.StateKey == b.StateKey
}
