package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/tutor"
)

func modelRegistryFile(t *testing.T, providers ...map[string]any) string {
	t.Helper()
	doc := map[string]any{"version": 1, "providers": providers}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "registry.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func modelProvider(id, endpoint string) map[string]any {
	return map[string]any{
		"id":             id,
		"name":           strings.ToUpper(id),
		"endpoint":       endpoint,
		"endpoint_shape": provider.EndpointShape,
		"credential_env": "PASSQUEST_" + strings.ToUpper(id) + "_API_KEY",
		"models": []map[string]any{
			{"id": id + "-text", "default": true, "capabilities": []string{"text.chat"}},
		},
	}
}

func modelRun(t *testing.T, db string, clock *stepClock, transport tutor.Transport, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	opt := Options{
		Stdin:          strings.NewReader(""),
		Stdout:         &stdout,
		Stderr:         &stderr,
		Now:            clock.Now,
		NewSession:     func() string { return "sess-test" },
		Getenv:         func(key string) string { return env[key] },
		TutorTransport: transport,
	}
	code := Run(append(args, "--db", db), opt)
	return code, stdout.String(), stderr.String()
}

func decodeModelList(t *testing.T, raw string) modelListDoc {
	t.Helper()
	var doc modelListDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode model list: %v\n%s", err, raw)
	}
	return doc
}

func TestModelListSeparatesProviderModelCapabilityAndAuthorization(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	registry := modelRegistryFile(t, modelProvider("alpha", "https://alpha.example.com/v1/chat/completions"))
	code, out, errOut := modelRun(t, db, clock, nil, map[string]string{provider.RegistryEnv: registry},
		"model", "list", "--json")
	if code != 0 {
		t.Fatalf("model list exit = %d: %s", code, errOut)
	}
	doc := decodeModelList(t, out)
	if len(doc.Providers) != 1 || doc.Providers[0].ID != "alpha" || doc.Providers[0].Auth != "no_credential" {
		t.Fatalf("provider row = %+v", doc.Providers)
	}
	model := doc.Providers[0].Models[0]
	if model.ID != "alpha-text" || len(model.Capabilities) != len(provider.Capabilities) {
		t.Fatalf("model row = %+v", model)
	}
	capabilities := map[string]modelCapabilityView{}
	for _, capability := range model.Capabilities {
		capabilities[capability.ID] = capability
	}
	chat, found := capabilities[string(provider.CapTextChat)]
	if !found || chat.State != "declared" || !chat.Declared {
		t.Fatalf("text.chat capability = %+v", chat)
	}
	search, found := capabilities[string(provider.CapSearch)]
	if !found || search.State != "unsupported" {
		t.Fatalf("undeclared search capability = %+v, found = %t", search, found)
	}
}

func TestModelUseLeavesPracticeStateByteIdentical(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	registry := modelRegistryFile(t, modelProvider("alpha", "https://alpha.example.com/v1/chat/completions"))
	env := map[string]string{provider.RegistryEnv: registry}
	mustRun(t, db, clock, "", "practice", "--json", "--answer", "B")
	_, reportBefore, _ := modelRun(t, db, clock, nil, env, "report", "--json")
	_, nextBefore, _ := modelRun(t, db, clock, nil, env, "next", "--json")

	code, out, errOut := modelRun(t, db, clock, nil, env, "model", "use", "alpha/alpha-text", "--json")
	if code != 0 {
		t.Fatalf("model use exit = %d: %s", code, errOut)
	}
	var useDoc modelUseDoc
	if err := json.Unmarshal([]byte(out), &useDoc); err != nil {
		t.Fatal(err)
	}
	if !useDoc.Active.Configured || useDoc.Active.Provider != "alpha" || useDoc.Active.Model != "alpha-text" {
		t.Fatalf("active model = %+v", useDoc.Active)
	}
	if useDoc.TextChat.State != "declared" || useDoc.TextChat.Verified {
		t.Fatalf("switch claimed verification: %+v", useDoc.TextChat)
	}
	listCode, listOut, _ := modelRun(t, db, clock, nil, env, "model", "list", "--json")
	if listCode != 0 {
		t.Fatalf("model list exit = %d", listCode)
	}
	list := decodeModelList(t, listOut)
	if !list.Active.Configured || list.Active.Provider != "alpha" || !list.Providers[0].Models[0].Active {
		t.Fatalf("active model not shown: %+v", list.Active)
	}
	_, reportAfter, _ := modelRun(t, db, clock, nil, env, "report", "--json")
	_, nextAfter, _ := modelRun(t, db, clock, nil, env, "next", "--json")
	if reportBefore != reportAfter || nextBefore != nextAfter {
		t.Fatal("model use changed report or next output")
	}
}

func TestModelTestRecordsVerifiedAndTypedFailure(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	registry := modelRegistryFile(t, modelProvider("alpha", "https://alpha.example.com/v1/chat/completions"))
	env := map[string]string{
		provider.RegistryEnv:      registry,
		"PASSQUEST_ALPHA_API_KEY": fakeTutorKey,
	}
	transport := &fakeTutorTransport{response: fixedTutorResponse("OK")}
	code, out, errOut := modelRun(t, db, clock, transport, env, "model", "test", "alpha/alpha-text", "--json")
	if code != 0 {
		t.Fatalf("model test exit = %d: %s", code, errOut)
	}
	var doc modelTestDoc
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Result != "verified" || doc.CheckedAt == "" || len(transport.requests) != 1 {
		t.Fatalf("verified doc = %+v, requests = %d", doc, len(transport.requests))
	}
	st, err := openDatabase(db, Options{})
	if err != nil {
		t.Fatal(err)
	}
	verification, found, err := st.LatestProviderVerification("alpha", "alpha-text", string(provider.CapTextChat))
	st.Close()
	if err != nil || !found || verification.Status != "verified" {
		t.Fatalf("stored verification = %+v, %v, %v", verification, found, err)
	}

	transport.response = fixedTutorError("provider_error", "model not found")
	code, out, errOut = modelRun(t, db, clock, transport, env, "model", "test", "alpha/alpha-text", "--json")
	if code != 3 {
		t.Fatalf("failed model test exit = %d: %s", code, errOut)
	}
	var failed modelTestDoc
	if err := json.Unmarshal([]byte(out), &failed); err != nil {
		t.Fatal(err)
	}
	if failed.Result != "failed:model_missing" || failed.Failure == nil || failed.Failure.Action == "" {
		t.Fatalf("failed doc = %+v", failed)
	}
}

func TestModelTestNoCredentialIsRecordedWithoutACall(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	registry := modelRegistryFile(t, modelProvider("alpha", "https://alpha.example.com/v1/chat/completions"))
	env := map[string]string{provider.RegistryEnv: registry}
	transport := &fakeTutorTransport{response: fixedTutorResponse("OK")}
	code, out, _ := modelRun(t, db, clock, transport, env, "model", "test", "alpha/alpha-text", "--json")
	if code != 2 {
		t.Fatalf("no-credential exit = %d", code)
	}
	var doc modelTestDoc
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Result != "failed:no_credential" || doc.Failure == nil || !strings.Contains(doc.Failure.Action, "PASSQUEST_ALPHA_API_KEY") {
		t.Fatalf("no-credential doc = %+v", doc)
	}
	if len(transport.requests) != 0 {
		t.Fatalf("no-credential test made %d provider calls", len(transport.requests))
	}
}

func TestModelTestBudgetRefusesEachProviderAtItsOwnLimit(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	registry := modelRegistryFile(t,
		modelProvider("alpha", "https://alpha.example.com/v1/chat/completions"),
		modelProvider("beta", "https://beta.example.com/v1/chat/completions"),
	)
	env := map[string]string{
		provider.RegistryEnv:      registry,
		"PASSQUEST_ALPHA_API_KEY": fakeTutorKey,
		"PASSQUEST_BETA_API_KEY":  fakeTutorKey,
	}
	if code, _, errOut := modelRun(t, db, clock, nil, env, "model", "use", "alpha/alpha-text", "--json"); code != 0 {
		t.Fatalf("model use exit = %d: %s", code, errOut)
	}
	if code, _, errOut := modelRun(t, db, clock, nil, env, "tutor", "config", "--daily-calls", "1", "--json"); code != 0 {
		t.Fatalf("tutor config exit = %d: %s", code, errOut)
	}
	transport := &fakeTutorTransport{response: fixedTutorResponse("OK")}
	if code, _, errOut := modelRun(t, db, clock, transport, env, "model", "test", "alpha/alpha-text", "--json"); code != 0 {
		t.Fatalf("alpha test exit = %d: %s", code, errOut)
	}
	if code, _, _ := modelRun(t, db, clock, transport, env, "model", "test", "alpha/alpha-text", "--json"); code != 2 {
		t.Fatalf("alpha over-limit exit = %d, want 2", code)
	}
	if code, _, errOut := modelRun(t, db, clock, transport, env, "model", "test", "beta/beta-text", "--json"); code != 0 {
		t.Fatalf("beta test exit = %d: %s", code, errOut)
	}
	code, out, errOut := modelRun(t, db, clock, nil, env, "model", "budget", "--json")
	if code != 0 {
		t.Fatalf("model budget exit = %d: %s", code, errOut)
	}
	var budget modelBudgetDoc
	if err := json.Unmarshal([]byte(out), &budget); err != nil {
		t.Fatal(err)
	}
	if budget.Budget.TotalCalls != 2 || budget.Budget.TotalLimit != 2 || budget.Budget.ProviderCount != 2 {
		t.Fatalf("budget = %+v", budget.Budget)
	}
	if budget.Budget.Ceiling != "2 providers × 1 = 2" {
		t.Fatalf("ceiling = %q", budget.Budget.Ceiling)
	}
}

func TestModelTestTransportFailureIsEndpointUnreachable(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	registry := modelRegistryFile(t, modelProvider("alpha", "https://alpha.example.com/v1/chat/completions"))
	env := map[string]string{
		provider.RegistryEnv:      registry,
		"PASSQUEST_ALPHA_API_KEY": fakeTutorKey,
	}
	transport := &fakeTutorTransport{response: tutor.HostResponse{
		ProtocolVersion: tutor.ProtocolVersion,
		Status:          "error",
		HTTPStatus:      0,
		Error:           &tutor.HostError{Code: "network_error", Message: "connection refused"},
	}}
	code, out, _ := modelRun(t, db, clock, transport, env, "model", "test", "alpha/alpha-text", "--json")
	if code != 3 {
		t.Fatalf("transport failure exit = %d", code)
	}
	var doc modelTestDoc
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Result != "failed:endpoint_unreachable" || doc.Failure == nil || doc.Failure.Action == "" {
		t.Fatalf("transport failure = %+v", doc)
	}
}

func TestModelListRendersDeclaredOptionalCapabilitiesAsUnverified(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	providerDoc := modelProvider("alpha", "https://alpha.example.com/v1/chat/completions")
	providerDoc["models"] = []map[string]any{{
		"id": "alpha-text", "default": true,
		"capabilities": []string{"text.chat", "search", "speech.synthesis"},
	}}
	registry := modelRegistryFile(t, providerDoc)
	code, out, errOut := modelRun(t, db, clock, nil, map[string]string{provider.RegistryEnv: registry},
		"model", "list", "--json")
	if code != 0 {
		t.Fatalf("model list exit = %d: %s", code, errOut)
	}
	doc := decodeModelList(t, out)
	capabilities := map[string]modelCapabilityView{}
	for _, capability := range doc.Providers[0].Models[0].Capabilities {
		capabilities[capability.ID] = capability
	}
	for _, id := range []provider.Capability{provider.CapTextChat, provider.CapSearch, provider.CapSpeechSynthesis} {
		capability, found := capabilities[string(id)]
		if !found {
			t.Fatalf("%s row is absent", id)
		}
		if id == provider.CapSearch || id == provider.CapSpeechSynthesis {
			if capability.State != "declared" || capability.Verified {
				t.Fatalf("%s = %+v; want declared, unverified", id, capability)
			}
		}
	}
}

func TestModelTestTimestampUsesTheClock(t *testing.T) {
	db := newDB(t)
	base := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	clock := &stepClock{base: base}
	registry := modelRegistryFile(t, modelProvider("alpha", "https://alpha.example.com/v1/chat/completions"))
	env := map[string]string{
		provider.RegistryEnv:      registry,
		"PASSQUEST_ALPHA_API_KEY": fakeTutorKey,
	}
	transport := &fakeTutorTransport{response: fixedTutorResponse("OK")}
	code, out, _ := modelRun(t, db, clock, transport, env, "model", "test", "alpha/alpha-text", "--json")
	if code != 0 {
		t.Fatalf("model test exit = %d", code)
	}
	var doc modelTestDoc
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	parsed, err := time.Parse(time.RFC3339, doc.CheckedAt)
	if err != nil || parsed.Before(base) || !parsed.Before(base.Add(time.Minute)) {
		t.Fatalf("checked_at = %q (%v); want within one minute of %s", doc.CheckedAt, err, base)
	}
}

func TestModelListKeepsVerificationPerModel(t *testing.T) {
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	providerDoc := modelProvider("alpha", "https://alpha.example.com/v1/chat/completions")
	providerDoc["models"] = []map[string]any{
		{"id": "alpha-one", "default": true, "capabilities": []string{"text.chat"}},
		{"id": "alpha-two", "capabilities": []string{"text.chat"}},
	}
	registry := modelRegistryFile(t, providerDoc)
	env := map[string]string{
		provider.RegistryEnv:      registry,
		"PASSQUEST_ALPHA_API_KEY": fakeTutorKey,
	}
	transport := &fakeTutorTransport{response: fixedTutorResponse("OK")}
	if code, _, errOut := modelRun(t, db, clock, transport, env, "model", "test", "alpha/alpha-one", "--json"); code != 0 {
		t.Fatalf("model test exit = %d: %s", code, errOut)
	}
	code, out, errOut := modelRun(t, db, clock, nil, env, "model", "list", "--json")
	if code != 0 {
		t.Fatalf("model list exit = %d: %s", code, errOut)
	}
	doc := decodeModelList(t, out)
	states := map[string]string{}
	for _, model := range doc.Providers[0].Models {
		for _, capability := range model.Capabilities {
			if capability.ID == string(provider.CapTextChat) {
				states[model.ID] = capability.State
			}
		}
	}
	if states["alpha-one"] != "verified" || states["alpha-two"] != "declared" {
		t.Fatalf("per-model verification = %+v; want alpha-one verified, alpha-two declared", states)
	}
}
