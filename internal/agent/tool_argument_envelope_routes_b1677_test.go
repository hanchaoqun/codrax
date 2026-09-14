package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/mcp"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// These counters are at the actual local/MCP dispatch boundary. A schema
// fingerprint is an optimization, not a receipt for original argument identity.
type b1677RouteTool struct {
	tool.ReadOnly
	tool.NonEvidenceTool
	name   string
	schema json.RawMessage
	calls  int
	got    json.RawMessage
}

func (t *b1677RouteTool) Name() string                { return t.name }
func (t *b1677RouteTool) Description() string         { return "Captures actual route execution." }
func (t *b1677RouteTool) Parameters() json.RawMessage { return t.schema }
func (t *b1677RouteTool) Execute(_ *types.BusContext, raw json.RawMessage) (types.ToolResult, error) {
	t.calls++
	t.got = append(json.RawMessage(nil), raw...)
	return types.ToolResult{ToolName: t.Name(), Success: true}, nil
}

type b1677RouteMCP struct {
	captureMCPServer
	calls int
}

func (s *b1677RouteMCP) CallTool(name string, raw json.RawMessage) (types.MCPResponse, error) {
	s.calls++
	return s.captureMCPServer.CallTool(name, raw)
}

type b1677RouteLLM struct {
	mixedContentToolLLM
	call    llm.ToolCall
	calls   int
	offered bool
}

func (l *b1677RouteLLM) Chat(_ context.Context, _ []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	for _, schema := range schemas {
		l.offered = l.offered || schema.Name == l.call.Name
	}
	return llm.Response{ToolCalls: []llm.ToolCall{l.call}}, nil
}

func TestB1677RoutesMCPRejectsAmbiguityDespiteMatchingFingerprint(t *testing.T) {
	for _, mode := range []string{types.ToolParamCompatOff, types.ToolParamCompatAudit, types.ToolParamCompatRepair} {
		for _, fingerprint := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fingerprint=%t", mode, fingerprint), func(t *testing.T) {
				server := &b1677RouteMCP{}
				registry := mcp.NewRegistry()
				registry.Register(server)
				base := NewBaseAgent(types.AgentExplorer, &Dependencies{MCPServers: registry,
					ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{types.AgentExplorer: {Mode: mode}}}, nil)
				raw := json.RawMessage(`{"arguments":{"pattern":"first"},"arg\u0075ments":{"pattern":"second"}}`)
				call := llm.ToolCall{ID: "b1677-mcp", Name: "capture_mcp__capture_mcp_params", Params: raw}
				if fingerprint {
					call.ParamSchemaFingerprint = toolParamSchemaFingerprint(server.ListTools()[0].Parameters)
				}
				res, response := base.executeTool(&types.AgentContext{Stage: types.StageExplore, Mutable: types.NewMutableState("MCP argument identity")}, call)
				if server.calls != 0 || server.got != nil || res == nil || res.Success || response != nil {
					t.Fatalf("ambiguous arguments reached MCP: calls=%d raw=%s local=%+v mcp=%+v", server.calls, server.got, res, response)
				}
				if string(raw) != string(call.Params) {
					t.Fatal("checking argument integrity mutated the input")
				}
			})
		}
	}
}

func TestB1677RoutesLocalMatchingFingerprintIsNotIntegrityAuthority(t *testing.T) {
	capture := &b1677RouteTool{name: "capture_route", schema: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"}}}`)}
	registry := tool.NewRegistry()
	registry.Register(capture)
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: registry,
		ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{types.AgentExplorer: {Mode: types.ToolParamCompatRepair}}}, nil)
	call := llm.ToolCall{ID: "b1677-local-fingerprint", Name: capture.Name(),
		Params:                 json.RawMessage(`{"function":{"arguments":{"pattern":"first"}},"function":{"arguments":{"pattern":"second"}}}`),
		ParamSchemaFingerprint: toolParamSchemaFingerprint(capture.Parameters())}
	res, _ := base.executeTool(&types.AgentContext{Stage: types.StageExplore}, call)
	if capture.calls != 0 || res == nil || res.Success {
		t.Fatalf("schema fingerprint bypassed original argument integrity: calls=%d result=%+v", capture.calls, res)
	}
}

func TestB1677RoutesAnalyzerGrepChecksIdentityBeforeFilesOnlyRewrite(t *testing.T) {
	for _, dynamic := range []bool{false, true} {
		for _, ambiguous := range []bool{false, true} {
			t.Run(fmt.Sprintf("dynamic=%t/ambiguous=%t", dynamic, ambiguous), func(t *testing.T) {
				capture := &b1677RouteTool{name: "grep", schema: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"files_only":{"type":"boolean"}}}`)}
				registry := tool.NewRegistry()
				registry.Register(capture)
				base := NewBaseAgent(types.AgentAnalyzer, &Dependencies{Tools: registry,
					ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{types.AgentAnalyzer: {Mode: types.ToolParamCompatRepair}}}, nil)
				raw := json.RawMessage(`{"arguments":{"pattern":"kept"}}`)
				if ambiguous {
					raw = json.RawMessage(`{"arguments":{"pattern":"first"},"arguments":{"pattern":"second"}}`)
				}
				call := llm.ToolCall{ID: "b1677-analyzer", Name: "grep", Params: raw}
				if dynamic {
					call = base.normalizeToolCallParams([]llm.ToolCall{call}, []llm.ToolSchema{{Name: capture.Name(), Parameters: capture.Parameters()}})[0]
					if ambiguous && string(call.Params) != string(raw) {
						t.Errorf("dynamic schema washed away competing grep patterns: %s", call.Params)
					}
				}
				res, _ := base.executeTool(&types.AgentContext{Stage: types.StageAnalyze}, call)
				if ambiguous {
					if capture.calls != 0 || res == nil || res.Success {
						t.Fatalf("files_only augmentation laundered competing patterns: calls=%d raw=%s result=%+v", capture.calls, capture.got, res)
					}
					return
				}
				var got struct {
					Pattern   string `json:"pattern"`
					FilesOnly bool   `json:"files_only"`
				}
				if err := json.Unmarshal(capture.got, &got); err != nil || capture.calls != 1 || res == nil || !res.Success || got.Pattern != "kept" || !got.FilesOnly {
					t.Fatalf("legitimate analyzer augmentation lost: err=%v calls=%d raw=%s result=%+v", err, capture.calls, capture.got, res)
				}
			})
		}
	}
}

// Exercise the exported agent loop, including its offered-schema normalization,
// history copy and eventual registry/MCP invocation, not just a helper return.
func TestB1677RoutesPublicExecuteKeepsNativeAndUnconsumedMetadata(t *testing.T) {
	for _, route := range []string{"local", "mcp"} {
		for _, tc := range []struct {
			name, raw string
			ambiguous bool
		}{
			{"unique", `{"arguments":{"pattern":"kept"}}`, false},
			{"equivalent", `{"arguments":{"pattern":"kept"},"arguments": { "pattern":"kept" }}`, false},
			{"irrelevant metadata", `{"id":"first","id":"second","arguments":{"pattern":"kept"}}`, false},
			{"unconsumed function", `{"arguments":{"pattern":"kept"},"function":{"arguments":{"pattern":"ignored-first"}},"function":{"arguments":{"pattern":"ignored-second"}}}`, false},
			{"ambiguous", `{"arguments":{"pattern":"first"},"arguments":{"pattern":"second"}}`, true},
		} {
			t.Run(route+"/"+tc.name, func(t *testing.T) {
				capture := &b1677RouteTool{name: "capture_route", schema: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"}}}`)}
				server := &b1677RouteMCP{}
				deps := &Dependencies{MaxIterations: 1, ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{types.AgentExplorer: {Mode: types.ToolParamCompatRepair}}}
				name := capture.Name()
				if route == "local" {
					deps.Tools = tool.NewRegistry()
					deps.Tools.Register(capture)
				} else {
					deps.MCPServers = mcp.NewRegistry()
					deps.MCPServers.Register(server)
					name = "capture_mcp__capture_mcp_params"
				}
				fake := &b1677RouteLLM{call: llm.ToolCall{ID: "b1677-public", Name: name, Params: json.RawMessage(tc.raw)}}
				deps.LLM = fake
				base := NewBaseAgent(types.AgentExplorer, deps, &stubEvaluator{})
				out, err := base.Execute(&types.AgentContext{Stage: types.StageExplore, Mutable: types.NewMutableState("public argument route")}, &skill.Config{ToolSuggestions: []string{name}})
				if err != nil || out == nil || fake.calls != 1 || !fake.offered {
					t.Fatalf("invalid public route fixture: err=%v calls=%d offered=%t out=%+v", err, fake.calls, fake.offered, out)
				}
				calls, got := capture.calls, capture.got
				if route == "mcp" {
					calls, got = server.calls, server.got
				}
				if tc.ambiguous {
					if calls != 0 || len(out.ToolResults) != 1 || out.ToolResults[0].Success {
						t.Fatalf("public loop executed competing arguments: calls=%d raw=%s out=%+v", calls, got, out)
					}
					return
				}
				var args struct {
					Pattern string `json:"pattern"`
				}
				if err := json.Unmarshal(got, &args); err != nil || calls != 1 || args.Pattern != "kept" {
					t.Fatalf("valid payload was blocked or changed: err=%v calls=%d raw=%s", err, calls, got)
				}
			})
		}
	}
}

func TestB1677RoutesPublicExecuteSchemaOwnsArguments(t *testing.T) {
	raw := json.RawMessage(`{"arguments":{"pattern":"native business value"}}`)
	capture := &b1677RouteTool{name: "capture_native_arguments", schema: json.RawMessage(`{"type":"object","properties":{"arguments":{"type":"object","properties":{"pattern":{"type":"string"}}}},"required":["arguments"]}`)}
	registry := tool.NewRegistry()
	registry.Register(capture)
	fake := &b1677RouteLLM{call: llm.ToolCall{ID: "b1677-native", Name: capture.Name(), Params: raw}}
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{LLM: fake, Tools: registry, MaxIterations: 1,
		ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{types.AgentExplorer: {Mode: types.ToolParamCompatRepair}}}, &stubEvaluator{})
	out, err := base.Execute(&types.AgentContext{Stage: types.StageExplore, Mutable: types.NewMutableState("native arguments")}, &skill.Config{ToolSuggestions: []string{capture.Name()}})
	if err != nil || out == nil || fake.calls != 1 || !fake.offered || capture.calls != 1 || string(capture.got) != string(raw) {
		t.Fatalf("schema-owned arguments were mistaken for a wrapper: err=%v calls=%d raw=%s out=%+v", err, capture.calls, capture.got, out)
	}
}
