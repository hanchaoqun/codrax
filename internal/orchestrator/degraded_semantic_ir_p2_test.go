// Package orchestrator — degraded_semantic_ir_p2_test.go (2026-05-10).
//
// P2 audit follow-up to Fix-A (e502fd9): the degraded recovery IR
// must preserve the typed L3 / L4 channels (RequiredFileHints /
// IrrelevantFiles) and adjacent multi-repo lanes (PrimaryScopes /
// MentionedEntities / DerivedEntities / ExactContextRoles) from
// the analyzer's last successful partial emit. Dropping these
// signals would force the explorer to rebuild file-relevance
// judgement from scratch using the deterministic resolver, after
// the LLM had already produced strong typed signals.
package orchestrator

import (
	"errors"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBuildDegradedSemanticIR_PreservesRequiredFileHints(t *testing.T) {
	partial := &types.AnalysisIR{}
	partial.RequestModel.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{
		{Path: "internal/agent/analyzer.go", Confidence: 0.95, Rationale: "primary"},
		{Path: "internal/orchestrator/orchestrator.go", Confidence: 0.6, Rationale: "soft"},
	}
	got := buildDegradedSemanticIR("q", partial, errors.New("test"))
	if len(got.RequestModel.AnalyzerHints.RequiredFileHints) != 2 {
		t.Fatalf("RequiredFileHints not preserved; got %d", len(got.RequestModel.AnalyzerHints.RequiredFileHints))
	}
	want := partial.RequestModel.AnalyzerHints.RequiredFileHints
	if !reflect.DeepEqual(got.RequestModel.AnalyzerHints.RequiredFileHints, want) {
		t.Errorf("hint slice mismatch; got %v, want %v",
			got.RequestModel.AnalyzerHints.RequiredFileHints, want)
	}
}

func TestBuildDegradedSemanticIR_PreservesIrrelevantFiles(t *testing.T) {
	partial := &types.AnalysisIR{}
	partial.RequestModel.AnalyzerHints.IrrelevantFiles = []string{
		"internal/types/violation_registry.go",
		"docs/legacy.md",
	}
	got := buildDegradedSemanticIR("q", partial, errors.New("test"))
	if len(got.RequestModel.AnalyzerHints.IrrelevantFiles) != 2 {
		t.Errorf("IrrelevantFiles not preserved; got %v",
			got.RequestModel.AnalyzerHints.IrrelevantFiles)
	}
}

func TestBuildDegradedSemanticIR_PreservesPrimaryScopes(t *testing.T) {
	partial := &types.AnalysisIR{}
	partial.RequestModel.AnalyzerHints.PrimaryScopes = []string{"repo-greet-go", "repo-tools-py"}
	got := buildDegradedSemanticIR("q", partial, errors.New("test"))
	if len(got.RequestModel.AnalyzerHints.PrimaryScopes) != 2 {
		t.Errorf("PrimaryScopes not preserved; got %v",
			got.RequestModel.AnalyzerHints.PrimaryScopes)
	}
}

func TestBuildDegradedSemanticIR_PreservesMentionedAndDerivedEntities(t *testing.T) {
	partial := &types.AnalysisIR{}
	partial.RequestModel.AnalyzerHints.MentionedEntities = []string{"gate.Run"}
	partial.RequestModel.AnalyzerHints.DerivedEntities = []string{"GateCheck", "QualityGate"}
	got := buildDegradedSemanticIR("q", partial, errors.New("test"))
	if len(got.RequestModel.AnalyzerHints.MentionedEntities) != 1 {
		t.Errorf("MentionedEntities not preserved")
	}
	if len(got.RequestModel.AnalyzerHints.DerivedEntities) != 2 {
		t.Errorf("DerivedEntities not preserved")
	}
}

func TestBuildDegradedSemanticIR_PreservesExactContextRoles(t *testing.T) {
	partial := &types.AnalysisIR{}
	partial.RequestModel.AnalyzerHints.ExactContextRoles = []types.EvidenceDiagramRole{
		types.EvidenceDiagramRoleDefault, types.EvidenceDiagramRoleConfig,
	}
	got := buildDegradedSemanticIR("q", partial, errors.New("test"))
	if len(got.RequestModel.AnalyzerHints.ExactContextRoles) != 2 {
		t.Errorf("ExactContextRoles not preserved; got %v",
			got.RequestModel.AnalyzerHints.ExactContextRoles)
	}
}

func TestBuildDegradedSemanticIR_NilPartial_NoNilDeref(t *testing.T) {
	// Defensive: nil partial → fall back to legacy IR; zero
	// AnalyzerHints fields, no panic from nil hint slices.
	got := buildDegradedSemanticIR("q", nil, errors.New("test"))
	if got == nil {
		t.Fatal("nil partial → expected non-nil IR (legacy fallback)")
	}
	if len(got.RequestModel.AnalyzerHints.RequiredFileHints) != 0 {
		t.Errorf("nil partial should produce empty hints; got %v",
			got.RequestModel.AnalyzerHints.RequiredFileHints)
	}
}
