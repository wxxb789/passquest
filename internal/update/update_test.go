package update

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func envOf(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

// releaseDoc is a release document with the fields the endpoint always
// serves; tag and published are the parts a case varies.
func releaseDoc(tag, published string) string {
	document, err := json.Marshal(map[string]any{
		"tag_name":     tag,
		"html_url":     "https://github.com/wxxb789/passquest/releases/tag/" + tag,
		"published_at": published,
		"draft":        false,
		"prerelease":   false,
	})
	if err != nil {
		panic(err)
	}
	return string(document)
}

// serve answers every request with one status, headers and body.
func serve(t *testing.T, status int, headers map[string]string, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for name, value := range headers {
			w.Header().Set(name, value)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

const latestPath = "/repos/wxxb789/passquest/releases/latest"

// targetFor returns the target a loopback test server answers: its URL under
// the endpoint's own path, with the given timeout. Targets are built here
// because only this package may choose the endpoint a check reaches.
func targetFor(server *httptest.Server, timeout time.Duration) Target {
	return Target{endpoint: server.URL + latestPath, timeout: timeout}
}

func check(t *testing.T, server *httptest.Server, current string) Result {
	t.Helper()
	return Check(context.Background(), NewClient(), targetFor(server, DefaultTimeout), current)
}

// TestCheckStates pins one case per state: what the window shows for a newer
// release, for the running version, for anything that is not newer, and for
// every way the check can fail.
func TestCheckStates(t *testing.T) {
	page := "https://github.com/wxxb789/passquest/releases/tag/v0.15.1"
	cases := []struct {
		name    string
		status  int
		headers map[string]string
		body    string
		current string
		want    Result
	}{
		{
			name:    "a newer release",
			current: "0.15.0",
			status:  http.StatusOK,
			body:    releaseDoc("v0.15.1", "2026-11-18T21:04:05Z"),
			want: Result{
				State:       StatusNewer,
				Version:     "v0.15.1",
				Published:   "2026-11-18",
				NotesURL:    page,
				DownloadURL: page,
			},
		},
		{
			name:    "the running version is the latest",
			current: "0.15.0",
			status:  http.StatusOK,
			body:    releaseDoc("v0.15.0", "2026-11-01T00:00:00Z"),
			want:    Result{State: StatusCurrent},
		},
		{
			name:   "an older tag",
			status: http.StatusOK,
			body:   releaseDoc("v0.14.0", "2026-10-01T00:00:00Z"),
			want:   Result{State: StatusCurrent},
		},
		{
			name:   "a pre-release entry",
			status: http.StatusOK,
			body:   `{"tag_name":"v0.16.0","published_at":"2026-11-18T00:00:00Z","prerelease":true}`,
			want:   Result{State: StatusCurrent},
		},
		{
			name:   "a draft entry",
			status: http.StatusOK,
			body:   `{"tag_name":"v0.16.0","published_at":"2026-11-18T00:00:00Z","draft":true}`,
			want:   Result{State: StatusCurrent},
		},
		{
			name:   "no release published yet",
			status: http.StatusNotFound,
			body:   `{"message":"Not Found"}`,
			want:   Result{State: StatusNoRelease},
		},
		{
			name:    "a rate limit, 403",
			status:  http.StatusForbidden,
			headers: map[string]string{"X-Ratelimit-Remaining": "0"},
			body:    `{"message":"API rate limit exceeded"}`,
			want:    failed(ReasonRateLimited),
		},
		{
			name:   "a rate limit, 429",
			status: http.StatusTooManyRequests,
			body:   `{"message":"Too many requests"}`,
			want:   failed(ReasonRateLimited),
		},
		{
			name:   "a server error",
			status: http.StatusInternalServerError,
			body:   `{"message":"Internal Server Error"}`,
			want:   failed(ReasonServer),
		},
		{
			name:   "an unexpected status",
			status: http.StatusTeapot,
			want:   failed(ReasonUnexpected),
		},
		{
			name:   "a malformed body",
			status: http.StatusOK,
			body:   `{"tag_name":`,
			want:   failed(ReasonMalformed),
		},
		{
			name:   "an empty body",
			status: http.StatusOK,
			body:   "",
			want:   failed(ReasonMalformed),
		},
		{
			name:   "an invalid tag",
			status: http.StatusOK,
			body:   releaseDoc("release-one", "2026-11-18T00:00:00Z"),
			want:   failed(ReasonInvalidTag),
		},
		{
			name:   "a tag with a path in its suffix",
			status: http.StatusOK,
			body:   releaseDoc("v9.0.0-x/../../../other", "2026-11-18T00:00:00Z"),
			want:   failed(ReasonInvalidTag),
		},
		{
			name:   "a tag with an encoded dot segment in its suffix",
			status: http.StatusOK,
			body:   releaseDoc("v9.0.0-%2e%2e", "2026-11-18T00:00:00Z"),
			want:   failed(ReasonInvalidTag),
		},
		{
			name:   "a tag with a dot segment in its suffix",
			status: http.StatusOK,
			body:   releaseDoc("v9.0.0-a..b", "2026-11-18T00:00:00Z"),
			want:   failed(ReasonInvalidTag),
		},
		{
			name:   "a tag with markup in its suffix",
			status: http.StatusOK,
			body:   releaseDoc(`v9.0.0-"><img src=x onerror=alert(1)>`, "2026-11-18T00:00:00Z"),
			want:   failed(ReasonInvalidTag),
		},
		{
			name:   "a tag with a path in its build tail",
			status: http.StatusOK,
			body:   releaseDoc("v9.0.0+x/../../evil", "2026-11-18T00:00:00Z"),
			want:   failed(ReasonInvalidTag),
		},
		{
			name:   "a newer release with a build tail",
			status: http.StatusOK,
			body:   releaseDoc("v0.16.0+build.7", "2026-11-18T00:00:00Z"),
			want: Result{
				State:       StatusNewer,
				Version:     "v0.16.0+build.7",
				Published:   "2026-11-18",
				NotesURL:    "https://github.com/wxxb789/passquest/releases/tag/v0.16.0+build.7",
				DownloadURL: "https://github.com/wxxb789/passquest/releases/tag/v0.16.0+build.7",
			},
		},
		{
			name:   "an unreadable publication date",
			status: http.StatusOK,
			body:   releaseDoc("v0.16.0", "yesterday"),
			want:   failed(ReasonMalformed),
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			server := serve(t, testCase.status, testCase.headers, testCase.body)
			current := testCase.current
			if current == "" {
				current = Version
			}
			if got := check(t, server, current); got != testCase.want {
				t.Fatalf("Check = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

// TestCheckCapsTheBody proves a response over the cap is not read as a
// release: the body is truncated at the cap and reported as too large.
func TestCheckCapsTheBody(t *testing.T) {
	server := serve(t, http.StatusOK, nil, strings.Repeat("a", maxBody+1))
	got := check(t, server, Version)
	if got != failed(ReasonTooLarge) {
		t.Fatalf("Check = %+v, want %+v", got, failed(ReasonTooLarge))
	}
}

// TestCheckTimeout proves the target's timeout ends a stalled answer: the
// request never gets headers, and the result is the timeout state.
func TestCheckTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(server.Close)
	if got := Check(context.Background(), NewClient(), targetFor(server, 100*time.Millisecond), Version); got != failed(ReasonTimeout) {
		t.Fatalf("Check = %+v, want %+v", got, failed(ReasonTimeout))
	}
}

// TestCheckStalledBodyTimeout proves the deadline covers the body too: the
// headers arrive, the rest does not, and a caller's shorter context deadline
// still bounds a check whose target timeout is longer.
func TestCheckStalledBodyTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if got := Check(ctx, NewClient(), targetFor(server, DefaultTimeout), Version); got != failed(ReasonTimeout) {
		t.Fatalf("Check = %+v, want %+v", got, failed(ReasonTimeout))
	}
}

// TestCheckRefusedConnection proves an endpoint nothing listens on is
// reported as offline, not as a version.
func TestCheckRefusedConnection(t *testing.T) {
	server := serve(t, http.StatusOK, nil, releaseDoc("v0.16.0", "2026-11-18T00:00:00Z"))
	target := targetFor(server, DefaultTimeout)
	server.Close()
	if got := Check(context.Background(), NewClient(), target, Version); got != failed(ReasonOffline) {
		t.Fatalf("Check = %+v, want %+v", got, failed(ReasonOffline))
	}
}

// TestCheckLeavesNoIdleConnection proves one check keeps nothing alive: the
// client's idle connections are closed when Check returns, so a learner's
// click cannot leave a socket and its read loop waiting beside the window.
func TestCheckLeavesNoIdleConnection(t *testing.T) {
	closed := make(chan struct{}, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, releaseDoc("v0.15.0", "2026-11-01T00:00:00Z"))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			select {
			case closed <- struct{}{}:
			default:
			}
		}
	}
	server.Start()
	t.Cleanup(server.Close)
	if got := check(t, server, Version); got.State != StatusCurrent {
		t.Fatalf("Check = %+v, want the current state", got)
	}
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the connection outlived the check that used it")
	}
}

// TestCheckRefusesRedirects proves the endpoint cannot send the check
// elsewhere: the redirect is reported and the target never sees a request.
func TestCheckRefusesRedirects(t *testing.T) {
	reached := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached++
		_, _ = io.WriteString(w, releaseDoc("v0.16.0", "2026-11-18T00:00:00Z"))
	}))
	t.Cleanup(target.Close)
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	t.Cleanup(redirect.Close)
	if got := Check(context.Background(), NewClient(), targetFor(redirect, DefaultTimeout), Version); got != failed(ReasonRedirect) {
		t.Fatalf("Check = %+v, want %+v", got, failed(ReasonRedirect))
	}
	if reached != 0 {
		t.Fatalf("the redirect target saw %d requests, want none", reached)
	}
}

// capturedRequest is every part of one request the server can see.
type capturedRequest struct {
	method  string
	path    string
	query   string
	headers http.Header
	body    string
	cookies []string
}

// TestCheckCapturesTheRequest pins the request itself: the one method, the
// one path, no query, no body, no cookie, and exactly the two headers the
// app sets beside Go's Accept-Encoding. Any addition - a device id, a
// learner field, a parameter - fails here.
func TestCheckCapturesTheRequest(t *testing.T) {
	requests := make(chan capturedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var cookies []string
		for _, cookie := range r.Cookies() {
			cookies = append(cookies, cookie.Name+"="+cookie.Value)
		}
		requests <- capturedRequest{
			method:  r.Method,
			path:    r.URL.Path,
			query:   r.URL.RawQuery,
			headers: r.Header.Clone(),
			body:    string(body),
			cookies: cookies,
		}
		_, _ = io.WriteString(w, releaseDoc("v0.15.0", "2026-11-01T00:00:00Z"))
	}))
	t.Cleanup(server.Close)
	if got := check(t, server, Version); got.State != StatusCurrent {
		t.Fatalf("Check = %+v, want the current state", got)
	}
	got := <-requests
	if got.method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.method)
	}
	if got.path != latestPath {
		t.Errorf("path = %q, want %q", got.path, latestPath)
	}
	if got.query != "" {
		t.Errorf("query = %q, want none", got.query)
	}
	if got.body != "" {
		t.Errorf("body = %q, want none", got.body)
	}
	if len(got.cookies) != 0 {
		t.Errorf("cookies = %v, want none", got.cookies)
	}
	want := map[string]string{
		"Accept":          "application/vnd.github+json",
		"User-Agent":      "PassQuest",
		"Accept-Encoding": "gzip",
	}
	if len(got.headers) != len(want) {
		t.Errorf("headers = %v, want exactly %v", got.headers, want)
	}
	for name, value := range want {
		if got.headers.Get(name) != value {
			t.Errorf("header %s = %q, want %q", name, got.headers.Get(name), value)
		}
	}
}

// TestLinksComeFromTheRepository proves the window's links are built from
// the tag, never taken from the response: whatever html_url the document
// carries - another repository, another host, plain http, javascript:, a
// query or fragment, or a dot segment spelled with backslashes, which a
// browser normalises like slashes - the result carries this repository's
// release page.
func TestLinksComeFromTheRepository(t *testing.T) {
	page := "https://github.com/wxxb789/passquest/releases/tag/v0.16.0"
	cases := []struct {
		name    string
		htmlURL string
		want    string
	}{
		{"the release page", page, page},
		{"the release page with a fragment", page + "#notes", page},
		{"another repository", "https://github.com/evil/passquest/releases/tag/v0.16.0", page},
		{"another host", "https://example.com/wxxb789/passquest/releases/", page},
		{"plain http", "http://github.com/wxxb789/passquest/releases/tag/v0.16.0", page},
		{"javascript", "javascript:alert(1)", page},
		{"a backslash dot segment", `https://github.com/wxxb789/passquest/releases/\..\..\evil`, page},
		{"an encoded dot segment", "https://github.com/wxxb789/passquest/releases/%2e%2e/%2e%2e/wxxb789/evil/releases/tag/v9.0.0", page},
		{"no link at all", "", page},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			document, err := json.Marshal(map[string]any{
				"tag_name":     "v0.16.0",
				"html_url":     testCase.htmlURL,
				"published_at": "2026-11-18T00:00:00Z",
			})
			if err != nil {
				t.Fatal(err)
			}
			server := serve(t, http.StatusOK, nil, string(document))
			want := Result{
				State:       StatusNewer,
				Version:     "v0.16.0",
				Published:   "2026-11-18",
				NotesURL:    testCase.want,
				DownloadURL: page,
			}
			if got := check(t, server, Version); got != want {
				t.Fatalf("Check = %+v, want %+v", got, want)
			}
		})
	}
}

// TestVersionMatchesMygoJSON proves the two versions cannot drift: the
// package name and the comparison base are the same number.
func TestVersionMatchesMygoJSON(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "mygo.json"))
	if err != nil {
		t.Fatalf("read mygo.json: %v", err)
	}
	var config struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse mygo.json: %v", err)
	}
	if config.Version != Version {
		t.Fatalf("mygo.json version = %q, update.Version = %q; the release check would compare against a version nobody runs", config.Version, Version)
	}
	if _, ok := parseVersion(Version); !ok {
		t.Fatalf("update.Version %q is not a version this package can compare", Version)
	}
}

// TestTargetForSmokeOverride proves the smoke-only override: with the smoke
// hook set, a loopback stub URL and a short timeout are honoured; without
// it, and for anything that is not http://127.0.0.1:<port>, both are the
// constants.
func TestTargetForSmokeOverride(t *testing.T) {
	stub := "http://127.0.0.1:45123/repos/wxxb789/passquest/releases/latest"
	env := envOf(map[string]string{SmokeEnvURL: stub, SmokeEnvTimeoutMS: "250"})
	if got, want := TargetFor(env, false), DefaultTarget(); got != want {
		t.Fatalf("TargetFor without the smoke hook = %+v, want %+v", got, want)
	}
	got := TargetFor(env, true)
	if got.endpoint != stub || got.timeout != 250*time.Millisecond {
		t.Fatalf("TargetFor = %+v, want the stub endpoint and 250ms", got)
	}
	// The cap is the real check's own timeout: the millisecond under it is
	// honoured, the one past it is not, so no scripted launch waits longer
	// than a real check.
	under := strconv.Itoa(smokeTimeoutMaxMS - 1)
	if got := TargetFor(envOf(map[string]string{SmokeEnvTimeoutMS: under}), true); got.timeout != time.Duration(smokeTimeoutMaxMS-1)*time.Millisecond {
		t.Fatalf("TargetFor(timeout %s) = %v, want %v", under, got.timeout, time.Duration(smokeTimeoutMaxMS-1)*time.Millisecond)
	}
	over := strconv.Itoa(smokeTimeoutMaxMS + 1)
	if got := TargetFor(envOf(map[string]string{SmokeEnvTimeoutMS: over}), true); got.timeout != DefaultTimeout {
		t.Fatalf("TargetFor(timeout %s) = %v, want %v", over, got.timeout, DefaultTimeout)
	}

	refused := []string{
		"https://127.0.0.1:45123/x",
		"http://localhost:45123/x",
		"http://[::1]:45123/x",
		"http://127.0.0.1/x",
		"http://127.0.0.1:port/x",
		"http://user@127.0.0.1:45123/x",
		"http://example.com:45123/x",
		"ftp://127.0.0.1:45123/x",
		stub + "?step=1",
		stub + "#step",
		"",
	}
	for _, raw := range refused {
		if got := TargetFor(envOf(map[string]string{SmokeEnvURL: raw}), true); got.endpoint != Endpoint {
			t.Errorf("TargetFor(%q) = %q, want the constant endpoint", raw, got.endpoint)
		}
	}
	for _, raw := range []string{"0", "-5", "abc", "60001", "1e3", ""} {
		if got := TargetFor(envOf(map[string]string{SmokeEnvTimeoutMS: raw}), true); got.timeout != DefaultTimeout {
			t.Errorf("TargetFor(timeout %q) = %v, want %v", raw, got.timeout, DefaultTimeout)
		}
	}
}

// TestVersionOrder pins the semver reading: the numbers order, a leading v
// does not matter, build metadata never orders, and a pre-release is below
// its release.
func TestVersionOrder(t *testing.T) {
	cases := []struct {
		candidate string
		running   string
		want      int
	}{
		{"0.15.0", "0.15.0", 0},
		{"v0.15.0", "0.15.0", 0},
		{"0.15.0+build.7", "0.15.0", 0},
		{"0.15.1", "0.15.0", 1},
		{"0.16.0", "0.15.9", 1},
		{"1.0.0", "0.99.99", 1},
		{"0.14.9", "0.15.0", -1},
		{"0.15.1-rc1", "0.15.0", 1},
		{"0.15.1-rc.1", "0.15.0", 1},
		{"0.15.0-rc1", "0.15.0", -1},
	}
	for _, testCase := range cases {
		candidate, ok := parseVersion(testCase.candidate)
		if !ok {
			t.Errorf("parseVersion(%q) failed", testCase.candidate)
			continue
		}
		running, ok := parseVersion(testCase.running)
		if !ok {
			t.Errorf("parseVersion(%q) failed", testCase.running)
			continue
		}
		if got := compareVersions(candidate, running); got != testCase.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", testCase.candidate, testCase.running, got, testCase.want)
		}
	}
	invalid := []string{
		"", "1", "1.2", "1.2.3.4", "one.two.three", "01.2.3", "1.2.3-", "v", "-1.2.3", "1.2.3rc",
		" 1.2.3", "1.2.3 ", "1.2.3-a b", "1.2.3-x/..", "1.2.3-%2e%2e", "1.2.3-a..b", "1.2.3-a.", "1.2.3-.a",
		"1.2.3+", "1.2.3+x/y",
	}
	for _, raw := range invalid {
		if _, ok := parseVersion(raw); ok {
			t.Errorf("parseVersion(%q) accepted a non-version", raw)
		}
	}
}
