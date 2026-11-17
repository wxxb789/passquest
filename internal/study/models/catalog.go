package models

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
	"github.com/wxxb789/passquest/internal/tutor"
)

// Listing is one catalog read: the registry, the stored configuration, the
// verification index, the day's budget and the active pair's resolution —
// exactly the inputs `model list`, `model budget` and the window's Today read.
// Each entry point below fills what its caller renders and reads nothing else.
type Listing struct {
	Source   string
	Registry provider.Registry
	// RegistryErr is the registry load failure, carried as data: the CLI
	// refuses it as invalid_registry; the window renders it as a problem line
	// with an empty budget.
	RegistryErr error
	Configured  bool
	// Active is the saved configuration, default-filled and endpoint-refreshed
	// when the pair resolves.
	Active tutor.Config
	// Model is the resolved active model, zero when there is none or it no
	// longer resolves.
	Model provider.Model
	// ActiveErr is the active pair's resolution failure, carried as data: the
	// CLI's `model list` refuses it as unknown_model; the window renders it as
	// a problem line and still shows the budget.
	ActiveErr error
	// Index is the recorded verification evidence. It is zero for the entry
	// points that render none: Usage, and Status without a resolved pair.
	Index  kernel.VerificationIndex
	Budget Budget
}

// Catalog is the read `model list` renders: the registry, the stored
// configuration, every provider's verification evidence and — once the active
// pair resolves — the day's recorded calls. `model list` refuses an
// unresolvable pair before it renders, so that usage read is not made when
// ActiveErr is set. It never refuses: the registry failure and the pair
// failure are returned as data.
func Catalog(ctx context.Context, s *study.Service) (Listing, error) {
	_ = ctx
	registry, source, err := provider.Load(kernel.P(s).Getenv)
	if err != nil {
		return Listing{RegistryErr: err}, nil
	}
	cfg, configured, err := storedConfig(s)
	if err != nil {
		return Listing{}, err
	}
	index, err := kernel.LoadVerificationIndex(s)
	if err != nil {
		return Listing{}, err
	}
	activeID := strings.TrimSpace(cfg.Provider)
	_, model, refreshed, _, activeErr := kernel.ActiveProvider(cfg, registry)
	listing := Listing{
		Source:     source,
		Registry:   registry,
		Configured: configured,
		Active:     cfg,
		Model:      model,
		ActiveErr:  activeErr,
		Index:      index,
	}
	if activeErr != nil {
		return listing, nil
	}
	listing.Active = refreshed
	budget, err := dayBudget(s, refreshed, registry, activeID)
	if err != nil {
		return Listing{}, err
	}
	listing.Budget = budget
	return listing, nil
}

// Usage is the read `model budget` renders: the registry, the stored
// configuration and the day's recorded calls. No verification evidence and no
// active pair, exactly like today's command.
func Usage(ctx context.Context, s *study.Service) (Listing, error) {
	_ = ctx
	registry, source, err := provider.Load(kernel.P(s).Getenv)
	if err != nil {
		return Listing{RegistryErr: err}, nil
	}
	cfg, configured, err := storedConfig(s)
	if err != nil {
		return Listing{}, err
	}
	budget, err := dayBudget(s, cfg, registry, strings.TrimSpace(cfg.Provider))
	if err != nil {
		return Listing{}, err
	}
	return Listing{
		Source:     source,
		Registry:   registry,
		Configured: configured,
		Active:     cfg,
		Budget:     budget,
	}, nil
}

// Status is the read the window's Today renders: the stored configuration
// first, then the registry, the active pair and the day's recorded calls; the
// pair's verification evidence only when Today renders a text.chat state for
// it (configured, non-legacy, resolved). A registry failure is data: Today
// renders the problem line and reads nothing else.
func Status(ctx context.Context, s *study.Service) (Listing, error) {
	_ = ctx
	cfg, configured, err := storedConfig(s)
	if err != nil {
		return Listing{}, err
	}
	registry, source, err := provider.Load(kernel.P(s).Getenv)
	if err != nil {
		return Listing{Configured: configured, Active: cfg, RegistryErr: err}, nil
	}
	activeID := strings.TrimSpace(cfg.Provider)
	_, model, refreshed, _, activeErr := kernel.ActiveProvider(cfg, registry)
	listing := Listing{
		Source:     source,
		Registry:   registry,
		Configured: configured,
		Active:     cfg,
		Model:      model,
		ActiveErr:  activeErr,
	}
	if activeErr == nil {
		listing.Active = refreshed
	}
	if configured && activeID != "" && activeErr == nil {
		index, err := kernel.LoadVerificationIndex(s)
		if err != nil {
			return Listing{}, err
		}
		listing.Index = index
	}
	budget, err := dayBudget(s, listing.Active, registry, activeID)
	if err != nil {
		return Listing{}, err
	}
	listing.Budget = budget
	return listing, nil
}

// dayBudget reads the day's recorded provider calls and renders them, the one
// store read every catalog entry point makes at its own point in its sequence.
func dayBudget(s *study.Service, cfg tutor.Config, registry provider.Registry, activeID string) (Budget, error) {
	counts, err := kernel.Store(s).ProviderCallCountsSince(kernel.DayStart(kernel.P(s).Now()))
	if err != nil {
		return Budget{}, err
	}
	return BudgetOf(cfg, registry, activeID, counts), nil
}

// Budget is the per-provider daily call budget of the UTC day.
type Budget struct {
	PerProviderLimit int
	Providers        []BudgetProvider
	TotalCalls       int
	TotalLimit       int
	ProviderCount    int
	Ceiling          string
}

// BudgetProvider is one provider's usage today.
type BudgetProvider struct {
	Provider string
	Label    string
	Calls    int
	Limit    int
	Active   bool
}

// BudgetOf turns the day's recorded calls into the per-provider budget both
// adapters render: today's cli.buildBudgetView, minus its store and clock
// reads.
func BudgetOf(cfg tutor.Config, registry provider.Registry, activeID string, counts []store.ProviderCallCount) Budget {
	limit := cfg.DailyCalls
	if limit <= 0 {
		limit = tutor.DefaultDailyCalls
	}
	countByProvider := map[string]int{}
	for _, item := range counts {
		countByProvider[item.Provider] += item.Calls
	}
	seen := map[string]bool{}
	view := Budget{PerProviderLimit: limit}
	add := func(id, label string, counted bool) {
		if seen[id] {
			return
		}
		seen[id] = true
		view.Providers = append(view.Providers, BudgetProvider{
			Provider: id,
			Label:    label,
			Calls:    countByProvider[id],
			Limit:    limit,
			Active:   id == activeID,
		})
		view.TotalCalls += countByProvider[id]
		if counted {
			view.TotalLimit += limit
			view.ProviderCount++
		}
	}
	for _, p := range registry.Providers {
		if p.Custom && p.ID != activeID {
			continue
		}
		add(p.ID, p.Name, true)
	}
	if activeID == "" && cfg.Endpoint != "" {
		if countByProvider[""] == 0 {
			add("legacy", "Manual tutor configuration", true)
		} else {
			// Usage rows are rendered as unattributed below; the legacy
			// allowance is still granted by ReserveTutorCall, so it counts in
			// the totals without a second synthetic zero-usage row.
			view.TotalLimit += limit
			view.ProviderCount++
		}
	}
	for id, calls := range countByProvider {
		if !seen[id] {
			label := id
			if id == "" {
				label = "unattributed (legacy)"
			}
			seen[id] = true
			view.Providers = append(view.Providers, BudgetProvider{
				Provider: id,
				Label:    label,
				Calls:    calls,
				Limit:    limit,
				Active:   id == activeID,
			})
			view.TotalCalls += calls
		}
	}
	sort.SliceStable(view.Providers, func(i, j int) bool {
		if view.Providers[i].Active != view.Providers[j].Active {
			return view.Providers[i].Active
		}
		if view.Providers[i].Provider == "" {
			return false
		}
		if view.Providers[j].Provider == "" {
			return true
		}
		return false
	})
	view.Ceiling = fmt.Sprintf("%d providers × %d = %d", view.ProviderCount, limit, view.TotalLimit)
	return view
}
