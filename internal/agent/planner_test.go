package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// newPlannerEvaluatorForTest builds a plannerEvaluator with the
// resolved AgentSettings defaults so the test stays in sync with
// whatever DefaultAgentSettings declares. The evaluator's per-
// dispatch cap fields are left zero — ShouldStop falls back to the
// defaults when no orchestrator-set PlannerSoftIterCapOverride has been
// captured via BuildInitialInstruction. Tests that exercise the
// per-dispatch path call captureOverride() to install one.
func newPlannerEvaluatorForTest(t *testing.T) *plannerEvaluator {
	t.Helper()
	s := types.ResolvedAgentSettings(types.AgentSettings{})
	mu := types.NewMutableState("obj")
	return &plannerEvaluator{
		mu:                 mu,
		defaultSoftIterCap: s.PlannerSoftIterCap,
		defaultHardIterCap: s.PlannerHardIterCap,
	}
}

// effectiveSoftCap returns the cap ShouldStop will compare against —
// per-dispatch override when present, otherwise the agent-settings
// default. Mirrors the resolution order in plannerEvaluator.ShouldStop
// so tests assert on the same value the production code uses.
func (e *plannerEvaluator) effectiveSoftCap() int {
	if e.dispatchSoftIterCap > 0 && e.dispatchHardIterCap > e.dispatchSoftIterCap {
		return e.dispatchSoftIterCap
	}
	return e.defaultSoftIterCap
}

func (e *plannerEvaluator) effectiveHardCap() int {
	if e.dispatchSoftIterCap > 0 && e.dispatchHardIterCap > e.dispatchSoftIterCap {
		return e.dispatchHardIterCap
	}
	return e.defaultHardIterCap
}

// TestPlannerShouldStop_PlanInstalledStops pins the happy-path exit:
// the moment emit_change_plan installs a plan on Mutable, the loop
// terminates regardless of iteration count.
func TestPlannerShouldStop_PlanInstalledStops(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)
	e.mu.SetChangePlan(&types.ChangePlan{ID: "plan-x"})

	if !e.ShouldStop(llm.Response{}, 0) {
		t.Fatalf("expected immediate stop when ChangePlan is installed")
	}
}

// TestPlannerShouldStop_BelowSoftCapContinues confirms the soft-cap
// gate: under the soft cap, ShouldStop never terminates regardless of
// what the LLM is calling.
func TestPlannerShouldStop_BelowSoftCapContinues(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)

	for i := 0; i < e.effectiveSoftCap(); i++ {
		if e.ShouldStop(llm.Response{}, i) {
			t.Fatalf("expected continue at iter=%d (below soft cap %d)", i, e.effectiveSoftCap())
		}
	}
}

// TestPlannerShouldStop_SoftCapStopsWhenIdle pins the default
// soft-cap behaviour: at the soft cap, with no emit_change_plan in
// flight, the loop terminates.
func TestPlannerShouldStop_SoftCapStopsWhenIdle(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)

	resp := llm.Response{ToolCalls: []llm.ToolCall{
		{Name: "read_file"},
	}}
	if !e.ShouldStop(resp, e.effectiveSoftCap()) {
		t.Fatalf("expected stop at soft cap when LLM is not retrying emit_change_plan")
	}
}

// TestPlannerShouldStop_SoftCapAllowsEmitRetry locks in the recovery
// behaviour for streaming-truncation: at the soft cap, a clean retry
// of emit_change_plan must be allowed to execute. Without this the
// LLM's recovery emit is discarded and the truncation error surfaces
// to the user.
func TestPlannerShouldStop_SoftCapAllowsEmitRetry(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)

	resp := llm.Response{ToolCalls: []llm.ToolCall{
		{Name: emitChangePlanToolName},
	}}
	if e.ShouldStop(resp, e.effectiveSoftCap()) {
		t.Fatalf("expected continue at soft cap when LLM is retrying emit_change_plan")
	}
}

// TestPlannerShouldStop_SoftCapAllowsTypedEmitRepairRead pins the recovery
// shape observed in SWE-bench-style patches: after a structured emit validator
// rejects a plan, the next useful action may be a read-only diagnostic tool
// call to fetch exact bytes before the corrected emit can be formed.
func TestPlannerShouldStop_SoftCapAllowsTypedEmitRepairRead(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)
	e.ObserveToolResults(nil, LoopObservation{
		CurrentToolResults: []types.ToolResult{{
			ToolName: emitChangePlanToolName,
			Success:  false,
			Summary:  "arbitrary validator text",
		}},
	})

	resp := llm.Response{ToolCalls: []llm.ToolCall{
		{Name: "read_file"},
	}}
	if e.ShouldStop(resp, e.effectiveSoftCap()) {
		t.Fatalf("expected read_file to run inside the structured emit repair window")
	}
}

func TestPlannerShouldStop_TypedEmitRepairStillHardCaps(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)
	e.ObserveToolResults(nil, LoopObservation{
		CurrentToolResults: []types.ToolResult{{
			ToolName: emitChangePlanToolName,
			Success:  false,
		}},
	})

	resp := llm.Response{ToolCalls: []llm.ToolCall{
		{Name: "read_file"},
	}}
	if !e.ShouldStop(resp, e.effectiveHardCap()) {
		t.Fatalf("expected hard cap to stop even during structured emit repair")
	}
}

func TestPlannerShouldStop_HandoffSynthesisAllowsReadOnlyAtSoftCap(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)
	mu := types.NewMutableState("handoff synthesis")
	mu.SetWriteExplorationHandoff(&types.WriteExplorationHandoff{
		BatchID:     "batch-1",
		TargetFiles: []string{"pkg/fix.py"},
	})
	_ = e.BuildInitialInstruction(&types.AgentContext{Mutable: mu}, nil)

	resp := llm.Response{ToolCalls: []llm.ToolCall{
		{Name: "read_file"},
	}}
	if e.ShouldStop(resp, e.effectiveSoftCap()) {
		t.Fatalf("expected read_file to run inside handoff synthesis window")
	}
	if !e.ShouldStop(resp, e.effectiveHardCap()) {
		t.Fatalf("expected handoff synthesis window to stop at hard cap")
	}
}

func TestPlannerHandoffSynthesisReadBudget_ExplorationPackIsTight(t *testing.T) {
	mu := types.NewMutableState("handoff synthesis")
	mu.SetWriteContextPack(&types.WriteContextPack{
		SourceStage: "explore",
		Items: []types.WriteContextItem{
			{
				Priority:    types.WriteContextP1,
				Kind:        "target_file",
				Text:        "pkg/fix.py",
				SourceStage: "explore",
				Consumers:   []types.WriteContextConsumer{types.WriteConsumerPlanner},
			},
			{
				Priority:    types.WriteContextP1,
				Kind:        "evidence_ref",
				Text:        "pkg/fix.py:12",
				SourceStage: "explore",
				Consumers:   []types.WriteContextConsumer{types.WriteConsumerPlanner},
			},
		},
	})
	ctx := &types.AgentContext{Mutable: mu}

	if !plannerContextHasWriteHandoffMaterial(ctx) {
		t.Fatalf("explore-stage localization pack should activate handoff synthesis")
	}
	if got := plannerHandoffSynthesisReadBudget(ctx); got != plannerHandoffSynthesisBaseReadBudget {
		t.Fatalf("small localization pack budget = %d; want %d", got, plannerHandoffSynthesisBaseReadBudget)
	}
}

func TestPlannerHandoffSynthesisReadBudget_WriteAnalysisOnlyDoesNotActivate(t *testing.T) {
	mu := types.NewMutableState("handoff synthesis")
	mu.SetWriteContextPack(&types.WriteContextPack{
		SourceStage: "write_analysis",
		Items: []types.WriteContextItem{
			{
				Priority:    types.WriteContextP0,
				Kind:        "constraint",
				Text:        "preserve read scheduler byte identity",
				SourceStage: "write_analysis",
				Consumers:   []types.WriteContextConsumer{types.WriteConsumerPlanner},
			},
			{
				Priority:    types.WriteContextP1,
				Kind:        "scope_anchor",
				Text:        "internal/orchestrator",
				SourceStage: "write_analysis",
				Consumers:   []types.WriteContextConsumer{types.WriteConsumerPlanner},
			},
		},
	})
	ctx := &types.AgentContext{Mutable: mu}

	if plannerContextHasWriteHandoffMaterial(ctx) {
		t.Fatalf("write_analysis-only context should not activate exploration handoff synthesis")
	}
	if got := plannerHandoffSynthesisReadBudget(ctx); got != 0 {
		t.Fatalf("write_analysis-only budget = %d; want 0", got)
	}
}

func TestPlannerHandoffSynthesisReadBudget_NonLocalizingExploreItemsDoNotActivate(t *testing.T) {
	mu := types.NewMutableState("handoff synthesis")
	mu.SetWriteExplorationHandoff(&types.WriteExplorationHandoff{
		BatchID:    "batch-1",
		RiskNotes:  []string{"review generated dependency updates"},
		Unknowns:   []string{"which exact function owns this behavior"},
		Confidence: "medium",
	})
	ctx := &types.AgentContext{Mutable: mu}

	if plannerContextHasWriteHandoffMaterial(ctx) {
		t.Fatalf("risk/unknown-only exploration handoff should not activate exact-byte synthesis")
	}
	if got := plannerHandoffSynthesisReadBudget(ctx); got != 0 {
		t.Fatalf("risk/unknown-only budget = %d; want 0", got)
	}
}

func TestPlannerFilterToolSchemas_HandoffSynthesisExhaustsReadBudget(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)
	mu := types.NewMutableState("handoff synthesis")
	mu.SetWriteExplorationHandoff(&types.WriteExplorationHandoff{
		BatchID:     "batch-1",
		TargetFiles: []string{"xarray/core/variable.py"},
		EvidenceRefs: []types.WriteExplorationEvidenceRef{{
			Source:    "xarray/core/variable.py",
			LineStart: 239,
			Summary:   "DataArray.values is forced during Variable construction",
		}},
	})
	_ = e.BuildInitialInstruction(&types.AgentContext{Mutable: mu}, nil)
	schemas := []llm.ToolSchema{
		{Name: "read_file"},
		{Name: "grep"},
		{Name: "repo_map"},
		{Name: "run_tests"},
		{Name: emitChangePlanToolName},
		{Name: emitPlanSkeletonToolName},
		{Name: emitPlanChangeToolName},
	}

	if got := e.FilterToolSchemas(&types.AgentContext{Mutable: mu}, schemas); len(got) != len(schemas) {
		t.Fatalf("tool surface narrowed before read budget was spent: got %v", toolSchemaNamesForTest(got))
	}
	for i := 0; i < e.handoffSynthesisReadBudget; i++ {
		e.ObserveToolResults(nil, LoopObservation{
			CurrentToolResults: []types.ToolResult{{
				ToolName: "read_file",
				Success:  true,
			}},
		})
	}

	got := e.FilterToolSchemas(&types.AgentContext{Mutable: mu}, schemas)
	if names := strings.Join(toolSchemaNamesForTest(got), ","); names != "run_tests,emit_change_plan,emit_plan_skeleton,emit_plan_change" {
		t.Fatalf("narrowed tool surface = %s", names)
	}
	if !e.ShouldStop(llm.Response{ToolCalls: []llm.ToolCall{{Name: "read_file"}}}, e.effectiveSoftCap()) {
		t.Fatalf("read_file should not extend beyond soft cap after handoff synthesis read budget is exhausted")
	}
	if e.ShouldStop(llm.Response{ToolCalls: []llm.ToolCall{{Name: emitChangePlanToolName}}}, e.effectiveSoftCap()) {
		t.Fatalf("emit_change_plan should remain callable after read budget is exhausted")
	}
}

func TestPlannerFilterToolSchemas_StructuredEmitRepairKeepsReadTools(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)
	mu := types.NewMutableState("handoff synthesis")
	mu.SetWriteContextPack(&types.WriteContextPack{
		Items: []types.WriteContextItem{{
			Priority:    types.WriteContextP1,
			Kind:        "target_file",
			Text:        "pkg/fix.py",
			SourceStage: "explore",
		}},
	})
	_ = e.BuildInitialInstruction(&types.AgentContext{Mutable: mu}, nil)
	schemas := []llm.ToolSchema{
		{Name: "read_file"},
		{Name: "run_tests"},
		{Name: emitChangePlanToolName},
	}
	for i := 0; i < e.handoffSynthesisReadBudget; i++ {
		e.ObserveToolResults(nil, LoopObservation{
			CurrentToolResults: []types.ToolResult{{
				ToolName: "grep",
				Success:  true,
			}},
		})
	}
	e.ObserveToolResults(nil, LoopObservation{
		CurrentToolResults: []types.ToolResult{{
			ToolName: emitChangePlanToolName,
			Success:  false,
		}},
	})

	got := e.FilterToolSchemas(&types.AgentContext{Mutable: mu}, schemas)
	if names := strings.Join(toolSchemaNamesForTest(got), ","); names != "read_file,run_tests,emit_change_plan" {
		t.Fatalf("structured emit repair should keep exact-read tools available, got %s", names)
	}
	if e.ShouldStop(llm.Response{ToolCalls: []llm.ToolCall{{Name: "read_file"}}}, e.effectiveSoftCap()) {
		t.Fatalf("read_file should still run inside structured emit repair window")
	}
}

func TestPlannerFilterToolSchemas_StructuredEmitRepairReadBudgetNarrows(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)
	mu := types.NewMutableState("handoff synthesis")
	mu.SetWriteContextPack(&types.WriteContextPack{
		Items: []types.WriteContextItem{{
			Priority:    types.WriteContextP1,
			Kind:        "target_file",
			Text:        "pkg/fix.py",
			SourceStage: "explore",
		}},
	})
	_ = e.BuildInitialInstruction(&types.AgentContext{Mutable: mu}, nil)
	schemas := []llm.ToolSchema{
		{Name: "read_file"},
		{Name: "grep"},
		{Name: "run_tests"},
		{Name: emitChangePlanToolName},
		{Name: emitPlanSkeletonToolName},
		{Name: emitPlanChangeToolName},
	}
	for i := 0; i < e.handoffSynthesisReadBudget; i++ {
		e.ObserveToolResults(nil, LoopObservation{
			CurrentToolResults: []types.ToolResult{{
				ToolName: "grep",
				Success:  true,
			}},
		})
	}
	e.ObserveToolResults(nil, LoopObservation{
		CurrentToolResults: []types.ToolResult{{
			ToolName: emitChangePlanToolName,
			Success:  false,
		}},
	})
	for i := 0; i < plannerStructuredEmitRepairReadBudget; i++ {
		e.ObserveToolResults(nil, LoopObservation{
			CurrentToolResults: []types.ToolResult{{
				ToolName: "read_file",
				Success:  true,
			}},
		})
	}

	got := e.FilterToolSchemas(&types.AgentContext{Mutable: mu}, schemas)
	if names := strings.Join(toolSchemaNamesForTest(got), ","); names != "run_tests,emit_change_plan,emit_plan_skeleton,emit_plan_change" {
		t.Fatalf("structured repair exhausted tool surface = %s", names)
	}
	if !e.ShouldStop(llm.Response{ToolCalls: []llm.ToolCall{{Name: "read_file"}}}, e.effectiveSoftCap()) {
		t.Fatalf("read_file should not extend beyond soft cap after structured repair reads are exhausted")
	}
	if e.ShouldStop(llm.Response{ToolCalls: []llm.ToolCall{{Name: emitChangePlanToolName}}}, e.effectiveSoftCap()) {
		t.Fatalf("emit_change_plan should remain callable after structured repair read budget is exhausted")
	}
}

func TestPlannerObserve_MaterializationOnlyUnavailableReadHintsThenStops(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)
	mu := types.NewMutableState("handoff synthesis")
	mu.SetWriteContextPack(&types.WriteContextPack{
		Items: []types.WriteContextItem{{
			Priority:    types.WriteContextP1,
			Kind:        "target_file",
			Text:        "pkg/fix.py",
			SourceStage: "explore",
		}},
	})
	ctx := &types.AgentContext{Mutable: mu}
	_ = e.BuildInitialInstruction(ctx, nil)
	for i := 0; i < e.handoffSynthesisReadBudget; i++ {
		e.ObserveToolResults(ctx, LoopObservation{
			CurrentToolResults: []types.ToolResult{{
				ToolName: "grep",
				Success:  true,
			}},
		})
	}
	unavailable := types.ToolResult{
		ToolName: "read_file",
		Success:  false,
		Repair:   &types.ToolRepair{Code: unavailableToolSurfaceCode},
	}
	obs := LoopObservation{
		Phase:              PhaseMidLoop,
		CurrentToolResults: []types.ToolResult{unavailable},
		AvailableToolNames: map[string]bool{
			"run_tests":              true,
			emitChangePlanToolName:   true,
			emitPlanSkeletonToolName: true,
			emitPlanChangeToolName:   true,
		},
	}

	e.ObserveToolResults(ctx, obs)
	first := e.Observe(ctx, obs)
	if !first.HintRequested || first.HintKey != "planner.materialization-tool-surface" || first.StopRequested {
		t.Fatalf("first unavailable read should inject materialization hint, got %+v", first)
	}

	e.ObserveToolResults(ctx, obs)
	second := e.Observe(ctx, obs)
	if !second.StopRequested || second.HintRequested {
		t.Fatalf("second unavailable read should stop dirty dispatch, got %+v", second)
	}
}

func TestPlannerObserve_StructuredRepairUnavailableReadUsesDistinctHintKey(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)
	mu := types.NewMutableState("handoff synthesis")
	mu.SetWriteContextPack(&types.WriteContextPack{
		Items: []types.WriteContextItem{{
			Priority:    types.WriteContextP1,
			Kind:        "target_file",
			Text:        "pkg/fix.py",
			SourceStage: "explore",
		}},
	})
	ctx := &types.AgentContext{Mutable: mu}
	_ = e.BuildInitialInstruction(ctx, nil)
	for i := 0; i < e.handoffSynthesisReadBudget; i++ {
		e.ObserveToolResults(ctx, LoopObservation{
			CurrentToolResults: []types.ToolResult{{
				ToolName: "grep",
				Success:  true,
			}},
		})
	}
	unavailable := types.ToolResult{
		ToolName: "read_file",
		Success:  false,
		Repair:   &types.ToolRepair{Code: unavailableToolSurfaceCode},
	}
	obs := LoopObservation{
		Phase:              PhaseMidLoop,
		CurrentToolResults: []types.ToolResult{unavailable},
	}

	e.ObserveToolResults(ctx, obs)
	first := e.Observe(ctx, obs)
	if !first.HintRequested || first.HintKey != "planner.materialization-tool-surface" || first.StopRequested {
		t.Fatalf("first materialization violation should use ordinary hint key, got %+v", first)
	}

	e.ObserveToolResults(ctx, LoopObservation{
		CurrentToolResults: []types.ToolResult{{
			ToolName: emitChangePlanToolName,
			Success:  false,
		}},
	})
	for i := 0; i < plannerStructuredEmitRepairReadBudget; i++ {
		e.ObserveToolResults(ctx, LoopObservation{
			CurrentToolResults: []types.ToolResult{{
				ToolName: "grep",
				Success:  true,
			}},
		})
	}

	e.ObserveToolResults(ctx, obs)
	repair := e.Observe(ctx, obs)
	if !repair.HintRequested || repair.HintKey != "planner.structured-emit-repair-tool-surface" || repair.StopRequested {
		t.Fatalf("post-repair violation should use distinct hint key, got %+v", repair)
	}

	e.ObserveToolResults(ctx, obs)
	stop := e.Observe(ctx, obs)
	if !stop.StopRequested || stop.HintRequested {
		t.Fatalf("second post-repair violation should stop dirty dispatch, got %+v", stop)
	}
}

func TestPlannerShouldStop_BuildInitialInstructionClearsTypedEmitRepair(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)
	e.ObserveToolResults(nil, LoopObservation{
		CurrentToolResults: []types.ToolResult{{
			ToolName: emitChangePlanToolName,
			Success:  false,
		}},
	})

	_ = e.BuildInitialInstruction(&types.AgentContext{Mutable: e.mu}, nil)
	resp := llm.Response{ToolCalls: []llm.ToolCall{
		{Name: "read_file"},
	}}
	if !e.ShouldStop(resp, e.effectiveSoftCap()) {
		t.Fatalf("expected new dispatch to clear structured emit repair state")
	}
}

// TestPlannerShouldStop_HardCapAlwaysStops bounds the recovery window:
// even when the LLM keeps spamming emit_change_plan, the hard cap
// ensures the loop cannot run indefinitely.
func TestPlannerShouldStop_HardCapAlwaysStops(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)

	resp := llm.Response{ToolCalls: []llm.ToolCall{
		{Name: emitChangePlanToolName},
	}}
	if !e.ShouldStop(resp, e.effectiveHardCap()) {
		t.Fatalf("expected stop at hard cap regardless of tool calls")
	}
}

// TestPlannerShouldStop_DispatchOverrideRaisesCap pins the
// architectural fix for the cold-start STOP-at-iter=6 failure mode:
// when the orchestrator computes a per-dispatch cap based on
// analyzer signals (sub-topic count + complexity), the planner's
// ShouldStop must respect it instead of the construction-time
// default. Without this the inner cap stays at 6 regardless of
// task complexity and feature-add requests fail at iter=6 even
// when the analyzer correctly classified them as moderate/complex
// with multiple sub-topics.
func TestPlannerShouldStop_DispatchOverrideRaisesCap(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)

	// Simulate orchestrator setting PlannerSoftIterCapOverride based on
	// "moderate complexity, 3 sub-topics" — the exact shape of the
	// failing SSH/MCP scenario.
	ctx := &types.AgentContext{
		Mutable:                    e.mu,
		PlannerSoftIterCapOverride: 13, // base 6 + 3*3 + moderate +2 = 17 capped — pick a representative value
	}
	_ = e.BuildInitialInstruction(ctx, nil)

	if got := e.effectiveSoftCap(); got != 13 {
		t.Errorf("effective soft cap = %d; want 13 (per-dispatch override)", got)
	}
	// Recovery slack from defaults (default hard 9 - default soft 6 = 3).
	if got := e.effectiveHardCap(); got != 16 {
		t.Errorf("effective hard cap = %d; want 16 (override + recovery slack)", got)
	}

	// Old default cap (6) must NOT terminate the loop now.
	if e.ShouldStop(llm.Response{ToolCalls: []llm.ToolCall{{Name: "read_file"}}}, 6) {
		t.Errorf("ShouldStop fired at iter=6 even though per-dispatch cap is 13 (regression: hardcoded inner cap)")
	}
	if e.ShouldStop(llm.Response{ToolCalls: []llm.ToolCall{{Name: "read_file"}}}, 12) {
		t.Errorf("ShouldStop fired at iter=12 even though soft cap is 13")
	}
	// At soft cap with a non-recovery tool: stop.
	if !e.ShouldStop(llm.Response{ToolCalls: []llm.ToolCall{{Name: "read_file"}}}, 13) {
		t.Errorf("ShouldStop did NOT fire at soft cap 13 with non-recovery tool")
	}
	// At soft cap retrying emit_change_plan: continue.
	if e.ShouldStop(llm.Response{ToolCalls: []llm.ToolCall{{Name: emitChangePlanToolName}}}, 13) {
		t.Errorf("ShouldStop fired at soft cap 13 even though LLM is retrying emit_change_plan")
	}
	// Hard cap stops unconditionally.
	if !e.ShouldStop(llm.Response{ToolCalls: []llm.ToolCall{{Name: emitChangePlanToolName}}}, 16) {
		t.Errorf("ShouldStop did NOT fire at hard cap 16")
	}
}

// TestPlannerShouldStop_NoOverrideFallsBackToDefaults locks the
// degraded path: when no per-dispatch cap has been computed (zero
// PlannerSoftIterCapOverride — analyzer ran without IR, or unit-test bypass),
// ShouldStop reverts to the agent-settings defaults.
func TestPlannerShouldStop_NoOverrideFallsBackToDefaults(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)
	d := types.ResolvedAgentSettings(types.AgentSettings{})

	// No BuildInitialInstruction call — dispatch caps stay zero.
	if got := e.effectiveSoftCap(); got != d.PlannerSoftIterCap {
		t.Errorf("effective soft cap = %d; want default %d", got, d.PlannerSoftIterCap)
	}
	if got := e.effectiveHardCap(); got != d.PlannerHardIterCap {
		t.Errorf("effective hard cap = %d; want default %d", got, d.PlannerHardIterCap)
	}

	// Zero override + BuildInitialInstruction = same fallback.
	ctx := &types.AgentContext{Mutable: e.mu, PlannerSoftIterCapOverride: 0}
	_ = e.BuildInitialInstruction(ctx, nil)
	if got := e.effectiveSoftCap(); got != d.PlannerSoftIterCap {
		t.Errorf("effective soft cap after zero-override BuildInitialInstruction = %d; want default %d",
			got, d.PlannerSoftIterCap)
	}
}

// TestAgentSettings_IterCapInvariants is the structural invariant
// for every two-stage cap: hard MUST be > soft, otherwise the
// recovery window collapses to zero. Resolved defaults must satisfy
// this.
func TestAgentSettings_IterCapInvariants(t *testing.T) {
	s := types.ResolvedAgentSettings(types.AgentSettings{})
	cases := []struct {
		name string
		soft int
		hard int
	}{
		{"planner", s.PlannerSoftIterCap, s.PlannerHardIterCap},
		{"verifier", s.VerifierSoftIterCap, s.VerifierHardIterCap},
		{"extractor", s.ExtractorSoftIterCap, s.ExtractorHardIterCap},
	}
	for _, c := range cases {
		if c.hard <= c.soft {
			t.Errorf("%s: hard cap (%d) must be > soft cap (%d)", c.name, c.hard, c.soft)
		}
	}
	// Coder uses len(plan.TargetPaths)+slack as soft cap and
	// soft+recovery as hard cap; recovery > 0 is the invariant.
	if s.CoderHardIterRecovery <= 0 {
		t.Errorf("coder: recovery window (%d) must be > 0", s.CoderHardIterRecovery)
	}
}

// TestAgentSettings_RejectsCollapsedRecoveryWindow pins the resolver
// safety net: a misconfigured yaml that sets hard <= soft must fall
// back to defaults rather than producing a useless zero-width window.
func TestAgentSettings_RejectsCollapsedRecoveryWindow(t *testing.T) {
	d := types.DefaultAgentSettings()

	// Caller deliberately sets hard == soft.
	bad := types.AgentSettings{
		PlannerSoftIterCap: 8,
		PlannerHardIterCap: 8,
	}
	got := types.ResolvedAgentSettings(bad)
	if got.PlannerSoftIterCap != d.PlannerSoftIterCap || got.PlannerHardIterCap != d.PlannerHardIterCap {
		t.Errorf("expected fallback to defaults on collapsed window, got soft=%d hard=%d",
			got.PlannerSoftIterCap, got.PlannerHardIterCap)
	}
}

func TestBuildVerifyFailureHandoffSection_LeadsReplanPrompt(t *testing.T) {
	mu := types.NewMutableState("handoff render")
	mu.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{
		PlanID:      "plan-1",
		BatchID:     "batch-1",
		Attempt:     2,
		FailureKind: types.FailureKindTestsFailed,
		Executed: []types.ExecutedCommand{
			{Runner: "make", WorkingDir: ".", Command: "make check", ExitCode: 2},
		},
		FailingTests: []types.TestResult{
			{AssertionID: "TestAscii", Suite: "RandomStringUtilsTest", Passed: false, FailureDetail: "fast path must require end <= 0x7f"},
		},
		BuildErrors:            []types.BuildError{{File: "src/x.c", Line: 42, Message: "expected ;"}},
		FailureSummary:         "1 of 3 tests failed",
		BlobRef:                "/tmp/blob/run.txt",
		DiffArtifactRef:        "plan-1.attempt-1.diff",
		DiffArtifactPath:       "/tmp/codrax/plans/plan-1.attempt-1.diff",
		SurfaceArtifactRef:     "plan-1.attempt-1.surface.json",
		SurfaceArtifactPath:    "/tmp/codrax/plans/plan-1.attempt-1.surface.json",
		NextSurfaceCandidateID: "make@.",
	})
	eval := &plannerEvaluator{}
	ctx := &types.AgentContext{Mutable: mu}
	section := eval.buildVerifyFailureHandoffSection(ctx)
	for _, want := range []string{
		"## Latest verification failure (authoritative)",
		"plan: plan-1 attempt: 2 failure_kind: tests_failed",
		"command: `make check` (cwd=., exit=2, runner=make)",
		"failing_test: TestAscii (suite=RandomStringUtilsTest)",
		"build_error: src/x.c:42",
		"full runner output: /tmp/blob/run.txt",
		"previous attempt patch: plan-1.attempt-1.diff",
		"read_file path: /tmp/codrax/plans/plan-1.attempt-1.diff",
		"test surface artifact: plan-1.attempt-1.surface.json",
		"read_file path: /tmp/codrax/plans/plan-1.attempt-1.surface.json",
		"unexecuted runnable test candidate: make@.",
	} {
		if !strings.Contains(section, want) {
			t.Fatalf("section missing %q:\n%s", want, section)
		}
	}
	full := eval.BuildInitialInstruction(ctx, nil)
	idx := strings.Index(full, "## Latest verification failure (authoritative)")
	if idx < 0 {
		t.Fatalf("handoff section must render in the planner prompt:\n%s", full)
	}
	if mid := strings.Index(full, "## Task framing"); mid >= 0 && mid < idx {
		t.Fatalf("failure section must lead the prompt:\n%s", full)
	}
}

func TestBuildVerifyFailureHandoffSection_EmptyWithoutHandoff(t *testing.T) {
	mu := types.NewMutableState("no handoff")
	eval := &plannerEvaluator{}
	if got := eval.buildVerifyFailureHandoffSection(&types.AgentContext{Mutable: mu}); got != "" {
		t.Fatalf("no handoff must render nothing, got %q", got)
	}
}

func toolSchemaNamesForTest(schemas []llm.ToolSchema) []string {
	out := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		out = append(out, schema.Name)
	}
	return out
}
