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
	manifest := fmt.Sprintf("page\tsection\tsource\tbytes\tchars_no_ws\n1\tfront-matter\tnative\t%d\t%d\n2\tmock-exam\tocr\t%d\t%d\n",
		len(page1), CountNonSpace(page1), len(page2), CountNonSpace(page2))
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
	if !first.HasFlag(FlagReadingOrderUnverified) {
		t.Fatalf("native page flags = %v, want reading_order_unverified", first.Flags)
	}
	second := src.Pages[1]
	for _, flag := range []Flag{FlagOCRUnverified, FlagColumnOrderUnverified} {
		if !second.HasFlag(flag) {
			t.Fatalf("OCR flags = %v, missing %s", second.Flags, flag)
		}
	}
	if second.HasFlag(FlagManifestMismatch) {
		// The manifest counts above were written to match the files.
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

func TestImportPDFRejectsMismatchedPageCounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.7 placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ImportPath(path, ImportOptions{
		Now:          testNow,
		PDFPageCount: func(string) (int, error) { return 1, nil },
		PDFText:      func(string) (string, error) { return "one\ftwo", nil },
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
}

func TestImportPDFReportsMissingTool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.7 placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ImportPath(path, ImportOptions{
		Now:          testNow,
		PDFPageCount: func(string) (int, error) { return 0, ErrToolMissing },
	})
	if !errors.Is(err, ErrToolMissing) {
		t.Fatalf("error = %v, want ErrToolMissing", err)
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
	cases := map[string]string{
		"unknown field":  `{"id":"q1","stem":"s","options":[],"evidence":"e","reasoning":"r","option_reasoning":{},"extra":1}`,
		"missing answer": `{"id":"q1","stem":"","options":[],"evidence":"","reasoning":"","option_reasoning":{}}`,
		"bad answer":     strings.Replace(validQuestionJSON("C"), `"answer": "C"`, `"answer": "C"`, 1),
	}
	for name, input := range cases {
		if _, err := ParseQuestionJSON([]byte(input), "src-test", 1, testNow, ""); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%s: error = %v, want ErrInvalidInput", name, err)
		}
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
	page := Page{Number: 2, Origin: PageNative, Review: Reviewed, ReviewedBy: "owner", Text: "it will close at 4 p.m."}
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
	if !strings.Contains(content.Notice, "reviewed by owner") || !strings.Contains(content.Notice, "private") {
		t.Fatalf("notice = %q", content.Notice)
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

func TestPracticeContentIDRoundTrip(t *testing.T) {
	id := ContentID("src-abc123", 24)
	if id != "src-abc123-p024" {
		t.Fatalf("id = %q", id)
	}
	sourceID, page, ok := ParseContentID(id)
	if !ok || sourceID != "src-abc123" || page != 24 {
		t.Fatalf("parse = %q/%d/%v", sourceID, page, ok)
	}
	if _, _, ok := ParseContentID("sample-0001"); ok {
		t.Fatal("sample id parsed as an imported content")
	}
}
