package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type hmc376WireReply struct {
	reasoning, content string
	call               llm.ToolCall
}

// hmc376WireAdapter is the real request-serializing adapter; it only records
// what the loop handed to it so the test can size requests exactly.
type hmc376WireAdapter struct {
	*llm.OpenAIAdapter
	messages [][]llm.Message
	tools    [][]llm.ToolSchema
}

func (a *hmc376WireAdapter) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	a.messages = append(a.messages, append([]llm.Message(nil), messages...))
	a.tools = append(a.tools, append([]llm.ToolSchema(nil), tools...))
	return a.OpenAIAdapter.Chat(ctx, messages, tools, opts)
}

type hmc376WireMessage struct {
	Role             string           `json:"role"`
	Content          *json.RawMessage `json:"content"`
	ReasoningContent *string          `json:"reasoning_content"`
	ToolCallID       string           `json:"tool_call_id"`
	ToolCalls        []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
}

type hmc376WireBody struct {
	Thinking *struct {
		Type string `json:"type"`
	} `json:"thinking"`
	Messages []hmc376WireMessage `json:"messages"`
}

// hmc376WireRun drives the finalizer against a provider that is in native
// thinking mode: every tool call comes back with reasoning on its own channel
// and with free text in the content channel.
func hmc376WireRun(t *testing.T, window int, replies []hmc376WireReply) (*hmc376WireAdapter, []hmc376WireBody, *types.MutableState, error) {
	t.Helper()
	ctx := hmc376CallChainContext(t)
	var mu sync.Mutex
	var bodies []hmc376WireBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body hmc376WireBody
		if err := json.Unmarshal(raw, &body); err != nil || len(body.Messages) == 0 {
			http.Error(w, "not a chat request", http.StatusNotFound)
			return
		}
		mu.Lock()
		bodies = append(bodies, body)
		n := len(bodies)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if n > len(replies) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"fixture has no further reply"}}`))
			return
		}
		if n == 1 {
			ctx.Mutable.SetLastRejectedAnswerDocumentV2(hmc376Draft())
		}
		reply := replies[n-1]
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"finish_reason": "tool_calls", "message": map[string]any{
				"role": "assistant", "reasoning_content": reply.reasoning, "content": reply.content,
				"tool_calls": []any{map[string]any{"id": reply.call.ID, "type": "function",
					"function": map[string]any{"name": reply.call.Name, "arguments": string(reply.call.Params)}}},
			}}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1},
		})
	}))
	defer server.Close()
	adapter := &hmc376WireAdapter{OpenAIAdapter: llm.NewOpenAIAdapter("test-key", "thinking-model", server.URL, llm.AdapterOptions{
		RequestTimeout: 30 * time.Second, RetryMaxAttempts: 1, ContextWindow: window, ProviderThinkingMode: "enabled",
	})}
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	registry.Register(&tool.EmitAnswerDocumentPatch{})
	base := NewFinalizerAgent(&Dependencies{LLM: adapter, Tools: registry, MaxIterations: len(replies) + 1,
		AgentSettings: types.AgentSettings{FinalizerRequestBytePolicy: types.FinalizerRequestByteEnforce}})
	_, err := base.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
	mu.Lock()
	defer mu.Unlock()
	return adapter, bodies, ctx.Mutable, err
}

// Wire safety of the repair ladder, checked on the bytes the provider receives.
// A provider in native thinking mode can reject a tool-call turn whose
// reasoning is not echoed back. The ladder gives up only the content channel:
// every tool-call turn that is sent still carries the reasoning it was returned
// with, a JSON string content, its own call and its paired result. Reasoning
// on the provider's channel is therefore content the ladder must keep: a
// request that is over the allocation because of it is refused, never sent
// without it.
func TestHMC376RepairRequestWireKeepsEveryToolCallTurnSendable(t *testing.T) {
	replies := func(latestReasoning int) []hmc376WireReply {
		return []hmc376WireReply{
			{strings.Repeat("NATIVE-ONE ", 30), strings.Repeat("INLINE-ONE ", 2800), llm.ToolCall{ID: "call-one", Name: hmc376Full, Params: json.RawMessage(`{"blocks":[],"caveats":["submitted premise"]}`)}},
			{strings.Repeat("NATIVE-TWO ", 30), strings.Repeat("INLINE-TWO ", 2800), hmc376UnstagedPatch("call-two", "guess-two")},
			{strings.Repeat("NATIVE-THREE ", latestReasoning), strings.Repeat("INLINE-THREE ", 9000), hmc376UnstagedPatch("call-three", "guess-three")},
		}
	}
	script := replies(30)
	wide, wideBodies, _, err := hmc376WireRun(t, 1_000_000, script)
	if err == nil || len(wideBodies) != 4 || len(wide.messages) != 4 {
		t.Fatalf("fixture: requests=%d err=%v", len(wideBodies), err)
	}
	first := llm.EstimateRequestBytes(wide.messages[0], wide.tools[0])
	window := hmc376Window(first + 9000)
	allocation := hmc376Allocation(window)
	tight, bodies, state, err := hmc376WireRun(t, window, script)
	var refused *AnswerContextByteBudgetError
	if errors.As(err, &refused) || len(bodies) != 4 {
		t.Fatalf("the repair request did not reach the provider: requests=%d err=%v", len(bodies), err)
	}
	if size := llm.EstimateRequestBytes(tight.messages[3], tight.tools[3]); size > allocation || state.AnswerDocumentV2() != nil {
		t.Fatalf("allocation bypassed or an answer was published: %d > %d", size, allocation)
	}
	body, reference := bodies[3], wideBodies[3]
	if body.Thinking == nil || body.Thinking.Type != "enabled" || len(body.Messages) != len(reference.Messages) {
		t.Fatalf("fixture: thinking=%v messages=%d/%d", body.Thinking, len(body.Messages), len(reference.Messages))
	}
	turns := 0
	for i, message := range body.Messages {
		want := reference.Messages[i]
		if message.Content == nil || len(*message.Content) == 0 || (*message.Content)[0] != '"' {
			t.Fatalf("message %d content is not a JSON string on the wire", i)
		}
		if message.Role != "assistant" {
			if message.Role != want.Role || message.ToolCallID != want.ToolCallID || (i > 0 && string(*message.Content) != string(*want.Content)) {
				t.Fatalf("message %d (%s) changed on the wire", i, message.Role)
			}
			continue
		}
		reply := script[turns]
		turns++
		if string(*message.Content) != `""` || string(*want.Content) == `""` {
			t.Fatalf("turn %d: expected only its free text to be given up", turns)
		}
		if message.ReasoningContent == nil || *message.ReasoningContent != reply.reasoning {
			t.Fatalf("turn %d was sent without the reasoning it was returned with", turns)
		}
		if len(message.ToolCalls) != 1 || message.ToolCalls[0].ID != reply.call.ID || message.ToolCalls[0].Type != "function" ||
			message.ToolCalls[0].Function.Name != reply.call.Name || message.ToolCalls[0].Function.Arguments != string(reply.call.Params) {
			t.Fatalf("turn %d: submitted call changed on the wire", turns)
		}
		if next := body.Messages[i+1]; next.Role != "tool" || next.ToolCallID != reply.call.ID || string(*next.Content) != string(*reference.Messages[i+1].Content) {
			t.Fatalf("turn %d is not followed by its own unchanged result", turns)
		}
	}
	if turns != len(script) {
		t.Fatalf("tool-call turns on the wire: %d", turns)
	}

	// Control: the same lane with reasoning on the provider's channel that is
	// too large for the allocation. It is not dropped, so nothing is sent.
	heavy := replies(9000)
	blocked, blockedBodies, state, err := hmc376WireRun(t, window, heavy)
	if !errors.As(err, &refused) || refused.ActualBytes <= refused.AllocatedBytes || len(blockedBodies) != 3 || len(blocked.messages) != 3 || state.AnswerDocumentV2() != nil {
		t.Fatalf("a tool-call turn was sent without its reasoning, or an answer was published: requests=%d err=%v", len(blockedBodies), err)
	}
}
