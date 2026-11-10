package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/tactic"
)

// Tactic stages are protocol values.
const (
	stageGuided      = "guided"
	stageIndependent = "independent"
)

// tacticRun is the CLI's view of one tactic attempt: the resolved stage plus
// the display fields copied from the lesson.
type tacticRun struct {
	ID       string
	Title    string
	Stage    string
	Guidance *guidanceView
}

func (r tacticRun) assistance() attempt.Assistance {
	if r.Stage == stageGuided {
		return attempt.AssistanceAssisted
	}
	return attempt.AssistanceIndependent
}

// deliveredGuidance returns the walkthrough delivered with the attempt. It is
// nil for an independent stage, so an answered independent document carries no
// guidance.
func (r *tacticRun) deliveredGuidance() *guidanceView {
	if r == nil {
		return nil
	}
	return r.Guidance
}

type tacticRefusal struct {
	code    string
	message string
}

func (e tacticRefusal) Error() string { return e.message }

func refuse(code, format string, args ...any) error {
	return tacticRefusal{code: code, message: fmt.Sprintf(format, args...)}
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

	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	records, err := st.List()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}

	stage, err := resolveTacticStage(*stageFlag, lesson.ID, records)
	if err != nil {
		return tacticError(opt, *jsonFlag, err)
	}
	candidates, err := tacticCandidates(st, lesson, *sourceFlag)
	if err != nil {
		return tacticError(opt, *jsonFlag, err)
	}
	candidate, err := pickTacticCandidate(lesson, stage, *questionFlag, candidates, records)
	if err != nil {
		return tacticError(opt, *jsonFlag, err)
	}

	selection := practiceSelection{
		Content:  candidate.Content,
		Question: candidate.Question,
		Number:   candidate.Number,
		Page:     candidate.Page,
		Review:   candidate.Review,
		Tactic: &tacticRun{
			ID:       lesson.ID,
			Title:    lesson.Title,
			Stage:    stage,
			Guidance: nil,
		},
	}
	if stage == stageGuided {
		selection.Tactic.Guidance = guidanceFor(lesson)
	}

	if *jsonFlag {
		if !answerSet && !*skipFlag {
			return writeJSON(opt, awaitingDoc{
				SchemaVersion: ProtocolVersion,
				State:         "awaiting_answer",
				Tactic:        tacticRunViewOf(selection.Tactic),
				Guidance:      selection.Tactic.Guidance,
				Content:       contentViewOf(selection.Content),
				Question:      questionViewOf(selection),
			})
		}
		return answerQuestion(opt, selection, st, answerFlag, answerSet, *skipFlag, true)
	}
	return answerQuestion(opt, selection, st, answerFlag, answerSet, *skipFlag, false)
}

func tacticError(opt Options, jsonMode bool, err error) int {
	var refusal tacticRefusal
	if errors.As(err, &refusal) {
		return usageError(opt, jsonMode, refusal.code, refusal.message)
	}
	switch {
	case errors.Is(err, source.ErrInvalidInput),
		errors.Is(err, source.ErrToolMissing),
		errors.Is(err, source.ErrNotPracticeable),
		errors.Is(err, store.ErrNotFound):
		return sourceError(opt, jsonMode, err)
	default:
		return internalError(opt, jsonMode, err)
	}
}

// resolveTacticStage applies the "after help" rule. An explicit stage is
// honoured; otherwise the first attempt is guided and later attempts are
// independent. Independent is refused until one guided attempt exists.
func resolveTacticStage(requested, tacticID string, records []attempt.Attempt) (string, error) {
	hasGuided := false
	for _, record := range records {
		if record.TacticID == tacticID && record.Assistance == attempt.AssistanceAssisted {
			hasGuided = true
			break
		}
	}
	switch strings.TrimSpace(requested) {
	case stageGuided:
		return stageGuided, nil
	case stageIndependent:
		if !hasGuided {
			return "", refuse("guided_required",
				"complete a guided %s attempt before the independent question; the guided walkthrough is the help this stage follows", tacticID)
		}
		return stageIndependent, nil
	case "":
		if hasGuided {
			return stageIndependent, nil
		}
		return stageGuided, nil
	default:
		return "", refuse("invalid_input", "unknown stage %q; use guided or independent", requested)
	}
}

type tacticCandidate struct {
	Content  exam.Exam
	Question exam.Question
	Number   int
	Page     *source.Page
	Review   *source.Question
}

func (c tacticCandidate) key() attempt.QuestionKey {
	return attempt.QuestionKey{ContentID: c.Content.ID, QuestionID: c.Question.ID}
}

func tacticCandidates(st *store.Store, lesson tactic.Tactic, sourceID string) ([]tacticCandidate, error) {
	if strings.TrimSpace(sourceID) == "" {
		return exerciseCandidates(lesson)
	}
	src, questions, err := loadSourceBundle(st, sourceID)
	if err != nil {
		return nil, err
	}
	var out []tacticCandidate
	for _, page := range src.Pages {
		content, _, buildErr := source.BuildPracticeExam(src, page, questions)
		if buildErr != nil {
			continue
		}
		for i, question := range content.Questions {
			if !slices.Contains(question.Tactics, lesson.ID) {
				continue
			}
			meta, ok := findQuestion(questions, question.ID)
			if !ok {
				continue
			}
			pageCopy := page
			metaCopy := meta
			out = append(out, tacticCandidate{
				Content:  content,
				Question: question,
				Number:   i,
				Page:     &pageCopy,
				Review:   &metaCopy,
			})
		}
	}
	if len(out) == 0 {
		return nil, refuse("not_practiceable",
			"source %s has no reviewed question tagged %q; add a tagged question with `question add`, then review the page and its answer (see `passquest questions %s`)",
			sourceID, lesson.ID, sourceID)
	}
	return out, nil
}

func exerciseCandidates(lesson tactic.Tactic) ([]tacticCandidate, error) {
	content, err := tactic.Exercise()
	if err != nil {
		return nil, err
	}
	tagged, err := tactic.Tagged(lesson.ID)
	if err != nil {
		return nil, err
	}
	out := make([]tacticCandidate, 0, len(tagged))
	for _, taggedQuestion := range tagged {
		question, number, ok := content.Question(taggedQuestion.ID)
		if !ok {
			return nil, fmt.Errorf("tagged exercise question %s is not in the exercise", taggedQuestion.ID)
		}
		out = append(out, tacticCandidate{
			Content:  content,
			Question: question,
			Number:   number,
		})
	}
	if len(out) == 0 {
		return nil, refuse("not_practiceable", "the first-party exercise has no question tagged %q", lesson.ID)
	}
	return out, nil
}

func pickTacticCandidate(lesson tactic.Tactic, stage, questionID string, candidates []tacticCandidate, records []attempt.Attempt) (tacticCandidate, error) {
	prior := make(map[attempt.QuestionKey]int, len(records))
	guided := make(map[attempt.QuestionKey]bool, len(records))
	for _, record := range records {
		prior[record.Key()]++
		if record.TacticID == lesson.ID && record.Assistance == attempt.AssistanceAssisted {
			guided[record.Key()] = true
		}
	}

	if strings.TrimSpace(questionID) != "" {
		for _, candidate := range candidates {
			if candidate.Question.ID == questionID {
				if err := checkIndependent(stage, candidate, prior, guided); err != nil {
					return tacticCandidate{}, err
				}
				return candidate, nil
			}
		}
		return tacticCandidate{}, refuse("invalid_input",
			"question %q is not a reviewed question tagged %q in this material", questionID, lesson.ID)
	}

	var fallback *tacticCandidate
	for i := range candidates {
		candidate := candidates[i]
		if fallback == nil {
			fallback = &candidate
		}
		if err := checkIndependent(stage, candidate, prior, guided); err != nil {
			continue
		}
		if stage == stageGuided && prior[candidate.key()] > 0 {
			continue
		}
		return candidate, nil
	}
	if stage == stageGuided && fallback != nil {
		// A second guided run is allowed; the attempt is recorded as assisted and
		// the existing repeat exposure remains visible.
		return *fallback, nil
	}
	if stage == stageIndependent {
		used := 0
		for _, candidate := range candidates {
			if guided[candidate.key()] {
				used++
			}
		}
		return tacticCandidate{}, refuse("not_practiceable",
			"no unattempted question tagged %q remains distinct from the guided example (%d candidate(s), %d used as guided); add and review another tagged question, or repeat the guided example with --stage guided",
			lesson.ID, len(candidates), used)
	}
	return tacticCandidate{}, refuse("not_practiceable", "no practiceable question tagged %q", lesson.ID)
}

func checkIndependent(stage string, candidate tacticCandidate, prior map[attempt.QuestionKey]int, guided map[attempt.QuestionKey]bool) error {
	if stage != stageIndependent {
		return nil
	}
	key := candidate.key()
	if guided[key] {
		return refuse("invalid_input", "question %s was the guided example; choose a different question", candidate.Question.ID)
	}
	if prior[key] > 0 {
		return refuse("not_practiceable",
			"question %s already has %d earlier attempt(s); an independent attempt must be a first attempt", candidate.Question.ID, prior[key])
	}
	return nil
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
