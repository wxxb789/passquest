// Package assessment orchestrates the timed, unassisted assessment workflow:
// creating a run from the unseen pool, stepping through its items under the
// stored clocks, and reading the pool, one run's report, a like-for-like
// comparison and the run list. Results carry exactly the inputs the CLI's view
// builders and the window's studio mappers read; nothing here renders. The
// CLI's assess commands and the window's assess methods are the two adapters.
package assessment

import (
	"context"
	"fmt"

	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
)

// Notify reports an item finalized without an answer while a run advanced:
// assess.StateUnavailable or assess.StateTimedOut.
type Notify func(kind string, position int)

// Action selects what one Step call does.
type Action uint8

const (
	// Present advances from position to the first item that awaits an answer
	// and presents it; a missing-content item is recorded unavailable, an item
	// whose clock already ran out is recorded as a timeout, and a run with
	// nothing left is completed.
	Present Action = iota + 1
	// Timeout closes a run whose whole window has passed under the loop's
	// rule: the item on screen is recorded as a timed-out attempt at its
	// deadline and only then is the run abandoned.
	Timeout
	// Resume counts one resume of an active run.
	Resume
	// Abandon records the learner's abandonment.
	Abandon
)

// Kind is what one Step call reached. Its zero value means the call changed
// nothing (Timeout on a run whose window has not passed).
type Kind uint8

const (
	// Ready: an item awaits an answer; Presented is set.
	Ready Kind = iota + 1
	// Done: the run had no open item left and was completed; Result is set.
	Done
	// Abandoned: the run's window passed and it was abandoned (Present,
	// Timeout).
	Abandoned
)

// Outcome is what one submitted answer produced (Submit).
type Outcome uint8

const (
	// AnswerRecorded: the answer was stored as the item's attempt.
	AnswerRecorded Outcome = iota + 1
	// AnswerTimedOut: the answer came after the item clock and was recorded as
	// a timeout.
	AnswerTimedOut
	// AnswerAbandoned: the answer came after the run window, which abandoned
	// the run.
	AnswerAbandoned
	// AnswerInvalid: the label did not parse; nothing was recorded and
	// ParseErr carries the reason.
	AnswerInvalid
)

// Presented is the item a Present step stopped on, with the unit and question
// the CLI prompt and the window's item view read.
type Presented struct {
	Index    int
	Item     assess.Item
	Content  exam.Exam
	Question exam.Question
}

// Started is one created run with its selected items in presentation order.
type Started struct {
	Run   assess.Run
	Items []assess.Item
}

// Result is a finished run read back with its items and recorded attempts: the
// inputs the CLI's run result document is built from.
type Result struct {
	Run      assess.Run
	Items    []assess.Item
	Attempts []attempt.Attempt
}

// Stepped is one Step call's result. Kind is set by Present, Timeout and
// Resume; Presented by Present when an item is ready; Outcome and ParseErr by
// Submit; Run and Items by Resume; Run by Abandon; Result by a call that read
// a finished run back (Present's Done and a resumed run that was completed).
type Stepped struct {
	Kind      Kind
	Presented *Presented
	Outcome   Outcome
	ParseErr  error
	Run       assess.Run
	Items     []assess.Item
	Result    *Result
}

// Step runs one action over the run runID identifies. It reads the run and its
// items from the store, one run per call, so both adapters step the same rules
// in the same order.
func Step(ctx context.Context, s *study.Service, runID int64, a Action, position int, notify Notify) (Stepped, error) {
	_ = ctx
	stepped, err := step(s, runID, a, position, notify)
	if err != nil {
		return Stepped{}, classify(err)
	}
	return stepped, nil
}

func step(s *study.Service, runID int64, a Action, position int, notify Notify) (Stepped, error) {
	switch a {
	case Present:
		return present(s, runID, position, notify)
	case Timeout:
		return timeout(s, runID, notify)
	case Resume:
		return resume(s, runID)
	case Abandon:
		run, err := abandonRun(s, runID)
		if err != nil {
			return Stepped{}, err
		}
		return Stepped{Run: run}, nil
	}
	return Stepped{}, kernel.Refuse("invalid_input", "unknown assessment action %d", a)
}

// Submit applies one raw label (today's stdin line, including "skip") to a
// presented item under the stored clocks: a label after the run window
// abandons the run, a label after the item clock is recorded as a timeout, an
// unparseable label records nothing, and anything else is stored as the item's
// attempt. run and p carry the clocks, item and question the caller resolved
// — the CLI loop's Present step output, the window's checked item — and Submit
// records against exactly them, so a content edit between the present and the
// answer cannot turn a stored answer into a refusal; the window's adapter
// keeps its own missing-content check before calling. ParseErr is set only for
// AnswerInvalid.
func Submit(ctx context.Context, s *study.Service, run assess.Run, p Presented, label string, notify Notify) (Stepped, error) {
	_ = ctx
	outcome, parseErr, err := submit(s, run, p.Item, p.Question, label, notify)
	if err != nil {
		return Stepped{}, classify(err)
	}
	return Stepped{Outcome: outcome, ParseErr: parseErr}, nil
}

// table maps the assessment store sentinels to the codes the CLI prints today.
var table = []kernel.Rule{
	{Is: store.ErrNotFound, Code: "unknown_run"},
	{Is: store.ErrAssessmentRunState, Code: "run_state"},
	{Is: store.ErrAssessmentWindowElapsed, Code: "run_abandoned"},
}

// classify wraps the store sentinels Step and Report return; a refusal already
// raised keeps its own code, anything else stays an internal error.
func classify(err error) error { return kernel.Classify(err, table) }

// AbandonedMessage is the text both adapters print when a run's whole window
// has passed: the CLI's run_abandoned error and the window's run message.
func AbandonedMessage(run assess.Run) string {
	return fmt.Sprintf("assessment run %d passed its %d-minute window and was recorded abandoned; finalized items remain readable with `passquest assess report %d`",
		run.ID, run.WindowMinutes, run.ID)
}

// Reconcile abandons every active run whose whole window has passed, recording
// the store's own window_elapsed abandonment. The CLI's assess commands run it
// right after opening, so a stale active run cannot block or mislead them; the
// window's reconcile applies the loop rule (Timeout) first and then this. It
// reads one clock value.
func Reconcile(ctx context.Context, s *study.Service) error {
	_ = ctx
	_, err := kernel.Store(s).ReconcileAssessmentRuns(kernel.P(s).Now())
	return err
}
