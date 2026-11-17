package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/wxxb789/passquest/internal/provider"
	"github.com/wxxb789/passquest/internal/tutor"
)

func runTutorConfig(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{
		"endpoint": true, "model": true, "timeout-seconds": true,
		"max-output-tokens": true, "daily-calls": true, "db": true,
	})
	fs := newFlagSet("tutor config", opt, wantJSON)
	endpointFlag := fs.String("endpoint", "", "OpenAI-compatible chat-completions endpoint URL")
	modelFlag := fs.String("model", "", "provider model name")
	timeoutFlag := fs.Int("timeout-seconds", 0, "provider timeout in seconds (1-120)")
	maxTokensFlag := fs.Int("max-output-tokens", 0, "provider max_tokens (64-2048)")
	dailyFlag := fs.Int("daily-calls", 0, "daily call budget (1-200)")
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

	cfg, configured, err := st.TutorConfig()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	if !configured {
		cfg = tutor.DefaultConfig()
	}
	changed := false
	manual := false
	invalid := ""
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "endpoint":
			changed = true
			manual = true
			cfg.Endpoint = *endpointFlag
		case "model":
			changed = true
			manual = true
			cfg.Model = *modelFlag
		case "timeout-seconds":
			changed = true
			if *timeoutFlag < tutor.MinTimeoutMS/1000 || *timeoutFlag > tutor.MaxTimeoutMS/1000 {
				invalid = fmt.Sprintf("--timeout-seconds %d is outside %d..%d",
					*timeoutFlag, tutor.MinTimeoutMS/1000, tutor.MaxTimeoutMS/1000)
				return
			}
			cfg.TimeoutMS = int64(*timeoutFlag) * 1000
		case "max-output-tokens":
			changed = true
			cfg.MaxOutputTokens = *maxTokensFlag
		case "daily-calls":
			changed = true
			cfg.DailyCalls = *dailyFlag
		}
	})
	if invalid != "" {
		return usageError(opt, *jsonFlag, "invalid_config", invalid)
	}
	if manual {
		// A hand-edited endpoint and model are the goal-0005 legacy
		// configuration; the registry selection no longer describes them.
		cfg.Provider = ""
	}
	if changed {
		if err := cfg.Validate(); err != nil {
			return usageError(opt, *jsonFlag, "invalid_config", err.Error())
		}
		if err := st.SaveTutorConfig(cfg, opt.Now()); err != nil {
			return internalError(opt, *jsonFlag, err)
		}
		configured = true
	}

	env, _, present := provider.CredentialValue(opt.Getenv, cfg.Provider)
	doc := tutorConfigDoc{
		SchemaVersion: ProtocolVersion,
		State:         "config",
		Configured:    configured,
		Config:        tutorConfigViewOf(cfg),
		Credential:    tutorCredentialView{Env: env, Present: present},
		Limits:        tutorLimits,
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeTutorConfig(opt.Stdout, doc)
	return 0
}

func writeTutorConfig(w io.Writer, doc tutorConfigDoc) {
	fmt.Fprintln(w, "Pass Quest — tutor configuration")
	if !doc.Configured {
		fmt.Fprintln(w, "Provider: not configured")
		fmt.Fprintln(w, "Set one OpenAI-compatible endpoint and model:")
		fmt.Fprintln(w, `  passquest tutor config --endpoint https://api.example.com/v1/chat/completions --model MODEL`)
	} else {
		if doc.Config.Provider != "" {
			fmt.Fprintf(w, "Provider: %s\n", doc.Config.Provider)
		}
		fmt.Fprintf(w, "Provider endpoint: %s\n", doc.Config.Endpoint)
		fmt.Fprintf(w, "Model: %s\n", doc.Config.Model)
		fmt.Fprintf(w, "Timeout: %d ms\n", doc.Config.TimeoutMS)
		fmt.Fprintf(w, "Max output tokens: %d\n", doc.Config.MaxOutputTokens)
		fmt.Fprintf(w, "Daily call budget: %d (UTC day, failed calls included)\n", doc.Config.DailyCalls)
	}
	if doc.Credential.Present {
		fmt.Fprintf(w, "Credential: %s is set (value never shown or stored)\n", doc.Credential.Env)
	} else {
		fmt.Fprintf(w, "Credential: %s is missing (export it in this environment before tutor ask)\n", doc.Credential.Env)
	}
	fmt.Fprintln(w, "\nThe agent host is the only component that contacts the provider; ordinary practice works without it.")
	fmt.Fprintln(w, "Prefer a registered provider: passquest model list, then passquest model use <provider>/<model>.")
}
