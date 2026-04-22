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
type OpenAIAdapter struct {
	apiKey     string
	model      string
	baseURL    string
	maxTokens  int
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

// NewOpenAIAdapter creates a new OpenAI adapter. apiKey, model, and
// baseURL are all required — the factory layer is responsible for
// rejecting configs that omit any of them. This also works with
// Azure OpenAI, DeepSeek, Ollama, and other OpenAI-compatible APIs.
// stream toggles SSE streaming on the chat/completions endpoint.
func NewOpenAIAdapter(apiKey, model, baseURL string, stream bool, tlsOpts TLSOptions) *OpenAIAdapter {
	return &OpenAIAdapter{
		apiKey:     apiKey,
		model:      model,
		baseURL:    baseURL,
		maxTokens:  4096,
		stream:     stream,
		httpClient: buildHTTPClient(tlsOpts, baseURL),
	}
}

// buildHTTPClient assembles the per-provider http.Client. When the
// caller supplies TLS overrides, a fresh Transport is attached with
// the requested TLS config; otherwise the default Transport is used
// (http.DefaultTransport won't be modified, so concurrent providers
// don't stomp each other). Errors loading the CA file surface as
// startup warnings and fall back to the system trust pool, matching
// the precedent of "never block on a misconfigured optional field."
func buildHTTPClient(tlsOpts TLSOptions, baseURL string) *http.Client {
	if tlsOpts.CAFile == "" && !tlsOpts.InsecureSkipVerify {
		return &http.Client{Timeout: 120 * time.Second}
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
		Timeout: 120 * time.Second,
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

func (o *OpenAIAdapter) ModelID() string       { return o.model }
func (o *OpenAIAdapter) MaxContextTokens() int { return 128000 }

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
	// routinely burned through. Extending to 6 attempts with 2/4/8/16/32s
	// backoff buys ~62s of coverage — enough for typical deployment
	// rotations without tripping into a total stall when something is
	// genuinely wrong upstream. 5xx errors reuse the same schedule
	// because a brief server hiccup happily tolerates the longer wait.
	const maxAttempts = 6
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if o.stream {
			resp, err = o.doStreamRequest(bodyBytes, opts.OnContentDelta)
		} else {
			resp, err = o.doRequest(bodyBytes)
		}
		if err == nil {
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
		Model:     o.model,
		MaxTokens: o.maxTokens,
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
