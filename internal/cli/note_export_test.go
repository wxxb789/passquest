package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wxxb789/passquest/internal/provider"
)

func TestNoteExportRechecksConsent(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := readyNoteSource(t, db, clock)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")
	mustRun(t, db, clock, "", "tutor", "revoke", sourceID, "--reviewer", "note-reviewer", "--json")

	out := filepath.Join(t.TempDir(), "revoked.tsv")
	code, _, errOut := runNoteCLI(t, db, clock, nil, nil, "", "note", "export", "--out", out, "--json")
	if code != 2 || !strings.Contains(errOut, "consent_required") {
		t.Fatalf("revoked export = %d, %s", code, errOut)
	}
	if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("revoked export wrote a file: %v", err)
	}

	noteConsent(t, db, clock, sourceID, "note-reviewer")
	exported := noteExportDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "export", "--out", out, "--json")), &exported); err != nil {
		t.Fatal(err)
	}
	if exported.Notes != 1 {
		t.Fatalf("re-consented export = %+v", exported)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("re-consented export did not write: %v", err)
	}
}

func TestNoteExportNeedsGlobalConsentNotProviderScope(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := readyNoteSource(t, db, clock)
	mustNoteRun(t, db, clock, nil, nil, "", "note", "propose", "--attempt", "1", "--json")
	mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")

	// Replace the global grant with a provider-scoped one: alpha may see the
	// source for inference, but no destination-wide export consent remains.
	mustRun(t, db, clock, "", "tutor", "revoke", sourceID, "--reviewer", "note-reviewer", "--json")
	mustRun(t, db, clock, "", "tutor", "consent", sourceID, "--reviewer", "note-reviewer", "--provider", "alpha", "--json")

	out := filepath.Join(t.TempDir(), "provider-scoped.tsv")
	code, _, errOut := runNoteCLI(t, db, clock, nil, nil, "", "note", "export", "--out", out, "--json")
	if code != 2 || !strings.Contains(errOut, "consent_required") {
		t.Fatalf("provider-scoped export = %d, %s", code, errOut)
	}
	if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("provider-scoped export wrote a file: %v", err)
	}
	ankiTransport := newFakeAnki()
	code, _, errOut = runNoteCLI(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")
	if code != 2 || !strings.Contains(errOut, "consent_required") || len(ankiTransport.requests) != 0 {
		t.Fatalf("provider-scoped push = %d, %s, requests %d", code, errOut, len(ankiTransport.requests))
	}

	// The same alpha grant still authorizes inference for that provider.
	registry := modelRegistryFile(t, modelProvider("alpha", "https://alpha.example.com/v1/chat/completions"))
	env := map[string]string{provider.RegistryEnv: registry, "PASSQUEST_ALPHA_API_KEY": fakeTutorKey}
	transport := &fakeTutorTransport{response: fixedTutorResponse("The answer follows from the Saturday sentence.")}
	if code, _, errOut := modelRun(t, db, clock, nil, env, "model", "use", "alpha/alpha-text", "--json"); code != 0 {
		t.Fatalf("model use exit = %d: %s", code, errOut)
	}
	if code, _, errOut := modelRun(t, db, clock, transport, env, "model", "test", "alpha/alpha-text", "--json"); code != 0 {
		t.Fatalf("model test exit = %d: %s", code, errOut)
	}
	code, askRaw, errOut := modelRun(t, db, clock, transport, env, "tutor", "ask", "--attempt", "1", "--json")
	if code != 0 {
		t.Fatalf("provider-scoped tutor ask = %d, %s", code, errOut)
	}
	ask := decodeTutorAdvisory(t, askRaw)
	if ask.Call.Provider != "alpha" || !ask.Context.ConsentGranted || !ask.Call.SourceTextShared {
		t.Fatalf("provider-scoped ask context = %+v, call = %+v", ask.Context, ask.Call)
	}

	// A global grant is what authorizes the note destinations.
	noteConsent(t, db, clock, sourceID, "note-reviewer")
	exported := noteExportDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, nil, nil, "", "note", "export", "--out", out, "--json")), &exported); err != nil {
		t.Fatal(err)
	}
	if exported.Notes != 1 {
		t.Fatalf("globally consented export = %+v", exported)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("globally consented export did not write: %v", err)
	}
	pushed := notePushDoc{}
	if err := json.Unmarshal([]byte(mustNoteRun(t, db, clock, ankiTransport, nil, "", "note", "push", "--json")), &pushed); err != nil {
		t.Fatal(err)
	}
	if pushed.State != "pushed" || pushed.Counts.Pushed != 1 {
		t.Fatalf("globally consented push = %+v", pushed)
	}
}
