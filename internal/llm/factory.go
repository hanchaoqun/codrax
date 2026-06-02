package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/config"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Code defaults for the operator-tunable HTTP-side knobs. There is
// NO code default for max_output_tokens — zero (the absent form) means
// "let the server use the model's own ceiling", which is what every
// other LLM client does and what we want by default. Capping output
// client-side is opt-in via providers.yaml::max_output_tokens.
const (
	defaultRequestTimeoutSeconds         = 240
	defaultRetryMaxAttempts              = 6
	defaultStreamStallTimeoutSeconds     = 120
	defaultStreamFirstByteTimeoutSeconds = 40
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
//   - api_key    — required for the default api_key auth mode; OAuth
//     provider modes obtain credentials through their auth block
//   - model      — explicit model wins; if omitted, models_path must
//     be configured and the first listed model is selected visibly
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
	authOpts := authOptionsFromConfig(cfg, requestTimeout)
	if normalizeAuthMode(authOpts.Mode) == authModeAPIKey && cfg.APIKey == "" {
		return nil, fmt.Errorf("providers.yaml: llm api_key is required when auth.mode is empty or api_key (inherit from default or set per-agent)")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		selected, err := discoverFirstModel(cfg, authOpts, requestTimeout)
		if err != nil {
			return nil, err
		}
		model = selected
	}

	return NewOpenAIAdapter(cfg.APIKey, model, cfg.BaseURL, AdapterOptions{
		Stream:                 stream,
		RecoverTextToolCalls:   recoverTextToolCalls,
		ProviderThinkingMode:   cfg.ThinkingMode,
		ContextWindow:          cfg.ContextWindow,
		MaxOutputTokens:        maxOutputTokens,
		RequestTimeout:         requestTimeout,
		RetryMaxAttempts:       retryMaxAttempts,
		StreamStallTimeout:     streamStallTimeout,
		StreamFirstByteTimeout: streamFirstByteTimeout,
		ChatCompletionsPath:    cfg.ChatCompletionsPath,
		ModelsPath:             cfg.ModelsPath,
		Auth:                   authOpts,
		RequestHeaders:         cfg.Headers,
		RequestExtra:           cfg.RequestExtra,
		TLS: TLSOptions{
			CAFile:             cfg.TLSCAFile,
			InsecureSkipVerify: cfg.TLSInsecureSkipVerify,
		},
	}), nil
}

func authOptionsFromConfig(cfg types.LLMProviderConfig, requestTimeout time.Duration) AuthOptions {
	opts := AuthOptions{
		Mode:           authModeAPIKey,
		BaseURL:        cfg.BaseURL,
		RequestTimeout: requestTimeout,
		TLS: TLSOptions{
			CAFile:             cfg.TLSCAFile,
			InsecureSkipVerify: cfg.TLSInsecureSkipVerify,
		},
	}
	if cfg.Auth == nil {
		return opts
	}
	a := cfg.Auth
	opts.Mode = a.Mode
	opts.AuthBaseURL = a.AuthBaseURL
	opts.ClientID = a.ClientID
	opts.Scope = a.Scope
	opts.ResponseType = a.ResponseType
	opts.ScopeResource = a.ScopeResource
	opts.AuthorizePath = a.AuthorizePath
	opts.CallbackPath = a.CallbackPath
	opts.TokenPath = a.TokenPath
	opts.TokenCacheFile = a.TokenCacheFile
	opts.PollTimeout = secondsDuration(a.PollTimeoutSeconds)
	opts.PollInterval = secondsDuration(a.PollIntervalSeconds)
	opts.RefreshBefore = secondsDuration(a.RefreshBeforeSeconds)
	opts.TokenTTL = secondsDuration(a.TokenTTLSeconds)
	opts.TokenHeader = a.AccessTokenHeader
	opts.TokenFormat = a.AccessTokenFormat
	return opts
}

func secondsDuration(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

type discoveredModel struct {
	Name        string
	Description string
}

type rawProviderModel struct {
	Name string  `json:"name"`
	ID   string  `json:"id"`
	Des  *string `json:"des"`
	Desc *string `json:"description"`
}

type modelDiscoveryResult struct {
	Models   []discoveredModel
	Selected string
}

var modelDiscoveryCache = struct {
	sync.Mutex
	entries map[string]modelDiscoveryResult
}{entries: map[string]modelDiscoveryResult{}}

func discoverFirstModel(cfg types.LLMProviderConfig, authOpts AuthOptions, requestTimeout time.Duration) (string, error) {
	if strings.TrimSpace(cfg.ModelsPath) == "" {
		return "", fmt.Errorf("providers.yaml: llm model is required when models_path is not configured")
	}
	key := modelDiscoveryKey(cfg, authOpts)
	modelDiscoveryCache.Lock()
	if cached, ok := modelDiscoveryCache.entries[key]; ok {
		modelDiscoveryCache.Unlock()
		return cached.Selected, nil
	}
	modelDiscoveryCache.Unlock()

	authenticator, err := newRequestAuthenticator(cfg.APIKey, authOpts)
	if err != nil {
		return "", err
	}
	client := buildHTTPClient(authOpts.TLS, cfg.BaseURL, requestTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	var body []byte
	for authAttempt := 0; authAttempt < 2; authAttempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURLPath(cfg.BaseURL, cfg.ModelsPath), nil)
		if err != nil {
			return "", fmt.Errorf("create model-list request: %w", err)
		}
		if err := authenticator.Apply(ctx, req); err != nil {
			return "", fmt.Errorf("apply auth for model-list request: %w", err)
		}
		applyConfiguredHeaders(req, cfg.Headers)
		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("query model list: %w", err)
		}
		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return "", fmt.Errorf("read model list: %w", readErr)
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			body = respBody
			break
		}
		if isAuthStatus(resp.StatusCode) && authAttempt == 0 {
			authenticator.Invalidate()
			continue
		}
		return "", fmt.Errorf("query model list HTTP %d: %s", resp.StatusCode, trimForLog(respBody, 512))
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return "", fmt.Errorf("parse model list response: empty body from %s", cfg.ModelsPath)
	}
	models, err := parseModelList(body)
	if err != nil {
		return "", err
	}
	selected := ""
	for _, m := range models {
		if strings.TrimSpace(m.Name) != "" {
			selected = strings.TrimSpace(m.Name)
			break
		}
	}
	if selected == "" {
		return "", fmt.Errorf("providers.yaml: model list from %s was empty; set llm.default.model explicitly", cfg.ModelsPath)
	}
	result := modelDiscoveryResult{Models: models, Selected: selected}
	modelDiscoveryCache.Lock()
	modelDiscoveryCache.entries[key] = result
	modelDiscoveryCache.Unlock()
	printModelSelectionNotice(result)
	return selected, nil
}

func modelDiscoveryKey(cfg types.LLMProviderConfig, authOpts AuthOptions) string {
	return strings.Join([]string{
		cfg.BaseURL,
		cfg.ModelsPath,
		normalizeAuthMode(authOpts.Mode),
		authOpts.AuthBaseURL,
		authOpts.ClientID,
		authOpts.Scope,
		authOpts.ScopeResource,
	}, "\x00")
}

func parseModelList(body []byte) ([]discoveredModel, error) {
	var arr []rawProviderModel
	if err := json.Unmarshal(body, &arr); err == nil {
		return normalizeDiscoveredModels(arr), nil
	}
	var obj struct {
		Data []rawProviderModel `json:"data"`
	}
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, fmt.Errorf("parse model list response: %w", err)
	}
	return normalizeDiscoveredModels(obj.Data), nil
}

func normalizeDiscoveredModels(raw []rawProviderModel) []discoveredModel {
	out := make([]discoveredModel, 0, len(raw))
	for _, r := range raw {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			name = strings.TrimSpace(r.ID)
		}
		desc := ""
		if r.Des != nil {
			desc = strings.TrimSpace(*r.Des)
		}
		if desc == "" && r.Desc != nil {
			desc = strings.TrimSpace(*r.Desc)
		}
		if name != "" {
			out = append(out, discoveredModel{Name: name, Description: desc})
		}
	}
	return out
}

func printModelSelectionNotice(result modelDiscoveryResult) {
	logging.Info("[llm] model not explicitly configured; selected first listed model %q", result.Selected)
	if preferChineseDisplay() {
		fmt.Fprintln(os.Stderr, "  › 未显式配置 LLM model，已获取模型列表并选择第一个可用模型：")
		for i, m := range result.Models {
			marker := ""
			if m.Name == result.Selected {
				marker = "  ← 已选择"
			}
			if m.Description != "" {
				fmt.Fprintf(os.Stderr, "    %d. %s - %s%s\n", i+1, m.Name, m.Description, marker)
			} else {
				fmt.Fprintf(os.Stderr, "    %d. %s%s\n", i+1, m.Name, marker)
			}
		}
		return
	}
	fmt.Fprintln(os.Stderr, "  › LLM model was not explicitly configured; fetched model list:")
	for i, m := range result.Models {
		marker := ""
		if m.Name == result.Selected {
			marker = "  ← selected"
		}
		if m.Description != "" {
			fmt.Fprintf(os.Stderr, "    %d. %s - %s%s\n", i+1, m.Name, m.Description, marker)
		} else {
			fmt.Fprintf(os.Stderr, "    %d. %s%s\n", i+1, m.Name, marker)
		}
	}
}
