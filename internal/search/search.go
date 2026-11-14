// Package search owns the one configured search adapter: endpoint validation,
// the exact provider request body, the Node host boundary and strict parsing
// of the provider's JSON result. It performs no database work; the caller
// decides what may be stored. Fetched text is data, never an instruction.
package search

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wxxb789/passquest/internal/hostproc"
)

const (
	// ProtocolVersion is the version of the Go <-> host document.
	ProtocolVersion = 1
	// EndpointEnv enables the adapter. Without it `find` reports "no provider
	// configured" and performs no network I/O.
	EndpointEnv = "PASSQUEST_SEARCH_ENDPOINT"
	// KeyEnv holds the optional provider credential. The host reads it and
	// places it only in the Authorization header.
	KeyEnv = "PASSQUEST_SEARCH_KEY"
	// HostEnv overrides the search host path.
	HostEnv = "PASSQUEST_SEARCH_HOST"
	// NodeEnv overrides the Node executable.
	NodeEnv = "PASSQUEST_NODE"

	DefaultTimeoutMS = 10000
	MinTimeoutMS     = 1000
	MaxTimeoutMS     = 120000
	// MaxResponseBytes bounds the provider body after the host envelope is
	// decoded. It is the provider contract enforced by agent/search-host.mjs.
	MaxResponseBytes = 1 << 20
	// MaxHostResponseBytes bounds the host's whole stdout document. The
	// provider body travels inside it as a JSON string, where a byte can cost
	// six escaped bytes (`\u0000`), so this cannot equal MaxResponseBytes
	// without rejecting legal responses as truncation. The extra 64 KiB covers
	// the envelope fields, the trailing newline and provider metadata.
	MaxHostResponseBytes = 6*MaxResponseBytes + (64 << 10)
	MaxResults           = 1000
	MaxTitleRunes        = 300
	MaxURLRunes          = 2048
	MaxSnippetRunes      = 2000
	MaxContentRunes      = 20000
	MaxLicenseRunes      = 100
	MaxProviderRunes     = 200
	// MaxEndpointHostChars matches find_queries.endpoint_host. A longer host is
	// refused before a reservation or a provider request.
	MaxEndpointHostChars = 300
)

// Item is one raw provider result. Classification and provenance are policy
// owned by internal/find; this package only validates the provider's shape.
type Item struct {
	Title     string
	URL       string
	Snippet   string
	Content   string
	License   string
	Generated bool
}

// Request is one configured search. Query is the bounded intent text; Limit is
// the explicit candidate cap the provider must honour.
type Request struct {
	Endpoint  string
	Query     string
	Limit     int
	TimeoutMS int
}

// Result is a validated provider response plus the exact request identity.
type Result struct {
	Provider      string
	Items         []Item
	HTTPStatus    int
	LatencyMS     int64
	RequestSHA256 string
	RequestBytes  int
}

// ProviderError is a structured failure. RequestSent is true only when the
// provider request may have left the machine; the caller keeps the budget slot
// in that case.
type ProviderError struct {
	Code        string
	Message     string
	HTTPStatus  int
	LatencyMS   int64
	RequestSent bool
	RequestSHA  string
	RequestSize int
}

func (e *ProviderError) Error() string {
	if e == nil {
		return "search: provider error"
	}
	if e.Message == "" {
		return "search: " + e.Code
	}
	return fmt.Sprintf("search: %s: %s", e.Code, e.Message)
}

// AsProviderError extracts a structured provider failure.
func AsProviderError(err error) (*ProviderError, bool) {
	var failure *ProviderError
	if errors.As(err, &failure) {
		return failure, true
	}
	return nil, false
}

// Adapter is the one search interface. The CLI depends on it so tests can
// replace the provider without starting Node or a network listener.
type Adapter interface {
	Search(ctx context.Context, req Request) (Result, error)
}

// NormalizeEndpoint validates a search endpoint and returns its normalized
// spelling. HTTPS is required off-loopback; plain HTTP is accepted only for a
// local stub. A path is allowed because providers commonly expose /search or
// /v1/search. Userinfo, query and fragment are refused.
func NormalizeEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("search: endpoint is empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("search: endpoint is not a URL: %w", err)
	}
	switch {
	case !parsed.IsAbs(), parsed.Opaque != "", parsed.Host == "":
		return "", errors.New("search: endpoint must be an absolute http(s) URL with a host")
	case parsed.User != nil:
		return "", fmt.Errorf("search: endpoint must not contain userinfo; the credential belongs only in %s", KeyEnv)
	case parsed.RawQuery != "":
		return "", errors.New("search: endpoint must not contain a query string")
	case parsed.Fragment != "":
		return "", errors.New("search: endpoint must not contain a fragment")
	case parsed.Scheme != "https" && parsed.Scheme != "http":
		return "", fmt.Errorf("search: endpoint scheme %q is not http or https", parsed.Scheme)
	case parsed.Scheme == "http" && !Loopback(parsed.Hostname()):
		return "", errors.New("search: endpoint uses http off-loopback; use https or a loopback address")
	case len(parsed.Host) > MaxEndpointHostChars:
		return "", fmt.Errorf("search: endpoint host is longer than %d characters", MaxEndpointHostChars)
	}
	return parsed.String(), nil
}

// Loopback reports whether host names a local interface. A search stub uses
// loopback; a real provider must use https.
func Loopback(host string) bool {
	if strings.EqualFold(strings.Trim(host, "[]"), "localhost") {
		return true
	}
	addr, err := netip.ParseAddr(strings.Trim(host, "[]"))
	return err == nil && addr.IsLoopback()
}

// EndpointHost returns the host and port recorded as provenance. The path,
// query, fragment and userinfo are never recorded.
func EndpointHost(endpoint string) string {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || len(parsed.Host) > MaxEndpointHostChars {
		return ""
	}
	return parsed.Host
}

// BuildBody returns the exact provider request bytes. The host posts these
// bytes unchanged, so the recorded SHA-256 identifies what left the machine.
func BuildBody(query string, limit int) (string, error) {
	query = strings.Join(strings.Fields(query), " ")
	if query == "" {
		return "", errors.New("search: query is empty")
	}
	if utf8.RuneCountInString(query) > 300 {
		return "", errors.New("search: query is longer than 300 characters")
	}
	if limit < 1 || limit > 100 {
		return "", fmt.Errorf("search: limit %d is outside 1..100", limit)
	}
	body, err := json.Marshal(struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}{Query: query, Limit: limit})
	if err != nil {
		return "", fmt.Errorf("search: encode request: %w", err)
	}
	return string(body), nil
}

type hostRequest struct {
	ProtocolVersion int    `json:"protocol_version"`
	Endpoint        string `json:"endpoint"`
	TimeoutMS       int    `json:"timeout_ms"`
	Query           string `json:"query"`
	Limit           int    `json:"limit"`
	Body            string `json:"body"`
}

type hostError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type hostResponse struct {
	ProtocolVersion int        `json:"protocol_version"`
	Status          string     `json:"status"`
	Error           *hostError `json:"error"`
	HTTPStatus      int        `json:"http_status"`
	LatencyMS       int64      `json:"latency_ms"`
	RequestSent     bool       `json:"request_sent"`
	Body            string     `json:"body"`
}

// HostTransport runs agent/search-host.mjs as a child process and exchanges
// one versioned JSON document over stdin/stdout. The credential is inherited
// through the environment and never appears in the document or an error.
type HostTransport struct {
	Getenv     func(string) string
	Environ    func() []string
	Getwd      func() (string, error)
	Executable func() (string, error)
}

func (t HostTransport) getenv(key string) string {
	if t.Getenv != nil {
		return t.Getenv(key)
	}
	return os.Getenv(key)
}

func (t HostTransport) environ() []string {
	if t.Environ != nil {
		return t.Environ()
	}
	return os.Environ()
}

func (t HostTransport) getwd() (string, error) {
	if t.Getwd != nil {
		return t.Getwd()
	}
	return os.Getwd()
}

func (t HostTransport) executable() (string, error) {
	if t.Executable != nil {
		return t.Executable()
	}
	return os.Executable()
}

// ResolveHost finds agent/search-host.mjs. An explicit PASSQUEST_SEARCH_HOST is
// authoritative; otherwise the working directory, then the executable's
// directory and its parent are checked. The returned path is absolute.
func ResolveHost(getenv func(string) string, getwd func() (string, error), executablePath func() (string, error)) (string, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	if value := strings.TrimSpace(getenv(HostEnv)); value != "" {
		path, err := filepath.Abs(value)
		if err != nil {
			return "", fmt.Errorf("search: resolve %s: %w", HostEnv, err)
		}
		if err := requireFile(path); err != nil {
			return "", err
		}
		return path, nil
	}

	var candidates []string
	if getwd == nil {
		getwd = os.Getwd
	}
	if cwd, err := getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "agent", "search-host.mjs"))
	}
	if executablePath == nil {
		executablePath = os.Executable
	}
	if exe, err := executablePath(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "agent", "search-host.mjs"),
			filepath.Join(dir, "..", "agent", "search-host.mjs"),
		)
	}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if requireFile(absolute) == nil {
			return absolute, nil
		}
	}
	return "", fmt.Errorf("search: host not found; set %s to agent/search-host.mjs (looked in %s)",
		HostEnv, strings.Join(candidates, ", "))
}

func requireFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("search: host %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("search: host %s is a directory", path)
	}
	return nil
}

// Search validates the request, runs the host with the inherited environment
// and returns a validated provider result. Provider and host failures are
// structured; only a local invocation failure is a Go error.
func (t HostTransport) Search(ctx context.Context, req Request) (Result, error) {
	endpoint, err := NormalizeEndpoint(req.Endpoint)
	if err != nil {
		return Result{}, &ProviderError{Code: "invalid_endpoint", Message: err.Error()}
	}
	body, err := BuildBody(req.Query, req.Limit)
	if err != nil {
		return Result{}, &ProviderError{Code: "invalid_request", Message: err.Error()}
	}
	if req.TimeoutMS < MinTimeoutMS || req.TimeoutMS > MaxTimeoutMS {
		return Result{}, &ProviderError{
			Code:    "invalid_request",
			Message: fmt.Sprintf("timeout_ms %d is outside %d..%d", req.TimeoutMS, MinTimeoutMS, MaxTimeoutMS),
		}
	}
	sum := sha256.Sum256([]byte(body))
	requestSHA := hex.EncodeToString(sum[:])
	requestBytes := len(body)

	host, err := ResolveHost(t.Getenv, t.Getwd, t.Executable)
	if err != nil {
		return Result{}, &ProviderError{
			Code:        "search_unavailable",
			Message:     err.Error(),
			RequestSHA:  requestSHA,
			RequestSize: requestBytes,
		}
	}
	node := strings.TrimSpace(t.getenv(NodeEnv))
	if node == "" {
		node = "node"
	}
	payload, err := json.Marshal(hostRequest{
		ProtocolVersion: ProtocolVersion,
		Endpoint:        endpoint,
		TimeoutMS:       req.TimeoutMS,
		Query:           req.Query,
		Limit:           req.Limit,
		Body:            body,
	})
	if err != nil {
		return Result{}, fmt.Errorf("search: encode host request: %w", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, time.Duration(req.TimeoutMS)*time.Millisecond+5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(callCtx, node, host)
	cmd.Env = t.environ()
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr limitedBuffer
	stdout.limit = MaxHostResponseBytes
	stderr.limit = 64 << 10
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = 2 * time.Second

	key := strings.TrimSpace(t.getenv(KeyEnv))
	runErr := hostproc.Run(cmd)
	if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
		return Result{}, &ProviderError{
			Code:        "timeout",
			Message:     fmt.Sprintf("search host did not finish within %d ms", req.TimeoutMS+5000),
			RequestSent: true,
			RequestSHA:  requestSHA,
			RequestSize: requestBytes,
		}
	}
	if runErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = runErr.Error()
		}
		failure := &ProviderError{
			Code:        "search_unavailable",
			Message:     redact(detail, key),
			RequestSHA:  requestSHA,
			RequestSize: requestBytes,
		}
		// A started process may have posted the body before it failed, so the
		// budget slot and a failed audit row must be kept. Only a start
		// failure (for example a missing Node binary) proves no request.
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			failure.Code = "provider_error"
			failure.Message = "search host exited after starting; a request may have been sent: " + redact(detail, key)
			failure.RequestSent = true
		}
		return Result{}, failure
	}
	if stdout.truncated {
		return Result{}, &ProviderError{
			Code:        "invalid_response",
			Message:     fmt.Sprintf("search host response exceeded %d bytes", MaxHostResponseBytes),
			RequestSent: true,
			RequestSHA:  requestSHA,
			RequestSize: requestBytes,
		}
	}

	var response hostResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return Result{}, &ProviderError{
			Code:        "invalid_response",
			Message:     "search host response is not JSON",
			RequestSent: true,
			RequestSHA:  requestSHA,
			RequestSize: requestBytes,
		}
	}
	if response.ProtocolVersion != ProtocolVersion {
		return Result{}, &ProviderError{
			Code:        "invalid_response",
			Message:     fmt.Sprintf("search host protocol %d is not supported", response.ProtocolVersion),
			RequestSent: true,
			RequestSHA:  requestSHA,
			RequestSize: requestBytes,
		}
	}
	if response.Status == "error" {
		message := "provider request failed"
		code := "provider_error"
		if response.Error != nil {
			if strings.TrimSpace(response.Error.Code) != "" {
				code = response.Error.Code
			}
			if strings.TrimSpace(response.Error.Message) != "" {
				message = response.Error.Message
			}
		}
		return Result{}, &ProviderError{
			Code:        code,
			Message:     redact(message, key),
			HTTPStatus:  response.HTTPStatus,
			LatencyMS:   response.LatencyMS,
			RequestSent: response.RequestSent,
			RequestSHA:  requestSHA,
			RequestSize: requestBytes,
		}
	}
	if response.Status != "ok" {
		return Result{}, &ProviderError{
			Code:        "invalid_response",
			Message:     fmt.Sprintf("search host returned unknown state %q", response.Status),
			RequestSent: response.RequestSent,
			RequestSHA:  requestSHA,
			RequestSize: requestBytes,
		}
	}
	// The host enforces the same bound on the provider body; re-check the
	// decoded string so the larger envelope buffer cannot raise the contract.
	if len(response.Body) > MaxResponseBytes {
		return Result{}, &ProviderError{
			Code:        "invalid_response",
			Message:     fmt.Sprintf("provider response exceeded %d bytes", MaxResponseBytes),
			HTTPStatus:  response.HTTPStatus,
			LatencyMS:   response.LatencyMS,
			RequestSent: response.RequestSent,
			RequestSHA:  requestSHA,
			RequestSize: requestBytes,
		}
	}
	provider, items, err := ParseProviderResponse([]byte(response.Body))
	if err != nil {
		return Result{}, &ProviderError{
			Code:        "invalid_response",
			Message:     err.Error(),
			HTTPStatus:  response.HTTPStatus,
			LatencyMS:   response.LatencyMS,
			RequestSent: response.RequestSent,
			RequestSHA:  requestSHA,
			RequestSize: requestBytes,
		}
	}
	return Result{
		Provider:      provider,
		Items:         items,
		HTTPStatus:    response.HTTPStatus,
		LatencyMS:     response.LatencyMS,
		RequestSHA256: requestSHA,
		RequestBytes:  requestBytes,
	}, nil
}

// ParseProviderResponse validates the documented provider JSON. Unknown fields
// are ignored so a provider can add metadata; every field this build consumes
// must have the documented type. A malformed body stores no candidate.
func ParseProviderResponse(data []byte) (string, []Item, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw struct {
		Provider *string            `json:"provider"`
		Results  *[]json.RawMessage `json:"results"`
	}
	if err := dec.Decode(&raw); err != nil {
		return "", nil, fmt.Errorf("provider response is not JSON: %v", err)
	}
	if dec.More() {
		return "", nil, errors.New("provider response has trailing JSON data")
	}
	if raw.Provider == nil || strings.TrimSpace(*raw.Provider) == "" {
		return "", nil, errors.New("provider response has no provider name")
	}
	provider := strings.TrimSpace(*raw.Provider)
	if utf8.RuneCountInString(provider) > MaxProviderRunes {
		return "", nil, fmt.Errorf("provider name is longer than %d characters", MaxProviderRunes)
	}
	if raw.Results == nil {
		return "", nil, errors.New("provider response has no results array")
	}
	if len(*raw.Results) > MaxResults {
		return "", nil, fmt.Errorf("provider returned %d results; the hard bound is %d", len(*raw.Results), MaxResults)
	}

	items := make([]Item, 0, len(*raw.Results))
	for index, entry := range *raw.Results {
		var item struct {
			Title     *string `json:"title"`
			URL       *string `json:"url"`
			Snippet   *string `json:"snippet"`
			Content   *string `json:"content"`
			License   *string `json:"license"`
			Generated *bool   `json:"generated"`
		}
		if err := json.Unmarshal(entry, &item); err != nil {
			return "", nil, fmt.Errorf("result %d is not an object: %v", index+1, err)
		}
		if item.Title == nil || strings.TrimSpace(*item.Title) == "" {
			return "", nil, fmt.Errorf("result %d has no title", index+1)
		}
		title := *item.Title
		if utf8.RuneCountInString(title) > MaxTitleRunes {
			return "", nil, fmt.Errorf("result %d title is longer than %d characters", index+1, MaxTitleRunes)
		}
		if item.URL == nil || strings.TrimSpace(*item.URL) == "" {
			return "", nil, fmt.Errorf("result %d has no URL", index+1)
		}
		rawURL := strings.TrimSpace(*item.URL)
		if err := validateResultURL(rawURL); err != nil {
			return "", nil, fmt.Errorf("result %d URL: %v", index+1, err)
		}
		snippet := optional(item.Snippet)
		content := optional(item.Content)
		license := optional(item.License)
		if utf8.RuneCountInString(snippet) > MaxSnippetRunes {
			return "", nil, fmt.Errorf("result %d snippet is longer than %d characters", index+1, MaxSnippetRunes)
		}
		if utf8.RuneCountInString(content) > MaxContentRunes {
			return "", nil, fmt.Errorf("result %d content is longer than %d characters", index+1, MaxContentRunes)
		}
		if utf8.RuneCountInString(license) > MaxLicenseRunes {
			return "", nil, fmt.Errorf("result %d license is longer than %d characters", index+1, MaxLicenseRunes)
		}
		generated := false
		if item.Generated != nil {
			generated = *item.Generated
		}
		items = append(items, Item{
			Title:     strings.TrimSpace(title),
			URL:       rawURL,
			Snippet:   snippet,
			Content:   content,
			License:   license,
			Generated: generated,
		})
	}
	return provider, items, nil
}

func optional(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func validateResultURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("is not a URL: %v", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("scheme is not http or https")
	}
	if parsed.Host == "" {
		return errors.New("has no host")
	}
	if parsed.User != nil {
		return errors.New("must not contain userinfo")
	}
	if utf8.RuneCountInString(raw) > MaxURLRunes {
		return fmt.Errorf("is longer than %d characters", MaxURLRunes)
	}
	return nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit > 0 && b.Len()+len(p) > b.limit {
		remaining := b.limit - b.Len()
		if remaining > 0 {
			b.Buffer.Write(p[:remaining])
		}
		b.truncated = true
		return len(p), nil
	}
	return b.Buffer.Write(p)
}

// redact removes a credential from host error text before it can be stored or
// printed. The value itself is never returned.
func redact(text, key string) string {
	if key == "" {
		return text
	}
	return strings.ReplaceAll(text, key, "[redacted]")
}
