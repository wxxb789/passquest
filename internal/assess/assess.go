// Package assess owns the holdout-pool rule, the timed-run schedule and the
// report arithmetic for an unassisted assessment. It is a pure policy package:
// the CLI supplies material, recorded spends and attempts, and the store owns
// persistence. Nothing here contacts a model or a network.
package assess

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
)

const (
	// MinPoolItems is Q-0009-03: a run refuses below this many qualifying
	// unseen items.
	MinPoolItems = 20
	// DefaultSize and DefaultMinutes are Q-0009-01. They are flags, not an
	// invented examination specification.
	DefaultSize    = 40
	DefaultMinutes = 60
	// AnnotationsNone is the only accepted assessment annotation mode in this
	// goal (Q-0009-05).
	AnnotationsNone = "none"
)

const (
	StatusActive    = "active"
	StatusCompleted = "completed"
	StatusAbandoned = "abandoned"
)

const (
	StatePending     = "pending"
	StatePresented   = "presented"
	StateAnswered    = "answered"
	StateTimedOut    = "timed_out"
	StateUnavailable = "unavailable"
)

// Event kinds. They are append-only audit facts; the run and item rows are the
// current state.
const (
	EventCreated     = "created"
	EventPresented   = "presented"
	EventAnswered    = "answered"
	EventTimedOut    = "timed_out"
	EventResumed     = "resumed"
	EventCompleted   = "completed"
	EventAbandoned   = "abandoned"
	EventUnavailable = "unavailable"
)

const (
	SurfacePractice = "practice"
	SurfaceTactic   = "tactic"
	SurfaceSkill    = "skill"
	SurfaceAssess   = "assess"
	SurfaceShow     = "show"
	SurfaceQuestion = "questions"
	SurfaceNext     = "next"
)

// ValidSurface reports whether surface is one this build records.
func ValidSurface(surface string) bool {
	switch surface {
	case SurfacePractice, SurfaceTactic, SurfaceSkill, SurfaceAssess,
		SurfaceShow, SurfaceQuestion, SurfaceNext:
		return true
	default:
		return false
	}
}

// Unit is one practiceable content unit: an imported source page with at least
// one reviewed question that still passes the practice gate. The page is the
// passage boundary this material actually has (Q-0002-03).
type Unit struct {
	ContentID string
	SourceID  string
	Page      int
	Version   string
	Passage   string
	Questions []exam.Question
}

// Spend is one prior presentation or answer. Surface is the recorded surface
// for a content presentation, or "attempt", "guided_exposure" or "note" for the
// older ledgers. RunID is non-zero only for an assessment presentation.
type Spend struct {
	ContentID  string
	QuestionID string
	Surface    string
	At         time.Time
	RunID      int64
}

// PoolUnit is one practiceable unit with its holdout state.
type PoolUnit struct {
	Unit
	Spent        bool
	SpentBy      []string
	FirstSpentAt time.Time
}

// Pool is the computed holdout picture. Spent units stay listed so the learner
// can see what removed them.
type Pool struct {
	Units             []PoolUnit
	PracticeableItems int
	PracticeableUnits int
	ItemCount         int
	UnitCount         int
	SpentUnits        int
}

// SourceIdentity is one imported source's stable family member.
type SourceIdentity struct {
	ID     string
	SHA256 string
}

// PoolFamily hashes the sorted source identities. Importing or removing a
// source changes the family; a page correction inside a source does not.
func PoolFamily(sources []SourceIdentity) string {
	lines := make([]string, 0, len(sources))
	for _, src := range sources {
		lines = append(lines, src.ID+"@"+src.SHA256)
	}
	sort.Strings(lines)
	hash := sha256.New()
	for _, line := range lines {
		hash.Write([]byte(line))
		hash.Write([]byte("\n"))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// BuildPool marks a unit spent when any question on it has any spend event.
// A unit with several questions therefore leaves the pool as a whole, not one
// item at a time.
func BuildPool(units []Unit, spends []Spend) Pool {
	byContent := make(map[string][]Spend, len(spends))
	for _, spend := range spends {
		if strings.TrimSpace(spend.ContentID) == "" {
			continue
		}
		byContent[spend.ContentID] = append(byContent[spend.ContentID], spend)
	}
	pool := Pool{
		Units:             make([]PoolUnit, 0, len(units)),
		PracticeableUnits: len(units),
	}
	for _, unit := range units {
		pool.PracticeableItems += len(unit.Questions)
		entry := PoolUnit{Unit: unit}
		events := byContent[unit.ContentID]
		if len(events) > 0 {
			entry.Spent = true
			pool.SpentUnits++
			seen := make(map[string]bool)
			for _, event := range events {
				surface := event.Surface
				if surface == "" {
					surface = "unknown"
				}
				if !seen[surface] {
					seen[surface] = true
					entry.SpentBy = append(entry.SpentBy, surface)
				}
				if entry.FirstSpentAt.IsZero() || event.At.Before(entry.FirstSpentAt) {
					entry.FirstSpentAt = event.At
				}
			}
			sort.Strings(entry.SpentBy)
		} else {
			pool.ItemCount += len(unit.Questions)
			pool.UnitCount++
		}
		pool.Units = append(pool.Units, entry)
	}
	return pool
}

// Select takes whole unseen units in pool order until at least size items are
// chosen. It never splits a unit: a unit is the holdout boundary, so leaving
// sibling questions behind would spend them without presenting them.
func (p Pool) Select(size int) []Unit {
	if size <= 0 {
		return nil
	}
	var selected []Unit
	items := 0
	for _, entry := range p.Units {
		if entry.Spent {
			continue
		}
		selected = append(selected, entry.Unit)
		items += len(entry.Questions)
		if items >= size {
			break
		}
	}
	return selected
}

// SelectedItems counts the questions in selected units.
func SelectedItems(units []Unit) int {
	total := 0
	for _, unit := range units {
		total += len(unit.Questions)
	}
	return total
}

// PerItemTimeout derives the per-item clock from the whole-run window. The
// value is stored and printed so no caller has to guess it.
func PerItemTimeout(minutes, size int) (int64, bool) {
	if minutes <= 0 || size <= 0 {
		return 0, false
	}
	ms := int64(minutes) * 60_000 / int64(size)
	if ms < 1 {
		return 0, false
	}
	return ms, true
}

// Run is one persisted assessment run.
type Run struct {
	ID               int64
	CreatedAt        time.Time
	StartedAt        time.Time
	DeadlineAt       time.Time
	WindowMinutes    int
	RequestedSize    int
	ActualSize       int
	PerItemTimeoutMS int64
	Annotations      string
	PoolFamily       string
	SourceCount      int
	Status           string
	ResumeCount      int
	LastResumedAt    time.Time
	CompletedAt      time.Time
	AbandonedAt      time.Time
	AbandonedReason  string
}

// Item is one selected question in run order.
type Item struct {
	RunID          int64
	Position       int
	ContentID      string
	SourceID       string
	Page           int
	ContentVersion string
	QuestionID     string
	Skills         []string
	State          string
	PresentedAt    time.Time
	DeadlineAt     time.Time
	AnsweredAt     time.Time
	AttemptID      int64
}

// Event is one append-only run event.
type Event struct {
	ID       int64
	RunID    int64
	Kind     string
	Position int
	At       time.Time
	Detail   string
}

// Finalized reports whether an item has a recorded outcome.
func (i Item) Finalized() bool {
	return i.State == StateAnswered || i.State == StateTimedOut
}

// Annotations is the withheld study material printed in the report after a run.
// Withheld marks material the report refuses to print. An active run withholds
// every item because it can still be resumed; any other run withholds the items
// with no recorded outcome (pending, or presented before an abandon), so a
// resumable or abandoned run never hands the learner the key. An unavailable
// item has no annotation content and keeps its separate unavailable label.
type Annotations struct {
	Withheld        bool
	Evidence        string
	Reasoning       string
	OptionReasoning map[string]string
	// OptionReasoningMissing lists the sorted labels with no authored
	// explanation. Absence is normal for imported book material; the report
	// prints a fixed sentence for it and never stores that sentence as data.
	OptionReasoningMissing []string
	Glossary               []exam.GlossaryEntry
}

// Counts is the recorded side of the report.
type Counts struct {
	Selected            int
	Presented           int
	Finalized           int
	Scored              int
	Correct             int
	Total               int
	Contaminated        int
	TimedOut            int
	Skipped             int
	Unavailable         int
	Pending             int
	AnsweredAfterResume int
}

// Accuracy is the headline ratio over scored items.
type Accuracy struct {
	Correct int
	Total   int
	Ratio   float64
	Band    string
}

// Speed is the elapsed summary over scored items.
type Speed struct {
	MedianMS         int64
	P90MS            int64
	PerItemTimeoutMS int64
	Band             string
	Basis            int
}

// SkillAccuracy is one skill's scored correct/total. An item with several
// skills contributes to each.
type SkillAccuracy struct {
	SkillID string
	Correct int
	Total   int
	Ratio   float64
}

// Contamination lists each category and every excluded item separately.
type Contamination struct {
	PriorExposure      int
	RepeatedVariant    int
	HintsDuringRun     int
	ResumedAfterWindow int
	Excluded           []ItemReport
}

// ItemReport is one item's outcome and annotation.
type ItemReport struct {
	Position    int
	ContentID   string
	QuestionID  string
	SourceID    string
	Page        int
	State       string
	Status      attempt.Status
	TimedOut    bool
	ElapsedMS   int64
	AfterResume bool
	Scored      bool
	Reasons     []string
	Annotations Annotations
}

// Report is the complete recomputed view of one run.
type Report struct {
	Run           Run
	Counts        Counts
	Accuracy      Accuracy
	Speed         Speed
	Skills        []SkillAccuracy
	Contamination Contamination
	Items         []ItemReport
	Unknowns      []string
	Limitations   []string
}

// Unknowns are printed verbatim; they are the honest limits of this database.
var Unknowns = []string{
	"pass threshold: unknown",
	"real item count: unknown",
	"real time limit: unknown",
	"readiness probability: not computed",
	"exam blueprint: not available",
}

// Limitations are printed with every report.
var Limitations = []string{
	"The selected units are the first unseen material in source order, not a random sample; the accuracy is local evidence, not an exam forecast.",
	"Item difficulty is not calibrated, so a cross-run comparison is only valid inside the same pool family and item count.",
	"Accuracy and speed bands are descriptive labels. They are not a pass threshold and no pass/fail verdict is produced.",
	"An item carrying two skills contributes to both per-skill denominators; skipped and timed-out items count as not correct.",
	"Contaminated items are excluded from the headline and per-skill accuracy and listed separately; they are never averaged away.",
	"Elapsed time is local prompt-to-answer time. A terminal command cannot interrupt a blocked read, so a late answer is recorded as a timed-out item when its input returns.",
}
