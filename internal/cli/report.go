package cli

import (
	"fmt"
	"io"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
)

func runReport(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	fs := newFlagSet("report", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}

	sample, err := exam.Sample()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
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

	ids := uniqueContentIDs(records)
	if len(ids) == 0 {
		ids = []string{sample.ID}
	}
	contents := make([]contentView, 0, len(ids))
	sources := make(map[string]*reportSource)
	var seeds []questionReport
	for _, id := range ids {
		view, questions := reportContentView(st, sample, id, sources)
		contents = append(contents, view)
		for _, q := range questions {
			seeds = append(seeds, questionReport{ID: q.ID, ContentID: id})
		}
	}
	counts, questions := summarize(records, seeds)
	var content *contentView
	if len(contents) == 1 {
		only := contents[0]
		content = &only
	}
	doc := reportDoc{
		SchemaVersion: ProtocolVersion,
		Content:       content,
		Contents:      contents,
		Counts:        counts,
		Questions:     questions,
		Attempts:      make([]attemptView, 0, len(records)),
		Limitations:   limitations,
	}
	for _, record := range records {
		doc.Attempts = append(doc.Attempts, attemptViewOf(record))
	}

	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeReport(opt.Stdout, doc, st.Path())
	return 0
}

func uniqueContentIDs(records []attempt.Attempt) []string {
	seen := make(map[string]bool, len(records))
	var ids []string
	for _, record := range records {
		if !seen[record.ContentID] {
			seen[record.ContentID] = true
			ids = append(ids, record.ContentID)
		}
	}
	return ids
}

type reportSource struct {
	src       source.Source
	questions []source.Question
	err       error
}

// reportContentView caches per-source loads because a report contains one
// content id per page of the same source.
func reportContentView(st *store.Store, sample exam.Exam, id string, sources map[string]*reportSource) (contentView, []exam.Question) {
	if id == sample.ID {
		return contentViewOf(sample), sample.Questions
	}
	if content, found, err := study.EmbeddedContent(id); err != nil {
		return unavailableContentView(id, err.Error()), nil
	} else if found {
		return contentViewOf(content), content.Questions
	}
	sourceID, page, ok := source.ParseContentID(id)
	if !ok {
		return unavailableContentView(id, "unrecognised content id"), nil
	}
	loaded := sources[sourceID]
	if loaded == nil {
		loaded = &reportSource{}
		if loaded.src, loaded.err = st.Source(sourceID); loaded.err == nil {
			loaded.questions, loaded.err = st.Questions(sourceID)
		}
		sources[sourceID] = loaded
	}
	if loaded.err != nil {
		return unavailableContentView(id, loaded.err.Error()), nil
	}
	pageRecord, ok := study.FindPage(loaded.src, page)
	if !ok {
		return unavailableSourceView(id, loaded.src, page, fmt.Errorf("page %d is not in the source", page)), nil
	}
	content, _, err := source.BuildPracticeExam(loaded.src, pageRecord, loaded.questions)
	if err != nil {
		return unavailableSourceView(id, loaded.src, page, err), nil
	}
	return contentViewOf(content), content.Questions
}

func unavailableContentView(id, reason string) contentView {
	return contentView{
		ID:      id,
		Version: "unavailable",
		Title:   fmt.Sprintf("Unavailable content %s", id),
		Notice:  fmt.Sprintf("This build cannot load content %s (%s). Stored attempts remain the evidence and are shown by content id.", id, reason),
		Provenance: provenanceView{
			Author:       "unknown",
			Source:       "local database",
			Rights:       "private local data",
			ExamRelation: "unavailable content; no exam claim is made",
		},
	}
}

func unavailableSourceView(id string, src source.Source, page int, cause error) contentView {
	return contentView{
		ID:      id,
		Version: "unavailable",
		Title:   fmt.Sprintf("%s — page %d", src.Title, page),
		Notice:  fmt.Sprintf("Content is not practiceable in this build: %v. Stored attempts remain the evidence; the source stays private and local.", cause),
		Provenance: provenanceView{
			Author:       src.Title,
			Source:       fmt.Sprintf("local %s import: %s", src.Import, src.Path),
			Rights:       src.Rights,
			ExamRelation: src.ExamRelation,
		},
	}
}

func summarize(records []attempt.Attempt, seeds []questionReport) (countsView, []questionReport) {
	var counts countsView
	index := make(map[attempt.QuestionKey]int, len(seeds))
	questions := append([]questionReport(nil), seeds...)
	for i, q := range questions {
		index[attempt.QuestionKey{ContentID: q.ContentID, QuestionID: q.ID}] = i
	}

	for _, record := range records {
		counts.Attempts++
		switch record.Status {
		case attempt.StatusCorrect:
			counts.Correct++
		case attempt.StatusIncorrect:
			counts.Incorrect++
		case attempt.StatusSkipped:
			counts.Skipped++
		}
		if record.Assistance == attempt.AssistanceAssisted {
			counts.AssistedAttempts++
			switch record.Status {
			case attempt.StatusCorrect:
				counts.AssistedCorrect++
			case attempt.StatusIncorrect:
				counts.AssistedIncorrect++
			case attempt.StatusSkipped:
				counts.AssistedSkipped++
			}
		} else if record.AttemptNumber == 1 {
			// Only a first attempt can be independent evidence; a repeat follows
			// the answer and evidence exposure.
			counts.IndependentAttempts++
			switch record.Status {
			case attempt.StatusCorrect:
				counts.IndependentCorrect++
			case attempt.StatusIncorrect:
				counts.IndependentIncorrect++
			case attempt.StatusSkipped:
				counts.IndependentSkipped++
			}
		}
		if record.AttemptNumber == 1 {
			counts.FirstAttempts++
			switch record.Status {
			case attempt.StatusCorrect:
				counts.FirstCorrect++
			case attempt.StatusIncorrect:
				counts.FirstIncorrect++
			case attempt.StatusSkipped:
				counts.FirstSkipped++
			}
		} else {
			counts.RepeatedAttempts++
			switch record.Status {
			case attempt.StatusCorrect:
				counts.RepeatedCorrect++
			case attempt.StatusIncorrect:
				counts.RepeatedIncorrect++
			case attempt.StatusSkipped:
				counts.RepeatedSkipped++
			}
		}

		key := record.Key()
		i, ok := index[key]
		if !ok {
			i = len(questions)
			index[key] = i
			questions = append(questions, questionReport{ID: record.QuestionID, ContentID: record.ContentID})
		}
		questions[i].Attempts++
		questions[i].LatestStatus = string(record.Status)
		if record.Assistance == attempt.AssistanceAssisted {
			questions[i].Assisted++
		} else if record.AttemptNumber == 1 {
			questions[i].Independent++
		}
		if record.AttemptNumber == 1 {
			questions[i].FirstStatus = string(record.Status)
		} else {
			questions[i].Repeated++
		}
	}
	return counts, questions
}

var limitations = []string{
	"Counts cover only the locally stored practice attempts shown above; they are not an exam score, pass estimate or readiness claim.",
	"Repeated attempts come after the correct answer and evidence were shown and are not independent evidence.",
	"Assisted attempts were made with a tactic walkthrough or other help visible; only independent first attempts without prior answer exposure count as independent correctness.",
	"The exam blueprint, time limit and pass threshold are unknown, so no pass probability is computed.",
	"Imported source content is private local material; page and answer reviews are user-attested and are not an external verification.",
}

func writeReport(w io.Writer, doc reportDoc, dbPath string) {
	primary := ""
	if doc.Content != nil {
		version := ""
		if doc.Content.Version != "unavailable" {
			version = " v" + doc.Content.Version
		}
		fmt.Fprintf(w, "Pass Quest — attempt evidence (%s%s)\n", doc.Content.ID, version)
		fmt.Fprintf(w, "%s\n", doc.Content.Title)
		primary = doc.Content.ID
	} else {
		fmt.Fprintf(w, "Pass Quest — attempt evidence (%d contents)\n", len(doc.Contents))
		fmt.Fprintln(w, "Contents")
		for _, content := range doc.Contents {
			fmt.Fprintf(w, "  %s  %s\n", content.ID, content.Title)
		}
		if len(doc.Contents) > 0 {
			primary = doc.Contents[0].ID
		}
	}
	fmt.Fprintln(w)
	c := doc.Counts
	fmt.Fprintf(w, "Attempts: %d (correct %d, incorrect %d, skipped %d)\n", c.Attempts, c.Correct, c.Incorrect, c.Skipped)
	fmt.Fprintf(w, "First attempts: %d (correct %d, incorrect %d, skipped %d)\n", c.FirstAttempts, c.FirstCorrect, c.FirstIncorrect, c.FirstSkipped)
	fmt.Fprintf(w, "Repeated attempts (answer already shown): %d (correct %d, incorrect %d, skipped %d)\n", c.RepeatedAttempts, c.RepeatedCorrect, c.RepeatedIncorrect, c.RepeatedSkipped)
	fmt.Fprintf(w, "Independent attempts (no hint): %d (correct %d, incorrect %d, skipped %d)\n", c.IndependentAttempts, c.IndependentCorrect, c.IndependentIncorrect, c.IndependentSkipped)
	fmt.Fprintf(w, "Assisted attempts (help shown): %d (correct %d, incorrect %d, skipped %d)\n\n", c.AssistedAttempts, c.AssistedCorrect, c.AssistedIncorrect, c.AssistedSkipped)

	qualify := func(contentID, id string) string {
		if contentID != primary {
			return contentID + "/" + id
		}
		return id
	}
	fmt.Fprintln(w, "Questions")
	for _, q := range doc.Questions {
		first := q.FirstStatus
		if first == "" {
			first = "not attempted"
		}
		latest := q.LatestStatus
		if latest == "" {
			latest = "—"
		}
		fmt.Fprintf(w, "  %s  attempts %d  first %s  latest %s  repeated %d  assisted %d  independent %d\n",
			qualify(q.ContentID, q.ID), q.Attempts, first, latest, q.Repeated, q.Assisted, q.Independent)
	}

	fmt.Fprintln(w, "\nAttempts")
	if len(doc.Attempts) == 0 {
		fmt.Fprintln(w, "  (none recorded)")
	}
	for _, a := range doc.Attempts {
		selected := "—"
		if a.SelectedLabel != nil {
			selected = *a.SelectedLabel
		}
		kind := "first"
		if a.Repeat {
			kind = "repeat"
		}
		condition := a.Assistance
		if a.TacticID != "" {
			condition += " (" + a.TacticID + ")"
		}
		fmt.Fprintf(w, "  #%d  %s  %s  %s  %s  selected %s  %d ms  session %s\n",
			a.ID, qualify(a.ContentID, a.QuestionID), a.Status, kind, condition, selected, a.ElapsedMS, a.SessionID)
	}

	fmt.Fprintln(w, "\nLimitations")
	for _, line := range doc.Limitations {
		fmt.Fprintf(w, "  - %s\n", line)
	}
	fmt.Fprintf(w, "\nDatabase: %s\n", dbPath)
}
