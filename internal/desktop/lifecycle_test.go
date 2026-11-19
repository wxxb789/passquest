package desktop

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo"
)

// dataLayout is one target system's shape: where the per-user config root is,
// where the app's own files live (a per-user install on Windows, an app
// bundle on macOS, an AppImage extraction or system prefix on Linux), and
// what the app's per-user data directory must therefore be. The names follow
// each system's real layout; the roots are the test's own directories, so the
// same test runs on every host.
type dataLayout struct {
	name       string
	goos       string
	configRoot string
	exeDir     string
	resources  string
	userData   string
}

// dataLayouts builds the three layouts under one temporary root. userData is
// what MyGo's App.Path(PathUserData) returns: os.UserConfigDir() joined with
// the app name (MyGo paths.go).
func dataLayouts(t *testing.T) []dataLayout {
	t.Helper()
	base := t.TempDir()
	join := func(parts ...string) string { return filepath.Join(append([]string{base}, parts...)...) }
	// MyGo's PathUserData: os.UserConfigDir() + App.Name().
	userDataOf := func(configRoot string) string { return filepath.Join(configRoot, AppName) }
	windowsRoot := join("Users", "learner", "AppData", "Roaming")
	darwinRoot := join("Users", "learner", "Library", "Application Support")
	linuxRoot := join("home", "learner", ".config")
	darwinApp := join("Users", "learner", "Applications", AppName+".app")
	return []dataLayout{
		{
			name:       "windows",
			goos:       "windows",
			configRoot: windowsRoot,
			exeDir:     join("Users", "learner", "AppData", "Local", "Programs", AppName),
			resources:  join("Users", "learner", "AppData", "Local", "Programs", AppName),
			userData:   userDataOf(windowsRoot),
		},
		{
			name:       "darwin",
			goos:       "darwin",
			configRoot: darwinRoot,
			exeDir:     filepath.Join(darwinApp, "Contents", "MacOS"),
			resources:  filepath.Join(darwinApp, "Contents", "Resources"),
			userData:   userDataOf(darwinRoot),
		},
		{
			name:       "linux",
			goos:       "linux",
			configRoot: linuxRoot,
			exeDir:     join("tmp", ".mount_PassQuest", "usr", "bin"),
			resources:  join("tmp", ".mount_PassQuest", "usr", "bin"),
			userData:   userDataOf(linuxRoot),
		},
	}
}

// under reports whether path is root or inside it, as the product must never
// find a data directory.
func under(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// TestDataDirectoryIsPerUserOnEveryLayout drives resolveStorage with each
// target system's inputs and pins the outcome: MyGo's per-user directory
// (os.UserConfigDir() joined with the app name), never a path under the
// executable or its packaged resources.
func TestDataDirectoryIsPerUserOnEveryLayout(t *testing.T) {
	for _, layout := range dataLayouts(t) {
		t.Run(layout.name, func(t *testing.T) {
			install := installDirs{
				Executable: func() (string, error) { return filepath.Join(layout.exeDir, "Pass Quest.exe"), nil },
				Resources:  func() (string, error) { return layout.resources, nil },
			}
			where, err := resolveStorage(envOf(nil), func() (string, error) { return layout.userData, nil }, install)
			if err != nil {
				t.Fatalf("default storage: %v", err)
			}
			if where.DataDir != layout.userData {
				t.Fatalf("data directory = %s, want the per-user %s", where.DataDir, layout.userData)
			}
			if where.Database != filepath.Join(layout.userData, DatabaseFile) {
				t.Fatalf("database = %s, want it inside %s", where.Database, where.DataDir)
			}
			if under(where.DataDir, layout.exeDir) {
				t.Fatalf("data directory %s is under the executable's %s", where.DataDir, layout.exeDir)
			}
			if under(where.DataDir, layout.resources) {
				t.Fatalf("data directory %s is under the resources directory %s", where.DataDir, layout.resources)
			}
			if info, err := os.Stat(layout.userData); err != nil || !info.IsDir() {
				t.Fatalf("per-user data directory not created: %v", err)
			}
		})
	}
}

// TestDataDirectoryRefusesInstallLocations pins the guard: a default that
// resolves inside the app's own files is refused before it is created, and
// the explicit EnvData override remains the learner's own choice.
func TestDataDirectoryRefusesInstallLocations(t *testing.T) {
	for _, layout := range dataLayouts(t) {
		t.Run(layout.name, func(t *testing.T) {
			install := installDirs{
				Executable: func() (string, error) { return filepath.Join(layout.exeDir, "Pass Quest.exe"), nil },
				Resources:  func() (string, error) { return layout.resources, nil },
				Home:       func() (string, error) { return filepath.Dir(filepath.Dir(layout.configRoot)), nil },
			}
			for _, root := range []string{layout.exeDir, layout.resources} {
				inside := filepath.Join(root, "learner-data")
				if _, err := resolveStorage(envOf(nil), func() (string, error) { return inside, nil }, install); err == nil {
					t.Fatalf("a default data directory under %s was accepted", root)
				}
				if _, err := os.Stat(inside); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("the refused data directory %s was created (%v)", inside, err)
				}
			}
			override := filepath.Join(layout.exeDir, "learner-data")
			where, err := resolveStorage(envOf(map[string]string{EnvData: override}), func() (string, error) {
				t.Fatal("user data directory consulted despite the override")
				return "", nil
			}, install)
			if err != nil {
				t.Fatalf("explicit %s override: %v", EnvData, err)
			}
			if where.DataDir != override {
				t.Fatalf("override data directory = %s, want %s", where.DataDir, override)
			}
		})
	}
}

// TestDataDirectoryAcceptsLooseBinaryAboveHome proves the install guard does
// not mistake a loose binary for an install: an executable placed in the
// learner's home directory, or in one of its ancestors such as a drive root,
// still opens the per-user data directory below it.
func TestDataDirectoryAcceptsLooseBinaryAboveHome(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home", "learner")
	userData := filepath.Join(home, ".config", AppName)
	for _, exeDir := range []string{home, filepath.Dir(home), base} {
		install := installDirs{
			Executable: func() (string, error) { return filepath.Join(exeDir, "passquest-desktop"), nil },
			Resources:  func() (string, error) { return exeDir, nil },
			Home:       func() (string, error) { return home, nil },
		}
		where, err := resolveStorage(envOf(nil), func() (string, error) { return userData, nil }, install)
		if err != nil {
			t.Fatalf("a binary in %s refused the per-user data directory: %v", exeDir, err)
		}
		if where.DataDir != userData {
			t.Fatalf("data directory = %s, want %s", where.DataDir, userData)
		}
	}
}

// fakeApp records what a refused launch does: the native message it shows,
// the loop and quit it uses, and any window it must not start.
type fakeApp struct {
	messages []string
	loops    int
	quits    int
	windows  int
}

func (f *fakeApp) deps(getenv func(string) string) appDeps {
	return appDeps{
		getenv:  getenv,
		webview: func() bool { return false },
		loop: func(ready func()) error {
			f.loops++
			ready()
			return nil
		},
		quit:    func() { f.quits++ },
		message: func(title, body string) { f.messages = append(f.messages, title+"\x00"+body) },
		window: func(mygo.WindowOptions) *mygo.Window {
			f.windows++
			return nil
		},
	}
}

// TestStartRefusesWithoutWebView pins the first-run branch: no webview
// runtime means exactly one native message box naming what to install, no
// window, and a non-zero exit.
func TestStartRefusesWithoutWebView(t *testing.T) {
	app := &fakeApp{}
	code := start(app.deps(envOf(nil)))
	if code == 0 {
		t.Fatal("a launch without a webview exited zero")
	}
	if len(app.messages) != 1 {
		t.Fatalf("%d message boxes, want exactly one", len(app.messages))
	}
	title, body := missingWebViewMessage(runtime.GOOS)
	if app.messages[0] != title+"\x00"+body {
		t.Fatalf("message = %q, want %q", app.messages[0], title+"\x00"+body)
	}
	if app.loops != 1 || app.quits != 1 {
		t.Fatalf("loop = %d, quit = %d; want the loop started once and quit after the message", app.loops, app.quits)
	}
	if app.windows != 0 {
		t.Fatalf("%d windows started without a webview", app.windows)
	}
}

// TestStartWithoutWebViewWritesSmokeResult pins the scripted launch: it must
// not block on a dialog, it writes the failure to the result file instead,
// and it starts neither the loop nor a window.
func TestStartWithoutWebViewWritesSmokeResult(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "script.js")
	if err := os.WriteFile(script, []byte("return 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(dir, "result.json")
	app := &fakeApp{}
	code := start(app.deps(envOf(map[string]string{
		EnvScript: script,
		EnvResult: result,
	})))
	if code == 0 {
		t.Fatal("a scripted launch without a webview exited zero")
	}
	raw, err := os.ReadFile(result)
	if err != nil {
		t.Fatalf("smoke result: %v", err)
	}
	var doc struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("smoke result %q is not JSON: %v", raw, err)
	}
	title, body := missingWebViewMessage(runtime.GOOS)
	if doc.OK || doc.Error == "" {
		t.Fatalf("smoke result = %s, want ok:false with the failure", raw)
	}
	if !strings.Contains(doc.Error, title) || !strings.Contains(doc.Error, body) {
		t.Fatalf("smoke result error = %q, want the missing-runtime message", doc.Error)
	}
	if app.messages != nil || app.loops != 0 || app.windows != 0 {
		t.Fatalf("a scripted launch without a webview showed a dialog (messages %v, loops %d, windows %d)", app.messages, app.loops, app.windows)
	}
}

// TestMissingWebViewMessageNamesEachRuntime pins the message a learner needs
// to act on: the runtime of each platform and where to get it.
func TestMissingWebViewMessageNamesEachRuntime(t *testing.T) {
	wants := map[string][]string{
		"windows": {"Microsoft Edge WebView2 Runtime", "https://developer.microsoft.com/microsoft-edge/webview2/"},
		"darwin":  {"WKWebView", "macOS"},
		"linux":   {"GTK 3", "WebKitGTK 4.1", "libwebkit2gtk-4.1-0"},
	}
	for goos, parts := range wants {
		title, body := missingWebViewMessage(goos)
		if !strings.Contains(title, AppName) {
			t.Errorf("%s title = %q, want the app name", goos, title)
		}
		for _, part := range parts {
			if !strings.Contains(body, part) {
				t.Errorf("%s message = %q, want it to name %q", goos, body, part)
			}
		}
	}
}
