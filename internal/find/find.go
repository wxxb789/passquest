// Package find owns the one rule that turns recorded learning evidence into a
// bounded search intent, and the one classification that decides what may be
// shown for local study. It is a pure policy package: no database, no network
// and no way to write an attempt, grade, answer key or recommendation.
package find

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/search"
	"github.com/wxxb789/passquest/internal/skill"
)

// Selection names the rule that produced the intent.
type Selection string

const (
	SelectionSkillErrorRatio   Selection = "skill_error_ratio"
	SelectionLatestWrongTactic Selection = "latest_wrong_tactic"
)

// Classification is the rights decision for one candidate.
type Classification string

const (
	ClassLinkOnly            Classification = "link-only"
	ClassLocalUsable         Classification = "local-usable"
	ClassGeneratedUnverified Classification = "generated-unverified"
)

// Level is an evidence-derived search-difficulty hint, not a measurement of
// exam readiness or a CEFR level.
type Level string

const (
	LevelEarly         Level = "early"
	LevelFoundation    Level = "foundation"
	LevelDeveloping    Level = "developing"
	LevelConsolidating Level = "consolidating"
)

// Record is one attempt as the search policy sees it. Skills is the effective
// snapshot (recorded snapshot, or current tags only for a pre-v6 row); the CLI
// resolves ExamSection from the content the attempt was taken on.
type Record struct {
	AttemptID     int64
	AttemptNumber int
	Assistance    attempt.Assistance
	Status        attempt.Status
	TacticID      string
	Skills        []string
	ExamSection   string
}

// Evidence is the recorded side of the intent, so a later audit can check the
// rule without trusting a reason sentence.
type Evidence struct {
	Attempts   int
	Incorrect  int
	Skipped    int
	AttemptIDs []int64
}

// LevelBasis explains the evidence bucket.
type LevelBasis struct {
	IndependentFirstAttempts int
	Correct                  int
	Skipped                  int
	Rule                     string
}

// Intent is the bounded search request derived from stored evidence.
type Intent struct {
	Selection      Selection
	SkillID        string
	SkillTitle     string
	Kind           string
	TacticID       string
	ExamSection    string
	Level          Level
	LevelBasis     LevelBasis
	CandidateLimit int
	Query          string
	Evidence       Evidence
	Reason         string
}

// Candidate is one classified provider result. Content is stored only for
// local-usable text; Displayable is true only then.
type Candidate struct {
	Title          string
	URL            string
	Classification Classification
	License        string
	Reason         string
	Content        string
	ContentSHA256  string
	Displayable    bool
}

// Derive applies contract clause 1. It returns false when there is no eligible
// skill and no wrong answer that names a tactic.
func Derive(records []Record, candidateLimit int) (Intent, bool) {
	if candidateLimit < 1 {
		candidateLimit = 1
	}
	if candidateLimit > 100 {
		candidateLimit = 100
	}
	ordered := append([]Record(nil), records...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].AttemptID < ordered[j].AttemptID })

	bySkill := make(map[string]*skillEvidence)
	var latestWrong Record
	for _, record := range ordered {
		if record.Status == attempt.StatusIncorrect {
			latestWrong = record
		}
		seen := make(map[string]bool, len(record.Skills))
		for _, id := range record.Skills {
			if !skill.IsRegistered(id) || seen[id] {
				continue
			}
			seen[id] = true
			entry := bySkill[id]
			if entry == nil {
				entry = &skillEvidence{}
				bySkill[id] = entry
			}
			entry.attempts++
			entry.ids = append(entry.ids, record.AttemptID)
			entry.latest = record
			switch record.Status {
			case attempt.StatusIncorrect:
				entry.incorrect++
			case attempt.StatusSkipped:
				entry.skipped++
			}
		}
	}

	var best skill.Skill
	var bestEvidence *skillEvidence
	for _, candidate := range skill.All() {
		entry := bySkill[candidate.ID]
		if entry == nil || entry.attempts < 3 {
			continue
		}
		if bestEvidence == nil || betterRatio(entry, bestEvidence) {
			best = candidate
			bestEvidence = entry
		}
	}

	level, basis := InferLevel(ordered)
	if bestEvidence != nil {
		section := sectionOrUnavailable(bestEvidence.latest.ExamSection)
		reason := fmt.Sprintf(
			"%d of %d attempt(s) for %s are wrong or skipped (%d incorrect, %d skipped); this is the highest error ratio among skills with at least three attempts.",
			bestEvidence.incorrect+bestEvidence.skipped, bestEvidence.attempts, best.ID,
			bestEvidence.incorrect, bestEvidence.skipped,
		)
		if bestEvidence.incorrect+bestEvidence.skipped == 0 {
			reason = fmt.Sprintf(
				"0 of %d attempt(s) for %s are wrong or skipped; this is the highest error ratio among skills with at least three attempts, so no observed error is being claimed.",
				bestEvidence.attempts, best.ID,
			)
		}
		return Intent{
			Selection:      SelectionSkillErrorRatio,
			SkillID:        best.ID,
			SkillTitle:     best.Title,
			Kind:           string(best.Kind),
			TacticID:       best.TacticID,
			ExamSection:    section,
			Level:          level,
			LevelBasis:     basis,
			CandidateLimit: candidateLimit,
			Query:          BuildQuery(best.Title, section, level),
			Evidence: Evidence{
				Attempts:   bestEvidence.attempts,
				Incorrect:  bestEvidence.incorrect,
				Skipped:    bestEvidence.skipped,
				AttemptIDs: append([]int64(nil), bestEvidence.ids...),
			},
			Reason: reason,
		}, true
	}

	tacticID := wrongTactic(latestWrong)
	if tacticID == "" {
		return Intent{}, false
	}
	lesson, _ := skill.Get(tacticID)
	evidence := Evidence{AttemptIDs: []int64{latestWrong.AttemptID}}
	if entry := bySkill[tacticID]; entry != nil {
		evidence = Evidence{
			Attempts:   entry.attempts,
			Incorrect:  entry.incorrect,
			Skipped:    entry.skipped,
			AttemptIDs: append([]int64(nil), entry.ids...),
		}
	}
	section := sectionOrUnavailable(latestWrong.ExamSection)
	return Intent{
		Selection:      SelectionLatestWrongTactic,
		SkillID:        lesson.ID,
		SkillTitle:     lesson.Title,
		Kind:           string(lesson.Kind),
		TacticID:       lesson.TacticID,
		ExamSection:    section,
		Level:          level,
		LevelBasis:     basis,
		CandidateLimit: candidateLimit,
		Query:          BuildQuery(lesson.Title, section, level),
		Evidence:       evidence,
		Reason: fmt.Sprintf(
			"No skill has three attempts; the most recent wrong answer (attempt %d) exercises %s.",
			latestWrong.AttemptID, lesson.ID,
		),
	}, true
}

type skillEvidence struct {
	attempts  int
	incorrect int
	skipped   int
	ids       []int64
	latest    Record
}

// betterRatio compares (incorrect+skipped)/attempts without floating point,
// then prefers the larger attempt count. Registry order is the caller's stable
// final tie-break.
func betterRatio(candidate, current *skillEvidence) bool {
	left := int64(candidate.incorrect+candidate.skipped) * int64(current.attempts)
	right := int64(current.incorrect+current.skipped) * int64(candidate.attempts)
	if left != right {
		return left > right
	}
	return candidate.attempts > current.attempts
}

func wrongTactic(record Record) string {
	if record.AttemptID == 0 {
		return ""
	}
	if lesson, ok := skill.Get(record.TacticID); ok && lesson.Kind == skill.KindScoring {
		return lesson.ID
	}
	for _, id := range record.Skills {
		if lesson, ok := skill.Get(id); ok && lesson.Kind == skill.KindScoring {
			return lesson.ID
		}
	}
	return ""
}

// InferLevel applies the disclosed first-independent-attempt rule.
func InferLevel(records []Record) (Level, LevelBasis) {
	basis := LevelBasis{Rule: "first independent attempts, skips included in the denominator"}
	for _, record := range records {
		if record.AttemptNumber != 1 || record.Assistance != attempt.AssistanceIndependent {
			continue
		}
		basis.IndependentFirstAttempts++
		switch record.Status {
		case attempt.StatusCorrect:
			basis.Correct++
		case attempt.StatusSkipped:
			basis.Skipped++
		}
	}
	switch {
	case basis.IndependentFirstAttempts < 3:
		return LevelEarly, basis
	case basis.Correct*100 < basis.IndependentFirstAttempts*40:
		return LevelFoundation, basis
	case basis.Correct*100 < basis.IndependentFirstAttempts*70:
		return LevelDeveloping, basis
	default:
		return LevelConsolidating, basis
	}
}

// BuildQuery turns the selected skill, section and level into bounded search
// text. It never includes a content id, source title, question, option or
// learner answer.
func BuildQuery(title, section string, level Level) string {
	parts := []string{strings.TrimSpace(title), "English", "exam", "practice"}
	if section != "" && section != "embedded" && section != "unavailable" && section != "unspecified" {
		parts = append(parts, section)
	}
	parts = append(parts, "for", string(level), "level", "learners")
	query := strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
	runes := []rune(query)
	if len(runes) > 300 {
		query = string(runes[:300])
	}
	return query
}

func sectionOrUnavailable(section string) string {
	section = strings.TrimSpace(section)
	if section == "" {
		return "unavailable"
	}
	return section
}

var openLicenses = map[string]bool{
	"cc0-1.0":       true,
	"pdm-1.0":       true,
	"public-domain": true,
	"cc-by-3.0":     true,
	"cc-by-4.0":     true,
	"cc-by-sa-3.0":  true,
	"cc-by-sa-4.0":  true,
}

// Classify applies one precedence: generated, then local-usable, then
// link-only. Only a recognised, provider-declared open license with returned
// text makes a candidate local-usable.
func Classify(items []search.Item) []Candidate {
	out := make([]Candidate, 0, len(items))
	for _, item := range items {
		candidate := Candidate{
			Title:         strings.TrimSpace(item.Title),
			URL:           strings.TrimSpace(item.URL),
			License:       strings.TrimSpace(item.License),
			ContentSHA256: ReturnedContentSHA256(item),
		}
		switch {
		case item.Generated:
			candidate.Classification = ClassGeneratedUnverified
			candidate.Reason = "model_generated"
		case openLicenses[strings.ToLower(candidate.License)] && strings.TrimSpace(item.Content) != "":
			candidate.Classification = ClassLocalUsable
			candidate.Content = item.Content
			candidate.Displayable = true
		default:
			candidate.Classification = ClassLinkOnly
			switch {
			case candidate.License == "":
				candidate.Reason = "no_open_license"
			case !openLicenses[strings.ToLower(candidate.License)]:
				candidate.Reason = "license_not_open"
			default:
				candidate.Reason = "no_returned_text"
			}
		}
		out = append(out, candidate)
	}
	return out
}

// ReturnedContentSHA256 hashes the returned text: content, else snippet, else
// the empty string. The hash identifies what the provider returned even when
// the text itself is not retained.
func ReturnedContentSHA256(item search.Item) string {
	text := item.Content
	if text == "" {
		text = item.Snippet
	}
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// Counts summarises classifications for the command document.
type Counts struct {
	LinkOnly            int
	LocalUsable         int
	GeneratedUnverified int
	Total               int
}

// Count classifies a candidate list.
func Count(candidates []Candidate) Counts {
	out := Counts{Total: len(candidates)}
	for _, candidate := range candidates {
		switch candidate.Classification {
		case ClassLinkOnly:
			out.LinkOnly++
		case ClassLocalUsable:
			out.LocalUsable++
		case ClassGeneratedUnverified:
			out.GeneratedUnverified++
		}
	}
	return out
}

// SanitizeText makes provider text safe for terminal display. Newlines and
// tabs remain; other C0/C1 controls and bidi overrides become spaces. Stored
// text is unchanged, so the provenance hash still matches the response.
func SanitizeText(value string) string {
	return sanitize(value, true)
}

// SanitizeLine makes provider-controlled metadata safe for one terminal line.
// Every C0/C1 control (including CR, LF and tab) and every bidi override
// becomes a space, so a stored newline or terminal escape cannot forge a
// following line. Stored text is unchanged.
func SanitizeLine(value string) string {
	return sanitize(value, false)
}

func sanitize(value string, keepLineBreaks bool) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		switch {
		case keepLineBreaks && (r == '\n' || r == '\t'):
			builder.WriteRune(r)
		case r < 0x20 || (r >= 0x7f && r <= 0x9f):
			builder.WriteRune(' ')
		case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
			builder.WriteRune(' ')
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

// TruncateRunes shortens display text and marks the cut.
func TruncateRunes(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "…"
}
