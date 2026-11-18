// Package recommend orchestrates the `next` recommendation: the policy input
// built from recorded attempts, the adaptive policy's choice, the structured
// target an adapter opens and the presentation the choice spends. Results
// carry exactly the inputs the CLI's view builder and the desktop's studio
// mapper read; nothing here renders.
package recommend

import (
	"context"
	"time"

	"github.com/wxxb789/passquest/internal/adaptive"
	"github.com/wxxb789/passquest/internal/assess"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
)

// Spend says what a recommendation read spends.
type Spend uint8

const (
	// SpendNone reads the recommendation and writes nothing.
	SpendNone Spend = iota
	// SpendRecommended records the unit the recommendation names as
	// presented, once per unit, at PresentationNow.
	SpendRecommended
)

// Result is one recommendation read: the policy's outcome, the input it was
// computed from, the attempts it could not classify and the catalog of
// practiceable material.
type Result struct {
	Adaptive     adaptive.Result
	Input        adaptive.Input
	Unclassified []Unclassified
	Catalog      Catalog
}

// Recommend reads the recorded attempts, runs the adaptive policy and, for
// SpendRecommended, records the recommended unit's presentation at
// PresentationNow. Errors are returned raw: a read raises no refusal.
func Recommend(ctx context.Context, s *study.Service, sourceID string, sp Spend) (Result, error) {
	st := kernel.Store(s)
	input, unclassified, catalog, err := BuildInput(st, sourceID)
	if err != nil {
		return Result{}, err
	}
	result := adaptive.Recommend(input)
	if sp == SpendRecommended {
		if err := RecordPresentation(ctx, s, result.Recommendation, kernel.P(s).PresentationNow()); err != nil {
			return Result{}, err
		}
	}
	return Result{Adaptive: result, Input: input, Unclassified: unclassified, Catalog: catalog}, nil
}

// RecordPresentation records the presentation of the unit the recommendation
// names, once per unit: the pool counts one spend per unit and keeps the
// earliest time, so recording the same recommendation again (the desktop Today
// view does on every render) writes nothing new. A recommendation that names
// no material records nothing.
//
// An adapter whose spend time is a clock read it already made calls this
// directly instead of asking Recommend to spend: the desktop spends at the
// read its render opened with, and a fresh port read would be a clock read the
// render did not make.
func RecordPresentation(ctx context.Context, s *study.Service, rec *adaptive.Recommendation, at time.Time) error {
	_ = ctx
	if rec == nil {
		return nil
	}
	material := rec.FreshMaterial
	if material == nil {
		material = rec.ReviewMaterial
	}
	if material == nil {
		return nil
	}
	_, err := kernel.Store(s).RecordPresentationOnce(material.ContentID, material.QuestionID, assess.SurfaceNext, at)
	return err
}
