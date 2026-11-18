package study

import (
	"sort"
	"strings"

	"github.com/wxxb789/passquest/internal/source"
)

// OptionExplanation returns the authored explanation for label and whether one
// exists. A blank entry counts as absent. Both adapters render it: the CLI's
// human feedback and report views, and the desktop's feedback mapper.
func OptionExplanation(reasoning map[string]string, label string) (string, bool) {
	text := reasoning[label]
	if strings.TrimSpace(text) == "" {
		return "", false
	}
	return text, true
}

// OptionReasoningTexts keeps the option explanations that carry text, under
// the one definition OptionExplanation gives a single label. JSON documents
// keep this map and list the other labels in option_reasoning_missing; the
// returned map is never nil, because the source question document carries the
// key without omitempty.
func OptionReasoningTexts(reasoning map[string]string) map[string]string {
	out := make(map[string]string, len(reasoning))
	for label := range reasoning {
		if text, ok := OptionExplanation(reasoning, label); ok {
			out[label] = text
		}
	}
	return out
}

// DisplayFlags prints one question's or page's flags the way the CLI's source
// views do; the text is also part of the practice module's not-practiceable
// refusal.
func DisplayFlags(flags []source.Flag) string {
	if len(flags) == 0 {
		return "—"
	}
	values := source.FlagStrings(flags)
	sort.Strings(values)
	return strings.Join(values, ",")
}
