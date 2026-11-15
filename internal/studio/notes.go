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

// NotePreview is the card rendered with the real note model: HTML is the
// rendered card, CSS the model stylesheet. The window shows it in a sandboxed
// frame.
type NotePreview struct {
	DraftID    int64      `json:"draft_id"`
	Fields     NoteFields `json:"fields"`
	Source     string     `json:"source"`
	HTML       string     `json:"html"`
	CSS        string     `json:"css"`
	HTMLSHA256 string     `json:"html_sha256"`
	Deck       string     `json:"deck"`
	Model      string     `json:"model"`
	Edited     bool       `json:"edited"`
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
}
