package notes

import (
	"context"

	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
)

// Action is what Settle does with one draft: `note accept` or `note skip`.
type Action uint8

const (
	Accept Action = iota + 1
	Skip
)

// SettleInput is one settle call. Skip locates the draft itself; Accept
// records the derivation the caller showed the learner with the exact fields
// and origin that caller resolved. Each adapter decides those fields, because
// `note accept`'s flag semantics and the window's edit semantics differ; the
// withheld-source refusal happens in the adapter too, before any edit is
// sanitised.
type SettleInput struct {
	DraftID int64
	Action  Action
	// Accept only.
	Effective Effective
	Front     string
	Back      string
	Tactic    string
	Origin    note.Origin
}

// Settled is one settled draft: the row, the state word the document carries,
// and for an accept the note that was written.
type Settled struct {
	Draft       store.NoteDraft
	State       string
	Note        store.Note
	Fields      note.Fields
	Front       string
	Back        string
	Tactic      string
	SourceLabel string
	Created     bool
	Changed     bool
	Previous    int
}

// Settle writes or closes one draft: today's noteAcceptRecord and the
// `note skip` write. The fields of an accept are recorded as the caller
// resolved them, and the timestamp is one clock read.
func Settle(ctx context.Context, s *study.Service, in SettleInput) (Settled, error) {
	_ = ctx
	st := kernel.Store(s)
	if in.Action == Skip {
		draft, err := locate(s, in.DraftID)
		if err != nil {
			return Settled{}, err
		}
		if err := st.SetNoteDraftStatus(draft.ID, store.NoteDraftSkipped); err != nil {
			return Settled{}, err
		}
		return Settled{Draft: draft, State: "skipped"}, nil
	}
	effective := in.Effective
	draft := effective.Draft
	target := store.NoteTarget{
		NoteID:         note.NoteID(draft.TargetKey),
		TargetKey:      draft.TargetKey,
		AttemptID:      draft.AttemptID,
		ContentID:      draft.ContentID,
		ContentVersion: effective.Input.Target.ContentVersion,
		QuestionID:     draft.QuestionID,
		SourceID:       draft.SourceID,
		Page:           draft.Page,
		Mark:           draft.Mark,
		Restricted:     effective.Built.Restricted,
	}
	content := store.NoteContent{
		Front:       in.Front,
		Back:        in.Back,
		Tactic:      in.Tactic,
		SourceLabel: effective.Built.SourceLabel,
		ContentHash: note.ContentHash(in.Front, in.Back, in.Tactic, effective.Built.SourceLabel, effective.Built.Restricted),
		Origin:      string(in.Origin),
		Model:       effective.Model,
	}
	result, err := st.AcceptDraftNote(draft.ID, target, content, kernel.P(s).Now())
	if err != nil {
		return Settled{}, err
	}
	fields := note.HTML(in.Front, in.Back, in.Tactic, effective.Built.SourceLabel)
	state := "accepted"
	if !result.Changed {
		state = "unchanged"
	}
	return Settled{
		Draft:       draft,
		State:       state,
		Note:        result.Note,
		Fields:      fields,
		Front:       in.Front,
		Back:        in.Back,
		Tactic:      in.Tactic,
		SourceLabel: effective.Built.SourceLabel,
		Created:     result.Created,
		Changed:     result.Changed,
		Previous:    result.Previous,
	}, nil
}
