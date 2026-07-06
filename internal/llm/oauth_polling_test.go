package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestOAuthPollingGetTokenConcurrentReadIsRaceFree exercises getToken's
// valid-token fast path concurrently with a writer that rewrites the
// token under a.mu (as Invalidate and the authorize slow path do). Before
// the fix, getToken read the multi-word a.token struct WITHOUT the lock,
// racing the writer; `go test -race` flags it. The writer always stores a
// valid token so readers stay on the fast path (hermetic — no network).
func TestOAuthPollingGetTokenConcurrentReadIsRaceFree(t *testing.T) {
	a := &oauthPollingAuthenticator{fingerprint: "fp"}
	valid := cachedOAuthToken{AccessToken: "tok", Fingerprint: "fp"} // zero ExpiresAt => valid
	a.token = valid

	stop := make(chan struct{})
	var writer sync.WaitGroup
	writer.Add(1)
	go func() {
		defer writer.Done()
		for {
			select {
			case <-stop:
				return
			default:
				a.mu.Lock()
				a.token = cachedOAuthToken{AccessToken: "tok", Fingerprint: "fp"}
				a.mu.Unlock()
			}
		}
	}()

	var readers sync.WaitGroup
	for g := 0; g < 4; g++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for i := 0; i < 2000; i++ {
				if _, err := a.getToken(context.Background()); err != nil {
					t.Errorf("getToken: %v", err)
					return
				}
			}
		}()
	}
	readers.Wait()
	close(stop)
	writer.Wait()
}

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
				w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
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

func TestOpenAIAdapter_OAuthPollingAmbiguous401PreservesDiskCache(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "token.json")
	var chatCalls atomic.Int32
	var tokenCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/getToken":
			tokenCalls.Add(1)
			http.Error(w, "unexpected token polling", http.StatusInternalServerError)
		case "/chat/completions":
			chatCalls.Add(1)
			if got := r.Header.Get("X-Auth-Token"); got != "cached-token" {
				t.Fatalf("token=%q, want cached-token", got)
			}
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("<html>temporary gateway auth failure</html>"))
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
	_, err = adapter.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, ChatOptions{})
	if err == nil {
		t.Fatal("ambiguous 401 should still surface after one retry with the preserved cached token")
	}
	if chatCalls.Load() != 2 {
		t.Fatalf("chatCalls=%d, want one retry with preserved disk token", chatCalls.Load())
	}
	if tokenCalls.Load() != 0 {
		t.Fatalf("tokenCalls=%d, want no re-authorization on first ambiguous 401", tokenCalls.Load())
	}
	if _, statErr := os.Stat(cache); statErr != nil {
		t.Fatalf("ambiguous 401 must preserve disk cache, stat err: %v", statErr)
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
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error_code":"AUTH_TOKEN_INVALID"}`))
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
				w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
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

// newTestOAuthAuthenticatorWithCache builds an oauth authenticator whose disk
// cache holds a valid token, for the invalidate-disposition pins below.
func newTestOAuthAuthenticatorWithCache(t *testing.T, cache string) *oauthPollingAuthenticator {
	t.Helper()
	auth, err := newOAuthPollingAuthenticator(AuthOptions{
		Mode:           authModeOAuthPolling,
		BaseURL:        "https://api.example.test/v1",
		AuthBaseURL:    "https://auth.example.test/oauth",
		ClientID:       "client",
		TokenCacheFile: cache,
		RefreshBefore:  time.Minute,
	})
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
	auth.token = tok
	return auth
}

// TestInvalidateForStatus_403KeepsDiskCache pins that a 403 (authorization /
// rate-limit, NOT token invalidation) clears only the in-memory token and
// PRESERVES the shared on-disk cache, so concurrent CLIs are not forced to
// re-run browser authorization for a token that is still valid.
func TestInvalidateForStatus_403KeepsDiskCache(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "token.json")
	auth := newTestOAuthAuthenticatorWithCache(t, cache)

	auth.InvalidateForStatus(http.StatusForbidden)

	if auth.token.AccessToken != "" {
		t.Fatalf("403 must clear in-memory token, got %q", auth.token.AccessToken)
	}
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("403 must preserve on-disk cache, stat err: %v", err)
	}
}

// TestInvalidateForAuthFailure_InvalidToken401DeletesDiskCache pins that a
// structured invalid-token 401 drops both the in-memory token and the on-disk
// cache immediately.
func TestInvalidateForAuthFailure_InvalidToken401DeletesDiskCache(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "token.json")
	auth := newTestOAuthAuthenticatorWithCache(t, cache)

	header := http.Header{}
	header.Set("WWW-Authenticate", `Bearer error="invalid_token"`)
	auth.InvalidateForAuthFailure(authFailure{
		Status: http.StatusUnauthorized,
		Header: header,
	})

	if auth.token.AccessToken != "" {
		t.Fatalf("invalid-token 401 must clear in-memory token, got %q", auth.token.AccessToken)
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("invalid-token 401 must delete on-disk cache, stat err: %v", err)
	}
}

func TestAuthErrorCodesFromWWWAuthenticateParsesBearerScheme(t *testing.T) {
	header := http.Header{}
	header.Set("WWW-Authenticate", `Bearer realm="gateway", error="invalid_token"`)
	got := authErrorCodesFromWWWAuthenticate(header)
	if len(got) != 1 || got[0] != "invalid_token" {
		t.Fatalf("authErrorCodesFromWWWAuthenticate=%v, want invalid_token", got)
	}
}

// TestInvalidateForAuthFailure_Ambiguous401KeepsDiskCache pins the default
// safety behavior for non-standard providers: a bare / HTML / unparsable 401 is
// ambiguous, so Codrax clears only memory and preserves the shared disk cache.
func TestInvalidateForAuthFailure_Ambiguous401KeepsDiskCache(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "token.json")
	auth := newTestOAuthAuthenticatorWithCache(t, cache)

	auth.InvalidateForAuthFailure(authFailure{
		Status: http.StatusUnauthorized,
		Body:   []byte("<html>temporary gateway auth failure</html>"),
	})

	if auth.token.AccessToken != "" {
		t.Fatalf("ambiguous 401 must clear in-memory token, got %q", auth.token.AccessToken)
	}
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("ambiguous 401 must preserve on-disk cache, stat err: %v", err)
	}
	if _, err := os.Stat(cache + ".failures.json"); err != nil {
		t.Fatalf("ambiguous 401 should persist a failure stamp, stat err: %v", err)
	}
}

func TestInvalidateForAuthFailure_ReasonMessage401StaysAmbiguous(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "token.json")
	auth := newTestOAuthAuthenticatorWithCache(t, cache)

	auth.InvalidateForAuthFailure(authFailure{
		Status: http.StatusUnauthorized,
		Body:   []byte(`{"reason":"token expired","message":"invalid_token"}`),
	})

	if auth.token.AccessToken != "" {
		t.Fatalf("ambiguous reason/message 401 must clear in-memory token, got %q", auth.token.AccessToken)
	}
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("free-form reason/message must not delete on-disk cache, stat err: %v", err)
	}
	if _, err := os.Stat(cache + ".failures.json"); err != nil {
		t.Fatalf("ambiguous reason/message 401 should persist a failure stamp, stat err: %v", err)
	}
}

func TestInvalidateForAuthFailure_Ambiguous401EscalatesAcrossProcesses(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "token.json")
	auth := newTestOAuthAuthenticatorWithCache(t, cache)

	for i := 1; i < defaultAmbiguous401Escalation; i++ {
		auth.InvalidateForAuthFailure(authFailure{Status: http.StatusUnauthorized, Body: []byte("not-json")})
		if _, err := os.Stat(cache); err != nil {
			t.Fatalf("ambiguous 401 #%d below threshold must preserve disk cache, stat err: %v", i, err)
		}
		// Simulate a fresh CLI process re-reading the same token from disk.
		auth = newTestOAuthAuthenticatorWithCache(t, cache)
	}
	auth.InvalidateForAuthFailure(authFailure{Status: http.StatusUnauthorized, Body: []byte("not-json")})
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("%dth ambiguous 401 must delete disk cache, stat err: %v", defaultAmbiguous401Escalation, err)
	}
	if _, err := os.Stat(cache + ".failures.json"); !os.IsNotExist(err) {
		t.Fatalf("escalation should remove failure stamp, stat err: %v", err)
	}
}

func TestRecordSuccessClearsAmbiguous401FailureStamp(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "token.json")
	auth := newTestOAuthAuthenticatorWithCache(t, cache)
	auth.InvalidateForAuthFailure(authFailure{Status: http.StatusUnauthorized, Body: []byte("not-json")})
	if _, err := os.Stat(cache + ".failures.json"); err != nil {
		t.Fatalf("expected failure stamp after ambiguous 401: %v", err)
	}

	auth = newTestOAuthAuthenticatorWithCache(t, cache)
	auth.RecordSuccess()
	if auth.consecutiveAmbiguous401 != 0 || auth.consecutive403 != 0 {
		t.Fatalf("success should clear counters, got 401=%d 403=%d", auth.consecutiveAmbiguous401, auth.consecutive403)
	}
	if _, err := os.Stat(cache + ".failures.json"); !os.IsNotExist(err) {
		t.Fatalf("success should remove failure stamp, stat err: %v", err)
	}
}

// TestInvalidate_DeletesDiskCache pins that the plain Invalidate() (the 401-
// equivalent disposition) removes the on-disk cache.
func TestInvalidate_DeletesDiskCache(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "token.json")
	auth := newTestOAuthAuthenticatorWithCache(t, cache)

	auth.Invalidate()

	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("Invalidate must delete on-disk cache, stat err: %v", err)
	}
}

// TestInvalidateForStatus_Consecutive403EscalatesToDiskDelete pins the
// revocation-403 escape hatch: N (=consecutiveForbiddenEscalation)
// back-to-back 403s on the same on-disk token escalate to a full Invalidate
// that deletes the disk cache, so a revoked token cannot loop forever.
func TestInvalidateForStatus_Consecutive403EscalatesToDiskDelete(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "token.json")
	auth := newTestOAuthAuthenticatorWithCache(t, cache)

	for i := 1; i < consecutiveForbiddenEscalation; i++ {
		auth.InvalidateForStatus(http.StatusForbidden)
		if _, err := os.Stat(cache); err != nil {
			t.Fatalf("403 #%d (below threshold) must preserve disk cache, stat err: %v", i, err)
		}
		// Simulate the loop re-reading the same still-valid disk token.
		auth.token = cachedOAuthToken{
			AccessToken: "cached-token", ExpiresAt: time.Now().Add(time.Hour), Fingerprint: auth.fingerprint,
		}
	}
	// The Nth consecutive 403 escalates and deletes the disk cache.
	auth.InvalidateForStatus(http.StatusForbidden)
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("%dth consecutive 403 must delete disk cache to force re-auth, stat err: %v", consecutiveForbiddenEscalation, err)
	}
	if auth.consecutive403 != 0 {
		t.Fatalf("counter must reset after escalation, got %d", auth.consecutive403)
	}
}

// TestInvalidateForStatus_403CounterResetByNon403 pins that a non-403 status
// (401) zeroes the consecutive-403 counter, so a later single 403 does not tip
// straight over the threshold.
func TestInvalidateForStatus_403CounterResetByNon403(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "token.json")
	auth := newTestOAuthAuthenticatorWithCache(t, cache)

	// Two 403s (still below threshold=3).
	auth.InvalidateForStatus(http.StatusForbidden)
	auth.InvalidateForStatus(http.StatusForbidden)
	if auth.consecutive403 != 2 {
		t.Fatalf("expected counter=2, got %d", auth.consecutive403)
	}
	// A precise invalid-token 401 resets the counter (and deletes disk, which
	// we re-seed).
	auth.InvalidateForAuthFailure(authFailure{
		Status: http.StatusUnauthorized,
		Body:   []byte(`{"error":"invalid_token"}`),
	})
	if auth.consecutive403 != 0 {
		t.Fatalf("401 must reset the 403 counter, got %d", auth.consecutive403)
	}
	auth = newTestOAuthAuthenticatorWithCache(t, cache)
	// A fresh single 403 after reset must NOT delete the disk cache.
	auth.InvalidateForStatus(http.StatusForbidden)
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("single 403 after reset must preserve disk cache, stat err: %v", err)
	}
}

// TestInvalidateForStatus_FreshAuthResetsCounter pins that a genuinely new
// credential (authorizeAndPoll success in getToken) clears the escalation
// counter: two 403s, a fresh authorization, then two more 403s must NOT reach
// the threshold, so the disk cache survives.
func TestInvalidateForStatus_FreshAuthResetsCounter(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "token.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/getToken" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fresh-token","expires_in":"259199","token_type":"Bearer","userAccount":"u"}`))
	}))
	defer srv.Close()

	auth, err := newOAuthPollingAuthenticator(AuthOptions{
		Mode:           authModeOAuthPolling,
		BaseURL:        srv.URL,
		AuthBaseURL:    srv.URL,
		ClientID:       "client",
		TokenCacheFile: cache,
		RefreshBefore:  time.Minute,
		PollTimeout:    time.Second,
		PollInterval:   time.Millisecond,
		RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("newOAuthPollingAuthenticator: %v", err)
	}

	auth.InvalidateForStatus(http.StatusForbidden)
	auth.InvalidateForStatus(http.StatusForbidden)
	if auth.consecutive403 != 2 {
		t.Fatalf("expected counter=2 before fresh auth, got %d", auth.consecutive403)
	}
	// Fresh authorization (no valid in-memory/disk token) resets the counter.
	if _, err := auth.getToken(context.Background()); err != nil {
		t.Fatalf("getToken (fresh auth): %v", err)
	}
	if auth.consecutive403 != 0 {
		t.Fatalf("fresh authorization must reset the 403 counter, got %d", auth.consecutive403)
	}
	// Two more 403s (with the loop re-reading the fresh disk token) stay below
	// threshold, so the disk cache survives.
	for i := 0; i < 2; i++ {
		auth.InvalidateForStatus(http.StatusForbidden)
		auth.token = cachedOAuthToken{AccessToken: "fresh-token", ExpiresAt: time.Now().Add(time.Hour), Fingerprint: auth.fingerprint}
	}
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("after a fresh-auth reset, two more 403s must not delete disk cache, stat err: %v", err)
	}
}

// TestExpandUserPath_LoneTildeExpandsToHome pins P4-1: a bare "~" cache path
// expands to the home directory (== "~/" semantics), not a CWD-relative
// literal that would reintroduce working-directory drift.
func TestExpandUserPath_LoneTildeExpandsToHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory available")
	}
	if got := expandUserPath("~"); got != home {
		t.Fatalf("expandUserPath(\"~\")=%q, want home %q (not CWD-relative)", got, home)
	}
	if got := expandUserPath("~/x"); got != filepath.Join(home, "x") {
		t.Fatalf("expandUserPath(\"~/x\")=%q, want %q", got, filepath.Join(home, "x"))
	}
	if got := expandUserPath(""); got != "" {
		t.Fatalf("expandUserPath(\"\")=%q, want empty", got)
	}
}

// TestOAuthCacheFile_LoneTildeIsHomeAnchoredNotCWD pins that a provider
// configured with token_cache_file:"~" resolves the cache under the home
// directory, not the current working directory.
func TestOAuthCacheFile_LoneTildeIsHomeAnchoredNotCWD(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory available")
	}
	auth, err := newOAuthPollingAuthenticator(AuthOptions{
		Mode:           authModeOAuthPolling,
		BaseURL:        "https://api.example.test/v1",
		AuthBaseURL:    "https://auth.example.test/oauth",
		ClientID:       "client",
		TokenCacheFile: "~",
		RefreshBefore:  time.Minute,
	})
	if err != nil {
		t.Fatalf("newOAuthPollingAuthenticator: %v", err)
	}
	if auth.cacheFile != home {
		t.Fatalf("token_cache_file \"~\" resolved to %q, want home %q (not CWD)", auth.cacheFile, home)
	}
}

// TestLoadCache_CorruptJSONLogsOffsetNotContent pins P4-2: a corrupt cache
// still classifies as a syntax error (so the offset-only, no-err.Error() log
// branch is exercised) and returns an error without succeeding.
func TestLoadCache_CorruptJSONLogsOffsetNotContent(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "token.json")
	// Invalid JSON whose bytes would leak into a naive err.Error() dump.
	if err := os.WriteFile(cache, []byte("{not-json SECRET-BYTE"), 0o600); err != nil {
		t.Fatalf("write corrupt cache: %v", err)
	}
	auth := &oauthPollingAuthenticator{cacheFile: cache, fingerprint: "fp", opts: AuthOptions{RefreshBefore: time.Minute}}
	_, err := auth.loadCache(time.Now())
	if err == nil {
		t.Fatal("corrupt JSON must not load successfully")
	}
	var se *json.SyntaxError
	if !errors.As(err, &se) {
		t.Fatalf("corrupt cache should surface a *json.SyntaxError (offset-only log path), got %T: %v", err, err)
	}
}

// TestSaveCache_AtomicWriteNoTempLeftAndValidJSON pins the atomic write:
// after saveCache the canonical file is present, mode 0600, complete valid
// JSON, and NO .tmp turds are left in the directory.
func TestSaveCache_AtomicWriteNoTempLeftAndValidJSON(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "token.json")
	auth := newTestOAuthAuthenticatorWithCache(t, cache)

	// saveCache already ran inside the helper; re-save to exercise replace.
	if err := auth.saveCache(auth.token); err != nil {
		t.Fatalf("saveCache: %v", err)
	}

	data, err := os.ReadFile(cache)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	var got cachedOAuthToken
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("cache is not complete valid JSON (torn write?): %v", err)
	}
	if got.AccessToken != "cached-token" {
		t.Fatalf("cache token=%q, want cached-token", got.AccessToken)
	}
	st, err := os.Stat(cache)
	if err != nil {
		t.Fatalf("stat cache: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("cache mode=%#o, want 0600", st.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".oauth2-") || strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("atomic write left a temp file behind: %q", e.Name())
		}
	}
}

// TestLoadCache_ClassifiesMissReasons pins the deterministic loadCache miss
// classification (the signal behind the INFO breadcrumbs): not-exist,
// fingerprint mismatch, and expired each yield the corresponding failure.
func TestLoadCache_ClassifiesMissReasons(t *testing.T) {
	now := time.Now()

	t.Run("not_exist", func(t *testing.T) {
		auth := &oauthPollingAuthenticator{
			cacheFile:   filepath.Join(t.TempDir(), "missing.json"),
			fingerprint: "fp",
			opts:        AuthOptions{RefreshBefore: time.Minute},
		}
		if _, err := auth.loadCache(now); !os.IsNotExist(err) {
			t.Fatalf("missing cache should return NotExist, got %v", err)
		}
	})

	t.Run("fingerprint_mismatch", func(t *testing.T) {
		cache := filepath.Join(t.TempDir(), "token.json")
		auth := &oauthPollingAuthenticator{cacheFile: cache, fingerprint: "fp-new", opts: AuthOptions{RefreshBefore: time.Minute}}
		writeTokenFile(t, cache, cachedOAuthToken{
			AccessToken: "t", ExpiresAt: now.Add(time.Hour), Fingerprint: "fp-old",
		})
		if _, err := auth.loadCache(now); err == nil || !strings.Contains(err.Error(), "fingerprint mismatch") {
			t.Fatalf("stale fingerprint should be classified, got %v", err)
		}
	})

	t.Run("expired", func(t *testing.T) {
		cache := filepath.Join(t.TempDir(), "token.json")
		auth := &oauthPollingAuthenticator{cacheFile: cache, fingerprint: "fp", opts: AuthOptions{RefreshBefore: time.Minute}}
		writeTokenFile(t, cache, cachedOAuthToken{
			AccessToken: "t", ExpiresAt: now.Add(-time.Hour), Fingerprint: "fp",
		})
		if _, err := auth.loadCache(now); err == nil || !strings.Contains(err.Error(), "expired") {
			t.Fatalf("expired token should be classified, got %v", err)
		}
	})
}

func writeTokenFile(t *testing.T, path string, tok cachedOAuthToken) {
	t.Helper()
	data, err := json.Marshal(tok)
	if err != nil {
		t.Fatalf("marshal token: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
}

func boolPtr(v bool) *bool { return &v }
