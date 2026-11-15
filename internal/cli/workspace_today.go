package cli

import (
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/adaptive"
	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/search"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/studio"
)

// todayLimitations are the statements the Today view carries with its
// numbers.
var todayLimitations = []string{
	"Skill evidence is inferred from the local attempts shown; it is not an exam score, pass estimate or readiness claim.",
	"The exam blueprint, time limit and pass threshold are unknown; the exam date and daily minutes are your own display-only settings.",
	"The recommendation spends the unseen-assessment unit it names, once, as `passquest next` does (Q-0009-06); while an assessment run is active nothing is spent, so opening Today cannot expose a pending run item.",
}

// Today assembles the opening view. It reads the same evidence `next`
// reads and records the recommended unit's presentation at most once per
// unit, so rendering Today again does not spend more of the pool.
func (w *Workspace) Today() (studio.Today, error) {
	var today studio.Today
	err := w.withStore(func(st *store.Store) error {
		now := w.opt.Now()
		today = studio.Today{
			GeneratedAt: formatTime(now),
			Skills:      []studio.SkillEvidence{},
			Limitations: todayLimitations,
		}
		if _, err := w.reconcileAssessments(st, func(string, int) {}); err != nil {
			return err
		}
		if err := w.todayRecommendation(st, &today, now); err != nil {
			return err
		}
		pool, identities, err := assessmentPool(st)
		if err != nil {
			return err
		}
		if err := convertJSON(poolDocOf(pool, identities), &today.Pool); err != nil {
			return err
		}
		if err := todayRuns(st, &today); err != nil {
			return err
		}
		if today.Anki, err = ankiCounts(st); err != nil {
			return err
		}
		if today.Brief, err = briefSummary(st); err != nil {
			return err
		}
		if today.Find, err = w.findStatus(st, now); err != nil {
			return err
		}
		if today.Model, err = w.modelStatus(st); err != nil {
			return err
		}
		today.Prep, err = prepStatus(st, now)
		return err
	})
	return today, err
}

func (w *Workspace) todayRecommendation(st *store.Store, today *studio.Today, now time.Time) error {
	input, unclassified, _, err := buildAdaptiveInput(st, "")
	if err != nil {
		return err
	}
	result := adaptive.Recommend(input)
	// The window renders Today on boot and on leaving a run without the
	// learner asking for `next`; a spend then could name a pending item of the
	// active run and mark it prior exposure. No spend while a run is active.
	_, running, err := st.ActiveAssessmentRun()
	if err != nil {
		return err
	}
	if !running {
		if err := recordNextPresentation(st, result.Recommendation, now); err != nil {
			return err
		}
	}
	shell, err := detectShell(w.getenv, runtime.GOOS)
	if err != nil {
		shell, _ = detectShell(func(string) string { return "" }, runtime.GOOS)
	}
	doc := nextDocOf(result, unclassified, "", 0, nextDatabaseArgument(w.dbPath, w.opt), commandQuoter{shell: shell})
	if err := convertJSON(doc.Skills, &today.Skills); err != nil {
		return err
	}
	today.Unclassified = len(unclassified)
	if result.Recommendation == nil {
		return nil
	}
	rec := doc.Recommendation
	primary, followUp := nextTargetsOf(result.Recommendation)
	today.Recommendation = &studio.Recommendation{
		Action:             rec.Action,
		State:              rec.State,
		Reason:             rec.Reason,
		Skill:              studio.SkillRef{ID: rec.Skill.ID, Kind: rec.Skill.Kind, Title: rec.Skill.Title},
		Target:             studioTarget(primary),
		Command:            rec.Command,
		FollowUpCommand:    rec.FollowUpCommand,
		EvidenceAttemptIDs: rec.EvidenceAttemptIDs,
	}
	if followUp != nil {
		target := studioTarget(followUp)
		today.Recommendation.FollowUp = &target
	}
	return nil
}

func studioTarget(t *nextTarget) studio.Target {
	if t == nil {
		return studio.Target{}
	}
	target := studio.Target{Kind: t.kind, Skill: t.skillID}
	if t.kind == nextTargetTactic {
		target.Stage = stageGuided
	}
	if t.material != nil {
		target.SourceID = t.material.SourceID
		target.ContentID = t.material.ContentID
		if t.includeQuestion {
			target.QuestionID = t.material.QuestionID
		}
	}
	return target
}

func todayRuns(st *store.Store, today *studio.Today) error {
	runs, err := st.AssessmentRuns()
	if err != nil {
		return err
	}
	for i := len(runs) - 1; i >= 0; i-- {
		view := studioRun(runs[i])
		if runs[i].Status == "active" && today.ActiveRun == nil {
			today.ActiveRun = &view
			continue
		}
		if today.LastRun == nil && runs[i].Status != "active" {
			today.LastRun = &view
		}
	}
	return nil
}

func ankiCounts(st *store.Store) (studio.AnkiCounts, error) {
	notes, err := st.Notes()
	if err != nil {
		return studio.AnkiCounts{}, err
	}
	var counts studio.AnkiCounts
	for _, n := range notes {
		counts.Total++
		switch n.AnkiState {
		case store.AnkiStateLocal:
			counts.Local++
		case store.AnkiStateQueued:
			counts.Queued++
		case store.AnkiStatePushed:
			counts.Pushed++
		case store.AnkiStateAlreadyExists:
			counts.AlreadyPresent++
		case store.AnkiStateConflict:
			counts.Conflict++
		case store.AnkiStateFailed:
			counts.Failed++
		case store.AnkiStateDeckMismatch:
			counts.DeckMismatch++
		}
	}
	if _, found, err := st.LatestOpenNoteDraft(); err != nil {
		return studio.AnkiCounts{}, err
	} else if found {
		counts.OpenDrafts = 1
	}
	return counts, nil
}

// briefSummary reports the latest unfinished study brief from the local
// trail, without spawning the durable host.
func briefSummary(st *store.Store) (*studio.BriefSummary, error) {
	run, found, err := st.LatestAgentRun(true)
	if err != nil || !found {
		return nil, err
	}
	latest, _, err := st.LatestAgentRunEvent(run.RunID)
	if err != nil {
		return nil, err
	}
	steps, err := st.AgentSteps(run.RunID)
	if err != nil {
		return nil, err
	}
	return &studio.BriefSummary{
		RunID:       run.RunID,
		Task:        run.TaskKind,
		State:       string(agentAppState(latest.Kind)),
		Step:        len(steps),
		MaxSteps:    run.MaxSteps,
		Provider:    run.Provider,
		Model:       run.Model,
		CreatedAt:   formatTime(run.CreatedAt),
		LatestEvent: latest.Kind,
	}, nil
}

func (w *Workspace) findStatus(st *store.Store, now time.Time) (studio.FindStatus, error) {
	day := now.UTC().Format("2006-01-02")
	usage, err := st.FindQueryUsage(day)
	if err != nil {
		return studio.FindStatus{}, err
	}
	return studio.FindStatus{
		Configured:   strings.TrimSpace(w.getenv(search.EndpointEnv)) != "",
		EndpointEnv:  search.EndpointEnv,
		Day:          day,
		QueriesUsed:  usage.Used(),
		QueriesLimit: findDefaultMaxQueries,
	}, nil
}

// modelStatus is the active model, its text.chat state, authorization and the
// per-provider budget, read the way `model list` reads them.
func (w *Workspace) modelStatus(st *store.Store) (studio.ModelStatus, error) {
	cfg, configured, err := configuredTutor(st)
	if err != nil {
		return studio.ModelStatus{}, err
	}
	registry, _, err := provider.Load(w.getenv)
	if err != nil {
		return studio.ModelStatus{Problem: err.Error(), Budget: studio.Budget{Providers: []studio.BudgetProvider{}}}, nil
	}
	activeID := strings.TrimSpace(cfg.Provider)
	status := studio.ModelStatus{
		Configured: configured && activeID != "",
		Legacy:     configured && activeID == "",
		Provider:   activeID,
		Model:      cfg.Model,
	}
	if !configured {
		status.Model = ""
	}
	_, model, refreshed, _, err := activeProvider(cfg, registry)
	if err != nil {
		status.Problem = err.Error()
	} else {
		cfg = refreshed
	}
	_, present := credentialState(w.opt, activeID)
	status.Auth = authState(present)
	switch {
	case status.Legacy:
		status.TextChatState = "legacy"
	case status.Configured && err == nil:
		index, err := loadVerificationIndex(st)
		if err != nil {
			return studio.ModelStatus{}, err
		}
		view := capabilityView(provider.CapTextChat, model.Has(provider.CapTextChat), index.forModel(activeID, cfg.Model))
		status.TextChatState = view.State
	default:
		status.TextChatState = "none"
	}
	budget, err := buildBudgetView(st, cfg, registry, activeID, w.opt)
	if err != nil {
		return studio.ModelStatus{}, err
	}
	if err := convertJSON(budget, &status.Budget); err != nil {
		return studio.ModelStatus{}, err
	}
	if status.Budget.Providers == nil {
		status.Budget.Providers = []studio.BudgetProvider{}
	}
	return status, nil
}

// prepStatus derives the prep day from the first recorded attempt and reads
// the optional display-only settings.
func prepStatus(st *store.Store, now time.Time) (studio.PrepStatus, error) {
	prep := studio.PrepStatus{PassThreshold: studio.PassThresholdUnknown}
	first, found, err := st.FirstAttemptAt()
	if err != nil {
		return prep, err
	}
	if found {
		prep.FirstAttemptAt = formatTime(first)
		startDay := time.Date(first.UTC().Year(), first.UTC().Month(), first.UTC().Day(), 0, 0, 0, 0, time.UTC)
		today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
		prep.Day = int(today.Sub(startDay).Hours()/24) + 1
		if prep.Day < 1 {
			prep.Day = 1
		}
	}
	if value, ok, err := st.LearnerSetting(store.LearnerSettingExamDate); err != nil {
		return prep, err
	} else if ok {
		prep.ExamDate = value
	}
	if value, ok, err := st.LearnerSetting(store.LearnerSettingDailyMinutes); err != nil {
		return prep, err
	} else if ok {
		prep.DailyMinutes, _ = strconv.Atoi(value)
	}
	return prep, nil
}

// SetLearnerSettings stores the optional exam date (YYYY-MM-DD) and daily
// minutes (1..1440). An empty date or zero minutes clears the value. Neither
// is an input to selection, scoring or assessment (Q-0011-06).
func (w *Workspace) SetLearnerSettings(settings studio.LearnerSettings) error {
	date := strings.TrimSpace(settings.ExamDate)
	if date != "" {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return &studio.Error{Code: "invalid_input", Message: "exam date must be YYYY-MM-DD"}
		}
	}
	if settings.DailyMinutes < 0 || settings.DailyMinutes > 1440 {
		return &studio.Error{Code: "invalid_input", Message: "daily minutes must be between 0 and 1440"}
	}
	minutes := ""
	if settings.DailyMinutes > 0 {
		minutes = strconv.Itoa(settings.DailyMinutes)
	}
	return w.withStore(func(st *store.Store) error {
		now := w.opt.Now()
		if err := st.SetLearnerSetting(store.LearnerSettingExamDate, date, now); err != nil {
			return err
		}
		return st.SetLearnerSetting(store.LearnerSettingDailyMinutes, minutes, now)
	})
}
