package llm

import (
	"encoding/json"
	"strings"
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

func TestRecoverTextToolCalls_RequiredModeSkipsUnknownEmbeddedJSONObjects(t *testing.T) {
	tools := []ToolSchema{{Name: "emit_hypothesis_verdict"}}
	content := `{"name":"unknown","arguments":{"completeness":"unknown"}} ` +
		`{"name":"emit_hypothesis_verdict","arguments":{"items":[{"hypothesis_id":"h1","status":"inconclusive","rationale":"not enough evidence","citation":""}]}}`
	got := recoverTextToolCalls(Response{Content: content}, tools, ChatOptions{ToolChoice: "required"})
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Name != "emit_hypothesis_verdict" {
		t.Fatalf("expected known embedded tool call to recover despite unknown sidecar, got %+v", got.ToolCalls)
	}
}

func TestRecoverTextToolCalls_RequiredModeEmbeddedFencedBlock(t *testing.T) {
	content := "Here is the function call:\n\n```\n{\"name\":\"read_file\",\"arguments\":{\"path\":\"a.go\"}}\n```"
	got := recoverTextToolCalls(Response{Content: content}, []ToolSchema{{Name: "read_file"}}, ChatOptions{ToolChoice: "required"})
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Name != "read_file" {
		t.Fatalf("expected one recovered fenced call, got %+v", got.ToolCalls)
	}
}

func TestRecoverTextToolCalls_RequiredModeBareArgsUniqueSchema(t *testing.T) {
	content := `{
		"intent":"return_value",
		"scenario":"generic",
		"complexity":"simple",
		"keywords":["recoverTextToolCalls"],
		"entities":[],
		"question_kind":"unknown",
		"intent_confidence":0.6,
		"complexity_confidence":0.5,
		"kind_confidence":0.7,
		"predicates":{"is_scalar_answer":true},
		"diagnostic_profile":{"is_diagnostic":false},
		"answer_role_profile":{"is_role_binding_requested":false},
		"error_granularity_profile":{"is_granularity_question":false}
	}`
	tools := []ToolSchema{
		{
			Name:       "grep",
			Parameters: json.RawMessage(`{"type":"object","required":["pattern"]}`),
		},
		{
			Name: "emit_analysis",
			Parameters: json.RawMessage(`{
				"type":"object",
				"required":["intent","scenario","complexity","keywords","entities","question_kind","intent_confidence","complexity_confidence","kind_confidence","predicates","diagnostic_profile","answer_role_profile","error_granularity_profile"]
			}`),
		},
	}
	got := recoverTextToolCalls(Response{Content: content}, tools, ChatOptions{ToolChoice: "required"})
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Name != "emit_analysis" {
		t.Fatalf("expected bare args to recover as emit_analysis, got content=%q calls=%+v", got.Content, got.ToolCalls)
	}
	var params map[string]any
	if err := json.Unmarshal(got.ToolCalls[0].Params, &params); err != nil {
		t.Fatalf("params json: %v", err)
	}
	if params["intent"] != "return_value" {
		t.Fatalf("intent = %#v", params["intent"])
	}
}

func TestRecoverTextToolCalls_BareArgsConservativeBoundaries(t *testing.T) {
	content := `{"pattern":"Agent"}`
	tools := []ToolSchema{
		{Name: "grep", Parameters: json.RawMessage(`{"type":"object","required":["pattern"]}`)},
		{Name: "other_search", Parameters: json.RawMessage(`{"type":"object","required":["pattern"]}`)},
	}
	got := recoverTextToolCalls(Response{Content: content}, tools, ChatOptions{ToolChoice: "required"})
	if len(got.ToolCalls) != 0 || got.Content != content {
		t.Fatalf("ambiguous bare args should stay as content, got content=%q calls=%+v", got.Content, got.ToolCalls)
	}

	got = recoverTextToolCalls(Response{Content: content}, tools[:1], ChatOptions{ToolChoice: "auto"})
	if len(got.ToolCalls) != 0 || got.Content != content {
		t.Fatalf("auto mode should not recover bare args, got content=%q calls=%+v", got.Content, got.ToolCalls)
	}
}

func TestRecoverTextToolCalls_NamedToolChoiceBareArgs(t *testing.T) {
	content := `Here is the call:

` + "```json" + `
{"pattern":"Agent","files_only":true}
` + "```"
	tools := []ToolSchema{
		{Name: "grep", Parameters: json.RawMessage(`{"type":"object","required":["pattern","files_only"]}`)},
		{Name: "read_file", Parameters: json.RawMessage(`{"type":"object","required":["path"]}`)},
	}
	got := recoverTextToolCalls(Response{Content: content}, tools, ChatOptions{
		ToolChoice: `{"type":"function","function":{"name":"grep"}}`,
	})
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Name != "grep" {
		t.Fatalf("expected named tool_choice bare args recovery, got %+v", got.ToolCalls)
	}
}

func TestRecoverTextToolCalls_ReActAliases(t *testing.T) {
	content := `{"action":"grep","action_input":{"pattern":"Agent","files_only":true}}`
	tools := []ToolSchema{{
		Name:       "grep",
		Parameters: json.RawMessage(`{"type":"object","required":["pattern","files_only"]}`),
	}}
	got := recoverTextToolCalls(Response{Content: content}, tools, ChatOptions{ToolChoice: "required"})
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Name != "grep" {
		t.Fatalf("expected ReAct-style alias recovery, got %+v", got.ToolCalls)
	}
	if string(got.ToolCalls[0].Params) != `{"files_only":true,"pattern":"Agent"}` {
		t.Fatalf("params = %s", got.ToolCalls[0].Params)
	}
}

func TestRecoverTextToolCalls_PrunesUnknownFieldsFromRecoveredParams(t *testing.T) {
	content := `{
		"blocks":[{
			"id":"l1",
			"kind":"ordered_list",
			"item_count":4,
			"items":[{"id":"i1","label":"recover_text_tool_calls","text":"found","citation_ref":0,"extra_item_field":"drop me"}]
		}],
		"citations":[{"file":"internal/llm/content_tool_call_recovery.go","line":27}]
	}`
	tools := []ToolSchema{{
		Name: "emit_answer_document",
		Parameters: json.RawMessage(`{
			"type":"object",
			"properties":{
				"blocks":{
					"type":"array",
					"items":{
						"type":"object",
						"properties":{
							"id":{"type":"string"},
							"kind":{"type":"string"},
							"items":{
								"type":"array",
								"items":{
									"type":"object",
									"properties":{
										"id":{"type":"string"},
										"label":{"type":"string"},
										"text":{"type":"string"},
										"citation_ref":{"type":"integer"}
									}
								}
							}
						},
						"required":["id","kind"]
					}
				},
				"citations":{"type":"array","items":{"type":"object"}}
			},
			"required":["blocks"]
		}`),
	}}
	got := recoverTextToolCalls(Response{Content: content}, tools, ChatOptions{ToolChoice: "required"})
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Name != "emit_answer_document" {
		t.Fatalf("expected recovered emit_answer_document, got %+v", got.ToolCalls)
	}
	params := string(got.ToolCalls[0].Params)
	if strings.Contains(params, "item_count") || strings.Contains(params, "extra_item_field") {
		t.Fatalf("unknown fields should be pruned from recovered params, got %s", params)
	}
	if !strings.Contains(params, `"citation_ref":0`) {
		t.Fatalf("known nested field should remain, got %s", params)
	}
}

func TestRecoverTextToolCalls_RepairsMissingTrailingObjectCloser(t *testing.T) {
	content := `{"name":"emit_analysis","arguments":{"entities":["recoverTextToolCalls"],"question_kind":"unknown"}`
	got := recoverTextToolCalls(Response{Content: content}, []ToolSchema{{Name: "emit_analysis"}}, ChatOptions{})
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Name != "emit_analysis" {
		t.Fatalf("expected missing final object closer to recover, got content=%q calls=%+v", got.Content, got.ToolCalls)
	}
	if got.Content != "" || got.StopReason != "tool_use" {
		t.Fatalf("recovered call should clear content and use tool stop, got stop=%q content=%q", got.StopReason, got.Content)
	}
}

func TestRecoverTextToolCalls_DoesNotRepairMidMemberTruncation(t *testing.T) {
	content := `{"name":"emit_analysis","arguments":{"entities":["recoverTextToolCalls"],`
	got := recoverTextToolCalls(Response{Content: content}, []ToolSchema{{Name: "emit_analysis"}}, ChatOptions{})
	if len(got.ToolCalls) != 0 || got.Content != content {
		t.Fatalf("mid-member truncation should stay as content, got content=%q calls=%+v", got.Content, got.ToolCalls)
	}
}

func TestRecoverTextToolCalls_RepairsSingleQuotedJSONArguments(t *testing.T) {
	content := `{"name":"emit_analysis","arguments": '{"entities":["recoverTextToolCalls"],"question_kind":"unknown"}'`
	got := recoverTextToolCalls(Response{Content: content}, []ToolSchema{{Name: "emit_analysis"}}, ChatOptions{})
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Name != "emit_analysis" {
		t.Fatalf("expected single-quoted JSON arguments to recover, got content=%q calls=%+v", got.Content, got.ToolCalls)
	}
	if string(got.ToolCalls[0].Params) != `{"entities":["recoverTextToolCalls"],"question_kind":"unknown"}` {
		t.Fatalf("params = %s", got.ToolCalls[0].Params)
	}
}

func TestRecoverTextToolCalls_DoesNotRepairSingleQuotedProseArguments(t *testing.T) {
	content := `{"name":"emit_analysis","arguments": 'entities are recoverTextToolCalls'}`
	got := recoverTextToolCalls(Response{Content: content}, []ToolSchema{{Name: "emit_analysis"}}, ChatOptions{})
	if len(got.ToolCalls) != 0 || got.Content != content {
		t.Fatalf("single-quoted prose should stay as content, got content=%q calls=%+v", got.Content, got.ToolCalls)
	}
}

func TestRecoverTextToolCalls_WrapsSingleRequiredArrayArguments(t *testing.T) {
	content := `{"name":"emit_evidence","arguments":[{"source":"a.go","line_start":3}]}`
	tools := []ToolSchema{{
		Name: "emit_evidence",
		Parameters: json.RawMessage(`{
			"type":"object",
			"properties":{"items":{"type":"array"}},
			"required":["items"]
		}`),
	}}
	got := recoverTextToolCalls(Response{Content: content}, tools, ChatOptions{})
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Name != "emit_evidence" {
		t.Fatalf("expected array arguments to wrap under items, got content=%q calls=%+v", got.Content, got.ToolCalls)
	}
	var params struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(got.ToolCalls[0].Params, &params); err != nil {
		t.Fatalf("params json: %v", err)
	}
	if len(params.Items) != 1 || params.Items[0]["source"] != "a.go" {
		t.Fatalf("items = %#v", params.Items)
	}
}

func TestRecoverTextToolCalls_DoesNotWrapArrayArgumentsWithoutSchemaMatch(t *testing.T) {
	content := `{"name":"emit_evidence","arguments":[{"source":"a.go"}]}`
	tools := []ToolSchema{{
		Name:       "emit_evidence",
		Parameters: json.RawMessage(`{"type":"object","properties":{"item":{"type":"object"}},"required":["item"]}`),
	}}
	got := recoverTextToolCalls(Response{Content: content}, tools, ChatOptions{})
	if len(got.ToolCalls) != 0 || got.Content != content {
		t.Fatalf("array arguments without single required array field should stay as content, got content=%q calls=%+v", got.Content, got.ToolCalls)
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
