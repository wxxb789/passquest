// Package anki owns the AnkiConnect wire format Pass Quest speaks: the action
// allowlist, endpoint validation, request/response documents, note payloads and
// the classification of Anki's duplicate/not-found answers. It never imports a
// network package; agent/anki-host.mjs is the only egress.
package anki

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"unicode/utf8"
)

// ProtocolVersion is the local host document version.
const ProtocolVersion = 1

// DefaultEndpoint is AnkiConnect's documented loopback address.
const DefaultEndpoint = "http://127.0.0.1:8765"

// MinAPIVersion is the minimum AnkiConnect API version this build understands.
const MinAPIVersion = 6

// MaxActionsPerCall bounds one host document; the CLI chunks a larger queue.
const MaxActionsPerCall = 50

// EndpointEnv and TimeoutEnv configure the default connection.
const (
	EndpointEnv = "PASSQUEST_ANKI_ENDPOINT"
	HostEnv     = "PASSQUEST_ANKI_HOST"
	NodeEnv     = "PASSQUEST_NODE"
)

// Action is one AnkiConnect request. Params is a JSON object.
type Action struct {
	Action string         `json:"action"`
	Params map[string]any `json:"params"`
}

// ActionResult is the host's report for one action. Result is Anki's raw
// result value; Error is Anki's error string, never a credential.
type ActionResult struct {
	Action     string          `json:"action"`
	State      string          `json:"state"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
	HTTPStatus int             `json:"http_status,omitempty"`
}

// HostRequest is one versioned stdin document.
type HostRequest struct {
	ProtocolVersion int      `json:"protocol_version"`
	Endpoint        string   `json:"endpoint"`
	TimeoutMS       int64    `json:"timeout_ms"`
	Actions         []Action `json:"actions"`
}

// HostResponse is the host's stdout document.
type HostResponse struct {
	ProtocolVersion int            `json:"protocol_version"`
	State           string         `json:"state"`
	Results         []ActionResult `json:"results"`
	Error           string         `json:"error,omitempty"`
}

// Transport is the seam to the Node host; tests inject a fake.
type Transport interface {
	Call(ctx context.Context, req HostRequest) (HostResponse, error)
}

var allowedActions = map[string]bool{
	"version":          true,
	"createDeck":       true,
	"modelNames":       true,
	"createModel":      true,
	"addNote":          true,
	"updateNoteFields": true,
	"notesInfo":        true,
}

var allowedResultStates = map[string]bool{"ok": true, "error": true, "unreachable": true}
var allowedHostStates = map[string]bool{"ok": true, "unreachable": true, "failed": true}

// NormalizeEndpoint validates and normalizes one AnkiConnect endpoint. Only a
// loopback http(s) endpoint is accepted: a note can carry private source text,
// and AnkiConnect is a local bridge, not a remote service.
func NormalizeEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = DefaultEndpoint
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("anki: endpoint is not a URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("anki: endpoint must start with http:// or https://")
	}
	if parsed.User != nil {
		return "", errors.New("anki: endpoint must not contain userinfo")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("anki: endpoint must not contain a query or fragment")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", errors.New("anki: endpoint path must be empty")
	}
	host := parsed.Hostname()
	if host == "" {
		return "", errors.New("anki: endpoint has no host")
	}
	if !loopback(host) {
		return "", fmt.Errorf("anki: endpoint host %q is not loopback; a note carries private source text", host)
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func loopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.IsLoopback()
}

// ValidateRequest checks a host document before it can leave the process.
func ValidateRequest(req HostRequest) error {
	if req.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("anki: host protocol %d is not supported", req.ProtocolVersion)
	}
	if _, err := NormalizeEndpoint(req.Endpoint); err != nil {
		return err
	}
	if req.TimeoutMS < 100 || req.TimeoutMS > 120_000 {
		return fmt.Errorf("anki: timeout %d ms is outside 100..120000", req.TimeoutMS)
	}
	if len(req.Actions) == 0 || len(req.Actions) > MaxActionsPerCall {
		return fmt.Errorf("anki: action count %d is outside 1..%d", len(req.Actions), MaxActionsPerCall)
	}
	total := 0
	for i, action := range req.Actions {
		if !allowedActions[action.Action] {
			return fmt.Errorf("anki: action %q is not allowed", action.Action)
		}
		params, err := json.Marshal(action.Params)
		if err != nil {
			return fmt.Errorf("anki: action %d params are not encodable: %w", i+1, err)
		}
		total += len(params)
	}
	if total > 1<<20 {
		return fmt.Errorf("anki: action params exceed 1 MiB")
	}
	return nil
}

// ValidateResponse checks the host's document against the request it answered.
func ValidateResponse(req HostRequest, resp HostResponse) error {
	if resp.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("anki: host returned protocol %d", resp.ProtocolVersion)
	}
	if !allowedHostStates[resp.State] {
		return fmt.Errorf("anki: host returned state %q", resp.State)
	}
	if len(resp.Results) != len(req.Actions) {
		message := fmt.Sprintf("anki: host returned %d results for %d actions", len(resp.Results), len(req.Actions))
		if detail := strings.TrimSpace(resp.Error); detail != "" {
			if utf8.RuneCountInString(detail) > 500 {
				detail = string([]rune(detail)[:500])
			}
			message += ": " + detail
		}
		return errors.New(message)
	}
	for i, result := range resp.Results {
		if result.Action != req.Actions[i].Action {
			return fmt.Errorf("anki: result %d names action %q, want %q", i+1, result.Action, req.Actions[i].Action)
		}
		if !allowedResultStates[result.State] {
			return fmt.Errorf("anki: result %d has state %q", i+1, result.State)
		}
		if utf8.RuneCountInString(result.Error) > 500 {
			return fmt.Errorf("anki: result %d error exceeds 500 characters", i+1)
		}
		if strings.ContainsRune(result.Error, 0) {
			return fmt.Errorf("anki: result %d error contains NUL", i+1)
		}
	}
	if utf8.RuneCountInString(resp.Error) > 500 {
		return errors.New("anki: host error exceeds 500 characters")
	}
	return nil
}

// IsUnreachable reports whether the host could not contact the endpoint.
func IsUnreachable(resp HostResponse) bool {
	if resp.State == "unreachable" {
		return true
	}
	for _, result := range resp.Results {
		if result.State == "unreachable" {
			return true
		}
	}
	return false
}

// IsDuplicateError recognizes Anki's duplicate report for addNote.
func IsDuplicateError(message string) bool {
	return strings.Contains(strings.ToLower(message), "duplicate")
}

// IsNotFoundError recognizes Anki's missing-note report for updateNoteFields.
func IsNotFoundError(message string) bool {
	return strings.Contains(strings.ToLower(message), "not found")
}

// ModelParams is the createModel payload owned by the caller.
type ModelParams struct {
	Name          string
	Fields        []string
	CSS           string
	FrontTemplate string
	BackTemplate  string
}

// CreateDeckParams builds the createDeck action.
func CreateDeckParams(deck string) map[string]any {
	return map[string]any{"deck": deck}
}

// CreateModelParams builds the createModel action.
func CreateModelParams(model ModelParams) map[string]any {
	return map[string]any{
		"modelName":     model.Name,
		"inOrderFields": append([]string(nil), model.Fields...),
		"css":           model.CSS,
		"cardTemplates": []map[string]string{
			{"Name": "PassQuest", "Front": model.FrontTemplate, "Back": model.BackTemplate},
		},
	}
}

// AddNoteParams builds an addNote action with duplicate protection.
func AddNoteParams(deck, model string, fields map[string]string, tags []string) map[string]any {
	return map[string]any{
		"note": map[string]any{
			"deckName":  deck,
			"modelName": model,
			"fields":    fields,
			"options":   map[string]any{"allowDuplicate": false},
			"tags":      tags,
		},
	}
}

// UpdateNoteParams builds an updateNoteFields action for an existing note id.
func UpdateNoteParams(id int64, fields map[string]string) map[string]any {
	return map[string]any{
		"note": map[string]any{"id": id, "fields": fields},
	}
}

// NotesInfoParams builds a notesInfo read-back action.
func NotesInfoParams(ids []int64) map[string]any {
	return map[string]any{"notes": ids}
}

// NoteInfo is the subset of notesInfo this build reads.
type NoteInfo struct {
	NoteID    int64                      `json:"noteId"`
	ModelName string                     `json:"modelName"`
	Fields    map[string]json.RawMessage `json:"fields"`
}

// FieldValue returns one field's value.
func (n NoteInfo) FieldValue(name string) string {
	raw, ok := n.Fields[name]
	if !ok {
		return ""
	}
	var field struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &field); err != nil {
		return ""
	}
	return field.Value
}

// DecodeNotesInfo decodes a notesInfo result. Anki returns null for no rows.
func DecodeNotesInfo(raw json.RawMessage) ([]NoteInfo, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var rows []NoteInfo
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("anki: decode notesInfo: %w", err)
	}
	return rows, nil
}

// DecodeInt decodes an integer result such as version, createDeck or addNote.
func DecodeInt(raw json.RawMessage) (int64, error) {
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("anki: decode integer result: %w", err)
	}
	return value, nil
}

// DecodeStringList decodes a string-array result such as modelNames.
func DecodeStringList(raw json.RawMessage) ([]string, error) {
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("anki: decode string list: %w", err)
	}
	return out, nil
}
