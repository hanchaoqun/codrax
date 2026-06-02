package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestFlexibleInt_UnmarshalStringAndNumber(t *testing.T) {
	for _, raw := range []string{`"259199"`, `259199`} {
		var got flexibleInt
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			t.Fatalf("UnmarshalJSON(%s): %v", raw, err)
		}
		if int64(got) != 259199 {
			t.Fatalf("UnmarshalJSON(%s)=%d, want 259199", raw, got)
		}
	}
}

func TestNormalizeAuthMode_LegacyOAuthAlias(t *testing.T) {
	if got := normalizeAuthMode("codeagent_oauth2"); got != authModeOAuthPolling {
		t.Fatalf("legacy alias normalized to %q, want %q", got, authModeOAuthPolling)
	}
}

func TestOAuthPolling_CachedTokenAppliesHeader(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "token.json")
	opts := AuthOptions{
		Mode:           authModeOAuthPolling,
		BaseURL:        "https://api.example.test/v1",
		AuthBaseURL:    "https://auth.example.test/oauth",
		ClientID:       "client",
		Scope:          "scope-a",
		ScopeResource:  "resource-a",
		TokenCacheFile: cache,
		TokenHeader:    "X-Auth-Token",
		TokenFormat:    "{token}",
		RequestTimeout: time.Second,
		RefreshBefore:  time.Minute,
		PollTimeout:    time.Second,
		PollInterval:   time.Millisecond,
		AuthorizePath:  defaultOAuthAuthorizePath,
		CallbackPath:   defaultOAuthCallbackPath,
		TokenPath:      defaultOAuthTokenPath,
		ResponseType:   defaultOAuthResponseType,
	}
	auth, err := newOAuthPollingAuthenticator(opts)
	if err != nil {
		t.Fatalf("newOAuthPollingAuthenticator: %v", err)
	}
	tok := cachedOAuthToken{
		AccessToken: "cached-token",
		IssuedAt:    time.Now(),
		ExpiresAt:   time.Now().Add(time.Hour),
		Fingerprint: auth.fingerprint,
	}
	if err := auth.saveCache(tok); err != nil {
		t.Fatalf("saveCache: %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://api.example.test/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.Apply(context.Background(), req); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := req.Header.Get("X-Auth-Token"); got != "cached-token" {
		t.Fatalf("X-Auth-Token=%q, want cached-token", got)
	}
	if st, err := os.Stat(cache); err != nil {
		t.Fatalf("stat cache: %v", err)
	} else if st.Mode().Perm() != 0o600 {
		t.Fatalf("cache mode=%#o, want 0600", st.Mode().Perm())
	}
}

func TestOpenAIAdapter_OAuthPollingHeadersPathAndExtras(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "token.json")
	var sawChat atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		sawChat.Store(true)
		if got := r.Header.Get("X-Auth-Token"); got != "cached-token" {
			t.Fatalf("X-Auth-Token=%q, want cached-token", got)
		}
		if got := r.Header.Get("app-id"); got != "GatewayApp" {
			t.Fatalf("app-id=%q, want GatewayApp", got)
		}
		if got := r.Header.Get("x-snap-traceid"); len(got) != len("00000000-0000-0000-0000-000000000000") {
			t.Fatalf("x-snap-traceid=%q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["model"] != "model-a" {
			t.Fatalf("model=%v, want model-a", body["model"])
		}
		if body["queue"] != true || body["tool_stream"] != true {
			t.Fatalf("request extras missing: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	cfg := types.LLMProviderConfig{
		Provider:            "openai",
		Model:               "model-a",
		BaseURL:             srv.URL,
		ChatCompletionsPath: "/chat/completions",
		Stream:              boolPtr(false),
		Auth: &types.LLMAuthConfig{
			Mode:              "oauth2_polling",
			AuthBaseURL:       srv.URL,
			ClientID:          "client",
			TokenCacheFile:    cache,
			AccessTokenHeader: "X-Auth-Token",
			AccessTokenFormat: "{token}",
		},
		Headers: map[string]string{
			"app-id":         "GatewayApp",
			"x-snap-traceid": "@uuid_v4",
		},
		RequestExtra: map[string]any{
			"queue":       true,
			"tool_stream": true,
		},
		RequestTimeoutSeconds: 5,
	}
	authOpts := authOptionsFromConfig(cfg, 5*time.Second)
	auth, err := newOAuthPollingAuthenticator(authOpts)
	if err != nil {
		t.Fatalf("newOAuthPollingAuthenticator: %v", err)
	}
	if err := auth.saveCache(cachedOAuthToken{
		AccessToken: "cached-token",
		IssuedAt:    time.Now(),
		ExpiresAt:   time.Now().Add(time.Hour),
		Fingerprint: auth.fingerprint,
	}); err != nil {
		t.Fatalf("saveCache: %v", err)
	}
	adapter, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewFromConfig: %v", err)
	}
	resp, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, ChatOptions{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "ok" || !sawChat.Load() {
		t.Fatalf("unexpected response %+v sawChat=%v", resp, sawChat.Load())
	}
}

func TestOpenAIAdapter_OAuthPollingInvalidatesOn401AndRetriesNonStream(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "token.json")
	var chatCalls atomic.Int32
	var tokenCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/getToken":
			tokenCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"fresh-token","expires_in":"259199","token_type":"Bearer","userAccount":"u"}`))
		case "/chat/completions":
			call := chatCalls.Add(1)
			if call == 1 {
				if got := r.Header.Get("X-Auth-Token"); got != "stale-token" {
					t.Fatalf("first token=%q, want stale-token", got)
				}
				http.Error(w, "expired", http.StatusUnauthorized)
				return
			}
			if got := r.Header.Get("X-Auth-Token"); got != "fresh-token" {
				t.Fatalf("second token=%q, want fresh-token", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	cfg := types.LLMProviderConfig{
		Provider:            "openai",
		Model:               "model-a",
		BaseURL:             srv.URL,
		ChatCompletionsPath: "/chat/completions",
		Stream:              boolPtr(false),
		Auth: &types.LLMAuthConfig{
			Mode:                "oauth2_polling",
			AuthBaseURL:         srv.URL,
			ClientID:            "client",
			TokenCacheFile:      cache,
			AccessTokenHeader:   "X-Auth-Token",
			PollTimeoutSeconds:  1,
			PollIntervalSeconds: 1,
		},
		RequestTimeoutSeconds: 5,
	}
	authOpts := authOptionsFromConfig(cfg, 5*time.Second)
	auth, err := newOAuthPollingAuthenticator(authOpts)
	if err != nil {
		t.Fatalf("newOAuthPollingAuthenticator: %v", err)
	}
	if err := auth.saveCache(cachedOAuthToken{
		AccessToken: "stale-token",
		IssuedAt:    time.Now(),
		ExpiresAt:   time.Now().Add(time.Hour),
		Fingerprint: auth.fingerprint,
	}); err != nil {
		t.Fatalf("saveCache: %v", err)
	}
	adapter, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewFromConfig: %v", err)
	}
	resp, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, ChatOptions{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("response=%+v", resp)
	}
	if chatCalls.Load() != 2 || tokenCalls.Load() != 1 {
		t.Fatalf("chatCalls=%d tokenCalls=%d, want 2/1", chatCalls.Load(), tokenCalls.Load())
	}
}

func TestOpenAIAdapter_OAuthPollingInvalidatesOn401AndRetriesStream(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "token.json")
	var chatCalls atomic.Int32
	var tokenCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/getToken":
			tokenCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"fresh-token","expires_in":"259199","token_type":"Bearer","userAccount":"u"}`))
		case "/chat/completions":
			call := chatCalls.Add(1)
			if call == 1 {
				if got := r.Header.Get("X-Auth-Token"); got != "stale-token" {
					t.Fatalf("first stream token=%q, want stale-token", got)
				}
				http.Error(w, "expired", http.StatusUnauthorized)
				return
			}
			if got := r.Header.Get("X-Auth-Token"); got != "fresh-token" {
				t.Fatalf("second stream token=%q, want fresh-token", got)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	cfg := types.LLMProviderConfig{
		Provider:            "openai",
		Model:               "model-a",
		BaseURL:             srv.URL,
		ChatCompletionsPath: "/chat/completions",
		Stream:              boolPtr(true),
		Auth: &types.LLMAuthConfig{
			Mode:                "oauth2_polling",
			AuthBaseURL:         srv.URL,
			ClientID:            "client",
			TokenCacheFile:      cache,
			AccessTokenHeader:   "X-Auth-Token",
			PollTimeoutSeconds:  1,
			PollIntervalSeconds: 1,
		},
		RequestTimeoutSeconds:         5,
		StreamFirstByteTimeoutSeconds: 5,
		StreamStallTimeoutSeconds:     5,
	}
	authOpts := authOptionsFromConfig(cfg, 5*time.Second)
	auth, err := newOAuthPollingAuthenticator(authOpts)
	if err != nil {
		t.Fatalf("newOAuthPollingAuthenticator: %v", err)
	}
	if err := auth.saveCache(cachedOAuthToken{
		AccessToken: "stale-token",
		IssuedAt:    time.Now(),
		ExpiresAt:   time.Now().Add(time.Hour),
		Fingerprint: auth.fingerprint,
	}); err != nil {
		t.Fatalf("saveCache: %v", err)
	}
	adapter, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewFromConfig: %v", err)
	}
	resp, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, ChatOptions{})
	if err != nil {
		t.Fatalf("Chat stream: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("stream response=%+v", resp)
	}
	if chatCalls.Load() != 2 || tokenCalls.Load() != 1 {
		t.Fatalf("chatCalls=%d tokenCalls=%d, want 2/1", chatCalls.Load(), tokenCalls.Load())
	}
}

func TestNewFromConfig_ModelListInvalidatesOAuthTokenOn401(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "token.json")
	var modelCalls atomic.Int32
	var tokenCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/getToken":
			tokenCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"fresh-token","expires_in":"259199","token_type":"Bearer","userAccount":"u"}`))
		case "/models":
			call := modelCalls.Add(1)
			if call == 1 {
				if got := r.Header.Get("X-Auth-Token"); got != "stale-token" {
					t.Fatalf("first model-list token=%q, want stale-token", got)
				}
				http.Error(w, "expired", http.StatusUnauthorized)
				return
			}
			if got := r.Header.Get("X-Auth-Token"); got != "fresh-token" {
				t.Fatalf("second model-list token=%q, want fresh-token", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"name":"first","des":"primary"}]`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	cfg := types.LLMProviderConfig{
		Provider:   "openai",
		BaseURL:    srv.URL,
		ModelsPath: "/models",
		Stream:     boolPtr(false),
		Auth: &types.LLMAuthConfig{
			Mode:                "oauth2_polling",
			AuthBaseURL:         srv.URL,
			ClientID:            "client",
			TokenCacheFile:      cache,
			AccessTokenHeader:   "X-Auth-Token",
			PollTimeoutSeconds:  1,
			PollIntervalSeconds: 1,
		},
		RequestTimeoutSeconds: 5,
	}
	authOpts := authOptionsFromConfig(cfg, 5*time.Second)
	auth, err := newOAuthPollingAuthenticator(authOpts)
	if err != nil {
		t.Fatalf("newOAuthPollingAuthenticator: %v", err)
	}
	if err := auth.saveCache(cachedOAuthToken{
		AccessToken: "stale-token",
		IssuedAt:    time.Now(),
		ExpiresAt:   time.Now().Add(time.Hour),
		Fingerprint: auth.fingerprint,
	}); err != nil {
		t.Fatalf("saveCache: %v", err)
	}
	adapter, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewFromConfig: %v", err)
	}
	if adapter.ModelID() != "first" {
		t.Fatalf("model=%q, want first", adapter.ModelID())
	}
	if modelCalls.Load() != 2 || tokenCalls.Load() != 1 {
		t.Fatalf("modelCalls=%d tokenCalls=%d, want 2/1", modelCalls.Load(), tokenCalls.Load())
	}
}

func TestNewFromConfig_UsesFirstModelWhenModelOmitted(t *testing.T) {
	var sawModels atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		sawModels.Store(true)
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Fatalf("Authorization=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"first","des":"primary"},{"name":"second"}]`))
	}))
	defer srv.Close()

	adapter, err := NewFromConfig(types.LLMProviderConfig{
		Provider:   "openai",
		APIKey:     "k",
		BaseURL:    srv.URL,
		ModelsPath: "/models",
		Stream:     boolPtr(false),
	})
	if err != nil {
		t.Fatalf("NewFromConfig: %v", err)
	}
	if adapter.ModelID() != "first" || !sawModels.Load() {
		t.Fatalf("model=%q sawModels=%v", adapter.ModelID(), sawModels.Load())
	}
}

func TestParseModelList_FlexibleShapes(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantNames []string
		wantDesc  string
	}{
		{
			name:      "array with mixed description types",
			body:      `[{"name":"first","des":{"zh":"primary"}},{"id":"second","description":123},{"model_name":"third","desc":null}]`,
			wantNames: []string{"first", "second", "third"},
			wantDesc:  "",
		},
		{
			name:      "openai data object",
			body:      `{"data":[{"id":"first","description":"primary"}]}`,
			wantNames: []string{"first"},
			wantDesc:  "primary",
		},
		{
			name:      "string array",
			body:      `["first","second"]`,
			wantNames: []string{"first", "second"},
			wantDesc:  "",
		},
		{
			name:      "models object",
			body:      `{"models":[{"model":"first","des":456}]}`,
			wantNames: []string{"first"},
			wantDesc:  "456",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseModelList([]byte(tc.body))
			if err != nil {
				t.Fatalf("parseModelList: %v", err)
			}
			if len(got) != len(tc.wantNames) {
				t.Fatalf("models=%+v, want %d", got, len(tc.wantNames))
			}
			for i, want := range tc.wantNames {
				if got[i].Name != want {
					t.Fatalf("model[%d].Name=%q, want %q; all=%+v", i, got[i].Name, want, got)
				}
			}
			if tc.wantDesc != "" && got[0].Description != tc.wantDesc {
				t.Fatalf("model[0].Description=%q, want %q", got[0].Description, tc.wantDesc)
			}
		})
	}
}

func TestMarshalRequest_RequestExtraProtectsReservedFields(t *testing.T) {
	adapter := &OpenAIAdapter{
		model: "model-a",
		requestExtra: map[string]any{
			"queue": true,
			"model": "bad-model",
		},
	}
	body, err := adapter.marshalRequest(adapter.buildRequest([]Message{{Role: "user", Content: "hi"}}, nil, ChatOptions{}))
	if err != nil {
		t.Fatalf("marshalRequest: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `"queue":true`) {
		t.Fatalf("queue extra missing: %s", text)
	}
	if strings.Contains(text, "bad-model") || !strings.Contains(text, `"model":"model-a"`) {
		t.Fatalf("reserved model override leaked: %s", text)
	}
}

func boolPtr(v bool) *bool { return &v }
