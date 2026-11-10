// Package source owns locally imported, page-linked study material: import
// parsing, the review state of pages and questions, and the conversion of a
// reviewed page into practice content. Imported material starts unreviewed and
// is never silently treated as an authoritative key.
package source

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/wxxb789/passquest/internal/exam"
)

// ImportOrigin is how a source entered the local database.
type ImportOrigin string

const (
	ImportTXTDir  ImportOrigin = "txt-directory"
	ImportTXTFile ImportOrigin = "txt-file"
	ImportPDF     ImportOrigin = "pdf"
)

// PageOrigin records where a page's text came from.
type PageOrigin string

const (
	PageNative  PageOrigin = "native"
	PageOCR     PageOrigin = "ocr"
	PageNone    PageOrigin = "none"
	PageUnknown PageOrigin = "unknown"
	PageManual  PageOrigin = "manual"
)

// Valid reports whether an import origin is one this build writes.
func (o ImportOrigin) Valid() bool {
	switch o {
	case ImportTXTDir, ImportTXTFile, ImportPDF:
		return true
	default:
		return false
	}
}

// Valid reports whether a page origin is one this build writes.
func (o PageOrigin) Valid() bool {
	switch o {
	case PageNative, PageOCR, PageNone, PageUnknown, PageManual:
		return true
	default:
		return false
	}
}

// ReviewStatus is the user's statement about a page or question. Imported
// material always starts unreviewed.
type ReviewStatus string

const (
	Unreviewed ReviewStatus = "unreviewed"
	Reviewed   ReviewStatus = "reviewed"
)

// AnswerStatus is whether a question's answer mapping has been reviewed.
type AnswerStatus string

const (
	AnswerUnmapped AnswerStatus = "unmapped"
	AnswerReviewed AnswerStatus = "reviewed"
)

// Flag is a visible doubt about imported material. Flags never change the text
// or the stored review status; they are shown next to it.
type Flag string

const (
	FlagScanUnextracted        Flag = "scan_unextracted"
	FlagOCRUnverified          Flag = "ocr_unverified"
	FlagColumnOrderUnverified  Flag = "column_order_unverified"
	FlagReadingOrderUnverified Flag = "reading_order_unverified"
	FlagProvenanceUnknown      Flag = "provenance_unknown"
	FlagLowText                Flag = "low_text"
	FlagEmpty                  Flag = "empty"
	FlagManifestMismatch       Flag = "manifest_mismatch"
	FlagPageMissing            Flag = "page_missing"
	FlagEvidenceMissing        Flag = "evidence_missing"
	FlagAnswerUnmapped         Flag = "answer_unmapped"
	FlagPageUnreviewed         Flag = "page_unreviewed"
	FlagCorrected              Flag = "corrected"
)

// DefaultRights is stored with every import. Publication rights for the
// owner's source material have not been established.
const DefaultRights = "Private local source: do not redistribute, publish or upload without a recorded rights/consent decision."

// DefaultExamRelation separates private study material from exam facts.
const DefaultExamRelation = "Private study material imported from a local file. Not a live examination, not an exam blueprint, and not a verified answer key unless a page and question are marked reviewed."

// LowTextThreshold is the non-whitespace character count below which a page is
// flagged low_text on import. It is a review prompt, not a quality verdict.
const LowTextThreshold = 300

// ScanThreshold is the non-whitespace character count below which a PDF page is
// treated as having no usable text layer.
const ScanThreshold = 20

// Source is one imported document with page-linked text.
type Source struct {
	ID           string
	Title        string
	Import       ImportOrigin
	Path         string
	SHA256       string
	Notice       string
	Rights       string
	ExamRelation string
	CreatedAt    time.Time
	Pages        []Page
}

// Page is one imported page and its current review state.
type Page struct {
	Number     int
	Origin     PageOrigin
	Section    string
	Review     ReviewStatus
	Flags      []Flag
	Text       string
	Revision   int
	ReviewNote string
	ReviewedBy string
	ReviewedAt time.Time
}

// Question is one question attached to a page. Options and reasoning are
// authored locally; an answer mapping exists only after a review.
type Question struct {
	SourceID        string
	ID              string
	Page            int
	Stem            string
	Options         []exam.Option
	Answer          string
	Evidence        string
	Reasoning       string
	OptionReasoning map[string]string
	AnswerStatus    AnswerStatus
	Review          ReviewStatus
	ReviewedBy      string
	ReviewedAt      time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Summary is the list view of a source.
type Summary struct {
	ID                string
	Title             string
	Import            ImportOrigin
	Path              string
	SHA256            string
	CreatedAt         time.Time
	PageCount         int
	ReviewedPages     int
	QuestionCount     int
	ReviewedQuestions int
}

// HasFlag reports whether the page carries the flag.
func (p Page) HasFlag(flag Flag) bool {
	for _, f := range p.Flags {
		if f == flag {
			return true
		}
	}
	return false
}

// FlagStrings returns the flags in storage order.
func FlagStrings(flags []Flag) []string {
	out := make([]string, 0, len(flags))
	for _, f := range flags {
		out = append(out, string(f))
	}
	return out
}

// FormatFlags renders flags for storage (comma separated) and display.
func FormatFlags(flags []Flag) string {
	return strings.Join(FlagStrings(flags), ",")
}

// ParseFlags decodes the stored flag list.
func ParseFlags(raw string) []Flag {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]Flag, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, Flag(part))
		}
	}
	return out
}

// CountNonSpace counts code points that are not Unicode whitespace. A form feed
// is excluded even where a platform treats it as whitespace, matching the
// baseline manifest's honesty check.
func CountNonSpace(text string) int {
	count := 0
	for _, r := range text {
		if r == '\f' || unicode.IsSpace(r) {
			continue
		}
		count++
	}
	return count
}

// ContentID is the stable practice-content identity of one source page.
func ContentID(sourceID string, page int) string {
	return fmt.Sprintf("%s-p%03d", sourceID, page)
}

// ParseContentID reverses ContentID. It reports false for the embedded sample
// and for ids that were not produced by this rule.
func ParseContentID(id string) (sourceID string, page int, ok bool) {
	i := strings.LastIndex(id, "-p")
	if i <= 0 || i+2 >= len(id) {
		return "", 0, false
	}
	n, err := strconv.Atoi(id[i+2:])
	if err != nil || n < 1 {
		return "", 0, false
	}
	return id[:i], n, true
}

// Identity computes the deterministic source id and content hash. The input is
// title, import kind and every page's number, origin, section and exact text.
func Identity(s Source) (id, digest string) {
	h := sha256.New()
	writeField := func(value string) {
		fmt.Fprintf(h, "%d:", len(value))
		h.Write([]byte(value))
	}
	writeField("passquest-source-v1")
	writeField(s.Title)
	writeField(string(s.Import))
	writeField(strconv.Itoa(len(s.Pages)))
	for _, page := range s.Pages {
		writeField(strconv.Itoa(page.Number))
		writeField(string(page.Origin))
		writeField(page.Section)
		writeField(page.Text)
	}
	sum := h.Sum(nil)
	digest = hex.EncodeToString(sum)
	return "src-" + digest[:16], digest
}

// NoticeFor builds the import notice. It states the origin, coverage and the
// review boundary without claiming the text is usable or verified.
func NoticeFor(s Source) string {
	var missing, scans int
	for _, page := range s.Pages {
		switch {
		case page.HasFlag(FlagPageMissing):
			missing++
		case page.HasFlag(FlagScanUnextracted):
			scans++
		}
	}
	notice := fmt.Sprintf(
		"Imported %d page(s) from a local %s file. Every page starts unreviewed; pages are usable only after a user review. Flags mark unextracted scans, unverified OCR, unverified reading order and manifest disagreements; they are not repaired automatically.",
		len(s.Pages), s.Import,
	)
	if missing > 0 {
		notice += fmt.Sprintf(" %d page(s) are not present in the import and are marked page_missing.", missing)
	}
	if scans > 0 {
		notice += fmt.Sprintf(" %d page(s) have no usable text layer and are marked scan_unextracted.", scans)
	}
	return notice
}

// DefaultTitle derives a title from a local path when the caller supplied none.
func DefaultTitle(path string) string {
	base := strings.TrimSpace(baseName(path))
	if base == "" {
		return "local source"
	}
	return base
}

func baseName(path string) string {
	path = strings.TrimRight(path, "/\\")
	if i := strings.LastIndexAny(path, "/\\"); i >= 0 {
		path = path[i+1:]
	}
	return path
}

// ValidUTF8 reports whether text is valid UTF-8; imported text must be.
func ValidUTF8(text string) bool { return utf8.ValidString(text) }

// nowUTC normalizes injected clocks.
func nowUTC(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}
