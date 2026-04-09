package llm

import "github.com/hanchaoqun/design/internal/types"

// NewFromConfig creates an Adapter from a resolved provider config.
// Returns nil if no provider is configured (no api_key, no provider).
func NewFromConfig(cfg types.LLMProviderConfig) Adapter {
	switch cfg.Provider {
	case "openai":
		if cfg.APIKey == "" {
			return nil
		}
		model := cfg.Model
		if model == "" {
			model = "gpt-4o"
		}
		return NewOpenAIAdapter(cfg.APIKey, model, cfg.BaseURL)
	default:
		return nil
	}
}
