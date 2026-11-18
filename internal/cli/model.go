package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/models"
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
	selection := models.SelectionOf(listing)
	doc := modelListDoc{
		SchemaVersion:   ProtocolVersion,
		State:           "models",
		RegistryVersion: listing.Registry.Version,
		RegistrySource:  listing.Source,
		Active: modelActiveView{
			Configured: selection.Configured,
			Provider:   selection.Provider,
			Model:      selection.Model,
			Endpoint:   selection.Endpoint,
			Legacy:     selection.Legacy,
		},
		Limits: models.Limits(),
	}
	for _, p := range models.ProvidersOf(listing) {
		env, present := models.CredentialState(opt.Getenv, p.CredentialID)
		item := modelProviderView{
			ID:            p.ID,
			Name:          p.Name,
			EndpointShape: p.EndpointShape,
			CredentialEnv: env,
			Auth:          models.AuthState(present),
			Custom:        p.Custom,
			Active:        p.Active,
			Models:        make([]modelView, 0, len(p.Models)),
		}
		for _, m := range p.Models {
			model := modelView{ID: m.ID, Label: m.Label, Default: m.Default, Active: m.Active}
			for _, c := range m.Capabilities {
				model.Capabilities = append(model.Capabilities, capabilityViewOf(c))
			}
			item.Models = append(item.Models, model)
		}
		doc.Providers = append(doc.Providers, item)
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
		Auth:              models.AuthState(used.CredentialPresent),
		Limits:            models.Limits(),
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
		Limits:        models.Limits(),
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
		Limits:          models.Limits(),
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeModelBudget(opt.Stdout, doc)
	return 0
}
