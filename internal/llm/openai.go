package llm

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// OpenAIAdapter implements the Adapter interface for OpenAI-compatible APIs.
type OpenAIAdapter struct {
	apiKey     string
	model      string
	baseURL    string
	maxTokens  int
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

// NewOpenAIAdapter creates a new OpenAI adapter.
// baseURL defaults to "https://api.openai.com/v1" if empty.
// This also works with Azure OpenAI, DeepSeek, Ollama, and other compatible APIs.
func NewOpenAIAdapter(apiKey, model, baseURL string, tlsOpts TLSOptions) *OpenAIAdapter {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAIAdapter{
		apiKey:     apiKey,
		model:      model,
		baseURL:    baseURL,
		maxTokens:  4096,
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
		resp, err = o.doRequest(bodyBytes)
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

// --- request types ---

type openaiRequest struct {
	Model      string          `json:"model"`
	Messages   []openaiMessage `json:"messages"`
	Tools      []openaiTool    `json:"tools,omitempty"`
	ToolChoice json.RawMessage `json:"tool_choice,omitempty"`
	MaxTokens  int             `json:"max_tokens,omitempty"`
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

	return req
}

func (o *OpenAIAdapter) parseResponse(body []byte) (Response, error) {
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
	return fmt.Sprintf("OpenAI API error (status %d): %s", e.StatusCode, e.Body)
}

func isRetryable(err error) bool {
	if ae, ok := err.(*apiError); ok {
		return ae.StatusCode == 429 || ae.StatusCode >= 500
	}
	return false
}
