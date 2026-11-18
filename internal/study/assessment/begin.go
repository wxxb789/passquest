package assessment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
)

// ValidateSettings refuses a run size, window or annotation mode outside the
// accepted ranges. Both adapters run it after their flag and input checks and
// before the store opens, so a store error keeps its precedence.
func ValidateSettings(size, minutes int, annotations string) error {
	if size < 1 || size > 10_000 {
		return kernel.Refuse("invalid_input", "--size must be between 1 and 10000")
	}
	if minutes < 1 || minutes > 1440 {
		return kernel.Refuse("invalid_input", "--minutes must be between 1 and 1440")
	}
	if annotations != assess.AnnotationsNone {
		return kernel.Refuse("invalid_annotations",
			"--annotations %q is not supported; the only accepted value in this goal is none", annotations)
	}
	return nil
}

// Begin selects unseen units and stores a new active run. The store must
// already be reconciled. Refusals carry the CLI error codes run_active,
// pool_floor, pool_short and invalid_input; every other failure is internal,
// exactly as `assess start` mapped it, so Begin classifies nothing.
func Begin(ctx context.Context, s *study.Service, size, minutes int) (Started, error) {
	_ = ctx
	st := kernel.Store(s)
	if active, found, err := st.ActiveAssessmentRun(); err != nil {
		return Started{}, err
	} else if found {
		return Started{}, kernel.Refuse("run_active",
			"assessment run %d is still active; resume it with `passquest assess resume %d` or abandon it with `passquest assess abandon %d`",
			active.ID, active.ID, active.ID)
	}
	pool, identities, err := poolOf(s)
	if err != nil {
		return Started{}, err
	}
	if pool.ItemCount < assess.MinPoolItems {
		return Started{}, kernel.Refuse("pool_floor", "%s", poolFloorMessage(pool))
	}
	if pool.ItemCount < size {
		return Started{}, kernel.Refuse("pool_short",
			"requested %d item(s), but the unseen pool has %d item(s) on %d unit(s); %d more item(s) must be reviewed and imported",
			size, pool.ItemCount, pool.UnitCount, size-pool.ItemCount)
	}
	units := pool.Select(size)
	actual := assess.SelectedItems(units)
	if actual == 0 {
		return Started{}, kernel.Refuse("pool_short", "no unseen assessment items are available")
	}
	perItem, ok := assess.PerItemTimeout(minutes, actual)
	if !ok {
		return Started{}, kernel.Refuse("invalid_input", "--minutes %d and --size %d leave no per-item timeout", minutes, size)
	}
	now := kernel.P(s).Now()
	run := assess.Run{
		CreatedAt:        now,
		StartedAt:        now,
		DeadlineAt:       now.Add(time.Duration(minutes) * time.Minute),
		WindowMinutes:    minutes,
		RequestedSize:    size,
		ActualSize:       actual,
		PerItemTimeoutMS: perItem,
		Annotations:      assess.AnnotationsNone,
		PoolFamily:       assess.PoolFamily(identities),
		SourceCount:      len(identities),
	}
	items := itemsOf(units)
	created, err := st.CreateAssessmentRun(run, items)
	if errors.Is(err, store.ErrAssessmentActive) {
		return Started{}, kernel.Refuse("run_active", "another assessment run is already active")
	}
	if err != nil {
		return Started{}, err
	}
	return Started{Run: created, Items: items}, nil
}

// poolOf builds every currently practiceable imported unit and the spend
// union in store order: sources by created_at, pages by number. Begin selects
// from this order (assess.Pool.Select takes whole units in pool order), so it
// must not be sorted here; report applies the content-id order the pool
// documents show. Title and passage stay in memory; documents carry ids only.
func poolOf(s *study.Service) (assess.Pool, []assess.SourceIdentity, error) {
	units, identities, err := unitsOf(s)
	if err != nil {
		return assess.Pool{}, nil, err
	}
	spends, err := kernel.Store(s).Spends()
	if err != nil {
		return assess.Pool{}, nil, err
	}
	return assess.BuildPool(units, spends), identities, nil
}

func unitsOf(s *study.Service) ([]assess.Unit, []assess.SourceIdentity, error) {
	st := kernel.Store(s)
	summaries, err := st.Sources()
	if err != nil {
		return nil, nil, err
	}
	identities := make([]assess.SourceIdentity, 0, len(summaries))
	units := make([]assess.Unit, 0)
	for _, summary := range summaries {
		identities = append(identities, assess.SourceIdentity{ID: summary.ID, SHA256: summary.SHA256})
		src, err := st.Source(summary.ID)
		if err != nil {
			return nil, nil, err
		}
		questions, err := st.Questions(summary.ID)
		if err != nil {
			return nil, nil, err
		}
		byPage := make(map[int][]source.Question, len(src.Pages))
		for _, question := range questions {
			byPage[question.Page] = append(byPage[question.Page], question)
		}
		for _, page := range src.Pages {
			content, _, err := source.BuildPracticeExam(src, page, byPage[page.Number])
			if err != nil {
				continue
			}
			units = append(units, assess.Unit{
				ContentID: content.ID,
				SourceID:  src.ID,
				Page:      page.Number,
				Version:   content.Version,
				Passage:   content.Passage.Text,
				Questions: content.Questions,
			})
		}
	}
	return units, identities, nil
}

func itemsOf(units []assess.Unit) []assess.Item {
	var items []assess.Item
	position := 0
	for _, unit := range units {
		for _, question := range unit.Questions {
			items = append(items, assess.Item{
				Position:       position,
				ContentID:      unit.ContentID,
				SourceID:       unit.SourceID,
				Page:           unit.Page,
				ContentVersion: unit.Version,
				QuestionID:     question.ID,
				Skills:         append([]string(nil), question.Tactics...),
				State:          assess.StatePending,
			})
			position++
		}
	}
	return items
}

// Shortfall is how many items the pool is below the floor, never negative: the
// pool document and the floor refusal both print it.
func Shortfall(pool assess.Pool) int {
	shortfall := assess.MinPoolItems - pool.ItemCount
	if shortfall < 0 {
		shortfall = 0
	}
	return shortfall
}

// FloorMet reports whether the pool reaches the item floor a run needs. Both
// pool documents carry the verdict beside the floor itself.
func FloorMet(pool assess.Pool) bool {
	return pool.ItemCount >= assess.MinPoolItems
}

// PoolLimitations are the statements every pool document carries: what one
// content unit is, which count binds, and what never enters the pool.
func PoolLimitations() []string {
	return []string{
		"The pool is counted per content unit (one imported source page): presenting any question on a page spends every question on it.",
		"The qualifying unit count is the binding constraint on how many independent assessments the imported material supports.",
		"No embedded first-party sample or skill passage enters the assessment pool.",
	}
}

func poolFloorMessage(pool assess.Pool) string {
	return fmt.Sprintf(
		"assessment pool has %d qualifying item(s) on %d unseen content unit(s); at least %d are required, so %d more reviewed item(s) must be imported (the unit count is the binding constraint on independent assessments)",
		pool.ItemCount, pool.UnitCount, assess.MinPoolItems, Shortfall(pool))
}
