package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/practice"
	"github.com/wxxb789/passquest/internal/tactic"
)

// Tactic stages are protocol values: the practice module's values are the one
// source of truth.
const (
	stageGuided      = practice.StageGuided
	stageIndependent = practice.StageIndependent
)

// tacticRun is the CLI's view of one tactic attempt: the resolved stage plus
// the display fields copied from the lesson.
type tacticRun struct {
	ID       string
	Title    string
	Stage    string
	Guidance *guidanceView
}

// tacticRunOf maps a practice tactic run to the CLI's display view.
func tacticRunOf(run *practice.TacticRun) *tacticRun {
	if run == nil {
		return nil
	}
	view := &tacticRun{ID: run.ID, Title: run.Title, Stage: run.Stage}
	if run.Lesson != nil {
		view.Guidance = guidanceFor(*run.Lesson)
	}
	return view
}

func (r tacticRun) assistance() attempt.Assistance {
	if r.Stage == practice.StageGuided {
		return attempt.AssistanceAssisted
	}
	return attempt.AssistanceIndependent
}

// deliveredGuidanceOf returns the walkthrough the answered document of a
// tactic run delivers. It is nil for an independent stage, so an answered
// independent document carries no guidance.
func deliveredGuidanceOf(run *practice.TacticRun) *guidanceView {
	if run == nil || run.Lesson == nil {
		return nil
	}
	return guidanceFor(*run.Lesson)
}

func runTactic(args []string, opt Options) int {
	if len(args) == 0 {
		return usageError(opt, jsonFlagRequested(args), "invalid_input", "tactic requires list, show or practice")
	}
	switch args[0] {
	case "list":
		return runTacticList(args[1:], opt)
	case "show":
		return runTacticShow(args[1:], opt)
	case "practice":
		return runTacticPractice(args[1:], opt)
	default:
		return usageError(opt, jsonFlagRequested(args), "invalid_input", fmt.Sprintf("unknown tactic command %q", args[0]))
	}
}

func runTacticList(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	fs := newFlagSet("tactic list", opt, wantJSON)
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	all := tactic.All()
	if *jsonFlag {
		doc := tacticListDoc{SchemaVersion: ProtocolVersion, Tactics: make([]tacticListView, 0, len(all))}
		for _, candidate := range all {
			doc.Tactics = append(doc.Tactics, tacticListView{
				ID: candidate.ID, Title: candidate.Title, Summary: candidate.Summary,
			})
		}
		return writeJSON(opt, doc)
	}
	fmt.Fprintln(opt.Stdout, "Pass Quest — scoring tactics")
	for _, candidate := range all {
		fmt.Fprintf(opt.Stdout, "  %s  %s\n", candidate.ID, candidate.Title)
		fmt.Fprintf(opt.Stdout, "    %s\n", candidate.Summary)
	}
	return 0
}

func runTacticShow(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{})
	fs := newFlagSet("tactic show", opt, wantJSON)
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 1, "tactic show requires exactly one tactic id"); code != 0 {
		return code
	}
	lesson, ok := tactic.Get(fs.Arg(0))
	if !ok {
		return usageError(opt, *jsonFlag, "unknown_tactic",
			fmt.Sprintf("tactic %q is not registered; available: %s", fs.Arg(0), strings.Join(tactic.Registered(), ", ")))
	}
	view := lessonViewOf(lesson)
	if *jsonFlag {
		return writeJSON(opt, tacticShowDoc{SchemaVersion: ProtocolVersion, Tactic: view})
	}
	writeLesson(opt.Stdout, view)
	return 0
}

func runTacticPractice(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{
		"source": true, "stage": true, "question": true, "answer": true, "db": true,
	})
	fs := newFlagSet("tactic practice", opt, wantJSON)
	sourceFlag := fs.String("source", "", "imported source id to practice")
	stageFlag := fs.String("stage", "", "guided or independent (default: guided until one guided attempt exists)")
	questionFlag := fs.String("question", "", "question id to practice")
	answerFlag := fs.String("answer", "", "answer label (A, B, C, ...)")
	skipFlag := fs.Bool("skip", false, "record a skipped attempt")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 1, "tactic practice requires exactly one tactic id"); code != 0 {
		return code
	}
	answerSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "answer" {
			answerSet = true
		}
	})
	if answerSet && *skipFlag {
		return usageError(opt, *jsonFlag, "invalid_input", "--answer and --skip are mutually exclusive")
	}
	tacticID := fs.Arg(0)
	lesson, ok := tactic.Get(tacticID)
	if !ok {
		return usageError(opt, *jsonFlag, "unknown_tactic",
			fmt.Sprintf("tactic %q is not registered; available: %s", tacticID, strings.Join(tactic.Registered(), ", ")))
	}

	svc, err := study.Open(opt.ports(*dbFlag))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	item, err := practice.Show(context.Background(), svc, practice.Request{
		Kind:       practice.KindTactic,
		TacticID:   lesson.ID,
		Stage:      *stageFlag,
		SourceID:   *sourceFlag,
		QuestionID: *questionFlag,
	}, practiceHold(*jsonFlag, answerSet, *skipFlag))
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}

	if *jsonFlag && !answerSet && !*skipFlag {
		return writeJSON(opt, awaitingDoc{
			SchemaVersion: ProtocolVersion,
			State:         "awaiting_answer",
			Tactic:        tacticRunViewOf(tacticRunOf(item.Tactic)),
			Guidance:      deliveredGuidanceOf(item.Tactic),
			Content:       contentViewOf(item.Content),
			Question:      questionViewOf(item),
		})
	}
	return answerItem(opt, svc, item, answerFlag, answerSet, *skipFlag, *jsonFlag)
}

func boundaryViewOf(boundary tactic.Boundary) boundaryView {
	return boundaryView{
		Folklore:   boundary.Folklore,
		Example:    boundary.Example,
		WhyItFails: boundary.WhyItFails,
	}
}

func guidanceViewOf(principle, whyItWorks, whenItWorks string, steps []string, boundary boundaryView) guidanceView {
	return guidanceView{
		Principle:   principle,
		WhyItWorks:  whyItWorks,
		WhenItWorks: whenItWorks,
		Steps:       append([]string(nil), steps...),
		Boundary:    boundary,
	}
}

func guidanceFor(lesson tactic.Tactic) *guidanceView {
	view := guidanceViewOf(lesson.Principle, lesson.WhyItWorks, lesson.WhenItWorks, lesson.Steps, boundaryViewOf(lesson.Boundary))
	return &view
}

func writeGuidance(w io.Writer, guidance guidanceView) {
	fmt.Fprintf(w, "Principle: %s\n", guidance.Principle)
	fmt.Fprintf(w, "Why it works: %s\n", guidance.WhyItWorks)
	fmt.Fprintf(w, "When it works: %s\n", guidance.WhenItWorks)
	fmt.Fprintln(w, "Steps:")
	for i, step := range guidance.Steps {
		fmt.Fprintf(w, "  %d. %s\n", i+1, step)
	}
	fmt.Fprintf(w, "Boundary — %s\n", guidance.Boundary.Folklore)
	fmt.Fprintf(w, "  Example: %s\n", guidance.Boundary.Example)
	fmt.Fprintf(w, "  Why it fails: %s\n", guidance.Boundary.WhyItFails)
}

func writeTacticPreamble(w io.Writer, run *tacticRun) {
	mode := "guided example"
	if run.Stage == stageIndependent {
		mode = "independent question"
	}
	fmt.Fprintf(w, "Pass Quest — tactic: %s (%s)\n", run.Title, mode)
	if run.Stage == stageIndependent {
		fmt.Fprintln(w, "No hint is shown. This question is different from the guided example and has no earlier attempt.")
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintln(w, "Help shown: this walkthrough is visible before the answer, so the attempt is recorded as assisted, not independent.")
	writeGuidance(w, *run.Guidance)
	fmt.Fprintln(w)
}

func writeLesson(w io.Writer, lesson lessonView) {
	fmt.Fprintf(w, "Pass Quest — tactic %s\n", lesson.ID)
	fmt.Fprintf(w, "Title: %s\n", lesson.Title)
	fmt.Fprintf(w, "Summary: %s\n", lesson.Summary)
	writeGuidance(w, guidanceViewOf(lesson.Principle, lesson.WhyItWorks, lesson.WhenItWorks, lesson.Steps, lesson.Boundary))
}

func lessonViewOf(lesson tactic.Tactic) lessonView {
	return lessonView{
		ID:          lesson.ID,
		Title:       lesson.Title,
		Summary:     lesson.Summary,
		Principle:   lesson.Principle,
		WhyItWorks:  lesson.WhyItWorks,
		WhenItWorks: lesson.WhenItWorks,
		Steps:       append([]string(nil), lesson.Steps...),
		Boundary:    boundaryViewOf(lesson.Boundary),
	}
}

func tacticRunViewOf(run *tacticRun) *tacticRunView {
	if run == nil {
		return nil
	}
	return &tacticRunView{
		ID:         run.ID,
		Title:      run.Title,
		Stage:      run.Stage,
		Assistance: string(run.assistance()),
	}
}

// JSON documents. Field names are protocol: tests and golden files pin them.

type tacticListView struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

type tacticListDoc struct {
	SchemaVersion int              `json:"schema_version"`
	Tactics       []tacticListView `json:"tactics"`
}

type boundaryView struct {
	Folklore   string `json:"folklore"`
	Example    string `json:"example"`
	WhyItFails string `json:"why_it_fails"`
}

type lessonView struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Summary     string       `json:"summary"`
	Principle   string       `json:"principle"`
	WhyItWorks  string       `json:"why_it_works"`
	WhenItWorks string       `json:"when_it_works"`
	Steps       []string     `json:"steps"`
	Boundary    boundaryView `json:"boundary"`
}

type tacticShowDoc struct {
	SchemaVersion int        `json:"schema_version"`
	Tactic        lessonView `json:"tactic"`
}

type guidanceView struct {
	Principle   string       `json:"principle"`
	WhyItWorks  string       `json:"why_it_works"`
	WhenItWorks string       `json:"when_it_works"`
	Steps       []string     `json:"steps"`
	Boundary    boundaryView `json:"boundary"`
}

type tacticRunView struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Stage      string `json:"stage"`
	Assistance string `json:"assistance"`
}
