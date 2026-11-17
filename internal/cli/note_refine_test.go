package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/tutor"
)

func TestNoteRefineUsesProviderAndRecordsIt(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := readyNoteSource(t, db, clock)
	mustRun(t, db, clock, "", "tutor", "config", "--endpoint", "https://api.example.com/v1/chat/completions", "--model", "note-model", "--json")

	transport := &fakeTutorTransport{response: tutor.HostResponse{
		ProtocolVersion: tutor.ProtocolVersion, Status: "ok",
		Text:  `{"front":"refined front","back":"refined meaning\n\nExample: refined example"}`,
		Usage: &tutor.Usage{PromptTokens: 10, CompletionTokens: 4}, HTTPStatus: 200,
	}}
	doc := noteDraftDoc{}
	raw := mustNoteRun(t, db, clock, nil, transport, "key-not-real", "note", "propose", "--attempt", "1", "--refine", "--json")
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Draft.Origin != "refined" || !doc.Advisory || doc.Refinement == nil {
		t.Fatalf("refined doc = %+v", doc)
	}
	if doc.Plain.Front != "refined front" || !strings.Contains(doc.Plain.Back, "refined meaning") {
		t.Fatalf("refined content = %+v", doc.Plain)
	}
	if len(transport.requests) != 1 || transport.requests[0].Chat.Model != "note-model" {
		t.Fatalf("provider requests = %+v", transport.requests)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	refinements, err := st.NoteRefinements()
	if err != nil || len(refinements) != 1 || refinements[0].Status != "ok" || refinements[0].PromptTokens == nil || *refinements[0].PromptTokens != 10 {
		t.Fatalf("refinements = %+v, %v", refinements, err)
	}
	count, err := st.TutorCallCountSince("", tutorDayStart(clock.Now()))
	if err != nil || count != 1 {
		t.Fatalf("daily count = %d, %v", count, err)
	}
	if pending, err := st.PendingNoteOutbox(); err != nil || len(pending) != 0 {
		t.Fatalf("refinement created a note: %+v, %v", pending, err)
	}

	// A revoke withholds the refined text again on the next preview and refuses accept.
	mustRun(t, db, clock, "", "tutor", "revoke", sourceID, "--reviewer", "note-reviewer", "--json")
	revokedRaw := mustNoteRun(t, db, clock, nil, nil, "", "note", "preview", "--json")
	if strings.Contains(revokedRaw, "refined front") || strings.Contains(revokedRaw, "refined meaning") {
		t.Fatalf("revoked preview kept refined text:\n%s", revokedRaw)
	}
	revoked := notePreviewDoc{}
	if err := json.Unmarshal([]byte(revokedRaw), &revoked); err != nil {
		t.Fatal(err)
	}
	if len(revoked.Draft.Withheld) == 0 || revoked.Plain.Front != "" || revoked.Plain.Back != "" {
		t.Fatalf("revoked preview = %+v", revoked)
	}
	code, _, errOut := runNoteCLI(t, db, clock, nil, nil, "", "note", "accept", "--json")
	if code != 2 || !strings.Contains(errOut, "consent_required") {
		t.Fatalf("revoked accept = %d, %s", code, errOut)
	}
}

func TestNoteRefineRefusesRestrictedSourceWithoutConsent(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := noteSourceFixture(t, db, clock)
	mustRun(t, db, clock, "", "tutor", "config", "--endpoint", "https://api.example.com/v1/chat/completions", "--model", "note-model", "--json")

	transport := &fakeTutorTransport{response: fixedTutorResponse(`{"front":"kept","back":"kept meaning"}`)}
	code, _, errOut := runNoteCLI(t, db, clock, nil, transport, "key-not-real", "note", "propose",
		"--content", sourceID+"-p001", "--question", "q2", "--mark", "Sunday", "--refine", "--json")
	if code != 2 || !strings.Contains(errOut, "consent_required") {
		t.Fatalf("refine without consent = %d, %s", code, errOut)
	}
	if len(transport.requests) != 0 {
		t.Fatalf("refine without consent contacted the provider: %+v", transport.requests)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if refinements, err := st.NoteRefinements(); err != nil || len(refinements) != 0 {
		t.Fatalf("refinements without consent = %+v, %v", refinements, err)
	}
}

func TestNoteRefineRequiresVerifiedTextChat(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	readyNoteSource(t, db, clock)
	registry := modelRegistryFile(t, modelProvider("alpha", "https://alpha.example.com/v1/chat/completions"))
	env := map[string]string{provider.RegistryEnv: registry, "PASSQUEST_ALPHA_API_KEY": fakeTutorKey}
	if code, _, errOut := modelRun(t, db, clock, nil, env, "model", "use", "alpha/alpha-text", "--json"); code != 0 {
		t.Fatalf("model use exit = %d: %s", code, errOut)
	}

	transport := &fakeTutorTransport{response: fixedTutorResponse(`{"front":"refined","back":"refined meaning"}`)}
	code, _, errOut := modelRun(t, db, clock, transport, env, "note", "propose", "--attempt", "1", "--refine", "--json")
	if code != 2 || !strings.Contains(errOut, "capability_unverified") {
		t.Fatalf("unverified refine = %d, %s", code, errOut)
	}
	if len(transport.requests) != 0 {
		t.Fatalf("unverified refine contacted the provider: %+v", transport.requests)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	refinements, err := st.NoteRefinements()
	if err != nil || len(refinements) != 0 {
		st.Close()
		t.Fatalf("unverified refinements = %+v, %v", refinements, err)
	}
	st.Close()

	if code, _, errOut := modelRun(t, db, clock, transport, env, "model", "test", "alpha/alpha-text", "--json"); code != 0 {
		t.Fatalf("model test exit = %d: %s", code, errOut)
	}
	beforeRefine := len(transport.requests)
	code, refineRaw, errOut := modelRun(t, db, clock, transport, env, "note", "propose", "--attempt", "1", "--refine", "--json")
	if code != 0 {
		t.Fatalf("verified refine = %d, %s", code, errOut)
	}
	doc := noteDraftDoc{}
	if err := json.Unmarshal([]byte(refineRaw), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Draft.Origin != "refined" || doc.Refinement == nil || doc.Refinement.Model != "alpha-text" {
		t.Fatalf("verified refine = %+v", doc)
	}
	if got := len(transport.requests) - beforeRefine; got != 1 {
		t.Fatalf("verified refine requests = %d, want 1", got)
	}
}

// noteRefineProviderEnv writes a one-provider registry at endpoint, selects
// alpha/alpha-text, verifies its text.chat evidence and returns the environment
// plus the registry path so a test can change the endpoint later.
func noteRefineProviderEnv(t *testing.T, db string, clock *stepClock, transport *fakeTutorTransport, endpoint string) (map[string]string, string) {
	t.Helper()
	registry := modelRegistryFile(t, modelProvider("alpha", endpoint))
	env := map[string]string{provider.RegistryEnv: registry, "PASSQUEST_ALPHA_API_KEY": fakeTutorKey}
	if code, _, errOut := modelRun(t, db, clock, transport, env, "model", "use", "alpha/alpha-text", "--json"); code != 0 {
		t.Fatalf("model use exit = %d: %s", code, errOut)
	}
	if code, _, errOut := modelRun(t, db, clock, transport, env, "model", "test", "alpha/alpha-text", "--json"); code != 0 {
		t.Fatalf("model test exit = %d: %s", code, errOut)
	}
	return env, registry
}

func TestNoteRefineRefusesAProviderScopedRevoke(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := readyNoteSource(t, db, clock)
	transport := &fakeTutorTransport{response: fixedTutorResponse(`{"front":"refined","back":"refined meaning"}`)}
	env, _ := noteRefineProviderEnv(t, db, clock, transport, "https://alpha.example.com/v1/chat/completions")
	if code, _, errOut := modelRun(t, db, clock, nil, env, "tutor", "revoke", sourceID,
		"--provider", "alpha", "--reviewer", "note-reviewer", "--json"); code != 0 {
		t.Fatalf("provider revoke exit = %d: %s", code, errOut)
	}

	before := len(transport.requests)
	code, _, errOut := modelRun(t, db, clock, transport, env, "note", "propose", "--attempt", "1", "--refine", "--json")
	if code != 2 || !strings.Contains(errOut, "consent_required") || !strings.Contains(errOut, "--provider alpha") {
		t.Fatalf("provider-revoked refine = %d, %s", code, errOut)
	}
	if len(transport.requests) != before {
		t.Fatalf("provider-revoked refine contacted the provider: %+v", transport.requests)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if refinements, err := st.NoteRefinements(); err != nil || len(refinements) != 0 {
		t.Fatalf("provider-revoked refinements = %+v, %v", refinements, err)
	}
}

func TestNoteRefineUsesTheCurrentRegistryEndpoint(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	readyNoteSource(t, db, clock)
	const current = "https://current.example.com/v1/chat/completions"
	transport := &fakeTutorTransport{response: fixedTutorResponse(`{"front":"refined","back":"refined meaning"}`)}
	env, registry := noteRefineProviderEnv(t, db, clock, transport, "https://retired.example.com/v1/chat/completions")
	writeModelRegistry(t, registry, modelProvider("alpha", current))

	before := len(transport.requests)
	code, out, errOut := modelRun(t, db, clock, transport, env, "note", "propose", "--attempt", "1", "--refine", "--json")
	if code != 0 {
		t.Fatalf("refine after endpoint change = %d, %s", code, errOut)
	}
	doc := noteDraftDoc{}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Draft.Origin != "refined" || doc.Refinement == nil {
		t.Fatalf("refine after endpoint change = %+v", doc)
	}
	if len(transport.requests) != before+1 {
		t.Fatalf("refine requests = %d, want 1", len(transport.requests)-before)
	}
	if got := transport.requests[before].Endpoint; got != current {
		t.Fatalf("refine endpoint = %q, want %q", got, current)
	}
}

func TestNoteRefineRefusesAGrantForADifferentProvider(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := noteSourceFixture(t, db, clock)
	if code, _, errOut := modelRun(t, db, clock, nil, nil, "tutor", "consent", sourceID,
		"--provider", "beta", "--reviewer", "note-reviewer", "--json"); code != 0 {
		t.Fatalf("beta consent exit = %d: %s", code, errOut)
	}
	transport := &fakeTutorTransport{response: fixedTutorResponse(`{"front":"refined","back":"refined meaning"}`)}
	env, _ := noteRefineProviderEnv(t, db, clock, transport, "https://alpha.example.com/v1/chat/completions")

	before := len(transport.requests)
	code, _, errOut := modelRun(t, db, clock, transport, env, "note", "propose",
		"--content", sourceID+"-p001", "--question", "q2", "--mark", "Sunday", "--refine", "--json")
	if code != 2 || !strings.Contains(errOut, "consent_required") {
		t.Fatalf("grant for another provider = %d, %s", code, errOut)
	}
	if len(transport.requests) != before {
		t.Fatalf("grant for another provider contacted the provider: %+v", transport.requests)
	}
}

func TestNoteRefineRejectsAnAnswerRevealingResponse(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := noteSourceFixture(t, db, clock)
	mustRun(t, db, clock, "", "question", "add", sourceID, "1", "--file", fixtureFile(t, "question-q1.json"), "--json")
	mustRun(t, db, clock, "", "question", "review", sourceID, "q1", "--answer", "B", "--reviewer", "note-reviewer", "--json")
	noteConsent(t, db, clock, sourceID, "note-reviewer")
	mustRun(t, db, clock, "", "tutor", "config", "--endpoint", "https://api.example.com/v1/chat/completions", "--model", "note-model", "--json")

	transport := &fakeTutorTransport{response: fixedTutorResponse(
		`{"front":"At 4 p.m.","back":"The library closes at 4 p.m. on Saturdays."}`)}
	code, _, errOut := runNoteCLI(t, db, clock, nil, transport, "key-not-real", "note", "propose",
		"--content", sourceID+"-p001", "--question", "q1", "--mark", "closes", "--refine", "--json")
	if code != 3 || !strings.Contains(errOut, "invalid_response") || !strings.Contains(errOut, "reveal") {
		t.Fatalf("answer-revealing refinement = %d, %s", code, errOut)
	}
	if len(transport.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(transport.requests))
	}
	body := transport.requests[0].Body
	for _, secret := range []string{"At 4 p.m.", "closes at 4 p.m. on Saturdays"} {
		if strings.Contains(body, secret) {
			t.Fatalf("unanswered refinement request carried the key (%q): %s", secret, body)
		}
	}

	previewRaw := mustNoteRun(t, db, clock, nil, nil, "", "note", "preview", "--json")
	for _, secret := range []string{"At 4 p.m.", "closes at 4 p.m.", "The notice states 4 p.m."} {
		if strings.Contains(previewRaw, secret) {
			t.Fatalf("preview leaked %q after a rejected refinement:\n%s", secret, previewRaw)
		}
	}
	preview := notePreviewDoc{}
	if err := json.Unmarshal([]byte(previewRaw), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Plain.Front != "closes" || preview.Draft.Origin != "deterministic" {
		t.Fatalf("deterministic draft was lost: %+v", preview)
	}
	acceptedRaw := mustNoteRun(t, db, clock, nil, nil, "", "note", "accept", "--json")
	if strings.Contains(acceptedRaw, "At 4 p.m.") {
		t.Fatalf("accepted note leaked the key:\n%s", acceptedRaw)
	}

	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	refinements, err := st.NoteRefinements()
	if err != nil || len(refinements) != 1 || refinements[0].Status != "error" || refinements[0].ErrorCode != "invalid_response" {
		t.Fatalf("rejected refinement record = %+v, %v", refinements, err)
	}
}

func TestNotePreviewIgnoresAStoredAnswerRevealingRefinement(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	sourceID := noteSourceFixture(t, db, clock)
	mustRun(t, db, clock, "", "question", "add", sourceID, "1", "--file", fixtureFile(t, "question-q1.json"), "--json")
	mustRun(t, db, clock, "", "question", "review", sourceID, "q1", "--answer", "B", "--reviewer", "note-reviewer", "--json")
	noteConsent(t, db, clock, sourceID, "note-reviewer")

	draft := noteDraftDoc{}
	raw := mustNoteRun(t, db, clock, nil, nil, "", "note", "propose",
		"--content", sourceID+"-p001", "--question", "q1", "--mark", "closes", "--json")
	if err := json.Unmarshal([]byte(raw), &draft); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	observed, _, err := st.NoteDraftByID(draft.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetNoteDraftRefinement(observed, 1, "hostile-model", "At 4 p.m.", "The library closes at 4 p.m. on Saturdays."); err != nil {
		t.Fatalf("store hostile refinement: %v", err)
	}
	st.Close()

	previewRaw := mustNoteRun(t, db, clock, nil, nil, "", "note", "preview", "--json")
	if strings.Contains(previewRaw, "At 4 p.m.") {
		t.Fatalf("stored refinement leaked the key:\n%s", previewRaw)
	}
	preview := notePreviewDoc{}
	if err := json.Unmarshal([]byte(previewRaw), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Plain.Front != "closes" || preview.Draft.Origin != "deterministic" {
		t.Fatalf("stored refinement was not discarded: %+v", preview)
	}
}

func TestNoteRefineFailureKeepsDeterministicDraft(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	readyNoteSource(t, db, clock)
	mustRun(t, db, clock, "", "tutor", "config", "--endpoint", "https://api.example.com/v1/chat/completions", "--model", "note-model", "--json")

	transport := &fakeTutorTransport{response: fixedTutorError("timeout", "provider timed out")}
	code, _, errOut := runNoteCLI(t, db, clock, nil, transport, "key-not-real", "note", "propose", "--attempt", "1", "--refine", "--json")
	if code != 3 || !strings.Contains(errOut, "timeout") {
		t.Fatalf("failed refinement = %d, %s", code, errOut)
	}
	preview := mustNoteRun(t, db, clock, nil, nil, "", "note", "preview", "--json")
	if !strings.Contains(preview, "No Sunday opening time") {
		t.Fatalf("deterministic draft was lost:\n%s", preview)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	refinements, err := st.NoteRefinements()
	if err != nil || len(refinements) != 1 || refinements[0].Status != "error" || refinements[0].ErrorCode != "timeout" {
		t.Fatalf("refinements = %+v, %v", refinements, err)
	}
}
