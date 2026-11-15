package studio

// Practice request kinds.
const (
	PracticeNext   = "next"
	PracticeSkill  = "skill"
	PracticeTactic = "tactic"
	PracticeSample = "sample"
	PracticeSource = "source"
)

// PracticeRequest selects local practice the way the matching CLI command
// does: Skill for kind skill, Tactic and Stage (guided|independent, empty for
// the default) for kind tactic, SourceID/Page for kind source; SourceID also
// scopes skill and tactic material; QuestionID pins a question.
type PracticeRequest struct {
	Kind       string `json:"kind"`
	Skill      string `json:"skill,omitempty"`
	Tactic     string `json:"tactic,omitempty"`
	Stage      string `json:"stage,omitempty"`
	SourceID   string `json:"source_id,omitempty"`
	Page       int    `json:"page,omitempty"`
	QuestionID string `json:"question_id,omitempty"`
}

// PracticeItem is one displayed question. It never carries the key, the
// evidence or any reasoning. Token binds an answer to this display.
type PracticeItem struct {
	Token           string          `json:"token"`
	Kind            string          `json:"kind"`
	ContentID       string          `json:"content_id"`
	ContentVersion  string          `json:"content_version"`
	QuestionID      string          `json:"question_id"`
	Passage         Passage         `json:"passage"`
	Stem            string          `json:"stem"`
	Options         []Option        `json:"options"`
	Glossary        []GlossaryEntry `json:"glossary"`
	Index           int             `json:"index"`
	Count           int             `json:"count"`
	Condition       string          `json:"condition"`
	ConditionReason string          `json:"condition_reason"`
	CanCount        bool            `json:"can_count"`
	PriorAttempts   int             `json:"prior_attempts"`
	HintAvailable   bool            `json:"hint_available"`
	HintReason      string          `json:"hint_reason,omitempty"`
	HintOpened      bool            `json:"hint_opened"`
	Hint            *Hint           `json:"hint"`
	Guidance        *Hint           `json:"guidance"`
	PresentedAt     string          `json:"presented_at"`
	SkillID         string          `json:"skill_id,omitempty"`
	TacticID        string          `json:"tactic_id,omitempty"`
	Stage           string          `json:"stage,omitempty"`
	Skills          []string        `json:"skills"`
	Notice          string          `json:"notice"`
}

// Hint is an authored tactic walkthrough. A tactic's guided stage shows it as
// Guidance; elsewhere it is a Hint, offered only where one is authored, and
// opening it makes the attempt assisted (Q-0011-05).
type Hint struct {
	TacticID    string   `json:"tactic_id"`
	Title       string   `json:"title"`
	Principle   string   `json:"principle"`
	WhyItWorks  string   `json:"why_it_works"`
	WhenItWorks string   `json:"when_it_works"`
	Steps       []string `json:"steps"`
	Boundary    Boundary `json:"boundary"`
}

// Boundary is where a tactic stops working.
type Boundary struct {
	Folklore   string `json:"folklore"`
	Example    string `json:"example"`
	WhyItFails string `json:"why_it_fails"`
}

// Feedback is the deterministic grade of one recorded attempt.
type Feedback struct {
	Token           string              `json:"token"`
	AttemptID       int64               `json:"attempt_id"`
	ContentID       string              `json:"content_id"`
	QuestionID      string              `json:"question_id"`
	Verdict         string              `json:"verdict"`
	Selected        *Option             `json:"selected"`
	Key             Option              `json:"key"`
	Options         []OptionExplanation `json:"options"`
	Evidence        EvidenceSpan        `json:"evidence"`
	Reasoning       string              `json:"reasoning"`
	Tactic          *TacticLink         `json:"tactic"`
	Condition       string              `json:"condition"`
	Assistance      string              `json:"assistance"`
	Exposure        string              `json:"exposure"`
	Counted         bool                `json:"counted"`
	AttemptNumber   int                 `json:"attempt_number"`
	ElapsedMS       int64               `json:"elapsed_ms"`
	PresentedAt     string              `json:"presented_at"`
	AnsweredAt      string              `json:"answered_at"`
	Passage         Passage             `json:"passage"`
	Stem            string              `json:"stem"`
	AlreadyAnswered bool                `json:"already_answered"`
}

// OptionExplanation is one option with its authored explanation, or the
// missing state when the source explains none.
type OptionExplanation struct {
	Label       string `json:"label"`
	Text        string `json:"text"`
	Explanation string `json:"explanation"`
	Missing     bool   `json:"missing"`
	Correct     bool   `json:"correct"`
	Selected    bool   `json:"selected"`
}

// EvidenceSpan is the evidence sentence and where it sits in the passage:
// paragraph index and half-open rune offsets in that paragraph. Located is
// false when the sentence cannot be placed in one paragraph.
type EvidenceSpan struct {
	Text      string `json:"text"`
	Located   bool   `json:"located"`
	Paragraph int    `json:"paragraph"`
	Start     int    `json:"start"`
	End       int    `json:"end"`
}

// TacticLink points from feedback to the tactic lesson.
type TacticLink struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
}
