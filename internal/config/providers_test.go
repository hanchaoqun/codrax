package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestResolveProvider_ContextWindow_DefaultInherits pins the zero-sentinel
// merge rule: an agent-level entry that leaves context_window unset
// inherits the default-level value. This matches the "non-empty wins"
// pattern used for every other per-agent override and keeps the typical
// deploy pattern (one default window declared once, all agents share it)
// working without per-agent duplication.
func TestResolveProvider_ContextWindow_DefaultInherits(t *testing.T) {
	cfg := &types.ProvidersConfig{
		LLM: types.LLMProvidersConfig{
			Default: types.LLMProviderConfig{
				Provider: "openai", APIKey: "k", Model: "big", BaseURL: "u",
				ContextWindow: 200000,
			},
			Agents: map[string]types.LLMProviderConfig{
				// Agent overrides model but NOT context_window.
				"analyzer": {Model: "big"},
			},
		},
	}
	resolved := ResolveProvider(cfg, "analyzer")
	if resolved.ContextWindow != 200000 {
		t.Errorf("agent without explicit context_window should inherit default; got %d", resolved.ContextWindow)
	}
}

// TestResolveProvider_ContextWindow_AgentOverride covers the explicit-
// agent-override case: when an agent points at a smaller (or larger)
// model, it declares its own context_window and that value wins.
// Canonical use case: route the cheap memory_summarizer at a 32K
// model while the main analyzer runs on a 200K one.
func TestResolveProvider_ContextWindow_AgentOverride(t *testing.T) {
	cfg := &types.ProvidersConfig{
		LLM: types.LLMProvidersConfig{
			Default: types.LLMProviderConfig{
				Provider: "openai", APIKey: "k", Model: "big", BaseURL: "u",
				ContextWindow: 200000,
			},
			Agents: map[string]types.LLMProviderConfig{
				"memory_summarizer": {
					Model:         "small",
					ContextWindow: 32000,
				},
			},
		},
	}
	resolved := ResolveProvider(cfg, "memory_summarizer")
	if resolved.ContextWindow != 32000 {
		t.Errorf("explicit agent context_window must win; got %d", resolved.ContextWindow)
	}
	if resolved.Model != "small" {
		t.Errorf("agent model override sanity check failed: got %q", resolved.Model)
	}
}

// TestResolveProvider_ContextWindow_AbsentLeavesZero confirms the
// "unknown" sentinel: a providers.yaml that omits context_window at
// every level (legacy deployments) yields 0, letting downstream
// consumers fall back to absolute byte caps / skip pressure tracking.
func TestResolveProvider_ContextWindow_AbsentLeavesZero(t *testing.T) {
	cfg := &types.ProvidersConfig{
		LLM: types.LLMProvidersConfig{
			Default: types.LLMProviderConfig{
				Provider: "openai", APIKey: "k", Model: "x", BaseURL: "u",
			},
		},
	}
	resolved := ResolveProvider(cfg, "analyzer")
	if resolved.ContextWindow != 0 {
		t.Errorf("absent context_window should resolve to zero sentinel; got %d", resolved.ContextWindow)
	}
}

// TestLoadProviders_ContextWindow_RoundTrip locks the yaml tag so
// renaming the field silently would be a compile-time break (field
// missing from the anonymous struct literal) rather than a silent
// "nobody reads this key any more" failure.
func TestLoadProviders_ContextWindow_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.yaml")
	content := `llm:
  default:
    provider: openai
    api_key: k
    model: m
    base_url: u
    context_window: 131072
  agents:
    memory_summarizer:
      model: small
      context_window: 8192
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("seed yaml: %v", err)
	}
	cfg, err := LoadProviders(path)
	if err != nil {
		t.Fatalf("LoadProviders: %v", err)
	}
	if cfg.LLM.Default.ContextWindow != 131072 {
		t.Errorf("default.context_window = %d, want 131072", cfg.LLM.Default.ContextWindow)
	}
	if cfg.LLM.Agents["memory_summarizer"].ContextWindow != 8192 {
		t.Errorf("agents.memory_summarizer.context_window = %d, want 8192",
			cfg.LLM.Agents["memory_summarizer"].ContextWindow)
	}
}
