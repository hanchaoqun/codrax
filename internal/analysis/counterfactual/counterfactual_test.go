package counterfactual

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func baseGraph() types.TaskGraph {
	return types.TaskGraph{
		Nodes: []types.TaskNode{
			{ID: "n0", Type: types.NodeProbe},
			{ID: "n1", Type: types.NodeEvidence},
			{ID: "n2", Type: types.NodeFinalize},
		},
		Edges: []types.TaskEdge{
			{From: "n0", To: "n1", EdgeType: types.EdgeHardDependency},
			{From: "n1", To: "n2", EdgeType: types.EdgeHardDependency},
		},
	}
}

func TestShouldExpand_GatedByComplexity(t *testing.T) {
	rm := types.RequestModel{
		Intent:      types.IntentRootCause,
		Complexity:  types.ComplexityModerate,
		Ambiguities: []types.Ambiguity{{Clause: "x", Options: []string{"a", "b"}}},
	}
	if ShouldExpand(rm) {
		t.Fatal("moderate complexity must not trigger expansion")
	}
	rm.Complexity = types.ComplexityComplex
	if !ShouldExpand(rm) {
		t.Fatal("complex + ambiguity + root_cause must trigger expansion")
	}
}

func TestShouldExpand_GatedByIntent(t *testing.T) {
	rm := types.RequestModel{
		Intent:      types.IntentConfigQuery,
		Complexity:  types.ComplexityComplex,
		Ambiguities: []types.Ambiguity{{Clause: "x", Options: []string{"a", "b"}}},
	}
	if ShouldExpand(rm) {
		t.Fatal("config_query intent must not trigger expansion")
	}
}

func TestShouldExpand_RequiresMultipleOptions(t *testing.T) {
	rm := types.RequestModel{
		Intent:      types.IntentExplain,
		Complexity:  types.ComplexityComplex,
		Ambiguities: []types.Ambiguity{{Clause: "x", Options: []string{"only one"}}},
	}
	if ShouldExpand(rm) {
		t.Fatal("single-option ambiguity must not trigger expansion")
	}
}

func TestExpand_DisabledIsNoop(t *testing.T) {
	tg := baseGraph()
	out, ids := Expand(tg, types.RequestModel{}, Options{Enabled: false})
	if len(ids) != 0 {
		t.Fatalf("disabled expansion must return empty ID list; got %v", ids)
	}
	if len(out.Nodes) != len(tg.Nodes) || len(out.Edges) != len(tg.Edges) {
		t.Fatalf("disabled expansion must not touch graph")
	}
}

func TestExpand_InsertsTwoBranchesAndReconcile(t *testing.T) {
	tg := baseGraph()
	rm := types.RequestModel{
		Ambiguities: []types.Ambiguity{
			{Clause: "stop", Options: []string{"soft", "hard"}},
		},
	}
	out, newIDs := Expand(tg, rm, Options{Enabled: true, MaxBranches: 1})
	if len(newIDs) != 2 {
		t.Fatalf("expected 2 counterfactual nodes per ambiguity; got %v", newIDs)
	}

	// Reconcile node must exist.
	reconcile := findFirstNode(out, types.NodeReconcile)
	if reconcile == "" {
		t.Fatal("expected reconcile node to be inserted")
	}
	// Finalize must now be reached via reconcile.
	finalize := findFirstNode(out, types.NodeFinalize)
	hasReconcileToFinalize := false
	for _, e := range out.Edges {
		if e.From == reconcile && e.To == finalize {
			hasReconcileToFinalize = true
		}
	}
	if !hasReconcileToFinalize {
		t.Fatal("reconcile must feed finalize")
	}
	// The original n1 → n2 edge must have been rewired.
	for _, e := range out.Edges {
		if e.From == "n1" && e.To == "n2" && e.EdgeType == types.EdgeHardDependency {
			t.Fatalf("original n1→n2 edge should have been rewired through reconcile; edges=%+v", out.Edges)
		}
	}
	// Every counterfactual node must connect to reconcile.
	for _, cfID := range newIDs {
		found := false
		for _, e := range out.Edges {
			if e.From == cfID && e.To == reconcile {
				found = true
			}
		}
		if !found {
			t.Fatalf("counterfactual node %s not wired to reconcile", cfID)
		}
	}
}

func TestExpand_ReusesExistingReconcile(t *testing.T) {
	tg := types.TaskGraph{
		Nodes: []types.TaskNode{
			{ID: "n0", Type: types.NodeProbe},
			{ID: "n1", Type: types.NodeEvidence},
			{ID: "r", Type: types.NodeReconcile},
			{ID: "f", Type: types.NodeFinalize},
		},
		Edges: []types.TaskEdge{
			{From: "n0", To: "n1", EdgeType: types.EdgeHardDependency},
			{From: "n1", To: "r", EdgeType: types.EdgeHardDependency},
			{From: "r", To: "f", EdgeType: types.EdgeHardDependency},
		},
	}
	rm := types.RequestModel{
		Ambiguities: []types.Ambiguity{{Clause: "c", Options: []string{"a", "b"}}},
	}
	out, _ := Expand(tg, rm, Options{Enabled: true, MaxBranches: 1})
	reconcileCount := 0
	for _, n := range out.Nodes {
		if n.Type == types.NodeReconcile {
			reconcileCount++
		}
	}
	if reconcileCount != 1 {
		t.Fatalf("should reuse existing reconcile; got %d", reconcileCount)
	}
}

func TestExpand_MaxBranchesLimit(t *testing.T) {
	tg := baseGraph()
	rm := types.RequestModel{
		Ambiguities: []types.Ambiguity{
			{Clause: "a1", Options: []string{"x", "y"}},
			{Clause: "a2", Options: []string{"p", "q"}},
			{Clause: "a3", Options: []string{"m", "n"}},
		},
	}
	_, newIDs := Expand(tg, rm, Options{Enabled: true, MaxBranches: 2})
	// Two ambiguities × 2 options = 4 nodes.
	if len(newIDs) != 4 {
		t.Fatalf("MaxBranches=2 with 3 ambiguities should emit 4 nodes; got %d", len(newIDs))
	}
}

func TestExpand_DoesNotMutateInput(t *testing.T) {
	tg := baseGraph()
	originalNodes := len(tg.Nodes)
	originalEdges := len(tg.Edges)
	rm := types.RequestModel{
		Ambiguities: []types.Ambiguity{{Clause: "x", Options: []string{"a", "b"}}},
	}
	_, _ = Expand(tg, rm, Options{Enabled: true, MaxBranches: 1})
	if len(tg.Nodes) != originalNodes || len(tg.Edges) != originalEdges {
		t.Fatal("Expand must not mutate input graph")
	}
}
