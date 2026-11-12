package cli

import "strings"

// missingOptionExplanation is the only learner-facing text for an option the
// source did not explain. Imported book material normally explains the keyed
// option and leaves distractors unexplained; the absence stays absent in stored
// data and JSON, and becomes this sentence only at presentation time.
const missingOptionExplanation = "原书未提供该选项的解析。"

// optionExplanation returns the authored explanation for label and whether one
// exists. A blank entry counts as absent.
func optionExplanation(reasoning map[string]string, label string) (string, bool) {
	text := reasoning[label]
	if strings.TrimSpace(text) == "" {
		return "", false
	}
	return text, true
}

// optionReasoningWithText copies the explanations that carry text. JSON output
// keeps this map and lists the other labels in option_reasoning_missing.
func optionReasoningWithText(reasoning map[string]string) map[string]string {
	out := make(map[string]string, len(reasoning))
	for label, text := range reasoning {
		if strings.TrimSpace(text) != "" {
			out[label] = text
		}
	}
	return out
}
