package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/search"
	"github.com/wxxb789/passquest/internal/studio"
)

// Models is `model list`: providers, models, capabilities and authorization
// as four separate facts, plus the per-provider budget. Usable is false with
// one aggregated line when no provider can answer (Q-0011-09).
func (w *Workspace) Models() (studio.ModelList, error) {
	var doc modelListDoc
	if err := w.run("model", "list").decode(&doc); err != nil {
		return studio.ModelList{}, err
	}
	var list studio.ModelList
	if err := convertJSON(doc, &list); err != nil {
		return studio.ModelList{}, workspaceError(err)
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
	args := []string{"model", "use"}
	if strings.TrimSpace(endpoint) != "" {
		args = append(args, "--endpoint="+endpoint)
	}
	args = append(args, id)
	if strings.HasPrefix(id, "-") {
		return studio.ModelUse{}, &studio.Error{Code: "invalid_input", Message: "a provider/model id is required"}
	}
	w.mu.Lock()
	result := w.run(args...)
	w.mu.Unlock()
	var doc modelUseDoc
	if err := result.decode(&doc); err != nil {
		return studio.ModelUse{}, err
	}
	var use studio.ModelUse
	return use, workspaceErrorOrNil(convertJSON(doc, &use))
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
	args := []string{"model", "test"}
	if strings.TrimSpace(endpoint) != "" {
		args = append(args, "--endpoint="+endpoint)
	}
	args = append(args, id)
	result := w.run(args...)
	var doc modelTestDoc
	if json.Unmarshal(result.stdout, &doc) != nil || doc.State == "" {
		return studio.ModelTest{}, result.err()
	}
	test := studio.ModelTest{
		State:         doc.State,
		Provider:      doc.Provider,
		ProviderName:  doc.ProviderName,
		Model:         doc.Model,
		Capability:    doc.Capability,
		Endpoint:      doc.Endpoint,
		CredentialEnv: doc.CredentialEnv,
		Result:        doc.Result,
		CheckedAt:     doc.CheckedAt,
		LatencyMS:     doc.LatencyMS,
		HTTPStatus:    doc.HTTPStatus,
	}
	if doc.Failure != nil {
		test.Failure = &studio.CoachFailure{Code: doc.Failure.Type, Message: doc.Failure.Message, Action: doc.Failure.Action}
	}
	return test, nil
}

// Budget is `model budget`: the per-provider daily call budget (UTC).
func (w *Workspace) Budget() (studio.Budget, error) {
	var doc modelBudgetDoc
	if err := w.run("model", "budget").decode(&doc); err != nil {
		return studio.Budget{}, err
	}
	budget := studio.Budget{Providers: []studio.BudgetProvider{}}
	return budget, workspaceErrorOrNil(convertJSON(doc.Budget, &budget))
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
	if strings.TrimSpace(w.baseGetenv(env)) != "" {
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
			return status
		}
	}
	if _, _, present := provider.CredentialValue(w.baseGetenv, providerID); present {
		status.Present, status.From = true, "env"
	}
	return status
}

func redactKey(message, key string) string {
	if key == "" {
		return message
	}
	return strings.ReplaceAll(message, key, "[redacted]")
}

func workspaceErrorOrNil(err error) error {
	if err == nil {
		return nil
	}
	return workspaceError(err)
}
