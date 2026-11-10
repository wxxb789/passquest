package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/skill"
	"github.com/wxxb789/passquest/internal/store"
)

// Passage exposure values are part of the skill practice protocol. A fresh
// passage can supply independent transfer evidence; an already-seen passage
// cannot, even when the question is new.
const (
	passageFresh = "fresh"
	passageSeen  = "already_seen"
)

// skillRun is the CLI's view of one skill practice question.
type skillRun struct {
	Skill           skill.Skill
	Assistance      attempt.Assistance
	PassageExposure string
}

func runSkill(args []string, opt Options) int {
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

	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	records, err := st.List()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	if err := requireGuidedTactic(lesson, records); err != nil {
		return tacticError(opt, *jsonFlag, err)
	}
	exposures, err := st.ListExposures()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	candidates, err := skillCandidates(st, lesson, *sourceFlag)
	if err != nil {
		return tacticError(opt, *jsonFlag, err)
	}
	candidate, err := pickSkillCandidate(lesson, *questionFlag, candidates, records, exposures)
	if err != nil {
		return tacticError(opt, *jsonFlag, err)
	}

	exposed, err := st.Exposed(candidate.key())
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	assistance := attempt.AssistanceIndependent
	if exposed {
		assistance = attempt.AssistanceAssisted
	}
	selection := practiceSelection{
		Content:  candidate.Content,
		Question: candidate.Question,
		Number:   candidate.Number,
		Page:     candidate.Page,
		Review:   candidate.Review,
		Skill: &skillRun{
			Skill:           lesson,
			Assistance:      assistance,
			PassageExposure: skillPassageExposure(candidate.Content.ID, records),
		},
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

func skillCandidates(st *store.Store, lesson skill.Skill, sourceID string) ([]practiceCandidate, error) {
	if strings.TrimSpace(sourceID) != "" {
		return sourceCandidates(st, sourceID, lesson.ID)
	}

	tagged, err := skill.Tagged(lesson.ID)
	if err != nil {
		return nil, err
	}
	out := make([]practiceCandidate, 0, len(tagged))
	for _, exercise := range tagged {
		out = append(out, practiceCandidate{
			Content: exercise.Content, Question: exercise.Question, Number: exercise.Number,
		})
	}
	if len(out) == 0 {
		return nil, refuse("not_practiceable", "the first-party exercises have no question tagged %q", lesson.ID)
	}
	return out, nil
}

// pickSkillCandidate prefers material that can add independent transfer: a
// fresh passage first, then an unattempted question on a seen passage, then a
// repeat. A question whose guided walkthrough was already shown is skipped in
// the first two passes because the attempt would be assisted, not independent;
// an explicit --question still selects it and is recorded as assisted. A repeat
// is allowed but its exposure is printed and never counted as independent
// transfer by the adaptive policy.
func pickSkillCandidate(lesson skill.Skill, questionID string, candidates []practiceCandidate, records []attempt.Attempt, exposures []store.Exposure) (practiceCandidate, error) {
	attempted := make(map[attempt.QuestionKey]bool, len(records))
	seenContent := make(map[string]bool)
	for _, record := range records {
		attempted[record.Key()] = true
		seenContent[record.ContentID] = true
	}
	exposed := make(map[attempt.QuestionKey]bool, len(exposures))
	for _, exposure := range exposures {
		exposed[exposure.Key()] = true
	}
	if strings.TrimSpace(questionID) != "" {
		for _, candidate := range candidates {
			if candidate.Question.ID == questionID {
				return candidate, nil
			}
		}
		return practiceCandidate{}, refuse("invalid_input",
			"question %q is not a practiceable question tagged %q in this material", questionID, lesson.ID)
	}
	for _, candidate := range candidates {
		if !seenContent[candidate.Content.ID] && !exposed[candidate.key()] {
			return candidate, nil
		}
	}
	for _, candidate := range candidates {
		if !attempted[candidate.key()] && !exposed[candidate.key()] {
			return candidate, nil
		}
	}
	return candidates[0], nil
}

// requireGuidedTactic applies Q-0003-02 to the scoring skill's practice surface.
// Ordinary language practice measures a prerequisite; scoring practice records
// independent evidence about a tactic, so it must follow the guided walkthrough
// rather than bypass it. Without the rule, a skill-practice answer on the
// guided-example question silently consumes it and makes the tactic's own
// independent stage unreachable.
func requireGuidedTactic(lesson skill.Skill, records []attempt.Attempt) error {
	if lesson.Kind != skill.KindScoring {
		return nil
	}
	for _, record := range records {
		if record.TacticID == lesson.ID && record.Assistance == attempt.AssistanceAssisted {
			return nil
		}
	}
	return refuse("guided_required",
		"complete a guided %s attempt before independent %s practice; run passquest tactic practice %s --stage guided",
		lesson.ID, lesson.ID, lesson.ID)
}

func skillPassageExposure(contentID string, records []attempt.Attempt) string {
	for _, record := range records {
		if record.ContentID == contentID {
			return passageSeen
		}
	}
	return passageFresh
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
