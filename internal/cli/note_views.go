package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
)

var noteLimits = []string{
	"A note is a study aid derived from recorded evidence, not a verified answer key or grade.",
	"Anki owns the card and its review scheduling after import or push.",
	"AnkiWeb and mobile synchronization are your own manual actions in Anki; Pass Quest never contacts AnkiWeb or a phone.",
	"A real Anki Desktop write is unverified on this machine; the adapter is proven against a loopback AnkiConnect stub.",
}

const noteSyncStatement = "AnkiWeb and mobile synchronization are your own manual actions in Anki. Pass Quest never contacts AnkiWeb or a phone."

type noteTargetView struct {
	ContentID  string `json:"content_id"`
	QuestionID string `json:"question_id"`
	SourceID   string `json:"source_id,omitempty"`
	Page       int    `json:"page,omitempty"`
	AttemptID  int64  `json:"attempt_id,omitempty"`
	Mark       string `json:"mark,omitempty"`
}

type noteDraftView struct {
	ID             int64          `json:"id"`
	Target         noteTargetView `json:"target"`
	Origin         string         `json:"origin"`
	Model          string         `json:"model,omitempty"`
	Restricted     bool           `json:"restricted"`
	ConsentGranted bool           `json:"consent_granted"`
	Withheld       []string       `json:"withheld"`
	Unanswered     bool           `json:"unanswered"`
	TacticID       string         `json:"tactic_id,omitempty"`
}

type noteFieldsView struct {
	Front  string `json:"Front"`
	Back   string `json:"Back"`
	Tactic string `json:"Tactic"`
	Source string `json:"Source"`
}

type notePlainView struct {
	Front  string `json:"front"`
	Back   string `json:"back"`
	Tactic string `json:"tactic"`
	Source string `json:"source"`
}

type noteRefinementView struct {
	ID               int64  `json:"id"`
	Model            string `json:"model"`
	RequestSHA256    string `json:"request_sha256"`
	RequestBytes     int    `json:"request_bytes"`
	OutputChars      int    `json:"output_chars"`
	PromptTokens     *int   `json:"prompt_tokens,omitempty"`
	CompletionTokens *int   `json:"completion_tokens,omitempty"`
	LatencyMS        int64  `json:"latency_ms"`
	Advisory         bool   `json:"advisory"`
}

type noteDraftDoc struct {
	SchemaVersion int                 `json:"schema_version"`
	State         string              `json:"state"`
	Draft         noteDraftView       `json:"draft"`
	Fields        noteFieldsView      `json:"fields"`
	Plain         notePlainView       `json:"plain"`
	Advisory      bool                `json:"advisory"`
	Refinement    *noteRefinementView `json:"refinement,omitempty"`
	Limits        []string            `json:"limits"`
}

type notePreviewDoc struct {
	SchemaVersion int            `json:"schema_version"`
	State         string         `json:"state"`
	Draft         noteDraftView  `json:"draft"`
	Fields        noteFieldsView `json:"fields"`
	Plain         notePlainView  `json:"plain"`
	HTMLSHA256    string         `json:"html_sha256"`
	Deck          string         `json:"deck"`
	Model         string         `json:"model"`
	Advisory      bool           `json:"advisory"`
	Limits        []string       `json:"limits"`
}

type noteAnkiView struct {
	State          string `json:"state"`
	NoteID         int64  `json:"note_id"`
	PushedRevision int    `json:"pushed_revision"`
	// Deck is what Anki reported for the note's card at the last completed deck
	// read; DeckExpected is set only when that is not the deck this build names.
	Deck         string `json:"deck,omitempty"`
	DeckExpected string `json:"deck_expected,omitempty"`
}

type noteView struct {
	NoteID     string         `json:"note_id"`
	Revision   int            `json:"revision"`
	Target     noteTargetView `json:"target"`
	Front      string         `json:"front"`
	Back       string         `json:"back"`
	Tactic     string         `json:"tactic"`
	Source     string         `json:"source"`
	Fields     noteFieldsView `json:"fields"`
	Restricted bool           `json:"restricted"`
	Origin     string         `json:"origin"`
	Model      string         `json:"model,omitempty"`
	Anki       noteAnkiView   `json:"anki"`
	CreatedAt  string         `json:"created_at"`
	UpdatedAt  string         `json:"updated_at"`
}

type noteRevisionView struct {
	Revision    int    `json:"revision"`
	Front       string `json:"front"`
	Back        string `json:"back"`
	Tactic      string `json:"tactic"`
	Source      string `json:"source"`
	ContentHash string `json:"content_hash"`
	Origin      string `json:"origin"`
	Model       string `json:"model,omitempty"`
	Restricted  bool   `json:"restricted"`
	AcceptedAt  string `json:"accepted_at"`
}

type noteAcceptDoc struct {
	SchemaVersion    int            `json:"schema_version"`
	State            string         `json:"state"`
	Note             noteView       `json:"note"`
	Fields           noteFieldsView `json:"fields"`
	Plain            notePlainView  `json:"plain"`
	Created          bool           `json:"created"`
	Changed          bool           `json:"changed"`
	PreviousRevision int            `json:"previous_revision"`
	Limits           []string       `json:"limits"`
}

type noteSkipDoc struct {
	SchemaVersion int      `json:"schema_version"`
	State         string   `json:"state"`
	DraftID       int64    `json:"draft_id"`
	ContentID     string   `json:"content_id"`
	QuestionID    string   `json:"question_id"`
	Mark          string   `json:"mark,omitempty"`
	Limits        []string `json:"limits"`
}

type noteShowItem struct {
	Note      noteView           `json:"note"`
	Revisions []noteRevisionView `json:"revisions"`
}

type noteShowDoc struct {
	SchemaVersion int            `json:"schema_version"`
	State         string         `json:"state"`
	Notes         []noteShowItem `json:"notes"`
	Limits        []string       `json:"limits"`
}

type noteModelDoc struct {
	SchemaVersion int            `json:"schema_version"`
	State         string         `json:"state"`
	Deck          string         `json:"deck"`
	Model         string         `json:"model"`
	Fields        []string       `json:"fields"`
	CSS           string         `json:"css"`
	FrontTemplate string         `json:"front_template"`
	BackTemplate  string         `json:"back_template"`
	SampleFields  noteFieldsView `json:"sample_fields"`
	Rendered      string         `json:"rendered_sample"`
	Limits        []string       `json:"limits"`
}

type noteExportDoc struct {
	SchemaVersion int      `json:"schema_version"`
	State         string   `json:"state"`
	Path          string   `json:"path"`
	Notes         int      `json:"notes"`
	Bytes         int      `json:"bytes"`
	SHA256        string   `json:"sha256"`
	Header        string   `json:"header"`
	Deck          string   `json:"deck"`
	Model         string   `json:"model"`
	Limits        []string `json:"limits"`
}

type notePushCounts struct {
	Pending        int `json:"pending"`
	Pushed         int `json:"pushed"`
	AlreadyPresent int `json:"already_present"`
	Conflicts      int `json:"conflicts"`
	Failed         int `json:"failed"`
	Queued         int `json:"queued"`
	// DeckMismatch counts notes Anki accepted into a deck other than the one
	// this push named. It is omitted when zero so a conforming push is
	// unchanged.
	DeckMismatch int `json:"deck_mismatch,omitempty"`
}

type notePushItemView struct {
	NoteID     string `json:"note_id"`
	Revision   int    `json:"revision"`
	AnkiNoteID int64  `json:"anki_note_id,omitempty"`
	State      string `json:"state"`
	Outcome    string `json:"outcome"`
	Code       string `json:"code,omitempty"`
	Detail     string `json:"detail,omitempty"`
	// Deck names where Anki put the card when that is not the deck this push
	// named. A matching card carries no deck field, so a conforming push
	// document is unchanged.
	Deck string `json:"deck,omitempty"`
}

type notePushDoc struct {
	SchemaVersion int                `json:"schema_version"`
	State         string             `json:"state"`
	Endpoint      string             `json:"endpoint"`
	Deck          string             `json:"deck"`
	Model         string             `json:"model"`
	ModelCreated  bool               `json:"model_created"`
	Counts        notePushCounts     `json:"counts"`
	Notes         []notePushItemView `json:"notes"`
	Error         *errorBody         `json:"error,omitempty"`
	Limits        []string           `json:"limits"`
}

func noteTargetViewOf(contentID, questionID, sourceID string, page int, attemptID int64, mark string) noteTargetView {
	return noteTargetView{
		ContentID: contentID, QuestionID: questionID, SourceID: sourceID,
		Page: page, AttemptID: attemptID, Mark: mark,
	}
}

func noteDraftViewOf(e noteEffective) noteDraftView {
	withheld := append([]string{}, e.Built.Withheld...)
	return noteDraftView{
		ID: e.Draft.ID,
		Target: noteTargetViewOf(e.Draft.ContentID, e.Draft.QuestionID, e.Draft.SourceID,
			e.Draft.Page, e.Draft.AttemptID, e.Draft.Mark),
		Origin:         string(e.Origin),
		Model:          e.Model,
		Restricted:     e.Built.Restricted,
		ConsentGranted: e.Built.ConsentGranted,
		Withheld:       withheld,
		Unanswered:     e.Built.Unanswered,
		TacticID:       e.Built.TacticID,
	}
}

func noteFieldsViewOf(f note.Fields) noteFieldsView {
	return noteFieldsView{Front: f.Front, Back: f.Back, Tactic: f.Tactic, Source: f.Source}
}

func notePlainViewOf(e noteEffective) notePlainView {
	return notePlainView{Front: e.Front, Back: e.Back, Tactic: e.Tactic, Source: e.Built.SourceLabel}
}

func noteViewOf(n store.Note) noteView {
	fields := note.HTML(n.Front, n.Back, n.Tactic, n.SourceLabel)
	anki := noteAnkiView{
		State:          n.AnkiState,
		NoteID:         n.AnkiNoteID,
		PushedRevision: n.AnkiPushedRevision,
		Deck:           n.AnkiDeck,
	}
	if n.AnkiDeck != "" && n.AnkiDeck != note.Deck {
		anki.DeckExpected = note.Deck
	}
	return noteView{
		NoteID:     n.NoteID,
		Revision:   n.Revision,
		Target:     noteTargetViewOf(n.ContentID, n.QuestionID, n.SourceID, n.Page, n.AttemptID, n.Mark),
		Front:      n.Front,
		Back:       n.Back,
		Tactic:     n.Tactic,
		Source:     n.SourceLabel,
		Fields:     noteFieldsViewOf(fields),
		Restricted: n.Restricted,
		Origin:     n.Origin,
		Model:      n.Model,
		Anki:       anki,
		CreatedAt:  source.FormatTime(n.CreatedAt),
		UpdatedAt:  source.FormatTime(n.UpdatedAt),
	}
}

func noteRevisionViewsOf(revisions []store.NoteRevision) []noteRevisionView {
	out := make([]noteRevisionView, 0, len(revisions))
	for _, revision := range revisions {
		out = append(out, noteRevisionView{
			Revision: revision.Revision, Front: revision.Front, Back: revision.Back,
			Tactic: revision.Tactic, Source: revision.SourceLabel, ContentHash: revision.ContentHash,
			Origin: revision.Origin, Model: revision.Model, Restricted: revision.Restricted,
			AcceptedAt: source.FormatTime(revision.AcceptedAt),
		})
	}
	return out
}

func noteRefinementViewOf(r store.NoteRefinement) noteRefinementView {
	return noteRefinementView{
		ID: r.ID, Model: r.Model, RequestSHA256: r.RequestSHA256, RequestBytes: r.RequestBytes,
		OutputChars:  len([]rune(r.OutputFront)) + len([]rune(r.OutputBack)),
		PromptTokens: r.PromptTokens, CompletionTokens: r.CompletionTokens,
		LatencyMS: r.LatencyMS, Advisory: true,
	}
}

func noteTargetText(target noteTargetView) string {
	parts := []string{target.ContentID, "question " + target.QuestionID}
	if target.SourceID != "" {
		parts = append(parts, fmt.Sprintf("page %d", target.Page))
	}
	if target.Mark != "" {
		parts = append(parts, fmt.Sprintf("mark %q", target.Mark))
	}
	if target.AttemptID > 0 {
		parts = append(parts, fmt.Sprintf("attempt #%d", target.AttemptID))
	}
	return strings.Join(parts, ", ")
}

func writeNotePrivacy(w io.Writer, draft noteDraftView) {
	switch {
	case draft.Restricted && draft.ConsentGranted:
		fmt.Fprintf(w, "Privacy: imported source %s is authorized; its text may be written to a local note and to Anki when you push.\n", draft.Target.SourceID)
	case draft.Restricted:
		fmt.Fprintf(w, "Privacy: imported source %s is NOT authorized; withheld: %s.\n",
			draft.Target.SourceID, strings.Join(draft.Withheld, ", "))
		fmt.Fprintf(w, "Authorize with: passquest tutor consent %s --reviewer NAME\n", draft.Target.SourceID)
	default:
		fmt.Fprintln(w, "Privacy: first-party material; no imported source text is involved.")
	}
}

func writeNoteDraft(w io.Writer, doc noteDraftDoc) {
	fmt.Fprintln(w, "Pass Quest — note draft (not written)")
	fmt.Fprintf(w, "Target: %s\n", noteTargetText(doc.Draft.Target))
	fmt.Fprintf(w, "Origin: %s", doc.Draft.Origin)
	if doc.Advisory {
		fmt.Fprint(w, " (advisory model text)")
	}
	fmt.Fprintln(w)
	if doc.Draft.Unanswered {
		fmt.Fprintln(w, "Answer state: unanswered; the draft does not write or print the answer key.")
	}
	writeNotePrivacy(w, doc.Draft)
	if len(doc.Draft.Withheld) > 0 {
		fmt.Fprintln(w, "The draft content is withheld until consent is recorded; the preview shows exactly what would be written now.")
	}
	fmt.Fprintf(w, "Front: %s\n", doc.Plain.Front)
	fmt.Fprintf(w, "Back: %s\n", indentNoteText(doc.Plain.Back))
	fmt.Fprintf(w, "Tactic: %s\n", doc.Plain.Tactic)
	fmt.Fprintf(w, "Source: %s\n", doc.Plain.Source)
	if doc.Refinement != nil {
		fmt.Fprintf(w, "Refinement: model %s, %d bytes, sha256 %s, %d ms\n",
			doc.Refinement.Model, doc.Refinement.RequestBytes, doc.Refinement.RequestSHA256, doc.Refinement.LatencyMS)
	}
	fmt.Fprintln(w, "\nRun `passquest note preview` to see the exact Anki field HTML, then `passquest note accept`.")
	fmt.Fprintf(w, "%s\n", noteSyncStatement)
}

func writeNotePreview(w io.Writer, doc notePreviewDoc) {
	fmt.Fprintln(w, "Pass Quest — note preview (not written, not sent)")
	fmt.Fprintf(w, "Target: %s\n", noteTargetText(doc.Draft.Target))
	fmt.Fprintf(w, "Origin: %s", doc.Draft.Origin)
	if doc.Advisory {
		fmt.Fprint(w, " (advisory model text; inspect before accepting)")
	}
	fmt.Fprintln(w)
	if doc.Draft.Unanswered {
		fmt.Fprintln(w, "Answer state: unanswered; the answer key is excluded from this note.")
	}
	writeNotePrivacy(w, doc.Draft)
	fmt.Fprintf(w, "Deck: %s · model: %s\n", doc.Deck, doc.Model)
	fmt.Fprintf(w, "Field HTML sha256: %s\n\n", doc.HTMLSHA256)
	fmt.Fprintf(w, "Front:\n%s\n", doc.Fields.Front)
	fmt.Fprintf(w, "Back:\n%s\n", doc.Fields.Back)
	fmt.Fprintf(w, "Tactic:\n%s\n", doc.Fields.Tactic)
	fmt.Fprintf(w, "Source:\n%s\n", doc.Fields.Source)
	if len(doc.Draft.Withheld) > 0 {
		fmt.Fprintf(w, "\nAccept is refused until consent is recorded.\n")
		fmt.Fprintf(w, "  passquest tutor consent %s --reviewer NAME\n", doc.Draft.Target.SourceID)
	} else {
		fmt.Fprintf(w, "\nWrite it locally with: passquest note accept\n")
	}
	fmt.Fprintf(w, "%s\n", noteSyncStatement)
}

func writeNoteAccept(w io.Writer, doc noteAcceptDoc) {
	fmt.Fprintf(w, "Pass Quest — note %s\n", doc.Note.NoteID)
	if doc.State == "unchanged" {
		fmt.Fprintf(w, "Unchanged: content already at revision %d; no new revision was written.\n", doc.Note.Revision)
	} else {
		fmt.Fprintf(w, "Accepted revision %d", doc.Note.Revision)
		if doc.PreviousRevision > 0 {
			fmt.Fprintf(w, " (previous revision %d retained)", doc.PreviousRevision)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "Target: %s\n", noteTargetText(doc.Note.Target))
	fmt.Fprintf(w, "Front: %s\n", doc.Plain.Front)
	fmt.Fprintf(w, "Back: %s\n", indentNoteText(doc.Plain.Back))
	fmt.Fprintf(w, "Tactic: %s\n", doc.Plain.Tactic)
	fmt.Fprintf(w, "Source: %s\n", doc.Plain.Source)
	fmt.Fprintf(w, "Restricted: %t · origin: %s\n", doc.Note.Restricted, doc.Note.Origin)
	fmt.Fprintf(w, "Anki: %s, note id %d, pushed revision %d\n",
		doc.Note.Anki.State, doc.Note.Anki.NoteID, doc.Note.Anki.PushedRevision)
	fmt.Fprintln(w, "\nNext: passquest note export · passquest note push")
	fmt.Fprintf(w, "%s\n", noteSyncStatement)
}

func writeNoteShow(w io.Writer, doc noteShowDoc) {
	fmt.Fprintln(w, "Pass Quest — local notes")
	if len(doc.Notes) == 0 {
		fmt.Fprintln(w, "No accepted note is recorded.")
		return
	}
	for _, item := range doc.Notes {
		n := item.Note
		deck := ""
		if n.Anki.Deck != "" {
			deck = fmt.Sprintf(" deck=%s", n.Anki.Deck)
			if n.Anki.DeckExpected != "" {
				deck = fmt.Sprintf(" deck=%s (expected %s)", n.Anki.Deck, n.Anki.DeckExpected)
			}
		}
		fmt.Fprintf(w, "\n%s  revision %d  %s  restricted=%t  origin=%s  anki=%s%s\n",
			n.NoteID, n.Revision, n.Target.ContentID+"/"+n.Target.QuestionID, n.Restricted, n.Origin, n.Anki.State, deck)
		fmt.Fprintf(w, "  front: %s\n", n.Front)
		fmt.Fprintf(w, "  back: %s\n", indentNoteText(n.Back))
		fmt.Fprintf(w, "  tactic: %s · source: %s\n", n.Tactic, n.Source)
		for _, revision := range item.Revisions {
			marker := "current"
			if revision.Revision != n.Revision {
				marker = "history"
			}
			fmt.Fprintf(w, "  revision %d (%s, %s): %s\n", revision.Revision, marker, revision.Origin, firstLine(revision.Front))
		}
	}
}

func writeNoteModel(w io.Writer, doc noteModelDoc) {
	fmt.Fprintln(w, "Pass Quest — Anki note model")
	fmt.Fprintf(w, "Deck: %s\nModel: %s\nFields: %s\n", doc.Deck, doc.Model, strings.Join(doc.Fields, ", "))
	fmt.Fprintln(w, "\nCSS:")
	fmt.Fprintln(w, doc.CSS)
	fmt.Fprintf(w, "\nFront template:\n%s\n", doc.FrontTemplate)
	fmt.Fprintf(w, "\nBack template:\n%s\n", doc.BackTemplate)
	fmt.Fprintln(w, "\nRendered sample:")
	fmt.Fprintln(w, doc.Rendered)
	fmt.Fprintf(w, "\nNo external font, stylesheet, script or image is referenced. %s\n", noteSyncStatement)
}

func writeNoteExport(w io.Writer, doc noteExportDoc) {
	fmt.Fprintln(w, "Pass Quest — Anki export")
	fmt.Fprintf(w, "File: %s\n", doc.Path)
	fmt.Fprintf(w, "Notes: %d · bytes: %d · sha256: %s\n", doc.Notes, doc.Bytes, doc.SHA256)
	fmt.Fprintf(w, "Deck: %s · model: %s\n", doc.Deck, doc.Model)
	fmt.Fprintf(w, "Columns directive: %s\n", doc.Header)
	fmt.Fprintln(w, "Import with Anki Desktop: File > Import, then choose the deck and note type above.")
	fmt.Fprintf(w, "%s\n", noteSyncStatement)
}

func writeNotePush(w io.Writer, doc notePushDoc) {
	fmt.Fprintln(w, "Pass Quest — Anki push")
	fmt.Fprintf(w, "Endpoint: %s\n", doc.Endpoint)
	fmt.Fprintf(w, "Deck: %s · model: %s (created: %t)\n", doc.Deck, doc.Model, doc.ModelCreated)
	fmt.Fprintf(w, "Pending %d · pushed %d · already present %d · conflicts %d · failed %d · queued %d\n",
		doc.Counts.Pending, doc.Counts.Pushed, doc.Counts.AlreadyPresent,
		doc.Counts.Conflicts, doc.Counts.Failed, doc.Counts.Queued)
	for _, item := range doc.Notes {
		fmt.Fprintf(w, "  %s rev %d  %s/%s  anki note %d", item.NoteID, item.Revision, item.State, item.Outcome, item.AnkiNoteID)
		if item.Detail != "" {
			fmt.Fprintf(w, "  %s", item.Detail)
		}
		fmt.Fprintln(w)
	}
	if doc.State == "queued" {
		fmt.Fprintln(w, "No endpoint answered; every note stays queued and the next reachable push drains it once.")
	}
	if doc.Counts.DeckMismatch > 0 {
		fmt.Fprintf(w, "Deck mismatch: %d note(s) are not in %q.\n", doc.Counts.DeckMismatch, doc.Deck)
		for _, item := range doc.Notes {
			if item.State != store.AnkiStateDeckMismatch {
				continue
			}
			if item.Deck == "" {
				fmt.Fprintf(w, "  %s rev %d reached no reported deck (anki note %d)\n", item.NoteID, item.Revision, item.AnkiNoteID)
				continue
			}
			fmt.Fprintf(w, "  %s rev %d landed in %q (anki note %d)\n", item.NoteID, item.Revision, item.Deck, item.AnkiNoteID)
		}
		fmt.Fprintf(w, "Move the card into %q in Anki — the note id is kept, so a later revision updates the same card.\n", doc.Deck)
	}
	if doc.Counts.Conflicts > 0 {
		fmt.Fprintln(w, "Conflicts were not overwritten. Inspect or edit the card in Anki, then push again.")
		forceable, idUnknown := false, false
		for _, item := range doc.Notes {
			switch item.Code {
			case "fields_changed":
				forceable = true
			case "note_id_unknown":
				idUnknown = true
			}
		}
		if forceable {
			fmt.Fprintln(w, "  --force overwrites a card whose Anki fields changed.")
		}
		if idUnknown {
			fmt.Fprintln(w, "  A duplicate conflict has no Anki note id; remove that card in Anki or align the local content. --force cannot update it.")
		}
	}
	if doc.Error != nil {
		fmt.Fprintf(w, "%s: %s\n", doc.Error.Code, doc.Error.Message)
	}
	fmt.Fprintf(w, "%s\n", noteSyncStatement)
}

func indentNoteText(value string) string {
	return strings.ReplaceAll(value, "\n", "\n        ")
}

func firstLine(value string) string {
	if index := strings.IndexByte(value, '\n'); index >= 0 {
		return value[:index]
	}
	return value
}

// noteCountsOf tallies push item states.
func noteCountsOf(items []notePushItemView) notePushCounts {
	var counts notePushCounts
	for _, item := range items {
		switch item.State {
		case store.AnkiStatePushed:
			counts.Pushed++
		case store.AnkiStateAlreadyExists:
			counts.AlreadyPresent++
		case store.AnkiStateConflict:
			counts.Conflicts++
		case store.AnkiStateFailed:
			counts.Failed++
		case store.AnkiStateQueued:
			counts.Queued++
		case store.AnkiStateDeckMismatch:
			counts.DeckMismatch++
		}
	}
	return counts
}
