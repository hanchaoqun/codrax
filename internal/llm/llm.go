package llm

import (
	"encoding/json"
)

// ToolSchema describes a tool for function calling.
type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"input_schema"`
}

// ToolCall represents an LLM's request to invoke a tool.
type ToolCall struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Params json.RawMessage `json:"input"`
}

// TokenUsage tracks token consumption.
type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Message represents a conversation message.
type Message struct {
	Role       string     `json:"role"`                   // system, user, assistant, tool
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // assistant messages carry tool calls
	ToolCallID string     `json:"tool_call_id,omitempty"` // tool messages reference the call
}

// Response is what the LLM returns.
type Response struct {
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	StopReason string     `json:"stop_reason"`
	Usage      TokenUsage `json:"usage"`
}

// ChatOptions carries per-call controls that are orthogonal to the
// message list and tool catalog. Empty value means "defaults" — a
// caller that does not care about any of these fields may pass
// ChatOptions{} and get the pre-options behavior.
type ChatOptions struct {
	// ToolChoice maps to the OpenAI-style `tool_choice` request field.
	// Recognised values:
	//   ""          → don't emit the field (provider default, normally "auto")
	//   "auto"      → model decides whether to call a tool
	//   "required"  → model MUST emit at least one tool_call this turn
	//   "none"      → model must NOT emit any tool_calls
	// Set by the agent layer on stages whose evaluator treats a
	// no-tool-call response as a retry trigger (analyzer / extractor /
	// finalizer / log_triager) so the protocol itself rejects the
	// failure mode, instead of burning the continuation retry budget.
	ToolChoice string
}

// Adapter defines the interface for LLM backends.
type Adapter interface {
	Chat(messages []Message, tools []ToolSchema, opts ChatOptions) (Response, error)
	ModelID() string
	MaxContextTokens() int
}
