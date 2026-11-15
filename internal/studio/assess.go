package studio

// AssessmentPool is the unseen-assessment pool.
type AssessmentPool struct {
	QualifyingItems   int        `json:"qualifying_items"`
	QualifyingUnits   int        `json:"qualifying_units"`
	PracticeableItems int        `json:"practiceable_items"`
	PracticeableUnits int        `json:"practiceable_units"`
	SpentUnits        int        `json:"spent_units"`
	FloorItems        int        `json:"floor_items"`
	FloorMet          bool       `json:"floor_met"`
	ShortfallItems    int        `json:"shortfall_items"`
	DefaultSize       int        `json:"default_size"`
	DefaultMinutes    int        `json:"default_minutes"`
	SourceCount       int        `json:"source_count"`
	ContentUnits      []PoolUnit `json:"content_units"`
	Limitations       []string   `json:"limitations"`
}

// PoolUnit is one imported content unit (a source page).
type PoolUnit struct {
	ContentID     string   `json:"content_id"`
	SourceID      string   `json:"source_id"`
	Page          int      `json:"page"`
	QuestionCount int      `json:"question_count"`
	Spent         bool     `json:"spent"`
	SpentBy       []string `json:"spent_by,omitempty"`
	FirstSpentAt  string   `json:"first_spent_at,omitempty"`
}

// AssessmentRun is one run's stored settings and state. Times are absolute
// RFC 3339 instants so the window clock is display-only.
type AssessmentRun struct {
	ID               int64  `json:"id"`
	Status           string `json:"status"`
	StartedAt        string `json:"started_at"`
	DeadlineAt       string `json:"deadline_at"`
	WindowMinutes    int    `json:"window_minutes"`
	RequestedSize    int    `json:"requested_size"`
	ActualSize       int    `json:"actual_size"`
	PerItemTimeoutMS int64  `json:"per_item_timeout_ms"`
	Annotations      string `json:"annotations"`
	PoolFamily       string `json:"pool_family"`
	SourceCount      int    `json:"source_count"`
	ResumeCount      int    `json:"resume_count"`
	LastResumedAt    string `json:"last_resumed_at,omitempty"`
	CompletedAt      string `json:"completed_at,omitempty"`
	AbandonedAt      string `json:"abandoned_at,omitempty"`
	AbandonedReason  string `json:"abandoned_reason,omitempty"`
}

// NavigatorItem is one position's progress. It never shows correctness.
type NavigatorItem struct {
	Position int    `json:"position"`
	State    string `json:"state"`
	Current  bool   `json:"current"`
}

// Assessment step states.
const (
	AssessmentItemState      = "item"
	AssessmentCompletedState = "completed"
	AssessmentAbandonedState = "abandoned"
	AssessmentNoneState      = "none"
)

// AssessmentState is the active or requested run with its navigator.
type AssessmentState struct {
	State     string          `json:"state"`
	Now       string          `json:"now"`
	Run       *AssessmentRun  `json:"run"`
	Navigator []NavigatorItem `json:"navigator"`
	Item      *AssessmentItem `json:"item"`
	Notices   []string        `json:"notices"`
	Message   string          `json:"message,omitempty"`
}

// AssessmentItem is the presented question: no glossary, evidence,
// reasoning or feedback.
type AssessmentItem struct {
	RunID         int64    `json:"run_id"`
	Position      int      `json:"position"`
	Count         int      `json:"count"`
	ContentID     string   `json:"content_id"`
	QuestionID    string   `json:"question_id"`
	Passage       Passage  `json:"passage"`
	Stem          string   `json:"stem"`
	Options       []Option `json:"options"`
	PresentedAt   string   `json:"presented_at"`
	DeadlineAt    string   `json:"deadline_at"`
	RunDeadlineAt string   `json:"run_deadline_at"`
}

// AssessmentAnswer is the outcome of one submitted answer: answered or
// timed_out, never correct or incorrect.
type AssessmentAnswer struct {
	Position int             `json:"position"`
	Outcome  string          `json:"outcome"`
	State    AssessmentState `json:"state"`
}

// AssessmentReport mirrors `assess report --json`, plus the pass threshold.
type AssessmentReport struct {
	PassThreshold string                  `json:"pass_threshold"`
	Run           AssessmentRun           `json:"run"`
	Counts        AssessmentCounts        `json:"counts"`
	Accuracy      AssessmentAccuracy      `json:"accuracy"`
	Speed         AssessmentSpeed         `json:"speed"`
	Skills        []AssessmentSkill       `json:"skills"`
	Contamination AssessmentContamination `json:"contamination"`
	Items         []AssessmentReportItem  `json:"items"`
	Unknowns      []string                `json:"unknowns"`
	Limitations   []string                `json:"limitations"`
}

// AssessmentCounts are the run's item counts.
type AssessmentCounts struct {
	Selected            int `json:"selected"`
	Presented           int `json:"presented"`
	Finalized           int `json:"finalized"`
	Scored              int `json:"scored"`
	Correct             int `json:"correct"`
	Total               int `json:"total"`
	Contaminated        int `json:"contaminated"`
	TimedOut            int `json:"timed_out"`
	Skipped             int `json:"skipped"`
	Unavailable         int `json:"unavailable"`
	Pending             int `json:"pending"`
	AnsweredAfterResume int `json:"answered_after_resume"`
}

// AssessmentAccuracy is the headline over scored items.
type AssessmentAccuracy struct {
	Correct int     `json:"correct"`
	Total   int     `json:"total"`
	Ratio   float64 `json:"ratio"`
	Band    string  `json:"band"`
}

// AssessmentSpeed summarises elapsed time.
type AssessmentSpeed struct {
	MedianMS         int64  `json:"median_ms"`
	P90MS            int64  `json:"p90_ms"`
	PerItemTimeoutMS int64  `json:"per_item_timeout_ms"`
	Basis            int    `json:"basis_items"`
	Band             string `json:"band"`
}

// AssessmentSkill is per-skill accuracy over scored items.
type AssessmentSkill struct {
	SkillID string  `json:"skill_id"`
	Correct int     `json:"correct"`
	Total   int     `json:"total"`
	Ratio   float64 `json:"ratio"`
}

// AssessmentContamination lists excluded items.
type AssessmentContamination struct {
	PriorExposure      int                    `json:"prior_exposure"`
	RepeatedVariant    int                    `json:"repeated_variant"`
	HintsDuringRun     int                    `json:"hints_during_run"`
	ResumedAfterWindow int                    `json:"resumed_after_window"`
	ExcludedItems      []AssessmentReportItem `json:"excluded_items"`
}

// AssessmentReportItem is one item with the annotations withheld during the
// run.
type AssessmentReportItem struct {
	Position    int                   `json:"position"`
	ContentID   string                `json:"content_id"`
	QuestionID  string                `json:"question_id"`
	State       string                `json:"state"`
	Status      string                `json:"status,omitempty"`
	TimedOut    bool                  `json:"timed_out"`
	ElapsedMS   int64                 `json:"elapsed_ms"`
	AfterResume bool                  `json:"after_resume"`
	Scored      bool                  `json:"scored"`
	Reasons     []string              `json:"contamination_reasons,omitempty"`
	Annotations AssessmentAnnotations `json:"annotations"`
}

// AssessmentAnnotations are the per-item explanations shown after the run.
type AssessmentAnnotations struct {
	Withheld               bool              `json:"withheld,omitempty"`
	Available              bool              `json:"available"`
	Evidence               string            `json:"evidence,omitempty"`
	Reasoning              string            `json:"reasoning,omitempty"`
	OptionReasoning        map[string]string `json:"option_reasoning,omitempty"`
	OptionReasoningMissing []string          `json:"option_reasoning_missing,omitempty"`
	Glossary               []GlossaryEntry   `json:"glossary,omitempty"`
}

// AssessmentComparison mirrors `assess compare --json`.
type AssessmentComparison struct {
	PassThreshold  string                 `json:"pass_threshold"`
	Runs           []AssessmentCompareRun `json:"runs"`
	AccuracyPoints float64                `json:"accuracy_change_points"`
	MedianChangeMS int64                  `json:"median_change_ms"`
	P90ChangeMS    int64                  `json:"p90_change_ms"`
	TimedOutChange int                    `json:"timed_out_change"`
	ScoredChange   int                    `json:"scored_item_change"`
	Dimensions     map[string]string      `json:"dimensions"`
	Limitations    []string               `json:"limitations"`
}

// AssessmentCompareRun is one side of a comparison.
type AssessmentCompareRun struct {
	Run      AssessmentRun      `json:"run"`
	Counts   AssessmentCounts   `json:"counts"`
	Accuracy AssessmentAccuracy `json:"accuracy"`
	Speed    AssessmentSpeed    `json:"speed"`
	Skills   []AssessmentSkill  `json:"skills"`
}
