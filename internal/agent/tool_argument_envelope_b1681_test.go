package agent

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/mcp"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type b1681NestedMCP struct{ b1677RouteMCP }

func (s *b1681NestedMCP) ListTools() []mcp.ToolSchema {
	return []mcp.ToolSchema{{Name: "capture_mcp_params", Parameters: json.RawMessage(`{"type":"object","properties":{"target":{"type":"object","properties":{"path":{"type":"string"}}}}}`)}}
}

func TestB1681NestedEnvelopeMCPBoundary(t *testing.T) {
	for _, mode := range []string{types.ToolParamCompatOff, types.ToolParamCompatAudit, types.ToolParamCompatRepair} {
		for _, ambiguous := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/ambiguous=%t", mode, ambiguous), func(t *testing.T) {
				server := &b1681NestedMCP{}
				registry := mcp.NewRegistry()
				registry.Register(server)
				base := NewBaseAgent(types.AgentExplorer, &Dependencies{MCPServers: registry,
					ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{types.AgentExplorer: {Mode: mode}}}, nil)
				raw := json.RawMessage(`{"target":{"path":"kept.go"}}`)
				if ambiguous {
					raw = json.RawMessage(`{"target":{"arguments":{"path":"first.go"},"arguments":{"path":"second.go"}}}`)
				}
				call := llm.ToolCall{ID: "b1681-mcp", Name: "capture_mcp__capture_mcp_params", Params: raw,
					ParamSchemaFingerprint: toolParamSchemaFingerprint(server.ListTools()[0].Parameters)}
				res, response := base.executeTool(&types.AgentContext{Stage: types.StageExplore}, call)
				if ambiguous {
					if server.calls != 0 || res == nil || res.Success || response != nil {
						t.Fatalf("nested conflict reached MCP: calls=%d result=%+v response=%+v", server.calls, res, response)
					}
				} else if server.calls != 1 || response == nil || !response.Success || string(server.got) != string(raw) {
					t.Fatalf("native MCP arguments changed: calls=%d got=%s response=%+v", server.calls, server.got, response)
				}
			})
		}
	}
}

func TestB1681PublicAgentNestedEnvelopeIdentity(t *testing.T) {
	for _, mode := range []string{types.ToolParamCompatOff, types.ToolParamCompatAudit, types.ToolParamCompatRepair} {
		for _, tc := range []struct {
			name, raw string
			ambiguous bool
		}{
			{"native", `{"requests":[{"path":"kept.go"}]}`, false},
			{"unique", `{"requests":[{"arguments":{"path":"kept.go"}}]}`, false},
			{"equivalent", `{"requests":[{"arguments":{"path":"kept.go"},"arguments": { "path":"kept.go" }}]}`, false},
			{"direct conflict", `{"requests":[{"arguments":{"path":"first.go"},"arguments":{"path":"second.go"}}]}`, true},
			{"function conflict", `{"requests":[{"function":{"arguments":{"path":"first.go"}},"function":{"arguments":{"path":"second.go"}}}]}`, true},
			{"invalid alternative", `{"requests":[{"arguments":null,"arguments":{"path":"second.go"}}]}`, true},
			{"unconsumed function", `{"requests":[{"arguments":{"path":"kept.go"},"function":{"arguments":{"path":"ignored1.go"}},"function":{"arguments":{"path":"ignored2.go"}}}]}`, false},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				capture := &b1677RouteTool{name: "capture_nested", schema: json.RawMessage(`{"type":"object","properties":{"requests":{"type":"array","items":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}},"required":["requests"]}`)}
				registry := tool.NewRegistry()
				registry.Register(capture)
				fake := &b1677RouteLLM{call: llm.ToolCall{ID: "b1681-public", Name: capture.Name(), Params: json.RawMessage(tc.raw)}}
				base := NewBaseAgent(types.AgentExplorer, &Dependencies{LLM: fake, Tools: registry, MaxIterations: 1,
					ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{types.AgentExplorer: {Mode: mode}}}, &stubEvaluator{})
				out, err := base.Execute(&types.AgentContext{Stage: types.StageExplore, Mutable: types.NewMutableState("nested argument identity")}, &skill.Config{ToolSuggestions: []string{capture.Name()}})
				if err != nil || out == nil || fake.calls != 1 || !fake.offered {
					t.Fatalf("invalid public route fixture: err=%v calls=%d offered=%t", err, fake.calls, fake.offered)
				}
				if tc.ambiguous {
					if capture.calls != 0 || len(out.ToolResults) != 1 || out.ToolResults[0].Success {
						t.Fatalf("competing nested argument reached tool: calls=%d raw=%s result=%+v", capture.calls, capture.got, out.ToolResults)
					}
					return
				}
				if mode == types.ToolParamCompatRepair || tc.name == "native" {
					var got struct {
						Requests []struct {
							Path string `json:"path"`
						} `json:"requests"`
					}
					if err := json.Unmarshal(capture.got, &got); err != nil || capture.calls != 1 || len(got.Requests) != 1 || got.Requests[0].Path != "kept.go" {
						t.Fatalf("valid nested arguments lost: err=%v calls=%d raw=%s", err, capture.calls, capture.got)
					}
				}
			})
		}
	}
}

func TestB1681NestedEnvelopeFingerprintDoesNotAuthorizeInvocation(t *testing.T) {
	for _, dynamic := range []bool{false, true} {
		t.Run(fmt.Sprint(dynamic), func(t *testing.T) {
			capture := &b1677RouteTool{name: "capture_nested", schema: json.RawMessage(`{"type":"object","properties":{"target":{"type":"object","properties":{"path":{"type":"string"}}}}}`)}
			registry := tool.NewRegistry()
			registry.Register(capture)
			base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: registry}, nil)
			raw := json.RawMessage(`{"target":{"arguments":{"path":"first.go"},"arguments":{"path":"second.go"}}}`)
			call := llm.ToolCall{ID: "b1681-fingerprint", Name: capture.Name(), Params: raw, ParamSchemaFingerprint: toolParamSchemaFingerprint(capture.Parameters())}
			if dynamic {
				call = base.normalizeToolCallParams([]llm.ToolCall{call}, []llm.ToolSchema{{Name: capture.Name(), Parameters: capture.Parameters()}})[0]
			}
			if string(call.Params) != string(raw) {
				t.Fatalf("schema compatibility erased competing values: %s", call.Params)
			}
			res, _ := base.executeTool(&types.AgentContext{Stage: types.StageExplore}, call)
			if capture.calls != 0 || res == nil || res.Success {
				t.Fatalf("fingerprint bypassed nested integrity: calls=%d res=%+v", capture.calls, res)
			}
		})
	}
}
