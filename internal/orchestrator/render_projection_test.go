package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderInvestigationUnitForEvidenceNodeFallsBackToOrdinal(t *testing.T) {
	plan := types.InvestigationPlan{Units: []types.InvestigationUnit{
		{ID: "unit-a", Index: 1, Label: "调度链"},
		{ID: "unit-b", Index: 2, Label: "算力供给"},
	}}

	if unit, ok := renderInvestigationUnitForEvidenceNode(plan, "renamed_evidence_a", 0, 2); !ok || unit.ID != "unit-a" {
		t.Fatalf("ordinal fallback should bind first evidence node to unit-a, got ok=%t unit=%+v", ok, unit)
	}
	if unit, ok := renderInvestigationUnitForEvidenceNode(plan, "renamed_evidence_b", 1, 2); !ok || unit.ID != "unit-b" {
		t.Fatalf("ordinal fallback should bind second evidence node to unit-b, got ok=%t unit=%+v", ok, unit)
	}
	if unit, ok := renderInvestigationUnitForEvidenceNode(plan, "n1_evidence_t1", 0, 2); !ok || unit.ID != "unit-b" {
		t.Fatalf("explicit _tN suffix should remain authoritative, got ok=%t unit=%+v", ok, unit)
	}
	if unit, ok := renderInvestigationUnitForEvidenceNode(plan, "renamed_evidence_extra", 0, 3); ok {
		t.Fatalf("ordinal fallback must not guess when counts diverge, got %+v", unit)
	}
}

func TestRenderVisibleEvidenceNodeCount(t *testing.T) {
	nodes := []types.TaskNode{
		{ID: "probe", Type: types.NodeProbe},
		{ID: "ev-a", Type: types.NodeEvidence},
		{ID: "ev-hidden", Type: types.NodeEvidence, IsCounterfactual: true},
		{ID: "ev-b", Type: types.NodeEvidence},
		{ID: "final", Type: types.NodeFinalize},
	}
	if got := renderVisibleEvidenceNodeCount(nodes); got != 2 {
		t.Fatalf("visible evidence count = %d, want 2", got)
	}
}
