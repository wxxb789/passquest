// Package cli implements the pre-GUI access surface: a scriptable practice
// command and an evidence report over the append-only attempts store.
package cli

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/tactic"
)

// ProtocolVersion is the JSON protocol emitted by --json.
const ProtocolVersion = 1

// Options carries process dependencies so tests never depend on wall-clock
// time, an ambient database path, or a random session id.
type Options struct {
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	Now        func() time.Time
	NewSession func() string
	Getenv     func(string) string
}

func (o Options) withDefaults() Options {
	if o.Stdin == nil {
		o.Stdin = os.Stdin
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.NewSession == nil {
		o.NewSession = newSessionID
	}
	if o.Getenv == nil {
		o.Getenv = os.Getenv
	}
	return o
}

// Run executes one CLI invocation and returns the process exit code.
func Run(args []string, options Options) int {
	opt := options.withDefaults()
	if len(args) == 0 {
		fmt.Fprint(opt.Stderr, usage)
		return 2
	}
	switch args[0] {
	case "practice":
		return runPractice(args[1:], opt)
	case "report":
		return runReport(args[1:], opt)
	case "import":
		return runImport(args[1:], opt)
	case "sources":
		return runSources(args[1:], opt)
	case "show":
		return runShow(args[1:], opt)
	case "questions":
		return runQuestions(args[1:], opt)
	case "page":
		return runPage(args[1:], opt)
	case "question":
		return runQuestion(args[1:], opt)
	case "tactic":
		return runTactic(args[1:], opt)
	case "skill":
		return runSkill(args[1:], opt)
	case "next":
		return runNext(args[1:], opt)
	case "version":
		return runVersion(opt)
	case "help", "-h", "--help":
		fmt.Fprint(opt.Stdout, usage)
		return 0
	default:
		fmt.Fprintf(opt.Stderr, "passquest: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

const usage = `Pass Quest — local practice and attempt evidence.

Usage:
  passquest import   --path PATH [--title TITLE] [--db PATH] [--json]
  passquest sources  [--db PATH] [--json]
  passquest show     SOURCE [--page N] [--db PATH] [--json]
  passquest questions SOURCE [--page N] [--db PATH] [--json]
  passquest page edit SOURCE PAGE --text-file PATH --reviewer NAME [--note TEXT] [--db PATH] [--json]
  passquest page review SOURCE PAGE --reviewer NAME [--note TEXT] [--db PATH] [--json]
  passquest question add SOURCE PAGE --file QUESTION.json [--db PATH] [--json]
  passquest question edit SOURCE QUESTION --file QUESTION.json [--db PATH] [--json]
  passquest question review SOURCE QUESTION --answer LABEL --reviewer NAME [--db PATH] [--json]
  passquest practice [--db PATH] [--source SOURCE] [--page N] [--question ID]
                     [--answer LABEL | --skip] [--json]
  passquest tactic list [--json]
  passquest tactic show TACTIC [--json]
  passquest tactic practice TACTIC [--source SOURCE] [--stage guided|independent]
                     [--question ID] [--answer LABEL | --skip] [--db PATH] [--json]
  passquest skill list [--json]
  passquest skill show SKILL [--json]
  passquest skill practice SKILL [--source SOURCE] [--question ID]
                     [--answer LABEL | --skip] [--db PATH] [--json]
  passquest next     [--source SOURCE] [--db PATH] [--json]
  passquest report   [--db PATH] [--json]
  passquest version
  passquest help

Imported pages and questions start unreviewed; only reviewed material can be
practiced. A tactic guided attempt shows the walkthrough and is recorded as
assisted; the independent question is a different, never-exposed, first-attempt,
answer-verified question with no hint. The shipped exercise is a self-authored
sample. No exam score, pass threshold or readiness probability is computed.
Default database: $PASSQUEST_DB or ./.passquest/passquest.sqlite
`

func newSessionID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("session-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func databasePath(flagValue string, getenv func(string) string) string {
	if strings.TrimSpace(flagValue) != "" {
		return flagValue
	}
	if v := strings.TrimSpace(getenv("PASSQUEST_DB")); v != "" {
		return v
	}
	return filepath.Join(".passquest", "passquest.sqlite")
}

// openDatabase opens the resolved database path. The app-owned default state
// directory is tightened to 0700; a directory chosen with --db or PASSQUEST_DB
// is left to the user.
func openDatabase(flagValue string, opt Options) (*store.Store, error) {
	path := databasePath(flagValue, opt.Getenv)
	if strings.TrimSpace(flagValue) == "" && strings.TrimSpace(opt.Getenv("PASSQUEST_DB")) == "" {
		return store.OpenPrivate(path)
	}
	return store.Open(path)
}

// jsonFlagRequested reports whether args ask for JSON before flag.Parse runs, so
// a parse failure still produces the documented JSON error document. A
// successful parse is authoritative for values such as --json=false.
func jsonFlagRequested(args []string) bool {
	for _, arg := range args {
		name := strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-")
		if name == "json" {
			return true
		}
		if value, ok := strings.CutPrefix(name, "json="); ok {
			if parsed, err := strconv.ParseBool(value); err == nil && parsed {
				return true
			}
		}
	}
	return false
}

func runPractice(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	fs := newFlagSet("practice", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite database path")
	sourceFlag := fs.String("source", "", "imported source id to practice")
	pageFlag := fs.Int("page", 0, "page number inside the imported source")
	questionFlag := fs.String("question", "", "question id to practice")
	answerFlag := fs.String("answer", "", "answer label (A, B, C, ...)")
	skipFlag := fs.Bool("skip", false, "record a skipped attempt")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
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
	if *pageFlag != 0 && *sourceFlag == "" {
		return usageError(opt, *jsonFlag, "invalid_input", "--page requires --source")
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

	var selection practiceSelection
	if *sourceFlag != "" {
		selection, err = selectSourcePractice(st, *sourceFlag, *pageFlag, *questionFlag, records)
		if err != nil {
			return sourceError(opt, *jsonFlag, err)
		}
	} else {
		sample, err := exam.Sample()
		if err != nil {
			return internalError(opt, *jsonFlag, err)
		}
		selection.Content = sample
		perQuestion := make(map[attempt.QuestionKey]int, len(records))
		for _, rec := range records {
			perQuestion[rec.Key()]++
		}
		if *questionFlag != "" {
			question, number, ok := sample.Question(*questionFlag)
			if !ok {
				return usageError(opt, *jsonFlag, "unknown_question", fmt.Sprintf("question %q is not in content %s", *questionFlag, sample.ID))
			}
			selection.Question, selection.Number = question, number
		} else if index := firstUnattempted(sample.ID, sample.Questions, perQuestion); index >= 0 {
			selection.Question, selection.Number = sample.Questions[index], index
		} else {
			selection.Question, selection.Number = sample.Questions[0], 0
		}
	}

	if *jsonFlag {
		if !answerSet && !*skipFlag {
			return writeJSON(opt, awaitingDoc{
				SchemaVersion: ProtocolVersion,
				State:         "awaiting_answer",
				Skill:         skillRunViewOf(selection.Skill),
				Content:       contentViewOf(selection.Content),
				Question:      questionViewOf(selection),
			})
		}
		return answerQuestion(opt, selection, st, answerFlag, answerSet, *skipFlag, true)
	}
	return answerQuestion(opt, selection, st, answerFlag, answerSet, *skipFlag, false)
}

func answerQuestion(opt Options, selection practiceSelection, st *store.Store, answerFlag *string, answerSet, skip, jsonMode bool) int {
	started := opt.Now()
	if !jsonMode {
		if selection.Tactic != nil {
			if err := recordGuidedExposure(st, selection.Tactic, selection.Content.ID, selection.Question.ID, started); err != nil {
				return internalError(opt, jsonMode, err)
			}
			writeTacticPreamble(opt.Stdout, selection.Tactic)
		}
		if selection.Skill != nil {
			writeSkillPreamble(opt.Stdout, selection.Skill)
		}
		writePrompt(opt.Stdout, selection.Content, selection.Question, selection.Number)
	}

	chosen := attempt.Selection{Skip: skip}
	if answerSet {
		parsed, err := attempt.ParseSelection(selection.Question, *answerFlag)
		if err != nil {
			return usageError(opt, jsonMode, "invalid_answer", err.Error())
		}
		chosen = parsed
	} else if !skip {
		var (
			recorded bool
			code     int
		)
		chosen, recorded, code = readSelection(opt, selection.Question)
		if code != 0 {
			return code
		}
		if !recorded {
			fmt.Fprintln(opt.Stdout, "No attempt recorded.")
			return 0
		}
	}

	answered := opt.Now()
	record, err := attempt.BuildRecord(selection.Content.ID, selection.Content.Version, selection.Question, chosen, opt.NewSession(), started, answered)
	if err != nil {
		return internalError(opt, jsonMode, err)
	}
	record, err = annotateAssistance(st, selection, record)
	if err != nil {
		return internalError(opt, jsonMode, err)
	}
	record, err = st.Append(record)
	if err != nil {
		return internalError(opt, jsonMode, err)
	}
	feedback := attempt.Explain(selection.Question, record)

	if jsonMode {
		return writeJSON(opt, answeredDoc{
			SchemaVersion: ProtocolVersion,
			State:         "answered",
			Tactic:        tacticRunViewOf(selection.Tactic),
			Guidance:      selection.Tactic.deliveredGuidance(),
			Skill:         skillRunViewOf(selection.Skill),
			Content:       contentViewOf(selection.Content),
			Question:      questionViewOf(selection),
			Attempt:       attemptViewOf(record),
			Feedback:      feedbackViewOf(feedback),
		})
	}
	skillID := ""
	if selection.Skill != nil {
		skillID = selection.Skill.Skill.ID
	}
	writeFeedback(opt.Stdout, record, feedback, st.Path(), skillID)
	if selection.Skill != nil {
		writeSkillExposureLine(opt.Stdout, selection.Skill, record)
	}
	return 0
}

// annotateAssistance records the condition under which the attempt was made. A
// tactic run carries its stage and id. Ordinary practice has no tactic id, but
// a question whose guided walkthrough was already delivered is still assisted:
// the help was shown even though this command did not show it.
func annotateAssistance(st *store.Store, selection practiceSelection, record attempt.Attempt) (attempt.Attempt, error) {
	if selection.Tactic != nil {
		return record.WithTactic(selection.Tactic.ID, selection.Tactic.assistance())
	}
	exposed, err := st.Exposed(record.Key())
	if err != nil {
		return attempt.Attempt{}, err
	}
	if !exposed {
		return record, nil
	}
	return record.WithAssistance(attempt.AssistanceAssisted)
}

// readSelection prompts until it gets an option, "skip", or an exit request.
// recorded is false when the learner quit or input ended before a valid answer.
func readSelection(opt Options, question exam.Question) (selection attempt.Selection, recorded bool, code int) {
	reader := bufio.NewReader(opt.Stdin)
	labels := optionLabels(question)
	for {
		fmt.Fprintf(opt.Stdout, "Your answer (%s; skip or quit): ", labels)
		line, readErr := reader.ReadString('\n')
		fmt.Fprintln(opt.Stdout)
		value := strings.TrimSpace(line)
		switch strings.ToLower(value) {
		case "quit", "q":
			return attempt.Selection{}, false, 0
		}
		if value == "" && errors.Is(readErr, io.EOF) {
			return attempt.Selection{}, false, 0
		}
		if !errors.Is(readErr, io.EOF) && readErr != nil {
			fmt.Fprintf(opt.Stderr, "passquest: read answer: %v\n", readErr)
			return attempt.Selection{}, false, 1
		}
		selection, err := attempt.ParseSelection(question, value)
		if err == nil {
			return selection, true, 0
		}
		fmt.Fprintf(opt.Stdout, "Enter %s, skip, or quit.\n", labels)
		if errors.Is(readErr, io.EOF) {
			return attempt.Selection{}, false, 0
		}
	}
}

func optionLabels(q exam.Question) string {
	labels := make([]string, 0, len(q.Options))
	for _, opt := range q.Options {
		labels = append(labels, opt.Label)
	}
	return strings.Join(labels, ", ")
}

func writePrompt(w io.Writer, content exam.Exam, question exam.Question, number int) {
	fmt.Fprintf(w, "Pass Quest — practice (%s v%s)\n", content.ID, content.Version)
	fmt.Fprintf(w, "%s\n\n", content.Title)
	fmt.Fprintf(w, "Notice: %s\n\n", content.Notice)
	fmt.Fprintf(w, "Passage: %s\n%s\n\n", content.Passage.Title, content.Passage.Text)
	fmt.Fprintf(w, "Question %d of %d (%s)\n%s\n\n", number+1, len(content.Questions), question.ID, question.Stem)
	for _, opt := range question.Options {
		fmt.Fprintf(w, "  %s. %s\n", opt.Label, opt.Text)
	}
	fmt.Fprintln(w)
}

func writeFeedback(w io.Writer, record attempt.Attempt, feedback attempt.Feedback, dbPath, skillID string) {
	if record.Status == attempt.StatusSkipped {
		fmt.Fprintln(w, "Result: SKIPPED (no answer chosen)")
	} else {
		fmt.Fprintf(w, "Result: %s\n", strings.ToUpper(string(record.Status)))
		fmt.Fprintf(w, "Your answer: %s — %s\n", record.SelectedLabel, record.SelectedText)
	}
	if record.Status == attempt.StatusIncorrect {
		fmt.Fprintf(w, "Why your answer is wrong: %s\n", feedback.SelectedReasoning)
	}
	fmt.Fprintf(w, "Correct answer: %s — %s\n", feedback.CorrectLabel, feedback.CorrectText)
	fmt.Fprintf(w, "Evidence: %q\n", feedback.Evidence)
	fmt.Fprintf(w, "Why: %s\n", feedback.Reasoning)
	if record.Repeat() {
		fmt.Fprintln(w, "Repeated attempt: the correct answer and evidence were shown on an earlier attempt, so this is not independent evidence.")
	} else {
		fmt.Fprintln(w, "First attempt on this question; no earlier answer or evidence was shown.")
	}
	switch record.Assistance {
	case attempt.AssistanceAssisted:
		fmt.Fprintln(w, "Assisted attempt: a tactic walkthrough was shown before this answer, so it is not independent evidence.")
	case attempt.AssistanceIndependent:
		if record.TacticID != "" {
			fmt.Fprintln(w, "Independent attempt: no hint was shown and this question had no earlier attempt.")
		}
	}
	condition := fmt.Sprintf("assistance %s", record.Assistance)
	if record.TacticID != "" {
		condition += ", tactic " + record.TacticID
	}
	if skillID != "" {
		condition += ", skill " + skillID
	}
	fmt.Fprintf(w, "Recorded attempt #%d (question %s, attempt %d on this question, %d ms, %s).\n",
		record.ID, record.QuestionID, record.AttemptNumber, record.ElapsedMS, condition)
	fmt.Fprintf(w, "Database: %s\n", dbPath)
}

func runReport(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	fs := newFlagSet("report", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}

	sample, err := exam.Sample()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
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

	ids := uniqueContentIDs(records)
	if len(ids) == 0 {
		ids = []string{sample.ID}
	}
	contents := make([]contentView, 0, len(ids))
	sources := make(map[string]*reportSource)
	var seeds []questionReport
	for _, id := range ids {
		view, questions := reportContentView(st, sample, id, sources)
		contents = append(contents, view)
		for _, q := range questions {
			seeds = append(seeds, questionReport{ID: q.ID, ContentID: id})
		}
	}
	counts, questions := summarize(records, seeds)
	var content *contentView
	if len(contents) == 1 {
		only := contents[0]
		content = &only
	}
	doc := reportDoc{
		SchemaVersion: ProtocolVersion,
		Content:       content,
		Contents:      contents,
		Counts:        counts,
		Questions:     questions,
		Attempts:      make([]attemptView, 0, len(records)),
		Limitations:   limitations,
	}
	for _, record := range records {
		doc.Attempts = append(doc.Attempts, attemptViewOf(record))
	}

	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeReport(opt.Stdout, doc, st.Path())
	return 0
}

func uniqueContentIDs(records []attempt.Attempt) []string {
	seen := make(map[string]bool, len(records))
	var ids []string
	for _, record := range records {
		if !seen[record.ContentID] {
			seen[record.ContentID] = true
			ids = append(ids, record.ContentID)
		}
	}
	return ids
}

type reportSource struct {
	src       source.Source
	questions []source.Question
	err       error
}

// reportContentView caches per-source loads because a report contains one
// content id per page of the same source.
func reportContentView(st *store.Store, sample exam.Exam, id string, sources map[string]*reportSource) (contentView, []exam.Question) {
	if id == sample.ID {
		return contentViewOf(sample), sample.Questions
	}
	if exercise, err := tactic.Exercise(); err == nil && id == exercise.ID {
		return contentViewOf(exercise), exercise.Questions
	}
	sourceID, page, ok := source.ParseContentID(id)
	if !ok {
		return unavailableContentView(id, "unrecognised content id"), nil
	}
	loaded := sources[sourceID]
	if loaded == nil {
		loaded = &reportSource{}
		if loaded.src, loaded.err = st.Source(sourceID); loaded.err == nil {
			loaded.questions, loaded.err = st.Questions(sourceID)
		}
		sources[sourceID] = loaded
	}
	if loaded.err != nil {
		return unavailableContentView(id, loaded.err.Error()), nil
	}
	pageRecord, ok := findPage(loaded.src, page)
	if !ok {
		return unavailableSourceView(id, loaded.src, page, fmt.Errorf("page %d is not in the source", page)), nil
	}
	content, _, err := source.BuildPracticeExam(loaded.src, pageRecord, loaded.questions)
	if err != nil {
		return unavailableSourceView(id, loaded.src, page, err), nil
	}
	return contentViewOf(content), content.Questions
}

func unavailableContentView(id, reason string) contentView {
	return contentView{
		ID:      id,
		Version: "unavailable",
		Title:   fmt.Sprintf("Unavailable content %s", id),
		Notice:  fmt.Sprintf("This build cannot load content %s (%s). Stored attempts remain the evidence and are shown by content id.", id, reason),
		Provenance: provenanceView{
			Author:       "unknown",
			Source:       "local database",
			Rights:       "private local data",
			ExamRelation: "unavailable content; no exam claim is made",
		},
	}
}

func unavailableSourceView(id string, src source.Source, page int, cause error) contentView {
	return contentView{
		ID:      id,
		Version: "unavailable",
		Title:   fmt.Sprintf("%s — page %d", src.Title, page),
		Notice:  fmt.Sprintf("Content is not practiceable in this build: %v. Stored attempts remain the evidence; the source stays private and local.", cause),
		Provenance: provenanceView{
			Author:       src.Title,
			Source:       fmt.Sprintf("local %s import: %s", src.Import, src.Path),
			Rights:       src.Rights,
			ExamRelation: src.ExamRelation,
		},
	}
}

func summarize(records []attempt.Attempt, seeds []questionReport) (countsView, []questionReport) {
	var counts countsView
	index := make(map[attempt.QuestionKey]int, len(seeds))
	questions := append([]questionReport(nil), seeds...)
	for i, q := range questions {
		index[attempt.QuestionKey{ContentID: q.ContentID, QuestionID: q.ID}] = i
	}

	for _, record := range records {
		counts.Attempts++
		switch record.Status {
		case attempt.StatusCorrect:
			counts.Correct++
		case attempt.StatusIncorrect:
			counts.Incorrect++
		case attempt.StatusSkipped:
			counts.Skipped++
		}
		if record.Assistance == attempt.AssistanceAssisted {
			counts.AssistedAttempts++
			switch record.Status {
			case attempt.StatusCorrect:
				counts.AssistedCorrect++
			case attempt.StatusIncorrect:
				counts.AssistedIncorrect++
			case attempt.StatusSkipped:
				counts.AssistedSkipped++
			}
		} else if record.AttemptNumber == 1 {
			// Only a first attempt can be independent evidence; a repeat follows
			// the answer and evidence exposure.
			counts.IndependentAttempts++
			switch record.Status {
			case attempt.StatusCorrect:
				counts.IndependentCorrect++
			case attempt.StatusIncorrect:
				counts.IndependentIncorrect++
			case attempt.StatusSkipped:
				counts.IndependentSkipped++
			}
		}
		if record.AttemptNumber == 1 {
			counts.FirstAttempts++
			switch record.Status {
			case attempt.StatusCorrect:
				counts.FirstCorrect++
			case attempt.StatusIncorrect:
				counts.FirstIncorrect++
			case attempt.StatusSkipped:
				counts.FirstSkipped++
			}
		} else {
			counts.RepeatedAttempts++
			switch record.Status {
			case attempt.StatusCorrect:
				counts.RepeatedCorrect++
			case attempt.StatusIncorrect:
				counts.RepeatedIncorrect++
			case attempt.StatusSkipped:
				counts.RepeatedSkipped++
			}
		}

		key := record.Key()
		i, ok := index[key]
		if !ok {
			i = len(questions)
			index[key] = i
			questions = append(questions, questionReport{ID: record.QuestionID, ContentID: record.ContentID})
		}
		questions[i].Attempts++
		questions[i].LatestStatus = string(record.Status)
		if record.Assistance == attempt.AssistanceAssisted {
			questions[i].Assisted++
		} else if record.AttemptNumber == 1 {
			questions[i].Independent++
		}
		if record.AttemptNumber == 1 {
			questions[i].FirstStatus = string(record.Status)
		} else {
			questions[i].Repeated++
		}
	}
	return counts, questions
}

var limitations = []string{
	"Counts cover only the locally stored practice attempts shown above; they are not an exam score, pass estimate or readiness claim.",
	"Repeated attempts come after the correct answer and evidence were shown and are not independent evidence.",
	"Assisted attempts were made with a tactic walkthrough or other help visible; only independent first attempts without prior answer exposure count as independent correctness.",
	"The exam blueprint, time limit and pass threshold are unknown, so no pass probability is computed.",
	"Imported source content is private local material; page and answer reviews are user-attested and are not an external verification.",
}

func writeReport(w io.Writer, doc reportDoc, dbPath string) {
	primary := ""
	if doc.Content != nil {
		version := ""
		if doc.Content.Version != "unavailable" {
			version = " v" + doc.Content.Version
		}
		fmt.Fprintf(w, "Pass Quest — attempt evidence (%s%s)\n", doc.Content.ID, version)
		fmt.Fprintf(w, "%s\n", doc.Content.Title)
		primary = doc.Content.ID
	} else {
		fmt.Fprintf(w, "Pass Quest — attempt evidence (%d contents)\n", len(doc.Contents))
		fmt.Fprintln(w, "Contents")
		for _, content := range doc.Contents {
			fmt.Fprintf(w, "  %s  %s\n", content.ID, content.Title)
		}
		if len(doc.Contents) > 0 {
			primary = doc.Contents[0].ID
		}
	}
	fmt.Fprintln(w)
	c := doc.Counts
	fmt.Fprintf(w, "Attempts: %d (correct %d, incorrect %d, skipped %d)\n", c.Attempts, c.Correct, c.Incorrect, c.Skipped)
	fmt.Fprintf(w, "First attempts: %d (correct %d, incorrect %d, skipped %d)\n", c.FirstAttempts, c.FirstCorrect, c.FirstIncorrect, c.FirstSkipped)
	fmt.Fprintf(w, "Repeated attempts (answer already shown): %d (correct %d, incorrect %d, skipped %d)\n", c.RepeatedAttempts, c.RepeatedCorrect, c.RepeatedIncorrect, c.RepeatedSkipped)
	fmt.Fprintf(w, "Independent attempts (no hint): %d (correct %d, incorrect %d, skipped %d)\n", c.IndependentAttempts, c.IndependentCorrect, c.IndependentIncorrect, c.IndependentSkipped)
	fmt.Fprintf(w, "Assisted attempts (help shown): %d (correct %d, incorrect %d, skipped %d)\n\n", c.AssistedAttempts, c.AssistedCorrect, c.AssistedIncorrect, c.AssistedSkipped)

	qualify := func(contentID, id string) string {
		if contentID != primary {
			return contentID + "/" + id
		}
		return id
	}
	fmt.Fprintln(w, "Questions")
	for _, q := range doc.Questions {
		first := q.FirstStatus
		if first == "" {
			first = "not attempted"
		}
		latest := q.LatestStatus
		if latest == "" {
			latest = "—"
		}
		fmt.Fprintf(w, "  %s  attempts %d  first %s  latest %s  repeated %d  assisted %d  independent %d\n",
			qualify(q.ContentID, q.ID), q.Attempts, first, latest, q.Repeated, q.Assisted, q.Independent)
	}

	fmt.Fprintln(w, "\nAttempts")
	if len(doc.Attempts) == 0 {
		fmt.Fprintln(w, "  (none recorded)")
	}
	for _, a := range doc.Attempts {
		selected := "—"
		if a.SelectedLabel != nil {
			selected = *a.SelectedLabel
		}
		kind := "first"
		if a.Repeat {
			kind = "repeat"
		}
		condition := a.Assistance
		if a.TacticID != "" {
			condition += " (" + a.TacticID + ")"
		}
		fmt.Fprintf(w, "  #%d  %s  %s  %s  %s  selected %s  %d ms  session %s\n",
			a.ID, qualify(a.ContentID, a.QuestionID), a.Status, kind, condition, selected, a.ElapsedMS, a.SessionID)
	}

	fmt.Fprintln(w, "\nLimitations")
	for _, line := range doc.Limitations {
		fmt.Fprintf(w, "  - %s\n", line)
	}
	fmt.Fprintf(w, "\nDatabase: %s\n", dbPath)
}

func runVersion(opt Options) int {
	content, err := exam.Sample()
	if err != nil {
		return internalError(opt, false, err)
	}
	fmt.Fprintf(opt.Stdout, "passquest 0.1.0-dev (content %s v%s, schema %d)\n", content.ID, content.Version, store.SchemaVersion)
	return 0
}

func usageError(opt Options, jsonMode bool, code, message string) int {
	if jsonMode {
		_ = writeJSONTo(opt.Stderr, errorDoc{
			SchemaVersion: ProtocolVersion,
			Error:         errorBody{Code: code, Message: message},
		})
	} else {
		fmt.Fprintf(opt.Stderr, "passquest: %s\n", message)
	}
	return 2
}

func internalError(opt Options, jsonMode bool, err error) int {
	if jsonMode {
		_ = writeJSONTo(opt.Stderr, errorDoc{
			SchemaVersion: ProtocolVersion,
			Error:         errorBody{Code: "internal_error", Message: err.Error()},
		})
	} else {
		fmt.Fprintf(opt.Stderr, "passquest: %v\n", err)
	}
	return 1
}

func writeJSON(opt Options, value any) int {
	if err := writeJSONTo(opt.Stdout, value); err != nil {
		fmt.Fprintf(opt.Stderr, "passquest: encode JSON: %v\n", err)
		return 1
	}
	return 0
}

func writeJSONTo(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func contentViewOf(content exam.Exam) contentView {
	return contentView{
		ID:      content.ID,
		Version: content.Version,
		Title:   content.Title,
		Notice:  content.Notice,
		Provenance: provenanceView{
			Author:       content.Provenance.Author,
			Source:       content.Provenance.Source,
			Rights:       content.Provenance.Rights,
			ExamRelation: content.Provenance.ExamRelation,
		},
	}
}

func questionViewOf(selection practiceSelection) questionView {
	view := questionView{
		ID:     selection.Question.ID,
		Number: selection.Number + 1,
		Total:  len(selection.Content.Questions),
		Passage: passageView{
			Title: selection.Content.Passage.Title,
			Text:  selection.Content.Passage.Text,
		},
		Stem:    selection.Question.Stem,
		Options: optionViews(selection.Question.Options),
	}
	if selection.Page != nil && selection.Review != nil {
		view.Review = &reviewView{
			PageStatus:         string(selection.Page.Review),
			PageReviewedBy:     selection.Page.ReviewedBy,
			PageReviewedAt:     source.FormatTime(selection.Page.ReviewedAt),
			QuestionStatus:     string(selection.Review.Review),
			QuestionReviewedBy: selection.Review.ReviewedBy,
			QuestionReviewedAt: source.FormatTime(selection.Review.ReviewedAt),
		}
	}
	return view
}

func optionViews(options []exam.Option) []optionView {
	views := make([]optionView, 0, len(options))
	for _, opt := range options {
		views = append(views, optionView{Label: opt.Label, Text: opt.Text})
	}
	return views
}

func attemptViewOf(record attempt.Attempt) attemptView {
	var selectedLabel, selectedText *string
	if record.SelectedLabel != "" {
		label := record.SelectedLabel
		selectedLabel = &label
	}
	if record.SelectedText != "" {
		text := record.SelectedText
		selectedText = &text
	}
	return attemptView{
		ID:             record.ID,
		SessionID:      record.SessionID,
		QuestionID:     record.QuestionID,
		AttemptNumber:  record.AttemptNumber,
		PriorAttempts:  record.PriorAttempts,
		Repeat:         record.Repeat(),
		Assistance:     string(record.Assistance),
		TacticID:       record.TacticID,
		Skills:         append([]string(nil), record.Skills...),
		Status:         string(record.Status),
		SelectedLabel:  selectedLabel,
		SelectedText:   selectedText,
		CorrectLabel:   record.CorrectLabel,
		CorrectText:    record.CorrectText,
		StartedAt:      record.StartedAt.UTC().Format(time.RFC3339Nano),
		AnsweredAt:     record.AnsweredAt.UTC().Format(time.RFC3339Nano),
		ElapsedMS:      record.ElapsedMS,
		ContentID:      record.ContentID,
		ContentVersion: record.ContentVersion,
	}
}

func feedbackViewOf(feedback attempt.Feedback) feedbackView {
	return feedbackView{
		Verdict:           string(feedback.Verdict),
		CorrectLabel:      feedback.CorrectLabel,
		CorrectText:       feedback.CorrectText,
		Evidence:          feedback.Evidence,
		Reasoning:         feedback.Reasoning,
		SelectedReasoning: feedback.SelectedReasoning,
		Exposure:          feedback.Exposure,
		Assistance:        string(feedback.Assistance),
	}
}

// JSON documents. Field names are protocol: tests and golden files pin them.

type contentView struct {
	ID         string         `json:"id"`
	Version    string         `json:"version"`
	Title      string         `json:"title"`
	Notice     string         `json:"notice"`
	Provenance provenanceView `json:"provenance"`
}

type provenanceView struct {
	Author       string `json:"author"`
	Source       string `json:"source"`
	Rights       string `json:"rights"`
	ExamRelation string `json:"exam_relation"`
}

type passageView struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type optionView struct {
	Label string `json:"label"`
	Text  string `json:"text"`
}

type questionView struct {
	ID      string       `json:"id"`
	Number  int          `json:"number"`
	Total   int          `json:"total"`
	Passage passageView  `json:"passage"`
	Stem    string       `json:"stem"`
	Options []optionView `json:"options"`
	Review  *reviewView  `json:"review,omitempty"`
}

type reviewView struct {
	PageStatus         string `json:"page_status"`
	PageReviewedBy     string `json:"page_reviewed_by"`
	PageReviewedAt     string `json:"page_reviewed_at"`
	QuestionStatus     string `json:"question_status"`
	QuestionReviewedBy string `json:"question_reviewed_by"`
	QuestionReviewedAt string `json:"question_reviewed_at"`
}

type awaitingDoc struct {
	SchemaVersion int            `json:"schema_version"`
	State         string         `json:"state"`
	Tactic        *tacticRunView `json:"tactic,omitempty"`
	Guidance      *guidanceView  `json:"guidance,omitempty"`
	Skill         *skillRunView  `json:"skill,omitempty"`
	Content       contentView    `json:"content"`
	Question      questionView   `json:"question"`
}

type attemptView struct {
	ID             int64    `json:"id"`
	SessionID      string   `json:"session_id"`
	QuestionID     string   `json:"question_id"`
	AttemptNumber  int      `json:"attempt_number"`
	PriorAttempts  int      `json:"prior_attempts"`
	Repeat         bool     `json:"repeat"`
	Assistance     string   `json:"assistance"`
	TacticID       string   `json:"tactic_id,omitempty"`
	Skills         []string `json:"skills,omitempty"`
	Status         string   `json:"status"`
	SelectedLabel  *string  `json:"selected_label"`
	SelectedText   *string  `json:"selected_text"`
	CorrectLabel   string   `json:"correct_label"`
	CorrectText    string   `json:"correct_text"`
	StartedAt      string   `json:"started_at"`
	AnsweredAt     string   `json:"answered_at"`
	ElapsedMS      int64    `json:"elapsed_ms"`
	ContentID      string   `json:"content_id"`
	ContentVersion string   `json:"content_version"`
}

type feedbackView struct {
	Verdict           string `json:"verdict"`
	CorrectLabel      string `json:"correct_label"`
	CorrectText       string `json:"correct_text"`
	Evidence          string `json:"evidence"`
	Reasoning         string `json:"reasoning"`
	SelectedReasoning string `json:"selected_reasoning,omitempty"`
	Exposure          string `json:"exposure"`
	Assistance        string `json:"assistance"`
}

type answeredDoc struct {
	SchemaVersion int            `json:"schema_version"`
	State         string         `json:"state"`
	Tactic        *tacticRunView `json:"tactic,omitempty"`
	Guidance      *guidanceView  `json:"guidance,omitempty"`
	Skill         *skillRunView  `json:"skill,omitempty"`
	Content       contentView    `json:"content"`
	Question      questionView   `json:"question"`
	Attempt       attemptView    `json:"attempt"`
	Feedback      feedbackView   `json:"feedback"`
}

type countsView struct {
	Attempts             int `json:"attempts"`
	Correct              int `json:"correct"`
	Incorrect            int `json:"incorrect"`
	Skipped              int `json:"skipped"`
	FirstAttempts        int `json:"first_attempts"`
	FirstCorrect         int `json:"first_correct"`
	FirstIncorrect       int `json:"first_incorrect"`
	FirstSkipped         int `json:"first_skipped"`
	RepeatedAttempts     int `json:"repeated_attempts"`
	RepeatedCorrect      int `json:"repeated_correct"`
	RepeatedIncorrect    int `json:"repeated_incorrect"`
	RepeatedSkipped      int `json:"repeated_skipped"`
	AssistedAttempts     int `json:"assisted_attempts"`
	AssistedCorrect      int `json:"assisted_correct"`
	AssistedIncorrect    int `json:"assisted_incorrect"`
	AssistedSkipped      int `json:"assisted_skipped"`
	IndependentAttempts  int `json:"independent_attempts"`
	IndependentCorrect   int `json:"independent_correct"`
	IndependentIncorrect int `json:"independent_incorrect"`
	IndependentSkipped   int `json:"independent_skipped"`
}

type questionReport struct {
	ID           string `json:"id"`
	ContentID    string `json:"content_id"`
	Attempts     int    `json:"attempts"`
	FirstStatus  string `json:"first_status"`
	LatestStatus string `json:"latest_status"`
	Repeated     int    `json:"repeated"`
	Assisted     int    `json:"assisted"`
	Independent  int    `json:"independent"`
}

type reportDoc struct {
	SchemaVersion int              `json:"schema_version"`
	Content       *contentView     `json:"content,omitempty"`
	Contents      []contentView    `json:"contents"`
	Counts        countsView       `json:"counts"`
	Questions     []questionReport `json:"questions"`
	Attempts      []attemptView    `json:"attempts"`
	Limitations   []string         `json:"limitations"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorDoc struct {
	SchemaVersion int       `json:"schema_version"`
	Error         errorBody `json:"error"`
}
