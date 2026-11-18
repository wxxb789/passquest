// Package notes orchestrates the note workflow: the deterministic proposal and
// its one explicit refinement, the draft as the learner edits it, accept and
// skip, and the local-note listing. Results carry exactly the inputs the CLI's
// view builders and the desktop's studio mappers read; nothing here renders.
// The CLI's note commands and the window's note methods are the two adapters.
package notes

import (
	"context"
	"fmt"
	"strings"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
)

// Effective is one draft's current content after consent and refinement are
// applied. Preview and accept both use it, so the payload a learner inspects
// is the payload that is written.
type Effective struct {
	Draft    store.NoteDraft
	Input    note.Input
	Built    note.Draft
	Front    string
	Back     string
	Tactic   string
	Origin   note.Origin
	Model    string
	Advisory bool
	Fields   note.Fields
}

// table maps the derivation sentinels an entry point classifies to the codes
// the CLI prints today.
var table = []kernel.Rule{
	{Is: note.ErrLowValue, Code: "low_value"},
	{Is: note.ErrUnansweredTarget, Code: "invalid_input"},
	{Is: note.ErrQuestionUnavailable, Code: "content_unavailable"},
}

// classify wraps the derivation sentinels one entry point returns; a refusal
// already raised keeps its own code, anything else stays an internal error.
func classify(err error) error { return kernel.Classify(err, table) }

// Locate resolves one draft row: id 0 selects the latest open draft. It is
// the lookup half of Draft, so the window can close an already accepted draft
// without deriving content it will not show.
func Locate(ctx context.Context, s *study.Service, draftID int64) (store.NoteDraft, error) {
	_ = ctx
	return locate(s, draftID)
}

// Draft resolves one draft and derives its effective content: the payload
// `note preview`, the window's draft view and the accept path all show.
func Draft(ctx context.Context, s *study.Service, draftID int64) (Effective, error) {
	draft, err := Locate(ctx, s, draftID)
	if err != nil {
		return Effective{}, err
	}
	return Derive(ctx, s, draft)
}

// Derive derives the effective content of an already located draft row, so a
// caller that inspects the row first — the window closes an already accepted
// draft without deriving it — reads the draft once, as it did before the seam.
func Derive(ctx context.Context, s *study.Service, draft store.NoteDraft) (Effective, error) {
	_ = ctx
	return draftOf(s, draft)
}

// locate is the draft one entry point works on, or a refusal naming it.
func locate(s *study.Service, id int64) (store.NoteDraft, error) {
	st := kernel.Store(s)
	if id > 0 {
		draft, found, err := st.NoteDraftByID(id)
		if err != nil {
			return store.NoteDraft{}, err
		}
		if !found {
			return store.NoteDraft{}, kernel.Refuse("unknown_draft", "note draft #%d is not recorded", id)
		}
		if draft.Status != store.NoteDraftOpen {
			return store.NoteDraft{}, kernel.Refuse("draft_closed", "note draft #%d is already %s", id, draft.Status)
		}
		return draft, nil
	}
	draft, found, err := st.LatestOpenNoteDraft()
	if err != nil {
		return store.NoteDraft{}, err
	}
	if !found {
		return store.NoteDraft{}, kernel.Refuse("no_draft", "no note draft is open; run passquest note propose first")
	}
	return draft, nil
}

// draftOf is the effective content of an already loaded draft.
func draftOf(s *study.Service, draft store.NoteDraft) (Effective, error) {
	input, err := inputFromDraft(s, draft)
	if err != nil {
		return Effective{}, classify(err)
	}
	built, err := note.Build(input)
	if err != nil {
		return Effective{}, classify(err)
	}
	return EffectiveOf(draft, input, built), nil
}

// inputFromDraft is the derivation input the draft points at, with the consent
// the note text needs.
func inputFromDraft(s *study.Service, draft store.NoteDraft) (note.Input, error) {
	loaded, err := loadContent(s, draft.ContentID, draft.QuestionID)
	if err != nil {
		return note.Input{}, err
	}
	if !loaded.Found {
		return note.Input{}, kernel.Refuse("content_unavailable",
			"question %s of %s is not available in this build; a note needs the reviewed question and its evidence",
			draft.QuestionID, draft.ContentID)
	}
	if draft.ContentVersion != "" && loaded.Content.Version != draft.ContentVersion {
		return note.Input{}, kernel.Refuse("content_changed",
			"the question changed after the draft was created; run passquest note propose again")
	}
	sourceID := loaded.SourceID
	if sourceID == "" {
		sourceID = draft.SourceID
	}
	var record *attempt.Attempt
	if draft.AttemptID > 0 {
		got, err := attemptRecord(s, draft.AttemptID)
		if err != nil {
			return note.Input{}, err
		}
		if got.ContentID != draft.ContentID || got.QuestionID != draft.QuestionID {
			return note.Input{}, kernel.Refuse("content_unavailable",
				"attempt #%d belongs to %s / %s, not the draft's question",
				draft.AttemptID, got.ContentID, got.QuestionID)
		}
		record = &got
	}
	consent := loaded.Private && sourceID != "" && consentGranted(s, sourceID)
	return note.Input{
		Target: note.Target{
			ContentID:      draft.ContentID,
			ContentVersion: loaded.Content.Version,
			QuestionID:     draft.QuestionID,
			SourceID:       sourceID,
			Page:           draft.Page,
			AttemptID:      draft.AttemptID,
			Mark:           draft.Mark,
			Restricted:     loaded.Private,
		},
		Attempt:        record,
		Question:       &loaded.Question,
		PassageText:    loaded.Content.Passage.Text,
		ConsentGranted: consent,
	}, nil
}

// consentGranted reports whether the latest recorded global consent for a
// source is a grant. Note text and its Anki export are a different destination
// from provider inference, so a provider-scoped tutor grant never authorizes
// them; only the global decision does. A lookup error is treated as no consent.
func consentGranted(s *study.Service, sourceID string) bool {
	event, ok, err := kernel.Store(s).TutorConsent(sourceID, "")
	return err == nil && ok && event.Granted
}

// EffectiveOf applies a draft's refinement to an already built input, so a
// command that has just derived the draft does not derive it again.
func EffectiveOf(draft store.NoteDraft, input note.Input, built note.Draft) Effective {
	effective := Effective{
		Draft:  draft,
		Input:  input,
		Built:  built,
		Front:  built.Front,
		Back:   built.Back,
		Tactic: built.Tactic,
		Origin: note.OriginDeterministic,
	}
	// Accepted values are the base for a repeated or incremental accept, so an
	// edit is never replaced by the original proposal. A withheld draft keeps
	// the deterministic empty fields instead: revoked consent hides the text.
	if len(built.Withheld) == 0 && draft.AcceptedFront != "" {
		effective.Front = draft.AcceptedFront
		effective.Back = draft.AcceptedBack
		effective.Tactic = draft.AcceptedTactic
		switch origin := note.Origin(draft.AcceptedOrigin); origin {
		case note.OriginDeterministic, note.OriginRefined, note.OriginEdited:
			effective.Origin = origin
		}
		effective.Model = draft.AcceptedModel
		effective.Advisory = effective.Origin == note.OriginRefined
	} else {
		refined := draft.Origin == string(note.OriginRefined) &&
			strings.TrimSpace(draft.RefinedFront) != "" && strings.TrimSpace(draft.RefinedBack) != "" &&
			len(built.Withheld) == 0
		if refined && built.Unanswered && input.Question != nil &&
			note.RevealsAnswer(*input.Question, draft.RefinedFront+"\n"+draft.RefinedBack) {
			// A stale or hostile refinement must not put an unanswered key into
			// preview or accept; the deterministic draft is key-safe.
			refined = false
		}
		if refined {
			effective.Front = draft.RefinedFront
			effective.Back = draft.RefinedBack
			effective.Origin = note.OriginRefined
			effective.Model = draft.Model
			effective.Advisory = true
		}
	}
	effective.Fields = note.HTML(effective.Front, effective.Back, effective.Tactic, built.SourceLabel)
	return effective
}

// RefinementRecord reads one recorded refinement row: the window restores an
// unused suggestion's provenance from it.
func RefinementRecord(ctx context.Context, s *study.Service, id int64) (store.NoteRefinement, error) {
	_ = ctx
	records, err := kernel.Store(s).NoteRefinements()
	if err != nil {
		return store.NoteRefinement{}, err
	}
	for _, record := range records {
		if record.ID == id {
			return record, nil
		}
	}
	return store.NoteRefinement{}, fmt.Errorf("note refinement #%d is not recorded", id)
}

// ConsentMessage is the refusal text naming the command that authorizes a
// withheld source; both adapters refuse with it.
func ConsentMessage(draft store.NoteDraft) string {
	return fmt.Sprintf(
		"source %s is private and its text is withheld; inspect the preview, then authorize with:\n  passquest tutor consent %s --reviewer NAME",
		draft.SourceID, draft.SourceID)
}

// refineConsentMessage names the provider that the inference grant has to
// cover, so a provider-scoped denial is actionable rather than looking like a
// missing global grant.
func refineConsentMessage(sourceID, providerID string) string {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return ConsentMessage(store.NoteDraft{SourceID: sourceID})
	}
	return fmt.Sprintf(
		"source %s is private and its consent does not cover provider %s; authorize with:\n  passquest tutor consent %s --provider %s --reviewer NAME",
		sourceID, providerID, sourceID, providerID)
}

// Limits is the disclosure every note document carries: what a note is, and
// what Anki and the app do or do not verify. The CLI's note documents and the
// window's note documents show the same statements.
func Limits() []string {
	return []string{
		"A note is a study aid derived from recorded evidence, not a verified answer key or grade.",
		"Anki owns the card and its review scheduling after import or push.",
		"AnkiWeb and mobile synchronization are your own manual actions in Anki; Pass Quest never contacts AnkiWeb or a phone.",
		"A real Anki Desktop write is unverified on this machine; the adapter is proven against a loopback AnkiConnect stub.",
	}
}

// DeckExpected is the deck a note's card belongs in, and is empty unless the
// last completed deck read found the card in another one. Both note documents
// set their deck_expected field from it.
func DeckExpected(n store.Note) string {
	if n.AnkiDeck != "" && n.AnkiDeck != note.Deck {
		return note.Deck
	}
	return ""
}
