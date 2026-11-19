// Package desktop is the Pass Quest window: a MyGo app whose page calls the
// bound Study service, which forwards to one workspace.Workspace. It opens no
// network listener: MyGo's single-instance lock is a Unix-domain socket and
// the smoke hook reads and writes files.
package desktop

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/wxxb789/passquest/internal/hostproc"
	"github.com/wxxb789/passquest/internal/workspace"
)

// web is the page: the committed index.html and not-built.html plus the
// gitignored dist/ that `bun run desktop:build` bundles from desktop/web.
//
//go:embed all:web
var web embed.FS

// quitGrace bounds how long a quit waits for calls still running.
const quitGrace = 5 * time.Second

// hostStopGrace bounds how long a host asked to stop at exit may take before
// it is killed.
const hostStopGrace = 3 * time.Second

// BriefChanged is broadcast when a brief resumed in the background starts and
// when its host returns.
var BriefChanged = mygo.NewEvent[BriefEvent]("brief:changed")

// Run runs the window until the app quits and returns the process exit code.
func Run() int {
	return start(appDeps{
		getenv:  os.Getenv,
		webview: mygo.WebViewAvailable,
		loop:    nativeLoop,
		quit:    mygo.App.Quit,
		message: showNativeMessage,
		window:  mygo.NewWindow,
	})
}

// appDeps are the running app's inputs and MyGo touch points. Run binds the
// real ones; tests replace them to reach the missing-webview branch, which
// must say what to install and start no window.
type appDeps struct {
	getenv  func(string) string
	webview func() bool
	loop    func(ready func()) error
	quit    func()
	message func(title, body string)
	window  func(mygo.WindowOptions) *mygo.Window
}

// nativeLoop schedules ready for when the app is ready and runs the native
// event loop until the app quits.
func nativeLoop(ready func()) error {
	mygo.App.WhenReady(ready)
	return mygo.App.Run()
}

// showNativeMessage shows one native message box. MyGo's dialogs need the
// running app but no webview: a task dialog on Windows, a GTK dialog on
// Linux, an alert on macOS.
func showNativeMessage(title, body string) {
	mygo.Dialog.Error(title, body)
}

// start runs one app instance and returns its exit code.
func start(deps appDeps) int {
	getenv := deps.getenv
	frontend, err := frontendFS()
	if err != nil {
		log.Printf("passquest-desktop: %v", err)
		return 1
	}
	mygo.SetFrontend(frontend)
	mygo.App.SetName(appName(getenv))

	generating := getenv("MYGO_GENERATE") != ""
	smoke, err := smokeFromEnv(getenv, deps.quit)
	if err != nil {
		log.Printf("passquest-desktop: %v", err)
		return 2
	}

	// The window shows the page in the system webview; without a runtime
	// the window is blank or the app dies inside it, so refuse before the
	// loop starts. A scripted launch reports through its result file rather
	// than a dialog nobody would dismiss. Generate mode opens no window (it
	// writes the TypeScript client), so it needs no webview.
	if !generating && !deps.webview() {
		return refuseNoWebView(deps, smoke)
	}

	study := newStudy(func(e BriefEvent) { _ = BriefChanged.Broadcast(e) })
	mygo.BindAs("Study", study)

	if !generating {
		if err := killChildrenOnExit(); err != nil {
			log.Printf("passquest-desktop: child processes may outlive the window: %v", err)
		}
		if !mygo.App.RequestSingleInstanceLock() {
			return 0 // the running instance was asked to come to the front
		}
	}

	var win *mygo.Window
	mygo.App.OnSecondInstance(func([]string, string) {
		if win == nil {
			return
		}
		if win.IsMinimized() {
			win.Restore()
		}
		win.Show()
		win.Focus()
	})
	// A quit waits, bounded, for calls still running; hostproc.Shutdown
	// then ends any host they left.
	drained := false
	mygo.App.OnWillQuit(func(e *mygo.QuitEvent) {
		if drained || e.SessionEnding || !study.busy() {
			return
		}
		e.PreventDefault()
		go func() {
			if !study.drain(quitGrace) {
				log.Printf("passquest-desktop: quitting with calls still running after %s", quitGrace)
			}
			mygo.App.WhenReady(func() {
				drained = true
				mygo.App.Quit()
			})
		}()
	})

	mygo.App.WhenReady(func() {
		openStudy(study, getenv)
		options := mygo.WindowOptions{
			URL:       startURL(frontend),
			Title:     AppName,
			Width:     1280,
			Height:    820,
			MinWidth:  880,
			MinHeight: 600,
			Hidden:    strings.TrimSpace(getenv(EnvHidden)) == "1",
			StateKey:  "main",
		}
		if options.Hidden {
			// A hidden window's page runs as a shown one's: web views
			// throttle the timers of a page that can't be seen to once a
			// second (WebKit also suspends its CSS animations), which a
			// scripted run waiting on timers and transitions would measure.
			options.Page.BackgroundThrottling = mygo.ThrottlingOff
		}
		smoke.sizeWindow(&options)
		win = deps.window(options)
		if options.Hidden {
			if err := layOutHidden(win); err != nil {
				log.Printf("passquest-desktop: %v", err)
			}
		}
		if smoke != nil {
			smoke.attach(win.Page())
		}
	})

	runErr := mygo.App.Run()
	// No Node host the window started may outlive it (goal 0011 clause 10):
	// stop the ones a call left running, then kill what remains. This is the
	// only mechanism on Linux and macOS; Windows also has the job object.
	hostproc.Shutdown(hostStopGrace)
	if runErr != nil {
		log.Printf("passquest-desktop: %v", runErr)
		return 1
	}
	if smoke != nil && smoke.writeFailed.Load() {
		return 1
	}
	return 0
}

// refuseNoWebView reports that the machine has no webview runtime and
// returns a non-zero exit code. It starts the loop for exactly one native
// message box, naming what to install, and quits; it creates no window. A
// scripted launch writes the failure to its result file instead, so a smoke
// run never waits on a dialog.
func refuseNoWebView(deps appDeps, smoke *smokeHook) int {
	title, body := missingWebViewMessage(runtime.GOOS)
	if smoke != nil {
		if err := smoke.writeFailure(fmt.Errorf("%s: %s", title, body)); err != nil {
			log.Printf("passquest-desktop: write smoke result: %v", err)
		}
		return 1
	}
	log.Printf("passquest-desktop: %s: %s", title, body)
	if err := deps.loop(func() {
		deps.message(title, body)
		deps.quit()
	}); err != nil {
		log.Printf("passquest-desktop: %v", err)
	}
	return 1
}

// openStudy opens the workspace on the resolved storage. A failure is kept
// by the service and shown by the window as an error state.
func openStudy(study *Study, getenv func(string) string) {
	where, err := resolveStorage(getenv,
		func() (string, error) { return mygo.App.Path(mygo.PathUserData) },
		installDirs{
			Executable: os.Executable,
			Resources:  func() (string, error) { return mygo.App.Path(mygo.PathResources) },
			Home:       os.UserHomeDir,
		})
	if err == nil {
		// Window state and anything else MyGo keeps follow the data directory.
		mygo.App.SetPath(mygo.PathUserData, where.DataDir)
		err = study.open(workspace.Options{
			DBPath:      where.Database,
			Credentials: newCredentialStore(mygo.Secrets.Service(secretService(getenv))),
			Resources:   resourceDir(),
		})
	}
	if err != nil {
		study.mu.Lock()
		study.openErr = err
		study.mu.Unlock()
		log.Printf("passquest-desktop: open workspace: %v", err)
	}
}

// resourceDir is the directory the packaged app carries its files in
// (mygo.PathResources): the bundled agent hosts and Node runtime live there.
// A development build keeps the source checkout's lookups, so it passes none.
func resourceDir() string {
	if mygo.IsDev() {
		return ""
	}
	dir, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		return ""
	}
	return dir
}

// frontendFS is the page's file system, rooted at web/.
func frontendFS() (fs.FS, error) { return fs.Sub(web, "web") }

// startURL is the page the window opens: the app, or a page that says the
// frontend bundle is missing instead of a blank window.
func startURL(frontend fs.FS) string {
	if _, err := fs.Stat(frontend, "dist/main.js"); err != nil {
		return "/not-built.html"
	}
	return "/"
}
