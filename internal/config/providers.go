package config

import (
	"fmt"
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
	// PIB-7: resolve !command / $VAR credential references once at
	// load, BEFORE per-agent inheritance (each declared !command runs
	// exactly once; merge's non-empty check never sees an unresolved
	// "$VAR" literal). A declared-but-unresolvable reference is a
	// configuration error — fail loud here beats a provider-side 401
	// with a literal "$VAR" key.
	if err := resolveProviderSecrets(&cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &cfg, nil
}

// ResolveProvider returns the effective LLM provider config for a given agent.
// Merge order: agent-level → default-level → environment variables.
func ResolveProvider(cfg *types.ProvidersConfig, agentName string) types.LLMProviderConfig {
	base := cfg.LLM.Default
	base.Auth = cloneAuthConfig(base.Auth)
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
	if src.ChatCompletionsPath != "" {
		dst.ChatCompletionsPath = src.ChatCompletionsPath
	}
	if src.ModelsPath != "" {
		dst.ModelsPath = src.ModelsPath
	}
	if src.Auth != nil {
		dst.Auth = mergeAuthConfig(dst.Auth, src.Auth)
	}
	if src.Headers != nil {
		dst.Headers = mergeStringMap(dst.Headers, src.Headers)
	}
	if src.RequestExtra != nil {
		dst.RequestExtra = mergeAnyMap(dst.RequestExtra, src.RequestExtra)
	}
	if src.ThinkAloud != nil {
		dst.ThinkAloud = src.ThinkAloud
	}
	if src.ThinkingMode != "" {
		dst.ThinkingMode = src.ThinkingMode
	}
	if src.RecoverTextToolCalls != nil {
		dst.RecoverTextToolCalls = src.RecoverTextToolCalls
	}
	if src.ToolParamCompat != nil {
		copied := *src.ToolParamCompat
		dst.ToolParamCompat = &copied
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
	// Output-side and HTTP-side sizing fields all use the same
	// non-zero-overrides pattern as ContextWindow. An agent-level
	// entry that leaves any field at zero / nil inherits the default;
	// to opt out (e.g. force a smaller cap on a cheap classifier),
	// the agent must set the field explicitly to a positive value.
	if src.MaxOutputTokens != 0 {
		dst.MaxOutputTokens = src.MaxOutputTokens
	}
	if src.MaxOutputFraction != nil {
		dst.MaxOutputFraction = src.MaxOutputFraction
	}
	if src.RequestTimeoutSeconds != 0 {
		dst.RequestTimeoutSeconds = src.RequestTimeoutSeconds
	}
	if src.RetryMaxAttempts != 0 {
		dst.RetryMaxAttempts = src.RetryMaxAttempts
	}
	if src.StreamStallTimeoutSeconds != 0 {
		dst.StreamStallTimeoutSeconds = src.StreamStallTimeoutSeconds
	}
	if src.StreamFirstByteTimeoutSeconds != 0 {
		dst.StreamFirstByteTimeoutSeconds = src.StreamFirstByteTimeoutSeconds
	}
}

func cloneAuthConfig(src *types.LLMAuthConfig) *types.LLMAuthConfig {
	if src == nil {
		return nil
	}
	copied := *src
	copied.InvalidTokenErrorCodes = append([]string(nil), src.InvalidTokenErrorCodes...)
	return &copied
}

func mergeAuthConfig(dst, src *types.LLMAuthConfig) *types.LLMAuthConfig {
	out := cloneAuthConfig(dst)
	if out == nil {
		out = &types.LLMAuthConfig{}
	}
	if src.Mode != "" {
		out.Mode = src.Mode
	}
	if src.AuthBaseURL != "" {
		out.AuthBaseURL = src.AuthBaseURL
	}
	if src.ClientID != "" {
		out.ClientID = src.ClientID
	}
	if src.Scope != "" {
		out.Scope = src.Scope
	}
	if src.ResponseType != "" {
		out.ResponseType = src.ResponseType
	}
	if src.ScopeResource != "" {
		out.ScopeResource = src.ScopeResource
	}
	if src.AuthorizePath != "" {
		out.AuthorizePath = src.AuthorizePath
	}
	if src.CallbackPath != "" {
		out.CallbackPath = src.CallbackPath
	}
	if src.TokenPath != "" {
		out.TokenPath = src.TokenPath
	}
	if src.TokenCacheFile != "" {
		out.TokenCacheFile = src.TokenCacheFile
	}
	if src.PollTimeoutSeconds != 0 {
		out.PollTimeoutSeconds = src.PollTimeoutSeconds
	}
	if src.PollIntervalSeconds != 0 {
		out.PollIntervalSeconds = src.PollIntervalSeconds
	}
	if src.RefreshBeforeSeconds != 0 {
		out.RefreshBeforeSeconds = src.RefreshBeforeSeconds
	}
	if src.TokenTTLSeconds != 0 {
		out.TokenTTLSeconds = src.TokenTTLSeconds
	}
	if src.AccessTokenHeader != "" {
		out.AccessTokenHeader = src.AccessTokenHeader
	}
	if src.AccessTokenFormat != "" {
		out.AccessTokenFormat = src.AccessTokenFormat
	}
	if len(src.InvalidTokenErrorCodes) > 0 {
		out.InvalidTokenErrorCodes = append([]string(nil), src.InvalidTokenErrorCodes...)
	}
	if src.Ambiguous401PreserveDisk != nil {
		out.Ambiguous401PreserveDisk = src.Ambiguous401PreserveDisk
	}
	if src.Ambiguous401Escalation != 0 {
		out.Ambiguous401Escalation = src.Ambiguous401Escalation
	}
	return out
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

func mergeStringMap(dst, src map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range dst {
		out[k] = v
	}
	for k, v := range src {
		out[k] = v
	}
	return out
}

func mergeAnyMap(dst, src map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range dst {
		out[k] = v
	}
	for k, v := range src {
		out[k] = v
	}
	return out
}
