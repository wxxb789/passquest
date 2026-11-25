package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// legacyWorkDir makes a working directory holding the old default database and
// returns it with that file's path.
func legacyWorkDir(t *testing.T) (work, oldPath string) {
	t.Helper()
	work = filepath.Join(t.TempDir(), "work dir")
	oldPath = filepath.Join(work, ".passquest", "passquest.sqlite")
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, []byte("old database bytes"), 0o640); err != nil {
		t.Fatal(err)
	}
	return work, oldPath
}

// noticeOptions is one run's options under a temp config root, with the notice
// state withDefaults installs.
func noticeOptions(stderr *bytes.Buffer, config string, env map[string]string) Options {
	return Options{
		Stderr:    stderr,
		Getenv:    func(name string) string { return env[name] },
		ConfigDir: func() (string, error) { return config, nil },
	}.withDefaults()
}

func TestOldDefaultNoticeBeforeWork(t *testing.T) {
	work, oldPath := legacyWorkDir(t)
	t.Chdir(work)
	config := t.TempDir()
	newPath := filepath.Join(config, "Pass Quest", "passquest.sqlite")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"report", "--json"}, Options{
		Stdout:    &stdout,
		Stderr:    &stderr,
		Now:       (&stepClock{base: goldenBase}).Now,
		Getenv:    func(key string) string { return map[string]string{"SHELL": "/bin/bash"}[key] },
		ConfigDir: func() (string, error) { return config, nil },
	})
	if code != 0 {
		t.Fatalf("report = %d, stderr %s", code, stderr.String())
	}
	want := "passquest: a database from the old default location exists at " + oldPath +
		"; the command line now uses " + newPath +
		". Pass Quest will not move it; move or merge it yourself: mv '" + oldPath + "' '" + newPath +
		"' [quoted for sh (from SHELL=/bin/bash); override with PASSQUEST_SHELL=sh|cmd|powershell]\n"
	if stderr.String() != want {
		t.Fatalf("stderr = %q,\nwant  %q", stderr.String(), want)
	}
	if !json.Valid(stdout.Bytes()) {
		t.Fatalf("stdout = %q, want the JSON document", stdout.String())
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("per-user database: %v", err)
	}
}

func TestOldDefaultNoticeOncePerRunAndNothingMoved(t *testing.T) {
	work, oldPath := legacyWorkDir(t)
	var oldMode os.FileMode
	if runtime.GOOS != "windows" {
		info, err := os.Stat(oldPath)
		if err != nil {
			t.Fatal(err)
		}
		oldMode = info.Mode().Perm()
	}
	t.Chdir(work)
	config := t.TempDir()
	newPath := filepath.Join(config, "Pass Quest", "passquest.sqlite")

	var stderr bytes.Buffer
	opt := noticeOptions(&stderr, config, nil)
	first, err := opt.resolveDatabase("")
	if err != nil {
		t.Fatal(err)
	}
	if first.Path != newPath || !first.Private {
		t.Fatalf("resolved = %+v, want %q private", first, newPath)
	}
	if _, err := opt.resolveDatabase(""); err != nil {
		t.Fatal(err)
	}
	if _, err := opt.ports(""); err != nil {
		t.Fatal(err)
	}

	want := "passquest: a database from the old default location exists at " + oldPath +
		"; the command line now uses " + newPath +
		". Pass Quest will not move it; move or merge it yourself: mv '" + oldPath + "' '" + newPath +
		"' [quoted for sh (no shell detected, defaulted); override with PASSQUEST_SHELL=sh|cmd|powershell]\n"
	if stderr.String() != want {
		t.Fatalf("three resolutions in one run printed %q,\nwant %q", stderr.String(), want)
	}

	after, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != "old database bytes" {
		t.Fatalf("old database bytes = %q", after)
	}
	entries, err := os.ReadDir(filepath.Dir(oldPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "passquest.sqlite" {
		t.Fatalf("old directory entries = %v, want only passquest.sqlite", entries)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(oldPath)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != oldMode {
			t.Fatalf("old database mode = %04o, want %04o unchanged", info.Mode().Perm(), oldMode)
		}
	}
}

func TestOldDefaultNoticeCommandPerShell(t *testing.T) {
	work, oldPath := legacyWorkDir(t)
	t.Chdir(work)
	config := t.TempDir()
	newPath := filepath.Join(config, "Pass Quest", "passquest.sqlite")
	prose := "passquest: a database from the old default location exists at " + oldPath +
		"; the command line now uses " + newPath + ". Pass Quest will not move it; move or merge it yourself"
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			"posix quoting",
			map[string]string{"SHELL": "/bin/bash"},
			prose + ": mv '" + oldPath + "' '" + newPath + "' [quoted for sh (from SHELL=/bin/bash); override with PASSQUEST_SHELL=sh|cmd|powershell]\n",
		},
		{
			"cmd quoting",
			map[string]string{"PASSQUEST_SHELL": "cmd"},
			prose + ": move \"" + oldPath + "\" \"" + newPath + "\" [quoted for cmd (from PASSQUEST_SHELL); override with PASSQUEST_SHELL=sh|cmd|powershell]\n",
		},
		{
			"powershell quoting",
			map[string]string{"PASSQUEST_SHELL": "powershell"},
			prose + ": Move-Item '" + oldPath + "' '" + newPath + "' [quoted for powershell (from PASSQUEST_SHELL); override with PASSQUEST_SHELL=sh|cmd|powershell]\n",
		},
		{
			"unknown override keeps the paths without a command",
			map[string]string{"PASSQUEST_SHELL": "tcsh"},
			prose + ".\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			opt := noticeOptions(&stderr, config, tt.env)
			if _, err := opt.resolveDatabase(""); err != nil {
				t.Fatal(err)
			}
			if stderr.String() != tt.want {
				t.Fatalf("notice = %q,\nwant  %q", stderr.String(), tt.want)
			}
		})
	}
}

// The steady state after the upgrade: any earlier run created the per-user
// database, so the notice names both paths but must not offer a move — a POSIX
// mv replaces the per-user database without a prompt.
func TestOldDefaultNoticeWithBothDatabasesPrintsNoMoveCommand(t *testing.T) {
	work, oldPath := legacyWorkDir(t)
	t.Chdir(work)
	config := t.TempDir()
	newPath := filepath.Join(config, "Pass Quest", "passquest.sqlite")
	if err := os.MkdirAll(filepath.Dir(newPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("per-user bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	want := "passquest: a database from the old default location exists at " + oldPath +
		"; the command line now uses " + newPath +
		". Pass Quest will not move it; move or merge it yourself (the per-user database already exists, so no move command is printed; merge instead of overwriting it).\n"
	for _, family := range []string{"sh", "cmd"} {
		var stderr bytes.Buffer
		opt := noticeOptions(&stderr, config, map[string]string{"PASSQUEST_SHELL": family})
		if _, err := opt.resolveDatabase(""); err != nil {
			t.Fatal(err)
		}
		if stderr.String() != want {
			t.Fatalf("%s notice = %q,\nwant  %q", family, stderr.String(), want)
		}
	}
	for path, wantBytes := range map[string]string{oldPath: "old database bytes", newPath: "per-user bytes"} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != wantBytes {
			t.Fatalf("%s = %q, want %q", path, got, wantBytes)
		}
	}
}

// cmd.exe expands %NAME% even inside double quotes, so a move command quoting
// a path with % is not copy-safe; the notice keeps the literal paths and drops
// the command instead.
func TestOldDefaultNoticeDropsCmdCommandOnPercentPath(t *testing.T) {
	work := filepath.Join(t.TempDir(), "100% done")
	oldPath := filepath.Join(work, ".passquest", "passquest.sqlite")
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, []byte("old database bytes"), 0o640); err != nil {
		t.Fatal(err)
	}
	t.Chdir(work)
	config := t.TempDir()
	newPath := filepath.Join(config, "Pass Quest", "passquest.sqlite")
	var stderr bytes.Buffer
	opt := noticeOptions(&stderr, config, map[string]string{"PASSQUEST_SHELL": "cmd"})
	if _, err := opt.resolveDatabase(""); err != nil {
		t.Fatal(err)
	}
	want := "passquest: a database from the old default location exists at " + oldPath +
		"; the command line now uses " + newPath +
		". Pass Quest will not move it; move or merge it yourself.\n"
	if stderr.String() != want {
		t.Fatalf("notice = %q,\nwant  %q", stderr.String(), want)
	}
}

func TestOldDefaultNoticeAbsent(t *testing.T) {
	config := t.TempDir()
	tests := []struct {
		name  string
		setup func(t *testing.T, work string) map[string]string
		flag  string
		old   bool
	}{
		{
			name:  "no old database",
			setup: func(t *testing.T, work string) map[string]string { return nil },
		},
		{
			name: "explicit --db",
			setup: func(t *testing.T, work string) map[string]string {
				return nil
			},
			flag: filepath.Join(t.TempDir(), "explicit.sqlite"),
			old:  true,
		},
		{
			name: "PASSQUEST_DB",
			setup: func(t *testing.T, work string) map[string]string {
				return map[string]string{"PASSQUEST_DB": filepath.Join(t.TempDir(), "explicit.sqlite")}
			},
			old: true,
		},
		{
			name: "the old file is the resolved database",
			setup: func(t *testing.T, work string) map[string]string {
				return map[string]string{"PASSQUEST_DESKTOP_DATA": filepath.Join(work, ".passquest")}
			},
			old: true,
		},
		{
			name: "the old path is a directory",
			setup: func(t *testing.T, work string) map[string]string {
				if err := os.MkdirAll(filepath.Join(work, ".passquest", "passquest.sqlite"), 0o750); err != nil {
					t.Fatal(err)
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			work := filepath.Join(t.TempDir(), "work dir")
			if err := os.MkdirAll(work, 0o755); err != nil {
				t.Fatal(err)
			}
			t.Chdir(work)
			env := tt.setup(t, work)
			if tt.old {
				oldPath := filepath.Join(work, ".passquest", "passquest.sqlite")
				if err := os.MkdirAll(filepath.Dir(oldPath), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(oldPath, []byte("old database bytes"), 0o640); err != nil {
					t.Fatal(err)
				}
			}
			var stderr bytes.Buffer
			opt := noticeOptions(&stderr, config, env)
			if _, err := opt.resolveDatabase(tt.flag); err != nil {
				t.Fatal(err)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want no notice", stderr.String())
			}
		})
	}
}
