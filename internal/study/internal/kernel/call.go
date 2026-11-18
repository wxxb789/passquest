package kernel

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wxxb789/passquest/internal/tutor"
)

// Redact removes the credential value and the control characters a host
// message may carry, and bounds its length.
func Redact(value, key string) string {
	value = strings.ReplaceAll(value, key, "[redacted]")
	value = strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t':
			return r
		}
		if (r < 0x20) || (r >= 0x7f && r <= 0x9f) {
			return ' '
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	if value == "" {
		return "no detail"
	}
	runes := []rune(value)
	if len(runes) > 500 {
		value = string(runes[:500]) + "…"
	}
	return value
}

// ValidText reports whether a provider's text answer is safe to use: non-blank
// valid UTF-8 without NUL or unsafe control characters, within the response
// bound.
func ValidText(text string) bool {
	return strings.TrimSpace(text) != "" && utf8.ValidString(text) && !strings.ContainsRune(text, 0) &&
		!tutor.ContainsUnsafeControl(text) && utf8.RuneCountInString(text) <= tutor.MaxResponseChars
}

// DayStart is the start of the provider budget's UTC day.
func DayStart(now time.Time) time.Time {
	year, month, day := now.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// HostError is the code and message a host error response carries; a response
// without error detail keeps the defaults.
func HostError(resp tutor.HostResponse) (code, message string) {
	code = "host_error"
	message = "the tutor host returned no error detail"
	if resp.Error != nil {
		if tutor.ValidHostErrorCode(resp.Error.Code) {
			code = resp.Error.Code
		}
		if strings.TrimSpace(resp.Error.Message) != "" {
			message = resp.Error.Message
		}
	}
	return code, message
}
