package desktop

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
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

// resolveStorage picks the data directory (EnvData, else the app's per-user
// directory from userData) and the database (EnvDB, else DatabaseFile in the
// data directory), creating the data directory.
func resolveStorage(getenv func(string) string, userData func() (string, error)) (storage, error) {
	dir := strings.TrimSpace(getenv(EnvData))
	if dir == "" {
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
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return storage{}, err
	}
	db := strings.TrimSpace(getenv(EnvDB))
	if db == "" {
		db = filepath.Join(dir, DatabaseFile)
	}
	return storage{DataDir: dir, Database: db}, nil
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
