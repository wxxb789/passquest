// Package wire names the provider wire formats an endpoint path selects. The
// provider registry and the tutor both read this one derivation, so a path
// cannot mean different formats on different routes through the app.
package wire

import (
	"net/url"
	"strings"
)

// Registry shape values. ShapeChatCompletions is the original shape; both are
// stored in the provider registry's endpoint_shape field.
const (
	ShapeChatCompletions = "openai-chat-completions"
	ShapeResponses       = "openai-responses"
)

// Format is the wire format an endpoint path selects.
type Format string

const (
	FormatChatCompletions Format = "chat-completions"
	FormatResponses       Format = "responses"
)

// Shape returns the registry shape value of this format.
func (f Format) Shape() string {
	if f == FormatResponses {
		return ShapeResponses
	}
	return ShapeChatCompletions
}

// ValidShape reports whether shape is one of the registry shape values.
func ValidShape(shape string) bool {
	return shape == ShapeChatCompletions || shape == ShapeResponses
}

// FormatOf reports the wire format the endpoint's path ends in. It reads the
// path only; the rest of the URL is the caller's validation business.
func FormatOf(raw string) (Format, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	return FormatOfPath(u.Path)
}

// FormatOfPath reports the wire format a parsed URL path ends in.
func FormatOfPath(path string) (Format, bool) {
	trimmed := strings.TrimRight(path, "/")
	switch {
	case strings.HasSuffix(trimmed, "/chat/completions"):
		return FormatChatCompletions, true
	case strings.HasSuffix(trimmed, "/responses"):
		return FormatResponses, true
	default:
		return "", false
	}
}
