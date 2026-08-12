package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// retiredTerminalEmitOnlyDefaultSeconds is the pre-STREAM-WAIT-2
// default (retired 2026-07-15, §29.92.1): 45s assumed the terminal
// emit-only request is a short structured emission. Reasoning models
// behind non-streaming-reasoning gateways re-enter a FULL thinking
// phase on that request and emit nothing until it completes, so the
// 45s ctx budget killed live requests as "context deadline exceeded"
// (customer witness MiniMax-M2.7).
const retiredTerminalEmitOnlyDefaultSeconds = 45

// TestAnalyzerTerminalEmitOnlyBudget_ReasoningGatewayScaledReplay keeps the
// non-streaming ownership arm pinned: a gateway that buffers its entire JSON
// response still needs the configured evaluator wall budget. Streaming
// adapters advertise their own first-byte/byte-stall watchdogs and BaseAgent
// deliberately does not layer this cumulative-age budget over them.
func TestAnalyzerTerminalEmitOnlyBudget_ReasoningGatewayScaledReplay(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.DefaultAnalysisLimits())

	e := &analyzerEvaluator{terminalEmitOnlyInstructionIssued: true}
	budget, reason := e.LLMRequestTimeout(&types.AgentContext{Stage: types.StageAnalyze}, LLMRequestBudgetObservation{
		ToolSurfaceKnown:   true,
		AvailableToolNames: map[string]bool{"emit_analysis": true},
	})
	if reason != "analyzer_terminal_emit_only" {
		t.Fatalf("evaluator reason = %q, want analyzer_terminal_emit_only", reason)
	}
	if budget != tool.DefaultAnalysisLimits().TerminalEmitOnlyRequestTimeout() {
		t.Fatalf("evaluator budget = %s, want the DefaultAnalysisLimits terminal emit-only budget", budget)
	}

	// 1000× compressed replay: 1s of wall clock becomes 1ms.
	scaledCurrent := budget / 1000
	scaledRetired := retiredTerminalEmitOnlyDefaultSeconds * time.Millisecond
	thinking := 90 * time.Millisecond // > retired budget, < current budget

	// Load-bearing ordering guard (default-value pin): the reasoning
	// gateway's thinking phase must overrun the retired budget and fit
	// inside the current one, otherwise the replay proves nothing.
	if scaledRetired >= thinking {
		t.Fatalf("scaled retired budget %v must be shorter than the thinking phase %v", scaledRetired, thinking)
	}
	if scaledCurrent <= thinking {
		t.Fatalf("scaled current default %v must outlast the reasoning thinking phase %v (default regressed below the reasoning-model-safe value?)", scaledCurrent, thinking)
	}

	// Reasoning-gateway stand-in: holds the ENTIRE response (headers
	// included) until the thinking phase completes, then answers with
	// a single terminal emit_analysis tool call.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(thinking):
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"emit_analysis","arguments":"{}"}}]}}]}`))
	}))
	defer server.Close()

	adapter := llm.NewOpenAIAdapter("test-key", "test-model", server.URL, llm.AdapterOptions{
		RequestTimeout:   30 * time.Second,
		RetryMaxAttempts: 1,
	})
	messages := []llm.Message{{Role: "user", Content: "emit the analysis now"}}
	tools := []llm.ToolSchema{{Name: "emit_analysis"}}

	t.Run("dead_under_retired_45_unit_budget", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), scaledRetired)
		defer cancel()
		_, err := adapter.Chat(ctx, messages, tools, llm.ChatOptions{})
		if err == nil {
			t.Fatalf("retired budget %v must kill a %v thinking phase, request survived", scaledRetired, thinking)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("retired-budget kill must surface the customer's deadline shape, got %v", err)
		}
	})

	t.Run("alive_under_current_default_budget", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), scaledCurrent)
		defer cancel()
		resp, err := adapter.Chat(ctx, messages, tools, llm.ChatOptions{})
		if err != nil {
			t.Fatalf("current default budget %v must let a %v thinking phase finish, got %v", scaledCurrent, thinking, err)
		}
		if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "emit_analysis" {
			t.Fatalf("terminal emit_analysis call must survive, got %+v", resp.ToolCalls)
		}
	})
}
