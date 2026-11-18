package workspace

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/studio"
	"github.com/wxxb789/passquest/internal/tutor"
)

func TestWorkspaceNotesFlow(t *testing.T) {
	t.Parallel()
	anki := newFakeAnki()
	f := newWSFixture(t, func(o *Options) { o.AnkiTransport = anki })
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample, QuestionID: "q2"})
	mustNoErr(t, err)
	feedback, err := f.w.Answer(item.Token, "A")
	mustNoErr(t, err)
	if open, err := f.w.OpenNoteDraft(); err != nil || open != nil {
		t.Fatalf("open draft before any proposal = %+v, %v", open, err)
	}
	draft, err := f.w.ProposeNote(studio.NoteRequest{AttemptID: feedback.AttemptID})
	mustNoErr(t, err)
	if draft.ID == 0 || draft.Fields.Front == "" || draft.Target.AttemptID != feedback.AttemptID {
		t.Fatalf("draft = %+v", draft)
	}
	// A restart finds the proposed draft unchanged.
	f.reopen(t)
	open, err := f.w.OpenNoteDraft()
	mustNoErr(t, err)
	if open == nil || !reflect.DeepEqual(*open, draft) {
		t.Fatalf("open draft after reopening = %+v, proposed %+v", open, draft)
	}
	preview, err := f.w.PreviewNote(draft.ID, studio.NoteFields{Front: "My own front"})
	mustNoErr(t, err)
	if !preview.Edited || !strings.Contains(preview.HTML, "My own front") || preview.CSS == "" || preview.HTMLSHA256 == "" {
		t.Fatalf("preview = %+v", preview)
	}
	accepted, err := f.w.AcceptNote(draft.ID, studio.NoteFields{Front: "My own front"})
	mustNoErr(t, err)
	if !accepted.Created || accepted.Note.Origin != "edited" || accepted.Note.Fields.Front != "My own front" {
		t.Fatalf("accept = %+v", accepted)
	}
	if open, err := f.w.OpenNoteDraft(); err != nil || open != nil {
		t.Fatalf("open draft after accept = %+v, %v", open, err)
	}
	list, err := f.w.Notes()
	mustNoErr(t, err)
	if len(list.Notes) != 1 || list.Counts.Total != 1 {
		t.Fatalf("notes = %+v", list)
	}
	anki.unreachable = true
	queued, err := f.w.PushNotes(false)
	mustNoErr(t, err)
	if queued.State != "queued" || queued.Counts.Queued != 1 || queued.Notes[0].Outcome != "unreachable" {
		t.Fatalf("unreachable push = %+v", queued)
	}
	anki.unreachable = false
	pushed, err := f.w.PushNotes(false)
	mustNoErr(t, err)
	if pushed.Error != nil || pushed.Counts.Pushed != 1 {
		t.Fatalf("push = %+v", pushed)
	}
	if after, _ := f.w.Notes(); after.Notes[0].AnkiState != store.AnkiStatePushed || after.Counts.Pushed != 1 {
		t.Fatalf("notes after push = %+v", after)
	}

	second, err := f.w.ProposeNote(studio.NoteRequest{ContentID: feedback.ContentID, QuestionID: "q3", Mark: "a passage phrase"})
	mustNoErr(t, err)
	if second.Target.Mark != "a passage phrase" || second.Target.AttemptID != 0 {
		t.Fatalf("mark draft = %+v", second)
	}
	if open, err := f.w.OpenNoteDraft(); err != nil || open == nil || open.ID != second.ID {
		t.Fatalf("open draft after the mark proposal = %+v, %v", open, err)
	}
	mustNoErr(t, f.w.SkipNote(second.ID))
	if open, err := f.w.OpenNoteDraft(); err != nil || open != nil {
		t.Fatalf("open draft after skip = %+v, %v", open, err)
	}
	if _, err := f.w.PreviewNote(second.ID, studio.NoteFields{}); studioCode(err) != "draft_closed" {
		t.Fatalf("preview of skipped draft = %v", err)
	}
	if _, err := f.w.ProposeNote(studio.NoteRequest{AttemptID: feedback.AttemptID, Mark: strings.Repeat("x", 200)}); studioCode(err) != "invalid_mark" {
		t.Fatalf("over-long mark = %v", err)
	}
}

// TestWorkspaceNotePreviewIsTheDeckCard: the window's card documents are the
// note package's rendering of the same fields with the model stylesheet, in
// Anki's DOM, for both sides and both themes.
func TestWorkspaceNotePreviewIsTheDeckCard(t *testing.T) {
	t.Parallel()
	f := newWSFixture(t, nil)
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample, QuestionID: "q2"})
	mustNoErr(t, err)
	feedback, err := f.w.Answer(item.Token, "A")
	mustNoErr(t, err)
	draft, err := f.w.ProposeNote(studio.NoteRequest{AttemptID: feedback.AttemptID})
	mustNoErr(t, err)
	preview, err := f.w.PreviewNote(draft.ID, studio.NoteFields{Front: "{{Back}} stays literal"})
	mustNoErr(t, err)
	fields := note.HTML(preview.Fields.Front, preview.Fields.Back, preview.Fields.Tactic, preview.Source)
	question, answer := note.RenderSides(fields)
	want := studio.NoteCardSide{Light: note.CardDocument(question, note.ModelCSS, false), Night: note.CardDocument(question, note.ModelCSS, true)}
	if preview.Question != want {
		t.Fatalf("question documents = %+v", preview.Question)
	}
	want = studio.NoteCardSide{Light: note.CardDocument(answer, note.ModelCSS, false), Night: note.CardDocument(answer, note.ModelCSS, true)}
	if preview.Answer != want || preview.HTML != answer || preview.CSS != note.ModelCSS {
		t.Fatalf("answer documents = %+v", preview.Answer)
	}
	if !strings.Contains(preview.Answer.Night, `<body class="card card1 nightMode night_mode">`) ||
		!strings.Contains(preview.Question.Light, `<body class="card card1">`) {
		t.Fatal("the documents do not carry Anki's body classes")
	}
	_, questionBody, _ := strings.Cut(preview.Question.Light, "<body")
	if !strings.Contains(questionBody, "{{Back}} stays literal") || strings.Contains(questionBody, "pq-tactic") {
		t.Fatalf("question side = %s", questionBody)
	}
	if preview.FieldsSHA256 != fields.Hash() || preview.HTMLSHA256 != preview.FieldsSHA256 {
		t.Fatalf("field hashes = %s / %s", preview.FieldsSHA256, preview.HTMLSHA256)
	}
	if preview.CSSSHA256 != note.CSSSHA256() || preview.ModelVersion != note.ModelVersion || !preview.Edited {
		t.Fatalf("preview model = %s / %d / %t", preview.CSSSHA256, preview.ModelVersion, preview.Edited)
	}
	keys := jsonKeys(t, preview)
	for _, key := range []string{"question", "answer", "html", "css", "html_sha256", "fields_sha256", "css_sha256", "model_version"} {
		if _, ok := keys[key]; !ok {
			t.Fatalf("preview JSON lacks %s: %v", key, keys)
		}
	}
}

// TestWorkspaceRefineNote: the refinement preview is the request the
// refinement sends; the refusals, the budget and the recorded failure are
// note propose --refine's; the suggestion is advisory and only accept applies
// it.
func TestWorkspaceRefineNote(t *testing.T) {
	t.Parallel()
	transport := &fakeTutorTransport{response: fixedTutorResponse("OK")}
	f := newWSFixture(t, func(o *Options) { o.TutorTransport = transport })
	f.env[provider.RegistryEnv] = modelRegistryFile(t, modelProvider("alpha", "http://127.0.0.1:9/v1/chat/completions"))
	f.env[provider.CredentialEnvFor("alpha")] = fakeTutorKey
	f.reopen(t)
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample, QuestionID: "q2"})
	mustNoErr(t, err)
	feedback, err := f.w.Answer(item.Token, "A")
	mustNoErr(t, err)
	draft, err := f.w.ProposeNote(studio.NoteRequest{AttemptID: feedback.AttemptID})
	mustNoErr(t, err)

	if _, err := f.w.RefineNotePreview(draft.ID); studioCode(err) != "not_configured" {
		t.Fatalf("preview without a model = %v", err)
	}
	if _, err := f.w.RefineNote(draft.ID); studioCode(err) != "not_configured" {
		t.Fatalf("refine without a model = %v", err)
	}
	_, err = f.w.UseModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	unverified, err := f.w.RefineNotePreview(draft.ID)
	mustNoErr(t, err)
	if unverified.Verified || unverified.Action == "" || unverified.TextChatState == "" {
		t.Fatalf("unverified preview = %+v", unverified)
	}
	if _, err := f.w.RefineNote(draft.ID); studioCode(err) != "capability_unverified" {
		t.Fatalf("refine before the model test = %v", err)
	}
	_, err = f.w.TestModel("alpha/alpha-text", "")
	mustNoErr(t, err)

	preview, err := f.w.RefineNotePreview(draft.ID)
	mustNoErr(t, err)
	if preview.DraftID != draft.ID || preview.Provider != "alpha" || preview.Model != "alpha-text" || preview.External ||
		!preview.Verified || preview.Action != "" || !preview.Credential.Present || preview.Unanswered {
		t.Fatalf("preview = %+v", preview)
	}
	if preview.Consent.Required || !preview.Consent.Granted || len(preview.Consent.Withheld) != 0 {
		t.Fatalf("preview consent = %+v", preview.Consent)
	}
	if len(preview.Messages) != 2 || preview.Messages[0].Role != "system" || preview.RequestSHA256 == "" || preview.RequestBytes == 0 {
		t.Fatalf("preview messages = %+v", preview)
	}
	if preview.Budget.Provider != "alpha" || preview.Budget.Limit == 0 {
		t.Fatalf("preview budget = %+v", preview.Budget)
	}
	callsBefore := preview.Budget.Calls
	sent := len(transport.requests)

	transport.response = fixedTutorResponse(`{"front": "closes at 4 p.m.", "back": "To shut at a stated time.\n\nExample: The library closes at 4 p.m."}`)
	refined, err := f.w.RefineNote(draft.ID)
	mustNoErr(t, err)
	if len(transport.requests) != sent+1 {
		t.Fatalf("provider calls = %d, want %d", len(transport.requests), sent+1)
	}
	body := transport.requests[len(transport.requests)-1].Body
	if sha, size := note.RequestIdentity(body); sha != preview.RequestSHA256 || size != preview.RequestBytes {
		t.Fatalf("the refinement sent %s/%d, the preview showed %s/%d", sha, size, preview.RequestSHA256, preview.RequestBytes)
	}
	if refined.State != "advisory" || !refined.Advisory || refined.DraftID != draft.ID || refined.RefinementID == 0 ||
		refined.Provider != "alpha" || refined.Model != "alpha-text" || refined.RequestSHA256 != preview.RequestSHA256 {
		t.Fatalf("refinement = %+v", refined)
	}
	if refined.Suggestion.Front != "closes at 4 p.m." || !strings.Contains(refined.Suggestion.Back, "Example: The library") ||
		refined.Suggestion.Tactic != draft.Fields.Tactic {
		t.Fatalf("suggestion = %+v", refined.Suggestion)
	}
	after, err := f.w.RefineNotePreview(draft.ID)
	mustNoErr(t, err)
	if after.Budget.Calls != callsBefore+1 {
		t.Fatalf("budget calls %d -> %d", callsBefore, after.Budget.Calls)
	}
	// The window's own fields stay the learner's: previewing them is not the
	// suggestion.
	kept, err := f.w.PreviewNote(draft.ID, draft.Fields)
	mustNoErr(t, err)
	if kept.Fields.Front != draft.Fields.Front || strings.Contains(kept.HTML, "closes at 4 p.m.</div>") {
		t.Fatalf("the suggestion replaced the learner's fields: %+v", kept.Fields)
	}

	// Typed failures are the CLI's: a provider error is recorded and spends
	// the slot; a malformed reply is invalid_response; no key refuses first.
	transport.response = fixedTutorError("rate_limited", "slow down")
	if _, err := f.w.RefineNote(draft.ID); studioCode(err) != "rate_limited" {
		t.Fatalf("provider failure = %v", err)
	}
	transport.response = fixedTutorResponse("no JSON here")
	if _, err := f.w.RefineNote(draft.ID); studioCode(err) != "invalid_response" {
		t.Fatalf("malformed reply = %v", err)
	}
	failed, err := f.w.RefineNotePreview(draft.ID)
	mustNoErr(t, err)
	if failed.Budget.Calls != callsBefore+3 {
		t.Fatalf("recorded failures did not spend the budget: %d -> %d", callsBefore, failed.Budget.Calls)
	}
	delete(f.env, provider.CredentialEnvFor("alpha"))
	sent = len(transport.requests)
	if _, err := f.w.RefineNote(draft.ID); studioCode(err) != "no_credential" || len(transport.requests) != sent {
		t.Fatalf("refine without a key = %v (calls %d -> %d)", err, sent, len(transport.requests))
	}
	if _, err := f.w.RefineNote(draft.ID + 99); studioCode(err) != "unknown_draft" {
		t.Fatalf("unknown draft = %v", err)
	}

	// The learner applies the suggestion explicitly.
	accepted, err := f.w.AcceptNote(draft.ID, refined.Suggestion)
	mustNoErr(t, err)
	if accepted.Note.Origin != "refined" || accepted.Note.Fields.Front != "closes at 4 p.m." {
		t.Fatalf("accept of the suggestion = %+v", accepted)
	}
	if listAttempts(t, f.db) != 1 {
		t.Fatal("a refinement changed attempts")
	}
	for _, value := range []any{preview, refined} {
		raw, _ := json.Marshal(value)
		if strings.Contains(string(raw), fakeTutorKey) {
			t.Fatalf("a document carries the key: %s", raw)
		}
	}
}

// TestWorkspaceRefineNoteRespectsConsent: a private source's draft shows no
// source text before the grant and the refinement refuses before any call.
func TestWorkspaceRefineNoteRespectsConsent(t *testing.T) {
	t.Parallel()
	transport := &fakeTutorTransport{response: fixedTutorResponse("OK")}
	f := newWSFixture(t, func(o *Options) { o.TutorTransport = transport })
	f.env[provider.RegistryEnv] = modelRegistryFile(t, modelProvider("alpha", "http://127.0.0.1:9/v1/chat/completions"))
	f.env[provider.CredentialEnvFor("alpha")] = fakeTutorKey
	f.reopen(t)
	clock := &stepClock{base: goldenBase}
	sourceID := noteSourceFixture(t, f.db, clock)
	mustRun(t, f.db, clock, "", "practice", "--source", sourceID, "--page", "1", "--question", "q2", "--answer", "A", "--json")
	draft, err := f.w.ProposeNote(studio.NoteRequest{AttemptID: 1})
	mustNoErr(t, err)
	_, err = f.w.UseModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	_, err = f.w.TestModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	sent := len(transport.requests)

	preview, err := f.w.RefineNotePreview(draft.ID)
	mustNoErr(t, err)
	if !preview.Consent.Required || preview.Consent.Granted || preview.Consent.SourceID != sourceID ||
		len(preview.Messages) != 0 || preview.RequestSHA256 != "" || !slices.Contains(preview.Consent.Withheld, "passage") {
		t.Fatalf("withheld preview = %+v", preview)
	}
	raw, _ := json.Marshal(preview)
	if strings.Contains(string(raw), "Westbridge") {
		t.Fatalf("the withheld preview shows source text: %s", raw)
	}
	if _, err := f.w.RefineNote(draft.ID); studioCode(err) != "consent_required" || len(transport.requests) != sent {
		t.Fatalf("refine before consent = %v", err)
	}
	// A grant for another provider covers the note text, not this provider.
	_, err = f.w.GrantConsent(sourceID, "")
	mustNoErr(t, err)
	_, err = f.w.RevokeConsent(sourceID, "alpha")
	mustNoErr(t, err)
	scoped, err := f.w.RefineNotePreview(draft.ID)
	mustNoErr(t, err)
	if scoped.Consent.Granted || len(scoped.Messages) != 0 {
		t.Fatalf("provider-scoped denial = %+v", scoped.Consent)
	}
	if _, err := f.w.RefineNote(draft.ID); studioCode(err) != "consent_required" || len(transport.requests) != sent {
		t.Fatalf("refine under a provider denial = %v", err)
	}
	_, err = f.w.GrantConsent(sourceID, "alpha")
	mustNoErr(t, err)
	granted, err := f.w.RefineNotePreview(draft.ID)
	mustNoErr(t, err)
	if !granted.Consent.Granted || !granted.Consent.SourceTextShared || len(granted.Messages) != 2 || granted.RequestSHA256 == "" {
		t.Fatalf("granted preview = %+v", granted)
	}
}

// TestWorkspaceRefinementStaysAdvisory: a stored refinement is a suggestion
// beside the draft, never the draft's own fields. After a restart the window
// restores the deterministic fields with the suggestion apart; an emptied
// field falls back to the deterministic value; accepting the draft's own
// fields records a deterministic note, and only the suggestion itself records
// a refined one.
// gatedTutorTransport answers like fakeTutorTransport; with a gate set, a call
// reports that it started and then waits for the gate before it replies.
type gatedTutorTransport struct {
	fakeTutorTransport
	started chan struct{}
	gate    chan struct{}
}

func (g *gatedTutorTransport) Call(ctx context.Context, req tutor.HostRequest) (tutor.HostResponse, error) {
	if g.gate != nil {
		close(g.started)
		<-g.gate
	}
	return g.fakeTutorTransport.Call(ctx, req)
}

// TestWorkspaceRefinementDoesNotUndoAnAccept: a refinement that is still
// waiting on the provider when the learner accepts the draft must not erase
// that acceptance when its reply lands. The paid call stays recorded; the
// draft keeps the accepted fields, so a restart does not bring it back.
func TestWorkspaceRefinementDoesNotUndoAnAccept(t *testing.T) {
	transport := &gatedTutorTransport{fakeTutorTransport: fakeTutorTransport{response: fixedTutorResponse("OK")}}
	f := newWSFixture(t, func(o *Options) { o.TutorTransport = transport })
	f.env[provider.RegistryEnv] = modelRegistryFile(t, modelProvider("alpha", "http://127.0.0.1:9/v1/chat/completions"))
	f.env[provider.CredentialEnvFor("alpha")] = fakeTutorKey
	f.reopen(t)
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample, QuestionID: "q2"})
	mustNoErr(t, err)
	feedback, err := f.w.Answer(item.Token, "A")
	mustNoErr(t, err)
	draft, err := f.w.ProposeNote(studio.NoteRequest{AttemptID: feedback.AttemptID})
	mustNoErr(t, err)
	_, err = f.w.UseModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	_, err = f.w.TestModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	before, err := f.w.RefineNotePreview(draft.ID)
	mustNoErr(t, err)

	transport.response = fixedTutorResponse(`{"front": "closes at 4 p.m.", "back": "To shut at a stated time.\n\nExample: The library closes at 4 p.m."}`)
	transport.started, transport.gate = make(chan struct{}), make(chan struct{})
	type outcome struct {
		refinement studio.NoteRefinement
		err        error
	}
	done := make(chan outcome, 1)
	go func() {
		refinement, err := f.w.RefineNote(draft.ID)
		done <- outcome{refinement, err}
	}()
	select {
	case <-transport.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the refinement never reached the provider")
	}
	// One paid request per draft at a time, whatever the window remembers.
	if _, err := f.w.RefineNote(draft.ID); studioCode(err) != "refine_in_progress" {
		t.Fatalf("a second refinement while the first waits = %v; want refine_in_progress", err)
	}
	accepted, err := f.w.AcceptNote(draft.ID, draft.Fields)
	mustNoErr(t, err)
	close(transport.gate)
	var result outcome
	select {
	case result = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the refinement did not return after its reply")
	}
	if studioCode(result.err) != "draft_changed" {
		t.Fatalf("a refinement that finished after the accept = %+v, %v; want draft_changed", result.refinement, result.err)
	}

	f.reopen(t)
	open, err := f.w.OpenNoteDraft()
	mustNoErr(t, err)
	if open != nil {
		t.Fatalf("after a restart the accepted draft is pending again: %+v", open)
	}
	st, err := store.Open(f.db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	stored, found, err := st.NoteDraftByID(draft.ID)
	if err != nil || !found {
		t.Fatalf("read draft = %+v, %t, %v", stored, found, err)
	}
	if stored.AcceptedFront != accepted.Note.Fields.Front || stored.AcceptedBack != accepted.Note.Fields.Back ||
		stored.RefinedFront != "" || stored.Origin != "deterministic" {
		t.Fatalf("the late refinement changed the accepted draft: %+v", stored)
	}
	records, err := st.NoteRefinements()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Status != "ok" || records[0].DraftID != draft.ID {
		t.Fatalf("the provider call is not recorded: %+v", records)
	}
	after, err := f.w.RefineNotePreview(draft.ID)
	mustNoErr(t, err)
	if after.Budget.Calls != before.Budget.Calls+1 {
		t.Fatalf("budget calls %d -> %d, want the paid call counted", before.Budget.Calls, after.Budget.Calls)
	}
}

func TestWorkspaceRefinementStaysAdvisory(t *testing.T) {
	t.Parallel()
	transport := &fakeTutorTransport{response: fixedTutorResponse("OK")}
	f := newWSFixture(t, func(o *Options) { o.TutorTransport = transport })
	f.env[provider.RegistryEnv] = modelRegistryFile(t, modelProvider("alpha", "http://127.0.0.1:9/v1/chat/completions"))
	f.env[provider.CredentialEnvFor("alpha")] = fakeTutorKey
	f.reopen(t)
	item, err := f.w.StartPractice(studio.PracticeRequest{Kind: studio.PracticeSample, QuestionID: "q2"})
	mustNoErr(t, err)
	feedback, err := f.w.Answer(item.Token, "A")
	mustNoErr(t, err)
	draft, err := f.w.ProposeNote(studio.NoteRequest{AttemptID: feedback.AttemptID})
	mustNoErr(t, err)
	_, err = f.w.UseModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	_, err = f.w.TestModel("alpha/alpha-text", "")
	mustNoErr(t, err)
	transport.response = fixedTutorResponse(`{"front": "closes at 4 p.m.", "back": "To shut at a stated time.\n\nExample: The library closes at 4 p.m."}`)
	refined, err := f.w.RefineNote(draft.ID)
	mustNoErr(t, err)

	f.reopen(t)
	open, err := f.w.OpenNoteDraft()
	mustNoErr(t, err)
	if open == nil || open.ID != draft.ID || open.Fields != draft.Fields || open.Origin != "deterministic" || open.Advisory {
		t.Fatalf("restored draft = %+v, want the deterministic fields %+v", open, draft.Fields)
	}
	if open.Suggestion == nil || open.Suggestion.Suggestion != refined.Suggestion || open.Suggestion.RefinementID != refined.RefinementID ||
		!open.Suggestion.Advisory || open.Suggestion.Provider != "alpha" || open.Suggestion.Model != "alpha-text" {
		t.Fatalf("restored suggestion = %+v, want %+v", open.Suggestion, refined)
	}

	own, err := f.w.PreviewNote(draft.ID, draft.Fields)
	mustNoErr(t, err)
	if own.Fields != draft.Fields || own.Edited {
		t.Fatalf("preview of the draft's own fields = %+v (edited %v)", own.Fields, own.Edited)
	}
	used, err := f.w.PreviewNote(draft.ID, refined.Suggestion)
	mustNoErr(t, err)
	if used.Fields != refined.Suggestion || used.Edited {
		t.Fatalf("preview of the used suggestion = %+v (edited %v)", used.Fields, used.Edited)
	}
	emptied, err := f.w.PreviewNote(draft.ID, studio.NoteFields{Front: "My own front", Tactic: draft.Fields.Tactic})
	mustNoErr(t, err)
	if emptied.Fields.Back != draft.Fields.Back || !emptied.Edited {
		t.Fatalf("an emptied back became %q, want the draft's %q", emptied.Fields.Back, draft.Fields.Back)
	}

	accepted, err := f.w.AcceptNote(draft.ID, draft.Fields)
	mustNoErr(t, err)
	if accepted.Note.Origin != "deterministic" || accepted.Note.Fields.Front != draft.Fields.Front ||
		accepted.Note.Fields.Back != draft.Fields.Back {
		t.Fatalf("accepting the draft's own fields recorded %+v", accepted.Note)
	}
	again, err := f.w.AcceptNote(draft.ID, refined.Suggestion)
	mustNoErr(t, err)
	if again.Note.Origin != "edited" || again.Note.Fields.Front != refined.Suggestion.Front {
		t.Fatalf("a later accept of the suggestion over the accepted fields = %+v", again.Note)
	}
}
