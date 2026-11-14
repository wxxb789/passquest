package cli

import (
	"fmt"
	"time"

	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/studio"
)

// withAssessmentStore is withStore plus the reconcile every assess
// subcommand runs first, so an expired window is recorded before it is read.
func (w *Workspace) withAssessmentStore(fn func(st *store.Store) error) error {
	return w.withStore(func(st *store.Store) error {
		if _, err := st.ReconcileAssessmentRuns(w.opt.Now()); err != nil {
			return err
		}
		return fn(st)
	})
}

// AssessmentPool reports the unseen-assessment pool as `assess pool` does.
func (w *Workspace) AssessmentPool() (studio.AssessmentPool, error) {
	var view studio.AssessmentPool
	err := w.withAssessmentStore(func(st *store.Store) error {
		pool, identities, err := assessmentPool(st)
		if err != nil {
			return err
		}
		return convertJSON(poolDocOf(pool, identities), &view)
	})
	return view, err
}

// StartAssessment creates a timed, unassisted run with the CLI's rules (zero
// selects the CLI defaults) and presents its first item, which starts that
// item's clock.
func (w *Workspace) StartAssessment(size, minutes int) (studio.AssessmentState, error) {
	if size == 0 {
		size = assess.DefaultSize
	}
	if minutes == 0 {
		minutes = assess.DefaultMinutes
	}
	if err := assessmentSettingsError(size, minutes, assess.AnnotationsNone); err != nil {
		return studio.AssessmentState{}, workspaceError(err)
	}
	var state studio.AssessmentState
	err := w.withAssessmentStore(func(st *store.Store) error {
		run, items, err := createAssessmentRun(w.opt, st, size, minutes)
		if err != nil {
			return err
		}
		state, err = w.advance(st, run, items)
		return err
	})
	return state, err
}

// AssessmentState reads the active run, its navigator and the item on screen
// without presenting anything new. State is none when no run is active.
func (w *Workspace) AssessmentState() (studio.AssessmentState, error) {
	var state studio.AssessmentState
	err := w.withAssessmentStore(func(st *store.Store) error {
		run, found, err := st.ActiveAssessmentRun()
		if err != nil {
			return err
		}
		if !found {
			state = studio.AssessmentState{State: studio.AssessmentNoneState, Now: formatTime(w.opt.Now()),
				Navigator: []studio.NavigatorItem{}, Notices: []string{}}
			return nil
		}
		run, items, err := st.AssessmentRunItems(run.ID)
		if err != nil {
			return err
		}
		state = assessmentStateOf(w.opt.Now(), run, items, nil, nil)
		for _, item := range items {
			if item.State != assess.StatePresented {
				continue
			}
			if content, question, ok := assessmentQuestion(st, item); ok {
				state = assessmentStateOf(w.opt.Now(), run, items, &assessmentStep{kind: stepReady, item: item, content: content, question: question}, nil)
			}
			break
		}
		return nil
	})
	return state, err
}

// PresentAssessmentItem advances run to its next unanswered item under the
// CLI loop's rules and presents it. Calling it again for the item on screen
// keeps its original clock. A run with nothing left is completed.
func (w *Workspace) PresentAssessmentItem(runID int64) (studio.AssessmentState, error) {
	var state studio.AssessmentState
	err := w.withAssessmentStore(func(st *store.Store) error {
		run, items, err := st.AssessmentRunItems(runID)
		if err != nil {
			return err
		}
		if run.Status != assess.StatusActive {
			state = assessmentStateOf(w.opt.Now(), run, items, nil, nil)
			return nil
		}
		state, err = w.advance(st, run, items)
		return err
	})
	return state, err
}

// AnswerAssessmentItem records label ("skip" skips) for the item at position,
// which must be the presented item. A late answer is recorded as a timeout and
// an answer after the run window abandons the run, as in the CLI. The outcome
// never says whether the answer was correct.
func (w *Workspace) AnswerAssessmentItem(runID int64, position int, label string) (studio.AssessmentAnswer, error) {
	var answer studio.AssessmentAnswer
	err := w.withAssessmentStore(func(st *store.Store) error {
		run, items, err := st.AssessmentRunItems(runID)
		if err != nil {
			return err
		}
		if run.Status != assess.StatusActive {
			return refuse("run_state", "assessment run %d is %s", run.ID, run.Status)
		}
		if position < 0 || position >= len(items) {
			return refuse("invalid_input", "assessment run %d has no item %d", run.ID, position+1)
		}
		item := items[position]
		if item.State != assess.StatePresented {
			return refuse("run_state", "item %d of run %d is %s, not presented", position+1, run.ID, item.State)
		}
		_, question, ok := assessmentQuestion(st, item)
		if !ok {
			return refuse("content_unavailable", "item %d of run %d is no longer available; present the run again", position+1, run.ID)
		}
		var notices []string
		outcome, parseErr, err := submitAssessmentAnswer(w.opt, st, run, item, question, label, collectNotices(&notices))
		if err != nil {
			return err
		}
		answer.Position = position
		switch outcome {
		case answerInvalid:
			return refuse("invalid_answer", "item %d: %v; the run remains active", position+1, parseErr)
		case answerAbandoned:
			answer.Outcome = assess.StatusAbandoned
		case answerTimedOut:
			answer.Outcome = assess.StateTimedOut
		default:
			answer.Outcome = assess.StateAnswered
		}
		run, items, err = st.AssessmentRunItems(run.ID)
		if err != nil {
			return err
		}
		answer.State = assessmentStateOf(w.opt.Now(), run, items, nil, notices)
		if outcome == answerAbandoned {
			answer.State.Message = abandonedRunMessage(run)
		}
		return nil
	})
	return answer, err
}

// ResumeAssessment counts one resume (resume_count+1) and presents the next
// item; the run clock kept running while the window was away.
func (w *Workspace) ResumeAssessment(runID int64) (studio.AssessmentState, error) {
	var state studio.AssessmentState
	err := w.withAssessmentStore(func(st *store.Store) error {
		run, items, completed, err := resumeAssessment(w.opt, st, runID)
		if err != nil {
			return err
		}
		if completed != nil {
			state = assessmentStateOf(w.opt.Now(), run, items, nil, nil)
			return nil
		}
		state, err = w.advance(st, run, items)
		return err
	})
	return state, err
}

// AbandonAssessment records the learner's abandonment; answered items stay
// readable in the report.
func (w *Workspace) AbandonAssessment(runID int64) (studio.AssessmentState, error) {
	var state studio.AssessmentState
	err := w.withAssessmentStore(func(st *store.Store) error {
		if _, err := abandonAssessment(w.opt, st, runID); err != nil {
			return err
		}
		run, items, err := st.AssessmentRunItems(runID)
		if err != nil {
			return err
		}
		state = assessmentStateOf(w.opt.Now(), run, items, nil, nil)
		return nil
	})
	return state, err
}

// AssessmentReport is `assess report` with the annotations withheld during
// the run; the pass threshold is unknown.
func (w *Workspace) AssessmentReport(runID int64) (studio.AssessmentReport, error) {
	var view studio.AssessmentReport
	err := w.withAssessmentStore(func(st *store.Store) error {
		report, err := assessmentReport(st, runID)
		if err != nil {
			return assessmentStudioError(err)
		}
		if err := convertJSON(reportDocOf(report), &view); err != nil {
			return err
		}
		view.PassThreshold = studio.PassThresholdUnknown
		return nil
	})
	return view, err
}

// AssessmentCompare is `assess compare`: like-for-like runs only.
func (w *Workspace) AssessmentCompare(first, second int64) (studio.AssessmentComparison, error) {
	var view studio.AssessmentComparison
	err := w.withAssessmentStore(func(st *store.Store) error {
		doc, err := compareAssessmentRuns(st, first, second)
		if err != nil {
			return assessmentStudioError(err)
		}
		if err := convertJSON(doc, &view); err != nil {
			return err
		}
		view.PassThreshold = studio.PassThresholdUnknown
		return nil
	})
	return view, err
}

// AssessmentRuns lists every run, newest first.
func (w *Workspace) AssessmentRuns() ([]studio.AssessmentRun, error) {
	views := []studio.AssessmentRun{}
	err := w.withAssessmentStore(func(st *store.Store) error {
		runs, err := st.AssessmentRuns()
		if err != nil {
			return err
		}
		for i := len(runs) - 1; i >= 0; i-- {
			views = append(views, studioRun(runs[i]))
		}
		return nil
	})
	return views, err
}

// advance runs the CLI loop's step rules and renders the result.
func (w *Workspace) advance(st *store.Store, run assess.Run, items []assess.Item) (studio.AssessmentState, error) {
	var notices []string
	step, err := advanceAssessment(w.opt, st, run, items, 0, collectNotices(&notices))
	if err != nil {
		return studio.AssessmentState{}, err
	}
	if step.kind == stepDone {
		if _, err := completeAssessment(w.opt, st, run); err != nil {
			return studio.AssessmentState{}, err
		}
	}
	run, current, err := st.AssessmentRunItems(run.ID)
	if err != nil {
		return studio.AssessmentState{}, err
	}
	if step.kind != stepReady {
		state := assessmentStateOf(w.opt.Now(), run, current, nil, notices)
		if step.kind == stepAbandoned {
			state.Message = abandonedRunMessage(run)
		}
		return state, nil
	}
	return assessmentStateOf(w.opt.Now(), run, current, &step, notices), nil
}

func collectNotices(notices *[]string) assessmentNotice {
	return func(kind string, position int) {
		switch kind {
		case assess.StateUnavailable:
			*notices = append(*notices, fmt.Sprintf("Item %d unavailable; it will be listed, not scored.", position+1))
		case assess.StateTimedOut:
			*notices = append(*notices, fmt.Sprintf("Item %d timed out.", position+1))
		}
	}
}

// assessmentStateOf renders a run's navigator (no correctness) and, when a
// step is ready, the presented item without glossary, evidence or reasoning.
func assessmentStateOf(now time.Time, run assess.Run, items []assess.Item, step *assessmentStep, notices []string) studio.AssessmentState {
	view := studioRun(run)
	state := studio.AssessmentState{
		State:     studio.AssessmentItemState,
		Now:       formatTime(now),
		Run:       &view,
		Navigator: make([]studio.NavigatorItem, 0, len(items)),
		Notices:   append([]string{}, notices...),
	}
	switch run.Status {
	case assess.StatusCompleted:
		state.State = studio.AssessmentCompletedState
	case assess.StatusAbandoned:
		state.State = studio.AssessmentAbandonedState
	}
	for _, item := range items {
		state.Navigator = append(state.Navigator, studio.NavigatorItem{
			Position: item.Position,
			State:    item.State,
			Current:  step != nil && step.item.Position == item.Position,
		})
	}
	if step == nil || run.Status != assess.StatusActive {
		return state
	}
	item := step.item
	content := step.content
	_, page, _ := source.ParseContentID(item.ContentID)
	state.Item = &studio.AssessmentItem{
		RunID:      run.ID,
		Position:   item.Position,
		Count:      len(items),
		ContentID:  item.ContentID,
		QuestionID: item.QuestionID,
		Passage: studio.Passage{
			Title: content.Passage.Title,
			Label: studio.SourceLabel{
				Kind:     studio.SourceImported,
				SourceID: item.SourceID,
				Page:     page,
				Reviewed: true,
				Title:    content.Title,
				Rights:   content.Provenance.Rights,
			},
			Paragraphs: exam.Paragraphs(content.Passage.Text),
		},
		Stem:          step.question.Stem,
		Options:       studioOptions(step.question.Options),
		PresentedAt:   formatTime(item.PresentedAt),
		DeadlineAt:    formatTime(item.DeadlineAt),
		RunDeadlineAt: formatTime(run.DeadlineAt),
	}
	return state
}

func studioRun(run assess.Run) studio.AssessmentRun {
	var view studio.AssessmentRun
	_ = convertJSON(runViewOf(run), &view)
	return view
}

// assessmentStudioError maps run lookups to the codes `assess report` and
// `assess compare` print (unknown_run, run_state, incomparable_runs).
func assessmentStudioError(err error) error {
	if code, ok := assessmentErrorCode(err); ok {
		return withAction(&studio.Error{Code: code, Message: err.Error()})
	}
	return err
}
