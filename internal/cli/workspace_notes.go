package cli

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/wxxb789/passquest/internal/note"
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
	draft := studio.NoteDraft{
		ID:             doc.Draft.ID,
		Origin:         doc.Draft.Origin,
		Restricted:     doc.Draft.Restricted,
		ConsentGranted: doc.Draft.ConsentGranted,
		Withheld:       append([]string{}, doc.Draft.Withheld...),
		Unanswered:     doc.Draft.Unanswered,
		TacticID:       doc.Draft.TacticID,
		Fields:         studio.NoteFields{Front: doc.Plain.Front, Back: doc.Plain.Back, Tactic: doc.Plain.Tactic},
		Source:         doc.Plain.Source,
		Advisory:       doc.Advisory,
		Limits:         doc.Limits,
	}
	if err := convertJSON(doc.Draft.Target, &draft.Target); err != nil {
		return studio.NoteDraft{}, workspaceError(err)
	}
	return draft, nil
}

// PreviewNote renders the draft with the real note model. Non-empty fields
// are the learner's edits, sanitised exactly as `note accept` sanitises them;
// empty fields keep the draft's values.
func (w *Workspace) PreviewNote(draftID int64, fields studio.NoteFields) (studio.NotePreview, error) {
	var preview studio.NotePreview
	err := w.withStore(func(st *store.Store) error {
		effective, edited, err := noteEdit(st, draftID, fields)
		if err != nil {
			return err
		}
		rendered := note.HTML(edited.Front, edited.Back, edited.Tactic, effective.Built.SourceLabel)
		preview = studio.NotePreview{
			DraftID:    effective.Draft.ID,
			Fields:     edited,
			Source:     effective.Built.SourceLabel,
			HTML:       note.RenderCard(rendered),
			CSS:        note.ModelCSS,
			HTMLSHA256: rendered.Hash(),
			Deck:       note.Deck,
			Model:      note.ModelName,
			Edited:     edited != studio.NoteFields{Front: effective.Front, Back: effective.Back, Tactic: effective.Tactic},
		}
		return nil
	})
	return preview, err
}

// noteEdit loads a draft's effective fields and applies sanitised edits.
func noteEdit(st *store.Store, draftID int64, fields studio.NoteFields) (noteEffective, studio.NoteFields, error) {
	draft, err := noteDraftFor(st, draftID)
	if err != nil {
		return noteEffective{}, studio.NoteFields{}, noteStudioError(err)
	}
	effective, err := noteEffectiveFor(st, draft)
	if err != nil {
		return noteEffective{}, studio.NoteFields{}, noteStudioError(err)
	}
	if len(effective.Built.Withheld) > 0 {
		return noteEffective{}, studio.NoteFields{}, refuse("consent_required", "%s", noteConsentMessage(draft))
	}
	edited := studio.NoteFields{Front: effective.Front, Back: effective.Back, Tactic: effective.Tactic}
	if strings.TrimSpace(fields.Front) != "" {
		if edited.Front, err = note.SanitizeFront(fields.Front); err != nil {
			return noteEffective{}, studio.NoteFields{}, refuse("invalid_front", "%s", err.Error())
		}
	}
	if strings.TrimSpace(fields.Back) != "" {
		if edited.Back, err = note.SanitizeBack(fields.Back); err != nil {
			return noteEffective{}, studio.NoteFields{}, refuse("invalid_back", "%s", err.Error())
		}
	}
	if strings.TrimSpace(fields.Tactic) != "" {
		if edited.Tactic, err = note.SanitizeTactic(fields.Tactic); err != nil {
			return noteEffective{}, studio.NoteFields{}, refuse("invalid_tactic", "%s", err.Error())
		}
	}
	return effective, edited, nil
}

func noteStudioError(err error) error {
	if code, message, ok := noteErrorCode(err); ok {
		return refuse(code, "%s", message)
	}
	return err
}

// AcceptNote is `note accept`: the changed fields are passed as edits, so an
// unchanged draft keeps its origin and an edited one is recorded as edited.
func (w *Workspace) AcceptNote(draftID int64, fields studio.NoteFields) (studio.NoteAccept, error) {
	var args []string
	err := w.withStore(func(st *store.Store) error {
		effective, edited, err := noteEdit(st, draftID, fields)
		if err != nil {
			return err
		}
		args = []string{"note", "accept", "--draft", strconv.FormatInt(effective.Draft.ID, 10)}
		if edited.Front != effective.Front {
			args = append(args, "--front="+edited.Front)
		}
		if edited.Back != effective.Back {
			args = append(args, "--back="+edited.Back)
		}
		if edited.Tactic != effective.Tactic {
			args = append(args, "--tactic="+edited.Tactic)
		}
		return nil
	})
	if err != nil {
		return studio.NoteAccept{}, err
	}
	w.mu.Lock()
	result := w.run(args...)
	w.mu.Unlock()
	var doc noteAcceptDoc
	if err := result.decode(&doc); err != nil {
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
		State:    doc.State,
		Endpoint: doc.Endpoint,
		Deck:     doc.Deck,
		Model:    doc.Model,
		Notes:    []studio.NotePushItem{},
		Limits:   doc.Limits,
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
