package practice

import (
	"fmt"
	"slices"
	"strings"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/skill"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
	"github.com/wxxb789/passquest/internal/tactic"
)

// practiceRules is the practice area's sentinel classification table: the
// learner-fixable domain errors the CLI prints as usage errors.
var practiceRules = []kernel.Rule{
	{Is: source.ErrNotPracticeable, Code: "not_practiceable"},
	{Is: source.ErrInvalidInput, Code: "invalid_input"},
	{Is: source.ErrToolMissing, Code: "invalid_input"},
	{Is: store.ErrNotFound, Code: "invalid_input"},
}

// selection is one selected practice item before it is displayed or held.
type selection struct {
	Content  exam.Exam
	Question exam.Question
	Number   int
	Page     *source.Page
	Review   *source.Question
	Tactic   *TacticRun
	Skill    *SkillRun
}

// item is the display value of a selection, bound to the presentation it was
// held as.
func (sel selection) item(p *store.PracticePresentation, r ref) Item {
	return Item{
		Content:      sel.Content,
		Question:     sel.Question,
		Number:       sel.Number,
		Page:         sel.Page,
		Review:       sel.Review,
		Tactic:       sel.Tactic,
		Skill:        sel.Skill,
		Presentation: p,
		ref:          r,
	}
}

// selection is the selection of an item that was returned by an entry point.
func (i Item) selection() selection {
	return selection{
		Content:  i.Content,
		Question: i.Question,
		Number:   i.Number,
		Page:     i.Page,
		Review:   i.Review,
		Tactic:   i.Tactic,
		Skill:    i.Skill,
	}
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

// choose resolves a request to the selection its rule picks. The refusals are
// the protocol's: the same codes and messages the CLI prints today.
func choose(st *store.Store, r Request) (selection, error) {
	switch r.Kind {
	case KindSample:
		records, err := st.List()
		if err != nil {
			return selection{}, err
		}
		return selectSamplePractice(r.QuestionID, records)
	case KindSource:
		if r.RequireSourceID && strings.TrimSpace(r.SourceID) == "" {
			return selection{}, kernel.Refuse("invalid_input", "source practice needs a source id")
		}
		records, err := st.List()
		if err != nil {
			return selection{}, err
		}
		return selectSourcePractice(st, r.SourceID, r.Page, r.QuestionID, records)
	case KindTactic:
		lesson, ok := tactic.Get(r.TacticID)
		if !ok {
			return selection{}, kernel.Refuse("unknown_tactic", "tactic %q is not registered; available: %s",
				r.TacticID, strings.Join(tactic.Registered(), ", "))
		}
		return selectTacticPractice(st, lesson, r.Stage, r.SourceID, r.QuestionID)
	case KindSkill:
		lesson, ok := skill.Get(r.SkillID)
		if !ok {
			return selection{}, kernel.Refuse("unknown_skill", "skill %q is not registered; available: %s",
				r.SkillID, strings.Join(skill.Registered(), ", "))
		}
		return selectSkillPractice(st, lesson, r.SourceID, r.QuestionID)
	case KindNext:
		return selectNextPractice(st, r)
	default:
		return selection{}, kernel.Refuse("invalid_input", "unknown practice kind %q; use next, skill, tactic, sample or source", r.Kind)
	}
}

// selectNextPractice selects the practice a resolved recommendation target
// names. The recommendation module resolves the target
// (recommend.PracticeRequest), so the request names it: a guided tactic, or a
// skill with the material the target pinned.
func selectNextPractice(st *store.Store, r Request) (selection, error) {
	switch {
	case r.TacticID != "":
		lesson, ok := tactic.Get(r.TacticID)
		if !ok {
			return selection{}, kernel.Refuse("unknown_tactic", "tactic %q is not registered", r.TacticID)
		}
		return selectTacticPractice(st, lesson, r.Stage, "", "")
	case r.SkillID != "":
		lesson, ok := skill.Get(r.SkillID)
		if !ok {
			return selection{}, kernel.Refuse("unknown_skill", "skill %q is not registered", r.SkillID)
		}
		return selectSkillPractice(st, lesson, r.SourceID, r.QuestionID)
	default:
		return selection{}, kernel.Refuse("no_recommendation", "no practiceable skill material is available")
	}
}

// selectSamplePractice picks a question of the embedded sample: the requested
// one, else the first unattempted one, else the first.
func selectSamplePractice(questionID string, records []attempt.Attempt) (selection, error) {
	sample, err := exam.Sample()
	if err != nil {
		return selection{}, err
	}
	sel := selection{Content: sample}
	perQuestion := make(map[attempt.QuestionKey]int, len(records))
	for _, rec := range records {
		perQuestion[rec.Key()]++
	}
	if questionID != "" {
		question, number, ok := sample.Question(questionID)
		if !ok {
			return selection{}, kernel.Refuse("unknown_question", "question %q is not in content %s", questionID, sample.ID)
		}
		sel.Question, sel.Number = question, number
	} else if index := firstUnattempted(sample.ID, sample.Questions, perQuestion); index >= 0 {
		sel.Question, sel.Number = sample.Questions[index], index
	} else {
		sel.Question, sel.Number = sample.Questions[0], 0
	}
	return sel, nil
}

// sourceCandidates scans a reviewed source for every question tagged with the
// skill id. A source without one is a refusal, never a silent fallback to
// embedded material.
func sourceCandidates(st *store.Store, sourceID, skillID string) ([]practiceCandidate, error) {
	src, questions, err := study.LoadSourceBundle(st, sourceID)
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
			meta, ok := study.FindQuestion(questions, question.ID)
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
		return nil, kernel.Refuse("not_practiceable",
			"source %s has no reviewed question tagged %q; add a tagged question with `question add`, then review the page and its answer (see `passquest questions %s`)",
			sourceID, skillID, sourceID)
	}
	return out, nil
}

func selectSourcePractice(st *store.Store, sourceID string, pageNumber int, questionID string, records []attempt.Attempt) (selection, error) {
	src, questions, err := study.LoadSourceBundle(st, sourceID)
	if err != nil {
		return selection{}, err
	}
	if len(src.Pages) == 0 {
		return selection{}, fmt.Errorf("%w: source %s has no pages", source.ErrNotPracticeable, sourceID)
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
			return selection{}, fmt.Errorf("%w: page %d is not in source %s", source.ErrInvalidInput, pageNumber, sourceID)
		}
		if questionID != "" {
			q, ok := study.FindQuestion(questions, questionID)
			if !ok {
				return selection{}, fmt.Errorf("%w: question %q is not in source %s", source.ErrInvalidInput, questionID, sourceID)
			}
			if q.Page != pageNumber {
				return selection{}, fmt.Errorf("%w: question %q belongs to page %d, not %d", source.ErrInvalidInput, questionID, q.Page, pageNumber)
			}
		}
	} else if questionID != "" {
		q, ok := study.FindQuestion(questions, questionID)
		if !ok {
			return selection{}, fmt.Errorf("%w: question %q is not in source %s", source.ErrInvalidInput, questionID, sourceID)
		}
		for i, page := range src.Pages {
			if page.Number == q.Page {
				pageIndex = i
				break
			}
		}
		if pageIndex < 0 {
			return selection{}, fmt.Errorf("%w: question %q points at missing page %d", source.ErrInvalidInput, questionID, q.Page)
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
			return selection{}, fmt.Errorf("%w: source %s has no reviewed page and question to practice; use passquest questions %s", source.ErrNotPracticeable, sourceID, sourceID)
		}
	}

	page := src.Pages[pageIndex]
	content, _, err := source.BuildPracticeExam(src, page, questions)
	if err != nil {
		return selection{}, err
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
			q, _ := study.FindQuestion(questions, questionID)
			return selection{}, fmt.Errorf(
				"%w: question %s is not practiceable (review %s, answer %s, flags %s)",
				source.ErrNotPracticeable, questionID, q.Review, q.AnswerStatus, study.DisplayFlags(q.Flags(page)))
		}
	} else if i := firstUnattempted(content.ID, content.Questions, attempts); i >= 0 {
		index = i
	} else {
		index = 0
	}

	reviewQuestion, ok := study.FindQuestion(questions, content.Questions[index].ID)
	if !ok {
		return selection{}, fmt.Errorf("%w: selected question is missing from the source", source.ErrNotPracticeable)
	}
	return selection{
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
