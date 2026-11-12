package cli

import (
	"github.com/wxxb789/passquest/internal/source"
)

type sourceSummaryView struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	Origin            string `json:"import_origin"`
	Path              string `json:"path"`
	SHA256            string `json:"sha256"`
	CreatedAt         string `json:"created_at"`
	PageCount         int    `json:"page_count"`
	ReviewedPages     int    `json:"reviewed_pages"`
	QuestionCount     int    `json:"question_count"`
	ReviewedQuestions int    `json:"reviewed_questions"`
}

type pageSummaryView struct {
	Number       int      `json:"number"`
	Origin       string   `json:"origin"`
	Section      string   `json:"section"`
	ReviewStatus string   `json:"review_status"`
	Flags        []string `json:"flags"`
	Chars        int      `json:"chars_no_ws"`
	Revision     int      `json:"revision"`
	ReviewedBy   string   `json:"reviewed_by"`
	ReviewedAt   string   `json:"reviewed_at"`
}

type pageDetailView struct {
	pageSummaryView
	ReviewNote string `json:"review_note"`
	Text       string `json:"text"`
}

type sourceQuestionView struct {
	ID              string              `json:"id"`
	SourceID        string              `json:"source_id"`
	Page            int                 `json:"page"`
	Stem            string              `json:"stem"`
	Options         []optionView        `json:"options"`
	Answer          *string             `json:"answer"`
	AnswerStatus    string              `json:"answer_status"`
	ReviewStatus    string              `json:"review_status"`
	Evidence        string              `json:"evidence"`
	Reasoning       string              `json:"reasoning"`
	OptionReasoning map[string]string   `json:"option_reasoning"`
	Tactics         []string            `json:"tactics,omitempty"`
	Glossary        []glossaryEntryView `json:"glossary,omitempty"`
	ReviewedBy      string              `json:"reviewed_by"`
	ReviewedAt      string              `json:"reviewed_at"`
	Flags           []string            `json:"flags"`
}

type importDoc struct {
	SchemaVersion int               `json:"schema_version"`
	State         string            `json:"state"`
	Source        sourceSummaryView `json:"source"`
}

type sourcesDoc struct {
	SchemaVersion int                 `json:"schema_version"`
	Sources       []sourceSummaryView `json:"sources"`
}

type showDoc struct {
	SchemaVersion int                  `json:"schema_version"`
	Source        sourceSummaryView    `json:"source"`
	Pages         []pageSummaryView    `json:"pages,omitempty"`
	Page          *pageDetailView      `json:"page,omitempty"`
	Questions     []sourceQuestionView `json:"questions"`
}

type questionsDoc struct {
	SchemaVersion int                  `json:"schema_version"`
	Source        sourceSummaryView    `json:"source"`
	Questions     []sourceQuestionView `json:"questions"`
}

type pageCommandDoc struct {
	SchemaVersion int            `json:"schema_version"`
	State         string         `json:"state"`
	Page          pageDetailView `json:"page"`
}

type questionCommandDoc struct {
	SchemaVersion int                `json:"schema_version"`
	State         string             `json:"state"`
	Question      sourceQuestionView `json:"question"`
}

func sourceSummaryViewOf(summary source.Summary) sourceSummaryView {
	return sourceSummaryView{
		ID:                summary.ID,
		Title:             summary.Title,
		Origin:            string(summary.Import),
		Path:              summary.Path,
		SHA256:            summary.SHA256,
		CreatedAt:         source.FormatTime(summary.CreatedAt),
		PageCount:         summary.PageCount,
		ReviewedPages:     summary.ReviewedPages,
		QuestionCount:     summary.QuestionCount,
		ReviewedQuestions: summary.ReviewedQuestions,
	}
}

func pageSummaryViewOf(page source.Page) pageSummaryView {
	return pageSummaryView{
		Number:       page.Number,
		Origin:       string(page.Origin),
		Section:      page.Section,
		ReviewStatus: string(page.Review),
		Flags:        source.FlagStrings(page.Flags),
		Chars:        source.CountNonSpace(page.Text),
		Revision:     page.Revision,
		ReviewedBy:   page.ReviewedBy,
		ReviewedAt:   source.FormatTime(page.ReviewedAt),
	}
}

func pageDetailViewOf(page source.Page) pageDetailView {
	return pageDetailView{
		pageSummaryView: pageSummaryViewOf(page),
		ReviewNote:      page.ReviewNote,
		Text:            page.Text,
	}
}

func sourceQuestionViewOf(q source.Question, page source.Page) sourceQuestionView {
	var answer *string
	if q.Answer != "" {
		value := q.Answer
		answer = &value
	}
	reasoning := q.OptionReasoning
	if reasoning == nil {
		reasoning = map[string]string{}
	}
	return sourceQuestionView{
		ID:              q.ID,
		SourceID:        q.SourceID,
		Page:            q.Page,
		Stem:            q.Stem,
		Options:         optionViews(q.Options),
		Answer:          answer,
		AnswerStatus:    string(q.AnswerStatus),
		ReviewStatus:    string(q.Review),
		Evidence:        q.Evidence,
		Reasoning:       q.Reasoning,
		OptionReasoning: reasoning,
		Tactics:         append([]string(nil), q.Tactics...),
		Glossary:        glossaryViews(q.Glossary),
		ReviewedBy:      q.ReviewedBy,
		ReviewedAt:      source.FormatTime(q.ReviewedAt),
		Flags:           source.FlagStrings(q.Flags(page)),
	}
}
