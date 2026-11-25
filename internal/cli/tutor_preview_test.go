package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wxxb789/passquest/internal/tutor"
)

func TestTutorPreviewFirstParty(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{}
	practiceSample(t, db, clock)
	configureTutor(t, db, clock, transport, true)

	raw := mustTutorRun(t, db, clock, transport, true, "tutor", "preview", "--attempt", "1", "--json")
	doc := decodeTutorPreview(t, raw)
	if doc.State != "preview" || doc.Mode != "explain" {
		t.Fatalf("preview state = %+v", doc)
	}
	if doc.Attempt.ID != 1 || doc.Attempt.ContentID != "sample-0001" || doc.Attempt.QuestionID != "q1" {
		t.Fatalf("attempt view = %+v", doc.Attempt)
	}
	if !doc.Provider.CredentialPresent || doc.Provider.CredentialEnv != tutor.CredentialEnv {
		t.Fatalf("provider = %+v", doc.Provider)
	}
	if doc.Context.PrivateSource || doc.Context.SourceTextShared || doc.Context.ContentUnavailable {
		t.Fatalf("context = %+v", doc.Context)
	}
	user := doc.Chat.Messages[1].Content
	for _, want := range []string{"Westbridge", "What time will the library close", "Verified key: B", "elapsed_ms=1000"} {
		if !strings.Contains(user, want) {
			t.Errorf("preview chat is missing %q\n%s", want, user)
		}
	}
	if len(doc.RequestSHA256) != 64 || doc.RequestBytes == 0 {
		t.Fatalf("request identity = %q/%d", doc.RequestSHA256, doc.RequestBytes)
	}
	if strings.Contains(raw, fakeTutorKey) {
		t.Fatal("preview output contains the credential")
	}
	if len(transport.requests) != 0 {
		t.Fatalf("preview sent %d provider requests", len(transport.requests))
	}
}

func TestTutorPrivateSourceConsentRoundTrip(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{response: fixedTutorResponse("Use the notice sentence as evidence.")}
	sourceID := importTutorSource(t, db, clock)
	configureTutor(t, db, clock, transport, true)

	preview := decodeTutorPreview(t, mustTutorRun(t, db, clock, transport, true, "tutor", "preview", "--json"))
	if !preview.Context.PrivateSource || !preview.Context.ConsentRequired || preview.Context.ConsentGranted || preview.Context.SourceTextShared {
		t.Fatalf("private context = %+v", preview.Context)
	}
	if preview.Context.SourceID != sourceID {
		t.Fatalf("preview source = %q, want %q", preview.Context.SourceID, sourceID)
	}
	redacted := preview.Chat.Messages[1].Content
	for _, secret := range []string{
		"The Westbridge Community Library closes at 4 p.m. on Saturdays.",
		"Which of the following is NOT stated",
		"The library closes at 4 p.m. on Saturdays.",
	} {
		if strings.Contains(redacted, secret) {
			t.Errorf("withheld source text leaked: %q\n%s", secret, redacted)
		}
	}
	if !strings.Contains(redacted, "Question content: withheld") || !strings.Contains(redacted, "not-except-polarity") {
		t.Fatalf("redacted prompt = %s", redacted)
	}

	consent := mustTutorRun(t, db, clock, transport, true, "tutor", "consent", sourceID, "--reviewer", "owner", "--json")
	if !strings.Contains(consent, `"granted": true`) || !strings.Contains(consent, `"reviewer": "owner"`) {
		t.Fatalf("consent output = %s", consent)
	}
	authorized := decodeTutorPreview(t, mustTutorRun(t, db, clock, transport, true, "tutor", "preview", "--json"))
	if !authorized.Context.ConsentGranted || !authorized.Context.SourceTextShared {
		t.Fatalf("authorized context = %+v", authorized.Context)
	}
	provided := authorized.Chat.Messages[1].Content
	for _, want := range []string{
		"The Westbridge Community Library closes at 4 p.m. on Saturdays.",
		"Which of the following is NOT stated",
		"Verified evidence: The Westbridge Community Library closes at 4 p.m. on Saturdays.",
	} {
		if !strings.Contains(provided, want) {
			t.Errorf("authorized prompt is missing %q\n%s", want, provided)
		}
	}

	ask := decodeTutorAdvisory(t, mustTutorRun(t, db, clock, transport, true, "tutor", "ask", "--json"))
	if !ask.Call.SourceTextShared {
		t.Fatal("ask record does not mark shared source text")
	}
	if len(transport.requests) != 1 || !strings.Contains(transport.requests[0].Chat.Messages[1].Content, "closes at 4 p.m.") {
		t.Fatal("provider request did not include authorized source text")
	}

	revoked := mustTutorRun(t, db, clock, transport, true, "tutor", "revoke", sourceID, "--reviewer", "owner", "--json")
	if !strings.Contains(revoked, `"granted": false`) {
		t.Fatalf("revoke output = %s", revoked)
	}
	afterRevoke := decodeTutorPreview(t, mustTutorRun(t, db, clock, transport, true, "tutor", "preview", "--json"))
	if afterRevoke.Context.ConsentGranted || afterRevoke.Context.SourceTextShared {
		t.Fatalf("revoked context = %+v", afterRevoke.Context)
	}
	if strings.Contains(afterRevoke.Chat.Messages[1].Content, "The Westbridge Community Library closes at 4 p.m.") {
		t.Fatal("revoked source text was still included")
	}

	consents := mustTutorRun(t, db, clock, transport, true, "tutor", "consents", "--json")
	var consentsDoc tutorConsentsDoc
	if err := json.Unmarshal([]byte(consents), &consentsDoc); err != nil {
		t.Fatal(err)
	}
	if len(consentsDoc.Sources) != 1 || consentsDoc.Sources[0].Granted || consentsDoc.Sources[0].Reviewer != "owner" {
		t.Fatalf("consents = %+v", consentsDoc.Sources)
	}
}

func TestTutorConsentValidation(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{}
	sourceID := importTutorSource(t, db, clock)

	code, _, stderr := runTutorCLI(t, db, clock, transport, true, "tutor", "consent", "src-missing", "--reviewer", "owner", "--json")
	if code != 2 || !strings.Contains(stderr, "unknown_source") {
		t.Fatalf("unknown source = %d %s", code, stderr)
	}
	code, _, stderr = runTutorCLI(t, db, clock, transport, true, "tutor", "consent", sourceID, "--json")
	if code != 2 || !strings.Contains(stderr, "reviewer") {
		t.Fatalf("missing reviewer = %d %s", code, stderr)
	}
	code, _, stderr = runTutorCLI(t, db, clock, transport, true, "tutor", "revoke", "--json")
	if code != 2 || !strings.Contains(stderr, "tutor revoke requires exactly one source id") {
		t.Fatalf("revoke usage = %d %s", code, stderr)
	}
}

// tutorLine returns the first output line containing needle, so a label
// assertion cannot be satisfied by a different line.
func tutorLine(t *testing.T, text, needle string) string {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	t.Fatalf("no output line contains %q:\n%s", needle, text)
	return ""
}

// TestWriteTutorPrivacyContentSummary pins the human content summary against
// the four reachable content states, including whether stored option texts
// were included or withheld.
func TestWriteTutorPrivacyContentSummary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		context tutorContextView
		want    []string
		absent  []string
	}{
		{
			name:    "included first-party content",
			context: tutorContextView{ContentMatched: true, ContentIncluded: true},
			want: []string{
				"Privacy: first-party material only; no private source text is involved",
				"Content: the current passage, question, options, verified key and evidence are included.",
			},
			absent: []string{"Included instead", "Withheld as well"},
		},
		{
			name:    "changed content keeps stored option texts",
			context: tutorContextView{ContentMatched: true, ContentChanged: true, StoredTextIncluded: true},
			want: []string{
				"Content: the current version differs from the recorded attempt; no current passage, question, option, evidence or key text is included.",
				"Included instead: the recorded selection and verified key labels with their stored option texts.",
			},
			absent: []string{"attempt facts and skill lessons", "Withheld as well"},
		},
		{
			name:    "unavailable content keeps stored option texts",
			context: tutorContextView{ContentUnavailable: true, StoredTextIncluded: true},
			want: []string{
				"Content: the attempted question is unavailable in this build; no current passage, question, option, evidence or key text is included.",
				"Included instead: the recorded selection and verified key labels with their stored option texts.",
			},
			absent: []string{"Withheld as well"},
		},
		{
			name: "private content without consent withholds everything",
			context: tutorContextView{
				PrivateSource: true, SourceID: "src-secret",
				Withheld: []string{"passage", "selected_option_text", "correct_option_text"},
			},
			want: []string{
				"Privacy: private source src-secret is NOT authorized; source text shared: no",
				"Content: the current passage and question are withheld because this private source is not authorized.",
				"Withheld as well: the recorded selection and verified key option texts.",
			},
			absent: []string{"Included instead"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var out strings.Builder
			writeTutorPrivacy(&out, test.context)
			got := out.String()
			for _, want := range test.want {
				if !strings.Contains(got, want) {
					t.Errorf("summary is missing %q\n%s", want, got)
				}
			}
			for _, absent := range test.absent {
				if strings.Contains(got, absent) {
					t.Errorf("summary contains %q\n%s", absent, got)
				}
			}
		})
	}
}

func TestTutorHumanPreviewMatchesItsChat(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{}
	practiceSample(t, db, clock)
	configureTutor(t, db, clock, transport, true)

	human := mustTutorRun(t, db, clock, transport, true, "tutor", "preview", "--attempt", "1")
	for _, want := range []string{
		"Pass Quest — tutor preview (not sent)",
		"Privacy: first-party material only; no private source text is involved",
		"Content: the current passage, question, options, verified key and evidence are included.",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("first-party preview is missing %q\n%s", want, human)
		}
	}

	sourceID := importTutorSource(t, db, clock)
	human = mustTutorRun(t, db, clock, transport, true, "tutor", "preview")
	for _, want := range []string{
		"Privacy: private source " + sourceID + " is NOT authorized; source text shared: no",
		"Content: the current passage and question are withheld because this private source is not authorized.",
		"Withheld as well: the recorded selection and verified key option texts.",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("private preview is missing %q\n%s", want, human)
		}
	}
	for _, falseClaim := range []string{"only the stored selection/key are included", "only attempt facts and skill lessons are included"} {
		if strings.Contains(human, falseClaim) {
			t.Errorf("preview repeats the false summary %q\n%s", falseClaim, human)
		}
	}
}

// TestTutorHumanOutputStatesAuthorityAndScope pins the learner-facing labels:
// credentials, advisory status, consent state and history provenance. The JSON
// tests do not protect these strings.
func TestTutorHumanOutputStatesAuthorityAndScope(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{response: fixedTutorResponse("Read the sentence with the closing time.")}

	human := mustTutorRun(t, db, clock, transport, false, "tutor", "config")
	for _, want := range []string{
		"Provider: not configured",
		"Credential: " + tutor.CredentialEnv + " is missing",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("unconfigured config is missing %q\n%s", want, human)
		}
	}

	configureTutor(t, db, clock, transport, true)
	human = mustTutorRun(t, db, clock, transport, true, "tutor", "config")
	for _, want := range []string{
		"Provider endpoint: https://api.example.com/v1/chat/completions",
		"Credential: " + tutor.CredentialEnv + " is set",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("configured config is missing %q\n%s", want, human)
		}
	}

	practiceSample(t, db, clock)
	human = mustTutorRun(t, db, clock, transport, true, "tutor", "ask")
	for _, want := range []string{
		"Pass Quest — advisory AI suggestion (not a verified answer)",
		"--- AI suggestion (advisory) ---",
		"Usage: 120 prompt tokens, 30 completion tokens",
		"This is model output. It is not a verified answer, grade or exam fact.",
		"Advisory call #1 recorded; request sha256 ",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("advisory output is missing %q\n%s", want, human)
		}
	}
	if strings.Contains(human, fakeTutorKey) {
		t.Fatalf("advisory output contains the credential:\n%s", human)
	}

	human = mustTutorRun(t, db, clock, transport, true, "tutor", "show", "1")
	for _, want := range []string{
		"Pass Quest — advisory call #1",
		"--- AI suggestion (advisory) ---",
		"This stored text is model output, not a verified answer, grade or exam fact.",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("show output is missing %q\n%s", want, human)
		}
	}

	human = mustTutorRun(t, db, clock, transport, true, "tutor", "history")
	if line := tutorLine(t, human, "sample-0001/q1"); !strings.Contains(line, "first-party") {
		t.Errorf("history does not label first-party calls:\n%s", line)
	}

	sourceID := importTutorSource(t, db, clock)
	human = mustTutorRun(t, db, clock, transport, true, "tutor", "consents")
	if line := tutorLine(t, human, sourceID); !strings.Contains(line, "withheld") {
		t.Errorf("consents does not label an ungranted source:\n%s", line)
	}

	human = mustTutorRun(t, db, clock, transport, true, "tutor", "consent", sourceID, "--reviewer", "owner")
	if !strings.Contains(human, "Tutor consent for "+sourceID+" (import): authorized") {
		t.Errorf("consent output does not state authorization:\n%s", human)
	}
	human = mustTutorRun(t, db, clock, transport, true, "tutor", "consents")
	if line := tutorLine(t, human, sourceID); !strings.Contains(line, "authorized") {
		t.Errorf("consents does not label a granted source:\n%s", line)
	}

	human = mustTutorRun(t, db, clock, transport, true, "tutor", "ask")
	if !strings.Contains(human, "Privacy: private source "+sourceID+" is authorized; source text shared: yes") {
		t.Errorf("authorized ask does not state shared source text:\n%s", human)
	}
	human = mustTutorRun(t, db, clock, transport, true, "tutor", "history")
	if line := tutorLine(t, human, sourceID); !strings.Contains(line, "source-text-shared") {
		t.Errorf("history does not label a shared private call:\n%s", line)
	}

	human = mustTutorRun(t, db, clock, transport, true, "tutor", "revoke", sourceID, "--reviewer", "owner")
	if !strings.Contains(human, "Tutor consent for "+sourceID+" (import): withheld") {
		t.Errorf("revoke output does not state withholding:\n%s", human)
	}
	human = mustTutorRun(t, db, clock, transport, true, "tutor", "consents")
	if line := tutorLine(t, human, sourceID); !strings.Contains(line, "withheld") {
		t.Errorf("consents does not label a revoked source:\n%s", line)
	}
	human = mustTutorRun(t, db, clock, transport, true, "tutor", "ask")
	if !strings.Contains(human, "Privacy: private source "+sourceID+" is NOT authorized; source text shared: no") {
		t.Errorf("revoked ask does not state withheld source text:\n%s", human)
	}
}

func TestTutorHumanPreviewShowsTheResponsesDocument(t *testing.T) {
	t.Parallel()
	db := newDB(t)
	clock := &stepClock{base: goldenBase}
	transport := &fakeTutorTransport{}
	practiceSample(t, db, clock)
	mustTutorRun(t, db, clock, transport, true,
		"tutor", "config", "--endpoint", "https://api.example.com/v1/responses",
		"--model", "example-model", "--json")

	human := mustTutorRun(t, db, clock, transport, true, "tutor", "preview", "--attempt", "1")
	for _, want := range []string{
		"--- provider responses content (what would be sent) ---",
		"[instructions]\nYou are Pass Quest's text study coach",
		"[user]\nMode: explain",
		"--- end provider responses content ---",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("responses preview is missing %q\n%s", want, human)
		}
	}
	if strings.Contains(human, "provider chat content") {
		t.Errorf("responses preview prints a chat section\n%s", human)
	}
}
