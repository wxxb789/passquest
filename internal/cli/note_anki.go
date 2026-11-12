package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/wxxb789/passquest/internal/anki"
	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/store"
)

func runNoteExport(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"out": true, "db": true})
	fs := newFlagSet("note export", opt, wantJSON)
	outFlag := fs.String("out", "", "output path; default <db dir>/passquest-notes.tsv")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	notes, err := st.Notes()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	if len(notes) == 0 {
		return usageError(opt, *jsonFlag, "no_notes", "no accepted note is recorded; run passquest note propose and passquest note accept first")
	}
	if message, denied := noteConsentRefusal(st, notes, "exported"); denied {
		return usageError(opt, *jsonFlag, "consent_required", message)
	}
	rows := make([]note.Fields, 0, len(notes))
	for _, n := range notes {
		rows = append(rows, note.HTML(n.Front, n.Back, n.Tactic, n.SourceLabel))
	}
	path := strings.TrimSpace(*outFlag)
	if path == "" {
		path = filepath.Join(filepath.Dir(st.Path()), "passquest-notes.tsv")
	}
	result, err := note.WriteExport(rows, path)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	doc := noteExportDoc{
		SchemaVersion: ProtocolVersion,
		State:         "exported",
		Path:          result.Path,
		Notes:         result.Notes,
		Bytes:         result.Bytes,
		SHA256:        result.SHA256,
		Header:        result.Header,
		Deck:          note.Deck,
		Model:         note.ModelName,
		Limits:        noteLimits,
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeNoteExport(opt.Stdout, doc)
	return 0
}

type notePushWork struct {
	Entry      store.NoteOutboxEntry
	Note       store.Note
	Fields     note.Fields
	Action     string
	Outcome    string
	Code       string
	Detail     string
	AnkiNoteID int64
	State      string
	Drain      bool
	Persisted  bool
}

func runNotePush(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"endpoint": true, "timeout-seconds": true, "db": true})
	fs := newFlagSet("note push", opt, wantJSON)
	endpointFlag := fs.String("endpoint", "", "AnkiConnect endpoint (loopback only)")
	timeoutFlag := fs.Int("timeout-seconds", 10, "AnkiConnect timeout in seconds (1-120)")
	forceFlag := fs.Bool("force", false, "overwrite a card whose Anki fields changed")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	if *timeoutFlag < 1 || *timeoutFlag > 120 {
		return usageError(opt, *jsonFlag, "invalid_input", "--timeout-seconds must be 1..120")
	}
	endpoint, err := anki.NormalizeEndpoint(noteAnkiEndpoint(*endpointFlag, opt.Getenv))
	if err != nil {
		return usageError(opt, *jsonFlag, "invalid_endpoint", err.Error())
	}
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	entries, err := st.PendingNoteOutbox()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	doc := notePushDoc{
		SchemaVersion: ProtocolVersion,
		State:         "idle",
		Endpoint:      endpoint,
		Deck:          note.Deck,
		Model:         note.ModelName,
		Notes:         []notePushItemView{},
		Limits:        noteLimits,
	}
	if len(entries) == 0 {
		doc.Counts.Pending = 0
		if *jsonFlag {
			return writeJSON(opt, doc)
		}
		fmt.Fprintln(opt.Stdout, "Pass Quest — Anki push")
		fmt.Fprintln(opt.Stdout, "Nothing is queued; no endpoint was contacted.")
		fmt.Fprintf(opt.Stdout, "%s\n", noteSyncStatement)
		return 0
	}
	doc.Counts.Pending = len(entries)

	work := make([]notePushWork, 0, len(entries))
	for _, entry := range entries {
		n, err := st.Note(entry.NoteID)
		if err != nil {
			work = append(work, notePushWork{Entry: entry, Action: "check", State: store.AnkiStateFailed, Outcome: "failed",
				Detail: "local note row is missing"})
			continue
		}
		revision, err := st.NoteRevision(entry.NoteID, entry.Revision)
		if err != nil {
			work = append(work, notePushWork{Entry: entry, Note: n, Action: "check", State: store.AnkiStateFailed, Outcome: "failed",
				Detail: "queued revision is missing"})
			continue
		}
		work = append(work, notePushWork{
			Entry:  entry,
			Note:   n,
			Fields: note.HTML(revision.Front, revision.Back, revision.Tactic, revision.SourceLabel),
		})
	}

	pushNotes := make([]store.Note, 0, len(work))
	for i := range work {
		pushNotes = append(pushNotes, work[i].Note)
	}
	if message, denied := noteConsentRefusal(st, pushNotes, "pushed"); denied {
		return usageError(opt, *jsonFlag, "consent_required", message)
	}

	timeoutMS := int64(*timeoutFlag) * 1000
	ctx := context.Background()
	call := func(actions []anki.Action) (anki.HostResponse, error) {
		return opt.AnkiTransport.Call(ctx, anki.HostRequest{
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
				Detail: item.Detail, At: opt.Now(), AnkiNoteID: item.AnkiNoteID, AnkiState: item.State,
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
	report := func(exit int, failure *errorBody) int {
		doc.Notes = doc.Notes[:0]
		for i := range work {
			item := &work[i]
			if item.State == "" {
				continue
			}
			view := notePushItemView{
				NoteID: item.Entry.NoteID, Revision: item.Entry.Revision,
				AnkiNoteID: item.AnkiNoteID, State: item.State, Outcome: item.Outcome,
				Code: item.Code, Detail: item.Detail,
			}
			if view.AnkiNoteID == 0 && item.Note.AnkiNoteID != 0 {
				view.AnkiNoteID = item.Note.AnkiNoteID
			}
			doc.Notes = append(doc.Notes, view)
		}
		counts := noteCountsOf(doc.Notes)
		doc.Counts.Pushed = counts.Pushed
		doc.Counts.AlreadyPresent = counts.AlreadyPresent
		doc.Counts.Conflicts = counts.Conflicts
		doc.Counts.Failed = counts.Failed
		doc.Counts.Queued = counts.Queued
		remaining, err := st.PendingNoteOutbox()
		if err != nil {
			return noteAnkiFailure(opt, *jsonFlag, "storage_error", err.Error())
		}
		doc.Counts.Pending = len(remaining)
		switch {
		case counts.Conflicts > 0:
			doc.State = "conflict"
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
		if failure != nil {
			doc.Error = failure
			exit = 3
		}
		if exit == 0 && (doc.State == "conflict" || doc.State == "failed") {
			exit = 3
		}
		if exit != 0 && doc.Error == nil {
			switch doc.State {
			case "conflict":
				doc.Error = &errorBody{Code: "anki_conflict", Message: fmt.Sprintf(
					"%d note(s) remain queued in conflict; nothing was overwritten", counts.Conflicts)}
			default:
				doc.Error = &errorBody{Code: "anki_failed", Message: fmt.Sprintf(
					"%d note(s) failed and remain queued", counts.Failed)}
			}
		}
		if *jsonFlag {
			if exit != 0 {
				_ = writeJSONTo(opt.Stderr, doc)
			} else {
				_ = writeJSONTo(opt.Stdout, doc)
			}
		} else {
			writer := opt.Stdout
			if exit != 0 {
				writer = opt.Stderr
			}
			writeNotePush(writer, doc)
		}
		return exit
	}
	// unreachable marks every unclassified note as queued before reporting. It
	// is used when the endpoint did not answer or the host call failed after an
	// earlier batch was already persisted.
	unreachable := func(exit int, failure *errorBody, detail string) int {
		if err := noteMarkUnreachable(work, st, opt, detail); err != nil {
			return noteAnkiFailure(opt, *jsonFlag, "storage_error", err.Error())
		}
		return report(exit, failure)
	}

	// Capability pass. A transport error means Node or the host is missing;
	// nothing was contacted and the queue is untouched.
	capabilities, err := call([]anki.Action{
		{Action: "version"},
		{Action: "createDeck", Params: anki.CreateDeckParams(note.Deck)},
		{Action: "modelNames"},
	})
	if err != nil {
		return noteAnkiFailure(opt, *jsonFlag, "anki_unavailable", err.Error())
	}
	if anki.IsUnreachable(capabilities) {
		return unreachable(0, nil, "endpoint did not answer")
	}
	version, err := noteActionResultInt(capabilities, 0)
	if err != nil {
		return noteAnkiFailure(opt, *jsonFlag, "anki_error", err.Error())
	}
	if version < anki.MinAPIVersion {
		return noteAnkiFailure(opt, *jsonFlag, "anki_protocol",
			fmt.Sprintf("AnkiConnect version %d is older than the required %d", version, anki.MinAPIVersion))
	}
	if err := noteResultOK(capabilities, 1, "createDeck"); err != nil {
		return noteAnkiFailure(opt, *jsonFlag, "anki_error", err.Error())
	}
	if err := noteResultOK(capabilities, 2, "modelNames"); err != nil {
		return noteAnkiFailure(opt, *jsonFlag, "anki_error", err.Error())
	}
	modelNames, err := anki.DecodeStringList(capabilities.Results[2].Result)
	if err != nil {
		return noteAnkiFailure(opt, *jsonFlag, "anki_error", err.Error())
	}
	if !containsString(modelNames, note.ModelName) {
		model := note.TemplateModel()
		created, err := call([]anki.Action{{Action: "createModel", Params: anki.CreateModelParams(anki.ModelParams{
			Name: model.Name, Fields: model.Fields, CSS: model.CSS,
			FrontTemplate: model.FrontTemplate, BackTemplate: model.BackTemplate,
		})}})
		if err != nil {
			return noteAnkiFailure(opt, *jsonFlag, "anki_unavailable", err.Error())
		}
		if anki.IsUnreachable(created) {
			return unreachable(0, nil, "endpoint did not answer while creating the model")
		}
		if err := noteResultOK(created, 0, "createModel"); err != nil {
			return noteAnkiFailure(opt, *jsonFlag, "anki_error", err.Error())
		}
		doc.ModelCreated = true
	}

	// Read back the cards we already pushed so a learner edit is a conflict,
	// not a silent overwrite. --force is the explicit overwrite control.
	if !*forceFlag {
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
					return unreachable(3, &errorBody{Code: "anki_unavailable", Message: err.Error()}, err.Error())
				}
				if anki.IsUnreachable(readback) {
					return unreachable(0, nil, "endpoint did not answer during read-back")
				}
				if err := noteResultOK(readback, 0, "notesInfo"); err != nil {
					return unreachable(3, &errorBody{Code: "anki_error", Message: err.Error()}, err.Error())
				}
				infos, err := anki.DecodeNotesInfo(readback.Results[0].Result)
				if err != nil {
					return unreachable(3, &errorBody{Code: "anki_error", Message: err.Error()}, err.Error())
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
						Detail: "note was missing in Anki", At: opt.Now(),
						AnkiNoteID: item.Note.AnkiNoteID, AnkiState: store.AnkiStateQueued,
					}); err != nil {
						return noteAnkiFailure(opt, *jsonFlag, "storage_error", err.Error())
					}
					item.Action = "readd"
					continue
				}
				if info.ModelName != "" && info.ModelName != note.ModelName {
					item.State = store.AnkiStateConflict
					item.Outcome = "conflict"
					item.Code = "model_mismatch"
					item.Detail = "Anki note uses model " + info.ModelName
					continue
				}
				expected, err := notePushedFields(st, item.Note)
				if err != nil {
					item.State = store.AnkiStateConflict
					item.Outcome = "conflict"
					item.Code = "revision_unavailable"
					item.Detail = "recorded pushed revision is unavailable"
					continue
				}
				if !noteFieldsMatch(info, expected) {
					item.State = store.AnkiStateConflict
					item.Outcome = "conflict"
					item.Code = "fields_changed"
					item.Detail = "Anki fields changed since the last push; use --force to overwrite"
					continue
				}
				item.Action = "update"
			}
			if err := persist(); err != nil {
				return noteAnkiFailure(opt, *jsonFlag, "storage_error", err.Error())
			}
		}
	}

	if *forceFlag {
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
				return unreachable(3, &errorBody{Code: "anki_unavailable", Message: err.Error()}, err.Error())
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
						Detail: "note was missing in Anki", At: opt.Now(), AnkiState: store.AnkiStateQueued,
					}); err != nil {
						return noteAnkiFailure(opt, *jsonFlag, "storage_error", err.Error())
					}
					readdIndexes = append(readdIndexes, ref.index)
				}
			}
			if err := persist(); err != nil {
				return noteAnkiFailure(opt, *jsonFlag, "storage_error", err.Error())
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
					return unreachable(3, &errorBody{Code: "anki_unavailable", Message: err.Error()}, err.Error())
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
					return noteAnkiFailure(opt, *jsonFlag, "storage_error", err.Error())
				}
			}
		}
	}

	// A storage failure leaves the outbox pending, so the next push retries
	// safely.
	if err := persist(); err != nil {
		return noteAnkiFailure(opt, *jsonFlag, "storage_error", err.Error())
	}
	return report(0, nil)
}

// classifyWriteResult applies one Anki write result to its work item. It
// returns true when an update found the note missing and the caller should
// retry that item once with addNote.
func classifyWriteResult(item *notePushWork, result anki.ActionResult, action string, readded bool) bool {
	if result.State == "unreachable" {
		item.State = store.AnkiStateQueued
		item.Outcome = "unreachable"
		item.Detail = result.Error
		return false
	}
	if result.State != "ok" {
		message := strings.TrimSpace(result.Error)
		if message == "" {
			message = "AnkiConnect returned an error"
		}
		switch {
		case !readded && action == "update" && anki.IsNotFoundError(message):
			item.Detail = "note was missing; re-adding"
			return true
		case readded && anki.IsDuplicateError(message):
			item.State = store.AnkiStateConflict
			item.Outcome = "conflict"
			item.Code = "note_id_unknown"
			item.Detail = "Anki already holds a note with this front and reports no note id; remove that card in Anki or align the local content, then push again"
		case anki.IsDuplicateError(message) && item.Note.AnkiPushedRevision == 0 && item.Entry.Revision == 1:
			item.State = store.AnkiStateAlreadyExists
			item.Outcome = "duplicate"
			item.Detail = "already present in Anki"
			item.Drain = true
			item.AnkiNoteID = 0
		case anki.IsDuplicateError(message):
			item.State = store.AnkiStateConflict
			item.Outcome = "conflict"
			item.Code = "note_id_unknown"
			item.Detail = "Anki already holds a note with this front and reports no note id; remove that card in Anki or align the local content, then push again"
		default:
			item.State = store.AnkiStateFailed
			item.Outcome = "failed"
			item.Detail = message
		}
		return false
	}
	if action == "update" {
		item.State = store.AnkiStatePushed
		item.Outcome = "ok"
		item.AnkiNoteID = item.Note.AnkiNoteID
		item.Drain = true
		return false
	}
	id, err := anki.DecodeInt(result.Result)
	if err != nil {
		item.State = store.AnkiStateFailed
		item.Outcome = "failed"
		item.Detail = err.Error()
		return false
	}
	item.State = store.AnkiStatePushed
	item.Outcome = "ok"
	if readded {
		item.Detail = "note was missing in Anki; re-added"
	}
	item.AnkiNoteID = id
	item.Drain = true
	return false
}

// noteConsentRefusal names restricted sources without a global grant. Export
// and push share this policy; verb states what was refused. A provider-scoped
// tutor grant authorizes inference, not this export destination.
func noteConsentRefusal(st *store.Store, notes []store.Note, verb string) (string, bool) {
	var denied []string
	for _, n := range notes {
		if n.Restricted && n.SourceID != "" && !consentGranted(st, n.SourceID) {
			denied = appendUnique(denied, n.SourceID)
		}
	}
	if len(denied) == 0 {
		return "", false
	}
	return fmt.Sprintf(
		"restricted note text from %s cannot be %s because consent is not recorded; run passquest tutor consent SOURCE --reviewer NAME",
		strings.Join(denied, ", "), verb), true
}

func noteMarkUnreachable(work []notePushWork, st *store.Store, opt Options, detail string) error {
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
			Detail: detail, At: opt.Now(), AnkiState: store.AnkiStateQueued,
		}); err != nil {
			return err
		}
		item.Persisted = true
	}
	return nil
}

func notePushedFields(st *store.Store, n store.Note) (note.Fields, error) {
	if n.AnkiPushedRevision == 0 {
		return note.Fields{}, fmt.Errorf("no pushed revision")
	}
	revision, err := st.NoteRevision(n.NoteID, n.AnkiPushedRevision)
	if err != nil {
		return note.Fields{}, err
	}
	return note.HTML(revision.Front, revision.Back, revision.Tactic, revision.SourceLabel), nil
}

func noteFieldsMatch(info anki.NoteInfo, expected note.Fields) bool {
	for name, want := range expected.Map() {
		if strings.TrimSpace(info.FieldValue(name)) != strings.TrimSpace(want) {
			return false
		}
	}
	return true
}

func noteActionResultInt(resp anki.HostResponse, index int) (int64, error) {
	if index < 0 || index >= len(resp.Results) {
		return 0, fmt.Errorf("anki: result %d is missing", index)
	}
	if err := noteResultOK(resp, index, resp.Results[index].Action); err != nil {
		return 0, err
	}
	return anki.DecodeInt(resp.Results[index].Result)
}

func noteResultOK(resp anki.HostResponse, index int, action string) error {
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

func noteAnkiFailure(opt Options, jsonMode bool, code, message string) int {
	if jsonMode {
		_ = writeJSONTo(opt.Stderr, errorDoc{SchemaVersion: ProtocolVersion, Error: errorBody{Code: code, Message: message}})
	} else {
		fmt.Fprintf(opt.Stderr, "passquest: %s: %s\n", code, message)
		fmt.Fprintln(opt.Stderr, "Pending notes stay queued; fix the problem and run passquest note push again.")
	}
	return 3
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func appendUnique(values []string, value string) []string {
	if !containsString(values, value) {
		values = append(values, value)
	}
	return values
}
