package types

// ProvidersConfig is the top-level configuration from providers.yaml.
type ProvidersConfig struct {
	LLM LLMProvidersConfig `yaml:"llm"`
}

// LLMProvidersConfig holds the default and per-agent LLM provider settings.
type LLMProvidersConfig struct {
	Default LLMProviderConfig            `yaml:"default"`
	Agents  map[string]LLMProviderConfig `yaml:"agents"`
}

// LLMProviderConfig is the configuration for a single LLM provider instance.
type LLMProviderConfig struct {
	Provider   string `yaml:"provider"` // "openai", etc.
	APIKey     string `yaml:"api_key"`
	Model      string `yaml:"model"`
	BaseURL    string `yaml:"base_url"`
	ThinkAloud *bool  `yaml:"think_aloud"` // nil = inherit from default; true/false = per-agent override

	// ContextWindow is the deploy-time-declared max input token window of
	// the selected model. Used by the runtime to derive byte budgets
	// (blob_max_inline / agent_max_tool_history) via the fraction-form
	// yaml knobs, and by BaseAgent's context-pressure watchdog to
	// issue soft / hard warnings before the model actually hits the
	// API's "context_length_exceeded" 400. Zero (the zero-value /
	// absent form) means "unknown" — downstream consumers must
	// gracefully degrade (fall back to absolute byte caps, skip
	// pressure tracking) so a legacy providers.yaml keeps working
	// byte-identically. Per-agent override: leaving the field zero
	// on an agent-level entry inherits the default-level value.
	ContextWindow int `yaml:"context_window"`

	// Stream enables SSE streaming of the chat completion response.
	// When true the adapter sets `stream: true` on the wire, parses
	// server-sent events, and accumulates content + tool_calls into
	// the same Response shape a non-streaming call produces. Runtime-
	// observable difference: intermediate content chunks surface in
	// the REPL task-row preview and the /chat live typewriter view
	// while the call is still in flight.
	//
	// Default: ON. nil resolves to true in factory.NewFromConfig so
	// new deployments get streaming UX without touching providers.
	// yaml. Operators who want the classic single-shot behaviour set
	// `stream: false` explicitly (at default level or per-agent).
	// Per-agent override uses the same nil-sentinel pattern as
	// ThinkAloud — an agent-level nil inherits from default.
	Stream *bool `yaml:"stream"`

	// TLSCAFile is an optional path to a PEM-encoded CA bundle that gets
	// appended to the system trust pool. Useful when the endpoint is
	// signed by a corporate / self-hosted CA the OS does not know
	// about. Empty = system trust pool only (default).
	TLSCAFile string `yaml:"tls_ca_file"`

	// TLSInsecureSkipVerify disables TLS certificate validation
	// entirely. THIS IS A NUCLEAR OPTION — any network attacker on
	// path can MITM the request and steal the API key. Only use it for
	// short-lived debugging against an endpoint you fully control, and
	// prefer TLSCAFile whenever possible. When true, codrax logs a
	// high-visibility warning on startup so the state is never silent.
	TLSInsecureSkipVerify bool `yaml:"tls_insecure_skip_verify"`
}
