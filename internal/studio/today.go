package studio

// Today is the opening view: the transparent policy's next action with its
// evidence, and the local state around it.
type Today struct {
	GeneratedAt    string          `json:"generated_at"`
	Recommendation *Recommendation `json:"recommendation"`
	Skills         []SkillEvidence `json:"skills"`
	Unclassified   int             `json:"unclassified_attempts"`
	Pool           AssessmentPool  `json:"pool"`
	ActiveRun      *AssessmentRun  `json:"active_run"`
	LastRun        *AssessmentRun  `json:"last_run"`
	Anki           AnkiCounts      `json:"anki"`
	Brief          *BriefSummary   `json:"brief"`
	Find           FindStatus      `json:"find"`
	Model          ModelStatus     `json:"model"`
	Prep           PrepStatus      `json:"prep"`
	Limitations    []string        `json:"limitations"`
}

// Recommendation is the adaptive policy's choice. Target is what the window
// opens; Command is the equivalent CLI line for transparency.
type Recommendation struct {
	Action             string   `json:"action"`
	State              string   `json:"state"`
	Reason             string   `json:"reason"`
	Skill              SkillRef `json:"skill"`
	Target             Target   `json:"target"`
	FollowUp           *Target  `json:"follow_up"`
	Command            string   `json:"command"`
	FollowUpCommand    string   `json:"follow_up_command,omitempty"`
	EvidenceAttemptIDs []int64  `json:"evidence_attempt_ids"`
}

// Target kinds.
const (
	TargetTactic = "tactic"
	TargetSkill  = "skill"
	TargetLesson = "lesson"
)

// Target is one structured destination: a tactic practice (with stage), a
// skill practice (optionally pinned to material) or a skill lesson.
type Target struct {
	Kind       string `json:"kind"`
	Skill      string `json:"skill"`
	Stage      string `json:"stage,omitempty"`
	ContentID  string `json:"content_id,omitempty"`
	QuestionID string `json:"question_id,omitempty"`
	SourceID   string `json:"source_id,omitempty"`
}

// SkillEvidence is one skill's recorded evidence and the inference drawn
// from it.
type SkillEvidence struct {
	Skill     SkillRef  `json:"skill"`
	Evidence  Evidence  `json:"evidence"`
	Inference Inference `json:"inference"`
}

// Evidence mirrors the `next` evidence document.
type Evidence struct {
	Attempts                int     `json:"attempts"`
	AssistedAttempts        int     `json:"assisted_attempts"`
	AssistedCorrect         int     `json:"assisted_correct"`
	GuidedAttempts          int     `json:"guided_attempts"`
	IndependentAttempts     int     `json:"independent_attempts"`
	IndependentCorrect      int     `json:"independent_correct"`
	IndependentIncorrect    int     `json:"independent_incorrect"`
	IndependentSkipped      int     `json:"independent_skipped"`
	Revisits                int     `json:"revisits"`
	PassageRepeats          int     `json:"passage_repeats"`
	DistinctPassages        int     `json:"distinct_passages"`
	QuestionsSeen           int     `json:"questions_seen"`
	FreshPassages           int     `json:"fresh_passages"`
	FastestIndependentMS    int64   `json:"fastest_independent_ms"`
	LatestIndependentMS     int64   `json:"latest_independent_ms"`
	LatestIndependentStatus string  `json:"latest_independent_status,omitempty"`
	LatestAttemptID         int64   `json:"latest_attempt_id,omitempty"`
	LatestStatus            string  `json:"latest_status,omitempty"`
	LatestAssistance        string  `json:"latest_assistance,omitempty"`
	AttemptIDs              []int64 `json:"attempt_ids"`
	IndependentAttemptIDs   []int64 `json:"independent_attempt_ids"`
	MappedFromCurrentTags   int     `json:"mapped_from_current_tags"`
}

// Inference is recomputed from evidence and never stored.
type Inference struct {
	State           string   `json:"state"`
	Action          string   `json:"action"`
	Reason          string   `json:"reason"`
	CausalDiagnosis string   `json:"causal_diagnosis"`
	Uncertainty     []string `json:"uncertainty"`
}

// AnkiCounts counts local notes by their Anki state.
type AnkiCounts struct {
	Total          int `json:"total"`
	Local          int `json:"local"`
	Queued         int `json:"queued"`
	Pushed         int `json:"pushed"`
	AlreadyPresent int `json:"already_present"`
	Conflict       int `json:"conflict"`
	Failed         int `json:"failed"`
	DeckMismatch   int `json:"deck_mismatch"`
	OpenDrafts     int `json:"open_drafts"`
}

// BriefSummary is the latest unfinished study brief.
type BriefSummary struct {
	RunID       string `json:"run_id"`
	Task        string `json:"task"`
	State       string `json:"state"`
	Step        int    `json:"step"`
	MaxSteps    int    `json:"max_steps"`
	Provider    string `json:"provider,omitempty"`
	Model       string `json:"model"`
	CreatedAt   string `json:"created_at"`
	LatestEvent string `json:"latest_event"`
}

// FindStatus is the bounded search adapter's configuration and today's usage.
type FindStatus struct {
	Configured   bool   `json:"configured"`
	EndpointEnv  string `json:"endpoint_env"`
	Day          string `json:"day"`
	QueriesUsed  int    `json:"queries_used"`
	QueriesLimit int    `json:"queries_limit"`
}

// ModelStatus is the active tutor model and the per-provider budget.
type ModelStatus struct {
	Configured    bool   `json:"configured"`
	Legacy        bool   `json:"legacy"`
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model,omitempty"`
	TextChatState string `json:"text_chat_state"`
	Auth          string `json:"auth"`
	Budget        Budget `json:"budget"`
	Problem       string `json:"problem,omitempty"`
}

// Budget is the per-provider daily call budget (UTC).
type Budget struct {
	PerProviderLimit int              `json:"per_provider_limit"`
	Providers        []BudgetProvider `json:"providers"`
	TotalCalls       int              `json:"total_calls"`
	TotalLimit       int              `json:"total_limit"`
	ProviderCount    int              `json:"provider_count"`
	Ceiling          string           `json:"ceiling"`
}

// BudgetProvider is one provider's usage today.
type BudgetProvider struct {
	Provider string `json:"provider"`
	Label    string `json:"label"`
	Calls    int    `json:"calls"`
	Limit    int    `json:"limit"`
	Active   bool   `json:"active"`
}

// PrepStatus carries the display-only learner settings (Q-0011-06). Empty
// values mean "not set"; the pass threshold is always unknown.
type PrepStatus struct {
	Day            int    `json:"day"`
	FirstAttemptAt string `json:"first_attempt_at,omitempty"`
	ExamDate       string `json:"exam_date"`
	DailyMinutes   int    `json:"daily_minutes"`
	PassThreshold  string `json:"pass_threshold"`
}

// LearnerSettings are optional and display-only. An empty exam date or zero
// minutes clears the value.
type LearnerSettings struct {
	ExamDate     string `json:"exam_date"`
	DailyMinutes int    `json:"daily_minutes"`
}
