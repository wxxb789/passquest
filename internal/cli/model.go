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
// behaviour on the next call. The active model id stays the stored setting.
func activeProvider(cfg tutor.Config, registry provider.Registry) (provider.Provider, provider.Model, bool, error) {
	providerID := strings.TrimSpace(cfg.Provider)
	if providerID == "" {
		return provider.Provider{}, provider.Model{}, false, nil
	}
	p, m, ok := registry.Find(providerID, cfg.Model)
	if !ok {
		return provider.Provider{}, provider.Model{}, true,
			fmt.Errorf("active model %s/%s is no longer in the provider registry", providerID, cfg.Model)
	}
	if !p.Custom {
		cfg.Endpoint = p.Endpoint
	}
	return p, m, true, nil
}

func credentialState(opt Options, providerID string) (string, bool) {
	env, _, present := provider.CredentialValue(opt.Getenv, providerID)
	return env, present
}

func verificationState(st *store.Store, providerID, modelID string) (map[provider.Capability]store.ProviderVerification, error) {
	all, err := st.ProviderVerifications()
	if err != nil {
		return nil, err
	}
	out := make(map[provider.Capability]store.ProviderVerification)
	for _, event := range all {
		if event.Provider == providerID && event.Model == modelID {
			out[provider.Capability(event.Capability)] = event
		}
	}
	return out, nil
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

func modelViewsOf(p provider.Provider, active bool, activeModel string, verifications map[provider.Capability]store.ProviderVerification) []modelView {
	models := make([]modelView, 0, len(p.Models))
	for _, m := range p.Models {
		item := modelView{
			ID:      m.ID,
			Label:   m.Label,
			Default: m.Default,
			Active:  active && activeModel == m.ID,
		}
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
	byProvider := map[string]map[provider.Capability]store.ProviderVerification{}
	all, err := st.ProviderVerifications()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	for _, event := range all {
		if byProvider[event.Provider] == nil {
			byProvider[event.Provider] = map[provider.Capability]store.ProviderVerification{}
		}
		byProvider[event.Provider][provider.Capability(event.Capability)] = event
	}

	activeID := strings.TrimSpace(cfg.Provider)
	if _, _, ok, err := activeProvider(cfg, registry); err != nil {
		return usageError(opt, *jsonFlag, "unknown_model", err.Error())
	} else if ok {
		if p, _, _ := registry.Find(activeID, cfg.Model); !p.Custom {
			cfg.Endpoint = p.Endpoint
		}
	}
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
		auth := "no_credential"
		if present {
			auth = "present"
		}
		item := modelProviderView{
			ID:            p.ID,
			Name:          p.Name,
			EndpointShape: p.EndpointShape,
			CredentialEnv: env,
			Auth:          auth,
			Custom:        p.Custom,
			Active:        activeID == p.ID,
		}
		models := modelViewsOf(p, activeID == p.ID, cfg.Model, byProvider[p.ID])
		if p.Custom && activeID == p.ID {
			custom := provider.Model{ID: cfg.Model, Capabilities: []provider.Capability{provider.CapTextChat}}
			models = modelViewsOf(provider.Provider{Models: []provider.Model{custom}}, true, cfg.Model, byProvider[p.ID])
		}
		item.Models = models
		doc.Providers = append(doc.Providers, item)
	}
	if configured && activeID == "" {
		env, present := credentialState(opt, "")
		auth := "no_credential"
		if present {
			auth = "present"
		}
		doc.Active.Legacy = true
		doc.Providers = append(doc.Providers, modelProviderView{
			ID:            "legacy",
			Name:          "Manual tutor configuration",
			EndpointShape: provider.EndpointShape,
			CredentialEnv: env,
			Auth:          auth,
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

func runModelUse(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"endpoint": true, "db": true})
	fs := newFlagSet("model use", opt, wantJSON)
	endpointFlag := fs.String("endpoint", "", "custom provider endpoint ending in /chat/completions")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 1, "model use requires <provider>/<model>"); code != 0 {
		return code
	}
	providerID, modelID, err := provider.ParseRef(fs.Arg(0))
	if err != nil {
		return usageError(opt, *jsonFlag, "invalid_input", err.Error())
	}
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	registry, _, code := registryOrError(opt, *jsonFlag)
	if code != 0 {
		return code
	}
	base, _, err := configuredTutor(st)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	cfg, err := providerConfig(base, registry, providerID, modelID, *endpointFlag)
	if err != nil {
		return usageError(opt, *jsonFlag, "invalid_model", err.Error())
	}
	if err := st.SaveTutorConfig(cfg, opt.Now()); err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	env, present := credentialState(opt, providerID)
	auth := "no_credential"
	if present {
		auth = "present"
	}
	verifications, err := verificationState(st, providerID, modelID)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	p, m, _ := registry.Find(providerID, modelID)
	capView := capabilityView(provider.CapTextChat, m.Has(provider.CapTextChat), verifications)
	doc := modelUseDoc{
		SchemaVersion: ProtocolVersion,
		State:         "selected",
		Active: modelActiveView{
			Configured: true,
			Provider:   cfg.Provider,
			Model:      cfg.Model,
			Endpoint:   cfg.Endpoint,
		},
		ProviderName:      p.Name,
		CredentialEnv:     env,
		CredentialPresent: present,
		TextChat:          capView,
		Auth:              auth,
		Limits:            modelLimits,
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeModelUse(opt.Stdout, doc)
	return 0
}

func runModelTest(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"endpoint": true, "db": true})
	fs := newFlagSet("model test", opt, wantJSON)
	endpointFlag := fs.String("endpoint", "", "custom provider endpoint ending in /chat/completions")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 1, "model test requires <provider>/<model>"); code != 0 {
		return code
	}
	providerID, modelID, err := provider.ParseRef(fs.Arg(0))
	if err != nil {
		return usageError(opt, *jsonFlag, "invalid_input", err.Error())
	}
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	registry, _, code := registryOrError(opt, *jsonFlag)
	if code != 0 {
		return code
	}
	base, _, err := configuredTutor(st)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	cfg, err := providerConfig(base, registry, providerID, modelID, *endpointFlag)
	if err != nil {
		return usageError(opt, *jsonFlag, "invalid_model", err.Error())
	}
	p, m, _ := registry.Find(providerID, modelID)
	if !m.Has(provider.CapTextChat) {
		return usageError(opt, *jsonFlag, "capability_not_declared",
			fmt.Sprintf("%s/%s does not declare text.chat", providerID, modelID))
	}
	env, _ := credentialState(opt, providerID)
	_, key, present := provider.CredentialValue(opt.Getenv, providerID)
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
		if _, err := appendVerification(st, providerID, modelID, failure, now, 0, 0, false); err != nil {
			return internalError(opt, *jsonFlag, err)
		}
		doc.Failure = &modelFailureView{Type: failure.Type, Message: failure.Message, Action: failure.Action}
		doc.Result = "failed:" + failure.Type
		doc.CheckedAt = now.UTC().Format(time.RFC3339)
		if *jsonFlag {
			_ = writeJSONTo(opt.Stdout, doc)
		} else {
			writeModelTest(opt.Stdout, doc)
		}
		return 2
	}
	budgetNow := opt.Now()
	reservation, used, err := st.ReserveTutorCall(providerID, tutorDayStart(budgetNow), budgetNow, cfg.DailyCalls)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	if reservation == 0 {
		return usageError(opt, *jsonFlag, "budget_exhausted",
			fmt.Sprintf("%s has used %d of %d provider calls today (UTC); raise the budget with passquest tutor config --daily-calls N",
				providerID, used, cfg.DailyCalls))
	}
	sendStarted := false
	defer func() {
		if !sendStarted {
			_ = st.ReleaseTutorCallReservation(reservation)
		}
	}()
	request, sha, bytes, err := tutor.ProbeRequest(cfg.Endpoint, cfg.Model, cfg.TimeoutMS)
	if err != nil {
		return usageError(opt, *jsonFlag, "invalid_model", err.Error())
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
		if _, err := appendVerification(st, providerID, modelID, failure, now, latency, response.HTTPStatus, true); err != nil {
			return internalError(opt, *jsonFlag, err)
		}
		_ = st.ReleaseTutorCallReservation(reservation)
		doc.Failure = &modelFailureView{Type: failure.Type, Message: failure.Message, Action: failure.Action}
		doc.Result = "failed:" + failure.Type
		doc.CheckedAt = now.UTC().Format(time.RFC3339)
		if *jsonFlag {
			_ = writeJSONTo(opt.Stdout, doc)
		} else {
			writeModelTest(opt.Stdout, doc)
		}
		return 3
	}
	if response.Status == "error" {
		failure := provider.ClassifyResponse(response)
		failure.Message = tutorMessage(failure.Message, key)
		if _, err := appendVerification(st, providerID, modelID, failure, now, latency, response.HTTPStatus, true); err != nil {
			return internalError(opt, *jsonFlag, err)
		}
		_ = st.ReleaseTutorCallReservation(reservation)
		doc.Failure = &modelFailureView{Type: failure.Type, Message: failure.Message, Action: failure.Action}
		doc.Result = "failed:" + failure.Type
		doc.CheckedAt = now.UTC().Format(time.RFC3339)
		if *jsonFlag {
			_ = writeJSONTo(opt.Stdout, doc)
		} else {
			writeModelTest(opt.Stdout, doc)
		}
		return 3
	}
	text := strings.TrimSpace(response.Text)
	if text == "" || !validTutorText(text) {
		failure := provider.Classify(provider.FailureInvalidResponse, response.HTTPStatus, "the capability probe returned no valid text")
		if _, err := appendVerification(st, providerID, modelID, failure, now, latency, response.HTTPStatus, true); err != nil {
			return internalError(opt, *jsonFlag, err)
		}
		_ = st.ReleaseTutorCallReservation(reservation)
		doc.Failure = &modelFailureView{Type: failure.Type, Message: failure.Message, Action: failure.Action}
		doc.Result = "failed:" + failure.Type
		doc.CheckedAt = now.UTC().Format(time.RFC3339)
		if *jsonFlag {
			_ = writeJSONTo(opt.Stdout, doc)
		} else {
			writeModelTest(opt.Stdout, doc)
		}
		return 3
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
	if _, err := st.AppendProviderVerification(verified); err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	_ = st.ReleaseTutorCallReservation(reservation)
	doc.State = "verified"
	doc.Result = "verified"
	doc.CheckedAt = now.UTC().Format(time.RFC3339)
	if *jsonFlag {
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
		if p.Custom && p.ID == activeID {
			add(p.ID, p.Name, true)
			continue
		}
		if p.TextModel() {
			add(p.ID, p.Name, true)
		}
	}
	if activeID == "" && cfg.Provider == "" && cfg.Endpoint != "" {
		add("legacy", "Manual tutor configuration", true)
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
