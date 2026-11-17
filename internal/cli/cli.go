// Package cli implements the pre-GUI access surface: a scriptable practice
// command and an evidence report over the append-only attempts store.
package cli

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/agent"
	"github.com/wxxb789/passquest/internal/anki"
	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/search"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/tutor"
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
	// TutorTransport isolates the CLI from the Node host. Tests inject a fake;
	// production defaults to the stdin/stdout host process.
	TutorTransport tutor.Transport
	// AnkiTransport isolates the AnkiConnect host. Tests inject a fake;
	// production spawns agent/anki-host.mjs.
	AnkiTransport anki.Transport
	// SearchTransport isolates the search host. Tests inject a fake;
	// production spawns agent/search-host.mjs.
	SearchTransport search.Adapter
	// AgentRunner isolates the durable-agent host. Tests inject a fake;
	// production spawns agent/durable-host.mjs with the Pi runtime.
	AgentRunner agent.Runner
	// NewRunID isolates run identity for tests. Production uses a UTC
	// timestamp plus random bits.
	NewRunID func() (string, error)
	// NewHostToken isolates heartbeat identity for tests.
	NewHostToken func() (string, error)
	// PresentationNow stamps side-effect-only exposure rows. It is separate
	// from Now so a command that writes no timestamp into its protocol
	// document does not consume a deterministic test clock reading.
	PresentationNow func() time.Time
	// agentKillGrace bounds how long cancel waits for a hard-killed host's
	// heartbeat to go stale before reporting cancellation unconfirmed. It is
	// unexported so only package tests can shorten it.
	agentKillGrace time.Duration
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
	if o.TutorTransport == nil {
		o.TutorTransport = tutor.HostTransport{Getenv: o.Getenv, Environ: os.Environ}
	}
	if o.AnkiTransport == nil {
		o.AnkiTransport = anki.HostTransport{Getenv: o.Getenv, Environ: os.Environ}
	}
	if o.SearchTransport == nil {
		o.SearchTransport = search.HostTransport{Getenv: o.Getenv, Environ: os.Environ}
	}
	if o.AgentRunner == nil {
		o.AgentRunner = agent.HostRunner{Getenv: o.Getenv, Environ: os.Environ}
	}
	if o.NewRunID == nil {
		o.NewRunID = func() (string, error) { return agent.NewRunID(o.Now()) }
	}
	if o.NewHostToken == nil {
		o.NewHostToken = agent.NewHostToken
	}
	if o.PresentationNow == nil {
		o.PresentationNow = time.Now
	}
	if o.agentKillGrace == 0 {
		o.agentKillGrace = agentKillGrace()
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
	case "tutor":
		return runTutor(args[1:], opt)
	case "model":
		return runModel(args[1:], opt)
	case "agent":
		return runAgent(args[1:], opt)
	case "note":
		return runNote(args[1:], opt)
	case "find":
		return runFind(args[1:], opt)
	case "assess":
		return runAssess(args[1:], opt)
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
  passquest tutor config [--endpoint URL --model NAME] [--timeout-seconds N]
                     [--max-output-tokens N] [--daily-calls N] [--db PATH] [--json]
  passquest tutor preview [--attempt ID] [--mode explain|probe|micro_lesson] [--db PATH] [--json]
  passquest tutor ask     [--attempt ID] [--mode explain|probe|micro_lesson] [--db PATH] [--json]
  passquest tutor history [--attempt ID] [--db PATH] [--json]
  passquest tutor show    CALL [--db PATH] [--json]
  passquest tutor consent SOURCE --reviewer NAME [--db PATH] [--json]
  passquest tutor revoke  SOURCE --reviewer NAME [--db PATH] [--json]
  passquest tutor consents [--db PATH] [--json]
  passquest model list   [--db PATH] [--json]
  passquest model use    PROVIDER/MODEL [--endpoint URL] [--db PATH] [--json]
  passquest model test   PROVIDER/MODEL [--endpoint URL] [--db PATH] [--json]
  passquest model budget [--db PATH] [--json]
  passquest agent start  [--task study-brief] [--max-steps N] [--timeout-seconds N]
                     [--db PATH] [--agent-store PATH] [--json]
  passquest agent status [RUN] [--db PATH] [--agent-store PATH] [--json]
  passquest agent resume [RUN] [--timeout-seconds N] [--db PATH] [--agent-store PATH] [--json]
  passquest agent cancel [RUN] [--wait-seconds N] [--db PATH] [--agent-store PATH] [--json]
  passquest agent runs   [--db PATH] [--json]
  passquest agent show   RUN [--db PATH] [--json]
  passquest agent transcript RUN [--agent-store PATH] [--db PATH] [--limit N] [--json]
  passquest note propose [--attempt ID] [--content CONTENT --question QUESTION] [--mark TEXT]
                     [--refine] [--db PATH] [--json]
  passquest note preview [--draft ID] [--db PATH] [--json]
  passquest note accept  [--draft ID] [--front TEXT] [--back TEXT] [--tactic TEXT]
                     [--db PATH] [--json]
  passquest note skip    [--draft ID] [--db PATH] [--json]
  passquest note show    [NOTE] [--db PATH] [--json]
  passquest note model   [--json]
  passquest note export  [--out PATH] [--db PATH] [--json]
  passquest note push    [--endpoint URL] [--timeout-seconds N] [--force] [--db PATH] [--json]
  passquest find     [--max-queries N] [--max-candidates N] [--timeout-seconds N]
                     [--db PATH] [--json]
  passquest find history [--limit N] [--max-queries N] [--db PATH] [--json]
  passquest find show QUERY [--db PATH] [--json]
  passquest assess pool   [--db PATH] [--json]
  passquest assess start  [--size N] [--minutes M] [--annotations none] [--db PATH] [--json]
  passquest assess resume RUN [--db PATH] [--json]
  passquest assess abandon RUN [--db PATH] [--json]
  passquest assess report RUN [--db PATH] [--json]
  passquest assess compare RUN_A RUN_B [--db PATH] [--json]
  passquest report   [--db PATH] [--json]
  passquest version
  passquest help

Imported pages and questions start unreviewed; only reviewed material can be
practiced. A tactic guided attempt shows the walkthrough and is recorded as
assisted; the independent question is a different, never-exposed, first-attempt,
answer-verified question with no hint. The shipped exercise is a self-authored
sample. No exam score, pass threshold or readiness probability is computed.
Notes are local revisions; AnkiWeb and mobile synchronization are the learner's
own manual actions, and Pass Quest never contacts AnkiWeb or a phone.
Assessments run only on reviewed imported items the learner has never been
shown. Presenting any question spends its whole content unit. During an active
run, tutor, skill and find are refused; no glossary, reasoning, evidence or
correctness feedback is shown. The real pass threshold, item count and time
limit are unknown, so no pass probability is produced.
Default database: $PASSQUEST_DB or ./.passquest/passquest.sqlite
`

func newSessionID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("session-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// defaultDatabasePath is the app-owned relative path resolved when neither
// --db nor PASSQUEST_DB selects a database.
var defaultDatabasePath = filepath.Join(".passquest", "passquest.sqlite")

func databasePath(flagValue string, getenv func(string) string) string {
	if strings.TrimSpace(flagValue) != "" {
		return flagValue
	}
	if v := strings.TrimSpace(getenv("PASSQUEST_DB")); v != "" {
		return v
	}
	return defaultDatabasePath
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
		selection, err = selectSamplePractice(*questionFlag, records)
		if err != nil {
			return tacticError(opt, *jsonFlag, err)
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

// selectSamplePractice picks a question of the embedded sample: the requested
// one, else the first unattempted one, else the first.
func selectSamplePractice(questionID string, records []attempt.Attempt) (practiceSelection, error) {
	sample, err := exam.Sample()
	if err != nil {
		return practiceSelection{}, err
	}
	selection := practiceSelection{Content: sample}
	perQuestion := make(map[attempt.QuestionKey]int, len(records))
	for _, rec := range records {
		perQuestion[rec.Key()]++
	}
	if questionID != "" {
		question, number, ok := sample.Question(questionID)
		if !ok {
			return practiceSelection{}, refuse("unknown_question", "question %q is not in content %s", questionID, sample.ID)
		}
		selection.Question, selection.Number = question, number
	} else if index := firstUnattempted(sample.ID, sample.Questions, perQuestion); index >= 0 {
		selection.Question, selection.Number = sample.Questions[index], index
	} else {
		selection.Question, selection.Number = sample.Questions[0], 0
	}
	return selection, nil
}

// practiceSurface is the holdout-ledger surface of a practice selection.
func practiceSurface(selection practiceSelection) string {
	surface := assess.SurfacePractice
	if selection.Tactic != nil {
		surface = assess.SurfaceTactic
	}
	if selection.Skill != nil {
		surface = assess.SurfaceSkill
	}
	return surface
}

// buildPracticeAttempt grades a selection and records the condition it was
// made under, without storing it.
func buildPracticeAttempt(st *store.Store, selection practiceSelection, chosen attempt.Selection, sessionID string, started, answered time.Time) (attempt.Attempt, error) {
	record, err := attempt.BuildRecord(selection.Content.ID, selection.Content.Version, selection.Question, chosen, sessionID, started, answered)
	if err != nil {
		return attempt.Attempt{}, err
	}
	return annotateAssistance(st, selection, record)
}

func answerQuestion(opt Options, selection practiceSelection, st *store.Store, answerFlag *string, answerSet, skip, jsonMode bool) int {
	surface := practiceSurface(selection)
	started := opt.Now()
	recordPresentation := func() int {
		if err := st.RecordPresentation(selection.Content.ID, selection.Question.ID, surface, started); err != nil {
			return internalError(opt, jsonMode, err)
		}
		return 0
	}
	if !jsonMode {
		if code := recordPresentation(); code != 0 {
			return code
		}

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
	} else if !answerSet && !skip {
		if code := recordPresentation(); code != 0 {
			return code
		}
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
	record, err := buildPracticeAttempt(st, selection, chosen, opt.NewSession(), started, answered)
	if err != nil {
		return internalError(opt, jsonMode, err)
	}
	record, err = st.Append(record)
	if err != nil {
		return internalError(opt, jsonMode, err)
	}
	feedback := attempt.Explain(selection.Question, record)

	if jsonMode {
		if answerSet || skip {
			if code := recordPresentation(); code != 0 {
				return code
			}
		}
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
	writeFeedback(opt.Stdout, selection.Question, record, feedback, st.Path(), skillID)
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
	if len(question.Glossary) > 0 {
		fmt.Fprintln(w, "Glossary:")
		for _, entry := range question.Glossary {
			fmt.Fprintf(w, "  %s — %s\n", entry.Term, entry.Meaning)
		}
	}
	fmt.Fprintln(w)
}

func writeFeedback(w io.Writer, question exam.Question, record attempt.Attempt, feedback attempt.Feedback, dbPath, skillID string) {
	if record.Status == attempt.StatusSkipped {
		fmt.Fprintln(w, "Result: SKIPPED (no answer chosen)")
	} else {
		fmt.Fprintf(w, "Result: %s\n", strings.ToUpper(string(record.Status)))
		fmt.Fprintf(w, "Your answer: %s — %s\n", record.SelectedLabel, record.SelectedText)
	}
	if record.Status == attempt.StatusIncorrect {
		text, ok := optionExplanation(question.OptionReasoning, record.SelectedLabel)
		if !ok {
			text = missingOptionExplanation
		}
		fmt.Fprintf(w, "Why your answer is wrong: %s\n", text)
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
	case attempt.AssistanceNone:
		fmt.Fprintln(w, "Assessment attempt: timed and unassisted; no coaching was available during the run.")
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

func runVersion(opt Options) int {
	content, err := exam.Sample()
	if err != nil {
		return internalError(opt, false, err)
	}
	fmt.Fprintf(opt.Stdout, "passquest 0.1.0-dev (content %s v%s, schema %d)\n", content.ID, content.Version, store.SchemaVersion)
	return 0
}
