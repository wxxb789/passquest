package workspace

import (
	"context"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/adaptive"
	"github.com/wxxb789/passquest/internal/cmdline"
	"github.com/wxxb789/passquest/internal/find"
	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/search"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/studio"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/assessment"
	"github.com/wxxb789/passquest/internal/study/brief"
	"github.com/wxxb789/passquest/internal/study/models"
	"github.com/wxxb789/passquest/internal/study/practice"
	"github.com/wxxb789/passquest/internal/study/recommend"
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
			GeneratedAt: source.FormatTime(now),
			Skills:      []studio.SkillEvidence{},
			Limitations: todayLimitations,
		}
		if _, err := w.reconcileAssessments(st, func(string, int) {}); err != nil {
			return err
		}
		if err := w.todayRecommendation(st, &today, now); err != nil {
			return err
		}
		// The render already holds this call's store (withStore); the unit
		// wraps it, so the pool and run reads do not open a second one.
		svc := study.Wrap(st, w.ports)
		reported, err := assessment.Report(context.Background(), svc, assessment.ReportQuery{Query: assessment.QueryPool})
		if err != nil {
			return err
		}
		today.Pool = studioPoolOf(reported.Pool, reported.Identities)
		if err := todayRuns(svc, &today); err != nil {
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
	// The render already holds this call's store (withStore); the unit wraps
	// it, so the recommendation read does not open a second one. The unit is
	// not closed: the store belongs to withStore.
	svc := study.Wrap(st, w.ports)
	result, err := recommend.Recommend(context.Background(), svc, "", recommend.SpendNone)
	if err != nil {
		return err
	}
	// The window renders Today on boot and on leaving a run without the
	// learner asking for `next`; a spend then could name a pending item of the
	// active run and mark it prior exposure. No spend while a run is active.
	_, running, err := st.ActiveAssessmentRun()
	if err != nil {
		return err
	}
	if !running {
		// The spend uses the clock read this render opened with: a fresh port
		// read would be a read the render did not make.
		if err := recommend.RecordPresentation(context.Background(), svc, result.Adaptive.Recommendation, now); err != nil {
			return err
		}
	}
	shell, err := cmdline.DetectShell(w.getenv, runtime.GOOS)
	if err != nil {
		shell, _ = cmdline.DetectShell(func(string) string { return "" }, runtime.GOOS)
	}
	today.Skills = studioSkillEvidenceOf(result.Adaptive.Skills)
	today.Unclassified = len(result.Unclassified)
	if result.Adaptive.Recommendation == nil {
		return nil
	}
	rec := result.Adaptive.Recommendation
	primary, followUp := recommend.Targets(result.Adaptive.Recommendation)
	command, followUpCommand := cmdline.NextCommands(rec, cmdline.NextDatabaseArgument(w.dbPath, study.DefaultDatabasePath), cmdline.CommandQuoter{Shell: shell})
	today.Recommendation = &studio.Recommendation{
		Action:             string(rec.Action),
		State:              string(rec.State),
		Reason:             rec.Reason,
		Skill:              studio.SkillRef{ID: rec.Skill.ID, Kind: rec.Skill.Kind, Title: rec.Skill.Title},
		Target:             studioTarget(primary),
		Command:            command,
		FollowUpCommand:    followUpCommand,
		EvidenceAttemptIDs: append([]int64{}, rec.EvidenceAttemptIDs...),
	}
	if followUp != nil {
		target := studioTarget(followUp)
		today.Recommendation.FollowUp = &target
	}
	return nil
}

func studioTarget(t *recommend.Target) studio.Target {
	if t == nil {
		return studio.Target{}
	}
	target := studio.Target{Kind: t.Kind, Skill: t.SkillID}
	if t.Kind == recommend.TargetTactic {
		target.Stage = practice.StageGuided
	}
	if t.Material != nil {
		target.SourceID = t.Material.SourceID
		target.ContentID = t.Material.ContentID
		if t.IncludeQuestion {
			target.QuestionID = t.Material.QuestionID
		}
	}
	return target
}

func todayRuns(svc *study.Service, today *studio.Today) error {
	reported, err := assessment.Report(context.Background(), svc, assessment.ReportQuery{Query: assessment.QueryRuns})
	if err != nil {
		return err
	}
	for i := len(reported.Runs) - 1; i >= 0; i-- {
		view := studioRun(reported.Runs[i])
		if reported.Runs[i].Status == "active" && today.ActiveRun == nil {
			today.ActiveRun = &view
			continue
		}
		if today.LastRun == nil && reported.Runs[i].Status != "active" {
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
		State:       string(brief.AppState(latest.Kind)),
		Step:        len(steps),
		MaxSteps:    run.MaxSteps,
		Provider:    run.Provider,
		Model:       run.Model,
		CreatedAt:   source.FormatTime(run.CreatedAt),
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
		QueriesLimit: find.DefaultMaxQueries,
	}, nil
}

// modelStatus is the active model, its text.chat state, authorization and the
// per-provider budget, read the way `model list` reads them.
func (w *Workspace) modelStatus(st *store.Store) (studio.ModelStatus, error) {
	// The render already holds this call's store (withStore); the unit wraps
	// it, so the catalog read does not open a second one. The unit is not
	// closed: the store belongs to withStore.
	svc := study.Wrap(st, w.ports)
	listing, err := models.Status(context.Background(), svc)
	if err != nil {
		return studio.ModelStatus{}, err
	}
	if listing.RegistryErr != nil {
		return studio.ModelStatus{Problem: listing.RegistryErr.Error(), Budget: studio.Budget{Providers: []studio.BudgetProvider{}}}, nil
	}
	activeID := strings.TrimSpace(listing.Active.Provider)
	status := studio.ModelStatus{
		Configured: listing.Configured && activeID != "",
		Legacy:     listing.Configured && activeID == "",
		Provider:   activeID,
		Model:      listing.Active.Model,
	}
	if !listing.Configured {
		status.Model = ""
	}
	if listing.ActiveErr != nil {
		status.Problem = listing.ActiveErr.Error()
	}
	_, present := models.CredentialState(w.opt.Getenv, activeID)
	status.Auth = models.AuthState(present)
	switch {
	case status.Legacy:
		status.TextChatState = "legacy"
	case status.Configured && listing.ActiveErr == nil:
		state := models.CapabilityStateOf(provider.CapTextChat,
			listing.Model.Has(provider.CapTextChat), listing.Index.ForModel(activeID, listing.Active.Model))
		status.TextChatState = state.State
	default:
		status.TextChatState = "none"
	}
	status.Budget = studioBudgetOf(listing.Budget)
	return status, nil
}

// studioBudgetOf renders the catalog's budget as the window's value. The
// provider list is always present, empty when there is none.
func studioBudgetOf(budget models.Budget) studio.Budget {
	view := studio.Budget{
		PerProviderLimit: budget.PerProviderLimit,
		Providers:        make([]studio.BudgetProvider, 0, len(budget.Providers)),
		TotalCalls:       budget.TotalCalls,
		TotalLimit:       budget.TotalLimit,
		ProviderCount:    budget.ProviderCount,
		Ceiling:          budget.Ceiling,
	}
	for _, p := range budget.Providers {
		view.Providers = append(view.Providers, studio.BudgetProvider{
			Provider: p.Provider,
			Label:    p.Label,
			Calls:    p.Calls,
			Limit:    p.Limit,
			Active:   p.Active,
		})
	}
	return view
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
		prep.FirstAttemptAt = source.FormatTime(first)
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

// studioSkillEvidenceOf maps the recommendation's per-skill evidence onto the
// window's value: the same fields the evidence document carried, always a
// list.
func studioSkillEvidenceOf(skills []adaptive.SkillResult) []studio.SkillEvidence {
	views := make([]studio.SkillEvidence, 0, len(skills))
	for _, candidate := range skills {
		views = append(views, studio.SkillEvidence{
			Skill:     studio.SkillRef{ID: candidate.Skill.ID, Kind: candidate.Skill.Kind, Title: candidate.Skill.Title},
			Evidence:  studioEvidenceOf(candidate.Evidence),
			Inference: studioInferenceOf(candidate.Inference),
		})
	}
	return views
}

func studioEvidenceOf(ev adaptive.Evidence) studio.Evidence {
	return studio.Evidence{
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

func studioInferenceOf(inference adaptive.Inference) studio.Inference {
	return studio.Inference{
		State:           string(inference.State),
		Action:          string(inference.Action),
		Reason:          inference.Reason,
		CausalDiagnosis: inference.CausalDiagnosis,
		Uncertainty:     append([]string(nil), inference.Uncertainty...),
	}
}
