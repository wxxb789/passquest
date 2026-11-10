package anki

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeEndpoint(t *testing.T) {
	cases := []struct {
		raw  string
		want string
		ok   bool
	}{
		{"", "http://127.0.0.1:8765", true},
		{"http://127.0.0.1:8765", "http://127.0.0.1:8765", true},
		{"http://localhost:8765/", "http://localhost:8765", true},
		{"http://[::1]:8765", "http://[::1]:8765", true},
		{"https://127.0.0.1:8765", "https://127.0.0.1:8765", true},
		{"http://127.0.0.2:8765", "http://127.0.0.2:8765", true},
		{"http://example.com:8765", "", false},
		{"http://10.0.0.2:8765", "", false},
		{"http://user:pass@127.0.0.1:8765", "", false},
		{"http://127.0.0.1:8765/anki", "", false},
		{"http://127.0.0.1:8765/?x=1", "", false},
		{"ftp://127.0.0.1", "", false},
	}
	for _, c := range cases {
		got, err := NormalizeEndpoint(c.raw)
		if c.ok && (err != nil || got != c.want) {
			t.Errorf("NormalizeEndpoint(%q) = %q, %v, want %q", c.raw, got, err, c.want)
		}
		if !c.ok && err == nil {
			t.Errorf("NormalizeEndpoint(%q) accepted a remote or malformed endpoint", c.raw)
		}
	}
}

func TestValidateRequestAndResponse(t *testing.T) {
	req := HostRequest{
		ProtocolVersion: ProtocolVersion,
		Endpoint:        DefaultEndpoint,
		TimeoutMS:       1000,
		Actions: []Action{
			{Action: "version"},
			{Action: "addNote", Params: AddNoteParams("Pass Quest", "PassQuest Note", map[string]string{"Front": "f"}, []string{"passquest"})},
		},
	}
	if err := ValidateRequest(req); err != nil {
		t.Fatalf("ValidateRequest: %v", err)
	}
	bad := req
	bad.Actions = []Action{{Action: "deleteEverything"}}
	if err := ValidateRequest(bad); err == nil {
		t.Fatal("ValidateRequest accepted an unknown action")
	}
	bad = req
	bad.TimeoutMS = 0
	if err := ValidateRequest(bad); err == nil {
		t.Fatal("ValidateRequest accepted a zero timeout")
	}
	resp := HostResponse{
		ProtocolVersion: ProtocolVersion,
		State:           "ok",
		Results: []ActionResult{
			{Action: "version", State: "ok", Result: json.RawMessage("6")},
			{Action: "addNote", State: "ok", Result: json.RawMessage("101")},
		},
	}
	if err := ValidateResponse(req, resp); err != nil {
		t.Fatalf("ValidateResponse: %v", err)
	}
	badResp := resp
	badResp.Results = resp.Results[:1]
	if err := ValidateResponse(req, badResp); err == nil {
		t.Fatal("ValidateResponse accepted a result-count mismatch")
	}
	badResp = resp
	badResp.Results[1].Action = "version"
	if err := ValidateResponse(req, badResp); err == nil {
		t.Fatal("ValidateResponse accepted an action-order mismatch")
	}
	badResp = resp
	badResp.State = "maybe"
	if err := ValidateResponse(req, badResp); err == nil {
		t.Fatal("ValidateResponse accepted an unknown state")
	}
	badResp = resp
	badResp.Results[1].Error = string(make([]byte, 501))
	if err := ValidateResponse(req, badResp); err == nil {
		t.Fatal("ValidateResponse accepted an oversized error")
	}
}

func TestClassifiers(t *testing.T) {
	for _, message := range []string{
		"cannot create note because it is a duplicate",
		"Note was a duplicate",
	} {
		if !IsDuplicateError(message) {
			t.Fatalf("IsDuplicateError(%q) = false", message)
		}
	}
	if IsDuplicateError("note was not found") {
		t.Fatal("IsDuplicateError matched a not-found message")
	}
	for _, message := range []string{"note was not found", "requested note not found"} {
		if !IsNotFoundError(message) {
			t.Fatalf("IsNotFoundError(%q) = false", message)
		}
	}
}

func TestPayloadBuilders(t *testing.T) {
	add := AddNoteParams("Pass Quest", "PassQuest Note", map[string]string{
		"Front": "f", "Back": "b", "Tactic": "t", "Source": "s",
	}, []string{"passquest"})
	note := add["note"].(map[string]any)
	if note["deckName"] != "Pass Quest" || note["modelName"] != "PassQuest Note" {
		t.Fatalf("add note identity = %#v", note)
	}
	options := note["options"].(map[string]any)
	if options["allowDuplicate"] != false {
		t.Fatalf("allowDuplicate = %v", options["allowDuplicate"])
	}
	update := UpdateNoteParams(101, map[string]string{"Front": "f"})
	updateNote := update["note"].(map[string]any)
	if updateNote["id"] != int64(101) {
		t.Fatalf("update id = %#v", updateNote["id"])
	}
	model := CreateModelParams(ModelParams{
		Name: "PassQuest Note", Fields: []string{"Front", "Back"},
		CSS: "css", FrontTemplate: "front", BackTemplate: "back",
	})
	if model["modelName"] != "PassQuest Note" || len(model["inOrderFields"].([]string)) != 2 {
		t.Fatalf("model params = %#v", model)
	}
	templates := model["cardTemplates"].([]map[string]string)
	if len(templates) != 1 || templates[0]["Name"] != "PassQuest" {
		t.Fatalf("card templates = %#v", templates)
	}
	if deck := CreateDeckParams("Pass Quest"); deck["deck"] != "Pass Quest" {
		t.Fatalf("deck params = %#v", deck)
	}
	if info := NotesInfoParams([]int64{1, 2}); len(info["notes"].([]int64)) != 2 {
		t.Fatalf("notesInfo params = %#v", info)
	}
}

func TestDecodeResults(t *testing.T) {
	value, err := DecodeInt(json.RawMessage("42"))
	if err != nil || value != 42 {
		t.Fatalf("DecodeInt = %d, %v", value, err)
	}
	if _, err := DecodeInt(json.RawMessage(`"x"`)); err == nil {
		t.Fatal("DecodeInt accepted a string")
	}
	names, err := DecodeStringList(json.RawMessage(`["a","b"]`))
	if err != nil || len(names) != 2 {
		t.Fatalf("DecodeStringList = %v, %v", names, err)
	}
	if names, err := DecodeStringList(json.RawMessage("null")); err != nil || names != nil {
		t.Fatalf("DecodeStringList(null) = %v, %v", names, err)
	}
	rows, err := DecodeNotesInfo(json.RawMessage(`[{"noteId":7,"modelName":"PassQuest Note","fields":{"Front":{"value":"f","order":0}}}]`))
	if err != nil || len(rows) != 1 || rows[0].NoteID != 7 || rows[0].FieldValue("Front") != "f" {
		t.Fatalf("DecodeNotesInfo = %#v, %v", rows, err)
	}
	if rows, err := DecodeNotesInfo(json.RawMessage("null")); err != nil || rows != nil {
		t.Fatalf("DecodeNotesInfo(null) = %#v, %v", rows, err)
	}
}

func TestResolveHost(t *testing.T) {
	dir := t.TempDir()
	hostDir := filepath.Join(dir, "agent")
	if err := os.MkdirAll(hostDir, 0o700); err != nil {
		t.Fatal(err)
	}
	host := filepath.Join(hostDir, "anki-host.mjs")
	if err := os.WriteFile(host, []byte("// host\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveHost(func(key string) string {
		if key == HostEnv {
			return host
		}
		return ""
	}, nil, nil)
	if err != nil || got != host {
		t.Fatalf("ResolveHost env = %q, %v", got, err)
	}
	got, err = ResolveHost(func(string) string { return "" }, func() (string, error) { return dir, nil },
		func() (string, error) { return filepath.Join(dir, "bin", "passquest"), nil })
	if err != nil || got != host {
		t.Fatalf("ResolveHost cwd = %q, %v", got, err)
	}
	missing := filepath.Join(dir, "missing.mjs")
	if _, err := ResolveHost(func(key string) string {
		if key == HostEnv {
			return missing
		}
		return ""
	}, nil, nil); err == nil {
		t.Fatal("ResolveHost accepted a missing override")
	}
}
