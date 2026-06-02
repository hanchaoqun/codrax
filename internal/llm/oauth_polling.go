package llm

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
)

const (
	authModeAPIKey       = "api_key"
	authModeOAuthPolling = "oauth2_polling"

	defaultOAuthAuthorizePath = "/oauth2/authorize"
	defaultOAuthCallbackPath  = "/oauth/callback"
	defaultOAuthTokenPath     = "/oauth/getToken"
	defaultOAuthResponseType  = "code"
	defaultOAuthTokenHeader   = "X-Auth-Token"
	defaultTokenFormat        = "{token}"
	defaultTokenPollTimeout   = 30 * time.Minute
	defaultTokenPollInterval  = time.Second
	defaultRefreshBefore      = 5 * time.Minute
)

// AuthOptions is the adapter-facing provider auth configuration. It is
// resolved from providers.yaml by the factory layer and is deliberately not
// model-facing tool JSON.
type AuthOptions struct {
	Mode string

	BaseURL     string
	AuthBaseURL string
	ClientID    string

	Scope         string
	ResponseType  string
	ScopeResource string

	AuthorizePath string
	CallbackPath  string
	TokenPath     string

	TokenCacheFile string

	PollTimeout    time.Duration
	PollInterval   time.Duration
	RefreshBefore  time.Duration
	TokenTTL       time.Duration
	TokenHeader    string
	TokenFormat    string
	RequestTimeout time.Duration
	TLS            TLSOptions
}

// OAuthAuthorizationNotice is the non-secret information shown when an OAuth
// polling provider needs the user to complete browser authorization.
type OAuthAuthorizationNotice struct {
	URL       string
	CacheFile string
}

var oauthAuthorizationNotice = struct {
	sync.Mutex
	hook func(OAuthAuthorizationNotice) bool
}{}

// SetOAuthAuthorizationNoticeHook installs a process-global UI hook for OAuth
// authorization prompts. The hook returns true when it rendered the prompt.
// Passing nil restores the default stderr prompt. This is intentionally a UI
// hook only; token polling, cache, and request semantics are unchanged.
func SetOAuthAuthorizationNoticeHook(hook func(OAuthAuthorizationNotice) bool) {
	oauthAuthorizationNotice.Lock()
	defer oauthAuthorizationNotice.Unlock()
	oauthAuthorizationNotice.hook = hook
}

func emitOAuthAuthorizationNotice(notice OAuthAuthorizationNotice) bool {
	oauthAuthorizationNotice.Lock()
	hook := oauthAuthorizationNotice.hook
	oauthAuthorizationNotice.Unlock()
	if hook != nil && hook(notice) {
		return true
	}
	return false
}

type requestAuthenticator interface {
	Apply(ctx context.Context, req *http.Request) error
	Invalidate()
	Name() string
}

type bearerAPIKeyAuthenticator struct {
	apiKey string
}

func (a *bearerAPIKeyAuthenticator) Apply(_ context.Context, req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *bearerAPIKeyAuthenticator) Invalidate()  {}
func (a *bearerAPIKeyAuthenticator) Name() string { return authModeAPIKey }

type oauthPollingAuthenticator struct {
	opts        AuthOptions
	client      *http.Client
	cacheFile   string
	fingerprint string

	mu    sync.Mutex
	token cachedOAuthToken
}

type cachedOAuthToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	UserAccount  string    `json:"userAccount,omitempty"`
	ExpiresIn    int64     `json:"expires_in,omitempty"`
	IssuedAt     time.Time `json:"issued_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	Fingerprint  string    `json:"fingerprint"`
}

type oauthTokenResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	TokenType    string      `json:"token_type"`
	Scope        string      `json:"scope"`
	ExpiresIn    flexibleInt `json:"expires_in"`
	UserAccount  string      `json:"userAccount"`
}

type flexibleInt int64

func (f *flexibleInt) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*f = 0
		return nil
	}
	var n int64
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		s = strings.TrimSpace(s)
		if s == "" {
			*f = 0
			return nil
		}
		parsed, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return err
		}
		n = parsed
	} else {
		parsed, err := strconv.ParseInt(string(data), 10, 64)
		if err != nil {
			return err
		}
		n = parsed
	}
	if n < 0 {
		n = 0
	}
	*f = flexibleInt(n)
	return nil
}

func newRequestAuthenticator(apiKey string, opts AuthOptions) (requestAuthenticator, error) {
	mode := normalizeAuthMode(opts.Mode)
	switch mode {
	case "", authModeAPIKey:
		if strings.TrimSpace(apiKey) == "" {
			return nil, fmt.Errorf("providers.yaml: llm api_key is required when auth.mode is empty or api_key")
		}
		return &bearerAPIKeyAuthenticator{apiKey: apiKey}, nil
	case authModeOAuthPolling:
		return newOAuthPollingAuthenticator(opts)
	default:
		return nil, fmt.Errorf("providers.yaml: unknown llm auth.mode %q (supported: api_key, oauth2_polling)", opts.Mode)
	}
}

func normalizeAuthMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", authModeAPIKey, "api-key", "bearer":
		return authModeAPIKey
	case authModeOAuthPolling, "oauth2", "oauth2_device", "oauth2_poll", "internal_oauth2",
		"codeagent_oauth2", "codeagent_oauth", "codeagent":
		return authModeOAuthPolling
	default:
		return strings.ToLower(strings.TrimSpace(mode))
	}
}

func newOAuthPollingAuthenticator(opts AuthOptions) (*oauthPollingAuthenticator, error) {
	opts.BaseURL = strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	opts.AuthBaseURL = strings.TrimRight(strings.TrimSpace(opts.AuthBaseURL), "/")
	if opts.BaseURL == "" {
		return nil, errors.New("providers.yaml: base_url is required for auth.mode=oauth2_polling")
	}
	if opts.AuthBaseURL == "" {
		return nil, errors.New("providers.yaml: auth.auth_base_url is required for auth.mode=oauth2_polling")
	}
	if strings.TrimSpace(opts.ClientID) == "" {
		return nil, errors.New("providers.yaml: auth.client_id is required for auth.mode=oauth2_polling")
	}
	if opts.AuthorizePath == "" {
		opts.AuthorizePath = defaultOAuthAuthorizePath
	}
	if opts.CallbackPath == "" {
		opts.CallbackPath = defaultOAuthCallbackPath
	}
	if opts.TokenPath == "" {
		opts.TokenPath = defaultOAuthTokenPath
	}
	if opts.ResponseType == "" {
		opts.ResponseType = defaultOAuthResponseType
	}
	if opts.TokenHeader == "" {
		opts.TokenHeader = defaultOAuthTokenHeader
	}
	if opts.TokenFormat == "" {
		opts.TokenFormat = defaultTokenFormat
	}
	if opts.PollTimeout <= 0 {
		opts.PollTimeout = defaultTokenPollTimeout
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = defaultTokenPollInterval
	}
	if opts.RefreshBefore <= 0 {
		opts.RefreshBefore = defaultRefreshBefore
	}
	if opts.RequestTimeout <= 0 {
		opts.RequestTimeout = 30 * time.Second
	}

	fp := oauthFingerprint(opts)
	cacheFile := strings.TrimSpace(opts.TokenCacheFile)
	if cacheFile == "" {
		cacheFile = defaultOAuthCacheFile("oauth2-" + fp[:16] + ".json")
	} else {
		cacheFile = expandUserPath(cacheFile)
	}

	return &oauthPollingAuthenticator{
		opts:        opts,
		client:      buildHTTPClient(opts.TLS, opts.BaseURL, opts.RequestTimeout),
		cacheFile:   cacheFile,
		fingerprint: fp,
	}, nil
}

func (a *oauthPollingAuthenticator) Name() string { return authModeOAuthPolling }

func (a *oauthPollingAuthenticator) Apply(ctx context.Context, req *http.Request) error {
	tok, err := a.getToken(ctx)
	if err != nil {
		return err
	}
	req.Header.Set(a.opts.TokenHeader, strings.ReplaceAll(a.opts.TokenFormat, "{token}", tok.AccessToken))
	return nil
}

func (a *oauthPollingAuthenticator) Invalidate() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.token = cachedOAuthToken{}
	if a.cacheFile != "" {
		if err := os.Remove(a.cacheFile); err != nil && !os.IsNotExist(err) {
			logging.Warning("[llm/auth] could not remove token cache %q: %v", a.cacheFile, err)
		}
	}
}

func (a *oauthPollingAuthenticator) getToken(ctx context.Context) (cachedOAuthToken, error) {
	if a.tokenValid(a.token, time.Now()) {
		return a.token, nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	if a.tokenValid(a.token, now) {
		return a.token, nil
	}
	if tok, err := a.loadCache(now); err == nil && a.tokenValid(tok, now) {
		a.token = tok
		return tok, nil
	}
	tok, err := a.authorizeAndPoll(ctx)
	if err != nil {
		return cachedOAuthToken{}, err
	}
	a.token = tok
	if err := a.saveCache(tok); err != nil {
		logging.Warning("[llm/auth] could not save OAuth token cache %q: %v", a.cacheFile, err)
	}
	return tok, nil
}

func (a *oauthPollingAuthenticator) tokenValid(tok cachedOAuthToken, now time.Time) bool {
	if tok.AccessToken == "" || tok.Fingerprint != a.fingerprint {
		return false
	}
	if tok.ExpiresAt.IsZero() {
		return true
	}
	return now.Before(tok.ExpiresAt.Add(-a.opts.RefreshBefore))
}

func (a *oauthPollingAuthenticator) authorizeAndPoll(ctx context.Context) (cachedOAuthToken, error) {
	clientCode, err := randomHex(16)
	if err != nil {
		return cachedOAuthToken{}, fmt.Errorf("generate OAuth client_code: %w", err)
	}
	authURL, redirectURL, err := a.buildAuthURL(clientCode)
	if err != nil {
		return cachedOAuthToken{}, err
	}

	if !emitOAuthAuthorizationNotice(OAuthAuthorizationNotice{URL: authURL, CacheFile: a.cacheFile}) {
		if preferChineseDisplay() {
			fmt.Fprintf(os.Stderr, "  › 需要完成 LLM OAuth 授权。请在浏览器打开：\n    %s\n", authURL)
		} else {
			fmt.Fprintf(os.Stderr, "  › LLM OAuth authorization required. Open this URL in a browser:\n    %s\n", authURL)
		}
	}
	logging.Info("[llm/auth] waiting for OAuth authorization (cache=%s)", a.cacheFile)

	pollCtx, cancel := context.WithTimeout(ctx, a.opts.PollTimeout)
	defer cancel()
	ticker := time.NewTicker(a.opts.PollInterval)
	defer ticker.Stop()
	attempt := 0
	var lastErr error
	for {
		attempt++
		resp, err := a.requestTokenOnce(pollCtx, clientCode, redirectURL)
		if err == nil && strings.TrimSpace(resp.AccessToken) != "" {
			return a.cacheFromResponse(resp, time.Now()), nil
		}
		if err != nil {
			lastErr = err
		}
		if attempt%10 == 0 {
			logging.Info("[llm/auth] still waiting for OAuth authorization result (polls=%d)", attempt)
		}
		select {
		case <-pollCtx.Done():
			if lastErr != nil {
				return cachedOAuthToken{}, fmt.Errorf("OAuth polling timed out: %w (last error: %v)", pollCtx.Err(), lastErr)
			}
			return cachedOAuthToken{}, fmt.Errorf("OAuth polling timed out: %w", pollCtx.Err())
		case <-ticker.C:
		}
	}
}

func (a *oauthPollingAuthenticator) buildAuthURL(clientCode string) (string, string, error) {
	authEndpoint := joinURLPath(a.opts.AuthBaseURL, a.opts.AuthorizePath)
	u, err := url.Parse(authEndpoint)
	if err != nil {
		return "", "", fmt.Errorf("parse auth URL: %w", err)
	}
	redirectURL := joinURLPath(a.opts.BaseURL, a.opts.CallbackPath) + "?client_code=" + url.QueryEscape(clientCode)
	q := u.Query()
	q.Set("client_id", a.opts.ClientID)
	q.Set("redirect_uri", redirectURL)
	if a.opts.Scope != "" {
		q.Set("scope", a.opts.Scope)
	}
	if a.opts.ResponseType != "" {
		q.Set("response_type", a.opts.ResponseType)
	}
	if a.opts.ScopeResource != "" {
		q.Set("scope_resource", a.opts.ScopeResource)
	}
	u.RawQuery = q.Encode()
	return u.String(), redirectURL, nil
}

func (a *oauthPollingAuthenticator) requestTokenOnce(ctx context.Context, clientCode, redirectURL string) (oauthTokenResponse, error) {
	reqCtx, cancel := context.WithTimeout(ctx, a.opts.RequestTimeout)
	defer cancel()
	payload := map[string]string{
		"clientCode":  clientCode,
		"redirectUrl": redirectURL,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return oauthTokenResponse{}, err
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, joinURLPath(a.opts.BaseURL, a.opts.TokenPath), bytes.NewReader(body))
	if err != nil {
		return oauthTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return oauthTokenResponse{}, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return oauthTokenResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return oauthTokenResponse{}, fmt.Errorf("getToken HTTP %d: %s", resp.StatusCode, trimForLog(respBody, 512))
	}
	if len(bytes.TrimSpace(respBody)) == 0 {
		return oauthTokenResponse{}, nil
	}
	var tokenResp oauthTokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return oauthTokenResponse{}, fmt.Errorf("parse getToken response: %w", err)
	}
	return tokenResp, nil
}

func (a *oauthPollingAuthenticator) cacheFromResponse(resp oauthTokenResponse, issuedAt time.Time) cachedOAuthToken {
	expiresIn := int64(resp.ExpiresIn)
	if expiresIn <= 0 && a.opts.TokenTTL > 0 {
		expiresIn = int64(a.opts.TokenTTL.Seconds())
	}
	var expiresAt time.Time
	if expiresIn > 0 {
		expiresAt = issuedAt.Add(time.Duration(expiresIn) * time.Second)
	}
	return cachedOAuthToken{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		TokenType:    resp.TokenType,
		Scope:        resp.Scope,
		UserAccount:  resp.UserAccount,
		ExpiresIn:    expiresIn,
		IssuedAt:     issuedAt,
		ExpiresAt:    expiresAt,
		Fingerprint:  a.fingerprint,
	}
}

func (a *oauthPollingAuthenticator) loadCache(now time.Time) (cachedOAuthToken, error) {
	if a.cacheFile == "" {
		return cachedOAuthToken{}, errors.New("token cache disabled")
	}
	data, err := os.ReadFile(a.cacheFile)
	if err != nil {
		return cachedOAuthToken{}, err
	}
	var tok cachedOAuthToken
	if err := json.Unmarshal(data, &tok); err != nil {
		return cachedOAuthToken{}, err
	}
	if tok.Fingerprint != a.fingerprint {
		return cachedOAuthToken{}, errors.New("token cache fingerprint mismatch")
	}
	if !a.tokenValid(tok, now) {
		return cachedOAuthToken{}, errors.New("token cache expired or near expiry")
	}
	return tok, nil
}

func (a *oauthPollingAuthenticator) saveCache(tok cachedOAuthToken) error {
	if a.cacheFile == "" || tok.AccessToken == "" {
		return nil
	}
	dir := filepath.Dir(a.cacheFile)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		logging.Warning("[llm/auth] could not chmod token cache dir %q: %v", dir, err)
	}
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.cacheFile, data, 0o600)
}

func oauthFingerprint(opts AuthOptions) string {
	h := sha256.New()
	parts := []string{
		opts.BaseURL,
		opts.AuthBaseURL,
		opts.ClientID,
		opts.Scope,
		opts.ScopeResource,
		opts.AuthorizePath,
		opts.CallbackPath,
		opts.TokenPath,
		opts.TokenHeader,
	}
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func defaultOAuthCacheFile(name string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "codrax-auth", name)
	}
	return filepath.Join(home, ".codrax", "auth", name)
}

func expandUserPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "~" {
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func trimForLog(body []byte, max int) string {
	s := strings.TrimSpace(string(body))
	if len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}

func joinURLPath(base, path string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	path = strings.TrimSpace(path)
	if path == "" {
		return base
	}
	if u, err := url.Parse(path); err == nil && u.Scheme != "" && u.Host != "" {
		return path
	}
	return base + "/" + strings.TrimLeft(path, "/")
}
