package llm

import (
	"context"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RequestTelemetry is best-effort pre-flight metadata for one Chat
// request. It is intentionally tokenizer-approximate: the UI needs a
// stable, cheap signal that operators can compare across requests,
// not provider-specific exact accounting.
type RequestTelemetry struct {
	ModelID               string
	ContextTokensEstimate int
	ContextWindowTokens   int
	MessageCount          int
	ToolCount             int
}

// EstimateMessagesBytes sums the serialized size of every message in
// the slice. Includes content, role surface, tool-call ids/names, and
// raw tool-call params because adapters turn those fields into wire
// JSON and dense tool calls can dominate the request.
func EstimateMessagesBytes(messages []Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Role) + len(m.Content) + len(m.ToolCallID)
		for _, tc := range m.ToolCalls {
			total += len(tc.ID) + len(tc.Name) + len(tc.Params)
		}
	}
	return total
}

// EstimateToolSchemasBytes estimates the request-side footprint of
// the tool catalog sent with a chat completion.
func EstimateToolSchemasBytes(tools []ToolSchema) int {
	total := 0
	for _, t := range tools {
		total += len(t.Name) + len(t.Description) + len(t.Parameters)
	}
	return total
}

func EstimateRequestBytes(messages []Message, tools []ToolSchema) int {
	return EstimateMessagesBytes(messages) + EstimateToolSchemasBytes(tools)
}

func EstimateRequestTokens(messages []Message, tools []ToolSchema) int {
	bytes := EstimateRequestBytes(messages, tools)
	if bytes <= 0 {
		return 0
	}
	return (bytes + types.BytesPerToken - 1) / types.BytesPerToken
}

func BuildRequestTelemetry(adapter Adapter, messages []Message, tools []ToolSchema) RequestTelemetry {
	t := RequestTelemetry{
		ContextTokensEstimate: EstimateRequestTokens(messages, tools),
		MessageCount:          len(messages),
		ToolCount:             len(tools),
	}
	if adapter != nil {
		t.ModelID = adapter.ModelID()
		t.ContextWindowTokens = adapter.MaxContextTokens()
	}
	return t
}

// RequestTelemetryObserver receives the pre-flight telemetry emitted
// by TelemetryAdapter immediately before delegating to the wrapped
// adapter. Observers must be best-effort side effects; a panic is
// recovered so UI telemetry cannot break an LLM request.
type RequestTelemetryObserver func(RequestTelemetry)

type TelemetryAdapter struct {
	inner   Adapter
	observe RequestTelemetryObserver
}

func NewTelemetryAdapter(inner Adapter, observe RequestTelemetryObserver) Adapter {
	if inner == nil || observe == nil {
		return inner
	}
	return &TelemetryAdapter{inner: inner, observe: observe}
}

func (t *TelemetryAdapter) Chat(ctx context.Context, messages []Message, tools []ToolSchema, opts ChatOptions) (Response, error) {
	if t != nil && t.observe != nil {
		telemetry := BuildRequestTelemetry(t.inner, messages, tools)
		func() {
			defer func() { _ = recover() }()
			t.observe(telemetry)
		}()
	}
	return t.inner.Chat(ctx, messages, tools, opts)
}

func (t *TelemetryAdapter) ModelID() string {
	if t == nil || t.inner == nil {
		return ""
	}
	return t.inner.ModelID()
}

func (t *TelemetryAdapter) MaxContextTokens() int {
	if t == nil || t.inner == nil {
		return 0
	}
	return t.inner.MaxContextTokens()
}

func (t *TelemetryAdapter) MaxOutputTokens() int {
	if t == nil || t.inner == nil {
		return 0
	}
	return t.inner.MaxOutputTokens()
}

func (t *TelemetryAdapter) RequestTimeout() time.Duration {
	if t == nil || t.inner == nil {
		return 0
	}
	return t.inner.RequestTimeout()
}

func (t *TelemetryAdapter) RetryMaxAttempts() int {
	if t == nil || t.inner == nil {
		return 0
	}
	return t.inner.RetryMaxAttempts()
}
