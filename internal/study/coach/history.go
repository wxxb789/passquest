package coach

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
	"github.com/wxxb789/passquest/internal/study/models"
	"github.com/wxxb789/passquest/internal/tutor"
)

// HistoryQuery selects the calls one history read returns: every call, or one
// attempt's.
type HistoryQuery struct {
	AttemptID int64
}

// CallHistory is the read `tutor history` renders: the matching stored calls,
// the stored configuration and the day's budget rows its summary shows.
type CallHistory struct {
	Calls      []store.TutorCall
	Configured bool
	Config     tutor.Config
	Daily      int
	Budget     models.Budget
}

// History reads the call ledger in today's `tutor history` order: the attempt
// check, the calls, the configuration, the day's count and the per-provider
// budget.
func History(ctx context.Context, s *study.Service, q HistoryQuery) (CallHistory, error) {
	_ = ctx
	if q.AttemptID < 0 {
		return CallHistory{}, kernel.Refuse("invalid_input", "--attempt must be positive or 0 for all")
	}
	if q.AttemptID > 0 {
		if _, err := attemptRecord(s, q.AttemptID); err != nil {
			return CallHistory{}, err
		}
	}
	calls, err := kernel.Store(s).TutorCalls(q.AttemptID)
	if err != nil {
		return CallHistory{}, err
	}
	cfg, configured, err := kernel.Store(s).TutorConfig()
	if err != nil {
		return CallHistory{}, err
	}
	now := kernel.P(s).Now()
	daily, err := kernel.Store(s).TutorCallCountSince(cfg.Provider, kernel.DayStart(now))
	if err != nil {
		return CallHistory{}, err
	}
	registry, _, err := provider.Load(kernel.P(s).Getenv)
	if err != nil {
		return CallHistory{}, kernel.Refuse("invalid_registry", "%s", err.Error())
	}
	counts, err := kernel.Store(s).ProviderCallCountsSince(kernel.DayStart(kernel.P(s).Now()))
	if err != nil {
		return CallHistory{}, err
	}
	return CallHistory{
		Calls:      calls,
		Configured: configured,
		Config:     cfg,
		Daily:      daily,
		Budget:     models.BudgetOf(cfg, registry, strings.TrimSpace(cfg.Provider), counts),
	}, nil
}

// ConsentEntry is one row of the consent ledger: a source with one recorded
// decision, or a source with none.
type ConsentEntry struct {
	SourceID   string
	Provider   string
	Title      string
	Granted    bool
	RecordedAt *time.Time
	Reviewer   string
}

// Consents reads the consent ledger: every imported source with its recorded
// decisions, grouped by source and provider in today's `tutor consents` order.
func Consents(ctx context.Context, s *study.Service) ([]ConsentEntry, error) {
	_ = ctx
	sources, err := kernel.Store(s).Sources()
	if err != nil {
		return nil, err
	}
	events, err := kernel.Store(s).TutorConsents()
	if err != nil {
		return nil, err
	}
	bySource := make(map[string][]store.TutorConsentEvent)
	for _, event := range events {
		bySource[event.SourceID] = append(bySource[event.SourceID], event)
	}
	rows := make([]ConsentEntry, 0, len(sources))
	for _, summary := range sources {
		matched := bySource[summary.ID]
		if len(matched) == 0 {
			rows = append(rows, ConsentEntry{SourceID: summary.ID, Title: summary.Title})
			continue
		}
		for _, event := range matched {
			at := event.At
			rows = append(rows, ConsentEntry{
				SourceID:   summary.ID,
				Provider:   event.Provider,
				Title:      summary.Title,
				Granted:    event.Granted,
				RecordedAt: &at,
				Reviewer:   event.Reviewer,
			})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].SourceID != rows[j].SourceID {
			return rows[i].SourceID < rows[j].SourceID
		}
		return rows[i].Provider < rows[j].Provider
	})
	return rows, nil
}

// Shown is one stored advisory call: today's `tutor show <id>` minus rendering.
type Shown struct {
	Call store.TutorCall
}

// Call reads one stored call: the read `tutor show` renders and the desktop's
// history performs once per ok call.
func Call(ctx context.Context, s *study.Service, callID int64) (Shown, error) {
	_ = ctx
	call, err := kernel.Store(s).TutorCall(callID)
	if errors.Is(err, store.ErrNotFound) {
		return Shown{}, kernel.Refuse("unknown_call", "tutor call #%d is not recorded", callID)
	}
	if err != nil {
		return Shown{}, err
	}
	return Shown{Call: call}, nil
}
