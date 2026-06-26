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

func TestBuildToolSchemas_WriteExplorationSubflowDoesNotExposeWriteTools(t *testing.T) {
	mut := types.NewMutableState("write needs source exploration first")
	mut.SetWriteExplorationRequest(&types.WriteExplorationRequest{
		BatchID:        "batch-1",
		Goal:           "inspect existing tests before planning edits",
		CandidatePaths: []string{"internal/foo_test.go"},
	})
	reg := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(reg)
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, nil)
	schemas := base.buildToolSchemas(&skill.Config{
		Name:            "explore-skill",
		ToolSuggestions: []string{"repo_map", "grep", "read_file", "exec_command", "emit_evidence", "emit_investigation_complete"},
	}, &types.AgentContext{
		Stage:   types.StageExplore,
		Mutable: mut,
		Mode:    types.ModeApply,
	})

	for _, forbidden := range []string{"exec_command", "emit_change_plan", "apply_patch", "emit_test_results"} {
		if schemaNamesContain(schemas, forbidden) {
			t.Fatalf("write exploration subflow must not expose %q; got %+v", forbidden, schemaNames(schemas))
		}
	}
	for _, want := range []string{"grep", "read_file"} {
		if !schemaNamesContain(schemas, want) {
			t.Fatalf("write exploration subflow should still expose %q; got %+v", want, schemaNames(schemas))
		}
	}
}

func TestBuildToolSchemas_AnalyzerExternalRuntimeFirstExposesOnlyEmitAnalysis(t *testing.T) {
	reg := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(reg)
	base := NewBaseAgent(types.AgentAnalyzer, &Dependencies{Tools: reg}, nil)
	schemas := base.buildToolSchemas(&skill.Config{
		Name:            "analysis-skill",
		ToolSuggestions: []string{"emit_analysis", "repo_map", "grep", "list_files"},
	}, &types.AgentContext{
		Stage:           types.StageAnalyze,
		AttachedHitrace: "app-20 (20) [001] .... 11.0: sched_switch: prev_comm=app prev_pid=20 prev_state=R ==> next_comm=rival next_pid=30",
		TurnRouteHint: types.TurnRouteHint{
			Route:  "repo",
			Source: "external_tool",
		},
	})

	if !schemaNamesContain(schemas, "emit_analysis") {
		t.Fatalf("external runtime-first analyzer must still expose emit_analysis, got %+v", schemaNames(schemas))
	}
	for _, forbidden := range []string{"repo_map", "grep", "list_files"} {
		if schemaNamesContain(schemas, forbidden) {
			t.Fatalf("external runtime-first analyzer must not expose %q; got %+v", forbidden, schemaNames(schemas))
		}
	}
}

func TestExecuteTool_WriteExplorationSubflowRejectsShellCommand(t *testing.T) {
	mut := types.NewMutableState("write needs source exploration first")
	mut.SetWriteExplorationRequest(&types.WriteExplorationRequest{
		BatchID:        "batch-1",
		Goal:           "inspect existing tests before planning edits",
		CandidatePaths: []string{"internal/foo_test.go"},
	})
	reg := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(reg)
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, nil)
	result, _ := base.executeTool(&types.AgentContext{
		Stage:   types.StageExplore,
		Mutable: mut,
		Mode:    types.ModeApply,
	}, llm.ToolCall{
		Name:   "exec_command",
		Params: json.RawMessage(`{"command":"python3 -m unittest"}`),
	})
	if result == nil {
		t.Fatal("expected write exploration read-only rejection")
	}
	if result.Success {
		t.Fatalf("exec_command should be rejected in write exploration, got %+v", result)
	}
	if result.Repair == nil || result.Repair.Code != "write_exploration_read_only_tool" {
		t.Fatalf("rejection should carry typed repair metadata, got %+v", result)
	}
}

func TestValidateWriteExplorationReadOnlyToolCall_RequiresWindowForLargeReadFile(t *testing.T) {
	repo := t.TempDir()
	var b strings.Builder
	for i := 0; i < writeModeUnboundedReadFileLineLimit+2; i++ {
		b.WriteString("line\n")
	}
	if err := os.WriteFile(filepath.Join(repo, "large.py"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	mut := types.NewMutableState("write needs source exploration first")
	mut.SetWriteExplorationRequest(&types.WriteExplorationRequest{
		BatchID:        "batch-1",
		Goal:           "inspect existing tests before planning edits",
		CandidatePaths: []string{"large.py"},
	})
	ctx := &types.AgentContext{
		Stage:    types.StageExplore,
		Mode:     types.ModeApply,
		Mutable:  mut,
		RepoRoot: repo,
	}

	large := validateWriteExplorationReadOnlyToolCall(ctx, llm.ToolCall{
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"large.py"}`),
	})
	if large == nil || large.Success || large.Repair == nil || large.Repair.Code != "write_exploration_read_file_requires_window" {
		t.Fatalf("large unbounded write exploration read_file should be rejected with typed policy repair, got %+v", large)
	}
	if got := validateWriteExplorationReadOnlyToolCall(ctx, llm.ToolCall{
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"large.py","line_offset":0,"limit":160}`),
	}); got != nil {
		t.Fatalf("explicitly windowed write exploration read_file should pass policy, got %+v", got)
	}
}

func TestValidateWriteAnalyzerToolPolicy_RequiresLimitForLargeReadFile(t *testing.T) {
	repo := t.TempDir()
	var b strings.Builder
	for i := 0; i < writeModeUnboundedReadFileLineLimit+2; i++ {
		b.WriteString("line\n")
	}
	if err := os.WriteFile(filepath.Join(repo, "large.py"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{
		Stage:    types.StageWriteAnalyze,
		Mode:     types.ModeApply,
		RepoRoot: repo,
	}

	offsetOnly := validateWriteAnalyzerToolPolicy(ctx, llm.ToolCall{
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"large.py","line_offset":10}`),
	})
	if offsetOnly == nil || offsetOnly.Success || offsetOnly.Repair == nil || offsetOnly.Repair.Code != "write_analyzer_read_file_requires_window" {
		t.Fatalf("offset-only large write_analyzer read_file should be rejected with typed repair, got %+v", offsetOnly)
	}
	if got := validateWriteAnalyzerToolPolicy(ctx, llm.ToolCall{
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"large.py","line_offset":10,"limit":160}`),
	}); got != nil {
		t.Fatalf("limit-bounded write_analyzer read_file should pass policy, got %+v", got)
	}
}

func TestBuildToolSchemas_WritePlannerHidesShellAndForcesDryRunProbe(t *testing.T) {
	reg := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(reg)
	base := NewBaseAgent(types.AgentPlanner, &Dependencies{Tools: reg}, nil)
	ctx := &types.AgentContext{
		Stage: types.StagePlan,
		Mode:  types.ModeApply,
	}
	sk := &skill.Config{ToolSuggestions: []string{
		"read_file",
		"exec_command",
		"run_tests",
		"apply_patch",
		"emit_change_plan",
	}}
	schemas := base.buildToolSchemas(sk, ctx)
	if schemaNamesContain(schemas, "exec_command") {
		t.Fatalf("write planner must not expose generic exec_command: %+v", schemaNames(schemas))
	}
	if schemaNamesContain(schemas, "apply_patch") {
		t.Fatalf("write planner must not expose apply_patch: %+v", schemaNames(schemas))
	}
	var runTests *llm.ToolSchema
	for i := range schemas {
		if schemas[i].Name == "run_tests" {
			runTests = &schemas[i]
			break
		}
	}
	if runTests == nil {
		t.Fatalf("write planner should keep typed run_tests dry-run probe available: %+v", schemaNames(schemas))
	}
	raw := string(runTests.Parameters)
	for _, want := range []string{`"dry_run"`, `"verification_probe"`, `"enum":[true]`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("planner run_tests schema should require dry_run=true, missing %q in %s", want, raw)
		}
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(runTests.Parameters, &schema); err != nil {
		t.Fatalf("decode run_tests schema: %v", err)
	}
	for _, want := range []string{"dry_run", "verification_probe"} {
		if !stringSliceContains(schema.Required, want) {
			t.Fatalf("planner run_tests schema required=%v, missing %q", schema.Required, want)
		}
	}
}

func TestBuildToolSchemas_WriteVerifierForcesDefaultRunTests(t *testing.T) {
	reg := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(reg)
	base := NewBaseAgent(types.AgentVerifier, &Dependencies{Tools: reg}, nil)
	ctx := &types.AgentContext{
		Stage: types.StageVerify,
		Mode:  types.ModeApply,
	}
	sk := &skill.Config{ToolSuggestions: []string{
		"run_tests",
		"emit_test_results",
		"exec_command",
	}}
	schemas := base.buildToolSchemas(sk, ctx)
	var runTests *llm.ToolSchema
	for i := range schemas {
		if schemas[i].Name == "run_tests" {
			runTests = &schemas[i]
			break
		}
	}
	if runTests == nil {
		t.Fatalf("write verifier should expose run_tests: %+v", schemaNames(schemas))
	}
	raw := string(runTests.Parameters)
	for _, banned := range []string{`"runner"`, `"framework"`, `"suite"`, `"working_dir"`, `"timeout"`, `"dry_run"`} {
		if strings.Contains(raw, banned) {
			t.Fatalf("verifier run_tests schema must not expose %s: %s", banned, raw)
		}
	}
	var schema struct {
		Properties           map[string]json.RawMessage `json:"properties"`
		AdditionalProperties bool                       `json:"additionalProperties"`
	}
	if err := json.Unmarshal(runTests.Parameters, &schema); err != nil {
		t.Fatalf("decode run_tests schema: %v", err)
	}
	if len(schema.Properties) != 0 {
		t.Fatalf("verifier run_tests schema should expose empty properties, got %v", schema.Properties)
	}
	if schema.AdditionalProperties {
		t.Fatalf("verifier run_tests schema should reject additional properties: %s", raw)
	}
}

func TestValidateWritePlannerToolPolicy_RejectsShellAndNonDryRunTests(t *testing.T) {
	ctx := &types.AgentContext{
		Stage: types.StagePlan,
		Mode:  types.ModeApply,
	}
	shell := validateWritePlannerToolPolicy(ctx, llm.ToolCall{
		Name:   "exec_command",
		Params: json.RawMessage(`{"command":"pytest"}`),
	})
	if shell == nil || shell.Success || shell.Repair == nil || shell.Repair.Code != "write_planner_tool_not_allowed" {
		t.Fatalf("exec_command should be rejected with typed policy repair, got %+v", shell)
	}
	nonDryRun := validateWritePlannerToolPolicy(ctx, llm.ToolCall{
		Name:   "run_tests",
		Params: json.RawMessage(`{"runner":"python"}`),
	})
	if nonDryRun == nil || nonDryRun.Success || nonDryRun.Repair == nil || nonDryRun.Repair.Code != "write_planner_run_tests_requires_dry_run" {
		t.Fatalf("run_tests without dry_run should be rejected with typed policy repair, got %+v", nonDryRun)
	}
	dryRunWithoutProbe := validateWritePlannerToolPolicy(ctx, llm.ToolCall{
		Name:   "run_tests",
		Params: json.RawMessage(`{"runner":"python","dry_run":true}`),
	})
	if dryRunWithoutProbe == nil || dryRunWithoutProbe.Success || dryRunWithoutProbe.Repair == nil ||
		dryRunWithoutProbe.Repair.Code != "write_planner_run_tests_requires_verification_probe" {
		t.Fatalf("run_tests dry_run=true without verification_probe should be rejected, got %+v", dryRunWithoutProbe)
	}
	if got := validateWritePlannerToolPolicy(ctx, llm.ToolCall{
		Name:   "run_tests",
		Params: json.RawMessage(`{"dry_run":true,"verification_probe":{"code":"assert True"}}`),
	}); got != nil {
		t.Fatalf("run_tests dry_run=true should pass write planner policy, got %+v", got)
	}
}

func TestValidateWriteVerifierToolPolicy_RejectsExplicitRunTestsTargets(t *testing.T) {
	ctx := &types.AgentContext{
		Stage: types.StageVerify,
		Mode:  types.ModeApply,
	}
	for name, params := range map[string]json.RawMessage{
		"nil":        nil,
		"empty":      json.RawMessage(``),
		"null":       json.RawMessage(`null`),
		"empty_obj":  json.RawMessage(`{}`),
		"spaced_obj": json.RawMessage(` { } `),
	} {
		if got := validateWriteVerifierToolPolicy(ctx, llm.ToolCall{Name: "run_tests", Params: params}); got != nil {
			t.Fatalf("%s default params should pass write verifier policy, got %+v", name, got)
		}
	}
	for name, params := range map[string]json.RawMessage{
		"runner":      json.RawMessage(`{"runner":"python"}`),
		"framework":   json.RawMessage(`{"framework":"pytest"}`),
		"suite":       json.RawMessage(`{"suite":"tests/test_foo.py"}`),
		"working_dir": json.RawMessage(`{"working_dir":"."}`),
		"timeout":     json.RawMessage(`{"timeout":300}`),
		"dry_run":     json.RawMessage(`{"dry_run":true}`),
	} {
		got := validateWriteVerifierToolPolicy(ctx, llm.ToolCall{Name: "run_tests", Params: params})
		if got == nil || got.Success || got.Repair == nil || got.Repair.Code != "write_verifier_run_tests_default_required" {
			t.Fatalf("%s params should be rejected with typed repair, got %+v", name, got)
		}
		if got.Repair.Metadata["policy"] != "write_verifier_default_run_tests_only" {
			t.Fatalf("%s repair policy metadata mismatch: %+v", name, got.Repair.Metadata)
		}
	}
	if got := validateWriteVerifierToolPolicy(&types.AgentContext{Stage: types.StageVerify, Mode: types.ModeRead}, llm.ToolCall{
		Name:   "run_tests",
		Params: json.RawMessage(`{"runner":"python"}`),
	}); got != nil {
		t.Fatalf("non-write verifier should not use write verifier policy, got %+v", got)
	}
	if got := validateWriteVerifierToolPolicy(ctx, llm.ToolCall{
		Name:   "emit_test_results",
		Params: json.RawMessage(`{"failure_summary":"x"}`),
	}); got != nil {
		t.Fatalf("non-run_tests verifier tool should not use run_tests policy, got %+v", got)
	}
}

func TestValidateWritePlannerToolPolicy_RequiresWindowForLargeReadFile(t *testing.T) {
	repo := t.TempDir()
	var b strings.Builder
	for i := 0; i < writeModeUnboundedReadFileLineLimit+2; i++ {
		b.WriteString("line\n")
	}
	if err := os.WriteFile(filepath.Join(repo, "large.py"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "small.py"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{
		Stage:    types.StagePlan,
		Mode:     types.ModeApply,
		RepoRoot: repo,
	}

	large := validateWritePlannerToolPolicy(ctx, llm.ToolCall{
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"large.py"}`),
	})
	if large == nil || large.Success || large.Repair == nil || large.Repair.Code != "write_planner_read_file_requires_window" {
		t.Fatalf("large unbounded read_file should be rejected with typed policy repair, got %+v", large)
	}
	if large.Repair.Metadata["line_limit"] == "" || large.Repair.Metadata["suggested_limit"] == "" {
		t.Fatalf("large read repair should carry policy limits, got %+v", large.Repair.Metadata)
	}

	if got := validateWritePlannerToolPolicy(ctx, llm.ToolCall{
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"large.py","line_offset":0,"limit":160}`),
	}); got != nil {
		t.Fatalf("explicitly windowed read_file should pass write planner policy, got %+v", got)
	}
	if got := validateWritePlannerToolPolicy(ctx, llm.ToolCall{
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"large.py","line_offset":160}`),
	}); got == nil || got.Success || got.Repair == nil || got.Repair.Code != "write_planner_read_file_requires_window" {
		t.Fatalf("offset-only read_file should be rejected for large files, got %+v", got)
	}
	if got := validateWritePlannerToolPolicy(ctx, llm.ToolCall{
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"small.py"}`),
	}); got != nil {
		t.Fatalf("small unbounded read_file should pass write planner policy, got %+v", got)
	}
}

func TestValidateExternalObservationOnlyToolCall_DoesNotBlockMCPResourceSourceTools(t *testing.T) {
	ctx := &types.AgentContext{
		Stage: types.StageExplore,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: "请使用 MCP fixture 工具查询 mcp://fixture/trace/sleep-wakeup 的 line 7 和 line 12",
		}},
	}
	if got := validateExternalObservationOnlyToolCall(ctx, llm.ToolCall{Name: "read_file", Params: json.RawMessage(`{"path":"internal/tracequery/types.go"}`)}); got != nil {
		t.Fatalf("MCP resource references must not hard-block current-source tools by default: %+v", got)
	}
}

func TestValidateExplorerTraceQueryFirstToolCall_BlocksNonTraceToolsBeforeAttempt(t *testing.T) {
	ctx := traceQueryFirstTestContext(traceQueryFirstRuntimeRequestModel(), types.NewMutableState("trace state churn"))
	for _, toolName := range []string{"read_file", "emit_investigation_complete"} {
		got := validateExplorerTraceQueryFirstToolCall(ctx, llm.ToolCall{
			Name:   toolName,
			Params: json.RawMessage(`{}`),
		}, true)
		if got == nil {
			t.Fatalf("expected %s to be rejected before trace_query", toolName)
		}
		if got.Success {
			t.Fatalf("%s rejection should be failed, got %+v", toolName, got)
		}
		if got.Repair == nil || got.Repair.Code != explorerTraceQueryFirstCode {
			t.Fatalf("%s rejection should carry trace-query-first repair, got %+v", toolName, got)
		}
	}
}

func TestValidateExplorerTraceQueryFirstToolCall_BlocksSuffixlessTracePathBeforeAttempt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture")
	body := "# tracer: nop\napp-1 (1) [000] .... 1.000000: sched_switch: prev_comm=app prev_pid=1 prev_state=S ==> next_comm=idle next_pid=0\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{
		Stage:     types.StageExplore,
		Objective: "只分析 ./capture 这个文件里的调度问题，不分析代码",
		RepoRoot:  dir,
		Mutable:   types.NewMutableState("suffixless trace"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: "只分析 ./capture 这个文件里的调度问题，不分析代码",
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
				CurrentSourceMode:    types.ExternalObservationCurrentSourceExclude,
				ExclusionKind:        types.ExternalObservationSourceExclusionExplicitUserBoundary,
				SourceQuotes:         []string{"不分析代码"},
				Confidence:           0.9,
			},
		}},
	}
	if !traceQueryToolAvailable(ctx) {
		t.Fatal("suffixless trace-like explicit path should expose trace_query")
	}
	rm := ctx.AnalysisIR.RequestModel
	if !explorerHasRuntimeTraceArtifact(ctx, &rm) {
		t.Fatal("suffixless trace-like explicit path should count as runtime trace artifact")
	}
	if got := rm.CurrentSourceLaneDecision(); got.RequiresCurrentSource() {
		t.Fatalf("suffixless trace external-only request should not require current source, got %s", got)
	}
	if !explorerTraceQueryFirstRequired(ctx, true) {
		t.Fatal("suffixless trace-like explicit path should activate trace-query-first guard")
	}
	got := validateExplorerTraceQueryFirstToolCall(ctx, llm.ToolCall{
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"capture"}`),
	}, true)
	if got == nil {
		t.Fatal("suffixless trace-like explicit path should require trace_query before read_file")
	}
	if got.Repair == nil || got.Repair.Code != explorerTraceQueryFirstCode {
		t.Fatalf("repair code = %+v, want %q", got.Repair, explorerTraceQueryFirstCode)
	}
}

func TestValidateExplorerTraceQueryFirstToolCall_AllowsFallbackAfterTraceAttempt(t *testing.T) {
	mut := types.NewMutableState("trace state churn")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  false,
		Summary:  "trace_query returned unsupported for this trace flavor",
	})
	ctx := traceQueryFirstTestContext(traceQueryFirstRuntimeRequestModel(), mut)
	got := validateExplorerTraceQueryFirstToolCall(ctx, llm.ToolCall{
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"internal/tracequery/types.go"}`),
	}, true)
	if got != nil {
		t.Fatalf("source fallback should be allowed after a trace_query attempt, got %+v", got)
	}
}

func TestValidateExplorerTraceQueryFirstToolCall_BlocksSourceFallbackAfterRuntimeObservations(t *testing.T) {
	mut := types.NewMutableState("trace state churn")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Summary:  "root_cause_rank returned structured runtime rows",
		Observations: []types.ObservationRecord{{
			ID:              "trace_query:attached#root_cause_rank:1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace"},
			ClaimKey:        "root_cause_primary",
		}},
	})
	ctx := traceQueryFirstTestContext(traceQueryFirstRuntimeRequestModel(), mut)
	for _, toolName := range []string{"read_file", "grep", "repo_map", "list_files", "exec_command"} {
		got := validateExplorerTraceQueryFirstToolCall(ctx, llm.ToolCall{
			Name:   toolName,
			Params: json.RawMessage(`{"path":"internal/tracequery/types.go"}`),
		}, true)
		if got == nil {
			t.Fatalf("%s should be rejected after trace_query published runtime observations", toolName)
		}
		if got.Repair == nil || got.Repair.Code != explorerTraceQuerySufficientRuntimeEvidenceCode {
			t.Fatalf("%s repair code = %+v, want %q", toolName, got.Repair, explorerTraceQuerySufficientRuntimeEvidenceCode)
		}
	}
	for _, toolName := range []string{"trace_query", "emit_investigation_complete"} {
		got := validateExplorerTraceQueryFirstToolCall(ctx, llm.ToolCall{
			Name:   toolName,
			Params: json.RawMessage(`{}`),
		}, true)
		if got != nil {
			t.Fatalf("%s should remain available after trace_query observations, got %+v", toolName, got)
		}
	}
	mut.ResetDispatchToolResults()
	got := validateExplorerTraceQueryFirstToolCall(ctx, llm.ToolCall{
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"internal/tracequery/types.go"}`),
	}, true)
	if got == nil || got.Repair == nil || got.Repair.Code != explorerTraceQuerySufficientRuntimeEvidenceCode {
		t.Fatalf("runtime observation count should block source fallback after dispatch reset, got %+v", got)
	}
}

func TestValidateExplorerTraceQueryFirstToolCall_AllowsSourceFallbackAfterEmptyTraceQuery(t *testing.T) {
	mut := types.NewMutableState("trace state churn")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Summary:  "trace_query returned no deterministic rows in the selected window",
	})
	ctx := traceQueryFirstTestContext(traceQueryFirstRuntimeRequestModel(), mut)
	got := validateExplorerTraceQueryFirstToolCall(ctx, llm.ToolCall{
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"internal/tracequery/types.go"}`),
	}, true)
	if got != nil {
		t.Fatalf("source fallback should remain available when trace_query produced no structured runtime observations, got %+v", got)
	}
}

func TestValidateExplorerTraceQueryFirstToolCall_AllowsCurrentSourceRequired(t *testing.T) {
	rm := traceQueryFirstRuntimeRequestModel()
	rm.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{{
		Path:       "internal/tracequery/types.go",
		Confidence: 0.95,
	}}
	ctx := traceQueryFirstTestContext(rm, types.NewMutableState("trace source verification"))
	got := validateExplorerTraceQueryFirstToolCall(ctx, llm.ToolCall{
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"internal/tracequery/types.go"}`),
	}, true)
	if got != nil {
		t.Fatalf("current-source required trace questions must keep source tools available, got %+v", got)
	}
}

func TestValidateExplorerTraceQueryFirstToolCall_IgnoresSurfaceWithoutTraceQuery(t *testing.T) {
	ctx := traceQueryFirstTestContext(traceQueryFirstRuntimeRequestModel(), types.NewMutableState("trace state churn"))
	got := validateExplorerTraceQueryFirstToolCall(ctx, llm.ToolCall{
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"internal/tracequery/types.go"}`),
	}, false)
	if got != nil {
		t.Fatalf("trace-query-first guard must not fire when trace_query is not in the current tool surface, got %+v", got)
	}
}

func traceQueryFirstRuntimeRequestModel() types.RequestModel {
	perf := &types.PerfBundle{
		Observations: []types.PerfObservation{{
			Kind:      "state_churn",
			Subject:   "app-20",
			Summary:   "runtime trace state churn metrics",
			LineStart: 3,
			LineEnd:   23,
		}},
	}
	return types.RequestModel{
		Intent:    types.IntentRootCause,
		Scenario:  types.ScenarioPerformanceBottleneck,
		PerfTrace: perf,
		ExternalObservationPolicy: &types.ExternalObservationPolicy{
			ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
			CurrentSourceMode:    types.ExternalObservationCurrentSourceDefault,
			Confidence:           0.9,
		},
	}
}

func traceQueryFirstTestContext(rm types.RequestModel, mut *types.MutableState) *types.AgentContext {
	if mut == nil {
		mut = types.NewMutableState("trace state churn")
	}
	mut.SetRequestModel(rm)
	mut.SetPerfTrace(rm.PerfTrace)
	return &types.AgentContext{
		Stage:      types.StageExplore,
		Objective:  "analyze attached runtime trace",
		PerfTrace:  rm.PerfTrace,
		Mutable:    mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: rm},
	}
}

func TestExecuteTool_AllowsRepoToolsForMixedMCPSourceQuestion(t *testing.T) {
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{}, nil)
	ctx := &types.AgentContext{
		Stage: types.StageExplore,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: "请使用 MCP fixture 工具查询 mcp://fixture/trace/sleep-wakeup，并结合源码解释实现",
			CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				Modes: []types.CurrentSourceExplanationMode{
					types.CurrentSourceExplanationExplainCurrentMechanism,
				},
				Confidence:   0.9,
				SourceQuotes: []string{"结合源码"},
			},
		}},
	}
	res, _ := base.executeTool(ctx, llm.ToolCall{
		ID:     "mixed-read-file",
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"internal/tracequery/types.go"}`),
	})
	if res != nil && res.Repair != nil && strings.Contains(res.Repair.Code, "observation_only") {
		t.Fatalf("MCP+source question must not use observation-only rejection: %+v", res)
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

func TestExecuteTool_SkipsDuplicateRegistryNormalizeWhenSchemaChecked(t *testing.T) {
	reg := toolpkg.NewRegistry()
	capture := &captureParamsTool{}
	reg.Register(capture)

	base := NewBaseAgent(types.AgentExplorer, &Dependencies{
		Tools: reg,
		ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{
			types.AgentExplorer: {Mode: types.ToolParamCompatRepair},
		},
	}, nil)
	raw := json.RawMessage(`{"sources":"Explorer","topN":"3","includeCounts":"true"}`)
	res, _ := base.executeTool(&types.AgentContext{Stage: types.StageExplore}, llm.ToolCall{
		ID:                     "compat-json",
		Name:                   capture.Name(),
		Params:                 raw,
		ParamSchemaFingerprint: toolParamSchemaFingerprint(capture.Parameters()),
	})
	if res == nil || !res.Success {
		t.Fatalf("executeTool failed: %+v", res)
	}
	if string(capture.got) != string(raw) {
		t.Fatalf("schema-checked call should skip duplicate normalize, got %s want %s", capture.got, raw)
	}
}

func TestExecuteTool_DifferentSchemaFingerprintStillNormalizesFromRegistry(t *testing.T) {
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
		ID:                     "compat-json",
		Name:                   capture.Name(),
		Params:                 json.RawMessage(`{"sources":"Explorer","topN":"3","includeCounts":"true"}`),
		ParamSchemaFingerprint: toolParamSchemaFingerprint(json.RawMessage(`{"type":"object","properties":{}}`)),
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
		t.Fatalf("registry schema should still normalize on fingerprint mismatch: %+v raw=%s", decoded, capture.got)
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
	if res.Repair == nil || res.Repair.Code != toolRepairCodeMalformedToolParams {
		t.Fatalf("malformed params should publish typed repair, got %+v", res.Repair)
	}
	if res.Refinement == nil || res.Refinement.ReasonCode != toolRepairCodeMalformedToolParams {
		t.Fatalf("malformed params should publish typed refinement, got %+v", res.Refinement)
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

func TestExecuteTool_MarkupSentinelParamsRejectedBeforeExecution(t *testing.T) {
	reg := toolpkg.NewRegistry()
	capture := &captureParamsTool{}
	reg.Register(capture)

	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, nil)
	res, _ := base.executeTool(&types.AgentContext{Stage: types.StageExplore}, llm.ToolCall{
		ID:     "polluted-json",
		Name:   capture.Name(),
		Params: json.RawMessage(`{"sources":["src/main.cpp</parameter"]}`),
	})
	if res == nil {
		t.Fatal("expected failed ToolResult")
	}
	if res.Success {
		t.Fatalf("markup-polluted params should fail before execution: %+v", res)
	}
	for _, want := range []string{
		"contain </parameter",
		"were not executed",
		"single native JSON object",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, res.Summary)
		}
	}
	if res.Refinement == nil || res.Refinement.ReasonCode != "tool_args_markup_sentinel" {
		t.Fatalf("expected typed refinement for markup sentinel, got %+v", res.Refinement)
	}
	if capture.got != nil {
		t.Fatalf("tool executed despite markup-polluted params: %s", capture.got)
	}
}

func TestExecuteTool_HTMLClosingTagStringIsNotRejectedAsMarkupSentinel(t *testing.T) {
	reg := toolpkg.NewRegistry()
	capture := &captureParamsTool{}
	reg.Register(capture)

	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, nil)
	res, _ := base.executeTool(&types.AgentContext{Stage: types.StageExplore}, llm.ToolCall{
		ID:     "html-json",
		Name:   capture.Name(),
		Params: json.RawMessage(`{"sources":["templates/card.html","</div>"]}`),
	})
	if res == nil || !res.Success {
		t.Fatalf("ordinary HTML closing tags should not be rejected as tool-call markup: %+v", res)
	}
	if capture.got == nil {
		t.Fatal("tool did not execute for ordinary HTML string params")
	}
}

func TestExecuteTool_StandaloneParameterClosingTagStringIsNotRejected(t *testing.T) {
	reg := toolpkg.NewRegistry()
	capture := &captureParamsTool{}
	reg.Register(capture)

	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, nil)
	res, _ := base.executeTool(&types.AgentContext{Stage: types.StageExplore}, llm.ToolCall{
		ID:     "xml-json",
		Name:   capture.Name(),
		Params: json.RawMessage(`{"sources":["</parameter>"]}`),
	})
	if res == nil || !res.Success {
		t.Fatalf("standalone XML/tool-like closing tag search text should not be rejected: %+v", res)
	}
	if capture.got == nil {
		t.Fatal("tool did not execute for standalone closing-tag string params")
	}
}
