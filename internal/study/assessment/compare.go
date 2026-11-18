package assessment

import (
	"fmt"

	"github.com/wxxb789/passquest/internal/assess"
)

// Comparison is what comparing two finished runs of one pool family reports:
// the headline deltas (b minus a) and the dimensions both runs share. The
// CLI's comparison document and the window's comparison value both carry it.
type Comparison struct {
	AccuracyPoints float64
	MedianChangeMS int64
	P90ChangeMS    int64
	TimedOutChange int
	ScoredChange   int
	Dimensions     map[string]string
}

// Compare computes the like-for-like difference of two reports. b is the run
// compared against a; assess.Compare has already allowed the pair and pinned
// the dimensions equal, so the first run's are reported.
func Compare(a, b assess.Report) Comparison {
	return Comparison{
		AccuracyPoints: (b.Accuracy.Ratio - a.Accuracy.Ratio) * 100,
		MedianChangeMS: b.Speed.MedianMS - a.Speed.MedianMS,
		P90ChangeMS:    b.Speed.P90MS - a.Speed.P90MS,
		TimedOutChange: b.Counts.TimedOut - a.Counts.TimedOut,
		ScoredChange:   b.Counts.Scored - a.Counts.Scored,
		Dimensions: map[string]string{
			"item_count":     fmt.Sprintf("%d", a.Run.ActualSize),
			"window_minutes": fmt.Sprintf("%d", a.Run.WindowMinutes),
			"pool_family":    a.Run.PoolFamily,
		},
	}
}

// CompareLimitations are the statements every comparison document carries:
// what was compared, what it cannot show, and how contamination was handled.
func CompareLimitations() []string {
	return []string{
		"This compares two local samples with the same item count, time window and pool family.",
		"It does not show that AI, coaching or the app caused any change.",
		"Contaminated items are excluded from each headline; an unequal scored-item count is shown rather than normalized away.",
	}
}
