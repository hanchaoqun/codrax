package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBaseAgentMixedFallbackDoesNotCancelActiveSSEAtEvaluatorBudget(t *testing.T) {
	for _, nested := range []bool{false, true} {
		name := "direct_fallback"
		if nested {
			name = "nested_telemetry_and_fallback"
		}
		t.Run(name, func(t *testing.T) {
			var writes atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				flusher := w.(http.Flusher)
				for i := 0; i < 25; i++ {
					_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking\"}}]}\n\n"))
					flusher.Flush()
					writes.Add(1)
					select {
					case <-r.Context().Done():
						return
					case <-time.After(2 * time.Millisecond):
					}
				}
				_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"active stream completed\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
				flusher.Flush()
			}))
			defer server.Close()
			primary := llm.NewOpenAIAdapter("test-key", "stream", server.URL, llm.AdapterOptions{
				Stream: true, RequestTimeout: 4 * time.Millisecond, RetryMaxAttempts: 1,
				StreamFirstByteTimeout: time.Second, StreamStallTimeout: time.Second,
			})
			fallback := &unexpectedFallbackBudgetLLM{}
			var adapter llm.Adapter = llm.NewFallbackAdapter(primary, fallback)
			if nested {
				adapter = llm.NewTelemetryAdapter(llm.NewFallbackAdapter(
					llm.NewTelemetryAdapter(adapter, func(llm.RequestTelemetry) {}), fallback,
				), func(llm.RequestTelemetry) {})
			}
			b := NewBaseAgent(types.AgentAnalyzer, &Dependencies{
				LLM: adapter, Tools: tool.NewRegistry(), MaxIterations: 1, Emit: func(render.Event) {},
			}, &requestBudgetEvaluator{timeout: 4 * time.Millisecond, reason: "analyzer_terminal_emit_only"})
			start := time.Now()
			_, err := b.Execute(&types.AgentContext{
				AgentName: types.AgentAnalyzer, Stage: types.StageAnalyze,
				Mutable: types.NewMutableState("runtime remains active before visible answer"),
			}, &skill.Config{})
			if err != nil {
				t.Fatalf("active streaming primary must not inherit a budget for its unused non-streaming fallback (SSE writes=%d): %v", writes.Load(), err)
			}
			if fallback.calls != 0 || writes.Load() != 25 || time.Since(start) < 40*time.Millisecond {
				t.Fatalf("must await the original stream: fallback calls=%d writes=%d elapsed=%s", fallback.calls, writes.Load(), time.Since(start))
			}
		})
	}
}

type unexpectedFallbackBudgetLLM struct{ calls int }

func (l *unexpectedFallbackBudgetLLM) Chat(ctx context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	return llm.Response{Content: "unexpected fallback"}, nil
}

func (*unexpectedFallbackBudgetLLM) ModelID() string               { return "non-streaming-fallback" }
func (*unexpectedFallbackBudgetLLM) MaxContextTokens() int         { return 128000 }
func (*unexpectedFallbackBudgetLLM) MaxOutputTokens() int          { return 4096 }
func (*unexpectedFallbackBudgetLLM) RequestTimeout() time.Duration { return time.Second }
func (*unexpectedFallbackBudgetLLM) RetryMaxAttempts() int         { return 1 }
