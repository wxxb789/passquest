package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// packagingConfig is the part of mygo.json the app's identity depends on;
// the icon, the resources and the output directory are staging and build
// details the Go code never reads.
type packagingConfig struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
	Version    string `json:"version"`
	Linux      struct {
		Maintainer string `json:"maintainer"`
		Comment    string `json:"comment"`
	} `json:"linux"`
	MacOS struct {
		MinimumSystemVersion string `json:"minimumSystemVersion"`
	} `json:"macos"`
}

var packagingVersion = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// TestPackagingIdentity pins mygo.json to the identity the app runs under. A
// package built with another name keeps the learner's data in another
// directory, and another identifier is another credential-store service, so a
// key stored in one build is missing in the other.
func TestPackagingIdentity(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "mygo.json"))
	if err != nil {
		t.Fatalf("read mygo.json: %v", err)
	}
	var config packagingConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse mygo.json: %v", err)
	}
	if config.Name != AppName {
		t.Errorf("mygo.json name = %q, want AppName %q", config.Name, AppName)
	}
	if config.Identifier != AppIdentifier {
		t.Errorf("mygo.json identifier = %q, want AppIdentifier %q", config.Identifier, AppIdentifier)
	}
	if !packagingVersion.MatchString(config.Version) {
		t.Errorf("mygo.json version = %q, want major.minor.patch", config.Version)
	}
	// The Linux desktop entry and the Debian and RPM packages take their
	// description from linux.comment (without one the entry has no Comment
	// line), and an empty linux.maintainer is derived by MyGo from the root
	// package.json, which is the harness's manifest rather than the app's.
	if config.Linux.Comment == "" {
		t.Error("mygo.json linux.comment is empty")
	}
	if config.Linux.Maintainer == "" {
		t.Error("mygo.json linux.maintainer is empty")
	}
	// The supported floor is a product decision, and MyGo copies
	// macos.minimumSystemVersion verbatim into the Info.plist's
	// LSMinimumSystemVersion (cmd/mygo/bundle.go:254); its own default is
	// 12.0. The effective floor is the higher of that key and the Mach-O's
	// `minos`, which the Go 1.27 linker writes as 13.0 — below the declared
	// 26.0 — so the plist key binds and raising minos is not needed (dyld
	// refuses a binary whose minos is newer than the running macOS).
	if config.MacOS.MinimumSystemVersion != "26.0" {
		t.Errorf("mygo.json macos.minimumSystemVersion = %q, want 26.0, the supported floor", config.MacOS.MinimumSystemVersion)
	}
}
