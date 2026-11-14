package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/adaptive"
	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
)

func runAssess(args []string, opt Options) int {
	if len(args) == 0 {
		return usageError(opt, jsonFlagRequested(args), "invalid_input",
			"assess requires pool, start, resume, abandon, report or compare")
	}
	switch args[0] {
	case "pool":
		return runAssessPool(args[1:], opt)
	case "start":
		return runAssessStart(args[1:], opt)
	case "resume":
		return runAssessResume(args[1:], opt)
	case "abandon":
		return runAssessAbandon(args[1:], opt)
	case "report":
		return runAssessReport(args[1:], opt)
	case "compare":
		return runAssessCompare(args[1:], opt)
	default:
		return usageError(opt, jsonFlagRequested(args), "invalid_input", fmt.Sprintf("unknown assess command %q", args[0]))
	}
}

// openAssessmentStore opens the selected database and reconciles expired runs.
// Every assess subcommand gets one write path for that reconcile.
func openAssessmentStore(dbFlag *string, opt Options, jsonMode bool) (*store.Store, int) {
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return nil, internalError(opt, jsonMode, err)
	}
	if _, err := st.ReconcileAssessmentRuns(opt.Now()); err != nil {
		st.Close()
		return nil, internalError(opt, jsonMode, err)
	}
	return st, 0
}

func runAssessPool(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"db": true})
	fs := newFlagSet("assess pool", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	st, code := openAssessmentStore(dbFlag, opt, *jsonFlag)
	if code != 0 {
		return code
	}
	defer st.Close()
	pool, identities, err := assessmentPool(st)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	doc := poolDocOf(pool, identities)
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writePool(opt.Stdout, doc, st.Path())
	return 0
}

func runAssessStart(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"size": true, "minutes": true, "annotations": true, "db": true})
	fs := newFlagSet("assess start", opt, wantJSON)
	sizeFlag := fs.Int("size", assess.DefaultSize, "target number of unseen items")
	minutesFlag := fs.Int("minutes", assess.DefaultMinutes, "whole-run time window in minutes")
	annotationsFlag := fs.String("annotations", assess.AnnotationsNone, "annotation mode; only none is accepted")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	if code := validateAssessmentFlags(*sizeFlag, *minutesFlag, *annotationsFlag, opt, *jsonFlag); code != 0 {
		return code
	}
	st, code := openAssessmentStore(dbFlag, opt, *jsonFlag)
	if code != 0 {
		return code
	}
	defer st.Close()
	created, items, err := createAssessmentRun(opt, st, *sizeFlag, *minutesFlag)
	if err != nil {
		return refusalOrInternal(opt, *jsonFlag, err)
	}
	return runAssessmentLoop(opt, st, created, items, *jsonFlag)
}

// createAssessmentRun selects unseen units and stores a new active run. The
// store must already be reconciled. Refusals carry the CLI error codes
// run_active, pool_floor, pool_short and invalid_input.
func createAssessmentRun(opt Options, st *store.Store, size, minutes int) (assess.Run, []assess.Item, error) {
	if active, found, err := st.ActiveAssessmentRun(); err != nil {
		return assess.Run{}, nil, err
	} else if found {
		return assess.Run{}, nil, refuse("run_active",
			"assessment run %d is still active; resume it with `passquest assess resume %d` or abandon it with `passquest assess abandon %d`",
			active.ID, active.ID, active.ID)
	}
	pool, identities, err := assessmentPool(st)
	if err != nil {
		return assess.Run{}, nil, err
	}
	if pool.ItemCount < assess.MinPoolItems {
		return assess.Run{}, nil, refuse("pool_floor", "%s", poolFloorMessage(pool))
	}
	if pool.ItemCount < size {
		return assess.Run{}, nil, refuse("pool_short",
			"requested %d item(s), but the unseen pool has %d item(s) on %d unit(s); %d more item(s) must be reviewed and imported",
			size, pool.ItemCount, pool.UnitCount, size-pool.ItemCount)
	}
	units := pool.Select(size)
	actual := assess.SelectedItems(units)
	if actual == 0 {
		return assess.Run{}, nil, refuse("pool_short", "no unseen assessment items are available")
	}
	perItem, ok := assess.PerItemTimeout(minutes, actual)
	if !ok {
		return assess.Run{}, nil, refuse("invalid_input", "--minutes %d and --size %d leave no per-item timeout", minutes, size)
	}
	now := opt.Now()
	run := assess.Run{
		CreatedAt:        now,
		StartedAt:        now,
		DeadlineAt:       now.Add(time.Duration(minutes) * time.Minute),
		WindowMinutes:    minutes,
		RequestedSize:    size,
		ActualSize:       actual,
		PerItemTimeoutMS: perItem,
		Annotations:      assess.AnnotationsNone,
		PoolFamily:       assess.PoolFamily(identities),
		SourceCount:      len(identities),
	}
	items := assessmentItems(units)
	created, err := st.CreateAssessmentRun(run, items)
	if errors.Is(err, store.ErrAssessmentActive) {
		return assess.Run{}, nil, refuse("run_active", "another assessment run is already active")
	}
	if err != nil {
		return assess.Run{}, nil, err
	}
	return created, items, nil
}

func runAssessResume(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"db": true})
	fs := newFlagSet("assess resume", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 1, "assess resume requires exactly one run id"); code != 0 {
		return code
	}
	runID, code := parseRunID(fs.Arg(0), opt, *jsonFlag)
	if code != 0 {
		return code
	}
	st, code := openAssessmentStore(dbFlag, opt, *jsonFlag)
	if code != 0 {
		return code
	}
	defer st.Close()
	run, items, completed, err := resumeAssessment(opt, st, runID)
	if err != nil {
		return assessmentError(opt, *jsonFlag, err)
	}
	if completed != nil {
		if *jsonFlag {
			return writeJSON(opt, *completed)
		}
		writeAssessmentSummary(opt.Stdout, *completed, st.Path())
		return 0
	}
	return runAssessmentLoop(opt, st, run, items, *jsonFlag)
}

// resumeAssessment counts one resume of an active run and returns it with its
// items. A completed run returns its result document instead; an abandoned or
// expired run is refused with run_abandoned.
func resumeAssessment(opt Options, st *store.Store, runID int64) (assess.Run, []assess.Item, *assessmentRunResultDoc, error) {
	run, items, err := st.AssessmentRunItems(runID)
	if err != nil {
		return assess.Run{}, nil, nil, err
	}
	if run.Status == assess.StatusAbandoned {
		return assess.Run{}, nil, nil, refuse("run_abandoned",
			"assessment run %d was abandoned (%s); its answered items remain readable with `passquest assess report %d`",
			run.ID, run.AbandonedReason, run.ID)
	}
	if run.Status == assess.StatusCompleted {
		doc, err := assessmentRunDocFor(st, run, items)
		if err != nil {
			return assess.Run{}, nil, nil, err
		}
		return run, items, &doc, nil
	}
	run, err = st.ResumeAssessmentRun(run.ID, opt.Now())
	if errors.Is(err, store.ErrAssessmentWindowElapsed) {
		return assess.Run{}, nil, nil, refuse("run_abandoned",
			"assessment run %d passed its deadline and was recorded abandoned; resume is refused", run.ID)
	}
	if err != nil {
		return assess.Run{}, nil, nil, err
	}
	return run, items, nil, nil
}

func runAssessAbandon(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"db": true})
	fs := newFlagSet("assess abandon", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 1, "assess abandon requires exactly one run id"); code != 0 {
		return code
	}
	runID, code := parseRunID(fs.Arg(0), opt, *jsonFlag)
	if code != 0 {
		return code
	}
	st, code := openAssessmentStore(dbFlag, opt, *jsonFlag)
	if code != 0 {
		return code
	}
	defer st.Close()
	run, err := abandonAssessment(opt, st, runID)
	if err != nil {
		return assessmentError(opt, *jsonFlag, err)
	}
	doc := abandonedDoc{SchemaVersion: ProtocolVersion, State: "abandoned", RunID: run.ID, Reason: run.AbandonedReason}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	fmt.Fprintf(opt.Stdout, "Assessment run %d abandoned (%s). Answered items remain readable with `passquest assess report %d`.\nDatabase: %s\n",
		run.ID, run.AbandonedReason, run.ID, st.Path())
	return 0
}

// abandonAssessment records the learner's abandonment of a run. A completed
// run is refused with run_completed.
func abandonAssessment(opt Options, st *store.Store, runID int64) (assess.Run, error) {
	run, err := st.AbandonAssessmentRun(runID, opt.Now(), "learner_abandoned")
	if errors.Is(err, store.ErrAssessmentRunState) {
		return assess.Run{}, refuse("run_completed",
			"assessment run %d is already completed; abandoning it would rewrite recorded evidence", runID)
	}
	return run, err
}

func runAssessReport(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"db": true})
	fs := newFlagSet("assess report", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 1, "assess report requires exactly one run id"); code != 0 {
		return code
	}
	runID, code := parseRunID(fs.Arg(0), opt, *jsonFlag)
	if code != 0 {
		return code
	}
	st, code := openAssessmentStore(dbFlag, opt, *jsonFlag)
	if code != 0 {
		return code
	}
	defer st.Close()
	report, err := assessmentReport(st, runID)
	if err != nil {
		return assessmentError(opt, *jsonFlag, err)
	}
	doc := reportDocOf(report)
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeAssessmentReport(opt.Stdout, doc, st.Path())
	return 0
}

func runAssessCompare(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"db": true})
	fs := newFlagSet("assess compare", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 2, "assess compare requires exactly two run ids"); code != 0 {
		return code
	}
	first, code := parseRunID(fs.Arg(0), opt, *jsonFlag)
	if code != 0 {
		return code
	}
	second, code := parseRunID(fs.Arg(1), opt, *jsonFlag)
	if code != 0 {
		return code
	}
	st, code := openAssessmentStore(dbFlag, opt, *jsonFlag)
	if code != 0 {
		return code
	}
	defer st.Close()
	doc, err := compareAssessmentRuns(st, first, second)
	if err != nil {
		return assessmentError(opt, *jsonFlag, err)
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeComparison(opt.Stdout, doc, st.Path())
	return 0
}

// compareAssessmentRuns builds both reports and their like-for-like
// comparison; incomparable runs are refused with incomparable_runs.
func compareAssessmentRuns(st *store.Store, first, second int64) (assessComparisonDoc, error) {
	material, err := loadAssessmentMaterial(st)
	if err != nil {
		return assessComparisonDoc{}, err
	}
	a, err := assessmentReportFrom(st, first, material)
	if err != nil {
		return assessComparisonDoc{}, err
	}
	b, err := assessmentReportFrom(st, second, material)
	if err != nil {
		return assessComparisonDoc{}, err
	}
	if err := assess.Compare(a, b); err != nil {
		var refusal *assess.CompareRefusal
		if errors.As(err, &refusal) {
			return assessComparisonDoc{}, refuse("incomparable_runs",
				"cannot compare runs %d and %d: %s", a.Run.ID, b.Run.ID, refusal.Error())
		}
		return assessComparisonDoc{}, err
	}
	return comparisonDocOf(a, b), nil
}

func validateAssessmentFlags(size, minutes int, annotations string, opt Options, jsonMode bool) int {
	if err := assessmentSettingsError(size, minutes, annotations); err != nil {
		return refusalOrInternal(opt, jsonMode, err)
	}
	return 0
}

// assessmentSettingsError refuses a run size, window or annotation mode
// outside the accepted ranges.
func assessmentSettingsError(size, minutes int, annotations string) error {
	if size < 1 || size > 10_000 {
		return refuse("invalid_input", "--size must be between 1 and 10000")
	}
	if minutes < 1 || minutes > 1440 {
		return refuse("invalid_input", "--minutes must be between 1 and 1440")
	}
	if annotations != assess.AnnotationsNone {
		return refuse("invalid_annotations",
			"--annotations %q is not supported; the only accepted value in this goal is none", annotations)
	}
	return nil
}

func parseRunID(raw string, opt Options, jsonMode bool) (int64, int) {
	id, err := parsePositiveInt64(raw)
	if err != nil {
		return 0, usageError(opt, jsonMode, "invalid_input", fmt.Sprintf("invalid assessment run id %q", raw))
	}
	return id, 0
}

func assessmentError(opt Options, jsonMode bool, err error) int {
	if code, ok := assessmentErrorCode(err); ok {
		return usageError(opt, jsonMode, code, err.Error())
	}
	return internalError(opt, jsonMode, err)
}

// assessmentErrorCode maps a refusal or an assessment store state error to
// its usage code; ok is false for an internal error.
func assessmentErrorCode(err error) (string, bool) {
	var refusal tacticRefusal
	switch {
	case errors.As(err, &refusal):
		return refusal.code, true
	case errors.Is(err, store.ErrNotFound):
		return "unknown_run", true
	case errors.Is(err, store.ErrAssessmentRunState):
		return "run_state", true
	case errors.Is(err, store.ErrAssessmentWindowElapsed):
		return "run_abandoned", true
	}
	return "", false
}

// assessmentPool builds every currently practiceable imported unit and the
// spend union. Title and passage stay in memory; documents carry ids only.
func assessmentPool(st *store.Store) (assess.Pool, []assess.SourceIdentity, error) {
	units, identities, err := assessmentUnits(st)
	if err != nil {
		return assess.Pool{}, nil, err
	}
	spends, err := st.Spends()
	if err != nil {
		return assess.Pool{}, nil, err
	}
	return assess.BuildPool(units, spends), identities, nil
}

func assessmentUnits(st *store.Store) ([]assess.Unit, []assess.SourceIdentity, error) {
	summaries, err := st.Sources()
	if err != nil {
		return nil, nil, err
	}
	identities := make([]assess.SourceIdentity, 0, len(summaries))
	units := make([]assess.Unit, 0)
	for _, summary := range summaries {
		identities = append(identities, assess.SourceIdentity{ID: summary.ID, SHA256: summary.SHA256})
		src, err := st.Source(summary.ID)
		if err != nil {
			return nil, nil, err
		}
		questions, err := st.Questions(summary.ID)
		if err != nil {
			return nil, nil, err
		}
		byPage := make(map[int][]source.Question, len(src.Pages))
		for _, question := range questions {
			byPage[question.Page] = append(byPage[question.Page], question)
		}
		for _, page := range src.Pages {
			content, _, err := source.BuildPracticeExam(src, page, byPage[page.Number])
			if err != nil {
				continue
			}
			units = append(units, assess.Unit{
				ContentID: content.ID,
				SourceID:  src.ID,
				Page:      page.Number,
				Version:   content.Version,
				Passage:   content.Passage.Text,
				Questions: content.Questions,
			})
		}
	}
	return units, identities, nil
}

func assessmentItems(units []assess.Unit) []assess.Item {
	var items []assess.Item
	position := 0
	for _, unit := range units {
		for _, question := range unit.Questions {
			items = append(items, assess.Item{
				Position:       position,
				ContentID:      unit.ContentID,
				SourceID:       unit.SourceID,
				Page:           unit.Page,
				ContentVersion: unit.Version,
				QuestionID:     question.ID,
				Skills:         append([]string(nil), question.Tactics...),
				State:          assess.StatePending,
			})
			position++
		}
	}
	return items
}

func poolShortfall(pool assess.Pool) int {
	shortfall := assess.MinPoolItems - pool.ItemCount
	if shortfall < 0 {
		shortfall = 0
	}
	return shortfall
}

func poolFloorMessage(pool assess.Pool) string {
	return fmt.Sprintf(
		"assessment pool has %d qualifying item(s) on %d unseen content unit(s); at least %d are required, so %d more reviewed item(s) must be imported (the unit count is the binding constraint on independent assessments)",
		pool.ItemCount, pool.UnitCount, assess.MinPoolItems, poolShortfall(pool))
}

func assessmentRunDocFor(st *store.Store, run assess.Run, items []assess.Item) (assessmentRunResultDoc, error) {
	attempts, err := st.AssessmentAttempts(run.ID)
	if err != nil {
		return assessmentRunResultDoc{}, err
	}
	return assessmentRunResultDoc{
		SchemaVersion: ProtocolVersion,
		State:         run.Status,
		Run:           runViewOf(run),
		Items:         itemOutcomeViews(items, attempts),
		Limitations:   assessmentRunLimitations(),
	}, nil
}

type assessmentMaterial struct {
	spends []assess.Spend
	units  []assess.Unit
}

func loadAssessmentMaterial(st *store.Store) (assessmentMaterial, error) {
	spends, err := st.Spends()
	if err != nil {
		return assessmentMaterial{}, err
	}
	units, _, err := assessmentUnits(st)
	if err != nil {
		return assessmentMaterial{}, err
	}
	return assessmentMaterial{spends: spends, units: units}, nil
}

func assessmentReport(st *store.Store, runID int64) (assess.Report, error) {
	material, err := loadAssessmentMaterial(st)
	if err != nil {
		return assess.Report{}, err
	}
	return assessmentReportFrom(st, runID, material)
}

func assessmentReportFrom(st *store.Store, runID int64, material assessmentMaterial) (assess.Report, error) {
	run, items, events, err := st.AssessmentRun(runID)
	if err != nil {
		return assess.Report{}, err
	}
	attempts, err := st.AssessmentAttempts(runID)
	if err != nil {
		return assess.Report{}, err
	}
	byContent := make(map[string]assess.Unit, len(material.units))
	for _, unit := range material.units {
		byContent[unit.ContentID] = unit
	}
	annotations := make(map[attempt.QuestionKey]assess.Annotations, len(items))
	for _, item := range items {
		unit, ok := byContent[item.ContentID]
		if !ok || unit.Version != item.ContentVersion {
			continue
		}
		for _, question := range unit.Questions {
			if question.ID != item.QuestionID {
				continue
			}
			annotations[attempt.QuestionKey{ContentID: item.ContentID, QuestionID: item.QuestionID}] = assess.Annotations{
				Evidence:               question.Evidence,
				Reasoning:              question.Reasoning,
				OptionReasoning:        maps.Clone(question.OptionReasoning),
				OptionReasoningMissing: exam.MissingOptionReasoning(question.Options, question.OptionReasoning),
				Glossary:               append([]exam.GlossaryEntry(nil), question.Glossary...),
			}
			break
		}
	}
	return assess.BuildReport(run, items, attempts, events, material.spends, annotations), nil
}

// runAssessmentLoop presents items in order, enforces the stored clocks and
// records each answer before moving on. A killed process leaves the run active;
// a second invocation resumes at the first unfinished item.
func runAssessmentLoop(opt Options, st *store.Store, run assess.Run, items []assess.Item, jsonMode bool) int {
	reader := bufio.NewReader(opt.Stdin)
	if !jsonMode {
		writeAssessmentPreamble(opt.Stdout, run, len(items))
	}
	notify := assessmentNotifier(opt, jsonMode)
	currentUnit := ""
	for index := 0; ; index++ {
		step, err := advanceAssessment(opt, st, run, items, index, notify)
		if err != nil {
			return internalError(opt, jsonMode, err)
		}
		if step.kind == stepAbandoned {
			return usageError(opt, jsonMode, "run_abandoned", abandonedRunMessage(run))
		}
		if step.kind == stepDone {
			break
		}
		index = step.index
		item, content, question := step.item, step.content, step.question
		if !jsonMode {
			if currentUnit != item.ContentID {
				writeAssessmentUnit(opt.Stdout, item, content)
				currentUnit = item.ContentID
			}
			writeAssessmentQuestion(opt.Stdout, item, question, len(items), item.DeadlineAt.Sub(opt.Now()))
		}
		for {
			line, eof, err := readAssessLine(reader)
			if err != nil {
				return internalError(opt, jsonMode, err)
			}
			if eof && strings.TrimSpace(line) == "" {
				return assessmentInterrupted(opt, run, jsonMode)
			}
			if strings.EqualFold(strings.TrimSpace(line), "quit") || strings.EqualFold(strings.TrimSpace(line), "q") {
				return assessmentInterrupted(opt, run, jsonMode)
			}
			outcome, parseErr, err := submitAssessmentAnswer(opt, st, run, item, question, line, notify)
			if err != nil {
				return internalError(opt, jsonMode, err)
			}
			if outcome == answerAbandoned {
				return usageError(opt, jsonMode, "run_abandoned", abandonedRunMessage(run))
			}
			if outcome == answerInvalid {
				if jsonMode {
					return usageError(opt, jsonMode, "invalid_answer",
						fmt.Sprintf("item %d: %v; the run remains active and can be resumed", item.Position+1, parseErr))
				}
				fmt.Fprintf(opt.Stdout, "Enter an option label, skip, or quit: ")
				if eof {
					return assessmentInterrupted(opt, run, jsonMode)
				}
				continue
			}
			if outcome == answerRecorded && !jsonMode {
				fmt.Fprintln(opt.Stdout, "Answer recorded; no feedback is shown until the report.")
			}
			break
		}
	}
	doc, err := completeAssessment(opt, st, run)
	if err != nil {
		return internalError(opt, jsonMode, err)
	}
	if jsonMode {
		return writeJSON(opt, doc)
	}
	writeAssessmentSummary(opt.Stdout, doc, st.Path())
	return 0
}

// Assessment step kinds returned by advanceAssessment.
const (
	stepReady = iota
	stepDone
	stepAbandoned
)

type assessmentStep struct {
	kind     int
	index    int
	item     assess.Item
	content  exam.Exam
	question exam.Question
}

// assessmentNotice reports an item finalized without an answer while the run
// advanced: assess.StateUnavailable or assess.StateTimedOut.
type assessmentNotice func(kind string, position int)

func assessmentNotifier(opt Options, jsonMode bool) assessmentNotice {
	return func(kind string, position int) {
		if jsonMode {
			return
		}
		switch kind {
		case assess.StateUnavailable:
			fmt.Fprintf(opt.Stdout, "Item %d unavailable; it will be listed, not scored.\n", position+1)
		case assess.StateTimedOut:
			fmt.Fprintf(opt.Stdout, "Item %d timed out.\n", position+1)
		}
	}
}

// advanceAssessment applies the run rules from items[start] on and stops at
// the first item that awaits an answer, presenting it (which starts its item
// clock) when it is still pending. On the way it records unavailable content,
// times out an item whose clock already ran out, and abandons the run when its
// whole window has passed. Re-presenting an item keeps its original clock, so
// calling it again for the same item is safe. The CLI loop and the desktop
// workspace both step through runs with it.
func advanceAssessment(opt Options, st *store.Store, run assess.Run, items []assess.Item, start int, notify assessmentNotice) (assessmentStep, error) {
	for index := start; index < len(items); index++ {
		item := items[index]
		if item.Finalized() || item.State == assess.StateUnavailable {
			continue
		}
		now := opt.Now()
		if !now.Before(run.DeadlineAt) {
			return assessmentStep{kind: stepAbandoned}, abandonElapsedAssessment(opt, st, run, item, notify)
		}
		content, question, ok := assessmentQuestion(st, item)
		if !ok {
			if err := st.MarkAssessmentUnavailable(run.ID, item.Position, now, "content is no longer practiceable at the recorded version"); err != nil {
				return assessmentStep{}, err
			}
			notify(assess.StateUnavailable, item.Position)
			continue
		}
		if item.State == assess.StatePending {
			presented, err := st.MarkAssessmentPresented(run.ID, item.Position, now)
			if errors.Is(err, store.ErrAssessmentWindowElapsed) {
				return assessmentStep{kind: stepAbandoned}, abandonElapsedAssessment(opt, st, run, item, notify)
			}
			if err != nil {
				return assessmentStep{}, err
			}
			item = presented
		}
		if !now.Before(item.DeadlineAt) {
			if err := appendAssessmentTimeout(opt, st, run, item, question); err != nil {
				return assessmentStep{}, err
			}
			notify(assess.StateTimedOut, item.Position)
			continue
		}
		return assessmentStep{kind: stepReady, index: index, item: item, content: content, question: question}, nil
	}
	return assessmentStep{kind: stepDone}, nil
}

// Outcomes of one submitted assessment answer.
const (
	answerRecorded = iota
	answerTimedOut
	answerAbandoned
	answerInvalid
)

// submitAssessmentAnswer applies the clocks to one raw answer for a presented
// item: an answer after the run window abandons the run, an answer after the
// item clock is recorded as a timeout, an unparseable answer records nothing,
// and anything else is stored as the item's attempt. parseErr is set only for
// answerInvalid.
func submitAssessmentAnswer(opt Options, st *store.Store, run assess.Run, item assess.Item, question exam.Question, raw string, notify assessmentNotice) (outcome int, parseErr error, err error) {
	line := raw
	if strings.EqualFold(strings.TrimSpace(line), "skip") {
		line = "skip"
	}
	parsed, parseErr := attempt.ParseSelection(question, strings.TrimSpace(line))
	now := opt.Now()
	if !now.Before(run.DeadlineAt) {
		return answerAbandoned, nil, abandonElapsedAssessment(opt, st, run, item, notify)
	}
	if !now.Before(item.DeadlineAt) {
		if err := appendAssessmentTimeout(opt, st, run, item, question); err != nil {
			return 0, nil, err
		}
		notify(assess.StateTimedOut, item.Position)
		return answerTimedOut, nil, nil
	}
	if parseErr != nil {
		return answerInvalid, parseErr, nil
	}
	record, err := attempt.BuildRecord(item.ContentID, item.ContentVersion, question, parsed, opt.NewSession(), item.PresentedAt, now)
	if err != nil {
		return 0, nil, err
	}
	record, err = record.WithAssessment(run.ID)
	if err != nil {
		return 0, nil, err
	}
	if _, err := st.AppendAssessmentAttempt(run.ID, item.Position, record, false, now); err != nil {
		return 0, nil, err
	}
	return answerRecorded, nil, nil
}

// completeAssessment marks a run with no open item completed and returns its
// result document. A run that is no longer active is read back unchanged.
func completeAssessment(opt Options, st *store.Store, run assess.Run) (assessmentRunResultDoc, error) {
	if _, err := st.CompleteAssessmentRun(run.ID, opt.Now()); err != nil && !errors.Is(err, store.ErrAssessmentRunState) {
		return assessmentRunResultDoc{}, err
	}
	completed, updatedItems, _, err := st.AssessmentRun(run.ID)
	if err != nil {
		return assessmentRunResultDoc{}, err
	}
	return assessmentRunDocFor(st, completed, updatedItems)
}

func abandonedRunMessage(run assess.Run) string {
	return fmt.Sprintf("assessment run %d passed its %d-minute window and was recorded abandoned; finalized items remain readable with `passquest assess report %d`",
		run.ID, run.WindowMinutes, run.ID)
}

// appendAssessmentTimeout records a timed-out skipped attempt at the item
// deadline, capped at the run deadline.
func appendAssessmentTimeout(opt Options, st *store.Store, run assess.Run, item assess.Item, question exam.Question) error {
	deadline := item.DeadlineAt
	if deadline.IsZero() || deadline.After(run.DeadlineAt) {
		deadline = run.DeadlineAt
	}
	record, err := attempt.BuildRecord(item.ContentID, item.ContentVersion, question,
		attempt.Selection{Skip: true}, opt.NewSession(), item.PresentedAt, deadline)
	if err != nil {
		return err
	}
	record, err = record.WithAssessment(run.ID)
	if err != nil {
		return err
	}
	_, err = st.AppendAssessmentAttempt(run.ID, item.Position, record, true, deadline)
	return err
}

func assessmentQuestion(st *store.Store, item assess.Item) (exam.Exam, exam.Question, bool) {
	src, err := st.SourceMeta(item.SourceID)
	if err != nil {
		return exam.Exam{}, exam.Question{}, false
	}
	page, err := st.Page(item.SourceID, item.Page)
	if err != nil {
		return exam.Exam{}, exam.Question{}, false
	}
	questions, err := st.Questions(item.SourceID)
	if err != nil {
		return exam.Exam{}, exam.Question{}, false
	}
	content, _, err := source.BuildPracticeExam(src, page, questions)
	if err != nil || content.Version != item.ContentVersion {
		return exam.Exam{}, exam.Question{}, false
	}
	question, _, found := content.Question(item.QuestionID)
	if !found {
		return exam.Exam{}, exam.Question{}, false
	}
	return content, question, true
}

func readAssessLine(reader *bufio.Reader) (string, bool, error) {
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return line, false, err
	}
	return line, errors.Is(err, io.EOF), nil
}

// abandonElapsedAssessment abandons a run whose window has passed. An item
// that is on screen has no recorded outcome yet; recording it as a timed-out
// attempt at its stored deadline keeps it in the timeout count and the score
// denominator instead of leaving it presented forever. A finalized item is not
// re-recorded.
func abandonElapsedAssessment(opt Options, st *store.Store, run assess.Run, item assess.Item, notify assessmentNotice) error {
	if item.State == assess.StatePresented {
		if _, question, ok := assessmentQuestion(st, item); ok {
			if err := appendAssessmentTimeout(opt, st, run, item, question); err != nil {
				return err
			}
			notify(assess.StateTimedOut, item.Position)
		}
	}
	if _, err := st.AbandonAssessmentRun(run.ID, opt.Now(), "window_elapsed"); err != nil && !errors.Is(err, store.ErrAssessmentRunState) {
		return err
	}
	return nil
}

func assessmentInterrupted(opt Options, run assess.Run, jsonMode bool) int {
	message := fmt.Sprintf("assessment run %d interrupted; resume it with `passquest assess resume %d`", run.ID, run.ID)
	writeError(opt, jsonMode, "run_interrupted", message)
	return 4
}

func assessmentRunLimitations() []string {
	return []string{
		"An assessment run records timed, unassisted answers on unseen units. It is not the real examination and computes no pass probability.",
		"The real item count, time limit and pass threshold are unknown; the requested values are local flags and the report prints the values it used.",
		"Contaminated items are excluded from the headline and listed separately; a partial run reports only finalized items.",
	}
}

// recordSourceQuestionPresentations appends one exposure per question a source
// inspection command will print. A page selected with --page narrows the set;
// a whole-source listing spends every unit whose question stem is shown.
//
// An explicitly displayed page also gets one page-level row with an empty
// question_id, independent of the questions currently authored on it: the
// passage itself is the exposure, so a page with no questions still spends its
// unit and a question added there later cannot enter the pool unseen. The
// per-question rows stay for the stems actually printed. Both kinds are rows in
// the append-only content_presentations table, and the pool counts one spend per
// content unit.
func recordSourceQuestionPresentations(opt Options, st *store.Store, sourceID string, questions []source.Question, page *source.Page, pageFilter int, surface string) int {
	at := opt.PresentationNow()
	entries := make([]store.Presentation, 0, len(questions)+1)
	if page != nil {
		entries = append(entries, store.Presentation{
			ContentID: source.ContentID(sourceID, page.Number),
			Surface:   surface,
			At:        at,
		})
	}
	for _, q := range questions {
		if page != nil && q.Page != page.Number {
			continue
		}
		if pageFilter != 0 && q.Page != pageFilter {
			continue
		}
		entries = append(entries, store.Presentation{
			ContentID:  source.ContentID(sourceID, q.Page),
			QuestionID: q.ID,
			Surface:    surface,
			At:         at,
		})
	}
	if err := st.RecordPresentations(entries); err != nil {
		return internalError(opt, false, err)
	}
	return 0
}

// recordNextPresentation spends the unit named by the recommendation. `next`
// prints a command rather than a stem, but the contract explicitly lists it as
// a presentation surface; this is the conservative reading. It is idempotent
// per recommended question: the pool counts one spend per unit and keeps the
// earliest time, so rendering the same recommendation again (the desktop Today
// view does on every render) writes nothing new.
func recordNextPresentation(st *store.Store, rec *adaptive.Recommendation, at time.Time) error {
	if rec == nil {
		return nil
	}
	material := rec.FreshMaterial
	if material == nil {
		material = rec.ReviewMaterial
	}
	if material == nil {
		return nil
	}
	_, err := st.RecordPresentationOnce(material.ContentID, material.QuestionID, assess.SurfaceNext, at)
	return err
}

// guardAssessmentCommand refuses tutor, skill and find while an assessment run
// is active. It opens the selected database only when it already exists, so a
// read-only lesson command does not create state as a side effect.
func guardAssessmentCommand(args []string, opt Options, command string) int {
	jsonMode := jsonFlagRequested(args)
	dbFlag := flagValue(args, "db")
	path := databasePath(dbFlag, opt.Getenv)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return 0
	} else if err != nil {
		return internalError(opt, jsonMode, err)
	}
	st, err := openDatabase(dbFlag, opt)
	if err != nil {
		return internalError(opt, jsonMode, err)
	}
	defer st.Close()
	if err := activeAssessmentRefusal(st, opt.Now, command); err != nil {
		return refusalOrInternal(opt, jsonMode, err)
	}
	return 0
}

// refusalOrInternal writes a refusal as a usage error and anything else as an
// internal error.
func refusalOrInternal(opt Options, jsonMode bool, err error) int {
	var refusal tacticRefusal
	if errors.As(err, &refusal) {
		return usageError(opt, jsonMode, refusal.code, refusal.message)
	}
	return internalError(opt, jsonMode, err)
}

// activeAssessmentRefusal refuses command with assessment_active while a run
// is active. A run whose window has passed is recorded abandoned instead and
// stops blocking. now is read only when a run is active.
func activeAssessmentRefusal(st *store.Store, now func() time.Time, command string) error {
	run, found, err := st.ActiveAssessmentRun()
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	at := now()
	if !at.Before(run.DeadlineAt) {
		if _, err := st.AbandonAssessmentRun(run.ID, at, "window_elapsed"); err != nil && !errors.Is(err, store.ErrAssessmentRunState) {
			return err
		}
		return nil
	}
	return refuse("assessment_active",
		"%s is unavailable while assessment run %d is active (deadline %s); complete it, or record `passquest assess abandon %d`, before using coaching",
		command, run.ID, run.DeadlineAt.UTC().Format(time.RFC3339), run.ID)
}

func flagValue(args []string, name string) string {
	value := ""
	for index := 0; index < len(args); index++ {
		arg := strings.TrimLeft(args[index], "-")
		if arg == name && index+1 < len(args) {
			value = args[index+1]
			index++
			continue
		}
		if candidate, ok := strings.CutPrefix(arg, name+"="); ok {
			value = candidate
		}
	}
	return value
}
