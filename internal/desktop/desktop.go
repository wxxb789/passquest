// Package desktop is the Pass Quest window: a MyGo app whose page calls the
// bound Study service, which forwards to one cli.Workspace. It opens no
// network listener: MyGo's single-instance lock is a Unix-domain socket and
// the smoke hook reads and writes files.
package desktop

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/wxxb789/passquest/internal/cli"
)

// web is the page: the committed index.html and not-built.html plus the
// gitignored dist/ that `bun run desktop:build` bundles from desktop/web.
//
//go:embed all:web
var web embed.FS

// quitGrace bounds how long a quit waits for calls still running.
const quitGrace = 5 * time.Second

// BriefChanged is broadcast when a brief resumed in the background starts and
// when its host returns.
var BriefChanged = mygo.NewEvent[BriefEvent]("brief:changed")

// Run runs the window until the app quits and returns the process exit code.
func Run() int {
	getenv := os.Getenv
	frontend, err := frontendFS()
	if err != nil {
		log.Printf("passquest-desktop: %v", err)
		return 1
	}
	mygo.SetFrontend(frontend)
	mygo.App.SetName(appName(getenv))

	study := newStudy(func(e BriefEvent) { _ = BriefChanged.Broadcast(e) })
	mygo.BindAs("Study", study)

	generating := getenv("MYGO_GENERATE") != ""
	smoke, err := smokeFromEnv(getenv, mygo.App.Quit)
	if err != nil {
		log.Printf("passquest-desktop: %v", err)
		return 2
	}
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
	// A quit waits, bounded, for calls still running; the job object then
	// ends any host they left (Windows).
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
		win = mygo.NewWindow(mygo.WindowOptions{
			URL:       startURL(frontend),
			Title:     AppName,
			Width:     1280,
			Height:    820,
			MinWidth:  880,
			MinHeight: 600,
			Hidden:    strings.TrimSpace(getenv(EnvHidden)) == "1",
			StateKey:  "main",
		})
		if smoke != nil {
			attachSmoke(smoke, win)
		}
	})

	if err := mygo.App.Run(); err != nil {
		log.Printf("passquest-desktop: %v", err)
		return 1
	}
	if smoke != nil && smoke.writeFailed.Load() {
		return 1
	}
	return 0
}

// openStudy opens the workspace on the resolved storage. A failure is kept
// by the service and shown by the window as an error state.
func openStudy(study *Study, getenv func(string) string) {
	where, err := resolveStorage(getenv, func() (string, error) { return mygo.App.Path(mygo.PathUserData) })
	if err == nil {
		// Window state and anything else MyGo keeps follow the data directory.
		mygo.App.SetPath(mygo.PathUserData, where.DataDir)
		err = study.open(cli.WorkspaceOptions{
			DBPath:      where.Database,
			Credentials: newCredentialStore(mygo.Secrets.Service(secretService(getenv))),
		})
	}
	if err != nil {
		study.mu.Lock()
		study.openErr = err
		study.mu.Unlock()
		log.Printf("passquest-desktop: open workspace: %v", err)
	}
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

// attachSmoke runs the smoke script once the page has loaded, and gives up
// with a failure result when it never does.
func attachSmoke(h *smokeHook, win *mygo.Window) {
	watchdog := time.AfterFunc(h.timeout+smokeLoadGrace, func() {
		h.finish(nil, fmt.Errorf("the page did not finish loading within %s", h.timeout+smokeLoadGrace))
	})
	started := false
	win.Page().OnDidFinishLoad(func() {
		if started {
			return
		}
		started = true
		go func() {
			h.run(func(ctx context.Context, script string) (any, error) { return win.Page().EvalContext(ctx, script) })
			watchdog.Stop()
		}()
	})
}
