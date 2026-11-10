package note

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
)

func sampleQuestion() exam.Question {
	return exam.Question{
		ID:   "q2",
		Stem: "Which of the following is NOT stated in the notice?",
		Options: []exam.Option{
			{Label: "A", Text: "The library closes at 4 p.m. on Saturdays."},
			{Label: "B", Text: "The notice is about Saturday hours."},
			{Label: "C", Text: "The library opens at 8 a.m. on Sundays."},
		},
		Answer:   "C",
		Evidence: "The Westbridge Community Library closes at 4 p.m. on Saturdays.",
		Reasoning: "The notice states the Saturday closing time and the page title gives Saturday hours; " +
			"no Sunday opening time is stated.",
		OptionReasoning: map[string]string{
			"A": "The notice states this closing time.",
			"B": "The page title names Saturday hours.",
			"C": "No Sunday opening time appears in the notice.",
		},
		Tactics: []string{"not-except-polarity"},
	}
}

const samplePassage = "Westbridge Library — Saturday hours\nThe Westbridge Community Library closes at 4 p.m. on Saturdays."

func sampleAttempt(status attempt.Status) *attempt.Attempt {
	return &attempt.Attempt{
		ID: 1, ContentID: "src-example-p001", QuestionID: "q2", Status: status,
		SelectedLabel: "A", SelectedText: "The library closes at 4 p.m. on Saturdays.",
		CorrectLabel: "C", CorrectText: "The library opens at 8 a.m. on Sundays.",
		TacticID: "not-except-polarity", Skills: []string{"not-except-polarity"},
	}
}

func baseTarget() Target {
	return Target{ContentID: "src-example-p001", ContentVersion: "v1", QuestionID: "q2",
		SourceID: "src-example", Page: 1, AttemptID: 1, Restricted: true}
}

func TestBuildIncorrectAttemptCarriesFields(t *testing.T) {
	q := sampleQuestion()
	got, err := Build(Input{Target: baseTarget(), Attempt: sampleAttempt(attempt.StatusIncorrect),
		Question: &q, PassageText: samplePassage, ConsentGranted: true})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got.Front != "The library opens at 8 a.m. on Sundays." {
		t.Fatalf("front = %q", got.Front)
	}
	if !strings.Contains(got.Back, "No Sunday opening time") {
		t.Fatalf("back does not carry the key option's explanation: %q", got.Back)
	}
	if !strings.Contains(got.Back, "Example: The Westbridge Community Library closes") {
		t.Fatalf("back does not carry the evidence example: %q", got.Back)
	}
	if got.Tactic != "NOT/EXCEPT polarity" || got.TacticID != "not-except-polarity" {
		t.Fatalf("tactic = %q / %q", got.Tactic, got.TacticID)
	}
	if got.SourceLabel != "src-example · p001 · q2" {
		t.Fatalf("source = %q", got.SourceLabel)
	}
	if !got.Restricted || got.Unanswered || len(got.Withheld) != 0 {
		t.Fatalf("flags = restricted %t unanswered %t withheld %v", got.Restricted, got.Unanswered, got.Withheld)
	}
}

func TestBuildSkippedAttemptNeverReadsTheKey(t *testing.T) {
	q := sampleQuestion()
	got, err := Build(Input{Target: baseTarget(), Attempt: sampleAttempt(attempt.StatusSkipped),
		Question: &q, PassageText: samplePassage, ConsentGranted: true})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !got.Unanswered {
		t.Fatal("skipped attempt is not marked unanswered")
	}
	for _, secret := range []string{"opens at 8 a.m. on Sundays", "No Sunday opening time"} {
		if strings.Contains(got.Front+got.Back, secret) {
			t.Fatalf("skipped draft contains the key (%q): front=%q back=%q", secret, got.Front, got.Back)
		}
	}
	if got.Front != "NOT/EXCEPT polarity" {
		t.Fatalf("front = %q, want the tactic title", got.Front)
	}
	if !strings.Contains(got.Back, "NOT stated") && !strings.Contains(got.Back, "Which of the following") {
		t.Fatalf("back does not carry a visible example: %q", got.Back)
	}
}

func TestBuildMarkOnlyExcludesKeyAndKeyRevealingSentence(t *testing.T) {
	q := sampleQuestion()
	target := Target{ContentID: "src-example-p001", ContentVersion: "v1", QuestionID: "q2",
		SourceID: "src-example", Page: 1, Mark: "closes", Restricted: true}
	got, err := Build(Input{Target: target, Question: &q, PassageText: samplePassage, ConsentGranted: true})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got.Front != "closes" {
		t.Fatalf("front = %q", got.Front)
	}
	for _, secret := range []string{"opens at 8 a.m. on Sundays", "No Sunday opening time"} {
		if strings.Contains(got.Front+got.Back, secret) {
			t.Fatalf("mark-only draft contains the key (%q): %q", secret, got.Back)
		}
	}
	if strings.Contains(got.Back, "Westbridge Community Library closes") {
		t.Fatalf("mark-only draft kept the key-revealing evidence sentence: %q", got.Back)
	}
	if !strings.Contains(got.Back, "Example: Which of the following is NOT stated") {
		t.Fatalf("mark-only example is not the visible stem: %q", got.Back)
	}
}

func TestBuildMarkOnlyKeepsASafePassageSentence(t *testing.T) {
	q := sampleQuestion()
	target := Target{ContentID: "src-example-p001", ContentVersion: "v1", QuestionID: "q2",
		SourceID: "src-example", Page: 1, Mark: "Saturday hours", Restricted: true}
	got, err := Build(Input{Target: target, Question: &q, PassageText: samplePassage, ConsentGranted: true})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(got.Back, "Example: Westbridge Library — Saturday hours") {
		t.Fatalf("back lost the safe sentence: %q", got.Back)
	}
}

func TestBuildRefusesLowValueAndUnansweredWithoutMark(t *testing.T) {
	q := sampleQuestion()
	if _, err := Build(Input{Target: baseTarget(), Attempt: sampleAttempt(attempt.StatusCorrect),
		Question: &q, PassageText: samplePassage}); err != ErrLowValue {
		t.Fatalf("correct attempt error = %v, want ErrLowValue", err)
	}
	if _, err := Build(Input{Target: baseTarget(), Question: &q, PassageText: samplePassage}); err != ErrUnansweredTarget {
		t.Fatalf("no attempt error = %v, want ErrUnansweredTarget", err)
	}
	if _, err := Build(Input{Target: baseTarget()}); err != ErrQuestionUnavailable {
		t.Fatalf("missing question error = %v, want ErrQuestionUnavailable", err)
	}
}

func TestBuildWithholdsRestrictedTextWithoutConsent(t *testing.T) {
	q := sampleQuestion()
	got, err := Build(Input{Target: baseTarget(), Attempt: sampleAttempt(attempt.StatusIncorrect),
		Question: &q, PassageText: samplePassage})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got.Front != "" || got.Back != "" {
		t.Fatalf("withheld draft kept text: front=%q back=%q", got.Front, got.Back)
	}
	if strings.Join(got.Withheld, ",") != "front,back" {
		t.Fatalf("withheld = %v", got.Withheld)
	}
	if got.Tactic == "" || got.SourceLabel == "" {
		t.Fatalf("withholding removed first-party/identifier fields: %#v", got)
	}
}

func TestNormalizeMarkAndSanitizers(t *testing.T) {
	if got := NormalizeMark("  closes\n\tat\t4  "); got != "closes at 4" {
		t.Fatalf("NormalizeMark = %q", got)
	}
	if ValidMark("") || !ValidMark("closes") {
		t.Fatal("ValidMark is wrong")
	}
	if _, err := SanitizeFront("   "); err == nil {
		t.Fatal("SanitizeFront accepted an empty value")
	}
	if got, err := SanitizeBack("meaning\r\n\r\nexample\ttab"); err != nil || got != "meaning\n\nexample tab" {
		t.Fatalf("SanitizeBack = %q, %v", got, err)
	}
	if _, err := SanitizeBack("  "); err == nil {
		t.Fatal("SanitizeBack accepted an empty value")
	}
	if got, err := SanitizeTactic("  NOT / EXCEPT  "); err != nil || got != "NOT / EXCEPT" {
		t.Fatalf("SanitizeTactic = %q, %v", got, err)
	}
}

func TestHTMLFieldsEscapeAndParagraph(t *testing.T) {
	fields := HTML(`a <b> & "quoted"`, "meaning & more\n\nExample: <closes>\nsecond line", "tactic", "src · p1")
	if fields.Front != `a &lt;b&gt; &amp; &#34;quoted&#34;` {
		t.Fatalf("front HTML = %q", fields.Front)
	}
	wantBack := `<div class="pq-meaning">meaning &amp; more</div><div class="pq-example">Example: &lt;closes&gt;<br>second line</div>`
	if fields.Back != wantBack {
		t.Fatalf("back HTML = %q, want %q", fields.Back, wantBack)
	}
	if strings.Contains(fields.Front, "\n") || strings.Contains(fields.Back, "\t") {
		t.Fatal("field HTML contains a raw tab or newline")
	}
	if fields.Hash() == "" || fields.Hash() == HTML("x", "y", "z", "s").Hash() {
		t.Fatal("field hash is not content-specific")
	}
}

func TestContentHashIgnoresProvenance(t *testing.T) {
	a := ContentHash("f", "b", "t", "s", true)
	if a != ContentHash("f", "b", "t", "s", true) {
		t.Fatal("hash is not deterministic")
	}
	if a == ContentHash("f", "b", "t", "s", false) {
		t.Fatal("hash ignores the restricted flag")
	}
	if ContentHash("f", "b", "t", "s", true) == ContentHash("f", "b", "t", "s ", true) {
		t.Fatal("hash ignores content")
	}
}

func TestTargetKeyAndNoteID(t *testing.T) {
	a := Target{ContentID: "c", QuestionID: "q", Mark: "Closes"}
	b := Target{ContentID: "c", QuestionID: "q", Mark: " closes "}
	if a.Key() != b.Key() {
		t.Fatal("mark normalization did not make the key stable")
	}
	if NoteID(a.Key()) != NoteID(a.Key()) || !strings.HasPrefix(NoteID(a.Key()), "note-") {
		t.Fatal("note id is not stable")
	}
	c := Target{ContentID: "c", QuestionID: "q", Mark: "opens"}
	if NoteID(a.Key()) == NoteID(c.Key()) {
		t.Fatal("different marks share a note id")
	}
}

func TestModelTemplateIsSelfContained(t *testing.T) {
	model := TemplateModel()
	if model.Deck != "Pass Quest" || model.Name != "PassQuest Note" {
		t.Fatalf("model names = %q / %q", model.Deck, model.Name)
	}
	if strings.Join(model.Fields, ",") != "Front,Back,Tactic,Source" {
		t.Fatalf("fields = %v", model.Fields)
	}
	css := strings.ToLower(model.CSS)
	for _, forbidden := range []string{"@import", "url(", "http://", "https://", "overflow: hidden", "position: fixed"} {
		if strings.Contains(css, forbidden) {
			t.Fatalf("CSS contains %q", forbidden)
		}
	}
	if !strings.Contains(model.CSS, "overflow-wrap: anywhere") {
		t.Fatal("CSS does not wrap long text")
	}
	if !strings.Contains(model.CSS, "font-family: -apple-system, BlinkMacSystemFont") {
		t.Fatal("CSS does not use the system font stack")
	}
	for _, tag := range []string{"<link", "<script", "<img", "<iframe"} {
		combined := strings.ToLower(model.FrontTemplate + model.BackTemplate + model.CSS)
		if strings.Contains(combined, tag) {
			t.Fatalf("template contains %q", tag)
		}
	}
	for _, placeholder := range []string{"{{Front}}", "{{FrontSide}}", "{{Back}}", "{{Tactic}}", "{{Source}}"} {
		if !strings.Contains(model.FrontTemplate+model.BackTemplate, placeholder) {
			t.Fatalf("template misses %s", placeholder)
		}
	}
	rendered := RenderCard(HTML("front", "meaning\n\nexample", "tactic", "source"))
	if strings.Contains(rendered, "{{") || !strings.Contains(rendered, "pq-front") {
		t.Fatalf("rendered sample is unresolved: %s", rendered)
	}
}

func TestExportBytesAndAtomicWrite(t *testing.T) {
	rows := []Fields{HTML("front", "meaning\n\nexample", "tactic", "source"),
		HTML("front2", "meaning2", "", "source2")}
	data, err := ExportBytes(rows)
	if err != nil {
		t.Fatalf("ExportBytes: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"#separator:tab\n", "#html:true\n", "#notetype:PassQuest Note\n", "#deck:Pass Quest\n",
		"#tags:passquest\n", "Front\tBack\tTactic\tSource\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("export misses %q", want)
		}
	}
	if strings.Count(text, "\n") != 8 {
		t.Fatalf("export line count = %d\n%s", strings.Count(text, "\n"), text)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.tsv")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := WriteExport(rows, path)
	if err != nil {
		t.Fatalf("WriteExport: %v", err)
	}
	if result.Notes != 2 || result.Bytes != len(data) || result.Header != ImportHeader {
		t.Fatalf("result = %+v", result)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != text {
		t.Fatalf("export content = %q, %v", got, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("export mode = %04o, want 0600", perm)
		}
		nested := filepath.Join(t.TempDir(), "private")
		if _, err := WriteExport(rows, filepath.Join(nested, "notes.tsv")); err != nil {
			t.Fatalf("WriteExport into a new directory: %v", err)
		}
		info, err = os.Stat(nested)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Fatalf("new export directory mode = %04o, want 0700", perm)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".notes.tsv.tmp-") {
			t.Fatalf("temporary file left behind: %s", entry.Name())
		}
	}
}

func TestWriteExportFailureLeavesExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX directory modes are not available on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.tsv")
	if err := os.WriteFile(path, []byte("previous"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	if _, err := WriteExport([]Fields{HTML("f", "b", "t", "s")}, path); err == nil {
		t.Fatal("WriteExport succeeded in a read-only directory")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "previous" {
		t.Fatalf("existing export changed: %q, %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".notes.tsv.tmp-") {
			t.Fatalf("temporary file left behind: %s", entry.Name())
		}
	}
}

func TestExportRejectsBreakableFields(t *testing.T) {
	if _, err := ExportBytes([]Fields{{Front: "a\tb"}}); err == nil {
		t.Fatal("ExportBytes accepted a tab")
	}
	if _, err := ExportBytes([]Fields{{Back: "a\nb"}}); err == nil {
		t.Fatal("ExportBytes accepted a newline")
	}
}

func TestParseRefinement(t *testing.T) {
	front, back, err := ParseRefinement("```json\n{\"front\": \"closes\", \"back\": \"meaning\\n\\nexample\"}\n```")
	if err != nil || front != "closes" || back != "meaning\n\nexample" {
		t.Fatalf("ParseRefinement = %q, %q, %v", front, back, err)
	}
	if _, _, err := ParseRefinement("not json"); err == nil {
		t.Fatal("ParseRefinement accepted non-JSON")
	}
	if _, _, err := ParseRefinement(`{"front":"","back":"x"}`); err == nil {
		t.Fatal("ParseRefinement accepted an empty front")
	}
	long := strings.Repeat("x", MaxBackRunes+50)
	_, gotBack, err := ParseRefinement(`{"front":"f","back":"` + long + `"}`)
	if err != nil || len([]rune(gotBack)) > MaxBackRunes+1 {
		t.Fatalf("ParseRefinement did not clip: %d, %v", len([]rune(gotBack)), err)
	}
}

func TestBuildCorrectAttemptWithMarkUsesAnsweredContext(t *testing.T) {
	q := sampleQuestion()
	target := baseTarget()
	target.Mark = "closes"
	got, err := Build(Input{Target: target, Attempt: sampleAttempt(attempt.StatusCorrect),
		Question: &q, PassageText: samplePassage, ConsentGranted: true})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got.Front != "closes" || got.Unanswered {
		t.Fatalf("correct+mark draft = %+v", got)
	}
	if !strings.Contains(got.Back, "The notice states the Saturday closing time") {
		t.Fatalf("correct+mark back lost the authored explanation: %q", got.Back)
	}
	if !strings.Contains(got.Back, "Example: The Westbridge Community Library closes") {
		t.Fatalf("correct+mark back lost the evidence example: %q", got.Back)
	}
}
