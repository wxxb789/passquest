// Package studio holds the data the desktop workspace exchanges with its
// window: pure, JSON-tagged values with no behaviour beyond formatting an
// error. The Go service in internal/cli fills them from the same orchestration
// the CLI runs; the desktop binds them and MyGo generates the TypeScript
// types. Nothing here may carry a credential, a model's hidden reasoning or an
// answer key before an answer is recorded.
package studio

import "fmt"

// PassThresholdUnknown is the only pass-threshold value the workspace reports:
// the real threshold is an owner input that does not exist.
const PassThresholdUnknown = "unknown"

// Practice conditions shown before an answer. They mirror how the evidence
// report and the adaptive policy will classify the recorded attempt.
const (
	ConditionIndependent = "independent"
	ConditionAssisted    = "assisted"
	ConditionReExposure  = "re_exposure"
	ConditionRevisit     = "revisit"
)

// Source label kinds.
const (
	SourceSample     = "sample"
	SourceFirstParty = "first_party"
	SourceImported   = "imported"
)

// Error is the typed failure every workspace method returns. Code reuses the
// CLI error codes (invalid_input, assessment_active, pool_floor,
// no_credential, …); Action, when set, is the learner's next step.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Action  string `json:"action,omitempty"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// SkillRef names a registered skill.
type SkillRef struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
}

// SourceLabel says where a passage comes from.
type SourceLabel struct {
	Kind     string `json:"kind"`
	SourceID string `json:"source_id,omitempty"`
	Page     int    `json:"page,omitempty"`
	Reviewed bool   `json:"reviewed"`
	Title    string `json:"title"`
	Rights   string `json:"rights"`
}

// Passage is display text split at blank lines.
type Passage struct {
	Title      string      `json:"title"`
	Label      SourceLabel `json:"label"`
	Paragraphs []string    `json:"paragraphs"`
}

// Option is one answer choice without any correctness information.
type Option struct {
	Label string `json:"label"`
	Text  string `json:"text"`
}

// GlossaryEntry is an authored gloss; showing it does not change assistance.
type GlossaryEntry struct {
	Term    string `json:"term"`
	Meaning string `json:"meaning"`
}
