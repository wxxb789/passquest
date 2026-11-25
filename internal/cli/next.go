package cli

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/wxxb789/passquest/internal/adaptive"
	"github.com/wxxb789/passquest/internal/cmdline"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/recommend"
)

func runNext(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"source": true, "db": true})
	fs := newFlagSet("next", opt, wantJSON)
	sourceFlag := fs.String("source", "", "imported source id to include; embedded first-party material is always included")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}

	// The shell is resolved before the recommendation read, which spends: an
	// unusable override must refuse before anything is written, as it did when
	// the check sat between the read and the spend. Its error is reported after
	// the read, so a failing read keeps precedence.
	shell, shellErr := cmdline.DetectShell(opt.Getenv, runtime.GOOS)
	spend := recommend.SpendRecommended
	if shellErr != nil {
		spend = recommend.SpendNone
	}
	db, err := opt.resolveDatabase(*dbFlag)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	svc, err := study.Open(opt.portsOf(db))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	result, err := recommend.Recommend(context.Background(), svc, *sourceFlag, spend)
	if err != nil {
		if *sourceFlag != "" {
			return sourceError(opt, *jsonFlag, err)
		}
		return internalError(opt, *jsonFlag, err)
	}
	if shellErr != nil {
		return usageError(opt, *jsonFlag, "invalid_input", shellErr.Error())
	}
	defaultDB, err := study.DefaultDatabasePath(opt.ConfigDir)
	if err != nil {
		defaultDB = ""
	}
	doc := nextDocOf(result.Adaptive, result.Unclassified, *sourceFlag, result.Catalog.SourceTaggedQuestions,
		cmdline.NextDatabaseArgument(db.Path, defaultDB), commandQuoter{shell: shell})
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeNext(opt.Stdout, doc, svc.Path())
	return 0
}

// buildAdaptiveInput delegates to the recommendation module, mapping its
// unclassified attempts onto this package's document type. It is shared by
// find and TestAssessNextSpendsRecommendedUnit.
func buildAdaptiveInput(st *store.Store, sourceID string) (adaptive.Input, []unclassifiedAttemptView, recommend.Catalog, error) {
	input, unclassified, catalog, err := recommend.BuildInput(st, sourceID)
	return input, unclassifiedViews(unclassified), catalog, err
}

// unclassifiedViews renders the recommendation module's unclassified attempts
// as this package's document type.
func unclassifiedViews(items []recommend.Unclassified) []unclassifiedAttemptView {
	out := make([]unclassifiedAttemptView, 0, len(items))
	for _, item := range items {
		out = append(out, unclassifiedAttemptView{
			ID:         item.ID,
			ContentID:  item.ContentID,
			QuestionID: item.QuestionID,
			Reason:     item.Reason,
		})
	}
	return out
}

func nextDocOf(result adaptive.Result, unclassified []recommend.Unclassified, sourceScope string, sourceTagged int, dbArg string, quoter commandQuoter) nextDoc {
	doc := nextDoc{
		SchemaVersion:         ProtocolVersion,
		State:                 "recommendation",
		Shell:                 shellViewOf(quoter.shell),
		Skills:                make([]nextSkillView, 0, len(result.Skills)),
		UnclassifiedAttempts:  unclassifiedViews(unclassified),
		Limitations:           nextLimitations(len(unclassified)),
		SourceScope:           sourceScope,
		SourceTaggedQuestions: sourceTagged,
	}
	if result.Recommendation == nil {
		doc.State = "no_recommendation"
	}
	for _, candidate := range result.Skills {
		doc.Skills = append(doc.Skills, nextSkillView{
			Skill:     skillRefOf(candidate.Skill),
			Evidence:  nextEvidenceViewOf(candidate.Evidence),
			Inference: nextInferenceViewOf(candidate.Inference),
		})
	}
	if result.Recommendation != nil {
		command, followUp := cmdline.NextCommands(result.Recommendation, dbArg, quoter.renderer())
		doc.Recommendation = &nextRecommendationView{
			Action:             string(result.Recommendation.Action),
			Skill:              skillRefOf(result.Recommendation.Skill),
			State:              string(result.Recommendation.State),
			Reason:             result.Recommendation.Reason,
			Command:            command,
			FollowUpCommand:    followUp,
			EvidenceAttemptIDs: append([]int64{}, result.Recommendation.EvidenceAttemptIDs...),
		}
	}
	return doc
}

func skillRefOf(candidate adaptive.Skill) skillRefView {
	return skillRefView{ID: candidate.ID, Kind: candidate.Kind, Title: candidate.Title}
}

func nextEvidenceViewOf(ev adaptive.Evidence) nextEvidenceView {
	return nextEvidenceView{
		Attempts:                ev.Attempts,
		AssistedAttempts:        ev.AssistedAttempts,
		AssistedCorrect:         ev.AssistedCorrect,
		GuidedAttempts:          ev.GuidedAttempts,
		IndependentAttempts:     ev.IndependentAttempts,
		IndependentCorrect:      ev.IndependentCorrect,
		IndependentIncorrect:    ev.IndependentIncorrect,
		IndependentSkipped:      ev.IndependentSkipped,
		Revisits:                ev.Revisits,
		PassageRepeats:          ev.PassageRepeats,
		DistinctPassages:        ev.DistinctPassages,
		QuestionsSeen:           ev.QuestionsSeen,
		FreshPassages:           ev.FreshPassages,
		FastestIndependentMS:    ev.FastestIndependentMS,
		LatestIndependentMS:     ev.LatestIndependentMS,
		LatestIndependentStatus: string(ev.LatestIndependentStatus),
		LatestAttemptID:         ev.LatestAttemptID,
		LatestStatus:            string(ev.LatestStatus),
		LatestAssistance:        string(ev.LatestAssistance),
		AttemptIDs:              append([]int64{}, ev.AttemptIDs...),
		IndependentAttemptIDs:   append([]int64{}, ev.IndependentAttemptIDs...),
		MappedFromCurrentTags:   ev.MappedFromCurrentTags,
	}
}

func nextInferenceViewOf(inference adaptive.Inference) nextInferenceView {
	return nextInferenceView{
		State:           string(inference.State),
		Action:          string(inference.Action),
		Reason:          inference.Reason,
		CausalDiagnosis: inference.CausalDiagnosis,
		Uncertainty:     append([]string(nil), inference.Uncertainty...),
	}
}

func nextLimitations(unclassified int) []string {
	out := []string{
		"Skill evidence is inferred from the local attempts shown; it is not an exam score, pass estimate or readiness claim.",
		"Only a first independent attempt on a fresh passage counts as independent transfer; assisted, repeated and same-passage attempts are shown separately.",
		"The exam blueprint, time limit and pass threshold are unknown; elapsed time is local prompt-to-answer time, not an exam clock.",
		"Recommendations are recomputed from attempts and their recorded skill snapshots; only pre-v6 attempts fall back to current question tags, and they are marked. No recommendation or derived learner state is stored.",
	}
	if unclassified > 0 {
		out = append(out, fmt.Sprintf("%d attempt(s) could not be mapped to a registered skill and are listed separately; they still mark their passage as seen.", unclassified))
	}
	return out
}

func writeNext(w io.Writer, doc nextDoc, dbPath string) {
	fmt.Fprintln(w, "Pass Quest — next practice")
	if doc.SourceScope != "" {
		fmt.Fprintf(w, "Imported source scope: %s (%d tagged question(s)); embedded first-party material is included.\n", doc.SourceScope, doc.SourceTaggedQuestions)
	}
	if doc.Recommendation == nil {
		fmt.Fprintln(w, "Recommendation: none (no practiceable skill material)")
	} else {
		rec := doc.Recommendation
		fmt.Fprintf(w, "Recommendation: %s — %s (%s)\n", rec.Action, rec.Skill.ID, rec.Skill.Kind)
		fmt.Fprintf(w, "State: %s\n", rec.State)
		fmt.Fprintf(w, "Why: %s\n", rec.Reason)
		fmt.Fprintf(w, "Command: %s\n", rec.Command)
		if rec.FollowUpCommand != "" {
			fmt.Fprintf(w, "Then: %s\n", rec.FollowUpCommand)
		}
		writeShellDisclosure(w, doc)
	}
	fmt.Fprintln(w, "\nEvidence (recorded attempts)")
	for _, candidate := range doc.Skills {
		ev := candidate.Evidence
		latest := ""
		if ev.LatestStatus != "" {
			latest = "  latest " + ev.LatestStatus
		}
		fmt.Fprintf(w, "  %-24s %-8s attempts %d  independent %d (correct %d, incorrect %d, skipped %d)  assisted %d (guided %d)  revisits %d  passage-repeats %d  fresh passages %d%s\n",
			candidate.Skill.ID, candidate.Skill.Kind, ev.Attempts,
			ev.IndependentAttempts, ev.IndependentCorrect, ev.IndependentIncorrect, ev.IndependentSkipped,
			ev.AssistedAttempts, ev.GuidedAttempts, ev.Revisits, ev.PassageRepeats, ev.FreshPassages, latest)
	}
	fmt.Fprintln(w, "\nInference (recomputed from evidence)")
	for _, candidate := range doc.Skills {
		action := candidate.Inference.Action
		if action == "" {
			action = "none"
		}
		fmt.Fprintf(w, "  %-24s %s -> %s\n", candidate.Skill.ID, candidate.Inference.State, action)
		if candidate.Skill.ID == selectedSkillID(doc) {
			for _, line := range candidate.Inference.Uncertainty {
				fmt.Fprintf(w, "    uncertain: %s\n", line)
			}
		}
	}
	if len(doc.UnclassifiedAttempts) > 0 {
		fmt.Fprintf(w, "\nUnclassified attempts: %d\n", len(doc.UnclassifiedAttempts))
		for _, item := range doc.UnclassifiedAttempts {
			fmt.Fprintf(w, "  #%d  %s/%s  %s\n", item.ID, item.ContentID, item.QuestionID, item.Reason)
		}
	}
	fmt.Fprintln(w, "\nLimitations")
	for _, line := range doc.Limitations {
		fmt.Fprintf(w, "  - %s\n", line)
	}
	fmt.Fprintf(w, "\nDatabase: %s\n", dbPath)
}

func selectedSkillID(doc nextDoc) string {
	if doc.Recommendation == nil {
		return ""
	}
	return doc.Recommendation.Skill.ID
}

// writeShellDisclosure states which shell the copied commands were quoted for
// and what decided that family. A guess must be visible: the same path quoted
// for the wrong shell fails only after the learner pastes it.
func writeShellDisclosure(w io.Writer, doc nextDoc) {
	if doc.Shell == nil {
		return
	}
	from := shellSourceDetail(doc.Shell.DetectedFrom, doc.Shell.Detail)
	fmt.Fprintf(w, "Quoted for %s (%s); override with %s.\n", doc.Shell.Family, from, doc.Shell.Override)
	if doc.Shell.Family == string(cmdline.ShellCmd) && recommendationExpandsPercent(doc) {
		fmt.Fprintln(w, "Warning: cmd.exe expands %NAME% even inside double quotes, so this command is not copy-safe with that path; set PASSQUEST_SHELL=powershell or pass the path another way.")
	}
}

func recommendationExpandsPercent(doc nextDoc) bool {
	if doc.Recommendation == nil {
		return false
	}
	return strings.Contains(doc.Recommendation.Command, "%") || strings.Contains(doc.Recommendation.FollowUpCommand, "%")
}

// JSON documents. Field names are protocol: tests and golden files pin them.

type skillRefView struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
}

type nextEvidenceView struct {
	Attempts                int     `json:"attempts"`
	AssistedAttempts        int     `json:"assisted_attempts"`
	AssistedCorrect         int     `json:"assisted_correct"`
	GuidedAttempts          int     `json:"guided_attempts"`
	IndependentAttempts     int     `json:"independent_attempts"`
	IndependentCorrect      int     `json:"independent_correct"`
	IndependentIncorrect    int     `json:"independent_incorrect"`
	IndependentSkipped      int     `json:"independent_skipped"`
	Revisits                int     `json:"revisits"`
	PassageRepeats          int     `json:"passage_repeats"`
	DistinctPassages        int     `json:"distinct_passages"`
	QuestionsSeen           int     `json:"questions_seen"`
	FreshPassages           int     `json:"fresh_passages"`
	FastestIndependentMS    int64   `json:"fastest_independent_ms"`
	LatestIndependentMS     int64   `json:"latest_independent_ms"`
	LatestIndependentStatus string  `json:"latest_independent_status,omitempty"`
	LatestAttemptID         int64   `json:"latest_attempt_id,omitempty"`
	LatestStatus            string  `json:"latest_status,omitempty"`
	LatestAssistance        string  `json:"latest_assistance,omitempty"`
	AttemptIDs              []int64 `json:"attempt_ids"`
	IndependentAttemptIDs   []int64 `json:"independent_attempt_ids"`
	MappedFromCurrentTags   int     `json:"mapped_from_current_tags"`
}

type nextInferenceView struct {
	State           string   `json:"state"`
	Action          string   `json:"action"`
	Reason          string   `json:"reason"`
	CausalDiagnosis string   `json:"causal_diagnosis"`
	Uncertainty     []string `json:"uncertainty"`
}

type nextSkillView struct {
	Skill     skillRefView      `json:"skill"`
	Evidence  nextEvidenceView  `json:"evidence"`
	Inference nextInferenceView `json:"inference"`
}

type nextRecommendationView struct {
	Action             string       `json:"action"`
	Skill              skillRefView `json:"skill"`
	State              string       `json:"state"`
	Reason             string       `json:"reason"`
	Command            string       `json:"command"`
	FollowUpCommand    string       `json:"follow_up_command,omitempty"`
	EvidenceAttemptIDs []int64      `json:"evidence_attempt_ids"`
}

type unclassifiedAttemptView struct {
	ID         int64  `json:"id"`
	ContentID  string `json:"content_id"`
	QuestionID string `json:"question_id"`
	Reason     string `json:"reason"`
}

type nextDoc struct {
	SchemaVersion         int                       `json:"schema_version"`
	State                 string                    `json:"state"`
	Shell                 *shellView                `json:"shell"`
	SourceScope           string                    `json:"source_scope,omitempty"`
	SourceTaggedQuestions int                       `json:"source_tagged_questions"`
	Recommendation        *nextRecommendationView   `json:"recommendation,omitempty"`
	Skills                []nextSkillView           `json:"skills"`
	UnclassifiedAttempts  []unclassifiedAttemptView `json:"unclassified_attempts"`
	Limitations           []string                  `json:"limitations"`
}

// shellView is the quoting dialect of `command` and `follow_up_command`, and
// what decided it. A consumer that prints or re-quotes those commands needs the
// family; the human output prints the same fact beside them.
type shellView struct {
	Family       string `json:"family"`
	DetectedFrom string `json:"detected_from"`
	Detail       string `json:"detail,omitempty"`
	Override     string `json:"override"`
}
