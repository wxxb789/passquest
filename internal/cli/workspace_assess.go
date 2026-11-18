package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/studio"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/assessment"
)

// withAssessmentStore is withStore plus the reconcile every assess
// subcommand runs first, so an expired window is recorded before it is read.
func (w *Workspace) withAssessmentStore(fn func(st *store.Store) error) error {
	return w.withStore(func(st *store.Store) error {
		if _, err := w.reconcileAssessments(st, func(string, int) {}); err != nil {
			return err
		}
		return fn(st)
	})
}

// reconcileAssessments closes an active run whose whole window has passed
// under the CLI loop's rule (assessment.Timeout): the item on screen is
// recorded as a timed-out attempt at its deadline before the run is abandoned,
// so it stays in the timeout count and the score denominator. The store-level
// reconcile then catches anything else. It reports the active run it closed,
// or zero.
func (w *Workspace) reconcileAssessments(st *store.Store, notify assessment.Notify) (int64, error) {
	var closed int64
	svc := study.Wrap(st, w.ports)
	active, found, err := st.ActiveAssessmentRun()
	if err != nil {
		return 0, err
	}
	if found {
		stepped, err := assessment.Step(context.Background(), svc, active.ID, assessment.Timeout, 0, notify)
		if err != nil {
			return 0, err
		}
		if stepped.Kind == assessment.Abandoned {
			closed = active.ID
		}
	}
	if err := assessment.Reconcile(context.Background(), svc); err != nil {
		return 0, err
	}
	return closed, nil
}

// AssessmentPool reports the unseen-assessment pool as `assess pool` does.
func (w *Workspace) AssessmentPool() (studio.AssessmentPool, error) {
	var view studio.AssessmentPool
	err := w.withAssessmentStore(func(st *store.Store) error {
		reported, err := assessment.Report(context.Background(), study.Wrap(st, w.ports), assessment.ReportQuery{Query: assessment.QueryPool})
		if err != nil {
			return err
		}
		return convertJSON(poolDocOf(reported.Pool, reported.Identities), &view)
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
	if err := assessment.ValidateSettings(size, minutes, assess.AnnotationsNone); err != nil {
		return studio.AssessmentState{}, workspaceError(err)
	}
	var state studio.AssessmentState
	err := w.withAssessmentStore(func(st *store.Store) error {
		started, err := assessment.Begin(context.Background(), study.Wrap(st, w.ports), size, minutes)
		if err != nil {
			return err
		}
		state, err = w.advance(st, started.Run.ID)
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
		svc := study.Wrap(st, w.ports)
		for _, item := range items {
			if item.State != assess.StatePresented {
				continue
			}
			if content, question, ok := assessment.Question(svc, item); ok {
				state = assessmentStateOf(w.opt.Now(), run, items,
					&assessment.Presented{Item: item, Content: content, Question: question}, nil)
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
		state, err = w.advance(st, runID)
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
	err := w.withStore(func(st *store.Store) error {
		var notices []string
		closed, err := w.reconcileAssessments(st, collectNotices(&notices))
		if err != nil {
			return err
		}
		svc := study.Wrap(st, w.ports)
		run, items, err := st.AssessmentRunItems(runID)
		if err != nil {
			return err
		}
		if closed == runID {
			answer.Position = position
			answer.Outcome = assess.StatusAbandoned
			answer.State = assessmentStateOf(w.opt.Now(), run, items, nil, notices)
			answer.State.Message = assessment.AbandonedMessage(run)
			return nil
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
		_, question, ok := assessment.Question(svc, item)
		if !ok {
			return refuse("content_unavailable", "item %d of run %d is no longer available; present the run again", position+1, run.ID)
		}
		stepped, err := assessment.Submit(context.Background(), svc, run, assessment.Presented{Item: item, Question: question}, label, collectNotices(&notices))
		if err != nil {
			return err
		}
		answer.Position = position
		switch stepped.Outcome {
		case assessment.AnswerInvalid:
			return refuse("invalid_answer", "item %d: %v; the run remains active", position+1, stepped.ParseErr)
		case assessment.AnswerAbandoned:
			answer.Outcome = assess.StatusAbandoned
		case assessment.AnswerTimedOut:
			answer.Outcome = assess.StateTimedOut
		default:
			answer.Outcome = assess.StateAnswered
		}
		run, items, err = st.AssessmentRunItems(run.ID)
		if err != nil {
			return err
		}
		answer.State = assessmentStateOf(w.opt.Now(), run, items, nil, notices)
		if stepped.Outcome == assessment.AnswerAbandoned {
			answer.State.Message = assessment.AbandonedMessage(run)
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
		stepped, err := assessment.Step(context.Background(), study.Wrap(st, w.ports), runID, assessment.Resume, 0, nil)
		if err != nil {
			return err
		}
		if stepped.Result != nil {
			state = assessmentStateOf(w.opt.Now(), stepped.Result.Run, stepped.Result.Items, nil, nil)
			return nil
		}
		state, err = w.advance(st, runID)
		return err
	})
	return state, err
}

// AbandonAssessment records the learner's abandonment; answered items stay
// readable in the report.
func (w *Workspace) AbandonAssessment(runID int64) (studio.AssessmentState, error) {
	var state studio.AssessmentState
	err := w.withAssessmentStore(func(st *store.Store) error {
		if _, err := assessment.Step(context.Background(), study.Wrap(st, w.ports), runID, assessment.Abandon, 0, nil); err != nil {
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
		reported, err := assessment.Report(context.Background(), study.Wrap(st, w.ports),
			assessment.ReportQuery{Query: assessment.QueryReport, RunID: runID})
		if err != nil {
			return assessmentStudioError(err)
		}
		if err := convertJSON(reportDocOf(reported.Report), &view); err != nil {
			return err
		}
		view.PassThreshold = studio.PassThresholdUnknown
		return attachReportQuestions(st, runID, reported.Units, &view)
	})
	return view, err
}

// attachReportQuestions adds each item's question, the learner's choice and
// the verified key to the report items whose annotations are open (after the
// run), so the window can show them beside the annotations. Only an item the
// learner was shown and that reached an outcome (answered or timed out) gets
// one: a pending or unavailable item may be selected again as unseen. An item
// whose stored content version no longer matches gets none, exactly as its
// annotations are unavailable.
func attachReportQuestions(st *store.Store, runID int64, units []assess.Unit, view *studio.AssessmentReport) error {
	_, items, _, err := st.AssessmentRun(runID)
	if err != nil {
		return err
	}
	attempts, err := st.AssessmentAttempts(runID)
	if err != nil {
		return err
	}
	byContent := make(map[string]assess.Unit, len(units))
	for _, unit := range units {
		byContent[unit.ContentID] = unit
	}
	byPosition := make(map[int]assess.Item, len(items))
	for _, item := range items {
		byPosition[item.Position] = item
	}
	for i := range view.Items {
		entry := &view.Items[i]
		if entry.Annotations.Withheld || !entry.Annotations.Available {
			continue
		}
		item, ok := byPosition[entry.Position]
		if !ok || !item.Finalized() || item.PresentedAt.IsZero() || entry.State == assess.StateUnavailable {
			continue
		}
		unit, found := byContent[item.ContentID]
		if !found || unit.Version != item.ContentVersion {
			continue
		}
		for _, question := range unit.Questions {
			if question.ID != item.QuestionID {
				continue
			}
			selected := ""
			for _, record := range attempts {
				if record.ID == item.AttemptID {
					selected = record.SelectedLabel
				}
			}
			entry.Question = &studio.AssessmentItemQuestion{
				Stem:     question.Stem,
				Options:  studioOptions(question.Options),
				Selected: selected,
				Key:      question.Answer,
				Skills:   append([]string{}, item.Skills...),
			}
			break
		}
	}
	return nil
}

// AssessmentCompare is `assess compare`: like-for-like runs only.
func (w *Workspace) AssessmentCompare(first, second int64) (studio.AssessmentComparison, error) {
	var view studio.AssessmentComparison
	err := w.withAssessmentStore(func(st *store.Store) error {
		reported, err := assessment.Report(context.Background(), study.Wrap(st, w.ports),
			assessment.ReportQuery{Query: assessment.QueryCompare, First: first, Second: second})
		if err != nil {
			return assessmentStudioError(err)
		}
		if err := convertJSON(comparisonDocOf(reported.First, reported.Second), &view); err != nil {
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
		reported, err := assessment.Report(context.Background(), study.Wrap(st, w.ports), assessment.ReportQuery{Query: assessment.QueryRuns})
		if err != nil {
			return err
		}
		for i := len(reported.Runs) - 1; i >= 0; i-- {
			views = append(views, studioRun(reported.Runs[i]))
		}
		return nil
	})
	return views, err
}

// advance runs the CLI loop's step rules and renders the result.
func (w *Workspace) advance(st *store.Store, runID int64) (studio.AssessmentState, error) {
	var notices []string
	stepped, err := assessment.Step(context.Background(), study.Wrap(st, w.ports), runID, assessment.Present, 0, collectNotices(&notices))
	if err != nil {
		return studio.AssessmentState{}, err
	}
	run, current, err := st.AssessmentRunItems(runID)
	if err != nil {
		return studio.AssessmentState{}, err
	}
	if stepped.Kind != assessment.Ready {
		state := assessmentStateOf(w.opt.Now(), run, current, nil, notices)
		if stepped.Kind == assessment.Abandoned {
			state.Message = assessment.AbandonedMessage(run)
		}
		return state, nil
	}
	return assessmentStateOf(w.opt.Now(), run, current, stepped.Presented, notices), nil
}

func collectNotices(notices *[]string) assessment.Notify {
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
func assessmentStateOf(now time.Time, run assess.Run, items []assess.Item, presented *assessment.Presented, notices []string) studio.AssessmentState {
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
			Current:  presented != nil && presented.Item.Position == item.Position,
		})
	}
	if presented == nil || run.Status != assess.StatusActive {
		return state
	}
	item := presented.Item
	content := presented.Content
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
		Stem:          presented.Question.Stem,
		Options:       studioOptions(presented.Question.Options),
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

// assessmentStudioError maps a run read's error the way `assess report` and
// `assess compare` always have: a study refusal — or an assessment sentinel
// the module already classified (unknown_run, run_state, run_abandoned,
// incomparable_runs) — becomes the studio error with its code, and anything
// else keeps the direct-path mapping. This is the one place the window still
// honours the assessment area's own codes instead of the desktop's sentinel
// table.
func assessmentStudioError(err error) error {
	if err == nil {
		return nil
	}
	var e *study.Error
	if errors.As(err, &e) {
		return withAction(&studio.Error{Code: e.Code, Message: e.Message})
	}
	return workspaceError(err)
}
