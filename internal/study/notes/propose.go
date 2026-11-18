package notes

import (
	"context"

	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
)

// ProposeInput is one `note propose` call: the attempt or the reviewed
// question the draft comes from, the learner's mark, and whether the draft is
// refined once through the configured provider.
type ProposeInput struct {
	AttemptID  int64
	ContentID  string
	QuestionID string
	Mark       string
	Refine     bool
}

// Propose derives and saves one deterministic draft and, with Refine, performs
// the one explicit refinement. A refinement's provider failure is a result
// (Refinement.Failure), not an error.
func Propose(ctx context.Context, s *study.Service, in ProposeInput) (Effective, *Refinement, error) {
	mark := note.NormalizeMark(in.Mark)
	input, err := proposeInput(s, in.AttemptID, in.ContentID, in.QuestionID, mark)
	if err != nil {
		return Effective{}, nil, classify(err)
	}
	built, err := note.Build(input)
	if err != nil {
		return Effective{}, nil, classify(err)
	}
	target := input.Target
	target.Mark = mark
	saved, err := kernel.Store(s).SaveNoteDraft(store.NoteDraft{
		TargetKey:      target.Key(),
		AttemptID:      target.AttemptID,
		ContentID:      target.ContentID,
		ContentVersion: target.ContentVersion,
		QuestionID:     target.QuestionID,
		SourceID:       target.SourceID,
		Page:           target.Page,
		Mark:           mark,
		Restricted:     target.Restricted,
		Origin:         string(note.OriginDeterministic),
	}, kernel.P(s).Now())
	if err != nil {
		return Effective{}, nil, err
	}
	if !in.Refine {
		return EffectiveOf(saved, input, built), nil, nil
	}
	if len(built.Withheld) > 0 {
		return Effective{}, nil, kernel.Refuse("consent_required", "%s", ConsentMessage(saved))
	}
	refined, record, failure, err := refine(ctx, s, saved, input, built)
	if err != nil {
		return Effective{}, nil, err
	}
	if failure != nil {
		return Effective{}, &Refinement{Draft: refined, Built: built, Failure: failure}, nil
	}
	return EffectiveOf(refined, input, built), &Refinement{Draft: refined, Built: built, Record: record}, nil
}

// proposeInput is the derivation input for one proposal, from a reviewed
// question or a recorded attempt.
func proposeInput(s *study.Service, attemptFlag int64, contentFlag, questionFlag, mark string) (note.Input, error) {
	if contentFlag != "" {
		loaded, err := loadContent(s, contentFlag, questionFlag)
		if err != nil {
			return note.Input{}, err
		}
		if !loaded.Found {
			return note.Input{}, kernel.Refuse("not_practiceable",
				"question %s of %s is not a reviewed, practiceable question", questionFlag, contentFlag)
		}
		sourceID, page, _ := source.ParseContentID(contentFlag)
		return note.Input{
			Target: note.Target{
				ContentID:      contentFlag,
				ContentVersion: loaded.Content.Version,
				QuestionID:     questionFlag,
				SourceID:       sourceID,
				Page:           page,
				Mark:           mark,
				Restricted:     loaded.Private,
			},
			Question:       &loaded.Question,
			PassageText:    loaded.Content.Passage.Text,
			ConsentGranted: loaded.Private && loaded.SourceID != "" && consentGranted(s, loaded.SourceID),
		}, nil
	}

	record, err := attemptRecord(s, attemptFlag)
	if err != nil {
		return note.Input{}, err
	}
	loaded, err := loadContent(s, record.ContentID, record.QuestionID)
	if err != nil {
		return note.Input{}, err
	}
	if !loaded.Found {
		return note.Input{}, kernel.Refuse("content_unavailable",
			"the attempted question %s of %s is no longer available in this build", record.QuestionID, record.ContentID)
	}
	if loaded.Content.Version != record.ContentVersion {
		return note.Input{}, kernel.Refuse("content_changed",
			"the question changed since attempt #%d; answer the current question again or propose from it with --mark", record.ID)
	}
	sourceID, page, _ := source.ParseContentID(record.ContentID)
	return note.Input{
		Target: note.Target{
			ContentID:      record.ContentID,
			ContentVersion: loaded.Content.Version,
			QuestionID:     record.QuestionID,
			SourceID:       sourceID,
			Page:           page,
			AttemptID:      record.ID,
			Mark:           mark,
			Restricted:     loaded.Private,
		},
		Attempt:        &record,
		Question:       &loaded.Question,
		PassageText:    loaded.Content.Passage.Text,
		ConsentGranted: loaded.Private && loaded.SourceID != "" && consentGranted(s, loaded.SourceID),
	}, nil
}
