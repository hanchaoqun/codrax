package config

import (
	"os"

	"github.com/hanchaoqun/codrax/internal/types"
	"gopkg.in/yaml.v3"
)

// LoadProviders loads the providers.yaml config file.
// Returns a zero-value config (not an error) if the file does not exist,
// so the caller can fall back to environment variables.
func LoadProviders(path string) (*types.ProvidersConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &types.ProvidersConfig{}, nil
		}
		return nil, err
	}

	var cfg types.ProvidersConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ResolveProvider returns the effective LLM provider config for a given agent.
// Merge order: agent-level → default-level → environment variables.
func ResolveProvider(cfg *types.ProvidersConfig, agentName string) types.LLMProviderConfig {
	base := cfg.LLM.Default
	if ac, ok := cfg.LLM.Agents[agentName]; ok {
		merge(&base, &ac)
	}
	mergeEnv(&base)
	return base
}

// merge overlays non-empty fields from src onto dst.
func merge(dst, src *types.LLMProviderConfig) {
	if src.Provider != "" {
		dst.Provider = src.Provider
	}
	if src.APIKey != "" {
		dst.APIKey = src.APIKey
	}
	if src.Model != "" {
		dst.Model = src.Model
	}
	if src.BaseURL != "" {
		dst.BaseURL = src.BaseURL
	}
	if src.ThinkAloud != nil {
		dst.ThinkAloud = src.ThinkAloud
	}
	if src.Stream != nil {
		dst.Stream = src.Stream
	}
	if src.TLSCAFile != "" {
		dst.TLSCAFile = src.TLSCAFile
	}
	// ContextWindow uses the "non-zero overrides" pattern: an agent-
	// level entry that leaves context_window at zero inherits the
	// default. An agent pointing at a smaller model than default would
	// declare its own context_window explicitly.
	if src.ContextWindow != 0 {
		dst.ContextWindow = src.ContextWindow
	}
	// TLSInsecureSkipVerify is a bool, not a pointer, so it has no
	// nil sentinel to distinguish "agent explicitly set false" from
	// "agent didn't mention it." An agent-level true wins over a
	// default-level false (security escape should be opt-in, not
	// silently inherited); an agent-level false cannot turn off a
	// default-level true (the operator who set true at default level
	// did so on purpose, so agents inherit the looser setting).
	if src.TLSInsecureSkipVerify {
		dst.TLSInsecureSkipVerify = true
	}
}

// mergeEnv fills empty fields from environment variables.
func mergeEnv(cfg *types.LLMProviderConfig) {
	if cfg.Provider == "" {
		if v := os.Getenv("LLM_PROVIDER"); v != "" {
			cfg.Provider = v
		}
	}
	if cfg.APIKey == "" {
		if v := os.Getenv("OPENAI_API_KEY"); v != "" {
			cfg.APIKey = v
		}
	}
	if cfg.Model == "" {
		if v := os.Getenv("OPENAI_MODEL"); v != "" {
			cfg.Model = v
		}
	}
	if cfg.BaseURL == "" {
		if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
			cfg.BaseURL = v
		}
	}
}
