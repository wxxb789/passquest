package userdata

import (
	"errors"
	"path/filepath"
	"testing"
)

func envOf(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func TestDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	failing := errors.New("no config directory")
	tests := []struct {
		name      string
		env       map[string]string
		configErr error
		want      string
	}{
		{"override wins, trimmed", map[string]string{DataEnv: "  " + filepath.Join(root, "chosen") + "\t"}, nil, filepath.Join(root, "chosen")},
		{"override wins over a failing config directory", map[string]string{DataEnv: filepath.Join(root, "chosen")}, failing, filepath.Join(root, "chosen")},
		{"app-owned default joins the name", nil, nil, filepath.Join(root, AppName)},
		{"whitespace-only override is ignored", map[string]string{DataEnv: " \t "}, nil, filepath.Join(root, AppName)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Dir(func() (string, error) { return root, tt.configErr }, envOf(tt.env))
			if err != nil {
				t.Fatalf("Dir = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Dir = %q, want %q", got, tt.want)
			}
		})
	}
	if _, err := Dir(func() (string, error) { return "", failing }, envOf(nil)); !errors.Is(err, failing) {
		t.Fatalf("Dir error = %v, want %v", err, failing)
	}
}

func TestDatabasePath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	failing := errors.New("no config directory")
	config := func() (string, error) { return root, nil }
	envPath := filepath.Join(root, "env.sqlite")
	tests := []struct {
		name string
		flag string
		env  map[string]string
		want string
	}{
		{"flag wins untrimmed", "  chosen.sqlite  ", map[string]string{DatabaseEnv: envPath}, "  chosen.sqlite  "},
		{"environment when the flag is whitespace", " \t", map[string]string{DatabaseEnv: " " + envPath + " "}, envPath},
		{"default is the database inside the per-user directory", "", nil, filepath.Join(root, AppName, DatabaseFile)},
		{"data directory override moves the default", "", map[string]string{DataEnv: filepath.Join(root, "moved")}, filepath.Join(root, "moved", DatabaseFile)},
		{"whitespace-only environment is ignored", "", map[string]string{DatabaseEnv: " ", DataEnv: " "}, filepath.Join(root, AppName, DatabaseFile)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DatabasePath(tt.flag, config, envOf(tt.env))
			if err != nil {
				t.Fatalf("DatabasePath = %v", err)
			}
			if got != tt.want {
				t.Fatalf("DatabasePath = %q, want %q", got, tt.want)
			}
		})
	}

	unconsulted := func() (string, error) {
		t.Fatal("config directory consulted despite an override")
		return "", nil
	}
	if got, err := DatabasePath("flag.sqlite", unconsulted, envOf(nil)); err != nil || got != "flag.sqlite" {
		t.Fatalf("DatabasePath(flag) = %q, %v", got, err)
	}
	if got, err := DatabasePath("", unconsulted, envOf(map[string]string{DatabaseEnv: envPath})); err != nil || got != envPath {
		t.Fatalf("DatabasePath(env) = %q, %v", got, err)
	}
	if got, err := DatabasePath("", unconsulted, envOf(map[string]string{DataEnv: root + "/moved"})); err != nil || got != filepath.Join(root, "moved", DatabaseFile) {
		t.Fatalf("DatabasePath(data env) = %q, %v", got, err)
	}
	if _, err := DatabasePath("", func() (string, error) { return "", failing }, envOf(nil)); !errors.Is(err, failing) {
		t.Fatalf("DatabasePath error = %v, want %v", err, failing)
	}
}
