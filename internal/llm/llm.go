package llm

import (
	"context"
	"encoding/json"
	"time"
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

	// OnContentDelta is an optional callback fired by a streaming
	// adapter every time a chunk of assistant content arrives. Delta
	// is the NEW text only (not the accumulated buffer). The final
	// accumulated content is still returned on the Response. Non-
	// streaming adapters never invoke this callback, even when
	// non-nil.
	OnContentDelta func(delta string)

	// OnToolCallDelta is an optional PASSIVE-READ callback fired by
	// a streaming adapter every time a chunk of a tool-call's
	// `arguments` field arrives over the wire. argsChunk is the NEW
	// raw JSON fragment (not the accumulated buffer); index is the
	// position of the tool call within the response (0 for the first
	// tool call, etc.); name is the tool name once it has been
	// observed (may be empty for the first few chunks if the name
	// has not yet streamed in).
	//
	// The callback is PASSIVE: it observes deltas without disturbing
	// the adapter's internal accumulation buffer. The complete tool
	// call (with full Arguments) is still returned on the Response
	// after the stream closes — byte-identical with vs without this
	// callback installed. This is the load-bearing invariant that
	// lets the finalizer-preview path (internal/render preview area
	// + summary field extractor) tap the stream for live UI without
	// affecting the parsed AnswerDocument the orchestrator builds
	// from the same stream.
	//
	// Tool-call arguments are PARTIAL JSON mid-stream — callers that
	// need a parseable value must wait for stream end. Live consumers
	// (like the summary preview extractor) are responsible for any
	// best-effort partial-parse logic on their side.
	OnToolCallDelta func(index int, name string, argsChunk string)

	// OnRetry fires from the in-adapter retry loop just before the
	// adapter sleeps on a transient error (HTTP 429 / 5xx / first-byte
	// timeout). attempt is the 1-based index of the attempt that just
	// failed (so attempt=1 means "first try failed, about to retry"),
	// delay is the next sleep duration, and reason is a short human
	// phrase describing why we're retrying (e.g. "rate limit", "stream
	// stalled"). The callback exists so the renderer's dock can flip
	// to the "重试中" status word — without it the dock keeps showing
	// "请求模型中" for the entire retry window, lying to the user.
	// Non-fatal: a panic inside the callback is recovered so the
	// retry loop keeps running.
	OnRetry func(attempt int, delay time.Duration, reason string)

	// OnFallback fires from the FallbackAdapter just before swapping
	// to the next adapter in the chain. from / to are the model IDs
	// of the outgoing / incoming adapters; reason is a short phrase.
	// Renderer dock uses this to flip the row-1 status word to
	// "切换 provider 中". Same panic-recovery contract as OnRetry.
	OnFallback func(from, to, reason string)
}

// Adapter defines the interface for LLM backends.
//
// Sizing accessors (MaxContextTokens / MaxOutputTokens /
// RequestTimeout / RetryMaxAttempts) are exposed so the orchestrator
// can log the effective per-adapter knobs at startup. This makes the
// "what does codrax actually think the cap is" question answerable
// from the log alone, without grepping the yaml + adapter source.
// MaxOutputTokens returns 0 when the operator has chosen "no client-
// side cap" (the default) — callers must treat 0 as "server uses
// model ceiling" rather than "infinite local budget."
type Adapter interface {
	// Chat dispatches one chat-completion round. ctx is the
	// cancellation-aware context the orchestrator hands down via the
	// CancelToken; HTTP-level cancellation (Ctrl+C unwind, /cancel,
	// stage timeout) propagates through ctx.Done() so the in-flight
	// request socket closes immediately rather than waiting for the
	// next cooperative agent-loop checkpoint. Callers that genuinely
	// have no cancel surface (single-shot CLI, unit tests) pass
	// context.Background() to declare "no cancel" explicitly — nil
	// is rejected by the canonical adapter implementation.
	Chat(ctx context.Context, messages []Message, tools []ToolSchema, opts ChatOptions) (Response, error)
	ModelID() string
	MaxContextTokens() int
	MaxOutputTokens() int
	RequestTimeout() time.Duration
	RetryMaxAttempts() int
}
