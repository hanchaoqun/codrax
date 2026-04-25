package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// coderFixtureCtxBaseline builds the AgentContext shape coder.BuildInitialInstruction
// reads. plan is mandatory; baseline is optional and goes through
// Mutable.SetBaselineReport so the coder's new baseline-aware path
// can render its section.
func coderFixtureCtxBaseline(plan *types.ChangePlan, baseline *types.ChangeReport) *types.AgentContext {
	mu := types.NewMutableState("test")
	if plan != nil {
		mu.SetChangePlan(plan)
	}
	if baseline != nil {
		mu.SetBaselineReport(baseline)
	}
	return &types.AgentContext{
		AgentName: types.AgentCoder,
		Stage:     types.StageApply,
		Objective: "apply plan",
		Mutable:   mu,
	}
}

// Baseline absent (capture disabled or no test runner) → coder
// prompt has zero baseline content. Existing dumb-marshaller shape
// preserved.
func TestCoder_BuildInitialInstruction_NoBaselineNoSection(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-no-baseline",
		Changes:     []types.FileChange{{Path: "a.go", Kind: "modify"}},
		TargetPaths: []string{"a.go"},
	}
	ctx := coderFixtureCtxBaseline(plan, nil)
	ev := &coderEvaluator{}
	inst := ev.BuildInitialInstruction(ctx, &skill.Config{})
	if strings.Contains(inst, "Pre-existing failures") || strings.Contains(inst, "Baseline status") {
		t.Errorf("absent baseline should not render any baseline section; got %q", inst)
	}
	// Apply plan section still present (regression check on the
	// existing dumb-marshaller path).
	if !strings.Contains(inst, "## Apply plan") {
		t.Errorf("apply-plan header missing; got %q", inst)
	}
}

// Baseline present with failures → coder prompt lists pre-existing
// failures + acceptance tests + the "stay within target paths"
// instruction.
func TestCoder_BuildInitialInstruction_BaselineFailuresInjected(t *testing.T) {
	plan := &types.ChangePlan{
		ID:              "plan-baseline-fail",
		Changes:         []types.FileChange{{Path: "a.go", Kind: "modify"}},
		TargetPaths:     []string{"a.go"},
		AcceptanceTests: []string{"the new feature works"},
	}
	baseline := &types.ChangeReport{
		TestResults: []types.TestResult{
			{Kind: types.TestResultKindUnit, AssertionID: "TestOldBroken", Suite: "legacy", Passed: false},
			{Kind: types.TestResultKindUnit, AssertionID: "TestStillFine", Passed: true},
		},
	}
	ctx := coderFixtureCtxBaseline(plan, baseline)
	ev := &coderEvaluator{}
	inst := ev.BuildInitialInstruction(ctx, &skill.Config{})
	if !strings.Contains(inst, "## Pre-existing failures (out of scope)") {
		t.Errorf("missing pre-existing-failures header; got %q", inst)
	}
	if !strings.Contains(inst, "TestOldBroken") {
		t.Errorf("baseline failure name not surfaced; got %q", inst)
	}
	if strings.Contains(inst, "TestStillFine") {
		t.Errorf("passing baseline tests must not appear; got %q", inst)
	}
	if !strings.Contains(inst, "the new feature works") {
		t.Errorf("acceptance tests should accompany the baseline section; got %q", inst)
	}
	if !strings.Contains(inst, "plan.TargetPaths") {
		t.Errorf("scope-stay instruction missing; got %q", inst)
	}
}

// Baseline present + all green → coder prompt renders a positive
// "any new failure is a regression" signal instead of pre-existing
// failures.
func TestCoder_BuildInitialInstruction_BaselineGreenSignal(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-green",
		Changes:     []types.FileChange{{Path: "a.go", Kind: "modify"}},
		TargetPaths: []string{"a.go"},
	}
	baseline := &types.ChangeReport{
		Passed: true,
		TestResults: []types.TestResult{
			{Kind: types.TestResultKindUnit, AssertionID: "TestOK1", Passed: true},
			{Kind: types.TestResultKindUnit, AssertionID: "TestOK2", Passed: true},
		},
	}
	ctx := coderFixtureCtxBaseline(plan, baseline)
	ev := &coderEvaluator{}
	inst := ev.BuildInitialInstruction(ctx, &skill.Config{})
	if strings.Contains(inst, "Pre-existing failures") {
		t.Errorf("clean baseline should NOT render pre-existing-failures section; got %q", inst)
	}
	if !strings.Contains(inst, "## Baseline status") {
		t.Errorf("clean baseline should render the baseline-status header; got %q", inst)
	}
	if !strings.Contains(inst, "fully green") {
		t.Errorf("status text should mention green baseline; got %q", inst)
	}
	if !strings.Contains(inst, "regression caused by this plan") {
		t.Errorf("status text should warn about regressions; got %q", inst)
	}
}

// All applied + baseline still set: coder returns the early-exit
// "all already applied" message, baseline section is a no-op (the
// coder has nothing to do).
func TestCoder_BuildInitialInstruction_AlreadyAppliedSkipsBaseline(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-applied",
		Changes:     []types.FileChange{{Path: "a.go", Kind: "modify"}},
		TargetPaths: []string{"a.go"},
	}
	ctx := coderFixtureCtxBaseline(plan, &types.ChangeReport{
		TestResults: []types.TestResult{{AssertionID: "TestX", Passed: false}},
	})
	// Mark a.go as already applied via WriteClosure.
	ctx.Mutable.WriteClosure().MarkApplied("a.go")
	ev := &coderEvaluator{}
	inst := ev.BuildInitialInstruction(ctx, &skill.Config{})
	if !strings.Contains(inst, "All plan changes are already applied") {
		t.Errorf("early-exit message missing; got %q", inst)
	}
	// Pre-existing-failures section MUST NOT render — the coder has
	// nothing to do, so baseline guidance is noise.
	if strings.Contains(inst, "Pre-existing failures") {
		t.Errorf("baseline section should not render on early-exit path; got %q", inst)
	}
}
