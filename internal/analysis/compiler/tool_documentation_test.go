package compiler_test

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/binder"
	"github.com/hanchaoqun/codrax/internal/analysis/budget"
	"github.com/hanchaoqun/codrax/internal/analysis/compiler"
	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/analysis/gate"
	"github.com/hanchaoqun/codrax/internal/analysis/hdp"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestToolDocumentationCompilerHypothesesAndCriteria(t *testing.T) {
	for _, intent := range []types.Intent{types.IntentExplain, types.IntentEnumerate, types.IntentConfigQuery} {
		t.Run(string(intent), func(t *testing.T) {
			rm := types.RequestModel{RawRequest: "documented capabilities", Language: "en", Intent: intent, Scenario: types.ScenarioArchitectureExplain, Complexity: types.ComplexitySimple, ToolDocumentationRequest: &types.ToolDocumentationRequest{Scope: types.ToolDocumentationRequestOnly}}
			out := compiler.Compile(rm, budget.BudgetSignals{Complexity: rm.Complexity, HypothesisCount: 1})
			hyp := hdp.Plan(rm)
			if err := binder.BindByRelevance(&out.TaskGraph, hyp, binder.Options{}); err != nil {
				t.Fatal(err)
			}
			ir := &types.AnalysisIR{RequestModel: rm, TaskGraph: out.TaskGraph, EvidencePlan: out.EvidencePlan, AnswerContract: out.AnswerContract, HypothesisSet: hyp}
			if ir.AnswerContract.CitationReq.Required || ir.AnswerContract.CitationReq.MinCitations != 0 {
				t.Fatalf("pure docs requires fake repo citation: %+v", ir.AnswerContract)
			}
			if len(hyp) != 1 || len(hyp[0].RequiredEvidence) != 1 || hyp[0].RequiredEvidence[0].Kind != types.CritToolDocumentationReady {
				t.Fatalf("source hypotheses leaked into static documentation: %+v", hyp)
			}
			for _, n := range ir.TaskGraph.Nodes {
				for _, c := range append(append([]types.Criterion{}, n.EntryConditions...), n.SuccessCriteria...) {
					if c.Kind == types.CritEvidenceCount || c.Kind == types.CritCitationCountGE || c.Kind == types.CritNoCallSites {
						t.Fatalf("source obligation leaked: %+v", n)
					}
				}
			}
			for _, check := range gate.Run(ir, gate.Thresholds{}, "").Checks {
				if check.Name == "criterion_resolvable" || check.Name == "hypothesis_coverage" {
					if !check.Passed {
						t.Fatalf("domain IR rejected: %+v", check)
					}
				}
			}
			for _, ready := range []bool{false, true} {
				env := criterion.Env{IR: ir, ToolDocumentationReady: ready}
				if got := criterion.Eval(hyp[0].RequiredEvidence[0], env).Satisfied; got != ready {
					t.Fatalf("support ready=%t got %t", ready, got)
				}
				if criterion.Eval(hyp[0].FalsificationCondition, env).Satisfied {
					t.Fatal("zero source evidence falsely disproved documentation")
				}
			}
		})
	}
}

func TestToolDocumentationCompilerMixedKeepsSourceContract(t *testing.T) {
	rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain, Complexity: types.ComplexitySimple,
		ToolDocumentationRequest: &types.ToolDocumentationRequest{Scope: types.ToolDocumentationRequestMixed, DimensionIndices: []int{1}}, RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Index: 1, Required: true, Role: types.RequestedAnswerDimensionFunctionOrPurpose}, {Index: 2, Required: true, Role: types.RequestedAnswerDimensionCurrentKeyCode}}}}
	out := compiler.Compile(rm, budget.BudgetSignals{Complexity: rm.Complexity, HypothesisCount: 1})
	if !out.AnswerContract.CitationReq.Required || out.AnswerContract.CitationReq.MinCitations < 1 {
		t.Fatal("mixed erased source citations")
	}
	found := false
	for _, n := range out.TaskGraph.Nodes {
		if n.Type == types.NodeFinalize {
			for _, c := range n.SuccessCriteria {
				found = found || c.Kind == types.CritToolDocumentationReady
			}
		}
	}
	if !found {
		t.Fatal("mixed lost independent documentation delivery obligation")
	}
	if h := hdp.Plan(rm); len(h) == 0 || h[0].RequiredEvidence[0].Kind == types.CritToolDocumentationReady {
		t.Fatal("mixed replaced original source hypotheses")
	}
}
