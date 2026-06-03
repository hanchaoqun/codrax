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
	if os.Getenv("CODRAX_REPL_FAKE_LOCAL_SKILL_CHAIN_DAG") == "1" {
		input, _ := payload["input"].(map[string]any)
		if deckPath, _ := input["deck_path"].(string); deckPath != "" {
			appendixPath, _ := input["appendix_path"].(string)
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
				"success":       true,
				"summary":       "manual reader composed DAG final report for " + deckPath + " and " + appendixPath,
				"artifact_refs": []string{"out/final-report.md"},
			})
			return
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":       true,
			"summary":       "manual reader split deck work into main and appendix branches",
			"artifact_refs": []string{"out/manual-notes.md", "out/appendix-notes.md"},
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
			"return_action": map[string]any{
				"provider":       "skill:manual_reader",
				"operation_kind": "artifact_generation",
				"target_surface": "local_file",
				"risk_level":     "low",
				"request":        "compose final workflow report after branch artifacts",
				"input":          map[string]any{"deck_path": "out/deck.pptx", "appendix_path": "out/appendix-deck.pptx"},
			},
			"workflow_state": map[string]any{
				"workflow_id": "wf-dag-deck",
				"step":        "branches_queued",
				"return_to":   "skill:manual_reader",
				"data":        map[string]any{"branch_count": 2},
			},
		})
		return
	}
	if os.Getenv("CODRAX_REPL_FAKE_LOCAL_SKILL_CHAIN_RETURN") == "1" {
		input, _ := payload["input"].(map[string]any)
		if deckPath, _ := input["deck_path"].(string); deckPath != "" {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
				"success":       true,
				"summary":       "manual reader composed final report for " + deckPath,
				"artifact_refs": []string{"out/final-report.md"},
			})
			return
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":       true,
			"summary":       "manual reader extracted deck notes with callback",
			"artifact_refs": []string{"out/manual-notes.md"},
			"next_actions": []map[string]any{{
				"provider":              "skill:ppt_builder",
				"operation_kind":        "presentation_generation",
				"target_surface":        "slides",
				"risk_level":            "medium",
				"side_effects":          []string{"local_file_write"},
				"requires_confirmation": true,
				"request":               "build slides before callback",
				"input":                 map[string]any{"source_payload_ref": "out/manual-notes.md"},
			}},
			"return_action": map[string]any{
				"provider":       "skill:manual_reader",
				"operation_kind": "artifact_generation",
				"target_surface": "local_file",
				"risk_level":     "low",
				"request":        "compose final workflow report",
				"input":          map[string]any{"deck_path": "out/deck.pptx"},
			},
			"workflow_state": map[string]any{
				"workflow_id": "wf-manual-deck",
				"step":        "manual_extracted",
				"return_to":   "skill:manual_reader",
			},
		})
		return
	}
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
		artifact := "out/deck.pptx"
		if strings.Contains(source, "appendix") {
			artifact = "out/appendix-deck.pptx"
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":              true,
			"summary":              "ppt builder consumed " + source,
			"artifact_refs":        []string{artifact},
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
	r.operationPolicy.AutoApprove = false
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
	r.operationPolicy.AutoLowRisk = false
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
	r.operationPolicy.AutoLowRisk = false
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

func TestOperationProviderEvaluatorContinuesWithCommandExtraction(t *testing.T) {
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
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			operationEvaluationResp(`{"status":"continue_command","reason":"provider returned a payload ref that needs bounded extraction","confidence":"high","material_refs":["/tmp/codrax/deck.pptx"]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"extract","title":"extract provider payload","program":"printf","args":["provider payload says deck has 5 slides\n"],"risk_level":"low","side_effects":[]}]}`),
			{Content: "已读取 provider 返回的材料，PPT 草稿包含 5 页。", StopReason: "end_turn"},
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "生成一份 PPT 并说明结果\n/approve\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
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
		t.Fatalf("provider-to-command continuation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(adapter.calls) != 3 {
		t.Fatalf("evaluator+command plan+answer calls=%d want 3", len(adapter.calls))
	}
	printed := out.String()
	for _, want := range []string{
		"created deck artifact",
		"provider payload says deck has 5 slides",
		"已读取 provider 返回的材料",
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("provider-to-command output missing %q:\n%s", want, printed)
		}
	}
	allPrompts := ""
	for _, call := range adapter.calls {
		for _, msg := range call.messages {
			allPrompts += "\n" + msg.Content
		}
	}
	for _, want := range []string{
		"operation_evaluation status=continue_command",
		"provider returned a payload ref",
		"payload_ref=/tmp/codrax/deck.pptx",
	} {
		if !strings.Contains(allPrompts, want) {
			t.Fatalf("provider-to-command handoff prompt missing %q:\n%s", want, allPrompts)
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

func TestOperationProviderLocalSkillQueuesReturnAction(t *testing.T) {
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
		Reason:               "user asked for a chained presentation workflow with callback",
	}}
	yes := true
	timeoutMS := 3000
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "根据说明生成一份 PPT 并总结\n/approve\n/workflow show\n/approve\n/approve\n/exit\n")
	r.operationEnabled = true
	r.runtimeAnchor = t.TempDir()
	r.operationSkillConfigs = []types.OperationSkillConfig{
		{
			Name:                          "manual_reader",
			OperationKinds:                []string{"presentation_generation", "artifact_generation"},
			OperationSurfaces:             []string{"slides", "local_file"},
			OperationSideEffects:          []string{"local_file_write"},
			OperationRequiresConfirmation: &yes,
			OperationLazyStart:            &yes,
			Command:                       os.Args[0],
			Args:                          []string{"-test.run=TestOperationProviderLocalSkillFakeHelper"},
			Env: map[string]string{
				"CODRAX_REPL_FAKE_LOCAL_SKILL":              "1",
				"CODRAX_REPL_FAKE_LOCAL_SKILL_CHAIN_RETURN": "1",
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
			Kind:         "*",
			Surfaces:     []string{"slides", "local_file"},
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
		t.Fatalf("return-action workflow should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingProviderOperation != nil {
		t.Fatalf("return-action workflow should finish after third approval: %+v\noutput:\n%s", r.pendingProviderOperation, out.String())
	}
	if r.providerWorkflow == nil {
		t.Fatal("workflow instance should be retained")
	}
	if len(r.providerWorkflow.Actions) != 3 || len(r.providerWorkflow.Edges) != 2 || r.providerWorkflow.Edges[1].Kind != operation.WorkflowEdgeReturn {
		t.Fatalf("return edge not preserved: wf=%+v", r.providerWorkflow)
	}
	printed := out.String()
	for _, want := range []string{
		"Suggested return action",
		"Operation workflow `provider-workflow-1`",
		"ppt builder consumed out/manual-notes.md",
		"manual reader composed final report for out/deck.pptx",
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("return-action workflow output missing %q:\n%s", want, printed)
		}
	}
}

func TestOperationProviderLocalSkillDAGSchedulesMultipleWorkflowActions(t *testing.T) {
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
		Reason:               "user asked for a presentation workflow that fans out and returns",
	}}
	yes := true
	timeoutMS := 3000
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "根据说明生成主报告和附录 PPT，并最终汇总\n/approve\n/workflow show\n/approve\n/workflow show\n/approve\n/approve\n/exit\n")
	r.operationEnabled = true
	r.runtimeAnchor = t.TempDir()
	r.operationMemory = operation.NewMemoryStore(filepath.Join(r.runtimeAnchor, "operation", "memory.jsonl"))
	r.operationSkillConfigs = []types.OperationSkillConfig{
		{
			Name:                          "manual_reader",
			OperationKinds:                []string{"presentation_generation", "artifact_generation"},
			OperationSurfaces:             []string{"slides", "local_file"},
			OperationSideEffects:          []string{"local_file_write"},
			OperationRequiresConfirmation: &yes,
			OperationLazyStart:            &yes,
			Command:                       os.Args[0],
			Args:                          []string{"-test.run=TestOperationProviderLocalSkillFakeHelper"},
			Env: map[string]string{
				"CODRAX_REPL_FAKE_LOCAL_SKILL":           "1",
				"CODRAX_REPL_FAKE_LOCAL_SKILL_CHAIN_DAG": "1",
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
			Kind:         "*",
			Surfaces:     []string{"slides", "local_file"},
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
		t.Fatalf("DAG workflow should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingProviderOperation != nil {
		t.Fatalf("DAG workflow should finish after queued approvals: %+v\noutput:\n%s", r.pendingProviderOperation, out.String())
	}
	if r.providerWorkflow == nil {
		t.Fatal("workflow instance should be retained")
	}
	if len(r.providerWorkflow.Actions) != 4 || len(r.providerWorkflow.Edges) != 3 {
		t.Fatalf("DAG workflow graph sizes actions=%d edges=%d wf=%+v", len(r.providerWorkflow.Actions), len(r.providerWorkflow.Edges), r.providerWorkflow)
	}
	if r.providerWorkflow.Edges[0].Kind != operation.WorkflowEdgeNext ||
		r.providerWorkflow.Edges[1].Kind != operation.WorkflowEdgeNext ||
		r.providerWorkflow.Edges[2].Kind != operation.WorkflowEdgeReturn {
		t.Fatalf("DAG workflow edge kinds = %+v", r.providerWorkflow.Edges)
	}
	printed := out.String()
	for _, want := range []string{
		"queued next workflow action(s): 3",
		"Operation workflow `provider-workflow-1`",
		"Graph: 4 node(s) / 3 edge(s)",
		"ppt builder consumed out/manual-notes.md",
		"ppt builder consumed out/appendix-notes.md",
		"manual reader composed DAG final report for out/deck.pptx",
		"out/appendix-deck.pptx",
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("DAG workflow output missing %q:\n%s", want, printed)
		}
	}
	handoff := r.renderCommandOperationHandoff()
	for _, want := range []string{
		"provider=\"skill:manual_reader\"",
		"provider=\"skill:ppt_builder\"",
		"request=build main slides",
		"request=\"build appendix slides\"",
		"request=\"compose final workflow report after branch artifacts\"",
		"next_action=\"provider=skill:ppt_builder",
		"return_action=\"provider=skill:manual_reader kind=artifact_generation target=local_file",
		"workflow_state=\"workflow_id=wf-dag-deck step=branches_queued return_to=skill:manual_reader",
	} {
		if !strings.Contains(handoff, want) {
			t.Fatalf("DAG workflow handoff missing %q:\n%s", want, handoff)
		}
	}
	entries, err := r.operationMemory.RecentMatches(r.commandOperationCapabilitySnapshot(), 8)
	if err != nil {
		t.Fatalf("read operation memory: %v", err)
	}
	renderedMemory := operation.RenderMemoryForPrompt(entries)
	for _, want := range []string{
		"provider=\"skill:manual_reader\"",
		"next_action=\"provider=skill:ppt_builder",
		"return_action=\"provider=skill:manual_reader",
		"manual reader composed DAG final report",
	} {
		if !strings.Contains(renderedMemory, want) {
			t.Fatalf("DAG workflow memory missing %q:\n%s", want, renderedMemory)
		}
	}
}

func TestOperationWorkflowCancelStopsQueuedProviderActions(t *testing.T) {
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
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "根据说明生成一份 PPT\n/approve\n/workflow cancel\n/approve\n/exit\n")
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
		t.Fatalf("cancelled workflow should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingProviderOperation != nil || r.providerWorkflow != nil {
		t.Fatalf("workflow cancel should clear pending state pending=%+v workflow=%+v", r.pendingProviderOperation, r.providerWorkflow)
	}
	printed := out.String()
	if !strings.Contains(printed, "cancelled") {
		t.Fatalf("workflow cancel message missing:\n%s", printed)
	}
	if strings.Contains(printed, "ppt builder consumed") {
		t.Fatalf("workflow cancel must stop queued builder action:\n%s", printed)
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

func TestCommandOperationE2E_AutoLowRiskSynthesizesFinalAnswer(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":true,"work_dir":".","steps":[{"id":"s1","title":"show go version","program":"go","args":["version"],"risk_level":"low","side_effects":[]}]}`),
			{Content: "当前 Go 环境可用，版本信息已经查询完成。", StopReason: "end_turn"},
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "查询 go 版本\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("auto operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("planner+answer calls=%d, want 2", len(adapter.calls))
	}
	printed := out.String()
	for _, want := range []string{
		"当前 Go 环境可用",
		"go version",
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("final operation output missing %q:\n%s", want, printed)
		}
	}
	if strings.Contains(printed, "Execution details:") || strings.Contains(printed, "执行详情：") {
		t.Fatalf("final operation answer should not append execution details:\n%s", printed)
	}
}

func TestCommandOperationE2E_AutoExecutionShowsProgress(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"show go version","program":"go","args":["version"],"risk_level":"low","side_effects":[]}]}`),
			{Content: "Go 版本查询完成。", StopReason: "end_turn"},
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()

	r.operationDispatch("查询 go 版本", "查询 go 版本", commandOperationPolicy("low"))

	if len(runner.requests) != 0 {
		t.Fatalf("auto operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	printed := out.String()
	for _, want := range []string{
		"will run automatically",
		"Go 版本查询完成",
		"go version",
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("auto execution output missing %q:\n%s", want, printed)
		}
	}
	if strings.Contains(printed, "Operating: show go version") {
		t.Fatalf("step progress should stay in the live status/log lane, not permanent output:\n%s", printed)
	}
	if !strings.Contains(printed, "\n  │\n\n  │\n  │ Operation plan `") {
		t.Fatalf("auto plan and execution result should render as separate bordered blocks:\n%s", printed)
	}
}

func TestCommandOperationE2E_AutoExecutionRendersPlanResultAndFinalHeaders(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"show go version","program":"go","args":["version"],"risk_level":"low","side_effects":[]}]}`),
			{Content: "Go 版本查询完成。", StopReason: "end_turn"},
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "/exit\n")
	r.renderer = render.New(out, true)
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()

	r.operationDispatch("查询 go 版本", "查询 go 版本", commandOperationPolicy("low"))

	if len(runner.requests) != 0 {
		t.Fatalf("auto operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	plain := stripANSIOnly(out.String())
	planIdx := strings.Index(plain, "operation plan · auto-run")
	resultIdx := strings.Index(plain, "operation execution · completed")
	finalIdx := strings.Index(plain, "final answer · ready")
	if planIdx < 0 || resultIdx < 0 || finalIdx < 0 {
		t.Fatalf("operation route headers missing from rendered output:\n%s", plain)
	}
	if !(planIdx < resultIdx && resultIdx < finalIdx) {
		t.Fatalf("operation route headers out of order plan=%d result=%d final=%d:\n%s", planIdx, resultIdx, finalIdx, plain)
	}
}

func TestCommandOperationE2E_ContinueAfterRunsSecondBatchBeforeAnswer(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"continue_after":true,"work_dir":".","steps":[{"id":"s1","title":"discover go version","program":"go","args":["version"],"risk_level":"low","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s2","title":"discover go os","program":"go","args":["env","GOOS"],"risk_level":"low","side_effects":[]}]}`),
			{Content: "已完成两轮探测：先确认 Go 版本，再确认 GOOS。", StopReason: "end_turn"},
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "查询 go 版本和 GOOS\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("continued operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(adapter.calls) != 3 {
		t.Fatalf("planner+continuation+answer calls=%d, want 3", len(adapter.calls))
	}
	printed := out.String()
	for _, want := range []string{
		"已完成两轮探测",
		"go version",
		"go env GOOS",
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("continued operation output missing %q:\n%s", want, printed)
		}
	}
	if strings.Contains(printed, "Round 2") || strings.Contains(printed, "Execution details:") || strings.Contains(printed, "执行详情：") {
		t.Fatalf("continued operation should show per-round panels, not final aggregated details:\n%s", printed)
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
	r.operationPolicy.AutoApprove = false
	r.operationPolicy.AutoLowRisk = false
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
	r.operationPolicy.AutoLowRisk = false
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

func TestCommandOperationE2E_UnknownProgramRequiresManualApprovalWhenAutoApproveDisabled(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"run custom tool","program":"corp-custom-tool","args":["--version"],"risk_level":"low","side_effects":[]}]}`)},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "运行内部工具查询版本\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	r.operationPolicy.AutoApprove = false
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
	r.operationPolicy.AutoApprove = false
	r.operationPolicy.AutoLowRisk = false
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

func TestCommandOperationE2E_FailedApprovedCommandAutoExecutesSafeReplanWhenAutoApproveEnabled(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("medium")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"high","requires_confirmation":true,"work_dir":".","steps":[{"id":"s1","title":"show missing tool version","program":"definitely-missing-codrax-command","args":["--version"],"risk_level":"high","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s2","title":"show go version instead","program":"go","args":["version"],"risk_level":"low","side_effects":[]}]}`),
			{Content: "缺失工具不可用，已自动改用 Go 版本查询并完成。", StopReason: "end_turn"},
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "查询工具版本\n/approve\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	r.operationPolicy.AutoApprove = true
	r.operationPolicy.AutoLowRisk = false
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("command operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(adapter.calls) != 3 {
		t.Fatalf("expected initial plan + replan + answer calls, got %d", len(adapter.calls))
	}
	if r.pendingOperation != nil {
		t.Fatalf("safe revised plan should auto-execute under global auto-approve, pending=%+v", r.pendingOperation)
	}
	printed := out.String()
	for _, want := range []string{
		"revised command plan",
		"will continue execution",
		"go version",
		"缺失工具不可用",
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("auto replan output missing %q:\n%s", want, printed)
		}
	}
	if strings.Contains(printed, "awaiting approval") || strings.Contains(printed, "等待批准") {
		t.Fatalf("safe replan should not wait for approval when AutoApprove=true:\n%s", printed)
	}
}

func TestCommandOperationE2E_FailedApprovedCommandAutoExecutesNetworkReadReplan(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("medium")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"high","requires_confirmation":true,"work_dir":".","steps":[{"id":"s1","title":"show missing tool version","program":"definitely-missing-codrax-command","args":["--version"],"risk_level":"high","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s2","title":"download manual page","shell":"printf 'manual page downloaded\\n'","risk_level":"low","side_effects":["network_read"]}]}`),
			{Content: "缺失工具不可用，已自动改用网络读取计划并完成。", StopReason: "end_turn"},
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "读取网站用户手册\n/approve\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	r.operationPolicy.AutoApprove = true
	r.operationPolicy.AutoLowRisk = false
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("command operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingOperation != nil {
		t.Fatalf("network-read revised plan should auto-execute under global auto-approve, pending=%+v", r.pendingOperation)
	}
	printed := out.String()
	for _, want := range []string{
		"will continue execution",
		"manual page downloaded",
		"网络读取计划",
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("network-read replan output missing %q:\n%s", want, printed)
		}
	}
	if strings.Contains(printed, "awaiting approval") || strings.Contains(printed, "等待批准") {
		t.Fatalf("network-read replan should not wait for approval when AutoApprove=true:\n%s", printed)
	}
}

func TestCommandOperationE2E_InvalidShellFilterReplansWithNumericStepID(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"medium","requires_confirmation":false,"work_dir":".","steps":[{"id":1,"title":"bad process filter","shell":"grep -i vpn | grep -v grep || echo none","risk_level":"medium","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":"[{\"id\":2,\"title\":\"safe version check\",\"shell\":\"printf 'vpn-tool 1.0\\\\n' | grep vpn\",\"risk_level\":\"low\",\"side_effects\":[]}]"}`),
			{Content: "已修复无输入过滤命令，并完成替代查询。", StopReason: "end_turn"},
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "查一下当前 VPN 软件和版本\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("command operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(adapter.calls) != 3 {
		t.Fatalf("planner+replan+answer calls=%d, want 3", len(adapter.calls))
	}
	printed := out.String()
	for _, want := range []string{
		"Commands to validate",
		"stdin-consuming shell command",
		"revised command plan",
		"will run automatically",
		"printf 'vpn-tool 1.0",
		"vpn-tool 1.0",
		"已修复无输入过滤命令",
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("invalid-shell replan output missing %q:\n%s", want, printed)
		}
	}
}

func TestCommandOperationE2E_ChangedWorkdirAutoEligibleReplanWaitsForApproval(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"medium","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"failing probe","shell":"false","risk_level":"medium","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"medium","requires_confirmation":false,"work_dir":"/tmp","steps":[{"id":"s2","title":"inspect network interfaces","shell":"/sbin/ifconfig | /usr/bin/grep -E '(inet [^f]|utun|ipsec|tun|vpn)' | /usr/bin/grep -v '::'","risk_level":"medium","side_effects":[]}]}`),
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "查看 VPN IP 连通性\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("command operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingOperation == nil {
		t.Fatalf("changed-workdir revised plan should wait for user approval; calls=%d output:\n%s", len(adapter.calls), out.String())
	}
	if r.pendingOperation.ApprovalMode != operation.ApprovalManual {
		t.Fatalf("changed-workdir replan approval=%q, want manual", r.pendingOperation.ApprovalMode)
	}
	printed := out.String()
	revisedIdx := strings.Index(printed, "Generated a revised command plan")
	if revisedIdx < 0 {
		t.Fatalf("revised plan section missing:\n%s", printed)
	}
	revisedSection := printed[revisedIdx:]
	for _, bad := range []string{"will continue execution", "will run automatically", "Approval: `auto_low_risk`", "审批：`auto_low_risk`"} {
		if strings.Contains(revisedSection, bad) {
			t.Fatalf("changed-workdir replan should not promise auto execution or display auto approval %q:\n%s", bad, printed)
		}
	}
	if !strings.Contains(revisedSection, "waiting for approval") && !strings.Contains(revisedSection, "等待批准") {
		t.Fatalf("changed-workdir replan should visibly wait for approval:\n%s", printed)
	}
}

func TestCommandOperationE2E_MultipleInvalidRepairsUntilSuccess(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"medium","requires_confirmation":false,"work_dir":".","steps":[{"id":"bad-grep","title":"bad process filter","shell":"grep -i vpn","risk_level":"medium","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"bad-cat","title":"bad cat","shell":"cat","risk_level":"low","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"fixed","title":"safe version check","shell":"printf 'vpn-tool 1.0\\n' | grep vpn","risk_level":"low","side_effects":[]}]}`),
			{Content: "经过多轮修复，已获取 VPN 工具版本：vpn-tool 1.0。", StopReason: "end_turn"},
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "查一下当前 VPN 软件和版本\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("command operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(adapter.calls) != 4 {
		t.Fatalf("planner+2 repairs+answer calls=%d, want 4", len(adapter.calls))
	}
	if len(r.operationResults) != 3 {
		t.Fatalf("operation results=%d, want 3 rounds: %+v", len(r.operationResults), r.operationResults)
	}
	printed := out.String()
	for _, want := range []string{
		"stdin-consuming shell command",
		"vpn-tool 1.0",
		"经过多轮修复",
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("multi-repair output missing %q:\n%s", want, printed)
		}
	}
}

func TestCommandOperationE2E_RepeatedFailedCommandRepairsWithoutRerun(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"missing","title":"missing command","shell":"definitely-missing-codrax-command --version","risk_level":"low","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"repeat","title":"bad retry","shell":"definitely-missing-codrax-command --version","risk_level":"low","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"fallback","title":"fallback check","shell":"printf 'fallback version 1.0\\n'","risk_level":"low","side_effects":[]}]}`),
			{Content: "已避免重复失败命令，改用替代检查得到 fallback version 1.0。", StopReason: "end_turn"},
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "查询 demo 工具版本\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("command operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(r.operationResults) != 3 {
		t.Fatalf("operation results=%d, want initial failure + lint repair + fallback: %+v", len(r.operationResults), r.operationResults)
	}
	commandNotFoundRuns := 0
	repeatedLint := false
	for _, rec := range r.operationResults {
		for _, step := range rec.Result.StepResults {
			if step.FailureClass == "command_not_found" {
				commandNotFoundRuns++
			}
			if step.FailureClass == "invalid_plan" && strings.Contains(step.Error, "repeats an already failed command") {
				repeatedLint = true
			}
		}
	}
	if commandNotFoundRuns != 1 {
		t.Fatalf("missing command should execute once, got %d runs; records=%+v", commandNotFoundRuns, r.operationResults)
	}
	if !repeatedLint {
		t.Fatalf("repeated failed command was not linted before execution; records=%+v", r.operationResults)
	}
	if !strings.Contains(out.String(), "fallback version 1.0") {
		t.Fatalf("fallback output missing:\n%s", out.String())
	}
}

func TestCommandOperationE2E_NonzeroExitRepairsToSuccessfulBatch(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"bad","title":"failing probe","shell":"printf 'probe failed\\n'; exit 2","risk_level":"low","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"fixed","title":"fallback probe","shell":"printf 'fallback ok\\n'","risk_level":"low","side_effects":[]}]}`),
			{Content: "已根据非零退出改用替代探测，结果：fallback ok。", StopReason: "end_turn"},
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "执行一个需要失败后替代探测的操作\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("command operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(r.operationResults) != 2 {
		t.Fatalf("operation results=%d, want failure + fallback: %+v", len(r.operationResults), r.operationResults)
	}
	if got := r.operationResults[0].Result.StepResults[0].FailureClass; got != "nonzero_exit" {
		t.Fatalf("first failure class=%q, want nonzero_exit", got)
	}
	if !strings.Contains(out.String(), "fallback ok") {
		t.Fatalf("fallback output missing:\n%s", out.String())
	}
}

func TestCommandOperationE2E_ContinuationRunsNextBoundedBatch(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"continue_after":true,"work_dir":".","goal":"collect two observations","next_batch":"first observation","steps":[{"id":"one","title":"first observation","shell":"printf 'phase1\\n'","risk_level":"low","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","goal":"collect two observations","next_batch":"second observation","steps":[{"id":"two","title":"second observation","shell":"printf 'phase2\\n'","risk_level":"low","side_effects":[]}]}`),
			{Content: "两轮观测已完成：phase1 和 phase2。", StopReason: "end_turn"},
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "分两步收集观测\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("command operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(r.operationResults) != 2 {
		t.Fatalf("operation results=%d, want two executed batches: %+v", len(r.operationResults), r.operationResults)
	}
	printed := out.String()
	for _, want := range []string{"phase1", "phase2", "两轮观测已完成"} {
		if !strings.Contains(printed, want) {
			t.Fatalf("continuation output missing %q:\n%s", want, printed)
		}
	}
}

func TestCommandOperationE2E_RepairBudgetExhaustionIsReported(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"bad1","title":"bad grep","shell":"grep vpn","risk_level":"low","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"bad2","title":"bad cat","shell":"cat","risk_level":"low","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"bad3","title":"bad head","shell":"head -20","risk_level":"low","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"bad4","title":"bad tail","shell":"tail -20","risk_level":"low","side_effects":[]}]}`),
			{Content: "修复预算已耗尽，当前只能给出部分结论。", StopReason: "end_turn"},
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "持续修复无效命令直到预算耗尽\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("command operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	foundBudget := false
	for _, rec := range r.operationResults {
		for _, step := range rec.Result.StepResults {
			if step.FailureClass == "budget_exhausted" {
				foundBudget = true
			}
		}
	}
	if !foundBudget {
		t.Fatalf("budget_exhausted result missing: %+v", r.operationResults)
	}
	if !strings.Contains(out.String(), "修复预算已耗尽") {
		t.Fatalf("budget final answer missing:\n%s", out.String())
	}
}

func TestCommandOperationE2E_RiskEscalatingRepairPausesForApproval(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"bad","title":"failing probe","shell":"printf 'probe failed\\n'; exit 2","risk_level":"low","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"high","requires_confirmation":true,"work_dir":".","steps":[{"id":"write","title":"write fallback","program":"touch","args":["/tmp/codrax-operation-risk-test"],"risk_level":"high","side_effects":["local_file_write"]}]}`),
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "失败后需要写文件才能继续\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("command operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingOperation == nil {
		t.Fatal("risk-escalating repair should pause for approval")
	}
	if r.pendingOperation.ApprovalMode != operation.ApprovalManual {
		t.Fatalf("approval=%q, want manual", r.pendingOperation.ApprovalMode)
	}
	printed := out.String()
	if !strings.Contains(printed, "Run `/approve`") && !strings.Contains(printed, "运行 `/approve`") {
		t.Fatalf("approval prompt missing:\n%s", printed)
	}
}

func TestCommandOperationE2E_SystemInfoQueryUsesMultiRoundGoalLoop(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"continue_after":true,"work_dir":".","goal":"collect portable system facts","missing_observations":["os","memory","cpu","gpu"],"success_criteria":["answer includes os/memory/cpu/gpu"],"next_batch":"discover OS first","steps":[{"id":"os","title":"OS probe","shell":"printf 'os=TestOS\\n'","risk_level":"low","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","goal":"collect portable system facts","next_batch":"collect hardware facts","steps":[{"id":"hardware","title":"hardware probe","shell":"printf 'memory=16GB\\ncpu=8\\ngpu=10\\n'","risk_level":"low","side_effects":[]}]}`),
			{Content: "系统信息已收集：TestOS，内存 16GB，CPU 8 核，GPU 10 核。", StopReason: "end_turn"},
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "当前是什么操作系统，内存多大，CPU，GPU多少核\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("system info operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(r.operationResults) != 2 {
		t.Fatalf("operation results=%d, want two bounded batches: %+v", len(r.operationResults), r.operationResults)
	}
	printed := out.String()
	for _, want := range []string{"TestOS", "16GB", "CPU 8", "GPU 10"} {
		if !strings.Contains(printed, want) {
			t.Fatalf("system-info output missing %q:\n%s", want, printed)
		}
	}
}

func TestCommandOperationE2E_SoftwareVersionQueryContinuesFromDiscovery(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"continue_after":true,"work_dir":".","goal":"identify running VPN-like software and version","next_batch":"discover candidate process","steps":[{"id":"discover","title":"candidate process","shell":"printf 'process=vpn-agent\\n'","risk_level":"low","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","goal":"identify running VPN-like software and version","next_batch":"query discovered version","steps":[{"id":"version","title":"version query","shell":"printf 'vpn-agent version 2.3\\n'","risk_level":"low","side_effects":[]}]}`),
			{Content: "已确认运行中的 VPN 相关软件为 vpn-agent，版本 2.3。", StopReason: "end_turn"},
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "查一下当前 VPN 软件和版本\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("software-version operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(r.operationResults) != 2 {
		t.Fatalf("operation results=%d, want discovery + version: %+v", len(r.operationResults), r.operationResults)
	}
	printed := out.String()
	for _, want := range []string{"vpn-agent", "2.3"} {
		if !strings.Contains(printed, want) {
			t.Fatalf("software-version output missing %q:\n%s", want, printed)
		}
	}
}

func TestCommandOperationE2E_LargeFileExtractionKeepsPayloadRefForHandoff(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	largeText := strings.Repeat("x", 128) + " ERROR42"
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","goal":"extract key marker from large output","success_criteria":["payload_ref retained for follow-up"],"steps":[{"id":"extract","title":"large extraction","shell":"printf '` + largeText + `\\n'","risk_level":"low","side_effects":[]}]}`),
			{Content: "大输出已保存为 payload，可继续围绕 ERROR42 缩小范围。", StopReason: "end_turn"},
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "从一个大文件输出里提取 ERROR42\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	r.operationPolicy.OutputPreviewBytes = 16
	r.runtimeAnchor = t.TempDir()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("large extraction operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(r.operationResults) != 1 {
		t.Fatalf("operation results=%d, want one extraction batch: %+v", len(r.operationResults), r.operationResults)
	}
	step := r.operationResults[0].Result.StepResults[0]
	if step.PayloadRef == "" {
		t.Fatalf("large output should keep payload ref: %+v", step)
	}
	printed := out.String()
	for _, want := range []string{"payload", "ERROR42"} {
		if !strings.Contains(printed, want) {
			t.Fatalf("large extraction output missing %q:\n%s", want, printed)
		}
	}
}

func TestCommandOperationE2E_UnfamiliarToolHelpGuidesFollowupCommand(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"continue_after":true,"work_dir":".","goal":"learn demo tool usage then run it","next_batch":"read tool help","steps":[{"id":"help","title":"demo help","shell":"printf 'Usage: demo --mode fast --input FILE\\n'","risk_level":"low","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","goal":"learn demo tool usage then run it","next_batch":"run learned command shape","steps":[{"id":"run","title":"demo run","shell":"printf 'demo output ok\\n'","risk_level":"low","side_effects":[]}]}`),
			{Content: "已先读取工具说明，再按 `--mode fast --input FILE` 形态完成执行：demo output ok。", StopReason: "end_turn"},
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "先学会 demo 工具怎么用，再驱动它完成任务\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("help-to-command operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(r.operationResults) != 2 {
		t.Fatalf("operation results=%d, want help + run: %+v", len(r.operationResults), r.operationResults)
	}
	printed := out.String()
	for _, want := range []string{"Usage: demo", "--mode fast", "demo output ok"} {
		if !strings.Contains(printed, want) {
			t.Fatalf("help-to-command output missing %q:\n%s", want, printed)
		}
	}
}

func TestCommandOperationE2E_AmbiguousDestructiveTargetAsksClarification(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("high")}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"needs_clarification","risk_level":"high","requires_confirmation":true,"questions":[{"id":"target","question":"Which exact directory or file should be deleted?","suggestions":["provide exact path","provide delete scope","cancel deletion"]}]}`),
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "帮我删除一些临时文件\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("ambiguous destructive operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingOperation != nil {
		t.Fatalf("ambiguous destructive operation must not create executable pending plan: %+v", r.pendingOperation)
	}
	printed := out.String()
	if !strings.Contains(printed, "Which exact directory or file should be deleted?") {
		t.Fatalf("destructive clarification missing:\n%s", printed)
	}
	if strings.Contains(printed, "rm ") {
		t.Fatalf("ambiguous destructive target must not render a delete command:\n%s", printed)
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
	policy := operation.DefaultCommandPolicy()
	policy.AutoApprove = true
	policy.AutoLowRisk = false
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
	if !commandReplanCanAutoExecute(policy, base, okPlan) {
		t.Fatal("same-dir lower-risk read-only eligible replan should be allowed to auto-continue")
	}
	clone := func(plan operation.CommandOperationPlan) operation.CommandOperationPlan {
		plan.Steps = append([]operation.CommandStep(nil), plan.Steps...)
		return plan
	}
	changedDir := clone(okPlan)
	changedDir.WorkDir = "/tmp"
	if commandReplanCanAutoExecute(policy, base, changedDir) {
		t.Fatal("changed workdir must require manual approval")
	}
	withShell := clone(okPlan)
	withShell.Steps[0].Shell = "go version"
	withShell.Steps[0].Program = ""
	withShell.Steps[0].Args = nil
	if !commandReplanCanAutoExecute(policy, base, withShell) {
		t.Fatal("auto-approved, same-dir, no-side-effect shell replan should be allowed to continue")
	}
	withNetworkRead := clone(okPlan)
	withNetworkRead.Steps[0].SideEffects = []string{"network_read"}
	if !commandReplanCanAutoExecute(policy, base, withNetworkRead) {
		t.Fatal("observation-only network_read replan should be allowed to continue")
	}
	withSideEffect := clone(okPlan)
	withSideEffect.Steps[0].SideEffects = []string{"local_file_write"}
	if commandReplanCanAutoExecute(policy, base, withSideEffect) {
		t.Fatal("write side-effecting replan must require manual approval")
	}
	escalated := clone(okPlan)
	escalated.RiskLevel = "high"
	if commandReplanCanAutoExecute(policy, base, escalated) {
		t.Fatal("risk-escalating replan must require manual approval")
	}
}
