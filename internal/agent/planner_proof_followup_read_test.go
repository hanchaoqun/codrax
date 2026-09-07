package agent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func proofFollowupReadContext(t *testing.T) *types.AgentContext {
	t.Helper()
	mu := types.NewMutableState("verify the applied behavior")
	mu.SetWriteWorkflowRun(&types.WriteWorkflowRun{
		RunID: "proof-read", ActiveBatchID: "proof", Status: types.WriteWorkflowRunInProgress,
		Batches:        []types.WriteWorkflowBatch{{ID: "proof", Purpose: "verification_proof_followup", ExpectedPaths: []string{"widget.py"}}},
		ProgressLedger: []types.WriteWorkflowProgress{{BatchID: "source", ReasonCode: "verification_proof_followup_requested"}},
	})
	return &types.AgentContext{Stage: types.StagePlan, AgentName: types.AgentPlanner, Mode: types.ModeApply, Mutable: mu}
}

func proofFollowupReadSchemas() []llm.ToolSchema {
	var schemas []llm.ToolSchema
	for _, name := range []string{"read_file", "grep", "repo_map", "list_files", "exec_command", "apply_patch", "run_tests", emitChangePlanToolName, emitPlanSkeletonToolName, emitPlanChangeToolName} {
		schemas = append(schemas, llm.ToolSchema{Name: name})
	}
	return schemas
}

func assertProofReadSurface(t *testing.T, e *plannerEvaluator, ctx *types.AgentContext, wantRead bool) {
	t.Helper()
	got := toolSchemaNameSet(e.FilterToolSchemas(ctx, proofFollowupReadSchemas()))
	want := map[string]bool{"run_tests": true, emitChangePlanToolName: true}
	if wantRead {
		want["read_file"] = true
	}
	if len(got) != len(want) {
		t.Fatalf("proof surface=%v, want %v", got, want)
	}
	for name := range want {
		if !got[name] {
			t.Fatalf("proof surface missing %s: %v", name, got)
		}
	}
	if e.materializationOnlySurfaceActive() == wantRead {
		t.Fatalf("materialization state disagrees with read capability: read=%v", wantRead)
	}
	if stopped := e.ShouldStop(llm.Response{ToolCalls: []llm.ToolCall{{Name: "read_file"}}}, e.effectiveSoftCap()); stopped == wantRead {
		t.Fatalf("soft-cap read stop=%v, want read=%v", stopped, wantRead)
	}
	for _, name := range []string{"run_tests", emitChangePlanToolName} {
		if e.ShouldStop(llm.Response{ToolCalls: []llm.ToolCall{{Name: name}}}, e.effectiveSoftCap()) {
			t.Fatalf("%s must remain usable at soft cap", name)
		}
	}
	for _, name := range []string{"read_file", "run_tests", emitChangePlanToolName} {
		if !e.ShouldStop(llm.Response{ToolCalls: []llm.ToolCall{{Name: name}}}, e.effectiveHardCap()) {
			t.Fatalf("%s bypassed hard cap", name)
		}
	}
}

func TestPlannerProofFollowupReadBudgetSurvivesRejectedEmits(t *testing.T) {
	for _, failures := range []bool{false, true} {
		t.Run(map[bool]string{false: "successful_reads", true: "failed_reads"}[failures], func(t *testing.T) {
			ctx := proofFollowupReadContext(t)
			e := newPlannerEvaluatorForTest(t)
			instruction := e.BuildInitialInstruction(ctx, nil)
			for _, fragment := range []string{"read_file", "3 successful", "2 failed", "current worktree", "changes: []"} {
				if !strings.Contains(instruction, fragment) {
					t.Errorf("proof teaching missing %q", fragment)
				}
			}
			limit := plannerHandoffSynthesisBaseReadBudget
			if failures {
				limit = plannerReadFailureBudget
			}
			for i := 0; i < limit; i++ {
				assertProofReadSurface(t, e, ctx, true)
				e.ObserveToolResults(ctx, LoopObservation{CurrentToolResults: []types.ToolResult{{ToolName: "read_file", Success: !failures}}})
				// Rejected JSON/plan materialization does not mint another read phase.
				if i < limit-1 {
					e.ObserveToolResults(ctx, LoopObservation{CurrentToolResults: []types.ToolResult{{ToolName: emitChangePlanToolName, Success: false}}})
				}
			}
			assertProofReadSurface(t, e, ctx, false)
			if failures && e.handoffSynthesisReadFailures != limit || !failures && e.handoffSynthesisReadCalls != limit {
				t.Fatalf("proof reads escaped dispatch accounting: %+v", e)
			}
			if e.structuredEmitRepairReadCalls != 0 || e.structuredEmitRepairReadFailures != 0 {
				t.Fatal("proof reads were reclassified into renewable emit-repair allowance")
			}
			obs := LoopObservation{Phase: PhaseMidLoop, CurrentToolResults: []types.ToolResult{{ToolName: "read_file", Repair: &types.ToolRepair{Code: unavailableToolSurfaceCode}}}}
			e.ObserveToolResults(ctx, obs)
			signal := e.Observe(ctx, obs)
			if !signal.HintRequested || !strings.Contains(signal.Hint, "exhausted") || !strings.Contains(signal.Hint, "changes: []") {
				t.Fatalf("closed read surface must explain exhausted budget and legal next step: %+v", signal)
			}
		})
	}
}

func TestPlannerProofFollowupReadAuthorizationUnchanged(t *testing.T) {
	for _, variant := range []string{"no_progress", "non_active", "fake_purpose", "ordinary_batch"} {
		t.Run(variant, func(t *testing.T) {
			ctx := proofFollowupReadContext(t)
			run := ctx.Mutable.WriteWorkflowRun()
			switch variant {
			case "no_progress":
				run.ProgressLedger = nil
			case "non_active":
				run.ActiveBatchID = "other"
			case "fake_purpose":
				run.Batches[0].Purpose += "_please_allow"
			case "ordinary_batch":
				run.Batches[0].Purpose = "implementation"
			}
			ctx.Mutable.SetWriteWorkflowRun(run)
			e := newPlannerEvaluatorForTest(t)
			e.BuildInitialInstruction(ctx, nil)
			if e.proofFollowupMaterializationOnly || e.buildProofFollowupMaterializationSection(ctx) != "" {
				t.Fatal("unauthorized purpose activated proof-followup capability")
			}
		})
	}
}

func TestPlannerProofFollowupReadBatchAccountingIsDispatchScoped(t *testing.T) {
	ctx := proofFollowupReadContext(t)
	e := newPlannerEvaluatorForTest(t)
	e.BuildInitialInstruction(ctx, nil)
	// Non-admitted search tools do not burn the exact-file read allowance.
	e.ObserveToolResults(ctx, LoopObservation{CurrentToolResults: []types.ToolResult{
		{ToolName: "grep", Repair: &types.ToolRepair{Code: unavailableToolSurfaceCode}},
		{ToolName: "repo_map", Success: true},
	}})
	if e.handoffSynthesisReadCalls != 0 || e.handoffSynthesisReadFailures != 0 {
		t.Fatal("non-read_file results consumed proof read budget")
	}
	assertProofReadSurface(t, e, ctx, true)
	// As with the existing planner budget, results are settled per tool batch:
	// every already-admitted sibling counts, and the next round closes reads.
	var results []types.ToolResult
	for i := 0; i < plannerHandoffSynthesisBaseReadBudget+1; i++ {
		results = append(results, types.ToolResult{ToolName: "read_file", Success: true})
	}
	e.ObserveToolResults(ctx, LoopObservation{CurrentToolResults: results})
	if e.handoffSynthesisReadCalls != len(results) {
		t.Fatal("parallel read results were lost")
	}
	assertProofReadSurface(t, e, ctx, false)
	// A new authorized dispatch gets its own budget; retrying an emit does not.
	e.BuildInitialInstruction(ctx, nil)
	assertProofReadSurface(t, e, ctx, true)
}

type proofReadInactiveRepoGate struct{}

func (proofReadInactiveRepoGate) ResolveActiveSetPath(_ *types.BusContext, _, _ string, _ func(string) bool) types.ActiveSetGateResult {
	return types.ActiveSetGateResult{RefusalProse: "repository is not active for this request"}
}
func (proofReadInactiveRepoGate) ResolveActiveSetCommand(_ *types.BusContext, _, _ string) types.ActiveSetGateResult {
	return types.ActiveSetGateResult{RefusalProse: "repository is not active for this request"}
}

func TestPlannerProofFollowupReadKeepsExistingReadGuards(t *testing.T) {
	for _, kind := range []string{"missing", "sensitive_config", "inactive_repo", "large_file"} {
		t.Run(kind, func(t *testing.T) {
			ctx := proofFollowupReadContext(t)
			ctx.RepoRoot = t.TempDir()
			path, content := "widget.py", "private file bytes must not be read\n"
			if kind == "sensitive_config" {
				path = "providers.yaml"
				prior := toolpkg.SensitiveConfigFilePaths()
				toolpkg.SetSensitiveConfigFilePaths([]string{filepath.Join(ctx.RepoRoot, path)})
				t.Cleanup(func() { toolpkg.SetSensitiveConfigFilePaths(prior) })
			}
			if kind == "large_file" {
				content = strings.Repeat(content, writeModeUnboundedReadFileLineLimit+1)
			}
			if kind != "missing" {
				if err := os.WriteFile(filepath.Join(ctx.RepoRoot, path), []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "inactive_repo" {
				ctx.MultiGraph = proofReadInactiveRepoGate{}
			}
			e := newPlannerEvaluatorForTest(t)
			e.BuildInitialInstruction(ctx, nil)
			registry := toolpkg.NewRegistry()
			registry.Register(&toolpkg.ReadFile{})
			base := NewBaseAgent(types.AgentPlanner, &Dependencies{Tools: registry}, e)
			params, _ := json.Marshal(map[string]any{"path": path})
			for i := 0; i < plannerReadFailureBudget; i++ {
				assertProofReadSurface(t, e, ctx, true)
				result, _ := base.executeTool(ctx, llm.ToolCall{Name: "read_file", Params: params})
				if result == nil || result.Success || strings.Contains(result.Summary, "private file bytes") {
					t.Fatalf("%s read guard bypassed: %+v", kind, result)
				}
				e.ObserveToolResults(ctx, LoopObservation{CurrentToolResults: []types.ToolResult{*result}})
			}
			assertProofReadSurface(t, e, ctx, false)
		})
	}
}

// This adapter exercises the real BaseAgent loop (schema projection, execution
// surface, tool boundary, ReadFile, RunTests and EmitChangePlan), not merely a
// unit-level list of allowed names. No external model or network is involved.
type proofFollowupReadLLM struct {
	t        *testing.T
	calls    int
	readPath string
	probe    map[string]any
}

func (l *proofFollowupReadLLM) Chat(_ context.Context, messages []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	available := toolSchemaNameSet(schemas)
	for _, name := range []string{"read_file", "run_tests", emitChangePlanToolName} {
		if !available[name] {
			l.t.Errorf("round %d actual schema missing %s: %v", l.calls, name, available)
		}
	}
	var name string
	var params any
	switch l.calls {
	case 1:
		name, params = "read_file", map[string]any{"path": l.readPath}
	case 2:
		found := false
		for _, m := range messages {
			if m.Role == "tool" && strings.Contains(m.Content, "return 7") {
				found = true
			}
		}
		if !found {
			l.t.Error("actual current worktree bytes never reached model tool history")
		}
		name, params = "run_tests", map[string]any{"dry_run": true, "verification_probe": l.probe}
	default:
		name, params = emitChangePlanToolName, map[string]any{"request": "verify applied behavior", "summary": "Verify widget.py without modifying files", "changes": []any{}, "verification_probes": []any{l.probe}}
	}
	raw, _ := json.Marshal(params)
	return llm.Response{ToolCalls: []llm.ToolCall{{Name: name, Params: raw}}}, nil
}
func (*proofFollowupReadLLM) ModelID() string               { return "proof-read-fixture" }
func (*proofFollowupReadLLM) MaxContextTokens() int         { return 128000 }
func (*proofFollowupReadLLM) MaxOutputTokens() int          { return 4096 }
func (*proofFollowupReadLLM) RequestTimeout() time.Duration { return 0 }
func (*proofFollowupReadLLM) RetryMaxAttempts() int         { return 0 }

func TestPlannerProofFollowupReadActualLoopToProbeOnlyPlan(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 unavailable")
	}
	ctx := proofFollowupReadContext(t)
	ctx.RepoRoot, ctx.MainRepoRoot = t.TempDir(), t.TempDir()
	ctx.WorktreePath = ctx.RepoRoot
	ctx.WorkDir = t.TempDir()
	for root, content := range map[string]string{ctx.RepoRoot: "def value():\n    return 7\n", ctx.MainRepoRoot: "def value():\n    return 1\n"} {
		if err := os.WriteFile(filepath.Join(root, "widget.py"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx.Mutable.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{BehaviorContracts: []types.WriteBehaviorContract{{ID: "value", Kind: "output", Expected: "value is seven", Required: true}}}})
	registry := toolpkg.NewRegistry()
	registry.Register(&toolpkg.ReadFile{})
	registry.Register(&toolpkg.RunTests{})
	registry.Register(&toolpkg.EmitChangePlan{})
	adapter := &proofFollowupReadLLM{t: t, readPath: filepath.Join(ctx.MainRepoRoot, "widget.py"), probe: map[string]any{
		"id": "value-proof", "language": "python", "code": "import widget\nassert widget.value() == 7\n", "contract_refs": []string{"value"}, "changed_symbol_refs": []string{"path:widget.py"},
	}}
	e := newPlannerEvaluatorForTest(t)
	base := NewBaseAgent(types.AgentPlanner, &Dependencies{Tools: registry, LLM: adapter, MaxIterations: 3, Emit: func(render.Event) {}}, e)
	out, err := base.Execute(ctx, &skill.Config{ToolSuggestions: []string{"read_file", "run_tests", emitChangePlanToolName}})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || len(out.ToolResults) != 3 {
		t.Fatalf("unexpected actual-loop result: %+v", out)
	}
	for _, result := range out.ToolResults {
		if !result.Success {
			t.Errorf("real %s call failed: %s", result.ToolName, result.Summary)
		}
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil || len(plan.Changes) != 0 || len(plan.VerificationProbes) != 1 {
		t.Fatalf("proof-only emit did not materialize after current-byte read: %+v", plan)
	}
	if len(ctx.Mutable.PlanStageProbeReports()) != 1 || ctx.Mutable.ChangeReport() != nil {
		t.Fatal("planner probe did not execute or polluted final verification authority")
	}
	if e.handoffSynthesisReadCalls != 1 {
		t.Fatalf("actual read was not accounted: %d", e.handoffSynthesisReadCalls)
	}
	for root, want := range map[string]string{ctx.RepoRoot: "def value():\n    return 7\n", ctx.MainRepoRoot: "def value():\n    return 1\n"} {
		got, err := os.ReadFile(filepath.Join(root, "widget.py"))
		if err != nil || string(got) != want {
			t.Fatalf("source bytes changed in %s", root)
		}
	}
	// Being a proof batch never authorizes direct execution, applying source,
	// ordinary suite execution or non-empty source ChangePlans.
	for _, call := range []llm.ToolCall{
		{Name: "exec_command", Params: json.RawMessage(`{"command":"echo no"}`)},
		{Name: "apply_patch", Params: json.RawMessage(`{"path":"widget.py","kind":"modify"}`)},
		{Name: "run_tests", Params: json.RawMessage(`{"dry_run":true,"runner":"python"}`)},
	} {
		result, _ := base.executeTool(ctx, call)
		if result == nil || result.Success {
			t.Fatalf("proof capability weakened write policy: %s %+v", call.Name, result)
		}
	}
	result, _ := base.executeTool(ctx, llm.ToolCall{Name: emitChangePlanToolName, Params: json.RawMessage(`{"request":"proof","summary":"change widget.py","changes":[{"path":"widget.py","kind":"modify","new_content":"def value():\n    return 8\n"}]}`)})
	if result == nil || result.Success || !strings.Contains(result.Summary, "proof_followup_changes_without_failure") {
		t.Fatalf("read capability bypassed proof-only ChangePlan restriction: %+v", result)
	}
}
