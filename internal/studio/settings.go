package studio

// ModelList keeps the four facts separate: provider, model, capability and
// authorization.
type ModelList struct {
	RegistryVersion int             `json:"registry_version"`
	RegistrySource  string          `json:"registry_source"`
	Active          ActiveModel     `json:"active"`
	Providers       []ModelProvider `json:"providers"`
	Budget          Budget          `json:"budget"`
	Usable          bool            `json:"usable"`
	Unusable        string          `json:"unusable,omitempty"`
	Limits          []string        `json:"limits"`
}

// ActiveModel is the stored tutor selection.
type ActiveModel struct {
	Configured bool   `json:"configured"`
	Legacy     bool   `json:"legacy,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
}

// ModelProvider is one registry provider and its authorization fact.
type ModelProvider struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	EndpointShape string  `json:"endpoint_shape"`
	CredentialEnv string  `json:"credential_env"`
	Auth          string  `json:"auth"`
	Custom        bool    `json:"custom"`
	Active        bool    `json:"active"`
	Models        []Model `json:"models"`
}

// Model is one registry model and its capability facts.
type Model struct {
	ID           string       `json:"id"`
	Label        string       `json:"label,omitempty"`
	Default      bool         `json:"default"`
	Active       bool         `json:"active"`
	Capabilities []Capability `json:"capabilities"`
}

// Capability is declared, verified, failed or unsupported.
type Capability struct {
	ID          string `json:"id"`
	Declared    bool   `json:"declared"`
	State       string `json:"state"`
	Verified    bool   `json:"verified"`
	CheckedAt   string `json:"checked_at,omitempty"`
	FailureType string `json:"failure_type,omitempty"`
	Action      string `json:"action,omitempty"`
}

// ModelUse is the result of selecting a model.
type ModelUse struct {
	Active            ActiveModel `json:"active"`
	ProviderName      string      `json:"provider_name"`
	CredentialEnv     string      `json:"credential_env"`
	CredentialPresent bool        `json:"credential_present"`
	Auth              string      `json:"auth"`
	TextChat          Capability  `json:"text_chat"`
}

// ModelTest is the result of the text.chat probe.
type ModelTest struct {
	State         string        `json:"state"`
	Provider      string        `json:"provider"`
	ProviderName  string        `json:"provider_name"`
	Model         string        `json:"model"`
	Capability    string        `json:"capability"`
	Endpoint      string        `json:"endpoint"`
	CredentialEnv string        `json:"credential_env"`
	Result        string        `json:"result"`
	Failure       *CoachFailure `json:"failure"`
	CheckedAt     string        `json:"checked_at"`
	LatencyMS     int64         `json:"latency_ms"`
	HTTPStatus    int           `json:"http_status"`
}

// Services reports the local helpers the window depends on.
type Services struct {
	NodeAvailable    bool               `json:"node_available"`
	NodePath         string             `json:"node_path,omitempty"`
	NodeAction       string             `json:"node_action,omitempty"`
	AnkiEndpoint     string             `json:"anki_endpoint"`
	SearchConfigured bool               `json:"search_configured"`
	SearchEnv        string             `json:"search_env"`
	CredentialStore  bool               `json:"credential_store"`
	Credentials      []CredentialStatus `json:"credentials"`
	Database         string             `json:"database"`
}

// CredentialStatus is one provider's credential presence and where it came
// from: env (the variable wins), store (the OS credential store) or none.
type CredentialStatus struct {
	Provider string `json:"provider"`
	Env      string `json:"env"`
	Present  bool   `json:"present"`
	From     string `json:"from"`
	Problem  string `json:"problem,omitempty"`
}

// BriefStatus is the latest study brief.
type BriefStatus struct {
	Found       bool        `json:"found"`
	RunID       string      `json:"run_id,omitempty"`
	Task        string      `json:"task,omitempty"`
	State       string      `json:"state"`
	Reason      string      `json:"reason,omitempty"`
	Action      string      `json:"action,omitempty"`
	Provider    string      `json:"provider,omitempty"`
	Model       string      `json:"model,omitempty"`
	MaxSteps    int         `json:"max_steps"`
	Steps       []BriefStep `json:"steps"`
	LatestEvent string      `json:"latest_event,omitempty"`
	Live        bool        `json:"live"`
	Answer      string      `json:"answer,omitempty"`
	Limits      []string    `json:"limits"`
}

// BriefStep is one recorded brief step; its text is advisory.
type BriefStep struct {
	Index    int    `json:"index"`
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Advisory bool   `json:"advisory"`
}
