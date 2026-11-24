package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
)

// SmokeWindow exists only with EnvScript: a normal launch has no hook, and a
// scripted one binds the service before the window exists.
func TestSmokeWindowComesOnlyFromAScriptedLaunch(t *testing.T) {
	if h, err := smokeFromEnv(envOf(nil), nil); h != nil || err != nil {
		t.Fatalf("no script = %v, %v", h, err)
	}
	var nilHook *smokeHook
	nilHook.bindWindow(nil)

	dir := t.TempDir()
	script := filepath.Join(dir, "scenario.js")
	if err := os.WriteFile(script, []byte("return 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	hook, err := smokeFromEnv(envOf(map[string]string{EnvScript: script, EnvResult: filepath.Join(dir, "result.json")}), nil)
	if err != nil || hook == nil {
		t.Fatalf("smokeFromEnv = %v, %v", hook, err)
	}
	// The name was still free after the nil hook above, so this call must not
	// panic; the probe below must, which is what proves this one bound it.
	hook.bindWindow(func() *mygo.Window { return nil })
	defer func() {
		if recover() == nil {
			t.Fatal("SmokeWindow was still free after a scripted launch bound it")
		}
	}()
	mygo.BindAs("SmokeWindow", &windowService{window: func() *mygo.Window { return nil }})
}

// A call before the window exists refuses by name instead of panicking.
func TestWindowServiceWithoutAWindow(t *testing.T) {
	service := &windowService{window: func() *mygo.Window { return nil }}
	if _, err := service.Title(); err == nil || !strings.Contains(err.Error(), "window does not exist") {
		t.Fatalf("Title without a window = %v, want the window error", err)
	}
}
