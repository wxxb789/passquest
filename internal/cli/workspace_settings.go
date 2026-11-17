package cli

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/search"
	"github.com/wxxb789/passquest/internal/studio"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/models"
)

// Models is `model list`: providers, models, capabilities and authorization
// as four separate facts, plus the per-provider budget. Usable is false with
// one aggregated line when no provider can answer (Q-0011-09).
func (w *Workspace) Models() (studio.ModelList, error) {
	if err := study.GuardPath(w.ports, "model"); err != nil {
		return studio.ModelList{}, cliCoded(err)
	}
	var list studio.ModelList
	if err := w.codedCall(func(svc *study.Service) error {
		listing, err := models.Catalog(context.Background(), svc)
		if err != nil {
			return err
		}
		if listing.RegistryErr != nil {
			return study.Refuse("invalid_registry", "%s", listing.RegistryErr.Error())
		}
		if listing.ActiveErr != nil {
			return study.Refuse("unknown_model", "%s", listing.ActiveErr.Error())
		}
		list = studioModelListOf(listing, w.opt)
		return nil
	}); err != nil {
		return studio.ModelList{}, err
	}
	missing, unverified := 0, 0
	for _, p := range list.Providers {
		verified := false
		for _, m := range p.Models {
			for _, c := range m.Capabilities {
				if c.ID == string(provider.CapTextChat) && (c.Verified || c.State == "legacy") {
					verified = true
				}
			}
		}
		switch {
		case p.Auth != "present":
			missing++
		case !verified:
			unverified++
		default:
			list.Usable = true
		}
	}
	if !list.Usable {
		list.Unusable = fmt.Sprintf(
			"No provider can answer: %d have no credential and %d have no verified text.chat model. Add a key in Settings, then run the model test.",
			missing, unverified)
	}
	return list, nil
}

// UseModel is `model use PROVIDER/MODEL [--endpoint URL]`.
func (w *Workspace) UseModel(id, endpoint string) (studio.ModelUse, error) {
	if strings.HasPrefix(id, "-") {
		return studio.ModelUse{}, &studio.Error{Code: "invalid_input", Message: "a provider/model id is required"}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := study.GuardPath(w.ports, "model"); err != nil {
		return studio.ModelUse{}, cliCoded(err)
	}
	if _, _, err := provider.ParseRef(id); err != nil {
		return studio.ModelUse{}, cliCoded(study.Refuse("invalid_input", "%s", err.Error()))
	}
	var use studio.ModelUse
	if err := w.codedCall(func(svc *study.Service) error {
		used, err := models.Use(context.Background(), svc, id, endpoint)
		if err != nil {
			return err
		}
		use = studioModelUseOf(used)
		return nil
	}); err != nil {
		return studio.ModelUse{}, err
	}
	return use, nil
}

// TestModel is `model test`: one bounded text.chat probe whose verdict is
// recorded as verification evidence. A failed probe is a result, not an
// error.
func (w *Workspace) TestModel(id, endpoint string) (studio.ModelTest, error) {
	if strings.HasPrefix(id, "-") {
		return studio.ModelTest{}, &studio.Error{Code: "invalid_input", Message: "a provider/model id is required"}
	}
	if err := w.requireNode(w.hostTutor); err != nil {
		return studio.ModelTest{}, err
	}
	if err := study.GuardPath(w.ports, "model"); err != nil {
		return studio.ModelTest{}, cliCoded(err)
	}
	if _, _, err := provider.ParseRef(id); err != nil {
		return studio.ModelTest{}, cliCoded(study.Refuse("invalid_input", "%s", err.Error()))
	}
	var test studio.ModelTest
	if err := w.codedCall(func(svc *study.Service) error {
		tested, err := models.Test(context.Background(), svc, id, endpoint)
		if err != nil {
			return err
		}
		test = studioModelTestOf(tested)
		return nil
	}); err != nil {
		return studio.ModelTest{}, err
	}
	return test, nil
}

// Budget is `model budget`: the per-provider daily call budget (UTC).
func (w *Workspace) Budget() (studio.Budget, error) {
	if err := study.GuardPath(w.ports, "model"); err != nil {
		return studio.Budget{}, cliCoded(err)
	}
	var budget studio.Budget
	if err := w.codedCall(func(svc *study.Service) error {
		listing, err := models.Usage(context.Background(), svc)
		if err != nil {
			return err
		}
		if listing.RegistryErr != nil {
			return study.Refuse("invalid_registry", "%s", listing.RegistryErr.Error())
		}
		budget = studioModelBudgetOf(listing.Budget)
		return nil
	}); err != nil {
		return studio.Budget{}, err
	}
	return budget, nil
}

// codedCall opens one study service over the workspace ports, runs fn and maps
// the error the way the shimmed CLI call mapped it. These four methods used to
// run through w.run; the command's error document carried the study refusal
// unchanged and everything else as internal_error, which is cliCoded.
func (w *Workspace) codedCall(fn func(svc *study.Service) error) error {
	svc, err := study.Open(w.ports)
	if err != nil {
		return cliCoded(err)
	}
	defer svc.Close()
	return cliCoded(fn(svc))
}

// studioModelListOf renders one catalog read as the window's model list. The
// providers and each provider's models are appended like `model list` built
// its document, so an empty registry stays a null list and a provider without
// models stays an empty one.
func studioModelListOf(listing models.Listing, opt Options) studio.ModelList {
	activeID := strings.TrimSpace(listing.Active.Provider)
	list := studio.ModelList{
		RegistryVersion: listing.Registry.Version,
		RegistrySource:  listing.Source,
		Active: studio.ActiveModel{
			Configured: listing.Configured && activeID != "",
			Provider:   activeID,
			Model:      listing.Active.Model,
			Endpoint:   listing.Active.Endpoint,
		},
		Limits: modelLimits,
	}
	for _, p := range listing.Registry.Providers {
		env, present := credentialState(opt, p.ID)
		item := studio.ModelProvider{
			ID:            p.ID,
			Name:          p.Name,
			EndpointShape: p.EndpointShape,
			CredentialEnv: env,
			Auth:          authState(present),
			Custom:        p.Custom,
			Active:        activeID == p.ID,
			Models:        studioModelsOf(listing, p.ID, p, activeID == p.ID),
		}
		if p.Custom && activeID == p.ID {
			custom := provider.Model{ID: listing.Active.Model, Capabilities: []provider.Capability{provider.CapTextChat}}
			item.Models = studioModelsOf(listing, p.ID, provider.Provider{Models: []provider.Model{custom}}, true)
		}
		list.Providers = append(list.Providers, item)
	}
	if listing.Configured && activeID == "" {
		env, present := credentialState(opt, "")
		list.Active.Legacy = true
		list.Providers = append(list.Providers, studio.ModelProvider{
			ID:            "legacy",
			Name:          "Manual tutor configuration",
			EndpointShape: provider.EndpointShape,
			CredentialEnv: env,
			Auth:          authState(present),
			Active:        true,
			Models: []studio.Model{{
				ID:     listing.Active.Model,
				Label:  "hand-configured endpoint",
				Active: true,
				Capabilities: []studio.Capability{
					{ID: string(provider.CapTextChat), Declared: true, State: "legacy"},
				},
			}},
		})
	}
	list.Budget = studioModelBudgetOf(listing.Budget)
	return list
}

// studioModelsOf renders one provider's models with their capability evidence,
// the way `model list` shows them.
func studioModelsOf(listing models.Listing, providerID string, p provider.Provider, active bool) []studio.Model {
	views := make([]studio.Model, 0, len(p.Models))
	for _, m := range p.Models {
		item := studio.Model{
			ID:      m.ID,
			Label:   m.Label,
			Default: m.Default,
			Active:  active && listing.Active.Model == m.ID,
		}
		verifications := listing.Index.ForModel(providerID, m.ID)
		for _, c := range provider.Capabilities {
			item.Capabilities = append(item.Capabilities,
				studioCapabilityOf(models.CapabilityStateOf(c, m.Has(c), verifications)))
		}
		views = append(views, item)
	}
	return views
}

// studioCapabilityOf renders one capability's evidence state as the window's
// value.
func studioCapabilityOf(state models.CapabilityState) studio.Capability {
	return studio.Capability{
		ID:          state.ID,
		Declared:    state.Declared,
		State:       state.State,
		Verified:    state.Verified,
		CheckedAt:   state.CheckedAt,
		FailureType: state.FailureType,
		Action:      state.Action,
	}
}

// studioModelUseOf renders one model selection as the window's value.
func studioModelUseOf(used models.Used) studio.ModelUse {
	return studio.ModelUse{
		Active: studio.ActiveModel{
			Configured: true,
			Provider:   used.Config.Provider,
			Model:      used.Config.Model,
			Endpoint:   used.Config.Endpoint,
		},
		ProviderName:      used.Provider.Name,
		CredentialEnv:     used.CredentialEnv,
		CredentialPresent: used.CredentialPresent,
		Auth:              authState(used.CredentialPresent),
		TextChat:          studioCapabilityOf(used.TextChat),
	}
}

// studioModelTestOf renders one capability probe as the window's value. A
// failed probe is a result, not an error.
func studioModelTestOf(tested models.Tested) studio.ModelTest {
	test := studio.ModelTest{
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
	}
	if tested.Failure == nil {
		test.State = "verified"
		test.Result = "verified"
		return test
	}
	test.Result = "failed:" + tested.Failure.Type
	test.Failure = &studio.CoachFailure{
		Code:    tested.Failure.Type,
		Message: tested.Failure.Message,
		Action:  tested.Failure.Action,
	}
	return test
}

// studioModelBudgetOf renders the catalog's budget the way the shimmed
// `model list` and `model budget` documents decoded: an empty catalog keeps a
// nil provider list (the JSON round trip of a null), unlike Today's
// studioBudgetOf, which always reports a list.
func studioModelBudgetOf(budget models.Budget) studio.Budget {
	view := studio.Budget{
		PerProviderLimit: budget.PerProviderLimit,
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

// SetCredential stores a provider key in the credential store. The key is
// never written to SQLite, a log or a returned document; only its presence is
// reported. A variable set in the environment still wins (Q-0011-03).
func (w *Workspace) SetCredential(providerID, key string) (studio.CredentialStatus, error) {
	p, err := w.registryProvider(providerID)
	if err != nil {
		return studio.CredentialStatus{}, err
	}
	key = strings.TrimSpace(key)
	if key == "" || strings.IndexFunc(key, unicode.IsControl) >= 0 || strings.ContainsAny(key, " \t") || len(key) > 4096 {
		return studio.CredentialStatus{}, &studio.Error{Code: "invalid_input", Message: "the key must be one line of printable characters without spaces"}
	}
	if w.credentials == nil {
		return studio.CredentialStatus{}, withAction(&studio.Error{Code: "credential_store_unavailable",
			Message: "no OS credential store is available; set " + provider.CredentialEnvFor(p.ID) + " in the environment instead"})
	}
	if err := w.credentials.Set(p.ID, key); err != nil {
		return studio.CredentialStatus{}, &studio.Error{Code: "credential_store_error", Message: "the credential store refused the key: " + redactKey(err.Error(), key)}
	}
	return w.credentialStatus(p.ID), nil
}

// ClearCredential removes a provider key from the credential store. An
// environment variable, if set, is not affected.
func (w *Workspace) ClearCredential(providerID string) (studio.CredentialStatus, error) {
	p, err := w.registryProvider(providerID)
	if err != nil {
		return studio.CredentialStatus{}, err
	}
	if w.credentials == nil {
		return studio.CredentialStatus{}, withAction(&studio.Error{Code: "credential_store_unavailable", Message: "no OS credential store is available"})
	}
	if err := w.credentials.Delete(p.ID); err != nil {
		return studio.CredentialStatus{}, &studio.Error{Code: "credential_store_error", Message: err.Error()}
	}
	return w.credentialStatus(p.ID), nil
}

// Services reports Node availability, the AnkiConnect endpoint, whether the
// search endpoint is configured, the credential store and each provider's
// credential presence.
func (w *Workspace) Services() (studio.Services, error) {
	services := studio.Services{
		AnkiEndpoint:     noteAnkiEndpoint("", w.getenv),
		SearchConfigured: strings.TrimSpace(w.getenv(search.EndpointEnv)) != "",
		SearchEnv:        search.EndpointEnv,
		CredentialStore:  w.credentials != nil,
		Credentials:      []studio.CredentialStatus{},
		Database:         w.dbPath,
	}
	if path, err := w.nodePath(); err == nil {
		services.NodeAvailable, services.NodePath = true, path
	} else {
		services.NodeAction = codeActions["node_unavailable"]
	}
	providers, err := w.credentialProviders()
	if err != nil {
		return services, err
	}
	for _, p := range providers {
		services.Credentials = append(services.Credentials, w.credentialStatus(p.ID))
	}
	return services, nil
}

func (w *Workspace) registryProvider(providerID string) (provider.Provider, error) {
	providers, err := w.credentialProviders()
	if err != nil {
		return provider.Provider{}, err
	}
	for _, p := range providers {
		if p.ID == strings.TrimSpace(providerID) {
			return p, nil
		}
	}
	return provider.Provider{}, &studio.Error{Code: "unknown_provider", Message: fmt.Sprintf("provider %q is not in the registry", providerID)}
}

// credentialStatus reports where a provider's key would come from, never the
// key itself.
func (w *Workspace) credentialStatus(providerID string) studio.CredentialStatus {
	env := provider.CredentialEnvFor(providerID)
	status := studio.CredentialStatus{Provider: providerID, Env: env, From: "none"}
	if _, _, present := provider.CredentialValue(w.baseGetenv, providerID); present {
		status.Present, status.From = true, "env"
		return status
	}
	if w.credentials != nil {
		value, found, err := w.credentials.Get(providerID)
		switch {
		case err != nil:
			status.Problem = "the credential store could not be read"
		case found && strings.TrimSpace(value) != "":
			status.Present, status.From = true, "store"
		}
	}
	return status
}

func redactKey(message, key string) string {
	if key == "" {
		return message
	}
	return strings.ReplaceAll(message, key, "[redacted]")
}
