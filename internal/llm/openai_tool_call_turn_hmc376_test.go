package llm

import (
	"encoding/json"
	"testing"
)

// A repair request may stop re-sending the free text that accompanied an
// earlier tool call. This pins what such a turn looks like on the wire of the
// one adapter that serializes requests, in every provider thinking mode:
// the content stays a JSON string (never null or absent), the tool call and
// its paired result are unchanged, and reasoning the provider returned on its
// own channel is echoed back exactly when it is supplied. A provider in native
// thinking mode can reject a tool-call turn without that echo, so callers must
// leave it on the message; this adapter never drops or invents it.
func TestBuildRequest_ToolCallTurnWithoutFreeTextKeepsItsWireShape(t *testing.T) {
	const arguments = `{"blocks":[],"caveats":["submitted premise"]}`
	messages := []Message{
		{Role: "system", Content: "teaching"},
		{Role: "user", Content: "request"},
		{Role: "assistant", Content: "", ReasoningContent: "native reasoning of the first call",
			ToolCalls: []ToolCall{{ID: "call-1", Name: "emit_answer_document", Params: json.RawMessage(arguments)}}},
		{Role: "tool", ToolCallID: "call-1", Content: "validator result one"},
		{Role: "user", Content: "correction one"},
		{Role: "assistant", Content: "",
			ToolCalls: []ToolCall{{ID: "call-2", Name: "emit_answer_document_patch", Params: json.RawMessage(`{"remove_block_ids":["b"]}`)}}},
		{Role: "tool", ToolCallID: "call-2", Content: "validator result two"},
	}
	schema := []ToolSchema{{Name: "emit_answer_document_patch", Parameters: json.RawMessage(`{"type":"object"}`)}}
	for _, provider := range []struct {
		name, model, baseURL, thinking string
	}{
		{"native thinking enabled", "deepseek-v4-pro", "https://api.deepseek.com", "enabled"},
		{"native thinking disabled by default", "deepseek-v4-flash", "https://api.deepseek.com", ""},
		{"provider default", "any-model", "https://example.test/v1", "provider_default"},
		{"unknown provider", "any-model", "https://example.test/v1", ""},
	} {
		t.Run(provider.name, func(t *testing.T) {
			adapter := NewOpenAIAdapter("k", provider.model, provider.baseURL, testAdapterOpts(AdapterOptions{ProviderThinkingMode: provider.thinking}))
			body, err := adapter.marshalRequest(adapter.buildRequest(messages, schema, ChatOptions{ToolChoice: "required"}))
			if err != nil {
				t.Fatal(err)
			}
			var wire struct {
				Messages []map[string]json.RawMessage `json:"messages"`
			}
			if err := json.Unmarshal(body, &wire); err != nil || len(wire.Messages) != len(messages) {
				t.Fatalf("wire request: %v / %d messages", err, len(wire.Messages))
			}
			for i, want := range messages {
				got := wire.Messages[i]
				var content any
				if raw, present := got["content"]; !present || json.Unmarshal(raw, &content) != nil {
					t.Fatalf("message %d has no content field: %s", i, body)
				}
				if text, isString := content.(string); !isString || text != want.Content {
					t.Fatalf("message %d content must stay the exact JSON string %q, got %s", i, want.Content, got["content"])
				}
				reasoning, echoed := got["reasoning_content"]
				if want.ReasoningContent == "" && echoed {
					t.Fatalf("message %d invented reasoning: %s", i, reasoning)
				}
				if want.ReasoningContent != "" {
					var text string
					if !echoed || json.Unmarshal(reasoning, &text) != nil || text != want.ReasoningContent {
						t.Fatalf("message %d lost the reasoning returned with its tool call: %s", i, body)
					}
				}
				if len(want.ToolCalls) == 0 {
					if _, present := got["tool_calls"]; present {
						t.Fatalf("message %d gained tool calls", i)
					}
					continue
				}
				var calls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				}
				if err := json.Unmarshal(got["tool_calls"], &calls); err != nil || len(calls) != 1 || calls[0].ID != want.ToolCalls[0].ID || calls[0].Type != "function" ||
					calls[0].Function.Name != want.ToolCalls[0].Name || calls[0].Function.Arguments != string(want.ToolCalls[0].Params) {
					t.Fatalf("message %d tool call changed on the wire: %s", i, got["tool_calls"])
				}
				var next struct {
					Role       string `json:"role"`
					ToolCallID string `json:"tool_call_id"`
				}
				paired, _ := json.Marshal(wire.Messages[i+1])
				if err := json.Unmarshal(paired, &next); err != nil || next.Role != "tool" || next.ToolCallID != calls[0].ID {
					t.Fatalf("message %d is not followed by its own result: %s", i, paired)
				}
			}
		})
	}
}
