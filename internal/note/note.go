// Package note owns the canonical Anki note Pass Quest derives from one study
// moment: a deterministic draft, its exact Anki field HTML, the note model and
// card template, and the text Export writes. It never contacts a provider, a
// network or Anki; the CLI resolves consent and the store owns persistence.
package note

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"strings"
	"unicode/utf8"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/skill"
)

// Origin records how a note revision was produced.
type Origin string

const (
	OriginDeterministic Origin = "deterministic"
	OriginRefined       Origin = "refined"
	OriginEdited        Origin = "edited"
)

// Target is the stable identity of one note: a content-scoped question plus the
// learner's marked wording. The same question without a mark is one note; two
// marks on one question are two notes.
type Target struct {
	ContentID      string
	ContentVersion string
	QuestionID     string
	SourceID       string
	Page           int
	AttemptID      int64
	Mark           string
	Restricted     bool
}

// Key is the canonical identity string. Fields are separated by U+001F, which
// no imported identifier may contain.
func (t Target) Key() string {
	return t.ContentID + "\x1f" + t.QuestionID + "\x1f" + strings.ToLower(NormalizeMark(t.Mark))
}

// NoteID derives a stable, readable local id from the target key. Re-deriving
// it does not require the draft row, so a repeated accept can report the same
// note id after the draft is gone.
func NoteID(targetKey string) string {
	sum := sha256.Sum256([]byte(targetKey))
	return "note-" + hex.EncodeToString(sum[:])[:16]
}

// SourceLabel is the short provenance written into the Anki Source field. It
// uses identifiers, never the imported source title, so a private title cannot
// leak through a note.
func (t Target) SourceLabel() string {
	if t.SourceID != "" {
		return fmt.Sprintf("%s · p%03d · %s", t.SourceID, t.Page, t.QuestionID)
	}
	return fmt.Sprintf("sample · %s · %s", t.ContentID, t.QuestionID)
}

// Input is everything Build may use. Build refuses a nil Question, so every
// derivation below can rely on it; PassageText may be empty.
type Input struct {
	Target         Target
	Attempt        *attempt.Attempt
	Question       *exam.Question
	PassageText    string
	ConsentGranted bool
}

// Draft is one proposed note before persistence. Front, Back and Tactic are
// plain text; Fields turns them into the exact Anki HTML values.
type Draft struct {
	Target         Target
	Origin         Origin
	Model          string
	Front          string
	Back           string
	TacticID       string
	Tactic         string
	SourceLabel    string
	Restricted     bool
	ConsentGranted bool
	Unanswered     bool
	Withheld       []string
}

// Refusal codes Build can return; the CLI maps them to exit 2.
var (
	ErrLowValue            = errors.New("note: a correct attempt is not a note signal; pass --mark for the wording worth keeping")
	ErrUnansweredTarget    = errors.New("note: an unanswered question needs --mark")
	ErrQuestionUnavailable = errors.New("note: the question is not available in this build")
)

const (
	// MaxMarkRunes bounds the learner's own wording.
	MaxMarkRunes = 120
	// MaxFrontRunes keeps the recognition prompt short on a phone.
	MaxFrontRunes = 120
	// MaxMeaningRunes bounds the explanation paragraph.
	MaxMeaningRunes = 320
	// MaxExampleRunes bounds the exam-usable example.
	MaxExampleRunes = 320
)

// Build derives the deterministic draft. It never reads a key for an
// unanswered target: a skipped attempt is unanswered for note content even
// though the attempt row records the key.
func Build(in Input) (Draft, error) {
	target := in.Target
	target.Mark = NormalizeMark(target.Mark)
	if strings.TrimSpace(target.ContentID) == "" || strings.TrimSpace(target.QuestionID) == "" {
		return Draft{}, errors.New("note: content id and question id are required")
	}
	if in.Question == nil {
		return Draft{}, ErrQuestionUnavailable
	}
	if in.Attempt == nil && target.Mark == "" {
		return Draft{}, ErrUnansweredTarget
	}
	if in.Attempt != nil && in.Attempt.Status == attempt.StatusCorrect && target.Mark == "" {
		return Draft{}, ErrLowValue
	}

	unanswered := in.Attempt == nil || in.Attempt.Status == attempt.StatusSkipped
	tacticID := tacticIDFor(in.Attempt, in.Question)
	tactic := tacticTitle(tacticID)
	lesson, hasLesson := skill.Get(tacticID)

	draft := Draft{
		Target:         target,
		Origin:         OriginDeterministic,
		Front:          frontFor(in, unanswered, tactic),
		TacticID:       tacticID,
		Tactic:         tactic,
		SourceLabel:    target.SourceLabel(),
		Restricted:     target.Restricted,
		ConsentGranted: in.ConsentGranted,
		Unanswered:     unanswered,
	}

	meaning, example := explanationFor(in, unanswered, lesson, hasLesson)
	if strings.TrimSpace(meaning) == "" {
		meaning = "Reviewed context for this item."
	}
	meaning = clipText(meaning, MaxMeaningRunes)
	if strings.TrimSpace(example) != "" {
		example = clipText(example, MaxExampleRunes)
		draft.Back = meaning + "\n\nExample: " + example
	} else {
		draft.Back = meaning
	}
	draft.Back = strings.TrimSpace(draft.Back)

	if target.Restricted && !in.ConsentGranted {
		draft.Withheld = []string{"front", "back"}
		draft.Front = ""
		draft.Back = ""
	}
	return draft, nil
}

// NormalizeMark collapses ordinary whitespace, including tabs and newlines, so
// the mark is one safe line. ValidMark reports whether the value is usable.
func NormalizeMark(raw string) string {
	return strings.Join(strings.Fields(raw), " ")
}

// ValidMark reports whether a normalized mark is usable.
func ValidMark(mark string) bool {
	if mark == "" {
		return false
	}
	return utf8.RuneCountInString(mark) <= MaxMarkRunes
}

func frontFor(in Input, unanswered bool, tactic string) string {
	if in.Target.Mark != "" {
		return clipText(in.Target.Mark, MaxFrontRunes)
	}
	if !unanswered && in.Attempt != nil {
		// An incorrect attempt's recorded key is the wording the learner missed.
		// This branch is unreachable for skipped attempts by construction.
		return clipText(in.Attempt.CorrectText, MaxFrontRunes)
	}
	if strings.TrimSpace(tactic) != "" {
		return clipText(tactic, MaxFrontRunes)
	}
	if in.Question != nil && strings.TrimSpace(in.Question.Stem) != "" {
		return clipText(in.Question.Stem, MaxFrontRunes)
	}
	return clipText(in.Target.QuestionID, MaxFrontRunes)
}

// explanationFor returns the meaning paragraph and the single example. The
// unanswered branch reads only pre-answer-visible material: the stem, the
// passage sentence around the mark and first-party lesson text.
func explanationFor(in Input, unanswered bool, lesson skill.Skill, hasLesson bool) (string, string) {
	question := in.Question
	if !unanswered && in.Attempt != nil {
		if in.Attempt.Status == attempt.StatusIncorrect {
			meaning := strings.TrimSpace(question.OptionReasoning[in.Attempt.CorrectLabel])
			if meaning == "" {
				meaning = strings.TrimSpace(question.Reasoning)
			}
			if meaning == "" {
				meaning = "The verified key is " + in.Attempt.CorrectText + "."
			}
			example := strings.TrimSpace(question.Evidence)
			if example == "" {
				example = strings.TrimSpace(question.Stem)
			}
			return meaning, example
		}
		// Correct attempt with an explicit mark: the learner already saw the key.
		meaning := strings.TrimSpace(question.Reasoning)
		if meaning == "" && hasLesson {
			meaning = strings.TrimSpace(lesson.Principle)
		}
		example := strings.TrimSpace(question.Evidence)
		if example == "" {
			example = sentenceWith(in.PassageText, in.Target.Mark)
		}
		return meaning, example
	}

	// Unanswered: no key label, key text, keyed option reasoning or question
	// reasoning is included. The key is read only to filter a passage sentence
	// that would reproduce the keyed option, then only the visible stem is kept.
	example := sentenceWith(in.PassageText, in.Target.Mark)
	if example != "" && revealsAnswer(example, *question) {
		example = strings.TrimSpace(question.Stem)
	}
	if example == "" {
		example = strings.TrimSpace(question.Stem)
	}
	if hasLesson {
		meaning := strings.TrimSpace(lesson.Principle)
		if meaning == "" {
			meaning = strings.TrimSpace(lesson.Summary)
		}
		return meaning, example
	}
	return "This item tests: " + strings.TrimSpace(question.Stem), example
}

// revealsAnswer reports whether a pre-answer passage sentence would reproduce
// the keyed option or the authored evidence sentence. It is used only to
// replace such a sentence with the visible stem; the key never enters a note.
func revealsAnswer(sentence string, question exam.Question) bool {
	lower := strings.ToLower(sentence)
	key := question.Correct()
	if key.Text != "" && strings.Contains(lower, strings.ToLower(key.Text)) {
		return true
	}
	evidence := strings.TrimSpace(question.Evidence)
	if evidence != "" {
		lowerEvidence := strings.ToLower(evidence)
		// A sentence extractor can stop at an abbreviation such as "p.m.", so a
		// prefix of the evidence must count as a reveal too.
		if strings.Contains(lower, lowerEvidence) || strings.Contains(lowerEvidence, lower) {
			return true
		}
	}
	return false
}

func tacticIDFor(record *attempt.Attempt, question *exam.Question) string {
	if record != nil && record.TacticID != "" && skill.IsRegistered(record.TacticID) {
		return record.TacticID
	}
	if record != nil {
		if id := firstScoring(record.Skills); id != "" {
			return id
		}
	}
	if question != nil {
		for _, tag := range question.Tactics {
			if candidate, ok := skill.Get(tag); ok && candidate.Kind == skill.KindScoring {
				return candidate.ID
			}
		}
		if id := firstRegistered(question.Tactics); id != "" {
			return id
		}
	}
	return ""
}

func firstScoring(ids []string) string {
	for _, id := range ids {
		if candidate, ok := skill.Get(id); ok && candidate.Kind == skill.KindScoring {
			return candidate.ID
		}
	}
	return ""
}

func firstRegistered(ids []string) string {
	for _, id := range ids {
		if skill.IsRegistered(id) {
			return id
		}
	}
	return ""
}

func tacticTitle(id string) string {
	if id == "" {
		return ""
	}
	if candidate, ok := skill.Get(id); ok {
		return candidate.Title
	}
	return id
}

// sentenceWith returns the passage sentence containing the mark, or "" when
// the mark is absent. A period inside an abbreviation such as "p.m." is not a
// sentence end: the punctuation must be followed by whitespace, or the
// boundary must be a line break. It is used only for pre-answer-visible text.
func sentenceWith(passage, mark string) string {
	mark = strings.TrimSpace(mark)
	if mark == "" || strings.TrimSpace(passage) == "" {
		return ""
	}
	index := strings.Index(strings.ToLower(passage), strings.ToLower(mark))
	if index < 0 {
		return ""
	}
	start, end := 0, len(passage)
	for i := index - 1; i >= 0; i-- {
		if passage[i] == '\n' {
			start = i + 1
			break
		}
		if (passage[i] == '.' || passage[i] == '!' || passage[i] == '?') && i+1 < len(passage) && isSpace(passage[i+1]) {
			start = i + 1
			break
		}
	}
	for i := index + len(mark); i < len(passage); i++ {
		if passage[i] == '\n' {
			end = i
			break
		}
		if (passage[i] == '.' || passage[i] == '!' || passage[i] == '?') &&
			(i+1 == len(passage) || isSpace(passage[i+1])) {
			end = i + 1
			break
		}
	}
	return strings.TrimSpace(passage[start:end])
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}

func clipText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:limit])) + "…"
}

// SanitizeFront validates a learner-edited front.
func SanitizeFront(raw string) (string, error) {
	value := NormalizeMark(raw)
	if !ValidMark(value) {
		return "", fmt.Errorf("note: front must be 1..%d characters of ordinary text", MaxMarkRunes)
	}
	return clipText(value, MaxMarkRunes), nil
}

// SanitizeTactic validates a learner-edited tactic. Empty is allowed.
func SanitizeTactic(raw string) (string, error) {
	value := NormalizeMark(raw)
	if utf8.RuneCountInString(value) > 80 {
		return "", errors.New("note: tactic must be at most 80 characters")
	}
	return value, nil
}

// SanitizeBack validates a learner-edited back and clips it to the field bound.
func SanitizeBack(raw string) (string, error) {
	value := normalizeBack(raw)
	if value == "" {
		return "", errors.New("note: back must not be empty")
	}
	return clipText(value, MaxBackRunes), nil
}

// Fields is the exact four-value Anki payload shared by preview, export and
// push, so those three surfaces cannot diverge.
type Fields struct {
	Front  string `json:"Front"`
	Back   string `json:"Back"`
	Tactic string `json:"Tactic"`
	Source string `json:"Source"`
}

// Map returns the fields in the shape AnkiConnect expects.
func (f Fields) Map() map[string]string {
	return map[string]string{"Front": f.Front, "Back": f.Back, "Tactic": f.Tactic, "Source": f.Source}
}

// Hash identifies one exact field payload.
func (f Fields) Hash() string {
	return hashStrings(f.Front, f.Back, f.Tactic, f.Source)
}

// HTML renders plain note text into the exact field HTML. Only the template
// supplies markup; note text is escaped, newlines become <br> and blank lines
// in Back become the meaning/example paragraphs.
func HTML(front, back, tactic, source string) Fields {
	paragraphs := splitParagraphs(back)
	var builder strings.Builder
	for i, paragraph := range paragraphs {
		class := "pq-meaning"
		if i > 0 {
			class = "pq-example"
		}
		fmt.Fprintf(&builder, `<div class="%s">%s</div>`, class, escapeBlock(paragraph))
	}
	return Fields{
		Front:  escapeBlock(front),
		Back:   builder.String(),
		Tactic: escapeBlock(tactic),
		Source: escapeBlock(source),
	}
}

// ContentHash identifies the accepted content. Origin and model are provenance,
// not content, so a re-accept after only a model change is still unchanged.
func ContentHash(front, back, tactic, source string, restricted bool) string {
	return hashStrings(front, back, tactic, source, fmt.Sprintf("%t", restricted))
}

func hashStrings(values ...string) string {
	h := sha256.New()
	for _, value := range values {
		fmt.Fprintf(h, "%d:%s", len(value), value)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func splitParagraphs(value string) []string {
	normalized := strings.ReplaceAll(value, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	raw := strings.Split(normalized, "\n\n")
	out := make([]string, 0, len(raw))
	for _, paragraph := range raw {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		out = append(out, paragraph)
	}
	return out
}

// escapeBlock escapes text and turns line breaks into <br>. Tabs are ordinary
// whitespace in a field; they become one space so a tab cannot break TSV.
func escapeBlock(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.ReplaceAll(value, "\t", " ")
	escaped := html.EscapeString(value)
	return strings.ReplaceAll(escaped, "\n", "<br>")
}
