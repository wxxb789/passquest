package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/tutor"
)

func runModel(args []string, opt Options) int {
	if code := guardAssessmentCommand(args, opt, "model"); code != 0 {
		return code
	}
	if len(args) == 0 {
		return usageError(opt, jsonFlagRequested(args), "invalid_input",
			"model requires list, use, test or budget")
	}
	switch args[0] {
	case "list":
		return runModelList(args[1:], opt)
	case "use":
		return runModelUse(args[1:], opt)
	case "test":
		return runModelTest(args[1:], opt)
	case "budget":
		return runModelBudget(args[1:], opt)
	default:
		return usageError(opt, jsonFlagRequested(args), "invalid_input", fmt.Sprintf("unknown model command %q", args[0]))
	}
}

func registryOrError(opt Options, jsonMode bool) (provider.Registry, string, int) {
	registry, source, err := provider.Load(opt.Getenv)
	if err != nil {
		return provider.Registry{}, "", usageError(opt, jsonMode, "invalid_registry", err.Error())
	}
	return registry, source, 0
}

func configuredTutor(st *store.Store) (tutor.Config, bool, error) {
	cfg, configured, err := st.TutorConfig()
	if err != nil {
		return tutor.Config{}, false, err
	}
	if !configured {
		cfg = tutor.DefaultConfig()
	}
	return cfg, configured, nil
}

// providerConfig resolves one registry model into the transport configuration
// the existing tutor/agent paths already speak. A custom provider needs the
// learner's endpoint; a registry provider's endpoint is authoritative.
func providerConfig(base tutor.Config, registry provider.Registry, providerID, modelID, endpoint string) (tutor.Config, error) {
	p, m, ok := registry.Find(providerID, modelID)
	if !ok {
		return tutor.Config{}, fmt.Errorf("provider/model %s/%s is not in the registry", providerID, modelID)
	}
	if p.Custom {
		if strings.TrimSpace(endpoint) == "" && strings.TrimSpace(base.Provider) == p.ID {
			endpoint = base.Endpoint
		}
		if strings.TrimSpace(endpoint) == "" {
			return tutor.Config{}, errors.New("a custom provider requires --endpoint URL")
		}
		if err := provider.ValidateEndpoint(endpoint); err != nil {
			return tutor.Config{}, err
		}
	} else {
		if strings.TrimSpace(endpoint) != "" {
			return tutor.Config{}, errors.New("--endpoint is only accepted for the custom provider")
		}
		endpoint = p.Endpoint
	}
	cfg := base
	cfg.Provider = p.ID
	cfg.Model = m.ID
	cfg.Endpoint = strings.TrimSpace(endpoint)
	if err := cfg.Validate(); err != nil {
		return tutor.Config{}, err
	}
	return cfg, nil
}

// activeProvider refreshes the endpoint from the registry so a data edit drives
// behaviour on the next call. The active model id stays the stored setting. The
// returned configuration is the one every caller should use.
func activeProvider(cfg tutor.Config, registry provider.Registry) (provider.Provider, provider.Model, tutor.Config, bool, error) {
	providerID := strings.TrimSpace(cfg.Provider)
	if providerID == "" {
		return provider.Provider{}, provider.Model{}, cfg, false, nil
	}
	p, m, ok := registry.Find(providerID, cfg.Model)
	if !ok {
		return provider.Provider{}, provider.Model{}, cfg, true,
			fmt.Errorf("active model %s/%s is no longer in the provider registry", providerID, cfg.Model)
	}
	if !p.Custom {
		cfg.Endpoint = p.Endpoint
	}
	return p, m, cfg, true, nil
}

// resolveActive loads the registry, resolves the active registry provider and
// refreshes the endpoint from registry data. It refuses a pair that does not
// declare text.chat, which every text surface requires. A legacy configuration
// (no provider) passes through with active=false.
func resolveActive(cfg tutor.Config, opt Options, jsonMode bool) (tutor.Config, provider.Registry, bool, int) {
	registry, _, err := provider.Load(opt.Getenv)
	if err != nil {
		return cfg, provider.Registry{}, false, usageError(opt, jsonMode, "invalid_registry", err.Error())
	}
	providerID := strings.TrimSpace(cfg.Provider)
	if providerID == "" {
		return cfg, registry, false, 0
	}
	_, model, refreshed, _, err := activeProvider(cfg, registry)
	if err != nil {
		return cfg, registry, false, usageError(opt, jsonMode, "unknown_model", err.Error())
	}
	if !model.Has(provider.CapTextChat) {
		return cfg, registry, false, usageError(opt, jsonMode, "capability_not_declared",
			fmt.Sprintf("%s/%s does not declare text.chat", providerID, cfg.Model))
	}
	return refreshed, registry, true, 0
}

// requireVerifiedTextChat refuses a registry pair whose text.chat has no
// verified evidence. The legacy hand-configured path needs no verification.
func requireVerifiedTextChat(st *store.Store, registry provider.Registry, opt Options, providerID, modelID string, jsonMode bool) int {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return 0
	}
	_, model, ok := registry.Find(providerID, modelID)
	if !ok {
		return usageError(opt, jsonMode, "unknown_model",
			fmt.Sprintf("%s/%s is no longer in the provider registry", providerID, modelID))
	}
	if !model.Has(provider.CapTextChat) {
		return usageError(opt, jsonMode, "capability_not_declared",
			fmt.Sprintf("%s/%s does not declare text.chat", providerID, modelID))
	}
	verification, found, err := st.LatestProviderVerification(providerID, modelID, string(provider.CapTextChat))
	if err != nil {
		return internalError(opt, jsonMode, err)
	}
	if !found || verification.Status != "verified" {
		detail := "text.chat is declared but unverified"
		if found && verification.Status == "failed" {
			detail = "the last text.chat test failed: " + verification.FailureType
		}
		return usageError(opt, jsonMode, "capability_unverified",
			fmt.Sprintf("%s/%s: %s; run passquest model test %s/%s before asking",
				providerID, modelID, detail, providerID, modelID))
	}
	return 0
}

func credentialState(opt Options, providerID string) (string, bool) {
	env, _, present := provider.CredentialValue(opt.Getenv, providerID)
	return env, present
}

// verificationIndex groups the latest verification per provider, model and
// capability. A model's evidence is never applied to a sibling.
type verificationIndex map[modelRef]map[provider.Capability]store.ProviderVerification

type modelRef struct {
	provider string
	model    string
}

func loadVerificationIndex(st *store.Store) (verificationIndex, error) {
	all, err := st.ProviderVerifications()
	if err != nil {
		return nil, err
	}
	index := verificationIndex{}
	for _, event := range all {
		ref := modelRef{provider: event.Provider, model: event.Model}
		if index[ref] == nil {
			index[ref] = map[provider.Capability]store.ProviderVerification{}
		}
		index[ref][provider.Capability(event.Capability)] = event
	}
	return index, nil
}

func (index verificationIndex) forModel(providerID, modelID string) map[provider.Capability]store.ProviderVerification {
	return index[modelRef{provider: providerID, model: modelID}]
}

func capabilityView(capability provider.Capability, declared bool, verifications map[provider.Capability]store.ProviderVerification) modelCapabilityView {
	view := modelCapabilityView{ID: string(capability), Declared: declared, State: "unsupported"}
	if !declared {
		return view
	}
	view.State = "declared"
	if event, ok := verifications[capability]; ok {
		view.CheckedAt = event.CheckedAt.UTC().Format(time.RFC3339)
		switch event.Status {
		case "verified":
			view.State = "verified"
			view.Verified = true
		case "failed":
			view.State = "failed"
			view.FailureType = event.FailureType
			view.Action = event.Action
		}
	}
	return view
}

func modelViewsOf(providerID string, p provider.Provider, active bool, activeModel string, index verificationIndex) []modelView {
	models := make([]modelView, 0, len(p.Models))
	for _, m := range p.Models {
		item := modelView{
			ID:      m.ID,
			Label:   m.Label,
			Default: m.Default,
			Active:  active && activeModel == m.ID,
		}
		verifications := index.forModel(providerID, m.ID)
		for _, c := range provider.Capabilities {
			item.Capabilities = append(item.Capabilities, capabilityView(c, m.Has(c), verifications))
		}
		models = append(models, item)
	}
	return models
}

func runModelList(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	fs := newFlagSet("model list", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	registry, source, code := registryOrError(opt, *jsonFlag)
	if code != 0 {
		return code
	}
	cfg, configured, err := configuredTutor(st)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	index, err := loadVerificationIndex(st)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}

	activeID := strings.TrimSpace(cfg.Provider)
	_, _, refreshed, _, err := activeProvider(cfg, registry)
	if err != nil {
		return usageError(opt, *jsonFlag, "unknown_model", err.Error())
	}
	cfg = refreshed
	doc := modelListDoc{
		SchemaVersion:   ProtocolVersion,
		State:           "models",
		RegistryVersion: registry.Version,
		RegistrySource:  source,
		Active: modelActiveView{
			Configured: configured && activeID != "",
			Provider:   activeID,
			Model:      cfg.Model,
			Endpoint:   cfg.Endpoint,
		},
		Limits: modelLimits,
	}
	for _, p := range registry.Providers {
		env, present := credentialState(opt, p.ID)
		item := modelProviderView{
			ID:            p.ID,
			Name:          p.Name,
			EndpointShape: p.EndpointShape,
			CredentialEnv: env,
			Auth:          authState(present),
			Custom:        p.Custom,
			Active:        activeID == p.ID,
		}
		models := modelViewsOf(p.ID, p, activeID == p.ID, cfg.Model, index)
		if p.Custom && activeID == p.ID {
			custom := provider.Model{ID: cfg.Model, Capabilities: []provider.Capability{provider.CapTextChat}}
			models = modelViewsOf(p.ID, provider.Provider{Models: []provider.Model{custom}}, true, cfg.Model, index)
		}
		item.Models = models
		doc.Providers = append(doc.Providers, item)
	}
	if configured && activeID == "" {
		env, present := credentialState(opt, "")
		doc.Active.Legacy = true
		doc.Providers = append(doc.Providers, modelProviderView{
			ID:            "legacy",
			Name:          "Manual tutor configuration",
			EndpointShape: provider.EndpointShape,
			CredentialEnv: env,
			Auth:          authState(present),
			Active:        true,
			Models: []modelView{{
				ID:     cfg.Model,
				Label:  "hand-configured endpoint",
				Active: true,
				Capabilities: []modelCapabilityView{
					{ID: string(provider.CapTextChat), Declared: true, State: "legacy"},
				},
			}},
		})
	}
	doc.Budget, err = buildBudgetView(st, cfg, registry, activeID, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeModelList(opt.Stdout, doc)
	return 0
}

type modelTarget struct {
	st         *store.Store
	registry   provider.Registry
	cfg        tutor.Config
	providerID string
	modelID    string
	jsonMode   bool
}

// resolveModelTarget parses <provider>/<model> and resolves it against the
// registry and the stored configuration. It opens the database; the caller
// closes it.
func resolveModelTarget(command string, args []string, opt Options, wantJSON bool) (modelTarget, int) {
	args = reorderArgs(args, map[string]bool{"endpoint": true, "db": true})
	fs := newFlagSet(command, opt, wantJSON)
	endpointFlag := fs.String("endpoint", "", "custom provider endpoint ending in /chat/completions")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 1, command+" requires <provider>/<model>"); code != 0 {
		return modelTarget{}, code
	}
	providerID, modelID, err := provider.ParseRef(fs.Arg(0))
	if err != nil {
		return modelTarget{}, usageError(opt, *jsonFlag, "invalid_input", err.Error())
	}
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return modelTarget{}, internalError(opt, *jsonFlag, err)
	}
	registry, _, code := registryOrError(opt, *jsonFlag)
	if code != 0 {
		st.Close()
		return modelTarget{}, code
	}
	base, _, err := configuredTutor(st)
	if err != nil {
		st.Close()
		return modelTarget{}, internalError(opt, *jsonFlag, err)
	}
	cfg, err := providerConfig(base, registry, providerID, modelID, *endpointFlag)
	if err != nil {
		st.Close()
		return modelTarget{}, usageError(opt, *jsonFlag, "invalid_model", err.Error())
	}
	return modelTarget{
		st:         st,
		registry:   registry,
		cfg:        cfg,
		providerID: providerID,
		modelID:    modelID,
		jsonMode:   *jsonFlag,
	}, 0
}

func runModelUse(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	target, code := resolveModelTarget("model use", args, opt, wantJSON)
	if code != 0 {
		return code
	}
	defer target.st.Close()
	if err := target.st.SaveTutorConfig(target.cfg, opt.Now()); err != nil {
		return internalError(opt, target.jsonMode, err)
	}
	env, present := credentialState(opt, target.providerID)
	index, err := loadVerificationIndex(target.st)
	if err != nil {
		return internalError(opt, target.jsonMode, err)
	}
	p, m, _ := target.registry.Find(target.providerID, target.modelID)
	capView := capabilityView(provider.CapTextChat, m.Has(provider.CapTextChat), index.forModel(target.providerID, target.modelID))
	doc := modelUseDoc{
		SchemaVersion: ProtocolVersion,
		State:         "selected",
		Active: modelActiveView{
			Configured: true,
			Provider:   target.cfg.Provider,
			Model:      target.cfg.Model,
			Endpoint:   target.cfg.Endpoint,
		},
		ProviderName:      p.Name,
		CredentialEnv:     env,
		CredentialPresent: present,
		TextChat:          capView,
		Auth:              authState(present),
		Limits:            modelLimits,
	}
	if target.jsonMode {
		return writeJSON(opt, doc)
	}
	writeModelUse(opt.Stdout, doc)
	return 0
}

// finishModelTestFailure appends one failed verification and writes the
// document. The missing-credential path passes callMade=false and reservation
// 0 because it never reaches the host.
func finishModelTestFailure(opt Options, jsonMode bool, st *store.Store, doc modelTestDoc, failure provider.Failure,
	at time.Time, callMade bool, reservation int64, code int) int {
	if _, err := appendVerification(st, doc.Provider, doc.Model, failure, at, doc.LatencyMS, doc.HTTPStatus, callMade); err != nil {
		return internalError(opt, jsonMode, err)
	}
	if reservation != 0 {
		_ = st.ReleaseTutorCallReservation(reservation)
	}
	doc.Failure = &modelFailureView{Type: failure.Type, Message: failure.Message, Action: failure.Action}
	doc.Result = "failed:" + failure.Type
	doc.CheckedAt = at.UTC().Format(time.RFC3339)
	if jsonMode {
		_ = writeJSONTo(opt.Stdout, doc)
	} else {
		writeModelTest(opt.Stdout, doc)
	}
	return code
}

func runModelTest(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	target, code := resolveModelTarget("model test", args, opt, wantJSON)
	if code != 0 {
		return code
	}
	defer target.st.Close()
	providerID, modelID := target.providerID, target.modelID
	cfg := target.cfg
	p, m, _ := target.registry.Find(providerID, modelID)
	if !m.Has(provider.CapTextChat) {
		return usageError(opt, target.jsonMode, "capability_not_declared",
			fmt.Sprintf("%s/%s does not declare text.chat", providerID, modelID))
	}
	env, key, present := provider.CredentialValue(opt.Getenv, providerID)
	now := opt.Now()
	doc := modelTestDoc{
		SchemaVersion: ProtocolVersion,
		State:         "failed",
		Provider:      providerID,
		ProviderName:  p.Name,
		Model:         modelID,
		Capability:    string(provider.CapTextChat),
		Endpoint:      cfg.Endpoint,
		CredentialEnv: env,
		Limits:        modelLimits,
	}
	if !present {
		failure := provider.Classify(provider.FailureNoCredential, 0, "")
		failure.Action = "export " + env + " (it is never stored)"
		return finishModelTestFailure(opt, target.jsonMode, target.st, doc, failure, now, false, 0, 2)
	}
	budgetNow := opt.Now()
	reservation, used, err := target.st.ReserveTutorCall(providerID, tutorDayStart(budgetNow), budgetNow, cfg.DailyCalls)
	if err != nil {
		return internalError(opt, target.jsonMode, err)
	}
	if reservation == 0 {
		return usageError(opt, target.jsonMode, "budget_exhausted",
			fmt.Sprintf("%s has used %d of %d provider calls today (UTC); raise the budget with passquest tutor config --daily-calls N",
				providerID, used, cfg.DailyCalls))
	}
	sendStarted := false
	defer func() {
		if !sendStarted {
			_ = target.st.ReleaseTutorCallReservation(reservation)
		}
	}()
	request, sha, bytes, err := tutor.ProbeRequest(cfg.Endpoint, cfg.Model, cfg.TimeoutMS)
	if err != nil {
		return usageError(opt, target.jsonMode, "invalid_model", err.Error())
	}
	request.CredentialEnv = env
	doc.RequestSHA256 = sha
	doc.RequestBytes = bytes
	sendStarted = true
	started := opt.Now()
	response, transportErr := opt.TutorTransport.Call(context.Background(), request)
	latency := opt.Now().Sub(started).Milliseconds()
	doc.LatencyMS = latency
	doc.HTTPStatus = response.HTTPStatus
	if transportErr != nil {
		failure := provider.ClassifyTransport(transportErr)
		failure.Message = tutorMessage(failure.Message, key)
		return finishModelTestFailure(opt, target.jsonMode, target.st, doc, failure, now, true, reservation, 3)
	}
	if response.Status == "error" {
		failure := provider.ClassifyResponse(response)
		failure.Message = tutorMessage(failure.Message, key)
		return finishModelTestFailure(opt, target.jsonMode, target.st, doc, failure, now, true, reservation, 3)
	}
	text := strings.TrimSpace(response.Text)
	if text == "" || !validTutorText(text) {
		failure := provider.Classify(provider.FailureInvalidResponse, response.HTTPStatus, "the capability probe returned no valid text")
		return finishModelTestFailure(opt, target.jsonMode, target.st, doc, failure, now, true, reservation, 3)
	}
	verified := store.ProviderVerification{
		Provider:   providerID,
		Model:      modelID,
		Capability: string(provider.CapTextChat),
		Status:     "verified",
		CallMade:   true,
		LatencyMS:  latency,
		HTTPStatus: response.HTTPStatus,
		CheckedAt:  now,
	}
	if _, err := target.st.AppendProviderVerification(verified); err != nil {
		return internalError(opt, target.jsonMode, err)
	}
	_ = target.st.ReleaseTutorCallReservation(reservation)
	doc.State = "verified"
	doc.Result = "verified"
	doc.CheckedAt = now.UTC().Format(time.RFC3339)
	if target.jsonMode {
		return writeJSON(opt, doc)
	}
	writeModelTest(opt.Stdout, doc)
	return 0
}

func appendVerification(st *store.Store, providerID, modelID string, failure provider.Failure, at time.Time, latency int64, status int, callMade bool) (store.ProviderVerification, error) {
	return st.AppendProviderVerification(store.ProviderVerification{
		Provider:    providerID,
		Model:       modelID,
		Capability:  string(provider.CapTextChat),
		Status:      "failed",
		FailureType: failure.Type,
		Action:      failure.Action,
		CallMade:    callMade,
		LatencyMS:   latency,
		HTTPStatus:  status,
		CheckedAt:   at,
	})
}

func validTutorText(text string) bool {
	return strings.TrimSpace(text) != "" && utf8.ValidString(text) && !strings.ContainsRune(text, 0) &&
		!tutor.ContainsUnsafeControl(text) && utf8.RuneCountInString(text) <= tutor.MaxResponseChars
}

func runModelBudget(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	fs := newFlagSet("model budget", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	registry, source, code := registryOrError(opt, *jsonFlag)
	if code != 0 {
		return code
	}
	cfg, _, err := configuredTutor(st)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	budget, err := buildBudgetView(st, cfg, registry, strings.TrimSpace(cfg.Provider), opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	doc := modelBudgetDoc{
		SchemaVersion:   ProtocolVersion,
		State:           "budget",
		RegistryVersion: registry.Version,
		RegistrySource:  source,
		Budget:          budget,
		Limits:          modelLimits,
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeModelBudget(opt.Stdout, doc)
	return 0
}

func buildBudgetView(st *store.Store, cfg tutor.Config, registry provider.Registry, activeID string, opt Options) (modelBudgetView, error) {
	limit := cfg.DailyCalls
	if limit <= 0 {
		limit = tutor.DefaultDailyCalls
	}
	counts, err := st.ProviderCallCountsSince(tutorDayStart(opt.Now()))
	if err != nil {
		return modelBudgetView{}, err
	}
	countByProvider := map[string]int{}
	for _, item := range counts {
		countByProvider[item.Provider] += item.Calls
	}
	seen := map[string]bool{}
	view := modelBudgetView{PerProviderLimit: limit}
	add := func(id, label string, counted bool) {
		if seen[id] {
			return
		}
		seen[id] = true
		view.Providers = append(view.Providers, modelBudgetProviderView{
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
			view.Providers = append(view.Providers, modelBudgetProviderView{
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
	return view, nil
}
