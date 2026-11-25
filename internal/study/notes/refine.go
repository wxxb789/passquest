package notes

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
	"github.com/wxxb789/passquest/internal/study/models"
	"github.com/wxxb789/passquest/internal/tutor"
)

// notConfigured is the refusal both refinement entry points print when no
// provider is saved; the notes variant names the way out of the refinement.
const notConfigured = "no tutor provider is configured; run passquest tutor config --endpoint URL --model NAME, or omit --refine"

// Failure is one recorded provider failure: `note propose --refine` and the
// window's RefineNote report it as a result — exit 3, the recorded code and
// message — never as an error. Recorded says the failure row was stored, which
// the human report discloses.
type Failure struct {
	Code     string
	Message  string
	Recorded bool
}

// Refinement is one refinement's outcome: the recorded request, the draft it
// was applied to, the deterministic draft it was built from, or the recorded
// failure. The window renders the unused suggestion from these values.
type Refinement struct {
	Draft   store.NoteDraft
	Built   note.Draft
	Record  store.NoteRefinement
	Failure *Failure
}

// Preview is what a refinement would send for one draft: the resolved request
// identity, the text.chat verification state and the consent that covers it.
// Nothing is sent and no budget slot is claimed.
type Preview struct {
	DraftID           int64
	Config            tutor.Config
	ProviderID        string
	HasCapability     bool
	Capability        models.CapabilityState
	CredentialEnv     string
	CredentialPresent bool
	Unanswered        bool
	Messages          []tutor.Message
	MaxTokens         int
	RequestSHA256     string
	RequestBytes      int
	Consent           Consent
}

// Consent is the source-consent state one refinement discloses.
type Consent struct {
	Private          bool
	SourceID         string
	Required         bool
	Granted          bool
	SourceTextShared bool
	Withheld         []string
}

// Refine performs one explicit provider refinement for a draft and records its
// exact request identity: the CLI's `note propose --refine` step and the
// window's RefineNote. A withheld draft is refused before anything resolves.
func Refine(ctx context.Context, s *study.Service, draftID int64) (Refinement, error) {
	effective, err := Draft(ctx, s, draftID)
	if err != nil {
		return Refinement{}, err
	}
	if len(effective.Built.Withheld) > 0 {
		return Refinement{}, kernel.Refuse("consent_required", "%s", ConsentMessage(effective.Draft))
	}
	draft, record, failure, err := refine(ctx, s, effective.Draft, effective.Input, effective.Built)
	if err != nil {
		return Refinement{}, err
	}
	return Refinement{Draft: draft, Built: effective.Built, Record: record, Failure: failure}, nil
}

// RefinePreview resolves what a refinement would send for this draft, built by
// the same request builder, before anything leaves the machine: provider and
// model, the text.chat verification, the credential's presence, the consent
// that covers this provider, the exact messages with their hash and size and
// the budget slot the call would draw on. Messages stay empty while consent
// withholds the source text, because that text must not be shown as if it
// would be sent.
func RefinePreview(ctx context.Context, s *study.Service, draftID int64) (Preview, error) {
	_ = ctx
	draft, err := locate(s, draftID)
	if err != nil {
		return Preview{}, err
	}
	input, err := inputFromDraft(s, draft)
	if err != nil {
		return Preview{}, classify(err)
	}
	built, err := note.Build(input)
	if err != nil {
		return Preview{}, classify(err)
	}
	cfg, err := kernel.Configured(s, notConfigured)
	if err != nil {
		return Preview{}, err
	}
	cfg, _, _, err = kernel.ResolveActive(s, cfg)
	if err != nil {
		return Preview{}, err
	}
	request, err := note.BuildRefineRequest(cfg, built, material(input, built))
	if err != nil {
		return Preview{}, kernel.Refuse("invalid_config", "%s", err.Error())
	}
	providerID := strings.TrimSpace(cfg.Provider)
	env, _, present := kernel.Credential(s, providerID)
	preview := Preview{
		DraftID:           draft.ID,
		Config:            cfg,
		ProviderID:        providerID,
		CredentialEnv:     env,
		CredentialPresent: present,
		Unanswered:        built.Unanswered,
		MaxTokens:         request.WireMaxTokens(),
	}
	if providerID != "" {
		index, err := kernel.LoadVerificationIndex(s)
		if err != nil {
			return Preview{}, err
		}
		preview.HasCapability = true
		preview.Capability = models.CapabilityStateOf(provider.CapTextChat, true, index.ForModel(providerID, cfg.Model))
	}
	// The note's own text needs the global decision (built.Withheld); the
	// provider needs the consent that covers it, exactly as refine checks.
	required := input.Target.Restricted && input.Target.SourceID != ""
	consent := Consent{
		Private:  input.Target.Restricted,
		SourceID: input.Target.SourceID,
		Required: required,
		Withheld: append([]string{}, built.Withheld...),
	}
	consent.Granted = !consent.Required
	if consent.Required && len(built.Withheld) == 0 {
		event, ok, err := kernel.Store(s).TutorConsent(input.Target.SourceID, providerID)
		if err != nil {
			return Preview{}, err
		}
		consent.Granted = ok && event.Granted
	}
	if consent.Granted {
		consent.SourceTextShared = consent.Private
		preview.Messages = request.WireMessages()
		preview.RequestSHA256, preview.RequestBytes = note.RequestIdentity(request.Body)
	} else {
		for _, field := range []string{"front", "back", "passage", "question_stem", "evidence"} {
			if !slices.Contains(consent.Withheld, field) {
				consent.Withheld = append(consent.Withheld, field)
			}
		}
	}
	preview.Consent = consent
	return preview, nil
}

// refine is one explicit provider refinement that records its exact request
// identity. It returns the updated draft and the recorded row, or the
// recorded failure whose slot stays spent.
func refine(ctx context.Context, s *study.Service, draft store.NoteDraft, input note.Input, deterministic note.Draft) (store.NoteDraft, store.NoteRefinement, *Failure, error) {
	st := kernel.Store(s)
	p := kernel.P(s)
	cfg, err := kernel.Configured(s, notConfigured)
	if err != nil {
		return draft, store.NoteRefinement{}, nil, err
	}
	// Refinement is an inference destination, so it resolves the active pair
	// exactly like tutor ask: the registry endpoint replaces a stale saved one,
	// the pair must declare text.chat, and its evidence must be verified before
	// any text leaves the machine.
	cfg, registry, hasProvider, err := kernel.ResolveActive(s, cfg)
	if err != nil {
		return draft, store.NoteRefinement{}, nil, err
	}
	if hasProvider {
		if err := kernel.RequireVerifiedTextChat(s, registry, cfg.Provider, cfg.Model); err != nil {
			return draft, store.NoteRefinement{}, nil, err
		}
	}
	// The draft's note text and its Anki export stay on the global consent
	// decision, which is a different destination (see consentGranted). Sending
	// this source to a provider needs the consent that covers that provider: a
	// provider-scoped denial after a global grant overrides it.
	if input.Target.Restricted && input.Target.SourceID != "" {
		event, ok, err := st.TutorConsent(input.Target.SourceID, strings.TrimSpace(cfg.Provider))
		if err != nil {
			return draft, store.NoteRefinement{}, nil, err
		}
		if !ok || !event.Granted {
			return draft, store.NoteRefinement{}, nil, kernel.Refuse("consent_required",
				"%s", refineConsentMessage(input.Target.SourceID, cfg.Provider))
		}
	}
	env, key, present := kernel.Credential(s, cfg.Provider)
	if !present {
		return draft, store.NoteRefinement{}, nil, kernel.Refuse("no_credential",
			"%s is not set; export it in this environment (it is never stored)", env)
	}
	now := p.Now()
	lease, used, err := kernel.Reserve(s, (*store.Store).ReserveNoteRefinement, cfg.Provider, now, cfg.DailyCalls)
	if err != nil {
		return draft, store.NoteRefinement{}, nil, err
	}
	defer lease.Close()
	if lease.ID() == 0 {
		return draft, store.NoteRefinement{}, nil, kernel.Refuse("budget_exhausted",
			"%d of %d provider calls are used today (UTC); raise the budget with passquest tutor config --daily-calls N",
			used, cfg.DailyCalls)
	}
	request, err := note.BuildRefineRequest(cfg, deterministic, material(input, deterministic))
	if err != nil {
		return draft, store.NoteRefinement{}, nil, kernel.Refuse("invalid_config", "%s", err.Error())
	}
	request.CredentialEnv = env
	sha, bytes := note.RequestIdentity(request.Body)
	record := store.NoteRefinement{
		DraftID:       draft.ID,
		CreatedAt:     now,
		Provider:      cfg.Provider,
		ProviderHost:  cfg.ProviderHost(),
		Model:         cfg.Model,
		PromptVersion: note.PromptVersion,
		RequestSHA256: sha,
		RequestBytes:  bytes,
	}
	// From here on the slot is never released implicitly: the provider may
	// already have been contacted, so a crash must under-spend, never
	// over-spend.
	lease.Sending()
	started := p.Now()
	response, transportErr := p.Tutor.Call(ctx, request)
	record.LatencyMS = p.Now().Sub(started).Milliseconds()
	if transportErr != nil {
		record.Status = "error"
		record.ErrorCode = "agent_unavailable"
		record.ErrorMessage = kernel.Redact(transportErr.Error(), key)
		return draft, record, recordFailure(s, record, lease), nil
	}
	record.HTTPStatus = response.HTTPStatus
	if response.Usage != nil {
		prompt, completion := response.Usage.PromptTokens, response.Usage.CompletionTokens
		record.PromptTokens = &prompt
		record.CompletionTokens = &completion
	}
	if response.Status == "error" {
		code, message := kernel.HostError(response)
		record.Status = "error"
		record.ErrorCode = code
		record.ErrorMessage = kernel.Redact(message, key)
		return draft, record, recordFailure(s, record, lease), nil
	}
	front, back, err := note.ParseRefinement(response.Text)
	if err != nil {
		record.Status = "error"
		record.ErrorCode = "invalid_response"
		record.ErrorMessage = kernel.Redact(err.Error(), key)
		return draft, record, recordFailure(s, record, lease), nil
	}
	if deterministic.Unanswered && input.Question != nil &&
		note.RevealsAnswer(*input.Question, front+"\n"+back) {
		record.Status = "error"
		record.ErrorCode = "invalid_response"
		record.ErrorMessage = "the refinement would reveal the answer of an unanswered question; the deterministic draft is unchanged"
		return draft, record, recordFailure(s, record, lease), nil
	}
	record.Status = "ok"
	record.OutputFront = front
	record.OutputBack = back
	saved, err := st.FinalizeNoteRefinement(record, lease.ID())
	if err != nil {
		return draft, store.NoteRefinement{}, nil, err
	}
	// The provider call stays recorded above; the draft takes the suggestion
	// only if nobody accepted or skipped it while the call ran.
	if err := st.SetNoteDraftRefinement(draft, saved.ID, cfg.Model, front, back); errors.Is(err, store.ErrNoteDraftChanged) {
		return draft, store.NoteRefinement{}, nil, kernel.Refuse("draft_changed",
			"note draft #%d was accepted or skipped while the refinement ran; refinement #%d is recorded but not applied", draft.ID, saved.ID)
	} else if err != nil {
		return draft, store.NoteRefinement{}, nil, err
	}
	updated, found, err := st.NoteDraftByID(draft.ID)
	if err != nil {
		return draft, store.NoteRefinement{}, nil, err
	}
	if !found {
		return draft, store.NoteRefinement{}, nil, errors.New("note draft disappeared after refinement")
	}
	return updated, saved, nil, nil
}

// recordFailure finalizes one failed refinement row and reports the failure as
// a result. A failure to record the failure itself keeps the slot held: the
// provider may already have been contacted.
func recordFailure(s *study.Service, record store.NoteRefinement, lease *kernel.Lease) *Failure {
	failure := &Failure{Code: record.ErrorCode, Message: record.ErrorMessage}
	if _, err := kernel.Store(s).FinalizeNoteRefinement(record, lease.ID()); err != nil {
		return failure
	}
	failure.Recorded = true
	return failure
}

// material is the context a refinement request carries. The window's
// refinement preview builds the same request from it, so what it shows is what
// would be sent.
func material(input note.Input, deterministic note.Draft) note.RefineMaterial {
	refineMaterial := note.RefineMaterial{}
	if input.Question != nil {
		refineMaterial.QuestionStem = input.Question.Stem
	}
	if deterministic.Unanswered {
		// The raw passage can contain the keyed option; send only the example
		// the deterministic draft already deemed pre-answer-visible.
		refineMaterial.PassageText = deterministic.Example
	} else {
		refineMaterial.PassageText = input.PassageText
		if input.Question != nil {
			refineMaterial.Evidence = input.Question.Evidence
		}
	}
	return refineMaterial
}
