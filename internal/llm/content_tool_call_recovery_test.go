package llm

import (
	"encoding/json"
	"testing"
)

func TestRecoverTextToolCalls_NameParametersEnvelope(t *testing.T) {
	resp := Response{
		Content:    `{"name":"emit_analysis","parameters":{"entities":["Agent"],"question_kind":"registration"}}`,
		StopReason: "end_turn",
	}
	got := recoverTextToolCalls(resp, []ToolSchema{{Name: "emit_analysis"}}, ChatOptions{ToolChoice: "required"})
	if got.StopReason != "tool_use" || len(got.ToolCalls) != 1 {
		t.Fatalf("expected recovered tool call, got stop=%q calls=%+v", got.StopReason, got.ToolCalls)
	}
	if got.Content != "" {
		t.Fatalf("recovered tool-call envelope should not remain as assistant prose, got %q", got.Content)
	}
	if got.ToolCalls[0].Name != "emit_analysis" {
		t.Fatalf("tool name = %q", got.ToolCalls[0].Name)
	}
	var params map[string]any
	if err := json.Unmarshal(got.ToolCalls[0].Params, &params); err != nil {
		t.Fatalf("params json: %v", err)
	}
	if _, ok := params["entities"].([]any); !ok {
		t.Fatalf("entities should remain a JSON array, got %#v", params["entities"])
	}
}

func TestRecoverTextToolCalls_OpenAIToolCallsEnvelope(t *testing.T) {
	resp := Response{Content: `{
		"tool_calls":[{
			"id":"call_1",
			"type":"function",
			"function":{"name":"grep","arguments":"{\"pattern\":\"Agent\",\"files_only\":true}"}
		}]
	}`}
	got := recoverTextToolCalls(resp, []ToolSchema{{Name: "grep"}}, ChatOptions{})
	if len(got.ToolCalls) != 1 {
		t.Fatalf("expected one recovered call, got %+v", got.ToolCalls)
	}
	if got.ToolCalls[0].ID != "call_1" || got.ToolCalls[0].Name != "grep" {
		t.Fatalf("unexpected call: %+v", got.ToolCalls[0])
	}
	if string(got.ToolCalls[0].Params) != `{"files_only":true,"pattern":"Agent"}` {
		t.Fatalf("params = %s", got.ToolCalls[0].Params)
	}
}

func TestRecoverTextToolCalls_TaggedAndFencedEnvelope(t *testing.T) {
	tools := []ToolSchema{{Name: "read_file"}}
	cases := []string{
		"<tool_call>{\"name\":\"read_file\",\"arguments\":{\"path\":\"a.go\"}}</tool_call>",
		"```json\n{\"name\":\"read_file\",\"arguments\":{\"path\":\"a.go\"}}\n```",
	}
	for _, content := range cases {
		got := recoverTextToolCalls(Response{Content: content}, tools, ChatOptions{})
		if len(got.ToolCalls) != 1 || got.ToolCalls[0].Name != "read_file" {
			t.Fatalf("content %q did not recover read_file call: %+v", content, got.ToolCalls)
		}
	}
}

func TestRecoverTextToolCalls_RequiredModeEmbeddedBlocks(t *testing.T) {
	tools := []ToolSchema{{Name: "read_file"}, {Name: "grep"}}
	content := "I will call the tools now:\n\n" +
		"<tool_call>{\"name\":\"read_file\",\"arguments\":{\"path\":\"a.go\"}}</tool_call>\n" +
		"<tool_call>{\"name\":\"grep\",\"arguments\":{\"pattern\":\"Agent\",\"files_only\":true}}</tool_call>"
	got := recoverTextToolCalls(Response{Content: content}, tools, ChatOptions{ToolChoice: "required"})
	if len(got.ToolCalls) != 2 {
		t.Fatalf("expected two recovered calls, got %+v", got.ToolCalls)
	}
	if got.ToolCalls[0].Name != "read_file" || got.ToolCalls[1].Name != "grep" {
		t.Fatalf("tool order/name mismatch: %+v", got.ToolCalls)
	}
	if got.ToolCalls[0].ID != "content_tool_call_0" || got.ToolCalls[1].ID != "content_tool_call_1" {
		t.Fatalf("generated ids should be stable across multiple blocks: %+v", got.ToolCalls)
	}
}

func TestRecoverTextToolCalls_RequiredModeEmbeddedJSONObjects(t *testing.T) {
	tools := []ToolSchema{{Name: "repo_map"}, {Name: "grep"}}
	content := `I will call them: ` +
		`{"name":"repo_map","parameters":{"path":"internal/types/context.go"}}; ` +
		`{"name":"grep","parameters":{"pattern":"Agent","files_only":true}}`
	got := recoverTextToolCalls(Response{Content: content}, tools, ChatOptions{ToolChoice: "required"})
	if len(got.ToolCalls) != 2 {
		t.Fatalf("expected two recovered calls, got %+v", got.ToolCalls)
	}
	if got.ToolCalls[0].Name != "repo_map" || got.ToolCalls[1].Name != "grep" {
		t.Fatalf("tool order/name mismatch: %+v", got.ToolCalls)
	}
}

func TestRecoverTextToolCalls_RequiredModeEmbeddedFencedBlock(t *testing.T) {
	content := "Here is the function call:\n\n```\n{\"name\":\"read_file\",\"arguments\":{\"path\":\"a.go\"}}\n```"
	got := recoverTextToolCalls(Response{Content: content}, []ToolSchema{{Name: "read_file"}}, ChatOptions{ToolChoice: "required"})
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Name != "read_file" {
		t.Fatalf("expected one recovered fenced call, got %+v", got.ToolCalls)
	}
}

func TestRecoverTextToolCalls_ConservativeBoundaries(t *testing.T) {
	tools := []ToolSchema{{Name: "emit_analysis"}}
	cases := []struct {
		name string
		resp Response
		opts ChatOptions
	}{
		{
			name: "prose wrapper",
			resp: Response{Content: `I will call {"name":"emit_analysis","parameters":{}} now.`},
		},
		{
			name: "unknown tool",
			resp: Response{Content: `{"name":"not_a_tool","parameters":{}}`},
		},
		{
			name: "malformed json",
			resp: Response{Content: `{"name":"emit_analysis","parameters":{"buckets":=[]}}`},
		},
		{
			name: "tool choice none",
			resp: Response{Content: `{"name":"emit_analysis","parameters":{}}`},
			opts: ChatOptions{ToolChoice: "none"},
		},
		{
			name: "embedded envelope requires required tool choice",
			resp: Response{Content: "Here is the function call:\n```json\n{\"name\":\"emit_analysis\",\"parameters\":{}}\n```"},
			opts: ChatOptions{ToolChoice: "auto"},
		},
		{
			name: "real tool calls already present",
			resp: Response{
				Content:   `{"name":"emit_analysis","parameters":{}}`,
				ToolCalls: []ToolCall{{Name: "emit_analysis", Params: json.RawMessage(`{}`)}},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := recoverTextToolCalls(c.resp, tools, c.opts)
			if c.name == "real tool calls already present" {
				if len(got.ToolCalls) != 1 || string(got.ToolCalls[0].Params) != `{}` || got.Content != c.resp.Content {
					t.Fatalf("existing protocol tool call should stay untouched: %+v", got)
				}
				return
			}
			if len(got.ToolCalls) != 0 || got.Content != c.resp.Content {
				t.Fatalf("should not recover %s, got content=%q calls=%+v", c.name, got.Content, got.ToolCalls)
			}
		})
	}
}
