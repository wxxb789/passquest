package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wxxb789/passquest/internal/userdata"
)

// TestUserdataParity pins the four names the desktop declares for packaging's
// sake against the shared resolution, so a drift in either fails before a run.
func TestUserdataParity(t *testing.T) {
	t.Parallel()
	if AppName != userdata.AppName || EnvData != userdata.DataEnv || EnvDB != userdata.DatabaseEnv || DatabaseFile != userdata.DatabaseFile {
		t.Fatalf("desktop names = %q/%q/%q/%q, internal/userdata = %q/%q/%q/%q",
			AppName, EnvData, EnvDB, DatabaseFile,
			userdata.AppName, userdata.DataEnv, userdata.DatabaseEnv, userdata.DatabaseFile)
	}
}

// TestDefaultDataSeam resolves the window's app-owned default through the same
// computation the CLI uses, with a temp config root.
func TestDefaultDataSeam(t *testing.T) {
	root := t.TempDir()
	getenv := envOf(nil)
	where, err := resolveStorage(getenv,
		func() (string, error) { return userdata.Dir(func() (string, error) { return root, nil }, getenv) },
		testInstall(t))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, userdata.AppName)
	if where.DataDir != want || where.Database != filepath.Join(want, userdata.DatabaseFile) {
		t.Fatalf("default storage = %+v, want data %q", where, want)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(want); err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("default data directory mode = %v (%v), want 0700", info.Mode().Perm(), err)
		}
	}
}
