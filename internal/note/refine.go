package note

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/wxxb789/passquest/internal/tutor"
)

// PromptVersion names the authored refinement prompt and its bounds.
const PromptVersion = "note-refine-v1"

// MaxBackRunes bounds a refined meaning-plus-example field.
const MaxBackRunes = 640

// RefineMaterial is the optional context a refinement request may carry. The
// caller decides what is permitted: for an unanswered target it passes only the
// draft's pre-answer-visible example, and this package never adds a key or
// imported text without consent.
type RefineMaterial struct {
	PassageText  string
	QuestionStem string
	Evidence     string
}

// BuildRefineRequest prepares one exact provider chat body. It is deterministic
// for the same draft and material, so the recorded SHA-256 identifies what left
// the machine.
func BuildRefineRequest(cfg tutor.Config, draft Draft, material RefineMaterial) (tutor.HostRequest, error) {
	if err := cfg.Validate(); err != nil {
		return tutor.HostRequest{}, err
	}
	user := refinementUserMessage(draft, material)
	chat := tutor.Chat{
		Model: cfg.Model,
		Messages: []tutor.Message{
			{Role: "system", Content: refinementSystemPrompt},
			{Role: "user", Content: user},
		},
		MaxTokens:   cfg.MaxOutputTokens,
		Temperature: 0,
		Stream:      false,
	}
	body, err := json.Marshal(chat)
	if err != nil {
		return tutor.HostRequest{}, fmt.Errorf("note: encode refinement request: %w", err)
	}
	return tutor.HostRequest{
		ProtocolVersion:  tutor.ProtocolVersion,
		Endpoint:         cfg.Endpoint,
		TimeoutMS:        cfg.TimeoutMS,
		MaxResponseChars: tutor.MaxResponseChars,
		Chat:             chat,
		Body:             string(body),
	}, nil
}

// RequestIdentity returns the SHA-256 and byte length of the exact body.
func RequestIdentity(body string) (string, int) {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:]), len(body)
}

const refinementSystemPrompt = `You improve one flashcard for an adult studying for a written English exam.
Return one JSON object only, with exactly two string keys: "front" and "back".
"front" is the word or exam pattern to recognise, at most 120 characters.
"back" is the meaning plus one short exam-usable example, at most 640 characters,
with a blank line between the meaning and the example. Use plain text, not HTML.
Do not invent facts that are absent from the supplied material. Do not mention
option letters or reveal an answer key when the material says the item is
unanswered. If the material is insufficient, keep the current front and back.`

func refinementUserMessage(draft Draft, material RefineMaterial) string {
	var b strings.Builder
	fmt.Fprintln(&b, "Current draft:")
	fmt.Fprintf(&b, "front: %s\n", draft.Front)
	fmt.Fprintf(&b, "back: %s\n", draft.Back)
	fmt.Fprintf(&b, "tactic: %s\n", draft.Tactic)
	fmt.Fprintf(&b, "answered: %t\n", !draft.Unanswered)
	if draft.Target.Mark != "" {
		fmt.Fprintf(&b, "learner mark: %s\n", draft.Target.Mark)
	}
	if material.PassageText != "" {
		fmt.Fprintln(&b, "\nReviewed passage excerpt:")
		fmt.Fprintln(&b, clipText(material.PassageText, 2_000))
	}
	if material.QuestionStem != "" {
		fmt.Fprintln(&b, "\nQuestion stem:")
		fmt.Fprintln(&b, clipText(material.QuestionStem, 800))
	}
	if material.Evidence != "" && !draft.Unanswered {
		fmt.Fprintln(&b, "\nReviewed evidence sentence:")
		fmt.Fprintln(&b, clipText(material.Evidence, 500))
	}
	return b.String()
}

// ParseRefinement decodes and sanitizes the model's strict JSON answer.
func ParseRefinement(text string) (front, back string, err error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return "", "", errors.New("note: refinement response has no JSON object")
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(text[start:end+1]), &payload); err != nil {
		return "", "", fmt.Errorf("note: refinement response is not a JSON string map: %w", err)
	}
	front = NormalizeMark(payload["front"])
	if front == "" {
		front = NormalizeMark(payload["Front"])
	}
	back = payload["back"]
	if strings.TrimSpace(back) == "" {
		back = payload["Back"]
	}
	if !ValidMark(front) {
		return "", "", errors.New("note: refined front is empty or too long")
	}
	back = normalizeBack(back)
	if strings.TrimSpace(back) == "" {
		return "", "", errors.New("note: refined back is empty")
	}
	if tutor.ContainsUnsafeControl(front) || tutor.ContainsUnsafeControl(back) {
		return "", "", errors.New("note: refinement contains a control character")
	}
	if utf8.RuneCountInString(back) > MaxBackRunes {
		back = clipText(back, MaxBackRunes)
	}
	return front, back, nil
}

func normalizeBack(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.ReplaceAll(value, "\t", " ")
	var lines []string
	blank := false
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(lines) > 0 && !blank {
				lines = append(lines, "")
				blank = true
			}
			continue
		}
		lines = append(lines, line)
		blank = false
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}
