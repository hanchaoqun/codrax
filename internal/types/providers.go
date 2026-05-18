package types

import "strings"

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

	// RecoverTextToolCalls enables a narrow compatibility shim for
	// OpenAI-compatible providers that receive a tool catalog but
	// serialize tool-call envelopes into assistant content instead of
	// the protocol-level tool_calls field. nil = inherit from default;
	// explicit true/false wins. Default when absent everywhere is
	// false so fully compliant providers keep byte-identical behavior.
	RecoverTextToolCalls *bool `yaml:"recover_text_tool_calls"`

	// ToolParamCompat controls the schema-aware tool parameter compatibility
	// layer that runs after protocol/text tool-call recovery but before
	// local tool execution. nil = inherit from default; when absent at every
	// provider level, the CLI runtime injects DefaultToolParamCompatConfig.
	ToolParamCompat *ToolParamCompatConfig `yaml:"tool_param_compat"`

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

	// MaxOutputTokens is the upper bound on tokens the server is asked
	// to generate in a single chat completion (`max_tokens` on the
	// wire). Symmetric to ContextWindow on the input side. Zero (the
	// absent / zero-value form) means "fall back to the fraction form,
	// then the code default." A non-zero value here always wins over
	// MaxOutputFraction.
	//
	// The historical hard-coded 4096 cap silently truncated long
	// emit_change_plan / write-mode payloads on every model — observed
	// failure mode: the planner streams `{"changes": ` and stops, the
	// LLM has no clue why because finish_reason=length never reached
	// it, and the dispatch loop burns its full retry budget. Making
	// this configurable closes that hole without coupling the cap to
	// any specific provider's docs.
	//
	// Per-agent override uses the same non-zero-overrides rule as
	// ContextWindow: an agent-level entry leaving the field at zero
	// inherits the default. Agents that emit large structured payloads
	// (planner / coder / verifier) typically declare their own here
	// or via MaxOutputFraction.
	MaxOutputTokens int `yaml:"max_output_tokens"`

	// MaxOutputFraction is the fraction-form twin of MaxOutputTokens,
	// mirroring the BlobMaxInlineFraction / AgentMaxToolHistoryFraction
	// pattern. When this is positive AND ContextWindow is positive,
	// the effective output cap is `ContextWindow * fraction` (tokens —
	// no BytesPerToken multiplier; this is a token budget, not a byte
	// budget). Falls back to MaxOutputTokens, then the code default.
	//
	// Pointer type so the merge logic can distinguish "absent" from
	// "explicit zero." A nil pointer or zero value disables the
	// fraction form on this entry.
	MaxOutputFraction *float64 `yaml:"max_output_fraction"`

	// RequestTimeoutSeconds is the per-call HTTP timeout for chat
	// completion requests. Long emit_change_plan generations on slow
	// providers can take several minutes; the legacy hard-coded 120s
	// was tuned for short single-shot stages and silently aborted long
	// streams. Zero (the absent form) inherits the code default (240s).
	// Per-agent override uses the same non-zero-overrides rule.
	RequestTimeoutSeconds int `yaml:"request_timeout_seconds"`

	// RetryMaxAttempts is the maximum number of HTTP attempts (initial
	// + retries) for transient 429 / 5xx errors. The retry schedule
	// itself (exponential backoff base = 2s) is fixed in code; only
	// the attempt count is operator-tunable. Zero (the absent form)
	// inherits the code default (6 attempts ≈ 62s of total backoff
	// coverage).
	RetryMaxAttempts int `yaml:"retry_max_attempts"`

	// StreamStallTimeoutSeconds caps the maximum time the SSE scanner
	// may go without receiving a single byte before the watchdog
	// aborts the request. Tuned per-provider — fast small models can
	// use 15-30s, deep-thinking / reasoning models often need 90-
	// 120s because they pause 60+ s between thinking blocks. Zero
	// (the absent form) inherits the code default (120s).
	StreamStallTimeoutSeconds int `yaml:"stream_stall_timeout_seconds"`

	// StreamFirstByteTimeoutSeconds caps the maximum time between
	// "request accepted (200 OK)" and the FIRST SSE chunk. Distinct
	// from StreamStallTimeoutSeconds: covers the dead-on-arrival
	// case where a provider hangs before emitting any bytes (server
	// deadlock, middlebox interference, model genuinely stuck before
	// its first token). Healthy providers serve first-byte in 100-
	// 500ms; a 20s default fails fast on dead requests without
	// compromising legitimate slow first-byte paths. Zero inherits
	// the code default.
	StreamFirstByteTimeoutSeconds int `yaml:"stream_first_byte_timeout_seconds"`
}

const (
	ToolParamCompatOff    = "off"
	ToolParamCompatAudit  = "audit"
	ToolParamCompatRepair = "repair"
)

// ToolParamCompatConfig is a provider-scoped compatibility policy for models
// that emit protocol-level tool calls with mechanically wrong JSON value types
// (for example "offset":"140" where the schema says integer). It is
// intentionally separate from RecoverTextToolCalls: that knob recovers missing
// protocol tool_calls from assistant text, while this knob normalizes
// already-present tool_call arguments against the tool schema.
type ToolParamCompatConfig struct {
	// Mode is one of off, audit, repair. Empty means off for an explicitly
	// supplied policy; the CLI runtime injects DefaultToolParamCompatConfig
	// when the provider tree omits tool_param_compat entirely.
	Mode string `yaml:"mode"`

	// SplitStringArrays controls the conservative string -> []string rule for
	// array-of-string fields. It is deliberately opt-in because a delimited
	// prose string is less lossless than scalar type parsing or JSON-string
	// unwrapping.
	SplitStringArrays *bool `yaml:"split_string_arrays"`
}

// DefaultToolParamCompatConfig is the production runtime default. It enables
// only schema-proven equivalence repairs; the less lossless delimited-string
// array split remains opt-in via split_string_arrays: true.
func DefaultToolParamCompatConfig() ToolParamCompatConfig {
	splitStringArrays := false
	return ToolParamCompatConfig{
		Mode:              ToolParamCompatRepair,
		SplitStringArrays: &splitStringArrays,
	}
}

func (c ToolParamCompatConfig) NormalizedMode() string {
	switch strings.ToLower(strings.TrimSpace(c.Mode)) {
	case "", ToolParamCompatOff:
		return ToolParamCompatOff
	case ToolParamCompatAudit:
		return ToolParamCompatAudit
	case ToolParamCompatRepair:
		return ToolParamCompatRepair
	default:
		return ""
	}
}

func (c ToolParamCompatConfig) Enabled() bool {
	mode := c.NormalizedMode()
	return mode == ToolParamCompatAudit || mode == ToolParamCompatRepair
}

func (c ToolParamCompatConfig) SplitStringArraysEnabled() bool {
	if c.SplitStringArrays == nil {
		return false
	}
	return *c.SplitStringArrays
}
