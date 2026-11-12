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
	if active, found, err := st.ActiveAssessmentRun(); err != nil {
		return internalError(opt, *jsonFlag, err)
	} else if found {
		return usageError(opt, *jsonFlag, "run_active",
			fmt.Sprintf("assessment run %d is still active; resume it with `passquest assess resume %d` or abandon it with `passquest assess abandon %d`",
				active.ID, active.ID, active.ID))
	}
	pool, identities, err := assessmentPool(st)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	if pool.ItemCount < assess.MinPoolItems {
		return usageError(opt, *jsonFlag, "pool_floor", poolFloorMessage(pool))
	}
	if pool.ItemCount < *sizeFlag {
		return usageError(opt, *jsonFlag, "pool_short",
			fmt.Sprintf("requested %d item(s), but the unseen pool has %d item(s) on %d unit(s); %d more item(s) must be reviewed and imported",
				*sizeFlag, pool.ItemCount, pool.UnitCount, *sizeFlag-pool.ItemCount))
	}
	units := pool.Select(*sizeFlag)
	actual := assess.SelectedItems(units)
	if actual == 0 {
		return usageError(opt, *jsonFlag, "pool_short", "no unseen assessment items are available")
	}
	perItem, ok := assess.PerItemTimeout(*minutesFlag, actual)
	if !ok {
		return usageError(opt, *jsonFlag, "invalid_input",
			fmt.Sprintf("--minutes %d and --size %d leave no per-item timeout", *minutesFlag, *sizeFlag))
	}
	now := opt.Now()
	run := assess.Run{
		CreatedAt:        now,
		StartedAt:        now,
		DeadlineAt:       now.Add(time.Duration(*minutesFlag) * time.Minute),
		WindowMinutes:    *minutesFlag,
		RequestedSize:    *sizeFlag,
		ActualSize:       actual,
		PerItemTimeoutMS: perItem,
		Annotations:      assess.AnnotationsNone,
		PoolFamily:       assess.PoolFamily(identities),
		SourceCount:      len(identities),
	}
	items := assessmentItems(units)
	created, err := st.CreateAssessmentRun(run, items)
	if errors.Is(err, store.ErrAssessmentActive) {
		return usageError(opt, *jsonFlag, "run_active", "another assessment run is already active")
	}
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	return runAssessmentLoop(opt, st, created, items, *jsonFlag)
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
	run, items, err := st.AssessmentRunItems(runID)
	if err != nil {
		return assessmentError(opt, *jsonFlag, err)
	}
	if run.Status == assess.StatusAbandoned {
		return usageError(opt, *jsonFlag, "run_abandoned",
			fmt.Sprintf("assessment run %d was abandoned (%s); its answered items remain readable with `passquest assess report %d`",
				run.ID, run.AbandonedReason, run.ID))
	}
	if run.Status == assess.StatusCompleted {
		doc, err := assessmentRunDocFor(st, run, items)
		if err != nil {
			return internalError(opt, *jsonFlag, err)
		}
		if *jsonFlag {
			return writeJSON(opt, doc)
		}
		writeAssessmentSummary(opt.Stdout, doc, st.Path())
		return 0
	}
	run, err = st.ResumeAssessmentRun(run.ID, opt.Now())
	if errors.Is(err, store.ErrAssessmentWindowElapsed) {
		return usageError(opt, *jsonFlag, "run_abandoned",
			fmt.Sprintf("assessment run %d passed its deadline and was recorded abandoned; resume is refused", run.ID))
	}
	if err != nil {
		return assessmentError(opt, *jsonFlag, err)
	}
	return runAssessmentLoop(opt, st, run, items, *jsonFlag)
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
	run, err := st.AbandonAssessmentRun(runID, opt.Now(), "learner_abandoned")
	if errors.Is(err, store.ErrAssessmentRunState) {
		return usageError(opt, *jsonFlag, "run_completed",
			fmt.Sprintf("assessment run %d is already completed; abandoning it would rewrite recorded evidence", runID))
	}
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
	material, err := loadAssessmentMaterial(st)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	a, err := assessmentReportFrom(st, first, material)
	if err != nil {
		return assessmentError(opt, *jsonFlag, err)
	}
	b, err := assessmentReportFrom(st, second, material)
	if err != nil {
		return assessmentError(opt, *jsonFlag, err)
	}
	if err := assess.Compare(a, b); err != nil {
		var refusal *assess.CompareRefusal
		if errors.As(err, &refusal) {
			return usageError(opt, *jsonFlag, "incomparable_runs",
				fmt.Sprintf("cannot compare runs %d and %d: %s", a.Run.ID, b.Run.ID, refusal.Error()))
		}
		return internalError(opt, *jsonFlag, err)
	}
	doc := comparisonDocOf(a, b)
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeComparison(opt.Stdout, doc, st.Path())
	return 0
}

func validateAssessmentFlags(size, minutes int, annotations string, opt Options, jsonMode bool) int {
	if size < 1 || size > 10_000 {
		return usageError(opt, jsonMode, "invalid_input", "--size must be between 1 and 10000")
	}
	if minutes < 1 || minutes > 1440 {
		return usageError(opt, jsonMode, "invalid_input", "--minutes must be between 1 and 1440")
	}
	if annotations != assess.AnnotationsNone {
		return usageError(opt, jsonMode, "invalid_annotations",
			fmt.Sprintf("--annotations %q is not supported; the only accepted value in this goal is none", annotations))
	}
	return 0
}

func parseRunID(raw string, opt Options, jsonMode bool) (int64, int) {
	id, err := parsePositiveInt64(raw)
	if err != nil {
		return 0, usageError(opt, jsonMode, "invalid_input", fmt.Sprintf("invalid assessment run id %q", raw))
	}
	return id, 0
}

func assessmentError(opt Options, jsonMode bool, err error) int {
	if errors.Is(err, store.ErrNotFound) {
		return usageError(opt, jsonMode, "unknown_run", err.Error())
	}
	if errors.Is(err, store.ErrAssessmentRunState) {
		return usageError(opt, jsonMode, "run_state", err.Error())
	}
	if errors.Is(err, store.ErrAssessmentWindowElapsed) {
		return usageError(opt, jsonMode, "run_abandoned", err.Error())
	}
	return internalError(opt, jsonMode, err)
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
				Evidence:        question.Evidence,
				Reasoning:       question.Reasoning,
				OptionReasoning: maps.Clone(question.OptionReasoning),
				Glossary:        append([]exam.GlossaryEntry(nil), question.Glossary...),
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
	currentUnit := ""
	for index := range items {
		item := items[index]
		if item.Finalized() || item.State == assess.StateUnavailable {
			continue
		}
		now := opt.Now()
		if !now.Before(run.DeadlineAt) {
			return abandonElapsedRun(opt, st, run, jsonMode)
		}
		content, question, ok := assessmentQuestion(st, item)
		if !ok {
			if err := st.MarkAssessmentUnavailable(run.ID, item.Position, now, "content is no longer practiceable at the recorded version"); err != nil {
				return internalError(opt, jsonMode, err)
			}
			if !jsonMode {
				fmt.Fprintf(opt.Stdout, "Item %d unavailable; it will be listed, not scored.\n", item.Position+1)
			}
			continue
		}
		if item.State == assess.StatePending {
			presented, err := st.MarkAssessmentPresented(run.ID, item.Position, now)
			if errors.Is(err, store.ErrAssessmentWindowElapsed) {
				return abandonElapsedRun(opt, st, run, jsonMode)
			}
			if err != nil {
				return internalError(opt, jsonMode, err)
			}
			item = presented
		}
		if !now.Before(item.DeadlineAt) {
			if code := recordAssessmentTimeout(opt, st, run, item, question, jsonMode); code != 0 {
				return code
			}
			continue
		}
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
			if strings.EqualFold(strings.TrimSpace(line), "skip") {
				line = "skip"
			}
			parsed, parseErr := attempt.ParseSelection(question, strings.TrimSpace(line))
			now = opt.Now()
			if !now.Before(run.DeadlineAt) {
				return abandonElapsedRun(opt, st, run, jsonMode)
			}
			if !now.Before(item.DeadlineAt) {
				if code := recordAssessmentTimeout(opt, st, run, item, question, jsonMode); code != 0 {
					return code
				}
				break
			}
			if parseErr != nil {
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
			record, err := attempt.BuildRecord(item.ContentID, item.ContentVersion, question, parsed, opt.NewSession(), item.PresentedAt, now)
			if err != nil {
				return internalError(opt, jsonMode, err)
			}
			record, err = record.WithAssessment(run.ID)
			if err != nil {
				return internalError(opt, jsonMode, err)
			}
			if _, err := st.AppendAssessmentAttempt(run.ID, item.Position, record, false, now); err != nil {
				return internalError(opt, jsonMode, err)
			}
			if !jsonMode {
				fmt.Fprintln(opt.Stdout, "Answer recorded; no feedback is shown until the report.")
			}
			break
		}
	}
	if _, err := st.CompleteAssessmentRun(run.ID, opt.Now()); err != nil && !errors.Is(err, store.ErrAssessmentRunState) {
		return internalError(opt, jsonMode, err)
	}
	completed, updatedItems, _, err := st.AssessmentRun(run.ID)
	if err != nil {
		return internalError(opt, jsonMode, err)
	}
	doc, err := assessmentRunDocFor(st, completed, updatedItems)
	if err != nil {
		return internalError(opt, jsonMode, err)
	}
	if jsonMode {
		return writeJSON(opt, doc)
	}
	writeAssessmentSummary(opt.Stdout, doc, st.Path())
	return 0
}

func recordAssessmentTimeout(opt Options, st *store.Store, run assess.Run, item assess.Item, question exam.Question, jsonMode bool) int {
	deadline := item.DeadlineAt
	if deadline.IsZero() || deadline.After(run.DeadlineAt) {
		deadline = run.DeadlineAt
	}
	record, err := attempt.BuildRecord(item.ContentID, item.ContentVersion, question,
		attempt.Selection{Skip: true}, opt.NewSession(), item.PresentedAt, deadline)
	if err != nil {
		return internalError(opt, jsonMode, err)
	}
	record, err = record.WithAssessment(run.ID)
	if err != nil {
		return internalError(opt, jsonMode, err)
	}
	if _, err := st.AppendAssessmentAttempt(run.ID, item.Position, record, true, deadline); err != nil {
		return internalError(opt, jsonMode, err)
	}
	if !jsonMode {
		fmt.Fprintf(opt.Stdout, "Item %d timed out.\n", item.Position+1)
	}
	return 0
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

func abandonElapsedRun(opt Options, st *store.Store, run assess.Run, jsonMode bool) int {
	if _, err := st.AbandonAssessmentRun(run.ID, opt.Now(), "window_elapsed"); err != nil && !errors.Is(err, store.ErrAssessmentRunState) {
		return internalError(opt, jsonMode, err)
	}
	return usageError(opt, jsonMode, "run_abandoned",
		fmt.Sprintf("assessment run %d passed its %d-minute window and was recorded abandoned; answered items remain readable with `passquest assess report %d`",
			run.ID, run.WindowMinutes, run.ID))
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
func recordSourceQuestionPresentations(opt Options, st *store.Store, sourceID string, questions []source.Question, page *source.Page, pageFilter int, surface string) int {
	at := opt.PresentationNow()
	entries := make([]store.Presentation, 0, len(questions))
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
// a presentation surface; this is the conservative reading.
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
	return st.RecordPresentation(material.ContentID, material.QuestionID, assess.SurfaceNext, at)
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
	run, found, err := st.ActiveAssessmentRun()
	if err != nil {
		return internalError(opt, jsonMode, err)
	}
	if !found {
		return 0
	}
	now := opt.Now()
	if !now.Before(run.DeadlineAt) {
		if _, err := st.AbandonAssessmentRun(run.ID, now, "window_elapsed"); err != nil && !errors.Is(err, store.ErrAssessmentRunState) {
			return internalError(opt, jsonMode, err)
		}
		return 0
	}
	return usageError(opt, jsonMode, "assessment_active",
		fmt.Sprintf("%s is unavailable while assessment run %d is active (deadline %s); complete it, or record `passquest assess abandon %d`, before using coaching",
			command, run.ID, run.DeadlineAt.UTC().Format(time.RFC3339), run.ID))
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
