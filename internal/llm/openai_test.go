package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBuildRequest_ToolChoiceWire locks the wire-format contract for
// ChatOptions.ToolChoice. The OpenAI API takes `tool_choice` as either
// a string ("auto" / "required" / "none") or an object (force a
// specific function); codrax only forwards the string form today.
// Three invariants matter:
//   - empty or "auto" → field omitted (same payload as pre-options callers)
//   - "required" → field present as JSON string "required"
//   - tool_choice is never emitted when tools[] is empty (OpenAI rejects that)
func TestBuildRequest_ToolChoiceWire(t *testing.T) {
	schema := []ToolSchema{
		{Name: "emit_analysis", Description: "x", Parameters: json.RawMessage(`{"type":"object"}`)},
	}
	msgs := []Message{{Role: "user", Content: "hi"}}
	adapter := &OpenAIAdapter{model: "m"}

	t.Run("empty_options_omits_tool_choice", func(t *testing.T) {
		req := adapter.buildRequest(msgs, schema, ChatOptions{})
		b, _ := json.Marshal(req)
		if strings.Contains(string(b), `"tool_choice"`) {
			t.Errorf("empty options should omit tool_choice, got: %s", b)
		}
	})

	t.Run("auto_omits_tool_choice", func(t *testing.T) {
		req := adapter.buildRequest(msgs, schema, ChatOptions{ToolChoice: "auto"})
		b, _ := json.Marshal(req)
		if strings.Contains(string(b), `"tool_choice"`) {
			t.Errorf("auto should omit tool_choice (matches provider default), got: %s", b)
		}
	})

	t.Run("required_emits_string", func(t *testing.T) {
		req := adapter.buildRequest(msgs, schema, ChatOptions{ToolChoice: "required"})
		b, _ := json.Marshal(req)
		if !strings.Contains(string(b), `"tool_choice":"required"`) {
			t.Errorf("required should serialize as \"tool_choice\":\"required\", got: %s", b)
		}
	})

	t.Run("required_without_tools_still_omitted", func(t *testing.T) {
		req := adapter.buildRequest(msgs, nil, ChatOptions{ToolChoice: "required"})
		b, _ := json.Marshal(req)
		// Without tools[] an OpenAI 400 would rejects tool_choice; we
		// omit it so the adapter still produces a valid request when a
		// caller declared no tools but asked for required.
		if strings.Contains(string(b), `"tool_choice"`) {
			t.Errorf("empty tools should suppress tool_choice even when required, got: %s", b)
		}
	})
}
