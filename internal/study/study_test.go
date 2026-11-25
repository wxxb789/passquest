package study

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestDatabasePath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	perUser := filepath.Join(root, "Pass Quest", "passquest.sqlite")
	config := func() (string, error) { return root, nil }
	noEnv := func(string) string { return "" }
	chosen := filepath.Join(root, "chosen.sqlite")
	envPath := filepath.Join(root, "environment.sqlite")
	tests := []struct {
		name   string
		flag   string
		getenv func(string) string
		want   string
	}{
		{"flag", chosen, noEnv, chosen},
		{"environment", "", func(name string) string { return map[string]string{"PASSQUEST_DB": envPath}[name] }, envPath},
		{"per-user default", "", noEnv, perUser},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DatabasePath(tt.flag, config, tt.getenv)
			if err != nil {
				t.Fatalf("DatabasePath = %v", err)
			}
			if got != tt.want {
				t.Fatalf("DatabasePath = %q, want %q", got, tt.want)
			}
		})
	}
	if got, err := DefaultDatabasePath(config); err != nil || got != perUser {
		t.Fatalf("DefaultDatabasePath = %q, %v; want %q", got, err, perUser)
	}
	failing := errors.New("no config directory")
	if _, err := DatabasePath("", func() (string, error) { return "", failing }, noEnv); !errors.Is(err, failing) {
		t.Fatalf("DatabasePath error = %v, want %v", err, failing)
	}
	if _, err := DefaultDatabasePath(func() (string, error) { return "", failing }); !errors.Is(err, failing) {
		t.Fatalf("DefaultDatabasePath error = %v, want %v", err, failing)
	}
}
