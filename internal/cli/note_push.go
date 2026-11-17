package cli

import (
	"context"
	"fmt"

	"github.com/wxxb789/passquest/internal/anki"
	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/outbox"
)

// runNotePush sends every pending note to AnkiConnect and renders the push
// report: exit 0 with the report on stdout, exit 3 with the same document on
// stderr when the push failed or left a conflict, and exit 3 with an error
// document when it stopped before its report.
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
	svc, err := study.Open(opt.ports(*dbFlag))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	out, err := outbox.Push(context.Background(), svc, outbox.PushRequest{
		Endpoint: endpoint, TimeoutSeconds: *timeoutFlag, Force: *forceFlag,
	})
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	if out.Report == nil {
		return noteAnkiFailure(opt, *jsonFlag, out.Failure.Code, out.Failure.Message)
	}
	report := out.Report
	doc := notePushDoc{
		SchemaVersion:   ProtocolVersion,
		State:           report.State,
		Endpoint:        report.Endpoint,
		Deck:            note.Deck,
		Model:           note.ModelName,
		ModelCreated:    report.ModelCreated,
		ModelUpdated:    report.ModelUpdated,
		ModelCustomized: report.ModelCustomized,
		Counts: notePushCounts{
			Pending:        report.Counts.Pending,
			Pushed:         report.Counts.Pushed,
			AlreadyPresent: report.Counts.AlreadyPresent,
			Conflicts:      report.Counts.Conflicts,
			Failed:         report.Counts.Failed,
			Queued:         report.Counts.Queued,
			DeckMismatch:   report.Counts.DeckMismatch,
		},
		Notes:  []notePushItemView{},
		Limits: noteLimits,
	}
	for _, item := range report.Items {
		doc.Notes = append(doc.Notes, notePushItemView{
			NoteID: item.NoteID, Revision: item.Revision,
			AnkiNoteID: item.AnkiNoteID, State: item.State, Outcome: item.Outcome,
			Code: item.Code, Detail: item.Detail, Deck: item.Deck,
		})
	}
	if report.Error != nil {
		doc.Error = &errorBody{Code: report.Error.Code, Message: report.Error.Message}
	}
	if out.Empty {
		if *jsonFlag {
			return writeJSON(opt, doc)
		}
		fmt.Fprintln(opt.Stdout, "Pass Quest — Anki push")
		fmt.Fprintln(opt.Stdout, "Nothing is queued; no endpoint was contacted.")
		fmt.Fprintf(opt.Stdout, "%s\n", noteSyncStatement)
		return 0
	}
	// Error is exactly the report's exit-3 condition: the report sets its
	// error body whenever the push failed or left notes in conflict.
	exit := 0
	if report.Error != nil {
		exit = 3
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

// noteAnkiFailure reports a push that stopped before its report: the error
// document on stderr — exit 3 — with the code and message the push returned.
func noteAnkiFailure(opt Options, jsonMode bool, code, message string) int {
	if jsonMode {
		_ = writeJSONTo(opt.Stderr, errorDoc{SchemaVersion: ProtocolVersion, Error: errorBody{Code: code, Message: message}})
	} else {
		fmt.Fprintf(opt.Stderr, "passquest: %s: %s\n", code, message)
		fmt.Fprintln(opt.Stderr, "Pending notes stay queued; fix the problem and run passquest note push again.")
	}
	return 3
}
