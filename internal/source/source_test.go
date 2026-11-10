package source

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

func writePageFiles(t *testing.T, dir string, pages map[int]string, manifest string) {
	t.Helper()
	pagesDir := filepath.Join(dir, "pages")
	if err := os.MkdirAll(pagesDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for number, text := range pages {
		name := filepath.Join(pagesDir, "page-"+pad3(number)+".txt")
		if err := os.WriteFile(name, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if manifest != "" {
		if err := os.WriteFile(filepath.Join(dir, "manifest.tsv"), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func pad3(n int) string {
	return fmt.Sprintf("%03d", n)
}

func TestImportTXTDirReadsManifestAndPages(t *testing.T) {
	dir := t.TempDir()
	page1 := "A native page with enough words to inspect and review carefully.\n"
	page2 := "oc r page ~ \\| with doubtful column order and a typo"
	manifest := "page\tsection\tsource\tbytes\tchars_no_ws\n" +
		"1\tfront-matter\tnative\t65\t54\n" +
		"2\tmock-exam\tocr\t52\t41\n"
	writePageFiles(t, dir, map[int]string{1: page1, 2: page2}, manifest)

	src, err := ImportPath(dir, ImportOptions{Now: testNow})
	if err != nil {
		t.Fatalf("ImportPath: %v", err)
	}
	if src.Import != ImportTXTDir {
		t.Fatalf("import = %q", src.Import)
	}
	if src.Title != filepath.Base(dir) {
		t.Fatalf("title = %q", src.Title)
	}
	if len(src.Pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(src.Pages))
	}
	first := src.Pages[0]
	if first.Origin != PageNative || first.Review != Unreviewed {
		t.Fatalf("page 1 origin/review = %s/%s", first.Origin, first.Review)
	}
	if first.Section != "front-matter" {
		t.Fatalf("page 1 section = %q, want front-matter", first.Section)
	}
	if !first.HasFlag(FlagReadingOrderUnverified) {
		t.Fatalf("native page flags = %v, want reading_order_unverified", first.Flags)
	}
	second := src.Pages[1]
	if second.Section != "mock-exam" {
		t.Fatalf("page 2 section = %q, want mock-exam", second.Section)
	}
	for _, flag := range []Flag{FlagOCRUnverified, FlagColumnOrderUnverified} {
		if !second.HasFlag(flag) {
			t.Fatalf("OCR flags = %v, missing %s", second.Flags, flag)
		}
	}
	if second.HasFlag(FlagManifestMismatch) {
		t.Fatalf("unexpected manifest mismatch: %v", second.Flags)
	}
	if !strings.HasPrefix(src.ID, "src-") || len(src.ID) != 20 {
		t.Fatalf("id = %q", src.ID)
	}
	if src.SHA256 == "" || src.CreatedAt != testNow {
		t.Fatalf("sha/created = %q/%s", src.SHA256, src.CreatedAt)
	}
	if !strings.Contains(src.Notice, "unreviewed") {
		t.Fatalf("notice = %q, want an unreviewed statement", src.Notice)
	}

	again, err := ImportPath(dir, ImportOptions{Now: testNow})
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if again.ID != src.ID || again.SHA256 != src.SHA256 {
		t.Fatalf("identity is not deterministic: %s/%s vs %s/%s", src.ID, src.SHA256, again.ID, again.SHA256)
	}
}

func TestImportTXTDirFlagsManifestMismatchAndMissingPages(t *testing.T) {
	dir := t.TempDir()
	writePageFiles(t, dir, map[int]string{
		1: "present text",
	}, "page\tsection\tsource\tbytes\tchars_no_ws\n"+
		"1\tfront-matter\tnative\t999\t999\n"+
		"2\tvocabulary\tnative\t10\t9\n"+
		"3\tmock-exam\tocr\t10\t9\n")

	src, err := ImportPath(dir, ImportOptions{Now: testNow})
	if err != nil {
		t.Fatalf("ImportPath: %v", err)
	}
	if len(src.Pages) != 3 {
		t.Fatalf("pages = %d, want 3 (gaps are preserved)", len(src.Pages))
	}
	if !src.Pages[0].HasFlag(FlagManifestMismatch) {
		t.Fatalf("page 1 flags = %v, want manifest_mismatch", src.Pages[0].Flags)
	}
	for _, page := range src.Pages[1:] {
		if page.Origin != PageNone || !page.HasFlag(FlagPageMissing) || page.Text != "" {
			t.Fatalf("page %d = origin %s flags %v text %q, want page_missing", page.Number, page.Origin, page.Flags, page.Text)
		}
	}
	if !strings.Contains(src.Notice, "2 page(s) are not present") {
		t.Fatalf("notice = %q", src.Notice)
	}
}

func TestImportTitleOverride(t *testing.T) {
	dir := t.TempDir()
	writePageFiles(t, dir, map[int]string{1: "a page with enough text to import"}, "")
	src, err := ImportPath(dir, ImportOptions{Title: "Owner's private baseline", Now: testNow})
	if err != nil {
		t.Fatalf("ImportPath: %v", err)
	}
	if src.Title != "Owner's private baseline" {
		t.Fatalf("title = %q", src.Title)
	}
	without, err := ImportPath(dir, ImportOptions{Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	if without.ID == src.ID {
		t.Fatal("explicit title did not participate in source identity")
	}
}

func TestImportTXTDirRejectsInvalidInput(t *testing.T) {
	empty := t.TempDir()
	if _, err := ImportPath(empty, ImportOptions{Now: testNow}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty dir error = %v, want ErrInvalidInput", err)
	}

	badUTF8 := t.TempDir()
	pagesDir := filepath.Join(badUTF8, "pages")
	if err := os.MkdirAll(pagesDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pagesDir, "page-001.txt"), []byte{0xff, 0xfe}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportPath(badUTF8, ImportOptions{Now: testNow}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid UTF-8 error = %v, want ErrInvalidInput", err)
	}

	badManifest := t.TempDir()
	writePageFiles(t, badManifest, map[int]string{1: "text"}, "page\tbroken\n")
	if _, err := ImportPath(badManifest, ImportOptions{Now: testNow}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("bad manifest error = %v, want ErrInvalidInput", err)
	}
}

func TestImportTXTFileSplitsOnFormFeeds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pages.txt")
	if err := os.WriteFile(path, []byte("first page text\f\fthird page text\f"), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := ImportPath(path, ImportOptions{Now: testNow})
	if err != nil {
		t.Fatalf("ImportPath: %v", err)
	}
	if src.Import != ImportTXTFile || len(src.Pages) != 3 {
		t.Fatalf("import/pages = %s/%d", src.Import, len(src.Pages))
	}
	if src.Pages[0].Origin != PageUnknown || !src.Pages[0].HasFlag(FlagProvenanceUnknown) {
		t.Fatalf("page 1 = %s/%v", src.Pages[0].Origin, src.Pages[0].Flags)
	}
	if !src.Pages[1].HasFlag(FlagEmpty) {
		t.Fatalf("page 2 flags = %v, want empty", src.Pages[1].Flags)
	}
	if src.Pages[2].Text != "third page text" {
		t.Fatalf("page 3 text = %q", src.Pages[2].Text)
	}
}

func TestSplitFormFeedPagesTreatsLineBreakAfterFormFeedAsTerminator(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		pages []string
	}{
		{"bare terminator", "page text\f", []string{"page text"}},
		{"LF after terminator", "page text\f\n", []string{"page text"}},
		{"CRLF after terminator", "page text\f\r\n", []string{"page text"}},
		{"empty last page before terminator", "page one\f\f\n", []string{"page one", ""}},
		{"page separator", "page one\f\npage two", []string{"page one", "page two"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitFormFeedPages(tc.text)
			if len(got) != len(tc.pages) {
				t.Fatalf("chunks = %q, want %q", got, tc.pages)
			}
			for i := range tc.pages {
				if got[i] != tc.pages[i] {
					t.Fatalf("chunks = %q, want %q", got, tc.pages)
				}
			}
		})
	}
}

func TestImportTXTFileTrailingFormFeedNewlineIsNotAPage(t *testing.T) {
	cases := []struct {
		name   string
		ending string
	}{
		{"form feed", "\f"},
		{"form feed LF", "\f\n"},
		{"form feed CRLF", "\f\r\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "trailing.txt")
			if err := os.WriteFile(path, []byte("page text with words"+tc.ending), 0o600); err != nil {
				t.Fatal(err)
			}
			src, err := ImportPath(path, ImportOptions{Now: testNow})
			if err != nil {
				t.Fatalf("ImportPath: %v", err)
			}
			if len(src.Pages) != 1 {
				t.Fatalf("pages = %d, want 1", len(src.Pages))
			}
			if src.Pages[0].Text != "page text with words" {
				t.Fatalf("page text = %q", src.Pages[0].Text)
			}
		})
	}
}

func TestImportTXTFileSplitsPageMarkersAndPreservesGaps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial.txt")
	text := "PASS QUEST — PRIVATE INCOMPLETE BASELINE\n\n" +
		"=== PDF PAGE 1 / 3 | TEXT LAYER (UNREVIEWED) ===\n" +
		"native page one with enough text\n\n" +
		"=== PDF PAGE 3 / 3 | TEXT LAYER (UNREVIEWED) ===\n" +
		"native page three with enough text\n"
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := ImportPath(path, ImportOptions{Now: testNow})
	if err != nil {
		t.Fatalf("ImportPath: %v", err)
	}
	if len(src.Pages) != 3 {
		t.Fatalf("pages = %d, want 3", len(src.Pages))
	}
	if src.Pages[0].Origin != PageNative || !strings.Contains(src.Pages[0].Text, "page one") {
		t.Fatalf("page 1 = %s/%q", src.Pages[0].Origin, src.Pages[0].Text)
	}
	if !src.Pages[1].HasFlag(FlagPageMissing) {
		t.Fatalf("page 2 flags = %v, want page_missing", src.Pages[1].Flags)
	}
	if !strings.Contains(src.Pages[2].Text, "page three") {
		t.Fatalf("page 3 text = %q", src.Pages[2].Text)
	}
}

func TestImportPDFMarksPagesWithoutTextLayer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.7 placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	text := strings.Repeat("native page words ", 3) + "\f\f" + strings.Repeat("third page words ", 3)
	src, err := ImportPath(path, ImportOptions{
		Now:          testNow,
		PDFPageCount: func(string) (int, error) { return 3, nil },
		PDFText:      func(string) (string, error) { return text, nil },
	})
	if err != nil {
		t.Fatalf("ImportPath: %v", err)
	}
	if src.Import != ImportPDF || len(src.Pages) != 3 {
		t.Fatalf("import/pages = %s/%d", src.Import, len(src.Pages))
	}
	if src.Pages[0].Origin != PageNative || !src.Pages[0].HasFlag(FlagReadingOrderUnverified) {
		t.Fatalf("page 1 = %s/%v", src.Pages[0].Origin, src.Pages[0].Flags)
	}
	scan := src.Pages[1]
	if scan.Origin != PageNone || !scan.HasFlag(FlagScanUnextracted) || !scan.HasFlag(FlagEmpty) {
		t.Fatalf("page 2 = %s/%v", scan.Origin, scan.Flags)
	}
	if scan.Text != "" {
		t.Fatalf("page 2 text = %q", scan.Text)
	}
}

func TestImportPDFAcceptsFormFeedNewlinePageSeparators(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.7 placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := ImportPath(path, ImportOptions{
		Now:          testNow,
		PDFPageCount: func(string) (int, error) { return 2, nil },
		PDFText:      func(string) (string, error) { return "first page words\f\nsecond page words", nil },
	})
	if err != nil {
		t.Fatalf("ImportPath: %v", err)
	}
	if len(src.Pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(src.Pages))
	}
	if src.Pages[0].Text != "first page words" || src.Pages[1].Text != "second page words" {
		t.Fatalf("page texts = %q/%q", src.Pages[0].Text, src.Pages[1].Text)
	}
}

func TestImportPDFRejectsMismatchedPageCounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.7 placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		count int
		text  string
	}{
		{"more chunks than pages", 1, "one\ftwo"},
		{"fewer chunks than pages", 3, "one\ftwo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ImportPath(path, ImportOptions{
				Now:          testNow,
				PDFPageCount: func(string) (int, error) { return tc.count, nil },
				PDFText:      func(string) (string, error) { return tc.text, nil },
			})
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestImportPDFThresholdsAndShortText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.7 placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	short, err := ImportPath(path, ImportOptions{
		Now:          testNow,
		PDFPageCount: func(string) (int, error) { return 1, nil },
		PDFText:      func(string) (string, error) { return "abcde", nil },
	})
	if err != nil {
		t.Fatalf("short-page import: %v", err)
	}
	page := short.Pages[0]
	if page.Origin != PageNone || !page.HasFlag(FlagScanUnextracted) || page.HasFlag(FlagEmpty) || page.Text != "abcde" {
		t.Fatalf("short page = %s/%v/%q, want scan_unextracted without empty and raw text kept", page.Origin, page.Flags, page.Text)
	}

	threshold := strings.Repeat("a", 19) + "\f" + strings.Repeat("b", 20)
	src, err := ImportPath(path, ImportOptions{
		Now:          testNow,
		PDFPageCount: func(string) (int, error) { return 2, nil },
		PDFText:      func(string) (string, error) { return threshold, nil },
	})
	if err != nil {
		t.Fatalf("threshold import: %v", err)
	}
	if got := src.Pages[0]; got.Origin != PageNone || !got.HasFlag(FlagScanUnextracted) {
		t.Fatalf("19 chars = %s/%v, want none + scan_unextracted", got.Origin, got.Flags)
	}
	if got := src.Pages[1]; got.Origin != PageNative || !got.HasFlag(FlagReadingOrderUnverified) {
		t.Fatalf("20 chars = %s/%v, want native + reading_order_unverified", got.Origin, got.Flags)
	}
}

func TestImportPDFReportsMissingTool(t *testing.T) {
	t.Setenv("PASSQUEST_PDFINFO", "passquest-test-missing-pdfinfo")
	if _, err := defaultPDFPageCount("/nonexistent/source.pdf"); !errors.Is(err, ErrToolMissing) {
		t.Fatalf("error = %v, want ErrToolMissing", err)
	}
}

func TestImportTXTFileWithoutSeparatorsIsOneUnknownPage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain.txt")
	text := "a plain page without form feeds or page markers"
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := ImportPath(path, ImportOptions{Now: testNow})
	if err != nil {
		t.Fatalf("ImportPath: %v", err)
	}
	if len(src.Pages) != 1 {
		t.Fatalf("pages = %d, want 1", len(src.Pages))
	}
	page := src.Pages[0]
	if page.Origin != PageUnknown || !page.HasFlag(FlagProvenanceUnknown) || page.Text != text {
		t.Fatalf("page = %s/%v/%q", page.Origin, page.Flags, page.Text)
	}
}

func TestFlagsAfterCorrectionKeepsImportFactsAndClearsResolvedDoubts(t *testing.T) {
	flags := FlagsAfterCorrection([]Flag{FlagOCRUnverified, FlagColumnOrderUnverified, FlagLowText, FlagScanUnextracted}, strings.Repeat("corrected text ", 30))
	if !containsFlag(flags, FlagOCRUnverified) || !containsFlag(flags, FlagColumnOrderUnverified) || !containsFlag(flags, FlagCorrected) {
		t.Fatalf("flags = %v, want OCR facts and corrected", flags)
	}
	if containsFlag(flags, FlagLowText) || containsFlag(flags, FlagScanUnextracted) {
		t.Fatalf("flags = %v, want low_text and scan_unextracted cleared", flags)
	}
	empty := FlagsAfterCorrection(flags, "   ")
	if !containsFlag(empty, FlagEmpty) {
		t.Fatalf("empty correction flags = %v", empty)
	}
}

func containsFlag(flags []Flag, want Flag) bool {
	for _, flag := range flags {
		if flag == want {
			return true
		}
	}
	return false
}

func validQuestionJSON(answer string) string {
	return `{
		"id": "q1",
		"stem": "What time does the library close on Saturdays?",
		"options": [
			{"label": "A", "text": "At 7 p.m."},
			{"label": "B", "text": "At 4 p.m."}
		],
		"answer": "` + answer + `",
		"evidence": "it will close at 4 p.m.",
		"reasoning": "The notice states the Saturday closing time.",
		"option_reasoning": {
			"A": "That is the weekday closing time.",
			"B": "The notice gives 4 p.m. for Saturdays."
		}
	}`
}

func TestParseQuestionJSONStaysUnreviewed(t *testing.T) {
	q, err := ParseQuestionJSON([]byte(validQuestionJSON("B")), "src-test", 2, testNow, "")
	if err != nil {
		t.Fatalf("ParseQuestionJSON: %v", err)
	}
	if q.Answer != "B" || q.AnswerStatus != AnswerUnmapped || q.Review != Unreviewed {
		t.Fatalf("question = answer %q/%s review %s, want a proposed but unreviewed answer", q.Answer, q.AnswerStatus, q.Review)
	}
	if q.CreatedAt != testNow || q.UpdatedAt != testNow {
		t.Fatalf("timestamps = %s/%s", q.CreatedAt, q.UpdatedAt)
	}
}

func TestParseQuestionJSONRejectsBadInput(t *testing.T) {
	base := validQuestionJSON("B")
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"unknown field", strings.Replace(base, `"reasoning"`, `"extra": 1, "reasoning"`, 1), "unknown field"},
		{"missing id", strings.Replace(base, `"id": "q1",`, "", 1), "question id is required"},
		{"empty options", `{"id":"q1","stem":"s","options":[],"answer":"","evidence":"e","reasoning":"r","option_reasoning":{}}`, "has 0 options, want 2..8"},
		{"answer not in options", validQuestionJSON("C"), `answer "C" is not one of its options`},
		{"duplicate label", strings.Replace(base, `{"label": "B", "text": "At 4 p.m."}`, `{"label": "A", "text": "At 4 p.m."}`, 1), `option label "A" is duplicated`},
		{"missing explanation", strings.Replace(base, `"A": "That is the weekday closing time.",`, "", 1), "option A has no explanation"},
		{"unknown explanation", strings.Replace(base, `"B": "The notice gives 4 p.m. for Saturdays."`, `"B": "The notice gives 4 p.m. for Saturdays.", "C": "x"`, 1), `explains unknown option "C"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseQuestionJSON([]byte(tc.input), "src-test", 1, testNow, "")
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want fragment %q", err, tc.want)
			}
		})
	}
}

func TestParseQuestionJSONAllowsProposedEmptyAnswer(t *testing.T) {
	q, err := ParseQuestionJSON([]byte(validQuestionJSON("")), "src-test", 1, testNow, "")
	if err != nil {
		t.Fatalf("ParseQuestionJSON: %v", err)
	}
	if q.Answer != "" || q.AnswerStatus != AnswerUnmapped || q.Review != Unreviewed {
		t.Fatalf("question = answer %q/%s review %s", q.Answer, q.AnswerStatus, q.Review)
	}
}

func TestQuestionEditResetsReviewAndAnswerMapping(t *testing.T) {
	page := Page{Number: 2, Review: Reviewed, Text: "it will close at 4 p.m."}
	q, err := ParseQuestionJSON([]byte(validQuestionJSON("B")), "src-test", 2, testNow, "")
	if err != nil {
		t.Fatal(err)
	}
	reviewed, err := q.ReviewAnswer(page, "B", "owner", testNow.Add(time.Hour))
	if err != nil {
		t.Fatalf("ReviewAnswer: %v", err)
	}
	replacement, err := ParseQuestionJSON([]byte(validQuestionJSON("B")), "src-test", 2, testNow, "q1")
	if err != nil {
		t.Fatal(err)
	}
	edited := reviewed.Edited(replacement, testNow.Add(2*time.Hour))
	if edited.Review != Unreviewed || edited.AnswerStatus != AnswerUnmapped || edited.ReviewedBy != "" {
		t.Fatalf("edited review state = %s/%s/%q", edited.Review, edited.AnswerStatus, edited.ReviewedBy)
	}
	if !edited.CreatedAt.Equal(reviewed.CreatedAt) {
		t.Fatalf("created_at changed: %s -> %s", reviewed.CreatedAt, edited.CreatedAt)
	}
}

func TestReviewAnswerRequiresReviewedPageAndMatchingEvidence(t *testing.T) {
	q, err := ParseQuestionJSON([]byte(validQuestionJSON("B")), "src-test", 2, testNow, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.ReviewAnswer(Page{Number: 2, Review: Unreviewed, Text: "it will close at 4 p.m."}, "B", "owner", testNow); !errors.Is(err, ErrNotPracticeable) {
		t.Fatalf("unreviewed page error = %v", err)
	}
	if _, err := q.ReviewAnswer(Page{Number: 2, Review: Reviewed, Text: "different text"}, "B", "owner", testNow); !errors.Is(err, ErrNotPracticeable) {
		t.Fatalf("missing evidence error = %v", err)
	}
	if _, err := q.ReviewAnswer(Page{Number: 2, Review: Reviewed, Text: "it will close at 4 p.m."}, "B", "", testNow); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty reviewer error = %v", err)
	}
	reviewed, err := q.ReviewAnswer(Page{Number: 2, Review: Reviewed, Text: "it will close at 4 p.m."}, "B", "owner", testNow)
	if err != nil {
		t.Fatalf("ReviewAnswer: %v", err)
	}
	if reviewed.AnswerStatus != AnswerReviewed || reviewed.Review != Reviewed || reviewed.ReviewedBy != "owner" {
		t.Fatalf("reviewed = %s/%s/%s", reviewed.AnswerStatus, reviewed.Review, reviewed.ReviewedBy)
	}
}

func TestBuildPracticeExamOnlyServesReviewedQuestions(t *testing.T) {
	page := Page{Number: 2, Origin: PageNative, Review: Reviewed, ReviewedBy: "owner", ReviewedAt: testNow, Text: "it will close at 4 p.m."}
	ready, err := ParseQuestionJSON([]byte(validQuestionJSON("B")), "src-test", 2, testNow, "")
	if err != nil {
		t.Fatal(err)
	}
	ready, err = ready.ReviewAnswer(page, "B", "owner", testNow)
	if err != nil {
		t.Fatal(err)
	}
	unreviewed, err := ParseQuestionJSON([]byte(strings.Replace(validQuestionJSON("A"), `"id": "q1"`, `"id": "q2"`, 1)), "src-test", 2, testNow, "")
	if err != nil {
		t.Fatal(err)
	}
	stale := ready
	stale.ID = "q3"
	stale.Evidence = "not on this page"

	src := Source{ID: "src-test", Title: "Test source", Import: ImportTXTFile, Path: "/tmp/x.txt", SHA256: "abc", Rights: DefaultRights, ExamRelation: DefaultExamRelation}

	content, staleIDs, err := BuildPracticeExam(src, page, []Question{ready, unreviewed, stale})
	if err != nil {
		t.Fatalf("BuildPracticeExam: %v", err)
	}
	if len(content.Questions) != 1 || content.Questions[0].ID != "q1" {
		t.Fatalf("questions = %+v, want only q1", content.Questions)
	}
	if len(staleIDs) != 1 || staleIDs[0] != "q3" {
		t.Fatalf("stale = %v, want q3", staleIDs)
	}
	if content.ID != ContentID("src-test", 2) || !strings.HasPrefix(content.Version, "1-") {
		t.Fatalf("content id/version = %s/%s", content.ID, content.Version)
	}
	for _, want := range []string{
		"page reviewed by owner at 2026-11-10T09:00:00Z",
		"q1 by owner at 2026-11-10T09:00:00Z",
		"private",
	} {
		if !strings.Contains(content.Notice, want) {
			t.Fatalf("notice = %q, want %q", content.Notice, want)
		}
	}

	_, _, err = BuildPracticeExam(src, Page{Number: 2, Review: Unreviewed, Text: page.Text}, []Question{ready})
	if !errors.Is(err, ErrNotPracticeable) {
		t.Fatalf("unreviewed page error = %v", err)
	}
	_, staleIDs, err = BuildPracticeExam(src, page, []Question{stale})
	if !errors.Is(err, ErrNotPracticeable) || len(staleIDs) != 1 {
		t.Fatalf("all-stale error = %v, stale = %v", err, staleIDs)
	}
}

func TestQuestionFlagsMarkEvidenceMissing(t *testing.T) {
	page := Page{Number: 2, Origin: PageNative, Review: Reviewed, Text: "it will close at 4 p.m."}
	q, err := ParseQuestionJSON([]byte(validQuestionJSON("B")), "src-test", 2, testNow, "")
	if err != nil {
		t.Fatal(err)
	}
	reviewed, err := q.ReviewAnswer(page, "B", "owner", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if flags := reviewed.Flags(page); containsFlag(flags, FlagEvidenceMissing) {
		t.Fatalf("flags = %v, want no evidence_missing while the evidence is present", flags)
	}
	corrected := page
	corrected.Text = "the page now says something else entirely"
	flags := reviewed.Flags(corrected)
	if !containsFlag(flags, FlagEvidenceMissing) {
		t.Fatalf("flags = %v, want evidence_missing after the page text changed", flags)
	}
	if PracticeReady(corrected, reviewed) {
		t.Fatal("PracticeReady accepted a reviewed question whose evidence is gone")
	}
}

func TestPracticeContentIDRoundTrip(t *testing.T) {
	id := ContentID("src-abc123", 24)
	if id != "src-abc123-p024" {
		t.Fatalf("id = %q", id)
	}
	accepted := []struct {
		id       string
		sourceID string
		page     int
	}{
		{"src-abc123-p024", "src-abc123", 24},
		{"src-abc123-p012", "src-abc123", 12},
	}
	for _, tc := range accepted {
		sourceID, page, ok := ParseContentID(tc.id)
		if !ok || sourceID != tc.sourceID || page != tc.page {
			t.Fatalf("ParseContentID(%q) = %q/%d/%v", tc.id, sourceID, page, ok)
		}
	}
	for _, raw := range []string{"sample-0001", "foo-p12", "src-abc123-p000", "-p001"} {
		if _, _, ok := ParseContentID(raw); ok {
			t.Fatalf("ParseContentID(%q) accepted an id not produced by ContentID", raw)
		}
	}
}
