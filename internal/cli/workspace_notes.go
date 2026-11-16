package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/studio"
)

// ProposeNote is `note propose`: a deterministic draft from a recorded
// attempt, or from a passage mark on a reviewed question.
func (w *Workspace) ProposeNote(request studio.NoteRequest) (studio.NoteDraft, error) {
	args := []string{"note", "propose"}
	if request.AttemptID != 0 {
		args = append(args, "--attempt", strconv.FormatInt(request.AttemptID, 10))
	}
	if request.ContentID != "" || request.QuestionID != "" {
		args = append(args, "--content="+request.ContentID, "--question="+request.QuestionID)
	}
	if strings.TrimSpace(request.Mark) != "" {
		args = append(args, "--mark="+request.Mark)
	}
	w.mu.Lock()
	result := w.run(args...)
	w.mu.Unlock()
	var doc noteDraftDoc
	if err := result.decode(&doc); err != nil {
		return studio.NoteDraft{}, err
	}
	return studioNoteDraft(doc.Draft, doc.Plain, doc.Advisory, doc.Limits)
}

// OpenNoteDraft is the draft `note preview` resolves without --draft (the
// latest open draft) while it is still pending: proposed and neither accepted
// nor skipped. It is nil otherwise. An accepted draft stays open in the store
// for a repeated accept, but the window has already closed it. It lets the
// window restore a pending draft after a restart: the draft's own fields, with
// a refinement the learner has not used returned apart as its Suggestion.
func (w *Workspace) OpenNoteDraft() (*studio.NoteDraft, error) {
	var open *studio.NoteDraft
	err := w.withStore(func(st *store.Store) error {
		draft, err := noteDraftFor(st, 0)
		if code, _, ok := noteErrorCode(err); ok && code == "no_draft" {
			return nil
		}
		if err != nil {
			return noteStudioError(err)
		}
		if draft.AcceptedFront != "" {
			return nil
		}
		effective, err := noteEffectiveFor(st, draft)
		if err != nil {
			return noteStudioError(err)
		}
		view := noteWindowOf(effective)
		restored, err := studioNoteDraft(noteDraftViewOf(view.base), notePlainViewOf(view.base), view.base.Advisory, noteLimits)
		if err != nil {
			return err
		}
		if view.suggestion != nil {
			record, err := noteRefinementRecord(st, draft.RefinementID)
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
	effective  noteEffective
	base       noteEffective
	suggestion *studio.NoteFields
}

func noteWindowOf(effective noteEffective) noteWindow {
	view := noteWindow{effective: effective, base: effective}
	if effective.Draft.AcceptedFront != "" || effective.Origin != note.OriginRefined {
		return view
	}
	own := effective.Draft
	own.Origin = string(note.OriginDeterministic)
	own.Model, own.RefinedFront, own.RefinedBack = "", "", ""
	view.base = noteEffectiveOf(own, effective.Input, effective.Built)
	view.base.Draft = effective.Draft
	view.suggestion = &studio.NoteFields{Front: effective.Front, Back: effective.Back, Tactic: effective.Tactic}
	return view
}

func noteFieldsOf(e noteEffective) studio.NoteFields {
	return studio.NoteFields{Front: e.Front, Back: e.Back, Tactic: e.Tactic}
}

func noteRefinementRecord(st *store.Store, id int64) (store.NoteRefinement, error) {
	records, err := st.NoteRefinements()
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
	err := w.withStore(func(st *store.Store) error {
		view, edited, err := noteEdit(st, draftID, fields)
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
	err := w.invoke(func(st *store.Store, opt Options) int {
		draft, err := noteDraftFor(st, draftID)
		if err != nil {
			return noteCommandError(opt, true, err)
		}
		input, err := noteInputFromDraft(st, draft)
		if err != nil {
			return noteCommandError(opt, true, err)
		}
		built, err := note.Build(input)
		if err != nil {
			return noteCommandError(opt, true, err)
		}
		cfg, configured, err := st.TutorConfig()
		if err != nil {
			return internalError(opt, true, err)
		}
		if !configured {
			return usageError(opt, true, "not_configured",
				"no tutor provider is configured; run passquest tutor config --endpoint URL --model NAME, or omit --refine")
		}
		cfg, _, _, code := resolveActive(cfg, opt, true)
		if code != 0 {
			return code
		}
		request, err := note.BuildRefineRequest(cfg, built, noteRefineMaterial(input, built))
		if err != nil {
			return usageError(opt, true, "invalid_config", err.Error())
		}
		providerID := strings.TrimSpace(cfg.Provider)
		view := tutorProviderViewOf(cfg, opt)
		preview = studio.NoteRefinePreview{
			DraftID:    draft.ID,
			Provider:   providerID,
			Model:      cfg.Model,
			Endpoint:   cfg.Endpoint,
			External:   !loopbackEndpoint(cfg.Endpoint),
			Verified:   true,
			Credential: studio.Credential{Env: view.CredentialEnv, Present: view.CredentialPresent},
			Unanswered: built.Unanswered,
			Messages:   []studio.CoachMessage{},
			MaxTokens:  request.Chat.MaxTokens,
			Limits:     noteLimits,
		}
		if providerID != "" {
			index, err := loadVerificationIndex(st)
			if err != nil {
				return internalError(opt, true, err)
			}
			state := capabilityView(provider.CapTextChat, true, index.forModel(providerID, cfg.Model))
			preview.TextChatState = renderCapability(state)
			preview.Verified = state.Verified
			if !state.Verified {
				preview.Action = fmt.Sprintf("passquest model test %s/%s", providerID, cfg.Model)
			}
		}
		// The note's own text needs the global decision (built.Withheld); the
		// provider needs the consent that covers it, exactly as noteRefine checks.
		consent := studio.CoachConsent{
			PrivateSource: input.Target.Restricted,
			SourceID:      input.Target.SourceID,
			Required:      input.Target.Restricted && input.Target.SourceID != "",
			Withheld:      append([]string{}, built.Withheld...),
		}
		consent.Granted = !consent.Required
		if consent.Required && len(built.Withheld) == 0 {
			event, ok, err := st.TutorConsent(input.Target.SourceID, providerID)
			if err != nil {
				return internalError(opt, true, err)
			}
			consent.Granted = ok && event.Granted
		}
		if consent.Granted {
			consent.SourceTextShared = consent.PrivateSource
			for _, message := range request.Chat.Messages {
				preview.Messages = append(preview.Messages, studio.CoachMessage{Role: message.Role, Content: message.Content})
			}
			preview.RequestSHA256, preview.RequestBytes = note.RequestIdentity(request.Body)
		} else {
			for _, field := range []string{"front", "back", "passage", "question_stem", "evidence"} {
				if !slices.Contains(consent.Withheld, field) {
					consent.Withheld = append(consent.Withheld, field)
				}
			}
		}
		preview.Consent = consent
		return 0
	})
	if err != nil {
		return studio.NoteRefinePreview{}, err
	}
	budget, err := w.Budget()
	if err != nil {
		return studio.NoteRefinePreview{}, err
	}
	preview.Budget = budgetSlot(budget, preview.Provider)
	return preview, nil
}

// RefineNote asks the active provider once for a better front and back,
// through the same path as `note propose --refine` (noteRefine): the active
// pair must be verified, the credential present and the consent must cover
// the provider; a budget slot is reserved, and a provider failure is recorded
// and returned with its typed code. The suggestion is advisory and is never
// applied by the window: the learner copies what they keep into the fields and
// accepts them with AcceptNote, which records a refined note only for the
// suggestion's own fields.
func (w *Workspace) RefineNote(draftID int64) (studio.NoteRefinement, error) {
	if err := w.requireNode(w.hostTutor); err != nil {
		return studio.NoteRefinement{}, err
	}
	var refinement studio.NoteRefinement
	err := w.invoke(func(st *store.Store, opt Options) int {
		draft, err := noteDraftFor(st, draftID)
		if err != nil {
			return noteCommandError(opt, true, err)
		}
		input, err := noteInputFromDraft(st, draft)
		if err != nil {
			return noteCommandError(opt, true, err)
		}
		built, err := note.Build(input)
		if err != nil {
			return noteCommandError(opt, true, err)
		}
		if len(built.Withheld) > 0 {
			return usageError(opt, true, "consent_required", noteConsentMessage(draft))
		}
		refined, view, code := noteRefine(st, draft, input, built, opt, true)
		if code != 0 {
			return code
		}
		record, err := noteRefinementRecord(st, view.ID)
		if err != nil {
			return internalError(opt, true, err)
		}
		refinement = studioNoteRefinement(refined.ID, record,
			studio.NoteFields{Front: refined.RefinedFront, Back: refined.RefinedBack, Tactic: built.Tactic})
		return 0
	})
	return refinement, err
}

// invoke runs fn as one in-process command on a store opened for the call:
// fn reports refusals exactly as the CLI does with --json, and a non-zero exit
// becomes the studio error with the same code. It does not hold the write
// lock, so fn may wait on a host process, as `tutor ask` does.
func (w *Workspace) invoke(fn func(st *store.Store, opt Options) int) error {
	st, err := w.open()
	if err != nil {
		return err
	}
	defer st.Close()
	var stdout, stderr bytes.Buffer
	opt := w.opt
	opt.Stdout = &stdout
	opt.Stderr = &stderr
	if code := fn(st, opt); code != 0 {
		return commandResult{code: code, stdout: stdout.Bytes(), stderr: stderr.Bytes()}.err()
	}
	return nil
}

// noteEdit loads a draft as the window edits it and applies sanitised edits
// over the draft's own fields: an emptied field keeps the draft's own value,
// never an unused suggestion.
func noteEdit(st *store.Store, draftID int64, fields studio.NoteFields) (noteWindow, studio.NoteFields, error) {
	draft, err := noteDraftFor(st, draftID)
	if err != nil {
		return noteWindow{}, studio.NoteFields{}, noteStudioError(err)
	}
	effective, err := noteEffectiveFor(st, draft)
	if err != nil {
		return noteWindow{}, studio.NoteFields{}, noteStudioError(err)
	}
	if len(effective.Built.Withheld) > 0 {
		return noteWindow{}, studio.NoteFields{}, refuse("consent_required", "%s", noteConsentMessage(draft))
	}
	view := noteWindowOf(effective)
	edited := noteFieldsOf(view.base)
	if strings.TrimSpace(fields.Front) != "" {
		if edited.Front, err = note.SanitizeFront(fields.Front); err != nil {
			return noteWindow{}, studio.NoteFields{}, refuse("invalid_front", "%s", err.Error())
		}
	}
	if strings.TrimSpace(fields.Back) != "" {
		if edited.Back, err = note.SanitizeBack(fields.Back); err != nil {
			return noteWindow{}, studio.NoteFields{}, refuse("invalid_back", "%s", err.Error())
		}
	}
	if strings.TrimSpace(fields.Tactic) != "" {
		if edited.Tactic, err = note.SanitizeTactic(fields.Tactic); err != nil {
			return noteWindow{}, studio.NoteFields{}, refuse("invalid_tactic", "%s", err.Error())
		}
	}
	return view, edited, nil
}

func noteStudioError(err error) error {
	if code, message, ok := noteErrorCode(err); ok {
		return refuse(code, "%s", message)
	}
	return err
}

// AcceptNote records the learner's fields as `note accept` records them. The
// origin follows what the learner accepts: the draft's own fields keep its
// origin, the unused suggestion's own fields are a refined note, and anything
// else is an edited one.
func (w *Workspace) AcceptNote(draftID int64, fields studio.NoteFields) (studio.NoteAccept, error) {
	var doc noteAcceptDoc
	err := w.withStore(func(st *store.Store) error {
		view, edited, err := noteEdit(st, draftID, fields)
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
		doc, err = noteAcceptRecord(st, recorded, edited.Front, edited.Back, edited.Tactic, origin, w.opt.Now())
		return err
	})
	if err != nil {
		return studio.NoteAccept{}, err
	}
	return studio.NoteAccept{
		State:            doc.State,
		Note:             studioNote(doc.Note),
		Created:          doc.Created,
		Changed:          doc.Changed,
		PreviousRevision: doc.PreviousRevision,
	}, nil
}

// SkipNote is `note skip`: the draft is closed and no note is written.
func (w *Workspace) SkipNote(draftID int64) error {
	w.mu.Lock()
	result := w.run("note", "skip", "--draft", strconv.FormatInt(draftID, 10))
	w.mu.Unlock()
	var doc noteSkipDoc
	return result.decode(&doc)
}

// Notes lists every local note with its Anki state.
func (w *Workspace) Notes() (studio.NoteList, error) {
	var doc noteShowDoc
	if err := w.run("note", "show").decode(&doc); err != nil {
		return studio.NoteList{}, err
	}
	list := studio.NoteList{Notes: []studio.Note{}, Limits: doc.Limits}
	for _, item := range doc.Notes {
		list.Notes = append(list.Notes, studioNote(item.Note))
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
	args := []string{"note", "push"}
	if force {
		args = append(args, "--force")
	}
	result := w.run(args...)
	raw := result.stdout
	if result.code != 0 {
		raw = result.stderr
	}
	var doc notePushDoc
	if json.Unmarshal(raw, &doc) != nil || doc.State == "" {
		return studio.NotePush{}, result.err()
	}
	push := studio.NotePush{
		State:           doc.State,
		Endpoint:        doc.Endpoint,
		Deck:            doc.Deck,
		Model:           doc.Model,
		Notes:           []studio.NotePushItem{},
		Limits:          doc.Limits,
		ModelUpdated:    doc.ModelUpdated,
		ModelCustomized: doc.ModelCustomized,
	}
	if err := convertJSON(doc.Counts, &push.Counts); err != nil {
		return studio.NotePush{}, workspaceError(err)
	}
	if err := convertJSON(doc.Notes, &push.Notes); err != nil {
		return studio.NotePush{}, workspaceError(err)
	}
	if doc.Error != nil {
		push.Error = withAction(&studio.Error{Code: doc.Error.Code, Message: doc.Error.Message})
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
