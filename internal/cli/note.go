package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/wxxb789/passquest/internal/anki"
	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/tutor"
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

// noteEffective is the current content of one draft after consent and
// refinement are applied. Preview and accept both use it, so the payload a
// learner inspects is the payload that is written.
type noteEffective struct {
	Draft    store.NoteDraft
	Input    note.Input
	Built    note.Draft
	Front    string
	Back     string
	Tactic   string
	Origin   note.Origin
	Model    string
	Advisory bool
	Fields   note.Fields
}

func noteDraftFor(st *store.Store, id int64) (store.NoteDraft, error) {
	if id > 0 {
		draft, found, err := st.NoteDraftByID(id)
		if err != nil {
			return store.NoteDraft{}, err
		}
		if !found {
			return store.NoteDraft{}, refuse("unknown_draft", "note draft #%d is not recorded", id)
		}
		if draft.Status != store.NoteDraftOpen {
			return store.NoteDraft{}, refuse("draft_closed", "note draft #%d is already %s", id, draft.Status)
		}
		return draft, nil
	}
	draft, found, err := st.LatestOpenNoteDraft()
	if err != nil {
		return store.NoteDraft{}, err
	}
	if !found {
		return store.NoteDraft{}, refuse("no_draft", "no note draft is open; run passquest note propose first")
	}
	return draft, nil
}

// consentGranted reports whether the latest recorded global consent for a
// source is a grant. Note text and its Anki export are a different destination
// from provider inference, so a provider-scoped tutor grant never authorizes
// them; only the global decision does. A lookup error is treated as no consent.
func consentGranted(st *store.Store, sourceID string) bool {
	event, ok, err := st.TutorConsent(sourceID, "")
	return err == nil && ok && event.Granted
}

func noteInputFromDraft(st *store.Store, draft store.NoteDraft) (note.Input, error) {
	loaded, err := loadTutorContent(st, draft.ContentID, draft.QuestionID)
	if err != nil {
		return note.Input{}, err
	}
	if !loaded.Found {
		return note.Input{}, refuse("content_unavailable",
			"question %s of %s is not available in this build; a note needs the reviewed question and its evidence",
			draft.QuestionID, draft.ContentID)
	}
	if draft.ContentVersion != "" && loaded.Content.Version != draft.ContentVersion {
		return note.Input{}, refuse("content_changed",
			"the question changed after the draft was created; run passquest note propose again")
	}
	sourceID := loaded.SourceID
	if sourceID == "" {
		sourceID = draft.SourceID
	}
	var record *attempt.Attempt
	if draft.AttemptID > 0 {
		got, err := tutorAttemptRecord(st, draft.AttemptID)
		if err != nil {
			return note.Input{}, err
		}
		if got.ContentID != draft.ContentID || got.QuestionID != draft.QuestionID {
			return note.Input{}, refuse("content_unavailable",
				"attempt #%d belongs to %s / %s, not the draft's question",
				draft.AttemptID, got.ContentID, got.QuestionID)
		}
		record = &got
	}
	consent := loaded.Private && sourceID != "" && consentGranted(st, sourceID)
	return note.Input{
		Target: note.Target{
			ContentID:      draft.ContentID,
			ContentVersion: loaded.Content.Version,
			QuestionID:     draft.QuestionID,
			SourceID:       sourceID,
			Page:           draft.Page,
			AttemptID:      draft.AttemptID,
			Mark:           draft.Mark,
			Restricted:     loaded.Private,
		},
		Attempt:        record,
		Question:       &loaded.Question,
		PassageText:    loaded.Content.Passage.Text,
		ConsentGranted: consent,
	}, nil
}

func noteEffectiveFor(st *store.Store, draft store.NoteDraft) (noteEffective, error) {
	input, err := noteInputFromDraft(st, draft)
	if err != nil {
		return noteEffective{}, err
	}
	built, err := note.Build(input)
	if err != nil {
		return noteEffective{}, err
	}
	return noteEffectiveOf(draft, input, built), nil
}

// noteEffectiveOf applies a draft's refinement to an already built input, so a
// command that has just derived the draft does not derive it again.
func noteEffectiveOf(draft store.NoteDraft, input note.Input, built note.Draft) noteEffective {
	effective := noteEffective{
		Draft:  draft,
		Input:  input,
		Built:  built,
		Front:  built.Front,
		Back:   built.Back,
		Tactic: built.Tactic,
		Origin: note.OriginDeterministic,
	}
	// Accepted values are the base for a repeated or incremental accept, so an
	// edit is never replaced by the original proposal. A withheld draft keeps
	// the deterministic empty fields instead: revoked consent hides the text.
	if len(built.Withheld) == 0 && draft.AcceptedFront != "" {
		effective.Front = draft.AcceptedFront
		effective.Back = draft.AcceptedBack
		effective.Tactic = draft.AcceptedTactic
		switch origin := note.Origin(draft.AcceptedOrigin); origin {
		case note.OriginDeterministic, note.OriginRefined, note.OriginEdited:
			effective.Origin = origin
		}
		effective.Model = draft.AcceptedModel
		effective.Advisory = effective.Origin == note.OriginRefined
	} else {
		refined := draft.Origin == string(note.OriginRefined) &&
			strings.TrimSpace(draft.RefinedFront) != "" && strings.TrimSpace(draft.RefinedBack) != "" &&
			len(built.Withheld) == 0
		if refined && built.Unanswered && input.Question != nil &&
			note.RevealsAnswer(*input.Question, draft.RefinedFront+"\n"+draft.RefinedBack) {
			// A stale or hostile refinement must not put an unanswered key into
			// preview or accept; the deterministic draft is key-safe.
			refined = false
		}
		if refined {
			effective.Front = draft.RefinedFront
			effective.Back = draft.RefinedBack
			effective.Origin = note.OriginRefined
			effective.Model = draft.Model
			effective.Advisory = true
		}
	}
	effective.Fields = note.HTML(effective.Front, effective.Back, effective.Tactic, built.SourceLabel)
	return effective
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

	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()

	input, err := noteProposeInput(st, *attemptFlag, *contentFlag, *questionFlag, mark)
	if err != nil {
		return noteCommandError(opt, *jsonFlag, err)
	}
	built, err := note.Build(input)
	if err != nil {
		return noteCommandError(opt, *jsonFlag, err)
	}
	target := input.Target
	target.Mark = mark
	saved, err := st.SaveNoteDraft(store.NoteDraft{
		TargetKey:      target.Key(),
		AttemptID:      target.AttemptID,
		ContentID:      target.ContentID,
		ContentVersion: target.ContentVersion,
		QuestionID:     target.QuestionID,
		SourceID:       target.SourceID,
		Page:           target.Page,
		Mark:           mark,
		Restricted:     target.Restricted,
		Origin:         string(note.OriginDeterministic),
	}, opt.Now())
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	refinement := (*noteRefinementView)(nil)
	if *refineFlag {
		if len(built.Withheld) > 0 {
			return usageError(opt, *jsonFlag, "consent_required", noteConsentMessage(saved))
		}
		refinedDraft, view, code := noteRefine(st, saved, input, built, opt, *jsonFlag)
		if code != 0 {
			return code
		}
		saved = refinedDraft
		refinement = view
	}
	effective := noteEffectiveOf(saved, input, built)
	doc := noteDraftDoc{
		SchemaVersion: ProtocolVersion,
		State:         "draft",
		Draft:         noteDraftViewOf(effective),
		Fields:        noteFieldsViewOf(effective.Fields),
		Plain:         notePlainViewOf(effective),
		Advisory:      effective.Advisory,
		Refinement:    refinement,
		Limits:        noteLimits,
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeNoteDraft(opt.Stdout, doc)
	return 0
}

func noteProposeInput(st *store.Store, attemptFlag int64, contentFlag, questionFlag, mark string) (note.Input, error) {
	if contentFlag != "" {
		loaded, err := loadTutorContent(st, contentFlag, questionFlag)
		if err != nil {
			return note.Input{}, err
		}
		if !loaded.Found {
			return note.Input{}, refuse("not_practiceable",
				"question %s of %s is not a reviewed, practiceable question", questionFlag, contentFlag)
		}
		sourceID, page, _ := source.ParseContentID(contentFlag)
		return note.Input{
			Target: note.Target{
				ContentID:      contentFlag,
				ContentVersion: loaded.Content.Version,
				QuestionID:     questionFlag,
				SourceID:       sourceID,
				Page:           page,
				Mark:           mark,
				Restricted:     loaded.Private,
			},
			Question:       &loaded.Question,
			PassageText:    loaded.Content.Passage.Text,
			ConsentGranted: loaded.Private && loaded.SourceID != "" && consentGranted(st, loaded.SourceID),
		}, nil
	}

	record, err := tutorAttemptRecord(st, attemptFlag)
	if err != nil {
		return note.Input{}, err
	}
	loaded, err := loadTutorContent(st, record.ContentID, record.QuestionID)
	if err != nil {
		return note.Input{}, err
	}
	if !loaded.Found {
		return note.Input{}, refuse("content_unavailable",
			"the attempted question %s of %s is no longer available in this build", record.QuestionID, record.ContentID)
	}
	if loaded.Content.Version != record.ContentVersion {
		return note.Input{}, refuse("content_changed",
			"the question changed since attempt #%d; answer the current question again or propose from it with --mark", record.ID)
	}
	sourceID, page, _ := source.ParseContentID(record.ContentID)
	return note.Input{
		Target: note.Target{
			ContentID:      record.ContentID,
			ContentVersion: loaded.Content.Version,
			QuestionID:     record.QuestionID,
			SourceID:       sourceID,
			Page:           page,
			AttemptID:      record.ID,
			Mark:           mark,
			Restricted:     loaded.Private,
		},
		Attempt:        &record,
		Question:       &loaded.Question,
		PassageText:    loaded.Content.Passage.Text,
		ConsentGranted: loaded.Private && loaded.SourceID != "" && consentGranted(st, loaded.SourceID),
	}, nil
}

func noteConsentMessage(draft store.NoteDraft) string {
	return fmt.Sprintf(
		"source %s is private and its text is withheld; inspect the preview, then authorize with:\n  passquest tutor consent %s --reviewer NAME",
		draft.SourceID, draft.SourceID)
}

// noteRefineConsentMessage names the provider that the inference grant has to
// cover, so a provider-scoped denial is actionable rather than looking like a
// missing global grant.
func noteRefineConsentMessage(sourceID, providerID string) string {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return noteConsentMessage(store.NoteDraft{SourceID: sourceID})
	}
	return fmt.Sprintf(
		"source %s is private and its consent does not cover provider %s; authorize with:\n  passquest tutor consent %s --provider %s --reviewer NAME",
		sourceID, providerID, sourceID, providerID)
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
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	draft, err := noteDraftFor(st, *draftFlag)
	if err != nil {
		return noteCommandError(opt, *jsonFlag, err)
	}
	effective, err := noteEffectiveFor(st, draft)
	if err != nil {
		return noteCommandError(opt, *jsonFlag, err)
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
		Limits:        noteLimits,
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

	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	draft, err := noteDraftFor(st, *draftFlag)
	if err != nil {
		return noteCommandError(opt, *jsonFlag, err)
	}
	effective, err := noteEffectiveFor(st, draft)
	if err != nil {
		return noteCommandError(opt, *jsonFlag, err)
	}
	if len(effective.Built.Withheld) > 0 {
		return usageError(opt, *jsonFlag, "consent_required", noteConsentMessage(draft))
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
	target := store.NoteTarget{
		NoteID:         note.NoteID(draft.TargetKey),
		TargetKey:      draft.TargetKey,
		AttemptID:      draft.AttemptID,
		ContentID:      draft.ContentID,
		ContentVersion: effective.Input.Target.ContentVersion,
		QuestionID:     draft.QuestionID,
		SourceID:       draft.SourceID,
		Page:           draft.Page,
		Mark:           draft.Mark,
		Restricted:     effective.Built.Restricted,
	}
	content := store.NoteContent{
		Front:       front,
		Back:        back,
		Tactic:      tactic,
		SourceLabel: effective.Built.SourceLabel,
		ContentHash: note.ContentHash(front, back, tactic, effective.Built.SourceLabel, effective.Built.Restricted),
		Origin:      string(origin),
		Model:       effective.Model,
	}
	result, err := st.AcceptDraftNote(draft.ID, target, content, opt.Now())
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	fields := note.HTML(front, back, tactic, effective.Built.SourceLabel)
	state := "accepted"
	if !result.Changed {
		state = "unchanged"
	}
	doc := noteAcceptDoc{
		SchemaVersion:    ProtocolVersion,
		State:            state,
		Note:             noteViewOf(result.Note),
		Fields:           noteFieldsViewOf(fields),
		Plain:            notePlainView{front, back, tactic, effective.Built.SourceLabel},
		Created:          result.Created,
		Changed:          result.Changed,
		PreviousRevision: result.Previous,
		Limits:           noteLimits,
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
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	draft, err := noteDraftFor(st, *draftFlag)
	if err != nil {
		return noteCommandError(opt, *jsonFlag, err)
	}
	if err := st.SetNoteDraftStatus(draft.ID, store.NoteDraftSkipped); err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	doc := noteSkipDoc{
		SchemaVersion: ProtocolVersion,
		State:         "skipped",
		DraftID:       draft.ID,
		ContentID:     draft.ContentID,
		QuestionID:    draft.QuestionID,
		Mark:          draft.Mark,
		Limits:        noteLimits,
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
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	var notes []store.Note
	if fs.NArg() == 1 {
		n, err := st.Note(fs.Arg(0))
		if errors.Is(err, store.ErrNotFound) {
			return usageError(opt, *jsonFlag, "unknown_note", fmt.Sprintf("note %q is not recorded", fs.Arg(0)))
		}
		if err != nil {
			return internalError(opt, *jsonFlag, err)
		}
		notes = append(notes, n)
	} else {
		notes, err = st.Notes()
		if err != nil {
			return internalError(opt, *jsonFlag, err)
		}
	}
	doc := noteShowDoc{SchemaVersion: ProtocolVersion, State: "show", Notes: []noteShowItem{}, Limits: noteLimits}
	for _, n := range notes {
		revisions, err := st.NoteRevisions(n.NoteID)
		if err != nil {
			return internalError(opt, *jsonFlag, err)
		}
		doc.Notes = append(doc.Notes, noteShowItem{Note: noteViewOf(n), Revisions: noteRevisionViewsOf(revisions)})
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
		Fields:        model.Fields,
		CSS:           model.CSS,
		FrontTemplate: model.FrontTemplate,
		BackTemplate:  model.BackTemplate,
		SampleFields:  noteFieldsViewOf(sample),
		Rendered:      note.RenderCard(sample),
		Limits:        noteLimits,
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeNoteModel(opt.Stdout, doc)
	return 0
}

// noteRefine performs one explicit provider refinement and records its exact
// request identity. It returns the updated draft and the recorded refinement.
func noteRefine(st *store.Store, draft store.NoteDraft, input note.Input, deterministic note.Draft, opt Options, jsonMode bool) (store.NoteDraft, *noteRefinementView, int) {
	cfg, configured, err := st.TutorConfig()
	if err != nil {
		return draft, nil, internalError(opt, jsonMode, err)
	}
	if !configured {
		return draft, nil, usageError(opt, jsonMode, "not_configured",
			"no tutor provider is configured; run passquest tutor config --endpoint URL --model NAME, or omit --refine")
	}
	// Refinement is an inference destination, so it resolves the active pair
	// exactly like tutor ask: the registry endpoint replaces a stale saved one,
	// the pair must declare text.chat, and its evidence must be verified before
	// any text leaves the machine.
	cfg, registry, hasProvider, code := resolveActive(cfg, opt, jsonMode)
	if code != 0 {
		return draft, nil, code
	}
	if hasProvider {
		if code := requireVerifiedTextChat(st, registry, opt, cfg.Provider, cfg.Model, jsonMode); code != 0 {
			return draft, nil, code
		}
	}
	// The draft's note text and its Anki export stay on the global consent
	// decision, which is a different destination (see consentGranted). Sending
	// this source to a provider needs the consent that covers that provider: a
	// provider-scoped denial after a global grant overrides it.
	if input.Target.Restricted && input.Target.SourceID != "" {
		event, ok, err := st.TutorConsent(input.Target.SourceID, strings.TrimSpace(cfg.Provider))
		if err != nil {
			return draft, nil, internalError(opt, jsonMode, err)
		}
		if !ok || !event.Granted {
			return draft, nil, usageError(opt, jsonMode, "consent_required",
				noteRefineConsentMessage(input.Target.SourceID, cfg.Provider))
		}
	}
	env, key, present := provider.CredentialValue(opt.Getenv, cfg.Provider)
	if !present {
		return draft, nil, usageError(opt, jsonMode, "no_credential",
			fmt.Sprintf("%s is not set; export it in this environment (it is never stored)", env))
	}
	now := opt.Now()
	reservation, used, err := st.ReserveNoteRefinement(cfg.Provider, tutorDayStart(now), now, cfg.DailyCalls)
	if err != nil {
		return draft, nil, internalError(opt, jsonMode, err)
	}
	if reservation == 0 {
		return draft, nil, usageError(opt, jsonMode, "budget_exhausted",
			fmt.Sprintf("%d of %d provider calls are used today (UTC); raise the budget with passquest tutor config --daily-calls N",
				used, cfg.DailyCalls))
	}
	sendStarted := false
	defer func() {
		if !sendStarted {
			_ = st.ReleaseTutorCallReservation(reservation)
		}
	}()

	material := note.RefineMaterial{}
	if input.Question != nil {
		material.QuestionStem = input.Question.Stem
	}
	if deterministic.Unanswered {
		// The raw passage can contain the keyed option; send only the example
		// the deterministic draft already deemed pre-answer-visible.
		material.PassageText = deterministic.Example
	} else {
		material.PassageText = input.PassageText
		if input.Question != nil {
			material.Evidence = input.Question.Evidence
		}
	}
	request, err := note.BuildRefineRequest(cfg, deterministic, material)
	if err != nil {
		return draft, nil, usageError(opt, jsonMode, "invalid_config", err.Error())
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
	sendStarted = true
	started := opt.Now()
	response, transportErr := opt.TutorTransport.Call(context.Background(), request)
	record.LatencyMS = opt.Now().Sub(started).Milliseconds()
	if transportErr != nil {
		record.Status = "error"
		record.ErrorCode = "agent_unavailable"
		record.ErrorMessage = tutorMessage(transportErr.Error(), key)
		return noteRefineFailure(st, draft, record, reservation, opt, jsonMode)
	}
	record.HTTPStatus = response.HTTPStatus
	if response.Usage != nil {
		prompt, completion := response.Usage.PromptTokens, response.Usage.CompletionTokens
		record.PromptTokens = &prompt
		record.CompletionTokens = &completion
	}
	if response.Status == "error" {
		code := "host_error"
		message := "the tutor host returned no error detail"
		if response.Error != nil {
			if tutor.ValidHostErrorCode(response.Error.Code) {
				code = response.Error.Code
			}
			if strings.TrimSpace(response.Error.Message) != "" {
				message = response.Error.Message
			}
		}
		record.Status = "error"
		record.ErrorCode = code
		record.ErrorMessage = tutorMessage(message, key)
		return noteRefineFailure(st, draft, record, reservation, opt, jsonMode)
	}
	front, back, err := note.ParseRefinement(response.Text)
	if err != nil {
		record.Status = "error"
		record.ErrorCode = "invalid_response"
		record.ErrorMessage = tutorMessage(err.Error(), key)
		return noteRefineFailure(st, draft, record, reservation, opt, jsonMode)
	}
	if deterministic.Unanswered && input.Question != nil &&
		note.RevealsAnswer(*input.Question, front+"\n"+back) {
		record.Status = "error"
		record.ErrorCode = "invalid_response"
		record.ErrorMessage = "the refinement would reveal the answer of an unanswered question; the deterministic draft is unchanged"
		return noteRefineFailure(st, draft, record, reservation, opt, jsonMode)
	}
	record.Status = "ok"
	record.OutputFront = front
	record.OutputBack = back
	saved, err := st.FinalizeNoteRefinement(record, reservation)
	if err != nil {
		return draft, nil, internalError(opt, jsonMode, err)
	}
	if err := st.SetNoteDraftRefinement(draft.ID, saved.ID, cfg.Model, front, back); err != nil {
		return draft, nil, internalError(opt, jsonMode, err)
	}
	updated, found, err := st.NoteDraftByID(draft.ID)
	if err != nil {
		return draft, nil, internalError(opt, jsonMode, err)
	}
	if !found {
		return draft, nil, internalError(opt, jsonMode, errors.New("note draft disappeared after refinement"))
	}
	view := noteRefinementViewOf(saved)
	return updated, &view, 0
}

func noteRefineFailure(st *store.Store, draft store.NoteDraft, record store.NoteRefinement, reservation int64, opt Options, jsonMode bool) (store.NoteDraft, *noteRefinementView, int) {
	if _, err := st.FinalizeNoteRefinement(record, reservation); err != nil {
		// Recording the failure itself failed, so the reservation stays held:
		// the provider may already have been contacted and the deferred release
		// is disabled once sendStarted is true.
		if jsonMode {
			_ = writeJSONTo(opt.Stderr, errorDoc{SchemaVersion: ProtocolVersion, Error: errorBody{
				Code: record.ErrorCode, Message: record.ErrorMessage,
			}})
		} else {
			fmt.Fprintf(opt.Stderr, "passquest: %s: %s\n", record.ErrorCode, record.ErrorMessage)
		}
		return draft, nil, 3
	}
	if jsonMode {
		_ = writeJSONTo(opt.Stderr, errorDoc{SchemaVersion: ProtocolVersion, Error: errorBody{
			Code: record.ErrorCode, Message: record.ErrorMessage,
		}})
	} else {
		fmt.Fprintf(opt.Stderr, "passquest: %s: %s\n", record.ErrorCode, record.ErrorMessage)
		fmt.Fprintln(opt.Stderr, "The deterministic draft remains open; run passquest note preview to inspect it.")
	}
	return draft, nil, 3
}

func noteCommandError(opt Options, jsonMode bool, err error) int {
	if code, message, ok := noteErrorCode(err); ok {
		return usageError(opt, jsonMode, code, message)
	}
	return internalError(opt, jsonMode, err)
}

// noteErrorCode maps a refusal or a note derivation error to its usage code;
// ok is false for an internal error.
func noteErrorCode(err error) (code, message string, ok bool) {
	var refusal tacticRefusal
	if errors.As(err, &refusal) {
		return refusal.code, refusal.message, true
	}
	switch {
	case errors.Is(err, note.ErrLowValue):
		return "low_value", err.Error(), true
	case errors.Is(err, note.ErrUnansweredTarget):
		return "invalid_input", err.Error(), true
	case errors.Is(err, note.ErrQuestionUnavailable):
		return "content_unavailable", err.Error(), true
	}
	return "", "", false
}

// noteAnkiEndpoint is shared by push and the human documentation.
func noteAnkiEndpoint(flagValue string, getenv func(string) string) string {
	if strings.TrimSpace(flagValue) != "" {
		return flagValue
	}
	if value := strings.TrimSpace(getenv(anki.EndpointEnv)); value != "" {
		return value
	}
	return anki.DefaultEndpoint
}
