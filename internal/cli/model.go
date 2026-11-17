package cli

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/models"
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

func runModelList(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	fs := newFlagSet("model list", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	svc, err := study.Open(opt.ports(*dbFlag))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	listing, err := models.Catalog(context.Background(), svc)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	if listing.RegistryErr != nil {
		return usageError(opt, *jsonFlag, "invalid_registry", listing.RegistryErr.Error())
	}
	if listing.ActiveErr != nil {
		return usageError(opt, *jsonFlag, "unknown_model", listing.ActiveErr.Error())
	}
	activeID := strings.TrimSpace(listing.Active.Provider)
	doc := modelListDoc{
		SchemaVersion:   ProtocolVersion,
		State:           "models",
		RegistryVersion: listing.Registry.Version,
		RegistrySource:  listing.Source,
		Active: modelActiveView{
			Configured: listing.Configured && activeID != "",
			Provider:   activeID,
			Model:      listing.Active.Model,
			Endpoint:   listing.Active.Endpoint,
		},
		Limits: modelLimits,
	}
	for _, p := range listing.Registry.Providers {
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
		item.Models = modelViewsOf(listing, p.ID, p, activeID == p.ID)
		if p.Custom && activeID == p.ID {
			custom := provider.Model{ID: listing.Active.Model, Capabilities: []provider.Capability{provider.CapTextChat}}
			item.Models = modelViewsOf(listing, p.ID, provider.Provider{Models: []provider.Model{custom}}, true)
		}
		doc.Providers = append(doc.Providers, item)
	}
	if listing.Configured && activeID == "" {
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
				ID:     listing.Active.Model,
				Label:  "hand-configured endpoint",
				Active: true,
				Capabilities: []modelCapabilityView{
					{ID: string(provider.CapTextChat), Declared: true, State: "legacy"},
				},
			}},
		})
	}
	doc.Budget = budgetViewOf(listing.Budget)
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
	// The flag layer's reference check ran before the store opened; the module
	// repeats it. Keeping it here preserves the error precedence.
	if _, _, err := provider.ParseRef(fs.Arg(0)); err != nil {
		return usageError(opt, *jsonFlag, "invalid_input", err.Error())
	}
	svc, err := study.Open(opt.ports(*dbFlag))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	used, err := models.Use(context.Background(), svc, fs.Arg(0), *endpointFlag)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	doc := modelUseDoc{
		SchemaVersion: ProtocolVersion,
		State:         "selected",
		Active: modelActiveView{
			Configured: true,
			Provider:   used.Config.Provider,
			Model:      used.Config.Model,
			Endpoint:   used.Config.Endpoint,
		},
		ProviderName:      used.Provider.Name,
		CredentialEnv:     used.CredentialEnv,
		CredentialPresent: used.CredentialPresent,
		TextChat:          capabilityViewOf(used.TextChat),
		Auth:              authState(used.CredentialPresent),
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
	if _, _, err := provider.ParseRef(fs.Arg(0)); err != nil {
		return usageError(opt, *jsonFlag, "invalid_input", err.Error())
	}
	svc, err := study.Open(opt.ports(*dbFlag))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	tested, err := models.Test(context.Background(), svc, fs.Arg(0), *endpointFlag)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	doc := modelTestDoc{
		SchemaVersion: ProtocolVersion,
		State:         "failed",
		Provider:      tested.Provider.ID,
		ProviderName:  tested.Provider.Name,
		Model:         tested.Model,
		Capability:    string(provider.CapTextChat),
		Endpoint:      tested.Config.Endpoint,
		CredentialEnv: tested.Env,
		CheckedAt:     tested.CheckedAt.UTC().Format(time.RFC3339),
		LatencyMS:     tested.LatencyMS,
		HTTPStatus:    tested.HTTPStatus,
		RequestSHA256: tested.SHA,
		RequestBytes:  tested.Bytes,
		Limits:        modelLimits,
	}
	if tested.Failure == nil {
		doc.State = "verified"
		doc.Result = "verified"
		if *jsonFlag {
			return writeJSON(opt, doc)
		}
		writeModelTest(opt.Stdout, doc)
		return 0
	}
	code := 3
	if !tested.CallMade {
		// The probe never reached the host; the document is the refusal's
		// detail, not an internal failure.
		code = 2
	}
	doc.Result = "failed:" + tested.Failure.Type
	doc.Failure = &modelFailureView{Type: tested.Failure.Type, Message: tested.Failure.Message, Action: tested.Failure.Action}
	if *jsonFlag {
		_ = writeJSONTo(opt.Stdout, doc)
		return code
	}
	writeModelTest(opt.Stdout, doc)
	return code
}

func runModelBudget(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	fs := newFlagSet("model budget", opt, wantJSON)
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	svc, err := study.Open(opt.ports(*dbFlag))
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer svc.Close()
	listing, err := models.Usage(context.Background(), svc)
	if err != nil {
		return fail(opt, *jsonFlag, err)
	}
	if listing.RegistryErr != nil {
		return usageError(opt, *jsonFlag, "invalid_registry", listing.RegistryErr.Error())
	}
	doc := modelBudgetDoc{
		SchemaVersion:   ProtocolVersion,
		State:           "budget",
		RegistryVersion: listing.Registry.Version,
		RegistrySource:  listing.Source,
		Budget:          budgetViewOf(listing.Budget),
		Limits:          modelLimits,
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeModelBudget(opt.Stdout, doc)
	return 0
}

func validTutorText(text string) bool {
	return strings.TrimSpace(text) != "" && utf8.ValidString(text) && !strings.ContainsRune(text, 0) &&
		!tutor.ContainsUnsafeControl(text) && utf8.RuneCountInString(text) <= tutor.MaxResponseChars
}

// buildBudgetView is the store-reading budget renderer for the paths that
// still read their own store; the model commands render the catalog's budget.
// Its remaining caller is the coach history, and it leaves this package with
// it.
func buildBudgetView(st *store.Store, cfg tutor.Config, registry provider.Registry, activeID string, opt Options) (modelBudgetView, error) {
	counts, err := st.ProviderCallCountsSince(tutorDayStart(opt.Now()))
	if err != nil {
		return modelBudgetView{}, err
	}
	return budgetViewOf(models.BudgetOf(cfg, registry, activeID, counts)), nil
}
