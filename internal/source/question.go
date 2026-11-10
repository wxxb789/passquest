package source

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/skill"
)

// ErrNotPracticeable means a page or question has not reached the reviewed
// state needed for deterministic practice.
var ErrNotPracticeable = errors.New("source material is not practiceable")

var questionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type questionFile struct {
	ID              string            `json:"id"`
	Stem            string            `json:"stem"`
	Options         []exam.Option     `json:"options"`
	Answer          string            `json:"answer"`
	Evidence        string            `json:"evidence"`
	Reasoning       string            `json:"reasoning"`
	OptionReasoning map[string]string `json:"option_reasoning"`
	Tactics         []string          `json:"tactics"`
}

// ParseQuestionJSON decodes a locally authored question file. The question is
// always stored unreviewed with an unmapped answer, even when the file proposes
// one: only Review records a user decision.
func ParseQuestionJSON(data []byte, sourceID string, page int, now time.Time, existingID string) (Question, error) {
	var file questionFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil {
		return Question{}, fmt.Errorf("%w: parse question: %v", ErrInvalidInput, err)
	}
	if dec.More() {
		return Question{}, fmt.Errorf("%w: trailing data after question", ErrInvalidInput)
	}
	if existingID != "" && file.ID != existingID {
		return Question{}, fmt.Errorf("%w: question file id %q does not match %q", ErrInvalidInput, file.ID, existingID)
	}
	if strings.TrimSpace(file.ID) == "" {
		return Question{}, fmt.Errorf("%w: question id is required", ErrInvalidInput)
	}

	q := Question{
		SourceID:        sourceID,
		ID:              strings.TrimSpace(file.ID),
		Page:            page,
		Stem:            file.Stem,
		Options:         file.Options,
		Answer:          strings.ToUpper(strings.TrimSpace(file.Answer)),
		Evidence:        file.Evidence,
		Reasoning:       file.Reasoning,
		OptionReasoning: file.OptionReasoning,
		Tactics:         append([]string(nil), file.Tactics...),
		AnswerStatus:    AnswerUnmapped,
		Review:          Unreviewed,
		CreatedAt:       nowUTC(now),
		UpdatedAt:       nowUTC(now),
	}
	if err := q.Validate(); err != nil {
		return Question{}, err
	}
	return q, nil
}

// Validate checks the structural invariants of a question. It does not require
// the answer mapping or the page evidence to be reviewed; that is Review's job.
func (q Question) Validate() error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	if !questionIDPattern.MatchString(q.ID) {
		add("id %q is not 1-64 letters, digits, '.', '_' or '-'", q.ID)
	}
	if q.Page < 1 {
		add("page is %d, want >= 1", q.Page)
	}
	if strings.TrimSpace(q.Stem) == "" {
		add("stem is empty")
	}
	if strings.TrimSpace(q.Evidence) == "" {
		add("evidence is empty")
	}
	if strings.TrimSpace(q.Reasoning) == "" {
		add("reasoning is empty")
	}
	seenTag := make(map[string]bool, len(q.Tactics))
	for _, tag := range q.Tactics {
		if !exam.ValidSkillTag(tag) {
			add("skill tag %q is not a lowercase slug", tag)
		} else if !skill.IsRegistered(tag) {
			add("skill tag %q is not registered; registered: %s", tag, strings.Join(skill.Registered(), ", "))
		}
		if seenTag[tag] {
			add("skill tag %q is repeated", tag)
		}
		seenTag[tag] = true
	}
	for _, problem := range exam.OptionProblems(q.Options, q.OptionReasoning) {
		problems = append(problems, problem)
	}
	if q.Answer != "" {
		if _, ok := (exam.Question{Options: q.Options}).Option(q.Answer); !ok {
			add("answer %q is not one of its options", q.Answer)
		}
	}
	if !q.AnswerStatus.Valid() {
		add("answer_status %q is invalid", q.AnswerStatus)
	}
	if !q.Review.Valid() {
		add("review_status %q is invalid", q.Review)
	}
	if len(problems) > 0 {
		return fmt.Errorf("%w: invalid question %s: %s", ErrInvalidInput, q.ID, strings.Join(problems, "; "))
	}
	return nil
}

func (s ReviewStatus) Valid() bool { return s == Unreviewed || s == Reviewed }

func (s AnswerStatus) Valid() bool { return s == AnswerUnmapped || s == AnswerReviewed }

// Edited applies a replacement question file to an existing question. The
// replacement is unreviewed and its answer is unmapped: the old review was a
// statement about different content.
func (q Question) Edited(replacement Question, at time.Time) Question {
	out := replacement
	out.SourceID = q.SourceID
	out.ID = q.ID
	out.Page = q.Page
	out.CreatedAt = q.CreatedAt
	out.UpdatedAt = nowUTC(at)
	out.AnswerStatus = AnswerUnmapped
	out.Review = Unreviewed
	out.ReviewedBy = ""
	out.ReviewedAt = time.Time{}
	return out
}

// ReviewAnswer records a user decision about a question's answer. The page must be
// reviewed first and the evidence must still be a verbatim substring of its
// current text; otherwise the mapping would be attached to different material.
func (q Question) ReviewAnswer(page Page, answer, reviewer string, at time.Time) (Question, error) {
	reviewer = strings.TrimSpace(reviewer)
	if reviewer == "" {
		return Question{}, fmt.Errorf("%w: reviewer is required", ErrInvalidInput)
	}
	if page.Number != q.Page {
		return Question{}, fmt.Errorf("%w: question page %d does not match page %d", ErrInvalidInput, q.Page, page.Number)
	}
	if page.Review != Reviewed {
		return Question{}, fmt.Errorf("%w: review page %d before reviewing question %s", ErrNotPracticeable, page.Number, q.ID)
	}
	answer = strings.ToUpper(strings.TrimSpace(answer))
	if answer == "" {
		return Question{}, fmt.Errorf("%w: answer label is required", ErrInvalidInput)
	}
	if !evidenceInPage(page, q) {
		return Question{}, fmt.Errorf("%w: question %s evidence is not present in the current page text; correct the page or the question first", ErrNotPracticeable, q.ID)
	}

	out := q
	out.Answer = answer
	out.AnswerStatus = AnswerReviewed
	out.Review = Reviewed
	out.ReviewedBy = reviewer
	out.ReviewedAt = nowUTC(at)
	out.UpdatedAt = nowUTC(at)
	if err := out.Validate(); err != nil {
		return Question{}, err
	}
	return out, nil
}

func (q Question) Flags(page Page) []Flag {
	var flags []Flag
	if page.Review != Reviewed {
		flags = appendFlag(flags, FlagPageUnreviewed)
	}
	if q.AnswerStatus != AnswerReviewed || q.Answer == "" {
		flags = appendFlag(flags, FlagAnswerUnmapped)
	}
	if !evidenceInPage(page, q) {
		flags = appendFlag(flags, FlagEvidenceMissing)
	}
	return flags
}

func PracticeReady(page Page, q Question) bool {
	return page.Review == Reviewed &&
		q.Review == Reviewed &&
		q.AnswerStatus == AnswerReviewed &&
		q.Answer != "" &&
		evidenceInPage(page, q)
}

func evidenceInPage(page Page, q Question) bool {
	return strings.Contains(page.Text, q.Evidence)
}

// BuildPracticeExam converts reviewed questions on one reviewed page into the
// same Exam shape goal 0001 scores. Stale reviewed questions are excluded and
// reported by id. The error is ErrNotPracticeable when nothing can be served.
func BuildPracticeExam(src Source, page Page, questions []Question) (exam.Exam, []string, error) {
	if page.Review != Reviewed {
		return exam.Exam{}, nil, fmt.Errorf("%w: page %d is %s; review it first", ErrNotPracticeable, page.Number, page.Review)
	}
	var ready []exam.Question
	var readyMeta []Question
	var stale []string
	for _, q := range questions {
		if q.Page != page.Number || q.Review != Reviewed {
			continue
		}
		if !PracticeReady(page, q) {
			stale = append(stale, q.ID)
			continue
		}
		ready = append(ready, exam.Question{
			ID:              q.ID,
			Stem:            q.Stem,
			Options:         append([]exam.Option(nil), q.Options...),
			Answer:          q.Answer,
			Evidence:        q.Evidence,
			Reasoning:       q.Reasoning,
			OptionReasoning: copyReasoning(q.OptionReasoning),
			Tactics:         append([]string(nil), q.Tactics...),
		})
		readyMeta = append(readyMeta, q)
	}
	sort.Strings(stale)
	if len(ready) == 0 {
		if len(stale) > 0 {
			return exam.Exam{}, stale, fmt.Errorf("%w: reviewed questions on page %d no longer match the page text (%s)", ErrNotPracticeable, page.Number, strings.Join(stale, ", "))
		}
		return exam.Exam{}, nil, fmt.Errorf("%w: page %d has no reviewed questions", ErrNotPracticeable, page.Number)
	}

	content := exam.Exam{
		SchemaVersion: exam.SchemaVersion,
		ID:            ContentID(src.ID, page.Number),
		Version:       materialVersion(page, ready),
		Title:         fmt.Sprintf("%s — page %d", src.Title, page.Number),
		Notice:        practiceNotice(src, page, readyMeta),
		Provenance: exam.Provenance{
			Author:       src.Title,
			Source:       fmt.Sprintf("local %s import: %s (sha256 %s)", src.Import, src.Path, src.SHA256),
			Rights:       src.Rights,
			ExamRelation: src.ExamRelation,
		},
		Passage: exam.Passage{
			Title: fmt.Sprintf("Page %d", page.Number),
			Text:  page.Text,
		},
		Questions: ready,
	}
	if err := content.Validate(); err != nil {
		return exam.Exam{}, stale, fmt.Errorf("%w: %v", ErrNotPracticeable, err)
	}
	return content, stale, nil
}

func copyReasoning(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func practiceNotice(src Source, page Page, questions []Question) string {
	sort.Slice(questions, func(i, j int) bool { return questions[i].ID < questions[j].ID })
	reviewed := make([]string, 0, len(questions))
	for _, q := range questions {
		reviewed = append(reviewed, fmt.Sprintf("%s by %s", q.ID, reviewAttribution(q.ReviewedBy, q.ReviewedAt)))
	}
	notice := fmt.Sprintf(
		"Imported private source page %d (%s, page reviewed by %s). Answer mapping reviewed for question(s) %s. Not a live exam, not an exam blueprint and not an external answer key.",
		page.Number, page.Origin, reviewAttribution(page.ReviewedBy, page.ReviewedAt), strings.Join(reviewed, "; "),
	)
	if src.Title != "" {
		notice = fmt.Sprintf("%s %s", src.Title, notice)
	}
	return notice
}

func reviewAttribution(reviewer string, at time.Time) string {
	if at.IsZero() {
		return reviewer
	}
	return fmt.Sprintf("%s at %s", reviewer, FormatTime(at))
}

func materialVersion(page Page, questions []exam.Question) string {
	h := sha256.New()
	writeHashField(h, "passquest-practice-v1")
	writeHashField(h, fmt.Sprintf("%d", page.Number))
	writeHashField(h, page.Text)
	for _, q := range questions {
		writeHashField(h, q.ID)
		writeHashField(h, q.Stem)
		writeHashField(h, q.Answer)
		writeHashField(h, q.Evidence)
		writeHashField(h, q.Reasoning)
		for _, opt := range q.Options {
			writeHashField(h, opt.Label)
			writeHashField(h, opt.Text)
		}
		labels := make([]string, 0, len(q.OptionReasoning))
		for label := range q.OptionReasoning {
			labels = append(labels, label)
		}
		sort.Strings(labels)
		for _, label := range labels {
			writeHashField(h, label)
			writeHashField(h, q.OptionReasoning[label])
		}
		if len(q.Tactics) > 0 {
			writeHashField(h, strings.Join(q.Tactics, ","))
		}
	}
	return "1-" + hex.EncodeToString(h.Sum(nil))[:8]
}

func SummaryOf(src Source, questions []Question) Summary {
	summary := Summary{
		ID:            src.ID,
		Title:         src.Title,
		Import:        src.Import,
		Path:          src.Path,
		SHA256:        src.SHA256,
		CreatedAt:     src.CreatedAt,
		PageCount:     len(src.Pages),
		QuestionCount: len(questions),
	}
	for _, page := range src.Pages {
		if page.Review == Reviewed {
			summary.ReviewedPages++
		}
	}
	for _, q := range questions {
		if q.Review == Reviewed && q.AnswerStatus == AnswerReviewed {
			summary.ReviewedQuestions++
		}
	}
	return summary
}
