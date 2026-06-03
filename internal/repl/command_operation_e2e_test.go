package repl

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/mcp"
	"github.com/hanchaoqun/codrax/internal/operation"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

type operationProviderMCPServer struct {
	got json.RawMessage
}

func TestOperationProviderLazyMCPFakeServerHelper(t *testing.T) {
	if os.Getenv("CODRAX_REPL_FAKE_MCP_SERVER") != "1" {
		return
	}
	defer os.Exit(0)
	type rpcRequest struct {
		ID     any             `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params,omitempty"`
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var req rpcRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		switch req.Method {
		case "initialize":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"protocolVersion": "2025-03-26",
					"capabilities":    map[string]any{},
					"serverInfo":      map[string]string{"name": "lazy-slides", "version": "test"},
				},
			})
		case "notifications/initialized":
			continue
		case "tools/list":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"tools": []map[string]any{{
						"name":        "run_operation",
						"description": "runs a lazy operation",
						"inputSchema": map[string]any{"type": "object"},
					}},
				},
			})
		case "resources/list":
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"resources": []any{}}})
		case "prompts/list":
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"prompts": []any{}}})
		case "tools/call":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"content": []map[string]string{{"type": "text", "text": "lazy provider created deck artifact"}},
					"isError": false,
				},
			})
		}
	}
}

func TestOperationProviderLocalSkillFakeHelper(t *testing.T) {
	if os.Getenv("CODRAX_REPL_FAKE_LOCAL_SKILL") != "1" {
		return
	}
	defer os.Exit(0)
	if os.Getenv("CODRAX_REPL_FAKE_LOCAL_SKILL_LARGE") == "1" {
		_, _ = os.Stdout.WriteString(strings.Repeat("x", 2048))
		return
	}
	var payload map[string]any
	_ = json.NewDecoder(os.Stdin).Decode(&payload)
	if os.Getenv("CODRAX_REPL_FAKE_LOCAL_SKILL_CHAIN_MULTI") == "1" {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":       true,
			"summary":       "manual reader extracted two deck sections",
			"artifact_refs": []string{"out/manual-notes.md"},
			"next_actions": []map[string]any{
				{
					"provider":              "skill:ppt_builder",
					"operation_kind":        "presentation_generation",
					"target_surface":        "slides",
					"risk_level":            "medium",
					"side_effects":          []string{"local_file_write"},
					"requires_confirmation": true,
					"request":               "build main slides",
					"input":                 map[string]any{"source_payload_ref": "out/manual-notes.md"},
				},
				{
					"provider":              "skill:ppt_builder",
					"operation_kind":        "presentation_generation",
					"target_surface":        "slides",
					"risk_level":            "medium",
					"side_effects":          []string{"local_file_write"},
					"requires_confirmation": true,
					"request":               "build appendix slides",
					"input":                 map[string]any{"source_payload_ref": "out/appendix-notes.md"},
				},
			},
			"workflow_state": map[string]any{
				"workflow_id": "wf-manual-deck",
				"step":        "manual_extracted",
				"return_to":   "skill:manual_reader",
			},
		})
		return
	}
	if os.Getenv("CODRAX_REPL_FAKE_LOCAL_SKILL_CHAIN_A") == "1" {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":       true,
			"summary":       "manual reader extracted deck notes",
			"artifact_refs": []string{"out/manual-notes.md"},
			"next_actions": []map[string]any{{
				"provider":              "skill:ppt_builder",
				"operation_kind":        "presentation_generation",
				"target_surface":        "slides",
				"risk_level":            "medium",
				"side_effects":          []string{"local_file_write"},
				"requires_confirmation": true,
				"request":               "build slides from extracted notes",
				"input":                 map[string]any{"source_payload_ref": "out/manual-notes.md"},
			}},
			"workflow_state": map[string]any{
				"workflow_id": "wf-manual-deck",
				"step":        "manual_extracted",
				"return_to":   "skill:manual_reader",
				"data":        map[string]any{"source_payload_ref": "out/manual-notes.md"},
			},
		})
		return
	}
	if os.Getenv("CODRAX_REPL_FAKE_LOCAL_SKILL_CHAIN_B") == "1" {
		input, _ := payload["input"].(map[string]any)
		source, _ := input["source_payload_ref"].(string)
		workflowDepth, _ := payload["workflow_depth"].(float64)
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":              true,
			"summary":              "ppt builder consumed " + source,
			"artifact_refs":        []string{"out/deck.pptx"},
			"verification_status":  "verified",
			"verification_summary": "workflow depth accepted",
			"observations":         int(workflowDepth),
		})
		return
	}
	kind, _ := payload["operation_kind"].(string)
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"success":              true,
		"summary":              "local skill created deck artifact for " + kind,
		"artifact_refs":        []string{"/tmp/codrax/local-skill-deck.pptx"},
		"verification_status":  "verified",
		"verification_summary": "fixture render passed",
		"observations":         []string{"outline", "render"},
	})
}

func (s *operationProviderMCPServer) Name() string                   { return "slides" }
func (s *operationProviderMCPServer) Transport() types.TransportType { return types.TransportStdio }
func (s *operationProviderMCPServer) ListTools() []mcp.ToolSchema {
	return []mcp.ToolSchema{{Name: "run_operation"}}
}
func (s *operationProviderMCPServer) ListResources() []mcp.ResourceSchema { return nil }
func (s *operationProviderMCPServer) ReadResource(string) (types.MCPResponse, error) {
	return types.MCPResponse{}, nil
}
func (s *operationProviderMCPServer) ListPrompts() []mcp.PromptSchema { return nil }
func (s *operationProviderMCPServer) CallTool(name string, params json.RawMessage) (types.MCPResponse, error) {
	s.got = append([]byte(nil), params...)
	return types.MCPResponse{ServerName: s.Name(), Method: "tools/call", Summary: "created deck artifact", PayloadRef: "/tmp/codrax/deck.pptx", Success: true, Timestamp: time.Now()}, nil
}
func (s *operationProviderMCPServer) Close() error { return nil }

func commandOperationPolicy(risk string) TurnPolicy {
	return TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "computer_operation",
		OperationKind:        "computer_operation",
		Source:               "current_message",
		RiskLevel:            risk,
		TargetSurface:        "desktop",
		Confidence:           0.9,
		Reason:               "user asked for a computer operation",
	}
}

func TestCommandOperationE2E_OperationMemoryFeedsPlannerOnlyOnOperationRoute(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"use remembered command","program":"demo-tool","args":["--input","a.txt"],"risk_level":"low","side_effects":[]}]}`)},
	}
	r, runner, _ := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	r.runtimeAnchor = t.TempDir()
	r.operationMemory = operation.NewMemoryStore(filepath.Join(r.runtimeAnchor, "operation", "memory.jsonl"))
	if err := r.operationMemory.Append(operation.MemoryEntry{
		Workspace:  r.commandOperationCapabilitySnapshot().RepoRoot,
		OS:         r.commandOperationCapabilitySnapshot().OS,
		Arch:       r.commandOperationCapabilitySnapshot().Arch,
		Capability: "computer_operation",
		Command:    "demo-tool --input a.txt",
		Outcome:    "executed",
		Lessons:    []string{"demo-tool worked with --input"},
	}); err != nil {
		t.Fatalf("append operation memory: %v", err)
	}

	r.operationDispatch("使用 demo-tool 处理 a.txt", "使用 demo-tool 处理 a.txt", commandOperationPolicy("low"))

	if len(runner.requests) != 0 {
		t.Fatalf("operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(adapter.calls) != 1 {
		t.Fatalf("planner calls=%d want 1", len(adapter.calls))
	}
	all := ""
	for _, msg := range adapter.calls[0].messages {
		all += "\n" + msg.Content
	}
	for _, want := range []string{
		"## operation_memory",
		"demo-tool worked with --input",
		"not source evidence",
	} {
		if !strings.Contains(all, want) {
			t.Fatalf("operation memory prompt missing %q:\n%s", want, all)
		}
	}
}

func TestCommandOperationE2E_NeedsClarificationDoesNotEnterSourcePipeline(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("medium")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{commandOperationPlanResp(`{"status":"needs_clarification","risk_level":"medium","requires_confirmation":true,"questions":[{"id":"paths","question":"Which source and destination paths should be used?","suggestions":["provide the source path","provide the destination path"]}]}`)},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "帮我移动一个文件\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("clarification should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingOperation != nil {
		t.Fatalf("clarification should not create a pending operation: %+v", r.pendingOperation)
	}
	printed := out.String()
	if !strings.Contains(printed, "Which source and destination paths should be used?") {
		t.Fatalf("clarification question missing:\n%s", printed)
	}
	if strings.Contains(printed, "completed") {
		t.Fatalf("clarification must not execute anything:\n%s", printed)
	}
}

func TestCommandOperationE2E_ClarificationAnswerResumesPlanning(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("medium")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"needs_clarification","risk_level":"medium","requires_confirmation":true,"questions":[{"id":"paths","question":"Which source and destination paths should be used?","suggestions":["provide the source path","provide the destination path"]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"medium","requires_confirmation":true,"work_dir":".","steps":[{"id":"s1","title":"move file","program":"mv","args":["a.txt","b.txt"],"risk_level":"medium","side_effects":["local_file_write"],"verify_hint":"path_exists:b.txt"}]}`),
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "帮我移动一个文件\n源是 a.txt，目标是 b.txt\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("clarification resume should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("planner calls=%d want 2", len(adapter.calls))
	}
	if r.pendingCommandClarification != nil {
		t.Fatalf("clarification should be cleared after ready plan: %+v", r.pendingCommandClarification)
	}
	if r.pendingOperation == nil {
		t.Fatal("ready resumed plan should be pending approval")
	}
	if got := r.pendingOperation.Steps[0].Program; got != "mv" {
		t.Fatalf("resumed plan program=%q want mv", got)
	}
	if !strings.Contains(out.String(), "Operation plan") && !strings.Contains(out.String(), "操作计划") {
		t.Fatalf("ready operation plan not rendered:\n%s", out.String())
	}
}

func TestOperationProviderMCPApproveExecutesConfiguredTool(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "presentation_generation",
		OperationKind:        "presentation_generation",
		Source:               "current_message",
		RiskLevel:            "low",
		TargetSurface:        "slides",
		SideEffects:          []string{"local_file_write"},
		Confidence:           0.9,
		Reason:               "user asked for a presentation artifact",
	}}
	server := &operationProviderMCPServer{}
	reg := mcp.NewRegistry()
	if err := reg.Register(server); err != nil {
		t.Fatalf("register MCP server: %v", err)
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "生成一份 PPT\n/approve\n/exit\n")
	r.operationEnabled = true
	r.mcpServers = reg
	r.operationProviders = []operation.ProviderInfo{{
		Name:         "mcp:slides",
		Kind:         "presentation_generation",
		Surfaces:     []string{"slides"},
		SideEffects:  []string{"local_file_write"},
		RequiresGate: true,
		ToolName:     "run_operation",
	}}
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("provider operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(server.got) == 0 {
		t.Fatal("MCP operation tool was not called")
	}
	var payload map[string]any
	if err := json.Unmarshal(server.got, &payload); err != nil {
		t.Fatalf("operation payload is not JSON: %v\n%s", err, string(server.got))
	}
	if payload["operation_kind"] != "presentation_generation" {
		t.Fatalf("operation_kind=%v payload=%v", payload["operation_kind"], payload)
	}
	if !strings.Contains(out.String(), "created deck artifact") {
		t.Fatalf("provider result not rendered:\n%s", out.String())
	}
	handoff := r.renderCommandOperationHandoff()
	for _, want := range []string{
		"provider_operation_result",
		"provider=\"mcp:slides\"",
		"tool=\"run_operation\"",
		"kind=\"presentation_generation\"",
		"created deck artifact",
		"artifact_refs=/tmp/codrax/deck.pptx",
	} {
		if !strings.Contains(handoff, want) {
			t.Fatalf("provider handoff missing %q:\n%s", want, handoff)
		}
	}
}

func TestOperationProviderMCPLazyApproveStartsConfiguredServer(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "presentation_generation",
		OperationKind:        "presentation_generation",
		Source:               "current_message",
		RiskLevel:            "low",
		TargetSurface:        "slides",
		SideEffects:          []string{"local_file_write"},
		Confidence:           0.9,
		Reason:               "user asked for a presentation artifact",
	}}
	yes := true
	timeoutMS := 3000
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "生成一份 PPT\n/approve\n/exit\n")
	r.operationEnabled = true
	r.mcpServers = mcp.NewRegistry()
	r.runtimeAnchor = t.TempDir()
	r.operationMemory = operation.NewMemoryStore(filepath.Join(r.runtimeAnchor, "operation", "memory.jsonl"))
	r.mcpServerConfigs = []types.MCPServerConfig{{
		Name:               "lazy_slides",
		Transport:          types.TransportStdio,
		Command:            os.Args[0],
		Args:               []string{"-test.run=TestOperationProviderLazyMCPFakeServerHelper"},
		Env:                map[string]string{"CODRAX_REPL_FAKE_MCP_SERVER": "1"},
		StartupTimeoutMS:   &timeoutMS,
		OperationProvider:  &yes,
		OperationLazyStart: &yes,
		OperationKinds:     []string{"presentation_generation"},
		OperationSurfaces:  []string{"slides"},
		OperationTool:      "run_operation",
	}}
	r.operationProviders = []operation.ProviderInfo{{
		Name:         "mcp:lazy_slides",
		Kind:         "presentation_generation",
		Surfaces:     []string{"slides"},
		SideEffects:  []string{"local_file_write"},
		RequiresGate: true,
		ToolName:     "run_operation",
		Source:       "mcp",
		LazyStart:    true,
		Loaded:       false,
	}}
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("lazy provider operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if _, err := r.mcpServers.Get("lazy_slides"); err != nil {
		t.Fatalf("lazy MCP server was not registered: %v", err)
	}
	if !r.operationProviders[0].Loaded {
		t.Fatalf("provider descriptor should be marked loaded: %+v", r.operationProviders[0])
	}
	if !strings.Contains(out.String(), "lazy provider created deck artifact") {
		t.Fatalf("lazy provider result not rendered:\n%s", out.String())
	}
	entries, err := r.operationMemory.RecentMatches(r.commandOperationCapabilitySnapshot(), 4)
	if err != nil {
		t.Fatalf("read operation memory: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("lazy provider result was not persisted to operation memory")
	}
	renderedMemory := operation.RenderMemoryForPrompt(entries)
	for _, want := range []string{
		"Historical operation lessons",
		"capability=presentation_generation",
		"provider=\"mcp:lazy_slides\"",
		"tool=\"run_operation\"",
		"lazy provider created deck artifact",
		"not current-source evidence",
	} {
		if !strings.Contains(renderedMemory, want) {
			t.Fatalf("provider memory missing %q:\n%s", want, renderedMemory)
		}
	}
}

func TestOperationProviderMCPLazyStartFailureStaysInOperationLane(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "presentation_generation",
		OperationKind:        "presentation_generation",
		Source:               "current_message",
		RiskLevel:            "low",
		TargetSurface:        "slides",
		SideEffects:          []string{"local_file_write"},
		Confidence:           0.9,
		Reason:               "user asked for a presentation artifact",
	}}
	yes := true
	timeoutMS := 500
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "生成一份 PPT\n/approve\n/exit\n")
	r.operationEnabled = true
	r.mcpServers = mcp.NewRegistry()
	missingCommand := filepath.Join(t.TempDir(), "missing-mcp-provider")
	r.mcpServerConfigs = []types.MCPServerConfig{{
		Name:               "broken_slides",
		Transport:          types.TransportStdio,
		Command:            missingCommand,
		StartupTimeoutMS:   &timeoutMS,
		OperationProvider:  &yes,
		OperationLazyStart: &yes,
		OperationKinds:     []string{"presentation_generation"},
		OperationSurfaces:  []string{"slides"},
		OperationTool:      "run_operation",
	}}
	r.operationProviders = []operation.ProviderInfo{{
		Name:         "mcp:broken_slides",
		Kind:         "presentation_generation",
		Surfaces:     []string{"slides"},
		SideEffects:  []string{"local_file_write"},
		RequiresGate: true,
		ToolName:     "run_operation",
		Source:       "mcp",
		LazyStart:    true,
		Loaded:       false,
	}}
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("failed lazy provider operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	printed := out.String()
	for _, want := range []string{
		"Operation provider `mcp:broken_slides` failed.",
		"start lazy MCP operation provider",
		"broken_slides",
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("lazy provider failure output missing %q:\n%s", want, printed)
		}
	}
	if r.operationProviders[0].Loaded {
		t.Fatalf("failed lazy provider should remain unloaded: %+v", r.operationProviders[0])
	}
}

func TestOperationProviderLocalSkillApproveExecutesManifestCommand(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "presentation_generation",
		OperationKind:        "presentation_generation",
		Source:               "current_message",
		RiskLevel:            "low",
		TargetSurface:        "slides",
		SideEffects:          []string{"local_file_write"},
		Confidence:           0.9,
		Reason:               "user asked for a presentation artifact",
	}}
	yes := true
	timeoutMS := 3000
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "生成一份 PPT\n/approve\n/exit\n")
	r.operationEnabled = true
	r.runtimeAnchor = t.TempDir()
	r.operationMemory = operation.NewMemoryStore(filepath.Join(r.runtimeAnchor, "operation", "memory.jsonl"))
	r.operationSkillConfigs = []types.OperationSkillConfig{{
		Name:                          "local_ppt",
		OperationKinds:                []string{"presentation_generation"},
		OperationSurfaces:             []string{"slides"},
		OperationSideEffects:          []string{"local_file_write"},
		OperationRequiresConfirmation: &yes,
		OperationLazyStart:            &yes,
		Command:                       os.Args[0],
		Args:                          []string{"-test.run=TestOperationProviderLocalSkillFakeHelper"},
		Env:                           map[string]string{"CODRAX_REPL_FAKE_LOCAL_SKILL": "1"},
		InputMode:                     "stdin_json",
		TimeoutMS:                     &timeoutMS,
	}}
	r.operationProviders = []operation.ProviderInfo{{
		Name:         "skill:local_ppt",
		Kind:         "presentation_generation",
		Surfaces:     []string{"slides"},
		SideEffects:  []string{"local_file_write"},
		RequiresGate: true,
		ToolName:     "run",
		Source:       "skill",
		LazyStart:    true,
		Loaded:       false,
	}}
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("local skill provider operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if !r.operationProviders[0].Loaded {
		t.Fatalf("local skill provider descriptor should be marked loaded: %+v", r.operationProviders[0])
	}
	printed := out.String()
	for _, want := range []string{
		"local skill created deck artifact",
		"/tmp/codrax/local-skill-deck.pptx",
		"verified",
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("local skill output missing %q:\n%s", want, printed)
		}
	}
	handoff := r.renderCommandOperationHandoff()
	for _, want := range []string{
		"provider_operation_result",
		"provider=\"skill:local_ppt\"",
		"tool=\"run\"",
		"local skill created deck artifact",
		"artifact_refs=/tmp/codrax/local-skill-deck.pptx",
		"observations=2",
		"verification_status=verified",
	} {
		if !strings.Contains(handoff, want) {
			t.Fatalf("local skill handoff missing %q:\n%s", want, handoff)
		}
	}
	entries, err := r.operationMemory.RecentMatches(r.commandOperationCapabilitySnapshot(), 4)
	if err != nil {
		t.Fatalf("read operation memory: %v", err)
	}
	renderedMemory := operation.RenderMemoryForPrompt(entries)
	for _, want := range []string{
		"provider=\"skill:local_ppt\"",
		"tool=\"run\"",
		"local skill created deck artifact",
		"not current-source evidence",
	} {
		if !strings.Contains(renderedMemory, want) {
			t.Fatalf("local skill memory missing %q:\n%s", want, renderedMemory)
		}
	}
}

func TestOperationProviderLocalSkillQueuesWorkflowNextAction(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "presentation_generation",
		OperationKind:        "presentation_generation",
		Source:               "current_message",
		RiskLevel:            "medium",
		TargetSurface:        "slides",
		SideEffects:          []string{"local_file_write"},
		Confidence:           0.9,
		Reason:               "user asked for a chained presentation workflow",
	}}
	yes := true
	timeoutMS := 3000
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "根据说明生成一份 PPT\n/approve\n/approve\n/exit\n")
	r.operationEnabled = true
	r.runtimeAnchor = t.TempDir()
	r.operationMemory = operation.NewMemoryStore(filepath.Join(r.runtimeAnchor, "operation", "memory.jsonl"))
	r.operationSkillConfigs = []types.OperationSkillConfig{
		{
			Name:                          "manual_reader",
			OperationKinds:                []string{"presentation_generation"},
			OperationSurfaces:             []string{"slides"},
			OperationSideEffects:          []string{"local_file_write"},
			OperationRequiresConfirmation: &yes,
			OperationLazyStart:            &yes,
			Command:                       os.Args[0],
			Args:                          []string{"-test.run=TestOperationProviderLocalSkillFakeHelper"},
			Env: map[string]string{
				"CODRAX_REPL_FAKE_LOCAL_SKILL":         "1",
				"CODRAX_REPL_FAKE_LOCAL_SKILL_CHAIN_A": "1",
			},
			InputMode: "stdin_json",
			TimeoutMS: &timeoutMS,
		},
		{
			Name:                          "ppt_builder",
			OperationKinds:                []string{"presentation_generation"},
			OperationSurfaces:             []string{"slides"},
			OperationSideEffects:          []string{"local_file_write"},
			OperationRequiresConfirmation: &yes,
			OperationLazyStart:            &yes,
			Command:                       os.Args[0],
			Args:                          []string{"-test.run=TestOperationProviderLocalSkillFakeHelper"},
			Env: map[string]string{
				"CODRAX_REPL_FAKE_LOCAL_SKILL":         "1",
				"CODRAX_REPL_FAKE_LOCAL_SKILL_CHAIN_B": "1",
			},
			InputMode: "stdin_json",
			TimeoutMS: &timeoutMS,
		},
	}
	r.operationProviders = []operation.ProviderInfo{
		{
			Name:         "skill:manual_reader",
			Kind:         "presentation_generation",
			Surfaces:     []string{"slides"},
			SideEffects:  []string{"local_file_write"},
			RequiresGate: true,
			ToolName:     "run",
			Source:       "skill",
			LazyStart:    true,
			Loaded:       false,
		},
		{
			Name:         "skill:ppt_builder",
			Kind:         "presentation_generation",
			Surfaces:     []string{"slides"},
			SideEffects:  []string{"local_file_write"},
			RequiresGate: true,
			ToolName:     "run",
			Source:       "skill",
			LazyStart:    true,
			Loaded:       false,
		},
	}
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("workflow provider operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingProviderOperation != nil {
		t.Fatalf("workflow next action should be consumed after second approval: %+v", r.pendingProviderOperation)
	}
	printed := out.String()
	for _, want := range []string{
		"manual reader extracted deck notes",
		"queued next workflow action",
		"ppt builder consumed out/manual-notes.md",
		"out/deck.pptx",
		"verified",
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("workflow output missing %q:\n%s", want, printed)
		}
	}
	handoff := r.renderCommandOperationHandoff()
	for _, want := range []string{
		"provider=\"skill:manual_reader\"",
		"provider=\"skill:ppt_builder\"",
		"next_actions=1",
		"workflow_state=\"workflow_id=wf-manual-deck step=manual_extracted return_to=skill:manual_reader",
		"ppt builder consumed out/manual-notes.md",
		"observations=1",
	} {
		if !strings.Contains(handoff, want) {
			t.Fatalf("workflow handoff missing %q:\n%s", want, handoff)
		}
	}
	entries, err := r.operationMemory.RecentMatches(r.commandOperationCapabilitySnapshot(), 6)
	if err != nil {
		t.Fatalf("read operation memory: %v", err)
	}
	renderedMemory := operation.RenderMemoryForPrompt(entries)
	for _, want := range []string{
		"provider=\"skill:manual_reader\"",
		"provider=\"skill:ppt_builder\"",
		"next_action=\"provider=skill:ppt_builder",
		"workflow_state=\"workflow_id=wf-manual-deck step=manual_extracted return_to=skill:manual_reader",
		"not current-source evidence",
	} {
		if !strings.Contains(renderedMemory, want) {
			t.Fatalf("workflow memory missing %q:\n%s", want, renderedMemory)
		}
	}
}

func TestOperationProviderLocalSkillQueuesMultipleWorkflowNextActionsSerially(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "presentation_generation",
		OperationKind:        "presentation_generation",
		Source:               "current_message",
		RiskLevel:            "medium",
		TargetSurface:        "slides",
		SideEffects:          []string{"local_file_write"},
		Confidence:           0.9,
		Reason:               "user asked for a chained presentation workflow",
	}}
	yes := true
	timeoutMS := 3000
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "根据说明生成一份 PPT\n/approve\n/approve\n/approve\n/exit\n")
	r.operationEnabled = true
	r.runtimeAnchor = t.TempDir()
	r.operationSkillConfigs = []types.OperationSkillConfig{
		{
			Name:                          "manual_reader",
			OperationKinds:                []string{"presentation_generation"},
			OperationSurfaces:             []string{"slides"},
			OperationSideEffects:          []string{"local_file_write"},
			OperationRequiresConfirmation: &yes,
			OperationLazyStart:            &yes,
			Command:                       os.Args[0],
			Args:                          []string{"-test.run=TestOperationProviderLocalSkillFakeHelper"},
			Env: map[string]string{
				"CODRAX_REPL_FAKE_LOCAL_SKILL":             "1",
				"CODRAX_REPL_FAKE_LOCAL_SKILL_CHAIN_MULTI": "1",
			},
			InputMode: "stdin_json",
			TimeoutMS: &timeoutMS,
		},
		{
			Name:                          "ppt_builder",
			OperationKinds:                []string{"presentation_generation"},
			OperationSurfaces:             []string{"slides"},
			OperationSideEffects:          []string{"local_file_write"},
			OperationRequiresConfirmation: &yes,
			OperationLazyStart:            &yes,
			Command:                       os.Args[0],
			Args:                          []string{"-test.run=TestOperationProviderLocalSkillFakeHelper"},
			Env: map[string]string{
				"CODRAX_REPL_FAKE_LOCAL_SKILL":         "1",
				"CODRAX_REPL_FAKE_LOCAL_SKILL_CHAIN_B": "1",
			},
			InputMode: "stdin_json",
			TimeoutMS: &timeoutMS,
		},
	}
	r.operationProviders = []operation.ProviderInfo{
		{
			Name:         "skill:manual_reader",
			Kind:         "presentation_generation",
			Surfaces:     []string{"slides"},
			SideEffects:  []string{"local_file_write"},
			RequiresGate: true,
			ToolName:     "run",
			Source:       "skill",
			LazyStart:    true,
		},
		{
			Name:         "skill:ppt_builder",
			Kind:         "presentation_generation",
			Surfaces:     []string{"slides"},
			SideEffects:  []string{"local_file_write"},
			RequiresGate: true,
			ToolName:     "run",
			Source:       "skill",
			LazyStart:    true,
		},
	}
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("multi-action workflow should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingProviderOperation != nil {
		t.Fatalf("all queued workflow actions should be consumed: %+v", r.pendingProviderOperation)
	}
	if r.providerWorkflow == nil {
		t.Fatal("workflow instance should be retained for status/handoff inspection")
	}
	if len(r.providerWorkflow.Actions) != 3 || len(r.providerWorkflow.Edges) != 2 {
		t.Fatalf("workflow graph sizes actions=%d edges=%d wf=%+v", len(r.providerWorkflow.Actions), len(r.providerWorkflow.Edges), r.providerWorkflow)
	}
	printed := out.String()
	for _, want := range []string{
		"queued next workflow action(s): 2",
		"ppt builder consumed out/manual-notes.md",
		"ppt builder consumed out/appendix-notes.md",
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("multi-action workflow output missing %q:\n%s", want, printed)
		}
	}
	handoff := r.renderCommandOperationHandoff()
	for _, want := range []string{
		"request=\"build main slides\"",
		"request=\"build appendix slides\"",
		"provider=\"skill:ppt_builder\"",
	} {
		if !strings.Contains(handoff, want) {
			t.Fatalf("multi-action workflow handoff missing %q:\n%s", want, handoff)
		}
	}
}

func TestOperationProviderLocalSkillMissingConfigStaysInOperationLane(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "presentation_generation",
		OperationKind:        "presentation_generation",
		Source:               "current_message",
		RiskLevel:            "low",
		TargetSurface:        "slides",
		SideEffects:          []string{"local_file_write"},
		Confidence:           0.9,
		Reason:               "user asked for a presentation artifact",
	}}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "生成一份 PPT\n/approve\n/exit\n")
	r.operationEnabled = true
	r.operationProviders = []operation.ProviderInfo{{
		Name:         "skill:missing",
		Kind:         "presentation_generation",
		Surfaces:     []string{"slides"},
		RequiresGate: true,
		ToolName:     "run",
		Source:       "skill",
		LazyStart:    true,
	}}
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("failed local skill provider operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	printed := out.String()
	for _, want := range []string{
		"Operation provider `skill:missing` failed.",
		"not configured",
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("missing local skill failure output missing %q:\n%s", want, printed)
		}
	}
}

func TestOperationProviderLocalSkillLargeOutputUsesPayloadRef(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "presentation_generation",
		OperationKind:        "presentation_generation",
		Source:               "current_message",
		RiskLevel:            "low",
		TargetSurface:        "slides",
		SideEffects:          []string{"local_file_write"},
		Confidence:           0.9,
		Reason:               "user asked for a presentation artifact",
	}}
	yes := true
	timeoutMS := 3000
	maxOutput := 64
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "生成一份 PPT\n/approve\n/exit\n")
	r.operationEnabled = true
	r.runtimeAnchor = t.TempDir()
	r.operationSkillConfigs = []types.OperationSkillConfig{{
		Name:                          "noisy",
		OperationKinds:                []string{"presentation_generation"},
		OperationSurfaces:             []string{"slides"},
		OperationRequiresConfirmation: &yes,
		Command:                       os.Args[0],
		Args:                          []string{"-test.run=TestOperationProviderLocalSkillFakeHelper"},
		Env:                           map[string]string{"CODRAX_REPL_FAKE_LOCAL_SKILL": "1", "CODRAX_REPL_FAKE_LOCAL_SKILL_LARGE": "1"},
		TimeoutMS:                     &timeoutMS,
		MaxOutputBytes:                &maxOutput,
	}}
	r.operationProviders = []operation.ProviderInfo{{
		Name:         "skill:noisy",
		Kind:         "presentation_generation",
		Surfaces:     []string{"slides"},
		RequiresGate: true,
		ToolName:     "run",
		Source:       "skill",
		LazyStart:    true,
	}}
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("large local skill provider operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if !strings.Contains(out.String(), "Full output:") && !strings.Contains(out.String(), "完整输出：") {
		t.Fatalf("large local skill output did not show payload ref:\n%s", out.String())
	}
	handoff := r.renderCommandOperationHandoff()
	if !strings.Contains(handoff, "payload_ref=") {
		t.Fatalf("large local skill handoff missing payload ref:\n%s", handoff)
	}
}

func TestCommandOperationE2E_AutoLowRiskExecutesWithoutApprove(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"show go version","program":"go","args":["version"],"risk_level":"low","side_effects":[]}]}`)},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "查询 go 版本\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	r.operationPolicy.AutoLowRisk = true
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("auto operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingOperation != nil {
		t.Fatalf("auto operation should not remain pending: %+v", r.pendingOperation)
	}
	printed := out.String()
	if !strings.Contains(printed, "completed") || !strings.Contains(printed, "go version") {
		t.Fatalf("auto operation result missing:\n%s", printed)
	}
	if strings.Contains(printed, "awaiting approval") {
		t.Fatalf("auto low-risk operation should not ask for approval:\n%s", printed)
	}
}

func TestCommandOperationE2E_StopsPrearmedRendererSpinner(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"show go version","program":"go","args":["version"],"risk_level":"low","side_effects":[]}]}`)},
	}
	r, runner, _ := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	var dock bytes.Buffer
	r.renderer = render.New(&dock, true)
	r.renderer.StartSpinner()
	t.Cleanup(func() {
		if r.renderer.SpinnerActive() {
			r.renderer.StopSpinner()
		}
	})

	r.operationDispatch("查询 go 版本", "查询 go 版本", commandOperationPolicy("low"))

	if r.renderer.SpinnerActive() {
		t.Fatal("operation planning must close the pre-armed classifier spinner before rendering the plan")
	}
	if len(runner.requests) != 0 {
		t.Fatalf("command operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingOperation == nil {
		t.Fatal("manual command plan should remain pending approval")
	}
}

func TestCommandOperationE2E_HardDeniedDestructiveCommandDoesNotExecute(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("high")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{commandOperationPlanResp(`{"status":"ready","risk_level":"high","requires_confirmation":true,"work_dir":".","steps":[{"id":"s1","title":"destructive remove","program":"rm","args":["-rf","/"],"risk_level":"high","side_effects":["delete files"]}]}`)},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "删除所有文件\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("blocked operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingOperation != nil {
		t.Fatalf("blocked operation should not remain pending: %+v", r.pendingOperation)
	}
	printed := out.String()
	if !strings.Contains(printed, "blocked") {
		t.Fatalf("blocked plan message missing:\n%s", printed)
	}
	if strings.Contains(printed, "completed") {
		t.Fatalf("blocked destructive operation must not execute:\n%s", printed)
	}
}

func TestCommandOperationE2E_UnknownProgramRequiresManualApprovalEvenWhenAutoEnabled(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"run custom tool","program":"corp-custom-tool","args":["--version"],"risk_level":"low","side_effects":[]}]}`)},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "运行内部工具查询版本\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	r.operationPolicy.AutoLowRisk = true
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("manual unknown program should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingOperation == nil {
		t.Fatal("unknown program should wait for manual approval, not auto-execute")
	}
	printed := out.String()
	if !strings.Contains(printed, "Run `/approve`") {
		t.Fatalf("manual approval message missing:\n%s", printed)
	}
	if strings.Contains(printed, "completed") {
		t.Fatalf("unknown program should not execute before approval:\n%s", printed)
	}
}

func TestCommandOperationE2E_PlannerRequestCarriesPolicySignals(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"needs_clarification","questions":[{"id":"target","question":"target?"}]}`),
		},
	}
	planner := NewCommandOperationPlanner(adapter)
	_, err := planner.PlanCommandOperation(context.Background(), "安装工具", "/repo", commandOperationPolicy("medium"))
	if err != nil {
		t.Fatalf("PlanCommandOperation: %v", err)
	}
	if len(adapter.calls) != 1 {
		t.Fatalf("Chat calls=%d, want 1", len(adapter.calls))
	}
	user := ""
	for i := len(adapter.calls[0].messages) - 1; i >= 0; i-- {
		if adapter.calls[0].messages[i].Role == "user" {
			user = adapter.calls[0].messages[i].Content
			break
		}
	}
	for _, want := range []string{
		"operation_kind=computer_operation",
		"risk=medium",
		"## repo_root\n/repo",
		"安装工具",
	} {
		if !strings.Contains(user, want) {
			t.Fatalf("planner request missing %q:\n%s", want, user)
		}
	}
}

func TestCommandOperationE2E_FailedApprovedCommandCreatesRevisedPlan(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("medium")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"medium","requires_confirmation":true,"work_dir":".","steps":[{"id":"s1","title":"show missing tool version","program":"definitely-missing-codrax-command","args":["--version"],"risk_level":"medium","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"show go version instead","program":"go","args":["version"],"risk_level":"low","side_effects":[]}]}`),
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "查询工具版本\n/approve\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("command operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("expected initial plan + replan calls, got %d", len(adapter.calls))
	}
	if r.pendingOperation == nil {
		t.Fatal("revised plan should wait for manual approval when auto-low-risk is disabled")
	}
	if got := r.pendingOperation.Steps[0].Program; got != "go" {
		t.Fatalf("revised pending program=%q, want go", got)
	}
	printed := out.String()
	for _, want := range []string{
		"Operation plan",
		"failed",
		"revised command plan",
		"go version",
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("replan output missing %q:\n%s", want, printed)
		}
	}
}

func TestDropRepeatedFailedCommandStepsRemovesOnlyFailedRetry(t *testing.T) {
	failedPlan := operation.CommandOperationPlan{
		ID: "op-failed",
		Steps: []operation.CommandStep{{
			ID:      "missing",
			Program: "definitely-missing-codrax-command",
			Args:    []string{"--version"},
		}, {
			ID:      "ok",
			Program: "pwd",
		}},
	}
	result := operation.CommandOperationResult{
		PlanID: "op-failed",
		Status: operation.StatusFailed,
		StepResults: []operation.CommandStepResult{{
			StepID: "missing",
			Status: operation.StatusFailed,
		}, {
			StepID: "ok",
			Status: operation.StatusExecuted,
		}},
	}
	revised := operation.CommandOperationRequest{
		RequiresConfirmation: true,
		Steps: []operation.CommandStep{{
			ID:      "retry-missing",
			Program: "definitely-missing-codrax-command",
			Args:    []string{"--version"},
		}, {
			ID:      "next",
			Program: "go",
			Args:    []string{"version"},
		}, {
			ID:      "repeat-ok",
			Program: "pwd",
		}},
	}
	filtered := dropRepeatedFailedCommandSteps(revised, failedPlan, result)
	if len(filtered.Steps) != 2 {
		t.Fatalf("filtered steps=%+v, want 2 steps", filtered.Steps)
	}
	if filtered.Steps[0].Program != "go" || filtered.Steps[1].Program != "pwd" {
		t.Fatalf("wrong steps filtered: %+v", filtered.Steps)
	}
}

func TestCommandReplanAutoExecuteEnvelope(t *testing.T) {
	base := operation.CommandOperationPlan{
		Status:       operation.StatusFailed,
		RiskLevel:    "medium",
		ApprovalMode: operation.ApprovalManual,
		WorkDir:      "/repo",
	}
	okPlan := operation.CommandOperationPlan{
		Status:       operation.StatusReady,
		RiskLevel:    "low",
		ApprovalMode: operation.ApprovalAutoLowRisk,
		WorkDir:      "/repo",
		Steps: []operation.CommandStep{{
			ID:           "s1",
			Program:      "go",
			Args:         []string{"version"},
			RiskLevel:    "low",
			AutoApproval: operation.StepAutoEligible,
		}},
	}
	if !commandReplanCanAutoExecute(base, okPlan) {
		t.Fatal("same-dir lower-risk read-only eligible replan should be allowed to auto-continue")
	}
	clone := func(plan operation.CommandOperationPlan) operation.CommandOperationPlan {
		plan.Steps = append([]operation.CommandStep(nil), plan.Steps...)
		return plan
	}
	changedDir := clone(okPlan)
	changedDir.WorkDir = "/tmp"
	if commandReplanCanAutoExecute(base, changedDir) {
		t.Fatal("changed workdir must require manual approval")
	}
	withShell := clone(okPlan)
	withShell.Steps[0].Shell = "go version"
	if commandReplanCanAutoExecute(base, withShell) {
		t.Fatal("shell replan must require manual approval")
	}
	withSideEffect := clone(okPlan)
	withSideEffect.Steps[0].SideEffects = []string{"local_file_write"}
	if commandReplanCanAutoExecute(base, withSideEffect) {
		t.Fatal("side-effecting replan must require manual approval")
	}
	escalated := clone(okPlan)
	escalated.RiskLevel = "high"
	if commandReplanCanAutoExecute(base, escalated) {
		t.Fatal("risk-escalating replan must require manual approval")
	}
}
