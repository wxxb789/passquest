package assessment

import (
	"context"
	"errors"
	"maps"

	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
)

// Query selects what one Report call reads.
type Query uint8

const (
	// QueryPool reads the unseen-assessment pool.
	QueryPool Query = iota + 1
	// QueryReport reads one run's report.
	QueryReport
	// QueryCompare reads both runs and checks they are like for like;
	// incomparable runs are refused with incomparable_runs.
	QueryCompare
	// QueryRuns lists every run, oldest first.
	QueryRuns
)

// ReportQuery selects one Report read.
type ReportQuery struct {
	Query  Query
	RunID  int64 // QueryReport
	First  int64 // QueryCompare
	Second int64 // QueryCompare
}

// Reported carries what the requested read produced; Query decides which
// fields are set.
type Reported struct {
	Pool       assess.Pool             // QueryPool
	Identities []assess.SourceIdentity // QueryPool
	Report     assess.Report           // QueryReport
	// Units are the units the report was built from; the window maps each
	// shown item's question from them.
	Units  []assess.Unit // QueryReport
	First  assess.Report // QueryCompare
	Second assess.Report // QueryCompare
	Runs   []assess.Run  // QueryRuns
}

// Report reads the unseen pool, one run's report, a like-for-like comparison
// or the run list.
func Report(ctx context.Context, s *study.Service, q ReportQuery) (Reported, error) {
	_ = ctx
	reported, err := report(s, q)
	if err != nil {
		return Reported{}, classify(err)
	}
	return reported, nil
}

func report(s *study.Service, q ReportQuery) (Reported, error) {
	switch q.Query {
	case QueryPool:
		pool, identities, err := poolOf(s)
		if err != nil {
			return Reported{}, err
		}
		return Reported{Pool: pool, Identities: identities}, nil
	case QueryReport:
		material, err := materialOf(s)
		if err != nil {
			return Reported{}, err
		}
		report, err := reportFrom(s, q.RunID, material)
		if err != nil {
			return Reported{}, err
		}
		return Reported{Report: report, Units: material.units}, nil
	case QueryCompare:
		material, err := materialOf(s)
		if err != nil {
			return Reported{}, err
		}
		a, err := reportFrom(s, q.First, material)
		if err != nil {
			return Reported{}, err
		}
		b, err := reportFrom(s, q.Second, material)
		if err != nil {
			return Reported{}, err
		}
		if err := assess.Compare(a, b); err != nil {
			var refusal *assess.CompareRefusal
			if errors.As(err, &refusal) {
				return Reported{}, kernel.Refuse("incomparable_runs",
					"cannot compare runs %d and %d: %s", a.Run.ID, b.Run.ID, refusal.Error())
			}
			return Reported{}, err
		}
		return Reported{First: a, Second: b}, nil
	case QueryRuns:
		runs, err := kernel.Store(s).AssessmentRuns()
		if err != nil {
			return Reported{}, err
		}
		return Reported{Runs: runs}, nil
	}
	return Reported{}, kernel.Refuse("invalid_input", "unknown assessment query %d", q.Query)
}

// Question loads the exam and the question one item was presented from at its
// recorded version. ok is false when the content no longer resolves at that
// version: a missing source, page or question, or a version mismatch.
func Question(s *study.Service, item assess.Item) (exam.Exam, exam.Question, bool) {
	st := kernel.Store(s)
	src, err := st.SourceMeta(item.SourceID)
	if err != nil {
		return exam.Exam{}, exam.Question{}, false
	}
	page, err := st.Page(item.SourceID, item.Page)
	if err != nil {
		return exam.Exam{}, exam.Question{}, false
	}
	questions, err := st.Questions(item.SourceID)
	if err != nil {
		return exam.Exam{}, exam.Question{}, false
	}
	content, _, err := source.BuildPracticeExam(src, page, questions)
	if err != nil || content.Version != item.ContentVersion {
		return exam.Exam{}, exam.Question{}, false
	}
	question, _, found := content.Question(item.QuestionID)
	if !found {
		return exam.Exam{}, exam.Question{}, false
	}
	return content, question, true
}

type material struct {
	spends []assess.Spend
	units  []assess.Unit
}

func materialOf(s *study.Service) (material, error) {
	spends, err := kernel.Store(s).Spends()
	if err != nil {
		return material{}, err
	}
	units, _, err := unitsOf(s)
	if err != nil {
		return material{}, err
	}
	return material{spends: spends, units: units}, nil
}

func reportFrom(s *study.Service, runID int64, material material) (assess.Report, error) {
	st := kernel.Store(s)
	run, items, events, err := st.AssessmentRun(runID)
	if err != nil {
		return assess.Report{}, err
	}
	attempts, err := st.AssessmentAttempts(runID)
	if err != nil {
		return assess.Report{}, err
	}
	byContent := make(map[string]assess.Unit, len(material.units))
	for _, unit := range material.units {
		byContent[unit.ContentID] = unit
	}
	annotations := make(map[attempt.QuestionKey]assess.Annotations, len(items))
	for _, item := range items {
		unit, ok := byContent[item.ContentID]
		if !ok || unit.Version != item.ContentVersion {
			continue
		}
		for _, question := range unit.Questions {
			if question.ID != item.QuestionID {
				continue
			}
			annotations[attempt.QuestionKey{ContentID: item.ContentID, QuestionID: item.QuestionID}] = assess.Annotations{
				Evidence:               question.Evidence,
				Reasoning:              question.Reasoning,
				OptionReasoning:        maps.Clone(question.OptionReasoning),
				OptionReasoningMissing: exam.MissingOptionReasoning(question.Options, question.OptionReasoning),
				Glossary:               append([]exam.GlossaryEntry(nil), question.Glossary...),
			}
			break
		}
	}
	return assess.BuildReport(run, items, attempts, events, material.spends, annotations), nil
}
