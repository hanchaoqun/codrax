package llm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// OpenAIAdapter implements the Adapter interface for OpenAI-compatible APIs.
//
// Sizing fields (maxOutputTokens / requestTimeout / retryMaxAttempts)
// are resolved by the factory layer from providers.yaml + code
// defaults; the adapter holds NO magic constants for these. This is
// the symmetry counterpart to ContextWindow on the input side: every
// cap that operators may want to tune is plumbed through the
// constructor, so a future deployment that needs a 60 KB output cap
// or a 30-minute timeout for a slow self-hosted model edits one yaml
// file instead of patching the adapter.
type OpenAIAdapter struct {
	apiKey  string
	model   string
	baseURL string

	chatCompletionsPath string
	modelsPath          string
	authenticator       requestAuthenticator
	requestHeaders      map[string]string
	requestExtra        map[string]any

	// maxOutputTokens is the wire-level `max_tokens` field sent on
	// every chat completion request. Resolved from
	// LLMProviderConfig.MaxOutputTokens / MaxOutputFraction /
	// ContextWindow via config.ResolveTokenBudget.
	//
	// Zero means "do NOT send max_tokens" — the server falls back to
	// the model's own output ceiling (typically 8K-128K depending on
	// the model). This is the default and the recommended setting:
	// every other LLM client (sdk, IDE assistant, langchain, etc.)
	// works this way, and capping output client-side is the root
	// cause of the emit_change_plan streaming-truncation failures we
	// observed. Operators set a positive value here only when they
	// want to bound cost / latency on a specific deploy.
	maxOutputTokens int

	// contextWindow is the deploy-time-declared max input token window
	// from providers.yaml :: context_window. Zero means "unknown" and
	// MaxContextTokens returns the historical 128000 fallback so
	// downstream consumers that assume a positive value (agent
	// pressure watchdog, fraction-form byte cap resolver) degrade
	// gracefully rather than divide by zero.
	contextWindow int

	// requestTimeout is the per-call HTTP timeout for chat completion
	// requests. Honored by httpClient.Timeout. Resolved from
	// LLMProviderConfig.RequestTimeoutSeconds via
	// config.ResolveDurationSeconds; always positive post-construction.
	requestTimeout time.Duration

	// retryMaxAttempts caps the 429/5xx retry loop in Chat. Resolved
	// from LLMProviderConfig.RetryMaxAttempts via
	// config.ResolveRetryAttempts; always positive post-construction.
	retryMaxAttempts int

	// streamStallTimeout is the SSE no-bytes ceiling the watchdog
	// uses to abort hung upstream streams. Resolved from
	// codrax.yaml :: llm_stream_stall_timeout_seconds; zero falls
	// through to defaultStreamStallTimeout (120s).
	streamStallTimeout time.Duration

	// streamFirstByteTimeout is the no-usable-SSE-data ceiling the
	// watchdog uses BEFORE the first assistant progress chunk arrives.
	// Distinct from streamStallTimeout because "request accepted but
	// server never speaks" must fail fast (typical first-byte is
	// 100-500ms even on slow providers), while mid-stream pauses on
	// thinking models legitimately reach 60s+.
	// Resolved from providers.yaml :: stream_first_byte_timeout_seconds;
	// zero falls through to defaultStreamFirstByteTimeout (40s).
	streamFirstByteTimeout time.Duration

	stream bool
	// recoverTextToolCalls is an opt-in provider compatibility shim for
	// small OpenAI-compatible models that write tool-call envelopes into
	// assistant content instead of the protocol tool_calls field.
	recoverTextToolCalls bool
	// providerThinkingMode controls provider-native thinking / reasoning
	// switches. It is independent of Codrax's prompt-side ThinkAloud
	// setting: ThinkAloud asks the model to narrate before tool calls;
	// this field controls request JSON such as DeepSeek's
	// {"thinking":{"type":"disabled"}}.
	providerThinkingMode string
	// httpClient is the non-streaming client; honors requestTimeout
	// as the outer cap on a single round-trip.
	httpClient *http.Client
	// streamHTTPClient is the streaming client with NO outer timeout
	// — cancellation is owned exclusively by the per-request context
	// + the streamStallTimeout watchdog (which cancels when no bytes
	// arrive for streamStallTimeout). Mixing an outer http.Client
	// timeout with watchdog-driven streaming caused premature
	// "context deadline exceeded" failures during long emit_change_plan
	// / emit_answer_document streams (Batch L forth-py, sgf-parsing) —
	// the upstream was actively producing bytes, just slowly enough
	// that the cumulative wall hit the 120 s cap. Watchdog already
	// catches the only case the outer timeout is meant to catch
	// (genuinely stalled connection), so the outer timeout is pure
	// false-positive risk on the streaming path.
	streamHTTPClient *http.Client
}

// TLSOptions carries the per-provider TLS knobs loaded from
// providers.yaml. Zero value = system defaults.
type TLSOptions struct {
	// CAFile is a path to a PEM-encoded CA bundle appended to the
	// system trust pool. When set the http.Client's TLS config uses a
	// RootCAs pool seeded from the system pool + this file.
	CAFile string
	// InsecureSkipVerify disables certificate validation entirely.
	// Only use against an endpoint you fully control for short-lived
	// debugging — any on-path attacker can steal the API key.
	InsecureSkipVerify bool
}

// AdapterOptions bundles the pre-resolved sizing knobs the factory
// layer hands to NewOpenAIAdapter. Keeping them in a struct (rather
// than positional args) makes the call site self-documenting and
// future-proofs the constructor against another sizing parameter
// being added — callers continue to compile because zero-value
// fields trip the "must be positive" guard in NewOpenAIAdapter.
type AdapterOptions struct {
	Stream               bool
	RecoverTextToolCalls bool
	ProviderThinkingMode string
	ContextWindow        int
	MaxOutputTokens      int
	RequestTimeout       time.Duration
	RetryMaxAttempts     int
	TLS                  TLSOptions

	ChatCompletionsPath string
	ModelsPath          string
	Auth                AuthOptions
	RequestHeaders      map[string]string
	RequestExtra        map[string]any

	// StreamStallTimeout is how long the SSE scanner may go without
	// receiving a single byte before the watchdog aborts the request.
	// Zero falls through to the package default
	// defaultStreamStallTimeout (120s). Surfaces in codrax.yaml as
	// llm_stream_stall_timeout_seconds; cmd/root.go threads the
	// resolved value here.
	StreamStallTimeout time.Duration

	// StreamFirstByteTimeout is the no-usable-SSE-data ceiling that
	// applies BEFORE the first assistant progress chunk arrives. Zero
	// falls through to defaultStreamFirstByteTimeout (40s). Distinct
	// from StreamStallTimeout — see field doc on OpenAIAdapter.
	StreamFirstByteTimeout time.Duration
}

// NewOpenAIAdapter creates a new OpenAI adapter. apiKey, model, and
// baseURL are all required — the factory layer is responsible for
// rejecting configs that omit any of them. This also works with
// Azure OpenAI, DeepSeek, Ollama, and other OpenAI-compatible APIs.
//
// AdapterOptions carries pre-resolved sizing (output cap / HTTP
// timeout / retry attempts) — the factory plumbs yaml → resolver →
// here, so the adapter itself owns no magic constants.
//
// Field semantics:
//   - MaxOutputTokens: 0 means "don't send max_tokens on the wire"
//     (server uses the model's own ceiling — the recommended default).
//     A positive value is sent verbatim as `max_tokens`.
//   - RequestTimeout / RetryMaxAttempts: must be positive. Both have
//     mandatory code defaults applied at the factory layer; a zero
//     here would silently disable HTTP timeout (= hang forever) or
//     retries (= one shot, breaks 429 rotation), so we panic to fail
//     loud at construction.
func NewOpenAIAdapter(apiKey, model, baseURL string, opts AdapterOptions) *OpenAIAdapter {
	adapter, err := NewOpenAIAdapterWithError(apiKey, model, baseURL, opts)
	if err != nil {
		panic(err)
	}
	return adapter
}

func NewOpenAIAdapterWithError(apiKey, model, baseURL string, opts AdapterOptions) (*OpenAIAdapter, error) {
	if opts.RequestTimeout <= 0 {
		panic("llm: AdapterOptions.RequestTimeout must be positive (factory must apply code default)")
	}
	if opts.RetryMaxAttempts <= 0 {
		panic("llm: AdapterOptions.RetryMaxAttempts must be positive (factory must apply code default)")
	}
	stallTimeout := opts.StreamStallTimeout
	if stallTimeout <= 0 {
		stallTimeout = defaultStreamStallTimeout
	}
	firstByteTimeout := opts.StreamFirstByteTimeout
	if firstByteTimeout <= 0 {
		firstByteTimeout = defaultStreamFirstByteTimeout
	}
	authenticator, err := newRequestAuthenticator(apiKey, opts.Auth)
	if err != nil {
		return nil, err
	}
	chatPath := strings.TrimSpace(opts.ChatCompletionsPath)
	if chatPath == "" {
		chatPath = "/chat/completions"
	}
	httpClient, err := buildHTTPClient(opts.TLS, baseURL, opts.RequestTimeout)
	if err != nil {
		return nil, err
	}
	streamHTTPClient, err := buildStreamHTTPClient(opts.TLS, baseURL, firstByteTimeout)
	if err != nil {
		return nil, err
	}
	return &OpenAIAdapter{
		apiKey:                 apiKey,
		model:                  model,
		baseURL:                baseURL,
		chatCompletionsPath:    chatPath,
		modelsPath:             opts.ModelsPath,
		authenticator:          authenticator,
		requestHeaders:         cloneStringMap(opts.RequestHeaders),
		requestExtra:           cloneRawMessageMapLLM(opts.RequestExtra),
		maxOutputTokens:        opts.MaxOutputTokens,
		contextWindow:          opts.ContextWindow,
		requestTimeout:         opts.RequestTimeout,
		retryMaxAttempts:       opts.RetryMaxAttempts,
		streamStallTimeout:     stallTimeout,
		streamFirstByteTimeout: firstByteTimeout,
		stream:                 opts.Stream,
		recoverTextToolCalls:   opts.RecoverTextToolCalls,
		providerThinkingMode:   normalizeProviderThinkingMode(opts.ProviderThinkingMode),
		httpClient:             httpClient,
		// Streaming client gets NO outer timeout (Duration(0)); the
		// per-request context + streamStallTimeout watchdog own
		// cancellation after response headers arrive. The transport
		// still caps response-header wait with firstByteTimeout so a
		// provider that never starts the stream cannot hang before the
		// watchdog exists.
		streamHTTPClient: streamHTTPClient,
	}, nil
}

// buildHTTPClient assembles the per-provider http.Client. When the
// caller supplies TLS overrides, a fresh Transport is attached with
// the requested TLS config; otherwise the default Transport is used
// (http.DefaultTransport won't be modified, so concurrent providers
// don't stomp each other). A configured tls_ca_file is fail-loud:
// missing/unreadable/invalid PEM files return an error so startup
// stops instead of silently falling back to a different trust model.
func buildHTTPClient(tlsOpts TLSOptions, baseURL string, timeout time.Duration) (*http.Client, error) {
	return buildHTTPClientWithTransportOptions(tlsOpts, baseURL, timeout, 0)
}

func buildStreamHTTPClient(tlsOpts TLSOptions, baseURL string, responseHeaderTimeout time.Duration) (*http.Client, error) {
	return buildHTTPClientWithTransportOptions(tlsOpts, baseURL, 0, responseHeaderTimeout)
}

func buildHTTPClientWithTransportOptions(tlsOpts TLSOptions, baseURL string, timeout, responseHeaderTimeout time.Duration) (*http.Client, error) {
	if tlsOpts.CAFile == "" && !tlsOpts.InsecureSkipVerify && responseHeaderTimeout <= 0 {
		return &http.Client{Timeout: timeout}, nil
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if responseHeaderTimeout > 0 {
		transport.ResponseHeaderTimeout = responseHeaderTimeout
	}
	tlsCfg := transport.TLSClientConfig
	if tlsCfg == nil {
		tlsCfg = &tls.Config{}
	} else {
		tlsCfg = tlsCfg.Clone()
	}
	customTLS := false

	if tlsOpts.CAFile != "" {
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		pem, readErr := os.ReadFile(tlsOpts.CAFile)
		if readErr != nil {
			return nil, fmt.Errorf("providers.yaml: tls_ca_file %q is configured but cannot be read: %w", tlsOpts.CAFile, readErr)
		} else if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("providers.yaml: tls_ca_file %q is configured but contains no valid PEM certificates", tlsOpts.CAFile)
		} else {
			logging.Info("[llm/tls] appended custom CA bundle %q to system trust pool for %s", tlsOpts.CAFile, baseURL)
			tlsCfg.RootCAs = pool
			customTLS = true
		}
	}

	if tlsOpts.InsecureSkipVerify {
		tlsCfg.InsecureSkipVerify = true
		customTLS = true
		logging.Warning("[llm/tls] ! tls_insecure_skip_verify=true for %s — certificate validation DISABLED, API key is vulnerable to on-path interception", baseURL)
		fmt.Fprintf(os.Stderr, "  ! TLS verification DISABLED for %s (tls_insecure_skip_verify=true)\n", baseURL)
	}
	if customTLS {
		transport.TLSClientConfig = tlsCfg
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}, nil
}

func (o *OpenAIAdapter) ModelID() string { return o.model }

// MaxContextTokens returns the configured context_window from
// providers.yaml. When zero (not declared), returns the 2026-05-02
// default of 200000 — recent model upgrades broadly raised effective
// context windows, and the 128000 historical fallback was triggering
// premature context truncation on complex tasks. User-set
// providers.yaml :: context_window still wins; if a model's actual
// limit is below the default, callers consulting model-specific caps
// override accordingly. Any consumer treating a positive return value
// as ground truth (fraction-cap resolver, pressure watchdog) keeps
// working.
func (o *OpenAIAdapter) MaxContextTokens() int {
	if o.contextWindow > 0 {
		return o.contextWindow
	}
	return DefaultContextWindow
}

// DefaultContextWindow is the system-wide default for adapters that
// don't declare context_window in providers.yaml. Bumped from 128000
// to 200000 on 2026-05-02 to track widespread model-upgrade headroom.
// Operators with smaller models should declare context_window
// explicitly so the adapter matches their model's true ceiling.
const DefaultContextWindow = 200000

// MaxOutputTokens returns the resolved client-side output cap. Zero
// means "no cap, server uses the model's own ceiling" — the default.
func (o *OpenAIAdapter) MaxOutputTokens() int { return o.maxOutputTokens }

// RequestTimeout returns the per-call HTTP timeout the adapter was
// constructed with. Used by cmd/root.go to surface effective values
// in the startup log so operators can sanity-check yaml resolution.
func (o *OpenAIAdapter) RequestTimeout() time.Duration { return o.requestTimeout }

// RetryMaxAttempts returns the resolved attempt count for the
// transient-error retry loop (429 / 5xx).
func (o *OpenAIAdapter) RetryMaxAttempts() int { return o.retryMaxAttempts }

// retryReasonForError produces a short human phrase classifying why
// the adapter is about to retry. Consumed by the OnRetry callback so
// the renderer dock can render "重试中（rate limit）" / "stream
// stalled" instead of an opaque "重试中". Order matters: more
// specific classifications first.
func retryReasonForError(err error) string {
	if err == nil {
		return "transient"
	}
	if errors.Is(err, ErrStreamFirstByteTimeout) {
		return "stream first-byte timeout"
	}
	if ae, ok := err.(*apiError); ok {
		if ae.QuotaError {
			return "quota"
		}
		if ae.StatusCode == 429 {
			return "rate limit"
		}
		if ae.StatusCode >= 500 {
			return fmt.Sprintf("server %d", ae.StatusCode)
		}
	}
	return "transient"
}

func (o *OpenAIAdapter) Chat(ctx context.Context, messages []Message, tools []ToolSchema, opts ChatOptions) (Response, error) {
	if ctx == nil {
		// Reject nil ctx fail-loud: callers MUST pass either a real
		// cancel-aware ctx or context.Background() to declare "no
		// cancel". Silent fallback to context.Background() would mask
		// integration mistakes where Ctrl+C should have flowed.
		return Response{}, fmt.Errorf("llm: Chat called with nil ctx (callers must pass at least context.Background())")
	}
	reqBody := o.buildRequest(messages, tools, opts)

	bodyBytes, err := o.marshalRequest(reqBody)
	if err != nil {
		return Response{}, fmt.Errorf("marshal request: %w", err)
	}

	var resp Response
	var lastErr error

	// Retry transient errors with exponential backoff. 429 covers both
	// true rate-limit and Azure-style "No deployments available for
	// selected model" (deployment rotation); the latter can take 30-60s
	// to clear, which the old 3-attempt × 1s/2s/4s schedule (~7s total)
	// routinely burned through. The default 6 attempts with 2/4/8/16/32s
	// backoff buys ~62s of coverage — enough for typical deployment
	// rotations without tripping into a total stall when something is
	// genuinely wrong upstream. Operators can tune via providers.yaml::
	// retry_max_attempts when their upstream's rotation pattern needs
	// more / less coverage.
	maxAttempts := o.retryMaxAttempts
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// ctx-aware cancel: exit retry loop immediately when the
		// caller's ctx is already canceled. Pre-Phase-2 the loop
		// would burn through all attempts before the cooperative
		// agent-loop checkpoint noticed and returned ErrCanceled.
		if cerr := ctx.Err(); cerr != nil {
			return Response{}, cerr
		}
		logging.Debug("[llm] request: model=%s attempt=%d/%d stream=%t messages=%d tools=%d body_bytes=%d timeout=%s first_byte_timeout=%s stall_timeout=%s",
			o.model, attempt+1, maxAttempts, o.stream, len(messages), len(tools), len(bodyBytes),
			o.requestTimeout, o.streamFirstByteTimeout, o.streamStallTimeout)
		if o.stream {
			resp, err = o.doStreamRequest(ctx, bodyBytes, opts.OnContentDelta, opts.OnToolCallDelta)
		} else {
			resp, err = o.doRequest(ctx, bodyBytes)
		}
		if err == nil {
			resp = recoverStructuredTextToolCalls(resp, tools, opts)
			if o.recoverTextToolCalls {
				resp = recoverTextToolCalls(resp, tools, opts)
			}
			// Diagnostic floor: every successful response logs its
			// finish_reason + completion_tokens at debug, escalating
			// to a warning when the server hit a length cap. This
			// closes the diagnostic blind spot that turned the
			// emit_change_plan truncation into a multi-round
			// investigation — any future client-side or model-side
			// output truncation is now visible in one log line.
			if resp.StopReason == "max_tokens" {
				logging.Warning("[llm] response: model=%s finish_reason=length output_tokens=%d cap=%d (request hit output cap — set max_output_tokens=0 in providers.yaml to remove client-side cap)",
					o.model, resp.Usage.OutputTokens, o.maxOutputTokens)
			} else {
				logging.Debug("[llm] response: model=%s finish_reason=%s output_tokens=%d cap=%d",
					o.model, resp.StopReason, resp.Usage.OutputTokens, o.maxOutputTokens)
			}
			return resp, nil
		}
		lastErr = err

		// L1 retry policy:
		//   - HTTP 429 / 5xx (isRetryable): always — provider may
		//     clear after backoff (rate limit / deployment rotation)
		//   - ErrStreamFirstByteTimeout: provably safe — by
		//     definition no callback fired before this error so a
		//     fresh attempt cannot duplicate user-visible state
		//
		// NOT retried at L1:
		//   - io.ErrUnexpectedEOF / mid-stream stalled: callbacks
		//     may have fired with partial content; replaying would
		//     duplicate. Outer layers (force-finalize) handle these
		//     after L1 returns, with the streaming partial-salvage
		//     path picking up whatever was already streamed.
		if !isRetryable(err) && !errors.Is(err, ErrStreamFirstByteTimeout) {
			return Response{}, err
		}

		// Backoff schedule (last attempt fires without a trailing sleep):
		//   - default: exponential 2s/4s/8s/16s/32s (~62s coverage,
		//     matches typical deployment rotation duration)
		//   - server Retry-After present: honor it (RFC 7231)
		//   - quota error (rate_limit_error / token plan / 套餐 / 配额):
		//     4× longer exponential 8s/16s/32s/64s/128s (~4 min) so a
		//     true quota stall gets a real chance to clear before the
		//     budget is exhausted
		// See nextRetryDelay for precedence.
		if attempt < maxAttempts-1 {
			delay := nextRetryDelay(err, attempt)
			if ae, ok := err.(*apiError); ok && (ae.RetryAfter > 0 || ae.QuotaError) {
				logging.Info("[llm] retry %d/%d after %v (status=%d quota=%v retry_after=%v)",
					attempt+1, maxAttempts-1, delay, ae.StatusCode, ae.QuotaError, ae.RetryAfter)
			}
			// Notify the renderer (or any other observer) so the dock
			// status row can flip to "重试中" while we sleep — without
			// this the user sees a stale "请求模型中" for the entire
			// backoff window. Panic-recovered so a buggy callback can
			// never wedge the retry loop.
			if cb := opts.OnRetry; cb != nil {
				func() {
					defer func() { _ = recover() }()
					cb(attempt+1, delay, retryReasonForError(err))
				}()
			}
			// Cancellation-aware sleep: a quota error can request a
			// 128s backoff, during which a user /cancel must NOT be
			// stranded waiting. select on ctx.Done so the retry loop
			// bails immediately when the orchestrator pulls the
			// cancellation token. Falls through to time.NewTimer on a
			// nil ctx (test fixtures pass context.Background which has
			// a non-nil Done channel that never fires).
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return Response{}, ctx.Err()
			case <-timer.C:
			}
		}
	}

	// Wrap the terminal error with ErrAllRetriesExhausted so outer
	// layers (FallbackAdapter, scheduler transient retry, force-
	// finalize) detect L1 exhaustion via errors.Is and skip duplicate
	// retry coverage. The underlying *apiError / network error stays
	// reachable via errors.As for diagnostics.
	return Response{}, fmt.Errorf("%w: %w", ErrAllRetriesExhausted, lastErr)
}

func (o *OpenAIAdapter) doRequest(ctx context.Context, bodyBytes []byte) (Response, error) {
	for authAttempt := 0; authAttempt < 2; authAttempt++ {
		req, err := o.newChatRequest(ctx, bodyBytes, false)
		if err != nil {
			return Response{}, err
		}
		httpResp, err := o.httpClient.Do(req)
		if err != nil {
			return Response{}, fmt.Errorf("http request: %w", err)
		}
		respBody, readErr := io.ReadAll(httpResp.Body)
		_ = httpResp.Body.Close()
		if readErr != nil {
			return Response{}, fmt.Errorf("read response: %w", readErr)
		}
		if httpResp.StatusCode == http.StatusOK {
			return o.parseResponse(respBody)
		}
		if isAuthStatus(httpResp.StatusCode) && authAttempt == 0 && o.authenticator != nil {
			o.authenticator.Invalidate()
			continue
		}
		return Response{}, newAPIError(httpResp, respBody)
	}
	return Response{}, errors.New("llm auth retry exhausted")
}

// doStreamRequest POSTs a streaming chat completion and accumulates
// the incremental SSE chunks into a single Response. Content deltas
// fire the optional onDelta callback as they arrive so the agent
// layer can surface a live preview. Tool-call argument deltas are
// accumulated AND optionally fired through onToolCallDelta as they
// arrive. The accumulator's behaviour is unchanged when
// onToolCallDelta is nil — adding the callback is purely a passive
// read-side tap so the renderer can show a streaming preview of the
// summary field without disturbing the canonical Args parse.
func (o *OpenAIAdapter) doStreamRequest(ctx context.Context, bodyBytes []byte, onDelta func(string), onToolCallDelta func(int, string, string)) (Response, error) {
	for authAttempt := 0; authAttempt < 2; authAttempt++ {
		resp, err := o.doStreamRequestOnce(ctx, bodyBytes, onDelta, onToolCallDelta)
		if err == nil {
			return resp, nil
		}
		var ae *apiError
		if errors.As(err, &ae) && isAuthStatus(ae.StatusCode) && authAttempt == 0 && o.authenticator != nil {
			o.authenticator.Invalidate()
			continue
		}
		return resp, err
	}
	return Response{}, errors.New("llm stream auth retry exhausted")
}

func (o *OpenAIAdapter) doStreamRequestOnce(ctx context.Context, bodyBytes []byte, onDelta func(string), onToolCallDelta func(int, string, string)) (Response, error) {
	// Stall-detection scaffold: a request-scoped context lets a
	// watchdog goroutine cancel the in-flight HTTP body read when
	// the upstream stops sending useful stream progress for too
	// long. The streaming HTTP client intentionally has no global
	// http.Client.Timeout so legitimate long answers are not killed
	// while bytes are flowing; instead the watchdog enforces three
	// product-level caps:
	//   - no first usable SSE chunk within streamFirstByteTimeout;
	//   - no SSE progress after the stream starts within streamStallTimeout;
	//   - no visible assistant/tool output within requestTimeout even
	//     if hidden reasoning chunks keep the TCP/SSE stream active.
	//
	// Bug provenance: eval Batch I+J sgf-parsing — planner LLM
	// streamed initial reasoning, then upstream went silent for
	// >120 s (model stuck mid-emit). The full client timeout fired,
	// burning the entire plan stage with zero useful output.
	// Watchdog catches this earlier (30 s of silence vs 120 s of
	// total) and surfaces a distinct error message so the planner
	// retry loop can react instead of waiting on the 120 s cap.
	// Derive from the caller-supplied ctx so an outer Cancel
	// (Ctrl+C / /cancel / orchestrator timeout) propagates to the
	// streaming HTTP request immediately. The watchdog cancel
	// path remains the inner mechanism for "no bytes for N
	// seconds"; the new outer parent makes Phase 2 user-cancel
	// HTTP-level instead of cooperative.
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	firstByteTimeout := o.streamFirstByteTimeout
	if firstByteTimeout <= 0 {
		firstByteTimeout = defaultStreamFirstByteTimeout
	}
	req, err := o.newChatRequest(reqCtx, bodyBytes, true)
	if err != nil {
		return Response{}, err
	}

	preHeaderStart := time.Now()
	var preHeaderFired atomic.Bool
	preHeaderDone := make(chan struct{})
	preHeaderTimer := time.AfterFunc(firstByteTimeout, func() {
		defer close(preHeaderDone)
		preHeaderFired.Store(true)
		logging.Warning("[llm/stream] no response headers for %v (firstByteTimeout=%v); aborting", time.Since(preHeaderStart), firstByteTimeout)
		cancel()
	})
	httpResp, err := o.streamHTTPClient.Do(req)
	if !preHeaderTimer.Stop() {
		<-preHeaderDone
	}
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return Response{}, cerr
		}
		if preHeaderFired.Load() {
			return Response{}, &StreamFirstByteTimeoutError{
				IdleFor: time.Since(preHeaderStart),
				Cause:   fmt.Errorf("http request before response headers: %w", err),
			}
		}
		if isResponseHeaderTimeout(err) {
			return Response{}, &StreamFirstByteTimeoutError{
				IdleFor: firstByteTimeout,
				Cause:   fmt.Errorf("http request: %w", err),
			}
		}
		return Response{}, fmt.Errorf("http request: %w", err)
	}
	if preHeaderFired.Load() {
		_ = httpResp.Body.Close()
		return Response{}, &StreamFirstByteTimeoutError{
			IdleFor: time.Since(preHeaderStart),
			Cause:   context.Canceled,
		}
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return Response{}, newAPIError(httpResp, body)
	}
	bodyClosed := make(chan struct{})
	go func() {
		select {
		case <-reqCtx.Done():
			_ = httpResp.Body.Close()
		case <-bodyClosed:
		}
	}()
	defer close(bodyClosed)

	// Start the stall watchdog. Tracker stores the Unix-nano time of
	// the last usable assistant progress chunk read by the SSE scanner;
	// watchdog ticks every 5 s and cancels the request context when
	// the gap exceeds streamStallTimeout. Cancellation closes the
	// response body, so the scanner's next Read call returns an error
	// and parseSSEStream exits with a stalled-stream error.
	//
	// watchdogFired distinguishes "we cancelled the stream because
	// upstream stalled" from "the surrounding ReAct loop / user
	// Ctrl+C cancelled the request" so the post-Scan error wrapper
	// below can attach the typed StreamStalledError sentinel. Without
	// this differentiation the resulting "context canceled" error
	// surfaces in the REPL as "request interrupted (likely Ctrl+C
	// or upstream connection closed)" — misleading the user into
	// thinking they pressed Ctrl+C when really upstream stalled.
	var lastReadNano atomic.Int64
	lastReadNano.Store(time.Now().UnixNano())
	var lastVisibleNano atomic.Int64
	lastVisibleNano.Store(time.Now().UnixNano())
	var firstByteReceived atomic.Bool
	var watchdogFired atomic.Bool
	var firstByteFired atomic.Bool
	var noVisibleFired atomic.Bool
	var watchdogIdle atomic.Int64  // ns
	var noVisibleIdle atomic.Int64 // ns
	watchDone := make(chan struct{})
	stallTimeout := o.streamStallTimeout
	if stallTimeout <= 0 {
		stallTimeout = defaultStreamStallTimeout
	}
	visibleTimeout := o.requestTimeout
	// Tick interval is the smaller of the two thresholds / 4 to give
	// the SHORTER timeout adequate detection resolution. Bounded
	// above by streamStallTickInterval (5s) so long stallTimeouts
	// don't drift the tick to wasteful tens of seconds.
	tickInterval := streamStallTickInterval
	if firstByteTimeout/4 < tickInterval {
		tickInterval = firstByteTimeout / 4
	}
	if tickInterval < streamWatchdogMinTickInterval {
		tickInterval = streamWatchdogMinTickInterval
	}
	go func() {
		defer close(watchDone)
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-reqCtx.Done():
				return
			case <-ticker.C:
				idle := time.Since(time.Unix(0, lastReadNano.Load()))
				// Two-threshold gate: pre-firstByte uses the SHORTER
				// firstByteTimeout (typical 40s); post-firstByte
				// switches to the LONGER stallTimeout (typical 120s).
				// Keeps the dead-on-arrival "provider never speaks"
				// case failing fast without compromising thinking-
				// model mid-stream pauses.
				if !firstByteReceived.Load() {
					if idle > firstByteTimeout {
						logging.Warning("[llm/stream] no usable first SSE data for %v (firstByteTimeout=%v); aborting", idle, firstByteTimeout)
						watchdogIdle.Store(int64(idle))
						firstByteFired.Store(true)
						cancel()
						return
					}
				} else {
					if idle > stallTimeout {
						logging.Warning("[llm/stream] stream stalled mid-stream (no bytes for %v, threshold=%v); aborting (outer http timeout was %v)", idle, stallTimeout, o.requestTimeout)
						watchdogIdle.Store(int64(idle))
						watchdogFired.Store(true)
						cancel()
						return
					}
				}
				if visibleTimeout > 0 && firstByteReceived.Load() {
					visibleIdle := time.Since(time.Unix(0, lastVisibleNano.Load()))
					if visibleIdle > visibleTimeout {
						logging.Warning("[llm/stream] stream produced no visible assistant/tool output for %v (requestTimeout=%v) while SSE was active; aborting", visibleIdle, visibleTimeout)
						noVisibleIdle.Store(int64(visibleIdle))
						noVisibleFired.Store(true)
						cancel()
						return
					}
				}
			}
		}
	}()

	resp, err := parseSSEStreamTracked(httpResp.Body, onDelta, onToolCallDelta, &lastReadNano, &firstByteReceived, &lastVisibleNano)
	cancel()
	<-watchDone
	if err != nil {
		// Disambiguate the cancel cause, in priority order:
		//   1. outer ctx cancel (user Ctrl+C / orch timeout) → return
		//      ctx.Err() so existing context.Canceled matchers fire.
		//   2. firstByteFired → no usable SSE data after handshake
		//      within firstByteTimeout. Distinct typed error so the REPL's
		//      friendlyRunError surfaces "provider never started
		//      streaming" rather than "stalled mid-stream".
		//   3. watchdogFired → mid-stream stall after first byte.
		// Order matters: user intent (1) wins over both watchdog
		// branches; first-byte (2) wins over generic stall because
		// it carries more specific operator guidance.
		if cerr := ctx.Err(); cerr != nil {
			err = cerr
		} else if firstByteFired.Load() {
			err = &StreamFirstByteTimeoutError{
				IdleFor: time.Duration(watchdogIdle.Load()),
				Cause:   err,
			}
		} else if noVisibleFired.Load() {
			err = &StreamNoVisibleOutputTimeoutError{
				IdleFor: time.Duration(noVisibleIdle.Load()),
				Cause:   err,
			}
		} else if watchdogFired.Load() {
			err = &StreamStalledError{
				IdleFor: time.Duration(watchdogIdle.Load()),
				Cause:   err,
			}
		}
	}
	return resp, err
}

func (o *OpenAIAdapter) newChatRequest(ctx context.Context, bodyBytes []byte, stream bool) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", joinURLPath(o.baseURL, o.chatCompletionsPath), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	if o.authenticator != nil {
		if err := o.authenticator.Apply(ctx, req); err != nil {
			return nil, fmt.Errorf("apply llm auth: %w", err)
		}
	}
	o.applyConfiguredHeaders(req)
	return req, nil
}

func (o *OpenAIAdapter) applyConfiguredHeaders(req *http.Request) {
	applyConfiguredHeaders(req, o.requestHeaders)
}

func applyConfiguredHeaders(req *http.Request, headers map[string]string) {
	for k, v := range headers {
		key := strings.TrimSpace(k)
		if key == "" || isReservedConfiguredHeader(key) {
			continue
		}
		req.Header.Set(key, expandHeaderValue(v))
	}
}

func isReservedConfiguredHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "authorization", "x-auth-token", "content-type", "accept":
		return true
	default:
		return false
	}
}

func expandHeaderValue(v string) string {
	switch strings.TrimSpace(v) {
	case "@uuid_v4":
		if id, err := uuidV4String(); err == nil {
			return id
		}
	}
	return v
}

func uuidV4String() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	s := hex.EncodeToString(buf)
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32], nil
}

func isAuthStatus(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden
}

func isResponseHeaderTimeout(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "timeout awaiting response headers") ||
		strings.Contains(msg, "ResponseHeaderTimeout")
}

// defaultStreamStallTimeout is the package-level fallback for the
// per-adapter o.streamStallTimeout. Hit when AdapterOptions doesn't
// supply a value and providers.yaml didn't tune the knob. Thinking
// models pause for several seconds between content chunks, deep-
// reasoning models routinely pause 60+ seconds between thinking
// blocks, so anything below ~60 s false-positives on legitimate slow
// upstreams. 120 s is the conservative middle ground; operators tune
// per-provider via providers.yaml :: stream_stall_timeout_seconds.
//
// streamStallTickInterval is how often the watchdog goroutine polls
// the idle counter. 5 s gives sub-default-cap reaction time without
// consuming meaningful CPU on long streams.
const (
	defaultStreamStallTimeout     = 120 * time.Second
	defaultStreamFirstByteTimeout = 40 * time.Second
	streamStallTickInterval       = 5 * time.Second
	// streamWatchdogMinTickInterval is the floor on the watchdog's
	// poll interval when firstByteTimeout / 4 would otherwise drop
	// below it. 100 ms gives sub-second cancel responsiveness without
	// burning CPU on long-lived streams (the watchdog is a single
	// goroutine, an extra 10 polls/sec is negligible vs the SSE
	// scanner work). Not operator-tunable: the 100 ms bound is a
	// kernel-scheduling sanity floor, not a knob worth exposing.
	streamWatchdogMinTickInterval = 100 * time.Millisecond
)

// parseSSEStream reads SSE frames from r and folds them into a single
// Response. Factored out so the parser is unit-testable without a
// live HTTP server.
//
// Wire format (per OpenAI streaming spec):
//
//	data: {"choices":[{"delta":{"content":"Hello"},"index":0}]}
//	data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_x","function":{"name":"emit_analysis","arguments":""}}]}}]}
//	data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"k\""}}]}}]}
//	data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}
//	data: [DONE]
//
// Key rules the accumulator enforces:
//   - delta.content chunks are appended to the running buffer
//   - delta.tool_calls[].index identifies which tool call the deltas
//     belong to; id and name usually arrive on the first chunk,
//     arguments accumulate across subsequent chunks
//   - finish_reason appears in the last non-DONE chunk
//   - usage typically ships in a dedicated last chunk (stream_options),
//     but providers vary; we read it when present
func parseSSEStream(r io.Reader, onDelta func(string), onToolCallDelta func(int, string, string), progress *atomic.Int64, firstByte *atomic.Bool) (Response, error) {
	return parseSSEStreamTracked(r, onDelta, onToolCallDelta, progress, firstByte, nil)
}

func parseSSEStreamTracked(r io.Reader, onDelta func(string), onToolCallDelta func(int, string, string), progress *atomic.Int64, firstByte *atomic.Bool, visibleProgress *atomic.Int64) (Response, error) {
	br := bufio.NewScanner(r)
	// Single SSE frames can exceed bufio's default 64 KB line cap when
	// a provider batches many deltas into one frame; raise to 1 MB to
	// match the upstream response size budget.
	br.Buffer(make([]byte, 64*1024), 1<<20)

	var contentBuf strings.Builder
	var reasoningBuf strings.Builder
	toolCalls := map[int]*openaiToolCall{}
	toolCallOrder := []int{}
	finishReason := ""
	usage := openaiUsage{}
	gotAnyChunk := false

	for br.Scan() {
		line := br.Text()
		// SSE lines before the data payload are event names / comments
		// / blank separators. Only "data: <payload>" lines matter for
		// chat-completion streams.
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string                `json:"content"`
					ReasoningContent string                `json:"reasoning_content"`
					Role             string                `json:"role"`
					ToolCalls        []openaiToolCallDelta `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *openaiUsage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			// A malformed chunk is not fatal — some providers emit
			// heartbeats / keep-alive comments outside the spec. Log
			// and skip; if the stream never produces a usable chunk
			// the final gotAnyChunk check fails loudly.
			logging.Debug("[llm/stream] skip malformed chunk: %v payload=%q", err, payload)
			continue
		}
		chunkProgress := chunk.Usage != nil
		visibleChunkProgress := false
		for _, ch := range chunk.Choices {
			if strings.TrimSpace(ch.FinishReason) != "" ||
				ch.Delta.Content != "" ||
				ch.Delta.ReasoningContent != "" ||
				strings.TrimSpace(ch.Delta.Role) != "" ||
				len(ch.Delta.ToolCalls) > 0 {
				chunkProgress = true
			}
			if strings.TrimSpace(ch.FinishReason) != "" ||
				ch.Delta.Content != "" ||
				len(ch.Delta.ToolCalls) > 0 {
				visibleChunkProgress = true
			}
		}
		if !chunkProgress {
			// Some OpenAI-compatible servers send valid-but-empty data
			// frames as keep-alives. They prove the TCP connection is
			// alive, but not that the model has begun answering. Do not
			// reset the watchdog on these frames, otherwise a provider
			// can keep the request open forever while producing no
			// usable assistant delta.
			continue
		}
		// Signal usable model progress to the watchdog. SSE comments,
		// blank separators, malformed chunks, and empty JSON keep-alives
		// are intentionally ignored so first-byte / stall timeouts
		// measure what the user can actually observe.
		if progress != nil {
			progress.Store(time.Now().UnixNano())
		}
		if !gotAnyChunk && firstByte != nil {
			firstByte.Store(true)
		}
		if visibleChunkProgress && visibleProgress != nil {
			visibleProgress.Store(time.Now().UnixNano())
		}
		gotAnyChunk = true

		if chunk.Usage != nil {
			usage = *chunk.Usage
		}

		for _, ch := range chunk.Choices {
			if ch.Delta.ReasoningContent != "" {
				reasoningBuf.WriteString(ch.Delta.ReasoningContent)
			}
			if ch.Delta.Content != "" {
				contentBuf.WriteString(ch.Delta.Content)
				if onDelta != nil {
					onDelta(ch.Delta.Content)
				}
			}
			for i, tc := range ch.Delta.ToolCalls {
				// Per-chunk index is authoritative. Providers that
				// ship a single tool_call put index 0 on every chunk
				// of that call; multi-tool responses interleave
				// indexes. A missing "index" field decodes as 0 which
				// is the correct default for a single-tool stream —
				// but for robustness use the position in the chunk
				// as a tie-breaker if tc.Index conflicts with state.
				idx := tc.Index
				if idx < 0 {
					idx = i
				}
				agg, seen := toolCalls[idx]
				if !seen {
					agg = &openaiToolCall{Type: "function"}
					toolCalls[idx] = agg
					toolCallOrder = append(toolCallOrder, idx)
				}
				if tc.ID != "" {
					agg.ID = tc.ID
				}
				if tc.Type != "" {
					agg.Type = tc.Type
				}
				if tc.Function.Name != "" {
					agg.Function.Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					agg.Function.Arguments += tc.Function.Arguments
					// Passive read-side tap: fire the optional
					// onToolCallDelta callback with the NEW chunk only
					// (not the cumulative buffer). The accumulator
					// state above is unaffected — adding this hook
					// does not alter what the caller eventually parses
					// from agg.Function.Arguments. Used by the
					// finalizer-preview path to live-stream the
					// `summary` field decoded from partial JSON;
					// callbacks that panic are isolated by the caller
					// (BaseAgent wraps them) so a buggy consumer
					// cannot kill the stream consumer goroutine.
					if onToolCallDelta != nil {
						onToolCallDelta(idx, agg.Function.Name, tc.Function.Arguments)
					}
				}
			}
			if ch.FinishReason != "" {
				finishReason = ch.FinishReason
			}
		}
	}
	if err := br.Err(); err != nil {
		return Response{}, fmt.Errorf("read stream: %w", err)
	}
	if !gotAnyChunk {
		return Response{}, fmt.Errorf("empty stream — provider closed the connection before any chunk")
	}

	resp := Response{
		Content:          contentBuf.String(),
		ReasoningContent: reasoningBuf.String(),
		StopReason:       mapFinishReason(finishReason),
		Usage: TokenUsage{
			InputTokens:  usage.PromptTokens,
			OutputTokens: usage.CompletionTokens,
		},
	}
	for _, idx := range toolCallOrder {
		tc := toolCalls[idx]
		resp.ToolCalls = append(resp.ToolCalls, ToolCall{
			ID:     tc.ID,
			Name:   tc.Function.Name,
			Params: json.RawMessage(tc.Function.Arguments),
		})
	}
	return resp, nil
}

// --- request types ---

type openaiRequest struct {
	Model      string          `json:"model"`
	Thinking   *openaiThinking `json:"thinking,omitempty"`
	Messages   []openaiMessage `json:"messages"`
	Tools      []openaiTool    `json:"tools,omitempty"`
	ToolChoice json.RawMessage `json:"tool_choice,omitempty"`
	MaxTokens  int             `json:"max_tokens,omitempty"`
	Stream     bool            `json:"stream,omitempty"`
}

type openaiThinking struct {
	Type string `json:"type"`
}

type openaiMessage struct {
	Role             string           `json:"role"`
	Content          any              `json:"content"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
}

type openaiContentPart struct {
	Type     string                 `json:"type"`
	Text     string                 `json:"text,omitempty"`
	ImageURL *openaiImageURLContent `json:"image_url,omitempty"`
}

type openaiImageURLContent struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type openaiTool struct {
	Type     string         `json:"type"`
	Function openaiFunction `json:"function"`
}

type openaiFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openaiToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openaiToolCallFunc `json:"function"`
}

// openaiToolCallDelta is the streaming-only variant. The server emits
// a per-chunk `index` that identifies which tool call the delta
// belongs to when multiple tools arrive interleaved; zero is a valid
// index (single-tool streams always use 0) so the decoder must not
// use `omitempty`. Request-side code never emits this type.
type openaiToolCallDelta struct {
	Index    int                `json:"index"`
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openaiToolCallFunc `json:"function"`
}

type openaiToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// --- response types ---

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
	Usage   openaiUsage    `json:"usage"`
}

type openaiChoice struct {
	Message      openaiMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// --- conversion ---

func (o *OpenAIAdapter) buildRequest(messages []Message, tools []ToolSchema, opts ChatOptions) openaiRequest {
	req := openaiRequest{
		Model: o.model,
		// MaxTokens has `omitempty`, so the zero default (= "no
		// client-side cap, server picks model ceiling") simply omits
		// the field on the wire. Operators set MaxOutputTokens > 0
		// only when they want to bound generation explicitly.
		MaxTokens: o.maxOutputTokens,
	}
	if thinkingType, ok := o.resolveProviderThinkingType(); ok {
		req.Thinking = &openaiThinking{Type: thinkingType}
	}

	// Convert messages
	for _, m := range messages {
		om := openaiMessage{
			Role:             m.Role,
			Content:          openAIRequestContent(m),
			ReasoningContent: m.ReasoningContent,
			ToolCallID:       m.ToolCallID,
		}
		// Convert tool calls on assistant messages
		for _, tc := range m.ToolCalls {
			om.ToolCalls = append(om.ToolCalls, openaiToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: openaiToolCallFunc{
					Name:      tc.Name,
					Arguments: string(tc.Params),
				},
			})
		}
		req.Messages = append(req.Messages, om)
	}

	// Convert tools
	for _, t := range tools {
		req.Tools = append(req.Tools, openaiTool{
			Type: "function",
			Function: openaiFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}

	// tool_choice is only meaningful when at least one tool is declared.
	// "auto" matches the provider default and is omitted (keeps the
	// wire payload identical for callers that pass ChatOptions{}). A
	// caller that wants the provider's default explicitly can still
	// set "auto" — nothing breaks, we just don't send the field.
	//
	// Two wire shapes per the OpenAI tool_choice spec:
	//   - bare string (auto / required / none): caller passes the
	//     keyword and we JSON-quote it.
	//   - named-function object: caller passes the JSON object literal
	//     `{"type":"function","function":{"name":"<tool>"}}` (a leading
	//     `{` is the discriminator). This is the most reliable form
	//     for "you MUST call THIS specific tool" — some providers
	//     (GLM / MiniMax / DeepSeek variants) honor named-function
	//     more reliably than bare "required".
	if len(req.Tools) > 0 && opts.ToolChoice != "" && opts.ToolChoice != "auto" && !o.suppressToolChoiceForNativeThinking() {
		trimmed := strings.TrimSpace(opts.ToolChoice)
		if strings.HasPrefix(trimmed, "{") {
			req.ToolChoice = json.RawMessage(trimmed)
		} else {
			req.ToolChoice = json.RawMessage(fmt.Sprintf("%q", opts.ToolChoice))
		}
	}

	if o.stream {
		req.Stream = true
	}

	return req
}

func openAIRequestContent(m Message) any {
	if len(m.ContentParts) == 0 {
		return m.Content
	}
	parts := make([]openaiContentPart, 0, len(m.ContentParts)+1)
	if strings.TrimSpace(m.Content) != "" {
		parts = append(parts, openaiContentPart{Type: "text", Text: m.Content})
	}
	for _, part := range m.ContentParts {
		switch strings.ToLower(strings.TrimSpace(part.Type)) {
		case "image_url", "image":
			url := strings.TrimSpace(part.ImageURL)
			if url == "" {
				continue
			}
			parts = append(parts, openaiContentPart{
				Type: "image_url",
				ImageURL: &openaiImageURLContent{
					URL:    url,
					Detail: strings.TrimSpace(part.Detail),
				},
			})
		case "text", "":
			text := strings.TrimSpace(part.Text)
			if text == "" {
				continue
			}
			parts = append(parts, openaiContentPart{Type: "text", Text: text})
		}
	}
	if len(parts) == 0 {
		return m.Content
	}
	return parts
}

func openAIResponseContentString(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(raw)
	}
}

func (o *OpenAIAdapter) marshalRequest(req openaiRequest) ([]byte, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if len(o.requestExtra) == 0 {
		return body, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	for k, v := range o.requestExtra {
		key := strings.TrimSpace(k)
		if key == "" || requestExtraReservedField(key) {
			if key != "" {
				logging.Warning("[llm] ignoring request_extra.%s because it is a reserved chat-completions field", key)
			}
			continue
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshal request_extra.%s: %w", key, err)
		}
		fields[key] = raw
	}
	return json.Marshal(fields)
}

func requestExtraReservedField(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "model", "messages", "tools", "tool_choice", "stream", "max_tokens", "thinking":
		return true
	default:
		return false
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneRawMessageMapLLM(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (o *OpenAIAdapter) suppressToolChoiceForNativeThinking() bool {
	thinkingType, ok := o.resolveProviderThinkingType()
	return ok && thinkingType == providerThinkingEnabled && isDeepSeekAPIEndpoint(o.baseURL)
}

const (
	providerThinkingAuto     = "auto"
	providerThinkingDisabled = "disabled"
	providerThinkingEnabled  = "enabled"
	providerThinkingDefault  = "provider_default"
)

func normalizeProviderThinkingMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", providerThinkingAuto:
		return providerThinkingAuto
	case providerThinkingDisabled, "disable", "off", "false":
		return providerThinkingDisabled
	case providerThinkingEnabled, "enable", "on", "true":
		return providerThinkingEnabled
	case providerThinkingDefault, "default", "omit", "none":
		return providerThinkingDefault
	default:
		logging.Warning("[llm] unknown provider thinking_mode=%q; omitting provider-native thinking fields", mode)
		return providerThinkingDefault
	}
}

func (o *OpenAIAdapter) resolveProviderThinkingType() (string, bool) {
	switch o.providerThinkingMode {
	case providerThinkingDisabled:
		return providerThinkingDisabled, true
	case providerThinkingEnabled:
		return providerThinkingEnabled, true
	case providerThinkingDefault:
		return "", false
	case providerThinkingAuto, "":
		if isDeepSeekAPIEndpoint(o.baseURL) {
			return providerThinkingDisabled, true
		}
	}
	return "", false
}

func isDeepSeekAPIEndpoint(baseURL string) bool {
	lower := strings.ToLower(strings.TrimSpace(baseURL))
	if lower == "" {
		return false
	}
	if u, err := url.Parse(lower); err == nil && u.Hostname() != "" {
		return isDeepSeekHost(u.Hostname())
	}
	return isDeepSeekHost(lower)
}

func isDeepSeekHost(host string) bool {
	host = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(host)), "www.")
	return host == "api.deepseek.com" || strings.HasSuffix(host, ".deepseek.com")
}

func (o *OpenAIAdapter) parseResponse(body []byte) (Response, error) {
	// Graceful fallback: some providers return SSE even on a non-
	// streaming request. Common cases: corporate gateways that always
	// stream, fine-tuned chat servers with no "stream: false" branch,
	// ollama deployments behind certain proxies. When we detect the
	// `data: ` prefix we hand the body to the same SSE parser the
	// streaming path uses so the caller still gets a normal Response.
	// Non-stream callback remains nil — nothing to surface mid-stream
	// because the body is already fully buffered by doRequest.
	if looksLikeSSEResponse(body) {
		logging.Debug("[llm] non-streaming request returned SSE; parsing as stream")
		return parseSSEStream(bytes.NewReader(body), nil, nil, nil, nil)
	}

	var oResp openaiResponse
	if err := json.Unmarshal(body, &oResp); err != nil {
		return Response{}, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(oResp.Choices) == 0 {
		return Response{}, fmt.Errorf("empty choices in response")
	}

	choice := oResp.Choices[0]
	resp := Response{
		Content:          openAIResponseContentString(choice.Message.Content),
		ReasoningContent: choice.Message.ReasoningContent,
		StopReason:       mapFinishReason(choice.FinishReason),
		Usage: TokenUsage{
			InputTokens:  oResp.Usage.PromptTokens,
			OutputTokens: oResp.Usage.CompletionTokens,
		},
	}

	// Convert tool calls
	for _, tc := range choice.Message.ToolCalls {
		resp.ToolCalls = append(resp.ToolCalls, ToolCall{
			ID:     tc.ID,
			Name:   tc.Function.Name,
			Params: json.RawMessage(tc.Function.Arguments),
		})
	}

	return resp, nil
}

func mapFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return reason
	}
}

// --- errors ---

// apiError captures the HTTP status, response body, and selected
// headers from a non-2xx LLM response. RetryAfter is parsed from the
// `Retry-After` header at construction time; QuotaError flips on when
// the body looks like a true rate-limit / quota refusal (vs Azure-style
// deployment rotation), which lets the retry loop back off longer
// before exhausting the budget.
type apiError struct {
	StatusCode int
	Body       string
	// RetryAfter is the parsed Retry-After header (RFC 7231): integer
	// seconds OR HTTP-date. Zero when absent or unparseable. The retry
	// loop uses this as the next sleep when present, falling back to
	// exponential when zero.
	RetryAfter time.Duration
	// QuotaError flips on when the response body matches a true
	// rate-limit / quota error pattern (token quota exhausted, plan
	// limit reached, etc.) — distinguishing it from transient
	// deployment-rotation 429s that clear in <60 s. The retry loop
	// uses a longer cap for QuotaError so the budget is not burned in
	// 62 s on a quota error that needs minutes to clear.
	QuotaError bool
}

func (e *apiError) Error() string {
	return fmt.Sprintf("LLM API error (status %d): %s", e.StatusCode, e.Body)
}

// newAPIError builds an apiError from an http.Response and the already
// drained body bytes. Parses Retry-After from the response headers and
// classifies the body for the QuotaError heuristic. Caller is
// responsible for the `httpResp.StatusCode != 200` gate; this just
// packages the error so the retry loop has structured signal.
func newAPIError(httpResp *http.Response, body []byte) *apiError {
	ae := &apiError{
		StatusCode: httpResp.StatusCode,
		Body:       string(body),
	}
	if httpResp.Header != nil {
		if v := httpResp.Header.Get("Retry-After"); v != "" {
			ae.RetryAfter = parseRetryAfter(v, time.Now())
		}
	}
	if httpResp.StatusCode == 429 {
		ae.QuotaError = looksLikeQuotaError(body)
	}
	return ae
}

// parseRetryAfter parses the RFC 7231 Retry-After header. Returns the
// duration to wait, or zero if the header is empty / unparseable. Two
// supported forms: an integer number of seconds, or an HTTP-date. The
// `now` parameter is injected so tests can pin time without mocking
// time.Now globally.
func parseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	// Form 1: integer seconds (most common).
	var secs int
	if _, err := fmt.Sscanf(v, "%d", &secs); err == nil && secs >= 0 {
		// Sanity cap: 1 hour. A larger Retry-After almost always means
		// the upstream is genuinely down and we should fall through to
		// the standard error path rather than blocking the agent for
		// an entire training session.
		if secs > 3600 {
			return time.Hour
		}
		return time.Duration(secs) * time.Second
	}
	// Form 2: HTTP-date. http.ParseTime handles RFC 1123 / RFC 850 /
	// ANSI C variants in a single call.
	if t, err := http.ParseTime(v); err == nil {
		d := t.Sub(now)
		if d <= 0 {
			return 0
		}
		if d > time.Hour {
			return time.Hour
		}
		return d
	}
	return 0
}

// looksLikeQuotaError scans the response body for tokens that signal a
// true quota / rate-limit refusal as opposed to a transient deployment
// rotation. Conservative: only flips on when one of a small set of
// recognized markers is present, so a deployment-rotation 429 (which
// uses different language) keeps the existing 62 s exponential schedule.
//
// Markers cover the OpenAI / Anthropic / Bytedance / DeepSeek /
// generic OpenAI-compatible vendor variants we have observed. Adding
// new vendor markers here is the right surgical fix for "vendor X's
// 429 message is misclassified" — single-line append, no behavior
// regression for existing markers.
func looksLikeQuotaError(body []byte) bool {
	lower := strings.ToLower(string(body))
	markers := []string{
		`"type":"rate_limit_error"`,
		`"code":"rate_limit_exceeded"`,
		`"code":"insufficient_quota"`,
		`token plan`,
		`quota`,
		`rate limit`,
		`套餐`,
		`配额`,
		`请求量较高`,
	}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

func isRetryable(err error) bool {
	if ae, ok := err.(*apiError); ok {
		return ae.StatusCode == 429 || ae.StatusCode >= 500
	}
	return false
}

// nextRetryDelay computes the wait before the next retry attempt.
// Precedence: server-supplied Retry-After (when present and positive)
// > quota-error long backoff > standard exponential. The standard
// schedule (2s/4s/8s/16s/32s) is unchanged for non-429 retries and
// for 429s that don't look quota-shaped, preserving the original
// deployment-rotation coverage. Quota errors get a 4× longer schedule
// (8s/16s/32s/64s/128s ≈ 4 minutes) so the retry budget actually has
// a chance to outlast a true quota stall before failing loud.
func nextRetryDelay(err error, attempt int) time.Duration {
	return NextRetryDelay(err, attempt)
}

// NextRetryDelay is the public accessor that callers outside the
// llm package use to share the same backoff schedule as the
// in-adapter retry loop. attempt is 0-indexed (0 = first retry).
//
// Schedule (priority order):
//   - HTTP 429 with Retry-After header → respect the server's value
//   - quota-shaped 429 (no header) → 8s/16s/32s/64s/128s long ramp
//   - everything else → 2s/4s/8s/16s/32s standard exponential
//
// Used by the orchestrator's force-finalize escape path so a
// retry-after-bearing 429 on the last-resort dispatch waits exactly
// the indicated interval; a transient EOF / stream stall falls
// through to the standard exponential schedule.
func NextRetryDelay(err error, attempt int) time.Duration {
	if ae, ok := err.(*apiError); ok {
		if ae.RetryAfter > 0 {
			return ae.RetryAfter
		}
		if ae.QuotaError {
			return time.Duration(8<<uint(attempt)) * time.Second
		}
	}
	return time.Duration(2<<uint(attempt)) * time.Second
}

// looksLikeSSEResponse reports whether body is formatted as a
// Server-Sent Events stream. Used by parseResponse to auto-redirect
// when a provider returns SSE on a non-streaming request. Detection
// is deliberately conservative: trim leading whitespace and check
// for the canonical `data:` line prefix. A JSON response starts with
// `{` or `[`; a malformed response that happens to begin with `d`
// (e.g. `deadbeef`) will still fail here because the SSE parser
// itself requires at least one `data: ` line and errors loudly.
func looksLikeSSEResponse(body []byte) bool {
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	return bytes.HasPrefix(trimmed, []byte("data:"))
}
