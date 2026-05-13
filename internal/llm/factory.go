package llm

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/config"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Code defaults for the operator-tunable HTTP-side knobs. There is
// NO code default for max_output_tokens — zero (the absent form) means
// "let the server use the model's own ceiling", which is what every
// other LLM client does and what we want by default. Capping output
// client-side is opt-in via providers.yaml::max_output_tokens.
const (
	defaultRequestTimeoutSeconds         = 120
	defaultRetryMaxAttempts              = 6
	defaultStreamStallTimeoutSeconds     = 60
	defaultStreamFirstByteTimeoutSeconds = 20
)

// NewFromConfig creates an Adapter from a resolved provider config.
// Returns (nil, err) when required fields are missing so the caller
// can surface the exact reason to the user — the old "silently
// return nil and fall back to a placeholder" behavior hid missing
// provider credentials behind an unrelated-looking runtime error.
//
// Fields an operator MUST set (no silent defaults):
//
//   - provider   — only "openai" is supported today
//   - api_key    — no sensible fallback
//   - model      — silently defaulting to a hard-coded model ID was
//     dangerous for users on internal / Ollama / Azure deployments
//     where the hosted model list is completely different
//   - base_url   — same reason; defaulting to api.openai.com/v1
//     silently steered corporate / private deployments onto the
//     public endpoint
func NewFromConfig(cfg types.LLMProviderConfig) (Adapter, error) {
	if cfg.Provider == "" {
		return nil, fmt.Errorf("providers.yaml: llm.default.provider is required")
	}
	switch cfg.Provider {
	case "openai":
	default:
		return nil, fmt.Errorf("providers.yaml: unknown provider %q (supported: openai)", cfg.Provider)
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("providers.yaml: llm api_key is required (inherit from default or set per-agent)")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("providers.yaml: llm model is required — no default model is assumed")
	}
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("providers.yaml: llm base_url is required — no default endpoint is assumed")
	}
	// Streaming defaults to ON. An unset `stream` field (cfg.Stream ==
	// nil) resolves to true, so new deployments get the live-output
	// UX (REPL task row preview, /chat typewriter replies) without
	// touching providers.yaml. Operators who need the classic single-
	// shot behaviour must set `stream: false` explicitly — at either
	// the default or per-agent level. Providers that silently return
	// SSE even with `stream: false` on the wire are still handled by
	// the JSON-path SSE auto-sniffer in openai.go, so this default
	// flip cannot make any previously-working provider stop working.
	stream := true
	if cfg.Stream != nil {
		stream = *cfg.Stream
	}
	recoverTextToolCalls := false
	if cfg.RecoverTextToolCalls != nil {
		recoverTextToolCalls = *cfg.RecoverTextToolCalls
	}

	// Resolve the three operator-tunable sizing knobs. Symmetric to
	// the input-side resolution chain (ContextWindow + fraction-form
	// blob caps): yaml fraction → yaml absolute → code default.
	//
	// max_output_tokens explicitly defaults to ZERO — that means we
	// do NOT send `max_tokens` on the wire and the server uses the
	// model's own output ceiling. This is the right default because
	// every other LLM client (sdk, IDE assistant, langchain, etc.)
	// works this way; capping output client-side was the root cause
	// of the emit_change_plan streaming-truncation failures.
	maxOutputTokens := config.ResolveTokenBudget(
		cfg.MaxOutputFraction,
		cfg.MaxOutputTokens,
		0, // code default = no client-side cap
		cfg.ContextWindow,
	)
	requestTimeout := config.ResolveDurationSeconds(
		cfg.RequestTimeoutSeconds,
		defaultRequestTimeoutSeconds,
	)
	retryMaxAttempts := config.ResolveRetryAttempts(
		cfg.RetryMaxAttempts,
		defaultRetryMaxAttempts,
	)
	streamStallTimeout := config.ResolveDurationSeconds(
		cfg.StreamStallTimeoutSeconds,
		defaultStreamStallTimeoutSeconds,
	)
	streamFirstByteTimeout := config.ResolveDurationSeconds(
		cfg.StreamFirstByteTimeoutSeconds,
		defaultStreamFirstByteTimeoutSeconds,
	)

	return NewOpenAIAdapter(cfg.APIKey, cfg.Model, cfg.BaseURL, AdapterOptions{
		Stream:                 stream,
		RecoverTextToolCalls:   recoverTextToolCalls,
		ContextWindow:          cfg.ContextWindow,
		MaxOutputTokens:        maxOutputTokens,
		RequestTimeout:         requestTimeout,
		RetryMaxAttempts:       retryMaxAttempts,
		StreamStallTimeout:     streamStallTimeout,
		StreamFirstByteTimeout: streamFirstByteTimeout,
		TLS: TLSOptions{
			CAFile:             cfg.TLSCAFile,
			InsecureSkipVerify: cfg.TLSInsecureSkipVerify,
		},
	}), nil
}
