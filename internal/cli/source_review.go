package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
)

func runPage(args []string, opt Options) int {
	if len(args) == 0 {
		return usageError(opt, jsonFlagRequested(args), "invalid_input", "page requires edit or review")
	}
	switch args[0] {
	case "edit":
		return runPageEdit(args[1:], opt)
	case "review":
		return runPageReview(args[1:], opt)
	default:
		return usageError(opt, jsonFlagRequested(args), "invalid_input", fmt.Sprintf("unknown page command %q", args[0]))
	}
}

func runPageEdit(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"text-file": true, "reviewer": true, "note": true, "db": true})
	fs := newFlagSet("page edit", opt, wantJSON)
	textFile := fs.String("text-file", "", "UTF-8 file with the corrected page text, or - for stdin")
	reviewer := fs.String("reviewer", "", "local reviewer label")
	note := fs.String("note", "", "why the text changed")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 2, "page edit requires SOURCE and PAGE"); code != 0 {
		return code
	}
	sourceID := fs.Arg(0)
	page, code := parsePageArg(fs.Arg(1), opt, *jsonFlag)
	if code != 0 {
		return code
	}
	if *textFile == "" {
		return usageError(opt, *jsonFlag, "invalid_input", "--text-file is required")
	}
	if code := requireReviewer(*reviewer, opt, *jsonFlag); code != 0 {
		return code
	}
	text, err := readInputFile(*textFile, opt)
	if err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	if !source.ValidUTF8(string(text)) {
		return usageError(opt, *jsonFlag, "invalid_input", "corrected page text is not valid UTF-8")
	}

	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	corrected, err := st.CorrectPage(sourceID, page, string(text), *note, *reviewer, opt.Now())
	if err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	if *jsonFlag {
		return writeJSON(opt, pageCommandDoc{
			SchemaVersion: ProtocolVersion,
			State:         "page_edited",
			Page:          pageDetailViewOf(corrected),
		})
	}
	fmt.Fprintf(opt.Stdout, "Page %d corrected: revision %d, review reset to %s.\nFlags: %s\nDatabase: %s\n",
		corrected.Number, corrected.Revision, corrected.Review, study.DisplayFlags(corrected.Flags), st.Path())
	return 0
}

func runPageReview(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"reviewer": true, "note": true, "db": true})
	fs := newFlagSet("page review", opt, wantJSON)
	reviewer := fs.String("reviewer", "", "local reviewer label")
	note := fs.String("note", "", "review note")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 2, "page review requires SOURCE and PAGE"); code != 0 {
		return code
	}
	sourceID := fs.Arg(0)
	page, code := parsePageArg(fs.Arg(1), opt, *jsonFlag)
	if code != 0 {
		return code
	}
	if code := requireReviewer(*reviewer, opt, *jsonFlag); code != 0 {
		return code
	}
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	reviewed, err := st.ReviewPage(sourceID, page, *reviewer, *note, opt.Now())
	if err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	if *jsonFlag {
		return writeJSON(opt, pageCommandDoc{
			SchemaVersion: ProtocolVersion,
			State:         "page_reviewed",
			Page:          pageDetailViewOf(reviewed),
		})
	}
	fmt.Fprintf(opt.Stdout, "Page %d reviewed by %s.\nFlags: %s\nDatabase: %s\n",
		reviewed.Number, reviewed.ReviewedBy, study.DisplayFlags(reviewed.Flags), st.Path())
	return 0
}

func runQuestion(args []string, opt Options) int {
	if len(args) == 0 {
		return usageError(opt, jsonFlagRequested(args), "invalid_input", "question requires add, edit or review")
	}
	switch args[0] {
	case "add":
		return runQuestionAdd(args[1:], opt)
	case "edit":
		return runQuestionEdit(args[1:], opt)
	case "review":
		return runQuestionReview(args[1:], opt)
	default:
		return usageError(opt, jsonFlagRequested(args), "invalid_input", fmt.Sprintf("unknown question command %q", args[0]))
	}
}

func runQuestionAdd(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"file": true, "db": true})
	fs := newFlagSet("question add", opt, wantJSON)
	fileFlag := fs.String("file", "", "question JSON file, or - for stdin")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 2, "question add requires SOURCE and PAGE"); code != 0 {
		return code
	}
	sourceID := fs.Arg(0)
	page, code := parsePageArg(fs.Arg(1), opt, *jsonFlag)
	if code != 0 {
		return code
	}
	if *fileFlag == "" {
		return usageError(opt, *jsonFlag, "invalid_input", "--file is required")
	}
	data, err := readInputFile(*fileFlag, opt)
	if err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	q, err := source.ParseQuestionJSON(data, sourceID, page, opt.Now(), "")
	if err != nil {
		return sourceError(opt, *jsonFlag, err)
	}

	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	if _, err := st.Question(sourceID, q.ID); err == nil {
		return usageError(opt, *jsonFlag, "invalid_input", fmt.Sprintf("question %q already exists in source %s", q.ID, sourceID))
	} else if !errors.Is(err, store.ErrNotFound) {
		return internalError(opt, *jsonFlag, err)
	}
	if err := st.AddQuestion(q); err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	pageRecord, err := st.Page(sourceID, page)
	if err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	if *jsonFlag {
		return writeJSON(opt, questionCommandDoc{
			SchemaVersion: ProtocolVersion,
			State:         "question_added",
			Question:      sourceQuestionViewOf(q, pageRecord),
		})
	}
	fmt.Fprintf(opt.Stdout, "Question %s added to page %d: review %s, answer %s.\nDatabase: %s\n",
		q.ID, q.Page, q.Review, q.AnswerStatus, st.Path())
	return 0
}

func runQuestionEdit(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"file": true, "db": true})
	fs := newFlagSet("question edit", opt, wantJSON)
	fileFlag := fs.String("file", "", "question JSON file, or - for stdin")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 2, "question edit requires SOURCE and QUESTION"); code != 0 {
		return code
	}
	sourceID, questionID := fs.Arg(0), fs.Arg(1)
	if *fileFlag == "" {
		return usageError(opt, *jsonFlag, "invalid_input", "--file is required")
	}
	data, err := readInputFile(*fileFlag, opt)
	if err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	existing, err := st.Question(sourceID, questionID)
	if err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	replacement, err := source.ParseQuestionJSON(data, sourceID, existing.Page, opt.Now(), questionID)
	if err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	edited := existing.Edited(replacement, opt.Now())
	if err := st.SaveQuestion(edited, store.EventQuestionEdited); err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	page, err := st.Page(sourceID, edited.Page)
	if err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	if *jsonFlag {
		return writeJSON(opt, questionCommandDoc{
			SchemaVersion: ProtocolVersion,
			State:         "question_edited",
			Question:      sourceQuestionViewOf(edited, page),
		})
	}
	fmt.Fprintf(opt.Stdout, "Question %s edited: review reset to %s, answer %s.\nDatabase: %s\n",
		edited.ID, edited.Review, edited.AnswerStatus, st.Path())
	return 0
}

func runQuestionReview(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"answer": true, "reviewer": true, "db": true})
	fs := newFlagSet("question review", opt, wantJSON)
	answerFlag := fs.String("answer", "", "confirmed answer label")
	reviewer := fs.String("reviewer", "", "local reviewer label")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 2, "question review requires SOURCE and QUESTION"); code != 0 {
		return code
	}
	if strings.TrimSpace(*answerFlag) == "" {
		return usageError(opt, *jsonFlag, "invalid_input", "--answer is required")
	}
	if code := requireReviewer(*reviewer, opt, *jsonFlag); code != 0 {
		return code
	}
	sourceID, questionID := fs.Arg(0), fs.Arg(1)
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	existing, err := st.Question(sourceID, questionID)
	if err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	page, err := st.Page(sourceID, existing.Page)
	if err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	reviewed, err := existing.ReviewAnswer(page, *answerFlag, *reviewer, opt.Now())
	if err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	if err := st.SaveQuestion(reviewed, store.EventQuestionReviewed); err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	if *jsonFlag {
		return writeJSON(opt, questionCommandDoc{
			SchemaVersion: ProtocolVersion,
			State:         "question_reviewed",
			Question:      sourceQuestionViewOf(reviewed, page),
		})
	}
	fmt.Fprintf(opt.Stdout, "Question %s reviewed: answer %s, reviewed by %s.\nDatabase: %s\n",
		reviewed.ID, reviewed.Answer, reviewed.ReviewedBy, st.Path())
	return 0
}
