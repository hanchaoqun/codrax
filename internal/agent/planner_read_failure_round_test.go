package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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

func plannerReadFailureRoundContext() *types.AgentContext {
	mu := types.NewMutableState("fix widget value")
	mu.SetWriteWorkflowRun(&types.WriteWorkflowRun{
		RunID: "read-recovery", ActiveBatchID: "source", Status: types.WriteWorkflowRunInProgress,
		Batches: []types.WriteWorkflowBatch{{ID: "source", ExpectedPaths: []string{"widget.py"}, Status: types.WriteWorkflowBatchReadyToPlan}},
	})
	return &types.AgentContext{Stage: types.StagePlan, AgentName: types.AgentPlanner, Mode: types.ModeApply, Mutable: mu}
}

type plannerReadFailureRoundLLM struct {
	t     *testing.T
	calls int
}

func (l *plannerReadFailureRoundLLM) Chat(_ context.Context, messages []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	available := toolSchemaNameSet(schemas)
	call := func(name string, params any) llm.ToolCall {
		raw, err := json.Marshal(params)
		if err != nil {
			l.t.Fatal(err)
		}
		return llm.ToolCall{Name: name, Params: raw}
	}
	if l.calls <= 3 {
		for _, name := range []string{"read_file", "grep", "list_files"} {
			if !available[name] {
				return llm.Response{}, fmt.Errorf("round %d lost %s before a failed-batch correction opportunity: %v", l.calls, name, available)
			}
		}
	}
	switch l.calls {
	case 1:
		return llm.Response{ToolCalls: []llm.ToolCall{
			call("read_file", map[string]any{"path": "guessed/widget.py"}),
			call("grep", map[string]any{"path": "guessed/widget.py", "pattern": "value"}),
		}}, nil
	case 2:
		foundTypedFailure := false
		for _, message := range messages {
			if message.Role == "tool" && strings.Contains(message.Content, "typed_tool_refinement") && strings.Contains(message.Content, "repo_map") {
				foundTypedFailure = true
			}
		}
		if !foundTypedFailure {
			l.t.Error("real missing-path refinement never reached the next model turn")
		}
		return llm.Response{ToolCalls: []llm.ToolCall{call("list_files", map[string]any{"path": "."})}}, nil
	case 3:
		foundPath := false
		for _, message := range messages {
			if message.Role == "tool" && strings.Contains(message.Content, "widget.py") && strings.Contains(message.Content, "test_widget.py") {
				foundPath = true
			}
		}
		if !foundPath {
			l.t.Error("real directory navigation did not publish the fixture paths")
		}
		return llm.Response{ToolCalls: []llm.ToolCall{
			call("read_file", map[string]any{"path": "widget.py"}),
			call("read_file", map[string]any{"path": "test_widget.py"}),
		}}, nil
	case 4:
		for _, name := range []string{"read_file", "grep", "list_files"} {
			if available[name] {
				l.t.Errorf("three successful acquisitions must still close %s", name)
			}
		}
		foundSource := false
		for _, message := range messages {
			if message.Role == "tool" && strings.Contains(message.Content, "return 1") {
				foundSource = true
			}
		}
		if !foundSource {
			l.t.Error("current source bytes never reached materialization")
		}
		return llm.Response{ToolCalls: []llm.ToolCall{call(emitChangePlanToolName, map[string]any{
			"request": "change widget value from one to two", "summary": "Update the observed widget function with a bounded source patch.",
			"changes": []any{map[string]any{"path": "widget.py", "kind": "patch", "rationale": "return the requested value",
				"edits": []any{map[string]any{"kind": "replace", "start_line": 2, "end_line": 2, "old_text": "    return 1\n", "content": "    return 2\n"}},
			}},
		})}}, nil
	default:
		return llm.Response{}, fmt.Errorf("unexpected extra model round %d", l.calls)
	}
}
func (*plannerReadFailureRoundLLM) ModelID() string               { return "planner-read-failure-round" }
func (*plannerReadFailureRoundLLM) MaxContextTokens() int         { return 128000 }
func (*plannerReadFailureRoundLLM) MaxOutputTokens() int          { return 4096 }
func (*plannerReadFailureRoundLLM) RequestTimeout() time.Duration { return 0 }
func (*plannerReadFailureRoundLLM) RetryMaxAttempts() int         { return 0 }

func TestPlannerReadFailureRoundActualLoopRecoversBeforeMaterialization(t *testing.T) {
	ctx := plannerReadFailureRoundContext()
	ctx.RepoRoot, ctx.WorkDir = t.TempDir(), t.TempDir()
	ctx.MainRepoRoot = ctx.RepoRoot
	const source = "def value():\n    return 1\n"
	for name, body := range map[string]string{"widget.py": source, "test_widget.py": "from widget import value\nassert value() == 1\n"} {
		if err := os.WriteFile(filepath.Join(ctx.RepoRoot, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	registry := toolpkg.NewRegistry()
	registry.Register(&toolpkg.ReadFile{})
	registry.Register(&toolpkg.GrepTool{})
	registry.Register(&toolpkg.ListFiles{})
	registry.Register(&toolpkg.EmitChangePlan{})
	adapter := &plannerReadFailureRoundLLM{t: t}
	e := newPlannerEvaluatorForTest(t)
	base := NewBaseAgent(types.AgentPlanner, &Dependencies{Tools: registry, LLM: adapter, MaxIterations: 5, Emit: func(render.Event) {}}, e)
	out, err := base.Execute(ctx, &skill.Config{ToolSuggestions: []string{"read_file", "grep", "list_files", emitChangePlanToolName}})
	if err != nil {
		t.Fatalf("same-batch failed reads must leave a real correction turn: %v", err)
	}
	if out == nil || len(out.ToolResults) != 6 || adapter.calls != 4 {
		t.Fatalf("unexpected real execution shape: rounds=%d output=%+v", adapter.calls, out)
	}
	for i, result := range out.ToolResults {
		if result.Success != (i >= 2) {
			t.Errorf("actual tool result %d has wrong outcome: %+v", i, result)
		}
		if result.Repair != nil && result.Repair.Code == unavailableToolSurfaceCode {
			t.Errorf("legal recovery was refused before execution: %+v", result)
		}
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil || len(plan.Changes) != 1 || plan.Changes[0].Path != "widget.py" || !strings.Contains(plan.Changes[0].Patch, "+    return 2") {
		t.Fatalf("observed bytes did not produce a validated bounded plan: %+v", plan)
	}
	if e.handoffSynthesisReadCalls != 3 || e.handoffSynthesisReadFailures != 1 {
		t.Fatalf("real accounting: success calls=%d failed rounds=%d, want 3/1", e.handoffSynthesisReadCalls, e.handoffSynthesisReadFailures)
	}
	if got, err := os.ReadFile(filepath.Join(ctx.RepoRoot, "widget.py")); err != nil || string(got) != source {
		t.Fatal("planning changed source bytes")
	}
}

func plannerReadFailureRoundLane(t *testing.T, lane string) (*plannerEvaluator, *types.AgentContext, int, func() (int, int)) {
	t.Helper()
	ctx := plannerReadFailureRoundContext()
	if lane == "verify_repair" {
		ctx.Mutable.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{PlanID: "previous", BatchID: "source", FailureKind: types.FailureKindTestsFailed})
	}
	e := newPlannerEvaluatorForTest(t)
	e.BuildInitialInstruction(ctx, nil)
	if lane == "structured_repair" || lane == "verify_repair" {
		for i := 0; i < e.handoffSynthesisReadBudget; i++ {
			e.ObserveToolResults(ctx, LoopObservation{CurrentToolResults: []types.ToolResult{{ToolName: "read_file", Success: true}}})
		}
	}
	switch lane {
	case "structured_repair":
		e.ObserveToolResults(ctx, LoopObservation{CurrentToolResults: []types.ToolResult{{ToolName: emitChangePlanToolName, Success: false}}})
		return e, ctx, plannerStructuredEmitRepairReadBudget, func() (int, int) { return e.structuredEmitRepairReadCalls, e.structuredEmitRepairReadFailures }
	case "verify_repair":
		return e, ctx, plannerVerifyFailureRepairReadBudget, func() (int, int) { return e.verifyFailureRepairReadCalls, e.verifyFailureRepairReadFailures }
	default:
		return e, ctx, e.handoffSynthesisReadBudget, func() (int, int) { return e.handoffSynthesisReadCalls, e.handoffSynthesisReadFailures }
	}
}

func TestPlannerReadFailureRoundOrdinaryLaneBoundaries(t *testing.T) {
	for _, lane := range []string{"handoff", "structured_repair", "verify_repair"} {
		for _, scenario := range []string{"two_failed_rounds", "mixed_success_failure", "success_calls", "unrelated_results"} {
			t.Run(lane+"/"+scenario, func(t *testing.T) {
				e, ctx, budget, counts := plannerReadFailureRoundLane(t, lane)
				schemas := []llm.ToolSchema{{Name: "read_file"}, {Name: "grep"}, {Name: "list_files"}, {Name: "repo_map"}, {Name: emitChangePlanToolName}}
				assertSurface := func(wantRead bool) {
					t.Helper()
					got := toolSchemaNameSet(e.FilterToolSchemas(ctx, schemas))
					for _, name := range []string{"read_file", "grep", "list_files", "repo_map"} {
						if got[name] != wantRead {
							t.Fatalf("read=%t: schema=%v", wantRead, got)
						}
					}
					if !got[emitChangePlanToolName] || e.materializationOnlySurfaceActive() == wantRead {
						t.Fatalf("materialization/schema disagree: %v", got)
					}
					if stopped := e.ShouldStop(llm.Response{ToolCalls: []llm.ToolCall{{Name: "read_file"}}}, e.effectiveSoftCap()); stopped == wantRead {
						t.Fatalf("soft-cap stop=%t, want read=%t", stopped, wantRead)
					}
					if !e.ShouldStop(llm.Response{ToolCalls: []llm.ToolCall{{Name: "read_file"}}}, e.effectiveHardCap()) {
						t.Fatal("read recovery escaped hard iteration cap")
					}
				}
				observe := func(results ...types.ToolResult) {
					e.ObserveToolResults(ctx, LoopObservation{CurrentToolResults: results})
				}
				assertSurface(true)
				switch scenario {
				case "two_failed_rounds":
					observe(types.ToolResult{ToolName: "read_file"}, types.ToolResult{ToolName: "grep"})
					if successes, failures := counts(); successes != 0 || failures != 1 {
						t.Fatalf("one failed observation batch=%d/%d, want 0/1", successes, failures)
					}
					assertSurface(true)
					observe(types.ToolResult{ToolName: "repo_map"}, types.ToolResult{ToolName: "list_files"})
					if successes, failures := counts(); successes != 0 || failures != 2 {
						t.Fatalf("two failed observation batches=%d/%d, want 0/2", successes, failures)
					}
					assertSurface(false)
				case "mixed_success_failure", "success_calls":
					if scenario == "mixed_success_failure" {
						observe(types.ToolResult{ToolName: "read_file", Success: true}, types.ToolResult{ToolName: "grep"}, types.ToolResult{ToolName: "list_files"})
						if successes, failures := counts(); successes != 1 || failures != 1 {
							t.Fatalf("mixed batch=%d/%d, want 1/1", successes, failures)
						}
						assertSurface(true)
					}
					current, _ := counts()
					var results []types.ToolResult
					for i := current; i < budget; i++ {
						results = append(results, types.ToolResult{ToolName: "read_file", Success: true})
					}
					observe(results...)
					if successes, _ := counts(); successes != budget {
						t.Fatalf("successful calls were collapsed: %d, want %d", successes, budget)
					}
					assertSurface(false)
				case "unrelated_results":
					observe()
					observe(types.ToolResult{ToolName: "unknown_read_file", Summary: "read_file failure"}, types.ToolResult{ToolName: "run_tests", Summary: "grep missing"})
					if successes, failures := counts(); successes != 0 || failures != 0 {
						t.Fatalf("unrelated names/prose consumed budget: %d/%d", successes, failures)
					}
					assertSurface(true)
				}
			})
		}
	}
}

func TestPlannerReadFailureRoundProofOnlyStillChargesEveryCall(t *testing.T) {
	ctx := proofFollowupReadContext(t)
	e := newPlannerEvaluatorForTest(t)
	e.BuildInitialInstruction(ctx, nil)
	e.ObserveToolResults(ctx, LoopObservation{CurrentToolResults: []types.ToolResult{{ToolName: "grep"}, {ToolName: "repo_map"}}})
	assertProofReadSurface(t, e, ctx, true)
	e.ObserveToolResults(ctx, LoopObservation{CurrentToolResults: []types.ToolResult{{ToolName: "read_file"}, {ToolName: "read_file"}}})
	if e.handoffSynthesisReadFailures != 2 || e.handoffSynthesisReadCalls != 0 {
		t.Fatalf("proof-only failed calls must remain individually charged: %+v", e)
	}
	assertProofReadSurface(t, e, ctx, false)
	e.ObserveToolResults(ctx, LoopObservation{CurrentToolResults: []types.ToolResult{{ToolName: emitChangePlanToolName}}})
	assertProofReadSurface(t, e, ctx, false)
	if e.handoffSynthesisReadFailures != 2 || e.structuredEmitRepairReadFailures != 0 {
		t.Fatal("rejected proof plan renewed or reclassified the read allowance")
	}
}
