package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/mcp"
	"github.com/hanchaoqun/codrax/internal/skill"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type captureBusContextTool struct {
	toolpkg.ReadOnly
	toolpkg.NonEvidenceTool
	got *types.BusContext
}

func (t *captureBusContextTool) Name() string        { return "capture_bus_context" }
func (t *captureBusContextTool) Description() string { return "captures the bus context for tests" }
func (t *captureBusContextTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func (t *captureBusContextTool) Execute(ctx *types.BusContext, _ json.RawMessage) (types.ToolResult, error) {
	t.got = ctx
	return types.ToolResult{ToolName: t.Name(), Success: true}, nil
}

type captureParamsTool struct {
	toolpkg.ReadOnly
	toolpkg.NonEvidenceTool
	got json.RawMessage
}

func (t *captureParamsTool) Name() string        { return "capture_params" }
func (t *captureParamsTool) Description() string { return "captures normalized params for tests" }
func (t *captureParamsTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"sources":{"type":"array","items":{"type":"string"}},
			"top_n":{"type":"integer"},
			"include_counts":{"type":"boolean"}
		}
	}`)
}

func (t *captureParamsTool) Execute(_ *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	t.got = append(t.got[:0], params...)
	return types.ToolResult{ToolName: t.Name(), Success: true}, nil
}

type captureMCPServer struct {
	got       json.RawMessage
	resources []mcp.ResourceSchema
	prompts   []mcp.PromptSchema
}

func (s *captureMCPServer) Name() string                   { return "capture_mcp" }
func (s *captureMCPServer) Transport() types.TransportType { return types.TransportStdio }
func (s *captureMCPServer) ListTools() []mcp.ToolSchema {
	return []mcp.ToolSchema{{
		Name:        "capture_mcp_params",
		Description: "captures normalized MCP params for tests",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"include":{"type":"string"},"top_n":{"type":"integer"}}}`),
	}}
}
func (s *captureMCPServer) CallTool(name string, params json.RawMessage) (types.MCPResponse, error) {
	s.got = append(s.got[:0], params...)
	return types.MCPResponse{
		ServerName: s.Name(),
		Method:     "tools/call",
		Summary:    "captured " + name,
		Success:    true,
		Timestamp:  time.Now(),
	}, nil
}
func (s *captureMCPServer) ListResources() []mcp.ResourceSchema {
	return append([]mcp.ResourceSchema(nil), s.resources...)
}
func (s *captureMCPServer) ReadResource(uri string) (types.MCPResponse, error) {
	return types.MCPResponse{ServerName: s.Name(), Method: "resources/read", ResourceURI: uri, Summary: "resource " + uri, Success: true, Timestamp: time.Now()}, nil
}
func (s *captureMCPServer) ListPrompts() []mcp.PromptSchema {
	return append([]mcp.PromptSchema(nil), s.prompts...)
}
func (s *captureMCPServer) Close() error { return nil }

func TestExecuteTool_PropagatesStageAndAttachmentsToBusContext(t *testing.T) {
	reg := toolpkg.NewRegistry()
	capture := &captureBusContextTool{}
	reg.Register(capture)

	base := NewBaseAgent(types.AgentLogTriager, &Dependencies{Tools: reg}, nil)
	ctx := &types.AgentContext{
		AgentName:       types.AgentLogTriager,
		Stage:           types.StageLogTriage,
		RepoRoot:        t.TempDir(),
		WorkDir:         t.TempDir(),
		Branch:          "main",
		Commit:          "deadbeef",
		AttachedLog:     "panic: boom",
		AttachedHitrace: "trace: frame",
		Mutable:         types.NewMutableState("test"),
	}

	res, _ := base.executeTool(ctx, llm.ToolCall{Name: capture.Name(), Params: json.RawMessage(`{}`)})
	if res == nil || !res.Success {
		t.Fatalf("executeTool failed: %+v", res)
	}
	if capture.got == nil {
		t.Fatal("tool did not receive BusContext")
	}
	if capture.got.PipelineStage != types.StageLogTriage {
		t.Fatalf("PipelineStage = %s, want %s", capture.got.PipelineStage, types.StageLogTriage)
	}
	if capture.got.ActiveAgent != types.AgentLogTriager {
		t.Fatalf("ActiveAgent = %s, want %s", capture.got.ActiveAgent, types.AgentLogTriager)
	}
	if capture.got.AttachedLog != ctx.AttachedLog {
		t.Fatalf("AttachedLog = %q, want %q", capture.got.AttachedLog, ctx.AttachedLog)
	}
	if capture.got.AttachedHitrace != ctx.AttachedHitrace {
		t.Fatalf("AttachedHitrace = %q, want %q", capture.got.AttachedHitrace, ctx.AttachedHitrace)
	}
}

func TestExecuteTool_RepairsSharedStructuralJSONBeforeLocalToolExecution(t *testing.T) {
	reg := toolpkg.NewRegistry()
	capture := &captureParamsTool{}
	reg.Register(capture)

	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, nil)
	res, _ := base.executeTool(&types.AgentContext{Stage: types.StageExplore}, llm.ToolCall{
		ID:     "combo-corrupt-json",
		Name:   capture.Name(),
		Params: json.RawMessage(`}{"sources":["trace.systrace"],"top_n":5`),
	})
	if res == nil || !res.Success {
		t.Fatalf("executeTool should repair deterministic structural JSON before local tool execution: %+v", res)
	}
	var decoded struct {
		Sources []string `json:"sources"`
		TopN    int      `json:"top_n"`
	}
	if err := json.Unmarshal(capture.got, &decoded); err != nil {
		t.Fatalf("local tool received invalid repaired params: %v\n%s", err, capture.got)
	}
	if strings.Join(decoded.Sources, "|") != "trace.systrace" || decoded.TopN != 5 {
		t.Fatalf("unexpected repaired local params: %+v raw=%s", decoded, capture.got)
	}
}

func TestExecuteTool_RepairsSharedStructuralJSONBeforeMCPToolExecution(t *testing.T) {
	mcpReg := mcp.NewRegistry()
	capture := &captureMCPServer{}
	mcpReg.Register(capture)

	base := NewBaseAgent(types.AgentExplorer, &Dependencies{MCPServers: mcpReg}, nil)
	res, mcpResp := base.executeTool(&types.AgentContext{Stage: types.StageExplore}, llm.ToolCall{
		ID:     "mcp-combo-corrupt-json",
		Name:   "capture_mcp__capture_mcp_params",
		Params: json.RawMessage(`}{"pattern":"Choreographer","include":"*.java"`),
	})
	if res != nil {
		t.Fatalf("MCP tool should return MCP response, got local result: %+v", res)
	}
	if mcpResp == nil || !mcpResp.Success {
		t.Fatalf("executeTool should repair deterministic structural JSON before MCP tool execution: %+v", mcpResp)
	}
	var decoded struct {
		Pattern string `json:"pattern"`
		Include string `json:"include"`
	}
	if err := json.Unmarshal(capture.got, &decoded); err != nil {
		t.Fatalf("MCP tool received invalid repaired params: %v\n%s", err, capture.got)
	}
	if decoded.Pattern != "Choreographer" || decoded.Include != "*.java" {
		t.Fatalf("unexpected repaired MCP params: %+v raw=%s", decoded, capture.got)
	}
}

func TestExecuteTool_AppliesSchemaAwareParamCompatToMCPTool(t *testing.T) {
	mcpReg := mcp.NewRegistry()
	capture := &captureMCPServer{}
	mcpReg.Register(capture)

	base := NewBaseAgent(types.AgentExplorer, &Dependencies{
		MCPServers: mcpReg,
		ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{
			types.AgentExplorer: {Mode: types.ToolParamCompatRepair},
		},
	}, nil)
	res, mcpResp := base.executeTool(&types.AgentContext{Stage: types.StageExplore}, llm.ToolCall{
		ID:     "mcp-schema-compat-json",
		Name:   "capture_mcp__capture_mcp_params",
		Params: json.RawMessage(`{"pattern":"Choreographer","topN":"3"}`),
	})
	if res != nil {
		t.Fatalf("MCP tool should return MCP response, got local result: %+v", res)
	}
	if mcpResp == nil || !mcpResp.Success {
		t.Fatalf("executeTool should repair schema-compatible MCP params: %+v", mcpResp)
	}
	var decoded struct {
		Pattern string `json:"pattern"`
		TopN    int    `json:"top_n"`
	}
	if err := json.Unmarshal(capture.got, &decoded); err != nil {
		t.Fatalf("MCP tool received invalid normalized params: %v\n%s", err, capture.got)
	}
	if decoded.Pattern != "Choreographer" || decoded.TopN != 3 {
		t.Fatalf("unexpected normalized MCP params: %+v raw=%s", decoded, capture.got)
	}
}

func TestBuildToolSchemas_ExposesMCPOnlyToExplorerFamily(t *testing.T) {
	mcpReg := mcp.NewRegistry()
	mcpReg.Register(&captureMCPServer{})
	sk := &skill.Config{}

	explorer := NewBaseAgent(types.AgentExplorer, &Dependencies{MCPServers: mcpReg}, nil)
	explorerSchemas := explorer.buildToolSchemas(sk, &types.AgentContext{Stage: types.StageExplore})
	if !schemaNamesContain(explorerSchemas, "capture_mcp__capture_mcp_params") {
		t.Fatalf("explorer should see namespaced MCP tool, got %+v", schemaNames(explorerSchemas))
	}

	extractor := NewBaseAgent(types.AgentExtractor, &Dependencies{MCPServers: mcpReg}, nil)
	extractorSchemas := extractor.buildToolSchemas(sk, &types.AgentContext{Stage: types.StageExtract})
	if schemaNamesContain(extractorSchemas, "capture_mcp__capture_mcp_params") {
		t.Fatalf("extractor must not see MCP tools, got %+v", schemaNames(extractorSchemas))
	}
}

func TestMCPReadResourceToolReturnsMCPResponse(t *testing.T) {
	mcpReg := mcp.NewRegistry()
	capture := &captureMCPServer{
		resources: []mcp.ResourceSchema{{URI: "mcp://docs/spec", Name: "spec", Description: "test spec"}},
		prompts:   []mcp.PromptSchema{{Name: "triage", Description: "test prompt"}},
	}
	mcpReg.Register(capture)

	base := NewBaseAgent(types.AgentExplorer, &Dependencies{MCPServers: mcpReg}, nil)
	schemas := base.buildToolSchemas(&skill.Config{}, &types.AgentContext{Stage: types.StageExplore})
	if !schemaNamesContain(schemas, "mcp_read_resource") {
		t.Fatalf("explorer should see mcp_read_resource when resources exist, got %+v", schemaNames(schemas))
	}
	res, mcpResp := base.executeTool(&types.AgentContext{Stage: types.StageExplore}, llm.ToolCall{
		ID:     "mcp-read-resource",
		Name:   "mcp_read_resource",
		Params: json.RawMessage(`{"uri":"mcp://docs/spec"}`),
	})
	if res != nil {
		t.Fatalf("mcp_read_resource should return MCP response, got local result: %+v", res)
	}
	if mcpResp == nil || !mcpResp.Success || mcpResp.ResourceURI != "mcp://docs/spec" {
		t.Fatalf("unexpected MCP resource response: %+v", mcpResp)
	}
	messages := base.buildInitialMessages(&types.AgentContext{Stage: types.StageExplore}, &skill.Config{Name: "explore-skill"})
	var joined strings.Builder
	for _, msg := range messages {
		joined.WriteString(msg.Content)
	}
	if !strings.Contains(joined.String(), "External Guidance (MCP)") ||
		!strings.Contains(joined.String(), "mcp://docs/spec") ||
		!strings.Contains(joined.String(), "capture_mcp.triage") {
		t.Fatalf("MCP external guidance missing resource/prompt details:\n%s", joined.String())
	}
}

func schemaNamesContain(schemas []llm.ToolSchema, name string) bool {
	for _, schema := range schemas {
		if schema.Name == name {
			return true
		}
	}
	return false
}

func schemaNames(schemas []llm.ToolSchema) []string {
	out := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		out = append(out, schema.Name)
	}
	return out
}

func TestExecuteTool_AppliesSchemaAwareParamCompatFromRegistry(t *testing.T) {
	reg := toolpkg.NewRegistry()
	capture := &captureParamsTool{}
	reg.Register(capture)

	base := NewBaseAgent(types.AgentExplorer, &Dependencies{
		Tools: reg,
		ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{
			types.AgentExplorer: {Mode: types.ToolParamCompatRepair},
		},
	}, nil)
	res, _ := base.executeTool(&types.AgentContext{Stage: types.StageExplore}, llm.ToolCall{
		ID:     "compat-json",
		Name:   capture.Name(),
		Params: json.RawMessage(`{"sources":"Explorer","topN":"3","includeCounts":"true"}`),
	})
	if res == nil || !res.Success {
		t.Fatalf("executeTool failed: %+v", res)
	}
	var decoded struct {
		Sources       []string `json:"sources"`
		TopN          int      `json:"top_n"`
		IncludeCounts bool     `json:"include_counts"`
	}
	if err := json.Unmarshal(capture.got, &decoded); err != nil {
		t.Fatalf("tool received invalid normalized params: %v\n%s", err, capture.got)
	}
	if strings.Join(decoded.Sources, "|") != "Explorer" || decoded.TopN != 3 || !decoded.IncludeCounts {
		t.Fatalf("unexpected normalized params: %+v raw=%s", decoded, capture.got)
	}
}

func TestExecuteTool_NormalizesGrepRuntimeParamsFromRegistry(t *testing.T) {
	root := t.TempDir()
	tracePath := filepath.Join(root, "trace.systrace")
	if err := os.WriteFile(tracePath, []byte(strings.Join([]string{
		"outside [GT]Thread#1",
		"2942.124416: [GT]Thread#1 wakeup",
		"outside [GT]Thread#1 again",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write trace fixture: %v", err)
	}

	reg := toolpkg.NewRegistry()
	reg.Register(&toolpkg.GrepTool{})
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{
		Tools: reg,
		ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{
			types.AgentExplorer: {Mode: types.ToolParamCompatRepair},
		},
	}, nil)
	raw, err := json.Marshal(map[string]any{
		"pattern":     "[GT]Thread#1",
		"path":        tracePath,
		"fixedString": "true",
		"lineStart":   "2",
		"lineEnd":     "2",
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	res, _ := base.executeTool(&types.AgentContext{Stage: types.StageExplore}, llm.ToolCall{
		ID:     "grep-runtime-compat",
		Name:   "grep",
		Params: raw,
	})
	if res == nil || !res.Success {
		t.Fatalf("grep should execute after schema-aware repair: %+v", res)
	}
	for _, want := range []string{"fixed_string=true", "line_start=2", "line_end=2", "2942.124416"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("grep summary missing %q:\n%s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "outside [GT]Thread#1") {
		t.Fatalf("line window should limit grep to line 2 only:\n%s", res.Summary)
	}
}

func TestExecuteTool_MalformedParamsRejectedBeforeToolExecution(t *testing.T) {
	reg := toolpkg.NewRegistry()
	capture := &captureBusContextTool{}
	reg.Register(capture)

	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, nil)
	res, _ := base.executeTool(&types.AgentContext{Stage: types.StageExplore}, llm.ToolCall{
		ID:     "bad-json",
		Name:   capture.Name(),
		Params: json.RawMessage(`}`),
	})
	if res == nil {
		t.Fatal("expected failed ToolResult")
	}
	if res.Success {
		t.Fatalf("malformed params should fail before execution: %+v", res)
	}
	if !strings.Contains(res.Summary, "malformed JSON tool arguments") {
		t.Fatalf("summary should explain malformed JSON, got %q", res.Summary)
	}
	if capture.got != nil {
		t.Fatalf("tool executed despite malformed params: %+v", capture.got)
	}
}

func TestExecuteTool_UnknownToolReturnsFailedResult(t *testing.T) {
	base := NewBaseAgent(types.AgentExtractor, &Dependencies{Tools: toolpkg.NewRegistry()}, nil)
	res, mcp := base.executeTool(&types.AgentContext{Stage: types.StageExtract}, llm.ToolCall{
		ID:     "unknown",
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"a.go"}`),
	})
	if mcp != nil {
		t.Fatalf("unknown local tool should not return MCP response: %+v", mcp)
	}
	if res == nil {
		t.Fatal("unknown tool should return a failed ToolResult, not disappear")
	}
	if res.Success {
		t.Fatalf("unknown tool result should fail: %+v", res)
	}
	for _, want := range []string{"tool \"read_file\" is not available", "stage extract", "emit_answer_symbol"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("unknown-tool summary missing %q: %q", want, res.Summary)
		}
	}
}

func TestExecuteTool_TruncatedParamsGetsTypedCompactGuidance(t *testing.T) {
	reg := toolpkg.NewRegistry()
	capture := &captureBusContextTool{}
	reg.Register(capture)

	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, nil)
	res, _ := base.executeTool(&types.AgentContext{Stage: types.StageExplore}, llm.ToolCall{
		ID:     "truncated-json",
		Name:   capture.Name(),
		Params: json.RawMessage(`{"items":[{"summary":"unterminated}`),
	})
	if res == nil {
		t.Fatal("expected failed ToolResult")
	}
	if res.Success {
		t.Fatalf("truncated params should fail before execution: %+v", res)
	}
	for _, want := range []string{
		"truncated JSON tool arguments",
		"smaller native JSON object",
		"preserve model-authored prose",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, res.Summary)
		}
	}
	if capture.got != nil {
		t.Fatalf("tool executed despite truncated params: %+v", capture.got)
	}
}
