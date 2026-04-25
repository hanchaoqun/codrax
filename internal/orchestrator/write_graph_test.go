package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// BuildWriteTaskGraph shape contracts: each Mode produces the right
// node set + edges + EntryConditions/SuccessCriteria.

func TestBuildWriteTaskGraph_ModePlan(t *testing.T) {
	g := BuildWriteTaskGraph(types.ModePlan, "", 0)
	if len(g.Nodes) != 1 {
		t.Fatalf("ModePlan should produce 1 node; got %d", len(g.Nodes))
	}
	if g.Nodes[0].Type != types.NodePlan {
		t.Errorf("node type = %q; want NodePlan", g.Nodes[0].Type)
	}
	if !g.Nodes[0].OneShot {
		t.Errorf("plan node should be OneShot=true")
	}
	if len(g.Edges) != 0 {
		t.Errorf("ModePlan should have no edges; got %d", len(g.Edges))
	}
}

func TestBuildWriteTaskGraph_ModeApply_FullChain(t *testing.T) {
	g := BuildWriteTaskGraph(types.ModeApply, "", 2)
	if len(g.Nodes) != 3 {
		t.Fatalf("ModeApply (no pre-supplied plan) should have 3 nodes; got %d", len(g.Nodes))
	}
	wantTypes := []types.TaskNodeType{types.NodePlan, types.NodeApply, types.NodeVerify}
	for i, want := range wantTypes {
		if g.Nodes[i].Type != want {
			t.Errorf("node[%d] type = %q; want %q", i, g.Nodes[i].Type, want)
		}
	}
	// Edges: plan→apply (hard), apply→verify (hard), verify→plan (validation_feedback).
	wantEdges := map[string]types.EdgeType{
		"plan->apply":   types.EdgeHardDependency,
		"apply->verify": types.EdgeHardDependency,
		"verify->plan":  types.EdgeValidationFeedback,
	}
	got := map[string]types.EdgeType{}
	for _, e := range g.Edges {
		got[e.From+"->"+e.To] = e.EdgeType
	}
	for k, v := range wantEdges {
		if got[k] != v {
			t.Errorf("edge %s: got %q; want %q", k, got[k], v)
		}
	}
	if g.ExecutionPolicy.RetryBudget != 2 {
		t.Errorf("RetryBudget = %d; want 2", g.ExecutionPolicy.RetryBudget)
	}
	if g.Nodes[1].OneShot { // apply
		t.Errorf("apply must not be OneShot — verify→plan retry must be able to requeue it")
	}
	if g.Nodes[2].OneShot { // verify
		t.Errorf("verify must not be OneShot")
	}
}

// R8a: when planPath is set, ModeApply skips the plan node (the user
// has a reviewed plan; do not regenerate). And no retry cycle —
// re-running the same reviewed plan is pointless after a verify
// failure, plus clearForReplan wipes PlanPath which would crash
// applyPreHook on retry.
func TestBuildWriteTaskGraph_ModeApply_PreSuppliedPlanSkipsPlanNode(t *testing.T) {
	g := BuildWriteTaskGraph(types.ModeApply, "/tmp/plan.json", 2)
	if len(g.Nodes) != 2 {
		t.Fatalf("apply with pre-supplied plan should have 2 nodes (apply, verify); got %d", len(g.Nodes))
	}
	if g.Nodes[0].Type != types.NodeApply || g.Nodes[1].Type != types.NodeVerify {
		t.Errorf("expected [apply, verify]; got [%s, %s]", g.Nodes[0].Type, g.Nodes[1].Type)
	}
	// apply's EntryCondition (CritPlanReady) is dropped because the
	// applyPreHook will load the plan + seed PendingApplies before
	// dispatch.
	if len(g.Nodes[0].EntryConditions) != 0 {
		t.Errorf("pre-supplied-plan apply should have no EntryConditions; got %v", g.Nodes[0].EntryConditions)
	}
	// No retry cycle in pre-supplied-plan mode (see comment above).
	for _, e := range g.Edges {
		if e.EdgeType == types.EdgeValidationFeedback {
			t.Errorf("pre-supplied-plan mode should have no validation_feedback edges; got %+v", e)
		}
	}
}

func TestBuildWriteTaskGraph_ModeVerify(t *testing.T) {
	g := BuildWriteTaskGraph(types.ModeVerify, "/tmp/plan.json", 0)
	if len(g.Nodes) != 1 {
		t.Fatalf("ModeVerify should produce 1 node; got %d", len(g.Nodes))
	}
	if g.Nodes[0].Type != types.NodeVerify {
		t.Errorf("node type = %q; want NodeVerify", g.Nodes[0].Type)
	}
	if len(g.Nodes[0].EntryConditions) != 0 {
		t.Errorf("verify-only mode should have no EntryConditions; got %v", g.Nodes[0].EntryConditions)
	}
}

func TestBuildWriteTaskGraph_ModeRead_EmptyDefensive(t *testing.T) {
	g := BuildWriteTaskGraph(types.ModeRead, "", 0)
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Errorf("read mode should never reach this builder; expected empty graph; got nodes=%d edges=%d",
			len(g.Nodes), len(g.Edges))
	}
}

func TestIsWriteGraph(t *testing.T) {
	cases := []struct {
		name   string
		nodes  []types.TaskNodeType
		expect bool
	}{
		{"empty", nil, false},
		{"read_only", []types.TaskNodeType{types.NodeProbe, types.NodeFinalize}, false},
		{"plan_only", []types.TaskNodeType{types.NodePlan}, true},
		{"full_write", []types.TaskNodeType{types.NodePlan, types.NodeApply, types.NodeVerify}, true},
		{"verify_only", []types.TaskNodeType{types.NodeVerify}, true},
		{"mixed", []types.TaskNodeType{types.NodeProbe, types.NodeApply}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var nodes []types.TaskNode
			for _, nt := range c.nodes {
				nodes = append(nodes, types.TaskNode{ID: string(nt), Type: nt})
			}
			g := types.TaskGraph{Nodes: nodes}
			if got := IsWriteGraph(g); got != c.expect {
				t.Errorf("IsWriteGraph = %v; want %v", got, c.expect)
			}
		})
	}
}
