package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestPlannerTaskFraming_RendersIRFields verifies the planner reads
// WriteAnalysisIR fields and renders them in the prompt section.
// Each LLM-emit field (kind, scope, summary, risk, anchors,
// constraints, outcomes) should appear in the rendered text.
func TestPlannerTaskFraming_RendersIRFields(t *testing.T) {
	mu := types.NewMutableState("the user request")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			RawRequest: "the user request",
			Task: types.WriteTask{
				Kind:    types.WriteTaskFeature,
				Scope:   types.ScopeMicro,
				Summary: "wire a --quiet flag through cmd/root.go",
			},
			Risk: types.WriteRiskProfile{
				AffectsPublicAPI:   true,
				ChangesBuildSystem: false,
				Overall:            types.RiskBandLow,
			},
			ScopeAnchors: []string{"cmd/root.go", "internal/render"},
			Constraints: []types.WriteConstraint{
				{Kind: "preserve_api", Target: "*.Public", Note: "do not break existing flags"},
			},
			ExpectedOutcomes: []string{
				"the --quiet flag suppresses INFO output",
				"existing tests still pass",
			},
		},
	})
	ctx := &types.AgentContext{Mutable: mu}
	eval := &plannerEvaluator{}
	got := eval.buildTaskFramingSection(ctx)
	for _, want := range []string{
		"## Task framing",
		"wire a --quiet flag through cmd/root.go",
		"feature",
		"micro",
		"low",
		"affects-public-api",
		"cmd/root.go, internal/render",
		"preserve_api",
		"do not break existing flags",
		"the --quiet flag suppresses INFO output",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered task framing missing %q; got:\n%s", want, got)
		}
	}
}

// TestPlannerTaskFraming_NoIRReturnsEmpty pins the degraded path:
// when WriteAnalysisIR is absent (write_analyzer didn't run or
// degraded), the planner falls through with no task-framing
// section, equivalent to pre-commit-1 behaviour.
func TestPlannerTaskFraming_NoIRReturnsEmpty(t *testing.T) {
	mu := types.NewMutableState("the user request")
	ctx := &types.AgentContext{Mutable: mu}
	eval := &plannerEvaluator{}
	if got := eval.buildTaskFramingSection(ctx); got != "" {
		t.Errorf("nil IR should yield empty section; got %q", got)
	}
}

// TestPlannerTaskFraming_NilCtxSafe pins defensive guards.
func TestPlannerTaskFraming_NilCtxSafe(t *testing.T) {
	eval := &plannerEvaluator{}
	if got := eval.buildTaskFramingSection(nil); got != "" {
		t.Errorf("nil ctx should yield empty section; got %q", got)
	}
}
