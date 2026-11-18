// Package coach orchestrates the advisory tutor surfaces: the preview of what
// a call would send, the one bounded advisory call with its failover, the
// consent ledger and the stored-call reads. Results carry exactly the inputs
// the CLI's view builders and the desktop's studio mappers read; nothing here
// renders. The CLI's tutor commands and the window's coach methods are the two
// adapters.
package coach

import (
	"context"
	"errors"
	"strings"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
	"github.com/wxxb789/passquest/internal/study/models"
	"github.com/wxxb789/passquest/internal/tutor"
)

// notConfigured is the refusal both configured-provider entry points print
// when no provider is saved.
const notConfigured = "no tutor provider is configured; run passquest tutor config --endpoint URL --model NAME"

// ParseMode is a pure function that gives the mode a preview or call works on,
// or the invalid_mode refusal. Both adapters call it after the group guard and
// before study.Open, as the CLI does today, so an assessment guard still wins
// over an invalid mode.
func ParseMode(raw string) (tutor.Mode, error) {
	mode := tutor.Mode(strings.TrimSpace(raw))
	if !mode.Valid() {
		return "", kernel.Refuse("invalid_mode", "mode %q is not supported; use explain, probe or micro_lesson", raw)
	}
	return mode, nil
}

// Request selects the attempt and mode one preview or call works on. AttemptID
// 0 selects the latest attempt.
type Request struct {
	AttemptID int64
	Mode      tutor.Mode
}

// Prepared is one resolved advisory request: the attempt, the refreshed
// configuration, the exact request the host would send, the text.chat evidence
// (nil for the legacy endpoint) and the credential state.
type Prepared struct {
	Attempt           attempt.Attempt
	Mode              tutor.Mode
	Config            tutor.Config
	Preview           tutor.Preview
	TextChat          *models.CapabilityState
	CredentialEnv     string
	CredentialPresent bool
}

// Preview resolves and builds one advisory request without sending it or
// claiming a budget slot: today's `tutor preview`.
func Preview(ctx context.Context, s *study.Service, r Request) (Prepared, error) {
	_ = ctx
	cfg, err := kernel.Configured(s, notConfigured)
	if err != nil {
		return Prepared{}, err
	}
	record, err := attemptRecord(s, r.AttemptID)
	if err != nil {
		return Prepared{}, err
	}
	cfg, _, _, err = kernel.ResolveActive(s, cfg)
	if err != nil {
		return Prepared{}, err
	}
	providerID := strings.TrimSpace(cfg.Provider)
	target, err := loadTarget(s, record, providerID)
	if err != nil {
		return Prepared{}, err
	}
	preview, err := tutor.Build(cfg, r.Mode, target)
	if err != nil {
		return Prepared{}, kernel.Refuse("invalid_input", "%s", err.Error())
	}
	env, _, present := kernel.Credential(s, cfg.Provider)
	prepared := Prepared{
		Attempt:           record,
		Mode:              r.Mode,
		Config:            cfg,
		Preview:           preview,
		CredentialEnv:     env,
		CredentialPresent: present,
	}
	if providerID != "" {
		index, err := kernel.LoadVerificationIndex(s)
		if err != nil {
			return Prepared{}, err
		}
		state := models.CapabilityStateOf(provider.CapTextChat, true, index.ForModel(providerID, cfg.Model))
		prepared.TextChat = &state
	}
	return prepared, nil
}

// ConsentChange is one consent decision: granted or revoked, for one source,
// optionally scoped to one provider.
type ConsentChange struct {
	SourceID string
	Provider string
	Granted  bool
	Reviewer string
}

// Consented is one recorded decision with the source it covers: the identity
// the CLI's confirmation prints.
type Consented struct {
	Event    store.TutorConsentEvent
	SourceID string
	Title    string
}

// Consent records one consent event: today's `tutor consent` and `tutor
// revoke`.
func Consent(ctx context.Context, s *study.Service, change ConsentChange) (Consented, error) {
	_ = ctx
	src, err := kernel.Store(s).Source(change.SourceID)
	if errors.Is(err, store.ErrNotFound) {
		return Consented{}, kernel.Refuse("unknown_source", "source %q is not imported", change.SourceID)
	}
	if err != nil {
		return Consented{}, err
	}
	event, err := kernel.Store(s).AppendTutorConsent(store.TutorConsentEvent{
		SourceID: src.ID,
		Provider: change.Provider,
		Granted:  change.Granted,
		Reviewer: change.Reviewer,
		At:       kernel.P(s).Now(),
	})
	if err != nil {
		return Consented{}, err
	}
	return Consented{Event: event, SourceID: src.ID, Title: src.Title}, nil
}

// attemptRecord is the stored attempt one entry point works on, or a refusal
// naming the attempt.
func attemptRecord(s *study.Service, id int64) (attempt.Attempt, error) {
	records, err := kernel.Store(s).List()
	if err != nil {
		return attempt.Attempt{}, err
	}
	if id < 0 {
		return attempt.Attempt{}, kernel.Refuse("invalid_input", "--attempt must be positive or 0 for the latest attempt")
	}
	if len(records) == 0 {
		return attempt.Attempt{}, kernel.Refuse("no_attempts", "no attempt is recorded; run passquest practice first")
	}
	if id == 0 {
		return records[len(records)-1], nil
	}
	for _, record := range records {
		if record.ID == id {
			return record, nil
		}
	}
	return attempt.Attempt{}, kernel.Refuse("unknown_attempt", "attempt #%d is not recorded", id)
}

// Limits is the disclosure every advisory document carries: what a model reply
// is, what one call sends and what it can never change. The CLI's tutor
// documents and the window's coach documents show the same statements.
func Limits() []string {
	return []string{
		"Model output is advisory study guidance; it is not a verified answer, grade, exam fact or readiness claim.",
		"One recorded attempt is sent per call. No attempt history, database path, source title, endpoint path or credential is sent.",
		"Raw prompt text is not stored: a call records the SHA-256 and byte size of the exact serialized request body plus scope flags. Run `passquest tutor preview` to inspect the exact text before sending.",
		"Private imported source text is withheld until a recorded `tutor consent` event for that source; first-party material needs no consent.",
		"A failed or malformed provider response is recorded as a failure and cannot append an attempt, change a grade or edit an answer key.",
		"The daily call budget counts failed calls and resets at 00:00 UTC. `tutor ask` retries at most once, only on another verified provider with the same recorded consent scope and remaining budget.",
	}
}
