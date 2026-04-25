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

// TestResolveProvider_OutputAndHTTPSizing_Inheritance covers the four
// new operator-tunable fields (max_output_tokens, max_output_fraction,
// request_timeout_seconds, retry_max_attempts). All four follow the
// non-zero-overrides pattern: an agent that leaves the field absent
// inherits the default; an agent that sets a positive value overrides.
// MaxOutputFraction is pointer-typed so absence vs explicit-zero is
// distinguishable, mirroring BlobMaxInlineFraction.
func TestResolveProvider_OutputAndHTTPSizing_Inheritance(t *testing.T) {
	frac := 0.25
	defaultFrac := 0.10
	cfg := &types.ProvidersConfig{
		LLM: types.LLMProvidersConfig{
			Default: types.LLMProviderConfig{
				Provider: "openai", APIKey: "k", Model: "big", BaseURL: "u",
				ContextWindow:         200000,
				MaxOutputTokens:       8192,
				MaxOutputFraction:     &defaultFrac,
				RequestTimeoutSeconds: 120,
				RetryMaxAttempts:      6,
			},
			Agents: map[string]types.LLMProviderConfig{
				// planner overrides max_output via fraction; otherwise inherits.
				"planner": {
					MaxOutputFraction:     &frac,
					RequestTimeoutSeconds: 600,
				},
				// analyzer inherits everything.
				"analyzer": {Model: "big"},
			},
		},
	}

	t.Run("agent override wins for set fields", func(t *testing.T) {
		got := ResolveProvider(cfg, "planner")
		if got.MaxOutputFraction == nil || *got.MaxOutputFraction != 0.25 {
			t.Errorf("planner.max_output_fraction = %v, want 0.25", got.MaxOutputFraction)
		}
		if got.RequestTimeoutSeconds != 600 {
			t.Errorf("planner.request_timeout_seconds = %d, want 600", got.RequestTimeoutSeconds)
		}
		// Unset fields fall through to default.
		if got.MaxOutputTokens != 8192 {
			t.Errorf("planner.max_output_tokens fallthrough = %d, want 8192", got.MaxOutputTokens)
		}
		if got.RetryMaxAttempts != 6 {
			t.Errorf("planner.retry_max_attempts fallthrough = %d, want 6", got.RetryMaxAttempts)
		}
	})

	t.Run("agent inherits everything when fields absent", func(t *testing.T) {
		got := ResolveProvider(cfg, "analyzer")
		if got.MaxOutputTokens != 8192 {
			t.Errorf("analyzer.max_output_tokens = %d, want 8192", got.MaxOutputTokens)
		}
		if got.MaxOutputFraction == nil || *got.MaxOutputFraction != 0.10 {
			t.Errorf("analyzer.max_output_fraction = %v, want 0.10", got.MaxOutputFraction)
		}
		if got.RequestTimeoutSeconds != 120 {
			t.Errorf("analyzer.request_timeout_seconds = %d, want 120", got.RequestTimeoutSeconds)
		}
		if got.RetryMaxAttempts != 6 {
			t.Errorf("analyzer.retry_max_attempts = %d, want 6", got.RetryMaxAttempts)
		}
	})

	t.Run("absent everywhere yields zero sentinels (let factory default)", func(t *testing.T) {
		bare := &types.ProvidersConfig{
			LLM: types.LLMProvidersConfig{
				Default: types.LLMProviderConfig{
					Provider: "openai", APIKey: "k", Model: "x", BaseURL: "u",
				},
			},
		}
		got := ResolveProvider(bare, "analyzer")
		if got.MaxOutputTokens != 0 || got.MaxOutputFraction != nil ||
			got.RequestTimeoutSeconds != 0 || got.RetryMaxAttempts != 0 {
			t.Errorf("absent fields should remain zero/nil for factory to apply code defaults; got %+v", got)
		}
	})
}
