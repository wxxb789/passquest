// Package exam owns the authored practice content and the checks that make an
// answer key inspectable. Content is inert data: it carries no learner state and
// makes no claim about any live examination.
package exam

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// SchemaVersion is the shape of the authored content file accepted by Parse.
const SchemaVersion = 1

// Exam is one coherent exercise: a passage and questions about it.
type Exam struct {
	SchemaVersion int        `json:"schema_version"`
	ID            string     `json:"id"`
	Version       string     `json:"version"`
	Title         string     `json:"title"`
	Notice        string     `json:"notice"`
	Provenance    Provenance `json:"provenance"`
	Passage       Passage    `json:"passage"`
	Questions     []Question `json:"questions"`
}

// Provenance records where the content came from and what it may be used for.
type Provenance struct {
	Author       string `json:"author"`
	Source       string `json:"source"`
	Rights       string `json:"rights"`
	ExamRelation string `json:"exam_relation"`
}

// Passage is the reading text a set of questions refers to.
type Passage struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

// Question is one multiple-choice item with a verified-by-construction key.
type Question struct {
	ID              string            `json:"id"`
	Stem            string            `json:"stem"`
	Options         []Option          `json:"options"`
	Answer          string            `json:"answer"`
	Evidence        string            `json:"evidence"`
	Reasoning       string            `json:"reasoning"`
	OptionReasoning map[string]string `json:"option_reasoning"`
}

// Option is one selectable answer.
type Option struct {
	Label string `json:"label"`
	Text  string `json:"text"`
}

// Parse decodes and validates authored content. Unknown fields are rejected so
// a typo in a key cannot silently produce a different exercise.
func Parse(data []byte) (Exam, error) {
	var e Exam
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&e); err != nil {
		return Exam{}, fmt.Errorf("parse exam: %w", err)
	}
	if dec.More() {
		return Exam{}, fmt.Errorf("parse exam: trailing JSON data")
	}
	if err := e.Validate(); err != nil {
		return Exam{}, err
	}
	return e, nil
}

// Validate enforces the invariants that make feedback deterministic and
// evidence-based.
func (e Exam) Validate() error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if e.SchemaVersion != SchemaVersion {
		add("schema_version is %d, want %d", e.SchemaVersion, SchemaVersion)
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{"id", e.ID},
		{"version", e.Version},
		{"title", e.Title},
		{"notice", e.Notice},
		{"provenance.author", e.Provenance.Author},
		{"provenance.source", e.Provenance.Source},
		{"provenance.rights", e.Provenance.Rights},
		{"provenance.exam_relation", e.Provenance.ExamRelation},
		{"passage.title", e.Passage.Title},
		{"passage.text", e.Passage.Text},
	} {
		if strings.TrimSpace(field.value) == "" {
			add("%s is empty", field.name)
		}
	}
	if len(e.Questions) == 0 {
		add("no questions")
	}

	seen := make(map[string]bool, len(e.Questions))
	for i, q := range e.Questions {
		where := fmt.Sprintf("questions[%d]", i)
		if q.ID == "" {
			add("%s.id is empty", where)
		} else {
			where = fmt.Sprintf("question %s", q.ID)
			if seen[q.ID] {
				add("%s is duplicated", where)
			}
			seen[q.ID] = true
		}
		if strings.TrimSpace(q.Stem) == "" {
			add("%s stem is empty", where)
		}
		if strings.TrimSpace(q.Evidence) == "" {
			add("%s evidence is empty", where)
		} else if !strings.Contains(e.Passage.Text, q.Evidence) {
			add("%s evidence is not a verbatim passage substring", where)
		}
		if strings.TrimSpace(q.Reasoning) == "" {
			add("%s reasoning is empty", where)
		}
		for _, problem := range OptionProblems(q.Options, q.OptionReasoning) {
			add("%s %s", where, problem)
		}
		if _, ok := q.Option(q.Answer); !ok {
			add("%s answer %q is not one of its options", where, q.Answer)
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid exam: %s", strings.Join(problems, "; "))
	}
	return nil
}

// OptionProblems reports structural problems in options and their per-label
// explanations. Source questions allow an empty answer until review; the
// caller checks answer membership separately.
func OptionProblems(options []Option, reasoning map[string]string) []string {
	var problems []string
	// Authored items are 2..8 options. Two is the minimum for a choice; eight
	// keeps one multiple-choice item readable in the CLI prompt.
	if len(options) < 2 || len(options) > 8 {
		problems = append(problems, fmt.Sprintf("has %d options, want 2..8", len(options)))
	}
	labels := make(map[string]bool, len(options))
	for i, opt := range options {
		if len(opt.Label) != 1 || opt.Label[0] < 'A' || opt.Label[0] > 'Z' {
			problems = append(problems, fmt.Sprintf("options[%d].label %q is not one uppercase letter", i, opt.Label))
		}
		if labels[opt.Label] {
			problems = append(problems, fmt.Sprintf("option label %q is duplicated", opt.Label))
		}
		labels[opt.Label] = true
		if strings.TrimSpace(opt.Text) == "" {
			problems = append(problems, fmt.Sprintf("option %s text is empty", opt.Label))
		}
	}
	for label := range labels {
		if strings.TrimSpace(reasoning[label]) == "" {
			problems = append(problems, fmt.Sprintf("option %s has no explanation", label))
		}
	}
	for label := range reasoning {
		if !labels[label] {
			problems = append(problems, fmt.Sprintf("explains unknown option %q", label))
		}
	}
	return problems
}

// Question returns the question with the given stable id and its position.
func (e Exam) Question(id string) (Question, int, bool) {
	for i, q := range e.Questions {
		if q.ID == id {
			return q, i, true
		}
	}
	return Question{}, 0, false
}

// Option returns the selectable option with the given label.
func (q Question) Option(label string) (Option, bool) {
	for _, opt := range q.Options {
		if opt.Label == label {
			return opt, true
		}
	}
	return Option{}, false
}

// Correct returns the keyed option. The question is expected to be validated.
func (q Question) Correct() Option {
	opt, _ := q.Option(q.Answer)
	return opt
}

var (
	sampleOnce sync.Once
	sampleExam Exam
	sampleErr  error
)

// Sample returns the embedded, self-authored practice exercise. The returned
// value is shared and must not be mutated by callers.
func Sample() (Exam, error) {
	sampleOnce.Do(func() {
		sampleExam, sampleErr = Parse(sampleJSON)
	})
	return sampleExam, sampleErr
}
