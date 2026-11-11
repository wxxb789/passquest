package search

import (
	"strings"
	"testing"
)

func TestNormalizeEndpoint(t *testing.T) {
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
}

func TestResolveHostNamesTheMissingFile(t *testing.T) {
	_, err := ResolveHost(func(string) string { return "/definitely/missing/search-host.mjs" }, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "search-host.mjs") {
		t.Fatalf("error = %v", err)
	}
}
