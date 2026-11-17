package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/practice"
)

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

	request := practice.Request{Kind: practice.KindSample, QuestionID: *questionFlag}
	if *sourceFlag != "" {
		request = practice.Request{Kind: practice.KindSource, SourceID: *sourceFlag, Page: *pageFlag, QuestionID: *questionFlag}
	}
	svc, err := study.Open(opt.ports(*dbFlag))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	item, err := practice.Show(context.Background(), svc, request, practiceHold(*jsonFlag, answerSet, *skipFlag))
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

// practiceHold is the display sequence a CLI invocation uses: the human
// prompt, the --json awaiting document, or (with --answer or --skip) a
// one-shot answer that records the presentation after the attempt.
func practiceHold(jsonMode, answerSet, skip bool) practice.Hold {
	if !jsonMode {
		return practice.HoldPrompt
	}
	if answerSet || skip {
		return practice.HoldOneShot
	}
	return practice.HoldDocument
}

// answerItem is the shared tail of the practice adapters: it prompts for the
// choice, asks the practice module to grade it, and writes the protocol
// document or the human feedback.
func answerItem(opt Options, svc *study.Service, item practice.Item, answerFlag *string, answerSet, skip, jsonMode bool) int {
	if !jsonMode {
		if item.Tactic != nil {
			writeTacticPreamble(opt.Stdout, tacticRunOf(item.Tactic))
		}
		if item.Skill != nil {
			writeSkillPreamble(opt.Stdout, skillRunOf(item.Skill))
		}
		writePrompt(opt.Stdout, item.Content, item.Question, item.Number)
	}

	choice := practice.Choice{Skip: skip}
	if answerSet {
		choice.Label = *answerFlag
	} else if !skip {
		selection, recorded, code := readSelection(opt, item.Question)
		if code != 0 {
			return code
		}
		if !recorded {
			fmt.Fprintln(opt.Stdout, "No attempt recorded.")
			return 0
		}
		choice.Parsed = &selection
	}
	graded, err := practice.Answer(context.Background(), svc, item, choice)
	if err != nil {
		return fail(opt, jsonMode, err)
	}

	if jsonMode {
		return writeJSON(opt, answeredDoc{
			SchemaVersion: ProtocolVersion,
			State:         "answered",
			Tactic:        tacticRunViewOf(tacticRunOf(item.Tactic)),
			Guidance:      deliveredGuidanceOf(item.Tactic),
			Skill:         skillRunViewOf(skillRunOf(item.Skill)),
			Content:       contentViewOf(item.Content),
			Question:      questionViewOf(item),
			Attempt:       attemptViewOf(graded.Attempt),
			Feedback:      feedbackViewOf(graded.Feedback),
		})
	}
	skillID := ""
	if item.Skill != nil {
		skillID = item.Skill.Skill.ID
	}
	writeFeedback(opt.Stdout, item.Question, graded.Attempt, graded.Feedback, svc.Path(), skillID)
	if item.Skill != nil {
		writeSkillExposureLine(opt.Stdout, skillRunOf(item.Skill), graded.Attempt)
	}
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
		text, ok := study.OptionExplanation(question.OptionReasoning, record.SelectedLabel)
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
