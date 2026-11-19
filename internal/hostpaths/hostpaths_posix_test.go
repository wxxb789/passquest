//go:build !windows

package hostpaths

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestResolveNodeSkipsANonExecutablePackagedRuntime pins the fall-through to
// PATH when the packaged runtime lost its executable bit (POSIX only: Windows
// has no such bit, so this file is not built there).
func TestResolveNodeSkipsANonExecutablePackagedRuntime(t *testing.T) {
	t.Parallel()
	resources := t.TempDir()
	packaged := filepath.Join(resources, "node", "bin", "node")
	if err := os.MkdirAll(filepath.Dir(packaged), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packaged, []byte("node\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := Source{Resources: resources, Getenv: func(string) string { return "" }}
	if got := ResolveNode(src, runtime.GOOS); got != "node" {
		t.Fatalf("ResolveNode = %q; want the PATH fallback %q", got, "node")
	}
}
