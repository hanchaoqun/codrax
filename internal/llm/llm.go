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
	Role       string `json:"role"`                      // system, developer, user, assistant, tool
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id,omitempty"`    // for tool role messages
}

// Response is what the LLM returns.
type Response struct {
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	StopReason string     `json:"stop_reason"`
	Usage      TokenUsage `json:"usage"`
}

// Adapter defines the interface for LLM backends.
type Adapter interface {
	Chat(messages []Message, tools []ToolSchema) (Response, error)
	ModelID() string
	MaxContextTokens() int
}
