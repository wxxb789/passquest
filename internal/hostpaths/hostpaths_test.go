package hostpaths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testHost is one representative host spec: the shapes that differ (prefix,
// noun, env, script) are exercised together in TestResolveHostMessages.
var testHost = Host{Prefix: "tutor", Noun: "agent host", Env: "PASSQUEST_TUTOR_HOST", Script: "tutor-host.mjs"}

// none is an environment with no override set.
func none(string) string { return "" }

// noFiles is a getwd/executable pair over empty directories.
func noFiles(t *testing.T) (func() (string, error), func() (string, error)) {
	t.Helper()
	return func() (string, error) { return t.TempDir(), nil },
		func() (string, error) { return filepath.Join(t.TempDir(), "passquest"), nil }
}

// writeHost creates <dir>/agent/<script> and returns the path.
func writeHost(t *testing.T, dir, script string) string {
	t.Helper()
	path := filepath.Join(dir, "agent", script)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("// host\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveHostOrder(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		// place is where a host file exists; the resources directory itself is
		// only passed when packaged=true.
		place    map[string]bool
		packaged bool
		want     string
	}{
		{name: "resources wins over cwd and exe", place: map[string]bool{"resources": true, "cwd": true, "exe": true}, packaged: true, want: "resources"},
		{name: "missing resources file falls through to cwd", place: map[string]bool{"cwd": true, "exe": true}, packaged: true, want: "cwd"},
		{name: "cwd wins over exe", place: map[string]bool{"cwd": true, "exe": true}, packaged: true, want: "cwd"},
		{name: "exe wins over its parent", place: map[string]bool{"exe": true, "parent": true}, packaged: true, want: "exe"},
		{name: "parent is the last candidate", place: map[string]bool{"parent": true}, packaged: true, want: "parent"},
		{name: "source checkout keeps cwd first", place: map[string]bool{"exe": true}, want: "exe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			resources := filepath.Join(base, "resources")
			cwd := filepath.Join(base, "cwd")
			exeDir := filepath.Join(base, "app", "bin")
			dirs := map[string]string{
				"resources": resources, "cwd": cwd, "exe": exeDir, "parent": filepath.Dir(exeDir),
			}
			made := map[string]string{}
			for place, dir := range dirs {
				if tc.place[place] {
					made[place] = writeHost(t, dir, testHost.Script)
				}
			}
			src := Source{Getenv: none,
				Getwd:      func() (string, error) { return cwd, nil },
				Executable: func() (string, error) { return filepath.Join(exeDir, "passquest"), nil }}
			if tc.packaged {
				src.Resources = resources
			}
			got, err := ResolveHost(testHost, src)
			if err != nil {
				t.Fatalf("ResolveHost = %v", err)
			}
			if want := made[tc.want]; got != want {
				t.Fatalf("ResolveHost = %q; want the %s host %q", got, tc.want, want)
			}
		})
	}
}

func TestResolveHostOverrideIsTheOnlyCandidate(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	override := writeHost(t, base, testHost.Script)
	cwd := filepath.Join(base, "cwd")
	exeDir := filepath.Join(base, "app", "bin")
	resources := filepath.Join(base, "resources")
	writeHost(t, cwd, testHost.Script)
	writeHost(t, exeDir, testHost.Script)
	writeHost(t, resources, testHost.Script)
	env := func(key string) string {
		if key == testHost.Env {
			return override
		}
		return ""
	}
	src := Source{Getenv: env, Resources: resources,
		Getwd:      func() (string, error) { return cwd, nil },
		Executable: func() (string, error) { return filepath.Join(exeDir, "passquest"), nil }}
	got, err := ResolveHost(testHost, src)
	if err != nil || got != override {
		t.Fatalf("ResolveHost = %q, %v; want the override %q", got, err, override)
	}

	// A missing override is refused even when every fallback exists.
	missing := filepath.Join(base, "missing.mjs")
	src.Getenv = func(key string) string {
		if key == testHost.Env {
			return missing
		}
		return ""
	}
	_, err = ResolveHost(testHost, src)
	want := fmt.Sprintf("%s: %s %s:", testHost.Prefix, testHost.Noun, missing)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("missing override error = %v; want %q", err, want)
	}
}

// TestResolveHostMessages pins the byte-identical diagnostic every host shape
// produces when nothing is found and no resources directory is given: a
// source checkout keeps exactly the message it had before the shared resolver.
func TestResolveHostMessages(t *testing.T) {
	t.Parallel()
	shapes := []Host{
		{Prefix: "tutor", Noun: "agent host", Env: "PASSQUEST_TUTOR_HOST", Script: "tutor-host.mjs"},
		{Prefix: "anki", Noun: "host", Env: "PASSQUEST_ANKI_HOST", Script: "anki-host.mjs"},
		{Prefix: "search", Noun: "host", Env: "PASSQUEST_SEARCH_HOST", Script: "search-host.mjs"},
		{Prefix: "agent", Noun: "durable host", Env: "PASSQUEST_AGENT_HOST", Script: "durable-host.mjs"},
	}
	base := t.TempDir()
	cwd := filepath.Join(base, "cwd")
	exeDir := filepath.Join(base, "app", "bin")
	src := Source{Getenv: none,
		Getwd:      func() (string, error) { return cwd, nil },
		Executable: func() (string, error) { return filepath.Join(exeDir, "passquest"), nil }}
	for _, h := range shapes {
		t.Run(h.Prefix, func(t *testing.T) {
			t.Parallel()
			_, err := ResolveHost(h, src)
			looked := []string{
				filepath.Join(cwd, "agent", h.Script),
				filepath.Join(exeDir, "agent", h.Script),
				filepath.Join(filepath.Dir(exeDir), "agent", h.Script),
			}
			want := fmt.Sprintf("%s: %s not found; set %s to agent/%s (looked in %s)",
				h.Prefix, h.Noun, h.Env, h.Script, strings.Join(looked, ", "))
			if err == nil || err.Error() != want {
				t.Fatalf("error = %v; want %q", err, want)
			}
		})
	}
}

// TestResolveHostPackagedCandidateSupersedesTheExecutable pins the packaged
// layout on Windows and Linux, where the resources directory is the
// executable's directory: the packaged path is looked in first and listed
// once, and the cwd copy no longer shadows it.
func TestResolveHostPackagedCandidateSupersedesTheExecutable(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	cwd := filepath.Join(base, "cwd")
	exeDir := filepath.Join(base, "app", "bin")
	resources := exeDir
	packaged := writeHost(t, exeDir, testHost.Script)
	cwdHost := writeHost(t, cwd, testHost.Script)
	src := Source{Getenv: none, Resources: resources,
		Getwd:      func() (string, error) { return cwd, nil },
		Executable: func() (string, error) { return filepath.Join(exeDir, "passquest"), nil }}
	got, err := ResolveHost(testHost, src)
	if err != nil || got != packaged {
		t.Fatalf("ResolveHost = %q, %v; want the packaged host %q", got, err, packaged)
	}

	// With the packaged file gone, the cwd copy answers next.
	if err := os.Remove(packaged); err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveHost(testHost, src); err != nil || got != cwdHost {
		t.Fatalf("ResolveHost = %q, %v; want the cwd host %q", got, err, cwdHost)
	}
	if err := os.Remove(cwdHost); err != nil {
		t.Fatal(err)
	}
	_, err = ResolveHost(testHost, src)
	looked := []string{
		filepath.Join(resources, "agent", testHost.Script),
		filepath.Join(cwd, "agent", testHost.Script),
		filepath.Join(filepath.Dir(exeDir), "agent", testHost.Script),
	}
	want := fmt.Sprintf("%s: %s not found; set %s to agent/%s (looked in %s)",
		testHost.Prefix, testHost.Noun, testHost.Env, testHost.Script, strings.Join(looked, ", "))
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v; want %q", err, want)
	}
}

func TestResolveHostFromTheProcessSeams(t *testing.T) {
	t.Parallel()
	// nil seams fall back to os.Getwd and os.Executable; with no candidate
	// anywhere the lookup is refused rather than inventing a path.
	base := t.TempDir()
	getwd, executable := noFiles(t)
	src := Source{Getenv: none, Getwd: getwd, Executable: executable}
	if _, err := ResolveHost(testHost, src); err == nil {
		t.Fatalf("ResolveHost with empty directories = nil error")
	}
	host := writeHost(t, base, testHost.Script)
	src.Resources = base
	got, err := ResolveHost(testHost, src)
	if err != nil || got != host {
		t.Fatalf("ResolveHost = %q, %v; want %q", got, err, host)
	}
	// An override that is a directory is reported as one, not run.
	dir := t.TempDir()
	src.Getenv = func(key string) string {
		if key == testHost.Env {
			return dir
		}
		return ""
	}
	_, err = ResolveHost(testHost, src)
	want := fmt.Sprintf("%s: %s %s is a directory", testHost.Prefix, testHost.Noun, dir)
	if err == nil || err.Error() != want {
		t.Fatalf("directory error = %v; want %q", err, want)
	}
}

func TestResolveNode(t *testing.T) {
	t.Parallel()
	override := filepath.Join(t.TempDir(), "custom-node")
	cases := []struct {
		name   string
		env    string
		layout string // "", "windows", "posix", "dir"
		goos   string
		want   string // "env", "packaged", "path"
	}{
		{name: "windows override wins over the packaged runtime", env: override, layout: "windows", goos: "windows", want: "env"},
		{name: "posix override wins over the packaged runtime", env: override, layout: "posix", goos: "linux", want: "env"},
		{name: "blank override is no override", env: "   ", layout: "posix", goos: "linux", want: "packaged"},
		{name: "windows uses node.exe", layout: "windows", goos: "windows", want: "packaged"},
		{name: "windows ignores the posix layout", layout: "posix", goos: "windows", want: "path"},
		{name: "linux uses node/bin/node", layout: "posix", goos: "linux", want: "packaged"},
		{name: "darwin uses node/bin/node", layout: "posix", goos: "darwin", want: "packaged"},
		{name: "linux ignores node.exe", layout: "windows", goos: "linux", want: "path"},
		{name: "missing packaged runtime falls back to PATH", layout: "", goos: "linux", want: "path"},
		{name: "a directory is not a runtime", layout: "dir", goos: "windows", want: "path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resources := t.TempDir()
			packaged := filepath.Join(resources, "node", "node.exe")
			switch tc.layout {
			case "windows":
				if err := os.MkdirAll(filepath.Dir(packaged), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(packaged, []byte("node\n"), 0o700); err != nil {
					t.Fatal(err)
				}
			case "posix":
				packaged = filepath.Join(resources, "node", "bin", "node")
				if err := os.MkdirAll(filepath.Dir(packaged), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(packaged, []byte("node\n"), 0o700); err != nil {
					t.Fatal(err)
				}
			case "dir":
				if err := os.MkdirAll(packaged, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			src := Source{Resources: resources, Getenv: func(key string) string {
				if key == NodeEnv {
					return tc.env
				}
				return ""
			}}
			got := ResolveNode(src, tc.goos)
			want := tc.want
			switch want {
			case "env":
				want = tc.env
			case "packaged":
				want = packaged
			default:
				want = "node"
			}
			if got != want {
				t.Fatalf("ResolveNode(%s) = %q; want %q", tc.goos, got, want)
			}
		})
	}
}

func TestResolveNodeWithoutResources(t *testing.T) {
	t.Parallel()
	if got := ResolveNode(Source{Getenv: none}, "windows"); got != "node" {
		t.Fatalf("ResolveNode = %q; want the PATH name %q", got, "node")
	}
}
