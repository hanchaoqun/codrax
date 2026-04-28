package compiler

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/budget"
	"github.com/hanchaoqun/codrax/internal/types"
)

// compileT is the test helper that mirrors Compile with a default
// BudgetSignals. Keeps the test call sites terse after the Compile
// signature gained the signals parameter.
func compileT(rm types.RequestModel) Output {
	return Compile(rm, budget.BudgetSignals{
		Complexity:      rm.Complexity,
		TermCount:       len(rm.TermGraph.Canonical),
		HypothesisCount: 1,
		PrescanHitRatio: 1.0,
	})
}

func sampleRM(scenario types.Scenario, intent types.Intent, complexity types.Complexity) types.RequestModel {
	return types.RequestModel{
		RawRequest: "sample",
		Language:   "zh",
		Intent:     intent,
		Scenario:   scenario,
		Complexity: complexity,
		TermGraph: types.TermGraph{
			Canonical: []types.CanonicalTerm{
				{ID: "code:explorer", Surface: "Explorer", Language: "code", Kind: types.TermSymbol},
				{ID: "en:stop", Surface: "stop", Language: "en", Kind: types.TermConcept},
				{ID: "cfg:config/orchestrator.yaml", Surface: "config/orchestrator.yaml", Language: "code", Kind: types.TermConfig},
			},
		},
	}
}

// countNodeType returns how many nodes of the given type exist in g.
func countNodeType(g types.TaskGraph, t types.TaskNodeType) int {
	n := 0
	for _, node := range g.Nodes {
		if node.Type == t {
			n++
		}
	}
	return n
}

func hasEdge(g types.TaskGraph, from, to string, et types.EdgeType) bool {
	for _, e := range g.Edges {
		if e.From == from && e.To == to && e.EdgeType == et {
			return true
		}
	}
	return false
}

func TestCompile_ArchitectureExplain_HasExplanationContract(t *testing.T) {
	out := compileT(sampleRM(types.ScenarioArchitectureExplain, types.IntentExplain, types.ComplexityModerate))
	if out.AnswerContract.RequiredAnswerShape != types.ShapeExplanation {
		t.Fatalf("shape=%q", out.AnswerContract.RequiredAnswerShape)
	}
	if countNodeType(out.TaskGraph, types.NodeProbe) != 1 {
		t.Fatalf("want 1 probe node")
	}
	if countNodeType(out.TaskGraph, types.NodeFinalize) != 1 {
		t.Fatalf("want 1 finalize node")
	}
	// All edges must reference nodes that exist.
	ids := make(map[string]bool)
	for _, n := range out.TaskGraph.Nodes {
		ids[n.ID] = true
	}
	for _, e := range out.TaskGraph.Edges {
		if !ids[e.From] || !ids[e.To] {
			t.Fatalf("dangling edge %+v", e)
		}
	}
}

func TestCompile_RootCause_HasValidationFeedback(t *testing.T) {
	out := compileT(sampleRM(types.ScenarioRootCause, types.IntentRootCause, types.ComplexityComplex))
	if out.AnswerContract.RequiredAnswerShape != types.ShapeStepList {
		t.Fatalf("shape=%q", out.AnswerContract.RequiredAnswerShape)
	}
	// validate → evidence feedback edge must exist.
	var validateID, evidenceID string
	for _, n := range out.TaskGraph.Nodes {
		if n.Type == types.NodeValidate {
			validateID = n.ID
		}
		if n.Type == types.NodeEvidence {
			evidenceID = n.ID
		}
	}
	if validateID == "" || evidenceID == "" {
		t.Fatalf("missing validate or evidence node")
	}
	if !hasEdge(out.TaskGraph, validateID, evidenceID, types.EdgeValidationFeedback) {
		t.Fatalf("expected validation_feedback edge %s→%s; edges=%+v", validateID, evidenceID, out.TaskGraph.Edges)
	}
}

func TestCompile_ConfigTrace_ShapeConfigValue(t *testing.T) {
	out := compileT(sampleRM(types.ScenarioConfigTrace, types.IntentConfigQuery, types.ComplexitySimple))
	if out.AnswerContract.RequiredAnswerShape != types.ShapeConfigValue {
		t.Fatalf("shape=%q", out.AnswerContract.RequiredAnswerShape)
	}
	// Simple complexity must give a smaller budget than moderate.
	moderate := compileT(sampleRM(types.ScenarioConfigTrace, types.IntentConfigQuery, types.ComplexityModerate))
	if out.EvidencePlan.Budget.MaxFiles >= moderate.EvidencePlan.Budget.MaxFiles {
		t.Fatalf("simple budget %d should be < moderate %d",
			out.EvidencePlan.Budget.MaxFiles, moderate.EvidencePlan.Budget.MaxFiles)
	}
}

func TestCompile_PerformanceBottleneck_ListOfSymbols(t *testing.T) {
	out := compileT(sampleRM(types.ScenarioPerformanceBottleneck, types.IntentTrace, types.ComplexityModerate))
	if out.AnswerContract.RequiredAnswerShape != types.ShapeListOfSymbols {
		t.Fatalf("shape=%q", out.AnswerContract.RequiredAnswerShape)
	}
}

func TestCompile_Generic_ReactsToIntent(t *testing.T) {
	cases := []struct {
		intent types.Intent
		want   types.AnswerShape
	}{
		{types.IntentExplain, types.ShapeExplanation},
		{types.IntentReturnValue, types.ShapeValue},
		{types.IntentEnumerate, types.ShapeListOfSymbols},
		{types.IntentConfigQuery, types.ShapeConfigValue},
	}
	for _, c := range cases {
		out := compileT(sampleRM(types.ScenarioGeneric, c.intent, types.ComplexityModerate))
		if out.AnswerContract.RequiredAnswerShape != c.want {
			t.Fatalf("intent=%s: want shape=%s got %s", c.intent, c.want, out.AnswerContract.RequiredAnswerShape)
		}
	}
}

func TestCompile_StructuralTraceUsesTraceWalkthroughTemplate(t *testing.T) {
	rm := sampleRM(types.ScenarioArchitectureExplain, types.IntentTrace, types.ComplexityModerate)
	rm.PredicateAxis = types.AxisCall
	rm.AnalyzerHints.Kind = "call_chain"

	out := compileT(rm)
	if countNodeType(out.TaskGraph, types.NodeReconcile) != 0 {
		t.Fatalf("single-topic structural trace should not allocate a reconcile node; graph=%+v", out.TaskGraph.Nodes)
	}
	if countNodeType(out.TaskGraph, types.NodeValidate) != 1 {
		t.Fatalf("single-topic structural trace should keep one validate node")
	}
	if out.AnswerContract.CitationReq.MinCitations != TmplCitationCountMedium {
		t.Fatalf("citation floor = %d, want %d", out.AnswerContract.CitationReq.MinCitations, TmplCitationCountMedium)
	}
}

func TestCompile_CrossComponentTraceKeepsArchitectureTemplate(t *testing.T) {
	rm := sampleRM(types.ScenarioArchitectureExplain, types.IntentTrace, types.ComplexityModerate)
	rm.PredicateAxis = types.AxisCall
	rm.AnalyzerHints.Kind = "call_chain"
	rm.Predicates.IsCrossComponent = true

	out := compileT(rm)
	if countNodeType(out.TaskGraph, types.NodeReconcile) != 1 {
		t.Fatalf("cross-component trace should keep reconcile node")
	}
}

func TestCompile_UnknownScenarioFallsBackToGeneric(t *testing.T) {
	out := compileT(sampleRM("no_such_scenario", types.IntentExplain, types.ComplexityModerate))
	// Generic template has exactly 3 nodes: probe, evidence, finalize.
	if len(out.TaskGraph.Nodes) != 3 {
		t.Fatalf("unknown scenario should fall back to generic (3 nodes); got %d", len(out.TaskGraph.Nodes))
	}
}

func TestCompile_HintsPropagateFromTermGraph(t *testing.T) {
	out := compileT(sampleRM(types.ScenarioArchitectureExplain, types.IntentExplain, types.ComplexityModerate))
	foundCodeHint := false
	for _, n := range out.TaskGraph.Nodes {
		for _, id := range n.SearchHints.EntityIDs {
			if id == "code:explorer" {
				foundCodeHint = true
			}
		}
	}
	if !foundCodeHint {
		t.Fatalf("code:explorer must reach node search hints")
	}
}

func TestCompile_ComplexityScalesBudget(t *testing.T) {
	simple := compileT(sampleRM(types.ScenarioArchitectureExplain, types.IntentExplain, types.ComplexitySimple))
	moderate := compileT(sampleRM(types.ScenarioArchitectureExplain, types.IntentExplain, types.ComplexityModerate))
	complex := compileT(sampleRM(types.ScenarioArchitectureExplain, types.IntentExplain, types.ComplexityComplex))
	if !(simple.EvidencePlan.Budget.MaxFiles < moderate.EvidencePlan.Budget.MaxFiles &&
		moderate.EvidencePlan.Budget.MaxFiles < complex.EvidencePlan.Budget.MaxFiles) {
		t.Fatalf("budget not monotonic: simple=%d moderate=%d complex=%d",
			simple.EvidencePlan.Budget.MaxFiles, moderate.EvidencePlan.Budget.MaxFiles,
			complex.EvidencePlan.Budget.MaxFiles)
	}
}

func TestInferScenario(t *testing.T) {
	cases := []struct {
		name   string
		rm     types.RequestModel
		expect types.Scenario
	}{
		{"config intent", types.RequestModel{Intent: types.IntentConfigQuery}, types.ScenarioConfigTrace},
		{"root cause intent", types.RequestModel{Intent: types.IntentRootCause}, types.ScenarioRootCause},
		{
			"single-topic structural trace uses generic scenario",
			types.RequestModel{
				Intent:        types.IntentTrace,
				PredicateAxis: types.AxisCall,
				AnalyzerHints: types.AnalyzerHints{Kind: "call_chain"},
			},
			types.ScenarioGeneric,
		},
		{"explain default", types.RequestModel{Intent: types.IntentExplain}, types.ScenarioArchitectureExplain},
		{
			"perf terms trigger perf scenario",
			types.RequestModel{Intent: types.IntentExplain, TermGraph: types.TermGraph{
				Canonical: []types.CanonicalTerm{{ID: "en:bottleneck"}},
			}},
			types.ScenarioPerformanceBottleneck,
		},
		{
			"zh perf term triggers perf scenario",
			types.RequestModel{Intent: types.IntentExplain, TermGraph: types.TermGraph{
				Canonical: []types.CanonicalTerm{{ID: "zh:性能"}},
			}},
			types.ScenarioPerformanceBottleneck,
		},
	}
	for _, c := range cases {
		if got := InferScenario(c.rm); got != c.expect {
			t.Errorf("%s: got %q want %q", c.name, got, c.expect)
		}
	}
}

func TestCompile_LanguagePropagatesToContract(t *testing.T) {
	rm := sampleRM(types.ScenarioArchitectureExplain, types.IntentExplain, types.ComplexityModerate)
	rm.Language = "en"
	out := compileT(rm)
	if out.AnswerContract.Language != "en" {
		t.Fatalf("language not propagated: %q", out.AnswerContract.Language)
	}
}

// Over-fit audit: each template must always produce a non-empty graph
// with at least one probe → evidence → finalize path and a
// fully-valid DAG (no dangling edges, no duplicate node IDs).
func TestCompile_AllTemplatesStructurallyValid(t *testing.T) {
	scenarios := []types.Scenario{
		types.ScenarioArchitectureExplain,
		types.ScenarioRootCause,
		types.ScenarioConfigTrace,
		types.ScenarioPerformanceBottleneck,
		types.ScenarioGeneric,
	}
	for _, sc := range scenarios {
		out := compileT(sampleRM(sc, types.IntentExplain, types.ComplexityModerate))
		if len(out.TaskGraph.Nodes) == 0 {
			t.Errorf("%s: empty node set", sc)
			continue
		}
		ids := make(map[string]bool)
		for _, n := range out.TaskGraph.Nodes {
			if ids[n.ID] {
				t.Errorf("%s: duplicate node id %q", sc, n.ID)
			}
			ids[n.ID] = true
		}
		for _, e := range out.TaskGraph.Edges {
			if !ids[e.From] || !ids[e.To] {
				t.Errorf("%s: dangling edge %+v", sc, e)
			}
		}
		if countNodeType(out.TaskGraph, types.NodeProbe) == 0 {
			t.Errorf("%s: missing probe node", sc)
		}
		if countNodeType(out.TaskGraph, types.NodeFinalize) == 0 {
			t.Errorf("%s: missing finalize node", sc)
		}
		if out.EvidencePlan.Budget.MaxFiles == 0 {
			t.Errorf("%s: empty evidence budget", sc)
		}
	}
}
