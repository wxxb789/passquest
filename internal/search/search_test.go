package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain lets this test binary stand in for the Node host, so the transport
// contract can be exercised without a Node runtime on any OS.
func TestMain(m *testing.M) {
	if os.Getenv("PASSQUEST_SEARCH_HELPER") == "1" {
		os.Exit(runSearchHelper())
	}
	os.Exit(m.Run())
}

func runSearchHelper() int {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return 2
	}
	var request hostRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return 3
	}
	switch os.Getenv("PASSQUEST_SEARCH_HELPER_MODE") {
	case "exit-after-read":
		return 1
	case "record-body":
		if err := os.WriteFile(os.Getenv("PASSQUEST_SEARCH_HELPER_BODY_FILE"), []byte(request.Body), 0o600); err != nil {
			return 4
		}
		if err := writeHostResponse(`{"provider":"helper","results":[]}`); err != nil {
			return 5
		}
		return 0
	case "quote-heavy":
		if err := writeHostResponse(quoteHeavyProvider()); err != nil {
			return 7
		}
		return 0
	case "oversize-body":
		if err := writeHostResponse(strings.Repeat("x", MaxResponseBytes+1)); err != nil {
			return 8
		}
		return 0
	}
	return 6
}

// writeHostResponse writes the host envelope and, when the test asks, copies
// the exact bytes it emitted so the test can measure what the transport read.
func writeHostResponse(body string) error {
	encoded, err := json.Marshal(hostResponse{
		ProtocolVersion: ProtocolVersion,
		Status:          "ok",
		HTTPStatus:      200,
		LatencyMS:       1,
		RequestSent:     true,
		Body:            body,
	})
	if err != nil {
		return err
	}
	if path := os.Getenv("PASSQUEST_SEARCH_HELPER_RESPONSE_FILE"); path != "" {
		if err := os.WriteFile(path, encoded, 0o600); err != nil {
			return err
		}
	}
	_, err = os.Stdout.Write(append(encoded, '\n'))
	return err
}

// quoteHeavyProvider is a provider body just under MaxResponseBytes whose
// quote characters double when the host nests it in the JSON envelope.
func quoteHeavyProvider() string {
	results := make([]map[string]any, 0, 20)
	for index := range 20 {
		results = append(results, map[string]any{
			"title":   fmt.Sprintf("result %d", index+1),
			"url":     fmt.Sprintf("https://example.com/%d", index+1),
			"content": strings.Repeat(`"`, MaxContentRunes),
		})
	}
	encoded, err := json.Marshal(map[string]any{"provider": "helper", "results": results})
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func helperTransport(t *testing.T, mode string) (HostTransport, string) {
	t.Helper()
	host := filepath.Join(t.TempDir(), "search-host.mjs")
	if err := os.WriteFile(host, []byte("// the test binary is the host\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PASSQUEST_SEARCH_HELPER", "1")
	t.Setenv("PASSQUEST_SEARCH_HELPER_MODE", mode)
	bodyFile := filepath.Join(t.TempDir(), "request-body")
	if mode == "record-body" {
		t.Setenv("PASSQUEST_SEARCH_HELPER_BODY_FILE", bodyFile)
	}
	env := map[string]string{
		HostEnv: host,
		NodeEnv: os.Args[0],
	}
	return HostTransport{Getenv: func(key string) string { return env[key] }}, bodyFile
}

// helperTransportWithResponse also captures the exact host document the
// helper wrote, so a test can measure the envelope rather than guess it.
func helperTransportWithResponse(t *testing.T, mode string) (HostTransport, string) {
	t.Helper()
	transport, _ := helperTransport(t, mode)
	responseFile := filepath.Join(t.TempDir(), "host-response")
	t.Setenv("PASSQUEST_SEARCH_HELPER_RESPONSE_FILE", responseFile)
	return transport, responseFile
}

func TestHostTransportHashesTheBodyItPosted(t *testing.T) {
	transport, bodyFile := helperTransport(t, "record-body")
	result, err := transport.Search(context.Background(), Request{
		Endpoint:  "https://api.example.com/search",
		Query:     "  vocabulary   in context ",
		Limit:     20,
		TimeoutMS: 5000,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	body, err := os.ReadFile(bodyFile)
	if err != nil {
		t.Fatalf("read helper body: %v", err)
	}
	wantBody, err := BuildBody("  vocabulary   in context ", 20)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != wantBody {
		t.Fatalf("posted body = %q, want %q", body, wantBody)
	}
	sum := sha256.Sum256(body)
	if result.RequestSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("request hash = %s, want the hash of the posted body", result.RequestSHA256)
	}
	if result.RequestBytes != len(body) {
		t.Fatalf("request bytes = %d, want %d", result.RequestBytes, len(body))
	}
}

// The host nests the provider body as a JSON string, so a legal body just
// under the provider limit can exceed it once quotes are escaped. The envelope
// has its own bound; reading it must not be mistaken for a truncated response.
func TestHostTransportAcceptsAnEscapedProviderBodyOverTheProviderLimit(t *testing.T) {
	transport, responseFile := helperTransportWithResponse(t, "quote-heavy")
	result, err := transport.Search(context.Background(), Request{
		Endpoint:  "https://api.example.com/search",
		Query:     "vocabulary in context",
		Limit:     20,
		TimeoutMS: 5000,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Items) != 20 {
		t.Fatalf("items = %d, want 20", len(result.Items))
	}
	if got := result.Items[0].Content; got != strings.Repeat(`"`, MaxContentRunes) {
		t.Fatalf("first content length = %d, want %d", len(got), MaxContentRunes)
	}
	envelope, err := os.ReadFile(responseFile)
	if err != nil {
		t.Fatalf("read the host document: %v", err)
	}
	if len(envelope) <= MaxResponseBytes {
		t.Fatalf("host document = %d bytes; the fixture must exceed the provider limit to reproduce the truncation", len(envelope))
	}
	if len(envelope) > MaxHostResponseBytes {
		t.Fatalf("host document = %d bytes, over the %d-byte envelope bound", len(envelope), MaxHostResponseBytes)
	}
}

// The larger envelope buffer must not raise the provider-body contract.
func TestHostTransportRejectsAProviderBodyOverTheProviderLimit(t *testing.T) {
	transport, _ := helperTransport(t, "oversize-body")
	_, err := transport.Search(context.Background(), Request{
		Endpoint:  "https://api.example.com/search",
		Query:     "vocabulary in context",
		Limit:     20,
		TimeoutMS: 5000,
	})
	failure, ok := AsProviderError(err)
	if !ok {
		t.Fatalf("error = %v, want a structured provider failure", err)
	}
	if failure.Code != "invalid_response" || !failure.RequestSent {
		t.Fatalf("failure = %+v; an over-limit provider body is an invalid response after a sent request", failure)
	}
	if !strings.Contains(failure.Message, "provider response exceeded") || !strings.Contains(failure.Message, fmt.Sprint(MaxResponseBytes)) {
		t.Fatalf("message = %q, want the provider-body limit named", failure.Message)
	}
}

func TestHostTransportKeepsTheSlotWhenAStartedHostExits(t *testing.T) {
	transport, _ := helperTransport(t, "exit-after-read")
	_, err := transport.Search(context.Background(), Request{
		Endpoint:  "https://api.example.com/search",
		Query:     "vocabulary in context",
		Limit:     5,
		TimeoutMS: 5000,
	})
	failure, ok := AsProviderError(err)
	if !ok {
		t.Fatalf("error = %v, want a structured provider failure", err)
	}
	if failure.Code != "provider_error" || !failure.RequestSent {
		t.Fatalf("failure = %+v; a started host may have posted the body", failure)
	}
	if len(failure.RequestSHA) != 64 || failure.RequestSize == 0 {
		t.Fatalf("failure request identity = %+v", failure)
	}
}

func TestHostTransportReportsAStartFailureAsUnsent(t *testing.T) {
	host := filepath.Join(t.TempDir(), "search-host.mjs")
	if err := os.WriteFile(host, []byte("// host\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		HostEnv: host,
		NodeEnv: filepath.Join(t.TempDir(), "missing-node"),
	}
	transport := HostTransport{Getenv: func(key string) string { return env[key] }}
	_, err := transport.Search(context.Background(), Request{
		Endpoint:  "https://api.example.com/search",
		Query:     "vocabulary in context",
		Limit:     5,
		TimeoutMS: 5000,
	})
	failure, ok := AsProviderError(err)
	if !ok {
		t.Fatalf("error = %v, want a structured provider failure", err)
	}
	if failure.Code != "search_unavailable" || failure.RequestSent {
		t.Fatalf("failure = %+v; a process that never started cannot have posted", failure)
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	longHost := "https://" + strings.Repeat("a", MaxEndpointHostChars+1) + "/search"
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{"https remote", "https://api.example.com/v1/search", "https://api.example.com/v1/search", ""},
		{"http loopback", "http://127.0.0.1:18768/search", "http://127.0.0.1:18768/search", ""},
		{"http localhost", "http://localhost:18768/search", "http://localhost:18768/search", ""},
		{"http ipv6", "http://[::1]:18768/search", "http://[::1]:18768/search", ""},
		{"http mapped", "http://[::ffff:127.0.0.1]:18768/search", "http://[::ffff:127.0.0.1]:18768/search", ""},
		{"http remote", "http://api.example.com/search", "", "off-loopback"},
		{"userinfo", "https://user:pass@api.example.com/search", "", "userinfo"},
		{"query", "https://api.example.com/search?key=secret", "", "query string"},
		{"fragment", "https://api.example.com/search#part", "", "fragment"},
		{"ftp", "ftp://api.example.com/search", "", "not http or https"},
		{"relative", "/search", "", "absolute"},
		{"host too long", longHost, "", "host is longer"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := NormalizeEndpoint(testCase.raw)
			if testCase.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
					t.Fatalf("NormalizeEndpoint(%q) = %q, %v; want error containing %q", testCase.raw, got, err, testCase.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeEndpoint(%q): %v", testCase.raw, err)
			}
			if got != testCase.want {
				t.Fatalf("NormalizeEndpoint(%q) = %q, want %q", testCase.raw, got, testCase.want)
			}
		})
	}
}

func TestBuildBodyIsExactAndBounded(t *testing.T) {
	body, err := BuildBody("  vocabulary   in context  ", 20)
	if err != nil {
		t.Fatal(err)
	}
	if body != `{"query":"vocabulary in context","limit":20}` {
		t.Fatalf("body = %s", body)
	}
	if _, err := BuildBody("", 20); err == nil {
		t.Fatal("empty query accepted")
	}
	if _, err := BuildBody("query", 0); err == nil {
		t.Fatal("limit 0 accepted")
	}
	if _, err := BuildBody("query", 101); err == nil {
		t.Fatal("limit 101 accepted")
	}
	if _, err := BuildBody(strings.Repeat("x", 301), 20); err == nil {
		t.Fatal("301-character query accepted")
	}
	if _, err := BuildBody(strings.Repeat("x", 300), 20); err != nil {
		t.Fatalf("300-character query refused: %v", err)
	}
}

func TestParseProviderResponse(t *testing.T) {
	body := []byte(`{
		"provider": "stub-search",
		"results": [
			{"title": "A", "url": "https://example.com/a", "content": "text", "license": "cc-by-4.0", "generated": false, "extra": 1},
			{"title": "B", "url": "http://127.0.0.1:1/page", "snippet": "snip"}
		],
		"extra": "ignored"
	}`)
	provider, items, err := ParseProviderResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if provider != "stub-search" || len(items) != 2 {
		t.Fatalf("provider/items = %q/%d", provider, len(items))
	}
	if items[0].Title != "A" || items[0].Content != "text" || items[0].License != "cc-by-4.0" || items[0].Generated {
		t.Fatalf("item 0 = %+v", items[0])
	}
	if items[1].Snippet != "snip" || items[1].Content != "" {
		t.Fatalf("item 1 = %+v", items[1])
	}
}

func TestParseProviderResponseRejectsMalformedShapes(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"not JSON", `not json`, "not JSON"},
		{"no provider", `{"results":[]}`, "provider"},
		{"null results", `{"provider":"p","results":null}`, "results array"},
		{"no results", `{"provider":"p"}`, "results array"},
		{"result not object", `{"provider":"p","results":["x"]}`, "not an object"},
		{"no title", `{"provider":"p","results":[{"url":"https://example.com/a"}]}`, "title"},
		{"title wrong type", `{"provider":"p","results":[{"title":3,"url":"https://example.com/a"}]}`, "cannot unmarshal"},
		{"no URL", `{"provider":"p","results":[{"title":"A"}]}`, "URL"},
		{"bad URL scheme", `{"provider":"p","results":[{"title":"A","url":"javascript:alert(1)"}]}`, "http or https"},
		{"userinfo URL", `{"provider":"p","results":[{"title":"A","url":"https://u:p@example.com/a"}]}`, "userinfo"},
		{"trailing data", `{"provider":"p","results":[]} {}`, "trailing"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, _, err := ParseProviderResponse([]byte(testCase.body))
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %v, want containing %q", err, testCase.want)
			}
		})
	}
}

func TestParseProviderResponseBounds(t *testing.T) {
	longTitle := strings.Repeat("t", MaxTitleRunes+1)
	body := `{"provider":"p","results":[{"title":"` + longTitle + `","url":"https://example.com/a"}]}`
	if _, _, err := ParseProviderResponse([]byte(body)); err == nil || !strings.Contains(err.Error(), "title is longer") {
		t.Fatalf("long title error = %v", err)
	}
	tooMany := `{"provider":"p","results":[` + strings.Repeat(`{"title":"A","url":"https://example.com/a"},`, MaxResults) + `{"title":"A","url":"https://example.com/a"}]}`
	if _, _, err := ParseProviderResponse([]byte(tooMany)); err == nil || !strings.Contains(err.Error(), "hard bound") {
		t.Fatalf("hard-bound error = %v", err)
	}
}

func TestEndpointHostKeepsOnlyHostAndPort(t *testing.T) {
	if got := EndpointHost("http://127.0.0.1:18768/v1/search"); got != "127.0.0.1:18768" {
		t.Fatalf("host = %q", got)
	}
	if got := EndpointHost("https://api.example.com/search"); got != "api.example.com" {
		t.Fatalf("host = %q", got)
	}
	if got := EndpointHost("https://" + strings.Repeat("a", MaxEndpointHostChars+1) + "/search"); got != "" {
		t.Fatalf("over-long host = %q, want empty", got)
	}
	if got := EndpointHost("https://" + strings.Repeat("a", MaxEndpointHostChars) + "/search"); got != strings.Repeat("a", MaxEndpointHostChars) {
		t.Fatalf("300-character host = %q", got)
	}
}

func TestResolveHostNamesTheMissingFile(t *testing.T) {
	_, err := ResolveHost(func(string) string { return "/definitely/missing/search-host.mjs" }, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "search-host.mjs") {
		t.Fatalf("error = %v", err)
	}
}
