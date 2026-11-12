package cli

import (
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/tutor"
)

// Tutor JSON documents. Field names are protocol; golden files pin them.
// Every advisory surface names the model output as advisory and never presents
// it as a verified answer or grade.

type tutorAttemptView struct {
	ID            int64    `json:"id"`
	ContentID     string   `json:"content_id"`
	QuestionID    string   `json:"question_id"`
	Status        string   `json:"status"`
	SelectedLabel string   `json:"selected_label,omitempty"`
	CorrectLabel  string   `json:"correct_label"`
	AttemptNumber int      `json:"attempt_number"`
	Repeat        bool     `json:"repeat"`
	Assistance    string   `json:"assistance"`
	ElapsedMS     int64    `json:"elapsed_ms"`
	Skills        []string `json:"skills"`
}

func tutorAttemptViewOf(record attempt.Attempt) tutorAttemptView {
	return tutorAttemptView{
		ID:            record.ID,
		ContentID:     record.ContentID,
		QuestionID:    record.QuestionID,
		Status:        string(record.Status),
		SelectedLabel: record.SelectedLabel,
		CorrectLabel:  record.CorrectLabel,
		AttemptNumber: record.AttemptNumber,
		Repeat:        record.Repeat(),
		Assistance:    string(record.Assistance),
		ElapsedMS:     record.ElapsedMS,
		Skills:        append([]string{}, record.Skills...),
	}
}

type tutorProviderView struct {
	tutorConfigView
	Provider          string `json:"provider,omitempty"`
	Auth              string `json:"auth"`
	CredentialEnv     string `json:"credential_env"`
	CredentialPresent bool   `json:"credential_present"`
	TextChatState     string `json:"text_chat_state,omitempty"`
	TextChatAction    string `json:"text_chat_action,omitempty"`
}

func tutorProviderViewOf(cfg tutor.Config, opt Options) tutorProviderView {
	env, _, present := provider.CredentialValue(opt.Getenv, cfg.Provider)
	return tutorProviderView{
		tutorConfigView:   tutorConfigViewOf(cfg),
		Provider:          cfg.Provider,
		Auth:              authState(present),
		CredentialEnv:     env,
		CredentialPresent: present,
	}
}

type tutorContextView struct {
	PrivateSource               bool     `json:"private_source"`
	SourceID                    string   `json:"source_id,omitempty"`
	ConsentRequired             bool     `json:"consent_required"`
	ConsentGranted              bool     `json:"consent_granted"`
	SourceTextShared            bool     `json:"source_text_shared"`
	Withheld                    []string `json:"withheld"`
	ContentMatched              bool     `json:"content_matched"`
	ContentChanged              bool     `json:"content_changed"`
	ContentUnavailable          bool     `json:"content_unavailable"`
	ContentIncluded             bool     `json:"content_included"`
	StoredTextIncluded          bool     `json:"stored_text_included"`
	Skills                      []string `json:"skills"`
	SkillsMappedFromCurrentTags bool     `json:"skills_mapped_from_current_tags"`
	TruncatedFields             []string `json:"truncated_fields"`
	PromptVersion               string   `json:"prompt_version"`
}

func tutorContextViewOf(context tutor.Context) tutorContextView {
	return tutorContextView{
		PrivateSource:               context.PrivateSource,
		SourceID:                    context.SourceID,
		ConsentRequired:             context.ConsentRequired,
		ConsentGranted:              context.ConsentGranted,
		SourceTextShared:            context.SourceTextShared,
		Withheld:                    append([]string{}, context.Withheld...),
		ContentMatched:              context.ContentMatched,
		ContentChanged:              context.ContentChanged,
		ContentUnavailable:          context.ContentUnavailable,
		ContentIncluded:             context.ContentIncluded,
		StoredTextIncluded:          context.StoredTextIncluded,
		Skills:                      append([]string{}, context.Skills...),
		SkillsMappedFromCurrentTags: context.SkillsMappedFromCurrentTags,
		TruncatedFields:             append([]string{}, context.TruncatedFields...),
		PromptVersion:               context.PromptVersion,
	}
}

type tutorConfigView struct {
	Provider        string `json:"provider,omitempty"`
	Endpoint        string `json:"endpoint"`
	Model           string `json:"model"`
	TimeoutMS       int64  `json:"timeout_ms"`
	MaxOutputTokens int    `json:"max_output_tokens"`
	DailyCalls      int    `json:"daily_calls"`
}

type tutorCredentialView struct {
	Env     string `json:"env"`
	Present bool   `json:"present"`
}

type tutorConfigDoc struct {
	SchemaVersion int                 `json:"schema_version"`
	State         string              `json:"state"`
	Configured    bool                `json:"configured"`
	Config        tutorConfigView     `json:"config"`
	Credential    tutorCredentialView `json:"credential"`
	Limits        []string            `json:"limits"`
}

func tutorConfigViewOf(cfg tutor.Config) tutorConfigView {
	return tutorConfigView{
		Provider:        cfg.Provider,
		Endpoint:        cfg.Endpoint,
		Model:           cfg.Model,
		TimeoutMS:       cfg.TimeoutMS,
		MaxOutputTokens: cfg.MaxOutputTokens,
		DailyCalls:      cfg.DailyCalls,
	}
}

type tutorPreviewDoc struct {
	SchemaVersion int               `json:"schema_version"`
	State         string            `json:"state"`
	Attempt       tutorAttemptView  `json:"attempt"`
	Mode          string            `json:"mode"`
	Provider      tutorProviderView `json:"provider"`
	Context       tutorContextView  `json:"context"`
	Chat          tutor.Chat        `json:"chat"`
	RequestSHA256 string            `json:"request_sha256"`
	RequestBytes  int               `json:"request_bytes"`
	Limits        []string          `json:"limits"`
}

type tutorSuggestionView struct {
	Text           string `json:"text"`
	Advisory       bool   `json:"advisory"`
	VerifiedAnswer bool   `json:"verified_answer"`
}

type tutorCallView struct {
	ID               int64  `json:"id"`
	CreatedAt        string `json:"created_at"`
	AttemptID        int64  `json:"attempt_id"`
	ContentID        string `json:"content_id"`
	QuestionID       string `json:"question_id"`
	SourceID         string `json:"source_id,omitempty"`
	Mode             string `json:"mode"`
	Status           string `json:"status"`
	ErrorCode        string `json:"error_code,omitempty"`
	ErrorMessage     string `json:"error_message,omitempty"`
	Provider         string `json:"provider,omitempty"`
	ProviderHost     string `json:"provider_host"`
	FailoverFrom     string `json:"failover_from,omitempty"`
	Model            string `json:"model"`
	PromptVersion    string `json:"prompt_version"`
	SourceTextShared bool   `json:"source_text_shared"`
	RequestSHA256    string `json:"request_sha256"`
	RequestBytes     int    `json:"request_bytes"`
	OutputChars      int    `json:"output_chars"`
	PromptTokens     *int   `json:"prompt_tokens,omitempty"`
	CompletionTokens *int   `json:"completion_tokens,omitempty"`
	HTTPStatus       int    `json:"http_status"`
	LatencyMS        int64  `json:"latency_ms"`
	OutputText       string `json:"output_text,omitempty"`
}

func tutorCallViewOf(call store.TutorCall, includeText bool) tutorCallView {
	view := tutorCallView{
		ID:               call.ID,
		CreatedAt:        call.CreatedAt.UTC().Format(time.RFC3339Nano),
		AttemptID:        call.AttemptID,
		ContentID:        call.ContentID,
		QuestionID:       call.QuestionID,
		SourceID:         call.SourceID,
		Mode:             call.Mode,
		Status:           call.Status,
		ErrorCode:        call.ErrorCode,
		ErrorMessage:     call.ErrorMessage,
		Provider:         call.Provider,
		ProviderHost:     call.ProviderHost,
		FailoverFrom:     call.FailoverFrom,
		Model:            call.Model,
		PromptVersion:    call.PromptVersion,
		SourceTextShared: call.SourceTextShared,
		RequestSHA256:    call.RequestSHA256,
		RequestBytes:     call.RequestBytes,
		OutputChars:      call.OutputChars,
		PromptTokens:     call.PromptTokens,
		CompletionTokens: call.CompletionTokens,
		HTTPStatus:       call.HTTPStatus,
		LatencyMS:        call.LatencyMS,
	}
	if includeText {
		view.OutputText = call.OutputText
	}
	return view
}

type tutorFailoverView struct {
	FromProvider string `json:"from_provider"`
	FromModel    string `json:"from_model"`
	ToProvider   string `json:"to_provider,omitempty"`
	ToModel      string `json:"to_model,omitempty"`
	Reason       string `json:"reason"`
	Applied      bool   `json:"applied"`
	Refusal      string `json:"refusal,omitempty"`
	Detail       string `json:"detail,omitempty"`
}

type tutorAdvisoryDoc struct {
	SchemaVersion int                 `json:"schema_version"`
	State         string              `json:"state"`
	Call          tutorCallView       `json:"call"`
	Suggestion    tutorSuggestionView `json:"suggestion"`
	Context       tutorContextView    `json:"context"`
	Failover      *tutorFailoverView  `json:"failover,omitempty"`
	Limits        []string            `json:"limits"`
}

type tutorFailureDoc struct {
	SchemaVersion int                `json:"schema_version"`
	State         string             `json:"state"`
	Error         errorBody          `json:"error"`
	Action        string             `json:"action,omitempty"`
	Call          tutorCallView      `json:"call"`
	Failover      *tutorFailoverView `json:"failover,omitempty"`
	Limits        []string           `json:"limits"`
}

type tutorShowDoc struct {
	SchemaVersion int                  `json:"schema_version"`
	State         string               `json:"state"`
	Call          tutorCallView        `json:"call"`
	Suggestion    *tutorSuggestionView `json:"suggestion,omitempty"`
	Limits        []string             `json:"limits"`
}

type tutorHistorySummaryView struct {
	Calls            int                       `json:"calls"`
	OK               int                       `json:"ok"`
	Failed           int                       `json:"failed"`
	PromptTokens     int                       `json:"prompt_tokens"`
	CompletionTokens int                       `json:"completion_tokens"`
	DailyCalls       int                       `json:"daily_calls"`
	DailyLimit       *int                      `json:"daily_limit,omitempty"`
	Providers        []modelBudgetProviderView `json:"providers"`
	TotalCalls       int                       `json:"total_calls"`
	TotalLimit       int                       `json:"total_limit"`
	Ceiling          string                    `json:"ceiling"`
}

type tutorHistoryDoc struct {
	SchemaVersion int                     `json:"schema_version"`
	State         string                  `json:"state"`
	Summary       tutorHistorySummaryView `json:"summary"`
	Calls         []tutorCallView         `json:"calls"`
	Limits        []string                `json:"limits"`
}

type tutorConsentDoc struct {
	SchemaVersion int      `json:"schema_version"`
	State         string   `json:"state"`
	SourceID      string   `json:"source_id"`
	Provider      string   `json:"provider,omitempty"`
	Granted       bool     `json:"granted"`
	RecordedAt    string   `json:"recorded_at"`
	Reviewer      string   `json:"reviewer"`
	Limits        []string `json:"limits"`
}

type tutorConsentSourceView struct {
	SourceID   string `json:"source_id"`
	Provider   string `json:"provider,omitempty"`
	Title      string `json:"title"`
	Granted    bool   `json:"granted"`
	RecordedAt string `json:"recorded_at,omitempty"`
	Reviewer   string `json:"reviewer,omitempty"`
}

type tutorConsentsDoc struct {
	SchemaVersion int                      `json:"schema_version"`
	State         string                   `json:"state"`
	Sources       []tutorConsentSourceView `json:"sources"`
	Limits        []string                 `json:"limits"`
}

var tutorLimits = []string{
	"Model output is advisory study guidance; it is not a verified answer, grade, exam fact or readiness claim.",
	"One recorded attempt is sent per call. No attempt history, database path, source title, endpoint path or credential is sent.",
	"Raw prompt text is not stored: a call records the SHA-256 and byte size of the exact serialized request body plus scope flags. Run `passquest tutor preview` to inspect the exact text before sending.",
	"Private imported source text is withheld until a recorded `tutor consent` event for that source; first-party material needs no consent.",
	"A failed or malformed provider response is recorded as a failure and cannot append an attempt, change a grade or edit an answer key.",
	"The daily call budget counts failed calls and resets at 00:00 UTC. `tutor ask` retries at most once, only on another verified provider with the same recorded consent scope and remaining budget.",
}
