package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wxxb789/passquest/internal/agent"
	"github.com/wxxb789/passquest/internal/anki"
	"github.com/wxxb789/passquest/internal/search"
	"github.com/wxxb789/passquest/internal/tutor"
)

// writeResourceTree builds the tree an installed copy carries: the agent hosts
// under agent/, the Node runtime under node/. The hosts the transports resolve
// are absent on purpose, so each resolution fails naming its own candidate.
func writeResourceTree(t *testing.T) string {
	t.Helper()
	tree := t.TempDir()
	write := func(relative string) {
		t.Helper()
		path := filepath.Join(tree, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("// host\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("agent/find-host.mjs")
	write("node/bin/node")
	write("node/node.exe")
	return tree
}

// TestOptionsResourcesReachTheDefaultTransports pins the threading an installed
// copy depends on: the directory cmd/passquest-desktop resolves reaches every
// default host transport, and a host that is not in it is reported with the
// tree's own candidate.
func TestOptionsResourcesReachTheDefaultTransports(t *testing.T) {
	tree := writeResourceTree(t)
	opt := Options{
		Getenv:    func(string) string { return "" },
		Resources: tree,
	}.withDefaults()
	for _, carried := range []struct {
		name string
		got  string
	}{
		{"tutor", opt.TutorTransport.(tutor.HostTransport).Resources},
		{"anki", opt.AnkiTransport.(anki.HostTransport).Resources},
		{"search", opt.SearchTransport.(search.HostTransport).Resources},
		{"agent", opt.AgentRunner.(agent.HostRunner).Resources},
	} {
		if carried.got != tree {
			t.Fatalf("the default %s transport carries %q, Options.Resources is %q", carried.name, carried.got, tree)
		}
	}

	_, err := opt.SearchTransport.Search(context.Background(), search.Request{
		Endpoint:  "http://127.0.0.1:1/search",
		Query:     "polarity",
		Limit:     5,
		TimeoutMS: search.MinTimeoutMS,
	})
	if err == nil {
		t.Fatal("the search host resolved although the resources tree does not carry it")
	}
	want := filepath.Join(tree, "agent", "search-host.mjs")
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("the resolution error does not name %s: %v", want, err)
	}
}
