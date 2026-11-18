package notes

import (
	"context"
	"errors"

	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
)

// Listed is one local note with its revision history, in today's order.
type Listed struct {
	Note      store.Note
	Revisions []store.NoteRevision
}

// List is `note show` minus rendering: the named note when noteID is set, or
// every local note, each with its revisions. Two adapters: runNoteShow and the
// window's Notes, which reads the list and counts Anki state separately.
func List(ctx context.Context, s *study.Service, noteID string) ([]Listed, error) {
	_ = ctx
	st := kernel.Store(s)
	var notes []store.Note
	if noteID != "" {
		n, err := st.Note(noteID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, kernel.Refuse("unknown_note", "note %q is not recorded", noteID)
		}
		if err != nil {
			return nil, err
		}
		notes = append(notes, n)
	} else {
		var err error
		notes, err = st.Notes()
		if err != nil {
			return nil, err
		}
	}
	listed := make([]Listed, 0, len(notes))
	for _, n := range notes {
		revisions, err := st.NoteRevisions(n.NoteID)
		if err != nil {
			return nil, err
		}
		listed = append(listed, Listed{Note: n, Revisions: revisions})
	}
	return listed, nil
}
