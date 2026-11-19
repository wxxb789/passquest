// Package hostpaths resolves the Node runtime and the four agent host
// scripts (tutor, Anki, search, durable agent). One resolution order serves
// every caller behind both adapters: the explicit environment override, then
// the directory a packaged app carries its files in, then the source
// checkout's candidates. The package imports nothing from the desktop or the
// CLI, so a package build and a source checkout use the same function.
package hostpaths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// NodeEnv is the variable that overrides the Node runtime for every host.
const NodeEnv = "PASSQUEST_NODE"

// Source is one lookup's inputs: the process seams each caller already has,
// plus the directory a packaged app carries its files in.
type Source struct {
	// Getenv, Getwd and Executable are the process seams; nil means os.Getenv,
	// os.Getwd and os.Executable.
	Getenv     func(string) string
	Getwd      func() (string, error)
	Executable func() (string, error)
	// Resources is the directory the packaged app's files live in
	// (mygo.PathResources in the desktop: Contents/Resources in a macOS
	// bundle, the executable's directory elsewhere). It is empty in a source
	// checkout, which keeps the fallback candidates exactly as they were.
	Resources string
}

// Host names one agent host script and the strings its error messages are
// built from.
type Host struct {
	// Prefix begins every error message, e.g. "tutor".
	Prefix string
	// Noun names the script in error messages, e.g. "agent host".
	Noun string
	// Env overrides the path, e.g. PASSQUEST_TUTOR_HOST; when it is set it is
	// the only candidate.
	Env string
	// Script is the file under agent/, e.g. "tutor-host.mjs".
	Script string
}

// ResolveHost finds the host script h names. An explicit override (h.Env) is
// authoritative and must exist. Otherwise the packaged resources directory,
// the working directory, then the executable's directory and its parent are
// checked in that order, and the first candidate that exists wins. The
// returned path is absolute.
func ResolveHost(h Host, src Source) (string, error) {
	getenv := src.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	if value := strings.TrimSpace(getenv(h.Env)); value != "" {
		path, err := filepath.Abs(value)
		if err != nil {
			return "", fmt.Errorf("%s: resolve %s: %w", h.Prefix, h.Env, err)
		}
		if err := requireFile(h, path); err != nil {
			return "", err
		}
		return path, nil
	}

	var candidates []string
	packaged := ""
	if src.Resources != "" {
		packaged = filepath.Clean(filepath.Join(src.Resources, "agent", h.Script))
		candidates = append(candidates, packaged)
	}
	getwd := src.Getwd
	if getwd == nil {
		getwd = os.Getwd
	}
	if cwd, err := getwd(); err == nil {
		candidates = appendFallback(candidates, packaged, filepath.Join(cwd, "agent", h.Script))
	}
	executable := src.Executable
	if executable == nil {
		executable = os.Executable
	}
	if exe, err := executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = appendFallback(candidates, packaged, filepath.Join(dir, "agent", h.Script))
		candidates = appendFallback(candidates, packaged, filepath.Join(dir, "..", "agent", h.Script))
	}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if requireFile(h, absolute) == nil {
			return absolute, nil
		}
	}
	return "", fmt.Errorf("%s: %s not found; set %s to agent/%s (looked in %s)",
		h.Prefix, h.Noun, h.Env, h.Script, strings.Join(candidates, ", "))
}

// appendFallback adds one source-checkout candidate unless the packaged
// directory already names the same path: on Windows and Linux the resources
// directory is the executable's directory, and the same path twice adds
// nothing while a cwd host would otherwise shadow the packaged one.
func appendFallback(candidates []string, packaged, candidate string) []string {
	if packaged != "" && filepath.Clean(candidate) == packaged {
		return candidates
	}
	return append(candidates, candidate)
}

// ResolveNode is the Node executable the hosts spawn: the PASSQUEST_NODE
// override unchanged, else the runtime a package carries under
// resources/node, else "node" from PATH. goos selects the packaged file name
// ("node.exe" on Windows, "bin/node" elsewhere) and is a parameter so both
// layouts are testable wherever the test runs.
func ResolveNode(src Source, goos string) string {
	getenv := src.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	if node := strings.TrimSpace(getenv(NodeEnv)); node != "" {
		return node
	}
	if src.Resources != "" {
		packaged := filepath.Join(src.Resources, "node", "bin", "node")
		if goos == "windows" {
			packaged = filepath.Join(src.Resources, "node", "node.exe")
		}
		// A packaged runtime that lost its executable bit falls through to PATH,
		// as an unusable host script falls through to the next candidate. The bit
		// is a property of this machine's file system, not of goos.
		if info, err := os.Stat(packaged); err == nil && !info.IsDir() && (runtime.GOOS == "windows" || info.Mode()&0o111 != 0) {
			return packaged
		}
	}
	return "node"
}

// requireFile reports why a host path cannot be run, in h's words.
func requireFile(h Host, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: %s %s: %w", h.Prefix, h.Noun, path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s: %s %s is a directory", h.Prefix, h.Noun, path)
	}
	return nil
}
