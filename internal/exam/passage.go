package exam

import (
	"strings"
	"unicode/utf8"
)

// Paragraphs splits passage text into display paragraphs at blank lines.
// Line breaks inside a paragraph are kept, because imported pages carry the
// layout's hard line breaks and no other structure. Leading and trailing
// whitespace of each paragraph is trimmed and empty paragraphs are dropped.
func Paragraphs(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var out []string
	var current []string
	flush := func() {
		paragraph := strings.TrimSpace(strings.Join(current, "\n"))
		if paragraph != "" {
			out = append(out, paragraph)
		}
		current = current[:0]
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	if out == nil {
		out = []string{}
	}
	return out
}

// Span locates a verbatim excerpt in a paragraph list: the paragraph index
// and the half-open rune range inside that paragraph.
type Span struct {
	Paragraph int
	Start     int
	End       int
}

// LocateExcerpt finds the first paragraph containing excerpt verbatim and
// returns its rune offsets there. Authored evidence is a verbatim substring of
// the passage (Validate and source.PracticeReady enforce it); an excerpt that
// spans a paragraph break, or a blank one, is reported as not found rather
// than guessed.
func LocateExcerpt(paragraphs []string, excerpt string) (Span, bool) {
	excerpt = strings.TrimSpace(excerpt)
	if excerpt == "" {
		return Span{}, false
	}
	for index, paragraph := range paragraphs {
		at := strings.Index(paragraph, excerpt)
		if at < 0 {
			continue
		}
		start := utf8.RuneCountInString(paragraph[:at])
		return Span{Paragraph: index, Start: start, End: start + utf8.RuneCountInString(excerpt)}, true
	}
	return Span{}, false
}
