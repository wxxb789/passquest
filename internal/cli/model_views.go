package cli

import (
	"fmt"
	"io"
)

// Model JSON documents. Field names are protocol; the four axes stay separate:
// provider, model, capability and authorization are never folded together.

type modelCapabilityView struct {
	ID          string `json:"id"`
	Declared    bool   `json:"declared"`
	State       string `json:"state"`
	Verified    bool   `json:"verified"`
	CheckedAt   string `json:"checked_at,omitempty"`
	FailureType string `json:"failure_type,omitempty"`
	Action      string `json:"action,omitempty"`
}

type modelView struct {
	ID           string                `json:"id"`
	Label        string                `json:"label,omitempty"`
	Default      bool                  `json:"default"`
	Active       bool                  `json:"active"`
	Capabilities []modelCapabilityView `json:"capabilities"`
}

type modelProviderView struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	EndpointShape string      `json:"endpoint_shape"`
	CredentialEnv string      `json:"credential_env"`
	Auth          string      `json:"auth"`
	Custom        bool        `json:"custom"`
	Active        bool        `json:"active"`
	Models        []modelView `json:"models"`
}

type modelActiveView struct {
	Configured bool   `json:"configured"`
	Legacy     bool   `json:"legacy,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
}

type modelBudgetProviderView struct {
	Provider string `json:"provider"`
	Label    string `json:"label"`
	Calls    int    `json:"calls"`
	Limit    int    `json:"limit"`
	Active   bool   `json:"active"`
}

type modelBudgetView struct {
	PerProviderLimit int                       `json:"per_provider_limit"`
	Providers        []modelBudgetProviderView `json:"providers"`
	TotalCalls       int                       `json:"total_calls"`
	TotalLimit       int                       `json:"total_limit"`
	ProviderCount    int                       `json:"provider_count"`
	Ceiling          string                    `json:"ceiling"`
}

type modelListDoc struct {
	SchemaVersion   int                 `json:"schema_version"`
	State           string              `json:"state"`
	RegistryVersion int                 `json:"registry_version"`
	RegistrySource  string              `json:"registry_source"`
	Active          modelActiveView     `json:"active"`
	Providers       []modelProviderView `json:"providers"`
	Budget          modelBudgetView     `json:"budget"`
	Limits          []string            `json:"limits"`
}

type modelUseDoc struct {
	SchemaVersion     int                 `json:"schema_version"`
	State             string              `json:"state"`
	Active            modelActiveView     `json:"active"`
	ProviderName      string              `json:"provider_name"`
	CredentialEnv     string              `json:"credential_env"`
	CredentialPresent bool                `json:"credential_present"`
	Auth              string              `json:"auth"`
	TextChat          modelCapabilityView `json:"text_chat"`
	Limits            []string            `json:"limits"`
}

type modelFailureView struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Action  string `json:"action"`
}

type modelTestDoc struct {
	SchemaVersion int               `json:"schema_version"`
	State         string            `json:"state"`
	Provider      string            `json:"provider"`
	ProviderName  string            `json:"provider_name"`
	Model         string            `json:"model"`
	Capability    string            `json:"capability"`
	Endpoint      string            `json:"endpoint"`
	CredentialEnv string            `json:"credential_env"`
	Result        string            `json:"result"`
	Failure       *modelFailureView `json:"failure,omitempty"`
	CheckedAt     string            `json:"checked_at"`
	LatencyMS     int64             `json:"latency_ms"`
	HTTPStatus    int               `json:"http_status"`
	RequestSHA256 string            `json:"request_sha256"`
	RequestBytes  int               `json:"request_bytes"`
	Limits        []string          `json:"limits"`
}

type modelBudgetDoc struct {
	SchemaVersion   int             `json:"schema_version"`
	State           string          `json:"state"`
	RegistryVersion int             `json:"registry_version"`
	RegistrySource  string          `json:"registry_source"`
	Budget          modelBudgetView `json:"budget"`
	Limits          []string        `json:"limits"`
}

var modelLimits = []string{
	"A registry entry is a declaration; only model test records verified evidence.",
	"A capability that is not verified is never offered as working.",
	"The daily budget is counted per provider in UTC; the ceiling is the printed sum, not one global cap.",
	"Model text, provider availability and price are not exam facts and never change an attempt or answer key.",
}

func writeModelList(w io.Writer, doc modelListDoc) {
	fmt.Fprintln(w, "Pass Quest — provider and model registry")
	active := "none (manual tutor configuration)"
	switch {
	case doc.Active.Legacy:
		active = "legacy/manual endpoint (no registry provider)"
	case doc.Active.Configured:
		active = doc.Active.Provider + "/" + doc.Active.Model
	}
	fmt.Fprintf(w, "Active: %s\n", active)
	fmt.Fprintf(w, "Registry: %s version %d\n", doc.RegistrySource, doc.RegistryVersion)
	fmt.Fprintln(w, "Axes: provider | model | capability | authorization")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%-10s %-20s %-14s %-24s %-18s %-12s %-10s %-18s\n",
		"PROVIDER", "MODEL", "AUTH", "TEXT.CHAT", "TOOL.CALLS", "VISION", "SEARCH", "SPEECH.SYNTHESIS")
	for _, p := range doc.Providers {
		if len(p.Models) == 0 {
			marker := " "
			if p.Active {
				marker = "*"
			}
			fmt.Fprintf(w, "%s%-9s %-20s %-14s %s\n", marker, p.ID, "(no model selected)", p.Auth,
				"choose one with model use custom/<model> --endpoint URL")
			continue
		}
		for index, m := range p.Models {
			id := ""
			if index == 0 {
				id = p.ID
			}
			marker := " "
			if m.Active {
				marker = "*"
			}
			caps := map[string]modelCapabilityView{}
			for _, c := range m.Capabilities {
				caps[c.ID] = c
			}
			fmt.Fprintf(w, "%s%-9s %-20s %-14s %-24s %-18s %-12s %-10s %-18s\n",
				marker, id, m.ID, p.Auth,
				renderCapability(caps[string(capTextChat)]),
				renderCapability(caps[string(capToolCalls)]),
				renderCapability(caps[string(capVision)]),
				renderCapability(caps[string(capSearch)]),
				renderCapability(caps[string(capSpeech)]))
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "AUTH: present means the provider credential variable is set; no_credential never means unsupported.")
	fmt.Fprintln(w, "CAPABILITY: declared means untested; verified/failed carry the model test timestamp; unsupported is not declared.")
	fmt.Fprintln(w, "Daily provider budget (UTC):")
	for _, item := range doc.Budget.Providers {
		marker := ""
		if item.Active {
			marker = " *active"
		}
		fmt.Fprintf(w, "  %-12s %3d / %3d%s\n", item.Provider, item.Calls, item.Limit, marker)
	}
	fmt.Fprintf(w, "  %-12s %3d / %3d  (%s)\n", "TOTAL", doc.Budget.TotalCalls, doc.Budget.TotalLimit, doc.Budget.Ceiling)
	fmt.Fprintf(w, "\nSwitch with: passquest model use <provider>/<model>\n")
	fmt.Fprintf(w, "Verify with: passquest model test <provider>/<model>\n")
}

func renderCapability(c modelCapabilityView) string {
	if c.ID == "" || !c.Declared {
		return "unsupported"
	}
	switch c.State {
	case "verified":
		return "verified@" + shortTimestamp(c.CheckedAt)
	case "failed":
		return "failed:" + c.FailureType + "@" + shortTimestamp(c.CheckedAt)
	case "legacy":
		return "legacy,unverified"
	default:
		return "declared,unverified"
	}
}

func shortTimestamp(value string) string {
	if len(value) >= 20 {
		return value[:19] + "Z"
	}
	return value
}

func writeModelUse(w io.Writer, doc modelUseDoc) {
	fmt.Fprintln(w, "Pass Quest — active model")
	fmt.Fprintf(w, "Active: %s/%s (%s)\n", doc.Active.Provider, doc.Active.Model, doc.ProviderName)
	fmt.Fprintf(w, "Endpoint: %s\n", doc.Active.Endpoint)
	if doc.CredentialPresent {
		fmt.Fprintf(w, "Authorization: present in %s (value never shown or stored)\n", doc.CredentialEnv)
	} else {
		fmt.Fprintf(w, "Authorization: no_credential — export %s (it is never stored)\n", doc.CredentialEnv)
	}
	fmt.Fprintf(w, "text.chat: %s\n", renderCapability(doc.TextChat))
	if doc.TextChat.State != "verified" {
		fmt.Fprintf(w, "Next: passquest model test %s/%s\n", doc.Active.Provider, doc.Active.Model)
	}
	fmt.Fprintln(w, "Practice state, attempts and the answer key are unchanged.")
}

func writeModelTest(w io.Writer, doc modelTestDoc) {
	fmt.Fprintln(w, "Pass Quest — model capability test")
	fmt.Fprintf(w, "Provider: %s (%s)\n", doc.Provider, doc.ProviderName)
	fmt.Fprintf(w, "Model: %s\n", doc.Model)
	fmt.Fprintf(w, "Capability: %s\n", doc.Capability)
	fmt.Fprintf(w, "Endpoint: %s\n", doc.Endpoint)
	fmt.Fprintf(w, "Credential: %s\n", doc.CredentialEnv)
	if doc.RequestSHA256 != "" {
		fmt.Fprintf(w, "Request: %d bytes, sha256 %s\n", doc.RequestBytes, doc.RequestSHA256)
	}
	if doc.Failure == nil {
		fmt.Fprintf(w, "Result: verified at %s (%d ms, HTTP %d)\n", doc.CheckedAt, doc.LatencyMS, doc.HTTPStatus)
		fmt.Fprintln(w, "The verified text.chat capability is now recorded for this model.")
		return
	}
	fmt.Fprintf(w, "Result: failed:%s at %s (%d ms, HTTP %d)\n",
		doc.Failure.Type, doc.CheckedAt, doc.LatencyMS, doc.HTTPStatus)
	fmt.Fprintf(w, "Detail: %s\n", doc.Failure.Message)
	fmt.Fprintf(w, "Next: %s\n", doc.Failure.Action)
	fmt.Fprintln(w, "No attempt, answer key or practice state was changed.")
}

func writeModelBudget(w io.Writer, doc modelBudgetDoc) {
	fmt.Fprintln(w, "Pass Quest — provider call budget (UTC day)")
	fmt.Fprintf(w, "Per-provider limit: %d (goal-0005 daily value)\n\n", doc.Budget.PerProviderLimit)
	fmt.Fprintf(w, "%-14s %6s %6s\n", "PROVIDER", "CALLS", "LIMIT")
	for _, item := range doc.Budget.Providers {
		marker := ""
		if item.Active {
			marker = " *active"
		}
		fmt.Fprintf(w, "%-14s %6d %6d %s\n", item.Provider, item.Calls, item.Limit, marker)
	}
	fmt.Fprintf(w, "%-14s %6d %6d\n", "TOTAL", doc.Budget.TotalCalls, doc.Budget.TotalLimit)
	fmt.Fprintf(w, "Ceiling: %s provider calls per day; the sum, not one global cap.\n", doc.Budget.Ceiling)
	fmt.Fprintln(w, "A refusal at one provider's limit leaves every other provider's budget usable.")
}

// Aliases keep the table renderer independent of the registry package import
// cycle that a direct provider.Capability map key would create in this file.
const (
	capTextChat  = "text.chat"
	capToolCalls = "tool.calls"
	capVision    = "vision"
	capSearch    = "search"
	capSpeech    = "speech.synthesis"
)
