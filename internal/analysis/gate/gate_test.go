package gate

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/binder"
	"github.com/hanchaoqun/codrax/internal/analysis/budget"
	"github.com/hanchaoqun/codrax/internal/analysis/compiler"
	"github.com/hanchaoqun/codrax/internal/analysis/hdp"
	"github.com/hanchaoqun/codrax/internal/types"
)

func sigFor(rm types.RequestModel) budget.BudgetSignals {
	return budget.BudgetSignals{
		Complexity:      rm.Complexity,
		TermCount:       len(rm.TermGraph.Canonical),
		HypothesisCount: 1,
		PrescanHitRatio: 1.0,
	}
}

// validIR builds a fully-realistic AnalysisIR by composing the real
// compiler / risk / hdp packages. This is the "golden path" input
// the gate must accept; individual tests mutate clones to test each
// failure mode in isolation. Using real packages here catches
// integration-level defects early.
func validIR() *types.AnalysisIR {
	rm := types.RequestModel{
		RawRequest: "explain explorer",
		Language:   "zh",
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Complexity: types.ComplexityModerate,
		TermGraph: types.TermGraph{
			Canonical: []types.CanonicalTerm{
				{ID: "code:explorer", Surface: "Explorer", Kind: types.TermSymbol, Confidence: 0.9},
				{ID: "code:shouldstop", Surface: "ShouldStop", Kind: types.TermSymbol, Confidence: 0.9},
			},
		},
	}
	out := compiler.Compile(rm, sigFor(rm))
	hs := hdp.Plan(rm)
	if err := binder.BindByRelevance(&out.TaskGraph, hs, binder.Options{}); err != nil {
		panic("gate test binder: " + err.Error())
	}
	ir := &types.AnalysisIR{
		Version:        types.AnalysisIRVersion,
		RequestModel:   rm,
		TaskGraph:      out.TaskGraph,
		EvidencePlan:   out.EvidencePlan,
		AnswerContract: out.AnswerContract,
		HypothesisSet:  hs,
	}
	return ir
}

func TestRun_GoldenPath_Passes(t *testing.T) {
	ir := validIR()
	report := Run(ir, Thresholds{}, "")
	if !report.Passed {
		t.Fatalf("golden IR should pass; report=%+v", report)
	}
	if report.Rejected {
		t.Fatalf("golden IR must not be rejected")
	}
	if len(report.Checks) < 5 {
		t.Fatalf("expected at least 5 checks; got %d", len(report.Checks))
	}
}

func TestRun_PendingFieldsWellformedSoftFailureAccepted(t *testing.T) {
	ir := validIR()
	if len(ir.TaskGraph.Nodes) == 0 {
		t.Fatal("validIR must include task nodes")
	}
	ir.TaskGraph.Nodes[0].Inputs = append(ir.TaskGraph.Nodes[0].Inputs, "Bad-Input")

	report := Run(ir, Thresholds{}, "")
	check := findCheck(report, "pending_fields_wellformed")
	if check == nil {
		t.Fatal("pending_fields_wellformed check missing")
	}
	if check.Passed {
		t.Fatalf("pending field check should record its own soft failure; got %+v", check)
	}
	if report.Rejected {
		t.Fatalf("pending field hygiene must not reject analyzer output; got %+v", report)
	}
	if !report.Passed {
		t.Fatalf("GateReport.Passed should mean hard-accepted; got %+v", report)
	}
	if report.Retryable {
		t.Fatalf("soft-only gate warning must not be retryable; got %+v", report)
	}
	if report.Fingerprint != "" {
		t.Fatalf("soft-only gate warning must not stamp retry fingerprint; got %q", report.Fingerprint)
	}
}

func TestRun_NilIR_RejectedNonRetryable(t *testing.T) {
	report := Run(nil, Thresholds{}, "")
	if !report.Rejected {
		t.Fatal("nil IR must be rejected")
	}
	if report.Retryable {
		t.Fatal("nil IR is not retryable")
	}
}

func findCheck(r types.GateReport, name string) *types.GateCheck {
	for i := range r.Checks {
		if r.Checks[i].Name == name {
			return &r.Checks[i]
		}
	}
	return nil
}

func TestRun_DAGClosure_DetectsDuplicateID(t *testing.T) {
	ir := validIR()
	// Duplicate node ID
	ir.TaskGraph.Nodes = append(ir.TaskGraph.Nodes, types.TaskNode{ID: ir.TaskGraph.Nodes[0].ID, Type: types.NodeEvidence})
	report := Run(ir, Thresholds{}, "")
	c := findCheck(report, "dag_closure")
	if c == nil || c.Passed {
		t.Fatalf("duplicate node id must fail dag_closure; got %+v", c)
	}
	if !strings.Contains(c.Detail, "duplicate") {
		t.Fatalf("detail should mention duplicate; got %q", c.Detail)
	}
}

func TestRun_DAGClosure_DetectsDanglingEdge(t *testing.T) {
	ir := validIR()
	ir.TaskGraph.Edges = append(ir.TaskGraph.Edges, types.TaskEdge{
		From: "does_not_exist", To: ir.TaskGraph.Nodes[0].ID, EdgeType: types.EdgeHardDependency,
	})
	report := Run(ir, Thresholds{}, "")
	if findCheck(report, "dag_closure").Passed {
		t.Fatal("dangling edge must fail dag_closure")
	}
}

func TestRun_DAGClosure_DetectsCycle(t *testing.T) {
	ir := validIR()
	// Introduce a hard-dependency cycle among existing nodes.
	first := ir.TaskGraph.Nodes[0].ID
	last := ir.TaskGraph.Nodes[len(ir.TaskGraph.Nodes)-1].ID
	ir.TaskGraph.Edges = append(ir.TaskGraph.Edges, types.TaskEdge{
		From: last, To: first, EdgeType: types.EdgeHardDependency,
	})
	report := Run(ir, Thresholds{}, "")
	if findCheck(report, "dag_closure").Passed {
		t.Fatal("hard-dependency cycle must fail dag_closure")
	}
}

func TestRun_DAGClosure_IgnoresValidationFeedbackCycles(t *testing.T) {
	// A root_cause graph is a natural test: it contains a
	// validation_feedback edge from validate → evidence and must
	// still pass the closure check.
	rm := types.RequestModel{
		Intent:   types.IntentRootCause,
		Scenario: types.ScenarioRootCause,
		TermGraph: types.TermGraph{Canonical: []types.CanonicalTerm{
			{ID: "code:explorer", Surface: "Explorer", Kind: types.TermSymbol, Confidence: 0.9},
		}},
	}
	out := compiler.Compile(rm, sigFor(rm))
	hs := hdp.Plan(rm)
	if err := binder.BindByRelevance(&out.TaskGraph, hs, binder.Options{}); err != nil {
		t.Fatalf("binder: %v", err)
	}
	ir := &types.AnalysisIR{
		Version: types.AnalysisIRVersion, RequestModel: rm,
		TaskGraph: out.TaskGraph, EvidencePlan: out.EvidencePlan,
		AnswerContract: out.AnswerContract, HypothesisSet: hs,
	}
	report := Run(ir, Thresholds{}, "")
	if !findCheck(report, "dag_closure").Passed {
		t.Fatalf("validation_feedback must not count as a cycle; got %+v", findCheck(report, "dag_closure"))
	}
}

func TestRun_BudgetTooSmall_Rejected(t *testing.T) {
	ir := validIR()
	ir.EvidencePlan.Budget.MaxFiles = 1
	ir.EvidencePlan.Budget.MaxReactIters = 1
	report := Run(ir, Thresholds{}, "")
	if findCheck(report, "budget_sanity").Passed {
		t.Fatal("tiny budget must fail")
	}
	if !report.Retryable {
		t.Fatal("budget failure should be retryable")
	}
}

func TestRun_BudgetTooLarge_Rejected(t *testing.T) {
	ir := validIR()
	ir.EvidencePlan.Budget.MaxFiles = 99999
	ir.EvidencePlan.Budget.MaxReactIters = 99999
	report := Run(ir, Thresholds{}, "")
	if findCheck(report, "budget_sanity").Passed {
		t.Fatal("absurd budget must fail")
	}
}

func TestRun_ContractMissingLanguage_Rejected(t *testing.T) {
	ir := validIR()
	ir.AnswerContract.Language = ""
	report := Run(ir, Thresholds{}, "")
	if findCheck(report, "contract_complete").Passed {
		t.Fatal("missing language must fail")
	}
}

func TestRun_ContractInvalidExactResolution_Rejected(t *testing.T) {
	ir := validIR()
	ir.AnswerContract.ExactResolution = &types.ExactResolutionContract{
		TargetLabel:          "config key",
		Targets:              []string{"foo.bar"},
		RelatedContextPolicy: "not_a_real_policy",
	}
	report := Run(ir, Thresholds{}, "")
	if findCheck(report, "contract_complete").Passed {
		t.Fatal("invalid exact-resolution policy must fail")
	}
}

func TestRun_Coverage_FailsWhenSymbolsMissing(t *testing.T) {
	ir := validIR()
	// Strip hints from all nodes so no symbol is covered.
	for i := range ir.TaskGraph.Nodes {
		ir.TaskGraph.Nodes[i].SearchHints = types.SearchHints{}
	}
	report := Run(ir, Thresholds{}, "")
	if findCheck(report, "coverage").Passed {
		t.Fatalf("coverage must fail when no symbols are hinted; report=%+v", report)
	}
}

func TestRun_HypothesisCoverage_FailsWhenNodeUnbound(t *testing.T) {
	ir := validIR()
	// Strip hypothesis bindings from every non-probe node.
	for i := range ir.TaskGraph.Nodes {
		if ir.TaskGraph.Nodes[i].Type != types.NodeProbe {
			ir.TaskGraph.Nodes[i].Hypotheses = nil
		}
	}
	report := Run(ir, Thresholds{}, "")
	if findCheck(report, "hypothesis_coverage").Passed {
		t.Fatal("unbound nodes must fail hypothesis_coverage")
	}
}

func TestRun_HypothesisCoverage_FailsWhenAllLowPriority(t *testing.T) {
	ir := validIR()
	for i := range ir.HypothesisSet {
		ir.HypothesisSet[i].Priority = 10
	}
	report := Run(ir, Thresholds{HypothesisMinPrio: 50}, "")
	if findCheck(report, "hypothesis_coverage").Passed {
		t.Fatal("low-priority-only hypotheses must fail")
	}
}

func TestRun_EmptyTaskGraph_FailsClosure(t *testing.T) {
	ir := validIR()
	ir.TaskGraph.Nodes = nil
	ir.TaskGraph.Edges = nil
	report := Run(ir, Thresholds{}, "")
	if findCheck(report, "dag_closure").Passed {
		t.Fatal("empty graph must fail")
	}
}
