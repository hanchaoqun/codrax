package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAIAdapter implements the Adapter interface for OpenAI-compatible APIs.
type OpenAIAdapter struct {
	apiKey     string
	model      string
	baseURL    string
	maxTokens  int
	httpClient *http.Client
}

// NewOpenAIAdapter creates a new OpenAI adapter.
// baseURL defaults to "https://api.openai.com/v1" if empty.
// This also works with Azure OpenAI, DeepSeek, Ollama, and other compatible APIs.
func NewOpenAIAdapter(apiKey, model, baseURL string) *OpenAIAdapter {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAIAdapter{
		apiKey:    apiKey,
		model:     model,
		baseURL:   baseURL,
		maxTokens: 4096,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (o *OpenAIAdapter) ModelID() string       { return o.model }
func (o *OpenAIAdapter) MaxContextTokens() int { return 128000 }

func (o *OpenAIAdapter) Chat(messages []Message, tools []ToolSchema) (Response, error) {
	reqBody := o.buildRequest(messages, tools)

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
	Model     string          `json:"model"`
	Messages  []openaiMessage `json:"messages"`
	Tools     []openaiTool    `json:"tools,omitempty"`
	MaxTokens int             `json:"max_tokens,omitempty"`
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

func (o *OpenAIAdapter) buildRequest(messages []Message, tools []ToolSchema) openaiRequest {
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
