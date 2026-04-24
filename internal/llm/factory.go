package llm

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
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
	return NewOpenAIAdapter(cfg.APIKey, cfg.Model, cfg.BaseURL, stream, cfg.ContextWindow, TLSOptions{
		CAFile:             cfg.TLSCAFile,
		InsecureSkipVerify: cfg.TLSInsecureSkipVerify,
	}), nil
}
