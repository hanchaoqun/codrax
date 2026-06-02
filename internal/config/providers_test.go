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

func TestResolveProvider_RequestDecoration_Inheritance(t *testing.T) {
	cfg := &types.ProvidersConfig{
		LLM: types.LLMProvidersConfig{
			Default: types.LLMProviderConfig{
				Provider:            "openai",
				APIKey:              "k",
				BaseURL:             "https://example.test/base",
				ChatCompletionsPath: "/chat/completions",
				ModelsPath:          "/models",
				Auth: &types.LLMAuthConfig{
					Mode:              "oauth2_polling",
					AuthBaseURL:       "https://auth.example.test",
					ClientID:          "client",
					Scope:             "scope-a",
					TokenPath:         "/oauth/getToken",
					AccessTokenHeader: "X-Auth-Token",
				},
				Headers: map[string]string{
					"app-id":         "GatewayApp",
					"x-snap-traceid": "@uuid_v4",
				},
				RequestExtra: map[string]any{
					"queue":       true,
					"tool_stream": true,
				},
			},
			Agents: map[string]types.LLMProviderConfig{
				"finalizer": {
					Model: "strong",
					Headers: map[string]string{
						"app-id": "CustomApp",
					},
					RequestExtra: map[string]any{
						"queue": false,
					},
				},
			},
		},
	}

	got := ResolveProvider(cfg, "finalizer")
	if got.ChatCompletionsPath != "/chat/completions" {
		t.Fatalf("chat_completions_path should inherit, got %q", got.ChatCompletionsPath)
	}
	if got.ModelsPath != "/models" {
		t.Fatalf("models_path should inherit, got %q", got.ModelsPath)
	}
	if got.Auth == nil || got.Auth.Mode != "oauth2_polling" || got.Auth.AccessTokenHeader != "X-Auth-Token" {
		t.Fatalf("auth should inherit as a copied struct, got %+v", got.Auth)
	}
	if got.Headers["app-id"] != "CustomApp" {
		t.Fatalf("agent header should override default app-id, got %+v", got.Headers)
	}
	if got.Headers["x-snap-traceid"] != "@uuid_v4" {
		t.Fatalf("default generated header should survive merge, got %+v", got.Headers)
	}
	if got.RequestExtra["queue"] != false {
		t.Fatalf("agent request_extra should override queue, got %v", got.RequestExtra["queue"])
	}
	if got.RequestExtra["tool_stream"] != true {
		t.Fatalf("default request_extra should survive merge, got %v", got.RequestExtra["tool_stream"])
	}
}

func TestLoadProviders_OAuthPolling_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.yaml")
	content := `llm:
  default:
    provider: openai
    base_url: https://gateway.example.test/api
    chat_completions_path: /chat/completions
    models_path: /models
    auth:
      mode: oauth2_polling
      auth_base_url: https://auth.example.test/oauth
      client_id: com.example.client
      scope: "scope-a"
      response_type: code
      scope_resource: resource-a
      authorize_path: /oauth2/authorize
      callback_path: /oauth/callback
      token_path: /oauth/getToken
      access_token_header: X-Auth-Token
      access_token_format: "{token}"
      poll_timeout_seconds: 1800
      poll_interval_seconds: 1
      refresh_before_seconds: 300
    headers:
      app-id: GatewayApp
      x-snap-traceid: "@uuid_v4"
    request_extra:
      queue: true
      tool_stream: true
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("seed yaml: %v", err)
	}
	cfg, err := LoadProviders(path)
	if err != nil {
		t.Fatalf("LoadProviders: %v", err)
	}
	got := cfg.LLM.Default
	if got.ChatCompletionsPath != "/chat/completions" || got.ModelsPath != "/models" {
		t.Fatalf("paths did not round-trip: %+v", got)
	}
	if got.Auth == nil || got.Auth.Mode != "oauth2_polling" || got.Auth.RefreshBeforeSeconds != 300 {
		t.Fatalf("auth did not round-trip: %+v", got.Auth)
	}
	if got.Headers["x-snap-traceid"] != "@uuid_v4" {
		t.Fatalf("headers did not round-trip: %+v", got.Headers)
	}
	if got.RequestExtra["queue"] != true || got.RequestExtra["tool_stream"] != true {
		t.Fatalf("request_extra did not round-trip: %+v", got.RequestExtra)
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

func TestResolveProvider_RecoverTextToolCalls_Inheritance(t *testing.T) {
	enabled := true
	disabled := false
	cfg := &types.ProvidersConfig{
		LLM: types.LLMProvidersConfig{
			Default: types.LLMProviderConfig{
				Provider: "openai", APIKey: "k", Model: "big", BaseURL: "u",
				RecoverTextToolCalls: &enabled,
			},
			Agents: map[string]types.LLMProviderConfig{
				"analyzer": {},
				"finalizer": {
					RecoverTextToolCalls: &disabled,
				},
			},
		},
	}

	inherited := ResolveProvider(cfg, "analyzer")
	if inherited.RecoverTextToolCalls == nil || !*inherited.RecoverTextToolCalls {
		t.Fatalf("analyzer should inherit recover_text_tool_calls=true, got %v", inherited.RecoverTextToolCalls)
	}
	overridden := ResolveProvider(cfg, "finalizer")
	if overridden.RecoverTextToolCalls == nil || *overridden.RecoverTextToolCalls {
		t.Fatalf("finalizer explicit false should override default true, got %v", overridden.RecoverTextToolCalls)
	}
	bare := ResolveProvider(&types.ProvidersConfig{
		LLM: types.LLMProvidersConfig{Default: types.LLMProviderConfig{Provider: "openai", APIKey: "k", Model: "x", BaseURL: "u"}},
	}, "analyzer")
	if bare.RecoverTextToolCalls != nil {
		t.Fatalf("absent recover_text_tool_calls should remain nil so factory defaults false, got %v", bare.RecoverTextToolCalls)
	}
}

func TestResolveProvider_ThinkingMode_Inheritance(t *testing.T) {
	cfg := &types.ProvidersConfig{
		LLM: types.LLMProvidersConfig{
			Default: types.LLMProviderConfig{
				Provider: "openai", APIKey: "k", Model: "deepseek-v4-flash", BaseURL: "https://api.deepseek.com",
				ThinkingMode: "disabled",
			},
			Agents: map[string]types.LLMProviderConfig{
				"analyzer": {},
				"finalizer": {
					ThinkingMode: "provider_default",
				},
			},
		},
	}

	inherited := ResolveProvider(cfg, "analyzer")
	if inherited.ThinkingMode != "disabled" {
		t.Fatalf("analyzer should inherit thinking_mode=disabled, got %q", inherited.ThinkingMode)
	}
	overridden := ResolveProvider(cfg, "finalizer")
	if overridden.ThinkingMode != "provider_default" {
		t.Fatalf("finalizer explicit thinking_mode should override default, got %q", overridden.ThinkingMode)
	}
	bare := ResolveProvider(&types.ProvidersConfig{
		LLM: types.LLMProvidersConfig{Default: types.LLMProviderConfig{Provider: "openai", APIKey: "k", Model: "x", BaseURL: "u"}},
	}, "analyzer")
	if bare.ThinkingMode != "" {
		t.Fatalf("absent thinking_mode should remain empty so factory/adapter auto policy applies, got %q", bare.ThinkingMode)
	}
}

func TestResolveProvider_ToolParamCompat_Inheritance(t *testing.T) {
	splitDisabled := false
	cfg := &types.ProvidersConfig{
		LLM: types.LLMProvidersConfig{
			Default: types.LLMProviderConfig{
				Provider: "openai", APIKey: "k", Model: "local", BaseURL: "u",
				ToolParamCompat: &types.ToolParamCompatConfig{Mode: types.ToolParamCompatRepair},
			},
			Agents: map[string]types.LLMProviderConfig{
				"analyzer": {},
				"finalizer": {
					ToolParamCompat: &types.ToolParamCompatConfig{
						Mode:              types.ToolParamCompatAudit,
						SplitStringArrays: &splitDisabled,
					},
				},
			},
		},
	}

	inherited := ResolveProvider(cfg, "analyzer")
	if inherited.ToolParamCompat == nil || inherited.ToolParamCompat.Mode != types.ToolParamCompatRepair {
		t.Fatalf("analyzer should inherit tool_param_compat repair mode, got %+v", inherited.ToolParamCompat)
	}
	overridden := ResolveProvider(cfg, "finalizer")
	if overridden.ToolParamCompat == nil || overridden.ToolParamCompat.Mode != types.ToolParamCompatAudit {
		t.Fatalf("finalizer explicit tool_param_compat should override default, got %+v", overridden.ToolParamCompat)
	}
	if overridden.ToolParamCompat.SplitStringArrays == nil || *overridden.ToolParamCompat.SplitStringArrays {
		t.Fatalf("finalizer split_string_arrays=false should survive merge, got %+v", overridden.ToolParamCompat)
	}
	bare := ResolveProvider(&types.ProvidersConfig{
		LLM: types.LLMProvidersConfig{Default: types.LLMProviderConfig{Provider: "openai", APIKey: "k", Model: "x", BaseURL: "u"}},
	}, "analyzer")
	if bare.ToolParamCompat != nil {
		t.Fatalf("provider merge should preserve absent tool_param_compat for runtime default injection, got %+v", bare.ToolParamCompat)
	}
}
