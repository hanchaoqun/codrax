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
}
