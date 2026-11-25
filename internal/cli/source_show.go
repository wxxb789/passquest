package cli

import (
	"fmt"

	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
)

func runSources(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"db": true})
	fs := newFlagSet("sources", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return fail(opt, *jsonFlag, err)
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
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	defer st.Close()
	src, questions, err := study.LoadSourceBundle(st, sourceID)
	if err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	summary := source.SummaryOf(src, questions)

	var page *source.Page
	if *pageFlag != 0 {
		candidate, ok := study.FindPage(src, *pageFlag)
		if !ok {
			return usageError(opt, *jsonFlag, "not_found", fmt.Sprintf("page %d is not in source %s", *pageFlag, sourceID))
		}
		page = &candidate
	}

	if code := recordSourceQuestionPresentations(opt, st, sourceID, questions, page, 0, assess.SurfaceShow); code != 0 {
		return code
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
			qPage, _ := study.FindPage(src, q.Page)
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
			page.Origin, page.Review, study.DisplayFlags(page.Flags), page.Revision, source.CountNonSpace(page.Text))
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
		qPage, _ := study.FindPage(src, q.Page)
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
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	defer st.Close()
	src, questions, err := study.LoadSourceBundle(st, fs.Arg(0))
	if err != nil {
		return sourceError(opt, *jsonFlag, err)
	}
	if *pageFlag != 0 {
		if _, ok := study.FindPage(src, *pageFlag); !ok {
			return usageError(opt, *jsonFlag, "not_found", fmt.Sprintf("page %d is not in source %s", *pageFlag, src.ID))
		}
	}

	if code := recordSourceQuestionPresentations(opt, st, src.ID, questions, nil, *pageFlag, assess.SurfaceQuestion); code != 0 {
		return code
	}

	if *jsonFlag {
		doc := questionsDoc{SchemaVersion: ProtocolVersion, Source: sourceSummaryViewOf(source.SummaryOf(src, questions)), Questions: make([]sourceQuestionView, 0)}
		for _, q := range questions {
			if *pageFlag != 0 && q.Page != *pageFlag {
				continue
			}
			qPage, _ := study.FindPage(src, q.Page)
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
		qPage, _ := study.FindPage(src, q.Page)
		fmt.Fprintln(opt.Stdout, "  "+questionLine(q, qPage))
	}
	fmt.Fprintf(opt.Stdout, "\nDatabase: %s\n", st.Path())
	return 0
}

// recordSourceQuestionPresentations appends one exposure per question a source
// inspection command will print. A page selected with --page narrows the set;
// a whole-source listing spends every unit whose question stem is shown.
//
// An explicitly displayed page also gets one page-level row with an empty
// question_id, independent of the questions currently authored on it: the
// passage itself is the exposure, so a page with no questions still spends its
// unit and a question added there later cannot enter the pool unseen. The
// per-question rows stay for the stems actually printed. Both kinds are rows in
// the append-only content_presentations table, and the pool counts one spend per
// content unit.
func recordSourceQuestionPresentations(opt Options, st *store.Store, sourceID string, questions []source.Question, page *source.Page, pageFilter int, surface string) int {
	at := opt.PresentationNow()
	entries := make([]store.Presentation, 0, len(questions)+1)
	if page != nil {
		entries = append(entries, store.Presentation{
			ContentID: source.ContentID(sourceID, page.Number),
			Surface:   surface,
			At:        at,
		})
	}
	for _, q := range questions {
		if page != nil && q.Page != page.Number {
			continue
		}
		if pageFilter != 0 && q.Page != pageFilter {
			continue
		}
		entries = append(entries, store.Presentation{
			ContentID:  source.ContentID(sourceID, q.Page),
			QuestionID: q.ID,
			Surface:    surface,
			At:         at,
		})
	}
	if err := st.RecordPresentations(entries); err != nil {
		return internalError(opt, false, err)
	}
	return 0
}
