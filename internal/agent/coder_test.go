package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// coderFixtureCtx builds an AgentContext with a plan + the
// Mutable state the evaluator captures in BuildInitialInstruction.
// Kept minimal so every test stays readable.
func coderFixtureCtx(plan *types.ChangePlan) *types.AgentContext {
	mu := types.NewMutableState("test")
	if plan != nil {
		mu.SetChangePlan(plan)
	}
	return &types.AgentContext{
		AgentName: types.AgentCoder,
		Stage:     types.StageApply,
		Objective: "apply plan",
		Mutable:   mu,
	}
}

// TestCoder_ShouldStop_AllApplied verifies stop fires as soon as
// every target path is applied.
func TestCoder_ShouldStop_AllApplied(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan",
		Changes:     []types.FileChange{{Path: "a.go", Kind: "create"}, {Path: "b.go", Kind: "create"}},
		TargetPaths: []string{"a.go", "b.go"},
	}
	ctx := coderFixtureCtx(plan)
	ev := &coderEvaluator{}
	ev.BuildInitialInstruction(ctx, &skill.Config{}) // capture mu

	if ev.ShouldStop(llm.Response{}, 0) {
		t.Error("should NOT stop when nothing applied yet")
	}
	ctx.Mutable.WriteClosure().MarkApplied("a.go")
	if ev.ShouldStop(llm.Response{}, 1) {
		t.Error("should NOT stop with partial apply")
	}
	ctx.Mutable.WriteClosure().MarkApplied("b.go")
	if !ev.ShouldStop(llm.Response{}, 2) {
		t.Error("should stop when all target paths applied")
	}
}

// TestCoder_ShouldStop_IterationCap verifies the iteration cap
// prevents runaway loops when the LLM never makes progress.
func TestCoder_ShouldStop_IterationCap(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan",
		Changes:     []types.FileChange{{Path: "x.go", Kind: "create"}},
		TargetPaths: []string{"x.go"},
	}
	ctx := coderFixtureCtx(plan)
	s := types.ResolvedAgentSettings(types.AgentSettings{})
	ev := &coderEvaluator{
		softIterSlack:    s.CoderSoftIterSlack,
		hardIterRecovery: s.CoderHardIterRecovery,
	}
	ev.BuildInitialInstruction(ctx, &skill.Config{})

	// Soft cap = len(TargetPaths) + slack. Below soft cap, no stop.
	softCap := len(plan.TargetPaths) + s.CoderSoftIterSlack
	for i := 0; i < softCap; i++ {
		if ev.ShouldStop(llm.Response{}, i) {
			t.Errorf("iter %d: should not stop (below soft cap %d, nothing applied)", i, softCap)
		}
	}
	// At/past soft cap with no apply_patch in flight → stop.
	if !ev.ShouldStop(llm.Response{}, softCap) {
		t.Errorf("iter %d: should stop at soft cap when LLM idle", softCap)
	}
	// Hard cap stops unconditionally.
	hardCap := softCap + s.CoderHardIterRecovery
	if !ev.ShouldStop(llm.Response{ToolCalls: []llm.ToolCall{{Name: applyPatchToolName}}}, hardCap) {
		t.Errorf("iter %d: should stop at hard cap regardless of tool calls", hardCap)
	}
}

// TestCoder_ShouldStop_NoPlan verifies the evaluator exits
// immediately when no plan is installed.
func TestCoder_ShouldStop_NoPlan(t *testing.T) {
	ctx := coderFixtureCtx(nil)
	ev := &coderEvaluator{}
	ev.BuildInitialInstruction(ctx, &skill.Config{})
	if !ev.ShouldStop(llm.Response{}, 0) {
		t.Error("should stop immediately when plan is nil")
	}
}

// TestCoder_ParseOutput_Complete verifies the success path.
func TestCoder_ParseOutput_Complete(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan",
		Changes:     []types.FileChange{{Path: "a.go", Kind: "create"}},
		TargetPaths: []string{"a.go"},
	}
	ctx := coderFixtureCtx(plan)
	ctx.Mutable.WriteClosure().MarkApplied("a.go")
	ev := &coderEvaluator{}
	ev.BuildInitialInstruction(ctx, &skill.Config{})

	out, err := ev.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out.Error != "" {
		t.Errorf("expected no error, got %q", out.Error)
	}
	if out.MissingPiece != types.MissingNone {
		t.Errorf("MissingPiece = %q, want MissingNone", out.MissingPiece)
	}
}

// TestCoder_ParseOutput_Incomplete verifies missing paths are
// reported in the error message.
func TestCoder_ParseOutput_Incomplete(t *testing.T) {
	plan := &types.ChangePlan{
		ID: "plan",
		Changes: []types.FileChange{
			{Path: "a.go", Kind: "create"},
			{Path: "b.go", Kind: "create"},
		},
		TargetPaths: []string{"a.go", "b.go"},
	}
	ctx := coderFixtureCtx(plan)
	ctx.Mutable.WriteClosure().MarkApplied("a.go") // only half applied
	ev := &coderEvaluator{}
	ev.BuildInitialInstruction(ctx, &skill.Config{})

	out, err := ev.ParseOutput(ctx, nil, nil, nil)
	if err == nil {
		t.Fatal("ParseOutput should error on incomplete apply")
	}
	if !strings.Contains(out.Error, "b.go") {
		t.Errorf("error should list missing path 'b.go'; got %q", out.Error)
	}
	if !strings.Contains(out.Error, "1 of 2") {
		t.Errorf("error should report count; got %q", out.Error)
	}
	if out.MissingPiece != types.MissingFacts {
		t.Errorf("MissingPiece should be MissingFacts; got %q", out.MissingPiece)
	}
}

// TestCoder_ParseOutput_CarriesLastToolError verifies the most
// recent apply_patch rejection is surfaced as context in the error.
func TestCoder_ParseOutput_CarriesLastToolError(t *testing.T) {
	plan := &types.ChangePlan{
		ID: "plan",
		Changes: []types.FileChange{
			{Path: "a.go", Kind: "create"},
		},
		TargetPaths: []string{"a.go"},
	}
	ctx := coderFixtureCtx(plan)
	ev := &coderEvaluator{}
	ev.BuildInitialInstruction(ctx, &skill.Config{})

	toolResults := []types.ToolResult{
		{ToolName: "apply_patch", Success: false, Summary: "W1 violation: path 'evil.go' not in plan"},
	}
	out, err := ev.ParseOutput(ctx, nil, toolResults, nil)
	if err == nil {
		t.Fatal("should error on incomplete apply")
	}
	if !strings.Contains(out.Error, "W1 violation") {
		t.Errorf("error should carry last apply_patch rejection; got %q", out.Error)
	}
}

// TestCoder_BuildInitialInstruction verifies the dynamic prompt
// supplement lists remaining paths.
func TestCoder_BuildInitialInstruction(t *testing.T) {
	plan := &types.ChangePlan{
		ID: "plan-inst",
		Changes: []types.FileChange{
			{Path: "foo.go", Kind: "create", NewContent: "x"},
			{Path: "bar.go", Kind: "modify", NewContent: "y",
				DependsOn: []string{"foo.go"}},
		},
		TargetPaths: []string{"foo.go", "bar.go"},
	}
	ctx := coderFixtureCtx(plan)
	ev := &coderEvaluator{}
	inst := ev.BuildInitialInstruction(ctx, &skill.Config{})
	if !strings.Contains(inst, "foo.go") {
		t.Errorf("instruction should list foo.go; got %q", inst)
	}
	if !strings.Contains(inst, "bar.go") {
		t.Errorf("instruction should list bar.go; got %q", inst)
	}
	if !strings.Contains(inst, "depends_on") {
		t.Errorf("instruction should mention depends_on; got %q", inst)
	}
	if !strings.Contains(inst, "plan-inst") {
		t.Errorf("instruction should name the plan ID; got %q", inst)
	}
}

// TestCoder_BuildInitialInstruction_NoPlan verifies graceful msg
// when no plan is available.
func TestCoder_BuildInitialInstruction_NoPlan(t *testing.T) {
	ctx := coderFixtureCtx(nil)
	ev := &coderEvaluator{}
	inst := ev.BuildInitialInstruction(ctx, &skill.Config{})
	if !strings.Contains(inst, "No ChangePlan") {
		t.Errorf("should explain missing plan; got %q", inst)
	}
}

// TestCoder_HelpersDirect covers the internal helpers independently.
func TestCoder_HelpersDirect(t *testing.T) {
	plan := &types.ChangePlan{
		ID: "p",
		Changes: []types.FileChange{
			{Path: "a"}, {Path: "b"}, {Path: "c"},
		},
		TargetPaths: []string{"a", "b", "c"},
	}
	applied := map[string]bool{"a": true}

	if isAppliedComplete(plan, applied) {
		t.Error("not complete when only 1 of 3 applied")
	}
	applied["b"] = true
	applied["c"] = true
	if !isAppliedComplete(plan, applied) {
		t.Error("complete when all applied")
	}

	pending := pendingPaths(plan, map[string]bool{"a": true})
	if len(pending) != 2 || pending[0] != "b" || pending[1] != "c" {
		t.Errorf("pendingPaths = %v, want [b c]", pending)
	}

	unapplied := orderedUnappliedChanges(plan, map[string]bool{"b": true})
	if len(unapplied) != 2 {
		t.Errorf("orderedUnapplied len = %d, want 2", len(unapplied))
	}
	if unapplied[0].Path != "a" || unapplied[1].Path != "c" {
		t.Errorf("orderedUnapplied paths = %v %v, want a c",
			unapplied[0].Path, unapplied[1].Path)
	}
}
