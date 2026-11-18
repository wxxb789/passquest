package cli

import (
	"context"
	"flag"
	"fmt"

	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/notes"
)

func runNote(args []string, opt Options) int {
	if len(args) == 0 {
		return usageError(opt, jsonFlagRequested(args), "invalid_input",
			"note requires propose, preview, accept, skip, show, model, export or push")
	}
	switch args[0] {
	case "propose":
		return runNotePropose(args[1:], opt)
	case "preview":
		return runNotePreview(args[1:], opt)
	case "accept":
		return runNoteAccept(args[1:], opt)
	case "skip":
		return runNoteSkip(args[1:], opt)
	case "show":
		return runNoteShow(args[1:], opt)
	case "model":
		return runNoteModel(args[1:], opt)
	case "export":
		return runNoteExport(args[1:], opt)
	case "push":
		return runNotePush(args[1:], opt)
	default:
		return usageError(opt, jsonFlagRequested(args), "invalid_input", fmt.Sprintf("unknown note command %q", args[0]))
	}
}

func runNotePropose(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{
		"attempt": true, "content": true, "question": true, "mark": true, "db": true,
	})
	fs := newFlagSet("note propose", opt, wantJSON)
	attemptFlag := fs.Int64("attempt", 0, "attempt id; 0 selects the latest attempt")
	contentFlag := fs.String("content", "", "content id of a reviewed question")
	questionFlag := fs.String("question", "", "question id of a reviewed question")
	markFlag := fs.String("mark", "", "the learner's word or pattern to keep")
	refineFlag := fs.Bool("refine", false, "refine the draft once through the configured tutor endpoint")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	if (*contentFlag == "") != (*questionFlag == "") {
		return usageError(opt, *jsonFlag, "invalid_input", "--content and --question must be given together")
	}
	if *contentFlag != "" && *attemptFlag != 0 {
		return usageError(opt, *jsonFlag, "invalid_input", "--attempt cannot be combined with --content/--question")
	}
	mark := note.NormalizeMark(*markFlag)
	if mark != "" && !note.ValidMark(mark) {
		return usageError(opt, *jsonFlag, "invalid_mark",
			fmt.Sprintf("--mark must be 1..%d characters of ordinary text", note.MaxMarkRunes))
	}
	if *contentFlag != "" && mark == "" {
		return usageError(opt, *jsonFlag, "invalid_input",
			"a question without a stored attempt needs --mark naming the word or pattern worth keeping")
	}

	svc, err := study.Open(opt.ports(*dbFlag))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	effective, refinement, err := notes.Propose(context.Background(), svc, notes.ProposeInput{
		AttemptID: *attemptFlag, ContentID: *contentFlag, QuestionID: *questionFlag,
		Mark: *markFlag, Refine: *refineFlag,
	})
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	if refinement != nil && refinement.Failure != nil {
		return writeNoteRefineFailure(opt, *jsonFlag, *refinement.Failure)
	}
	refinementView := (*noteRefinementView)(nil)
	if refinement != nil {
		view := noteRefinementViewOf(refinement.Record)
		refinementView = &view
	}
	doc := noteDraftDoc{
		SchemaVersion: ProtocolVersion,
		State:         "draft",
		Draft:         noteDraftViewOf(effective),
		Fields:        noteFieldsViewOf(effective.Fields),
		Plain:         notePlainViewOf(effective),
		Advisory:      effective.Advisory,
		Refinement:    refinementView,
		Limits:        notes.Limits(),
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeNoteDraft(opt.Stdout, doc)
	return 0
}

// writeNoteRefineFailure reports one recorded refinement failure the way the
// refinement path always has: the error document on stderr — exit 3 — with the
// recorded code and message. The human report adds the reassurance only when
// the failure row was actually stored.
func writeNoteRefineFailure(opt Options, jsonMode bool, failure notes.Failure) int {
	if jsonMode {
		_ = writeJSONTo(opt.Stderr, errorDoc{SchemaVersion: ProtocolVersion, Error: errorBody{
			Code: failure.Code, Message: failure.Message,
		}})
	} else {
		fmt.Fprintf(opt.Stderr, "passquest: %s: %s\n", failure.Code, failure.Message)
		if failure.Recorded {
			fmt.Fprintln(opt.Stderr, "The deterministic draft remains open; run passquest note preview to inspect it.")
		}
	}
	return 3
}

func runNotePreview(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	fs := newFlagSet("note preview", opt, wantJSON)
	draftFlag := fs.Int64("draft", 0, "draft id; 0 selects the latest open draft")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	svc, err := study.Open(opt.ports(*dbFlag))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	effective, err := notes.Draft(context.Background(), svc, *draftFlag)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	doc := notePreviewDoc{
		SchemaVersion: ProtocolVersion,
		State:         "preview",
		Draft:         noteDraftViewOf(effective),
		Fields:        noteFieldsViewOf(effective.Fields),
		Plain:         notePlainViewOf(effective),
		HTMLSHA256:    effective.Fields.Hash(),
		Deck:          note.Deck,
		Model:         note.ModelName,
		Advisory:      effective.Advisory,
		Limits:        notes.Limits(),
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeNotePreview(opt.Stdout, doc)
	return 0
}

func runNoteAccept(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{
		"draft": true, "front": true, "back": true, "tactic": true, "db": true,
	})
	fs := newFlagSet("note accept", opt, wantJSON)
	draftFlag := fs.Int64("draft", 0, "draft id; 0 selects the latest open draft")
	frontFlag := fs.String("front", "", "edited front text")
	backFlag := fs.String("back", "", "edited back text (meaning, blank line, example)")
	tacticFlag := fs.String("tactic", "", "edited tactic text")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	frontSet, backSet, tacticSet := false, false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "front":
			frontSet = true
		case "back":
			backSet = true
		case "tactic":
			tacticSet = true
		}
	})
	edited := frontSet || backSet || tacticSet

	svc, err := study.Open(opt.ports(*dbFlag))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	effective, err := notes.Draft(context.Background(), svc, *draftFlag)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	if len(effective.Built.Withheld) > 0 {
		return usageError(opt, *jsonFlag, "consent_required", notes.ConsentMessage(effective.Draft))
	}
	front, back, tactic := effective.Front, effective.Back, effective.Tactic
	if frontSet {
		value, err := note.SanitizeFront(*frontFlag)
		if err != nil {
			return usageError(opt, *jsonFlag, "invalid_front", err.Error())
		}
		front = value
	}
	if backSet {
		value, err := note.SanitizeBack(*backFlag)
		if err != nil {
			return usageError(opt, *jsonFlag, "invalid_back", err.Error())
		}
		back = value
	}
	if tacticSet {
		value, err := note.SanitizeTactic(*tacticFlag)
		if err != nil {
			return usageError(opt, *jsonFlag, "invalid_tactic", err.Error())
		}
		tactic = value
	}
	origin := effective.Origin
	if edited {
		origin = note.OriginEdited
	}
	settled, err := notes.Settle(context.Background(), svc, notes.SettleInput{
		DraftID: effective.Draft.ID, Action: notes.Accept, Effective: effective,
		Front: front, Back: back, Tactic: tactic, Origin: origin,
	})
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	doc := noteAcceptDoc{
		SchemaVersion:    ProtocolVersion,
		State:            settled.State,
		Note:             noteViewOf(settled.Note),
		Fields:           noteFieldsViewOf(settled.Fields),
		Plain:            notePlainView{settled.Front, settled.Back, settled.Tactic, settled.SourceLabel},
		Created:          settled.Created,
		Changed:          settled.Changed,
		Limits:           notes.Limits(),
		PreviousRevision: settled.Previous,
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeNoteAccept(opt.Stdout, doc)
	return 0
}

func runNoteSkip(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	fs := newFlagSet("note skip", opt, wantJSON)
	draftFlag := fs.Int64("draft", 0, "draft id; 0 selects the latest open draft")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	svc, err := study.Open(opt.ports(*dbFlag))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	settled, err := notes.Settle(context.Background(), svc, notes.SettleInput{DraftID: *draftFlag, Action: notes.Skip})
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	draft := settled.Draft
	doc := noteSkipDoc{
		SchemaVersion: ProtocolVersion,
		State:         settled.State,
		DraftID:       draft.ID,
		ContentID:     draft.ContentID,
		QuestionID:    draft.QuestionID,
		Mark:          draft.Mark,
		Limits:        notes.Limits(),
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	fmt.Fprintf(opt.Stdout, "Skipped note draft #%d for %s / %s (no note was written).\n",
		doc.DraftID, doc.ContentID, doc.QuestionID)
	return 0
}

func runNoteShow(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"db": true})
	fs := newFlagSet("note show", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if err := fs.Parse(args); err != nil {
		if wantJSON {
			return usageError(opt, true, "invalid_flags", err.Error())
		}
		return 2
	}
	if fs.NArg() > 1 {
		return usageError(opt, *jsonFlag, "invalid_input", "note show accepts at most one note id")
	}
	noteID := ""
	if fs.NArg() == 1 {
		noteID = fs.Arg(0)
	}
	svc, err := study.Open(opt.ports(*dbFlag))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	listed, err := notes.List(context.Background(), svc, noteID)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	doc := noteShowDoc{SchemaVersion: ProtocolVersion, State: "show", Notes: []noteShowItem{}, Limits: notes.Limits()}
	for _, item := range listed {
		doc.Notes = append(doc.Notes, noteShowItem{Note: noteViewOf(item.Note), Revisions: noteRevisionViewsOf(item.Revisions)})
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeNoteShow(opt.Stdout, doc)
	return 0
}

func runNoteModel(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	fs := newFlagSet("note model", opt, wantJSON)
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	model := note.TemplateModel()
	sample := note.HTML(
		"closes at 4 p.m.",
		"To shut at a stated time.\n\nExample: The library closes at 4 p.m. on Saturdays.",
		"NOT / EXCEPT polarity",
		"src-example · p001 · q2",
	)
	doc := noteModelDoc{
		SchemaVersion: ProtocolVersion,
		State:         "model",
		Deck:          model.Deck,
		Model:         model.Name,
		ModelVersion:  model.Version,
		Fields:        model.Fields,
		CSS:           model.CSS,
		FrontTemplate: model.FrontTemplate,
		BackTemplate:  model.BackTemplate,
		SampleFields:  noteFieldsViewOf(sample),
		Rendered:      note.RenderCard(sample),
		Limits:        notes.Limits(),
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeNoteModel(opt.Stdout, doc)
	return 0
}
