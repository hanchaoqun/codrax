package llm

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
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

	stream     bool
	httpClient *http.Client
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
	Stream           bool
	ContextWindow    int
	MaxOutputTokens  int
	RequestTimeout   time.Duration
	RetryMaxAttempts int
	TLS              TLSOptions
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
	if opts.RequestTimeout <= 0 {
		panic("llm: AdapterOptions.RequestTimeout must be positive (factory must apply code default)")
	}
	if opts.RetryMaxAttempts <= 0 {
		panic("llm: AdapterOptions.RetryMaxAttempts must be positive (factory must apply code default)")
	}
	return &OpenAIAdapter{
		apiKey:           apiKey,
		model:            model,
		baseURL:          baseURL,
		maxOutputTokens:  opts.MaxOutputTokens,
		contextWindow:    opts.ContextWindow,
		requestTimeout:   opts.RequestTimeout,
		retryMaxAttempts: opts.RetryMaxAttempts,
		stream:           opts.Stream,
		httpClient:       buildHTTPClient(opts.TLS, baseURL, opts.RequestTimeout),
	}
}

// buildHTTPClient assembles the per-provider http.Client. When the
// caller supplies TLS overrides, a fresh Transport is attached with
// the requested TLS config; otherwise the default Transport is used
// (http.DefaultTransport won't be modified, so concurrent providers
// don't stomp each other). Errors loading the CA file surface as
// startup warnings and fall back to the system trust pool, matching
// the precedent of "never block on a misconfigured optional field."
func buildHTTPClient(tlsOpts TLSOptions, baseURL string, timeout time.Duration) *http.Client {
	if tlsOpts.CAFile == "" && !tlsOpts.InsecureSkipVerify {
		return &http.Client{Timeout: timeout}
	}

	tlsCfg := &tls.Config{}

	if tlsOpts.CAFile != "" {
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		pem, readErr := os.ReadFile(tlsOpts.CAFile)
		if readErr != nil {
			logging.Warning("[llm/tls] could not read tls_ca_file %q: %v — falling back to system trust pool", tlsOpts.CAFile, readErr)
		} else if !pool.AppendCertsFromPEM(pem) {
			logging.Warning("[llm/tls] tls_ca_file %q contained no valid PEM certificates — falling back to system trust pool", tlsOpts.CAFile)
		} else {
			logging.Info("[llm/tls] appended custom CA bundle %q to system trust pool for %s", tlsOpts.CAFile, baseURL)
			tlsCfg.RootCAs = pool
		}
	}

	if tlsOpts.InsecureSkipVerify {
		tlsCfg.InsecureSkipVerify = true
		logging.Warning("[llm/tls] ⚠ tls_insecure_skip_verify=true for %s — certificate validation DISABLED, API key is vulnerable to on-path interception", baseURL)
		fmt.Fprintf(os.Stderr, "  ⚠ TLS verification DISABLED for %s (tls_insecure_skip_verify=true)\n", baseURL)
	}

	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
			// Mirror the non-TLS knobs of http.DefaultTransport so a
			// TLS-overriding client doesn't silently regress on
			// connection pooling, proxy detection, etc.
			Proxy:                 http.ProxyFromEnvironment,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

func (o *OpenAIAdapter) ModelID() string { return o.model }

// MaxContextTokens returns the configured context_window from
// providers.yaml. When zero (not declared), returns the historical
// 128000 fallback so any consumer that treats a positive return
// value as ground truth (divide-by in fraction cap resolver, ratio
// check in pressure watchdog) keeps working. Callers that need to
// distinguish "unknown" from "128K" should consult the struct field
// directly via a capability query rather than parsing this return.
func (o *OpenAIAdapter) MaxContextTokens() int {
	if o.contextWindow > 0 {
		return o.contextWindow
	}
	return 128000
}

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

func (o *OpenAIAdapter) Chat(messages []Message, tools []ToolSchema, opts ChatOptions) (Response, error) {
	reqBody := o.buildRequest(messages, tools, opts)

	bodyBytes, err := json.Marshal(reqBody)
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
		if o.stream {
			resp, err = o.doStreamRequest(bodyBytes, opts.OnContentDelta)
		} else {
			resp, err = o.doRequest(bodyBytes)
		}
		if err == nil {
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

		// Only retry on rate limit (429) or server errors (5xx)
		if !isRetryable(err) {
			return Response{}, err
		}

		// Exponential backoff: 2s, 4s, 8s, 16s, 32s. The last attempt
		// fires without a trailing sleep so the total wait matches the
		// schedule above.
		if attempt < maxAttempts-1 {
			time.Sleep(time.Duration(2<<uint(attempt)) * time.Second)
		}
	}

	return Response{}, fmt.Errorf("all retries failed: %w", lastErr)
}

func (o *OpenAIAdapter) doRequest(bodyBytes []byte) (Response, error) {
	req, err := http.NewRequest("POST", o.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return Response{}, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	httpResp, err := o.httpClient.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("http request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return Response{}, &apiError{
			StatusCode: httpResp.StatusCode,
			Body:       string(respBody),
		}
	}

	return o.parseResponse(respBody)
}

// doStreamRequest POSTs a streaming chat completion and accumulates
// the incremental SSE chunks into a single Response. Content deltas
// fire the optional onDelta callback as they arrive so the agent
// layer can surface a live preview. Tool-call argument deltas are
// accumulated silently — they arrive as partial JSON that cannot be
// parsed until the stream closes, and mid-stream surfacing would
// leak broken JSON into logs / UI.
func (o *OpenAIAdapter) doStreamRequest(bodyBytes []byte, onDelta func(string)) (Response, error) {
	req, err := http.NewRequest("POST", o.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return Response{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	httpResp, err := o.httpClient.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("http request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		// Non-OK responses are returned as a single JSON body, not SSE.
		body, _ := io.ReadAll(httpResp.Body)
		return Response{}, &apiError{
			StatusCode: httpResp.StatusCode,
			Body:       string(body),
		}
	}

	return parseSSEStream(httpResp.Body, onDelta)
}

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
func parseSSEStream(r io.Reader, onDelta func(string)) (Response, error) {
	br := bufio.NewScanner(r)
	// Single SSE frames can exceed bufio's default 64 KB line cap when
	// a provider batches many deltas into one frame; raise to 1 MB to
	// match the upstream response size budget.
	br.Buffer(make([]byte, 64*1024), 1<<20)

	var contentBuf strings.Builder
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
		gotAnyChunk = true

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string                `json:"content"`
					ToolCalls []openaiToolCallDelta `json:"tool_calls"`
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

		if chunk.Usage != nil {
			usage = *chunk.Usage
		}

		for _, ch := range chunk.Choices {
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
		Content:    contentBuf.String(),
		StopReason: mapFinishReason(finishReason),
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
	Messages   []openaiMessage `json:"messages"`
	Tools      []openaiTool    `json:"tools,omitempty"`
	ToolChoice json.RawMessage `json:"tool_choice,omitempty"`
	MaxTokens  int             `json:"max_tokens,omitempty"`
	Stream     bool            `json:"stream,omitempty"`
}

type openaiMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
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

	// Convert messages
	for _, m := range messages {
		om := openaiMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
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
	if len(req.Tools) > 0 && opts.ToolChoice != "" && opts.ToolChoice != "auto" {
		req.ToolChoice = json.RawMessage(fmt.Sprintf("%q", opts.ToolChoice))
	}

	if o.stream {
		req.Stream = true
	}

	return req
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
		return parseSSEStream(bytes.NewReader(body), nil)
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
		Content:    choice.Message.Content,
		StopReason: mapFinishReason(choice.FinishReason),
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

type apiError struct {
	StatusCode int
	Body       string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("LLM API error (status %d): %s", e.StatusCode, e.Body)
}

func isRetryable(err error) bool {
	if ae, ok := err.(*apiError); ok {
		return ae.StatusCode == 429 || ae.StatusCode >= 500
	}
	return false
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
