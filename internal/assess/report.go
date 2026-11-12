package assess

import (
	"fmt"
	"sort"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
)

const (
	ReasonPriorExposure      = "prior_exposure"
	ReasonRepeatedVariant    = "repeated_variant"
	ReasonHintsDuringRun     = "hints_during_run"
	ReasonResumedAfterWindow = "resumed_after_window"
)

// BuildReport recomputes the report from persisted rows and the recorded
// spend union. annotations is keyed by the content-scoped question; a missing
// entry prints as an unavailable annotation rather than a fabricated one.
func BuildReport(run Run, items []Item, attempts []attempt.Attempt, events []Event, spends []Spend, annotations map[attempt.QuestionKey]Annotations) Report {
	byAttempt := make(map[int64]attempt.Attempt, len(attempts))
	for _, record := range attempts {
		byAttempt[record.ID] = record
	}
	resumeTimes := make([]time.Time, 0, run.ResumeCount)
	resumedAfterWindow := 0
	for _, event := range events {
		if event.Kind != EventResumed {
			continue
		}
		resumeTimes = append(resumeTimes, event.At)
		if !event.At.Before(run.DeadlineAt) {
			resumedAfterWindow++
		}
	}
	sort.Slice(resumeTimes, func(i, j int) bool { return resumeTimes[i].Before(resumeTimes[j]) })
	firstResume := time.Time{}
	if len(resumeTimes) > 0 {
		firstResume = resumeTimes[0]
	}

	spendsByContent := make(map[string][]Spend, len(spends))
	spendsByQuestion := make(map[string][]Spend, len(spends))
	for _, spend := range spends {
		spendsByContent[spend.ContentID] = append(spendsByContent[spend.ContentID], spend)
		spendsByQuestion[spend.QuestionID] = append(spendsByQuestion[spend.QuestionID], spend)
	}

	report := Report{
		Run:         run,
		Items:       make([]ItemReport, 0, len(items)),
		Unknowns:    append([]string(nil), Unknowns...),
		Limitations: append([]string(nil), Limitations...),
	}
	counts := Counts{Selected: len(items)}
	var elapsed []int64
	skillCounts := make(map[string]*SkillAccuracy)

	for _, item := range items {
		entry := ItemReport{
			Position:    item.Position,
			ContentID:   item.ContentID,
			QuestionID:  item.QuestionID,
			SourceID:    item.SourceID,
			Page:        item.Page,
			State:       item.State,
			Annotations: annotations[attempt.QuestionKey{ContentID: item.ContentID, QuestionID: item.QuestionID}],
		}
		if !item.PresentedAt.IsZero() {
			counts.Presented++
		}
		if item.State == StateUnavailable {
			counts.Unavailable++
			report.Items = append(report.Items, entry)
			continue
		}
		if !item.Finalized() {
			report.Items = append(report.Items, entry)
			continue
		}
		record, found := byAttempt[item.AttemptID]
		if !found || item.AttemptID == 0 {
			entry.State = StateUnavailable
			counts.Unavailable++
			report.Items = append(report.Items, entry)
			continue
		}
		counts.Finalized++
		entry.Status = record.Status
		entry.TimedOut = item.State == StateTimedOut
		entry.ElapsedMS = record.ElapsedMS
		if entry.TimedOut {
			counts.TimedOut++
		}
		if record.Status == attempt.StatusSkipped {
			counts.Skipped++
		}
		entry.AfterResume = !firstResume.IsZero() && !item.AnsweredAt.Before(firstResume)
		if entry.AfterResume {
			counts.AnsweredAfterResume++
		}
		entry.Reasons = contaminationReasons(run, item, spendsByContent[item.ContentID], spendsByQuestion[item.QuestionID], resumedAfterWindow)
		if len(entry.Reasons) > 0 {
			counts.Contaminated++
			addReasons(&report.Contamination, entry.Reasons)
			report.Contamination.Excluded = append(report.Contamination.Excluded, entry)
			report.Items = append(report.Items, entry)
			continue
		}
		entry.Scored = true
		counts.Scored++
		elapsed = append(elapsed, record.ElapsedMS)
		if record.Status == attempt.StatusCorrect {
			counts.Correct++
		}
		skills := record.Skills
		if len(skills) == 0 {
			skills = item.Skills
		}
		if len(skills) == 0 {
			skills = []string{"unclassified"}
		}
		seen := make(map[string]bool, len(skills))
		for _, skillID := range skills {
			if seen[skillID] {
				continue
			}
			seen[skillID] = true
			stat := skillCounts[skillID]
			if stat == nil {
				stat = &SkillAccuracy{SkillID: skillID}
				skillCounts[skillID] = stat
			}
			stat.Total++
			if record.Status == attempt.StatusCorrect {
				stat.Correct++
			}
		}
		report.Items = append(report.Items, entry)
	}

	counts.Total = counts.Scored
	counts.Pending = counts.Selected - counts.Finalized - counts.Unavailable
	report.Counts = counts
	report.Accuracy = Accuracy{Correct: counts.Correct, Total: counts.Total}
	if counts.Total > 0 {
		report.Accuracy.Ratio = float64(counts.Correct) / float64(counts.Total)
		report.Accuracy.Band = accuracyBand(report.Accuracy.Ratio)
	} else {
		report.Accuracy.Band = "no_scored_items"
	}
	report.Speed = speedSummary(elapsed, run.PerItemTimeoutMS)
	report.Contamination.ResumedAfterWindow = resumedAfterWindow

	report.Skills = make([]SkillAccuracy, 0, len(skillCounts))
	for _, stat := range skillCounts {
		if stat.Total > 0 {
			stat.Ratio = float64(stat.Correct) / float64(stat.Total)
		}
		report.Skills = append(report.Skills, *stat)
	}
	sort.Slice(report.Skills, func(i, j int) bool {
		a, b := report.Skills[i], report.Skills[j]
		if (a.SkillID == "unclassified") != (b.SkillID == "unclassified") {
			return b.SkillID == "unclassified"
		}
		return a.SkillID < b.SkillID
	})
	return report
}

func contaminationReasons(run Run, item Item, byContent, byQuestion []Spend, resumedAfterWindow int) []string {
	var prior, repeated, hints bool
	for _, spend := range byContent {
		if spend.RunID == run.ID {
			continue
		}
		if !item.PresentedAt.IsZero() && spend.At.Before(item.PresentedAt) {
			prior = true
		}
		if spend.Surface == SurfaceAssess && spend.At.Before(run.StartedAt) {
			repeated = true
		}
		if spend.Surface != SurfaceAssess &&
			!spend.At.Before(run.StartedAt) &&
			!spend.At.Before(item.PresentedAt) &&
			spend.At.Before(item.AnsweredAt) {
			hints = true
		}
	}
	for _, spend := range byQuestion {
		if spend.RunID == run.ID {
			continue
		}
		if spend.ContentID != item.ContentID && !item.PresentedAt.IsZero() && spend.At.Before(item.PresentedAt) {
			repeated = true
		}
	}
	var reasons []string
	if prior {
		reasons = append(reasons, ReasonPriorExposure)
	}
	if repeated {
		reasons = append(reasons, ReasonRepeatedVariant)
	}
	if hints {
		reasons = append(reasons, ReasonHintsDuringRun)
	}
	if resumedAfterWindow > 0 && !item.AnsweredAt.Before(run.DeadlineAt) {
		reasons = append(reasons, ReasonResumedAfterWindow)
	}
	return reasons
}

func addReasons(totals *Contamination, reasons []string) {
	for _, reason := range reasons {
		switch reason {
		case ReasonPriorExposure:
			totals.PriorExposure++
		case ReasonRepeatedVariant:
			totals.RepeatedVariant++
		case ReasonHintsDuringRun:
			totals.HintsDuringRun++
		}
	}
}

func accuracyBand(ratio float64) string {
	switch {
	case ratio < 0.50:
		return "low"
	case ratio < 0.70:
		return "mixed"
	case ratio < 0.85:
		return "developing"
	default:
		return "strong"
	}
}

func speedSummary(elapsed []int64, perItemTimeoutMS int64) Speed {
	out := Speed{PerItemTimeoutMS: perItemTimeoutMS, Basis: len(elapsed)}
	if len(elapsed) == 0 {
		return out
	}
	values := append([]int64(nil), elapsed...)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	out.MedianMS = Median(values)
	out.P90MS = P90(values)
	if perItemTimeoutMS > 0 {
		switch {
		case out.MedianMS*100 < perItemTimeoutMS*50:
			out.Band = "comfortable"
		case out.MedianMS*100 < perItemTimeoutMS*75:
			out.Band = "steady"
		default:
			out.Band = "pressed"
		}
	}
	return out
}

// Median returns the middle value, or the integer average of the two middle
// values for an even count. values must be sorted.
func Median(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle]
	}
	return (values[middle-1] + values[middle]) / 2
}

// P90 returns the nearest-rank 90th percentile. values must be sorted.
func P90(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	index := (9*len(values)+9)/10 - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

// CompareRefusal names the first dimension that makes two runs unlike.
type CompareRefusal struct {
	Dimension string
	A         string
	B         string
}

func (e *CompareRefusal) Error() string {
	return fmt.Sprintf("%s differs (%s vs %s)", e.Dimension, e.A, e.B)
}

// Compare accepts two reports only when both ran to completion with the same
// actual item count, the same time window and the same pool family. The order
// of the checks is part of the refusal message.
func Compare(a, b Report) error {
	if a.Run.Status != StatusCompleted || b.Run.Status != StatusCompleted {
		return &CompareRefusal{Dimension: "status", A: a.Run.Status, B: b.Run.Status}
	}
	if a.Run.ActualSize != b.Run.ActualSize {
		return &CompareRefusal{
			Dimension: "item_count",
			A:         fmt.Sprintf("%d", a.Run.ActualSize),
			B:         fmt.Sprintf("%d", b.Run.ActualSize),
		}
	}
	if a.Run.WindowMinutes != b.Run.WindowMinutes {
		return &CompareRefusal{
			Dimension: "window_minutes",
			A:         fmt.Sprintf("%d", a.Run.WindowMinutes),
			B:         fmt.Sprintf("%d", b.Run.WindowMinutes),
		}
	}
	if a.Run.PoolFamily != b.Run.PoolFamily {
		return &CompareRefusal{Dimension: "pool_family", A: shortHash(a.Run.PoolFamily), B: shortHash(b.Run.PoolFamily)}
	}
	return nil
}

func shortHash(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}
