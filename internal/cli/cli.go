// Package cli implements the pre-GUI access surface: a scriptable practice
// command and an evidence report over the append-only attempts store.
package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/agent"
	"github.com/wxxb789/passquest/internal/anki"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/search"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/tutor"
	"github.com/wxxb789/passquest/internal/userdata"
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
	// ConfigDir resolves the per-user configuration root the default data
	// directory lives under. Tests inject a temp root; production uses
	// os.UserConfigDir, the same read the window's own resolution makes.
	ConfigDir func() (string, error)
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
	// notice is the run's once-per-run state for the old-default notice. It is
	// unexported so a caller never supplies it.
	notice *noticeState
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
		o.NewSession = study.NewSessionID
	}
	if o.Getenv == nil {
		o.Getenv = os.Getenv
	}
	if o.ConfigDir == nil {
		o.ConfigDir = os.UserConfigDir
	}
	if o.notice == nil {
		o.notice = &noticeState{}
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

// database is the database one command opens, resolved by resolveDatabase.
type database struct {
	Path    string
	Private bool
}

// resolveDatabase resolves the database a command opens through
// internal/userdata: --db, then PASSQUEST_DB, then passquest.sqlite in the
// per-user data directory. Private is true only for the app-owned per-user
// default: a path chosen with --db or PASSQUEST_DB, or a directory chosen with
// PASSQUEST_DESKTOP_DATA, is the learner's and is not tightened. An
// unresolvable per-user directory is a refusal naming the overrides, never a
// fallback. A run that resolves the default warns about an old working
// directory database once per run, before the command's own work.
func (o Options) resolveDatabase(dbFlag string) (database, error) {
	path, err := study.DatabasePath(dbFlag, o.ConfigDir, o.Getenv)
	if err != nil {
		return database{}, study.Refuse("config_dir_unavailable", "%s", study.ConfigDirUnavailableMessage(err))
	}
	if strings.TrimSpace(dbFlag) == "" && strings.TrimSpace(o.Getenv(userdata.DatabaseEnv)) == "" {
		o.noticeOldDefault(path)
	}
	return database{Path: path, Private: privateDefault(dbFlag, o.Getenv)}, nil
}

// ports is the composition root's port set for one command in the database
// dbFlag selects: the process seams withDefaults filled, plus the
// private-directory decision resolveDatabase makes.
func (o Options) ports(dbFlag string) (study.Ports, error) {
	db, err := o.resolveDatabase(dbFlag)
	if err != nil {
		return study.Ports{}, err
	}
	return o.portsOf(db), nil
}

// portsOf is ports for an already resolved database, so a caller that also
// needs the path (next's database comparison) resolves once.
func (o Options) portsOf(db database) study.Ports {
	p := study.Ports{
		DBPath:          db.Path,
		PrivateDir:      db.Private,
		Now:             o.Now,
		PresentationNow: o.PresentationNow,
		NewSession:      o.NewSession,
		NewRunID:        o.NewRunID,
		NewHostToken:    o.NewHostToken,
		Getenv:          o.Getenv,
		Tutor:           o.TutorTransport,
		Anki:            o.AnkiTransport,
		Search:          o.SearchTransport,
		Agent:           o.AgentRunner,
		KillGrace:       o.agentKillGrace,
	}
	study.Complete(&p)
	return p
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
Default database: $PASSQUEST_DB or passquest.sqlite in the per-user data directory (docs/install.md)
`

// privateDefault reports whether only the app-owned per-user directory is in
// use: neither --db nor PASSQUEST_DB selects a database, and no
// PASSQUEST_DESKTOP_DATA selects a directory. That directory is the app's own
// and is tightened to 0700; an overridden one is the learner's explicit choice,
// taken as given exactly as the window takes it.
func privateDefault(flagValue string, getenv func(string) string) bool {
	return strings.TrimSpace(flagValue) == "" &&
		strings.TrimSpace(getenv(userdata.DatabaseEnv)) == "" &&
		strings.TrimSpace(getenv(userdata.DataEnv)) == ""
}

// openDatabase opens the resolved database path. The app-owned default
// directory is tightened to 0700; a directory the learner selected is left to
// them.
func openDatabase(flagValue string, opt Options) (*store.Store, error) {
	db, err := opt.resolveDatabase(flagValue)
	if err != nil {
		return nil, err
	}
	if db.Private {
		return store.OpenPrivate(db.Path)
	}
	return store.Open(db.Path)
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

func runVersion(opt Options) int {
	content, err := exam.Sample()
	if err != nil {
		return internalError(opt, false, err)
	}
	fmt.Fprintf(opt.Stdout, "passquest 0.1.0-dev (content %s v%s, schema %d)\n", content.ID, content.Version, store.SchemaVersion)
	return 0
}
