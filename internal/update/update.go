// Package update answers one question, and only when the learner asks it:
// does a newer Pass Quest release exist?
//
// It is the app's one network package: one GET to this repository's GitHub
// Releases endpoint, with the two headers below, no cookie, no query, no
// body, no redirect and no proxy, and nothing about the learner or the
// machine. It downloads nothing, installs nothing and writes nothing.
//
// Result carries codes instead of prose, because the window shows either
// language: State says what the check found, and Reason says why a check
// that could not answer did not. Both are closed sets, the constants below.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Version is the running app version. A test fails when mygo.json differs;
// the release check compares it against the latest published tag.
const Version = "0.15.0"

// Endpoint is the one URL the app contacts: this repository's latest
// published release. /releases/latest already excludes drafts and
// pre-releases, so it is the newest release a learner can install.
const Endpoint = "https://api.github.com/repos/wxxb789/passquest/releases/latest"

// releasesURL is the prefix of every link this package emits. The window's
// release links are built from the tag under this prefix and never taken
// from the response, so the window cannot offer a link out of this
// repository's releases.
const releasesURL = "https://github.com/wxxb789/passquest/releases/"

// DefaultTimeout bounds one check from the request to the end of the body.
const DefaultTimeout = 10 * time.Second

// The smoke-only overrides. A scripted launch may point the check at a
// loopback stub and shorten its timeout; both variables are read only when
// the smoke hook is set (PASSQUEST_DESKTOP_SCRIPT), and a value that is not
// exactly an http://127.0.0.1:<port> URL, or a timeout outside
// 1..smokeTimeoutMaxMS milliseconds, is ignored. A packaged run reads
// neither variable and can only reach Endpoint.
const (
	SmokeEnvURL       = "PASSQUEST_DESKTOP_UPDATE_URL"
	SmokeEnvTimeoutMS = "PASSQUEST_DESKTOP_UPDATE_TIMEOUT_MS"
)

// smokeTimeoutMaxMS caps the smoke-only timeout at the real check's own
// timeout: the scenario wants a fast timeout state, never a longer one than a
// real check has.
const smokeTimeoutMaxMS = int(DefaultTimeout / time.Millisecond)

// maxBody caps the response the app reads (1 MiB). The real answer is a few
// kilobytes; a larger one is not the endpoint's document.
const maxBody = 1 << 20

// The whole identifying surface of the request, and the only headers it
// carries beside Go's own Accept-Encoding. No version, no device and no
// learner field: the release check says nothing about who asks.
const (
	acceptHeader = "application/vnd.github+json"
	userAgent    = "PassQuest"
)

// Status is what one check found.
type Status string

const (
	// StatusNewer: a newer release exists; Version, Published, NotesURL and
	// DownloadURL describe it.
	StatusNewer Status = "newer"
	// StatusCurrent: the running version is the latest published release.
	StatusCurrent Status = "current"
	// StatusNoRelease: this repository has published no release yet, so
	// there is nothing newer to find (HTTP 404).
	StatusNoRelease Status = "no_release"
	// StatusUnavailable: the check could not answer; Reason says why. Study
	// is never blocked by this.
	StatusUnavailable Status = "unavailable"
)

// Reason is why a check could not answer. It is empty whenever one did.
type Reason string

const (
	ReasonTimeout     Reason = "timeout"      // the deadline passed: no answer in time
	ReasonOffline     Reason = "offline"      // the request never reached the endpoint
	ReasonRateLimited Reason = "rate_limited" // HTTP 403 or 429: GitHub refused this check
	ReasonServer      Reason = "server_error" // HTTP 5xx: GitHub could not answer
	ReasonUnexpected  Reason = "unexpected"   // an HTTP status with no meaning here
	ReasonRedirect    Reason = "redirect"     // the endpoint answered with a redirect, which is refused
	ReasonTooLarge    Reason = "too_large"    // the body exceeded the cap
	ReasonMalformed   Reason = "malformed"    // the body was not the release document
	ReasonInvalidTag  Reason = "invalid_tag"  // a tag that is not a version
)

// Result is one check's outcome, and the only thing the window shows.
type Result struct {
	// State is what the check found, one of the Status values.
	State Status `json:"state"`
	// Version is the newest release's tag, as published (e.g. "v0.15.1").
	Version string `json:"version,omitempty"`
	// Published is that release's publication date, UTC, YYYY-MM-DD.
	Published string `json:"published,omitempty"`
	// NotesURL is the release's notes page, built from the tag and never
	// taken from the response: whatever html_url the document carries, the
	// window gets this repository's release URL.
	NotesURL string `json:"notes_url,omitempty"`
	// DownloadURL is the release page built from the tag, where the release
	// and its files are listed; the same page as NotesURL. The app downloads
	// nothing from it.
	DownloadURL string `json:"download_url,omitempty"`
	// Reason is why the check could not answer, empty when it did.
	Reason Reason `json:"reason,omitempty"`
}

// Target is where one check goes and how long it may take. The endpoint and
// the timeout are unexported, so no caller can name a URL of its own:
// DefaultTarget returns the constant endpoint, and TargetFor returns it too
// unless the smoke hook is set and the environment carries a
// http://127.0.0.1:<port> override. Who may import this package at all is
// the local-only gate's rule 3: internal/desktop and cmd/passquest-desktop.
type Target struct {
	endpoint string
	timeout  time.Duration
}

// DefaultTarget is the constant endpoint and timeout of a packaged run.
func DefaultTarget() Target {
	return Target{endpoint: Endpoint, timeout: DefaultTimeout}
}

// TargetFor returns the target of one launch: DefaultTarget, or the
// smoke-only override when the smoke hook is set. getenv is read only in
// that case, and only a loopback http URL and a sane timeout are honoured.
func TargetFor(getenv func(string) string, smokeHook bool) Target {
	target := DefaultTarget()
	if !smokeHook {
		return target
	}
	if raw := getenv(SmokeEnvURL); loopbackURL(raw) {
		target.endpoint = raw
	}
	if raw := getenv(SmokeEnvTimeoutMS); raw != "" {
		if ms, err := strconv.Atoi(raw); err == nil && ms >= 1 && ms <= smokeTimeoutMaxMS {
			target.timeout = time.Duration(ms) * time.Millisecond
		}
	}
	return target
}

// loopbackURL reports whether raw is an http URL on 127.0.0.1 with a
// numeric port and nothing else: the only endpoint a scripted launch may
// redirect the check to. Another host, https, a userinfo, a query or a
// fragment is ignored, so the variable cannot send a check anywhere but the
// stub inside the harness process. The net package is not used: this
// package's one network import is net/http.
func loopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	host, port, ok := strings.Cut(u.Host, ":")
	if !ok || host != "127.0.0.1" || port == "" {
		return false
	}
	for _, digit := range port {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

// NewClient returns the client one check uses: the environment's proxy
// settings are ignored (the request goes to the endpoint it names or
// nowhere), a redirect is refused and reported rather than followed, and the
// deadline is the target's own timeout, applied by Check.
func NewClient() *http.Client {
	return &http.Client{
		Transport:     &http.Transport{Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// Check asks the target's endpoint once, within the target's timeout, for
// the latest release and decides whether it is newer than current. It sets
// exactly the two headers the request carries, sends no body and reads at
// most maxBody. It closes the client's idle connections on every return: one
// check keeps no socket beside the window. A failure is a Result, never an
// error: the window reports the state and study continues.
func Check(ctx context.Context, client *http.Client, target Target, current string) Result {
	ctx, cancel := context.WithTimeout(ctx, target.timeout)
	defer cancel()
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.endpoint, nil)
	if err != nil {
		return failed(ReasonUnexpected)
	}
	request.Header.Set("Accept", acceptHeader)
	request.Header.Set("User-Agent", userAgent)
	response, err := client.Do(request)
	if err != nil {
		return failed(reasonFor(ctx, err))
	}
	defer response.Body.Close()
	switch code := response.StatusCode; {
	case code == http.StatusNotFound:
		return Result{State: StatusNoRelease}
	case code == http.StatusForbidden || code == http.StatusTooManyRequests:
		return failed(ReasonRateLimited)
	case code >= 300 && code < 400:
		return failed(ReasonRedirect)
	case code >= 500:
		return failed(ReasonServer)
	case code != http.StatusOK:
		return failed(ReasonUnexpected)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return failed(reasonFor(ctx, err))
	}
	if len(body) > maxBody {
		return failed(ReasonTooLarge)
	}
	var release releaseDocument
	if err := json.Unmarshal(body, &release); err != nil {
		return failed(ReasonMalformed)
	}
	return decide(release, current)
}

// reasonFor classifies a transport or read failure: a context that is out
// of time, or the client's own deadline, is a timeout; anything else never
// reached the endpoint.
func reasonFor(ctx context.Context, err error) Reason {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ReasonTimeout
	}
	var timeout interface{ Timeout() bool }
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
		return ReasonTimeout
	}
	return ReasonOffline
}

// releaseDocument is the part of the release document the check reads.
type releaseDocument struct {
	TagName     string `json:"tag_name"`
	PublishedAt string `json:"published_at"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
}

// decide turns the release document into the result. The latest endpoint
// excludes drafts and pre-releases; an entry carrying either flag anyway is
// not the newest release a learner can install, and is not newer.
func decide(release releaseDocument, current string) Result {
	if release.Draft || release.Prerelease {
		return Result{State: StatusCurrent}
	}
	latest, ok := parseVersion(release.TagName)
	if !ok {
		return failed(ReasonInvalidTag)
	}
	running, ok := parseVersion(current)
	if !ok {
		return failed(ReasonInvalidTag)
	}
	if compareVersions(latest, running) <= 0 {
		return Result{State: StatusCurrent}
	}
	published, err := time.Parse(time.RFC3339, release.PublishedAt)
	if err != nil {
		return failed(ReasonMalformed)
	}
	page := releasePage(release.TagName)
	return Result{
		State:       StatusNewer,
		Version:     release.TagName,
		Published:   published.UTC().Format("2006-01-02"),
		NotesURL:    page,
		DownloadURL: page,
	}
}

// releasePage is the release's own page, built from the tag: the only source
// of the links the window shows, and safe in a URL path because parseVersion
// has already restricted the tag to semver - no slash, no space, no dot
// segment and no escape.
func releasePage(tag string) string { return releasesURL + "tag/" + tag }

// version is a parsed release tag: MAJOR.MINOR.PATCH and an optional
// pre-release suffix. Build metadata never orders, so it is dropped.
type version struct {
	major, minor, patch int
	prerelease          string
}

// parseVersion reads "1.2.3" and "v1.2.3", each with an optional "-suffix"
// and "+build" tail. The leading v is GitHub's habit; everything else is
// semver, and anything that is not is not a version this app can compare. The
// suffix and the tail must be semver identifier lists, so a tag that passes
// here is also safe in a URL path: a tag carrying a slash, a space or a dot
// segment of its own is not a version.
func parseVersion(raw string) (version, bool) {
	text := strings.TrimPrefix(raw, "v")
	text, build, hasBuild := strings.Cut(text, "+")
	core, prerelease, hasPrerelease := strings.Cut(text, "-")
	if hasPrerelease && !isIdentifierList(prerelease) {
		return version{}, false
	}
	if hasBuild && !isIdentifierList(build) {
		return version{}, false
	}
	fields := strings.Split(core, ".")
	if len(fields) != 3 {
		return version{}, false
	}
	numbers := [3]int{}
	for index, field := range fields {
		for _, digit := range field {
			if digit < '0' || digit > '9' {
				return version{}, false
			}
		}
		if field == "" || (len(field) > 1 && field[0] == '0') {
			return version{}, false
		}
		number, err := strconv.Atoi(field)
		if err != nil {
			return version{}, false
		}
		numbers[index] = number
	}
	return version{major: numbers[0], minor: numbers[1], patch: numbers[2], prerelease: prerelease}, true
}

// isIdentifierList reports whether text is a semver identifier list: one or
// more non-empty identifiers of [0-9A-Za-z-], separated by dots.
func isIdentifierList(text string) bool {
	for _, identifier := range strings.Split(text, ".") {
		if identifier == "" {
			return false
		}
		for _, character := range identifier {
			switch {
			case character >= '0' && character <= '9',
				character >= 'a' && character <= 'z',
				character >= 'A' && character <= 'Z',
				character == '-':
			default:
				return false
			}
		}
	}
	return true
}

// compareVersions orders two versions by semver precedence: the numbers
// first, then a pre-release below its release. Two pre-release suffixes
// compare as text, which is enough here because the running version never
// carries one.
func compareVersions(candidate, running version) int {
	if candidate.major != running.major {
		return candidate.major - running.major
	}
	if candidate.minor != running.minor {
		return candidate.minor - running.minor
	}
	if candidate.patch != running.patch {
		return candidate.patch - running.patch
	}
	switch {
	case candidate.prerelease == running.prerelease:
		return 0
	case candidate.prerelease == "":
		return 1
	case running.prerelease == "":
		return -1
	}
	return strings.Compare(candidate.prerelease, running.prerelease)
}

// failed is the "could not check" result.
func failed(reason Reason) Result {
	return Result{State: StatusUnavailable, Reason: reason}
}
