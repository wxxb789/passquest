package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
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

func pageLine(page source.Page) string {
	reviewed := string(page.Review)
	if page.ReviewedBy != "" {
		reviewed += " by " + page.ReviewedBy
	}
	return fmt.Sprintf("page %03d  %-7s  %-10s  flags %s  %d chars  rev %d",
		page.Number, page.Origin, reviewed, study.DisplayFlags(page.Flags), source.CountNonSpace(page.Text), page.Revision)
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
		q.ID, q.Page, answer, reviewed, study.DisplayFlags(flags), q.Stem)
	if len(q.Tactics) > 0 {
		line += "  skills " + strings.Join(q.Tactics, ",")
	}
	return line
}
