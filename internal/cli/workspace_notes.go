package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/wxxb789/passquest/internal/anki"
	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/studio"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/notes"
	"github.com/wxxb789/passquest/internal/study/outbox"
)

// ProposeNote is `note propose`: a deterministic draft from a recorded
// attempt, or from a passage mark on a reviewed question.
func (w *Workspace) ProposeNote(request studio.NoteRequest) (studio.NoteDraft, error) {
	// The command's flag checks ran before its store opened; reproduce them,
	// in order, so an invalid request still refuses without opening a store.
	if (request.ContentID == "") != (request.QuestionID == "") {
		return studio.NoteDraft{}, cliCoded(study.Refuse("invalid_input", "--content and --question must be given together"))
	}
	if request.ContentID != "" && request.AttemptID != 0 {
		return studio.NoteDraft{}, cliCoded(study.Refuse("invalid_input", "--attempt cannot be combined with --content/--question"))
	}
	mark := note.NormalizeMark(request.Mark)
	if mark != "" && !note.ValidMark(mark) {
		return studio.NoteDraft{}, cliCoded(study.Refuse("invalid_mark",
			"--mark must be 1..%d characters of ordinary text", note.MaxMarkRunes))
	}
	if request.ContentID != "" && mark == "" {
		return studio.NoteDraft{}, cliCoded(study.Refuse("invalid_input",
			"a question without a stored attempt needs --mark naming the word or pattern worth keeping"))
	}
	var effective notes.Effective
	w.mu.Lock()
	err := w.codedCall(func(svc *study.Service) error {
		var err error
		effective, _, err = notes.Propose(context.Background(), svc, notes.ProposeInput{
			AttemptID: request.AttemptID, ContentID: request.ContentID, QuestionID: request.QuestionID,
			Mark: request.Mark,
		})
		return err
	})
	w.mu.Unlock()
	if err != nil {
		return studio.NoteDraft{}, err
	}
	return studioNoteDraft(noteDraftViewOf(effective), notePlainViewOf(effective), effective.Advisory, noteLimits)
}

// OpenNoteDraft is the draft `note preview` resolves without --draft (the
// latest open draft) while it is still pending: proposed and neither accepted
// nor skipped. It is nil otherwise. An accepted draft stays open in the store
// for a repeated accept, but the window has already closed it. It lets the
// window restore a pending draft after a restart: the draft's own fields, with
// a refinement the learner has not used returned apart as its Suggestion.
func (w *Workspace) OpenNoteDraft() (*studio.NoteDraft, error) {
	var open *studio.NoteDraft
	err := w.lockedNote(func(svc *study.Service) error {
		draft, err := notes.Locate(context.Background(), svc, 0)
		if err != nil {
			var refusal *study.Error
			if errors.As(err, &refusal) && refusal.Code == "no_draft" {
				return nil
			}
			return err
		}
		if draft.AcceptedFront != "" {
			return nil
		}
		effective, err := notes.Derive(context.Background(), svc, draft)
		if err != nil {
			return err
		}
		view := noteWindowOf(effective)
		restored, err := studioNoteDraft(noteDraftViewOf(view.base), notePlainViewOf(view.base), view.base.Advisory, noteLimits)
		if err != nil {
			return err
		}
		if view.suggestion != nil {
			record, err := notes.RefinementRecord(context.Background(), svc, draft.RefinementID)
			if err != nil {
				return err
			}
			suggestion := studioNoteRefinement(draft.ID, record, *view.suggestion)
			restored.Suggestion = &suggestion
		}
		open = &restored
		return nil
	})
	return open, err
}

// noteWindow is a draft as the window edits it. base holds the draft's own
// fields: the deterministic proposal, or the values accepted last. A stored
// refinement the learner has not used stays apart as suggestion; it reaches
// the card only when the learner puts it in the fields. effective is what
// `note accept` without edits would record.
type noteWindow struct {
	effective  notes.Effective
	base       notes.Effective
	suggestion *studio.NoteFields
}

func noteWindowOf(effective notes.Effective) noteWindow {
	view := noteWindow{effective: effective, base: effective}
	if effective.Draft.AcceptedFront != "" || effective.Origin != note.OriginRefined {
		return view
	}
	own := effective.Draft
	own.Origin = string(note.OriginDeterministic)
	own.Model, own.RefinedFront, own.RefinedBack = "", "", ""
	view.base = notes.EffectiveOf(own, effective.Input, effective.Built)
	view.base.Draft = effective.Draft
	view.suggestion = &studio.NoteFields{Front: effective.Front, Back: effective.Back, Tactic: effective.Tactic}
	return view
}

func noteFieldsOf(e notes.Effective) studio.NoteFields {
	return studio.NoteFields{Front: e.Front, Back: e.Back, Tactic: e.Tactic}
}

func studioNoteRefinement(draftID int64, record store.NoteRefinement, suggestion studio.NoteFields) studio.NoteRefinement {
	return studio.NoteRefinement{
		State:         "advisory",
		DraftID:       draftID,
		RefinementID:  record.ID,
		Suggestion:    suggestion,
		Provider:      record.Provider,
		Model:         record.Model,
		Advisory:      true,
		RequestSHA256: record.RequestSHA256,
		RequestBytes:  record.RequestBytes,
		LatencyMS:     record.LatencyMS,
		Limits:        noteLimits,
	}
}

func studioNoteDraft(view noteDraftView, plain notePlainView, advisory bool, limits []string) (studio.NoteDraft, error) {
	draft := studio.NoteDraft{
		ID:             view.ID,
		Origin:         view.Origin,
		Restricted:     view.Restricted,
		ConsentGranted: view.ConsentGranted,
		Withheld:       append([]string{}, view.Withheld...),
		Unanswered:     view.Unanswered,
		TacticID:       view.TacticID,
		Fields:         studio.NoteFields{Front: plain.Front, Back: plain.Back, Tactic: plain.Tactic},
		Source:         plain.Source,
		Advisory:       advisory,
		Limits:         limits,
	}
	if err := convertJSON(view.Target, &draft.Target); err != nil {
		return studio.NoteDraft{}, workspaceError(err)
	}
	return draft, nil
}

// PreviewNote renders the draft with the real note model. Non-empty fields
// are the learner's edits, sanitised exactly as `note accept` sanitises them;
// empty fields keep the draft's own values, never an unused suggestion. The card documents come from
// note.RenderSides and note.CardDocument: the same templates and stylesheet a
// push sends to Anki.
func (w *Workspace) PreviewNote(draftID int64, fields studio.NoteFields) (studio.NotePreview, error) {
	var preview studio.NotePreview
	err := w.lockedNote(func(svc *study.Service) error {
		view, edited, err := noteEdit(svc, draftID, fields)
		if err != nil {
			return err
		}
		effective := view.base
		rendered := note.HTML(edited.Front, edited.Back, edited.Tactic, effective.Built.SourceLabel)
		question, answer := note.RenderSides(rendered)
		model := note.TemplateModel()
		preview = studio.NotePreview{
			DraftID: effective.Draft.ID,
			Fields:  edited,
			Source:  effective.Built.SourceLabel,
			Question: studio.NoteCardSide{
				Light: note.CardDocument(question, model.CSS, false),
				Night: note.CardDocument(question, model.CSS, true),
			},
			Answer: studio.NoteCardSide{
				Light: note.CardDocument(answer, model.CSS, false),
				Night: note.CardDocument(answer, model.CSS, true),
			},
			HTML:         answer,
			CSS:          model.CSS,
			FieldsSHA256: rendered.Hash(),
			HTMLSHA256:   rendered.Hash(),
			CSSSHA256:    note.CSSSHA256(),
			ModelVersion: model.Version,
			Deck:         note.Deck,
			Model:        note.ModelName,
			Edited:       edited != noteFieldsOf(effective) && (view.suggestion == nil || edited != *view.suggestion),
		}
		return nil
	})
	return preview, err
}

// RefineNotePreview is what `note propose --refine` would send for this draft,
// built by the same request builder, before anything leaves the machine:
// provider and model, whether the endpoint is external, the text.chat
// verification, the credential's presence, the consent that covers this
// provider, the exact messages with their hash and size and the budget slot.
// Nothing is sent or reserved. Messages stay empty while consent withholds the
// source text, because that text must not be shown as if it would be sent.
func (w *Workspace) RefineNotePreview(draftID int64) (studio.NoteRefinePreview, error) {
	var preview studio.NoteRefinePreview
	if err := w.codedCall(func(svc *study.Service) error {
		resolved, err := notes.RefinePreview(context.Background(), svc, draftID)
		if err != nil {
			return err
		}
		preview = studioNoteRefinePreviewOf(resolved)
		return nil
	}); err != nil {
		return studio.NoteRefinePreview{}, err
	}
	budget, err := w.Budget()
	if err != nil {
		return studio.NoteRefinePreview{}, err
	}
	preview.Budget = budgetSlot(budget, preview.Provider)
	return preview, nil
}

// studioNoteRefinePreviewOf renders one resolved refinement request the way
// the shimmed preview decoded it: the verification state as its rendered
// sentence, the message list always a list, and the request identity only when
// consent covers the source.
func studioNoteRefinePreviewOf(resolved notes.Preview) studio.NoteRefinePreview {
	preview := studio.NoteRefinePreview{
		DraftID:    resolved.DraftID,
		Provider:   resolved.ProviderID,
		Model:      resolved.Config.Model,
		Endpoint:   resolved.Config.Endpoint,
		External:   !loopbackEndpoint(resolved.Config.Endpoint),
		Verified:   true,
		Credential: studio.Credential{Env: resolved.CredentialEnv, Present: resolved.CredentialPresent},
		Unanswered: resolved.Unanswered,
		Messages:   []studio.CoachMessage{},
		MaxTokens:  resolved.MaxTokens,
		Limits:     noteLimits,
	}
	if resolved.HasCapability {
		state := capabilityViewOf(resolved.Capability)
		preview.TextChatState = renderCapability(state)
		preview.Verified = state.Verified
		if !state.Verified {
			preview.Action = fmt.Sprintf("passquest model test %s/%s", resolved.ProviderID, resolved.Config.Model)
		}
	}
	consent := studio.CoachConsent{
		PrivateSource:    resolved.Consent.Private,
		SourceID:         resolved.Consent.SourceID,
		Required:         resolved.Consent.Required,
		Granted:          resolved.Consent.Granted,
		SourceTextShared: resolved.Consent.SourceTextShared,
		Withheld:         append([]string{}, resolved.Consent.Withheld...),
	}
	for _, message := range resolved.Messages {
		preview.Messages = append(preview.Messages, studio.CoachMessage{Role: message.Role, Content: message.Content})
	}
	preview.RequestSHA256, preview.RequestBytes = resolved.RequestSHA256, resolved.RequestBytes
	preview.Consent = consent
	return preview
}

// RefineNote asks the active provider once for a better front and back,
// through the same path as `note propose --refine`: the active pair must be
// verified, the credential present and the consent must cover the provider; a
// budget slot is reserved, and a provider failure is recorded and returned
// with its typed code. The suggestion is advisory and is never applied by the
// window: the learner copies what they keep into the fields and accepts them
// with AcceptNote, which records a refined note only for the suggestion's own
// fields.
func (w *Workspace) RefineNote(draftID int64) (studio.NoteRefinement, error) {
	if err := w.requireNode(w.hostTutor); err != nil {
		return studio.NoteRefinement{}, err
	}
	if !w.startRefining(draftID) {
		return studio.NoteRefinement{}, &studio.Error{Code: "refine_in_progress",
			Message: "a refinement of this draft is already waiting for its reply; nothing was sent"}
	}
	defer w.stopRefining(draftID)
	var refinement studio.NoteRefinement
	if err := w.codedCall(func(svc *study.Service) error {
		refined, err := notes.Refine(context.Background(), svc, draftID)
		if err != nil {
			return err
		}
		if refined.Failure != nil {
			return study.Refuse(refined.Failure.Code, "%s", refined.Failure.Message)
		}
		refinement = studioNoteRefinement(refined.Draft.ID, refined.Record,
			studio.NoteFields{Front: refined.Draft.RefinedFront, Back: refined.Draft.RefinedBack, Tactic: refined.Built.Tactic})
		return nil
	}); err != nil {
		return studio.NoteRefinement{}, err
	}
	return refinement, nil
}

// lockedNote runs fn with one study service opened for this call while holding
// the write lock, mapping its error the way the three note-window methods
// mapped it before the notes table moved behind the seam.
func (w *Workspace) lockedNote(fn func(svc *study.Service) error) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	svc, err := study.Open(w.ports)
	if err != nil {
		return noteWindowError(err)
	}
	defer svc.Close()
	return noteWindowError(fn(svc))
}

// noteWindowError maps a note-window call's error: a study refusal — or a note
// sentinel the notes module already classified — becomes the studio error with
// its code, and anything else (a raw store error) keeps the direct-path
// mapping. This is the one place the window still honours the notes area's own
// codes instead of the desktop's sentinel table.
func noteWindowError(err error) error {
	if err == nil {
		return nil
	}
	var e *study.Error
	if errors.As(err, &e) {
		return withAction(&studio.Error{Code: e.Code, Message: e.Message})
	}
	return workspaceError(err)
}

// noteEdit loads a draft as the window edits it and applies sanitised edits
// over the draft's own fields: an emptied field keeps the draft's own value,
// never an unused suggestion.
func noteEdit(svc *study.Service, draftID int64, fields studio.NoteFields) (noteWindow, studio.NoteFields, error) {
	effective, err := notes.Draft(context.Background(), svc, draftID)
	if err != nil {
		return noteWindow{}, studio.NoteFields{}, err
	}
	if len(effective.Built.Withheld) > 0 {
		return noteWindow{}, studio.NoteFields{}, study.Refuse("consent_required", "%s", notes.ConsentMessage(effective.Draft))
	}
	view := noteWindowOf(effective)
	edited := noteFieldsOf(view.base)
	if strings.TrimSpace(fields.Front) != "" {
		if edited.Front, err = note.SanitizeFront(fields.Front); err != nil {
			return noteWindow{}, studio.NoteFields{}, study.Refuse("invalid_front", "%s", err.Error())
		}
	}
	if strings.TrimSpace(fields.Back) != "" {
		if edited.Back, err = note.SanitizeBack(fields.Back); err != nil {
			return noteWindow{}, studio.NoteFields{}, study.Refuse("invalid_back", "%s", err.Error())
		}
	}
	if strings.TrimSpace(fields.Tactic) != "" {
		if edited.Tactic, err = note.SanitizeTactic(fields.Tactic); err != nil {
			return noteWindow{}, studio.NoteFields{}, study.Refuse("invalid_tactic", "%s", err.Error())
		}
	}
	return view, edited, nil
}

// AcceptNote records the learner's fields as `note accept` records them. The
// origin follows what the learner accepts: the draft's own fields keep its
// origin, the unused suggestion's own fields are a refined note, and anything
// else is an edited one.
func (w *Workspace) AcceptNote(draftID int64, fields studio.NoteFields) (studio.NoteAccept, error) {
	var settled notes.Settled
	err := w.lockedNote(func(svc *study.Service) error {
		view, edited, err := noteEdit(svc, draftID, fields)
		if err != nil {
			return err
		}
		recorded, origin := view.base, view.base.Origin
		switch {
		case edited == noteFieldsOf(view.base):
		case view.suggestion != nil && edited == *view.suggestion:
			recorded, origin = view.effective, view.effective.Origin
		default:
			recorded, origin = view.effective, note.OriginEdited
		}
		settled, err = notes.Settle(context.Background(), svc, notes.SettleInput{
			DraftID: recorded.Draft.ID, Action: notes.Accept, Effective: recorded,
			Front: edited.Front, Back: edited.Back, Tactic: edited.Tactic, Origin: origin,
		})
		return err
	})
	if err != nil {
		return studio.NoteAccept{}, err
	}
	return studio.NoteAccept{
		State:            settled.State,
		Note:             studioNote(noteViewOf(settled.Note)),
		Created:          settled.Created,
		Changed:          settled.Changed,
		PreviousRevision: settled.Previous,
	}, nil
}

// SkipNote is `note skip`: the draft is closed and no note is written.
func (w *Workspace) SkipNote(draftID int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.codedCall(func(svc *study.Service) error {
		_, err := notes.Settle(context.Background(), svc, notes.SettleInput{DraftID: draftID, Action: notes.Skip})
		return err
	})
}

// Notes lists every local note with its Anki state.
func (w *Workspace) Notes() (studio.NoteList, error) {
	var list studio.NoteList
	if err := w.codedCall(func(svc *study.Service) error {
		listed, err := notes.List(context.Background(), svc, "")
		if err != nil {
			return err
		}
		list = studio.NoteList{Notes: []studio.Note{}, Limits: noteLimits}
		for _, item := range listed {
			list.Notes = append(list.Notes, studioNote(noteViewOf(item.Note)))
		}
		return nil
	}); err != nil {
		return studio.NoteList{}, err
	}
	err := w.withStore(func(st *store.Store) error {
		var err error
		list.Counts, err = ankiCounts(st)
		return err
	})
	return list, err
}

// PushNotes is `note push [--force]` through the outbox. An unreachable
// AnkiConnect leaves the notes queued and is reported in the result's Error;
// force keeps the local version of a conflicting note (Q-0011-07).
func (w *Workspace) PushNotes(force bool) (studio.NotePush, error) {
	pending := 0
	if err := w.withStore(func(st *store.Store) error {
		outbox, err := st.PendingNoteOutbox()
		pending = len(outbox)
		return err
	}); err != nil {
		return studio.NotePush{}, err
	}
	if pending > 0 {
		if err := w.requireNode(w.hostAnki); err != nil {
			return studio.NotePush{}, err
		}
	}
	// The shimmed call ran `note push` without flags, so the endpoint came from
	// the environment or the loopback default and the timeout stayed the
	// flag's default.
	endpoint, err := anki.NormalizeEndpoint(noteAnkiEndpoint("", w.getenv))
	if err != nil {
		return studio.NotePush{}, cliCoded(study.Refuse("invalid_endpoint", "%s", err.Error()))
	}
	var push studio.NotePush
	if err := w.codedCall(func(svc *study.Service) error {
		out, err := outbox.Push(context.Background(), svc, outbox.PushRequest{
			Endpoint: endpoint, TimeoutSeconds: defaultAnkiTimeoutSeconds, Force: force,
		})
		if err != nil {
			return err
		}
		if out.Report == nil {
			// The push stopped before its report: the CLI printed this
			// failure's code and message as its error document.
			return study.Refuse(out.Failure.Code, "%s", out.Failure.Message)
		}
		report := out.Report
		push = studio.NotePush{
			State:    report.State,
			Endpoint: report.Endpoint,
			Deck:     note.Deck,
			Model:    note.ModelName,
			Counts: studio.NotePushCounts{
				Pending:        report.Counts.Pending,
				Pushed:         report.Counts.Pushed,
				AlreadyPresent: report.Counts.AlreadyPresent,
				Conflicts:      report.Counts.Conflicts,
				Failed:         report.Counts.Failed,
				Queued:         report.Counts.Queued,
				DeckMismatch:   report.Counts.DeckMismatch,
			},
			Notes:           []studio.NotePushItem{},
			Limits:          noteLimits,
			ModelUpdated:    report.ModelUpdated,
			ModelCustomized: report.ModelCustomized,
		}
		for _, item := range report.Items {
			push.Notes = append(push.Notes, studio.NotePushItem{
				NoteID: item.NoteID, Revision: item.Revision,
				AnkiNoteID: item.AnkiNoteID, State: item.State, Outcome: item.Outcome,
				Code: item.Code, Detail: item.Detail, Deck: item.Deck,
			})
		}
		if report.Error != nil {
			push.Error = withAction(&studio.Error{Code: report.Error.Code, Message: report.Error.Message})
		}
		return nil
	}); err != nil {
		return studio.NotePush{}, err
	}
	return push, nil
}

func studioNote(n noteView) studio.Note {
	view := studio.Note{
		NoteID:         n.NoteID,
		Revision:       n.Revision,
		Fields:         studio.NoteFields{Front: n.Front, Back: n.Back, Tactic: n.Tactic},
		Source:         n.Source,
		Restricted:     n.Restricted,
		Origin:         n.Origin,
		AnkiState:      n.Anki.State,
		AnkiNoteID:     n.Anki.NoteID,
		PushedRevision: n.Anki.PushedRevision,
		Deck:           n.Anki.Deck,
		DeckExpected:   n.Anki.DeckExpected,
		CreatedAt:      n.CreatedAt,
		UpdatedAt:      n.UpdatedAt,
	}
	_ = convertJSON(n.Target, &view.Target)
	return view
}

// startRefining claims draftID for one refinement; false when one is in flight.
func (w *Workspace) startRefining(draftID int64) bool {
	w.refiningMu.Lock()
	defer w.refiningMu.Unlock()
	if w.refining[draftID] {
		return false
	}
	if w.refining == nil {
		w.refining = map[int64]bool{}
	}
	w.refining[draftID] = true
	return true
}

func (w *Workspace) stopRefining(draftID int64) {
	w.refiningMu.Lock()
	defer w.refiningMu.Unlock()
	delete(w.refining, draftID)
}
