package studio

// NoteRequest proposes a note from a recorded attempt, or from a passage mark
// on a reviewed question. Mark is a passage selection (at most 120 runes after
// normalisation).
type NoteRequest struct {
	AttemptID  int64  `json:"attempt_id,omitempty"`
	ContentID  string `json:"content_id,omitempty"`
	QuestionID string `json:"question_id,omitempty"`
	Mark       string `json:"mark,omitempty"`
}

// NoteFields are the learner-editable plain fields of a card.
type NoteFields struct {
	Front  string `json:"front"`
	Back   string `json:"back"`
	Tactic string `json:"tactic"`
}

// NoteTarget is what a note is about.
type NoteTarget struct {
	ContentID  string `json:"content_id"`
	QuestionID string `json:"question_id"`
	SourceID   string `json:"source_id,omitempty"`
	Page       int    `json:"page,omitempty"`
	AttemptID  int64  `json:"attempt_id,omitempty"`
	Mark       string `json:"mark,omitempty"`
}

// NoteDraft is a proposed note awaiting accept or skip.
type NoteDraft struct {
	ID             int64      `json:"id"`
	Target         NoteTarget `json:"target"`
	Origin         string     `json:"origin"`
	Restricted     bool       `json:"restricted"`
	ConsentGranted bool       `json:"consent_granted"`
	Withheld       []string   `json:"withheld"`
	Unanswered     bool       `json:"unanswered"`
	TacticID       string     `json:"tactic_id,omitempty"`
	Fields         NoteFields `json:"fields"`
	Source         string     `json:"source"`
	Advisory       bool       `json:"advisory"`
	Limits         []string   `json:"limits"`
}

// NotePreview is the card rendered with the real note model. Question and
// Answer are the complete documents Anki would show, light and night: the
// same templates, the same stylesheet and the same body classes, so the
// window only places them in a sandboxed frame. HTML and CSS are the answer
// side's body and the model stylesheet on their own.
type NotePreview struct {
	DraftID  int64        `json:"draft_id"`
	Fields   NoteFields   `json:"fields"`
	Source   string       `json:"source"`
	Question NoteCardSide `json:"question"`
	Answer   NoteCardSide `json:"answer"`
	HTML     string       `json:"html"`
	CSS      string       `json:"css"`
	// FieldsSHA256 identifies the exact field payload a push would send.
	FieldsSHA256 string `json:"fields_sha256"`
	// HTMLSHA256 is FieldsSHA256 under its earlier name, kept until the window
	// reads fields_sha256; it never was a hash of the rendered HTML.
	HTMLSHA256 string `json:"html_sha256"`
	// CSSSHA256 identifies the model stylesheet, ModelVersion its design.
	CSSSHA256    string `json:"css_sha256"`
	ModelVersion int    `json:"model_version"`
	Deck         string `json:"deck"`
	Model        string `json:"model"`
	Edited       bool   `json:"edited"`
}

// NoteCardSide is one side of the card as complete documents for an
// <iframe srcdoc>: Light without and Night with Anki's night-mode classes.
type NoteCardSide struct {
	Light string `json:"light"`
	Night string `json:"night"`
}

// NoteRefinePreview shows what an AI refinement of a draft would send before
// anything leaves the machine: who receives it, whether the endpoint is
// external, the verification and credential state, the consent covering this
// provider, the exact messages with their hash and size, and the budget slot.
// Messages are empty while consent withholds the source text.
type NoteRefinePreview struct {
	DraftID       int64          `json:"draft_id"`
	Provider      string         `json:"provider"`
	Model         string         `json:"model"`
	Endpoint      string         `json:"endpoint"`
	External      bool           `json:"external"`
	Verified      bool           `json:"verified"`
	TextChatState string         `json:"text_chat_state,omitempty"`
	Action        string         `json:"action,omitempty"`
	Credential    Credential     `json:"credential"`
	Consent       CoachConsent   `json:"consent"`
	Unanswered    bool           `json:"unanswered"`
	Messages      []CoachMessage `json:"messages"`
	MaxTokens     int            `json:"max_tokens"`
	RequestSHA256 string         `json:"request_sha256"`
	RequestBytes  int            `json:"request_bytes"`
	Budget        BudgetProvider `json:"budget"`
	Limits        []string       `json:"limits"`
}

// NoteRefinement is an advisory suggestion for a draft's front and back. It
// is never applied by itself: the learner copies what they keep into the
// fields and accepts them with AcceptNote. Tactic is the draft's own.
type NoteRefinement struct {
	State         string     `json:"state"`
	DraftID       int64      `json:"draft_id"`
	RefinementID  int64      `json:"refinement_id"`
	Suggestion    NoteFields `json:"suggestion"`
	Provider      string     `json:"provider"`
	Model         string     `json:"model"`
	Advisory      bool       `json:"advisory"`
	RequestSHA256 string     `json:"request_sha256"`
	RequestBytes  int        `json:"request_bytes"`
	LatencyMS     int64      `json:"latency_ms"`
	Limits        []string   `json:"limits"`
}

// Note is one accepted local note with its Anki state (local, queued,
// pushed, already_present, conflict, failed, deck_mismatch).
type Note struct {
	NoteID         string     `json:"note_id"`
	Revision       int        `json:"revision"`
	Target         NoteTarget `json:"target"`
	Fields         NoteFields `json:"fields"`
	Source         string     `json:"source"`
	Restricted     bool       `json:"restricted"`
	Origin         string     `json:"origin"`
	AnkiState      string     `json:"anki_state"`
	AnkiNoteID     int64      `json:"anki_note_id"`
	PushedRevision int        `json:"pushed_revision"`
	Deck           string     `json:"deck,omitempty"`
	DeckExpected   string     `json:"deck_expected,omitempty"`
	CreatedAt      string     `json:"created_at"`
	UpdatedAt      string     `json:"updated_at"`
}

// NoteAccept is the result of accepting a draft.
type NoteAccept struct {
	State            string `json:"state"`
	Note             Note   `json:"note"`
	Created          bool   `json:"created"`
	Changed          bool   `json:"changed"`
	PreviousRevision int    `json:"previous_revision"`
}

// NoteList is every local note plus the open drafts count.
type NoteList struct {
	Notes  []Note     `json:"notes"`
	Counts AnkiCounts `json:"counts"`
	Limits []string   `json:"limits"`
}

// NotePushCounts summarises one push.
type NotePushCounts struct {
	Pending        int `json:"pending"`
	Pushed         int `json:"pushed"`
	AlreadyPresent int `json:"already_present"`
	Conflicts      int `json:"conflicts"`
	Failed         int `json:"failed"`
	Queued         int `json:"queued"`
	DeckMismatch   int `json:"deck_mismatch,omitempty"`
}

// NotePushItem is one note's push outcome.
type NotePushItem struct {
	NoteID     string `json:"note_id"`
	Revision   int    `json:"revision"`
	AnkiNoteID int64  `json:"anki_note_id,omitempty"`
	State      string `json:"state"`
	Outcome    string `json:"outcome"`
	Code       string `json:"code,omitempty"`
	Detail     string `json:"detail,omitempty"`
	Deck       string `json:"deck,omitempty"`
}

// NotePush is the result of pushing the outbox. An unreachable AnkiConnect
// leaves the notes queued (state "queued", outcome "unreachable"); Error is set
// when Anki refused or a conflict, deck mismatch or failure remains.
type NotePush struct {
	State    string         `json:"state"`
	Endpoint string         `json:"endpoint"`
	Deck     string         `json:"deck"`
	Model    string         `json:"model"`
	Counts   NotePushCounts `json:"counts"`
	Notes    []NotePushItem `json:"notes"`
	Error    *Error         `json:"error"`
	Limits   []string       `json:"limits"`
	// ModelUpdated: an earlier Pass Quest card design in Anki was replaced in
	// place. ModelCustomized: the note type was changed in Anki and left as
	// it is; the notes were still pushed.
	ModelUpdated    bool `json:"model_updated"`
	ModelCustomized bool `json:"model_customized"`
}
