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
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/store"
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
  passquest practice [--db PATH] [--question ID] [--answer LABEL | --skip] [--json]
  passquest report   [--db PATH] [--json]
  passquest version
  passquest help

The shipped exercise is a self-authored sample, not live exam content. No exam
score, pass threshold or readiness probability is computed. Default database:
$PASSQUEST_DB or ./.passquest/passquest.sqlite
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

func runPractice(args []string, opt Options) int {
	fs := flag.NewFlagSet("practice", flag.ContinueOnError)
	fs.SetOutput(opt.Stderr)
	dbFlag := fs.String("db", "", "SQLite database path")
	questionFlag := fs.String("question", "", "question id to practice")
	answerFlag := fs.String("answer", "", "answer label (A, B, C, ...)")
	skipFlag := fs.Bool("skip", false, "record a skipped attempt")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(opt.Stderr, "passquest: unexpected argument %q\n", fs.Arg(0))
		return 2
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

	content, err := exam.Sample()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	st, err := store.Open(databasePath(*dbFlag, opt.Getenv))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()

	records, err := st.List()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	perQuestion := make(map[string]int, len(records))
	for _, rec := range records {
		perQuestion[rec.QuestionID]++
	}

	var (
		question exam.Question
		number   int
	)
	if *questionFlag != "" {
		var ok bool
		question, number, ok = content.Question(*questionFlag)
		if !ok {
			return usageError(opt, *jsonFlag, "unknown_question", fmt.Sprintf("question %q is not in content %s", *questionFlag, content.ID))
		}
	} else {
		for i, candidate := range content.Questions {
			if perQuestion[candidate.ID] == 0 {
				question, number = candidate, i
				break
			}
		}
		if question.ID == "" {
			question, number = content.Questions[0], 0
		}
	}

	if *jsonFlag {
		if !answerSet && !*skipFlag {
			return writeJSON(opt, awaitingDoc{
				SchemaVersion: ProtocolVersion,
				State:         "awaiting_answer",
				Content:       contentViewOf(content),
				Question:      questionViewOf(content, question, number),
			})
		}
		return answerQuestion(opt, content, question, number, st, answerFlag, answerSet, *skipFlag, true)
	}
	return answerQuestion(opt, content, question, number, st, answerFlag, answerSet, *skipFlag, false)
}

func answerQuestion(opt Options, content exam.Exam, question exam.Question, number int, st *store.Store, answerFlag *string, answerSet, skip, jsonMode bool) int {
	started := opt.Now()
	if !jsonMode {
		writePrompt(opt.Stdout, content, question, number)
	}

	selection := attempt.Selection{Skip: skip}
	if answerSet {
		parsed, err := attempt.ParseSelection(question, *answerFlag)
		if err != nil {
			return usageError(opt, jsonMode, "invalid_answer", err.Error())
		}
		selection = parsed
	} else if !skip {
		var (
			recorded bool
			code     int
		)
		selection, recorded, code = readSelection(opt, question)
		if code != 0 {
			return code
		}
		if !recorded {
			fmt.Fprintln(opt.Stdout, "No attempt recorded.")
			return 0
		}
	}

	answered := opt.Now()
	record, err := attempt.BuildRecord(content.ID, content.Version, question, selection, opt.NewSession(), started, answered)
	if err != nil {
		return internalError(opt, jsonMode, err)
	}
	record, err = st.Append(record)
	if err != nil {
		return internalError(opt, jsonMode, err)
	}
	feedback := attempt.Explain(question, record)

	if jsonMode {
		return writeJSON(opt, answeredDoc{
			SchemaVersion: ProtocolVersion,
			State:         "answered",
			Content:       contentViewOf(content),
			Question:      questionViewOf(content, question, number),
			Attempt:       attemptViewOf(record),
			Feedback:      feedbackViewOf(feedback),
		})
	}
	writeFeedback(opt.Stdout, record, feedback, st.Path())
	return 0
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

func writeFeedback(w io.Writer, record attempt.Attempt, feedback attempt.Feedback, dbPath string) {
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
	fmt.Fprintf(w, "Recorded attempt #%d (question %s, attempt %d on this question, %d ms).\n",
		record.ID, record.QuestionID, record.AttemptNumber, record.ElapsedMS)
	fmt.Fprintf(w, "Database: %s\n", dbPath)
}

func runReport(args []string, opt Options) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(opt.Stderr)
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(opt.Stderr, "passquest: unexpected argument %q\n", fs.Arg(0))
		return 2
	}

	content, err := exam.Sample()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	st, err := store.Open(databasePath(*dbFlag, opt.Getenv))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	records, err := st.List()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}

	counts, questions := summarize(records, content)
	doc := reportDoc{
		SchemaVersion: ProtocolVersion,
		Content:       contentViewOf(content),
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

func summarize(records []attempt.Attempt, content exam.Exam) (countsView, []questionReport) {
	var counts countsView
	index := make(map[string]int, len(content.Questions))
	questions := make([]questionReport, 0, len(content.Questions))
	for _, q := range content.Questions {
		index[q.ID] = len(questions)
		questions = append(questions, questionReport{ID: q.ID})
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

		i, ok := index[record.QuestionID]
		if !ok {
			i = len(questions)
			index[record.QuestionID] = i
			questions = append(questions, questionReport{ID: record.QuestionID})
		}
		questions[i].Attempts++
		questions[i].LatestStatus = string(record.Status)
		if record.AttemptNumber == 1 {
			questions[i].FirstStatus = string(record.Status)
		} else {
			questions[i].Repeated++
		}
	}
	return counts, questions
}

var limitations = []string{
	"Counts cover only this self-authored sample; they are not an exam score, pass estimate or readiness claim.",
	"Repeated attempts come after the correct answer and evidence were shown and are not independent evidence.",
	"The exam blueprint, time limit and pass threshold are unknown, so no pass probability is computed.",
}

func writeReport(w io.Writer, doc reportDoc, dbPath string) {
	fmt.Fprintf(w, "Pass Quest — attempt evidence (%s v%s)\n", doc.Content.ID, doc.Content.Version)
	fmt.Fprintf(w, "%s\n\n", doc.Content.Title)
	c := doc.Counts
	fmt.Fprintf(w, "Attempts: %d (correct %d, incorrect %d, skipped %d)\n", c.Attempts, c.Correct, c.Incorrect, c.Skipped)
	fmt.Fprintf(w, "First attempts: %d (correct %d, incorrect %d, skipped %d)\n", c.FirstAttempts, c.FirstCorrect, c.FirstIncorrect, c.FirstSkipped)
	fmt.Fprintf(w, "Repeated attempts (answer already shown): %d (correct %d, incorrect %d, skipped %d)\n\n", c.RepeatedAttempts, c.RepeatedCorrect, c.RepeatedIncorrect, c.RepeatedSkipped)

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
		fmt.Fprintf(w, "  %s  attempts %d  first %s  latest %s  repeated %d\n", q.ID, q.Attempts, first, latest, q.Repeated)
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
		fmt.Fprintf(w, "  #%d  %s  %s  %s  selected %s  %d ms  session %s\n",
			a.ID, a.QuestionID, a.Status, kind, selected, a.ElapsedMS, a.SessionID)
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

func questionViewOf(content exam.Exam, question exam.Question, number int) questionView {
	options := make([]optionView, 0, len(question.Options))
	for _, opt := range question.Options {
		options = append(options, optionView{Label: opt.Label, Text: opt.Text})
	}
	return questionView{
		ID:     question.ID,
		Number: number + 1,
		Total:  len(content.Questions),
		Passage: passageView{
			Title: content.Passage.Title,
			Text:  content.Passage.Text,
		},
		Stem:    question.Stem,
		Options: options,
	}
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
}

type awaitingDoc struct {
	SchemaVersion int          `json:"schema_version"`
	State         string       `json:"state"`
	Content       contentView  `json:"content"`
	Question      questionView `json:"question"`
}

type attemptView struct {
	ID             int64   `json:"id"`
	SessionID      string  `json:"session_id"`
	QuestionID     string  `json:"question_id"`
	AttemptNumber  int     `json:"attempt_number"`
	PriorAttempts  int     `json:"prior_attempts"`
	Repeat         bool    `json:"repeat"`
	Status         string  `json:"status"`
	SelectedLabel  *string `json:"selected_label"`
	SelectedText   *string `json:"selected_text"`
	CorrectLabel   string  `json:"correct_label"`
	CorrectText    string  `json:"correct_text"`
	StartedAt      string  `json:"started_at"`
	AnsweredAt     string  `json:"answered_at"`
	ElapsedMS      int64   `json:"elapsed_ms"`
	ContentID      string  `json:"content_id"`
	ContentVersion string  `json:"content_version"`
}

type feedbackView struct {
	Verdict           string `json:"verdict"`
	CorrectLabel      string `json:"correct_label"`
	CorrectText       string `json:"correct_text"`
	Evidence          string `json:"evidence"`
	Reasoning         string `json:"reasoning"`
	SelectedReasoning string `json:"selected_reasoning,omitempty"`
	Exposure          string `json:"exposure"`
}

type answeredDoc struct {
	SchemaVersion int          `json:"schema_version"`
	State         string       `json:"state"`
	Content       contentView  `json:"content"`
	Question      questionView `json:"question"`
	Attempt       attemptView  `json:"attempt"`
	Feedback      feedbackView `json:"feedback"`
}

type countsView struct {
	Attempts          int `json:"attempts"`
	Correct           int `json:"correct"`
	Incorrect         int `json:"incorrect"`
	Skipped           int `json:"skipped"`
	FirstAttempts     int `json:"first_attempts"`
	FirstCorrect      int `json:"first_correct"`
	FirstIncorrect    int `json:"first_incorrect"`
	FirstSkipped      int `json:"first_skipped"`
	RepeatedAttempts  int `json:"repeated_attempts"`
	RepeatedCorrect   int `json:"repeated_correct"`
	RepeatedIncorrect int `json:"repeated_incorrect"`
	RepeatedSkipped   int `json:"repeated_skipped"`
}

type questionReport struct {
	ID           string `json:"id"`
	Attempts     int    `json:"attempts"`
	FirstStatus  string `json:"first_status"`
	LatestStatus string `json:"latest_status"`
	Repeated     int    `json:"repeated"`
}

type reportDoc struct {
	SchemaVersion int              `json:"schema_version"`
	Content       contentView      `json:"content"`
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
