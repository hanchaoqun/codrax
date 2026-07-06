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

	defaultOAuthAuthorizePath     = "/oauth2/authorize"
	defaultOAuthCallbackPath      = "/oauth/callback"
	defaultOAuthTokenPath         = "/oauth/getToken"
	defaultOAuthResponseType      = "code"
	defaultOAuthTokenHeader       = "X-Auth-Token"
	defaultTokenFormat            = "{token}"
	defaultTokenPollTimeout       = 30 * time.Minute
	defaultTokenPollInterval      = time.Second
	defaultRefreshBefore          = 5 * time.Minute
	defaultAmbiguous401Escalation = 3
)

var defaultInvalidTokenErrorCodes = []string{
	"invalid_token",
	"invalid_grant",
	"expired_token",
	"token_expired",
	"access_token_expired",
	"access_token_invalid",
	"auth_token_invalid",
}

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

	InvalidTokenErrorCodes   []string
	Ambiguous401PreserveDisk *bool
	Ambiguous401Escalation   int
}

// OAuthAuthorizationNotice is the non-secret information shown when an OAuth
// polling provider needs the user to complete browser authorization.
type OAuthAuthorizationNotice struct {
	URL       string
	CacheFile string
}

// OAuthAuthorizationCompleteNotice is emitted once browser authorization has
// produced a usable token. It intentionally contains no token material.
type OAuthAuthorizationCompleteNotice struct {
	CacheFile string
}

var oauthAuthorizationNotice = struct {
	sync.Mutex
	hook func(OAuthAuthorizationNotice) bool
}{}

var oauthAuthorizationCompleteNotice = struct {
	sync.Mutex
	hook func(OAuthAuthorizationCompleteNotice) bool
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

// SetOAuthAuthorizationCompleteNoticeHook installs a UI hook for the
// successful end of an OAuth browser authorization flow.
func SetOAuthAuthorizationCompleteNoticeHook(hook func(OAuthAuthorizationCompleteNotice) bool) {
	oauthAuthorizationCompleteNotice.Lock()
	defer oauthAuthorizationCompleteNotice.Unlock()
	oauthAuthorizationCompleteNotice.hook = hook
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

func emitOAuthAuthorizationCompleteNotice(notice OAuthAuthorizationCompleteNotice) bool {
	oauthAuthorizationCompleteNotice.Lock()
	hook := oauthAuthorizationCompleteNotice.hook
	oauthAuthorizationCompleteNotice.Unlock()
	if hook != nil && hook(notice) {
		return true
	}
	return false
}

type requestAuthenticator interface {
	Apply(ctx context.Context, req *http.Request) error
	// Invalidate drops any cached credential (memory + on-disk) because it
	// is believed to be truly invalid, e.g. an authentication (401) failure.
	Invalidate()
	// InvalidateForAuthFailure drops cached credential state according to the
	// HTTP status plus structured provider auth-error signals that provoked
	// the auth retry.
	InvalidateForAuthFailure(f authFailure)
	// RecordSuccess clears transient auth-failure counters/stamps after a
	// request succeeds with the current credential.
	RecordSuccess()
	Name() string
}

type authFailure struct {
	Status int
	Header http.Header
	Body   []byte
}

type bearerAPIKeyAuthenticator struct {
	apiKey string
}

func (a *bearerAPIKeyAuthenticator) Apply(_ context.Context, req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *bearerAPIKeyAuthenticator) Invalidate()                            {}
func (a *bearerAPIKeyAuthenticator) InvalidateForAuthFailure(_ authFailure) {}
func (a *bearerAPIKeyAuthenticator) RecordSuccess()                         {}
func (a *bearerAPIKeyAuthenticator) Name() string                           { return authModeAPIKey }

// consecutiveForbiddenEscalation is the number of back-to-back 403 responses
// on the same fingerprint after which a preserve-disk 403 disposition is
// upgraded to a full Invalidate (disk delete → re-authorization). Without this,
// a server-side REVOCATION-type 403 would loop forever: the 403 branch clears
// only the in-memory token, the next request re-reads the same still-within-
// expiry token from disk, and the server 403s again. A precise integer
// threshold (not a noisy heuristic) drives this hard escalation, per the
// precise-signals-for-hard-gates red line.
const consecutiveForbiddenEscalation = 3

type oauthPollingAuthenticator struct {
	opts        AuthOptions
	client      *http.Client
	cacheFile   string
	fingerprint string

	mu                      sync.Mutex
	token                   cachedOAuthToken
	consecutive403          int
	consecutiveAmbiguous401 int
}

type oauthFailureStamp struct {
	Fingerprint string    `json:"fingerprint"`
	TokenHash   string    `json:"token_hash"`
	Status      int       `json:"status"`
	Class       string    `json:"class"`
	Count       int       `json:"count"`
	UpdatedAt   time.Time `json:"updated_at"`
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
	if len(opts.InvalidTokenErrorCodes) == 0 {
		opts.InvalidTokenErrorCodes = append([]string(nil), defaultInvalidTokenErrorCodes...)
	}
	if opts.Ambiguous401Escalation <= 0 {
		opts.Ambiguous401Escalation = defaultAmbiguous401Escalation
	}

	fp := oauthFingerprint(opts)
	cacheFile := strings.TrimSpace(opts.TokenCacheFile)
	if cacheFile == "" {
		cacheFile = defaultOAuthCacheFile("oauth2-" + fp[:16] + ".json")
	} else {
		cacheFile = expandUserPath(cacheFile)
	}

	client, err := buildHTTPClient(opts.TLS, opts.BaseURL, opts.RequestTimeout)
	if err != nil {
		return nil, err
	}

	return &oauthPollingAuthenticator{
		opts:        opts,
		client:      client,
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

// Invalidate clears the in-memory token AND deletes the on-disk cache. It is
// the disposition for a token that is believed truly invalid (401).
func (a *oauthPollingAuthenticator) Invalidate() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dropMemoryLocked()
	a.dropDiskLocked()
	a.dropFailureStampLocked()
}

// InvalidateForStatus is kept for narrow internal tests and compatibility
// with old call sites. Production callers should pass the full auth failure so
// 401 handling can inspect structured auth-error headers/body before deleting
// disk cache.
func (a *oauthPollingAuthenticator) InvalidateForStatus(status int) {
	a.InvalidateForAuthFailure(authFailure{Status: status})
}

func (a *oauthPollingAuthenticator) RecordSuccess() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.consecutive403 = 0
	a.consecutiveAmbiguous401 = 0
	a.dropFailureStampLocked()
}

// InvalidateForAuthFailure dispatches on the provoking HTTP status and precise
// provider auth-error signals. A structured invalid-token signal on 401 deletes
// the disk cache immediately. A bare / unparsable / HTML 401 is ambiguous: by
// default it preserves the disk cache, clears memory, and escalates to disk
// delete only after the same token reaches the configured consecutive failure
// threshold. A 403 is an authorization / rate-limit / policy signal, NOT proof
// the token is invalid, so the on-disk cache is preserved and only the in-memory
// copy is dropped.
//
// Escape hatch for revocation-type 403: if the SAME on-disk token keeps
// provoking 403s, the preserve-disk behavior would loop forever (drop memory →
// re-read the same still-within-expiry token → 403 again). A consecutive-403
// counter therefore escalates to a full Invalidate (disk delete →
// re-authorization) once it reaches consecutiveForbiddenEscalation. Precise
// invalid-token 401s and non-403/non-401 auth failures reset the 403 counter;
// ambiguous 401s use their own token-bound escalation stamp. A successful
// authenticated request clears both counters.
func (a *oauthPollingAuthenticator) InvalidateForAuthFailure(f authFailure) {
	status := f.Status
	if status == http.StatusUnauthorized {
		invalidTokenSignal := a.hasInvalidTokenSignal(f)
		if invalidTokenSignal || !a.ambiguous401PreserveDisk() {
			a.mu.Lock()
			a.consecutive403 = 0
			a.consecutiveAmbiguous401 = 0
			a.dropMemoryLocked()
			a.dropDiskLocked()
			a.dropFailureStampLocked()
			a.mu.Unlock()
			if invalidTokenSignal {
				logging.Info("[llm/auth] precise invalid-token 401 on OAuth-authenticated request: deleting cache %q to force re-authorization", a.cacheFile)
			} else {
				logging.Info("[llm/auth] ambiguous 401 on OAuth-authenticated request: legacy policy deletes cache %q to force re-authorization", a.cacheFile)
			}
			return
		}
		a.mu.Lock()
		a.consecutive403 = 0
		n := a.nextAmbiguous401CountLocked()
		if n >= a.opts.Ambiguous401Escalation {
			a.consecutiveAmbiguous401 = 0
			a.dropMemoryLocked()
			a.dropDiskLocked()
			a.dropFailureStampLocked()
			a.mu.Unlock()
			logging.Warning("[llm/auth] %d consecutive ambiguous 401s on the same OAuth token: deleting cache %q to force re-authorization", n, a.cacheFile)
			return
		}
		a.saveAmbiguous401StampLocked(n)
		a.dropMemoryLocked()
		a.mu.Unlock()
		logging.Info("[llm/auth] ambiguous 401 on OAuth-authenticated request: dropping in-memory token but preserving on-disk cache %q (no structured invalid-token signal; count=%d/%d)", a.cacheFile, n, a.opts.Ambiguous401Escalation)
		return
	}
	if status == http.StatusForbidden {
		a.mu.Lock()
		a.consecutiveAmbiguous401 = 0
		a.dropFailureStampLocked()
		a.consecutive403++
		if a.consecutive403 >= consecutiveForbiddenEscalation {
			n := a.consecutive403
			a.consecutive403 = 0
			a.consecutiveAmbiguous401 = 0
			a.dropMemoryLocked()
			a.dropDiskLocked()
			a.dropFailureStampLocked()
			a.mu.Unlock()
			logging.Warning("[llm/auth] %d consecutive 403s on OAuth-authenticated request: the on-disk token appears revoked; deleting cache %q to force re-authorization", n, a.cacheFile)
			return
		}
		a.dropMemoryLocked()
		a.mu.Unlock()
		logging.Info("[llm/auth] 403 on OAuth-authenticated request: dropping in-memory token but preserving on-disk cache %q (403 is authorization/rate-limit, not token invalidation)", a.cacheFile)
		return
	}
	// Any other auth failure clears transient escalation state and deletes both
	// memory and disk.
	a.mu.Lock()
	a.consecutive403 = 0
	a.consecutiveAmbiguous401 = 0
	a.dropFailureStampLocked()
	a.mu.Unlock()
	a.Invalidate()
}

// dropMemoryLocked zeroes the in-memory token. Callers MUST hold a.mu.
func (a *oauthPollingAuthenticator) dropMemoryLocked() {
	a.token = cachedOAuthToken{}
}

// dropDiskLocked removes the on-disk token cache. Callers MUST hold a.mu.
func (a *oauthPollingAuthenticator) dropDiskLocked() {
	if a.cacheFile != "" {
		if err := os.Remove(a.cacheFile); err != nil && !os.IsNotExist(err) {
			logging.Warning("[llm/auth] could not remove token cache %q: %v", a.cacheFile, err)
		}
	}
}

func (a *oauthPollingAuthenticator) ambiguous401PreserveDisk() bool {
	if a.opts.Ambiguous401PreserveDisk == nil {
		return true
	}
	return *a.opts.Ambiguous401PreserveDisk
}

func (a *oauthPollingAuthenticator) hasInvalidTokenSignal(f authFailure) bool {
	valid := normalizedInvalidTokenCodes(a.opts.InvalidTokenErrorCodes)
	if len(valid) == 0 {
		return false
	}
	for _, code := range authErrorCodesFromWWWAuthenticate(f.Header) {
		if valid[normalizeAuthErrorCode(code)] {
			return true
		}
	}
	for _, code := range authErrorCodesFromJSONBody(f.Body) {
		if valid[normalizeAuthErrorCode(code)] {
			return true
		}
	}
	return false
}

func normalizedInvalidTokenCodes(codes []string) map[string]bool {
	out := map[string]bool{}
	for _, code := range codes {
		if norm := normalizeAuthErrorCode(code); norm != "" {
			out[norm] = true
		}
	}
	return out
}

func normalizeAuthErrorCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	code = strings.Trim(code, `"'`)
	code = strings.ToLower(code)
	code = strings.ReplaceAll(code, "-", "_")
	code = strings.ReplaceAll(code, " ", "_")
	return code
}

func authErrorCodesFromWWWAuthenticate(header http.Header) []string {
	if header == nil {
		return nil
	}
	var out []string
	for _, raw := range header.Values("WWW-Authenticate") {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if fields := strings.Fields(part); len(fields) > 1 {
				part = fields[len(fields)-1]
			}
			key, value, ok := strings.Cut(part, "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(key), "error") {
				continue
			}
			value = strings.TrimSpace(value)
			value = strings.Trim(value, `"'`)
			if value != "" {
				out = append(out, value)
			}
		}
	}
	return out
}

func authErrorCodesFromJSONBody(body []byte) []string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '{' {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil
	}
	var out []string
	collectAuthErrorCodes(obj, 0, &out)
	return out
}

func collectAuthErrorCodes(v any, depth int, out *[]string) {
	if depth > 3 {
		return
	}
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if authErrorCodeKey(k) {
				if s, ok := child.(string); ok && strings.TrimSpace(s) != "" {
					*out = append(*out, s)
					continue
				}
			}
			collectAuthErrorCodes(child, depth+1, out)
		}
	case []any:
		for _, child := range x {
			collectAuthErrorCodes(child, depth+1, out)
		}
	}
}

func authErrorCodeKey(key string) bool {
	switch normalizeAuthErrorCode(key) {
	case "error", "error_code", "err_code", "code", "errorcode":
		return true
	default:
		return false
	}
}

func (a *oauthPollingAuthenticator) nextAmbiguous401CountLocked() int {
	a.consecutiveAmbiguous401++
	stamp, err := a.loadFailureStampLocked()
	if err != nil || stamp.Count <= 0 {
		return a.consecutiveAmbiguous401
	}
	if stamp.Fingerprint != a.fingerprint ||
		stamp.TokenHash != hashTokenForStamp(a.token.AccessToken) ||
		stamp.Status != http.StatusUnauthorized ||
		stamp.Class != "ambiguous_401" {
		return a.consecutiveAmbiguous401
	}
	if stamp.Count >= a.consecutiveAmbiguous401 {
		a.consecutiveAmbiguous401 = stamp.Count + 1
	}
	return a.consecutiveAmbiguous401
}

func (a *oauthPollingAuthenticator) failureStampPathLocked() string {
	if strings.TrimSpace(a.cacheFile) == "" {
		return ""
	}
	return a.cacheFile + ".failures.json"
}

func (a *oauthPollingAuthenticator) loadFailureStampLocked() (oauthFailureStamp, error) {
	path := a.failureStampPathLocked()
	if path == "" {
		return oauthFailureStamp{}, errors.New("auth failure stamp disabled")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return oauthFailureStamp{}, err
	}
	var stamp oauthFailureStamp
	if err := json.Unmarshal(data, &stamp); err != nil {
		return oauthFailureStamp{}, err
	}
	return stamp, nil
}

func (a *oauthPollingAuthenticator) saveAmbiguous401StampLocked(count int) {
	path := a.failureStampPathLocked()
	if path == "" || count <= 0 {
		return
	}
	stamp := oauthFailureStamp{
		Fingerprint: a.fingerprint,
		TokenHash:   hashTokenForStamp(a.token.AccessToken),
		Status:      http.StatusUnauthorized,
		Class:       "ambiguous_401",
		Count:       count,
		UpdatedAt:   time.Now(),
	}
	if err := writeAuthFailureStamp(path, stamp); err != nil {
		logging.Warning("[llm/auth] could not write OAuth auth-failure stamp %q: %v", path, err)
	}
}

func (a *oauthPollingAuthenticator) dropFailureStampLocked() {
	path := a.failureStampPathLocked()
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		logging.Warning("[llm/auth] could not remove OAuth auth-failure stamp %q: %v", path, err)
	}
}

func writeAuthFailureStamp(path string, stamp oauthFailureStamp) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(stamp, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".oauth2-failure-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func hashTokenForStamp(token string) string {
	if strings.TrimSpace(token) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (a *oauthPollingAuthenticator) getToken(ctx context.Context) (cachedOAuthToken, error) {
	// a.token is a multi-word struct mutated under a.mu by this function
	// and Invalidate(). There is no unlocked fast path: an unsynchronized
	// read could observe a torn token (a fresh AccessToken paired with a
	// stale ExpiresAt/Fingerprint). The lock is held only briefly in the
	// common valid-token case, which is negligible next to the LLM round
	// trip this token authorizes.
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
	// No usable cached token: neither the in-memory copy nor the on-disk cache
	// yielded a valid token, so a full browser authorization round follows.
	// Emit the effective cache path here so a cold start that lands on the
	// wrong (unexpectedly relative / drifted) path is self-diagnosing before
	// the interactive prompt appears. No token material is logged.
	logging.Info("[llm/auth] no usable cached token; effective cache path=%q, starting authorization", a.cacheFile)
	tok, err := a.authorizeAndPoll(ctx)
	if err != nil {
		return cachedOAuthToken{}, err
	}
	a.token = tok
	// A freshly authorized token clears the consecutive-403 escalation counter:
	// this is the unambiguous "success" reset. We intentionally do NOT reset on
	// the in-memory / disk fast paths above, because the revocation-403 loop
	// re-reads exactly that disk token — resetting there would defeat the
	// escalation. Only a genuinely new credential counts as success. (Held
	// under a.mu for the whole getToken body.)
	a.consecutive403 = 0
	a.consecutiveAmbiguous401 = 0
	a.dropFailureStampLocked()
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
			tok := a.cacheFromResponse(resp, time.Now())
			if !emitOAuthAuthorizationCompleteNotice(OAuthAuthorizationCompleteNotice{CacheFile: a.cacheFile}) {
				if preferChineseDisplay() {
					fmt.Fprintln(os.Stderr, "  ✓ LLM OAuth 授权已完成，正在继续初始化。")
				} else {
					fmt.Fprintln(os.Stderr, "  ✓ LLM OAuth authorization complete; continuing startup.")
				}
			}
			return tok, nil
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
		// Classify the miss so cold-start path drift is self-evident in the
		// logs. A NotExist miss on first run is normal (INFO); other read
		// errors (permissions, IO) are WARN. No token material is logged.
		if os.IsNotExist(err) {
			logging.Info("[llm/auth] token cache miss (not_exist) at %q; will authorize", a.cacheFile)
		} else {
			logging.Warning("[llm/auth] token cache read error at %q: %v", a.cacheFile, err)
		}
		return cachedOAuthToken{}, err
	}
	var tok cachedOAuthToken
	if err := json.Unmarshal(data, &tok); err != nil {
		// Log only the error TYPE and (for a syntax error) the byte OFFSET —
		// never err.Error(), whose text for *json.SyntaxError can embed a
		// literal character copied out of the cache file. That file holds
		// token material, so echoing a fragment of it into the log would leak
		// a byte of secret. The offset is enough to locate the corruption.
		if se := (*json.SyntaxError)(nil); errors.As(err, &se) {
			logging.Info("[llm/auth] token cache miss (corrupt_json: syntax error at byte offset %d) at %q; will authorize", se.Offset, a.cacheFile)
		} else {
			logging.Info("[llm/auth] token cache miss (corrupt_json: %T) at %q; will authorize", err, a.cacheFile)
		}
		return cachedOAuthToken{}, err
	}
	if tok.Fingerprint != a.fingerprint {
		logging.Info("[llm/auth] token cache miss (fingerprint_mismatch) at %q; provider auth config changed, will authorize", a.cacheFile)
		return cachedOAuthToken{}, errors.New("token cache fingerprint mismatch")
	}
	if !a.tokenValid(tok, now) {
		logging.Info("[llm/auth] token cache miss (expired) at %q; token expired or within refresh_before window, will authorize", a.cacheFile)
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
	// Atomic write: create a temp file in the SAME directory (so the rename
	// stays on one filesystem and is a real atomic replace), write the
	// payload at 0600, then rename over the canonical path. A concurrent CLI
	// sharing this cache (same fingerprint = same file, by design) therefore
	// never observes a half-written JSON document — it reads either the old
	// complete file or the new complete file, never a torn one. On any
	// failure the temp file is removed so no 0600 turd is left behind.
	tmp, err := os.CreateTemp(dir, ".oauth2-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, a.cacheFile); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
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
	if p == "" {
		return p
	}
	// A lone "~" and a "~/"-prefixed path share one home-expansion semantics:
	// ~ == ~/ == home. Returning a bare "~" verbatim (the earlier behavior)
	// meant os.ReadFile("~") resolved it as a CWD-relative literal, which
	// reintroduced the working-directory drift the anchoring layer removes.
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return home
		}
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
