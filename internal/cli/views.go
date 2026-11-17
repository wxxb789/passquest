package cli

import (
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/source"
)

func contentViewOf(content exam.Exam) contentView {
	return contentView{
		ID:      content.ID,
		Version: content.Version,
		Title:   content.Title,
		Notice:  content.Notice,
		Provenance: provenanceView{
			Author:       content.Provenance.Author,
			Source:       content.Provenance.Source,
			Rights:       content.Provenance.Rights,
			ExamRelation: content.Provenance.ExamRelation,
		},
	}
}

func questionViewOf(selection practiceSelection) questionView {
	view := questionView{
		ID:     selection.Question.ID,
		Number: selection.Number + 1,
		Total:  len(selection.Content.Questions),
		Passage: passageView{
			Title: selection.Content.Passage.Title,
			Text:  selection.Content.Passage.Text,
		},
		Stem:     selection.Question.Stem,
		Options:  optionViews(selection.Question.Options),
		Glossary: glossaryViews(selection.Question.Glossary),
	}
	if selection.Page != nil && selection.Review != nil {
		view.Review = &reviewView{
			PageStatus:         string(selection.Page.Review),
			PageReviewedBy:     selection.Page.ReviewedBy,
			PageReviewedAt:     source.FormatTime(selection.Page.ReviewedAt),
			QuestionStatus:     string(selection.Review.Review),
			QuestionReviewedBy: selection.Review.ReviewedBy,
			QuestionReviewedAt: source.FormatTime(selection.Review.ReviewedAt),
		}
	}
	return view
}

func optionViews(options []exam.Option) []optionView {
	views := make([]optionView, 0, len(options))
	for _, opt := range options {
		views = append(views, optionView{Label: opt.Label, Text: opt.Text})
	}
	return views
}

func glossaryViews(entries []exam.GlossaryEntry) []glossaryEntryView {
	views := make([]glossaryEntryView, 0, len(entries))
	for _, entry := range entries {
		views = append(views, glossaryEntryView{Term: entry.Term, Meaning: entry.Meaning})
	}
	return views
}

func attemptViewOf(record attempt.Attempt) attemptView {
	var selectedLabel, selectedText *string
	if record.SelectedLabel != "" {
		label := record.SelectedLabel
		selectedLabel = &label
	}
	if record.SelectedText != "" {
		text := record.SelectedText
		selectedText = &text
	}
	return attemptView{
		ID:              record.ID,
		SessionID:       record.SessionID,
		QuestionID:      record.QuestionID,
		AttemptNumber:   record.AttemptNumber,
		PriorAttempts:   record.PriorAttempts,
		Repeat:          record.Repeat(),
		Assistance:      string(record.Assistance),
		TacticID:        record.TacticID,
		Skills:          append([]string(nil), record.Skills...),
		Status:          string(record.Status),
		SelectedLabel:   selectedLabel,
		SelectedText:    selectedText,
		CorrectLabel:    record.CorrectLabel,
		CorrectText:     record.CorrectText,
		StartedAt:       record.StartedAt.UTC().Format(time.RFC3339Nano),
		AnsweredAt:      record.AnsweredAt.UTC().Format(time.RFC3339Nano),
		ElapsedMS:       record.ElapsedMS,
		ContentID:       record.ContentID,
		ContentVersion:  record.ContentVersion,
		AssessmentRunID: record.AssessmentRunID,
	}
}

func feedbackViewOf(feedback attempt.Feedback) feedbackView {
	return feedbackView{
		Verdict:           string(feedback.Verdict),
		CorrectLabel:      feedback.CorrectLabel,
		CorrectText:       feedback.CorrectText,
		Evidence:          feedback.Evidence,
		Reasoning:         feedback.Reasoning,
		SelectedReasoning: feedback.SelectedReasoning,
		Exposure:          feedback.Exposure,
		Assistance:        string(feedback.Assistance),
	}
}

// JSON documents. Field names are protocol: tests and golden files pin them.

type contentView struct {
	ID         string         `json:"id"`
	Version    string         `json:"version"`
	Title      string         `json:"title"`
	Notice     string         `json:"notice"`
	Provenance provenanceView `json:"provenance"`
}

type provenanceView struct {
	Author       string `json:"author"`
	Source       string `json:"source"`
	Rights       string `json:"rights"`
	ExamRelation string `json:"exam_relation"`
}

type passageView struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type optionView struct {
	Label string `json:"label"`
	Text  string `json:"text"`
}

type glossaryEntryView struct {
	Term    string `json:"term"`
	Meaning string `json:"meaning"`
}

type questionView struct {
	ID       string              `json:"id"`
	Number   int                 `json:"number"`
	Total    int                 `json:"total"`
	Passage  passageView         `json:"passage"`
	Stem     string              `json:"stem"`
	Options  []optionView        `json:"options"`
	Glossary []glossaryEntryView `json:"glossary,omitempty"`
	Review   *reviewView         `json:"review,omitempty"`
}

type reviewView struct {
	PageStatus         string `json:"page_status"`
	PageReviewedBy     string `json:"page_reviewed_by"`
	PageReviewedAt     string `json:"page_reviewed_at"`
	QuestionStatus     string `json:"question_status"`
	QuestionReviewedBy string `json:"question_reviewed_by"`
	QuestionReviewedAt string `json:"question_reviewed_at"`
}

type awaitingDoc struct {
	SchemaVersion int            `json:"schema_version"`
	State         string         `json:"state"`
	Tactic        *tacticRunView `json:"tactic,omitempty"`
	Guidance      *guidanceView  `json:"guidance,omitempty"`
	Skill         *skillRunView  `json:"skill,omitempty"`
	Content       contentView    `json:"content"`
	Question      questionView   `json:"question"`
}

type attemptView struct {
	ID              int64    `json:"id"`
	SessionID       string   `json:"session_id"`
	QuestionID      string   `json:"question_id"`
	AttemptNumber   int      `json:"attempt_number"`
	PriorAttempts   int      `json:"prior_attempts"`
	Repeat          bool     `json:"repeat"`
	Assistance      string   `json:"assistance"`
	TacticID        string   `json:"tactic_id,omitempty"`
	Skills          []string `json:"skills,omitempty"`
	Status          string   `json:"status"`
	SelectedLabel   *string  `json:"selected_label"`
	SelectedText    *string  `json:"selected_text"`
	CorrectLabel    string   `json:"correct_label"`
	CorrectText     string   `json:"correct_text"`
	StartedAt       string   `json:"started_at"`
	AnsweredAt      string   `json:"answered_at"`
	ElapsedMS       int64    `json:"elapsed_ms"`
	ContentID       string   `json:"content_id"`
	ContentVersion  string   `json:"content_version"`
	AssessmentRunID int64    `json:"assessment_run_id,omitempty"`
}

type feedbackView struct {
	Verdict           string `json:"verdict"`
	CorrectLabel      string `json:"correct_label"`
	CorrectText       string `json:"correct_text"`
	Evidence          string `json:"evidence"`
	Reasoning         string `json:"reasoning"`
	SelectedReasoning string `json:"selected_reasoning,omitempty"`
	Exposure          string `json:"exposure"`
	Assistance        string `json:"assistance"`
}

type answeredDoc struct {
	SchemaVersion int            `json:"schema_version"`
	State         string         `json:"state"`
	Tactic        *tacticRunView `json:"tactic,omitempty"`
	Guidance      *guidanceView  `json:"guidance,omitempty"`
	Skill         *skillRunView  `json:"skill,omitempty"`
	Content       contentView    `json:"content"`
	Question      questionView   `json:"question"`
	Attempt       attemptView    `json:"attempt"`
	Feedback      feedbackView   `json:"feedback"`
}

type countsView struct {
	Attempts             int `json:"attempts"`
	Correct              int `json:"correct"`
	Incorrect            int `json:"incorrect"`
	Skipped              int `json:"skipped"`
	FirstAttempts        int `json:"first_attempts"`
	FirstCorrect         int `json:"first_correct"`
	FirstIncorrect       int `json:"first_incorrect"`
	FirstSkipped         int `json:"first_skipped"`
	RepeatedAttempts     int `json:"repeated_attempts"`
	RepeatedCorrect      int `json:"repeated_correct"`
	RepeatedIncorrect    int `json:"repeated_incorrect"`
	RepeatedSkipped      int `json:"repeated_skipped"`
	AssistedAttempts     int `json:"assisted_attempts"`
	AssistedCorrect      int `json:"assisted_correct"`
	AssistedIncorrect    int `json:"assisted_incorrect"`
	AssistedSkipped      int `json:"assisted_skipped"`
	IndependentAttempts  int `json:"independent_attempts"`
	IndependentCorrect   int `json:"independent_correct"`
	IndependentIncorrect int `json:"independent_incorrect"`
	IndependentSkipped   int `json:"independent_skipped"`
}

type questionReport struct {
	ID           string `json:"id"`
	ContentID    string `json:"content_id"`
	Attempts     int    `json:"attempts"`
	FirstStatus  string `json:"first_status"`
	LatestStatus string `json:"latest_status"`
	Repeated     int    `json:"repeated"`
	Assisted     int    `json:"assisted"`
	Independent  int    `json:"independent"`
}

type reportDoc struct {
	SchemaVersion int              `json:"schema_version"`
	Content       *contentView     `json:"content,omitempty"`
	Contents      []contentView    `json:"contents"`
	Counts        countsView       `json:"counts"`
	Questions     []questionReport `json:"questions"`
	Attempts      []attemptView    `json:"attempts"`
	Limitations   []string         `json:"limitations"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorDoc struct {
	SchemaVersion int       `json:"schema_version"`
	Error         errorBody `json:"error"`
}
