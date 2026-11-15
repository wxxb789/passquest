package anki

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestHostAllowlistMatchesGo: agent/anki-host.mjs is the only egress, so its
// action allowlist and this package's must be the same set.
func TestHostAllowlistMatchesGo(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "agent", "anki-host.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)const ALLOWED_ACTIONS = new Set\(\[(.*?)\]\);`).FindSubmatch(source)
	if block == nil {
		t.Fatal("anki-host.mjs has no ALLOWED_ACTIONS set")
	}
	var host []string
	for _, match := range regexp.MustCompile(`"([A-Za-z]+)"`).FindAllSubmatch(block[1], -1) {
		host = append(host, string(match[1]))
	}
	var goActions []string
	for action := range allowedActions {
		goActions = append(goActions, action)
	}
	slices.Sort(host)
	slices.Sort(goActions)
	if strings.Join(host, ",") != strings.Join(goActions, ",") {
		t.Fatalf("host allowlist = %v, Go allowlist = %v", host, goActions)
	}
	for _, want := range []string{"modelStyling", "modelTemplates", "updateModelStyling", "updateModelTemplates"} {
		if !slices.Contains(goActions, want) {
			t.Fatalf("%s is not allowed", want)
		}
	}
}

func TestModelDesignActionsAreAllowedAndLoopbackOnly(t *testing.T) {
	actions := []Action{
		{Action: "modelStyling", Params: ModelNameParams("PassQuest Note")},
		{Action: "modelTemplates", Params: ModelNameParams("PassQuest Note")},
		{Action: "updateModelTemplates", Params: UpdateModelTemplatesParams("PassQuest Note", "f", "b")},
		{Action: "updateModelStyling", Params: UpdateModelStylingParams("PassQuest Note", "css")},
	}
	if err := ValidateRequest(HostRequest{ProtocolVersion: ProtocolVersion, Endpoint: DefaultEndpoint, TimeoutMS: 1000, Actions: actions}); err != nil {
		t.Fatalf("model design actions are refused: %v", err)
	}
	if err := ValidateRequest(HostRequest{ProtocolVersion: ProtocolVersion, Endpoint: "http://10.0.0.2:8765", TimeoutMS: 1000, Actions: actions}); err == nil {
		t.Fatal("a remote endpoint was accepted")
	}
	for _, refused := range []string{"updateModel", "deleteNotes", "modelNamesAndIds", "createModelTemplate", "removeTemplate", "sync"} {
		if err := ValidateRequest(HostRequest{ProtocolVersion: ProtocolVersion, Endpoint: DefaultEndpoint, TimeoutMS: 1000,
			Actions: []Action{{Action: refused}}}); err == nil {
			t.Fatalf("%s was accepted", refused)
		}
	}
}

func TestModelDesignParams(t *testing.T) {
	if got := ModelNameParams("PassQuest Note"); got["modelName"] != "PassQuest Note" || len(got) != 1 {
		t.Fatalf("ModelNameParams = %#v", got)
	}
	styling, _ := json.Marshal(UpdateModelStylingParams("PassQuest Note", ".card{}"))
	if string(styling) != `{"model":{"css":".card{}","name":"PassQuest Note"}}` {
		t.Fatalf("UpdateModelStylingParams = %s", styling)
	}
	templates, _ := json.Marshal(UpdateModelTemplatesParams("PassQuest Note", "F", "B"))
	if string(templates) != `{"model":{"name":"PassQuest Note","templates":{"PassQuest":{"Back":"B","Front":"F"}}}}` {
		t.Fatalf("UpdateModelTemplatesParams = %s", templates)
	}
	created := CreateModelParams(ModelParams{Name: "PassQuest Note", Fields: []string{"Front"}, CSS: "c", FrontTemplate: "F", BackTemplate: "B"})
	if created["cardTemplates"].([]map[string]string)[0]["Name"] != TemplateName || TemplateName != "PassQuest" {
		t.Fatalf("createModel template name = %#v", created["cardTemplates"])
	}
}

func TestDecodeModelDesign(t *testing.T) {
	css, err := DecodeModelStyling(json.RawMessage(`{"css": ".card { color: black; }"}`))
	if err != nil || css != ".card { color: black; }" {
		t.Fatalf("DecodeModelStyling = %q, %v", css, err)
	}
	if css, err := DecodeModelStyling(json.RawMessage(`{"css": ""}`)); err != nil || css != "" {
		t.Fatalf("DecodeModelStyling(empty) = %q, %v", css, err)
	}
	for _, raw := range []string{`null`, `{}`, `"css"`, `{"css": 1}`} {
		if _, err := DecodeModelStyling(json.RawMessage(raw)); err == nil {
			t.Fatalf("DecodeModelStyling accepted %s", raw)
		}
	}
	templates, err := DecodeModelTemplates(json.RawMessage(`{"PassQuest": {"Front": "{{Front}}", "Back": "{{FrontSide}}"}}`))
	if err != nil || len(templates) != 1 || templates["PassQuest"] != (CardTemplate{Front: "{{Front}}", Back: "{{FrontSide}}"}) {
		t.Fatalf("DecodeModelTemplates = %#v, %v", templates, err)
	}
	for _, raw := range []string{`null`, `[]`, `{"PassQuest": "x"}`} {
		if _, err := DecodeModelTemplates(json.RawMessage(raw)); err == nil {
			t.Fatalf("DecodeModelTemplates accepted %s", raw)
		}
	}
}
