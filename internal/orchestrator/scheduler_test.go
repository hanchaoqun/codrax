package orchestrator

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// scheduler_test.go locks the graphState abstraction and the
// node-type→stage mapping. These are pure-data tests; they do not
// dispatch any agent, so they can assert exact-state invariants.

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

func TestGraphState_ReadyOrderRespectsHardEdges(t *testing.T) {
	s := newGraphState(smallChainGraph())

	// Initially only n0 is ready (no preds).
	ready := s.readyNodes()
	if len(ready) != 1 || ready[0].ID != "n0" {
		t.Fatalf("first ready: want [n0], got %v", idsOf(ready))
	}

	// After n0 done, n1 should be ready alone.
	s.markRunning("n0")
	s.markDone("n0")
	ready = s.readyNodes()
	if len(ready) != 1 || ready[0].ID != "n1" {
		t.Errorf("after n0 done: want [n1], got %v", idsOf(ready))
	}
}

func TestGraphState_PendingExplorerWindowSkipsFinalize(t *testing.T) {
	s := newGraphState(smallChainGraph())
	// Walk the chain so n3 (finalize) becomes ready.
	for _, id := range []string{"n0", "n1", "n2"} {
		s.markRunning(id)
		s.markDone(id)
	}
	if w := s.pendingExplorerWindow(); len(w) != 0 {
		t.Errorf("explorer window after chain done: want empty, got %v", idsOf(w))
	}
	fin := s.firstFinalizeReady()
	if fin == nil || fin.ID != "n3" {
		t.Errorf("finalize ready: want n3, got %v", fin)
	}
}

func TestGraphState_FeedbackTargetWalksValidationEdge(t *testing.T) {
	s := newGraphState(smallChainGraph())
	// n2 has a validation_feedback edge to n1.
	target := s.feedbackTarget("n2")
	if target != "n1" {
		t.Errorf("feedback from n2: want n1, got %q", target)
	}
	// No feedback edge out of n0.
	if got := s.feedbackTarget("n0"); got != "" {
		t.Errorf("feedback from n0: want \"\", got %q", got)
	}
}

func TestGraphState_FeedbackTargetRespectsMaxRetries(t *testing.T) {
	g := smallChainGraph()
	// n1 has MaxRetries=0 (unlimited per the schema), n2 has MaxRetries=2.
	// Force n1 to have a per-node cap so feedback should refuse re-entry.
	for i := range g.Nodes {
		if g.Nodes[i].ID == "n1" {
			g.Nodes[i].MaxRetries = 1
		}
	}
	s := newGraphState(g)
	s.markRunning("n1") // attempts[n1]=1
	s.markRunning("n1") // attempts[n1]=2
	if got := s.feedbackTarget("n2"); got != "" {
		t.Errorf("feedback to n1 with attempts=2 cap=1+1: want refused, got %q", got)
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

func TestGraphState_ReadyDeprioritizesCounterfactual(t *testing.T) {
	g := smallChainGraph()
	cf := types.TaskNode{ID: "ncf", Type: types.NodeEvidence, Objective: "alt", IsCounterfactual: true}
	g.Nodes = append(g.Nodes, cf)
	g.Edges = append(g.Edges, types.TaskEdge{From: "n0", To: "ncf", EdgeType: types.EdgeHardDependency})
	s := newGraphState(g)
	s.markRunning("n0")
	s.markDone("n0")
	ready := s.readyNodes()
	// Both n1 and ncf should be ready, n1 (non-counterfactual) first.
	if len(ready) < 2 {
		t.Fatalf("want at least 2 ready, got %v", idsOf(ready))
	}
	if ready[0].ID != "n1" {
		t.Errorf("non-counterfactual should sort first; got %v", idsOf(ready))
	}
	// Counterfactual must appear, just not first.
	found := false
	for _, n := range ready {
		if n.ID == "ncf" {
			found = true
		}
	}
	if !found {
		t.Error("counterfactual node missing from ready list")
	}
}

func TestGraphState_AllDoneOnlyWhenTerminal(t *testing.T) {
	s := newGraphState(smallChainGraph())
	if s.allDone() {
		t.Fatal("fresh state should not be allDone")
	}
	for _, id := range []string{"n0", "n1", "n2", "n3"} {
		s.markRunning(id)
		s.markDone(id)
	}
	if !s.allDone() {
		t.Error("after all done should be allDone")
	}
}

func TestStageMapping_AllNodeTypes(t *testing.T) {
	g := types.TaskGraph{
		Nodes: []types.TaskNode{
			{Type: types.NodeImplement, ID: "imp"}, // presence enables code_review branch
		},
	}
	cases := []struct {
		nt      types.TaskNodeType
		writing bool
		want    types.PipelineStage
	}{
		{types.NodeProbe, false, types.StageExplore},
		{types.NodeEvidence, false, types.StageExplore},
		{types.NodeValidate, false, types.StageExplore},
		{types.NodeReconcile, false, types.StageExplore},
		{types.NodeDesign, true, types.StagePlan},
		{types.NodeImplement, true, types.StageImplement},
		{types.NodeReview, false, types.StageDesignReview},
		{types.NodeReview, true, types.StageCodeReview},
		{types.NodeVerify, true, types.StageVerify},
		{types.NodeFinalize, false, types.StageFinalize},
	}
	for _, c := range cases {
		got, err := stageMapping(g, &types.TaskNode{Type: c.nt}, c.writing)
		if err != nil {
			t.Errorf("nt=%s writing=%v: unexpected err %v", c.nt, c.writing, err)
			continue
		}
		if got != c.want {
			t.Errorf("nt=%s writing=%v: want %s, got %s", c.nt, c.writing, c.want, got)
		}
	}
}

func TestStageMapping_UnknownTypeFailsLoud(t *testing.T) {
	g := types.TaskGraph{}
	if _, err := stageMapping(g, &types.TaskNode{Type: "bogus"}, false); err == nil {
		t.Error("want error for unknown node type, got nil")
	}
}

func TestStageMapping_ReviewWithoutImplementMapsToDesignReview(t *testing.T) {
	g := types.TaskGraph{Nodes: []types.TaskNode{
		{ID: "n0", Type: types.NodeProbe},
		{ID: "n1", Type: types.NodeReview},
	}}
	got, err := stageMapping(g, &types.TaskNode{Type: types.NodeReview}, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != types.StageDesignReview {
		t.Errorf("review without implement: want design_review, got %s", got)
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
	surfaces := map[string]string{
		"k1": "explorer", "k2": "tool", "e1": "Explorer",
	}
	hint := renderWindowHint(window, func(id string) string { return surfaces[id] }, "")
	for _, want := range []string{"Locate the explorer agent", "Read its tools", "explorer", "Explorer", "tool"} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint missing %q\n%s", want, hint)
		}
	}
}

func TestRenderWindowHint_PrependsViolationPreamble(t *testing.T) {
	hint := renderWindowHint(nil, func(string) string { return "" },
		"citation count too low; collect more file:line anchors")
	if !strings.Contains(hint, "previous final answer") {
		t.Errorf("missing preamble: %s", hint)
	}
	if !strings.Contains(hint, "citation count too low") {
		t.Errorf("missing violation text: %s", hint)
	}
}

func TestRenderWindowHint_EmptyAllReturnsEmpty(t *testing.T) {
	if got := renderWindowHint(nil, func(string) string { return "" }, ""); got != "" {
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
	if got := fn("missing"); got != "" {
		t.Errorf("missing id should return empty; got %q", got)
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

// reflect-based zero check for use in stricter tests if needed.
var _ = reflect.DeepEqual
