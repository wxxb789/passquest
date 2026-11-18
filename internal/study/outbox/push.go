// Package outbox orchestrates the Anki outbox: one push of every pending note
// to the local AnkiConnect endpoint, and the consent policy export and push
// share. Results carry exactly the inputs the CLI's view builders and the
// window's studio mappers read; nothing here renders. The CLI's `note push`
// and the window's PushNotes are the two adapters.
package outbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/anki"
	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
)

// DefaultTimeoutSeconds is the AnkiConnect timeout of a push when nothing
// names one: the CLI's --timeout-seconds default and the window's push.
const DefaultTimeoutSeconds = 10

// Endpoint resolves the AnkiConnect endpoint a push (or the settings read)
// uses: the flag, then the environment, then the loopback default.
func Endpoint(flagValue string, getenv func(string) string) string {
	if strings.TrimSpace(flagValue) != "" {
		return flagValue
	}
	if value := strings.TrimSpace(getenv(anki.EndpointEnv)); value != "" {
		return value
	}
	return anki.DefaultEndpoint
}

// pushWork is one pending note's work item: the stored outbox entry, the local
// note row and the fields this push writes for it. Every classified outcome is
// recorded through persist.
type pushWork struct {
	Entry      store.NoteOutboxEntry
	Note       store.Note
	Fields     note.Fields
	Action     string
	Outcome    string
	Code       string
	Detail     string
	AnkiNoteID int64
	CardIDs    []int64
	Deck       string
	State      string
	Drain      bool
	Persisted  bool
}

// PushRequest is one push's inputs: the AnkiConnect endpoint the caller
// normalized, the host timeout in seconds and the explicit overwrite control.
type PushRequest struct {
	Endpoint       string
	TimeoutSeconds int
	Force          bool
}

// Failure is one push failure's body: the code and message the CLI prints as
// an error document and the window maps to a studio error.
type Failure struct {
	Code    string
	Message string
}

// Item is one note's report line, in work order. It carries exactly the fields
// the CLI's view builder and the window's studio item read.
type Item struct {
	NoteID     string
	Revision   int
	AnkiNoteID int64
	State      string
	Outcome    string
	Code       string
	Detail     string
	Deck       string
}

// Counts is the report's tally of item states, plus the pending count read
// back after the push.
type Counts struct {
	Pending        int
	Pushed         int
	AlreadyPresent int
	Conflicts      int
	Failed         int
	Queued         int
	DeckMismatch   int
}

// Report is one completed push report: the endpoint and model verdicts, the
// doc-level state, the counts, the per-note lines, and the error body of a
// failed push (nil when the push ended clean).
type Report struct {
	Endpoint        string
	ModelCreated    bool
	ModelUpdated    bool
	ModelCustomized bool
	State           string
	Counts          Counts
	Items           []Item
	Error           *Failure
}

// Pushed is the outcome of one push. Report is set for every push that reached
// the outbox — Empty says nothing was queued, so no endpoint was contacted. A
// push that stopped before its report (the endpoint or the host failed, or an
// outcome could not be recorded) sets Failure instead; the caller prints that
// failure alone.
type Pushed struct {
	Empty   bool
	Report  *Report
	Failure *Failure
}

// Push sends every pending note to the local AnkiConnect endpoint: today's
// `note push`, in today's order — capability pass, model upgrade, read-back,
// write, readd and deck verification. A withheld source is a refusal (the
// shared consent policy); a failure the CLI reports with exit 3 is a result.
func Push(ctx context.Context, s *study.Service, req PushRequest) (Pushed, error) {
	st := kernel.Store(s)
	p := kernel.P(s)
	endpoint := req.Endpoint
	entries, err := st.PendingNoteOutbox()
	if err != nil {
		return Pushed{}, err
	}
	doc := Report{
		State:    "idle",
		Endpoint: endpoint,
		Items:    []Item{},
	}
	if len(entries) == 0 {
		doc.Counts.Pending = 0
		return Pushed{Empty: true, Report: &doc}, nil
	}
	doc.Counts.Pending = len(entries)

	work := make([]pushWork, 0, len(entries))
	for _, entry := range entries {
		n, err := st.Note(entry.NoteID)
		if err != nil {
			work = append(work, pushWork{Entry: entry, Action: "check", State: store.AnkiStateFailed, Outcome: "failed",
				Detail: "local note row is missing"})
			continue
		}
		revision, err := st.NoteRevision(entry.NoteID, entry.Revision)
		if err != nil {
			work = append(work, pushWork{Entry: entry, Note: n, Action: "check", State: store.AnkiStateFailed, Outcome: "failed",
				Detail: "queued revision is missing"})
			continue
		}
		work = append(work, pushWork{
			Entry:  entry,
			Note:   n,
			Fields: note.HTML(revision.Front, revision.Back, revision.Tactic, revision.SourceLabel),
		})
	}

	pushNotes := make([]store.Note, 0, len(work))
	for i := range work {
		pushNotes = append(pushNotes, work[i].Note)
	}
	if message, denied := ConsentRefusal(st, pushNotes, "pushed"); denied {
		return Pushed{}, study.Refuse("consent_required", "%s", message)
	}

	timeoutMS := int64(req.TimeoutSeconds) * 1000
	call := func(actions []anki.Action) (anki.HostResponse, error) {
		return p.Anki.Call(ctx, anki.HostRequest{
			ProtocolVersion: anki.ProtocolVersion,
			Endpoint:        endpoint,
			TimeoutMS:       timeoutMS,
			Actions:         actions,
		})
	}
	// persist records every classified outcome immediately. Each completed host
	// batch is written before the next call, so a later transport or protocol
	// failure cannot discard an acknowledged note id or a conflict verdict.
	persist := func() error {
		for i := range work {
			item := &work[i]
			if item.State == "" || item.Persisted {
				continue
			}
			action := item.Action
			if action == "" {
				action = "check"
			}
			if action != "add" && action != "update" && action != "readd" && action != "check" {
				action = "check"
			}
			outcome := item.Outcome
			if outcome == "" {
				outcome = "failed"
			}
			attempt := store.AnkiAttempt{
				NoteID: item.Entry.NoteID, Revision: item.Entry.Revision, Action: action, Outcome: outcome,
				Detail: item.Detail, At: p.Now(), AnkiNoteID: item.AnkiNoteID, AnkiState: item.State,
				Drain: item.Drain,
			}
			if item.Drain {
				attempt.PushedRevision = item.Entry.Revision
			}
			if err := st.RecordAnkiAttempt(attempt); err != nil {
				return err
			}
			item.Persisted = true
		}
		return nil
	}
	// report builds the outcome from the work items: the per-note lines, the
	// counts, the doc-level state and the error body of a failed push, which is
	// also the decision the caller reads the exit code off.
	report := func(failure *Failure) Pushed {
		doc.Items = doc.Items[:0]
		for i := range work {
			item := &work[i]
			if item.State == "" {
				continue
			}
			entry := Item{
				NoteID: item.Entry.NoteID, Revision: item.Entry.Revision,
				AnkiNoteID: item.AnkiNoteID, State: item.State, Outcome: item.Outcome,
				Code: item.Code, Detail: item.Detail, Deck: item.Deck,
			}
			if entry.AnkiNoteID == 0 && item.Note.AnkiNoteID != 0 {
				entry.AnkiNoteID = item.Note.AnkiNoteID
			}
			doc.Items = append(doc.Items, entry)
		}
		counts := countsOf(doc.Items)
		doc.Counts.Pushed = counts.Pushed
		doc.Counts.AlreadyPresent = counts.AlreadyPresent
		doc.Counts.Conflicts = counts.Conflicts
		doc.Counts.Failed = counts.Failed
		doc.Counts.Queued = counts.Queued
		doc.Counts.DeckMismatch = counts.DeckMismatch
		remaining, err := st.PendingNoteOutbox()
		if err != nil {
			return failed("storage_error", err.Error())
		}
		doc.Counts.Pending = len(remaining)
		switch {
		case counts.Conflicts > 0:
			doc.State = "conflict"
		case counts.DeckMismatch > 0:
			doc.State = "deck_mismatch"
		case counts.Failed > 0:
			doc.State = "failed"
		case counts.Queued > 0 && counts.Pushed == 0 && counts.AlreadyPresent == 0:
			doc.State = "queued"
		case counts.Queued > 0:
			doc.State = "partial"
		case counts.Pushed > 0:
			doc.State = "pushed"
		case counts.AlreadyPresent > 0:
			doc.State = "already_present"
		default:
			doc.State = "idle"
		}
		exit := 0
		if failure != nil {
			doc.Error = failure
			exit = 3
		}
		if exit == 0 && (doc.State == "conflict" || doc.State == "failed" || doc.State == "deck_mismatch") {
			exit = 3
		}
		if exit != 0 && doc.Error == nil {
			switch doc.State {
			case "conflict":
				doc.Error = &Failure{Code: "anki_conflict", Message: fmt.Sprintf(
					"%d note(s) remain queued in conflict; nothing was overwritten", counts.Conflicts)}
			case "deck_mismatch":
				doc.Error = &Failure{Code: "anki_deck_mismatch", Message: fmt.Sprintf(
					"%d note(s) landed in a deck other than %q; their Anki note ids are kept", counts.DeckMismatch, note.Deck)}
			default:
				doc.Error = &Failure{Code: "anki_failed", Message: fmt.Sprintf(
					"%d note(s) failed and remain queued", counts.Failed)}
			}
		}
		return Pushed{Report: &doc}
	}
	// unreachable marks every unclassified note as queued before reporting. It
	// is used when the endpoint did not answer or the host call failed after an
	// earlier batch was already persisted.
	unreachable := func(failure *Failure, detail string) Pushed {
		if err := markUnreachable(work, st, p.Now, detail); err != nil {
			return failed("storage_error", err.Error())
		}
		return report(failure)
	}

	// Capability pass. A transport error means Node or the host is missing;
	// nothing was contacted and the queue is untouched.
	capabilities, err := call([]anki.Action{
		{Action: "version"},
		{Action: "createDeck", Params: anki.CreateDeckParams(note.Deck)},
		{Action: "modelNames"},
	})
	if err != nil {
		return failed("anki_unavailable", err.Error()), nil
	}
	if anki.IsUnreachable(capabilities) {
		return unreachable(nil, "endpoint did not answer"), nil
	}
	version, err := actionResultInt(capabilities, 0)
	if err != nil {
		return failed("anki_error", err.Error()), nil
	}
	if version < anki.MinAPIVersion {
		return failed("anki_protocol",
			fmt.Sprintf("AnkiConnect version %d is older than the required %d", version, anki.MinAPIVersion)), nil
	}
	if err := resultOK(capabilities, 1, "createDeck"); err != nil {
		return failed("anki_error", err.Error()), nil
	}
	if err := resultOK(capabilities, 2, "modelNames"); err != nil {
		return failed("anki_error", err.Error()), nil
	}
	modelNames, err := anki.DecodeStringList(capabilities.Results[2].Result)
	if err != nil {
		return failed("anki_error", err.Error()), nil
	}
	if !containsString(modelNames, note.ModelName) {
		model := note.TemplateModel()
		created, err := call([]anki.Action{{Action: "createModel", Params: anki.CreateModelParams(anki.ModelParams{
			Name: model.Name, Fields: model.Fields, CSS: model.CSS,
			FrontTemplate: model.FrontTemplate, BackTemplate: model.BackTemplate,
		})}})
		if err != nil {
			return failed("anki_unavailable", err.Error()), nil
		}
		if anki.IsUnreachable(created) {
			return unreachable(nil, "endpoint did not answer while creating the model"), nil
		}
		if err := resultOK(created, 0, "createModel"); err != nil {
			return failed("anki_error", err.Error()), nil
		}
		doc.ModelCreated = true
	} else if out, stop := upgradeModel(call, &doc, unreachable); stop {
		return out, nil
	}

	// Read back the cards we already pushed so a learner edit is a conflict,
	// not a silent overwrite. --force is the explicit overwrite control.
	if !req.Force {
		var ids []int64
		for _, item := range work {
			if item.Note.AnkiNoteID > 0 && item.State == "" {
				ids = append(ids, item.Note.AnkiNoteID)
			}
		}
		if len(ids) > 0 {
			// A note's fields are bounded by the note package limits, so this
			// action count keeps one response well under the host's 1 MiB cap.
			byID := make(map[int64]anki.NoteInfo, len(ids))
			for start := 0; start < len(ids); start += anki.MaxActionsPerCall {
				end := min(start+anki.MaxActionsPerCall, len(ids))
				readback, err := call([]anki.Action{{Action: "notesInfo", Params: anki.NotesInfoParams(ids[start:end])}})
				if err != nil {
					return unreachable(&Failure{Code: "anki_unavailable", Message: err.Error()}, err.Error()), nil
				}
				if anki.IsUnreachable(readback) {
					return unreachable(nil, "endpoint did not answer during read-back"), nil
				}
				if err := resultOK(readback, 0, "notesInfo"); err != nil {
					return unreachable(&Failure{Code: "anki_error", Message: err.Error()}, err.Error()), nil
				}
				infos, err := anki.DecodeNotesInfo(readback.Results[0].Result)
				if err != nil {
					return unreachable(&Failure{Code: "anki_error", Message: err.Error()}, err.Error()), nil
				}
				for _, info := range infos {
					byID[info.NoteID] = info
				}
			}
			for i := range work {
				item := &work[i]
				if item.Note.AnkiNoteID == 0 || item.State != "" {
					continue
				}
				info, ok := byID[item.Note.AnkiNoteID]
				if !ok {
					// The card is gone. Record the miss before the readd so the
					// ledger shows why a new id was written.
					if err := st.RecordAnkiAttempt(store.AnkiAttempt{
						NoteID: item.Entry.NoteID, Revision: item.Entry.Revision, Action: "check", Outcome: "not_found",
						Detail: "note was missing in Anki", At: p.Now(),
						AnkiNoteID: item.Note.AnkiNoteID, AnkiState: store.AnkiStateQueued,
					}); err != nil {
						return failed("storage_error", err.Error()), nil
					}
					item.Action = "readd"
					continue
				}
				item.CardIDs = info.Cards
				if info.ModelName != "" && info.ModelName != note.ModelName {
					item.State = store.AnkiStateConflict
					item.Outcome = "conflict"
					item.Code = "model_mismatch"
					item.Detail = "Anki note uses model " + info.ModelName
					continue
				}
				expected, err := pushedFields(st, item.Note)
				if err != nil {
					item.State = store.AnkiStateConflict
					item.Outcome = "conflict"
					item.Code = "revision_unavailable"
					item.Detail = "recorded pushed revision is unavailable"
					continue
				}
				if !fieldsMatch(info, expected) {
					item.State = store.AnkiStateConflict
					item.Outcome = "conflict"
					item.Code = "fields_changed"
					item.Detail = "Anki fields changed since the last push; use --force to overwrite"
					continue
				}
				item.Action = "update"
			}
			if err := persist(); err != nil {
				return failed("storage_error", err.Error()), nil
			}
		}
	}

	if req.Force {
		for i := range work {
			if work[i].Note.AnkiNoteID > 0 && work[i].State == "" {
				work[i].Action = "update"
			}
		}
	}

	// Build the write batch in work order.
	type writeRef struct {
		index  int
		action string
	}
	var writes []writeRef
	var actions []anki.Action
	for i := range work {
		item := &work[i]
		if item.State != "" {
			continue
		}
		if item.Action == "" {
			item.Action = "add"
		}
		switch item.Action {
		case "add", "readd":
			writes = append(writes, writeRef{index: i, action: item.Action})
			actions = append(actions, anki.Action{Action: "addNote", Params: anki.AddNoteParams(
				note.Deck, note.ModelName, item.Fields.Map(), []string{note.Tag})})
		case "update":
			writes = append(writes, writeRef{index: i, action: "update"})
			actions = append(actions, anki.Action{Action: "updateNoteFields", Params: anki.UpdateNoteParams(
				item.Note.AnkiNoteID, item.Fields.Map())})
		default:
			item.State = store.AnkiStateFailed
			item.Outcome = "failed"
			item.Detail = "no Anki action was selected"
		}
	}
	if len(actions) > 0 {
		var readdIndexes []int
		for start := 0; start < len(writes); start += anki.MaxActionsPerCall {
			end := min(start+anki.MaxActionsPerCall, len(writes))
			response, err := call(actions[start:end])
			if err != nil {
				return unreachable(&Failure{Code: "anki_unavailable", Message: err.Error()}, err.Error()), nil
			}
			for position, ref := range writes[start:end] {
				item := &work[ref.index]
				if position >= len(response.Results) {
					item.State = store.AnkiStateFailed
					item.Outcome = "failed"
					item.Detail = "AnkiConnect returned fewer results than actions"
					continue
				}
				if classifyWriteResult(item, response.Results[position], ref.action, false) {
					if err := st.RecordAnkiAttempt(store.AnkiAttempt{
						NoteID: item.Entry.NoteID, Revision: item.Entry.Revision, Action: "update", Outcome: "not_found",
						Detail: "note was missing in Anki", At: p.Now(), AnkiState: store.AnkiStateQueued,
					}); err != nil {
						return failed("storage_error", err.Error()), nil
					}
					item.CardIDs = nil
					readdIndexes = append(readdIndexes, ref.index)
				}
			}
			if err := persist(); err != nil {
				return failed("storage_error", err.Error()), nil
			}
		}
		// A card deleted after the read-back is re-added once; there is no
		// review history left to preserve.
		if len(readdIndexes) > 0 {
			var readdActions []anki.Action
			var readdRefs []int
			for _, index := range readdIndexes {
				readdRefs = append(readdRefs, index)
				readdActions = append(readdActions, anki.Action{Action: "addNote", Params: anki.AddNoteParams(
					note.Deck, note.ModelName, work[index].Fields.Map(), []string{note.Tag})})
			}
			for start := 0; start < len(readdActions); start += anki.MaxActionsPerCall {
				end := min(start+anki.MaxActionsPerCall, len(readdActions))
				readd, err := call(readdActions[start:end])
				if err != nil {
					return unreachable(&Failure{Code: "anki_unavailable", Message: err.Error()}, err.Error()), nil
				}
				for position := start; position < end; position++ {
					item := &work[readdRefs[position]]
					item.Action = "readd"
					if position-start >= len(readd.Results) {
						item.State = store.AnkiStateFailed
						item.Outcome = "failed"
						item.Detail = "AnkiConnect returned fewer results than actions"
						continue
					}
					classifyWriteResult(item, readd.Results[position-start], "readd", true)
				}
				if err := persist(); err != nil {
					return failed("storage_error", err.Error()), nil
				}
			}
		}
	}

	// Deck verification. AnkiConnect's addNote is known to ignore deckName on
	// some builds, so every note this push wrote is read back.
	deckFailed := func(code, detail string) Pushed {
		if err := persist(); err != nil {
			return failed("storage_error", err.Error())
		}
		return report(&Failure{Code: code, Message: fmt.Sprintf(
			"the deck read-back failed after the write was acknowledged: %s; no note records a deck", detail)})
	}
	var verified []int
	var askNotes []int64
	for i := range work {
		item := &work[i]
		if item.State == store.AnkiStatePushed && item.Outcome == "ok" && item.AnkiNoteID > 0 {
			verified = append(verified, i)
			if len(item.CardIDs) == 0 {
				askNotes = append(askNotes, item.AnkiNoteID)
			}
		}
	}
	if len(verified) > 0 {
		if len(askNotes) > 0 {
			cards := make(map[int64][]int64, len(askNotes))
			for start := 0; start < len(askNotes); start += anki.MaxActionsPerCall {
				end := min(start+anki.MaxActionsPerCall, len(askNotes))
				read, err := call([]anki.Action{{Action: "notesInfo", Params: anki.NotesInfoParams(askNotes[start:end])}})
				if err != nil {
					return deckFailed("anki_unavailable", err.Error()), nil
				}
				if anki.IsUnreachable(read) {
					return deckFailed("anki_unavailable", "the endpoint did not answer"), nil
				}
				if err := resultOK(read, 0, "notesInfo"); err != nil {
					return deckFailed("anki_error", err.Error()), nil
				}
				infos, err := anki.DecodeNotesInfo(read.Results[0].Result)
				if err != nil {
					return deckFailed("anki_error", err.Error()), nil
				}
				for _, info := range infos {
					cards[info.NoteID] = info.Cards
				}
			}
			for _, index := range verified {
				if len(work[index].CardIDs) == 0 {
					work[index].CardIDs = cards[work[index].AnkiNoteID]
				}
			}
		}
		// The card rows are attributed by the card id this push asked about, so
		// a row about anything else cannot become this note's deck claim.
		owner := make(map[int64]int, len(verified))
		var cardIDs []int64
		for _, index := range verified {
			for _, id := range work[index].CardIDs {
				if _, seen := owner[id]; !seen {
					owner[id] = index
					cardIDs = append(cardIDs, id)
				}
			}
		}
		rows := make(map[int][]anki.CardInfo, len(verified))
		for start := 0; start < len(cardIDs); start += anki.MaxActionsPerCall {
			end := min(start+anki.MaxActionsPerCall, len(cardIDs))
			read, err := call([]anki.Action{{Action: "cardsInfo", Params: anki.CardsInfoParams(cardIDs[start:end])}})
			if err != nil {
				return deckFailed("anki_unavailable", err.Error()), nil
			}
			if anki.IsUnreachable(read) {
				return deckFailed("anki_unavailable", "the endpoint did not answer"), nil
			}
			if err := resultOK(read, 0, "cardsInfo"); err != nil {
				return deckFailed("anki_error", err.Error()), nil
			}
			cards, err := anki.DecodeCardsInfo(read.Results[0].Result)
			if err != nil {
				return deckFailed("anki_error", err.Error()), nil
			}
			for _, card := range cards {
				if index, ok := owner[card.CardID]; ok {
					rows[index] = append(rows[index], card)
				}
			}
		}
		for _, index := range verified {
			item := &work[index]
			verdict := readNoteDecks(rows[index], item.CardIDs, note.Deck)
			// SetDeck: this is the note's whole deck claim, so an unconfirmed
			// verdict must clear a name an earlier read recorded instead of
			// leaving it to contradict the deck_mismatch state.
			attempt := store.AnkiAttempt{
				NoteID: item.Entry.NoteID, Revision: item.Entry.Revision, Action: "check",
				Outcome: "ok", At: p.Now(), AnkiDeck: note.Deck, SetDeck: true, AnkiState: store.AnkiStatePushed,
				Detail: fmt.Sprintf("card is in %q", note.Deck), Keep: true,
			}
			if !verdict.Matched {
				item.State = store.AnkiStateDeckMismatch
				item.Outcome = "deck_mismatch"
				item.Deck = verdict.Deck
				item.Detail = verdict.Detail
				attempt.Outcome = "deck_mismatch"
				attempt.AnkiDeck = verdict.Deck
				attempt.AnkiState = store.AnkiStateDeckMismatch
				attempt.Detail = verdict.Detail
			}
			if err := st.RecordAnkiAttempt(attempt); err != nil {
				return failed("storage_error", err.Error()), nil
			}
		}
	}

	// A storage failure leaves the outbox pending, so the next push retries
	// safely.
	if err := persist(); err != nil {
		return failed("storage_error", err.Error()), nil
	}
	return report(nil), nil
}

// failed builds the outcome of a push that stopped before its report: the code
// and message today's noteAnkiFailure printed as an error document. The caller
// renders them; the message names what keeps the notes queued.
func failed(code, message string) Pushed {
	return Pushed{Failure: &Failure{Code: code, Message: message}}
}

// upgradeModel reads the existing model's stylesheet and card template and
// brings a known earlier Pass Quest version up to this build's design in
// place (Q-0012-02). A model the learner changed in Anki is reported and never
// overwritten; its notes are still pushed, because fields, field order, model
// name and template name are unchanged. stop is true when the push ends here,
// with that outcome.
func upgradeModel(call func([]anki.Action) (anki.HostResponse, error), doc *Report,
	unreachable func(*Failure, string) Pushed) (out Pushed, stop bool) {
	read, err := call([]anki.Action{
		{Action: "modelStyling", Params: anki.ModelNameParams(note.ModelName)},
		{Action: "modelTemplates", Params: anki.ModelNameParams(note.ModelName)},
	})
	if err != nil {
		return failed("anki_unavailable", err.Error()), true
	}
	if anki.IsUnreachable(read) {
		return unreachable(nil, "endpoint did not answer while reading the model"), true
	}
	if err := resultOK(read, 0, "modelStyling"); err != nil {
		return failed("anki_error", err.Error()), true
	}
	if err := resultOK(read, 1, "modelTemplates"); err != nil {
		return failed("anki_error", err.Error()), true
	}
	css, err := anki.DecodeModelStyling(read.Results[0].Result)
	if err != nil {
		return failed("anki_error", err.Error()), true
	}
	templates, err := anki.DecodeModelTemplates(read.Results[1].Result)
	if err != nil {
		return failed("anki_error", err.Error()), true
	}
	// A second card type or a renamed template is the learner's design too.
	state := note.ModelCustomized
	if template, ok := templates[anki.TemplateName]; ok && len(templates) == 1 {
		state = note.ClassifyModel(css, template.Front, template.Back)
	}
	switch state {
	case note.ModelCurrent:
		return Pushed{}, false
	case note.ModelCustomized:
		doc.ModelCustomized = true
		return Pushed{}, false
	}
	model := note.TemplateModel()
	updated, err := call([]anki.Action{
		{Action: "updateModelTemplates", Params: anki.UpdateModelTemplatesParams(model.Name, model.FrontTemplate, model.BackTemplate)},
		{Action: "updateModelStyling", Params: anki.UpdateModelStylingParams(model.Name, model.CSS)},
	})
	if err != nil {
		return failed("anki_unavailable", err.Error()), true
	}
	if anki.IsUnreachable(updated) {
		return unreachable(nil, "endpoint did not answer while updating the model"), true
	}
	if err := resultOK(updated, 0, "updateModelTemplates"); err != nil {
		return failed("anki_error", err.Error()), true
	}
	if err := resultOK(updated, 1, "updateModelStyling"); err != nil {
		return failed("anki_error", err.Error()), true
	}
	doc.ModelUpdated = true
	return Pushed{}, false
}

// markUnreachable records one queued attempt per unclassified work item, so an
// endpoint that never answered leaves every note queued for the next push.
func markUnreachable(work []pushWork, st *store.Store, now func() time.Time, detail string) error {
	for i := range work {
		item := &work[i]
		if item.State != "" {
			continue
		}
		item.State = store.AnkiStateQueued
		item.Outcome = "unreachable"
		item.Detail = detail
		action := item.Action
		if action != "add" && action != "update" && action != "readd" {
			action = "check"
		}
		if err := st.RecordAnkiAttempt(store.AnkiAttempt{
			NoteID: item.Entry.NoteID, Revision: item.Entry.Revision, Action: action, Outcome: "unreachable",
			Detail: detail, At: now(), AnkiState: store.AnkiStateQueued,
		}); err != nil {
			return err
		}
		item.Persisted = true
	}
	return nil
}

// pushedFields reads the fields the last pushed revision recorded, so the
// read-back compares against what Anki actually holds.
func pushedFields(st *store.Store, n store.Note) (note.Fields, error) {
	if n.AnkiPushedRevision == 0 {
		return note.Fields{}, fmt.Errorf("no pushed revision")
	}
	revision, err := st.NoteRevision(n.NoteID, n.AnkiPushedRevision)
	if err != nil {
		return note.Fields{}, err
	}
	return note.HTML(revision.Front, revision.Back, revision.Tactic, revision.SourceLabel), nil
}

// fieldsMatch reports whether every field the push wrote still holds its
// trimmed value in Anki.
func fieldsMatch(info anki.NoteInfo, expected note.Fields) bool {
	for name, want := range expected.Map() {
		if strings.TrimSpace(info.FieldValue(name)) != strings.TrimSpace(want) {
			return false
		}
	}
	return true
}

// actionResultInt decodes one integer result, or reports the action's failure.
func actionResultInt(resp anki.HostResponse, index int) (int64, error) {
	if index < 0 || index >= len(resp.Results) {
		return 0, fmt.Errorf("anki: result %d is missing", index)
	}
	if err := resultOK(resp, index, resp.Results[index].Action); err != nil {
		return 0, err
	}
	return anki.DecodeInt(resp.Results[index].Result)
}

// resultOK turns one non-ok action result into the error the push prints.
func resultOK(resp anki.HostResponse, index int, action string) error {
	if index < 0 || index >= len(resp.Results) {
		return fmt.Errorf("anki: %s result is missing", action)
	}
	result := resp.Results[index]
	if result.State == "ok" {
		return nil
	}
	message := strings.TrimSpace(result.Error)
	if message == "" {
		message = "no error detail"
	}
	return fmt.Errorf("anki: %s failed: %s", action, message)
}

// countsOf tallies push item states.
func countsOf(items []Item) Counts {
	var counts Counts
	for _, item := range items {
		switch item.State {
		case store.AnkiStatePushed:
			counts.Pushed++
		case store.AnkiStateAlreadyExists:
			counts.AlreadyPresent++
		case store.AnkiStateConflict:
			counts.Conflicts++
		case store.AnkiStateFailed:
			counts.Failed++
		case store.AnkiStateQueued:
			counts.Queued++
		case store.AnkiStateDeckMismatch:
			counts.DeckMismatch++
		}
	}
	return counts
}
