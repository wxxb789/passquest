package workspace

// support_test.go holds the internal/cli test helpers the desktop workspace's
// tests used before the package moved (spec §4.3): their unexported copies,
// kept byte-similar to the originals in internal/cli, which are not touched.
// The moved tests pin the copies to the same behaviour; the CLI-driving
// helpers go through cli.Run, a test-only import (the local-only gate checks
// non-test imports only). A later maintenance task deduplicates the two sets.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wxxb789/passquest/internal/anki"
	"github.com/wxxb789/passquest/internal/cli"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study/practice"
	"github.com/wxxb789/passquest/internal/tutor"
)

// TestMain mirrors internal/cli's: the cached assessment seeds are removed
// after the run.
func TestMain(m *testing.M) {
	code := m.Run()
	removeAssessmentSeeds()
	os.Exit(code)
}

// From internal/cli/cli_test.go.

var goldenBase = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

type stepClock struct {
	base  time.Time
	calls int
}

func (c *stepClock) Now() time.Time {
	c.calls++
	return c.base.Add(time.Duration(c.calls) * time.Second)
}

func newDB(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "attempts.sqlite")
}

func runCLI(t *testing.T, db string, clock *stepClock, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	opt := cli.Options{
		Stdin:      strings.NewReader(stdin),
		Stdout:     &stdout,
		Stderr:     &stderr,
		Now:        clock.Now,
		NewSession: func() string { return "sess-test" },
		Getenv:     func(string) string { return "" },
	}
	code := cli.Run(append(args, "--db", db), opt)
	return code, stdout.String(), stderr.String()
}

func mustRun(t *testing.T, db string, clock *stepClock, stdin string, args ...string) string {
	t.Helper()
	code, out, errOut := runCLI(t, db, clock, stdin, args...)
	if code != 0 {
		t.Fatalf("Run(%v) = %d\nstdout:\n%s\nstderr:\n%s", args, code, out, errOut)
	}
	return out
}

// reportDoc and reportAttempt mirror the internal/cli report view's attempt
// rows as far as the moved tests read them (internal/cli/views.go holds the
// full document); the tests keep decoding the CLI's own stdout.
type reportDoc struct {
	Attempts []reportAttempt `json:"attempts"`
}

type reportAttempt struct {
	ID         int64  `json:"id"`
	Repeat     bool   `json:"repeat"`
	Assistance string `json:"assistance"`
	TacticID   string `json:"tactic_id,omitempty"`
	ElapsedMS  int64  `json:"elapsed_ms"`
}

func decodeReport(t *testing.T, raw string) reportDoc {
	t.Helper()
	var doc reportDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode report doc: %v\n%s", err, raw)
	}
	return doc
}

func containsExact(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// From internal/cli/tactic.go: the practice stage names the CLI re-exports.
const (
	stageGuided      = practice.StageGuided
	stageIndependent = practice.StageIndependent
)

// From internal/cli/tutor_test.go.

const fakeTutorKey = "tutor-test-key-not-real"

type fakeTutorTransport struct {
	requests []tutor.HostRequest
	response tutor.HostResponse
	err      error
}

func (f *fakeTutorTransport) Call(_ context.Context, req tutor.HostRequest) (tutor.HostResponse, error) {
	f.requests = append(f.requests, req)
	return f.response, f.err
}

func fixedTutorResponse(text string) tutor.HostResponse {
	return tutor.HostResponse{
		ProtocolVersion: tutor.ProtocolVersion,
		Status:          "ok",
		Text:            text,
		HTTPStatus:      200,
		LatencyMS:       42,
		RequestSent:     true,
		Usage:           &tutor.Usage{PromptTokens: 120, CompletionTokens: 30},
	}
}

func fixedTutorError(code, message string) tutor.HostResponse {
	return tutor.HostResponse{
		ProtocolVersion: tutor.ProtocolVersion,
		Status:          "error",
		HTTPStatus:      500,
		LatencyMS:       20,
		RequestSent:     true,
		Error:           &tutor.HostError{Code: code, Message: message},
	}
}

// From internal/cli/model_test.go.

func modelRegistryFile(t *testing.T, providers ...map[string]any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "registry.json")
	writeModelRegistry(t, path, providers...)
	return path
}

// writeModelRegistry replaces the document at path, so a test can change a
// provider's endpoint between two commands.
func writeModelRegistry(t *testing.T, path string, providers ...map[string]any) {
	t.Helper()
	doc := map[string]any{"version": 1, "providers": providers}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func modelProvider(id, endpoint string) map[string]any {
	return map[string]any{
		"id":             id,
		"name":           strings.ToUpper(id),
		"endpoint":       endpoint,
		"endpoint_shape": provider.EndpointShape,
		"credential_env": "PASSQUEST_" + strings.ToUpper(id) + "_API_KEY",
		"models": []map[string]any{
			{"id": id + "-text", "default": true, "capabilities": []string{"text.chat"}},
		},
	}
}

// From internal/cli/source_test.go and internal/cli/note_test.go.

// fixtureDir is internal/cli's committed import fixture: the workspace tests
// drive the CLI, whose fixtures and goldens stay in internal/cli (spec §0.1).
func fixtureDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "cli", "testdata", "import"))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func fixtureFile(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(fixtureDir(t), name)
}

// importSourceID is internal/cli's importFixture reduced to what the moved
// tests read: the id of the imported source. (importDoc is an internal/cli
// view type, so the copy decodes the two fields it needs.)
func importSourceID(t *testing.T, db string, clock *stepClock) string {
	t.Helper()
	raw := mustRun(t, db, clock, "", "import", "--path", fixtureDir(t), "--json")
	var doc struct {
		Source struct {
			ID string `json:"id"`
		} `json:"source"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode import doc: %v\n%s", err, raw)
	}
	return doc.Source.ID
}

func noteSourceFixture(t *testing.T, db string, clock *stepClock) string {
	t.Helper()
	sourceID := importSourceID(t, db, clock)
	mustRun(t, db, clock, "", "question", "add", sourceID, "1", "--file", fixtureFile(t, "question-q2-tagged.json"), "--json")
	mustRun(t, db, clock, "", "page", "review", sourceID, "1", "--reviewer", "note-reviewer", "--json")
	mustRun(t, db, clock, "", "question", "review", sourceID, "q2", "--answer", "C", "--reviewer", "note-reviewer", "--json")
	return sourceID
}

// From internal/cli/assess_test.go.

func writeAssessmentPages(t *testing.T, dir string, pages int) {
	t.Helper()
	pagesDir := filepath.Join(dir, "pages")
	if err := os.MkdirAll(pagesDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for page := 1; page <= pages; page++ {
		text := fmt.Sprintf("Page %d. The library closes at 4 p.m. on Saturdays. It opens at 9 a.m. on weekdays.\n", page)
		if err := os.WriteFile(filepath.Join(pagesDir, fmt.Sprintf("page-%03d.txt", page)), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// assessmentSeeds caches one seeded database per fixture shape. The first
// test that asks for a shape seeds it under the package cache directory; every
// test then receives a byte copy of that closed database at its own path, so
// each starts from the same initial state without repeating the seeding
// transactions. TestMain removes the directory after m.Run returns; a panic or
// a -timeout kill skips that and leaves a passquest-assess-seed-* directory in
// the OS temp folder. Each run uses a fresh MkdirTemp name, so a leftover is
// never reused as a stale cache.
var assessmentSeeds struct {
	mu      sync.Mutex
	dir     string
	entries map[[2]int]*assessmentSeed
}

type assessmentSeed struct {
	mu       sync.Mutex
	path     string
	sourceID string
}

func removeAssessmentSeeds() {
	assessmentSeeds.mu.Lock()
	defer assessmentSeeds.mu.Unlock()
	if assessmentSeeds.dir != "" {
		_ = os.RemoveAll(assessmentSeeds.dir)
	}
}

// seedAssessmentSource writes an assessment fixture of the given shape to db,
// which must not exist yet, and returns the source id.
func seedAssessmentSource(t *testing.T, db string, pages int, firstPageQuestions int) string {
	t.Helper()
	assessmentSeeds.mu.Lock()
	if assessmentSeeds.dir == "" {
		dir, err := os.MkdirTemp("", "passquest-assess-seed-")
		if err != nil {
			assessmentSeeds.mu.Unlock()
			t.Fatal(err)
		}
		assessmentSeeds.dir = dir
		assessmentSeeds.entries = map[[2]int]*assessmentSeed{}
	}
	key := [2]int{pages, firstPageQuestions}
	seed := assessmentSeeds.entries[key]
	if seed == nil {
		seed = &assessmentSeed{}
		assessmentSeeds.entries[key] = seed
	}
	cacheDir := filepath.Join(assessmentSeeds.dir, fmt.Sprintf("p%d-q%d", pages, firstPageQuestions))
	assessmentSeeds.mu.Unlock()

	seed.mu.Lock()
	defer seed.mu.Unlock()
	if seed.path == "" {
		path := filepath.Join(cacheDir, "seed.sqlite")
		// A failed earlier attempt may have left a partial cache; start over.
		if err := os.RemoveAll(cacheDir); err != nil {
			t.Fatal(err)
		}
		sourceID := seedAssessmentStore(t, path, cacheDir, pages, firstPageQuestions)
		seed.path, seed.sourceID = path, sourceID
	}
	raw, err := os.ReadFile(seed.path)
	if err != nil {
		t.Fatalf("read seeded fixture: %v", err)
	}
	out, err := os.OpenFile(db, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatalf("seedAssessmentSource needs a fresh database path: %v", err)
	}
	if _, err := out.Write(raw); err != nil {
		out.Close()
		t.Fatalf("copy seeded fixture: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("copy seeded fixture: %v", err)
	}
	return seed.sourceID
}

// seedAssessmentStore imports a pages-long fixture from dir into the database
// at db, reviews every page and gives page 1 firstPageQuestions reviewed
// questions and every other page one.
func seedAssessmentStore(t *testing.T, db, dir string, pages int, firstPageQuestions int) string {
	t.Helper()
	writeAssessmentPages(t, dir, pages)
	src, err := source.ImportPath(dir, source.ImportOptions{Title: "assessment fixture", Now: goldenBase})
	if err != nil {
		t.Fatalf("import fixture: %v", err)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.ImportSource(src); err != nil {
		t.Fatalf("store import: %v", err)
	}
	for pageNumber := 1; pageNumber <= pages; pageNumber++ {
		count := 1
		if pageNumber == 1 {
			count = firstPageQuestions
		}
		if _, err := st.ReviewPage(src.ID, pageNumber, "fixture-reviewer", "", goldenBase); err != nil {
			t.Fatalf("review page %d: %v", pageNumber, err)
		}
		page, err := st.Page(src.ID, pageNumber)
		if err != nil {
			t.Fatal(err)
		}
		for index := 1; index <= count; index++ {
			addReviewedFixtureQuestion(t, st, page, src.ID, pageNumber, index)
		}
	}
	return src.ID
}

// addReviewedFixtureQuestion authors and review-approves one fixture question
// on an already reviewed page. It is used by the seed helper and by tests that
// add a question to a page the learner already viewed.
func addReviewedFixtureQuestion(t *testing.T, st *store.Store, page source.Page, sourceID string, pageNumber, index int) {
	t.Helper()
	questionID := fmt.Sprintf("p%02d-q%d", pageNumber, index)
	tag := "vocabulary-in-context"
	if index == 1 {
		tag = "not-except-polarity"
	}
	q := source.Question{
		SourceID: sourceID,
		ID:       questionID,
		Page:     pageNumber,
		Stem:     fmt.Sprintf("Page %d question %d: When does the library close on Saturdays?", pageNumber, index),
		Options: []exam.Option{
			{Label: "A", Text: "At 9 a.m."},
			{Label: "B", Text: "At 4 p.m."},
		},
		Evidence:        "closes at 4 p.m. on Saturdays",
		Reasoning:       fmt.Sprintf("Reasoning for page %d question %d.", pageNumber, index),
		OptionReasoning: map[string]string{"A": "Nine is the weekday opening time.", "B": "The passage states 4 p.m. on Saturdays."},
		Tactics:         []string{tag},
		Glossary: []exam.GlossaryEntry{
			{Term: "closes", Meaning: "shuts"},
		},
		AnswerStatus: source.AnswerUnmapped,
		Review:       source.Unreviewed,
		CreatedAt:    goldenBase,
		UpdatedAt:    goldenBase,
	}
	if err := st.AddQuestion(q); err != nil {
		t.Fatalf("add question %s: %v", questionID, err)
	}
	reviewed, err := q.ReviewAnswer(page, "B", "fixture-reviewer", goldenBase)
	if err != nil {
		t.Fatalf("review answer %s: %v", questionID, err)
	}
	if err := st.SaveQuestion(reviewed, store.EventQuestionReviewed); err != nil {
		t.Fatalf("save question %s: %v", questionID, err)
	}
}

// From internal/cli/note_anki_fake_test.go.

type fakeAnkiTransport struct {
	requests []anki.HostRequest
	actions  []string
	models   []string
	// styles holds each model's stylesheet and card templates; noteModel,
	// when set, is the model name notesInfo reports for every note.
	styles       map[string]fakeAnkiModel
	noteModel    string
	notes        map[int64]map[string]string
	decks        map[int64]string
	cards        map[int64][]int64
	next         int64
	nextCard     int64
	deckOverride string
	duplicate    bool
	unreachable  bool
	// deleteAfterNotesInfo deletes the notes the next notesInfo answer reported,
	// once, right after it is sent: a card the learner removes between the
	// read-back and the write.
	deleteAfterNotesInfo bool
	// strayCards are rows a cardsInfo answer returns for cards this push never
	// asked about.
	strayCards   []map[string]any
	failAddAfter int
	adds         int
	calls        int
	failCall     int
	shortCall    int
	err          error
}

type fakeAnkiModel struct {
	css       string
	templates map[string]anki.CardTemplate
}

func newFakeAnki() *fakeAnkiTransport {
	return &fakeAnkiTransport{
		styles: map[string]fakeAnkiModel{},
		notes:  map[int64]map[string]string{}, decks: map[int64]string{},
		cards: map[int64][]int64{}, next: 101, nextCard: 501,
	}
}

// moveCard records the learner's manual move of a card.
func (f *fakeAnkiTransport) moveCard(id int64, deck string) {
	if _, ok := f.notes[id]; ok {
		f.decks[id] = deck
	}
}

func (f *fakeAnkiTransport) Call(_ context.Context, req anki.HostRequest) (anki.HostResponse, error) {
	f.requests = append(f.requests, req)
	f.calls++
	if f.err != nil {
		return anki.HostResponse{}, f.err
	}
	if f.failCall > 0 && f.calls == f.failCall {
		return anki.HostResponse{}, errors.New("anki host exited before answering")
	}
	resp := anki.HostResponse{ProtocolVersion: anki.ProtocolVersion, State: "ok"}
	for _, action := range req.Actions {
		f.actions = append(f.actions, action.Action)
		result := anki.ActionResult{Action: action.Action, State: "ok"}
		if f.unreachable {
			result.State = "unreachable"
			result.Error = "connection refused"
			resp.State = "unreachable"
			resp.Results = append(resp.Results, result)
			continue
		}
		switch action.Action {
		case "version":
			result.Result = json.RawMessage("6")
		case "createDeck":
			result.Result = json.RawMessage("1")
		case "modelNames":
			raw, _ := json.Marshal(f.models)
			result.Result = raw
		case "createModel":
			name := action.Params["modelName"].(string)
			f.models = append(f.models, name)
			model := fakeAnkiModel{css: action.Params["css"].(string), templates: map[string]anki.CardTemplate{}}
			for _, template := range action.Params["cardTemplates"].([]map[string]string) {
				model.templates[template["Name"]] = anki.CardTemplate{Front: template["Front"], Back: template["Back"]}
			}
			f.styles[name] = model
			result.Result = json.RawMessage("null")
		case "modelStyling", "modelTemplates":
			model, ok := f.styles[action.Params["modelName"].(string)]
			if !ok {
				result.State = "error"
				result.Error = "model was not found"
				break
			}
			if action.Action == "modelStyling" {
				result.Result, _ = json.Marshal(map[string]string{"css": model.css})
			} else {
				result.Result, _ = json.Marshal(model.templates)
			}
		case "updateModelStyling", "updateModelTemplates":
			payload := action.Params["model"].(map[string]any)
			name := payload["name"].(string)
			model, ok := f.styles[name]
			if !ok {
				result.State = "error"
				result.Error = "model was not found"
				break
			}
			if action.Action == "updateModelStyling" {
				model.css = payload["css"].(string)
			} else {
				for templateName, template := range payload["templates"].(map[string]map[string]string) {
					model.templates[templateName] = anki.CardTemplate{Front: template["Front"], Back: template["Back"]}
				}
			}
			f.styles[name] = model
			result.Result = json.RawMessage("null")
		case "addNote":
			f.adds++
			if f.failAddAfter > 0 && f.adds > f.failAddAfter {
				result.State = "unreachable"
				result.Error = "connection lost"
				resp.State = "unreachable"
				break
			}
			if f.duplicate {
				result.State = "error"
				result.Error = "cannot create note because it is a duplicate"
				break
			}
			payload := action.Params["note"].(map[string]any)
			fields := payload["fields"].(map[string]string)
			id := f.next
			f.next++
			f.notes[id] = copyFields(fields)
			f.decks[id] = payload["deckName"].(string)
			if f.deckOverride != "" {
				f.decks[id] = f.deckOverride
			}
			f.cards[id] = []int64{f.nextCard}
			f.nextCard++
			raw, _ := json.Marshal(id)
			result.Result = raw
		case "updateNoteFields":
			payload := action.Params["note"].(map[string]any)
			id := payload["id"].(int64)
			fields, ok := f.notes[id]
			if !ok {
				result.State = "error"
				result.Error = "note was not found"
				break
			}
			for name, value := range payload["fields"].(map[string]string) {
				fields[name] = value
			}
			result.Result = json.RawMessage("null")
		case "notesInfo":
			ids := action.Params["notes"].([]int64)
			var rows []map[string]any
			for _, id := range ids {
				fields, ok := f.notes[id]
				if !ok {
					continue
				}
				encoded := map[string]any{}
				for name, value := range fields {
					encoded[name] = map[string]any{"value": value, "order": 0}
				}
				modelName := note.ModelName
				if f.noteModel != "" {
					modelName = f.noteModel
				}
				rows = append(rows, map[string]any{
					"noteId": id, "modelName": modelName, "fields": encoded, "cards": f.cards[id],
				})
			}
			if f.deleteAfterNotesInfo {
				f.deleteAfterNotesInfo = false
				for _, id := range ids {
					delete(f.notes, id)
					delete(f.cards, id)
					delete(f.decks, id)
				}
			}
			raw, _ := json.Marshal(rows)
			result.Result = raw
		case "cardsInfo":
			cards := action.Params["cards"].([]int64)
			rows := []map[string]any{}
			for _, id := range cards {
				for noteID, ids := range f.cards {
					if !slices.Contains(ids, id) {
						continue
					}
					rows = append(rows, map[string]any{
						"cardId": id, "note": noteID, "deckName": f.decks[noteID],
						"modelName": note.ModelName,
					})
				}
			}
			rows = append(rows, f.strayCards...)
			raw, _ := json.Marshal(rows)
			result.Result = raw
		default:
			result.State = "error"
			result.Error = "unsupported action"
		}
		resp.Results = append(resp.Results, result)
	}
	if f.unreachable {
		resp.State = "unreachable"
	}
	if f.shortCall > 0 && f.calls == f.shortCall && len(resp.Results) > 0 {
		resp.Results = resp.Results[:len(resp.Results)-1]
	}
	return resp, nil
}

func copyFields(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for name, value := range in {
		out[name] = value
	}
	return out
}

func (f *fakeAnkiTransport) edit(id int64, field, value string) {
	if fields, ok := f.notes[id]; ok {
		fields[field] = value
	}
}

func (f *fakeAnkiTransport) count(action string) int {
	total := 0
	for _, got := range f.actions {
		if got == action {
			total++
		}
	}
	return total
}

func (f *fakeAnkiTransport) resetActions() {
	f.actions = nil
	f.requests = nil
	f.calls = 0
}
