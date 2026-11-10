package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
)

// reorderArgs moves flags before positionals so "show SOURCE --page 2" works.
// valueFlags names the flags that consume the following argument.
func reorderArgs(args []string, valueFlags map[string]bool) []string {
	var flags, positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if len(arg) > 1 && strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			name := strings.TrimLeft(arg, "-")
			if strings.Contains(name, "=") {
				continue
			}
			if valueFlags[name] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positionals = append(positionals, arg)
	}
	return append(flags, positionals...)
}

func newFlagSet(name string, opt Options, wantJSON bool) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	if wantJSON {
		fs.SetOutput(io.Discard)
	} else {
		fs.SetOutput(opt.Stderr)
	}
	return fs
}

func parseArgs(fs *flag.FlagSet, args []string, opt Options, wantJSON bool, positionals int, usage string) int {
	if err := fs.Parse(args); err != nil {
		if wantJSON {
			return usageError(opt, true, "invalid_flags", err.Error())
		}
		return 2
	}
	if positionals == 0 {
		if fs.NArg() > 0 {
			return usageError(opt, wantJSON, "unexpected_argument", fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
		}
		return 0
	}
	if fs.NArg() != positionals {
		return usageError(opt, wantJSON, "invalid_input", usage)
	}
	return 0
}

func parsePageArg(raw string, opt Options, jsonMode bool) (int, int) {
	page, err := strconv.Atoi(raw)
	if err != nil || page < 1 {
		return 0, usageError(opt, jsonMode, "invalid_input", fmt.Sprintf("invalid page %q", raw))
	}
	return page, 0
}

func requireReviewer(reviewer string, opt Options, jsonMode bool) int {
	if strings.TrimSpace(reviewer) == "" {
		return usageError(opt, jsonMode, "invalid_input", "--reviewer is required")
	}
	return 0
}

func sourceError(opt Options, jsonMode bool, err error) int {
	code := "invalid_input"
	if errors.Is(err, source.ErrNotPracticeable) {
		code = "not_practiceable"
	}
	switch {
	case errors.Is(err, source.ErrInvalidInput),
		errors.Is(err, source.ErrToolMissing),
		errors.Is(err, source.ErrNotPracticeable),
		errors.Is(err, store.ErrNotFound):
		return usageError(opt, jsonMode, code, err.Error())
	default:
		return internalError(opt, jsonMode, err)
	}
}

func runImport(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"path": true, "title": true, "db": true})
	fs := newFlagSet("import", opt, wantJSON)
	pathFlag := fs.String("path", "", "local TXT directory, .txt file or .pdf file")
	titleFlag := fs.String("title", "", "source title")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	if strings.TrimSpace(*pathFlag) == "" {
		return usageError(opt, *jsonFlag, "invalid_input", "--path is required")
	}

	src, err := source.ImportPath(*pathFlag, source.ImportOptions{
		Title: *titleFlag,
		Now:   opt.Now(),
	})
	if err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	st, err := store.Open(databasePath(*dbFlag, opt.Getenv))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	created, err := st.ImportSource(src)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	questions, err := st.Questions(src.ID)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	summary := source.SummaryOf(src, questions)
	state := "already_imported"
	if created {
		state = "imported"
	} else {
		// The second import reuses the stored title/notice, which is the
		// authoritative record; recompute the summary from that record.
		loaded, err := st.Source(src.ID)
		if err != nil {
			return internalError(opt, *jsonFlag, err)
		}
		summary = source.SummaryOf(loaded, questions)
	}

	if *jsonFlag {
		return writeJSON(opt, importDoc{
			SchemaVersion: ProtocolVersion,
			State:         state,
			Source:        sourceSummaryViewOf(summary),
		})
	}
	if created {
		fmt.Fprintf(opt.Stdout, "Imported source %s (%d pages, %d reviewed, %d questions).\n",
			summary.ID, summary.PageCount, summary.ReviewedPages, summary.QuestionCount)
	} else {
		fmt.Fprintf(opt.Stdout, "Source already imported: %s (%d pages, %d reviewed).\n",
			summary.ID, summary.PageCount, summary.ReviewedPages)
	}
	fmt.Fprintf(opt.Stdout, "Title: %s\nOrigin: %s\nDatabase: %s\n", summary.Title, summary.Import, st.Path())
	return 0
}

func runSources(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"db": true})
	fs := newFlagSet("sources", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	st, err := store.Open(databasePath(*dbFlag, opt.Getenv))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	summaries, err := st.Sources()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}

	if *jsonFlag {
		doc := sourcesDoc{SchemaVersion: ProtocolVersion, Sources: make([]sourceSummaryView, 0, len(summaries))}
		for _, summary := range summaries {
			doc.Sources = append(doc.Sources, sourceSummaryViewOf(summary))
		}
		return writeJSON(opt, doc)
	}
	fmt.Fprintln(opt.Stdout, "Pass Quest — imported sources")
	if len(summaries) == 0 {
		fmt.Fprintln(opt.Stdout, "  (none imported; use passquest import --path PATH)")
	}
	for _, summary := range summaries {
		fmt.Fprintf(opt.Stdout, "  %s  pages %d (reviewed %d)  questions %d (reviewed %d)  %s\n",
			summary.ID, summary.PageCount, summary.ReviewedPages, summary.QuestionCount,
			summary.ReviewedQuestions, summary.Import)
		fmt.Fprintf(opt.Stdout, "    %s\n", summary.Title)
	}
	fmt.Fprintf(opt.Stdout, "\nDatabase: %s\n", st.Path())
	return 0
}

func runShow(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"page": true, "db": true})
	fs := newFlagSet("show", opt, wantJSON)
	pageFlag := fs.Int("page", 0, "page number to show in detail")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 1, "show requires exactly one source id"); code != 0 {
		return code
	}
	sourceID := fs.Arg(0)
	st, err := store.Open(databasePath(*dbFlag, opt.Getenv))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	src, questions, err := loadSourceBundle(st, sourceID)
	if err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	summary := source.SummaryOf(src, questions)

	var page *source.Page
	if *pageFlag != 0 {
		candidate, ok := findPage(src, *pageFlag)
		if !ok {
			return usageError(opt, *jsonFlag, "not_found", fmt.Sprintf("page %d is not in source %s", *pageFlag, sourceID))
		}
		page = &candidate
	}

	if *jsonFlag {
		doc := showDoc{
			SchemaVersion: ProtocolVersion,
			Source:        sourceSummaryViewOf(summary),
			Questions:     make([]sourceQuestionView, 0),
		}
		if page != nil {
			detail := pageDetailViewOf(*page)
			doc.Page = &detail
		} else {
			doc.Pages = make([]pageSummaryView, 0, len(src.Pages))
			for _, candidate := range src.Pages {
				doc.Pages = append(doc.Pages, pageSummaryViewOf(candidate))
			}
		}
		for _, q := range questions {
			if page != nil && q.Page != page.Number {
				continue
			}
			qPage, _ := findPage(src, q.Page)
			doc.Questions = append(doc.Questions, sourceQuestionViewOf(q, qPage))
		}
		return writeJSON(opt, doc)
	}

	fmt.Fprintf(opt.Stdout, "Pass Quest — imported source %s\n%s\n", src.ID, src.Title)
	fmt.Fprintf(opt.Stdout, "Origin: %s\nPath: %s\nSHA-256: %s\nCreated: %s\n",
		src.Import, src.Path, src.SHA256, source.FormatTime(src.CreatedAt))
	fmt.Fprintf(opt.Stdout, "Notice: %s\n", src.Notice)
	fmt.Fprintf(opt.Stdout, "Pages: %d (reviewed %d)\n", summary.PageCount, summary.ReviewedPages)
	if page == nil {
		for _, candidate := range src.Pages {
			fmt.Fprintln(opt.Stdout, "  "+pageLine(candidate))
		}
	} else {
		fmt.Fprintf(opt.Stdout, "\nPage %d\n", page.Number)
		fmt.Fprintf(opt.Stdout, "Origin: %s\nReview: %s\nFlags: %s\nRevision: %d\nChars (no whitespace): %d\n",
			page.Origin, page.Review, displayFlags(page.Flags), page.Revision, source.CountNonSpace(page.Text))
		if page.ReviewedBy != "" {
			fmt.Fprintf(opt.Stdout, "Reviewed by: %s at %s\n", page.ReviewedBy, source.FormatTime(page.ReviewedAt))
		}
		if page.ReviewNote != "" {
			fmt.Fprintf(opt.Stdout, "Review note: %s\n", page.ReviewNote)
		}
		fmt.Fprintf(opt.Stdout, "\n%s\n", page.Text)
	}

	fmt.Fprintf(opt.Stdout, "\nQuestions: %d (reviewed %d)\n", summary.QuestionCount, summary.ReviewedQuestions)
	for _, q := range questions {
		if page != nil && q.Page != page.Number {
			continue
		}
		qPage, _ := findPage(src, q.Page)
		fmt.Fprintln(opt.Stdout, "  "+questionLine(q, qPage))
	}
	fmt.Fprintf(opt.Stdout, "\nDatabase: %s\n", st.Path())
	return 0
}

func runQuestions(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"page": true, "db": true})
	fs := newFlagSet("questions", opt, wantJSON)
	pageFlag := fs.Int("page", 0, "only questions on this page")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 1, "questions requires exactly one source id"); code != 0 {
		return code
	}
	st, err := store.Open(databasePath(*dbFlag, opt.Getenv))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	src, questions, err := loadSourceBundle(st, fs.Arg(0))
	if err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	if *pageFlag != 0 {
		if _, ok := findPage(src, *pageFlag); !ok {
			return usageError(opt, *jsonFlag, "not_found", fmt.Sprintf("page %d is not in source %s", *pageFlag, src.ID))
		}
	}

	if *jsonFlag {
		doc := questionsDoc{SchemaVersion: ProtocolVersion, Source: sourceSummaryViewOf(source.SummaryOf(src, questions)), Questions: make([]sourceQuestionView, 0)}
		for _, q := range questions {
			if *pageFlag != 0 && q.Page != *pageFlag {
				continue
			}
			qPage, _ := findPage(src, q.Page)
			doc.Questions = append(doc.Questions, sourceQuestionViewOf(q, qPage))
		}
		return writeJSON(opt, doc)
	}
	fmt.Fprintf(opt.Stdout, "Pass Quest — questions for %s\n", src.ID)
	if len(questions) == 0 {
		fmt.Fprintln(opt.Stdout, "  (none added)")
	}
	for _, q := range questions {
		if *pageFlag != 0 && q.Page != *pageFlag {
			continue
		}
		qPage, _ := findPage(src, q.Page)
		fmt.Fprintln(opt.Stdout, "  "+questionLine(q, qPage))
	}
	fmt.Fprintf(opt.Stdout, "\nDatabase: %s\n", st.Path())
	return 0
}

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

	st, err := store.Open(databasePath(*dbFlag, opt.Getenv))
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
		corrected.Number, corrected.Revision, corrected.Review, displayFlags(corrected.Flags), st.Path())
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
	st, err := store.Open(databasePath(*dbFlag, opt.Getenv))
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
		reviewed.Number, reviewed.ReviewedBy, displayFlags(reviewed.Flags), st.Path())
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

	st, err := store.Open(databasePath(*dbFlag, opt.Getenv))
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
	st, err := store.Open(databasePath(*dbFlag, opt.Getenv))
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
	st, err := store.Open(databasePath(*dbFlag, opt.Getenv))
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

type practiceSelection struct {
	Content  exam.Exam
	Question exam.Question
	Number   int
	Page     *source.Page
	Review   *source.Question
}

func selectSourcePractice(st *store.Store, sourceID string, pageNumber int, questionID string, records []attempt.Attempt) (practiceSelection, error) {
	src, questions, err := loadSourceBundle(st, sourceID)
	if err != nil {
		return practiceSelection{}, err
	}
	if len(src.Pages) == 0 {
		return practiceSelection{}, fmt.Errorf("%w: source %s has no pages", source.ErrNotPracticeable, sourceID)
	}

	attempts := make(map[attempt.QuestionKey]int, len(records))
	for _, rec := range records {
		attempts[rec.Key()]++
	}

	pageIndex := -1
	if pageNumber != 0 {
		for i, page := range src.Pages {
			if page.Number == pageNumber {
				pageIndex = i
				break
			}
		}
		if pageIndex < 0 {
			return practiceSelection{}, fmt.Errorf("%w: page %d is not in source %s", source.ErrInvalidInput, pageNumber, sourceID)
		}
		if questionID != "" {
			q, ok := findQuestion(questions, questionID)
			if !ok {
				return practiceSelection{}, fmt.Errorf("%w: question %q is not in source %s", source.ErrInvalidInput, questionID, sourceID)
			}
			if q.Page != pageNumber {
				return practiceSelection{}, fmt.Errorf("%w: question %q belongs to page %d, not %d", source.ErrInvalidInput, questionID, q.Page, pageNumber)
			}
		}
	} else if questionID != "" {
		q, ok := findQuestion(questions, questionID)
		if !ok {
			return practiceSelection{}, fmt.Errorf("%w: question %q is not in source %s", source.ErrInvalidInput, questionID, sourceID)
		}
		for i, page := range src.Pages {
			if page.Number == q.Page {
				pageIndex = i
				break
			}
		}
		if pageIndex < 0 {
			return practiceSelection{}, fmt.Errorf("%w: question %q points at missing page %d", source.ErrInvalidInput, questionID, q.Page)
		}
	} else {
		// Prefer a page with an unattempted reviewed question; otherwise use the
		// first page that has any reviewed question.
		firstPage := -1
		for i, page := range src.Pages {
			content, _, buildErr := source.BuildPracticeExam(src, page, questions)
			if buildErr != nil {
				continue
			}
			if firstPage < 0 {
				firstPage = i
			}
			if firstUnattempted(content.ID, content.Questions, attempts) >= 0 {
				pageIndex = i
				break
			}
		}
		if pageIndex < 0 {
			pageIndex = firstPage
		}
		if pageIndex < 0 {
			return practiceSelection{}, fmt.Errorf("%w: source %s has no reviewed page and question to practice; use passquest questions %s", source.ErrNotPracticeable, sourceID, sourceID)
		}
	}

	page := src.Pages[pageIndex]
	content, _, err := source.BuildPracticeExam(src, page, questions)
	if err != nil {
		return practiceSelection{}, err
	}

	index := -1
	if questionID != "" {
		for i, candidate := range content.Questions {
			if candidate.ID == questionID {
				index = i
				break
			}
		}
		if index < 0 {
			q, _ := findQuestion(questions, questionID)
			return practiceSelection{}, fmt.Errorf(
				"%w: question %s is not practiceable (review %s, answer %s, flags %s)",
				source.ErrNotPracticeable, questionID, q.Review, q.AnswerStatus, displayFlags(q.Flags(page)))
		}
	} else if i := firstUnattempted(content.ID, content.Questions, attempts); i >= 0 {
		index = i
	} else {
		index = 0
	}

	reviewQuestion, ok := findQuestion(questions, content.Questions[index].ID)
	if !ok {
		return practiceSelection{}, fmt.Errorf("%w: selected question is missing from the source", source.ErrNotPracticeable)
	}
	return practiceSelection{
		Content:  content,
		Question: content.Questions[index],
		Number:   index,
		Page:     &page,
		Review:   &reviewQuestion,
	}, nil
}

func firstUnattempted(contentID string, questions []exam.Question, attempts map[attempt.QuestionKey]int) int {
	for i, q := range questions {
		if attempts[attempt.QuestionKey{ContentID: contentID, QuestionID: q.ID}] == 0 {
			return i
		}
	}
	return -1
}

func loadSourceBundle(st *store.Store, sourceID string) (source.Source, []source.Question, error) {
	src, err := st.Source(sourceID)
	if err != nil {
		return source.Source{}, nil, err
	}
	questions, err := st.Questions(sourceID)
	if err != nil {
		return source.Source{}, nil, err
	}
	return src, questions, nil
}

func findPage(src source.Source, number int) (source.Page, bool) {
	for _, page := range src.Pages {
		if page.Number == number {
			return page, true
		}
	}
	return source.Page{}, false
}

func findQuestion(questions []source.Question, id string) (source.Question, bool) {
	for _, q := range questions {
		if q.ID == id {
			return q, true
		}
	}
	return source.Question{}, false
}

func readInputFile(path string, opt Options) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(opt.Stdin)
		if err != nil {
			return nil, fmt.Errorf("%w: read stdin: %v", source.ErrInvalidInput, err)
		}
		return data, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %v", source.ErrInvalidInput, path, err)
	}
	return data, nil
}

func displayFlags(flags []source.Flag) string {
	if len(flags) == 0 {
		return "—"
	}
	values := source.FlagStrings(flags)
	sort.Strings(values)
	return strings.Join(values, ",")
}

func pageLine(page source.Page) string {
	reviewed := string(page.Review)
	if page.ReviewedBy != "" {
		reviewed += " by " + page.ReviewedBy
	}
	return fmt.Sprintf("page %03d  %-7s  %-10s  flags %s  %d chars  rev %d",
		page.Number, page.Origin, reviewed, displayFlags(page.Flags), source.CountNonSpace(page.Text), page.Revision)
}

func questionLine(q source.Question, page source.Page) string {
	answer := "unmapped"
	if q.AnswerStatus == source.AnswerReviewed {
		answer = q.Answer + " (reviewed)"
	}
	reviewed := string(q.Review)
	if q.ReviewedBy != "" {
		reviewed += " by " + q.ReviewedBy
	}
	flags := q.Flags(page)
	return fmt.Sprintf("%s  page %03d  answer %-14s  review %-10s  flags %s  %s",
		q.ID, q.Page, answer, reviewed, displayFlags(flags), q.Stem)
}
