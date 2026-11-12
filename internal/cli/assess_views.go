package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/source"
)

type poolUnitView struct {
	ContentID     string   `json:"content_id"`
	SourceID      string   `json:"source_id"`
	Page          int      `json:"page"`
	QuestionCount int      `json:"question_count"`
	Spent         bool     `json:"spent"`
	SpentBy       []string `json:"spent_by,omitempty"`
	FirstSpentAt  string   `json:"first_spent_at,omitempty"`
}

type poolDoc struct {
	SchemaVersion     int            `json:"schema_version"`
	State             string         `json:"state"`
	QualifyingItems   int            `json:"qualifying_items"`
	QualifyingUnits   int            `json:"qualifying_units"`
	PracticeableItems int            `json:"practiceable_items"`
	PracticeableUnits int            `json:"practiceable_units"`
	SpentUnits        int            `json:"spent_units"`
	FloorItems        int            `json:"floor_items"`
	FloorMet          bool           `json:"floor_met"`
	ShortfallItems    int            `json:"shortfall_items"`
	DefaultSize       int            `json:"default_size"`
	DefaultMinutes    int            `json:"default_minutes"`
	SourceCount       int            `json:"source_count"`
	ContentUnits      []poolUnitView `json:"content_units"`
	Limitations       []string       `json:"limitations"`
}

type assessmentRunView struct {
	ID               int64  `json:"id"`
	Status           string `json:"status"`
	StartedAt        string `json:"started_at"`
	DeadlineAt       string `json:"deadline_at"`
	WindowMinutes    int    `json:"window_minutes"`
	RequestedSize    int    `json:"requested_size"`
	ActualSize       int    `json:"actual_size"`
	PerItemTimeoutMS int64  `json:"per_item_timeout_ms"`
	Annotations      string `json:"annotations"`
	PoolFamily       string `json:"pool_family"`
	SourceCount      int    `json:"source_count"`
	ResumeCount      int    `json:"resume_count"`
	LastResumedAt    string `json:"last_resumed_at,omitempty"`
	CompletedAt      string `json:"completed_at,omitempty"`
	AbandonedAt      string `json:"abandoned_at,omitempty"`
	AbandonedReason  string `json:"abandoned_reason,omitempty"`
}

type assessmentItemOutcomeView struct {
	Position   int    `json:"position"`
	ContentID  string `json:"content_id"`
	QuestionID string `json:"question_id"`
	State      string `json:"state"`
	Status     string `json:"status,omitempty"`
	TimedOut   bool   `json:"timed_out"`
	ElapsedMS  int64  `json:"elapsed_ms"`
	AttemptID  int64  `json:"attempt_id,omitempty"`
}

type assessmentRunResultDoc struct {
	SchemaVersion int                         `json:"schema_version"`
	State         string                      `json:"state"`
	Run           assessmentRunView           `json:"run"`
	Items         []assessmentItemOutcomeView `json:"items"`
	Limitations   []string                    `json:"limitations"`
}

type abandonedDoc struct {
	SchemaVersion int    `json:"schema_version"`
	State         string `json:"state"`
	RunID         int64  `json:"run_id"`
	Reason        string `json:"reason"`
}

type assessCountsView struct {
	Selected            int `json:"selected"`
	Presented           int `json:"presented"`
	Finalized           int `json:"finalized"`
	Scored              int `json:"scored"`
	Correct             int `json:"correct"`
	Total               int `json:"total"`
	Contaminated        int `json:"contaminated"`
	TimedOut            int `json:"timed_out"`
	Skipped             int `json:"skipped"`
	Unavailable         int `json:"unavailable"`
	Pending             int `json:"pending"`
	AnsweredAfterResume int `json:"answered_after_resume"`
}

type assessAccuracyView struct {
	Correct int     `json:"correct"`
	Total   int     `json:"total"`
	Ratio   float64 `json:"ratio"`
	Band    string  `json:"band"`
}

type assessSpeedView struct {
	MedianMS         int64  `json:"median_ms"`
	P90MS            int64  `json:"p90_ms"`
	PerItemTimeoutMS int64  `json:"per_item_timeout_ms"`
	Basis            int    `json:"basis_items"`
	Band             string `json:"band"`
}

type assessSkillView struct {
	SkillID string  `json:"skill_id"`
	Correct int     `json:"correct"`
	Total   int     `json:"total"`
	Ratio   float64 `json:"ratio"`
}

type assessAnnotationView struct {
	Withheld        bool                `json:"withheld,omitempty"`
	Available       bool                `json:"available"`
	Evidence        string              `json:"evidence,omitempty"`
	Reasoning       string              `json:"reasoning,omitempty"`
	OptionReasoning map[string]string   `json:"option_reasoning,omitempty"`
	Glossary        []glossaryEntryView `json:"glossary,omitempty"`
}

type assessReportItemView struct {
	Position    int                  `json:"position"`
	ContentID   string               `json:"content_id"`
	QuestionID  string               `json:"question_id"`
	State       string               `json:"state"`
	Status      string               `json:"status,omitempty"`
	TimedOut    bool                 `json:"timed_out"`
	ElapsedMS   int64                `json:"elapsed_ms"`
	AfterResume bool                 `json:"after_resume"`
	Scored      bool                 `json:"scored"`
	Reasons     []string             `json:"contamination_reasons,omitempty"`
	Annotations assessAnnotationView `json:"annotations"`
}

type assessContaminationView struct {
	PriorExposure      int                    `json:"prior_exposure"`
	RepeatedVariant    int                    `json:"repeated_variant"`
	HintsDuringRun     int                    `json:"hints_during_run"`
	ResumedAfterWindow int                    `json:"resumed_after_window"`
	ExcludedItems      []assessReportItemView `json:"excluded_items"`
}

type assessReportDoc struct {
	SchemaVersion int                     `json:"schema_version"`
	State         string                  `json:"state"`
	Run           assessmentRunView       `json:"run"`
	Counts        assessCountsView        `json:"counts"`
	Accuracy      assessAccuracyView      `json:"accuracy"`
	Speed         assessSpeedView         `json:"speed"`
	Skills        []assessSkillView       `json:"skills"`
	Contamination assessContaminationView `json:"contamination"`
	Items         []assessReportItemView  `json:"items"`
	Unknowns      []string                `json:"unknowns"`
	Limitations   []string                `json:"limitations"`
}

type assessCompareRunView struct {
	Run      assessmentRunView  `json:"run"`
	Counts   assessCountsView   `json:"counts"`
	Accuracy assessAccuracyView `json:"accuracy"`
	Speed    assessSpeedView    `json:"speed"`
	Skills   []assessSkillView  `json:"skills"`
}

type assessComparisonDoc struct {
	SchemaVersion  int                    `json:"schema_version"`
	State          string                 `json:"state"`
	Runs           []assessCompareRunView `json:"runs"`
	AccuracyPoints float64                `json:"accuracy_change_points"`
	MedianChangeMS int64                  `json:"median_change_ms"`
	P90ChangeMS    int64                  `json:"p90_change_ms"`
	TimedOutChange int                    `json:"timed_out_change"`
	ScoredChange   int                    `json:"scored_item_change"`
	Dimensions     map[string]string      `json:"dimensions"`
	Limitations    []string               `json:"limitations"`
}

func runViewOf(run assess.Run) assessmentRunView {
	return assessmentRunView{
		ID:               run.ID,
		Status:           run.Status,
		StartedAt:        source.FormatTime(run.StartedAt),
		DeadlineAt:       source.FormatTime(run.DeadlineAt),
		WindowMinutes:    run.WindowMinutes,
		RequestedSize:    run.RequestedSize,
		ActualSize:       run.ActualSize,
		PerItemTimeoutMS: run.PerItemTimeoutMS,
		Annotations:      run.Annotations,
		PoolFamily:       run.PoolFamily,
		SourceCount:      run.SourceCount,
		ResumeCount:      run.ResumeCount,
		LastResumedAt:    source.FormatTime(run.LastResumedAt),
		CompletedAt:      source.FormatTime(run.CompletedAt),
		AbandonedAt:      source.FormatTime(run.AbandonedAt),
		AbandonedReason:  run.AbandonedReason,
	}
}

func poolDocOf(pool assess.Pool, identities []assess.SourceIdentity) poolDoc {
	doc := poolDoc{
		SchemaVersion:     ProtocolVersion,
		State:             "pool",
		QualifyingItems:   pool.ItemCount,
		QualifyingUnits:   pool.UnitCount,
		PracticeableItems: pool.PracticeableItems,
		PracticeableUnits: pool.PracticeableUnits,
		SpentUnits:        pool.SpentUnits,
		FloorItems:        assess.MinPoolItems,
		FloorMet:          pool.ItemCount >= assess.MinPoolItems,
		ShortfallItems:    poolShortfall(pool),
		DefaultSize:       assess.DefaultSize,
		DefaultMinutes:    assess.DefaultMinutes,
		SourceCount:       len(identities),
		ContentUnits:      make([]poolUnitView, 0, len(pool.Units)),
		Limitations: []string{
			"The pool is counted per content unit (one imported source page): presenting any question on a page spends every question on it.",
			"The qualifying unit count is the binding constraint on how many independent assessments the imported material supports.",
			"No embedded first-party sample or skill passage enters the assessment pool.",
		},
	}
	for _, unit := range pool.Units {
		doc.ContentUnits = append(doc.ContentUnits, poolUnitView{
			ContentID:     unit.ContentID,
			SourceID:      unit.SourceID,
			Page:          unit.Page,
			QuestionCount: len(unit.Questions),
			Spent:         unit.Spent,
			SpentBy:       append([]string(nil), unit.SpentBy...),
			FirstSpentAt:  source.FormatTime(unit.FirstSpentAt),
		})
	}
	sort.SliceStable(doc.ContentUnits, func(i, j int) bool {
		return doc.ContentUnits[i].ContentID < doc.ContentUnits[j].ContentID
	})
	return doc
}

func itemOutcomeViews(items []assess.Item, attempts []attempt.Attempt) []assessmentItemOutcomeView {
	byID := make(map[int64]attempt.Attempt, len(attempts))
	for _, record := range attempts {
		byID[record.ID] = record
	}
	out := make([]assessmentItemOutcomeView, 0, len(items))
	for _, item := range items {
		view := assessmentItemOutcomeView{
			Position:   item.Position,
			ContentID:  item.ContentID,
			QuestionID: item.QuestionID,
			State:      item.State,
			TimedOut:   item.State == assess.StateTimedOut,
			AttemptID:  item.AttemptID,
		}
		if record, ok := byID[item.AttemptID]; ok {
			view.Status = string(record.Status)
			view.ElapsedMS = record.ElapsedMS
		}
		out = append(out, view)
	}
	return out
}

func countsViewOf(counts assess.Counts) assessCountsView {
	return assessCountsView{
		Selected:            counts.Selected,
		Presented:           counts.Presented,
		Finalized:           counts.Finalized,
		Scored:              counts.Scored,
		Correct:             counts.Correct,
		Total:               counts.Total,
		Contaminated:        counts.Contaminated,
		TimedOut:            counts.TimedOut,
		Skipped:             counts.Skipped,
		Unavailable:         counts.Unavailable,
		Pending:             counts.Pending,
		AnsweredAfterResume: counts.AnsweredAfterResume,
	}
}

func accuracyViewOf(accuracy assess.Accuracy) assessAccuracyView {
	return assessAccuracyView{
		Correct: accuracy.Correct,
		Total:   accuracy.Total,
		Ratio:   accuracy.Ratio,
		Band:    accuracy.Band,
	}
}

func speedViewOf(speed assess.Speed) assessSpeedView {
	return assessSpeedView{
		MedianMS:         speed.MedianMS,
		P90MS:            speed.P90MS,
		PerItemTimeoutMS: speed.PerItemTimeoutMS,
		Basis:            speed.Basis,
		Band:             speed.Band,
	}
}

func skillViewsOf(skills []assess.SkillAccuracy) []assessSkillView {
	if len(skills) == 0 {
		return nil
	}
	out := make([]assessSkillView, 0, len(skills))
	for _, skill := range skills {
		out = append(out, assessSkillView{
			SkillID: skill.SkillID, Correct: skill.Correct, Total: skill.Total, Ratio: skill.Ratio,
		})
	}
	return out
}

func reportDocOf(report assess.Report) assessReportDoc {
	doc := assessReportDoc{
		SchemaVersion: ProtocolVersion,
		State:         "report",
		Run:           runViewOf(report.Run),
		Counts:        countsViewOf(report.Counts),
		Accuracy:      accuracyViewOf(report.Accuracy),
		Speed:         speedViewOf(report.Speed),
		Contamination: assessContaminationView{
			PriorExposure:      report.Contamination.PriorExposure,
			RepeatedVariant:    report.Contamination.RepeatedVariant,
			HintsDuringRun:     report.Contamination.HintsDuringRun,
			ResumedAfterWindow: report.Contamination.ResumedAfterWindow,
			ExcludedItems:      make([]assessReportItemView, 0, len(report.Contamination.Excluded)),
		},
		Skills:      skillViewsOf(report.Skills),
		Items:       make([]assessReportItemView, 0, len(report.Items)),
		Unknowns:    append([]string(nil), report.Unknowns...),
		Limitations: append([]string(nil), report.Limitations...),
	}
	for _, item := range report.Items {
		view := reportItemViewOf(item)
		doc.Items = append(doc.Items, view)
	}
	for _, item := range report.Contamination.Excluded {
		doc.Contamination.ExcludedItems = append(doc.Contamination.ExcludedItems, reportItemViewOf(item))
	}
	return doc
}

func reportItemViewOf(item assess.ItemReport) assessReportItemView {
	view := assessReportItemView{
		Position:    item.Position,
		ContentID:   item.ContentID,
		QuestionID:  item.QuestionID,
		State:       item.State,
		Status:      string(item.Status),
		TimedOut:    item.TimedOut,
		ElapsedMS:   item.ElapsedMS,
		AfterResume: item.AfterResume,
		Scored:      item.Scored,
		Reasons:     append([]string(nil), item.Reasons...),
		Annotations: assessAnnotationView{
			Withheld:        item.Annotations.Withheld,
			Available:       item.Annotations.Evidence != "" || item.Annotations.Reasoning != "" || len(item.Annotations.Glossary) > 0 || len(item.Annotations.OptionReasoning) > 0,
			Evidence:        item.Annotations.Evidence,
			Reasoning:       item.Annotations.Reasoning,
			OptionReasoning: item.Annotations.OptionReasoning,
			Glossary:        glossaryViews(item.Annotations.Glossary),
		},
	}
	return view
}

func comparisonDocOf(a, b assess.Report) assessComparisonDoc {
	doc := assessComparisonDoc{
		SchemaVersion:  ProtocolVersion,
		State:          "compared",
		Runs:           []assessCompareRunView{compareRunViewOf(a), compareRunViewOf(b)},
		AccuracyPoints: (b.Accuracy.Ratio - a.Accuracy.Ratio) * 100,
		MedianChangeMS: b.Speed.MedianMS - a.Speed.MedianMS,
		P90ChangeMS:    b.Speed.P90MS - a.Speed.P90MS,
		TimedOutChange: b.Counts.TimedOut - a.Counts.TimedOut,
		ScoredChange:   b.Counts.Scored - a.Counts.Scored,
		Dimensions: map[string]string{
			"item_count":     fmt.Sprintf("%d", a.Run.ActualSize),
			"window_minutes": fmt.Sprintf("%d", a.Run.WindowMinutes),
			"pool_family":    a.Run.PoolFamily,
		},
		Limitations: []string{
			"This compares two local samples with the same item count, time window and pool family.",
			"It does not show that AI, coaching or the app caused any change.",
			"Contaminated items are excluded from each headline; an unequal scored-item count is shown rather than normalized away.",
		},
	}
	return doc
}

func compareRunViewOf(report assess.Report) assessCompareRunView {
	return assessCompareRunView{
		Run:      runViewOf(report.Run),
		Counts:   countsViewOf(report.Counts),
		Accuracy: accuracyViewOf(report.Accuracy),
		Speed:    speedViewOf(report.Speed),
		Skills:   skillViewsOf(report.Skills),
	}
}

func writePool(w io.Writer, doc poolDoc, dbPath string) {
	fmt.Fprintln(w, "Pass Quest — unseen assessment pool")
	fmt.Fprintf(w, "Qualifying items: %d\n", doc.QualifyingItems)
	fmt.Fprintf(w, "Qualifying units: %d\n", doc.QualifyingUnits)
	fmt.Fprintf(w, "Practiceable units: %d (spent %d)\n", doc.PracticeableUnits, doc.SpentUnits)
	fmt.Fprintf(w, "Pool floor: %d items (%s)\n", doc.FloorItems, map[bool]string{true: "met", false: fmt.Sprintf("%d more item(s) needed", doc.ShortfallItems)}[doc.FloorMet])
	fmt.Fprintf(w, "Default run: --size %d --minutes %d\n", doc.DefaultSize, doc.DefaultMinutes)
	fmt.Fprintf(w, "Imported sources in the pool family: %d\n", doc.SourceCount)
	fmt.Fprintln(w, "\nContent units")
	for _, unit := range doc.ContentUnits {
		state := "unseen"
		if unit.Spent {
			state = "spent by " + strings.Join(unit.SpentBy, ",")
		}
		fmt.Fprintf(w, "  %-28s page %-3d questions %d  %s\n", unit.ContentID, unit.Page, unit.QuestionCount, state)
	}
	fmt.Fprintln(w, "\nLimitations")
	for _, line := range doc.Limitations {
		fmt.Fprintf(w, "  - %s\n", line)
	}
	fmt.Fprintf(w, "\nDatabase: %s\n", dbPath)
}

func writeAssessmentPreamble(w io.Writer, run assess.Run, itemCount int) {
	fmt.Fprintf(w, "Pass Quest — unseen assessment run %d\n", run.ID)
	fmt.Fprintf(w, "Selected: %d item(s) in source order; window %d minute(s); per-item timeout %d ms.\n",
		itemCount, run.WindowMinutes, run.PerItemTimeoutMS)
	fmt.Fprintf(w, "Deadline: %s\n", run.DeadlineAt.UTC().Format(time.RFC3339))
	fmt.Fprintln(w, "No coaching, glossary, reasoning, evidence or correctness feedback is shown during the run.")
	fmt.Fprintln(w)
}

func writeAssessmentUnit(w io.Writer, item assess.Item, content exam.Exam) {
	fmt.Fprintf(w, "Unit %s (page %d)\n", content.ID, item.Page)
	fmt.Fprintf(w, "%s\n\n", content.Passage.Text)
}

func writeAssessmentQuestion(w io.Writer, item assess.Item, question exam.Question, total int, remaining time.Duration) {
	if remaining < 0 {
		remaining = 0
	}
	fmt.Fprintf(w, "Item %d of %d — %s  (%s left)\n", item.Position+1, total, question.ID, remaining.Round(time.Second))
	fmt.Fprintf(w, "%s\n", question.Stem)
	for _, option := range question.Options {
		fmt.Fprintf(w, "  %s. %s\n", option.Label, option.Text)
	}
	fmt.Fprint(w, "Answer (A, B, ...; skip or quit): ")
}

func writeAssessmentSummary(w io.Writer, doc assessmentRunResultDoc, dbPath string) {
	fmt.Fprintf(w, "\nAssessment run %d: %s\n", doc.Run.ID, doc.Run.Status)
	presented := 0
	finalized := 0
	for _, item := range doc.Items {
		if item.State == assess.StatePresented || item.State == assess.StateAnswered || item.State == assess.StateTimedOut {
			presented++
		}
		if item.State == assess.StateAnswered || item.State == assess.StateTimedOut {
			finalized++
		}
	}
	fmt.Fprintf(w, "Selected %d item(s); presented %d; finalized %d.\n", doc.Run.ActualSize, presented, finalized)
	fmt.Fprintf(w, "Report: passquest assess report %d\n", doc.Run.ID)
	if len(doc.Limitations) > 0 {
		fmt.Fprintln(w, "Limitations")
		for _, line := range doc.Limitations {
			fmt.Fprintf(w, "  - %s\n", line)
		}
	}
	fmt.Fprintf(w, "Database: %s\n", dbPath)
}

func writeAssessmentReport(w io.Writer, doc assessReportDoc, dbPath string) {
	fmt.Fprintf(w, "Pass Quest — unseen assessment report (run %d)\n", doc.Run.ID)
	fmt.Fprintf(w, "Status: %s\n", doc.Run.Status)
	if doc.Run.Status == assess.StatusActive {
		fmt.Fprintf(w, "Annotations: withheld until the run is finished (run %d is active)\n", doc.Run.ID)
	}
	fmt.Fprintf(w, "Window: %d minute(s); per-item timeout: %d ms; annotations: %s\n",
		doc.Run.WindowMinutes, doc.Run.PerItemTimeoutMS, doc.Run.Annotations)
	fmt.Fprintf(w, "Selected: %d; presented: %d; finalized: %d; scored: %d; contaminated: %d; pending: %d; unavailable: %d\n",
		doc.Counts.Selected, doc.Counts.Presented, doc.Counts.Finalized, doc.Counts.Scored,
		doc.Counts.Contaminated, doc.Counts.Pending, doc.Counts.Unavailable)
	fmt.Fprintf(w, "Resumes: %d; answered after a resume: %d\n", doc.Run.ResumeCount, doc.Counts.AnsweredAfterResume)
	if doc.Run.AbandonedReason != "" {
		fmt.Fprintf(w, "Abandoned: %s (%s)\n", doc.Run.AbandonedReason, doc.Run.AbandonedAt)
	}
	if doc.Counts.Total > 0 {
		fmt.Fprintf(w, "Correct: %d/%d (%.1f%%)\n", doc.Accuracy.Correct, doc.Accuracy.Total, doc.Accuracy.Ratio*100)
	} else {
		fmt.Fprintf(w, "Correct: 0/0 (no scored items)\n")
	}
	fmt.Fprintf(w, "Accuracy band: %s (descriptive; not a pass threshold)\n", doc.Accuracy.Band)
	fmt.Fprintf(w, "Elapsed: median %d ms; p90 %d ms; timed out %d; skipped %d; basis %d scored item(s)\n",
		doc.Speed.MedianMS, doc.Speed.P90MS, doc.Counts.TimedOut, doc.Counts.Skipped, doc.Speed.Basis)
	fmt.Fprintf(w, "Speed band: %s (descriptive)\n", doc.Speed.Band)

	fmt.Fprintln(w, "\nPer-skill accuracy (scored items)")
	if len(doc.Skills) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, skill := range doc.Skills {
		fmt.Fprintf(w, "  %-24s %d/%d (%.1f%%)\n", skill.SkillID, skill.Correct, skill.Total, skill.Ratio*100)
	}

	fmt.Fprintln(w, "\nContamination (excluded from the headline)")
	fmt.Fprintf(w, "  prior_exposure: %d\n", doc.Contamination.PriorExposure)
	fmt.Fprintf(w, "  repeated_variant: %d\n", doc.Contamination.RepeatedVariant)
	fmt.Fprintf(w, "  hints_during_run: %d\n", doc.Contamination.HintsDuringRun)
	fmt.Fprintf(w, "  resumed_after_window: %d\n", doc.Contamination.ResumedAfterWindow)
	for _, item := range doc.Contamination.ExcludedItems {
		fmt.Fprintf(w, "  #%d %s/%s  %s  %d ms  reasons %s\n",
			item.Position+1, item.ContentID, item.QuestionID, displayItemState(item), item.ElapsedMS,
			strings.Join(item.Reasons, ","))
	}

	fmt.Fprintln(w, "\nItems and withheld annotations")
	for _, item := range doc.Items {
		flags := make([]string, 0, 2)
		if item.TimedOut {
			flags = append(flags, "timed out")
		}
		if item.AfterResume {
			flags = append(flags, "after resume")
		}
		if !item.Scored {
			flags = append(flags, "excluded")
		}
		suffix := ""
		if len(flags) > 0 {
			suffix = " [" + strings.Join(flags, ", ") + "]"
		}
		fmt.Fprintf(w, "  #%d %s/%s  %s  %d ms%s\n",
			item.Position+1, item.ContentID, item.QuestionID, displayItemState(item), item.ElapsedMS, suffix)
		if item.Annotations.Withheld {
			if doc.Run.Status == assess.StatusActive {
				fmt.Fprintln(w, "    Annotations: withheld (run is still active)")
			} else {
				fmt.Fprintln(w, "    Annotations: withheld (item was never finalized)")
			}
			continue
		}
		if !item.Annotations.Available {
			fmt.Fprintln(w, "    Annotations: unavailable (content changed or is no longer practiceable)")
			continue
		}
		fmt.Fprintf(w, "    Evidence: %s\n", item.Annotations.Evidence)
		fmt.Fprintf(w, "    Reasoning: %s\n", item.Annotations.Reasoning)
		labels := make([]string, 0, len(item.Annotations.OptionReasoning))
		for label := range item.Annotations.OptionReasoning {
			labels = append(labels, label)
		}
		sort.Strings(labels)
		for _, label := range labels {
			fmt.Fprintf(w, "    Option %s: %s\n", label, item.Annotations.OptionReasoning[label])
		}
		if len(item.Annotations.Glossary) == 0 {
			fmt.Fprintln(w, "    Glossary: none")
		} else {
			parts := make([]string, 0, len(item.Annotations.Glossary))
			for _, entry := range item.Annotations.Glossary {
				parts = append(parts, entry.Term+" — "+entry.Meaning)
			}
			fmt.Fprintf(w, "    Glossary: %s\n", strings.Join(parts, "; "))
		}
	}

	fmt.Fprintln(w, "\nUnknowns")
	for _, line := range doc.Unknowns {
		fmt.Fprintf(w, "  %s\n", line)
	}
	fmt.Fprintln(w, "\nLimitations")
	for _, line := range doc.Limitations {
		fmt.Fprintf(w, "  - %s\n", line)
	}
	fmt.Fprintf(w, "\nDatabase: %s\n", dbPath)
}

func displayItemState(item assessReportItemView) string {
	if item.TimedOut {
		return "timed_out"
	}
	if item.Status != "" {
		return item.Status
	}
	return item.State
}

func writeComparison(w io.Writer, doc assessComparisonDoc, dbPath string) {
	fmt.Fprintln(w, "Pass Quest — assessment comparison")
	for index, run := range doc.Runs {
		fmt.Fprintf(w, "Run %d: correct %d/%d (%.1f%%), median %d ms, p90 %d ms, timed out %d, scored %d\n",
			index+1, run.Accuracy.Correct, run.Accuracy.Total, run.Accuracy.Ratio*100,
			run.Speed.MedianMS, run.Speed.P90MS, run.Counts.TimedOut, run.Counts.Scored)
	}
	fmt.Fprintf(w, "\nChange: accuracy %+.1f percentage points; median %+d ms; p90 %+d ms; timed out %+d; scored items %+d\n",
		doc.AccuracyPoints, doc.MedianChangeMS, doc.P90ChangeMS, doc.TimedOutChange, doc.ScoredChange)
	if len(doc.Runs) == 2 {
		fmt.Fprintln(w, "\nPer-skill accuracy")
		ids := make(map[string]bool)
		for _, run := range doc.Runs {
			for _, skill := range run.Skills {
				ids[skill.SkillID] = true
			}
		}
		ordered := make([]string, 0, len(ids))
		for id := range ids {
			ordered = append(ordered, id)
		}
		sort.Strings(ordered)
		for _, id := range ordered {
			a := skillRow(doc.Runs[0].Skills, id)
			b := skillRow(doc.Runs[1].Skills, id)
			fmt.Fprintf(w, "  %-24s run 1 %d/%d (%.1f%%)  run 2 %d/%d (%.1f%%)\n",
				id, a.Correct, a.Total, a.Ratio*100, b.Correct, b.Total, b.Ratio*100)
		}
	}
	fmt.Fprintln(w, "\nLimitations")
	for _, line := range doc.Limitations {
		fmt.Fprintf(w, "  - %s\n", line)
	}
	fmt.Fprintf(w, "\nDatabase: %s\n", dbPath)
}

func skillRow(rows []assessSkillView, id string) assessSkillView {
	for _, row := range rows {
		if row.SkillID == id {
			return row
		}
	}
	return assessSkillView{SkillID: id}
}
