package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/types"
)

// scheduler_test.go locks the graphState abstraction and the
// node-type→stage mapping.

func smallChainGraph() types.TaskGraph {
	probe := types.TaskNode{ID: "n0", Type: types.NodeProbe, Objective: "scan"}
	ev := types.TaskNode{ID: "n1", Type: types.NodeEvidence, Objective: "collect"}
	val := types.TaskNode{ID: "n2", Type: types.NodeValidate, Objective: "check", MaxRetries: 2}
	final := types.TaskNode{ID: "n3", Type: types.NodeFinalize, Objective: "answer"}
	return types.TaskGraph{
		Nodes: []types.TaskNode{probe, ev, val, final},
		Edges: []types.TaskEdge{
			{From: "n0", To: "n1", EdgeType: types.EdgeHardDependency},
			{From: "n1", To: "n2", EdgeType: types.EdgeHardDependency},
			{From: "n2", To: "n3", EdgeType: types.EdgeHardDependency},
			{From: "n2", To: "n1", EdgeType: types.EdgeValidationFeedback, Guard: "any_unknown"},
		},
		ExecutionPolicy: types.ExecutionPolicy{
			MaxParallelism: 1, RetryBudget: 3,
			CriticalPath: []string{"n0", "n1", "n2", "n3"},
		},
	}
}

func emptyEnv() criterion.Env {
	return criterion.Env{}
}

func TestGraphState_ReadyWindow_MergedInitial(t *testing.T) {
	s := newGraphState(smallChainGraph())
	window, _ := s.readyExplorerWindow(emptyEnv())
	// Merged-window schedule: every non-finalize node is ready on
	// round 1 regardless of hard-dep chain.
	if len(window) != 3 {
		t.Fatalf("first window: want 3 nodes (merged), got %v", idsOf(window))
	}
	// Mark all done and verify window empties.
	for _, n := range window {
		s.markDone(n.ID)
	}
	window, _ = s.readyExplorerWindow(emptyEnv())
	if len(window) != 0 {
		t.Errorf("after marking all done: want empty, got %v", idsOf(window))
	}
}

func TestGraphState_WindowExcludesFinalize(t *testing.T) {
	s := newGraphState(smallChainGraph())
	for _, id := range []string{"n0", "n1", "n2"} {
		s.markRunning(id)
		s.markDone(id)
	}
	window, _ := s.readyExplorerWindow(emptyEnv())
	if len(window) != 0 {
		t.Errorf("window after chain done: want empty, got %v", idsOf(window))
	}
	fin := s.firstFinalizeReadyMerged()
	if fin == nil || fin.ID != "n3" {
		t.Errorf("finalize ready: want n3, got %v", fin)
	}
}

func TestGraphState_RetryBudgetExhausted(t *testing.T) {
	s := newGraphState(smallChainGraph())
	if s.retryBudgetExhausted() {
		t.Fatal("fresh state should not be exhausted (budget=3)")
	}
	s.recordRetry()
	s.recordRetry()
	s.recordRetry()
	if !s.retryBudgetExhausted() {
		t.Error("after 3 retries with budget=3 should be exhausted")
	}
}

func TestGraphState_RequeueValidationTargets(t *testing.T) {
	s := newGraphState(smallChainGraph())
	// Walk to validate stage.
	for _, id := range []string{"n0", "n1", "n2"} {
		s.markRunning(id)
		s.markDone(id)
	}
	targets := s.requeueValidationTargets("n2")
	// Validation feedback: n2 → n1 (from smallChainGraph edges).
	if len(targets) != 1 || targets[0] != "n1" {
		t.Fatalf("want [n1], got %v", targets)
	}
	if s.status["n1"] != nodeRequeued || s.status["n2"] != nodeRequeued {
		t.Errorf("n1/n2 should be requeued; n1=%v n2=%v", s.status["n1"], s.status["n2"])
	}
	if s.status["n0"] != nodeDone {
		t.Error("n0 (not a validation target) must stay done")
	}
}

func TestGraphState_ForceCloseExploreWindow(t *testing.T) {
	s := newGraphState(smallChainGraph())
	s.forceCloseExploreWindow()
	for _, id := range []string{"n0", "n1", "n2"} {
		if s.status[id] != nodeDone {
			t.Errorf("%s should be forced to done; got %v", id, s.status[id])
		}
	}
	if s.status["n3"] == nodeDone {
		t.Error("finalize must not be force-closed")
	}
}

func TestGraphState_EntryConditionBlocks(t *testing.T) {
	g := smallChainGraph()
	// Gate n1 on has_enough_facts. Env.Signals is zero → blocked.
	g.Nodes[1].EntryConditions = []types.Criterion{
		{Kind: types.CritHasEnoughFacts},
	}
	s := newGraphState(g)
	window, blocked := s.readyExplorerWindow(emptyEnv())
	// n0, n2 ready (no gate); n1 blocked by entry condition.
	if len(window) != 2 {
		t.Errorf("want 2 ready nodes (n0, n2); got %v", idsOf(window))
	}
	foundBlocked := false
	for _, b := range blocked {
		if b.NodeID == "n1" {
			foundBlocked = true
		}
	}
	if !foundBlocked {
		t.Error("n1 should be blocked on entry condition")
	}
	env := criterion.Env{Signals: types.ExecutionSignals{HasEnoughFacts: true}}
	window, _ = s.readyExplorerWindow(env)
	if len(window) != 3 {
		t.Errorf("after signal: want 3 nodes; got %v", idsOf(window))
	}
}

func TestStageMapping_AllNodeTypes(t *testing.T) {
	g := types.TaskGraph{}
	cases := []struct {
		nt   types.TaskNodeType
		want types.PipelineStage
	}{
		{types.NodeProbe, types.StageExplore},
		{types.NodeEvidence, types.StageExplore},
		{types.NodeValidate, types.StageExplore},
		{types.NodeReconcile, types.StageExplore},
		{types.NodeFinalize, types.StageFinalize},
	}
	for _, c := range cases {
		got, err := stageMapping(g, &types.TaskNode{Type: c.nt}, false)
		if err != nil {
			t.Errorf("nt=%s: %v", c.nt, err)
			continue
		}
		if got != c.want {
			t.Errorf("nt=%s: want %s, got %s", c.nt, c.want, got)
		}
	}
}

func TestStageMapping_UnknownTypeFailsLoud(t *testing.T) {
	g := types.TaskGraph{}
	if _, err := stageMapping(g, &types.TaskNode{Type: "bogus"}, false); err == nil {
		t.Error("want error for unknown node type")
	}
}

func TestRenderWindowHint_StructuralOutput(t *testing.T) {
	window := []*types.TaskNode{
		{
			ID: "n0", Type: types.NodeProbe, Objective: "Locate the explorer agent",
			SearchHints: types.SearchHints{KeywordIDs: []string{"k1"}, EntityIDs: []string{"e1"}},
		},
		{
			ID: "n1", Type: types.NodeEvidence, Objective: "Read its tools",
			SearchHints: types.SearchHints{KeywordIDs: []string{"k2"}},
		},
	}
	surfaces := map[string]string{"k1": "explorer", "k2": "tool", "e1": "Explorer"}
	hint := renderWindowHint(window, nil, nil, func(id string) string { return surfaces[id] }, "")
	for _, want := range []string{"Locate the explorer agent", "Read its tools", "explorer", "Explorer", "tool"} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint missing %q\n%s", want, hint)
		}
	}
}

func TestRenderWindowHint_PrependsViolationPreamble(t *testing.T) {
	hint := renderWindowHint(nil, nil, nil, func(string) string { return "" },
		"citation count too low; collect more file:line anchors")
	if !strings.Contains(hint, "previous final answer") {
		t.Errorf("missing preamble: %s", hint)
	}
	if !strings.Contains(hint, "citation count too low") {
		t.Errorf("missing violation text: %s", hint)
	}
}

func TestRenderWindowHint_ValidationTargets(t *testing.T) {
	hint := renderWindowHint(nil, nil, []string{"n1_evidence"}, func(string) string { return "" }, "")
	if !strings.Contains(hint, "n1_evidence") {
		t.Errorf("expected validation target name in hint: %s", hint)
	}
	if !strings.Contains(hint, "Validation rejected") {
		t.Errorf("expected validation-reject preamble in hint: %s", hint)
	}
}

func TestRenderWindowHint_EmptyAllReturnsEmpty(t *testing.T) {
	if got := renderWindowHint(nil, nil, nil, func(string) string { return "" }, ""); got != "" {
		t.Errorf("empty inputs should yield empty hint; got %q", got)
	}
}

func TestTermSurfaceLookup_NilSafe(t *testing.T) {
	fn := termSurfaceLookup(nil)
	if got := fn("any"); got != "" {
		t.Errorf("nil IR lookup should return empty; got %q", got)
	}
}

func TestTermSurfaceLookup_ResolvesKnownID(t *testing.T) {
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			TermGraph: types.TermGraph{
				Canonical: []types.CanonicalTerm{
					{ID: "k1", Surface: "explorer"},
					{ID: "k2", Surface: "Finalizer"},
				},
			},
		},
	}
	fn := termSurfaceLookup(ir)
	if got := fn("k1"); got != "explorer" {
		t.Errorf("k1: want explorer, got %q", got)
	}
	if got := fn("k2"); got != "Finalizer" {
		t.Errorf("k2: want Finalizer, got %q", got)
	}
}

// ── helpers ──────────────────────────────────────────────────────

func idsOf(ns []*types.TaskNode) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.ID
	}
	return out
}
