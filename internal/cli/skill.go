package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/skill"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/practice"
)

// Passage exposure values are part of the skill practice protocol. A fresh
// passage can supply independent transfer evidence; an already-seen passage
// cannot, even when the question is new. The practice module's values are the
// one source of truth.
const passageFresh = practice.PassageFresh

// skillRun is the CLI's view of one skill practice question.
type skillRun struct {
	Skill           skill.Skill
	Assistance      attempt.Assistance
	PassageExposure string
}

// skillRunOf maps a practice skill run to the CLI's display view.
func skillRunOf(run *practice.SkillRun) *skillRun {
	if run == nil {
		return nil
	}
	return &skillRun{Skill: run.Skill, Assistance: run.Assistance, PassageExposure: run.PassageExposure}
}

func runSkill(args []string, opt Options) int {
	if code := guardAssessmentCommand(args, opt, "skill"); code != 0 {
		return code
	}
	if len(args) == 0 {
		return usageError(opt, jsonFlagRequested(args), "invalid_input", "skill requires list, show or practice")
	}
	switch args[0] {
	case "list":
		return runSkillList(args[1:], opt)
	case "show":
		return runSkillShow(args[1:], opt)
	case "practice":
		return runSkillPractice(args[1:], opt)
	default:
		return usageError(opt, jsonFlagRequested(args), "invalid_input", fmt.Sprintf("unknown skill command %q", args[0]))
	}
}

func runSkillList(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	fs := newFlagSet("skill list", opt, wantJSON)
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	all := skill.All()
	if *jsonFlag {
		doc := skillListDoc{SchemaVersion: ProtocolVersion, Skills: make([]skillListView, 0, len(all))}
		for _, candidate := range all {
			doc.Skills = append(doc.Skills, skillListView{
				ID: candidate.ID, Kind: string(candidate.Kind), Title: candidate.Title, Summary: candidate.Summary,
			})
		}
		return writeJSON(opt, doc)
	}
	fmt.Fprintln(opt.Stdout, "Pass Quest — skills")
	for _, candidate := range all {
		fmt.Fprintf(opt.Stdout, "  %-24s %-8s %s\n", candidate.ID, candidate.Kind, candidate.Title)
		fmt.Fprintf(opt.Stdout, "    %s\n", candidate.Summary)
	}
	return 0
}

func runSkillShow(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{})
	fs := newFlagSet("skill show", opt, wantJSON)
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 1, "skill show requires exactly one skill id"); code != 0 {
		return code
	}
	lesson, ok := skill.Get(fs.Arg(0))
	if !ok {
		return usageError(opt, *jsonFlag, "unknown_skill",
			fmt.Sprintf("skill %q is not registered; available: %s", fs.Arg(0), strings.Join(skill.Registered(), ", ")))
	}
	view := skillLessonViewOf(lesson)
	if *jsonFlag {
		return writeJSON(opt, skillShowDoc{SchemaVersion: ProtocolVersion, Skill: view})
	}
	writeSkillLesson(opt.Stdout, view)
	return 0
}

func runSkillPractice(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{
		"source": true, "question": true, "answer": true, "db": true,
	})
	fs := newFlagSet("skill practice", opt, wantJSON)
	sourceFlag := fs.String("source", "", "imported source id to practice")
	questionFlag := fs.String("question", "", "question id to practice")
	answerFlag := fs.String("answer", "", "answer label (A, B, C, ...)")
	skipFlag := fs.Bool("skip", false, "record a skipped attempt")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 1, "skill practice requires exactly one skill id"); code != 0 {
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
	lesson, ok := skill.Get(fs.Arg(0))
	if !ok {
		return usageError(opt, *jsonFlag, "unknown_skill",
			fmt.Sprintf("skill %q is not registered; available: %s", fs.Arg(0), strings.Join(skill.Registered(), ", ")))
	}

	ports, err := opt.ports(*dbFlag)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	svc, err := study.Open(ports)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	item, err := practice.Show(context.Background(), svc, practice.Request{
		Kind:       practice.KindSkill,
		SkillID:    lesson.ID,
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
			Skill:         skillRunViewOf(skillRunOf(item.Skill)),
			Content:       contentViewOf(item.Content),
			Question:      questionViewOf(item),
		})
	}
	return answerItem(opt, svc, item, answerFlag, answerSet, *skipFlag, *jsonFlag)
}

func skillRunViewOf(run *skillRun) *skillRunView {
	if run == nil {
		return nil
	}
	return &skillRunView{
		ID:              run.Skill.ID,
		Title:           run.Skill.Title,
		Kind:            string(run.Skill.Kind),
		Assistance:      string(run.Assistance),
		PassageExposure: run.PassageExposure,
	}
}

func writeSkillPreamble(w io.Writer, run *skillRun) {
	fmt.Fprintf(w, "Pass Quest — skill: %s (%s)\n", run.Skill.Title, run.Skill.Kind)
	if run.Assistance == attempt.AssistanceAssisted {
		fmt.Fprintln(w, "A tactic walkthrough was previously shown for this question; this attempt is recorded as assisted, not independent.")
	}
	switch run.PassageExposure {
	case passageFresh:
		if run.Assistance == attempt.AssistanceIndependent {
			fmt.Fprintln(w, "No hint is shown. This is the first recorded attempt on this passage, so a correct answer can be independent transfer evidence.")
		} else {
			fmt.Fprintln(w, "This is the first recorded attempt on this passage, but the earlier walkthrough makes this attempt assisted; it is not independent transfer evidence.")
		}
	default:
		fmt.Fprintln(w, "This passage was already seen in an earlier attempt. The attempt is repeat exposure, not independent transfer evidence.")
	}
	fmt.Fprintln(w)
}

func writeSkillExposureLine(w io.Writer, run *skillRun, record attempt.Attempt) {
	switch {
	case record.Assistance == attempt.AssistanceAssisted:
		fmt.Fprintln(w, "Passage exposure: "+run.PassageExposure+". The attempt is assisted, so it is not independent transfer evidence.")
	case run.PassageExposure == passageFresh:
		fmt.Fprintln(w, "Passage exposure: fresh. This is the first recorded attempt on this passage and no hint was shown.")
	default:
		fmt.Fprintln(w, "Passage exposure: already seen. This attempt is repeat exposure, not independent transfer evidence.")
	}
}

func writeSkillLesson(w io.Writer, lesson skillLessonView) {
	fmt.Fprintf(w, "Pass Quest — skill %s\n", lesson.ID)
	fmt.Fprintf(w, "Kind: %s\n", lesson.Kind)
	fmt.Fprintf(w, "Title: %s\n", lesson.Title)
	fmt.Fprintf(w, "Summary: %s\n", lesson.Summary)
	fmt.Fprintf(w, "Principle: %s\n", lesson.Principle)
	fmt.Fprintf(w, "Why it works: %s\n", lesson.WhyItWorks)
	fmt.Fprintf(w, "When it works: %s\n", lesson.WhenItWorks)
	fmt.Fprintln(w, "Steps:")
	for i, step := range lesson.Steps {
		fmt.Fprintf(w, "  %d. %s\n", i+1, step)
	}
	fmt.Fprintf(w, "Watch out: %s\n", lesson.WatchOut)
	if lesson.TacticID != "" {
		fmt.Fprintf(w, "Full tactic lesson: passquest tactic show %s\n", lesson.TacticID)
	}
}

func skillLessonViewOf(lesson skill.Skill) skillLessonView {
	return skillLessonView{
		ID:          lesson.ID,
		Kind:        string(lesson.Kind),
		Title:       lesson.Title,
		Summary:     lesson.Summary,
		Principle:   lesson.Principle,
		WhyItWorks:  lesson.WhyItWorks,
		WhenItWorks: lesson.WhenItWorks,
		Steps:       append([]string(nil), lesson.Steps...),
		WatchOut:    lesson.WatchOut,
		TacticID:    lesson.TacticID,
	}
}

// JSON documents. Field names are protocol: tests and golden files pin them.

type skillListView struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

type skillListDoc struct {
	SchemaVersion int             `json:"schema_version"`
	Skills        []skillListView `json:"skills"`
}

type skillLessonView struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Principle   string   `json:"principle"`
	WhyItWorks  string   `json:"why_it_works"`
	WhenItWorks string   `json:"when_it_works"`
	Steps       []string `json:"steps"`
	WatchOut    string   `json:"watch_out"`
	TacticID    string   `json:"tactic_id,omitempty"`
}

type skillShowDoc struct {
	SchemaVersion int             `json:"schema_version"`
	Skill         skillLessonView `json:"skill"`
}

type skillRunView struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Kind            string `json:"kind"`
	Assistance      string `json:"assistance"`
	PassageExposure string `json:"passage_exposure"`
}
