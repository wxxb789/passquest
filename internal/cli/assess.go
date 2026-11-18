package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/assessment"
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

// openAssessmentService opens the selected database as one assessment service
// and reconciles expired runs. Every assess subcommand gets one write path for
// that reconcile.
func openAssessmentService(dbFlag *string, opt Options, jsonMode bool) (*study.Service, int) {
	svc, err := study.Open(opt.ports(*dbFlag))
	if err != nil {
		return nil, internalError(opt, jsonMode, err)
	}
	if err := assessment.Reconcile(context.Background(), svc); err != nil {
		svc.Close()
		return nil, internalError(opt, jsonMode, err)
	}
	return svc, 0
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
	svc, code := openAssessmentService(dbFlag, opt, *jsonFlag)
	if code != 0 {
		return code
	}
	defer svc.Close()
	reported, err := assessment.Report(context.Background(), svc, assessment.ReportQuery{Query: assessment.QueryPool})
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	doc := poolDocOf(reported.Pool, reported.Identities)
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writePool(opt.Stdout, doc, svc.Path())
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
	svc, code := openAssessmentService(dbFlag, opt, *jsonFlag)
	if code != 0 {
		return code
	}
	defer svc.Close()
	started, err := assessment.Begin(context.Background(), svc, *sizeFlag, *minutesFlag)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	return runAssessmentLoop(opt, svc, started.Run, started.Items, *jsonFlag)
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
	svc, code := openAssessmentService(dbFlag, opt, *jsonFlag)
	if code != 0 {
		return code
	}
	defer svc.Close()
	stepped, err := assessment.Step(context.Background(), svc, runID, assessment.Resume, 0, nil)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	if stepped.Result != nil {
		doc := assessmentRunDocFor(*stepped.Result)
		if *jsonFlag {
			return writeJSON(opt, doc)
		}
		writeAssessmentSummary(opt.Stdout, doc, svc.Path())
		return 0
	}
	return runAssessmentLoop(opt, svc, stepped.Run, stepped.Items, *jsonFlag)
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
	svc, code := openAssessmentService(dbFlag, opt, *jsonFlag)
	if code != 0 {
		return code
	}
	defer svc.Close()
	stepped, err := assessment.Step(context.Background(), svc, runID, assessment.Abandon, 0, nil)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	run := stepped.Run
	doc := abandonedDoc{SchemaVersion: ProtocolVersion, State: "abandoned", RunID: run.ID, Reason: run.AbandonedReason}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	fmt.Fprintf(opt.Stdout, "Assessment run %d abandoned (%s). Answered items remain readable with `passquest assess report %d`.\nDatabase: %s\n",
		run.ID, run.AbandonedReason, run.ID, svc.Path())
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
	svc, code := openAssessmentService(dbFlag, opt, *jsonFlag)
	if code != 0 {
		return code
	}
	defer svc.Close()
	reported, err := assessment.Report(context.Background(), svc, assessment.ReportQuery{Query: assessment.QueryReport, RunID: runID})
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	doc := reportDocOf(reported.Report)
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeAssessmentReport(opt.Stdout, doc, svc.Path())
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
	svc, code := openAssessmentService(dbFlag, opt, *jsonFlag)
	if code != 0 {
		return code
	}
	defer svc.Close()
	reported, err := assessment.Report(context.Background(), svc,
		assessment.ReportQuery{Query: assessment.QueryCompare, First: first, Second: second})
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	doc := comparisonDocOf(reported.First, reported.Second)
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeComparison(opt.Stdout, doc, svc.Path())
	return 0
}

func validateAssessmentFlags(size, minutes int, annotations string, opt Options, jsonMode bool) int {
	if err := assessment.ValidateSettings(size, minutes, annotations); err != nil {
		return fail(opt, jsonMode, err)
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

// runAssessmentLoop presents items in order, enforces the stored clocks and
// records each answer before moving on. A killed process leaves the run active;
// a second invocation resumes at the first unfinished item.
func runAssessmentLoop(opt Options, svc *study.Service, run assess.Run, items []assess.Item, jsonMode bool) int {
	reader := bufio.NewReader(opt.Stdin)
	if !jsonMode {
		writeAssessmentPreamble(opt.Stdout, run, len(items))
	}
	notify := assessmentNotifier(opt, jsonMode)
	currentUnit := ""
	for index := 0; ; index++ {
		stepped, err := assessment.Step(context.Background(), svc, run.ID, assessment.Present, index, notify)
		if err != nil {
			return internalError(opt, jsonMode, err)
		}
		if stepped.Kind == assessment.Abandoned {
			return usageError(opt, jsonMode, "run_abandoned", assessment.AbandonedMessage(run))
		}
		if stepped.Kind == assessment.Done {
			doc := assessmentRunDocFor(*stepped.Result)
			if jsonMode {
				return writeJSON(opt, doc)
			}
			writeAssessmentSummary(opt.Stdout, doc, svc.Path())
			return 0
		}
		presented := stepped.Presented
		index = presented.Index
		item, content, question := presented.Item, presented.Content, presented.Question
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
			stepped, err := assessment.Submit(context.Background(), svc, run, *presented, line, notify)
			if err != nil {
				return internalError(opt, jsonMode, err)
			}
			if stepped.Outcome == assessment.AnswerAbandoned {
				return usageError(opt, jsonMode, "run_abandoned", assessment.AbandonedMessage(run))
			}
			if stepped.Outcome == assessment.AnswerInvalid {
				if jsonMode {
					return usageError(opt, jsonMode, "invalid_answer",
						fmt.Sprintf("item %d: %v; the run remains active and can be resumed", item.Position+1, stepped.ParseErr))
				}
				fmt.Fprintf(opt.Stdout, "Enter an option label, skip, or quit: ")
				if eof {
					return assessmentInterrupted(opt, run, jsonMode)
				}
				continue
			}
			if stepped.Outcome == assessment.AnswerRecorded && !jsonMode {
				fmt.Fprintln(opt.Stdout, "Answer recorded; no feedback is shown until the report.")
			}
			break
		}
	}
}

func assessmentNotifier(opt Options, jsonMode bool) assessment.Notify {
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

func readAssessLine(reader *bufio.Reader) (string, bool, error) {
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return line, false, err
	}
	return line, errors.Is(err, io.EOF), nil
}

func assessmentInterrupted(opt Options, run assess.Run, jsonMode bool) int {
	message := fmt.Sprintf("assessment run %d interrupted; resume it with `passquest assess resume %d`", run.ID, run.ID)
	writeError(opt, jsonMode, "run_interrupted", message)
	return 4
}

// guardAssessmentCommand refuses tutor, skill and find while an assessment run
// is active. It opens the selected database only when it already exists, so a
// read-only lesson command does not create state as a side effect.
func guardAssessmentCommand(args []string, opt Options, command string) int {
	jsonMode := jsonFlagRequested(args)
	dbFlag := flagValue(args, "db")
	if err := study.GuardPath(opt.ports(dbFlag), command); err != nil {
		return fail(opt, jsonMode, err)
	}
	return 0
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
