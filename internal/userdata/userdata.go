// Package userdata owns the per-user data directory and the default database
// path both entry points (the command line and the desktop window) resolve, so
// a window and a command line on one machine read the same store.
package userdata

import (
	"path/filepath"
	"strings"
)

// AppName names the per-user directory. The desktop app declares the same name
// in internal/desktop, where packaging's identity lives; a parity test pins the
// two.
const AppName = "Pass Quest"

// Environment variables both entry points read.
const (
	// DataEnv overrides the per-user data directory (database, window state).
	DataEnv = "PASSQUEST_DESKTOP_DATA"
	// DatabaseEnv overrides the database file.
	DatabaseEnv = "PASSQUEST_DB"
)

// DatabaseFile is the database's name inside the data directory.
const DatabaseFile = "passquest.sqlite"

// LegacyDatabasePath is the working-directory default the command line used
// before goal 0020. Nothing resolves it any more; it names the file the notice
// about the old default looks for.
const LegacyDatabasePath = ".passquest/passquest.sqlite"

// Dir resolves the per-user data directory: the DataEnv override (whitespace
// trimmed, taken only when non-empty, as the window takes it), else configDir
// joined with AppName. configDir is os.UserConfigDir in both roots, so the
// directory is that function's own answer.
func Dir(configDir func() (string, error), getenv func(string) string) (string, error) {
	if dir := strings.TrimSpace(getenv(DataEnv)); dir != "" {
		return dir, nil
	}
	root, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, AppName), nil
}

// DatabasePath resolves the database a command opens: the flag as given when it
// is not whitespace-only, then the trimmed DatabaseEnv, then DatabaseFile
// inside Dir. A flag value that is only whitespace selects the next source; the
// selected value is returned as given.
func DatabasePath(flagValue string, configDir func() (string, error), getenv func(string) string) (string, error) {
	if strings.TrimSpace(flagValue) != "" {
		return flagValue, nil
	}
	if v := strings.TrimSpace(getenv(DatabaseEnv)); v != "" {
		return v, nil
	}
	dir, err := Dir(configDir, getenv)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DatabaseFile), nil
}
