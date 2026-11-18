package desktop

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Identity of the app. AppIdentifier is the reverse-DNS identifier packaging
// (goal 0013) must keep; it also names the credential-store service, so a key
// stored by a `go build` binary is the one the packaged app reads.
const (
	AppName       = "Pass Quest"
	AppIdentifier = "com.github.wxxb789.passquest"
)

// Environment variables of the desktop app.
const (
	// EnvData overrides the per-user data directory (database, window state).
	EnvData = "PASSQUEST_DESKTOP_DATA"
	// EnvDB overrides the database file, as it does for the CLI (Q-0011-04).
	EnvDB = "PASSQUEST_DB"
	// EnvSecretService overrides the credential-store service.
	EnvSecretService = "PASSQUEST_DESKTOP_SECRET_SERVICE"
	// EnvHidden=1 creates the window hidden (smoke runs).
	EnvHidden = "PASSQUEST_DESKTOP_HIDDEN"
	// EnvScript and EnvResult drive the smoke hook (see smoke.go).
	EnvScript = "PASSQUEST_DESKTOP_SCRIPT"
	EnvResult = "PASSQUEST_DESKTOP_RESULT"
)

// DatabaseFile is the database's name inside the data directory.
const DatabaseFile = "passquest.sqlite"

// storage is where one app instance keeps its data.
type storage struct {
	DataDir  string
	Database string
}

// installDirs name the directories the app's own files live in: the running
// executable's directory, and the MyGo resources directory (their
// Contents/Resources inside a macOS bundle, the executable's directory
// elsewhere). The learner's data must live under neither, however the
// per-user directory is resolved.
type installDirs struct {
	Executable func() (string, error)
	Resources  func() (string, error)
	// Home is the learner's home directory. A binary placed in it or in one
	// of its ancestors (a drive root) is loose, not installed: that directory
	// holds the learner's own files, so it is not refused.
	Home func() (string, error)
}

// resolveStorage picks the data directory (EnvData, else the app's per-user
// directory from userData) and the database (EnvDB, else DatabaseFile in the
// data directory), creating the data directory 0700. The app-owned default
// directory is tightened to 0700 on POSIX even when it already exists, because
// MyGo creates it 0755 (Q-0002-02: the default database lives in a 0700
// directory).
//
// A default directory that lands inside the app's own files (installDirs) is
// refused before it is created or written: an install is replaced by an
// upgrade and removed by an uninstall, and the learner's evidence must survive
// both. The EnvData override is the learner's explicit choice and is taken as
// given.
func resolveStorage(getenv func(string) string, userData func() (string, error), install installDirs) (storage, error) {
	dir := strings.TrimSpace(getenv(EnvData))
	appOwned := dir == ""
	if appOwned {
		var err error
		if dir, err = userData(); err != nil {
			return storage{}, err
		}
	}
	if dir == "" {
		return storage{}, errors.New("no user data directory")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return storage{}, err
	}
	if appOwned && insideInstall(dir, install) {
		return storage{}, fmt.Errorf("refusing to keep data at %s: it is inside the app's own files; set %s to a per-user directory", dir, EnvData)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return storage{}, err
	}
	if appOwned && runtime.GOOS != "windows" {
		if err := os.Chmod(dir, 0o700); err != nil {
			return storage{}, err
		}
	}
	db := strings.TrimSpace(getenv(EnvDB))
	if db == "" {
		db = filepath.Join(dir, DatabaseFile)
	}
	return storage{DataDir: dir, Database: db}, nil
}

// insideInstall reports whether dir is the app's executable directory, its
// resources directory, or under either. A root that is the learner's home or
// one of its ancestors holds the learner's own files, not the app's, so it
// never counts.
func insideInstall(dir string, install installDirs) bool {
	for _, source := range []func() (string, error){install.executableDir, install.Resources} {
		if source == nil {
			continue
		}
		if root, err := source(); err == nil && root != "" && within(dir, root) && !holdsHome(root, install) {
			return true
		}
	}
	return false
}

// executableDir is the directory of the running executable.
func (install installDirs) executableDir() (string, error) {
	if install.Executable == nil {
		return "", nil
	}
	exe, err := install.Executable()
	if err != nil || exe == "" {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// holdsHome reports whether root is the learner's home directory or one of
// its ancestors.
func holdsHome(root string, install installDirs) bool {
	if install.Home == nil {
		return false
	}
	home, err := install.Home()
	if err != nil || home == "" {
		return false
	}
	home, err = filepath.Abs(home)
	return err == nil && within(home, root)
}

// within reports whether path is root or inside it.
func within(path, root string) bool {
	root, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// appName is the name the app runs under. An overridden data directory is a
// separate instance: its own single-instance lock and WebView profile, so a
// smoke run never hands over to the learner's open window.
func appName(getenv func(string) string) string {
	dir := strings.TrimSpace(getenv(EnvData))
	if dir == "" {
		return AppName
	}
	sum := sha256.Sum256([]byte(dir))
	return AppName + " " + hex.EncodeToString(sum[:4])
}

// secretService is the credential-store service keys are kept under.
func secretService(getenv func(string) string) string {
	if service := strings.TrimSpace(getenv(EnvSecretService)); service != "" {
		return service
	}
	return AppIdentifier
}
