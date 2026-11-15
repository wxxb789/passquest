package studio

// Coach modes.
const (
	CoachExplain     = "explain"
	CoachProbe       = "probe"
	CoachMicroLesson = "micro_lesson"
)

// CoachPreview shows everything about an advisory request before it is sent:
// who receives it, the exact messages, their hash and size, the consent state
// of the source and the budget slot.
type CoachPreview struct {
	AttemptID     int64          `json:"attempt_id"`
	Mode          string         `json:"mode"`
	Provider      string         `json:"provider"`
	Model         string         `json:"model"`
	Endpoint      string         `json:"endpoint"`
	External      bool           `json:"external"`
	Verified      bool           `json:"verified"`
	TextChatState string         `json:"text_chat_state,omitempty"`
	Action        string         `json:"action,omitempty"`
	Credential    Credential     `json:"credential"`
	Consent       CoachConsent   `json:"consent"`
	Messages      []CoachMessage `json:"messages"`
	MaxTokens     int            `json:"max_tokens"`
	RequestSHA256 string         `json:"request_sha256"`
	RequestBytes  int            `json:"request_bytes"`
	Budget        BudgetProvider `json:"budget"`
	Limits        []string       `json:"limits"`
}

// CoachMessage is one message of the exact request body.
type CoachMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CoachConsent is the payload scope for a private source.
type CoachConsent struct {
	PrivateSource    bool     `json:"private_source"`
	SourceID         string   `json:"source_id,omitempty"`
	Required         bool     `json:"required"`
	Granted          bool     `json:"granted"`
	SourceTextShared bool     `json:"source_text_shared"`
	Withheld         []string `json:"withheld"`
}

// Credential reports presence only; the value never leaves the core.
type Credential struct {
	Env     string `json:"env"`
	Present bool   `json:"present"`
}

// CoachReply is an advisory answer, or a recorded failure. Neither changes a
// grade, an attempt or an answer key.
type CoachReply struct {
	State          string         `json:"state"`
	CallID         int64          `json:"call_id"`
	AttemptID      int64          `json:"attempt_id"`
	Mode           string         `json:"mode"`
	Provider       string         `json:"provider"`
	Model          string         `json:"model"`
	Text           string         `json:"text"`
	Advisory       bool           `json:"advisory"`
	VerifiedAnswer bool           `json:"verified_answer"`
	RequestSHA256  string         `json:"request_sha256"`
	RequestBytes   int            `json:"request_bytes"`
	LatencyMS      int64          `json:"latency_ms"`
	Consent        CoachConsent   `json:"consent"`
	Failover       *CoachFailover `json:"failover"`
	Failure        *CoachFailure  `json:"failure"`
	Limits         []string       `json:"limits"`
}

// CoachFailure is a provider failure recorded as a call.
type CoachFailure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Action  string `json:"action,omitempty"`
}

// CoachFailover describes the at-most-one retry on another provider.
type CoachFailover struct {
	FromProvider string `json:"from_provider"`
	FromModel    string `json:"from_model"`
	ToProvider   string `json:"to_provider,omitempty"`
	ToModel      string `json:"to_model,omitempty"`
	Reason       string `json:"reason"`
	Applied      bool   `json:"applied"`
	Refusal      string `json:"refusal,omitempty"`
	Detail       string `json:"detail,omitempty"`
}

// Consent is the latest decision for one source and provider scope.
type Consent struct {
	SourceID   string `json:"source_id"`
	Provider   string `json:"provider,omitempty"`
	Title      string `json:"title"`
	Granted    bool   `json:"granted"`
	RecordedAt string `json:"recorded_at,omitempty"`
	Reviewer   string `json:"reviewer,omitempty"`
}
