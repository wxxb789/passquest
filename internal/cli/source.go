package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
)

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
	st, err := openDatabase(*dbFlag, opt)
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

type practiceSelection struct {
	Content  exam.Exam
	Question exam.Question
	Number   int
	Page     *source.Page
	Review   *source.Question
	Tactic   *tacticRun
	Skill    *skillRun
}

// practiceCandidate is one practiceable question selected by skill tag. Page
// and Review are nil for embedded first-party content.
type practiceCandidate struct {
	Content  exam.Exam
	Question exam.Question
	Number   int
	Page     *source.Page
	Review   *source.Question
}

func (c practiceCandidate) key() attempt.QuestionKey {
	return attempt.QuestionKey{ContentID: c.Content.ID, QuestionID: c.Question.ID}
}

// sourceCandidates scans a reviewed source for every question tagged with the
// skill id. A source without one is a refusal, never a silent fallback to
// embedded material.
func sourceCandidates(st *store.Store, sourceID, skillID string) ([]practiceCandidate, error) {
	src, questions, err := loadSourceBundle(st, sourceID)
	if err != nil {
		return nil, err
	}
	var out []practiceCandidate
	for _, page := range src.Pages {
		content, _, buildErr := source.BuildPracticeExam(src, page, questions)
		if buildErr != nil {
			continue
		}
		for i, question := range content.Questions {
			if !slices.Contains(question.Tactics, skillID) {
				continue
			}
			meta, ok := findQuestion(questions, question.ID)
			if !ok {
				continue
			}
			pageCopy := page
			metaCopy := meta
			out = append(out, practiceCandidate{
				Content: content, Question: question, Number: i, Page: &pageCopy, Review: &metaCopy,
			})
		}
	}
	if len(out) == 0 {
		return nil, refuse("not_practiceable",
			"source %s has no reviewed question tagged %q; add a tagged question with `question add`, then review the page and its answer (see `passquest questions %s`)",
			sourceID, skillID, sourceID)
	}
	return out, nil
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
	line := fmt.Sprintf("%s  page %03d  answer %-14s  review %-10s  flags %s  %s",
		q.ID, q.Page, answer, reviewed, displayFlags(flags), q.Stem)
	if len(q.Tactics) > 0 {
		line += "  skills " + strings.Join(q.Tactics, ",")
	}
	return line
}
