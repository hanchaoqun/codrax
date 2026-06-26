package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// planner_multi_round_test.go covers the two-paths-into-one-slot
// upgrade landed alongside the empty-dir scaffold flow:
//
//  - ShouldStop now treats emit_plan_skeleton + emit_plan_change as
//    recovery tools on parity with emit_change_plan, so a multi-
//    round emission cannot be cut off at the soft cap.
//  - ParseOutput surfaces a structured "skeleton landed but X/Y
//    bodies missing" message when the loop exits with a partial
//    plan, so the verify→plan retry hint carries actionable signal
//    instead of a generic "did not emit" line.

// TestPlannerShouldStop_SoftCapAllowsSkeletonRetry — at the soft
// cap, a clean retry of emit_plan_skeleton must execute (same
// recovery contract as emit_change_plan in single-shot mode).
func TestPlannerShouldStop_SoftCapAllowsSkeletonRetry(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)
	resp := llm.Response{ToolCalls: []llm.ToolCall{
		{Name: emitPlanSkeletonToolName},
	}}
	if e.ShouldStop(resp, e.effectiveSoftCap()) {
		t.Fatalf("expected continue at soft cap when LLM is retrying %s", emitPlanSkeletonToolName)
	}
}

// TestPlannerShouldStop_SoftCapAllowsPlanChangeRetry — at the soft
// cap, a clean retry of emit_plan_change (per-file body fill) must
// also execute. This is the load-bearing case: in a multi-round
// emission the LLM is mid-stream of N per-file emits when the soft
// cap fires; if the recovery list omitted emit_plan_change, the
// emission would be discarded.
func TestPlannerShouldStop_SoftCapAllowsPlanChangeRetry(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)
	resp := llm.Response{ToolCalls: []llm.ToolCall{
		{Name: emitPlanChangeToolName},
	}}
	if e.ShouldStop(resp, e.effectiveSoftCap()) {
		t.Fatalf("expected continue at soft cap when LLM is calling %s", emitPlanChangeToolName)
	}
}

// TestPlannerParseOutput_PartialPlanReportsMissingBodies — when
// the loop exits with PartialChangePlan installed but file bodies
// still empty, ParseOutput surfaces a structured "X/Y still need
// contents" message naming the missing paths.
func TestPlannerParseOutput_PartialPlanReportsMissingBodies(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)
	// Skeleton with two create slots: one filled, one empty.
	partial := &types.ChangePlan{
		ID: "plan-partial-1",
		Changes: []types.FileChange{
			{Path: "main.go", Kind: "create", NewContent: "package main\n"},
			{Path: "go.mod", Kind: "create"}, // empty body — pending
		},
	}
	e.mu.SetPartialChangePlan(partial)
	// ChangePlan is intentionally NOT installed (PromotePartial never
	// fired), simulating the loop terminating mid-emission.

	ctx := &types.AgentContext{Mutable: e.mu}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err == nil {
		t.Fatal("ParseOutput should error when partial plan has missing bodies")
	}
	if out == nil {
		t.Fatal("ParseOutput must return a non-nil StageOutput even on error")
	}
	// Error should mention the missing path explicitly.
	if !strings.Contains(out.Error, "go.mod") {
		t.Errorf("error should name the missing path go.mod; got %q", out.Error)
	}
	// Progress fraction (1 still needed of 2 non-delete slots).
	if !strings.Contains(out.Error, "1") || !strings.Contains(out.Error, "2") {
		t.Errorf("error should expose the X/Y progress fraction; got %q", out.Error)
	}
	// User-facing — no internal tool names leak.
	for _, banned := range []string{
		"emit_change_plan", "emit_plan_skeleton", "emit_plan_change",
		"PartialChangePlan", "ChangePlan",
	} {
		if strings.Contains(out.Error, banned) {
			t.Errorf("ParseOutput error must not leak internal token %q; got %q", banned, out.Error)
		}
	}
	if out.MissingPiece != types.MissingFacts {
		t.Errorf("MissingPiece should be MissingFacts on partial-plan path; got %v", out.MissingPiece)
	}
}

// TestPlannerParseOutput_PartialPlanAllBodiesFilled — when the loop
// exits with every body present BUT promotion never fired, surface
// an internal-state-mismatch message (no tool names leaked).
func TestPlannerParseOutput_PartialPlanAllBodiesFilled(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)
	partial := &types.ChangePlan{
		ID: "plan-partial-complete",
		Changes: []types.FileChange{
			{Path: "main.go", Kind: "create", NewContent: "package main\n"},
		},
	}
	e.mu.SetPartialChangePlan(partial)

	ctx := &types.AgentContext{Mutable: e.mu}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err == nil {
		t.Fatal("ParseOutput should error when ChangePlan slot is empty despite filled bodies")
	}
	if !strings.Contains(out.Error, "never finalized") {
		t.Errorf("error should describe 'never finalized'; got %q", out.Error)
	}
	for _, banned := range []string{
		"emit_change_plan", "emit_plan_skeleton", "emit_plan_change",
		"PartialChangePlan",
	} {
		if strings.Contains(out.Error, banned) {
			t.Errorf("must not leak internal token %q; got %q", banned, out.Error)
		}
	}
}

func TestPlannerParseOutput_PartialPlanCompleteReportsLatestEmitRejection(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)
	partial := &types.ChangePlan{
		ID: "plan-partial-rejected",
		Changes: []types.FileChange{
			{
				Path:  "main.go",
				Kind:  "patch",
				Edits: []types.StructuredEdit{{Kind: "replace", StartLine: 1, Content: "package main\n"}},
			},
		},
	}
	e.mu.SetPartialChangePlan(partial)
	ctx := &types.AgentContext{Mutable: e.mu}

	out, err := e.ParseOutput(ctx, nil, []types.ToolResult{{
		ToolName: emitPlanChangeToolName,
		Success:  false,
		Summary:  "UNTRUSTED rendered validator summary should not enter ParseOutput error",
		Repair: plannerPlanRepairToolRepairForTest(types.PlanRepairPack{
			ToolName:          emitPlanChangeToolName,
			ReasonCode:        "old_text_mismatch",
			FailingFieldPaths: []string{"$.changes[0].edits[0].old_text"},
			FailingPaths:      []string{"main.go"},
			CurrentBytes: []types.PlanRepairCurrentBytes{{
				Path:      "main.go",
				StartLine: 1,
				EndLine:   1,
			}},
			RetryInstruction: "replace old_text with the exact current bytes from the pack",
		}),
	}}, nil)
	if err == nil {
		t.Fatal("ParseOutput should error when latest complete partial plan was rejected")
	}
	if out == nil {
		t.Fatal("ParseOutput must return a non-nil StageOutput even on error")
	}
	if !strings.Contains(out.Error, "old_text_mismatch") {
		t.Fatalf("error should preserve typed validator reason, got %q", out.Error)
	}
	if strings.Contains(out.Error, "UNTRUSTED") {
		t.Fatalf("error must not be built from rendered ToolResult.Summary: %q", out.Error)
	}
	if strings.Contains(out.Error, "never finalized") {
		t.Fatalf("validator rejection must not be misreported as internal mismatch: %q", out.Error)
	}
}

func TestPlannerParseOutput_StructuredEmitRejectWithoutTypedRepairDoesNotLeakSummary(t *testing.T) {
	e := newPlannerEvaluatorForTest(t)
	ctx := &types.AgentContext{Mutable: e.mu}

	out, err := e.ParseOutput(ctx, nil, []types.ToolResult{{
		ToolName: emitChangePlanToolName,
		Success:  false,
		Summary:  "old_text mismatch from rendered summary only",
	}}, nil)
	if err == nil {
		t.Fatal("ParseOutput should error when a plan emit was rejected")
	}
	if out == nil {
		t.Fatal("ParseOutput must return a non-nil StageOutput even on error")
	}
	if strings.Contains(out.Error, "old_text mismatch from rendered summary only") {
		t.Fatalf("summary-only rejection text must not become planner error: %q", out.Error)
	}
	if !strings.Contains(out.Error, "structured validation") {
		t.Fatalf("expected generic structured validation error, got %q", out.Error)
	}
}

func plannerPlanRepairToolRepairForTest(pack types.PlanRepairPack) *types.ToolRepair {
	normalized := types.NormalizePlanRepairPack(pack)
	return &types.ToolRepair{
		Code: types.PlanRepairToolCode,
		Metadata: map[string]string{
			types.PlanRepairMetadataKey: types.PlanRepairPackJSON(normalized),
		},
	}
}

// TestPlannerMissingBodies_Helper — direct unit test for the
// helper used by ParseOutput, so the internal contract is locked:
// only kinds that need a body (create/modify/patch with empty
// content) are returned; delete kinds never appear.
func TestPlannerMissingBodies_Helper(t *testing.T) {
	plan := &types.ChangePlan{
		Changes: []types.FileChange{
			{Path: "a.go", Kind: "create"},                                                  // missing
			{Path: "b.go", Kind: "modify", NewContent: "x"},                                 // filled
			{Path: "c.go", Kind: "patch"},                                                   // missing
			{Path: "d.go", Kind: "patch", Patch: "@@ -1 +1 @@"},                             // filled
			{Path: "e.go", Kind: "patch", Edits: []types.StructuredEdit{{Kind: "replace"}}}, // filled
			{Path: "f.go", Kind: "delete"},                                                  // body-trivial
		},
	}
	missing := plannerMissingBodies(plan)
	want := []string{"a.go", "c.go"}
	if len(missing) != len(want) {
		t.Fatalf("missing list len=%d want %d; got %v", len(missing), len(want), missing)
	}
	for i, p := range want {
		if missing[i] != p {
			t.Errorf("missing[%d]=%q want %q; full=%v", i, missing[i], p, missing)
		}
	}
	// Non-delete slot count: a, b, c, d, e (5); f is delete-trivial.
	if got := plannerNonDeleteSlotCount(plan); got != 5 {
		t.Errorf("plannerNonDeleteSlotCount = %d; want 5", got)
	}
}
