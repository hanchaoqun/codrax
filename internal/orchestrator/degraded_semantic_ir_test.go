// Package orchestrator — degraded_semantic_ir_test.go (2026-05-10).
//
// Fix-A of the analyzer-failure remediation: buildDegradedSemanticIR
// preserves the analyzer's last successful partial emit (RequestModel
// + AnalyzerHints + Predicates) and includes a richer TaskGraph
// (probe → evidence → reconcile → finalize) so explorer + extractor
// still run when the analyzer's quality gate exhausted retries.
package orchestrator

import (
	"errors"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// === buildDegradedSemanticIR ===

func TestBuildDegradedSemanticIR_NilPartial_DelegatesToLegacy(t *testing.T) {
	got := buildDegradedSemanticIR("user q", nil, errors.New("test"))
	if got == nil {
		t.Fatal("got nil IR")
	}
	// Legacy path: single finalize node, IntentExplain default.
	if len(got.TaskGraph.Nodes) != 1 {
		t.Errorf("nil partial: expected single finalize node (legacy); got %d nodes", len(got.TaskGraph.Nodes))
	}
	if got.TaskGraph.Nodes[0].Type != types.NodeFinalize {
		t.Errorf("nil partial: expected NodeFinalize; got %v", got.TaskGraph.Nodes[0].Type)
	}
	if got.RequestModel.Intent != types.IntentExplain {
		t.Errorf("nil partial: expected IntentExplain default; got %v", got.RequestModel.Intent)
	}
}

func TestBuildDegradedSemanticIR_PreservesAnalyzerHints(t *testing.T) {
	partial := &types.AnalysisIR{}
	partial.RequestModel.Intent = types.IntentRootCause
	partial.RequestModel.Scenario = types.ScenarioRootCause
	partial.RequestModel.Complexity = types.ComplexityModerate
	partial.RequestModel.Language = "zh"
	partial.RequestModel.AnalyzerHints.Keywords = []string{"KeyError", "RuntimeError", "config"}
	partial.RequestModel.AnalyzerHints.Entities = []string{"load_config"}
	partial.RequestModel.AnalyzerHints.PrimaryEntities = []string{"load_config"}
	partial.RequestModel.AnalyzerHints.Kind = "call_chain"
	partial.RequestModel.AnswerSubject = types.AnswerSubject{Kind: types.SubjectFunctionName, Confidence: 0.8}

	got := buildDegradedSemanticIR("explain this traceback", partial, errors.New("R2.2 storm"))
	if got == nil {
		t.Fatal("got nil IR")
	}
	if got.RequestModel.Intent != types.IntentRootCause {
		t.Errorf("intent should be preserved; got %v", got.RequestModel.Intent)
	}
	if got.RequestModel.Scenario != types.ScenarioRootCause {
		t.Errorf("scenario should be preserved; got %v", got.RequestModel.Scenario)
	}
	if got.RequestModel.Language != "zh" {
		t.Errorf("language should be preserved; got %v", got.RequestModel.Language)
	}
	if len(got.RequestModel.AnalyzerHints.Keywords) != 3 {
		t.Errorf("keywords should be preserved; got %v", got.RequestModel.AnalyzerHints.Keywords)
	}
	if len(got.RequestModel.AnalyzerHints.Entities) != 1 || got.RequestModel.AnalyzerHints.Entities[0] != "load_config" {
		t.Errorf("entities should be preserved; got %v", got.RequestModel.AnalyzerHints.Entities)
	}
	// Non-scalar AnswerSubject preserved (function_name is not in
	// the scalar list).
	if got.RequestModel.AnswerSubject.Kind != types.SubjectFunctionName {
		t.Errorf("non-scalar AnswerSubject should be preserved; got %v", got.RequestModel.AnswerSubject.Kind)
	}
}

func TestBuildDegradedSemanticIR_AutoCorrectsScalarSubject(t *testing.T) {
	partial := &types.AnalysisIR{}
	partial.RequestModel.AnswerSubject = types.AnswerSubject{Kind: types.SubjectStringLiteral, Confidence: 0.9}
	got := buildDegradedSemanticIR("q", partial, errors.New("test"))
	if got.RequestModel.AnswerSubject.Kind != types.SubjectUnknown {
		t.Errorf("scalar string_literal should be cleared to Unknown; got %v", got.RequestModel.AnswerSubject.Kind)
	}
	if got.RequestModel.AnswerSubject.Confidence != 0 {
		t.Errorf("confidence should be 0; got %v", got.RequestModel.AnswerSubject.Confidence)
	}
}

func TestBuildDegradedSemanticIR_RicherTaskGraph(t *testing.T) {
	partial := &types.AnalysisIR{}
	partial.RequestModel.Intent = types.IntentExplain
	got := buildDegradedSemanticIR("q", partial, errors.New("test"))
	// 4 nodes: probe → evidence → reconcile → finalize.
	if len(got.TaskGraph.Nodes) != 4 {
		t.Fatalf("expected 4 nodes (probe→evidence→reconcile→finalize); got %d", len(got.TaskGraph.Nodes))
	}
	wantTypes := []types.TaskNodeType{
		types.NodeProbe,
		types.NodeEvidence,
		types.NodeReconcile,
		types.NodeFinalize,
	}
	for i, want := range wantTypes {
		if got.TaskGraph.Nodes[i].Type != want {
			t.Errorf("node[%d] type = %v, want %v", i, got.TaskGraph.Nodes[i].Type, want)
		}
	}
}

func TestBuildDegradedSemanticIR_CitationReqRelaxed(t *testing.T) {
	partial := &types.AnalysisIR{}
	got := buildDegradedSemanticIR("q", partial, errors.New("test"))
	if got.AnswerContract.CitationReq.Required {
		t.Error("CitationReq.Required should be false (relaxed) in degraded path")
	}
}

func TestBuildDegradedSemanticIR_GateReportObservability(t *testing.T) {
	partial := &types.AnalysisIR{}
	partial.RequestModel.AnalyzerHints.Keywords = []string{"foo"}
	got := buildDegradedSemanticIR("q", partial, errors.New("R2.2 storm"))
	if got.QualityGate.Rejected {
		t.Error("QualityGate should not be rejected (recovery path passed)")
	}
	found := false
	for _, c := range got.QualityGate.Checks {
		if c.Name == "degraded_semantic_recovery" && c.Passed {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected degraded_semantic_recovery PASS check for observability")
	}
}

func TestBuildDegradedSemanticIR_PreservesPredicates(t *testing.T) {
	partial := &types.AnalysisIR{}
	partial.RequestModel.Predicates = types.SemanticPredicates{
		IsScalarAnswer:        true,
		IsCountQuestion:       true,
		IsCategoryEnumeration: false,
	}
	partial.RequestModel.PredicateAxis = types.AxisDefine
	got := buildDegradedSemanticIR("q", partial, errors.New("test"))
	if !got.RequestModel.Predicates.IsCountQuestion {
		t.Error("IsCountQuestion predicate should be preserved")
	}
	if !got.RequestModel.Predicates.IsScalarAnswer {
		t.Error("IsScalarAnswer predicate should be preserved")
	}
	if got.RequestModel.PredicateAxis != types.AxisDefine {
		t.Errorf("PredicateAxis should be preserved; got %v", got.RequestModel.PredicateAxis)
	}
}

func TestBuildDegradedSemanticIR_DroppedFieldsFromPartial(t *testing.T) {
	// Verify that EvidencePlan / HypothesisSet / TermGraph from a
	// malformed partial are NOT carried forward (they were
	// malformed enough to fail the gate; preserving them risks
	// downstream contract failures).
	partial := &types.AnalysisIR{}
	partial.RequestModel.AnalyzerHints.Keywords = []string{"foo"}
	got := buildDegradedSemanticIR("q", partial, errors.New("test"))
	// EvidencePlan should be zero-value (legacy hardening: dropped).
	if len(got.EvidencePlan.RequiredFiles) > 0 {
		t.Errorf("EvidencePlan should be dropped; got RequiredFiles=%v", got.EvidencePlan.RequiredFiles)
	}
}

// === dedupedStrings + fallback* ===

func TestDedupedStrings_Empty(t *testing.T) {
	if got := dedupedStrings(nil); got != nil {
		t.Errorf("nil: expected nil; got %v", got)
	}
	if got := dedupedStrings([]string{"", "  "}); got != nil {
		t.Errorf("blanks-only: expected nil; got %v", got)
	}
}

func TestDedupedStrings_DedupsAndTrims(t *testing.T) {
	got := dedupedStrings([]string{"foo", "foo", " bar ", "bar", "baz"})
	if len(got) != 3 {
		t.Fatalf("got %v, want 3 entries", got)
	}
	if got[0] != "foo" {
		t.Errorf("got[0] = %q, want foo", got[0])
	}
}

func TestFallbackIntent_Empty_Default(t *testing.T) {
	if got := fallbackIntent(""); got != types.IntentExplain {
		t.Errorf("empty: expected IntentExplain default; got %v", got)
	}
}

func TestFallbackIntent_NonEmpty_PassThrough(t *testing.T) {
	if got := fallbackIntent(types.IntentRootCause); got != types.IntentRootCause {
		t.Errorf("non-empty: expected pass-through; got %v", got)
	}
}

func TestFallbackScenario_Empty_Default(t *testing.T) {
	if got := fallbackScenario(""); got != types.ScenarioGeneric {
		t.Errorf("empty: expected ScenarioGeneric; got %v", got)
	}
}

func TestFallbackComplexity_Empty_Default(t *testing.T) {
	if got := fallbackComplexity(""); got != types.ComplexitySimple {
		t.Errorf("empty: expected ComplexitySimple; got %v", got)
	}
}
