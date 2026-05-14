package llm

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEstimateRequestTokensIncludesToolSchemas(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "abcd"},
		{Role: "user", Content: "efghijk"},
	}
	tools := []ToolSchema{
		{
			Name:        "read_file",
			Description: "read bytes",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		},
	}
	wantBytes := EstimateMessagesBytes(messages) + len("read_file") + len("read bytes") + len(`{"type":"object","properties":{"path":{"type":"string"}}}`)
	wantTokens := (wantBytes + types.BytesPerToken - 1) / types.BytesPerToken
	if got := EstimateRequestTokens(messages, tools); got != wantTokens {
		t.Errorf("EstimateRequestTokens = %d, want %d", got, wantTokens)
	}
}

func TestTelemetryAdapterEmitsBeforeDelegating(t *testing.T) {
	inner := &telemetryStubAdapter{modelID: "local-model", maxContext: 260000}
	var seen RequestTelemetry
	wrapped := NewTelemetryAdapter(inner, func(t RequestTelemetry) {
		seen = t
	})
	_, err := wrapped.Chat(context.Background(),
		[]Message{{Role: "user", Content: "hello"}},
		[]ToolSchema{{Name: "emit", Description: "done", Parameters: json.RawMessage(`{"type":"object"}`)}},
		ChatOptions{},
	)
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("inner calls = %d, want 1", inner.calls)
	}
	if seen.ModelID != "local-model" || seen.ContextWindowTokens != 260000 {
		t.Fatalf("telemetry model/window = %q/%d", seen.ModelID, seen.ContextWindowTokens)
	}
	if seen.ContextTokensEstimate <= 0 || seen.MessageCount != 1 || seen.ToolCount != 1 {
		t.Fatalf("telemetry incomplete: %+v", seen)
	}
}

func TestTelemetryAdapterObserverPanicDoesNotBreakChat(t *testing.T) {
	inner := &telemetryStubAdapter{}
	wrapped := NewTelemetryAdapter(inner, func(RequestTelemetry) {
		panic("observer bug")
	})
	if _, err := wrapped.Chat(context.Background(), nil, nil, ChatOptions{}); err != nil {
		t.Fatalf("observer panic must not break Chat: %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("inner calls = %d, want 1", inner.calls)
	}
}

type telemetryStubAdapter struct {
	modelID    string
	maxContext int
	calls      int
}

func (t *telemetryStubAdapter) Chat(context.Context, []Message, []ToolSchema, ChatOptions) (Response, error) {
	t.calls++
	return Response{Content: "ok"}, nil
}

func (t *telemetryStubAdapter) ModelID() string {
	if t.modelID == "" {
		return "stub"
	}
	return t.modelID
}
func (t *telemetryStubAdapter) MaxContextTokens() int         { return t.maxContext }
func (t *telemetryStubAdapter) MaxOutputTokens() int          { return 0 }
func (t *telemetryStubAdapter) RequestTimeout() time.Duration { return time.Second }
func (t *telemetryStubAdapter) RetryMaxAttempts() int         { return 1 }
