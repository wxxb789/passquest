package assessment

import (
	"errors"
	"strings"

	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
)

// present applies the run rules from items[position] on and stops at the first
// item that awaits an answer, presenting it (which starts its item clock) when
// it is still pending. On the way it records unavailable content, times out an
// item whose clock already ran out, and abandons the run when its whole window
// has passed. Re-presenting an item keeps its original clock, so calling it
// again for the same item is safe. A run with nothing left is completed.
func present(s *study.Service, runID int64, position int, notify Notify) (Stepped, error) {
	run, items, err := kernel.Store(s).AssessmentRunItems(runID)
	if err != nil {
		return Stepped{}, err
	}
	kind, presented, err := advance(s, run, items, position, notify)
	if err != nil {
		return Stepped{}, err
	}
	switch kind {
	case Abandoned:
		return Stepped{Kind: Abandoned}, nil
	case Done:
		result, err := complete(s, run.ID)
		if err != nil {
			return Stepped{}, err
		}
		return Stepped{Kind: Done, Result: &result}, nil
	}
	return Stepped{Kind: Ready, Presented: presented}, nil
}

// timeout closes a run whose whole window has passed under the loop's rule: the
// item on screen is recorded as a timed-out attempt at its deadline before the
// run is abandoned, so it stays in the timeout count and the score denominator.
// A run whose window has not passed is left unchanged.
func timeout(s *study.Service, runID int64, notify Notify) (Stepped, error) {
	run, items, err := kernel.Store(s).AssessmentRunItems(runID)
	if err != nil {
		return Stepped{}, err
	}
	if kernel.P(s).Now().Before(run.DeadlineAt) {
		return Stepped{}, nil
	}
	var onScreen assess.Item
	for _, item := range items {
		if item.State == assess.StatePresented {
			onScreen = item
			break
		}
	}
	if err := abandonElapsed(s, run, onScreen, notify); err != nil {
		return Stepped{}, err
	}
	return Stepped{Kind: Abandoned}, nil
}

// resume counts one resume of an active run and returns it with its items. A
// completed run returns its result read back instead; an abandoned or expired
// run is refused with run_abandoned.
func resume(s *study.Service, runID int64) (Stepped, error) {
	st := kernel.Store(s)
	run, items, err := st.AssessmentRunItems(runID)
	if err != nil {
		return Stepped{}, err
	}
	if run.Status == assess.StatusAbandoned {
		return Stepped{}, kernel.Refuse("run_abandoned",
			"assessment run %d was abandoned (%s); its answered items remain readable with `passquest assess report %d`",
			run.ID, run.AbandonedReason, run.ID)
	}
	if run.Status == assess.StatusCompleted {
		result, err := readBack(s, run.ID)
		if err != nil {
			return Stepped{}, err
		}
		return Stepped{Run: run, Items: items, Result: &result}, nil
	}
	run, err = st.ResumeAssessmentRun(run.ID, kernel.P(s).Now())
	if errors.Is(err, store.ErrAssessmentWindowElapsed) {
		return Stepped{}, kernel.Refuse("run_abandoned",
			"assessment run %d passed its deadline and was recorded abandoned; resume is refused", run.ID)
	}
	if err != nil {
		return Stepped{}, err
	}
	return Stepped{Run: run, Items: items}, nil
}

// abandonRun records the learner's abandonment of a run. A completed run is
// refused with run_completed.
func abandonRun(s *study.Service, runID int64) (assess.Run, error) {
	run, err := kernel.Store(s).AbandonAssessmentRun(runID, kernel.P(s).Now(), "learner_abandoned")
	if errors.Is(err, store.ErrAssessmentRunState) {
		return assess.Run{}, kernel.Refuse("run_completed",
			"assessment run %d is already completed; abandoning it would rewrite recorded evidence", runID)
	}
	return run, err
}

// advance is today's run rule: read one item at a time, record what cannot be
// answered, and stop where the caller's prompt takes over.
func advance(s *study.Service, run assess.Run, items []assess.Item, start int, notify Notify) (Kind, *Presented, error) {
	st := kernel.Store(s)
	for index := start; index < len(items); index++ {
		item := items[index]
		if item.Finalized() || item.State == assess.StateUnavailable {
			continue
		}
		now := kernel.P(s).Now()
		if !now.Before(run.DeadlineAt) {
			return Abandoned, nil, abandonElapsed(s, run, item, notify)
		}
		content, question, ok := Question(s, item)
		if !ok {
			if err := st.MarkAssessmentUnavailable(run.ID, item.Position, now, "content is no longer practiceable at the recorded version"); err != nil {
				return 0, nil, err
			}
			notify(assess.StateUnavailable, item.Position)
			continue
		}
		if item.State == assess.StatePending {
			presented, err := st.MarkAssessmentPresented(run.ID, item.Position, now)
			if errors.Is(err, store.ErrAssessmentWindowElapsed) {
				return Abandoned, nil, abandonElapsed(s, run, item, notify)
			}
			if err != nil {
				return 0, nil, err
			}
			item = presented
		}
		if !now.Before(item.DeadlineAt) {
			if err := appendTimeout(s, run, item, question); err != nil {
				return 0, nil, err
			}
			notify(assess.StateTimedOut, item.Position)
			continue
		}
		return Ready, &Presented{Index: index, Item: item, Content: content, Question: question}, nil
	}
	return Done, nil, nil
}

// submit applies one label to a presented item.
func submit(s *study.Service, run assess.Run, item assess.Item, question exam.Question, raw string, notify Notify) (Outcome, error, error) {
	line := raw
	if strings.EqualFold(strings.TrimSpace(line), "skip") {
		line = "skip"
	}
	parsed, parseErr := attempt.ParseSelection(question, strings.TrimSpace(line))
	now := kernel.P(s).Now()
	if !now.Before(run.DeadlineAt) {
		return AnswerAbandoned, nil, abandonElapsed(s, run, item, notify)
	}
	if !now.Before(item.DeadlineAt) {
		if err := appendTimeout(s, run, item, question); err != nil {
			return 0, nil, err
		}
		notify(assess.StateTimedOut, item.Position)
		return AnswerTimedOut, nil, nil
	}
	if parseErr != nil {
		return AnswerInvalid, parseErr, nil
	}
	record, err := attempt.BuildRecord(item.ContentID, item.ContentVersion, question, parsed, kernel.P(s).NewSession(), item.PresentedAt, now)
	if err != nil {
		return 0, nil, err
	}
	record, err = record.WithAssessment(run.ID)
	if err != nil {
		return 0, nil, err
	}
	if _, err := kernel.Store(s).AppendAssessmentAttempt(run.ID, item.Position, record, false, now); err != nil {
		return 0, nil, err
	}
	return AnswerRecorded, nil, nil
}

// complete marks a run with no open item completed and reads it back. A run
// that is no longer active is read back unchanged.
func complete(s *study.Service, runID int64) (Result, error) {
	if _, err := kernel.Store(s).CompleteAssessmentRun(runID, kernel.P(s).Now()); err != nil && !errors.Is(err, store.ErrAssessmentRunState) {
		return Result{}, err
	}
	return readBack(s, runID)
}

// readBack reads one run with its items and recorded attempts.
func readBack(s *study.Service, runID int64) (Result, error) {
	st := kernel.Store(s)
	run, items, _, err := st.AssessmentRun(runID)
	if err != nil {
		return Result{}, err
	}
	attempts, err := st.AssessmentAttempts(runID)
	if err != nil {
		return Result{}, err
	}
	return Result{Run: run, Items: items, Attempts: attempts}, nil
}

// appendTimeout records a timed-out skipped attempt at the item deadline,
// capped at the run deadline.
func appendTimeout(s *study.Service, run assess.Run, item assess.Item, question exam.Question) error {
	deadline := item.DeadlineAt
	if deadline.IsZero() || deadline.After(run.DeadlineAt) {
		deadline = run.DeadlineAt
	}
	record, err := attempt.BuildRecord(item.ContentID, item.ContentVersion, question,
		attempt.Selection{Skip: true}, kernel.P(s).NewSession(), item.PresentedAt, deadline)
	if err != nil {
		return err
	}
	record, err = record.WithAssessment(run.ID)
	if err != nil {
		return err
	}
	_, err = kernel.Store(s).AppendAssessmentAttempt(run.ID, item.Position, record, true, deadline)
	return err
}

// abandonElapsed abandons a run whose window has passed. An item that is on
// screen has no recorded outcome yet; recording it as a timed-out attempt at
// its stored deadline keeps it in the timeout count and the score denominator
// instead of leaving it presented forever. A finalized item is not re-recorded.
func abandonElapsed(s *study.Service, run assess.Run, item assess.Item, notify Notify) error {
	if item.State == assess.StatePresented {
		if _, question, ok := Question(s, item); ok {
			if err := appendTimeout(s, run, item, question); err != nil {
				return err
			}
			notify(assess.StateTimedOut, item.Position)
		}
	}
	if _, err := kernel.Store(s).AbandonAssessmentRun(run.ID, kernel.P(s).Now(), "window_elapsed"); err != nil && !errors.Is(err, store.ErrAssessmentRunState) {
		return err
	}
	return nil
}
