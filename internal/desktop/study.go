package desktop

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/wxxb789/passquest/internal/cli"
	"github.com/wxxb789/passquest/internal/studio"
)

// Error convention of the bound service: a method that fails rejects its
// promise with a CallError whose message is the JSON of a studio.Error,
// {"code","message","action"}. Errors that are not studio errors arrive as
// code "internal_error". The generated client's wrapper
// (desktop/web/src/api/client.ts) parses the message back into a StudioError.

// Brief phases carried by BriefChanged.
const (
	BriefStarted  = "started"
	BriefFinished = "finished"
)

// BriefEvent reports a study brief resumed in the background: once when the
// host starts and once when it returns, with its status or its error.
type BriefEvent struct {
	Phase  string              `json:"phase"`
	Status *studio.BriefStatus `json:"status"`
	Error  *studio.Error       `json:"error"`
}

// Study is the service the window calls: every method forwards to the one
// cli.Workspace of this window. MyGo runs each call on its own goroutine, so
// the methods that wait on a Node host (CoachAsk, TestModel, PushNotes,
// BriefStatus) never block the window; ResumeBrief returns at once and reports
// through the brief:changed event. A failed call rejects with a CallError
// whose message is the JSON of a studio.Error: {"code","message","action"}.
type Study struct {
	mu        sync.Mutex
	ws        *cli.Workspace
	openErr   error
	closing   bool
	calls     int
	inFlight  sync.WaitGroup
	resuming  bool
	emitBrief func(BriefEvent)
}

// newStudy returns a service with no workspace yet: calls fail with
// "starting" until open succeeds.
func newStudy(emitBrief func(BriefEvent)) *Study {
	return &Study{emitBrief: emitBrief, openErr: &studio.Error{Code: "starting", Message: "the study workspace is still opening"}}
}

// open binds the workspace; a failure is what every later call reports.
func (s *Study) open(options cli.WorkspaceOptions) error {
	ws, err := cli.OpenWorkspace(options)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ws, s.openErr = ws, err
	return err
}

// begin registers one call. It fails once the app started quitting, so the
// drain in drain is bounded by the calls already running.
func (s *Study) begin() (*cli.Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return nil, &studio.Error{Code: "quitting", Message: "Pass Quest is closing"}
	}
	if s.ws == nil {
		return nil, s.openErr
	}
	s.calls++
	s.inFlight.Add(1)
	return s.ws, nil
}

// end unregisters one call.
func (s *Study) end() {
	s.mu.Lock()
	s.calls--
	s.mu.Unlock()
	s.inFlight.Done()
}

// busy reports whether any call is running.
func (s *Study) busy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls > 0
}

// drain refuses new calls and waits at most grace for the running ones. It
// reports whether every call returned in time.
func (s *Study) drain(grace time.Duration) bool {
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()
	done := make(chan struct{})
	go func() {
		s.inFlight.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(grace):
		return false
	}
}

// callError turns a workspace error into the JSON-message error of the
// convention above.
func callError(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(string(errorJSON(asStudioError(err))))
}

func asStudioError(err error) *studio.Error {
	var typed *studio.Error
	if errors.As(err, &typed) && typed != nil {
		return typed
	}
	return &studio.Error{Code: "internal_error", Message: err.Error()}
}

func errorJSON(e *studio.Error) []byte {
	b, err := json.Marshal(e)
	if err != nil {
		return []byte(`{"code":"internal_error","message":"unencodable error"}`)
	}
	return b
}

// call runs fn on the workspace as one tracked call.
func call[T any](s *Study, fn func(*cli.Workspace) (T, error)) (T, error) {
	ws, err := s.begin()
	if err != nil {
		var zero T
		return zero, callError(err)
	}
	defer s.end()
	value, err := fn(ws)
	return value, callError(err)
}

func callNoValue(s *Study, fn func(*cli.Workspace) error) error {
	_, err := call(s, func(ws *cli.Workspace) (struct{}, error) { return struct{}{}, fn(ws) })
	return err
}

// Today returns the opening view: the next action, its evidence and the local state around it.
func (s *Study) Today() (studio.Today, error) { return call(s, (*cli.Workspace).Today) }

// SetLearnerSettings stores the display-only exam date and daily minutes; empty values clear them.
func (s *Study) SetLearnerSettings(settings studio.LearnerSettings) error {
	return callNoValue(s, func(ws *cli.Workspace) error { return ws.SetLearnerSettings(settings) })
}

// StartPractice presents a practice item and records its presentation.
func (s *Study) StartPractice(request studio.PracticeRequest) (studio.PracticeItem, error) {
	return call(s, func(ws *cli.Workspace) (studio.PracticeItem, error) { return ws.StartPractice(request) })
}

// ResumePractice returns the open practice item after an interruption, or null.
func (s *Study) ResumePractice() (*studio.PracticeItem, error) {
	return call(s, (*cli.Workspace).ResumePractice)
}

// OpenHint opens the authored hint of the item; the attempt then counts as assisted.
func (s *Study) OpenHint(token string) (studio.PracticeItem, error) {
	return call(s, func(ws *cli.Workspace) (studio.PracticeItem, error) { return ws.OpenHint(token) })
}

// Answer records the answer to the presented item and returns its feedback.
func (s *Study) Answer(token, label string) (studio.Feedback, error) {
	return call(s, func(ws *cli.Workspace) (studio.Feedback, error) { return ws.Answer(token, label) })
}

// Skip records the presented item as skipped and returns its feedback.
func (s *Study) Skip(token string) (studio.Feedback, error) {
	return call(s, func(ws *cli.Workspace) (studio.Feedback, error) { return ws.Skip(token) })
}

// CoachPreview shows what a coach request would send; attemptID 0 is the latest attempt.
func (s *Study) CoachPreview(attemptID int64, mode string) (studio.CoachPreview, error) {
	return call(s, func(ws *cli.Workspace) (studio.CoachPreview, error) { return ws.CoachPreview(attemptID, mode) })
}

// CoachAsk sends the coach request through the Node host and waits for the advisory reply.
func (s *Study) CoachAsk(attemptID int64, mode string) (studio.CoachReply, error) {
	return call(s, func(ws *cli.Workspace) (studio.CoachReply, error) { return ws.CoachAsk(attemptID, mode) })
}

// GrantConsent records consent to send a source to a provider ("" = every provider).
func (s *Study) GrantConsent(sourceID, providerID string) (studio.Consent, error) {
	return call(s, func(ws *cli.Workspace) (studio.Consent, error) { return ws.GrantConsent(sourceID, providerID) })
}

// RevokeConsent revokes consent to send a source to a provider ("" = every provider).
func (s *Study) RevokeConsent(sourceID, providerID string) (studio.Consent, error) {
	return call(s, func(ws *cli.Workspace) (studio.Consent, error) { return ws.RevokeConsent(sourceID, providerID) })
}

// Consents lists the recorded consent ledger.
func (s *Study) Consents() ([]studio.Consent, error) { return call(s, (*cli.Workspace).Consents) }

// AssessmentPool reports the unseen assessment pool.
func (s *Study) AssessmentPool() (studio.AssessmentPool, error) {
	return call(s, (*cli.Workspace).AssessmentPool)
}

// StartAssessment starts a run (0 = default size and minutes) and presents its first item.
func (s *Study) StartAssessment(size, minutes int) (studio.AssessmentState, error) {
	return call(s, func(ws *cli.Workspace) (studio.AssessmentState, error) { return ws.StartAssessment(size, minutes) })
}

// AssessmentState returns the current run without presenting anything.
func (s *Study) AssessmentState() (studio.AssessmentState, error) {
	return call(s, (*cli.Workspace).AssessmentState)
}

// PresentAssessmentItem presents the run's current item.
func (s *Study) PresentAssessmentItem(runID int64) (studio.AssessmentState, error) {
	return call(s, func(ws *cli.Workspace) (studio.AssessmentState, error) { return ws.PresentAssessmentItem(runID) })
}

// AnswerAssessmentItem records an answer ("skip" skips) for the item at position.
func (s *Study) AnswerAssessmentItem(runID int64, position int, label string) (studio.AssessmentAnswer, error) {
	return call(s, func(ws *cli.Workspace) (studio.AssessmentAnswer, error) {
		return ws.AnswerAssessmentItem(runID, position, label)
	})
}

// ResumeAssessment resumes an interrupted run and presents its next item.
func (s *Study) ResumeAssessment(runID int64) (studio.AssessmentState, error) {
	return call(s, func(ws *cli.Workspace) (studio.AssessmentState, error) { return ws.ResumeAssessment(runID) })
}

// AbandonAssessment abandons a run.
func (s *Study) AbandonAssessment(runID int64) (studio.AssessmentState, error) {
	return call(s, func(ws *cli.Workspace) (studio.AssessmentState, error) { return ws.AbandonAssessment(runID) })
}

// AssessmentReport returns a run's report; the pass threshold is always "unknown".
func (s *Study) AssessmentReport(runID int64) (studio.AssessmentReport, error) {
	return call(s, func(ws *cli.Workspace) (studio.AssessmentReport, error) { return ws.AssessmentReport(runID) })
}

// AssessmentCompare compares two comparable runs.
func (s *Study) AssessmentCompare(first, second int64) (studio.AssessmentComparison, error) {
	return call(s, func(ws *cli.Workspace) (studio.AssessmentComparison, error) {
		return ws.AssessmentCompare(first, second)
	})
}

// AssessmentRuns lists the runs, newest first.
func (s *Study) AssessmentRuns() ([]studio.AssessmentRun, error) {
	return call(s, (*cli.Workspace).AssessmentRuns)
}

// ProposeNote drafts an Anki note from an attempt or a question.
func (s *Study) ProposeNote(request studio.NoteRequest) (studio.NoteDraft, error) {
	return call(s, func(ws *cli.Workspace) (studio.NoteDraft, error) { return ws.ProposeNote(request) })
}

// PreviewNote renders the draft as the card Anki will show; empty fields keep the draft's values.
func (s *Study) PreviewNote(draftID int64, fields studio.NoteFields) (studio.NotePreview, error) {
	return call(s, func(ws *cli.Workspace) (studio.NotePreview, error) { return ws.PreviewNote(draftID, fields) })
}

// AcceptNote accepts the draft with the edited fields.
func (s *Study) AcceptNote(draftID int64, fields studio.NoteFields) (studio.NoteAccept, error) {
	return call(s, func(ws *cli.Workspace) (studio.NoteAccept, error) { return ws.AcceptNote(draftID, fields) })
}

// SkipNote discards the draft.
func (s *Study) SkipNote(draftID int64) error {
	return callNoValue(s, func(ws *cli.Workspace) error { return ws.SkipNote(draftID) })
}

// Notes lists the local notes with their Anki state.
func (s *Study) Notes() (studio.NoteList, error) { return call(s, (*cli.Workspace).Notes) }

// PushNotes pushes accepted notes to AnkiConnect; force keeps the local version of a conflict.
func (s *Study) PushNotes(force bool) (studio.NotePush, error) {
	return call(s, func(ws *cli.Workspace) (studio.NotePush, error) { return ws.PushNotes(force) })
}

// Models lists the provider registry with each model's separate facts.
func (s *Study) Models() (studio.ModelList, error) { return call(s, (*cli.Workspace).Models) }

// UseModel selects "provider/model" as the coach model.
func (s *Study) UseModel(id, endpoint string) (studio.ModelUse, error) {
	return call(s, func(ws *cli.Workspace) (studio.ModelUse, error) { return ws.UseModel(id, endpoint) })
}

// TestModel probes a model through the Node host; a failed probe is a result, not an error.
func (s *Study) TestModel(id, endpoint string) (studio.ModelTest, error) {
	return call(s, func(ws *cli.Workspace) (studio.ModelTest, error) { return ws.TestModel(id, endpoint) })
}

// Budget reports today's per-provider call budget (UTC).
func (s *Study) Budget() (studio.Budget, error) { return call(s, (*cli.Workspace).Budget) }

// SetCredential stores a provider key in the OS credential store; only its presence is returned.
func (s *Study) SetCredential(providerID, key string) (studio.CredentialStatus, error) {
	return call(s, func(ws *cli.Workspace) (studio.CredentialStatus, error) { return ws.SetCredential(providerID, key) })
}

// ClearCredential removes a provider key from the OS credential store.
func (s *Study) ClearCredential(providerID string) (studio.CredentialStatus, error) {
	return call(s, func(ws *cli.Workspace) (studio.CredentialStatus, error) { return ws.ClearCredential(providerID) })
}

// Services reports Node, AnkiConnect, search, the credential store and the database path.
func (s *Study) Services() (studio.Services, error) { return call(s, (*cli.Workspace).Services) }

// BriefStatus reports the latest study brief; it may start the durable host for up to 20 s.
func (s *Study) BriefStatus() (studio.BriefStatus, error) {
	return call(s, (*cli.Workspace).BriefStatus)
}

// ResumeBrief resumes the latest unfinished brief in the background and returns at once;
// brief:changed reports when the host starts and when it returns. A second resume while one
// runs fails with brief_running.
func (s *Study) ResumeBrief() error {
	ws, err := s.begin()
	if err != nil {
		return callError(err)
	}
	s.mu.Lock()
	if s.resuming {
		s.mu.Unlock()
		s.end()
		return callError(&studio.Error{Code: "brief_running", Message: "a study brief is already resuming"})
	}
	s.resuming = true
	s.mu.Unlock()
	s.emitBrief(BriefEvent{Phase: BriefStarted})
	go func() {
		defer s.end()
		status, err := ws.ResumeBrief()
		s.mu.Lock()
		s.resuming = false
		s.mu.Unlock()
		event := BriefEvent{Phase: BriefFinished}
		if err != nil {
			event.Error = asStudioError(err)
		} else {
			event.Status = &status
		}
		s.emitBrief(event)
	}()
	return nil
}

// CancelBrief cancels the latest unfinished brief.
func (s *Study) CancelBrief() (studio.BriefStatus, error) {
	return call(s, (*cli.Workspace).CancelBrief)
}
